// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"math"
	"sort"
)

// SubtaskProgress counts presentable children of one trace, not lineages in a
// chain. These counts are NOT the weighted percentage. Source: Models 1111-1136.
type SubtaskProgress struct {
	Total, Completed, Pending, Unfinished, Deferred, Abandoned int
}

func (p SubtaskProgress) IsPartiallyCompleted() bool {
	return p.Completed > 0 && p.Completed < p.Total-p.Abandoned
}
func (p SubtaskProgress) CanCompleteParent() bool { return p.Pending == 0 && p.Unfinished == 0 }

type TraceProgressMode string

const (
	ProgressManual           TraceProgressMode = "manual"
	ProgressWeightedSubtasks TraceProgressMode = "weightedSubtasks"
)

// TraceProgress.Percent includes the historical floor; CompletedWeight and
// TotalWeight describe raw progress and need not yield Percent when divided.
// Completed parents use manual mode, 100 percent and weights 1/1, as upstream.
type TraceProgress struct {
	Mode                                                TraceProgressMode
	Percent, FloorPercent, CompletedWeight, TotalWeight int
}

type DayTraceCompletionAction string
type DayTraceCompletionBlocker string

const (
	CompletionComplete DayTraceCompletionAction = "complete"
	CompletionUndo     DayTraceCompletionAction = "undo"

	CompletionLockedDay         DayTraceCompletionBlocker = "lockedDay"
	CompletionHistoricalDay     DayTraceCompletionBlocker = "historicalDay"
	CompletionFutureDay         DayTraceCompletionBlocker = "futureDay"
	CompletionUnsupportedStatus DayTraceCompletionBlocker = "unsupportedStatus"
	CompletionOpenSubtasks      DayTraceCompletionBlocker = "openSubtasks"
)

// Exactly one of Action and Blocker is set on a successful capability query.
// Status is populated only for the unsupportedStatus blocker. A capability is
// a snapshot observation, not permission to bypass command-time validation.
type DayTraceCompletionCapability struct {
	Action  DayTraceCompletionAction
	Blocker DayTraceCompletionBlocker
	Status  TraceStatus
}

// ProgressProjection is an immutable, read-only view of a validated bounded
// task graph. It has no persistence, UI, service or command authority. Construct
// a fresh projection after a graph changes. Its zero value is an empty view.
// Concurrent reads are safe; the caller must not mutate input during creation.
type ProgressProjection struct {
	traces             map[DayTraceID]DayTrace
	days               map[LocalDate]Day
	traceProgress      map[DayTraceID]TraceProgress
	subtaskProgress    map[DayTraceID]SubtaskProgress
	chainsWithSubtasks map[TaskChainID]bool
}

// NewProgressProjection rejects malformed graphs through the existing
// validator, including duplicate identities, broken links and invalid weights.
// It does not validate an omitted full snapshot, supplement or recurring parent.
func NewProgressProjection(graph TaskGraph) (*ProgressProjection, error) {
	if err := ValidateTaskGraph(graph); err != nil {
		return nil, err
	}
	graph = graph.Clone()
	p := &ProgressProjection{
		traces:             make(map[DayTraceID]DayTrace, len(graph.Traces)),
		days:               make(map[LocalDate]Day, len(graph.Days)),
		traceProgress:      make(map[DayTraceID]TraceProgress, len(graph.Traces)),
		subtaskProgress:    make(map[DayTraceID]SubtaskProgress),
		chainsWithSubtasks: make(map[TaskChainID]bool),
	}
	chainTraces := make(map[TaskChainID][]DayTrace)
	subtasks := make(map[DayTraceID][]Subtask)
	for _, day := range graph.Days {
		p.days[day.Date] = day
	}
	for _, trace := range graph.Traces {
		p.traces[trace.ID] = trace
		chainTraces[trace.ChainID] = append(chainTraces[trace.ChainID], trace)
	}
	for _, subtask := range graph.Subtasks {
		if subtask.Status == SubtaskCancelledDraft {
			continue
		}
		subtasks[subtask.TraceID] = append(subtasks[subtask.TraceID], subtask)
		counts := p.subtaskProgress[subtask.TraceID]
		counts.Total++
		switch subtask.Status {
		case SubtaskCompleted:
			counts.Completed++
		case SubtaskPending:
			counts.Pending++
		case SubtaskUnfinished:
			counts.Unfinished++
		case SubtaskDeferred:
			counts.Deferred++
		case SubtaskAbandoned:
			counts.Abandoned++
		}
		p.subtaskProgress[subtask.TraceID] = counts
		trace := p.traces[subtask.TraceID]
		if trace.Status.FormsDayHistory() {
			p.chainsWithSubtasks[trace.ChainID] = true
		}
	}
	for _, traces := range chainTraces {
		p.projectChainProgress(traces, subtasks)
	}
	return p, nil
}

// SubtaskProgress follows Engine 2866-2878. Like upstream, it does not filter
// the parent status: an explicitly queried hidden trace can have pending child
// counts. Callers must independently filter user-facing trace rows.
func (p *ProgressProjection) SubtaskProgress(traceID DayTraceID) SubtaskProgress {
	if p == nil {
		return SubtaskProgress{}
	}
	return p.subtaskProgress[traceID]
}

// TraceProgress follows Engine 2880-2899 and 3950-4020. The chain is selected
// from the trace, not from its definition. Missing IDs return manual 0 with 0/0
// weights, whereas a known trace without child history has manual 0 with 0/100.
func (p *ProgressProjection) TraceProgress(traceID DayTraceID) TraceProgress {
	if p == nil {
		return TraceProgress{Mode: ProgressManual}
	}
	result, ok := p.traceProgress[traceID]
	if !ok {
		return TraceProgress{Mode: ProgressManual}
	}
	return result
}

// The source repeatedly scans historical raw values. This equivalent sweep
// precomputes every trace in O(T log T + S) work, O(T + S) additional space;
// individual queries are O(1). Constructor validation has its existing cost.
// Same-day records must all enter raw weights BEFORE any same-day trace is
// evaluated. Floors advance only after a complete date/continuationSeq group.
func (p *ProgressProjection) projectChainProgress(traces []DayTrace, subtasks map[DayTraceID][]Subtask) {
	sort.Slice(traces, func(i, j int) bool {
		if comparison := traces[i].Date.Compare(traces[j].Date); comparison != 0 {
			return comparison < 0
		}
		return traces[i].ContinuationSeq < traces[j].ContinuationSeq
	})
	type lineageProgress struct {
		weight    int
		completed bool
	}
	lineages := make(map[SubtaskLineageID]lineageProgress)
	totalWeight, completedWeight, floor := 0, 0, 0
	for start := 0; start < len(traces); {
		end := start + 1
		for end < len(traces) && traces[end].Date == traces[start].Date {
			end++
		}
		for _, trace := range traces[start:end] {
			if !trace.Status.FormsDayHistory() {
				continue
			}
			for _, subtask := range subtasks[trace.ID] {
				old := lineages[subtask.LineageID]
				next := lineageProgress{weight: max(old.weight, int(subtask.Difficulty)), completed: old.completed || subtask.Status == SubtaskCompleted}
				totalWeight += next.weight - old.weight
				if old.completed {
					completedWeight -= old.weight
				}
				if next.completed {
					completedWeight += next.weight
				}
				lineages[subtask.LineageID] = next
			}
		}
		// Max difficulty and any-completed are order independent. No current
		// definition, carried-from link, creation clock or latest-child shortcut.
		weightedPercent := 0
		if totalWeight != 0 {
			// Swift Double arithmetic and nearest-away-from-zero, not truncation or
			// round-to-even. Avoid integer multiplication overflow as well.
			weightedPercent = int(math.Round(float64(completedWeight) / float64(totalWeight) * 100))
		}
		for groupStart := start; groupStart < end; {
			groupEnd := groupStart + 1
			for groupEnd < end && traces[groupEnd].ContinuationSeq == traces[groupStart].ContinuationSeq {
				groupEnd++
			}
			nextFloor := floor
			for _, trace := range traces[groupStart:groupEnd] {
				raw := TraceProgress{Mode: ProgressManual, TotalWeight: 100}
				switch {
				case trace.Status == TraceCompleted:
					raw = TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 1, TotalWeight: 1}
				case totalWeight != 0:
					raw = TraceProgress{Mode: ProgressWeightedSubtasks, Percent: weightedPercent, CompletedWeight: completedWeight, TotalWeight: totalWeight}
				case trace.ManualProgressPercent != nil:
					raw.Percent = *trace.ManualProgressPercent
					raw.CompletedWeight = raw.Percent
				}
				if trace.Status.FormsDayHistory() {
					nextFloor = max(nextFloor, raw.Percent)
				}
				raw.FloorPercent = floor
				raw.Percent = max(raw.Percent, floor)
				p.traceProgress[trace.ID] = raw
			}
			floor = nextFloor
			groupStart = groupEnd
		}
		start = end
	}
}

// CanSetManualProgress projects the guards of Engine 2840-2864; command-time
// validation remains in SetManualProgress. Manual display mode alone is insufficient:
// any presentable child on ANY history-forming trace of the chain, including a
// future trace, disallows editing. Planned definition children do not count.
func (p *ProgressProjection) CanSetManualProgress(traceID DayTraceID, today LocalDate) (bool, error) {
	trace, err := p.progressQueryTrace(traceID, today)
	if err != nil {
		return false, err
	}
	return p.days[trace.Date].LockedAt == nil && trace.Date == today && trace.Status == TracePending && !p.chainsWithSubtasks[trace.ChainID], nil
}

// CompletionCapability follows Engine 1979-1987 and 3684-3708, including
// locked-day precedence and trace-local (rather than chain-wide) open children.
func (p *ProgressProjection) CompletionCapability(traceID DayTraceID, today LocalDate) (DayTraceCompletionCapability, error) {
	trace, err := p.progressQueryTrace(traceID, today)
	if err != nil {
		return DayTraceCompletionCapability{}, err
	}
	if p.days[trace.Date].LockedAt != nil {
		return DayTraceCompletionCapability{Blocker: CompletionLockedDay}, nil
	}
	if comparison := trace.Date.Compare(today); comparison < 0 {
		return DayTraceCompletionCapability{Blocker: CompletionHistoricalDay}, nil
	} else if comparison > 0 {
		return DayTraceCompletionCapability{Blocker: CompletionFutureDay}, nil
	}
	switch trace.Status {
	case TracePending:
		if !p.SubtaskProgress(traceID).CanCompleteParent() {
			return DayTraceCompletionCapability{Blocker: CompletionOpenSubtasks}, nil
		}
		return DayTraceCompletionCapability{Action: CompletionComplete}, nil
	case TraceCompleted:
		return DayTraceCompletionCapability{Action: CompletionUndo}, nil
	default:
		return DayTraceCompletionCapability{Blocker: CompletionUnsupportedStatus, Status: trace.Status}, nil
	}
}

func (p *ProgressProjection) progressQueryTrace(traceID DayTraceID, today LocalDate) (DayTrace, error) {
	// Go permits construction of invalid LocalDate values; Swift's typed
	// source does not. Keep that port boundary fail-closed without panics.
	if !today.Valid() {
		return DayTrace{}, ErrInvalidInput
	}
	if p != nil {
		if trace, ok := p.traces[traceID]; ok {
			return trace, nil
		}
	}
	return DayTrace{}, ErrNotFound
}
