// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func plannedEditFixture() (TaskGraph, PlannedSubtaskEditInput) {
	g := baseGraph()
	g.Days = nil
	g.Traces = nil
	return g, PlannedSubtaskEditInput{Kind: SubtaskAdd, ChainID: g.Chains[0].ID, ExpectedDefinitionID: g.Definitions[0].ID, SubtaskID: PlannedSubtaskID(identity(70)), LineageID: SubtaskLineageID(identity(71)), Title: "  计划\r\n子项  ", Difficulty: DifficultyMedium, Now: instant(110)}
}
func dayEditFixture() (TaskGraph, SubtaskEditInput) {
	g := baseGraph()
	return g, SubtaskEditInput{Kind: SubtaskAdd, ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, SubtaskID: SubtaskID(identity(70)), LineageID: SubtaskLineageID(identity(71)), Title: "  当日\r子项  ", Difficulty: DifficultyHard, Today: g.Traces[0].Date, Now: instant(110)}
}
func TestPlannedSubtaskEditExactFacts(t *testing.T) {
	g, in := plannedEditFixture()
	k := kernel(t, g)
	if err := k.EditPlannedSubtask(in); err != nil {
		t.Fatal(err)
	}
	want := g.Clone()
	want.Definitions[0].ContentUpdatedAt = in.Now
	want.Chains[0].UpdatedAt = in.Now
	want.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: in.SubtaskID, LineageID: in.LineageID, Title: "计划\n子项", Difficulty: DifficultyMedium, Position: 1, CreatedAt: in.Now}}
	if !reflect.DeepEqual(k.Snapshot(), want) {
		t.Fatal("add changed non-source facts")
	}
	in.Kind = SubtaskTitle
	in.LineageID = ""
	in.Difficulty = 0
	in.Title = " 新名 "
	in.Now = instant(120)
	if err := k.EditPlannedSubtask(in); err != nil {
		t.Fatal(err)
	}
	want.Definitions[0].ContentUpdatedAt = in.Now
	want.Chains[0].UpdatedAt = in.Now
	want.Definitions[0].PlannedSubtasks[0].Title = "新名"
	if !reflect.DeepEqual(k.Snapshot(), want) {
		t.Fatal("title changed identity, createdAt or unrelated facts")
	}
	in.Kind = SubtaskDifficulty
	in.Title = ""
	in.Difficulty = DifficultySimple
	in.Now = instant(130)
	if err := k.EditPlannedSubtask(in); err != nil {
		t.Fatal(err)
	}
	want.Definitions[0].ContentUpdatedAt = in.Now
	want.Chains[0].UpdatedAt = in.Now
	want.Definitions[0].PlannedSubtasks[0].Difficulty = DifficultySimple
	if !reflect.DeepEqual(k.Snapshot(), want) {
		t.Fatal("difficulty changed unrelated facts")
	}
}
func TestPlannedRemoveCompactsPositionsWithoutVersioning(t *testing.T) {
	g, in := plannedEditFixture()
	k := kernel(t, g)
	for i := 0; i < 3; i++ {
		in.SubtaskID = PlannedSubtaskID(identity(70 + i))
		in.LineageID = SubtaskLineageID(identity(80 + i))
		in.Now = instant(float64(110 + i))
		if err := k.EditPlannedSubtask(in); err != nil {
			t.Fatal(err)
		}
	}
	before := k.Snapshot()
	in = PlannedSubtaskEditInput{Kind: PlannedSubtaskRemove, ChainID: in.ChainID, ExpectedDefinitionID: in.ExpectedDefinitionID, SubtaskID: PlannedSubtaskID(identity(71)), Now: instant(120)}
	if err := k.EditPlannedSubtask(in); err != nil {
		t.Fatal(err)
	}
	want := before.Clone()
	d := &want.Definitions[0]
	d.PlannedSubtasks = []PlannedSubtask{d.PlannedSubtasks[0], d.PlannedSubtasks[2]}
	d.PlannedSubtasks[1].Position = 2
	d.ContentUpdatedAt = in.Now
	want.Chains[0].UpdatedAt = in.Now
	if !reflect.DeepEqual(k.Snapshot(), want) {
		t.Fatal("remove failed exact array compaction")
	}
	unchanged(t, k, ErrNotFound, func() error { return k.EditPlannedSubtask(in) })
}
func TestPlannedNoopValidatesOwnerBeforeIgnoringClock(t *testing.T) {
	g, in := plannedEditFixture()
	k := kernel(t, g)
	if e := k.EditPlannedSubtask(in); e != nil {
		t.Fatal(e)
	}
	in.Kind = SubtaskTitle
	in.LineageID = ""
	in.Difficulty = 0
	in.Now = Instant(math.Float64bits(math.NaN()))
	before := k.Snapshot()
	if e := k.EditPlannedSubtask(in); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("no-op rewrote bytes")
	}
	in.ExpectedDefinitionID = TaskDefinitionID(identity(99))
	unchanged(t, k, ErrInvalidTransition, func() error { return k.EditPlannedSubtask(in) })
}
func TestPlannedMutationGuardsAreAtomic(t *testing.T) {
	cases := []struct {
		name  string
		alter func(*TaskGraph, *PlannedSubtaskEditInput)
		err   error
	}{
		{"wrong-definition", func(g *TaskGraph, in *PlannedSubtaskEditInput) {
			in.ExpectedDefinitionID = TaskDefinitionID(identity(99))
		}, ErrInvalidTransition},
		{"missing-chain", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.ChainID = TaskChainID(identity(99)) }, ErrNotFound},
		{"abandoned", func(g *TaskGraph, in *PlannedSubtaskEditInput) { g.Chains[0].State = ChainAbandoned }, ErrInvalidTransition},
		{"backward-clock", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.Now = instant(99) }, ErrInvalidClock},
		{"empty", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.Title = " \n " }, ErrInvalidTitle},
		{"invalid-utf8", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.Title = "\xff" }, ErrInvalidTitle},
		{"invalid-id", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.SubtaskID = "bad" }, ErrInvalidInput},
		{"invalid-difficulty", func(g *TaskGraph, in *PlannedSubtaskEditInput) { in.Difficulty = 4 }, ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, in := plannedEditFixture()
			tc.alter(&g, &in)
			k := kernel(t, g)
			unchanged(t, k, tc.err, func() error { return k.EditPlannedSubtask(in) })
		})
	}
	g, in := plannedEditFixture()
	g.Days = baseGraph().Days
	g.Traces = baseGraph().Traces
	k := kernel(t, g)
	unchanged(t, k, ErrInvalidTransition, func() error { return k.EditPlannedSubtask(in) })
}
func TestDaySubtaskAddAndEditDoNotTouchParents(t *testing.T) {
	for _, offset := range []int{0, 2} {
		t.Run(map[int]string{0: "today", 2: "future"}[offset], func(t *testing.T) {
			g, in := dayEditFixture()
			d, _ := in.Today.AddDays(offset)
			g.Days[0].Date = d
			g.Traces[0].Date = d
			k := kernel(t, g)
			if e := k.EditSubtask(in); e != nil {
				t.Fatal(e)
			}
			want := g.Clone()
			want.Subtasks = []Subtask{{ID: in.SubtaskID, LineageID: in.LineageID, TraceID: in.TraceID, Title: "当日\n子项", Status: SubtaskPending, Difficulty: DifficultyHard, Position: 1, CreatedAt: in.Now, UpdatedAt: in.Now}}
			if !reflect.DeepEqual(k.Snapshot(), want) {
				t.Fatal("add touched parent clocks or identity")
			}
			in = SubtaskEditInput{Kind: SubtaskTitle, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: in.SubtaskID, Today: in.Today, Title: "改名", Now: instant(120)}
			if e := k.EditSubtask(in); e != nil {
				t.Fatal(e)
			}
			want.Subtasks[0].Title = in.Title
			want.Subtasks[0].UpdatedAt = in.Now
			if !reflect.DeepEqual(k.Snapshot(), want) {
				t.Fatal("title touched unrelated facts")
			}
			in.Kind = SubtaskDifficulty
			in.Title = ""
			in.Difficulty = DifficultySimple
			in.Now = instant(130)
			if e := k.EditSubtask(in); e != nil {
				t.Fatal(e)
			}
			want.Subtasks[0].Difficulty = in.Difficulty
			want.Subtasks[0].UpdatedAt = in.Now
			if !reflect.DeepEqual(k.Snapshot(), want) {
				t.Fatal("difficulty touched unrelated facts")
			}
		})
	}
}
func TestDaySubtaskCompletionExplicitParentAndUndo(t *testing.T) {
	g, in := dayEditFixture()
	k := kernel(t, g)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	unchanged(t, k, ErrOpenSubtasksPreventCompletion, func() error { return k.MarkCompleted(in.TraceID, in.Today, instant(111)) })
	in = SubtaskEditInput{Kind: SubtaskComplete, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: in.SubtaskID, Today: in.Today, Now: instant(120)}
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	got := k.Snapshot()
	if got.Traces[0].Status != TracePending || got.Traces[0].ContentUpdatedAt != g.Traces[0].ContentUpdatedAt || got.Subtasks[0].CompletedAt == nil || *got.Subtasks[0].CompletedAt != in.Now {
		t.Fatal("completion cascaded or clock wrong")
	}
	unchanged(t, k, ErrInvalidTransition, func() error { return k.EditSubtask(in) })
	if e := k.MarkCompleted(in.TraceID, in.Today, instant(130)); e != nil {
		t.Fatal(e)
	}
	in.Kind = SubtaskUndoComplete
	in.Now = instant(140)
	unchanged(t, k, ErrImmutableHistory, func() error { return k.EditSubtask(in) })
	if e := k.UndoCompleted(in.TraceID, in.Today, instant(150)); e != nil {
		t.Fatal(e)
	}
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	got = k.Snapshot()
	if got.Subtasks[0].Status != SubtaskPending || got.Subtasks[0].CompletedAt != nil {
		t.Fatal("undo child facts")
	}
}
func TestDayDeleteKeepsCompletedIdentityAndPosition(t *testing.T) {
	g, in := dayEditFixture()
	k := kernel(t, g)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	in = SubtaskEditInput{Kind: SubtaskComplete, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: in.SubtaskID, Today: in.Today, Now: instant(120)}
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	// Source permits title and difficulty edits while the child is completed,
	// provided the parent remains pending.
	in.Kind = SubtaskTitle
	in.Title = "完成后仍可编辑"
	in.Now = instant(130)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	before := k.Snapshot()
	in.Kind = SubtaskDelete
	in.Title = ""
	in.CancellationID = CancellationID(identity(88))
	in.Now = instant(140)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	want := before.Clone()
	s := &want.Subtasks[0]
	s.Status = SubtaskCancelledDraft
	s.CompletedAt = nil
	s.SettledAt = ptr(in.Now)
	s.UpdatedAt = in.Now
	s.DraftCancellationID = ptr(in.CancellationID)
	if !reflect.DeepEqual(k.Snapshot(), want) {
		t.Fatal("delete lost identity/history")
	}
	in.CancellationID = CancellationID(identity(89))
	unchanged(t, k, ErrNotFound, func() error { return k.EditSubtask(in) })
	add := dayEditFixtureInput()
	add.SubtaskID = SubtaskID(identity(90))
	add.LineageID = SubtaskLineageID(identity(91))
	add.Now = instant(150)
	if e := k.EditSubtask(add); e != nil {
		t.Fatal(e)
	}
	if k.Snapshot().Subtasks[1].Position != 2 {
		t.Fatal("tombstone not counted")
	}
}
func dayEditFixtureInput() SubtaskEditInput { _, in := dayEditFixture(); return in }
func TestDaySubtaskDatesOwnersAndClocks(t *testing.T) {
	for _, kind := range []SubtaskEditKind{SubtaskAdd, SubtaskTitle, SubtaskDifficulty, SubtaskDelete, SubtaskComplete, SubtaskUndoComplete, SubtaskAbandon} {
		for _, mode := range []string{"past", "locked", "parent-completed", "future", "wrong-chain", "wrong-trace", "backwards"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				g, in := dayEditFixture()
				in.Kind = kind
				in.Now = instant(110)
				if kind != SubtaskAdd {
					g.Subtasks = []Subtask{child()}
					in.SubtaskID = g.Subtasks[0].ID
					in.LineageID = ""
					in.Title = ""
					in.Difficulty = 0
				}
				switch kind {
				case SubtaskTitle:
					in.Title = "新名"
				case SubtaskDifficulty:
					in.Difficulty = DifficultyHard
				case SubtaskDelete:
					in.CancellationID = CancellationID(identity(99))
				case SubtaskUndoComplete:
					g.Subtasks[0].Status = SubtaskCompleted
					g.Subtasks[0].CompletedAt = ptr(instant(100))
				}
				wantErr := ErrImmutableHistory
				switch mode {
				case "past":
					in.Today, _ = in.Today.AddDays(1)
				case "locked":
					g.Days[0].LockedAt = ptr(instant(100))
				case "parent-completed":
					if kind == SubtaskAdd {
						wantErr = ErrInvalidTransition
					}
					g.Traces[0].Status = TraceCompleted
					g.Traces[0].CompletedAt = ptr(instant(100))
				case "future":
					g.Days[0].Date, _ = in.Today.AddDays(1)
					g.Traces[0].Date = g.Days[0].Date
					if kind == SubtaskAdd || kind == SubtaskTitle || kind == SubtaskDifficulty || kind == SubtaskDelete {
						wantErr = nil
					}
				case "wrong-chain":
					in.ChainID = TaskChainID(identity(95))
					wantErr = ErrInvalidInput
				case "wrong-trace":
					in.TraceID = DayTraceID(identity(95))
					wantErr = ErrNotFound
				case "backwards":
					in.Now = instant(99)
					wantErr = ErrInvalidClock
					if kind == SubtaskAdd {
						wantErr = nil
					} // Source low-level add only requires finite own clock; service allocates whole-graph frontier.
				}
				k := kernel(t, g)
				if wantErr != nil {
					unchanged(t, k, wantErr, func() error { return k.EditSubtask(in) })
				} else if e := k.EditSubtask(in); e != nil {
					t.Fatal(e)
				}
			})
		}
	}
}
func TestSubtaskEditFreshIdentitiesAndNoopBytes(t *testing.T) {
	g, in := dayEditFixture()
	k := kernel(t, g)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	unchanged(t, k, ErrInvalidInput, func() error { return k.EditSubtask(in) })
	in.SubtaskID = SubtaskID(identity(77))
	unchanged(t, k, ErrInvalidInput, func() error { return k.EditSubtask(in) }) // lineage reuse
	in = SubtaskEditInput{Kind: SubtaskTitle, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: SubtaskID(identity(70)), Today: in.Today, Title: "当日\n子项", Now: Instant(math.Float64bits(math.NaN()))}
	before := k.Snapshot()
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("noop bytes")
	}
	// Literal CGJ remains content, even beyond stream-safe normalization length.
	g = k.Snapshot()
	g.Subtasks[0].Title = strings.Repeat("\u0301", 31)
	k = kernel(t, g)
	in.Title = strings.Repeat("\u0301", 30) + "\u034f\u0301"
	in.Now = instant(150)
	if e := k.EditSubtask(in); e != nil {
		t.Fatal(e)
	}
	if k.Snapshot().Subtasks[0].Title != in.Title {
		t.Fatal("explicit CGJ lost")
	}
}
