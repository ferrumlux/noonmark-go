// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
)

func Apply(ctx OrdinaryContext, s State, p Plan, confirmation Confirmation, now core.Instant, ids CommitIDs) (State, Receipt, error) {
	if p.sealed == nil || p.sealed.context != ctx.sealed {
		return State{}, Receipt{}, ErrContext
	}
	b, e := entry(ctx, s, p.sealed.request, p.sealed.ids, p.sealed.data, ids)
	if e != nil {
		return State{}, Receipt{}, e
	}
	return apply(b, ctx, s, p, confirmation, now, ids)
}
func apply(b *budget, ctx OrdinaryContext, s State, p Plan, confirmation Confirmation, now core.Instant, ids CommitIDs) (State, Receipt, error) {
	fail := func(e error) (State, Receipt, error) { return State{}, Receipt{}, e }
	if p.sealed == nil || ctx.sealed == nil || p.sealed.context != ctx.sealed {
		return fail(ErrContext)
	}
	if e := validateIntegrity(b, ctx, s); e != nil {
		return fail(e)
	}
	data := p.sealed.data
	if e := validatePlan(b, data); e != nil {
		return fail(e)
	}
	if confirmation.sealed == nil || confirmation.sealed.plan != p.sealed || !validUUID(confirmation.sealed.decisionID) {
		return fail(ErrConfirmation)
	}
	decision := confirmation.sealed.decisionID
	// Fact idempotence precedes stale and current preview recomputation. Do not
	// read/validate now or consume any ID slot on this path, including NaN/Inf now.
	if receipt, ok := s.CommittedReceiptsByInteractionID[data.InteractionID]; ok {
		if receipt.PlanID != data.ID || receipt.DecisionID != decision {
			return fail(ErrConflict)
		}
		for _, record := range s.ChangeRecords {
			if record.ID == receipt.ChangeRecordID {
				if record.InteractionID != data.InteractionID || record.PlanID != data.ID || record.PlanDigest != data.Digest || record.Source != data.Source || record.DecisionID != decision || data.BaseRevision == math.MaxUint64 || record.Revision != data.BaseRevision+1 || !sameChanges(record.Changes, data.Changes) || !sameNotices(record.Notices, data.Notices) {
					return fail(ErrConflict)
				}
				if e := b.scan(s, receipt); e != nil {
					return fail(e)
				}
				return cloneState(s), cloneReceipt(receipt), nil
			}
		}
		return fail(ErrInvalid)
	}
	if data.BaseRevision != s.Revision {
		return fail(ErrStale)
	}
	if e := validateExpectedCurrent(b, ctx, s, data); e != nil {
		return fail(e)
	}
	if !now.Finite() {
		return fail(ErrClock)
	}
	if s.Revision == math.MaxUint64 {
		return fail(ErrResourceLimit)
	}
	previous := s.CurrentByChainID[data.Intent.ChainID].Category
	if previous != nil && now.Before(previous.UpdatedAt) {
		return fail(ErrClock)
	}
	if !validUUID(ids.RecordID) {
		return fail(ErrInvalid)
	}
	for _, record := range s.ChangeRecords {
		if record.ID == ids.RecordID || record.PlanID == data.ID {
			return fail(ErrConflict)
		}
	}
	if e := b.charge(8192, 256); e != nil {
		return fail(e)
	} // all prospective one-commit wrappers
	// Resolve applies D again; TaskCategory initialization applies D once more.
	// It deliberately does not run ValidateDecodedEntities or repair a name.
	var created *Category
	selected := data.Intent.Choice.CategoryID
	switch data.Intent.Choice.Kind {
	case ChoiceNone:
		selected = ""
	case ChoiceExisting:
	case ChoiceNew:
		name, e := b.normalized(data.Intent.Choice.Name)
		if e != nil {
			return fail(e)
		}
		key, e := b.key(name)
		if e != nil {
			return fail(e)
		}
		found, ok, e := lookup(b, s, key)
		if e != nil {
			return fail(e)
		}
		if ok {
			selected = found.ID
		} else {
			if _, occupied := s.Categories[data.PlannedCategoryID]; occupied {
				return fail(ErrConflict)
			}
			if !validUUID(ids.NameVersionID) {
				return fail(ErrInvalid)
			}
			name, e = b.display(name)
			if e != nil {
				return fail(e)
			}
			key, e = b.key(name)
			if e != nil {
				return fail(e)
			}
			c := Category{ID: data.PlannedCategoryID, Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, ColorHex: data.Intent.Choice.ColorHex, PresentationApproval: "userApproved", Lifecycle: "active", CreatedAt: now, UpdatedAt: now, NameVersions: []NameVersion{{ID: ids.NameVersionID, Name: name, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, ValidFrom: now}}}
			created = &c
			selected = c.ID
		}
	}
	removalNeeded := previous != nil && previous.CategoryID != selected
	if removalNeeded {
		if !validUUID(ids.RemovalID) {
			return fail(ErrInvalid)
		}
		for _, h := range s.RelationHistory {
			if h.ID == ids.RemovalID {
				return fail(ErrConflict)
			}
		}
	}
	// Prepay complete owned candidate + maximal one-commit growth before cloning.
	if e := b.scan(s, data.Changes, data.Notices, created); e != nil {
		return fail(e)
	}
	next := cloneState(s)
	next.Revision++
	if created != nil {
		next.Categories[created.ID] = *created
	}
	relation := clonePointer(previous)
	if removalNeeded {
		next.RelationHistory = append(next.RelationHistory, RelationHistory{ID: ids.RemovalID, Kind: "category", ChainID: data.Intent.ChainID, ItemID: previous.CategoryID, OriginSource: previous.Source, OriginDecisionID: previous.DecisionID, CreatedAt: previous.CreatedAt, CreatedRevision: previous.Revision, RemovedBySource: UserDirect, RemovedByDecisionID: decision, RemovedAt: now, RemovedRevision: next.Revision})
	}
	if previous == nil || previous.CategoryID != selected {
		relation = nil
		if selected != "" {
			relation = &CategoryRelation{CategoryID: selected, Source: UserDirect, DecisionID: decision, CreatedAt: now, UpdatedAt: now, Revision: next.Revision}
		}
	}
	next.CurrentByChainID[data.Intent.ChainID] = Current{Category: relation}
	record := ChangeRecord{ID: ids.RecordID, PlanID: data.ID, InteractionID: data.InteractionID, Source: UserDirect, DecisionID: decision, Changes: cloneChanges(data.Changes), Notices: cloneNotices(data.Notices), CommittedAt: now, Revision: next.Revision, PlanDigest: data.Digest}
	integrityDigest, e := recordDigest(b, record)
	record.IntegrityDigest = integrityDigest
	if e != nil {
		return fail(e)
	}
	receipt := Receipt{PlanID: record.PlanID, Revision: record.Revision, Notices: cloneNotices(record.Notices), ChangeRecordID: record.ID, DecisionID: decision, ChangeRecordIntegrityDigest: record.IntegrityDigest}
	next.ChangeRecords = append(next.ChangeRecords, record)
	next.CommittedReceiptsByInteractionID[data.InteractionID] = cloneReceipt(receipt)
	if e = validateIntegrity(b, ctx, next); e != nil {
		return fail(e)
	}
	return next, receipt, nil
}
