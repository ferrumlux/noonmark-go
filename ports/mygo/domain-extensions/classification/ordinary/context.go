// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

func NewEmptyContext(graph core.TaskGraph) (OrdinaryContext, error) {
	return NewEmptyContextWithLimits(graph, DefaultLimits())
}
func NewEmptyContextWithLimits(graph core.TaskGraph, limits Limits) (OrdinaryContext, error) {
	b, e := newBudget(limits)
	if e != nil {
		return OrdinaryContext{}, e
	}
	return newContext(b, graph)
}
func newContext(b *budget, graph core.TaskGraph) (OrdinaryContext, error) {
	if e := b.scan(graph); e != nil {
		return OrdinaryContext{}, e
	}
	// Pay for the validator's indexes, potential issues, and bounded cross-entity
	// work before it can allocate or inspect the input's contents.
	n := uint64(len(graph.Days) + len(graph.Chains) + len(graph.Definitions) + len(graph.Traces) + len(graph.Subtasks))
	for _, c := range graph.Chains {
		n2, err := add(n, uint64(len(c.NoteEntries)))
		if err != nil {
			return OrdinaryContext{}, err
		}
		n = n2
	}
	for _, d := range graph.Definitions {
		n2, err := add(n, uint64(len(d.PlannedSubtasks)))
		if err != nil {
			return OrdinaryContext{}, err
		}
		n = n2
	}
	for _, t := range graph.Traces {
		n2, err := add(n, uint64(len(t.NoteEntries)))
		if err != nil {
			return OrdinaryContext{}, err
		}
		n = n2
	}
	work, e := mul(n, n)
	if e != nil {
		return OrdinaryContext{}, e
	}
	if e = b.scale(work, 32); e != nil {
		return OrdinaryContext{}, e
	}
	if e = b.scan(graph); e != nil {
		return OrdinaryContext{}, e
	} // prospective owned clone
	if !validStrings(graph) {
		return OrdinaryContext{}, ErrInvalid
	}
	for _, chain := range graph.Chains {
		if chain.CycleMembership != nil {
			return OrdinaryContext{}, ErrUnsupported
		}
	}
	if e = core.ValidateTaskGraph(graph); e != nil {
		return OrdinaryContext{}, e
	}
	owned := graph.Clone()
	chains := make(map[core.TaskChainID]struct{}, len(owned.Chains))
	for _, c := range owned.Chains {
		chains[c.ID] = struct{}{}
	}
	return OrdinaryContext{&contextData{owned, chains, b.lim}}, nil
}
func (ctx OrdinaryContext) Graph() (core.TaskGraph, error) {
	b, e := entry(ctx)
	if e != nil {
		return core.TaskGraph{}, e
	}
	if e = b.scan(ctx.sealed.graph); e != nil {
		return core.TaskGraph{}, e
	}
	return ctx.sealed.graph.Clone(), nil
}
func entry(ctx OrdinaryContext, values ...any) (*budget, error) {
	if ctx.sealed == nil {
		return nil, ErrContext
	}
	b, e := newBudget(ctx.sealed.limits)
	if e != nil {
		return nil, e
	}
	if e = b.scan(ctx.sealed.graph); e != nil {
		return nil, e
	}
	if e = b.scan(values...); e != nil {
		return nil, e
	}
	if !validStrings(values...) {
		return nil, ErrInvalid
	}
	return b, nil
}
func validUUID[S ~string](s S) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
func (ctx OrdinaryContext) hasChain(id core.TaskChainID) bool {
	_, ok := ctx.sealed.chains[id]
	return ok
}
