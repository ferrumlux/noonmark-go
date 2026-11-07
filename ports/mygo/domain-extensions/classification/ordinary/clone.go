// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

type coreChain = core.TaskChainID

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func clonePreview(p SelectionPreview) SelectionPreview {
	p.Category = clonePointer(p.Category)
	p.Labels = append([]PlanItem(nil), p.Labels...)
	return p
}
func cloneChanges(cs []PlanChange) []PlanChange {
	out := make([]PlanChange, len(cs))
	for i, c := range cs {
		c.Before = clonePreview(c.Before)
		c.After = clonePreview(c.After)
		out[i] = c
	}
	return out
}
func cloneNotices(ns []Notice) []Notice { return append([]Notice(nil), ns...) }
func cloneReceipt(r Receipt) Receipt    { r.Notices = cloneNotices(r.Notices); return r }
func clonePlan(p PlanData) PlanData {
	p.Changes = cloneChanges(p.Changes)
	p.Notices = cloneNotices(p.Notices)
	p.PlannedLabelIDs = append([]UUID(nil), p.PlannedLabelIDs...)
	p.Blockers = append([]string(nil), p.Blockers...)
	return p
}
func cloneState(s State) State {
	out := State{FormatVersion: s.FormatVersion, Revision: s.Revision, Categories: make(map[CategoryID]Category, len(s.Categories)), CurrentByChainID: make(map[coreChain]Current, len(s.CurrentByChainID)), CommittedReceiptsByInteractionID: make(map[UUID]Receipt, len(s.CommittedReceiptsByInteractionID))}
	for id, c := range s.Categories {
		c.NameVersions = append([]NameVersion(nil), c.NameVersions...)
		for i := range c.NameVersions {
			c.NameVersions[i].ValidUntil = clonePointer(c.NameVersions[i].ValidUntil)
		}
		out.Categories[id] = c
	}
	for id, c := range s.CurrentByChainID {
		c.Category = clonePointer(c.Category)
		out.CurrentByChainID[id] = c
	}
	out.RelationHistory = append([]RelationHistory(nil), s.RelationHistory...)
	out.ChangeRecords = make([]ChangeRecord, len(s.ChangeRecords))
	for i, r := range s.ChangeRecords {
		r.Changes = cloneChanges(r.Changes)
		r.Notices = cloneNotices(r.Notices)
		out.ChangeRecords[i] = r
	}
	for id, r := range s.CommittedReceiptsByInteractionID {
		out.CommittedReceiptsByInteractionID[id] = cloneReceipt(r)
	}
	return out
}
func cloneSteps(steps []CommittedStep) []CommittedStep {
	out := make([]CommittedStep, len(steps))
	for i, s := range steps {
		s.Plan = clonePlan(s.Plan)
		s.Receipt = cloneReceipt(s.Receipt)
		out[i] = s
	}
	return out
}
func (p Plan) Data() (PlanData, error) {
	if p.sealed == nil {
		return PlanData{}, ErrInvalid
	}
	b, e := entry(OrdinaryContext{p.sealed.context}, p.sealed.data)
	if e != nil {
		return PlanData{}, e
	}
	if e = b.scan(p.sealed.data); e != nil {
		return PlanData{}, e
	}
	return clonePlan(p.sealed.data), nil
}
func (p ReplayProof) State() (State, error) {
	if p.sealed == nil {
		return State{}, ErrInvalid
	}
	b, e := entry(OrdinaryContext{p.sealed.context}, p.sealed.state)
	if e != nil {
		return State{}, e
	}
	if e = b.scan(p.sealed.state); e != nil {
		return State{}, e
	}
	return cloneState(p.sealed.state), nil
}
func (p ReplayProof) Transcript() ([]CommittedStep, error) {
	if p.sealed == nil {
		return nil, ErrInvalid
	}
	b, e := entry(OrdinaryContext{p.sealed.context}, p.sealed.steps)
	if e != nil {
		return nil, e
	}
	if e = b.scan(p.sealed.steps); e != nil {
		return nil, e
	}
	return cloneSteps(p.sealed.steps), nil
}
