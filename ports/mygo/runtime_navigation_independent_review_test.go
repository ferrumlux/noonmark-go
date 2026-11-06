// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math/rand"
	"reflect"
	"testing"
)

// Independent metamorphic gate: disconnected task components must add, while
// all completed records within one chain count once, independent of ordering.
func TestNavigationReviewDisjointUnionAndChildStatus(t *testing.T) {
	day := liveTestDay
	future, _ := day.AddDays(1)
	g := core.TaskGraph{}
	statuses := []core.SubtaskStatus{core.SubtaskPending, core.SubtaskCompleted, core.SubtaskUnfinished, core.SubtaskDeferred, core.SubtaskAbandoned, core.SubtaskCancelledDraft}
	for i, status := range statuses {
		id := string(rune('a' + i))
		g.Chains = append(g.Chains, core.TaskChain{ID: core.TaskChainID(id), State: core.ChainActive})
		g.Definitions = append(g.Definitions, core.TaskDefinition{ID: core.TaskDefinitionID(id), ChainID: core.TaskChainID(id)})
		g.Traces = append(g.Traces, core.DayTrace{ID: core.DayTraceID(id), ChainID: core.TaskChainID(id), DefinitionID: core.TaskDefinitionID(id), Status: core.TracePending, Date: future})
		for j := 0; j < 3; j++ {
			g.Subtasks = append(g.Subtasks, core.Subtask{TraceID: core.DayTraceID(id), Status: status})
		}
	}
	want := runtimeNavigationCounts{upcoming: 6, completed: 1}
	rng := rand.New(rand.NewSource(541))
	for i := 0; i < 100; i++ {
		rng.Shuffle(len(g.Chains), func(i, j int) { g.Chains[i], g.Chains[j] = g.Chains[j], g.Chains[i] })
		rng.Shuffle(len(g.Definitions), func(i, j int) { g.Definitions[i], g.Definitions[j] = g.Definitions[j], g.Definitions[i] })
		rng.Shuffle(len(g.Traces), func(i, j int) { g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i] })
		rng.Shuffle(len(g.Subtasks), func(i, j int) { g.Subtasks[i], g.Subtasks[j] = g.Subtasks[j], g.Subtasks[i] })
		p := newRuntimeNavigationProjection(g)
		if got := p.counts(day); got != want {
			t.Fatalf("permutation %d: %+v", i, got)
		}
		if got := p.counts(future); got != (runtimeNavigationCounts{day: 6, completed: 1}) {
			t.Fatal("date boundary", got)
		}
	}
}

func TestNavigationReviewPublicationOwnsSnapshotAndRejectsUnknown(t *testing.T) {
	f, _, _, _ := progressFixtureUI(t)
	original := f.svc.Snapshot()
	p := newRuntimeProgressCache(original)
	if p.err != nil {
		t.Fatal(p.err)
	}
	owned := p.graph.Clone()
	counts := p.navigation.counts(liveTestDay)
	projection := p.navigation
	// Mutate slices and nested optional timestamps in the producer's snapshot.
	original.Chains[0].State = core.ChainAbandoned
	original.Definitions[0].Title = "mutated producer"
	original.Traces[0].Status = core.TraceCompleted
	for i := range original.Subtasks {
		if original.Subtasks[i].CompletedAt != nil {
			*original.Subtasks[i].CompletedAt = core.Instant(0)
		}
		original.Subtasks[i].Status = core.SubtaskCancelledDraft
	}
	if !reflect.DeepEqual(p.graph, owned) || p.navigation.counts(liveTestDay) != counts {
		t.Fatal("snapshot alias leaked into confirmed cache")
	}
	cases := []struct {
		name   string
		change func(*core.TaskGraph)
	}{
		{"unknown chain", func(g *core.TaskGraph) { g.Chains[0].State = core.TaskChainState("unknown") }},
		{"unknown child", func(g *core.TaskGraph) { g.Subtasks[0].Status = core.SubtaskStatus("unknown") }},
		{"orphan trace", func(g *core.TaskGraph) { g.Traces[0].DefinitionID = "missing" }},
		{"orphan child", func(g *core.TaskGraph) { g.Subtasks[0].TraceID = "missing" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := owned.Clone()
			tc.change(&bad)
			if p.publish(bad) || p.err == nil {
				t.Fatal("invalid graph accepted")
			}
			s := *f.s
			s.progress = p
			if s.navigationCounts() != (runtimeNavigationCounts{}) {
				t.Fatal("invalid publication exposed badges")
			}
			if p.navigation != projection || !reflect.DeepEqual(p.graph, owned) {
				t.Fatal("invalid publication replaced confirmed facts")
			}
			if !p.publish(owned.Clone()) || p.err != nil || p.navigation != projection {
				t.Fatal("same confirmed graph recovery rebuilt or stayed failed")
			}
			if s.navigationCounts() != counts {
				t.Fatal("recovery count")
			}
		})
	}
}
