// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"sync"
	"time"
)

type Service struct {
	mu        sync.RWMutex
	disk      *diskStore
	graph     core.TaskGraph
	snapshot  canonical.Value
	today     core.LocalDate
	clock     func() time.Time
	closed    bool
	health    SessionHealth
	aggregate *admittedAggregate
	lineage   *serviceLineage
}

func Create(path string, today core.LocalDate) (*Service, error) {
	if !today.Valid() {
		return nil, core.ErrInvalidInput
	}
	d, e := createStore(path)
	if e != nil {
		return nil, e
	}
	s := &Service{disk: d, today: today, clock: time.Now, health: SessionHealth{SessionWritable, HealthOK, StageNone}}
	wire, e := initialSnapshot()
	if e != nil {
		d.root.Close()
		return nil, e
	}
	wire, g, e := normalizeSnapshot(wire)
	if e != nil {
		d.root.Close()
		return nil, e
	}
	if e = d.commit(wire); e != nil {
		d.root.Close()
		return nil, e
	}
	s.graph = g
	s.snapshot = wire
	return s, nil
}
func Open(path string, today core.LocalDate) (*Service, error) {
	if !today.Valid() {
		return nil, core.ErrInvalidInput
	}
	d, wire, e := openStore(path)
	if e != nil {
		return nil, e
	}
	state, _, e := v7.ToDomain(wire)
	if e != nil {
		d.root.Close()
		return nil, e
	}
	g := state.Tasks
	s := &Service{disk: d, graph: g, snapshot: wire, today: today, clock: time.Now, health: SessionHealth{SessionWritable, HealthOK, StageNone}}
	if e = s.reconcileDayLocked(today); e != nil {
		d.root.Close()
		return nil, e
	}
	return s, nil
}

// AdvanceTo applies the natural-day boundary and its empty classification
// history in one persisted transaction. A failed write publishes neither the
// candidate task facts nor its audit nor the new today value.
func (s *Service) AdvanceTo(today core.LocalDate) error { return s.AdvanceToOutcome(today).Err }
func (s *Service) AdvanceToOutcome(today core.LocalDate) AdvanceOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := newAttempt()
	var err error
	if !today.Valid() {
		// Preserve the original pure-parameter error priority, but report existing
		// health (including the legacy blocked guard) without any disk scan.
		if s.disk.blocked {
			s.invalidateLocked(HealthBlocked, StageSessionGate)
		}
		a.stage, err = StageParameter, core.ErrInvalidInput
	} else {
		err = s.reconcileDayLocked(today, &a)
	}
	o := AdvanceOutcome{Outcome: s.outcomeLocked(a, err)}
	if o.Err == nil {
		o.Receipt = &AdvanceReceipt{s.disk.revision, s.disk.hash, s.today}
	}
	return o
}
func (s *Service) reconcileDayLocked(today core.LocalDate, a ...*attempt) error {
	if s.aggregate != nil {
		return s.reconcileOrdinaryDayLocked(today, a...)
	}
	if err := s.gateLocked(); err != nil {
		setAttempt(a, WriteNone, StageSessionGate)
		return err
	}
	setAttempt(a, WriteNone, StageClock)
	// Probe at the existing finite frontier, without allocating a mutation
	// clock. The probe is never published; real changes are replayed below.
	frontier, e := s.persistedFrontier()
	if e != nil {
		return e
	}
	settle := func(now core.Instant) (canonical.Value, core.TaskGraph, error) {
		setAttempt(a, WriteNone, StageDomain)
		k, err := core.NewTaskKernel(s.graph)
		if err != nil {
			return canonical.Value{}, core.TaskGraph{}, err
		}
		if err = k.SettleTaskDays(today, now); err != nil {
			return canonical.Value{}, core.TaskGraph{}, err
		}
		setAttempt(a, WriteNone, StageCandidate)
		captured, err := CaptureEmptyHistory(s.snapshot, s.graph, k.Snapshot())
		if err != nil {
			return canonical.Value{}, core.TaskGraph{}, err
		}
		return normalizeSnapshot(captured)
	}
	setAttempt(a, WriteNone, StageCandidate)
	wire, _, e := settle(frontier)
	if e != nil {
		return e
	}
	if historyEqual(s.snapshot, wire) {
		if _, e = s.verifyPrefixLocked(a...); e != nil {
			return e
		}
	} else {
		setAttempt(a, WriteNone, StageClock)
		now, err := s.nextInstant()
		if err != nil {
			return err
		}
		setAttempt(a, WriteNone, StageCandidate)
		wire, graph, err := settle(now)
		if err != nil {
			return err
		}
		if err = s.commitLocked(wire, a...); err != nil {
			return err
		}
		s.graph, s.snapshot = graph, wire
	}
	s.today = today
	return nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.lineage = nil
	s.health.Session = SessionClosed
	return s.disk.close()
}
func (s *Service) Today() core.LocalDate { s.mu.RLock(); defer s.mu.RUnlock(); return s.today }
func (s *Service) Snapshot() core.TaskGraph {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.graph.Clone()
}
func (s *Service) transact(fn func(*core.TaskKernel, core.Instant) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.gateLocked(); err != nil {
		return err
	}
	k, e := core.NewTaskKernel(s.graph)
	if e != nil {
		return e
	}
	now, e := s.nextInstant()
	if e != nil {
		return e
	}
	if e = fn(k, now); e != nil {
		return e
	}
	captured, e := CaptureEmptyHistory(s.snapshot, s.graph, k.Snapshot())
	if e != nil {
		return e
	}
	wire, g, e := normalizeSnapshot(captured)
	if e != nil {
		return e
	}
	if e = s.commitLocked(wire); e != nil {
		return e
	}
	s.graph = g
	s.snapshot = wire
	return nil
}
func (s *Service) nextInstant() (core.Instant, error) {
	wall := s.clock()
	ref, e := core.InstantFromSeconds(float64(wall.Unix()-978307200) + float64(wall.Nanosecond())/1e9)
	if e != nil {
		return 0, e
	}
	clocks, err := s.allPersistedClocks()
	if err != nil {
		return 0, err
	}
	return core.NextMutationInstant(ref, clocks...)
}

// persistedFrontier does not advance time or sample the wall clock. Zero is a
// valid probe for an empty graph, where settlement cannot create facts.
func (s *Service) persistedFrontier() (core.Instant, error) {
	clocks, err := s.allPersistedClocks()
	if err != nil {
		return 0, err
	}
	var frontier core.Instant
	for i, clock := range clocks {
		if !clock.Finite() {
			return 0, core.ErrInvalidClock
		}
		if i == 0 || clock.After(frontier) {
			frontier = clock
		}
	}
	return frontier, nil
}

func (s *Service) persistedClocks() []core.Instant {
	var clocks []core.Instant
	// Validate all stored clocks first, then include every frontier-bearing clock.
	for _, d := range s.graph.Days {
		clocks = append(clocks, d.CreatedAt, d.UpdatedAt)
		if d.LockedAt != nil {
			clocks = append(clocks, *d.LockedAt)
		}
	}
	for _, c := range s.graph.Chains {
		clocks = append(clocks, c.CreatedAt, c.UpdatedAt)
		for _, n := range c.NoteEntries {
			clocks = append(clocks, n.CreatedAt, n.UpdatedAt)
		}
	}
	for _, d := range s.graph.Definitions {
		clocks = append(clocks, d.CreatedAt, d.ContentUpdatedAt)
		if d.SupersededAt != nil {
			clocks = append(clocks, *d.SupersededAt)
		}
	}
	for _, t := range s.graph.Traces {
		clocks = append(clocks, t.CreatedAt, t.ContentUpdatedAt)
		for _, n := range t.NoteEntries {
			clocks = append(clocks, n.CreatedAt, n.UpdatedAt)
		}
	}
	for _, c := range s.graph.Subtasks {
		clocks = append(clocks, c.CreatedAt, c.UpdatedAt)
	}

	return clocks
}
func newIDs(n int) ([]string, error) {
	out := make([]string, n)
	for i := range out {
		id, e := core.NewIdentity()
		if e != nil {
			return nil, e
		}
		out[i] = id
	}
	return out, nil
}
func (s *Service) AddPool(title string) error  { return s.add(title, false, nil) }
func (s *Service) AddToday(title string) error { return s.add(title, true, nil) }

// AddOnDate creates and schedules a task in one persisted aggregate revision.
// Past, invalid and locked dates are rejected without publishing a pool task.
func (s *Service) AddOnDate(title string, date core.LocalDate) error {
	return s.add(title, true, &date)
}
func (s *Service) add(title string, schedule bool, date *core.LocalDate) error {
	if s.ordinaryEnabled() {
		return s.addOrdinary(title, schedule, date)
	}
	return s.transact(func(k *core.TaskKernel, now core.Instant) error {
		ids, e := newIDs(4)
		if e != nil {
			return e
		}
		if e = k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(ids[0]), DefinitionID: core.TaskDefinitionID(ids[1]), Title: title, Now: now}); e != nil {
			return e
		}
		if !schedule {
			return nil
		}
		// Resolve today's shortcut only under transact's lock, so AdvanceTo cannot
		// interleave between choosing the target and validating against today.
		target := s.today
		if date != nil {
			target = *date
		}
		return k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(ids[0]), TraceID: core.DayTraceID(ids[2]), DayID: core.DayID(ids[3]), Date: target, Today: s.today, Now: now})
	})
}
func (s *Service) Schedule(chain core.TaskChainID, date core.LocalDate) error {
	if s.ordinaryEnabled() {
		return s.scheduleOrdinary(chain, date)
	}
	return s.transact(func(k *core.TaskKernel, now core.Instant) error {
		ids, e := newIDs(2)
		if e != nil {
			return e
		}
		subtaskIDs := map[core.PlannedSubtaskID]core.SubtaskID{}
		for _, d := range k.Snapshot().Definitions {
			if d.ChainID == chain && d.SupersededAt == nil {
				for _, p := range d.PlannedSubtasks {
					child, err := newIDs(1)
					if err != nil {
						return err
					}
					subtaskIDs[p.ID] = core.SubtaskID(child[0])
				}
			}
		}
		return k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: date, Today: s.today, Now: now, SubtaskIDs: subtaskIDs})
	})
}
func (s *Service) SetCompleted(trace core.DayTraceID, completed bool) error {
	if s.ordinaryEnabled() {
		return s.completeOrdinary(trace, completed)
	}
	return s.transact(func(k *core.TaskKernel, now core.Instant) error {
		if completed {
			return k.MarkCompleted(trace, s.today, now)
		}
		return k.UndoCompleted(trace, s.today, now)
	})
}
