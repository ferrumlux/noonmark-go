// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Independent fixture projection: no vn*/author test helpers, command helper,
// capture helper or append builder constructs these expected history values.
func icID(n int) string { return fmt.Sprintf("ABCDEF00-ABCD-4000-8000-%012X", n) }
func icAt(n float64) core.Instant {
	v, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return v
}
func icP[T any](x T) *T { return &x }
func icOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func icBytes(t *testing.T, v canonical.Value) []byte {
	t.Helper()
	b, e := canonical.Marshal(v)
	icOK(t, e)
	return b
}
func icEqual(t *testing.T, a, b canonical.Value) {
	t.Helper()
	if !bytes.Equal(icBytes(t, a), icBytes(t, b)) {
		t.Fatal("complete snapshot bytes differ")
	}
}
func icObj(m map[string]canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Object, Object: m}
}
func icStr(s string) canonical.Value { return canonical.Value{Kind: canonical.String, String: s} }
func icNum(n uint64) canonical.Value {
	return canonical.Value{Kind: canonical.Number, TextNumber: fmt.Sprint(n)}
}
func icKey(s string) canonical.Value { return icObj(map[string]canonical.Value{"rawValue": icStr(s)}) }

type icEvent struct {
	id, trace string
	status    core.TraceStatus
	at        core.Instant
}

func icSnapshot(t *testing.T, g core.TaskGraph, events []icEvent) canonical.Value {
	t.Helper()
	extra, e := emptySupplement()
	icOK(t, e)
	// Explicit schema projection in source event order; no tested builder oracle.
	c := extra.Classifications
	latest, streams := canonical.Value{Kind: canonical.Array}, canonical.Value{Kind: canonical.Array}
	positions := map[string]int{}
	for i, x := range events {
		ev := icObj(map[string]canonical.Value{"id": icStr(x.id), "traceID": icKey(x.trace), "status": icStr(string(x.status)), "labels": {Kind: canonical.Array}, "capturedAt": icNum(uint64(x.at)), "revision": icNum(uint64(i + 1))})
		j, ok := positions[x.trace]
		if !ok {
			j = len(streams.Array)
			positions[x.trace] = j
			streams.Array = append(streams.Array, icKey(x.trace), canonical.Value{Kind: canonical.Array})
			latest.Array = append(latest.Array, icKey(x.trace), canonical.Value{})
		}
		streams.Array[j+1].Array = append(streams.Array[j+1].Array, ev)
		latest.Array[j+1] = ev
	}
	c.Object["snapshotsByTraceID"] = latest
	c.Object["snapshotEventsByTraceID"] = streams
	c.Object["revision"] = icNum(uint64(len(events)))
	extra.Classifications = c
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	icOK(t, e)
	return s
}
func icProof(t *testing.T, g core.TaskGraph, ev []icEvent) validatedHistorySnapshot {
	t.Helper()
	p, e := inspectForwardClockOrdinaryHistoryVNext(icSnapshot(t, g, ev))
	icOK(t, e)
	return p
}
func icKernel(t *testing.T, g core.TaskGraph) *core.TaskKernel {
	t.Helper()
	k, e := core.NewTaskKernel(g)
	icOK(t, e)
	return k
}
func icSeed(t *testing.T, k *core.TaskKernel, n int, d, today core.LocalDate, now core.Instant) {
	t.Helper()
	icOK(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(icID(n)), DefinitionID: core.TaskDefinitionID(icID(n + 1)), Title: fmt.Sprint("synthetic ", n), DescriptionText: icP("description"), InitialNote: &core.TaskNoteEntry{ID: core.TaskNoteEntryID(icID(n + 3)), Body: "note", CreatedAt: now, UpdatedAt: now}, Now: now}))
	icOK(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(icID(n)), TraceID: core.DayTraceID(icID(n + 2)), DayID: core.DayID(icID(n + 4)), Date: d, Today: today, Now: now}))
}

var icDate = core.LocalDate{Year: 2026, Month: 2, Day: 27}

func icNext(t *testing.T, d core.LocalDate, n int) core.LocalDate {
	t.Helper()
	v, e := d.AddDays(n)
	icOK(t, e)
	return v
}
func icSource(t *testing.T, status string) (*core.TaskKernel, []icEvent) {
	t.Helper()
	k := icKernel(t, core.TaskGraph{})
	d := icDate
	today := d
	if status == "X" {
		today = icNext(t, d, -1)
	}
	icSeed(t, k, 100, d, today, icAt(0))
	ch, tr := core.TaskChainID(icID(100)), core.DayTraceID(icID(102))
	var ev []icEvent
	switch status {
	case "C":
		icOK(t, k.MarkCompleted(tr, d, icAt(1)))
	case "A", "U", "AP":
		icOK(t, k.AbandonChain(core.AbandonChainInput{ChainID: ch, TraceID: tr, Now: icAt(1)}))
		ev = append(ev, icEvent{icID(900), string(tr), core.TraceAbandoned, icAt(1)})
		if status != "A" {
			day := d
			if status == "U" {
				day = icNext(t, d, 1)
			}
			_, e := k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: ch, TraceID: tr, Today: day, Now: icAt(2)})
			icOK(t, e)
			if status == "U" {
				ev = append(ev, icEvent{icID(901), string(tr), core.TraceUnfinished, icAt(2)})
			}
		}
	case "R", "X":
		icOK(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: ch, TraceID: tr, NewDefinitionID: core.TaskDefinitionID(icID(105)), CancellationID: core.CancellationID(icID(106)), Today: today, Now: icAt(1)}))
		if status == "R" {
			ev = append(ev, icEvent{icID(900), string(tr), core.TraceReturnedToPool, icAt(1)})
		}
	case "D":
		icOK(t, k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: ch, TraceID: tr, NewTraceID: core.DayTraceID(icID(107)), DayID: core.DayID(icID(108)), Today: d, TargetDate: icNext(t, d, 1), Now: icAt(1)}))
		ev = append(ev, icEvent{icID(900), string(tr), core.TraceDeferred, icAt(1)})
	}
	return k, ev
}
func icResult(t *testing.T, r auditedHistoryCommand, expected canonical.Value, changed bool) {
	t.Helper()
	s, e := r.snapshotCopy()
	icOK(t, e)
	icEqual(t, s, expected)
	b, e := r.changed()
	icOK(t, e)
	if b != changed {
		t.Fatalf("changed=%v want %v", b, changed)
	}
}
func icZero(t *testing.T, r auditedHistoryCommand, e error) {
	t.Helper()
	if e == nil || r.proof != nil {
		t.Fatalf("wanted zero result and rejection, got %v, %v", r, e)
	}
	if _, e = r.snapshotCopy(); e == nil {
		t.Fatal("zero result snapshot")
	}
	if _, e = r.changed(); e == nil {
		t.Fatal("zero result changed")
	}
}
func icUnchanged(t *testing.T, p validatedHistorySnapshot, prior []byte, b, a, oldB, oldA core.TaskGraph, ids, oldIDs []string) {
	t.Helper()
	s, e := p.snapshotCopy()
	icOK(t, e)
	if !bytes.Equal(prior, icBytes(t, s)) || !reflect.DeepEqual(b, oldB) || !reflect.DeepEqual(a, oldA) || !reflect.DeepEqual(ids, oldIDs) {
		t.Fatal("input mutated")
	}
}

func TestICSourceSettlement(t *testing.T) {
	for _, status := range []string{"P", "C", "A", "U", "R", "D", "X", "AP"} {
		t.Run(status, func(t *testing.T) {
			k, old := icSource(t, status)
			before := k.Snapshot()
			p := icProof(t, before, old)
			today := icNext(t, icDate, 1)
			icOK(t, k.SettleTaskDays(today, icAt(3)))
			after := k.Snapshot()
			events := append([]icEvent(nil), old...)
			var ids []string
			if status == "P" || status == "AP" || status == "C" {
				s := core.TraceUnfinished
				if status == "C" {
					s = core.TraceCompleted
				}
				ids = []string{icID(950)}
				events = append(events, icEvent{ids[0], icID(102), s, icAt(3)})
			}
			r, e := auditSettlementHistoryVNext(p, before, after, settlementHistoryInput{today, icAt(3), ids})
			icOK(t, e)
			expected := icSnapshot(t, after, events)
			icResult(t, r, expected, status != "X")
			if status == "C" && *after.Traces[0].CompletedAt != icAt(1) {
				t.Fatal("lost actual completion")
			}
			if status == "U" && *after.Traces[0].SettledAt != icAt(1) {
				t.Fatal("reactivation settledAt rewritten")
			}
			p2, e := inspectForwardClockOrdinaryHistoryVNext(expected)
			icOK(t, e)
			r, e = auditSettlementHistoryVNext(p2, after, after, settlementHistoryInput{today, icAt(math.MaxFloat64), nil})
			icOK(t, e)
			icResult(t, r, expected, false)
		})
	}
	t.Run("mixed-X", func(t *testing.T) {
		k, old := icSource(t, "X")
		icSeed(t, k, 200, icDate, icDate, icAt(2))
		b := k.Snapshot()
		p := icProof(t, b, old)
		icOK(t, k.SettleTaskDays(icNext(t, icDate, 1), icAt(3)))
		a := k.Snapshot()
		if !reflect.DeepEqual(b.Traces[0], a.Traces[0]) {
			t.Fatal("X changed")
		}
		ev := []icEvent{{icID(950), icID(202), core.TraceUnfinished, icAt(3)}}
		r, e := auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{icNext(t, icDate, 1), icAt(3), []string{icID(950)}})
		icOK(t, e)
		icResult(t, r, icSnapshot(t, a, ev), true)
	})
}

func TestICStableCaptureOrder(t *testing.T) {
	k := icKernel(t, core.TaskGraph{})
	d2 := icNext(t, icDate, 1)
	icSeed(t, k, 300, d2, icDate, icAt(0))
	icSeed(t, k, 200, icDate, icDate, icAt(0))
	icSeed(t, k, 100, icDate, icDate, icAt(0))
	icOK(t, k.MarkCompleted(core.DayTraceID(icID(202)), icDate, icAt(1)))
	b := k.Snapshot()
	p := icProof(t, b, nil)
	icOK(t, k.SettleTaskDays(icNext(t, icDate, 2), icAt(3)))
	a := k.Snapshot()
	ids := []string{icID(990), icID(991), icID(992)}
	want := icSnapshot(t, a, []icEvent{{ids[0], icID(102), core.TraceUnfinished, icAt(3)}, {ids[1], icID(202), core.TraceCompleted, icAt(3)}, {ids[2], icID(302), core.TraceUnfinished, icAt(3)}})
	for i := 0; i < 3; i++ {
		r, e := auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{icNext(t, icDate, 2), icAt(3), ids})
		icOK(t, e)
		icResult(t, r, want, true)
	}
	for _, bad := range [][]string{ids[:2], append(append([]string(nil), ids...), icID(993)), {ids[0], ids[0], ids[2]}, {ids[0], strings.ToLower(ids[0]), ids[2]}, {strings.Repeat("z", 36), ids[1], ids[2]}} {
		r, e := auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{icNext(t, icDate, 2), icAt(3), bad})
		icZero(t, r, e)
	}
}

func TestICCompletionRetainsHistory(t *testing.T) {
	for _, lower := range []bool{false, true} {
		t.Run(fmt.Sprint(lower), func(t *testing.T) {
			k, ev := icSource(t, "AP")
			if lower {
				ev[0].id = strings.ToLower(ev[0].id)
				ev[0].trace = strings.ToLower(ev[0].trace)
			}
			for i, action := range []completionHistoryAction{completionHistoryComplete, completionHistoryUndo, completionHistoryComplete, completionHistoryUndo, completionHistoryComplete} {
				b := k.Snapshot()
				p := icProof(t, b, ev)
				now := icAt(float64(3 + i))
				var e error
				if action == completionHistoryComplete {
					e = k.MarkCompleted(core.DayTraceID(icID(102)), icDate, now)
				} else {
					e = k.UndoCompleted(core.DayTraceID(icID(102)), icDate, now)
				}
				icOK(t, e)
				a := k.Snapshot()
				r, e := auditCompletionHistoryVNext(p, b, a, completionHistoryInput{action, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, now})
				icOK(t, e)
				icResult(t, r, icSnapshot(t, a, ev), true)
			}
			b := k.Snapshot()
			p := icProof(t, b, ev)
			today := icNext(t, icDate, 1)
			icOK(t, k.SettleTaskDays(today, icAt(9)))
			a := k.Snapshot()
			r, e := auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{today, icAt(9), []string{icID(999)}})
			icOK(t, e)
			// Canonical pair key retains old lowercase, while newly appended event is upper.
			expected := icSnapshot(t, a, ev)
			c := expected.Object["classifications"]
			newEvent := icObj(map[string]canonical.Value{"id": icStr(icID(999)), "traceID": icKey(icID(102)), "status": icStr(string(core.TraceCompleted)), "labels": {Kind: canonical.Array}, "capturedAt": icNum(uint64(icAt(9))), "revision": icNum(2)})
			streams := c.Object["snapshotEventsByTraceID"]
			streams.Array[1].Array = append(streams.Array[1].Array, newEvent)
			c.Object["snapshotEventsByTraceID"] = streams
			latest := c.Object["snapshotsByTraceID"]
			latest.Array[1] = newEvent
			c.Object["snapshotsByTraceID"] = latest
			c.Object["revision"] = icNum(2)
			expected.Object["classifications"] = c
			icResult(t, r, expected, true)
			if *a.Traces[0].CompletedAt != icAt(7) {
				t.Fatal("complete clock lost at lock")
			}
			if ValidateEmptyHistory(expected) == nil {
				t.Fatal("old reader accepted new history")
			}
		})
	}
}

func TestICWholeGraphAndGuardFailures(t *testing.T) {
	k, ev := icSource(t, "AP")
	icSeed(t, k, 200, icDate, icDate, icAt(0))
	b := k.Snapshot()
	p := icProof(t, b, ev)
	icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), icDate, icAt(4)))
	a := k.Snapshot()
	in := completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(4)}
	ps, _ := p.snapshotCopy()
	pb := icBytes(t, ps)
	mutations := map[string]func(*core.TaskGraph){
		"chain-clock": func(g *core.TaskGraph) { g.Chains[1].UpdatedAt = icAt(5) },
		"chain-note":  func(g *core.TaskGraph) { g.Chains[1].NoteEntries[0].Body = "other" },
		"trace-note-tombstone": func(g *core.TaskGraph) {
			n := &g.Traces[1].NoteEntries[0]
			n.Body = ""
			n.UpdatedAt = icAt(5)
			n.DeletedAt = icP(icAt(5))
		},
		"definition-title":  func(g *core.TaskGraph) { g.Definitions[1].Title = "different" },
		"definition-clock":  func(g *core.TaskGraph) { g.Definitions[1].ContentUpdatedAt = icAt(5) },
		"trace-progress":    func(g *core.TaskGraph) { g.Traces[1].ManualProgressPercent = icP(42) },
		"trace-priority":    func(g *core.TaskGraph) { g.Traces[1].Priority = 8 },
		"day-review":        func(g *core.TaskGraph) { g.Days[0].ReviewTomorrowNote = icP("different") },
		"trace-description": func(g *core.TaskGraph) { g.Traces[1].DescriptionText = icP("different") },
		"array-order":       func(g *core.TaskGraph) { g.Traces[0], g.Traces[1] = g.Traces[1], g.Traces[0] },
		"extra-chain": func(g *core.TaskGraph) {
			g.Chains = append(g.Chains, core.TaskChain{ID: core.TaskChainID(icID(700)), State: core.ChainActive, CreatedAt: icAt(0), UpdatedAt: icAt(0)})
			g.Definitions = append(g.Definitions, core.TaskDefinition{ID: core.TaskDefinitionID(icID(701)), ChainID: core.TaskChainID(icID(700)), Sequence: 1, Title: "extra", CreatedAt: icAt(0), ContentUpdatedAt: icAt(0)})
		},
	}
	for name, m := range mutations {
		for _, side := range []string{"before", "after"} {
			t.Run(name+"/"+side, func(t *testing.T) {
				bb, aa := b.Clone(), a.Clone()
				if side == "before" {
					m(&bb)
				} else {
					m(&aa)
				}
				icOK(t, core.ValidateTaskGraph(bb))
				icOK(t, core.ValidateTaskGraph(aa))
				ob, oa := bb.Clone(), aa.Clone()
				r, e := auditCompletionHistoryVNext(p, bb, aa, in)
				icZero(t, r, e)
				icUnchanged(t, p, pb, bb, aa, ob, oa, nil, nil)
			})
		}
	}
	bad := []completionHistoryInput{in, in, in, in, in, in, in, in}
	bad[0].ChainID = core.TaskChainID(icID(200))
	bad[1].TraceID = core.DayTraceID(icID(202))
	bad[2].Today = icNext(t, icDate, 1)
	bad[3].Today = icNext(t, icDate, -1)
	bad[4].Now = icAt(5)
	bad[5].Action = completionHistoryUndo
	bad[6].Action = 0
	bad[7].TraceID = core.DayTraceID(icID(800))
	for _, x := range bad {
		r, e := auditCompletionHistoryVNext(p, b, a, x)
		icZero(t, r, e)
	}
	r, e := auditCompletionHistoryVNext(validatedHistorySnapshot{}, b, a, in)
	icZero(t, r, e)
	// Settlement independently rejects unrelated valid deltas too.
	k = icKernel(t, b)
	icOK(t, k.SettleTaskDays(icNext(t, icDate, 1), icAt(4)))
	sa := k.Snapshot()
	for name, m := range mutations {
		t.Run("settle-"+name, func(t *testing.T) {
			x := sa.Clone()
			m(&x)
			if e := core.ValidateTaskGraph(x); e != nil {
				t.Fatal(e)
			}
			r, e := auditSettlementHistoryVNext(p, b, x, settlementHistoryInput{icNext(t, icDate, 1), icAt(4), []string{icID(950), icID(951)}})
			icZero(t, r, e)
		})
	}
}

func TestICChildOwnerDateClockGuards(t *testing.T) {
	k, _ := icSource(t, "P")
	icOK(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(icID(100)), TraceID: core.DayTraceID(icID(102)), SubtaskID: core.SubtaskID(icID(500)), LineageID: core.SubtaskLineageID(icID(501)), Title: "child", Difficulty: core.DifficultySimple, Today: icDate, Now: icAt(1)}))
	b := k.Snapshot()
	p := icProof(t, b, nil)
	in := completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(2)}
	r, e := auditCompletionHistoryVNext(p, b, b, in)
	icZero(t, r, e)
	if e != core.ErrOpenSubtasksPreventCompletion {
		t.Fatalf("unexpected child guard: %v", e)
	}
	icOK(t, k.SettleTaskDays(icNext(t, icDate, 1), icAt(2)))
	a := k.Snapshot()
	r, e = auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{icNext(t, icDate, 1), icAt(2), []string{icID(950)}})
	icOK(t, e)
	icResult(t, r, icSnapshot(t, a, []icEvent{{icID(950), icID(102), core.TraceUnfinished, icAt(2)}}), true)
	if a.Subtasks[0].Status != core.SubtaskUnfinished || *a.Subtasks[0].SettledAt != icAt(2) {
		t.Fatal("child settlement")
	}
	for _, undo := range []bool{false, true} {
		k, _ = icSource(t, "P")
		if undo {
			icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), icDate, icAt(1)))
		}
		b = k.Snapshot()
		b.Chains[0].UpdatedAt = icAt(10)
		p = icProof(t, b, nil)
		in.Now = icAt(5)
		in.Action = completionHistoryComplete
		if undo {
			in.Action = completionHistoryUndo
		}
		r, e = auditCompletionHistoryVNext(p, b, b, in)
		icZero(t, r, e)
		if e != core.ErrInvalidClock {
			t.Fatalf("owner clock: %v", e)
		}
	}
}

func TestICExactClocksAndDates(t *testing.T) {
	for _, seconds := range []float64{-2, -0.25, math.SmallestNonzeroFloat64, 0, math.Copysign(0, -1), math.MaxFloat64} {
		t.Run(fmt.Sprintf("bits-%x", math.Float64bits(seconds)), func(t *testing.T) {
			k := icKernel(t, core.TaskGraph{})
			start := seconds
			if seconds == 0 {
				start = 0
			}
			icSeed(t, k, 100, icDate, icDate, icAt(start))
			b := k.Snapshot()
			p := icProof(t, b, nil)
			now := icAt(seconds)
			icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), icDate, now))
			a := k.Snapshot()
			in := completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, now}
			r, e := auditCompletionHistoryVNext(p, b, a, in)
			icOK(t, e)
			icResult(t, r, icSnapshot(t, a, nil), true)
			if uint64(now) == 1<<63 {
				bad := a.Clone()
				bad.Traces[0].CompletedAt = icP(icAt(0))
				r, e = auditCompletionHistoryVNext(p, b, bad, in)
				icZero(t, r, e)
			}
			pp := icProof(t, a, nil)
			icOK(t, k.UndoCompleted(core.DayTraceID(icID(102)), icDate, now))
			undo := k.Snapshot()
			in.Action = completionHistoryUndo
			r, e = auditCompletionHistoryVNext(pp, a, undo, in)
			icOK(t, e)
			icResult(t, r, icSnapshot(t, undo, nil), true)
		})
	}
	for _, d := range []core.LocalDate{{Year: 1, Month: 1, Day: 1}, {Year: 9999, Month: 12, Day: 31}} {
		k := icKernel(t, core.TaskGraph{})
		icSeed(t, k, 100, d, d, icAt(0))
		b := k.Snapshot()
		p := icProof(t, b, nil)
		r, e := auditSettlementHistoryVNext(p, b, b, settlementHistoryInput{d, icAt(math.MaxFloat64), nil})
		icOK(t, e)
		icResult(t, r, icSnapshot(t, b, nil), false)
		icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), d, icAt(1)))
		a := k.Snapshot()
		r, e = auditCompletionHistoryVNext(p, b, a, completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), d, icAt(1)})
		icOK(t, e)
		icResult(t, r, icSnapshot(t, a, nil), true)
	}
	k, _ := icSource(t, "P")
	b := k.Snapshot()
	p := icProof(t, b, nil)
	for _, now := range []core.Instant{core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), core.Instant(math.Float64bits(math.Inf(-1)))} {
		r, e := auditSettlementHistoryVNext(p, b, b, settlementHistoryInput{icDate, now, nil})
		icZero(t, r, e)
	}
	for _, d := range []core.LocalDate{{}, {Year: 2026, Month: 2, Day: 29}, {Year: 10000, Month: 1, Day: 1}} {
		r, e := auditSettlementHistoryVNext(p, b, b, settlementHistoryInput{d, icAt(1), nil})
		icZero(t, r, e)
	}
	// Pure helper does not impose global frontier of untouched chains.
	icSeed(t, k, 200, icNext(t, icDate, 2), icDate, icAt(20))
	b = k.Snapshot()
	p = icProof(t, b, nil)
	icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), icDate, icAt(1)))
	a := k.Snapshot()
	r, e := auditCompletionHistoryVNext(p, b, a, completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(1)})
	icOK(t, e)
	icResult(t, r, icSnapshot(t, a, nil), true)
}

func TestICCopyConcurrencyAndTaskCase(t *testing.T) {
	k, ev := icSource(t, "AP")
	b := k.Snapshot()
	p := icProof(t, b, ev)
	ps, _ := p.snapshotCopy()
	pb := icBytes(t, ps)
	icOK(t, k.MarkCompleted(core.DayTraceID(icID(102)), icDate, icAt(3)))
	a := k.Snapshot()
	in := completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(3)}
	want := icSnapshot(t, a, ev)
	r, e := auditCompletionHistoryVNext(p, b, a, in)
	icOK(t, e)
	bc, ac := b.Clone(), a.Clone()
	b.Traces[0].NoteEntries[0].Body = "caller mutation"
	a.Definitions[0].DescriptionText = icP("caller mutation")
	icResult(t, r, want, true)
	returned, e := r.snapshotCopy()
	icOK(t, e)
	returned.Object["traces"].Array[0].Object["status"] = icStr("wrong")
	icResult(t, r, want, true)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				rr, e := auditCompletionHistoryVNext(p, bc, ac, in)
				if e != nil {
					t.Error(e)
					return
				}
				s, e := rr.snapshotCopy()
				if e != nil {
					t.Error(e)
					return
				}
				if !bytes.Equal(icBytes(t, s), icBytes(t, want)) {
					t.Error("parallel mismatch")
				}
				s.Object["days"] = canonical.Value{Kind: canonical.Array}
			}
		}()
	}
	wg.Wait()
	ps, _ = p.snapshotCopy()
	if !bytes.Equal(pb, icBytes(t, ps)) {
		t.Fatal("proof changed")
	}
	// Reader permits lowercase task UUID, but bridge exact binding must reject.
	lower := icSnapshot(t, bc, ev)
	id := lower.Object["traces"].Array[0].Object["id"]
	id.Object["rawValue"] = icStr(strings.ToLower(icID(102)))
	lower.Object["traces"].Array[0].Object["id"] = id
	lp, e := inspectForwardClockOrdinaryHistoryVNext(lower)
	icOK(t, e)
	state, _, e := v7.ToDomain(lower)
	icOK(t, e)
	rr, e := auditCompletionHistoryVNext(lp, state.Tasks, ac, in)
	icZero(t, rr, e)
	copy, e := lp.snapshotCopy()
	icOK(t, e)
	icEqual(t, lower, copy)
}

func TestICFailureStagesAndLayeredHistory(t *testing.T) {
	// Core can silently lock an abandoned trace with a clock older than its
	// capture; full-profile validation must reject even though replay is exact.
	k, ev := icSource(t, "A")
	g := k.Snapshot()
	g.Traces[0].ContentUpdatedAt = icAt(10)
	g.Traces[0].SettledAt = icP(icAt(10))
	g.Chains[0].UpdatedAt = icAt(10)
	ev[0].at = icAt(10)
	p := icProof(t, g, ev)
	k = icKernel(t, g)
	today := icNext(t, icDate, 1)
	icOK(t, k.SettleTaskDays(today, icAt(5)))
	a := k.Snapshot()
	ps, _ := p.snapshotCopy()
	pb := icBytes(t, ps)
	ob, oa := g.Clone(), a.Clone()
	r, e := auditSettlementHistoryVNext(p, g, a, settlementHistoryInput{today, icAt(5), nil})
	icZero(t, r, e)
	var he *historyVNextError
	if !errors.As(e, &he) {
		t.Fatalf("expected final history rejection, got %T %v", e, e)
	}
	icUnchanged(t, p, pb, g, a, ob, oa, nil, nil)
	// Builder conflict after exact replay also returns no result and no mutation.
	k, ev = icSource(t, "AP")
	g = k.Snapshot()
	p = icProof(t, g, ev)
	ps, _ = p.snapshotCopy()
	pb = icBytes(t, ps)
	icOK(t, k.SettleTaskDays(today, icAt(4)))
	a = k.Snapshot()
	ids := []string{strings.ToLower(ev[0].id)}
	oldIDs := append([]string(nil), ids...)
	ob, oa = g.Clone(), a.Clone()
	r, e = auditSettlementHistoryVNext(p, g, a, settlementHistoryInput{today, icAt(4), ids})
	icZero(t, r, e)
	icUnchanged(t, p, pb, g, a, ob, oa, ids, oldIDs)
	// Endpoints may both pass the necessary-conditions inspector, while changing
	// old event identity/time remains forbidden by the separate prefix layer.
	for _, change := range []string{"identity", "clock"} {
		other := append([]icEvent(nil), ev...)
		if change == "identity" {
			other[0].id = icID(777)
		} else {
			other[0].at = icAt(1.5)
		}
		next := icProof(t, g, other)
		if checkForwardHistorySuccessor(p, next) == nil {
			t.Fatal("old prefix rewritten", change)
		}
	}
	// A missing newly required U capture is rejected by the complete inspector.
	if _, e = inspectForwardClockOrdinaryHistoryVNext(icSnapshot(t, a, ev)); e == nil {
		t.Fatal("missing terminal capture accepted")
	}
	// API never receives the malicious classification above as its after graph.
	// Its protection is fixed construction plus mandatory final inspector/prefix.
	if ValidateEmptyHistory(icSnapshot(t, g, ev)) == nil {
		t.Fatal("old reader accepted A history")
	}
	_, openU := icSource(t, "U")
	uk, _ := icSource(t, "U")
	if ValidateEmptyHistory(icSnapshot(t, uk.Snapshot(), openU)) == nil {
		t.Fatal("old reader accepted open U history")
	}
}

func TestICAdditionalGuardsAndSettlementCopies(t *testing.T) {
	for _, status := range []string{"P", "C"} {
		k, ev := icSource(t, status)
		b := k.Snapshot()
		p := icProof(t, b, ev)
		action := completionHistoryUndo
		if status == "C" {
			action = completionHistoryComplete
		}
		r, e := auditCompletionHistoryVNext(p, b, b, completionHistoryInput{action, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(2)})
		icZero(t, r, e)
		if e != core.ErrInvalidTransition {
			t.Fatalf("same state guard %v", e)
		}
	}
	// Isolated domain-valid child status fixture. This is not a Service history.
	k, _ := icSource(t, "P")
	icOK(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(icID(100)), TraceID: core.DayTraceID(icID(102)), SubtaskID: core.SubtaskID(icID(500)), LineageID: core.SubtaskLineageID(icID(501)), Title: "child", Difficulty: core.DifficultySimple, Today: icDate, Now: icAt(1)}))
	g := k.Snapshot()
	g.Subtasks[0].Status = core.SubtaskUnfinished
	g.Subtasks[0].SettledAt = icP(icAt(1))
	icOK(t, core.ValidateTaskGraph(g))
	p := icProof(t, g, nil)
	r, e := auditCompletionHistoryVNext(p, g, g, completionHistoryInput{completionHistoryComplete, core.TaskChainID(icID(100)), core.DayTraceID(icID(102)), icDate, icAt(2)})
	icZero(t, r, e)
	if e != core.ErrOpenSubtasksPreventCompletion {
		t.Fatal(e)
	}
	// Successful settlement result cannot alias either graph or EventIDs.
	k, _ = icSource(t, "P")
	b := k.Snapshot()
	p = icProof(t, b, nil)
	icOK(t, k.SettleTaskDays(icNext(t, icDate, 1), icAt(3)))
	a := k.Snapshot()
	ids := []string{icID(950)}
	r, e = auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{icNext(t, icDate, 1), icAt(3), ids})
	icOK(t, e)
	want := icSnapshot(t, a, []icEvent{{icID(950), icID(102), core.TraceUnfinished, icAt(3)}})
	bc, ac := b.Clone(), a.Clone()
	ids[0] = icID(999)
	a.Traces[0].NoteEntries[0].Body = "mutated"
	b.Definitions[0].DescriptionText = icP("mutated")
	icResult(t, r, want, true)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr, e := auditSettlementHistoryVNext(p, bc, ac, settlementHistoryInput{icNext(t, icDate, 1), icAt(3), []string{icID(950)}})
			if e != nil {
				t.Error(e)
				return
			}
			icResult(t, rr, want, true)
		}()
	}
	wg.Wait()
	// Mutating only a valid child title after exact lock is still an illegal delta.
	k = icKernel(t, g)
	icOK(t, k.SettleTaskDays(icNext(t, icDate, 1), icAt(3)))
	a = k.Snapshot()
	a.Subtasks[0].Title = "unrelated child edit"
	icOK(t, core.ValidateTaskGraph(a))
	p = icProof(t, g, nil)
	r, e = auditSettlementHistoryVNext(p, g, a, settlementHistoryInput{icNext(t, icDate, 1), icAt(3), []string{icID(950)}})
	icZero(t, r, e)
	// Max-date cancellation has no legal later date to lock and remains a no-op.
	max := core.LocalDate{Year: 9999, Month: 12, Day: 31}
	earlier := icNext(t, max, -1)
	k = icKernel(t, core.TaskGraph{})
	icSeed(t, k, 100, max, earlier, icAt(0))
	icOK(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: core.TaskChainID(icID(100)), TraceID: core.DayTraceID(icID(102)), NewDefinitionID: core.TaskDefinitionID(icID(105)), CancellationID: core.CancellationID(icID(106)), Today: earlier, Now: icAt(1)}))
	g = k.Snapshot()
	p = icProof(t, g, nil)
	r, e = auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{max, icAt(math.MaxFloat64), nil})
	icOK(t, e)
	icResult(t, r, icSnapshot(t, g, nil), false)
}
