// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"math"
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

func progressFixtureUI(t *testing.T) (*titleFixture, core.DayTrace, core.Subtask, core.Subtask) {
	t.Helper()
	f, tr, easy := subtaskFixture(t)
	subtaskAddUI(t, f, "day", string(tr.ID), "困难子任务")
	hard := f.svc.Snapshot().Subtasks[1]
	liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".difficulty")
	if err := f.tt.ChooseMenuItem("困难"); err != nil {
		t.Fatal(err)
	}
	f.all()
	liveClick(t, f.tt, subtaskPrefix("day", string(easy.ID))+".completion")
	f.all()
	return f, tr, easy, subtaskPersisted(t, f, hard.ID)
}
func wantUIProgress(t *testing.T, s *app, id core.DayTraceID, percent, completed, total int) core.TraceProgress {
	t.Helper()
	p, ok := s.traceProgress(id)
	if !ok || p.Percent != percent || p.CompletedWeight != completed || p.TotalWeight != total {
		t.Fatalf("UI progress %+v, ok=%v; want %d%% %d/%d", p, ok, percent, completed, total)
	}
	return p
}
func progressAccess(t *testing.T, tt *ui.Tester, mode core.TraceProgressMode, percent int) ui.AccessInfo {
	t.Helper()
	label := manualProgressLabel
	if mode == core.ProgressWeightedSubtasks {
		label = weightedProgressLabel
	}
	a, ok := tt.AccessByRole(ui.RoleProgress, label)
	if !ok || a.Value != fmt.Sprintf("%d%%", percent) || a.Min != 0 || a.Max != 100 || a.Now != float64(percent) || a.Focusable || a.Step != 0 {
		t.Fatalf("read-only accessible progress %+v found=%v", a, ok)
	}
	if _, ok := tt.AccessByRole(ui.RoleSlider, ""); ok {
		t.Fatal("invented writable slider")
	}
	return a
}
func TestProgressUIWeightedCommandsAndExplicitParent(t *testing.T) {
	f, tr, easy, hard := progressFixtureUI(t)
	p := wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	if p.Mode != core.ProgressWeightedSubtasks {
		t.Fatal(p)
	}
	progressAccess(t, f.tt, p.Mode, 25)
	if f.h.progress.projection.SubtaskProgress(tr.ID).Completed != 1 {
		t.Fatal("counts")
	}
	liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".difficulty")
	if err := f.tt.ChooseMenuItem("中等"); err != nil {
		t.Fatal(err)
	}
	f.all()
	wantUIProgress(t, f.s, tr.ID, 33, 1, 3)
	liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".completion")
	f.all()
	wantUIProgress(t, f.s, tr.ID, 100, 3, 3)
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("100 percent auto-completed parent")
	}
	liveClick(t, f.tt, "切换完成 "+string(tr.ID))
	f.all()
	p = wantUIProgress(t, f.s, tr.ID, 100, 1, 1)
	if p.Mode != core.ProgressManual {
		t.Fatal("completed parent must be manual", p)
	}
	progressAccess(t, f.tt, p.Mode, 100)
	liveClick(t, f.tt, "切换完成 "+string(tr.ID))
	f.all()
	wantUIProgress(t, f.s, tr.ID, 100, 3, 3)
	liveClick(t, f.tt, subtaskPrefix("day", string(easy.ID))+".completion")
	f.all()
	wantUIProgress(t, f.s, tr.ID, 67, 2, 3)
	liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".delete")
	f.all()
	wantUIProgress(t, f.s, tr.ID, 0, 0, 1)
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 0)
}
func TestProgressUIReturnLineageReopenAndNoPoolProgress(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	f.s.returnTaskToPool(tr.ChainID, tr.ID)
	f.all()
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(tr.ChainID) })
	f.all()
	if _, ok := f.tt.AccessByRole(ui.RoleProgress, ""); ok {
		t.Fatal("Pool invented progress")
	}
	liveClick(t, f.tt, "排到今天 "+string(tr.ChainID))
	f.all()
	g := f.svc.Snapshot()
	newTr := g.Traces[len(g.Traces)-1]
	if newTr.ID == tr.ID {
		t.Fatal("not new trace")
	}
	p := wantUIProgress(t, f.s, newTr.ID, 25, 1, 4)
	counts := f.s.progress.projection.SubtaskProgress(newTr.ID)
	if counts.Completed != 0 || counts.Total != 1 {
		t.Fatal("fixture must distinguish lineage from current counts", counts)
	}
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(newTr.ID); f.s.runtimeDayRail = true })
	f.all()
	progressAccess(t, f.tt, p.Mode, 25)
	before := f.s.progress.projection
	for i := 0; i < 40; i++ {
		f.tt.Frame()
		f.ui()
	}
	if f.s.progress.projection != before {
		t.Fatal("per-frame rebuild")
	}
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s := newRuntimeApp(reopened)
	s.runtimeSelection = string(newTr.ID)
	s.runtimeDayRail = true
	tt := ui.NewTester(s.view, 1200, 768)
	wantUIProgress(t, s, newTr.ID, 25, 1, 4)
	progressAccess(t, tt, p.Mode, 25)
	if !reflect.DeepEqual(g, reopened.Snapshot()) {
		t.Fatal("reopen facts changed")
	}
}
func TestProgressUICacheTwoWindowsAndFIFOPublication(t *testing.T) {
	f, tr, _, hard := progressFixtureUI(t)
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	original := f.h.progress.projection
	if other.progress != f.s.progress || other.progress != f.h.progress {
		t.Fatal("per-window cache")
	}
	f.h.command(f.s, func() error { return f.svc.SetSubtaskDifficulty(tr.ChainID, tr.ID, hard.ID, core.DifficultyMedium) }, false, nil)
	f.h.command(other, func() error { return f.svc.SetSubtaskCompleted(tr.ChainID, tr.ID, hard.ID, true) }, false, nil)
	f.ui()
	if len(f.io.q) != 2 {
		t.Fatal("missing writes")
	}
	f.io.one()
	f.io.one()
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	wantUIProgress(t, other, tr.ID, 25, 1, 4)
	if original != f.h.progress.projection {
		t.Fatal("published before UI FIFO")
	}
	f.posts.one() // first confirmed graph publication
	wantUIProgress(t, f.s, tr.ID, 33, 1, 3)
	wantUIProgress(t, other, tr.ID, 33, 1, 3)
	first := f.h.progress.projection
	if first == original {
		t.Fatal("changed graph not invalidated")
	}
	f.posts.one() // completion
	if first != f.h.progress.projection {
		t.Fatal("completion rebuilt")
	}
	f.posts.one() // second confirmed graph publication
	wantUIProgress(t, f.s, tr.ID, 100, 3, 3)
	wantUIProgress(t, other, tr.ID, 100, 3, 3)
	if first == f.h.progress.projection {
		t.Fatal("second graph not invalidated")
	}
	f.all()
	tt.Frame()
	f.ui()
	progressAccess(t, tt, core.ProgressWeightedSubtasks, 100)
	last := f.h.progress.projection
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	for i := 0; i < 30; i++ {
		f.tt.Frame()
		tt.Frame()
		f.ui()
	}
	if last != f.h.progress.projection {
		t.Fatal("unchanged publication or frame rebuilt")
	}
	if allocs := testing.AllocsPerRun(1000, func() { f.s.traceProgress(tr.ID); other.traceProgress(tr.ID) }); allocs != 0 {
		t.Fatal("row query allocations", allocs)
	}
}
func TestProgressUIDurableFailureNeverPublishesCandidate(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f, tr, _, hard := progressFixtureUI(t)
			previous := f.h.progress.projection
			graph := f.h.graph.Clone()
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".completion")
			f.ui()
			wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
			f.all()
			if f.h.progress.projection != previous || !reflect.DeepEqual(graph, f.h.graph) || f.s.runtimeError == "" {
				t.Fatal("failed candidate published or cache rebuilt")
			}
			wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
			progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 25)
			f.svc.Close()
			again, err := taskapp.Open(f.path, liveTestDay)
			if phase == "before-reserve" || phase == "after-persist" {
				if err != nil {
					t.Fatal(err)
				}
				defer again.Close()
				s := newRuntimeApp(again)
				s.runtimeSelection = string(tr.ID)
				s.runtimeDayRail = true
				tt := ui.NewTester(s.view, 1200, 768)
				if phase == "after-persist" {
					wantUIProgress(t, s, tr.ID, 100, 4, 4)
					progressAccess(t, tt, core.ProgressWeightedSubtasks, 100)
				} else {
					wantUIProgress(t, s, tr.ID, 25, 1, 4)
				}
			} else if err == nil {
				again.Close()
				t.Fatal("incomplete commit opened")
			}
		})
	}
}
func TestProgressUICacheRejectsMalformedAndOwnsSnapshot(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	previous := f.h.progress.projection
	graph := f.h.graph.Clone()
	bad := graph.Clone()
	bad.Subtasks[0].Difficulty = core.Difficulty(99)
	f.h.publish(bad, liveTestDay)
	f.ui()
	if f.h.progress.err == nil || !reflect.DeepEqual(f.h.graph, graph) || f.h.progress.projection != previous {
		t.Fatal("invalid graph published")
	}
	if _, ok := f.s.traceProgress(tr.ID); ok {
		t.Fatal("stale percentage displayed for rejected input")
	}
	if _, ok := f.tt.AccessByRole(ui.RoleProgress, ""); ok {
		t.Fatal("invalid graph gave fake percentage")
	}
	if !f.tt.HasText(progressFailureMessage) {
		t.Fatal("missing failure text")
	}
	f.h.publish(graph, liveTestDay)
	f.ui()
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	graph.Subtasks[0].Title = "outside mutation"
	if reflect.DeepEqual(f.h.graph, graph) {
		t.Fatal("published graph aliases input")
	}
	if _, ok := f.s.traceProgress(core.DayTraceID("missing")); ok {
		t.Fatal("missing trace gave 0 percent")
	}
}
func TestProgressUIVisibilityMatrixAndSourceTokens(t *testing.T) {
	for _, status := range []core.TraceStatus{core.TracePending, core.TraceDeferred, core.TraceUnfinished, core.TraceCompleted, core.TraceChanged, core.TraceReturnedToPool, core.TraceAbandoned, core.TraceCancelledDraft} {
		for _, percent := range []int{0, 1, 17, 50, 99, 100} {
			want := (status == core.TracePending || status == core.TraceDeferred || status == core.TraceUnfinished) && percent > 0 && percent < 100
			if showsRowProgress(status, core.TraceProgress{Percent: percent}) != want {
				t.Fatal(status, percent)
			}
		}
	}
	if rowProgressWidth != 150 || rowProgressHeight != 4 || rowProgressGap != 6 || rowProgressTop != 5 || rowProgressFont != 10 || detailProgressHeight != 6 || detailProgressGap != 8 || detailProgressFont != 13 {
		t.Fatal("Swift source tokens drifted")
	}
	if progressColor(99) != sourceVisual.accent || progressColor(100) != sourceVisual.success {
		t.Fatal("source fill colors")
	}
	if weightedProgressLabel != "按子任务自动计算的完成进度" || manualProgressLabel != "手动完成进度" {
		t.Fatal("source accessibility labels")
	}
}
func TestProgressUIConstructorOnlyAtPublishBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "NewProgressProjection" {
					calls++
					if path != "runtime_progress.go" || fn.Name.Name != "publish" {
						t.Fatalf("projection constructed outside publication: %s/%s", path, fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	if calls != 1 {
		t.Fatal("constructor boundary changed", calls)
	}
}
func progressSavePNG(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if root == "" {
		t.Fatal("explicit evidence root required")
	}
	dir := filepath.Join(root, "evidence", "progress-ui")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
func progressPixel(t *testing.T, im *image.RGBA, x, y int, want ui.Color) {
	t.Helper()
	got := im.RGBAAt(x, y)
	if math.Abs(float64(got.R)-float64(want.R)) > 1 || math.Abs(float64(got.G)-float64(want.G)) > 1 || math.Abs(float64(got.B)-float64(want.B)) > 1 {
		t.Fatalf("pixel %d,%d = %v want %v", x, y, got, want)
	}
}

func TestProgressUISourceGeometryAndHeadlessCapture(t *testing.T) {
	f, tr, _, hard := progressFixtureUI(t)
	f.tt.SetScale(2)
	f.all()
	liveClick(t, f.tt, "展开子任务 "+string(tr.ID))
	f.all()
	bar, ok := f.tt.Find(weightedProgressLabel)
	if !ok {
		t.Fatal("row progress absent")
	}
	title, _ := f.tt.Find("原题")
	pct, _ := f.tt.Find("25%")
	if math.Abs(float64(bar.X-title.X)) > .01 || math.Abs(float64(bar.Y-title.Y-title.H-rowProgressTop)) > .01 || math.Abs(float64(pct.X-bar.X-rowProgressWidth-rowProgressGap)) > .01 {
		t.Fatal("row source spacing", bar, title, pct)
	}
	check, _ := f.tt.Find(subtaskPrefix("day-list", string(hard.ID)) + ".completion")
	if check.Y <= bar.Y+bar.H {
		t.Fatal("progress intersects expanded children", bar, check)
	}
	x, y := int(math.Round(float64(bar.X*2))), int(math.Round(float64((bar.Y+(bar.H-rowProgressHeight)/2)*2)))
	progressPixel(t, f.tt.Image(), x+20, y+4, sourceVisual.accent)
	progressPixel(t, f.tt.Image(), x+100, y+4, sourceVisual.chip)
	// Capsule ends must not paint their extreme corner as a rectangle.
	if f.tt.Image().RGBAAt(x, y) == f.tt.Image().RGBAAt(x+4, y+4) {
		t.Fatal("row capsule is square")
	}
	progressSavePNG(t, f.tt, "weighted-list-detail-2x.png")
	f.tt.SetSize(960, 720)
	f.all()
	row, _ := f.tt.Find("查看任务 " + string(tr.ID))
	capsule, _ := f.tt.Find("展开子任务 " + string(tr.ID))
	bar, _ = f.tt.Find(weightedProgressLabel)
	if capsule.X+capsule.W > row.X+row.W || bar.X+bar.W > capsule.X {
		t.Fatal("short title progress/capsule overlap", row, capsule, bar)
	}
	progressSavePNG(t, f.tt, "weighted-minimum-2x.png")
	// Exercise the actual detail renderer separately so both identical labels
	// can be inspected without changing the user-facing accessibility strings.
	detail := ui.NewTester(func(c *ui.Context) { f.s.liveDetailProgress(c, 252, tr.ID) }, 300, 100)
	detail.SetScale(2)
	a := progressAccess(t, detail, core.ProgressWeightedSubtasks, 25)
	p, _ := detail.Find("25%")
	if a.Bounds.W != 252 || math.Abs(float64(p.X+p.W-252)) > .01 || math.Abs(float64(a.Bounds.H-p.H-detailProgressGap-detailProgressHeight)) > .01 {
		t.Fatal("detail source geometry", a.Bounds, p)
	}
	dy := int(math.Round(float64((p.H + detailProgressGap) * 2)))
	progressPixel(t, detail.Image(), 20, dy+6, sourceVisual.accent)
	progressPixel(t, detail.Image(), 200, dy+6, sourceVisual.chip)
	if detail.Image().RGBAAt(0, dy) == detail.Image().RGBAAt(6, dy+6) {
		t.Fatal("detail capsule is square")
	}
	progressSavePNG(t, detail, "weighted-detail-component-2x.png")
}

func progressNewID(t *testing.T) string {
	t.Helper()
	id, err := core.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func progressPriorManual(t *testing.T, g core.TaskGraph, percent int) core.TaskGraph {
	t.Helper()
	g = g.Clone()
	current := &g.Traces[0]
	old := *current
	old.ID = core.DayTraceID(progressNewID(t))
	old.Date, _ = current.Date.AddDays(-1)
	old.Status = core.TraceUnfinished
	old.SettledAt = &old.CreatedAt
	old.ManualProgressPercent = &percent
	old.CompletedAt = nil
	current.CarriedFromTraceID = &old.ID
	current.ContinuationSeq = 1
	day := g.Days[0]
	day.ID = core.DayID(progressNewID(t))
	day.Date = old.Date
	g.Days = append(g.Days, day)
	g.Traces = append(g.Traces, old)
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatal("synthetic historical graph", err)
	}
	return g
}
func TestProgressUIHistoryFloorAndManualReadonly(t *testing.T) {
	for _, weighted := range []bool{false, true} {
		t.Run(fmt.Sprint(weighted), func(t *testing.T) {
			var f *titleFixture
			var tr core.DayTrace
			if weighted {
				f, tr, _, _ = progressFixtureUI(t)
			} else {
				f = newTitleFixture(t, true)
				tr = f.svc.Snapshot().Traces[0]
			}
			g := progressPriorManual(t, f.svc.Snapshot(), 80)
			if !weighted {
				manual := 20
				g.Traces[0].ManualProgressPercent = &manual
			}
			persisted := f.svc.Snapshot()
			// Explicit validated source-shaped history only. No setter/continuation
			// command is invented and this graph is never written to the Service.
			f.h.publish(g, liveTestDay)
			f.ui()
			var p core.TraceProgress
			if weighted {
				p = wantUIProgress(t, f.s, tr.ID, 80, 1, 4)
			} else {
				p = wantUIProgress(t, f.s, tr.ID, 80, 20, 100)
			}
			if p.FloorPercent != 80 {
				t.Fatal("floor lost", p)
			}
			progressAccess(t, f.tt, p.Mode, 80)
			if !f.tt.HasText("分类、完整 Markdown、撤销恢复与手动调整进度尚未接通。") {
				t.Fatal("manual limitation hidden")
			}
			before := f.h.progress.projection
			// Static bar must not respond to click/arrow as an editing control.
			detail := ui.NewTester(func(c *ui.Context) { f.s.liveDetailProgress(c, 252, tr.ID) }, 300, 100)
			detail.ClickAt(200, 30)
			detail.Key(0, ui.KeyRight)
			detail.Key(0, ui.KeyUp)
			f.all()
			if !reflect.DeepEqual(persisted, f.svc.Snapshot()) || before != f.h.progress.projection {
				t.Fatal("readonly progress mutated facts")
			}
			f.tt.SetScale(2)
			progressSavePNG(t, f.tt, fmt.Sprintf("synthetic-history-floor-weighted-%t-2x.png", weighted))
		})
	}
}
func TestProgressUIListAccessibilityAndSelection(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	f.s.titleTransition(func() { f.s.runtimeDayRail = false; f.s.runtimeSelection = "" })
	f.all()
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 25)
	bar, ok := f.tt.Find(weightedProgressLabel)
	if !ok {
		t.Fatal("missing list progress")
	}
	f.tt.ClickAt(bar.X+100, bar.Y+bar.H/2)
	f.all()
	if f.s.runtimeSelection != string(tr.ID) || !f.s.runtimeDayRail {
		t.Fatal("progress overlay swallowed row selection")
	}
}
func TestProgressUIDayAdvanceInvalidatesOnce(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	before := f.h.progress.projection
	next, _ := liveTestDay.AddDays(1)
	f.clock.now = f.clock.now.AddDate(0, 0, 1)
	f.h.observeDay()
	f.all()
	if f.h.progress.projection == before || f.h.today != next {
		t.Fatal("natural day did not publish fresh graph")
	}
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	f.s.selectRuntimeDate(liveTestDay, false)
	f.all()
	f.s.titleTransition(func() { f.s.runtimeSelection = string(tr.ID); f.s.runtimeDayRail = true })
	f.all()
	if f.h.graph.Traces[0].Status != core.TraceUnfinished {
		t.Fatal("not historical fixture")
	}
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 25)
	last := f.h.progress.projection
	f.h.observeDay()
	f.all()
	if last != f.h.progress.projection {
		t.Fatal("same day rebuilt")
	}
}
func TestProgressUICompletedGreenAndZeroTrack(t *testing.T) {
	f, tr, _, hard := progressFixtureUI(t)
	liveClick(t, f.tt, subtaskPrefix("day", string(hard.ID))+".completion")
	f.all()
	liveClick(t, f.tt, "切换完成 "+string(tr.ID))
	f.all()
	detail := ui.NewTester(func(c *ui.Context) { f.s.liveDetailProgress(c, 252, tr.ID) }, 300, 100)
	detail.SetScale(2)
	progressAccess(t, detail, core.ProgressManual, 100)
	p, _ := detail.Find("100%")
	y := int(math.Round(float64((p.H + detailProgressGap) * 2)))
	progressPixel(t, detail.Image(), 250, y+6, sourceVisual.success)
	progressSavePNG(t, detail, "completed-green-component-2x.png")
	f = newTitleFixture(t, true)
	tr = f.svc.Snapshot().Traces[0]
	detail = ui.NewTester(func(c *ui.Context) { f.s.liveDetailProgress(c, 252, tr.ID) }, 300, 100)
	detail.SetScale(2)
	progressAccess(t, detail, core.ProgressManual, 0)
	p, _ = detail.Find("0%")
	y = int(math.Round(float64((p.H + detailProgressGap) * 2)))
	progressPixel(t, detail.Image(), 250, y+6, sourceVisual.chip)
	progressSavePNG(t, detail, "manual-zero-component-2x.png")
}

func TestProgressUIEmptyCurrentChildrenDoNotEnableManual(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	f.s.returnTaskToPool(tr.ChainID, tr.ID)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(tr.ChainID) })
	f.all()
	g := f.svc.Snapshot()
	def := g.Definitions[len(g.Definitions)-1]
	if len(def.PlannedSubtasks) != 1 {
		t.Fatal("expected remaining plan")
	}
	liveClick(t, f.tt, subtaskPrefix("pool", string(def.PlannedSubtasks[0].ID))+".delete")
	f.all()
	liveClick(t, f.tt, "排到今天 "+string(tr.ChainID))
	f.all()
	g = f.svc.Snapshot()
	next := g.Traces[len(g.Traces)-1]
	if f.h.progress.projection.SubtaskProgress(next.ID).Total != 0 {
		t.Fatal("current trace is not empty")
	}
	can, err := f.h.progress.projection.CanSetManualProgress(next.ID, liveTestDay)
	if err != nil || can {
		t.Fatal("full chain manual guard bypassed", can, err)
	}
	wantUIProgress(t, f.s, next.ID, 25, 1, 4)
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(next.ID); f.s.runtimeDayRail = true })
	f.all()
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 25)
	// No current open children: completion remains an explicit parent command,
	// irrespective of weighted history not yet reaching 100 percent.
	liveClick(t, f.tt, "切换完成 "+string(next.ID))
	f.all()
	p := wantUIProgress(t, f.s, next.ID, 100, 1, 1)
	if p.Mode != core.ProgressManual {
		t.Fatal(p)
	}
}
func TestProgressUILongParentAndMultipleRowsDoNotOverlap(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	long := strings.Repeat("合成长父任务标题需要自然换行，", 8)
	f.edit(long)
	f.advance(700 * time.Millisecond)
	f.all()
	f.h.command(f.s, func() error { return f.svc.AddToday("下一个父项") }, false, nil)
	f.all()
	g := f.svc.Snapshot()
	other := g.Traces[len(g.Traces)-1]
	liveClick(t, f.tt, "展开子任务 "+string(tr.ID))
	f.all()
	f.tt.SetSize(960, 720)
	f.tt.SetScale(2)
	f.all()
	row, _ := f.tt.Find("查看任务 " + string(tr.ID))
	next, _ := f.tt.Find("查看任务 " + string(other.ID))
	bar, _ := f.tt.Find(weightedProgressLabel)
	title, _ := f.tt.Find(long)
	if title.H <= 28 || row.Y+row.H > next.Y+.01 || bar.Y < title.Y+title.H+rowProgressTop-.01 || bar.X+bar.W > row.X+row.W {
		t.Fatal("long row overlap", row, next, bar, title)
	}
	progressSavePNG(t, f.tt, "long-parent-expanded-minimum-2x.png")
}
func TestProgressUIStandalonePublicationAndDayBoundary(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	s := newRuntimeApp(f.svc)
	p := s.progress.projection
	if s.titles != nil {
		t.Fatal("not standalone")
	}
	s.runCommand(func() error {
		return f.svc.SetSubtaskCompleted(tr.ChainID, tr.ID, f.svc.Snapshot().Subtasks[1].ID, true)
	}, false)
	wantUIProgress(t, s, tr.ID, 100, 4, 4)
	if s.progress.projection == p {
		t.Fatal("standalone command left stale cache")
	}
	before := s.progress.projection
	s.observeRuntimeDay(func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) })
	if err := s.refreshDay(); err != nil {
		t.Fatal(err)
	}
	if s.progress.projection == before {
		t.Fatal("standalone boundary left stale cache")
	}
	last := s.progress.projection
	if err := s.refreshDay(); err != nil {
		t.Fatal(err)
	}
	if last != s.progress.projection {
		t.Fatal("unchanged boundary rebuilt")
	}
}
