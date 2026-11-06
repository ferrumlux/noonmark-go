// SPDX-License-Identifier: AGPL-3.0-only
package core

import "testing"

func TestProgressExistingCommandsDriveCompletionAndCancellation(t *testing.T) {
	// Official weighted / parent completion / SnapshotUndoCancellation cases,
	// exercised with this port's existing commands rather than adding commands.
	k := kernel(t, TaskGraph{})
	chain, def, trace := TaskChainID(identity(501)), TaskDefinitionID(identity(502)), DayTraceID(identity(503))
	today := date("2026-11-05")
	if e := k.CreatePoolTask(CreatePoolTaskInput{ChainID: chain, DefinitionID: def, Title: "合成进度流程", Now: instant(100)}); e != nil {
		t.Fatal(e)
	}
	if e := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: DayID(identity(504)), Date: today, Today: today, Now: instant(101)}); e != nil {
		t.Fatal(e)
	}
	for i, w := range []Difficulty{DifficultySimple, DifficultyHard} {
		if e := k.EditSubtask(SubtaskEditInput{Kind: SubtaskAdd, ChainID: chain, TraceID: trace, SubtaskID: SubtaskID(identity(510 + i)), LineageID: SubtaskLineageID(identity(520 + i)), Title: "合成步骤", Difficulty: w, Today: today, Now: instant(102 + float64(i))}); e != nil {
			t.Fatal(e)
		}
	}
	first := SubtaskEditInput{Kind: SubtaskComplete, ChainID: chain, TraceID: trace, SubtaskID: SubtaskID(identity(510)), Today: today, Now: instant(104)}
	if e := k.EditSubtask(first); e != nil {
		t.Fatal(e)
	}
	p := progressView(t, k.Snapshot())
	wantProgress(t, p, trace, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	wantCompletion(t, p, trace, today, DayTraceCompletionCapability{Blocker: CompletionOpenSubtasks})
	if e := k.MarkCompleted(trace, today, instant(105)); e != ErrOpenSubtasksPreventCompletion {
		t.Fatal(e)
	}
	second := SubtaskEditInput{Kind: SubtaskAbandon, ChainID: chain, TraceID: trace, SubtaskID: SubtaskID(identity(511)), Today: today, Now: instant(106)}
	if e := k.EditSubtask(second); e != nil {
		t.Fatal(e)
	}
	p = progressView(t, k.Snapshot())
	wantCompletion(t, p, trace, today, DayTraceCompletionCapability{Action: CompletionComplete})
	if e := k.MarkCompleted(trace, today, instant(107)); e != nil {
		t.Fatal(e)
	}
	p = progressView(t, k.Snapshot())
	wantProgress(t, p, trace, TraceProgress{Mode: ProgressManual, Percent: 100, CompletedWeight: 1, TotalWeight: 1})
	wantCompletion(t, p, trace, today, DayTraceCompletionCapability{Action: CompletionUndo})
	if e := k.UndoCompleted(trace, today, instant(108)); e != nil {
		t.Fatal(e)
	}
	p = progressView(t, k.Snapshot())
	wantProgress(t, p, trace, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 25, CompletedWeight: 1, TotalWeight: 4})
	for i := 0; i < 2; i++ {
		in := SubtaskEditInput{Kind: SubtaskDelete, ChainID: chain, TraceID: trace, SubtaskID: SubtaskID(identity(510 + i)), CancellationID: CancellationID(identity(530 + i)), Today: today, Now: instant(109 + float64(i))}
		if e := k.EditSubtask(in); e != nil {
			t.Fatal(e)
		}
	}
	p = progressView(t, k.Snapshot())
	wantProgress(t, p, trace, TraceProgress{Mode: ProgressManual, TotalWeight: 100})
	wantManual(t, p, trace, today, true)
	if got := p.SubtaskProgress(trace); got != (SubtaskProgress{}) {
		t.Fatal(got)
	}
}

func TestProgressReturnAndRescheduleUsesSameLineageAcrossDefinitions(t *testing.T) {
	g := baseGraph()
	g.Subtasks = []Subtask{child()}
	k := kernel(t, g)
	old := g.Traces[0]
	s := g.Subtasks[0]
	in := returnInput()
	in.PlannedIDs = map[SubtaskID]PlannedSubtaskID{s.ID: PlannedSubtaskID(identity(22))}
	if e := k.ReturnToPool(in); e != nil {
		t.Fatal(e)
	}
	next := DayTraceID(identity(30))
	if e := k.ScheduleFromPool(ScheduleFromPoolInput{ChainID: old.ChainID, TraceID: next, DayID: DayID(identity(31)), Date: old.Date, Today: old.Date, Now: instant(300), SubtaskIDs: map[PlannedSubtaskID]SubtaskID{PlannedSubtaskID(identity(22)): SubtaskID(identity(32))}}); e != nil {
		t.Fatal(e)
	}
	edit := SubtaskEditInput{Kind: SubtaskDifficulty, ChainID: old.ChainID, TraceID: next, SubtaskID: SubtaskID(identity(32)), Difficulty: DifficultySimple, Today: old.Date, Now: instant(301)}
	if e := k.EditSubtask(edit); e != nil {
		t.Fatal(e)
	}
	edit.Kind = SubtaskComplete
	edit.Difficulty = 0
	edit.Now = instant(302)
	if e := k.EditSubtask(edit); e != nil {
		t.Fatal(e)
	}
	p := progressView(t, k.Snapshot())
	// Both same-day traces see the completion, even the movement-source trace;
	// the historical medium weight wins over current simple. No carry link needed.
	for _, id := range []DayTraceID{old.ID, next} {
		wantProgress(t, p, id, TraceProgress{Mode: ProgressWeightedSubtasks, Percent: 100, CompletedWeight: 2, TotalWeight: 2})
	}
	wantCompletion(t, p, next, old.Date, DayTraceCompletionCapability{Action: CompletionComplete})
}
