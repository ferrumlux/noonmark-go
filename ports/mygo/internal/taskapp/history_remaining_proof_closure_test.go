// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

// Fast closure gate enumerates all pre-existing kinds against each new detail.
func TestHistoryRemainingOldKindDetailClosure(t *testing.T) {
	f := vnSeed(t, vnDate)
	p := vnGood(t, f.snapshot(t))
	for _, kind := range []historyCommandKind{historyCommandSettlement, historyCommandComplete, historyCommandUndo, historyCommandAbandon, historyCommandReactivate, historyCommandRename, historyCommandTitleInput, historyCommandManualProgress, historyCommandPlannedSubtaskEdit, historyCommandActualSubtaskEdit} {
		base := historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, prior: p, after: p}
		if !(auditedHistoryCommand{&base}).valid() {
			t.Fatal("old no-detail validity changed", kind)
		}
		for detail := 0; detail < 3; detail++ {
			t.Run(fmt.Sprintf("kind-%d-detail-%d", kind, detail), func(t *testing.T) {
				q := base
				switch detail {
				case 0:
					q.returnDetail = &returnHistoryInput{}
				case 1:
					q.deferDetail = &deferHistoryInput{}
				case 2:
					q.continueDetail = &continueHistoryInput{}
				}
				if (auditedHistoryCommand{&q}).valid() {
					t.Fatal("old kind accepted new typed detail")
				}
			})
		}
	}
}

// Symptom gate exercises the affected private helper's actual success path.
// It is not a Service/UI or persistence test.
func TestHistoryRemainingReturnedProofCannotMasquerade(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := returnHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(880)), Today: vnDate, Now: vnAt(30), EventID: vnID(881)}
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, Today: in.Today, Now: in.Now}))
	r, e := auditReturnHistoryVNext(p, g, k.Snapshot(), in)
	must(t, e)
	if !r.valid() {
		t.Fatal("return success invalid")
	}
	q := *r.proof
	q.kind = historyCommandComplete
	if (auditedHistoryCommand{&q}).valid() {
		t.Fatal("Return proof masqueraded as old completion kind")
	}
}

func remainingClosureReject(t *testing.T, p historyCommandProof) {
	t.Helper()
	r := auditedHistoryCommand{proof: &p}
	if r.valid() {
		t.Fatal("mismatched proof valid")
	}
	if _, e := r.snapshotCopy(); e == nil {
		t.Fatal("invalid proof exposed snapshot")
	}
	if _, e := r.changed(); e == nil {
		t.Fatal("invalid proof exposed changed")
	}
}

// Obtain all old kinds from their real successful adapters. Some are legitimate
// no-op successes; none is assembled by assigning a kind to a synthetic proof.
func remainingClosureOldSuccesses(t *testing.T) map[historyCommandKind]auditedHistoryCommand {
	t.Helper()
	out := map[historyCommandKind]auditedHistoryCommand{}
	save := func(r auditedHistoryCommand, e error) {
		t.Helper()
		must(t, e)
		if !r.valid() {
			t.Fatal("real old success invalid")
		}
		out[r.proof.kind] = r
	}
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	save(auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: vnDate, Now: vnAt(30)}))
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, k.MarkCompleted(trace, vnDate, vnAt(30)))
	save(auditCompletionHistoryVNext(p, g, k.Snapshot(), completionHistoryInput{Action: completionHistoryComplete, ChainID: chain, TraceID: trace, Today: vnDate, Now: vnAt(30)}))
	cf := vnSeed(t, vnDate)
	cf.run(t, vnStep{"C", 10})
	cg := cf.k.Snapshot()
	cp := vnGood(t, cf.snapshot(t))
	k, e = core.NewTaskKernel(cg)
	must(t, e)
	must(t, k.UndoCompleted(trace, vnDate, vnAt(30)))
	save(auditCompletionHistoryVNext(cp, cg, k.Snapshot(), completionHistoryInput{Action: completionHistoryUndo, ChainID: chain, TraceID: trace, Today: vnDate, Now: vnAt(30)}))
	k, e = core.NewTaskKernel(g)
	must(t, e)
	must(t, k.AbandonChain(core.AbandonChainInput{ChainID: chain, TraceID: trace, Now: vnAt(30)}))
	save(auditAbandonHistoryVNext(p, g, k.Snapshot(), abandonHistoryInput{ChainID: chain, TraceID: trace, Now: vnAt(30), EventID: vnID(990)}))
	af := vnSeed(t, vnDate)
	af.run(t, vnStep{"A", 10})
	ag := af.k.Snapshot()
	ap := vnGood(t, af.snapshot(t))
	k, e = core.NewTaskKernel(ag)
	must(t, e)
	_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: chain, TraceID: trace, Today: vnDate, Now: vnAt(30)})
	must(t, e)
	save(auditReactivateHistoryVNext(ap, ag, k.Snapshot(), reactivateHistoryInput{ChainID: chain, TraceID: trace, Today: vnDate, Now: vnAt(30)}))
	for _, action := range []titleHistoryAction{titleHistoryRename, titleHistorySaveInput} {
		save(auditTitleHistoryVNext(p, g, g, titleHistoryInput{Action: action, Route: titleHistoryChain, ChainID: chain, Title: g.Definitions[0].Title, Today: vnDate, Now: vnAt(30)}))
	}
	mi := manualProgressHistoryInput{ChainID: chain, TraceID: trace, Percent: 20, Today: vnDate, Now: vnAt(30)}
	k, e = core.NewTaskKernel(g)
	must(t, e)
	must(t, k.SetManualProgress(core.SetManualProgressInput(mi)))
	save(auditManualProgressHistoryVNext(p, g, k.Snapshot(), mi))
	pg := hsPool(t, 1)
	pp := vnGood(t, hsSnapshot(t, pg))
	pc := pg.Definitions[0].PlannedSubtasks[0]
	save(auditPlannedSubtaskHistoryVNext(pp, pg, pg, plannedSubtaskHistoryInput{Kind: core.SubtaskTitle, ChainID: pg.Chains[0].ID, ExpectedDefinitionID: pg.Definitions[0].ID, SubtaskID: pc.ID, Title: pc.Title, Now: vnAt(30)}))
	sg := hsActual(t)
	sp := vnGood(t, hsSnapshot(t, sg))
	sc := sg.Subtasks[0]
	save(auditActualSubtaskHistoryVNext(sp, sg, sg, actualSubtaskHistoryInput{Kind: core.SubtaskTitle, ChainID: sg.Chains[0].ID, TraceID: sc.TraceID, SubtaskID: sc.ID, Title: sc.Title, Today: vnDate, Now: vnAt(30)}))
	if len(out) != 10 {
		t.Fatalf("real old kinds: %d", len(out))
	}
	return out
}
func remainingClosureNewSuccesses(t *testing.T) []auditedHistoryCommand {
	t.Helper()
	var out []auditedHistoryCommand
	target, e := vnDate.AddDays(1)
	must(t, e)
	for _, kind := range []historyCommandKind{historyCommandReturn, historyCommandDefer, historyCommandContinue} {
		f := vnSeed(t, vnDate)
		if kind == historyCommandContinue {
			f.run(t, vnStep{"L", 10})
		}
		g := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		k, e := core.NewTaskKernel(g)
		must(t, e)
		var r auditedHistoryCommand
		switch kind {
		case historyCommandReturn:
			in := returnHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(880)), Today: vnDate, Now: vnAt(30), EventID: vnID(881)}
			must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, Today: in.Today, Now: in.Now}))
			r, e = auditReturnHistoryVNext(p, g, k.Snapshot(), in)
		case historyCommandDefer:
			in := deferHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(880)), DayID: core.DayID(vnID(882)), Today: vnDate, TargetDate: target, Now: vnAt(30), EventID: vnID(881)}
			must(t, k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, Today: in.Today, TargetDate: target, Now: in.Now}))
			r, e = auditDeferHistoryVNext(p, g, k.Snapshot(), in)
		case historyCommandContinue:
			in := continueHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(880)), DayID: core.DayID(vnID(882)), Today: target, TargetDate: target, Now: vnAt(30)}
			must(t, k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, Today: in.Today, TargetDate: target, Now: in.Now}))
			r, e = auditContinueHistoryVNext(p, g, k.Snapshot(), in)
		}
		must(t, e)
		if !r.valid() || r.proof.kind != kind {
			t.Fatal("new success kind invalid")
		}
		out = append(out, r)
	}
	return out
}
func TestHistoryRemainingProofClosureRealSuccessMatrix(t *testing.T) {
	old := remainingClosureOldSuccesses(t)
	fresh := remainingClosureNewSuccesses(t)
	for kind, r := range old {
		if r.proof.returnDetail != nil || r.proof.deferDetail != nil || r.proof.continueDetail != nil || !r.valid() {
			t.Fatal("old success no longer has nil details")
		}
		for _, n := range fresh {
			t.Run(fmt.Sprintf("old-%d-injected-new-%d", kind, n.proof.kind), func(t *testing.T) {
				q := *r.proof
				q.returnDetail = n.proof.returnDetail
				q.deferDetail = n.proof.deferDetail
				q.continueDetail = n.proof.continueDetail
				remainingClosureReject(t, q)
			})
			t.Run(fmt.Sprintf("new-%d-as-old-%d", n.proof.kind, kind), func(t *testing.T) { q := *n.proof; q.kind = kind; remainingClosureReject(t, q) })
		}
	}
	// All eight detail-presence combinations for each kind. The only allowed
	// combination is its one matching typed detail; zero, mismatched singles,
	// all three pairs, and the triple must reject through both result accessors.
	for i, r := range fresh {
		for mask := 0; mask < 8; mask++ {
			t.Run(fmt.Sprintf("new-%d-mask-%d", r.proof.kind, mask), func(t *testing.T) {
				q := *r.proof
				q.returnDetail = nil
				q.deferDetail = nil
				q.continueDetail = nil
				if mask&1 != 0 {
					q.returnDetail = fresh[0].proof.returnDetail
				}
				if mask&2 != 0 {
					q.deferDetail = fresh[1].proof.deferDetail
				}
				if mask&4 != 0 {
					q.continueDetail = fresh[2].proof.continueDetail
				}
				if mask == 1<<i {
					x := auditedHistoryCommand{proof: &q}
					if !x.valid() {
						t.Fatal("matching single detail rejected")
					}
					_, e := x.snapshotCopy()
					must(t, e)
					_, e = x.changed()
					must(t, e)
				} else {
					remainingClosureReject(t, q)
				}
			})
		}
	}
}
