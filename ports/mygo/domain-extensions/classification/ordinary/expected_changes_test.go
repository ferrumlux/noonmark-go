// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Independent review M2-EQ: TC:2327-2342 compares expectedChanges using Swift
// Equatable AFTER validating the original plan digest. Canonically equivalent
// imported state bytes must neither invalidate nor replace the original plan.
func TestExpectedChangesCanonicalEquivalencePreservesOriginalFacts(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		chain  int
		choice Choice
	}{
		{"after_existing", 2, Choice{Kind: ChoiceExisting}},
		{"before_and_after_existing", 1, Choice{Kind: ChoiceExisting}},
		{"before_none", 1, Choice{Kind: ChoiceNone}},
		{"after_reuse_notice", 2, Choice{Kind: ChoiceNew, Name: "é", ColorHex: "unused"}},
		{"before_new", 1, Choice{Kind: ChoiceNew, Name: "研究", ColorHex: "#123456"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newFixture(t, 2)
			f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "é", ColorHex: "é"}, instant(0))
			cat := f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category.CategoryID
			choice := scenario.choice
			if choice.Kind == ChoiceExisting {
				choice.CategoryID = cat
			}
			req := Request{core.TaskChainID(uuid(scenario.chain)), choice, uuid(5000)}
			ids := PrepareIDs{uuid(5001), CategoryID(uuid(5002))}
			p, e := Prepare(f.ctx, f.s, req, ids)
			if e != nil {
				t.Fatal(e)
			}
			original, e := p.Data()
			if e != nil {
				t.Fatal(e)
			}
			confirm, e := ConfirmUser(p, req.InteractionID)
			if e != nil {
				t.Fatal(e)
			}
			entity := f.s.Categories[cat]
			entity.Name = "e\u0301"
			entity.NameVersions[0].Name = entity.Name
			entity.ColorHex = "e\u0301"
			f.s.Categories[cat] = entity
			if e = ValidateDecodedEntities(f.ctx, f.s); e != nil {
				t.Fatal("canonical-equivalent positive state", e)
			}
			before := bytesOf(t, f.s)
			next, receipt, e := Apply(f.ctx, f.s, p, confirm, instant(1), CommitIDs{uuid(5003), uuid(5004), uuid(5005)})
			if e != nil {
				t.Fatal("Swift Equatable expectedChanges should accept without changing plan digest", e)
			}
			if bytesOf(t, f.s) != before {
				t.Fatal("input changed")
			}
			retained, _ := p.Data()
			if !reflect.DeepEqual(retained, original) {
				t.Fatal("original plan bytes changed")
			}
			if next.Categories[cat].Name != "e\u0301" || next.Categories[cat].ColorHex != "e\u0301" {
				t.Fatal("state bytes repaired")
			}
			record := next.ChangeRecords[len(next.ChangeRecords)-1]
			if record.PlanDigest != original.Digest || !sameChanges(record.Changes, original.Changes) || !sameNotices(record.Notices, original.Notices) || receipt.ChangeRecordID != record.ID || receipt.ChangeRecordIntegrityDigest != record.IntegrityDigest {
				t.Fatal("original plan/record/receipt bytes or binding changed")
			}
			retry, r, e := Apply(f.ctx, next, p, confirm, core.Instant(math.Float64bits(math.NaN())), CommitIDs{})
			if e != nil || !reflect.DeepEqual(retry, next) || !sameReceipt(r, receipt) {
				t.Fatal("original retry changed after semantic guard", e)
			}
		})
	}
}

func TestExpectedChangesRejectsActualFieldChanges(t *testing.T) {
	f := newFixture(t, 2)
	p, _, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "é", ColorHex: "é"}, instant(0))
	reference := p.sealed.data.Changes
	for name, mutate := range map[string]func([]PlanChange){
		"name":            func(c []PlanChange) { c[0].After.Category.Name = "different" },
		"color":           func(c []PlanChange) { c[0].After.Category.ColorHex = "different" },
		"item_id":         func(c []PlanChange) { c[0].After.Category.ID = CategoryID(uuid(999)) },
		"item_kind":       func(c []PlanChange) { c[0].After.Category.Kind = "label" },
		"resolution":      func(c []PlanChange) { c[0].After.Category.Resolution = "existing" },
		"chain":           func(c []PlanChange) { c[0].ChainID = core.TaskChainID(uuid(2)) },
		"change_kind":     func(c []PlanChange) { c[0].Kind = "create" },
		"missing_preview": func(c []PlanChange) { c[0].After.Category = nil },
		"labels":          func(c []PlanChange) { c[0].After.Labels = []PlanItem{{Kind: "label"}} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneChanges(reference)
			mutate(changed)
			before := bytesOf(t, changed)
			b, _ := newBudget(DefaultLimits())
			if equal, e := expectedChangesEquivalent(b, reference, changed); equal || e != nil && !errors.Is(e, ErrUnsupported) {
				t.Fatal("accepted wrong field or unexpected error", equal, e)
			}
			if bytesOf(t, changed) != before {
				t.Fatal("comparator changed input")
			}
		})
	}
}

func TestExpectedChangesRejectsDifferentValidImportedState(t *testing.T) {
	for _, field := range []string{"name", "color"} {
		t.Run(field, func(t *testing.T) {
			f := newFixture(t, 2)
			f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "é", ColorHex: "é"}, instant(0))
			cat := f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category.CategoryID
			req := Request{core.TaskChainID(uuid(2)), Choice{Kind: ChoiceExisting, CategoryID: cat}, uuid(6000)}
			p, e := Prepare(f.ctx, f.s, req, PrepareIDs{PlanID: uuid(6001)})
			if e != nil {
				t.Fatal(e)
			}
			confirmation, e := ConfirmUser(p, req.InteractionID)
			if e != nil {
				t.Fatal(e)
			}
			entity := f.s.Categories[cat]
			if field == "name" {
				entity.Name = "different"
				entity.CanonicalKey = "different"
				entity.NameVersions[0].Name = "different"
				entity.NameVersions[0].CanonicalKey = "different"
			} else {
				entity.ColorHex = "different"
			}
			f.s.Categories[cat] = entity
			if e = ValidateDecodedEntities(f.ctx, f.s); e != nil {
				t.Fatal("valid imported-state positive control", e)
			}
			before := bytesOf(t, f.s)
			if _, _, e = Apply(f.ctx, f.s, p, confirmation, instant(1), CommitIDs{RecordID: uuid(6003)}); !errors.Is(e, ErrInvalid) {
				t.Fatal("genuinely different field accepted", e)
			}
			if bytesOf(t, f.s) != before {
				t.Fatal("failed apply changed input")
			}
		})
	}
}

func TestExpectedChangesPrechargesBothOperandsAndRepeatedWork(t *testing.T) {
	reference := []PlanChange{{Kind: "setCurrent", ChainID: core.TaskChainID(uuid(1)), After: SelectionPreview{Category: &PlanItem{ID: CategoryID(uuid(2)), Kind: "category", Name: "é", ColorHex: "é", Resolution: "existing"}}}}
	equivalent := cloneChanges(reference)
	equivalent[0].After.Category.Name = "e\u0301"
	equivalent[0].After.Category.ColorHex = "e\u0301"
	// Two names and two colors each pay NFC's output/rune/sort/count reservation.
	cost := 2 * (64*(len("é")+len("e\u0301")) + 2*2048)
	b, _ := newBudget(Limits{MaxBytes: cost})
	equal, e := expectedChangesEquivalent(b, reference, equivalent)
	if e != nil || !equal || b.bytes != uint64(cost) {
		t.Fatal("exact combined boundary", equal, e, b.bytes, cost)
	}
	if _, e = expectedChangesEquivalent(b, reference, equivalent); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("comparison reset ledger", e)
	}
	before := bytesOf(t, equivalent)
	b, _ = newBudget(Limits{MaxBytes: cost - 1})
	if _, e = expectedChangesEquivalent(b, reference, equivalent); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("both operand costs were not charged", e)
	}
	if bytesOf(t, equivalent) != before {
		t.Fatal("budget failure changed input")
	}
}

func TestExpectedGuardLeavesOriginalRetryAndTranscriptBytesStrict(t *testing.T) {
	f := newFixture(t, 1)
	p, c, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "é", ColorHex: "é"}, instant(0))
	state := cloneState(f.s)
	state.ChangeRecords[0].Changes[0].After.Category.Name = "e\u0301"
	rehashRecord(t, &state, 0)
	if e := ValidateDecodedEntities(f.ctx, state); e != nil {
		t.Fatal("equivalent audit positive control", e)
	}
	if _, _, e := Apply(f.ctx, state, p, c, instant(1), CommitIDs{}); !errors.Is(e, ErrConflict) {
		t.Fatal("retry original record bytes binding relaxed", e)
	}
	steps := cloneSteps(f.steps)
	steps[0].Plan.Changes[0].After.Category.Name = "e\u0301"
	b, _ := newBudget(DefaultLimits())
	digest, e := planDigest(b, steps[0].Plan)
	if e != nil {
		t.Fatal(e)
	}
	steps[0].Plan.Digest = digest
	if _, e = Replay(f.ctx, steps); e == nil {
		t.Fatal("retained transcript plan bytes binding relaxed")
	}
	data, e := p.Data()
	if e != nil {
		t.Fatal(e)
	}
	if data.Digest != f.steps[0].Plan.Digest || data.Changes[0].After.Category.Name != "é" {
		t.Fatal("original plan mutated")
	}
}

func TestExpectedGuardDoesNotOverwriteOccupiedPlannedIdentity(t *testing.T) {
	f := newFixture(t, 2)
	f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(0))
	req := Request{core.TaskChainID(uuid(2)), Choice{Kind: ChoiceNew, Name: "研究"}, uuid(7000)}
	ids := PrepareIDs{uuid(7001), CategoryID(uuid(7002))}
	p, e := Prepare(f.ctx, f.s, req, ids)
	if e != nil {
		t.Fatal(e)
	}
	confirmation, e := ConfirmUser(p, req.InteractionID)
	if e != nil {
		t.Fatal(e)
	}
	var occupied Category
	for _, category := range f.s.Categories {
		occupied = category
		break
	}
	occupied.ID = ids.CategoryID
	occupied.Name = "学习"
	occupied.CanonicalKey = "学习"
	occupied.NameVersions = append([]NameVersion(nil), occupied.NameVersions...)
	occupied.NameVersions[0].Name = "学习"
	occupied.NameVersions[0].CanonicalKey = "学习"
	occupied.NameVersions[0].ID = uuid(7999)
	f.s.Categories[occupied.ID] = occupied
	if e = ValidateDecodedEntities(f.ctx, f.s); e != nil {
		t.Fatal("imported catalog positive control", e)
	}
	before := bytesOf(t, f.s)
	if _, _, e = Apply(f.ctx, f.s, p, confirmation, instant(1), CommitIDs{RecordID: uuid(7003), NameVersionID: uuid(7004)}); !errors.Is(e, ErrConflict) {
		t.Fatal("occupied planned ID not rejected", e)
	}
	if bytesOf(t, f.s) != before {
		t.Fatal("occupied category overwritten")
	}
}
