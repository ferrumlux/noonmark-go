// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Explicit model manifest, independent of the implementation and generated
// neither from live schema nor the production gate. Any type/field drift fails.
func TestHistoryCommandBudgetModelManifest(t *testing.T) {
	want := map[string]string{
		"TaskGraph":           "Days:[]core.Day Chains:[]core.TaskChain Definitions:[]core.TaskDefinition Traces:[]core.DayTrace Subtasks:[]core.Subtask",
		"Day":                 "ID:core.DayID Date:core.LocalDate LockedAt:*core.Instant ReviewSummary:*string ReviewUnfinishedReason:*string ReviewTomorrowNote:*string CreatedAt:core.Instant UpdatedAt:core.Instant",
		"LocalDate":           "Year:int Month:int Day:int",
		"TaskChain":           "ID:core.TaskChainID State:core.TaskChainState CycleMembership:*core.TaskCycleMembership NoteEntries:[]core.TaskNoteEntry CreatedAt:core.Instant UpdatedAt:core.Instant",
		"TaskCycleMembership": "SeriesID:core.TaskCycleSeriesID OccurrenceDate:core.LocalDate",
		"TaskNoteEntry":       "ID:core.TaskNoteEntryID Body:string CreatedAt:core.Instant UpdatedAt:core.Instant DeletedAt:*core.Instant",
		"PlannedSubtask":      "ID:core.PlannedSubtaskID LineageID:core.SubtaskLineageID Title:string Difficulty:core.Difficulty Position:int CreatedAt:core.Instant",
		"TaskDefinition":      "ID:core.TaskDefinitionID ChainID:core.TaskChainID Sequence:int Title:string DescriptionText:*string PlannedSubtasks:[]core.PlannedSubtask CreatedAt:core.Instant ContentUpdatedAt:core.Instant SupersededAt:*core.Instant SupersededByDefinitionID:*core.TaskDefinitionID",
		"DayTrace":            "ID:core.DayTraceID ChainID:core.TaskChainID DefinitionID:core.TaskDefinitionID Date:core.LocalDate Status:core.TraceStatus Priority:int PinOrder:*int ContinuationSeq:int DescriptionText:*string NoteEntries:[]core.TaskNoteEntry ManualProgressPercent:*int CarriedFromTraceID:*core.DayTraceID ChangedToTraceID:*core.DayTraceID CreatedAt:core.Instant ContentUpdatedAt:core.Instant CompletedAt:*core.Instant SettledAt:*core.Instant DraftCancellationID:*core.CancellationID DraftCancelledOn:*core.LocalDate",
		"Subtask":             "ID:core.SubtaskID LineageID:core.SubtaskLineageID TraceID:core.DayTraceID Title:string Status:core.SubtaskStatus Difficulty:core.Difficulty Position:int CarriedFromSubtaskID:*core.SubtaskID CreatedAt:core.Instant UpdatedAt:core.Instant CompletedAt:*core.Instant SettledAt:*core.Instant DraftCancellationID:*core.CancellationID",
	}
	got := map[string]string{}
	var visit func(reflect.Type)
	visit = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		if _, ok := got[typ.Name()]; ok {
			return
		}
		var fields []string
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			fields = append(fields, f.Name+":"+f.Type.String())
		}
		got[typ.Name()] = strings.Join(fields, " ")
		for i := 0; i < typ.NumField(); i++ {
			visit(typ.Field(i).Type)
		}
	}
	visit(reflect.TypeOf(core.TaskGraph{}))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("budget/schema review required: got %#v", got)
	}
}

func hcMetadataGraph() core.TaskGraph {
	return core.TaskGraph{Days: []core.Day{{}}, Chains: []core.TaskChain{{NoteEntries: []core.TaskNoteEntry{{}}}}, Definitions: []core.TaskDefinition{{PlannedSubtasks: []core.PlannedSubtask{{}}}}, Traces: []core.DayTrace{{NoteEntries: []core.TaskNoteEntry{{}}}}, Subtasks: []core.Subtask{{}}}
}
func hcPopulatePointers(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		hcPopulatePointers(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			hcPopulatePointers(v.Field(i))
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			hcPopulatePointers(v.Index(i))
		}
	}
}

// Independent test projection of current wire types. It deliberately accepts
// domain-invalid fixtures so every optional field can be costed independently.
func hcMetadataWire(v reflect.Value) canonical.Value {
	if v.Kind() == reflect.Pointer {
		return hcMetadataWire(v.Elem())
	}
	switch v.Kind() {
	case reflect.Slice:
		a := canonical.Value{Kind: canonical.Array}
		for i := 0; i < v.Len(); i++ {
			a.Array = append(a.Array, hcMetadataWire(v.Index(i)))
		}
		return a
	case reflect.Struct:
		o := canonical.Value{Kind: canonical.Object, Object: map[string]canonical.Value{}}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Kind() == reflect.Pointer && f.IsNil() {
				continue
			}
			key := v.Type().Field(i).Name
			if key == "ID" {
				key = "id"
			} else {
				key = strings.ToLower(key[:1]) + key[1:]
			}
			o.Object[key] = hcMetadataWire(f)
		}
		return o
	case reflect.String:
		x := historyString(v.String())
		if strings.HasSuffix(v.Type().Name(), "ID") && v.Type().Name() != "CancellationID" {
			return historyObject(map[string]canonical.Value{"rawValue": x})
		}
		return x
	default:
		return historyNumber(0)
	}
}
func hcCountNodes(v canonical.Value) uint64 {
	n := uint64(1)
	for key, child := range v.Object {
		_ = key
		n += 1 + hcCountNodes(child)
	}
	for _, child := range v.Array {
		n += hcCountNodes(child)
	}
	return n
}
func hcForestNodes(g core.TaskGraph) uint64 {
	v := reflect.ValueOf(g)
	var n uint64
	for i := 0; i < v.NumField(); i++ {
		n += hcCountNodes(hcMetadataWire(v.Field(i)))
	}
	return n
}
func TestHistoryCommandBudgetNodesAndEveryOptional(t *testing.T) {
	g := hcMetadataGraph()
	b, e := historyCommandGraphBudget(g, historyCommandBefore)
	must(t, e)
	if b.nodes != 151 || b.nodes != hcForestNodes(g) {
		t.Fatalf("base costs %d", b.nodes)
	}
	opts := map[string]uint64{
		"Days.LockedAt": 2, "Days.ReviewSummary": 2, "Days.ReviewUnfinishedReason": 2, "Days.ReviewTomorrowNote": 2,
		"Chains.CycleMembership": 14, "Chains.NoteEntries.DeletedAt": 2,
		"Definitions.DescriptionText": 2, "Definitions.SupersededAt": 2, "Definitions.SupersededByDefinitionID": 4,
		"Traces.PinOrder": 2, "Traces.DescriptionText": 2, "Traces.ManualProgressPercent": 2, "Traces.CompletedAt": 2, "Traces.SettledAt": 2, "Traces.DraftCancellationID": 2, "Traces.CarriedFromTraceID": 4, "Traces.ChangedToTraceID": 4, "Traces.DraftCancelledOn": 8, "Traces.NoteEntries.DeletedAt": 2,
		"Subtasks.CarriedFromSubtaskID": 4, "Subtasks.CompletedAt": 2, "Subtasks.SettledAt": 2, "Subtasks.DraftCancellationID": 2,
	}
	for path, cost := range opts {
		t.Run(path, func(t *testing.T) {
			g := hcMetadataGraph()
			v := reflect.ValueOf(&g).Elem()
			for _, p := range strings.Split(path, ".") {
				if v.Kind() == reflect.Slice {
					v = v.Index(0)
				}
				v = v.FieldByName(p)
			}
			v.Set(reflect.New(v.Type().Elem()))
			got, e := historyCommandGraphBudget(g, historyCommandBefore)
			must(t, e)
			if got.nodes != 151+cost || got.nodes != hcForestNodes(g) {
				t.Fatalf("cost mismatch %d want %d", got.nodes, 151+cost)
			}
		})
	}
	hcPopulatePointers(reflect.ValueOf(&g).Elem())
	got, e := historyCommandGraphBudget(g, historyCommandBefore)
	must(t, e)
	if got.nodes != hcForestNodes(g) {
		t.Fatal("all optionals mismatch")
	}
}
func hcBudgetError(t *testing.T, err error, phase historyCommandPhase, reason historyCommandBudgetReason) {
	t.Helper()
	var typed *historyCommandBudgetError
	if !errors.As(err, &typed) || typed.Phase != phase || typed.Reason != reason {
		t.Fatalf("got %v (%#v), want budget %d/%d", err, typed, phase, reason)
	}
}
func TestHistoryCommandBudgetArithmetic(t *testing.T) {
	for _, c := range []struct {
		sum, count, cost, limit, want uint64
		ok                            bool
	}{
		{0, 0, 17, 100, 0, true}, {99, 1, 1, 100, 100, true}, {100, 0, 17, 100, 100, true}, {100, 1, 1, 100, 0, false}, {101, 0, 1, 100, 0, false},
		{0, math.MaxUint64, 17, math.MaxUint64, 0, false}, {math.MaxUint64 - 1, 1, 1, math.MaxUint64, math.MaxUint64, true}, {math.MaxUint64, 1, 1, math.MaxUint64, 0, false},
		{0, math.MaxUint64, 1, math.MaxUint64, math.MaxUint64, true}, {0, 1, 0, 100, 0, false},
	} {
		got, ok := historyCommandAddProduct(c.sum, c.count, c.cost, c.limit)
		if got != c.want || ok != c.ok {
			t.Fatalf("product %+v => %d/%v", c, got, ok)
		}
	}
	for _, c := range []struct {
		a, b, limit, want uint64
		ok                bool
	}{{99, 0, 100, 99, true}, {99, 1, 100, 100, true}, {99, 2, 100, 0, false}, {100, 0, 100, 100, true}, {101, 0, 100, 0, false}, {math.MaxUint64, 1, math.MaxUint64, 0, false}, {0, math.MaxUint64, math.MaxUint64, math.MaxUint64, true}} {
		got, ok := historyCheckedAdd(c.a, c.b, c.limit)
		if got != c.want || ok != c.ok {
			t.Fatal("checked add mismatch", c)
		}
	}
}
func TestHistoryCommandBudgetAllStringLeaves(t *testing.T) {
	huge := strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	template := hcMetadataGraph()
	hcPopulatePointers(reflect.ValueOf(&template).Elem())
	var paths []string
	var walk func(reflect.Value, string)
	walk = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.Pointer:
			walk(v.Elem(), path)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i), path+"."+v.Type().Field(i).Name)
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), path)
			}
		case reflect.String:
			paths = append(paths, strings.TrimPrefix(path, "."))
		}
	}
	walk(reflect.ValueOf(template), "")
	if len(paths) != 34 {
		t.Fatalf("review string coverage: %d %v", len(paths), paths)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			g := hcMetadataGraph()
			hcPopulatePointers(reflect.ValueOf(&g).Elem())
			v := reflect.ValueOf(&g).Elem()
			for _, p := range strings.Split(path, ".") {
				for v.Kind() == reflect.Pointer || v.Kind() == reflect.Slice {
					if v.Kind() == reflect.Pointer {
						v = v.Elem()
					} else {
						v = v.Index(0)
					}
				}
				v = v.FieldByName(p)
			}
			if v.Kind() == reflect.Pointer {
				v = v.Elem()
			}
			v.SetString(huge)
			_, e := historyCommandGraphBudget(g, historyCommandAfter)
			hcBudgetError(t, e, historyCommandAfter, historyCommandRawBytes)
		})
	}
	// No UTF-8/content interpretation in the gate. Later schema still rejects it.
	g := hcMetadataGraph()
	g.Definitions[0].Title = "\xff"
	b, e := historyCommandGraphBudget(g, historyCommandBefore)
	must(t, e)
	if b.rawBytes != 1 {
		t.Fatal("raw byte not counted")
	}
}
func TestHistoryCommandBudgetLogicalSharingAndBoundaries(t *testing.T) {
	limit := canonical.DefaultLimits().MaxBytes
	text := strings.Repeat("a", limit/2+1)
	g := core.TaskGraph{Days: []core.Day{{ReviewSummary: &text, ReviewTomorrowNote: &text}}}
	_, e := historyCommandGraphBudget(g, historyCommandBefore)
	hcBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
	notes := []core.TaskNoteEntry{{Body: text}}
	g = core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: notes}}, Traces: []core.DayTrace{{NoteEntries: notes}}}
	_, e = historyCommandGraphBudget(g, historyCommandBefore)
	hcBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
	exact := strings.Repeat("a", limit)
	g = core.TaskGraph{Days: []core.Day{{ReviewSummary: &exact}}}
	b, e := historyCommandGraphBudget(g, historyCommandBefore)
	must(t, e)
	if b.rawBytes != uint64(limit) {
		t.Fatal("raw B rejected")
	}
	g.Days[0].ID = "x"
	_, e = historyCommandGraphBudget(g, historyCommandBefore)
	hcBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
	n := canonical.DefaultLimits().MaxNodes
	cases := map[string]core.TaskGraph{
		"top aggregate":   {Days: make([]core.Day, n/34+1), Traces: make([]core.DayTrace, n/66+1)},
		"one chain notes": {Chains: []core.TaskChain{{NoteEntries: make([]core.TaskNoteEntry, n/11+1)}}},
		"one trace notes": {Traces: []core.DayTrace{{NoteEntries: make([]core.TaskNoteEntry, n/11+1)}}},
		"planned":         {Definitions: []core.TaskDefinition{{PlannedSubtasks: make([]core.PlannedSubtask, n/17+1)}}},
	}
	shared := make([]core.TaskNoteEntry, n/22+1)
	cases["shared nested aggregate"] = core.TaskGraph{Chains: []core.TaskChain{{NoteEntries: shared}, {NoteEntries: shared}}}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			_, e := historyCommandGraphBudget(g, historyCommandAfter)
			hcBudgetError(t, e, historyCommandAfter, historyCommandShape)
		})
	}
}
func TestHistoryCommandBudgetEntryPrecedence(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	invalid := before.Clone()
	invalid.Traces[0].ID = "domain invalid"
	huge := core.TaskGraph{Days: make([]core.Day, canonical.DefaultLimits().MaxNodes/17+1)}
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: vnAt(1)}
	checks := []struct {
		before, after core.TaskGraph
		phase         historyCommandPhase
	}{{huge, invalid, historyCommandBefore}, {invalid, huge, historyCommandAfter}}
	for _, c := range checks {
		r, e := auditCompletionHistoryVNext(prior, c.before, c.after, in)
		hcBudgetError(t, e, c.phase, historyCommandShape)
		if r.valid() {
			t.Fatal("failure sealed")
		}
		r, e = auditSettlementHistoryVNext(prior, c.before, c.after, settlementHistoryInput{})
		hcBudgetError(t, e, c.phase, historyCommandShape)
		if r.valid() {
			t.Fatal("failure sealed")
		}
	}
	for _, length := range []int{0, 35, 37, canonical.DefaultLimits().MaxBytes + 1} {
		id := strings.Repeat("x", length)
		r, e := auditSettlementHistoryVNext(prior, invalid, invalid, settlementHistoryInput{EventIDs: []string{id}})
		hcBudgetError(t, e, historyCommandIDs, historyCommandIDLength)
		if r.valid() {
			t.Fatal("invalid ID sealed")
		}
		bad := in
		bad.ChainID = core.TaskChainID(id)
		r, e = auditCompletionHistoryVNext(prior, invalid, invalid, bad)
		hcBudgetError(t, e, historyCommandIDs, historyCommandIDLength)
		if r.valid() {
			t.Fatal("invalid owner sealed")
		}
		bad = in
		bad.TraceID = core.DayTraceID(id)
		_, e = auditCompletionHistoryVNext(prior, invalid, invalid, bad)
		hcBudgetError(t, e, historyCommandIDs, historyCommandIDLength)
	}
	_, e := auditSettlementHistoryVNext(prior, invalid, invalid, settlementHistoryInput{EventIDs: []string{"bad", "bad"}})
	hcBudgetError(t, e, historyCommandIDs, historyCommandIDCount)
	// Metadata pass on each graph independently, never combined across endpoints.
	text := strings.Repeat("q", canonical.DefaultLimits().MaxBytes/2+1)
	g := core.TaskGraph{Days: []core.Day{{ReviewSummary: &text}}}
	_, e = historyCommandGraphBudget(g, historyCommandBefore)
	must(t, e)
	_, e = historyCommandGraphBudget(g, historyCommandAfter)
	must(t, e)
}
func TestHistoryCommandBudgetRealDomainAndEnvelope(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 2}, vnStep{"P", 3})
	snapshot := f.snapshot(t)
	proof := vnGood(t, snapshot)
	state, _, e := v7.ToDomain(snapshot)
	must(t, e)
	budget, e := historyCommandGraphBudget(state.Tasks, historyCommandBefore)
	must(t, e)
	var actual uint64
	for _, key := range []string{"days", "chains", "definitions", "traces", "subtasks"} {
		actual += hcCountNodes(snapshot.Object[key])
	}
	if actual != budget.nodes {
		t.Fatalf("wire nodes %d != %d", actual, budget.nodes)
	}
	// Legal text with controls and UTF-8 is lower-bounded by raw byte count, not
	// six times its size. Keep graph valid and exercise actual final inspector.
	g := state.Tasks.Clone()
	g.Definitions[0].Title = "合成 \"quoted\" \n\t文本"
	extra, e := emptySupplement()
	must(t, e)
	extra.Classifications = snapshot.Object["classifications"]
	wire, e := v7.FromDomain(hcState(g), extra)
	must(t, e)
	vnGood(t, wire)
	_, e = historyCommandGraphBudget(g, historyCommandAfter)
	must(t, e)
	// Gate B success is not envelope acceptance: all nonempty structure consumes
	// extra encoded bytes. This is synthetic and never published.
	empty, e := emptySupplement()
	must(t, e)
	large := f.k.Snapshot()
	fixed, e := historyCommandGraphBudget(large, historyCommandBefore)
	must(t, e)
	old := len(large.Definitions[0].Title)
	large.Definitions[0].Title = strings.Repeat("z", canonical.DefaultLimits().MaxBytes-int(fixed.rawBytes)+old)
	b, e := historyCommandGraphBudget(large, historyCommandBefore)
	must(t, e)
	if b.rawBytes != uint64(canonical.DefaultLimits().MaxBytes) {
		t.Fatal("not at B")
	}
	largeWire, e := v7.FromDomain(hcState(large), empty)
	must(t, e)
	if _, e = inspectForwardClockOrdinaryHistoryVNext(largeWire); e == nil {
		t.Fatal("final envelope limit bypassed")
	}
	// Ensure sealed prior remains intact after unrelated capacity checks.
	copy, e := proof.snapshotCopy()
	must(t, e)
	if !historyEqual(copy, snapshot) {
		t.Fatal("proof mutated")
	}
}
func hcCallName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			return x.Name + "." + f.Sel.Name
		}
	}
	return ""
}
func TestHistoryCommandBudgetASTOrderGate(t *testing.T) {
	fs := token.NewFileSet()
	file, e := parser.ParseFile(fs, "history_command_vnext.go", nil, 0)
	must(t, e)
	funcs := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
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
	edit, e := parser.ParseFile(fs, "history_edit_command_vnext.go", nil, 0)
	must(t, e)
	for _, decl := range edit.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok {
			funcs[f.Name.Name] = f
		}
	}
	for _, name := range []string{"auditSettlementHistoryVNext", "auditCompletionHistoryVNext", "auditAbandonHistoryVNext", "auditReactivateHistoryVNext", "auditTitleHistoryVNext", "auditManualProgressHistoryVNext"} {
		f := funcs[name]
		if f == nil {
			t.Fatal("missing entry")
		}
		for i, want := range []string{"prior.valid", "historyCommandGraphBudget", "historyCommandGraphBudget", "historyCommandEventIDBudget"} {
			if name == "auditCompletionHistoryVNext" && i == 3 {
				want = "historyCommandCompletionIDBudget"
			}
			if i == 3 && name == "auditAbandonHistoryVNext" {
				want = "historyCommandAbandonIDBudget"
			}
			if i == 3 && name == "auditReactivateHistoryVNext" {
				want = "historyCommandReactivateIDBudget"
			}
			if i == 3 && name == "auditTitleHistoryVNext" {
				want = "historyCommandTitleInputBudget"
			}
			if i == 3 && name == "auditManualProgressHistoryVNext" {
				want = "historyCommandManualProgressInputBudget"
			}
			stmt, ok := f.Body.List[i].(*ast.IfStmt)
			if !ok {
				t.Fatalf("%s preflight %d is not straight-line if", name, i)
			}
			var first string
			ast.Inspect(stmt, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok && first == "" {
					first = hcCallName(c)
				}
				return true
			})
			if first != want {
				t.Fatalf("%s phase %d starts with %s not %s", name, i, first, want)
			}
			if len(stmt.Body.List) != 1 {
				t.Fatal("non-immediate gate failure")
			}
			r, ok := stmt.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(r.Results) != 2 {
				t.Fatal("gate must return zero result immediately")
			}
			zero, ok := r.Results[0].(*ast.CompositeLit)
			if !ok || len(zero.Elts) != 0 {
				t.Fatal("gate failure must return zero")
			}
		}
		var initial []string
		for _, s := range f.Body.List[:4] {
			ast.Inspect(s, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					initial = append(initial, hcCallName(c))
				}
				return true
			})
		}
		want := []string{"prior.valid", "historyVNextFail", "historyCommandGraphBudget", "historyCommandGraphBudget", "historyCommandEventIDBudget"}
		if name == "auditCompletionHistoryVNext" {
			want[4] = "historyCommandCompletionIDBudget"
		}
		if name == "auditAbandonHistoryVNext" {
			want[4] = "historyCommandAbandonIDBudget"
		}
		if name == "auditReactivateHistoryVNext" {
			want[4] = "historyCommandReactivateIDBudget"
		}
		if name == "auditTitleHistoryVNext" {
			want[4] = "historyCommandTitleInputBudget"
		}
		if name == "auditManualProgressHistoryVNext" {
			want[4] = "historyCommandManualProgressInputBudget"
		}
		if !reflect.DeepEqual(initial, want) {
			t.Fatalf("unchecked pre-gate call: %v", initial)
		}
		// Bind itself is the immediately next operation, never hidden in a gate.
		assign, ok := f.Body.List[4].(*ast.AssignStmt)
		if !ok || hcCallName(assign.Rhs[0].(*ast.CallExpr)) != "historyCommandBind" {
			t.Fatal("bind placement drift")
		}
	}
	var calls []string
	ast.Inspect(funcs["historyCommandFinish"], func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			calls = append(calls, hcCallName(c))
		}
		return true
	})
	required := []string{"appendEmptyHistoryEvents", "candidate.classificationsCopy", "v7.FromDomain", "inspectForwardClockOrdinaryHistoryVNext", "checkForwardHistorySuccessor"}
	pos := 0
	for _, c := range calls {
		if pos < len(required) && c == required[pos] {
			pos++
		}
	}
	if pos != len(required) {
		t.Fatal("finish triple-validation order drift", calls)
	}
	budget, e := parser.ParseFile(fs, "history_command_vnext_budget.go", nil, 0)
	must(t, e)
	allowed := map[string]bool{}
	for _, n := range []string{"uint64", "len", "string", "canonical.DefaultLimits", "historyCommandAddProduct", "historyCheckedAdd", "historyCommandBudgetFail", "b.nodesAdd", "b.optional", "b.raw", "historyCommandRawOptional"} {
		allowed[n] = true
	}
	ast.Inspect(budget, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && !allowed[hcCallName(c)] {
			t.Fatalf("review unexpected metadata-gate call %s", hcCallName(c))
		}
		if r, ok := n.(*ast.RangeStmt); ok && r.Value != nil {
			t.Fatal("budget must use index/header access, no whole-record copy")
		}
		return true
	})
}

func hcState(g core.TaskGraph) recurrence.State { return recurrence.State{Tasks: g} }

func TestHistoryCommandBudgetRawEntryPrecedenceAndNoMutation(t *testing.T) {
	f := vnSeed(t, vnDate)
	prior := vnGood(t, f.snapshot(t))
	normal := f.k.Snapshot()
	invalid := normal.Clone()
	invalid.Traces[0].ID = "bad"
	huge := normal.Clone()
	huge.Definitions[0].Title = strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	saved := huge.Clone()
	in := completionHistoryInput{Action: completionHistoryComplete, ChainID: normal.Chains[0].ID, TraceID: normal.Traces[0].ID, Today: vnDate, Now: vnAt(1)}
	for _, c := range []struct {
		before, after core.TaskGraph
		phase         historyCommandPhase
	}{{huge, invalid, historyCommandBefore}, {invalid, huge, historyCommandAfter}} {
		r, e := auditCompletionHistoryVNext(prior, c.before, c.after, in)
		hcBudgetError(t, e, c.phase, historyCommandRawBytes)
		hcZero(t, r, e)
		r, e = auditSettlementHistoryVNext(prior, c.before, c.after, settlementHistoryInput{EventIDs: []string{"bad"}})
		hcBudgetError(t, e, c.phase, historyCommandRawBytes)
		hcZero(t, r, e)
	}
	if !reflect.DeepEqual(huge, saved) {
		t.Fatal("budget failure mutated caller")
	}
	// Every string alias and shared optional contributes its logical raw length.
	all := hcMetadataGraph()
	hcPopulatePointers(reflect.ValueOf(&all).Elem())
	var expected uint64
	var fill func(reflect.Value)
	fill = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			fill(v.Elem())
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				fill(v.Index(i))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				fill(v.Field(i))
			}
		case reflect.String:
			v.SetString("abc")
			expected += 3
		}
	}
	fill(reflect.ValueOf(&all).Elem())
	stats, e := historyCommandGraphBudget(all, historyCommandBefore)
	must(t, e)
	if stats.rawBytes != expected || expected != 102 {
		t.Fatal("logical raw accounting", stats.rawBytes, expected)
	}
}
func TestHistoryCommandBudgetNearEnvelopePositiveAndExactNodeBoundary(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	g.Definitions[0].Title = strings.Repeat("界", (canonical.DefaultLimits().MaxBytes-32768)/3)
	// This genuine complete snapshot fits its envelope and must not be rejected
	// using the 6x escaping upper bound. No domain or JSON fixture is persisted.
	wire := hcWire(t, g, canonical.Value{})
	vnGood(t, wire)
	state, _, e := v7.ToDomain(wire)
	must(t, e)
	for _, phase := range []historyCommandPhase{historyCommandBefore, historyCommandAfter} {
		stats, e := historyCommandGraphBudget(state.Tasks, phase)
		must(t, e)
		if stats.rawBytes < uint64(canonical.DefaultLimits().MaxBytes-65536) {
			t.Fatal("fixture not near limit")
		}
	}
	n := uint64(canonical.DefaultLimits().MaxNodes)
	// 5 + 17*58823 = 999996. Two optional scalar fields reach exactly N.
	boundary := core.TaskGraph{Days: make([]core.Day, (n-5)/17)}
	boundary.Days[0].LockedAt = vnPtr(vnAt(0))
	boundary.Days[0].ReviewSummary = vnPtr("")
	b, e := historyCommandGraphBudget(boundary, historyCommandBefore)
	must(t, e)
	if b.nodes != n {
		t.Fatalf("node boundary %d", b.nodes)
	}
	boundary.Days[0].ReviewTomorrowNote = vnPtr("")
	_, e = historyCommandGraphBudget(boundary, historyCommandBefore)
	hcBudgetError(t, e, historyCommandBefore, historyCommandShape)
}
