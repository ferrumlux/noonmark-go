// SPDX-License-Identifier: AGPL-3.0-only
package classification

import (
	"encoding/json"
	"errors"
	"fmt"
	core "github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"testing"
)

func id(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", i) }
func at(n float64) core.Instant {
	v, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return v
}
func canon() FixtureCanonicalizer {
	return FixtureCanonicalizer{"unicode-fixture-nfkc-casefold-whitespace-v1", map[string]CanonicalName{"ＡＢＣ": {"ＡＢＣ", "abc"}, "abc": {"abc", "abc"}, "Straße": {"Straße", "strasse"}, "STRASSE": {"STRASSE", "strasse"}, "foo\u00ADbar": {"foo\u00ADbar", "foobar"}, "foobar": {"foobar", "foobar"}, "Old": {"Old", "old"}, "New": {"New", "new"}, "Other": {"Other", "other"}, "  Old  ": {"Old", "old"}}}
}
func graph() core.TaskGraph {
	d, _ := core.ParseLocalDate("2026-11-05")
	done := at(1)
	return core.TaskGraph{Days: []core.Day{{ID: core.DayID(id(1)), Date: d, CreatedAt: at(0), UpdatedAt: at(1)}}, Chains: []core.TaskChain{{ID: core.TaskChainID(id(2)), State: core.ChainActive, CreatedAt: at(0), UpdatedAt: at(1)}, {ID: core.TaskChainID(id(5)), State: core.ChainActive, CreatedAt: at(0), UpdatedAt: at(1)}}, Definitions: []core.TaskDefinition{{ID: core.TaskDefinitionID(id(3)), ChainID: core.TaskChainID(id(2)), Sequence: 1, Title: "synthetic", CreatedAt: at(0), ContentUpdatedAt: at(0)}, {ID: core.TaskDefinitionID(id(6)), ChainID: core.TaskChainID(id(5)), Sequence: 1, Title: "synthetic", CreatedAt: at(0), ContentUpdatedAt: at(0)}}, Traces: []core.DayTrace{{ID: core.DayTraceID(id(4)), ChainID: core.TaskChainID(id(2)), DefinitionID: core.TaskDefinitionID(id(3)), Date: d, Status: core.TraceCompleted, Priority: 1, CreatedAt: at(0), ContentUpdatedAt: at(1), CompletedAt: &done}}}
}

type fixture struct {
	t   *testing.T
	e   *Engine
	g   core.TaskGraph
	seq int
}

func setup(t *testing.T) *fixture {
	e, err := New(canon())
	if err != nil {
		t.Fatal(err)
	}
	g := graph()
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatalf("graph %#v", err)
	}
	return &fixture{t, e, g, 100}
}
func (f *fixture) command(c Command) Command {
	f.seq++
	c.InteractionID = id(f.seq)
	c.DecisionID = id(f.seq + 10000)
	c.Source = Source{Kind: UserDirect}
	c.At = at(float64(f.seq))
	c.ExpectedRevision = f.e.Snapshot().Revision
	return c
}
func (f *fixture) run(c Command) Receipt {
	f.t.Helper()
	r, e := f.e.Execute(f.command(c), f.g)
	if e != nil {
		f.t.Fatal(e)
	}
	return r
}
func (f *fixture) reject(c Command, want error) {
	f.t.Helper()
	before := f.e.Snapshot()
	_, e := f.e.Execute(c, f.g)
	if !errors.Is(e, want) {
		f.t.Fatalf("got %v want %v", e, want)
	}
	if !reflect.DeepEqual(before, f.e.Snapshot()) {
		f.t.Fatal("failure published state")
	}
}

var cat = CategoryRef(CategoryID(id(20)))
var lab = LabelRef(LabelID(id(21)))
var other = LabelRef(LabelID(id(22)))
var chain = core.TaskChainID(id(2))

// Upstream: testCanonicalNamesReuseEquivalentCaseAndWhitespaceVariantsWithinEachType.
func TestCanonicalFixturesAndStrictUnsupported(t *testing.T) {
	f := setup(t)
	f.run(Command{Operation: Create, Item: cat, Name: "ＡＢＣ"})
	if r, e := f.e.ResolveName(Category, "abc"); e != nil || r != cat {
		t.Fatal(r, e)
	}
	f.run(Command{Operation: Create, Item: lab, Name: "Straße"})
	if r, e := f.e.ResolveName(Label, "STRASSE"); e != nil || r != lab {
		t.Fatal(r, e)
	}
	a, b, e := canon().Canonicalize("foo\u00ADbar")
	if e != nil || a == "" || b != "foobar" {
		t.Fatal(a, b, e)
	}
	f.reject(f.command(Command{Operation: Create, Item: other, Name: "unknown Unicode 🦊"}), ErrUnsupported)
	f.reject(f.command(Command{Operation: Create, Item: other, Name: "STRASSE"}), ErrConflict)
}

// Upstream: testRenamingCategoryKeepsIdentityHistoryAndReservesTheOldName,
// testExactRenameNoOpKeepsRevisionAndBackdatedRenameFailsClosed.
func TestRenameFrozenHistoryAliasAndNoOp(t *testing.T) {
	f := setup(t)
	f.run(Command{Operation: Create, Item: cat, Name: "Old"})
	f.run(Command{Operation: SetCurrent, ChainID: chain, CategoryID: CategoryID(cat.ID)})
	f.run(Command{Operation: Capture, ChainID: chain, TraceID: core.DayTraceID(id(4)), Status: core.TraceCompleted})
	before := f.e.Snapshot().History
	f.run(Command{Operation: Rename, Item: cat, Name: "New"})
	s := f.e.Snapshot()
	v := s.Items[cat.key()]
	if len(v.NameVersions) != 2 || v.NameVersions[0].ValidUntil == nil || !v.NameVersions[0].ValidUntil.Equal(v.NameVersions[1].ValidFrom) || !reflect.DeepEqual(before, s.History) {
		t.Fatal("version/history drift")
	}
	if r, e := f.e.ResolveName(Category, "  Old  "); e != nil || r != cat {
		t.Fatal(r, e)
	}
	r := f.run(Command{Operation: Rename, Item: cat, Name: "New"})
	if r.Changed || r.Revision != s.Revision {
		t.Fatal(r)
	}
	c := f.command(Command{Operation: Rename, Item: cat, Name: "Other"})
	c.At = at(1)
	f.reject(c, ErrConflict)
}

// Upstream: testCurrentRelationsPreserveOriginAndAppendRemovalHistory,
// testLabelMergeDeduplicatesExistingTargetWithoutReplacingItsOrigin.
func TestRelationsMergeDedupAndOrigins(t *testing.T) {
	f := setup(t)
	f.run(Command{Operation: Create, Item: lab, Name: "Old"})
	f.run(Command{Operation: Create, Item: other, Name: "New"})
	f.run(Command{Operation: SetCurrent, ChainID: chain, LabelIDs: []LabelID{LabelID(lab.ID), LabelID(other.ID), LabelID(lab.ID)}})
	prior := f.e.Snapshot().Current[chain].Labels
	f.run(Command{Operation: MergeItems, Item: lab, Target: other})
	s := f.e.Snapshot()
	if len(s.Current[chain].Labels) != 1 || s.Current[chain].Labels[0] != prior[1] || s.RelationHistory[0].Origin != prior[0] || s.Items[lab.key()].Lifecycle != Merged {
		t.Fatal("merge lost origin")
	}
	f.reject(f.command(Command{Operation: Archive, Item: other}), ErrConflict)
	f.reject(f.command(Command{Operation: HardDelete, Item: lab}), ErrConflict)
	f.run(Command{Operation: SetCurrent, ChainID: chain})
	f.reject(f.command(Command{Operation: HardDelete, Item: other}), ErrConflict)
}

// Upstream: testArchivedItemsStayVisibleOnExistingTaskButCannotBeNewlyAssignedUntilRestored.
func TestArchiveRestoreAndUnusedDeletion(t *testing.T) {
	f := setup(t)
	f.run(Command{Operation: Create, Item: cat, Name: "Old"})
	f.run(Command{Operation: SetCurrent, ChainID: chain, CategoryID: CategoryID(cat.ID)})
	f.run(Command{Operation: Archive, Item: cat})
	f.run(Command{Operation: SetCurrent, ChainID: chain, CategoryID: CategoryID(cat.ID)})
	c := f.command(Command{Operation: SetCurrent, ChainID: core.TaskChainID(id(5)), CategoryID: CategoryID(cat.ID)})
	f.reject(c, ErrConflict)
	f.run(Command{Operation: Restore, Item: cat})
	f.run(Command{Operation: SetCurrent, ChainID: core.TaskChainID(id(5)), CategoryID: CategoryID(cat.ID)})
	f.run(Command{Operation: Create, Item: lab, Name: "Old"})
	f.run(Command{Operation: HardDelete, Item: lab})
	f.run(Command{Operation: Create, Item: other, Name: "Old"})
	f.reject(f.command(Command{Operation: Create, Item: lab, Name: "New"}), ErrConflict)
}

// Upstream: state decode integrity and interaction-idempotency regression families.
func TestRoundTripReplayAndTamperRejection(t *testing.T) {
	f := setup(t)
	c := f.command(Command{Operation: Create, Item: cat, Name: "Old"})
	r, e := f.e.Execute(c, f.g)
	if e != nil {
		t.Fatal(e)
	}
	f.run(Command{Operation: SetCurrent, ChainID: chain, CategoryID: CategoryID(cat.ID)})
	again, e := f.e.Execute(c, f.g)
	if e != nil || again != r {
		t.Fatal(again, e)
	}
	c.Name = "New"
	f.reject(c, ErrConflict)
	s := f.e.Snapshot()
	b, _ := json.Marshal(s)
	var decoded Snapshot
	if json.Unmarshal(b, &decoded) != nil {
		t.Fatal("decode")
	}
	if _, e := RestoreSnapshot(canon(), decoded, f.g); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Snapshot){func(s *Snapshot) { v := s.Items[cat.key()]; v.Name = "forged"; s.Items[cat.key()] = v }, func(s *Snapshot) { s.Revision++ }, func(s *Snapshot) { s.Records[0].Command.Source.Generation = 1 }, func(s *Snapshot) { v := s.Current[chain]; v.Category.DecisionID = id(999) }, func(s *Snapshot) { s.Records = nil }, func(s *Snapshot) { s.CanonicalKeyVersion = "future" }} {
		bad := clone(s)
		mutate(&bad)
		if _, e := RestoreSnapshot(canon(), bad, f.g); e == nil {
			t.Fatal("accepted tamper")
		}
	}
	s.Items[cat.key()] = Item{}
	if f.e.Snapshot().Items[cat.key()].Name != "Old" {
		t.Fatal("snapshot alias")
	}
}
func TestFiniteClockUnknownFieldsAndAuthoritiesFailClosed(t *testing.T) {
	f := setup(t)
	base := Command{Operation: Create, Item: cat, Name: "Old"}
	for _, bits := range []uint64{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))} {
		c := f.command(base)
		c.At = core.Instant(bits)
		f.reject(c, ErrInvalid)
	}
	c := f.command(base)
	c.Target = other
	f.reject(c, ErrInvalid)
	c = f.command(base)
	c.Source = Source{Kind: AutomaticAI, JobID: id(400), Generation: 1}
	f.reject(c, ErrUnsupported)
	c = f.command(base)
	c.Source = Source{Kind: ZhulongSuggestion, SessionID: id(1), DraftID: id(2), EvidenceID: id(3), DraftVersion: 1}
	f.reject(c, ErrUnsupported)
	c = f.command(base)
	c.ExpectedRevision = 99
	f.reject(c, ErrConflict)
	c = f.command(Command{Operation: SetCurrent, ChainID: core.TaskChainID(id(999))})
	f.reject(c, ErrInvalid)
}
func TestSourceValidation(t *testing.T) {
	for _, s := range []Source{{Kind: UserDirect}, {Kind: AutomaticAI, JobID: id(1), Generation: 1}, {Kind: ZhulongSuggestion, SessionID: id(1), DraftID: id(2), EvidenceID: id(3), DraftVersion: 1}, {Kind: Inherited, FromChainID: chain}, {Kind: DeterministicDomainAction, Reason: "settle"}} {
		if s.Validate() != nil {
			t.Fatal(s)
		}
		s.JobID = "bad"
		if s.Validate() == nil {
			t.Fatal("extraneous/invalid field accepted")
		}
	}
}

// Upstream: testCategoryMergeSupportsAChainWhoseFinalTargetRemainsActive,
// testMergeAcceptsArchivedSourceButRejectsArchivedTarget.
func TestMergeChainsArchivedSourceAndTarget(t *testing.T) {
	f := setup(t)
	a := cat
	b := CategoryRef(CategoryID(id(23)))
	c := CategoryRef(CategoryID(id(24)))
	f.run(Command{Operation: Create, Item: a, Name: "Old"})
	f.run(Command{Operation: Create, Item: b, Name: "New"})
	f.run(Command{Operation: Create, Item: c, Name: "Other"})
	f.run(Command{Operation: Archive, Item: b})
	f.reject(f.command(Command{Operation: MergeItems, Item: a, Target: b}), ErrConflict)
	f.run(Command{Operation: Restore, Item: b})
	f.run(Command{Operation: Archive, Item: a})
	f.run(Command{Operation: MergeItems, Item: a, Target: b})
	f.run(Command{Operation: MergeItems, Item: b, Target: c})
	if got, e := f.e.ResolveName(Category, "Old"); e != nil || got != c {
		t.Fatal(got, e)
	}
	f.reject(f.command(Command{Operation: MergeItems, Item: c, Target: a}), ErrConflict)
	f.reject(f.command(Command{Operation: Archive, Item: c}), ErrConflict)
	if _, e := RestoreSnapshot(canon(), f.e.Snapshot(), f.g); e != nil {
		t.Fatal(e)
	}
}
func TestCaptureAppendsAndRejectsWrongTraceOrGraph(t *testing.T) {
	f := setup(t)
	f.run(Command{Operation: Create, Item: cat, Name: "Old"})
	f.run(Command{Operation: SetCurrent, ChainID: chain, CategoryID: CategoryID(cat.ID)})
	c := Command{Operation: Capture, ChainID: chain, TraceID: core.DayTraceID(id(4)), Status: core.TraceCompleted}
	f.run(c)
	first := f.e.Snapshot().History[0]
	f.run(Command{Operation: Rename, Item: cat, Name: "New"})
	f.run(c)
	s := f.e.Snapshot()
	if len(s.History) != 2 || !reflect.DeepEqual(s.History[0], first) || s.History[1].Category.Name != "New" {
		t.Fatal("overwrote history")
	}
	c.Status = core.TraceAbandoned
	f.reject(f.command(c), ErrInvalid)
	before := f.e.Snapshot()
	_, e := f.e.Execute(f.command(Command{Operation: Rename, Item: cat, Name: "Other"}), core.TaskGraph{})
	if e == nil || !reflect.DeepEqual(before, f.e.Snapshot()) {
		t.Fatal("accepted dangling graph")
	}
	if _, e := RestoreSnapshot(canon(), s, core.TaskGraph{}); e == nil {
		t.Fatal("restored dangling graph")
	}
}
func TestDeterministicDomainSourceAndConcurrentCAS(t *testing.T) {
	f := setup(t)
	c := f.command(Command{Operation: Create, Item: cat, Name: "Old"})
	c.Source = Source{Kind: DeterministicDomainAction, Reason: "synthetic fixture"}
	c.DecisionID = ""
	if _, e := f.e.Execute(c, f.g); e != nil {
		t.Fatal(e)
	}
	commands := []Command{f.command(Command{Operation: Rename, Item: cat, Name: "New"}), f.command(Command{Operation: Rename, Item: cat, Name: "Other"})}
	commands[1].At = commands[0].At
	done := make(chan error, 2)
	for _, c := range commands {
		go func(c Command) { _, e := f.e.Execute(c, f.g); done <- e }(c)
	}
	success := 0
	for range commands {
		if e := <-done; e == nil {
			success++
		} else if !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("lost CAS", success)
	}
}
func FuzzSourceNeverPanics(f *testing.F) {
	f.Add("userDirect", "", 0)
	f.Add("automaticAI", id(1), 1)
	f.Fuzz(func(t *testing.T, k, j string, n int) {
		_ = (Source{Kind: SourceKind(k), JobID: j, Generation: n}).Validate()
	})
}
func TestInvalidUTF8CannotBeSilentlyRewritten(t *testing.T) {
	f := setup(t)
	c := f.command(Command{Operation: Create, Item: cat, Name: "Old", ColorHex: string([]byte{0xff})})
	f.reject(c, ErrInvalid)
	if (Source{Kind: DeterministicDomainAction, Reason: string([]byte{0xff})}).Validate() == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
