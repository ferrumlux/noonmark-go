// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// DeferCurrentTrace returns a new identity only after the aggregate commit is confirmed.
func (s *Service) DeferCurrentTrace(chain core.TaskChainID, trace core.DayTraceID, target core.LocalDate) (core.DayTraceID, error) {
	return s.carryoverTrace(chain, trace, target, false)
}

// ContinueUnfinishedTrace preserves the source and its complete classification history.
func (s *Service) ContinueUnfinishedTrace(chain core.TaskChainID, trace core.DayTraceID, target core.LocalDate) (core.DayTraceID, error) {
	return s.carryoverTrace(chain, trace, target, true)
}

func replayCarryover(k *core.TaskKernel, in core.TraceCarryoverInput, continuation bool) error {
	if continuation {
		return k.ContinueUnfinishedTrace(in)
	}
	return k.DeferCurrentTrace(in)
}

func (s *Service) carryoverTrace(chain core.TaskChainID, trace core.DayTraceID, target core.LocalDate, continuation bool) (core.DayTraceID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.gateLocked(); err != nil {
		return "", err
	}
	k, err := core.NewTaskKernel(s.graph)
	if err != nil {
		return "", err
	}
	now, err := s.nextInstant()
	if err != nil {
		return "", err
	}
	ids, err := newIDs(2)
	if err != nil {
		return "", err
	}
	in := core.TraceCarryoverInput{ChainID: chain, TraceID: trace, NewTraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), TargetDate: target, Today: s.today, Now: now, SubtaskIDs: map[core.SubtaskID]core.SubtaskID{}}
	for _, sub := range s.graph.Subtasks {
		if sub.TraceID == trace && (sub.Status == core.SubtaskPending || sub.Status == core.SubtaskUnfinished) {
			child, err := newIDs(1)
			if err != nil {
				return "", err
			}
			in.SubtaskIDs[sub.ID] = core.SubtaskID(child[0])
		}
	}
	if err = replayCarryover(k, in, continuation); err != nil {
		return "", err
	}
	captured, err := captureCarryoverHistory(s.snapshot, s.graph, k.Snapshot(), in, continuation)
	if err != nil {
		return "", err
	}
	wire, graph, err := normalizeSnapshot(captured)
	if err != nil {
		return "", err
	}
	if err = s.commitLocked(wire); err != nil {
		return "", err
	}
	s.snapshot, s.graph = wire, graph
	return in.NewTraceID, nil
}

// The only exemption is exact whole-aggregate replay of the complete core input.
func captureCarryoverHistory(prior canonical.Value, before, after core.TaskGraph, in core.TraceCarryoverInput, continuation bool) (canonical.Value, error) {
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
	if !historyEqual(old, prior) {
		return fail(historyError("prior does not match before graph"))
	}
	k, err := core.NewTaskKernel(before)
	if err != nil {
		return fail(err)
	}
	if err = replayCarryover(k, in, continuation); err != nil {
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
		return fail(historyError("candidate differs from carryover command replay"))
	}
	for _, t := range after.Traces {
		if t.ID == in.TraceID && !continuation && t.Status == core.TraceDeferred {
			id, err := core.NewIdentity()
			if err != nil {
				return fail(err)
			}
			c := result.Object["classifications"]
			rev := historyUint(c.Object["revision"]) + 1
			key := historyObject(map[string]canonical.Value{"rawValue": historyString(string(t.ID))})
			event := historyObject(map[string]canonical.Value{"id": historyString(id), "traceID": key, "status": historyString(string(t.Status)), "labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(in.Now)), "revision": historyNumber(rev)})
			latest := c.Object["snapshotsByTraceID"]
			latest.Array = append(latest.Array, key, event)
			c.Object["snapshotsByTraceID"] = latest
			streams := c.Object["snapshotEventsByTraceID"]
			streams.Array = append(streams.Array, key, canonical.Value{Kind: canonical.Array, Array: []canonical.Value{event}})
			c.Object["snapshotEventsByTraceID"] = streams
			c.Object["revision"] = historyNumber(rev)
			result.Object["classifications"] = c
		}
	}
	if err = ValidateEmptyHistory(result); err != nil {
		return fail(err)
	}
	return result, nil
}
