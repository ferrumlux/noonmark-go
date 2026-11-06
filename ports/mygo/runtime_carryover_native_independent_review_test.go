package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestReviewerCarryoverLateMarkedComposerNativeOwnerSurvives(t *testing.T) {
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
	var original *runtimeSubtaskComposer
	for _, c := range other.subtaskComposers {
		if c.target.trace == tr.ID {
			original = c
		}
	}
	if original == nil {
		t.Fatal("missing native composer")
	}
	target := original.target
	titleDirectClient(t, tt).SetMarkedText(nil, "已交付但未同步候选", ui.TextInputRange{Start: 9, End: 9})
	// Do not run a build frame that might synchronize the application's draft.
	f.all()
	tt.Frame()
	f.ui()
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("setup: commit missing")
	}
	snapshot, ok := original.native.handle.TextInputSnapshot(original.native.context)
	if !ok || !strings.Contains(snapshot.Text+snapshot.MarkedText, "已交付但未同步候选") {
		t.Fatal("confirmed history lost already delivered native text", snapshot, ok)
	}
	if original.target != target || !original.mounted {
		t.Fatal("original native composer is not reachable for recovery")
	}
	if len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("unsubmitted text automatically created a task")
	}
}
