// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type titleMode uint8

const (
	titleRename titleMode = iota + 1
	titleInput
)

// A closed command vocabulary, never a caller-provided field exemption list.
type titleCommand struct {
	mode  titleMode
	input core.EditTaskTitleInput
}

func (c titleCommand) apply(k *core.TaskKernel) error {
	switch c.mode {
	case titleRename:
		return k.RenameTaskTitle(c.input)
	case titleInput:
		return k.SaveTaskTitleInput(c.input)
	default:
		return core.ErrInvalidInput
	}
}

func (s *Service) RenameTaskTitle(chain core.TaskChainID, title string) error {
	return s.editTitle(chain, title, titleRename)
}
func (s *Service) SaveTaskTitleInput(chain core.TaskChainID, title string) error {
	return s.editTitle(chain, title, titleInput)
}
func (s *Service) editTitle(chain core.TaskChainID, title string, mode titleMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.editTitleLocked(chain, title, mode)
	return err
}

// SaveTraceTitleInput is the Day detail adapter. Check the selected trace, not
// merely whether its chain has some other editable trace. Keep this check and
// the complete title transaction under one lock to prevent stale UI callbacks
// from modifying a current task through an immutable historical detail.
func (s *Service) SaveTraceTitleInput(trace core.DayTraceID, title string) error {
	_, err := s.SaveTraceTitleInputAcknowledged(trace, title)
	return err
}

// SaveTraceTitleInputAcknowledged returns the exact verified persisted title
// from the same locked transaction. A canonical-equivalent no-op returns its
// existing bytes; async input owners must not invent normalized stored bytes.
func (s *Service) SaveTraceTitleInputAcknowledged(trace core.DayTraceID, title string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrSession
	}
	if s.disk.blocked {
		return "", ErrIncomplete
	}
	for _, tr := range s.graph.Traces {
		if tr.ID != trace {
			continue
		}
		if tr.Date.Compare(s.today) < 0 {
			return "", core.ErrImmutableHistory
		}
		if tr.Status != core.TracePending {
			return "", core.ErrInvalidTransition
		}
		return s.editTitleLocked(tr.ChainID, title, titleInput)
	}
	return "", core.ErrNotFound
}

// Caller owns s.mu for both any route validation and the complete transaction.
func (s *Service) editTitleLocked(chain core.TaskChainID, title string, mode titleMode) (string, error) {
	if s.closed {
		return "", ErrSession
	}
	if s.disk.blocked {
		return "", ErrIncomplete
	}
	k, err := core.NewTaskKernel(s.graph)
	if err != nil {
		return "", err
	}
	now, err := s.nextInstant()
	if err != nil {
		return "", err
	}
	ids, err := newIDs(1)
	if err != nil {
		return "", err
	}
	command := titleCommand{mode, core.EditTaskTitleInput{ChainID: chain, Title: title, Today: s.today, Now: now, NewDefinitionID: core.TaskDefinitionID(ids[0])}}
	if err = command.apply(k); err != nil {
		return "", err
	}
	captured, err := captureTitleHistory(s.snapshot, s.graph, k.Snapshot(), command)
	if err != nil {
		return "", err
	}
	wire, graph, err := normalizeSnapshot(captured)
	if err != nil {
		return "", err
	}
	// Derive the acknowledgment from the normalized candidate before any write.
	// Its bytes are returned only after prefix verification or durable commit.
	acceptedTitle, found := "", false
	for _, definition := range graph.Definitions {
		if definition.ChainID == chain && definition.SupersededAt == nil {
			acceptedTitle, found = definition.Title, true
			break
		}
	}
	if !found {
		return "", core.ErrNotFound
	}
	if historyEqual(s.snapshot, wire) {
		// No revision consumed, but a stale/uncertain handle cannot acknowledge a save.
		_, err = s.disk.verifyPrefix(s.disk.revision, s.disk.hash, false)
		if err != nil {
			return "", err
		}
		return acceptedTitle, nil
	}
	if err = s.disk.commit(wire); err != nil {
		return "", err
	}
	s.graph, s.snapshot = graph, wire
	return acceptedTitle, nil
}

// captureTitleHistory audits precisely one replayable title command. The strict
// settlement capture remains unchanged. Missing audit is never reconstructed.
func captureTitleHistory(prior canonical.Value, before, after core.TaskGraph, command titleCommand) (canonical.Value, error) {
	fail := func(e error) (canonical.Value, error) { return canonical.Value{}, e }
	if err := ValidateEmptyHistory(prior); err != nil {
		return fail(err)
	}
	prior, err := inspectEmptyHistory(prior)
	if err != nil {
		return fail(err)
	}
	_, extra, err := v7.ToDomain(prior)
	if err != nil {
		return fail(err)
	}
	old, err := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	if err != nil {
		return fail(err)
	}
	if !historyEqual(prior, old) {
		return fail(historyError("prior does not match before graph"))
	}
	k, err := core.NewTaskKernel(before)
	if err != nil {
		return fail(err)
	}
	if err = command.apply(k); err != nil {
		return fail(err)
	}
	expected, err := v7.FromDomain(recurrence.State{Tasks: k.Snapshot()}, extra)
	if err != nil {
		return fail(err)
	}
	result, err := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	if err != nil {
		return fail(err)
	}
	if !historyEqual(expected, result) {
		return fail(historyError("candidate differs from title command replay"))
	}
	// Defence in depth independent of replay: all day and materialised subtask
	// bytes, all non-pending traces, and all historical definition content freeze.
	for _, section := range []string{"days", "subtasks", "classifications"} {
		if !historyEqual(prior.Object[section], result.Object[section]) {
			return fail(historyError("title changed " + section))
		}
	}
	newFacts := func(section string) map[string]canonical.Value {
		facts := map[string]canonical.Value{}
		for _, v := range result.Object[section].Array {
			facts[historyID(v.Object["id"])] = v
		}
		return facts
	}
	traces := newFacts("traces")
	historical := map[string]bool{}
	for _, v := range prior.Object["traces"].Array {
		if v.Object["status"].String != string(core.TracePending) {
			historical[historyID(v.Object["definitionID"])] = true
			if !historyEqual(v, traces[historyID(v.Object["id"])]) {
				return fail(historyError("historical trace changed"))
			}
		}
	}
	defs := newFacts("definitions")
	oldIDs := map[core.TaskDefinitionID]bool{}
	maxSequence := 0
	for _, d := range before.Definitions {
		oldIDs[d.ID] = true
		if d.ChainID == command.input.ChainID && d.Sequence > maxSequence {
			maxSequence = d.Sequence
		}
	}
	for _, v := range prior.Object["definitions"].Array {
		id := historyID(v.Object["id"])
		if !historical[id] {
			continue
		}
		n, ok := defs[id]
		if !ok {
			return fail(historyError("historical definition removed"))
		}
		if historyEqual(v, n) {
			continue
		}
		var original, successor *core.TaskDefinition
		for i := range before.Definitions {
			if string(before.Definitions[i].ID) == id {
				original = &before.Definitions[i]
			}
		}
		for i := range after.Definitions {
			if after.Definitions[i].ID == command.input.NewDefinitionID {
				successor = &after.Definitions[i]
			}
		}
		if original == nil || original.SupersededAt != nil || original.ChainID != command.input.ChainID || successor == nil || oldIDs[successor.ID] || successor.ChainID != original.ChainID || maxSequence == int(^uint(0)>>1) || successor.Sequence != maxSequence+1 || successor.CreatedAt != command.input.Now || successor.ContentUpdatedAt != command.input.Now || successor.SupersededAt != nil || successor.SupersededByDefinitionID != nil {
			return fail(historyError("invalid historical forward successor"))
		}
		// Copy only the three explicitly derived metadata values into an independent
		// old projection. A restoration witness on the old current is allowed.
		projected := historyObject(map[string]canonical.Value{})
		for key, value := range v.Object {
			projected.Object[key] = value
		}
		projected.Object["contentUpdatedAt"] = historyNumber(uint64(command.input.Now))
		projected.Object["supersededAt"] = historyNumber(uint64(command.input.Now))
		projected.Object["supersededByDefinitionID"] = historyObject(map[string]canonical.Value{"rawValue": historyString(string(successor.ID))})
		if !historyEqual(projected, n) {
			return fail(historyError("historical definition content changed"))
		}
		for _, field := range []string{"descriptionText", "plannedSubtasks"} {
			oldValue, oldPresent := v.Object[field]
			newValue, newPresent := defs[string(successor.ID)].Object[field]
			if oldPresent != newPresent || oldPresent && !historyEqual(oldValue, newValue) {
				return fail(historyError("successor content not copied"))
			}
		}
	}
	if err = ValidateEmptyHistory(result); err != nil {
		return fail(err)
	}
	return result, nil
}
