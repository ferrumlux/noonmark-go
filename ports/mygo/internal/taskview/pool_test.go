// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func poolProject(t *testing.T, g core.TaskGraph) []PoolRow {
	t.Helper()
	rows, err := ProjectPool(g)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestPoolEligibility(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		status                     core.TraceStatus
		abandoned, recurring, want bool
	}{
		{name: "no trace", want: true},
		{name: "pending", status: core.TracePending},
		{name: "completed", status: core.TraceCompleted},
		{name: "unfinished", status: core.TraceUnfinished},
		{name: "abandoned trace", status: core.TraceAbandoned},
		{name: "returned", status: core.TraceReturnedToPool, want: true},
		{name: "cancelled draft", status: core.TraceCancelledDraft, want: true},
		{name: "abandoned unscheduled chain", abandoned: true},
		{name: "abandoned returned chain", status: core.TraceReturnedToPool, abandoned: true},
		{name: "deleted draft chain", status: core.TraceCancelledDraft, abandoned: true},
		{name: "recurring unscheduled", recurring: true},
		{name: "recurring returned", status: core.TraceReturnedToPool, recurring: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := fixture(core.TracePending)
			if tc.status == "" {
				g.Traces = nil
			} else {
				g = fixture(tc.status)
			}
			if tc.abandoned {
				g.Chains[0].State = core.ChainAbandoned
			}
			if tc.recurring {
				g.Chains[0].CycleMembership = &core.TaskCycleMembership{SeriesID: core.TaskCycleSeriesID(id(900)), OccurrenceDate: date("2026-07-24")}
			}
			rows := poolProject(t, g)
			if (len(rows) == 1) != tc.want {
				t.Fatalf("rows = %+v, want membership %v", rows, tc.want)
			}
		})
	}
}

func TestPoolCommandLifecycle(t *testing.T) {
	// Mirrors Swift testSchedulingFromPoolCreatesDayTraceAndRemovesFromTaskPool with domain commands,
	// not UI fixture rows or copied production data.
	k, err := core.NewTaskKernel(core.TaskGraph{})
	if err != nil {
		t.Fatal(err)
	}
	c, d := core.TaskChainID(id(100)), core.TaskDefinitionID(id(1000))
	if err := k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: c, DefinitionID: d, Title: "合成开发任务", Now: instant(1)}); err != nil {
		t.Fatal(err)
	}
	if rows := poolProject(t, k.Snapshot()); len(rows) != 1 || rows[0].DefinitionID != d {
		t.Fatal(rows)
	}
	today := date("2026-07-24")
	tr := core.DayTraceID(id(10000))
	if err := k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: tr, DayID: core.DayID(id(1)), Date: today, Today: today, Now: instant(2)}); err != nil {
		t.Fatal(err)
	}
	if rows := poolProject(t, k.Snapshot()); len(rows) != 0 {
		t.Fatal(rows)
	}
	if err := k.MarkCompleted(tr, today, instant(3)); err != nil {
		t.Fatal(err)
	}
	if rows := poolProject(t, k.Snapshot()); len(rows) != 0 {
		t.Fatal(rows)
	}
}

func poolTwoTraces(first, second core.TraceStatus) core.TaskGraph {
	g := fixture(first, second)
	g.Traces[1].ChainID = g.Chains[0].ID
	g.Traces[1].DefinitionID = g.Definitions[0].ID
	g.Chains = g.Chains[:1]
	g.Definitions = g.Definitions[:1]
	return g
}

func TestPoolLatestTraceOrdering(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		first, second                          core.TraceStatus
		createdA, updatedA, createdB, updatedB float64
		want                                   bool
	}{
		{"update precedes creation", core.TraceReturnedToPool, core.TraceCompleted, 1, 4, 2, 3, true},
		{"newer terminal excludes old return", core.TraceReturnedToPool, core.TraceCompleted, 1, 2, 2, 3, false},
		{"creation breaks update tie", core.TraceReturnedToPool, core.TraceCompleted, 2, 4, 1, 4, true},
		{"ID breaks all time ties", core.TraceCompleted, core.TraceCancelledDraft, 1, 4, 1, 4, true},
		{"larger terminal ID wins", core.TraceReturnedToPool, core.TraceCompleted, 1, 4, 1, 4, false},
		{"any pending excludes newer return", core.TracePending, core.TraceReturnedToPool, 1, 1, 2, 3, false},
		{"any pending excludes newer cancellation", core.TracePending, core.TraceCancelledDraft, 1, 1, 2, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := poolTwoTraces(tc.first, tc.second)
			for i, times := range [][2]float64{{tc.createdA, tc.updatedA}, {tc.createdB, tc.updatedB}} {
				tr := &g.Traces[i]
				tr.CreatedAt, tr.ContentUpdatedAt = instant(times[0]), instant(times[1])
				if tr.SettledAt != nil {
					tr.SettledAt = ptr(tr.ContentUpdatedAt)
				}
				if tr.CompletedAt != nil {
					tr.CompletedAt = ptr(tr.ContentUpdatedAt)
				}
			}
			for i := 0; i < 2; i++ {
				if rows := poolProject(t, g); (len(rows) == 1) != tc.want {
					t.Fatalf("rows %+v, want membership %v", rows, tc.want)
				}
				g.Traces[0], g.Traces[1] = g.Traces[1], g.Traces[0]
			}
		})
	}
}

func TestPoolCurrentDefinitionAndOrdering(t *testing.T) {
	g := fixture(core.TraceReturnedToPool, core.TraceReturnedToPool, core.TraceReturnedToPool)
	// Chain zero is oldest, but its current definition is newest. The trace
	// still names the retired definition and must not supply the displayed title.
	g.Chains[0].CreatedAt = instant(-10)
	old := g.Definitions[0]
	current := old
	current.ID = core.TaskDefinitionID(id(9999))
	current.Sequence = 2
	current.Title = "当前版本"
	current.CreatedAt, current.ContentUpdatedAt = instant(8), instant(8)
	g.Definitions[0].SupersededAt = ptr(instant(8))
	g.Definitions[0].ContentUpdatedAt = instant(8)
	g.Definitions[0].SupersededByDefinitionID = ptr(current.ID)
	g.Definitions = append(g.Definitions, current)
	// Equal creation times deliberately use ChainID order, unlike Swift's
	// unspecified dictionary-order tie. Negative instants must sort by time.
	for i := 1; i <= 2; i++ {
		g.Definitions[i].CreatedAt = instant(-2)
	}
	rows := poolProject(t, g)
	if len(rows) != 3 || rows[0].ChainID != g.Chains[1].ID || rows[1].ChainID != g.Chains[2].ID || rows[2].Title != current.Title || rows[2].DefinitionID != current.ID || !rows[2].CreatedAt.Equal(instant(8)) {
		t.Fatal(rows)
	}
	g.Chains[0], g.Chains[2] = g.Chains[2], g.Chains[0]
	if got := poolProject(t, g); !reflect.DeepEqual(got, rows) {
		t.Fatalf("input order changed projection: %+v", got)
	}
}

func TestPoolInvalidGraphFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*core.TaskGraph)
	}{
		{"missing current definition", func(g *core.TaskGraph) { g.Definitions = nil }},
		{"duplicate identity", func(g *core.TaskGraph) { g.Chains = append(g.Chains, g.Chains[0]) }},
		{"orphan subtask", func(g *core.TaskGraph) { g.Subtasks = append(g.Subtasks, core.Subtask{ID: core.SubtaskID(id(5000))}) }},
		{"invalid hidden recurring chain", func(g *core.TaskGraph) { g.Chains[0].CycleMembership = &core.TaskCycleMembership{} }},
		{"nonfinite creation time", func(g *core.TaskGraph) { g.Definitions[0].CreatedAt = core.Instant(math.Float64bits(math.NaN())) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := fixture(core.TraceReturnedToPool)
			tc.mutate(&g)
			rows, err := ProjectPool(g)
			var validation *core.ValidationError
			if rows != nil || !errors.As(err, &validation) {
				t.Fatalf("rows=%+v error=%v", rows, err)
			}
		})
	}
}

func TestPoolProjectionDoesNotMutateOrAlias(t *testing.T) {
	g := fixture(core.TraceReturnedToPool, core.TraceCancelledDraft)
	before := g.Clone()
	rows := poolProject(t, g)
	if !reflect.DeepEqual(g, before) {
		t.Fatal("projection mutated graph")
	}
	rows[0].Title = "输出修改"
	rows[0].CreatedAt = instant(999)
	if !reflect.DeepEqual(g, before) {
		t.Fatal("output aliases graph")
	}
	another := poolProject(t, g)
	g.Definitions[0].Title = "输入修改"
	if another[0].Title != before.Definitions[0].Title {
		t.Fatal("input aliases output")
	}
	if another[0].Title == rows[0].Title {
		t.Fatal("results alias one another")
	}
	if empty := poolProject(t, core.TaskGraph{}); len(empty) != 0 {
		t.Fatal(empty)
	}
}

func TestPoolDeferredAndChangedHistory(t *testing.T) {
	t.Run("deferred history with cancelled future target", func(t *testing.T) {
		// Swift testDeferredFutureTargetCanReturnToPoolWithoutRewritingSourceHistory.
		g := poolTwoTraces(core.TraceDeferred, core.TraceCancelledDraft)
		tomorrow := date("2026-07-25")
		g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: tomorrow})
		g.Traces[1].Date = tomorrow
		g.Traces[1].CarriedFromTraceID = ptr(g.Traces[0].ID)
		g.Traces[1].ContentUpdatedAt = instant(1)
		g.Traces[1].SettledAt = ptr(instant(1))
		if rows := poolProject(t, g); len(rows) != 1 {
			t.Fatal(rows)
		}
		// A completed target, rather than cancelled target, excludes the chain.
		g.Traces[1].Status = core.TraceCompleted
		g.Traces[1].DraftCancelledOn = nil
		g.Traces[1].DraftCancellationID = nil
		g.Traces[1].SettledAt = nil
		g.Traces[1].CompletedAt = ptr(instant(1))
		if rows := poolProject(t, g); len(rows) != 0 {
			t.Fatal(rows)
		}
	})
	t.Run("changed chain is not a pool task", func(t *testing.T) {
		g := fixture(core.TraceChanged, core.TracePending)
		g.Traces[0].ChangedToTraceID = ptr(g.Traces[1].ID)
		if rows := poolProject(t, g); len(rows) != 0 {
			t.Fatal(rows)
		}
	})
}

func TestPoolCurrentDefinitionMayRetainRestorationWitness(t *testing.T) {
	g := fixture(core.TraceReturnedToPool)
	retired := g.Definitions[0]
	retired.ID = core.TaskDefinitionID(id(9999))
	retired.Sequence = 2
	retired.Title = "已撤销版本"
	retired.SupersededAt = ptr(instant(1))
	retired.ContentUpdatedAt = instant(1)
	retired.SupersededByDefinitionID = ptr(g.Definitions[0].ID)
	// A current definition may retain this witness after snapshot undo;
	// SupersededByDefinitionID alone must not make it historical.
	g.Definitions[0].SupersededByDefinitionID = ptr(retired.ID)
	g.Definitions = append(g.Definitions, retired)
	if rows := poolProject(t, g); len(rows) != 1 || rows[0].DefinitionID != g.Definitions[0].ID {
		t.Fatal(rows)
	}
}
