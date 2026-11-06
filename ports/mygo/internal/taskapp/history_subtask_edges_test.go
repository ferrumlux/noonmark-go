// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"testing"
)

// Parameter fixtures in this file prove core/profile acceptance first; they
// are not assertions of Service reachability or allocator/frontier behaviour.
func hsReclock(g core.TaskGraph, at core.Instant) core.TaskGraph {
	for i := range g.Chains {
		g.Chains[i].CreatedAt = at
		g.Chains[i].UpdatedAt = at
	}
	for i := range g.Definitions {
		g.Definitions[i].CreatedAt = at
		g.Definitions[i].ContentUpdatedAt = at
		for j := range g.Definitions[i].PlannedSubtasks {
			g.Definitions[i].PlannedSubtasks[j].CreatedAt = at
		}
	}
	for i := range g.Days {
		g.Days[i].CreatedAt = at
		g.Days[i].UpdatedAt = at
	}
	for i := range g.Traces {
		g.Traces[i].CreatedAt = at
		g.Traces[i].ContentUpdatedAt = at
	}
	for i := range g.Subtasks {
		g.Subtasks[i].CreatedAt = at
		g.Subtasks[i].UpdatedAt = at
	}
	return g
}
func TestHistorySubtaskExactLocalClocks(t *testing.T) {
	negzero := core.Instant(math.Float64bits(math.Copysign(0, -1)))
	for _, pair := range [][2]core.Instant{{negzero, 0}, {0, negzero}, {vnAt(-2), vnAt(-1)}, {vnAt(-1), vnAt(-1)}, {vnAt(math.MaxFloat64), vnAt(math.MaxFloat64)}} {
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
			b := hsReclock(hsPool(t, 1), pair[0])
			p := vnGood(t, hsSnapshot(t, b))
			in := plannedSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, ExpectedDefinitionID: b.Definitions[0].ID, SubtaskID: b.Definitions[0].PlannedSubtasks[0].ID, Now: pair[1]}
			w := b.Clone()
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.PlannedSubtaskID(vnID(199))
				in.LineageID = core.SubtaskLineageID(vnID(299))
				in.Title = "new"
				in.Difficulty = 2
				w.Definitions[0].PlannedSubtasks = append(w.Definitions[0].PlannedSubtasks, core.PlannedSubtask{ID: in.SubtaskID, LineageID: in.LineageID, Title: "new", Difficulty: 2, Position: 2, CreatedAt: pair[1]})
			case core.SubtaskTitle:
				in.Title = "new"
				w.Definitions[0].PlannedSubtasks[0].Title = "new"
			case core.SubtaskDifficulty:
				in.Difficulty = 2
				w.Definitions[0].PlannedSubtasks[0].Difficulty = 2
			case core.PlannedSubtaskRemove:
				w.Definitions[0].PlannedSubtasks = nil
			}
			w.Chains[0].UpdatedAt = pair[1]
			w.Definitions[0].ContentUpdatedAt = pair[1]
			r, e := auditPlannedSubtaskHistoryVNext(p, b, w, in)
			hsAssert(t, r, e, w, p, true)
		}
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
			b := hsReclock(hsActual(t), pair[0])
			if kind == core.SubtaskUndoComplete {
				b.Subtasks[0].Status = core.SubtaskCompleted
				b.Subtasks[0].CompletedAt = vnPtr(pair[0])
			}
			p := vnGood(t, hsSnapshot(t, b))
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[0].ID, Today: vnDate, Now: pair[1]}
			w := b.Clone()
			s := &w.Subtasks[0]
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.SubtaskID(vnID(399))
				in.LineageID = core.SubtaskLineageID(vnID(499))
				in.Title = "new"
				in.Difficulty = 2
				w.Subtasks = append(w.Subtasks, core.Subtask{ID: in.SubtaskID, LineageID: in.LineageID, TraceID: in.TraceID, Title: "new", Status: core.SubtaskPending, Difficulty: 2, Position: 4, CreatedAt: pair[1], UpdatedAt: pair[1]})
			case core.SubtaskTitle:
				in.Title = "new"
				s.Title = "new"
				s.UpdatedAt = pair[1]
			case core.SubtaskDifficulty:
				in.Difficulty = 2
				s.Difficulty = 2
				s.UpdatedAt = pair[1]
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(vnID(599))
				s.Status = core.SubtaskCancelledDraft
				s.CompletedAt = nil
				s.SettledAt = vnPtr(pair[1])
				s.UpdatedAt = pair[1]
				s.DraftCancellationID = vnPtr(in.CancellationID)
			case core.SubtaskComplete:
				s.Status = core.SubtaskCompleted
				s.CompletedAt = vnPtr(pair[1])
				s.UpdatedAt = pair[1]
			case core.SubtaskUndoComplete:
				s.Status = core.SubtaskPending
				s.CompletedAt = nil
				s.UpdatedAt = pair[1]
			case core.SubtaskAbandon:
				s.Status = core.SubtaskAbandoned
				s.SettledAt = vnPtr(pair[1])
				s.UpdatedAt = pair[1]
			}
			r, e := auditActualSubtaskHistoryVNext(p, b, w, in)
			hsAssert(t, r, e, w, p, true)
		}
	}
}
func TestHistorySubtaskRestoredWitnessAndPresentableChildren(t *testing.T) {
	for _, status := range []core.SubtaskStatus{core.SubtaskPending, core.SubtaskCompleted, core.SubtaskAbandoned, core.SubtaskUnfinished} {
		for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
			if (kind == core.SubtaskComplete || kind == core.SubtaskAbandon) && status != core.SubtaskPending {
				continue
			}
			if kind == core.SubtaskUndoComplete && status != core.SubtaskCompleted {
				continue
			}
			b := hsActual(t)
			b.Subtasks[0].Status = status
			b.Subtasks[0].DraftCancellationID = vnPtr(core.CancellationID(vnID(600)))
			if status == core.SubtaskCompleted {
				b.Subtasks[0].CompletedAt = vnPtr(vnAt(2))
			}
			if status == core.SubtaskAbandoned || status == core.SubtaskUnfinished {
				b.Subtasks[0].SettledAt = vnPtr(vnAt(2))
			}
			p := vnGood(t, hsSnapshot(t, b))
			w := b.Clone()
			s := &w.Subtasks[0]
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[0].ID, Today: vnDate, Now: vnAt(3)}
			switch kind {
			case core.SubtaskTitle:
				in.Title = "new"
				s.Title = "new"
			case core.SubtaskDifficulty:
				in.Difficulty = 3
				s.Difficulty = 3
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(vnID(601))
				s.Status = core.SubtaskCancelledDraft
				s.CompletedAt = nil
				s.SettledAt = vnPtr(in.Now)
				s.DraftCancellationID = vnPtr(in.CancellationID)
			case core.SubtaskComplete:
				s.Status = core.SubtaskCompleted
				s.CompletedAt = vnPtr(in.Now)
			case core.SubtaskUndoComplete:
				s.Status = core.SubtaskPending
				s.CompletedAt = nil
			case core.SubtaskAbandon:
				s.Status = core.SubtaskAbandoned
				s.SettledAt = vnPtr(in.Now)
			}
			s.UpdatedAt = in.Now
			r, e := auditActualSubtaskHistoryVNext(p, b, w, in)
			hsAssert(t, r, e, w, p, true)
		}
	}
}
func TestHistorySubtaskActualHiddenCountAndLocalClock(t *testing.T) {
	b := hsActual(t)
	k, e := core.NewTaskKernel(b)
	must(t, e)
	must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskDelete, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[0].ID, CancellationID: core.CancellationID(vnID(600)), Today: vnDate, Now: vnAt(3)}))
	b = k.Snapshot()
	// A later parent/chain clock is not an additional child mutation floor.
	b.Chains[0].UpdatedAt = vnAt(100)
	b.Traces[0].ContentUpdatedAt = vnAt(100)
	p := vnGood(t, hsSnapshot(t, b))
	w := b.Clone()
	in := actualSubtaskHistoryInput{Kind: core.SubtaskAdd, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: core.SubtaskID(vnID(399)), LineageID: core.SubtaskLineageID(vnID(499)), Title: "new", Difficulty: 1, Today: vnDate, Now: vnAt(-1)}
	w.Subtasks = append(w.Subtasks, core.Subtask{ID: in.SubtaskID, LineageID: in.LineageID, TraceID: in.TraceID, Title: "new", Status: core.SubtaskPending, Difficulty: 1, Position: 4, CreatedAt: in.Now, UpdatedAt: in.Now})
	r, e := auditActualSubtaskHistoryVNext(p, b, w, in)
	hsAssert(t, r, e, w, p, true)
	in = actualSubtaskHistoryInput{Kind: core.SubtaskTitle, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[2].ID, Title: "new", Today: vnDate, Now: vnAt(2)}
	w = b.Clone()
	w.Subtasks[2].Title = "new"
	w.Subtasks[2].UpdatedAt = in.Now
	r, e = auditActualSubtaskHistoryVNext(p, b, w, in)
	hsAssert(t, r, e, w, p, true)
}
func TestHistorySubtaskDifficultyPairs(t *testing.T) {
	for from := core.DifficultySimple; from <= core.DifficultyHard; from++ {
		for to := core.DifficultySimple; to <= core.DifficultyHard; to++ {
			b := hsPool(t, 1)
			b.Definitions[0].PlannedSubtasks[0].Difficulty = from
			p := vnGood(t, hsSnapshot(t, b))
			w := b.Clone()
			w.Definitions[0].PlannedSubtasks[0].Difficulty = to
			if from != to {
				w.Chains[0].UpdatedAt = vnAt(3)
				w.Definitions[0].ContentUpdatedAt = vnAt(3)
			}
			in := plannedSubtaskHistoryInput{Kind: core.SubtaskDifficulty, ChainID: b.Chains[0].ID, ExpectedDefinitionID: b.Definitions[0].ID, SubtaskID: b.Definitions[0].PlannedSubtasks[0].ID, Difficulty: to, Now: vnAt(3)}
			r, e := auditPlannedSubtaskHistoryVNext(p, b, w, in)
			hsAssert(t, r, e, w, p, from != to)
			a := hsActual(t)
			a.Subtasks[0].Difficulty = from
			ap := vnGood(t, hsSnapshot(t, a))
			aw := a.Clone()
			aw.Subtasks[0].Difficulty = to
			if from != to {
				aw.Subtasks[0].UpdatedAt = vnAt(3)
			}
			ai := actualSubtaskHistoryInput{Kind: core.SubtaskDifficulty, ChainID: a.Chains[0].ID, TraceID: a.Traces[0].ID, SubtaskID: a.Subtasks[0].ID, Today: vnDate, Difficulty: to, Now: vnAt(3)}
			r, e = auditActualSubtaskHistoryVNext(ap, a, aw, ai)
			hsAssert(t, r, e, aw, ap, from != to)
		}
	}
}

func TestHistorySubtaskNestedOwnedEndpoints(t *testing.T) {
	for _, fail := range []bool{false, true} {
		for _, actual := range []bool{false, true} {
			b := hsPool(t, 2)
			if actual {
				b = hsActual(t)
			}
			b.Definitions[0].DescriptionText = vnPtr("")
			b.Definitions[1].DescriptionText = vnPtr("other")
			b.Chains[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(vnID(700)), Body: "", CreatedAt: vnAt(0), UpdatedAt: vnAt(0), DeletedAt: vnPtr(vnAt(0))}}
			if actual {
				b.Traces[0].DescriptionText = vnPtr("")
				b.Traces[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(vnID(701)), Body: "trace note", CreatedAt: vnAt(1), UpdatedAt: vnAt(1)}}
				b.Subtasks[0].DraftCancellationID = vnPtr(core.CancellationID(vnID(702)))
			} else {
				b.Definitions[1].PlannedSubtasks = []core.PlannedSubtask{{ID: core.PlannedSubtaskID(vnID(703)), LineageID: core.SubtaskLineageID(vnID(704)), Title: "other child", Difficulty: 1, Position: 1, CreatedAt: vnAt(0)}}
			}
			p := vnGood(t, hsSnapshot(t, b))
			priorSnapshot, e := p.snapshotCopy()
			must(t, e)
			a := b.Clone()
			// Sharing unchanged leaves is allowed. Audits must still own both endpoints.
			a.Definitions[0].DescriptionText = b.Definitions[0].DescriptionText
			a.Definitions[1].DescriptionText = b.Definitions[1].DescriptionText
			a.Chains[0].NoteEntries = b.Chains[0].NoteEntries
			var r auditedHistoryCommand
			if actual {
				a.Traces[0].DescriptionText = b.Traces[0].DescriptionText
				a.Traces[0].NoteEntries = b.Traces[0].NoteEntries
				a.Subtasks[0].DraftCancellationID = b.Subtasks[0].DraftCancellationID
				a.Subtasks[0].Title = "changed"
				a.Subtasks[0].UpdatedAt = vnAt(5)
				in := actualSubtaskHistoryInput{Kind: core.SubtaskTitle, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[0].ID, Title: "changed", Today: vnDate, Now: vnAt(5)}
				if fail {
					in.Title = "wrong request"
				}
				r, e = auditActualSubtaskHistoryVNext(p, b, a, in)
			} else {
				a.Definitions[1].PlannedSubtasks = b.Definitions[1].PlannedSubtasks
				a.Definitions[0].PlannedSubtasks[0].Title = "changed"
				a.Definitions[0].ContentUpdatedAt = vnAt(5)
				a.Chains[0].UpdatedAt = vnAt(5)
				in := plannedSubtaskHistoryInput{Kind: core.SubtaskTitle, ChainID: b.Chains[0].ID, ExpectedDefinitionID: b.Definitions[0].ID, SubtaskID: b.Definitions[0].PlannedSubtasks[0].ID, Title: "changed", Now: vnAt(5)}
				if fail {
					in.Title = "wrong request"
				}
				r, e = auditPlannedSubtaskHistoryVNext(p, b, a, in)
			}
			var resultSnapshot = hsSnapshot(t, a)
			if fail {
				lifecycleZero(t, r, e)
			} else {
				must(t, e)
				got, e := r.snapshotCopy()
				must(t, e)
				if !historyEqual(got, resultSnapshot) {
					t.Fatal("owned result mismatch")
				}
			}
			*b.Definitions[0].DescriptionText = "poison"
			*a.Definitions[1].DescriptionText = "poison"
			b.Chains[0].NoteEntries[0].Body = "poison"
			*a.Chains[0].NoteEntries[0].DeletedAt = vnAt(-9)
			if actual {
				*a.Traces[0].DescriptionText = "poison"
				b.Traces[0].NoteEntries[0].Body = "poison"
				*a.Subtasks[0].DraftCancellationID = core.CancellationID(vnID(999))
				a.Subtasks[0].Title = "poison"
			} else {
				b.Definitions[0].PlannedSubtasks[0].Title = "poison"
				a.Definitions[0].PlannedSubtasks[0].Title = "poison"
				b.Definitions[1].PlannedSubtasks[0].Title = "poison"
			}
			got, e := p.snapshotCopy()
			must(t, e)
			if !historyEqual(got, priorSnapshot) {
				t.Fatal("caller mutated prior owned nested leaves")
			}
			if !fail {
				got, e = r.snapshotCopy()
				must(t, e)
				if !historyEqual(got, resultSnapshot) {
					t.Fatal("caller mutated result owned nested leaves")
				}
				got.Object["definitions"].Array[0].Object["descriptionText"] = historyString("copy poison")
				got.Object["chains"].Array[0].Object["noteEntries"].Array[0].Object["body"] = historyString("copy poison")
				again, e := r.snapshotCopy()
				must(t, e)
				if !historyEqual(again, resultSnapshot) {
					t.Fatal("returned nested leaves alias proof")
				}
			}
		}
	}
}
func TestHistorySubtaskChildTransitionOrder(t *testing.T) {
	for _, status := range []core.SubtaskStatus{core.SubtaskPending, core.SubtaskCompleted, core.SubtaskAbandoned, core.SubtaskUnfinished, core.SubtaskCancelledDraft} {
		for _, kind := range []core.SubtaskEditKind{core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
			if ((kind == core.SubtaskComplete || kind == core.SubtaskAbandon) && status == core.SubtaskPending) || (kind == core.SubtaskUndoComplete && status == core.SubtaskCompleted) {
				continue
			}
			b := hsActual(t)
			b.Subtasks[0].Status = status
			switch status {
			case core.SubtaskCompleted:
				b.Subtasks[0].CompletedAt = vnPtr(vnAt(2))
			case core.SubtaskAbandoned, core.SubtaskUnfinished:
				b.Subtasks[0].SettledAt = vnPtr(vnAt(2))
			case core.SubtaskCancelledDraft:
				b.Subtasks[0].SettledAt = vnPtr(vnAt(2))
				b.Subtasks[0].DraftCancellationID = vnPtr(core.CancellationID(vnID(599)))
			}
			p := vnGood(t, hsSnapshot(t, b))
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, SubtaskID: b.Subtasks[0].ID, Today: vnDate, Now: core.Instant(math.Float64bits(math.NaN()))}
			r, e := auditActualSubtaskHistoryVNext(p, b, b, in)
			want := core.ErrInvalidTransition
			if status == core.SubtaskCancelledDraft {
				want = core.ErrNotFound
			}
			hsgError(t, r, e, want)
		}
	}
}
