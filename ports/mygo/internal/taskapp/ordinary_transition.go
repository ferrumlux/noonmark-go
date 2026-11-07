// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
)

// ordinaryTransition is a closed, owned command. There is deliberately no
// after-graph, mutation callback, patch, or caller-selected capture list.
// A successful replay is a task edge, not an aggregate or persistence licence.
type ordinaryTransition struct {
	kind       ordinaryTransitionKind
	add        core.CreatePoolTaskInput
	schedule   core.ScheduleFromPoolInput
	scheduled  bool
	completion completionHistoryInput
	settle     ordinarySettlementInput
	title      titleHistoryInput
	progress   manualProgressHistoryInput
	planned    core.PlannedSubtaskEditInput
	actual     core.SubtaskEditInput
	returning  core.ReturnToPoolInput
	carry      core.TraceCarryoverInput
}
type ordinarySettlementInput struct {
	Today core.LocalDate
	Now   core.Instant
}

type ordinaryTransitionKind uint8

const (
	ordinaryAdd ordinaryTransitionKind = iota + 1
	ordinarySchedule
	ordinaryComplete
	ordinarySettle
	ordinaryTitle
	ordinaryProgress
	ordinaryPlanned
	ordinaryActual
	ordinaryReturn
	ordinaryDefer
	ordinaryContinue
)

func (in ordinaryTransition) owned() ordinaryTransition {
	in.schedule.SubtaskIDs = historyRemainingCopyIDs(in.schedule.SubtaskIDs)
	in.returning.PlannedIDs = historyRemainingCopyIDs(in.returning.PlannedIDs)
	in.carry.SubtaskIDs = historyRemainingCopyIDs(in.carry.SubtaskIDs)
	return in
}

func (in ordinaryTransition) replay(before core.TaskGraph) (core.TaskGraph, error) {
	in = in.owned()
	k, err := core.NewTaskKernel(before)
	if err != nil {
		return core.TaskGraph{}, err
	}
	switch in.kind {
	case ordinaryAdd:
		if in.scheduled && (in.schedule.ChainID != in.add.ChainID || in.schedule.Now != in.add.Now) {
			return core.TaskGraph{}, core.ErrInvalidInput
		}
		err = k.CreatePoolTask(in.add)
		if err == nil && in.scheduled {
			err = k.ScheduleFromPool(in.schedule)
		}
	case ordinarySchedule:
		err = k.ScheduleFromPool(in.schedule)
	case ordinaryComplete:
		if in.completion.Action != completionHistoryComplete && in.completion.Action != completionHistoryUndo {
			return core.TaskGraph{}, core.ErrInvalidInput
		}
		if in.completion.ChainID != "" {
			found := false
			for _, tr := range before.Traces {
				if tr.ID == in.completion.TraceID && tr.ChainID == in.completion.ChainID {
					found = true
				}
			}
			if !found {
				return core.TaskGraph{}, core.ErrInvalidInput
			}
		}
		if in.completion.Action == completionHistoryComplete {
			err = k.MarkCompleted(in.completion.TraceID, in.completion.Today, in.completion.Now)
		} else {
			err = k.UndoCompleted(in.completion.TraceID, in.completion.Today, in.completion.Now)
		}
	case ordinarySettle:
		err = k.SettleTaskDays(in.settle.Today, in.settle.Now)
	case ordinaryTitle:
		if in.title.Route == titleHistoryTrace {
			if in.title.Action != titleHistorySaveInput {
				return core.TaskGraph{}, core.ErrInvalidInput
			}
			found := false
			for _, tr := range before.Traces {
				if tr.ID == in.title.TraceID {
					found = true
					if tr.ChainID != in.title.ChainID {
						return core.TaskGraph{}, core.ErrInvalidInput
					}
					if tr.Date.Compare(in.title.Today) < 0 {
						return core.TaskGraph{}, core.ErrImmutableHistory
					}
					if tr.Status != core.TracePending {
						return core.TaskGraph{}, core.ErrInvalidTransition
					}
				}
			}
			if !found {
				return core.TaskGraph{}, core.ErrNotFound
			}
		} else if in.title.Route != titleHistoryChain || in.title.TraceID != "" {
			return core.TaskGraph{}, core.ErrInvalidInput
		}
		v := core.EditTaskTitleInput{ChainID: in.title.ChainID, Title: in.title.Title, Today: in.title.Today, Now: in.title.Now, NewDefinitionID: in.title.NewDefinitionID}
		switch in.title.Action {
		case titleHistoryRename:
			err = k.RenameTaskTitle(v)
		case titleHistorySaveInput:
			err = k.SaveTaskTitleInput(v)
		default:
			err = core.ErrInvalidInput
		}
	case ordinaryProgress:
		err = k.SetManualProgress(core.SetManualProgressInput{ChainID: in.progress.ChainID, TraceID: in.progress.TraceID, Percent: in.progress.Percent, Today: in.progress.Today, Now: in.progress.Now})
	case ordinaryPlanned:
		err = k.EditPlannedSubtask(in.planned)
	case ordinaryActual:
		err = k.EditSubtask(in.actual)
	case ordinaryReturn:
		err = k.ReturnToPool(in.returning)
	case ordinaryDefer:
		err = k.DeferCurrentTrace(in.carry)
	case ordinaryContinue:
		err = k.ContinueUnfinishedTrace(in.carry)
	default:
		err = core.ErrInvalidInput
	}
	if err != nil {
		return core.TaskGraph{}, err
	}
	return k.Snapshot(), nil
}

// match rejects even unrelated after-graph edits. This is used before an edge
// can be associated with an admitted candidate, never instead of admission.
func (in ordinaryTransition) match(before, after core.TaskGraph) error {
	replay, err := in.replay(before)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(replay, after) {
		return core.ErrInvalidInput
	}
	return nil
}
