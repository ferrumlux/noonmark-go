// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func choiceShape(c Choice) error {
	switch c.Kind {
	case ChoiceNone:
		if c.CategoryID != "" || c.Name != "" || c.ColorHex != "" {
			return ErrInvalid
		}
	case ChoiceExisting:
		if !validUUID(c.CategoryID) || c.Name != "" || c.ColorHex != "" {
			return ErrInvalid
		}
	case ChoiceNew:
		if c.CategoryID != "" {
			return ErrInvalid
		}
	default:
		return ErrUnsupported
	}
	return nil
}
func requestShape(r Request) error {
	if !validUUID(r.ChainID) || !validUUID(r.InteractionID) {
		return ErrInvalid
	}
	return choiceShape(r.Choice)
}
func previewShape(p SelectionPreview, before bool) error {
	if len(p.Labels) != 0 {
		return ErrUnsupported
	}
	if p.Category == nil {
		return nil
	}
	i := p.Category
	if !validUUID(i.ID) || i.Kind != "category" {
		return ErrUnsupported
	}
	if i.Resolution != "existing" && (before || i.Resolution != "new") {
		return ErrUnsupported
	}
	return nil
}
func changesShape(cs []PlanChange) error {
	if len(cs) != 1 {
		return ErrUnsupported
	}
	c := cs[0]
	if c.Kind != "setCurrent" || !validUUID(c.ChainID) {
		return ErrUnsupported
	}
	if e := previewShape(c.Before, true); e != nil {
		return e
	}
	return previewShape(c.After, false)
}
func noticesShape(ns []Notice) error {
	if len(ns) > 1 {
		return ErrUnsupported
	}
	for _, n := range ns {
		if n.Kind != "existingItemReused" || n.ItemKind != "category" || n.MatchedHistoricalAlias {
			return ErrUnsupported
		}
	}
	return nil
}
func planShape(p PlanData) error {
	if !validUUID(p.ID) || !validUUID(p.InteractionID) || p.Source != UserDirect {
		return ErrInvalid
	}
	if p.CreatedItemID != "" || len(p.PlannedLabelIDs) != 0 || len(p.Blockers) != 0 {
		return ErrUnsupported
	}
	if e := requestShape(p.Intent); e != nil {
		return e
	}
	if p.Intent.InteractionID != p.InteractionID {
		return ErrInvalid
	}
	if e := changesShape(p.Changes); e != nil {
		return e
	}
	if p.Changes[0].ChainID != p.Intent.ChainID {
		return ErrInvalid
	}
	if e := noticesShape(p.Notices); e != nil {
		return e
	}
	if p.Intent.Choice.Kind == ChoiceNew {
		if !validUUID(p.PlannedCategoryID) {
			return ErrInvalid
		}
	} else if p.PlannedCategoryID != "" {
		return ErrInvalid
	}
	return nil
}
func recordShape(r ChangeRecord) error {
	if !validUUID(r.ID) || !validUUID(r.PlanID) || !validUUID(r.InteractionID) || !validUUID(r.DecisionID) || r.Source != UserDirect || !r.CommittedAt.Finite() || !validDigest(r.PlanDigest) {
		return ErrInvalid
	}
	if e := changesShape(r.Changes); e != nil {
		return e
	}
	return noticesShape(r.Notices)
}
func categoryShape(c Category) error {
	if !validUUID(c.ID) || c.CanonicalKeyVersion != canonical16.AlgorithmVersion || c.Lifecycle != "active" || c.PresentationApproval != "userApproved" || len(c.NameVersions) != 1 {
		return ErrUnsupported
	}
	v := c.NameVersions[0]
	if !validUUID(v.ID) || v.ValidUntil != nil || v.CanonicalKeyVersion != canonical16.AlgorithmVersion {
		return ErrUnsupported
	}
	if !c.CreatedAt.Finite() || !c.UpdatedAt.Finite() || !v.ValidFrom.Finite() || c.UpdatedAt.Before(c.CreatedAt) || !v.ValidFrom.Equal(c.CreatedAt) {
		return ErrInvalid
	}
	return nil
}
func itemID(p SelectionPreview) CategoryID {
	if p.Category == nil {
		return ""
	}
	return p.Category.ID
}
func currentID(c Current) CategoryID {
	if c.Category == nil {
		return ""
	}
	return c.Category.CategoryID
}
func equalRelation(a, c *CategoryRelation) bool {
	if a == nil || c == nil {
		return a == nil && c == nil
	}
	return a.CategoryID == c.CategoryID && a.Source == c.Source && a.DecisionID == c.DecisionID && a.CreatedAt.Equal(c.CreatedAt) && a.UpdatedAt.Equal(c.UpdatedAt) && a.Revision == c.Revision
}
func equalHistory(a, c RelationHistory) bool {
	return a.Kind == c.Kind && a.ChainID == c.ChainID && a.ItemID == c.ItemID && a.OriginSource == c.OriginSource && a.OriginDecisionID == c.OriginDecisionID && a.CreatedAt.Equal(c.CreatedAt) && a.CreatedRevision == c.CreatedRevision && a.RemovedBySource == c.RemovedBySource && a.RemovedByDecisionID == c.RemovedByDecisionID && a.RemovedAt.Equal(c.RemovedAt) && a.RemovedRevision == c.RemovedRevision
}
func noticesEqual(b *budget, a, c []Notice) (bool, error) {
	if len(a) != len(c) {
		return false, nil
	}
	for i, n := range a {
		m := c[i]
		if n.Kind != m.Kind || n.ItemKind != m.ItemKind || n.MatchedHistoricalAlias != m.MatchedHistoricalAlias {
			return false, nil
		}
		for _, pair := range [][2]string{{n.InputName, m.InputName}, {n.CurrentName, m.CurrentName}} {
			ok, e := b.equivalent(pair[0], pair[1])
			if e != nil || !ok {
				return false, e
			}
		}
	}
	return true, nil
}
func receiptMatches(b *budget, receipt Receipt, r ChangeRecord) (bool, error) {
	if receipt.PlanID != r.PlanID || receipt.Revision != r.Revision || receipt.ChangeRecordID != r.ID || receipt.DecisionID != r.DecisionID || receipt.ChangeRecordIntegrityDigest != r.IntegrityDigest {
		return false, nil
	}
	return noticesEqual(b, receipt.Notices, r.Notices)
}

// ValidateApplyIntegrity checks the applicable Swift integrity invariants plus
// this narrower ordinary shape and relation coverage. It does NOT run Codable
// entity decoding and does NOT prove that catalog creation had retained plans.
func ValidateApplyIntegrity(ctx OrdinaryContext, state State) error {
	b, e := entry(ctx, state)
	if e != nil {
		return e
	}
	return validateIntegrity(b, ctx, state)
}
func validateIntegrity(b *budget, ctx OrdinaryContext, s State) error {
	if s.FormatVersion != FormatVersion || s.Revision != uint64(len(s.ChangeRecords)) {
		return ErrInvalid
	}
	// Prepay indexes and reconstructed relation output before creating any maps.
	if e := b.scan(s); e != nil {
		return e
	}
	owners := map[string]CategoryID{}
	for id, c := range s.Categories {
		if id != c.ID {
			return ErrInvalid
		}
		if e := categoryShape(c); e != nil {
			return e
		}
		v := c.NameVersions[0]
		for _, pair := range [][2]string{{c.Name, v.Name}, {c.CanonicalKey, v.CanonicalKey}} {
			ok, e := b.equivalent(pair[0], pair[1])
			if e != nil {
				return e
			}
			if !ok {
				return ErrInvalid
			}
		}
		key, e := b.nfc(c.CanonicalKey)
		if e != nil {
			return e
		}
		if owner, ok := owners[key]; ok && owner != id {
			return ErrConflict
		}
		owners[key] = id
	}
	records, plans, interactions := map[UUID]bool{}, map[UUID]bool{}, map[UUID]bool{}
	expected := map[core.TaskChainID]Current{}
	removed := map[uint64]RelationHistory{}
	if len(s.CommittedReceiptsByInteractionID) != len(s.ChangeRecords) {
		return ErrInvalid
	}
	for i, r := range s.ChangeRecords {
		if e := recordShape(r); e != nil {
			return e
		}
		if r.Revision != uint64(i)+1 || records[r.ID] || plans[r.PlanID] || interactions[r.InteractionID] {
			return ErrConflict
		}
		records[r.ID], plans[r.PlanID], interactions[r.InteractionID] = true, true, true
		want, e := recordDigest(b, r)
		if e != nil {
			return e
		}
		if r.IntegrityDigest != want {
			return ErrInvalid
		}
		receipt, ok := s.CommittedReceiptsByInteractionID[r.InteractionID]
		if !ok {
			return ErrInvalid
		}
		match, e := receiptMatches(b, receipt, r)
		if e != nil {
			return e
		}
		if !match {
			return ErrInvalid
		}
		c := r.Changes[0]
		if !ctx.hasChain(c.ChainID) {
			return ErrContext
		}
		for _, p := range []SelectionPreview{c.Before, c.After} {
			if p.Category != nil {
				category, ok := s.Categories[p.Category.ID]
				if !ok {
					return ErrInvalid
				}
				if p.Category.Resolution == "existing" {
					eq, e := b.equivalent(p.Category.Name, category.Name)
					if e != nil {
						return e
					}
					if !eq {
						return ErrInvalid
					}
					eq, e = b.equivalent(p.Category.ColorHex, category.ColorHex)
					if e != nil {
						return e
					}
					if !eq {
						return ErrInvalid
					}
				}
			}
		}
		if len(r.Notices) > 0 {
			if c.After.Category == nil || c.After.Category.Resolution != "existing" {
				return ErrInvalid
			}
			eq, e := b.equivalent(r.Notices[0].CurrentName, c.After.Category.Name)
			if e != nil {
				return e
			}
			if !eq {
				return ErrInvalid
			}
		}
		old := expected[c.ChainID]
		if itemID(c.Before) != currentID(old) {
			return ErrInvalid
		}
		if old.Category != nil && r.CommittedAt.Before(old.Category.UpdatedAt) {
			return ErrClock
		}
		after := itemID(c.After)
		if currentID(old) != after {
			if old.Category != nil {
				p := old.Category
				removed[r.Revision] = RelationHistory{Kind: "category", ChainID: c.ChainID, ItemID: p.CategoryID, OriginSource: p.Source, OriginDecisionID: p.DecisionID, CreatedAt: p.CreatedAt, CreatedRevision: p.Revision, RemovedBySource: r.Source, RemovedByDecisionID: r.DecisionID, RemovedAt: r.CommittedAt, RemovedRevision: r.Revision}
			}
			old.Category = nil
			if after != "" {
				old.Category = &CategoryRelation{CategoryID: after, Source: r.Source, DecisionID: r.DecisionID, CreatedAt: r.CommittedAt, UpdatedAt: r.CommittedAt, Revision: r.Revision}
			}
		}
		expected[c.ChainID] = old
	}
	if len(expected) != len(s.CurrentByChainID) || len(removed) != len(s.RelationHistory) {
		return ErrInvalid
	}
	for id, c := range s.CurrentByChainID {
		if !ctx.hasChain(id) {
			return ErrContext
		}
		x, ok := expected[id]
		if !ok || !equalRelation(x.Category, c.Category) {
			return ErrInvalid
		}
	}
	historyIDs := map[UUID]bool{}
	var previous uint64
	for _, h := range s.RelationHistory {
		if !validUUID(h.ID) || historyIDs[h.ID] || h.Kind != "category" || !ctx.hasChain(h.ChainID) || h.OriginSource != UserDirect || h.RemovedBySource != UserDirect || !validUUID(h.OriginDecisionID) || !validUUID(h.RemovedByDecisionID) || !h.CreatedAt.Finite() || !h.RemovedAt.Finite() || h.RemovedAt.Before(h.CreatedAt) || h.CreatedRevision >= h.RemovedRevision || h.RemovedRevision <= previous {
			return ErrInvalid
		}
		expected, ok := removed[h.RemovedRevision]
		if !ok || !equalHistory(h, expected) {
			return ErrInvalid
		}
		historyIDs[h.ID] = true
		previous = h.RemovedRevision
	}
	return nil
}

// ValidateDecodedEntities adds TC:146-213's entity Codable constraints. Swift
// commit/integrity can succeed for a non-idempotent D name that decoding rejects;
// this distinction is intentional. Original bytes are never repaired.
func ValidateDecodedEntities(ctx OrdinaryContext, state State) error {
	b, e := entry(ctx, state)
	if e != nil {
		return e
	}
	if e = validateIntegrity(b, ctx, state); e != nil {
		return e
	}
	return validateDecodedEntities(b, state)
}
func validateDecodedEntities(b *budget, s State) error {
	for _, c := range s.Categories {
		v := c.NameVersions[0]
		for _, item := range [][2]string{{c.Name, c.CanonicalKey}, {v.Name, v.CanonicalKey}} {
			if item[0] == "" || item[1] == "" {
				return ErrName
			}
			d, e := b.display(item[0])
			if e != nil {
				return e
			}
			ok, e := b.equivalent(d, item[0])
			if e != nil {
				return e
			}
			if !ok {
				return ErrName
			}
			k, e := b.key(item[0])
			if e != nil {
				return e
			}
			ok, e = b.equivalent(k, item[1])
			if e != nil {
				return e
			}
			if !ok {
				return ErrName
			}
		}
	}
	return nil
}
