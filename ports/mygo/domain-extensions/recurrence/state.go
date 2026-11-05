// SPDX-License-Identifier: AGPL-3.0-only
package recurrence

import (
	"fmt"
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type State struct {
	Tasks  core.TaskGraph
	Series []Series
}

func (s State) Clone() State {
	out := State{Tasks: s.Tasks.Clone()}
	if s.Series != nil {
		out.Series = make([]Series, len(s.Series))
		for i, p := range s.Series {
			out.Series[i] = p.Clone()
		}
	}
	return out
}

// Validate adds all recurring membership/occurrence invariants to core validation.
// The full snapshot owner must additionally validate classification references.
func (s State) Validate() error {
	if e := core.ValidateTaskGraph(s.Tasks); e != nil {
		return e
	}
	series := map[core.TaskCycleSeriesID]Series{}
	for _, p := range s.Series {
		if _, duplicate := series[p.ID]; duplicate {
			return fmt.Errorf("%w: duplicate series", ErrInvalidSeries)
		}
		if e := p.Validate(); e != nil {
			return e
		}
		series[p.ID] = p
	}
	traced := map[core.TaskChainID]bool{}
	for _, t := range s.Tasks.Traces {
		traced[t.ChainID] = true
	}
	members := map[core.TaskCycleSeriesID]map[core.LocalDate]bool{}
	for _, c := range s.Tasks.Chains {
		m := c.CycleMembership
		if m == nil {
			continue
		}
		if _, ok := series[m.SeriesID]; !ok || !traced[c.ID] {
			return fmt.Errorf("%w: missing series or occurrence trace", ErrInvalidSeries)
		}
		if members[m.SeriesID] == nil {
			members[m.SeriesID] = map[core.LocalDate]bool{}
		}
		if members[m.SeriesID][m.OccurrenceDate] {
			return fmt.Errorf("%w: duplicate occurrence", ErrInvalidSeries)
		}
		members[m.SeriesID][m.OccurrenceDate] = true
	}
	for _, p := range s.Series {
		dates, e := p.EverPlannedDates()
		if e != nil {
			return e
		}
		if len(members[p.ID]) != len(dates) {
			return fmt.Errorf("%w: missing scheduled occurrence", ErrInvalidSeries)
		}
		for _, d := range dates {
			if !members[p.ID][d] {
				return fmt.Errorf("%w: occurrence history differs", ErrInvalidSeries)
			}
		}
	}
	return nil
}
func (s State) ValidateCatalogReferences(categoryExists func(classification.CategoryID) bool, labelExists func(classification.LabelID) bool) error {
	for _, p := range s.Series {
		for _, r := range p.ClassificationRevisions {
			if r.CategoryID != nil && (categoryExists == nil || !categoryExists(*r.CategoryID)) {
				return ErrInvalidSeries
			}
			for _, id := range r.LabelIDs {
				if labelExists == nil || !labelExists(id) {
					return ErrInvalidSeries
				}
			}
		}
	}
	return nil
}
func (s State) seriesIndex(id core.TaskCycleSeriesID) int {
	for i, p := range s.Series {
		if p.ID == id {
			return i
		}
	}
	return -1
}
func (s State) memberChain(id core.TaskCycleSeriesID, date core.LocalDate) *core.TaskChain {
	for _, c := range s.Tasks.Chains {
		if m := c.CycleMembership; m != nil && m.SeriesID == id && m.OccurrenceDate == date {
			return ptr(c)
		}
	}
	return nil
}
func (s State) EditableMemberChainIDs(id core.TaskCycleSeriesID, today core.LocalDate) []core.TaskChainID {
	var members []core.TaskChain
	for _, c := range s.Tasks.Chains {
		if c.State != core.ChainActive || c.CycleMembership == nil || c.CycleMembership.SeriesID != id || c.CycleMembership.OccurrenceDate.Compare(today) < 0 {
			continue
		}
		for _, t := range s.Tasks.Traces {
			if t.ChainID == c.ID && t.Status == core.TracePending && t.Date.Compare(today) >= 0 {
				members = append(members, c)
				break
			}
		}
	}
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if a.CycleMembership.OccurrenceDate != b.CycleMembership.OccurrenceDate {
			return a.CycleMembership.OccurrenceDate.Compare(b.CycleMembership.OccurrenceDate) < 0
		}
		return a.ID < b.ID
	})
	out := make([]core.TaskChainID, len(members))
	for i, c := range members {
		out[i] = c.ID
	}
	return out
}
func (s State) CanReviseStartDate(id core.TaskCycleSeriesID, today core.LocalDate) bool {
	i := s.seriesIndex(id)
	if i < 0 || s.Series[i].StoppedAfterDate() != nil || s.Series[i].StartDate.Compare(today) <= 0 {
		return false
	}
	members := map[core.TaskChainID]bool{}
	for _, c := range s.Tasks.Chains {
		if c.CycleMembership != nil && c.CycleMembership.SeriesID == id {
			members[c.ID] = true
		}
	}
	if len(members) == 0 {
		return false
	}
	for _, t := range s.Tasks.Traces {
		if members[t.ChainID] && t.Status != core.TraceCancelledDraft && (t.Status != core.TracePending || t.Date.Compare(today) <= 0) {
			return false
		}
	}
	return true
}

type LifecycleKind string

const (
	Upcoming LifecycleKind = "upcoming"
	Active   LifecycleKind = "active"
	Ended    LifecycleKind = "ended"
	Stopped  LifecycleKind = "stopped"
)

type Lifecycle struct {
	Kind LifecycleKind
	Date *core.LocalDate
}

func (l Lifecycle) AllowsPlanEditing() bool { return l.Kind == Active || l.Kind == Upcoming }
func (s Series) Lifecycle(today core.LocalDate) Lifecycle {
	var terminal *CancellationFact
	for _, f := range s.CancellationFacts {
		if f.Scope != FollowingDates || (f.Reason != StoppedEarly && f.Reason != CompletionTargetReached) {
			continue
		}
		if terminal == nil || f.Date.Compare(terminal.Date) < 0 || (f.Date == terminal.Date && factBefore(f, *terminal)) {
			terminal = ptr(f)
		}
	}
	if terminal != nil {
		kind := Stopped
		if terminal.Reason == CompletionTargetReached {
			kind = Ended
		}
		return Lifecycle{Kind: kind, Date: ptr(terminal.Date)}
	}
	if today.Compare(s.StartDate) < 0 {
		return Lifecycle{Kind: Upcoming, Date: ptr(s.StartDate)}
	}
	if today.Compare(s.EndDate) > 0 {
		return Lifecycle{Kind: Ended, Date: ptr(s.EndDate)}
	}
	return Lifecycle{Kind: Active}
}

type TrackState string

const (
	NotScheduled   TrackState = "notScheduled"
	PendingPast    TrackState = "pendingPast"
	PendingToday   TrackState = "pendingToday"
	Planned        TrackState = "planned"
	Completed      TrackState = "completed"
	Unfinished     TrackState = "unfinished"
	Deferred       TrackState = "deferred"
	Changed        TrackState = "changed"
	ReturnedToPool TrackState = "returnedToPool"
	Skipped        TrackState = "skipped"
	Abandoned      TrackState = "abandoned"
)

type TraceTarget struct {
	TraceID core.DayTraceID
	Date    core.LocalDate
	State   TrackState
}
type TrackDay struct {
	Date                                                                  core.LocalDate
	ChainID                                                               *core.TaskChainID
	OccurrenceTarget, FuturePlanTarget, UnfinishedTarget, CompletedTarget *TraceTarget
}

func (d TrackDay) State() TrackState {
	if d.OccurrenceTarget == nil {
		return NotScheduled
	}
	return d.OccurrenceTarget.State
}
func (d TrackDay) NavigationTarget() *TraceTarget {
	if d.State() == Skipped || d.State() == ReturnedToPool {
		return nil
	}
	return cp(d.OccurrenceTarget)
}

type Track struct {
	ID                 core.TaskCycleSeriesID
	Title              string
	StartDate, EndDate core.LocalDate
	Schedule           Schedule
	EndCondition       EndCondition
	Lifecycle          Lifecycle
	Days               []TrackDay
}

func (t Track) ScheduledCount() int {
	n := 0
	for _, d := range t.Days {
		if d.State() != NotScheduled {
			n++
		}
	}
	return n
}
func (t Track) CompletedCount() int {
	n := 0
	for _, d := range t.Days {
		if d.CompletedTarget != nil {
			n++
		}
	}
	return n
}
func (t Track) UnfinishedCount() int {
	n := 0
	for _, d := range t.Days {
		if d.UnfinishedTarget != nil {
			n++
		}
	}
	return n
}
func (t Track) PlannedCount() int {
	n := 0
	for _, d := range t.Days {
		if d.State() == Planned {
			n++
		}
	}
	return n
}
func (t Track) FuturePlanCount() int {
	n := 0
	for _, d := range t.Days {
		if d.FuturePlanTarget != nil {
			n++
		}
	}
	return n
}
func (t Track) PooledOccurrenceCount() int {
	n := 0
	for _, d := range t.Days {
		if d.State() == ReturnedToPool {
			n++
		}
	}
	return n
}
func (t Track) CompletionGoalCount() int {
	if t.EndCondition.Kind == CompletionTarget {
		return t.EndCondition.Count
	}
	return t.ScheduledCount()
}
func (t Track) CanStop() bool { return t.Lifecycle.AllowsPlanEditing() && t.FuturePlanCount() > 0 }
func (s State) Track(id core.TaskCycleSeriesID, today core.LocalDate) (Track, error) {
	if err := s.Validate(); err != nil {
		return Track{}, err
	}
	if !today.Valid() {
		return Track{}, ErrInvalidRange
	}
	i := s.seriesIndex(id)
	if i < 0 {
		return Track{}, ErrNotFound
	}
	series := s.Series[i]
	start, end, e := series.TrackBounds()
	if e != nil {
		return Track{}, e
	}
	members := map[core.LocalDate]core.TaskChain{}
	traces := map[core.TaskChainID][]core.DayTrace{}
	for _, c := range s.Tasks.Chains {
		if m := c.CycleMembership; m != nil && m.SeriesID == id {
			members[m.OccurrenceDate] = c
			if m.OccurrenceDate.Compare(start) < 0 {
				start = m.OccurrenceDate
			}
			if m.OccurrenceDate.Compare(end) > 0 {
				end = m.OccurrenceDate
			}
		}
	}
	for _, t := range s.Tasks.Traces {
		traces[t.ChainID] = append(traces[t.ChainID], t)
	}
	dates, e := materializableDates(start, end)
	if e != nil {
		return Track{}, e
	}
	out := Track{ID: id, Title: series.Title, StartDate: start, EndDate: end, Schedule: cloneSchedule(series.Schedule), EndCondition: series.EndCondition, Lifecycle: series.Lifecycle(today)}
	for _, date := range dates {
		day := TrackDay{Date: date}
		if c, ok := members[date]; ok {
			day.ChainID = ptr(c.ID)
			ts := traces[c.ID]
			scheduled := latestTrace(ts, func(t core.DayTrace) bool { return t.Date == date })
			latest := latestTrace(ts, func(core.DayTrace) bool { return true })
			representative := scheduled
			if scheduled == nil || (scheduled.Status == core.TraceCancelledDraft && latest != nil && latest.ID != scheduled.ID) {
				representative = latest
			}
			cancelledState := ReturnedToPool
			if series.IsOccurrenceSkipped(date) {
				cancelledState = Skipped
			}
			day.OccurrenceTarget = traceTarget(representative, today, cancelledState)
			day.FuturePlanTarget = traceTarget(latestTrace(ts, func(t core.DayTrace) bool { return t.Status == core.TracePending && t.Date.Compare(today) > 0 }), today, ReturnedToPool)
			day.CompletedTarget = traceTarget(latestTrace(ts, func(t core.DayTrace) bool { return t.Status == core.TraceCompleted }), today, ReturnedToPool)
			if day.CompletedTarget == nil {
				day.UnfinishedTarget = traceTarget(latestTrace(ts, func(t core.DayTrace) bool { return t.Status == core.TraceUnfinished || t.Status == core.TraceAbandoned }), today, ReturnedToPool)
			}
		}
		out.Days = append(out.Days, day)
	}
	if out.Lifecycle.Kind == Active {
		for _, d := range out.Days {
			var candidate *core.LocalDate
			if d.FuturePlanTarget != nil {
				candidate = ptr(d.FuturePlanTarget.Date)
			} else if d.Date == today && d.State() == PendingToday {
				candidate = ptr(d.OccurrenceTarget.Date)
			}
			if candidate != nil && (out.Lifecycle.Date == nil || candidate.Compare(*out.Lifecycle.Date) < 0) {
				out.Lifecycle.Date = candidate
			}
		}
	}
	return out, nil
}
func (s State) Tracks(today core.LocalDate) ([]Track, error) {
	var out []Track
	for _, p := range s.Series {
		t, e := s.Track(p.ID, today)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	rank := map[LifecycleKind]int{Active: 0, Upcoming: 1, Ended: 2, Stopped: 3}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank[a.Lifecycle.Kind] != rank[b.Lifecycle.Kind] {
			return rank[a.Lifecycle.Kind] < rank[b.Lifecycle.Kind]
		}
		ad, bd := a.Lifecycle.Date, b.Lifecycle.Date
		if (ad == nil) != (bd == nil) {
			return ad == nil
		}
		if ad != nil && bd != nil && *ad != *bd {
			if a.Lifecycle.Kind == Ended || a.Lifecycle.Kind == Stopped {
				return ad.Compare(*bd) > 0
			}
			return ad.Compare(*bd) < 0
		}
		return a.ID < b.ID
	})
	return out, nil
}
func latestTrace(values []core.DayTrace, accept func(core.DayTrace) bool) *core.DayTrace {
	var latest *core.DayTrace
	for _, t := range values {
		if accept(t) && (latest == nil || t.CreatedAt.After(latest.CreatedAt) || (t.CreatedAt.Equal(latest.CreatedAt) && t.ID > latest.ID)) {
			latest = ptr(t)
		}
	}
	return latest
}
func traceTarget(t *core.DayTrace, today core.LocalDate, cancelled TrackState) *TraceTarget {
	if t == nil {
		return nil
	}
	var state TrackState
	switch t.Status {
	case core.TracePending:
		state = Planned
		if t.Date == today {
			state = PendingToday
		} else if t.Date.Compare(today) < 0 {
			state = PendingPast
		}
	case core.TraceCompleted:
		state = Completed
	case core.TraceUnfinished:
		state = Unfinished
	case core.TraceDeferred:
		state = Deferred
	case core.TraceChanged:
		state = Changed
	case core.TraceReturnedToPool:
		state = ReturnedToPool
	case core.TraceAbandoned:
		state = Abandoned
	case core.TraceCancelledDraft:
		state = cancelled
	default:
		return nil
	}
	return &TraceTarget{TraceID: t.ID, Date: t.Date, State: state}
}
