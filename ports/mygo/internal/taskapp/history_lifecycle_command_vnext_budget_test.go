// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestHistoryLifecycleIDMetadata(t *testing.T) {
	// Metadata deliberately accepts non-UUID content of the right fixed length.
	id := strings.Repeat("!", 36)
	must(t, historyCommandAbandonIDBudget(abandonHistoryInput{ChainID: core.TaskChainID(id), TraceID: core.DayTraceID(id), EventID: id}))
	for _, ids := range [][]string{nil, {}, {id}} {
		must(t, historyCommandReactivateIDBudget(reactivateHistoryInput{ChainID: core.TaskChainID(id), TraceID: core.DayTraceID(id), EventIDs: ids}))
	}
	for _, size := range []int{0, 35, 37, 1 << 20} {
		bad := strings.Repeat("x", size)
		for field := 0; field < 3; field++ {
			a := abandonHistoryInput{ChainID: core.TaskChainID(id), TraceID: core.DayTraceID(id), EventID: id}
			r := reactivateHistoryInput{ChainID: core.TaskChainID(id), TraceID: core.DayTraceID(id), EventIDs: []string{id}}
			switch field {
			case 0:
				a.ChainID, r.ChainID = core.TaskChainID(bad), core.TaskChainID(bad)
			case 1:
				a.TraceID, r.TraceID = core.DayTraceID(bad), core.DayTraceID(bad)
			case 2:
				a.EventID, r.EventIDs[0] = bad, bad
			}
			hcBudgetError(t, historyCommandAbandonIDBudget(a), historyCommandIDs, historyCommandIDLength)
			hcBudgetError(t, historyCommandReactivateIDBudget(r), historyCommandIDs, historyCommandIDLength)
		}
	}
	// Count wins over every malformed scalar and element, without a UUID scan.
	hcBudgetError(t, historyCommandReactivateIDBudget(reactivateHistoryInput{EventIDs: []string{"bad", "bad"}}), historyCommandIDs, historyCommandIDCount)
}

func TestHistoryLifecycleBudgetPrecedence(t *testing.T) {
	f := vnSeed(t, vnDate)
	prior := vnGood(t, f.snapshot(t))
	normal := f.k.Snapshot()
	invalid := normal.Clone()
	invalid.Traces[0].ID = "invalid domain"
	raw := normal.Clone()
	raw.Definitions[0].Title = strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	shape := normal.Clone()
	shape.Days = make([]core.Day, canonical.DefaultLimits().MaxNodes/17+1)
	for _, oversized := range []struct {
		graph  core.TaskGraph
		reason historyCommandBudgetReason
	}{{raw, historyCommandRawBytes}, {shape, historyCommandShape}} {
		for _, c := range []struct {
			before, after core.TaskGraph
			phase         historyCommandPhase
		}{{oversized.graph, invalid, historyCommandBefore}, {invalid, oversized.graph, historyCommandAfter}} {
			beforeCopy, afterCopy := c.before.Clone(), c.after.Clone()
			r, e := auditAbandonHistoryVNext(prior, c.before, c.after, abandonHistoryInput{})
			hcBudgetError(t, e, c.phase, oversized.reason)
			hcZero(t, r, e)
			r, e = auditReactivateHistoryVNext(prior, c.before, c.after, reactivateHistoryInput{EventIDs: []string{"bad", "bad"}})
			hcBudgetError(t, e, c.phase, oversized.reason)
			hcZero(t, r, e)
			if !reflect.DeepEqual(c.before, beforeCopy) || !reflect.DeepEqual(c.after, afterCopy) {
				t.Fatal("budget rejection mutated input")
			}
		}
	}
	// Both graph gates pass; caller ID metadata still precedes domain validation.
	r, e := auditAbandonHistoryVNext(prior, invalid, invalid, abandonHistoryInput{})
	hcBudgetError(t, e, historyCommandIDs, historyCommandIDLength)
	hcZero(t, r, e)
	r, e = auditReactivateHistoryVNext(prior, invalid, invalid, reactivateHistoryInput{EventIDs: []string{"bad", "bad"}})
	hcBudgetError(t, e, historyCommandIDs, historyCommandIDCount)
	hcZero(t, r, e)
	copy, e := prior.snapshotCopy()
	must(t, e)
	if !historyEqual(copy, f.snapshot(t)) {
		t.Fatal("prior changed")
	}
}

func TestHistoryLifecycleIDMetadataAST(t *testing.T) {
	file, e := parser.ParseFile(token.NewFileSet(), "history_lifecycle_command_vnext.go", nil, 0)
	must(t, e)
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok || (f.Name.Name != "historyCommandAbandonIDBudget" && f.Name.Name != "historyCommandReactivateIDBudget") {
			continue
		}
		seen[f.Name.Name] = true
		ast.Inspect(f.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				name := hcCallName(n)
				if name != "len" && name != "historyCommandBudgetFail" {
					t.Fatalf("non-metadata ID gate call %s", name)
				}
			case *ast.RangeStmt, *ast.ForStmt, *ast.GoStmt, *ast.DeferStmt:
				t.Fatal("ID gate must be constant-time straight-line metadata")
			}
			return true
		})
		if f.Name.Name == "historyCommandReactivateIDBudget" {
			first, ok := f.Body.List[0].(*ast.IfStmt)
			if !ok || first.Init != nil {
				t.Fatal("count must be the first gate")
			}
			cond, ok := first.Cond.(*ast.BinaryExpr)
			if !ok || cond.Op != token.GTR {
				t.Fatal("count must reject lengths greater than one")
			}
			call, ok := cond.X.(*ast.CallExpr)
			if !ok || hcCallName(call) != "len" || len(call.Args) != 1 {
				t.Fatal("count must read only slice length")
			}
			arg, ok := call.Args[0].(*ast.SelectorExpr)
			if !ok || arg.Sel.Name != "EventIDs" {
				t.Fatal("wrong count input")
			}
			one, ok := cond.Y.(*ast.BasicLit)
			if !ok || one.Value != "1" {
				t.Fatal("wrong ID count limit")
			}
			ast.Inspect(first, func(n ast.Node) bool {
				if _, ok := n.(*ast.IndexExpr); ok {
					t.Fatal("count gate accesses an element")
				}
				return true
			})
			if len(first.Body.List) != 1 {
				t.Fatal("count failure must immediately return")
			}
			if _, ok := first.Body.List[0].(*ast.ReturnStmt); !ok {
				t.Fatal("count failure does not return")
			}
		}
	}
	if len(seen) != 2 {
		t.Fatal("missing lifecycle ID gates")
	}
}
