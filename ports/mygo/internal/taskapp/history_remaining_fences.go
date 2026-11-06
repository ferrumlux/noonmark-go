// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"sort"
	"strings"
)

// These independent expectations read only the sealed BEFORE and the typed
// request. No kernel command or AFTER-derived projection is used. Re-encoding
// the independently constructed graph freezes every field not assigned below,
// including array positions, optional presence and Instant bits.
func historyReturnFence(prior, final validatedHistorySnapshot, in returnHistoryInput) bool {
	if !prior.valid() || !final.valid() {
		return false
	}
	state, extra, err := v7.ToDomain(prior.proof.snapshot)
	if err != nil {
		return false
	}
	g := state.Tasks
	ti, ci, di := historyRemainingIndices(g, in.TraceID, in.ChainID)
	if ti < 0 || ci < 0 || di < 0 {
		return false
	}
	source := g.Traces[ti]
	if source.ChainID != in.ChainID {
		return false
	}
	maxSeq := 0
	for _, d := range g.Definitions {
		if d.ChainID == in.ChainID && d.Sequence > maxSeq {
			maxSeq = d.Sequence
		}
	}
	if maxSeq == int(^uint(0)>>1) {
		return false
	}
	open := historyRemainingOpen(g, source.ID)
	if len(open) != len(in.PlannedIDs) {
		return false
	}
	planned := make([]core.PlannedSubtask, 0, len(open))
	for i, s := range open {
		id, ok := in.PlannedIDs[s.ID]
		if !ok {
			return false
		}
		planned = append(planned, core.PlannedSubtask{ID: id, LineageID: s.LineageID, Title: s.Title, Difficulty: s.Difficulty, Position: i + 1, CreatedAt: in.Now})
	}
	next := core.TaskDefinition{ID: in.NewDefinitionID, ChainID: in.ChainID, Sequence: maxSeq + 1, Title: g.Definitions[di].Title, DescriptionText: source.DescriptionText, PlannedSubtasks: planned, CreatedAt: in.Now, ContentUpdatedAt: in.Now}
	g.Definitions[di].ContentUpdatedAt = in.Now
	g.Definitions[di].SupersededAt = &in.Now
	g.Definitions[di].SupersededByDefinitionID = &in.NewDefinitionID
	g.Definitions = append(g.Definitions, next)
	g.Chains[ci].NoteEntries = source.NoteEntries
	g.Chains[ci].UpdatedAt = in.Now
	status := core.TraceReturnedToPool
	if source.Date.Compare(in.Today) > 0 {
		status = core.TraceCancelledDraft
		g.Traces[ti].DraftCancellationID = &in.CancellationID
		g.Traces[ti].DraftCancelledOn = &in.Today
		for i, s := range g.Subtasks {
			if s.TraceID == source.ID && s.Status == core.SubtaskCancelledDraft {
				g.Subtasks[i].Status = core.SubtaskPending
				g.Subtasks[i].UpdatedAt = in.Now
				g.Subtasks[i].CompletedAt = nil
				g.Subtasks[i].SettledAt = nil
				g.Subtasks[i].DraftCancellationID = nil
			}
		}
	}
	g.Traces[ti].Status = status
	g.Traces[ti].PinOrder = nil
	g.Traces[ti].SettledAt = &in.Now
	g.Traces[ti].ContentUpdatedAt = in.Now
	if status == core.TraceReturnedToPool {
		if !historyRemainingCapture(prior, final, source.ID, status, in.EventID, in.Now) {
			return false
		}
	} else if in.EventID != "" || prior.proof.count != final.proof.count || !historyEqual(extra.Classifications, final.proof.snapshot.Object["classifications"]) {
		return false
	}
	extra.Classifications = final.proof.snapshot.Object["classifications"]
	expected, err := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	return err == nil && historyEqual(expected, final.proof.snapshot)
}

func historyDeferFence(prior, final validatedHistorySnapshot, in deferHistoryInput) bool {
	return historyRemainingCarryFence(prior, final, core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, SubtaskIDs: in.SubtaskIDs, TargetDate: in.TargetDate, Today: in.Today, Now: in.Now}, in.EventID, false)
}
func historyContinueFence(prior, final validatedHistorySnapshot, in continueHistoryInput) bool {
	return historyRemainingCarryFence(prior, final, core.TraceCarryoverInput{ChainID: in.ChainID, TraceID: in.TraceID, NewTraceID: in.NewTraceID, DayID: in.DayID, SubtaskIDs: in.SubtaskIDs, TargetDate: in.TargetDate, Today: in.Today, Now: in.Now}, "", true)
}

// continuation is fixed by the two private typed wrappers; never a caller
// exemption, validation bypass or adapter input.
func historyRemainingCarryFence(prior, final validatedHistorySnapshot, in core.TraceCarryoverInput, eventID string, continuation bool) bool {
	if !prior.valid() || !final.valid() {
		return false
	}
	state, extra, err := v7.ToDomain(prior.proof.snapshot)
	if err != nil {
		return false
	}
	g := state.Tasks
	ti, ci, di := historyRemainingIndices(g, in.TraceID, in.ChainID)
	if ti < 0 || ci < 0 || di < 0 {
		return false
	}
	source := g.Traces[ti]
	if source.ChainID != in.ChainID {
		return false
	}
	priority, hasPriority := 0, false
	for _, tr := range g.Traces {
		if tr.Date == in.TargetDate && tr.Status.FormsDayHistory() && (!hasPriority || tr.Priority > priority) {
			priority = tr.Priority
			hasPriority = true
		}
	}
	if priority == int(^uint(0)>>1) || (continuation && source.ContinuationSeq == int(^uint(0)>>1)) {
		return false
	}
	projection, err := core.NewProgressProjection(g)
	if err != nil {
		return false
	}
	percent := projection.TraceProgress(source.ID).Percent
	next := core.DayTrace{ID: in.NewTraceID, ChainID: in.ChainID, DefinitionID: g.Definitions[di].ID, Date: in.TargetDate, Status: core.TracePending, Priority: priority + 1, ContinuationSeq: source.ContinuationSeq, DescriptionText: source.DescriptionText, ManualProgressPercent: &percent, CarriedFromTraceID: &source.ID, CreatedAt: in.Now, ContentUpdatedAt: in.Now}
	for _, note := range source.NoteEntries {
		if note.DeletedAt == nil {
			next.NoteEntries = append(next.NoteEntries, note)
		}
	}
	if continuation {
		next.ContinuationSeq++
	} else {
		g.Traces[ti].Status = core.TraceDeferred
		g.Traces[ti].PinOrder = nil
		g.Traces[ti].SettledAt = &in.Now
		g.Traces[ti].ContentUpdatedAt = in.Now
	}
	g.Traces = append(g.Traces, next)
	open := historyRemainingOpen(g, source.ID)
	if len(open) != len(in.SubtaskIDs) {
		return false
	}
	for i, old := range open {
		id, ok := in.SubtaskIDs[old.ID]
		if !ok {
			return false
		}
		if !continuation {
			for j := range g.Subtasks {
				if g.Subtasks[j].ID == old.ID {
					g.Subtasks[j].Status = core.SubtaskDeferred
					g.Subtasks[j].UpdatedAt = in.Now
					g.Subtasks[j].SettledAt = &in.Now
				}
			}
		}
		oldID := old.ID
		g.Subtasks = append(g.Subtasks, core.Subtask{ID: id, LineageID: old.LineageID, TraceID: in.NewTraceID, Title: old.Title, Status: core.SubtaskPending, Difficulty: old.Difficulty, Position: i + 1, CarriedFromSubtaskID: &oldID, CreatedAt: in.Now, UpdatedAt: in.Now})
	}
	dayExists := false
	for _, day := range g.Days {
		if day.Date == in.TargetDate {
			dayExists = true
		}
	}
	if !dayExists {
		g.Days = append(g.Days, core.Day{ID: in.DayID, Date: in.TargetDate, CreatedAt: in.Now, UpdatedAt: in.Now})
	}
	if in.Now.After(g.Chains[ci].UpdatedAt) {
		g.Chains[ci].UpdatedAt = in.Now
	}
	if continuation {
		if prior.proof.count != final.proof.count || !historyEqual(extra.Classifications, final.proof.snapshot.Object["classifications"]) {
			return false
		}
	} else if !historyRemainingCapture(prior, final, source.ID, core.TraceDeferred, eventID, in.Now) {
		return false
	}
	extra.Classifications = final.proof.snapshot.Object["classifications"]
	expected, err := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	return err == nil && historyEqual(expected, final.proof.snapshot)
}

func historyRemainingIndices(g core.TaskGraph, trace core.DayTraceID, chain core.TaskChainID) (int, int, int) {
	ti, ci, di := -1, -1, -1
	for i, t := range g.Traces {
		if t.ID == trace {
			ti = i
		}
	}
	for i, c := range g.Chains {
		if c.ID == chain {
			ci = i
		}
	}
	for i, d := range g.Definitions {
		if d.ChainID == chain && d.SupersededAt == nil {
			di = i
		}
	}
	return ti, ci, di
}
func historyRemainingOpen(g core.TaskGraph, trace core.DayTraceID) []core.Subtask {
	var open []core.Subtask
	for _, s := range g.Subtasks {
		if s.TraceID == trace && (s.Status == core.SubtaskPending || s.Status == core.SubtaskUnfinished) {
			open = append(open, s)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Position < open[j].Position })
	return open
}

// Check the one permitted capture independently of the event builder. Both
// dictionaries preserve their own pair positions; old streams are exact
// prefixes, and category absence and empty labels are part of the new event.
func historyRemainingCapture(prior, final validatedHistorySnapshot, trace core.DayTraceID, status core.TraceStatus, eventID string, at core.Instant) bool {
	if eventID == "" || prior.proof.count == ^uint64(0) || final.proof.count != prior.proof.count+1 {
		return false
	}
	expected := historyVNextClone(prior.proof.snapshot.Object["classifications"])
	key := historyObject(map[string]canonical.Value{"rawValue": historyString(strings.ToUpper(string(trace)))})
	event := historyObject(map[string]canonical.Value{"id": historyString(strings.ToUpper(eventID)), "traceID": key, "status": historyString(string(status)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(at)), "revision": historyNumber(prior.proof.count + 1)})
	for _, name := range []string{"snapshotsByTraceID", "snapshotEventsByTraceID"} {
		pairs := expected.Object[name]
		found := false
		for i := 0; i+1 < len(pairs.Array); i += 2 {
			if historyID(pairs.Array[i]) == strings.ToUpper(string(trace)) {
				if name == "snapshotsByTraceID" {
					pairs.Array[i+1] = event
				} else {
					stream := pairs.Array[i+1]
					stream.Array = append(stream.Array, event)
					pairs.Array[i+1] = stream
				}
				found = true
				break
			}
		}
		if !found {
			value := event
			if name == "snapshotEventsByTraceID" {
				value = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{event}}
			}
			pairs.Array = append(pairs.Array, key, value)
		}
		expected.Object[name] = pairs
	}
	expected.Object["revision"] = historyNumber(prior.proof.count + 1)
	return historyEqual(expected, final.proof.snapshot.Object["classifications"])
}
