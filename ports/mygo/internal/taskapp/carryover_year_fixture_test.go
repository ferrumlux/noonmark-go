// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// TestCarryoverYearFixture is a test-only aggregate installer, not an import
// API. The root native UI test invokes only this test in a separate process so
// no production symbol, unsafe field access or hand-edited disk JSON is needed.
// Without the export variable it still builds and verifies its own fresh store.
func TestCarryoverYearFixture(t *testing.T) {
	started := time.Now()
	path := os.Getenv("NOONMARK_CARRYOVER_YEAR_SESSION")
	if path == "" {
		path = filepath.Join(t.TempDir(), "session")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("fixture export requires a clean absolute new directory")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("fixture export must not reuse an existing path", err)
	}
	s, err := Create(path, testDay)
	must(t, err)
	defer s.Close()
	k, err := core.NewTaskKernel(core.TaskGraph{})
	must(t, err)
	serial := 20000
	id := func() string { serial++; return fmt.Sprintf("00000000-0000-4000-8000-%012X", serial) }
	clock := int64(100)
	now := func() core.Instant {
		clock++
		x, e := core.InstantFromSeconds(float64(clock))
		must(t, e)
		return x
	}
	start, err := testDay.AddDays(-182)
	must(t, err)
	for i := 0; i < 365; i++ {
		date, e := start.AddDays(i)
		must(t, e)
		commandToday := date
		if date.Compare(testDay) > 0 {
			commandToday = testDay
		}
		for slot := 0; slot < 2; slot++ {
			chain, trace := core.TaskChainID(id()), core.DayTraceID(id())
			title := fmt.Sprintf("合成年度任务 %s · %d", date.String(), slot+1)
			must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(id()), Title: title, Now: now()}))
			must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(id()), Date: date, Today: commandToday, Now: now()}))
			if i <= 182 && slot == 0 {
				must(t, k.MarkCompleted(trace, date, now()))
			}
		}
	}
	// Additional today tasks make the native popup exercise a real long list.
	for i := 0; i < 32; i++ {
		chain, trace := core.TaskChainID(id()), core.DayTraceID(id())
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(id()), Title: fmt.Sprintf("今日长滚动任务 %02d", i+1), Now: now()}))
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(id()), Date: testDay, Today: testDay, Now: now()}))
	}
	must(t, k.SettleTaskDays(testDay, now()))
	graph := k.Snapshot()
	if len(graph.Days) != 365 || len(graph.Chains) != 762 || len(graph.Definitions) != 762 || len(graph.Traces) != 762 || len(graph.Subtasks) != 0 {
		t.Fatal("unexpected yearly domain fixture cardinality")
	}
	dates := map[core.LocalDate]int{}
	completed, unfinished, pending, locked := 0, 0, 0, 0
	for _, trace := range graph.Traces {
		dates[trace.Date]++
		switch trace.Status {
		case core.TraceCompleted:
			completed++
		case core.TraceUnfinished:
			unfinished++
		case core.TracePending:
			pending++
		default:
			t.Fatal("unexpected trace status", trace.Status)
		}
	}
	for i := 0; i < 365; i++ {
		date, e := start.AddDays(i)
		must(t, e)
		want := 2
		if date == testDay {
			want = 34
		}
		if dates[date] != want {
			t.Fatal("missing date or wrong daily task count", date)
		}
	}
	for _, day := range graph.Days {
		if (day.LockedAt != nil) != (day.Date.Compare(testDay) < 0) {
			t.Fatal("incorrect locked-day boundary", day.Date)
		}
		if day.LockedAt != nil {
			locked++
		}
	}
	for _, chain := range graph.Chains {
		if chain.CycleMembership != nil {
			t.Fatal("fixture escaped ordinary-task scope")
		}
	}
	if completed != 183 || unfinished != 182 || pending != 397 || locked != 182 {
		t.Fatalf("completed=%d unfinished=%d pending=%d locked=%d", completed, unfinished, pending, locked)
	}
	// Capture from the genuine empty aggregate, then perform all normal schema,
	// domain and classification-history checks before the single test install.
	wire, err := CaptureEmptyHistory(s.snapshot, s.graph, graph)
	must(t, err)
	wire, graph, err = normalizeSnapshot(wire)
	must(t, err)
	must(t, ValidateEmptyHistory(wire))
	must(t, s.disk.commit(wire))
	s.graph, s.snapshot = graph, wire
	payload, err := encodeSnapshot(wire)
	must(t, err)
	if s.disk.revision != 2 {
		t.Fatal("fixture must use initial commit plus one test aggregate install", s.disk.revision)
	}
	must(t, s.Close())
	reopened, err := Open(path, testDay)
	must(t, err)
	defer reopened.Close()
	if !reflect.DeepEqual(graph, reopened.Snapshot()) || !historyEqual(wire, reopened.snapshot) || reopened.disk.revision != 2 {
		t.Fatal("fixture Close/Open changed graph, history or revision")
	}
	must(t, ValidateEmptyHistory(reopened.snapshot))
	end, err := testDay.AddDays(182)
	must(t, err)
	t.Logf("year fixture start=%s today=%s end=%s dates=365 tasks=762 completed=183 unfinished=182 pending=397 locked=182 revisions=2 snapshot_bytes=%d elapsed=%s", start, testDay, end, len(payload), time.Since(started))
}
