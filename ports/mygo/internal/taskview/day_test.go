// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func projectDayTest(t *testing.T, g core.TaskGraph, selected, today core.LocalDate) DayPage {
	t.Helper()
	p, e := ProjectDay(g, selected, today)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestDayStripFixedAnchor(t *testing.T) {
	today := date("2026-07-24")
	for _, tc := range []struct {
		offset, index int
		start         string
	}{
		{-21, 13, "2026-06-20"}, {-20, 0, "2026-07-04"}, {-7, 13, "2026-07-04"}, {-6, 0, "2026-07-18"}, {0, 6, "2026-07-18"}, {7, 13, "2026-07-18"}, {8, 0, "2026-08-01"}, {21, 13, "2026-08-01"}, {22, 0, "2026-08-15"},
	} {
		selected, _ := today.AddDays(tc.offset)
		s, e := ProjectDateStrip(today, selected)
		if e != nil || s.SelectedIndex != tc.index || s.Cells[0].Date == nil || *s.Cells[0].Date != date(tc.start) {
			t.Fatalf("%+v: %+v %v", tc, s, e)
		}
		for i, c := range s.Cells {
			want, _ := date(tc.start).AddDays(i)
			if c.Date == nil || *c.Date != want || c.IsSelected != (i == tc.index) || c.IsToday != (want == today) {
				t.Fatalf("cell %d: %+v", i, c)
			}
		}
	}
}
func TestDayStripCivilLimits(t *testing.T) {
	for _, todayS := range []string{"0001-01-01", "9999-12-31", "2026-12-31", "2028-02-29", "1900-03-01", "2000-03-01"} {
		today := date(todayS)
		for _, selectedS := range []string{todayS, "0001-01-01", "9999-12-31", "1000-07-01", "9000-07-01"} {
			selected := date(selectedS)
			s, e := ProjectDateStrip(today, selected)
			if e != nil {
				t.Fatal(e)
			}
			if s.SelectedIndex < 0 || s.SelectedIndex > 13 || s.Cells[s.SelectedIndex].Date == nil || *s.Cells[s.SelectedIndex].Date != selected {
				t.Fatalf("lost selection %s %s %+v", todayS, selectedS, s)
			}
			anchor := civilOrdinal(today) - 6
			start := civilOrdinal(selected) - s.SelectedIndex
			if (start-anchor)%14 != 0 {
				t.Fatal("reanchored")
			}
			seen := map[core.LocalDate]bool{}
			for i, c := range s.Cells {
				n := start + i
				if n < 1 || n > 3652059 {
					if c.Date != nil || c.IsToday || c.IsSelected {
						t.Fatal("nonempty edge")
					}
					continue
				}
				if c.Date == nil || !c.Date.Valid() || civilOrdinal(*c.Date) != n || seen[*c.Date] {
					t.Fatal("invalid/duplicate cell")
				}
				seen[*c.Date] = true
			}
		}
	}
	for _, bad := range []core.LocalDate{{}, {Year: 2026, Month: 2, Day: 29}, {Year: 10000, Month: 1, Day: 1}} {
		if _, e := ProjectDateStrip(bad, date("2026-07-24")); e == nil {
			t.Fatal("bad today")
		}
		if _, e := ProjectDay(core.TaskGraph{}, bad, date("2026-07-24")); e == nil {
			t.Fatal("bad selection")
		}
	}
	// A reference based on calendar year-day, not the implementation's formula.
	for year := 1; year <= 9999; year++ {
		for _, month := range []int{1, 3, 12} {
			d := core.LocalDate{Year: year, Month: month, Day: 1}
			p := dateAtOrdinal(civilOrdinal(d))
			if p == nil || *p != d {
				t.Fatalf("ordinal roundtrip %v", d)
			}
		}
	}
	old := time.Local
	defer func() { time.Local = old }()
	a, _ := ProjectDateStrip(date("2026-03-08"), date("2026-11-01"))
	time.Local = time.FixedZone("test", 14*3600)
	b, _ := ProjectDateStrip(date("2026-03-08"), date("2026-11-01"))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("zone dependent")
	}
}
func TestDayDefaultPresentationOrder(t *testing.T) {
	g := fixture(core.TraceCompleted, core.TracePending, core.TracePending, core.TracePending, core.TracePending, core.TracePending, core.TraceUnfinished)
	for i := range g.Traces {
		g.Traces[i].Priority = 20 - i
		g.Traces[i].CreatedAt = instant(-10)
	}
	g.Traces[0].CreatedAt = instant(-100)
	g.Traces[1].CreatedAt = instant(-20)
	g.Traces[3].PinOrder = ptr(2)
	g.Traces[4].PinOrder = ptr(1)
	g.Traces[5].PinOrder = ptr(1)
	// Reverse input to prove deterministic ties rather than stable input order.
	for i, j := 0, len(g.Traces)-1; i < j; i, j = i+1, j-1 {
		g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i]
	}
	p := projectDayTest(t, g, date("2026-07-24"), date("2026-07-24"))
	for i, n := range []int{4, 5, 3, 1, 2, 0, 6} {
		r := p.Rows[i]
		if r.TraceID != core.DayTraceID(id(10000+n)) || r.Pinned != (i < 3) || (r.PinOrder != nil) != (i < 3) {
			t.Fatalf("row %d %+v", i, r)
		}
	}
	before := g.Clone()
	*p.Rows[0].PinOrder = 99
	p.Rows[0].Title = "changed"
	p.Strip.Cells[6].Date.Day = 1
	p.Review.Trend[6].Date.Day = 2
	if !reflect.DeepEqual(g, before) {
		t.Fatal("aliases graph")
	}
}
func TestDayStatsAndReview(t *testing.T) {
	d := date("2026-07-24")
	for _, tc := range []struct{ complete, total, rate int }{{0, 0, 0}, {0, 3, 0}, {1, 3, 33}, {2, 3, 67}, {1, 24, 4}, {3, 3, 100}} {
		statuses := make([]core.TraceStatus, tc.total)
		for i := range statuses {
			statuses[i] = core.TracePending
			if i < tc.complete {
				statuses[i] = core.TraceCompleted
			}
		}
		g := fixture(statuses...)
		p := projectDayTest(t, g, d, d)
		if p.Stats.Total != tc.total || p.Stats.Completed != tc.complete || p.Stats.Pending != tc.total-tc.complete || p.Stats.CompletionRate != tc.rate || p.Strip.Cells[6].Pending != p.Stats.Pending {
			t.Fatalf("counts %+v", p)
		}
		if p.Review == nil || (p.Review.Trend[6].Ratio == nil) != (tc.total == 0) {
			t.Fatal("nil distinction")
		}
		if tc.total > 0 && *p.Review.Trend[6].Ratio != float64(tc.complete)/float64(tc.total) {
			t.Fatal("ratio")
		}
		for i, pt := range p.Review.Trend {
			want, _ := d.AddDays(i - 6)
			if pt.Date == nil || *pt.Date != want || pt.IsSelected != (i == 6) {
				t.Fatal("trend date")
			}
			if i < 6 && pt.Ratio != nil {
				t.Fatal("empty day ratio")
			}
		}
	}
	g := fixture(core.TracePending, core.TraceCompleted, core.TraceUnfinished, core.TraceAbandoned)
	p := projectDayTest(t, g, d, date("2026-08-01"))
	if !p.IsHistory || p.IsFuture || p.Stats.Unfinished != 1 || p.Stats.Abandoned != 1 || p.Stats.Total != 4 {
		t.Fatal(p)
	}
	p = projectDayTest(t, g, d, date("2026-07-23"))
	if p.Review != nil || !p.IsFuture || p.Stats.Total != 4 {
		t.Fatal("future review")
	}
	p = projectDayTest(t, g, date("2026-07-23"), date("2026-07-23"))
	if p.Review.TomorrowTotal != 4 {
		t.Fatal("tomorrow")
	}
	p = projectDayTest(t, g, date("2026-07-23"), date("2026-07-24"))
	if p.Review.TomorrowTotal != 0 {
		t.Fatal("historic signal")
	}
	p = projectDayTest(t, core.TaskGraph{}, date("0001-01-01"), d)
	for i, pt := range p.Review.Trend {
		if (pt.Date == nil) != (i < 6) || pt.Ratio != nil {
			t.Fatal("trend bounds")
		}
	}
	p = projectDayTest(t, core.TaskGraph{}, date("9999-12-31"), date("9999-12-31"))
	if p.Review.TomorrowTotal != 0 {
		t.Fatal("max tomorrow")
	}
}
func TestDayEligibilityAndReadonly(t *testing.T) {
	g := fixture(core.TracePending, core.TraceReturnedToPool, core.TraceCancelledDraft, core.TraceAbandoned, core.TraceChanged, core.TracePending, core.TraceDeferred, core.TraceCompleted)
	g.Traces[4].ChangedToTraceID = &g.Traces[5].ID
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
	d := date("2026-07-24")
	p := projectDayTest(t, g, d, d)
	if p.Stats.Total != 4 || p.Stats.Pending != 2 || p.Stats.Completed != 1 || p.Stats.Deferred != 0 || p.Stats.Abandoned != 1 || p.Strip.Cells[6].Pending != 2 || p.Review.TomorrowTotal != 1 {
		t.Fatalf("eligibility %+v", p)
	}
	// Day preserves the recurring occurrence; Calendar intentionally excludes it.
	calendar := project(t, g, d, d)
	if calendar.SelectedDay.Total != 3 || calendar.SelectedDay.Pending != 1 {
		t.Fatal("Calendar recurring exclusion changed")
	}
	found := false
	for _, row := range p.Rows {
		if row.TraceID == g.Traces[0].ID {
			found = true
		}
	}
	if !found || p.Review.Trend[6].Ratio == nil || *p.Review.Trend[6].Ratio != 0.25 {
		t.Fatal("Day recurring occurrence omitted from rows or review")
	}
	if _, err := core.NewTaskKernel(g); err != core.ErrUnsupportedRecurring {
		t.Fatal("ordinary kernel recurring command guard changed")
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("mutated")
	}
	// Remove the newer same-day replacement; deferred becomes visible again.
	g.Traces = append(g.Traces[:7], g.Traces[8:]...)
	p = projectDayTest(t, g, d, d)
	if p.Stats.Deferred != 1 {
		t.Fatal("deferred missing")
	}
	g.Chains[0].CycleMembership.SeriesID = "invalid"
	before = g.Clone()
	p, e := ProjectDay(g, d, d)
	if e == nil || !reflect.DeepEqual(p, DayPage{}) || !reflect.DeepEqual(g, before) {
		t.Fatal("invalid recurring accepted")
	}
}
func TestDayHistoricalTitleAndInvalidGraph(t *testing.T) {
	g := fixture(core.TraceCompleted)
	old := g.Definitions[0]
	newDef := old
	newDef.ID = core.TaskDefinitionID(id(999))
	newDef.Sequence = 2
	newDef.Title = "新标题"
	newDef.CreatedAt = instant(1)
	newDef.ContentUpdatedAt = instant(1)
	g.Definitions[0].SupersededAt = ptr(instant(1))
	g.Definitions[0].ContentUpdatedAt = instant(1)
	g.Definitions[0].SupersededByDefinitionID = &newDef.ID
	g.Definitions = append(g.Definitions, newDef)
	d := date("2026-07-24")
	p := projectDayTest(t, g, d, d)
	if p.Rows[0].Title != old.Title {
		t.Fatal("rewrote historical definition")
	}
	for _, bad := range []core.TaskGraph{func() core.TaskGraph { b := g.Clone(); b.Traces[0].PinOrder = ptr(1); return b }(), func() core.TaskGraph {
		b := g.Clone()
		b.Traces[0].DefinitionID = core.TaskDefinitionID(id(9999))
		return b
	}()} {
		before := bad.Clone()
		p, e := ProjectDay(bad, d, d)
		if e == nil || !reflect.DeepEqual(p, DayPage{}) || !reflect.DeepEqual(before, bad) {
			t.Fatal("failed to reject invalid graph")
		}
	}
}
