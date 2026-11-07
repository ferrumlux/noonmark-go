// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Exact, test-only authorization for the single reviewed ordinary entry deltas.
// The complete incoming frozen set is bound, not only the single changed entries.
const ordinaryEntryFrozenBeforeSHA256 = "aa2c9bb51c38bd1663ec58b25795e80cab078273e4fd173a24e5d3741b4aea31"
const ordinaryEntryFrozenRows = `23bd5f30645a75fb9f835307f03e0b116ad8d3f995ff108c9081a1cdfc629849	2ff3666a303fa97c6d91861523af2f896152d4a57f99dd9000cc227fbb63de2a	ports/mygo/main.go
`

func ordinaryEntryFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	if overlay != ordinaryEntryFrozenRows {
		return nil, fmt.Errorf("unapproved ordinary entry overlay")
	}
	if len(expected) != 73 {
		return nil, fmt.Errorf("unapproved ordinary entry frozen path count")
	}
	paths := make([]string, 0, len(expected))
	for path, hash := range expected {
		if strings.ContainsAny(path, "\t\r\n") || len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return nil, fmt.Errorf("invalid ordinary entry frozen entry")
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var baseline strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&baseline, "%s\t%s\n", expected[path], path)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(baseline.String()))) != ordinaryEntryFrozenBeforeSHA256 {
		return nil, fmt.Errorf("unapproved ordinary entry incoming frozen scope")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	for _, row := range strings.Split(strings.TrimSuffix(ordinaryEntryFrozenRows, "\n"), "\n") {
		fields := strings.Split(row, "\t")
		next[fields[2]] = fields[1]
	}
	return next, nil
}

func ordinaryEntryFrozenBaseline(t *testing.T) map[string]string {
	t.Helper()
	base, err := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, err)
	first, err := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, err)
	expected, err := outcomeFrozenOverlay(string(base), string(first))
	must(t, err)
	for _, step := range []struct {
		path  string
		apply func(map[string]string, string) (map[string]string, error)
	}{
		{"align-day-detail-header", dayDetailFrozenOverlay},
		{"diagnose-root-test-timeout", daySnapshotFrozenOverlay},
		{"optimize-day-todo-read", dayReadFrozenOverlay},
		{"verify-native-window", nativeWindowFrozenOverlay},
		{"optimize-canonical-quote", quoteFastFrozenOverlay},
		{"integrate-ordinary-service", ordinaryServiceFrozenOverlay},
		{"integrate-category-ui", categoryUIFrozenOverlay},
	} {
		rows, e := os.ReadFile("../../docs/tasks/" + step.path + "/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
		must(t, e)
		expected, e = step.apply(expected, string(rows))
		must(t, e)
	}
	return expected
}

func TestOrdinaryEntryFrozenScopeRejectsUnapprovedChanges(t *testing.T) {
	expected := ordinaryEntryFrozenBaseline(t)
	b, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, err)
	clone := func(in map[string]string) map[string]string {
		if in == nil {
			return nil
		}
		out := make(map[string]string, len(in))
		for p, h := range in {
			out[p] = h
		}
		return out
	}
	before := clone(expected)
	next, err := ordinaryEntryFrozenOverlay(expected, string(b))
	must(t, err)
	if !reflect.DeepEqual(before, expected) || len(next) != len(expected) {
		t.Fatal("overlay mutated input or path set")
	}
	// The published full before/after manifests are exact views of the real chain.
	for _, phase := range []struct {
		name    string
		entries map[string]string
	}{{"BEFORE", expected}, {"AFTER", next}} {
		paths := make([]string, 0, len(phase.entries))
		for path := range phase.entries {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		var content strings.Builder
		for _, path := range paths {
			fmt.Fprintf(&content, "%s\t%s\n", phase.entries[path], path)
		}
		published, e := os.ReadFile("../../docs/tasks/connect-ordinary-entry/FROZEN-" + phase.name + "-v1.tsv")
		must(t, e)
		if string(published) != content.String() {
			t.Fatal("published full manifest drift", phase.name)
		}
	}
	changes := map[string][2]string{}
	rows := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(rows) != 1 {
		t.Fatal("scope must contain one delta")
	}
	for _, row := range rows {
		p := strings.Split(row, "\t")
		changes[p[2]] = [2]string{p[0], p[1]}
	}
	for p, h := range expected {
		want := h
		if binding, ok := changes[p]; ok {
			if h != binding[0] {
				t.Fatal("old binding", p)
			}
			want = binding[1]
		}
		if next[p] != want {
			t.Fatal("unauthorized delta", p)
		}
	}
	variants := map[string]string{
		"empty": "", "whitespace": " \n", "duplicate all": string(b) + string(b),
		"extra row": string(b) + "unapproved\n", "missing LF": strings.TrimSuffix(string(b), "\n"),
		"CRLF": strings.ReplaceAll(string(b), "\n", "\r\n"),
	}
	for i, row := range rows {
		p := strings.Split(row, "\t")
		variants[fmt.Sprintf("missing %d", i)] = strings.Replace(string(b), row+"\n", "", 1)
		variants[fmt.Sprintf("duplicate %d", i)] = string(b) + row + "\n"
		for name, changed := range map[string]string{
			"old hash":       strings.Repeat("a", 64) + "\t" + p[1] + "\t" + p[2],
			"new hash":       p[0] + "\t" + strings.Repeat("b", 64) + "\t" + p[2],
			"malformed hash": p[0] + "\tbad\t" + p[2],
			"no advance":     p[0] + "\t" + p[0] + "\t" + p[2],
			"extra column":   row + "\textra",
			"extra path":     p[0] + "\t" + p[1] + "\tports/mygo/runtime_category.go",
			"alias":          p[0] + "\t" + p[1] + "\tports/mygo/../mygo/" + strings.TrimPrefix(p[2], "ports/mygo/"),
		} {
			variants[fmt.Sprintf("row %d %s", i, name)] = strings.Replace(string(b), row, changed, 1)
		}
		for _, state := range []string{"missing", "wrong", "already advanced"} {
			t.Run(fmt.Sprintf("baseline row %d %s", i, state), func(t *testing.T) {
				bad := clone(expected)
				switch state {
				case "missing":
					delete(bad, p[2])
				case "wrong":
					bad[p[2]] = strings.Repeat("c", 64)
				case "already advanced":
					bad[p[2]] = p[1]
				}
				saved := clone(bad)
				result, e := ordinaryEntryFrozenOverlay(bad, string(b))
				if e == nil || result != nil {
					t.Fatal("unapproved baseline accepted")
				}
				if !reflect.DeepEqual(saved, bad) {
					t.Fatal("failure mutated baseline")
				}
			})
		}
	}
	for name, value := range variants {
		t.Run(name, func(t *testing.T) {
			result, e := ordinaryEntryFrozenOverlay(expected, value)
			if e == nil || result != nil {
				t.Fatal("unapproved overlay accepted")
			}
			if !reflect.DeepEqual(before, expected) {
				t.Fatal("failure mutated baseline")
			}
		})
	}
	for _, state := range []string{"nil", "empty", "extra path", "unrelated missing", "unrelated changed", "all advanced", "malformed hash", "path delimiter"} {
		t.Run("baseline "+state, func(t *testing.T) {
			bad := clone(expected)
			switch state {
			case "nil":
				bad = nil
			case "empty":
				bad = map[string]string{}
			case "extra path":
				bad["ports/mygo/extra.go"] = strings.Repeat("d", 64)
			case "unrelated missing":
				delete(bad, quoteFastFrozenPath)
			case "unrelated changed":
				bad[quoteFastFrozenPath] = quoteFastFrozenOld
			case "all advanced":
				bad = clone(next)
			case "malformed hash":
				bad[quoteFastFrozenPath] = "bad\tentry"
			case "path delimiter":
				delete(bad, quoteFastFrozenPath)
				bad[quoteFastFrozenPath+"\n"] = quoteFastFrozenNew
			}
			saved := clone(bad)
			result, e := ordinaryEntryFrozenOverlay(bad, string(b))
			if e == nil || result != nil {
				t.Fatal("unapproved full baseline accepted")
			}
			if !reflect.DeepEqual(saved, bad) {
				t.Fatal("failure mutated full baseline")
			}
		})
	}
	// Result owns a new map; callers cannot mutate the historical baseline through it.
	next[quoteFastFrozenPath] = "changed"
	if !reflect.DeepEqual(before, expected) {
		t.Fatal("result aliases input")
	}
}

func TestOrdinaryEntryFrozenConsumerScope(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "history_edit_ast_test.go", nil, 0)
	must(t, err)
	var calls []string
	var body *ast.BlockStmt
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "TestHistoryEditFrozenProduction" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("missing frozen consumer")
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && strings.HasSuffix(id.Name, "FrozenOverlay") {
				calls = append(calls, id.Name)
			}
		}
		return true
	})
	want := []string{"outcomeFrozenOverlay", "dayDetailFrozenOverlay", "daySnapshotFrozenOverlay", "dayReadFrozenOverlay", "nativeWindowFrozenOverlay", "quoteFastFrozenOverlay", "ordinaryServiceFrozenOverlay", "categoryUIFrozenOverlay", "ordinaryEntryFrozenOverlay", "ordinaryEntryCleanupFrozenOverlay", "nativeBusinessFrozenOverlay"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("consumer scope chain = %v", calls)
	}
	// Require the scope return to replace expected before the original production loop.
	scopeIndex, rangeIndex := -1, -1
	for i, statement := range body.List {
		if assign, ok := statement.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 {
			if call, ok := assign.Rhs[0].(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "ordinaryEntryFrozenOverlay" {
					if len(assign.Lhs) != 2 || len(call.Args) != 2 {
						t.Fatal("scope result or input dropped")
					}
					lhs, ok := assign.Lhs[0].(*ast.Ident)
					if !ok || lhs.Name != "expected" {
						t.Fatal("scope result unused")
					}
					arg, ok := call.Args[0].(*ast.Ident)
					if !ok || arg.Name != "expected" {
						t.Fatal("scope does not consume previous chain")
					}
					scopeIndex = i
				}
			}
		}
		if _, ok := statement.(*ast.RangeStmt); ok && rangeIndex < 0 {
			rangeIndex = i
		}
	}
	if scopeIndex < 0 || rangeIndex <= scopeIndex {
		t.Fatal("scope must precede original production hash loop")
	}
}

func TestOrdinaryEntryFrozenEveryByteAndPathMutation(t *testing.T) {
	expected := ordinaryEntryFrozenBaseline(t)
	beforeExpected := map[string]string{}
	for p, h := range expected {
		beforeExpected[p] = h
	}
	for i := range []byte(ordinaryEntryFrozenRows) {
		b := []byte(ordinaryEntryFrozenRows)
		b[i] ^= 1
		result, err := ordinaryEntryFrozenOverlay(expected, string(b))
		if err == nil || result != nil || !reflect.DeepEqual(beforeExpected, expected) {
			t.Fatalf("overlay byte mutation %d accepted or mutated input", i)
		}
	}
	clone := func(in map[string]string) map[string]string {
		out := map[string]string{}
		for p, h := range in {
			out[p] = h
		}
		return out
	}
	for path := range expected {
		for _, kind := range []string{"missing", "hash", "alias"} {
			bad := clone(expected)
			switch kind {
			case "missing":
				delete(bad, path)
			case "hash":
				bad[path] = strings.Repeat("f", 64)
			case "alias":
				delete(bad, path)
				bad[path+"/../alias"] = expected[path]
			}
			before := clone(bad)
			result, err := ordinaryEntryFrozenOverlay(bad, ordinaryEntryFrozenRows)
			if err == nil || result != nil || !reflect.DeepEqual(before, bad) {
				t.Fatalf("baseline %s %s accepted/mutated", path, kind)
			}
		}
	}
	t.Logf("rejected %d byte mutations and %d full-baseline mutations", len(ordinaryEntryFrozenRows), len(expected)*3)
}
