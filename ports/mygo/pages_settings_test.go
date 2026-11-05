package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func utilityTestView(s *app) func(*ui.Context) {
	return func(c *ui.Context) {
		th := ui.LightTheme()
		th.Font = "Noto Sans CJK SC"
		c.SetTheme(th)
		if s.page == "quick-entry" || s.page == "flylight-quick-capture" {
			s.captureView(c)
		} else {
			s.settingsView(c)
		}
	}
}
func TestUtilityWindowFrames(t *testing.T) {
	for _, tc := range []struct {
		page string
		w, h int
	}{{"settings-general", 860, 660}, {"settings-groups", 860, 660}, {"settings-data", 860, 660}, {"settings-privacy", 860, 660}, {"settings-sync", 860, 660}, {"settings-provider", 860, 660}, {"quick-entry", 520, 168}, {"flylight-quick-capture", 520, 262}} {
		s := newApp()
		s.page = tc.page
		im := ui.Render(utilityTestView(s), tc.w, tc.h, 2)
		if im.Bounds().Dx() != tc.w*2 || im.Bounds().Dy() != tc.h*2 {
			t.Fatal(tc.page, im.Bounds())
		}
	}
}
func TestUtilityWindowState(t *testing.T) {
	s := newApp()
	s.page = "settings-general"
	tt := ui.NewTester(utilityTestView(s), 860, 660)
	if err := tt.Click("选择微暖纸感"); err != nil {
		t.Fatal(err)
	}
	if !s.done[900] {
		t.Fatal("theme selection")
	}
	if err := tt.Click("选择冷灰"); err != nil {
		t.Fatal(err)
	}
	if s.done[900] {
		t.Fatal("theme reversal")
	}
	if err := tt.Click("设置：组织"); err != nil {
		t.Fatal(err)
	}
	if s.page != "settings-groups" {
		t.Fatal(s.page)
	}
	if err := tt.Click("打开分组与标签预览"); err != nil {
		t.Fatal(err)
	}
	if !s.done[910] {
		t.Fatal("group preview")
	}
	if err := tt.Click("打开分组与标签预览"); err != nil {
		t.Fatal(err)
	}
	if s.done[910] {
		t.Fatal("group preview dismissal")
	}
	s = newApp()
	s.page = "quick-entry"
	tt = ui.NewTester(utilityTestView(s), 520, 168)
	if err := tt.Click("快速记录输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("原生速记")
	if s.draft != "原生速记" {
		t.Fatal(s.draft)
	}
	if err := tt.Click("取消快速记录"); err != nil {
		t.Fatal(err)
	}
	if s.page != "day" || s.draft != "" {
		t.Fatal("cancel state")
	}
}
func TestFlylightCaptureMultiline(t *testing.T) {
	s := newApp()
	s.page = "flylight-quick-capture"
	tt := ui.NewTester(s.view, 520, 262)
	if err := tt.Click("飞光速记输入"); err != nil {
		t.Fatal(err)
	}
	tt.Type("第一行\n第二行")
	if s.draft != "第一行\n第二行" {
		t.Fatal(s.draft)
	}
}
