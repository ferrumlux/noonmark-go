// SPDX-License-Identifier: AGPL-3.0-only
// Package recurrence ports Noonmark's bounded recurring-plan facts and commands.
package recurrence

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

const MaximumMaterializedDays = 366

var (
	ErrInvalidSeries        = errors.New("invalid recurring series facts")
	ErrInvalidSchedule      = errors.New("invalid recurring schedule")
	ErrInvalidRange         = errors.New("recurring date range must contain 1 to 366 days")
	ErrInvalidTransition    = errors.New("recurring operation is not permitted")
	ErrNotFound             = errors.New("recurring series or occurrence not found")
	ErrUnsupportedAuthority = errors.New("classification authority is not available at this seam")
)

type ScheduleKind string

const (
	EveryDays        ScheduleKind = "everyDays"
	SelectedWeekdays ScheduleKind = "selectedWeekdays"
)

// Weekdays use Swift Calendar numbering: Sunday=1 ... Saturday=7.
type Schedule struct {
	Kind        ScheduleKind
	DayInterval int
	Weekdays    []int
	EveryWeeks  int
}

func Daily() Schedule { return Schedule{Kind: EveryDays, DayInterval: 1} }
func Weekdays() Schedule {
	return Schedule{Kind: SelectedWeekdays, Weekdays: []int{2, 3, 4, 5, 6}, EveryWeeks: 1}
}
func (s Schedule) Validate() error {
	switch s.Kind {
	case EveryDays:
		if s.DayInterval >= 1 && s.DayInterval <= 30 && len(s.Weekdays) == 0 && s.EveryWeeks == 0 {
			return nil
		}
	case SelectedWeekdays:
		if s.DayInterval != 0 || s.EveryWeeks < 1 || s.EveryWeeks > 4 || len(s.Weekdays) == 0 {
			return ErrInvalidSchedule
		}
		seen := map[int]bool{}
		for _, d := range s.Weekdays {
			if d < 1 || d > 7 || seen[d] {
				return ErrInvalidSchedule
			}
			seen[d] = true
		}
		return nil
	}
	return ErrInvalidSchedule
}
func (s Schedule) Includes(date, anchor core.LocalDate) bool {
	if s.Validate() != nil {
		return false
	}
	offset, err := dayDistance(anchor, date)
	if err != nil || offset < 0 {
		return false
	}
	if s.Kind == EveryDays {
		return offset%s.DayInterval == 0
	}
	if (offset/7)%s.EveryWeeks != 0 {
		return false
	}
	weekday := int(time.Date(date.Year, time.Month(date.Month), date.Day, 12, 0, 0, 0, time.UTC).Weekday()) + 1
	for _, d := range s.Weekdays {
		if d == weekday {
			return true
		}
	}
	return false
}
func (s Schedule) OccurrenceDates(start, end core.LocalDate) ([]core.LocalDate, error) {
	if e := s.Validate(); e != nil {
		return nil, e
	}
	dates, e := materializableDates(start, end)
	if e != nil {
		return nil, e
	}
	var out []core.LocalDate
	for _, d := range dates {
		if s.Includes(d, start) {
			out = append(out, d)
		}
	}
	return out, nil
}

type EndKind string

const (
	DurationDays     EndKind = "durationDays"
	OnDate           EndKind = "onDate"
	CompletionTarget EndKind = "completionTarget"
)

type EndCondition struct {
	Kind       EndKind
	Days       int
	Date       core.LocalDate
	Count      int
	LatestDate core.LocalDate
}

func Until(date core.LocalDate) EndCondition { return EndCondition{Kind: OnDate, Date: date} }
func (e EndCondition) ResolvedEndDate(anchor core.LocalDate) (core.LocalDate, error) {
	if !anchor.Valid() {
		return core.LocalDate{}, ErrInvalidRange
	}
	switch e.Kind {
	case DurationDays:
		if e.Days < 1 || e.Days > MaximumMaterializedDays || e.Count != 0 || e.Date != (core.LocalDate{}) || e.LatestDate != (core.LocalDate{}) {
			return core.LocalDate{}, ErrInvalidRange
		}
		return anchor.AddDays(e.Days - 1)
	case OnDate:
		if !e.Date.Valid() || e.Days != 0 || e.Count != 0 || e.LatestDate != (core.LocalDate{}) {
			return core.LocalDate{}, ErrInvalidRange
		}
		return e.Date, nil
	case CompletionTarget:
		if e.Count < 1 || e.Count > MaximumMaterializedDays || !e.LatestDate.Valid() || e.Days != 0 || e.Date != (core.LocalDate{}) {
			return core.LocalDate{}, ErrInvalidRange
		}
		return e.LatestDate, nil
	}
	return core.LocalDate{}, ErrInvalidRange
}

type PlanRevision struct {
	ID                                    string
	EffectiveFrom, EndConditionAnchorDate core.LocalDate
	Schedule                              Schedule
	EndCondition                          EndCondition
	RecordedAt                            core.Instant
}
type CancellationScope string

const (
	Occurrence     CancellationScope = "occurrence"
	FollowingDates CancellationScope = "followingDates"
)

type CancellationReason string

const (
	UserSkipped             CancellationReason = "userSkipped"
	PlanChanged             CancellationReason = "planRevision"
	StoppedEarly            CancellationReason = "stoppedEarly"
	CompletionTargetReached CancellationReason = "completionTargetReached"
)

type CancellationFact struct {
	ID         string
	Scope      CancellationScope
	Date       core.LocalDate
	Reason     CancellationReason
	RecordedAt core.Instant
}
type ClassificationRevision struct {
	ID                 string
	PreviousRevisionID *string
	InteractionID      string
	Source             classification.Source
	DecisionID         *string
	CategoryID         *classification.CategoryID
	LabelIDs           []classification.LabelID
	RecordedAt         core.Instant
}
type Series struct {
	ID                      core.TaskCycleSeriesID
	Title                   string
	DescriptionText         *string
	PlannedSubtasks         []core.PlannedSubtask
	CategoryID              *classification.CategoryID
	LabelIDs                []classification.LabelID
	StartDate, EndDate      core.LocalDate
	Schedule                Schedule
	EndCondition            EndCondition
	PlanRevisions           []PlanRevision
	ClassificationRevisions []ClassificationRevision
	CancellationFacts       []CancellationFact
	CreatedAt, UpdatedAt    core.Instant
}

func (s Series) StoppedAfterDate() *core.LocalDate {
	var earliest *core.LocalDate
	for _, f := range s.CancellationFacts {
		if f.Scope == FollowingDates && (earliest == nil || f.Date.Compare(*earliest) < 0) {
			earliest = ptr(f.Date)
		}
	}
	return earliest
}
func (s Series) IsOccurrenceSkipped(date core.LocalDate) bool {
	for _, f := range s.CancellationFacts {
		if f.Scope == Occurrence && f.Date == date {
			return true
		}
	}
	return false
}
func (s Series) latestCancellationReason(date core.LocalDate) (CancellationReason, bool) {
	var latest *CancellationFact
	for i := range s.CancellationFacts {
		f := &s.CancellationFacts[i]
		if f.Scope == Occurrence && f.Date == date && (latest == nil || factBefore(*latest, *f)) {
			latest = f
		}
	}
	if latest == nil {
		return "", false
	}
	return latest.Reason, true
}
func factBefore(a, b CancellationFact) bool {
	if !a.RecordedAt.Equal(b.RecordedAt) {
		return a.RecordedAt.Before(b.RecordedAt)
	}
	return a.ID < b.ID
}
func revisionBefore(a, b ClassificationRevision) bool {
	if !a.RecordedAt.Equal(b.RecordedAt) {
		return a.RecordedAt.Before(b.RecordedAt)
	}
	return a.ID < b.ID
}
func (s Series) CurrentClassificationRevision() *ClassificationRevision {
	parents := map[string]bool{}
	for _, r := range s.ClassificationRevisions {
		if r.PreviousRevisionID != nil {
			parents[*r.PreviousRevisionID] = true
		}
	}
	var latest *ClassificationRevision
	for _, r := range s.ClassificationRevisions {
		if !parents[r.ID] && (latest == nil || revisionBefore(*latest, r)) {
			latest = ptr(cloneClassificationRevision(r))
		}
	}
	return latest
}
func (s Series) EverPlannedDates() ([]core.LocalDate, error) {
	dates := map[core.LocalDate]bool{}
	for _, r := range s.PlanRevisions {
		end, e := r.EndCondition.ResolvedEndDate(r.EndConditionAnchorDate)
		if e != nil {
			return nil, e
		}
		ds, e := r.Schedule.OccurrenceDates(r.EffectiveFrom, end)
		if e != nil {
			return nil, e
		}
		for _, d := range ds {
			dates[d] = true
		}
	}
	out := make([]core.LocalDate, 0, len(dates))
	for d := range dates {
		out = append(out, d)
	}
	sortDates(out)
	return out, nil
}
func (s Series) TrackBounds() (core.LocalDate, core.LocalDate, error) {
	low, high := s.StartDate, s.EndDate
	for _, r := range s.PlanRevisions {
		end, e := r.EndCondition.ResolvedEndDate(r.EndConditionAnchorDate)
		if e != nil {
			return core.LocalDate{}, core.LocalDate{}, e
		}
		for _, d := range []core.LocalDate{r.EffectiveFrom, r.EndConditionAnchorDate, end} {
			if d.Compare(low) < 0 {
				low = d
			}
			if d.Compare(high) > 0 {
				high = d
			}
		}
	}
	return low, high, nil
}

func (s Series) Validate() error {
	fail := func(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidSeries, reason) }
	if !validID(string(s.ID)) || s.Title == "" || strings.TrimSpace(s.Title) != s.Title || !clock(s.CreatedAt, s.UpdatedAt) {
		return fail("identity, title or clock")
	}
	ids := map[core.PlannedSubtaskID]bool{}
	lineage := map[core.SubtaskLineageID]bool{}
	for i, p := range s.PlannedSubtasks {
		if !validID(string(p.ID)) || !validID(string(p.LineageID)) || ids[p.ID] || lineage[p.LineageID] || p.Position != i+1 || p.Title == "" || strings.TrimSpace(p.Title) != p.Title || !p.CreatedAt.Finite() || p.CreatedAt.After(s.UpdatedAt) || p.Difficulty < 1 || p.Difficulty > 3 {
			return fail("planned subtasks")
		}
		ids[p.ID] = true
		lineage[p.LineageID] = true
	}
	if _, e := materializableDates(s.StartDate, s.EndDate); e != nil {
		return e
	}
	end, e := s.EndCondition.ResolvedEndDate(s.StartDate)
	if e != nil || end != s.EndDate || s.Schedule.Validate() != nil || len(s.PlanRevisions) == 0 {
		return fail("current plan")
	}
	revisionIDs := map[string]bool{}
	for _, r := range s.PlanRevisions {
		if !validID(r.ID) || revisionIDs[r.ID] || !r.EffectiveFrom.Valid() || r.EffectiveFrom.Compare(r.EndConditionAnchorDate) < 0 || !clock(s.CreatedAt, r.RecordedAt) || r.RecordedAt.After(s.UpdatedAt) || r.Schedule.Validate() != nil {
			return fail("plan revision")
		}
		revisionIDs[r.ID] = true
		end, e := r.EndCondition.ResolvedEndDate(r.EndConditionAnchorDate)
		if e != nil {
			return e
		}
		if _, e = materializableDates(r.EndConditionAnchorDate, end); e != nil {
			return e
		}
	}
	latest := s.PlanRevisions[len(s.PlanRevisions)-1]
	if !scheduleEqual(latest.Schedule, s.Schedule) || latest.EndCondition != s.EndCondition || latest.EndConditionAnchorDate != s.StartDate {
		return fail("current plan differs from latest revision")
	}
	if e := s.validateClassificationRevisions(); e != nil {
		return e
	}
	planned, e := s.EverPlannedDates()
	if e != nil || len(planned) == 0 {
		return fail("empty or invalid occurrence history")
	}
	plannedSet := map[core.LocalDate]bool{}
	for _, d := range planned {
		plannedSet[d] = true
	}
	facts := map[string]bool{}
	for i, f := range s.CancellationFacts {
		if !validID(f.ID) || facts[f.ID] || !f.Date.Valid() || !clock(s.CreatedAt, f.RecordedAt) || f.RecordedAt.After(s.UpdatedAt) || (i > 0 && factBefore(f, s.CancellationFacts[i-1])) {
			return fail("cancellation fact")
		}
		facts[f.ID] = true
		if f.Reason != UserSkipped && f.Reason != PlanChanged && f.Reason != StoppedEarly && f.Reason != CompletionTargetReached {
			return fail("cancellation reason")
		}
		switch f.Scope {
		case Occurrence:
			if !plannedSet[f.Date] {
				return fail("cancellation date not planned")
			}
		case FollowingDates:
			if f.Date.Compare(s.EndDate) >= 0 {
				return fail("terminal cutoff")
			}
		default:
			return fail("cancellation scope")
		}
	}
	return nil
}
func (s Series) validateClassificationRevisions() error {
	if len(s.ClassificationRevisions) == 0 {
		return ErrInvalidSeries
	}
	ids := map[string]ClassificationRevision{}
	interactions := map[string]bool{}
	roots := 0
	for i, r := range s.ClassificationRevisions {
		_, duplicate := ids[r.ID]
		if !validID(r.ID) || !validID(r.InteractionID) || duplicate || interactions[r.InteractionID] || !clock(s.CreatedAt, r.RecordedAt) || r.RecordedAt.After(s.UpdatedAt) || (i > 0 && revisionBefore(r, s.ClassificationRevisions[i-1])) || !validLabels(r.LabelIDs) || (r.CategoryID != nil && !validID(string(*r.CategoryID))) || (r.DecisionID != nil && !validID(*r.DecisionID)) {
			return ErrInvalidSeries
		}
		if e := r.Source.Validate(); e != nil {
			return e
		}
		ids[r.ID] = r
		interactions[r.InteractionID] = true
		if r.PreviousRevisionID == nil {
			roots++
		}
	}
	if roots != 1 {
		return ErrInvalidSeries
	}
	for _, r := range s.ClassificationRevisions {
		visited := map[string]bool{r.ID: true}
		previous := r.PreviousRevisionID
		current := r
		for previous != nil {
			p, ok := ids[*previous]
			if !ok || visited[*previous] || p.RecordedAt.After(current.RecordedAt) {
				return ErrInvalidSeries
			}
			visited[*previous] = true
			previous = p.PreviousRevisionID
			current = p
		}
	}
	leaf := s.CurrentClassificationRevision()
	if leaf == nil || !categoryEqual(leaf.CategoryID, s.CategoryID) || !labelsEqual(leaf.LabelIDs, s.LabelIDs) {
		return ErrInvalidSeries
	}
	return nil
}
func validLabels(ids []classification.LabelID) bool {
	seen := map[classification.LabelID]bool{}
	for _, id := range ids {
		if !validID(string(id)) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func labelsEqual(a, b []classification.LabelID) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[classification.LabelID]bool{}
	for _, id := range a {
		set[id] = true
	}
	for _, id := range b {
		if !set[id] {
			return false
		}
	}
	return true
}
func categoryEqual(a, b *classification.CategoryID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func scheduleEqual(a, b Schedule) bool {
	if a.Kind != b.Kind || a.DayInterval != b.DayInterval || a.EveryWeeks != b.EveryWeeks || len(a.Weekdays) != len(b.Weekdays) {
		return false
	}
	seen := map[int]bool{}
	for _, x := range a.Weekdays {
		seen[x] = true
	}
	for _, x := range b.Weekdays {
		if !seen[x] {
			return false
		}
	}
	return true
}
func ptr[T any](x T) *T { return &x }
func cp[T any](x *T) *T {
	if x == nil {
		return nil
	}
	return ptr(*x)
}
func validID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
func clock(before, after core.Instant) bool {
	return before.Finite() && after.Finite() && !after.Before(before)
}
func dayDistance(a, b core.LocalDate) (int, error) {
	if !a.Valid() || !b.Valid() {
		return 0, ErrInvalidRange
	}
	aa := time.Date(a.Year, time.Month(a.Month), a.Day, 12, 0, 0, 0, time.UTC).Unix()
	bb := time.Date(b.Year, time.Month(b.Month), b.Day, 12, 0, 0, 0, time.UTC).Unix()
	return int((bb - aa) / 86400), nil
}
func materializableDates(start, end core.LocalDate) ([]core.LocalDate, error) {
	distance, e := dayDistance(start, end)
	if e != nil || distance < 0 || distance >= MaximumMaterializedDays {
		return nil, ErrInvalidRange
	}
	out := make([]core.LocalDate, 0, distance+1)
	for i := 0; i <= distance; i++ {
		d, e := start.AddDays(i)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, nil
}
func sortDates(d []core.LocalDate) {
	sort.Slice(d, func(i, j int) bool { return d[i].Compare(d[j]) < 0 })
}
func cloneSchedule(s Schedule) Schedule { s.Weekdays = append([]int(nil), s.Weekdays...); return s }
func cloneClassificationRevision(r ClassificationRevision) ClassificationRevision {
	r.PreviousRevisionID = cp(r.PreviousRevisionID)
	r.DecisionID = cp(r.DecisionID)
	r.CategoryID = cp(r.CategoryID)
	r.LabelIDs = append([]classification.LabelID(nil), r.LabelIDs...)
	return r
}
func (s Series) Clone() Series {
	s.DescriptionText = cp(s.DescriptionText)
	s.PlannedSubtasks = append([]core.PlannedSubtask(nil), s.PlannedSubtasks...)
	s.CategoryID = cp(s.CategoryID)
	s.LabelIDs = append([]classification.LabelID(nil), s.LabelIDs...)
	s.Schedule = cloneSchedule(s.Schedule)
	s.PlanRevisions = append([]PlanRevision(nil), s.PlanRevisions...)
	for i := range s.PlanRevisions {
		s.PlanRevisions[i].Schedule = cloneSchedule(s.PlanRevisions[i].Schedule)
	}
	s.ClassificationRevisions = append([]ClassificationRevision(nil), s.ClassificationRevisions...)
	for i, r := range s.ClassificationRevisions {
		s.ClassificationRevisions[i] = cloneClassificationRevision(r)
	}
	s.CancellationFacts = append([]CancellationFact(nil), s.CancellationFacts...)
	return s
}
