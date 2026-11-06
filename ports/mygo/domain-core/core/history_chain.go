// SPDX-License-Identifier: AGPL-3.0-only
package core

// AbandonChainInput binds the requested trace to its owner. The request may
// target a historical trace, while the command changes that chain's pending
// trace instead. No civil date or new identity is consumed by this command.
type AbandonChainInput struct {
	ChainID TaskChainID
	TraceID DayTraceID
	Now     Instant
}

// ReactivateAbandonedChainInput names the abandoned trace to reactivate in
// place. Today determines pending versus unfinished only when its day is open.
type ReactivateAbandonedChainInput struct {
	ChainID TaskChainID
	TraceID DayTraceID
	Today   LocalDate
	Now     Instant
}

// AbandonChain implements ordinary task facts, including activeTrace routing.
// Success does not mean the requested historical trace changed. The aggregate
// caller must independently resolve and audit the actual target, then capture
// its abandoned classification snapshot in the same persistence transaction.
func (k *TaskKernel) AbandonChain(in AbandonChainInput) error {
	return k.mutate(func(g *TaskGraph) error {
		ti, ci, err := historyChainTarget(g, in.ChainID, in.TraceID)
		if err != nil {
			return err
		}
		chain := &g.Chains[ci]
		if chain.State != ChainActive {
			return ErrChainAbandoned
		}
		if active := historyChainActiveTrace(g, chain.ID); active >= 0 {
			ti = active
		}
		trace := &g.Traces[ti]
		if trace.Status != TracePending && trace.Status != TraceUnfinished {
			return ErrInvalidTransition
		}
		if !historyChainClockValid(*trace, *chain, in.Now) {
			return ErrInvalidClock
		}
		trace.ContentUpdatedAt = in.Now
		trace.Status = TraceAbandoned
		trace.PinOrder = nil
		trace.SettledAt = clonePtr(&in.Now)
		chain.UpdatedAt = in.Now
		chain.State = ChainAbandoned
		return nil
	})
}

// ReactivateAbandonedChain returns the original trace identity after successful
// validation, or an empty ID on every failure. It neither copies children or
// notes nor removes historical classification facts. An aggregate caller must
// capture unfinished results, but preserve existing audit unchanged for pending.
func (k *TaskKernel) ReactivateAbandonedChain(in ReactivateAbandonedChainInput) (DayTraceID, error) {
	err := k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() {
			return ErrInvalidInput
		}
		ti, ci, err := historyChainTarget(g, in.ChainID, in.TraceID)
		if err != nil {
			return err
		}
		source, chain := &g.Traces[ti], &g.Chains[ci]
		if chain.State != ChainAbandoned || source.Status != TraceAbandoned {
			return ErrInvalidTransition
		}
		if historyChainActiveTrace(g, chain.ID) >= 0 {
			return ErrActiveTraceAlreadyExists
		}
		if !historyChainClockValid(*source, *chain, in.Now) {
			return ErrInvalidClock
		}
		locked := false
		for _, d := range g.Days {
			if d.Date == source.Date {
				locked = d.LockedAt != nil
				break
			}
		}
		source.ContentUpdatedAt = in.Now
		if locked || source.Date.Compare(in.Today) < 0 {
			source.Status = TraceUnfinished
			if source.SettledAt == nil {
				source.SettledAt = clonePtr(&in.Now)
			}
		} else {
			source.Status = TracePending
			source.SettledAt = nil
		}
		source.PinOrder = nil
		chain.UpdatedAt = in.Now
		chain.State = ChainActive
		return nil
	})
	if err != nil {
		return "", err
	}
	return in.TraceID, nil
}

// The owner fence precedes activeTrace redirection and all state checks. The
// recurring guard is defensive: NewTaskKernel already rejects recurring graphs.
func historyChainTarget(g *TaskGraph, chainID TaskChainID, traceID DayTraceID) (int, int, error) {
	for ti, trace := range g.Traces {
		if trace.ID != traceID {
			continue
		}
		if trace.ChainID != chainID {
			return -1, -1, ErrInvalidInput
		}
		for ci, chain := range g.Chains {
			if chain.ID == trace.ChainID {
				if chain.CycleMembership != nil {
					return -1, -1, ErrUnsupportedRecurring
				}
				return ti, ci, nil
			}
		}
		break
	}
	return -1, -1, ErrNotFound
}

// Swift activeTrace sorts pending traces by createdAt descending. Valid bounded
// graphs have at most one pending trace per chain; unfinished is never active.
func historyChainActiveTrace(g *TaskGraph, chainID TaskChainID) int {
	active := -1
	for i, trace := range g.Traces {
		if trace.ChainID == chainID && trace.Status == TracePending &&
			(active < 0 || trace.CreatedAt.After(g.Traces[active].CreatedAt)) {
			active = i
		}
	}
	return active
}

// These commands use markContentModified on both objects, not touchChain's max.
func historyChainClockValid(trace DayTrace, chain TaskChain, now Instant) bool {
	return now.Finite() && trace.ContentUpdatedAt.Finite() && chain.UpdatedAt.Finite() &&
		!now.Before(trace.ContentUpdatedAt) && !now.Before(chain.UpdatedAt)
}
