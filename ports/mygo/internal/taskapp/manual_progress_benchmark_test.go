// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// One real domain-command-created task on each of 365 dates. Earlier days are
// settled, including completed and unfinished traces, with complete empty-
// catalog history. Fixture installation is an isolated test aggregate commit,
// not an application import path or a claim of the upstream demo's full scope.
func manualServiceYearFixture(b *testing.B) (*Service, core.TaskChainID, core.DayTraceID, int) {
	b.Helper()
	must := func(err error) {
		if err != nil {
			b.Fatal(err)
		}
	}
	s, err := Create(filepath.Join(b.TempDir(), "session"), testDay)
	must(err)
	k, err := core.NewTaskKernel(core.TaskGraph{})
	must(err)
	serial := 10000
	id := func() string { serial++; return fmt.Sprintf("00000000-0000-4000-8000-%012X", serial) }
	now, err := core.InstantFromSeconds(100)
	must(err)
	start, err := testDay.AddDays(-364)
	must(err)
	var chain core.TaskChainID
	var trace core.DayTraceID
	for i := 0; i < 365; i++ {
		day, err := start.AddDays(i)
		must(err)
		chain, trace = core.TaskChainID(id()), core.DayTraceID(id())
		must(k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(id()), Title: "年度合成任务", Now: now}))
		must(k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(id()), Date: day, Today: day, Now: now}))
		if i < 364 && i%3 == 0 {
			must(k.MarkCompleted(trace, day, now))
		}
	}
	must(k.SettleTaskDays(testDay, now))
	wire, err := CaptureEmptyHistory(s.snapshot, s.graph, k.Snapshot())
	must(err)
	wire, graph, err := normalizeSnapshot(wire)
	must(err)
	must(s.disk.commit(wire))
	s.graph, s.snapshot = graph, wire
	bytes, err := encodeSnapshot(wire)
	must(err)
	return s, chain, trace, len(bytes)
}

// Run with -benchtime=1x: each benchmark iteration is a complete fresh isolated
// session so no test silently crosses the bounded store's revision/byte limits.
func BenchmarkManualProgressServiceYear(b *testing.B) {
	for _, mode := range []string{"single", "continuous-20", "noop-20"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				s, chain, trace, size := manualServiceYearFixture(b)
				if mode == "noop-20" {
					if err := s.SetManualProgress(chain, trace, 40); err != nil {
						b.Fatal(err)
					}
				}
				count := 1
				if mode != "single" {
					count = 20
				}
				b.ReportMetric(float64(size), "snapshot-bytes")
				b.ReportMetric(float64(count), "commands/op")
				b.StartTimer()
				for j := 0; j < count; j++ {
					percent := j + 1
					if mode == "noop-20" {
						percent = 40
					}
					accepted, err := s.SetManualProgressAcknowledged(chain, trace, percent)
					if err != nil || accepted != percent {
						b.Fatalf("accepted %d want %d: %v", accepted, percent, err)
					}
				}
				b.StopTimer()
				if err := s.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
