// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
)

func prepareCarryover(t *testing.T, continuation bool) (*Service, string, core.DayTrace) {
	t.Helper()
	s, path := fresh(t)
	must(t, s.AddToday("合成迁延"))
	tr := s.Snapshot().Traces[0]
	if continuation {
		next, _ := testDay.AddDays(1)
		must(t, s.AdvanceTo(next))
	}
	return s, path, tr
}
func callCarryover(s *Service, tr core.DayTrace, target core.LocalDate, continuation bool) (core.DayTraceID, error) {
	if continuation {
		return s.ContinueUnfinishedTrace(tr.ChainID, tr.ID, target)
	}
	return s.DeferCurrentTrace(tr.ChainID, tr.ID, target)
}
func TestCarryoverServiceLifecycle(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			t.Run(map[bool]string{false: "defer", true: "continue"}[continuation]+map[bool]string{false: "-newday", true: "-existingday"}[existing], func(t *testing.T) {
				s, path := fresh(t)
				must(t, s.AddToday("合成继承"))
				tr := s.Snapshot().Traces[0]
				open, e := s.AddSubtask(tr.ChainID, tr.ID, "未完成子项", core.DifficultyHard)
				must(t, e)
				done, e := s.AddSubtask(tr.ChainID, tr.ID, "已完成子项", core.DifficultySimple)
				must(t, e)
				must(t, s.SetSubtaskCompleted(tr.ChainID, tr.ID, done, true))
				if continuation {
					next, _ := testDay.AddDays(1)
					must(t, s.AdvanceTo(next))
				}
				target, _ := s.Today().AddDays(1)
				if existing {
					must(t, s.AddOnDate("其他任务", target))
				}
				before := s.Snapshot()
				priorAudit := s.snapshot.Object["classifications"]
				rev := s.disk.revision
				today := s.Today()
				s.clock = func() time.Time { return time.Unix(978307201, 0) }
				id, e := callCarryover(s, tr, target, continuation)
				must(t, e)
				if id == "" || s.disk.revision != rev+1 || s.Today() != today {
					t.Fatal("missing single confirmed commit")
				}
				after := s.Snapshot()
				var src, dst core.DayTrace
				for _, v := range after.Traces {
					if v.ID == tr.ID {
						src = v
					}
					if v.ID == id {
						dst = v
					}
				}
				if dst.CarriedFromTraceID == nil || *dst.CarriedFromTraceID != tr.ID || dst.Date != target || dst.Status != core.TracePending {
					t.Fatal("trace lineage")
				}
				if !dst.CreatedAt.After(before.Traces[0].ContentUpdatedAt) {
					t.Fatal("nonmonotonic clock")
				}
				if continuation {
					if !reflect.DeepEqual(src, before.Traces[0]) || dst.ContinuationSeq != src.ContinuationSeq+1 || !historyEqual(priorAudit, s.snapshot.Object["classifications"]) {
						t.Fatal("continuation changed source/audit")
					}
				} else {
					if src.Status != core.TraceDeferred || src.SettledAt == nil || *src.SettledAt != src.ContentUpdatedAt {
						t.Fatal("deferral terminal facts")
					}
				}
				count := 0
				for _, sub := range after.Subtasks {
					if sub.TraceID == id {
						count++
						if sub.CarriedFromSubtaskID == nil || *sub.CarriedFromSubtaskID != open || sub.ID == open || sub.LineageID != before.Subtasks[0].LineageID || sub.Position != 1 || sub.Status != core.SubtaskPending {
							t.Fatal("child lineage")
						}
					}
				}
				if count != 1 {
					t.Fatal("copied completed child or missing open child")
				}
				must(t, ValidateEmptyHistory(s.snapshot))
				saved := futureAggregate(t, s)
				if got, e := callCarryover(s, tr, target, continuation); e == nil || got != "" {
					t.Fatal("repeat succeeded")
				}
				if string(saved) != string(futureAggregate(t, s)) {
					t.Fatal("repeat mutated")
				}
				must(t, s.Close())
				r, e := Open(path, today)
				must(t, e)
				defer r.Close()
				if !reflect.DeepEqual(after, r.Snapshot()) {
					t.Fatal("reopen changed facts")
				}
				later, _ := target.AddDays(1)
				must(t, r.AdvanceTo(later))
				must(t, ValidateEmptyHistory(r.snapshot))
				next, e := r.ContinueUnfinishedTrace(dst.ChainID, dst.ID, later)
				must(t, e)
				if next == "" {
					t.Fatal("continued chain lost id")
				}
			})
		}
	}
}
func TestCarryoverServiceFailuresNeverPublish(t *testing.T) {
	for _, cont := range []bool{false, true} {
		for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
			t.Run(map[bool]string{false: "defer", true: "continue"}[cont]+phase, func(t *testing.T) {
				s, path, tr := prepareCarryover(t, cont)
				target, _ := s.Today().AddDays(1)
				today := s.Today()
				before := futureAggregate(t, s)
				rev := s.disk.revision
				s.disk.fault = func(at string) error {
					if at == phase {
						return ErrIncomplete
					}
					return nil
				}
				id, e := callCarryover(s, tr, target, cont)
				if !errors.Is(e, ErrIncomplete) || id != "" {
					t.Fatal("false acknowledgement", id, e)
				}
				if string(before) != string(futureAggregate(t, s)) || s.disk.revision != rev || s.Today() != today {
					t.Fatal("failure published")
				}
				s.disk.fault = nil
				if phase != "before-reserve" {
					if id, e = callCarryover(s, tr, target, cont); e == nil || id != "" {
						t.Fatal("uncertain handle reused")
					}
				}
				must(t, s.Close())
				r, e := Open(path, today)
				if phase == "after-reserve" || phase == "before-commit" {
					if e == nil {
						r.Close()
						t.Fatal("incomplete reopened")
					}
					return
				}
				must(t, e)
				defer r.Close()
				n := 1
				if phase == "after-persist" {
					n = 2
				}
				if len(r.Snapshot().Traces) != n {
					t.Fatal("durable outcome wrong")
				}
				must(t, ValidateEmptyHistory(r.snapshot))
			})
		}
	}
}
func TestCarryoverServiceGuards(t *testing.T) {
	for _, cont := range []bool{false, true} {
		for _, mode := range []string{"wrong-owner", "wrong-trace", "past", "invalid-date", "closed", "blocked", "wrong-state"} {
			t.Run(map[bool]string{false: "defer", true: "continue"}[cont]+mode, func(t *testing.T) {
				s, _, tr := prepareCarryover(t, cont)
				target, _ := s.Today().AddDays(1)
				switch mode {
				case "wrong-owner":
					tr.ChainID = "00000000-0000-4000-8000-000000000099"
				case "wrong-trace":
					tr.ID = "00000000-0000-4000-8000-000000000099"
				case "past":
					target, _ = testDay.AddDays(-1)
				case "invalid-date":
					target = core.LocalDate{}
				case "closed":
					must(t, s.Close())
				case "blocked":
					s.disk.blocked = true
				case "wrong-state":
					if cont {
						cont = false
					} else {
						must(t, s.SetCompleted(tr.ID, true))
					}
				}
				before := futureAggregate(t, s)
				rev := s.disk.revision
				today := s.Today()
				id, e := callCarryover(s, tr, target, cont)
				if e == nil || id != "" {
					t.Fatal("guard accepted")
				}
				if string(before) != string(futureAggregate(t, s)) || s.disk.revision != rev || s.Today() != today {
					t.Fatal("guard mutated")
				}
			})
		}
	}
}
func TestCarryoverServiceConcurrentWinner(t *testing.T) {
	for _, cont := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[cont], func(t *testing.T) {
			s, path, tr := prepareCarryover(t, cont)
			other, e := Open(path, s.Today())
			must(t, e)
			defer other.Close()
			target, _ := s.Today().AddDays(1)
			var wg sync.WaitGroup
			wg.Add(2)
			ids := make([]core.DayTraceID, 2)
			errs := make([]error, 2)
			for i, h := range []*Service{s, other} {
				go func(i int, h *Service) { defer wg.Done(); ids[i], errs[i] = callCarryover(h, tr, target, cont) }(i, h)
			}
			wg.Wait()
			wins := 0
			for i, e := range errs {
				if e == nil {
					wins++
					if ids[i] == "" {
						t.Fatal("winner missing id")
					}
				} else if ids[i] != "" {
					t.Fatal("loser acknowledged")
				}
			}
			if wins != 1 {
				t.Fatal("not exactly one winner", errs)
			}
			r, e := Open(path, s.Today())
			must(t, e)
			defer r.Close()
			if len(r.Snapshot().Traces) != 2 {
				t.Fatal("lost or duplicate trace")
			}
		})
	}
}

func TestCarryoverServiceNotesProgressAndExtremeClock(t *testing.T) {
	for _, cont := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[cont], func(t *testing.T) {
			s, path := fresh(t)
			ids, e := newIDs(3)
			must(t, e)
			chain := core.TaskChainID(ids[0])
			description := "合成描述"
			must(t, s.transact(func(k *core.TaskKernel, now core.Instant) error {
				return k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(ids[1]), Title: "合成札记", DescriptionText: &description, InitialNote: &core.TaskNoteEntry{ID: core.TaskNoteEntryID(ids[2]), Body: "合成备注", CreatedAt: now, UpdatedAt: now}, Now: now})
			}))
			must(t, s.Schedule(chain, testDay))
			tr := s.Snapshot().Traces[0]
			must(t, s.SetManualProgress(chain, tr.ID, 37))
			if cont {
				next, _ := testDay.AddDays(1)
				must(t, s.AdvanceTo(next))
			}
			target, _ := s.Today().AddDays(1)
			id, e := callCarryover(s, tr, target, cont)
			must(t, e)
			var dst core.DayTrace
			for _, v := range s.Snapshot().Traces {
				if v.ID == id {
					dst = v
				}
			}
			if dst.ManualProgressPercent == nil || *dst.ManualProgressPercent != 37 || dst.DescriptionText == nil || *dst.DescriptionText != description || !reflect.DeepEqual(dst.NoteEntries, tr.NoteEntries) {
				t.Fatal("note/description/progress lost")
			}
			today := s.Today()
			must(t, s.Close())
			r, e := Open(path, today)
			must(t, e)
			defer r.Close()
			if !reflect.DeepEqual(s.Snapshot(), r.Snapshot()) {
				t.Fatal("notes reopen changed")
			}
		})
	}
	for _, cont := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer-max", true: "continue-max"}[cont], func(t *testing.T) {
			s, _, tr := prepareCarryover(t, cont)
			// A separate live task supplies an exhausted global frontier through a real
			// audited manual-progress command, even when carryover source is immutable.
			must(t, s.AddToday("合成极值旁项"))
			before := s.Snapshot()
			live := before.Traces[len(before.Traces)-1]
			now, e := core.InstantFromSeconds(math.MaxFloat64)
			must(t, e)
			cmd := core.SetManualProgressInput{ChainID: live.ChainID, TraceID: live.ID, Percent: 1, Today: s.Today(), Now: now}
			k, e := core.NewTaskKernel(before)
			must(t, e)
			must(t, k.SetManualProgress(cmd))
			wire, e := captureManualProgressHistory(s.snapshot, before, k.Snapshot(), cmd)
			must(t, e)
			wire, g, e := normalizeSnapshot(wire)
			must(t, e)
			must(t, s.disk.commit(wire))
			s.graph, s.snapshot = g, wire
			aggregate := futureAggregate(t, s)
			rev := s.disk.revision
			target, _ := s.Today().AddDays(1)
			id, e := callCarryover(s, tr, target, cont)
			if !errors.Is(e, core.ErrInvalidClock) || id != "" || s.disk.revision != rev || string(aggregate) != string(futureAggregate(t, s)) {
				t.Fatal("exhausted clock mutated or acknowledged", e)
			}
		})
	}
}

func TestCarryoverServiceUnsupportedPriorNeverCommits(t *testing.T) {
	for _, cont := range []bool{false, true} {
		for _, field := range []string{"categories", "labels", "currentByChainID", "relationHistory", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones", "committedReceiptsByInteractionID", "changeRecords", "ideas", "taskCycleSeries"} {
			t.Run(map[bool]string{false: "defer-", true: "continue-"}[cont]+field, func(t *testing.T) {
				s, _, tr := prepareCarryover(t, cont)
				bad, e := inspectEmptyHistory(s.snapshot)
				must(t, e)
				if field == "ideas" || field == "taskCycleSeries" {
					v := bad.Object[field]
					v.Array = append(v.Array, historyString("unsupported"))
					bad.Object[field] = v
				} else {
					c := bad.Object["classifications"]
					v := c.Object[field]
					v.Array = append(v.Array, historyString("unsupported"))
					c.Object[field] = v
					bad.Object["classifications"] = c
				}
				s.snapshot = bad
				rev := s.disk.revision
				before := s.Snapshot()
				target, _ := s.Today().AddDays(1)
				id, e := callCarryover(s, tr, target, cont)
				if e == nil || id != "" || s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) {
					t.Fatal("unsupported supplement committed")
				}
			})
		}
	}
}
