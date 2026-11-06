// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

func liID(n int) string { return fmt.Sprintf("ABCDEFAA-1234-4567-8ABC-%012X", n) }
func liAt(n float64) core.Instant {
	a, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return a
}
func liPtr[T any](v T) *T { return &v }
func liOK(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func liS(s string) canonical.Value { return canonical.Value{Kind: canonical.String, String: s} }
func liN(n uint64) canonical.Value {
	return canonical.Value{Kind: canonical.Number, TextNumber: fmt.Sprint(n)}
}
func liObj(m map[string]canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Object, Object: m}
}
func liArr(v ...canonical.Value) canonical.Value {
	if v == nil {
		v = []canonical.Value{}
	}
	return canonical.Value{Kind: canonical.Array, Array: v}
}
func liKey(id string) canonical.Value { return liObj(map[string]canonical.Value{"rawValue": liS(id)}) }

// Independent explicit event projection: no builder, capture selector or lifecycle helper.
type liEvent struct {
	trace, id string
	status    core.TraceStatus
	at        core.Instant
}

func liSnapshot(t *testing.T, g core.TaskGraph, events []liEvent) canonical.Value {
	t.Helper()
	x, e := emptySupplement()
	liOK(t, e)
	streams, latest := []canonical.Value{}, []canonical.Value{}
	slots := map[string]int{}
	for i, v := range events {
		event := liObj(map[string]canonical.Value{"id": liS(v.id), "traceID": liKey(v.trace), "status": liS(string(v.status)), "labels": liArr(), "capturedAt": liN(uint64(v.at)), "revision": liN(uint64(i + 1))})
		slot, ok := slots[v.trace]
		if !ok {
			slot = len(streams)
			slots[v.trace] = slot
			streams = append(streams, liKey(v.trace), liArr())
			latest = append(latest, liKey(v.trace), event)
		}
		streams[slot+1].Array = append(streams[slot+1].Array, event)
		latest[slot+1] = event
	}
	x.Classifications.Object["snapshotEventsByTraceID"] = liArr(streams...)
	x.Classifications.Object["snapshotsByTraceID"] = liArr(latest...)
	x.Classifications.Object["revision"] = liN(uint64(len(events)))
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, x)
	liOK(t, e)
	return s
}
func liPrior(t *testing.T, g core.TaskGraph, ev []liEvent) validatedHistorySnapshot {
	t.Helper()
	p, e := inspectForwardClockOrdinaryHistoryVNext(liSnapshot(t, g, ev))
	liOK(t, e)
	return p
}

var liDate = core.LocalDate{Year: 2026, Month: 11, Day: 6}

func liDay(t *testing.T, n int) core.LocalDate { d, e := liDate.AddDays(n); liOK(t, e); return d }
func liSeed(t *testing.T, date core.LocalDate) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	liOK(t, e)
	liOK(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(liID(1)), DefinitionID: core.TaskDefinitionID(liID(2)), Title: "independent fixture", Now: liAt(0)}))
	liOK(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(liID(1)), TraceID: core.DayTraceID(liID(3)), DayID: core.DayID(liID(4)), Date: date, Today: date, Now: liAt(1)}))
	return k.Snapshot()
}
func liAbandon(g core.TaskGraph, i int, at core.Instant) core.TaskGraph {
	g = g.Clone()
	g.Traces[i].Status = core.TraceAbandoned
	g.Traces[i].ContentUpdatedAt = at
	g.Traces[i].SettledAt = liPtr(at)
	g.Traces[i].PinOrder = nil
	for j := range g.Chains {
		if g.Chains[j].ID == g.Traces[i].ChainID {
			g.Chains[j].State = core.ChainAbandoned
			g.Chains[j].UpdatedAt = at
		}
	}
	return g
}
func liReactivate(g core.TaskGraph, i int, status core.TraceStatus, at core.Instant) core.TaskGraph {
	g = g.Clone()
	g.Traces[i].Status = status
	g.Traces[i].ContentUpdatedAt = at
	g.Traces[i].PinOrder = nil
	if status == core.TracePending {
		g.Traces[i].SettledAt = nil
	}
	for j := range g.Chains {
		if g.Chains[j].ID == g.Traces[i].ChainID {
			g.Chains[j].State = core.ChainActive
			g.Chains[j].UpdatedAt = at
		}
	}
	return g
}
func liZero(t *testing.T, r auditedHistoryCommand, e error) {
	t.Helper()
	if e == nil || r.proof != nil || r.valid() {
		t.Fatalf("failure must be zero: %v %#v", e, r)
	}
	if _, x := r.changed(); x == nil {
		t.Fatal("zero changed")
	}
	if _, x := r.snapshotCopy(); x == nil {
		t.Fatal("zero snapshot")
	}
}
func liCheck(t *testing.T, r auditedHistoryCommand, e error, want canonical.Value) {
	t.Helper()
	liOK(t, e)
	got, e := r.snapshotCopy()
	liOK(t, e)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("whole canonical projection mismatch\ngot %#v\nwant %#v", got, want)
	}
	c, e := r.changed()
	liOK(t, e)
	if !c {
		t.Fatal("missing changed")
	}
	a, e := r.snapshotCopy()
	liOK(t, e)
	a.Object["traces"].Array[0].Object["status"] = liS("poison")
	b, e := r.snapshotCopy()
	liOK(t, e)
	if !reflect.DeepEqual(b, want) {
		t.Fatal("copy alias")
	}
}
func TestLifecycleIndependentSourceMatrix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		delta  int
		locked bool
		today  int
		status core.TraceStatus
	}{{"today", 0, false, 0, core.TracePending}, {"future", 1, false, 0, core.TracePending}, {"past", -1, false, 0, core.TraceUnfinished}, {"lockedPast", -1, true, 0, core.TraceUnfinished}, {"lockedCurrent", 0, true, 0, core.TraceUnfinished}, {"lockedFuture", 1, true, 0, core.TraceUnfinished}} {
		t.Run(tc.name, func(t *testing.T) {
			g := liSeed(t, liDay(t, tc.delta))
			a := liAbandon(g, 0, liAt(10))
			ev := []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}}
			// Initial abandon uses manually declared full post-state and event.
			p := liPrior(t, g, nil)
			in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(10), EventID: strings.ToLower(liID(100))}
			r, e := auditAbandonHistoryVNext(p, g, a, in)
			liCheck(t, r, e, liSnapshot(t, a, ev))
			if r.proof.eventID != in.EventID || r.proof.trace != in.TraceID || r.proof.effectiveTrace != in.TraceID || r.proof.today != (core.LocalDate{}) || r.proof.now != in.Now || r.proof.kind != historyCommandAbandon {
				t.Fatal("abandon identity")
			}
			if tc.locked {
				a.Days[0].LockedAt = liPtr(liAt(11))
				a.Days[0].UpdatedAt = liAt(11)
			}
			p = liPrior(t, a, ev)
			after := liReactivate(a, 0, tc.status, liAt(20))
			ids := []string{}
			wantEvents := append([]liEvent{}, ev...)
			if tc.status == core.TraceUnfinished {
				ids = []string{liID(101)}
				wantEvents = append(wantEvents, liEvent{liID(3), liID(101), core.TraceUnfinished, liAt(20)})
			}
			ri := reactivateHistoryInput{ChainID: a.Chains[0].ID, TraceID: a.Traces[0].ID, Today: liDay(t, tc.today), Now: liAt(20), EventIDs: ids}
			r, e = auditReactivateHistoryVNext(p, a, after, ri)
			want := liSnapshot(t, after, wantEvents)
			liCheck(t, r, e, want)
			if r.proof.kind != historyCommandReactivate || r.proof.trace != ri.TraceID || r.proof.effectiveTrace != ri.TraceID || r.proof.today != ri.Today || r.proof.now != ri.Now {
				t.Fatal("reactivate identity")
			}
			if len(ids) > 0 {
				ids[0] = liID(199)
				if r.proof.eventID != liID(101) {
					t.Fatal("retained caller slice")
				}
			} else if r.proof.eventID != "" {
				t.Fatal("P allocated event")
			}
			// Every legitimate endpoint must reject an unrelated, independently valid delta.
			bad := after.Clone()
			bad.Definitions[0].Title = "valid malicious delta"
			liOK(t, core.ValidateTaskGraph(bad))
			rr, ee := auditReactivateHistoryVNext(p, a, bad, ri)
			liZero(t, rr, ee)
			after.Traces[0].DescriptionText = liPtr("caller mutation")
			a.Definitions[0].Title = "caller mutation"
			copy, e := r.snapshotCopy()
			liOK(t, e)
			if !reflect.DeepEqual(copy, want) {
				t.Fatal("input alias")
			}
		})
	}
}

// Structural fixed-profile parameter: independent roots, NOT claimed Service-reachable.
func liTwo(t *testing.T, status core.TraceStatus) (core.TaskGraph, []liEvent) {
	g := liSeed(t, liDay(t, -1))
	g = liAbandon(g, 0, liAt(10))
	d := g.Days[0]
	d.ID = core.DayID(liID(5))
	d.Date = liDate
	g.Days = append(g.Days, d)
	v := g.Traces[0]
	v.ID = core.DayTraceID(liID(6))
	v.Date = liDate
	v.ContentUpdatedAt = liAt(20)
	v.SettledAt = liPtr(liAt(20))
	g.Traces = append(g.Traces, v)
	g.Chains[0].UpdatedAt = liAt(20)
	ev := []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}, {liID(6), liID(101), core.TraceAbandoned, liAt(20)}}
	if status == core.TraceUnfinished {
		for i := range g.Traces {
			g.Traces[i].Status = status
			g.Traces[i].ContentUpdatedAt = liAt(25)
		}
		g.Chains[0].State = core.ChainActive
		g.Chains[0].UpdatedAt = liAt(25)
		ev = append(ev, liEvent{liID(3), liID(102), status, liAt(25)}, liEvent{liID(6), liID(103), status, liAt(25)})
	}
	return g, ev
}
func TestLifecycleIndependentRequestedIdentity(t *testing.T) {
	g, ev := liTwo(t, core.TraceAbandoned)
	p := liPrior(t, g, ev)
	for i := range g.Traces {
		after := liReactivate(g, i, core.TraceUnfinished, liAt(30))
		in := reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[i].ID, Today: liDay(t, 1), Now: liAt(30), EventIDs: []string{liID(110)}}
		r, e := auditReactivateHistoryVNext(p, g, after, in)
		wantEvents := append(append([]liEvent{}, ev...), liEvent{string(in.TraceID), liID(110), core.TraceUnfinished, in.Now})
		liCheck(t, r, e, liSnapshot(t, after, wantEvents))
		if r.proof.effectiveTrace != in.TraceID {
			t.Fatal("latest-A selector")
		}
	}
	g, ev = liTwo(t, core.TraceUnfinished)
	p = liPrior(t, g, ev)
	for i := range g.Traces {
		after := liAbandon(g, i, liAt(30))
		in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[i].ID, Now: liAt(30), EventID: liID(110)}
		r, e := auditAbandonHistoryVNext(p, g, after, in)
		liCheck(t, r, e, liSnapshot(t, after, append(append([]liEvent{}, ev...), liEvent{string(in.TraceID), liID(110), core.TraceAbandoned, in.Now})))
		if r.proof.effectiveTrace != in.TraceID {
			t.Fatal("unfinished chosen as active")
		}
	}
	// Add a true continued pending trace; historical requests redirect to pending.
	base := liSeed(t, liDay(t, -1))
	k, e := core.NewTaskKernel(base)
	liOK(t, e)
	liOK(t, k.SettleTaskDays(liDate, liAt(10)))
	liOK(t, k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: base.Chains[0].ID, TraceID: base.Traces[0].ID, NewTraceID: core.DayTraceID(liID(6)), DayID: core.DayID(liID(5)), TargetDate: liDate, Today: liDate, Now: liAt(20)}))
	g = k.Snapshot()
	ev = []liEvent{{liID(3), liID(100), core.TraceUnfinished, liAt(10)}}
	p = liPrior(t, g, ev)
	after := liAbandon(g, 1, liAt(30))
	for _, requested := range []core.DayTraceID{g.Traces[0].ID, g.Traces[1].ID} {
		in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: requested, Now: liAt(30), EventID: liID(101)}
		r, e := auditAbandonHistoryVNext(p, g, after, in)
		liCheck(t, r, e, liSnapshot(t, after, append(append([]liEvent{}, ev...), liEvent{liID(6), liID(101), core.TraceAbandoned, liAt(30)})))
		if r.proof.trace != requested || r.proof.effectiveTrace != g.Traces[1].ID {
			t.Fatal("request identity collapsed")
		}
	}
	// Once the descendant is U, abandoning its true predecessor would violate continuation.
	liOK(t, k.SettleTaskDays(liDay(t, 1), liAt(25)))
	g = k.Snapshot()
	ev = append(ev, liEvent{liID(6), liID(101), core.TraceUnfinished, liAt(25)})
	p = liPrior(t, g, ev)
	bad := liAbandon(g, 0, liAt(30))
	if core.ValidateTaskGraph(bad) == nil {
		t.Fatal("invalid continuation fixture unexpectedly valid")
	}
	in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(30), EventID: liID(102)}
	r, e := auditAbandonHistoryVNext(p, g, bad, in)
	liZero(t, r, e)
}
func TestLifecycleIndependentFailures(t *testing.T) {
	g := liSeed(t, liDate)
	a := liAbandon(g, 0, liAt(10))
	p := liPrior(t, g, nil)
	ap := liPrior(t, a, []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}})
	afterP := liReactivate(a, 0, core.TracePending, liAt(20))
	afterU := liReactivate(a, 0, core.TraceUnfinished, liAt(20))
	ai := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(10), EventID: liID(100)}
	ri := reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(20), Today: liDate}
	for _, mode := range []string{"zero", "owner", "missing", "short chain", "short trace", "long event", "bad uuid", "nan", "inf", "rollback", "before binding", "after delta", "core inactive"} {
		t.Run(mode, func(t *testing.T) {
			bp, bf, af, in := p, g.Clone(), a.Clone(), ai
			switch mode {
			case "zero":
				bp = validatedHistorySnapshot{}
			case "owner":
				in.ChainID = core.TaskChainID(liID(999))
			case "missing":
				in.TraceID = core.DayTraceID(liID(999))
			case "short chain":
				in.ChainID = "x"
			case "short trace":
				in.TraceID = "x"
			case "long event":
				in.EventID = strings.Repeat("x", 37)
			case "bad uuid":
				in.EventID = strings.Repeat("x", 36)
			case "nan":
				in.Now = core.Instant(math.Float64bits(math.NaN()))
			case "inf":
				in.Now = core.Instant(math.Float64bits(math.Inf(1)))
			case "rollback":
				in.Now = liAt(0)
			case "before binding":
				bf.Definitions[0].Title = "different"
			case "after delta":
				af.Traces[0].Priority++
			case "core inactive":
				bp = ap
				bf = a.Clone()
			}
			oldB, oldA, oldP := bf.Clone(), af.Clone(), bp.proof
			r, e := auditAbandonHistoryVNext(bp, bf, af, in)
			liZero(t, r, e)
			if !reflect.DeepEqual(bf, oldB) || !reflect.DeepEqual(af, oldA) || bp.proof != oldP {
				t.Fatal("failure changed inputs")
			}
		})
	}
	for _, mode := range []string{"P extra ID", "U missing ID", "U duplicate", "U bad UUID", "two IDs", "bad date", "settled replaced", "active chain", "non A", "wrong owner"} {
		t.Run(mode, func(t *testing.T) {
			bp, bf, af, in := ap, a.Clone(), afterP.Clone(), ri
			switch mode {
			case "P extra ID":
				in.EventIDs = []string{liID(101)}
			case "U missing ID", "U duplicate", "U bad UUID", "settled replaced":
				af = afterU.Clone()
				in.Today = liDay(t, 1)
				if mode != "U missing ID" {
					in.EventIDs = []string{liID(101)}
				}
				if mode == "U duplicate" {
					in.EventIDs[0] = strings.ToLower(liID(100))
				}
				if mode == "U bad UUID" {
					in.EventIDs[0] = strings.Repeat("z", 36)
				}
				if mode == "settled replaced" {
					af.Traces[0].SettledAt = liPtr(liAt(20))
				}
			case "two IDs":
				in.EventIDs = []string{"", ""}
			case "bad date":
				in.Today = core.LocalDate{}
			case "active chain":
				bp = p
				bf = g.Clone()
			case "non A":
				bf.Traces[0].Status = core.TraceUnfinished
				bf.Traces[0].ContentUpdatedAt = liAt(11)
				bp = liPrior(t, bf, []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}, {liID(3), liID(101), core.TraceUnfinished, liAt(11)}})
			case "wrong owner":
				in.ChainID = core.TaskChainID(liID(999))
			}
			oldB, oldA, oldIDs := bf.Clone(), af.Clone(), append([]string(nil), in.EventIDs...)
			r, e := auditReactivateHistoryVNext(bp, bf, af, in)
			liZero(t, r, e)
			if !reflect.DeepEqual(bf, oldB) || !reflect.DeepEqual(af, oldA) || !reflect.DeepEqual(in.EventIDs, oldIDs) {
				t.Fatal("failure mutation")
			}
		})
	}
	// A bad capture stream cannot be blessed by a graph-only successful core replay.
	s := liSnapshot(t, afterU, []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}})
	if _, e := inspectForwardClockOrdinaryHistoryVNext(s); e == nil {
		t.Fatal("uncaptured U accepted")
	}
	for kind := historyCommandManualProgress + 1; kind < 16; kind++ {
		r := auditedHistoryCommand{proof: &historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, prior: p, after: ap}}
		if r.valid() {
			t.Fatal("unknown kind")
		}
	}
}
func TestLifecycleIndependentBudgetAndAST(t *testing.T) {
	g := liSeed(t, liDate)
	p := liPrior(t, g, nil)
	bad := g.Clone()
	bad.Traces[0].ID = "not UUID"
	huge := g.Clone()
	huge.Definitions[0].Title = strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	for _, react := range []bool{false, true} {
		for _, tc := range []struct {
			b, a  core.TaskGraph
			phase historyCommandPhase
		}{{huge, bad, historyCommandBefore}, {bad, huge, historyCommandAfter}, {bad, bad, historyCommandIDs}} {
			var r auditedHistoryCommand
			var e error
			if react {
				r, e = auditReactivateHistoryVNext(p, tc.b, tc.a, reactivateHistoryInput{EventIDs: []string{"", ""}})
			} else {
				r, e = auditAbandonHistoryVNext(p, tc.b, tc.a, abandonHistoryInput{})
			}
			liZero(t, r, e)
			var be *historyCommandBudgetError
			if !errors.As(e, &be) || be.Phase != tc.phase {
				t.Fatalf("budget precedence: %v", e)
			}
		}
	}
	f, e := parser.ParseFile(token.NewFileSet(), "history_lifecycle_command_vnext.go", nil, 0)
	liOK(t, e)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if v, ok := d.(*ast.FuncDecl); ok {
			funcs[v.Name.Name] = v
		}
	}
	callName := func(c *ast.CallExpr) string {
		switch x := c.Fun.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			if v, ok := x.X.(*ast.Ident); ok {
				return v.Name + "." + x.Sel.Name
			}
		}
		return "unexpected"
	}
	for _, name := range []string{"auditAbandonHistoryVNext", "auditReactivateHistoryVNext"} {
		fn := funcs[name]
		if fn == nil {
			t.Fatal("entry missing")
		}
		gate := "historyCommandAbandonIDBudget"
		if strings.Contains(name, "Reactivate") {
			gate = "historyCommandReactivateIDBudget"
		}
		want := []string{"prior.valid", "historyCommandGraphBudget", "historyCommandGraphBudget", gate}
		for i, w := range want {
			s, ok := fn.Body.List[i].(*ast.IfStmt)
			if !ok || s.Else != nil {
				t.Fatal("nonlinear preflight")
			}
			var calls []string
			ast.Inspect(s, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					calls = append(calls, callName(c))
				}
				return true
			})
			allowed := []string{w}
			if i == 0 {
				allowed = append(allowed, "historyVNextFail")
			}
			if !reflect.DeepEqual(calls, allowed) {
				t.Fatalf("gate calls %v", calls)
			}
			if len(s.Body.List) != 1 {
				t.Fatal("gate must return")
			}
			r, ok := s.Body.List[0].(*ast.ReturnStmt)
			if !ok {
				t.Fatal("gate return")
			}
			z, ok := r.Results[0].(*ast.CompositeLit)
			if !ok || len(z.Elts) != 0 {
				t.Fatal("nonzero gate result")
			}
		}
		bind, ok := fn.Body.List[4].(*ast.AssignStmt)
		if !ok || callName(bind.Rhs[0].(*ast.CallExpr)) != "historyCommandBind" {
			t.Fatal("binding before gates")
		}
	}
	for _, name := range []string{"historyCommandAbandonIDBudget", "historyCommandReactivateIDBudget"} {
		ast.Inspect(funcs[name], func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				v := callName(c)
				if v != "len" && v != "historyCommandBudgetFail" {
					t.Fatalf("metadata scans or allocates: %s", v)
				}
			}
			switch n.(type) {
			case *ast.RangeStmt, *ast.ForStmt:
				t.Fatal("unbounded ID metadata loop")
			}
			return true
		})
	}
	first := funcs["historyCommandReactivateIDBudget"].Body.List[0].(*ast.IfStmt)
	index := false
	ast.Inspect(first, func(n ast.Node) bool {
		if _, ok := n.(*ast.IndexExpr); ok {
			index = true
		}
		return true
	})
	if index {
		t.Fatal("ID indexed before count guard")
	}
}
func TestLifecycleIndependentCyclesAndConcurrency(t *testing.T) {
	g := liSeed(t, liDate)
	ev := []liEvent{}
	for round := 0; round < 3; round++ {
		at := liAt(float64(10 + round*10))
		p := liPrior(t, g, ev)
		a := liAbandon(g, 0, at)
		id := liID(100 + round)
		in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: at, EventID: id}
		r, e := auditAbandonHistoryVNext(p, g, a, in)
		ev = append(ev, liEvent{liID(3), id, core.TraceAbandoned, at})
		liCheck(t, r, e, liSnapshot(t, a, ev))
		p = liPrior(t, a, ev)
		g = liReactivate(a, 0, core.TracePending, liAt(float64(11+round*10)))
		ri := reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(float64(11 + round*10)), Today: liDate}
		r, e = auditReactivateHistoryVNext(p, a, g, ri)
		liCheck(t, r, e, liSnapshot(t, g, ev))
	}
	p := liPrior(t, g, ev)
	a := liAbandon(g, 0, liAt(50))
	in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(50), EventID: liID(150)}
	want := liSnapshot(t, a, append(append([]liEvent{}, ev...), liEvent{liID(3), liID(150), core.TraceAbandoned, liAt(50)}))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				r, e := auditAbandonHistoryVNext(p, g, a, in)
				if e != nil {
					t.Error(e)
					return
				}
				s, e := r.snapshotCopy()
				if e != nil || !reflect.DeepEqual(s, want) {
					t.Error("concurrent projection")
				}
				s.Object["traces"].Array[0].Object["status"] = liS("local poison")
			}
		}()
	}
	wg.Wait()
}
func TestLifecycleIndependentRichFactsAndValidTampering(t *testing.T) {
	g := liSeed(t, liDate)
	g.Traces[0].DescriptionText = liPtr("trace description")
	g.Traces[0].ManualProgressPercent = liPtr(37)
	g.Traces[0].PinOrder = liPtr(1)
	g.Definitions[0].DescriptionText = liPtr("draft description")
	g.Definitions[0].PlannedSubtasks = []core.PlannedSubtask{{ID: core.PlannedSubtaskID(liID(40)), LineageID: core.SubtaskLineageID(liID(41)), Title: "planned", Difficulty: core.DifficultyHard, Position: 1, CreatedAt: liAt(0)}}
	g.Chains[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(liID(42)), Body: "chain note", CreatedAt: liAt(0), UpdatedAt: liAt(0)}}
	g.Traces[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(liID(43)), Body: "trace note", CreatedAt: liAt(1), UpdatedAt: liAt(1)}, {ID: core.TaskNoteEntryID(liID(44)), Body: "", CreatedAt: liAt(1), UpdatedAt: liAt(2), DeletedAt: liPtr(liAt(2))}}
	for i, status := range []core.SubtaskStatus{core.SubtaskPending, core.SubtaskCompleted, core.SubtaskCancelledDraft} {
		s := core.Subtask{ID: core.SubtaskID(liID(50 + i)), LineageID: core.SubtaskLineageID(liID(60 + i)), TraceID: g.Traces[0].ID, Title: fmt.Sprint("child ", i), Status: status, Difficulty: core.DifficultyMedium, Position: i + 1, CreatedAt: liAt(1), UpdatedAt: liAt(2)}
		if i == 1 {
			s.CompletedAt = liPtr(liAt(2))
		}
		if i == 2 {
			s.SettledAt = liPtr(liAt(2))
			s.DraftCancellationID = liPtr(core.CancellationID(liID(70)))
		}
		g.Subtasks = append(g.Subtasks, s)
	}
	// Unrelated ordinary chain clock is not a global mutation frontier.
	g.Chains = append(g.Chains, core.TaskChain{ID: core.TaskChainID(liID(80)), State: core.ChainActive, CreatedAt: liAt(0), UpdatedAt: liAt(1000)})
	g.Definitions = append(g.Definitions, core.TaskDefinition{ID: core.TaskDefinitionID(liID(81)), ChainID: core.TaskChainID(liID(80)), Sequence: 1, Title: "unrelated", CreatedAt: liAt(0), ContentUpdatedAt: liAt(0)})
	p := liPrior(t, g, nil)
	after := liAbandon(g, 0, liAt(10))
	in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: liAt(10), EventID: liID(100)}
	r, e := auditAbandonHistoryVNext(p, g, after, in)
	liCheck(t, r, e, liSnapshot(t, after, []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}}))
	mutations := []func(*core.TaskGraph){func(x *core.TaskGraph) { x.Definitions[0].Title = "changed" }, func(x *core.TaskGraph) { *x.Definitions[0].DescriptionText = "changed" }, func(x *core.TaskGraph) { x.Definitions[0].PlannedSubtasks[0].Title = "changed" }, func(x *core.TaskGraph) { x.Subtasks[0].Title = "changed" }, func(x *core.TaskGraph) { x.Traces[0].NoteEntries[0].Body = "changed" }, func(x *core.TaskGraph) { *x.Traces[0].ManualProgressPercent = 42 }, func(x *core.TaskGraph) { x.Chains[1].UpdatedAt = liAt(1001) }, func(x *core.TaskGraph) { x.Chains[0].NoteEntries[0].Body = "changed" }, func(x *core.TaskGraph) { x.Days[0].ReviewSummary = liPtr("changed") }, func(x *core.TaskGraph) { x.Definitions[0], x.Definitions[1] = x.Definitions[1], x.Definitions[0] }}
	for i, m := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			bad := after.Clone()
			m(&bad)
			liOK(t, core.ValidateTaskGraph(bad))
			r, e := auditAbandonHistoryVNext(p, g, bad, in)
			liZero(t, r, e)
		})
	}
}
func TestLifecycleIndependentOpenUSettlementAndFutureCancellation(t *testing.T) {
	g := liSeed(t, liDate)
	a := liAbandon(g, 0, liAt(10))
	ev := []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}}
	p := liPrior(t, a, ev)
	u := liReactivate(a, 0, core.TraceUnfinished, liAt(20))
	in := reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: liDay(t, 1), Now: liAt(20), EventIDs: []string{liID(101)}}
	r, e := auditReactivateHistoryVNext(p, a, u, in)
	ev = append(ev, liEvent{liID(3), liID(101), core.TraceUnfinished, liAt(20)})
	liCheck(t, r, e, liSnapshot(t, u, ev))
	p = r.proof.after
	k, e := core.NewTaskKernel(u)
	liOK(t, e)
	liOK(t, k.SettleTaskDays(liDay(t, 1), liAt(30)))
	locked := k.Snapshot()
	r, e = auditSettlementHistoryVNext(p, u, locked, settlementHistoryInput{Today: liDay(t, 1), Now: liAt(30)})
	liCheck(t, r, e, liSnapshot(t, locked, ev))
	// X with retained A is rejected by complete fixed snapshot, not a relaxation request.
	g = liSeed(t, liDay(t, 1))
	a = liAbandon(g, 0, liAt(10))
	ev = []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}}
	p = liPrior(t, a, ev)
	g = liReactivate(a, 0, core.TracePending, liAt(20))
	r, e = auditReactivateHistoryVNext(p, a, g, reactivateHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: liDate, Now: liAt(20)})
	liCheck(t, r, e, liSnapshot(t, g, ev))
	k, e = core.NewTaskKernel(g)
	liOK(t, e)
	liOK(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(liID(200)), CancellationID: core.CancellationID(liID(201)), Today: liDate, Now: liAt(30)}))
	if _, e := inspectForwardClockOrdinaryHistoryVNext(liSnapshot(t, k.Snapshot(), ev)); e == nil {
		t.Fatal("X plus A accepted")
	}
}

func TestLifecycleIndependentClockBitsAndRepresentation(t *testing.T) {
	for _, now := range []float64{-5.25, math.SmallestNonzeroFloat64, 0, math.Copysign(0, -1), math.MaxFloat64} {
		t.Run(fmt.Sprintf("%016x", math.Float64bits(now)), func(t *testing.T) {
			g := liSeed(t, liDate)
			for i := range g.Days {
				g.Days[i].CreatedAt = liAt(-10)
				g.Days[i].UpdatedAt = liAt(-10)
			}
			g.Chains[0].CreatedAt = liAt(-10)
			g.Chains[0].UpdatedAt = liAt(-10)
			g.Definitions[0].CreatedAt = liAt(-10)
			g.Definitions[0].ContentUpdatedAt = liAt(-10)
			g.Traces[0].CreatedAt = liAt(-10)
			g.Traces[0].ContentUpdatedAt = liAt(-10)
			p := liPrior(t, g, nil)
			at := liAt(now)
			a := liAbandon(g, 0, at)
			in := abandonHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Now: at, EventID: liID(100)}
			r, e := auditAbandonHistoryVNext(p, g, a, in)
			liCheck(t, r, e, liSnapshot(t, a, []liEvent{{liID(3), liID(100), core.TraceAbandoned, at}}))
			if r.proof.now != at {
				t.Fatal("clock bits normalized")
			}
			if now == 0 {
				bad := a.Clone()
				other := core.Instant(uint64(at) ^ (uint64(1) << 63))
				bad.Traces[0].ContentUpdatedAt = other
				rr, ee := auditAbandonHistoryVNext(p, g, bad, in)
				liZero(t, rr, ee)
			}
		})
	}
	g := liSeed(t, liDate)
	a := liAbandon(g, 0, liAt(10))
	lower := strings.ToLower(liID(3))
	events := []liEvent{{lower, strings.ToLower(liID(100)), core.TraceAbandoned, liAt(10)}}
	p := liPrior(t, a, events)
	u := liReactivate(a, 0, core.TraceUnfinished, liAt(20))
	in := reactivateHistoryInput{ChainID: a.Chains[0].ID, TraceID: a.Traces[0].ID, Today: liDay(t, 1), Now: liAt(20), EventIDs: []string{strings.ToLower(liID(101))}}
	// Preserve old pair-key/event raw bytes; new event identity alone is normalized.
	want := liSnapshot(t, u, []liEvent{{lower, strings.ToLower(liID(100)), core.TraceAbandoned, liAt(10)}, {lower, liID(101), core.TraceUnfinished, liAt(20)}})
	c := want.Object["classifications"]
	streams := c.Object["snapshotEventsByTraceID"]
	streams.Array[1].Array[1].Object["traceID"] = liKey(liID(3))
	latest := c.Object["snapshotsByTraceID"]
	latest.Array[1].Object["traceID"] = liKey(liID(3))
	r, e := auditReactivateHistoryVNext(p, a, u, in)
	liCheck(t, r, e, want)
	// Independently valid snapshots with rewritten old identities still fail prefix.
	rewritten := liSnapshot(t, u, []liEvent{{liID(3), liID(199), core.TraceAbandoned, liAt(10)}, {liID(3), liID(101), core.TraceUnfinished, liAt(20)}})
	q, e := inspectForwardClockOrdinaryHistoryVNext(rewritten)
	liOK(t, e)
	if checkForwardHistorySuccessor(p, q) == nil {
		t.Fatal("rewritten prefix accepted")
	}
	// Canonical task representation must also bind round-trip exactly.
	snapshot := liSnapshot(t, a, []liEvent{{liID(3), liID(100), core.TraceAbandoned, liAt(10)}})
	snapshot.Object["traces"].Array[0].Object["id"] = liKey(lower)
	p, e = inspectForwardClockOrdinaryHistoryVNext(snapshot)
	liOK(t, e)
	rr, ee := auditReactivateHistoryVNext(p, a, u, in)
	liZero(t, rr, ee)
}
