// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"math"
	"reflect"
	"testing"
)

func carryoverSeed(t *testing.T, children bool) (*TaskKernel, TraceCarryoverInput) {
	t.Helper()
	k := kernel(t, TaskGraph{})
	in := TraceCarryoverInput{ChainID: TaskChainID(identity(800)), TraceID: DayTraceID(identity(802)), NewTraceID: DayTraceID(identity(803)), DayID: DayID(identity(804)), Today: date("2026-11-05"), TargetDate: date("2026-11-06"), Now: instant(110)}
	desc := "来源描述"
	note := TaskNoteEntry{ID: TaskNoteEntryID(identity(805)), Body: "有效附言", CreatedAt: instant(100), UpdatedAt: instant(100)}
	if err := k.CreatePoolTask(CreatePoolTaskInput{ChainID: in.ChainID, DefinitionID: TaskDefinitionID(identity(801)), Title: "合成任务", DescriptionText: &desc, InitialNote: &note, Now: instant(100)}); err != nil {
		t.Fatal(err)
	}
	if err := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, DayID: DayID(identity(806)), Date: in.Today, Today: in.Today, Now: instant(101)}); err != nil {
		t.Fatal(err)
	}
	if children {
		for i := 0; i < 4; i++ {
			if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: SubtaskID(identity(810 + i)), LineageID: SubtaskLineageID(identity(820 + i)), Title: "步骤", Difficulty: Difficulty(i%3 + 1), Today: in.Today, Now: instant(102)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskComplete, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: SubtaskID(identity(810)), Today: in.Today, Now: instant(103)}); err != nil {
			t.Fatal(err)
		}
		if err := k.EditSubtask(SubtaskEditInput{Kind: SubtaskDelete, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: SubtaskID(identity(812)), CancellationID: CancellationID(identity(830)), Today: in.Today, Now: instant(104)}); err != nil {
			t.Fatal(err)
		}
		in.SubtaskIDs = map[SubtaskID]SubtaskID{SubtaskID(identity(811)): SubtaskID(identity(831)), SubtaskID(identity(813)): SubtaskID(identity(833))}
	}
	return k, in
}
func carryoverRun(k *TaskKernel, in TraceCarryoverInput, continuation bool) error {
	if continuation {
		return k.ContinueUnfinishedTrace(in)
	}
	return k.DeferCurrentTrace(in)
}
func carryoverSettle(t *testing.T, k *TaskKernel, in *TraceCarryoverInput) {
	t.Helper()
	in.Today = in.TargetDate
	if err := k.SettleTaskDays(in.Today, instant(105)); err != nil {
		t.Fatal(err)
	}
}
func carryoverReject(t *testing.T, k *TaskKernel, in TraceCarryoverInput, c bool, want error) {
	t.Helper()
	before := k.Snapshot()
	err := carryoverRun(k, in, c)
	if err == nil || (want != nil && err != want) {
		t.Fatalf("got %v want %v", err, want)
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("failed command changed graph")
	}
}

func TestCarryoverCommandBuiltOpenChildrenAndHistory(t *testing.T) {
	for _, c := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[c], func(t *testing.T) {
			k, in := carryoverSeed(t, true)
			if c {
				carryoverSettle(t, k, &in)
			}
			before := k.Snapshot()
			wantPercent := progressView(t, before).TraceProgress(in.TraceID).Percent
			if err := carryoverRun(k, in, c); err != nil {
				t.Fatal(err)
			}
			after := k.Snapshot()
			next := after.Traces[1]
			if next.ID != in.NewTraceID || next.Status != TracePending || next.CarriedFromTraceID == nil || *next.CarriedFromTraceID != in.TraceID || next.PinOrder != nil || next.ManualProgressPercent == nil || *next.ManualProgressPercent != wantPercent || next.ContinuationSeq != map[bool]int{false: 0, true: 1}[c] {
				t.Fatal(next)
			}
			if !reflect.DeepEqual(next.NoteEntries, before.Traces[0].NoteEntries) || !reflect.DeepEqual(next.DescriptionText, before.Traces[0].DescriptionText) || !reflect.DeepEqual(before.Definitions, after.Definitions) {
				t.Fatal("content not copied/preserved")
			}
			if c {
				if !reflect.DeepEqual(before.Traces[0], after.Traces[0]) || !reflect.DeepEqual(before.Subtasks, after.Subtasks[:4]) || !reflect.DeepEqual(before.Days, after.Days[:1]) {
					t.Fatal("history changed")
				}
			} else if after.Traces[0].Status != TraceDeferred || !after.Traces[0].SettledAt.Equal(in.Now) {
				t.Fatal(after.Traces[0])
			}
			for i, sourceIndex := range []int{1, 3} {
				old, s := before.Subtasks[sourceIndex], after.Subtasks[4+i]
				if s.ID != in.SubtaskIDs[old.ID] || s.LineageID != old.LineageID || s.Title != old.Title || s.Difficulty != old.Difficulty || s.Position != i+1 || s.Status != SubtaskPending || s.CarriedFromSubtaskID == nil || *s.CarriedFromSubtaskID != old.ID {
					t.Fatal(s)
				}
				if !c && after.Subtasks[sourceIndex].Status != SubtaskDeferred {
					t.Fatal("source child not deferred")
				}
			}
			if !reflect.DeepEqual(before.Subtasks[0], after.Subtasks[0]) || !reflect.DeepEqual(before.Subtasks[2], after.Subtasks[2]) {
				t.Fatal("completed/deleted child changed")
			}
			if after.Chains[0].UpdatedAt != in.Now || after.Days[1].CreatedAt != in.Now || after.Days[1].UpdatedAt != in.Now {
				t.Fatal("clocks")
			}
			if progressView(t, after).TraceProgress(next.ID).Percent < wantPercent {
				t.Fatal("progress floor decreased")
			}
			carryoverReject(t, k, in, c, ErrInvalidTransition)
		})
	}
}

func TestCarryoverInvalidIdentitiesAndClocksAreAtomic(t *testing.T) {
	for _, c := range []bool{false, true} {
		cases := []struct {
			name string
			edit func(*TraceCarryoverInput)
		}{
			{"missing trace", func(i *TraceCarryoverInput) { i.TraceID = DayTraceID(identity(999)) }},
			{"owner", func(i *TraceCarryoverInput) { i.ChainID = TaskChainID(identity(999)) }},
			{"new trace missing", func(i *TraceCarryoverInput) { i.NewTraceID = "" }},
			{"new trace reused", func(i *TraceCarryoverInput) { i.NewTraceID = i.TraceID }},
			{"day missing", func(i *TraceCarryoverInput) { i.DayID = "" }},
			{"day reused", func(i *TraceCarryoverInput) { i.DayID = DayID(identity(806)) }},
			{"child map missing", func(i *TraceCarryoverInput) { i.SubtaskIDs = nil }},
			{"child key wrong", func(i *TraceCarryoverInput) {
				delete(i.SubtaskIDs, SubtaskID(identity(811)))
				i.SubtaskIDs[SubtaskID(identity(999))] = SubtaskID(identity(831))
			}},
			{"child id reused", func(i *TraceCarryoverInput) { i.SubtaskIDs[SubtaskID(identity(811))] = SubtaskID(identity(810)) }},
			{"child id duplicate", func(i *TraceCarryoverInput) { i.SubtaskIDs[SubtaskID(identity(811))] = SubtaskID(identity(833)) }},
			{"extra child", func(i *TraceCarryoverInput) { i.SubtaskIDs[SubtaskID(identity(999))] = SubtaskID(identity(998)) }},
			{"invalid today", func(i *TraceCarryoverInput) { i.Today = LocalDate{} }},
			{"invalid target", func(i *TraceCarryoverInput) { i.TargetDate = LocalDate{} }},
			{"nan", func(i *TraceCarryoverInput) { i.Now = Instant(math.Float64bits(math.NaN())) }},
			{"inf", func(i *TraceCarryoverInput) { i.Now = Instant(math.Float64bits(math.Inf(1))) }},
			{"predates source creation", func(i *TraceCarryoverInput) { i.Now = instant(99) }},
		}
		for _, tc := range cases {
			t.Run(map[bool]string{false: "defer/", true: "continue/"}[c]+tc.name, func(t *testing.T) {
				k, in := carryoverSeed(t, true)
				if c {
					carryoverSettle(t, k, &in)
				}
				tc.edit(&in)
				carryoverReject(t, k, in, c, nil)
			})
		}
	}
}

func TestCarryoverDateAndStateGuards(t *testing.T) {
	for _, c := range []bool{false, true} {
		for _, target := range []string{"2026-11-04", "2026-11-05"} {
			t.Run(map[bool]string{false: "defer/", true: "continue/"}[c]+target, func(t *testing.T) {
				k, in := carryoverSeed(t, false)
				if c {
					carryoverSettle(t, k, &in)
				}
				in.TargetDate = date(target)
				want := ErrInvalidTransition
				if c && target == "2026-11-05" {
					want = ErrImmutableHistory
				}
				carryoverReject(t, k, in, c, want)
			})
		}
	}
	k, in := carryoverSeed(t, false)
	carryoverReject(t, k, in, true, ErrInvalidTransition)
	in.Today = date("2026-11-04")
	carryoverReject(t, k, in, false, ErrInvalidTransition)
	in.Today = date("2026-11-06")
	carryoverReject(t, k, in, false, ErrInvalidTransition)
	k, in = carryoverSeed(t, false)
	if err := k.MarkCompleted(in.TraceID, in.Today, in.Now); err != nil {
		t.Fatal(err)
	}
	carryoverReject(t, k, in, false, ErrInvalidTransition)
}

func TestCarryoverManualFloorThroughMultipleDays(t *testing.T) {
	k, in := carryoverSeed(t, false)
	if err := k.SetManualProgress(SetManualProgressInput{ChainID: in.ChainID, TraceID: in.TraceID, Percent: 67, Today: in.Today, Now: instant(104)}); err != nil {
		t.Fatal(err)
	}
	if err := k.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
	in.TraceID = in.NewTraceID
	in.NewTraceID = DayTraceID(identity(850))
	in.DayID = DayID(identity(851))
	in.Today = date("2026-11-07")
	in.TargetDate = in.Today
	in.Now = instant(120)
	if err := k.SettleTaskDays(in.Today, instant(115)); err != nil {
		t.Fatal(err)
	}
	before := k.Snapshot()
	if err := k.ContinueUnfinishedTrace(in); err != nil {
		t.Fatal(err)
	}
	after := k.Snapshot()
	if *after.Traces[2].ManualProgressPercent != 67 || after.Traces[2].ContinuationSeq != 1 || !reflect.DeepEqual(before.Traces, after.Traces[:2]) {
		t.Fatal(after.Traces)
	}
	if err := k.SetManualProgress(SetManualProgressInput{ChainID: in.ChainID, TraceID: in.NewTraceID, Percent: 1, Today: in.Today, Now: instant(121)}); err != nil {
		t.Fatal(err)
	}
	if *k.Snapshot().Traces[2].ManualProgressPercent != 67 {
		t.Fatal("floor lowered")
	}
}

func TestCarryoverReturnRescheduleLineageAndFrontier(t *testing.T) {
	for _, c := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[c], func(t *testing.T) {
			k, in := carryoverSeed(t, true)
			if c {
				carryoverSettle(t, k, &in)
				in.TargetDate = date("2026-11-07")
			}
			if err := carryoverRun(k, in, c); err != nil {
				t.Fatal(err)
			}
			source := k.Snapshot().Traces[0]
			planned := map[SubtaskID]PlannedSubtaskID{SubtaskID(identity(831)): PlannedSubtaskID(identity(861)), SubtaskID(identity(833)): PlannedSubtaskID(identity(863))}
			if err := k.ReturnToPool(ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.NewTraceID, NewDefinitionID: TaskDefinitionID(identity(864)), CancellationID: CancellationID(identity(865)), PlannedIDs: planned, Today: in.Today, Now: instant(120)}); err != nil {
				t.Fatal(err)
			}
			carryoverReject(t, k, in, c, ErrInvalidTransition)
			current := DayTraceID(identity(870))
			if err := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: in.ChainID, TraceID: current, DayID: DayID(identity(871)), Date: in.Today, Today: in.Today, Now: instant(121), SubtaskIDs: map[PlannedSubtaskID]SubtaskID{PlannedSubtaskID(identity(861)): SubtaskID(identity(872)), PlannedSubtaskID(identity(863)): SubtaskID(identity(873))}}); err != nil {
				t.Fatal(err)
			}
			in.TraceID = current
			in.NewTraceID = DayTraceID(identity(874))
			in.DayID = DayID(identity(875))
			in.TargetDate = date("2026-11-08")
			in.Now = instant(122)
			in.SubtaskIDs = map[SubtaskID]SubtaskID{SubtaskID(identity(872)): SubtaskID(identity(876)), SubtaskID(identity(873)): SubtaskID(identity(877))}
			if err := k.DeferCurrentTrace(in); err != nil {
				t.Fatal(err)
			}
			g := k.Snapshot()
			last := g.Traces[len(g.Traces)-1]
			if last.DefinitionID != TaskDefinitionID(identity(864)) || !reflect.DeepEqual(source, g.Traces[0]) || g.Subtasks[len(g.Subtasks)-1].LineageID != SubtaskLineageID(identity(823)) {
				t.Fatal("return/reschedule lineage or definition lost")
			}
		})
	}
}

// These structural fixtures exercise source-valid states for which this kernel
// has no note/pin/abandon/undo command. Every fixture passes the public validator.
func TestCarryoverStructuralSnapshotContentAndClockBoundaries(t *testing.T) {
	for _, c := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[c], func(t *testing.T) {
			original, in := carryoverSeed(t, false)
			if c {
				carryoverSettle(t, original, &in)
			}
			g := original.Snapshot()
			tomb := cloneNote(g.Traces[0].NoteEntries[0])
			tomb.ID = TaskNoteEntryID(identity(890))
			tomb.Body = ""
			tomb.DeletedAt = clonePtr(&tomb.UpdatedAt)
			g.Traces[0].NoteEntries = append(g.Traces[0].NoteEntries, tomb)
			empty := ""
			g.Traces[0].DescriptionText = &empty
			g.Chains[0].UpdatedAt = instant(999)
			if !c {
				pin := 3
				g.Traces[0].PinOrder = &pin
			}
			// Existing day with a later clock is never touched by ensureDay.
			g.Days = append(g.Days, Day{ID: DayID(identity(891)), Date: in.TargetDate, CreatedAt: instant(500), UpdatedAt: instant(600)})
			k := kernel(t, g)
			in.DayID = ""
			in.Now = instant(105)
			if err := carryoverRun(k, in, c); err != nil {
				t.Fatal(err)
			}
			out := k.Snapshot()
			next := out.Traces[1]
			if next.DescriptionText == nil || *next.DescriptionText != "" || len(next.NoteEntries) != 1 || next.NoteEntries[0].ID != g.Traces[0].NoteEntries[0].ID || next.PinOrder != nil || out.Traces[0].PinOrder != nil || !reflect.DeepEqual(g.Days, out.Days) || !reflect.DeepEqual(g.Chains, out.Chains) {
				t.Fatal("snapshot/pin/chain/day mismatch")
			}
			// Mutating the returned snapshot must not mutate kernel-held copied notes.
			out.Traces[1].NoteEntries[0].Body = "外部编辑"
			if k.Snapshot().Traces[1].NoteEntries[0].Body == "外部编辑" {
				t.Fatal("aliased snapshot")
			}
		})
	}
	for _, c := range []bool{false, true} {
		original, in := carryoverSeed(t, true)
		if c {
			carryoverSettle(t, original, &in)
		}
		g := original.Snapshot()
		for i := range g.Chains {
			g.Chains[i].CreatedAt = instant(-100)
			g.Chains[i].UpdatedAt = instant(-10)
			for j := range g.Chains[i].NoteEntries {
				g.Chains[i].NoteEntries[j].CreatedAt = instant(-100)
				g.Chains[i].NoteEntries[j].UpdatedAt = instant(-100)
			}
		}
		for i := range g.Definitions {
			g.Definitions[i].CreatedAt = instant(-100)
			g.Definitions[i].ContentUpdatedAt = instant(-100)
		}
		for i := range g.Traces {
			g.Traces[i].CreatedAt = instant(-90)
			g.Traces[i].ContentUpdatedAt = instant(-10)
			if g.Traces[i].SettledAt != nil {
				g.Traces[i].SettledAt = ptr(instant(-10))
			}
			for j := range g.Traces[i].NoteEntries {
				g.Traces[i].NoteEntries[j].CreatedAt = instant(-100)
				g.Traces[i].NoteEntries[j].UpdatedAt = instant(-100)
			}
		}
		for i := range g.Days {
			g.Days[i].CreatedAt = instant(-100)
			g.Days[i].UpdatedAt = instant(-10)
			if g.Days[i].LockedAt != nil {
				g.Days[i].LockedAt = ptr(instant(-10))
			}
		}
		for i := range g.Subtasks {
			g.Subtasks[i].CreatedAt = instant(-80)
			g.Subtasks[i].UpdatedAt = instant(-10)
			if g.Subtasks[i].SettledAt != nil {
				g.Subtasks[i].SettledAt = ptr(instant(-10))
			}
			if g.Subtasks[i].CompletedAt != nil {
				g.Subtasks[i].CompletedAt = ptr(instant(-10))
			}
		}
		k := kernel(t, g)
		in.Now = instant(-10)
		if err := carryoverRun(k, in, c); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCarryoverStructuralGuardPrecedenceAndAtomicFailure(t *testing.T) {
	cases := []struct {
		name string
		c    bool
		edit func(*TaskGraph, *TraceCarryoverInput)
		want error
	}{
		{"abandoned precedes locked", false, func(g *TaskGraph, i *TraceCarryoverInput) {
			g.Chains[0].State = ChainAbandoned
			g.Days[0].LockedAt = ptr(instant(101))
		}, ErrChainAbandoned},
		{"locked source precedes status", false, func(g *TaskGraph, i *TraceCarryoverInput) {
			g.Days[0].LockedAt = ptr(instant(101))
			i.Today = date("2026-11-04")
		}, ErrImmutableHistory},
		{"locked target precedes status", true, func(g *TaskGraph, i *TraceCarryoverInput) { i.TargetDate = g.Days[0].Date }, ErrImmutableHistory},
		{"deferral child clock after source", false, func(g *TaskGraph, i *TraceCarryoverInput) { g.Subtasks[1].UpdatedAt = instant(111) }, ErrInvalidClock},
		{"source clock", false, func(g *TaskGraph, i *TraceCarryoverInput) { g.Traces[0].ContentUpdatedAt = instant(111) }, ErrInvalidClock},
		{"target priority overflow", false, func(g *TaskGraph, i *TraceCarryoverInput) {
			g.Traces = append(g.Traces, DayTrace{ID: DayTraceID(identity(900)), ChainID: g.Chains[0].ID, DefinitionID: g.Definitions[0].ID, Date: i.TargetDate, Status: TraceReturnedToPool, Priority: int(^uint(0) >> 1), CreatedAt: instant(100), ContentUpdatedAt: instant(100), SettledAt: ptr(instant(100))})
			g.Days = append(g.Days, Day{ID: DayID(identity(901)), Date: i.TargetDate, CreatedAt: instant(100), UpdatedAt: instant(100)})
		}, ErrInvalidTransition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, in := carryoverSeed(t, true)
			if tc.c {
				carryoverSettle(t, k, &in)
			}
			g := k.Snapshot()
			tc.edit(&g, &in)
			k = kernel(t, g)
			carryoverReject(t, k, in, tc.c, tc.want)
		})
	}
}

func TestCarryoverStructuralCurrentDefinitionAndPriority(t *testing.T) {
	k, in := carryoverSeed(t, false)
	g := k.Snapshot()
	// Source can refer to a superseded revision after a snapshot undo. Do not
	// silently copy its definition instead of Engine.currentDefinition.
	newer := g.Definitions[0]
	newer.ID = TaskDefinitionID(identity(910))
	newer.Sequence = 2
	newer.Title = "当前版本"
	newer.CreatedAt = instant(102)
	newer.ContentUpdatedAt = instant(102)
	g.Definitions[0].ContentUpdatedAt = instant(102)
	g.Definitions[0].SupersededAt = ptr(instant(102))
	g.Definitions[0].SupersededByDefinitionID = ptr(newer.ID)
	g.Definitions = append(g.Definitions, newer)
	// Another chain's moved history still occupies its target-day priority slot.
	other := TaskChain{ID: TaskChainID(identity(911)), State: ChainActive, CreatedAt: instant(100), UpdatedAt: instant(100)}
	g.Chains = append(g.Chains, other)
	def := TaskDefinition{ID: TaskDefinitionID(identity(912)), ChainID: other.ID, Sequence: 1, Title: "其他", CreatedAt: instant(100), ContentUpdatedAt: instant(100)}
	g.Definitions = append(g.Definitions, def)
	g.Days = append(g.Days, Day{ID: in.DayID, Date: in.TargetDate, CreatedAt: instant(100), UpdatedAt: instant(100)})
	g.Traces = append(g.Traces, DayTrace{ID: DayTraceID(identity(913)), ChainID: other.ID, DefinitionID: def.ID, Date: in.TargetDate, Status: TraceReturnedToPool, Priority: -2, CreatedAt: instant(100), ContentUpdatedAt: instant(100), SettledAt: ptr(instant(100))})
	k = kernel(t, g)
	if err := k.DeferCurrentTrace(in); err != nil {
		t.Fatal(err)
	}
	next := k.Snapshot().Traces[2]
	if next.DefinitionID != newer.ID || next.Priority != -1 {
		t.Fatal(next)
	}
	g.Traces[1].Priority = int(^uint(0) >> 1)
	k = kernel(t, g)
	carryoverReject(t, k, in, false, ErrInvalidInput)
}

func TestCarryoverChronologyIsSourceLexicographic(t *testing.T) {
	a := DayTrace{ID: DayTraceID(identity(1)), Date: date("2026-11-05"), CreatedAt: instant(-1)}
	cases := []DayTrace{a, a, a, a, a}
	cases[0].Date = date("2026-11-06")
	cases[0].ContinuationSeq = -1
	cases[1].ContinuationSeq = 1
	cases[1].Priority = -1
	cases[2].Priority = 1
	cases[2].CreatedAt = instant(-2)
	cases[3].CreatedAt = instant(0)
	cases[3].ID = DayTraceID(identity(0))
	cases[4].ID = DayTraceID(identity(2))
	for _, b := range cases {
		if !carryoverTraceEarlier(a, b) || carryoverTraceEarlier(b, a) || carryoverTraceEarlier(a, a) {
			t.Fatal(a, b)
		}
	}
}

func TestCarryoverCommandBuiltNonFrontierWithoutPending(t *testing.T) {
	k, in := carryoverSeed(t, false)
	carryoverSettle(t, k, &in)
	if err := k.ContinueUnfinishedTrace(in); err != nil {
		t.Fatal(err)
	}
	in.Today = date("2026-11-07")
	in.TargetDate = in.Today
	in.Now = instant(120)
	if err := k.SettleTaskDays(in.Today, instant(115)); err != nil {
		t.Fatal(err)
	}
	carryoverReject(t, k, in, true, ErrInvalidTransition)
}

// Structural-only snapshot states: this kernel does not expose snapshot undo
// or direct day locking. Constructor validation remains mandatory.
func TestCarryoverStructuralContinuationGuards(t *testing.T) {
	for _, name := range []string{"pending before frontier", "pool clock before frontier", "locked current day"} {
		t.Run(name, func(t *testing.T) {
			k, in := carryoverSeed(t, false)
			carryoverSettle(t, k, &in)
			g := k.Snapshot()
			want := ErrInvalidTransition
			switch name {
			case "pending before frontier", "pool clock before frontier":
				older := g.Traces[0]
				older.ID = DayTraceID(identity(950))
				older.Date = date("2026-11-04")
				older.Priority = 1
				older.ContentUpdatedAt = instant(109)
				older.Status = TraceReturnedToPool
				if name == "pending before frontier" {
					older.Status = TracePending
					older.SettledAt = nil
					want = ErrActiveTraceAlreadyExists
				}
				g.Traces = append(g.Traces, older)
				g.Days = append(g.Days, Day{ID: DayID(identity(951)), Date: older.Date, CreatedAt: instant(100), UpdatedAt: instant(100)})
			case "locked current day":
				in.Today = g.Traces[0].Date
			}
			k = kernel(t, g)
			if name == "locked current day" {
				if err := k.ContinueUnfinishedTrace(in); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(g.Traces[0], k.Snapshot().Traces[0]) {
					t.Fatal("locked source changed")
				}
			} else {
				carryoverReject(t, k, in, true, want)
			}
		})
	}
}

func TestCarryoverDeepCloneIsolation(t *testing.T) {
	for _, c := range []bool{false, true} {
		k, in := carryoverSeed(t, true)
		if c {
			carryoverSettle(t, k, &in)
		}
		if err := carryoverRun(k, in, c); err != nil {
			t.Fatal(err)
		}
		before := k.Snapshot()
		out := k.Snapshot()
		*out.Traces[1].DescriptionText = "changed"
		*out.Traces[1].ManualProgressPercent = 99
		*out.Traces[1].CarriedFromTraceID = DayTraceID(identity(999))
		out.Traces[1].NoteEntries[0].Body = "changed"
		*out.Subtasks[4].CarriedFromSubtaskID = SubtaskID(identity(999))
		*out.Traces[0].SettledAt = instant(999)
		*out.Subtasks[1].SettledAt = instant(999)
		if !reflect.DeepEqual(before, k.Snapshot()) {
			t.Fatal("snapshot aliases kernel")
		}
		// Separate source and target content storage inside the published candidate.
		if k.graph.Traces[0].DescriptionText == k.graph.Traces[1].DescriptionText || &k.graph.Traces[0].NoteEntries[0] == &k.graph.Traces[1].NoteEntries[0] {
			t.Fatal("target aliases source")
		}
	}
}
