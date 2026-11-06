// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"testing"
)

func TestHistoryEditTitleNoopCannotBypassGuards(t *testing.T) {
	for _, name := range []string{"C", "A", "U", "past", "locked"} {
		t.Run(name, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			want := core.ErrInvalidTransition
			switch name {
			case "C":
				f.run(t, vnStep{"C", 1})
			case "A":
				f.run(t, vnStep{"A", 1})
				want = core.ErrChainAbandoned
			case "U":
				f.run(t, vnStep{"A", 1}, vnStep{"U", 2})
			case "past":
				want = core.ErrImmutableHistory
			case "locked":
				want = core.ErrImmutableHistory
			}
			g := f.k.Snapshot()
			if name == "locked" {
				g.Days[0].LockedAt = vnPtr(vnAt(1))
				g.Days[0].UpdatedAt = vnAt(1)
			}
			if name == "locked" {
				// Locked P is outside the accepted history profile, even though the
				// core has a defensive lock guard. Do not manufacture a sealed prior.
				if _, err := inspectForwardClockOrdinaryHistoryVNext(lifecycleSnapshot(t, g, f.c)); err == nil {
					t.Fatal("locked P profile accepted")
				}
				k, err := core.NewTaskKernel(g)
				must(t, err)
				err = k.SaveTaskTitleInput(core.EditTaskTitleInput{ChainID: g.Chains[0].ID, Title: g.Definitions[0].Title, Today: vnDate, Now: core.Instant(math.Float64bits(math.NaN()))})
				if !errors.Is(err, core.ErrImmutableHistory) {
					t.Fatal("core lock guard", err)
				}
				return
			}
			p := vnGood(t, lifecycleSnapshot(t, g, f.c))
			in := editInput(g)
			for _, d := range g.Definitions {
				if d.SupersededAt == nil {
					in.Title = d.Title
				}
			}
			in.Now = core.Instant(math.Float64bits(math.NaN()))
			if name == "past" {
				in.Today, _ = vnDate.AddDays(1)
			}
			r, e := auditTitleHistoryVNext(p, g, g, in)
			hcZero(t, r, e)
			if !errors.Is(e, want) {
				t.Fatal(e, want)
			}
		})
	}
}
func TestHistoryEditManualSameGroupPeerAndCoreContinuation(t *testing.T) {
	// Same-date R peer has no floor. Return/schedule are core-only fixture edges.
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	must(t, f.k.SetManualProgress(core.SetManualProgressInput{ChainID: chain, TraceID: trace, Percent: 90, Today: vnDate, Now: vnAt(1)}))
	f.run(t, vnStep{"R", 2})
	target := core.DayTraceID(vnID(40))
	must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: target, DayID: core.DayID(vnID(41)), Date: vnDate, Today: vnDate, Now: vnAt(3)}))
	g = f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := manualProgressHistoryInput{ChainID: chain, TraceID: target, Percent: 10, Today: vnDate, Now: vnAt(4)}
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
	a := f.k.Snapshot()
	r, e := auditManualProgressHistoryVNext(p, g, a, in)
	editCheck(t, p, r, e)
	if *a.Traces[1].ManualProgressPercent != 10 {
		t.Fatal("same group peer entered floor")
	}
	// Settled U -> Continue is a real core-only continuation. The U event itself
	// is generated and inspected by the existing settlement adapter.
	f = vnSeed(t, vnDate)
	g = f.k.Snapshot()
	p = vnGood(t, f.snapshot(t))
	in.TraceID = trace
	in.Percent = 60
	in.Now = vnAt(1)
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
	r, e = auditManualProgressHistoryVNext(p, g, f.k.Snapshot(), in)
	editCheck(t, p, r, e)
	p = r.proof.after
	g = f.k.Snapshot()
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	si := settlementHistoryInput{Today: tomorrow, Now: vnAt(2), EventIDs: []string{vnID(100)}}
	must(t, f.k.SettleTaskDays(si.Today, si.Now))
	r, e = auditSettlementHistoryVNext(p, g, f.k.Snapshot(), si)
	must(t, e)
	p = r.proof.after
	must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: chain, TraceID: trace, NewTraceID: target, DayID: core.DayID(vnID(41)), TargetDate: tomorrow, Today: tomorrow, Now: vnAt(3)}))
	g = f.k.Snapshot()
	p = vnGood(t, lifecycleSnapshot(t, g, p.proof.snapshot.Object["classifications"]))
	in.TraceID = target
	in.Today = tomorrow
	in.Percent = 20
	in.Now = core.Instant(math.Float64bits(math.NaN()))
	r, e = auditManualProgressHistoryVNext(p, g, g, in)
	editCheck(t, p, r, e)
	if r.proof.changed || *g.Traces[1].ManualProgressPercent != 60 {
		t.Fatal("continuation floor/no-op")
	}
}
