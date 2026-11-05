// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"testing"
	"time"
)

func TestReviewInitialGapDirtyClose(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddToday("初始"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), f.svc.Today())
	f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
	f.s.runtimeDayRail = true
	f.tt.Frame()
	liveClick(t, f.tt, "日任务详情标题")
	titleDirectClient(t, f.tt).ReplaceText(nil, "新")
	if f.s.titleEditor.registered {
		t.Fatal("registration gap absent")
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("premature close")
	}
	f.all()
	if f.persisted() != "初始新" || f.closed != 1 {
		t.Fatal(f.persisted(), f.closed)
	}
}
func TestReviewStaleNavigationAfterClose(t *testing.T) {
	f := newTitleFixture(t, true)
	f.s.titleTransition(func() { t.Fatal("closed navigation invoked") })
	if !f.h.beforeClose(f.s) {
		t.Fatal("clean close blocked")
	}
	f.ui()
	if len(f.h.targets) != 0 {
		t.Fatalf("closed window retained %d navigation closure(s)", len(f.h.targets))
	}
}
func TestReviewIdleNativeOwnerStable(t *testing.T) {
	f := newTitleFixture(t, true)
	liveClick(t, f.tt, "日任务详情标题")
	client := titleDirectClient(t, f.tt)
	for range 10 {
		f.tt.Frame()
		f.ui()
	}
	client.ReplaceText(nil, "稳定")
	if f.h.beforeClose(f.s) {
		t.Fatal("premature close")
	}
	f.all()
	if f.persisted() != "原题稳定" {
		t.Fatal(f.persisted())
	}
}
func TestReviewPostCloseTimersNoReopen(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("已存")
	if f.h.beforeClose(f.s) {
		t.Fatal("premature close")
	}
	f.all()
	f.advance(time.Hour)
	f.all()
	if len(f.h.windows) != 0 || f.s.titleEditor != nil || len(f.io.q) != 0 || f.closed != 1 {
		t.Fatal("stale callback revived closed window")
	}
}
func TestReviewSupersededNavigationReleasesClosure(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("保留")
	f.s.titleTransition(func() { t.Fatal("superseded closure invoked") })
	f.ui()
	f.s.titleTransition(func() { f.s.page = "pool" })
	f.ui()
	f.all()
	if len(f.h.targets) != 0 {
		t.Fatalf("completed navigation retained %d superseded closure(s)", len(f.h.targets))
	}
}
