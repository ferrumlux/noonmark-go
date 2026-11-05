// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"crypto/sha256"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

func calendarDiskSignature(t *testing.T, path string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(path, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		result[rel] = sha256.Sum256(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestLiveCalendarNavigationIsReadOnly(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	liveType(t, tt, "真实月历任务")
	liveClick(t, tt, "添加任务")
	before := svc.Snapshot()
	files := calendarDiskSignature(t, path)
	liveClick(t, tt, "切换到日历")
	if s.runtimeCalendarRail || !tt.HasText("2026年11月") || !tt.HasText("真实月历任务") {
		t.Fatal("calendar not connected")
	}
	liveClick(t, tt, "选择日期 2026-11-30")
	liveClick(t, tt, "下个月")
	if s.runtimeCalendarDate != (core.LocalDate{Year: 2026, Month: 12, Day: 1}) {
		t.Fatal("month navigation must select day one")
	}
	liveClick(t, tt, "上个月")
	liveClick(t, tt, "日历回到今天")
	if s.runtimeCalendarDate != liveTestDay {
		t.Fatal("today did not restore")
	}
	liveClick(t, tt, "选择日期 2026-11-06")
	liveClick(t, tt, "展开日历详情")
	if !tt.HasText("这一天没有留下任务。") || !tt.HasText("未来") {
		t.Fatal("empty future detail")
	}
	liveClick(t, tt, "在 Day Todo 打开这一天")
	if s.page != "day" || s.runtimeDate.Day != 6 || !tt.HasText("暂无任务") {
		t.Fatal("open selected date")
	}
	if !reflect.DeepEqual(before, svc.Snapshot()) || !reflect.DeepEqual(files, calendarDiskSignature(t, path)) {
		t.Fatal("read-only navigation wrote facts")
	}
}
func TestLiveCalendarKeyboardFocusAndYearLimits(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	before := calendarDiskSignature(t, path)
	liveClick(t, tt, "切换到日历")
	liveClick(t, tt, "选择日期 2026-11-30")
	tt.Key(0, ui.KeyRight)
	if s.runtimeCalendarDate != (core.LocalDate{Year: 2026, Month: 12, Day: 1}) {
		t.Fatal("right across month", s.runtimeCalendarDate)
	}
	tt.Key(0, ui.KeyUp)
	if s.runtimeCalendarDate != (core.LocalDate{Year: 2026, Month: 11, Day: 24}) {
		t.Fatal("up across month", s.runtimeCalendarDate)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyLeft)
	if s.runtimeCalendarDate != (core.LocalDate{Year: 2026, Month: 11, Day: 30}) {
		t.Fatal("down and left", s.runtimeCalendarDate)
	}
	liveClick(t, tt, "切换到任务池")
	liveType(t, tt, "草稿不能被日历键盘窃取")
	tt.Key(0, ui.KeyLeft)
	if s.runtimeCalendarDate.Day != 30 {
		t.Fatal("calendar caught another page key")
	}
	s.page = "calendar"
	s.selectCalendarDate(core.LocalDate{Year: 1, Month: 1, Day: 1}, false)
	tt.Frame()
	liveClick(t, tt, "选择日期 0001-01-01")
	tt.Key(0, ui.KeyLeft)
	if s.runtimeCalendarDate.Year != 1 || s.runtimeCalendarDate.Day != 1 {
		t.Fatal("minimum date overflow")
	}
	s.selectCalendarDate(core.LocalDate{Year: 9999, Month: 12, Day: 31}, false)
	tt.Frame()
	liveClick(t, tt, "选择日期 9999-12-31")
	tt.Key(0, ui.KeyRight)
	if s.runtimeCalendarDate.Year != 9999 || s.runtimeCalendarDate.Day != 31 {
		t.Fatal("maximum date overflow")
	}
	if len(svc.Snapshot().Chains) != 0 || !reflect.DeepEqual(before, calendarDiskSignature(t, path)) {
		t.Fatal("navigation mutated session")
	}
}
func TestLiveCalendarSettlementAndRestart(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	for _, title := range []string{"昨日完成", "昨日未完成"} {
		liveType(t, tt, title)
		liveClick(t, tt, "添加任务")
	}
	trace := svc.Snapshot().Traces[0].ID
	liveClick(t, tt, "切换完成 "+string(trace))
	next, _ := liveTestDay.AddDays(1)
	if err := svc.AdvanceTo(next); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(path, next)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s = newRuntimeApp(reopened)
	s.runtimeCalendarDate = liveTestDay
	s.page = "calendar"
	tt = ui.NewTester(s.view, 1200, 768)
	files := calendarDiskSignature(t, path)
	snapshot := reopened.Snapshot()
	liveClick(t, tt, "展开日历详情")
	if !tt.HasText("待完成 0 · 已完成 1 · 未完成 1") || !tt.HasText("昨日完成") || !tt.HasText("昨日未完成") || !tt.HasText("历史") {
		t.Fatal("settlement not reflected in calendar")
	}
	m, err := taskview.ProjectCalendar(reopened.Snapshot(), liveTestDay, next)
	if err != nil || m.SelectedDay.HeatLevel != 1 {
		t.Fatal("wrong calendar heat", err)
	}
	liveClick(t, tt, "收起日历详情")
	liveClick(t, tt, "选择日期 2026-11-07")
	if !reflect.DeepEqual(snapshot, reopened.Snapshot()) || !reflect.DeepEqual(files, calendarDiskSignature(t, path)) {
		t.Fatal("inspection changed committed audit")
	}
}
func TestLiveCalendarNaturalDaySelection(t *testing.T) {
	s, tt, svc, _ := liveTest(t)
	s.observeRuntimeDay(func() time.Time { return time.Date(2026, 11, 6, 0, 1, 0, 0, time.UTC) })
	s.page = "calendar"
	tt.Frame()
	if s.runtimeCalendarDate.Day != 6 || svc.Today().Day != 6 {
		t.Fatal("calendar failed to follow today")
	}
	s.runtimeCalendarDate = liveTestDay
	s.observeRuntimeDay(func() time.Time { return time.Date(2026, 11, 7, 0, 1, 0, 0, time.UTC) })
	tt.Frame()
	if s.runtimeCalendarDate != liveTestDay || svc.Today().Day != 7 {
		t.Fatal("historical selection lost")
	}
}

type invalidCalendarService struct{ *taskapp.Service }

func (s invalidCalendarService) Snapshot() core.TaskGraph {
	return core.TaskGraph{Traces: []core.DayTrace{{ID: "invalid"}}}
}
func TestLiveCalendarInvalidGraphNeverLooksEmpty(t *testing.T) {
	_, _, svc, _ := liveTest(t)
	s := newRuntimeApp(invalidCalendarService{svc})
	s.page = "calendar"
	tt := ui.NewTester(s.view, 1200, 768)
	if !tt.HasText("日历无法安全显示") || tt.HasText("这一天没有留下任务。") || tt.HasText("2026年11月") {
		t.Fatal("invalid facts hidden as empty calendar")
	}
}
func TestLiveCalendarTopologyAndHoverPixels(t *testing.T) {
	s, _, svc, _ := liveTest(t)
	s.page = "calendar"
	s.runtimeCalendarDate = core.LocalDate{Year: 2026, Month: 7, Day: 24}
	tt := ui.NewTester(s.view, 1200, 768)
	// July's two leading and two trailing blanks still keep their internal edges.
	cw := float32(962) / 7
	x, y := int(238+cw-1), int(calendarGridTop+20)
	if got := tt.Image().RGBAAt(x, y); got.R != line.R {
		t.Fatal("leading blank boundary missing", got)
	}
	before := tt.Image().RGBAAt(535, 160)
	tt.Move(535, 160)
	hover := tt.Image().RGBAAt(535, 160)
	if before == hover {
		t.Fatal("date hover missing")
	}
	tt.Move(100, 600)
	if tt.Image().RGBAAt(535, 160) != before {
		t.Fatal("date hover persisted")
	}
	// Six-row month and minimum supported window render through the same real view.
	s.runtimeCalendarDate = core.LocalDate{Year: 2026, Month: 8, Day: 31}
	tt.SetSize(960, 720)
	liveClick(t, tt, "选择日期 2026-08-31")
	if s.runtimeCalendarDate.Day != 31 || len(svc.Snapshot().Days) != 0 {
		t.Fatal("sixth-row selection")
	}
}
func TestLiveCalendarEvidenceFrames(t *testing.T) {
	dir := os.Getenv("NOONMARK_UI_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("external absolute evidence root required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "calendar-session")
	today := core.LocalDate{Year: 2026, Month: 7, Day: 24}
	svc, err := taskapp.Create(path, today)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	s := newRuntimeApp(svc)
	tt := ui.NewTester(s.view, 1200, 768)
	for i := 0; i < 8; i++ {
		liveType(t, tt, fmt.Sprintf("合成月历：第%d项任务，验证长标题不越过格线", i+1))
		liveClick(t, tt, "添加任务")
	}
	liveClick(t, tt, "切换完成 "+string(svc.Snapshot().Traces[0].ID))
	liveClick(t, tt, "切换到日历")
	save := func(name string, w, h int) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, ui.Render(s.view, w, h, 2))
		ce := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if ce != nil {
			t.Fatal(ce)
		}
	}
	save("live-calendar-five-rows-2x.png", 1200, 768)
	if !tt.HasText("+2") {
		t.Fatal("six previews plus remainder missing")
	}
	liveClick(t, tt, "展开日历详情")
	save("live-calendar-detail-2x.png", 1200, 768)
	liveClick(t, tt, "收起日历详情")
	liveClick(t, tt, "下个月")
	save("live-calendar-six-rows-2x.png", 1200, 768)
	tt.SetSize(960, 720)
	save("live-calendar-min-size-2x.png", 960, 720)
}

func TestLiveCalendarBoundaryFailureIsVisibleAndRecovers(t *testing.T) {
	s, tt, svc, path := liveTest(t)
	s.page = "calendar"
	s.draft = "未提交草稿"
	before := svc.Snapshot()
	files := calendarDiskSignature(t, path)
	s.refreshDay = func() error { return fmt.Errorf("synthetic boundary error") }
	tt.Frame()
	if s.runtimeError != naturalDayFailureMessage || !tt.HasText(naturalDayFailureMessage) {
		t.Fatal("boundary failure hidden by calendar")
	}
	box, ok := tt.Find(naturalDayFailureMessage)
	if !ok || box.H <= 0 || box.Y+box.H > calendarGridTop {
		t.Fatal("warning not visible above grid", box)
	}
	saveCalendarTestEvidence(t, s, "live-calendar-boundary-warning-2x.png", 1200, 768)
	if s.draft != "未提交草稿" || !reflect.DeepEqual(before, svc.Snapshot()) || !reflect.DeepEqual(files, calendarDiskSignature(t, path)) {
		t.Fatal("warning mutated facts or draft")
	}
	s.refreshDay = func() error { return nil }
	tt.Frame()
	if s.runtimeError != "" || tt.HasText(naturalDayFailureMessage) {
		t.Fatal("successful boundary did not clear its stale warning")
	}
}
func TestCalendarPreviewFit(t *testing.T) {
	for _, tc := range []struct {
		h           float32
		total, want int
		row, footer float32
	}{
		{131, 8, 6, 12, 11}, {109.5, 8, 5, 12, 11}, {101.5, 8, 4, 12, 11}, {88, 8, 3, 12, 11},
		{131, 8, 6, 15, 14}, {109.5, 8, 4, 15, 14}, {101.5, 8, 4, 15, 14}, {101.5, 5, 4, 15, 14}, {131, 0, 0, 15, 14},
	} {
		n, hidden := calendarPreviewFit(tc.h, tc.total, tc.row, tc.footer)
		if n != tc.want || n+hidden != tc.total {
			t.Fatalf("fit %+v got %d+%d", tc, n, hidden)
		}
		if hidden > 0 && 23+max(float32(14), tc.row)*float32(n)+tc.footer > tc.h-4 {
			t.Fatal("count clipped")
		}
	}
}

func TestLiveCalendarDenseSixRowsShowsActualRemainder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	day := core.LocalDate{Year: 2026, Month: 8, Day: 31}
	svc, err := taskapp.Create(path, day)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	for i := 0; i < 8; i++ {
		if err := svc.AddToday(fmt.Sprintf("密集任务%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	s := newRuntimeApp(svc)
	s.page = "calendar"
	tt := ui.NewTester(s.view, 960, 720)
	cell, ok := tt.Find("选择日期 2026-08-31")
	if !ok {
		t.Fatal("last row missing")
	}
	count, ok := tt.Find("+4")
	if !ok || count.H <= 0 || count.Y+count.H > cell.Y+cell.H {
		t.Fatal("actual remainder not visually inside dense cell", cell, count)
	}
	last, ok := tt.Find("密集任务3")
	if !ok || last.H <= 0 || last.Y+last.H > count.Y {
		t.Fatal("last visible preview clipped", last, count)
	}
	saveCalendarTestEvidence(t, s, "live-calendar-dense-six-2x.png", 960, 720)
	liveClick(t, tt, "展开日历详情")
	tt.Scroll(850, 600, 0, 600)
	if !tt.HasText("密集任务7") {
		t.Fatal("adaptive preview lost full detail")
	}
}
func TestLiveCalendarTabFocusIsVisibleAndArrowStartsAtFocus(t *testing.T) {
	s, tt, _, _ := liveTest(t)
	liveClick(t, tt, "切换到日历")
	liveClick(t, tt, "选择日期 2026-11-05")
	tt.Move(100, 600)
	before := append([]byte(nil), tt.Image().Pix...)
	tt.Key(0, ui.KeyTab)
	if s.runtimeCalendarFocus != "2026-11-06" || reflect.DeepEqual(before, tt.Image().Pix) {
		t.Fatal("Tab focus invisible or wrong")
	}
	tt.Key(0, ui.KeyRight)
	if s.runtimeCalendarDate != (core.LocalDate{Year: 2026, Month: 11, Day: 7}) {
		t.Fatal("arrow ignored actual focus", s.runtimeCalendarDate)
	}
}

func TestCalendarMeasuredHeightMatchesRenderedText(t *testing.T) {
	var titleHeight, footerHeight float32
	tt := ui.NewTester(func(c *ui.Context) {
		titleHeight = calendarMeasureHeight(c, "measure-title", 9.5, []string{"密集任务"})
		footerHeight = calendarMeasureHeight(c, "measure-footer", 9, []string{"+4"})
		txt(c, 10, 10, 9.5, "密集任务", ink, 400).SingleLine()
		txt(c, 10, 50, 9, "+4", ink, 400)
	}, 200, 120)
	title, _ := tt.Find("密集任务")
	footer, _ := tt.Find("+4")
	if titleHeight != title.H || footerHeight != footer.H {
		t.Fatalf("measurement differs from renderer: %.2f/%.2f actual %.2f/%.2f", titleHeight, footerHeight, title.H, footer.H)
	}
}

func saveCalendarTestEvidence(t *testing.T, s *app, name string, w, h int) {
	t.Helper()
	dir := os.Getenv("NOONMARK_UI_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("external absolute evidence root required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	f, e := os.Create(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	e = png.Encode(f, ui.Render(s.view, w, h, 2))
	ce := f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if ce != nil {
		t.Fatal(ce)
	}
}
func TestLiveCalendarFiveRowsPreservesSixVisiblePreviews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	day := core.LocalDate{Year: 2026, Month: 7, Day: 24}
	svc, err := taskapp.Create(path, day)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	for i := 0; i < 8; i++ {
		if err := svc.AddToday(fmt.Sprintf("普通月格任务%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	s := newRuntimeApp(svc)
	s.page = "calendar"
	tt := ui.NewTester(s.view, 1200, 768)
	cell, _ := tt.Find("选择日期 2026-07-24")
	count, ok := tt.Find("+2")
	last, lastOK := tt.Find("普通月格任务5")
	if !ok || !lastOK || count.H <= 0 || last.H <= 0 || last.Y+last.H > count.Y || count.Y+count.H > cell.Y+cell.H {
		t.Fatal("source six previews plus count not visible without collision", cell, last, count)
	}
}
