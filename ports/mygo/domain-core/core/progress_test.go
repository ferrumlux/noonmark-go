// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
)

func TestProgressOfficialWeightedSubtaskExample(t *testing.T) {
	// NoonmarkEngineTests.testWeightedSubtaskProgressUsesDifficultyAndBlocksManualProgress.
	f := newProgressFixture()
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	f.sub(0, DifficultyHard, SubtaskPending, "")
	p := progressView(t, f.g)
	id := f.g.Traces[0].ID
	wantProgress(t, p, id, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	wantManual(t, p, id, f.g.Traces[0].Date, false)
	if got := p.SubtaskProgress(id); got != (SubtaskProgress{Total: 2, Completed: 1, Pending: 1}) || !got.IsPartiallyCompleted() || got.CanCompleteParent() {
		t.Fatal(got)
	}
	wantCompletion(t, p, id, f.g.Traces[0].Date, DayTraceCompletionCapability{Blocker: CompletionOpenSubtasks})
}
func TestProgressOfficialManualContinuationFloor(t *testing.T) {
	// NoonmarkEngineTests.testManualProgressCarriesForwardAndCannotRegressAfterContinuation.
	for _, manual := range []int{0, 10, 35, 60} {
		t.Run(fmt.Sprint(manual), func(t *testing.T) {
			f := newProgressFixture()
			f.traceStatus(0, TraceUnfinished)
			f.g.Traces[0].ManualProgressPercent = ptr(35)
			i := f.continuation(0, date("2026-11-06"), TracePending)
			f.g.Traces[i].ManualProgressPercent = ptr(manual)
			p := progressView(t, f.g)
			wantProgress(t, p, f.g.Traces[i].ID, TraceProgress{Mode: ProgressManual, Percent: max(35, manual), FloorPercent: 35, CompletedWeight: manual, TotalWeight: 100})
			wantManual(t, p, f.g.Traces[i].ID, f.g.Traces[i].Date, true)
		})
	}
}
func TestProgressLineageUsesHistoricalMaximumNotCurrentDifficulty(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	a := f.sub(0, DifficultyHard, SubtaskUnfinished, "")
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	second := f.continuation(0, date("2026-11-06"), TraceUnfinished)
	b := f.sub(second, DifficultyMedium, SubtaskUnfinished, f.g.Subtasks[a].LineageID)
	f.g.Subtasks[b].CarriedFromSubtaskID = ptr(f.g.Subtasks[a].ID)
	third := f.continuation(second, date("2026-11-07"), TracePending)
	c := f.sub(third, DifficultySimple, SubtaskCompleted, f.g.Subtasks[a].LineageID)
	f.g.Subtasks[c].CarriedFromSubtaskID = ptr(f.g.Subtasks[b].ID)
	f.sub(third, DifficultyMedium, SubtaskPending, "")
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	wantProgress(t, p, f.g.Traces[second].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, FloorPercent: 25, CompletedWeight: 1, TotalWeight: 4})
	wantProgress(t, p, f.g.Traces[third].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 67, FloorPercent: 25, CompletedWeight: 4, TotalWeight: 6})
	if got := p.SubtaskProgress(f.g.Traces[third].ID); got != (SubtaskProgress{Total: 2, Completed: 1, Pending: 1}) {
		t.Fatal(got)
	}
}
func TestProgressHistoricalFloorCanExceedRawWeightRatio(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	f.sub(0, DifficultyHard, SubtaskCompleted, "")
	f.sub(0, DifficultySimple, SubtaskUnfinished, "")
	next := f.continuation(0, date("2026-11-06"), TracePending)
	f.sub(next, DifficultyHard, SubtaskPending, "")
	f.sub(next, DifficultyMedium, SubtaskAbandoned, "")
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 75, FloorPercent: 75, CompletedWeight: 3, TotalWeight: 9})
}
func TestProgressFloorUsesMaximumOfAllEarlierRawValues(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceReturnedToPool)
	f.g.Traces[0].ManualProgressPercent = ptr(80)
	second := f.trace(date("2026-11-06"), TraceReturnedToPool)
	f.g.Traces[second].ManualProgressPercent = ptr(20)
	third := f.trace(date("2026-11-07"), TracePending)
	f.g.Traces[third].ManualProgressPercent = ptr(10)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[third].ID, TraceProgress{Mode: ProgressManual, Percent: 80, FloorPercent: 80, CompletedWeight: 10, TotalWeight: 100})
}
func TestProgressSameDayRawIncludesLaterSequenceButFloorDoesNot(t *testing.T) {
	// Continuation still comes from an earlier date; no invalid same-day carry.
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	later := f.continuation(0, date("2026-11-06"), TraceReturnedToPool)
	f.g.Traces[later].ManualProgressPercent = ptr(90)
	early := f.trace(date("2026-11-06"), TracePending)
	f.g.Traces[early].ManualProgressPercent = ptr(10)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[early].ID, TraceProgress{Mode: ProgressManual, Percent: 10, CompletedWeight: 10, TotalWeight: 100})
	wantProgress(t, p, f.g.Traces[later].ID, TraceProgress{Mode: ProgressManual, Percent: 90, FloorPercent: 10, CompletedWeight: 90, TotalWeight: 100})
	f.sub(later, DifficultyHard, SubtaskCompleted, "")
	f.sub(early, DifficultySimple, SubtaskPending, "")
	p = progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[early].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 75, CompletedWeight: 3, TotalWeight: 4})
	wantProgress(t, p, f.g.Traces[later].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 75, FloorPercent: 75, CompletedWeight: 3, TotalWeight: 4})
}
func TestProgressSameDateSameSequenceDoesNotCreateFloor(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceReturnedToPool)
	f.g.Traces[0].ManualProgressPercent = ptr(90)
	next := f.trace(f.g.Traces[0].Date, TracePending)
	f.g.Traces[next].ManualProgressPercent = ptr(10)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressManual, Percent: 10, CompletedWeight: 10, TotalWeight: 100})
}
func TestProgressCancelledTraceAndChildHaveDifferentProjectionRules(t *testing.T) {
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(12)
	f.sub(0, DifficultyHard, SubtaskCancelledDraft, "")
	hidden := f.trace(f.g.Traces[0].Date, TraceCancelledDraft)
	f.g.Traces[hidden].ManualProgressPercent = ptr(99)
	f.sub(hidden, DifficultyHard, SubtaskPending, "")
	p := progressView(t, f.g)
	id := f.g.Traces[0].ID
	wantProgress(t, p, id, TraceProgress{Mode: ProgressManual, Percent: 12, CompletedWeight: 12, TotalWeight: 100})
	if got := p.SubtaskProgress(id); got != (SubtaskProgress{}) {
		t.Fatal(got)
	}
	wantManual(t, p, id, f.g.Traces[0].Date, true)
	// Direct subtaskProgress does not filter parent visibility.
	if got := p.SubtaskProgress(f.g.Traces[hidden].ID); got != (SubtaskProgress{Total: 1, Pending: 1}) {
		t.Fatal(got)
	}
	wantProgress(t, p, f.g.Traces[hidden].ID, TraceProgress{Mode: ProgressManual, Percent: 99, CompletedWeight: 99, TotalWeight: 100})
	wantManual(t, p, f.g.Traces[hidden].ID, f.g.Traces[hidden].Date, false)
}
func TestProgressCancellationDoesNotEraseEarlierLineageWeight(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	old := f.sub(0, DifficultyHard, SubtaskUnfinished, "")
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	next := f.continuation(0, date("2026-11-06"), TracePending)
	hidden := f.sub(next, DifficultySimple, SubtaskCancelledDraft, f.g.Subtasks[old].LineageID)
	f.g.Subtasks[hidden].CarriedFromSubtaskID = ptr(f.g.Subtasks[old].ID)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, FloorPercent: 25, CompletedWeight: 1, TotalWeight: 4})
	if got := p.SubtaskProgress(f.g.Traces[next].ID); got != (SubtaskProgress{}) || !got.CanCompleteParent() {
		t.Fatal(got)
	}
	wantManual(t, p, f.g.Traces[next].ID, f.g.Traces[next].Date, false)
	wantCompletion(t, p, f.g.Traces[next].ID, f.g.Traces[next].Date, DayTraceCompletionCapability{Action: CompletionComplete})
}
func TestProgressAbandonedCountsInWeightButNotCompletionBlocker(t *testing.T) {
	f := newProgressFixture()
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	f.sub(0, DifficultyHard, SubtaskAbandoned, "")
	p := progressView(t, f.g)
	id := f.g.Traces[0].ID
	wantProgress(t, p, id, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	got := p.SubtaskProgress(id)
	if got != (SubtaskProgress{Total: 2, Completed: 1, Abandoned: 1}) || got.IsPartiallyCompleted() || !got.CanCompleteParent() {
		t.Fatal(got)
	}
	wantCompletion(t, p, id, f.g.Traces[0].Date, DayTraceCompletionCapability{Action: CompletionComplete})
	wantManual(t, p, id, f.g.Traces[0].Date, false)
	f.traceStatus(0, TraceCompleted)
	p = progressView(t, f.g)
	wantProgress(t, p, id, TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 1, TotalWeight: 1})
	wantCompletion(t, p, id, f.g.Traces[0].Date, DayTraceCompletionCapability{Action: CompletionUndo})
}
func TestProgressAllChildStatusesAndDeferredSemantics(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceDeferred)
	next := f.continuation(0, date("2026-11-06"), TracePending)
	for _, s := range []SubtaskStatus{SubtaskCompleted, SubtaskPending, SubtaskUnfinished, SubtaskDeferred, SubtaskAbandoned, SubtaskCancelledDraft} {
		i := f.sub(0, DifficultySimple, s, "")
		if s == SubtaskDeferred {
			j := f.sub(next, DifficultySimple, SubtaskPending, f.g.Subtasks[i].LineageID)
			f.g.Subtasks[j].CarriedFromSubtaskID = ptr(f.g.Subtasks[i].ID)
		}
	}
	p := progressView(t, f.g)
	got := p.SubtaskProgress(f.g.Traces[0].ID)
	if got != (SubtaskProgress{Total: 5, Completed: 1, Pending: 1, Unfinished: 1, Deferred: 1, Abandoned: 1}) || !got.IsPartiallyCompleted() || got.CanCompleteParent() {
		t.Fatal(got)
	}
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 20, CompletedWeight: 1, TotalWeight: 5})
	if !(SubtaskProgress{Total: 2, Completed: 1, Deferred: 1}).CanCompleteParent() || !(SubtaskProgress{Total: 2, Completed: 1, Deferred: 1}).IsPartiallyCompleted() {
		t.Fatal("deferred remains actionable but does not block completion")
	}
}
func TestProgressFutureRecordsExcludedButManualGuardUsesWholeChain(t *testing.T) {
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(23)
	future := f.trace(date("2026-11-07"), TraceReturnedToPool)
	f.g.Traces[future].ManualProgressPercent = ptr(99)
	f.sub(future, DifficultyHard, SubtaskCompleted, "")
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressManual, Percent: 23, CompletedWeight: 23, TotalWeight: 100})
	wantManual(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, false)
	wantCompletion(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, DayTraceCompletionCapability{Action: CompletionComplete})
	wantProgress(t, p, f.g.Traces[future].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, FloorPercent: 23, CompletedWeight: 3, TotalWeight: 3})
}
func TestProgressDefinitionsDoNotContributeOrSplitChainHistory(t *testing.T) {
	f := newProgressFixture()
	f.g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(f.id()), LineageID: SubtaskLineageID(f.id()), Title: "仅计划", Difficulty: DifficultyHard, Position: 1, CreatedAt: instant(100)}}
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressManual, TotalWeight: 100})
	wantManual(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, true)
	f.traceStatus(0, TraceReturnedToPool)
	old := f.sub(0, DifficultyHard, SubtaskCompleted, "")
	d := f.g.Definitions[0]
	d.ID = TaskDefinitionID(f.id())
	d.Sequence = 2
	d.PlannedSubtasks = nil
	f.g.Definitions[0].SupersededAt = ptr(instant(100))
	f.g.Definitions[0].SupersededByDefinitionID = ptr(d.ID)
	f.g.Definitions = append(f.g.Definitions, d)
	next := f.trace(date("2026-11-06"), TracePending)
	f.g.Traces[next].DefinitionID = d.ID
	f.sub(next, DifficultySimple, SubtaskPending, f.g.Subtasks[old].LineageID)
	f.sub(next, DifficultyMedium, SubtaskPending, "")
	p = progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, FloorPercent: 100, CompletedWeight: 3, TotalWeight: 5})
}
func TestProgressChainIsolationEvenForSharedLineageIdentity(t *testing.T) {
	f := newProgressFixture()
	c := f.g.Chains[0]
	c.ID = TaskChainID(f.id())
	f.g.Chains = append(f.g.Chains, c)
	d := f.g.Definitions[0]
	d.ID = TaskDefinitionID(f.id())
	d.ChainID = c.ID
	f.g.Definitions = append(f.g.Definitions, d)
	own := f.sub(0, DifficultySimple, SubtaskPending, "")
	other := f.trace(f.g.Traces[0].Date, TraceCompleted)
	f.g.Traces[other].ChainID = c.ID
	f.g.Traces[other].DefinitionID = d.ID
	f.sub(other, DifficultyHard, SubtaskCompleted, f.g.Subtasks[own].LineageID)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, TotalWeight: 1})
	if p.SubtaskProgress(f.g.Traces[0].ID).Completed != 0 {
		t.Fatal("cross-chain completion leaked")
	}
}
func TestProgressRoundingMatchesSwiftNearestAwayFromZero(t *testing.T) {
	for _, tc := range []struct{ completed, total, want int }{{1, 3, 33}, {2, 3, 67}, {1, 8, 13}, {5, 8, 63}, {7, 8, 88}, {1, 6, 17}, {0, 1, 0}, {1, 1, 100}} {
		f := newProgressFixture()
		for i := 0; i < tc.total; i++ {
			status := SubtaskPending
			if i < tc.completed {
				status = SubtaskCompleted
			}
			f.sub(0, DifficultySimple, status, "")
		}
		p := progressView(t, f.g)
		wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: tc.want, CompletedWeight: tc.completed, TotalWeight: tc.total})
	}
}
func TestProgressCompletionCapabilityGuardsAndStatuses(t *testing.T) {
	for _, status := range []TraceStatus{TracePending, TraceCompleted, TraceUnfinished, TraceDeferred, TraceChanged, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned} {
		for _, when := range []string{"today", "past", "future", "locked-today", "locked-past", "locked-future"} {
			t.Run(string(status)+"/"+when, func(t *testing.T) {
				f := newProgressFixture()
				f.traceStatus(0, status)
				if status == TraceDeferred {
					f.continuation(0, date("2026-11-06"), TracePending)
				}
				if status == TraceChanged {
					c := f.g.Chains[0]
					c.ID = TaskChainID(f.id())
					f.g.Chains = append(f.g.Chains, c)
					d := f.g.Definitions[0]
					d.ID = TaskDefinitionID(f.id())
					d.ChainID = c.ID
					f.g.Definitions = append(f.g.Definitions, d)
					i := f.trace(f.g.Traces[0].Date, TracePending)
					f.g.Traces[i].ChainID = c.ID
					f.g.Traces[i].DefinitionID = d.ID
					f.g.Traces[0].ChangedToTraceID = ptr(f.g.Traces[i].ID)
				}
				today := f.g.Traces[0].Date
				want := DayTraceCompletionCapability{Blocker: CompletionUnsupportedStatus, Status: status}
				if status == TracePending {
					want = DayTraceCompletionCapability{Action: CompletionComplete}
				}
				if status == TraceCompleted {
					want = DayTraceCompletionCapability{Action: CompletionUndo}
				}
				switch when {
				case "past", "locked-past":
					today = date("2026-11-06")
					want = DayTraceCompletionCapability{Blocker: CompletionHistoricalDay}
				case "future", "locked-future":
					today = date("2026-11-04")
					want = DayTraceCompletionCapability{Blocker: CompletionFutureDay}
				}
				if len(when) >= 6 && when[:6] == "locked" {
					f.g.Days[0].LockedAt = ptr(instant(100))
					want = DayTraceCompletionCapability{Blocker: CompletionLockedDay}
				}
				p := progressView(t, f.g)
				wantCompletion(t, p, f.g.Traces[0].ID, today, want)
				wantManual(t, p, f.g.Traces[0].ID, today, status == TracePending && when == "today")
			})
		}
	}
}
func TestProgressHistoricalChildrenDoNotBlockCurrentParent(t *testing.T) {
	// testCurrentTraceCanCompleteWhenOnlyHistoricalChainTraceHasSubtasks.
	f := newProgressFixture()
	f.traceStatus(0, TraceDeferred)
	f.sub(0, DifficultyHard, SubtaskCompleted, "")
	current := f.continuation(0, date("2026-11-06"), TracePending)
	p := progressView(t, f.g)
	wantCompletion(t, p, f.g.Traces[current].ID, f.g.Traces[current].Date, DayTraceCompletionCapability{Action: CompletionComplete})
	wantManual(t, p, f.g.Traces[current].ID, f.g.Traces[current].Date, false)
	wantProgress(t, p, f.g.Traces[current].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, FloorPercent: 100, CompletedWeight: 3, TotalWeight: 3})
}
func TestProgressUnfinishedChildBlocksPendingParent(t *testing.T) {
	f := newProgressFixture()
	f.sub(0, DifficultyHard, SubtaskUnfinished, "")
	p := progressView(t, f.g)
	wantCompletion(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, DayTraceCompletionCapability{Blocker: CompletionOpenSubtasks})
}
func TestProgressCompletedParentOverridesWeightsAndRetainsFloor(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	f.g.Traces[0].ManualProgressPercent = ptr(71)
	next := f.continuation(0, date("2026-11-06"), TraceCompleted)
	f.sub(next, DifficultyHard, SubtaskAbandoned, "")
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressManual, Percent: 100, FloorPercent: 71, CompletedWeight: 1, TotalWeight: 1})
}
func TestProgressEmptyMissingInvalidAndNilQueriesDoNotPanic(t *testing.T) {
	empty := progressView(t, TaskGraph{})
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(0)
	normal := progressView(t, f.g)
	for _, p := range []*ProgressProjection{empty, normal, {}, nil} {
		for _, id := range []DayTraceID{"", DayTraceID(identity(90000)), "invalid"} {
			wantProgress(t, p, id, TraceProgress{Mode: ProgressManual})
			got := p.SubtaskProgress(id)
			if got != (SubtaskProgress{}) || !got.CanCompleteParent() || got.IsPartiallyCompleted() {
				t.Fatal(got)
			}
			if got, e := p.CompletionCapability(id, date("2026-11-05")); !errors.Is(e, ErrNotFound) || got != (DayTraceCompletionCapability{}) {
				t.Fatal(got, e)
			}
			if got, e := p.CanSetManualProgress(id, date("2026-11-05")); !errors.Is(e, ErrNotFound) || got {
				t.Fatal(got, e)
			}
		}
		if _, e := p.CompletionCapability(f.g.Traces[0].ID, LocalDate{}); !errors.Is(e, ErrInvalidInput) {
			t.Fatal(e)
		}
		if got, e := p.CanSetManualProgress(f.g.Traces[0].ID, LocalDate{}); !errors.Is(e, ErrInvalidInput) || got {
			t.Fatal(got, e)
		}
	}
	wantProgress(t, normal, f.g.Traces[0].ID, TraceProgress{Mode: ProgressManual, TotalWeight: 100})
}
func TestProgressRejectsMalformedGraphsWithoutMutatingOrPanicking(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TaskGraph)
	}{
		{"duplicate-trace", func(g *TaskGraph) { g.Traces = append(g.Traces, g.Traces[0]) }},
		{"duplicate-child", func(g *TaskGraph) { g.Subtasks = append(g.Subtasks, g.Subtasks[0]) }},
		{"missing-parent", func(g *TaskGraph) { g.Subtasks[0].TraceID = DayTraceID(identity(90000)) }},
		{"weight-zero", func(g *TaskGraph) { g.Subtasks[0].Difficulty = 0 }},
		{"weight-overflow", func(g *TaskGraph) { g.Subtasks[0].Difficulty = Difficulty(int(^uint(0) >> 1)) }},
		{"bad-manual", func(g *TaskGraph) { g.Traces[0].ManualProgressPercent = ptr(101) }},
		{"bad-date", func(g *TaskGraph) { g.Traces[0].Date = LocalDate{Year: 2026, Month: 2, Day: 30} }},
		{"bad-status", func(g *TaskGraph) { g.Subtasks[0].Status = "unknown" }},
		{"bad-instant", func(g *TaskGraph) { g.Subtasks[0].UpdatedAt = Instant(math.Float64bits(math.NaN())) }},
		{"broken-lineage", func(g *TaskGraph) { g.Subtasks[0].CarriedFromSubtaskID = ptr(SubtaskID(identity(90000))) }},
		{"bad-cancellation", func(g *TaskGraph) { g.Subtasks[0].Status = SubtaskCancelledDraft }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := baseGraph()
			g.Subtasks = []Subtask{child()}
			tc.edit(&g)
			before := g.Clone()
			p, e := NewProgressProjection(g)
			var ve *ValidationError
			if p != nil || !errors.As(e, &ve) {
				t.Fatalf("accepted malformed graph: %v, %v", p, e)
			}
			if !reflect.DeepEqual(before, g) {
				t.Fatal("malformed input mutated")
			}
		})
	}
}
func TestProgressProjectionOwnsSnapshotAndAllowsConcurrentReads(t *testing.T) {
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(31)
	p := progressView(t, f.g)
	id := f.g.Traces[0].ID
	day := f.g.Traces[0].Date
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 100; j++ {
				wantProgress(t, p, id, TraceProgress{Mode: ProgressManual, Percent: 31, CompletedWeight: 31, TotalWeight: 100})
				wantCompletion(t, p, id, day, DayTraceCompletionCapability{Action: CompletionComplete})
				wantManual(t, p, id, day, true)
			}
		})
	}
	for i := 0; i < 100; i++ {
		*f.g.Traces[0].ManualProgressPercent = i % 100
		f.g.Traces[0].Status = TraceAbandoned
		f.g.Days[0].LockedAt = ptr(instant(100))
	}
	wg.Wait()
}
func TestProgressInputPermutationDoesNotChangeResults(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	a := f.sub(0, DifficultyHard, SubtaskUnfinished, "")
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	next := f.continuation(0, date("2026-11-06"), TracePending)
	b := f.sub(next, DifficultySimple, SubtaskCompleted, f.g.Subtasks[a].LineageID)
	f.g.Subtasks[b].CarriedFromSubtaskID = ptr(f.g.Subtasks[a].ID)
	f.sub(next, DifficultyMedium, SubtaskPending, "")
	baseline := progressView(t, f.g)
	rng := rand.New(rand.NewPCG(43, 6))
	for iteration := 0; iteration < 50; iteration++ {
		g := f.g.Clone()
		rng.Shuffle(len(g.Days), func(i, j int) { g.Days[i], g.Days[j] = g.Days[j], g.Days[i] })
		rng.Shuffle(len(g.Traces), func(i, j int) { g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i] })
		rng.Shuffle(len(g.Subtasks), func(i, j int) { g.Subtasks[i], g.Subtasks[j] = g.Subtasks[j], g.Subtasks[i] })
		p := progressView(t, g)
		for _, tr := range g.Traces {
			wantProgress(t, p, tr.ID, baseline.TraceProgress(tr.ID))
			if got := p.SubtaskProgress(tr.ID); got != baseline.SubtaskProgress(tr.ID) {
				t.Fatal(got)
			}
		}
	}
}
