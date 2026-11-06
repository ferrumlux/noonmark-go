// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryEditASTSealedAndBudget(t *testing.T) {
	fs := token.NewFileSet()
	file, e := parser.ParseFile(fs, "history_edit_command_vnext.go", nil, 0)
	must(t, e)
	coreCalls := map[string]int{}
	constructors := map[string]int{}
	for _, decl := range file.Decls {
		f, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		finish := 0
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				name := hcCallName(c)
				switch name {
				case "in.Now.Finite":
					t.Fatal("unconditional clock check")
				case "k.RenameTaskTitle", "k.SaveTaskTitleInput", "k.SetManualProgress":
					coreCalls[name]++
				case "historyCommandFinish":
					finish++
					if len(c.Args) != 4 {
						t.Fatal("finish arity")
					}
					v, ok := c.Args[3].(*ast.Ident)
					if !ok || v.Name != "nil" {
						t.Fatal("edit captures")
					}
				}
			}
			if v, ok := n.(*ast.CompositeLit); ok {
				if id, ok := v.Type.(*ast.Ident); ok && id.Name == "historyCommandProof" {
					constructors[f.Name.Name]++
				}
			}
			if id, ok := n.(*ast.Ident); ok {
				switch id.Name {
				case "Service", "Store", "allocateMutationClock", "collectHistoryClocks":
					t.Fatal("wiring escaped pure boundary")
				}
			}
			return true
		})
		if f.Name.Name == "auditTitleHistoryVNext" || f.Name.Name == "auditManualProgressHistoryVNext" {
			if finish != 1 {
				t.Fatal("finish missing")
			}
		}
	}
	for _, name := range []string{"k.RenameTaskTitle", "k.SaveTaskTitleInput", "k.SetManualProgress"} {
		if coreCalls[name] != 1 {
			t.Fatal("core call drift", name)
		}
	}
	if len(constructors) != 2 || constructors["auditTitleHistoryVNext"] != 1 || constructors["auditManualProgressHistoryVNext"] != 1 {
		t.Fatal("constructor drift")
	}
	b, e := parser.ParseFile(fs, "history_edit_command_vnext_budget.go", nil, 0)
	must(t, e)
	allowed := map[string]bool{"len": true, "string": true, "b.raw": true, "historyCommandBudgetFail": true}
	ast.Inspect(b, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && !allowed[hcCallName(c)] {
			t.Fatal("non-metadata budget call", hcCallName(c))
		}
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt:
			t.Fatal("input budget should be fixed-size")
		}
		return true
	})
}

func TestHistoryEditFrozenProduction(t *testing.T) {
	rows, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, e)
	overlay, e := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, e)
	expected, e := outcomeFrozenOverlay(string(rows), string(overlay))
	must(t, e)
	for _, line := range strings.Split(strings.TrimSpace(string(rows)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			t.Fatal("manifest shape")
		}
		b, e := os.ReadFile(filepath.Join("../../../..", parts[1]))
		must(t, e)
		if fmt.Sprintf("%x", sha256.Sum256(b)) != expected[parts[1]] {
			t.Fatal("frozen production changed", parts[1])
		}
	}
	b, e := os.ReadFile("history_command_vnext.go")
	must(t, e)
	start := strings.Index(string(b), "func auditSettlementHistoryVNext")
	if start < 0 {
		t.Fatal("missing old audit")
	}
	hash, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-COMMAND-SUFFIX.sha256")
	must(t, e)
	if fmt.Sprintf("%x", sha256.Sum256(b[start:])) != strings.TrimSpace(string(hash)) {
		t.Fatal("old audit/bind/match/finish changed")
	}
}
