// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"reflect"
	"testing"
)

func returnInput() ReturnToPoolInput {
	return ReturnToPoolInput{TraceID: DayTraceID(identity(4)), ChainID: TaskChainID(identity(2)), NewDefinitionID: TaskDefinitionID(identity(20)), CancellationID: CancellationID(identity(21)), Today: date("2026-11-05"), Now: instant(200)}
}
func TestReturnToPoolCurrentAndFuture(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "today", true: "future"}[future], func(t *testing.T) {
			g := baseGraph()
			in := returnInput()
			if future {
				g.Days[0].Date = date("2026-11-06")
				g.Traces[0].Date = g.Days[0].Date
			}
			g.Traces[0].DescriptionText = ptr("轨迹说明")
			g.Traces[0].PinOrder = ptr(1)
			g.Traces[0].NoteEntries = []TaskNoteEntry{{ID: TaskNoteEntryID(identity(40)), Body: "合成备注", CreatedAt: instant(100), UpdatedAt: instant(100)}}
			s := child()
			g.Subtasks = []Subtask{s}
			in.PlannedIDs = map[SubtaskID]PlannedSubtaskID{s.ID: PlannedSubtaskID(identity(22))}
			k, e := NewTaskKernel(g)
			if e != nil {
				t.Fatal(e)
			}
			if e = k.ReturnToPool(in); e != nil {
				t.Fatal(e)
			}
			got := k.Snapshot()
			tr := got.Traces[0]
			d := got.Definitions[1]
			status := TraceReturnedToPool
			if future {
				status = TraceCancelledDraft
			}
			if tr.Status != status || tr.SettledAt == nil || *tr.SettledAt != in.Now || tr.PinOrder != nil || tr.DefinitionID != g.Traces[0].DefinitionID {
				t.Fatal("trace facts", tr)
			}
			if (tr.DraftCancellationID != nil) != future || (tr.DraftCancelledOn != nil) != future {
				t.Fatal("cancellation facts")
			}
			if d.Title != g.Definitions[0].Title || *d.DescriptionText != "轨迹说明" || d.Sequence != 2 || len(d.PlannedSubtasks) != 1 || d.PlannedSubtasks[0].LineageID != s.LineageID || d.PlannedSubtasks[0].CreatedAt != in.Now {
				t.Fatal("materialization", d)
			}
			if !reflect.DeepEqual(got.Chains[0].NoteEntries, g.Traces[0].NoteEntries) || !reflect.DeepEqual(got.Subtasks, g.Subtasks) {
				t.Fatal("content copy")
			}
			if len(k.DayTodo(tr.Date)) != 0 || !got.inTaskPool(got.Chains[0]) {
				t.Fatal("projection")
			}
			if e = k.ReturnToPool(in); !errors.Is(e, ErrInvalidTransition) {
				t.Fatal("duplicate", e)
			}
			if !reflect.DeepEqual(got, k.Snapshot()) {
				t.Fatal("duplicate mutated")
			}
			e = k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: in.ChainID, TraceID: DayTraceID(identity(30)), DayID: DayID(identity(31)), Date: in.Today, Today: in.Today, Now: instant(300), SubtaskIDs: map[PlannedSubtaskID]SubtaskID{d.PlannedSubtasks[0].ID: SubtaskID(identity(32))}})
			if e != nil {
				t.Fatal("reschedule", e)
			}
		})
	}
}
func TestReturnToPoolRejectsAtomically(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TaskGraph, *ReturnToPoolInput)
	}{
		{"past", func(g *TaskGraph, in *ReturnToPoolInput) { in.Today = date("2026-11-06") }},
		{"locked", func(g *TaskGraph, in *ReturnToPoolInput) {
			g.Traces[0].Status = TraceUnfinished
			g.Traces[0].SettledAt = ptr(instant(100))
			g.Days[0].LockedAt = ptr(instant(100))
		}},
		{"completed", func(g *TaskGraph, in *ReturnToPoolInput) {
			g.Traces[0].Status = TraceCompleted
			g.Traces[0].CompletedAt = ptr(instant(100))
		}},
		{"wrong-chain", func(g *TaskGraph, in *ReturnToPoolInput) { in.ChainID = TaskChainID(identity(90)) }},
		{"missing", func(g *TaskGraph, in *ReturnToPoolInput) { in.TraceID = DayTraceID(identity(90)) }},
		{"old-clock", func(g *TaskGraph, in *ReturnToPoolInput) { in.Now = instant(99) }},
		{"duplicate-definition", func(g *TaskGraph, in *ReturnToPoolInput) { in.NewDefinitionID = g.Definitions[0].ID }},
		{"future-no-cancellation", func(g *TaskGraph, in *ReturnToPoolInput) { in.Today = date("2026-11-04"); in.CancellationID = "" }},
		{"missing-planned-id", func(g *TaskGraph, in *ReturnToPoolInput) { g.Subtasks = []Subtask{child()} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := baseGraph()
			in := returnInput()
			tc.edit(&g, &in)
			k, e := NewTaskKernel(g)
			if e != nil {
				t.Fatal(e)
			}
			before := k.Snapshot()
			if k.ReturnToPool(in) == nil {
				t.Fatal("accepted")
			}
			if !reflect.DeepEqual(before, k.Snapshot()) {
				t.Fatal("mutated")
			}
		})
	}
}

func TestReturnMaterializesOnlyOpenSubtasks(t *testing.T) {
	g := baseGraph()
	in := returnInput()
	a := child()
	b := child()
	b.ID = SubtaskID(identity(12))
	b.LineageID = SubtaskLineageID(identity(13))
	b.Position = 2
	b.Status = SubtaskCompleted
	b.CompletedAt = ptr(instant(100))
	c := child()
	c.ID = SubtaskID(identity(14))
	c.LineageID = SubtaskLineageID(identity(15))
	c.Position = 3
	c.Status = SubtaskUnfinished
	c.SettledAt = ptr(instant(100))
	g.Subtasks = []Subtask{c, b, a}
	in.PlannedIDs = map[SubtaskID]PlannedSubtaskID{a.ID: PlannedSubtaskID(identity(23)), c.ID: PlannedSubtaskID(identity(24))}
	k, e := NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ReturnToPool(in); e != nil {
		t.Fatal(e)
	}
	got := k.Snapshot()
	planned := got.Definitions[1].PlannedSubtasks
	if len(planned) != 2 || planned[0].LineageID != a.LineageID || planned[1].LineageID != c.LineageID || planned[1].Position != 2 {
		t.Fatal(planned)
	}
	if !reflect.DeepEqual(g.Subtasks, got.Subtasks) {
		t.Fatal("today subtasks changed")
	}
}
func TestReturnFutureNormalizesHiddenCancelledChild(t *testing.T) {
	g := baseGraph()
	in := returnInput()
	in.Today = date("2026-11-04")
	s := child()
	s.Status = SubtaskCancelledDraft
	s.SettledAt = ptr(instant(100))
	s.DraftCancellationID = ptr(CancellationID(identity(33)))
	g.Subtasks = []Subtask{s}
	k, e := NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ReturnToPool(in); e != nil {
		t.Fatal(e)
	}
	got := k.Snapshot()
	sub := got.Subtasks[0]
	if sub.Status != SubtaskPending || sub.SettledAt != nil || sub.DraftCancellationID != nil || sub.UpdatedAt != in.Now || len(got.Definitions[1].PlannedSubtasks) != 0 {
		t.Fatal("hidden child normalization")
	}
}

func TestReturnRejectsRecurringEvenInsideKernel(t *testing.T) {
	for _, mode := range []string{"current", "future", "past", "locked"} {
		t.Run(mode, func(t *testing.T) {
			g := baseGraph()
			in := returnInput()
			switch mode {
			case "future":
				in.Today = date("2026-11-04")
			case "past":
				in.Today = date("2026-11-06")
			case "locked":
				g.Days[0].LockedAt = ptr(instant(100))
				g.Traces[0].Status = TraceUnfinished
				g.Traces[0].SettledAt = ptr(instant(100))
			}
			g.Chains[0].CycleMembership = &TaskCycleMembership{SeriesID: TaskCycleSeriesID(identity(90)), OccurrenceDate: g.Traces[0].Date}
			assertValid(t, g)
			if _, e := NewTaskKernel(g); !errors.Is(e, ErrUnsupportedRecurring) {
				t.Fatal("public constructor accepted recurring graph", e)
			}
			// Internal-only fixture exercises the command beyond the public constructor's
			// stricter current scope. This does not enable recurrence in the runtime.
			k := &TaskKernel{graph: g.Clone()}
			before := k.Snapshot()
			if e := k.ReturnToPool(in); !errors.Is(e, ErrInvalidTransition) {
				t.Fatal("command accepted recurring occurrence or wrong precedence", e)
			}
			if !reflect.DeepEqual(before, k.Snapshot()) {
				t.Fatal("recurring rejection mutated graph")
			}
		})
	}
}
