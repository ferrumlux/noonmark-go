// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"sort"
)

// deriveOrdinaryCaptures executes the closed task edge itself. It cannot be
// supplied an after graph or arbitrary event/status/time. IDs are consumed only
// after the command-derived capture set has been determined. This is pure
// derivation, not admission: the complete candidate still requires validation.
func deriveOrdinaryCaptures(before core.TaskGraph, state ordinary.AggregateState, in ordinaryTransition, ids []ordinary.UUID) (core.TaskGraph, []ordinary.TraceClassificationSnapshot, error) {
	after, err := in.replay(before)
	if err != nil {
		return core.TaskGraph{}, nil, err
	}
	oldDays := map[core.LocalDate]core.Day{}
	newDays := map[core.LocalDate]core.Day{}
	for _, d := range before.Days {
		oldDays[d.Date] = d
	}
	for _, d := range after.Days {
		newDays[d.Date] = d
	}
	var capture []core.DayTrace
	for _, tr := range after.Traces {
		switch in.kind {
		case ordinarySettle:
			if newDays[tr.Date].LockedAt != nil && oldDays[tr.Date].LockedAt == nil && (tr.Status == core.TraceCompleted || tr.Status == core.TraceUnfinished) {
				capture = append(capture, tr)
			}
		case ordinaryReturn:
			if tr.ID == in.returning.TraceID && tr.Status == core.TraceReturnedToPool {
				capture = append(capture, tr)
			}
		case ordinaryDefer:
			if tr.ID == in.carry.TraceID && tr.Status == core.TraceDeferred {
				capture = append(capture, tr)
			}
		}
	}
	// Deterministic local allocation order, not a claim about Swift Set order.
	sort.Slice(capture, func(i, j int) bool {
		if n := capture[i].Date.Compare(capture[j].Date); n != 0 {
			return n < 0
		}
		return capture[i].ID < capture[j].ID
	})
	if len(ids) != len(capture) || uint64(len(capture)) > math.MaxUint64-state.Revision {
		return core.TaskGraph{}, nil, ordinary.ErrInvalid
	}
	events := make([]ordinary.TraceClassificationSnapshot, 0, len(capture))
	seen := map[ordinary.UUID]bool{}
	for _, stream := range state.SnapshotEventsByTraceID {
		for _, event := range stream {
			seen[event.ID] = true
		}
	}
	for i, tr := range capture {
		if !historyVNextUUID(string(ids[i])) || seen[ids[i]] || len(state.SnapshotEventsByTraceID[tr.ID]) != 0 {
			return core.TaskGraph{}, nil, ordinary.ErrConflict
		}
		seen[ids[i]] = true
		at := newDays[tr.Date].LockedAt
		if tr.Status == core.TraceReturnedToPool || tr.Status == core.TraceDeferred {
			at = tr.SettledAt
		}
		if at == nil {
			return core.TaskGraph{}, nil, ordinary.ErrInvalid
		}
		event := ordinary.TraceClassificationSnapshot{ID: ids[i], TraceID: tr.ID, Status: tr.Status, CapturedAt: *at, Revision: state.Revision + uint64(i) + 1, Labels: []ordinary.HistoricalLabelValue{}}
		if relation := state.CurrentByChainID[tr.ChainID].Category; relation != nil {
			category, exists := state.Categories[relation.CategoryID]
			if !exists {
				return core.TaskGraph{}, nil, ordinary.ErrInvalid
			}
			event.Category = &ordinary.HistoricalCategoryValue{ID: category.ID, Name: category.Name, ColorHex: category.ColorHex, PresentationApproval: category.PresentationApproval}
		}
		events = append(events, event)
	}
	return after, events, nil
}
