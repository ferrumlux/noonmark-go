// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
	"time"
)

func TestCarryoverPendingNativeDateMonthTodayAndConfirmDisabled(t *testing.T) {
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
			openCarryoverMenu(t, f, kind)
			f.tt.Key(0, ui.KeyRight)
			f.ui()
			p := f.s.carryover
			target, month := p.picker.selected, p.picker.month
			confirmCarryover(t, f)
			f.tt.Frame()
			labels := []string{"日期选择下个月", "正在保存" + p.picker.identity.action(), "选择日期 " + target.String()}
			if kind == carryoverContinue {
				labels = append(labels, "日期选择回到今天")
			}
			for _, label := range labels {
				a, ok := f.tt.Access(label)
				if !ok || !a.Disabled {
					t.Fatal("pending control not disabled", label, a)
				}
				b, ok := f.tt.Find(label)
				if !ok {
					t.Fatal(label)
				}
				f.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
			}
			opened := 0
			f.s.openUtility = func(string) { opened++ }
			for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyEnter} {
				f.tt.Key(0, key)
				f.tt.Key(ui.Shift, key)
			}
			f.tt.Key(ui.Cmd, ui.KeyN)
			f.ui()
			if p.picker.selected != target || p.picker.month != month || opened != 0 || len(f.io.q) != 1 {
				t.Fatal("pending target changed or shortcut leaked")
			}
			// Close is enabled and only dismisses presentation; the accepted target persists.
			a, ok := f.tt.Access("关闭日期对话框")
			if !ok || a.Disabled {
				t.Fatal("cancel disabled")
			}
			liveClick(t, f.tt, "关闭日期对话框")
			f.ui()
			if f.s.carryover != nil {
				t.Fatal("pending cancel ignored")
			}
			f.all()
			if f.s.runtimeDate == target {
				t.Fatal("cancelled presentation stole navigation")
			}
			found := false
			for _, tr := range f.svc.Snapshot().Traces {
				if tr.Status == core.TracePending && tr.Date == target {
					found = true
				}
			}
			if !found {
				t.Fatal("cancel rolled back accepted target")
			}
		})
	}
}
func (k carryoverPickerKind) actionForTest() string {
	if k == carryoverDefer {
		return "defer"
	}
	return "continue"
}
