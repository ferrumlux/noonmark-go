// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

type titleTestQueue struct{ q []func() }

func (q *titleTestQueue) Enqueue(f func()) { q.q = append(q.q, f) }
func (q *titleTestQueue) one()             { f := q.q[0]; q.q = q.q[1:]; f() }

type titleTestTimer struct {
	at     time.Time
	f      func()
	cancel bool
}
type titleTestClock struct {
	now    time.Time
	timers []*titleTestTimer
}

func (c *titleTestClock) Now() time.Time { return c.now }
func (c *titleTestClock) After(d time.Duration, f func()) func() {
	x := &titleTestTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, x)
	return func() { x.cancel = true }
}
func (c *titleTestClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
	old := c.timers
	c.timers = nil
	for _, x := range old {
		if x.at.After(c.now) {
			c.timers = append(c.timers, x)
		} else if !x.cancel {
			x.f()
		}
	}
}

type titleFixture struct {
	t            *testing.T
	s            *app
	tt           *ui.Tester
	svc          *taskapp.Service
	path         string
	h            *titleHub
	posts, io    titleTestQueue
	clock        titleTestClock
	invalid      bool
	closed, quit int
}

func newTitleFixture(t *testing.T, seed bool) *titleFixture {
	f := &titleFixture{t: t}
	f.clock.now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f.path = filepath.Join(t.TempDir(), "session")
	var err error
	f.svc, err = taskapp.Create(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.svc.Close() })
	if seed {
		if err = f.svc.AddToday("原题"); err != nil {
			t.Fatal(err)
		}
	}
	f.h = newTestTitleHub(f.svc, &f.io, f.posts.Enqueue, &f.clock, f.clock.Now, func() {
		if f.h.beforeQuit() {
			f.quit++
		}
	})
	f.s = newRuntimeApp(f.h.readView())
	f.h.attach(f.s, func() { f.invalid = true }, func() {
		if f.h.beforeClose(f.s) {
			f.closed++
		}
	})
	if seed {
		f.h.publish(f.svc.Snapshot(), f.svc.Today())
		f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
		f.s.runtimeDayRail = true
	}
	f.tt = ui.NewTester(f.s.view, 1200, 768)
	f.ui()
	return f
}
func (f *titleFixture) ui() {
	f.t.Helper()
	for i := 0; i < 100; i++ {
		if len(f.posts.q) > 0 {
			f.posts.one()
			continue
		}
		if f.invalid {
			f.invalid = false
			f.tt.Frame()
			continue
		}
		return
	}
	f.t.Fatal("UI self-triggered loop")
}
func (f *titleFixture) all() {
	f.t.Helper()
	for i := 0; i < 100; i++ {
		f.ui()
		if len(f.io.q) == 0 {
			return
		}
		f.io.one()
	}
	f.t.Fatal("writer did not settle")
}
func (f *titleFixture) edit(text string) {
	f.t.Helper()
	liveClick(f.t, f.tt, "日任务详情标题")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type(text)
	f.ui()
}
func (f *titleFixture) persisted() string {
	g := f.svc.Snapshot()
	if len(g.Traces) == 0 {
		return ""
	}
	r := g.Traces[0]
	return currentTitle(g, r.ChainID, r.DefinitionID)
}
func (f *titleFixture) advance(d time.Duration) { f.clock.advance(d); f.ui() }

// Test-only private host access injects the same direct platform client calls
// used by real native services, without Tester.Type's automatic frame. No
// production code imports unsafe or retains an Element; no OS IME is simulated.
type deliveredNative interface {
	ReplaceText(*ui.TextInputRange, string)
	SetMarkedText(*ui.TextInputRange, string, ui.TextInputRange)
}

func titleDirectClient(t *testing.T, tt *ui.Tester) deliveredNative {
	t.Helper()
	field := reflect.ValueOf(tt).Elem().FieldByName("h").Elem().FieldByName("ime").FieldByName("Client")
	value := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	client, ok := value.(deliveredNative)
	if !ok || client == nil {
		t.Fatal("native client not active")
	}
	return client
}
func TestTitleUIDebounceReturnReopen(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("新标题")
	f.advance(699 * time.Millisecond)
	if len(f.io.q) != 0 || f.persisted() != "原题" {
		t.Fatal("early save")
	}
	f.advance(time.Millisecond)
	f.all()
	if f.persisted() != "新标题" {
		t.Fatal("not saved", f.s.runtimeError)
	}
	liveClick(t, f.tt, "日任务详情标题")
	f.tt.Key(0, ui.KeyEnd)
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Type("第二行")
	f.ui()
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "新标题\n第二行" {
		t.Fatal("Return submitted instead of newline", f.persisted())
	}
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	g := again.Snapshot()
	if currentTitle(g, g.Traces[0].ChainID, g.Traces[0].DefinitionID) != f.persisted() {
		t.Fatal("reopen mismatch")
	}
}
func TestTitleUICompositionNoIntermediateSave(t *testing.T) {
	f := newTitleFixture(t, true)
	liveClick(t, f.tt, "日任务详情标题")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Compose("未定", 2)
	f.ui()
	f.advance(time.Second)
	f.all()
	if f.persisted() != "原题" {
		t.Fatal("saved preedit")
	}
	f.tt.Type("确定")
	f.ui()
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "确定" {
		t.Fatal("commit missing", f.persisted())
	}
}
func TestTitleUINewerInputDuringSave(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("先保存")
	f.advance(700 * time.Millisecond)
	if len(f.io.q) != 1 {
		t.Fatal("missing save")
	}
	f.edit("后输入")
	f.io.one()
	f.ui()
	if f.s.titleEditor.native.draft != "后输入" {
		t.Fatal("old completion overwrote input")
	}
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "后输入" {
		t.Fatal("latest input not saved")
	}
}
func TestTitleUINavigationFlush(t *testing.T) {
	for _, label := range []string{"后一天", "收起日待办详情", "关闭任务详情", "切换到任务池", "从任务池排期"} {
		t.Run(label, func(t *testing.T) {
			f := newTitleFixture(t, true)
			f.edit("离开前标题")
			liveClick(t, f.tt, label)
			f.ui()
			if f.s.titleEditor == nil {
				t.Fatal("removed before save")
			}
			f.all()
			if f.persisted() != "离开前标题" || f.s.titleEditor != nil {
				t.Fatal("navigation did not flush/dispose", f.s.runtimeError)
			}
		})
	}
}
func TestTitleUICompletionOrderedAfterTitle(t *testing.T) {
	f := newTitleFixture(t, true)
	id := f.svc.Snapshot().Traces[0].ID
	f.edit("完成前标题")
	liveClick(t, f.tt, "切换完成 "+string(id))
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatal("title not first")
	}
	f.io.one()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("completion overtook title")
	}
	f.all()
	if f.persisted() != "完成前标题" || f.svc.Snapshot().Traces[0].Status != core.TraceCompleted {
		t.Fatal("completion failed")
	}
}
func TestTitleUIBlankBlurAndLifecycle(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit(" ")
	f.advance(time.Second)
	f.all()
	if f.s.titleEditor.native.draft != " " || f.persisted() != "原题" {
		t.Fatal("blank debounce discarded")
	}
	if !f.h.beforeClose(f.s) {
		f.all()
	}
	if f.closed == 0 && len(f.h.windows) != 0 {
		t.Fatal("blank close failed")
	}
	if f.persisted() != "原题" {
		t.Fatal("blank persisted")
	}
}
func TestTitleUICloseBeforeFrameAndInitialRegistration(t *testing.T) {
	for _, marked := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "marked-selection"}[marked], func(t *testing.T) {
			f := newTitleFixture(t, true)
			liveClick(t, f.tt, "日任务详情标题")
			client := titleDirectClient(t, f.tt)
			r := ui.TextInputRange{Start: 0, End: 2}
			if marked {
				client.SetMarkedText(&r, "帧前组合", ui.TextInputRange{Start: 4, End: 4})
			} else {
				client.ReplaceText(&r, "帧前提交")
			}
			if f.h.beforeClose(f.s) {
				t.Fatal("allowed before durable")
			}
			f.all()
			want := "帧前提交"
			if marked {
				want = "帧前组合"
			}
			if f.persisted() != want || f.closed != 1 {
				t.Fatal("lost frame-gap input", f.persisted(), f.closed)
			}
		})
	}
	f := newTitleFixture(t, false)
	if err := f.svc.AddToday("初始"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), f.svc.Today())
	f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
	f.s.runtimeDayRail = true
	f.tt.Frame()
	if f.s.titleEditor == nil || f.s.titleEditor.registered {
		t.Fatal("not in registration gap")
	}
	if !f.h.beforeClose(f.s) {
		t.Fatal("pending registration not finalized")
	}
	f.ui()
}
func TestTitleUIReplaceRejectsLateNative(t *testing.T) {
	f := newTitleFixture(t, true)
	liveClick(t, f.tt, "日任务详情标题")
	n := &f.s.titleEditor.native
	if _, ok := n.Snapshot(); !ok {
		t.Fatal("snapshot")
	}
	client := titleDirectClient(t, f.tt)
	client.ReplaceText(nil, "晚字")
	if n.Replace("旧确认") {
		t.Fatal("overwrote late input")
	}
	v, ok := n.Snapshot()
	if !ok || v.Text == "旧确认" || v.Text == "原题" {
		t.Fatal("native input lost", v)
	}
}
func TestTitleUIFailedSaveCancelsClose(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("不能丢")
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("premature close")
	}
	f.all()
	if f.closed != 0 || f.s.titleEditor == nil || f.s.titleEditor.native.draft != "不能丢" || f.s.runtimeError == "" {
		t.Fatal("failure lost draft")
	}
	f.advance(time.Hour)
	f.all()
	if len(f.io.q) != 0 {
		t.Fatal("permanent retry")
	}
}
func TestTitleUIIdleFramesDoNotPostOrWrite(t *testing.T) {
	f := newTitleFixture(t, true)
	for range 30 {
		f.tt.Frame()
	}
	if len(f.posts.q) != 0 || len(f.io.q) != 0 {
		t.Fatal("idle frame queued work", len(f.posts.q), len(f.io.q))
	}
}
func TestTitleUINaturalDayRetainsOldDraft(t *testing.T) {
	f := newTitleFixture(t, true)
	id := f.svc.Snapshot().Traces[0].ID
	f.edit("午夜草稿")
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.tt.Frame()
	f.all()
	if f.s.titleEditor == nil || f.s.titleEditor.native.draft != "午夜草稿" || f.s.titleEditor.row.TraceID != id {
		t.Fatal("midnight migrated/lost draft")
	}
	if f.s.titleEditor.row.Status != core.TraceUnfinished || !f.tt.HasText("未完成") {
		t.Fatal("retained detail has stale status", f.s.titleEditor.row.Status)
	}
	if f.persisted() != "原题" {
		t.Fatal("backfilled history")
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("allowed lost history draft")
	}
}
func TestTitleUIQuickAddCloseAndDuplicate(t *testing.T) {
	f := newTitleFixture(t, false)
	liveType(t, f.tt, "新增一次")
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	if f.h.beforeClose(f.s) {
		t.Fatal("pending command allowed close")
	}
	f.all()
	if f.closed != 1 || len(f.svc.Snapshot().Chains) != 1 {
		t.Fatal("duplicate or lost add", f.closed, len(f.svc.Snapshot().Chains))
	}
}
func TestTitleUIQuickAddFailureRetainsDraft(t *testing.T) {
	f := newTitleFixture(t, false)
	liveType(t, f.tt, "失败草稿")
	f.tt.Key(0, ui.KeyEnter)
	if f.h.beforeClose(f.s) {
		t.Fatal("early close")
	}
	f.svc.Close()
	f.all()
	if f.closed != 0 || f.s.draft != "失败草稿" || f.s.runtimeError == "" {
		t.Fatal("failed command lost draft")
	}
}
func TestTitleUIQuickAddPreservesNewerDraftAndDate(t *testing.T) {
	f := newTitleFixture(t, false)
	f.s.runtimeDate = core.LocalDate{Year: 2026, Month: 11, Day: 6}
	f.tt.Frame()
	liveType(t, f.tt, "明日捕获")
	f.tt.Key(0, ui.KeyEnter)
	f.s.runtimeDate = liveTestDay
	f.s.draft = "后写草稿"
	f.all()
	if f.s.draft != "后写草稿" || f.svc.Snapshot().Traces[0].Date.Day != 6 {
		t.Fatal("closure reread mutable state")
	}
}
func TestTitleUIMultipleWindowsQuitAndSync(t *testing.T) {
	f := newTitleFixture(t, true)
	other := newRuntimeApp(f.h.readView())
	other.runtimeSelection = f.s.runtimeSelection
	other.runtimeDayRail = true
	closed := false
	f.h.attach(other, func() {}, func() { closed = f.h.beforeClose(other) })
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	f.edit("共享标题")
	f.advance(700 * time.Millisecond)
	f.all()
	tt.Frame()
	if other.titleEditor.native.draft != "共享标题" {
		t.Fatal("clean window not synchronized")
	}
	liveClick(t, tt, "日任务详情标题")
	titleDirectClient(t, tt).ReplaceText(nil, "晚")
	if f.h.beforeQuit() {
		t.Fatal("quit missed another native source")
	}
	f.all()
	if f.quit != 0 || closed || f.h.quitting {
		t.Fatal("cancelled quit retried after flush", f.quit)
	}
	if !f.h.beforeQuit() {
		t.Fatal("fresh explicit quit failed after actual flush")
	}
	if !f.h.beforeClose(other) {
		t.Fatal("old quit blocked fresh explicit close")
	}
}
func TestTitleUIVisualEvidence(t *testing.T) {
	f := newTitleFixture(t, true)
	f.tt.SetScale(2)
	f.edit("合成：标题第一行\n第二行，保留原生换行\n第三行\n第四行\n第五行")
	b, ok := f.tt.Find("日任务详情标题")
	if !ok || b.H < 22 || b.H > 72 {
		t.Fatal("title height outside source bounds", b)
	}
	save := func(name string) {
		dir := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
		if dir == "" {
			return
		}
		im := f.tt.Image()
		if im.Bounds().Dx() != 2400 || im.Bounds().Dy() != 1536 {
			t.Fatal("evidence is not native 2x", im.Bounds())
		}
		out, err := os.Create(filepath.Join(dir, "evidence", name))
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		if err = png.Encode(out, im); err != nil {
			t.Fatal(err)
		}
	}
	save("title-native-ui.png")
	f.edit("合成：标题会自动保存")
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "合成：标题会自动保存" {
		t.Fatal("short title evidence is not saved")
	}
	save("title-native-ui-saved.png")
}

func TestTitleUIOrdinaryBlankAfterInFlightNeedsLifecycle(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("已排队")
	f.advance(700 * time.Millisecond)
	f.edit(" ")
	liveClick(t, f.tt, "开发任务标题")
	f.ui()
	f.all()
	e := f.s.titleEditor
	if e.native.draft != " " || !e.state.Dirty || f.persisted() != "已排队" {
		t.Fatal("ordinary blur lost source blank boundary", e.native.draft, e.state.Dirty, f.persisted())
	}
	if !f.h.beforeClose(f.s) {
		f.all()
	}
	if f.persisted() != "已排队" || len(f.h.windows) != 0 {
		t.Fatal("lifecycle did not restore last ack")
	}
}
func TestTitleUILatestNavigationWins(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("先保存再导航")
	liveClick(t, f.tt, "后一天")
	f.ui()
	liveClick(t, f.tt, "切换到任务池")
	f.ui()
	f.all()
	if f.s.page != "pool" || f.s.runtimeDate != liveTestDay || f.persisted() != "先保存再导航" {
		t.Fatal("old target overwrote latest")
	}
}
func TestTitleUIRepeatedCloseAndLateInput(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("A")
	for range 3 {
		if f.h.beforeClose(f.s) {
			t.Fatal("closed dirty")
		}
	}
	liveClick(t, f.tt, "日任务详情标题")
	titleDirectClient(t, f.tt).ReplaceText(&ui.TextInputRange{Start: 1, End: 1}, "B")
	f.all()
	if f.closed != 1 || f.persisted() != "AB" {
		t.Fatalf("repeated close lost latest closed=%d persisted=%q state=%+v error=%s io=%d posts=%d", f.closed, f.persisted(), f.s.titleEditor.state, f.s.runtimeError, len(f.io.q), len(f.posts.q))
	}
}
func TestTitleUIFinishFailureKeepsTransition(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("保留")
	e := f.s.titleEditor
	handle := e.native.handle
	e.native.handle = ui.Handle{}
	if f.h.beforeClose(f.s) {
		t.Fatal("missing native allowed close")
	}
	f.s.titleTransition(func() { f.s.page = "pool" })
	f.ui()
	if f.s.page != "day" || f.s.titleEditor != e || e.native.draft != "保留" {
		t.Fatal("failed finish removed editor")
	}
	e.native.handle = handle
}
func TestTitleUIStaleGenerationEvent(t *testing.T) {
	f := newTitleFixture(t, true)
	old := f.s.titleEditor
	oldID := old.identity
	f.edit(" ")
	liveClick(t, f.tt, "开发任务标题")
	f.ui()
	if old.identity == oldID {
		t.Fatal("discard did not advance generation")
	}
	if err := f.h.coordinator.Observe(old.reg, oldID); err == nil {
		t.Fatal("stale identity accepted")
	}
	if old.native.draft != "原题" {
		t.Fatal("stale event changed text")
	}
}
func TestTitleUICleanNaturalDayBecomesReadonly(t *testing.T) {
	f := newTitleFixture(t, true)
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.tt.Frame()
	f.all()
	if f.s.titleEditor != nil || f.persisted() != "原题" {
		t.Fatal("clean history did not retire")
	}
	if !f.h.beforeClose(f.s) {
		t.Fatal("clean history blocked close")
	}
}
func TestTitleUIExternalPublishProtectsUnboundNative(t *testing.T) {
	f := newTitleFixture(t, true)
	liveClick(t, f.tt, "日任务详情标题")
	titleDirectClient(t, f.tt).ReplaceText(nil, "尚未建帧")
	g := f.svc.Snapshot()
	if _, err := f.svc.SaveTraceTitleInputAcknowledged(g.Traces[0].ID, "外部确认"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	if f.s.titleEditor.state.Draft != "原题尚未建帧" {
		t.Fatal("publication overwrote unbound input", f.s.titleEditor.native.draft)
	}
}
func TestTitleUIMultiwindowQuitFailureKeepsEveryWindow(t *testing.T) {
	f := newTitleFixture(t, true)
	other := newRuntimeApp(f.h.readView())
	other.runtimeSelection = f.s.runtimeSelection
	other.runtimeDayRail = true
	f.h.attach(other, func() {}, func() { t.Fatal("quit prematurely closed a window") })
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	f.edit("窗口一")
	liveClick(t, tt, "日任务详情标题")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("窗口二")
	f.ui()
	f.svc.Close()
	if f.h.beforeQuit() {
		t.Fatal("dirty quit allowed")
	}
	f.all()
	if f.quit != 0 || len(f.h.windows) != 2 || f.s.titleEditor == nil || other.titleEditor == nil {
		t.Fatal("quit failure destroyed window")
	}
}
func TestTitleUIConflictStopsTitleAndOtherCommands(t *testing.T) {
	f := newTitleFixture(t, true)
	other, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err = other.AddToday("竞争进程"); err != nil {
		t.Fatal(err)
	}
	f.edit("冲突草稿")
	f.advance(700 * time.Millisecond)
	f.all()
	if !f.h.stopped || f.s.titleEditor.native.draft != "冲突草稿" {
		t.Fatal("conflict not retained/stopped")
	}
	liveType(t, f.tt, "不得继续写入")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if f.s.draft != "不得继续写入" || len(f.svc.Snapshot().Chains) != 1 {
		t.Fatal("command ignored stopped service")
	}
}

// Only synthetic test stores expose their existing fault seam through reflection;
// there is no exported production fault switch or persistence bypass.
func titleStoreFault(s *taskapp.Service, fault func(string) error) {
	field := reflect.ValueOf(s).Elem().FieldByName("disk").Elem().FieldByName("fault")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(fault))
}
func TestTitleUIDurableFaultMatrixRetainsDraft(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			f.edit("待核实草稿")
			if f.h.beforeClose(f.s) {
				t.Fatal("premature close")
			}
			f.all()
			if f.closed != 0 || f.s.titleEditor == nil || f.s.titleEditor.state.LastAcknowledgedText != "原题" || f.s.titleEditor.native.draft != "待核实草稿" || f.persisted() != "原题" {
				t.Fatal("uncertain write acknowledged or draft lost")
			}
			f.svc.Close()
			again, err := taskapp.Open(f.path, liveTestDay)
			switch phase {
			case "before-reserve", "after-persist":
				if err != nil {
					t.Fatal(err)
				}
				defer again.Close()
				g := again.Snapshot()
				actual := currentTitle(g, g.Traces[0].ChainID, g.Traces[0].DefinitionID)
				want := "原题"
				if phase == "after-persist" {
					want = "待核实草稿"
				}
				if actual != want {
					t.Fatal("incorrect recovered bytes", actual)
				}
			default:
				if !errors.Is(err, taskapp.ErrIncomplete) {
					if again != nil {
						again.Close()
					}
					t.Fatal("partial commit reopened", err)
				}
			}
		})
	}
}
func TestTitleUITemporaryFaultRetriesWithoutLosingDraft(t *testing.T) {
	f := newTitleFixture(t, true)
	calls := 0
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			calls++
			if calls == 1 {
				return errors.New("synthetic temporary IO")
			}
		}
		return nil
	})
	f.edit("可重试")
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "原题" || f.s.titleEditor.native.draft != "可重试" {
		t.Fatal("failed draft not retained")
	}
	f.advance(10 * time.Second)
	f.all()
	if f.persisted() != "可重试" || f.s.titleEditor.state.Dirty || f.s.runtimeError != "" {
		t.Fatal("scheduled retry failed")
	}
}

func TestTitleUIReadsStayResponsiveWhileServiceCommitHoldsLock(t *testing.T) {
	f := newTitleFixture(t, true)
	entered, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			close(entered)
			<-release
		}
		return nil
	})
	f.edit("后台落盘")
	f.advance(700 * time.Millisecond)
	go func() { f.io.one(); close(writerDone) }()
	<-entered // Service.mu is held across this real disk transaction seam.
	uiDone := make(chan struct{})
	var projected string
	var date core.LocalDate
	go func() {
		g := f.s.runtime.Snapshot()
		date = f.s.runtime.Today()
		r := g.Traces[0]
		projected = currentTitle(g, r.ChainID, r.DefinitionID)
		f.tt.Frame()
		other := newRuntimeApp(f.h.readView())
		f.h.attach(other, func() {}, func() {})
		_ = ui.NewTester(other.view, 1200, 768)
		g.Definitions[0].Title = "修改副本不会污染读视图"
		close(uiDone)
	}()
	select {
	case <-uiDone:
	case <-time.After(3 * time.Second):
		close(release)
		<-writerDone
		t.Fatal("UI read/frame/new-window blocked behind Service disk lock")
	}
	if projected != "原题" || date != liveTestDay {
		close(release)
		<-writerDone
		t.Fatal("unconfirmed graph leaked")
	}
	close(release)
	<-writerDone
	// Storage is durable, but the UI publication queue has not been delivered.
	g := f.s.runtime.Snapshot()
	if currentTitle(g, g.Traces[0].ChainID, g.Traces[0].DefinitionID) != "原题" {
		t.Fatal("read view advanced without ordered publication")
	}
	f.all()
	g = f.s.runtime.Snapshot()
	if currentTitle(g, g.Traces[0].ChainID, g.Traces[0].DefinitionID) != "后台落盘" {
		t.Fatal("verified publication not applied")
	}
}

// Existing deterministic fixtures keep their explicit queues. New admission tests
// inject rejection into the classified hooks rather than pretending every try works.
func newTestTitleHub(store titleStore, lane interface{ Enqueue(func()) }, post func(func()), clock coordinator.Clock, now func() time.Time, quit func()) *titleHub {
	return newTitleHub(store, func(work func(func(func()) bool)) bool {
		lane.Enqueue(func() { work(func(fn func()) bool { post(fn); return true }) })
		return true
	}, func(fn func()) bool { post(fn); return true }, clock, now, quit)
}
