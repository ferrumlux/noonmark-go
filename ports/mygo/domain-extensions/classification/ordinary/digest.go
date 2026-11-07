// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// foundationWriter implements ONLY the fixed ASCII-key payloads below, not a
// generic JSON/canonical-v7 serializer. Slash escapes, raw non-ASCII (including
// U+2028/U+2029), short control escapes, omitted optionals and UUID wrappers are
// explicit. These are source-derived candidate bytes, NOT captured Swift proof.
type foundationWriter struct {
	out    []byte
	limit  int
	failed bool
}

func (w *foundationWriter) appendByte(c byte) {
	if w.limit > 0 && len(w.out) >= w.limit {
		w.failed = true
		return
	}
	w.out = append(w.out, c)
}
func (w *foundationWriter) appendString(s string) {
	if w.limit > 0 && len(s) > w.limit-len(w.out) {
		w.failed = true
		return
	}
	w.out = append(w.out, s...)
}
func (w *foundationWriter) String() string { return string(w.out) }
func newFoundationWriter(capacity int) foundationWriter {
	return foundationWriter{out: make([]byte, 0, capacity), limit: capacity}
}

func (w *foundationWriter) str(s string) {
	const digits = "0123456789abcdef"
	w.appendByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\', '/':
			w.appendByte('\\')
			w.appendByte(c)
		case '\b':
			w.appendString(`\b`)
		case '\f':
			w.appendString(`\f`)
		case '\n':
			w.appendString(`\n`)
		case '\r':
			w.appendString(`\r`)
		case '\t':
			w.appendString(`\t`)
		default:
			if c < 32 {
				w.appendString(`\u00`)
				w.appendByte(digits[c>>4])
				w.appendByte(digits[c&15])
			} else {
				w.appendByte(c)
			}
		}
	}
	w.appendByte('"')
}
func (w *foundationWriter) uint(n uint64) { w.appendString(strconv.FormatUint(n, 10)) }
func (w *foundationWriter) id(id string) {
	w.appendString(`{"rawValue":`)
	w.str(id)
	w.appendByte('}')
}
func (w *foundationWriter) source() { w.appendString(`{"userDirect":{}}`) }
func (w *foundationWriter) item(p PlanItem) {
	w.appendString(`{"colorHex":`)
	w.str(p.ColorHex)
	w.appendString(`,"id":`)
	w.str(string(p.ID))
	w.appendString(`,"kind":`)
	w.str(p.Kind)
	w.appendString(`,"name":`)
	w.str(p.Name)
	w.appendString(`,"resolution":`)
	w.str(p.Resolution)
	w.appendByte('}')
}
func (w *foundationWriter) preview(p SelectionPreview) {
	w.appendByte('{')
	if p.Category != nil {
		w.appendString(`"category":`)
		w.item(*p.Category)
		w.appendByte(',')
	}
	w.appendString(`"labels":[]}`)
}
func (w *foundationWriter) changes(changes []PlanChange) {
	w.appendByte('[')
	for i, c := range changes {
		if i > 0 {
			w.appendByte(',')
		}
		w.appendString(`{"setCurrent":{"after":`)
		w.preview(c.After)
		w.appendString(`,"before":`)
		w.preview(c.Before)
		w.appendString(`,"chainID":`)
		w.id(string(c.ChainID))
		w.appendString(`}}`)
	}
	w.appendByte(']')
}
func (w *foundationWriter) notices(notices []Notice) {
	w.appendByte('[')
	for i, n := range notices {
		if i > 0 {
			w.appendByte(',')
		}
		w.appendString(`{"existingItemReused":{"currentName":`)
		w.str(n.CurrentName)
		w.appendString(`,"inputName":`)
		w.str(n.InputName)
		w.appendString(`,"kind":"category","matchedHistoricalAlias":false}}`)
	}
	w.appendByte(']')
}
func planPayload(b *budget, p PlanData) ([]byte, error) {
	capacity, e := b.encodingCapacity(p)
	if e != nil {
		return nil, e
	}
	w := newFoundationWriter(capacity)
	w.appendString(`{"baseRevision":`)
	w.uint(p.BaseRevision)
	w.appendString(`,"blockers":[],"changes":`)
	w.changes(p.Changes)
	if p.CreatedItemID != "" {
		w.appendString(`,"createdItemID":`)
		w.str(string(p.CreatedItemID))
	}
	w.appendString(`,"formatVersion":1,"interactionID":`)
	w.str(string(p.InteractionID))
	w.appendString(`,"notices":`)
	w.notices(p.Notices)
	w.appendString(`,"planID":`)
	w.str(string(p.ID))
	if p.PlannedCategoryID != "" {
		w.appendString(`,"plannedCategoryID":`)
		w.id(string(p.PlannedCategoryID))
	}
	w.appendString(`,"plannedLabelIDs":[],"source":`)
	w.source()
	w.appendByte('}')
	if w.failed {
		return nil, ErrResourceLimit
	}
	return w.out, nil
}
func recordPayload(b *budget, r ChangeRecord) ([]byte, error) {
	capacity, e := b.encodingCapacity(r)
	if e != nil {
		return nil, e
	}
	w := newFoundationWriter(capacity)
	w.appendString(`{"changes":`)
	w.changes(r.Changes)
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
	w.notices(r.Notices)
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
func digest(bytes []byte) string { sum := sha256.Sum256(bytes); return hex.EncodeToString(sum[:]) }
func planDigest(b *budget, p PlanData) (string, error) {
	data, e := planPayload(b, p)
	if e != nil {
		return "", e
	}
	return digest(data), nil
}
func recordDigest(b *budget, r ChangeRecord) (string, error) {
	data, e := recordPayload(b, r)
	if e != nil {
		return "", e
	}
	return digest(data), nil
}
func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// CandidatePlanDigestPayload and CandidateRecordDigestPayload provide bounded
// offline evidence bytes. They confer no confirmation, replay, or write proof.
func CandidatePlanDigestPayload(p PlanData) ([]byte, error) {
	b, _ := newBudget(DefaultLimits())
	if e := b.scan(p); e != nil {
		return nil, e
	}
	if !validStrings(p) {
		return nil, ErrInvalid
	}
	if e := planShape(p); e != nil {
		return nil, e
	}
	return planPayload(b, p)
}
func CandidateRecordDigestPayload(r ChangeRecord) ([]byte, error) {
	b, _ := newBudget(DefaultLimits())
	if e := b.scan(r); e != nil {
		return nil, e
	}
	if !validStrings(r) {
		return nil, ErrInvalid
	}
	if e := recordShape(r); e != nil {
		return nil, e
	}
	return recordPayload(b, r)
}
