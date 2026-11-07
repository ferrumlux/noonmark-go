// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func auditedFixture(t testing.TB) *fixture {
	f := newFixture(t, 2)
	f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程", ColorHex: "#D87831"}, instant(1))
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(2))
	return f
}
func rehashRecord(t testing.TB, s *State, index int) {
	t.Helper()
	b, _ := newBudget(DefaultLimits())
	r := &s.ChangeRecords[index]
	d, e := recordDigest(b, *r)
	if e != nil {
		t.Fatal(e)
	}
	r.IntegrityDigest = d
	receipt := s.CommittedReceiptsByInteractionID[r.InteractionID]
	receipt.ChangeRecordIntegrityDigest = d
	s.CommittedReceiptsByInteractionID[r.InteractionID] = receipt
}
func TestAuditMutationsFromPositiveControl(t *testing.T) {
	f := auditedFixture(t)
	if e := ValidateApplyIntegrity(f.ctx, f.s); e != nil {
		t.Fatal("positive control", e)
	}
	for name, mutate := range map[string]func(*State){
		"format":                         func(s *State) { s.FormatVersion++ },
		"revision count":                 func(s *State) { s.Revision++ },
		"huge claimed revision":          func(s *State) { s.Revision = math.MaxUint64 },
		"lost empty current":             func(s *State) { delete(s.CurrentByChainID, core.TaskChainID(uuid(1))) },
		"unknown current chain":          func(s *State) { s.CurrentByChainID[core.TaskChainID(uuid(888))] = Current{} },
		"missing removal":                func(s *State) { s.RelationHistory = nil },
		"removal wrong origin":           func(s *State) { s.RelationHistory[0].OriginDecisionID = uuid(999) },
		"removal wrong created revision": func(s *State) { s.RelationHistory[0].CreatedRevision = 2 },
		"removal missing chain":          func(s *State) { s.RelationHistory[0].ChainID = core.TaskChainID(uuid(888)) },
		"label removal":                  func(s *State) { s.RelationHistory[0].Kind = "label" },
		"missing receipt":                func(s *State) { delete(s.CommittedReceiptsByInteractionID, s.ChangeRecords[0].InteractionID) },
		"extra receipt": func(s *State) {
			s.CommittedReceiptsByInteractionID[uuid(888)] = s.CommittedReceiptsByInteractionID[s.ChangeRecords[0].InteractionID]
		},
		"receipt binding": func(s *State) {
			id := s.ChangeRecords[0].InteractionID
			r := s.CommittedReceiptsByInteractionID[id]
			r.DecisionID = uuid(999)
			s.CommittedReceiptsByInteractionID[id] = r
		},
		"record digest":     func(s *State) { s.ChangeRecords[0].IntegrityDigest = strings.Repeat("0", 64) },
		"plan digest shape": func(s *State) { s.ChangeRecords[0].PlanDigest = "x"; rehashRecord(t, s, 0) },
		"audit order":       func(s *State) { s.ChangeRecords[0], s.ChangeRecords[1] = s.ChangeRecords[1], s.ChangeRecords[0] },
		"duplicate plan":    func(s *State) { s.ChangeRecords[1].PlanID = s.ChangeRecords[0].PlanID; rehashRecord(t, s, 1) },
		"AI source":         func(s *State) { s.ChangeRecords[0].Source = "automaticAI"; rehashRecord(t, s, 0) },
		"management change": func(s *State) { s.ChangeRecords[0].Changes[0].Kind = "create"; rehashRecord(t, s, 0) },
		"extra change": func(s *State) {
			s.ChangeRecords[0].Changes = append(s.ChangeRecords[0].Changes, s.ChangeRecords[0].Changes[0])
			rehashRecord(t, s, 0)
		},
		"label preview": func(s *State) {
			s.ChangeRecords[0].Changes[0].After.Labels = []PlanItem{{ID: CategoryID(uuid(777)), Kind: "label"}}
			rehashRecord(t, s, 0)
		},
		"unknown preview resolution": func(s *State) {
			s.ChangeRecords[0].Changes[0].After.Category.Resolution = "historicalAlias"
			rehashRecord(t, s, 0)
		},
		"unknown record chain": func(s *State) {
			s.ChangeRecords[0].Changes[0].ChainID = core.TaskChainID(uuid(999))
			rehashRecord(t, s, 0)
		},
		"bad before preview": func(s *State) { s.ChangeRecords[1].Changes[0].Before.Category = nil; rehashRecord(t, s, 1) },
		"entity dictionary key": func(s *State) {
			for id, c := range s.Categories {
				delete(s.Categories, id)
				s.Categories[CategoryID(uuid(99))] = c
				break
			}
		},
		"entity lifecycle": func(s *State) {
			for id, c := range s.Categories {
				c.Lifecycle = "archived"
				s.Categories[id] = c
			}
		},
		"unknown algorithm": func(s *State) {
			for id, c := range s.Categories {
				c.CanonicalKeyVersion = "future"
				s.Categories[id] = c
			}
		},
		"extra name version": func(s *State) {
			for id, c := range s.Categories {
				c.NameVersions = append(c.NameVersions, c.NameVersions[0])
				s.Categories[id] = c
			}
		},
		"closed name version": func(s *State) {
			for id, c := range s.Categories {
				end := instant(2)
				c.NameVersions[0].ValidUntil = &end
				s.Categories[id] = c
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := cloneState(f.s)
			mutate(&s)
			before := bytesOf(t, s)
			if e := ValidateApplyIntegrity(f.ctx, s); e == nil {
				t.Fatal("accepted mutation")
			}
			if bytesOf(t, s) != before {
				t.Fatal("validation mutated input")
			}
		})
	}
}
func TestReplayRejectsRetainedPlanTampering(t *testing.T) {
	f := auditedFixture(t)
	if _, e := Replay(f.ctx, f.steps); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func([]CommittedStep){
		"raw prepare input":           func(s []CommittedStep) { s[0].Request.Choice.Name = "学习" },
		"intent excluded from digest": func(s []CommittedStep) { s[0].Plan.Intent.Choice.Name = "学习" },
		"preview":                     func(s []CommittedStep) { s[0].Plan.Changes[0].After.Category.Name = "学习" },
		"planned ID":                  func(s []CommittedStep) { s[0].Plan.PlannedCategoryID = CategoryID(uuid(800)) },
		"notice": func(s []CommittedStep) {
			s[0].Plan.Notices = []Notice{{Kind: "existingItemReused", ItemKind: "category", InputName: "工程", CurrentName: "工程"}}
		},
		"decision":          func(s []CommittedStep) { s[0].DecisionID = uuid(800) },
		"clock bits":        func(s []CommittedStep) { s[0].Now = instant(1.00001) },
		"record slot":       func(s []CommittedStep) { s[0].CommitIDs.RecordID = uuid(800) },
		"name version slot": func(s []CommittedStep) { s[0].CommitIDs.NameVersionID = "" },
		"removal slot":      func(s []CommittedStep) { s[1].CommitIDs.RemovalID = "" },
		"receipt":           func(s []CommittedStep) { s[0].Receipt.Revision = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			steps := cloneSteps(f.steps)
			mutate(steps)
			before := bytesOf(t, steps)
			if _, e := Replay(f.ctx, steps); e == nil {
				t.Fatal("accepted")
			}
			if bytesOf(t, steps) != before {
				t.Fatal("mutated transcript")
			}
		})
	}
}
func TestRetryOriginalPlanAfterLaterFactsAndIgnoreNewClock(t *testing.T) {
	f := newFixture(t, 2)
	p, c, r := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(2))
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(3))
	f.commit(t, 2, Choice{Kind: ChoiceNew, Name: "研究"}, instant(-5))
	for _, now := range []core.Instant{instant(-999), core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(-1)))} {
		next, retry, e := Apply(f.ctx, f.s, p, c, now, CommitIDs{})
		if e != nil || bytesOf(t, next) != bytesOf(t, f.s) || bytesOf(t, retry) != bytesOf(t, r) {
			t.Fatal("retry failed", e)
		}
	}
	first := f.steps[0]
	first.Now = core.Instant(math.Float64bits(math.NaN()))
	first.CommitIDs = CommitIDs{}
	f.steps = append(f.steps, first)
	if _, e := Replay(f.ctx, f.steps); e != nil {
		t.Fatal(e)
	}
}
func TestIndependentIdentityNamespacesAndRawInstantBits(t *testing.T) {
	f := newFixture(t, 1)
	negativeZero := core.Instant(math.Float64bits(math.Copysign(0, -1)))
	p, c, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, negativeZero)
	relation := f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category
	if relation.DecisionID != p.sealed.data.InteractionID || relation.CreatedAt != negativeZero || f.s.ChangeRecords[0].CommittedAt != negativeZero {
		t.Fatal("ID namespace or date bits changed")
	}
	// Numeric +0 equals -0 for clocks, but raw bits remain in the first facts.
	f.commit(t, 1, Choice{Kind: ChoiceExisting, CategoryID: p.sealed.data.PlannedCategoryID}, instant(0))
	if f.s.CurrentByChainID[core.TaskChainID(uuid(1))].Category.CreatedAt != negativeZero {
		t.Fatal("origin rewritten")
	}
	if _, _, e := Apply(f.ctx, f.s, p, c, instant(123), CommitIDs{}); e != nil {
		t.Fatal(e)
	}
	if _, e := Replay(f.ctx, f.steps); e != nil {
		t.Fatal(e)
	}
	// An invalid UUID slot fails; ID namespaces are not globally disjoint.
	req := Request{core.TaskChainID(uuid(1)), Choice{Kind: ChoiceNone}, uuid(555)}
	same := req.InteractionID
	plan, e := Prepare(f.ctx, f.s, req, PrepareIDs{PlanID: same})
	if e != nil {
		t.Fatal(e)
	}
	confirmation, e := ConfirmUser(plan, same)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Apply(f.ctx, f.s, plan, confirmation, instant(1), CommitIDs{RecordID: same, RemovalID: same}); e != nil {
		t.Fatal("invented global role uniqueness", e)
	}
	if _, e = Prepare(f.ctx, f.s, req, PrepareIDs{}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}

func TestEmptySliceRepresentationDoesNotChangeOriginalFacts(t *testing.T) {
	f := newFixture(t, 1)
	p, c, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(1))
	t.Run("retry", func(t *testing.T) {
		state := cloneState(f.s)
		state.ChangeRecords[0].Changes[0].Before.Labels = []PlanItem{}
		state.ChangeRecords[0].Changes[0].After.Labels = []PlanItem{}
		state.ChangeRecords[0].Notices = []Notice{}
		receipt := state.CommittedReceiptsByInteractionID[p.sealed.data.InteractionID]
		receipt.Notices = []Notice{}
		state.CommittedReceiptsByInteractionID[p.sealed.data.InteractionID] = receipt
		if e := ValidateApplyIntegrity(f.ctx, state); e != nil {
			t.Fatal("positive integrity", e)
		}
		if _, _, e := Apply(f.ctx, state, p, c, instant(2), CommitIDs{}); e != nil {
			t.Fatal("nil and [] encode the same original facts", e)
		}
	})
	t.Run("replay", func(t *testing.T) {
		steps := cloneSteps(f.steps)
		steps[0].Plan.Changes[0].Before.Labels = []PlanItem{}
		steps[0].Plan.Changes[0].After.Labels = []PlanItem{}
		steps[0].Plan.Notices = []Notice{}
		steps[0].Plan.Blockers = []string{}
		steps[0].Plan.PlannedLabelIDs = []UUID{}
		steps[0].Receipt.Notices = []Notice{}
		if _, e := Replay(f.ctx, steps); e != nil {
			t.Fatal("empty slice backing representation is not a fact", e)
		}
	})
}
