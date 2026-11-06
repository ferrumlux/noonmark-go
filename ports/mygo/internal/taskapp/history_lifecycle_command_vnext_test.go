// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Explicit event oracle: never invokes the lifecycle adapter or its builder.
func lifecycleEvent(c canonical.Value, trace core.DayTraceID, id string, status core.TraceStatus, at core.Instant) canonical.Value {
	c = historyVNextClone(c)
	rev := uint64(1)
	// The fixtures use small, independently specified integer revisions.
	for _, v := range c.Object["snapshotEventsByTraceID"].Array {
		if v.Kind == canonical.Array {
			rev += uint64(len(v.Array))
		}
	}
	key := historyObject(map[string]canonical.Value{"rawValue": historyString(string(trace))})
	ev := historyObject(map[string]canonical.Value{"id": historyString(strings.ToUpper(id)), "traceID": key, "status": historyString(string(status)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(at)), "revision": historyNumber(rev)})
	streams := c.Object["snapshotEventsByTraceID"]
	found := false
	for i := 0; i < len(streams.Array); i += 2 {
		if historyID(streams.Array[i]) == string(trace) {
			streams.Array[i+1].Array = append(streams.Array[i+1].Array, ev)
			found = true
		}
	}
	if !found {
		streams.Array = append(streams.Array, key, canonical.Value{Kind: canonical.Array, Array: []canonical.Value{ev}})
	}
	latest := c.Object["snapshotsByTraceID"]
	found = false
	for i := 0; i < len(latest.Array); i += 2 {
		if historyID(latest.Array[i]) == string(trace) {
			latest.Array[i+1] = historyVNextClone(ev)
			found = true
		}
	}
	if !found {
		latest.Array = append(latest.Array, key, historyVNextClone(ev))
	}
	c.Object["snapshotEventsByTraceID"] = streams
	c.Object["snapshotsByTraceID"] = latest
	c.Object["revision"] = historyNumber(rev)
	return c
}
func lifecycleSnapshot(t *testing.T, g core.TaskGraph, c canonical.Value) canonical.Value {
	t.Helper()
	extra, e := emptySupplement()
	must(t, e)
	extra.Classifications = c
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	return s
}
func lifecycleAssert(t *testing.T, r auditedHistoryCommand, want canonical.Value, kind historyCommandKind, chain core.TaskChainID, request, effective core.DayTraceID, id string, now core.Instant, today core.LocalDate) {
	t.Helper()
	got, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(got, want) {
		t.Fatal("whole graph/supplement differs from explicit delta and event oracle")
	}
	changed, e := r.changed()
	must(t, e)
	if !changed {
		t.Fatal("state change incorrectly reported as no-op")
	}
	p := r.proof
	for _, badKind := range []historyCommandKind{0, historyCommandManualProgress + 1, 255} {
		bad := *p
		bad.kind = badKind
		invalid := auditedHistoryCommand{proof: &bad}
		if invalid.valid() {
			t.Fatal("unknown command kind accepted")
		}
		if _, e := invalid.snapshotCopy(); e == nil {
			t.Fatal("unknown kind exposed snapshot")
		}
	}
	if p.kind != kind || p.chain != chain || p.trace != request || p.effectiveTrace != effective || p.eventID != id || p.now != now || p.today != today {
		t.Fatal("scalar request/effective identity drift")
	}
	got.Object["traces"].Array[0].Object["contentUpdatedAt"] = historyNumber(0)
	again, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(again, want) {
		t.Fatal("returned copy aliases proof")
	}
}
func lifecycleZero(t *testing.T, r auditedHistoryCommand, e error) {
	t.Helper()
	if e == nil || r.proof != nil || r.valid() {
		t.Fatal("failure must return zero proof", e)
	}
	if _, e = r.snapshotCopy(); e == nil {
		t.Fatal("zero snapshot")
	}
	if _, e = r.changed(); e == nil {
		t.Fatal("zero changed")
	}
}

func TestHistoryLifecycleAbandonSources(t *testing.T) {
	for name, steps := range map[string][]vnStep{"current P": {}, "future P": {}, "locked U": {{"L", 5}}, "open U": {{"A", 5}, {"U", 10}}, "repeated A": {{"A", 5}, {"P", 10}}} {
		t.Run(name, func(t *testing.T) {
			date := vnDate
			if name == "future P" {
				var e error
				date, e = vnDate.AddDays(1)
				must(t, e)
			}
			f := vnSeed(t, date)
			f.run(t, steps...)
			before := f.k.Snapshot()
			prior := vnGood(t, f.snapshot(t))
			after := before.Clone()
			now := vnAt(30)
			after.Traces[0].Status = core.TraceAbandoned
			after.Traces[0].ContentUpdatedAt = now
			after.Traces[0].SettledAt = vnPtr(now)
			after.Traces[0].PinOrder = nil
			after.Chains[0].State = core.ChainAbandoned
			after.Chains[0].UpdatedAt = now
			in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: now, EventID: strings.ToLower(vnID(999))}
			want := lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceAbandoned, now))
			vnGood(t, want)
			r, e := auditAbandonHistoryVNext(prior, before, after, in)
			must(t, e)
			lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, now, core.LocalDate{})
			bad := after.Clone()
			bad.Definitions[0].Title = "legal but unrelated"
			r, e = auditAbandonHistoryVNext(prior, before, bad, in)
			lifecycleZero(t, r, e)
			if !reflect.DeepEqual(before, f.k.Snapshot()) {
				t.Fatal("input mutated")
			}
		})
	}
}
func TestHistoryLifecycleReactivateBranches(t *testing.T) {
	for _, locked := range []bool{false, true} {
		for _, offset := range []int{-1, 0, 1} {
			t.Run(strings.Join([]string{map[bool]string{false: "open", true: "locked"}[locked], string(rune('1' + offset))}, "/"), func(t *testing.T) {
				f := vnSeed(t, vnDate)
				f.run(t, vnStep{"A", 10})
				if locked {
					f.run(t, vnStep{"L", 20})
				}
				before := f.k.Snapshot()
				prior := vnGood(t, f.snapshot(t))
				today, e := vnDate.AddDays(offset)
				must(t, e)
				now := vnAt(30)
				after := before.Clone()
				after.Chains[0].State = core.ChainActive
				after.Chains[0].UpdatedAt = now
				after.Traces[0].ContentUpdatedAt = now
				after.Traces[0].PinOrder = nil
				status := core.TracePending
				if locked || offset > 0 {
					status = core.TraceUnfinished
				}
				after.Traces[0].Status = status
				if status == core.TracePending {
					after.Traces[0].SettledAt = nil
				}
				in := reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: today, Now: now}
				c := f.c
				id := ""
				if status == core.TraceUnfinished {
					id = vnID(999)
					in.EventIDs = []string{id}
					c = lifecycleEvent(c, in.TraceID, id, status, now)
				}
				want := lifecycleSnapshot(t, after, c)
				vnGood(t, want)
				r, e := auditReactivateHistoryVNext(prior, before, after, in)
				must(t, e)
				lifecycleAssert(t, r, want, historyCommandReactivate, in.ChainID, in.TraceID, in.TraceID, id, now, today)
				if len(in.EventIDs) > 0 {
					in.EventIDs[0] = vnID(998)
					lifecycleAssert(t, r, want, historyCommandReactivate, in.ChainID, in.TraceID, in.TraceID, id, now, today)
				}
				if status == core.TracePending {
					in.EventIDs = []string{vnID(998)}
				} else {
					in.EventIDs = nil
				}
				r, e = auditReactivateHistoryVNext(prior, before, after, in)
				lifecycleZero(t, r, e)
			})
		}
	}
}

// Separately rooted structural parameter fixtures. Profile-consistent is not a
// claim of Service reachability; either same-chain A may be requested.
func TestHistoryLifecycleSyntheticMultiA(t *testing.T) {
	f := vnSeed(t, core.LocalDate{Year: 2026, Month: 11, Day: 5})
	f.run(t, vnStep{"A", 10})
	g := f.k.Snapshot()
	g.Days[0].CreatedAt, g.Days[0].UpdatedAt = vnAt(1), vnAt(1)
	g.Chains[0].CreatedAt = vnAt(1)
	g.Definitions[0].CreatedAt, g.Definitions[0].ContentUpdatedAt = vnAt(1), vnAt(1)
	g.Traces[0].CreatedAt = vnAt(1)
	date := core.LocalDate{Year: 2026, Month: 11, Day: 6}
	d := g.Days[0]
	d.ID = core.DayID(vnID(51))
	d.Date = date
	g.Days = append(g.Days, d)
	tr := g.Traces[0]
	tr.ID = core.DayTraceID(vnID(52))
	tr.Date = date
	tr.ContentUpdatedAt = vnAt(20)
	tr.SettledAt = vnPtr(vnAt(20))
	g.Traces = append(g.Traces, tr)
	g.Chains[0].UpdatedAt = vnAt(20)
	c := lifecycleEvent(f.c, tr.ID, vnID(102), core.TraceAbandoned, vnAt(20))
	prior := vnGood(t, lifecycleSnapshot(t, g, c))
	for i := 0; i < 2; i++ {
		before := g.Clone()
		after := g.Clone()
		now := vnAt(30)
		after.Chains[0].State = core.ChainActive
		after.Chains[0].UpdatedAt = now
		after.Traces[i].Status = core.TraceUnfinished
		after.Traces[i].ContentUpdatedAt = now
		in := reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[i].ID, Today: core.LocalDate{Year: 2026, Month: 11, Day: 7}, Now: now, EventIDs: []string{vnID(103)}}
		want := lifecycleSnapshot(t, after, lifecycleEvent(c, in.TraceID, in.EventIDs[0], core.TraceUnfinished, now))
		vnGood(t, want)
		r, e := auditReactivateHistoryVNext(prior, before, after, in)
		must(t, e)
		lifecycleAssert(t, r, want, historyCommandReactivate, in.ChainID, in.TraceID, in.TraceID, in.EventIDs[0], now, in.Today)
	}
}
