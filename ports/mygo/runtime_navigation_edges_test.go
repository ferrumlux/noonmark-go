// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

// Projection-only source truth table. These deliberately minimal graphs test
// predicates, not Service acceptance or support for recurring-task workflows.
func TestNavigationProjectionSourcePredicates(t *testing.T) {
	today := liveTestDay
	future, _ := today.AddDays(1)
	stamp, _ := core.InstantFromSeconds(100)
	later, _ := core.InstantFromSeconds(101)
	base := func() core.TaskGraph {
		return core.TaskGraph{Chains: []core.TaskChain{{ID: "c", State: core.ChainActive}}, Definitions: []core.TaskDefinition{{ID: "d", ChainID: "c"}}}
	}
	trace := func(id core.DayTraceID, status core.TraceStatus) core.DayTrace {
		return core.DayTrace{ID: id, ChainID: "c", DefinitionID: "d", Date: today, Status: status, CreatedAt: stamp, ContentUpdatedAt: stamp}
	}
	tests := []struct {
		name   string
		mutate func(*core.TaskGraph)
		want   runtimeNavigationCounts
	}{
		{"empty chain", func(g *core.TaskGraph) {}, runtimeNavigationCounts{pool: 1}},
		{"no current definition", func(g *core.TaskGraph) { g.Definitions[0].SupersededAt = &stamp }, runtimeNavigationCounts{}},
		{"abandoned not pool", func(g *core.TaskGraph) { g.Chains[0].State = core.ChainAbandoned }, runtimeNavigationCounts{}},
		{"pending today", func(g *core.TaskGraph) { g.Traces = []core.DayTrace{trace("a", core.TracePending)} }, runtimeNavigationCounts{day: 1}},
		{"future pending", func(g *core.TaskGraph) {
			tr := trace("a", core.TracePending)
			tr.Date = future
			g.Traces = []core.DayTrace{tr}
		}, runtimeNavigationCounts{upcoming: 1}},
		{"missing future definition", func(g *core.TaskGraph) {
			tr := trace("a", core.TracePending)
			tr.Date = future
			g.Traces = []core.DayTrace{tr}
			g.Definitions = nil
		}, runtimeNavigationCounts{}},
		{"unfinished chain dedup", func(g *core.TaskGraph) {
			g.Traces = []core.DayTrace{trace("a", core.TraceUnfinished), trace("b", core.TraceAbandoned)}
		}, runtimeNavigationCounts{unfinished: 1}},
		{"abandoned unfinished", func(g *core.TaskGraph) {
			g.Chains[0].State = core.ChainAbandoned
			g.Traces = []core.DayTrace{trace("a", core.TraceAbandoned)}
		}, runtimeNavigationCounts{unfinished: 1}},
		{"completed suppresses unfinished", func(g *core.TaskGraph) {
			g.Traces = []core.DayTrace{trace("a", core.TraceUnfinished), trace("b", core.TraceCompleted), trace("c", core.TraceCompleted)}
		}, runtimeNavigationCounts{completed: 1}},
		{"returned pool and unfinished", func(g *core.TaskGraph) {
			g.Traces = []core.DayTrace{trace("a", core.TraceUnfinished), trace("b", core.TraceReturnedToPool)}
		}, runtimeNavigationCounts{pool: 1, unfinished: 1}},
		{"cancelled pool", func(g *core.TaskGraph) { g.Traces = []core.DayTrace{trace("a", core.TraceCancelledDraft)} }, runtimeNavigationCounts{pool: 1}},
		{"pending blocks pool despite latest return", func(g *core.TaskGraph) {
			g.Traces = []core.DayTrace{trace("a", core.TracePending), trace("b", core.TraceReturnedToPool)}
		}, runtimeNavigationCounts{day: 1}},
		{"updated time precedes UUID", func(g *core.TaskGraph) {
			a := trace("a", core.TraceCompleted)
			a.ContentUpdatedAt = later
			g.Traces = []core.DayTrace{a, trace("z", core.TraceReturnedToPool)}
		}, runtimeNavigationCounts{completed: 1}},
		{"created time precedes UUID", func(g *core.TaskGraph) {
			a := trace("a", core.TraceCompleted)
			a.CreatedAt = later
			g.Traces = []core.DayTrace{a, trace("z", core.TraceReturnedToPool)}
		}, runtimeNavigationCounts{completed: 1}},
		{"UUID final tie break", func(g *core.TaskGraph) {
			g.Traces = []core.DayTrace{trace("z", core.TraceCompleted), trace("a", core.TraceReturnedToPool)}
		}, runtimeNavigationCounts{completed: 1}},
		{"historic definition completed", func(g *core.TaskGraph) {
			g.Definitions[0].SupersededAt = &stamp
			g.Traces = []core.DayTrace{trace("a", core.TraceCompleted)}
		}, runtimeNavigationCounts{completed: 1}},
		{"missing completed definition", func(g *core.TaskGraph) {
			g.Definitions = nil
			g.Traces = []core.DayTrace{trace("a", core.TraceCompleted)}
		}, runtimeNavigationCounts{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := base()
			tt.mutate(&g)
			p := newRuntimeNavigationProjection(g)
			if got := p.counts(today); got != tt.want {
				t.Fatalf("%+v want %+v", got, tt.want)
			}
			for i, j := 0, len(g.Traces)-1; i < j; i, j = i+1, j-1 {
				g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i]
			}
			if got := newRuntimeNavigationProjection(g).counts(today); got != tt.want {
				t.Fatal("input ordering changed counts", got)
			}
		})
	}
	for _, status := range []core.TraceStatus{core.TracePending, core.TraceDeferred, core.TraceUnfinished, core.TraceCompleted, core.TraceChanged, core.TraceReturnedToPool, core.TraceAbandoned, core.TraceCancelledDraft} {
		g := base()
		g.Traces = []core.DayTrace{trace("a", status)}
		g.Subtasks = []core.Subtask{{TraceID: "a", Status: core.SubtaskCompleted}, {TraceID: "a", Status: core.SubtaskCompleted}}
		want := 0
		if status.FormsDayHistory() {
			want = 1
		}
		if got := newRuntimeNavigationProjection(g).counts(today).completed; got != want {
			t.Fatal(status, got, want)
		}
		g.Chains[0].CycleMembership = &core.TaskCycleMembership{}
		p := newRuntimeNavigationProjection(g)
		if p.pool != 0 || p.unfinished != 0 || p.completed != 0 {
			t.Fatal("ordinary hierarchy counted recurring chain", p)
		}
	}
	p := newRuntimeNavigationProjection(core.TaskGraph{Traces: []core.DayTrace{trace("a", core.TracePending)}})
	if p.counts(future).day != 0 || p.counts(today).day != 1 || p.counts(core.LocalDate{}) != (runtimeNavigationCounts{}) {
		t.Fatal("date query")
	}
	if (runtimeNavigationCounts{day: 9}).forPage("ideas") != 0 {
		t.Fatal("invented unsupported count")
	}
}

func TestNavigationUIReturnFutureReopenAndDayBoundary(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	f.s.returnTaskToPool(tr.ChainID, tr.ID)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"pool": 1, "completed": 1})
	future, _ := liveTestDay.AddDays(1)
	f.h.command(f.s, func() error { return f.svc.Schedule(tr.ChainID, future) }, false, nil)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"upcoming": 1, "completed": 1})
	g := f.svc.Snapshot()
	next := g.Traces[len(g.Traces)-1]
	f.s.returnTaskToPool(next.ChainID, next.ID)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"pool": 1, "completed": 1})
	f.h.command(f.s, func() error { return f.svc.Schedule(tr.ChainID, liveTestDay) }, false, nil)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 1, "completed": 1})
	f.s.selectRuntimeDate(future, false)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 1, "completed": 1})
	f.clock.now = f.clock.now.AddDate(0, 0, 1)
	f.h.observeDay()
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"unfinished": 1, "completed": 1})
	before := f.h.progress.navigation
	f.h.observeDay()
	f.all()
	if before != f.h.progress.navigation {
		t.Fatal("no-op day rebuilt")
	}
	g = f.svc.Snapshot()
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(f.path, future)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	wantNavigationBadges(t, newRuntimeApp(reopened), map[string]int{"unfinished": 1, "completed": 1})
	if !reflect.DeepEqual(g, reopened.Snapshot()) {
		t.Fatal("reopen changed graph")
	}
}

func TestNavigationUICacheTwoWindowsPublicationAndFailure(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	before := f.h.progress.navigation
	f.h.command(f.s, func() error { return f.svc.SetCompleted(tr.ID, true) }, false, nil)
	f.h.command(other, func() error { return f.svc.SetCompleted(tr.ID, false) }, false, nil)
	f.ui()
	f.io.one()
	f.io.one()
	if f.h.progress.navigation != before {
		t.Fatal("premature publish")
	}
	wantNavigationBadges(t, other, map[string]int{"day": 1})
	f.posts.one()
	wantNavigationBadges(t, f.s, map[string]int{"completed": 1})
	wantNavigationBadges(t, other, map[string]int{"completed": 1})
	first := f.h.progress.navigation
	f.posts.one()
	if first != f.h.progress.navigation {
		t.Fatal("completion callback rebuild")
	}
	f.posts.one()
	wantNavigationBadges(t, other, map[string]int{"day": 1})
	f.all()
	last := f.h.progress.navigation
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	for i := 0; i < 20; i++ {
		f.tt.Frame()
		tt.Frame()
		f.ui()
	}
	if last != f.h.progress.navigation || other.progress != f.s.progress {
		t.Fatal("per-window/per-frame rebuild")
	}
	if allocs := testing.AllocsPerRun(1000, func() { f.s.navigationCounts(); other.navigationCounts() }); allocs != 0 {
		t.Fatal("query allocations", allocs)
	}
	bad := f.svc.Snapshot()
	bad.Traces[0].Status = core.TraceStatus("bad")
	f.h.publish(bad, liveTestDay)
	f.ui()
	wantNavigationBadges(t, f.s, map[string]int{})
	if last != f.h.progress.navigation {
		t.Fatal("invalid graph replaced projection")
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	wantNavigationBadges(t, f.s, map[string]int{"day": 1})
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			before := f.h.progress.navigation
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			liveClick(t, f.tt, "切换完成 "+string(tr.ID))
			f.all()
			wantNavigationBadges(t, f.s, map[string]int{"day": 1})
			if before != f.h.progress.navigation || f.s.runtimeError == "" {
				t.Fatal("failed write published")
			}
			f.svc.Close()
			reopened, err := taskapp.Open(f.path, liveTestDay)
			if phase == "before-reserve" || phase == "after-persist" {
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				want := map[string]int{"day": 1}
				if phase == "after-persist" {
					want = map[string]int{"completed": 1}
				}
				wantNavigationBadges(t, newRuntimeApp(reopened), want)
			} else if err == nil {
				reopened.Close()
				t.Fatal("incomplete write reopened")
			}
		})
	}
}

func TestNavigationUILongListCompactAccessibility(t *testing.T) {
	f := newTitleFixture(t, false)
	for i := 0; i < 45; i++ {
		if err := f.svc.AddToday(fmt.Sprintf("长列表任务 %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 45})
	before := f.h.progress.navigation
	f.tt.Scroll(700, 500, 0, 1800)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 45})
	for _, n := range nav {
		if _, ok := f.tt.AccessByRole(ui.RoleButton, "切换到"+n.title); !ok {
			t.Fatal("lost navigation access", n.page)
		}
	}
	f.tt.SetScale(2)
	navigationSavePNG(t, f.tt, "long-scroll-2x.png")
	liveClick(t, f.tt, "收起侧栏")
	f.all()
	if got := navigationBadges(t, f.s, true); len(got) != 0 {
		t.Fatal("compact badges", got)
	}
	for _, n := range nav {
		if _, ok := f.tt.AccessByRole(ui.RoleButton, "切换到"+n.title); !ok {
			t.Fatal("compact access", n.page)
		}
	}
	navigationSavePNG(t, f.tt, "compact-2x.png")
	if f.h.progress.navigation != before {
		t.Fatal("presentation changed projection")
	}
}

func TestNavigationConstructorOnlyAtPublication(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "newRuntimeNavigationProjection" {
					calls++
					if path != "runtime_progress.go" || fn.Name.Name != "publish" {
						t.Fatal("navigation rebuilt outside shared publication", path, fn.Name.Name)
					}
				}
				if path == "runtime_navigation.go" {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Clone" || sel.Sel.Name == "NewTaskKernel") {
						t.Fatal("navigation adds whole graph ownership", sel.Sel.Name)
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
func TestNavigationDateOnlyPublicationUsesNewToday(t *testing.T) {
	f := newTitleFixture(t, false)
	future, _ := liveTestDay.AddDays(1)
	if err := f.svc.AddOnDate("未来任务", future); err != nil {
		t.Fatal(err)
	}
	graph := f.svc.Snapshot()
	f.h.publish(graph, liveTestDay)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"upcoming": 1})
	before := f.h.progress.navigation
	f.h.publish(graph, future)
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 1})
	if before != f.h.progress.navigation {
		t.Fatal("date-only identical graph rebuilt")
	}
}
