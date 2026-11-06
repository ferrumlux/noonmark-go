// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Two reviewed UI-only deltas, after the existing Service overlay. This is not
// a general manifest replacement: even another valid SHA256 is unapproved.
var dayDetailFrozenChanges = map[string][2]string{
	"ports/mygo/runtime_day.go":   {"68d1e5b0d7a206650868df65cbdc0c99d794cfbb90364756b3bc58f7efce0f50", "e761a9e269c4c07ce0334b0fc011d1515c8c226675de561a92315571810c47af"},
	"ports/mygo/visual_tokens.go": {"214046c2f7917a65ec08888e348c97e8fb0dcfc84a0b03587f4fe19ed6aa6414", "5e29c91578efa267ed790913dd5027660104a8a4e3c77b1d2ed91a8a65a980f8"},
}

func dayDetailFrozenOverlay(expected map[string]string, overlay string) (map[string]string, error) {
	rows := strings.Split(strings.TrimSpace(overlay), "\n")
	if len(rows) != 2 {
		return nil, fmt.Errorf("day detail overlay must contain exactly two reviewed rows")
	}
	seen := map[string]bool{}
	for _, row := range rows {
		p := strings.Split(row, "\t")
		if len(p) != 3 {
			return nil, fmt.Errorf("invalid day detail overlay shape")
		}
		binding, ok := dayDetailFrozenChanges[p[2]]
		if !ok || seen[p[2]] || p[0] != binding[0] || p[1] != binding[1] || expected[p[2]] != binding[0] {
			return nil, fmt.Errorf("unapproved day detail overlay binding")
		}
		seen[p[2]] = true
	}
	if len(seen) != len(dayDetailFrozenChanges) {
		return nil, fmt.Errorf("incomplete day detail overlay")
	}
	next := make(map[string]string, len(expected))
	for path, hash := range expected {
		next[path] = hash
	}
	for path, binding := range dayDetailFrozenChanges {
		next[path] = binding[1]
	}
	return next, nil
}

func TestDayDetailFrozenOverlayRejectsUnapprovedChanges(t *testing.T) {
	base, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, e)
	service, e := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, e)
	expected, e := outcomeFrozenOverlay(string(base), string(service))
	must(t, e)
	overlay, e := os.ReadFile("../../docs/tasks/align-day-detail-header/FROZEN-PRODUCTION-OVERLAY-v1.tsv")
	must(t, e)
	clone := func(in map[string]string) map[string]string {
		out := make(map[string]string, len(in))
		for p, h := range in {
			out[p] = h
		}
		return out
	}
	before := clone(expected)
	next, e := dayDetailFrozenOverlay(expected, string(overlay))
	must(t, e)
	if !reflect.DeepEqual(before, expected) {
		t.Fatal("input manifest mutated")
	}
	if len(next) != len(expected) {
		t.Fatal("overlay lost/added paths")
	}
	for p, h := range expected {
		if binding, ok := dayDetailFrozenChanges[p]; ok {
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
				if _, e := dayDetailFrozenOverlay(bad, string(overlay)); e == nil {
					t.Fatal("unapproved baseline accepted")
				}
			})
		}
	}
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			if _, e := dayDetailFrozenOverlay(expected, v); e == nil {
				t.Fatal("unapproved overlay accepted")
			}
			if !reflect.DeepEqual(before, expected) {
				t.Fatal("failure mutated input")
			}
		})
	}
}
