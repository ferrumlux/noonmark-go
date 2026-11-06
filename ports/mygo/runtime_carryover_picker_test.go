// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
	"time"
)

func pickerID(k carryoverPickerKind) carryoverPickerIdentity {
	return carryoverPickerIdentity{Kind: k, Window: "w", Session: "s", SourceContext: "day", Chain: "chain", Trace: "trace", Presentation: 1}
}
func pickerDate(s string) core.LocalDate {
	d, e := core.ParseLocalDate(s)
	if e != nil {
		panic(e)
	}
	return d
}
func pickerHarness(t *testing.T, k carryoverPickerKind, date string) (*carryoverPicker, *ui.Tester, *[]carryoverPickerIntent, *core.LocalDate) {
	t.Helper()
	today := pickerDate(date)
	p, e := newCarryoverPicker(pickerID(k), today)
	if e != nil {
		t.Fatal(e)
	}
	var intents []carryoverPickerIntent
	tt := ui.NewTester(func(c *ui.Context) {
		if i := p.draw(c, p.identity, today); i != nil {
			intents = append(intents, *i)
		}
	}, 600, 600)
	return p, tt, &intents, &today
}
func TestCarryoverPickerNodesAndIntents(t *testing.T) {
	for _, k := range []carryoverPickerKind{carryoverDefer, carryoverContinue} {
		p, tt, intents, today := pickerHarness(t, k, "2026-11-05")
		if len(p.grid()) != 42 {
			t.Fatal("grid")
		}
		panel, ok := tt.Find(p.identity.action() + "日期对话框")
		if !ok || panel.W != 340 {
			t.Fatal("panel", panel)
		}
		for _, d := range p.grid() {
			if d.Valid() {
				b, ok := tt.Find("选择日期 " + d.String())
				if !ok || b.W != 40 || b.H != 30 {
					t.Fatal("full hit", b, d)
				}
			}
		}
		if _, ok := tt.Find("日期选择回到今天"); ok != (k == carryoverContinue) {
			t.Fatal("today shortcut")
		}
		before := p.selected
		tt.Key(0, ui.KeyRight)
		want, _ := before.AddDays(1)
		if p.selected != want {
			t.Fatal("right", p.selected, want)
		}
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyUp)
		tt.Key(0, ui.KeyLeft)
		if p.selected != before {
			t.Fatal("arrows")
		}
		target := pickerDate("2026-11-19")
		b, _ := tt.Find("选择日期 " + target.String())
		tt.ClickAt(b.X+1, b.Y+1)
		if p.selected != target || len(*intents) != 0 {
			t.Fatal("selection must only draft")
		}
		tt.Advance(120 * time.Millisecond)
		savePickerTestFrame(t, tt, "carryover-"+p.identity.action()+"-2x.png")
		tt.ClickAt(10, 10)
		if !p.open || len(*intents) != 0 {
			t.Fatal("backdrop must not dismiss native sheet equivalent")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Key(0, ui.KeyEnter)
		if p.open || len(*intents) != 1 || (*intents)[0].Cancelled || (*intents)[0].Date != target {
			t.Fatal("single confirm", *intents)
		}
		if p.confirm(p.identity, *today) != nil {
			t.Fatal("repeated confirm")
		}
	}
}
func TestCarryoverPickerCancelAndMidnight(t *testing.T) {
	for _, key := range []bool{true, false} {
		p, tt, intents, _ := pickerHarness(t, carryoverDefer, "2026-11-05")
		if key {
			tt.Key(0, ui.KeyEscape)
		} else {
			if e := tt.Click("取消日期选择"); e != nil {
				t.Fatal(e)
			}
		}
		if p.open || len(*intents) != 1 || !(*intents)[0].Cancelled {
			t.Fatal("cancel", *intents)
		}
	}
	p, tt, intents, today := pickerHarness(t, carryoverDefer, "2026-11-05")
	*today = pickerDate("2026-11-06")
	tt.Key(0, ui.KeyEnter)
	if !p.open || len(*intents) != 0 || p.validation != p.identity.hint() {
		t.Fatal("midnight")
	}
	savePickerTestFrame(t, tt, "carryover-midnight-2x.png")
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	if len(*intents) != 1 || (*intents)[0].Date != pickerDate("2026-11-07") {
		t.Fatal("midnight recovery", *intents)
	}
}
func TestCarryoverPickerDatesAndIdentity(t *testing.T) {
	for _, v := range []struct{ today, want string }{{"2028-02-28", "2028-02-29"}, {"2028-02-29", "2028-03-01"}, {"2026-12-31", "2027-01-01"}} {
		p, e := newCarryoverPicker(pickerID(carryoverDefer), pickerDate(v.today))
		if e != nil || p.selected != pickerDate(v.want) {
			t.Fatal(v, e)
		}
		if _, ok := p.shiftMonth(-1, pickerDate(v.today)); ok {
			t.Fatal("minimum month")
		}
	}
	p, _ := newCarryoverPicker(pickerID(carryoverContinue), pickerDate("2026-11-05"))
	old := p.identity
	for _, change := range []func(*carryoverPickerIdentity){func(i *carryoverPickerIdentity) { i.Presentation++ }, func(i *carryoverPickerIdentity) { i.Kind = carryoverDefer }, func(i *carryoverPickerIdentity) { i.Window = "other" }, func(i *carryoverPickerIdentity) { i.Trace = "other" }, func(i *carryoverPickerIdentity) { i.Session = "other" }, func(i *carryoverPickerIdentity) { i.SourceContext = "other" }} {
		wrong := old
		change(&wrong)
		p.selectDate(wrong, pickerDate("2026-11-20"), pickerDate("2026-11-05"))
		if p.selected.Day != 5 || p.confirm(wrong, p.selected) != nil || p.cancel(wrong) != nil {
			t.Fatal("stale identity")
		}
	}
	if _, e := newCarryoverPicker(pickerID(carryoverDefer), pickerDate("9999-12-31")); e == nil {
		t.Fatal("representation overflow")
	}
	p, _ = newCarryoverPicker(pickerID(carryoverContinue), pickerDate("9999-12-31"))
	if _, ok := p.shiftMonth(1, p.selected); ok {
		t.Fatal("overflow month")
	}
	if len(p.grid()) != 42 {
		t.Fatal("boundary grid")
	}
}
func TestCarryoverPickerReturnFocusAndOuterShortcuts(t *testing.T) {
	for _, focus := range []string{"calendar", "2026-11-06", "next", "cancel", "confirm"} {
		t.Run(focus, func(t *testing.T) {
			today := pickerDate("2026-11-05")
			id := pickerID(carryoverDefer)
			p, _ := newCarryoverPicker(id, today)
			var intents []carryoverPickerIntent
			outer := 0
			tt := ui.NewTester(func(c *ui.Context) {
				if c.Shortcut(0, ui.KeyEnter) {
					outer++
				}
				if c.Shortcut(0, ui.KeyRight) {
					outer++
				}
				if i := p.draw(c, id, today); i != nil {
					intents = append(intents, *i)
				}
			}, 600, 600)
			p.focus = focus
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			if outer != 0 || len(intents) != 1 || intents[0].Cancelled || intents[0].Date != pickerDate("2026-11-06") {
				t.Fatalf("default Return focus %s: outer=%d intents=%+v", focus, outer, intents)
			}
		})
	}
}
func TestCarryoverPickerReplacedPurposeNodes(t *testing.T) {
	today := pickerDate("2026-11-05")
	id := pickerID(carryoverDefer)
	p, _ := newCarryoverPicker(id, today)
	old := p
	var intents []carryoverPickerIntent
	tt := ui.NewTester(func(c *ui.Context) {
		if i := p.draw(c, id, today); i != nil {
			intents = append(intents, *i)
		}
	}, 600, 600)
	id.Kind = carryoverContinue
	id.Presentation++
	p, _ = newCarryoverPicker(id, today)
	tt.Frame()
	if old.confirm(id, today) != nil || old.cancel(id) != nil {
		t.Fatal("old callback reached replacement")
	}
	if !tt.HasText("延续到哪一天？") || tt.HasText("延期到哪一天？") {
		t.Fatal("old nodes survived")
	}
	tt.Key(0, ui.KeyEnter)
	if len(intents) != 1 || intents[0].Identity != id || intents[0].Date != today {
		t.Fatal("replacement", intents)
	}
}
func TestCarryoverPickerMonthAndTodayNodes(t *testing.T) {
	p, tt, intents, today := pickerHarness(t, carryoverDefer, "2026-11-05")
	selected := p.selected
	// Disabled past date and invalid left movement cannot change the draft.
	b, _ := tt.Find("选择日期 2026-11-05")
	tt.ClickAt(b.X+20, b.Y+15)
	tt.Key(0, ui.KeyLeft)
	if p.selected != selected {
		t.Fatal("invalid date selected")
	}
	if e := tt.Click("日期选择下个月"); e != nil {
		t.Fatal(e)
	}
	if p.month != pickerDate("2026-12-01") || p.selected != selected || len(*intents) != 0 {
		t.Fatal("month mutated draft")
	}
	// Adjacent previous-month dates retain their whole-cell targets and selecting
	// them displays their actual month, as DatePickerSheet.select does.
	if e := tt.Click("选择日期 2026-11-30"); e != nil {
		t.Fatal(e)
	}
	if p.month.Month != 11 || p.selected.Day != 30 {
		t.Fatal("adjacent month selection")
	}
	if !tt.Focused("日期选择日历") {
		t.Fatal("calendar focus not restored")
	}
	q, qt, qi, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	if e := qt.Click("选择日期 2026-11-19"); e != nil {
		t.Fatal(e)
	}
	if e := qt.Click("日期选择回到今天"); e != nil {
		t.Fatal(e)
	}
	if q.selected != *today || len(*qi) != 0 {
		t.Fatal("today action")
	}
}
func TestCarryoverPickerArrowFocusScope(t *testing.T) {
	for _, focus := range []string{"cancel", "confirm", "next"} {
		p, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
		p.focus = focus
		tt.Frame()
		before := p.selected
		tt.Key(0, ui.KeyRight)
		if p.selected != before {
			t.Fatal("arrow escaped calendar focus", focus)
		}
	}
}
func TestCarryoverPickerSelectedTodayPixels(t *testing.T) {
	p, tt, _, today := pickerHarness(t, carryoverContinue, "2026-11-05")
	p.selectDate(p.identity, pickerDate("2026-11-06"), *today)
	tt.Frame()
	tt.Advance(120 * time.Millisecond)
	b, _ := tt.Find("选择日期 2026-11-06")
	before := append([]byte(nil), tt.Image().Pix...)
	stride := tt.Image().Stride
	*today = pickerDate("2026-11-06")
	tt.Frame()
	after := tt.Image()
	for y := int(b.Y); y < int(b.Y+b.H); y++ {
		for x := int(b.X); x < int(b.X+b.W); x++ {
			i := y*stride + x*4
			for k := 0; k < 4; k++ {
				if before[i+k] != after.Pix[i+k] {
					t.Fatalf("selected today changes circle pixels at %d,%d", x, y)
				}
			}
		}
	}
	pixel := after.RGBAAt(int(b.X)+3, int(b.Y)+15)
	if pixel.R < 245 || pixel.G < 245 || pixel.B < 245 {
		t.Fatalf("selected cell must not have full-cell blue fill: %+v", pixel)
	}
	savePickerTestFrame(t, tt, "carryover-selected-today-2x.png")
}
func TestCarryoverPickerShiftAndSelectedAccessibility(t *testing.T) {
	p, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	a, ok := tt.Access("选择日期 2026-11-05")
	// GO-MYGO-SELECTED-001: the original frozen diagnostic asserted the inherited
	// Checked=true/Selected=false defect. The pinned repair must expose the real
	// selected state while preserving the Button role and independent Checked.
	if !ok || a.Role != ui.RoleButton || !a.Selected || a.Checked {
		t.Errorf("selected button accessibility contract: %+v", a)
	}
	if _, ok := tt.AccessByRole(ui.RoleHeading, "延续到哪一天？"); !ok {
		t.Error("missing heading role")
	}
	if _, ok := tt.Access("一"); ok {
		t.Error("weekday decoration exposed")
	}
	tt.Key(ui.Shift, ui.KeyRight)
	if p.selected != pickerDate("2026-11-06") {
		t.Error("Shift arrow differs from native")
	}
}
func TestCarryoverPickerSelectionMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		p, tt, _, today := pickerHarness(t, carryoverContinue, "2026-11-05")
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		initial, _ := tt.Find("选择日期 2026-11-05")
		first := tt.Image().RGBAAt(int(initial.X)+20, int(initial.Y)+25)
		if first.R != sourceVisual.accent.R || first.G != sourceVisual.accent.G || first.B != sourceVisual.accent.B {
			t.Fatal("first appearance incorrectly animated", first)
		}
		b, _ := tt.Find("选择日期 2026-11-06")
		sample := func() [3]uint8 { v := tt.Image().RGBAAt(int(b.X)+20, int(b.Y)+25); return [3]uint8{v.R, v.G, v.B} }
		before := sample()
		p.selectDate(p.identity, pickerDate("2026-11-06"), *today)
		tt.Frame()
		start := sample()
		tt.Advance(60 * time.Millisecond)
		middle := sample()
		tt.Advance(60 * time.Millisecond)
		end := sample()
		expected := [3]uint8{sourceVisual.accent.R, sourceVisual.accent.G, sourceVisual.accent.B}
		if end != expected {
			t.Fatal("not at target after 120ms", end)
		}
		if reduced {
			if start != expected || middle != expected {
				t.Fatal("reduced motion did not snap", start, middle)
			}
		} else if start != before || middle == before || middle == end {
			t.Fatal("missing genuine intermediate frame", before, start, middle, end)
		}
	}
}
func TestCarryoverPickerReduceMotionDuringTransition(t *testing.T) {
	p, tt, _, today := pickerHarness(t, carryoverContinue, "2026-11-05")
	p.selectDate(p.identity, pickerDate("2026-11-06"), *today)
	tt.Frame()
	tt.Advance(30 * time.Millisecond)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	b, _ := tt.Find("选择日期 2026-11-06")
	pixel := tt.Image().RGBAAt(int(b.X)+20, int(b.Y)+25)
	if pixel.R != sourceVisual.accent.R || pixel.G != sourceVisual.accent.G || pixel.B != sourceVisual.accent.B {
		t.Fatal("reducing motion mid-transition did not snap", pixel)
	}
	tt.SetPreferences(ui.Preferences{})
	pixel = tt.Image().RGBAAt(int(b.X)+20, int(b.Y)+25)
	if pixel.R != sourceVisual.accent.R || pixel.G != sourceVisual.accent.G || pixel.B != sourceVisual.accent.B {
		t.Fatal("old transition resumed", pixel)
	}
}
