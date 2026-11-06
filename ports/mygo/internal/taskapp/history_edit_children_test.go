// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func TestHistoryEditManualWholeChainChildren(t *testing.T) {
	for _, offset := range []int{-1, 0, 1} {
		t.Run(string(rune('1'+offset)), func(t *testing.T) {
			f := vnSeed(t, vnDate)
			g := f.k.Snapshot()
			chain, trace := g.Chains[0].ID, g.Traces[0].ID
			child := core.SubtaskID(vnID(30))
			must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, TraceID: trace, SubtaskID: child, LineageID: core.SubtaskLineageID(vnID(31)), Title: "child", Difficulty: core.DifficultyHard, Today: vnDate, Now: vnAt(1)}))
			must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: chain, TraceID: trace, SubtaskID: child, Today: vnDate, Now: vnAt(2)}))
			f.run(t, vnStep{"R", 3})
			day, e := vnDate.AddDays(offset)
			must(t, e)
			newTrace := core.DayTraceID(vnID(40))
			must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: newTrace, DayID: core.DayID(vnID(41)), Date: day, Today: day, Now: vnAt(4)}))
			g = f.k.Snapshot()
			p := vnGood(t, f.snapshot(t))
			in := manualProgressHistoryInput{ChainID: chain, TraceID: newTrace, Percent: 50, Today: day, Now: vnAt(5)}
			r, e := auditManualProgressHistoryVNext(p, g, g, in)
			hcZero(t, r, e)
			if !errors.Is(e, core.ErrInvalidTransition) {
				t.Fatal("whole-chain child", e)
			}
		})
	}
	for _, other := range []bool{false, true} {
		f := vnSeed(t, vnDate)
		g := f.k.Snapshot()
		chain, trace := g.Chains[0].ID, g.Traces[0].ID
		targetChain, targetTrace := chain, trace
		if other {
			chain = core.TaskChainID(vnID(20))
			trace = core.DayTraceID(vnID(22))
			must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(vnID(21)), Title: "other", Now: vnAt(1)}))
			must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(vnID(23)), Date: vnDate, Today: vnDate, Now: vnAt(2)}))
		}
		child := core.SubtaskID(vnID(30))
		must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, TraceID: trace, SubtaskID: child, LineageID: core.SubtaskLineageID(vnID(31)), Title: "child", Difficulty: core.DifficultyHard, Today: vnDate, Now: vnAt(3)}))
		if !other {
			must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskDelete, ChainID: chain, TraceID: trace, SubtaskID: child, CancellationID: core.CancellationID(vnID(32)), Today: vnDate, Now: vnAt(4)}))
		}
		g = f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		in := manualProgressHistoryInput{ChainID: targetChain, TraceID: targetTrace, Percent: 50, Today: vnDate, Now: vnAt(5)}
		must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
		r, e := auditManualProgressHistoryVNext(p, g, f.k.Snapshot(), in)
		editCheck(t, p, r, e)
	}
}
func TestHistoryEditTitlePlannedCopyAndTyping(t *testing.T) {
	// This is a sealed profile fixture, not a Service-generated restoration.
	for _, description := range []*string{nil, vnPtr(""), vnPtr("body")} {
		for _, planned := range []bool{false, true} {
			f := vnSeed(t, vnDate)
			g := f.k.Snapshot()
			g.Definitions[0].DescriptionText = description
			if planned {
				g.Definitions[0].PlannedSubtasks = []core.PlannedSubtask{{ID: core.PlannedSubtaskID(vnID(61)), LineageID: core.SubtaskLineageID(vnID(62)), Title: "planned", Difficulty: core.DifficultyHard, Position: 1}}
			}
			in := editInput(g)
			in.Action = titleHistoryRename
			p := editPrior(t, g)
			a, e := editTitle(t, g, in)
			must(t, e)
			r, e := auditTitleHistoryVNext(p, g, a, in)
			editCheck(t, p, r, e)
			// Once separated from old definitions, each subsequent input reuses current.
			p = r.proof.after
			before := a
			in.Action = titleHistorySaveInput
			in.Title = "changed again"
			in.Now = vnAt(6)
			in.NewDefinitionID = "\xff"
			a, e = editTitle(t, before, in)
			must(t, e)
			r, e = auditTitleHistoryVNext(p, before, a, in)
			editCheck(t, p, r, e)
			if len(a.Definitions) != len(before.Definitions) {
				t.Fatal("per-key version growth")
			}
			if planned {
				a.Definitions[1].PlannedSubtasks[0].Title = "caller mutation"
				copy, e := r.snapshotCopy()
				must(t, e)
				if historyEqual(copy, r.proof.prior.proof.snapshot) {
					t.Fatal("invalid detached snapshot")
				}
				if before.Definitions[1].PlannedSubtasks[0].Title != "planned" {
					t.Fatal("core version aliases")
				}
			}
		}
	}
}
func TestHistoryEditTitleHistoricalReferenceVersion(t *testing.T) {
	for _, action := range []titleHistoryAction{titleHistoryRename, titleHistorySaveInput} {
		f := vnSeed(t, vnDate)
		f.run(t, vnStep{"R", 1})
		g := f.k.Snapshot()
		// A parameter/profile fixture with R referencing current, not a command path.
		for _, d := range g.Definitions {
			if d.SupersededAt == nil {
				g.Traces[0].DefinitionID = d.ID
			}
		}
		p := vnGood(t, lifecycleSnapshot(t, g, f.c))
		in := editInput(g)
		in.Action = action
		a, e := editTitle(t, g, in)
		must(t, e)
		r, e := auditTitleHistoryVNext(p, g, a, in)
		editCheck(t, p, r, e)
		if len(a.Definitions) != len(g.Definitions)+1 || a.Traces[0].DefinitionID != g.Traces[0].DefinitionID {
			t.Fatal("historical current not versioned/frozen")
		}
	}
}
