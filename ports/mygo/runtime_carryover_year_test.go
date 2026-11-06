// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

// The production FIFO worker executes real Service calls. Gates only control
// when a queued job may start, allowing native duplicate-confirm checks while
// the writer is pending. No business result or acknowledgement is simulated.
type yearWriterJob struct{ release, done chan struct{} }
type yearWriter struct {
	lane *coordinator.SerialLane
	jobs []yearWriterJob
}

func (w *yearWriter) Enqueue(fn func()) {
	j := yearWriterJob{make(chan struct{}), make(chan struct{})}
	w.jobs = append(w.jobs, j)
	w.lane.Enqueue(func() { <-j.release; fn(); close(j.done) })
}
func (w *yearWriter) one(t *testing.T) {
	t.Helper()
	j := w.jobs[0]
	w.jobs = w.jobs[1:]
	close(j.release)
	select {
	case <-j.done:
	case <-time.After(60 * time.Second):
		t.Fatal("real Service writer failed to complete within test bound")
	}
}

type yearServiceCall struct {
	Kind        string
	Date        core.LocalDate
	Source, Ack core.DayTraceID
	Elapsed     time.Duration
}
type yearCountingStore struct {
	*taskapp.Service
	mu    sync.Mutex
	calls []yearServiceCall
}

func (s *yearCountingStore) record(call yearServiceCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}
func (s *yearCountingStore) observed() []yearServiceCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]yearServiceCall(nil), s.calls...)
}
func (s *yearCountingStore) AdvanceTo(date core.LocalDate) error {
	start := time.Now()
	err := s.Service.AdvanceTo(date)
	s.record(yearServiceCall{Kind: "advance", Date: date, Elapsed: time.Since(start)})
	return err
}
func (s *yearCountingStore) DeferCurrentTrace(chain core.TaskChainID, trace core.DayTraceID, date core.LocalDate) (core.DayTraceID, error) {
	start := time.Now()
	id, err := s.Service.DeferCurrentTrace(chain, trace, date)
	s.record(yearServiceCall{Kind: "defer", Date: date, Source: trace, Ack: id, Elapsed: time.Since(start)})
	return id, err
}
func (s *yearCountingStore) ContinueUnfinishedTrace(chain core.TaskChainID, trace core.DayTraceID, date core.LocalDate) (core.DayTraceID, error) {
	start := time.Now()
	id, err := s.Service.ContinueUnfinishedTrace(chain, trace, date)
	s.record(yearServiceCall{Kind: "continue", Date: date, Source: trace, Ack: id, Elapsed: time.Since(start)})
	return id, err
}

type yearUI struct {
	t       *testing.T
	s       *app
	tt      *ui.Tester
	h       *titleHub
	store   *yearCountingStore
	writer  yearWriter
	posts   chan func()
	clock   titleTestClock
	invalid bool
	closed  int
}

func newYearUI(t *testing.T, service *taskapp.Service) *yearUI {
	t.Helper()
	f := &yearUI{t: t, store: &yearCountingStore{Service: service}, posts: make(chan func(), 128)}
	f.clock.now = time.Date(liveTestDay.Year, time.Month(liveTestDay.Month), liveTestDay.Day, 12, 0, 0, 0, time.UTC)
	f.writer.lane = coordinator.NewSerialLane()
	f.h = newTitleHub(f.store, &f.writer, func(fn func()) { f.posts <- fn }, &f.clock, f.clock.Now, func() {})
	f.s = newRuntimeApp(f.h.readView())
	f.h.attach(f.s, func() { f.invalid = true }, func() {
		if f.h.beforeClose(f.s) {
			f.closed++
		}
	})
	f.tt = ui.NewTester(f.s.view, 1200, 768)
	f.all()
	t.Cleanup(func() {
		// Release any pending gate on a failed assertion before stopping the real
		// FIFO. This does not issue a new command, cancel one or retry storage.
		for _, job := range f.writer.jobs {
			close(job.release)
		}
		f.writer.jobs = nil
		f.writer.lane.StopAfterDrain()
		select {
		case <-f.writer.lane.Done():
		case <-time.After(60 * time.Second):
			t.Error("year writer cleanup did not drain")
		}
	})
	return f
}
func (f *yearUI) ui() {
	f.t.Helper()
	for n := 0; n < 200; n++ {
		select {
		case fn := <-f.posts:
			fn()
			continue
		default:
		}
		if f.invalid {
			f.invalid = false
			f.tt.Frame()
			continue
		}
		return
	}
	f.t.Fatal("year native UI did not settle")
}
func (f *yearUI) all() {
	f.t.Helper()
	for n := 0; n < 100; n++ {
		f.ui()
		if len(f.writer.jobs) == 0 {
			return
		}
		f.writer.one(f.t)
	}
	f.t.Fatal("year writer queue did not settle")
}
func (f *yearUI) click(label string) {
	f.t.Helper()
	liveClick(f.t, f.tt, label)
	f.all()
	f.tt.Frame()
	f.ui()
}
func (f *yearUI) bringTrace(trace core.DayTraceID) {
	f.t.Helper()
	label := "查看任务 " + string(trace)
	for n := 0; n < 20; n++ {
		r, ok := f.tt.Find(label)
		if !ok {
			f.t.Fatal("expected year row absent", label)
		}
		if r.Y >= 150 && r.Y+r.H <= 748 {
			return
		}
		dy := float32(400)
		if r.Y < 150 {
			dy = -400
		}
		f.tt.Scroll(700, 500, 0, dy)
		f.all()
	}
	f.t.Fatal("native scrolling could not expose the year row", label)
}
func (f *yearUI) selectTrace(trace core.DayTraceID) {
	f.t.Helper()
	f.bringTrace(trace)
	f.click("查看任务 " + string(trace))
	if !f.s.runtimeDayRail {
		f.click("展开日待办详情")
	}
	if f.s.runtimeSelection != string(trace) {
		f.t.Fatal("native selection missed source", trace)
	}
}
func (f *yearUI) menu(trace core.DayTraceID, want []string) {
	f.t.Helper()
	f.bringTrace(trace)
	openReturnMenu(f.t, f.tt, trace)
	if !reflect.DeepEqual(f.tt.Menu(), want) {
		f.t.Fatalf("native menu %s = %v want %v", trace, f.tt.Menu(), want)
	}
	f.tt.CloseMenu()
	f.all()
}
func (f *yearUI) open(trace core.DayTraceID, label string) {
	f.t.Helper()
	openReturnMenu(f.t, f.tt, trace)
	if err := f.tt.ChooseMenuItem(label); err != nil {
		f.t.Fatal(err)
	}
	f.all()
	f.tt.Frame()
	if f.s.carryover == nil || f.s.carryover.picker.identity.Trace != trace {
		f.t.Fatal("wrong source opened", f.s.runtimeError)
	}
}
func (f *yearUI) close() {
	f.t.Helper()
	if f.h.beforeClose(f.s) {
		f.closed++
	}
	f.all()
	if f.closed != 1 || len(f.h.windows) != 0 || len(f.h.carryoverInFlight) != 0 || len(f.writer.jobs) != 0 {
		f.t.Fatal("native Close did not detach cleanly")
	}
}

// Advance only the headless presentation clock before recording the actual
// rendered pixels. Keep all raw images from earlier intermediate animation runs.
func saveYearFrame(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if os.Getenv("NOONMARK_UI_EVIDENCE_DIR") == "" {
		return
	}
	tt.Advance(200 * time.Millisecond)
	savePickerTestFrame(t, tt, name)
}

func yearTrace(t *testing.T, graph core.TaskGraph, date core.LocalDate, status core.TraceStatus) core.DayTrace {
	t.Helper()
	for _, trace := range graph.Traces {
		if trace.Date == date && trace.Status == status {
			return trace
		}
	}
	t.Fatal("fixture source absent", date, status)
	return core.DayTrace{}
}
func yearTraceID(t *testing.T, graph core.TaskGraph, id core.DayTraceID) core.DayTrace {
	t.Helper()
	for _, trace := range graph.Traces {
		if trace.ID == id {
			return trace
		}
	}
	t.Fatal("trace ID absent", id)
	return core.DayTrace{}
}
func yearSnapshot(t *testing.T, path string) (canonical.Value, int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(path, "*", "state.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing year revision", err)
	}
	sort.Strings(files)
	data, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Revision int
		Payload  []byte
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	codec := v7.New(v7.Options{ClassificationAuditValidator: taskapp.ValidateEmptyHistory})
	doc, err := codec.Decode(record.Payload)
	if err != nil {
		t.Fatal(err)
	}
	value := doc.Snapshot()
	if err = taskapp.ValidateEmptyHistory(value); err != nil {
		t.Fatal(err)
	}
	return value, record.Revision
}
func yearHistoricalFacts(graph core.TaskGraph) core.TaskGraph {
	out := core.TaskGraph{}
	for _, day := range graph.Days {
		if day.Date.Compare(liveTestDay) < 0 {
			out.Days = append(out.Days, day)
		}
	}
	for _, trace := range graph.Traces {
		if trace.Date.Compare(liveTestDay) < 0 {
			out.Traces = append(out.Traces, trace)
		}
	}
	return out
}
func yearAssertUnrelated(t *testing.T, before, after core.TaskGraph, chain core.TaskChainID, source core.DayTraceID, continuation bool) {
	t.Helper()
	for _, trace := range before.Traces {
		if !continuation && trace.ID == source {
			continue
		}
		if !reflect.DeepEqual(trace, yearTraceID(t, after, trace.ID)) {
			t.Fatal("unrelated/original source trace changed", trace.ID)
		}
	}
	if !reflect.DeepEqual(before.Definitions, after.Definitions) || !reflect.DeepEqual(before.Subtasks, after.Subtasks) {
		t.Fatal("unrelated definitions/children changed")
	}
	for _, old := range before.Chains {
		if old.ID == chain {
			found := false
			for _, now := range after.Chains {
				if now.ID == old.ID {
					found = true
					now.UpdatedAt = old.UpdatedAt
					if !reflect.DeepEqual(old, now) {
						t.Fatal("operated chain changed beyond its legal update clock")
					}
				}
			}
			if !found {
				t.Fatal("operated chain vanished")
			}
			continue
		}
		found := false
		for _, now := range after.Chains {
			if now.ID == old.ID {
				found = true
				if !reflect.DeepEqual(old, now) {
					t.Fatal("unrelated chain changed", old.ID)
				}
			}
		}
		if !found {
			t.Fatal("unrelated chain vanished")
		}
	}
	if !reflect.DeepEqual(before.Days, after.Days) {
		t.Fatal("existing target/source day changed")
	}
	if !reflect.DeepEqual(yearHistoricalFacts(before), yearHistoricalFacts(after)) {
		t.Fatal("historical trace/day facts changed")
	}
}
func yearAssertNoWrite(t *testing.T, path string, service *taskapp.Service, graph core.TaskGraph, disk map[string][32]byte) {
	t.Helper()
	if !reflect.DeepEqual(graph, service.Snapshot()) || !reflect.DeepEqual(disk, calendarDiskSignature(t, path)) {
		t.Fatal("read-only UI/cancel/navigation wrote")
	}
	if service.Today() != liveTestDay {
		t.Fatal("browsing changed the actual Today")
	}
}

func TestCarryoverYearMiddleNativeService(t *testing.T) {
	base := t.TempDir()
	if dir := os.Getenv("NOONMARK_CARRYOVER_YEAR_EVIDENCE_DIR"); dir != "" {
		if !filepath.IsAbs(dir) {
			t.Fatal("absolute year evidence directory required")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		var err error
		base, err = os.MkdirTemp(dir, "development-year-")
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(base, "session")
	buildYearFixture(t, path)
	service, err := taskapp.Open(path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	before := service.Snapshot()
	if len(before.Days) != 365 || len(before.Traces) != 762 {
		t.Fatal("year fixture absent")
	}
	history, revision := yearSnapshot(t, path)
	if revision != 2 {
		t.Fatal("unexpected fixture installation revisions", revision)
	}
	yesterday, _ := liveTestDay.AddDays(-1)
	target, _ := liveTestDay.AddDays(2)
	todaySource := yearTrace(t, before, liveTestDay, core.TracePending)
	for _, trace := range before.Traces {
		if trace.Date == liveTestDay && trace.Status == core.TracePending && trace.CreatedAt.After(todaySource.CreatedAt) {
			todaySource = trace
		}
	}
	pastSource := yearTrace(t, before, yesterday, core.TraceUnfinished)
	completedToday := yearTrace(t, before, liveTestDay, core.TraceCompleted)
	completedPast := yearTrace(t, before, yesterday, core.TraceCompleted)
	futureSource := yearTrace(t, before, target, core.TracePending)
	disk := calendarDiskSignature(t, path)
	f := newYearUI(t, service)
	f.click("展开日待办详情")
	stripBefore, ok := f.tt.Find("日条选择日期 " + liveTestDay.String())
	if !ok {
		t.Fatal("date strip absent before long scroll")
	}
	beforeScroll, ok := f.tt.Find("查看任务 " + string(todaySource.ID))
	if !ok || beforeScroll.Y < 768 {
		t.Fatal("fixture did not produce an offscreen last row", beforeScroll)
	}
	if err := f.tt.RightClick("查看任务 " + string(todaySource.ID)); err == nil {
		t.Fatal("offscreen row unexpectedly accepted native menu hit")
	}
	if f.tt.Menu() != nil || f.s.runtimeSelection != "" {
		t.Fatal("offscreen failed click changed native state")
	}
	saveYearFrame(t, f.tt, "year-00-long-list-top-2x.png")
	f.selectTrace(todaySource.ID)
	afterScroll, ok := f.tt.Find("查看任务 " + string(todaySource.ID))
	if !ok || afterScroll.Y < stripBefore.Y+stripBefore.H || afterScroll.Y+afterScroll.H > 748 || afterScroll.Y >= beforeScroll.Y {
		t.Fatal("last row not truly visible after native scroll", beforeScroll, afterScroll)
	}
	stripAfter, ok := f.tt.Find("日条选择日期 " + liveTestDay.String())
	if !ok || stripBefore != stripAfter {
		t.Fatal("long scrolling moved fixed date strip")
	}
	t.Logf("native long scroll today_rows=34 viewport=1200x768 source_before=%+v source_after=%+v fixed_strip=%+v", beforeScroll, afterScroll, stripAfter)
	f.menu(todaySource.ID, []string{"标记完成", deferTaskLabel, returnToPoolLabel})
	f.menu(completedToday.ID, []string{"撤销完成"})
	saveYearFrame(t, f.tt, "year-01-today-before-2x.png")
	f.open(todaySource.ID, deferTaskLabel)
	f.click("选择日期 " + target.String())
	f.tt.Key(0, ui.KeyEscape)
	f.all()
	if f.s.carryover != nil {
		t.Fatal("cancel did not close sheet")
	}
	yearAssertNoWrite(t, path, service, before, disk)
	f.open(todaySource.ID, deferTaskLabel)
	f.click("选择日期 " + target.String())
	panel, ok := f.tt.Find("延期日期对话框")
	if !ok || panel.W != 340 || panel.H != 395 || panel.X < 0 || panel.Y < 0 || panel.X+panel.W > 1200 || panel.Y+panel.H > 768 {
		t.Fatal("scrolled row popup not contained in viewport", panel)
	}
	saveYearFrame(t, f.tt, "year-02-defer-picker-2x.png")
	// Confirm through the real date button, leave the production writer gated,
	// then send duplicate Return events before allowing any storage mutation.
	liveClick(t, f.tt, "确认延期")
	f.ui()
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.s.carryover == nil || !f.s.carryover.pending || len(f.writer.jobs) != 1 || len(f.store.observed()) != 0 {
		t.Fatal("defer confirmation was early or duplicated")
	}
	yearAssertNoWrite(t, path, service, before, disk)
	started := time.Now()
	f.all()
	t.Logf("defer native confirmation-through-publication elapsed=%s", time.Since(started))
	afterDefer := service.Snapshot()
	calls := f.store.observed()
	if len(calls) != 2 || calls[0].Kind != "advance" || calls[1].Kind != "defer" || calls[0].Date != liveTestDay {
		t.Fatal("defer FIFO calls", calls)
	}
	ack := calls[1].Ack
	if ack == "" || ack == todaySource.ID || f.s.runtimeSelection != string(ack) || f.s.runtimeDate != target || f.s.carryover != nil || f.s.runtimeError != "" {
		t.Fatal("defer acknowledgement/navigation", calls, f.s.runtimeError)
	}
	newTrace := yearTraceID(t, afterDefer, ack)
	oldTrace := yearTraceID(t, afterDefer, todaySource.ID)
	if len(afterDefer.Traces) != 763 || len(afterDefer.Days) != 365 || newTrace.ChainID != todaySource.ChainID || newTrace.Date != target || newTrace.Status != core.TracePending || newTrace.ContinuationSeq != todaySource.ContinuationSeq || newTrace.CarriedFromTraceID == nil || *newTrace.CarriedFromTraceID != todaySource.ID || oldTrace.Status != core.TraceDeferred || oldTrace.SettledAt == nil || !reflect.DeepEqual(oldTrace.ChangedToTraceID, todaySource.ChangedToTraceID) {
		t.Fatal("defer identity/fact mismatch")
	}
	// Defer can settle/unpin the old source, but cannot silently rewrite any
	// other original content or identity while creating the acknowledged row.
	sourceContent := oldTrace
	sourceContent.Status = todaySource.Status
	sourceContent.ContentUpdatedAt = todaySource.ContentUpdatedAt
	sourceContent.SettledAt = todaySource.SettledAt
	sourceContent.PinOrder = todaySource.PinOrder
	if !reflect.DeepEqual(sourceContent, todaySource) || newTrace.DefinitionID != todaySource.DefinitionID {
		t.Fatal("defer changed original source content/definition")
	}
	yearAssertUnrelated(t, before, afterDefer, todaySource.ChainID, todaySource.ID, false)
	afterDeferWire, revision := yearSnapshot(t, path)
	if revision != 3 {
		t.Fatal("defer wrote more than one revision", revision)
	}
	// Every previously captured snapshot/event remains unchanged; only the
	// newly deferred today source may append its own classification audit.
	for _, key := range []string{"snapshotsByTraceID", "snapshotEventsByTraceID"} {
		old := history.Object["classifications"].Object[key].Array
		now := afterDeferWire.Object["classifications"].Object[key].Array
		for i := 0; i < len(old); i += 2 {
			found := false
			for j := 0; j < len(now); j += 2 {
				if reflect.DeepEqual(old[i], now[j]) {
					found = true
					if !reflect.DeepEqual(old[i+1], now[j+1]) {
						t.Fatal("defer changed existing history audit", key)
					}
				}
			}
			if !found {
				t.Fatal("defer lost existing history audit", key)
			}
		}
	}
	f.menu(futureSource.ID, []string{returnToPoolLabel})
	saveYearFrame(t, f.tt, "year-03-deferred-future-2x.png")
	disk = calendarDiskSignature(t, path)
	f.click("回到今天")
	f.click("前一天")
	if f.s.runtimeDate != yesterday || !f.tt.HasText("历史已锁定") {
		t.Fatal("native historical navigation failed")
	}
	f.selectTrace(pastSource.ID)
	f.menu(pastSource.ID, []string{continueTaskLabel})
	f.menu(completedPast.ID, nil)
	if f.s.titleEditor != nil || actualSubtaskTargetEditable(f.h.graph, f.h.today, runtimeSubtaskTarget{chain: pastSource.ChainID, trace: pastSource.ID}) {
		t.Fatal("historical source exposed editable owner")
	}
	// Clicking the static title and typing must not create an editor or mutate
	// history. Check the service and disk, not only the displayed text.
	liveClick(t, f.tt, "日任务详情标题")
	f.tt.Type("不应写入历史")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	yearAssertNoWrite(t, path, service, afterDefer, disk)
	saveYearFrame(t, f.tt, "year-04-history-readonly-2x.png")
	f.open(pastSource.ID, continueTaskLabel)
	f.click("选择日期 " + target.String())
	f.click("日期选择回到今天")
	if f.s.carryover == nil || f.s.carryover.picker.selected != liveTestDay {
		t.Fatal("continue today shortcut failed")
	}
	saveYearFrame(t, f.tt, "year-05-continue-picker-2x.png")
	liveClick(t, f.tt, "确认延续")
	f.ui()
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.s.carryover == nil || !f.s.carryover.pending || len(f.writer.jobs) != 1 || len(f.store.observed()) != 2 {
		t.Fatal("continue confirmation was early or duplicated")
	}
	yearAssertNoWrite(t, path, service, afterDefer, disk)
	started = time.Now()
	f.all()
	t.Logf("continue native confirmation-through-publication elapsed=%s", time.Since(started))
	afterContinue := service.Snapshot()
	calls = f.store.observed()
	if len(calls) != 4 || calls[2].Kind != "advance" || calls[3].Kind != "continue" || calls[2].Date != liveTestDay {
		t.Fatal("continue FIFO calls", calls)
	}
	continued := calls[3].Ack
	if continued == "" || continued == pastSource.ID || continued == ack || f.s.runtimeSelection != string(continued) || f.s.runtimeDate != liveTestDay || f.s.carryover != nil || f.s.runtimeError != "" {
		t.Fatal("continue acknowledgement/navigation", calls, f.s.runtimeError)
	}
	newTrace = yearTraceID(t, afterContinue, continued)
	if len(afterContinue.Traces) != 764 || len(afterContinue.Days) != 365 || newTrace.ChainID != pastSource.ChainID || newTrace.Date != liveTestDay || newTrace.Status != core.TracePending || newTrace.CarriedFromTraceID == nil || *newTrace.CarriedFromTraceID != pastSource.ID || newTrace.ContinuationSeq != pastSource.ContinuationSeq+1 {
		t.Fatal("continue identity/fact mismatch")
	}
	if newTrace.DefinitionID != pastSource.DefinitionID {
		t.Fatal("continue changed definition identity")
	}
	yearAssertUnrelated(t, afterDefer, afterContinue, pastSource.ChainID, pastSource.ID, true)
	afterContinueWire, revision := yearSnapshot(t, path)
	if revision != 4 || !reflect.DeepEqual(afterDeferWire.Object["classifications"], afterContinueWire.Object["classifications"]) {
		t.Fatal("continue changed source audit or wrote duplicate revisions", revision)
	}
	saveYearFrame(t, f.tt, "year-06-continued-today-2x.png")
	disk = calendarDiskSignature(t, path)
	f.click("前一天")
	f.selectTrace(pastSource.ID)
	f.menu(pastSource.ID, nil) // Existing new pending trace prevents re-continuation.
	f.click("后一天")
	f.click("后一天")
	f.click("后一天")
	f.selectTrace(ack)
	if f.s.runtimeDate != target || f.s.runtimeSelection != string(ack) {
		t.Fatal("native future navigation lost deferred task")
	}
	for i := 0; i < 5; i++ {
		f.tt.Frame()
		f.all()
	}
	yearAssertNoWrite(t, path, service, afterContinue, disk)
	if len(f.store.observed()) != 4 {
		t.Fatal("navigation/frame replayed command", f.store.observed())
	}
	f.close()
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	yearAssertNoWrite(t, path, reopened, afterContinue, disk)
	reopenWire, revision := yearSnapshot(t, path)
	if revision != 4 || !reflect.DeepEqual(reopenWire, afterContinueWire) {
		t.Fatal("Close/Open changed full audited snapshot")
	}
	r := newYearUI(t, reopened)
	r.selectTrace(continued)
	r.click("前一天")
	r.selectTrace(pastSource.ID)
	if !r.tt.HasText("历史已锁定") || r.s.titleEditor != nil {
		t.Fatal("reopened historical source no longer read-only")
	}
	r.click("回到今天")
	r.click("日条选择日期 " + target.String())
	r.selectTrace(ack)
	saveYearFrame(t, r.tt, "year-07-reopened-future-2x.png")
	yearAssertNoWrite(t, path, reopened, afterContinue, disk)
	if len(r.store.observed()) != 0 {
		t.Fatal("reopen browsing unexpectedly wrote", r.store.observed())
	}
	r.close()
	for i, call := range calls {
		t.Logf("writer[%d] kind=%s source=%s target=%s ack=%s elapsed=%s", i, call.Kind, call.Source, call.Date, call.Ack, call.Elapsed)
	}
	t.Logf("PASS headless native Tester + real SerialLane + real Service; dates=365 tasks=762→764 revisions=2→4 native_close=2 reopen_equal=true development_session=%s", path)
}
