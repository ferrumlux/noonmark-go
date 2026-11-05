// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
)

// Synthetic fixtures, not a Swift oracle. Source-derived counterparts:
// NoonmarkEngineTests.swift:74–250 (scopes, continuation, clearing, pool notes).
func titleInput() EditTaskTitleInput {
	return EditTaskTitleInput{ChainID: TaskChainID(identity(2)), Title: "新标题", Today: date("2026-11-05"), Now: instant(110), NewDefinitionID: TaskDefinitionID(identity(50))}
}
func titleCommand(k *TaskKernel, reuse bool, in EditTaskTitleInput) error {
	if reuse {
		return k.SaveTaskTitleInput(in)
	}
	return k.RenameTaskTitle(in)
}
func titleRichGraph() TaskGraph {
	g := baseGraph()
	g.Definitions[0].DescriptionText = ptr("定义描述")
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: PlannedSubtaskID(identity(31)), LineageID: SubtaskLineageID(identity(32)), Title: "计划", Difficulty: DifficultyHard, Position: 1, CreatedAt: instant(100)}}
	g.Traces[0].DescriptionText = ptr("当日描述")
	g.Traces[0].PinOrder = ptr(2)
	g.Traces[0].ManualProgressPercent = ptr(30)
	g.Traces[0].NoteEntries = []TaskNoteEntry{{ID: TaskNoteEntryID(identity(33)), Body: "当日附言", CreatedAt: instant(100), UpdatedAt: instant(100)}}
	g.Chains[0].NoteEntries = cloneNotes(g.Traces[0].NoteEntries)
	g.Subtasks = []Subtask{child()}
	return g
}
func TestTitlePoolBothCommandsReuseWithoutIdentity(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := baseGraph()
		g.Traces = nil
		g.Days = nil
		k := kernel(t, g)
		in := titleInput()
		in.NewDefinitionID = "invalid-unused"
		if err := titleCommand(k, reuse, in); err != nil {
			t.Fatal(err)
		}
		want := g.Clone()
		want.Definitions[0].Title = in.Title
		want.Definitions[0].ContentUpdatedAt = in.Now
		want.Chains[0].UpdatedAt = in.Now
		if !reflect.DeepEqual(want, k.Snapshot()) {
			t.Fatal("pool edit changed unrelated facts")
		}
	}
}
func TestTitlePendingRenameVersusInputExactDelta(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		for _, future := range []bool{false, true} {
			g := titleRichGraph()
			if future {
				g.Days[0].Date = date("2026-11-06")
				g.Traces[0].Date = g.Days[0].Date
			}
			k := kernel(t, g)
			in := titleInput()
			if err := titleCommand(k, reuse, in); err != nil {
				t.Fatal(err)
			}
			want := g.Clone()
			want.Chains[0].UpdatedAt = in.Now
			want.Definitions[0].ContentUpdatedAt = in.Now
			if reuse {
				want.Definitions[0].Title = in.Title
			} else {
				d := g.Clone().Definitions[0]
				d.ID = in.NewDefinitionID
				d.Sequence = 2
				d.Title = in.Title
				d.CreatedAt = in.Now
				d.ContentUpdatedAt = in.Now
				want.Definitions[0].SupersededAt = ptr(in.Now)
				want.Definitions[0].SupersededByDefinitionID = ptr(in.NewDefinitionID)
				want.Definitions = append(want.Definitions, d)
				want.Traces[0].DefinitionID = in.NewDefinitionID
				want.Traces[0].ContentUpdatedAt = in.Now
			}
			if !reflect.DeepEqual(want, k.Snapshot()) {
				t.Fatalf("unexpected delta reuse=%v future=%v", reuse, future)
			}
		}
	}
}
func TestTitleContinuationForkThenInputReuses(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := continuedGraph(TraceUnfinished)
		g.Days[0].LockedAt = ptr(instant(100))
		k := kernel(t, g)
		in := titleInput()
		in.Today = g.Traces[1].Date
		if err := titleCommand(k, reuse, in); err != nil {
			t.Fatal(err)
		}
		first := k.Snapshot()
		if len(first.Definitions) != 2 || first.Traces[1].DefinitionID != in.NewDefinitionID {
			t.Fatal("not isolated")
		}
		if !reflect.DeepEqual(first.Traces[0], g.Traces[0]) || first.Definitions[0].Title != g.Definitions[0].Title {
			t.Fatal("history rewritten")
		}
		in.Title = "第二次"
		in.Now = instant(111)
		in.NewDefinitionID = "unused"
		if err := k.SaveTaskTitleInput(in); err != nil {
			t.Fatal(err)
		}
		want := first.Clone()
		want.Definitions[1].Title = in.Title
		want.Definitions[1].ContentUpdatedAt = in.Now
		want.Chains[0].UpdatedAt = in.Now
		if !reflect.DeepEqual(want, k.Snapshot()) {
			t.Fatal("autosave inflated versions or changed traces")
		}
	}
}
func TestTitleReturnedHistoricalReferenceAndNotes(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := titleRichGraph()
		g.Traces[0].Status = TraceReturnedToPool
		g.Traces[0].SettledAt = ptr(instant(100))
		g.Traces[0].PinOrder = nil
		g.Subtasks = nil
		k := kernel(t, g)
		if err := titleCommand(k, reuse, titleInput()); err != nil {
			t.Fatal(err)
		}
		got := k.Snapshot()
		if len(got.Definitions) != 2 || !reflect.DeepEqual(got.Traces, g.Traces) || !reflect.DeepEqual(got.Chains[0].NoteEntries, g.Chains[0].NoteEntries) {
			t.Fatal("returned history or notes rewritten")
		}
	}
}
func TestTitleUnlockedCompletedReferenceStillForks(t *testing.T) {
	g := continuedGraph(TraceUnfinished)
	g.Traces[0].Status = TraceCompleted
	g.Traces[0].SettledAt = nil
	g.Traces[0].CompletedAt = ptr(instant(100))
	g.Traces[1].CarriedFromTraceID = nil
	g.Traces[1].ContinuationSeq = 0
	k := kernel(t, g)
	in := titleInput()
	in.Today = g.Traces[1].Date
	if err := k.SaveTaskTitleInput(in); err != nil {
		t.Fatal(err)
	}
	if len(k.Snapshot().Definitions) != 2 {
		t.Fatal("unlocked completion treated as editable")
	}
}
func TestTitleCancelledDraftIsNotDayHistory(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := baseGraph()
		tr := &g.Traces[0]
		tr.Status = TraceCancelledDraft
		tr.SettledAt = ptr(instant(100))
		tr.DraftCancellationID = ptr(CancellationID(identity(90)))
		tr.DraftCancelledOn = ptr(tr.Date)
		g.Days[0].LockedAt = ptr(instant(100))
		k := kernel(t, g)
		in := titleInput()
		in.Today = date("2026-11-06")
		in.NewDefinitionID = "unused"
		if err := titleCommand(k, reuse, in); err != nil {
			t.Fatal(err)
		}
		got := k.Snapshot()
		if len(got.Definitions) != 1 || !reflect.DeepEqual(got.Traces, g.Traces) || !reflect.DeepEqual(got.Days, g.Days) {
			t.Fatal("cancelled counted as history")
		}
	}
}
func TestTitleNormalizationAndClearing(t *testing.T) {
	// Go Unicode whitespace expectations only, not Foundation parity evidence.
	cases := []struct{ input, want string }{{" \t中文\r\n标题\r末尾 \n", "中文\n标题\n末尾"}, {"\u00a0\u2003👩🏽‍💻 e\u0301\u202f\u3000", "👩🏽‍💻 e\u0301"}, {" \r\n\t ", ""}, {"a\n\nb", "a\n\nb"}, {"\u200b文本\u200b", "\u200b文本\u200b"}}
	for _, tc := range cases {
		for _, reuse := range []bool{false, true} {
			k := kernel(t, baseGraph())
			in := titleInput()
			in.Title = tc.input
			if err := titleCommand(k, reuse, in); err != nil {
				t.Fatal(err)
			}
			got := k.Snapshot()
			if got.Definitions[len(got.Definitions)-1].Title != tc.want {
				t.Fatalf("normalize %q", tc.input)
			}
		}
	}
	k := kernel(t, TaskGraph{})
	unchanged(t, k, ErrInvalidTitle, func() error {
		return k.CreatePoolTask(CreatePoolTaskInput{ChainID: titleInput().ChainID, DefinitionID: titleInput().NewDefinitionID, Title: " \n ", Now: instant(100)})
	})
}
func TestTitleNoOpDoesNotConsumeClockOrIdentity(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		k := kernel(t, baseGraph())
		before := k.Snapshot()
		in := titleInput()
		in.Title = " \r\n测试任务\t "
		in.NewDefinitionID = "invalid-unused"
		for _, now := range []Instant{instant(1), Instant(math.Float64bits(math.NaN()))} {
			in.Now = now
			if err := titleCommand(k, reuse, in); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, k.Snapshot()) {
				t.Fatal("no-op changed graph")
			}
		}
	}
}
func TestTitleCapabilityGuardsBeforeNoOp(t *testing.T) {
	cases := []struct {
		name   string
		change func(*TaskGraph, *EditTaskTitleInput)
		want   error
	}{
		{"missing", func(g *TaskGraph, in *EditTaskTitleInput) { in.ChainID = TaskChainID(identity(999)) }, ErrNotFound},
		{"abandoned", func(g *TaskGraph, in *EditTaskTitleInput) { g.Chains[0].State = ChainAbandoned }, ErrChainAbandoned},
		{"completed", func(g *TaskGraph, in *EditTaskTitleInput) {
			g.Traces[0].Status = TraceCompleted
			g.Traces[0].CompletedAt = ptr(instant(100))
		}, ErrInvalidTransition},
		{"unfinished", func(g *TaskGraph, in *EditTaskTitleInput) {
			g.Traces[0].Status = TraceUnfinished
			g.Traces[0].SettledAt = ptr(instant(100))
		}, ErrInvalidTransition},
		{"past", func(g *TaskGraph, in *EditTaskTitleInput) { in.Today = date("2026-11-06") }, ErrImmutableHistory},
		{"locked", func(g *TaskGraph, in *EditTaskTitleInput) { g.Days[0].LockedAt = ptr(instant(100)) }, ErrImmutableHistory},
		{"invalid-today", func(g *TaskGraph, in *EditTaskTitleInput) { in.Today = LocalDate{2026, 2, 30} }, ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, reuse := range []bool{false, true} {
				g := baseGraph()
				in := titleInput()
				in.Title = g.Definitions[0].Title
				tc.change(&g, &in)
				k := kernel(t, g)
				unchanged(t, k, tc.want, func() error { return titleCommand(k, reuse, in) })
			}
		})
	}
}
func TestTitleInvalidMutationIsAtomic(t *testing.T) {
	cases := []struct {
		name   string
		change func(*TaskGraph, *EditTaskTitleInput)
		want   error
	}{
		{"definition-clock", func(g *TaskGraph, in *EditTaskTitleInput) { g.Definitions[0].ContentUpdatedAt = instant(120) }, ErrInvalidClock},
		{"trace-clock", func(g *TaskGraph, in *EditTaskTitleInput) { g.Traces[0].ContentUpdatedAt = instant(120) }, ErrInvalidClock},
		{"nan", func(g *TaskGraph, in *EditTaskTitleInput) { in.Now = Instant(math.Float64bits(math.NaN())) }, ErrInvalidClock},
		{"infinity", func(g *TaskGraph, in *EditTaskTitleInput) { in.Now = Instant(math.Float64bits(math.Inf(1))) }, ErrInvalidClock},
		{"duplicate-id", func(g *TaskGraph, in *EditTaskTitleInput) { in.NewDefinitionID = g.Definitions[0].ID }, ErrInvalidInput},
		{"invalid-id", func(g *TaskGraph, in *EditTaskTitleInput) { in.NewDefinitionID = "bad" }, ErrInvalidInput},
		{"missing-id", func(g *TaskGraph, in *EditTaskTitleInput) { in.NewDefinitionID = "" }, ErrInvalidInput},
		{"sequence-overflow", func(g *TaskGraph, in *EditTaskTitleInput) { g.Definitions[0].Sequence = int(^uint(0) >> 1) }, ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := baseGraph()
			in := titleInput()
			tc.change(&g, &in)
			k := kernel(t, g)
			unchanged(t, k, tc.want, func() error { return k.RenameTaskTitle(in) })
		})
	}
	// Reuse does not mutate the pending trace clock.
	g := baseGraph()
	g.Traces[0].ContentUpdatedAt = instant(120)
	k := kernel(t, g)
	if err := k.SaveTaskTitleInput(titleInput()); err != nil {
		t.Fatal(err)
	}
}
func TestTitleEqualClockAndNewerChainClock(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := baseGraph()
		g.Chains[0].UpdatedAt = instant(200)
		k := kernel(t, g)
		in := titleInput()
		in.Now = instant(100)
		if err := titleCommand(k, reuse, in); err != nil {
			t.Fatal(err)
		}
		got := k.Snapshot()
		if got.Chains[0].UpdatedAt != instant(200) || got.Definitions[0].ContentUpdatedAt != instant(100) {
			t.Fatal("clock diverged from source")
		}
	}
}
func titleWitnessGraph() TaskGraph {
	g := baseGraph()
	old := g.Definitions[0]
	old.ID = TaskDefinitionID(identity(20))
	old.Sequence = 7
	old.SupersededAt = ptr(instant(100))
	old.SupersededByDefinitionID = ptr(g.Definitions[0].ID)
	g.Definitions[0].SupersededByDefinitionID = ptr(old.ID)
	g.Definitions = append(g.Definitions, old)
	return g
}
func TestTitleRestorationWitnessAndMaxSequence(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		g := titleWitnessGraph()
		k := kernel(t, g)
		in := titleInput()
		if err := titleCommand(k, reuse, in); err != nil {
			t.Fatal(err)
		}
		got := k.Snapshot()
		if !reflect.DeepEqual(g.Definitions[1], got.Definitions[1]) {
			t.Fatal("old supersession changed")
		}
		if reuse {
			if !reflect.DeepEqual(g.Definitions[0].SupersededByDefinitionID, got.Definitions[0].SupersededByDefinitionID) {
				t.Fatal("witness removed")
			}
		} else if len(got.Definitions) != 3 || got.Definitions[2].Sequence != 8 || *got.Definitions[0].SupersededByDefinitionID != in.NewDefinitionID {
			t.Fatal("did not replace witness with max+1 successor")
		}
	}
}
func TestTitleRenameRebindsPendingNonCurrentDefinition(t *testing.T) {
	g := titleWitnessGraph()
	g.Traces[0].DefinitionID = g.Definitions[1].ID
	k := kernel(t, g)
	in := titleInput()
	if err := k.RenameTaskTitle(in); err != nil {
		t.Fatal(err)
	}
	if k.Snapshot().Traces[0].DefinitionID != in.NewDefinitionID {
		t.Fatal("pending older version left behind")
	}
}
func TestTitleVersionCopiesAreDetached(t *testing.T) {
	g := titleRichGraph()
	k := kernel(t, g)
	if err := k.RenameTaskTitle(titleInput()); err != nil {
		t.Fatal(err)
	}
	want := k.Snapshot()
	*g.Definitions[0].DescriptionText = "外部"
	g.Definitions[0].PlannedSubtasks[0].Title = "外部"
	snap := k.Snapshot()
	*snap.Definitions[1].DescriptionText = "快照"
	snap.Definitions[1].PlannedSubtasks[0].Title = "快照"
	if !reflect.DeepEqual(want, k.Snapshot()) {
		t.Fatal("caller alias")
	}
	if k.graph.Definitions[0].DescriptionText == k.graph.Definitions[1].DescriptionText || &k.graph.Definitions[0].PlannedSubtasks[0] == &k.graph.Definitions[1].PlannedSubtasks[0] {
		t.Fatal("versions alias")
	}
}
func TestTitleMultiplePendingRemainOutsideValidKernel(t *testing.T) {
	g := baseGraph()
	next := g.Traces[0]
	next.ID = DayTraceID(identity(22))
	next.Date = date("2026-11-06")
	g.Traces = append(g.Traces, next)
	g.Days = append(g.Days, Day{ID: DayID(identity(23)), Date: next.Date, CreatedAt: instant(100), UpdatedAt: instant(100)})
	assertIssue(t, g, "duplicateActiveTrace")
	if _, err := NewTaskKernel(g); err == nil {
		t.Fatal("invalid multi-pending graph accepted")
	}
	k := &TaskKernel{graph: g.Clone()}
	before := k.Snapshot()
	err := k.RenameTaskTitle(titleInput())
	var validation *ValidationError
	if !errors.As(err, &validation) || !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("invalid candidate published")
	}
}
func TestTitleConcurrentInputAndSnapshots(t *testing.T) {
	k := kernel(t, baseGraph())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := titleInput()
			if i%2 == 0 {
				in.Title = "甲"
			} else {
				in.Title = "乙"
			}
			if err := k.SaveTaskTitleInput(in); err != nil {
				t.Error(err)
			}
			_ = k.Snapshot()
		}(i)
	}
	wg.Wait()
	got := k.Snapshot()
	assertValid(t, got)
	if len(got.Definitions) != 1 {
		t.Fatal("autosave versions inflated")
	}
}

func TestTitleRepeatedExplicitRenameCreatesVersions(t *testing.T) {
	k := kernel(t, baseGraph())
	in := titleInput()
	if err := k.RenameTaskTitle(in); err != nil {
		t.Fatal(err)
	}
	first := k.Snapshot()
	in.NewDefinitionID = TaskDefinitionID(identity(51))
	in.Title = "另一个标题"
	in.Now = instant(111)
	if err := k.RenameTaskTitle(in); err != nil {
		t.Fatal(err)
	}
	got := k.Snapshot()
	if len(got.Definitions) != 3 || got.Definitions[2].Sequence != 3 || got.Traces[0].DefinitionID != in.NewDefinitionID || !reflect.DeepEqual(got.Definitions[0], first.Definitions[0]) {
		t.Fatal("explicit rename failed to version independently")
	}
}
func TestTitleIdentityConflictIncludesOtherChains(t *testing.T) {
	k := kernel(t, baseGraph())
	foreign := TaskDefinitionID(identity(61))
	if err := k.CreatePoolTask(CreatePoolTaskInput{ChainID: TaskChainID(identity(60)), DefinitionID: foreign, Title: "另一链", Now: instant(100)}); err != nil {
		t.Fatal(err)
	}
	in := titleInput()
	in.NewDefinitionID = foreign
	unchanged(t, k, ErrInvalidInput, func() error { return k.RenameTaskTitle(in) })
	// IDs are only validated on the forking branch, including existing IDs.
	if err := k.SaveTaskTitleInput(in); err != nil {
		t.Fatal(err)
	}
}
func TestTitleReuseDoesNotConsumeSequenceOrIdentity(t *testing.T) {
	g := baseGraph()
	g.Definitions[0].Sequence = int(^uint(0) >> 1)
	k := kernel(t, g)
	in := titleInput()
	in.NewDefinitionID = g.Definitions[0].ID
	if err := k.SaveTaskTitleInput(in); err != nil {
		t.Fatal(err)
	}
	if len(k.Snapshot().Definitions) != 1 {
		t.Fatal("unused identity consumed")
	}
}
func TestTitleInputForkRejectsOldTraceClockAtomically(t *testing.T) {
	g := continuedGraph(TraceUnfinished)
	g.Traces[1].ContentUpdatedAt = instant(200)
	k := kernel(t, g)
	in := titleInput()
	in.Today = g.Traces[1].Date
	unchanged(t, k, ErrInvalidClock, func() error { return k.SaveTaskTitleInput(in) })
}
func TestTitleNonfiniteClockCannotPublishOnReuse(t *testing.T) {
	for _, now := range []Instant{Instant(math.Float64bits(math.NaN())), Instant(math.Float64bits(math.Inf(-1))), instant(99)} {
		k := kernel(t, baseGraph())
		in := titleInput()
		in.Now = now
		unchanged(t, k, ErrInvalidClock, func() error { return k.SaveTaskTitleInput(in) })
	}
	// Finite boundary is valid: commands don't synthesize a next representable tick.
	k := kernel(t, baseGraph())
	in := titleInput()
	in.Now = instant(math.MaxFloat64)
	if err := k.RenameTaskTitle(in); err != nil {
		t.Fatal(err)
	}
}

func TestTitleCanonicalEquivalenceNoOp(t *testing.T) {
	pairs := [][2]string{{"café", "cafe\u0301"}, {"가", "\u1100\u1161"}, {"a\u0323\u0301", "a\u0301\u0323"}}
	for _, pair := range pairs {
		for _, reverse := range []bool{false, true} {
			for _, reuse := range []bool{false, true} {
				stored, input := pair[0], pair[1]
				if reverse {
					stored, input = input, stored
				}
				g := baseGraph()
				g.Definitions[0].Title = stored
				k := kernel(t, g)
				in := titleInput()
				in.Title = " \r\n" + input + "\t "
				in.NewDefinitionID = "invalid-unused"
				in.Now = Instant(math.Float64bits(math.NaN()))
				if err := titleCommand(k, reuse, in); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(g, k.Snapshot()) {
					t.Fatalf("canonical no-op changed stored bytes or facts: %q -> %q reuse=%v", stored, input, reuse)
				}
			}
		}
	}
}
func TestTitleCanonicalEquivalentStillChecksCapability(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		for _, status := range []string{"abandoned", "completed", "past", "locked"} {
			g := baseGraph()
			g.Definitions[0].Title = "café"
			in := titleInput()
			in.Title = "cafe\u0301"
			in.Now = Instant(math.Float64bits(math.NaN()))
			in.NewDefinitionID = "invalid-unused"
			want := ErrImmutableHistory
			switch status {
			case "abandoned":
				g.Chains[0].State = ChainAbandoned
				want = ErrChainAbandoned
			case "completed":
				g.Traces[0].Status = TraceCompleted
				g.Traces[0].CompletedAt = ptr(instant(100))
				want = ErrInvalidTransition
			case "past":
				in.Today = date("2026-11-06")
			case "locked":
				g.Days[0].LockedAt = ptr(instant(100))
			}
			k := kernel(t, g)
			unchanged(t, k, want, func() error { return titleCommand(k, reuse, in) })
		}
	}
}
func TestTitleCompatibilityDistinctAndNewStorageBytesPreserved(t *testing.T) {
	// NFC must not be replaced by NFKC, case folding, or accent stripping.
	pairs := [][2]string{{"ﬀ", "ff"}, {"Ａ", "A"}, {"①", "1"}, {"café", "cafe"}, {"CAFÉ", "café"}, {"other", "cafe\u0301"}}
	for _, pair := range pairs {
		for _, reuse := range []bool{false, true} {
			g := baseGraph()
			g.Definitions[0].Title = pair[0]
			k := kernel(t, g)
			in := titleInput()
			in.Title = pair[1]
			if err := titleCommand(k, reuse, in); err != nil {
				t.Fatal(err)
			}
			got := k.Snapshot()
			d := got.Definitions[len(got.Definitions)-1]
			if d.Title != pair[1] || d.ContentUpdatedAt != in.Now || got.Chains[0].UpdatedAt != in.Now {
				t.Fatalf("distinct input not saved byte-for-byte: %q -> %q", pair[0], pair[1])
			}
			if !reuse && len(got.Definitions) != 2 {
				t.Fatal("distinct rename did not fork")
			}
		}
	}
}
