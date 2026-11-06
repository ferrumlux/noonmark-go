// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

// Every valid graph in this suite is generated through real domain commands.
// There are no hand-edited persisted facts or production fixtures.
type manualFixture struct {
	t     *testing.T
	k     *TaskKernel
	chain TaskChainID
	trace DayTraceID
	day   LocalDate
	next  int
	clock float64
}

func newManualFixture(t *testing.T) *manualFixture {
	t.Helper()
	f := &manualFixture{t: t, k: kernel(t, TaskGraph{}), day: date("2026-11-05"), next: 3000, clock: 100}
	f.chain = TaskChainID(f.id())
	f.must(f.k.CreatePoolTask(CreatePoolTaskInput{ChainID: f.chain, DefinitionID: TaskDefinitionID(f.id()), Title: "合成手动进度", Now: f.now()}))
	f.schedule(f.day)
	return f
}
func (f *manualFixture) id() string   { f.next++; return identity(f.next) }
func (f *manualFixture) now() Instant { f.clock++; return instant(f.clock) }
func (f *manualFixture) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}
func (f *manualFixture) set(percent int) {
	f.t.Helper()
	f.must(f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: percent, Today: f.day, Now: f.now()}))
}
func (f *manualFixture) schedule(day LocalDate) {
	f.t.Helper()
	f.trace = DayTraceID(f.id())
	f.must(f.k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: f.chain, TraceID: f.trace, DayID: DayID(f.id()), Date: day, Today: day, Now: f.now()}))
	f.day = day
}
func (f *manualFixture) returned(today LocalDate, planned map[SubtaskID]PlannedSubtaskID) {
	f.t.Helper()
	f.must(f.k.ReturnToPool(ReturnToPoolInput{ChainID: f.chain, TraceID: f.trace, NewDefinitionID: TaskDefinitionID(f.id()), CancellationID: CancellationID(f.id()), PlannedIDs: planned, Today: today, Now: f.now()}))
}
func (f *manualFixture) child() SubtaskID {
	f.t.Helper()
	id := SubtaskID(f.id())
	f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: f.chain, TraceID: f.trace, SubtaskID: id, LineageID: SubtaskLineageID(f.id()), Title: "合成步骤", Difficulty: DifficultyHard, Today: f.day, Now: f.now()}))
	return id
}
func manualTrace(t *testing.T, g TaskGraph, id DayTraceID) DayTrace {
	t.Helper()
	for _, tr := range g.Traces {
		if tr.ID == id {
			return tr
		}
	}
	t.Fatal("trace missing")
	return DayTrace{}
}

func TestManualProgressClampingOptionalNoopAndExactMutation(t *testing.T) {
	f := newManualFixture(t)
	before := f.k.Snapshot()
	if manualTrace(t, before, f.trace).ManualProgressPercent != nil {
		t.Fatal("fixture already has manual progress")
	}
	f.set(math.MinInt)
	after := f.k.Snapshot()
	tr := manualTrace(t, after, f.trace)
	if tr.ManualProgressPercent == nil || *tr.ManualProgressPercent != 0 || !tr.ContentUpdatedAt.After(before.Traces[0].ContentUpdatedAt) {
		t.Fatal("nil to zero must write")
	}
	for i := range after.Traces {
		if after.Traces[i].ID == f.trace {
			after.Traces[i] = before.Traces[0]
		}
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("changed fields beyond target percent/clock")
	}
	for _, percent := range []int{73, 12, math.MaxInt, 99, -1} {
		f.set(percent)
		tr = manualTrace(t, f.k.Snapshot(), f.trace)
		if *tr.ManualProgressPercent != min(100, max(0, percent)) || tr.Status != TracePending || tr.CompletedAt != nil {
			t.Fatal("clamp, lowering, or 100 completion mismatch", tr)
		}
	}
	before = f.k.Snapshot()
	for _, now := range []Instant{instant(-1), Instant(math.Float64bits(math.NaN())), Instant(math.Float64bits(math.Inf(1))), Instant(math.Float64bits(math.Inf(-1)))} {
		if err := f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: math.MinInt, Today: f.day, Now: now}); err != nil {
			t.Fatal("no-op read unused clock", err)
		}
		if !reflect.DeepEqual(before, f.k.Snapshot()) {
			t.Fatal("no-op rewrote facts")
		}
	}
	// Snapshot pointers must never alias the kernel, in either direction.
	*before.Traces[0].ManualProgressPercent = 88
	if *f.k.Snapshot().Traces[0].ManualProgressPercent != 0 {
		t.Fatal("snapshot percent pointer aliases kernel")
	}
}

func TestManualProgressClockValidationAndNoPublication(t *testing.T) {
	f := newManualFixture(t)
	before := f.k.Snapshot()
	for _, now := range []Instant{instant(1), Instant(math.Float64bits(math.NaN())), Instant(math.Float64bits(math.Inf(1))), Instant(math.Float64bits(math.Inf(-1)))} {
		err := f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 20, Today: f.day, Now: now})
		if !errors.Is(err, ErrInvalidClock) || !reflect.DeepEqual(before, f.k.Snapshot()) {
			t.Fatal("invalid clock published", err)
		}
	}
	f.must(f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 20, Today: f.day, Now: before.Traces[0].ContentUpdatedAt}))
	if f.k.Snapshot().Traces[0].ContentUpdatedAt != before.Traces[0].ContentUpdatedAt {
		t.Fatal("equal clock not preserved")
	}
}

func TestManualProgressGuardOrderAndStatuses(t *testing.T) {
	for _, kind := range []string{"missing", "wrong-chain", "invalid-day", "past", "future", "completed", "returned", "cancelled", "settled-locked", "child"} {
		t.Run(kind, func(t *testing.T) {
			f := newManualFixture(t)
			in := SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 0, Today: f.day, Now: Instant(math.Float64bits(math.NaN()))}
			want := ErrImmutableHistory
			switch kind {
			case "missing":
				in.TraceID = DayTraceID(f.id())
				want = ErrNotFound
			case "wrong-chain":
				in.ChainID = TaskChainID(f.id())
				want = ErrInvalidInput
			case "invalid-day":
				in.Today = LocalDate{}
				want = ErrInvalidInput
			case "past":
				in.Today, _ = f.day.AddDays(1)
			case "future":
				in.Today, _ = f.day.AddDays(-1)
			case "completed":
				f.must(f.k.MarkCompleted(f.trace, f.day, f.now()))
			case "returned":
				f.returned(f.day, nil)
			case "cancelled":
				yesterday, _ := f.day.AddDays(-1)
				f.returned(yesterday, nil)
			case "settled-locked":
				tomorrow, _ := f.day.AddDays(1)
				f.must(f.k.SettleTaskDays(tomorrow, f.now()))
			case "child":
				f.child()
				want = ErrInvalidTransition
			}
			before := f.k.Snapshot()
			if err := f.k.SetManualProgress(in); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			if !reflect.DeepEqual(before, f.k.Snapshot()) {
				t.Fatal("rejected command published")
			}
		})
	}
}

func TestManualProgressCompletionUndoReturnAndDateFloor(t *testing.T) {
	f := newManualFixture(t)
	f.set(35)
	f.must(f.k.MarkCompleted(f.trace, f.day, f.now()))
	f.must(f.k.UndoCompleted(f.trace, f.day, f.now()))
	if got := progressView(t, f.k.Snapshot()).TraceProgress(f.trace).Percent; got != 35 {
		t.Fatal("undo lost manual progress", got)
	}
	f.set(60)
	old := f.trace
	f.returned(f.day, nil)
	tomorrow, _ := f.day.AddDays(1)
	f.must(f.k.SettleTaskDays(tomorrow, f.now()))
	f.schedule(tomorrow)
	if manualTrace(t, f.k.Snapshot(), f.trace).ManualProgressPercent != nil {
		t.Fatal("schedule silently copied optional manual field")
	}
	f.set(10)
	if got := progressView(t, f.k.Snapshot()).TraceProgress(f.trace); got.Percent != 60 || got.FloorPercent != 60 {
		t.Fatal("history floor lost", got)
	}
	f.set(90)
	f.set(1)
	if *manualTrace(t, f.k.Snapshot(), f.trace).ManualProgressPercent != 60 || *manualTrace(t, f.k.Snapshot(), old).ManualProgressPercent != 60 {
		t.Fatal("current value or history changed")
	}
	// Same date and sequence does not define a floor, regardless of creation time.
	f.returned(f.day, nil)
	f.schedule(f.day)
	f.set(0)
	if *manualTrace(t, f.k.Snapshot(), f.trace).ManualProgressPercent != 60 {
		t.Fatal("earlier-date floor lost after same-day reschedule")
	}
}

func TestManualProgressSameDayHistoryIsNotFloor(t *testing.T) {
	f := newManualFixture(t)
	f.set(90)
	f.returned(f.day, nil)
	f.schedule(f.day)
	f.set(10)
	if got := progressView(t, f.k.Snapshot()).TraceProgress(f.trace); got.Percent != 10 || got.FloorPercent != 0 {
		t.Fatal(got)
	}
}

func TestManualProgressWholeChainChildHistoryAndTombstones(t *testing.T) {
	for _, offset := range []int{-1, 0, 1} {
		t.Run(map[int]string{-1: "future-history", 0: "same-day-history", 1: "past-history"}[offset], func(t *testing.T) {
			f := newManualFixture(t)
			child := f.child()
			// A completed child is not retained as a planned child by ReturnToPool,
			// so the new trace has zero children. The old actual still blocks.
			f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskComplete, ChainID: f.chain, TraceID: f.trace, SubtaskID: child, Today: f.day, Now: f.now()}))
			old := f.trace
			f.returned(f.day, nil)
			target, _ := f.day.AddDays(offset)
			f.schedule(target)
			if progressView(t, f.k.Snapshot()).SubtaskProgress(f.trace).Total != 0 {
				t.Fatal("current trace unexpectedly has children")
			}
			before := f.k.Snapshot()
			err := f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 30, Today: f.day, Now: f.now()})
			if !errors.Is(err, ErrInvalidTransition) || !reflect.DeepEqual(before, f.k.Snapshot()) {
				t.Fatal("ignored actual history on other trace", old, err)
			}
		})
	}
	f := newManualFixture(t)
	child := f.child()
	f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskDelete, ChainID: f.chain, TraceID: f.trace, SubtaskID: child, CancellationID: CancellationID(f.id()), Today: f.day, Now: f.now()}))
	f.set(42)
	if *manualTrace(t, f.k.Snapshot(), f.trace).ManualProgressPercent != 42 {
		t.Fatal("cancelled child incorrectly blocks")
	}
}

func TestManualProgressIgnoresHiddenParentAndPlannedDefinitions(t *testing.T) {
	f := newManualFixture(t)
	child := f.child()
	plan := PlannedSubtaskID(f.id())
	yesterday, _ := f.day.AddDays(-1)
	f.returned(yesterday, map[SubtaskID]PlannedSubtaskID{child: plan})
	var current TaskDefinitionID
	for _, d := range f.k.Snapshot().Definitions {
		if d.SupersededAt == nil {
			current = d.ID
		}
	}
	// Schedule and future-cancel this planned definition, retaining it as a
	// historical definition containing a real planned child.
	tr := DayTraceID(f.id())
	f.must(f.k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: f.chain, TraceID: tr, DayID: DayID(f.id()), Date: f.day, Today: yesterday, Now: f.now(), SubtaskIDs: map[PlannedSubtaskID]SubtaskID{plan: SubtaskID(f.id())}}))
	f.trace = tr
	child2 := f.k.Snapshot().Subtasks[1].ID
	plan2 := PlannedSubtaskID(f.id())
	f.returned(yesterday, map[SubtaskID]PlannedSubtaskID{child2: plan2})
	for _, d := range f.k.Snapshot().Definitions {
		if d.SupersededAt == nil {
			current = d.ID
		}
	}
	f.must(f.k.EditPlannedSubtask(PlannedSubtaskEditInput{Kind: PlannedSubtaskRemove, ChainID: f.chain, ExpectedDefinitionID: current, SubtaskID: plan2, Now: f.now()}))
	f.schedule(yesterday)
	f.set(48)
	g := f.k.Snapshot()
	foundPlan := false
	for _, d := range g.Definitions {
		foundPlan = foundPlan || len(d.PlannedSubtasks) > 0
	}
	if !foundPlan || len(g.Subtasks) != 2 || *manualTrace(t, g, f.trace).ManualProgressPercent != 48 {
		t.Fatal("hidden actual / planned history handling")
	}
}

func TestManualProgressChildGuardsPrecedeSameValueNoop(t *testing.T) {
	f := newManualFixture(t)
	f.set(40)
	child := f.child()
	before := f.k.Snapshot()
	if err := f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 40, Today: f.day, Now: Instant(math.Float64bits(math.NaN()))}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("same value bypassed child guard", err)
	}
	if !reflect.DeepEqual(before, f.k.Snapshot()) {
		t.Fatal("rejection changed facts")
	}
	f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskDelete, ChainID: f.chain, TraceID: f.trace, SubtaskID: child, CancellationID: CancellationID(f.id()), Today: f.day, Now: f.now()}))
	f.set(40)
	other := TaskChainID(f.id())
	otherTrace := DayTraceID(f.id())
	f.must(f.k.CreatePoolTask(CreatePoolTaskInput{ChainID: other, DefinitionID: TaskDefinitionID(f.id()), Title: "其他合成任务", Now: f.now()}))
	f.must(f.k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: other, TraceID: otherTrace, DayID: DayID(f.id()), Date: f.day, Today: f.day, Now: f.now()}))
	f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: other, TraceID: otherTrace, SubtaskID: SubtaskID(f.id()), LineageID: SubtaskLineageID(f.id()), Title: "其他任务子项", Difficulty: DifficultySimple, Today: f.day, Now: f.now()}))
	f.set(41)
}

func TestManualProgressOnlyComparesTargetContentClock(t *testing.T) {
	f := newManualFixture(t)
	originalClock := f.k.Snapshot().Traces[0].ContentUpdatedAt
	child := f.child()
	f.must(f.k.EditSubtask(SubtaskEditInput{Kind: SubtaskDelete, ChainID: f.chain, TraceID: f.trace, SubtaskID: child, CancellationID: CancellationID(f.id()), Today: f.day, Now: f.now()}))
	f.must(f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 23, Today: f.day, Now: originalClock}))
	if f.k.Snapshot().Traces[0].ContentUpdatedAt != originalClock {
		t.Fatal("core must not silently advance beyond supplied clock")
	}
	maximum := Instant(math.Float64bits(math.MaxFloat64))
	f.must(f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 24, Today: f.day, Now: maximum}))
	before := f.k.Snapshot()
	f.must(f.k.SetManualProgress(SetManualProgressInput{ChainID: f.chain, TraceID: f.trace, Percent: 24, Today: f.day, Now: Instant(math.Float64bits(math.Inf(1)))}))
	if !reflect.DeepEqual(before, f.k.Snapshot()) {
		t.Fatal("exhausted frontier changed on no-op")
	}
}
