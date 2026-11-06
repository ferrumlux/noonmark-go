// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
)

type titleHistoryAction uint8

const (
	titleHistoryRename titleHistoryAction = iota + 1
	titleHistorySaveInput
)

type titleHistoryRoute uint8

const (
	titleHistoryChain titleHistoryRoute = iota + 1
	titleHistoryTrace
)

type titleHistoryInput struct {
	Action          titleHistoryAction
	Route           titleHistoryRoute
	ChainID         core.TaskChainID
	TraceID         core.DayTraceID
	Title           string
	Today           core.LocalDate
	Now             core.Instant
	NewDefinitionID core.TaskDefinitionID
}
type manualProgressHistoryInput struct {
	ChainID core.TaskChainID
	TraceID core.DayTraceID
	Percent int
	Today   core.LocalDate
	Now     core.Instant
}

func auditTitleHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in titleHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandTitleInputBudget(in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !in.Today.Valid() || !historyVNextUUID(string(in.ChainID)) || (in.Action != titleHistoryRename && in.Action != titleHistorySaveInput) || (in.Route != titleHistoryChain && in.Route != titleHistoryTrace) || (in.Route == titleHistoryChain && in.TraceID != "") || (in.Route == titleHistoryTrace && (in.Action != titleHistorySaveInput || !historyVNextUUID(string(in.TraceID)))) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandInput)
	}
	if in.Route == titleHistoryTrace {
		found := false
		for _, tr := range owned.Traces {
			if tr.ID == in.TraceID {
				if tr.ChainID != in.ChainID {
					return auditedHistoryCommand{}, historyCommandFail(historyCommandOwner)
				}
				if tr.Date.Compare(in.Today) < 0 {
					return auditedHistoryCommand{}, core.ErrImmutableHistory
				}
				if tr.Status != core.TracePending {
					return auditedHistoryCommand{}, core.ErrInvalidTransition
				}
				found = true
				break
			}
		}
		if !found {
			return auditedHistoryCommand{}, core.ErrNotFound
		}
	}
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	input := core.EditTaskTitleInput{ChainID: in.ChainID, Title: in.Title, Today: in.Today, Now: in.Now, NewDefinitionID: in.NewDefinitionID}
	kind := historyCommandTitleInput
	switch in.Action {
	case titleHistoryRename:
		kind = historyCommandRename
		err = k.RenameTaskTitle(input)
	case titleHistorySaveInput:
		err = k.SaveTaskTitleInput(input)
	}
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	final, err := historyCommandFinish(prior, replay, extra, nil)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !historyEditSupplementFence(prior, final) || !historyTitleFence(owned, replay, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	// now and requested fields identify the request, not consumed clocks or IDs.
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, titleAction: in.Action, titleRoute: in.Route, requestedTitle: in.Title, requestedNewDefinitionID: in.NewDefinitionID, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}
func auditManualProgressHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in manualProgressHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandManualProgressInputBudget(in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	err = k.SetManualProgress(core.SetManualProgressInput(in))
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	final, err := historyCommandFinish(prior, replay, extra, nil)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !historyEditSupplementFence(prior, final) || !historyManualProgressFence(owned, replay, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	// now and requested fields identify the request, not consumed clocks or IDs.
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandManualProgress, requestedPercent: in.Percent, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

// The top-level key set and every non-task supplement are fixed. Finish also
// checks the full inspector, prefix, capture count and supplement fences.
func historyEditSupplementFence(a, b validatedHistorySnapshot) bool {
	x, y := a.proof.snapshot.Object, b.proof.snapshot.Object
	if len(x) != len(y) || a.proof.count != b.proof.count {
		return false
	}
	for key, v := range x {
		n, ok := y[key]
		if !ok {
			return false
		}
		switch key {
		case "chains", "definitions", "traces", "days", "subtasks":
		default:
			if !historyEqual(v, n) {
				return false
			}
		}
	}
	return true
}

// Build a private fixed-field projection from the owned endpoint. No caller
// allowlist exists. Exact replay separately fixes all accepted changed values.
func historyManualProgressFence(before, after core.TaskGraph, in manualProgressHistoryInput) bool {
	expected := before.Clone()
	if len(before.Traces) != len(after.Traces) {
		return false
	}
	found := false
	for i, tr := range before.Traces {
		if tr.ID != after.Traces[i].ID {
			return false
		}
		if tr.ID == in.TraceID {
			if tr.ChainID != in.ChainID {
				return false
			}
			found = true
			expected.Traces[i].ManualProgressPercent = after.Traces[i].ManualProgressPercent
			expected.Traces[i].ContentUpdatedAt = after.Traces[i].ContentUpdatedAt
		}
	}
	return found && reflect.DeepEqual(expected, after)
}

func historyTitleFence(before, after core.TaskGraph, in titleHistoryInput) bool {
	if len(before.Chains) != len(after.Chains) || len(before.Traces) != len(after.Traces) {
		return false
	}
	current := -1
	maxSequence := 0
	chain := -1
	for i, c := range before.Chains {
		if c.ID == in.ChainID {
			chain = i
		}
	}
	for i, d := range before.Definitions {
		if d.ChainID == in.ChainID {
			if d.SupersededAt == nil {
				current = i
			}
			if d.Sequence > maxSequence {
				maxSequence = d.Sequence
			}
		}
	}
	if current < 0 || chain < 0 {
		return false
	}
	n := len(after.Definitions) - len(before.Definitions)
	if n != 0 && n != 1 {
		return false
	}
	expected := before.Clone()
	old := before.Definitions[current]
	// No-op must not read Now, even when its bits represent NaN or infinity.
	if n == 0 && reflect.DeepEqual(before.Definitions, after.Definitions) {
		return reflect.DeepEqual(expected, after)
	}
	if n == 0 {
		expected.Definitions[current].Title = after.Definitions[current].Title
		expected.Definitions[current].ContentUpdatedAt = in.Now
	} else {
		if maxSequence == int(^uint(0)>>1) {
			return false
		}
		next := core.TaskDefinition{ID: in.NewDefinitionID, ChainID: in.ChainID, Sequence: maxSequence + 1, Title: after.Definitions[len(before.Definitions)].Title, DescriptionText: old.DescriptionText, PlannedSubtasks: old.PlannedSubtasks, CreatedAt: in.Now, ContentUpdatedAt: in.Now}
		expected.Definitions[current].ContentUpdatedAt = in.Now
		expected.Definitions[current].SupersededAt = &in.Now
		expected.Definitions[current].SupersededByDefinitionID = &in.NewDefinitionID
		expected.Definitions = append(expected.Definitions, next)
		for i, tr := range expected.Traces {
			if tr.ChainID == in.ChainID && tr.Status == core.TracePending {
				expected.Traces[i].DefinitionID = in.NewDefinitionID
				expected.Traces[i].ContentUpdatedAt = in.Now
			}
		}
	}
	// Strict numerical comparison retains old bits for the +0/-0 tie.
	if in.Now.After(before.Chains[chain].UpdatedAt) {
		expected.Chains[chain].UpdatedAt = in.Now
	}
	return reflect.DeepEqual(expected, after)
}
