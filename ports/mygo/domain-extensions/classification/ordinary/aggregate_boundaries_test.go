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

func TestAggregateSourceBoundariesNotGenesisReconstruction(t *testing.T) {
	s := aggregateFixture(t)
	// The source validator checks authority, not a replay reconstruction of all
	// historical clicks. A catalog may be restored even without retained plans.
	catalog := AggregateState{FormatVersion: 1, Categories: s.Categories}
	if e := ValidateAggregate(catalog, Limits{}); e != nil {
		t.Fatal("catalog-only source state", e)
	}
	// Swift permits current empty entries without an introducing record.
	catalog.CurrentByChainID = map[core.TaskChainID]Current{core.TaskChainID(uuid(1)): {}}
	if e := ValidateAggregate(catalog, Limits{}); e != nil {
		t.Fatal(e)
	}
	// Source revision count counts advancing records, not all records. Empty
	// change records are Codable/source-valid, but not a supported command edge.
	r := s.ChangeRecords[0]
	r.ID = uuid(700)
	r.PlanID = uuid(701)
	r.InteractionID = uuid(702)
	r.Revision = 0
	r.Changes = nil
	r.Notices = nil
	catalog.ChangeRecords = []AggregateChangeRecord{r}
	catalog.CommittedReceiptsByInteractionID = map[UUID]Receipt{}
	aggregateRehash(t, &catalog, 0)
	if e := ValidateAggregate(catalog, Limits{}); e != nil {
		t.Fatal("nonadvancing record", e)
	}
	// No invented global event revision uniqueness or cross-stream clock order.
	event := s.SnapshotEventsByTraceID[core.DayTraceID(uuid(901))][0]
	event.ID = uuid(703)
	event.TraceID = core.DayTraceID(uuid(704))
	event.Revision = 2
	event.CapturedAt = instant(-100)
	s.Revision++
	s.SnapshotEventsByTraceID[event.TraceID] = []TraceClassificationSnapshot{event}
	s.SnapshotsByTraceID[event.TraceID] = event
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("source is not runtime forward-clock gate", e)
	}
}
func TestAggregateCanonicalHistoricalOwnership(t *testing.T) {
	s := aggregateFixture(t)
	// Another category can have another current key, but cannot claim an old
	// capture's key via any name version or its current identity.
	var old Category
	for _, c := range s.Categories {
		old = c
	}
	second := old
	second.ID = CategoryID(uuid(800))
	second.Name = "其他"
	second.CanonicalKey, _ = canonical16.CanonicalKey(second.Name)
	second.NameVersions = []NameVersion{{uuid(801), second.Name, second.CanonicalKey, second.CanonicalKeyVersion, second.CreatedAt, nil}}
	s.Categories[second.ID] = second
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal(e)
	}
	aggregateEvent(&s, func(e *TraceClassificationSnapshot) { e.Category.Name = "其他" })
	if e := ValidateAggregate(s, Limits{}); !errors.Is(e, ErrInvalid) {
		t.Fatal("historical name ownership", e)
	}
	s = aggregateFixture(t)
	var id CategoryID
	for key := range s.Categories {
		id = key
	}
	// Historical strings are values, not current-identity Codable entities. Do
	// not trim historical names or rewrite their color/approval from the catalog.
	aggregateEvent(&s, func(e *TraceClassificationSnapshot) {
		e.Category.Name = " 工程 "
		e.Category.ColorHex = "old raw"
		e.Category.PresentationApproval = "pendingAIReview"
	})
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("historical raw value", id, e)
	}
}
func TestAggregateNameVersionGuards(t *testing.T) {
	s := aggregateFixture(t)
	var id CategoryID
	for k := range s.Categories {
		id = k
	}
	c := s.Categories[id]
	end := instant(10)
	c.NameVersions[0].ValidUntil = &end
	c.NameVersions = append(c.NameVersions, NameVersion{uuid(805), "other", "other", c.CanonicalKeyVersion, end, nil})
	c.Name = "other"
	c.CanonicalKey = "other"
	c.UpdatedAt = end
	s.Categories[id] = c
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal(e)
	}
	for name, mut := range map[string]func(*Category){
		"duplicate version":    func(c *Category) { c.NameVersions[1].ID = c.NameVersions[0].ID },
		"backwards interval":   func(c *Category) { v := instant(0); c.NameVersions[0].ValidUntil = &v },
		"nonfinite end":        func(c *Category) { v := core.Instant(math.Float64bits(math.Inf(1))); c.NameVersions[0].ValidUntil = &v },
		"gap":                  func(c *Category) { c.NameVersions[1].ValidFrom = instant(11) },
		"open prior":           func(c *Category) { c.NameVersions[0].ValidUntil = nil },
		"latest after updated": func(c *Category) { c.UpdatedAt = instant(9) },
	} {
		t.Run(name, func(t *testing.T) {
			v := aggregateCopy(t, s)
			aggregateCategory(&v, mut)
			if e := ValidateAggregate(v, Limits{}); e == nil {
				t.Fatal("accepted")
			}
		})
	}
}
func TestAggregateStreamAndOrderGuards(t *testing.T) {
	base := aggregateFixture(t)
	trace := core.DayTraceID(uuid(901))
	for name, mut := range map[string]func(*AggregateState){
		"duplicate event same stream": func(s *AggregateState) {
			e := s.SnapshotEventsByTraceID[trace][0]
			e.Revision = 5
			s.Revision++
			s.SnapshotEventsByTraceID[trace] = append(s.SnapshotEventsByTraceID[trace], e)
			s.SnapshotsByTraceID[trace] = e
		},
		"duplicate event across streams": func(s *AggregateState) {
			e := s.SnapshotEventsByTraceID[trace][0]
			e.TraceID = core.DayTraceID(uuid(902))
			s.Revision++
			s.SnapshotEventsByTraceID[e.TraceID] = []TraceClassificationSnapshot{e}
			s.SnapshotsByTraceID[e.TraceID] = e
		},
		"nonincreasing stream": func(s *AggregateState) {
			e := s.SnapshotEventsByTraceID[trace][0]
			e.ID = uuid(903)
			s.Revision++
			s.SnapshotEventsByTraceID[trace] = append(s.SnapshotEventsByTraceID[trace], e)
			s.SnapshotsByTraceID[trace] = e
		},
		"removal sort": func(s *AggregateState) {
			h := s.RelationHistory[0]
			h.ID = uuid(1)
			s.RelationHistory = append(s.RelationHistory, h)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := aggregateCopy(t, base)
			mut(&s)
			if e := ValidateAggregate(s, Limits{}); e == nil {
				t.Fatal("accepted")
			}
		})
	}
	// Same revision records are not prohibited by the source order contract,
	// which orders them by timestamp then UUID. Fact count is checked separately.
	a, b := base.ChangeRecords[0], base.ChangeRecords[1]
	b.Revision = a.Revision
	b.CommittedAt = a.CommittedAt
	b.ID = uuid(1)
	if !aggregateRecordLess(b, a) {
		t.Fatal("record tie-break")
	}
	h, j := base.RelationHistory[0], base.RelationHistory[0]
	j.ID = uuid(1)
	if !aggregateHistoryLess(j, h) {
		t.Fatal("history tie-break")
	}
}
func TestAggregateDigestOptionalAliasAndLegacyAgreement(t *testing.T) {
	s := aggregateFixture(t)
	r := s.ChangeRecords[0]
	b, _ := newBudget(Limits{})
	legacy := newFixture(t, 2)
	legacy.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程", ColorHex: "raw color"}, instant(1))
	old, e := recordPayload(b, legacy.s.ChangeRecords[0])
	if e != nil {
		t.Fatal(e)
	}
	b, _ = newBudget(Limits{})
	got, e := aggregateRecordPayload(b, r)
	if e != nil || string(old) != string(got) {
		t.Fatal("changed existing digest", e)
	}
	r.Notices = []Notice{{Kind: "existingItemReused", ItemKind: "category", InputName: "old", CurrentName: "工程", MatchedHistoricalAlias: true}}
	r.Changes[0].After.Category.Resolution = "historicalAlias"
	s.ChangeRecords[0] = r
	aggregateRehash(t, &s, 0)
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("source historical alias notice", e)
	}
	b, _ = newBudget(Limits{})
	got, e = aggregateRecordPayload(b, s.ChangeRecords[0])
	if e != nil || !strings.Contains(string(got), `"matchedHistoricalAlias":true`) {
		t.Fatal(e)
	}
	// Optional item ID must be omitted, not encoded as an empty string.
	r.Changes[0].After.Category.ID = nil
	b, _ = newBudget(Limits{})
	got, e = aggregateRecordPayload(b, r)
	if e != nil || strings.Contains(string(got), `"id":""`) {
		t.Fatal(e)
	}
}
func TestAggregateReferencesAndNoGraphPointerGuard(t *testing.T) {
	s := aggregateFixture(t)
	g := graphFixture(t, 2)
	k, e := core.NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	day, e := core.ParseLocalDate("2026-11-07")
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(uuid(1)), TraceID: core.DayTraceID(uuid(901)), DayID: core.DayID(uuid(950)), Date: day, Today: day, Now: instant(1)}); e != nil {
		t.Fatal(e)
	}
	g = k.Snapshot()
	if e = core.ValidateTaskGraph(g); e != nil {
		t.Fatal(e)
	}
	if e = ValidateAggregateReferences(s, g, Limits{}); e != nil {
		t.Fatal(e)
	}
	if e = k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(uuid(3)), DefinitionID: core.TaskDefinitionID(uuid(103)), Title: "unrelated", Now: instant(8)}); e != nil {
		t.Fatal(e)
	}
	if e = ValidateAggregateReferences(s, k.Snapshot(), Limits{}); e != nil {
		t.Fatal("unrelated task mutation invalidated state", e)
	}
	for name, mut := range map[string]func(*AggregateState){
		"current": func(s *AggregateState) { s.CurrentByChainID[core.TaskChainID(uuid(999))] = Current{} },
		"removal": func(s *AggregateState) { s.RelationHistory[0].ChainID = core.TaskChainID(uuid(999)) },
		"record":  func(s *AggregateState) { s.ChangeRecords[0].Changes[0].ChainID = core.TaskChainID(uuid(999)) },
		"latest": func(s *AggregateState) {
			s.SnapshotsByTraceID[core.DayTraceID(uuid(999))] = TraceClassificationSnapshot{}
		},
		"stream": func(s *AggregateState) { s.SnapshotEventsByTraceID[core.DayTraceID(uuid(999))] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			v := aggregateCopy(t, s)
			mut(&v)
			if e := ValidateAggregateReferences(v, g, Limits{}); !errors.Is(e, ErrInvalid) {
				t.Fatal(e)
			}
		})
	}
}
func TestAggregateCloneAndClock(t *testing.T) {
	s := aggregateFixture(t)
	v := aggregateCopy(t, s)
	if !reflect.DeepEqual(s, v) {
		t.Fatal("clone changed fields")
	}
	for _, es := range v.SnapshotEventsByTraceID {
		es[0].Category.Name = "mutated"
	}
	for _, e := range v.SnapshotsByTraceID {
		if e.Category.Name == "mutated" {
			t.Fatal("latest aliased event")
		}
	}
	v.CurrentByChainID[core.TaskChainID(uuid(2))].Category.DecisionID = uuid(9)
	v.ChangeRecords[0].Changes[0].After.Category.Name = "mutated"
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("clone polluted source", e)
	}
	empty := EmptyAggregateState()
	copy := aggregateCopy(t, empty)
	if !reflect.DeepEqual(empty, copy) {
		t.Fatal("nil changed")
	}
	clock, have, e := AggregateClock(empty, Limits{})
	if e != nil || have || clock != 0 {
		t.Fatal(clock, have, e)
	}
	clock, have, e = AggregateClock(s, Limits{})
	if e != nil || !have || !clock.Equal(instant(4)) {
		t.Fatal(clock, have, e)
	}
	for name, f := range map[string]func(*AggregateState, core.Instant){
		"category create": func(s *AggregateState, v core.Instant) { aggregateCategory(s, func(c *Category) { c.CreatedAt = v }) },
		"category update": func(s *AggregateState, v core.Instant) { aggregateCategory(s, func(c *Category) { c.UpdatedAt = v }) },
		"version start": func(s *AggregateState, v core.Instant) {
			aggregateCategory(s, func(c *Category) { c.NameVersions[0].ValidFrom = v })
		},
		"version end": func(s *AggregateState, v core.Instant) {
			aggregateCategory(s, func(c *Category) { c.NameVersions[0].ValidUntil = &v })
		},
		"current create": func(s *AggregateState, v core.Instant) {
			s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.CreatedAt = v
		},
		"current update": func(s *AggregateState, v core.Instant) {
			s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.UpdatedAt = v
		},
		"removal create": func(s *AggregateState, v core.Instant) { s.RelationHistory[0].CreatedAt = v },
		"removal remove": func(s *AggregateState, v core.Instant) { s.RelationHistory[0].RemovedAt = v },
		"record":         func(s *AggregateState, v core.Instant) { s.ChangeRecords[0].CommittedAt = v },
		"latest": func(s *AggregateState, v core.Instant) {
			for id, e := range s.SnapshotsByTraceID {
				e.CapturedAt = v
				s.SnapshotsByTraceID[id] = e
			}
		},
		"stream": func(s *AggregateState, v core.Instant) {
			for _, es := range s.SnapshotEventsByTraceID {
				es[0].CapturedAt = v
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := aggregateCopy(t, s)
			f(&v, instant(100))
			got, have, e := AggregateClock(v, Limits{})
			if e != nil || !have || !got.Equal(instant(100)) {
				t.Fatal(got, have, e)
			}
			f(&v, core.Instant(math.Float64bits(math.NaN())))
			if _, _, e = AggregateClock(v, Limits{}); !errors.Is(e, ErrClock) {
				t.Fatal(e)
			}
		})
	}
}

func TestAggregateRawAuditStringsAreNotUUIDNormalized(t *testing.T) {
	s := aggregateFixture(t)
	raw := CategoryID(strings.ToLower(string(s.RelationHistory[0].ItemID)))
	if raw == s.RelationHistory[0].ItemID {
		t.Fatal("fixture lacks hex letters")
	}
	s.RelationHistory[0].ItemID = raw
	rawString := string(raw)
	s.ChangeRecords[0].Changes[0].After.Category.ID = &rawString
	s.ChangeRecords[1].Changes[0].Before.Category.ID = &rawString
	aggregateRehash(t, &s, 0)
	aggregateRehash(t, &s, 1)
	if e := ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("source raw lowercase audit", e)
	}
	// UUID identity lookup is case-insensitive; authority is raw String equality.
	s.RelationHistory[0].ItemID = CategoryID(strings.ToUpper(string(raw)))
	if e := ValidateAggregate(s, Limits{}); !errors.Is(e, ErrInvalid) {
		t.Fatal("wrong raw causal identity accepted", e)
	}
}

func TestAggregateOptionalPreviewIDHasThreeDistinctDigests(t *testing.T) {
	base := aggregateFixture(t)
	// Source-state validity does not reconstruct unretained clicks; omit current
	// and removal claims so an absent/empty/raw-string preview claims no authority.
	base.CurrentByChainID = nil
	base.RelationHistory = nil
	digests := map[string]bool{}
	for _, mode := range []string{"absent", "empty", "lowercase"} {
		t.Run(mode, func(t *testing.T) {
			s := aggregateCopy(t, base)
			p := s.ChangeRecords[0].Changes[0].After.Category
			switch mode {
			case "absent":
				p.ID = nil
			case "empty":
				v := ""
				p.ID = &v
			case "lowercase":
				v := strings.ToLower(*p.ID)
				p.ID = &v
			}
			aggregateRehash(t, &s, 0)
			if e := ValidateAggregate(s, Limits{}); e != nil {
				t.Fatal(mode, e)
			}
			d := s.ChangeRecords[0].IntegrityDigest
			if digests[d] {
				t.Fatal("optional ID digest collision")
			}
			digests[d] = true
			b, _ := newBudget(Limits{})
			raw, e := aggregateRecordPayload(b, s.ChangeRecords[0])
			if e != nil {
				t.Fatal(e)
			}
			if mode == "empty" && !strings.Contains(string(raw), `"id":""`) {
				t.Fatal("present empty omitted")
			}
			copied := aggregateCopy(t, s)
			if !reflect.DeepEqual(copied, s) {
				t.Fatal("optional representation changed")
			}
			replacement := "not the original"
			copied.ChangeRecords[0].Changes[0].After.Category.ID = &replacement
			if e := ValidateAggregate(copied, Limits{}); !errors.Is(e, ErrInvalid) {
				t.Fatal("changed raw digest accepted", e)
			}
			if e := ValidateAggregate(s, Limits{}); e != nil {
				t.Fatal("copy changed source", e)
			}
		})
	}
}
func TestAggregateConversionsRetainOwnershipAndMetadata(t *testing.T) {
	f := auditedFixture(t)
	s, e := FromOrdinaryState(f.s, Limits{})
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal(e)
	}
	r, e := RecordFromOrdinary(f.s.ChangeRecords[0], Limits{})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r, s.ChangeRecords[0]) {
		t.Fatal("record conversion differs")
	}
	*r.Changes[0].After.Category.ID = "changed"
	r.Changes[0].After.Category.Name = "changed"
	if e = ValidateAggregate(s, Limits{}); e != nil {
		t.Fatal("record aliases converted state", e)
	}
	if e = ValidateApplyIntegrity(f.ctx, f.s); e != nil {
		t.Fatal("converted state aliases original", e)
	}
	if _, e = FromOrdinaryState(f.s, Limits{MaxBytes: 1}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
	if _, e = RecordFromOrdinary(f.s.ChangeRecords[0], Limits{MaxBytes: 1}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
}
