package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewerCompetingCompletionDoesNotRevokeRecoveryLease(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	second := newRuntimeApp(f.h.readView())
	f.h.attach(second, func() {}, func() {})
	st := ui.NewTester(second.view, 1200, 768)
	recovery := newRuntimeApp(f.h.readView())
	f.h.attach(recovery, func() {}, func() {})
	recovery.runtimeSelection = string(tr.ID)
	recovery.runtimeDayRail = true
	rt := ui.NewTester(recovery.view, 1200, 768)
	f.ui()
	rt.Frame()
	openCarryoverMenu(t, f, carryoverDefer)
	openReturnMenu(t, st, tr.ID)
	if err := st.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.ui()
	st.Frame()
	st.Key(0, ui.KeyRight)
	confirmCarryover(t, f)
	st.Key(0, ui.KeyEnter)
	f.ui()
	if len(f.io.q) != 2 {
		t.Fatal("expected two admitted distinct targets", len(f.io.q))
	}
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, rt, label)
	titleDirectClient(t, rt).SetMarkedText(nil, "首条完成前候选", ui.TextInputRange{Start: 7, End: 7})
	f.io.one()
	f.ui()
	rt.Frame()
	liveClick(t, rt, label)
	native := titleDirectClient(t, rt)
	native.SetMarkedText(nil, "恢复字段新租约", ui.TextInputRange{Start: 7, End: 7})
	f.io.one()
	f.ui()
	native.SetMarkedText(nil, "失败完成后继续输入", ui.TextInputRange{Start: 9, End: 9})
	var owner *runtimeSubtaskComposer
	for _, c := range recovery.subtaskComposers {
		if c.target.trace == tr.ID {
			owner = c
		}
	}
	snap, ok := owner.native.handle.TextInputSnapshot(owner.native.context)
	if !ok || snap.MarkedText != "失败完成后继续输入" {
		t.Fatal("losing command completion revoked established recovery lease", snap, ok)
	}
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("competing commands did not leave one winner")
	}
}
