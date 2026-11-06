package main

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewerCarryoverOtherWindowFailedComposerExplicitClear(t *testing.T) {
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
	tt.Type("failed draft")
	titleStoreFault(f.svc, func(at string) error {
		if at == "before-reserve" {
			return errors.New("synthetic")
		}
		return nil
	})
	tt.Key(0, ui.KeyEnter)
	f.all()
	tt.Frame()
	for _, c := range other.subtaskComposers {
		if c.failure == nil {
			t.Fatal("setup: expected failed Add")
		}
	}
	titleStoreFault(f.svc, nil)
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	f.ui()
	tt.Frame()
	// User explicitly clears in original window, then starts operation in sibling.
	openReturnMenu(t, f.tt, tr.ID)
	if err := f.tt.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.all()
	f.tt.Frame()
	if f.s.carryover == nil {
		t.Fatal("explicit native clear in original window did not release sibling operation:", f.s.runtimeError)
	}
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("clear recovery failed")
	}
}

func TestReviewerCarryoverSiblingLateComposerRemainsRecoverable(t *testing.T) {
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
	// The command is accepted but writer deliberately has not run yet.
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	tt.Type("late unsubmitted native draft")
	f.ui()
	tt.Frame()
	f.all()
	tt.Frame()
	f.ui()
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("setup: accepted mutation did not persist")
	}
	for _, c := range other.subtaskComposers {
		if c.native.draft == "late unsubmitted native draft" && !c.mounted {
			t.Fatal("late sibling native composer is stranded after source became read-only; no recovery field mounted")
		}
	}
}

func TestReviewerCarryoverPendingTargetDoesNotVisuallyChange(t *testing.T) {
	f := newTitleFixture(t, true)
	openCarryoverMenu(t, f, carryoverDefer)
	p := f.s.carryover
	target := p.picker.selected
	confirmCarryover(t, f)
	f.tt.Key(0, ui.KeyRight)
	f.ui()
	f.tt.Frame()
	if p.picker.selected != target {
		t.Fatal("pending sheet displays changed target while queued mutation still uses original target", target, p.picker.selected)
	}
	f.all()
	if f.s.runtimeDate != target {
		t.Fatal("confirmed target changed")
	}
}
