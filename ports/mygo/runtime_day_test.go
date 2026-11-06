// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
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

func TestLiveDayStripPaginationKeyboardAndReadOnly(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	before := calendarDiskSignature(t, path)
	if s.runtimeDayRail {
		t.Fatal("source rail default must be collapsed")
	}
	if _, ok := tt.Find("日条选择日期 2026-10-30"); !ok {
		t.Fatal("today-6 missing")
	}
	if _, ok := tt.Find("日条选择日期 2026-11-12"); !ok {
		t.Fatal("today+7 missing")
	}
	liveClick(t, tt, "日条选择日期 2026-11-12")
	tt.Key(0, ui.KeyRight)
	if s.runtimeDate != (core.LocalDate{Year: 2026, Month: 11, Day: 13}) {
		t.Fatalf("strip page did not advance: date=%v focus=%q", s.runtimeDate, s.runtimeDayFocus)
	}
	if _, ok := tt.Find("日条选择日期 2026-11-26"); !ok {
		t.Fatal("next block not fixed14")
	}
	tt.Key(0, ui.KeyLeft)
	if s.runtimeDate.Day != 12 {
		t.Fatal("keyboard did not cross back")
	}
	liveClick(t, tt, "回到今天")
	liveType(t, tt, "保留日期导航草稿")
	tt.Key(0, ui.KeyLeft)
	if s.runtimeDate != liveTestDay {
		t.Fatal("date key stole editor input")
	}
	liveClick(t, tt, "前一天")
	if s.draft != "保留日期导航草稿" || len(svc.Snapshot().Days) != 0 || !reflect.DeepEqual(before, calendarDiskSignature(t, path)) {
		t.Fatal("navigation mutated facts or draft")
	}
	if !tt.HasText("历史已锁定") {
		t.Fatal("history badge missing")
	}
	if _, ok := tt.Find("开发任务标题"); ok {
		t.Fatal("history editor exposed")
	}
}
func TestLiveDayFutureQuickAddAndRestart(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	liveClick(t, tt, "后一天")
	liveType(t, tt, "明日原子新增")
	tt.Key(0, ui.KeyEnter)
	g := svc.Snapshot()
	if len(g.Chains) != 1 || len(g.Traces) != 1 || g.Traces[0].Date.Day != 6 || s.runtimeError != "" {
		t.Fatal("future input went to today", g, s.runtimeError)
	}
	trace := g.Traces[0].ID
	control, _ := tt.Find("切换完成 " + string(trace))
	tt.ClickAt(control.X+control.W/2, control.Y+control.H/2)
	if svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("future completion allowed")
	}
	liveClick(t, tt, "展开日待办详情")
	if !tt.HasText("未来日尚不能生成复盘。") {
		t.Fatal("fake future review")
	}
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	s.observeRuntimeDay(func() time.Time { return at })
	tt.Frame()
	liveClick(t, tt, "切换完成 "+string(trace))
	if svc.Snapshot().Traces[0].Status != core.TraceCompleted {
		t.Fatal("completion on arrival")
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(path, core.LocalDate{Year: 2026, Month: 11, Day: 7})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rs := newRuntimeApp(reopened)
	rs.runtimeDate = core.LocalDate{Year: 2026, Month: 11, Day: 6}
	rs.runtimeDayRail = true
	rt := ui.NewTester(rs.view, 1200, 768)
	if !rt.HasText("明日原子新增") || !rt.HasText("100%") || !rt.HasText("历史已锁定") {
		t.Fatal("future lifecycle lost on restart")
	}
}
func TestLiveDayDynamicRowsAndIndependentCompletion(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	long := strings.Repeat("长标题应自动换行且不能覆盖下一行，", 8)
	for _, title := range []string{long, "短标题"} {
		if err := svc.AddToday(title); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	g := svc.Snapshot()
	a, _ := tt.Find("查看任务 " + string(g.Traces[0].ID))
	b, _ := tt.Find("查看任务 " + string(g.Traces[1].ID))
	if a.H <= 44 || b.H != 44 || a.Y+a.H > b.Y {
		t.Fatal("row heights ignored text", a, b)
	}
	liveClick(t, tt, "切换完成 "+string(g.Traces[1].ID))
	if s.runtimeSelection != "" || s.runtimeDayRail || svc.Snapshot().Traces[1].Status != core.TraceCompleted {
		t.Fatal("completion click also selected row")
	}
	liveClick(t, tt, "查看任务 "+string(g.Traces[0].ID))
	if !s.runtimeDayRail || s.runtimeSelection != string(g.Traces[0].ID) {
		t.Fatal("selection did not expand detail")
	}
	// Long task detail must not intersect its status or controls.
	title, ok := tt.Find(long)
	if !ok || title.H <= 0 {
		t.Fatal("long task vanished")
	}
	_ = title
	detail, present := tt.Find("日任务详情标题")
	status, statusPresent := tt.Find("日任务详情状态")
	if !present || !statusPresent || detail.H <= 0 || status.H <= 0 || detail.Y+detail.H > status.Y {
		t.Fatal("long detail intersects status", detail, status)
	}
	tt.SetSize(960, 720)
	saveCalendarTestEvidence(t, s, "live-day-long-selected-min-2x.png", 960, 720)
}
func TestLiveDayReviewUsesSelectedDateAndTruthfulTrend(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	for i := 0; i < 3; i++ {
		if err := svc.AddToday(fmt.Sprintf("今日%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := svc.SetCompleted(svc.Snapshot().Traces[i].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	next, _ := liveTestDay.AddDays(1)
	if err := svc.AddOnDate("真实明日", next); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	liveClick(t, tt, "展开日待办详情")
	hero, heroOK := tt.Find("67%")
	label, labelOK := tt.Find("完成率")
	if !heroOK || !labelOK || hero.H <= 0 || label.H <= 0 || label.X+label.W >= hero.X || label.Y >= hero.Y+hero.H {
		t.Fatal("hero label/value must share a visible horizontal row", label, hero)
	}
	if !tt.HasText("已延期") || !tt.HasText("已废弃") {
		t.Fatal("zero-valued source statuses missing")
	}
	if !tt.HasText("67%") || !tt.HasText("明日有 1 项任务。") {
		t.Fatal("selected review counters")
	}
	liveClick(t, tt, "查看明天")
	if s.runtimeDate != next || !tt.HasText("未来日尚不能生成复盘。") || tt.HasText("67%") {
		t.Fatal("future reused today review")
	}
	liveClick(t, tt, "回到今天")
	liveClick(t, tt, "前一天")
	if !tt.HasText("0%") || tt.HasText("67%") {
		t.Fatal("empty past reused session totals")
	}
}
func TestLiveDayStripStaysFixedWhileEditorScrolls(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	for i := 0; i < 35; i++ {
		if err := svc.AddToday(fmt.Sprintf("滚动%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	strip, _ := tt.Find("日条选择日期 2026-11-05")
	input, _ := tt.Find("开发任务标题")
	tt.Scroll(700, 500, 0, 1300)
	after, _ := tt.Find("日条选择日期 2026-11-05")
	later, _ := tt.Find("开发任务标题")
	if strip != after || later.H != 0 || input.H <= 0 {
		t.Fatal("strip moved with list or editor was fixed", strip, after, input, later)
	}
	if s.runtimeDate != liveTestDay {
		t.Fatal("scroll navigated date")
	}
}
func TestLiveDaySourceGeometryHoverAndFocus(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	if err := svc.AddToday("几何标题"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	input, _ := tt.Find("开发任务标题")
	row, _ := tt.Find("查看任务 " + string(svc.Snapshot().Traces[0].ID))
	title, _ := tt.Find("几何标题")
	if input.Y != 146 || row.H != 44 || title.X != row.X+52 {
		t.Fatal("source input/row metrics", input, row, title)
	}
	x, y := int(row.X+row.W-10), int(row.Y+10)
	before := tt.Image().RGBAAt(x, y)
	tt.Move(float32(x), float32(y))
	if tt.Image().RGBAAt(x, y) == before {
		t.Fatal("row hover missing")
	}
	tt.Move(100, 600)
	liveClick(t, tt, "日条选择日期 2026-11-05")
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("日条选择日期 2026-11-06") {
		t.Fatal("tab focus not on next date")
	}
	tt.Key(0, ui.KeyRight)
	if s.runtimeDate.Day != 7 {
		t.Fatal("arrow ignored actual focus")
	}
}
func TestLiveDayVeryLongDetailScrollsToControls(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	if err := svc.AddToday(strings.Repeat("合成长详情需要安全滚动。", 120)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	liveClick(t, tt, "查看任务 "+string(svc.Snapshot().Traces[0].ID))
	tt.SetSize(960, 720)
	before, _ := tt.Find("收起日待办详情")
	tt.Scroll(850, 500, 0, 8000)
	button, ok := tt.Find("关闭任务详情")
	if !ok || button.H != 28 || button.Y != before.Y || button.Y+button.H > 56 {
		t.Fatal("long detail controls unreachable", button)
	}
	after, _ := tt.Find("收起日待办详情")
	if before != after {
		t.Fatal("rail toggle scrolled away")
	}
	liveClick(t, tt, "关闭任务详情")
	if s.runtimeSelection != "" || !tt.HasText("每日复盘") {
		t.Fatal("cannot return from long detail")
	}
}
func TestLiveDayInvalidGraphAndBoundaryFailureVisible(t *testing.T) {
	_, _, svc, _ := liveTest(t)
	s := newRuntimeApp(invalidCalendarService{svc})
	tt := ui.NewTester(s.view, 1200, 768)
	if !tt.HasText("日待办无法安全显示") || tt.HasText("这一天没有留下任务。") {
		t.Fatal("bad graph looks empty")
	}
	s = newRuntimeApp(svc)
	s.runtimeDate = core.LocalDate{Year: 2026, Month: 11, Day: 4}
	s.refreshDay = func() error { return fmt.Errorf("boundary failure") }
	tt = ui.NewTester(s.view, 960, 720)
	warning, ok := tt.Find(naturalDayFailureMessage)
	if !ok || warning.H <= 0 || warning.Y+warning.H > 720 {
		t.Fatal("warning hidden without editor", warning)
	}
}
func TestLiveDayEvidenceFrames(t *testing.T) {
	if os.Getenv("NOONMARK_UI_EVIDENCE_DIR") == "" {
		return
	}
	svc, err := taskapp.Create(filepath.Join(t.TempDir(), "session"), core.LocalDate{Year: 2026, Month: 7, Day: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	s := newRuntimeApp(svc)
	tt := ui.NewTester(s.view, 1200, 768)
	for _, title := range []string{"合成：整理今日评审材料", "合成：核对明日计划", "合成：完成回归验证"} {
		liveType(t, tt, title)
		liveSubmit(t, tt)
	}
	liveClick(t, tt, "切换完成 "+string(svc.Snapshot().Traces[0].ID))
	saveCalendarTestEvidence(t, s, "live-day-strip-2x.png", 1200, 768)
	liveClick(t, tt, "展开日待办详情")
	saveCalendarTestEvidence(t, s, "live-day-review-2x.png", 1200, 768)
	liveClick(t, tt, "后一天")
	liveType(t, tt, "合成：未来日期新增")
	tt.Key(0, ui.KeyEnter)
	saveCalendarTestEvidence(t, s, "live-day-future-2x.png", 1200, 768)
	liveClick(t, tt, "前一天")
	liveClick(t, tt, "前一天")
	saveCalendarTestEvidence(t, s, "live-day-history-2x.png", 1200, 768)
}
