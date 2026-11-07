// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
const nativeBusinessProductBeforeSHA256 = "81a93f13d6d3e8d70c27f3bdd43fb3a87627c171c733d47651695f7b46943cec"
const nativeBusinessProductAfterSHA256 = "5109881f5dd9784dea5902470c52c9120e170a9393070bd21e93d477def25f01"
const nativeBusinessProductRows = `c98d2afcbf81472783aca205a796febb51405d754d1d8bedc1ac2a222b082878	a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077	ports/mygo/main.go
absent	ae6f1cadf53d354fe1ae54f29127061ff498502fe42b9fe54289df1c1e5f406d	ports/mygo/native_business_facts.go
absent	c5ea59afc503637bef9b12383c318951d1c18bc0610ee6459083dc7eb8f3aab6	ports/mygo/native_business_probe.go
333aa81b40966087be4e7b499fe39c0c0e1622b04baddfee44b03a410a2c21ca	668a089d0cda8e1fe5e98cb4505f4a5ea95981c949832bc1a2443af6f867c99b	ports/mygo/native_window_preflight.go
50959266a37ea4bd0b872a4b40dadb39ff4702a4d19640870a99a30e23bf8478	786271af84fc9ee1de9797b16f7d931a3794b1e2b8cc1d2473fd79bd011da9cd	ports/mygo/native_window_probe.go
f041dac2a53b1e361778506e06ee3c953d946fcc82bec635654c188150becb25	bc30965570be4c22cce103ba5687f3520dc43adb1a45468414b64d74d4d86370	ports/mygo/runtime_startup.go
`
const nativeBusinessHistoryBeforeSHA256 = "2ae69effd683916884184725a97e6e34d4effde5b25db903c7c1f9065f017e7a"
const nativeBusinessHistoryAfterSHA256 = "2b5491f5e2bf9d699a48a9ee7353a0cd013dc78f04f511ac9ba977ba9917a14d"
const nativeBusinessFrozenRows = `c98d2afcbf81472783aca205a796febb51405d754d1d8bedc1ac2a222b082878	a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077	ports/mygo/main.go
`

func nativeBusinessSourceHash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func nativeBusinessValidHash(s string) bool {
	return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == ""
}
func nativeBusinessValidPath(p string) bool {
	return strings.HasPrefix(p, "ports/mygo/") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\t\r\n\x00") && !strings.HasPrefix(p, "/") && p != "ports/mygo/" && (strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") || path.Base(p) == "go.mod" || path.Base(p) == "go.sum")
}
func nativeBusinessRenderSources(entries map[string]string) (string, error) {
	keys := make([]string, 0, len(entries))
	for p, h := range entries {
		if !nativeBusinessValidPath(p) || !nativeBusinessValidHash(h) {
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
func nativeBusinessParseSources(raw string, count int, hash string) (map[string]string, error) {
	if nativeBusinessSourceHash(raw) != hash || !strings.HasSuffix(raw, "\n") {
		return nil, fmt.Errorf("unapproved full source bytes")
	}
	result := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 2 || !nativeBusinessValidHash(f[0]) || !nativeBusinessValidPath(f[1]) {
			return nil, fmt.Errorf("invalid manifest row")
		}
		if _, ok := result[f[1]]; ok {
			return nil, fmt.Errorf("duplicate source path")
		}
		result[f[1]] = f[0]
	}
	rendered, err := nativeBusinessRenderSources(result)
	if err != nil || rendered != raw || len(result) != count {
		return nil, fmt.Errorf("unapproved source shape/count/order")
	}
	return result, nil
}
func nativeBusinessApplySources(before map[string]string, raw, want string, beforeCount, afterCount int, beforeHash string) (map[string]string, error) {
	if raw != want {
		return nil, fmt.Errorf("unapproved source delta bytes")
	}
	rendered, err := nativeBusinessRenderSources(before)
	if err != nil || len(before) != beforeCount || nativeBusinessSourceHash(rendered) != beforeHash {
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
		if len(f) != 3 || !nativeBusinessValidPath(f[2]) || !nativeBusinessValidHash(f[1]) || f[0] == f[1] || seen[f[2]] || f[2] <= last {
			return nil, fmt.Errorf("invalid delta")
		}
		old, exists := before[f[2]]
		if f[0] == "absent" {
			if exists {
				return nil, fmt.Errorf("claimed absent path exists")
			}
		} else if !nativeBusinessValidHash(f[0]) || !exists || old != f[0] {
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
func nativeBusinessProductContract(before, delta, after string, actual map[string]string) (map[string]string, error) {
	base, err := nativeBusinessParseSources(before, 120, nativeBusinessProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	next, err := nativeBusinessApplySources(base, delta, nativeBusinessProductRows, 120, 122, nativeBusinessProductBeforeSHA256)
	if err != nil {
		return nil, err
	}
	published, err := nativeBusinessParseSources(after, 122, nativeBusinessProductAfterSHA256)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(published, next) || !reflect.DeepEqual(actual, next) {
		return nil, fmt.Errorf("v3 full product set mismatch")
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
		return nil, fmt.Errorf("v3 inverse not exact v2")
	}
	return historical, nil
}
func nativeBusinessProductHistoricalInput(t *testing.T, actual map[string]string) map[string]string {
	t.Helper()
	before, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/PRODUCT-SOURCES-v2.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/native-category-business/PRODUCT-CHANGES-v3.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/native-category-business/PRODUCT-SOURCES-v3.tsv")
	must(t, err)
	result, err := nativeBusinessProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	return result
}
func nativeBusinessFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	return nativeBusinessApplySources(expected, overlay, nativeBusinessFrozenRows, 73, 73, nativeBusinessHistoryBeforeSHA256)
}
func nativeBusinessHistoryBaseline(t *testing.T) map[string]string {
	t.Helper()
	before := ordinaryEntryCleanupFrozenBaseline(t)
	raw, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/FROZEN-PRODUCTION-OVERLAY-v2.tsv")
	must(t, err)
	next, err := ordinaryEntryCleanupFrozenOverlay(before, string(raw))
	must(t, err)
	return next
}
func nativeBusinessCloneSources(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := map[string]string{}
	for p, h := range in {
		out[p] = h
	}
	return out
}

func TestNativeBusinessProductV3RejectsEveryByteAndPathMutation(t *testing.T) {
	before, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/PRODUCT-SOURCES-v2.tsv")
	must(t, err)
	delta, err := os.ReadFile("../../docs/tasks/native-category-business/PRODUCT-CHANGES-v3.tsv")
	must(t, err)
	after, err := os.ReadFile("../../docs/tasks/native-category-business/PRODUCT-SOURCES-v3.tsv")
	must(t, err)
	actual, err := nativeBusinessParseSources(string(after), 122, nativeBusinessProductAfterSHA256)
	must(t, err)
	historical, err := nativeBusinessProductContract(string(before), string(delta), string(after), actual)
	must(t, err)
	if len(historical) != 120 {
		t.Fatal("v2 path count")
	}
	originals := []string{string(before), string(delta), string(after)}
	for part, raw := range originals {
		for i := range []byte(raw) {
			args := append([]string(nil), originals...)
			bad := []byte(raw)
			bad[i] ^= 1
			args[part] = string(bad)
			if result, e := nativeBusinessProductContract(args[0], args[1], args[2], actual); e == nil || result != nil {
				t.Fatalf("accepted manifest %d byte %d", part, i)
			}
		}
	}
	reject := func(bad map[string]string) {
		t.Helper()
		saved := nativeBusinessCloneSources(bad)
		if result, e := nativeBusinessProductContract(string(before), string(delta), string(after), bad); e == nil || result != nil || !reflect.DeepEqual(saved, bad) {
			t.Fatal("accepted/mutated actual source set")
		}
	}
	for p := range actual {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := nativeBusinessCloneSources(actual)
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
	extra := nativeBusinessCloneSources(actual)
	extra["ports/mygo/undeclared.go"] = strings.Repeat("e", 64)
	reject(extra)
	for p := range historical {
		bad := nativeBusinessCloneSources(historical)
		bad[p] = strings.Repeat("f", 64)
		if next, e := nativeBusinessApplySources(bad, string(delta), nativeBusinessProductRows, 120, 122, nativeBusinessProductBeforeSHA256); e == nil || next != nil {
			t.Fatal("accepted incoming mutation", p)
		}
	}
	for _, raw := range []string{"", string(delta) + string(delta), strings.TrimSuffix(string(delta), "\n"), strings.ReplaceAll(string(delta), "\n", "\r\n"), string(delta) + "extra\n"} {
		if result, e := nativeBusinessApplySources(historical, raw, nativeBusinessProductRows, 120, 122, nativeBusinessProductBeforeSHA256); e == nil || result != nil {
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
			if next, e := nativeBusinessApplySources(historical, strings.Replace(string(delta), row, changed, 1), nativeBusinessProductRows, 120, 122, nativeBusinessProductBeforeSHA256); e == nil || next != nil {
				t.Fatal("accepted row mutation")
			}
		}
	}
}
func TestNativeBusinessHistoryOverlayExactScope(t *testing.T) {
	base := nativeBusinessHistoryBaseline(t)
	saved := nativeBusinessCloneSources(base)
	raw, err := os.ReadFile("../../docs/tasks/native-category-business/FROZEN-PRODUCTION-OVERLAY-v3.tsv")
	must(t, err)
	next, err := nativeBusinessFrozenOverlay(base, string(raw))
	must(t, err)
	for _, phase := range []struct {
		name, hash string
		entries    map[string]string
	}{{"BEFORE", nativeBusinessHistoryBeforeSHA256, base}, {"AFTER", nativeBusinessHistoryAfterSHA256, next}} {
		published, e := os.ReadFile("../../docs/tasks/native-category-business/FROZEN-" + phase.name + "-v3.tsv")
		must(t, e)
		parsed, e := nativeBusinessParseSources(string(published), 73, phase.hash)
		must(t, e)
		if !reflect.DeepEqual(parsed, phase.entries) {
			t.Fatal("published history drift", phase.name)
		}
		for i := range published {
			bad := append([]byte(nil), published...)
			bad[i] ^= 1
			if result, err := nativeBusinessParseSources(string(bad), 73, phase.hash); err == nil || result != nil {
				t.Fatalf("accepted history %s manifest byte %d", phase.name, i)
			}
		}
	}
	reject := func(bad map[string]string, rows string) {
		t.Helper()
		copy := nativeBusinessCloneSources(bad)
		if result, e := nativeBusinessFrozenOverlay(bad, rows); e == nil || result != nil || !reflect.DeepEqual(copy, bad) {
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
			bad := nativeBusinessCloneSources(base)
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
	extra := nativeBusinessCloneSources(base)
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

func TestNativeBusinessSourceConsumers(t *testing.T) {
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
	if !reflect.DeepEqual(conversions, []string{"nativeBusinessProductHistoricalInput", "entryProductV2HistoricalInput"}) {
		t.Fatal("v3 must independently pass before v2/v1", conversions)
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
	want := []string{"outcomeFrozenOverlay", "dayDetailFrozenOverlay", "daySnapshotFrozenOverlay", "dayReadFrozenOverlay", "nativeWindowFrozenOverlay", "quoteFastFrozenOverlay", "ordinaryServiceFrozenOverlay", "categoryUIFrozenOverlay", "ordinaryEntryFrozenOverlay", "ordinaryEntryCleanupFrozenOverlay", "nativeBusinessFrozenOverlay"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatal("history chain changed", calls)
	}
	for i, st := range body.List {
		if a, ok := st.(*ast.AssignStmt); ok && len(a.Rhs) == 1 {
			if call, ok := a.Rhs[0].(*ast.CallExpr); ok {
				if f, ok := call.Fun.(*ast.Ident); ok && f.Name == "nativeBusinessFrozenOverlay" {
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

// Script scope is independent of product Go/module scope. Both complete smoke
// directories are scanned without an extension filter; build/dependency inputs
// outside those directories are fixed explicitly. No report chooses this scope.
const nativeBusinessScriptManifestSHA256 = "2b1edb1513ab5c18fc823ea3b389e53df9bf6e7597eba8903c374f0fd805cdb8"
const nativeBusinessScriptCount = 38
const nativeBusinessScriptFixedPaths = `.github/workflows/native-category-business.yml
.github/workflows/native-window-smoke.yml
ports/mygo/patches/mygo/native-finalize/0001-native-finalize.patch
ports/mygo/patches/mygo/native-finalize/0002-native-finalize-regression-tests.patch
ports/mygo/patches/mygo/native-finalize/0003-native-menu-lifetime.patch
ports/mygo/patches/mygo/native-finalize/0004-native-menu-regression-tests.patch
ports/mygo/patches/mygo/native-finalize/0005-selected-access.patch
ports/mygo/patches/mygo/native-finalize/0006-selected-access-regression-tests.patch
ports/mygo/patches/mygo/native-finalize/0007-selected-layout-regression-tests.patch
ports/mygo/patches/mygo/native-finalize/NOTICE
ports/mygo/patches/mygo/native-finalize/README.md
ports/mygo/patches/mygo/native-finalize/UPSTREAM-LICENSE
ports/mygo/patches/mygo/native-finalize/manifest.json
ports/mygo/scripts/build
ports/mygo/scripts/ci/requirements.txt
ports/mygo/scripts/cross-build
ports/mygo/scripts/dependency/prepare_dependency.py
ports/mygo/scripts/dependency/test_prepare_dependency.py
ports/mygo/scripts/dependency/verify_selection.py
ports/mygo/scripts/environment
`

var nativeBusinessScriptDirectories = []string{"ports/mygo/scripts/native-smoke", "ports/mygo/scripts/native-window-smoke"}

func nativeBusinessScriptPath(p string) bool {
	if p == "" || path.Clean(p) != p || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\t\r\n\x00") {
		return false
	}
	for _, dir := range nativeBusinessScriptDirectories {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	for _, fixed := range strings.Split(strings.TrimSuffix(nativeBusinessScriptFixedPaths, "\n"), "\n") {
		if p == fixed {
			return true
		}
	}
	return false
}
func nativeBusinessScriptContract(raw string, actual map[string]string) error {
	if nativeBusinessSourceHash(raw) != nativeBusinessScriptManifestSHA256 || !strings.HasSuffix(raw, "\n") {
		return fmt.Errorf("unapproved script manifest bytes")
	}
	expected := map[string]string{}
	last := ""
	for _, row := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 2 || !nativeBusinessValidHash(f[0]) || !nativeBusinessScriptPath(f[1]) || f[1] <= last {
			return fmt.Errorf("invalid script manifest row")
		}
		if _, ok := expected[f[1]]; ok {
			return fmt.Errorf("duplicate script path")
		}
		expected[f[1]] = f[0]
		last = f[1]
	}
	if len(expected) != nativeBusinessScriptCount || !reflect.DeepEqual(expected, actual) {
		return fmt.Errorf("full script source set mismatch")
	}
	return nil
}
func nativeBusinessScanScripts(root string) (map[string]string, error) {
	result := map[string]string{}
	// Check every ancestor as well as leaf: WalkDir never follows a linked child,
	// but an unchecked parent of a fixed path could otherwise redirect its read.
	plainParents := func(relative string) error {
		for p := relative; ; p = filepath.Dir(p) {
			info, err := os.Lstat(filepath.Join(root, p))
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("script parent is not plain: %s", p)
			}
			if p == "." {
				return nil
			}
		}
	}
	read := func(relative string) error {
		if !nativeBusinessScriptPath(filepath.ToSlash(relative)) {
			return fmt.Errorf("unexpected script source path")
		}
		if err := plainParents(filepath.Dir(relative)); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(root, relative))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("script source is not plain: %s", relative)
		}
		key := filepath.ToSlash(relative)
		if _, ok := result[key]; ok {
			return fmt.Errorf("duplicate script scan path")
		}
		b, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			return err
		}
		result[key] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	}
	for _, dir := range nativeBusinessScriptDirectories {
		if err := plainParents(filepath.FromSlash(dir)); err != nil {
			return nil, err
		}
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(p string, entry fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("script source link: %s", p)
			}
			if entry.IsDir() {
				return nil
			}
			relative, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			return read(relative)
		})
		if err != nil {
			return nil, err
		}
	}
	for _, p := range strings.Split(strings.TrimSuffix(nativeBusinessScriptFixedPaths, "\n"), "\n") {
		if err := read(filepath.FromSlash(p)); err != nil {
			return nil, err
		}
	}
	return result, nil
}
func TestNativeBusinessScriptSourceContract(t *testing.T) {
	raw, err := os.ReadFile("../../docs/tasks/native-category-business/SCRIPT-SOURCES-v1.tsv")
	must(t, err)
	actual, err := nativeBusinessScanScripts("../../../..")
	must(t, err)
	must(t, nativeBusinessScriptContract(string(raw), actual))
	for i := range raw {
		bad := append([]byte(nil), raw...)
		bad[i] ^= 1
		if nativeBusinessScriptContract(string(bad), actual) == nil {
			t.Fatal("accepted script manifest byte", i)
		}
	}
	reject := func(bad map[string]string) {
		t.Helper()
		saved := nativeBusinessCloneSources(bad)
		if nativeBusinessScriptContract(string(raw), bad) == nil || !reflect.DeepEqual(saved, bad) {
			t.Fatal("accepted/mutated script set")
		}
	}
	for p := range actual {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := nativeBusinessCloneSources(actual)
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
	for _, dir := range nativeBusinessScriptDirectories {
		bad := nativeBusinessCloneSources(actual)
		bad[dir+"/unlisted.no-extension"] = strings.Repeat("a", 64)
		reject(bad)
	}
	reject(nil)
	reject(map[string]string{})
	for _, invalid := range []string{"", string(raw) + string(raw), strings.TrimSuffix(string(raw), "\n"), strings.ReplaceAll(string(raw), "\n", "\r\n"), string(raw) + "extra\n"} {
		if nativeBusinessScriptContract(invalid, actual) == nil {
			t.Fatal("accepted altered script manifest")
		}
	}
}
func TestNativeBusinessScriptScannerRejectsUndeclaredFilesAndLinks(t *testing.T) {
	root := t.TempDir()
	actual, err := nativeBusinessScanScripts("../../../..")
	must(t, err)
	for p := range actual {
		destination := filepath.Join(root, filepath.FromSlash(p))
		must(t, os.MkdirAll(filepath.Dir(destination), 0700))
		b, e := os.ReadFile(filepath.Join("../../../..", filepath.FromSlash(p)))
		must(t, e)
		must(t, os.WriteFile(destination, b, 0600))
	}
	raw, err := os.ReadFile("../../docs/tasks/native-category-business/SCRIPT-SOURCES-v1.tsv")
	must(t, err)
	scanned, err := nativeBusinessScanScripts(root)
	must(t, err)
	must(t, nativeBusinessScriptContract(string(raw), scanned))
	for _, dir := range nativeBusinessScriptDirectories {
		extra := filepath.Join(root, filepath.FromSlash(dir), "nested", "unlisted.no-extension")
		must(t, os.MkdirAll(filepath.Dir(extra), 0700))
		must(t, os.WriteFile(extra, []byte("undeclared"), 0600))
		scanned, e := nativeBusinessScanScripts(root)
		must(t, e)
		if nativeBusinessScriptContract(string(raw), scanned) == nil {
			t.Fatal("ignored undeclared file")
		}
		must(t, os.Remove(extra))
	}
	missing := filepath.Join(root, "ports/mygo/scripts/build")
	must(t, os.Remove(missing))
	if _, e := nativeBusinessScanScripts(root); e == nil {
		t.Fatal("ignored missing fixed build script")
	}
	must(t, os.Mkdir(missing, 0700))
	if _, e := nativeBusinessScanScripts(root); e == nil {
		t.Fatal("accepted directory as build script")
	}
	must(t, os.Remove(missing))
	target := filepath.Join(root, "outside-scope")
	must(t, os.WriteFile(target, []byte("outside"), 0600))
	if e := os.Symlink(target, missing); e != nil {
		t.Log("symlink creation unavailable; byte/set/missing/directory checks still ran:", e)
		return
	}
	if _, e := nativeBusinessScanScripts(root); e == nil {
		t.Fatal("accepted fixed-path symlink")
	}
	must(t, os.Remove(missing))
	must(t, os.WriteFile(missing, []byte("restored"), 0600))
	link := filepath.Join(root, "ports/mygo/scripts/native-window-smoke/linked-file")
	must(t, os.Symlink(target, link))
	if _, e := nativeBusinessScanScripts(root); e == nil {
		t.Fatal("accepted scanned symlink")
	}
	must(t, os.Remove(link))
	parent := filepath.Join(root, "ports/mygo/scripts/native-window-smoke/linked-directory")
	must(t, os.Symlink(filepath.Dir(target), parent))
	if _, e := nativeBusinessScanScripts(root); e == nil {
		t.Fatal("accepted scanned directory symlink")
	}
}
