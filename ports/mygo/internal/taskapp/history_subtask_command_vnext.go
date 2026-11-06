// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/textcanon"
)

// Two closed, by-value families preserve the core command shapes. IDs come
// from the caller; these pure audits do not allocate or authorize a disk write.
type plannedSubtaskHistoryInput core.PlannedSubtaskEditInput
type actualSubtaskHistoryInput core.SubtaskEditInput

func auditPlannedSubtaskHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in plannedSubtaskHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandPlannedSubtaskInputBudget(in); err != nil {
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
	if err = k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)); err != nil {
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
	if !historyPlannedSubtaskFence(prior, final, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	// Now is request identity, not evidence that a no-op consumed a clock.
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandPlannedSubtaskEdit, subtaskEditKind: in.Kind, chain: in.ChainID, expectedDefinition: in.ExpectedDefinitionID, plannedChild: in.SubtaskID, requestedLineage: in.LineageID, requestedTitle: in.Title, requestedDifficulty: in.Difficulty, now: in.Now, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

func auditActualSubtaskHistoryVNext(prior validatedHistorySnapshot, before, after core.TaskGraph, in actualSubtaskHistoryInput) (auditedHistoryCommand, error) {
	if !prior.valid() {
		return auditedHistoryCommand{}, historyVNextFail(historyProof)
	}
	if _, err := historyCommandGraphBudget(before, historyCommandBefore); err != nil {
		return auditedHistoryCommand{}, err
	}
	if _, err := historyCommandGraphBudget(after, historyCommandAfter); err != nil {
		return auditedHistoryCommand{}, err
	}
	if err := historyCommandActualSubtaskInputBudget(in); err != nil {
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
	if err = k.EditSubtask(core.SubtaskEditInput(in)); err != nil {
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
	if !historyActualSubtaskFence(prior, final, in) {
		return auditedHistoryCommand{}, historyCommandFail(historyCommandFence)
	}
	return auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandActualSubtaskEdit, subtaskEditKind: in.Kind, chain: in.ChainID, trace: in.TraceID, actualChild: in.SubtaskID, requestedLineage: in.LineageID, requestedTitle: in.Title, requestedDifficulty: in.Difficulty, requestedCancellation: in.CancellationID, today: in.Today, now: in.Now, prior: prior, after: final, changed: !historyEqual(prior.proof.snapshot, final.proof.snapshot)}}, nil
}

// Fences construct a private canonical expectation from the sealed prior. No
// caller-selected fields or projections are accepted. Full canonical equality
// preserves order, optional presence, unknown fields, and exact Instant bits.
func historyPlannedSubtaskFence(prior, final validatedHistorySnapshot, in plannedSubtaskHistoryInput) bool {
	if !prior.valid() || !final.valid() || prior.proof.count != final.proof.count {
		return false
	}
	before, after := prior.proof.snapshot, final.proof.snapshot
	chains, defs := before.Object["chains"].Array, before.Object["definitions"].Array
	nextDefs := after.Object["definitions"].Array
	if len(chains) != len(after.Object["chains"].Array) || len(defs) != len(nextDefs) {
		return false
	}
	ci, di := historySubtaskIndex(chains, string(in.ChainID)), historySubtaskIndex(defs, string(in.ExpectedDefinitionID))
	if ci < 0 || di < 0 || historyID(defs[di].Object["chainID"]) != string(in.ChainID) {
		return false
	}
	// FormsDayHistory includes pending; only cancelledDraft is excluded. Freeze
	// the entire referenced definition, not a title projection or only its body.
	historical := make(map[string]bool)
	for _, tr := range before.Object["traces"].Array {
		if core.TraceStatus(tr.Object["status"].String).FormsDayHistory() {
			historical[historyID(tr.Object["definitionID"])] = true
		}
	}
	for i, d := range defs {
		if historyID(d.Object["id"]) != historyID(nextDefs[i].Object["id"]) || (historical[historyID(d.Object["id"])] && !historyEqual(d, nextDefs[i])) {
			return false
		}
	}
	old := defs[di].Object["plannedSubtasks"].Array
	si := historySubtaskIndex(old, string(in.SubtaskID))
	title := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(in.Title, "\r\n", "\n"), "\r", "\n"))
	switch in.Kind {
	case core.SubtaskAdd:
		if si >= 0 {
			return false
		}
	case core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove:
		if si < 0 {
			return false
		}
	default:
		return false
	}
	if (in.Kind == core.SubtaskTitle && textcanon.Equivalent(old[si].Object["title"].String, title)) || (in.Kind == core.SubtaskDifficulty && historyEqual(old[si].Object["difficulty"], historyNumber(uint64(in.Difficulty)))) {
		return historyEqual(before, after)
	}
	expected := historyVNextClone(before)
	d := expected.Object["definitions"].Array[di].Object
	children := d["plannedSubtasks"].Array
	switch in.Kind {
	case core.SubtaskAdd:
		children = append(children, historyObject(map[string]canonical.Value{"id": historySubtaskID(string(in.SubtaskID)), "lineageID": historySubtaskID(string(in.LineageID)), "title": historyString(title), "difficulty": historyNumber(uint64(in.Difficulty)), "position": historyNumber(uint64(len(old) + 1)), "createdAt": historyNumber(uint64(in.Now))}))
	case core.SubtaskTitle:
		children[si].Object["title"] = historyString(title)
	case core.SubtaskDifficulty:
		children[si].Object["difficulty"] = historyNumber(uint64(in.Difficulty))
	case core.PlannedSubtaskRemove:
		children = append(children[:si], children[si+1:]...)
		for i := range children {
			children[i].Object["position"] = historyNumber(uint64(i + 1))
		}
	}
	d["plannedSubtasks"] = canonical.Value{Kind: canonical.Array, Array: children}
	d["contentUpdatedAt"] = historyNumber(uint64(in.Now))
	expected.Object["chains"].Array[ci].Object["updatedAt"] = historyNumber(uint64(in.Now))
	return historyEqual(expected, after)
}

func historyActualSubtaskFence(prior, final validatedHistorySnapshot, in actualSubtaskHistoryInput) bool {
	if !prior.valid() || !final.valid() || prior.proof.count != final.proof.count {
		return false
	}
	before, after := prior.proof.snapshot, final.proof.snapshot
	traces := before.Object["traces"].Array
	ti := historySubtaskIndex(traces, string(in.TraceID))
	if ti < 0 || historyID(traces[ti].Object["chainID"]) != string(in.ChainID) {
		return false
	}
	old := before.Object["subtasks"].Array
	si := historySubtaskIndex(old, string(in.SubtaskID))
	title := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(in.Title, "\r\n", "\n"), "\r", "\n"))
	switch in.Kind {
	case core.SubtaskAdd:
		if si >= 0 {
			return false
		}
	case core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon:
		if si < 0 || historyID(old[si].Object["traceID"]) != string(in.TraceID) {
			return false
		}
	default:
		return false
	}
	if (in.Kind == core.SubtaskTitle && textcanon.Equivalent(old[si].Object["title"].String, title)) || (in.Kind == core.SubtaskDifficulty && historyEqual(old[si].Object["difficulty"], historyNumber(uint64(in.Difficulty)))) {
		return historyEqual(before, after)
	}
	expected := historyVNextClone(before)
	children := expected.Object["subtasks"].Array
	if in.Kind == core.SubtaskAdd {
		count := 0
		for _, s := range old {
			if historyID(s.Object["traceID"]) == string(in.TraceID) {
				count++
			}
		}
		children = append(children, historyObject(map[string]canonical.Value{"id": historySubtaskID(string(in.SubtaskID)), "lineageID": historySubtaskID(string(in.LineageID)), "traceID": historySubtaskID(string(in.TraceID)), "title": historyString(title), "difficulty": historyNumber(uint64(in.Difficulty)), "status": historyString(string(core.SubtaskPending)), "position": historyNumber(uint64(count + 1)), "createdAt": historyNumber(uint64(in.Now)), "updatedAt": historyNumber(uint64(in.Now))}))
	} else {
		s := children[si].Object
		switch in.Kind {
		case core.SubtaskTitle:
			s["title"] = historyString(title)
		case core.SubtaskDifficulty:
			s["difficulty"] = historyNumber(uint64(in.Difficulty))
		case core.SubtaskDelete:
			s["status"] = historyString(string(core.SubtaskCancelledDraft))
			delete(s, "completedAt")
			s["settledAt"] = historyNumber(uint64(in.Now))
			s["draftCancellationID"] = historyString(string(in.CancellationID))
		case core.SubtaskComplete:
			s["status"] = historyString(string(core.SubtaskCompleted))
			s["completedAt"] = historyNumber(uint64(in.Now))
		case core.SubtaskUndoComplete:
			s["status"] = historyString(string(core.SubtaskPending))
			delete(s, "completedAt")
		case core.SubtaskAbandon:
			s["status"] = historyString(string(core.SubtaskAbandoned))
			s["settledAt"] = historyNumber(uint64(in.Now))
		}
		s["updatedAt"] = historyNumber(uint64(in.Now))
	}
	expected.Object["subtasks"] = canonical.Value{Kind: canonical.Array, Array: children}
	return historyEqual(expected, after)
}

func historySubtaskIndex(values []canonical.Value, id string) int {
	for i, v := range values {
		if historyID(v.Object["id"]) == id {
			return i
		}
	}
	return -1
}
func historySubtaskID(id string) canonical.Value {
	return historyObject(map[string]canonical.Value{"rawValue": historyString(id)})
}
