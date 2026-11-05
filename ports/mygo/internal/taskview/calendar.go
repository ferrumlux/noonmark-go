// SPDX-License-Identifier: AGPL-3.0-only
// Package taskview contains read-only projections without persistence authority.
package taskview

import (
	"fmt"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type CalendarPosition string

const (
	CalendarHistory CalendarPosition = "history"
	CalendarToday   CalendarPosition = "today"
	CalendarFuture  CalendarPosition = "future"
)

type CalendarRow struct {
	TraceID      core.DayTraceID
	ChainID      core.TaskChainID
	DefinitionID core.TaskDefinitionID
	Title        string
	Status       core.TraceStatus
}
type CalendarDay struct {
	Date                                                          core.LocalDate
	IsToday, IsSelected                                           bool
	Position                                                      CalendarPosition
	Rows, PreviewRows                                             []CalendarRow
	HiddenCount, Total, Completed, Pending, Unfinished, HeatLevel int
	HasHeat                                                       bool
}
type CalendarSlot struct {
	Index, Row, Column               int
	Date                             *core.LocalDate
	Day                              *CalendarDay
	BottomBoundary, TrailingBoundary bool
}
type CalendarMonth struct {
	Month, Selected, Today core.LocalDate
	RowCount               int
	Slots                  []CalendarSlot
	SelectedDay            CalendarDay
	CanPrevious, CanNext   bool
}

// ShiftCalendarMonth always selects day one, even for a zero offset.
func ShiftCalendarMonth(selected core.LocalDate, delta int) (core.LocalDate, error) {
	if !selected.Valid() {
		return core.LocalDate{}, fmt.Errorf("invalid calendar selection")
	}
	index := (selected.Year-1)*12 + selected.Month - 1
	// Check before addition to avoid overflowing int on extreme caller input.
	if delta < -index || delta > 9999*12-1-index {
		return core.LocalDate{}, fmt.Errorf("calendar month outside supported range")
	}
	index += delta
	return core.LocalDate{Year: index/12 + 1, Month: index%12 + 1, Day: 1}, nil
}
func MoveCalendarSelection(selected core.LocalDate, days int) (core.LocalDate, error) {
	return selected.AddDays(days)
}

// ProjectCalendar never settles, creates days, changes the graph or consults the
// clock. Its domain-level filtering does not expand Service capabilities.
func ProjectCalendar(graph core.TaskGraph, selected, today core.LocalDate) (CalendarMonth, error) {
	if !selected.Valid() || !today.Valid() {
		return CalendarMonth{}, fmt.Errorf("invalid calendar date")
	}
	if err := core.ValidateTaskGraph(graph); err != nil {
		return CalendarMonth{}, fmt.Errorf("calendar graph: %w", err)
	}
	recurring := map[core.TaskChainID]bool{}
	readGraph := graph.Clone()
	for i := range readGraph.Chains {
		if readGraph.Chains[i].CycleMembership != nil {
			recurring[readGraph.Chains[i].ID] = true
			// The kernel's recurring guard protects commands. This private, validated
			// clone is only used for DayTodo reads; original membership controls exclusion.
			readGraph.Chains[i].CycleMembership = nil
		}
	}
	kernel, err := core.NewTaskKernel(readGraph)
	if err != nil {
		return CalendarMonth{}, fmt.Errorf("calendar read kernel: %w", err)
	}
	titles := map[core.TaskDefinitionID]string{}
	for _, definition := range graph.Definitions {
		titles[definition.ID] = definition.Title
	}
	month := core.LocalDate{Year: selected.Year, Month: selected.Month, Day: 1}
	lead := (int(time.Date(month.Year, time.Month(month.Month), 1, 12, 0, 0, 0, time.UTC).Weekday()) + 6) % 7
	days := 31
	for !(core.LocalDate{Year: month.Year, Month: month.Month, Day: days}).Valid() {
		days--
	}
	rows := max(5, (lead+days+6)/7)
	result := CalendarMonth{Month: month, Selected: selected, Today: today, RowCount: rows, Slots: make([]CalendarSlot, rows*7)}
	_, err = ShiftCalendarMonth(selected, -1)
	result.CanPrevious = err == nil
	_, err = ShiftCalendarMonth(selected, 1)
	result.CanNext = err == nil
	for i := range result.Slots {
		slot := CalendarSlot{Index: i, Row: i / 7, Column: i % 7, BottomBoundary: i/7 < rows-1, TrailingBoundary: i%7 < 6}
		n := i - lead + 1
		if n >= 1 && n <= days {
			date := core.LocalDate{Year: month.Year, Month: month.Month, Day: n}
			day := CalendarDay{Date: date, IsToday: date == today, IsSelected: date == selected, Position: CalendarToday}
			if date.Compare(today) < 0 {
				day.Position = CalendarHistory
			} else if date.Compare(today) > 0 {
				day.Position = CalendarFuture
			}
			for _, trace := range kernel.DayTodo(date) {
				if recurring[trace.ChainID] {
					continue
				}
				day.Rows = append(day.Rows, CalendarRow{TraceID: trace.ID, ChainID: trace.ChainID, DefinitionID: trace.DefinitionID, Title: titles[trace.DefinitionID], Status: trace.Status})
				switch trace.Status {
				case core.TraceCompleted:
					day.Completed++
				case core.TracePending:
					day.Pending++
				case core.TraceUnfinished:
					day.Unfinished++
				}
			}
			day.Total = len(day.Rows)
			day.HeatLevel = min(day.Completed, 4)
			day.HasHeat = day.Total > 0
			day.PreviewRows = append([]CalendarRow(nil), day.Rows[:min(day.Total, 6)]...)
			day.HiddenCount = max(0, day.Total-6)
			slot.Date = &date
			slot.Day = &day
			if day.IsSelected {
				result.SelectedDay = day
				result.SelectedDay.Rows = append([]CalendarRow(nil), day.Rows...)
				result.SelectedDay.PreviewRows = append([]CalendarRow(nil), day.PreviewRows...)
			}
		}
		result.Slots[i] = slot
	}
	return result, nil
}
