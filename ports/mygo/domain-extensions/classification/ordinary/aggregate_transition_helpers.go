// SPDX-License-Identifier: AGPL-3.0-only
// Derived from Noonmark 43e6eefaae3a04be89cab3e234d470111afd80ef.
package ordinary

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
)

// These pure helpers use the complete aggregate validator, not R4's fixed
// graph proof. They convey no Service ownership or persistence capability.
// The supported transition profile is userDirect with unmanaged categories.
func aggregateTransitionContext(s AggregateState, graph core.TaskGraph, limits Limits) (OrdinaryContext, *budget, error) {
	if err := ValidateAggregate(s, limits); err != nil {
		return OrdinaryContext{}, nil, err
	}
	if err := ValidateAggregateReferences(s, graph, limits); err != nil {
		return OrdinaryContext{}, nil, err
	}
	b, err := newBudget(limits)
	if err != nil {
		return OrdinaryContext{}, nil, err
	}
	if err = b.scan(s, graph); err != nil {
		return OrdinaryContext{}, nil, err
	}
	// Prepay the transient record-metadata workspace before allocation.
	if err = b.charge(uint64(len(s.ChangeRecords))*256, uint64(len(s.ChangeRecords))*16); err != nil {
		return OrdinaryContext{}, nil, err
	}
	if err = core.ValidateTaskGraph(graph); err != nil {
		return OrdinaryContext{}, nil, err
	}
	for _, c := range s.Categories {
		if c.Lifecycle != "active" || c.PresentationApproval != "userApproved" || len(c.NameVersions) != 1 {
			return OrdinaryContext{}, nil, ErrUnsupported
		}
	}
	if err = b.charge(uint64(len(graph.Chains))*64, uint64(len(graph.Chains))); err != nil {
		return OrdinaryContext{}, nil, err
	}
	chains := make(map[core.TaskChainID]struct{}, len(graph.Chains))
	for _, c := range graph.Chains {
		chains[c.ID] = struct{}{}
	}
	return OrdinaryContext{&contextData{chains: chains, limits: b.lim}}, b, nil
}
func PrepareAggregatePlan(s AggregateState, graph core.TaskGraph, r Request, ids PrepareIDs, limits Limits) (PlanData, error) {
	ctx, b, err := aggregateTransitionContext(s, graph, limits)
	if err != nil {
		return PlanData{}, err
	}
	if err = b.scan(r, ids); err != nil {
		return PlanData{}, err
	}
	if !validStrings(r, ids) {
		return PlanData{}, ErrInvalid
	}
	p, err := prepareData(b, ctx, aggregateCommandWorkspace(s, nil), r, ids)
	if err != nil {
		return PlanData{}, err
	}
	if err = b.scan(p); err != nil {
		return PlanData{}, err
	}
	return p, nil
}
func ValidateAggregatePlan(p PlanData, limits Limits) error {
	b, err := newBudget(limits)
	if err != nil {
		return err
	}
	if err = b.scan(p); err != nil {
		return err
	}
	if !validStrings(p) {
		return ErrInvalid
	}
	return validatePlan(b, p)
}
func ApplyAggregatePlan(s AggregateState, graph core.TaskGraph, p PlanData, decision UUID, now core.Instant, ids CommitIDs, limits Limits) (AggregateState, Receipt, error) {
	ctx, b, err := aggregateTransitionContext(s, graph, limits)
	if err != nil {
		return AggregateState{}, Receipt{}, err
	}
	if err = b.scan(p, decision, ids); err != nil {
		return AggregateState{}, Receipt{}, err
	}
	if !validStrings(p, decision, ids) {
		return AggregateState{}, Receipt{}, ErrInvalid
	}
	if receipt, ok := s.CommittedReceiptsByInteractionID[p.InteractionID]; ok {
		for _, record := range s.ChangeRecords {
			if record.ID == receipt.ChangeRecordID && !aggregateChangesMatchPlan(record.Changes, p.Changes) {
				return AggregateState{}, Receipt{}, ErrConflict
			}
		}
	}
	next, receipt, err := applyAggregateState(b, ctx, aggregateCommandWorkspace(s, &p), p, decision, now, ids)
	if err != nil {
		return AggregateState{}, Receipt{}, err
	}
	out, err := CloneAggregate(s, limits)
	if err != nil {
		return AggregateState{}, Receipt{}, err
	}
	out.FormatVersion = next.FormatVersion
	out.Revision = next.Revision
	out.Categories = next.Categories
	out.CurrentByChainID = next.CurrentByChainID
	out.RelationHistory = next.RelationHistory
	out.CommittedReceiptsByInteractionID = next.CommittedReceiptsByInteractionID
	// Old aggregate records (including absent versus present-empty raw item IDs)
	// remain byte-for-byte facts. Only the actual new command record is lifted.
	if len(next.ChangeRecords) > len(s.ChangeRecords) {
		r, err := RecordFromOrdinary(next.ChangeRecords[len(next.ChangeRecords)-1], limits)
		if err != nil {
			return AggregateState{}, Receipt{}, err
		}
		out.ChangeRecords = append(out.ChangeRecords, r)
	}
	if err = ValidateAggregate(out, limits); err != nil {
		return AggregateState{}, Receipt{}, err
	}
	return out, receipt, nil
}
func applyAggregateState(b *budget, ctx OrdinaryContext, s State, plan PlanData, decision UUID, now core.Instant, ids CommitIDs) (State, Receipt, error) {
	fail := func(err error) (State, Receipt, error) { return State{}, Receipt{}, err }
	data := plan
	if e := validatePlan(b, data); e != nil {
		return fail(e)
	}
	if !validUUID(decision) {
		return fail(ErrConfirmation)
	}
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
	return next, receipt, nil
}

// This deliberately incomplete command workspace is NOT a representation or
// reverse codec of AggregateState. Imported record previews are never projected
// into R4's lossy type; only metadata needed for collision checks is copied.
// A retry's previews are supplied from its original plan only after exact full
// aggregate preview matching above. No workspace record is persisted.
func aggregateCommandWorkspace(s AggregateState, p *PlanData) State {
	out := State{FormatVersion: s.FormatVersion, Revision: s.Revision, Categories: s.Categories, CurrentByChainID: s.CurrentByChainID, RelationHistory: s.RelationHistory, CommittedReceiptsByInteractionID: s.CommittedReceiptsByInteractionID}
	out.ChangeRecords = make([]ChangeRecord, len(s.ChangeRecords))
	for i, r := range s.ChangeRecords {
		v := ChangeRecord{ID: r.ID, PlanID: r.PlanID, InteractionID: r.InteractionID, Source: r.Source, DecisionID: r.DecisionID, Notices: r.Notices, CommittedAt: r.CommittedAt, Revision: r.Revision, PlanDigest: r.PlanDigest, IntegrityDigest: r.IntegrityDigest}
		if p != nil && r.InteractionID == p.InteractionID {
			v.Changes = p.Changes
		}
		out.ChangeRecords[i] = v
	}
	return out
}
func aggregateChangesMatchPlan(a []AggregatePlanChange, b []PlanChange) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		y := b[i]
		if x.Kind != y.Kind || x.ChainID != y.ChainID || !aggregatePreviewMatchesPlan(x.Before, y.Before) || !aggregatePreviewMatchesPlan(x.After, y.After) {
			return false
		}
	}
	return true
}
func aggregatePreviewMatchesPlan(a AggregateSelectionPreview, b SelectionPreview) bool {
	if len(a.Labels) != 0 || len(b.Labels) != 0 {
		return false
	}
	if a.Category == nil || b.Category == nil {
		return a.Category == nil && b.Category == nil
	}
	x, y := a.Category, b.Category
	if x.ID == nil || *x.ID != string(y.ID) {
		return false
	}
	return x.Kind == y.Kind && x.Name == y.Name && x.ColorHex == y.ColorHex && x.Resolution == y.Resolution
}
