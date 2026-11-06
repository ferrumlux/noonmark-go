// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"testing"
)

func hsSnapshot(t *testing.T, g core.TaskGraph) canonical.Value {
	t.Helper()
	extra, e := emptySupplement()
	must(t, e)
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	return s
}
func hsPool(t *testing.T, n int) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	for c := 0; c < 2; c++ {
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(1 + c*10)), DefinitionID: core.TaskDefinitionID(vnID(2 + c*10)), Title: "parent", Now: vnAt(0)}))
	}
	for i := 0; i < n; i++ {
		must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(2)), SubtaskID: core.PlannedSubtaskID(vnID(100 + i)), LineageID: core.SubtaskLineageID(vnID(200 + i)), Title: "cafe\u0301", Difficulty: core.DifficultySimple, Now: vnAt(1)}))
	}
	return k.Snapshot()
}
func hsActual(t *testing.T) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(hsPool(t, 0))
	must(t, e)
	for c := 0; c < 2; c++ {
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(1 + c*10)), TraceID: core.DayTraceID(vnID(3 + c*10)), DayID: core.DayID(vnID(4)), Date: vnDate, Today: vnDate, Now: vnAt(1)}))
	}
	// Interleaved global ordering, with three target children.
	for i, c := range []int{0, 1, 0, 1, 0} {
		must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1 + c*10)), TraceID: core.DayTraceID(vnID(3 + c*10)), SubtaskID: core.SubtaskID(vnID(300 + i)), LineageID: core.SubtaskLineageID(vnID(400 + i)), Title: "cafe\u0301", Difficulty: core.DifficultySimple, Today: vnDate, Now: vnAt(2)}))
	}
	return k.Snapshot()
}
func hsAssert(t *testing.T, r auditedHistoryCommand, e error, expected core.TaskGraph, prior validatedHistorySnapshot, changed bool) {
	t.Helper()
	must(t, e)
	if !r.valid() {
		t.Fatal("invalid proof")
	}
	v, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(v, hsSnapshot(t, expected)) {
		t.Fatal("independent full snapshot oracle")
	}
	if got, e := r.changed(); e != nil || got != changed {
		t.Fatalf("changed %v %v", got, e)
	}
	if !historyEqual(v.Object["classifications"], prior.proof.snapshot.Object["classifications"]) || r.proof.after.proof.count != prior.proof.count {
		t.Fatal("zero capture drift")
	}
	must(t, checkForwardHistorySuccessor(prior, r.proof.after))
	// All returned copies are independent of the sealed endpoint.
	v.Object["definitions"].Array[0].Object["title"] = historyString("copy poison")
	v2, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(v2, hsSnapshot(t, expected)) {
		t.Fatal("copy alias")
	}
}
func TestHistorySubtaskPlannedIndependentOracle(t *testing.T) {
	for _, n := range []int{0, 1, 3} {
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
			for target := 0; target < max(1, n); target++ {
				if n == 0 && kind != core.SubtaskAdd {
					continue
				}
				t.Run(string(kind)+"/"+vnID(n*10+target), func(t *testing.T) {
					before := hsPool(t, n)
					want := before.Clone()
					prior := vnGood(t, hsSnapshot(t, before))
					in := plannedSubtaskHistoryInput{Kind: kind, ChainID: before.Chains[0].ID, ExpectedDefinitionID: before.Definitions[0].ID, SubtaskID: core.PlannedSubtaskID(vnID(100 + target)), Now: vnAt(3)}
					p := &want.Definitions[0].PlannedSubtasks
					switch kind {
					case core.SubtaskAdd:
						in.SubtaskID = core.PlannedSubtaskID(vnID(199))
						in.LineageID = core.SubtaskLineageID(vnID(299))
						in.Title = " \r\n甲\r乙\r\n "
						in.Difficulty = core.DifficultyHard
						*p = append(*p, core.PlannedSubtask{ID: in.SubtaskID, LineageID: in.LineageID, Title: "甲\n乙", Difficulty: 3, Position: n + 1, CreatedAt: in.Now})
					case core.SubtaskTitle:
						in.Title = " \r\n甲\r乙\r\n "
						(*p)[target].Title = "甲\n乙"
					case core.SubtaskDifficulty:
						in.Difficulty = core.DifficultyMedium
						(*p)[target].Difficulty = 2
					case core.PlannedSubtaskRemove:
						*p = append((*p)[:target], (*p)[target+1:]...)
						for i := range *p {
							(*p)[i].Position = i + 1
						}
					}
					want.Chains[0].UpdatedAt = in.Now
					want.Definitions[0].ContentUpdatedAt = in.Now
					r, e := auditPlannedSubtaskHistoryVNext(prior, before, want, in)
					hsAssert(t, r, e, want, prior, true)
					if r.proof.kind != historyCommandPlannedSubtaskEdit || r.proof.subtaskEditKind != kind || r.proof.expectedDefinition != in.ExpectedDefinitionID || r.proof.plannedChild != in.SubtaskID || r.proof.actualChild != "" || r.proof.requestedLineage != in.LineageID || r.proof.requestedTitle != in.Title || r.proof.requestedDifficulty != in.Difficulty || r.proof.now != in.Now {
						t.Fatal("planned identity")
					}
					// Inputs retain no shared mutable graph leaves in either proof endpoint.
					before.Definitions[0].Title = "before poison"
					want.Definitions[0].Title = "after poison"
					if r.proof.prior.proof.snapshot.Object["definitions"].Array[0].Object["title"].String != "parent" {
						t.Fatal("before alias")
					}
					if r.proof.after.proof.snapshot.Object["definitions"].Array[0].Object["title"].String != "parent" {
						t.Fatal("after alias")
					}
				})
			}
		}
	}
}
func TestHistorySubtaskActualIndependentOracle(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
		t.Run(string(kind), func(t *testing.T) {
			before := hsActual(t)
			if kind == core.SubtaskUndoComplete {
				k, e := core.NewTaskKernel(before)
				must(t, e)
				must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, SubtaskID: before.Subtasks[2].ID, Today: vnDate, Now: vnAt(3)}))
				before = k.Snapshot()
			}
			prior := vnGood(t, hsSnapshot(t, before))
			want := before.Clone()
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, SubtaskID: before.Subtasks[2].ID, Today: vnDate, Now: vnAt(5)}
			s := &want.Subtasks[2]
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.SubtaskID(vnID(399))
				in.LineageID = core.SubtaskLineageID(vnID(499))
				in.Title = " \r\n甲\r乙\r\n "
				in.Difficulty = 3
				want.Subtasks = append(want.Subtasks, core.Subtask{ID: in.SubtaskID, LineageID: in.LineageID, TraceID: in.TraceID, Title: "甲\n乙", Difficulty: 3, Status: core.SubtaskPending, Position: 4, CreatedAt: in.Now, UpdatedAt: in.Now})
			case core.SubtaskTitle:
				in.Title = " \r\n甲\r乙\r\n "
				s.Title = "甲\n乙"
				s.UpdatedAt = in.Now
			case core.SubtaskDifficulty:
				in.Difficulty = 3
				s.Difficulty = 3
				s.UpdatedAt = in.Now
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(vnID(599))
				s.Status = core.SubtaskCancelledDraft
				s.CompletedAt = nil
				s.SettledAt = vnPtr(in.Now)
				s.UpdatedAt = in.Now
				s.DraftCancellationID = vnPtr(in.CancellationID)
			case core.SubtaskComplete:
				s.Status = core.SubtaskCompleted
				s.CompletedAt = vnPtr(in.Now)
				s.UpdatedAt = in.Now
			case core.SubtaskUndoComplete:
				s.Status = core.SubtaskPending
				s.CompletedAt = nil
				s.UpdatedAt = in.Now
			case core.SubtaskAbandon:
				s.Status = core.SubtaskAbandoned
				s.SettledAt = vnPtr(in.Now)
				s.UpdatedAt = in.Now
			}
			r, e := auditActualSubtaskHistoryVNext(prior, before, want, in)
			hsAssert(t, r, e, want, prior, true)
			if r.proof.kind != historyCommandActualSubtaskEdit || r.proof.subtaskEditKind != kind || r.proof.actualChild != in.SubtaskID || r.proof.plannedChild != "" || r.proof.expectedDefinition != "" || r.proof.requestedLineage != in.LineageID || r.proof.requestedTitle != in.Title || r.proof.requestedDifficulty != in.Difficulty || r.proof.requestedCancellation != in.CancellationID || r.proof.today != in.Today || r.proof.now != in.Now {
				t.Fatal("actual identity")
			}
		})
	}
}
func TestHistorySubtaskNoopClockIdentity(t *testing.T) {
	for _, now := range []core.Instant{vnAt(-100), core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), core.Instant(math.Float64bits(math.Inf(-1))), vnAt(math.MaxFloat64)} {
		for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty} {
			p := hsPool(t, 1)
			pp := vnGood(t, hsSnapshot(t, p))
			pi := plannedSubtaskHistoryInput{Kind: kind, ChainID: p.Chains[0].ID, ExpectedDefinitionID: p.Definitions[0].ID, SubtaskID: p.Definitions[0].PlannedSubtasks[0].ID, Now: now}
			if kind == core.SubtaskTitle {
				pi.Title = " café "
			} else {
				pi.Difficulty = 1
			}
			r, e := auditPlannedSubtaskHistoryVNext(pp, p, p, pi)
			hsAssert(t, r, e, p, pp, false)
			if r.proof.now != now {
				t.Fatal("raw Now lost")
			}
			a := hsActual(t)
			ap := vnGood(t, hsSnapshot(t, a))
			ai := actualSubtaskHistoryInput{Kind: kind, ChainID: a.Chains[0].ID, TraceID: a.Traces[0].ID, SubtaskID: a.Subtasks[0].ID, Today: vnDate, Now: now}
			if kind == core.SubtaskTitle {
				ai.Title = " café "
			} else {
				ai.Difficulty = 1
			}
			r, e = auditActualSubtaskHistoryVNext(ap, a, a, ai)
			hsAssert(t, r, e, a, ap, false)
			if r.proof.now != now {
				t.Fatal("raw Now lost")
			}
		}
	}
}
func TestHistorySubtaskPlannedRemoveArrayOrder(t *testing.T) {
	before := hsPool(t, 3)
	ps := before.Definitions[0].PlannedSubtasks
	ps[0], ps[2] = ps[2], ps[0]
	prior := vnGood(t, hsSnapshot(t, before))
	want := before.Clone()
	want.Definitions[0].PlannedSubtasks = append(want.Definitions[0].PlannedSubtasks[:1], want.Definitions[0].PlannedSubtasks[2:]...)
	for i := range want.Definitions[0].PlannedSubtasks {
		want.Definitions[0].PlannedSubtasks[i].Position = i + 1
	}
	want.Chains[0].UpdatedAt = vnAt(3)
	want.Definitions[0].ContentUpdatedAt = vnAt(3)
	in := plannedSubtaskHistoryInput{Kind: core.PlannedSubtaskRemove, ChainID: before.Chains[0].ID, ExpectedDefinitionID: before.Definitions[0].ID, SubtaskID: ps[1].ID, Now: vnAt(3)}
	r, e := auditPlannedSubtaskHistoryVNext(prior, before, want, in)
	hsAssert(t, r, e, want, prior, true)
	if !reflect.DeepEqual(want.Definitions[0].PlannedSubtasks[0].ID, ps[0].ID) {
		t.Fatal("removed via position sort")
	}
}
