// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
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

func openReturnMenu(t *testing.T, tt *ui.Tester, trace core.DayTraceID) {
	t.Helper()
	if err := tt.RightClick("查看任务 " + string(trace)); err != nil {
		t.Fatal(err)
	}
}
func chooseReturn(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if err := tt.ChooseMenuItem(returnToPoolLabel); err != nil {
		t.Fatal(err)
	}
}
func TestReturnMenuNativeCurrentFutureAndReopen(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "today", true: "future"}[future], func(t *testing.T) {
			f := newTitleFixture(t, false)
			date := liveTestDay
			if future {
				date, _ = date.AddDays(1)
			}
			if err := f.svc.AddOnDate("合成回池任务", date); err != nil {
				t.Fatal(err)
			}
			f.h.publish(f.svc.Snapshot(), liveTestDay)
			trace := f.svc.Snapshot().Traces[0]
			f.s.runtimeDate = date
			f.s.runtimeSelection = string(trace.ID)
			f.s.runtimeDayRail = true
			f.tt.Frame()
			f.ui()
			openReturnMenu(t, f.tt, trace.ID)
			want := []string{"标记完成", returnToPoolLabel}
			if future {
				want = want[1:]
			}
			if !reflect.DeepEqual(f.tt.Menu(), want) {
				t.Fatalf("menu %v", f.tt.Menu())
			}
			f.tt.CloseMenu()
			f.all()
			if f.svc.Snapshot().Traces[0].Status != core.TracePending {
				t.Fatal("cancel wrote")
			}
			openReturnMenu(t, f.tt, trace.ID)
			chooseReturn(t, f.tt)
			f.all()
			wantStatus := core.TraceReturnedToPool
			if future {
				wantStatus = core.TraceCancelledDraft
			}
			if f.svc.Snapshot().Traces[0].Status != wantStatus || f.s.runtimeSelection != "" || f.s.runtimeError != "" {
				t.Fatalf("return failed %v %s", f.svc.Snapshot().Traces, f.s.runtimeError)
			}
			if len(rowsFor(f.h.graph, "pool", date, liveTestDay)) != 1 || len(rowsFor(f.h.graph, "day", date, liveTestDay)) != 0 {
				t.Fatal("projection")
			}
			f.svc.Close()
			again, err := taskapp.Open(f.path, liveTestDay)
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			if again.Snapshot().Traces[0].Status != wantStatus {
				t.Fatal("reopen")
			}
		})
	}
}
func TestReturnMenuWaitsForInflightTitleAndDeduplicates(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	f.edit("先保存")
	f.advance(700 * time.Millisecond)
	f.edit("最后标题")
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.ui()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("early return")
	}
	f.all()
	if f.persisted() != "最后标题" || f.svc.Snapshot().Traces[0].Status != core.TraceReturnedToPool || len(f.svc.Snapshot().Definitions) != 2 || f.s.runtimeError != "" {
		t.Fatalf("title/action FIFO %s %s", f.persisted(), f.s.runtimeError)
	}
}
func TestReturnMenuTwoWindowsDeduplicate(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	openReturnMenu(t, tt, trace.ID)
	chooseReturn(t, tt)
	f.all()
	if len(f.svc.Snapshot().Definitions) != 2 || f.s.runtimeError != "" || other.runtimeError != "" {
		t.Fatalf("duplicate return %s %s", f.s.runtimeError, other.runtimeError)
	}
}
func TestReturnMenuLateCompletionPreservesNewNavigation(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.ui()
	// Accepted transaction remains pending. Cancel/reopen the native menu and
	// navigate to a different page before its writer result is delivered.
	openReturnMenu(t, f.tt, trace.ID)
	f.tt.CloseMenu()
	liveClick(t, f.tt, "切换到任务池")
	f.ui()
	f.all()
	if f.s.page != "pool" || f.s.runtimeSelection != "" || f.s.runtimeError != "" {
		t.Fatal("late callback replaced navigation")
	}
}
func TestReturnMenuFailureRetainsSelectionAndRetries(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	titleStoreFault(f.svc, func(at string) error {
		if at == "before-reserve" {
			return errors.New("synthetic disk unavailable")
		}
		return nil
	})
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending || f.s.runtimeSelection != string(trace.ID) || f.s.runtimeError == "" {
		t.Fatal("failed command lost selection")
	}
	titleStoreFault(f.svc, func(string) error { return nil })
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TraceReturnedToPool || f.s.runtimeError != "" {
		t.Fatal("retry failed", f.s.runtimeError)
	}
}
func TestReturnMenuTitleFaultNeverReturns(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			f.edit("不能丢的草稿")
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			openReturnMenu(t, f.tt, trace.ID)
			chooseReturn(t, f.tt)
			f.all()
			if f.svc.Snapshot().Traces[0].Status != core.TracePending || f.s.titleEditor == nil || f.s.titleEditor.native.draft != "不能丢的草稿" || f.s.runtimeSelection != string(trace.ID) || f.s.runtimeError == "" {
				t.Fatal("title fault lost draft/selection or ran command")
			}
		})
	}
}
func TestReturnMenuCrossDayAndStaleTraceRejected(t *testing.T) {
	for _, mode := range []string{"midnight", "stale", "wrong-chain"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			if mode == "midnight" {
				f.clock.now = f.clock.now.Add(24 * time.Hour)
			}
			if mode == "stale" {
				trace.ID = "missing-trace"
			}
			if mode == "wrong-chain" {
				trace.ChainID = "missing-chain"
			}
			f.s.returnTaskToPool(trace.ChainID, trace.ID)
			f.all()
			if f.svc.Snapshot().Traces[0].Status == core.TraceReturnedToPool || len(f.svc.Snapshot().Definitions) != 1 || f.s.runtimeError == "" {
				t.Fatal("invalid action admitted")
			}
		})
	}
}
func TestReturnMenuHeadlessEvidence(t *testing.T) {
	root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if root == "" {
		t.Skip("external evidence directory not configured")
	}
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	save := func(name string) {
		t.Helper()
		out, err := os.Create(filepath.Join(root, "evidence", name))
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		if err = png.Encode(out, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	openReturnMenu(t, f.tt, trace.ID)
	if err := os.WriteFile(filepath.Join(root, "evidence", "return-native-menu.txt"), []byte("Native ContextMenu via real MyGo Tester (system chrome is not rasterized by headless):\n"+f.tt.Menu()[0]+"\n"+f.tt.Menu()[1]+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	save("return-day-before.png")
	chooseReturn(t, f.tt)
	f.all()
	liveClick(t, f.tt, "切换到任务池")
	f.all()
	save("return-pool-after.png")
}

func TestReturnMenuNewSameSelectionIntentSurvives(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.ui()
	// A newer user intent can end with the same page/date/selection. Equality
	// alone cannot give an old completion ownership of that selection.
	f.s.titleTransition(func() { f.s.runtimeSelection = string(trace.ID) })
	f.ui()
	f.all()
	if f.s.runtimeSelection != string(trace.ID) {
		t.Fatal("old completion cleared newer identical selection")
	}
}
func TestReturnMenuNativeBeforeFrameFinalizes(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	liveClick(t, f.tt, "日任务详情标题")
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "最终组合", ui.TextInputRange{Start: 4, End: 4})
	// Direct action entry before the next frame models the native menu delivery
	// boundary; the separate menu tests exercise real secondary clicks/choices.
	f.s.returnTaskToPool(trace.ChainID, trace.ID)
	f.all()
	if f.persisted() != "最终组合" || f.svc.Snapshot().Traces[0].Status != core.TraceReturnedToPool {
		t.Fatalf("native finalization %q %s", f.persisted(), f.s.runtimeError)
	}
}
func TestReturnMenuCommitDoesNotBlockFrameOrPublishEarly(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	titleStoreFault(f.svc, func(at string) error {
		if at == "before-reserve" {
			close(entered)
			<-release
		}
		return nil
	})
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatalf("writer count %d", len(f.io.q))
	}
	go func() { f.io.one(); close(done) }()
	<-entered
	frame := make(chan struct{})
	go func() { f.tt.Frame(); close(frame) }()
	select {
	case <-frame:
	case <-time.After(3 * time.Second):
		close(release)
		<-done
		t.Fatal("frame blocked on return disk lock")
	}
	if f.s.runtime.Snapshot().Traces[0].Status != core.TracePending {
		close(release)
		<-done
		t.Fatal("early graph")
	}
	close(release)
	<-done
	if f.s.runtime.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("publication bypassed FIFO")
	}
	f.all()
	if f.s.runtime.Snapshot().Traces[0].Status != core.TraceReturnedToPool {
		t.Fatal("missing acknowledged publication")
	}
}
func TestReturnMenuOmitsUnsupportedStatuses(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	if err := f.svc.SetCompleted(trace.ID, true); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	f.tt.Frame()
	openReturnMenu(t, f.tt, trace.ID)
	if !reflect.DeepEqual(f.tt.Menu(), []string{"撤销完成"}) {
		t.Fatal("unsupported menu", f.tt.Menu())
	}
	f.tt.CloseMenu()
	tomorrow, _ := liveTestDay.AddDays(1)
	if err := f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), tomorrow)
	f.ui()
	f.s.runtimeDate = liveTestDay
	f.tt.Frame()
	openReturnMenu(t, f.tt, trace.ID)
	if len(f.tt.Menu()) != 0 {
		t.Fatal("history menu", f.tt.Menu())
	}
}

func TestReturnMenuNewOtherTaskMenuSurvivesLateCompletion(t *testing.T) {
	f := newTitleFixture(t, true)
	first := f.svc.Snapshot().Traces[0]
	if err := f.svc.AddToday("第二项"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	second := f.svc.Snapshot().Traces[1]
	openReturnMenu(t, f.tt, first.ID)
	chooseReturn(t, f.tt)
	f.ui()
	openReturnMenu(t, f.tt, second.ID)
	f.tt.CloseMenu()
	openReturnMenu(t, f.tt, second.ID)
	f.all()
	if !reflect.DeepEqual(f.tt.Menu(), []string{"标记完成", returnToPoolLabel}) {
		t.Fatal("newer menu lost", f.tt.Menu())
	}
	chooseReturn(t, f.tt)
	f.all()
	if len(rowsFor(f.h.graph, "pool", liveTestDay, liveTestDay)) != 2 {
		t.Fatal("new menu target replaced")
	}
}
func TestReturnMenuCommandDurableFaultMatrix(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			openReturnMenu(t, f.tt, trace.ID)
			chooseReturn(t, f.tt)
			f.all()
			if f.svc.Snapshot().Traces[0].Status != core.TracePending || f.s.runtime.Snapshot().Traces[0].Status != core.TracePending || f.s.runtimeSelection != string(trace.ID) || f.s.runtimeError == "" || !f.h.stopped {
				t.Fatal("unconfirmed return published or selection lost")
			}
		})
	}
}

func TestReturnMenuOpenedBeforeNavigationCannotActOnReentry(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
	f.ui()
	f.tt.Frame()
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
	f.ui()
	f.tt.Frame()
	// A platform menu may remain visible across programmatic navigation. Deliver
	// its retained callback anyway; the retired host identity must reject it.
	if len(f.tt.Menu()) > 0 {
		_ = f.tt.ChooseMenuItem(returnToPoolLabel)
	}
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("stale menu admitted after reentry")
	}
	openReturnMenu(t, f.tt, trace.ID)
	chooseReturn(t, f.tt)
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TraceReturnedToPool {
		t.Fatal("fresh menu failed")
	}
}

func TestReturnMenuAndCheckboxShareCompletionCapability(t *testing.T) {
	f := newTitleFixture(t, true)
	g := f.svc.Snapshot()
	tr := g.Traces[0]
	id, _ := core.NewIdentity()
	lineage, _ := core.NewIdentity()
	g.Subtasks = []core.Subtask{{ID: core.SubtaskID(id), LineageID: core.SubtaskLineageID(lineage), TraceID: tr.ID, Title: "待完成子任务", Status: core.SubtaskPending, Difficulty: core.DifficultyMedium, Position: 1, CreatedAt: tr.CreatedAt, UpdatedAt: tr.CreatedAt}}
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatal(err)
	}
	f.h.publish(g, liveTestDay)
	f.ui()
	f.tt.Frame()
	control, ok := f.tt.Find("切换完成 " + string(tr.ID))
	if !ok {
		t.Fatal("missing completion checkbox")
	}
	f.tt.ClickAt(control.X+control.W/2, control.Y+control.H/2)
	f.ui()
	if len(f.io.q) != 0 {
		t.Fatal("disabled checkbox enqueued command")
	}
	openReturnMenu(t, f.tt, tr.ID)
	if !reflect.DeepEqual(f.tt.Menu(), []string{returnToPoolLabel}) {
		t.Fatal("menu disagrees", f.tt.Menu())
	}
}
