// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

// This versioned overlay does not replace the historical freeze. Exactly the
// seven approved Service files may advance, and each must bind its old hash.
func outcomeFrozenOverlay(baseline, overlay string) (map[string]string, error) {
	allowed := map[string]bool{}
	for _, name := range []string{"service", "store", "manual_progress", "title", "subtask", "carryover", "return"} {
		allowed["ports/mygo/internal/taskapp/"+name+".go"] = true
	}
	validHash := func(s string) bool {
		b, e := hex.DecodeString(s)
		return e == nil && len(b) == 32 && strings.ToLower(s) == s
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(baseline), "\n") {
		p := strings.Split(line, "\t")
		if len(p) != 2 || !validHash(p[0]) || expected[p[1]] != "" {
			return nil, fmt.Errorf("invalid baseline")
		}
		expected[p[1]] = p[0]
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(overlay), "\n") {
		p := strings.Split(line, "\t")
		if len(p) != 3 {
			return nil, fmt.Errorf("invalid overlay shape")
		}
		old, next, path := p[0], p[1], p[2]
		if !allowed[path] || seen[path] || expected[path] != old || !validHash(next) || old == next {
			return nil, fmt.Errorf("invalid overlay binding")
		}
		seen[path] = true
		expected[path] = next
	}
	if len(seen) != len(allowed) {
		return nil, fmt.Errorf("incomplete overlay")
	}
	return expected, nil
}
func TestOutcomeFrozenOverlayRejectsUnapprovedChanges(t *testing.T) {
	base, e := os.ReadFile("../../docs/tasks/audit-history-edits/FROZEN-PRODUCTION-SHA256.tsv")
	must(t, e)
	overlay, e := os.ReadFile("../../docs/tasks/service-typed-outcome/FROZEN-PRODUCTION-OVERLAY.tsv")
	must(t, e)
	rows := strings.Split(strings.TrimSpace(string(overlay)), "\n")
	_, e = outcomeFrozenOverlay(string(base), string(overlay))
	must(t, e)
	changed := strings.Split(rows[0], "\t")
	variants := map[string]string{"missing": strings.Join(rows[1:], "\n"), "duplicate": string(overlay) + rows[0] + "\n", "extra": string(overlay) + strings.Repeat("a", 64) + "\t" + strings.Repeat("b", 64) + "\tports/mygo/domain-core/core/kernel.go\n", "badshape": "broken"}
	old := append([]string{}, changed...)
	old[0] = strings.Repeat("f", 64)
	variants["old hash mismatch"] = strings.Join(append([]string{strings.Join(old, "\t")}, rows[1:]...), "\n")
	next := append([]string{}, changed...)
	next[1] = "bad"
	variants["invalid new hash"] = strings.Join(append([]string{strings.Join(next, "\t")}, rows[1:]...), "\n")
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			if _, e := outcomeFrozenOverlay(string(base), v); e == nil {
				t.Fatal("invalid overlay accepted")
			}
		})
	}
}
func TestOutcomeNewProductionFrozen(t *testing.T) {
	b, e := os.ReadFile("outcome.go")
	must(t, e)
	hash, e := os.ReadFile("../../docs/tasks/service-typed-outcome/OUTCOME.sha256")
	must(t, e)
	if fmt.Sprintf("%x", sha256.Sum256(b)) != strings.TrimSpace(string(hash)) {
		t.Fatal("outcome production changed")
	}
}
