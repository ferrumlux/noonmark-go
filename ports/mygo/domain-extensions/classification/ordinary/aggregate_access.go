// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

// CloneAggregate is a bounded defensive copy, not an admission or a write seal.
// Raw strings, instant bits, slice order and nil/empty distinctions are retained.
func CloneAggregate(s AggregateState, l Limits) (AggregateState, error) {
	b, e := aggregateEntry(s, l)
	if e != nil {
		return AggregateState{}, e
	}
	if e = b.scan(s); e != nil {
		return AggregateState{}, e
	}
	out := s
	if s.Categories != nil {
		out.Categories = make(map[CategoryID]Category, len(s.Categories))
		for id, c := range s.Categories {
			c.NameVersions = aggregateSlice(c.NameVersions)
			for i := range c.NameVersions {
				c.NameVersions[i].ValidUntil = clonePointer(c.NameVersions[i].ValidUntil)
			}
			out.Categories[id] = c
		}
	}
	if s.CurrentByChainID != nil {
		out.CurrentByChainID = make(map[core.TaskChainID]Current, len(s.CurrentByChainID))
		for id, c := range s.CurrentByChainID {
			c.Category = clonePointer(c.Category)
			out.CurrentByChainID[id] = c
		}
	}
	out.RelationHistory = aggregateSlice(s.RelationHistory)
	out.ChangeRecords = aggregateSlice(s.ChangeRecords)
	for i, r := range s.ChangeRecords {
		r.Notices = aggregateSlice(r.Notices)
		r.Changes = aggregateSlice(r.Changes)
		for j, c := range r.Changes {
			c.Before = aggregatePreview(c.Before)
			c.After = aggregatePreview(c.After)
			r.Changes[j] = c
		}
		out.ChangeRecords[i] = r
	}
	if s.CommittedReceiptsByInteractionID != nil {
		out.CommittedReceiptsByInteractionID = make(map[UUID]Receipt, len(s.CommittedReceiptsByInteractionID))
		for id, r := range s.CommittedReceiptsByInteractionID {
			r.Notices = aggregateSlice(r.Notices)
			out.CommittedReceiptsByInteractionID[id] = r
		}
	}
	if s.SnapshotsByTraceID != nil {
		out.SnapshotsByTraceID = make(map[core.DayTraceID]TraceClassificationSnapshot, len(s.SnapshotsByTraceID))
		for id, v := range s.SnapshotsByTraceID {
			out.SnapshotsByTraceID[id] = aggregateCloneEvent(v)
		}
	}
	if s.SnapshotEventsByTraceID != nil {
		out.SnapshotEventsByTraceID = make(map[core.DayTraceID][]TraceClassificationSnapshot, len(s.SnapshotEventsByTraceID))
		for id, es := range s.SnapshotEventsByTraceID {
			copy := aggregateSlice(es)
			for i, e := range es {
				copy[i] = aggregateCloneEvent(e)
			}
			out.SnapshotEventsByTraceID[id] = copy
		}
	}
	return out, nil
}
func aggregateSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	copy(out, s)
	return out
}
func aggregatePreview(p AggregateSelectionPreview) AggregateSelectionPreview {
	p.Category = clonePointer(p.Category)
	if p.Category != nil {
		p.Category.ID = clonePointer(p.Category.ID)
	}
	p.Labels = aggregateSlice(p.Labels)
	for i := range p.Labels {
		p.Labels[i].ID = clonePointer(p.Labels[i].ID)
	}
	return p
}
func aggregateCloneEvent(e TraceClassificationSnapshot) TraceClassificationSnapshot {
	e.Category = clonePointer(e.Category)
	e.Labels = aggregateSlice(e.Labels)
	return e
}

// AggregateClock scans every classification mutation timestamp (including both
// latest and streams), after a complete budget preflight. It rejects nonfinite
// time but does not assert integrity. The bool distinguishes an empty frontier
// from zero seconds; callers combine it with the task/supplement frontier.
func AggregateClock(s AggregateState, l Limits) (core.Instant, bool, error) {
	if _, e := aggregateEntry(s, l); e != nil {
		return 0, false, e
	}
	var max core.Instant
	have := false
	take := func(v core.Instant) error {
		if !v.Finite() {
			return ErrClock
		}
		if !have || max.Before(v) {
			max = v
			have = true
		}
		return nil
	}
	for _, c := range s.Categories {
		for _, v := range []core.Instant{c.CreatedAt, c.UpdatedAt} {
			if e := take(v); e != nil {
				return 0, false, e
			}
		}
		for _, v := range c.NameVersions {
			if e := take(v.ValidFrom); e != nil {
				return 0, false, e
			}
			if v.ValidUntil != nil {
				if e := take(*v.ValidUntil); e != nil {
					return 0, false, e
				}
			}
		}
	}
	for _, c := range s.CurrentByChainID {
		if c.Category != nil {
			if e := take(c.Category.CreatedAt); e != nil {
				return 0, false, e
			}
			if e := take(c.Category.UpdatedAt); e != nil {
				return 0, false, e
			}
		}
	}
	for _, h := range s.RelationHistory {
		if e := take(h.CreatedAt); e != nil {
			return 0, false, e
		}
		if e := take(h.RemovedAt); e != nil {
			return 0, false, e
		}
	}
	for _, r := range s.ChangeRecords {
		if e := take(r.CommittedAt); e != nil {
			return 0, false, e
		}
	}
	for _, e := range s.SnapshotsByTraceID {
		if err := take(e.CapturedAt); err != nil {
			return 0, false, err
		}
	}
	for _, es := range s.SnapshotEventsByTraceID {
		for _, e := range es {
			if err := take(e.CapturedAt); err != nil {
				return 0, false, err
			}
		}
	}
	return max, have, nil
}
