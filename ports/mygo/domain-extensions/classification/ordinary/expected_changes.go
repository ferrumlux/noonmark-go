// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

// validateExpectedCurrent mirrors TC:2327-2342 for this fixed subset. The
// ORIGINAL sealed plan has already passed shape/identity/digest/confirmation
// validation. Recompute only changes from its PREPARED intent and current
// state, not a second prepare or a second plan digest. All supported notices
// have alias=false, so reusedResolution is always existing (TC:2880-2894).
// setCurrent never has blockers; every nonempty blocker list stays rejected.
func validateExpectedCurrent(b *budget, ctx OrdinaryContext, state State, p PlanData) error {
	if len(p.Blockers) != 0 {
		return ErrUnsupported
	}
	if !ctx.hasChain(p.Intent.ChainID) {
		return ErrContext
	}
	if e := b.charge(2048, 64); e != nil {
		return e
	} // expected change and two preview wrappers
	after := SelectionPreview{}
	switch choice := p.Intent.Choice; choice.Kind {
	case ChoiceNone:
	case ChoiceExisting:
		category, ok := state.Categories[choice.CategoryID]
		if !ok {
			return ErrInvalid
		}
		after.Category = existingPreview(category)
	case ChoiceNew:
		// planShape checks planned identity presence and the inactive ID slots.
		// Collision remains checked at resolve/create, as in TC:3013-3019.
		if !validUUID(p.PlannedCategoryID) {
			return ErrInvalid
		}
		after.Category = &PlanItem{ID: p.PlannedCategoryID, Kind: "category", Name: choice.Name, ColorHex: choice.ColorHex, Resolution: "new"}
	default:
		return ErrUnsupported
	}
	expected := []PlanChange{{Kind: "setCurrent", ChainID: p.Intent.ChainID, Before: currentPreview(state, p.Intent), After: after}}
	equal, e := expectedChangesEquivalent(b, expected, p.Changes)
	if e != nil {
		return e
	}
	if !equal {
		return ErrInvalid
	}
	return nil
}

// These comparators are used ONLY by the upstream expectedChanges guard. Do
// not use them for retained transcript, original digest, receipt, or exact retry
// binding. Kinds, UUIDs, chains and enum resolutions remain exact; only Swift
// String name/color equality uses two individually prepaid NFC calls.
func expectedChangesEquivalent(b *budget, a, c []PlanChange) (bool, error) {
	if len(a) != len(c) {
		return false, nil
	}
	for i, x := range a {
		y := c[i]
		if x.Kind != y.Kind || x.ChainID != y.ChainID {
			return false, nil
		}
		ok, e := expectedPreviewEquivalent(b, x.Before, y.Before)
		if e != nil || !ok {
			return false, e
		}
		ok, e = expectedPreviewEquivalent(b, x.After, y.After)
		if e != nil || !ok {
			return false, e
		}
	}
	return true, nil
}
func expectedPreviewEquivalent(b *budget, a, c SelectionPreview) (bool, error) {
	if len(a.Labels) != 0 || len(c.Labels) != 0 {
		return false, ErrUnsupported
	}
	if a.Category == nil || c.Category == nil {
		return a.Category == nil && c.Category == nil, nil
	}
	x, y := a.Category, c.Category
	if x.ID != y.ID || x.Kind != y.Kind || x.Resolution != y.Resolution {
		return false, nil
	}
	equal, e := b.equivalent(x.Name, y.Name)
	if e != nil || !equal {
		return false, e
	}
	return b.equivalent(x.ColorHex, y.ColorHex)
}
