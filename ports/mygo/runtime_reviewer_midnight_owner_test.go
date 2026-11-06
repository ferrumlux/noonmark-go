package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestReviewerMidnightAutofollowCannotDiscardFailedNativeFinish(t *testing.T) {
	for _, failFinish := range []bool{false, true} {
		t.Run(map[bool]string{false: "finish-success", true: "finish-failure"}[failFinish], func(t *testing.T) {
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
			// An actual native menu action leaves this window's title editor finalized
			// while its command is queued; no state injection clears titleEditor.
			openReturnMenu(t, tt, tr.ID)
			if err := tt.ChooseMenuItem("标记完成"); err != nil {
				t.Fatal(err)
			}
			f.ui()
			tt.Frame()
			if other.titleEditor != nil {
				t.Fatal("setup: actual menu did not retire title editor")
			}
			confirmCarryover(t, f)
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			var original *runtimeSubtaskComposer
			for _, c := range other.subtaskComposers {
				if c.target.trace == tr.ID {
					original = c
				}
			}
			titleDirectClient(t, tt).SetMarkedText(nil, "午夜前已交付候选", ui.TextInputRange{Start: 8, End: 8})
			if failFinish {
				original.native.handle = ui.Handle{}
			}
			f.clock.now = f.clock.now.Add(24 * time.Hour)
			f.io.one()
			f.ui()
			tt.Frame()
			if failFinish {
				snap, ok := original.native.handle.TextInputSnapshot(original.native.context)
				if !ok || !strings.Contains(snap.Text+snap.MarkedText, "午夜前已交付候选") || !original.mounted {
					t.Fatal("midnight auto-follow removed native owner after failed Finish", snap, ok, other.runtimeDate, other.runtimeSelection)
				}
			} else {
				if _, ok := tt.Find("查看原任务子任务草稿"); !ok {
					t.Fatal("successful Finish draft has no recovery portal")
				}
				liveClick(t, tt, "查看原任务子任务草稿")
				f.all()
				tt.Frame()
				f.ui()
				snap, ok := original.native.handle.TextInputSnapshot(original.native.context)
				if !ok || !strings.Contains(snap.Text+snap.MarkedText, "午夜前已交付候选") {
					t.Fatal("successful Finish could not recover", snap, ok)
				}
			}
		})
	}
}
