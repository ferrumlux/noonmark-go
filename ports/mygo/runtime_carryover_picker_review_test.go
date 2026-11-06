package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

// Swift DateNavigationKeyboardFocusView excludes command/control/option, not Shift.
func TestReviewCarryoverShiftArrow(t *testing.T) {
	p, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	tt.Key(ui.Shift, ui.KeyRight)
	if p.selected != pickerDate("2026-11-06") {
		t.Fatalf("Shift+Right must match native source: got %v", p.selected)
	}
}
func TestReviewCarryoverAccessAndBounds(t *testing.T) {
	_, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	a, ok := tt.Access("选择日期 2026-11-05")
	if !ok || a.Role != ui.RoleButton || !strings.Contains(a.Value, "今天") || !strings.Contains(a.Value, "已选择") || a.Disabled {
		t.Fatalf("selected access %+v", a)
	}
	t.Logf("selected trait=%v, value=%q", a.Selected, a.Value)
	a, ok = tt.Access("选择日期 2026-11-04")
	if !ok || !a.Disabled || !strings.Contains(a.Value, "不可选择") {
		t.Fatalf("disabled access %+v", a)
	}
	for _, date := range []string{"0001-01-01", "9999-12-31"} {
		p, qt, qi, _ := pickerHarness(t, carryoverContinue, date)
		for _, d := range p.grid() {
			if d.Valid() {
				if _, ok := qt.Find("选择日期 " + d.String()); !ok {
					t.Fatal("missing boundary node", d)
				}
			}
		}
		qt.Key(0, ui.KeyEnter)
		if len(*qi) != 1 || (*qi)[0].Date != pickerDate(date) {
			t.Fatal("boundary confirm", date, *qi)
		}
	}
}
func TestReviewCarryoverEveryHitCornerAndDisabledMonth(t *testing.T) {
	for _, xy := range [][2]float32{{.2, .2}, {39.8, .2}, {.2, 29.8}, {39.8, 29.8}} {
		p, tt, _, _ := pickerHarness(t, carryoverContinue, "2028-02-29")
		b, ok := tt.Find("选择日期 2028-03-01")
		if !ok {
			t.Fatal("adjacent missing")
		}
		tt.ClickAt(b.X+xy[0], b.Y+xy[1])
		if p.selected != pickerDate("2028-03-01") {
			t.Fatal("corner miss", xy, p.selected)
		}
	}
	p, tt, _, _ := pickerHarness(t, carryoverDefer, "2026-11-30")
	a, ok := tt.Access("日期选择上个月")
	if !ok || !a.Disabled {
		t.Fatalf("minimum-month enabled %+v", a)
	}
	b, _ := tt.Find("日期选择上个月")
	tt.ClickAt(b.X+14, b.Y+14)
	if p.month != pickerDate("2026-12-01") {
		t.Fatal("disabled month changed")
	}
}
func TestReviewCarryoverNativeAccessibilitySemantics(t *testing.T) {
	_, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	a, ok := tt.Access("选择日期 2026-11-05")
	if !ok || !a.Selected {
		t.Errorf("missing selected accessibility trait: %+v", a)
	}
	if a, ok := tt.Access("延续到哪一天？"); !ok || a.Role != ui.RoleHeading {
		t.Errorf("missing heading semantics: %+v", a)
	}
	if a, ok := tt.Access("一"); ok {
		t.Errorf("weekday should be accessibility-hidden: %+v", a)
	}
}
func TestReviewCarryoverModalOuterAndRepeatedArrows(t *testing.T) {
	today := pickerDate("2026-11-05")
	id := pickerID(carryoverContinue)
	p, _ := newCarryoverPicker(id, today)
	outer := 0
	tt := ui.NewTester(func(c *ui.Context) {
		for _, k := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyEscape, ui.KeyEnter} {
			if c.Shortcut(0, k) {
				outer++
			}
		}
		p.draw(c, id, today)
	}, 600, 600)
	for i := 0; i < 10; i++ {
		tt.Key(0, ui.KeyRight)
	}
	if p.selected != pickerDate("2026-11-15") || outer != 0 {
		t.Fatalf("repeat/outer mismatch %v %d", p.selected, outer)
	}
	for _, mod := range []ui.Modifiers{ui.Ctrl, ui.Alt, ui.Super} {
		tt.Key(mod, ui.KeyRight)
	}
	if p.selected != pickerDate("2026-11-15") {
		t.Fatal("modified arrow changed draft")
	}
	tt.KeyRepeat(0, ui.KeyRight)
	if p.selected != pickerDate("2026-11-16") {
		t.Fatal("held arrow repeat lost")
	}
	tt.Key(0, ui.KeyTab)
	if tt.Focused("日期选择日历") {
		t.Fatal("Tab did not leave calendar")
	}
	tt.Key(0, ui.KeyRight)
	if p.selected != pickerDate("2026-11-16") {
		t.Fatal("arrow escaped calendar after real Tab")
	}
	tt.Key(0, ui.KeyEscape)
	if p.open || outer != 0 {
		t.Fatal("escape leaked")
	}
}

func TestReviewCarryoverHoverAndSelectedPixels(t *testing.T) {
	_, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	b, _ := tt.Find("选择日期 2026-11-05")
	pixel := tt.Image().RGBAAt(int(b.X+3), int(b.Y+15))
	if pixel.R < 245 || pixel.G < 245 || pixel.B < 245 {
		t.Fatalf("selected full-cell fill %+v", pixel)
	}
	other, _ := tt.Find("选择日期 2026-11-20")
	tt.Move(other.X+2, other.Y+15)
	pixel = tt.Image().RGBAAt(int(other.X+2), int(other.Y+15))
	// CoolGray listRowHover (.977,.979,.984), 8-bit source rounding.
	if pixel.R < 248 || pixel.R > 250 || pixel.G < 249 || pixel.G > 251 || pixel.B < 250 || pixel.B > 252 {
		t.Fatalf("hover colour %+v", pixel)
	}
	tt.Move(0, 0)
	pixel = tt.Image().RGBAAt(int(other.X+2), int(other.Y+15))
	if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
		t.Fatalf("hover did not clear %+v", pixel)
	}
}
