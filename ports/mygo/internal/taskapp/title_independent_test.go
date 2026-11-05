// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Independent review regressions use only synthetic task graphs and isolated
// development stores created by the test helpers.
func TestTitleIndependentReplayRejectsOtherEditableMutations(t *testing.T) {
	prior, b, c := titleHistoryFixture(t)
	valid := titleCandidate(t, b, c)
	attacks := map[string]func(*core.TaskGraph){
		"chain notes": func(g *core.TaskGraph) {
			g.Chains[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(titleID(99)), Body: "injected", CreatedAt: titleAt(10), UpdatedAt: titleAt(10)}}
		},
		"pending description": func(g *core.TaskGraph) { g.Traces[1].DescriptionText = titlePtr("injected") },
		"pending priority":    func(g *core.TaskGraph) { g.Traces[1].Priority = 9 },
		"pending clock":       func(g *core.TaskGraph) { g.Traces[1].ContentUpdatedAt = titleAt(11) },
		"chain clock":         func(g *core.TaskGraph) { g.Chains[0].UpdatedAt = titleAt(11) },
		"old supersededAt":    func(g *core.TaskGraph) { g.Definitions[0].SupersededAt = nil },
	}
	for name, attack := range attacks {
		t.Run(name, func(t *testing.T) {
			a := valid.Clone()
			attack(&a)
			if _, err := captureTitleHistory(prior, b, a, c); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
func TestTitleIndependentCanonicalNoopVerifiesHistoricalPrefix(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "input", true: "rename"}[rename], func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("caf\u00e9"))
			before := s.Snapshot()
			rev := s.disk.revision
			must(t, os.WriteFile(filepath.Join(path, "rev-0000000001", "COMMITTED"), []byte("tampered\n"), 0600))
			edit := s.SaveTaskTitleInput
			if rename {
				edit = s.RenameTaskTitle
			}
			if err := edit(before.Chains[0].ID, "cafe\u0301"); err == nil {
				t.Fatal("corrupt prefix accepted")
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
				t.Fatal("failed no-op published")
			}
		})
	}
}
