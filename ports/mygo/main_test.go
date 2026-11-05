package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestNativeScreensAndNavigation(t *testing.T) {
	s := newApp()
	tt := ui.NewTester(s.view, 1200, 768)
	if !tt.HasText("2026年7月24日") {
		t.Fatal("day heading missing")
	}
	for _, tc := range []struct{ label, page, want string }{{"任务池", "pool", "尚未安排日期的任务。排期后会出现在对应的 Day Todo。"}, {"日历", "calendar", "2026年7月"}, {"Day Todo", "day", "修复登录阻断问题"}} {
		if err := tt.Click("切换到" + tc.label); err != nil {
			t.Fatal(err)
		}
		if s.page != tc.page || !tt.HasText(tc.want) {
			t.Fatalf("navigation %s: page=%s", tc.label, s.page)
		}
	}
	if err := tt.Click("快速输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("原生输入测试 #原型")
	if s.draft != "原生输入测试 #原型" {
		t.Fatalf("input: %q", s.draft)
	}
	if err := tt.Click("完成 修复登录阻断问题"); err != nil {
		t.Fatal(err)
	}
	if !s.done[0] {
		t.Fatal("check did not toggle")
	}
	if err := tt.Click("完成 修复登录阻断问题"); err != nil {
		t.Fatal(err)
	}
	if s.done[0] {
		t.Fatal("repeated check did not revert")
	}
	if err := tt.Click("7月25日"); err != nil {
		t.Fatal(err)
	}
	if s.selectedDay != 25 {
		t.Fatal("date selection")
	}
	if err := tt.Click("收起侧栏"); err != nil {
		t.Fatal(err)
	}
	if !s.compact {
		t.Fatal("sidebar did not compact")
	}
	if err := tt.Click("收起侧栏"); err != nil {
		t.Fatal(err)
	}
	if s.compact {
		t.Fatal("sidebar did not expand")
	}
}
func TestNativeSnapshotSizes(t *testing.T) {
	for _, page := range []string{"day", "pool", "calendar"} {
		s := newApp()
		s.page = page
		img := ui.Render(s.view, 1200, 768, 2)
		if img.Bounds().Dx() != 2400 || img.Bounds().Dy() != 1536 {
			t.Fatalf("%s size %v", page, img.Bounds())
		}
	}
}
func TestNarrowWindow(t *testing.T) {
	s := newApp()
	tt := ui.NewTester(s.view, 960, 720)
	if !tt.HasText("Day Todo") {
		t.Fatal("narrow sidebar missing")
	}
	if err := tt.Click("切换到日历"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2026年7月") {
		t.Fatal("narrow calendar missing")
	}
}

func TestAllPrimaryNavigation(t *testing.T) {
	s := newApp()
	tt := ui.NewTester(s.view, 1200, 768)
	for round := 0; round < 2; round++ {
		for _, n := range nav {
			if err := tt.Click("切换到" + n.title); err != nil {
				t.Fatal(err)
			}
			if s.page != n.page {
				t.Fatalf("page=%s want=%s", s.page, n.page)
			}
			if tt.HasText("此页面尚未实现。本轮原型已完成 Day Todo、任务池、日历。") {
				t.Fatalf("stub shown for %s", n.page)
			}
		}
	}
}
func TestEveryRegisteredScreen(t *testing.T) {
	for _, spec := range screens {
		t.Run(spec.key, func(t *testing.T) {
			s := newApp()
			s.page = spec.key
			img := ui.Render(s.view, spec.width, spec.height, 2)
			if img.Bounds().Dx() != spec.width*2 || img.Bounds().Dy() != spec.height*2 {
				t.Fatal(img.Bounds())
			}
		})
	}
}
func TestUtilityWindowShortcuts(t *testing.T) {
	s := newApp()
	var opened string
	s.openUtility = func(page string) { opened = page }
	tt := ui.NewTester(s.view, 1200, 768)
	tt.Key(ui.Cmd, ui.KeyComma)
	if opened != "settings-general" {
		t.Fatal(opened)
	}
	tt.Key(ui.Cmd, ui.KeyN)
	if opened != "quick-entry" {
		t.Fatal(opened)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyI)
	if opened != "flylight-quick-capture" {
		t.Fatal(opened)
	}
	s.page = "quick-entry"
	closed := false
	s.closeWindow = func() { closed = true }
	tt.SetSize(520, 168)
	tt.Key(0, ui.KeyEscape)
	if !closed {
		t.Fatal("escape did not close utility")
	}
}
