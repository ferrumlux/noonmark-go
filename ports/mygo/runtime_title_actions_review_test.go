// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewActionFailureReleasesKey(t *testing.T) {
	f := newTitleFixture(t, false)
	calls := 0
	f.s.runAction("same", func() error { calls++; return errors.New("transient") })
	f.s.runAction("same", func() error { t.Fatal("duplicate executed"); return nil })
	f.all()
	f.s.runAction("same", func() error { calls++; return nil })
	f.all()
	if calls != 2 || len(f.h.actionPending) != 0 || f.s.runtimeError != "" {
		t.Fatalf("retry/key cleanup failed calls=%d pending=%v error=%s", calls, f.h.actionPending, f.s.runtimeError)
	}
}
func TestReviewTwoWindowActualClicksDedupOnlySame(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("A"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddPool("B"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	liveClick(t, f.tt, "切换到任务池")
	f.all()
	other := newRuntimeApp(f.h.readView())
	other.page = "pool"
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	chains := f.svc.Snapshot().Chains
	label := "排到今天 " + string(chains[0].ID)
	liveClick(t, f.tt, label)
	liveClick(t, tt, label)
	liveClick(t, tt, "排到今天 "+string(chains[1].ID))
	f.ui()
	if len(f.io.q) != 2 {
		t.Fatalf("expected two independent actions, got %d", len(f.io.q))
	}
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 || f.s.runtimeError != "" || other.runtimeError != "" {
		t.Fatal("actual cross-window actions failed")
	}
}
