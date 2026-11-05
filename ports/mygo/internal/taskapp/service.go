// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"sync"
	"time"
)

type Service struct {
	mu     sync.RWMutex
	disk   *diskStore
	graph  core.TaskGraph
	today  core.LocalDate
	clock  func() time.Time
	closed bool
}

func Create(path string, today core.LocalDate) (*Service, error) {
	if !today.Valid() {
		return nil, core.ErrInvalidInput
	}
	d, e := createStore(path)
	if e != nil {
		return nil, e
	}
	s := &Service{disk: d, today: today, clock: time.Now}
	normalized, e := normalizeGraph(s.graph)
	if e != nil {
		d.root.Close()
		return nil, e
	}
	s.graph = normalized
	if e = d.commit(s.graph); e != nil {
		d.root.Close()
		return nil, e
	}
	return s, nil
}
func Open(path string, today core.LocalDate) (*Service, error) {
	if !today.Valid() {
		return nil, core.ErrInvalidInput
	}
	d, g, e := openStore(path)
	if e != nil {
		return nil, e
	}
	s := &Service{disk: d, graph: g, today: today, clock: time.Now}
	// Empty classification is the only supported runtime scope. Closing earlier
	// task days therefore cannot omit any classification history facts.
	k, e := core.NewTaskKernel(g)
	if e != nil {
		d.root.Close()
		return nil, e
	}
	now, e := s.nextInstant()
	if e != nil {
		d.root.Close()
		return nil, e
	}
	if e = k.SettleTaskDays(today, now); e != nil {
		d.root.Close()
		return nil, e
	}
	settled, e := normalizeGraph(k.Snapshot())
	if e != nil {
		d.root.Close()
		return nil, e
	}
	if !reflect.DeepEqual(g, settled) {
		if e = d.commit(settled); e != nil {
			d.root.Close()
			return nil, e
		}
		s.graph = settled
	}
	return s, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.disk.root.Close()
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
	if s.closed {
		return ErrSession
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
	g, e := normalizeGraph(k.Snapshot())
	if e != nil {
		return e
	}
	if e = s.disk.commit(g); e != nil {
		return e
	}
	s.graph = g
	return nil
}
func (s *Service) nextInstant() (core.Instant, error) {
	wall := s.clock()
	ref, e := core.InstantFromSeconds(float64(wall.Unix()-978307200) + float64(wall.Nanosecond())/1e9)
	if e != nil {
		return 0, e
	}
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
	return core.NextMutationInstant(ref, clocks...)
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
func (s *Service) AddPool(title string) error  { return s.add(title, false) }
func (s *Service) AddToday(title string) error { return s.add(title, true) }
func (s *Service) add(title string, schedule bool) error {
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
		return k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(ids[0]), TraceID: core.DayTraceID(ids[2]), DayID: core.DayID(ids[3]), Date: s.today, Today: s.today, Now: now})
	})
}
func (s *Service) Schedule(chain core.TaskChainID, date core.LocalDate) error {
	return s.transact(func(k *core.TaskKernel, now core.Instant) error {
		ids, e := newIDs(2)
		if e != nil {
			return e
		}
		return k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: date, Today: s.today, Now: now})
	})
}
func (s *Service) SetCompleted(trace core.DayTraceID, completed bool) error {
	return s.transact(func(k *core.TaskKernel, now core.Instant) error {
		if completed {
			return k.MarkCompleted(trace, s.today, now)
		}
		return k.UndoCompleted(trace, s.today, now)
	})
}

func normalizeGraph(g core.TaskGraph) (core.TaskGraph, error) {
	b, e := encodeGraph(g)
	if e != nil {
		return core.TaskGraph{}, e
	}
	return decodeGraph(b)
}
