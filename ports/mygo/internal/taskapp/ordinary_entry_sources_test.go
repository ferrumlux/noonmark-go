// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const entryProductManifestSHA256 = "27db681ae65bec2435e8da59d5099551b2e5be7c0fb31636339260cc08c44d20"
const entryProductChangesSHA256 = "b4e2ddab5d63c3466d92058dae4deea061f90d8c40174fa9cb59564f5087ee60"

func entryProductSourceContract(manifest, changes string, actual map[string]string) error {
	if fmt.Sprintf("%x", sha256.Sum256([]byte(manifest))) != entryProductManifestSHA256 || fmt.Sprintf("%x", sha256.Sum256([]byte(changes))) != entryProductChangesSHA256 {
		return fmt.Errorf("unapproved entry source contract")
	}
	paths := make([]string, 0, len(actual))
	for path := range actual {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var rows strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&rows, "%s\t%s\n", actual[path], path)
	}
	if rows.String() != manifest {
		return fmt.Errorf("undeclared product source change")
	}
	return nil
}

func TestOrdinaryEntryProductSourceContract(t *testing.T) {
	manifest, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/PRODUCT-SOURCES-v1.tsv")
	must(t, err)
	changes, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/ENTRY-SOURCE-CHANGES-v1.tsv")
	must(t, err)

	beforeBytes, err := os.ReadFile("../../docs/tasks/connect-ordinary-entry/PRODUCT-SOURCES-BEFORE-v1.tsv")
	must(t, err)
	if fmt.Sprintf("%x", sha256.Sum256(beforeBytes)) != "58513f94c4b2ddba92fce23125b41f0dff37d0f4bf29ac3883675515b10f286c" {
		t.Fatal("unapproved entry baseline source manifest")
	}
	beforeSources := map[string]string{}
	for _, row := range strings.Split(strings.TrimSuffix(string(beforeBytes), "\n"), "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) != 2 {
			t.Fatal("before shape")
		}
		beforeSources[fields[1]] = fields[0]
	}
	for _, row := range strings.Split(strings.TrimSuffix(string(changes), "\n"), "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) != 3 {
			t.Fatal("change shape")
		}
		old, exists := beforeSources[fields[2]]
		if fields[0] == "absent" {
			if exists {
				t.Fatal("new source already present")
			}
		} else if !exists || old != fields[0] {
			t.Fatal("old source hash mismatch")
		}
		beforeSources[fields[2]] = fields[1]
	}
	must(t, entryProductSourceContract(string(manifest), string(changes), beforeSources))
	actual := map[string]string{}
	err = filepath.WalkDir("../..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !(strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")) && entry.Name() != "go.mod" && entry.Name() != "go.sum" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink: %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel("../..", path)
		if err != nil {
			return err
		}
		actual["ports/mygo/"+filepath.ToSlash(relative)] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	must(t, err)
	actual = runtimeAdmissionProductHistoricalInput(t, actual)
	actual = nativeBusinessProductHistoricalInput(t, actual)
	actual = entryProductV2HistoricalInput(t, actual)
	must(t, entryProductSourceContract(string(manifest), string(changes), actual))
	clone := func() map[string]string {
		result := map[string]string{}
		for p, h := range actual {
			result[p] = h
		}
		return result
	}
	for _, input := range []string{"manifest", "changes"} {
		original := manifest
		if input == "changes" {
			original = changes
		}
		for i := range original {
			b := append([]byte(nil), original...)
			b[i] ^= 1
			m, c := string(manifest), string(changes)
			if input == "manifest" {
				m = string(b)
			} else {
				c = string(b)
			}
			if entryProductSourceContract(m, c, actual) == nil {
				t.Fatalf("accepted %s byte %d", input, i)
			}
		}
	}
	for path := range actual {
		for _, kind := range []string{"hash", "missing", "alias"} {
			bad := clone()
			switch kind {
			case "hash":
				bad[path] = strings.Repeat("a", 64)
			case "missing":
				delete(bad, path)
			case "alias":
				delete(bad, path)
				bad[path+"/../alias"] = actual[path]
			}
			before := map[string]string{}
			for p, h := range bad {
				before[p] = h
			}
			if entryProductSourceContract(string(manifest), string(changes), bad) == nil || !reflect.DeepEqual(before, bad) {
				t.Fatalf("accepted/mutated %s %s", kind, path)
			}
		}
	}
	extra := clone()
	extra["ports/mygo/undeclared.go"] = strings.Repeat("b", 64)
	if entryProductSourceContract(string(manifest), string(changes), extra) == nil {
		t.Fatal("accepted undeclared product")
	}
	for _, invalid := range []string{"", string(manifest) + string(manifest), string(manifest) + "extra\n", strings.Replace(string(manifest), "\n", "\r\n", 1)} {
		if entryProductSourceContract(invalid, string(changes), actual) == nil {
			t.Fatal("accepted altered manifest")
		}
	}
	t.Logf("bound %d product paths, rejected every manifest/change byte mutation and every product path/hash mutation", len(actual))
}
