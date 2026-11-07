// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// serviceLineage is stable across task edits and confirmed disk revisions.
// A fresh Open creates a new owner. A digest alone never creates authority.
type serviceLineage struct{ epoch byte }
type CategoryPlan struct{ sealed *categoryPrepared }
type categoryPrepared struct {
	owner   *serviceLineage
	request ordinary.Request
	ids     ordinary.PrepareIDs
	data    ordinary.PlanData
}
type CategoryConfirmation struct{ sealed *categoryConfirmed }
type categoryConfirmed struct {
	plan     *categoryPrepared
	decision ordinary.UUID
}

type CategoryOutcome struct {
	Outcome
	Preparation   Outcome
	InteractionID ordinary.UUID
	PlanID        ordinary.UUID
	Receipt       *ordinary.Receipt
	View          OrdinaryReadView
}

func cloneCategoryPlanData(p ordinary.PlanData) ordinary.PlanData {
	p.Changes = append([]ordinary.PlanChange(nil), p.Changes...)
	for i := range p.Changes {
		for _, v := range []*ordinary.SelectionPreview{&p.Changes[i].Before, &p.Changes[i].After} {
			if v.Category != nil {
				c := *v.Category
				v.Category = &c
			}
			v.Labels = append([]ordinary.PlanItem(nil), v.Labels...)
		}
	}
	p.Notices = append([]ordinary.Notice(nil), p.Notices...)
	p.Blockers = append([]string(nil), p.Blockers...)
	p.PlannedLabelIDs = append([]ordinary.UUID(nil), p.PlannedLabelIDs...)
	return p
}
func (p CategoryPlan) Inspect() (ordinary.PlanData, error) {
	if p.sealed == nil {
		return ordinary.PlanData{}, ordinary.ErrInvalid
	}
	return cloneCategoryPlanData(p.sealed.data), nil
}
func ConfirmCategoryPlan(p CategoryPlan) (CategoryConfirmation, error) {
	if p.sealed == nil {
		return CategoryConfirmation{}, ordinary.ErrInvalid
	}
	if err := ordinary.ValidateAggregatePlan(p.sealed.data, ordinary.DefaultLimits()); err != nil {
		return CategoryConfirmation{}, err
	}
	ids, err := newIDs(1)
	if err != nil {
		return CategoryConfirmation{}, err
	}
	return CategoryConfirmation{&categoryConfirmed{p.sealed, ordinary.UUID(ids[0])}}, nil
}

func (s *Service) PrepareCategorySelection(chain core.TaskChainID, choice ordinary.Choice, interaction ordinary.UUID) (CategoryPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcileDayLocked(s.today); err != nil {
		return CategoryPlan{}, err
	}
	return s.prepareCategoryLocked(ordinary.Request{ChainID: chain, Choice: choice, InteractionID: interaction})
}
func (s *Service) prepareCategoryLocked(r ordinary.Request) (CategoryPlan, error) {
	if err := s.gateLocked(); err != nil {
		return CategoryPlan{}, err
	}
	if _, err := s.verifyPrefixLocked(); err != nil {
		return CategoryPlan{}, err
	}
	if s.lineage == nil || s.aggregate == nil {
		return CategoryPlan{}, ordinary.ErrContext
	}
	g, state, err := s.aggregate.document.Facts()
	if err != nil {
		return CategoryPlan{}, err
	}
	ids, err := newIDs(2)
	if err != nil {
		return CategoryPlan{}, err
	}
	slots := ordinary.PrepareIDs{PlanID: ordinary.UUID(ids[0]), CategoryID: ordinary.CategoryID(ids[1])}
	data, err := ordinary.PrepareAggregatePlan(state, g, r, slots, ordinary.DefaultLimits())
	if err != nil {
		return CategoryPlan{}, err
	}
	return CategoryPlan{&categoryPrepared{s.lineage, r, slots, data}}, nil
}
func (s *Service) ReplaceCategorySelection(chain core.TaskChainID, choice ordinary.Choice, interaction ordinary.UUID) CategoryOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceCategoryOnLocked(s.today, chain, choice, interaction)
}

// today is the actual natural day, never the selected historical detail date.
func (s *Service) ReplaceCategorySelectionOn(today core.LocalDate, chain core.TaskChainID, choice ordinary.Choice, interaction ordinary.UUID) CategoryOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceCategoryOnLocked(today, chain, choice, interaction)
}
func (s *Service) replaceCategoryOnLocked(today core.LocalDate, chain core.TaskChainID, choice ordinary.Choice, interaction ordinary.UUID) CategoryOutcome {
	prep := newAttempt()
	err := s.prepareCategoryDayLocked(today, &prep)
	preparation := s.outcomeLocked(prep, err)
	if err != nil {
		return s.categoryOutcomeLocked(prep, err, preparation, CategoryPlan{}, nil)
	}
	p, err := s.prepareCategoryLocked(ordinary.Request{ChainID: chain, Choice: choice, InteractionID: interaction})
	if err != nil {
		return s.categoryOutcomeLocked(newAttempt(), err, preparation, p, nil)
	}
	c, err := ConfirmCategoryPlan(p)
	if err != nil {
		return s.categoryOutcomeLocked(prep, err, preparation, p, nil)
	}
	return s.commitCategoryLocked(p, c, preparation)
}
func (s *Service) CommitCategorySelection(p CategoryPlan, c CategoryConfirmation) CategoryOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitCategoryOnLocked(s.today, p, c)
}
func (s *Service) CommitCategorySelectionOn(today core.LocalDate, p CategoryPlan, c CategoryConfirmation) CategoryOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitCategoryOnLocked(today, p, c)
}
func (s *Service) commitCategoryOnLocked(today core.LocalDate, p CategoryPlan, c CategoryConfirmation) CategoryOutcome {
	prep := newAttempt()
	err := s.prepareCategoryDayLocked(today, &prep)
	preparation := s.outcomeLocked(prep, err)
	if err != nil {
		return s.categoryOutcomeLocked(newAttempt(), err, preparation, p, nil)
	}
	return s.commitCategoryLocked(p, c, preparation)
}
func (s *Service) prepareCategoryDayLocked(today core.LocalDate, a *attempt) error {
	if !today.Valid() {
		a.stage = StageParameter
		return core.ErrInvalidInput
	}
	return s.reconcileDayLocked(today, a)
}
func (s *Service) commitCategoryLocked(p CategoryPlan, c CategoryConfirmation, preparation Outcome) CategoryOutcome {
	a := newAttempt()
	fail := func(err error) CategoryOutcome { return s.categoryOutcomeLocked(a, err, preparation, p, nil) }
	if err := s.gateLocked(); err != nil {
		return fail(err)
	}
	if _, err := s.verifyPrefixLocked(&a); err != nil {
		return fail(err)
	}
	a.stage = StageDomain
	if s.aggregate == nil || s.lineage == nil || p.sealed == nil || p.sealed.owner != s.lineage {
		return fail(ordinary.ErrContext)
	}
	if c.sealed == nil || c.sealed.plan != p.sealed {
		return fail(ordinary.ErrConfirmation)
	}
	g, state, err := s.aggregate.document.Facts()
	if err != nil {
		return fail(err)
	}
	// Validate and retrieve exact retry before sampling a mutation clock or IDs.
	if _, ok := state.CommittedReceiptsByInteractionID[p.sealed.data.InteractionID]; ok {
		_, receipt, err := ordinary.ApplyAggregatePlan(state, g, p.sealed.data, c.sealed.decision, 0, ordinary.CommitIDs{}, ordinary.DefaultLimits())
		if err != nil {
			return fail(err)
		}
		return s.categoryOutcomeLocked(a, nil, preparation, p, &receipt)
	}
	if p.sealed.data.BaseRevision != state.Revision {
		return fail(ordinary.ErrStale)
	}
	now, err := s.nextInstant()
	if err != nil {
		return fail(err)
	}
	ids, err := newIDs(3)
	if err != nil {
		return fail(err)
	}
	next, receipt, err := ordinary.ApplyAggregatePlan(state, g, p.sealed.data, c.sealed.decision, now, ordinary.CommitIDs{RecordID: ordinary.UUID(ids[0]), NameVersionID: ordinary.UUID(ids[1]), RemovalID: ordinary.UUID(ids[2])}, ordinary.DefaultLimits())
	if err != nil {
		return fail(err)
	}
	candidate, err := s.categoryCandidateLocked(p, c, next, now, ordinary.CommitIDs{RecordID: ordinary.UUID(ids[0]), NameVersionID: ordinary.UUID(ids[1]), RemovalID: ordinary.UUID(ids[2])})
	if err != nil {
		return fail(err)
	}
	if err = s.persistOrdinaryLocked(candidate, &a); err != nil {
		return fail(err)
	}
	return s.categoryOutcomeLocked(a, nil, preparation, p, &receipt)
}
func (s *Service) categoryOutcomeLocked(a attempt, err error, preparation Outcome, p CategoryPlan, r *ordinary.Receipt) CategoryOutcome {
	out := CategoryOutcome{Outcome: s.outcomeLocked(a, err), Preparation: preparation, Receipt: r}
	if p.sealed != nil {
		out.InteractionID = p.sealed.data.InteractionID
		out.PlanID = p.sealed.data.ID
	}
	out.View, _ = s.readOrdinaryViewLocked()
	return out
}
