// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"math"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"
)

// This intentionally slow, test-only translation follows the source's group /
// sort / max / contains / scan-floor shape rather than the production sweep.
// It is an independent Go cross-check, NOT a run of the Swift oracle.
func sourceShapeProgress(g TaskGraph, id DayTraceID) TraceProgress {
	var target *DayTrace
	for i := range g.Traces {
		if g.Traces[i].ID == id {
			target = &g.Traces[i]
			break
		}
	}
	if target == nil {
		return TraceProgress{Mode: ProgressManual}
	}
	traces := make(map[DayTraceID]DayTrace)
	for _, tr := range g.Traces {
		if tr.ChainID == target.ChainID && tr.Status.FormsDayHistory() {
			traces[tr.ID] = tr
		}
	}
	raw := func(trace DayTrace) TraceProgress {
		if trace.Status == TraceCompleted {
			return TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 1, TotalWeight: 1}
		}
		type record struct {
			sub   Subtask
			trace DayTrace
		}
		groups := make(map[SubtaskLineageID][]record)
		for _, s := range g.Subtasks {
			tr, ok := traces[s.TraceID]
			if ok && s.Status != SubtaskCancelledDraft && tr.Date.Compare(trace.Date) <= 0 {
				groups[s.LineageID] = append(groups[s.LineageID], record{s, tr})
			}
		}
		if len(groups) == 0 {
			n := 0
			if trace.ManualProgressPercent != nil {
				n = *trace.ManualProgressPercent
			}
			return TraceProgress{Mode: ProgressManual, Percent: n, CompletedWeight: n, TotalWeight: 100}
		}
		result := TraceProgress{Mode: ProgressWeightedSubtasks}
		for _, records := range groups {
			sort.Slice(records, func(i, j int) bool {
				if records[i].trace.Date != records[j].trace.Date {
					return records[i].trace.Date.Compare(records[j].trace.Date) < 0
				}
				return records[i].sub.CreatedAt.Before(records[j].sub.CreatedAt)
			})
			weight := 0
			done := false
			for _, r := range records {
				if int(r.sub.Difficulty) > weight {
					weight = int(r.sub.Difficulty)
				}
				if r.sub.Status == SubtaskCompleted && r.trace.Date.Compare(trace.Date) <= 0 {
					done = true
				}
			}
			result.TotalWeight += weight
			if done {
				result.CompletedWeight += weight
			}
		}
		result.Percent = int(math.Round(float64(result.CompletedWeight) / float64(result.TotalWeight) * 100))
		return result
	}
	result := raw(*target)
	for _, tr := range traces {
		if tr.Date.Compare(target.Date) < 0 || tr.Date == target.Date && tr.ContinuationSeq < target.ContinuationSeq {
			value := raw(tr).Percent
			if value > result.FloorPercent {
				result.FloorPercent = value
			}
		}
	}
	if result.FloorPercent > result.Percent {
		result.Percent = result.FloorPercent
	}
	return result
}

func TestProgressSweepMatchesSourceShapeAcrossGeneratedHistories(t *testing.T) {
	rng := rand.New(rand.NewPCG(43, 6))
	for seed := 0; seed < 120; seed++ {
		f := newProgressFixture()
		f.traceStatus(0, TraceReturnedToPool)
		lineageIDs := []SubtaskLineageID{SubtaskLineageID(f.id()), SubtaskLineageID(f.id()), SubtaskLineageID(f.id()), SubtaskLineageID(f.id())}
		for row := 0; row < 12; row++ {
			day, _ := date("2026-11-05").AddDays(row / 2)
			status := []TraceStatus{TraceReturnedToPool, TraceCompleted, TraceCancelledDraft, TraceAbandoned}[rng.IntN(4)]
			i := 0
			if row > 0 {
				i = f.trace(day, status)
			} else {
				f.traceStatus(0, status)
			}
			f.g.Traces[i].ManualProgressPercent = ptr(rng.IntN(101))
			if rng.IntN(4) == 0 {
				continue
			}
			for j, lineage := range lineageIDs {
				if rng.IntN(3) == 0 {
					continue
				}
				state := []SubtaskStatus{SubtaskPending, SubtaskCompleted, SubtaskUnfinished, SubtaskAbandoned, SubtaskCancelledDraft}[rng.IntN(5)]
				if status == TraceCancelledDraft {
					state = SubtaskPending
				}
				f.sub(i, Difficulty(1+(row+j)%3), state, lineage)
			}
		}
		p := progressView(t, f.g)
		for _, tr := range f.g.Traces {
			wantProgress(t, p, tr.ID, sourceShapeProgress(f.g, tr.ID))
		}
	}
}

func TestProgressCancelledHistoryNeverRaisesFloor(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceCancelledDraft)
	f.g.Traces[0].ManualProgressPercent = ptr(99)
	f.sub(0, DifficultyHard, SubtaskPending, "")
	next := f.trace(date("2026-11-06"), TracePending)
	f.g.Traces[next].ManualProgressPercent = ptr(10)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressManual, Percent: 10, CompletedWeight: 10, TotalWeight: 100})
	wantManual(t, p, f.g.Traces[next].ID, f.g.Traces[next].Date, true)
}
func TestProgressFutureWeightIncreaseDoesNotChangeHistoricalRaw(t *testing.T) {
	f := newProgressFixture()
	f.traceStatus(0, TraceReturnedToPool)
	a := f.sub(0, DifficultySimple, SubtaskCompleted, "")
	f.sub(0, DifficultyMedium, SubtaskAbandoned, "")
	next := f.trace(date("2026-11-06"), TracePending)
	f.sub(next, DifficultyHard, SubtaskPending, f.g.Subtasks[a].LineageID)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 33, CompletedWeight: 1, TotalWeight: 3})
	wantProgress(t, p, f.g.Traces[next].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 60, FloorPercent: 33, CompletedWeight: 3, TotalWeight: 5})
}
func TestProgressCompletedParentNotImpliedByHundredPercent(t *testing.T) {
	f := newProgressFixture()
	f.g.Traces[0].ManualProgressPercent = ptr(100)
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 100, TotalWeight: 100})
	wantCompletion(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, DayTraceCompletionCapability{Action: CompletionComplete})
	f.sub(0, DifficultyHard, SubtaskCompleted, "")
	p = progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, CompletedWeight: 3, TotalWeight: 3})
	if f.g.Traces[0].Status != TracePending {
		t.Fatal("projection completed parent")
	}
}

func FuzzProgressProjectionNoPanic(f *testing.F) {
	for _, seed := range [][]byte{{}, {0}, {1}, {2, 255}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10}, {11}, {12}, {13, 255}, {14}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		g := baseGraph()
		g.Subtasks = []Subtask{child()}
		if len(data) > 0 {
			value := 0
			if len(data) > 1 {
				value = int(data[1])
			}
			switch data[0] % 15 {
			case 0:
				g.Traces = append(g.Traces, g.Traces[0])
			case 1:
				g.Subtasks[0].Difficulty = Difficulty(value - 1)
			case 2:
				g.Traces[0].ManualProgressPercent = ptr(value - 1)
			case 3:
				g.Subtasks[0].TraceID = DayTraceID(string(data))
			case 4:
				g.Traces[0].Date = LocalDate{Year: value, Month: value, Day: value}
			case 5:
				g.Subtasks[0].CarriedFromSubtaskID = ptr(g.Subtasks[0].ID)
			case 6:
				g.Traces[0].CarriedFromTraceID = ptr(g.Traces[0].ID)
			case 7:
				g.Subtasks[0].Status = SubtaskStatus(string(data))
			case 8:
				g.Subtasks[0].Position = value - 1
			case 9:
				g.Subtasks[0].LineageID = SubtaskLineageID(string(data))
			case 10:
				g.Definitions[0].SupersededByDefinitionID = ptr(g.Definitions[0].ID)
				g.Definitions[0].SupersededAt = ptr(instant(100))
			case 11:
				g.Subtasks[0].Status = SubtaskCancelledDraft
				g.Subtasks[0].SettledAt = ptr(instant(100))
				g.Subtasks[0].DraftCancellationID = ptr(CancellationID(identity(50)))
			case 12:
				g.Subtasks[0].Status = SubtaskCompleted
				g.Subtasks[0].CompletedAt = ptr(instant(100))
			case 13:
				g.Subtasks[0].UpdatedAt = Instant(uint64(value) << 56)
			case 14:
				g.Days = nil
			}
		}
		before := g.Clone()
		p, err := NewProgressProjection(g)
		if !reflect.DeepEqual(before, g) {
			t.Fatal("input mutated")
		}
		if err != nil {
			if p != nil {
				t.Fatal("partial projection returned on error")
			}
			return
		}
		for _, tr := range g.Traces {
			wantProgress(t, p, tr.ID, sourceShapeProgress(g, tr.ID))
			p.SubtaskProgress(tr.ID)
			p.CompletionCapability(tr.ID, tr.Date)
			p.CanSetManualProgress(tr.ID, tr.Date)
		}
	})
}

func TestProgressAbandonedChainRetainsWeightedHistory(t *testing.T) {
	f := newProgressFixture()
	f.g.Chains[0].State = ChainAbandoned
	f.traceStatus(0, TraceAbandoned)
	f.sub(0, DifficultySimple, SubtaskCompleted, "")
	f.sub(0, DifficultyHard, SubtaskAbandoned, "")
	p := progressView(t, f.g)
	wantProgress(t, p, f.g.Traces[0].ID, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	wantCompletion(t, p, f.g.Traces[0].ID, f.g.Traces[0].Date, DayTraceCompletionCapability{Blocker: CompletionUnsupportedStatus, Status: TraceAbandoned})
}
