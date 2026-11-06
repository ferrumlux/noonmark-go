package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewerOffscreenEmptyNativeFinishAllowsSiblingDefer(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	tt.Type("explicitly cleared")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	other.titleTransition(func() { other.page = "pool"; other.runtimeSelection = "" })
	f.all()
	tt.Frame()
	f.ui()
	for _, c := range other.subtaskComposers {
		if c.mounted || !c.finalizedValid || c.finalizedText != "" {
			t.Fatal("offscreen native Finish proof missing")
		}
	}
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("empty offscreen input incorrectly blocked")
	}
	if _, ok := tt.Find("查看原任务子任务草稿"); ok {
		t.Fatal("empty native proof produced false recovery")
	}
}

func TestReviewerInFlightSiblingCloseProtectsDeliveredMarkedDraft(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "关闭前已交付", ui.TextInputRange{Start: 6, End: 6})
	if f.h.beforeClose(other) {
		t.Fatal("sibling close discarded affected in-flight native draft before publication")
	}
	if f.h.windows[other.titleWindowID] == nil {
		t.Fatal("sibling detached despite draft")
	}
	f.all()
	tt.Frame()
	f.ui()
	if _, ok := tt.Find(label); !ok {
		t.Fatal("blocked-close draft not recoverable after commit")
	}
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	f.ui()
	if !f.h.beforeClose(other) {
		t.Fatal("explicit native clear did not permit close")
	}
}
