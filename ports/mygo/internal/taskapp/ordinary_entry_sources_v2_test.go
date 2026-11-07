// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Exact, test-only authorization for the single reviewed ordinary entry cleanup deltas.
// The complete incoming frozen set is bound, not only the single changed entries.
const entryProductV2BeforeSHA256 = "27db681ae65bec2435e8da59d5099551b2e5be7c0fb31636339260cc08c44d20"
const entryProductV2Rows = `2ff3666a303fa97c6d91861523af2f896152d4a57f99dd9000cc227fbb63de2a	c98d2afcbf81472783aca205a796febb51405d754d1d8bedc1ac2a222b082878	ports/mygo/main.go
6686cb2e2c9fd2f8f5a23d31d021c23b580b1b8640df08302c9e0e4e45df12fc	b6bc3fae51a38f6cb84c53355727d36488e7b22e564ab13a5dfc433be8d965a7	ports/mygo/runtime_category.go
`

func entryProductV2Overlay(expected map[string]string, overlay string) (map[string]string, error) {
	if overlay != entryProductV2Rows {
		return nil, fmt.Errorf("unapproved ordinary entry cleanup overlay")
	}
	if len(expected) != 120 {
		return nil, fmt.Errorf("unapproved ordinary entry cleanup frozen path count")
	}
	paths := make([]string, 0, len(expected))
	for path, hash := range expected {
		if strings.ContainsAny(path, "\t\r\n") || len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return nil, fmt.Errorf("invalid ordinary entry cleanup frozen entry")
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var baseline strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&baseline, "%s\t%s\n", expected[path], path)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(baseline.String()))) != entryProductV2BeforeSHA256 {
		return nil, fmt.Errorf("unapproved ordinary entry cleanup incoming frozen scope")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	for _, row := range strings.Split(strings.TrimSuffix(entryProductV2Rows, "\n"), "\n") {
		fields := strings.Split(row, "\t")
		next[fields[2]] = fields[1]
	}
	return next, nil
}

const entryProductV2ManifestSHA256 = "81a93f13d6d3e8d70c27f3bdd43fb3a87627c171c733d47651695f7b46943cec"

// Preserve the complete v1 contract by reconstructing its exact input only
// after the independent v2 transition and actual full v2 source set pass.
func entryProductV2HistoricalInput(t *testing.T, actual map[string]string) map[string]string {
	t.Helper()
	dir := "../../docs/tasks/connect-ordinary-entry/"
	before, err := os.ReadFile(dir + "PRODUCT-SOURCES-v1.tsv")
	must(t, err)
	changes, err := os.ReadFile(dir + "ENTRY-SOURCE-CHANGES-v2.tsv")
	must(t, err)
	after, err := os.ReadFile(dir + "PRODUCT-SOURCES-v2.tsv")
	must(t, err)
	if fmt.Sprintf("%x", sha256.Sum256(after)) != entryProductV2ManifestSHA256 {
		t.Fatal("v2 manifest drift")
	}
	expected := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(string(before), "\n"), "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 2 {
			t.Fatal("v1 shape")
		}
		expected[f[1]] = f[0]
	}
	next, err := entryProductV2Overlay(expected, string(changes))
	must(t, err)
	if !reflect.DeepEqual(next, actual) {
		t.Fatal("v2 full source set mismatch")
	}
	var rendered strings.Builder
	paths := []string{}
	for p := range next {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&rendered, "%s\t%s\n", next[p], p)
	}
	if rendered.String() != string(after) {
		t.Fatal("v2 published manifest mismatch")
	}
	return expected
}

func TestOrdinaryEntryProductV2ScopeRejectsUnapprovedChanges(t *testing.T) {
	raw, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/PRODUCT-SOURCES-v1.tsv")
	must(t, err)
	base := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		f := strings.Split(row, "\t")
		base[f[1]] = f[0]
	}
	clone := func(in map[string]string) map[string]string {
		if in == nil {
			return nil
		}
		out := map[string]string{}
		for p, h := range in {
			out[p] = h
		}
		return out
	}
	next, err := entryProductV2Overlay(base, entryProductV2Rows)
	must(t, err)
	reject := func(input map[string]string, rows string) {
		t.Helper()
		saved := clone(input)
		result, err := entryProductV2Overlay(input, rows)
		if err == nil || result != nil || !reflect.DeepEqual(saved, input) {
			t.Fatal("accepted/mutated unapproved v2 scope")
		}
	}
	for i := range []byte(entryProductV2Rows) {
		b := []byte(entryProductV2Rows)
		b[i] ^= 1
		reject(base, string(b))
	}
	for _, bad := range []string{"", entryProductV2Rows + entryProductV2Rows, entryProductV2Rows + "extra\n", strings.TrimSuffix(entryProductV2Rows, "\n")} {
		reject(base, bad)
	}
	for p := range base {
		for _, kind := range []string{"missing", "hash", "alias"} {
			bad := clone(base)
			switch kind {
			case "missing":
				delete(bad, p)
			case "hash":
				bad[p] = strings.Repeat("f", 64)
			case "alias":
				delete(bad, p)
				bad[p+"/../alias"] = base[p]
			}
			reject(bad, entryProductV2Rows)
		}
	}
	reject(nil, entryProductV2Rows)
	reject(map[string]string{}, entryProductV2Rows)
	reject(next, entryProductV2Rows)
	for _, row := range strings.Split(strings.TrimSuffix(entryProductV2Rows, "\n"), "\n") {
		f := strings.Split(row, "\t")
		bad := clone(base)
		bad[f[2]] = f[1]
		reject(bad, entryProductV2Rows)
		reject(base, strings.Replace(entryProductV2Rows, row+"\n", "", 1))
		reject(base, entryProductV2Rows+row+"\n")
	}
	bad := clone(base)
	bad["ports/mygo/undeclared.go"] = strings.Repeat("a", 64)
	reject(bad, entryProductV2Rows)
	saved := clone(base)
	next["ports/mygo/main.go"] = "changed"
	if !reflect.DeepEqual(base, saved) {
		t.Fatal("v2 result aliases input")
	}
	t.Logf("rejected %d overlay byte mutations and %d baseline path mutations", len(entryProductV2Rows), len(base)*3)
}
