// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

// Synthetic query scales, not the real annual development-path acceptance.
// Setup and validation are outside the measured region. ProjectDay is unchanged.
func BenchmarkDayReadProjectDay(b *testing.B) {
	for _, count := range []int{1, 30, 365} {
		b.Run(fmt.Sprintf("Days%d", count), func(b *testing.B) {
			k, err := core.NewTaskKernel(core.TaskGraph{})
			if err != nil {
				b.Fatal(err)
			}
			selected := core.LocalDate{Year: 2026, Month: 1, Day: 1}
			for n := 0; n < count; n++ {
				d, err := selected.AddDays(n)
				if err != nil {
					b.Fatal(err)
				}
				base := 900000 + n*10
				chain := core.TaskChainID(id(base))
				if err := k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(id(base + 1)), Title: "synthetic projection", Now: instant(float64(n*2 + 1))}); err != nil {
					b.Fatal(err)
				}
				if err := k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(id(base + 2)), DayID: core.DayID(id(base + 3)), Date: d, Today: d, Now: instant(float64(n*2 + 2))}); err != nil {
					b.Fatal(err)
				}
			}
			g := k.Snapshot()
			if err := core.ValidateTaskGraph(g); err != nil {
				b.Fatal(err)
			}
			if _, err := ProjectDay(g, selected, selected); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ProjectDay(g, selected, selected); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
