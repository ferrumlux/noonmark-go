// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func identity(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func instant(n float64) Instant {
	v, err := InstantFromSeconds(n)
	if err != nil {
		panic(err)
	}
	return v
}
func ptr[T any](v T) *T { return &v }
func date(s string) LocalDate {
	d, e := ParseLocalDate(s)
	if e != nil {
		panic(e)
	}
	return d
}
func baseGraph() TaskGraph {
	now := instant(100)
	today := date("2026-11-05")
	return TaskGraph{
		Days:        []Day{{ID: DayID(identity(1)), Date: today, CreatedAt: now, UpdatedAt: now}},
		Chains:      []TaskChain{{ID: TaskChainID(identity(2)), State: ChainActive, CreatedAt: now, UpdatedAt: now}},
		Definitions: []TaskDefinition{{ID: TaskDefinitionID(identity(3)), ChainID: TaskChainID(identity(2)), Sequence: 1, Title: "测试任务", CreatedAt: now, ContentUpdatedAt: now}},
		Traces:      []DayTrace{{ID: DayTraceID(identity(4)), ChainID: TaskChainID(identity(2)), DefinitionID: TaskDefinitionID(identity(3)), Date: today, Status: TracePending, Priority: 1, CreatedAt: now, ContentUpdatedAt: now}},
	}
}
func child() Subtask {
	return Subtask{ID: SubtaskID(identity(10)), LineageID: SubtaskLineageID(identity(11)), TraceID: DayTraceID(identity(4)), Title: "子任务", Status: SubtaskPending, Difficulty: DifficultyMedium, Position: 1, CreatedAt: instant(100), UpdatedAt: instant(100)}
}
func assertValid(t *testing.T, g TaskGraph) {
	t.Helper()
	if err := ValidateTaskGraph(g); err != nil {
		t.Fatalf("unexpected validation error: %#v", err)
	}
}
func assertIssue(t *testing.T, g TaskGraph, code string) {
	t.Helper()
	err := ValidateTaskGraph(g)
	var v *ValidationError
	if !errors.As(err, &v) || !v.Has(code) {
		t.Fatalf("want %s, got %#v", code, err)
	}
}
func kernel(t *testing.T, g TaskGraph) *TaskKernel {
	t.Helper()
	k, e := NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func unchanged(t *testing.T, k *TaskKernel, want error, op func() error) {
	t.Helper()
	before := k.Snapshot()
	err := op()
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("rejected mutation changed graph")
	}
}

func TestInstantPreservesSubmillisecondAndLargeUnsignedBits(t *testing.T) {
	for _, seconds := range []float64{0, math.Copysign(0, -1), -1, math.SmallestNonzeroFloat64, 100.00000000000001, 815590081.1234567, math.MaxFloat64} {
		original := instant(seconds)
		encoded, e := json.Marshal(original)
		if e != nil {
			t.Fatal(e)
		}
		var decoded Instant
		if e = json.Unmarshal(encoded, &decoded); e != nil {
			t.Fatal(e)
		}
		if decoded != original {
			t.Fatalf("bits changed: %016X -> %016X", original, decoded)
		}
	}
}
func TestInstantRejectsHostileJSONWithoutChangingReceiver(t *testing.T) {
	for _, input := range []string{"null", "  null  ", "-1", "1.2", "1e3", "\"123\"", "18446744073709551616", "9218868437227405312", "9221120237041090560"} {
		t.Run(input, func(t *testing.T) {
			v := instant(42)
			old := v
			if e := json.Unmarshal([]byte(input), &v); e == nil {
				t.Fatalf("accepted %s", input)
			}
			if v != old {
				t.Fatal("failed decode modified receiver")
			}
		})
	}
}
func TestMutationClockNextUpAndBoundaries(t *testing.T) {
	old := instant(500)
	next, e := NextMutationInstant(instant(1), old)
	if e != nil {
		t.Fatal(e)
	}
	if next.Seconds() != math.Nextafter(500, math.Inf(1)) {
		t.Fatal("not one representable step")
	}
	equal, e := NextMutationInstant(old, old)
	if e != nil || equal != next {
		t.Fatal("equal clock did not advance")
	}
	fresh, e := NextMutationInstant(instant(600), old)
	if e != nil || fresh != instant(600) {
		t.Fatal("future wall clock not retained")
	}
	for _, bad := range []Instant{Instant(math.Float64bits(math.NaN())), Instant(math.Float64bits(math.Inf(1)))} {
		if _, e := NextMutationInstant(bad); e == nil {
			t.Fatal("bad reference accepted")
		}
		if _, e := NextMutationInstant(old, bad); e == nil {
			t.Fatal("bad persisted clock accepted")
		}
	}
	if _, e := NextMutationInstant(instant(0), instant(math.MaxFloat64)); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestCivilDatesLeapDaysAndBounds(t *testing.T) {
	for _, bad := range []string{"2026-02-29", "2024-02-30", "2026-2-01", "0000-01-01", "2026-13-01", "2026-11-00", "2026-11-05Z", "202A-11-05"} {
		if _, e := ParseLocalDate(bad); e == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{{"2024-02-28", 1, "2024-02-29"}, {"2024-02-29", 1, "2024-03-01"}, {"2026-12-31", 1, "2027-01-01"}, {"2026-03-08", 1, "2026-03-09"}, {"2026-11-01", -1, "2026-10-31"}} {
		got, e := date(tc.in).AddDays(tc.n)
		if e != nil || got.String() != tc.want {
			t.Errorf("date result %v %v", got, e)
		}
	}
	if _, e := date("9999-12-31").AddDays(1); e == nil {
		t.Fatal("overflow accepted")
	}
	if _, e := date("0001-01-01").AddDays(-1); e == nil {
		t.Fatal("underflow accepted")
	}
}
func TestIdentitiesAndStatuses(t *testing.T) {
	a, e := NewIdentity()
	if e != nil || !validID(a) {
		t.Fatalf("invalid generated UUID %q %v", a, e)
	}
	b, e := NewIdentity()
	if e != nil || a == b {
		t.Fatal("UUID collision or generation failure")
	}
	for _, s := range []TraceStatus{TracePending, TraceCompleted, TraceUnfinished, TraceDeferred, TraceChanged, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned} {
		if !s.Valid() {
			t.Fatal(s)
		}
		if s.FormsDayHistory() != (s != TraceCancelledDraft) {
			t.Fatal(s)
		}
	}
	if TraceStatus("corrupt").FormsDayHistory() || TraceStatus("corrupt").VisibleInDayTodo() {
		t.Fatal("unknown status exposed")
	}
	for _, s := range []TraceStatus{TraceChanged, TraceReturnedToPool, TraceCancelledDraft} {
		if s.VisibleInDayTodo() {
			t.Fatal("internal/movement row visible")
		}
	}
}

func TestTaskGraphRejectsMalformedFacts(t *testing.T) {
	cases := []struct {
		name, code string
		change     func(*TaskGraph)
	}{
		{"duplicate chain", "duplicateIdentity", func(g *TaskGraph) { g.Chains = append(g.Chains, g.Chains[0]) }},
		{"duplicate date", "duplicateDayDate", func(g *TaskGraph) { d := g.Days[0]; d.ID = DayID(identity(20)); g.Days = append(g.Days, d) }},
		{"identity", "invalidIdentity", func(g *TaskGraph) { g.Chains[0].ID = "not-uuid" }},
		{"civil date", "invalidLocalDate", func(g *TaskGraph) { g.Days[0].Date = LocalDate{2026, 2, 30} }},
		{"chain state", "invalidChainState", func(g *TaskGraph) { g.Chains[0].State = "invalid" }},
		{"chain clock", "invalidChainClock", func(g *TaskGraph) { g.Chains[0].UpdatedAt = instant(99) }},
		{"day locked clock", "invalidDayClock", func(g *TaskGraph) { g.Days[0].LockedAt = ptr(instant(101)) }},
		{"missing chain", "missingDefinitionChain", func(g *TaskGraph) { g.Chains = nil }},
		{"missing definition", "missingDefinitionForChain", func(g *TaskGraph) { g.Definitions = nil }},
		{"definition sequence", "invalidDefinitionSequence", func(g *TaskGraph) { g.Definitions[0].Sequence = 0 }},
		{"definition title", "invalidDefinitionTitle", func(g *TaskGraph) { g.Definitions[0].Title = " x " }},
		{"definition clock", "invalidDefinitionClock", func(g *TaskGraph) { g.Definitions[0].ContentUpdatedAt = instant(99) }},
		{"supersession", "incompleteSupersession", func(g *TaskGraph) { g.Definitions[0].SupersededAt = ptr(instant(100)) }},
		{"self successor", "selfSuccessor", func(g *TaskGraph) { g.Definitions[0].SupersededByDefinitionID = ptr(g.Definitions[0].ID) }},
		{"missing successor", "missingDefinitionSuccessor", func(g *TaskGraph) { g.Definitions[0].SupersededByDefinitionID = ptr(TaskDefinitionID(identity(90))) }},
		{"two current definitions", "invalidCurrentDefinitionCount", func(g *TaskGraph) {
			d := g.Definitions[0]
			d.ID = TaskDefinitionID(identity(20))
			d.Sequence = 2
			g.Definitions = append(g.Definitions, d)
		}},
		{"duplicate sequence", "duplicateDefinitionSequence", func(g *TaskGraph) {
			d := g.Definitions[0]
			d.ID = TaskDefinitionID(identity(20))
			g.Definitions = append(g.Definitions, d)
		}},
		{"missing day", "missingTraceDay", func(g *TaskGraph) { g.Days = nil }},
		{"trace wrong definition", "missingTraceDefinition", func(g *TaskGraph) { g.Traces[0].DefinitionID = TaskDefinitionID(identity(99)) }},
		{"trace clock", "invalidTraceClock", func(g *TaskGraph) { g.Traces[0].ContentUpdatedAt = Instant(math.Float64bits(math.NaN())) }},
		{"trace unknown status", "invalidTraceStatusFacts", func(g *TaskGraph) { g.Traces[0].Status = "other" }},
		{"pending completion", "invalidTraceStatusFacts", func(g *TaskGraph) { g.Traces[0].CompletedAt = ptr(instant(100)) }},
		{"progress overflow", "invalidTraceProgress", func(g *TaskGraph) { g.Traces[0].ManualProgressPercent = ptr(101) }},
		{"unlinked nonroot", "invalidTraceSequence", func(g *TaskGraph) { g.Traces[0].ContinuationSeq = 1 }},
		{"pin zero", "invalidTraceStatusFacts", func(g *TaskGraph) { g.Traces[0].PinOrder = ptr(0) }},
		{"two pending", "duplicateActiveTrace", func(g *TaskGraph) {
			x := g.Traces[0]
			x.ID = DayTraceID(identity(50))
			x.Priority = 2
			g.Traces = append(g.Traces, x)
		}},
		{"duplicate priority", "duplicateVisiblePriority", func(g *TaskGraph) { x := g.Traces[0]; x.ID = DayTraceID(identity(50)); g.Traces = append(g.Traces, x) }},
		{"cancel witness", "invalidTraceStatusFacts", func(g *TaskGraph) {
			g.Traces[0].Status = TraceCancelledDraft
			g.Traces[0].SettledAt = ptr(instant(100))
		}},
		{"deferred needs target", "missingTraceContinuationTarget", func(g *TaskGraph) { g.Traces[0].Status = TraceDeferred; g.Traces[0].SettledAt = ptr(instant(100)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { g := baseGraph(); tc.change(&g); assertIssue(t, g, tc.code) })
	}
}
func TestEmptyExistingTitleAndDefinitionRestorationWitness(t *testing.T) {
	g := baseGraph()
	g.Definitions[0].Title = ""
	assertValid(t, g)
	d2 := g.Definitions[0]
	d2.ID = TaskDefinitionID(identity(20))
	d2.Sequence = 2
	d2.SupersededAt = ptr(instant(100))
	d2.SupersededByDefinitionID = ptr(g.Definitions[0].ID)
	g.Definitions[0].SupersededByDefinitionID = ptr(d2.ID)
	g.Definitions = append(g.Definitions, d2)
	assertValid(t, g) // restored current pointer is a witness, not an active edge.
	g.Definitions[0].SupersededAt = ptr(instant(100))
	assertIssue(t, g, "definitionSuccessorCycle")
}
func TestPlannedSubtaskContracts(t *testing.T) {
	valid := PlannedSubtask{ID: PlannedSubtaskID(identity(20)), LineageID: SubtaskLineageID(identity(21)), Title: "计划", Difficulty: DifficultyHard, Position: 1, CreatedAt: instant(100)}
	g := baseGraph()
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{valid}
	assertValid(t, g)
	changes := []func(*PlannedSubtask){func(p *PlannedSubtask) { p.Position = 2 }, func(p *PlannedSubtask) { p.Title = " " }, func(p *PlannedSubtask) { p.CreatedAt = instant(99) }, func(p *PlannedSubtask) { p.CreatedAt = instant(101) }, func(p *PlannedSubtask) { p.Difficulty = 4 }}
	for i, change := range changes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			c := g.Clone()
			change(&c.Definitions[0].PlannedSubtasks[0])
			assertIssue(t, c, "invalidPlannedSubtasks")
		})
	}
	g.Definitions[0].PlannedSubtasks = append(g.Definitions[0].PlannedSubtasks, valid)
	assertIssue(t, g, "invalidPlannedSubtasks")
}
func TestStructuredNoteIdentityClockAndTombstone(t *testing.T) {
	note := TaskNoteEntry{ID: TaskNoteEntryID(identity(10)), Body: "笔记", CreatedAt: instant(100), UpdatedAt: instant(100)}
	g := baseGraph()
	g.Chains[0].NoteEntries = []TaskNoteEntry{note}
	assertValid(t, g)
	for _, tc := range []struct {
		code   string
		change func(*TaskNoteEntry)
	}{
		{"invalidNoteBody", func(n *TaskNoteEntry) { n.Body = "\n text " }},
		{"invalidNoteClock", func(n *TaskNoteEntry) { n.UpdatedAt = instant(99) }},
		{"invalidNoteTombstone", func(n *TaskNoteEntry) { n.DeletedAt = ptr(instant(100)) }},
	} {
		c := g.Clone()
		tc.change(&c.Chains[0].NoteEntries[0])
		assertIssue(t, c, tc.code)
	}
	g.Chains[0].NoteEntries[0].Body = ""
	g.Chains[0].NoteEntries[0].DeletedAt = ptr(instant(100))
	assertValid(t, g)
	g.Chains[0].NoteEntries = append(g.Chains[0].NoteEntries, g.Chains[0].NoteEntries[0])
	assertIssue(t, g, "duplicateNoteIdentity")
}

func continuedGraph(sourceStatus TraceStatus) TaskGraph {
	g := baseGraph()
	g.Traces[0].Status = sourceStatus
	g.Traces[0].SettledAt = ptr(instant(100))
	nextDate := date("2026-11-06")
	g.Days = append(g.Days, Day{ID: DayID(identity(20)), Date: nextDate, CreatedAt: instant(101), UpdatedAt: instant(101)})
	next := g.Traces[0]
	next.ID = DayTraceID(identity(21))
	next.Date = nextDate
	next.Status = TracePending
	next.SettledAt = nil
	next.CreatedAt = instant(101)
	next.ContentUpdatedAt = instant(101)
	next.CarriedFromTraceID = ptr(g.Traces[0].ID)
	if sourceStatus == TraceUnfinished {
		next.ContinuationSeq = 1
	}
	g.Traces = append(g.Traces, next)
	return g
}
func TestDeferralAndContinuationAreDistinct(t *testing.T) {
	for _, status := range []TraceStatus{TraceDeferred, TraceUnfinished} {
		g := continuedGraph(status)
		assertValid(t, g)
		g.Traces[1].ContinuationSeq++
		assertIssue(t, g, "invalidTraceContinuation")
	}
	g := continuedGraph(TraceUnfinished)
	g.Traces[1].Date = g.Traces[0].Date
	g.Traces[1].Priority = 2
	assertIssue(t, g, "invalidTraceContinuation")
	g = continuedGraph(TraceUnfinished)
	g.Traces[1].CarriedFromTraceID = ptr(DayTraceID(identity(99)))
	assertIssue(t, g, "missingTraceContinuationSource")
	g = continuedGraph(TraceUnfinished)
	x := g.Traces[1]
	x.ID = DayTraceID(identity(22))
	x.Priority = 2
	g.Traces = append(g.Traces, x)
	assertIssue(t, g, "reusedTraceContinuationSource")
}
func TestCancelledUndoAndReservedWitness(t *testing.T) {
	g := continuedGraph(TraceDeferred)
	g.Traces[0].Status = TracePending
	g.Traces[0].SettledAt = nil
	g.Traces[1].Status = TraceCancelledDraft
	g.Traces[1].DraftCancellationID = ptr(CancellationID(identity(90)))
	g.Traces[1].DraftCancelledOn = ptr(g.Traces[1].Date)
	g.Traces[1].SettledAt = ptr(instant(101))
	assertValid(t, g)
	g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(91)))
	assertValid(t, g) // reactivation retains its witness
	g.Traces[0].DraftCancellationID = g.Traces[1].DraftCancellationID
	assertIssue(t, g, "reusedDraftCancellationIdentity")
	g = continuedGraph(TraceUnfinished)
	g.Traces[0].Status = TraceCancelledDraft
	g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(90)))
	g.Traces[0].DraftCancelledOn = ptr(g.Traces[0].Date)
	assertIssue(t, g, "invalidTraceContinuation")
}
func TestChangedTraceMustPointToNewChainOnSameDate(t *testing.T) {
	g := baseGraph()
	c := g.Chains[0]
	c.ID = TaskChainID(identity(20))
	g.Chains = append(g.Chains, c)
	d := g.Definitions[0]
	d.ID = TaskDefinitionID(identity(21))
	d.ChainID = c.ID
	g.Definitions = append(g.Definitions, d)
	target := g.Traces[0]
	target.ID = DayTraceID(identity(22))
	target.ChainID = c.ID
	target.DefinitionID = d.ID
	target.Priority = 2
	g.Traces = append(g.Traces, target)
	g.Traces[0].Status = TraceChanged
	g.Traces[0].SettledAt = ptr(instant(100))
	g.Traces[0].ChangedToTraceID = ptr(target.ID)
	assertValid(t, g)
	g.Traces[1].ChainID = g.Traces[0].ChainID
	g.Traces[1].DefinitionID = g.Traces[0].DefinitionID
	assertIssue(t, g, "invalidChangedTraceLink")
}
func TestSubtaskFactsAndTopology(t *testing.T) {
	g := baseGraph()
	g.Subtasks = []Subtask{child()}
	assertValid(t, g)
	for _, tc := range []struct {
		code   string
		change func(*TaskGraph)
	}{
		{"invalidSubtaskFacts", func(g *TaskGraph) { g.Subtasks[0].Status = SubtaskCompleted }},
		{"missingSubtaskParent", func(g *TaskGraph) { g.Subtasks[0].TraceID = DayTraceID(identity(99)) }},
		{"noncontiguousSubtaskPosition", func(g *TaskGraph) { g.Subtasks[0].Position = 2 }},
		{"duplicateSubtaskPosition", func(g *TaskGraph) {
			s := g.Subtasks[0]
			s.ID = SubtaskID(identity(30))
			s.LineageID = SubtaskLineageID(identity(31))
			g.Subtasks = append(g.Subtasks, s)
		}},
		{"duplicateSubtaskLineage", func(g *TaskGraph) {
			s := g.Subtasks[0]
			s.ID = SubtaskID(identity(30))
			s.Position = 2
			g.Subtasks = append(g.Subtasks, s)
		}},
		{"missingSubtaskContinuationSource", func(g *TaskGraph) { g.Subtasks[0].CarriedFromSubtaskID = ptr(SubtaskID(identity(99))) }},
		{"missingSubtaskContinuationTarget", func(g *TaskGraph) {
			g.Subtasks[0].Status = SubtaskDeferred
			g.Subtasks[0].SettledAt = ptr(instant(100))
		}},
	} {
		c := g.Clone()
		tc.change(&c)
		assertIssue(t, c, tc.code)
	}
}
func TestSubtaskContinuationPreservesLineage(t *testing.T) {
	g := continuedGraph(TraceUnfinished)
	s := child()
	s.Status = SubtaskUnfinished
	s.SettledAt = ptr(instant(100))
	next := s
	next.ID = SubtaskID(identity(31))
	next.TraceID = g.Traces[1].ID
	next.Status = SubtaskPending
	next.SettledAt = nil
	next.CarriedFromSubtaskID = ptr(s.ID)
	next.CreatedAt = instant(101)
	next.UpdatedAt = instant(101)
	g.Subtasks = []Subtask{s, next}
	assertValid(t, g)
	g.Subtasks[1].LineageID = SubtaskLineageID(identity(90))
	assertIssue(t, g, "invalidSubtaskContinuation")
}
func TestCycleDetectionIsIterative(t *testing.T) {
	edges := map[string][]string{}
	for i := 0; i < 30000; i++ {
		edges[fmt.Sprint(i)] = []string{fmt.Sprint(i + 1)}
	}
	if _, ok := firstCycle(edges); ok {
		t.Fatal("false cycle")
	}
	edges["30000"] = []string{"0"}
	if _, ok := firstCycle(edges); !ok {
		t.Fatal("missed cycle")
	}
}

func TestPoolScheduleCompleteUndoAndRollover(t *testing.T) {
	k := kernel(t, TaskGraph{})
	today := date("2026-11-05")
	if e := k.CreatePoolTask(CreatePoolTaskInput{ChainID: TaskChainID(identity(2)), DefinitionID: TaskDefinitionID(identity(3)), Title: "  今日任务  ", Now: instant(100)}); e != nil {
		t.Fatal(e)
	}
	if k.Snapshot().Definitions[0].Title != "今日任务" {
		t.Fatal("title not normalized")
	}
	if e := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: TaskChainID(identity(2)), TraceID: DayTraceID(identity(4)), DayID: DayID(identity(1)), Date: today, Today: today, Now: instant(101)}); e != nil {
		t.Fatal(e)
	}
	if e := k.MarkCompleted(DayTraceID(identity(4)), today, instant(102)); e != nil {
		t.Fatal(e)
	}
	if e := k.UndoCompleted(DayTraceID(identity(4)), today, instant(103)); e != nil {
		t.Fatal(e)
	}
	if e := k.SettleTaskDays(date("2026-11-06"), instant(104)); e != nil {
		t.Fatal(e)
	}
	g := k.Snapshot()
	if g.Traces[0].Status != TraceUnfinished || g.Days[0].LockedAt == nil || g.Traces[0].CompletedAt != nil {
		t.Fatal("incorrect settled state")
	}
	unchanged(t, k, ErrImmutableHistory, func() error { return k.MarkCompleted(g.Traces[0].ID, today, instant(105)) })
	before := k.Snapshot()
	if e := k.SettleTaskDays(date("2026-11-06"), instant(106)); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("repeated settlement changed locked facts")
	}
}
func TestCreationAndScheduleRejectAtomically(t *testing.T) {
	k := kernel(t, TaskGraph{})
	in := CreatePoolTaskInput{ChainID: TaskChainID(identity(2)), DefinitionID: TaskDefinitionID(identity(3)), Title: " ", Now: instant(100)}
	unchanged(t, k, ErrInvalidTitle, func() error { return k.CreatePoolTask(in) })
	in.Title = "任务"
	if e := k.CreatePoolTask(in); e != nil {
		t.Fatal(e)
	}
	unchanged(t, k, ErrInvalidInput, func() error { return k.CreatePoolTask(in) })
	sched := ScheduleFromPoolInput{ChainID: in.ChainID, TraceID: DayTraceID(identity(4)), DayID: DayID(identity(1)), Date: date("2026-11-04"), Today: date("2026-11-05"), Now: instant(101)}
	unchanged(t, k, ErrInvalidTransition, func() error { return k.ScheduleFromPool(sched) })
	sched.Date = sched.Today
	if e := k.ScheduleFromPool(sched); e != nil {
		t.Fatal(e)
	}
	sched.TraceID = DayTraceID(identity(5))
	unchanged(t, k, ErrInvalidTransition, func() error { return k.ScheduleFromPool(sched) })
}
func TestScheduleCopiesPlannedSubtasksAndActiveNotes(t *testing.T) {
	g := baseGraph()
	g.Days = nil
	g.Traces = nil
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(identity(20)), LineageID: SubtaskLineageID(identity(21)), Title: "计划子项", Difficulty: DifficultyHard, Position: 1, CreatedAt: instant(100)}}
	g.Chains[0].NoteEntries = []TaskNoteEntry{{ID: TaskNoteEntryID(identity(30)), Body: "有效", CreatedAt: instant(100), UpdatedAt: instant(100)}, {ID: TaskNoteEntryID(identity(31)), Body: "", CreatedAt: instant(100), UpdatedAt: instant(100), DeletedAt: ptr(instant(100))}}
	k := kernel(t, g)
	s := ScheduleFromPoolInput{ChainID: g.Chains[0].ID, TraceID: DayTraceID(identity(4)), DayID: DayID(identity(1)), Date: date("2026-11-05"), Today: date("2026-11-05"), Now: instant(101)}
	unchanged(t, k, ErrInvalidInput, func() error { return k.ScheduleFromPool(s) })
	s.SubtaskIDs = map[PlannedSubtaskID]SubtaskID{g.Definitions[0].PlannedSubtasks[0].ID: SubtaskID(identity(40))}
	if e := k.ScheduleFromPool(s); e != nil {
		t.Fatal(e)
	}
	result := k.Snapshot()
	if len(result.Traces[0].NoteEntries) != 1 || len(result.Subtasks) != 1 || result.Subtasks[0].LineageID != SubtaskLineageID(identity(21)) {
		t.Fatal("planned context lost")
	}
	unchanged(t, k, ErrOpenSubtasksPreventCompletion, func() error { return k.MarkCompleted(s.TraceID, s.Today, instant(102)) })
}
func TestCompletionCapabilityErrors(t *testing.T) {
	g := baseGraph()
	k := kernel(t, g)
	id := g.Traces[0].ID
	unchanged(t, k, ErrFuturePlanCannotComplete, func() error { return k.MarkCompleted(id, date("2026-11-04"), instant(101)) })
	unchanged(t, k, ErrImmutableHistory, func() error { return k.MarkCompleted(id, date("2026-11-06"), instant(101)) })
	unchanged(t, k, ErrInvalidTransition, func() error { return k.UndoCompleted(id, g.Traces[0].Date, instant(101)) })
	unchanged(t, k, ErrInvalidClock, func() error { return k.MarkCompleted(id, g.Traces[0].Date, instant(99)) })
	if e := k.MarkCompleted(id, g.Traces[0].Date, instant(101)); e != nil {
		t.Fatal(e)
	}
	unchanged(t, k, ErrHistoricalCompletionCannotBeUndone, func() error { return k.UndoCompleted(id, date("2026-11-06"), instant(102)) })
}
func TestCompletionChainClockFailureDoesNotPartiallyMutateTrace(t *testing.T) {
	g := baseGraph()
	g.Chains[0].UpdatedAt = instant(200)
	k := kernel(t, g)
	unchanged(t, k, ErrInvalidClock, func() error { return k.MarkCompleted(g.Traces[0].ID, g.Traces[0].Date, instant(150)) })
}
func TestSettlementSubtaskFailureIsAtomicAcrossDays(t *testing.T) {
	g := baseGraph()
	g.Subtasks = []Subtask{child()}
	g.Subtasks[0].UpdatedAt = instant(300)
	k := kernel(t, g)
	unchanged(t, k, ErrInvalidClock, func() error { return k.SettleTaskDays(date("2026-11-06"), instant(200)) })
	if e := k.SettleTaskDays(date("2026-11-06"), instant(301)); e != nil {
		t.Fatal(e)
	}
	if k.Snapshot().Subtasks[0].Status != SubtaskUnfinished {
		t.Fatal("pending child not settled")
	}
}
func TestKernelRejectsRecurringParentsRatherThanDroppingThem(t *testing.T) {
	g := baseGraph()
	g.Chains[0].CycleMembership = &TaskCycleMembership{SeriesID: TaskCycleSeriesID(identity(90)), OccurrenceDate: g.Traces[0].Date}
	assertValid(t, g)
	if _, e := NewTaskKernel(g); !errors.Is(e, ErrUnsupportedRecurring) {
		t.Fatal(e)
	}
}
func TestSnapshotIsDeeplyDetached(t *testing.T) {
	g := baseGraph()
	g.Traces[0].PinOrder = ptr(1)
	g.Traces[0].DescriptionText = ptr("描述")
	g.Chains[0].NoteEntries = []TaskNoteEntry{{ID: TaskNoteEntryID(identity(10)), Body: "附言", CreatedAt: instant(100), UpdatedAt: instant(100)}}
	k := kernel(t, g)
	expected := k.Snapshot()
	*g.Traces[0].PinOrder = 99
	g.Chains[0].NoteEntries[0].Body = "外部修改"
	snap := k.Snapshot()
	*snap.Traces[0].DescriptionText = "快照修改"
	snap.Chains[0].NoteEntries[0].Body = "改"
	if !reflect.DeepEqual(k.Snapshot(), expected) {
		t.Fatal("caller mutated kernel through aliases")
	}
}
func TestConcurrentCreationAndReads(t *testing.T) {
	k := kernel(t, TaskGraph{})
	var wg sync.WaitGroup
	for i := 1; i <= 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := k.CreatePoolTask(CreatePoolTaskInput{ChainID: TaskChainID(identity(100 + i)), DefinitionID: TaskDefinitionID(identity(200 + i)), Title: fmt.Sprint("任务", i), Now: instant(float64(i))})
			if e != nil {
				t.Error(e)
			}
			_ = k.Snapshot()
		}(i)
	}
	wg.Wait()
	if len(k.Snapshot().Chains) != 40 {
		t.Fatal("concurrent create lost facts")
	}
	assertValid(t, k.Snapshot())
}
func TestDayTodoFiltersSupersededDeferralAndKeepsMovementFacts(t *testing.T) {
	g := continuedGraph(TraceDeferred)
	g.Traces[1].Status = TraceReturnedToPool
	g.Traces[1].SettledAt = ptr(instant(101))
	replacement := g.Traces[0]
	replacement.ID = DayTraceID(identity(50))
	replacement.Status = TracePending
	replacement.SettledAt = nil
	replacement.Priority = 2
	replacement.CreatedAt = instant(102)
	replacement.ContentUpdatedAt = instant(102)
	g.Traces = append(g.Traces, replacement)
	k := kernel(t, g)
	rows := k.DayTodo(g.Traces[0].Date)
	if len(rows) != 1 || rows[0].ID != replacement.ID {
		t.Fatalf("superseded source still visible: %#v", rows)
	}
	if len(k.Snapshot().Traces) != 3 {
		t.Fatal("projection destroyed source facts")
	}
}
func TestValidationErrorDoesNotExposeTaskText(t *testing.T) {
	g := baseGraph()
	g.Definitions[0].Title = " private-title "
	e := ValidateTaskGraph(g)
	if e == nil || strings.Contains(e.Error(), "private-title") {
		t.Fatal("error leaks title")
	}
}

func TestTraceStatusFactMatrix(t *testing.T) {
	for _, status := range []TraceStatus{TracePending, TraceCompleted, TraceUnfinished, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned} {
		t.Run(string(status), func(t *testing.T) {
			g := baseGraph()
			x := &g.Traces[0]
			x.Status = status
			switch status {
			case TraceCompleted:
				x.CompletedAt = ptr(instant(100))
			case TraceUnfinished, TraceReturnedToPool, TraceAbandoned:
				x.SettledAt = ptr(instant(100))
			case TraceCancelledDraft:
				x.SettledAt = ptr(instant(100))
				x.DraftCancellationID = ptr(CancellationID(identity(90)))
				x.DraftCancelledOn = ptr(x.Date)
			}
			assertValid(t, g)
			if status != TracePending {
				x.PinOrder = ptr(1)
				assertIssue(t, g, "invalidTraceStatusFacts")
			}
		})
	}
	g := baseGraph()
	g.Traces[0].Status = TraceCancelledDraft
	g.Traces[0].SettledAt = ptr(instant(100))
	g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(90)))
	g.Traces[0].DraftCancelledOn = ptr(date("2026-11-06"))
	assertIssue(t, g, "invalidTraceStatusFacts")
}
func TestSubtaskStatusFactMatrix(t *testing.T) {
	for _, status := range []SubtaskStatus{SubtaskPending, SubtaskCompleted, SubtaskUnfinished, SubtaskAbandoned, SubtaskCancelledDraft} {
		t.Run(string(status), func(t *testing.T) {
			g := baseGraph()
			s := child()
			s.Status = status
			switch status {
			case SubtaskCompleted:
				s.CompletedAt = ptr(instant(100))
			case SubtaskUnfinished, SubtaskAbandoned:
				s.SettledAt = ptr(instant(100))
			case SubtaskCancelledDraft:
				s.SettledAt = ptr(instant(100))
				s.DraftCancellationID = ptr(CancellationID(identity(90)))
			}
			g.Subtasks = []Subtask{s}
			assertValid(t, g)
		})
	}
}
func TestCancelledTraceKeepsOnlyPendingChildren(t *testing.T) {
	g := baseGraph()
	g.Traces[0].Status = TraceCancelledDraft
	g.Traces[0].SettledAt = ptr(instant(100))
	g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(90)))
	g.Traces[0].DraftCancelledOn = ptr(g.Traces[0].Date)
	s := child()
	g.Subtasks = []Subtask{s}
	assertValid(t, g)
	g.Subtasks[0].Status = SubtaskCompleted
	g.Subtasks[0].CompletedAt = ptr(instant(100))
	assertIssue(t, g, "cancelledTraceHasHistoricalSubtask")
	g.Subtasks[0].DraftCancellationID = g.Traces[0].DraftCancellationID
	assertIssue(t, g, "reusedDraftCancellationIdentity")
}
func TestDayTodoPendingPinsPrecedeTerminalFacts(t *testing.T) {
	g := baseGraph()
	for i := 0; i < 3; i++ {
		c := g.Chains[0]
		c.ID = TaskChainID(identity(20 + i))
		g.Chains = append(g.Chains, c)
		d := g.Definitions[0]
		d.ID = TaskDefinitionID(identity(30 + i))
		d.ChainID = c.ID
		g.Definitions = append(g.Definitions, d)
		tr := g.Traces[0]
		tr.ID = DayTraceID(identity(40 + i))
		tr.ChainID = c.ID
		tr.DefinitionID = d.ID
		tr.Priority = i + 2
		g.Traces = append(g.Traces, tr)
	}
	g.Traces[0].Status = TraceCompleted
	g.Traces[0].CompletedAt = ptr(instant(100))
	g.Traces[2].PinOrder = ptr(2)
	g.Traces[3].PinOrder = ptr(1)
	k := kernel(t, g)
	rows := k.DayTodo(g.Traces[0].Date)
	want := []DayTraceID{g.Traces[3].ID, g.Traces[2].ID, g.Traces[1].ID, g.Traces[0].ID}
	for i, id := range want {
		if rows[i].ID != id {
			t.Fatalf("row%d: %s want%s", i, rows[i].ID, id)
		}
	}
}
func TestConstructorRejectsInvalidInputWithoutMutation(t *testing.T) {
	g := baseGraph()
	g.Traces[0].Status = TraceCompleted
	before := g.Clone()
	if _, err := NewTaskKernel(g); err == nil {
		t.Fatal("invalid graph accepted")
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("constructor changed input")
	}
}
func TestScheduleLateFailureDoesNotAddHalfTraceOrChildren(t *testing.T) {
	g := baseGraph()
	g.Traces = nil
	g.Days = nil
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(identity(20)), LineageID: SubtaskLineageID(identity(21)), Title: "计划", Difficulty: DifficultySimple, Position: 1, CreatedAt: instant(100)}}
	k := kernel(t, g)
	in := ScheduleFromPoolInput{ChainID: g.Chains[0].ID, TraceID: DayTraceID(identity(4)), DayID: "invalid", Date: date("2026-11-05"), Today: date("2026-11-05"), Now: instant(101), SubtaskIDs: map[PlannedSubtaskID]SubtaskID{g.Definitions[0].PlannedSubtasks[0].ID: SubtaskID(identity(30))}}
	unchanged(t, k, ErrInvalidInput, func() error { return k.ScheduleFromPool(in) })
}
func TestNegativePrioritiesRetainUpstreamNextPriorityRule(t *testing.T) {
	g := baseGraph()
	g.Traces[0].Priority = -5
	g.Traces[0].Status = TraceCompleted
	g.Traces[0].CompletedAt = ptr(instant(100))
	k := kernel(t, g)
	if e := k.CreatePoolTask(CreatePoolTaskInput{ChainID: TaskChainID(identity(20)), DefinitionID: TaskDefinitionID(identity(21)), Title: "另一任务", Now: instant(101)}); e != nil {
		t.Fatal(e)
	}
	if e := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: TaskChainID(identity(20)), TraceID: DayTraceID(identity(22)), Date: g.Traces[0].Date, Today: g.Traces[0].Date, Now: instant(102)}); e != nil {
		t.Fatal(e)
	}
	if k.Snapshot().Traces[1].Priority != -4 {
		t.Fatal("negative max priority incorrectly clamped")
	}
}
func FuzzTaskGraphValidationDoesNotPanic(f *testing.F) {
	f.Add(uint64(0), "pending", int64(1))
	f.Add(uint64(0x7ff0000000000000), "cancelledDraft", int64(-1))
	f.Add(uint64(0x7ff8000000000000), "changed", int64(100))
	f.Fuzz(func(t *testing.T, bits uint64, status string, seq int64) {
		g := baseGraph()
		g.Traces[0].ContentUpdatedAt = Instant(bits)
		g.Traces[0].Status = TraceStatus(status)
		g.Traces[0].ContinuationSeq = int(seq)
		before := g.Clone()
		_ = ValidateTaskGraph(g)
		if !reflect.DeepEqual(before, g) {
			t.Fatal("validator mutated input")
		}
	})
}
