// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type historyCommandPhase uint8

const (
	historyCommandBefore historyCommandPhase = iota + 1
	historyCommandAfter
	historyCommandIDs
)

type historyCommandBudgetReason uint8

const (
	historyCommandShape historyCommandBudgetReason = iota + 1
	historyCommandRawBytes
	historyCommandIDCount
	historyCommandIDLength
)

// Fixed typed errors intentionally contain no caller strings or graph content.
type historyCommandBudgetError struct {
	Phase  historyCommandPhase
	Reason historyCommandBudgetReason
}

func (e *historyCommandBudgetError) Error() string {
	return "candidate history command: input preflight rejected"
}
func historyCommandBudgetFail(p historyCommandPhase, r historyCommandBudgetReason) error {
	return &historyCommandBudgetError{p, r}
}

type historyCommandBudget struct{ nodes, rawBytes uint64 }

func historyCommandAddProduct(sum, count, cost, limit uint64) (uint64, bool) {
	if cost == 0 || sum > limit || count > (limit-sum)/cost {
		return 0, false
	}
	return sum + count*cost, true
}
func (b *historyCommandBudget) nodesAdd(count, cost uint64) bool {
	n, ok := historyCommandAddProduct(b.nodes, count, cost, uint64(canonical.DefaultLimits().MaxNodes))
	if ok {
		b.nodes = n
	}
	return ok
}
func (b *historyCommandBudget) optional(present bool, cost uint64) bool {
	if !present {
		return true
	}
	return b.nodesAdd(1, cost)
}
func (b *historyCommandBudget) raw(s string) bool {
	n, ok := historyCheckedAdd(b.rawBytes, uint64(len(s)), uint64(canonical.DefaultLimits().MaxBytes))
	if ok {
		b.rawBytes = n
	}
	return ok
}
func historyCommandRawOptional[T ~string](b *historyCommandBudget, s *T) bool {
	return s == nil || b.raw(string(*s))
}

// Fixed typed metadata only: no content scan, validation, clone, AST, index or
// caller-sized allocation. Shared leaves are charged for EVERY logical use.
// All top-level base costs precede any record access. All structure costs
// precede the raw-string pass. Nested base costs precede their element access.
func historyCommandGraphBudget(g core.TaskGraph, phase historyCommandPhase) (historyCommandBudget, error) {
	b := historyCommandBudget{nodes: 5}
	if !b.nodesAdd(uint64(len(g.Days)), 17) || !b.nodesAdd(uint64(len(g.Chains)), 13) ||
		!b.nodesAdd(uint64(len(g.Definitions)), 19) || !b.nodesAdd(uint64(len(g.Traces)), 33) || !b.nodesAdd(uint64(len(g.Subtasks)), 25) {
		return b, historyCommandBudgetFail(phase, historyCommandShape)
	}
	for i := range g.Days {
		d := &g.Days[i]
		if !b.optional(d.LockedAt != nil, 2) || !b.optional(d.ReviewSummary != nil, 2) || !b.optional(d.ReviewUnfinishedReason != nil, 2) || !b.optional(d.ReviewTomorrowNote != nil, 2) {
			return b, historyCommandBudgetFail(phase, historyCommandShape)
		}
	}
	for i := range g.Chains {
		c := &g.Chains[i]
		if !b.optional(c.CycleMembership != nil, 14) || !b.nodesAdd(uint64(len(c.NoteEntries)), 11) {
			return b, historyCommandBudgetFail(phase, historyCommandShape)
		}
		for j := range c.NoteEntries {
			if !b.optional(c.NoteEntries[j].DeletedAt != nil, 2) {
				return b, historyCommandBudgetFail(phase, historyCommandShape)
			}
		}
	}
	for i := range g.Definitions {
		d := &g.Definitions[i]
		if !b.optional(d.DescriptionText != nil, 2) || !b.optional(d.SupersededAt != nil, 2) || !b.optional(d.SupersededByDefinitionID != nil, 4) || !b.nodesAdd(uint64(len(d.PlannedSubtasks)), 17) {
			return b, historyCommandBudgetFail(phase, historyCommandShape)
		}
	}
	for i := range g.Traces {
		t := &g.Traces[i]
		if !b.optional(t.PinOrder != nil, 2) || !b.optional(t.DescriptionText != nil, 2) || !b.optional(t.ManualProgressPercent != nil, 2) ||
			!b.optional(t.CompletedAt != nil, 2) || !b.optional(t.SettledAt != nil, 2) || !b.optional(t.DraftCancellationID != nil, 2) ||
			!b.optional(t.CarriedFromTraceID != nil, 4) || !b.optional(t.ChangedToTraceID != nil, 4) || !b.optional(t.DraftCancelledOn != nil, 8) ||
			!b.nodesAdd(uint64(len(t.NoteEntries)), 11) {
			return b, historyCommandBudgetFail(phase, historyCommandShape)
		}
		for j := range t.NoteEntries {
			if !b.optional(t.NoteEntries[j].DeletedAt != nil, 2) {
				return b, historyCommandBudgetFail(phase, historyCommandShape)
			}
		}
	}
	for i := range g.Subtasks {
		s := &g.Subtasks[i]
		if !b.optional(s.CarriedFromSubtaskID != nil, 4) || !b.optional(s.CompletedAt != nil, 2) || !b.optional(s.SettledAt != nil, 2) || !b.optional(s.DraftCancellationID != nil, 2) {
			return b, historyCommandBudgetFail(phase, historyCommandShape)
		}
	}
	// This pass reads only string headers after every record count is bounded.
	for i := range g.Days {
		d := &g.Days[i]
		if !b.raw(string(d.ID)) || !historyCommandRawOptional(&b, d.ReviewSummary) || !historyCommandRawOptional(&b, d.ReviewUnfinishedReason) || !historyCommandRawOptional(&b, d.ReviewTomorrowNote) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
	}
	for i := range g.Chains {
		c := &g.Chains[i]
		if !b.raw(string(c.ID)) || !b.raw(string(c.State)) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
		if c.CycleMembership != nil && !b.raw(string(c.CycleMembership.SeriesID)) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
		for j := range c.NoteEntries {
			n := &c.NoteEntries[j]
			if !b.raw(string(n.ID)) || !b.raw(n.Body) {
				return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
			}
		}
	}
	for i := range g.Definitions {
		d := &g.Definitions[i]
		if !b.raw(string(d.ID)) || !b.raw(string(d.ChainID)) || !b.raw(d.Title) || !historyCommandRawOptional(&b, d.DescriptionText) || !historyCommandRawOptional(&b, d.SupersededByDefinitionID) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
		for j := range d.PlannedSubtasks {
			s := &d.PlannedSubtasks[j]
			if !b.raw(string(s.ID)) || !b.raw(string(s.LineageID)) || !b.raw(s.Title) {
				return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
			}
		}
	}
	for i := range g.Traces {
		t := &g.Traces[i]
		if !b.raw(string(t.ID)) || !b.raw(string(t.ChainID)) || !b.raw(string(t.DefinitionID)) || !b.raw(string(t.Status)) ||
			!historyCommandRawOptional(&b, t.DescriptionText) || !historyCommandRawOptional(&b, t.CarriedFromTraceID) || !historyCommandRawOptional(&b, t.ChangedToTraceID) || !historyCommandRawOptional(&b, t.DraftCancellationID) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
		for j := range t.NoteEntries {
			n := &t.NoteEntries[j]
			if !b.raw(string(n.ID)) || !b.raw(n.Body) {
				return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
			}
		}
	}
	for i := range g.Subtasks {
		s := &g.Subtasks[i]
		if !b.raw(string(s.ID)) || !b.raw(string(s.LineageID)) || !b.raw(string(s.TraceID)) || !b.raw(s.Title) || !b.raw(string(s.Status)) ||
			!historyCommandRawOptional(&b, s.CarriedFromSubtaskID) || !historyCommandRawOptional(&b, s.DraftCancellationID) {
			return b, historyCommandBudgetFail(phase, historyCommandRawBytes)
		}
	}
	return b, nil
}

// prior has already passed the fixed-profile seal check. Reading this private
// array's length does not clone/decode or traverse caller input. One settlement
// can capture each prior trace at most once; final exact count is checked later.
func historyCommandEventIDBudget(prior validatedHistorySnapshot, ids []string) error {
	limits := canonical.DefaultLimits()
	countLimit := uint64(len(prior.proof.snapshot.Object["traces"].Array))
	if n := uint64(limits.MaxNodes) / 15; n < countLimit {
		countLimit = n
	}
	if n := uint64(limits.MaxBytes) / 36; n < countLimit {
		countLimit = n
	}
	if uint64(len(ids)) > countLimit {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDCount)
	}
	var bytes uint64
	for i := range ids {
		if len(ids[i]) != 36 {
			return historyCommandBudgetFail(historyCommandIDs, historyCommandIDLength)
		}
		var ok bool
		bytes, ok = historyCheckedAdd(bytes, uint64(len(ids[i])), uint64(limits.MaxBytes))
		if !ok {
			return historyCommandBudgetFail(historyCommandIDs, historyCommandRawBytes)
		}
	}
	return nil
}
func historyCommandCompletionIDBudget(in completionHistoryInput) error {
	if len(in.ChainID) != 36 || len(in.TraceID) != 36 {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDLength)
	}
	return nil
}
