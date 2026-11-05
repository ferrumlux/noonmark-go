// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func date(s string) core.LocalDate {
	d, e := core.ParseLocalDate(s)
	if e != nil {
		panic(e)
	}
	return d
}
func id(n int) string   { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func ptr[T any](v T) *T { return &v }
func instant(n float64) core.Instant {
	v, e := core.InstantFromSeconds(n)
	if e != nil {
		panic(e)
	}
	return v
}
func project(t *testing.T, g core.TaskGraph, s, today core.LocalDate) CalendarMonth {
	t.Helper()
	m, e := ProjectCalendar(g, s, today)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func fixture(statuses ...core.TraceStatus) core.TaskGraph {
	d := date("2026-07-24")
	g := core.TaskGraph{Days: []core.Day{{ID: core.DayID(id(1)), Date: d}}}
	for i, status := range statuses {
		c := core.TaskChainID(id(100 + i))
		def := core.TaskDefinitionID(id(1000 + i))
		tr := core.DayTrace{ID: core.DayTraceID(id(10000 + i)), ChainID: c, DefinitionID: def, Date: d, Priority: i, Status: status}
		switch status {
		case core.TraceCompleted:
			tr.CompletedAt = ptr(core.Instant(0))
		case core.TraceUnfinished, core.TraceAbandoned, core.TraceReturnedToPool, core.TraceDeferred, core.TraceChanged:
			tr.SettledAt = ptr(core.Instant(0))
		case core.TraceCancelledDraft:
			tr.SettledAt = ptr(core.Instant(0))
			tr.DraftCancelledOn = &d
			tr.DraftCancellationID = ptr(core.CancellationID(id(20000 + i)))
		}
		g.Chains = append(g.Chains, core.TaskChain{ID: c, State: core.ChainActive})
		g.Definitions = append(g.Definitions, core.TaskDefinition{ID: def, ChainID: c, Sequence: 1, Title: fmt.Sprintf("任务 %d", i)})
		g.Traces = append(g.Traces, tr)
	}
	return g
}
func TestMonthTopology(t *testing.T) {
	for _, tc := range []struct {
		s                string
		rows, lead, days int
	}{{"2026-07-24", 5, 2, 31}, {"2026-08-01", 6, 5, 31}, {"2021-02-01", 5, 0, 28}, {"2028-02-01", 5, 1, 29}, {"2100-02-01", 5, 0, 28}, {"2000-02-01", 5, 1, 29}, {"0001-01-01", 5, 0, 31}, {"9999-12-31", 5, 2, 31}} {
		t.Run(tc.s, func(t *testing.T) {
			s := date(tc.s)
			m := project(t, core.TaskGraph{}, s, s)
			if m.RowCount != tc.rows || len(m.Slots) != tc.rows*7 {
				t.Fatalf("topology: %+v", m)
			}
			seen := map[core.LocalDate]bool{}
			for i, slot := range m.Slots {
				if slot.Index != i || slot.Row != i/7 || slot.Column != i%7 || slot.BottomBoundary != (i/7 < tc.rows-1) || slot.TrailingBoundary != (i%7 < 6) {
					t.Fatalf("boundary %d: %+v", i, slot)
				}
				n := i - tc.lead + 1
				if n < 1 || n > tc.days {
					if slot.Date != nil || slot.Day != nil {
						t.Fatalf("blank %d", i)
					}
					continue
				}
				if slot.Date == nil || slot.Day == nil || slot.Date.Day != n || slot.Day.Date != *slot.Date || seen[*slot.Date] {
					t.Fatalf("date slot %d", i)
				}
				seen[*slot.Date] = true
			}
			if len(seen) != tc.days {
				t.Fatal(len(seen))
			}
		})
	}
}
func TestDateNavigationAndLimits(t *testing.T) {
	for _, tc := range []struct {
		from  string
		delta int
		want  string
	}{{"2026-01-31", 1, "2026-02-01"}, {"2026-12-31", 1, "2027-01-01"}, {"2026-01-01", -1, "2025-12-01"}, {"2028-02-29", 0, "2028-02-01"}} {
		got, e := ShiftCalendarMonth(date(tc.from), tc.delta)
		if e != nil || got != date(tc.want) {
			t.Fatalf("shift %+v %v", got, e)
		}
	}
	for _, tc := range []struct {
		s string
		n int
	}{{"0001-01-01", -1}, {"9999-12-31", 1}, {"2026-01-31", math.MaxInt}, {"2026-01-31", math.MinInt}} {
		if _, e := ShiftCalendarMonth(date(tc.s), tc.n); e == nil {
			t.Fatal("accepted overflow")
		}
	}
	for _, bad := range []core.LocalDate{{}, {Year: 2026, Month: 2, Day: 29}, {Year: 10000, Month: 1, Day: 1}} {
		if _, e := ProjectCalendar(core.TaskGraph{}, bad, date("2026-07-24")); e == nil {
			t.Fatal("bad selection")
		}
		if _, e := ProjectCalendar(core.TaskGraph{}, date("2026-07-24"), bad); e == nil {
			t.Fatal("bad today")
		}
		if _, e := ShiftCalendarMonth(bad, 0); e == nil {
			t.Fatal("bad month")
		}
	}
	if project(t, core.TaskGraph{}, date("0001-01-01"), date("0001-01-01")).CanPrevious {
		t.Fatal("lower bound")
	}
	if project(t, core.TaskGraph{}, date("9999-12-31"), date("9999-12-31")).CanNext {
		t.Fatal("upper bound")
	}
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{{"2026-03-08", 1, "2026-03-09"}, {"2026-11-01", -1, "2026-10-31"}, {"2026-12-31", 1, "2027-01-01"}, {"2028-02-28", 1, "2028-02-29"}} {
		got, e := MoveCalendarSelection(date(tc.s), tc.n)
		if e != nil || got != date(tc.want) {
			t.Fatalf("day shift %v %v", got, e)
		}
	}
	if _, e := MoveCalendarSelection(date("0001-01-01"), -1); e == nil {
		t.Fatal("lower day")
	}
	if _, e := MoveCalendarSelection(date("9999-12-31"), 1); e == nil {
		t.Fatal("upper day")
	}
}
func TestExplicitCivilDatesIgnoreLocalZone(t *testing.T) {
	old := time.Local
	defer func() { time.Local = old }()
	s, today := date("2026-11-01"), date("2026-03-08")
	want := project(t, core.TaskGraph{}, s, today)
	for _, offset := range []int{-8 * 3600, 5*3600 + 45*60, 14 * 3600} {
		time.Local = time.FixedZone("fixture", offset)
		got := project(t, core.TaskGraph{}, s, today)
		if !reflect.DeepEqual(want, got) {
			t.Fatal("timezone-dependent projection")
		}
	}
}
func TestSelectedTodayAndEmptyDates(t *testing.T) {
	m := project(t, core.TaskGraph{}, date("2026-07-25"), date("2026-07-24"))
	if !m.SelectedDay.IsSelected || m.SelectedDay.IsToday || m.SelectedDay.Position != CalendarFuture || m.SelectedDay.HasHeat {
		t.Fatalf("selected %+v", m.SelectedDay)
	}
	for _, s := range m.Slots {
		if s.Day == nil {
			continue
		}
		if s.Date.Day == 24 && (!s.Day.IsToday || s.Day.IsSelected || s.Day.Position != CalendarToday) {
			t.Fatal("today")
		}
		if s.Date.Day == 23 && s.Day.Position != CalendarHistory {
			t.Fatal("history")
		}
	}
	m = project(t, core.TaskGraph{}, date("2026-07-24"), date("2026-07-24"))
	if !m.SelectedDay.IsToday || !m.SelectedDay.IsSelected {
		t.Fatal("same day flags")
	}
}
func TestHeatAndPreview(t *testing.T) {
	for _, tc := range []struct{ completed, pending, unfinished, heat, hidden int }{{0, 0, 0, 0, 0}, {0, 3, 0, 0, 0}, {1, 100, 0, 1, 95}, {4, 0, 0, 4, 0}, {5, 0, 2, 4, 1}, {0, 6, 0, 0, 0}, {0, 7, 0, 0, 1}} {
		statuses := []core.TraceStatus{}
		for range tc.completed {
			statuses = append(statuses, core.TraceCompleted)
		}
		for range tc.pending {
			statuses = append(statuses, core.TracePending)
		}
		for range tc.unfinished {
			statuses = append(statuses, core.TraceUnfinished)
		}
		g := fixture(statuses...)
		d := project(t, g, date("2026-07-24"), date("2026-07-24")).SelectedDay
		if d.Total != len(statuses) || d.Completed != tc.completed || d.Pending != tc.pending || d.Unfinished != tc.unfinished || d.HeatLevel != tc.heat || d.HiddenCount != tc.hidden || len(d.PreviewRows) != min(6, len(statuses)) || d.HasHeat != (len(statuses) > 0) {
			t.Fatalf("counts %+v", d)
		}
	}
}
func TestDomainOrderAndHistoricalDefinition(t *testing.T) {
	g := fixture(core.TraceCompleted, core.TracePending, core.TracePending, core.TraceUnfinished, core.TracePending)
	g.Traces[4].PinOrder = ptr(1)
	g.Traces[2].PinOrder = ptr(2)
	old := g.Definitions[0]
	newDef := old
	newDef.ID = core.TaskDefinitionID(id(999))
	newDef.Sequence = 2
	newDef.Title = "更新后标题"
	newDef.CreatedAt = instant(1)
	newDef.ContentUpdatedAt = instant(1)
	g.Definitions[0].SupersededAt = ptr(instant(1))
	g.Definitions[0].ContentUpdatedAt = instant(1)
	g.Definitions[0].SupersededByDefinitionID = &newDef.ID
	g.Definitions = append(g.Definitions, newDef)
	m := project(t, g, date("2026-07-24"), date("2026-07-24"))
	want := []int{4, 2, 1, 0, 3}
	for i, index := range want {
		if m.SelectedDay.Rows[i].TraceID != g.Traces[index].ID {
			t.Fatalf("order %+v", m.SelectedDay.Rows)
		}
	}
	if m.SelectedDay.Rows[3].Title != old.Title {
		t.Fatal("history rewritten")
	}
	for _, s := range m.Slots {
		if s.Day != nil && s.Day.IsSelected && !reflect.DeepEqual(*s.Day, m.SelectedDay) {
			t.Fatal("detail disagrees")
		}
	}
}
func TestMovementRecurringFilteringAndImmutability(t *testing.T) {
	g := fixture(core.TracePending, core.TraceReturnedToPool, core.TraceCancelledDraft, core.TraceAbandoned, core.TraceChanged, core.TracePending, core.TraceDeferred, core.TraceCompleted)
	g.Traces[4].ChangedToTraceID = &g.Traces[5].ID
	// Deferred has a valid later-day continuation, but a newer same-day fact
	// supersedes its DayTodo visibility. Distinct priorities remain reserved.
	g.Traces[7].ChainID = g.Traces[6].ChainID
	g.Traces[7].DefinitionID = g.Traces[6].DefinitionID
	g.Traces[7].CreatedAt = instant(1)
	g.Traces[7].ContentUpdatedAt = instant(1)
	g.Traces[7].CompletedAt = ptr(instant(1))
	next := g.Traces[6]
	next.ID = core.DayTraceID(id(30000))
	next.Date = date("2026-07-25")
	next.Status = core.TracePending
	next.SettledAt = nil
	next.CarriedFromTraceID = ptr(g.Traces[6].ID)
	g.Traces = append(g.Traces, next)
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: next.Date})
	g.Chains[0].CycleMembership = &core.TaskCycleMembership{SeriesID: core.TaskCycleSeriesID(id(40000)), OccurrenceDate: date("2026-07-24")}
	before := g.Clone()
	m := project(t, g, date("2026-07-24"), date("2026-07-24"))
	if m.SelectedDay.Total != 3 || m.SelectedDay.Completed != 1 || m.SelectedDay.Pending != 1 {
		t.Fatalf("filtered %+v", m.SelectedDay)
	}
	for _, r := range m.SelectedDay.Rows {
		if r.Status != core.TracePending && r.Status != core.TraceCompleted && r.Status != core.TraceAbandoned {
			t.Fatal("movement shown")
		}
		if r.ChainID == g.Chains[0].ID {
			t.Fatal("recurring shown")
		}
	}
	for _, s := range []string{"2026-08-31", "0001-01-01", "9999-12-31", "2026-07-25"} {
		project(t, g, date(s), date("2026-07-24"))
	}
	m.SelectedDay.Rows[0].Title = "mutated result"
	m.Slots[2].Date.Day = 30
	if !reflect.DeepEqual(g, before) {
		t.Fatal("projection mutated graph")
	}
	// Recurrence is validated before adaptation, not silently normalised.
	g.Chains[0].CycleMembership.SeriesID = "invalid"
	before = g.Clone()
	if _, e := ProjectCalendar(g, date("2026-07-24"), date("2026-07-24")); e == nil {
		t.Fatal("bad recurrence accepted")
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("failure mutated graph")
	}
}
func TestInvalidGraphFailsClosed(t *testing.T) {
	g := fixture(core.TracePending)
	g.Traces[0].DefinitionID = core.TaskDefinitionID(id(999))
	before := g.Clone()
	m, e := ProjectCalendar(g, date("2026-07-24"), date("2026-07-24"))
	if e == nil || !reflect.DeepEqual(m, CalendarMonth{}) {
		t.Fatalf("not closed %+v %v", m, e)
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("changed invalid graph")
	}
}

func TestDeferredVisibleWithoutReplacementAndFuturePending(t *testing.T) {
	g := fixture(core.TraceDeferred)
	next := g.Traces[0]
	next.ID = core.DayTraceID(id(30000))
	next.Date = date("2026-07-25")
	next.Status = core.TracePending
	next.SettledAt = nil
	next.CarriedFromTraceID = ptr(g.Traces[0].ID)
	g.Traces = append(g.Traces, next)
	g.Days = append(g.Days, core.Day{ID: core.DayID(id(2)), Date: next.Date})
	d := project(t, g, date("2026-07-24"), date("2026-07-24")).SelectedDay
	if d.Total != 1 || d.Rows[0].Status != core.TraceDeferred || d.Completed != 0 || d.Pending != 0 || d.Unfinished != 0 || !d.HasHeat || d.HeatLevel != 0 {
		t.Fatalf("deferred %+v", d)
	}
	d = project(t, g, next.Date, date("2026-07-24")).SelectedDay
	if d.Position != CalendarFuture || d.Total != 1 || d.Pending != 1 {
		t.Fatalf("future %+v", d)
	}
}

func TestProjectionSlicesDoNotAliasEachOtherOrInput(t *testing.T) {
	g := fixture(core.TracePending)
	before := g.Clone()
	m := project(t, g, date("2026-07-24"), date("2026-07-24"))
	var slot *CalendarSlot
	for i := range m.Slots {
		if m.Slots[i].Day != nil && m.Slots[i].Day.IsSelected {
			slot = &m.Slots[i]
			break
		}
	}
	original := slot.Day.Rows[0].Title
	m.SelectedDay.Rows[0].Title = "detail edit"
	if slot.Day.Rows[0].Title != original || m.SelectedDay.PreviewRows[0].Title != original {
		t.Fatal("detail aliases grid or preview")
	}
	slot.Day.PreviewRows[0].Title = "preview edit"
	if slot.Day.Rows[0].Title != original {
		t.Fatal("preview aliases full rows")
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("output aliases graph")
	}
}
