// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func vnID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func vnAt(n float64) core.Instant {
	v, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return v
}
func vnPtr[T any](v T) *T { return &v }

type vnStep struct {
	op string
	at float64
}
type vnFixture struct {
	k        *core.TaskKernel
	c        canonical.Value
	date     core.LocalDate
	captures int
}

func vnSeed(t *testing.T, date core.LocalDate) *vnFixture {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(1)), DefinitionID: core.TaskDefinitionID(vnID(2)), Title: "synthetic", Now: vnAt(0)}))
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), DayID: core.DayID(vnID(4)), Date: date, Today: date, Now: vnAt(0)}))
	extra, e := emptySupplement()
	must(t, e)
	return &vnFixture{k: k, c: extra.Classifications, date: date}
}

// These test captures are explicit projection fixtures, not builder-generated
// evidence, and are not described as Service/writer output.
func (f *vnFixture) capture(status core.TraceStatus, at core.Instant) {
	f.captures++
	key := historyObject(map[string]canonical.Value{"rawValue": historyString(vnID(3))})
	event := historyObject(map[string]canonical.Value{"id": historyString(vnID(100 + f.captures)), "traceID": key, "status": historyString(string(status)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(at)), "revision": historyNumber(uint64(f.captures))})
	streams := f.c.Object["snapshotEventsByTraceID"]
	if len(streams.Array) == 0 {
		streams.Array = []canonical.Value{key, {Kind: canonical.Array, Array: []canonical.Value{event}}}
	} else {
		streams.Array[1].Array = append(streams.Array[1].Array, event)
	}
	f.c.Object["snapshotEventsByTraceID"] = streams
	f.c.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{key, historyVNextClone(event)}}
	f.c.Object["revision"] = historyNumber(uint64(f.captures))
}
func (f *vnFixture) run(t *testing.T, steps ...vnStep) {
	t.Helper()
	chain, trace := core.TaskChainID(vnID(1)), core.DayTraceID(vnID(3))
	for _, s := range steps {
		at := vnAt(s.at)
		switch s.op {
		case "A":
			must(t, f.k.AbandonChain(core.AbandonChainInput{ChainID: chain, TraceID: trace, Now: at}))
			f.capture(core.TraceAbandoned, at)
		case "P", "U":
			today := f.date
			if s.op == "U" {
				var e error
				today, e = f.date.AddDays(1)
				must(t, e)
			}
			_, e := f.k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: chain, TraceID: trace, Today: today, Now: at})
			must(t, e)
			if f.k.Snapshot().Traces[0].Status == core.TraceUnfinished {
				f.capture(core.TraceUnfinished, at)
			}
		case "C":
			must(t, f.k.MarkCompleted(trace, f.date, at))
		case "undo":
			must(t, f.k.UndoCompleted(trace, f.date, at))
		case "L":
			before := f.k.Snapshot()
			today, e := f.date.AddDays(1)
			must(t, e)
			must(t, f.k.SettleTaskDays(today, at))
			if before.Days[0].LockedAt == nil {
				switch before.Traces[0].Status {
				case core.TracePending:
					f.capture(core.TraceUnfinished, at)
				case core.TraceCompleted:
					f.capture(core.TraceCompleted, at)
				}
			}
		case "R", "X":
			today := f.date
			if s.op == "X" {
				var e error
				today, e = f.date.AddDays(-1)
				must(t, e)
			}
			must(t, f.k.ReturnToPool(core.ReturnToPoolInput{ChainID: chain, TraceID: trace, NewDefinitionID: core.TaskDefinitionID(vnID(8)), CancellationID: core.CancellationID(vnID(9)), Today: today, Now: at}))
			if s.op == "R" {
				f.capture(core.TraceReturnedToPool, at)
			}
		case "D":
			tomorrow, e := f.date.AddDays(1)
			must(t, e)
			must(t, f.k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: chain, TraceID: trace, NewTraceID: core.DayTraceID(vnID(6)), DayID: core.DayID(vnID(7)), TargetDate: tomorrow, Today: f.date, Now: at}))
			f.capture(core.TraceDeferred, at)
		default:
			t.Fatalf("unknown fixture action %s", s.op)
		}
	}
}
func (f *vnFixture) snapshot(t *testing.T) canonical.Value {
	t.Helper()
	extra, e := emptySupplement()
	must(t, e)
	extra.Classifications = f.c
	s, e := v7.FromDomain(recurrence.State{Tasks: f.k.Snapshot()}, extra)
	must(t, e)
	return s
}
func vnGood(t *testing.T, s canonical.Value) validatedHistorySnapshot {
	t.Helper()
	v, e := inspectForwardClockOrdinaryHistoryVNext(s)
	must(t, e)
	if !v.valid() || v.proof.stats.MaxSlots > 84 || v.proof.stats.MaxClosureRounds > 84 {
		t.Fatal("proof/bound invalid")
	}
	return v
}
func vnBad(t *testing.T, s canonical.Value, reason historyVNextReason) {
	t.Helper()
	v, e := inspectForwardClockOrdinaryHistoryVNext(s)
	if e == nil || v.valid() {
		t.Fatal("mutation accepted")
	}
	var typed *historyVNextError
	if !errors.As(e, &typed) {
		t.Fatal("untyped failure", e)
	}
	if reason != "" && typed.Reason != reason {
		t.Fatalf("got %v, want %s", e, reason)
	}
}

var vnDate = core.LocalDate{Year: 2026, Month: 11, Day: 1}

func TestHistoryVNextCoreCommandProjectionMatrix(t *testing.T) {
	cases := map[string][]vnStep{
		"empty pending": {}, "empty completed": {{"C", 5}}, "future X": {{"X", 5}},
		"single U": {{"L", 5}}, "single C": {{"C", 5}, {"L", 10}}, "single R": {{"R", 5}}, "single D": {{"D", 5}},
		"repeated A": {{"A", 5}, {"P", 6}, {"A", 10}}, "pending latest A": {{"A", 5}, {"P", 6}},
		"completed latest A": {{"A", 5}, {"P", 6}, {"C", 10}}, "completed anchor after undo": {{"C", 1}, {"undo", 2}, {"A", 5}, {"P", 6}, {"C", 10}, {"L", 20}},
		"A P R": {{"A", 5}, {"P", 6}, {"R", 10}}, "A P D": {{"A", 5}, {"P", 6}, {"D", 10}},
		"locked repeated U A U": {{"L", 5}, {"A", 10}, {"U", 15}, {"A", 20}, {"U", 25}},
		"A then lock":           {{"A", 10}, {"L", 20}}, "lock then A": {{"L", 5}, {"A", 10}},
		// Explicit source-supported synthetic read extension, NOT Service reachable.
		"open U extension": {{"A", 10}, {"U", 20}}, "open U later silent lock": {{"A", 10}, {"U", 20}, {"L", 30}},
		"open repeated U": {{"A", 5}, {"U", 10}, {"A", 15}, {"U", 20}},
		"equal branching": {{"A", 5}, {"U", 5}, {"A", 5}, {"U", 5}, {"L", 5}},
		"locked R":        {{"A", 5}, {"P", 6}, {"R", 10}, {"L", 20}}, "locked D": {{"A", 5}, {"P", 6}, {"D", 10}, {"L", 20}},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) { f := vnSeed(t, vnDate); f.run(t, steps...); s := f.snapshot(t); vnGood(t, s) })
	}
	// A real existing narrow capture remains accepted by the candidate.
	before, g, after := historyFixture(t)
	s, e := CaptureEmptyHistory(before, g, after)
	must(t, e)
	vnGood(t, s)
	must(t, ValidateEmptyHistory(s))
}

func TestHistoryVNextOldReaderStillClosed(t *testing.T) {
	for _, steps := range [][]vnStep{{{"A", 5}}, {{"A", 5}, {"P", 6}}, {{"A", 5}, {"U", 6}}, {{"L", 5}, {"A", 6}, {"U", 7}}} {
		f := vnSeed(t, vnDate)
		f.run(t, steps...)
		s := f.snapshot(t)
		vnGood(t, s)
		if ValidateEmptyHistory(s) == nil {
			t.Fatal("old reader expanded")
		}
		if _, e := encodeSnapshot(s); e == nil {
			t.Fatal("old codec save audit expanded")
		}
	}
}
func TestHistoryVNextProfileAndCompletionWitness(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 10}, vnStep{"L", 5}) // Real core output, unsupported profile, not source-invalid.
	vnBad(t, f.snapshot(t), historyProfile)
	for _, locked := range []bool{false, true} {
		f := vnSeed(t, vnDate)
		f.run(t, vnStep{"A", 10}, vnStep{"P", 11}, vnStep{"C", 15})
		if locked {
			f.run(t, vnStep{"L", 20})
		}
		s := f.snapshot(t)
		vnGood(t, s)
		s.Object["traces"].Array[0].Object["completedAt"] = historyNumber(uint64(vnAt(5)))
		vnBad(t, s, historyCompletion)
	}
}
func TestHistoryVNextLockAutomatonMutations(t *testing.T) {
	for _, word := range []string{"UAA", "UAC", "UAD", "UAR", "UU", "CA", "RA", "DA"} {
		t.Run(word, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			last := word[len(word)-1:]
			switch last {
			case "A":
				f.run(t, vnStep{"A", 5}, vnStep{"L", 5})
			case "C":
				f.run(t, vnStep{"C", 5}, vnStep{"L", 5})
			case "D":
				f.run(t, vnStep{"D", 5}, vnStep{"L", 5})
			case "R":
				f.run(t, vnStep{"R", 5}, vnStep{"L", 5})
			case "U":
				f.run(t, vnStep{"L", 5})
			}
			extra, e := emptySupplement()
			must(t, e)
			f.c = extra.Classifications
			f.captures = 0
			statuses := map[rune]core.TraceStatus{'U': core.TraceUnfinished, 'A': core.TraceAbandoned, 'C': core.TraceCompleted, 'R': core.TraceReturnedToPool, 'D': core.TraceDeferred}
			for _, r := range word {
				f.capture(statuses[r], vnAt(5))
			}
			vnBad(t, f.snapshot(t), historyProfile)
		})
	}
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"U", 10})
	s := f.snapshot(t)
	s.Object["traces"].Array[0].Object["settledAt"] = historyNumber(uint64(vnAt(6)))
	vnBad(t, s, historyOrder)
	// Core future cancellation succeeds after A->P. Aggregate refuses and keeps history.
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"P", 6}, vnStep{"X", 10})
	s = f.snapshot(t)
	bytesBefore, _ := canonical.Marshal(s)
	vnBad(t, s, historyOrder)
	bytesAfter, _ := canonical.Marshal(s)
	if string(bytesBefore) != string(bytesAfter) {
		t.Fatal("audit erased X history")
	}
}
func TestHistoryVNextDateEndpoints(t *testing.T) {
	min := core.LocalDate{Year: 1, Month: 1, Day: 1}
	max := core.LocalDate{Year: 9999, Month: 12, Day: 31}
	for _, steps := range [][]vnStep{{{"A", 5}, {"U", 10}}, {{"A", 5}, {"U", 10}, {"L", 20}}, {{"L", 5}}} {
		f := vnSeed(t, min)
		f.run(t, steps...)
		vnGood(t, f.snapshot(t))
	}
	f := vnSeed(t, max)
	f.run(t, vnStep{"A", 5}, vnStep{"P", 10}, vnStep{"C", 15})
	vnGood(t, f.snapshot(t))
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"U", 10})
	s := f.snapshot(t)
	setDate := func(s canonical.Value, d core.LocalDate) {
		v := historyObject(map[string]canonical.Value{"year": historyNumber(uint64(d.Year)), "month": historyNumber(uint64(d.Month)), "day": historyNumber(uint64(d.Day))})
		s.Object["days"].Array[0].Object["date"] = historyVNextClone(v)
		s.Object["traces"].Array[0].Object["date"] = v
	}
	setDate(s, max)
	vnBad(t, s, historyToday)
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"L", 5})
	s = f.snapshot(t)
	setDate(s, max)
	vnBad(t, s, historyToday)
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"X", 5})
	s = f.snapshot(t)
	setDate(s, min)
	vnBad(t, s, "")
}
func vnUpdateEvent(s canonical.Value, index int, fn func(map[string]canonical.Value)) {
	c := s.Object["classifications"]
	stream := c.Object["snapshotEventsByTraceID"].Array[1]
	fn(stream.Array[index].Object)
	if index == len(stream.Array)-1 {
		c.Object["snapshotsByTraceID"].Array[1] = historyVNextClone(stream.Array[index])
	}
}
func TestHistoryVNextGlobalAndScopeMutations(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"P", 6}, vnStep{"A", 10})
	base := f.snapshot(t)
	mutations := map[string]func(canonical.Value){
		"duplicate UUID case": func(s canonical.Value) {
			vnUpdateEvent(s, 1, func(o map[string]canonical.Value) { o["id"] = historyString(strings.ToLower(vnID(101))) })
		},
		"duplicate revision": func(s canonical.Value) {
			vnUpdateEvent(s, 1, func(o map[string]canonical.Value) { o["revision"] = historyNumber(1) })
		},
		"gap revision": func(s canonical.Value) {
			vnUpdateEvent(s, 1, func(o map[string]canonical.Value) { o["revision"] = historyNumber(3) })
		},
		"huge revision": func(s canonical.Value) {
			s.Object["classifications"].Object["revision"] = historyNumber(math.MaxUint64)
		},
		"clock backwards": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(vnAt(11))) })
		},
		"key mismatch": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[0].Object["rawValue"] = historyString(vnID(99))
		},
		"stale latest": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotsByTraceID"].Array[1].Object["revision"] = historyNumber(1)
		},
		"empty stream": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array = nil
		},
		"missing latest": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array}
		},
		"dangling trace": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["traceID"].Object["rawValue"] = historyString(vnID(99)) })
		},
		"event pending": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["status"] = historyString("pending") })
		},
		"event changed": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["status"] = historyString("changed") })
		},
		"NaN": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(math.Float64bits(math.NaN())) })
		},
		"Inf": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(math.Float64bits(math.Inf(1))) })
		},
		"preferences": func(s canonical.Value) {
			s.Object["preferences"].Object["themeLanguageWriterID"] = historyString("other")
		},
		"unknown supplement": func(s canonical.Value) { s.Object["unknown"] = historyNumber(1) },
		"catalog": func(s canonical.Value) {
			s.Object["classifications"].Object["changeRecords"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyString("unsupported")}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) { s := historyVNextClone(base); mutate(s); vnBad(t, s, "") })
	}
}
func TestHistoryVNextFloatBitsAndEqualOrders(t *testing.T) {
	for _, n := range []float64{-math.MaxFloat64, -2.5, -math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0, math.SmallestNonzeroFloat64, 2.5, math.MaxFloat64} {
		t.Run(fmt.Sprintf("%016X", math.Float64bits(n)), func(t *testing.T) {
			f := vnSeed(t, vnDate)
			f.run(t, vnStep{"A", 5}, vnStep{"U", 5}, vnStep{"L", 5})
			s := f.snapshot(t)
			// Mutation fixture: shift all object/event clocks to the same finite value,
			// preserving exact bits. This probes representation, not command provenance.
			var clocks func(canonical.Value)
			clocks = func(v canonical.Value) {
				for k, x := range v.Object {
					if strings.HasSuffix(k, "At") && x.Kind == canonical.Number {
						v.Object[k] = historyNumber(math.Float64bits(n))
					} else {
						clocks(x)
					}
				}
				for _, x := range v.Array {
					clocks(x)
				}
			}
			clocks(s)
			// Neutral preference's clock is intentionally fixed to +0.
			s.Object["preferences"].Object["themeLanguageUpdatedAt"] = historyNumber(0)
			vnGood(t, s)
		})
	}
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 0}, vnStep{"U", 0}, vnStep{"L", 0})
	s := f.snapshot(t)
	vnGood(t, s)
	// Same numerical zero does not authorize changing a copied retained token.
	s.Object["traces"].Array[0].Object["settledAt"] = historyNumber(math.Float64bits(math.Copysign(0, -1)))
	vnBad(t, s, historyOrder)
	// Numeric chronological order is opposite uint64 order for negative seconds.
	f = vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"P", 6}, vnStep{"A", 10})
	s = f.snapshot(t)
	for _, section := range []string{"days", "chains", "definitions", "traces"} {
		for _, v := range s.Object[section].Array {
			v.Object["createdAt"] = historyNumber(uint64(vnAt(-10)))
			if _, ok := v.Object["updatedAt"]; ok {
				v.Object["updatedAt"] = historyNumber(uint64(vnAt(-1)))
			}
			if _, ok := v.Object["contentUpdatedAt"]; ok {
				v.Object["contentUpdatedAt"] = historyNumber(uint64(vnAt(-1)))
			}
		}
	}
	s.Object["traces"].Array[0].Object["settledAt"] = historyNumber(uint64(vnAt(-1)))
	vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(vnAt(-5))) })
	vnUpdateEvent(s, 1, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(vnAt(-1))) })
	vnGood(t, s)
}

func TestHistoryVNextBuilderStageAliasingAndZeroProof(t *testing.T) {
	zero := validatedHistorySnapshot{}
	if _, e := appendEmptyHistoryEvents(zero, nil); e == nil {
		t.Fatal("zero builder proof")
	}
	if checkForwardHistorySuccessor(zero, zero) == nil {
		t.Fatal("zero successor proof")
	}
	if _, e := zero.snapshotCopy(); e == nil {
		t.Fatal("zero snapshot copy")
	}
	if _, e := (classificationCandidate{}).classificationsCopy(); e == nil {
		t.Fatal("zero candidate")
	}
	f := vnSeed(t, vnDate)
	before := f.snapshot(t)
	prior := vnGood(t, before)
	f.run(t, vnStep{"A", 5})
	after := f.snapshot(t)
	after.Object["classifications"] = historyVNextClone(before.Object["classifications"])
	vnBad(t, after, historyMissing)
	intent := historyCaptureIntent{EventID: vnID(101), TraceID: core.DayTraceID(vnID(3)), Status: core.TraceAbandoned, At: vnAt(5)}
	c, e := appendEmptyHistoryEvents(prior, []historyCaptureIntent{intent})
	must(t, e)
	classes, e := c.classificationsCopy()
	must(t, e)
	after.Object["classifications"] = classes
	next := vnGood(t, after)
	must(t, checkForwardHistorySuccessor(prior, next))
	c2, e := appendEmptyHistoryEvents(prior, []historyCaptureIntent{intent})
	must(t, e)
	if !historyEqual(c.classifications, c2.classifications) {
		t.Fatal("nondeterminism")
	}
	unchanged, e := appendEmptyHistoryEvents(next, nil)
	must(t, e)
	if !historyEqual(unchanged.classifications, classes) {
		t.Fatal("zero intents consumed revision")
	}
	f.run(t, vnStep{"P", 6}, vnStep{"A", 10})
	final := f.snapshot(t)
	intent.EventID = vnID(102)
	intent.At = vnAt(10)
	c, e = appendEmptyHistoryEvents(next, []historyCaptureIntent{intent})
	must(t, e)
	final.Object["classifications"], e = c.classificationsCopy()
	must(t, e)
	last := vnGood(t, final)
	must(t, checkForwardHistorySuccessor(next, last))
	got := final.Object["classifications"]
	if len(got.Object["snapshotsByTraceID"].Array) != 2 || len(got.Object["snapshotEventsByTraceID"].Array) != 2 || len(got.Object["snapshotEventsByTraceID"].Array[1].Array) != 2 {
		t.Fatal("latest was appended, not replaced")
	}
	saved, _ := last.snapshotCopy()
	final.Object["classifications"].Object["revision"] = historyNumber(9)
	view, _ := last.snapshotCopy()
	view.Object["traces"].Array[0].Object["status"] = historyString("pending")
	after.Object["traces"].Array[0].Object["status"] = historyString("pending")
	copyAgain, _ := last.snapshotCopy()
	if !historyEqual(saved, copyAgain) {
		t.Fatal("input/output aliases proof")
	}
	cc, _ := c.classificationsCopy()
	cc.Object["revision"] = historyNumber(99)
	cc2, _ := c.classificationsCopy()
	if historyUint(cc2.Object["revision"]) != 2 {
		t.Fatal("candidate copy aliases")
	}
	failures := []historyCaptureIntent{{EventID: vnID(101), TraceID: core.DayTraceID(vnID(3)), Status: core.TraceAbandoned, At: vnAt(20)}, {EventID: "invalid", TraceID: core.DayTraceID(vnID(3)), Status: core.TraceAbandoned, At: vnAt(20)}, {EventID: vnID(103), TraceID: "bad", Status: core.TraceAbandoned, At: vnAt(20)}, {EventID: vnID(103), TraceID: core.DayTraceID(vnID(3)), Status: core.TracePending, At: vnAt(20)}, {EventID: vnID(103), TraceID: core.DayTraceID(vnID(3)), Status: core.TraceAbandoned, At: core.Instant(math.Float64bits(math.Inf(1)))}}
	for _, in := range failures {
		if _, e := appendEmptyHistoryEvents(last, []historyCaptureIntent{in}); e == nil {
			t.Fatal("invalid intent accepted")
		}
	}
	in := historyCaptureIntent{EventID: vnID(103), TraceID: core.DayTraceID(vnID(3)), Status: core.TraceAbandoned, At: vnAt(20)}
	if _, e := appendEmptyHistoryEvents(last, []historyCaptureIntent{in, in}); e == nil {
		t.Fatal("duplicate intent accepted")
	}
	now, _ := last.snapshotCopy()
	if !historyEqual(saved, now) {
		t.Fatal("failure mutated prior")
	}
	// A new key without a graph is intentionally only a candidate, not proof.
	in.TraceID = core.DayTraceID(vnID(999))
	candidate, e := appendEmptyHistoryEvents(last, []historyCaptureIntent{in})
	must(t, e)
	view, _ = last.snapshotCopy()
	view.Object["classifications"], e = candidate.classificationsCopy()
	must(t, e)
	vnBad(t, view, historyMalformed)
}

func TestHistoryVNextSuccessorNecessaryOnly(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 5}, vnStep{"P", 6}, vnStep{"A", 10})
	base := f.snapshot(t)
	prior := vnGood(t, base)
	variants := map[string]func(canonical.Value){
		"rewrite UUID": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["id"] = historyString(vnID(999)) })
		},
		"rewrite clock": func(s canonical.Value) {
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["capturedAt"] = historyNumber(uint64(vnAt(4))) })
		},
		"delete and renumber": func(s canonical.Value) {
			c := s.Object["classifications"]
			c.Object["snapshotEventsByTraceID"].Array[1].Array = c.Object["snapshotEventsByTraceID"].Array[1].Array[1:]
			vnUpdateEvent(s, 0, func(o map[string]canonical.Value) { o["revision"] = historyNumber(1) })
			c.Object["revision"] = historyNumber(1)
		},
		"day identity": func(s canonical.Value) {
			s.Object["days"].Array[0].Object["id"].Object["rawValue"] = historyString(vnID(999))
		},
	}
	for name, mutate := range variants {
		t.Run(name, func(t *testing.T) {
			s := historyVNextClone(base)
			mutate(s)
			next := vnGood(t, s)
			if checkForwardHistorySuccessor(prior, next) == nil {
				t.Fatal("rewritten prefix accepted")
			}
		})
	}
	f.run(t, vnStep{"L", 20})
	locked := f.snapshot(t)
	a := vnGood(t, locked)
	locked.Object["days"].Array[0].Object["lockedAt"] = historyNumber(uint64(vnAt(19)))
	b := vnGood(t, locked)
	if checkForwardHistorySuccessor(a, b) == nil {
		t.Fatal("changed lock accepted")
	}
	// Prefix success explicitly does NOT authorize graph changes.
	s := historyVNextClone(base)
	s.Object["definitions"].Array[0].Object["title"] = historyString("another synthetic title")
	b = vnGood(t, s)
	must(t, checkForwardHistorySuccessor(prior, b))
	// Deleting all streams can leave an independently valid pending snapshot.
	f.run(t) // no-op fixture call, no mutation
	p := vnSeed(t, vnDate)
	empty := vnGood(t, p.snapshot(t))
	if checkForwardHistorySuccessor(prior, empty) == nil {
		t.Fatal("deleted history accepted")
	}
}

func TestHistoryVNextBoundsAndNoInputRepair(t *testing.T) {
	if _, ok := historyCheckedAdd(math.MaxUint64, 1, math.MaxUint64); ok {
		t.Fatal("unchecked overflow")
	}
	if n, ok := historyCheckedAdd(math.MaxUint64-1, 1, math.MaxUint64); !ok || n != math.MaxUint64 {
		t.Fatal("exact limit refused")
	}
	f := vnSeed(t, vnDate)
	base := f.snapshot(t)
	prior := vnGood(t, base)
	inputs := []canonical.Value{}
	cycle := historyVNextClone(base)
	cycle.Object["loop"] = cycle
	inputs = append(inputs, cycle)
	deep := historyString("x")
	for i := 0; i < 129; i++ {
		deep = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{deep}}
	}
	inputs = append(inputs, deep)
	wide := canonical.Value{Kind: canonical.Array, Array: make([]canonical.Value, canonical.DefaultLimits().MaxNodes)}
	for i := range wide.Array {
		wide.Array[i] = historyNumber(0)
	}
	inputs = append(inputs, wide)
	for _, s := range inputs {
		vnBad(t, s, historyMalformed)
	}
	if _, e := appendEmptyHistoryEvents(prior, make([]historyCaptureIntent, canonical.DefaultLimits().MaxNodes/15+1)); e == nil {
		t.Fatal("oversized request not rejected before traversal")
	}
	// Exact node count matters for append preflight (keys count as nodes).
	f.run(t, vnStep{"A", 5})
	s := f.snapshot(t)
	stream := s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0]
	if n := historyVNextNodes(stream); n != 15 {
		t.Fatalf("event nodes %d", n)
	}
	before, e := canonical.Marshal(base)
	must(t, e)
	_, e = inspectForwardClockOrdinaryHistoryVNext(base)
	must(t, e)
	after, e := canonical.Marshal(base)
	must(t, e)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("input repaired")
	}
}
