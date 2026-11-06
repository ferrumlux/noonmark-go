// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"testing"
)

func TestHistoryLifecycleExactClocksAndDateLimits(t *testing.T) {
	for _, seconds := range []float64{-10, -0.25, math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0, math.MaxFloat64} {
		t.Run(coreClockName(seconds), func(t *testing.T) {
			// Real typed core creation at the exact supplied clock, including signed zero.
			now := vnAt(seconds)
			k, e := core.NewTaskKernel(core.TaskGraph{})
			must(t, e)
			chain, trace := core.TaskChainID(vnID(1)), core.DayTraceID(vnID(3))
			date := core.LocalDate{Year: 9999, Month: 12, Day: 31}
			must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(vnID(2)), Title: "clock fixture", Now: now}))
			must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(vnID(4)), Date: date, Today: date, Now: now}))
			extra, e := emptySupplement()
			must(t, e)
			before := k.Snapshot()
			prior := vnGood(t, lifecycleSnapshot(t, before, extra.Classifications))
			after := before.Clone()
			after.Chains[0].State = core.ChainAbandoned
			after.Chains[0].UpdatedAt = now
			after.Traces[0].Status = core.TraceAbandoned
			after.Traces[0].ContentUpdatedAt = now
			after.Traces[0].SettledAt = vnPtr(now)
			id := vnID(500)
			want := lifecycleSnapshot(t, after, lifecycleEvent(extra.Classifications, trace, id, core.TraceAbandoned, now))
			vnGood(t, want)
			r, e := auditAbandonHistoryVNext(prior, before, after, abandonHistoryInput{ChainID: chain, TraceID: trace, Now: now, EventID: id})
			must(t, e)
			lifecycleAssert(t, r, want, historyCommandAbandon, chain, trace, trace, id, now, core.LocalDate{})
			// Max date never needs an invented tomorrow: open date == Today is P.
			next := after.Clone()
			next.Chains[0].State = core.ChainActive
			next.Traces[0].Status = core.TracePending
			next.Traces[0].SettledAt = nil
			expected := lifecycleSnapshot(t, next, want.Object["classifications"])
			rp, e := auditReactivateHistoryVNext(r.proof.after, after, next, reactivateHistoryInput{ChainID: chain, TraceID: trace, Today: date, Now: now})
			must(t, e)
			lifecycleAssert(t, rp, expected, historyCommandReactivate, chain, trace, trace, "", now, date)
		})
	}
}
func coreClockName(n float64) string { return string([]byte{'c'}) + fmtClockBits(math.Float64bits(n)) }
func fmtClockBits(n uint64) string {
	const hex = "0123456789abcdef"
	var out [16]byte
	for i := 15; i >= 0; i-- {
		out[i] = hex[n&15]
		n >>= 4
	}
	return string(out[:])
}

func TestHistoryLifecycleSignedZeroAfterMustBeExact(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	after := before.Clone()
	now := vnAt(math.Copysign(0, -1))
	after.Chains[0].State = core.ChainAbandoned
	after.Chains[0].UpdatedAt = now
	after.Traces[0].Status = core.TraceAbandoned
	after.Traces[0].ContentUpdatedAt = now
	after.Traces[0].SettledAt = vnPtr(now)
	in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: now, EventID: vnID(500)}
	r, e := auditAbandonHistoryVNext(prior, before, after, in)
	must(t, e)
	bad := after.Clone()
	bad.Traces[0].ContentUpdatedAt = vnAt(0)
	r, e = auditAbandonHistoryVNext(prior, before, bad, in)
	lifecycleZero(t, r, e)
}

func TestHistoryLifecycleFiniteAndLocalClockFailures(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 10})
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	after := before.Clone()
	after.Chains[0].State = core.ChainActive
	after.Chains[0].UpdatedAt = vnAt(20)
	after.Traces[0].Status = core.TracePending
	after.Traces[0].SettledAt = nil
	after.Traces[0].ContentUpdatedAt = vnAt(20)
	for _, now := range []core.Instant{core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), core.Instant(math.Float64bits(math.Inf(-1))), vnAt(9)} {
		r, e := auditReactivateHistoryVNext(prior, before, after, reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: now})
		lifecycleZero(t, r, e)
	}
	for _, date := range []core.LocalDate{{}, {Year: 2026, Month: 2, Day: 29}, {Year: 10000, Month: 1, Day: 1}} {
		r, e := auditReactivateHistoryVNext(prior, before, after, reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: date, Now: vnAt(20)})
		lifecycleZero(t, r, e)
	}
}

func TestHistoryLifecycleLocalClockNotGlobalFrontier(t *testing.T) {
	for _, withEvent := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrelated newer pool clock accepted", true: "new event before old event rejected"}[withEvent], func(t *testing.T) {
			f := vnSeed(t, vnDate)
			chain := core.TaskChainID(vnID(30))
			created := vnAt(100)
			if withEvent {
				created = vnAt(0)
			}
			must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(vnID(31)), Title: "unrelated", Now: created}))
			c := f.c
			if withEvent {
				trace := core.DayTraceID(vnID(32))
				must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(vnID(33)), Date: vnDate, Today: vnDate, Now: vnAt(0)}))
				must(t, f.k.AbandonChain(core.AbandonChainInput{ChainID: chain, TraceID: trace, Now: vnAt(100)}))
				c = lifecycleEvent(c, trace, vnID(700), core.TraceAbandoned, vnAt(100))
			}
			before := f.k.Snapshot()
			prior := vnGood(t, lifecycleSnapshot(t, before, c))
			after := lifecycleAbandonDelta(before, 0, vnAt(20))
			in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: vnAt(20), EventID: vnID(701)}
			r, e := auditAbandonHistoryVNext(prior, before, after, in)
			if withEvent {
				lifecycleZero(t, r, e)
			} else {
				must(t, e)
				want := lifecycleSnapshot(t, after, lifecycleEvent(c, in.TraceID, in.EventID, core.TraceAbandoned, in.Now))
				lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, in.Now, core.LocalDate{})
			}
		})
	}
}
func TestHistoryLifecycleOpenUSilentSettlement(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 10})
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	after := before.Clone()
	after.Chains[0].State = core.ChainActive
	after.Chains[0].UpdatedAt = vnAt(20)
	after.Traces[0].Status = core.TraceUnfinished
	after.Traces[0].ContentUpdatedAt = vnAt(20)
	today, e := vnDate.AddDays(1)
	must(t, e)
	in := reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: today, Now: vnAt(20), EventIDs: []string{vnID(700)}}
	r, e := auditReactivateHistoryVNext(prior, before, after, in)
	must(t, e)
	k, e := core.NewTaskKernel(after)
	must(t, e)
	must(t, k.SettleTaskDays(today, vnAt(30)))
	final := k.Snapshot()
	s, e := auditSettlementHistoryVNext(r.proof.after, after, final, settlementHistoryInput{Today: today, Now: vnAt(30)})
	must(t, e)
	got, e := s.snapshotCopy()
	must(t, e)
	if !historyEqual(got.Object["classifications"], r.proof.after.proof.snapshot.Object["classifications"]) {
		t.Fatal("silent lock recaptured U")
	}
	if final.Days[0].LockedAt == nil {
		t.Fatal("day did not lock")
	}
}

func TestHistoryLifecycleZeroPriorAndClosedResult(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	r, e := auditAbandonHistoryVNext(validatedHistorySnapshot{}, g, g, abandonHistoryInput{})
	lifecycleZero(t, r, e)
	r, e = auditReactivateHistoryVNext(validatedHistorySnapshot{}, g, g, reactivateHistoryInput{EventIDs: []string{"bad", "bad"}})
	lifecycleZero(t, r, e)
}
