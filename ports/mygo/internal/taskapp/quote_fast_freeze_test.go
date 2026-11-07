// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// This test-only overlay admits exactly the reviewed escape-free quote fast path.
// It does not replace the historical manifest or authorize any history wiring.
const quoteFastFrozenPath = "ports/mygo/domain-codec/canonical/canonical.go"
const quoteFastFrozenOld = "6076f80103b6ea549df109a45722138918a42dd11c6c69757adbfbcb5cc100e8"
const quoteFastFrozenNew = "165f1be6668310a35899652ddb364649db5c257f06cb4e60c7d3fd19fcfb3d4a"

func quoteFastFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	want := quoteFastFrozenOld + "\t" + quoteFastFrozenNew + "\t" + quoteFastFrozenPath + "\n"
	if overlay != want || expected[quoteFastFrozenPath] != quoteFastFrozenOld {
		return nil, fmt.Errorf("unapproved canonical quote overlay binding")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	next[quoteFastFrozenPath] = quoteFastFrozenNew
	return next, nil
}

func TestQuoteFastFrozenOverlayRejectsUnapprovedChanges(t *testing.T) {
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
		{"verify-native-window", nativeWindowFrozenOverlay},
	} {
		b, err := os.ReadFile("../../docs/tasks/" + step.path + "/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
		must(t, err)
		expected, err = step.apply(expected, string(b))
		must(t, err)
	}
	b, e := os.ReadFile("../../docs/tasks/optimize-canonical-quote/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	clone := func(in map[string]string) map[string]string {
		out := make(map[string]string, len(in))
		for path, hash := range in {
			out[path] = hash
		}
		return out
	}
	before := clone(expected)
	next, e := quoteFastFrozenOverlay(expected, string(b))
	must(t, e)
	if !reflect.DeepEqual(before, expected) || len(next) != len(expected) {
		t.Fatal("overlay changed the input or path set")
	}
	for path, hash := range expected {
		want := hash
		if path == quoteFastFrozenPath {
			want = quoteFastFrozenNew
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
		"old hash":       strings.Replace(string(b), quoteFastFrozenOld, strings.Repeat("a", 64), 1),
		"new hash":       strings.Replace(string(b), quoteFastFrozenNew, strings.Repeat("b", 64), 1),
		"malformed hash": strings.Replace(string(b), quoteFastFrozenNew, "bad", 1),
		"no advance":     strings.Replace(string(b), quoteFastFrozenNew, quoteFastFrozenOld, 1),
		"different path": strings.Replace(string(b), quoteFastFrozenPath, "ports/mygo/runtime_title.go", 1),
		"path alias":     strings.Replace(string(b), quoteFastFrozenPath, "ports/mygo/../mygo/domain-codec/canonical/canonical.go", 1),
	}
	for name, value := range variants {
		t.Run(name, func(t *testing.T) {
			if _, err := quoteFastFrozenOverlay(expected, value); err == nil {
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
				delete(bad, quoteFastFrozenPath)
			case "wrong":
				bad[quoteFastFrozenPath] = strings.Repeat("c", 64)
			case "already advanced":
				bad[quoteFastFrozenPath] = quoteFastFrozenNew
			}
			before := clone(bad)
			if _, err := quoteFastFrozenOverlay(bad, string(b)); err == nil {
				t.Fatal("unapproved baseline accepted")
			}
			if bad != nil && !reflect.DeepEqual(before, bad) {
				t.Fatal("failure mutated bad baseline")
			}
		})
	}
}
