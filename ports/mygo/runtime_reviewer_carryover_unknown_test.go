package main

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestReviewerFailedCompletionKeepsLateNativeDraftUntilExplicitClear(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
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
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					calls++
					return errors.New("synthetic")
				}
				return nil
			})
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			titleDirectClient(t, tt).SetMarkedText(nil, "未确认结果的原生草稿", ui.TextInputRange{Start: 10, End: 10})
			f.all()
			tt.Frame()
			f.ui()
			if strings.Contains(other.runtimeError, "已保存") {
				t.Fatal("unacknowledged result represented as saved")
			}
			if f.h.beforeClose(other) {
				t.Fatal("failed completion released late native draft without explicit discard")
			}
			if calls != 1 {
				t.Fatal("automatic mutation replay", calls)
			}
			var owner *runtimeSubtaskComposer
			for _, c := range other.subtaskComposers {
				if c.target.trace == tr.ID {
					owner = c
				}
			}
			snap, ok := owner.native.handle.TextInputSnapshot(owner.native.context)
			if !ok || !strings.Contains(snap.Text+snap.MarkedText, "未确认结果的原生草稿") {
				t.Fatal("failed result lost native draft", snap, ok)
			}
			liveClick(t, tt, label)
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Key(0, ui.KeyBackspace)
			f.ui()
			if !f.h.beforeClose(other) {
				t.Fatal("explicit native blank cannot close failed recovery")
			}
		})
	}
}
