// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

func TestReviewerGenericRejectsArbitrarySupersession(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("synthetic"))
	before := s.Snapshot()
	after := before.Clone()
	now, e := s.nextInstant()
	must(t, e)
	ids, e := newIDs(1)
	must(t, e)
	next := after.Definitions[0]
	next.ID = core.TaskDefinitionID(ids[0])
	next.Sequence++
	next.Title = "forged successor"
	next.CreatedAt = now
	next.ContentUpdatedAt = now
	after.Definitions[0].SupersededAt = &now
	after.Definitions[0].SupersededByDefinitionID = &next.ID
	after.Definitions[0].ContentUpdatedAt = now
	after.Definitions = append(after.Definitions, next)
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic capture accepted arbitrary new definition/supersession with unchanged pending trace")
	}
}

func TestReviewerGenericRejectsReturnedDefinitionRewrite(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("synthetic"))
	tr := s.Snapshot().Traces[0]
	must(t, s.ReturnToPool(tr.ChainID, tr.ID))
	before := s.Snapshot()
	after := before.Clone()
	after.Definitions[0].Title = "forged historical title"
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic capture rewrote old definition referenced by unlocked returned trace")
	}
}

// Synthetic fixture only: no production sources or service files are read.
func reviewerSeedChildren(t *testing.T, s *Service) {
	g := s.Snapshot()
	tr := &g.Traces[0]
	ids, e := newIDs(6)
	must(t, e)
	at := tr.ContentUpdatedAt
	tr.NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(ids[4]), Body: "live", CreatedAt: at, UpdatedAt: at}, {ID: core.TaskNoteEntryID(ids[5]), CreatedAt: at, UpdatedAt: at, DeletedAt: &at}}
	for i := 0; i < 2; i++ {
		g.Subtasks = append(g.Subtasks, core.Subtask{ID: core.SubtaskID(ids[i]), LineageID: core.SubtaskLineageID(ids[2+i]), TraceID: tr.ID, Title: "synthetic child", Status: core.SubtaskPending, Difficulty: core.DifficultySimple, Position: i + 1, CreatedAt: at, UpdatedAt: at})
	}
	_, extra, e := v7.ToDomain(s.snapshot)
	must(t, e)
	wire, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	wire, g, e = normalizeSnapshot(wire)
	must(t, e)
	must(t, s.disk.commit(wire))
	s.snapshot = wire
	s.graph = g
}
func TestReviewerGenericRejectsTerminalChildren(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "today", true: "future"}[future], func(t *testing.T) {
			s, _ := fresh(t)
			day := testDay
			if future {
				day, _ = day.AddDays(1)
			}
			must(t, s.AddOnDate("synthetic", day))
			reviewerSeedChildren(t, s)
			tr := s.Snapshot().Traces[0]
			must(t, s.ReturnToPool(tr.ChainID, tr.ID))
			before := s.Snapshot()
			after := before.Clone()
			after.Subtasks[0].Title = "forged terminal child"
			if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
				t.Fatal("generic capture rewrote terminal child on unlocked day")
			}
		})
	}
}
func TestReviewerReturnRetainedChildrenRescheduleReopen(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "today", true: "future"}[future], func(t *testing.T) {
			s, p := fresh(t)
			day := testDay
			if future {
				day, _ = day.AddDays(2)
			}
			must(t, s.AddOnDate("synthetic", day))
			reviewerSeedChildren(t, s)
			before := s.Snapshot()
			tr := before.Traces[0]
			must(t, s.ReturnToPool(tr.ChainID, tr.ID))
			returned := s.Snapshot()
			if !reflect.DeepEqual(returned.Chains[0].NoteEntries, before.Traces[0].NoteEntries) {
				t.Fatal("notes/tombstones not copied exactly")
			}
			must(t, s.Schedule(tr.ChainID, testDay))
			g := s.Snapshot()
			if len(g.Subtasks) != 4 || len(g.Traces[1].NoteEntries) != 1 {
				t.Fatal("reschedule children/active notes")
			}
			for i := 0; i < 2; i++ {
				if g.Subtasks[i+2].ID == g.Subtasks[i].ID || g.Subtasks[i+2].LineageID != g.Subtasks[i].LineageID || g.Subtasks[i+2].CarriedFromSubtaskID != nil {
					t.Fatal("reschedule identity/lineage")
				}
			}
			must(t, s.Close())
			r, e := Open(p, testDay)
			must(t, e)
			defer r.Close()
			if !reflect.DeepEqual(g, r.Snapshot()) {
				t.Fatal("reopen mismatch")
			}
			oldEvents := r.snapshot.Object["classifications"]
			next, _ := day.AddDays(1)
			must(t, r.AdvanceTo(next))
			if !future {
				events := historyPairs(r.snapshot.Object["classifications"].Object["snapshotEventsByTraceID"])
				original := historyPairs(oldEvents.Object["snapshotEventsByTraceID"])
				if !historyEqual(events[string(tr.ID)], original[string(tr.ID)]) {
					t.Fatal("return event clock changed on day lock")
				}
			}
		})
	}
}
func TestReviewerReturnConflictClosed(t *testing.T) {
	s, p := fresh(t)
	must(t, s.AddToday("synthetic"))
	tr := s.Snapshot().Traces[0]
	other, e := Open(p, testDay)
	must(t, e)
	defer other.Close()
	before := futureAggregate(t, other)
	must(t, s.ReturnToPool(tr.ChainID, tr.ID))
	if e = other.ReturnToPool(tr.ChainID, tr.ID); !errors.Is(e, ErrConflict) {
		t.Fatalf("got %v", e)
	}
	if string(before) != string(futureAggregate(t, other)) {
		t.Fatal("conflict published")
	}
	must(t, s.Close())
	if e = s.ReturnToPool(tr.ChainID, tr.ID); !errors.Is(e, ErrSession) {
		t.Fatalf("closed got %v", e)
	}
}
func TestReviewerReturnFutureWriteFailures(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, p := fresh(t)
			future, _ := testDay.AddDays(1)
			must(t, s.AddOnDate("synthetic", future))
			tr := s.Snapshot().Traces[0]
			before := futureAggregate(t, s)
			s.disk.fault = func(at string) error {
				if at == phase {
					return ErrIncomplete
				}
				return nil
			}
			if e := s.ReturnToPool(tr.ChainID, tr.ID); !errors.Is(e, ErrIncomplete) {
				t.Fatal(e)
			}
			if string(before) != string(futureAggregate(t, s)) {
				t.Fatal("fault published")
			}
			s.disk.fault = nil
			must(t, s.Close())
			r, e := Open(p, testDay)
			if phase == "after-reserve" || phase == "before-commit" {
				if e == nil {
					r.Close()
					t.Fatal("uncertain reopen succeeded")
				}
				return
			}
			must(t, e)
			defer r.Close()
			want := core.TracePending
			if phase == "after-persist" {
				want = core.TraceCancelledDraft
			}
			if r.Snapshot().Traces[0].Status != want {
				t.Fatal("durable state mismatch")
			}
			must(t, ValidateEmptyHistory(r.snapshot))
		})
	}
}
func TestReviewerReturnTamper(t *testing.T) {
	for _, mode := range []string{"capture", "revision", "missing-stream", "trace-clock"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.AddToday("synthetic"))
			tr := s.Snapshot().Traces[0]
			must(t, s.ReturnToPool(tr.ChainID, tr.ID))
			bad, e := inspectEmptyHistory(s.snapshot)
			must(t, e)
			c := bad.Object["classifications"]
			switch mode {
			case "capture":
				c.Object["snapshotsByTraceID"].Array[1].Object["capturedAt"] = historyNumber(0)
				c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["capturedAt"] = historyNumber(0)
			case "revision":
				c.Object["revision"] = historyNumber(2)
			case "missing-stream":
				x := c.Object["snapshotEventsByTraceID"]
				x.Array = nil
				c.Object["snapshotEventsByTraceID"] = x
			case "trace-clock":
				bad.Object["traces"].Array[0].Object["settledAt"] = historyNumber(0)
			}
			bad.Object["classifications"] = c
			if ValidateEmptyHistory(bad) == nil {
				t.Fatal("tamper accepted")
			}
		})
	}
}

func TestReviewerFutureCancellationSharesLockedDayWithoutEvent(t *testing.T) {
	s, p := fresh(t)
	future, _ := testDay.AddDays(2)
	must(t, s.AddOnDate("return synthetic", future))
	tr := s.Snapshot().Traces[0]
	must(t, s.ReturnToPool(tr.ChainID, tr.ID))
	cancelled := s.Snapshot().Traces[0]
	must(t, s.AddOnDate("remaining synthetic", future))
	next, _ := future.AddDays(1)
	must(t, s.AdvanceTo(next))
	g := s.Snapshot()
	if g.Days[0].LockedAt == nil || !reflect.DeepEqual(g.Traces[0], cancelled) {
		t.Fatal("cancelled trace or day lock changed incorrectly")
	}
	if historyUint(s.snapshot.Object["classifications"].Object["revision"]) != 1 {
		t.Fatal("cancelled trace captured event")
	}
	must(t, s.Close())
	r, e := Open(p, next)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(g, r.Snapshot()) {
		t.Fatal("reopen changed locked cancellation")
	}
}
