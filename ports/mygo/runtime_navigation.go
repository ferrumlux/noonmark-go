// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Counts are navigation facts, not lengths of the currently supported page
// rows. Swift's completed/unfinished pages group by chain; completed children
// also make a hierarchy count even while their parent is still pending.
type runtimeNavigationCounts struct {
	day, pool, upcoming, unfinished, completed int
}

func (n runtimeNavigationCounts) forPage(page string) int {
	switch page {
	case "day":
		return n.day
	case "pool":
		return n.pool
	case "upcoming":
		return n.upcoming
	case "unfinished":
		return n.unfinished
	case "completed":
		return n.completed
	}
	return 0
}

type runtimeNavigationProjection struct {
	pool, unfinished, completed int
	pendingByDate               map[core.LocalDate]int
	futureCandidateDates        []core.LocalDate
}

type navigationChainFacts struct {
	latest                         *core.DayTrace
	pending, unfinished, completed bool
}

// Called only after the shared progress publication gate validates the graph.
// This has no graph ownership and no extra kernel/clone. The runtime Service's
// ordinary-task scope is unchanged; this does not implement recurring windows,
// classification badges or the full completed/unfinished page hierarchies.
func newRuntimeNavigationProjection(g core.TaskGraph) *runtimeNavigationProjection {
	p := &runtimeNavigationProjection{pendingByDate: map[core.LocalDate]int{}}
	definitions := make(map[core.TaskDefinitionID]bool, len(g.Definitions))
	current := make(map[core.TaskChainID]bool, len(g.Chains))
	ordinary := make(map[core.TaskChainID]bool, len(g.Chains))
	for _, c := range g.Chains {
		ordinary[c.ID] = c.CycleMembership == nil
	}
	for _, d := range g.Definitions {
		definitions[d.ID] = true
		if d.SupersededAt == nil {
			current[d.ChainID] = true
		}
	}
	facts := make(map[core.TaskChainID]navigationChainFacts, len(g.Chains))
	traces := make(map[core.DayTraceID]*core.DayTrace, len(g.Traces))
	completed := make(map[core.TaskChainID]bool)
	for i := range g.Traces {
		tr := &g.Traces[i]
		traces[tr.ID] = tr
		f := facts[tr.ChainID]
		if tr.Status == core.TracePending {
			f.pending = true
			// getDayTodo suppresses superseded deferred traces, never pending traces.
			p.pendingByDate[tr.Date]++
			if definitions[tr.DefinitionID] {
				p.futureCandidateDates = append(p.futureCandidateDates, tr.Date)
			}
		}
		if tr.Status == core.TraceUnfinished || tr.Status == core.TraceAbandoned {
			f.unfinished = true
		}
		if tr.Status == core.TraceCompleted {
			f.completed = true
			if ordinary[tr.ChainID] && definitions[tr.DefinitionID] {
				completed[tr.ChainID] = true
			}
		}
		if f.latest == nil || tr.ContentUpdatedAt.After(f.latest.ContentUpdatedAt) || tr.ContentUpdatedAt.Equal(f.latest.ContentUpdatedAt) && (tr.CreatedAt.After(f.latest.CreatedAt) || tr.CreatedAt.Equal(f.latest.CreatedAt) && tr.ID > f.latest.ID) {
			f.latest = tr
		}
		facts[tr.ChainID] = f
	}
	for _, chain := range g.Chains {
		if !ordinary[chain.ID] || !current[chain.ID] {
			continue
		}
		f := facts[chain.ID]
		if chain.State == core.ChainActive && !f.pending && (f.latest == nil || f.latest.Status == core.TraceReturnedToPool || f.latest.Status == core.TraceCancelledDraft) {
			p.pool++
		}
		if (chain.State == core.ChainActive || chain.State == core.ChainAbandoned) && f.unfinished && !f.completed {
			p.unfinished++
		}
	}
	for _, child := range g.Subtasks {
		tr := traces[child.TraceID]
		if child.Status == core.SubtaskCompleted && tr != nil && tr.Status.FormsDayHistory() && ordinary[tr.ChainID] && definitions[tr.DefinitionID] {
			completed[tr.ChainID] = true
		}
	}
	p.completed = len(completed)
	sort.Slice(p.futureCandidateDates, func(i, j int) bool { return p.futureCandidateDates[i].Compare(p.futureCandidateDates[j]) < 0 })
	return p
}

func (p *runtimeNavigationProjection) counts(today core.LocalDate) runtimeNavigationCounts {
	if p == nil || !today.Valid() {
		return runtimeNavigationCounts{}
	}
	// Date is a query key rather than a graph field: a confirmed date-only
	// publication must update badges without reconstructing an identical graph.
	firstFuture := sort.Search(len(p.futureCandidateDates), func(i int) bool { return p.futureCandidateDates[i].Compare(today) > 0 })
	return runtimeNavigationCounts{day: p.pendingByDate[today], pool: p.pool, upcoming: len(p.futureCandidateDates) - firstFuture, unfinished: p.unfinished, completed: p.completed}
}
func (s *app) navigationCounts() runtimeNavigationCounts {
	if s.progress == nil || s.progress.err != nil {
		return runtimeNavigationCounts{}
	}
	return s.progress.navigation.counts(s.runtime.Today())
}
