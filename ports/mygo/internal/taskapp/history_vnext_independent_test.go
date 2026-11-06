// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

// Independent synthetic review. Source-supported positive graphs are made with
// the baseline kernel, captures are projected here without the candidate builder.
// Passing this reader is not Service reachability, v2 reuse, or write authority.
import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func iaID(n int) string            { return fmt.Sprintf("AA000000-0000-4000-8000-%012X", n) }
func iaNow(s float64) core.Instant { return core.Instant(math.Float64bits(s)) }
func iaPtr[T any](x T) *T          { return &x }
func iaMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func iaNumber(n uint64) canonical.Value {
	return canonical.Value{Kind: canonical.Number, TextNumber: strconv.FormatUint(n, 10)}
}
func iaText(s string) canonical.Value { return canonical.Value{Kind: canonical.String, String: s} }
func iaObj(o map[string]canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Object, Object: o}
}
func iaKey(s string) canonical.Value { return iaObj(map[string]canonical.Value{"rawValue": iaText(s)}) }
func iaCopy(t *testing.T, v canonical.Value) canonical.Value {
	t.Helper()
	b, e := canonical.Marshal(v)
	iaMust(t, e)
	x, e := canonical.Parse(b, canonical.DefaultLimits())
	iaMust(t, e)
	return x
}
func iaBytes(t *testing.T, v canonical.Value) string {
	t.Helper()
	b, e := canonical.Marshal(v)
	iaMust(t, e)
	return string(b)
}

type iaCapture struct {
	status core.TraceStatus
	at     core.Instant
}
type iaStep struct {
	action string
	at     core.Instant
}

func iaSeed(t *testing.T, d core.LocalDate, start core.Instant) *core.TaskKernel {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	iaMust(t, e)
	iaMust(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(iaID(1)), DefinitionID: core.TaskDefinitionID(iaID(2)), Title: "independent synthetic", Now: start}))
	iaMust(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(iaID(1)), TraceID: core.DayTraceID(iaID(3)), DayID: core.DayID(iaID(4)), Date: d, Today: d, Now: start}))
	return k
}
func iaWire(t *testing.T, g core.TaskGraph, caps []iaCapture) canonical.Value {
	t.Helper()
	x, e := emptySupplement()
	iaMust(t, e)
	v, e := v7.FromDomain(recurrence.State{Tasks: g}, x)
	iaMust(t, e)
	c := v.Object["classifications"]
	c.Object["revision"] = iaNumber(uint64(len(caps)))
	if len(caps) > 0 {
		events := make([]canonical.Value, len(caps))
		for i, cap := range caps {
			events[i] = iaObj(map[string]canonical.Value{"id": iaText(iaID(100 + i)), "traceID": iaKey(iaID(3)), "status": iaText(string(cap.status)), "capturedAt": iaNumber(uint64(cap.at)), "revision": iaNumber(uint64(i + 1)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}})
		}
		c.Object["snapshotEventsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{iaKey(iaID(3)), {Kind: canonical.Array, Array: events}}}
		c.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{iaKey(iaID(3)), iaCopy(t, events[len(events)-1])}}
	}
	return v
}
func iaPath(t *testing.T, d core.LocalDate, start core.Instant, steps ...iaStep) (canonical.Value, core.TaskGraph, []iaCapture) {
	t.Helper()
	k := iaSeed(t, d, start)
	var caps []iaCapture
	for i, step := range steps {
		old := k.Snapshot()
		tr := old.Traces[0]
		var e error
		var cap core.TraceStatus
		switch step.action {
		case "A":
			e = k.AbandonChain(core.AbandonChainInput{ChainID: tr.ChainID, TraceID: tr.ID, Now: step.at})
			cap = core.TraceAbandoned
		case "P", "U":
			today := d
			if step.action == "U" {
				today, e = d.AddDays(1)
				iaMust(t, e)
			}
			_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: tr.ChainID, TraceID: tr.ID, Today: today, Now: step.at})
			if e == nil && k.Snapshot().Traces[0].Status == core.TraceUnfinished {
				cap = core.TraceUnfinished
			}
		case "C":
			e = k.MarkCompleted(tr.ID, d, step.at)
		case "undo":
			e = k.UndoCompleted(tr.ID, d, step.at)
		case "L":
			today, x := d.AddDays(1)
			iaMust(t, x)
			e = k.SettleTaskDays(today, step.at)
			if old.Days[0].LockedAt == nil {
				if tr.Status == core.TracePending {
					cap = core.TraceUnfinished
				}
				if tr.Status == core.TraceCompleted {
					cap = core.TraceCompleted
				}
			}
		case "R", "X":
			today := d
			if step.action == "X" {
				today, e = d.AddDays(-1)
				iaMust(t, e)
			}
			e = k.ReturnToPool(core.ReturnToPoolInput{TraceID: tr.ID, ChainID: tr.ChainID, NewDefinitionID: core.TaskDefinitionID(iaID(20 + i)), CancellationID: core.CancellationID(iaID(30 + i)), Today: today, Now: step.at})
			if step.action == "R" {
				cap = core.TraceReturnedToPool
			}
		case "D":
			next, x := d.AddDays(1)
			iaMust(t, x)
			e = k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: tr.ChainID, TraceID: tr.ID, NewTraceID: core.DayTraceID(iaID(40 + i)), DayID: core.DayID(iaID(50 + i)), TargetDate: next, Today: d, Now: step.at})
			cap = core.TraceDeferred
		default:
			t.Fatalf("unknown independent step %s", step.action)
		}
		iaMust(t, e)
		if cap != "" {
			caps = append(caps, iaCapture{cap, step.at})
		}
	}
	g := k.Snapshot()
	return iaWire(t, g, caps), g, caps
}
func iaAccept(t *testing.T, v canonical.Value) {
	t.Helper()
	if _, e := inspectForwardClockOrdinaryHistoryVNext(v); e != nil {
		t.Fatalf("candidate rejects source-supported/profile positive: %v", e)
	}
}
func iaReject(t *testing.T, v canonical.Value) {
	t.Helper()
	if _, e := inspectForwardClockOrdinaryHistoryVNext(v); e == nil {
		t.Fatal("candidate accepts necessary-condition violation")
	}
}
func iaMutateEvent(v canonical.Value, i int, key string, value canonical.Value) {
	c := v.Object["classifications"]
	events := c.Object["snapshotEventsByTraceID"].Array[1].Array
	events[i].Object[key] = value
	if i == len(events)-1 {
		c.Object["snapshotsByTraceID"].Array[1].Object[key] = value
	}
}

func TestIndependentHistorySourcePaths(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tests := []struct {
		name    string
		actions []string
	}{
		{"empty-pending", nil}, {"empty-completed", []string{"C"}}, {"settled-unfinished", []string{"L"}}, {"settled-completed", []string{"C", "L"}},
		{"AA", []string{"A", "P", "A"}}, {"P-latest-A", []string{"A", "P"}}, {"C-latest-A", []string{"A", "P", "C"}},
		{"anchored-locked-C", []string{"A", "P", "C", "L"}}, {"redo-anchor", []string{"A", "P", "C", "undo", "C", "L"}},
		{"lock-before-abandon", []string{"L", "A"}}, {"lock-after-abandon", []string{"A", "L"}},
		{"lock-A-U-A-U", []string{"L", "A", "P", "A", "P"}},
		{"synthetic-open-U", []string{"A", "U"}}, {"synthetic-open-U-later-lock", []string{"A", "U", "L"}},
		{"synthetic-open-U-A-P", []string{"A", "U", "A", "P"}},
		{"first-R", []string{"R"}}, {"A-R", []string{"A", "P", "R"}}, {"first-R-lock", []string{"R", "L"}}, {"A-R-lock", []string{"A", "P", "R", "L"}},
		{"first-D", []string{"D"}}, {"A-D", []string{"A", "P", "D"}}, {"first-D-lock", []string{"D", "L"}}, {"A-D-lock", []string{"A", "P", "D", "L"}},
		{"future-X", []string{"X"}},
	}
	for _, tc := range tests {
		for _, equal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/equal=%v", tc.name, equal), func(t *testing.T) {
				steps := []iaStep{}
				for i, a := range tc.actions {
					n := float64(i + 1)
					if equal {
						n = 0
					}
					steps = append(steps, iaStep{a, iaNow(n)})
				}
				v, _, _ := iaPath(t, d, iaNow(0), steps...)
				before := iaBytes(t, v)
				iaAccept(t, v)
				if iaBytes(t, v) != before {
					t.Fatal("inspector mutated input")
				}
			})
		}
	}
}

func TestIndependentHistoryProfileAndActualCompletion(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, _, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"L", iaNow(5)})
	iaReject(t, v) // Source-supported, profile-unsupported, not a core bug.
	for _, locked := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed/locked=%v", locked), func(t *testing.T) {
			steps := []iaStep{{"A", iaNow(10)}, {"P", iaNow(11)}, {"C", iaNow(15)}}
			if locked {
				steps = append(steps, iaStep{"L", iaNow(20)})
			}
			good, _, _ := iaPath(t, d, iaNow(0), steps...)
			iaAccept(t, good)
			bad := iaCopy(t, good)
			bad.Object["traces"].Array[0].Object["completedAt"] = iaNumber(uint64(iaNow(5))) // Keep content=15, all object-local clocks valid.
			if _, e := inspectEmptyHistory(bad); e != nil {
				t.Fatalf("counterexample is not structurally valid: %v", e)
			}
			iaReject(t, bad)
		})
	}
	x, _, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"P", iaNow(11)}, iaStep{"X", iaNow(20)})
	before := iaBytes(t, x)
	iaReject(t, x)
	if iaBytes(t, x) != before {
		t.Fatal("X history was deleted/repaired")
	}
}

func TestIndependentHistoryDateBounds(t *testing.T) {
	for _, d := range []core.LocalDate{{Year: 1, Month: 1, Day: 1}, {Year: 9999, Month: 12, Day: 31}} {
		for _, a := range [][]string{{"A", "P"}, {"A", "P", "C"}} {
			steps := []iaStep{}
			for i, s := range a {
				steps = append(steps, iaStep{s, iaNow(float64(i + 1))})
			}
			v, _, _ := iaPath(t, d, iaNow(0), steps...)
			iaAccept(t, v)
		}
	}
	min := core.LocalDate{Year: 1, Month: 1, Day: 1}
	v, _, _ := iaPath(t, min, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"U", iaNow(20)}, iaStep{"L", iaNow(30)})
	iaAccept(t, v)
	// Move valid source-supported graph+wire to max date. Core still admits its
	// object-local structure; reader must not invent Today=10000-01-01.
	for _, acts := range [][]iaStep{{{"A", iaNow(10)}, {"U", iaNow(20)}}, {{"L", iaNow(20)}}} {
		s, g, c := iaPath(t, min, iaNow(0), acts...)
		_ = s
		max := core.LocalDate{Year: 9999, Month: 12, Day: 31}
		g.Traces[0].Date = max
		g.Days[0].Date = max
		iaReject(t, iaWire(t, g, c))
	}
}

func TestIndependentHistorySignedZeroSources(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	plus := iaNow(0)
	minus := iaNow(math.Copysign(0, -1))
	for _, a := range []core.Instant{plus, minus} {
		for _, u := range []core.Instant{plus, minus} {
			for _, l := range []core.Instant{plus, minus} {
				for _, order := range []string{"AUL", "ALU", "LAU"} {
					t.Run(fmt.Sprintf("%s/%x/%x/%x", order, a, u, l), func(t *testing.T) {
						steps := []iaStep{}
						for _, x := range order {
							switch x {
							case 'A':
								steps = append(steps, iaStep{"A", a})
							case 'U':
								steps = append(steps, iaStep{"U", u})
							case 'L':
								steps = append(steps, iaStep{"L", l})
							}
						}
						v, _, _ := iaPath(t, d, iaNow(-1), steps...)
						iaAccept(t, v)
						bad := iaCopy(t, v)
						settled := a
						if settled == plus {
							settled = minus
						} else {
							settled = plus
						}
						bad.Object["traces"].Array[0].Object["settledAt"] = iaNumber(uint64(settled))
						if order != "LAU" && u == l && settled == l {
							// Same numeric clock can instead mean A -> reactive P ->
							// Lock(P -> U). A reader cannot infer one actual source.
							iaAccept(t, bad)
						} else {
							iaReject(t, bad)
						}
					})
				}
			}
		}
	}
	for _, status := range []string{"R", "D", "X"} {
		v, _, _ := iaPath(t, d, iaNow(-1), iaStep{status, minus})
		iaAccept(t, v)
		bad := iaCopy(t, v)
		bad.Object["traces"].Array[0].Object["settledAt"] = iaNumber(uint64(plus))
		iaReject(t, bad)
	}
}

func TestIndependentHistoryNonAdjacentAndTerminalRejection(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tests := []struct {
		seq   string
		steps []iaStep
		caps  []iaCapture
	}{
		{"UU", []iaStep{{"L", iaNow(1)}}, []iaCapture{{core.TraceUnfinished, iaNow(1)}, {core.TraceUnfinished, iaNow(1)}}},
		{"UAA", []iaStep{{"L", iaNow(1)}, {"A", iaNow(3)}}, []iaCapture{{core.TraceUnfinished, iaNow(1)}, {core.TraceAbandoned, iaNow(2)}, {core.TraceAbandoned, iaNow(3)}}},
		{"UAC", []iaStep{{"C", iaNow(3)}, {"L", iaNow(4)}}, []iaCapture{{core.TraceUnfinished, iaNow(1)}, {core.TraceAbandoned, iaNow(2)}, {core.TraceCompleted, iaNow(4)}}},
		{"UAR", []iaStep{{"R", iaNow(3)}, {"L", iaNow(4)}}, []iaCapture{{core.TraceUnfinished, iaNow(1)}, {core.TraceAbandoned, iaNow(2)}, {core.TraceReturnedToPool, iaNow(3)}}},
		{"UAD", []iaStep{{"D", iaNow(3)}, {"L", iaNow(4)}}, []iaCapture{{core.TraceUnfinished, iaNow(1)}, {core.TraceAbandoned, iaNow(2)}, {core.TraceDeferred, iaNow(3)}}},
	}
	for _, tc := range tests {
		t.Run(tc.seq, func(t *testing.T) {
			_, g, _ := iaPath(t, d, iaNow(0), tc.steps...)
			v := iaWire(t, g, tc.caps)
			if _, e := inspectEmptyHistory(v); e != nil {
				t.Fatalf("negative fixture structurally invalid: %v", e)
			}
			iaReject(t, v)
		})
	}
	for _, terminal := range []core.TraceStatus{core.TraceCompleted, core.TraceReturnedToPool, core.TraceDeferred} {
		for _, after := range []core.TraceStatus{core.TraceAbandoned, core.TraceUnfinished, core.TraceCompleted, core.TraceReturnedToPool, core.TraceDeferred} {
			t.Run(string(terminal)+"/then/"+string(after), func(t *testing.T) {
				actions := []iaStep{{"A", iaNow(3)}}
				switch after {
				case core.TraceUnfinished:
					actions = []iaStep{{"L", iaNow(3)}}
				case core.TraceCompleted:
					actions = []iaStep{{"C", iaNow(2)}, {"L", iaNow(3)}}
				case core.TraceReturnedToPool:
					actions = []iaStep{{"R", iaNow(3)}}
				case core.TraceDeferred:
					actions = []iaStep{{"D", iaNow(3)}}
				}
				_, g, _ := iaPath(t, d, iaNow(0), actions...)
				iaReject(t, iaWire(t, g, []iaCapture{{terminal, iaNow(1)}, {after, iaNow(3)}}))
			})
		}
	}
}

func TestIndependentHistoryOneFactorWireCorruption(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, _, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"P", iaNow(11)}, iaStep{"A", iaNow(20)})
	iaAccept(t, v)
	mutations := map[string]func(canonical.Value){
		"revision-gap":            func(x canonical.Value) { iaMutateEvent(x, 1, "revision", iaNumber(3)) },
		"revision-duplicate":      func(x canonical.Value) { iaMutateEvent(x, 1, "revision", iaNumber(1)) },
		"count":                   func(x canonical.Value) { x.Object["classifications"].Object["revision"] = iaNumber(1) },
		"UUID-case-collision":     func(x canonical.Value) { iaMutateEvent(x, 1, "id", iaText(strings.ToLower(iaID(100)))) },
		"UUID-invalid":            func(x canonical.Value) { iaMutateEvent(x, 0, "id", iaText("bad")) },
		"capture-before-creation": func(x canonical.Value) { iaMutateEvent(x, 0, "capturedAt", iaNumber(uint64(iaNow(-1)))) },
		"capture-reversed":        func(x canonical.Value) { iaMutateEvent(x, 0, "capturedAt", iaNumber(uint64(iaNow(30)))) },
		"event-nan":               func(x canonical.Value) { iaMutateEvent(x, 0, "capturedAt", iaNumber(uint64(iaNow(math.NaN())))) },
		"event-infinity":          func(x canonical.Value) { iaMutateEvent(x, 0, "capturedAt", iaNumber(uint64(iaNow(math.Inf(1))))) },
		"event-P":                 func(x canonical.Value) { iaMutateEvent(x, 0, "status", iaText("pending")) },
		"event-category":          func(x canonical.Value) { iaMutateEvent(x, 0, "category", canonical.Value{Kind: canonical.Null}) },
		"event-dangling":          func(x canonical.Value) { iaMutateEvent(x, 0, "traceID", iaKey(iaID(900))) },
		"latest-stale": func(x canonical.Value) {
			x.Object["classifications"].Object["snapshotsByTraceID"].Array[1] = iaCopy(t, x.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0])
		},
		"empty-stream": func(x canonical.Value) {
			x.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}
		},
		"preferences-scope": func(x canonical.Value) { x.Object["preferences"].Object["language"] = iaText("english") },
	}
	for name, m := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := iaCopy(t, v)
			m(bad)
			before := iaBytes(t, bad)
			iaReject(t, bad)
			if iaBytes(t, bad) != before {
				t.Fatal("rejection changed input")
			}
		})
	}
}

func TestIndependentHistoryBuilderStagesAliasingAndZeroProof(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	before, g, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)})
	p, e := inspectForwardClockOrdinaryHistoryVNext(before)
	iaMust(t, e)
	original := iaBytes(t, before)
	// Purecore after already has new terminal facts. With the OLD classifications
	// this intermediate is deliberately invalid; only compose-after-append passes.
	k, e := core.NewTaskKernel(g)
	iaMust(t, e)
	_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: g.Traces[0].ChainID, TraceID: g.Traces[0].ID, Today: d, Now: iaNow(11)})
	iaMust(t, e)
	iaMust(t, k.AbandonChain(core.AbandonChainInput{ChainID: g.Traces[0].ChainID, TraceID: g.Traces[0].ID, Now: iaNow(20)}))
	after := iaWire(t, k.Snapshot(), []iaCapture{{core.TraceAbandoned, iaNow(10)}})
	iaReject(t, after)
	intents := []historyCaptureIntent{{EventID: iaID(101), TraceID: g.Traces[0].ID, Status: core.TraceAbandoned, At: iaNow(20)}}
	a, e := appendEmptyHistoryEvents(p, intents)
	iaMust(t, e)
	c, e := a.classificationsCopy()
	iaMust(t, e)
	after.Object["classifications"] = c
	q, e := inspectForwardClockOrdinaryHistoryVNext(after)
	iaMust(t, e)
	iaMust(t, checkForwardHistorySuccessor(p, q))
	if iaBytes(t, before) != original {
		t.Fatal("builder mutated original")
	}
	again, e := appendEmptyHistoryEvents(p, intents)
	iaMust(t, e)
	cc, e := again.classificationsCopy()
	iaMust(t, e)
	if iaBytes(t, c) != iaBytes(t, cc) {
		t.Fatal("builder is nondeterministic")
	}
	zero, e := appendEmptyHistoryEvents(p, nil)
	iaMust(t, e)
	zc, e := zero.classificationsCopy()
	iaMust(t, e)
	if iaBytes(t, zc) != iaBytes(t, before.Object["classifications"]) {
		t.Fatal("zero intents changed classifications")
	}
	// Mutate both original and public copies, then inspect prior's independently
	// owned copy and builder output again. No original/copy/candidate alias.
	before.Object["traces"].Array[0].Object["contentUpdatedAt"] = iaNumber(uint64(iaNow(500)))
	before.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["id"] = iaText("mutated")
	pc, e := p.snapshotCopy()
	iaMust(t, e)
	if iaBytes(t, pc) != original {
		t.Fatal("input escaped into proof")
	}
	pc.Object["traces"].Array[0].Object["status"] = iaText("mutated")
	pc2, e := p.snapshotCopy()
	iaMust(t, e)
	if iaBytes(t, pc2) != original {
		t.Fatal("copy escaped into proof")
	}
	c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["id"] = iaText("mutated")
	ca, e := a.classificationsCopy()
	iaMust(t, e)
	if iaBytes(t, ca) != iaBytes(t, cc) {
		t.Fatal("copy escaped into candidate")
	}
	var invalid validatedHistorySnapshot
	if _, e := invalid.snapshotCopy(); e == nil {
		t.Fatal("zero proof snapshot accepted")
	}
	if _, e := appendEmptyHistoryEvents(invalid, nil); e == nil {
		t.Fatal("zero proof append accepted")
	}
	if e := checkForwardHistorySuccessor(invalid, p); e == nil {
		t.Fatal("zero previous accepted")
	}
	if e := checkForwardHistorySuccessor(p, invalid); e == nil {
		t.Fatal("zero next accepted")
	}
	if _, e := (classificationCandidate{}).classificationsCopy(); e == nil {
		t.Fatal("zero candidate accepted")
	}
}

func TestIndependentHistoryBuilderFailureAndUnboundCandidate(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, _, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)})
	p, e := inspectForwardClockOrdinaryHistoryVNext(v)
	iaMust(t, e)
	base, e := p.snapshotCopy()
	iaMust(t, e)
	original := iaBytes(t, base)
	good := historyCaptureIntent{EventID: iaID(101), TraceID: core.DayTraceID(iaID(3)), Status: core.TraceAbandoned, At: iaNow(20)}
	cases := map[string][]historyCaptureIntent{}
	for _, name := range []string{"old-uuid", "bad-uuid", "bad-trace", "pending", "changed", "nan", "infinity"} {
		x := good
		switch name {
		case "old-uuid":
			x.EventID = strings.ToLower(iaID(100))
		case "bad-uuid":
			x.EventID = "bad"
		case "bad-trace":
			x.TraceID = "bad"
		case "pending":
			x.Status = core.TracePending
		case "changed":
			x.Status = core.TraceChanged
		case "nan":
			x.At = iaNow(math.NaN())
		case "infinity":
			x.At = iaNow(math.Inf(1))
		}
		cases[name] = []historyCaptureIntent{x}
	}
	duplicate := good
	duplicate.EventID = strings.ToLower(good.EventID)
	cases["duplicate-intent"] = []historyCaptureIntent{good, duplicate}
	for name, intents := range cases {
		t.Run(name, func(t *testing.T) {
			if _, e := appendEmptyHistoryEvents(p, intents); e == nil {
				t.Fatal("invalid intent accepted")
			}
			now, e := p.snapshotCopy()
			iaMust(t, e)
			if iaBytes(t, now) != original {
				t.Fatal("failed append mutated proof")
			}
		})
	}
	// Builder may add a syntactically valid, not-yet-existing trace, without
	// claiming it is valid against the graph. This is the deliberate API split.
	ghost := good
	ghost.TraceID = core.DayTraceID(iaID(900))
	a, e := appendEmptyHistoryEvents(p, []historyCaptureIntent{ghost})
	iaMust(t, e)
	c, e := a.classificationsCopy()
	iaMust(t, e)
	bad := iaCopy(t, v)
	bad.Object["classifications"] = c
	iaReject(t, bad)
	if n, ok := historyCheckedAdd(^uint64(0), 1, ^uint64(0)); ok || n != 0 {
		t.Fatal("overflow accepted")
	}
	if _, ok := historyCheckedAdd(83, 1, 84); !ok {
		t.Fatal("exact checked bound rejected")
	}
	if _, ok := historyCheckedAdd(84, 1, 84); ok {
		t.Fatal("checked bound exceeded")
	}
}

func TestIndependentHistorySuccessorRejectsValidEndpointTamper(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, g, caps := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"P", iaNow(11)}, iaStep{"A", iaNow(20)})
	p, e := inspectForwardClockOrdinaryHistoryVNext(v)
	iaMust(t, e)
	endpoints := map[string]canonical.Value{}
	rewritten := iaCopy(t, v)
	iaMutateEvent(rewritten, 0, "id", iaText(iaID(999)))
	endpoints["old-UUID-rewritten"] = rewritten
	retimed := iaCopy(t, v)
	iaMutateEvent(retimed, 0, "capturedAt", iaNumber(uint64(iaNow(9))))
	endpoints["old-capture-rewritten"] = retimed
	endpoints["old-event-deleted-revision-reset"] = iaWire(t, g, caps[1:])
	empty := iaSeed(t, d, iaNow(0)).Snapshot()
	endpoints["old-stream-deleted"] = iaWire(t, empty, nil)
	for name, next := range endpoints {
		t.Run(name, func(t *testing.T) {
			q, e := inspectForwardClockOrdinaryHistoryVNext(next)
			iaMust(t, e)
			a, b := iaBytes(t, v), iaBytes(t, next)
			if checkForwardHistorySuccessor(p, q) == nil {
				t.Fatal("valid endpoint tamper accepted")
			}
			if iaBytes(t, v) != a || iaBytes(t, next) != b {
				t.Fatal("prefix failure mutated endpoints")
			}
		})
	}
	locked, lg, lc := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)}, iaStep{"L", iaNow(20)})
	lp, e := inspectForwardClockOrdinaryHistoryVNext(locked)
	iaMust(t, e)
	for _, kind := range []string{"change-lock", "remove-lock", "change-day-ID", "change-day-date"} {
		t.Run(kind, func(t *testing.T) {
			gg := lg.Clone()
			switch kind {
			case "change-lock":
				gg.Days[0].LockedAt = iaPtr(iaNow(30))
				gg.Days[0].UpdatedAt = iaNow(30)
			case "remove-lock":
				gg.Days[0].LockedAt = nil
			case "change-day-ID":
				gg.Days[0].ID = core.DayID(iaID(901))
			case "change-day-date":
				gg.Days[0].Date = core.LocalDate{Year: 2026, Month: 11, Day: 2}
				gg.Traces[0].Date = gg.Days[0].Date
			}
			q, e := inspectForwardClockOrdinaryHistoryVNext(iaWire(t, gg, lc))
			iaMust(t, e)
			if checkForwardHistorySuccessor(lp, q) == nil {
				t.Fatal("day invariant tamper accepted")
			}
		})
	}
	// This check intentionally does not freeze arbitrary graph facts or grant
	// replay/write authority: a changed title is outside its narrow guarantee.
	changed := iaCopy(t, locked)
	changed.Object["definitions"].Array[0].Object["title"] = iaText("different synthetic title")
	q, e := inspectForwardClockOrdinaryHistoryVNext(changed)
	iaMust(t, e)
	iaMust(t, checkForwardHistorySuccessor(lp, q))
}

func TestIndependentHistoryBoundedASTAndStateSlots(t *testing.T) {
	var cyclic canonical.Value
	cyclic = iaObj(map[string]canonical.Value{})
	cyclic.Object["cycle"] = cyclic
	iaReject(t, cyclic)
	deep := canonical.Value{Kind: canonical.Null}
	for i := 0; i < canonical.DefaultLimits().MaxDepth+2; i++ {
		deep = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{deep}}
	}
	iaReject(t, deep)
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, g, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(0)})
	tooLarge := iaCopy(t, v)
	tooLarge.Object["definitions"].Array[0].Object["title"] = iaText(strings.Repeat("a", canonical.DefaultLimits().MaxBytes+1))
	iaReject(t, tooLarge)
	// Thousands of legal equal-time A/P cycles exercise closure and append at
	// scale. Fixed arrays, not a wall-time cutoff, bound the NFA state space.
	caps := make([]iaCapture, 4096)
	for i := range caps {
		caps[i] = iaCapture{core.TraceAbandoned, iaNow(0)}
	}
	large := iaWire(t, g, caps)
	p, e := inspectForwardClockOrdinaryHistoryVNext(large)
	iaMust(t, e)
	if historyMachineSlots != 84 || p.proof.stats.MaxSlots > 84 || p.proof.stats.MaxClosureRounds > 84 {
		t.Fatalf("unbounded NFA: slots=%d stats=%+v", historyMachineSlots, p.proof.stats)
	}
	t.Logf("4096-event source-supported loop: max slots=%d, closure rounds=%d", p.proof.stats.MaxSlots, p.proof.stats.MaxClosureRounds)
}

func TestIndependentHistoryGlobalInterleavingAndNarrowReader(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tomorrow, _ := d.AddDays(1)
	k, e := core.NewTaskKernel(core.TaskGraph{})
	iaMust(t, e)
	for i := 0; i < 3; i++ {
		n := 200 + i*10
		chain, def, tr := core.TaskChainID(iaID(n)), core.TaskDefinitionID(iaID(n+1)), core.DayTraceID(iaID(n+2))
		iaMust(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: def, Title: "multi synthetic", Now: iaNow(0)}))
		on, day := d, core.DayID(iaID(300))
		if i == 2 {
			on, day = tomorrow, core.DayID(iaID(301))
		}
		iaMust(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: tr, DayID: day, Date: on, Today: d, Now: iaNow(0)}))
	}
	iaMust(t, k.SettleTaskDays(tomorrow, iaNow(10)))
	iaMust(t, k.AbandonChain(core.AbandonChainInput{ChainID: core.TaskChainID(iaID(220)), TraceID: core.DayTraceID(iaID(222)), Now: iaNow(10)}))
	extra, e := emptySupplement()
	iaMust(t, e)
	v, e := v7.FromDomain(recurrence.State{Tasks: k.Snapshot()}, extra)
	iaMust(t, e)
	c := v.Object["classifications"]
	latest, streams := []canonical.Value{}, []canonical.Value{}
	for revision, n := range []int{202, 222, 212} {
		status := core.TraceUnfinished
		if n == 222 {
			status = core.TraceAbandoned
		}
		event := iaObj(map[string]canonical.Value{"id": iaText(iaID(500 + revision)), "traceID": iaKey(iaID(n)), "status": iaText(string(status)), "capturedAt": iaNumber(uint64(iaNow(10))), "revision": iaNumber(uint64(revision + 1)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}})
		latest = append(latest, iaKey(iaID(n)), iaCopy(t, event))
		streams = append(streams, iaKey(iaID(n)), canonical.Value{Kind: canonical.Array, Array: []canonical.Value{event}})
	}
	c.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: latest}
	c.Object["snapshotEventsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: streams}
	c.Object["revision"] = iaNumber(3)
	// Ux,Az,Uy can pass necessary conditions; this interleaving is deliberately
	// NOT a Service batch/reachability proof. No reader or writer is connected.
	iaAccept(t, v)
	for _, kind := range []string{"uuid", "revision", "clock", "missing-key", "wrong-key"} {
		t.Run(kind, func(t *testing.T) {
			bad := iaCopy(t, v)
			bc := bad.Object["classifications"]
			s := bc.Object["snapshotEventsByTraceID"].Array
			l := bc.Object["snapshotsByTraceID"].Array
			switch kind {
			case "uuid":
				s[5].Array[0].Object["id"] = iaText(strings.ToLower(iaID(500)))
				l[5].Object["id"] = iaText(strings.ToLower(iaID(500)))
			case "revision":
				s[5].Array[0].Object["revision"] = iaNumber(1)
				l[5].Object["revision"] = iaNumber(1)
			case "clock":
				s[3].Array[0].Object["capturedAt"] = iaNumber(uint64(iaNow(9)))
				l[3].Object["capturedAt"] = iaNumber(uint64(iaNow(9)))
				bad.Object["traces"].Array[2].Object["contentUpdatedAt"] = iaNumber(uint64(iaNow(9)))
				bad.Object["traces"].Array[2].Object["settledAt"] = iaNumber(uint64(iaNow(9)))
			case "missing-key":
				bc.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: l[:4]}
			case "wrong-key":
				s[2] = iaKey(iaID(999))
			}
			iaReject(t, bad)
		})
	}
	for _, path := range [][]iaStep{nil, {{"C", iaNow(1)}}, {{"L", iaNow(2)}}, {{"C", iaNow(1)}, {"L", iaNow(2)}}, {{"R", iaNow(2)}}, {{"D", iaNow(2)}}, {{"X", iaNow(2)}}} {
		single, _, _ := iaPath(t, d, iaNow(0), path...)
		iaMust(t, ValidateEmptyHistory(single))
		iaAccept(t, single)
	}
	for _, path := range [][]iaStep{{{"A", iaNow(1)}}, {{"A", iaNow(1)}, {"P", iaNow(2)}}, {{"A", iaNow(1)}, {"U", iaNow(2)}}} {
		next, _, _ := iaPath(t, d, iaNow(0), path...)
		iaAccept(t, next)
		if ValidateEmptyHistory(next) == nil {
			t.Fatal("old narrow reader widened")
		}
	}
}

func TestIndependentHistoryNumericFiniteFrontiers(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	for _, tc := range []struct {
		name           string
		start, a, u, l float64
	}{
		{"negative", -100, -10, -5, -1}, {"fractional", 0, 0.125, 0.25, 0.5}, {"subnormal", 0, math.SmallestNonzeroFloat64, 2 * math.SmallestNonzeroFloat64, 3 * math.SmallestNonzeroFloat64}, {"max", -math.MaxFloat64, math.MaxFloat64, math.MaxFloat64, math.MaxFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, _, _ := iaPath(t, d, iaNow(tc.start), iaStep{"A", iaNow(tc.a)}, iaStep{"U", iaNow(tc.u)}, iaStep{"L", iaNow(tc.l)})
			iaAccept(t, v)
		})
	}
}

func TestIndependentHistoryExactByteAndNodeBounds(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, _, _ := iaPath(t, d, iaNow(0))
	envelope := iaObj(map[string]canonical.Value{"formatVersion": iaNumber(7), "snapshot": v})
	initial, e := canonical.Marshal(envelope)
	iaMust(t, e)
	oldTitle := v.Object["definitions"].Array[0].Object["title"].String
	size := canonical.DefaultLimits().MaxBytes - len(initial) + len(oldTitle)
	v.Object["definitions"].Array[0].Object["title"] = iaText(strings.Repeat("a", size))
	exact, e := canonical.Marshal(envelope)
	iaMust(t, e)
	if len(exact) != canonical.DefaultLimits().MaxBytes {
		t.Fatalf("wrong byte boundary: %d", len(exact))
	}
	exact = nil
	iaAccept(t, v)
	v.Object["definitions"].Array[0].Object["title"] = iaText(strings.Repeat("a", size+1))
	iaReject(t, v)
	// Share immutable leaves in the caller AST to exercise a million logical
	// nodes without allocating a million independent Go objects. The writer must
	// count occurrences, not unique pointers. Schema validity is separate.
	leaf := canonical.Value{Kind: canonical.Array, Array: make([]canonical.Value, 999)}
	for i := range leaf.Array {
		leaf.Array[i] = canonical.Value{Kind: canonical.Null}
	}
	nodes := canonical.Value{Kind: canonical.Array, Array: make([]canonical.Value, 1000)}
	for i := range nodes.Array {
		nodes.Array[i] = leaf
	}
	// Root 1 + 1000*(array 1 + 999 nulls) = 1,000,001.
	if _, e := canonical.Marshal(nodes); e == nil || !strings.Contains(e.Error(), "node limit") {
		t.Fatalf("wanted canonical node limit, got %v", e)
	}
	iaReject(t, nodes)
	short := canonical.Value{Kind: canonical.Array, Array: append([]canonical.Value{}, leaf.Array[:998]...)}
	nodes.Array[0] = short
	if _, e := canonical.Marshal(nodes); e != nil {
		t.Fatalf("exact 1,000,000 logical nodes rejected: %v", e)
	}
	t.Logf("accepted exact %d-byte full envelope; rejected +1 byte and %d logical nodes", canonical.DefaultLimits().MaxBytes, canonical.DefaultLimits().MaxNodes+1)
}

func TestIndependentHistoryMaximumCountScanAndBuilderCapacity(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tr := core.DayTrace{Date: d, Status: core.TraceAbandoned, CreatedAt: iaNow(0), ContentUpdatedAt: iaNow(0), SettledAt: iaPtr(iaNow(0))}
	// Complete ASTs hit their byte/node limits sooner. This exercises the machine
	// at the conservative maximum 1e6/15 event bound independently of encoding.
	n := canonical.DefaultLimits().MaxNodes / 15
	events := make([]historyVNextEvent, n)
	for i := range events {
		events[i] = historyVNextEvent{status: core.TraceAbandoned, at: iaNow(0)}
	}
	stats, e := historyVNextScan(tr, core.Day{Date: d}, events)
	iaMust(t, e)
	if stats.MaxSlots > 84 || stats.MaxClosureRounds > 84 {
		t.Fatal("fixed state bound exceeded")
	}
	v, _, _ := iaPath(t, d, iaNow(0))
	p, e := inspectForwardClockOrdinaryHistoryVNext(v)
	iaMust(t, e)
	over := make([]historyCaptureIntent, n+1)
	if _, e := appendEmptyHistoryEvents(p, over); e == nil {
		t.Fatal("oversize intent preflight accepted")
	}
	t.Logf("scanned %d events with max slots=%d closure rounds=%d; over-count builder rejected", n, stats.MaxSlots, stats.MaxClosureRounds)
}

func TestIndependentHistoryConcurrentImmutableReaders(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	v, _, _ := iaPath(t, d, iaNow(0), iaStep{"A", iaNow(10)})
	p, e := inspectForwardClockOrdinaryHistoryVNext(v)
	iaMust(t, e)
	intents := []historyCaptureIntent{{EventID: iaID(101), TraceID: core.DayTraceID(iaID(3)), Status: core.TraceAbandoned, At: iaNow(20)}}
	errors := make(chan error, 16)
	for worker := 0; worker < 16; worker++ {
		go func() {
			for i := 0; i < 20; i++ {
				a, e := appendEmptyHistoryEvents(p, intents)
				if e != nil {
					errors <- e
					return
				}
				c, e := a.classificationsCopy()
				if e != nil {
					errors <- e
					return
				}
				c.Object["revision"] = iaNumber(99)
				s, e := p.snapshotCopy()
				if e != nil {
					errors <- e
					return
				}
				s.Object["traces"].Array[0].Object["status"] = iaText("mutated copy")
				if e := checkForwardHistorySuccessor(p, p); e != nil {
					errors <- e
					return
				}
			}
			errors <- nil
		}()
	}
	for i := 0; i < 16; i++ {
		iaMust(t, <-errors)
	}
}

func TestIndependentHistoryFutureCancellationAndLaterDayLock(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tomorrow, _ := d.AddDays(1)
	_, g, _ := iaPath(t, d, iaNow(0), iaStep{"X", iaNow(10)})
	k, e := core.NewTaskKernel(g)
	iaMust(t, e)
	iaMust(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(iaID(600)), DefinitionID: core.TaskDefinitionID(iaID(601)), Title: "lock companion synthetic", Now: iaNow(11)}))
	iaMust(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(iaID(600)), TraceID: core.DayTraceID(iaID(602)), DayID: g.Days[0].ID, Date: d, Today: d, Now: iaNow(11)}))
	iaMust(t, k.SettleTaskDays(tomorrow, iaNow(20)))
	v := iaWire(t, k.Snapshot(), nil)
	c := v.Object["classifications"]
	event := iaObj(map[string]canonical.Value{"id": iaText(iaID(603)), "traceID": iaKey(iaID(602)), "status": iaText(string(core.TraceUnfinished)), "capturedAt": iaNumber(uint64(iaNow(20))), "revision": iaNumber(1), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}})
	c.Object["revision"] = iaNumber(1)
	c.Object["snapshotEventsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{iaKey(iaID(602)), {Kind: canonical.Array, Array: []canonical.Value{event}}}}
	c.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{iaKey(iaID(602)), iaCopy(t, event)}}
	iaAccept(t, v)
	iaMust(t, ValidateEmptyHistory(v))
	// Min-date X has no possible earlier valid Today even though core's broad
	// structural rule allows draftCancelledOn == trace.Date.
	min := core.LocalDate{Year: 1, Month: 1, Day: 1}
	g.Traces[0].Date = min
	g.Days[0].Date = min
	g.Traces[0].DraftCancelledOn = iaPtr(min)
	iaReject(t, iaWire(t, g, nil))
}

func TestIndependentHistoryCompletedContentNeedNotEqualCompletion(t *testing.T) {
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	for _, locked := range []bool{false, true} {
		steps := []iaStep{{"A", iaNow(10)}, {"P", iaNow(11)}, {"C", iaNow(15)}}
		if locked {
			steps = append(steps, iaStep{"L", iaNow(20)})
		}
		_, g, c := iaPath(t, d, iaNow(0), steps...)
		g.Traces[0].ContentUpdatedAt = iaNow(17)
		iaAccept(t, iaWire(t, g, c))
	}
}

func TestIndependentHistoryBuilderNewStreamBatchAndSlotEdges(t *testing.T) {
	seen := map[int]bool{}
	var full historyMachineSet
	for status := 0; status < 7; status++ {
		for lock := 0; lock < 2; lock++ {
			for token := 0; token < 3; token++ {
				for anchor := 0; anchor < 2; anchor++ {
					key := historyMachineKey{status: status, lock: lock, token: token, anchor: anchor}
					slot := key.slot()
					if slot < 0 || slot >= 84 || seen[slot] || historyMachineDecode(slot) != key {
						t.Fatalf("invalid fixed slot %d", slot)
					}
					seen[slot] = true
					full.put(key, 0)
				}
			}
		}
	}
	stats := historyMachineStats{}
	stats.count(&full)
	if len(seen) != 84 || !seen[83] || stats.MaxSlots != 84 {
		t.Fatal("slot upper boundary not represented")
	}
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	k := iaSeed(t, d, iaNow(0))
	iaMust(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(iaID(610)), DefinitionID: core.TaskDefinitionID(iaID(611)), Title: "batch companion synthetic", Now: iaNow(0)}))
	iaMust(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(iaID(610)), TraceID: core.DayTraceID(iaID(612)), DayID: core.DayID(iaID(4)), Date: d, Today: d, Now: iaNow(0)}))
	iaMust(t, k.MarkCompleted(core.DayTraceID(iaID(3)), d, iaNow(1)))
	before := iaWire(t, k.Snapshot(), nil)
	p, e := inspectForwardClockOrdinaryHistoryVNext(before)
	iaMust(t, e)
	next, _ := d.AddDays(1)
	iaMust(t, k.SettleTaskDays(next, iaNow(10)))
	intents := []historyCaptureIntent{{EventID: iaID(700), TraceID: core.DayTraceID(iaID(3)), Status: core.TraceCompleted, At: iaNow(10)}, {EventID: iaID(701), TraceID: core.DayTraceID(iaID(612)), Status: core.TraceUnfinished, At: iaNow(10)}}
	candidate, e := appendEmptyHistoryEvents(p, intents)
	iaMust(t, e)
	classification, e := candidate.classificationsCopy()
	iaMust(t, e)
	if len(classification.Object["snapshotsByTraceID"].Array) != 4 || len(classification.Object["snapshotEventsByTraceID"].Array) != 4 {
		t.Fatal("new stream batch not mapped once per trace")
	}
	after := iaWire(t, k.Snapshot(), nil)
	after.Object["classifications"] = classification
	q, e := inspectForwardClockOrdinaryHistoryVNext(after)
	iaMust(t, e)
	iaMust(t, checkForwardHistorySuccessor(p, q))
	iaMust(t, k.AbandonChain(core.AbandonChainInput{ChainID: core.TaskChainID(iaID(610)), TraceID: core.DayTraceID(iaID(612)), Now: iaNow(20)}))
	candidate, e = appendEmptyHistoryEvents(q, []historyCaptureIntent{{EventID: iaID(702), TraceID: core.DayTraceID(iaID(612)), Status: core.TraceAbandoned, At: iaNow(20)}})
	iaMust(t, e)
	classification, e = candidate.classificationsCopy()
	iaMust(t, e)
	if len(classification.Object["snapshotsByTraceID"].Array) != 4 || len(classification.Object["snapshotEventsByTraceID"].Array) != 4 {
		t.Fatal("existing stream created duplicate map key")
	}
	after = iaWire(t, k.Snapshot(), nil)
	after.Object["classifications"] = classification
	r, e := inspectForwardClockOrdinaryHistoryVNext(after)
	iaMust(t, e)
	iaMust(t, checkForwardHistorySuccessor(q, r))
}
