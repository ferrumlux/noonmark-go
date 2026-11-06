// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// These are adapter tests, not an independent core oracle: expected task graphs
// use the unchanged core; event deltas are separately constructed literal facts.
func remainingAssert(t *testing.T, r auditedHistoryCommand, err error, want canonical.Value) {
	t.Helper()
	must(t, err)
	if !r.valid() {
		t.Fatal("invalid success proof")
	}
	got, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(got, want) {
		t.Fatal("endpoint differs, including event prefix")
	}
	changed, e := r.changed()
	must(t, e)
	if !changed {
		t.Fatal("successful command reported no-op")
	}
	got.Object["traces"].Array[0].Object["contentUpdatedAt"] = historyNumber(123)
	again, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(again, want) {
		t.Fatal("snapshot copy alias")
	}
}
func TestHistoryRemainingReturnMultiEvent(t *testing.T) {
	for _, steps := range [][]vnStep{nil, {{"A", 2}, {"P", 3}}, {{"A", 2}, {"P", 3}, {"A", 4}, {"P", 5}}} {
		f := vnSeed(t, vnDate)
		f.run(t, steps...)
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		in := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(800)), CancellationID: "unused-invalid", Today: vnDate, Now: vnAt(30), EventID: vnID(900)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, CancellationID: in.CancellationID, Today: in.Today, Now: in.Now}))
		after := k.Snapshot()
		want := lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceReturnedToPool, in.Now))
		r, e := auditReturnHistoryVNext(prior, before, after, in)
		remainingAssert(t, r, e, want)
		frozen := before.Clone()
		bad := after.Clone()
		bad.Definitions[0].Title = "unrelated"
		r, e = auditReturnHistoryVNext(prior, before, bad, in)
		lifecycleZero(t, r, e)
		if !reflect.DeepEqual(before, frozen) {
			t.Fatal("failed call mutated input")
		}
		in.EventID = ""
		r, e = auditReturnHistoryVNext(prior, before, after, in)
		lifecycleZero(t, r, e)
		var ce *historyCommandError
		if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
			t.Fatalf("empty capture: %v", e)
		}
	}
}
func TestHistoryRemainingFutureZeroAndOldStream(t *testing.T) {
	future, e := vnDate.AddDays(1)
	must(t, e)
	for _, old := range []bool{false, true} {
		f := vnSeed(t, future)
		if old {
			f.run(t, vnStep{"A", 2}, vnStep{"P", 3})
		}
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		in := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(800)), CancellationID: core.CancellationID(vnID(801)), Today: vnDate, Now: vnAt(30)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, CancellationID: in.CancellationID, Today: in.Today, Now: in.Now}))
		after := k.Snapshot()
		r, e := auditReturnHistoryVNext(prior, before, after, in)
		if old {
			lifecycleZero(t, r, e)
		} else {
			remainingAssert(t, r, e, lifecycleSnapshot(t, after, f.c))
		}
		in.EventID = vnID(900)
		r, e = auditReturnHistoryVNext(prior, before, after, in)
		lifecycleZero(t, r, e)
		var ce *historyCommandError
		if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
			t.Fatalf("X capture precedes final inspector: %v", e)
		}
	}
}
func TestHistoryRemainingDeferMultiEvent(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	for _, steps := range [][]vnStep{nil, {{"A", 2}, {"P", 3}}, {{"A", 2}, {"P", 3}, {"A", 4}, {"P", 5}}} {
		f := vnSeed(t, vnDate)
		f.run(t, steps...)
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		in := deferHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(800)), DayID: core.DayID(vnID(801)), Today: vnDate, TargetDate: target, Now: vnAt(30), EventID: vnID(900)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, Today: in.Today, TargetDate: in.TargetDate, Now: in.Now}))
		after := k.Snapshot()
		r, e := auditDeferHistoryVNext(prior, before, after, in)
		remainingAssert(t, r, e, lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceDeferred, in.Now)))
	}
}
func TestHistoryRemainingContinueZeroCapture(t *testing.T) {
	today, e := vnDate.AddDays(1)
	must(t, e)
	for _, locked := range []bool{false, true} {
		f := vnSeed(t, vnDate)
		// Open U is a source-supported synthetic fixture, not Service reachability.
		if locked {
			f.run(t, vnStep{"L", 10})
		} else {
			f.run(t, vnStep{"A", 5}, vnStep{"U", 10})
		}
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		in := continueHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(800)), DayID: core.DayID(vnID(801)), Today: today, TargetDate: today, Now: vnAt(30)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, Today: in.Today, TargetDate: in.TargetDate, Now: in.Now}))
		after := k.Snapshot()
		r, e := auditContinueHistoryVNext(prior, before, after, in)
		remainingAssert(t, r, e, lifecycleSnapshot(t, after, f.c))
		if !reflect.DeepEqual(before.Traces[0], after.Traces[0]) {
			t.Fatal("continue source changed")
		}
	}
}
func TestHistoryRemainingGuardOrder(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	target, e := vnDate.AddDays(1)
	must(t, e)
	r, e := auditReturnHistoryVNext(prior, before, before, returnHistoryInput{TraceID: core.DayTraceID(vnID(999)), NewDefinitionID: core.TaskDefinitionID(vnID(800)), Today: vnDate, Now: core.Instant(math.Float64bits(math.NaN()))})
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal(e)
	}
	r, e = auditDeferHistoryVNext(prior, before, before, deferHistoryInput{TraceID: core.DayTraceID(vnID(999)), Today: vnDate, TargetDate: target, Now: core.Instant(math.Float64bits(math.NaN()))})
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrNotFound) {
		t.Fatal(e)
	}
	r, e = auditContinueHistoryVNext(prior, before, before, continueHistoryInput{TraceID: core.DayTraceID(vnID(999)), Today: vnDate, TargetDate: target, Now: core.Instant(math.Float64bits(math.NaN()))})
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrNotFound) {
		t.Fatal(e)
	}
	// Count metadata beats invalid date, missing trace, and NaN; no child scan.
	r, e = auditReturnHistoryVNext(prior, before, before, returnHistoryInput{PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{"private-key": "private-value"}})
	lifecycleZero(t, r, e)
	if strings.Contains(e.Error(), "private") {
		t.Fatal("budget error leaked input")
	}
	if errors.Is(e, core.ErrInvalidInput) {
		t.Fatal("semantic error before map budget")
	}
}

func TestHistoryRemainingOwnershipAndChildren(t *testing.T) {
	before := hsActual(t)
	prior := vnGood(t, hsSnapshot(t, before))
	planned := map[core.SubtaskID]core.PlannedSubtaskID{}
	for i, s := range before.Subtasks {
		if s.TraceID == before.Traces[0].ID {
			planned[s.ID] = core.PlannedSubtaskID(vnID(800 + i))
		}
	}
	in := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(890)), PlannedIDs: planned, Today: vnDate, Now: vnAt(30), EventID: vnID(900)}
	k, e := core.NewTaskKernel(before)
	must(t, e)
	must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, PlannedIDs: in.PlannedIDs, Today: in.Today, Now: in.Now}))
	after := k.Snapshot()
	r, e := auditReturnHistoryVNext(prior, before, after, in)
	must(t, e)
	want, e := r.snapshotCopy()
	must(t, e)
	// Caller graph leaves, input map, and copies must not share proof storage.
	original := make(map[core.SubtaskID]core.PlannedSubtaskID)
	for k, v := range planned {
		original[k] = v
		planned[k] = "poison"
	}
	before.Subtasks[0].Title = "before poison"
	after.Definitions[len(after.Definitions)-1].PlannedSubtasks[0].Title = "after poison"
	again, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(again, want) {
		t.Fatal("caller graph alias")
	}
	if r.proof.returnDetail == nil || !reflect.DeepEqual(r.proof.returnDetail.PlannedIDs, original) {
		t.Fatal("request map alias")
	}
	// A mismatched new kind/detail is not a sealed success result.
	invalid := *r.proof
	invalid.kind = historyCommandDefer
	if (auditedHistoryCommand{proof: &invalid}).valid() {
		t.Fatal("mismatched typed detail accepted")
	}
}
func TestHistoryRemainingCarryoverChildrenOwnership(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	for _, cont := range []bool{false, true} {
		before := hsActual(t)
		if cont {
			k, e := core.NewTaskKernel(before)
			must(t, e)
			must(t, k.SettleTaskDays(target, vnAt(10)))
			before = k.Snapshot()
		}
		c, e := emptySupplement()
		must(t, e)
		if cont {
			for _, tr := range before.Traces {
				c.Classifications = lifecycleEvent(c.Classifications, tr.ID, vnID(950+tr.Priority), core.TraceUnfinished, vnAt(10))
			}
		}
		prior := vnGood(t, lifecycleSnapshot(t, before, c.Classifications))
		ids := map[core.SubtaskID]core.SubtaskID{}
		for i, s := range before.Subtasks {
			if s.TraceID == before.Traces[0].ID {
				ids[s.ID] = core.SubtaskID(vnID(800 + i))
			}
		}
		ci := core.TraceCarryoverInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(890)), DayID: core.DayID(vnID(891)), SubtaskIDs: ids, Today: vnDate, TargetDate: target, Now: vnAt(30)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		var r auditedHistoryCommand
		if cont {
			ci.Today = target
			must(t, k.ContinueUnfinishedTrace(ci))
			r, e = auditContinueHistoryVNext(prior, before, k.Snapshot(), continueHistoryInput{ChainID: ci.ChainID, TraceID: ci.TraceID, NewTraceID: ci.NewTraceID, DayID: ci.DayID, SubtaskIDs: ids, Today: ci.Today, TargetDate: target, Now: ci.Now})
		} else {
			must(t, k.DeferCurrentTrace(ci))
			r, e = auditDeferHistoryVNext(prior, before, k.Snapshot(), deferHistoryInput{ChainID: ci.ChainID, TraceID: ci.TraceID, NewTraceID: ci.NewTraceID, DayID: ci.DayID, SubtaskIDs: ids, Today: ci.Today, TargetDate: target, Now: ci.Now, EventID: vnID(900)})
		}
		must(t, e)
		original := make(map[core.SubtaskID]core.SubtaskID)
		for k, v := range ids {
			original[k] = v
			ids[k] = "poison"
		}
		if cont {
			if r.proof.continueDetail == nil || !reflect.DeepEqual(r.proof.continueDetail.SubtaskIDs, original) {
				t.Fatal("continue map alias")
			}
		} else {
			if r.proof.deferDetail == nil || !reflect.DeepEqual(r.proof.deferDetail.SubtaskIDs, original) {
				t.Fatal("defer map alias")
			}
		}
	}
}
func TestHistoryRemainingUnusedRawBudgetAndSeal(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	huge := strings.Repeat("x", 16*1024*1024+1)
	r, e := auditReturnHistoryVNext(prior, before, before, returnHistoryInput{CancellationID: core.CancellationID(huge)})
	lifecycleZero(t, r, e)
	var be *historyCommandBudgetError
	if !errors.As(e, &be) || be.Phase != historyCommandInputFields || be.Reason != historyCommandRawBytes {
		t.Fatalf("unused return raw budget: %v", e)
	}
	r, e = auditDeferHistoryVNext(prior, before, before, deferHistoryInput{DayID: core.DayID(huge)})
	lifecycleZero(t, r, e)
	if !errors.As(e, &be) || be.Phase != historyCommandInputFields || be.Reason != historyCommandRawBytes {
		t.Fatal(e)
	}
	r, e = auditContinueHistoryVNext(prior, before, before, continueHistoryInput{DayID: core.DayID(huge)})
	lifecycleZero(t, r, e)
	if !errors.As(e, &be) || be.Phase != historyCommandInputFields || be.Reason != historyCommandRawBytes {
		t.Fatal(e)
	}
	r, e = auditReturnHistoryVNext(validatedHistorySnapshot{}, before, before, returnHistoryInput{CancellationID: core.CancellationID(huge)})
	lifecycleZero(t, r, e)
	if errors.As(e, &be) {
		t.Fatal("budget before seal")
	}
}

func TestHistoryRemainingReturnFiniteClockBits(t *testing.T) {
	for _, now := range []core.Instant{vnAt(-5), core.Instant(math.Float64bits(math.Copysign(0, -1))), vnAt(0), vnAt(math.MaxFloat64)} {
		f := vnSeed(t, vnDate)
		before := f.k.Snapshot()
		old := vnAt(-10)
		before.Chains[0].CreatedAt = old
		before.Chains[0].UpdatedAt = old
		before.Definitions[0].CreatedAt = old
		before.Definitions[0].ContentUpdatedAt = old
		before.Traces[0].CreatedAt = old
		before.Traces[0].ContentUpdatedAt = old
		before.Days[0].CreatedAt = old
		before.Days[0].UpdatedAt = old
		prior := vnGood(t, lifecycleSnapshot(t, before, f.c))
		in := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(800)), Today: vnDate, Now: now, EventID: vnID(900)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, Today: in.Today, Now: now}))
		after := k.Snapshot()
		r, e := auditReturnHistoryVNext(prior, before, after, in)
		remainingAssert(t, r, e, lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceReturnedToPool, now)))
		if r.proof.returnDetail.Now != now {
			t.Fatal("request clock bits lost")
		}
	}
}
func TestHistoryRemainingMapErrorsBeforeNaN(t *testing.T) {
	before := hsActual(t)
	prior := vnGood(t, hsSnapshot(t, before))
	target, e := vnDate.AddDays(1)
	must(t, e)
	// Count is within global prior child count, so core's exact source-map
	// cardinality error beats the later carryover finite-clock guard.
	in := deferHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(800)), DayID: core.DayID(vnID(801)), Today: vnDate, TargetDate: target, Now: core.Instant(math.Float64bits(math.NaN())), EventID: vnID(900)}
	r, e := auditDeferHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal(e)
	}
	in.SubtaskIDs = map[core.SubtaskID]core.SubtaskID{}
	for i, s := range before.Subtasks {
		if s.TraceID == in.TraceID {
			in.SubtaskIDs[s.ID] = core.SubtaskID(vnID(850 + i))
		}
	}
	r, e = auditDeferHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal(e)
	}
}

func TestHistoryRemainingDeferExistingDaySignedZero(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	negzero := core.Instant(math.Float64bits(math.Copysign(0, -1)))
	for _, pair := range [][2]core.Instant{{negzero, vnAt(0)}, {vnAt(0), negzero}} {
		f := vnSeed(t, vnDate)
		before := f.k.Snapshot()
		old := vnAt(-10)
		before.Chains[0].CreatedAt = old
		before.Chains[0].UpdatedAt = pair[0]
		before.Definitions[0].CreatedAt = old
		before.Definitions[0].ContentUpdatedAt = old
		before.Traces[0].CreatedAt = old
		before.Traces[0].ContentUpdatedAt = old
		before.Days[0].CreatedAt = old
		before.Days[0].UpdatedAt = old
		before.Days = append(before.Days, core.Day{ID: core.DayID(vnID(801)), Date: target, CreatedAt: old, UpdatedAt: old})
		prior := vnGood(t, lifecycleSnapshot(t, before, f.c))
		in := deferHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(800)), DayID: "unused-invalid", Today: vnDate, TargetDate: target, Now: pair[1], EventID: vnID(900)}
		k, e := core.NewTaskKernel(before)
		must(t, e)
		must(t, k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, Today: in.Today, TargetDate: target, Now: in.Now}))
		after := k.Snapshot()
		r, e := auditDeferHistoryVNext(prior, before, after, in)
		remainingAssert(t, r, e, lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceDeferred, in.Now)))
		if after.Chains[0].UpdatedAt != pair[0] || after.Traces[1].CreatedAt != pair[1] {
			t.Fatal("equal numerical clock must preserve chain bits and assign new trace bits")
		}
		if r.proof.deferDetail.DayID != in.DayID {
			t.Fatal("unused request ID discarded")
		}
	}
}

func TestHistoryRemainingReturnHiddenCancellationOrder(t *testing.T) {
	before := hsActual(t)
	future, e := vnDate.AddDays(1)
	must(t, e)
	for i := range before.Traces {
		before.Traces[i].Date = future
	}
	before.Days[0].Date = future
	hidden := &before.Subtasks[0]
	hidden.Status = core.SubtaskCancelledDraft
	hidden.UpdatedAt = vnAt(40)
	hidden.SettledAt = vnPtr(vnAt(40))
	hidden.DraftCancellationID = vnPtr(core.CancellationID(vnID(888)))
	prior := vnGood(t, hsSnapshot(t, before))
	in := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(800)), CancellationID: *hidden.DraftCancellationID, Today: vnDate, Now: vnAt(30)}
	// Cancellation collision and missing retained-child map both return InvalidInput;
	// this combined case proves rejection but alone cannot distinguish those guards.
	r, e := auditReturnHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal(e)
	}
	in.CancellationID = core.CancellationID(vnID(889))
	r, e = auditReturnHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal("map error must precede hidden-child backward clock", e)
	}
	in.PlannedIDs = map[core.SubtaskID]core.PlannedSubtaskID{}
	for i, s := range before.Subtasks {
		if s.TraceID == in.TraceID && s.Status == core.SubtaskPending {
			in.PlannedIDs[s.ID] = core.PlannedSubtaskID(vnID(850 + i))
		}
	}
	r, e = auditReturnHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal("hidden child clock only after map fixed", e)
	}
	in.CancellationID = *hidden.DraftCancellationID
	r, e = auditReturnHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal("cancellation freshness before hidden clock", e)
	}
	in.CancellationID = core.CancellationID(vnID(889))
	in.Now = vnAt(50)
	k, e := core.NewTaskKernel(before)
	must(t, e)
	must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, CancellationID: in.CancellationID, PlannedIDs: in.PlannedIDs, Today: in.Today, Now: in.Now}))
	after := k.Snapshot()
	r, e = auditReturnHistoryVNext(prior, before, after, in)
	remainingAssert(t, r, e, hsSnapshot(t, after))
	if len(after.Definitions[len(after.Definitions)-1].PlannedSubtasks) != 2 || after.Subtasks[0].Status != core.SubtaskPending || after.Subtasks[0].DraftCancellationID != nil {
		t.Fatal("selected children after, rather than before, X normalization")
	}
}
func TestHistoryRemainingDeferGuardPairs(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	for _, tc := range []struct {
		name   string
		change func(*core.TaskGraph, *deferHistoryInput)
		want   error
	}{
		{"owner before new identity", func(g *core.TaskGraph, in *deferHistoryInput) {
			in.ChainID = core.TaskChainID(vnID(999))
			in.NewTraceID = "invalid"
		}, core.ErrInvalidInput},
		{"new identity before NaN", func(g *core.TaskGraph, in *deferHistoryInput) {
			in.NewTraceID = "invalid"
			in.Now = core.Instant(math.Float64bits(math.NaN()))
		}, core.ErrInvalidInput},
		{"day identity before NaN", func(g *core.TaskGraph, in *deferHistoryInput) {
			in.DayID = "invalid"
			in.Now = core.Instant(math.Float64bits(math.NaN()))
		}, core.ErrInvalidInput},
		{"priority overflow before NaN", func(g *core.TaskGraph, in *deferHistoryInput) {
			g.Traces[1].Date = target
			g.Traces[1].Priority = int(^uint(0) >> 1)
			g.Days = append(g.Days, core.Day{ID: core.DayID(vnID(898)), Date: target, CreatedAt: vnAt(0), UpdatedAt: vnAt(0)})
			in.Now = core.Instant(math.Float64bits(math.NaN()))
		}, core.ErrInvalidInput},
		{"source clock before child clock", func(g *core.TaskGraph, in *deferHistoryInput) { in.Now = vnAt(1) }, core.ErrInvalidClock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := hsActual(t)
			in := deferHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(800)), DayID: core.DayID(vnID(801)), Today: vnDate, TargetDate: target, Now: vnAt(30), EventID: vnID(900), SubtaskIDs: map[core.SubtaskID]core.SubtaskID{}}
			for i, s := range before.Subtasks {
				if s.TraceID == in.TraceID {
					in.SubtaskIDs[s.ID] = core.SubtaskID(vnID(850 + i))
				}
			}
			tc.change(&before, &in)
			prior := vnGood(t, hsSnapshot(t, before))
			frozen := before.Clone()
			r, e := auditDeferHistoryVNext(prior, before, before, in)
			lifecycleZero(t, r, e)
			if !errors.Is(e, tc.want) {
				t.Fatalf("want %v got %v", tc.want, e)
			}
			if !reflect.DeepEqual(before, frozen) {
				t.Fatal("failed guard mutated caller")
			}
		})
	}
}

// Two P traces are not a sealed ordinary prior: preserve that reachability fence.
func TestHistoryRemainingDuplicatePendingCannotSeal(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	extra := before.Traces[0]
	extra.ID = core.DayTraceID(vnID(700))
	extra.Priority = 2
	before.Traces = append(before.Traces, extra)
	_, e := core.NewTaskKernel(before)
	var ve *core.ValidationError
	if !errors.As(e, &ve) || !ve.Has("duplicateActiveTrace") {
		t.Fatalf("expected duplicateActiveTrace: %v", e)
	}
}
func TestHistoryRemainingDeferFrontierBeforeIdentity(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	extra := before.Traces[0]
	extra.ID = core.DayTraceID(vnID(700))
	extra.Priority = 2
	extra.Date = target
	extra.Status = core.TraceCompleted
	extra.CompletedAt = vnPtr(vnAt(0))
	before.Traces = append(before.Traces, extra)
	before.Days = append(before.Days, core.Day{ID: core.DayID(vnID(701)), Date: target, CreatedAt: vnAt(0), UpdatedAt: vnAt(0)})
	// Synthetic source-supported shape, not a Service reachability claim.
	prior := vnGood(t, hsSnapshot(t, before))
	in := deferHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: "invalid", DayID: "invalid", Today: vnDate, TargetDate: target, Now: core.Instant(math.Float64bits(math.NaN()))}
	r, e := auditDeferHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidTransition) {
		t.Fatalf("frontier before identity: %v", e)
	}
}

func TestHistoryRemainingContinuePendingBeforeIdentity(t *testing.T) {
	target, e := vnDate.AddDays(1)
	must(t, e)
	past, e := vnDate.AddDays(-1)
	must(t, e)
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"L", 10})
	before := f.k.Snapshot()
	extra := before.Traces[0]
	extra.ID = core.DayTraceID(vnID(700))
	extra.Date = past
	extra.Priority = 2
	extra.Status = core.TracePending
	extra.SettledAt = nil
	extra.CompletedAt = nil
	extra.ContentUpdatedAt = vnAt(0)
	before.Traces = append(before.Traces, extra)
	before.Days = append(before.Days, core.Day{ID: core.DayID(vnID(701)), Date: past, CreatedAt: vnAt(0), UpdatedAt: vnAt(0)})
	prior := vnGood(t, lifecycleSnapshot(t, before, f.c))
	in := continueHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewTraceID: "invalid", DayID: "invalid", Today: target, TargetDate: target, Now: core.Instant(math.Float64bits(math.NaN()))}
	r, e := auditContinueHistoryVNext(prior, before, before, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrActiveTraceAlreadyExists) {
		t.Fatal(e)
	}
}
