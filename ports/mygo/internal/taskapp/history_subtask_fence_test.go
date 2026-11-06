// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

// Only direct fence tests copy the proof holder to inject canonical corruption.
// These values are NEVER passed to an audit, builder, or inspector as seals.
func sfPolluted(p validatedHistorySnapshot, change func(canonical.Value)) validatedHistorySnapshot {
	q := *p.proof
	q.snapshot = historyVNextClone(p.proof.snapshot)
	change(q.snapshot)
	return validatedHistorySnapshot{proof: &q}
}
func sfPool(t *testing.T) *vnFixture {
	t.Helper()
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"R", 1})
	for i := 0; i < 3; i++ {
		must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(8)), SubtaskID: core.PlannedSubtaskID(vnID(800 + i)), LineageID: core.SubtaskLineageID(vnID(810 + i)), Title: "child", Difficulty: core.DifficultySimple, Now: vnAt(2)}))
	}
	return f
}
func sfMutations() map[string]func(canonical.Value) {
	return map[string]func(canonical.Value){
		"unknown key":             func(s canonical.Value) { s.Object["unrecognized"] = historyString("changed") },
		"version":                 func(s canonical.Value) { s.Object["version"] = historyNumber(999) },
		"classification revision": func(s canonical.Value) { s.Object["classifications"].Object["revision"] = historyNumber(999) },
		"classification clock bits": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["capturedAt"] = historyNumber(0x8000000000000000)
		},
		"classification latest": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotsByTraceID"].Array[1].Object["id"] = historyString(vnID(999))
		},
		"classification pair order": func(s canonical.Value) {
			a := s.Object["classifications"].Object["snapshotsByTraceID"].Array
			a[0], a[1] = a[1], a[0]
		},
		"trace clock":      func(s canonical.Value) { s.Object["traces"].Array[0].Object["contentUpdatedAt"] = historyNumber(99) },
		"definition order": func(s canonical.Value) { a := s.Object["definitions"].Array; a[0], a[1] = a[1], a[0] },
		"historical definition bytes": func(s canonical.Value) {
			s.Object["definitions"].Array[0].Object["descriptionText"] = historyString("")
		},
	}
}
func TestHistorySubtaskPlannedIndependentFence(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
		t.Run(string(kind), func(t *testing.T) {
			f := sfPool(t)
			b := f.k.Snapshot()
			p := vnGood(t, f.snapshot(t))
			in := plannedSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, ExpectedDefinitionID: b.Definitions[1].ID, SubtaskID: core.PlannedSubtaskID(vnID(801)), Now: vnAt(3)}
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.PlannedSubtaskID(vnID(803))
				in.LineageID = core.SubtaskLineageID(vnID(813))
				in.Title = "new"
				in.Difficulty = core.DifficultySimple
			case core.SubtaskTitle:
				in.Title = "new"
			case core.SubtaskDifficulty:
				in.Difficulty = core.DifficultyHard
			}
			must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
			final := vnGood(t, f.snapshot(t))
			if !historyPlannedSubtaskFence(p, final, in) {
				t.Fatal("legal delta rejected")
			}
			mutations := sfMutations()
			mutations["planned created bits"] = func(s canonical.Value) {
				s.Object["definitions"].Array[1].Object["plannedSubtasks"].Array[0].Object["createdAt"] = historyNumber(0x8000000000000000)
			}
			mutations["planned lineage"] = func(s canonical.Value) {
				s.Object["definitions"].Array[1].Object["plannedSubtasks"].Array[0].Object["lineageID"] = historyObject(map[string]canonical.Value{"rawValue": historyString(vnID(999))})
			}
			mutations["planned array order"] = func(s canonical.Value) {
				a := s.Object["definitions"].Array[1].Object["plannedSubtasks"].Array
				a[0], a[1] = a[1], a[0]
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					bad := sfPolluted(final, mutate)
					if historyPlannedSubtaskFence(p, bad, in) {
						t.Fatal("pollution accepted")
					}
				})
			}
			if !historyPlannedSubtaskFence(p, final, in) {
				t.Fatal("fence mutated endpoint")
			}
		})
	}
}
func TestHistorySubtaskActualIndependentFence(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
		t.Run(string(kind), func(t *testing.T) {
			f := sfPool(t)
			for i := 0; i < 3; i++ {
				must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.PlannedSubtaskRemove, ChainID: core.TaskChainID(vnID(1)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(8)), SubtaskID: core.PlannedSubtaskID(vnID(800 + i)), Now: vnAt(2)}))
			}
			must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(820)), DayID: core.DayID(vnID(821)), Date: vnDate, Today: vnDate, Now: vnAt(3)}))
			for i := 0; i < 2; i++ {
				must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(820)), SubtaskID: core.SubtaskID(vnID(830 + i)), LineageID: core.SubtaskLineageID(vnID(840 + i)), Title: "actual", Difficulty: core.DifficultySimple, Today: vnDate, Now: vnAt(4)}))
			}
			if kind == core.SubtaskUndoComplete {
				must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(820)), SubtaskID: core.SubtaskID(vnID(830)), Today: vnDate, Now: vnAt(5)}))
			}
			b := f.k.Snapshot()
			p := vnGood(t, f.snapshot(t))
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, TraceID: core.DayTraceID(vnID(820)), SubtaskID: core.SubtaskID(vnID(830)), Today: vnDate, Now: vnAt(6)}
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.SubtaskID(vnID(832))
				in.LineageID = core.SubtaskLineageID(vnID(842))
				in.Title = "new"
				in.Difficulty = core.DifficultySimple
			case core.SubtaskTitle:
				in.Title = "new"
			case core.SubtaskDifficulty:
				in.Difficulty = core.DifficultyHard
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(vnID(850))
			}
			must(t, f.k.EditSubtask(core.SubtaskEditInput(in)))
			final := vnGood(t, f.snapshot(t))
			if !historyActualSubtaskFence(p, final, in) {
				t.Fatal("legal delta rejected")
			}
			mutations := sfMutations()
			mutations["parent chain clock"] = func(s canonical.Value) { s.Object["chains"].Array[0].Object["updatedAt"] = historyNumber(123) }
			mutations["actual order"] = func(s canonical.Value) { a := s.Object["subtasks"].Array; a[0], a[1] = a[1], a[0] }
			mutations["sibling title"] = func(s canonical.Value) { s.Object["subtasks"].Array[1].Object["title"] = historyString("polluted") }
			mutations["sibling witness"] = func(s canonical.Value) {
				s.Object["subtasks"].Array[1].Object["draftCancellationID"] = historyString(vnID(999))
			}
			mutations["target carried identity"] = func(s canonical.Value) {
				s.Object["subtasks"].Array[0].Object["carriedFromSubtaskID"] = historyObject(map[string]canonical.Value{"rawValue": historyString(vnID(998))})
			}
			mutations["target created bits"] = func(s canonical.Value) {
				s.Object["subtasks"].Array[0].Object["createdAt"] = historyNumber(0x8000000000000000)
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					if historyActualSubtaskFence(p, sfPolluted(final, mutate), in) {
						t.Fatal("pollution accepted")
					}
				})
			}
			if !historyActualSubtaskFence(p, final, in) {
				t.Fatal("fence mutated endpoint")
			}
		})
	}
}
