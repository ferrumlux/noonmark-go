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
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These constants bind a single proposed transition, not permission to bless
// arbitrary actual source. Changes require review of both exact full manifests.
const captureCloseProductBeforeSHA256 = "01251fda3a9c54ad55a940f70d67713dbc16fd9e261ce585b57bd1a165f56a5d"
const captureCloseProductAfterSHA256 = "032a3a010dfc7fff54f2a40533f0e905122e4d33550aea73d96f64e256898986"
const captureCloseProductRows = `9a51a58b6eb2ddbd22e3dd8126613fecaf461202270bef8e458b2a710a4257bd	334071b43b3b8fb3a5c9852a9068bf2a9652c408be6fcb7bbc094f1cd9d7b25e	ports/mygo/internal/titlecoordinator/coordinator.go
488bb5aad753dc31bc1fc7c935ae4b3fd62707e320ce9f3dad0f7e24adbc8115	08169c490e7202e956a89dbd234ac061d663c2cc5f3fa5e37306b720ce392dcd	ports/mygo/runtime_carryover_actions.go
65a3f6e24b19728ab5d95b9b33c15a417e51567bcc52c926884d3bc46376ee2e	3bdd760b127857860715305bbf0e8d5a7486194c828c1242a44f4580f34bb8a8	ports/mygo/runtime_category.go
afc398d5007e4565c28b048d022234e41d886dab0843baed2af70d19f08519af	38205c56cc0bb052a715997a0e67cced514462826b4dfb159891fabc29fdadac	ports/mygo/runtime_title.go
`
const captureCloseHistoryBeforeSHA256 = "15b7413a97a378f29f354c9f25c53adb4107972dfb168213add559729312b62d"
const captureCloseHistoryAfterSHA256 = "59ebc85299e72f8ac840321aaf34cba2312b7c42928f812de30600eaffaa18ba"
const captureCloseFrozenRows = `9a51a58b6eb2ddbd22e3dd8126613fecaf461202270bef8e458b2a710a4257bd	334071b43b3b8fb3a5c9852a9068bf2a9652c408be6fcb7bbc094f1cd9d7b25e	ports/mygo/internal/titlecoordinator/coordinator.go
488bb5aad753dc31bc1fc7c935ae4b3fd62707e320ce9f3dad0f7e24adbc8115	08169c490e7202e956a89dbd234ac061d663c2cc5f3fa5e37306b720ce392dcd	ports/mygo/runtime_carryover_actions.go
afc398d5007e4565c28b048d022234e41d886dab0843baed2af70d19f08519af	38205c56cc0bb052a715997a0e67cced514462826b4dfb159891fabc29fdadac	ports/mygo/runtime_title.go
`

func captureCloseSourceHash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func captureCloseValidHash(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}
func captureCloseValidPath(p string) bool {
	return strings.HasPrefix(p, "ports/mygo/") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\t\r\n\x00") && !strings.HasPrefix(p, "/") && p != "ports/mygo/" && (strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") || path.Base(p) == "go.mod" || path.Base(p) == "go.sum")
}
func captureCloseRenderSources(entries map[string]string) (string, error) {
	keys := make([]string, 0, len(entries))
	for p, h := range entries {
		if !captureCloseValidPath(p) || !captureCloseValidHash(h) {
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
func captureCloseParseSources(raw string, count int, hash string) (map[string]string, error) {
	if captureCloseSourceHash(raw) != hash || !strings.HasSuffix(raw, "\n") {
		return nil, fmt.Errorf("unapproved full source bytes")
	}
	result := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 2 || !captureCloseValidHash(f[0]) || !captureCloseValidPath(f[1]) {
			return nil, fmt.Errorf("invalid manifest row")
		}
		if _, ok := result[f[1]]; ok {
			return nil, fmt.Errorf("duplicate source path")
		}
		result[f[1]] = f[0]
	}
	rendered, err := captureCloseRenderSources(result)
	if err != nil || rendered != raw || len(result) != count {
		return nil, fmt.Errorf("unapproved source shape/count/order")
	}
	return result, nil
}
func captureCloseApplySources(before map[string]string, raw, want string, beforeCount, afterCount int, beforeHash string) (map[string]string, error) {
	if raw != want {
		return nil, fmt.Errorf("unapproved source delta bytes")
	}
	rendered, err := captureCloseRenderSources(before)
	if err != nil || len(before) != beforeCount || captureCloseSourceHash(rendered) != beforeHash {
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
		if len(f) != 3 || !captureCloseValidPath(f[2]) || !captureCloseValidHash(f[1]) || f[0] == f[1] || seen[f[2]] || f[2] <= last {
			return nil, fmt.Errorf("invalid delta")
		}
		old, exists := before[f[2]]
		if f[0] == "absent" {
			if exists {
				return nil, fmt.Errorf("claimed absent path exists")
			}
		} else if !captureCloseValidHash(f[0]) || !exists || old != f[0] {
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
func captureCloseProductContract(before, delta, after string, actual map[string]string) (map[string]string, error) {
	base, err := captureCloseParseSources(before, 123, captureCloseProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	next, err := captureCloseApplySources(base, delta, captureCloseProductRows, 123, 123, captureCloseProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	published, err := captureCloseParseSources(after, 123, captureCloseProductAfterSHA256)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(published, next) || !reflect.DeepEqual(actual, next) {
		return nil, fmt.Errorf("v5 full product set mismatch")
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
		return nil, fmt.Errorf("v5 inverse not exact v4")
	}
	return historical, nil
}
func captureCloseProductHistoricalInput(t *testing.T, actual map[string]string) map[string]string {
	t.Helper()
	before, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-BEFORE-v5.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-CHANGES-v5.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-SOURCES-v5.tsv")
	must(t, err)
	result, err := captureCloseProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	return result
}
func captureCloseFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	next, err := captureCloseApplySources(expected, overlay, captureCloseFrozenRows, 73, 73, captureCloseHistoryBeforeSHA256)
	if err != nil {
		return nil, err
	}
	rendered, err := captureCloseRenderSources(next)
	if err != nil || captureCloseSourceHash(rendered) != captureCloseHistoryAfterSHA256 {
		return nil, fmt.Errorf("unreviewed full history output")
	}
	inverse := captureCloseCloneSources(next)
	for _, row := range strings.Split(strings.TrimSuffix(overlay, "\n"), "\n") {
		fields := strings.Split(row, "\t")
		if fields[0] == "absent" {
			delete(inverse, fields[2])
		} else {
			inverse[fields[2]] = fields[0]
		}
	}
	if !reflect.DeepEqual(inverse, expected) {
		return nil, fmt.Errorf("v5 history inverse not exact v4")
	}
	return next, nil
}
func captureCloseHistoryBaseline(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/tasks/capture-close-once/FROZEN-BEFORE-v5.tsv")
	must(t, err)
	base, err := captureCloseParseSources(string(raw), 73, captureCloseHistoryBeforeSHA256)
	must(t, err)
	return base
}
func captureCloseCloneSources(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := map[string]string{}
	for p, h := range in {
		out[p] = h
	}
	return out
}

func TestCaptureCloseProductV5RejectsEveryByteAndPathMutation(t *testing.T) {
	before, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-BEFORE-v5.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-CHANGES-v5.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/capture-close-once/PRODUCT-SOURCES-v5.tsv")
	must(t, err)
	actual, err := captureCloseParseSources(string(after), 123, captureCloseProductAfterSHA256)
	must(t, err)
	historical, err := captureCloseProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	if len(historical) != 123 {
		t.Fatal("v4 path count")
	}
	originals := []string{string(before), string(delta), string(after)}
	for part, raw := range originals {
		for i := range []byte(raw) {
			args := append([]string(nil), originals...)
			bad := []byte(raw)
			bad[i] ^= 1
			args[part] = string(bad)
			if result, e := captureCloseProductContract(args[0], args[1], args[2], actual); e == nil || result != nil {
				t.Fatalf("accepted manifest %d byte %d", part, i)
			}
		}
	}
	reject := func(bad map[string]string) {
		t.Helper()
		saved := captureCloseCloneSources(bad)
		if result, e := captureCloseProductContract(string(before), string(delta), string(after), bad); e == nil || result != nil || !reflect.DeepEqual(saved, bad) {
			t.Fatal("accepted/mutated actual source set")
		}
	}
	for p := range actual {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := captureCloseCloneSources(actual)
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
	extra := captureCloseCloneSources(actual)
	extra["ports/mygo/undeclared.go"] = strings.Repeat("e", 64)
	reject(extra)
	for p := range historical {
		bad := captureCloseCloneSources(historical)
		bad[p] = strings.Repeat("f", 64)
		if next, e := captureCloseApplySources(bad, string(delta), captureCloseProductRows, 123, 123, captureCloseProductBeforeSHA256); e == nil || next != nil {
			t.Fatal("accepted incoming mutation", p)
		}
	}
	for _, raw := range []string{"", string(delta) + string(delta), strings.TrimSuffix(string(delta), "\n"), strings.ReplaceAll(string(delta), "\n", "\r\n"), string(delta) + "extra\n"} {
		if result, e := captureCloseApplySources(historical, raw, captureCloseProductRows, 123, 123, captureCloseProductBeforeSHA256); e == nil || result != nil {
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
			if next, e := captureCloseApplySources(historical, strings.Replace(string(delta), row, changed, 1), captureCloseProductRows, 123, 123, captureCloseProductBeforeSHA256); e == nil || next != nil {
				t.Fatal("accepted row mutation")
			}
		}
	}
}
func TestCaptureCloseHistoryOverlayExactScope(t *testing.T) {
	base := captureCloseHistoryBaseline(t)
	saved := captureCloseCloneSources(base)
	raw, err := os.ReadFile("../../docs/tasks/capture-close-once/FROZEN-PRODUCTION-OVERLAY-v5.tsv")
	must(t, err)
	next, err := captureCloseFrozenOverlay(base, string(raw))
	must(t, err)
	for _, phase := range []struct {
		name, hash string
		entries    map[string]string
	}{{"BEFORE", captureCloseHistoryBeforeSHA256, base}, {"AFTER", captureCloseHistoryAfterSHA256, next}} {
		published, e := os.ReadFile("../../docs/tasks/capture-close-once/FROZEN-" + phase.name + "-v5.tsv")
		must(t, e)
		parsed, e := captureCloseParseSources(string(published), 73, phase.hash)
		must(t, e)
		if !reflect.DeepEqual(parsed, phase.entries) {
			t.Fatal("published history drift", phase.name)
		}
		for i := range published {
			bad := append([]byte(nil), published...)
			bad[i] ^= 1
			if result, err := captureCloseParseSources(string(bad), 73, phase.hash); err == nil || result != nil {
				t.Fatalf("accepted history %s manifest byte %d", phase.name, i)
			}
		}
	}
	reject := func(bad map[string]string, rows string) {
		t.Helper()
		copy := captureCloseCloneSources(bad)
		if result, e := captureCloseFrozenOverlay(bad, rows); e == nil || result != nil || !reflect.DeepEqual(copy, bad) {
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
			bad := captureCloseCloneSources(base)
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
	extra := captureCloseCloneSources(base)
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

func TestCaptureCloseSourceConsumers(t *testing.T) {
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
	if !reflect.DeepEqual(conversions, []string{"captureCloseProductHistoricalInput", "runtimeAdmissionProductHistoricalInput", "nativeBusinessProductHistoricalInput", "entryProductV2HistoricalInput"}) {
		t.Fatal("v5 must independently pass before v4/v2/v1", conversions)
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
	want := []string{"outcomeFrozenOverlay", "dayDetailFrozenOverlay", "daySnapshotFrozenOverlay", "dayReadFrozenOverlay", "nativeWindowFrozenOverlay", "quoteFastFrozenOverlay", "ordinaryServiceFrozenOverlay", "categoryUIFrozenOverlay", "ordinaryEntryFrozenOverlay", "ordinaryEntryCleanupFrozenOverlay", "nativeBusinessFrozenOverlay", "runtimeAdmissionFrozenOverlay", "captureCloseFrozenOverlay"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatal("history chain changed", calls)
	}
	for i, st := range body.List {
		if a, ok := st.(*ast.AssignStmt); ok && len(a.Rhs) == 1 {
			if call, ok := a.Rhs[0].(*ast.CallExpr); ok {
				if f, ok := call.Fun.(*ast.Ident); ok && f.Name == "captureCloseFrozenOverlay" {
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

// Independently bind real history bytes before attempting the inverse transition.
func TestCaptureCloseHistoryActualCompleteBeforeInverse(t *testing.T) {
	before, err := os.ReadFile("../../docs/tasks/capture-close-once/FROZEN-BEFORE-v5.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/capture-close-once/FROZEN-AFTER-v5.tsv")
	must(t, err)
	base, err := captureCloseParseSources(string(before), 73, captureCloseHistoryBeforeSHA256)
	must(t, err)
	want, err := captureCloseParseSources(string(after), 73, captureCloseHistoryAfterSHA256)
	must(t, err)
	actual := map[string]string{}
	for p := range want {
		name := filepath.Join("../../../..", p)
		info, e := os.Lstat(name)
		must(t, e)
		if !info.Mode().IsRegular() {
			t.Fatal("history source not regular", p)
		}
		raw, e := os.ReadFile(name)
		must(t, e)
		actual[p] = captureCloseSourceHash(string(raw))
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatal("full actual history differs before inverse")
	}
	inverse := captureCloseCloneSources(actual)
	for _, row := range strings.Split(strings.TrimSuffix(captureCloseFrozenRows, "\n"), "\n") {
		f := strings.Split(row, "\t")
		inverse[f[2]] = f[0]
	}
	if !reflect.DeepEqual(inverse, base) {
		t.Fatal("actual inverse is not exact v4")
	}
}

// Validate actual consumer syntax, not a list supplied by the test author.
func captureCloseConsumerFlow(source, name string, product bool) error {
	file, err := parser.ParseFile(token.NewFileSet(), "consumer.go", source, 0)
	if err != nil {
		return err
	}
	var body *ast.BlockStmt
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == name {
			body = f.Body
		}
	}
	if body == nil {
		return fmt.Errorf("missing consumer")
	}
	scan, scanChecked, v5, v4, firstRange := -1, -1, -1, -1, -1
	for i, st := range body.List {
		if _, ok := st.(*ast.RangeStmt); ok && firstRange < 0 {
			firstRange = i
		}
		if ex, ok := st.(*ast.ExprStmt); ok {
			if call, ok := ex.X.(*ast.CallExpr); ok {
				if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "must" && len(call.Args) == 2 {
					if arg, ok := call.Args[1].(*ast.Ident); ok && arg.Name == "err" && scan >= 0 && i > scan && v5 < 0 {
						scanChecked = i
					}
				}
			}
		}
		a, ok := st.(*ast.AssignStmt)
		if !ok || len(a.Rhs) != 1 {
			continue
		}
		call, ok := a.Rhs[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "filepath" && sel.Sel.Name == "WalkDir" {
				scan = i
			}
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok {
			continue
		}
		expected, previous, variable, argindex, lhsCount := "captureCloseFrozenOverlay", "runtimeAdmissionFrozenOverlay", "expected", 0, 2
		if product {
			expected, previous, variable, argindex, lhsCount = "captureCloseProductHistoricalInput", "runtimeAdmissionProductHistoricalInput", "actual", 1, 1
		}
		if fn.Name != expected && fn.Name != previous {
			continue
		}
		if len(a.Lhs) != lhsCount || len(call.Args) != 2 {
			return fmt.Errorf("flow arity")
		}
		lhs, lok := a.Lhs[0].(*ast.Ident)
		arg, aok := call.Args[argindex].(*ast.Ident)
		if !lok || !aok || lhs.Name != variable || arg.Name != variable {
			return fmt.Errorf("substituted input or discarded output")
		}
		if fn.Name == expected {
			if v5 >= 0 {
				return fmt.Errorf("duplicate v5")
			}
			v5 = i
		} else {
			if v4 >= 0 {
				return fmt.Errorf("duplicate v4")
			}
			v4 = i
		}
	}
	if product {
		if scan < 0 || scanChecked <= scan || v5 <= scanChecked || v4 <= v5 {
			return fmt.Errorf("actual scan/check/v5/v4 order")
		}
	} else if v4 < 0 || v5 <= v4 || firstRange <= v5 {
		return fmt.Errorf("v4/v5/actual-check order")
	}
	return nil
}

func TestCaptureCloseConsumerFlowRejectsBypassMutations(t *testing.T) {
	for _, c := range []struct {
		file, name string
		product    bool
	}{
		{"ordinary_entry_sources_test.go", "TestOrdinaryEntryProductSourceContract", true},
		{"history_edit_ast_test.go", "TestHistoryEditFrozenProduction", false},
	} {
		raw, err := os.ReadFile(c.file)
		must(t, err)
		source := string(raw)
		must(t, captureCloseConsumerFlow(source, c.name, c.product))
		var mutations [][2]string
		if c.product {
			mutations = [][2]string{
				{"actual = captureCloseProductHistoricalInput(t, actual)", "actual = runtimeAdmissionProductHistoricalInput(t, actual)"},
				{"actual = captureCloseProductHistoricalInput(t, actual)", "actual = captureCloseProductHistoricalInput(t, beforeSources)"},
				{"actual = captureCloseProductHistoricalInput(t, actual)", "_ = captureCloseProductHistoricalInput(t, actual)"},
				{"filepath.WalkDir", "filepath.NotWalkDir"},
				{"actual = captureCloseProductHistoricalInput(t, actual)\n\tactual = runtimeAdmissionProductHistoricalInput(t, actual)", "actual = runtimeAdmissionProductHistoricalInput(t, actual)\n\tactual = captureCloseProductHistoricalInput(t, actual)"},
			}
		} else {
			mutations = [][2]string{
				{"expected, e = captureCloseFrozenOverlay(expected, string(captureOverlay))", "expected, e = runtimeAdmissionFrozenOverlay(expected, string(captureOverlay))"},
				{"expected, e = captureCloseFrozenOverlay(expected, string(captureOverlay))", "expected, e = captureCloseFrozenOverlay(nil, string(captureOverlay))"},
				{"expected, e = captureCloseFrozenOverlay(expected, string(captureOverlay))", "_, e = captureCloseFrozenOverlay(expected, string(captureOverlay))"},
				{"for _, line := range strings.Split(strings.TrimSpace(string(rows)), \"\\n\")", "for false"},
			}
		}
		for _, m := range mutations {
			if strings.Count(source, m[0]) != 1 {
				t.Fatal("mutation anchor not unique", c.file, m[0])
			}
			changed := strings.Replace(source, m[0], m[1], 1)
			if captureCloseConsumerFlow(changed, c.name, c.product) == nil {
				t.Fatal("accepted consumer bypass", c.file, m[0])
			}
		}
	}
}
