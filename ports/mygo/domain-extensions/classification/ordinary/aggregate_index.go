// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

type aggregateAuthority struct {
	revision uint64
	source   Source
	decision UUID
	chain    core.TaskChainID
	item     CategoryID
	removed  bool
}
type aggregateCanonicalPair struct{ key, version string }
type aggregateIndex struct {
	owners                                        map[aggregateCanonicalPair]CategoryID
	records, plans, interactions, history, events map[UUID]bool
	authority                                     map[aggregateAuthority]bool
}

func newAggregateIndex() *aggregateIndex {
	return &aggregateIndex{map[aggregateCanonicalPair]CategoryID{}, map[UUID]bool{}, map[UUID]bool{}, map[UUID]bool{}, map[UUID]bool{}, map[UUID]bool{}, map[aggregateAuthority]bool{}}
}
func (x *aggregateIndex) claim(b *budget, key, version string, id CategoryID) error {
	key, e := b.nfc(key)
	if e != nil {
		return e
	}
	p := aggregateCanonicalPair{key, version}
	if old, ok := x.owners[p]; ok && old != id {
		return invalidAggregate("canonical name ownership")
	}
	x.owners[p] = id
	return nil
}
func aggregateRecordLess(a, b AggregateChangeRecord) bool {
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	if !a.CommittedAt.Equal(b.CommittedAt) {
		return a.CommittedAt.Before(b.CommittedAt)
	}
	return a.ID < b.ID
}
func aggregateHistoryLess(a, b RelationHistory) bool {
	if a.RemovedRevision != b.RemovedRevision {
		return a.RemovedRevision < b.RemovedRevision
	}
	if !a.RemovedAt.Equal(b.RemovedAt) {
		return a.RemovedAt.Before(b.RemovedAt)
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.ChainID != b.ChainID {
		return a.ChainID < b.ChainID
	}
	if a.ItemID != b.ItemID {
		return a.ItemID < b.ItemID
	}
	return a.ID < b.ID
}

// ValidateAggregateReferences checks the classification-to-task edges of
// NoonmarkSnapshot. The caller separately validates the complete task graph;
// this routine neither clones it nor applies R4's quadratic context gate.
func ValidateAggregateReferences(s AggregateState, g core.TaskGraph, l Limits) error {
	b, e := aggregateEntry(s, l)
	if e != nil {
		return e
	}
	if e = b.scan(g); e != nil {
		return e
	}
	if e = b.scan(g.Chains, g.Traces); e != nil {
		return e
	}
	chains := make(map[core.TaskChainID]bool, len(g.Chains))
	traces := make(map[core.DayTraceID]bool, len(g.Traces))
	for _, c := range g.Chains {
		chains[c.ID] = true
	}
	for _, t := range g.Traces {
		traces[t.ID] = true
	}
	for id := range s.CurrentByChainID {
		if !chains[id] {
			return invalidAggregate("current chain reference")
		}
	}
	for _, h := range s.RelationHistory {
		if !chains[h.ChainID] {
			return invalidAggregate("removal chain reference")
		}
	}
	for id := range s.SnapshotsByTraceID {
		if !traces[id] {
			return invalidAggregate("latest trace reference")
		}
	}
	for id := range s.SnapshotEventsByTraceID {
		if !traces[id] {
			return invalidAggregate("event trace reference")
		}
	}
	for _, r := range s.ChangeRecords {
		for _, c := range r.Changes {
			if c.Kind != "setCurrent" {
				return ErrUnsupported
			}
			if !chains[c.ChainID] {
				return invalidAggregate("record chain reference")
			}
		}
	}
	return nil
}
