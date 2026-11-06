// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// SetManualProgress never completes a task. The explicit chain/trace route is
// checked by the domain command, in the same lock as persistence and publication.
func (s *Service) SetManualProgress(chain core.TaskChainID, trace core.DayTraceID, percent int) error {
	_, err := s.SetManualProgressAcknowledged(chain, trace, percent)
	return err
}

// SetManualProgressAcknowledged returns only the exact normalized percentage
// confirmed by this transaction, including clamping and the historical floor.
// On any failure the returned zero is not an acknowledgment.
func (s *Service) SetManualProgressAcknowledged(chain core.TaskChainID, trace core.DayTraceID, percent int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, ErrSession
	}
	if s.disk.blocked {
		return 0, ErrIncomplete
	}
	k, err := core.NewTaskKernel(s.graph)
	if err != nil {
		return 0, err
	}
	command := core.SetManualProgressInput{ChainID: chain, TraceID: trace, Percent: percent, Today: s.today}
	var previous *int
	for _, tr := range s.graph.Traces {
		if tr.ID == trace {
			command.Now = tr.ContentUpdatedAt
			previous = tr.ManualProgressPercent
			break
		}
	}
	// Determine no-op with an existing valid trace clock. Do not call the wall
	// clock or advance an exhausted frontier for a command that changes nothing.
	// Guard errors still come from the source-aligned command, not a UI query.
	if err = k.SetManualProgress(command); err != nil {
		return 0, err
	}
	candidate := k.Snapshot()
	changed := false
	for _, tr := range candidate.Traces {
		if tr.ID == trace {
			changed = previous == nil || tr.ManualProgressPercent == nil || *previous != *tr.ManualProgressPercent
			break
		}
	}
	if changed {
		command.Now, err = s.nextInstant()
		if err != nil {
			return 0, err
		}
		k, err = core.NewTaskKernel(s.graph)
		if err != nil {
			return 0, err
		}
		if err = k.SetManualProgress(command); err != nil {
			return 0, err
		}
		candidate = k.Snapshot()
	}
	captured, err := captureManualProgressHistory(s.snapshot, s.graph, candidate, command)
	if err != nil {
		return 0, err
	}
	wire, graph, err := normalizeSnapshot(captured)
	if err != nil {
		return 0, err
	}
	accepted, found := 0, false
	for _, tr := range graph.Traces {
		if tr.ID == trace && tr.ChainID == chain && tr.ManualProgressPercent != nil {
			accepted, found = *tr.ManualProgressPercent, true
			break
		}
	}
	if !found {
		return 0, core.ErrNotFound
	}
	if historyEqual(s.snapshot, wire) {
		if _, err = s.disk.verifyPrefix(s.disk.revision, s.disk.hash, false); err != nil {
			return 0, err
		}
		return accepted, nil
	}
	if err = s.disk.commit(wire); err != nil {
		return 0, err
	}
	s.graph, s.snapshot = graph, wire
	return accepted, nil
}

// Replay exactly one routed manual-progress command. No caller-selected field
// exemptions, inferred audit reconstruction, or broader runtime scope.
func captureManualProgressHistory(prior canonical.Value, before, after core.TaskGraph, command core.SetManualProgressInput) (canonical.Value, error) {
	fail := func(err error) (canonical.Value, error) { return canonical.Value{}, err }
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
		return fail(historyError("prior does not match manual progress before graph"))
	}
	k, err := core.NewTaskKernel(before)
	if err != nil {
		return fail(err)
	}
	if err = k.SetManualProgress(command); err != nil {
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
		return fail(historyError("candidate differs from manual progress command replay"))
	}
	for key, value := range prior.Object {
		if key != "traces" && !historyEqual(value, result.Object[key]) {
			return fail(historyError("manual progress changed frozen " + key))
		}
	}
	traces := make(map[string]canonical.Value, len(result.Object["traces"].Array))
	for _, tr := range result.Object["traces"].Array {
		traces[historyID(tr.Object["id"])] = tr
	}
	for _, tr := range prior.Object["traces"].Array {
		id := historyID(tr.Object["id"])
		actual, ok := traces[id]
		if !ok {
			return fail(historyError("manual progress removed trace"))
		}
		if id != string(command.TraceID) {
			if !historyEqual(tr, actual) {
				return fail(historyError("manual progress changed other trace"))
			}
			continue
		}
		// Full replay above fixes both permitted values. This independent fence
		// also makes every other field, including identity/status, immutable.
		projected := historyObject(map[string]canonical.Value{})
		for key, value := range tr.Object {
			projected.Object[key] = value
		}
		projected.Object["manualProgressPercent"] = actual.Object["manualProgressPercent"]
		projected.Object["contentUpdatedAt"] = actual.Object["contentUpdatedAt"]
		if !historyEqual(projected, actual) {
			return fail(historyError("manual progress changed other trace fields"))
		}
	}
	if err = ValidateEmptyHistory(result); err != nil {
		return fail(err)
	}
	return result, nil
}
