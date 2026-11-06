// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
)

type carryoverImmediateLane struct{}

func (carryoverImmediateLane) Enqueue(fn func()) { fn() }
func TestCarryoverAdmissionPrecedesEvenSynchronousCompletion(t *testing.T) {
	f := newTitleFixture(t, true)
	f.all()
	f.h.post = func(fn func()) { fn() }
	f.h.lane = carryoverImmediateLane{}
	count := 0
	completed := false
	accepted := f.h.commandOnceAdmitted(f.s, "synchronous-admission", func() { count++ }, func() error {
		if count != 1 {
			t.Fatal("writer before admission")
		}
		return nil
	}, func(error) {
		if count != 1 {
			t.Fatal("completion before admission")
		}
		count--
		completed = true
	})
	if !accepted || !completed || count != 0 {
		t.Fatal("synchronous lease leak")
	}
	called := false
	delete(f.h.windows, f.s.titleWindowID)
	if f.h.commandOnceAdmitted(f.s, "detached", func() { called = true }, func() error { return nil }, nil) || called {
		t.Fatal("detached command admitted")
	}
}
func TestCarryoverSourceLeasesCountOnlyAcceptedCommands(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	source := carryoverSourceKey{tr.ChainID, tr.ID}
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	openCarryoverMenu(t, f, carryoverDefer)
	openReturnMenu(t, tt, tr.ID)
	if err := tt.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.ui()
	tt.Frame()
	confirmCarryover(t, f)
	tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.h.carryoverInFlight[source] != 1 || len(f.io.q) != 1 {
		t.Fatal("duplicate acquired source lease")
	}
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.h.carryoverInFlight[source] != 2 || len(f.io.q) != 2 {
		t.Fatal("distinct target missing lease")
	}
	f.tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEscape)
	f.ui()
	if f.h.carryoverInFlight[source] != 2 {
		t.Fatal("cancel released admitted source")
	}
	empty := newRuntimeApp(f.h.readView())
	f.h.attach(empty, func() {}, func() {})
	if !f.h.beforeClose(empty) {
		t.Fatal("empty unrelated sibling cannot detach")
	}
	f.io.one()
	f.posts.one()
	if f.h.carryoverInFlight[source] != 2 {
		t.Fatal("publication released before completion")
	}
	f.posts.one()
	if f.h.carryoverInFlight[source] != 1 {
		t.Fatal("first completion released competitor")
	}
	f.all()
	if len(f.h.carryoverInFlight) != 0 || len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("completion leaked source lease")
	}
}
func TestCarryoverFailureRecoveryExplicitAddAcknowledgmentClearsReason(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	titleStoreFault(f.svc, func(at string) error {
		if at == "before-reserve" {
			return errors.New("synthetic")
		}
		return nil
	})
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "用户稍后明确提交", ui.TextInputRange{Start: 8, End: 8})
	f.all()
	tt.Frame()
	f.ui()
	if f.h.beforeClose(other) {
		t.Fatal("unconfirmed input lost")
	}
	titleStoreFault(f.svc, nil)
	liveClick(t, tt, label)
	tt.Key(0, ui.KeyEnter)
	f.all()
	tt.Frame()
	f.ui()
	if len(f.svc.Snapshot().Subtasks) != 1 || len(f.svc.Snapshot().Traces) != 1 {
		t.Fatal("explicit Add failed or carryover replayed")
	}
	if other.deferredComposerRecovery() != nil || other.hasSubtaskComposerFailure() {
		t.Fatal("real Add/native clear did not release recovery")
	}
	if !f.h.beforeClose(other) {
		t.Fatal("acknowledged blank cannot close")
	}
}
func TestCarryoverAdvanceAndStoppedFailuresReleaseOnlyTheirLease(t *testing.T) {
	for _, mode := range []string{"advance-failure", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			other := newRuntimeApp(f.h.readView())
			f.h.attach(other, func() {}, func() {})
			other.runtimeSelection = string(tr.ID)
			other.runtimeDayRail = true
			tt := ui.NewTester(other.view, 1200, 768)
			f.ui()
			tt.Frame()
			openCarryoverMenu(t, f, carryoverDefer)
			confirmCarryover(t, f)
			calls := 0
			if mode == "stopped" {
				f.h.stopped = true
			} else {
				f.clock.now = f.clock.now.Add(24 * time.Hour)
				titleStoreFault(f.svc, func(at string) error {
					if at == "before-reserve" {
						calls++
						return errors.New("advance unavailable")
					}
					return nil
				})
			}
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			tt.Type("不能丢的晚输入")
			f.ui()
			f.all()
			tt.Frame()
			f.ui()
			if len(f.h.carryoverInFlight) != 0 || f.s.carryover != nil || f.s.runtimeError == "" {
				t.Fatal("failure lease/presentation not released")
			}
			if f.h.beforeClose(other) {
				t.Fatal("failure released native input")
			}
			if mode == "advance-failure" {
				// The existing natural-day observer and the explicit command
				// are two separate admitted boundaries, neither retries itself.
				if calls != 2 {
					t.Fatal("unexpected boundary attempts", calls)
				}
				for i := 0; i < 4; i++ {
					f.tt.Frame()
					f.all()
				}
				if calls != 2 {
					t.Fatal("failed boundary replayed itself", calls)
				}
			}
			if len(f.svc.Snapshot().Traces) != 1 {
				t.Fatal("failure wrote carryover")
			}
		})
	}
}
func TestCarryoverSiblingQuitAndLateExplicitAddRemainProtected(t *testing.T) {
	for _, mode := range []string{"quit", "late-after-quit", "late-add"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			other := newRuntimeApp(f.h.readView())
			f.h.attach(other, func() {}, func() {})
			other.runtimeSelection = string(tr.ID)
			other.runtimeDayRail = true
			tt := ui.NewTester(other.view, 1200, 768)
			f.ui()
			tt.Frame()
			openCarryoverMenu(t, f, carryoverDefer)
			confirmCarryover(t, f)
			if mode == "late-after-quit" {
				if f.h.beforeQuit() || !f.h.quitting {
					t.Fatal("initial quit did not wait for writer")
				}
			}
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			tt.Type("晚提交输入")
			f.ui()
			if mode == "quit" {
				if f.h.beforeQuit() || f.h.quitting {
					t.Fatal("quit recorded discard of protected input")
				}
			} else if mode == "late-add" {
				tt.Key(0, ui.KeyEnter)
				f.ui()
			}
			f.all()
			tt.Frame()
			f.ui()
			if f.quit != 0 || len(f.svc.Snapshot().Traces) != 2 || len(f.svc.Snapshot().Subtasks) != 0 {
				t.Fatal("auto quit or unsafe Add")
			}
			if f.h.beforeQuit() {
				t.Fatal("unresolved recovery allowed quit")
			}
			liveClick(t, tt, label)
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Key(0, ui.KeyBackspace)
			f.ui()
			tt.Frame()
			if !f.h.beforeQuit() {
				t.Fatal("explicit cleared recovery blocked quit")
			}
		})
	}
}
func TestCarryoverActualEditablePredicateMatchesUnknownAndLockedSources(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	target := runtimeSubtaskTarget{chain: tr.ChainID, trace: tr.ID}
	g := f.svc.Snapshot()
	if !actualSubtaskTargetEditable(g, liveTestDay, target) || !f.s.canAddSubtask(target) {
		t.Fatal("renderer predicate mismatch")
	}
	missing := g.Clone()
	missing.Traces = nil
	if actualSubtaskTargetEditable(missing, liveTestDay, target) {
		t.Fatal("lookup failure guessed editable")
	}
	tomorrow, _ := liveTestDay.AddDays(1)
	if actualSubtaskTargetEditable(g, tomorrow, target) {
		t.Fatal("wrong natural day")
	}
}
