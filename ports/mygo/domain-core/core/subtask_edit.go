// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"strings"
	"unicode/utf8"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/textcanon"
)

// SubtaskEditKind names only source-supported commands. Pool removal and Day
// cancellation deliberately have different kinds and different facts.
type SubtaskEditKind string

const (
	SubtaskAdd           SubtaskEditKind = "add"
	SubtaskTitle         SubtaskEditKind = "title"
	SubtaskDifficulty    SubtaskEditKind = "difficulty"
	PlannedSubtaskRemove SubtaskEditKind = "removePlanned"
	SubtaskDelete        SubtaskEditKind = "delete"
	SubtaskComplete      SubtaskEditKind = "complete"
	SubtaskUndoComplete  SubtaskEditKind = "undoComplete"
	SubtaskAbandon       SubtaskEditKind = "abandon"
)

// ExpectedDefinitionID is an explicit stale-owner fence in this port's API.
// New identities are supplied for deterministic command replay, never invented.
type PlannedSubtaskEditInput struct {
	Kind                 SubtaskEditKind
	ChainID              TaskChainID
	ExpectedDefinitionID TaskDefinitionID
	SubtaskID            PlannedSubtaskID
	LineageID            SubtaskLineageID
	Title                string
	Difficulty           Difficulty
	Now                  Instant
}

type SubtaskEditInput struct {
	Kind           SubtaskEditKind
	ChainID        TaskChainID
	TraceID        DayTraceID
	SubtaskID      SubtaskID
	LineageID      SubtaskLineageID
	Title          string
	Difficulty     Difficulty
	CancellationID CancellationID
	Today          LocalDate
	Now            Instant
}

func subtaskEditTitle(title string) (string, error) {
	if !utf8.ValidString(title) {
		return "", ErrInvalidTitle
	}
	title = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(title, "\r\n", "\n"), "\r", "\n"))
	if title == "" {
		return "", ErrInvalidTitle
	}
	return title, nil
}
func freshSubtaskLineage(g *TaskGraph, id SubtaskLineageID) bool {
	if !validID(string(id)) {
		return false
	}
	for _, d := range g.Definitions {
		for _, s := range d.PlannedSubtasks {
			if s.LineageID == id {
				return false
			}
		}
	}
	for _, s := range g.Subtasks {
		if s.LineageID == id {
			return false
		}
	}
	return true
}
func freshSubtaskCancellation(g *TaskGraph, id CancellationID) bool {
	if !validID(string(id)) {
		return false
	}
	for _, t := range g.Traces {
		if t.DraftCancellationID != nil && *t.DraftCancellationID == id {
			return false
		}
	}
	for _, s := range g.Subtasks {
		if s.DraftCancellationID != nil && *s.DraftCancellationID == id {
			return false
		}
	}
	return true
}

// EditPlannedSubtask follows Engine 1324-1423: mutate the pool's current
// definition in place, preserve identities, and touch only definition/chain.
func (k *TaskKernel) EditPlannedSubtask(in PlannedSubtaskEditInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !validID(string(in.ChainID)) || !validID(string(in.ExpectedDefinitionID)) || !validID(string(in.SubtaskID)) {
			return ErrInvalidInput
		}
		switch in.Kind {
		case SubtaskAdd:
			if !validDifficulty(in.Difficulty) || !freshSubtaskLineage(g, in.LineageID) {
				return ErrInvalidInput
			}
		case SubtaskTitle:
			if in.LineageID != "" || in.Difficulty != 0 {
				return ErrInvalidInput
			}
		case SubtaskDifficulty:
			if in.LineageID != "" || in.Title != "" || !validDifficulty(in.Difficulty) {
				return ErrInvalidInput
			}
		case PlannedSubtaskRemove:
			if in.LineageID != "" || in.Title != "" || in.Difficulty != 0 {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
		var title string
		if in.Kind == SubtaskAdd || in.Kind == SubtaskTitle {
			var err error
			title, err = subtaskEditTitle(in.Title)
			if err != nil {
				return err
			}
		}
		ci, di := -1, -1
		for i, c := range g.Chains {
			if c.ID == in.ChainID {
				ci = i
				break
			}
		}
		if ci < 0 {
			return ErrNotFound
		}
		if !g.inTaskPool(g.Chains[ci]) {
			return ErrInvalidTransition
		}
		for i, d := range g.Definitions {
			if d.ChainID == in.ChainID && d.SupersededAt == nil {
				di = i
				break
			}
		}
		if di < 0 {
			return ErrNotFound
		}
		d, c := &g.Definitions[di], &g.Chains[ci]
		if d.ID != in.ExpectedDefinitionID {
			return ErrInvalidTransition
		}
		si := -1
		for i, s := range d.PlannedSubtasks {
			if s.ID == in.SubtaskID {
				si = i
				break
			}
		}
		if in.Kind == SubtaskAdd {
			for _, old := range g.Definitions {
				for _, s := range old.PlannedSubtasks {
					if s.ID == in.SubtaskID {
						return ErrInvalidInput
					}
				}
			}
			if len(d.PlannedSubtasks) == int(^uint(0)>>1) {
				return ErrInvalidInput
			}
		} else {
			if si < 0 {
				return ErrNotFound
			}
			if in.Kind == SubtaskTitle && textcanon.Equivalent(d.PlannedSubtasks[si].Title, title) {
				return nil
			}
			if in.Kind == SubtaskDifficulty && d.PlannedSubtasks[si].Difficulty == in.Difficulty {
				return nil
			}
		}
		if !in.Now.Finite() || in.Now.Before(d.ContentUpdatedAt) || in.Now.Before(c.UpdatedAt) {
			return ErrInvalidClock
		}
		switch in.Kind {
		case SubtaskAdd:
			d.PlannedSubtasks = append(d.PlannedSubtasks, PlannedSubtask{ID: in.SubtaskID, LineageID: in.LineageID, Title: title, Difficulty: in.Difficulty, Position: len(d.PlannedSubtasks) + 1, CreatedAt: in.Now})
		case SubtaskTitle:
			d.PlannedSubtasks[si].Title = title
		case SubtaskDifficulty:
			d.PlannedSubtasks[si].Difficulty = in.Difficulty
		case PlannedSubtaskRemove:
			d.PlannedSubtasks = append(d.PlannedSubtasks[:si], d.PlannedSubtasks[si+1:]...)
			for i := range d.PlannedSubtasks {
				d.PlannedSubtasks[i].Position = i + 1
			}
		}
		d.ContentUpdatedAt = in.Now
		c.UpdatedAt = in.Now
		return nil
	})
}

// EditSubtask preserves parent/definition bytes. Source Day edits only advance
// the actual child clock. Unlike the source's low-level add, this routed command
// also rejects historical additions: the source UI imposes that same boundary.
func (k *TaskKernel) EditSubtask(in SubtaskEditInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() || !validID(string(in.ChainID)) || !validID(string(in.TraceID)) || !validID(string(in.SubtaskID)) {
			return ErrInvalidInput
		}
		switch in.Kind {
		case SubtaskAdd:
			if !validDifficulty(in.Difficulty) || !freshSubtaskLineage(g, in.LineageID) || in.CancellationID != "" {
				return ErrInvalidInput
			}
		case SubtaskTitle:
			if in.LineageID != "" || in.Difficulty != 0 || in.CancellationID != "" {
				return ErrInvalidInput
			}
		case SubtaskDifficulty:
			if in.LineageID != "" || in.Title != "" || !validDifficulty(in.Difficulty) || in.CancellationID != "" {
				return ErrInvalidInput
			}
		case SubtaskDelete:
			if in.LineageID != "" || in.Title != "" || in.Difficulty != 0 || !freshSubtaskCancellation(g, in.CancellationID) {
				return ErrInvalidInput
			}
		case SubtaskComplete, SubtaskUndoComplete, SubtaskAbandon:
			if in.LineageID != "" || in.Title != "" || in.Difficulty != 0 || in.CancellationID != "" {
				return ErrInvalidInput
			}
		default:
			return ErrInvalidInput
		}
		var title string
		if in.Kind == SubtaskAdd || in.Kind == SubtaskTitle {
			var err error
			title, err = subtaskEditTitle(in.Title)
			if err != nil {
				return err
			}
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
		if trace.Date.Compare(in.Today) < 0 {
			return ErrImmutableHistory
		}
		for _, d := range g.Days {
			if d.Date == trace.Date && d.LockedAt != nil {
				return ErrImmutableHistory
			}
		}
		if trace.Status != TracePending {
			if in.Kind == SubtaskAdd {
				return ErrInvalidTransition
			}
			return ErrImmutableHistory
		}
		if (in.Kind == SubtaskComplete || in.Kind == SubtaskUndoComplete || in.Kind == SubtaskAbandon) && trace.Date != in.Today {
			return ErrImmutableHistory
		}
		if in.Kind == SubtaskAdd {
			count := 0
			for _, s := range g.Subtasks {
				if s.ID == in.SubtaskID {
					return ErrInvalidInput
				}
				if s.TraceID == in.TraceID {
					count++
				}
			}
			if count == int(^uint(0)>>1) {
				return ErrInvalidInput
			}
			if !in.Now.Finite() {
				return ErrInvalidClock
			}
			g.Subtasks = append(g.Subtasks, Subtask{ID: in.SubtaskID, LineageID: in.LineageID, TraceID: in.TraceID, Title: title, Status: SubtaskPending, Difficulty: in.Difficulty, Position: count + 1, CreatedAt: in.Now, UpdatedAt: in.Now})
			return nil
		}
		si := -1
		for i, s := range g.Subtasks {
			if s.ID == in.SubtaskID {
				si = i
				break
			}
		}
		if si < 0 {
			return ErrNotFound
		}
		s := &g.Subtasks[si]
		if s.TraceID != in.TraceID {
			return ErrInvalidInput
		}
		if s.Status == SubtaskCancelledDraft {
			return ErrNotFound
		}
		if in.Kind == SubtaskTitle && textcanon.Equivalent(s.Title, title) {
			return nil
		}
		if in.Kind == SubtaskDifficulty && s.Difficulty == in.Difficulty {
			return nil
		}
		if in.Kind == SubtaskComplete || in.Kind == SubtaskAbandon {
			if s.Status != SubtaskPending {
				return ErrInvalidTransition
			}
		}
		if in.Kind == SubtaskUndoComplete && s.Status != SubtaskCompleted {
			return ErrInvalidTransition
		}
		if !in.Now.Finite() || in.Now.Before(s.UpdatedAt) {
			return ErrInvalidClock
		}
		switch in.Kind {
		case SubtaskTitle:
			s.Title = title
		case SubtaskDifficulty:
			s.Difficulty = in.Difficulty
		case SubtaskDelete:
			s.Status = SubtaskCancelledDraft
			s.CompletedAt = nil
			s.SettledAt = clonePtr(&in.Now)
			s.DraftCancellationID = clonePtr(&in.CancellationID)
		case SubtaskComplete:
			s.Status = SubtaskCompleted
			s.CompletedAt = clonePtr(&in.Now)
		case SubtaskUndoComplete:
			s.Status = SubtaskPending
			s.CompletedAt = nil
		case SubtaskAbandon:
			s.Status = SubtaskAbandoned
			s.SettledAt = clonePtr(&in.Now)
		}
		s.UpdatedAt = in.Now
		return nil
	})
}
