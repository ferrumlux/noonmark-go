// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

// Command-generated synthetic history, not evidence of Service reachability.
// Each event edge uses its existing typed adapter and full successor inspector.
func TestHistoryEditCommandGeneratedLineages(t *testing.T) {
	for _, path := range []string{"A-P", "A-A-P", "A-U", "U-A-U"} {
		t.Run(path, func(t *testing.T) {
			tomorrow, e := vnDate.AddDays(1)
			must(t, e)
			f := vnSeed(t, vnDate)
			// A separate future P survives settlement, then becomes today's manual owner.
			other := core.TaskChainID(vnID(20))
			otherTrace := core.DayTraceID(vnID(22))
			must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: other, DefinitionID: core.TaskDefinitionID(vnID(21)), Title: "other", Now: vnAt(1)}))
			must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: other, TraceID: otherTrace, DayID: core.DayID(vnID(23)), Date: tomorrow, Today: vnDate, Now: vnAt(2)}))
			p := vnGood(t, f.snapshot(t))
			nextEvent := 100
			clock := 3.0
			abandon := func() {
				b := f.k.Snapshot()
				in := abandonHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Now: vnAt(clock), EventID: vnID(nextEvent)}
				clock++
				nextEvent++
				must(t, f.k.AbandonChain(core.AbandonChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Now: in.Now}))
				r, e := auditAbandonHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				if !r.valid() {
					t.Fatal("abandon proof")
				}
				p = r.proof.after
			}
			react := func(day core.LocalDate, capture bool) {
				b := f.k.Snapshot()
				in := reactivateHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Today: day, Now: vnAt(clock)}
				clock++
				if capture {
					in.EventIDs = []string{vnID(nextEvent)}
					nextEvent++
				}
				_, e := f.k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Today: day, Now: in.Now})
				must(t, e)
				r, e := auditReactivateHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				p = r.proof.after
			}
			settle := func() {
				b := f.k.Snapshot()
				in := settlementHistoryInput{Today: tomorrow, Now: vnAt(clock), EventIDs: []string{vnID(nextEvent)}}
				clock++
				nextEvent++
				must(t, f.k.SettleTaskDays(in.Today, in.Now))
				r, e := auditSettlementHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				p = r.proof.after
			}
			today := vnDate
			switch path {
			case "A-P":
				abandon()
				react(vnDate, false)
			case "A-A-P":
				abandon()
				react(vnDate, false)
				abandon()
				react(vnDate, false)
			case "A-U":
				abandon()
				react(vnDate, false)
				settle()
				today = tomorrow
			case "U-A-U":
				settle()
				abandon()
				react(tomorrow, true)
				today = tomorrow
			}
			g := f.k.Snapshot()
			in := editInput(g)
			in.Today = today
			in.Now = vnAt(clock)
			clock++
			if today == tomorrow {
				r, e := auditTitleHistoryVNext(p, g, g, in)
				hcZero(t, r, e)
				if !errors.Is(e, core.ErrInvalidTransition) {
					t.Fatal("U source guard", e)
				}
				in.ChainID = other
			}
			a, e := editTitle(t, g, in)
			must(t, e)
			r, e := auditTitleHistoryVNext(p, g, a, in)
			editCheck(t, p, r, e)
			p = r.proof.after
			g = a
			target := g.Traces[0].ID
			chain := g.Chains[0].ID
			if today == tomorrow {
				target = otherTrace
				chain = other
			}
			mi := manualProgressHistoryInput{ChainID: chain, TraceID: target, Percent: 60, Today: today, Now: vnAt(clock)}
			k, e := core.NewTaskKernel(g)
			must(t, e)
			must(t, k.SetManualProgress(core.SetManualProgressInput(mi)))
			r, e = auditManualProgressHistoryVNext(p, g, k.Snapshot(), mi)
			editCheck(t, p, r, e)
		})
	}
}

// The R/return+schedule fixtures below use real core commands; the return and
// scheduling edges have no new typed adapter in this scope and are core-only.
func TestHistoryEditHistoricalDefinitionAndManualFloor(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	must(t, f.k.SetManualProgress(core.SetManualProgressInput{ChainID: chain, TraceID: trace, Percent: 60, Today: vnDate, Now: vnAt(1)}))
	must(t, f.k.ReturnToPool(core.ReturnToPoolInput{ChainID: chain, TraceID: trace, NewDefinitionID: core.TaskDefinitionID(vnID(40)), CancellationID: core.CancellationID(vnID(41)), Today: vnDate, Now: vnAt(2)}))
	f.capture(core.TraceReturnedToPool, vnAt(2))
	// R alone is editable on chain route. Rename versions and freezes its reference.
	g = f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := editInput(g)
	in.Action = titleHistoryRename
	a, e := editTitle(t, g, in)
	must(t, e)
	r, e := auditTitleHistoryVNext(p, g, a, in)
	editCheck(t, p, r, e)
	if a.Traces[0].DefinitionID != g.Traces[0].DefinitionID {
		t.Fatal("R reference changed")
	}
	// Next-day P floor is independently expected to be the earlier raw 60.
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(vnID(50)), DayID: core.DayID(vnID(51)), Date: tomorrow, Today: tomorrow, Now: vnAt(3)}))
	g = f.k.Snapshot()
	p = vnGood(t, f.snapshot(t))
	for _, percent := range []int{20, 60, 100} {
		mi := manualProgressHistoryInput{ChainID: chain, TraceID: core.DayTraceID(vnID(50)), Percent: percent, Today: tomorrow, Now: vnAt(6)}
		k, e := core.NewTaskKernel(g)
		must(t, e)
		must(t, k.SetManualProgress(core.SetManualProgressInput(mi)))
		a = k.Snapshot()
		r, e = auditManualProgressHistoryVNext(p, g, a, mi)
		editCheck(t, p, r, e)
		want := 60
		if percent > 60 {
			want = 100
		}
		if *a.Traces[1].ManualProgressPercent != want || *a.Traces[0].ManualProgressPercent != 60 {
			t.Fatal("history floor")
		}
	}
}

func TestHistoryEditManualCompleteUndoAndContinuation(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	mi := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Percent: 100, Today: vnDate, Now: vnAt(1)}
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(mi)))
	r, e := auditManualProgressHistoryVNext(p, g, f.k.Snapshot(), mi)
	editCheck(t, p, r, e)
	p = r.proof.after
	for _, action := range []completionHistoryAction{completionHistoryComplete, completionHistoryUndo} {
		g = f.k.Snapshot()
		in := completionHistoryInput{Action: action, ChainID: mi.ChainID, TraceID: mi.TraceID, Today: vnDate, Now: vnAt(float64(action) + 2)}
		if action == completionHistoryComplete {
			must(t, f.k.MarkCompleted(in.TraceID, in.Today, in.Now))
		} else {
			must(t, f.k.UndoCompleted(in.TraceID, in.Today, in.Now))
		}
		r, e = auditCompletionHistoryVNext(p, g, f.k.Snapshot(), in)
		must(t, e)
		p = r.proof.after
		if *f.k.Snapshot().Traces[0].ManualProgressPercent != 100 {
			t.Fatal("completion/undo lost manual")
		}
	}
}
