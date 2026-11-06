// SPDX-License-Identifier: AGPL-3.0-only
package core

// SetManualProgressInput binds the selected trace to its owner. ChainID is a
// port routing fence; the source command addresses TraceID directly.
type SetManualProgressInput struct {
	ChainID TaskChainID
	TraceID DayTraceID
	Percent int
	Today   LocalDate
	Now     Instant
}

// SetManualProgress follows Engine 2842-2864. In particular, 100 is not a
// completion command, nil differs from zero, and a no-op never reads Now.
func (k *TaskKernel) SetManualProgress(in SetManualProgressInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() {
			return ErrInvalidInput
		}
		var trace *DayTrace
		for i := range g.Traces {
			if g.Traces[i].ID == in.TraceID {
				trace = &g.Traces[i]
				break
			}
		}
		if trace == nil {
			return ErrNotFound
		}
		if trace.ChainID != in.ChainID {
			return ErrInvalidInput
		}
		for _, day := range g.Days {
			if day.Date == trace.Date && day.LockedAt != nil {
				return ErrImmutableHistory
			}
		}
		if trace.Date != in.Today || trace.Status != TracePending {
			return ErrImmutableHistory
		}
		progress, err := NewProgressProjection(*g)
		if err != nil {
			return err
		}
		if progress.chainsWithSubtasks[trace.ChainID] {
			return ErrInvalidTransition
		}
		next := max(progress.TraceProgress(trace.ID).FloorPercent, min(100, max(0, in.Percent)))
		if trace.ManualProgressPercent != nil && *trace.ManualProgressPercent == next {
			return nil
		}
		if !in.Now.Finite() || in.Now.Before(trace.ContentUpdatedAt) {
			return ErrInvalidClock
		}
		trace.ContentUpdatedAt = in.Now
		trace.ManualProgressPercent = &next
		return nil
	})
}
