// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

// FromOrdinaryState lifts R4-shaped facts without granting admission/authority.
// It is only a convenience for constructing/testing new typed command output.
// There is intentionally no inverse: source optional/raw IDs are not losslessly
// representable by the frozen R4 command types.
func FromOrdinaryState(s State, l Limits) (AggregateState, error) {
	b, e := newBudget(l)
	if e != nil {
		return AggregateState{}, e
	}
	if e = b.scan(s, s); e != nil {
		return AggregateState{}, e
	}
	if !validStrings(s) {
		return AggregateState{}, ErrInvalid
	}
	// Prepay optional ID pointers absent in the old model before conversion.
	for _, r := range s.ChangeRecords {
		if e = aggregatePrepayRecordConversion(b, r); e != nil {
			return AggregateState{}, e
		}
	}
	out := AggregateState{FormatVersion: s.FormatVersion, Revision: s.Revision, Categories: s.Categories, CurrentByChainID: s.CurrentByChainID, RelationHistory: s.RelationHistory, CommittedReceiptsByInteractionID: s.CommittedReceiptsByInteractionID}
	if s.ChangeRecords != nil {
		out.ChangeRecords = make([]AggregateChangeRecord, len(s.ChangeRecords))
		for i, r := range s.ChangeRecords {
			out.ChangeRecords[i] = aggregateFromRecord(r)
		}
	}
	// The conversion output owns all maps, pointers and slices. Clone's separate
	// budget is bounded by the same limits; total is a fixed two-entry bound.
	return CloneAggregate(out, l)
}

// RecordFromOrdinary is the bounded one-way conversion for a new ordinary
// command record. It cannot convert imported optional IDs back to command IDs.
func RecordFromOrdinary(r ChangeRecord, l Limits) (AggregateChangeRecord, error) {
	b, e := newBudget(l)
	if e != nil {
		return AggregateChangeRecord{}, e
	}
	if e = b.scan(r, r); e != nil {
		return AggregateChangeRecord{}, e
	}
	if !validStrings(r) {
		return AggregateChangeRecord{}, ErrInvalid
	}
	if e = aggregatePrepayRecordConversion(b, r); e != nil {
		return AggregateChangeRecord{}, e
	}
	return aggregateFromRecord(r), nil
}
func aggregateFromRecord(r ChangeRecord) AggregateChangeRecord {
	out := AggregateChangeRecord{ID: r.ID, PlanID: r.PlanID, InteractionID: r.InteractionID, Source: r.Source, DecisionID: r.DecisionID, Notices: aggregateSlice(r.Notices), CommittedAt: r.CommittedAt, Revision: r.Revision, PlanDigest: r.PlanDigest, IntegrityDigest: r.IntegrityDigest}
	if r.Changes != nil {
		out.Changes = make([]AggregatePlanChange, len(r.Changes))
		for i, c := range r.Changes {
			out.Changes[i] = AggregatePlanChange{Kind: c.Kind, ChainID: c.ChainID, Before: aggregateFromPreview(c.Before), After: aggregateFromPreview(c.After)}
		}
	}
	return out
}
func aggregateFromPreview(p SelectionPreview) AggregateSelectionPreview {
	out := AggregateSelectionPreview{}
	if p.Category != nil {
		v := aggregateFromItem(*p.Category)
		out.Category = &v
	}
	if p.Labels != nil {
		out.Labels = make([]AggregatePlanItem, len(p.Labels))
		for i, p := range p.Labels {
			out.Labels[i] = aggregateFromItem(p)
		}
	}
	return out
}
func aggregateFromItem(p PlanItem) AggregatePlanItem {
	v := AggregatePlanItem{Kind: p.Kind, Name: p.Name, ColorHex: p.ColorHex, Resolution: p.Resolution}
	if p.ID != "" {
		id := string(p.ID)
		v.ID = &id
	}
	return v
}

func aggregatePrepayRecordConversion(b *budget, r ChangeRecord) error {
	for _, c := range r.Changes {
		n := uint64(2) + uint64(len(c.Before.Labels)) + uint64(len(c.After.Labels))
		// scan already bounded all lengths; checked multiply still precedes growth.
		bytes, e := mul(n, 32)
		if e != nil {
			return e
		}
		nodes, e := mul(n, 2)
		if e != nil {
			return e
		}
		if e = b.charge(bytes, nodes); e != nil {
			return e
		}
	}
	return nil
}
