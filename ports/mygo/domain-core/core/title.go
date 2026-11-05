// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// EditTaskTitleInput supplies every external fact needed for a title edit.
// NewDefinitionID is required only when the command creates a version; an
// unused identity is neither checked nor reserved. Existing titles may be empty.
// This task-graph command does not authorize persistence or UI editability.
type EditTaskTitleInput struct {
	ChainID         TaskChainID
	Title           string
	Today           LocalDate
	Now             Instant
	NewDefinitionID TaskDefinitionID
}

// RenameTaskTitle creates a new definition whenever the chain has day history,
// including pending facts. Historical trace content and references stay intact.
func (k *TaskKernel) RenameTaskTitle(in EditTaskTitleInput) error {
	return k.updateTaskTitle(in, false)
}

// SaveTaskTitleInput reuses the current definition when no non-pending day fact
// references it. Character-level saves therefore do not create redundant versions.
func (k *TaskKernel) SaveTaskTitleInput(in EditTaskTitleInput) error {
	return k.updateTaskTitle(in, true)
}

func (k *TaskKernel) updateTaskTitle(in EditTaskTitleInput, reuseEditable bool) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Today.Valid() {
			return ErrInvalidInput
		}
		// Foundation whitespace parity still needs an Apple oracle. Keep the
		// existing kernel's Unicode trim contract, but normalize CRLF/CR first.
		title := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(in.Title, "\r\n", "\n"), "\r", "\n"))
		var chain *TaskChain
		for i := range g.Chains {
			if g.Chains[i].ID == in.ChainID {
				chain = &g.Chains[i]
				break
			}
		}
		if chain == nil {
			return ErrNotFound
		}
		if chain.State != ChainActive {
			return ErrChainAbandoned
		}
		var traces []int
		editable := false
		for i, tr := range g.Traces {
			if tr.ChainID != in.ChainID || !tr.Status.FormsDayHistory() {
				continue
			}
			traces = append(traces, i)
			if tr.Status == TracePending || tr.Status == TraceReturnedToPool {
				editable = true
			}
		}
		if len(traces) != 0 && !editable {
			return ErrInvalidTransition
		}
		// These capability guards precede even a normalized-title no-op.
		for _, i := range traces {
			tr := &g.Traces[i]
			if tr.Status == TracePending && tr.Date.Compare(in.Today) < 0 {
				return ErrImmutableHistory
			}
		}
		for _, i := range traces {
			tr := &g.Traces[i]
			if tr.Status != TracePending {
				continue
			}
			for _, day := range g.Days {
				if day.Date == tr.Date && day.LockedAt != nil {
					return ErrImmutableHistory
				}
			}
		}
		current := -1
		maxSequence := 0
		for i, d := range g.Definitions {
			if d.ChainID != in.ChainID {
				continue
			}
			if d.Sequence > maxSequence {
				maxSequence = d.Sequence
			}
			// A restored current definition may retain a successor witness.
			if d.SupersededAt == nil {
				current = i
			}
		}
		if current < 0 {
			return ErrNotFound
		}
		d := &g.Definitions[current]
		// Swift String equality is canonical-equivalence aware. Compare NFC
		// without normalizing either stored value: a no-op preserves its bytes.
		if norm.NFC.String(d.Title) == norm.NFC.String(title) {
			return nil
		}
		if !in.Now.Finite() || in.Now.Before(d.ContentUpdatedAt) {
			return ErrInvalidClock
		}
		historicalReference := false
		for _, i := range traces {
			tr := &g.Traces[i]
			if tr.DefinitionID == d.ID && tr.Status != TracePending {
				historicalReference = true
			}
		}
		if len(traces) == 0 || (reuseEditable && !historicalReference) {
			d.Title = title
			d.ContentUpdatedAt = in.Now
		} else {
			if maxSequence == int(^uint(0)>>1) || !validID(string(in.NewDefinitionID)) {
				return ErrInvalidInput
			}
			for _, old := range g.Definitions {
				if old.ID == in.NewDefinitionID {
					return ErrInvalidInput
				}
			}
			for _, i := range traces {
				tr := &g.Traces[i]
				if tr.Status == TracePending && in.Now.Before(tr.ContentUpdatedAt) {
					return ErrInvalidClock
				}
			}
			renamed := TaskDefinition{
				ID: in.NewDefinitionID, ChainID: in.ChainID, Sequence: maxSequence + 1,
				Title: title, DescriptionText: clonePtr(d.DescriptionText),
				CreatedAt: in.Now, ContentUpdatedAt: in.Now,
			}
			if d.PlannedSubtasks != nil {
				renamed.PlannedSubtasks = append([]PlannedSubtask{}, d.PlannedSubtasks...)
			}
			d.ContentUpdatedAt = in.Now
			d.SupersededAt = clonePtr(&in.Now)
			d.SupersededByDefinitionID = clonePtr(&in.NewDefinitionID)
			g.Definitions = append(g.Definitions, renamed)
			for _, i := range traces {
				tr := &g.Traces[i]
				if tr.Status == TracePending {
					tr.DefinitionID = renamed.ID
					tr.ContentUpdatedAt = in.Now
				}
			}
		}
		// NoonmarkEngine.touchChain preserves a newer chain clock (e.g. notes).
		if in.Now.After(chain.UpdatedAt) {
			chain.UpdatedAt = in.Now
		}
		return nil
	})
}
