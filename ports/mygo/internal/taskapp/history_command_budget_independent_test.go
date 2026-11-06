// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Full typed model manifest, independently transcribed from frozen core model.
// Alias names and pointers are explicit so additions cannot silently bypass it.
const icManifest = `TaskGraph:Days=[]core.Day,Chains=[]core.TaskChain,Definitions=[]core.TaskDefinition,Traces=[]core.DayTrace,Subtasks=[]core.Subtask
Day:ID=core.DayID,Date=core.LocalDate,LockedAt=*core.Instant,ReviewSummary=*string,ReviewUnfinishedReason=*string,ReviewTomorrowNote=*string,CreatedAt=core.Instant,UpdatedAt=core.Instant
TaskChain:ID=core.TaskChainID,State=core.TaskChainState,CycleMembership=*core.TaskCycleMembership,NoteEntries=[]core.TaskNoteEntry,CreatedAt=core.Instant,UpdatedAt=core.Instant
TaskDefinition:ID=core.TaskDefinitionID,ChainID=core.TaskChainID,Sequence=int,Title=string,DescriptionText=*string,PlannedSubtasks=[]core.PlannedSubtask,CreatedAt=core.Instant,ContentUpdatedAt=core.Instant,SupersededAt=*core.Instant,SupersededByDefinitionID=*core.TaskDefinitionID
DayTrace:ID=core.DayTraceID,ChainID=core.TaskChainID,DefinitionID=core.TaskDefinitionID,Date=core.LocalDate,Status=core.TraceStatus,Priority=int,PinOrder=*int,ContinuationSeq=int,DescriptionText=*string,NoteEntries=[]core.TaskNoteEntry,ManualProgressPercent=*int,CarriedFromTraceID=*core.DayTraceID,ChangedToTraceID=*core.DayTraceID,CreatedAt=core.Instant,ContentUpdatedAt=core.Instant,CompletedAt=*core.Instant,SettledAt=*core.Instant,DraftCancellationID=*core.CancellationID,DraftCancelledOn=*core.LocalDate
Subtask:ID=core.SubtaskID,LineageID=core.SubtaskLineageID,TraceID=core.DayTraceID,Title=string,Status=core.SubtaskStatus,Difficulty=core.Difficulty,Position=int,CarriedFromSubtaskID=*core.SubtaskID,CreatedAt=core.Instant,UpdatedAt=core.Instant,CompletedAt=*core.Instant,SettledAt=*core.Instant,DraftCancellationID=*core.CancellationID
TaskNoteEntry:ID=core.TaskNoteEntryID,Body=string,CreatedAt=core.Instant,UpdatedAt=core.Instant,DeletedAt=*core.Instant
PlannedSubtask:ID=core.PlannedSubtaskID,LineageID=core.SubtaskLineageID,Title=string,Difficulty=core.Difficulty,Position=int,CreatedAt=core.Instant
TaskCycleMembership:SeriesID=core.TaskCycleSeriesID,OccurrenceDate=core.LocalDate
LocalDate:Year=int,Month=int,Day=int`

func icTypeLines(typ reflect.Type, seen map[reflect.Type]bool, out *[]string) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	fields := []string{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fields = append(fields, f.Name+"="+f.Type.String())
		icTypeLines(f.Type, seen, out)
	}
	*out = append(*out, typ.Name()+":"+strings.Join(fields, ","))
}
func icFillModel(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			icFillModel(v.Field(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		icFillModel(v.Index(0))
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		icFillModel(v.Elem())
	}
}
func icLeaves(v reflect.Value, path string, out map[string]reflect.Value, kind reflect.Kind) {
	if v.Kind() == kind {
		out[path] = v
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			icLeaves(v.Field(i), path+"."+v.Type().Field(i).Name, out, kind)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			icLeaves(v.Index(i), path+"[]", out, kind)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			icLeaves(v.Elem(), path+"*", out, kind)
		}
	}
}
func icModel() core.TaskGraph {
	var g core.TaskGraph
	icFillModel(reflect.ValueOf(&g).Elem())
	return g
}
func icBudgetError(t *testing.T, e error, phase historyCommandPhase, reason historyCommandBudgetReason) {
	t.Helper()
	var b *historyCommandBudgetError
	if !errors.As(e, &b) || b.Phase != phase || b.Reason != reason {
		t.Fatalf("wanted phase=%v reason=%v, got %T %v", phase, reason, e, e)
	}
}

func TestICModelManifestAndEveryString(t *testing.T) {
	var got []string
	icTypeLines(reflect.TypeOf(core.TaskGraph{}), map[reflect.Type]bool{}, &got)
	want := strings.Split(icManifest, "\n")
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("typed model drift:\n%s", strings.Join(got, "\n"))
	}
	// Metadata fixtures are intentionally domain-invalid, making budget errors
	// decisive about phase. Reusing one backing string does not reduce raw charge.
	huge := strings.Repeat("\xff", canonical.DefaultLimits().MaxBytes+1)
	g := icModel()
	leaves := map[string]reflect.Value{}
	icLeaves(reflect.ValueOf(&g).Elem(), "TaskGraph", leaves, reflect.String)
	if len(leaves) != 34 {
		t.Fatalf("string leaves=%d", len(leaves))
	}
	k, _ := icSource(t, "P")
	valid := k.Snapshot()
	prior := icProof(t, valid, nil)
	for path, leaf := range leaves {
		t.Run(path, func(t *testing.T) {
			leaf.SetString(huge)
			defer leaf.SetString("")
			for _, phase := range []historyCommandPhase{historyCommandBefore, historyCommandAfter} {
				_, e := historyCommandGraphBudget(g, phase)
				icBudgetError(t, e, phase, historyCommandRawBytes)
				b, a := g, core.TaskGraph{}
				if phase == historyCommandAfter {
					b, a = core.TaskGraph{}, g
				}
				r, e := auditSettlementHistoryVNext(prior, b, a, settlementHistoryInput{Today: icDate})
				icZero(t, r, e)
				icBudgetError(t, e, phase, historyCommandRawBytes)
				r, e = auditCompletionHistoryVNext(prior, b, a, completionHistoryInput{})
				icZero(t, r, e)
				icBudgetError(t, e, phase, historyCommandRawBytes)
			}
			if leaf.String() != huge {
				t.Fatal("oversize input altered")
			}
		})
	}
}

func TestICStructureOptionalAndLogicalCosts(t *testing.T) {
	costs := map[string]uint64{
		"TaskGraph.Days[].LockedAt": 2, "TaskGraph.Days[].ReviewSummary": 2, "TaskGraph.Days[].ReviewUnfinishedReason": 2, "TaskGraph.Days[].ReviewTomorrowNote": 2,
		"TaskGraph.Chains[].CycleMembership": 14, "TaskGraph.Chains[].NoteEntries[].DeletedAt": 2,
		"TaskGraph.Definitions[].DescriptionText": 2, "TaskGraph.Definitions[].SupersededAt": 2, "TaskGraph.Definitions[].SupersededByDefinitionID": 4,
		"TaskGraph.Traces[].PinOrder": 2, "TaskGraph.Traces[].DescriptionText": 2, "TaskGraph.Traces[].ManualProgressPercent": 2, "TaskGraph.Traces[].CarriedFromTraceID": 4, "TaskGraph.Traces[].ChangedToTraceID": 4, "TaskGraph.Traces[].CompletedAt": 2, "TaskGraph.Traces[].SettledAt": 2, "TaskGraph.Traces[].DraftCancellationID": 2, "TaskGraph.Traces[].DraftCancelledOn": 8, "TaskGraph.Traces[].NoteEntries[].DeletedAt": 2,
		"TaskGraph.Subtasks[].CarriedFromSubtaskID": 4, "TaskGraph.Subtasks[].CompletedAt": 2, "TaskGraph.Subtasks[].SettledAt": 2, "TaskGraph.Subtasks[].DraftCancellationID": 2,
	}
	g := icModel()
	pointers := map[string]reflect.Value{}
	icLeaves(reflect.ValueOf(&g).Elem(), "TaskGraph", pointers, reflect.Pointer)
	if len(pointers) != len(costs) {
		t.Fatalf("optional drift %d", len(pointers))
	}
	all, e := historyCommandGraphBudget(g, historyCommandBefore)
	icOK(t, e)
	want := uint64(5 + 17 + 13 + 19 + 33 + 25 + 11 + 11 + 17)
	for _, n := range costs {
		want += n
	}
	if all.nodes != want {
		t.Fatalf("nodes=%d want %d", all.nodes, want)
	}
	for path, v := range pointers {
		cost, ok := costs[path]
		if !ok {
			t.Fatal("unknown optional", path)
		}
		save := reflect.New(v.Type()).Elem()
		save.Set(v)
		v.SetZero()
		b, e := historyCommandGraphBudget(g, historyCommandBefore)
		icOK(t, e)
		if all.nodes-b.nodes != cost {
			t.Fatalf("%s: delta=%d want %d", path, all.nodes-b.nodes, cost)
		}
		v.Set(save)
	}
	// Shared nested slices and optional strings are charged per logical occurrence.
	body := strings.Repeat("x", canonical.DefaultLimits().MaxBytes/2)
	notes := []core.TaskNoteEntry{{Body: body}}
	share := core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: notes}, {NoteEntries: notes}}, Days: []core.Day{{ReviewSummary: &body}}}
	_, e = historyCommandGraphBudget(share, historyCommandBefore)
	icBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
	small := core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: notes}, {NoteEntries: notes}}}
	b, e := historyCommandGraphBudget(small, historyCommandBefore)
	icOK(t, e)
	if b.rawBytes != uint64(len(body)*2) || b.nodes != 53 {
		t.Fatalf("logical alias cost %+v", b)
	}
	// Compare production gate with actual canonical task forest nodes and raw bytes
	// for source-derived full snapshots, independently walking canonical values.
	for _, state := range []string{"P", "C", "A", "U", "R", "D", "X"} {
		k, ev := icSource(t, state)
		g := k.Snapshot()
		s := icSnapshot(t, g, ev)
		got, e := historyCommandGraphBudget(g, historyCommandBefore)
		icOK(t, e)
		var nodes, raw uint64
		for _, key := range []string{"days", "chains", "definitions", "traces", "subtasks"} {
			n, b := icCanonicalCost(s.Object[key])
			nodes += n
			raw += b
		}
		if got.nodes != nodes || got.rawBytes != raw {
			t.Fatalf("%s gate=%+v canonical nodes=%d bytes=%d", state, got, nodes, raw)
		}
	}
}
func icCanonicalCost(v canonical.Value) (uint64, uint64) {
	nodes := uint64(1 + len(v.Object))
	raw := uint64(0)
	if v.Kind == canonical.String {
		raw = uint64(len(v.String))
	}
	for _, x := range v.Object {
		n, b := icCanonicalCost(x)
		nodes += n
		raw += b
	}
	for _, x := range v.Array {
		n, b := icCanonicalCost(x)
		nodes += n
		raw += b
	}
	return nodes, raw
}

func TestICBudgetShapeAndIDStages(t *testing.T) {
	N := canonical.DefaultLimits().MaxNodes
	cases := map[string]func() core.TaskGraph{
		"days":        func() core.TaskGraph { return core.TaskGraph{Days: make([]core.Day, N/17+1)} },
		"chains":      func() core.TaskGraph { return core.TaskGraph{Chains: make([]core.TaskChain, N/13+1)} },
		"definitions": func() core.TaskGraph { return core.TaskGraph{Definitions: make([]core.TaskDefinition, N/19+1)} },
		"traces":      func() core.TaskGraph { return core.TaskGraph{Traces: make([]core.DayTrace, N/33+1)} },
		"subtasks":    func() core.TaskGraph { return core.TaskGraph{Subtasks: make([]core.Subtask, N/25+1)} },
		"aggregate": func() core.TaskGraph {
			return core.TaskGraph{Days: make([]core.Day, 30000), Chains: make([]core.TaskChain, 40000)}
		},
		"chain-notes": func() core.TaskGraph {
			return core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: make([]core.TaskNoteEntry, N/11+1)}}}
		},
		"trace-notes": func() core.TaskGraph {
			return core.TaskGraph{Traces: []core.DayTrace{{NoteEntries: make([]core.TaskNoteEntry, N/11+1)}}}
		},
		"planned": func() core.TaskGraph {
			return core.TaskGraph{Definitions: []core.TaskDefinition{{PlannedSubtasks: make([]core.PlannedSubtask, N/17+1)}}}
		},
		"shared-notes-total": func() core.TaskGraph {
			notes := make([]core.TaskNoteEntry, 50000)
			return core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: notes}, {NoteEntries: notes}}}
		},
	}
	k, _ := icSource(t, "P")
	p := icProof(t, k.Snapshot(), nil)
	for name, makeGraph := range cases {
		t.Run(name, func(t *testing.T) {
			g := makeGraph()
			for _, phase := range []historyCommandPhase{historyCommandBefore, historyCommandAfter} {
				b, a := g, core.TaskGraph{}
				if phase == historyCommandAfter {
					b, a = core.TaskGraph{}, g
				}
				r, e := auditSettlementHistoryVNext(p, b, a, settlementHistoryInput{})
				icZero(t, r, e)
				icBudgetError(t, e, phase, historyCommandShape)
				r, e = auditCompletionHistoryVNext(p, b, a, completionHistoryInput{})
				icZero(t, r, e)
				icBudgetError(t, e, phase, historyCommandShape)
			}
		})
	}
	// Both caller graphs are domain-invalid but bounded. IDs must reject first.
	for _, ids := range [][]string{{"", ""}, {""}, {strings.Repeat("A", 35)}, {strings.Repeat("A", 37)}, {strings.Repeat("A", canonical.DefaultLimits().MaxBytes+1)}} {
		r, e := auditSettlementHistoryVNext(p, core.TaskGraph{}, core.TaskGraph{}, settlementHistoryInput{EventIDs: ids})
		icZero(t, r, e)
		reason := historyCommandIDLength
		if len(ids) > 1 {
			reason = historyCommandIDCount
		}
		icBudgetError(t, e, historyCommandIDs, reason)
	}
	for _, in := range []completionHistoryInput{{}, {ChainID: core.TaskChainID(icID(100))}, {ChainID: core.TaskChainID(strings.Repeat("x", 100)), TraceID: core.DayTraceID(icID(102))}} {
		r, e := auditCompletionHistoryVNext(p, core.TaskGraph{}, core.TaskGraph{}, in)
		icZero(t, r, e)
		icBudgetError(t, e, historyCommandIDs, historyCommandIDLength)
	}
}

func TestICCheckedArithmeticAndCanonicalBoundary(t *testing.T) {
	max := ^uint64(0)
	values := []uint64{0, 1, 2, 999999, 1000000, 1000001, max / 2, max - 1, max}
	for _, sum := range values {
		for _, count := range values {
			for _, cost := range []uint64{0, 1, 2, 17, max} {
				for _, limit := range []uint64{0, 1000000, max} {
					out, ok := historyCommandAddProduct(sum, count, cost, limit)
					n := new(big.Int).Mul(new(big.Int).SetUint64(count), new(big.Int).SetUint64(cost))
					n.Add(n, new(big.Int).SetUint64(sum))
					want := cost > 0 && n.Cmp(new(big.Int).SetUint64(limit)) <= 0
					if ok != want || (ok && out != n.Uint64()) {
						t.Fatal("checked product mismatch", sum, count, cost, limit)
					}
					a, ok := historyCheckedAdd(sum, count, limit)
					n.Add(new(big.Int).SetUint64(sum), new(big.Int).SetUint64(count))
					want = n.Cmp(new(big.Int).SetUint64(limit)) <= 0
					if ok != want || (ok && a != n.Uint64()) {
						t.Fatal("checked add mismatch")
					}
				}
			}
		}
	}
	B := canonical.DefaultLimits().MaxBytes
	s := strings.Repeat("x", B)
	g := core.TaskGraph{Definitions: []core.TaskDefinition{{Title: s}}}
	b, e := historyCommandGraphBudget(g, historyCommandBefore)
	icOK(t, e)
	if b.rawBytes != uint64(B) {
		t.Fatal("raw boundary")
	}
	g.Definitions[0].Title = s + "x"
	_, e = historyCommandGraphBudget(g, historyCommandBefore)
	icBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
	// Exact legal full-envelope boundary, not merely metadata acceptance. ASCII
	// leaves are a strict lower bound, not 6x pessimistic expansion. Both graphs
	// independently get B, so this > B/2 no-op must not be rejected in aggregate.
	k, _ := icSource(t, "P")
	g = k.Snapshot()
	g.Days[0].ReviewSummary = icP("")
	snapshot := icSnapshot(t, g, nil)
	envelope := icObj(map[string]canonical.Value{"formatVersion": icNum(7), "snapshot": snapshot})
	overhead := len(icBytes(t, envelope))
	g.Days[0].ReviewSummary = icP(strings.Repeat("x", B-overhead))
	snapshot = icSnapshot(t, g, nil)
	envelope.Object["snapshot"] = snapshot
	if len(icBytes(t, envelope)) != B {
		t.Fatal("not exact canonical byte boundary")
	}
	p, e := inspectForwardClockOrdinaryHistoryVNext(snapshot)
	icOK(t, e)
	r, e := auditSettlementHistoryVNext(p, g, g, settlementHistoryInput{Today: icDate, Now: icAt(1)})
	icOK(t, e)
	icResult(t, r, snapshot, false)
	// One extra encoded byte still passes raw gate but the complete inspector fails.
	larger := g.Clone()
	*larger.Days[0].ReviewSummary += "x"
	_, e = historyCommandGraphBudget(larger, historyCommandBefore)
	icOK(t, e)
	largeSnapshot := icSnapshot(t, larger, nil)
	if _, e = inspectForwardClockOrdinaryHistoryVNext(largeSnapshot); e == nil {
		t.Fatal("final envelope limit bypass")
	}
}

func icCallName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}
func TestICASTPreflightAndClosedPipeline(t *testing.T) {
	fs := token.NewFileSet()
	file, e := parser.ParseFile(fs, "history_command_vnext.go", nil, 0)
	icOK(t, e)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok {
			funcs[f.Name.Name] = f
		}
	}
	lifecycle, e := parser.ParseFile(fs, "history_lifecycle_command_vnext.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, decl := range lifecycle.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
			funcs[f.Name.Name] = f
		}
	}
	for _, name := range []string{"auditSettlementHistoryVNext", "auditCompletionHistoryVNext", "auditAbandonHistoryVNext", "auditReactivateHistoryVNext"} {
		f := funcs[name]
		if f == nil {
			t.Fatal("missing entry")
		}
		want := []string{"valid", "historyCommandGraphBudget", "historyCommandGraphBudget", "historyCommandEventIDBudget"}
		if strings.Contains(name, "Completion") {
			want[3] = "historyCommandCompletionIDBudget"
		}
		if name == "auditAbandonHistoryVNext" {
			want[3] = "historyCommandAbandonIDBudget"
		}
		if name == "auditReactivateHistoryVNext" {
			want[3] = "historyCommandReactivateIDBudget"
		}
		for i, fn := range want {
			stmt, ok := f.Body.List[i].(*ast.IfStmt)
			if !ok {
				t.Fatal("not straight-line error gate", name, i)
			}
			calls := []string{}
			ast.Inspect(stmt, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					calls = append(calls, icCallName(c))
				}
				return true
			})
			allowed := map[string]bool{fn: true}
			if i == 0 {
				allowed["historyVNextFail"] = true
			}
			for _, c := range calls {
				if !allowed[c] {
					t.Fatal("unreviewed preflight call", name, i, c)
				}
			}
			if len(stmt.Body.List) != 1 {
				t.Fatal("gate must immediately return")
			}
			if _, ok := stmt.Body.List[0].(*ast.ReturnStmt); !ok {
				t.Fatal("gate nonreturn")
			}
			if i == 1 || i == 2 {
				assign := stmt.Init.(*ast.AssignStmt)
				call := assign.Rhs[0].(*ast.CallExpr)
				arg := call.Args[0].(*ast.Ident).Name
				expected := "before"
				if i == 2 {
					expected = "after"
				}
				if arg != expected {
					t.Fatal("wrong graph gated", arg)
				}
			}
		}
		calls := map[string]int{}
		ast.Inspect(f.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				calls[icCallName(c)]++
			}
			return true
		})
		for _, fn := range []string{"historyCommandBind", "NewTaskKernel", "Snapshot", "historyCommandMatch", "historyCommandFinish"} {
			if calls[fn] != 1 {
				t.Fatal("pipeline call count", name, fn, calls[fn])
			}
		}
		if name == "auditAbandonHistoryVNext" && calls["AbandonChain"] != 1 {
			t.Fatal("abandon replay count")
		}
		if name == "auditReactivateHistoryVNext" && calls["ReactivateAbandonedChain"] != 1 {
			t.Fatal("reactivate replay count")
		}
		if strings.Contains(name, "Settlement") && calls["SettleTaskDays"] != 1 {
			t.Fatal("settlement replay count")
		}
		if strings.Contains(name, "Completion") && (calls["MarkCompleted"] != 1 || calls["UndoCompleted"] != 1) {
			t.Fatal("completion closed switch")
		}
	}
	finish := funcs["historyCommandFinish"]
	calls := []string{}
	ast.Inspect(finish.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			calls = append(calls, icCallName(c))
		}
		return true
	})
	last := -1
	for _, fn := range []string{"appendEmptyHistoryEvents", "classificationsCopy", "FromDomain", "inspectForwardClockOrdinaryHistoryVNext", "checkForwardHistorySuccessor"} {
		index := -1
		for i, n := range calls {
			if n == fn {
				if index >= 0 {
					t.Fatal("duplicate finish call", fn)
				}
				index = i
			}
		}
		if index <= last {
			t.Fatal("finish order", fn)
		}
		last = index
	}
	// Production budget must not hide content scans, reflection or allocations.
	budget, e := parser.ParseFile(fs, "history_command_vnext_budget.go", nil, 0)
	icOK(t, e)
	allowed := map[string]bool{"len": true, "uint64": true, "string": true, "DefaultLimits": true, "historyCheckedAdd": true, "historyCommandAddProduct": true, "historyCommandBudgetFail": true, "nodesAdd": true, "optional": true, "raw": true, "historyCommandRawOptional": true}
	ast.Inspect(budget, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			fn := icCallName(c)
			if !allowed[fn] {
				t.Errorf("unreviewed budget call: %s", fn)
			}
		}
		return true
	})
}
