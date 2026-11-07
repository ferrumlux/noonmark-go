// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"strings"
)

// ValidateAggregate implements the supported ordinary portion of the pinned
// Swift entity Codable + state integrity contract. It does not replay history,
// require genesis, infer original plan intents, or apply runtime capture rules.
// Management, labels and non-userDirect sources are Unsupported, not evidence
// that such source states are corrupt. The wire adapter must reject omitted
// unsupported containers before calling this typed projection.
func ValidateAggregate(s AggregateState, limits Limits) error {
	b, e := aggregateEntry(s, limits)
	if e != nil {
		return e
	}
	return validateAggregate(b, s)
}
func aggregateEntry(s AggregateState, l Limits) (*budget, error) {
	b, e := newBudget(l)
	if e != nil {
		return nil, e
	}
	// Entire input before normalization, digesting, indexes or prospective copies.
	if e = b.scan(s); e != nil {
		return nil, e
	}
	if !validStrings(s) {
		return nil, ErrInvalid
	}
	return b, nil
}
func invalidAggregate(field string) error { return fmt.Errorf("%w: %s", ErrInvalid, field) }
func validateAggregate(b *budget, s AggregateState) error {
	if s.FormatVersion != FormatVersion {
		return invalidAggregate("format")
	}
	count := uint64(0)
	for _, r := range s.ChangeRecords {
		if r.Source != UserDirect {
			return ErrUnsupported
		}
		for _, c := range r.Changes {
			if c.Kind != "setCurrent" {
				return ErrUnsupported
			}
		}
		if len(r.Changes) > 0 {
			count++
		}
	}
	for _, es := range s.SnapshotEventsByTraceID {
		var e error
		count, e = add(count, uint64(len(es)))
		if e != nil {
			return e
		}
	}
	if count != s.Revision {
		return invalidAggregate("revision fact count")
	}
	// A second full scan prepays bounded linear indexes (keys and fact values).
	if e := b.scan(s); e != nil {
		return e
	}
	x := newAggregateIndex()
	for id, c := range s.Categories {
		if id != c.ID || !validUUID(id) {
			return invalidAggregate("category identity")
		}
		if e := validateAggregateCategory(b, c); e != nil {
			return e
		}
		if e := x.claim(b, c.CanonicalKey, c.CanonicalKeyVersion, id); e != nil {
			return e
		}
		for _, v := range c.NameVersions {
			if e := x.claim(b, v.CanonicalKey, v.CanonicalKeyVersion, id); e != nil {
				return e
			}
		}
	}
	for i, r := range s.ChangeRecords {
		if r.Source != UserDirect {
			return ErrUnsupported
		}
		if !validUUID(r.ID) || !validUUID(r.PlanID) || !validUUID(r.InteractionID) || !validUUID(r.DecisionID) || !r.CommittedAt.Finite() || !validDigest(r.PlanDigest) || r.Revision > s.Revision {
			return invalidAggregate("record fields")
		}
		if e := aggregateChangesShape(r.Changes); e != nil {
			return e
		}
		for _, n := range r.Notices {
			if n.Kind != "existingItemReused" || n.ItemKind != "category" {
				return ErrUnsupported
			}
		}
		if i > 0 && aggregateRecordLess(r, s.ChangeRecords[i-1]) {
			return invalidAggregate("record order")
		}
		if x.records[r.ID] || x.plans[r.PlanID] || x.interactions[r.InteractionID] {
			return invalidAggregate("record identity uniqueness")
		}
		x.records[r.ID], x.plans[r.PlanID], x.interactions[r.InteractionID] = true, true, true
		digest, e := aggregateRecordDigest(b, r)
		if e != nil {
			return e
		}
		if digest != r.IntegrityDigest {
			return invalidAggregate("record integrity digest")
		}
		receipt, ok := s.CommittedReceiptsByInteractionID[r.InteractionID]
		if !ok {
			return invalidAggregate("missing receipt")
		}
		match, e := aggregateReceiptMatches(b, receipt, r)
		if e != nil {
			return e
		}
		if !match {
			return invalidAggregate("receipt correspondence")
		}
		for _, c := range r.Changes {
			if id := aggregateItemID(c.After); id != "" {
				x.authority[aggregateAuthority{r.Revision, r.Source, r.DecisionID, c.ChainID, id, false}] = true
			}
			if id := aggregateItemID(c.Before); id != "" && id != aggregateItemID(c.After) {
				x.authority[aggregateAuthority{r.Revision, r.Source, r.DecisionID, c.ChainID, id, true}] = true
			}
		}
	}
	if len(s.CommittedReceiptsByInteractionID) != len(s.ChangeRecords) {
		return invalidAggregate("receipt index")
	}
	for chain, c := range s.CurrentByChainID {
		if !validUUID(chain) {
			return invalidAggregate("current chain")
		}
		if c.Category == nil {
			continue
		}
		r := c.Category
		if r.Source != UserDirect {
			return ErrUnsupported
		}
		category, ok := s.Categories[r.CategoryID]
		if !ok || category.Lifecycle == "merged" || !validUUID(r.DecisionID) || !r.CreatedAt.Finite() || !r.UpdatedAt.Finite() || r.UpdatedAt.Before(r.CreatedAt) || r.Revision > s.Revision {
			return invalidAggregate("current relation")
		}
		if !x.authority[aggregateAuthority{r.Revision, r.Source, r.DecisionID, chain, r.CategoryID, false}] {
			return invalidAggregate("current causal authority")
		}
	}
	for i, h := range s.RelationHistory {
		if h.Kind != "category" || h.OriginSource != UserDirect || h.RemovedBySource != UserDirect {
			return ErrUnsupported
		}
		if !validUUID(h.ID) || !validUUID(h.ChainID) || !aggregateRawUUID(h.ItemID) || !validUUID(h.OriginDecisionID) || !validUUID(h.RemovedByDecisionID) || x.history[h.ID] {
			return invalidAggregate("removal identity")
		}
		x.history[h.ID] = true
		if _, ok := s.Categories[CategoryID(strings.ToUpper(string(h.ItemID)))]; !ok {
			return invalidAggregate("removal category")
		}
		if !h.CreatedAt.Finite() || !h.RemovedAt.Finite() || h.RemovedAt.Before(h.CreatedAt) || h.CreatedRevision >= h.RemovedRevision || h.RemovedRevision > s.Revision {
			return invalidAggregate("removal timeline")
		}
		if i > 0 && aggregateHistoryLess(h, s.RelationHistory[i-1]) {
			return invalidAggregate("removal order")
		}
		if !x.authority[aggregateAuthority{h.CreatedRevision, h.OriginSource, h.OriginDecisionID, h.ChainID, h.ItemID, false}] || !x.authority[aggregateAuthority{h.RemovedRevision, h.RemovedBySource, h.RemovedByDecisionID, h.ChainID, h.ItemID, true}] {
			return invalidAggregate("removal causal authority")
		}
	}
	if len(s.SnapshotsByTraceID) != len(s.SnapshotEventsByTraceID) {
		return invalidAggregate("snapshot index keys")
	}
	for trace, events := range s.SnapshotEventsByTraceID {
		latest, ok := s.SnapshotsByTraceID[trace]
		if !validUUID(trace) || !ok || len(events) == 0 {
			return invalidAggregate("snapshot stream")
		}
		equal, e := aggregateEventEqual(b, latest, events[len(events)-1])
		if e != nil {
			return e
		}
		if !equal {
			return invalidAggregate("snapshot latest")
		}
		for i, event := range events {
			if len(event.Labels) != 0 {
				return ErrUnsupported
			}
			if event.TraceID != trace || !validUUID(event.ID) || x.events[event.ID] || !event.Status.Valid() || !event.CapturedAt.Finite() || event.Revision == 0 || event.Revision > s.Revision {
				return invalidAggregate("snapshot event")
			}
			x.events[event.ID] = true
			if i > 0 && events[i-1].Revision >= event.Revision {
				return invalidAggregate("snapshot revision order")
			}
			if c := event.Category; c != nil {
				if !validUUID(c.ID) || !aggregateApproval(c.PresentationApproval) {
					return invalidAggregate("historical category")
				}
				if _, ok := s.Categories[c.ID]; !ok {
					return invalidAggregate("historical category identity")
				}
				key, e := b.key(c.Name)
				if e != nil {
					return e
				}
				if e = x.claim(b, key, canonical16.AlgorithmVersion, c.ID); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func aggregateApproval(s string) bool { return s == "userApproved" || s == "pendingAIReview" }
func validateAggregateName(b *budget, name, key, version string) error {
	if name == "" || key == "" {
		return ErrName
	}
	if version != canonical16.AlgorithmVersion {
		return ErrUnsupported
	}
	d, e := b.display(name)
	if e != nil {
		return e
	}
	ok, e := b.equivalent(d, name)
	if e != nil {
		return e
	}
	if !ok {
		return ErrName
	}
	k, e := b.key(name)
	if e != nil {
		return e
	}
	ok, e = b.equivalent(k, key)
	if e != nil {
		return e
	}
	if !ok {
		return ErrName
	}
	return nil
}
func validateAggregateCategory(b *budget, c Category) error {
	if !aggregateApproval(c.PresentationApproval) {
		return invalidAggregate("presentation approval")
	}
	if c.Lifecycle == "merged" {
		return ErrUnsupported
	}
	if c.Lifecycle != "active" && c.Lifecycle != "archived" {
		return invalidAggregate("lifecycle")
	}
	if !c.CreatedAt.Finite() || !c.UpdatedAt.Finite() || c.UpdatedAt.Before(c.CreatedAt) || len(c.NameVersions) == 0 {
		return invalidAggregate("category timeline")
	}
	if e := validateAggregateName(b, c.Name, c.CanonicalKey, c.CanonicalKeyVersion); e != nil {
		return e
	}
	ids := map[UUID]bool{}
	for i, v := range c.NameVersions {
		if !validUUID(v.ID) || ids[v.ID] || !v.ValidFrom.Finite() {
			return invalidAggregate("name version identity/time")
		}
		ids[v.ID] = true
		if e := validateAggregateName(b, v.Name, v.CanonicalKey, v.CanonicalKeyVersion); e != nil {
			return e
		}
		if v.ValidUntil != nil && (!v.ValidUntil.Finite() || !v.ValidFrom.Before(*v.ValidUntil)) {
			return invalidAggregate("name interval")
		}
		if i < len(c.NameVersions)-1 {
			if v.ValidUntil == nil || !v.ValidUntil.Equal(c.NameVersions[i+1].ValidFrom) {
				return invalidAggregate("name interval continuity")
			}
		} else if v.ValidUntil != nil {
			return invalidAggregate("current name interval")
		}
	}
	first, last := c.NameVersions[0], c.NameVersions[len(c.NameVersions)-1]
	if !first.ValidFrom.Equal(c.CreatedAt) || c.UpdatedAt.Before(last.ValidFrom) || last.CanonicalKeyVersion != c.CanonicalKeyVersion {
		return invalidAggregate("name version boundary")
	}
	for _, pair := range [][2]string{{last.Name, c.Name}, {last.CanonicalKey, c.CanonicalKey}} {
		ok, e := b.equivalent(pair[0], pair[1])
		if e != nil {
			return e
		}
		if !ok {
			return invalidAggregate("current name version")
		}
	}
	return nil
}
func aggregateEventEqual(b *budget, a, c TraceClassificationSnapshot) (bool, error) {
	if a.ID != c.ID || a.TraceID != c.TraceID || a.Status != c.Status || !a.CapturedAt.Equal(c.CapturedAt) || a.Revision != c.Revision {
		return false, nil
	}
	if len(a.Labels) != 0 || len(c.Labels) != 0 {
		return false, ErrUnsupported
	}
	if a.Category == nil || c.Category == nil {
		return a.Category == nil && c.Category == nil, nil
	}
	if a.Category.ID != c.Category.ID || a.Category.PresentationApproval != c.Category.PresentationApproval {
		return false, nil
	}
	for _, pair := range [][2]string{{a.Category.Name, c.Category.Name}, {a.Category.ColorHex, c.Category.ColorHex}} {
		ok, e := b.equivalent(pair[0], pair[1])
		if e != nil || !ok {
			return false, e
		}
	}
	return true, nil
}

// Codable permits a setCurrent item to carry no identity. Such an item cannot
// account for any relation. This is source integrity, not command preparation.
func aggregateChangesShape(cs []AggregatePlanChange) error {
	for _, c := range cs {
		if c.Kind != "setCurrent" {
			return ErrUnsupported
		}
		if !validUUID(c.ChainID) {
			return invalidAggregate("change chain")
		}
		for _, p := range []AggregateSelectionPreview{c.Before, c.After} {
			if len(p.Labels) != 0 {
				return ErrUnsupported
			}
			if p.Category == nil {
				continue
			}
			i := p.Category
			if i.Kind != "category" {
				return ErrUnsupported
			}
			if i.Resolution != "existing" && i.Resolution != "new" && i.Resolution != "historicalAlias" {
				return invalidAggregate("preview resolution")
			}
		}
	}
	return nil
}

// History itemID is a raw Swift String parsed as UUID for identity lookup. Its
// original spelling remains significant for source audit authority and order.
func aggregateRawUUID(s CategoryID) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range []byte(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func aggregateItemID(p AggregateSelectionPreview) CategoryID {
	if p.Category == nil || p.Category.ID == nil {
		return ""
	}
	return CategoryID(*p.Category.ID)
}
func aggregateReceiptMatches(b *budget, receipt Receipt, r AggregateChangeRecord) (bool, error) {
	if receipt.PlanID != r.PlanID || receipt.Revision != r.Revision || receipt.ChangeRecordID != r.ID || receipt.DecisionID != r.DecisionID || receipt.ChangeRecordIntegrityDigest != r.IntegrityDigest {
		return false, nil
	}
	return noticesEqual(b, receipt.Notices, r.Notices)
}
