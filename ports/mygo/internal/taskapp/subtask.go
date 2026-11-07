// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type subtaskEditCommand struct {
	planned *core.PlannedSubtaskEditInput
	actual  *core.SubtaskEditInput
}

func (c subtaskEditCommand) apply(k *core.TaskKernel) error {
	if (c.planned == nil) == (c.actual == nil) {
		return core.ErrInvalidInput
	}
	if c.planned != nil {
		return k.EditPlannedSubtask(*c.planned)
	}
	return k.EditSubtask(*c.actual)
}

type subtaskEditAck struct {
	planned core.PlannedSubtaskID
	actual  core.SubtaskID
	title   string
}

func (s *Service) AddPlannedSubtask(chain core.TaskChainID, definition core.TaskDefinitionID, title string, difficulty core.Difficulty) (core.PlannedSubtaskID, error) {
	a, e := s.editSubtask(subtaskEditCommand{planned: &core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, ExpectedDefinitionID: definition, Title: title, Difficulty: difficulty}})
	return a.planned, e
}
func (s *Service) SavePlannedSubtaskTitleAcknowledged(chain core.TaskChainID, definition core.TaskDefinitionID, id core.PlannedSubtaskID, title string) (string, error) {
	a, e := s.editSubtask(subtaskEditCommand{planned: &core.PlannedSubtaskEditInput{Kind: core.SubtaskTitle, ChainID: chain, ExpectedDefinitionID: definition, SubtaskID: id, Title: title}})
	return a.title, e
}
func (s *Service) SetPlannedSubtaskDifficulty(chain core.TaskChainID, definition core.TaskDefinitionID, id core.PlannedSubtaskID, difficulty core.Difficulty) error {
	_, e := s.editSubtask(subtaskEditCommand{planned: &core.PlannedSubtaskEditInput{Kind: core.SubtaskDifficulty, ChainID: chain, ExpectedDefinitionID: definition, SubtaskID: id, Difficulty: difficulty}})
	return e
}
func (s *Service) RemovePlannedSubtask(chain core.TaskChainID, definition core.TaskDefinitionID, id core.PlannedSubtaskID) error {
	_, e := s.editSubtask(subtaskEditCommand{planned: &core.PlannedSubtaskEditInput{Kind: core.PlannedSubtaskRemove, ChainID: chain, ExpectedDefinitionID: definition, SubtaskID: id}})
	return e
}
func (s *Service) AddSubtask(chain core.TaskChainID, trace core.DayTraceID, title string, difficulty core.Difficulty) (core.SubtaskID, error) {
	a, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, TraceID: trace, Title: title, Difficulty: difficulty}})
	return a.actual, e
}
func (s *Service) SaveSubtaskTitleAcknowledged(chain core.TaskChainID, trace core.DayTraceID, id core.SubtaskID, title string) (string, error) {
	a, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: core.SubtaskTitle, ChainID: chain, TraceID: trace, SubtaskID: id, Title: title}})
	return a.title, e
}
func (s *Service) SetSubtaskDifficulty(chain core.TaskChainID, trace core.DayTraceID, id core.SubtaskID, difficulty core.Difficulty) error {
	_, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: core.SubtaskDifficulty, ChainID: chain, TraceID: trace, SubtaskID: id, Difficulty: difficulty}})
	return e
}
func (s *Service) DeleteSubtask(chain core.TaskChainID, trace core.DayTraceID, id core.SubtaskID) error {
	_, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: core.SubtaskDelete, ChainID: chain, TraceID: trace, SubtaskID: id}})
	return e
}
func (s *Service) SetSubtaskCompleted(chain core.TaskChainID, trace core.DayTraceID, id core.SubtaskID, completed bool) error {
	kind := core.SubtaskComplete
	if !completed {
		kind = core.SubtaskUndoComplete
	}
	_, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: kind, ChainID: chain, TraceID: trace, SubtaskID: id}})
	return e
}

// AbandonSubtask mirrors the low-level source command; no new UI action is implied.
func (s *Service) AbandonSubtask(chain core.TaskChainID, trace core.DayTraceID, id core.SubtaskID) error {
	_, e := s.editSubtask(subtaskEditCommand{actual: &core.SubtaskEditInput{Kind: core.SubtaskAbandon, ChainID: chain, TraceID: trace, SubtaskID: id}})
	return e
}

func (s *Service) editSubtask(c subtaskEditCommand) (subtaskEditAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aggregate != nil {
		if err := s.reconcileDayLocked(s.today); err != nil {
			return subtaskEditAck{}, err
		}
	}
	fail := func(e error) (subtaskEditAck, error) { return subtaskEditAck{}, e }
	if err := s.gateLocked(); err != nil {
		return fail(err)
	}
	if (c.planned == nil) == (c.actual == nil) {
		return fail(core.ErrInvalidInput)
	}
	k, e := core.NewTaskKernel(s.graph)
	if e != nil {
		return fail(e)
	}
	now, e := s.nextInstant()
	if e != nil {
		return fail(e)
	}
	if c.planned != nil {
		c.planned.Now = now
		if c.planned.Kind == core.SubtaskAdd {
			ids, e := newIDs(2)
			if e != nil {
				return fail(e)
			}
			c.planned.SubtaskID = core.PlannedSubtaskID(ids[0])
			c.planned.LineageID = core.SubtaskLineageID(ids[1])
		}
	} else {
		c.actual.Now = now
		c.actual.Today = s.today
		if c.actual.Kind == core.SubtaskAdd {
			ids, e := newIDs(2)
			if e != nil {
				return fail(e)
			}
			c.actual.SubtaskID = core.SubtaskID(ids[0])
			c.actual.LineageID = core.SubtaskLineageID(ids[1])
		} else if c.actual.Kind == core.SubtaskDelete {
			ids, e := newIDs(1)
			if e != nil {
				return fail(e)
			}
			c.actual.CancellationID = core.CancellationID(ids[0])
		}
	}
	if e = c.apply(k); e != nil {
		return fail(e)
	}
	if s.aggregate != nil {
		return s.ordinarySubtaskAckLocked(c)
	}
	captured, e := captureSubtaskEditHistory(s.snapshot, s.graph, k.Snapshot(), c)
	if e != nil {
		return fail(e)
	}
	wire, g, e := normalizeSnapshot(captured)
	if e != nil {
		return fail(e)
	}
	// Take acknowledgments only from the exact canonical candidate. Do not return
	// requested bytes or a subsequent snapshot belonging to another transaction.
	ack := subtaskEditAck{}
	if c.planned != nil {
		ack.planned = c.planned.SubtaskID
		found := false
		for _, d := range g.Definitions {
			if d.ID == c.planned.ExpectedDefinitionID && d.ChainID == c.planned.ChainID {
				for _, sub := range d.PlannedSubtasks {
					if sub.ID == ack.planned {
						ack.title = sub.Title
						found = true
					}
				}
			}
		}
		if !found && c.planned.Kind != core.PlannedSubtaskRemove {
			return fail(core.ErrNotFound)
		}
	} else {
		ack.actual = c.actual.SubtaskID
		found := false
		for _, sub := range g.Subtasks {
			if sub.ID == ack.actual && sub.TraceID == c.actual.TraceID {
				ack.title = sub.Title
				found = true
			}
		}
		if !found {
			return fail(core.ErrNotFound)
		}
	}
	if historyEqual(s.snapshot, wire) {
		if _, e = s.verifyPrefixLocked(); e != nil {
			return fail(e)
		}
		return ack, nil
	}
	if e = s.commitLocked(wire); e != nil {
		return fail(e)
	}
	s.graph, s.snapshot = g, wire
	return ack, nil
}

// No generic field exemptions: replay the precise routed command against the
// complete before graph and require the full candidate to match. Classifications
// and all other supplements retain their exact bytes and unsupported scope.
func captureSubtaskEditHistory(prior canonical.Value, before, after core.TaskGraph, c subtaskEditCommand) (canonical.Value, error) {
	fail := func(e error) (canonical.Value, error) { return canonical.Value{}, e }
	if e := ValidateEmptyHistory(prior); e != nil {
		return fail(e)
	}
	prior, e := inspectEmptyHistory(prior)
	if e != nil {
		return fail(e)
	}
	_, extra, e := v7.ToDomain(prior)
	if e != nil {
		return fail(e)
	}
	old, e := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	if e != nil {
		return fail(e)
	}
	if !historyEqual(prior, old) {
		return fail(historyError("prior does not match subtask before graph"))
	}
	k, e := core.NewTaskKernel(before)
	if e != nil {
		return fail(e)
	}
	if e = c.apply(k); e != nil {
		return fail(e)
	}
	expected, e := v7.FromDomain(recurrence.State{Tasks: k.Snapshot()}, extra)
	if e != nil {
		return fail(e)
	}
	result, e := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	if e != nil {
		return fail(e)
	}
	if !historyEqual(expected, result) {
		return fail(historyError("candidate differs from subtask command replay"))
	}
	frozen := []string{"days", "traces", "classifications", "preferences", "ideas", "taskCycleSeries"}
	if c.actual != nil {
		frozen = append(frozen, "chains", "definitions")
	} else {
		frozen = append(frozen, "subtasks")
	}
	for _, key := range frozen {
		if !historyEqual(prior.Object[key], result.Object[key]) {
			return fail(historyError("subtask changed frozen " + key))
		}
	}
	// Defense in depth within this runtime's supported subset: ordinary returns
	// create a fresh pool definition. The source's snapshot undo may restore a
	// current-definition/history alias, but its cancellation witnesses are not
	// yet accepted by ValidateEmptyHistory. Do not silently expand undo support
	// or rewrite such historical aliases through this editing adapter.
	if c.planned != nil {
		historical := map[string]bool{}
		for _, t := range before.Traces {
			if t.Status.FormsDayHistory() {
				historical[string(t.DefinitionID)] = true
			}
		}
		defs := map[string]canonical.Value{}
		for _, d := range result.Object["definitions"].Array {
			defs[historyID(d.Object["id"])] = d
		}
		for _, d := range prior.Object["definitions"].Array {
			id := historyID(d.Object["id"])
			if historical[id] && !historyEqual(d, defs[id]) {
				return fail(historyError("planned edit changed historical definition"))
			}
		}
	}
	if e = ValidateEmptyHistory(result); e != nil {
		return fail(e)
	}
	return result, nil
}
