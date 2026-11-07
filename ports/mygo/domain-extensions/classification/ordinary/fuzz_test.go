// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"reflect"
	"testing"
)

func FuzzOrdinaryReplayDoesNotMutateInputs(f *testing.F) {
	for _, s := range []string{"工程", "E\u0301", "a\n\u0301b", "\u200b", "\xff", "Straße"} {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, name string, mode uint8) {
		if len(name) > 4096 {
			t.Skip()
		} // per-iteration harness cap; model has its own full budget
		ctx := contextFixture(t, 1)
		state := EmptyState()
		req := Request{ctx.sealed.graph.Chains[0].ID, Choice{Kind: ChoiceNew, Name: name}, uuid(200)}
		ids := PrepareIDs{uuid(201), CategoryID(uuid(202))}
		original := bytesOf(t, state)
		p, e := Prepare(ctx, state, req, ids)
		if bytesOf(t, state) != original {
			t.Fatal("prepare input mutated")
		}
		if e != nil {
			return
		}
		confirmation, e := ConfirmUser(p, req.InteractionID)
		if e != nil {
			t.Fatal(e)
		}
		commitIDs := CommitIDs{RecordID: uuid(203), NameVersionID: uuid(204)}
		next, receipt, e := Apply(ctx, state, p, confirmation, instant(1), commitIDs)
		if bytesOf(t, state) != original {
			t.Fatal("apply input mutated")
		}
		if e != nil {
			return
		}
		data, _ := p.Data()
		steps := []CommittedStep{{req, ids, data, req.InteractionID, instant(1), commitIDs, receipt}}
		proof, e := Replay(ctx, steps)
		if e != nil {
			t.Fatal("positive replay", e)
		}
		actual, _ := proof.State()
		if !reflect.DeepEqual(actual, next) {
			t.Fatal("replay differs")
		}
		if mode%2 == 1 {
			steps[0].Plan.Intent.Choice.Name += "x"
		}
		before := bytesOf(t, steps)
		_, _ = Replay(ctx, steps)
		if bytesOf(t, steps) != before {
			t.Fatal("replay input mutated")
		}
	})
}
func FuzzOrdinaryAuditRejectsTampering(f *testing.F) {
	for i := uint8(0); i < 8; i++ {
		f.Add(i, "工程")
	}
	f.Fuzz(func(t *testing.T, mode uint8, noise string) {
		if len(noise) > 1024 {
			t.Skip()
		}
		fx := auditedFixture(t)
		s := cloneState(fx.s)
		switch mode % 8 {
		case 0:
			s.Revision++
		case 1:
			s.RelationHistory = nil
		case 2:
			s.ChangeRecords[0].IntegrityDigest = "tampered" + noise
		case 3:
			s.ChangeRecords[0].Changes[0].After.Category.Kind = "label"
			rehashRecord(t, &s, 0)
		case 4:
			s.RelationHistory[0].OriginDecisionID = uuid(888)
		case 5:
			delete(s.CommittedReceiptsByInteractionID, s.ChangeRecords[0].InteractionID)
		case 6:
			s.ChangeRecords[0].Changes[0].After.Category.ID = CategoryID(uuid(888))
			rehashRecord(t, &s, 0)
		case 7:
			s.CurrentByChainID[fx.ctx.sealed.graph.Chains[1].ID] = Current{}
		}
		before := bytesOf(t, s)
		if e := ValidateApplyIntegrity(fx.ctx, s); e == nil {
			t.Fatal("accepted tampered positive control")
		}
		if bytesOf(t, s) != before {
			t.Fatal("validator mutated input")
		}
	})
}
