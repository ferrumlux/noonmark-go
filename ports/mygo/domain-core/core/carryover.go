// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"sort"
)

var ErrActiveTraceAlreadyExists = errors.New("active trace already exists")

// TraceCarryoverInput supplies all identities for deterministic replay. ChainID
// is a port routing fence. SubtaskIDs must map exactly the source's open children
// to fresh IDs. DayID is consumed only if TargetDate does not already exist.
type TraceCarryoverInput struct {
	ChainID             TaskChainID
	TraceID, NewTraceID DayTraceID
	DayID               DayID
	SubtaskIDs          map[SubtaskID]SubtaskID
	TargetDate, Today   LocalDate
	Now                 Instant
}

// DeferCurrentTrace implements ordinary task facts only. The aggregate caller
// must capture the source classification snapshot/event in the same transaction
// and independently audit exact replay before persistence or user publication.
func (k *TaskKernel) DeferCurrentTrace(in TraceCarryoverInput) error {
	return k.carryover(in, false)
}

// ContinueUnfinishedTrace preserves every source fact. Neither command grants
// authority to manufacture catalog, classification or other supplement facts.
func (k *TaskKernel) ContinueUnfinishedTrace(in TraceCarryoverInput) error {
	return k.carryover(in, true)
}

func (k *TaskKernel) carryover(in TraceCarryoverInput, continuation bool) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() || !in.TargetDate.Valid() {
			return ErrInvalidInput
		}
		ti, ci, di := -1, -1, -1
		for i, t := range g.Traces {
			if t.ID == in.TraceID {
				ti = i
				break
			}
		}
		if ti < 0 {
			return ErrNotFound
		}
		source := g.Traces[ti]
		if source.ChainID != in.ChainID {
			return ErrInvalidInput
		}
		for i, c := range g.Chains {
			if c.ID == source.ChainID {
				ci = i
				break
			}
		}
		if ci < 0 {
			return ErrNotFound
		}
		if g.Chains[ci].State != ChainActive {
			return ErrChainAbandoned
		}
		sourceLocked, targetLocked, dayExists := false, false, false
		for _, d := range g.Days {
			if d.Date == source.Date {
				sourceLocked = d.LockedAt != nil
			}
			if d.Date == in.TargetDate {
				dayExists = true
				targetLocked = d.LockedAt != nil
			}
		}
		if (!continuation && sourceLocked) || targetLocked {
			return ErrImmutableHistory
		}
		if continuation {
			if source.Status != TraceUnfinished || !(source.Date.Compare(in.Today) < 0 || sourceLocked) {
				return ErrInvalidTransition
			}
			if in.TargetDate.Compare(in.Today) < 0 || in.TargetDate.Compare(source.Date) <= 0 {
				return ErrInvalidTransition
			}
		} else {
			if source.Status != TracePending || source.Date != in.Today {
				return ErrInvalidTransition
			}
			if in.TargetDate.Compare(in.Today) <= 0 {
				return ErrInvalidTransition
			}
		}
		frontier := -1
		for i, t := range g.Traces {
			if t.ChainID == source.ChainID && t.Status.FormsDayHistory() && (frontier < 0 || carryoverTraceEarlier(g.Traces[frontier], t)) {
				frontier = i
			}
		}
		if frontier != ti {
			return ErrInvalidTransition
		}
		for _, t := range g.Traces {
			if t.ChainID == source.ChainID && t.Status == TracePending && (continuation || t.ID != source.ID) {
				return ErrActiveTraceAlreadyExists
			}
		}
		if continuation && g.inTaskPool(g.Chains[ci]) {
			return ErrInvalidTransition
		}
		for i, d := range g.Definitions {
			if d.ChainID == source.ChainID && d.SupersededAt == nil {
				di = i
				break
			}
		}
		if di < 0 {
			return ErrNotFound
		}
		if !validID(string(in.NewTraceID)) {
			return ErrInvalidInput
		}
		priority, hasPriority := 0, false
		for _, t := range g.Traces {
			if t.ID == in.NewTraceID {
				return ErrInvalidInput
			}
			if t.Date == in.TargetDate && t.Status.FormsDayHistory() && (!hasPriority || t.Priority > priority) {
				priority = t.Priority
				hasPriority = true
			}
		}
		maxInt := int(^uint(0) >> 1)
		if priority == maxInt || (continuation && source.ContinuationSeq == maxInt) {
			return ErrInvalidInput
		}
		priority++
		if !dayExists {
			if !validID(string(in.DayID)) {
				return ErrInvalidInput
			}
			for _, d := range g.Days {
				if d.ID == in.DayID {
					return ErrInvalidInput
				}
			}
		}
		var open []Subtask
		used := make(map[SubtaskID]bool, len(g.Subtasks))
		for _, s := range g.Subtasks {
			used[s.ID] = true
			if s.TraceID == source.ID && (s.Status == SubtaskPending || s.Status == SubtaskUnfinished) {
				open = append(open, s)
			}
		}
		sort.Slice(open, func(i, j int) bool { return open[i].Position < open[j].Position })
		if len(in.SubtaskIDs) != len(open) {
			return ErrInvalidInput
		}
		for _, s := range open {
			id, ok := in.SubtaskIDs[s.ID]
			if !ok || !validID(string(id)) || used[id] {
				return ErrInvalidInput
			}
			used[id] = true
		}
		progress, err := NewProgressProjection(*g)
		if err != nil {
			return err
		}
		percent := progress.TraceProgress(source.ID).Percent
		if !in.Now.Finite() {
			return ErrInvalidClock
		}
		next := DayTrace{ID: in.NewTraceID, ChainID: source.ChainID, DefinitionID: g.Definitions[di].ID, Date: in.TargetDate, Status: TracePending, Priority: priority, ContinuationSeq: source.ContinuationSeq, DescriptionText: clonePtr(source.DescriptionText), ManualProgressPercent: &percent, CarriedFromTraceID: clonePtr(&source.ID), CreatedAt: in.Now, ContentUpdatedAt: in.Now}
		for _, n := range source.NoteEntries {
			if n.DeletedAt == nil {
				next.NoteEntries = append(next.NoteEntries, cloneNote(n))
			}
		}
		if continuation {
			next.ContinuationSeq++
		} else {
			if in.Now.Before(source.ContentUpdatedAt) {
				return ErrInvalidClock
			}
			source.ContentUpdatedAt = in.Now
			source.Status = TraceDeferred
			source.PinOrder = nil
			source.SettledAt = clonePtr(&in.Now)
			g.Traces[ti] = source
		}
		g.Traces = append(g.Traces, next)
		for i, old := range open {
			if !continuation {
				if in.Now.Before(old.UpdatedAt) {
					return ErrInvalidClock
				}
				for j := range g.Subtasks {
					if g.Subtasks[j].ID == old.ID {
						g.Subtasks[j].UpdatedAt = in.Now
						g.Subtasks[j].Status = SubtaskDeferred
						g.Subtasks[j].SettledAt = clonePtr(&in.Now)
						break
					}
				}
			}
			g.Subtasks = append(g.Subtasks, Subtask{ID: in.SubtaskIDs[old.ID], LineageID: old.LineageID, TraceID: next.ID, Title: old.Title, Status: SubtaskPending, Difficulty: old.Difficulty, Position: i + 1, CarriedFromSubtaskID: clonePtr(&old.ID), CreatedAt: in.Now, UpdatedAt: in.Now})
		}
		if !dayExists {
			g.Days = append(g.Days, Day{ID: in.DayID, Date: in.TargetDate, CreatedAt: in.Now, UpdatedAt: in.Now})
		}
		// Source touchChain takes max; an unrelated newer chain clock is not an error.
		if in.Now.After(g.Chains[ci].UpdatedAt) {
			g.Chains[ci].UpdatedAt = in.Now
		}
		return nil
	})
}

// Engine.traceChronology, not the pool location/content clock ordering.
func carryoverTraceEarlier(a, b DayTrace) bool {
	if a.Date != b.Date {
		return a.Date.Compare(b.Date) < 0
	}
	if a.ContinuationSeq != b.ContinuationSeq {
		return a.ContinuationSeq < b.ContinuationSeq
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}
