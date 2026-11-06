// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
)

func historyChainSeed(t *testing.T, on LocalDate, children bool) *TaskKernel {
	t.Helper()
	k := kernel(t, TaskGraph{})
	chainID, definitionID := TaskChainID(identity(2)), TaskDefinitionID(identity(3))
	traceID := DayTraceID(identity(4))
	desc := "保留说明 / synthetic"
	note := TaskNoteEntry{ID: TaskNoteEntryID(identity(5)), Body: "保留附言", CreatedAt: instant(100), UpdatedAt: instant(100)}
	if err := k.CreatePoolTask(CreatePoolTaskInput{ChainID: chainID, DefinitionID: definitionID, Title: "历史任务", DescriptionText: &desc, InitialNote: &note, Now: instant(100)}); err != nil {
		t.Fatal(err)
	}
	if err := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: chainID, TraceID: traceID, DayID: DayID(identity(1)), Date: on, Today: on, Now: instant(101)}); err != nil {
		t.Fatal(err)
	}
	if children {
		for i := 0; i < 3; i++ {
			if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: chainID, TraceID: traceID, SubtaskID: SubtaskID(identity(10 + i)), LineageID: SubtaskLineageID(identity(20 + i)), Title: "保留子项", Difficulty: Difficulty(i + 1), Today: on, Now: instant(102)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskComplete, ChainID: chainID, TraceID: traceID, SubtaskID: SubtaskID(identity(11)), Today: on, Now: instant(103)}); err != nil {
			t.Fatal(err)
		}
		if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskDelete, ChainID: chainID, TraceID: traceID, SubtaskID: SubtaskID(identity(12)), CancellationID: CancellationID(identity(30)), Today: on, Now: instant(104)}); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

func historyChainAbandonInput() AbandonChainInput {
	return AbandonChainInput{ChainID: TaskChainID(identity(2)), TraceID: DayTraceID(identity(4)), Now: instant(110)}
}

func historyChainReactivateInput() ReactivateAbandonedChainInput {
	return ReactivateAbandonedChainInput{ChainID: TaskChainID(identity(2)), TraceID: DayTraceID(identity(4)), Today: date("2026-11-05"), Now: instant(120)}
}

func historyChainWant(t *testing.T, k *TaskKernel, want TaskGraph) {
	t.Helper()
	if got := k.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("graph differs from exact permitted changes\ngot: %#v\nwant: %#v", got, want)
	}
	assertValid(t, k.Snapshot())
}

func historyChainRejectReactivate(t *testing.T, k *TaskKernel, in ReactivateAbandonedChainInput, want error) {
	t.Helper()
	unchanged(t, k, want, func() error {
		id, err := k.ReactivateAbandonedChain(in)
		if id != "" {
			t.Fatalf("failure returned identity %q", id)
		}
		return err
	})
}

// Source: NoonmarkEngineTests testAbandonedChainStaysInUnfinishedPoolAndCanBeReenabledInPlace.
func TestHistoryChainAbandonReactivateInPlacePreservesAllOtherFacts(t *testing.T) {
	k := historyChainSeed(t, date("2026-11-05"), true)
	g := k.Snapshot()
	// These valid structural additions cover fields without a public editing API.
	g.Traces[0].PinOrder = ptr(4)
	g.Traces[0].ManualProgressPercent = ptr(63)
	g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(40)))
	tombstone := TaskNoteEntry{ID: TaskNoteEntryID(identity(6)), CreatedAt: instant(100), UpdatedAt: instant(105), DeletedAt: ptr(instant(105))}
	g.Chains[0].NoteEntries = append(g.Chains[0].NoteEntries, tombstone)
	g.Traces[0].NoteEntries = append(g.Traces[0].NoteEntries, tombstone)
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(identity(41)), LineageID: SubtaskLineageID(identity(42)), Title: "保留计划", Position: 1, Difficulty: DifficultyHard, CreatedAt: instant(100)}}
	k = kernel(t, g)
	a := historyChainAbandonInput()
	if err := k.AbandonChain(a); err != nil {
		t.Fatal(err)
	}
	want := g.Clone()
	want.Chains[0].State, want.Chains[0].UpdatedAt = ChainAbandoned, a.Now
	want.Traces[0].Status, want.Traces[0].ContentUpdatedAt = TraceAbandoned, a.Now
	want.Traces[0].PinOrder, want.Traces[0].SettledAt = nil, ptr(a.Now)
	historyChainWant(t, k, want)
	r := historyChainReactivateInput()
	r.Today = date("2026-11-06")
	id, err := k.ReactivateAbandonedChain(r)
	if err != nil || id != a.TraceID {
		t.Fatalf("got %q, %v", id, err)
	}
	want.Chains[0].State, want.Chains[0].UpdatedAt = ChainActive, r.Now
	want.Traces[0].Status, want.Traces[0].ContentUpdatedAt = TraceUnfinished, r.Now
	historyChainWant(t, k, want)
	if k.Snapshot().Subtasks[0].Status != SubtaskPending || len(k.Snapshot().Traces) != 1 {
		t.Fatal("reactivation copied or settled children")
	}
}

func TestHistoryChainReactivationDateAndLockMatrix(t *testing.T) {
	for _, on := range []string{"2026-11-04", "2026-11-05", "2026-11-06"} {
		for _, locked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/locked=%v", on, locked), func(t *testing.T) {
				k := historyChainSeed(t, date(on), true)
				a := historyChainAbandonInput()
				if err := k.AbandonChain(a); err != nil {
					t.Fatal(err)
				}
				if locked {
					after, err := date(on).AddDays(1)
					if err != nil {
						t.Fatal(err)
					}
					if err := k.SettleTaskDays(after, instant(115)); err != nil {
						t.Fatal(err)
					}
				}
				want := k.Snapshot()
				r := historyChainReactivateInput()
				id, err := k.ReactivateAbandonedChain(r)
				if err != nil || id != r.TraceID {
					t.Fatalf("got %q, %v", id, err)
				}
				want.Chains[0].State, want.Chains[0].UpdatedAt = ChainActive, r.Now
				want.Traces[0].ContentUpdatedAt = r.Now
				if locked || date(on).Compare(r.Today) < 0 {
					want.Traces[0].Status = TraceUnfinished
				} else {
					want.Traces[0].Status, want.Traces[0].SettledAt = TracePending, nil
				}
				historyChainWant(t, k, want)
				historyChainRejectReactivate(t, k, r, ErrInvalidTransition)
			})
		}
	}
}

// Source: testAbandoningContinuedChainTargetsItsActiveTraceAndReactivatesInPlace.
func TestHistoryChainHistoricalRequestTargetsFuturePending(t *testing.T) {
	k := historyChainSeed(t, date("2026-11-05"), true)
	if err := k.SettleTaskDays(date("2026-11-06"), instant(105)); err != nil {
		t.Fatal(err)
	}
	c := TraceCarryoverInput{ChainID: TaskChainID(identity(2)), TraceID: DayTraceID(identity(4)), NewTraceID: DayTraceID(identity(50)), DayID: DayID(identity(51)), Today: date("2026-11-06"), TargetDate: date("2026-11-09"), Now: instant(106), SubtaskIDs: map[SubtaskID]SubtaskID{SubtaskID(identity(10)): SubtaskID(identity(52))}}
	if err := k.ContinueUnfinishedTrace(c); err != nil {
		t.Fatal(err)
	}
	g := k.Snapshot()
	// Only the actual target/chain clock is read. Newer historical content must
	// not redirect the request back to the source or cause a clock rejection.
	g.Traces[0].ContentUpdatedAt = instant(900)
	g.Traces[1].PinOrder = ptr(2)
	k = kernel(t, g)
	a := historyChainAbandonInput()
	if err := k.AbandonChain(a); err != nil {
		t.Fatal(err)
	}
	want := g.Clone()
	want.Chains[0].State, want.Chains[0].UpdatedAt = ChainAbandoned, a.Now
	want.Traces[1].Status, want.Traces[1].ContentUpdatedAt = TraceAbandoned, a.Now
	want.Traces[1].PinOrder, want.Traces[1].SettledAt = nil, ptr(a.Now)
	historyChainWant(t, k, want)
	r := historyChainReactivateInput()
	historyChainRejectReactivate(t, k, r, ErrInvalidTransition)
	r.TraceID, r.Today = c.NewTraceID, c.Today
	id, err := k.ReactivateAbandonedChain(r)
	if err != nil || id != c.NewTraceID {
		t.Fatalf("reactivated requested history rather than active target: %q %v", id, err)
	}
	want.Chains[0].State, want.Chains[0].UpdatedAt = ChainActive, r.Now
	want.Traces[1].Status, want.Traces[1].ContentUpdatedAt = TracePending, r.Now
	want.Traces[1].SettledAt = nil
	historyChainWant(t, k, want)
}

// This builder supplies valid structural boundary fixtures, not claims that
// every combination can be reached through this bounded public kernel.
func historyChainStatusGraph(t *testing.T, status TraceStatus) TaskGraph {
	t.Helper()
	g := baseGraph()
	g.Traces[0].Status = status
	switch status {
	case TraceCompleted:
		g.Traces[0].CompletedAt = ptr(instant(100))
	case TraceUnfinished, TraceAbandoned, TraceReturnedToPool, TraceDeferred, TraceChanged, TraceCancelledDraft:
		g.Traces[0].SettledAt = ptr(instant(100))
	}
	if status == TraceCancelledDraft {
		g.Traces[0].DraftCancellationID = ptr(CancellationID(identity(70)))
		g.Traces[0].DraftCancelledOn = ptr(g.Traces[0].Date)
	}
	if status == TraceDeferred || status == TraceChanged {
		next := g.Traces[0]
		next.ID, next.Status, next.SettledAt = DayTraceID(identity(71)), TraceCompleted, nil
		next.CompletedAt = ptr(instant(100))
		if status == TraceDeferred {
			next.Date, next.CarriedFromTraceID = date("2026-11-06"), ptr(g.Traces[0].ID)
			g.Days = append(g.Days, Day{ID: DayID(identity(72)), Date: next.Date, CreatedAt: instant(100), UpdatedAt: instant(100)})
		} else {
			other := g.Chains[0]
			other.ID = TaskChainID(identity(73))
			g.Chains = append(g.Chains, other)
			def := g.Definitions[0]
			def.ID, def.ChainID = TaskDefinitionID(identity(74)), other.ID
			g.Definitions = append(g.Definitions, def)
			next.ChainID, next.DefinitionID, next.Priority = other.ID, def.ID, 2
			g.Traces[0].ChangedToTraceID = ptr(next.ID)
		}
		g.Traces = append(g.Traces, next)
	}
	assertValid(t, g)
	return g
}

func TestHistoryChainAbandonStateGuardsAndActiveRedirection(t *testing.T) {
	statuses := []TraceStatus{TracePending, TraceUnfinished, TraceCompleted, TraceDeferred, TraceChanged, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned}
	for _, status := range statuses {
		for _, hasActive := range []bool{false, true} {
			if status == TracePending && hasActive {
				continue // A second pending is rejected by the constructor.
			}
			t.Run(fmt.Sprintf("%s/active=%v", status, hasActive), func(t *testing.T) {
				g := historyChainStatusGraph(t, status)
				if hasActive {
					next := baseGraph().Traces[0]
					next.ID, next.Date, next.Priority = DayTraceID(identity(75)), date("2026-11-07"), 3
					next.PinOrder = ptr(3)
					g.Days = append(g.Days, Day{ID: DayID(identity(76)), Date: next.Date, CreatedAt: instant(100), UpdatedAt: instant(100)})
					g.Traces = append(g.Traces, next)
				}
				k, a := kernel(t, g), historyChainAbandonInput()
				if !hasActive && status != TracePending && status != TraceUnfinished {
					unchanged(t, k, ErrInvalidTransition, func() error { return k.AbandonChain(a) })
					return
				}
				if err := k.AbandonChain(a); err != nil {
					t.Fatal(err)
				}
				want := g.Clone()
				ti := 0
				if hasActive {
					ti = len(g.Traces) - 1
				}
				want.Chains[0].State, want.Chains[0].UpdatedAt = ChainAbandoned, a.Now
				want.Traces[ti].Status, want.Traces[ti].ContentUpdatedAt = TraceAbandoned, a.Now
				want.Traces[ti].PinOrder, want.Traces[ti].SettledAt = nil, ptr(a.Now)
				historyChainWant(t, k, want)
				unchanged(t, k, ErrChainAbandoned, func() error { return k.AbandonChain(a) })
			})
		}
	}
}

func TestHistoryChainReactivateStateAndActiveGuards(t *testing.T) {
	for _, state := range []TaskChainState{ChainActive, ChainAbandoned} {
		for _, status := range []TraceStatus{TracePending, TraceCompleted, TraceUnfinished, TraceDeferred, TraceChanged, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned} {
			if state == ChainAbandoned && status == TraceAbandoned {
				continue
			}
			t.Run(fmt.Sprintf("%s/%s", state, status), func(t *testing.T) {
				g := historyChainStatusGraph(t, status)
				g.Chains[0].State = state
				historyChainRejectReactivate(t, kernel(t, g), historyChainReactivateInput(), ErrInvalidTransition)
			})
		}
	}
	g := historyChainStatusGraph(t, TraceAbandoned)
	g.Chains[0].State = ChainAbandoned
	next := baseGraph().Traces[0]
	next.ID, next.Priority = DayTraceID(identity(80)), 2
	g.Traces = append(g.Traces, next)
	historyChainRejectReactivate(t, kernel(t, g), historyChainReactivateInput(), ErrActiveTraceAlreadyExists)
	// Pending on a different chain must not block this chain's reactivation.
	other, def := g.Chains[0], g.Definitions[0]
	other.ID, other.State = TaskChainID(identity(81)), ChainActive
	def.ID, def.ChainID = TaskDefinitionID(identity(82)), other.ID
	g.Chains, g.Definitions = append(g.Chains, other), append(g.Definitions, def)
	g.Traces[1].ChainID, g.Traces[1].DefinitionID = other.ID, def.ID
	k := kernel(t, g)
	if id, err := k.ReactivateAbandonedChain(historyChainReactivateInput()); err != nil || id != g.Traces[0].ID {
		t.Fatalf("other chain blocked reactivation: %q %v", id, err)
	}
}

func TestHistoryChainIdentityFencePrecedesStateAndRedirection(t *testing.T) {
	for _, abandoned := range []bool{false, true} {
		g := historyChainStatusGraph(t, TraceUnfinished)
		if abandoned {
			g.Chains[0].State, g.Traces[0].Status = ChainAbandoned, TraceAbandoned
		}
		other, def, active := baseGraph().Chains[0], baseGraph().Definitions[0], baseGraph().Traces[0]
		other.ID = TaskChainID(identity(83))
		def.ID, def.ChainID = TaskDefinitionID(identity(84)), other.ID
		active.ID, active.ChainID, active.DefinitionID, active.Priority = DayTraceID(identity(85)), other.ID, def.ID, 2
		g.Chains, g.Definitions, g.Traces = append(g.Chains, other), append(g.Definitions, def), append(g.Traces, active)
		for _, chainID := range []TaskChainID{"", TaskChainID(identity(999)), other.ID} {
			k, a, r := kernel(t, g), historyChainAbandonInput(), historyChainReactivateInput()
			a.ChainID, r.ChainID = chainID, chainID
			a.Now, r.Now = Instant(math.Float64bits(math.NaN())), Instant(math.Float64bits(math.NaN()))
			unchanged(t, k, ErrInvalidInput, func() error { return k.AbandonChain(a) })
			historyChainRejectReactivate(t, k, r, ErrInvalidInput)
		}
		for _, traceID := range []DayTraceID{"", DayTraceID(identity(999))} {
			k, a, r := kernel(t, g), historyChainAbandonInput(), historyChainReactivateInput()
			a.TraceID, r.TraceID = traceID, traceID
			unchanged(t, k, ErrNotFound, func() error { return k.AbandonChain(a) })
			historyChainRejectReactivate(t, k, r, ErrNotFound)
		}
	}
	g := historyChainStatusGraph(t, TraceAbandoned)
	g.Chains[0].State = ChainAbandoned
	for _, day := range []LocalDate{{}, {2026, 2, 29}, {0, 1, 1}, {10000, 1, 1}} {
		r := historyChainReactivateInput()
		r.Today = day
		historyChainRejectReactivate(t, kernel(t, g), r, ErrInvalidInput)
	}
}

func TestHistoryChainEachContentClockRejectsAtomically(t *testing.T) {
	for _, reactivation := range []bool{false, true} {
		for _, field := range []string{"chain", "trace"} {
			for _, newer := range []float64{121, math.MaxFloat64} {
				t.Run(fmt.Sprintf("reactivate=%v/%s/%g", reactivation, field, newer), func(t *testing.T) {
					g := historyChainStatusGraph(t, TracePending)
					if reactivation {
						g = historyChainStatusGraph(t, TraceAbandoned)
						g.Chains[0].State = ChainAbandoned
					}
					if field == "chain" {
						g.Chains[0].UpdatedAt = instant(newer)
					} else {
						g.Traces[0].ContentUpdatedAt = instant(newer)
					}
					k := kernel(t, g)
					if reactivation {
						historyChainRejectReactivate(t, k, historyChainReactivateInput(), ErrInvalidClock)
					} else {
						unchanged(t, k, ErrInvalidClock, func() error { return k.AbandonChain(historyChainAbandonInput()) })
					}
				})
			}
		}
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.Nextafter(100, math.Inf(-1))} {
			g := historyChainStatusGraph(t, TracePending)
			if reactivation {
				g = historyChainStatusGraph(t, TraceAbandoned)
				g.Chains[0].State = ChainAbandoned
			}
			k := kernel(t, g)
			if reactivation {
				r := historyChainReactivateInput()
				r.Now = Instant(math.Float64bits(bad))
				historyChainRejectReactivate(t, k, r, ErrInvalidClock)
			} else {
				a := historyChainAbandonInput()
				a.Now = Instant(math.Float64bits(bad))
				unchanged(t, k, ErrInvalidClock, func() error { return k.AbandonChain(a) })
			}
		}
	}
}

func TestHistoryChainFiniteEqualClocksPreserveExactBits(t *testing.T) {
	for _, seconds := range []float64{-math.MaxFloat64, -100.125, -math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0, math.SmallestNonzeroFloat64, 100.00000000000001, math.MaxFloat64} {
		t.Run(fmt.Sprintf("%016X", math.Float64bits(seconds)), func(t *testing.T) {
			g := baseGraph()
			clock := instant(seconds)
			g.Chains[0].CreatedAt, g.Chains[0].UpdatedAt = clock, clock
			g.Traces[0].CreatedAt, g.Traces[0].ContentUpdatedAt = clock, clock
			g.Days[0].CreatedAt, g.Days[0].UpdatedAt = clock, clock
			g.Definitions[0].CreatedAt, g.Definitions[0].ContentUpdatedAt = clock, clock
			k, a, r := kernel(t, g), historyChainAbandonInput(), historyChainReactivateInput()
			a.Now, r.Now = clock, clock
			for round := 0; round < 3; round++ {
				if err := k.AbandonChain(a); err != nil {
					t.Fatal(err)
				}
				out := k.Snapshot()
				if out.Chains[0].UpdatedAt != clock || out.Traces[0].ContentUpdatedAt != clock || *out.Traces[0].SettledAt != clock {
					t.Fatal("abandon changed clock bits or advanced equality")
				}
				if id, err := k.ReactivateAbandonedChain(r); err != nil || id != a.TraceID {
					t.Fatalf("%q %v", id, err)
				}
				if out := k.Snapshot(); out.Chains[0].UpdatedAt != clock || out.Traces[0].ContentUpdatedAt != clock || out.Traces[0].SettledAt != nil {
					t.Fatal("reactivate changed clock bits or kept settledAt")
				}
			}
		})
	}
	// IEEE equality permits changing the stored sign of zero to the supplied Now.
	g := baseGraph()
	g.Chains[0].CreatedAt, g.Chains[0].UpdatedAt = instant(0), instant(0)
	g.Traces[0].CreatedAt, g.Traces[0].ContentUpdatedAt = instant(0), instant(0)
	a := historyChainAbandonInput()
	a.Now = instant(math.Copysign(0, -1))
	k := kernel(t, g)
	if err := k.AbandonChain(a); err != nil || k.Snapshot().Chains[0].UpdatedAt != a.Now {
		t.Fatal("signed-zero mutation rejected or normalized", err)
	}
}

func TestHistoryChainAbandonDoesNotImposeLockDateOrFrontierGuard(t *testing.T) {
	k := historyChainSeed(t, date("2026-11-05"), true)
	if err := k.SettleTaskDays(date("2026-11-06"), instant(105)); err != nil {
		t.Fatal(err)
	}
	want := k.Snapshot()
	a := historyChainAbandonInput()
	if err := k.AbandonChain(a); err != nil {
		t.Fatal(err)
	}
	want.Chains[0].State, want.Chains[0].UpdatedAt = ChainAbandoned, a.Now
	want.Traces[0].Status, want.Traces[0].ContentUpdatedAt, want.Traces[0].SettledAt = TraceAbandoned, a.Now, ptr(a.Now)
	historyChainWant(t, k, want)
	// An unrelated newer unfinished fact is not activeTrace or an implicit
	// frontier restriction. Both facts are structurally valid independent roots.
	g := historyChainStatusGraph(t, TraceUnfinished)
	next := g.Traces[0]
	next.ID, next.Date = DayTraceID(identity(90)), date("2026-11-06")
	next.ContentUpdatedAt = instant(1000)
	g.Days = append(g.Days, Day{ID: DayID(identity(91)), Date: next.Date, CreatedAt: instant(100), UpdatedAt: instant(1000)})
	g.Traces = append(g.Traces, next)
	k = kernel(t, g)
	if err := k.AbandonChain(a); err != nil || !reflect.DeepEqual(k.Snapshot().Traces[1], next) {
		t.Fatal("invented a newest-history or clock restriction", err)
	}
}

func TestHistoryChainReferencedSourceValidationFailureRollsBack(t *testing.T) {
	k := historyChainSeed(t, date("2026-11-05"), false)
	if err := k.SettleTaskDays(date("2026-11-06"), instant(105)); err != nil {
		t.Fatal(err)
	}
	c := TraceCarryoverInput{ChainID: TaskChainID(identity(2)), TraceID: DayTraceID(identity(4)), NewTraceID: DayTraceID(identity(100)), DayID: DayID(identity(101)), Today: date("2026-11-06"), TargetDate: date("2026-11-06"), Now: instant(106)}
	if err := k.ContinueUnfinishedTrace(c); err != nil {
		t.Fatal(err)
	}
	if err := k.MarkCompleted(c.NewTraceID, c.Today, instant(107)); err != nil {
		t.Fatal(err)
	}
	before := k.Snapshot()
	err := k.AbandonChain(historyChainAbandonInput())
	var validationError *ValidationError
	if !errors.As(err, &validationError) || !validationError.Has("invalidTraceContinuation") {
		t.Fatalf("expected bounded graph topology rejection, got %#v", err)
	}
	historyChainWant(t, k, before)
}

func TestHistoryChainNewerUnchangedClocksAndCloneIsolation(t *testing.T) {
	g := historyChainSeed(t, date("2026-11-05"), true).Snapshot()
	g.Definitions[0].ContentUpdatedAt, g.Days[0].UpdatedAt = instant(1000), instant(1000)
	g.Chains[0].NoteEntries[0].UpdatedAt = instant(1000)
	g.Traces[0].NoteEntries[0].UpdatedAt = instant(1000)
	g.Subtasks[0].UpdatedAt = instant(1000)
	k := kernel(t, g)
	a, r := historyChainAbandonInput(), historyChainReactivateInput()
	if err := k.AbandonChain(a); err != nil {
		t.Fatal(err)
	}
	if id, err := k.ReactivateAbandonedChain(r); err != nil || id != a.TraceID {
		t.Fatalf("newer untouched fact blocked operation: %q %v", id, err)
	}
	want := g.Clone()
	want.Chains[0].UpdatedAt, want.Traces[0].ContentUpdatedAt = r.Now, r.Now
	historyChainWant(t, k, want)
	// Mutating constructor input, output, and input structs cannot change k.
	g.Chains[0].NoteEntries[0].Body = "外部修改"
	*g.Traces[0].DescriptionText = "外部修改"
	g.Subtasks[0].Title = "外部修改"
	output := k.Snapshot()
	output.Chains[0].NoteEntries[0].Body = "快照修改"
	output.Traces[0].NoteEntries[0].Body = "快照修改"
	*output.Traces[0].DescriptionText = "快照修改"
	output.Definitions[0].Title, output.Subtasks[0].Title = "快照修改", "快照修改"
	a.Now, r.Now = instant(999), instant(999)
	historyChainWant(t, k, want)
}

func TestHistoryChainConstructorStillRejectsUnsupportedOrInvalidFacts(t *testing.T) {
	g := baseGraph()
	g.Chains[0].CycleMembership = &TaskCycleMembership{SeriesID: TaskCycleSeriesID(identity(110)), OccurrenceDate: g.Traces[0].Date}
	assertValid(t, g)
	if _, err := NewTaskKernel(g); !errors.Is(err, ErrUnsupportedRecurring) {
		t.Fatal("recurring scope widened", err)
	}
	g = historyChainStatusGraph(t, TraceAbandoned)
	g.Chains[0].State = ChainAbandoned
	g.Traces[0].SettledAt = nil
	if _, err := NewTaskKernel(g); err == nil {
		t.Fatal("invalid abandoned nil settledAt accepted")
	}
	g = historyChainStatusGraph(t, TraceAbandoned)
	g.Traces[0].PinOrder = ptr(1)
	if _, err := NewTaskKernel(g); err == nil {
		t.Fatal("invalid abandoned pin accepted")
	}
	g = baseGraph()
	next := g.Traces[0]
	next.ID, next.Priority = DayTraceID(identity(111)), 2
	g.Traces = append(g.Traces, next)
	if _, err := NewTaskKernel(g); err == nil {
		t.Fatal("duplicate active trace accepted")
	}
}

func TestHistoryChainConcurrentDuplicateCommandsAndSnapshotReads(t *testing.T) {
	k := historyChainSeed(t, date("2026-11-05"), true)
	a, r := historyChainAbandonInput(), historyChainReactivateInput()
	for _, reactivate := range []bool{false, true} {
		results := make(chan error, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if reactivate {
					id, err := k.ReactivateAbandonedChain(r)
					if (err == nil && id != r.TraceID) || (err != nil && id != "") {
						results <- fmt.Errorf("unexpected returned ID %q for %v", id, err)
						return
					}
					results <- err
				} else {
					results <- k.AbandonChain(a)
				}
				_ = k.Snapshot()
			}()
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if (!reactivate && !errors.Is(err, ErrChainAbandoned)) || (reactivate && !errors.Is(err, ErrInvalidTransition)) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatalf("expected one admitted command, got %d", success)
		}
		assertValid(t, k.Snapshot())
	}
}
