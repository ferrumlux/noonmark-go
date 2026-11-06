// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"testing"
)

func TestHistorySubtaskInputBudgetAST(t *testing.T) {
	file, e := parser.ParseFile(token.NewFileSet(), "history_subtask_command_vnext_budget.go", nil, 0)
	must(t, e)
	expected := map[string][]string{
		"historyCommandPlannedSubtaskInputBudget": {"Kind", "ChainID", "ExpectedDefinitionID", "SubtaskID", "LineageID", "Title"},
		"historyCommandActualSubtaskInputBudget":  {"Kind", "ChainID", "TraceID", "SubtaskID", "LineageID", "Title", "CancellationID"},
	}
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fields, ok := expected[f.Name.Name]
		if !ok {
			t.Fatalf("unexpected budget helper %s", f.Name.Name)
		}
		seen := map[string]int{}
		ast.Inspect(f.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				name := hcCallName(n)
				if name != "string" && name != "b.raw" && name != "historyCommandBudgetFail" {
					t.Fatalf("nonmetadata call %s", name)
				}
				if name == "b.raw" {
					if len(n.Args) != 1 {
						t.Fatal("raw arity")
					}
					arg := n.Args[0]
					if c, ok := arg.(*ast.CallExpr); ok {
						if hcCallName(c) != "string" || len(c.Args) != 1 {
							t.Fatal("unexpected raw conversion")
						}
						arg = c.Args[0]
					}
					sel, ok := arg.(*ast.SelectorExpr)
					if !ok {
						t.Fatal("raw must use direct input field")
					}
					owner, ok := sel.X.(*ast.Ident)
					if !ok || owner.Name != "in" {
						t.Fatal("raw source")
					}
					seen[sel.Sel.Name]++
				}
			case *ast.RangeStmt, *ast.ForStmt, *ast.GoStmt, *ast.DeferStmt, *ast.IndexExpr, *ast.SliceExpr, *ast.FuncLit:
				t.Fatal("budget cannot scan, allocate, loop or defer")
			}
			return true
		})
		want := map[string]int{}
		for _, field := range fields {
			want[field] = 1
		}
		if !reflect.DeepEqual(seen, want) {
			t.Fatalf("string coverage %s: %v", f.Name.Name, seen)
		}
		delete(expected, f.Name.Name)
	}
	if len(expected) != 0 {
		t.Fatal("missing budget helpers", expected)
	}
}
func TestHistorySubtaskReplayAST(t *testing.T) {
	file, e := parser.ParseFile(token.NewFileSet(), "history_subtask_command_vnext.go", nil, 0)
	must(t, e)
	for _, imp := range file.Imports {
		name, e := strconv.Unquote(imp.Path.Value)
		must(t, e)
		switch name {
		case "strings", "github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical", "github.com/ferrumlux/noonmark/ports/mygo/domain/core", "github.com/ferrumlux/noonmark/ports/mygo/domain/textcanon":
		default:
			t.Fatalf("unexpected wiring/allocator/import dependency %s", name)
		}
	}
	wanted := map[string]string{"auditPlannedSubtaskHistoryVNext": "EditPlannedSubtask", "auditActualSubtaskHistoryVNext": "EditSubtask"}
	proofCount := 0
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		method, isAudit := wanted[f.Name.Name]
		coreCalls, finishes, proofs := 0, 0, 0
		ast.Inspect(f.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "EditPlannedSubtask" || sel.Sel.Name == "EditSubtask") {
					if !isAudit || sel.Sel.Name != method {
						t.Fatal("core replay in wrong entry")
					}
					coreCalls++
				}
				if hcCallName(n) == "historyCommandFinish" {
					finishes++
					if !isAudit || len(n.Args) != 4 {
						t.Fatal("finish outside closed audit")
					}
					last, ok := n.Args[3].(*ast.Ident)
					if !ok || last.Name != "nil" {
						t.Fatal("subtask must have nil captures")
					}
				}
			case *ast.CompositeLit:
				if name, ok := n.Type.(*ast.Ident); ok && name.Name == "historyCommandProof" {
					proofs++
					proofCount++
					if !isAudit {
						t.Fatal("proof outside success entry")
					}
				}
			}
			return true
		})
		if isAudit {
			last, ok := f.Body.List[len(f.Body.List)-1].(*ast.ReturnStmt)
			if !ok || len(last.Results) != 2 {
				t.Fatal("success must be final return")
			}
			successProofs := 0
			ast.Inspect(last, func(n ast.Node) bool {
				if lit, ok := n.(*ast.CompositeLit); ok {
					if id, ok := lit.Type.(*ast.Ident); ok && id.Name == "historyCommandProof" {
						successProofs++
					}
				}
				return true
			})
			if successProofs != 1 {
				t.Fatal("proof must be constructed only at final success")
			}
			ast.Inspect(f.Body, func(n ast.Node) bool {
				r, ok := n.(*ast.ReturnStmt)
				if !ok || r == last {
					return true
				}
				if len(r.Results) != 2 {
					t.Fatal("failure return arity")
				}
				zero, ok := r.Results[0].(*ast.CompositeLit)
				if !ok || len(zero.Elts) != 0 {
					t.Fatal("failure must return zero proof")
				}
				return true
			})
			if coreCalls != 1 || finishes != 1 || proofs != 1 {
				t.Fatalf("%s replay=%d finish=%d proof=%d", f.Name.Name, coreCalls, finishes, proofs)
			}
			delete(wanted, f.Name.Name)
		}
	}
	if len(wanted) != 0 || proofCount != 2 {
		t.Fatal("missing closed entries")
	}
}

func TestHistorySubtaskInputStringSchema(t *testing.T) {
	for _, tc := range []struct {
		input   any
		strings []string
	}{
		{plannedSubtaskHistoryInput{}, []string{"Kind", "ChainID", "ExpectedDefinitionID", "SubtaskID", "LineageID", "Title"}},
		{actualSubtaskHistoryInput{}, []string{"Kind", "ChainID", "TraceID", "SubtaskID", "LineageID", "Title", "CancellationID"}},
	} {
		typ := reflect.TypeOf(tc.input)
		var fields []string
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.String {
				fields = append(fields, f.Name)
			}
			switch f.Type.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface, reflect.Func:
				t.Fatal("mutable/open request leaf", f.Name)
			}
		}
		if !reflect.DeepEqual(fields, tc.strings) {
			t.Fatalf("new string field must update budget manifest: %v", fields)
		}
	}
}
