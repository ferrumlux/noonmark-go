// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Structural fixtures exercise source selectors, not unsupported Continue,
// Defer, Abandon or Reactivate commands.
func reviewOneChain(statuses ...core.TraceStatus) core.TaskGraph {
	g := fixture(statuses...)
	for i := range g.Traces {
		g.Traces[i].ChainID = g.Chains[0].ID
		g.Traces[i].DefinitionID = g.Definitions[0].ID
	}
	g.Chains = g.Chains[:1]
	g.Definitions = g.Definitions[:1]
	return g
}
func reviewProjection(t *testing.T, g core.TaskGraph) *HistoryProjection {
	t.Helper()
	p, e := NewHistoryProjection(g, strings.Compare)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestIndependentHistoryCompletionMomentNotDateAndLatestChild(t *testing.T) {
	g := reviewOneChain(core.TraceCompleted, core.TraceCompleted)
	// Completion moment, not trace date or input order, selects the parent.
	g.Traces[1].Date = date("2026-07-25")
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: g.Traces[1].Date})
	g.Traces[0].CompletedAt = ptr(instant(9))
	g.Traces[0].ContentUpdatedAt = instant(9)
	g.Traces[1].CompletedAt = ptr(instant(8))
	g.Traces[1].ContentUpdatedAt = instant(8)
	for i := 0; i < 2; i++ {
		g.Subtasks = append(g.Subtasks, core.Subtask{ID: core.SubtaskID(id(700 + i)), LineageID: core.SubtaskLineageID(id(750)), TraceID: g.Traces[i].ID, Title: "same lineage", Position: 1, Difficulty: core.DifficultyMedium, Status: core.SubtaskCompleted, CompletedAt: ptr(instant(10)), UpdatedAt: instant(10)})
	}
	h := reviewProjection(t, g).CompletedTaskHierarchies()[0]
	if h.ParentCompletion.Trace.ID != g.Traces[0].ID || h.LatestCompletionAt != instant(10) || len(h.CompletedChildren) != 1 || h.CompletedChildren[0].Subtask.ID != g.Subtasks[1].ID {
		t.Fatalf("wrong moment/lineage selection: %+v", h)
	}
	if h.ParentCompletion.Trajectory.CompletedDate != date("2026-07-24") || len(h.ParentCompletion.Trajectory.Traces) != 2 {
		t.Fatal("trajectory incorrectly clipped at parent completion")
	}
	st := h.ParentCompletion.Trajectory.SubtaskTrajectories[0]
	if st.CompletedDate == nil || *st.CompletedDate != date("2026-07-24") || len(st.Records) != 2 {
		t.Fatal("trajectory must retain earliest completion even though hierarchy selects latest record")
	}
	slices.Reverse(g.Traces)
	slices.Reverse(g.Subtasks)
	if !reflect.DeepEqual(h, reviewProjection(t, g).CompletedTaskHierarchies()[0]) {
		t.Fatal("source-defined order is input dependent")
	}
}
func TestIndependentHistoryUnfinishedDateTiesAndActionPrecedence(t *testing.T) {
	g := reviewOneChain(core.TraceUnfinished, core.TraceAbandoned, core.TraceUnfinished)
	// Equal-date details stay in source enumeration order: no invented UUID key.
	slices.Reverse(g.Traces)
	p := reviewProjection(t, g)
	u := p.UnfinishedPool()[0]
	for i := range g.Traces {
		if u.UnfinishedTraces[i].ID != g.Traces[i].ID {
			t.Fatal("invented detail tie-break")
		}
	}
	a := u.ActionPlan()
	if len(a) != 2 || a[0].Kind != ContinueHistoryTrace || a[0].TraceID != g.Traces[2].ID {
		t.Fatal(a)
	}
	g.Chains[0].State = core.ChainAbandoned
	u = reviewProjection(t, g).UnfinishedPool()[0]
	a = u.ActionPlan()
	if len(a) != 1 || a[0].Kind != ReactivateHistoryChain || a[0].TraceID != g.Traces[1].ID {
		t.Fatal(a)
	}
	g.Chains[0].State = core.ChainActive
	g.Traces[0].Status = core.TracePending
	g.Traces[0].SettledAt = nil
	u = reviewProjection(t, g).UnfinishedPool()[0]
	a = u.ActionPlan()
	if !u.IsContinuedPending() || u.IsInTaskPool || len(a) != 1 || a[0].Kind != AbandonHistoryChain || a[0].TraceID != g.Traces[0].ID {
		t.Fatal(u, a)
	}
}
func TestIndependentHistoryComparatorRequiredAndUsed(t *testing.T) {
	g := fixture(core.TraceCompleted, core.TraceCompleted)
	g.Definitions[0].Title = "A"
	g.Definitions[1].Title = "Z"
	calls := 0
	p, e := NewHistoryProjection(g, func(a, b string) int { calls++; return -strings.Compare(a, b) })
	if e != nil {
		t.Fatal(e)
	}
	h := p.CompletedTaskHierarchies()
	if calls == 0 || h[0].Definition.Title != "Z" {
		t.Fatal("not using injected presentation comparator")
	}
	if _, e := NewHistoryProjection(g, nil); e == nil {
		t.Fatal("silent default comparator")
	}
}
func TestIndependentHistoryUnfinishedPoolLatestContentClock(t *testing.T) {
	g := reviewOneChain(core.TraceUnfinished, core.TraceReturnedToPool)
	g.Traces[0].CreatedAt = instant(5)
	g.Traces[0].SettledAt = ptr(instant(5))
	g.Traces[0].ContentUpdatedAt = instant(5)
	g.Traces[1].ContentUpdatedAt = instant(9)
	p := reviewProjection(t, g)
	u := p.UnfinishedPool()[0]
	if !u.IsInTaskPool || u.Chain.UpdatedAt != instant(5) || u.ActionPlan()[0].Kind != OpenHistoryTaskPool {
		t.Fatal(u)
	}
	u.UnfinishedTraces[0].SettledAt = ptr(instant(99))
	u.Chain.NoteEntries = append(u.Chain.NoteEntries, core.TaskNoteEntry{})
	if !reflect.DeepEqual(p.UnfinishedPool()[0], reviewProjection(t, g).UnfinishedPool()[0]) {
		t.Fatal("query mutation leaked")
	}
	// A completion excludes the whole chain, regardless of latest returned fact.
	g.Traces[0].Status = core.TraceCompleted
	g.Traces[0].SettledAt = nil
	g.Traces[0].CompletedAt = ptr(instant(5))
	if len(reviewProjection(t, g).UnfinishedPool()) != 0 {
		t.Fatal("completed chain remained unresolved")
	}
}
func TestIndependentHistoryRecurringAndInvalidGraphs(t *testing.T) {
	g := fixture(core.TraceCompleted)
	g.Chains[0].CycleMembership = &core.TaskCycleMembership{SeriesID: core.TaskCycleSeriesID(id(900)), OccurrenceDate: date("2026-07-24")}
	if len(reviewProjection(t, g).CompletedTaskHierarchies()) != 0 {
		t.Fatal("recurring chain leaked")
	}
	// Validation is whole-graph, including excluded recurring facts.
	g.Traces[0].CompletedAt = nil
	if _, e := NewHistoryProjection(g, strings.Compare); e == nil {
		t.Fatal("invalid excluded graph accepted")
	}
}
func TestIndependentHistoryContinuationVersusDeferralAndFirstTitle(t *testing.T) {
	for _, sourceStatus := range []core.TraceStatus{core.TraceUnfinished, core.TraceDeferred} {
		g := reviewOneChain(sourceStatus, core.TraceCompleted)
		g.Traces[1].Date = date("2026-07-25")
		g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: g.Traces[1].Date})
		g.Traces[1].CarriedFromTraceID = ptr(g.Traces[0].ID)
		childStatus := core.SubtaskDeferred
		if sourceStatus == core.TraceUnfinished {
			g.Traces[1].ContinuationSeq = 1
			childStatus = core.SubtaskUnfinished
		}
		g.Subtasks = []core.Subtask{
			{ID: core.SubtaskID(id(710)), LineageID: core.SubtaskLineageID(id(760)), TraceID: g.Traces[0].ID, Title: "first title", Position: 1, Difficulty: core.DifficultyMedium, Status: childStatus, SettledAt: ptr(instant(0))},
			{ID: core.SubtaskID(id(711)), LineageID: core.SubtaskLineageID(id(760)), TraceID: g.Traces[1].ID, Title: "latest title", Position: 1, Difficulty: core.DifficultyMedium, Status: core.SubtaskCompleted, CompletedAt: ptr(instant(0)), CarriedFromSubtaskID: ptr(core.SubtaskID(id(710)))},
		}
		tr := reviewProjection(t, g).CompletedTaskHierarchies()[0].ParentCompletion.Trajectory
		wantDates := 0
		if sourceStatus == core.TraceUnfinished {
			wantDates = 1
		}
		if len(tr.ContinuedDates) != wantDates || len(tr.SubtaskTrajectories[0].ContinuedDates) != wantDates || tr.SubtaskTrajectories[0].Title != "first title" || len(tr.SubtaskTrajectories[0].Records) != 2 {
			t.Fatal(sourceStatus, tr)
		}
		if wantDates == 1 && tr.ContinuedDates[0] != g.Traces[1].Date {
			t.Fatal("continuation date must be target date")
		}
	}
}
