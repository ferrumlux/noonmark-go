package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func memoClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
}
func TestStickyWallStreamRepeatedToggle(t *testing.T) {
	s := newApp()
	s.page = "sticky"
	tt := ui.NewTester(s.view, 1200, 768)
	for i := 0; i < 3; i++ {
		for _, want := range []string{"sticky-stream", "sticky"} {
			memoClick(t, tt, "切换便签墙或清单流")
			if s.page != want {
				t.Fatalf("toggle: got %s, want %s", s.page, want)
			}
			for _, copy := range []string{memoFirst, memoHandoff, memoExpense} {
				if !tt.HasText(copy) {
					t.Fatalf("%s lost %q", want, copy)
				}
			}
		}
	}
	memoClick(t, tt, "切换到飞光")
	if s.page != "flylight" {
		t.Fatal("navigation away failed")
	}
	memoClick(t, tt, "切换到Sticky Note")
	if s.page != "sticky" {
		t.Fatal("navigation back failed")
	}
}
func TestFlylightEditCancelAndChineseDraft(t *testing.T) {
	s := newApp()
	s.page = "flylight"
	tt := ui.NewTester(s.view, 1200, 768)
	for i := 0; i < 3; i++ {
		memoClick(t, tt, "在列表中编辑飞光")
		if s.page != "flylight-edit" || !tt.HasText("保存修改 ↑") {
			t.Fatal("inline edit missing")
		}
		memoClick(t, tt, "飞光操作：取消")
		if s.page != "flylight" {
			t.Fatal("cancel did not return to list")
		}
		memoClick(t, tt, "飞光操作：编辑飞光")
		if s.page != "flylight-edit" {
			t.Fatal("rail edit did not open")
		}
		memoClick(t, tt, "飞光操作：收起")
		if s.page != "flylight" {
			t.Fatal("collapse did not return to list")
		}
	}
	memoClick(t, tt, "飞光草稿，仅本次演示")
	tt.Type("中文草稿 #灵感 @协作")
	if s.draft != "中文草稿 #灵感 @协作" {
		t.Fatalf("Chinese draft: %q", s.draft)
	}
	memoClick(t, tt, "飞光操作：展开")
	if s.page != "flylight-edit" || s.draft != "中文草稿 #灵感 @协作" {
		t.Fatal("expansion lost draft")
	}
	memoClick(t, tt, "飞光操作：取消")
	if s.draft != "中文草稿 #灵感 @协作" {
		t.Fatal("cancel affected separate composer draft")
	}
	memoClick(t, tt, "切换到烛龙")
	if s.page != "ai" || s.draft != "" {
		t.Fatal("AI navigation did not reset demo draft")
	}
	memoClick(t, tt, "烛龙静态演示输入，不会发送")
	tt.Type("复盘一下延期原因")
	if s.draft != "复盘一下延期原因" {
		t.Fatalf("AI Chinese draft: %q", s.draft)
	}
	if !tt.HasText("复盘一下最近任务经常延期的模式。") {
		t.Fatal("static conversation changed")
	}
}
