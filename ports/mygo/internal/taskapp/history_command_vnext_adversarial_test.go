// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func hcWire(t *testing.T, g core.TaskGraph, c canonical.Value) canonical.Value {
	t.Helper()
	extra, e := emptySupplement()
	must(t, e)
	if c.Kind == canonical.Object {
		extra.Classifications = c
	}
	s, e := v7.FromDomain(hcState(g), extra)
	must(t, e)
	return s
}
func hcZero(t *testing.T, r auditedHistoryCommand, e error) {
	t.Helper()
	if e == nil || r.valid() || r.proof != nil {
		t.Fatalf("failure leaked seal: %v", e)
	}
	if _, e = r.snapshotCopy(); e == nil {
		t.Fatal("zero snapshot")
	}
	if _, e = r.changed(); e == nil {
		t.Fatal("zero changed")
	}
}
func hcRich(t *testing.T) *core.TaskKernel {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	// Reverse ID/date layout deliberately differs from deterministic capture order.
	for _, n := range []int{30, 20, 10} {
		desc := "synthetic description"
		note := core.TaskNoteEntry{ID: core.TaskNoteEntryID(vnID(n + 4)), Body: "synthetic note", CreatedAt: vnAt(0), UpdatedAt: vnAt(0)}
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(n)), DefinitionID: core.TaskDefinitionID(vnID(n + 1)), Title: "synthetic", DescriptionText: &desc, InitialNote: &note, Now: vnAt(0)}))
		if n == 10 {
			must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(n)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(n + 1)), SubtaskID: core.PlannedSubtaskID(vnID(n + 5)), LineageID: core.SubtaskLineageID(vnID(n + 6)), Title: "synthetic child", Difficulty: core.DifficultySimple, Now: vnAt(0)}))
		}
		date := vnDate
		if n == 30 {
			date, e = date.AddDays(1)
			must(t, e)
		}
		subIDs := map[core.PlannedSubtaskID]core.SubtaskID{}
		if n == 10 {
			subIDs[core.PlannedSubtaskID(vnID(n+5))] = core.SubtaskID(vnID(n + 7))
		}
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(n)), TraceID: core.DayTraceID(vnID(n + 2)), DayID: core.DayID(vnID(n + 3)), Date: date, Today: vnDate, Now: vnAt(0), SubtaskIDs: subIDs}))
	}
	return k
}
func hcRichCompletedChild(t *testing.T, k *core.TaskKernel) {
	t.Helper()
	must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), SubtaskID: core.SubtaskID(vnID(17)), Today: vnDate, Now: vnAt(1)}))
}

// Test oracle manually projects events; it NEVER calls the command helper,
// capture selector, production builder or production result to derive expected.
func hcProject(c canonical.Value, trace, event string, status core.TraceStatus, at core.Instant, revision uint64) {
	key := historyObject(map[string]canonical.Value{"rawValue": historyString(trace)})
	value := historyObject(map[string]canonical.Value{"id": historyString(event), "traceID": key, "status": historyString(string(status)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(at)), "revision": historyNumber(revision)})
	streams, latest := c.Object["snapshotEventsByTraceID"], c.Object["snapshotsByTraceID"]
	found := false
	for i := 0; i < len(streams.Array); i += 2 {
		if strings.EqualFold(streams.Array[i].Object["rawValue"].String, trace) {
			streams.Array[i+1].Array = append(streams.Array[i+1].Array, value)
			found = true
			break
		}
	}
	if !found {
		streams.Array = append(streams.Array, key, canonical.Value{Kind: canonical.Array, Array: []canonical.Value{value}})
	}
	found = false
	for i := 0; i < len(latest.Array); i += 2 {
		if strings.EqualFold(latest.Array[i].Object["rawValue"].String, trace) {
			latest.Array[i+1] = historyVNextClone(value)
			found = true
			break
		}
	}
	if !found {
		latest.Array = append(latest.Array, historyVNextClone(key), historyVNextClone(value))
	}
	c.Object["snapshotEventsByTraceID"], c.Object["snapshotsByTraceID"], c.Object["revision"] = streams, latest, historyNumber(revision)
}
func TestHistoryCommandDeterministicMultiCapture(t *testing.T) {
	k := hcRich(t)
	hcRichCompletedChild(t, k)
	// Old A stream remains before a new U append, mixed with wholly new streams.
	must(t, k.AbandonChain(core.AbandonChainInput{ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Now: vnAt(2)}))
	_, e := k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Today: vnDate, Now: vnAt(3)})
	must(t, e)
	must(t, k.MarkCompleted(core.DayTraceID(vnID(22)), vnDate, vnAt(4)))
	extra, e := emptySupplement()
	must(t, e)
	hcProject(extra.Classifications, vnID(12), vnID(1000), core.TraceAbandoned, vnAt(2), 1)
	before := k.Snapshot()
	priorWire := hcWire(t, before, extra.Classifications)
	prior := vnGood(t, priorWire)
	today, e := vnDate.AddDays(2)
	must(t, e)
	must(t, k.SettleTaskDays(today, vnAt(5)))
	after := k.Snapshot()
	ids := []string{vnID(1001), vnID(1002), vnID(1003)}
	wantC := historyVNextClone(extra.Classifications)
	for i, item := range []struct {
		id     int
		status core.TraceStatus
	}{{12, core.TraceUnfinished}, {22, core.TraceCompleted}, {32, core.TraceUnfinished}} {
		hcProject(wantC, vnID(item.id), ids[i], item.status, vnAt(5), uint64(i+2))
	}
	want := hcWire(t, after, wantC)
	vnGood(t, want)
	var first canonical.Value
	for i := 0; i < 3; i++ {
		r, e := auditSettlementHistoryVNext(prior, before, after, settlementHistoryInput{Today: today, Now: vnAt(5), EventIDs: ids})
		must(t, e)
		got, e := r.snapshotCopy()
		must(t, e)
		if !historyEqual(got, want) {
			t.Fatal("capture ID/revision/date order")
		}
		if i > 0 && !historyEqual(first, got) {
			t.Fatal("nondeterminism")
		}
		first = got
	}
	// IDs cannot select the target or create an extra capture; builder keeps its
	// own case-insensitive uniqueness and old-ID checks after metadata succeeds.
	for name, bad := range map[string][]string{"missing": ids[:2], "extra": append(append([]string{}, ids...), vnID(1004)), "old": {vnID(1000), ids[1], ids[2]}, "duplicate case": {vnID(0xABC), strings.ToLower(vnID(0xABC)), ids[2]}, "malformed": {strings.Repeat("z", 36), ids[1], ids[2]}} {
		t.Run(name, func(t *testing.T) {
			r, e := auditSettlementHistoryVNext(prior, before, after, settlementHistoryInput{Today: today, Now: vnAt(5), EventIDs: bad})
			hcZero(t, r, e)
		})
	}
	// Even a genuine core result for different Today/Now is not this command.
	for _, in := range []settlementHistoryInput{{Today: vnDate, Now: vnAt(5), EventIDs: ids}, {Today: today, Now: vnAt(6), EventIDs: ids}, {Now: vnAt(5), EventIDs: ids}} {
		r, e := auditSettlementHistoryVNext(prior, before, after, in)
		hcZero(t, r, e)
	}
}
func TestHistoryCommandWholeGraphMaliciousDeltas(t *testing.T) {
	k := hcRich(t)
	hcRichCompletedChild(t, k)
	before := k.Snapshot()
	prior := vnGood(t, hcWire(t, before, canonical.Value{}))
	must(t, k.MarkCompleted(core.DayTraceID(vnID(12)), vnDate, vnAt(5)))
	after := k.Snapshot()
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Today: vnDate, Now: vnAt(5)}
	mutations := map[string]func(*core.TaskGraph){
		"other chain state": func(g *core.TaskGraph) { g.Chains[0].State = core.ChainAbandoned },
		"other chain clock": func(g *core.TaskGraph) { g.Chains[0].UpdatedAt = vnAt(10) },
		"chain note":        func(g *core.TaskGraph) { g.Chains[0].NoteEntries[0].Body = "changed" },
		"chain tombstone": func(g *core.TaskGraph) {
			g.Chains[0].NoteEntries[0].DeletedAt = vnPtr(vnAt(0))
			g.Chains[0].NoteEntries[0].Body = ""
		},
		"definition title":   func(g *core.TaskGraph) { g.Definitions[0].Title = "changed" },
		"definition version": func(g *core.TaskGraph) { g.Definitions[0].Sequence = 2 },
		"definition clock":   func(g *core.TaskGraph) { g.Definitions[0].ContentUpdatedAt = vnAt(10) },
		"planned child":      func(g *core.TaskGraph) { g.Definitions[2].PlannedSubtasks[0].Title = "changed" },
		"trace note":         func(g *core.TaskGraph) { g.Traces[0].NoteEntries[0].Body = "changed" },
		"trace tombstone": func(g *core.TaskGraph) {
			g.Traces[0].NoteEntries[0].DeletedAt = vnPtr(vnAt(0))
			g.Traces[0].NoteEntries[0].Body = ""
		},
		"priority":         func(g *core.TaskGraph) { g.Traces[0].Priority = 100 },
		"manual progress":  func(g *core.TaskGraph) { g.Traces[0].ManualProgressPercent = vnPtr(43) },
		"description":      func(g *core.TaskGraph) { g.Traces[0].DescriptionText = vnPtr("changed") },
		"child title":      func(g *core.TaskGraph) { g.Subtasks[0].Title = "changed" },
		"child difficulty": func(g *core.TaskGraph) { g.Subtasks[0].Difficulty = core.DifficultyHard },
		"child status": func(g *core.TaskGraph) {
			g.Subtasks[0].Status = core.SubtaskAbandoned
			g.Subtasks[0].SettledAt = g.Subtasks[0].CompletedAt
			g.Subtasks[0].CompletedAt = nil
		},
		"remove child": func(g *core.TaskGraph) { g.Subtasks = nil },
		"add empty day": func(g *core.TaskGraph) {
			g.Days = append(g.Days, core.Day{ID: core.DayID(vnID(400)), Date: core.LocalDate{Year: 2030, Month: 1, Day: 1}})
		},
		"remove other aggregate": func(g *core.TaskGraph) {
			g.Chains = g.Chains[1:]
			g.Definitions = g.Definitions[1:]
			g.Traces = g.Traces[1:]
		},
		"day review":              func(g *core.TaskGraph) { g.Days[0].ReviewSummary = vnPtr("changed") },
		"array order":             func(g *core.TaskGraph) { g.Traces[0], g.Traces[1] = g.Traces[1], g.Traces[0] },
		"target completion clock": func(g *core.TaskGraph) { g.Traces[2].CompletedAt = vnPtr(vnAt(4)) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := after.Clone()
			mutate(&bad)
			must(t, core.ValidateTaskGraph(bad))
			saved := bad.Clone()
			r, e := auditCompletionHistoryVNext(prior, before, bad, in)
			hcZero(t, r, e)
			if !reflect.DeepEqual(saved, bad) {
				t.Fatal("failure mutated caller after")
			}
			// Binding rejects equally legitimate changes before the target operation.
			if name != "target completion clock" && name != "child status" {
				bad = before.Clone()
				mutate(&bad)
				must(t, core.ValidateTaskGraph(bad))
				r, e = auditCompletionHistoryVNext(prior, bad, after, in)
				hcZero(t, r, e)
			}
		})
	}
	// A correct completion of another target does not establish this target.
	wrong, e := core.NewTaskKernel(before)
	must(t, e)
	must(t, wrong.MarkCompleted(core.DayTraceID(vnID(22)), vnDate, vnAt(5)))
	r, e := auditCompletionHistoryVNext(prior, before, wrong.Snapshot(), in)
	hcZero(t, r, e)
	for name, mutate := range map[string]func(*completionHistoryInput){"owner": func(x *completionHistoryInput) { x.ChainID = core.TaskChainID(vnID(20)) }, "target": func(x *completionHistoryInput) { x.TraceID = core.DayTraceID(vnID(22)) }, "dangling": func(x *completionHistoryInput) { x.TraceID = core.DayTraceID(vnID(999)) }, "zero action": func(x *completionHistoryInput) { x.Action = 0 }, "unknown action": func(x *completionHistoryInput) { x.Action = 99 }, "undo": func(x *completionHistoryInput) { x.Action = completionHistoryUndo }, "now": func(x *completionHistoryInput) { x.Now = vnAt(6) }, "today": func(x *completionHistoryInput) { x.Today.Day++ }} {
		t.Run(name, func(t *testing.T) {
			bad := in
			mutate(&bad)
			r, e := auditCompletionHistoryVNext(prior, before, after, bad)
			hcZero(t, r, e)
		})
	}
}
func TestHistoryCommandEligibilityAndProfileFailure(t *testing.T) {
	k := hcRich(t)
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Today: vnDate, Now: vnAt(5)}
	r, e := auditCompletionHistoryVNext(prior, g, g, in)
	hcZero(t, r, e)
	if !errors.Is(e, core.ErrOpenSubtasksPreventCompletion) {
		t.Fatal(e)
	}
	for _, steps := range [][]vnStep{{{"C", 2}}, {{"A", 2}}, {{"R", 2}}, {{"L", 2}}} {
		f := vnSeed(t, vnDate)
		f.run(t, steps...)
		g = f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		in.ChainID = g.Chains[0].ID
		in.TraceID = g.Traces[0].ID
		r, e = auditCompletionHistoryVNext(p, g, g, in)
		hcZero(t, r, e)
	}
	for _, date := range []core.LocalDate{{Year: 2026, Month: 10, Day: 31}, {Year: 2026, Month: 11, Day: 2}} {
		f := vnSeed(t, date)
		g = f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		in.ChainID = g.Chains[0].ID
		in.TraceID = g.Traces[0].ID
		r, e = auditCompletionHistoryVNext(p, g, g, in)
		hcZero(t, r, e)
	}
	// Current Go's observed clock policy is intentionally retained, not repaired.
	f := vnSeed(t, vnDate)
	g = f.k.Snapshot()
	g.Chains[0].UpdatedAt = vnAt(10)
	p := vnGood(t, hcWire(t, g, canonical.Value{}))
	in.ChainID = g.Chains[0].ID
	in.TraceID = g.Traces[0].ID
	r, e = auditCompletionHistoryVNext(p, g, g, in)
	hcZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal(e)
	}
	// Core permits silent lock@5 after A@10, but the complete profile does not.
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 10})
	before := f.k.Snapshot()
	p = vnGood(t, f.snapshot(t))
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(tomorrow, vnAt(5)))
	after := f.k.Snapshot()
	r, e = auditSettlementHistoryVNext(p, before, after, settlementHistoryInput{Today: tomorrow, Now: vnAt(5)})
	hcZero(t, r, e)
	var profile *historyVNextError
	if !errors.As(e, &profile) || profile.Reason != historyProfile {
		t.Fatal("full after inspector skipped", e)
	}
}
func TestHistoryCommandClocksDatesAndNoop(t *testing.T) {
	dates := []core.LocalDate{{Year: 1, Month: 1, Day: 1}, {Year: 9999, Month: 12, Day: 31}, vnDate}
	for _, date := range dates {
		f := vnSeed(t, date)
		g := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		for _, now := range []core.Instant{vnAt(0), vnAt(math.SmallestNonzeroFloat64), vnAt(0.125), vnAt(math.MaxFloat64)} {
			k, e := core.NewTaskKernel(g)
			must(t, e)
			must(t, k.MarkCompleted(g.Traces[0].ID, date, now))
			in := completionHistoryInput{Action: completionHistoryComplete, ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: date, Now: now}
			r, e := auditCompletionHistoryVNext(p, g, k.Snapshot(), in)
			must(t, e)
			if changed, e := r.changed(); e != nil || !changed {
				t.Fatal("equal/max completion is a mutation")
			}
			w, e := r.snapshotCopy()
			must(t, e)
			completed := k.Snapshot()
			cp := vnGood(t, w)
			must(t, k.UndoCompleted(g.Traces[0].ID, date, now))
			in.Action = completionHistoryUndo
			r, e = auditCompletionHistoryVNext(cp, completed, k.Snapshot(), in)
			must(t, e)
			r, e = auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: date, Now: now})
			must(t, e)
			changed, e := r.changed()
			must(t, e)
			if changed {
				t.Fatal("today no-op mutated")
			}
		}
	}
	// Negative finite clocks use a fresh source construction, not a global rule.
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(1)), DefinitionID: core.TaskDefinitionID(vnID(2)), Title: "negative", Now: vnAt(-10)}))
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), DayID: core.DayID(vnID(4)), Date: vnDate, Today: vnDate, Now: vnAt(-10)}))
	before := k.Snapshot()
	p := vnGood(t, hcWire(t, before, canonical.Value{}))
	must(t, k.MarkCompleted(before.Traces[0].ID, vnDate, vnAt(-5)))
	r, e := auditCompletionHistoryVNext(p, before, k.Snapshot(), completionHistoryInput{Action: completionHistoryComplete, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: vnAt(-5)})
	must(t, e)
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	p = vnGood(t, f.snapshot(t))
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(tomorrow, vnAt(math.MaxFloat64)))
	r, e = auditSettlementHistoryVNext(p, g, f.k.Snapshot(), settlementHistoryInput{Today: tomorrow, Now: vnAt(math.MaxFloat64), EventIDs: []string{vnID(900)}})
	must(t, e)
	// NaN/Inf reject even when settlement would otherwise be a true no-op.
	for _, now := range []core.Instant{core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), core.Instant(math.Float64bits(math.Inf(-1)))} {
		r, e = auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: vnDate, Now: now})
		hcZero(t, r, e)
		r, e = auditCompletionHistoryVNext(p, g, g, completionHistoryInput{Action: completionHistoryComplete, ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: vnDate, Now: now})
		hcZero(t, r, e)
	}
	for _, today := range []core.LocalDate{{}, {Year: 2026, Month: 2, Day: 29}, {Year: 10000, Month: 1, Day: 1}} {
		r, e = auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: today, Now: vnAt(5)})
		hcZero(t, r, e)
	}
	// Max date cannot acquire a lock. X-only max date stays unchanged too.
	f = vnSeed(t, dates[1])
	f.run(t, vnStep{"X", 1})
	g = f.k.Snapshot()
	p = vnGood(t, f.snapshot(t))
	r, e = auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: dates[1], Now: vnAt(math.MaxFloat64)})
	must(t, e)
	changed, e := r.changed()
	must(t, e)
	if changed {
		t.Fatal("X-only max date locked")
	}
}
func TestHistoryCommandSignedZeroAndFrozenBits(t *testing.T) {
	minus := core.Instant(math.Float64bits(math.Copysign(0, -1)))
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	must(t, f.k.MarkCompleted(before.Traces[0].ID, vnDate, minus))
	after := f.k.Snapshot()
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: minus}
	r, e := auditCompletionHistoryVNext(p, before, after, in)
	must(t, e)
	bad := after.Clone()
	bad.Traces[0].CompletedAt = vnPtr(vnAt(0))
	r, e = auditCompletionHistoryVNext(p, before, bad, in)
	hcZero(t, r, e)
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(tomorrow, minus))
	r, e = auditSettlementHistoryVNext(p, before, f.k.Snapshot(), settlementHistoryInput{Today: tomorrow, Now: minus, EventIDs: []string{vnID(888)}})
	hcZero(t, r, e) // P input cannot submit C output.
	// Actual completion's C capture retains completedAt bits and locks at +0.
	completed, e := auditCompletionHistoryVNext(p, before, after, in)
	must(t, e)
	wire, e := completed.snapshotCopy()
	must(t, e)
	cp := vnGood(t, wire)
	ck, e := core.NewTaskKernel(after)
	must(t, e)
	must(t, ck.SettleTaskDays(tomorrow, vnAt(0)))
	r, e = auditSettlementHistoryVNext(cp, after, ck.Snapshot(), settlementHistoryInput{Today: tomorrow, Now: vnAt(0), EventIDs: []string{vnID(889)}})
	must(t, e)
	got, e := r.snapshotCopy()
	must(t, e)
	if got.Object["traces"].Array[0].Object["completedAt"].TextNumber != historyNumber(uint64(minus)).TextNumber {
		t.Fatal("anchor zero normalized")
	}
	// Both endpoints can be necessary-condition valid while rewriting old event
	// bits; prefix still rejects. This is a layer test, not a nonexistent API input.
	mutated := historyVNextClone(got)
	vnUpdateEvent(mutated, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(minus)) })
	mutated.Object["days"].Array[0].Object["lockedAt"] = historyNumber(uint64(minus))
	mutated.Object["days"].Array[0].Object["updatedAt"] = historyNumber(uint64(minus))
	a, b := vnGood(t, got), vnGood(t, mutated)
	if checkForwardHistorySuccessor(a, b) == nil {
		t.Fatal("old signed-zero lock/event rewritten")
	}
}
func TestHistoryCommandImmutableAliasesAndConcurrentProof(t *testing.T) {
	k := hcRich(t)
	hcRichCompletedChild(t, k)
	before := k.Snapshot()
	p := vnGood(t, hcWire(t, before, canonical.Value{}))
	must(t, k.MarkCompleted(core.DayTraceID(vnID(12)), vnDate, vnAt(3)))
	after := k.Snapshot()
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), Today: vnDate, Now: vnAt(3)}
	r, e := auditCompletionHistoryVNext(p, before, after, in)
	must(t, e)
	want, e := r.snapshotCopy()
	must(t, e)
	old, e := p.snapshotCopy()
	must(t, e)
	before.Definitions[0].Title = "caller changed"
	after.Traces[0].NoteEntries[0].Body = "caller changed"
	copy, e := r.snapshotCopy()
	must(t, e)
	copy.Object["traces"].Array[0].Object["status"] = historyString("bogus")
	fresh, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(want, fresh) || !historyEqual(old, p.proof.snapshot) {
		t.Fatal("result/prior aliases caller or returned copy")
	}
	state, _, e := v7.ToDomain(old)
	must(t, e)
	shared := state.Tasks
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				local, e := core.NewTaskKernel(shared)
				if e != nil {
					t.Error(e)
					return
				}
				ci := in
				if i%2 == 1 {
					ci.TraceID = core.DayTraceID(vnID(22))
					ci.ChainID = core.TaskChainID(vnID(20))
				}
				if e = local.MarkCompleted(ci.TraceID, ci.Today, ci.Now); e != nil {
					t.Error(e)
					return
				}
				out, e := auditCompletionHistoryVNext(p, shared, local.Snapshot(), ci)
				if e != nil {
					t.Error(e)
					return
				}
				v, e := out.snapshotCopy()
				if e != nil {
					t.Error(e)
					return
				}
				v.Object["days"] = canonical.Value{}
				if _, e = out.snapshotCopy(); e != nil {
					t.Error(e)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	if !historyEqual(old, p.proof.snapshot) {
		t.Fatal("shared prior changed")
	}
	r, e = auditCompletionHistoryVNext(validatedHistorySnapshot{}, shared, shared, in)
	hcZero(t, r, e)
	r, e = auditSettlementHistoryVNext(validatedHistorySnapshot{}, shared, shared, settlementHistoryInput{})
	hcZero(t, r, e)
}

func TestHistoryCommandMixedXAndUnrelatedFrontier(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"X", 1})
	must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(10)), DefinitionID: core.TaskDefinitionID(vnID(11)), Title: "companion", Now: vnAt(2)}))
	must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(10)), TraceID: core.DayTraceID(vnID(12)), DayID: core.DayID(vnID(13)), Date: vnDate, Today: vnDate, Now: vnAt(2)}))
	// An unrelated pool-only chain has a later clock. This is not a Service clock.
	must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(20)), DefinitionID: core.TaskDefinitionID(vnID(21)), Title: "unrelated later", Now: vnAt(100)}))
	before := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	today, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(today, vnAt(5)))
	after := f.k.Snapshot()
	ids := []string{vnID(500)}
	result, e := auditSettlementHistoryVNext(p, before, after, settlementHistoryInput{Today: today, Now: vnAt(5), EventIDs: ids})
	must(t, e)
	got, e := result.snapshotCopy()
	must(t, e)
	if !reflect.DeepEqual(before.Traces[0], after.Traces[0]) || after.Days[0].LockedAt == nil {
		t.Fatal("mixed X changed or companion failed to lock")
	}
	if got.Object["classifications"].Object["revision"].TextNumber != "1" {
		t.Fatal("X captured")
	}
	ids[0] = vnID(999)
	after.Days[0].LockedAt = vnPtr(vnAt(100))
	copy, e := result.snapshotCopy()
	must(t, e)
	if !historyEqual(got, copy) {
		t.Fatal("result aliases IDs/graph")
	}
	// An explicit empty prior date does not manufacture a day lock either.
	emptyGraph := core.TaskGraph{Days: []core.Day{{ID: core.DayID(vnID(90)), Date: vnDate}}}
	emptyPrior := vnGood(t, hcWire(t, emptyGraph, canonical.Value{}))
	r, e := auditSettlementHistoryVNext(emptyPrior, emptyGraph, emptyGraph, settlementHistoryInput{Today: today, Now: vnAt(math.MaxFloat64)})
	must(t, e)
	changed, e := r.changed()
	must(t, e)
	if changed {
		t.Fatal("empty day locked")
	}
}
func TestHistoryCommandIDRepresentationAndHistoricalPrefixLayers(t *testing.T) {
	wire, g, _ := iaPath(t, vnDate, iaNow(0), iaStep{"A", iaNow(1)}, iaStep{"P", iaNow(2)}, iaStep{"A", iaNow(3)}, iaStep{"P", iaNow(4)})
	// Lowercase task rawValues are accepted by the necessary-condition reader,
	// whose bridge uppercases IDs, but cannot bind exactly back to their bytes.
	lower := historyVNextClone(wire)
	var lowerIDs func(canonical.Value)
	lowerIDs = func(v canonical.Value) {
		for k, c := range v.Object {
			if k == "rawValue" {
				v.Object[k] = historyString(strings.ToLower(c.String))
			} else {
				lowerIDs(c)
			}
		}
		for _, c := range v.Array {
			lowerIDs(c)
		}
	}
	for _, key := range []string{"days", "chains", "definitions", "traces", "subtasks"} {
		lowerIDs(lower.Object[key])
	}
	p := vnGood(t, lower)
	state, _, e := v7.ToDomain(lower)
	must(t, e)
	k, e := core.NewTaskKernel(state.Tasks)
	must(t, e)
	must(t, k.MarkCompleted(state.Tasks.Traces[0].ID, vnDate, vnAt(5)))
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: vnDate, Now: vnAt(5)}
	r, e := auditCompletionHistoryVNext(p, state.Tasks, k.Snapshot(), in)
	hcZero(t, r, e)
	var typed *historyCommandError
	if !errors.As(e, &typed) || typed.Reason != historyCommandBinding {
		t.Fatal("task representation not bound", e)
	}
	if !historyEqual(lower, p.proof.snapshot) {
		t.Fatal("lowercase prior normalized")
	}
	// Classification event IDs AND pair keys/traceIDs preserve their original
	// bytes through complete, undo and an append onto that existing stream.
	lower = historyVNextClone(wire)
	lowerIDs(lower.Object["classifications"])
	for _, key := range []string{"snapshotEventsByTraceID", "snapshotsByTraceID"} {
		pairs := lower.Object["classifications"].Object[key]
		for i := 1; i < len(pairs.Array); i += 2 {
			v := pairs.Array[i]
			if v.Kind == canonical.Array {
				for _, event := range v.Array {
					event.Object["id"] = historyString(strings.ToLower(event.Object["id"].String))
				}
			} else {
				v.Object["id"] = historyString(strings.ToLower(v.Object["id"].String))
			}
		}
	}
	p = vnGood(t, lower)
	r, e = auditCompletionHistoryVNext(p, g, k.Snapshot(), in)
	must(t, e)
	completed, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(lower.Object["classifications"], completed.Object["classifications"]) {
		t.Fatal("classification bytes normalized")
	}
	completedGraph := k.Snapshot()
	must(t, k.UndoCompleted(g.Traces[0].ID, vnDate, vnAt(6)))
	in.Action = completionHistoryUndo
	in.Now = vnAt(6)
	r, e = auditCompletionHistoryVNext(vnGood(t, completed), completedGraph, k.Snapshot(), in)
	must(t, e)
	undone, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(lower.Object["classifications"], undone.Object["classifications"]) {
		t.Fatal("undo classification bytes changed")
	}
	before := k.Snapshot()
	today, e := vnDate.AddDays(1)
	must(t, e)
	must(t, k.SettleTaskDays(today, vnAt(7)))
	r, e = auditSettlementHistoryVNext(vnGood(t, undone), before, k.Snapshot(), settlementHistoryInput{Today: today, Now: vnAt(7), EventIDs: []string{vnID(500)}})
	must(t, e)
	final, e := r.snapshotCopy()
	must(t, e)
	oldPairs, newPairs := lower.Object["classifications"].Object["snapshotEventsByTraceID"].Array, final.Object["classifications"].Object["snapshotEventsByTraceID"].Array
	if !historyEqual(oldPairs[0], newPairs[0]) || !historyEqual(oldPairs[1].Array[0], newPairs[1].Array[0]) || !historyEqual(oldPairs[1].Array[1], newPairs[1].Array[1]) {
		t.Fatal("append rewrote old event/key")
	}
	// These malicious full snapshots are layer tests: command input has no
	// caller classifications. Each old-ID/old-clock/removal endpoint is itself
	// reader-valid, so only checking final inspector would be insufficient.
	old := vnGood(t, wire)
	for name, mutate := range map[string]func(canonical.Value){
		"ID": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["id"] = historyString(iaID(999)) })
		},
		"clock": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(vnAt(1.5))) })
		},
		"remove A and renumber": func(s canonical.Value) {
			c := s.Object["classifications"]
			a := c.Object["snapshotEventsByTraceID"]
			a.Array[1].Array = a.Array[1].Array[1:]
			c.Object["snapshotEventsByTraceID"] = a
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["revision"] = historyNumber(1) })
			c.Object["revision"] = historyNumber(1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := historyVNextClone(wire)
			mutate(bad)
			next := vnGood(t, bad)
			if checkForwardHistorySuccessor(old, next) == nil {
				t.Fatal("old prefix rewritten")
			}
			if !historyEqual(old.proof.snapshot, wire) {
				t.Fatal("prefix failure mutated prior")
			}
		})
	}
}
func TestHistoryCommandScopeAndOldReaderStayClosed(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
	base := f.snapshot(t)
	g := f.k.Snapshot()
	for name, mutate := range map[string]func(canonical.Value){
		"preferences": func(s canonical.Value) {
			s.Object["preferences"].Object["themeLanguageWriterID"] = historyString("synthetic other")
		},
		"catalog": func(s canonical.Value) {
			s.Object["classifications"].Object["categories"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyString("unsupported")}}
		},
		"ideas": func(s canonical.Value) {
			s.Object["ideas"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyString("unsupported")}}
		},
		"series": func(s canonical.Value) {
			s.Object["taskCycleSeries"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyString("unsupported")}}
		},
		"membership": func(s canonical.Value) {
			s.Object["chains"].Array[0].Object["cycleMembership"] = historyObject(map[string]canonical.Value{"seriesID": historyObject(map[string]canonical.Value{"rawValue": historyString(vnID(99))}), "occurrenceDate": s.Object["traces"].Array[0].Object["date"]})
		},
		"changed": func(s canonical.Value) { s.Object["traces"].Array[0].Object["status"] = historyString("changed") },
	} {
		t.Run(name, func(t *testing.T) {
			bad := historyVNextClone(base)
			mutate(bad)
			p, e := inspectForwardClockOrdinaryHistoryVNext(bad)
			if e == nil || p.valid() {
				t.Fatal("scope expanded")
			}
			r, e := auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: vnDate, Now: vnAt(3)})
			hcZero(t, r, e)
		})
	}
	for _, steps := range [][]vnStep{{{"A", 1}}, {{"A", 1}, {"P", 2}, {"A", 3}}, {{"A", 1}, {"U", 2}}} {
		x := vnSeed(t, vnDate)
		x.run(t, steps...)
		s := x.snapshot(t)
		p := vnGood(t, s)
		before := x.k.Snapshot()
		r, e := auditSettlementHistoryVNext(p, before, before, settlementHistoryInput{Today: vnDate, Now: vnAt(4)})
		must(t, e)
		got, e := r.snapshotCopy()
		must(t, e)
		if ValidateEmptyHistory(got) == nil {
			t.Fatal("old reader expanded")
		}
		if _, e = encodeSnapshot(got); e == nil {
			t.Fatal("old codec callback expanded")
		}
	}
}

func TestHistoryCommandSettlementExactDeltaAndAtomicFailure(t *testing.T) {
	k := hcRich(t)
	before := k.Snapshot()
	old := hcWire(t, before, canonical.Value{})
	p := vnGood(t, old)
	today, e := vnDate.AddDays(1)
	must(t, e)
	must(t, k.SettleTaskDays(today, vnAt(5)))
	after := k.Snapshot()
	in := settlementHistoryInput{Today: today, Now: vnAt(5), EventIDs: []string{vnID(900), vnID(901)}}
	r, e := auditSettlementHistoryVNext(p, before, after, in)
	must(t, e)
	out, e := r.snapshotCopy()
	must(t, e)
	// Today's future-date trace stays pending, its objects and chain clocks frozen.
	if after.Traces[0].Status != core.TracePending || after.Subtasks[0].Status != core.SubtaskUnfinished || *after.Subtasks[0].SettledAt != vnAt(5) {
		t.Fatal("wrong settlement facts")
	}
	for name, mutate := range map[string]func(*core.TaskGraph){
		"chain touch":           func(g *core.TaskGraph) { g.Chains[2].UpdatedAt = vnAt(5) },
		"definition":            func(g *core.TaskGraph) { g.Definitions[1].Title = "changed" },
		"other date":            func(g *core.TaskGraph) { g.Traces[0].Priority = 99 },
		"pending child omitted": func(g *core.TaskGraph) { g.Subtasks[0] = before.Subtasks[0] },
		"child title":           func(g *core.TaskGraph) { g.Subtasks[0].Title = "changed" },
		"extra day lock":        func(g *core.TaskGraph) { g.Days[0].LockedAt = vnPtr(vnAt(5)); g.Days[0].UpdatedAt = vnAt(5) },
		"array order":           func(g *core.TaskGraph) { g.Days[0], g.Days[1] = g.Days[1], g.Days[0] },
	} {
		t.Run(name, func(t *testing.T) {
			bad := after.Clone()
			mutate(&bad)
			must(t, core.ValidateTaskGraph(bad))
			savedBefore, savedAfter := before.Clone(), bad.Clone()
			ids := append([]string{}, in.EventIDs...)
			r, e := auditSettlementHistoryVNext(p, before, bad, in)
			hcZero(t, r, e)
			if !reflect.DeepEqual(before, savedBefore) || !reflect.DeepEqual(bad, savedAfter) || !reflect.DeepEqual(ids, in.EventIDs) || !historyEqual(p.proof.snapshot, old) {
				t.Fatal("replay failure mutated inputs")
			}
		})
	}
	// Builder-stage failure leaves every input and sealed prior untouched.
	savedBefore, savedAfter := before.Clone(), after.Clone()
	badIDs := []string{vnID(0xABC), strings.ToLower(vnID(0xABC))}
	bad := in
	bad.EventIDs = badIDs
	r, e = auditSettlementHistoryVNext(p, before, after, bad)
	hcZero(t, r, e)
	var typed *historyVNextError
	if !errors.As(e, &typed) {
		t.Fatal("duplicate IDs not rejected by append", e)
	}
	if !reflect.DeepEqual(before, savedBefore) || !reflect.DeepEqual(after, savedAfter) || badIDs[1] != strings.ToLower(vnID(0xABC)) || !historyEqual(old, p.proof.snapshot) {
		t.Fatal("append failure mutated inputs")
	}
	// Existing old locks cannot be retimed during a subsequent no-op.
	next := vnGood(t, out)
	wrong := after.Clone()
	wrong.Days[1].LockedAt = vnPtr(vnAt(6))
	wrong.Days[1].UpdatedAt = vnAt(6)
	must(t, core.ValidateTaskGraph(wrong))
	r, e = auditSettlementHistoryVNext(next, after, wrong, settlementHistoryInput{Today: today, Now: vnAt(7)})
	hcZero(t, r, e)
}
