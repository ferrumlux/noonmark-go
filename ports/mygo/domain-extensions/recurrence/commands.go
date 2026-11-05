// SPDX-License-Identifier: AGPL-3.0-only
package recurrence

import (
	"sort"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// IdentitySource is injected for deterministic replay; nil uses crypto/rand UUIDs.
// Consuming an identity on a failed command is harmless: no fact is committed.
type IdentitySource func() (string, error)

func nextID(ids IdentitySource) (string, error) {
	if ids == nil {
		ids = core.NewIdentity
	}
	id, e := ids()
	if e != nil {
		return "", e
	}
	if !validID(id) {
		return "", ErrInvalidSeries
	}
	return id, nil
}
func requireInput(today core.LocalDate, now core.Instant) error {
	if !today.Valid() {
		return ErrInvalidRange
	}
	if !now.Finite() {
		return core.ErrInvalidClock
	}
	return nil
}
func finish(original, next State, err error) (State, error) {
	if err != nil {
		return original, err
	}
	if e := next.Validate(); e != nil {
		return original, e
	}
	return next, nil
}

type CreateInput struct {
	Title           string
	DescriptionText *string
	PlannedSubtasks []core.PlannedSubtask
	StartDate       core.LocalDate
	Schedule        Schedule
	EndCondition    EndCondition
	Today           core.LocalDate
	Now             core.Instant
}

func (s State) Create(in CreateInput, ids IdentitySource) (State, core.TaskCycleSeriesID, error) {
	if e := s.Validate(); e != nil {
		return s, "", e
	}
	if e := requireInput(in.Today, in.Now); e != nil {
		return s, "", e
	}
	if in.StartDate.Compare(in.Today) < 0 {
		return s, "", ErrInvalidTransition
	}
	series, e := newSeries(in, ids)
	if e != nil {
		return s, "", e
	}
	dates, e := series.Schedule.OccurrenceDates(series.StartDate, series.EndDate)
	if e != nil || len(dates) == 0 {
		return s, "", ErrInvalidSchedule
	}
	candidate := s.Clone()
	candidate.Series = append(candidate.Series, series)
	for _, date := range dates {
		if _, e = candidate.installOccurrence(series, date, in.Now, ids); e != nil {
			return s, "", e
		}
	}
	candidate, e = finish(s, candidate, nil)
	if e != nil {
		return s, "", e
	}
	return candidate, series.ID, nil
}
func newSeries(in CreateInput, ids IdentitySource) (Series, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Series{}, core.ErrInvalidTitle
	}
	end, e := in.EndCondition.ResolvedEndDate(in.StartDate)
	if e != nil {
		return Series{}, e
	}
	id, e := nextID(ids)
	if e != nil {
		return Series{}, e
	}
	revisionID, e := nextID(ids)
	if e != nil {
		return Series{}, e
	}
	classID, e := nextID(ids)
	if e != nil {
		return Series{}, e
	}
	interaction, e := nextID(ids)
	if e != nil {
		return Series{}, e
	}
	planned := append([]core.PlannedSubtask(nil), in.PlannedSubtasks...)
	sort.SliceStable(planned, func(i, j int) bool { return planned[i].Position < planned[j].Position })
	p := Series{ID: core.TaskCycleSeriesID(id), Title: title, DescriptionText: cp(in.DescriptionText), PlannedSubtasks: planned, StartDate: in.StartDate, EndDate: end, Schedule: cloneSchedule(in.Schedule), EndCondition: in.EndCondition, CreatedAt: in.Now, UpdatedAt: in.Now,
		PlanRevisions:           []PlanRevision{{ID: revisionID, EffectiveFrom: in.StartDate, EndConditionAnchorDate: in.StartDate, Schedule: cloneSchedule(in.Schedule), EndCondition: in.EndCondition, RecordedAt: in.Now}},
		ClassificationRevisions: []ClassificationRevision{{ID: classID, InteractionID: interaction, Source: classification.Source{Kind: classification.DeterministicDomainAction, Reason: "task cycle created"}, RecordedAt: in.Now}}}
	return p, p.Validate()
}
func (s *State) installOccurrence(p Series, date core.LocalDate, now core.Instant, ids IdentitySource) (core.TaskChainID, error) {
	chainID, e := nextID(ids)
	if e != nil {
		return "", e
	}
	definitionID, e := nextID(ids)
	if e != nil {
		return "", e
	}
	traceID, e := nextID(ids)
	if e != nil {
		return "", e
	}
	priority, e := s.nextPriority(date)
	if e != nil {
		return "", e
	}
	chain := core.TaskChain{ID: core.TaskChainID(chainID), State: core.ChainActive, CycleMembership: &core.TaskCycleMembership{SeriesID: p.ID, OccurrenceDate: date}, CreatedAt: now, UpdatedAt: now}
	planned, e := cloneOccurrencePlanned(p.PlannedSubtasks, now, ids)
	if e != nil {
		return "", e
	}
	definition := core.TaskDefinition{ID: core.TaskDefinitionID(definitionID), ChainID: chain.ID, Sequence: 1, Title: p.Title, DescriptionText: cp(p.DescriptionText), PlannedSubtasks: planned, CreatedAt: now, ContentUpdatedAt: now}
	trace := core.DayTrace{ID: core.DayTraceID(traceID), ChainID: chain.ID, DefinitionID: definition.ID, Date: date, Status: core.TracePending, Priority: priority, DescriptionText: cp(p.DescriptionText), CreatedAt: now, ContentUpdatedAt: now}
	if e = s.appendSubtasks(definition.PlannedSubtasks, trace.ID, now, ids); e != nil {
		return "", e
	}
	if e = s.ensureDay(date, now, ids); e != nil {
		return "", e
	}
	s.Tasks.Chains = append(s.Tasks.Chains, chain)
	s.Tasks.Definitions = append(s.Tasks.Definitions, definition)
	s.Tasks.Traces = append(s.Tasks.Traces, trace)
	return chain.ID, nil
}
func cloneOccurrencePlanned(planned []core.PlannedSubtask, now core.Instant, ids IdentitySource) ([]core.PlannedSubtask, error) {
	out := append([]core.PlannedSubtask(nil), planned...)
	for i := range out {
		id, e := nextID(ids)
		if e != nil {
			return nil, e
		}
		out[i].ID = core.PlannedSubtaskID(id)
		out[i].CreatedAt = now
	}
	return out, nil
}
func (s *State) appendSubtasks(planned []core.PlannedSubtask, trace core.DayTraceID, now core.Instant, ids IdentitySource) error {
	sorted := append([]core.PlannedSubtask(nil), planned...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	for i, p := range sorted {
		id, e := nextID(ids)
		if e != nil {
			return e
		}
		s.Tasks.Subtasks = append(s.Tasks.Subtasks, core.Subtask{ID: core.SubtaskID(id), LineageID: p.LineageID, TraceID: trace, Title: p.Title, Status: core.SubtaskPending, Difficulty: p.Difficulty, Position: i + 1, CreatedAt: now, UpdatedAt: now})
	}
	return nil
}
func (s *State) ensureDay(date core.LocalDate, now core.Instant, ids IdentitySource) error {
	for _, d := range s.Tasks.Days {
		if d.Date == date {
			return nil
		}
	}
	id, e := nextID(ids)
	if e != nil {
		return e
	}
	s.Tasks.Days = append(s.Tasks.Days, core.Day{ID: core.DayID(id), Date: date, CreatedAt: now, UpdatedAt: now})
	return nil
}
func (s State) unlocked(date core.LocalDate) bool {
	for _, d := range s.Tasks.Days {
		if d.Date == date && d.LockedAt != nil {
			return false
		}
	}
	return true
}
func (s State) nextPriority(date core.LocalDate) (int, error) {
	var max int
	found := false
	for _, t := range s.Tasks.Traces {
		if t.Date == date && t.Status.FormsDayHistory() && (!found || t.Priority > max) {
			max = t.Priority
			found = true
		}
	}
	if max == int(^uint(0)>>1) {
		return 0, core.ErrInvalidInput
	}
	return max + 1, nil
}
func (s State) pendingTrace(chain core.TaskChainID) *core.DayTrace {
	for _, t := range s.Tasks.Traces {
		if t.ChainID == chain && t.Status == core.TracePending {
			return ptr(t)
		}
	}
	return nil
}
func (s *State) appendCancellation(seriesIndex int, scope CancellationScope, date core.LocalDate, reason CancellationReason, now core.Instant, ids IdentitySource) error {
	p := &s.Series[seriesIndex]
	if !clock(p.UpdatedAt, now) {
		return core.ErrInvalidClock
	}
	id, e := nextID(ids)
	if e != nil {
		return e
	}
	p.UpdatedAt = now
	p.CancellationFacts = append(p.CancellationFacts, CancellationFact{ID: id, Scope: scope, Date: date, Reason: reason, RecordedAt: now})
	sort.Slice(p.CancellationFacts, func(i, j int) bool { return factBefore(p.CancellationFacts[i], p.CancellationFacts[j]) })
	return nil
}
func (s *State) cancelTrace(id core.DayTraceID, today core.LocalDate, now core.Instant, ids IdentitySource) error {
	for i := range s.Tasks.Traces {
		t := &s.Tasks.Traces[i]
		if t.ID != id {
			continue
		}
		if !s.unlocked(t.Date) {
			return core.ErrImmutableHistory
		}
		if !clock(t.ContentUpdatedAt, now) {
			return core.ErrInvalidClock
		}
		cid, e := nextID(ids)
		if e != nil {
			return e
		}
		t.Status = core.TraceCancelledDraft
		t.PinOrder = nil
		t.CompletedAt = nil
		t.SettledAt = ptr(now)
		t.DraftCancellationID = ptr(core.CancellationID(cid))
		t.DraftCancelledOn = ptr(today)
		t.ContentUpdatedAt = now
		for j := range s.Tasks.Chains {
			c := &s.Tasks.Chains[j]
			if c.ID == t.ChainID && now.After(c.UpdatedAt) {
				c.UpdatedAt = now
			}
		}
		return nil
	}
	return ErrNotFound
}
func (s State) Skip(id core.TaskCycleSeriesID, occurrence, today core.LocalDate, now core.Instant, ids IdentitySource) (State, error) {
	if e := s.Validate(); e != nil {
		return s, e
	}
	if e := requireInput(today, now); e != nil {
		return s, e
	}
	i := s.seriesIndex(id)
	if i < 0 {
		return s, ErrNotFound
	}
	c := s.memberChain(id, occurrence)
	if c == nil || occurrence.Compare(today) <= 0 {
		return s, ErrInvalidTransition
	}
	t := s.pendingTrace(c.ID)
	if t == nil || t.Date.Compare(today) <= 0 {
		return s, ErrInvalidTransition
	}
	candidate := s.Clone()
	if e := candidate.appendCancellation(i, Occurrence, occurrence, UserSkipped, now, ids); e != nil {
		return s, e
	}
	e := candidate.cancelTrace(t.ID, today, now, ids)
	return finish(s, candidate, e)
}
func (s State) Stop(id core.TaskCycleSeriesID, today core.LocalDate, now core.Instant, ids IdentitySource) (State, int, error) {
	if e := s.Validate(); e != nil {
		return s, 0, e
	}
	if e := requireInput(today, now); e != nil {
		return s, 0, e
	}
	i := s.seriesIndex(id)
	if i < 0 {
		return s, 0, ErrNotFound
	}
	p := s.Series[i]
	if today.Compare(p.EndDate) >= 0 {
		return s, 0, nil
	}
	if stopped := p.StoppedAfterDate(); stopped != nil && stopped.Compare(today) <= 0 {
		return s, 0, nil
	}
	candidate := s.Clone()
	count := 0
	for _, c := range sortedMembers(s, id) {
		m := c.CycleMembership
		if m.OccurrenceDate.Compare(today) <= 0 {
			continue
		}
		t := s.pendingTrace(c.ID)
		if t == nil || t.Date.Compare(today) <= 0 {
			continue
		}
		if e := candidate.appendCancellation(i, Occurrence, m.OccurrenceDate, StoppedEarly, now, ids); e != nil {
			return s, 0, e
		}
		if e := candidate.cancelTrace(t.ID, today, now, ids); e != nil {
			return s, 0, e
		}
		count++
	}
	if e := candidate.appendCancellation(i, FollowingDates, today, StoppedEarly, now, ids); e != nil {
		return s, 0, e
	}
	candidate, e := finish(s, candidate, nil)
	if e != nil {
		return s, 0, e
	}
	return candidate, count, nil
}
func sortedMembers(s State, id core.TaskCycleSeriesID) []core.TaskChain {
	var out []core.TaskChain
	for _, c := range s.Tasks.Chains {
		if c.CycleMembership != nil && c.CycleMembership.SeriesID == id {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.CycleMembership.OccurrenceDate != b.CycleMembership.OccurrenceDate {
			return a.CycleMembership.OccurrenceDate.Compare(b.CycleMembership.OccurrenceDate) < 0
		}
		return a.ID < b.ID
	})
	return out
}

type Assignment struct {
	ChainID    core.TaskChainID
	CategoryID *classification.CategoryID
	LabelIDs   []classification.LabelID
}
type RevisionOutcome struct {
	EffectiveFrom                                    core.LocalDate
	CancelledOccurrenceCount, CreatedOccurrenceCount int
	ClassificationAssignments                        []Assignment
}
type ReviseInput struct {
	SeriesID     core.TaskCycleSeriesID
	StartDate    *core.LocalDate
	Schedule     Schedule
	EndCondition EndCondition
	Today        core.LocalDate
	Now          core.Instant
}

func (s State) Revise(in ReviseInput, ids IdentitySource) (State, RevisionOutcome, error) {
	zero := RevisionOutcome{}
	if e := s.Validate(); e != nil {
		return s, zero, e
	}
	if e := requireInput(in.Today, in.Now); e != nil {
		return s, zero, e
	}
	i := s.seriesIndex(in.SeriesID)
	if i < 0 {
		return s, zero, ErrNotFound
	}
	p := s.Series[i]
	if p.StoppedAfterDate() != nil || !p.Lifecycle(in.Today).AllowsPlanEditing() {
		return s, zero, ErrInvalidTransition
	}
	tomorrow, e := in.Today.AddDays(1)
	if e != nil {
		return s, zero, e
	}
	revisesStart := in.StartDate != nil && *in.StartDate != p.StartDate
	if revisesStart && (!s.CanReviseStartDate(p.ID, in.Today) || !in.StartDate.Valid() || in.StartDate.Compare(in.Today) < 0) {
		return s, zero, ErrInvalidTransition
	}
	anchor := p.StartDate
	if revisesStart {
		anchor = *in.StartDate
	}
	effective := anchor
	if !revisesStart && tomorrow.Compare(effective) > 0 {
		effective = tomorrow
	}
	editableStart := effective
	if revisesStart {
		editableStart = in.Today
	}
	end, e := in.EndCondition.ResolvedEndDate(anchor)
	if e != nil || end.Compare(effective) < 0 {
		return s, zero, ErrInvalidTransition
	}
	revisedDates, e := in.Schedule.OccurrenceDates(effective, end)
	if e != nil || len(revisedDates) == 0 {
		return s, zero, ErrInvalidSchedule
	}
	if _, e = materializableDates(anchor, end); e != nil {
		return s, zero, e
	}
	if !clock(p.UpdatedAt, in.Now) {
		return s, zero, core.ErrInvalidClock
	}
	members := sortedMembers(s, p.ID)
	if len(members) == 0 {
		return s, zero, ErrInvalidSeries
	}
	byDate := map[core.LocalDate]core.TaskChain{}
	desired := map[core.LocalDate]bool{}
	for _, c := range members {
		byDate[c.CycleMembership.OccurrenceDate] = c
	}
	for _, d := range revisedDates {
		desired[d] = true
	}
	candidate := s.Clone()
	revisionID, e := nextID(ids)
	if e != nil {
		return s, zero, e
	}
	cp := &candidate.Series[i]
	cp.StartDate = anchor
	cp.EndDate = end
	cp.Schedule = cloneSchedule(in.Schedule)
	cp.EndCondition = in.EndCondition
	cp.UpdatedAt = in.Now
	cp.PlanRevisions = append(cp.PlanRevisions, PlanRevision{ID: revisionID, EffectiveFrom: effective, EndConditionAnchorDate: anchor, Schedule: cloneSchedule(in.Schedule), EndCondition: in.EndCondition, RecordedAt: in.Now})
	out := RevisionOutcome{EffectiveFrom: effective}
	for _, c := range members {
		d := c.CycleMembership.OccurrenceDate
		t := s.pendingTrace(c.ID)
		if d.Compare(editableStart) < 0 || desired[d] || t == nil || t.Date != d || t.Date.Compare(editableStart) < 0 {
			continue
		}
		if e := candidate.appendCancellation(i, Occurrence, d, PlanChanged, in.Now, ids); e != nil {
			return s, zero, e
		}
		if e := candidate.cancelTrace(t.ID, in.Today, in.Now, ids); e != nil {
			return s, zero, e
		}
		out.CancelledOccurrenceCount++
	}
	for _, d := range revisedDates {
		c, exists := byDate[d]
		if !exists {
			chain, e := candidate.installOccurrence(candidate.Series[i], d, in.Now, ids)
			if e != nil {
				return s, zero, e
			}
			out.CreatedOccurrenceCount++
			if p.CategoryID != nil || len(p.LabelIDs) > 0 {
				out.ClassificationAssignments = append(out.ClassificationAssignments, Assignment{ChainID: chain, CategoryID: cpCategory(p.CategoryID), LabelIDs: append([]classification.LabelID(nil), p.LabelIDs...)})
			}
			continue
		}
		reason, has := p.latestCancellationReason(d)
		if d.Compare(editableStart) >= 0 && has && reason == PlanChanged && s.pendingTrace(c.ID) == nil {
			if !s.unlocked(d) {
				return s, zero, core.ErrImmutableHistory
			}
			if e := candidate.reactivate(c.ID, candidate.Series[i], d, in.Now, ids); e != nil {
				return s, zero, e
			}
			out.CreatedOccurrenceCount++
		}
	}
	candidate, e = finish(s, candidate, nil)
	if e != nil {
		return s, zero, e
	}
	return candidate, out, nil
}
func cpCategory(c *classification.CategoryID) *classification.CategoryID {
	if c == nil {
		return nil
	}
	return ptr(*c)
}
func (s *State) reactivate(chain core.TaskChainID, p Series, date core.LocalDate, now core.Instant, ids IdentitySource) error {
	current := -1
	maxSequence := 0
	for i, d := range s.Tasks.Definitions {
		if d.ChainID != chain {
			continue
		}
		if d.SupersededAt == nil {
			current = i
		}
		if d.Sequence > maxSequence {
			maxSequence = d.Sequence
		}
	}
	if current < 0 || maxSequence == int(^uint(0)>>1) {
		return ErrInvalidSeries
	}
	old := s.Tasks.Definitions[current]
	if !clock(old.ContentUpdatedAt, now) {
		return core.ErrInvalidClock
	}
	definitionID, e := nextID(ids)
	if e != nil {
		return e
	}
	traceID, e := nextID(ids)
	if e != nil {
		return e
	}
	planned, e := cloneOccurrencePlanned(p.PlannedSubtasks, now, ids)
	if e != nil {
		return e
	}
	priority, e := s.nextPriority(date)
	if e != nil {
		return e
	}
	d := core.TaskDefinition{ID: core.TaskDefinitionID(definitionID), ChainID: chain, Sequence: maxSequence + 1, Title: p.Title, DescriptionText: cp(p.DescriptionText), PlannedSubtasks: planned, CreatedAt: now, ContentUpdatedAt: now}
	old.ContentUpdatedAt = now
	old.SupersededAt = ptr(now)
	old.SupersededByDefinitionID = ptr(d.ID)
	s.Tasks.Definitions[current] = old
	s.Tasks.Definitions = append(s.Tasks.Definitions, d)
	trace := core.DayTrace{ID: core.DayTraceID(traceID), ChainID: chain, DefinitionID: d.ID, Date: date, Status: core.TracePending, Priority: priority, DescriptionText: cp(p.DescriptionText), CreatedAt: now, ContentUpdatedAt: now}
	s.Tasks.Traces = append(s.Tasks.Traces, trace)
	if e = s.appendSubtasks(planned, trace.ID, now, ids); e != nil {
		return e
	}
	if e = s.ensureDay(date, now, ids); e != nil {
		return e
	}
	for i := range s.Tasks.Chains {
		c := &s.Tasks.Chains[i]
		if c.ID == chain {
			if !clock(c.UpdatedAt, now) {
				return core.ErrInvalidClock
			}
			c.UpdatedAt = now
		}
	}
	return nil
}

type ReconciliationOutcome struct{ StoppedSeriesCount, CancelledOccurrenceCount int }

func (s State) Reconcile(today core.LocalDate, now core.Instant, ids IdentitySource) (State, ReconciliationOutcome, error) {
	zero := ReconciliationOutcome{}
	if e := s.Validate(); e != nil {
		return s, zero, e
	}
	if e := requireInput(today, now); e != nil {
		return s, zero, e
	}
	candidate := s.Clone()
	out := zero
	order := make([]int, len(s.Series))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return s.Series[order[i]].ID < s.Series[order[j]].ID })
	for _, i := range order {
		p := s.Series[i]
		if p.StoppedAfterDate() != nil || p.EndCondition.Kind != CompletionTarget {
			continue
		}
		members := map[core.TaskChainID]bool{}
		for _, c := range sortedMembers(s, p.ID) {
			members[c.ID] = true
		}
		completed := map[core.TaskChainID]bool{}
		var cutoff *core.LocalDate
		for _, t := range s.Tasks.Traces {
			if members[t.ChainID] && t.Status == core.TraceCompleted && t.Date.Compare(today) < 0 && !s.unlocked(t.Date) {
				completed[t.ChainID] = true
				if cutoff == nil || t.Date.Compare(*cutoff) > 0 {
					cutoff = ptr(t.Date)
				}
			}
		}
		if len(completed) < p.EndCondition.Count || cutoff == nil || cutoff.Compare(p.EndDate) >= 0 {
			continue
		}
		for _, c := range sortedMembers(s, p.ID) {
			if c.CycleMembership.OccurrenceDate.Compare(*cutoff) <= 0 {
				continue
			}
			t := s.pendingTrace(c.ID)
			if t == nil || t.Date.Compare(today) < 0 {
				continue
			}
			if e := candidate.appendCancellation(i, Occurrence, c.CycleMembership.OccurrenceDate, CompletionTargetReached, now, ids); e != nil {
				return s, zero, e
			}
			if e := candidate.cancelTrace(t.ID, today, now, ids); e != nil {
				return s, zero, e
			}
			out.CancelledOccurrenceCount++
		}
		if e := candidate.appendCancellation(i, FollowingDates, *cutoff, CompletionTargetReached, now, ids); e != nil {
			return s, zero, e
		}
		out.StoppedSeriesCount++
	}
	candidate, e := finish(s, candidate, nil)
	if e != nil {
		return s, zero, e
	}
	return candidate, out, nil
}

type ClassificationInput struct {
	SeriesID      core.TaskCycleSeriesID
	CategoryID    *classification.CategoryID
	LabelIDs      []classification.LabelID
	Source        classification.Source
	InteractionID string
	DecisionID    *string
	Now           core.Instant
}

// SetTemplateClassification mutates parent revision facts only, matching the
// upstream core seam. Caller must validate catalog refs and compose propagation
// to EditableMemberChainIDs through classification authority in one transaction.
func (s State) SetTemplateClassification(in ClassificationInput, ids IdentitySource) (State, error) {
	if e := s.Validate(); e != nil {
		return s, e
	}
	if !in.Now.Finite() {
		return s, core.ErrInvalidClock
	}
	if in.Source.Kind != classification.UserDirect && in.Source.Kind != classification.DeterministicDomainAction {
		return s, ErrUnsupportedAuthority
	}
	if e := in.Source.Validate(); e != nil {
		return s, e
	}
	i := s.seriesIndex(in.SeriesID)
	if i < 0 {
		return s, ErrNotFound
	}
	p := s.Series[i]
	if !clock(p.UpdatedAt, in.Now) {
		return s, core.ErrInvalidClock
	}
	if !validID(in.InteractionID) || !validLabels(in.LabelIDs) || (in.CategoryID != nil && !validID(string(*in.CategoryID))) {
		return s, ErrInvalidSeries
	}
	for _, r := range p.ClassificationRevisions {
		if r.InteractionID == in.InteractionID {
			return s, ErrInvalidTransition
		}
	}
	revisionID, e := nextID(ids)
	if e != nil {
		return s, e
	}
	current := p.CurrentClassificationRevision()
	if current == nil {
		return s, ErrInvalidSeries
	}
	candidate := s.Clone()
	target := &candidate.Series[i]
	target.CategoryID = cp(in.CategoryID)
	target.LabelIDs = append([]classification.LabelID(nil), in.LabelIDs...)
	target.UpdatedAt = in.Now
	target.ClassificationRevisions = append(target.ClassificationRevisions, ClassificationRevision{ID: revisionID, PreviousRevisionID: ptr(current.ID), InteractionID: in.InteractionID, Source: in.Source, DecisionID: cp(in.DecisionID), CategoryID: cp(in.CategoryID), LabelIDs: append([]classification.LabelID(nil), in.LabelIDs...), RecordedAt: in.Now})
	sort.Slice(target.ClassificationRevisions, func(i, j int) bool {
		return revisionBefore(target.ClassificationRevisions[i], target.ClassificationRevisions[j])
	})
	return finish(s, candidate, nil)
}
