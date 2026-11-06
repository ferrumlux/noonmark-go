// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// These pure, package-private adapters are deliberately NOT wired to Service,
// store, v2 or codec callbacks. A proof of one exact edge is not permission to
// write, a session binding, a global mutation clock, or historical reachability.
// Callers must not mutate inputs (including shared leaves) during an audit.
type settlementHistoryInput struct {
	Today    core.LocalDate
	Now      core.Instant
	EventIDs []string
}
type completionHistoryAction uint8

const (
	completionHistoryComplete completionHistoryAction = iota + 1
	completionHistoryUndo
)

type completionHistoryInput struct {
	Action  completionHistoryAction
	ChainID core.TaskChainID
	TraceID core.DayTraceID
	Today   core.LocalDate
	Now     core.Instant
}
type historyCommandKind uint8

const (
	historyCommandSettlement historyCommandKind = iota + 1
	historyCommandComplete
	historyCommandUndo
	historyCommandAbandon
	historyCommandReactivate
	historyCommandRename
	historyCommandTitleInput
	historyCommandManualProgress
	historyCommandPlannedSubtaskEdit
	historyCommandActualSubtaskEdit
	historyCommandReturn
	historyCommandDefer
	historyCommandContinue
)

// Only successful closed audit entries construct this result. Neither
// caller-selected captures nor mutable proofs are returned. We retain scalar
// input identity and immutable, owned endpoints, not the caller's EventIDs.
type auditedHistoryCommand struct{ proof *historyCommandProof }
type historyCommandProof struct {
	seal                     *historyProfileSeal
	kind                     historyCommandKind
	today                    core.LocalDate
	now                      core.Instant
	chain                    core.TaskChainID
	trace                    core.DayTraceID
	effectiveTrace           core.DayTraceID
	eventID                  string
	prior, after             validatedHistorySnapshot
	titleAction              titleHistoryAction
	titleRoute               titleHistoryRoute
	requestedTitle           string
	requestedNewDefinitionID core.TaskDefinitionID
	requestedPercent         int
	subtaskEditKind          core.SubtaskEditKind
	expectedDefinition       core.TaskDefinitionID
	plannedChild             core.PlannedSubtaskID
	actualChild              core.SubtaskID
	requestedLineage         core.SubtaskLineageID
	requestedDifficulty      core.Difficulty
	requestedCancellation    core.CancellationID
	returnDetail             *returnHistoryInput
	deferDetail              *deferHistoryInput
	continueDetail           *continueHistoryInput
	changed                  bool
}

func (r auditedHistoryCommand) valid() bool {
	if r.proof == nil || r.proof.seal != forwardClockOrdinaryHistoryVNext {
		return false
	}
	switch r.proof.kind {
	case historyCommandSettlement, historyCommandComplete, historyCommandUndo, historyCommandAbandon, historyCommandReactivate, historyCommandRename, historyCommandTitleInput, historyCommandManualProgress, historyCommandPlannedSubtaskEdit, historyCommandActualSubtaskEdit:
		return r.proof.prior.valid() && r.proof.after.valid() && r.proof.returnDetail == nil && r.proof.deferDetail == nil && r.proof.continueDetail == nil
	case historyCommandReturn:
		return r.proof.prior.valid() && r.proof.after.valid() && r.proof.returnDetail != nil && r.proof.deferDetail == nil && r.proof.continueDetail == nil
	case historyCommandDefer:
		return r.proof.prior.valid() && r.proof.after.valid() && r.proof.returnDetail == nil && r.proof.deferDetail != nil && r.proof.continueDetail == nil
	case historyCommandContinue:
		return r.proof.prior.valid() && r.proof.after.valid() && r.proof.returnDetail == nil && r.proof.deferDetail == nil && r.proof.continueDetail != nil
	default:
		return false
	}
}
func (r auditedHistoryCommand) snapshotCopy() (canonical.Value, error) {
	if !r.valid() {
		return canonical.Value{}, historyVNextFail(historyProof)
	}
	return r.proof.after.snapshotCopy()
}
func (r auditedHistoryCommand) changed() (bool, error) {
	if !r.valid() {
		return false, historyVNextFail(historyProof)
	}
	return r.proof.changed, nil
}

type historyCommandReason string

const (
	historyCommandBinding historyCommandReason = "before mismatch"
	historyCommandReplay  historyCommandReason = "after mismatch"
	historyCommandInput   historyCommandReason = "invalid input"
	historyCommandOwner   historyCommandReason = "owner mismatch"
	historyCommandCount   historyCommandReason = "capture count mismatch"
	historyCommandFence   historyCommandReason = "frozen facts changed"
)

type historyCommandError struct{ Reason historyCommandReason }

func (e *historyCommandError) Error() string          { return "candidate history command: " + string(e.Reason) }
func historyCommandFail(r historyCommandReason) error { return &historyCommandError{r} }

func auditSettlementHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in settlementHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	// Do not move binding, validation, copying, indexing or string scans above
	// these three straight-line fail-closed preflights. See the AST order gate.
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandEventIDBudget(prior, in.EventIDs); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !in.Today.Valid() {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandInput)
	}
	if !in.Now.Finite() {
		return auditedHistoryCommand{}, core.ErrInvalidClock
	}
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if err = k.SettleTaskDays(in.Today, in.Now); err != nil {
		return auditedHistoryCommand{}, err
	}
	replay := k.Snapshot()
	if err = historyCommandMatch(replay, after, extra); err != nil {
		return auditedHistoryCommand{}, err
	}
	captures := historyCommandSettlementCaptures(owned, replay)
	if len(in.EventIDs) != len(captures) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandCount)
	}
	intents := make([]historyCaptureIntent, len(captures))
	for i, t := range captures {
		intents[i] = historyCaptureIntent{EventID: in.EventIDs[i], TraceID: t.ID, Status: t.Status, At: in.Now}
	}
	final, err := historyCommandFinish(prior, replay, extra, intents)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	// Independent fixed fences complement full replay, never grant exemptions.
	for _, key := range []string{"chains", "definitions"} {
		if !historyEqual(prior.proof.snapshot.Object[key], final.proof.snapshot.Object[key]) {
			return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
		}
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandSettlement, today: in.Today, now: in.Now, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func auditCompletionHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in completionHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandCompletionIDBudget(in); err != nil {
		return auditedHistoryCommand{}, err
	}
	owned, extra, err := historyCommandBind(prior, before)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	if !in.Today.Valid() || (in.Action != completionHistoryComplete && in.Action != completionHistoryUndo) ||
		!historyVNextUUID(string(in.ChainID)) || !historyVNextUUID(string(in.TraceID)) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandInput)
	}
	if !in.Now.Finite() {
		return auditedHistoryCommand{}, core.ErrInvalidClock
	}
	found := false
	for _, t := range owned.Traces {
		if t.ID == in.TraceID {
			if t.ChainID != in.ChainID {
				return auditedHistoryCommand{}, historyCommandFail(historyCommandOwner)
			}
			found = true
			break
		}
	}
	if !found {
		return auditedHistoryCommand{}, core.ErrNotFound
	}
	k, err := core.NewTaskKernel(owned)
	if err != nil {
		return auditedHistoryCommand{}, err
	}
	kind := historyCommandComplete
	switch in.Action {
	case completionHistoryComplete:
		err = k.MarkCompleted(in.TraceID, in.Today, in.Now)
	case completionHistoryUndo:
		kind = historyCommandUndo
		err = k.UndoCompleted(in.TraceID, in.Today, in.Now)
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
	for _, key := range []string{"days", "definitions", "subtasks", "classifications"} {
		if !historyEqual(prior.proof.snapshot.Object[key], final.proof.snapshot.Object[key]) {
			return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
		}
	}
	for _, key := range []string{"chains", "traces"} {
		old, next := prior.proof.snapshot.Object[key].Array, final.proof.snapshot.Object[key].Array
		if len(old) != len(next) {
			return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
		}
		target := string(in.TraceID)
		if key == "chains" {
			target = string(in.ChainID)
		}
		for i, v := range old {
			if historyID(v.Object["id"]) != target && !historyEqual(v, next[i]) {
				return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
			}
		}
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, today: in.Today, now: in.Now, chain: in.ChainID, trace: in.TraceID, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

// Only reached after BOTH caller graphs AND all input IDs pass metadata gates.
// Actual replay uses this owned graph decoded from the sealed prior, not before.
func historyCommandBind(prior validatedHistorySnapshot, before core.TaskGraph) (core.TaskGraph, v7.Supplement, error) {
	snapshot, err := prior.snapshotCopy()
	if err != nil {
		return core.TaskGraph{}, v7.Supplement{}, err
	}
	state, extra, err := v7.ToDomain(snapshot)
	if err != nil {
		return core.TaskGraph{}, v7.Supplement{}, err
	}
	bound, err := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	if err != nil {
		return core.TaskGraph{}, v7.Supplement{}, err
	}
	if !historyEqual(snapshot, bound) {
		return core.TaskGraph{}, v7.Supplement{}, historyCommandFail(historyCommandBinding)
	}
	// A bridge that normalizes task rawValue is not an exact input binding. Old
	// classification bytes stay in Supplement and are never normalized here.
	roundtrip, err := v7.FromDomain(state, extra)
	if err != nil {
		return core.TaskGraph{}, v7.Supplement{}, err
	}
	if !historyEqual(snapshot, roundtrip) {
		return core.TaskGraph{}, v7.Supplement{}, historyCommandFail(historyCommandBinding)
	}
	return state.Tasks, extra, nil
}
func historyCommandMatch(replay, after core.TaskGraph, extra v7.Supplement) error {
	expected, err := v7.FromDomain(recurrence.State{Tasks: replay}, extra)
	if err != nil {
		return err
	}
	actual, err := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	if err != nil {
		return err
	}
	if !historyEqual(expected, actual) {
		return historyCommandFail(historyCommandReplay)
	}
	return nil
}

func historyCommandSettlementCaptures(before, after core.TaskGraph) []core.DayTrace {
	locks := make(map[core.LocalDate]bool, len(before.Days))
	for _, day := range before.Days {
		locks[day.Date] = day.LockedAt != nil
	}
	for _, day := range after.Days {
		locks[day.Date] = !locks[day.Date] && day.LockedAt != nil
	}
	var captures []core.DayTrace
	for _, t := range before.Traces {
		if !locks[t.Date] {
			continue
		}
		switch t.Status {
		case core.TracePending:
			t.Status = core.TraceUnfinished
			captures = append(captures, t)
		case core.TraceCompleted:
			captures = append(captures, t)
		}
	}
	sort.Slice(captures, func(i, j int) bool {
		if d := captures[i].Date.Compare(captures[j].Date); d != 0 {
			return d < 0
		}
		return captures[i].ID < captures[j].ID
	})
	return captures
}

// Always append -> compose -> full inspector -> prefix -> fixed supplement and
// exact capture-count fence. No callback, bypass flag, generic frozen exemption,
// or classification-only candidate can escape as a complete command result.
func historyCommandFinish(prior validatedHistorySnapshot, replay core.TaskGraph, extra v7.Supplement, intents []historyCaptureIntent) (validatedHistorySnapshot, error) {
	candidate, err := appendEmptyHistoryEvents(prior, intents)
	if err != nil {
		return validatedHistorySnapshot{}, err
	}
	extra.Classifications, err = candidate.classificationsCopy()
	if err != nil {
		return validatedHistorySnapshot{}, err
	}
	snapshot, err := v7.FromDomain(recurrence.State{Tasks: replay}, extra)
	if err != nil {
		return validatedHistorySnapshot{}, err
	}
	final, err := inspectForwardClockOrdinaryHistoryVNext(snapshot)
	if err != nil {
		return validatedHistorySnapshot{}, err
	}
	if err = checkForwardHistorySuccessor(prior, final); err != nil {
		return validatedHistorySnapshot{}, err
	}
	for _, key := range []string{"preferences", "ideas", "taskCycleSeries"} {
		if !historyEqual(prior.proof.snapshot.Object[key], final.proof.snapshot.Object[key]) {
			return validatedHistorySnapshot{}, historyCommandFail(historyCommandFence)
		}
	}
	expected, ok := historyCheckedAdd(prior.proof.count, uint64(len(intents)), ^uint64(0))
	if !ok || final.proof.count != expected {
		return validatedHistorySnapshot{}, historyCommandFail(historyCommandCount)
	}
	return final, nil
}
