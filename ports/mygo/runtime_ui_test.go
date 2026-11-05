// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var liveTestDay = core.LocalDate{Year: 2026, Month: 11, Day: 5}

func liveTest(t *testing.T) (*app, *ui.Tester, *taskapp.Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session")
	svc, e := taskapp.Create(path, liveTestDay)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { svc.Close() })
	s := newRuntimeApp(svc)
	return s, ui.NewTester(s.view, 1200, 768), svc, path
}
func liveClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if e := tt.Click(label); e != nil {
		t.Fatal(e)
	}
}
func liveType(t *testing.T, tt *ui.Tester, title string) {
	t.Helper()
	liveClick(t, tt, "开发任务标题")
	tt.Type(title)
}
func TestLiveUICommandsPersistAcrossRestart(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	if !tt.HasText("这一天没有留下任务。") || tt.HasText("修复登录阻断问题") {
		t.Fatal("static facts leaked into live session")
	}
	liveType(t, tt, "合成：编写发布说明")
	liveClick(t, tt, "添加任务")
	g := svc.Snapshot()
	if len(g.Traces) != 1 || s.draft != "" || s.runtimeError != "" || !tt.HasText("合成：编写发布说明") {
		t.Fatal("create via UI did not persist")
	}
	trace := g.Traces[0].ID
	liveClick(t, tt, "切换完成 "+string(trace))
	if svc.Snapshot().Traces[0].Status != core.TraceCompleted {
		t.Fatal("completion not persisted")
	}
	liveClick(t, tt, "切换到已完成")
	if !tt.HasText("合成：编写发布说明") {
		t.Fatal("completed projection missing")
	}
	liveClick(t, tt, "切换到任务池")
	liveType(t, tt, "合成：准备明天会议")
	tt.Key(0, ui.KeyEnter)
	g = svc.Snapshot()
	if len(g.Chains) != 2 || s.draft != "" {
		t.Fatal("enter did not submit exactly once")
	}
	chain := g.Chains[1].ID
	liveClick(t, tt, "排到明天 "+string(chain))
	liveClick(t, tt, "切换到未来计划")
	if !tt.HasText("合成：准备明天会议") {
		t.Fatal("future projection missing")
	}
	saved := svc.Snapshot()
	if e := svc.Close(); e != nil {
		t.Fatal(e)
	}
	restored, e := taskapp.Open(path, liveTestDay)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if !reflect.DeepEqual(saved, restored.Snapshot()) {
		t.Fatal("restart changed facts")
	}
	reopened := newRuntimeApp(restored)
	tt = ui.NewTester(reopened.view, 1200, 768)
	liveClick(t, tt, "切换完成 "+string(trace))
	if restored.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("undo after reopen")
	}
	liveClick(t, tt, "切换到任务池")
	if !tt.HasText("暂无任务") {
		t.Fatal("scheduled chain remained in pool")
	}
}
func TestLiveUIDraftsErrorsCancelAndRepeatedClicks(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	liveClick(t, tt, "添加任务")
	if len(svc.Snapshot().Chains) != 0 || s.runtimeError == "" {
		t.Fatal("blank title accepted")
	}
	liveType(t, tt, "保留草稿")
	liveClick(t, tt, "切换到任务池")
	if s.draft != "保留草稿" {
		t.Fatal("navigation lost unsaved draft")
	}
	liveClick(t, tt, "添加任务")
	liveClick(t, tt, "添加任务")
	if len(svc.Snapshot().Chains) != 1 {
		t.Fatal("repeated submit duplicated task")
	}
	liveType(t, tt, "磁盘失败时保留")
	before := svc.Snapshot()
	if e := os.Mkdir(filepath.Join(path, "unexpected"), 0700); e != nil {
		t.Fatal(e)
	}
	liveClick(t, tt, "添加任务")
	if s.draft != "磁盘失败时保留" || s.runtimeError == "" || !reflect.DeepEqual(before, svc.Snapshot()) {
		t.Fatal("failed save lost draft or published")
	}
	capture := newRuntimeApp(svc)
	capture.page = "quick-entry"
	ct := ui.NewTester(capture.view, 520, 168)
	liveType(t, ct, "取消的任务")
	liveClick(t, ct, "取消快速记录")
	if capture.page != "pool" || capture.draft != "" || !reflect.DeepEqual(before, svc.Snapshot()) {
		t.Fatal("cancel mutated data")
	}
}
func TestLiveUIDateAndUnsupportedPagesDoNotInventData(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	liveType(t, tt, "今日事项")
	liveClick(t, tt, "添加任务")
	liveClick(t, tt, "后一天")
	if s.runtimeDate.Day != 6 || !tt.HasText("这一天没有留下任务。") {
		t.Fatal("future date projection")
	}
	liveClick(t, tt, "回到今天")
	if !tt.HasText("今日事项") {
		t.Fatal("date restoration")
	}
	for _, title := range []string{"重复计划", "烛龙", "Sticky Note", "飞光"} {
		liveClick(t, tt, "切换到"+title)
		if !tt.HasText("此页面尚未连接领域与存储。") {
			t.Fatal("unsupported feature pretended available")
		}
	}
	if len(svc.Snapshot().Chains) != 1 {
		t.Fatal("navigation mutated data")
	}
}
func TestLiveUILongListScrollAndStableIdentity(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	for i := 0; i < 26; i++ {
		if e := svc.AddToday(fmt.Sprintf("长列表合成任务 %02d", i)); e != nil {
			t.Fatal(e)
		}
	}
	tt.Frame()
	last := svc.Snapshot().Traces[25]
	tt.Scroll(600, 500, 0, 1700)
	liveClick(t, tt, "切换完成 "+string(last.ID))
	if svc.Snapshot().Traces[25].Status != core.TraceCompleted {
		t.Fatal("scrolled row action targeted wrong identity")
	}
	liveClick(t, tt, "切换到任务池")
	liveClick(t, tt, "切换到Day Todo")
	if s.page != "day" {
		t.Fatal("navigation after scrolling")
	}
	tt.SetSize(960, 720)
	if e := renderApp(s, screenSpec{"day", 960, 720}, t.TempDir()); e != nil {
		t.Fatal(e)
	}
}
func TestLiveUICaptureAndSharedSession(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	capture := newRuntimeApp(svc)
	capture.page = "quick-entry"
	ct := ui.NewTester(capture.view, 520, 168)
	liveType(t, ct, "速记持久化")
	liveClick(t, ct, "保存到任务池")
	if len(svc.Snapshot().Chains) != 1 || capture.draft != "" {
		t.Fatal("capture not persisted")
	}
	liveClick(t, tt, "切换到任务池")
	if !tt.HasText("速记持久化") {
		t.Fatal("shared session not visible")
	}
	_ = s
}
func TestLiveUIEvidenceFrames(t *testing.T) {
	dir := os.Getenv("NOONMARK_UI_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute evidence path required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	s, tt, svc, _ := liveTest(t)
	for _, title := range []string{"合成：整理评审材料", "合成：确认明天议程", "合成：准备发布说明"} {
		liveType(t, tt, title)
		liveClick(t, tt, "添加任务")
	}
	liveClick(t, tt, "切换完成 "+string(svc.Snapshot().Traces[0].ID))
	save := func(name string) {
		t.Helper()
		f, e := os.Create(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = png.Encode(f, tt.Image()); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
	}
	save("live-day.png")
	liveClick(t, tt, "切换到任务池")
	liveType(t, tt, "合成：稍后处理的想法")
	liveClick(t, tt, "添加任务")
	save("live-pool.png")
	liveClick(t, tt, "切换到已完成")
	save("live-completed.png")
	_ = s
}

func TestLiveUICorruptHistoryRefusesSave(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	liveType(t, tt, "first")
	liveClick(t, tt, "添加任务")
	before := svc.Snapshot()
	file := filepath.Join(path, "rev-0000000002/state.json")
	b, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	b[len(b)-1] = '!'
	if e = os.WriteFile(file, b, 0600); e != nil {
		t.Fatal(e)
	}
	liveType(t, tt, "keep this draft")
	liveClick(t, tt, "添加任务")
	if s.runtimeError == "" || s.runtimeNotice != "" || s.draft != "keep this draft" || !reflect.DeepEqual(before, svc.Snapshot()) {
		t.Fatal("corrupt open session reported success or lost draft")
	}
	if _, e = os.Stat(filepath.Join(path, "rev-0000000003")); !os.IsNotExist(e) {
		t.Fatal("corrupt session reserved another revision")
	}
}

func TestLiveUINaturalDayAdvancesBeforeCommands(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	at := time.Date(2026, 11, 5, 23, 59, 0, 0, time.UTC)
	s.observeRuntimeDay(func() time.Time { return at })
	liveType(t, tt, "yesterday")
	liveClick(t, tt, "添加任务")
	at = at.Add(2 * time.Minute)
	tt.Frame()
	if s.runtimeDate.Day != 6 || svc.Today().Day != 6 || svc.Snapshot().Traces[0].Status != core.TraceUnfinished || !tt.HasText("2026年11月6日") {
		t.Fatal("day boundary not applied before view")
	}
	liveType(t, tt, "today")
	liveClick(t, tt, "添加任务")
	g := svc.Snapshot()
	if len(g.Traces) != 2 || g.Traces[1].Date.Day != 6 {
		t.Fatal("new command used yesterday")
	}
	second := newRuntimeApp(svc)
	second.runtimeDate = core.LocalDate{Year: 2026, Month: 11, Day: 4}
	second.observeRuntimeDay(func() time.Time { return at })
	st := ui.NewTester(second.view, 1200, 768)
	at = at.Add(24 * time.Hour)
	tt.Frame()
	st.Frame()
	if s.runtimeDate.Day != 7 || second.runtimeDate.Day != 4 || second.runtimeAppliedDay.Day != 7 {
		t.Fatal("shared boundary lost explicit historical selection")
	}
}
func TestLiveUIBoundaryFailureBlocksCommandsAndPreservesDraft(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	at := time.Date(2026, 11, 5, 23, 59, 0, 0, time.UTC)
	s.observeRuntimeDay(func() time.Time { return at })
	liveType(t, tt, "original")
	liveClick(t, tt, "添加任务")
	liveType(t, tt, "unsaved next day draft")
	before := svc.Snapshot()
	if e := os.Remove(filepath.Join(path, "SESSION_IDENTITY")); e != nil {
		t.Fatal(e)
	}
	at = at.Add(2 * time.Minute)
	tt.Frame()
	liveClick(t, tt, "添加任务")
	if svc.Today() != liveTestDay || s.runtimeDate != liveTestDay || s.draft != "unsaved next day draft" || s.runtimeError == "" || !reflect.DeepEqual(before, svc.Snapshot()) {
		t.Fatal("boundary failure partially published or allowed mutation")
	}
}
