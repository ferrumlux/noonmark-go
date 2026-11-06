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

func TestHistoryRemainingBudgetAST(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "history_remaining_command_vnext_budget.go", nil, 0)
	must(t, err)
	fixed := map[string][]string{
		"historyCommandReturnInputBudget":   {"ChainID", "TraceID", "NewDefinitionID", "CancellationID", "EventID"},
		"historyCommandDeferInputBudget":    {"ChainID", "TraceID", "NewTraceID", "DayID", "EventID"},
		"historyCommandContinueInputBudget": {"ChainID", "TraceID", "NewTraceID", "DayID"},
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		seen := []string{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				name := hcCallName(n)
				switch name {
				case "len", "uint64", "string", "canonical.DefaultLimits", "b.nodesAdd", "b.raw", "historyCommandBudgetFail", "historyRemainingMapBudget":
				default:
					t.Fatalf("nonmetadata budget call %s", name)
				}
				if name == "b.raw" && fn.Name.Name != "historyRemainingMapBudget" {
					c, ok := n.Args[0].(*ast.CallExpr)
					if !ok || hcCallName(c) != "string" {
						t.Fatal("not scalar string header")
					}
					s, ok := c.Args[0].(*ast.SelectorExpr)
					if !ok {
						t.Fatal("not input scalar")
					}
					if x, ok := s.X.(*ast.Ident); !ok || x.Name != "in" {
						t.Fatal("not direct input")
					}
					seen = append(seen, s.Sel.Name)
				}
			case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit, *ast.ForStmt:
				t.Fatal("unexpected executable budget logic")
			case *ast.RangeStmt:
				if fn.Name.Name != "historyRemainingMapBudget" {
					t.Fatal("unexpected scalar traversal")
				}
				x, ok := n.X.(*ast.Ident)
				if !ok || x.Name != "ids" {
					t.Fatal("non-map traversal")
				}
			}
			return true
		})
		if fn.Name.Name == "historyRemainingMapBudget" {
			if len(fn.Body.List) != 5 {
				t.Fatal("map budget order changed")
			}
			guard, ok := fn.Body.List[2].(*ast.IfStmt)
			if !ok {
				t.Fatal("map count guard missing")
			}
			var calls []string
			ast.Inspect(guard.Cond, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					calls = append(calls, hcCallName(c))
				}
				return true
			})
			if !reflect.DeepEqual(calls, []string{"uint64", "len", "uint64", "canonical.DefaultLimits", "b.nodesAdd"}) {
				t.Fatal("count and checked multiply guard", calls)
			}
			if _, ok := fn.Body.List[3].(*ast.RangeStmt); !ok {
				t.Fatal("range must follow count check")
			}
		} else if !reflect.DeepEqual(seen, fixed[fn.Name.Name]) {
			t.Fatalf("raw fields %s: %v", fn.Name.Name, seen)
		}
	}
}

func TestHistoryRemainingClosedReplayAST(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "history_remaining_command_vnext.go", nil, 0)
	must(t, err)
	for _, imp := range f.Imports {
		s, e := strconv.Unquote(imp.Path.Value)
		must(t, e)
		if s != "github.com/ferrumlux/noonmark/ports/mygo/domain/core" {
			t.Fatal("unexpected dependency", s)
		}
	}
	want := map[string]string{"auditReturnHistoryVNext": "ReturnToPool", "auditDeferHistoryVNext": "DeferCurrentTrace", "auditContinueHistoryVNext": "ContinueUnfinishedTrace"}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		method, isAudit := want[fn.Name.Name]
		calls, finish, proof := 0, 0, 0
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if s, ok := n.Fun.(*ast.SelectorExpr); ok {
					switch s.Sel.Name {
					case "ReturnToPool", "DeferCurrentTrace", "ContinueUnfinishedTrace":
						if !isAudit || s.Sel.Name != method {
							t.Fatal("incorrect replay")
						}
						calls++
					}
				}
				if hcCallName(n) == "historyCommandFinish" {
					finish++
					if fn.Name.Name == "auditContinueHistoryVNext" {
						if x, ok := n.Args[3].(*ast.Ident); !ok || x.Name != "nil" {
							t.Fatal("Continue capture")
						}
					}
				}
			case *ast.CompositeLit:
				if x, ok := n.Type.(*ast.Ident); ok && x.Name == "historyCommandProof" {
					proof++
					if !isAudit {
						t.Fatal("unsealed constructor")
					}
				}
			}
			return true
		})
		if isAudit && (calls != 1 || finish != 1 || proof != 1) {
			t.Fatal("audit topology", fn.Name.Name, calls, finish, proof)
		}
	}
}
