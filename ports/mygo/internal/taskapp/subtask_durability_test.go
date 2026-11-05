// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Rename's ack matrix lives in subtask_test.go. Exercise every other write
// through the same persistence phases, including generated identities.
func TestSubtaskAllCommandsDurableFaultMatrix(t *testing.T) {
	for _, op := range []string{"planned-add", "planned-remove", "planned-difficulty", "actual-add", "actual-delete", "actual-difficulty", "actual-complete", "actual-undo", "actual-abandon"} {
		for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
			t.Run(op+"/"+phase, func(t *testing.T) {
				planned := strings.HasPrefix(op, "planned-")
				s, path, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
				if op == "actual-undo" {
					must(t, s.SetSubtaskCompleted(ch, tr, a, true))
				}
				before, wire, rev := s.Snapshot(), s.snapshot, s.disk.revision
				run := func() error {
					switch op {
					case "planned-add":
						id, e := s.AddPlannedSubtask(ch, def, "新计划", core.DifficultyHard)
						if e != nil && id != "" {
							t.Error("unconfirmed planned identity")
						}
						return e
					case "planned-remove":
						return s.RemovePlannedSubtask(ch, def, p)
					case "planned-difficulty":
						return s.SetPlannedSubtaskDifficulty(ch, def, p, core.DifficultyHard)
					case "actual-add":
						id, e := s.AddSubtask(ch, tr, "新实际项", core.DifficultyHard)
						if e != nil && id != "" {
							t.Error("unconfirmed actual identity")
						}
						return e
					case "actual-delete":
						return s.DeleteSubtask(ch, tr, a)
					case "actual-difficulty":
						return s.SetSubtaskDifficulty(ch, tr, a, core.DifficultyHard)
					case "actual-complete":
						return s.SetSubtaskCompleted(ch, tr, a, true)
					case "actual-undo":
						return s.SetSubtaskCompleted(ch, tr, a, false)
					default:
						return s.AbandonSubtask(ch, tr, a)
					}
				}
				s.disk.fault = func(at string) error {
					if at == phase {
						return ErrIncomplete
					}
					return nil
				}
				if e := run(); !errors.Is(e, ErrIncomplete) {
					t.Fatal("fault did not fail closed", e)
				}
				if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || s.disk.revision != rev {
					t.Fatal("uncertain write published")
				}
				s.disk.fault = nil
				if phase != "before-reserve" {
					if e := run(); !errors.Is(e, ErrIncomplete) {
						t.Fatal("blocked command retried", e)
					}
				}
				must(t, s.Close())
				r, e := Open(path, testDay)
				if phase == "after-reserve" || phase == "before-commit" {
					if !errors.Is(e, ErrIncomplete) {
						if r != nil {
							r.Close()
						}
						t.Fatal("partial session reopened", e)
					}
					return
				}
				must(t, e)
				defer r.Close()
				if phase == "before-reserve" {
					if !reflect.DeepEqual(before, r.Snapshot()) || r.disk.revision != rev {
						t.Fatal("unreserved write changed disk")
					}
					return
				}
				if r.disk.revision != rev+1 || reflect.DeepEqual(before, r.Snapshot()) {
					t.Fatal("committed transaction disappeared")
				}
				g := r.Snapshot()
				switch op {
				case "planned-add":
					if len(g.Definitions[0].PlannedSubtasks) != 2 || g.Definitions[0].PlannedSubtasks[1].Title != "新计划" {
						t.Fatal("wrong planned add")
					}
				case "planned-remove":
					if len(g.Definitions[0].PlannedSubtasks) != 0 {
						t.Fatal("wrong planned remove")
					}
				case "planned-difficulty":
					if g.Definitions[0].PlannedSubtasks[0].Difficulty != core.DifficultyHard {
						t.Fatal("wrong planned difficulty")
					}
				case "actual-add":
					if len(g.Subtasks) != 2 || g.Subtasks[1].Title != "新实际项" {
						t.Fatal("wrong actual add")
					}
				case "actual-delete":
					if g.Subtasks[0].Status != core.SubtaskCancelledDraft || g.Subtasks[0].DraftCancellationID == nil {
						t.Fatal("lost tombstone")
					}
				case "actual-difficulty":
					if g.Subtasks[0].Difficulty != core.DifficultyHard {
						t.Fatal("wrong difficulty")
					}
				case "actual-complete":
					if g.Subtasks[0].Status != core.SubtaskCompleted || g.Subtasks[0].CompletedAt == nil {
						t.Fatal("lost completion")
					}
				case "actual-undo":
					if g.Subtasks[0].Status != core.SubtaskPending || g.Subtasks[0].CompletedAt != nil {
						t.Fatal("lost undo")
					}
				case "actual-abandon":
					if g.Subtasks[0].Status != core.SubtaskAbandoned || g.Subtasks[0].SettledAt == nil {
						t.Fatal("lost abandon")
					}
				}
			})
		}
	}
}

func TestSubtaskConcurrentAcknowledgmentsAndMonotonicClock(t *testing.T) {
	for _, planned := range []bool{false, true} {
		name := "actual"
		if planned {
			name = "planned"
		}
		t.Run(name, func(t *testing.T) {
			s, path, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
			before := s.Snapshot()
			// A wall clock before all existing data must not rewind child or parent facts.
			s.clock = func() time.Time { return time.Unix(978307201, 0) }
			rev := s.disk.revision
			var wg sync.WaitGroup
			for _, title := range []string{"第一窗口", "第二窗口", "第三窗口", "第四窗口"} {
				wg.Add(1)
				go func(title string) {
					defer wg.Done()
					var got string
					var e error
					if planned {
						got, e = s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, title)
					} else {
						got, e = s.SaveSubtaskTitleAcknowledged(ch, tr, a, title)
					}
					if e != nil || got != title {
						t.Errorf("crossed acknowledgment: want %q got %q error %v", title, got, e)
					}
				}(title)
			}
			wg.Wait()
			after := s.Snapshot()
			if s.disk.revision != rev+4 {
				t.Fatal("lost serial write")
			}
			if planned {
				if !before.Definitions[0].ContentUpdatedAt.Before(after.Definitions[0].ContentUpdatedAt) || !before.Chains[0].UpdatedAt.Before(after.Chains[0].UpdatedAt) {
					t.Fatal("planned clock went backwards")
				}
			} else {
				if !before.Subtasks[0].UpdatedAt.Before(after.Subtasks[0].UpdatedAt) {
					t.Fatal("child clock went backwards")
				}
				if !reflect.DeepEqual(before.Definitions, after.Definitions) || !reflect.DeepEqual(before.Chains, after.Chains) || !reflect.DeepEqual(before.Traces, after.Traces) {
					t.Fatal("child touched parent clocks")
				}
			}
			must(t, s.Close())
			r, e := Open(path, testDay)
			must(t, e)
			defer r.Close()
			if !reflect.DeepEqual(after, r.Snapshot()) {
				t.Fatal("concurrent edits changed on reopen")
			}
		})
	}
}
