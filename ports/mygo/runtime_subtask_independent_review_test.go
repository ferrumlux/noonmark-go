// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func TestReviewSubtaskOldMenuAcrossReattachedSession(t *testing.T) {
	f, _, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	liveClick(t, f.tt, prefix+".difficulty")
	old := independentNativeMenuCallback(t, f.tt)
	if !f.h.beforeClose(f.s) {
		t.Fatal("clean close blocked")
	}
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.svc, err = taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	f.h = newTitleHub(f.svc, &f.io, f.posts.Enqueue, &f.clock, f.clock.Now, func() {})
	f.h.attach(f.s, func() { f.invalid = true }, func() {})
	f.tt.Frame()
	f.all()
	before := f.svc.Snapshot()
	old(3)
	f.tt.Frame()
	f.all()
	if !reflect.DeepEqual(before, f.svc.Snapshot()) {
		t.Fatalf("old difficulty callback crossed closed session: %v -> %v", before.Subtasks[0].Difficulty, f.svc.Snapshot().Subtasks[0].Difficulty)
	}
}

func TestReviewSubtaskMenuCancelledAndNavigated(t *testing.T) {
	for _, mode := range []string{"cancel-reopen", "navigation-roundtrip", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			f, trace, child := subtaskFixture(t)
			prefix := subtaskPrefix("day", string(child.ID))
			liveClick(t, f.tt, prefix+".difficulty")
			old := independentNativeMenuCallback(t, f.tt)
			switch mode {
			case "cancel-reopen":
				f.tt.CloseMenu()
				liveClick(t, f.tt, prefix+".difficulty")
			case "navigation-roundtrip":
				f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
				f.all()
				f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
				f.all()
			case "deleted":
				if err := f.svc.DeleteSubtask(trace.ChainID, trace.ID, child.ID); err != nil {
					t.Fatal(err)
				}
				f.h.publish(f.svc.Snapshot(), liveTestDay)
				f.all()
			}
			before := f.svc.Snapshot()
			old(3)
			f.tt.Frame()
			f.all()
			if !reflect.DeepEqual(before, f.svc.Snapshot()) {
				t.Fatal("stale difficulty menu changed graph")
			}
		})
	}
}

func TestReviewSubtaskFailedComposerClose(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "提交失败必须保留")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if len(f.svc.Snapshot().Subtasks) != 0 || f.s.runtimeError == "" {
		t.Fatal("failure setup failed")
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("window accepted close after failed child add; unsaved composer draft is now unreachable")
	}
}

func TestReviewSubtaskCompletedChildStillEditable(t *testing.T) {
	f, _, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	liveClick(t, f.tt, prefix+".completion")
	f.all()
	subtaskEdit(t, f, prefix+".title", "已完成但父pending可改")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	liveClick(t, f.tt, prefix+".difficulty")
	if err := f.tt.ChooseMenuItem("困难"); err != nil {
		t.Fatal(err)
	}
	f.all()
	if got := subtaskPersisted(t, f, child.ID); got.Title != "已完成但父pending可改" || got.Difficulty != core.DifficultyHard || got.Status != core.SubtaskCompleted {
		t.Fatal(got)
	}
	liveClick(t, f.tt, prefix+".delete")
	f.all()
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskCancelledDraft {
		t.Fatal("completed child could not delete")
	}
}

func TestReviewSubtaskComposerReturnAfterMarkedLateAck(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "第一条已提交")
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "第二条候选", ui.TextInputRange{Start: 6, End: 6})
	f.all()
	var composer *runtimeSubtaskComposer
	for _, c := range f.s.subtaskComposers {
		if c.mounted {
			composer = c
		}
	}
	if composer == nil || !composer.hasAcknowledged || len(f.svc.Snapshot().Subtasks) != 1 {
		t.Fatalf("marked late ack setup failed: %+v", composer)
	}
	// OS commits the next text between frames, then the user submits it once.
	titleDirectClient(t, f.tt).ReplaceText(nil, "第二条确认")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	g := f.svc.Snapshot()
	if len(g.Subtasks) != 2 || g.Subtasks[1].Title != "第二条确认" {
		t.Fatalf("Return consumed by obsolete acknowledgment: children=%+v, draft=%q, ack=%v", g.Subtasks, composer.native.draft, composer.hasAcknowledged)
	}
}

func TestReviewSubtaskFailedComposerOffscreenCloseAndQuit(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "离屏失败草稿")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
	f.all()
	if f.s.page != "pool" {
		t.Fatal("navigation unexpectedly blocked")
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("offscreen failed composer lost on close")
	}
	if f.h.beforeQuit() {
		t.Fatal("offscreen failed composer lost on quit")
	}
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
	f.all()
	for _, c := range f.s.subtaskComposers {
		if c.mounted && c.native.draft != "离屏失败草稿" {
			t.Fatal("return destroyed original failure draft")
		}
	}
	titleStoreFault(f.svc, nil)
	liveClick(t, f.tt, label)
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if len(f.svc.Snapshot().Subtasks) != 1 {
		t.Fatal("retry did not persist")
	}
	if !f.h.beforeClose(f.s) {
		t.Fatal("acknowledged retry still blocks close")
	}
}

func TestReviewSubtaskFailedComposerExplicitClear(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "失败后明确清空")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if f.h.beforeClose(f.s) {
		t.Fatal("failed draft did not block close")
	}
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).ReplaceText(nil, "") // Native clear delivered between frames.
	if !f.h.beforeClose(f.s) {
		t.Fatal("explicit native clear did not permit close")
	}
	if len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("clear submitted unwanted child")
	}
}
