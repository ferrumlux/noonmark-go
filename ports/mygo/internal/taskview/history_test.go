// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func history(t *testing.T, g core.TaskGraph) *HistoryProjection {
	t.Helper()
	p, e := NewHistoryProjection(g, strings.Compare)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func histOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func histKernel(t *testing.T) (*core.TaskKernel, core.TaskChainID, core.DayTraceID) {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	histOK(t, e)
	c, tr := core.TaskChainID(id(300)), core.DayTraceID(id(302))
	desc := "synthetic description"
	histOK(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: c, DefinitionID: core.TaskDefinitionID(id(301)), Title: "original", DescriptionText: &desc, InitialNote: &core.TaskNoteEntry{ID: core.TaskNoteEntryID(id(303)), Body: "synthetic note", CreatedAt: instant(1), UpdatedAt: instant(1)}, Now: instant(1)}))
	histOK(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: tr, DayID: core.DayID(id(304)), Date: date("2026-07-24"), Today: date("2026-07-24"), Now: instant(2)}))
	return k, c, tr
}
func histChild(t *testing.T, k *core.TaskKernel, c core.TaskChainID, tr core.DayTraceID, n int, now float64) core.SubtaskEditInput {
	t.Helper()
	in := core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: c, TraceID: tr, SubtaskID: core.SubtaskID(id(n)), LineageID: core.SubtaskLineageID(id(n + 100)), Title: "child", Difficulty: core.DifficultyMedium, Today: date("2026-07-24"), Now: instant(now)}
	histOK(t, k.EditSubtask(in))
	in.LineageID = ""
	in.Title = ""
	in.Difficulty = 0
	return in
}
func TestHistoryCommandsChildUnderOpenParentAndVersions(t *testing.T) {
	k, c, tr := histKernel(t)
	in := histChild(t, k, c, tr, 310, 3)
	in.Kind = core.SubtaskComplete
	in.Now = instant(4)
	histOK(t, k.EditSubtask(in))
	h := history(t, k.Snapshot()).CompletedTaskHierarchies()
	if len(h) != 1 || h[0].ParentCompletion != nil || len(h[0].CompletedChildren) != 1 || h[0].LatestCompletionAt != instant(4) {
		t.Fatal(h)
	}
	histOK(t, k.RenameTaskTitle(core.EditTaskTitleInput{ChainID: c, NewDefinitionID: core.TaskDefinitionID(id(320)), Title: "renamed", Today: in.Today, Now: instant(5)}))
	h = history(t, k.Snapshot()).CompletedTaskHierarchies()
	if h[0].Definition.Title != "renamed" {
		t.Fatal(h)
	}
	histOK(t, k.MarkCompleted(tr, in.Today, instant(6)))
	h = history(t, k.Snapshot()).CompletedTaskHierarchies()
	if h[0].Definition.Title != "renamed" || h[0].ParentCompletion == nil || h[0].CompletedChildren[0].ParentDefinition.Title != "renamed" || h[0].LatestCompletionAt != instant(6) {
		t.Fatal(h)
	}
	if len(history(t, k.Snapshot()).UnfinishedPool()) != 0 {
		t.Fatal("completed chain in unfinished pool")
	}
}
func TestHistoryCommandsDeletionAndAllChildTerminalStates(t *testing.T) {
	k, c, tr := histKernel(t)
	done := histChild(t, k, c, tr, 310, 3)
	done.Kind = core.SubtaskComplete
	done.Now = instant(4)
	histOK(t, k.EditSubtask(done))
	abandoned := histChild(t, k, c, tr, 311, 5)
	abandoned.Kind = core.SubtaskAbandon
	abandoned.Now = instant(6)
	histOK(t, k.EditSubtask(abandoned))
	deleted := histChild(t, k, c, tr, 312, 7)
	deleted.Kind = core.SubtaskDelete
	deleted.CancellationID = core.CancellationID(id(600))
	deleted.Now = instant(8)
	histOK(t, k.EditSubtask(deleted))
	histOK(t, k.MarkCompleted(tr, done.Today, instant(9)))
	h := history(t, k.Snapshot()).CompletedTaskHierarchies()[0]
	if len(h.CompletedChildren) != 1 || len(h.ParentCompletion.Trajectory.SubtaskTrajectories) != 2 {
		t.Fatal(h)
	}
	statuses := map[core.SubtaskStatus]bool{}
	for _, st := range h.ParentCompletion.Trajectory.SubtaskTrajectories {
		for _, r := range st.Records {
			statuses[r.Subtask.Status] = true
		}
	}
	if !statuses[core.SubtaskCompleted] || !statuses[core.SubtaskAbandoned] || statuses[core.SubtaskCancelledDraft] {
		t.Fatal(statuses)
	}
	histOK(t, k.UndoCompleted(tr, done.Today, instant(10)))
	done.Kind = core.SubtaskDelete
	done.CancellationID = core.CancellationID(id(601))
	done.Now = instant(11)
	histOK(t, k.EditSubtask(done))
	if len(history(t, k.Snapshot()).CompletedTaskHierarchies()) != 0 {
		t.Fatal("deleted completed child remains visible")
	}
}
func TestHistoryCommandsReturnReschedulePreservesLineageAndFullTrajectory(t *testing.T) {
	k, c, tr := histKernel(t)
	done := histChild(t, k, c, tr, 310, 3)
	done.Kind = core.SubtaskComplete
	done.Now = instant(4)
	histOK(t, k.EditSubtask(done))
	open := histChild(t, k, c, tr, 311, 5)
	histOK(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: c, TraceID: tr, NewDefinitionID: core.TaskDefinitionID(id(330)), PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{open.SubtaskID: core.PlannedSubtaskID(id(331))}, Today: open.Today, Now: instant(6)}))
	if h := history(t, k.Snapshot()).CompletedTaskHierarchies(); len(h) != 1 || h[0].ParentCompletion != nil || h[0].CompletedChildren[0].ParentTrace.Status != core.TraceReturnedToPool {
		t.Fatal(h)
	}
	next := core.DayTraceID(id(332))
	histOK(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: next, DayID: core.DayID(id(304)), Date: open.Today, Today: open.Today, Now: instant(7), SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{core.PlannedSubtaskID(id(331)): core.SubtaskID(id(333))}}))
	in := core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: c, TraceID: next, SubtaskID: core.SubtaskID(id(333)), Today: open.Today, Now: instant(8)}
	histOK(t, k.EditSubtask(in))
	histOK(t, k.MarkCompleted(next, open.Today, instant(9)))
	h := history(t, k.Snapshot()).CompletedTaskHierarchies()[0]
	if len(h.CompletedChildren) != 2 || len(h.ParentCompletion.Trajectory.Traces) != 2 || len(h.ParentCompletion.Trajectory.ContinuedDates) != 0 {
		t.Fatal(h)
	}
	if h.ParentCompletion.Trajectory.Traces[0].Status != core.TraceReturnedToPool {
		t.Fatal(h)
	}
	found := false
	for _, st := range h.ParentCompletion.Trajectory.SubtaskTrajectories {
		if st.LineageID == core.SubtaskLineageID(id(411)) {
			found = true
			if len(st.Records) != 2 || st.Records[0].Subtask.Status != core.SubtaskCompleted || st.Records[1].Subtask.Status != core.SubtaskPending || st.CompletedDate == nil || len(st.ContinuedDates) != 0 {
				t.Fatal(st)
			}
		}
	}
	if !found {
		t.Fatal("lost lineage")
	}
}
func TestHistoryCommandsSettleAndFutureReturnTombstones(t *testing.T) {
	k, c, tr := histKernel(t)
	_ = histChild(t, k, c, tr, 310, 3)
	tomorrow := date("2026-07-25")
	histOK(t, k.SettleTaskDays(tomorrow, instant(4)))
	p := history(t, k.Snapshot())
	u := p.UnfinishedPool()
	if len(u) != 1 || len(u[0].UnfinishedTraces) != 1 || u[0].ActiveTrace != nil || u[0].IsInTaskPool || u[0].Chain.UpdatedAt != instant(2) || u[0].ActionPlan()[0].Kind != ContinueHistoryTrace {
		t.Fatal(u)
	}
	if k.Snapshot().Subtasks[0].Status != core.SubtaskUnfinished {
		t.Fatal("settle did not form child unfinished fact")
	}
	// A future plan returned to pool is a cancelled draft, unlike a current return.
	k2, e := core.NewTaskKernel(core.TaskGraph{})
	histOK(t, e)
	histOK(t, k2.CreatePoolTask(core.CreatePoolTaskInput{ChainID: c, DefinitionID: core.TaskDefinitionID(id(301)), Title: "future", Now: instant(1)}))
	histOK(t, k2.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: tr, DayID: core.DayID(id(304)), Date: tomorrow, Today: date("2026-07-24"), Now: instant(2)}))
	histOK(t, k2.ReturnToPool(core.ReturnToPoolInput{ChainID: c, TraceID: tr, NewDefinitionID: core.TaskDefinitionID(id(340)), CancellationID: core.CancellationID(id(341)), Today: date("2026-07-24"), Now: instant(3)}))
	histOK(t, k2.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: core.DayTraceID(id(342)), DayID: core.DayID(id(343)), Date: date("2026-07-24"), Today: date("2026-07-24"), Now: instant(4)}))
	histOK(t, k2.MarkCompleted(core.DayTraceID(id(342)), date("2026-07-24"), instant(5)))
	h := history(t, k2.Snapshot()).CompletedTaskHierarchies()[0]
	if len(h.ParentCompletion.Trajectory.Traces) != 1 || h.ParentCompletion.Trajectory.StartDate != date("2026-07-24") {
		t.Fatal(h)
	}
}
func TestHistoryDetachedSnapshot(t *testing.T) {
	k, _, tr := histKernel(t)
	histOK(t, k.MarkCompleted(tr, date("2026-07-24"), instant(3)))
	g := k.Snapshot()
	saved := g.Clone()
	p := history(t, g)
	want := p.CompletedTaskHierarchies()
	*g.Definitions[0].DescriptionText = "mutated input"
	g.Chains[0].NoteEntries[0].Body = "mutated input"
	*g.Traces[0].CompletedAt = instant(90)
	if !reflect.DeepEqual(want, p.CompletedTaskHierarchies()) {
		t.Fatal("input alias")
	}
	h := p.CompletedTaskHierarchies()
	*h[0].ParentCompletion.Trace.CompletedAt = instant(80)
	h[0].Chain.NoteEntries[0].Body = "mutated result"
	*h[0].Definition.DescriptionText = "changed result"
	h[0].ParentCompletion.Trajectory.Traces[0].NoteEntries[0].Body = "changed trace"
	if !reflect.DeepEqual(want, p.CompletedTaskHierarchies()) || !reflect.DeepEqual(saved, k.Snapshot()) {
		t.Fatal("result alias")
	}
}
func TestHistoryValidationRecurrenceAndDefinedOrdering(t *testing.T) {
	g := fixture(core.TraceCompleted, core.TraceCompleted, core.TraceUnfinished)
	g.Definitions[0].Title = "same"
	g.Definitions[1].Title = "same"
	h := history(t, g).CompletedTaskHierarchies()
	if len(h) != 2 || h[0].Chain.ID != g.Chains[0].ID {
		t.Fatal(h)
	}
	slices.Reverse(g.Chains)
	slices.Reverse(g.Definitions)
	slices.Reverse(g.Traces)
	if !reflect.DeepEqual(h, history(t, g).CompletedTaskHierarchies()) {
		t.Fatal("source-defined equal title/clock UUID ordering changed with input permutation")
	}
	g.Chains[0].CycleMembership = &core.TaskCycleMembership{SeriesID: core.TaskCycleSeriesID(id(900)), OccurrenceDate: date("2026-07-24")}
	if len(history(t, g).UnfinishedPool()) != 0 {
		t.Fatal("recurrence leaked")
	}
	if _, e := NewHistoryProjection(g, nil); e == nil {
		t.Fatal("missing comparator accepted")
	}
	g.Definitions = nil
	if _, e := NewHistoryProjection(g, strings.Compare); e == nil {
		t.Fatal("invalid graph accepted")
	}
}

// Structural tests below exercise validator-supported topology, not missing
// Continue/Defer/Abandon commands. Command coverage is explicitly separate.
func historyTopology() core.TaskGraph {
	g := fixture(core.TraceUnfinished)
	a := g.Traces[0]
	a.CreatedAt = instant(1)
	a.ContentUpdatedAt = instant(2)
	a.SettledAt = ptr(instant(2))
	g.Traces[0] = a
	b := a
	b.ID = core.DayTraceID(id(11000))
	b.Date = date("2026-07-25")
	b.Status = core.TraceDeferred
	b.CreatedAt = instant(3)
	b.ContentUpdatedAt = instant(4)
	b.SettledAt = ptr(instant(4))
	b.CarriedFromTraceID = &a.ID
	b.ContinuationSeq = 1
	c := b
	c.ID = core.DayTraceID(id(12000))
	c.Date = date("2026-07-26")
	c.Status = core.TraceCompleted
	c.CreatedAt = instant(5)
	c.ContentUpdatedAt = instant(6)
	c.SettledAt = nil
	c.CompletedAt = ptr(instant(6))
	c.CarriedFromTraceID = &b.ID
	g.Traces = append(g.Traces, b, c)
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: b.Date}, core.Day{ID: core.DayID(id(3)), Date: c.Date})
	for i, t := range g.Traces {
		s := core.Subtask{ID: core.SubtaskID(id(400 + i)), LineageID: core.SubtaskLineageID(id(500)), TraceID: t.ID, Title: "first", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: t.CreatedAt, UpdatedAt: t.ContentUpdatedAt, SettledAt: t.SettledAt}
		switch i {
		case 0:
			s.Status = core.SubtaskUnfinished
		case 1:
			s.Status = core.SubtaskDeferred
		case 2:
			s.Status = core.SubtaskCompleted
			s.CompletedAt = t.CompletedAt
		}
		if i > 0 {
			s.CarriedFromSubtaskID = ptr(core.SubtaskID(id(399 + i)))
			s.Title = "later"
		}
		g.Subtasks = append(g.Subtasks, s)
	}
	return g
}
func TestHistoryStructuralContinuationDeferralAndPermutation(t *testing.T) {
	g := historyTopology()
	p := history(t, g)
	h := p.CompletedTaskHierarchies()
	tr := h[0].ParentCompletion.Trajectory
	if len(tr.Traces) != 3 || tr.StartDate != date("2026-07-24") || tr.CompletedDate != date("2026-07-26") || !reflect.DeepEqual(tr.ContinuedDates, []core.LocalDate{date("2026-07-25")}) {
		t.Fatal(tr)
	}
	st := tr.SubtaskTrajectories[0]
	if st.Title != "first" || len(st.Records) != 3 || st.CompletedDate == nil || *st.CompletedDate != date("2026-07-26") || !reflect.DeepEqual(st.ContinuedDates, tr.ContinuedDates) {
		t.Fatal(st)
	}
	if len(p.UnfinishedPool()) != 0 {
		t.Fatal("completion must exclude chain")
	}
	slices.Reverse(g.Traces)
	slices.Reverse(g.Subtasks)
	slices.Reverse(g.Days)
	if !reflect.DeepEqual(h, history(t, g).CompletedTaskHierarchies()) {
		t.Fatal("source-ordered history changed with permutation")
	}
}
func TestHistoryStructuralUnfinishedActionsAndCurrentDefinition(t *testing.T) {
	g := historyTopology()
	g.Traces[2].Status = core.TracePending
	g.Traces[2].CompletedAt = nil
	g.Subtasks[2].Status = core.SubtaskPending
	g.Subtasks[2].CompletedAt = nil
	p := history(t, g)
	u := p.UnfinishedPool()
	if len(u) != 1 || len(u[0].UnfinishedTraces) != 1 || !u[0].IsContinuedPending() || u[0].ActionPlan()[0].TraceID != g.Traces[2].ID || u[0].ActionPlan()[0].Kind != AbandonHistoryChain {
		t.Fatal(u)
	}
	// Abandoned source and chain are visible, with reactivate rather than continue.
	g = fixture(core.TraceAbandoned)
	g.Chains[0].State = core.ChainAbandoned
	u = history(t, g).UnfinishedPool()
	if len(u) != 1 || u[0].ActionPlan()[0].Kind != ReactivateHistoryChain {
		t.Fatal(u)
	}
	// Current definition is distinct from the historical trace definition.
	old := g.Definitions[0]
	next := old
	next.ID = core.TaskDefinitionID(id(700))
	next.Sequence = 2
	next.Title = "current"
	old.SupersededAt = ptr(instant(0))
	old.SupersededByDefinitionID = &next.ID
	g.Definitions = []core.TaskDefinition{old, next}
	if history(t, g).UnfinishedPool()[0].Definition.ID != next.ID {
		t.Fatal("history definition incorrectly used for unfinished title")
	}
	// Returning a continuation leaves unfinished history and a task-pool link.
	g = fixture(core.TraceUnfinished)
	r := g.Traces[0]
	r.ID = core.DayTraceID(id(13000))
	r.Status = core.TraceReturnedToPool
	r.Date = date("2026-07-25")
	r.CreatedAt = instant(1)
	r.ContentUpdatedAt = instant(2)
	r.SettledAt = ptr(instant(2))
	g.Traces = append(g.Traces, r)
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: r.Date})
	u = history(t, g).UnfinishedPool()
	if !u[0].IsInTaskPool || u[0].ActionPlan()[0].Kind != OpenHistoryTaskPool || len(u[0].ActionPlan()) != 2 {
		t.Fatal(u)
	}
}
func TestHistoryStructuralLatestCompletedLineageAndHistoricalDefinition(t *testing.T) {
	// Duplicate completed lineage records are validator-supported persisted facts;
	// no command for manufacturing such history is claimed.
	g := fixture(core.TraceCompleted)
	a := g.Traces[0]
	b := a
	b.ID = core.DayTraceID(id(11000))
	b.Date = date("2026-07-25")
	b.CompletedAt = ptr(instant(2))
	b.ContentUpdatedAt = instant(2)
	g.Traces = append(g.Traces, b)
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: b.Date})
	for i, tr := range g.Traces {
		g.Subtasks = append(g.Subtasks, core.Subtask{ID: core.SubtaskID(id(400 + i)), LineageID: core.SubtaskLineageID(id(500)), TraceID: tr.ID, Title: "done", Status: core.SubtaskCompleted, Position: 1, Difficulty: core.DifficultySimple, CompletedAt: ptr(instant(0))})
	}
	old := g.Definitions[0]
	next := old
	next.ID = core.TaskDefinitionID(id(700))
	next.Sequence = 2
	next.Title = "new current"
	old.SupersededAt = ptr(instant(0))
	old.SupersededByDefinitionID = &next.ID
	g.Definitions = []core.TaskDefinition{old, next}
	h := history(t, g).CompletedTaskHierarchies()[0]
	if h.ParentCompletion.Trace.ID != b.ID || h.Definition.ID != old.ID || len(h.CompletedChildren) != 1 || h.CompletedChildren[0].Subtask.ID != core.SubtaskID(id(401)) {
		t.Fatal(h)
	}
	// Equal child clocks explicitly use max UUID, unlike parent-completion ties.
	slices.Reverse(g.Subtasks)
	if h2 := history(t, g).CompletedTaskHierarchies()[0]; !reflect.DeepEqual(h, h2) {
		t.Fatal("child tie must use source UUID max")
	}
}
func TestHistorySourceComparatorEqualityDoesNotInventUUID(t *testing.T) {
	g := historyTopology()
	a, b := g.Traces[0], g.Traces[2]
	a.Status = core.TraceCompleted
	a.SettledAt = nil
	a.CompletedAt = b.CompletedAt
	a.ContentUpdatedAt = *a.CompletedAt
	// Isolate two unrelated completions of one chain, without carryover edges.
	b.CarriedFromTraceID = nil
	b.ContinuationSeq = 0
	g.Traces = []core.DayTrace{b, a}
	g.Subtasks = nil
	h := history(t, g).CompletedTaskHierarchies()[0]
	if h.ParentCompletion.Trace.ID != b.ID {
		t.Fatal("source max tie should preserve encountered candidate")
	}
	slices.Reverse(g.Traces)
	if history(t, g).CompletedTaskHierarchies()[0].ParentCompletion.Trace.ID != a.ID {
		t.Fatal("invented UUID parent tie-break")
	}
}

func TestHistoryUnfinishedPresentationMetadataAndIsolation(t *testing.T) {
	g := fixture(core.TraceUnfinished)
	p := history(t, g)
	want := p.UnfinishedPool()
	got := p.UnfinishedPool()
	*got[0].UnfinishedTraces[0].SettledAt = instant(50)
	if !reflect.DeepEqual(want, p.UnfinishedPool()) {
		t.Fatal("unfinished result aliases projection")
	}
	last := want[0].LatestUnfinishedTrace()
	*last.SettledAt = instant(30)
	if !reflect.DeepEqual(want, p.UnfinishedPool()) {
		t.Fatal("latest trace accessor aliases item")
	}
	if want[0].IsAbandoned() || want[0].ContinuationCount() != 0 {
		t.Fatal(want)
	}
	// Source UI arithmetic is tested directly; no fake command sequence.
	item := UnfinishedPoolItem{UnfinishedTraces: []core.DayTrace{{ContinuationSeq: 2, Status: core.TraceUnfinished}}, ActiveTrace: &core.DayTrace{ContinuationSeq: 4}}
	if item.ContinuationCount() != 3 {
		t.Fatal(item.ContinuationCount())
	}
	item.UnfinishedTraces[0].Status = core.TraceAbandoned
	if !item.IsAbandoned() {
		t.Fatal("latest abandoned status lost")
	}
}
func TestHistoryInjectedTitleComparatorOrdersEqualClocks(t *testing.T) {
	g := fixture(core.TraceCompleted, core.TraceCompleted)
	g.Definitions[0].Title = "a"
	g.Definitions[1].Title = "b"
	p, e := NewHistoryProjection(g, func(a, b string) int { return strings.Compare(b, a) })
	histOK(t, e)
	if p.CompletedTaskHierarchies()[0].Definition.Title != "b" {
		t.Fatal("ignored title comparator")
	}
	g = fixture(core.TraceUnfinished, core.TraceUnfinished)
	g.Definitions[0].Title = "a"
	g.Definitions[1].Title = "b"
	p, e = NewHistoryProjection(g, func(a, b string) int { return strings.Compare(b, a) })
	histOK(t, e)
	if p.UnfinishedPool()[0].Definition.Title != "b" {
		t.Fatal("ignored unfinished title comparator")
	}
}
