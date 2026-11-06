// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

// These closed inputs do not authorize persistence. During the call, callers
// must not concurrently mutate graphs, maps, or shared leaves.
type returnHistoryInput struct {
	TraceID         core.DayTraceID
	ChainID         core.TaskChainID
	NewDefinitionID core.TaskDefinitionID
	CancellationID  core.CancellationID
	PlannedIDs      map[core.SubtaskID]core.PlannedSubtaskID
	Today           core.LocalDate
	Now             core.Instant
	EventID         string
}
type deferHistoryInput struct {
	ChainID             core.TaskChainID
	TraceID, NewTraceID core.DayTraceID
	DayID               core.DayID
	SubtaskIDs          map[core.SubtaskID]core.SubtaskID
	TargetDate, Today   core.LocalDate
	Now                 core.Instant
	EventID             string
}
type continueHistoryInput struct {
	ChainID             core.TaskChainID
	TraceID, NewTraceID core.DayTraceID
	DayID               core.DayID
	SubtaskIDs          map[core.SubtaskID]core.SubtaskID
	TargetDate, Today   core.LocalDate
	Now                 core.Instant
}

func auditReturnHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in returnHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandReturnInputBudget(prior, in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	in.PlannedIDs = historyRemainingCopyIDs(in.PlannedIDs)
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = k.ReturnToPool(core.ReturnToPoolInput{TraceID: in.TraceID, ChainID: in.ChainID, NewDefinitionID: in.NewDefinitionID, CancellationID: in.CancellationID, PlannedIDs: in.PlannedIDs, Today: in.Today, Now: in.Now}); err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	var intents []historyCaptureIntent
	// Route only from the successful owned replay, never a caller switch.
	for _, t := range replay.Traces {
		if t.ID != in.TraceID {
			continue
		}
		if t.Status == core.TraceReturnedToPool {
			if in.EventID == "" {
				return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
			}
			intents = []historyCaptureIntent{{EventID: in.EventID, TraceID: in.TraceID, Status: core.TraceReturnedToPool, At: in.Now}}
		} else if t.Status != core.TraceCancelledDraft || in.EventID != "" {
			return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
		}
	}
	final, err := historyCommandFinish(prior, replay, extra, intents)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !historyReturnFence(prior, final, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandReturn, returnDetail: &in, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func auditDeferHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in deferHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandDeferInputBudget(prior, in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	in.SubtaskIDs = historyRemainingCopyIDs(in.SubtaskIDs)
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = k.DeferCurrentTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, SubtaskIDs: in.SubtaskIDs, TargetDate: in.TargetDate, Today: in.Today, Now: in.Now}); err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	if in.EventID == "" {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
	}
	intents := []historyCaptureIntent{{EventID: in.EventID, TraceID: in.TraceID, Status: core.TraceDeferred, At: in.Now}}
	final, err := historyCommandFinish(prior, replay, extra, intents)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !historyDeferFence(prior, final, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandDefer, deferDetail: &in, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func auditContinueHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in continueHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandContinueInputBudget(prior, in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	in.SubtaskIDs = historyRemainingCopyIDs(in.SubtaskIDs)
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, SubtaskIDs: in.SubtaskIDs, TargetDate: in.TargetDate, Today: in.Today, Now: in.Now}); err != nil {
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
	if !historyContinueFence(prior, final, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandContinue, continueDetail: &in, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func historyRemainingCopyIDs[K ~string, V ~string](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
