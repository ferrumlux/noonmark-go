// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"math"
	"reflect"
	"testing"
)

// Normal-path fixtures use real creation, scheduling, child, and settlement commands.
func recoveryCarryoverSeed(t *testing.T) (*TaskKernel, TraceCarryoverInput) {
	t.Helper()
	k := kernel(t, TaskGraph{})
	in := TraceCarryoverInput{ChainID: TaskChainID(identity(1500)), TraceID: DayTraceID(identity(1502)), NewTraceID: DayTraceID(identity(1503)), DayID: DayID(identity(1504)), Today: date("2026-11-05"), TargetDate: date("2026-11-06"), Now: instant(10)}
	if err := k.CreatePoolTask(CreatePoolTaskInput{ChainID: in.ChainID, DefinitionID: TaskDefinitionID(identity(1501)), Title: "review", Now: instant(-10)}); err != nil {
		t.Fatal(err)
	}
	if err := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, DayID: DayID(identity(1505)), Date: in.Today, Today: in.Today, Now: instant(-9)}); err != nil {
		t.Fatal(err)
	}
	in.SubtaskIDs = map[SubtaskID]SubtaskID{}
	for j := 0; j < 2; j++ {
		id := SubtaskID(identity(1510 + j))
		if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: id, LineageID: SubtaskLineageID(identity(1520 + j)), Title: "step", Difficulty: Difficulty(j + 1), Today: in.Today, Now: instant(-8)}); err != nil {
			t.Fatal(err)
		}
		in.SubtaskIDs[id] = SubtaskID(identity(1530 + j))
	}
	return k, in
}

func TestRecoveryCarryoverDeterminismAndEqualNegativeClock(t *testing.T) {
	a, in := recoveryCarryoverSeed(t)
	b := kernel(t, a.Snapshot())
	in.Now = instant(-8)
	if err := a.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
	in.SubtaskIDs = map[SubtaskID]SubtaskID{SubtaskID(identity(1511)): SubtaskID(identity(1531)), SubtaskID(identity(1510)): SubtaskID(identity(1530))}
	if err := b.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Snapshot(), b.Snapshot()) {
		t.Fatal("map insertion order affected replay")
	}
	out := a.Snapshot()
	if out.Traces[1].CreatedAt != in.Now || out.Chains[0].UpdatedAt != in.Now {
		t.Fatal("negative equal clock changed")
	}
	in.SubtaskIDs[SubtaskID(identity(1510))] = ""
	if a.Snapshot().Subtasks[2].ID != SubtaskID(identity(1530)) {
		t.Fatal("input map aliased")
	}
}

func TestRecoveryContinueDoesNotRequireSourceContentClock(t *testing.T) {
	k, in := recoveryCarryoverSeed(t)
	in.Today = in.TargetDate
	if err := k.SettleTaskDays(in.Today, instant(50)); err != nil {
		t.Fatal(err)
	}
	before := k.Snapshot()
	in.Now = instant(-7.5)
	if err := k.ContinueUnfinishedTrace(in); err != nil {
		t.Fatal(err)
	}
	after := k.Snapshot()
	if !reflect.DeepEqual(before.Traces, after.Traces[:1]) || !reflect.DeepEqual(before.Subtasks, after.Subtasks[:2]) {
		t.Fatal("continuation rewrote settled trace/child history")
	}
	// Swift settleDays does not touchChain; continuation does, using max(old, now).
	if after.Chains[0].UpdatedAt != in.Now {
		t.Fatal("chain was not touched using max")
	}
	if after.Traces[1].CreatedAt != in.Now || after.Subtasks[2].UpdatedAt != in.Now {
		t.Fatal("fractional negative clock not retained")
	}
}

// Structural fixtures below model snapshot/undo/clock states not writable through
// current ordinary-task commands. kernel(t, g) invokes the unchanged validator.
func TestRecoveryContinueActiveAndPoolGuards(t *testing.T) {
	for _, pending := range []bool{true, false} {
		k, in := recoveryCarryoverSeed(t)
		in.Today = in.TargetDate
		if err := k.SettleTaskDays(in.Today, instant(50)); err != nil {
			t.Fatal(err)
		}
		g := k.Snapshot()
		old := DayTrace{ID: DayTraceID(identity(1540)), ChainID: in.ChainID, DefinitionID: g.Definitions[0].ID, Date: date("2026-11-04"), Status: TracePending, Priority: 1, CreatedAt: instant(-9), ContentUpdatedAt: instant(60)}
		want := ErrActiveTraceAlreadyExists
		if !pending {
			old.Status = TraceReturnedToPool
			old.SettledAt = ptr(instant(60))
			want = ErrInvalidTransition
		}
		g.Traces = append(g.Traces, old)
		g.Days = append(g.Days, Day{ID: DayID(identity(1541)), Date: old.Date, CreatedAt: instant(-9), UpdatedAt: instant(-9)})
		k = kernel(t, g)
		before := k.Snapshot()
		if err := k.ContinueUnfinishedTrace(in); err != want {
			t.Fatalf("pending=%v: %v want %v", pending, err, want)
		}
		if !reflect.DeepEqual(before, k.Snapshot()) {
			t.Fatal("guard failure published partial graph")
		}
	}
}

func TestRecoveryDeferChildClockAtomicAndFractional(t *testing.T) {
	k, in := recoveryCarryoverSeed(t)
	g := k.Snapshot()
	g.Subtasks[1].UpdatedAt = instant(10.25)
	k = kernel(t, g)
	before := k.Snapshot()
	in.Now = instant(math.Nextafter(10.25, math.Inf(-1)))
	if err := k.DeferCurrentTrace(in); err != ErrInvalidClock {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("late second-child clock leaked first child/trace changes")
	}
	in.Now = instant(10.25)
	if err := k.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
}

// Structural cancellation record requires snapshot-undo support absent here.
func TestRecoveryCancelledDraftExcludedAndExistingDayDoesNotConsumeID(t *testing.T) {
	k, in := recoveryCarryoverSeed(t)
	g := k.Snapshot()
	g.Days = append(g.Days, Day{ID: DayID(identity(1550)), Date: in.TargetDate, CreatedAt: instant(-9), UpdatedAt: instant(999)})
	cancelled := DayTrace{ID: DayTraceID(identity(1551)), ChainID: in.ChainID, DefinitionID: g.Definitions[0].ID, Date: in.TargetDate, Status: TraceCancelledDraft, Priority: int(^uint(0) >> 1), CreatedAt: instant(-9), ContentUpdatedAt: instant(-8), SettledAt: ptr(instant(-8)), DraftCancellationID: ptr(CancellationID(identity(1552))), DraftCancelledOn: ptr(in.Today)}
	g.Traces = append(g.Traces, cancelled)
	k = kernel(t, g)
	in.DayID = DayID(identity(1505)) // Existing source-day identity is ignored, not consumed.
	if err := k.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
	out := k.Snapshot()
	if !reflect.DeepEqual(g.Days, out.Days) || !reflect.DeepEqual(cancelled, out.Traces[1]) || out.Traces[2].Priority != 1 {
		t.Fatal("cancelled draft occupied frontier/priority or existing day was touched")
	}
}
