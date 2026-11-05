// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"testing"
)

func TestReviewReadViewTwoWritesStayFIFO(t *testing.T) {
	f := newTitleFixture(t, false)
	f.h.command(f.s, func() error { return f.s.runtime.AddPool("A") }, false, nil)
	f.h.command(f.s, func() error { return f.s.runtime.AddPool("B") }, false, nil)
	f.ui()
	f.io.one()
	f.io.one()
	if len(f.svc.Snapshot().Chains) != 2 {
		t.Fatal("writes did not persist")
	}
	if len(f.s.runtime.Snapshot().Chains) != 0 {
		t.Fatal("UI read through to unposted writes")
	}
	want := []int{1, 1, 2, 2}
	for _, n := range want {
		if len(f.posts.q) == 0 {
			t.Fatal("missing publication/completion")
		}
		f.posts.one()
		if got := len(f.s.runtime.Snapshot().Chains); got != n {
			t.Fatalf("publication order got %d want %d", got, n)
		}
	}
	f.all()
}
func TestReviewReadViewFailureKeepsVerifiedGraph(t *testing.T) {
	f := newTitleFixture(t, false)
	f.h.command(f.s, func() error { return f.s.runtime.AddPool("A") }, false, nil)
	f.all()
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return errors.New("review disk failure")
		}
		return nil
	})
	f.h.command(f.s, func() error { return f.s.runtime.AddPool("UNCONFIRMED") }, false, nil)
	f.all()
	for _, g := range []int{len(f.s.runtime.Snapshot().Chains), len(f.svc.Snapshot().Chains)} {
		if g != 1 {
			t.Fatalf("failed candidate leaked, chains=%d", g)
		}
	}
	if f.s.runtimeError == "" {
		t.Fatal("missing failure")
	}
}
func TestReviewPoolOldCompletionDoesNotDismissNewPicker(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("A"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	chain := f.svc.Snapshot().Chains[0].ID
	liveClick(t, f.tt, "从任务池排期")
	f.ui()
	liveClick(t, f.tt, "排期任务 "+string(chain))
	f.ui()
	// Cancel first picker, then choose another date and open a fresh picker.
	liveClick(t, f.tt, "取消从任务池排期")
	f.ui()
	liveClick(t, f.tt, "后一天")
	f.ui()
	liveClick(t, f.tt, "从任务池排期")
	f.ui()
	newerSelection := f.s.runtimeSelection
	f.all()
	if !f.s.runtimePoolPicker || f.s.runtimeSelection != newerSelection {
		t.Fatalf("old schedule completion hijacked newer picker: open=%v selection=%s date=%v", f.s.runtimePoolPicker, f.s.runtimeSelection, f.s.runtimeDate)
	}
}
func TestReviewPoolRepeatedClickSchedulesOnce(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("A"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	chain := f.svc.Snapshot().Chains[0].ID
	liveClick(t, f.tt, "从任务池排期")
	f.ui()
	liveClick(t, f.tt, "排期任务 "+string(chain))
	liveClick(t, f.tt, "排期任务 "+string(chain))
	f.ui()
	if len(f.io.q) != 1 {
		t.Errorf("repeat click enqueued %d writers, want 1", len(f.io.q))
	}
	f.all()
	if f.s.runtimeError != "" {
		t.Errorf("second request masked saved schedule with error: %s", f.s.runtimeError)
	}
	if len(f.svc.Snapshot().Traces) != 1 {
		t.Errorf("trace count=%d", len(f.svc.Snapshot().Traces))
	}
}
func TestReviewPoolListRepeatSchedule(t *testing.T) {
	for _, label := range []string{"排到今天 ", "排到明天 "} {
		t.Run(label, func(t *testing.T) {
			f := newTitleFixture(t, false)
			if err := f.svc.AddPool("A"); err != nil {
				t.Fatal(err)
			}
			f.h.publish(f.svc.Snapshot(), liveTestDay)
			f.ui()
			chain := f.svc.Snapshot().Chains[0].ID
			liveClick(t, f.tt, "切换到任务池")
			f.all()
			liveClick(t, f.tt, label+string(chain))
			liveClick(t, f.tt, label+string(chain))
			f.ui()
			if len(f.io.q) != 1 {
				t.Errorf("repeat list action enqueued %d writes", len(f.io.q))
			}
			f.all()
			if f.s.runtimeError != "" {
				t.Errorf("success masked: %s", f.s.runtimeError)
			}
		})
	}
}
func TestReviewCompletedListRepeatUndo(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddToday("A"); err != nil {
		t.Fatal(err)
	}
	trace := f.svc.Snapshot().Traces[0].ID
	if err := f.svc.SetCompleted(trace, true); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	liveClick(t, f.tt, "切换到已完成")
	f.all()
	liveClick(t, f.tt, "切换完成 "+string(trace))
	liveClick(t, f.tt, "切换完成 "+string(trace))
	f.ui()
	if len(f.io.q) != 1 {
		t.Errorf("repeat undo enqueued %d writes", len(f.io.q))
	}
	f.all()
	if f.s.runtimeError != "" {
		t.Errorf("success masked: %s", f.s.runtimeError)
	}
}

func TestReviewCrossWindowSameActionSharesPendingKey(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("A"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddPool("B"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	chains := f.svc.Snapshot().Chains
	key := "schedule:" + string(chains[0].ID) + ":" + liveTestDay.String()
	f.s.runAction(key, func() error { return f.s.runtime.Schedule(chains[0].ID, liveTestDay) })
	other.runAction(key, func() error { t.Fatal("duplicate cross-window action executed"); return nil })
	other.runAction("schedule:"+string(chains[1].ID)+":"+liveTestDay.String(), func() error { return other.runtime.Schedule(chains[1].ID, liveTestDay) })
	f.ui()
	if len(f.io.q) != 2 {
		t.Fatal("same-action dedupe swallowed independent command", len(f.io.q))
	}
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 || f.s.runtimeError != "" || other.runtimeError != "" {
		t.Fatal("pending action publication failed")
	}
}
