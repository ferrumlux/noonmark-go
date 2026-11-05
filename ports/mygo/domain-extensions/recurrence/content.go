// SPDX-License-Identifier: AGPL-3.0-only
package recurrence

import (
	"sort"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type ContentInput struct {
	SeriesID        core.TaskCycleSeriesID
	Title           string
	DescriptionText *string
	PlannedSubtasks []core.PlannedSubtask
	Today           core.LocalDate
	Now             core.Instant
}

func (s State) UpdateContent(in ContentInput, ids IdentitySource) (State, error) {
	if e := s.Validate(); e != nil {
		return s, e
	}
	if e := requireInput(in.Today, in.Now); e != nil {
		return s, e
	}
	i := s.seriesIndex(in.SeriesID)
	if i < 0 {
		return s, ErrNotFound
	}
	p := s.Series[i]
	if !p.Lifecycle(in.Today).AllowsPlanEditing() {
		return s, ErrInvalidTransition
	}
	if !clock(p.UpdatedAt, in.Now) {
		return s, core.ErrInvalidClock
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return s, core.ErrInvalidTitle
	}
	planned := append([]core.PlannedSubtask(nil), in.PlannedSubtasks...)
	sort.SliceStable(planned, func(i, j int) bool { return planned[i].Position < planned[j].Position })
	for i := range planned {
		planned[i].Title = strings.TrimSpace(planned[i].Title)
		planned[i].Position = i + 1
		if planned[i].Title == "" {
			return s, core.ErrInvalidTitle
		}
	}
	candidate := s.Clone()
	for _, chain := range s.EditableMemberChainIDs(p.ID, in.Today) {
		if e := candidate.replaceMemberContent(chain, title, in.DescriptionText, planned, in.Today, in.Now, ids); e != nil {
			return s, e
		}
	}
	target := &candidate.Series[i]
	target.Title = title
	target.DescriptionText = cp(in.DescriptionText)
	target.PlannedSubtasks = planned
	target.UpdatedAt = in.Now
	return finish(s, candidate, nil)
}
func (s *State) replaceMemberContent(chain core.TaskChainID, title string, description *string, planned []core.PlannedSubtask, today core.LocalDate, now core.Instant, ids IdentitySource) error {
	var pending []int
	for i, t := range s.Tasks.Traces {
		if t.ChainID == chain && t.Status == core.TracePending && t.Date.Compare(today) >= 0 {
			if !s.unlocked(t.Date) {
				return core.ErrImmutableHistory
			}
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return ErrInvalidTransition
	}
	current := -1
	maxSequence := 0
	for i, d := range s.Tasks.Definitions {
		if d.ChainID == chain {
			if d.SupersededAt == nil {
				current = i
			}
			if d.Sequence > maxSequence {
				maxSequence = d.Sequence
			}
		}
	}
	if current < 0 {
		return ErrInvalidSeries
	}
	definition := s.Tasks.Definitions[current]
	if !clock(definition.ContentUpdatedAt, now) {
		return core.ErrInvalidClock
	}
	changed := !plannedEqual(definition.PlannedSubtasks, planned)
	historical := false
	for _, t := range s.Tasks.Traces {
		if t.ChainID == chain && t.Status.FormsDayHistory() && t.DefinitionID == definition.ID && t.Status != core.TracePending {
			historical = true
		}
	}
	cloned, e := cloneOccurrencePlanned(planned, now, ids)
	if e != nil {
		return e
	}
	if !historical {
		definition.Title = title
		definition.DescriptionText = cp(description)
		definition.PlannedSubtasks = cloned
		definition.ContentUpdatedAt = now
		s.Tasks.Definitions[current] = definition
	} else {
		if maxSequence == int(^uint(0)>>1) {
			return core.ErrInvalidInput
		}
		id, e := nextID(ids)
		if e != nil {
			return e
		}
		replacement := core.TaskDefinition{ID: core.TaskDefinitionID(id), ChainID: chain, Sequence: maxSequence + 1, Title: title, DescriptionText: cp(description), PlannedSubtasks: cloned, CreatedAt: now, ContentUpdatedAt: now}
		definition.ContentUpdatedAt = now
		definition.SupersededAt = ptr(now)
		definition.SupersededByDefinitionID = ptr(replacement.ID)
		s.Tasks.Definitions[current] = definition
		s.Tasks.Definitions = append(s.Tasks.Definitions, replacement)
		for _, i := range pending {
			t := &s.Tasks.Traces[i]
			if !clock(t.ContentUpdatedAt, now) {
				return core.ErrInvalidClock
			}
			t.DefinitionID = replacement.ID
			t.DescriptionText = cp(description)
			t.ContentUpdatedAt = now
		}
	}
	for _, i := range pending {
		t := &s.Tasks.Traces[i]
		if !stringPtrEqual(t.DescriptionText, description) {
			if !clock(t.ContentUpdatedAt, now) {
				return core.ErrInvalidClock
			}
			t.DescriptionText = cp(description)
			t.ContentUpdatedAt = now
		}
		if changed {
			if e := s.replaceTraceSubtasks(t.ID, planned, now, ids); e != nil {
				return e
			}
		}
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
func (s *State) replaceTraceSubtasks(trace core.DayTraceID, planned []core.PlannedSubtask, now core.Instant, ids IdentitySource) error {
	desired := map[core.SubtaskLineageID]bool{}
	for _, p := range planned {
		desired[p.LineageID] = true
	}
	existing := map[core.SubtaskLineageID]int{}
	nextPosition := 1
	for i := range s.Tasks.Subtasks {
		v := &s.Tasks.Subtasks[i]
		if v.TraceID != trace {
			continue
		}
		existing[v.LineageID] = i
		if v.Position >= nextPosition {
			if v.Position == int(^uint(0)>>1) {
				return core.ErrInvalidInput
			}
			nextPosition = v.Position + 1
		}
		if v.Status == core.SubtaskPending && !desired[v.LineageID] {
			if !clock(v.UpdatedAt, now) {
				return core.ErrInvalidClock
			}
			id, e := nextID(ids)
			if e != nil {
				return e
			}
			v.Status = core.SubtaskCancelledDraft
			v.CompletedAt = nil
			v.SettledAt = ptr(now)
			v.DraftCancellationID = ptr(core.CancellationID(id))
			v.UpdatedAt = now
		}
	}
	for _, p := range planned {
		if i, ok := existing[p.LineageID]; ok {
			v := &s.Tasks.Subtasks[i]
			if v.Status != core.SubtaskPending && v.Status != core.SubtaskCancelledDraft {
				return ErrInvalidTransition
			}
			if !clock(v.UpdatedAt, now) {
				return core.ErrInvalidClock
			}
			v.Title = p.Title
			v.Difficulty = p.Difficulty
			v.Status = core.SubtaskPending
			v.CompletedAt = nil
			v.SettledAt = nil
			v.DraftCancellationID = nil
			v.UpdatedAt = now
			continue
		}
		id, e := nextID(ids)
		if e != nil {
			return e
		}
		s.Tasks.Subtasks = append(s.Tasks.Subtasks, core.Subtask{ID: core.SubtaskID(id), LineageID: p.LineageID, TraceID: trace, Title: p.Title, Status: core.SubtaskPending, Difficulty: p.Difficulty, Position: nextPosition, CreatedAt: now, UpdatedAt: now})
		nextPosition++
	}
	return nil
}
func plannedEqual(a, b []core.PlannedSubtask) bool {
	if len(a) != len(b) {
		return false
	}
	a = append([]core.PlannedSubtask(nil), a...)
	b = append([]core.PlannedSubtask(nil), b...)
	sort.SliceStable(a, func(i, j int) bool { return a[i].Position < a[j].Position })
	sort.SliceStable(b, func(i, j int) bool { return b[i].Position < b[j].Position })
	for i, x := range a {
		y := b[i]
		if x.LineageID != y.LineageID || x.Title != y.Title || x.Difficulty != y.Difficulty || x.Position != y.Position {
			return false
		}
	}
	return true
}
func stringPtrEqual(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

type ConvertInput struct {
	ChainID      core.TaskChainID
	StartDate    core.LocalDate
	Schedule     Schedule
	EndCondition EndCondition
	Today        core.LocalDate
	Now          core.Instant
}

// Convert preserves the existing chain and, when scheduled on the first
// occurrence, the existing trace. Ordinary historical chains cannot be adopted.
func (s State) Convert(in ConvertInput, ids IdentitySource) (State, core.TaskCycleSeriesID, error) {
	if e := s.Validate(); e != nil {
		return s, "", e
	}
	if e := requireInput(in.Today, in.Now); e != nil {
		return s, "", e
	}
	if in.StartDate.Compare(in.Today) < 0 {
		return s, "", ErrInvalidTransition
	}
	chainIndex := -1
	for i, c := range s.Tasks.Chains {
		if c.ID == in.ChainID {
			chainIndex = i
			break
		}
	}
	if chainIndex < 0 {
		return s, "", ErrNotFound
	}
	chain := s.Tasks.Chains[chainIndex]
	if chain.State != core.ChainActive || chain.CycleMembership != nil {
		return s, "", ErrInvalidTransition
	}
	if !clock(chain.UpdatedAt, in.Now) {
		return s, "", core.ErrInvalidClock
	}
	var definition *core.TaskDefinition
	for _, d := range s.Tasks.Definitions {
		if d.ChainID == chain.ID && d.SupersededAt == nil {
			definition = ptr(d)
			break
		}
	}
	if definition == nil {
		return s, "", ErrInvalidSeries
	}
	end, e := in.EndCondition.ResolvedEndDate(in.StartDate)
	if e != nil {
		return s, "", e
	}
	dates, e := in.Schedule.OccurrenceDates(in.StartDate, end)
	if e != nil || len(dates) == 0 {
		return s, "", ErrInvalidSchedule
	}
	active := s.pendingTrace(chain.ID)
	if active != nil {
		if active.Date != dates[0] {
			return s, "", ErrInvalidTransition
		}
	} else {
		var latest *core.DayTrace
		for _, t := range s.Tasks.Traces {
			if t.ChainID != chain.ID {
				continue
			}
			if latest == nil || taskFactLater(t, *latest) {
				latest = ptr(t)
			}
		}
		if latest != nil && latest.Status != core.TraceReturnedToPool && latest.Status != core.TraceCancelledDraft {
			return s, "", ErrInvalidTransition
		}
	}
	for _, d := range dates {
		if !s.unlocked(d) {
			return s, "", core.ErrImmutableHistory
		}
	}
	description := definition.DescriptionText
	if active != nil && active.DescriptionText != nil {
		description = active.DescriptionText
	}
	p, e := newSeries(CreateInput{Title: definition.Title, DescriptionText: description, PlannedSubtasks: definition.PlannedSubtasks, StartDate: in.StartDate, Schedule: in.Schedule, EndCondition: in.EndCondition, Today: in.Today, Now: in.Now}, ids)
	if e != nil {
		return s, "", e
	}
	candidate := s.Clone()
	candidate.Series = append(candidate.Series, p)
	c := &candidate.Tasks.Chains[chainIndex]
	c.CycleMembership = &core.TaskCycleMembership{SeriesID: p.ID, OccurrenceDate: dates[0]}
	c.UpdatedAt = in.Now
	if active == nil {
		id, e := nextID(ids)
		if e != nil {
			return s, "", e
		}
		priority, e := candidate.nextPriority(dates[0])
		if e != nil {
			return s, "", e
		}
		trace := core.DayTrace{ID: core.DayTraceID(id), ChainID: chain.ID, DefinitionID: definition.ID, Date: dates[0], Status: core.TracePending, Priority: priority, DescriptionText: cp(description), CreatedAt: in.Now, ContentUpdatedAt: in.Now}
		candidate.Tasks.Traces = append(candidate.Tasks.Traces, trace)
		if e = candidate.appendSubtasks(definition.PlannedSubtasks, trace.ID, in.Now, ids); e != nil {
			return s, "", e
		}
		if e = candidate.ensureDay(dates[0], in.Now, ids); e != nil {
			return s, "", e
		}
	}
	for _, d := range dates[1:] {
		if _, e = candidate.installOccurrence(p, d, in.Now, ids); e != nil {
			return s, "", e
		}
	}
	candidate, e = finish(s, candidate, nil)
	if e != nil {
		return s, "", e
	}
	return candidate, p.ID, nil
}
func taskFactLater(a, b core.DayTrace) bool {
	if !a.ContentUpdatedAt.Equal(b.ContentUpdatedAt) {
		return a.ContentUpdatedAt.After(b.ContentUpdatedAt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}
