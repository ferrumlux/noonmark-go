// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Synthetic source-derived states, NOT Swift captures. Operations use the
// existing fixed-graph command model; inserted captures follow TC:4022-4056.
func aggregateFixture(t testing.TB) AggregateState {
	t.Helper()
	f := newFixture(t, 2)
	p, _, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程", ColorHex: "raw color"}, instant(1))
	id := p.sealed.data.PlannedCategoryID
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(3))
	f.commit(t, 2, Choice{Kind: ChoiceExisting, CategoryID: id}, instant(4))
	s, err := FromOrdinaryState(f.s, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	// A capture at revision 2 separates records 1 and 3, unlike R4.
	s.Revision = 4
	s.ChangeRecords[1].Revision = 3
	s.ChangeRecords[2].Revision = 4
	s.RelationHistory[0].RemovedRevision = 3
	relation := *s.CurrentByChainID[core.TaskChainID(uuid(2))].Category
	relation.Revision = 4
	s.CurrentByChainID[core.TaskChainID(uuid(2))] = Current{&relation}
	for i := range s.ChangeRecords {
		aggregateRehash(t, &s, i)
	}
	c := s.Categories[id]
	event := TraceClassificationSnapshot{ID: uuid(900), TraceID: core.DayTraceID(uuid(901)), Status: core.TraceCompleted, Category: &HistoricalCategoryValue{id, c.Name, c.ColorHex, c.PresentationApproval}, CapturedAt: instant(2), Revision: 2}
	s.SnapshotEventsByTraceID = map[core.DayTraceID][]TraceClassificationSnapshot{event.TraceID: {event}}
	s.SnapshotsByTraceID = map[core.DayTraceID]TraceClassificationSnapshot{event.TraceID: event}
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("aggregate positive fixture", e)
	}
	return s
}
func aggregateRehash(t testing.TB, s *AggregateState, index int) {
	t.Helper()
	r := &s.ChangeRecords[index]
	b, _ := newBudget(Limits{})
	d, e := aggregateRecordDigest(b, *r)
	if e != nil {
		t.Fatal(e)
	}
	r.IntegrityDigest = d
	s.CommittedReceiptsByInteractionID[r.InteractionID] = Receipt{PlanID: r.PlanID, Revision: r.Revision, Notices: aggregateSlice(r.Notices), ChangeRecordID: r.ID, DecisionID: r.DecisionID, ChangeRecordIntegrityDigest: d}
}
func aggregateCopy(t testing.TB, s AggregateState) AggregateState {
	t.Helper()
	v, e := CloneAggregate(s, Limits{})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func aggregateCategory(s *AggregateState, f func(*Category)) {
	for id, c := range s.Categories {
		f(&c)
		s.Categories[id] = c
		return
	}
}
func aggregateEvent(s *AggregateState, f func(*TraceClassificationSnapshot)) {
	for id, es := range s.SnapshotEventsByTraceID {
		f(&es[0])
		s.SnapshotEventsByTraceID[id] = es
		s.SnapshotsByTraceID[id] = es[len(es)-1]
		return
	}
}
func TestAggregateSourceRestoreAndInterleavedRevision(t *testing.T) {
	if e := ValidateAggregate(EmptyAggregateState(), Limits{}); e != nil {
		t.Fatal(e)
	}
	s := aggregateFixture(t)
	before := aggregateCopy(t, s)
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("validator changed input")
	}
	if e := ValidateApplyIntegrity(contextFixture(t, 2), State{FormatVersion: s.FormatVersion, Revision: s.Revision}); e == nil {
		t.Fatal("R4 limited boundary unexpectedly expanded")
	}
	// Complete legitimate state restores without creation plans or task transcript.
	var id CategoryID
	for k := range s.Categories {
		id = k
	}
	c := s.Categories[id]
	c.Name = "另名"
	c.CanonicalKey, _ = canonical16.CanonicalKey(c.Name)
	until := instant(5)
	c.NameVersions[0].ValidUntil = &until
	c.NameVersions = append(c.NameVersions, NameVersion{uuid(908), c.Name, c.CanonicalKey, c.CanonicalKeyVersion, until, nil})
	c.UpdatedAt = until
	s.Categories[id] = c
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("entity versions/source-state restore", e)
	}
	if s.SnapshotsByTraceID[core.DayTraceID(uuid(901))].Category.Name != "工程" {
		t.Fatal("history rewritten")
	}
	// Multi-event is source legal; the runtime profile must reject it separately.
	old := s.SnapshotEventsByTraceID[core.DayTraceID(uuid(901))][0]
	next := old
	next.ID = uuid(909)
	next.Revision = 5
	next.CapturedAt = instant(6)
	next.Status = core.TraceAbandoned
	s.Revision = 5
	s.SnapshotEventsByTraceID[next.TraceID] = append(s.SnapshotEventsByTraceID[next.TraceID], next)
	s.SnapshotsByTraceID[next.TraceID] = next
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("source multi-event", e)
	}
}
func TestAggregateTamperMatrix(t *testing.T) {
	base := aggregateFixture(t)
	nan := core.Instant(math.Float64bits(math.NaN()))
	tests := map[string]func(*AggregateState){
		"format":         func(s *AggregateState) { s.FormatVersion++ },
		"revision count": func(s *AggregateState) { s.Revision++ },
		"max revision":   func(s *AggregateState) { s.Revision = math.MaxUint64 },
		"category key": func(s *AggregateState) {
			for id, c := range s.Categories {
				delete(s.Categories, id)
				s.Categories[CategoryID(uuid(920))] = c
				break
			}
		},
		"category id syntax":     func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.ID = "bad" }) },
		"empty name":             func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.Name = "" }) },
		"raw nonnormalized name": func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.Name = " 工程 " }) },
		"wrong key":              func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.CanonicalKey = "wrong" }) },
		"unknown key version":    func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.CanonicalKeyVersion = "future" }) },
		"category created NaN":   func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.CreatedAt = nan }) },
		"category backwards":     func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.UpdatedAt = instant(-1) }) },
		"unknown approval": func(s *AggregateState) {
			aggregateCategory(s, func(c *Category) { c.PresentationApproval = "unknown" })
		},
		"unknown lifecycle": func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.Lifecycle = "unknown" }) },
		"missing versions":  func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.NameVersions = nil }) },
		"version id":        func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.NameVersions[0].ID = "bad" }) },
		"version name":      func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.NameVersions[0].Name = "other" }) },
		"version key": func(s *AggregateState) {
			aggregateCategory(s, func(c *Category) { c.NameVersions[0].CanonicalKey = "other" })
		},
		"version start": func(s *AggregateState) {
			aggregateCategory(s, func(c *Category) { c.NameVersions[0].ValidFrom = instant(0) })
		},
		"version finite": func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.NameVersions[0].ValidFrom = nan }) },
		"version current closed": func(s *AggregateState) {
			aggregateCategory(s, func(c *Category) { v := instant(2); c.NameVersions[0].ValidUntil = &v })
		},
		"current chain id": func(s *AggregateState) { s.CurrentByChainID["bad"] = Current{} },
		"current missing category": func(s *AggregateState) {
			s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.CategoryID = CategoryID(uuid(999))
		},
		"current decision":        func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.DecisionID = uuid(999) },
		"current revision":        func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.Revision = 2 },
		"current future revision": func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.Revision = 99 },
		"current time": func(s *AggregateState) {
			s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.UpdatedAt = instant(-1)
		},
		"current nonfinite":            func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.CreatedAt = nan },
		"removal id":                   func(s *AggregateState) { s.RelationHistory[0].ID = "bad" },
		"removal duplicate":            func(s *AggregateState) { s.RelationHistory = append(s.RelationHistory, s.RelationHistory[0]) },
		"removal missing category":     func(s *AggregateState) { s.RelationHistory[0].ItemID = CategoryID(uuid(999)) },
		"removal created revision":     func(s *AggregateState) { s.RelationHistory[0].CreatedRevision = 2 },
		"removal equal revision":       func(s *AggregateState) { s.RelationHistory[0].CreatedRevision = s.RelationHistory[0].RemovedRevision },
		"removal future revision":      func(s *AggregateState) { s.RelationHistory[0].RemovedRevision = 99 },
		"removal origin decision":      func(s *AggregateState) { s.RelationHistory[0].OriginDecisionID = uuid(999) },
		"removal decision":             func(s *AggregateState) { s.RelationHistory[0].RemovedByDecisionID = uuid(999) },
		"removal chain authority":      func(s *AggregateState) { s.RelationHistory[0].ChainID = core.TaskChainID(uuid(2)) },
		"removal time":                 func(s *AggregateState) { s.RelationHistory[0].RemovedAt = instant(-1) },
		"removal nonfinite":            func(s *AggregateState) { s.RelationHistory[0].RemovedAt = nan },
		"record id":                    func(s *AggregateState) { s.ChangeRecords[0].ID = "bad" },
		"record plan id":               func(s *AggregateState) { s.ChangeRecords[0].PlanID = "bad" },
		"record interaction":           func(s *AggregateState) { s.ChangeRecords[0].InteractionID = "bad" },
		"record decision missing":      func(s *AggregateState) { s.ChangeRecords[0].DecisionID = "" },
		"record timestamp":             func(s *AggregateState) { s.ChangeRecords[0].CommittedAt = nan },
		"record future":                func(s *AggregateState) { s.ChangeRecords[0].Revision = 99 },
		"record integrity":             func(s *AggregateState) { s.ChangeRecords[0].IntegrityDigest = strings.Repeat("0", 64) },
		"record malformed plan digest": func(s *AggregateState) { s.ChangeRecords[0].PlanDigest = "a"; aggregateRehash(t, s, 0) },
		"record duplicate id":          func(s *AggregateState) { s.ChangeRecords[1].ID = s.ChangeRecords[0].ID; aggregateRehash(t, s, 1) },
		"record duplicate plan": func(s *AggregateState) {
			s.ChangeRecords[1].PlanID = s.ChangeRecords[0].PlanID
			aggregateRehash(t, s, 1)
		},
		"record duplicate interaction": func(s *AggregateState) {
			s.ChangeRecords[1].InteractionID = s.ChangeRecords[0].InteractionID
			aggregateRehash(t, s, 1)
		},
		"record order": func(s *AggregateState) {
			s.ChangeRecords[0], s.ChangeRecords[1] = s.ChangeRecords[1], s.ChangeRecords[0]
		},
		"record causal after":  func(s *AggregateState) { s.ChangeRecords[2].Changes[0].After.Category = nil; aggregateRehash(t, s, 2) },
		"record causal before": func(s *AggregateState) { s.ChangeRecords[1].Changes[0].Before.Category = nil; aggregateRehash(t, s, 1) },
		"receipt missing":      func(s *AggregateState) { delete(s.CommittedReceiptsByInteractionID, s.ChangeRecords[0].InteractionID) },
		"receipt extra":        func(s *AggregateState) { s.CommittedReceiptsByInteractionID[uuid(999)] = Receipt{} },
		"receipt wrong digest": func(s *AggregateState) {
			id := s.ChangeRecords[0].InteractionID
			r := s.CommittedReceiptsByInteractionID[id]
			r.ChangeRecordIntegrityDigest = "wrong"
			s.CommittedReceiptsByInteractionID[id] = r
		},
		"latest missing": func(s *AggregateState) { s.SnapshotsByTraceID = nil },
		"latest extra": func(s *AggregateState) {
			s.SnapshotsByTraceID[core.DayTraceID(uuid(999))] = TraceClassificationSnapshot{}
		},
		"latest stale": func(s *AggregateState) {
			for id, e := range s.SnapshotsByTraceID {
				e.Revision = 1
				s.SnapshotsByTraceID[id] = e
			}
		},
		"latest color": func(s *AggregateState) {
			for id, e := range s.SnapshotsByTraceID {
				c := *e.Category
				c.ColorHex = "changed"
				e.Category = &c
				s.SnapshotsByTraceID[id] = e
			}
		},
		"event stream empty": func(s *AggregateState) {
			for id := range s.SnapshotEventsByTraceID {
				s.SnapshotEventsByTraceID[id] = nil
				s.Revision--
			}
		},
		"event trace mismatch": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.TraceID = core.DayTraceID(uuid(999)) })
		},
		"event id":              func(s *AggregateState) { aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.ID = "bad" }) },
		"event zero revision":   func(s *AggregateState) { aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Revision = 0 }) },
		"event future revision": func(s *AggregateState) { aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Revision = 99 }) },
		"event invalid status": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Status = "invalid" })
		},
		"event nonfinite": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.CapturedAt = nan })
		},
		"event missing category": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Category.ID = CategoryID(uuid(999)) })
		},
		"event invalid approval": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Category.PresentationApproval = "bad" })
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s := aggregateCopy(t, base)
			mutate(&s)
			before := aggregateCopy(t, s)
			if e := ValidateAggregate(s, Limits{}); e == nil {
				t.Fatal("tamper accepted")
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal("mutated rejected input")
			}
		})
	}
}
func TestAggregateUnsupportedIsNotCorruption(t *testing.T) {
	base := aggregateFixture(t)
	for name, f := range map[string]func(*AggregateState){
		"management lifecycle":  func(s *AggregateState) { aggregateCategory(s, func(c *Category) { c.Lifecycle = "merged" }) },
		"record source":         func(s *AggregateState) { s.ChangeRecords[0].Source = "automaticAI" },
		"management change":     func(s *AggregateState) { s.ChangeRecords[0].Changes[0].Kind = "rename" },
		"label preview":         func(s *AggregateState) { s.ChangeRecords[0].Changes[0].After.Labels = []AggregatePlanItem{{}} },
		"current source":        func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.Source = "inherited" },
		"removal origin source": func(s *AggregateState) { s.RelationHistory[0].OriginSource = "inherited" },
		"removal source":        func(s *AggregateState) { s.RelationHistory[0].RemovedBySource = "deterministicDomainAction" },
		"label history":         func(s *AggregateState) { s.RelationHistory[0].Kind = "label" },
		"event labels": func(s *AggregateState) {
			aggregateEvent(s, func(e *TraceClassificationSnapshot) { e.Labels = []HistoricalLabelValue{{ID: uuid(999)}} })
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := aggregateCopy(t, base)
			f(&s)
			if e := ValidateAggregate(s, Limits{}); !errors.Is(e, ErrUnsupported) {
				t.Fatal(e)
			}
		})
	}
}
