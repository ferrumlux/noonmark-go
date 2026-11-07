// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

// ExportCandidate checks decode+integrity and reserves owned typed outputs and
// 6x encoder expansion BEFORE cloning them. The outputs are offline candidates,
// never write authority or causal proof. No normalization is left to the codec.
func ExportCandidate(ctx OrdinaryContext, state State) (core.TaskGraph, State, error) {
	b, e := entry(ctx, state)
	if e != nil {
		return core.TaskGraph{}, State{}, e
	}
	// Bounded neutral preferences, omitted empty supplements and envelope keys.
	if e = b.charge(8192, 256); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	if e = b.scan(ctx.sealed.graph, state); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	if e = b.reserveEncoding(ctx.sealed.graph); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	if e = b.reserveEncoding(state); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	if e = validateIntegrity(b, ctx, state); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	if e = validateDecodedEntities(b, state); e != nil {
		return core.TaskGraph{}, State{}, e
	}
	return ctx.sealed.graph.Clone(), cloneState(state), nil
}

// ImportCandidateWithLimits gives offline adapters one shared typed ledger for
// graph validation/cloning and state decode/integrity/cloning. The adapter must
// FIRST bound and reject unsupported complete-wire supplements, and subtract
// its input/projection cost from limits. No imported candidate is ReplayProof.
func ImportCandidateWithLimits(graph core.TaskGraph, state State, limits Limits) (OrdinaryContext, State, error) {
	b, e := newBudget(limits)
	if e != nil {
		return OrdinaryContext{}, State{}, e
	}
	if e = b.scan(graph, state); e != nil {
		return OrdinaryContext{}, State{}, e
	}
	if !validStrings(graph, state) {
		return OrdinaryContext{}, State{}, ErrInvalid
	}
	ctx, e := newContext(b, graph)
	if e != nil {
		return OrdinaryContext{}, State{}, e
	}
	if e = b.scan(state); e != nil {
		return OrdinaryContext{}, State{}, e
	}
	if e = validateIntegrity(b, ctx, state); e != nil {
		return OrdinaryContext{}, State{}, e
	}
	if e = validateDecodedEntities(b, state); e != nil {
		return OrdinaryContext{}, State{}, e
	}
	return ctx, cloneState(state), nil
}
