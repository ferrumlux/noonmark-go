// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"time"
)

type OrdinaryReadView struct {
	Graph          core.TaskGraph
	Today          core.LocalDate
	Classification ordinary.AggregateState
	StoreRevision  int
	Health         SessionHealth
}

func (s *Service) ReadView() (OrdinaryReadView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readOrdinaryViewLocked()
}
func (s *Service) readOrdinaryViewLocked() (OrdinaryReadView, error) {
	if s.aggregate == nil {
		return OrdinaryReadView{}, ordinary.ErrContext
	}
	g, state, err := s.aggregate.document.Facts()
	if err != nil {
		return OrdinaryReadView{}, err
	}
	return OrdinaryReadView{g, s.today, state, s.disk.revision, s.health}, nil
}
func (s *Service) admitInitialOrdinaryLocked() error {
	b, err := canonical.Marshal(historyObject(map[string]canonical.Value{"formatVersion": historyNumber(7), "snapshot": s.snapshot}))
	if err != nil {
		return err
	}
	a, err := admitOrdinaryAggregate(b, canonical.DefaultLimits())
	if err != nil {
		return err
	}
	if s.disk == nil || s.disk.ordinaryService != nil || s.disk.ordinaryHead != nil {
		return ordinary.ErrContext
	}
	s.aggregate = &a
	s.lineage = &serviceLineage{epoch: 1}
	s.disk.ordinaryService = s
	s.disk.ordinaryHead = s.aggregate
	return nil
}

type ordinaryTaskEdge struct {
	input    ordinaryTransition
	eventIDs []ordinary.UUID
}
type ordinaryCategoryEdge struct {
	plan         CategoryPlan
	confirmation CategoryConfirmation
	now          core.Instant
	ids          ordinary.CommitIDs
}
type persistableOrdinaryCandidate struct {
	before   *admittedAggregate
	owner    *serviceLineage
	head     string
	revision int
	after    admittedAggregate
	bytes    []byte
	task     *ordinaryTaskEdge
	category *ordinaryCategoryEdge
}

func (s *Service) finishOrdinaryCandidateLocked(document v7.OrdinaryAggregate) (persistableOrdinaryCandidate, error) {
	payload, err := v7.EncodeOrdinaryAggregate(document)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	after, err := admitOrdinaryAggregate(payload, canonical.DefaultLimits())
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	check, err := v7.EncodeOrdinaryAggregate(after.document)
	if err != nil || !bytes.Equal(payload, check) {
		return persistableOrdinaryCandidate{}, ordinary.ErrInvalid
	}
	return persistableOrdinaryCandidate{before: s.aggregate, owner: s.lineage, head: s.disk.hash, revision: s.disk.revision, after: after, bytes: payload}, nil
}
func (s *Service) categoryCandidateLocked(p CategoryPlan, c CategoryConfirmation, next ordinary.AggregateState, now core.Instant, ids ordinary.CommitIDs) (persistableOrdinaryCandidate, error) {
	if p.sealed == nil || c.sealed == nil || c.sealed.plan != p.sealed || p.sealed.owner != s.lineage {
		return persistableOrdinaryCandidate{}, ordinary.ErrContext
	}
	g, state, err := s.aggregate.document.Facts()
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	expected, _, err := ordinary.ApplyAggregatePlan(state, g, p.sealed.data, c.sealed.decision, now, ids, ordinary.DefaultLimits())
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	if !reflect.DeepEqual(next, expected) {
		return persistableOrdinaryCandidate{}, ordinary.ErrInvalid
	}
	d, err := v7.WithOrdinaryFacts(s.aggregate.document, g, expected)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	candidate, err := s.finishOrdinaryCandidateLocked(d)
	candidate.category = &ordinaryCategoryEdge{p, c, now, ids}
	return candidate, err
}
func (s *Service) taskCandidateLocked(in ordinaryTransition) (persistableOrdinaryCandidate, error) {
	g, state, err := s.aggregate.document.Facts()
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	after, err := in.replay(g)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	// Count only the successful replay's source-defined capture cases.
	oldDays := map[core.LocalDate]core.Day{}
	newDays := map[core.LocalDate]core.Day{}
	for _, d := range g.Days {
		oldDays[d.Date] = d
	}
	for _, d := range after.Days {
		newDays[d.Date] = d
	}
	count := 0
	for _, tr := range after.Traces {
		if in.kind == ordinarySettle && newDays[tr.Date].LockedAt != nil && oldDays[tr.Date].LockedAt == nil && (tr.Status == core.TraceCompleted || tr.Status == core.TraceUnfinished) {
			count++
		}
		if in.kind == ordinaryReturn && tr.ID == in.returning.TraceID && tr.Status == core.TraceReturnedToPool {
			count++
		}
		if in.kind == ordinaryDefer && tr.ID == in.carry.TraceID && tr.Status == core.TraceDeferred {
			count++
		}
	}
	raw, err := newIDs(count)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	ids := make([]ordinary.UUID, count)
	for i, id := range raw {
		ids[i] = ordinary.UUID(id)
	}
	after, events, err := deriveOrdinaryCaptures(g, state, in, ids)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	if state.SnapshotsByTraceID == nil {
		state.SnapshotsByTraceID = map[core.DayTraceID]ordinary.TraceClassificationSnapshot{}
	}
	if state.SnapshotEventsByTraceID == nil {
		state.SnapshotEventsByTraceID = map[core.DayTraceID][]ordinary.TraceClassificationSnapshot{}
	}
	for _, event := range events {
		state.SnapshotsByTraceID[event.TraceID] = event
		state.SnapshotEventsByTraceID[event.TraceID] = append(state.SnapshotEventsByTraceID[event.TraceID], event)
		state.Revision = event.Revision
	}
	d, err := v7.WithOrdinaryFacts(s.aggregate.document, after, state)
	if err != nil {
		return persistableOrdinaryCandidate{}, err
	}
	candidate, err := s.finishOrdinaryCandidateLocked(d)
	candidate.task = &ordinaryTaskEdge{in.owned(), append([]ordinary.UUID(nil), ids...)}
	return candidate, err
}
func (s *Service) persistOrdinaryLocked(c persistableOrdinaryCandidate, a ...*attempt) error {
	if err := s.gateLocked(); err != nil {
		return err
	}
	if (c.task == nil) == (c.category == nil) {
		return ordinary.ErrContext
	}
	if c.before == nil || c.before != s.aggregate || c.owner == nil || c.owner != s.lineage || c.head != s.disk.hash || c.revision != s.disk.revision {
		return ordinary.ErrContext
	}
	g, _, err := c.after.document.Facts()
	if err != nil {
		return err
	}
	wire, err := c.after.document.Snapshot()
	if err != nil {
		return err
	}
	// The writer independently binds the actual handle and replays the retained
	// edge before reserve. Only its confirmed aggregate head is published below.
	o := s.disk.commitOrdinaryObserved(c)
	setAttempt(a, o.extent, o.stage)
	if o.invalidates {
		s.invalidateLocked(o.reason, o.stage)
	}
	if o.err != nil {
		return o.err
	}
	s.aggregate = s.disk.ordinaryHead
	s.graph = g
	s.snapshot = wire
	return nil
}
func (s *Service) commitOrdinaryTaskLocked(in ordinaryTransition, a ...*attempt) error {
	if err := s.gateLocked(); err != nil {
		return err
	}
	setAttempt(a, WriteNone, StageCandidate)
	candidate, err := s.taskCandidateLocked(in)
	if err != nil {
		return err
	}
	old, err := v7.EncodeOrdinaryAggregate(s.aggregate.document)
	if err != nil {
		return err
	}
	if bytes.Equal(old, candidate.bytes) {
		_, err = s.verifyPrefixLocked(a...)
		return err
	}
	return s.persistOrdinaryLocked(candidate, a...)
}
func (s *Service) reconcileOrdinaryDayLocked(today core.LocalDate, a ...*attempt) error {
	if err := s.gateLocked(); err != nil {
		return err
	}
	frontier, err := s.persistedFrontier()
	if err != nil {
		return err
	}
	in := ordinaryTransition{kind: ordinarySettle, settle: ordinarySettlementInput{Today: today, Now: frontier}}
	after, err := in.replay(s.graph)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(after, s.graph) {
		if _, err = s.verifyPrefixLocked(a...); err != nil {
			return err
		}
		s.today = today
		return nil
	}
	now, err := s.nextInstant()
	if err != nil {
		return err
	}
	in.settle.Now = now
	if err = s.commitOrdinaryTaskLocked(in, a...); err != nil {
		return err
	}
	s.today = today
	return nil
}

func (s *Service) ordinarySubtaskAckLocked(c subtaskEditCommand) (subtaskEditAck, error) {
	fail := func(err error) (subtaskEditAck, error) { return subtaskEditAck{}, err }
	var in ordinaryTransition
	if c.planned != nil {
		in = ordinaryTransition{kind: ordinaryPlanned, planned: *c.planned}
	} else {
		in = ordinaryTransition{kind: ordinaryActual, actual: *c.actual}
	}
	candidate, err := s.taskCandidateLocked(in)
	if err != nil {
		return fail(err)
	}
	g, _, err := candidate.after.document.Facts()
	if err != nil {
		return fail(err)
	}
	ack := subtaskEditAck{}
	if c.planned != nil {
		ack.planned = c.planned.SubtaskID
		found := false
		for _, d := range g.Definitions {
			if d.ID == c.planned.ExpectedDefinitionID && d.ChainID == c.planned.ChainID {
				for _, sub := range d.PlannedSubtasks {
					if sub.ID == ack.planned {
						ack.title = sub.Title
						found = true
					}
				}
			}
		}
		if !found && c.planned.Kind != core.PlannedSubtaskRemove {
			return fail(core.ErrNotFound)
		}
	} else {
		ack.actual = c.actual.SubtaskID
		found := false
		for _, sub := range g.Subtasks {
			if sub.ID == ack.actual && sub.TraceID == c.actual.TraceID {
				ack.title = sub.Title
				found = true
			}
		}
		if !found {
			return fail(core.ErrNotFound)
		}
	}

	old, err := v7.EncodeOrdinaryAggregate(s.aggregate.document)
	if err != nil {
		return fail(err)
	}
	if bytes.Equal(old, candidate.bytes) {
		_, err = s.verifyPrefixLocked()
	} else {
		err = s.persistOrdinaryLocked(candidate)
	}
	if err != nil {
		return fail(err)
	}
	return ack, nil
}

// These explicit development entrypoints keep the frozen default gate intact.
// They return the same Service and retain every current V2 command; they are
// not a reduced task mode or a production/import compatibility promise.
func CreateOrdinaryDevelopment(path string, today core.LocalDate) (*Service, error) {
	s, err := Create(path, today)
	if err != nil {
		return nil, err
	}
	if err = s.admitInitialOrdinaryLocked(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func OpenOrdinaryDevelopment(path string, today core.LocalDate) (*Service, error) {
	if !today.Valid() {
		return nil, core.ErrInvalidInput
	}
	d, wire, err := openOrdinaryStore(path)
	if err != nil {
		return nil, err
	}
	s := &Service{disk: d, snapshot: wire, today: today, clock: time.Now, health: SessionHealth{SessionWritable, HealthOK, StageNone}}
	if err = s.admitInitialOrdinaryLocked(); err != nil {
		d.root.Close()
		return nil, err
	}
	s.graph, _, err = s.aggregate.document.Facts()
	if err != nil {
		d.root.Close()
		return nil, err
	}
	if err = s.reconcileDayLocked(today); err != nil {
		d.root.Close()
		return nil, err
	}
	return s, nil
}
func (s *Service) ordinaryEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aggregate != nil
}
func (s *Service) addOrdinary(title string, schedule bool, date *core.LocalDate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcileDayLocked(s.today); err != nil {
		return err
	}
	now, err := s.nextInstant()
	if err != nil {
		return err
	}
	ids, err := newIDs(4)
	if err != nil {
		return err
	}
	target := s.today
	if date != nil {
		target = *date
	}
	in := ordinaryTransition{kind: ordinaryAdd, scheduled: schedule, add: core.CreatePoolTaskInput{ChainID: core.TaskChainID(ids[0]), DefinitionID: core.TaskDefinitionID(ids[1]), Title: title, Now: now}, schedule: core.ScheduleFromPoolInput{ChainID: core.TaskChainID(ids[0]), TraceID: core.DayTraceID(ids[2]), DayID: core.DayID(ids[3]), Date: target, Today: s.today, Now: now}}
	return s.commitOrdinaryTaskLocked(in)
}
func (s *Service) scheduleOrdinary(chain core.TaskChainID, date core.LocalDate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcileDayLocked(s.today); err != nil {
		return err
	}
	now, err := s.nextInstant()
	if err != nil {
		return err
	}
	ids, err := newIDs(2)
	if err != nil {
		return err
	}
	children := map[core.PlannedSubtaskID]core.SubtaskID{}
	for _, d := range s.graph.Definitions {
		if d.ChainID == chain && d.SupersededAt == nil {
			for _, p := range d.PlannedSubtasks {
				id, err := newIDs(1)
				if err != nil {
					return err
				}
				children[p.ID] = core.SubtaskID(id[0])
			}
		}
	}
	return s.commitOrdinaryTaskLocked(ordinaryTransition{kind: ordinarySchedule, schedule: core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: date, Today: s.today, Now: now, SubtaskIDs: children}})
}
func (s *Service) completeOrdinary(trace core.DayTraceID, completed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcileDayLocked(s.today); err != nil {
		return err
	}
	now, err := s.nextInstant()
	if err != nil {
		return err
	}
	action := completionHistoryUndo
	if completed {
		action = completionHistoryComplete
	}
	return s.commitOrdinaryTaskLocked(ordinaryTransition{kind: ordinaryComplete, completion: completionHistoryInput{Action: action, TraceID: trace, Today: s.today, Now: now}})
}

func (s *Service) allPersistedClocks() ([]core.Instant, error) {
	clocks := s.persistedClocks()
	if s.aggregate == nil {
		return clocks, nil
	}
	_, state, err := s.aggregate.document.Facts()
	if err != nil {
		return nil, err
	}
	clock, ok, err := ordinary.AggregateClock(state, ordinary.DefaultLimits())
	if err != nil {
		return nil, err
	}
	if ok {
		clocks = append(clocks, clock)
	}
	return clocks, nil
}

// Rebind the retained closed edge immediately before the writer. A valid raw
// snapshot or a hash never substitutes for this before/owner/head/edge check.
func (s *Service) verifyOrdinaryCandidateLocked(c persistableOrdinaryCandidate) error {
	if (c.task == nil) == (c.category == nil) {
		return ordinary.ErrContext
	}
	graph, state, err := c.before.document.Facts()
	if err != nil {
		return err
	}
	if c.task != nil {
		var events []ordinary.TraceClassificationSnapshot
		graph, events, err = deriveOrdinaryCaptures(graph, state, c.task.input, c.task.eventIDs)
		if err != nil {
			return err
		}
		if state.SnapshotsByTraceID == nil {
			state.SnapshotsByTraceID = map[core.DayTraceID]ordinary.TraceClassificationSnapshot{}
		}
		if state.SnapshotEventsByTraceID == nil {
			state.SnapshotEventsByTraceID = map[core.DayTraceID][]ordinary.TraceClassificationSnapshot{}
		}
		for _, event := range events {
			state.SnapshotsByTraceID[event.TraceID] = event
			state.SnapshotEventsByTraceID[event.TraceID] = append(state.SnapshotEventsByTraceID[event.TraceID], event)
			state.Revision = event.Revision
		}
	} else {
		e := c.category
		if e.plan.sealed == nil || e.plan.sealed.owner != s.lineage || e.confirmation.sealed == nil || e.confirmation.sealed.plan != e.plan.sealed {
			return ordinary.ErrContext
		}
		state, _, err = ordinary.ApplyAggregatePlan(state, graph, e.plan.sealed.data, e.confirmation.sealed.decision, e.now, e.ids, ordinary.DefaultLimits())
		if err != nil {
			return err
		}
	}
	doc, err := v7.WithOrdinaryFacts(c.before.document, graph, state)
	if err != nil {
		return err
	}
	expected, err := v7.EncodeOrdinaryAggregate(doc)
	if err != nil {
		return err
	}
	if !bytes.Equal(expected, c.bytes) {
		return ordinary.ErrInvalid
	}
	return nil
}
