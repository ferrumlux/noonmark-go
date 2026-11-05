// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestReturnCurrentFutureReopenAndSettle(t *testing.T) {
	for _, days := range []int{0, 2} {
		t.Run(map[int]string{0: "current", 2: "future"}[days], func(t *testing.T) {
			s, path := fresh(t)
			target, _ := testDay.AddDays(days)
			must(t, s.AddOnDate("合成退回", target))
			g := s.Snapshot()
			tr := g.Traces[0]
			rev := s.disk.revision
			must(t, s.ReturnToPool(tr.ChainID, tr.ID))
			got := s.Snapshot()
			if s.disk.revision != rev+1 || len(got.Definitions) != 2 {
				t.Fatal("not one aggregate")
			}
			expected := core.TraceReturnedToPool
			events := uint64(1)
			if days > 0 {
				expected = core.TraceCancelledDraft
				events = 0
			}
			if got.Traces[0].Status != expected || historyUint(s.snapshot.Object["classifications"].Object["revision"]) != events {
				t.Fatal("return semantics")
			}
			must(t, ValidateEmptyHistory(s.snapshot))
			must(t, s.Close())
			r, e := Open(path, testDay)
			must(t, e)
			defer r.Close()
			if !reflect.DeepEqual(got, r.Snapshot()) {
				t.Fatal("reopen changed graph")
			}
			before := futureAggregate(t, r)
			if r.ReturnToPool(tr.ChainID, tr.ID) == nil {
				t.Fatal("duplicate accepted")
			}
			if string(before) != string(futureAggregate(t, r)) {
				t.Fatal("duplicate wrote")
			}
			next, _ := target.AddDays(1)
			must(t, r.AdvanceTo(next))
			must(t, ValidateEmptyHistory(r.snapshot))
			if historyUint(r.snapshot.Object["classifications"].Object["revision"]) != events {
				t.Fatal("settlement duplicated return history")
			}
			must(t, r.Schedule(tr.ChainID, next))
			must(t, r.ReturnToPool(tr.ChainID, r.Snapshot().Traces[1].ID))
			must(t, r.SaveTaskTitleInput(tr.ChainID, "返回后改名"))
			must(t, r.Close())
			again, e := Open(path, next)
			must(t, e)
			defer again.Close()
			must(t, ValidateEmptyHistory(again.snapshot))
		})
	}
}
func TestReturnLateDayWrongOwnerAndCompleted(t *testing.T) {
	for _, mode := range []string{"late", "wrong-owner", "completed"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.AddToday("合成拒绝"))
			tr := s.Snapshot().Traces[0]
			chain := tr.ChainID
			switch mode {
			case "late":
				next, _ := testDay.AddDays(1)
				must(t, s.AdvanceTo(next))
			case "completed":
				must(t, s.SetCompleted(tr.ID, true))
			case "wrong-owner":
				chain = core.TaskChainID("00000000-0000-4000-8000-000000000099")
			}
			before := futureAggregate(t, s)
			rev := s.disk.revision
			if s.ReturnToPool(chain, tr.ID) == nil {
				t.Fatal("accepted")
			}
			if string(before) != string(futureAggregate(t, s)) || s.disk.revision != rev {
				t.Fatal("rejection mutated")
			}
		})
	}
}
func TestReturnWriteFailuresNeverPublish(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("合成故障"))
			tr := s.Snapshot().Traces[0]
			before := futureAggregate(t, s)
			rev := s.disk.revision
			s.disk.fault = func(at string) error {
				if at == phase {
					return ErrIncomplete
				}
				return nil
			}
			if e := s.ReturnToPool(tr.ChainID, tr.ID); !errors.Is(e, ErrIncomplete) {
				t.Fatal(e)
			}
			if string(before) != string(futureAggregate(t, s)) || s.disk.revision != rev {
				t.Fatal("failure published")
			}
			s.disk.fault = nil
			if phase == "before-reserve" {
				must(t, s.ReturnToPool(tr.ChainID, tr.ID))
			} else if s.ReturnToPool(tr.ChainID, tr.ID) == nil {
				t.Fatal("uncertain handle reused")
			}
			must(t, s.Close())
			r, e := Open(path, testDay)
			if phase == "after-reserve" || phase == "before-commit" {
				if e == nil {
					r.Close()
					t.Fatal("incomplete reopened")
				}
				return
			}
			must(t, e)
			defer r.Close()
			if r.Snapshot().Traces[0].Status != core.TraceReturnedToPool {
				t.Fatal("persisted result missing")
			}
			must(t, ValidateEmptyHistory(r.snapshot))
		})
	}
}
func TestReturnAuditRejectsForgedCandidateAndMissingEvent(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("合成审计"))
	before := s.Snapshot()
	tr := before.Traces[0]
	ids, e := newIDs(2)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	in := core.ReturnToPoolInput{TraceID: tr.ID, ChainID: tr.ChainID, NewDefinitionID: core.TaskDefinitionID(ids[0]), CancellationID: core.CancellationID(ids[1]), Today: testDay, Now: now}
	k, e := core.NewTaskKernel(before)
	must(t, e)
	must(t, k.ReturnToPool(in))
	after := k.Snapshot()
	if _, e = CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic capture accepted return")
	}
	good, e := captureReturnHistory(s.snapshot, before, after, in)
	must(t, e)
	for _, mode := range []string{"title", "day", "trace", "chain"} {
		t.Run(mode, func(t *testing.T) {
			bad := after.Clone()
			switch mode {
			case "title":
				bad.Definitions[0].Title = "伪造"
			case "day":
				bad.Days[0].UpdatedAt = now
			case "trace":
				bad.Traces[0].Priority++
			case "chain":
				bad.Chains[0].NoteEntries = []core.TaskNoteEntry{}
			}
			// A nil-to-empty note slice has identical encoding; use a clock difference instead.
			if mode == "chain" {
				bad.Chains[0].UpdatedAt = before.Chains[0].UpdatedAt
			}
			if _, e := captureReturnHistory(s.snapshot, before, bad, in); e == nil {
				t.Fatal("forged candidate accepted")
			}
		})
	}
	good.Object["classifications"] = s.snapshot.Object["classifications"]
	if ValidateEmptyHistory(good) == nil {
		t.Fatal("missing return audit accepted")
	}
}

func TestReturnGenericCaptureCannotRewriteTerminalFact(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("合成终态"))
	tr := s.Snapshot().Traces[0]
	must(t, s.ReturnToPool(tr.ChainID, tr.ID))
	before := s.Snapshot()
	after := before.Clone()
	description := "伪造说明"
	after.Traces[0].DescriptionText = &description
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic terminal rewrite accepted")
	}
}
