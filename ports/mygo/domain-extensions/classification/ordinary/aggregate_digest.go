// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

func aggregateRecordPayload(b *budget, r AggregateChangeRecord) ([]byte, error) {
	capacity, e := b.encodingCapacity(r)
	if e != nil {
		return nil, e
	}
	w := newFoundationWriter(capacity)
	w.appendString(`{"changes":`)
	aggregateWriteChanges(&w, r.Changes)
	w.appendString(`,"committedAtBitPattern":`)
	w.uint(uint64(r.CommittedAt))
	if r.DecisionID != "" {
		w.appendString(`,"decisionID":`)
		w.str(string(r.DecisionID))
	}
	w.appendString(`,"formatVersion":1,"id":`)
	w.str(string(r.ID))
	w.appendString(`,"interactionID":`)
	w.str(string(r.InteractionID))
	w.appendString(`,"notices":`)
	aggregateWriteNotices(&w, r.Notices)
	w.appendString(`,"planDigest":`)
	w.str(r.PlanDigest)
	w.appendString(`,"planID":`)
	w.str(string(r.PlanID))
	w.appendString(`,"revision":`)
	w.uint(r.Revision)
	w.appendString(`,"source":`)
	w.source()
	w.appendByte('}')
	if w.failed {
		return nil, ErrResourceLimit
	}
	return w.out, nil
}
func aggregateRecordDigest(b *budget, r AggregateChangeRecord) (string, error) {
	p, e := aggregateRecordPayload(b, r)
	if e != nil {
		return "", e
	}
	return digest(p), nil
}
func aggregateWriteNotices(w *foundationWriter, ns []Notice) {
	w.appendByte('[')
	for i, n := range ns {
		if i > 0 {
			w.appendByte(',')
		}
		w.appendString(`{"existingItemReused":{"currentName":`)
		w.str(n.CurrentName)
		w.appendString(`,"inputName":`)
		w.str(n.InputName)
		w.appendString(`,"kind":"category","matchedHistoricalAlias":`)
		if n.MatchedHistoricalAlias {
			w.appendString("true")
		} else {
			w.appendString("false")
		}
		w.appendString("}}")
	}
	w.appendByte(']')
}
func aggregateWriteItem(w *foundationWriter, p AggregatePlanItem) {
	w.appendString(`{"colorHex":`)
	w.str(p.ColorHex)
	if p.ID != nil {
		w.appendString(`,"id":`)
		w.str(*p.ID)
	}
	w.appendString(`,"kind":`)
	w.str(p.Kind)
	w.appendString(`,"name":`)
	w.str(p.Name)
	w.appendString(`,"resolution":`)
	w.str(p.Resolution)
	w.appendByte('}')
}
func aggregateWritePreview(w *foundationWriter, p AggregateSelectionPreview) {
	w.appendByte('{')
	if p.Category != nil {
		w.appendString(`"category":`)
		aggregateWriteItem(w, *p.Category)
		w.appendByte(',')
	}
	w.appendString(`"labels":[]}`)
}
func aggregateWriteChanges(w *foundationWriter, cs []AggregatePlanChange) {
	w.appendByte('[')
	for i, c := range cs {
		if i > 0 {
			w.appendByte(',')
		}
		w.appendString(`{"setCurrent":{"after":`)
		aggregateWritePreview(w, c.After)
		w.appendString(`,"before":`)
		aggregateWritePreview(w, c.Before)
		w.appendString(`,"chainID":`)
		w.id(string(c.ChainID))
		w.appendString("}}")
	}
	w.appendByte(']')
}
