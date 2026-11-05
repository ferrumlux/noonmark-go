// SPDX-License-Identifier: AGPL-3.0-only
package core

import "sort"

// ReturnToPoolInput supplies fresh identities explicitly, making replay exact.
// ChainID binds a selected trace to its UI owner. CancellationID is used only
// for future plans; PlannedIDs contains exactly the retained open subtasks.
type ReturnToPoolInput struct {
	TraceID         DayTraceID
	ChainID         TaskChainID
	NewDefinitionID TaskDefinitionID
	CancellationID  CancellationID
	PlannedIDs      map[SubtaskID]PlannedSubtaskID
	Today           LocalDate
	Now             Instant
}

// ReturnToPool preserves today's trace, but hides a cancelled future plan.
// Classification capture belongs to the caller's atomic aggregate transaction.
func (k *TaskKernel) ReturnToPool(in ReturnToPoolInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() || !validID(string(in.NewDefinitionID)) {
			return ErrInvalidInput
		}
		if !in.Now.Finite() {
			return ErrInvalidClock
		}
		ti, ci, di, seq := -1, -1, -1, 0
		for i, t := range g.Traces {
			if t.ID == in.TraceID {
				ti = i
			}
		}
		if ti < 0 {
			return ErrNotFound
		}
		t := &g.Traces[ti]
		if t.ChainID != in.ChainID {
			return ErrInvalidInput
		}
		for i, c := range g.Chains {
			if c.ID == t.ChainID {
				ci = i
			}
		}
		if ci < 0 {
			return ErrNotFound
		}
		// The constructor currently rejects recurring parents too. Keep this
		// command's own upstream guard if the kernel scope later expands.
		if g.Chains[ci].CycleMembership != nil {
			return ErrInvalidTransition
		}
		for _, d := range g.Days {
			if d.Date == t.Date && d.LockedAt != nil {
				return ErrImmutableHistory
			}
		}
		if t.Date.Compare(in.Today) < 0 {
			return ErrImmutableHistory
		}
		if t.Status != TracePending {
			return ErrInvalidTransition
		}
		for i, d := range g.Definitions {
			if d.ID == in.NewDefinitionID {
				return ErrInvalidInput
			}
			if d.ChainID == t.ChainID {
				if d.Sequence > seq {
					seq = d.Sequence
				}
				if d.SupersededAt == nil {
					di = i
				}
			}
		}
		if di < 0 {
			return ErrNotFound
		}
		if seq == int(^uint(0)>>1) {
			return ErrInvalidInput
		}
		c, d := &g.Chains[ci], &g.Definitions[di]
		if in.Now.Before(t.ContentUpdatedAt) || in.Now.Before(c.UpdatedAt) || in.Now.Before(d.ContentUpdatedAt) {
			return ErrInvalidClock
		}
		future := t.Date.Compare(in.Today) > 0
		if future && !validID(string(in.CancellationID)) {
			return ErrInvalidInput
		}
		// Freshness is checked against BEFORE facts: future normalization below
		// clears hidden-child cancellation IDs and must not erase collision proof.
		if future {
			for _, old := range g.Traces {
				if old.DraftCancellationID != nil && *old.DraftCancellationID == in.CancellationID {
					return ErrInvalidInput
				}
			}
			for _, old := range g.Subtasks {
				if old.DraftCancellationID != nil && *old.DraftCancellationID == in.CancellationID {
					return ErrInvalidInput
				}
			}
		}
		usedPlanned := map[PlannedSubtaskID]bool{}
		for _, old := range g.Definitions {
			for _, planned := range old.PlannedSubtasks {
				usedPlanned[planned.ID] = true
			}
		}
		var retained []Subtask
		for _, s := range g.Subtasks {
			if s.TraceID == t.ID && (s.Status == SubtaskPending || s.Status == SubtaskUnfinished) {
				retained = append(retained, s)
			}
		}
		sort.Slice(retained, func(i, j int) bool { return retained[i].Position < retained[j].Position })
		if len(in.PlannedIDs) != len(retained) {
			return ErrInvalidInput
		}
		planned := make([]PlannedSubtask, 0, len(retained))
		for i, s := range retained {
			id, ok := in.PlannedIDs[s.ID]
			if !ok || !validID(string(id)) || usedPlanned[id] {
				return ErrInvalidInput
			}
			usedPlanned[id] = true
			planned = append(planned, PlannedSubtask{ID: id, LineageID: s.LineageID, Title: s.Title, Difficulty: s.Difficulty, Position: i + 1, CreatedAt: in.Now})
		}
		next := TaskDefinition{ID: in.NewDefinitionID, ChainID: t.ChainID, Sequence: seq + 1, Title: d.Title, DescriptionText: clonePtr(t.DescriptionText), PlannedSubtasks: planned, CreatedAt: in.Now, ContentUpdatedAt: in.Now}
		d.ContentUpdatedAt = in.Now
		d.SupersededAt = clonePtr(&in.Now)
		d.SupersededByDefinitionID = clonePtr(&in.NewDefinitionID)
		c.NoteEntries = nil
		for _, n := range t.NoteEntries {
			c.NoteEntries = append(c.NoteEntries, cloneNote(n))
		}
		c.UpdatedAt = in.Now
		if future {
			for i := range g.Subtasks {
				s := &g.Subtasks[i]
				if s.TraceID == t.ID && s.Status == SubtaskCancelledDraft {
					if in.Now.Before(s.UpdatedAt) {
						return ErrInvalidClock
					}
					s.Status = SubtaskPending
					s.UpdatedAt = in.Now
					s.CompletedAt = nil
					s.SettledAt = nil
					s.DraftCancellationID = nil
				}
			}
			t.Status = TraceCancelledDraft
			t.DraftCancellationID = clonePtr(&in.CancellationID)
			t.DraftCancelledOn = clonePtr(&in.Today)
		} else {
			t.Status = TraceReturnedToPool
		}
		t.PinOrder = nil
		t.SettledAt = clonePtr(&in.Now)
		t.ContentUpdatedAt = in.Now
		g.Definitions = append(g.Definitions, next)
		return nil
	})
}
