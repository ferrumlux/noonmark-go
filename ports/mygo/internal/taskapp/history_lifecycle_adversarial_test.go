// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func lifecycleAbandonDelta(g core.TaskGraph, index int, now core.Instant) core.TaskGraph {
	out := g.Clone()
	out.Traces[index].Status = core.TraceAbandoned
	out.Traces[index].ContentUpdatedAt = now
	out.Traces[index].SettledAt = vnPtr(now)
	out.Traces[index].PinOrder = nil
	for i := range out.Chains {
		if out.Chains[i].ID == out.Traces[index].ChainID {
			out.Chains[i].State = core.ChainAbandoned
			out.Chains[i].UpdatedAt = now
		}
	}
	return out
}
func TestHistoryLifecycleHistoricalRoutingAndOwner(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"L", 5})
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), NewTraceID: core.DayTraceID(vnID(6)), DayID: core.DayID(vnID(7)), TargetDate: tomorrow, Today: tomorrow, Now: vnAt(10)}))
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	after := lifecycleAbandonDelta(before, 1, vnAt(20))
	in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: vnAt(20), EventID: vnID(300)}
	want := lifecycleSnapshot(t, after, lifecycleEvent(f.c, before.Traces[1].ID, in.EventID, core.TraceAbandoned, in.Now))
	vnGood(t, want)
	r, e := auditAbandonHistoryVNext(prior, before, after, in)
	must(t, e)
	lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, before.Traces[1].ID, in.EventID, in.Now, core.LocalDate{})
	// Same effective target, distinct request identity must remain distinct.
	in.TraceID = before.Traces[1].ID
	r, e = auditAbandonHistoryVNext(prior, before, after, in)
	must(t, e)
	lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, in.Now, core.LocalDate{})
	in.TraceID = before.Traces[0].ID
	in.ChainID = core.TaskChainID(vnID(999))
	r, e = auditAbandonHistoryVNext(prior, before, after, in)
	lifecycleZero(t, r, e)
	// Wrong capture-target delta is a legal graph, but not this command.
	in.ChainID = before.Chains[0].ID
	wrong := lifecycleAbandonDelta(before, 0, in.Now)
	r, e = auditAbandonHistoryVNext(prior, before, wrong, in)
	lifecycleZero(t, r, e)
}
func TestHistoryLifecycleNoPendingSelectsRequestedUnfinished(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"L", 5})
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), NewTraceID: core.DayTraceID(vnID(6)), DayID: core.DayID(vnID(7)), TargetDate: tomorrow, Today: tomorrow, Now: vnAt(10)}))
	next, e := tomorrow.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(next, vnAt(15)))
	before := f.k.Snapshot()
	c := lifecycleEvent(f.c, before.Traces[1].ID, vnID(102), core.TraceUnfinished, vnAt(15))
	prior := vnGood(t, lifecycleSnapshot(t, before, c))
	for i := range before.Traces {
		after := lifecycleAbandonDelta(before, i, vnAt(20))
		in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[i].ID, Now: vnAt(20), EventID: vnID(300)}
		if i == 0 {
			// The source has a live continuation edge. Abandoning that source
			// invalidates the descendant's U/D witness; preserve the real core
			// rejection, rather than treating every multi-U request as legal.
			k, err := core.NewTaskKernel(before)
			must(t, err)
			err = k.AbandonChain(core.AbandonChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Now: in.Now})
			var validation *core.ValidationError
			if !errors.As(err, &validation) || !validation.Has("invalidTraceContinuation") {
				t.Fatalf("expected exact continuation invariant, got %v", err)
			}
			t.Log("source Abandon rejected by invalidTraceContinuation; core and profile unchanged")
			r, err := auditAbandonHistoryVNext(prior, before, after, in)
			lifecycleZero(t, r, err)
			continue
		}
		want := lifecycleSnapshot(t, after, lifecycleEvent(c, in.TraceID, in.EventID, core.TraceAbandoned, in.Now))
		vnGood(t, want)
		r, e := auditAbandonHistoryVNext(prior, before, after, in)
		must(t, e)
		lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, in.Now, core.LocalDate{})
	}
}
func TestHistoryLifecycleFailureImmutability(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 10})
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	after := before.Clone()
	after.Chains[0].State = core.ChainActive
	after.Chains[0].UpdatedAt = vnAt(20)
	after.Traces[0].ContentUpdatedAt = vnAt(20)
	after.Traces[0].Status = core.TraceUnfinished
	today, e := vnDate.AddDays(1)
	must(t, e)
	base := reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: today, Now: vnAt(20), EventIDs: []string{vnID(300)}}
	mutations := map[string]func(*reactivateHistoryInput, *core.TaskGraph, *core.TaskGraph){
		"nan": func(in *reactivateHistoryInput, _, _ *core.TaskGraph) {
			in.Now = core.Instant(math.Float64bits(math.NaN()))
		},
		"positive infinity": func(in *reactivateHistoryInput, _, _ *core.TaskGraph) {
			in.Now = core.Instant(math.Float64bits(math.Inf(1)))
		},
		"negative infinity": func(in *reactivateHistoryInput, _, _ *core.TaskGraph) {
			in.Now = core.Instant(math.Float64bits(math.Inf(-1)))
		},
		"local rollback": func(in *reactivateHistoryInput, _, _ *core.TaskGraph) { in.Now = vnAt(9) },
		"bad today":      func(in *reactivateHistoryInput, _, _ *core.TaskGraph) { in.Today = core.LocalDate{} },
		"uuid content": func(in *reactivateHistoryInput, _, _ *core.TaskGraph) {
			in.TraceID = core.DayTraceID(strings.Repeat("z", 36))
		},
		"duplicate old event":   func(in *reactivateHistoryInput, _, _ *core.TaskGraph) { in.EventIDs[0] = strings.ToLower(vnID(101)) },
		"wrong owner":           func(in *reactivateHistoryInput, _, _ *core.TaskGraph) { in.ChainID = core.TaskChainID(vnID(999)) },
		"unbound before":        func(_ *reactivateHistoryInput, b, _ *core.TaskGraph) { b.Definitions[0].Title = "unbound" },
		"unrelated title":       func(_ *reactivateHistoryInput, _, a *core.TaskGraph) { a.Definitions[0].Title = "changed" },
		"settledAt overwritten": func(_ *reactivateHistoryInput, _, a *core.TaskGraph) { a.Traces[0].SettledAt = vnPtr(vnAt(20)) },
		"clock bits":            func(_ *reactivateHistoryInput, _, a *core.TaskGraph) { a.Traces[0].ContentUpdatedAt = vnAt(21) },
		"priority":              func(_ *reactivateHistoryInput, _, a *core.TaskGraph) { a.Traces[0].Priority++ },
		"new identity":          func(_ *reactivateHistoryInput, _, a *core.TaskGraph) { a.Traces[0].ID = core.DayTraceID(vnID(998)) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			in := base
			in.EventIDs = append([]string(nil), base.EventIDs...)
			b, a := before.Clone(), after.Clone()
			mutate(&in, &b, &a)
			oldB, oldA := b.Clone(), a.Clone()
			oldIDs := append([]string(nil), in.EventIDs...)
			oldPrior, e := prior.snapshotCopy()
			must(t, e)
			r, e := auditReactivateHistoryVNext(prior, b, a, in)
			lifecycleZero(t, r, e)
			if !reflect.DeepEqual(b, oldB) || !reflect.DeepEqual(a, oldA) || !reflect.DeepEqual(in.EventIDs, oldIDs) || !historyEqual(prior.proof.snapshot, oldPrior) {
				t.Fatal("failure mutated inputs")
			}
		})
	}
}
func TestHistoryLifecycleRepeatedCompositionAndFutureCancellation(t *testing.T) {
	f := vnSeed(t, vnDate)
	for _, step := range []vnStep{{"A", 5}, {"P", 6}, {"A", 10}, {"P", 11}} {
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		f.run(t, step)
		after := f.k.Snapshot()
		var r auditedHistoryCommand
		var e error
		if step.op == "A" {
			r, e = auditAbandonHistoryVNext(prior, before, after, abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: vnAt(step.at), EventID: vnID(100 + f.captures)})
		} else {
			r, e = auditReactivateHistoryVNext(prior, before, after, reactivateHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: vnAt(step.at)})
		}
		must(t, e)
		got, e := r.snapshotCopy()
		must(t, e)
		if !historyEqual(got, f.snapshot(t)) {
			t.Fatal("composition changed history")
		}
	}
	for _, step := range []vnStep{{"C", 12}, {"undo", 13}, {"C", 14}} {
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		f.run(t, step)
		action := completionHistoryComplete
		if step.op == "undo" {
			action = completionHistoryUndo
		}
		r, e := auditCompletionHistoryVNext(prior, before, f.k.Snapshot(), completionHistoryInput{Action: action, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: vnAt(step.at)})
		must(t, e)
		got, e := r.snapshotCopy()
		must(t, e)
		if !historyEqual(got, f.snapshot(t)) {
			t.Fatal("old completion lost A")
		}
	}
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	f.run(t, vnStep{"L", 20})
	today, e := vnDate.AddDays(1)
	must(t, e)
	r, e := auditSettlementHistoryVNext(prior, before, f.k.Snapshot(), settlementHistoryInput{Today: today, Now: vnAt(20), EventIDs: []string{vnID(100 + f.captures)}})
	must(t, e)
	got, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(got, f.snapshot(t)) {
		t.Fatal("settlement differs")
	}
	future := vnSeed(t, vnDate)
	future.run(t, vnStep{"A", 5}, vnStep{"P", 6})
	vnGood(t, future.snapshot(t))
	future.run(t, vnStep{"X", 10})
	if _, e := inspectForwardClockOrdinaryHistoryVNext(future.snapshot(t)); e == nil {
		t.Fatal("X plus retained A must remain rejected")
	}
}
func TestHistoryLifecycleConcurrentOwnedResults(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	in := abandonHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Now: vnAt(10), EventID: vnID(300)}
	after := lifecycleAbandonDelta(before, 0, in.Now)
	want := lifecycleSnapshot(t, after, lifecycleEvent(f.c, in.TraceID, in.EventID, core.TraceAbandoned, in.Now))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := auditAbandonHistoryVNext(prior, before, after, in)
			if e != nil {
				t.Error(e)
				return
			}
			got, e := r.snapshotCopy()
			if e != nil || !historyEqual(got, want) {
				t.Error("concurrent owned result differs", e)
			}
		}()
	}
	wg.Wait()
	r, e := auditAbandonHistoryVNext(prior, before, after, in)
	must(t, e)
	before.Definitions[0].Title = "caller mutation"
	after.Traces[0].SettledAt = vnPtr(vnAt(99))
	got, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(got, want) {
		t.Fatal("caller mutation reached proof")
	}
}

func TestHistoryLifecycleRichFactsAndMaliciousLeaves(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending child", true: "completed child"}[completed], func(t *testing.T) {
			k := hcRich(t)
			if completed {
				hcRichCompletedChild(t, k)
			}
			before := k.Snapshot()
			extra, e := emptySupplement()
			must(t, e)
			prior := vnGood(t, lifecycleSnapshot(t, before, extra.Classifications))
			target := -1
			for i, tr := range before.Traces {
				if tr.ID == core.DayTraceID(vnID(12)) {
					target = i
				}
			}
			if target < 0 {
				t.Fatal("fixture target missing")
			}
			now := vnAt(10)
			after := lifecycleAbandonDelta(before, target, now)
			in := abandonHistoryInput{ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Now: now, EventID: vnID(400)}
			c := lifecycleEvent(extra.Classifications, in.TraceID, in.EventID, core.TraceAbandoned, now)
			want := lifecycleSnapshot(t, after, c)
			vnGood(t, want)
			r, e := auditAbandonHistoryVNext(prior, before, after, in)
			must(t, e)
			lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, now, core.LocalDate{})
			for name, mutate := range map[string]func(*core.TaskGraph){
				"child title": func(g *core.TaskGraph) { g.Subtasks[0].Title = "changed" },
				"child clock": func(g *core.TaskGraph) { g.Subtasks[0].UpdatedAt = vnAt(9) },
				"description": func(g *core.TaskGraph) { g.Traces[target].DescriptionText = vnPtr("changed") },
				"note body":   func(g *core.TaskGraph) { g.Traces[target].NoteEntries[0].Body = "changed" },
				"planned child": func(g *core.TaskGraph) {
					for i := range g.Definitions {
						if len(g.Definitions[i].PlannedSubtasks) > 0 {
							g.Definitions[i].PlannedSubtasks[0].Title = "changed"
						}
					}
				},
				"unrelated chain": func(g *core.TaskGraph) { g.Chains[0].UpdatedAt = vnAt(9) },
			} {
				t.Run(name, func(t *testing.T) {
					bad := after.Clone()
					mutate(&bad)
					r, e := auditAbandonHistoryVNext(prior, before, bad, in)
					lifecycleZero(t, r, e)
				})
			}
			// Reactivation on an open past date preserves actual children verbatim.
			next := after.Clone()
			next.Traces[target].Status = core.TraceUnfinished
			next.Traces[target].ContentUpdatedAt = vnAt(20)
			// Locate owner independently rather than assuming chain and trace order.
			for i := range next.Chains {
				if next.Chains[i].ID == in.ChainID {
					next.Chains[i].State = core.ChainActive
					next.Chains[i].UpdatedAt = vnAt(20)
				}
			}
			today, e := vnDate.AddDays(1)
			must(t, e)
			ri := reactivateHistoryInput{ChainID: in.ChainID, TraceID: in.TraceID, Today: today, Now: vnAt(20), EventIDs: []string{vnID(401)}}
			rw := lifecycleSnapshot(t, next, lifecycleEvent(c, in.TraceID, ri.EventIDs[0], core.TraceUnfinished, ri.Now))
			vnGood(t, rw)
			rr, e := auditReactivateHistoryVNext(r.proof.after, after, next, ri)
			must(t, e)
			lifecycleAssert(t, rr, rw, historyCommandReactivate, ri.ChainID, ri.TraceID, ri.TraceID, ri.EventIDs[0], ri.Now, ri.Today)
		})
	}
}

// Synthetic independently rooted multi-U shape, not claimed Service-reachable.
// No continuation edge makes either requested U eligible under real core rules.
func TestHistoryLifecycleSyntheticMultiUExplicitTarget(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"L", 5})
	g := f.k.Snapshot()
	date, e := vnDate.AddDays(1)
	must(t, e)
	day := g.Days[0]
	day.ID = core.DayID(vnID(701))
	day.Date = date
	day.LockedAt = vnPtr(vnAt(15))
	day.UpdatedAt = vnAt(15)
	g.Days = append(g.Days, day)
	tr := g.Traces[0]
	tr.ID = core.DayTraceID(vnID(702))
	tr.Date = date
	tr.ContentUpdatedAt = vnAt(15)
	tr.SettledAt = vnPtr(vnAt(15))
	g.Traces = append(g.Traces, tr)
	c := lifecycleEvent(f.c, tr.ID, vnID(703), core.TraceUnfinished, vnAt(15))
	prior := vnGood(t, lifecycleSnapshot(t, g, c))
	for i := range g.Traces {
		after := lifecycleAbandonDelta(g, i, vnAt(20))
		in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[i].ID, Now: vnAt(20), EventID: vnID(704)}
		want := lifecycleSnapshot(t, after, lifecycleEvent(c, in.TraceID, in.EventID, core.TraceAbandoned, in.Now))
		vnGood(t, want)
		r, e := auditAbandonHistoryVNext(prior, g, after, in)
		must(t, e)
		lifecycleAssert(t, r, want, historyCommandAbandon, in.ChainID, in.TraceID, in.TraceID, in.EventID, in.Now, core.LocalDate{})
	}
}
