// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func subtaskServiceFixture(t *testing.T, planned bool) (*Service, string, core.TaskChainID, core.TaskDefinitionID, core.DayTraceID, core.PlannedSubtaskID, core.SubtaskID) {
	t.Helper()
	s, path := fresh(t)
	var trace core.DayTraceID
	if planned {
		must(t, s.AddPool("合成父项"))
	} else {
		must(t, s.AddToday("合成父项"))
		trace = s.graph.Traces[0].ID
	}
	chain, def := s.graph.Chains[0].ID, s.graph.Definitions[0].ID
	var p core.PlannedSubtaskID
	var a core.SubtaskID
	var err error
	if planned {
		p, err = s.AddPlannedSubtask(chain, def, "cafe\u0301", core.DifficultyMedium)
	} else {
		a, err = s.AddSubtask(chain, trace, "cafe\u0301", core.DifficultyMedium)
	}
	must(t, err)
	return s, path, chain, def, trace, p, a
}
func TestSubtaskServicePoolLifecycleScheduleAndReopen(t *testing.T) {
	s, path, ch, def, _, p, _ := subtaskServiceFixture(t, true)
	p2, e := s.AddPlannedSubtask(ch, def, "第二项", core.DifficultyHard)
	must(t, e)
	got, e := s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, "  新计划\r\n内容 ")
	must(t, e)
	if got != "新计划\n内容" {
		t.Fatal("wrong acknowledged bytes")
	}
	must(t, s.SetPlannedSubtaskDifficulty(ch, def, p, core.DifficultySimple))
	must(t, s.RemovePlannedSubtask(ch, def, p2))
	plan := s.graph.Definitions[0].PlannedSubtasks[0]
	if len(s.graph.Definitions) != 1 || plan.ID != p || plan.Position != 1 {
		t.Fatal("pool edit versioned or lost identity")
	}
	must(t, s.Schedule(ch, testDay))
	g := s.Snapshot()
	tr := g.Traces[0]
	sub := g.Subtasks[0]
	if sub.LineageID != plan.LineageID || sub.Title != plan.Title || sub.Difficulty != plan.Difficulty {
		t.Fatal("materialization lost plan")
	}
	before := futureAggregate(t, s)
	if _, e = s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, "旧池回调"); e == nil {
		t.Fatal("scheduled pool still editable")
	}
	if string(before) != string(futureAggregate(t, s)) {
		t.Fatal("stale pool published")
	}
	must(t, s.SetSubtaskCompleted(ch, tr.ID, sub.ID, true))
	if s.graph.Traces[0].Status != core.TracePending {
		t.Fatal("auto-completed parent")
	}
	got, e = s.SaveSubtaskTitleAcknowledged(ch, tr.ID, sub.ID, "已完成但父项待办")
	must(t, e)
	if got != "已完成但父项待办" {
		t.Fatal("completed child title rejected")
	}
	must(t, s.SetSubtaskDifficulty(ch, tr.ID, sub.ID, core.DifficultyHard))
	must(t, s.SetCompleted(tr.ID, true))
	if e = s.SetSubtaskCompleted(ch, tr.ID, sub.ID, false); !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatal("completed parent child editable", e)
	}
	must(t, s.SetCompleted(tr.ID, false))
	must(t, s.SetSubtaskCompleted(ch, tr.ID, sub.ID, false))
	saved := s.Snapshot()
	must(t, s.Close())
	r, e := Open(path, testDay)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("reopen changed exact child facts")
	}
}
func TestSubtaskServiceReturnRescheduleAndOldOwners(t *testing.T) {
	s, path, ch, _, tr, _, a := subtaskServiceFixture(t, false)
	discarded, e := s.AddSubtask(ch, tr, "取消子项", core.DifficultySimple)
	must(t, e)
	must(t, s.DeleteSubtask(ch, tr, discarded))
	before := s.Snapshot()
	must(t, s.ReturnToPool(ch, tr))
	g := s.Snapshot()
	current := g.Definitions[len(g.Definitions)-1]
	if len(current.PlannedSubtasks) != 1 || current.PlannedSubtasks[0].LineageID != before.Subtasks[0].LineageID {
		t.Fatal("return included cancelled child or lost lineage")
	}
	oldBytes := futureAggregate(t, s)
	if _, e = s.SaveSubtaskTitleAcknowledged(ch, tr, a, "旧实际回调"); e == nil {
		t.Fatal("returned child editable")
	}
	if string(oldBytes) != string(futureAggregate(t, s)) {
		t.Fatal("stale actual published")
	}
	_, e = s.SavePlannedSubtaskTitleAcknowledged(ch, current.ID, current.PlannedSubtasks[0].ID, "新池计划")
	must(t, e)
	tomorrow, _ := testDay.AddDays(1)
	must(t, s.Schedule(ch, tomorrow))
	newTrace := s.graph.Traces[1]
	newChild := s.graph.Subtasks[len(s.graph.Subtasks)-1]
	if newChild.ID == a || newChild.LineageID != before.Subtasks[0].LineageID || newChild.Title != "新池计划" {
		t.Fatal("reschedule identity")
	}
	must(t, s.SetSubtaskDifficulty(ch, newTrace.ID, newChild.ID, core.DifficultyHard))
	if e = s.SetSubtaskCompleted(ch, newTrace.ID, newChild.ID, true); !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatal("future child completed", e)
	}
	must(t, s.AdvanceTo(tomorrow))
	must(t, s.SetSubtaskCompleted(ch, newTrace.ID, newChild.ID, true))
	must(t, ValidateEmptyHistory(s.snapshot))
	saved := s.Snapshot()
	must(t, s.Close())
	r, e := Open(path, tomorrow)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("return/settlement reopen")
	}
}
func TestSubtaskNoopAcknowledgesStoredBytesAndVerifiesPrefix(t *testing.T) {
	for _, planned := range []bool{false, true} {
		t.Run(fmt.Sprint(planned), func(t *testing.T) {
			s, path, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
			edit := func(title string) (string, error) {
				if planned {
					return s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, title)
				}
				return s.SaveSubtaskTitleAcknowledged(ch, tr, a, title)
			}
			before := s.Snapshot()
			rev := s.disk.revision
			ack, e := edit("café")
			must(t, e)
			if ack != "cafe\u0301" || s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("canonical no-op rewrote bytes or ack")
			}
			other, e := Open(path, testDay)
			must(t, e)
			defer other.Close()
			must(t, other.AddPool("外部新事实"))
			if ack, e = edit("café"); !errors.Is(e, ErrConflict) || ack != "" {
				t.Fatal("stale no-op acknowledged", ack, e)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev {
				t.Fatal("conflict published")
			}
		})
	}
}
func TestSubtaskExplicitCGJAndInvalidInputPersistence(t *testing.T) {
	for _, planned := range []bool{false, true} {
		t.Run(fmt.Sprint(planned), func(t *testing.T) {
			s, _, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
			edit := func(title string) (string, error) {
				if planned {
					return s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, title)
				}
				return s.SaveSubtaskTitleAcknowledged(ch, tr, a, title)
			}
			plain := strings.Repeat("\u0301", 31)
			with := strings.Repeat("\u0301", 30) + "\u034f\u0301"
			ack, e := edit(plain)
			must(t, e)
			if ack != plain {
				t.Fatal("initial bytes")
			}
			rev := s.disk.revision
			ack, e = edit(with)
			must(t, e)
			if ack != with || s.disk.revision != rev+1 {
				t.Fatal("explicit CGJ suppressed")
			}
			before := futureAggregate(t, s)
			for _, bad := range []string{" \n ", "\xff", "new\xc3"} {
				if ack, e = edit(bad); e == nil || ack != "" {
					t.Fatal("invalid title acknowledged")
				}
				if string(before) != string(futureAggregate(t, s)) {
					t.Fatal("invalid title wrote")
				}
			}
		})
	}
}
func TestSubtaskMutationFaultMatrix(t *testing.T) {
	for _, planned := range []bool{false, true} {
		for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
			t.Run(fmt.Sprint(planned)+"/"+phase, func(t *testing.T) {
				s, path, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
				before := s.Snapshot()
				wire := s.snapshot
				rev := s.disk.revision
				edit := func(title string) (string, error) {
					if planned {
						return s.SavePlannedSubtaskTitleAcknowledged(ch, def, p, title)
					}
					return s.SaveSubtaskTitleAcknowledged(ch, tr, a, title)
				}
				s.disk.fault = func(at string) error {
					if at == phase {
						return ErrIncomplete
					}
					return nil
				}
				ack, e := edit("未确认新子项")
				if !errors.Is(e, ErrIncomplete) || ack != "" {
					t.Fatal("fault ack", ack, e)
				}
				if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || s.disk.revision != rev {
					t.Fatal("uncertain publication")
				}
				s.disk.fault = nil
				if phase != "before-reserve" {
					if _, e = edit("café"); !errors.Is(e, ErrIncomplete) {
						t.Fatal("blocked no-op", e)
					}
				}
				must(t, s.Close())
				r, e := Open(path, testDay)
				if phase == "after-reserve" || phase == "before-commit" {
					if !errors.Is(e, ErrIncomplete) {
						if r != nil {
							r.Close()
						}
						t.Fatal("partial write reopened", e)
					}
					return
				}
				must(t, e)
				defer r.Close()
				expected := "cafe\u0301"
				if phase == "after-persist" {
					expected = "未确认新子项"
				}
				title := ""
				if planned {
					title = r.graph.Definitions[0].PlannedSubtasks[0].Title
				} else {
					title = r.graph.Subtasks[0].Title
				}
				if title != expected {
					t.Fatal("durable facts", title)
				}
			})
		}
	}
}
func TestSubtaskDeleteRetainsTombstoneAcrossReopenAndSettlement(t *testing.T) {
	s, path, ch, _, tr, _, a := subtaskServiceFixture(t, false)
	must(t, s.SetSubtaskCompleted(ch, tr, a, true))
	before := s.graph.Subtasks[0]
	must(t, s.DeleteSubtask(ch, tr, a))
	deleted := s.graph.Subtasks[0]
	if deleted.ID != before.ID || deleted.LineageID != before.LineageID || deleted.Title != before.Title || deleted.Position != before.Position || deleted.CreatedAt != before.CreatedAt || deleted.CompletedAt != nil || deleted.SettledAt == nil || deleted.DraftCancellationID == nil || deleted.Status != core.SubtaskCancelledDraft {
		t.Fatal("bad cancellation facts")
	}
	_, e := s.AddSubtask(ch, tr, "新的实际项", core.DifficultySimple)
	must(t, e)
	if s.graph.Subtasks[1].Position != 2 {
		t.Fatal("hidden slot reused")
	}
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
	if !reflect.DeepEqual(deleted, s.graph.Subtasks[0]) {
		t.Fatal("rollover changed cancellation")
	}
	saved := s.Snapshot()
	must(t, s.Close())
	r, e := Open(path, next)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("tombstone reopen")
	}
}
func TestSubtaskServicesRejectStaleWrongAndClosedRoutes(t *testing.T) {
	s, _, ch, def, tr, p, a := subtaskServiceFixture(t, false)
	unused := core.TaskChainID(titleID(99))
	before := futureAggregate(t, s)
	cases := []func() error{
		func() error { _, e := s.AddSubtask(unused, tr, "错父项", core.DifficultySimple); return e },
		func() error { _, e := s.SaveSubtaskTitleAcknowledged(unused, tr, a, "错父项"); return e },
		func() error { return s.DeleteSubtask(ch, core.DayTraceID(titleID(99)), a) },
		func() error { return s.SetSubtaskDifficulty(ch, tr, core.SubtaskID(titleID(99)), core.DifficultyHard) },
		func() error { _, e := s.AddPlannedSubtask(ch, def, "已有排期", core.DifficultySimple); return e },
		func() error { return s.RemovePlannedSubtask(ch, def, p) },
	}
	for _, f := range cases {
		if f() == nil {
			t.Fatal("bad route accepted")
		}
		if string(before) != string(futureAggregate(t, s)) {
			t.Fatal("bad route published")
		}
	}
	must(t, s.Close())
	if _, e := s.SaveSubtaskTitleAcknowledged(ch, tr, a, "café"); !errors.Is(e, ErrSession) {
		t.Fatal("closed noop accepted", e)
	}
}
func TestSubtaskNoopRejectsHistoricalDiskCorruption(t *testing.T) {
	s, path, ch, _, tr, _, a := subtaskServiceFixture(t, false)
	before := s.Snapshot()
	rev := s.disk.revision
	p := filepath.Join(path, "rev-0000000001", "state.json")
	b, e := os.ReadFile(p)
	must(t, e)
	b[len(b)/2] ^= 1
	must(t, os.WriteFile(p, b, 0600))
	if ack, e := s.SaveSubtaskTitleAcknowledged(ch, tr, a, "café"); e == nil || ack != "" {
		t.Fatal("corrupt prefix acknowledged")
	}
	if s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("corrupt noop published")
	}
}
func TestSubtaskExactReplayRejectsUnrelatedChanges(t *testing.T) {
	for _, planned := range []bool{false, true} {
		t.Run(fmt.Sprint(planned), func(t *testing.T) {
			s, _, ch, def, tr, p, a := subtaskServiceFixture(t, planned)
			before := s.Snapshot()
			now, e := s.nextInstant()
			must(t, e)
			cmd := subtaskEditCommand{}
			if planned {
				cmd.planned = &core.PlannedSubtaskEditInput{Kind: core.SubtaskTitle, ChainID: ch, ExpectedDefinitionID: def, SubtaskID: p, Title: "合法修改", Now: now}
			} else {
				cmd.actual = &core.SubtaskEditInput{Kind: core.SubtaskTitle, ChainID: ch, TraceID: tr, SubtaskID: a, Title: "合法修改", Today: testDay, Now: now}
			}
			k, e := core.NewTaskKernel(before)
			must(t, e)
			must(t, cmd.apply(k))
			after := k.Snapshot()
			good, e := captureSubtaskEditHistory(s.snapshot, before, after, cmd)
			must(t, e)
			if !historyEqual(good.Object["classifications"], s.snapshot.Object["classifications"]) {
				t.Fatal("classification changed")
			}
			attacks := []func(*core.TaskGraph){func(g *core.TaskGraph) { g.Chains[0].CreatedAt = now }, func(g *core.TaskGraph) { g.Definitions[0].Title = "隐藏改父标题" }, func(g *core.TaskGraph) { g.Definitions[0].Sequence++ }}
			if !planned {
				attacks = append(attacks, func(g *core.TaskGraph) { g.Subtasks[0].LineageID = core.SubtaskLineageID(titleID(99)) }, func(g *core.TaskGraph) { g.Traces[0].Priority++ }, func(g *core.TaskGraph) { g.Days[0].UpdatedAt = now })
			} else {
				attacks = append(attacks, func(g *core.TaskGraph) { g.Definitions[0].PlannedSubtasks[0].CreatedAt = now })
			}
			for _, f := range attacks {
				bad := after.Clone()
				f(&bad)
				if _, e = captureSubtaskEditHistory(s.snapshot, before, bad, cmd); e == nil {
					t.Fatal("forged after accepted")
				}
			}
			wrong := before.Clone()
			wrong.Definitions[0].Title = "错误before"
			if _, e = captureSubtaskEditHistory(s.snapshot, wrong, after, cmd); e == nil {
				t.Fatal("wrong before accepted")
			}
			forged, e := inspectEmptyHistory(s.snapshot)
			must(t, e)
			forged.Object["classifications"].Object["categories"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyObject(nil)}}
			if _, e = captureSubtaskEditHistory(forged, before, after, cmd); e == nil {
				t.Fatal("nonempty catalog bypassed")
			}
		})
	}
}
