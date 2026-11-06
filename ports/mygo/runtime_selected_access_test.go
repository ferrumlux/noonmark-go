package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

// The actual component must announce Swift isSelected without becoming a list item.
func TestCarryoverPickerSelectedAccessibilityContract(t *testing.T) {
	_, tt, _, _ := pickerHarness(t, carryoverContinue, "2026-11-05")
	for _, date := range []string{"2026-11-05", "2026-11-06"} {
		a, ok := tt.Access("选择日期 " + date)
		if !ok || a.Role != ui.RoleButton || a.Selected != (date == "2026-11-05") || a.Checked {
			t.Fatalf("date %s: %+v", date, a)
		}
	}
	tt.Key(0, ui.KeyRight)
	for _, date := range []string{"2026-11-05", "2026-11-06"} {
		a, ok := tt.Access("选择日期 " + date)
		if !ok || a.Role != ui.RoleButton || a.Selected != (date == "2026-11-06") || a.Checked {
			t.Fatalf("after key date %s: %+v", date, a)
		}
	}
}

func TestSelectedButtonIndependentAccessibility(t *testing.T) {
	selected, checked := true, false
	tt := ui.NewTester(func(c *ui.Context) {
		ui.ButtonBase(c).Label("Selected button").Size(80, 30).Selected(selected).Checked(checked).Background(ui.Transparent)
	}, 120, 80)
	for _, state := range [][2]bool{{true, false}, {false, true}, {true, true}, {false, false}} {
		selected, checked = state[0], state[1]
		tt.Frame()
		a, ok := tt.Access("Selected button")
		if !ok || a.Role != ui.RoleButton || a.Selected != selected || a.Checked != checked {
			t.Fatalf("selected=%v checked=%v: %+v", selected, checked, a)
		}
	}
}
