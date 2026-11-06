// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// A full synthetic year: 24 chains x 365 traces x 4 children = 8,760 traces /
// 35,040 child records. Two child lineages carry correctly across each chain;
// other completed/abandoned lineages retain history. No production data used.
func progressYearGraph(chains, days int) TaskGraph {
	var g TaskGraph
	serial := 100000
	next := func() string { serial++; return identity(serial) }
	start := date("2025-11-06")
	now := instant(100)
	for day := 0; day < days; day++ {
		d, _ := start.AddDays(day)
		item := Day{ID: DayID(next()), Date: d, CreatedAt: now, UpdatedAt: now}
		if day < days-1 {
			item.LockedAt = ptr(now)
		}
		g.Days = append(g.Days, item)
	}
	for c := 0; c < chains; c++ {
		chainID, defID := TaskChainID(next()), TaskDefinitionID(next())
		g.Chains = append(g.Chains, TaskChain{ID: chainID, State: ChainActive, CreatedAt: now, UpdatedAt: now})
		g.Definitions = append(g.Definitions, TaskDefinition{ID: defID, ChainID: chainID, Sequence: 1, Title: "年度合成任务", CreatedAt: now, ContentUpdatedAt: now})
		lineages := [2]SubtaskLineageID{SubtaskLineageID(next()), SubtaskLineageID(next())}
		var prevTrace DayTraceID
		var prevChild [2]SubtaskID
		for day := 0; day < days; day++ {
			trace := DayTrace{ID: DayTraceID(next()), ChainID: chainID, DefinitionID: defID, Date: g.Days[day].Date, Status: TraceUnfinished, Priority: c + 1, ContinuationSeq: day, CreatedAt: now, ContentUpdatedAt: now, SettledAt: ptr(now)}
			if day > 0 {
				trace.CarriedFromTraceID = ptr(prevTrace)
			}
			if day == days-1 {
				trace.Status = TracePending
				trace.SettledAt = nil
			}
			g.Traces = append(g.Traces, trace)
			for child := 0; child < 4; child++ {
				s := Subtask{ID: SubtaskID(next()), LineageID: SubtaskLineageID(next()), TraceID: trace.ID, Title: "年度合成步骤", Difficulty: Difficulty(1 + (c+day+child)%3), Position: child + 1, CreatedAt: now, UpdatedAt: now}
				if child < 2 {
					s.LineageID = lineages[child]
					s.Status = SubtaskUnfinished
					s.SettledAt = ptr(now)
					if day > 0 {
						s.CarriedFromSubtaskID = ptr(prevChild[child])
					}
					if day == days-1 {
						s.Status = SubtaskPending
						s.SettledAt = nil
						if child == 0 {
							s.Status = SubtaskCompleted
							s.CompletedAt = ptr(now)
						}
					}
					prevChild[child] = s.ID
				} else if child == 2 {
					s.Status = SubtaskCompleted
					s.CompletedAt = ptr(now)
				} else {
					s.Status = SubtaskAbandoned
					s.SettledAt = ptr(now)
				}
				g.Subtasks = append(g.Subtasks, s)
			}
			prevTrace = trace.ID
		}
	}
	return g
}

func TestProgressYearMultiChainDeterminism(t *testing.T) {
	g := progressYearGraph(24, 365)
	if len(g.Traces) != 8760 || len(g.Subtasks) != 35040 {
		t.Fatal("year fixture size drift")
	}
	baseline := progressView(t, g)
	rng := rand.New(rand.NewPCG(2026, 1105))
	for iteration := 0; iteration < 3; iteration++ {
		shuffled := g.Clone()
		rng.Shuffle(len(shuffled.Traces), func(i, j int) { shuffled.Traces[i], shuffled.Traces[j] = shuffled.Traces[j], shuffled.Traces[i] })
		rng.Shuffle(len(shuffled.Subtasks), func(i, j int) {
			shuffled.Subtasks[i], shuffled.Subtasks[j] = shuffled.Subtasks[j], shuffled.Subtasks[i]
		})
		rng.Shuffle(len(shuffled.Definitions), func(i, j int) {
			shuffled.Definitions[i], shuffled.Definitions[j] = shuffled.Definitions[j], shuffled.Definitions[i]
		})
		rng.Shuffle(len(shuffled.Chains), func(i, j int) { shuffled.Chains[i], shuffled.Chains[j] = shuffled.Chains[j], shuffled.Chains[i] })
		rng.Shuffle(len(shuffled.Days), func(i, j int) { shuffled.Days[i], shuffled.Days[j] = shuffled.Days[j], shuffled.Days[i] })
		p := progressView(t, shuffled)
		for _, tr := range g.Traces {
			wantProgress(t, p, tr.ID, baseline.TraceProgress(tr.ID))
			if got := p.SubtaskProgress(tr.ID); got != baseline.SubtaskProgress(tr.ID) {
				t.Fatal("year counts changed")
			}
			cap, err := baseline.CompletionCapability(tr.ID, tr.Date)
			if err != nil {
				t.Fatal(err)
			}
			wantCompletion(t, p, tr.ID, tr.Date, cap)
			manual, err := baseline.CanSetManualProgress(tr.ID, tr.Date)
			if err != nil {
				t.Fatal(err)
			}
			wantManual(t, p, tr.ID, tr.Date, manual)
		}
	}
	// Literal source-shape comparison for several full-year chains, sampled at
	// start/middle/end. This bounds test cost without making a self-comparison.
	for c := 0; c < 24; c += 7 {
		one := TaskGraph{Traces: g.Traces[c*365 : (c+1)*365], Subtasks: g.Subtasks[c*365*4 : (c+1)*365*4]}
		for _, day := range []int{0, 1, 182, 364} {
			id := one.Traces[day].ID
			wantProgress(t, baseline, id, sourceShapeProgress(one, id))
		}
	}
}

var progressBenchmarkSink *ProgressProjection
var progressBenchmarkPercent int

func BenchmarkProgressYearConstruction(b *testing.B) {
	for _, chains := range []int{1, 8, 24} {
		b.Run(fmt.Sprintf("chains_%d", chains), func(b *testing.B) {
			g := progressYearGraph(chains, 365)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				p, err := NewProgressProjection(g)
				if err != nil {
					b.Fatal(err)
				}
				progressBenchmarkSink = p
			}
		})
	}
}
func BenchmarkProgressYearAllRows(b *testing.B) {
	g := progressYearGraph(24, 365)
	p, err := NewProgressProjection(g)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		total := 0
		for _, tr := range g.Traces {
			total += p.TraceProgress(tr.ID).Percent
			total += p.SubtaskProgress(tr.ID).Total
			cap, e := p.CompletionCapability(tr.ID, tr.Date)
			if e != nil {
				b.Fatal(e)
			}
			if cap.Action != "" {
				total++
			}
			manual, e := p.CanSetManualProgress(tr.ID, tr.Date)
			if e != nil {
				b.Fatal(e)
			}
			if manual {
				total++
			}
		}
		progressBenchmarkPercent = total
	}
}
func BenchmarkProgressYearSingleRow(b *testing.B) {
	g := progressYearGraph(24, 365)
	p, err := NewProgressProjection(g)
	if err != nil {
		b.Fatal(err)
	}
	id := g.Traces[len(g.Traces)-1].ID
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		progressBenchmarkPercent = p.TraceProgress(id).Percent
	}
}
