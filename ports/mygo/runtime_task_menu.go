// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
)

const returnToPoolLabel = "回到任务池（留下轨迹）"

// The source uses a system context menu on the task row, not a toolbar or
// sheet. Keep native menu chrome and expose only the connected action subset.
func (s *app) liveTaskContextMenu(m *ui.Menu, row taskview.DayRow) {
	complete, returnable := taskMenuCapabilities(s.runtime.Snapshot(), row, s.runtime.Today())
	if complete {
		label := "标记完成"
		if row.Status == core.TraceCompleted {
			label = "撤销完成"
		}
		if m.Item(label).Chosen() {
			s.titleTransition(func() {
				s.runAction(fmt.Sprintf("complete:%s:%t", row.TraceID, row.Status != core.TraceCompleted), func() error { return s.runtime.SetCompleted(row.TraceID, row.Status != core.TraceCompleted) })
			})
		}
	}
	if s.titles != nil {
		if _, ok := s.titles.store.(carryoverStore); ok {
			d, c := carryoverMenuCapabilities(s.titles.graph, row, s.runtime.Today())
			if d && m.Item(deferTaskLabel).Chosen() {
				s.openCarryover(carryoverDefer, row)
			}
			if c && m.Item(continueTaskLabel).Chosen() {
				s.openCarryover(carryoverContinue, row)
			}
		}
	}
	if returnable && m.Item(returnToPoolLabel).Chosen() {
		s.returnTaskToPool(row.ChainID, row.TraceID)
	}
}

func (s *app) returnTaskToPool(chain core.TaskChainID, trace core.DayTraceID) {
	// No synchronous fallback: this menu is available only with the shared
	// writer/session boundary. Navigation finalizes native text and confirms its
	// persistence before the immutable action is admitted to the writer FIFO.
	if s.titles == nil {
		return
	}
	page, date, selection := s.page, s.runtimeDate, s.runtimeSelection
	s.titleTransition(func() {
		h := s.titles
		w := h.windows[s.titleWindowID]
		if w == nil {
			return
		}
		navigation := w.navigation
		h.commandOnce(s, "return:"+string(chain)+":"+string(trace), func() error { return s.runtime.ReturnToPool(chain, trace) }, func(err error) {
			if err == nil && h.windows[s.titleWindowID] == w && w.navigation == navigation && s.page == page && s.runtimeDate == date && s.runtimeSelection == selection && selection == string(trace) && s.automaticOwnerExitAllowed() {
				s.runtimeSelection = ""
			}
		})
	})
}

// Project only source-supported actions from the confirmed read view. A pending
// parent with open subtasks may return to the pool but cannot be completed.
func taskMenuCapabilities(g core.TaskGraph, row taskview.DayRow, today core.LocalDate) (complete, returnable bool) {
	var trace *core.DayTrace
	for i := range g.Traces {
		if g.Traces[i].ID == row.TraceID && g.Traces[i].ChainID == row.ChainID && g.Traces[i].Status == row.Status {
			trace = &g.Traces[i]
			break
		}
	}
	if trace == nil || trace.Date.Compare(today) < 0 {
		return
	}
	for _, day := range g.Days {
		if day.Date == trace.Date && day.LockedAt != nil {
			return
		}
	}
	ordinary := false
	for _, chain := range g.Chains {
		if chain.ID == trace.ChainID && chain.CycleMembership == nil {
			ordinary = true
			break
		}
	}
	if !ordinary {
		return
	}
	returnable = trace.Status == core.TracePending
	complete = trace.Date == today && (trace.Status == core.TracePending || trace.Status == core.TraceCompleted)
	if complete && trace.Status == core.TracePending {
		for _, subtask := range g.Subtasks {
			if subtask.TraceID == trace.ID && (subtask.Status == core.SubtaskPending || subtask.Status == core.SubtaskUnfinished) {
				complete = false
				break
			}
		}
	}
	return
}
