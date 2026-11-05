// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type DateStripCell struct {
	Date                *core.LocalDate
	IsToday, IsSelected bool
	Pending             int
}
type DateStrip struct {
	Cells         [14]DateStripCell
	SelectedIndex int
}
type DayRow struct {
	CalendarRow
	Pinned   bool
	PinOrder *int
}
type DayStats struct {
	Total, Completed, Pending, Unfinished, Deferred, Abandoned int
	CompletionRate                                             int
}
type DayTrendPoint struct {
	Date       *core.LocalDate
	Ratio      *float64
	IsSelected bool
}
type DayReview struct {
	Trend         [7]DayTrendPoint
	TomorrowTotal int
}
type DayPage struct {
	Selected, Today     core.LocalDate
	Strip               DateStrip
	Rows                []DayRow
	Stats               DayStats
	Review              *DayReview
	IsHistory, IsFuture bool
}

// civilOrdinal is exact throughout years 1..9999 and does not use Duration.
func civilOrdinal(d core.LocalDate) int {
	y := d.Year - 1
	days := 365*y + y/4 - y/100 + y/400
	starts := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	days += starts[d.Month-1] + d.Day
	if d.Month > 2 && (d.Year%400 == 0 || (d.Year%4 == 0 && d.Year%100 != 0)) {
		days++
	}
	return days
}
func dateAtOrdinal(n int) *core.LocalDate {
	if n < 1 || n > 3652059 {
		return nil
	}
	// AddDate operates in civil days, without a nanosecond duration difference.
	t := time.Date(1, 1, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, n-1)
	d := core.LocalDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
	return &d
}

// ProjectDateStrip retains invalid edge slots as nil, never clamping dates.
func ProjectDateStrip(today, selected core.LocalDate) (DateStrip, error) {
	if !today.Valid() || !selected.Valid() {
		return DateStrip{}, fmt.Errorf("invalid day date")
	}
	anchor := civilOrdinal(today) - 6
	offset := civilOrdinal(selected) - anchor
	block := offset / 14
	if offset%14 < 0 {
		block--
	}
	start := anchor + block*14
	out := DateStrip{SelectedIndex: civilOrdinal(selected) - start}
	for i := range out.Cells {
		d := dateAtOrdinal(start + i)
		out.Cells[i].Date = d
		if d != nil {
			out.Cells[i].IsToday = *d == today
			out.Cells[i].IsSelected = *d == selected
		}
	}
	return out, nil
}

func dayStats(traces []core.DayTrace) DayStats {
	s := DayStats{Total: len(traces)}
	for _, t := range traces {
		switch t.Status {
		case core.TracePending:
			s.Pending++
		case core.TraceCompleted:
			s.Completed++
		case core.TraceUnfinished:
			s.Unfinished++
		case core.TraceDeferred:
			s.Deferred++
		case core.TraceAbandoned:
			s.Abandoned++
		}
	}
	if s.Total > 0 {
		s.CompletionRate = int(math.Round(float64(s.Completed) / float64(s.Total) * 100))
	}
	return s
}

// ProjectDay is a read-only default flat/time/ascending presentation. It neither
// consults the clock nor settles days or broadens the command service's scope.
func ProjectDay(graph core.TaskGraph, selected, today core.LocalDate) (DayPage, error) {
	strip, err := ProjectDateStrip(today, selected)
	if err != nil {
		return DayPage{}, err
	}
	if err = core.ValidateTaskGraph(graph); err != nil {
		return DayPage{}, fmt.Errorf("day graph: %w", err)
	}
	readGraph := graph.Clone()
	for i := range readGraph.Chains {
		if readGraph.Chains[i].CycleMembership != nil {
			// Only this validated private read clone bypasses the command guard.
			// DayTodo retains recurring traces, unlike Calendar; no command is run.
			readGraph.Chains[i].CycleMembership = nil
		}
	}
	kernel, err := core.NewTaskKernel(readGraph)
	if err != nil {
		return DayPage{}, fmt.Errorf("day read kernel: %w", err)
	}
	titles := map[core.TaskDefinitionID]string{}
	for _, d := range readGraph.Definitions {
		titles[d.ID] = d.Title
	}
	cache := map[core.LocalDate][]core.DayTrace{}
	tracesFor := func(d core.LocalDate) []core.DayTrace {
		if ts, ok := cache[d]; ok {
			return ts
		}
		ts := kernel.DayTodo(d)
		cache[d] = ts
		return ts
	}
	out := DayPage{Selected: selected, Today: today, Strip: strip, IsHistory: selected.Compare(today) < 0, IsFuture: selected.Compare(today) > 0}
	for i := range out.Strip.Cells {
		c := &out.Strip.Cells[i]
		if c.Date != nil {
			c.Pending = dayStats(tracesFor(*c.Date)).Pending
		}
	}
	ts := append([]core.DayTrace(nil), tracesFor(selected)...)
	sort.Slice(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		ap, bp := a.Status == core.TracePending && a.PinOrder != nil, b.Status == core.TracePending && b.PinOrder != nil
		if ap != bp {
			return ap
		}
		if ap {
			if *a.PinOrder != *b.PinOrder {
				return *a.PinOrder < *b.PinOrder
			}
			return a.ID < b.ID
		}
		if (a.Status == core.TracePending) != (b.Status == core.TracePending) {
			return a.Status == core.TracePending
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	for _, t := range ts {
		r := DayRow{CalendarRow: CalendarRow{TraceID: t.ID, ChainID: t.ChainID, DefinitionID: t.DefinitionID, Title: titles[t.DefinitionID], Status: t.Status}}
		if t.Status == core.TracePending && t.PinOrder != nil {
			p := *t.PinOrder
			r.Pinned = true
			r.PinOrder = &p
		}
		out.Rows = append(out.Rows, r)
	}
	out.Stats = dayStats(ts)
	if !out.IsFuture {
		out.Review = &DayReview{}
		for i := range out.Review.Trend {
			p := &out.Review.Trend[i]
			p.Date = dateAtOrdinal(civilOrdinal(selected) - 6 + i)
			p.IsSelected = i == 6
			if p.Date != nil {
				s := dayStats(tracesFor(*p.Date))
				if s.Total > 0 {
					r := float64(s.Completed) / float64(s.Total)
					p.Ratio = &r
				}
			}
		}
		if selected == today {
			if next := dateAtOrdinal(civilOrdinal(today) + 1); next != nil {
				out.Review.TomorrowTotal = len(tracesFor(*next))
			}
		}
	}
	return out, nil
}
