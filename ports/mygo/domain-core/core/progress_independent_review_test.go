// SPDX-License-Identifier: AGPL-3.0-only
// External review overlay. Synthetic development/e2e data only.
package core

import (
	"math"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"
)

// A predicate-based, deliberately cubic reference with no sweep, sorting or
// accumulated lineage state. This is a Go translation of the reviewed source
// predicates, not execution of Swift or an independent specification oracle.
func reviewPredicateRaw(g TaskGraph, target DayTrace) TraceProgress {
	if target.Status == TraceCompleted {
		return TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 1, TotalWeight: 1}
	}
	eligible := func(s Subtask) bool {
		if s.Status == SubtaskCancelledDraft {
			return false
		}
		for _, tr := range g.Traces {
			if tr.ID == s.TraceID {
				return tr.ChainID == target.ChainID && tr.Status != TraceCancelledDraft && tr.Date.Compare(target.Date) <= 0
			}
		}
		return false
	}
	result := TraceProgress{Mode: ProgressWeightedSubtasks}
	for i, s := range g.Subtasks {
		if !eligible(s) {
			continue
		}
		seen := false
		for j := 0; j < i; j++ {
			if g.Subtasks[j].LineageID == s.LineageID && eligible(g.Subtasks[j]) {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		weight, done := 0, false
		for _, same := range g.Subtasks {
			if same.LineageID != s.LineageID || !eligible(same) {
				continue
			}
			if int(same.Difficulty) > weight {
				weight = int(same.Difficulty)
			}
			done = done || same.Status == SubtaskCompleted
		}
		result.TotalWeight += weight
		if done {
			result.CompletedWeight += weight
		}
	}
	if result.TotalWeight == 0 {
		result.Mode = ProgressManual
		result.TotalWeight = 100
		if target.ManualProgressPercent != nil {
			result.Percent = *target.ManualProgressPercent
		}
		result.CompletedWeight = result.Percent
	} else {
		result.Percent = int(math.Round(float64(result.CompletedWeight) / float64(result.TotalWeight) * 100))
	}
	return result
}

func reviewPredicateProgress(g TaskGraph, target DayTrace) TraceProgress {
	result := reviewPredicateRaw(g, target)
	for _, tr := range g.Traces {
		if tr.ChainID != target.ChainID || tr.Status == TraceCancelledDraft {
			continue
		}
		if tr.Date.Compare(target.Date) < 0 || tr.Date == target.Date && tr.ContinuationSeq < target.ContinuationSeq {
			candidate := reviewPredicateRaw(g, tr).Percent
			if candidate > result.FloorPercent {
				result.FloorPercent = candidate
			}
		}
	}
	if result.FloorPercent > result.Percent {
		result.Percent = result.FloorPercent
	}
	return result
}

func TestReviewProgressMachineRoundingBoundary(t *testing.T) {
	// 23/40*100 in IEEE binary64 is 57.49999999999999, not an exact 57.5.
	// Preserving the source arithmetic order therefore means 57, not 58.
	for _, tc := range []struct{ complete, total, percent int }{{23, 40, 57}, {1, 8, 13}, {5, 8, 63}} {
		f := newProgressFixture()
		for i := 0; i < tc.total; i++ {
			state := SubtaskPending
			if i < tc.complete {
				state = SubtaskCompleted
			}
			f.sub(0, DifficultySimple, state, "")
		}
		p := progressView(t, f.g)
		wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: tc.percent, CompletedWeight: tc.complete, TotalWeight: tc.total})
	}
}

func TestReviewProgressCompletedPeerDoesNotLeakBackwardFloor(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceUnfinished)
	f.g.Traces[0].ManualProgressPercent = ptr(80)
	late := f.continuation(0, date("2026-11-06"), TraceCompleted)
	early := f.trace(date("2026-11-06"), TraceReturnedToPool)
	f.g.Traces[early].ManualProgressPercent = ptr(10)
	hidden := f.trace(date("2026-11-06"), TraceCancelledDraft)
	f.g.Traces[hidden].ManualProgressPercent = ptr(99)
	f.sub(hidden, DifficultyHard, SubtaskPending, "")
	next := f.trace(date("2026-11-07"), TracePending)
	f.g.Traces[next].ManualProgressPercent = ptr(5)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[early].ID, TraceProgress{Mode: ProgressManual, Percent: 80, FloorPercent: 80, CompletedWeight: 10, TotalWeight: 100})
	wantProgress(t, p, f.g.Traces[late].ID, TraceProgress{Mode: ProgressManual, Percent: 100, FloorPercent: 80, CompletedWeight: 1, TotalWeight: 1})
	wantProgress(t, p, f.g.Traces[hidden].ID, TraceProgress{Mode: ProgressManual, Percent: 99, FloorPercent: 80, CompletedWeight: 99, TotalWeight: 100})
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressManual, Percent: 100, FloorPercent: 100, CompletedWeight: 5, TotalWeight: 100})
	wantManual(t, p, f.g.Traces[next].ID, f.g.Traces[next].Date, true)
	// Higher-seq children DO affect lower-seq raw, but the higher-seq 100 floor
	// must still not leak into it. Hidden-parent children remain irrelevant.
	f.sub(late, DifficultySimple, SubtaskCompleted, "")
	f.sub(early, DifficultyHard, SubtaskAbandoned, "")
	p = progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[early].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 80, FloorPercent: 80, CompletedWeight: 1, TotalWeight: 4})
	wantProgress(t, p, f.g.Traces[hidden].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 80, FloorPercent: 80, CompletedWeight: 1, TotalWeight: 4})
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, FloorPercent: 100, CompletedWeight: 1, TotalWeight: 4})
}

func TestReviewProgressMixedSequencePredicateDifferential(t *testing.T) {
	rng := rand.New(rand.NewPCG(0x5e94, 0x20261105))
	for seed := 0; seed < 1000; seed++ {
		f := newProgressFixture()
		f.traceStatus(0, TraceUnfinished)
		one := f.continuation(0, date("2026-11-06"), TraceUnfinished)
		f.continuation(one, date("2026-11-07"), TraceReturnedToPool)
		for _, d := range []LocalDate{date("2026-11-05"), date("2026-11-06"), date("2026-11-07")} {
			f.trace(d, []TraceStatus{TraceReturnedToPool, TraceCompleted, TraceCancelledDraft, TraceAbandoned}[rng.IntN(4)])
		}
		f.trace(date("2026-11-08"), TracePending)
		lineages := []SubtaskLineageID{SubtaskLineageID(f.id()), SubtaskLineageID(f.id()), SubtaskLineageID(f.id()), SubtaskLineageID(f.id())}
		for row := range f.g.Traces {
			if rng.IntN(3) != 0 {
				f.g.Traces[row].ManualProgressPercent = ptr(rng.IntN(101))
			}
			for _, lineage := range lineages {
				if rng.IntN(3) == 0 {
					continue
				}
				state := []SubtaskStatus{SubtaskPending, SubtaskCompleted, SubtaskUnfinished, SubtaskAbandoned, SubtaskCancelledDraft}[rng.IntN(5)]
				if f.g.Traces[row].Status == TraceCancelledDraft {
					state = SubtaskPending
				}
				f.sub(row, Difficulty(1+rng.IntN(3)), state, lineage)
			}
		}
		rng.Shuffle(len(f.g.Traces), func(i, j int) { f.g.Traces[i], f.g.Traces[j] = f.g.Traces[j], f.g.Traces[i] })
		rng.Shuffle(len(f.g.Subtasks), func(i, j int) { f.g.Subtasks[i], f.g.Subtasks[j] = f.g.Subtasks[j], f.g.Subtasks[i] })
		p := progressView(t, f.g)
		for _, tr := range f.g.Traces {
			want := reviewPredicateProgress(f.g, tr)
			if got := p.TraceProgress(tr.ID); got != want {
				t.Fatalf("seed %d trace %s: got %+v want %+v", seed, tr.ID, got, want)
			}
		}
	}
}

func TestReviewProgressRetainedSnapshotDeepOwnership(t *testing.T) {
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(35)
	f.g.Traces[0].DescriptionText = ptr("合成描述")
	f.g.Traces[0].NoteEntries = []TaskNoteEntry{{ID: TaskNoteEntryID(f.id()), Body: "", CreatedAt: instant(100), UpdatedAt: instant(100), DeletedAt: ptr(instant(100))}}
	f.g.Days[0].LockedAt = ptr(instant(100))
	f.g.Days[0].ReviewSummary = ptr("合成总结")
	p := progressView(t, f.g)
	id, day := f.g.Traces[0].ID, f.g.Traces[0].Date
	before := f.g.Clone()
	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Go(func() {
			for i := 0; i < 500; i++ {
				if !reflect.DeepEqual(p.traces[id], before.Traces[0]) || !reflect.DeepEqual(p.days[day], before.Days[0]) {
					t.Error("retained snapshot aliased input")
					return
				}
				if got := p.TraceProgress(id); got.Percent != 35 {
					t.Error("progress changed")
					return
				}
				cap, err := p.CompletionCapability(id, day)
				if err != nil || cap.Blocker != CompletionLockedDay {
					t.Error("capability changed")
					return
				}
			}
		})
	}
	for i := 0; i < 500; i++ {
		*f.g.Traces[0].ManualProgressPercent = i % 101
		*f.g.Traces[0].DescriptionText = "输入已修改"
		f.g.Traces[0].NoteEntries[0].Body = "修改备注"
		*f.g.Traces[0].NoteEntries[0].DeletedAt = instant(float64(100 + i))
		*f.g.Days[0].LockedAt = instant(float64(100 + i))
		*f.g.Days[0].ReviewSummary = "修改总结"
	}
	wg.Wait()
	if !reflect.DeepEqual(p.traces[id], before.Traces[0]) || !reflect.DeepEqual(p.days[day], before.Days[0]) {
		t.Fatal("input mutation changed retained snapshot after readers finished")
	}
}

func TestReviewProgressAllFourQueriesAllocateZero(t *testing.T) {
	for _, chains := range []int{1, 8, 24} {
		g := progressYearGraph(chains, 365)
		p, err := NewProgressProjection(g)
		if err != nil {
			t.Fatal(err)
		}
		last := g.Traces[len(g.Traces)-1]
		allocs := testing.AllocsPerRun(1000, func() {
			_ = p.TraceProgress(last.ID)
			_ = p.SubtaskProgress(last.ID)
			_, _ = p.CompletionCapability(last.ID, last.Date)
			_, _ = p.CanSetManualProgress(last.ID, last.Date)
		})
		if allocs != 0 {
			t.Fatalf("%d chains: %v allocations", chains, allocs)
		}
	}
}
