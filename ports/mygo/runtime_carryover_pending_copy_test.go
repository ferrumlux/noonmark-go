// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func TestCarryoverPendingCopySemanticsAndGeometry(t *testing.T) {
	for _, kind := range []carryoverPickerKind{carryoverDefer, carryoverContinue} {
		t.Run(kind.actionForTest(), func(t *testing.T) {
			today := pickerDate("2026-11-05")
			p, err := newCarryoverPicker(pickerID(kind), today)
			if err != nil {
				t.Fatal(err)
			}
			pending := false
			title := "正在保存" + p.identity.action() + "…"
			var widths [4]float32
			tt := ui.NewTester(func(c *ui.Context) {
				for i, v := range []struct {
					text   string
					size   float32
					weight int
				}{{title, 16, 600}, {"关闭不会撤销已提交的操作。", 11, 400}, {"保存中", 12, 400}, {"9999年12月31日 · 周四", 11.5, 500}} {
					widths[i], _ = c.MeasureText(0, ui.Span{Text: v.text, Size: v.size, Weight: v.weight})
				}
				p.drawPending(c, p.identity, today, pending)
			}, 600, 600)
			panelLabel := p.identity.action() + "日期对话框"
			panel, _ := tt.Find(panelLabel)
			calendar, _ := tt.Find("日期选择日历")
			summary, _ := tt.Find("日期选择摘要")
			cancel, _ := tt.Find("取消日期选择")
			confirm, _ := tt.Find("确认" + p.identity.action())
			if !tt.HasText(p.identity.action()+"到哪一天？") || !tt.HasText(p.identity.hint()) || !tt.HasText("取消") || !tt.HasText(p.identity.action()) {
				t.Fatal("unsubmitted copy changed")
			}
			if a, _ := tt.Access("取消日期选择"); a.Description != "" || a.Disabled {
				t.Fatal("unsubmitted cancel changed", a)
			}
			savePickerTestFrame(t, tt, "pending-copy-"+kind.actionForTest()+"-draft-2x.png")
			pending = true
			tt.Frame()
			if !tt.HasText(title) || !tt.HasText("关闭不会撤销已提交的操作。") || !tt.HasText("关闭") || !tt.HasText("保存中") {
				t.Error("pending must identify action, saving, and presentation-only close")
			}
			if tt.HasText(p.identity.hint()) || tt.HasText("取消") || tt.HasText(p.identity.action()+"到哪一天？") {
				t.Error("pending retains unsubmitted choice/cancel instructions")
			}
			if a, ok := tt.Access(title); !ok || a.Role != ui.RoleHeading {
				t.Error("pending heading", a)
			}
			if a, ok := tt.Access(panelLabel); !ok || !strings.Contains(a.Description, "正在保存") || !strings.Contains(a.Description, "关闭不会撤销") {
				t.Error("pending dialog description", a)
			}
			if a, ok := tt.Access("关闭日期对话框"); !ok || a.Disabled || a.Role != ui.RoleButton || a.Description != "仅关闭日期对话框，已提交的操作会继续保存。" {
				t.Error("close accessibility implies wrong effect", a)
			}
			if a, ok := tt.Access("正在保存" + p.identity.action()); !ok || !a.Disabled || a.Role != ui.RoleButton || !strings.Contains(a.Description, "等待保存结果") {
				t.Error("saving accessibility", a)
			}
			for label, want := range map[string]ui.Rect{panelLabel: panel, "日期选择日历": calendar, "日期选择摘要": summary, "关闭日期对话框": cancel, "正在保存" + p.identity.action(): confirm} {
				if got, ok := tt.Find(label); !ok || got != want {
					t.Errorf("pending geometry %q: got %+v want %+v", label, got, want)
				}
			}
			if panel.W != 340 || panel.H != 395 {
				t.Fatal("source sheet dimensions", panel)
			}
			for i, limit := range []float32{304, 304, 52, 184} {
				if widths[i] <= 0 || widths[i] > limit {
					t.Errorf("text %d cropped: width %.2f limit %.2f", i, widths[i], limit)
				}
			}
			t.Logf("measured title/hint/button/longest-date widths: %.2f/%.2f/%.2f/%.2f DIP", widths[0], widths[1], widths[2], widths[3])
			for _, d := range p.grid() {
				a, ok := tt.Access("选择日期 " + d.String())
				if !ok || !a.Disabled || a.Selected != (d == p.selected) || a.Checked || a.Invalid || !strings.Contains(a.Description, "保存期间不可更改目标日期") {
					t.Errorf("pending date semantics %v: %+v", d, a)
				}
				if d == p.selected && !strings.Contains(a.Description, "已提交的目标日期") {
					t.Error("selected target not announced", a)
				}
			}
			savePickerTestFrame(t, tt, "pending-copy-"+kind.actionForTest()+"-component-2x.png")
		})
	}
}

func TestCarryoverPendingSelectedDateRemainsReadable(t *testing.T) {
	for _, kind := range []carryoverPickerKind{carryoverDefer, carryoverContinue} {
		t.Run(kind.actionForTest(), func(t *testing.T) {
			today := pickerDate("2026-11-05")
			p, _ := newCarryoverPicker(pickerID(kind), today)
			tt := ui.NewTester(func(c *ui.Context) { p.drawPending(c, p.identity, today, true) }, 600, 600)
			tt.SetScale(2)
			b, _ := tt.Find("选择日期 " + p.selected.String())
			img := tt.Image()
			whiteText := 0
			for y := int((b.Y + 8) * 2); y < int((b.Y+22)*2); y++ {
				for x := int((b.X + 14) * 2); x < int((b.X+26)*2); x++ {
					c := img.RGBAAt(x, y)
					if c.R > 245 && c.G > 245 && c.B > 245 {
						whiteText++
					}
				}
			}
			if whiteText < 4 {
				t.Errorf("disabled selected target lost white text: pixels=%d", whiteText)
			}
			other, _ := p.selected.AddDays(1)
			b, _ = tt.Find("选择日期 " + other.String())
			darkText := 0
			for y := int((b.Y + 8) * 2); y < int((b.Y+22)*2); y++ {
				for x := int((b.X + 14) * 2); x < int((b.X+26)*2); x++ {
					c := img.RGBAAt(x, y)
					if c.R < 160 && c.G < 160 && c.B < 160 {
						darkText++
					}
				}
			}
			if darkText < 4 {
				t.Errorf("pending valid date rendered like invalid date: pixels=%d", darkText)
			}
		})
	}
}

func TestCarryoverPendingCloseAndEscapePersistOnceWithoutStealingNewSheet(t *testing.T) {
	for _, kind := range []carryoverPickerKind{carryoverDefer, carryoverContinue} {
		for _, closeWith := range []string{"button", "escape"} {
			for _, reopen := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reopen=%v", kind.actionForTest(), closeWith, reopen), func(t *testing.T) {
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
					source := f.svc.Snapshot().Traces[0]
					original := f.s.carryover
					target := original.picker.selected
					date, selection := f.s.runtimeDate, f.s.runtimeSelection
					disk := calendarDiskSignature(t, f.path)
					writes := 0
					titleStoreFault(f.svc, func(phase string) error {
						if phase == "before-reserve" {
							writes++
						}
						return nil
					})
					confirmCarryover(t, f)
					f.tt.Frame()
					if !f.tt.HasText("正在保存" + original.picker.identity.action() + "…") {
						t.Error("native pending missing saving action")
					}
					if closeWith == "button" && !reopen {
						savePickerTestFrame(t, f.tt, "pending-copy-"+kind.actionForTest()+"-native-2x.png")
					}
					for i := 0; i < 3; i++ {
						f.tt.Key(0, ui.KeyEnter)
					}
					f.ui()
					if len(f.io.q) != 1 || writes != 0 || !reflect.DeepEqual(disk, calendarDiskSignature(t, f.path)) {
						t.Fatal("pending duplicated or wrote before writer")
					}
					if closeWith == "button" {
						if err := f.tt.Click("关闭日期对话框"); err != nil {
							t.Fatal(err)
						}
					} else {
						f.tt.Key(0, ui.KeyEscape)
					}
					f.ui()
					f.tt.Frame()
					if f.s.carryover != nil || len(f.io.q) != 1 || writes != 0 || f.h.carryoverInFlight[carryoverSourceKey{source.ChainID, source.ID}] != 1 || !reflect.DeepEqual(disk, calendarDiskSignature(t, f.path)) {
						t.Fatal("closing cancelled admitted writer or changed disk")
					}
					var fresh *carryoverPresentation
					if reopen {
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
						fresh = f.s.carryover
						if fresh == nil || fresh == original || fresh.pending || !f.tt.HasText("取消") {
							t.Fatal("native menu did not create a fresh unsubmitted sheet")
						}
					}
					f.all()
					for i := 0; i < 3; i++ {
						f.tt.Frame()
						f.all()
					}
					if writes != 1 || len(f.io.q) != 0 || len(f.h.carryoverInFlight) != 0 || f.s.carryover != fresh || f.s.runtimeDate != date || f.s.runtimeSelection != selection {
						t.Fatal("late completion repeated, lost source lease, stole navigation or dismissed new sheet", writes)
					}
					g := f.svc.Snapshot()
					if len(g.Traces) != 2 {
						t.Fatal("unexpected persisted traces", g.Traces)
					}
					found := 0
					for _, tr := range g.Traces {
						if tr.Status == core.TracePending && tr.Date == target && tr.ChainID == source.ChainID && tr.ID != source.ID {
							found++
						}
					}
					if found != 1 || reflect.DeepEqual(disk, calendarDiskSignature(t, f.path)) {
						t.Fatal("close did not preserve exactly one accepted target")
					}
					f.svc.Close()
					again, err := taskapp.Open(f.path, f.h.today)
					if err != nil {
						t.Fatal(err)
					}
					defer again.Close()
					if !reflect.DeepEqual(g, again.Snapshot()) {
						t.Fatal("closed-presentation result did not survive reopen")
					}
				})
			}
		}
	}
}
