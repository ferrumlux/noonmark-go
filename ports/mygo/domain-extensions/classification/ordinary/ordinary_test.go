// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func uuid(n int) UUID { return UUID(fmt.Sprintf("00000000-0000-4000-8000-%012X", n)) }
func instant(n float64) core.Instant {
	x, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return x
}
func graphFixture(t testing.TB, chains int) core.TaskGraph {
	t.Helper()
	kernel, e := core.NewTaskKernel(core.TaskGraph{})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < chains; i++ {
		if e = kernel.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(uuid(i + 1)), DefinitionID: core.TaskDefinitionID(uuid(i + 101)), Title: "合成开发任务", Now: instant(0)}); e != nil {
			t.Fatal(e)
		}
	}
	return kernel.Snapshot()
}
func contextFixture(t testing.TB, n int) OrdinaryContext {
	t.Helper()
	ctx, e := NewEmptyContext(graphFixture(t, n))
	if e != nil {
		t.Fatal(e)
	}
	return ctx
}

type fixture struct {
	ctx   OrdinaryContext
	s     State
	steps []CommittedStep
	n     int
}

func newFixture(t testing.TB, n int) *fixture {
	return &fixture{ctx: contextFixture(t, n), s: EmptyState(), n: 1000}
}
func bytesOf(t testing.TB, v any) string {
	t.Helper()
	x, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(x)
}
func (f *fixture) commit(t testing.TB, chain int, choice Choice, now core.Instant) (Plan, Confirmation, Receipt) {
	t.Helper()
	f.n += 10
	r := Request{core.TaskChainID(uuid(chain)), choice, uuid(f.n)}
	ids := PrepareIDs{uuid(f.n + 1), CategoryID(uuid(f.n + 2))}
	commitIDs := CommitIDs{uuid(f.n + 3), uuid(f.n + 4), uuid(f.n + 5)}
	before := bytesOf(t, f.s)
	p, e := Prepare(f.ctx, f.s, r, ids)
	if e != nil {
		t.Fatal("prepare", e)
	}
	c, e := ConfirmUser(p, r.InteractionID)
	if e != nil {
		t.Fatal(e)
	}
	next, receipt, e := Apply(f.ctx, f.s, p, c, now, commitIDs)
	if e != nil {
		t.Fatal("apply", e)
	}
	if bytesOf(t, f.s) != before {
		t.Fatal("input mutated")
	}
	d, e := p.Data()
	if e != nil {
		t.Fatal(e)
	}
	f.steps = append(f.steps, CommittedStep{r, ids, d, r.InteractionID, now, commitIDs, receipt})
	f.s = next
	return p, c, receipt
}
func TestOrdinaryMainSequenceAndBudgetPositive(t *testing.T) {
	f := newFixture(t, 2)
	p, c, r := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程", ColorHex: "#D87831"}, instant(10))
	id := p.sealed.data.PlannedCategoryID
	if f.s.Revision != 1 || len(f.s.Categories) != 1 || len(f.s.Categories[id].NameVersions) != 1 || len(f.s.ChangeRecords) != 1 || len(f.s.ChangeRecords[0].Changes) != 1 {
		t.Fatal("create/select is one fact")
	}
	origin := *f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category
	p, c, r = f.commit(t, 1, Choice{Kind: ChoiceExisting, CategoryID: id}, instant(11))
	if f.s.Revision != 2 || *f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category != origin {
		t.Fatal("equal new interaction must audit, preserving origin")
	}
	before := bytesOf(t, f.s)
	next, retry, e := Apply(f.ctx, f.s, p, c, core.Instant(math.Float64bits(math.NaN())), CommitIDs{})
	if e != nil || !reflect.DeepEqual(retry, r) || bytesOf(t, next) != before || bytesOf(t, f.s) != before {
		t.Fatal("exact retry before stale/preview/clock", e)
	}
	prior := f.steps[len(f.steps)-1]
	prior.Now = core.Instant(math.Float64bits(math.Inf(1)))
	prior.CommitIDs = CommitIDs{}
	f.steps = append(f.steps, prior)
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(12))
	if len(f.s.RelationHistory) != 1 || f.s.RelationHistory[0].OriginDecisionID != origin.DecisionID || f.s.RelationHistory[0].CreatedRevision != 1 {
		t.Fatal("removal lost origin")
	}
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(1))  // no relation clock remains
	f.commit(t, 2, Choice{Kind: ChoiceNone}, instant(-1)) // independent chain may go backwards globally
	f.commit(t, 2, Choice{Kind: ChoiceExisting, CategoryID: id}, instant(2))
	f.commit(t, 2, Choice{Kind: ChoiceNew, Name: " 工程\t", ColorHex: "ignored"}, instant(3))
	if f.s.Revision != 7 || len(f.s.CurrentByChainID) != 2 || len(f.s.Categories) != 1 || len(f.s.ChangeRecords) != 7 || len(f.s.CommittedReceiptsByInteractionID) != 7 {
		t.Fatal("main sequence facts")
	}
	last := f.steps[len(f.steps)-1].Plan
	if last.PlannedCategoryID != "" || last.Intent.Choice.Kind != ChoiceExisting || len(last.Notices) != 1 || last.Notices[0].MatchedHistoricalAlias {
		t.Fatal("reuse")
	}
	proof, e := Replay(f.ctx, f.steps)
	if e != nil {
		t.Fatal("whole transcript budget positive", e)
	}
	actual, e := proof.State()
	if e != nil || bytesOf(t, actual) != bytesOf(t, f.s) {
		t.Fatal("replay", e)
	}
	prefix, e := Replay(f.ctx, f.steps[:4])
	if e != nil {
		t.Fatal("prefix replay", e)
	}
	continued, e := Continue(prefix, f.steps[4:])
	if e != nil {
		t.Fatal("full proof continuation budget positive", e)
	}
	continuedState, e := continued.State()
	if e != nil || bytesOf(t, continuedState) != bytesOf(t, f.s) {
		t.Fatal("continued full transcript", e)
	}
	if e = ValidateDecodedEntities(f.ctx, f.s); e != nil {
		t.Fatal("entity decode", e)
	}
}
func TestMultipleShortChineseEntitiesAndChainsBudgetPositive(t *testing.T) {
	f := newFixture(t, 4)
	for i, name := range []string{"工程", "研究", "学习", "生活"} {
		f.commit(t, i+1, Choice{Kind: ChoiceNew, Name: name, ColorHex: "#123456"}, instant(float64(i)))
	}
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(8))
	f.commit(t, 2, Choice{Kind: ChoiceNone}, instant(9))
	if _, e := Replay(f.ctx, f.steps); e != nil {
		t.Fatal("practical multi-entity replay rejected", e)
	}
}
func TestAtomicFailuresAndConcurrentBasePlans(t *testing.T) {
	f := newFixture(t, 1)
	f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(10))
	req := Request{core.TaskChainID(uuid(1)), Choice{Kind: ChoiceNone}, uuid(2000)}
	ids := PrepareIDs{PlanID: uuid(2001)}
	p, e := Prepare(f.ctx, f.s, req, ids)
	if e != nil {
		t.Fatal(e)
	}
	c, e := ConfirmUser(p, req.InteractionID)
	if e != nil {
		t.Fatal(e)
	}
	for name, tc := range map[string]struct {
		now          core.Instant
		ids          CommitIDs
		confirmation Confirmation
	}{
		"nonfinite":            {core.Instant(math.Float64bits(math.NaN())), CommitIDs{RecordID: uuid(2003), RemovalID: uuid(2005)}, c},
		"backward relation":    {instant(9), CommitIDs{RecordID: uuid(2003), RemovalID: uuid(2005)}, c},
		"record slot failure":  {instant(11), CommitIDs{RemovalID: uuid(2005)}, c},
		"removal slot failure": {instant(11), CommitIDs{RecordID: uuid(2003)}, c},
		"zero authority":       {instant(11), CommitIDs{RecordID: uuid(2003), RemovalID: uuid(2005)}, Confirmation{}},
	} {
		t.Run(name, func(t *testing.T) {
			before := bytesOf(t, f.s)
			if _, _, e := Apply(f.ctx, f.s, p, tc.confirmation, tc.now, tc.ids); e == nil {
				t.Fatal("accepted")
			}
			if bytesOf(t, f.s) != before {
				t.Fatal("changed input")
			}
		})
	}
	secondReq := req
	secondReq.InteractionID = uuid(2100)
	second, e := Prepare(f.ctx, f.s, secondReq, PrepareIDs{PlanID: uuid(2101)})
	if e != nil {
		t.Fatal(e)
	}
	secondC, e := ConfirmUser(second, secondReq.InteractionID)
	if e != nil {
		t.Fatal(e)
	}
	next, _, e := Apply(f.ctx, f.s, p, c, instant(11), CommitIDs{RecordID: uuid(2003), RemovalID: uuid(2005)})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Apply(f.ctx, next, second, secondC, instant(12), CommitIDs{RecordID: uuid(2103)}); !errors.Is(e, ErrStale) {
		t.Fatal("stale", e)
	}
	wrong, e := ConfirmUser(p, uuid(2999))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Apply(f.ctx, next, p, wrong, instant(12), CommitIDs{}); !errors.Is(e, ErrConflict) {
		t.Fatal("decision retry", e)
	}
	other := contextFixture(t, 1)
	if _, _, e = Apply(other, f.s, p, c, instant(12), CommitIDs{}); !errors.Is(e, ErrContext) {
		t.Fatal("context replacement", e)
	}
}
func TestInputAndReturnedOwnershipAndSealedProof(t *testing.T) {
	graph := graphFixture(t, 2)
	ctx, e := NewEmptyContext(graph)
	if e != nil {
		t.Fatal(e)
	}
	graph.Chains[0].ID = "broken"
	view, e := ctx.Graph()
	if e != nil || !validUUID(view.Chains[0].ID) {
		t.Fatal("graph alias", e)
	}
	view.Definitions[0].Title = "mutated"
	f := &fixture{ctx: ctx, s: EmptyState(), n: 1000}
	p, _, receipt := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(1))
	d, e := p.Data()
	if e != nil {
		t.Fatal(e)
	}
	d.Changes[0].After.Category.Name = "mutated"
	again, _ := p.Data()
	if again.Changes[0].After.Category.Name != "工程" {
		t.Fatal("plan alias")
	}
	proof, e := Replay(ctx, f.steps)
	if e != nil {
		t.Fatal(e)
	}
	got, e := proof.State()
	if e != nil {
		t.Fatal(e)
	}
	for id, c := range got.Categories {
		c.NameVersions[0].Name = "mutated"
		got.Categories[id] = c
	}
	got.ChangeRecords[0].Changes[0].After.Category.Name = "mutated"
	got.CommittedReceiptsByInteractionID[uuid(1010)] = Receipt{}
	transcript, e := proof.Transcript()
	if e != nil {
		t.Fatal(e)
	}
	transcript[0].Plan.Changes[0].After.Category.Name = "mutated"
	transcript[0].Receipt = Receipt{}
	unchanged, e := proof.State()
	if e != nil || bytesOf(t, unchanged) != bytesOf(t, f.s) {
		t.Fatal("proof state alias", e)
	}
	if _, e = Continue(proof, nil); e != nil {
		t.Fatal("proof transcript alias", e)
	}
	if _, e = Continue(ReplayProof{}, f.steps); e == nil {
		t.Fatal("zero proof accepted")
	}
	if _, e = (Plan{}).Data(); e == nil {
		t.Fatal("zero plan")
	}
	// Returned receipt must not alias the state's notice backing array.
	receipt.Notices = append(receipt.Notices, Notice{Kind: "bad"})
	if len(f.s.ChangeRecords[0].Notices) != 0 {
		t.Fatal("receipt alias")
	}
}
func TestGraphOrdinaryScopeHasNoStatusOrDateGate(t *testing.T) {
	graph := graphFixture(t, 1)
	graph.Chains[0].State = core.ChainAbandoned
	ctx, e := NewEmptyContext(graph)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Prepare(ctx, EmptyState(), Request{graph.Chains[0].ID, Choice{Kind: ChoiceNone}, uuid(333)}, PrepareIDs{PlanID: uuid(334)}); e != nil {
		t.Fatal("invented active/pending gate", e)
	}
	graph.Chains[0].CycleMembership = &core.TaskCycleMembership{SeriesID: core.TaskCycleSeriesID(uuid(999)), OccurrenceDate: core.LocalDate{Year: 2026, Month: 11, Day: 7}}
	if _, e = NewEmptyContext(graph); !errors.Is(e, ErrUnsupported) {
		t.Fatal("membership", e)
	}
}
func TestNameStagesPreserveBytes(t *testing.T) {
	inputs := []string{"工程", "E\u0301", "ＡＢＣ", "Straße", "İIı", "foo\u00adbar", "a\n\u0301b", " \u0301X", "x" + strings.Repeat("\u0315\u0300", 31)}
	for _, raw := range inputs {
		t.Run(fmt.Sprintf("%x", raw), func(t *testing.T) {
			stages := []string{raw}
			for i := 0; i < 4; i++ {
				d, e := canonical16.DisplayName(stages[len(stages)-1])
				if e != nil {
					t.Fatal(e)
				}
				stages = append(stages, d)
			}
			direct, key, e := (canonical16.Canonicalizer{}).Canonicalize(raw)
			if e != nil || direct != stages[1] {
				t.Fatal(e)
			}
			wantKey, _ := canonical16.CanonicalKey(direct)
			if key != wantKey {
				t.Fatal("direct key")
			}
			f := newFixture(t, 1)
			p, _, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: raw}, instant(1))
			if p.sealed.data.Intent.Choice.Name != stages[1] || p.sealed.data.Changes[0].After.Category.Name != stages[1] {
				t.Fatal("prepare/preview extra D")
			}
			stored := f.s.Categories[p.sealed.data.PlannedCategoryID]
			if stored.Name != stages[3] {
				t.Fatal("core D3", stored.Name, stages[3])
			}
			wantKey, _ = canonical16.CanonicalKey(stages[3])
			if stored.CanonicalKey != wantKey {
				t.Fatal("key")
			}
			ui := newFixture(t, 1)
			up, _, _ := ui.commit(t, 1, Choice{Kind: ChoiceNew, Name: stages[1]}, instant(1))
			if ui.s.Categories[up.sealed.data.PlannedCategoryID].Name != stages[4] {
				t.Fatal("UI D4")
			}
		})
	}
	for _, bad := range []string{"", " \t\n", "\u200b", "\xff"} {
		f := newFixture(t, 1)
		before := bytesOf(t, f.s)
		if _, e := Prepare(f.ctx, f.s, Request{core.TaskChainID(uuid(1)), Choice{Kind: ChoiceNew, Name: bad}, uuid(200)}, PrepareIDs{uuid(201), CategoryID(uuid(202))}); e == nil {
			t.Fatalf("accepted %q", bad)
		}
		if bytesOf(t, f.s) != before {
			t.Fatal("failed name mutated state")
		}
	}
}
func TestDecodeIsSeparateAndSwiftEquivalent(t *testing.T) {
	ctx := contextFixture(t, 1)
	state := EmptyState()
	name := "a \u0301b"
	key, _ := canonical16.CanonicalKey(name)
	id := CategoryID(uuid(600))
	state.Categories = map[CategoryID]Category{id: {ID: id, Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, PresentationApproval: "userApproved", Lifecycle: "active", NameVersions: []NameVersion{{ID: uuid(601), Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, ValidFrom: instant(0)}}, CreatedAt: instant(0), UpdatedAt: instant(0)}}
	if e := ValidateApplyIntegrity(ctx, state); e != nil {
		t.Fatal("integrity unexpectedly invoked decoder", e)
	}
	if e := ValidateDecodedEntities(ctx, state); !errors.Is(e, ErrName) {
		t.Fatal("decoder did not reject nonfixed name", e)
	}
	c := state.Categories[id]
	c.Name = "E\u0301"
	c.CanonicalKey = "é"
	c.NameVersions[0].Name = "É"
	c.NameVersions[0].CanonicalKey = "e\u0301"
	state.Categories[id] = c
	if e := ValidateDecodedEntities(ctx, state); e != nil {
		t.Fatal("Swift equivalence must not be raw byte equality", e)
	}
	if _, e := Replay(ctx, nil); e != nil {
		t.Fatal(e)
	} // imported catalog is not an anchor API
}
func TestParallelImmutableContextAndPlan(t *testing.T) {
	f := newFixture(t, 1)
	p, e := Prepare(f.ctx, f.s, Request{core.TaskChainID(uuid(1)), Choice{Kind: ChoiceNew, Name: "工程"}, uuid(900)}, PrepareIDs{uuid(901), CategoryID(uuid(902))})
	if e != nil {
		t.Fatal(e)
	}
	c, e := ConfirmUser(p, uuid(900))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, _, e := Apply(f.ctx, f.s, p, c, instant(1), CommitIDs{RecordID: uuid(903), NameVersionID: uuid(904)}); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
}
