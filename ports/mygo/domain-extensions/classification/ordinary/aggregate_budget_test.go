// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"strings"
	"testing"
)

func TestAggregateBudgetBoundaries(t *testing.T) {
	s := aggregateFixture(t)
	b, e := aggregateEntry(s, Limits{})
	if e != nil {
		t.Fatal(e)
	}
	if e = validateAggregate(b, s); e != nil {
		t.Fatal(e)
	}
	t.Logf("synthetic 1 category, 3 records, 1 removal, 1 event: logical bytes=%d nodes=%d", b.bytes, b.nodes)
	exact := Limits{MaxBytes: int(b.bytes), MaxNodes: int(b.nodes)}
	if e = ValidateAggregate(s, exact); e != nil {
		t.Fatal("exact budget", e)
	}
	for _, l := range []Limits{{MaxBytes: int(b.bytes) - 1}, {MaxNodes: int(b.nodes) - 1}, {MaxDepth: 1}, {MaxBytes: -1}, {MaxDepth: 129}, {MaxNodes: 1_000_001}} {
		if e = ValidateAggregate(s, l); !errors.Is(e, ErrResourceLimit) {
			t.Fatal(l, e)
		}
	}
	// Preflight precedes malformed string/format work, normalization and hashing.
	huge := EmptyAggregateState()
	huge.FormatVersion = -1
	huge.Categories = map[CategoryID]Category{CategoryID(uuid(1)): {Name: strings.Repeat("x", 8192) + "\xff"}}
	for name, call := range map[string]func() error{
		"validate":   func() error { return ValidateAggregate(huge, Limits{MaxBytes: 1024}) },
		"clone":      func() error { _, e := CloneAggregate(huge, Limits{MaxBytes: 1024}); return e },
		"clock":      func() error { _, _, e := AggregateClock(huge, Limits{MaxBytes: 1024}); return e },
		"references": func() error { return ValidateAggregateReferences(huge, core.TaskGraph{}, Limits{MaxBytes: 1024}) },
	} {
		t.Run(name, func(t *testing.T) {
			if e := call(); !errors.Is(e, ErrResourceLimit) {
				t.Fatal(e)
			}
		})
	}
	// A claimed huge revision cannot direct an allocation or replay loop.
	huge = EmptyAggregateState()
	huge.Revision = ^uint64(0)
	if e = ValidateAggregate(huge, Limits{}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestAggregateUnicodeExpansionBudget(t *testing.T) {
	// U+FDFA has a large pinned casefold expansion. Limit must be charged from
	// the raw bytes before allocating its canonical output.
	s := EmptyAggregateState()
	name := strings.Repeat("ﷺ", 1000)
	key, e := canonical16.CanonicalKey(name)
	if e != nil {
		t.Fatal(e)
	}
	c := Category{ID: CategoryID(uuid(1)), Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, PresentationApproval: "userApproved", Lifecycle: "active", NameVersions: []NameVersion{{ID: uuid(2), Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion}}}
	s.Categories = map[CategoryID]Category{c.ID: c}
	if e = ValidateAggregate(s, Limits{MaxBytes: 200_000}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
}
func TestAggregateConcurrentReadOnly(t *testing.T) {
	s := aggregateFixture(t)
	original := aggregateCopy(t, s)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			for j := 0; j < 5; j++ {
				if e := ValidateAggregate(s, Limits{}); e != nil {
					errs <- e
					return
				}
				if _, e := CloneAggregate(s, Limits{}); e != nil {
					errs <- e
					return
				}
				if _, _, e := AggregateClock(s, Limits{}); e != nil {
					errs <- e
					return
				}
			}
			errs <- nil
		}()
	}
	for i := 0; i < 16; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	if !reflect.DeepEqual(s, original) {
		t.Fatal("shared input changed")
	}
}
func BenchmarkAggregateCatalog(b *testing.B) {
	for _, n := range []int{0, 1, 10, 100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := EmptyAggregateState()
			s.Categories = map[CategoryID]Category{}
			for i := 0; i < n; i++ {
				name := fmt.Sprintf("category %d", i)
				c := Category{ID: CategoryID(uuid(i + 1)), Name: name, CanonicalKey: name, CanonicalKeyVersion: canonical16.AlgorithmVersion, PresentationApproval: "userApproved", Lifecycle: "active", NameVersions: []NameVersion{{ID: uuid(i + 1001), Name: name, CanonicalKey: name, CanonicalKeyVersion: canonical16.AlgorithmVersion}}}
				s.Categories[c.ID] = c
			}
			if e := ValidateAggregate(s, Limits{}); e != nil {
				b.Fatal(e)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if e := ValidateAggregate(s, Limits{}); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
func FuzzAggregateRejectsCausalTamper(f *testing.F) {
	for i := uint8(0); i < 8; i++ {
		f.Add(i, "工程")
	}
	f.Fuzz(func(t *testing.T, mode uint8, noise string) {
		if len(noise) > 2048 {
			t.Skip()
		}
		s := aggregateFixture(t)
		switch mode % 8 {
		case 0:
			s.Revision++
		case 1:
			s.ChangeRecords[0].IntegrityDigest = "tamper" + noise
		case 2:
			s.CurrentByChainID[core.TaskChainID(uuid(2))].Category.DecisionID = uuid(999)
		case 3:
			s.RelationHistory[0].OriginDecisionID = uuid(999)
		case 4:
			s.RelationHistory[0].RemovedByDecisionID = uuid(999)
		case 5:
			delete(s.CommittedReceiptsByInteractionID, s.ChangeRecords[0].InteractionID)
		case 6:
			aggregateEvent(&s, func(e *TraceClassificationSnapshot) { e.ID = UUID(noise) + "!" })
		case 7:
			s.SnapshotsByTraceID = nil
		}
		if e := ValidateAggregate(s, Limits{}); e == nil {
			t.Fatal("tamper accepted")
		}
	})
}
func FuzzAggregateNamesAndBudget(f *testing.F) {
	for _, name := range []string{"工程", "E\u0301", "a\n\u0301b", "\u200b", "\xff", "Straße", "ﷺ"} {
		f.Add(name, uint16(0))
	}
	f.Fuzz(func(t *testing.T, name string, size uint16) {
		if len(name) > 4096 {
			t.Skip()
		}
		s := EmptyAggregateState()
		key, e := canonical16.CanonicalKey(name)
		if e != nil {
			return
		}
		c := Category{ID: CategoryID(uuid(1)), Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, PresentationApproval: "userApproved", Lifecycle: "active", NameVersions: []NameVersion{{ID: uuid(2), Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion}}}
		s.Categories = map[CategoryID]Category{c.ID: c}
		before := c
		_ = ValidateAggregate(s, Limits{MaxBytes: int(size) + 1})
		if !reflect.DeepEqual(s.Categories[c.ID], before) {
			t.Fatal("input changed")
		}
	})
}
