// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// This test-only overlay admits exactly the reviewed development window hook.
// It does not replace the historical manifest or authorize any history wiring.
const nativeWindowFrozenPath = "ports/mygo/main.go"
const nativeWindowFrozenOld = "a678fc92870f94e841520ad31ddb4cfa9099ee7ed4508b83f8b8791d2a33f6c5"
const nativeWindowFrozenNew = "23bd5f30645a75fb9f835307f03e0b116ad8d3f995ff108c9081a1cdfc629849"

func nativeWindowFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	want := nativeWindowFrozenOld + "\t" + nativeWindowFrozenNew + "\t" + nativeWindowFrozenPath + "\n"
	if overlay != want || expected[nativeWindowFrozenPath] != nativeWindowFrozenOld {
		return nil, fmt.Errorf("unapproved native window overlay binding")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	next[nativeWindowFrozenPath] = nativeWindowFrozenNew
	return next, nil
}

func TestNativeWindowFrozenOverlayRejectsUnapprovedChanges(t *testing.T) {
	rows, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, e)
	service, e := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, e)
	expected, e := outcomeFrozenOverlay(string(rows), string(service))
	must(t, e)
	for _, step := range []struct {
		path  string
		apply func(map[string]string, string) (map[string]string, error)
	}{
		{"align-day-detail-header", dayDetailFrozenOverlay},
		{"diagnose-root-test-timeout", daySnapshotFrozenOverlay},
		{"optimize-day-todo-read", dayReadFrozenOverlay},
	} {
		b, err := os.ReadFile("../../docs/tasks/" + step.path + "/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
		must(t, err)
		expected, err = step.apply(expected, string(b))
		must(t, err)
	}
	b, e := os.ReadFile("../../docs/tasks/verify-native-window/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	clone := func(in map[string]string) map[string]string {
		out := make(map[string]string, len(in))
		for path, hash := range in {
			out[path] = hash
		}
		return out
	}
	before := clone(expected)
	next, e := nativeWindowFrozenOverlay(expected, string(b))
	must(t, e)
	if !reflect.DeepEqual(before, expected) || len(next) != len(expected) {
		t.Fatal("overlay changed the input or path set")
	}
	for path, hash := range expected {
		want := hash
		if path == nativeWindowFrozenPath {
			want = nativeWindowFrozenNew
		}
		if next[path] != want {
			t.Fatal("unapproved path changed", path)
		}
	}
	variants := map[string]string{
		"empty": "", "whitespace": " \n", "duplicate": string(b) + string(b),
		"extra field": strings.TrimSuffix(string(b), "\n") + "\textra\n",
		"extra row":   string(b) + "unapproved\n", "missing LF": strings.TrimSuffix(string(b), "\n"),
		"CRLF":           strings.ReplaceAll(string(b), "\n", "\r\n"),
		"old hash":       strings.Replace(string(b), nativeWindowFrozenOld, strings.Repeat("a", 64), 1),
		"new hash":       strings.Replace(string(b), nativeWindowFrozenNew, strings.Repeat("b", 64), 1),
		"malformed hash": strings.Replace(string(b), nativeWindowFrozenNew, "bad", 1),
		"no advance":     strings.Replace(string(b), nativeWindowFrozenNew, nativeWindowFrozenOld, 1),
		"different path": strings.Replace(string(b), nativeWindowFrozenPath, "ports/mygo/runtime_title.go", 1),
		"path alias":     strings.Replace(string(b), nativeWindowFrozenPath, "ports/mygo/../mygo/main.go", 1),
	}
	for name, value := range variants {
		t.Run(name, func(t *testing.T) {
			if _, err := nativeWindowFrozenOverlay(expected, value); err == nil {
				t.Fatal("unapproved overlay accepted")
			}
			if !reflect.DeepEqual(before, expected) {
				t.Fatal("failure mutated baseline")
			}
		})
	}
	for _, state := range []string{"nil", "missing", "wrong", "already advanced"} {
		t.Run("baseline "+state, func(t *testing.T) {
			bad := clone(expected)
			switch state {
			case "nil":
				bad = nil
			case "missing":
				delete(bad, nativeWindowFrozenPath)
			case "wrong":
				bad[nativeWindowFrozenPath] = strings.Repeat("c", 64)
			case "already advanced":
				bad[nativeWindowFrozenPath] = nativeWindowFrozenNew
			}
			before := clone(bad)
			if _, err := nativeWindowFrozenOverlay(bad, string(b)); err == nil {
				t.Fatal("unapproved baseline accepted")
			}
			if bad != nil && !reflect.DeepEqual(before, bad) {
				t.Fatal("failure mutated bad baseline")
			}
		})
	}
}
