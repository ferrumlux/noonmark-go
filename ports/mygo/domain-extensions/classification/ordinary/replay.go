// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

// Replay's only anchor is the empty classification state. The context freezes
// a validated ordinary task graph; this proves classification reachability for
// that graph, NOT the graph's task lifecycle or a product/store transaction.
func Replay(ctx OrdinaryContext, steps []CommittedStep) (ReplayProof, error) {
	b, e := entry(ctx, steps)
	if e != nil {
		return ReplayProof{}, e
	}
	return replay(b, ctx, steps)
}
func replay(b *budget, ctx OrdinaryContext, steps []CommittedStep) (ReplayProof, error) {
	if e := b.scan(steps); e != nil {
		return ReplayProof{}, e
	} // retained transcript/output
	state := EmptyState()
	retained := map[UUID]Plan{}
	for _, step := range steps {
		var p Plan
		if old, ok := retained[step.Plan.ID]; ok {
			if step.Request != old.sealed.request || step.PrepareIDs != old.sealed.ids || !samePlan(step.Plan, old.sealed.data) {
				return ReplayProof{}, ErrConflict
			}
			p = old
		} else {
			var e error
			p, e = prepare(b, ctx, state, step.Request, step.PrepareIDs)
			if e != nil {
				return ReplayProof{}, e
			}
			if !samePlan(step.Plan, p.sealed.data) {
				return ReplayProof{}, ErrInvalid
			}
			retained[p.sealed.data.ID] = p
		}
		if !validUUID(step.DecisionID) {
			return ReplayProof{}, ErrInvalid
		}
		confirmation := Confirmation{&confirmationData{p.sealed, step.DecisionID}}
		next, receipt, e := apply(b, ctx, state, p, confirmation, step.Now, step.CommitIDs)
		if e != nil {
			return ReplayProof{}, e
		}
		if !sameReceipt(receipt, step.Receipt) {
			return ReplayProof{}, ErrInvalid
		}
		state = next
	}
	if e := b.scan(state); e != nil {
		return ReplayProof{}, e
	}
	return ReplayProof{&replayData{context: ctx.sealed, steps: cloneSteps(steps), state: cloneState(state)}}, nil
}

// Continue replays the sealed complete original transcript plus new steps. It
// accepts neither an imported state nor a hash/revision/success declaration.
func Continue(proof ReplayProof, steps []CommittedStep) (ReplayProof, error) {
	if proof.sealed == nil {
		return ReplayProof{}, ErrInvalid
	}
	ctx := OrdinaryContext{proof.sealed.context}
	b, e := entry(ctx, proof.sealed.steps, steps)
	if e != nil {
		return ReplayProof{}, e
	}
	if e = b.scan(proof.sealed.steps, steps); e != nil {
		return ReplayProof{}, e
	}
	count, e := add(uint64(len(proof.sealed.steps)), uint64(len(steps)))
	if e != nil || count > uint64(b.lim.MaxNodes) {
		return ReplayProof{}, ErrResourceLimit
	}
	all := make([]CommittedStep, 0, int(count))
	all = append(all, proof.sealed.steps...)
	all = append(all, steps...)
	return replay(b, ctx, all)
}
