package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestReviewerPendingCloseDoesNotLeakDisabledSavingStateIntoFreshSheet(t *testing.T) {
	for _, kind := range []carryoverPickerKind{carryoverDefer, carryoverContinue} {
		t.Run(kind.actionForTest(), func(t *testing.T) {
			f := newTitleFixture(t, true)
			if kind == carryoverContinue {
				tomorrow, _ := liveTestDay.AddDays(1)
				f.clock.now = f.clock.now.Add(24 * time.Hour)
				if err := f.svc.AdvanceTo(tomorrow); err != nil {
					t.Fatal(err)
				}
				f.h.publish(f.svc.Snapshot(), tomorrow)
				f.ui()
				f.tt.Frame()
			}
			source := f.svc.Snapshot().Traces[0]
			openCarryoverMenu(t, f, kind)
			confirmCarryover(t, f)
			f.tt.Frame()
			liveClick(t, f.tt, "关闭日期对话框")
			f.ui()
			openReturnMenu(t, f.tt, source.ID)
			label := deferTaskLabel
			if kind == carryoverContinue {
				label = continueTaskLabel
			}
			if err := f.tt.ChooseMenuItem(label); err != nil {
				t.Fatal(err)
			}
			f.ui()
			f.tt.Frame()
			p := f.s.carryover
			if p == nil || p.pending {
				t.Fatal("missing fresh draft")
			}
			if a, ok := f.tt.Access(p.picker.identity.action() + "日期对话框"); !ok || strings.Contains(a.Description, "正在保存") {
				t.Fatal("pending description leaked", a)
			}
			if a, ok := f.tt.Access("确认" + p.picker.identity.action()); !ok || a.Disabled {
				t.Fatal("pending disabled state leaked", a)
			}
			if a, ok := f.tt.Access("取消日期选择"); !ok || a.Description != "" || a.Disabled {
				t.Fatal("pending close state leaked", a)
			}
			before := p.picker.selected
			f.tt.Key(0, ui.KeyRight)
			f.ui()
			f.tt.Frame()
			if p.picker.selected == before {
				t.Fatal("fresh draft inherited pending keyboard freeze")
			}
			if len(f.io.q) != 1 {
				t.Fatal("fresh draft edited writer queue")
			}
			f.tt.Key(0, ui.KeyEscape)
			f.all()
			if len(f.svc.Snapshot().Traces) != 2 {
				t.Fatal("old submitted operation not persisted exactly once")
			}
		})
	}
}
