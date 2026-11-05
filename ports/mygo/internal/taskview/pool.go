// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"fmt"
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// PoolRow is a detached selector entry. CreatedAt belongs to the current
// definition, not the chain or its latest trace.
type PoolRow struct {
	ChainID      core.TaskChainID
	DefinitionID core.TaskDefinitionID
	Title        string
	CreatedAt    core.Instant
}

// ProjectPool validates the entire bounded graph before selecting active,
// non-recurring pool tasks. It does not read the clock, run commands, or mutate
// the graph. Validation here does not validate omitted snapshot supplements.
func ProjectPool(graph core.TaskGraph) ([]PoolRow, error) {
	if err := core.ValidateTaskGraph(graph); err != nil {
		return nil, fmt.Errorf("pool graph: %w", err)
	}
	definitions := make(map[core.TaskChainID]core.TaskDefinition)
	for _, d := range graph.Definitions {
		if d.SupersededAt == nil {
			definitions[d.ChainID] = d
		}
	}
	latest := make(map[core.TaskChainID]core.DayTrace)
	pending := make(map[core.TaskChainID]bool)
	for _, trace := range graph.Traces {
		if trace.Status == core.TracePending {
			pending[trace.ChainID] = true
		}
		previous, exists := latest[trace.ChainID]
		if !exists || poolTraceLater(trace, previous) {
			latest[trace.ChainID] = trace
		}
	}
	rows := make([]PoolRow, 0)
	for _, chain := range graph.Chains {
		if chain.State != core.ChainActive || chain.CycleMembership != nil || pending[chain.ID] {
			continue
		}
		trace, exists := latest[chain.ID]
		if exists && trace.Status != core.TraceReturnedToPool && trace.Status != core.TraceCancelledDraft {
			continue
		}
		definition := definitions[chain.ID] // Validation guarantees exactly one current definition.
		rows = append(rows, PoolRow{ChainID: chain.ID, DefinitionID: definition.ID, Title: definition.Title, CreatedAt: definition.CreatedAt})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		// Swift specifies only definition creation time. Make ties deterministic
		// across platforms and independent of graph slice / dictionary order.
		return rows[i].ChainID < rows[j].ChainID
	})
	return rows, nil
}

func poolTraceLater(a, b core.DayTrace) bool {
	if !a.ContentUpdatedAt.Equal(b.ContentUpdatedAt) {
		return a.ContentUpdatedAt.After(b.ContentUpdatedAt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}
