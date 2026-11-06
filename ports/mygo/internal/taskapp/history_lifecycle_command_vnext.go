// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

// Pure, unconnected lifecycle edge audits. These do not establish a current
// head, write authority, global mutation frontier or whole-history reachability.
type abandonHistoryInput struct {
	ChainID core.TaskChainID
	TraceID core.DayTraceID
	Now     core.Instant
	EventID string
}
type reactivateHistoryInput struct {
	ChainID  core.TaskChainID
	TraceID  core.DayTraceID
	Today    core.LocalDate
	Now      core.Instant
	EventIDs []string
}

// Metadata only: at most three string headers, no caller-sized allocation or
// UUID content scans. Count is checked before accessing the optional element.
func historyCommandAbandonIDBudget(in abandonHistoryInput) error {
	if len(in.ChainID) != 36 || len(in.TraceID) != 36 || len(in.EventID) != 36 {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDLength)
	}
	return nil
}
func historyCommandReactivateIDBudget(in reactivateHistoryInput) error {
	if len(in.EventIDs) > 1 {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDCount)
	}
	if len(in.ChainID) != 36 || len(in.TraceID) != 36 {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDLength)
	}
	if len(in.EventIDs) == 1 && len(in.EventIDs[0]) != 36 {
		return historyCommandBudgetFail(historyCommandIDs, historyCommandIDLength)
	}
	return nil
}

func auditAbandonHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in abandonHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandAbandonIDBudget(in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !historyVNextUUID(string(in.ChainID)) || !historyVNextUUID(string(in.TraceID)) || !historyVNextUUID(in.EventID) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandInput)
	}
	if !in.Now.Finite() {
		return auditedHistoryCommand{}, core.ErrInvalidClock
	}
	// Owner is established on the requested identity BEFORE pending redirection.
	target, err := historyLifecycleRequestedTrace(owned, in.ChainID, in.TraceID)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	for i, t := range owned.Traces {
		if t.ChainID == in.ChainID && t.Status == core.TracePending {
			target = i
			break
		}
	}
	effective := owned.Traces[target].ID
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = k.AbandonChain(core.AbandonChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Now: in.Now}); err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if replay.Traces[target].ID != effective || replay.Traces[target].Status != core.TraceAbandoned {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandReplay)
	}
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	final, err := historyCommandFinish(prior, replay, extra, []historyCaptureIntent{{EventID: in.EventID, TraceID: effective, Status: core.TraceAbandoned, At: in.Now}})
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = historyLifecycleFence(prior, final, in.ChainID, effective); err != nil {
		return auditedHistoryCommand{}, err
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandAbandon, now: in.Now, chain: in.ChainID, trace: in.TraceID, effectiveTrace: effective, eventID: in.EventID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func auditReactivateHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in reactivateHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandReactivateIDBudget(in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !in.Today.Valid() || !historyVNextUUID(string(in.ChainID)) || !historyVNextUUID(string(in.TraceID)) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandInput)
	}
	if !in.Now.Finite() {
		return auditedHistoryCommand{}, core.ErrInvalidClock
	}
	target, err := historyLifecycleRequestedTrace(owned, in.ChainID, in.TraceID)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	source := owned.Traces[target]
	// Derive the branch solely from owned prior. There is intentionally no
	// latest-A rule and no pending redirection for reactivation.
	status := core.TracePending
	for _, d := range owned.Days {
		if d.Date == source.Date && d.LockedAt != nil {
			status = core.TraceUnfinished
			break
		}
	}
	if source.Date.Compare(in.Today) < 0 {
		status = core.TraceUnfinished
	}
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	effective, err := k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Today: in.Today, Now: in.Now})
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if effective != in.TraceID || replay.Traces[target].ID != in.TraceID || replay.Traces[target].Status != status {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandReplay)
	}
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	var intents []historyCaptureIntent
	eventID := ""
	if status == core.TraceUnfinished {
		if len(in.EventIDs) != 1 {
			return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
		}
		eventID = in.EventIDs[0]
		intents = []historyCaptureIntent{{EventID: eventID, TraceID: in.TraceID, Status: core.TraceUnfinished, At: in.Now}}
	} else if len(in.EventIDs) != 0 {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
	}
	final, err := historyCommandFinish(prior, replay, extra, intents)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = historyLifecycleFence(prior, final, in.ChainID, in.TraceID); err != nil {
		return auditedHistoryCommand{}, err
	}
	if status == core.TracePending && !historyEqual(prior.proof.snapshot.Object["classifications"], final.proof.snapshot.Object["classifications"]) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandReactivate, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, effectiveTrace: effective, eventID: eventID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

// Used only after all metadata gates and exact binding to the owned prior.
func historyLifecycleRequestedTrace(g core.TaskGraph, chain core.TaskChainID, trace core.DayTraceID) (int, error) {
	for i, t := range g.Traces {
		if t.ID == trace {
			if t.ChainID != chain {
				return -1, historyCommandFail(historyCommandOwner)
			}
			return i, nil
		}
	}
	return -1, core.ErrNotFound
}

// Fixed lifecycle fences supplement, never replace, whole-graph typed replay.
// No callback or caller-selected list of exempt fields can weaken this fence.
func historyLifecycleFence(prior, final validatedHistorySnapshot, chain core.TaskChainID, trace core.DayTraceID) error {
	for _, key := range []string{"days", "definitions", "subtasks"} {
		if !historyEqual(prior.proof.snapshot.Object[key], final.proof.snapshot.Object[key]) {
			return historyCommandFail(historyCommandFence)
		}
	}
	for _, key := range []string{"chains", "traces"} {
		old, next := prior.proof.snapshot.Object[key].Array, final.proof.snapshot.Object[key].Array
		if len(old) != len(next) {
			return historyCommandFail(historyCommandFence)
		}
		target := string(trace)
		if key == "chains" {
			target = string(chain)
		}
		for i, v := range old {
			if historyID(v.Object["id"]) != target && !historyEqual(v, next[i]) {
				return historyCommandFail(historyCommandFence)
			}
		}
	}
	return nil
}
