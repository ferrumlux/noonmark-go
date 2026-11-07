// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

func lookup(b *budget, s State, key string) (Category, bool, error) {
	// K's result is already NFC. Existing imported bytes need their own prepaid
	// NFC before Swift-equivalent comparison; do not feed keys back through K.
	for _, c := range s.Categories {
		candidate, e := b.nfc(c.CanonicalKey)
		if e != nil {
			return Category{}, false, e
		}
		if candidate == key {
			return c, true, nil
		}
	}
	return Category{}, false, nil
}
func existingPreview(c Category) *PlanItem {
	return &PlanItem{ID: c.ID, Kind: "category", Name: c.Name, ColorHex: c.ColorHex, Resolution: "existing"}
}
func currentPreview(s State, r Request) SelectionPreview {
	if c := s.CurrentByChainID[r.ChainID].Category; c != nil {
		return SelectionPreview{Category: existingPreview(s.Categories[c.CategoryID])}
	}
	return SelectionPreview{}
}
func prepareData(b *budget, ctx OrdinaryContext, s State, r Request, ids PrepareIDs) (PlanData, error) {
	if e := requestShape(r); e != nil {
		return PlanData{}, e
	}
	if !ctx.hasChain(r.ChainID) {
		return PlanData{}, ErrContext
	}
	if !validUUID(ids.PlanID) {
		return PlanData{}, ErrInvalid
	}
	for _, record := range s.ChangeRecords {
		if record.PlanID == ids.PlanID {
			return PlanData{}, ErrConflict
		}
	}
	if e := b.charge(2048, 64); e != nil {
		return PlanData{}, e
	} // bounded plan/preview wrappers
	prepared := r
	var notices []Notice
	if r.Choice.Kind == ChoiceNew {
		name, e := b.normalized(r.Choice.Name)
		if e != nil {
			return PlanData{}, e
		}
		key, e := b.key(name)
		if e != nil {
			return PlanData{}, e
		}
		found, ok, e := lookup(b, s, key)
		if e != nil {
			return PlanData{}, e
		}
		if ok {
			prepared.Choice = Choice{Kind: ChoiceExisting, CategoryID: found.ID}
			notices = []Notice{{Kind: "existingItemReused", ItemKind: "category", InputName: name, CurrentName: found.Name}}
		} else {
			prepared.Choice.Name = name
		}
	}
	p := PlanData{ID: ids.PlanID, BaseRevision: s.Revision, InteractionID: r.InteractionID, Intent: prepared, Source: UserDirect, Notices: notices}
	after := SelectionPreview{}
	switch prepared.Choice.Kind {
	case ChoiceNone:
	case ChoiceExisting:
		category, ok := s.Categories[prepared.Choice.CategoryID]
		if !ok {
			return PlanData{}, ErrInvalid
		}
		after.Category = existingPreview(category)
	case ChoiceNew:
		if !validUUID(ids.CategoryID) {
			return PlanData{}, ErrInvalid
		}
		if _, exists := s.Categories[ids.CategoryID]; exists {
			return PlanData{}, ErrConflict
		}
		p.PlannedCategoryID = ids.CategoryID
		after.Category = &PlanItem{ID: ids.CategoryID, Kind: "category", Name: prepared.Choice.Name, ColorHex: prepared.Choice.ColorHex, Resolution: "new"}
	}
	p.Changes = []PlanChange{{Kind: "setCurrent", ChainID: r.ChainID, Before: currentPreview(s, r), After: after}}
	digest, e := planDigest(b, p)
	if e != nil {
		return PlanData{}, e
	}
	p.Digest = digest
	return p, nil
}

// Prepare never changes classification facts/revision or consumes an ID source.
// Each call has a new explicit plan ID; retry means reusing its returned Plan.
func Prepare(ctx OrdinaryContext, s State, r Request, ids PrepareIDs) (Plan, error) {
	b, e := entry(ctx, s, r, ids)
	if e != nil {
		return Plan{}, e
	}
	return prepare(b, ctx, s, r, ids)
}
func prepare(b *budget, ctx OrdinaryContext, s State, r Request, ids PrepareIDs) (Plan, error) {
	if e := validateIntegrity(b, ctx, s); e != nil {
		return Plan{}, e
	}
	p, e := prepareData(b, ctx, s, r, ids)
	if e != nil {
		return Plan{}, e
	}
	if e = b.scan(p); e != nil {
		return Plan{}, e
	}
	return Plan{&preparedPlan{context: ctx.sealed, request: r, ids: ids, data: clonePlan(p)}}, nil
}
func ConfirmUser(p Plan, decisionID UUID) (Confirmation, error) {
	if p.sealed == nil {
		return Confirmation{}, ErrInvalid
	}
	b, e := entry(OrdinaryContext{p.sealed.context}, p.sealed.data, decisionID)
	if e != nil {
		return Confirmation{}, e
	}
	if !validUUID(decisionID) {
		return Confirmation{}, ErrInvalid
	}
	if e = validatePlan(b, p.sealed.data); e != nil {
		return Confirmation{}, e
	}
	return Confirmation{&confirmationData{p.sealed, decisionID}}, nil
}
func validatePlan(b *budget, p PlanData) error {
	if e := planShape(p); e != nil {
		return e
	}
	want, e := planDigest(b, p)
	if e != nil {
		return e
	}
	if want != p.Digest {
		return ErrInvalid
	}
	return nil
}
func samePlan(a, c PlanData) bool {
	return a.ID == c.ID && a.BaseRevision == c.BaseRevision && a.InteractionID == c.InteractionID && a.Intent == c.Intent && a.Source == c.Source && a.CreatedItemID == c.CreatedItemID && a.PlannedCategoryID == c.PlannedCategoryID && a.Digest == c.Digest && sameChanges(a.Changes, c.Changes) && sameNotices(a.Notices, c.Notices) && sameElements(a.PlannedLabelIDs, c.PlannedLabelIDs) && sameElements(a.Blockers, c.Blockers)
}

// Swift arrays have no nil/empty distinction. Compare logical elements without
// allocation; preserve every actual string byte and identity in the evidence.
func sameElements[T comparable](a, c []T) bool {
	if len(a) != len(c) {
		return false
	}
	for i := range a {
		if a[i] != c[i] {
			return false
		}
	}
	return true
}
func samePreview(a, c SelectionPreview) bool {
	if a.Category == nil || c.Category == nil {
		if a.Category != nil || c.Category != nil {
			return false
		}
	} else if *a.Category != *c.Category {
		return false
	}
	return sameElements(a.Labels, c.Labels)
}
func sameChanges(a, c []PlanChange) bool {
	if len(a) != len(c) {
		return false
	}
	for i, x := range a {
		y := c[i]
		if x.Kind != y.Kind || x.ChainID != y.ChainID || !samePreview(x.Before, y.Before) || !samePreview(x.After, y.After) {
			return false
		}
	}
	return true
}
func sameNotices(a, c []Notice) bool { return sameElements(a, c) }
func sameReceipt(a, c Receipt) bool {
	return a.PlanID == c.PlanID && a.Revision == c.Revision && a.ChangeRecordID == c.ChangeRecordID && a.DecisionID == c.DecisionID && a.ChangeRecordIntegrityDigest == c.ChangeRecordIntegrityDigest && sameNotices(a.Notices, c.Notices)
}
