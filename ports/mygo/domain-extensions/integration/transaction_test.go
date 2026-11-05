// SPDX-License-Identifier: AGPL-3.0-only
// These examples exercise composition; they do not provide durable transactions.
package integration_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func id(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func instant(v float64) core.Instant {
	x, e := core.InstantFromSeconds(v)
	if e != nil {
		panic(e)
	}
	return x
}
func date(v string) core.LocalDate {
	x, e := core.ParseLocalDate(v)
	if e != nil {
		panic(e)
	}
	return x
}
func canonicalizer() classification.NameCanonicalizer {
	return classification.FixtureCanonicalizer{Algorithm: "fixture-only-not-icu", Names: map[string]classification.CanonicalName{"工作": {Display: "工作", Key: "工作"}}}
}
func catalogReferences(s recurrence.State, c classification.Snapshot) error {
	return s.ValidateCatalogReferences(func(id classification.CategoryID) bool { _, ok := c.Items["category:"+string(id)]; return ok }, func(id classification.LabelID) bool { _, ok := c.Items["label:"+string(id)]; return ok })
}

// A caller must stage BOTH stores before publishing. This example deliberately
// gives the last child an invalid catalog target, proving the originals survive.
func TestParentAndOccurrenceClassificationCanBeStagedAtomically(t *testing.T) {
	seq := 0
	ids := func() (string, error) { seq++; return id(seq), nil }
	state, seriesID, err := (recurrence.State{}).Create(recurrence.CreateInput{Title: "合成计划", StartDate: date("2026-07-20"), Schedule: recurrence.Daily(), EndCondition: recurrence.Until(date("2026-07-22")), Today: date("2026-07-20"), Now: instant(100)}, ids)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := classification.New(canonicalizer())
	if err != nil {
		t.Fatal(err)
	}
	category := classification.CategoryID(id(900))
	_, err = engine.Execute(classification.Command{Operation: classification.Create, Item: classification.CategoryRef(category), Name: "工作", Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(901), DecisionID: id(902), At: instant(101)}, state.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	originalState := state.Clone()
	originalCatalog := engine.Snapshot()
	stage := func(failLast bool) (recurrence.State, *classification.Engine, error) {
		candidate, e := state.SetTemplateClassification(recurrence.ClassificationInput{SeriesID: seriesID, CategoryID: &category, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(903), DecisionID: ptr(id(904)), Now: instant(102)}, ids)
		if e != nil {
			return state, engine, e
		}
		staged, e := classification.RestoreSnapshot(canonicalizer(), originalCatalog, candidate.Tasks)
		if e != nil {
			return state, engine, e
		}
		members := candidate.EditableMemberChainIDs(seriesID, date("2026-07-20"))
		for i, chain := range members {
			selected := category
			if failLast && i == len(members)-1 {
				selected = classification.CategoryID(id(999))
			}
			_, e = staged.Execute(classification.Command{Operation: classification.SetCurrent, ChainID: chain, CategoryID: selected, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(910 + i), DecisionID: id(920 + i), ExpectedRevision: staged.Snapshot().Revision, At: instant(102)}, candidate.Tasks)
			if e != nil {
				return state, engine, e
			}
		}
		if e = catalogReferences(candidate, staged.Snapshot()); e != nil {
			return state, engine, e
		}
		return candidate, staged, nil
	}
	rejected, rejectedEngine, err := stage(true)
	if err == nil {
		t.Fatal("missing catalog accepted")
	}
	if !reflect.DeepEqual(originalState, rejected) || !reflect.DeepEqual(originalCatalog, rejectedEngine.Snapshot()) || !reflect.DeepEqual(originalCatalog, engine.Snapshot()) {
		t.Fatal("partial publication")
	}
	committed, committedEngine, err := stage(false)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Series[0].CategoryID == nil || len(committedEngine.Snapshot().Current) != 3 {
		t.Fatal("propagation incomplete")
	}
	if !reflect.DeepEqual(originalState, state) || !reflect.DeepEqual(originalCatalog, engine.Snapshot()) {
		t.Fatal("staging mutated owner")
	}
	if _, err = classification.RestoreSnapshot(canonicalizer(), committedEngine.Snapshot(), committed.Tasks); err != nil {
		t.Fatal(err)
	}
}
func ptr[T any](v T) *T { return &v }

// Catalog hard-delete does not know about recurring-parent references. The
// aggregate validator MUST inspect all revisions, not merely current metadata.
func TestHistoricalParentReferencesBlockAggregateCatalogDeletion(t *testing.T) {
	seq := 0
	ids := func() (string, error) { seq++; return id(seq), nil }
	state, seriesID, err := (recurrence.State{}).Create(recurrence.CreateInput{Title: "历史引用", StartDate: date("2026-07-20"), Schedule: recurrence.Daily(), EndCondition: recurrence.Until(date("2026-07-20")), Today: date("2026-07-20"), Now: instant(100)}, ids)
	if err != nil {
		t.Fatal(err)
	}
	engine, _ := classification.New(canonicalizer())
	category := classification.CategoryID(id(900))
	_, err = engine.Execute(classification.Command{Operation: classification.Create, Item: classification.CategoryRef(category), Name: "工作", Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(901), DecisionID: id(902), At: instant(101)}, state.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.SetTemplateClassification(recurrence.ClassificationInput{SeriesID: seriesID, CategoryID: &category, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(903), Now: instant(102)}, ids)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.SetTemplateClassification(recurrence.ClassificationInput{SeriesID: seriesID, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(904), Now: instant(103)}, ids)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalogReferences(state, engine.Snapshot()); err != nil {
		t.Fatal(err)
	}
	staged, _ := classification.RestoreSnapshot(canonicalizer(), engine.Snapshot(), state.Tasks)
	_, err = staged.Execute(classification.Command{Operation: classification.HardDelete, Item: classification.CategoryRef(category), Source: classification.Source{Kind: classification.UserDirect}, InteractionID: id(905), DecisionID: id(906), ExpectedRevision: staged.Snapshot().Revision, At: instant(104)}, state.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	if catalogReferences(state, staged.Snapshot()) == nil {
		t.Fatal("historical parent reference orphaned")
	}
	if len(engine.Snapshot().Items) != 1 {
		t.Fatal("original catalog changed")
	}
}
