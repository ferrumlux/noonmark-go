// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Exact file hashes are independently bound by the taskapp freeze gate. This
// check also makes the ownership-critical lock/call structure explicit.
func TestDayReadScopeLockedSelection(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "kernel.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "DayTodo" {
			body = fn.Body
		}
	}
	if body == nil || len(body.List) != 3 {
		t.Fatal("DayTodo must select, sort, return")
	}
	assign, ok := body.List[0].(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		t.Fatal("missing protected selection")
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		t.Fatal("selection is not invoked")
	}
	local, ok := call.Fun.(*ast.FuncLit)
	if !ok {
		t.Fatal("selection must use scoped function")
	}
	name := func(c *ast.CallExpr) string {
		if id, ok := c.Fun.(*ast.Ident); ok {
			return id.Name
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
			return sel.Sel.Name
		}
		return ""
	}
	counts := map[string]int{}
	ranges := 0
	ast.Inspect(local.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			counts[name(c)]++
		}
		if r, ok := n.(*ast.RangeStmt); ok {
			sel, ok := r.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Traces" {
				t.Fatal("non-trace selection range")
			}
			graph, ok := sel.X.(*ast.SelectorExpr)
			if !ok || graph.Sel.Name != "graph" {
				t.Fatal("selection does not read graph")
			}
			ranges++
		}
		return true
	})
	if counts["RLock"] != 1 || counts["RUnlock"] != 1 || counts["cloneDayTrace"] != 1 || counts["Snapshot"] != 0 || counts["Clone"] != 0 || ranges != 2 {
		t.Fatalf("selection scope drift: %v ranges=%d", counts, ranges)
	}
	lock, ok := local.Body.List[0].(*ast.ExprStmt)
	if !ok {
		t.Fatal("lock must be first")
	}
	lc, ok := lock.X.(*ast.CallExpr)
	if !ok || name(lc) != "RLock" {
		t.Fatal("lock must be first")
	}
	unlock, ok := local.Body.List[1].(*ast.DeferStmt)
	if !ok || name(unlock.Call) != "RUnlock" {
		t.Fatal("unlock must be deferred immediately")
	}
	sortStmt, ok := body.List[1].(*ast.ExprStmt)
	if !ok {
		t.Fatal("sort outside lock missing")
	}
	sc, ok := sortStmt.X.(*ast.CallExpr)
	if !ok || name(sc) != "Slice" {
		t.Fatal("sort outside lock missing")
	}
	ast.Inspect(sortStmt, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "graph" {
			t.Fatal("sort accesses kernel graph")
		}
		return true
	})
}
