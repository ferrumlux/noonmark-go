// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

// Frozen from 3346d052; independent copies deliberately do not call production clone helpers.
func dayReadOldPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func dayReadOldNote(n TaskNoteEntry) TaskNoteEntry {
	n.DeletedAt = dayReadOldPtr(n.DeletedAt)
	return n
}
func dayReadOldNotes(ns []TaskNoteEntry) []TaskNoteEntry {
	if ns == nil {
		return nil
	}
	out := make([]TaskNoteEntry, len(ns))
	for i, n := range ns {
		out[i] = dayReadOldNote(n)
	}
	return out
}
func dayReadOldGraph(g TaskGraph) TaskGraph {
	out := TaskGraph{}
	if g.Days != nil {
		out.Days = make([]Day, len(g.Days))
	}
	for i, d := range g.Days {
		d.LockedAt = dayReadOldPtr(d.LockedAt)
		d.ReviewSummary = dayReadOldPtr(d.ReviewSummary)
		d.ReviewUnfinishedReason = dayReadOldPtr(d.ReviewUnfinishedReason)
		d.ReviewTomorrowNote = dayReadOldPtr(d.ReviewTomorrowNote)
		out.Days[i] = d
	}
	if g.Chains != nil {
		out.Chains = make([]TaskChain, len(g.Chains))
	}
	for i, c := range g.Chains {
		c.CycleMembership = dayReadOldPtr(c.CycleMembership)
		c.NoteEntries = dayReadOldNotes(c.NoteEntries)
		out.Chains[i] = c
	}
	if g.Definitions != nil {
		out.Definitions = make([]TaskDefinition, len(g.Definitions))
	}
	for i, d := range g.Definitions {
		d.DescriptionText = dayReadOldPtr(d.DescriptionText)
		d.SupersededAt = dayReadOldPtr(d.SupersededAt)
		d.SupersededByDefinitionID = dayReadOldPtr(d.SupersededByDefinitionID)
		if d.PlannedSubtasks != nil {
			d.PlannedSubtasks = append([]PlannedSubtask{}, d.PlannedSubtasks...)
		}
		out.Definitions[i] = d
	}
	if g.Traces != nil {
		out.Traces = make([]DayTrace, len(g.Traces))
	}
	for i, t := range g.Traces {
		t.PinOrder = dayReadOldPtr(t.PinOrder)
		t.DescriptionText = dayReadOldPtr(t.DescriptionText)
		t.NoteEntries = dayReadOldNotes(t.NoteEntries)
		t.ManualProgressPercent = dayReadOldPtr(t.ManualProgressPercent)
		t.CarriedFromTraceID = dayReadOldPtr(t.CarriedFromTraceID)
		t.ChangedToTraceID = dayReadOldPtr(t.ChangedToTraceID)
		t.CompletedAt = dayReadOldPtr(t.CompletedAt)
		t.SettledAt = dayReadOldPtr(t.SettledAt)
		t.DraftCancellationID = dayReadOldPtr(t.DraftCancellationID)
		t.DraftCancelledOn = dayReadOldPtr(t.DraftCancelledOn)
		out.Traces[i] = t
	}
	if g.Subtasks != nil {
		out.Subtasks = make([]Subtask, len(g.Subtasks))
	}
	for i, s := range g.Subtasks {
		s.CarriedFromSubtaskID = dayReadOldPtr(s.CarriedFromSubtaskID)
		s.CompletedAt = dayReadOldPtr(s.CompletedAt)
		s.SettledAt = dayReadOldPtr(s.SettledAt)
		s.DraftCancellationID = dayReadOldPtr(s.DraftCancellationID)
		out.Subtasks[i] = s
	}
	return out
}
func dayReadOldTodo(k *TaskKernel, date LocalDate) []DayTrace {
	k.mu.RLock()
	g := dayReadOldGraph(k.graph)
	k.mu.RUnlock()
	newestByChain := map[TaskChainID]Instant{}
	for _, t := range g.Traces {
		if t.Date == date && t.Status.FormsDayHistory() {
			old, exists := newestByChain[t.ChainID]
			if !exists || t.CreatedAt.After(old) {
				newestByChain[t.ChainID] = t.CreatedAt
			}
		}
	}
	var out []DayTrace
	for _, t := range g.Traces {
		if t.Date != date || !t.Status.VisibleInDayTodo() {
			continue
		}
		if t.Status == TraceDeferred && newestByChain[t.ChainID].After(t.CreatedAt) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Status == TracePending) != (b.Status == TracePending) {
			return a.Status == TracePending
		}
		if a.Status == TracePending && b.Status == TracePending {
			if (a.PinOrder != nil) != (b.PinOrder != nil) {
				return a.PinOrder != nil
			}
			if a.PinOrder != nil && b.PinOrder != nil && *a.PinOrder != *b.PinOrder {
				return *a.PinOrder < *b.PinOrder
			}
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return out
}

func dayReadCheck(t *testing.T, k *TaskKernel, d LocalDate, ids []DayTraceID) {
	t.Helper()
	got, old := k.DayTodo(d), dayReadOldTodo(k, d)
	if !reflect.DeepEqual(got, old) {
		t.Fatalf("old/new mismatch: %#v / %#v", got, old)
	}
	var actual []DayTraceID
	for _, row := range got {
		actual = append(actual, row.ID)
	}
	if !reflect.DeepEqual(actual, ids) {
		t.Fatalf("IDs %v want %v", actual, ids)
	}
}

// All load fixtures pass the public constructor before measurement. Each added
// unrelated row includes a distinct chain, definition, subtask and nested notes.
func dayReadLoad(unrelated, selected, notes int) TaskGraph {
	g := TaskGraph{}
	today, other := date("2026-11-05"), date("2026-11-06")
	for i, d := range []LocalDate{today, other} {
		g.Days = append(g.Days, Day{ID: DayID(identity(900000 + i)), Date: d, CreatedAt: instant(-100), UpdatedAt: instant(100), ReviewSummary: ptr("合成复盘")})
	}
	for i := 0; i < selected+unrelated; i++ {
		n := 1000 + i*100
		if i >= selected {
			n = 1000000 + (i-selected)*100
		}
		d := today
		if i >= selected {
			d = other
		}
		c := TaskChain{ID: TaskChainID(identity(n)), State: ChainActive, CreatedAt: instant(-100), UpdatedAt: instant(100)}
		def := TaskDefinition{ID: TaskDefinitionID(identity(n + 1)), ChainID: c.ID, Sequence: 1, Title: "合成任务", DescriptionText: ptr("合成说明"), CreatedAt: instant(-100), ContentUpdatedAt: instant(100)}
		tr := DayTrace{ID: DayTraceID(identity(n + 2)), ChainID: c.ID, DefinitionID: def.ID, Date: d, Status: TracePending, Priority: i + 1, CreatedAt: instant(-1), ContentUpdatedAt: instant(100), DescriptionText: ptr("轨迹说明"), ManualProgressPercent: ptr(20)}
		if i >= selected {
			tr.Priority = i - selected + 1
		}
		noteCount := notes
		if i >= selected {
			noteCount = 2
		}
		for j := 0; j < noteCount; j++ {
			note := TaskNoteEntry{ID: TaskNoteEntryID(identity(n + 10 + j)), Body: "合成附言", CreatedAt: instant(-1), UpdatedAt: instant(1)}
			if j%2 == 1 {
				note.DeletedAt = ptr(instant(1))
				note.Body = ""
			}
			tr.NoteEntries = append(tr.NoteEntries, note)
			c.NoteEntries = append(c.NoteEntries, note)
		}
		def.PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(identity(n + 4)), LineageID: SubtaskLineageID(identity(n + 5)), Title: "计划子项", Difficulty: DifficultySimple, Position: 1, CreatedAt: instant(-1)}}
		g.Subtasks = append(g.Subtasks, Subtask{ID: SubtaskID(identity(n + 3)), LineageID: SubtaskLineageID(identity(n + 5)), TraceID: tr.ID, Title: "合成子项", Status: SubtaskPending, Difficulty: DifficultySimple, Position: 1, CreatedAt: instant(-1), UpdatedAt: instant(100)})
		g.Chains = append(g.Chains, c)
		g.Definitions = append(g.Definitions, def)
		g.Traces = append(g.Traces, tr)
	}
	return g
}

func TestDayReadNilDatesAndNotes(t *testing.T) {
	for _, g := range []TaskGraph{{}, {Traces: []DayTrace{}}, dayReadLoad(3, 0, 2)} {
		k := kernel(t, g)
		dayReadCheck(t, k, date("2026-11-05"), nil)
	}
	var zero TaskKernel
	dayReadCheck(t, &zero, date("2026-11-05"), nil)
	for _, notes := range [][]TaskNoteEntry{nil, {}, {{ID: TaskNoteEntryID(identity(80)), Body: "合成", CreatedAt: instant(100), UpdatedAt: instant(100)}}, {{ID: TaskNoteEntryID(identity(81)), Body: "", CreatedAt: instant(100), UpdatedAt: instant(100), DeletedAt: ptr(instant(100))}}} {
		g := baseGraph()
		g.Traces[0].NoteEntries = notes
		k := kernel(t, g)
		dayReadCheck(t, k, g.Traces[0].Date, []DayTraceID{g.Traces[0].ID})
		if !reflect.DeepEqual(k.DayTodo(g.Traces[0].Date)[0].NoteEntries, notes) {
			t.Fatal("notes nil/value lost")
		}
		for _, bad := range []LocalDate{{}, {Year: 2026, Month: 13, Day: 1}, {Year: 2026, Month: 2, Day: 30}, date("2025-01-01")} {
			dayReadCheck(t, k, bad, nil)
		}
	}
}

func TestDayReadPublicStatusAndBits(t *testing.T) {
	for _, status := range []TraceStatus{TracePending, TraceCompleted, TraceUnfinished, TraceReturnedToPool, TraceAbandoned, TraceCancelledDraft} {
		for _, seconds := range []float64{-100, -1, math.Copysign(0, -1), 0, 1} {
			g := baseGraph()
			x := &g.Traces[0]
			x.Status = status
			x.CreatedAt = instant(seconds)
			x.ContentUpdatedAt = instant(seconds)
			switch status {
			case TraceCompleted:
				x.CompletedAt = ptr(x.CreatedAt)
			case TraceUnfinished, TraceReturnedToPool, TraceAbandoned:
				x.SettledAt = ptr(x.CreatedAt)
			case TraceCancelledDraft:
				x.SettledAt = ptr(x.CreatedAt)
				x.DraftCancellationID = ptr(CancellationID(identity(99)))
				x.DraftCancelledOn = ptr(x.Date)
			}
			k := kernel(t, g)
			var ids []DayTraceID
			if status != TraceReturnedToPool && status != TraceCancelledDraft {
				ids = []DayTraceID{x.ID}
			}
			dayReadCheck(t, k, x.Date, ids)
			if len(ids) > 0 && k.DayTodo(x.Date)[0].CreatedAt != x.CreatedAt {
				t.Fatal("Instant bits lost")
			}
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		g := baseGraph()
		g.Traces[0].CreatedAt = Instant(math.Float64bits(bad))
		if _, e := NewTaskKernel(g); e == nil {
			t.Fatal("nonfinite accepted")
		}
	}
}

func TestDayReadHistoryVisibility(t *testing.T) {
	for _, status := range []TraceStatus{TraceReturnedToPool, TraceChanged, TraceCancelledDraft, TraceAbandoned} {
		for _, seconds := range []float64{99, 100, 101} {
			g := continuedGraph(TraceDeferred)
			g.Traces[1].Status = TraceReturnedToPool
			g.Traces[1].SettledAt = ptr(instant(101))
			x := g.Traces[0]
			x.ID = DayTraceID(identity(50))
			x.Status = status
			x.Priority = 2
			x.CreatedAt = instant(seconds)
			x.ContentUpdatedAt = instant(102)
			x.SettledAt = ptr(instant(102))
			if status == TraceCancelledDraft {
				x.DraftCancellationID = ptr(CancellationID(identity(90)))
				x.DraftCancelledOn = ptr(x.Date)
			}
			if status == TraceChanged {
				c := g.Chains[0]
				c.ID = TaskChainID(identity(70))
				g.Chains = append(g.Chains, c)
				def := g.Definitions[0]
				def.ID = TaskDefinitionID(identity(71))
				def.ChainID = c.ID
				g.Definitions = append(g.Definitions, def)
				target := x
				target.ID = DayTraceID(identity(72))
				target.ChainID = c.ID
				target.DefinitionID = def.ID
				target.Status = TracePending
				target.SettledAt = nil
				target.Priority = 3
				target.CreatedAt = instant(102)
				g.Traces = append(g.Traces, target)
				x.ChangedToTraceID = ptr(target.ID)
			}
			g.Traces = append(g.Traces, x)
			k := kernel(t, g)
			var ids []DayTraceID
			if status == TraceChanged {
				ids = append(ids, DayTraceID(identity(72)))
			}
			if seconds <= 100 || status == TraceCancelledDraft {
				ids = append(ids, g.Traces[0].ID)
			}
			if status == TraceAbandoned {
				ids = append(ids, x.ID)
			}
			dayReadCheck(t, k, x.Date, ids)
			// The later, hidden row on another date never suppresses this source.
			for i, j := 0, len(g.Traces)-1; i < j; i, j = i+1, j-1 {
				g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i]
			}
			dayReadCheck(t, kernel(t, g), x.Date, ids)
		}
	}
}

func TestDayReadSortingAndUnrelatedLoad(t *testing.T) {
	for _, n := range []int{0, 20, 365} {
		g := dayReadLoad(n, 5, 2)
		g.Traces[0].Status = TraceCompleted
		g.Traces[0].CompletedAt = ptr(instant(1))
		g.Traces[2].PinOrder = ptr(2)
		g.Traces[3].PinOrder = ptr(1)
		g.Traces[4].PinOrder = ptr(2)
		ids := []DayTraceID{g.Traces[3].ID, g.Traces[2].ID, g.Traces[4].ID, g.Traces[1].ID, g.Traces[0].ID}
		dayReadCheck(t, kernel(t, g), date("2026-11-05"), ids)
		for i, j := 0, len(g.Traces)-1; i < j; i, j = i+1, j-1 {
			g.Traces[i], g.Traces[j] = g.Traces[j], g.Traces[i]
		}
		dayReadCheck(t, kernel(t, g), date("2026-11-05"), ids)
	}
	// Duplicate same-day priority is rejected publicly. These are pure comparator
	// specimens, not claims that the constructor accepts priority ties.
	g := dayReadLoad(0, 5, 0)
	for i := range g.Traces {
		g.Traces[i].Priority = 1
	}
	for i, v := range []float64{1, 0, math.Copysign(0, -1), -1, -2} {
		g.Traces[i].CreatedAt = instant(v)
	}
	if _, e := NewTaskKernel(g); e == nil {
		t.Fatal("priority tie unexpectedly valid")
	}
	k := &TaskKernel{graph: dayReadOldGraph(g)}
	dayReadCheck(t, k, date("2026-11-05"), []DayTraceID{g.Traces[4].ID, g.Traces[3].ID, g.Traces[1].ID, g.Traces[2].ID, g.Traces[0].ID})
}

func TestDayReadCommandGeneratedOracle(t *testing.T) {
	k := kernel(t, TaskGraph{})
	today := date("2026-11-05")
	id := DayTraceID(identity(4))
	for _, op := range []func() error{
		func() error {
			return k.CreatePoolTask(CreatePoolTaskInput{ChainID: TaskChainID(identity(2)), DefinitionID: TaskDefinitionID(identity(3)), Title: "合成", Now: instant(-5)})
		},
		func() error {
			return k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: TaskChainID(identity(2)), TraceID: id, DayID: DayID(identity(1)), Date: today, Today: today, Now: instant(-4)})
		},
		func() error { return k.MarkCompleted(id, today, instant(-3)) }, func() error { return k.UndoCompleted(id, today, instant(-2)) },
		func() error {
			return k.DeferCurrentTrace(TraceCarryoverInput{ChainID: TaskChainID(identity(2)), TraceID: id, NewTraceID: DayTraceID(identity(5)), DayID: DayID(identity(6)), TargetDate: date("2026-11-06"), Today: today, Now: instant(-1)})
		},
	} {
		if e := op(); e != nil {
			t.Fatal(e)
		}
		for _, d := range []LocalDate{today, date("2026-11-06")} {
			if !reflect.DeepEqual(k.DayTodo(d), dayReadOldTodo(k, d)) {
				t.Fatal("command oracle mismatch")
			}
		}
	}
	dayReadCheck(t, k, today, []DayTraceID{id})
	dayReadCheck(t, k, date("2026-11-06"), []DayTraceID{DayTraceID(identity(5))})
}

var dayReadBenchSink []DayTrace

func BenchmarkDayReadAB(b *testing.B) {
	for _, tc := range []struct {
		name                       string
		unrelated, selected, notes int
	}{{"small", 0, 4, 2}, {"medium", 100, 4, 2}, {"annual", 3650, 4, 2}, {"returns", 100, 40, 2}, {"notes", 100, 4, 20}} {
		g := dayReadLoad(tc.unrelated, tc.selected, tc.notes)
		k, e := NewTaskKernel(g)
		if e != nil {
			b.Fatal(e)
		}
		d := date("2026-11-05")
		if !reflect.DeepEqual(k.DayTodo(d), dayReadOldTodo(k, d)) {
			b.Fatal("fixture mismatch")
		}
		for _, old := range []bool{true, false} {
			b.Run(fmt.Sprintf("%s/old=%v", tc.name, old), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if old {
						dayReadBenchSink = dayReadOldTodo(k, d)
					} else {
						dayReadBenchSink = k.DayTodo(d)
					}
				}
			})
		}
	}
}

func TestDayReadAllocationScaling(t *testing.T) {
	var newAllocs, oldAllocs []float64
	for _, n := range []int{0, 100, 1000} {
		k, e := NewTaskKernel(dayReadLoad(n, 4, 2))
		if e != nil {
			t.Fatal(e)
		}
		d := date("2026-11-05")
		newAllocs = append(newAllocs, testing.AllocsPerRun(10, func() { dayReadBenchSink = k.DayTodo(d) }))
		oldAllocs = append(oldAllocs, testing.AllocsPerRun(10, func() { dayReadBenchSink = dayReadOldTodo(k, d) }))
	}
	if newAllocs[2] > newAllocs[0]*1.5+4 {
		t.Fatalf("unrelated allocation growth: %v", newAllocs)
	}
	if oldAllocs[2] < oldAllocs[0]*5 || oldAllocs[2] < newAllocs[2]*5 {
		t.Fatalf("missing A/B separation new=%v old=%v", newAllocs, oldAllocs)
	}
	base := dayReadLoad(100, 4, 2)
	for _, tc := range []struct{ selected, notes int }{{40, 2}, {4, 20}} {
		more := dayReadLoad(100, tc.selected, tc.notes)
		if !reflect.DeepEqual(base.Traces[4:], more.Traces[tc.selected:]) || !reflect.DeepEqual(base.Chains[4:], more.Chains[tc.selected:]) || !reflect.DeepEqual(base.Definitions[4:], more.Definitions[tc.selected:]) || !reflect.DeepEqual(base.Subtasks[4:], more.Subtasks[tc.selected:]) {
			t.Fatal("return-growth fixture changed unrelated data")
		}
		k, e := NewTaskKernel(more)
		if e != nil {
			t.Fatal(e)
		}
		alloc := testing.AllocsPerRun(10, func() { dayReadBenchSink = k.DayTodo(date("2026-11-05")) })
		if alloc <= newAllocs[1] {
			t.Fatalf("returned data allocation did not grow: %v <= %v", alloc, newAllocs[1])
		}
	}

	t.Logf("new=%v old=%v", newAllocs, oldAllocs)
}
