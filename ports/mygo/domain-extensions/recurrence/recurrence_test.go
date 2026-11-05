// SPDX-License-Identifier: AGPL-3.0-only
package recurrence

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func uid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func idsFrom(start int) IdentitySource {
	n := start
	return func() (string, error) { n++; return uid(n), nil }
}
func at(v float64) core.Instant {
	x, e := core.InstantFromSeconds(v)
	if e != nil {
		panic(e)
	}
	return x
}
func day(s string) core.LocalDate {
	d, e := core.ParseLocalDate(s)
	if e != nil {
		panic(e)
	}
	return d
}
func createFive(t *testing.T) (State, core.TaskCycleSeriesID, IdentitySource) {
	t.Helper()
	ids := idsFrom(0)
	s, id, e := (State{}).Create(CreateInput{Title: "每日伸展", StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: Until(day("2026-07-24")), Today: day("2026-07-20"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	return s, id, ids
}
func valid(t *testing.T, s State) {
	t.Helper()
	if e := s.Validate(); e != nil {
		t.Fatalf("invalid state: %#v", e)
	}
}
func traceOn(t *testing.T, s State, id core.TaskCycleSeriesID, d core.LocalDate) core.DayTrace {
	t.Helper()
	c := s.memberChain(id, d)
	if c == nil {
		t.Fatal("missing chain")
	}
	tr := s.pendingTrace(c.ID)
	if tr == nil {
		t.Fatal("missing active trace")
	}
	return *tr
}
func checkUnchanged(t *testing.T, before, after State) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed command changed state")
	}
}

func TestDailyCreationMaterializesIndependentChains(t *testing.T) {
	s, id, _ := createFive(t)
	valid(t, s)
	if len(s.Tasks.Chains) != 5 || len(s.Tasks.Traces) != 5 || len(s.Tasks.Days) != 5 {
		t.Fatal("wrong materialization")
	}
	seen := map[core.TaskChainID]bool{}
	for _, c := range s.Tasks.Chains {
		if seen[c.ID] || c.CycleMembership == nil || c.CycleMembership.SeriesID != id {
			t.Fatal("wrong identity/membership")
		}
		seen[c.ID] = true
	}
	track, e := s.Track(id, day("2026-07-20"))
	if e != nil || len(track.Days) != 5 || track.ScheduledCount() != 5 || track.FuturePlanCount() != 4 || track.Lifecycle.Kind != Active || track.Lifecycle.Date == nil || *track.Lifecycle.Date != day("2026-07-20") {
		t.Fatalf("bad track %#v %v", track, e)
	}
}
func TestScheduleBoundsAndAnchoring(t *testing.T) {
	anchor := day("2026-07-21")
	weekly := Schedule{Kind: SelectedWeekdays, EveryWeeks: 2, Weekdays: []int{2}}
	for _, tc := range []struct {
		d    string
		want bool
	}{{"2026-07-20", false}, {"2026-07-27", true}, {"2026-08-03", false}, {"2026-08-10", true}} {
		if got := weekly.Includes(day(tc.d), anchor); got != tc.want {
			t.Errorf("%s got %v want %v", tc.d, got, tc.want)
		}
	}
	for _, schedule := range []Schedule{{Kind: EveryDays, DayInterval: 0}, {Kind: EveryDays, DayInterval: 31}, {Kind: SelectedWeekdays, EveryWeeks: 0, Weekdays: []int{1}}, {Kind: SelectedWeekdays, EveryWeeks: 5, Weekdays: []int{1}}, {Kind: SelectedWeekdays, EveryWeeks: 1}, {Kind: SelectedWeekdays, EveryWeeks: 1, Weekdays: []int{1, 1}}, {Kind: SelectedWeekdays, EveryWeeks: 1, Weekdays: []int{8}}} {
		if schedule.Validate() == nil {
			t.Fatalf("invalid schedule accepted %#v", schedule)
		}
	}
	ds, e := (Schedule{Kind: EveryDays, DayInterval: 2}).OccurrenceDates(day("2026-07-20"), day("2026-07-24"))
	if e != nil || len(ds) != 3 || ds[1] != day("2026-07-22") {
		t.Fatal(ds, e)
	}
}
func TestBoundedEndConditions(t *testing.T) {
	if end, e := (EndCondition{Kind: DurationDays, Days: 366}).ResolvedEndDate(day("2026-01-01")); e != nil || end != day("2027-01-01") {
		t.Fatal(end, e)
	}
	if ds, e := materializableDates(day("2026-01-01"), day("2027-01-01")); e != nil || len(ds) != 366 {
		t.Fatal(len(ds), e)
	}
	if _, e := materializableDates(day("2026-01-01"), day("2027-01-02")); e == nil {
		t.Fatal("unbounded range")
	}
	for _, end := range []EndCondition{{Kind: DurationDays, Days: 0}, {Kind: DurationDays, Days: 367}, {Kind: CompletionTarget, Count: 0, LatestDate: day("2026-12-31")}, {Kind: CompletionTarget, Count: 367, LatestDate: day("2026-12-31")}, {Kind: "unknown"}} {
		if _, e := end.ResolvedEndDate(day("2026-01-01")); e == nil {
			t.Fatal("invalid end accepted")
		}
	}
}
func TestCreationRejectsPastEmptyAndMissingOccurrencesAtomically(t *testing.T) {
	for _, in := range []CreateInput{
		{Title: " ", StartDate: day("2026-07-20"), Today: day("2026-07-20"), Now: at(100), Schedule: Daily(), EndCondition: Until(day("2026-07-24"))},
		{Title: "任务", StartDate: day("2026-07-19"), Today: day("2026-07-20"), Now: at(100), Schedule: Daily(), EndCondition: Until(day("2026-07-24"))},
		{Title: "任务", StartDate: day("2026-07-20"), Today: day("2026-07-20"), Now: at(100), Schedule: Schedule{Kind: SelectedWeekdays, EveryWeeks: 1, Weekdays: []int{1}}, EndCondition: Until(day("2026-07-20"))},
	} {
		before := State{}
		after, _, e := before.Create(in, idsFrom(0))
		if e == nil {
			t.Fatal("invalid input accepted")
		}
		checkUnchanged(t, before, after)
	}
}
func TestRevisionOnlyChangesFutureAndRetainsHistory(t *testing.T) {
	s, id, ids := createFive(t)
	before := s.Clone()
	after, out, e := s.Revise(ReviseInput{SeriesID: id, Schedule: Schedule{Kind: EveryDays, DayInterval: 2}, EndCondition: Until(day("2026-07-24")), Today: day("2026-07-20"), Now: at(101)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	valid(t, after)
	if out.EffectiveFrom != day("2026-07-21") || out.CancelledOccurrenceCount != 2 || out.CreatedOccurrenceCount != 0 {
		t.Fatalf("wrong outcome %#v", out)
	}
	if !reflect.DeepEqual(before.Tasks.Traces[0], after.Tasks.Traces[0]) {
		t.Fatal("today rewritten")
	}
	if len(after.Series[0].PlanRevisions) != 2 || len(after.Tasks.Chains) != 5 {
		t.Fatal("history discarded")
	}
	checkUnchanged(t, before, s)
	reintroduced, out, e := after.Revise(ReviseInput{SeriesID: id, Schedule: Daily(), EndCondition: Until(day("2026-07-24")), Today: day("2026-07-20"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if out.CreatedOccurrenceCount != 2 || len(reintroduced.Tasks.Chains) != 5 || len(reintroduced.Tasks.Traces) != 7 {
		t.Fatal("reactivation cloned chain or erased old fact")
	}
	valid(t, reintroduced)
	for _, c := range reintroduced.Tasks.Chains {
		if c.CycleMembership.OccurrenceDate == day("2026-07-22") {
			n := 0
			for _, d := range reintroduced.Tasks.Definitions {
				if d.ChainID == c.ID {
					n++
				}
			}
			if n != 2 {
				t.Fatal("reactivation lacks definition history")
			}
		}
	}
}
func TestUserSkippedOccurrenceDoesNotReactivateOnPlanChange(t *testing.T) {
	s, id, ids := createFive(t)
	s, e := s.Skip(id, day("2026-07-22"), day("2026-07-20"), at(101), ids)
	if e != nil {
		t.Fatal(e)
	}
	next, out, e := s.Revise(ReviseInput{SeriesID: id, Schedule: Daily(), EndCondition: Until(day("2026-07-24")), Today: day("2026-07-20"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if out.CreatedOccurrenceCount != 0 || next.pendingTrace(next.memberChain(id, day("2026-07-22")).ID) != nil {
		t.Fatal("user skip silently undone")
	}
}
func TestUnstartedStartDateRevisionAndStartedRejection(t *testing.T) {
	ids := idsFrom(0)
	s, id, e := (State{}).Create(CreateInput{Title: "明日起", StartDate: day("2026-07-21"), Schedule: Daily(), EndCondition: EndCondition{Kind: DurationDays, Days: 3}, Today: day("2026-07-20"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if !s.CanReviseStartDate(id, day("2026-07-20")) {
		t.Fatal("unstarted plan blocked")
	}
	next, out, e := s.Revise(ReviseInput{SeriesID: id, StartDate: ptr(day("2026-07-23")), Schedule: Daily(), EndCondition: EndCondition{Kind: DurationDays, Days: 3}, Today: day("2026-07-20"), Now: at(101)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if out.EffectiveFrom != day("2026-07-23") || next.Series[0].StartDate != day("2026-07-23") || next.Series[0].EndDate != day("2026-07-25") {
		t.Fatal(out)
	}
	valid(t, next)
	old := next.Clone()
	after, _, e := next.Revise(ReviseInput{SeriesID: id, StartDate: ptr(day("2026-07-24")), Schedule: Daily(), EndCondition: Until(day("2026-07-25")), Today: day("2026-07-23"), Now: at(102)}, ids)
	if e == nil {
		t.Fatal("started plan moved")
	}
	checkUnchanged(t, old, after)
}
func TestSkipAndStopAreAtomicAndPreserveToday(t *testing.T) {
	s, id, ids := createFive(t)
	for _, d := range []string{"2026-07-19", "2026-07-20"} {
		after, e := s.Skip(id, day(d), day("2026-07-20"), at(101), ids)
		if e == nil {
			t.Fatal("non-future skipped")
		}
		checkUnchanged(t, s, after)
	}
	skipped, e := s.Skip(id, day("2026-07-22"), day("2026-07-20"), at(101), ids)
	if e != nil {
		t.Fatal(e)
	}
	tr, e := skipped.Track(id, day("2026-07-20"))
	if e != nil || tr.Days[2].State() != Skipped || tr.Days[2].NavigationTarget() != nil {
		t.Fatal(tr, e)
	}
	stopped, n, e := skipped.Stop(id, day("2026-07-20"), at(102), ids)
	if e != nil || n != 3 {
		t.Fatal(n, e)
	}
	if stopped.pendingTrace(stopped.memberChain(id, day("2026-07-20")).ID) == nil {
		t.Fatal("today cancelled")
	}
	if stopped.Series[0].Lifecycle(day("2026-07-19")).Kind != Stopped {
		t.Fatal("terminal fact ignored after clock moves backwards")
	}
	again, n, e := stopped.Stop(id, day("2026-07-20"), at(103), ids)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	checkUnchanged(t, stopped, again)
}
func TestStopKeepsTodayDeferredIntoFuture(t *testing.T) {
	s, id, ids := createFive(t)
	source := traceOn(t, s, id, day("2026-07-20"))
	for i := range s.Tasks.Traces {
		if s.Tasks.Traces[i].ID == source.ID {
			s.Tasks.Traces[i].Status = core.TraceDeferred
			s.Tasks.Traces[i].SettledAt = ptr(at(101))
			s.Tasks.Traces[i].ContentUpdatedAt = at(101)
		}
	}
	next := source
	newID, _ := nextID(ids)
	next.ID = core.DayTraceID(newID)
	next.Date = day("2026-07-21")
	next.Priority = 2
	next.CarriedFromTraceID = ptr(source.ID)
	next.CreatedAt = at(101)
	next.ContentUpdatedAt = at(101)
	s.Tasks.Traces = append(s.Tasks.Traces, next)
	valid(t, s)
	stopped, n, e := s.Stop(id, day("2026-07-20"), at(102), ids)
	if e != nil || n != 4 {
		t.Fatal(n, e)
	}
	pending := stopped.pendingTrace(source.ChainID)
	if pending == nil || pending.ID != next.ID {
		t.Fatal("today's deferred target cancelled")
	}
	valid(t, stopped)
}
func TestCompletionTargetWaitsForLockedPastDays(t *testing.T) {
	ids := idsFrom(0)
	s, id, e := (State{}).Create(CreateInput{Title: "两次完成", StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: EndCondition{Kind: CompletionTarget, Count: 2, LatestDate: day("2026-07-24")}, Today: day("2026-07-20"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		s.Tasks.Traces[i].Status = core.TraceCompleted
		s.Tasks.Traces[i].CompletedAt = ptr(at(101))
		s.Tasks.Traces[i].ContentUpdatedAt = at(101)
	}
	valid(t, s)
	unchanged, out, e := s.Reconcile(day("2026-07-22"), at(102), ids)
	if e != nil || out.StoppedSeriesCount != 0 {
		t.Fatal(out, e)
	}
	checkUnchanged(t, s, unchanged)
	for i := 0; i < 2; i++ {
		s.Tasks.Days[i].LockedAt = ptr(at(102))
		s.Tasks.Days[i].UpdatedAt = at(102)
	}
	done, out, e := s.Reconcile(day("2026-07-22"), at(103), ids)
	if e != nil || out.StoppedSeriesCount != 1 || out.CancelledOccurrenceCount != 3 {
		t.Fatal(out, e)
	}
	l := done.Series[0].Lifecycle(day("2026-07-19"))
	if l.Kind != Ended || l.Date == nil || *l.Date != day("2026-07-21") {
		t.Fatal("target terminal authority lost")
	}
	track, e := done.Track(id, day("2026-07-22"))
	if e != nil || track.CompletedCount() != 2 || track.CompletionGoalCount() != 2 {
		t.Fatal(track, e)
	}
}
func TestFailureAtLateCancellationLeavesAllDaysUnchanged(t *testing.T) {
	s, id, ids := createFive(t)
	s.Tasks.Traces[4].ContentUpdatedAt = at(500)
	valid(t, s)
	after, n, e := s.Stop(id, day("2026-07-20"), at(200), ids)
	if !errors.Is(e, core.ErrInvalidClock) || n != 0 {
		t.Fatal(n, e)
	}
	checkUnchanged(t, s, after)
	locked := s.Clone()
	locked.Tasks.Days[2].LockedAt = ptr(at(100))
	after, e = locked.Skip(id, day("2026-07-22"), day("2026-07-20"), at(600), ids)
	if !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatal(e)
	}
	checkUnchanged(t, locked, after)
}
func TestContentUpdatesEditableInstancesOnlyAndPreservesSubtaskTombstones(t *testing.T) {
	ids := idsFrom(0)
	planned := []core.PlannedSubtask{{ID: core.PlannedSubtaskID(uid(900)), LineageID: core.SubtaskLineageID(uid(901)), Title: "原子项", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: at(100)}}
	s, id, e := (State{}).Create(CreateInput{Title: "旧标题", PlannedSubtasks: planned, StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: Until(day("2026-07-22")), Today: day("2026-07-20"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	// Synthetic persisted historical facts, not a claim that completion UI exists.
	s.Tasks.Traces[0].Status = core.TraceUnfinished
	s.Tasks.Traces[0].SettledAt = ptr(at(101))
	s.Tasks.Traces[0].ContentUpdatedAt = at(101)
	s.Tasks.Subtasks[0].Status = core.SubtaskUnfinished
	s.Tasks.Subtasks[0].SettledAt = ptr(at(101))
	s.Tasks.Subtasks[0].UpdatedAt = at(101)
	s.Tasks.Days[0].LockedAt = ptr(at(101))
	s.Tasks.Days[0].UpdatedAt = at(101)
	old := s.Clone()
	replacement := []core.PlannedSubtask{{ID: core.PlannedSubtaskID(uid(902)), LineageID: core.SubtaskLineageID(uid(903)), Title: " 新子项 ", Difficulty: core.DifficultyHard, Position: 9, CreatedAt: at(102)}}
	next, e := s.UpdateContent(ContentInput{SeriesID: id, Title: " 新标题 ", PlannedSubtasks: replacement, Today: day("2026-07-21"), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	valid(t, next)
	if !reflect.DeepEqual(old.Tasks.Traces[0], next.Tasks.Traces[0]) || !reflect.DeepEqual(old.Tasks.Definitions[0], next.Tasks.Definitions[0]) {
		t.Fatal("historical definition changed")
	}
	if next.Series[0].Title != "新标题" || next.Tasks.Definitions[1].Title != "新标题" {
		t.Fatal("template not applied")
	}
	cancelled := 0
	for _, sub := range next.Tasks.Subtasks {
		if sub.Status == core.SubtaskCancelledDraft {
			cancelled++
		}
	}
	if cancelled != 2 {
		t.Fatalf("missing append-only child cancellations: %d", cancelled)
	}
}
func TestTerminalContentEditRejectedButClassificationParentMayChange(t *testing.T) {
	s, id, ids := createFive(t)
	stopped, _, e := s.Stop(id, day("2026-07-20"), at(101), ids)
	if e != nil {
		t.Fatal(e)
	}
	after, e := stopped.UpdateContent(ContentInput{SeriesID: id, Title: "不允许", Today: day("2026-07-20"), Now: at(102)}, ids)
	if e == nil {
		t.Fatal("stopped content edited")
	}
	checkUnchanged(t, stopped, after)
	category := classification.CategoryID(uid(900))
	changed, e := stopped.SetTemplateClassification(ClassificationInput{SeriesID: id, CategoryID: &category, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: uid(901), Now: at(102)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(changed.Tasks, stopped.Tasks) || *changed.Series[0].CategoryID != category {
		t.Fatal("parent classification rewrote child facts")
	}
	if changed.ValidateCatalogReferences(nil, nil) == nil {
		t.Fatal("missing catalog accepted")
	}
	if e := changed.ValidateCatalogReferences(func(id classification.CategoryID) bool { return id == category }, nil); e != nil {
		t.Fatal(e)
	}
	again, e := changed.SetTemplateClassification(ClassificationInput{SeriesID: id, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: uid(901), Now: at(103)}, ids)
	if e == nil {
		t.Fatal("interaction reused")
	}
	checkUnchanged(t, changed, again)
}
func TestClassificationChildWinsTimestampTieRegardlessOfUUID(t *testing.T) {
	s, id, _ := createFive(t)
	p := &s.Series[0]
	p.ClassificationRevisions[0].ID = uid(999)
	category := classification.CategoryID(uid(900))
	next, e := s.SetTemplateClassification(ClassificationInput{SeriesID: id, CategoryID: &category, Source: classification.Source{Kind: classification.UserDirect}, InteractionID: uid(901), Now: at(100)}, idsFrom(0))
	if e != nil {
		t.Fatal(e)
	}
	current := next.Series[0].CurrentClassificationRevision()
	if current == nil || current.ID != uid(1) || current.CategoryID == nil {
		t.Fatal("UUID tie defeated child ancestry")
	}
	hostile := next.Clone()
	for i := range hostile.Series[0].ClassificationRevisions {
		if hostile.Series[0].ClassificationRevisions[i].ID == uid(999) {
			hostile.Series[0].ClassificationRevisions[i].PreviousRevisionID = ptr(uid(1))
		}
	}
	if hostile.Validate() == nil {
		t.Fatal("classification cycle accepted")
	}
}
func TestWeekdayTrackShowsOffScheduleDaysAndLifecycle(t *testing.T) {
	ids := idsFrom(0)
	s, id, e := (State{}).Create(CreateInput{Title: "工作日", StartDate: day("2026-07-20"), Schedule: Weekdays(), EndCondition: Until(day("2026-07-26")), Today: day("2026-07-19"), Now: at(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		today string
		kind  LifecycleKind
	}{{"2026-07-19", Upcoming}, {"2026-07-20", Active}, {"2026-07-27", Ended}} {
		track, e := s.Track(id, day(tc.today))
		if e != nil || track.Lifecycle.Kind != tc.kind || len(track.Days) != 7 || track.Days[5].State() != NotScheduled || track.Days[6].State() != NotScheduled {
			t.Fatal(track, e)
		}
	}
}
func TestConversionAdoptsChainAndScheduledTrace(t *testing.T) {
	k, e := core.NewTaskKernel(core.TaskGraph{})
	if e != nil {
		t.Fatal(e)
	}
	if e = k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(uid(900)), DefinitionID: core.TaskDefinitionID(uid(901)), Title: "转重复", Now: at(99)}); e != nil {
		t.Fatal(e)
	}
	for _, scheduled := range []bool{false, true} {
		graph := k.Snapshot()
		if scheduled {
			k2, _ := core.NewTaskKernel(graph)
			e = k2.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(uid(900)), TraceID: core.DayTraceID(uid(902)), DayID: core.DayID(uid(903)), Date: day("2026-07-20"), Today: day("2026-07-20"), Now: at(100)})
			if e != nil {
				t.Fatal(e)
			}
			graph = k2.Snapshot()
		}
		s, id, e := (State{Tasks: graph}).Convert(ConvertInput{ChainID: core.TaskChainID(uid(900)), StartDate: day("2026-07-20"), Schedule: Daily(), EndCondition: Until(day("2026-07-22")), Today: day("2026-07-20"), Now: at(101)}, idsFrom(0))
		if e != nil {
			t.Fatal(e)
		}
		valid(t, s)
		if len(s.Tasks.Chains) != 3 || s.memberChain(id, day("2026-07-20")).ID != core.TaskChainID(uid(900)) {
			t.Fatal("source chain replaced")
		}
		if scheduled && s.pendingTrace(core.TaskChainID(uid(900))).ID != core.DayTraceID(uid(902)) {
			t.Fatal("source trace replaced")
		}
	}
}
func TestStateRejectsMissingParentDuplicateOccurrenceAndCorruptHistory(t *testing.T) {
	cases := []func(*State){
		func(s *State) { s.Series = nil },
		func(s *State) {
			s.Tasks.Chains[1].CycleMembership.OccurrenceDate = s.Tasks.Chains[0].CycleMembership.OccurrenceDate
		},
		func(s *State) { s.Tasks.Traces = s.Tasks.Traces[1:] },
		func(s *State) { s.Series[0].PlanRevisions = nil },
		func(s *State) { s.Series[0].PlanRevisions[0].RecordedAt = at(99) },
		func(s *State) { s.Series[0].ClassificationRevisions[0].InteractionID = "bad" },
		func(s *State) { s.Series[0].EndDate = day("2027-07-30") },
		func(s *State) {
			s.Series[0].CancellationFacts = []CancellationFact{{ID: uid(900), Scope: Occurrence, Date: day("2026-07-27"), Reason: UserSkipped, RecordedAt: at(100)}}
		},
	}
	for i, change := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, _, _ := createFive(t)
			change(&s)
			if s.Validate() == nil {
				t.Fatal("invalid state accepted")
			}
		})
	}
}
func TestFailedIdentityAllocationAndNonFiniteClockDoNotMutate(t *testing.T) {
	s, id, _ := createFive(t)
	failure := errors.New("entropy unavailable")
	after, _, e := s.Stop(id, day("2026-07-20"), at(101), func() (string, error) { return "", failure })
	if !errors.Is(e, failure) {
		t.Fatal(e)
	}
	checkUnchanged(t, s, after)
	after, e = s.Skip(id, day("2026-07-21"), day("2026-07-20"), core.Instant(math.Float64bits(math.NaN())), idsFrom(100))
	if e == nil {
		t.Fatal("NaN accepted")
	}
	checkUnchanged(t, s, after)
}
func FuzzScheduleDoesNotPanic(f *testing.F) {
	f.Add(1, 1, 2)
	f.Add(30, 4, 7)
	f.Fuzz(func(t *testing.T, interval, weeks, weekday int) {
		s := Schedule{Kind: EveryDays, DayInterval: interval}
		_ = s.Includes(day("2026-07-20"), day("2026-07-19"))
		s = Schedule{Kind: SelectedWeekdays, EveryWeeks: weeks, Weekdays: []int{weekday}}
		_ = s.Includes(day("2026-07-20"), day("2026-07-19"))
	})
}
