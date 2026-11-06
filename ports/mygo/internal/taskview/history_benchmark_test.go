// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"strings"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// historyYearGraph replays 365 independent ordinary tasks through domain
// commands. It is a query-cost fixture, not the full interactive-demo contract.
func historyYearGraph(b *testing.B) core.TaskGraph {
	b.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	if e != nil {
		b.Fatal(e)
	}
	check := func(e error) {
		b.Helper()
		if e != nil {
			b.Fatal(e)
		}
	}
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	for n := 0; n < 365; n++ {
		d := start.AddDate(0, 0, n)
		day := core.LocalDate{Year: d.Year(), Month: int(d.Month()), Day: d.Day()}
		base := 100000 + n*10
		c, tr := core.TaskChainID(id(base)), core.DayTraceID(id(base+2))
		clock := float64(n*10 + 1)
		check(k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: c, DefinitionID: core.TaskDefinitionID(id(base + 1)), Title: "synthetic yearly task", Now: instant(clock)}))
		check(k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: c, TraceID: tr, DayID: core.DayID(id(base + 3)), Date: day, Today: day, Now: instant(clock + 1)}))
		check(k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: c, TraceID: tr, SubtaskID: core.SubtaskID(id(base + 4)), LineageID: core.SubtaskLineageID(id(base + 5)), Title: "synthetic yearly step", Difficulty: core.DifficultyMedium, Today: day, Now: instant(clock + 2)}))
		if n%2 == 0 {
			check(k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskComplete, ChainID: c, TraceID: tr, SubtaskID: core.SubtaskID(id(base + 4)), Today: day, Now: instant(clock + 3)}))
			check(k.MarkCompleted(tr, day, instant(clock+4)))
		}
	}
	check(k.SettleTaskDays(core.LocalDate{Year: 2026, Month: 1, Day: 1}, instant(4000)))
	return k.Snapshot()
}
func BenchmarkHistoryYear(b *testing.B) {
	g := historyYearGraph(b)
	p, e := NewHistoryProjection(g, strings.Compare)
	if e != nil {
		b.Fatal(e)
	}
	if len(p.CompletedTaskHierarchies()) != 183 || len(p.UnfinishedPool()) != 182 {
		b.Fatal("year fixture projection count")
	}
	b.Run("ConstructValidatedSnapshot", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := NewHistoryProjection(g, strings.Compare); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("CompletedHierarchy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = p.CompletedTaskHierarchies()
		}
	})
	b.Run("UnfinishedPool", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = p.UnfinishedPool()
		}
	})
}
