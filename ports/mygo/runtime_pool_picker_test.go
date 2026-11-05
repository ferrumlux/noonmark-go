// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func TestLivePoolPickerEmptyAndCancellationAreReadOnly(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	liveType(t, tt, "不应丢失的草稿")
	before := svc.Snapshot()
	disk := calendarDiskSignature(t, path)
	liveClick(t, tt, "从任务池排期")
	if !s.runtimePoolPicker || !tt.HasText("任务池是空的。") || !tt.Focused("取消从任务池排期") {
		t.Fatal("empty sheet focus/content")
	}
	date := s.runtimeDate
	tt.Key(0, ui.KeyRight)
	if s.runtimeDate != date {
		t.Fatal("modal leaked date shortcut")
	}
	tt.Key(0, ui.KeyEscape)
	if s.runtimePoolPicker {
		t.Fatal("escape did not cancel")
	}
	liveClick(t, tt, "从任务池排期")
	liveClick(t, tt, "取消从任务池排期")
	liveClick(t, tt, "从任务池排期")
	tt.ClickAt(10, 10)
	if s.runtimePoolPicker || s.draft != "不应丢失的草稿" || !reflect.DeepEqual(before, svc.Snapshot()) || !reflect.DeepEqual(disk, calendarDiskSignature(t, path)) {
		t.Fatal("dismissal wrote or lost draft")
	}
	// The pool page still has an independent, working add button.
	liveClick(t, tt, "切换到任务池")
	liveClick(t, tt, "添加任务")
	if len(svc.Snapshot().Chains) != 1 {
		t.Fatal("pool add button stopped working")
	}
}
func TestLivePoolPickerSchedulesFutureOnceAndRestarts(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	if e := svc.AddPool("等待排期的真实合成任务"); e != nil {
		t.Fatal(e)
	}
	chain := svc.Snapshot().Chains[0].ID
	liveClick(t, tt, "后一天")
	liveType(t, tt, "日快速输入草稿")
	liveClick(t, tt, "从任务池排期")
	label := "排期任务 " + string(chain)
	if !tt.Focused(label) {
		t.Fatal("source first-row focus missing")
	}
	tt.Key(0, ui.KeyEnter)
	g := svc.Snapshot()
	if s.runtimePoolPicker || len(g.Traces) != 1 || g.Traces[0].Date.Day != 6 || s.runtimeSelection != string(g.Traces[0].ID) || !s.runtimeDayRail || s.draft != "日快速输入草稿" {
		t.Fatal("successful scheduling did not preserve/select", g)
	}
	// Reopening the picker cannot offer the now-pending chain a second time.
	liveClick(t, tt, "从任务池排期")
	if _, ok := tt.Find(label); ok {
		t.Fatal("pending chain remained in pool")
	}
	tt.Key(0, ui.KeyEnter) // Cancel has initial focus, not the task editor behind it.
	if len(svc.Snapshot().Chains) != 1 || len(svc.Snapshot().Traces) != 1 || s.runtimePoolPicker {
		t.Fatal("repeated submit duplicated or leaked")
	}
	if e := svc.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := taskapp.Open(path, liveTestDay)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(g, reopened.Snapshot()) {
		t.Fatal("scheduled facts lost on reopen")
	}
}
func TestLivePoolPickerFailurePreservesFactsAndDraft(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	if e := svc.AddPool("不能半保存"); e != nil {
		t.Fatal(e)
	}
	chain := svc.Snapshot().Chains[0].ID
	liveType(t, tt, "需要保留")
	before := svc.Snapshot()
	if e := os.Mkdir(filepath.Join(path, "unexpected"), 0700); e != nil {
		t.Fatal(e)
	}
	liveClick(t, tt, "从任务池排期")
	liveClick(t, tt, "排期任务 "+string(chain))
	warning, ok := tt.Find("任务池排期错误")
	panel, _ := tt.Find("从任务池排期对话框")
	if !s.runtimePoolPicker || s.runtimeError == "" || !ok || warning.H <= 0 || warning.X < panel.X || warning.Y+warning.H > panel.Y+panel.H || s.draft != "需要保留" || !reflect.DeepEqual(before, svc.Snapshot()) {
		t.Fatal("failure silently closed/lost data", warning)
	}
	savePickerTestFrame(t, tt, "live-pool-picker-failure-2x.png")
	liveClick(t, tt, "取消从任务池排期")
	if s.runtimePoolPicker || s.runtimeError == "" {
		t.Fatal("cancel lost failure message")
	}
}
func TestLivePoolPickerModalBlocksBackgroundAndLongListScrolls(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	for i := 0; i < 22; i++ {
		if e := svc.AddPool(fmt.Sprintf("合成任务%02d：%s", i, strings.Repeat("长标题换行。", 5))); e != nil {
			t.Fatal(e)
		}
	}
	tt.SetSize(960, 720)
	next, _ := tt.Find("后一天")
	liveClick(t, tt, "从任务池排期")
	date := s.runtimeDate
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	if s.runtimeDate != date {
		t.Fatal("modal keyboard changed background date")
	}
	list, ok := tt.Find("任务池排期列表")
	if !ok || list.H != 320 {
		t.Fatal("source scroll cap", list)
	}
	g := svc.Snapshot()
	lastLabel := "排期任务 " + string(g.Chains[len(g.Chains)-1].ID)
	last, _ := tt.Find(lastLabel)
	if last.H != 0 {
		t.Fatal("last row not outside viewport", last)
	}
	tt.Scroll(list.X+list.W/2, list.Y+list.H/2, 0, 5000)
	last, ok = tt.Find(lastLabel)
	if !ok || last.H <= 0 || last.Y+last.H > list.Y+list.H {
		t.Fatal("last row inaccessible", last, list)
	}
	cancel, _ := tt.Find("取消从任务池排期")
	if cancel.H != 28 || cancel.Y+cancel.H > 720 {
		t.Fatal("cancel clipped", cancel)
	}
	savePickerTestFrame(t, tt, "live-pool-picker-long-min-2x.png")
	tt.ClickAt(next.X+next.W/2, next.Y+next.H/2)
	if s.runtimePoolPicker || s.runtimeDate != date || len(svc.Snapshot().Traces) != 0 {
		t.Fatal("outside click activated background")
	}
}
func TestLivePoolPickerNaturalDayFailureAndHistoryGuard(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	if e := svc.AddPool("跨日保护"); e != nil {
		t.Fatal(e)
	}
	chain := svc.Snapshot().Chains[0].ID
	liveClick(t, tt, "从任务池排期")
	s.refreshDay = func() error { return fmt.Errorf("synthetic boundary failure") }
	tt.Frame()
	liveClick(t, tt, "排期任务 "+string(chain))
	if !s.runtimePoolPicker || len(svc.Snapshot().Traces) != 0 || s.runtimeError != naturalDayFailureMessage {
		t.Fatal("boundary failure not blocked")
	}
	s.refreshDay = nil
	s.runtimeError = ""
	s.runtimeDate = core.LocalDate{Year: 2026, Month: 11, Day: 4}
	tt.Frame()
	if !tt.HasText("任务池不能排期到过去日期。请先切到今天或未来日期。") {
		t.Fatal("past notice missing")
	}
	if _, ok := tt.Find("排期任务 " + string(chain)); ok {
		t.Fatal("past scheduling exposed")
	}
	liveClick(t, tt, "取消从任务池排期")
	liveClick(t, tt, "回到今天")
	liveClick(t, tt, "从任务池排期")
	s.observeRuntimeDay(func() time.Time { return time.Date(2026, 11, 6, 0, 1, 0, 0, time.UTC) })
	tt.Frame()
	liveClick(t, tt, "排期任务 "+string(chain))
	if svc.Snapshot().Traces[0].Date.Day != 6 || s.runtimeDate.Day != 6 {
		t.Fatal("rollover did not use current visible day")
	}
}
func TestLivePoolPickerEvidenceFrame(t *testing.T) {
	if os.Getenv("NOONMARK_UI_EVIDENCE_DIR") == "" {
		return
	}
	_, tt, svc, _ := liveTest(t)
	for _, title := range []string{"合成：整理产品评审记录", "合成：核对发布前回归清单", "合成：阅读与未来计划"} {
		if e := svc.AddPool(title); e != nil {
			t.Fatal(e)
		}
	}
	liveClick(t, tt, "从任务池排期")
	tt.Key(0, ui.KeyTab)
	savePickerTestFrame(t, tt, "live-pool-picker-2x.png")
}

// Preserve the tested focus/scroll state rather than re-rendering a new window.
func savePickerTestFrame(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("NOONMARK_UI_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute evidence directory required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	tt.SetScale(2)
	f, e := os.Create(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, tt.Image()); e != nil {
		f.Close()
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	tt.SetScale(1)
}
