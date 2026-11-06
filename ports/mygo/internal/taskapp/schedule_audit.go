// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// auditGenericSchedules constructs a bounded source-command witness whenever a
// generic batch adds traces. It proves a legal complete result, not the unique
// historical sequence or user authorization. No field is exempt from final
// canonical equality. Service commands separately bind their actual Today.
func auditGenericSchedules(before, after core.TaskGraph, extra v7.Supplement) error {
	oldTraces := map[core.DayTraceID]core.DayTrace{}
	oldChains := map[core.TaskChainID]core.TaskChain{}
	newChains := map[core.TaskChainID]core.TaskChain{}
	oldDays := map[core.LocalDate]core.Day{}
	for _, v := range before.Traces {
		oldTraces[v.ID] = v
	}
	for _, v := range before.Chains {
		oldChains[v.ID] = v
	}
	for _, v := range after.Chains {
		newChains[v.ID] = v
	}
	for _, v := range before.Days {
		oldDays[v.Date] = v
	}
	hasNew := false
	for _, v := range after.Traces {
		if _, ok := oldTraces[v.ID]; !ok {
			hasNew = true
		}
	}
	if !hasNew {
		return nil
	}
	fail := func() error { return historyError("generic new-trace batch has no supported exact command witness") }
	type step struct {
		at    core.Instant
		rank  int
		date  core.LocalDate
		apply func(*core.TaskKernel) error
	}
	var steps []step
	for _, chain := range after.Chains {
		if _, ok := oldChains[chain.ID]; ok {
			continue
		}
		var defs []core.TaskDefinition
		for _, d := range after.Definitions {
			if d.ChainID == chain.ID {
				defs = append(defs, d)
			}
		}
		if len(defs) != 1 || len(defs[0].PlannedSubtasks) != 0 || len(chain.NoteEntries) > 1 {
			return fail()
		}
		in := core.CreatePoolTaskInput{ChainID: chain.ID, DefinitionID: defs[0].ID, Title: defs[0].Title, DescriptionText: defs[0].DescriptionText, Now: chain.CreatedAt}
		if len(chain.NoteEntries) == 1 {
			n := chain.NoteEntries[0]
			in.InitialNote = &n
		}
		steps = append(steps, step{at: in.Now, rank: 0, apply: func(k *core.TaskKernel) error { return k.CreatePoolTask(in) }})
	}
	for _, trace := range after.Traces {
		old, exists := oldTraces[trace.ID]
		if !exists {
			in := core.ScheduleFromPoolInput{ChainID: trace.ChainID, TraceID: trace.ID, Date: trace.Date, Today: trace.Date, Now: trace.CreatedAt, SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{}}
			for _, day := range after.Days {
				if day.Date == trace.Date {
					in.DayID = day.ID
				}
			}
			for _, definition := range after.Definitions {
				if definition.ChainID == trace.ChainID && definition.SupersededAt == nil {
					for _, planned := range definition.PlannedSubtasks {
						for _, child := range after.Subtasks {
							if child.TraceID == trace.ID && child.LineageID == planned.LineageID {
								if _, ok := in.SubtaskIDs[planned.ID]; ok {
									return fail()
								}
								in.SubtaskIDs[planned.ID] = child.ID
							}
						}
					}
				}
			}
			steps = append(steps, step{at: in.Now, rank: 1, date: trace.Date, apply: func(k *core.TaskKernel) error { return k.ScheduleFromPool(in) }})
			old = core.DayTrace{ID: trace.ID, ChainID: trace.ChainID, Status: core.TracePending, Date: trace.Date, CreatedAt: trace.CreatedAt, ContentUpdatedAt: trace.CreatedAt}
		}
		// Only completion/undo can change a chain clock in this supported batch.
		// For unfinished final facts the chain clock retains the last such command,
		// while the trace clock is overwritten by settlement.
		complete := func(at core.Instant, rank int) {
			id, date := trace.ID, trace.Date
			steps = append(steps, step{at: at, rank: rank, apply: func(k *core.TaskKernel) error { return k.MarkCompleted(id, date, at) }})
		}
		undo := func(at core.Instant, rank int) {
			id, date := trace.ID, trace.Date
			steps = append(steps, step{at: at, rank: rank, apply: func(k *core.TaskKernel) error { return k.UndoCompleted(id, date, at) }})
		}
		switch trace.Status {
		case core.TraceCompleted:
			if trace.CompletedAt == nil {
				return fail()
			}
			if old.Status == core.TraceCompleted {
				if old.CompletedAt == nil || *old.CompletedAt != *trace.CompletedAt {
					undo(*trace.CompletedAt, 2)
					complete(*trace.CompletedAt, 3)
				}
			} else if old.Status == core.TracePending {
				complete(*trace.CompletedAt, 2)
			}
		case core.TracePending, core.TraceUnfinished:
			at := trace.ContentUpdatedAt
			if trace.Status == core.TraceUnfinished {
				at = newChains[trace.ChainID].UpdatedAt
			}
			if old.Status == core.TraceCompleted {
				undo(at, 2)
			} else if old.Status == core.TracePending && at.After(old.ContentUpdatedAt) {
				frontier := oldChains[trace.ChainID].UpdatedAt
				if trace.Status == core.TracePending || at.After(frontier) {
					complete(at, 2)
					undo(at, 3)
				}
			}
		}
	}
	for _, day := range after.Days {
		if day.LockedAt == nil || oldDays[day.Date].LockedAt != nil {
			continue
		}
		today, err := day.Date.AddDays(1)
		if err != nil {
			return err
		}
		at := *day.LockedAt
		steps = append(steps, step{at: at, rank: 4, date: day.Date, apply: func(k *core.TaskKernel) error { return k.SettleTaskDays(today, at) }})
	}
	sort.SliceStable(steps, func(i, j int) bool {
		a, b := steps[i], steps[j]
		if !a.at.Equal(b.at) {
			return a.at.Before(b.at)
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.rank == 4 {
			return a.date.Compare(b.date) < 0
		}
		// Preserve actual entity insertion order on equal-time independent
		// commands. The core validates priority; final equality validates all
		// array order rather than treating it as an ignorable difference.
		return false
	})
	k, err := core.NewTaskKernel(before)
	if err != nil {
		return err
	}
	for _, op := range steps {
		if err = op.apply(k); err != nil {
			return fail()
		}
	}
	expected, err := v7.FromDomain(recurrence.State{Tasks: k.Snapshot()}, extra)
	if err != nil {
		return err
	}
	actual, err := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	if err != nil {
		return err
	}
	if !historyEqual(expected, actual) {
		return fail()
	}
	return nil
}
