// SPDX-License-Identifier: AGPL-3.0-only
package recurrence

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Mirrors TaskCycleSeriesTests.testTemplateContentEditPreservesHistoryAndUpdatesEditableOccurrences.
// A deferred trace is still history even when another trace on its chain is editable.
func TestContentForksDefinitionWhenEditableChainHasHistory(t *testing.T) {
	s, id, ids := createFive(t)
	source := s.Tasks.Traces[0]
	s.Tasks.Traces[0].Status = core.TraceDeferred
	s.Tasks.Traces[0].SettledAt = ptr(at(101))
	s.Tasks.Traces[0].ContentUpdatedAt = at(101)
	target := source
	raw, _ := nextID(ids)
	target.ID = core.DayTraceID(raw)
	target.Date = day("2026-07-21")
	target.Priority = 2
	target.CarriedFromTraceID = ptr(source.ID)
	target.CreatedAt = at(101)
	target.ContentUpdatedAt = at(101)
	s.Tasks.Traces = append(s.Tasks.Traces, target)
	valid(t, s)
	description := "新说明"
	next, e := s.UpdateContent(ContentInput{SeriesID: id, Title: "新版", DescriptionText: &description, Today: day("2026-07-20"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	oldDef := next.Tasks.Definitions[0]
	pending := next.pendingTrace(source.ChainID)
	if oldDef.Title != "每日伸展" || oldDef.SupersededAt == nil || pending.DefinitionID == oldDef.ID || pending.DescriptionText == nil || *pending.DescriptionText != "新说明" {
		t.Fatal("historical definition was rewritten or pending not repointed")
	}
	if next.Tasks.Traces[0].DefinitionID != oldDef.ID || !reflect.DeepEqual(next.Tasks.Traces[0], s.Tasks.Traces[0]) {
		t.Fatal("historical trace was modified")
	}
	if len(next.Tasks.Definitions) != len(s.Tasks.Definitions)+1 {
		t.Fatal("wrong version count")
	}
	description = "caller edit"
	if *next.Series[0].DescriptionText != "新说明" {
		t.Fatal("caller pointer leaked")
	}
	valid(t, next)
}

func TestContentReactivatesSameSubtaskLineageWithoutReorderingHistory(t *testing.T) {
	ids := idsFrom(0)
	a := core.PlannedSubtask{ID: core.PlannedSubtaskID(uid(900)), LineageID: core.SubtaskLineageID(uid(901)), Title: "A", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: at(100)}
	b := core.PlannedSubtask{ID: core.PlannedSubtaskID(uid(902)), LineageID: core.SubtaskLineageID(uid(903)), Title: "B", Difficulty: core.DifficultyHard, Position: 2, CreatedAt: at(100)}
	s, id, e := (State{}).Create(CreateInput{Title: "父计划", PlannedSubtasks: []core.PlannedSubtask{a, b}, StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: Until(day("2026-07-20")), Today: day("2026-07-20"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	removed, e := s.UpdateContent(ContentInput{SeriesID: id, Title: "父计划", PlannedSubtasks: []core.PlannedSubtask{b}, Today: day("2026-07-20"), Now: at(101)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if removed.Tasks.Subtasks[0].Status != core.SubtaskCancelledDraft || removed.Tasks.Subtasks[0].DraftCancellationID == nil {
		t.Fatal("missing child cancellation witness")
	}
	a.Title = "A restored"
	a.Position = 2
	b.Position = 1
	restored, e := removed.UpdateContent(ContentInput{SeriesID: id, Title: "父计划", PlannedSubtasks: []core.PlannedSubtask{b, a}, Today: day("2026-07-20"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if len(restored.Tasks.Subtasks) != 2 || restored.Tasks.Subtasks[0].ID != s.Tasks.Subtasks[0].ID || restored.Tasks.Subtasks[0].Position != 1 || restored.Tasks.Subtasks[1].Position != 2 || restored.Tasks.Subtasks[0].Status != core.SubtaskPending || restored.Tasks.Subtasks[0].DraftCancellationID != nil {
		t.Fatal("lineage identity/order not preserved")
	}
	valid(t, restored)
}

// Source taskCycleTracks sorts active/upcoming ascending, ended/stopped descending.
func TestTracksSortLifecyclesAndTerminalFactsIgnoreClockRollback(t *testing.T) {
	ids := idsFrom(0)
	s := State{}
	var err error
	var activeID, upcomingID, endedID, stoppedID core.TaskCycleSeriesID
	create := func(title, start, end string) core.TaskCycleSeriesID {
		var id core.TaskCycleSeriesID
		s, id, err = s.Create(CreateInput{Title: title, StartDate: day(start), Schedule: Daily(), EndCondition: Until(day(end)), Today: day("2026-07-01"), Now: at(100)}, ids)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	upcomingID = create("upcoming", "2026-07-25", "2026-07-26")
	endedID = create("ended", "2026-07-01", "2026-07-02")
	activeID = create("active", "2026-07-20", "2026-07-24")
	stoppedID = create("stopped", "2026-07-20", "2026-07-24")
	s, _, err = s.Stop(stoppedID, day("2026-07-20"), at(101), ids)
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := s.Tracks(day("2026-07-21"))
	if err != nil {
		t.Fatal(err)
	}
	want := []core.TaskCycleSeriesID{activeID, upcomingID, endedID, stoppedID}
	for i, tr := range tracks {
		if tr.ID != want[i] {
			t.Fatalf("order %#v", tracks)
		}
	}
	p := s.Series[s.seriesIndex(stoppedID)]
	if p.Lifecycle(day("2026-07-01")).Kind != Stopped {
		t.Fatal("terminal fact lost to local date rollback")
	}
}

func TestTrackTargetsKeepCompletedContinuationButRemoveUnfinishedCount(t *testing.T) {
	s, id, ids := createFive(t)
	source := s.Tasks.Traces[0]
	s.Tasks.Traces[0].Status = core.TraceUnfinished
	s.Tasks.Traces[0].SettledAt = ptr(at(101))
	s.Tasks.Traces[0].ContentUpdatedAt = at(101)
	next := source
	raw, _ := nextID(ids)
	next.ID = core.DayTraceID(raw)
	next.Date = day("2026-07-21")
	next.Priority = 2
	next.ContinuationSeq = 1
	next.CarriedFromTraceID = ptr(source.ID)
	next.CreatedAt = at(102)
	next.ContentUpdatedAt = at(103)
	next.Status = core.TraceCompleted
	next.CompletedAt = ptr(at(103))
	s.Tasks.Traces = append(s.Tasks.Traces, next)
	valid(t, s)
	track, e := s.Track(id, day("2026-07-22"))
	if e != nil {
		t.Fatal(e)
	}
	d := track.Days[0]
	if d.State() != Unfinished || d.CompletedTarget == nil || d.CompletedTarget.TraceID != next.ID || d.UnfinishedTarget != nil || track.UnfinishedCount() != 0 || track.CompletedCount() != 1 {
		t.Fatal("continuation projection lost", d)
	}
	if d.NavigationTarget() == nil || d.NavigationTarget().TraceID != source.ID {
		t.Fatal("occurrence navigation changed")
	}
}

func TestRevisionProducesAssignmentsOnlyForNewDatesNotReactivatedChains(t *testing.T) {
	s, id, ids := createFive(t)
	category := classification.CategoryID(uid(900))
	s, e := s.SetTemplateClassification(ClassificationInput{SeriesID: id, CategoryID: &category, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: uid(901), Now: at(101)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	reduced, out, e := s.Revise(ReviseInput{SeriesID: id, Schedule: Schedule{Kind: EveryDays, DayInterval: 2}, EndCondition: Until(day("2026-07-24")), Today: day("2026-07-20"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.ClassificationAssignments) != 0 {
		t.Fatal("existing chains should retain their own classification")
	}
	expanded, out, e := reduced.Revise(ReviseInput{SeriesID: id, Schedule: Daily(), EndCondition: Until(day("2026-07-25")), Today: day("2026-07-20"), Now: at(103)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if out.CreatedOccurrenceCount != 3 || len(out.ClassificationAssignments) != 1 || *out.ClassificationAssignments[0].CategoryID != category || expanded.memberChain(id, day("2026-07-25")).ID != out.ClassificationAssignments[0].ChainID {
		t.Fatal(out)
	}
	if len(expanded.Series[0].PlanRevisions) != 3 || len(expanded.Tasks.Traces) != 8 {
		t.Fatal("old fact history lost")
	}
}

func TestTrackStateMappingCountsAndSkippedNavigation(t *testing.T) {
	today := day("2026-07-20")
	cases := []struct {
		status core.TraceStatus
		want   TrackState
	}{
		{core.TracePending, PendingToday}, {core.TraceCompleted, Completed}, {core.TraceUnfinished, Unfinished}, {core.TraceDeferred, Deferred}, {core.TraceChanged, Changed}, {core.TraceReturnedToPool, ReturnedToPool}, {core.TraceCancelledDraft, Skipped}, {core.TraceAbandoned, Abandoned},
	}
	for _, tc := range cases {
		trace := core.DayTrace{ID: core.DayTraceID(uid(1)), Date: today, Status: tc.status}
		got := traceTarget(&trace, today, Skipped)
		if got == nil || got.State != tc.want {
			t.Fatalf("%s -> %#v", tc.status, got)
		}
		d := TrackDay{OccurrenceTarget: got}
		if (d.NavigationTarget() == nil) != (tc.want == Skipped || tc.want == ReturnedToPool) {
			t.Fatal("invalid navigation", tc)
		}
	}
	if traceTarget(nil, today, Skipped) != nil || (TrackDay{}).State() != NotScheduled {
		t.Fatal("missing target not empty")
	}
	a := core.DayTrace{Date: day("2026-07-19"), Status: core.TracePending}
	b := core.DayTrace{Date: day("2026-07-21"), Status: core.TracePending}
	if traceTarget(&a, today, Skipped).State != PendingPast || traceTarget(&b, today, Skipped).State != Planned {
		t.Fatal("pending date state")
	}
	tr := Track{Lifecycle: Lifecycle{Kind: Active}, Days: []TrackDay{{OccurrenceTarget: &TraceTarget{State: ReturnedToPool}}, {OccurrenceTarget: &TraceTarget{State: Planned}, FuturePlanTarget: &TraceTarget{State: Planned}}, {}}}
	if tr.PooledOccurrenceCount() != 1 || tr.PlannedCount() != 1 || tr.ScheduledCount() != 2 || tr.CompletionGoalCount() != 2 || !tr.CanStop() {
		t.Fatal("projection counts")
	}
}

func TestEqualTimeCancellationFactsHaveStableUUIDOrder(t *testing.T) {
	s, id, _ := createFive(t)
	ids := idsFrom(999)
	next, e := s.Skip(id, day("2026-07-21"), day("2026-07-20"), at(101), ids)
	if e != nil {
		t.Fatal(e)
	}
	next, e = next.Skip(id, day("2026-07-22"), day("2026-07-20"), at(101), idsFrom(499))
	if e != nil {
		t.Fatal(e)
	}
	facts := next.Series[0].CancellationFacts
	if len(facts) != 2 || facts[0].ID != uid(500) || facts[1].ID != uid(1000) {
		t.Fatal("unstable fact ordering", facts)
	}
	hostile := next.Clone()
	hostile.Series[0].CancellationFacts[0], hostile.Series[0].CancellationFacts[1] = hostile.Series[0].CancellationFacts[1], hostile.Series[0].CancellationFacts[0]
	if hostile.Validate() == nil {
		t.Fatal("unordered facts accepted")
	}
}

// appendPlanRevision also rejects nonterminal-reason followingDates facts.
// Such a fact is valid persisted data even though current commands do not emit it.
func TestRevisionRejectsAnyFollowingDatesFact(t *testing.T) {
	s, id, ids := createFive(t)
	s.Series[0].CancellationFacts = []CancellationFact{{ID: uid(900), Scope: FollowingDates, Date: day("2026-07-21"), Reason: PlanChanged, RecordedAt: at(100)}}
	valid(t, s)
	if s.Series[0].Lifecycle(day("2026-07-20")).Kind != Active {
		t.Fatal("synthetic fact should be nonterminal lifecycle")
	}
	next, _, e := s.Revise(ReviseInput{SeriesID: id, Schedule: Daily(), EndCondition: Until(day("2026-07-25")), Today: day("2026-07-20"), Now: at(101)}, ids)
	if e == nil {
		t.Fatal("followingDates plan revision accepted")
	}
	checkUnchanged(t, s, next)
}

func FuzzRecurrenceCommandsRemainValidAndAtomic(f *testing.F) {
	f.Add([]byte{0, 2, 3, 4, 5, 1})
	f.Add([]byte{2, 2, 0, 4, 3, 1, 5})
	f.Fuzz(func(t *testing.T, commands []byte) {
		if len(commands) > 32 {
			commands = commands[:32]
		}
		ids := idsFrom(0)
		s, id, e := (State{}).Create(CreateInput{Title: "fuzz fixture", StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: Until(day("2026-07-22")), Today: day("2026-07-20"), Now: at(100)}, ids)
		if e != nil {
			t.Fatal(e)
		}
		for i, b := range commands {
			before := s.Clone()
			var next State
			var err error
			today, _ := day("2026-07-20").AddDays(int(b % 3))
			now := at(float64(101 + i))
			switch b % 6 {
			case 0:
				occurrence, _ := day("2026-07-20").AddDays(int(b % 6))
				next, err = s.Skip(id, occurrence, today, now, ids)
			case 1:
				next, _, err = s.Stop(id, today, now, ids)
			case 2:
				end, _ := day("2026-07-22").AddDays(int(b % 4))
				next, _, err = s.Revise(ReviseInput{SeriesID: id, Schedule: Schedule{Kind: EveryDays, DayInterval: int(b%5) + 1}, EndCondition: Until(end), Today: today, Now: now}, ids)
			case 3:
				next, err = s.UpdateContent(ContentInput{SeriesID: id, Title: fmt.Sprint("edit ", i), Today: today, Now: now}, ids)
			case 4:
				interaction, _ := nextID(ids)
				next, err = s.SetTemplateClassification(ClassificationInput{SeriesID: id, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: interaction, Now: now}, ids)
			case 5:
				next, _, err = s.Reconcile(today, now, ids)
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("command modified receiver")
			}
			if err != nil {
				checkUnchanged(t, before, next)
			} else {
				valid(t, next)
				s = next
			}
		}
	})
}

func TestNaturallyEndedRejectsPlanAndContentEdits(t *testing.T) {
	s, id, ids := createFive(t)
	next, _, e := s.Revise(ReviseInput{SeriesID: id, Schedule: Daily(), EndCondition: Until(day("2026-07-26")), Today: day("2026-07-25"), Now: at(101)}, ids)
	if e == nil {
		t.Fatal("ended plan revised")
	}
	checkUnchanged(t, s, next)
	next, e = s.UpdateContent(ContentInput{SeriesID: id, Title: "invalid stale edit", Today: day("2026-07-25"), Now: at(101)}, ids)
	if e == nil {
		t.Fatal("ended content edited")
	}
	checkUnchanged(t, s, next)
}
func TestCreationRejectsUnboundedWindowAndMissingMember(t *testing.T) {
	original := State{}
	next, _, e := original.Create(CreateInput{Title: "too long", StartDate: day("2026-01-01"), Schedule: Daily(), EndCondition: Until(day("2027-01-02")), Today: day("2026-01-01"), Now: at(100)}, idsFrom(0))
	if e == nil {
		t.Fatal("367 days accepted")
	}
	checkUnchanged(t, original, next)
	s, _, _ := createFive(t)
	s.Tasks.Chains = s.Tasks.Chains[1:]
	s.Tasks.Definitions = s.Tasks.Definitions[1:]
	s.Tasks.Traces = s.Tasks.Traces[1:]
	if e = core.ValidateTaskGraph(s.Tasks); e != nil {
		t.Fatal("fixture should have a valid task graph", e)
	}
	if s.Validate() == nil {
		t.Fatal("missing historical occurrence accepted")
	}
}
func TestSkipRescheduledOccurrenceShowsSkippedWithoutNavigation(t *testing.T) {
	s, id, ids := createFive(t)
	c := s.memberChain(id, day("2026-07-21"))
	for i := range s.Tasks.Traces {
		if s.Tasks.Traces[i].ChainID == c.ID {
			s.Tasks.Traces[i].Date = day("2026-07-23")
			s.Tasks.Traces[i].Priority = 2
			s.Tasks.Traces[i].ContentUpdatedAt = at(101)
		}
	}
	valid(t, s)
	skipped, e := s.Skip(id, day("2026-07-21"), day("2026-07-20"), at(102), ids)
	if e != nil {
		t.Fatal(e)
	}
	track, e := skipped.Track(id, day("2026-07-20"))
	if e != nil {
		t.Fatal(e)
	}
	d := track.Days[1]
	if d.State() != Skipped || d.NavigationTarget() != nil || d.FuturePlanTarget != nil || d.OccurrenceTarget.Date != day("2026-07-23") {
		t.Fatal("rescheduled skip lost identity", d)
	}
}
func TestFinalDeferredOccurrenceRetainsTargetsPastPlanEnd(t *testing.T) {
	for _, status := range []core.TraceStatus{core.TracePending, core.TraceCompleted, core.TraceUnfinished} {
		t.Run(string(status), func(t *testing.T) {
			s, id, ids := createFive(t)
			source := s.Tasks.Traces[4]
			s.Tasks.Traces[4].Status = core.TraceDeferred
			s.Tasks.Traces[4].SettledAt = ptr(at(101))
			s.Tasks.Traces[4].ContentUpdatedAt = at(101)
			next := source
			raw, _ := nextID(ids)
			next.ID = core.DayTraceID(raw)
			next.Date = day("2026-07-25")
			next.CarriedFromTraceID = ptr(source.ID)
			next.CreatedAt = at(101)
			next.ContentUpdatedAt = at(102)
			next.Status = status
			if status == core.TraceCompleted {
				next.CompletedAt = ptr(at(102))
			}
			if status == core.TraceUnfinished {
				next.SettledAt = ptr(at(102))
			}
			s.Tasks.Traces = append(s.Tasks.Traces, next)
			if e := s.ensureDay(next.Date, at(101), ids); e != nil {
				t.Fatal(e)
			}
			valid(t, s)
			track, e := s.Track(id, day("2026-07-24"))
			if e != nil {
				t.Fatal(e)
			}
			d := track.Days[4]
			if d.State() != Deferred {
				t.Fatal("origin state rewritten")
			}
			switch status {
			case core.TracePending:
				if d.FuturePlanTarget == nil || d.FuturePlanTarget.TraceID != next.ID {
					t.Fatal("future target hidden")
				}
			case core.TraceCompleted:
				if d.CompletedTarget == nil || d.CompletedTarget.TraceID != next.ID {
					t.Fatal("completion hidden")
				}
			case core.TraceUnfinished:
				if d.UnfinishedTarget == nil || d.UnfinishedTarget.TraceID != next.ID {
					t.Fatal("unfinished target hidden")
				}
			}
		})
	}
}
