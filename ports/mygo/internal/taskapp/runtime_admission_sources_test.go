// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These constants bind a single proposed transition, not permission to bless
// arbitrary actual source. Changes require review of both exact full manifests.
const runtimeAdmissionProductBeforeSHA256 = "5109881f5dd9784dea5902470c52c9120e170a9393070bd21e93d477def25f01"
const runtimeAdmissionProductAfterSHA256 = "01251fda3a9c54ad55a940f70d67713dbc16fd9e261ce585b57bd1a165f56a5d"
const runtimeAdmissionProductRows = `32758ca3d4ef31b7352a96235b03395c082b17494d0e8398ade8c902b488f055	9a51a58b6eb2ddbd22e3dd8126613fecaf461202270bef8e458b2a710a4257bd	ports/mygo/internal/titlecoordinator/coordinator.go
d456939dec42e332d58d42c3247ac3c17e4888a5b540901ed820ede7970a3450	b85a4043e14a22f7ce26b0d7d4f0270163d3f2f19cb636633d6c447c8b8c04c9	ports/mygo/internal/titlecoordinator/lane.go
a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077	426fdf2864efefebbd0c54f9c09da91364a3b16b728ba2915524075a4da0cf48	ports/mygo/main.go
c5ea59afc503637bef9b12383c318951d1c18bc0610ee6459083dc7eb8f3aab6	b3d5c26dce70cbfb769ec22fa966f28abd0c21e82e23281d63984a3503b933fe	ports/mygo/native_business_probe.go
absent	1de77901a5f2b99b74bda2c93c42bf178709f43c0773d2e92c586cc12da55e75	ports/mygo/runtime_admission.go
b6bc3fae51a38f6cb84c53355727d36488e7b22e564ab13a5dfc433be8d965a7	65a3f6e24b19728ab5d95b9b33c15a417e51567bcc52c926884d3bc46376ee2e	ports/mygo/runtime_category.go
6361bd443bf4fc568a74580a8077293127ebb3e6b1fc2f81fa21d997c1d9a27b	44b828384409eab3b868020380efe40484a21a3abb327ae8ba9dd50dc05c8b2d	ports/mygo/runtime_subtask.go
c0b945694a946b08725ac0cf75b24317c1c5548e49ad2446fd5bf7f277200565	afc398d5007e4565c28b048d022234e41d886dab0843baed2af70d19f08519af	ports/mygo/runtime_title.go
464b3894b3bba341f30460d15f37e92ce7526c0ed0e730bfeacc9290a3db4155	044ff4a889d8ff6cde9dd402fc900d0dc67abce25877e6db6fcae73e162f97a5	ports/mygo/runtime_title_disposal.go
`
const runtimeAdmissionHistoryBeforeSHA256 = "2b5491f5e2bf9d699a48a9ee7353a0cd013dc78f04f511ac9ba977ba9917a14d"
const runtimeAdmissionHistoryAfterSHA256 = "15b7413a97a378f29f354c9f25c53adb4107972dfb168213add559729312b62d"
const runtimeAdmissionFrozenRows = `32758ca3d4ef31b7352a96235b03395c082b17494d0e8398ade8c902b488f055	9a51a58b6eb2ddbd22e3dd8126613fecaf461202270bef8e458b2a710a4257bd	ports/mygo/internal/titlecoordinator/coordinator.go
d456939dec42e332d58d42c3247ac3c17e4888a5b540901ed820ede7970a3450	b85a4043e14a22f7ce26b0d7d4f0270163d3f2f19cb636633d6c447c8b8c04c9	ports/mygo/internal/titlecoordinator/lane.go
a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077	426fdf2864efefebbd0c54f9c09da91364a3b16b728ba2915524075a4da0cf48	ports/mygo/main.go
6361bd443bf4fc568a74580a8077293127ebb3e6b1fc2f81fa21d997c1d9a27b	44b828384409eab3b868020380efe40484a21a3abb327ae8ba9dd50dc05c8b2d	ports/mygo/runtime_subtask.go
c0b945694a946b08725ac0cf75b24317c1c5548e49ad2446fd5bf7f277200565	afc398d5007e4565c28b048d022234e41d886dab0843baed2af70d19f08519af	ports/mygo/runtime_title.go
464b3894b3bba341f30460d15f37e92ce7526c0ed0e730bfeacc9290a3db4155	044ff4a889d8ff6cde9dd402fc900d0dc67abce25877e6db6fcae73e162f97a5	ports/mygo/runtime_title_disposal.go
`

func runtimeAdmissionSourceHash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func runtimeAdmissionValidHash(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}
func runtimeAdmissionValidPath(p string) bool {
	return strings.HasPrefix(p, "ports/mygo/") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\t\r\n\x00") && !strings.HasPrefix(p, "/") && p != "ports/mygo/" && (strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") || path.Base(p) == "go.mod" || path.Base(p) == "go.sum")
}
func runtimeAdmissionRenderSources(entries map[string]string) (string, error) {
	keys := make([]string, 0, len(entries))
	for p, h := range entries {
		if !runtimeAdmissionValidPath(p) || !runtimeAdmissionValidHash(h) {
			return "", fmt.Errorf("invalid source entry")
		}
		keys = append(keys, p)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, p := range keys {
		fmt.Fprintf(&out, "%s\t%s\n", entries[p], p)
	}
	return out.String(), nil
}
func runtimeAdmissionParseSources(raw string, count int, hash string) (map[string]string, error) {
	if runtimeAdmissionSourceHash(raw) != hash || !strings.HasSuffix(raw, "\n") {
		return nil, fmt.Errorf("unapproved full source bytes")
	}
	result := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 2 || !runtimeAdmissionValidHash(f[0]) || !runtimeAdmissionValidPath(f[1]) {
			return nil, fmt.Errorf("invalid manifest row")
		}
		if _, ok := result[f[1]]; ok {
			return nil, fmt.Errorf("duplicate source path")
		}
		result[f[1]] = f[0]
	}
	rendered, err := runtimeAdmissionRenderSources(result)
	if err != nil || rendered != raw || len(result) != count {
		return nil, fmt.Errorf("unapproved source shape/count/order")
	}
	return result, nil
}
func runtimeAdmissionApplySources(before map[string]string, raw, want string, beforeCount, afterCount int, beforeHash string) (map[string]string, error) {
	if raw != want {
		return nil, fmt.Errorf("unapproved source delta bytes")
	}
	rendered, err := runtimeAdmissionRenderSources(before)
	if err != nil || len(before) != beforeCount || runtimeAdmissionSourceHash(rendered) != beforeHash {
		return nil, fmt.Errorf("unapproved full incoming source set")
	}
	next := map[string]string{}
	for p, h := range before {
		next[p] = h
	}
	seen := map[string]bool{}
	last := ""
	for _, row := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 3 || !runtimeAdmissionValidPath(f[2]) || !runtimeAdmissionValidHash(f[1]) || f[0] == f[1] || seen[f[2]] || f[2] <= last {
			return nil, fmt.Errorf("invalid delta")
		}
		old, exists := before[f[2]]
		if f[0] == "absent" {
			if exists {
				return nil, fmt.Errorf("claimed absent path exists")
			}
		} else if !runtimeAdmissionValidHash(f[0]) || !exists || old != f[0] {
			return nil, fmt.Errorf("old source mismatch")
		}
		seen[f[2]] = true
		last = f[2]
		next[f[2]] = f[1]
	}
	if len(next) != afterCount {
		return nil, fmt.Errorf("unapproved outgoing count")
	}
	return next, nil
}
func runtimeAdmissionProductContract(before, delta, after string, actual map[string]string) (map[string]string, error) {
	base, err := runtimeAdmissionParseSources(before, 122, runtimeAdmissionProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	next, err := runtimeAdmissionApplySources(base, delta, runtimeAdmissionProductRows, 122, 123, runtimeAdmissionProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	published, err := runtimeAdmissionParseSources(after, 123, runtimeAdmissionProductAfterSHA256)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(published, next) || !reflect.DeepEqual(actual, next) {
		return nil, fmt.Errorf("v4 full product set mismatch")
	}
	// Reverse only the reviewed delta, after the whole incoming/outgoing contract
	// passed. No substitution of actual hashes into historical expected values.
	historical := map[string]string{}
	for p, h := range actual {
		historical[p] = h
	}
	for _, row := range strings.Split(strings.TrimSuffix(delta, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if f[0] == "absent" {
			delete(historical, f[2])
		} else {
			historical[f[2]] = f[0]
		}
	}
	if !reflect.DeepEqual(historical, base) {
		return nil, fmt.Errorf("v4 inverse not exact v3")
	}
	return historical, nil
}
func runtimeAdmissionProductHistoricalInput(t *testing.T, actual map[string]string) map[string]string {
	t.Helper()
	before, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-BEFORE-v4.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-CHANGES-v4.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-SOURCES-v4.tsv")
	must(t, err)
	result, err := runtimeAdmissionProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	return result
}
func runtimeAdmissionFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	next, err := runtimeAdmissionApplySources(expected, overlay, runtimeAdmissionFrozenRows, 73, 73, runtimeAdmissionHistoryBeforeSHA256)
	if err != nil {
		return nil, err
	}
	rendered, err := runtimeAdmissionRenderSources(next)
	if err != nil || runtimeAdmissionSourceHash(rendered) != runtimeAdmissionHistoryAfterSHA256 {
		return nil, fmt.Errorf("unreviewed full history output")
	}
	inverse := runtimeAdmissionCloneSources(next)
	for _, row := range strings.Split(strings.TrimSuffix(overlay, "\n"), "\n") {
		fields := strings.Split(row, "\t")
		if fields[0] == "absent" {
			delete(inverse, fields[2])
		} else {
			inverse[fields[2]] = fields[0]
		}
	}
	if !reflect.DeepEqual(inverse, expected) {
		return nil, fmt.Errorf("v4 history inverse not exact v3")
	}
	return next, nil
}
func runtimeAdmissionHistoryBaseline(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/FROZEN-BEFORE-v4.tsv")
	must(t, err)
	base, err := runtimeAdmissionParseSources(string(raw), 73, runtimeAdmissionHistoryBeforeSHA256)
	must(t, err)
	return base
}
func runtimeAdmissionCloneSources(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := map[string]string{}
	for p, h := range in {
		out[p] = h
	}
	return out
}

func TestRuntimeAdmissionProductV4RejectsEveryByteAndPathMutation(t *testing.T) {
	before, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-BEFORE-v4.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-CHANGES-v4.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/PRODUCT-SOURCES-v4.tsv")
	must(t, err)
	actual, err := runtimeAdmissionParseSources(string(after), 123, runtimeAdmissionProductAfterSHA256)
	must(t, err)
	historical, err := runtimeAdmissionProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	if len(historical) != 122 {
		t.Fatal("v3 path count")
	}
	originals := []string{string(before), string(delta), string(after)}
	for part, raw := range originals {
		for i := range []byte(raw) {
			args := append([]string(nil), originals...)
			bad := []byte(raw)
			bad[i] ^= 1
			args[part] = string(bad)
			if result, e := runtimeAdmissionProductContract(args[0], args[1], args[2], actual); e == nil || result != nil {
				t.Fatalf("accepted manifest %d byte %d", part, i)
			}
		}
	}
	reject := func(bad map[string]string) {
		t.Helper()
		saved := runtimeAdmissionCloneSources(bad)
		if result, e := runtimeAdmissionProductContract(string(before), string(delta), string(after), bad); e == nil || result != nil || !reflect.DeepEqual(saved, bad) {
			t.Fatal("accepted/mutated actual source set")
		}
	}
	for p := range actual {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := runtimeAdmissionCloneSources(actual)
			switch kind {
			case "hash":
				bad[p] = strings.Repeat("f", 64)
			case "missing":
				delete(bad, p)
			case "alias":
				delete(bad, p)
				bad[p+"/../alias"] = actual[p]
			}
			reject(bad)
		}
	}
	reject(nil)
	reject(map[string]string{})
	extra := runtimeAdmissionCloneSources(actual)
	extra["ports/mygo/undeclared.go"] = strings.Repeat("e", 64)
	reject(extra)
	for p := range historical {
		bad := runtimeAdmissionCloneSources(historical)
		bad[p] = strings.Repeat("f", 64)
		if next, e := runtimeAdmissionApplySources(bad, string(delta), runtimeAdmissionProductRows, 122, 123, runtimeAdmissionProductBeforeSHA256); e == nil || next != nil {
			t.Fatal("accepted incoming mutation", p)
		}
	}
	for _, raw := range []string{"", string(delta) + string(delta), strings.TrimSuffix(string(delta), "\n"), strings.ReplaceAll(string(delta), "\n", "\r\n"), string(delta) + "extra\n"} {
		if result, e := runtimeAdmissionApplySources(historical, raw, runtimeAdmissionProductRows, 122, 123, runtimeAdmissionProductBeforeSHA256); e == nil || result != nil {
			t.Fatal("accepted malformed delta")
		}
	}
	// Every row's absent/old/new claim is independently bound, including aliases.
	for _, row := range strings.Split(strings.TrimSuffix(string(delta), "\n"), "\n") {
		f := strings.Split(row, "\t")
		for _, changed := range []string{"absent\t" + f[1] + "\t" + f[2], f[1] + "\t" + f[1] + "\t" + f[2], f[0] + "\t" + f[1] + "\tports/mygo/../mygo/" + strings.TrimPrefix(f[2], "ports/mygo/"), row + "\textra"} {
			if changed == row {
				continue
			}
			if next, e := runtimeAdmissionApplySources(historical, strings.Replace(string(delta), row, changed, 1), runtimeAdmissionProductRows, 122, 123, runtimeAdmissionProductBeforeSHA256); e == nil || next != nil {
				t.Fatal("accepted row mutation")
			}
		}
	}
}
func TestRuntimeAdmissionHistoryOverlayExactScope(t *testing.T) {
	base := runtimeAdmissionHistoryBaseline(t)
	saved := runtimeAdmissionCloneSources(base)
	raw, err := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/FROZEN-PRODUCTION-OVERLAY-v4.tsv")
	must(t, err)
	next, err := runtimeAdmissionFrozenOverlay(base, string(raw))
	must(t, err)
	for _, phase := range []struct {
		name, hash string
		entries    map[string]string
	}{{"BEFORE", runtimeAdmissionHistoryBeforeSHA256, base}, {"AFTER", runtimeAdmissionHistoryAfterSHA256, next}} {
		published, e := os.ReadFile("../../docs/tasks/runtime-admission-lifecycle/FROZEN-" + phase.name + "-v4.tsv")
		must(t, e)
		parsed, e := runtimeAdmissionParseSources(string(published), 73, phase.hash)
		must(t, e)
		if !reflect.DeepEqual(parsed, phase.entries) {
			t.Fatal("published history drift", phase.name)
		}
		for i := range published {
			bad := append([]byte(nil), published...)
			bad[i] ^= 1
			if result, err := runtimeAdmissionParseSources(string(bad), 73, phase.hash); err == nil || result != nil {
				t.Fatalf("accepted history %s manifest byte %d", phase.name, i)
			}
		}
	}
	reject := func(bad map[string]string, rows string) {
		t.Helper()
		copy := runtimeAdmissionCloneSources(bad)
		if result, e := runtimeAdmissionFrozenOverlay(bad, rows); e == nil || result != nil || !reflect.DeepEqual(copy, bad) {
			t.Fatal("accepted/mutated history overlay")
		}
	}
	for i := range []byte(raw) {
		bad := append([]byte(nil), raw...)
		bad[i] ^= 1
		reject(base, string(bad))
	}
	for p := range base {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := runtimeAdmissionCloneSources(base)
			switch kind {
			case "hash":
				bad[p] = strings.Repeat("f", 64)
			case "missing":
				delete(bad, p)
			case "alias":
				delete(bad, p)
				bad[p+"/../alias"] = base[p]
			}
			reject(bad, string(raw))
		}
	}
	reject(nil, string(raw))
	reject(map[string]string{}, string(raw))
	reject(next, string(raw))
	extra := runtimeAdmissionCloneSources(base)
	extra["ports/mygo/native_business_probe.go"] = strings.Repeat("a", 64)
	reject(extra, string(raw))
	for _, rows := range []string{"", string(raw) + string(raw), strings.TrimSuffix(string(raw), "\n"), strings.ReplaceAll(string(raw), "\n", "\r\n"), string(raw) + "extra\n"} {
		reject(base, rows)
	}
	next["ports/mygo/main.go"] = "mutated"
	if !reflect.DeepEqual(saved, base) {
		t.Fatal("result aliases incoming")
	}
}

func TestRuntimeAdmissionSourceConsumers(t *testing.T) {
	parseBody := func(file, name string) *ast.BlockStmt {
		t.Helper()
		f, e := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		must(t, e)
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
				return fn.Body
			}
		}
		t.Fatal("missing consumer", name)
		return nil
	}
	product := parseBody("ordinary_entry_sources_test.go", "TestOrdinaryEntryProductSourceContract")
	var conversions []string
	for _, st := range product.List {
		if a, ok := st.(*ast.AssignStmt); ok && len(a.Lhs) == 1 && len(a.Rhs) == 1 {
			if call, ok := a.Rhs[0].(*ast.CallExpr); ok {
				if fn, ok := call.Fun.(*ast.Ident); ok && strings.HasSuffix(fn.Name, "HistoricalInput") {
					lhs, lok := a.Lhs[0].(*ast.Ident)
					if !lok || lhs.Name != "actual" || len(call.Args) != 2 {
						t.Fatal("historical result unused")
					}
					arg, aok := call.Args[1].(*ast.Ident)
					if !aok || arg.Name != "actual" {
						t.Fatal("historical input substituted")
					}
					conversions = append(conversions, fn.Name)
				}
			}
		}
	}
	if !reflect.DeepEqual(conversions, []string{"runtimeAdmissionProductHistoricalInput", "nativeBusinessProductHistoricalInput", "entryProductV2HistoricalInput"}) {
		t.Fatal("v4 must independently pass before v3/v2/v1", conversions)
	}
	body := parseBody("history_edit_ast_test.go", "TestHistoryEditFrozenProduction")
	var calls []string
	scopeIndex, rangeIndex := -1, -1
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if f, ok := c.Fun.(*ast.Ident); ok && strings.HasSuffix(f.Name, "FrozenOverlay") {
				calls = append(calls, f.Name)
			}
		}
		return true
	})
	want := []string{"outcomeFrozenOverlay", "dayDetailFrozenOverlay", "daySnapshotFrozenOverlay", "dayReadFrozenOverlay", "nativeWindowFrozenOverlay", "quoteFastFrozenOverlay", "ordinaryServiceFrozenOverlay", "categoryUIFrozenOverlay", "ordinaryEntryFrozenOverlay", "ordinaryEntryCleanupFrozenOverlay", "nativeBusinessFrozenOverlay", "runtimeAdmissionFrozenOverlay"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatal("history chain changed", calls)
	}
	for i, st := range body.List {
		if a, ok := st.(*ast.AssignStmt); ok && len(a.Rhs) == 1 {
			if call, ok := a.Rhs[0].(*ast.CallExpr); ok {
				if f, ok := call.Fun.(*ast.Ident); ok && f.Name == "runtimeAdmissionFrozenOverlay" {
					if len(a.Lhs) != 2 || len(call.Args) != 2 {
						t.Fatal("overlay arity")
					}
					lhs, lok := a.Lhs[0].(*ast.Ident)
					arg, aok := call.Args[0].(*ast.Ident)
					if !lok || !aok || lhs.Name != "expected" || arg.Name != "expected" {
						t.Fatal("overlay bypassed")
					}
					scopeIndex = i
				}
			}
		}
		if _, ok := st.(*ast.RangeStmt); ok && rangeIndex < 0 {
			rangeIndex = i
		}
	}
	if scopeIndex < 0 || rangeIndex <= scopeIndex {
		t.Fatal("business overlay must precede original frozen loop")
	}
}
