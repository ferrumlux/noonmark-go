// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Exactly two source-reviewed narrow-read deltas. Prior overlays remain unchanged.
var dayReadFrozenChanges = map[string][2]string{
	"ports/mygo/domain-core/core/kernel.go": {"9998149755425448eca175ec9947f266d2c7ecaed5af43e7096392583050c32c", "009a5b4a659fbf008224be72966a3b28dc0dbab1fd1fc200152bbcb0c92989d6"},
	"ports/mygo/domain-core/core/clone.go":  {"4876f2f2b8c0a1811f0789da55451ec70928257fd7ad64c2d2f6972ec8180916", "121b2254886f84507929e5f99936b638ddf2ab5524a635cf7fe5383afc9ff76d"},
}

func dayReadFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	rows := strings.Split(strings.TrimSpace(overlay), "\n")
	if len(rows) != 2 {
		return nil, fmt.Errorf("day read overlay must contain exactly two reviewed rows")
	}
	seen := map[string]bool{}
	for _, row := range rows {
		p := strings.Split(row, "\t")
		if len(p) != 3 {
			return nil, fmt.Errorf("invalid day read overlay shape")
		}
		binding, ok := dayReadFrozenChanges[p[2]]
		if !ok || seen[p[2]] || p[0] != binding[0] || p[1] != binding[1] || expected[p[2]] != binding[0] {
			return nil, fmt.Errorf("unapproved day read overlay binding")
		}
		seen[p[2]] = true
	}
	if len(seen) != len(dayReadFrozenChanges) {
		return nil, fmt.Errorf("incomplete day read overlay")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	for path, binding := range dayReadFrozenChanges {
		next[path] = binding[1]
	}
	return next, nil
}

func TestDayReadFrozenOverlayRejectsUnapprovedChanges(t *testing.T) {
	base, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, e)
	service, e := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, e)
	expected, e := outcomeFrozenOverlay(string(base), string(service))
	must(t, e)
	day, e := os.ReadFile("../../docs/tasks/align-day-detail-header/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	expected, e = dayDetailFrozenOverlay(expected, string(day))
	must(t, e)
	snapshot, e := os.ReadFile("../../docs/tasks/diagnose-root-test-timeout/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	expected, e = daySnapshotFrozenOverlay(expected, string(snapshot))
	must(t, e)
	overlay, e := os.ReadFile("../../docs/tasks/optimize-day-todo-read/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	if _, e := dayReadFrozenOverlay(nil, string(overlay)); e == nil {
		t.Fatal("absent baseline accepted")
	}
	clone := func(in map[string]string) map[string]string {
		out := make(map[string]string, len(in))
		for p, h := range in {
			out[p] = h
		}
		return out
	}
	before := clone(expected)
	next, e := dayReadFrozenOverlay(expected, string(overlay))
	must(t, e)
	if !reflect.DeepEqual(before, expected) {
		t.Fatal("input manifest mutated")
	}
	if len(next) != len(expected) {
		t.Fatal("overlay lost/added paths")
	}
	for p, h := range expected {
		if binding, ok := dayReadFrozenChanges[p]; ok {
			if next[p] != binding[1] {
				t.Fatal("reviewed path not advanced", p)
			}
		} else if next[p] != h {
			t.Fatal("unapproved path advanced", p)
		}
	}
	rows := strings.Split(strings.TrimSpace(string(overlay)), "\n")
	variants := map[string]string{"missing all": "", "whitespace only": " \n", "bad shape": "broken", "extra": string(overlay) + rows[0]}
	for i, row := range rows {
		p := strings.Split(row, "\t")
		other := rows[1-i]
		variants[fmt.Sprintf("missing row %d", i)] = other
		variants[fmt.Sprintf("duplicate row %d", i)] = row + "\n" + row
		changes := map[string]string{
			"extra column":              row + "\textra",
			"old hash mismatch":         strings.Replace(row, p[0], strings.Repeat("a", 64), 1),
			"valid unapproved new hash": strings.Replace(row, p[1], strings.Repeat("b", 64), 1),
			"malformed new hash":        strings.Replace(row, p[1], "bad", 1),
			"no advance":                strings.Replace(row, p[1], p[0], 1),
			"alternate path":            strings.Replace(row, p[2], "ports/mygo/runtime_title.go", 1),
			"traversal path":            strings.Replace(row, p[2], "ports/mygo/../mygo/"+strings.TrimPrefix(p[2], "ports/mygo/"), 1),
		}
		for name, v := range changes {
			variants[fmt.Sprintf("row %d %s", i, name)] = v + "\n" + other
		}
		for _, state := range []string{"missing", "wrong", "already advanced"} {
			bad := clone(expected)
			switch state {
			case "missing":
				delete(bad, p[2])
			case "wrong":
				bad[p[2]] = strings.Repeat("c", 64)
			case "already advanced":
				bad[p[2]] = p[1]
			}
			t.Run(fmt.Sprintf("row %d baseline %s", i, state), func(t *testing.T) {
				badBefore := clone(bad)
				if _, e := dayReadFrozenOverlay(bad, string(overlay)); e == nil {
					t.Fatal("unapproved baseline accepted")
				}
				if !reflect.DeepEqual(badBefore, bad) {
					t.Fatal("failure mutated bad baseline")
				}
			})
		}
	}
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			if _, e := dayReadFrozenOverlay(expected, v); e == nil {
				t.Fatal("unapproved overlay accepted")
			}
			if !reflect.DeepEqual(before, expected) {
				t.Fatal("failure mutated input")
			}
		})
	}
}

// The phase helper authorizes only its two deltas; the real consumer must execute
// the entire historical overlay chain in order and inspect production hashes.
func TestDayReadFrozenConsumerChain(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "history_edit_ast_test.go", nil, 0)
	must(t, err)
	var calls []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "TestHistoryEditFrozenProduction" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := c.Fun.(*ast.Ident); ok && strings.HasSuffix(id.Name, "FrozenOverlay") {
				calls = append(calls, id.Name)
			}
			return true
		})
	}
	want := []string{"outcomeFrozenOverlay", "dayDetailFrozenOverlay", "daySnapshotFrozenOverlay", "dayReadFrozenOverlay", "nativeWindowFrozenOverlay", "quoteFastFrozenOverlay"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("consumer overlay chain = %v", calls)
	}
}
