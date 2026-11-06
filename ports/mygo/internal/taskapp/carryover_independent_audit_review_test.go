// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

func auditReviewFixture(t *testing.T, continuation bool) (*Service, string, core.DayTraceID) {
	t.Helper()
	s, p := fresh(t)
	must(t, s.AddToday("独立合成父项"))
	tr := s.Snapshot().Traces[0]
	_, e := s.AddSubtask(tr.ChainID, tr.ID, "开放子项", core.DifficultyMedium)
	must(t, e)
	done, e := s.AddSubtask(tr.ChainID, tr.ID, "已完成子项", core.DifficultyHard)
	must(t, e)
	must(t, s.SetSubtaskCompleted(tr.ChainID, tr.ID, done, true))
	if continuation {
		next, _ := testDay.AddDays(1)
		must(t, s.AdvanceTo(next))
	}
	return s, p, tr.ID
}
func TestIndependentCarryoverExactReplay(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[continuation], func(t *testing.T) {
			s, _, _ := auditReviewFixture(t, continuation)
			before, after, in := carryoverInput(t, s, continuation)
			_, e := captureCarryoverHistory(s.snapshot, before, after, in, continuation)
			must(t, e)
			mutations := map[string]func(*core.TaskGraph){
				"day review":         func(g *core.TaskGraph) { v := "forged"; g.Days[len(g.Days)-1].ReviewSummary = &v },
				"chain clock":        func(g *core.TaskGraph) { g.Chains[0].UpdatedAt++ },
				"definition title":   func(g *core.TaskGraph) { g.Definitions[0].Title = "forged" },
				"source description": func(g *core.TaskGraph) { v := "forged"; g.Traces[0].DescriptionText = &v },
				"target progress":    func(g *core.TaskGraph) { v := 77; g.Traces[len(g.Traces)-1].ManualProgressPercent = &v },
				"child title":        func(g *core.TaskGraph) { g.Subtasks[len(g.Subtasks)-1].Title = "forged" },
				"child carry":        func(g *core.TaskGraph) { g.Subtasks[len(g.Subtasks)-1].CarriedFromSubtaskID = nil },
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					g := after.Clone()
					mutate(&g)
					if _, e := captureCarryoverHistory(s.snapshot, before, g, in, continuation); e == nil {
						t.Fatal("accepted extra fact")
					}
				})
			}
			badBefore := before.Clone()
			badBefore.Definitions[0].Title = "mismatched prior"
			if _, e := captureCarryoverHistory(s.snapshot, badBefore, after, in, continuation); e == nil {
				t.Fatal("accepted mismatched before")
			}
			wrong := in
			wrong.Now++
			if _, e := captureCarryoverHistory(s.snapshot, before, after, wrong, continuation); e == nil {
				t.Fatal("accepted altered input")
			}
			if _, e := captureCarryoverHistory(s.snapshot, before, after, in, !continuation); e == nil {
				t.Fatal("accepted wrong command")
			}
		})
	}
}
func TestIndependentCarryoverAuditReopen(t *testing.T) {
	s, p, source := auditReviewFixture(t, false)
	tr := s.Snapshot().Traces[0]
	target, _ := testDay.AddDays(1)
	id, e := s.DeferCurrentTrace(tr.ChainID, source, target)
	must(t, e)
	if id == "" {
		t.Fatal("empty acknowledgement")
	}
	prior, e := inspectEmptyHistory(s.snapshot)
	must(t, e)
	events := historyPairs(prior.Object["classifications"].Object["snapshotEventsByTraceID"])
	if len(events) != 1 || len(events[string(source)].Array) != 1 || events[string(source)].Array[0].Object["status"].String != "deferred" {
		t.Fatal("defer event cardinality/status")
	}
	must(t, s.AdvanceTo(target))
	later, _ := target.AddDays(1)
	must(t, s.AdvanceTo(later))
	before := s.Snapshot()
	audit, e := inspectEmptyHistory(s.snapshot)
	must(t, e)
	cid, e := s.ContinueUnfinishedTrace(tr.ChainID, id, later)
	must(t, e)
	if cid == "" {
		t.Fatal("empty continuation")
	}
	if !historyEqual(audit.Object["classifications"], s.snapshot.Object["classifications"]) {
		t.Fatal("continuation changed audit")
	}
	for _, old := range before.Traces {
		for _, now := range s.Snapshot().Traces {
			if old.ID == now.ID && !reflect.DeepEqual(old, now) {
				t.Fatal("continuation mutated prior trace")
			}
		}
	}
	saved, e := inspectEmptyHistory(s.snapshot)
	must(t, e)
	must(t, s.Close())
	r, e := Open(p, later)
	must(t, e)
	defer r.Close()
	if !historyEqual(saved, r.snapshot) {
		t.Fatal("reopen changed aggregate")
	}
	if !historyEqual(events[string(source)], historyPairs(r.snapshot.Object["classifications"].Object["snapshotEventsByTraceID"])[string(source)]) {
		t.Fatal("old deferred event changed")
	}
}
func TestIndependentDeferredAuditCorruption(t *testing.T) {
	s, _, source := auditReviewFixture(t, false)
	tr := s.Snapshot().Traces[0]
	target, _ := testDay.AddDays(1)
	_, e := s.DeferCurrentTrace(tr.ChainID, source, target)
	must(t, e)
	for name, mutate := range map[string]func(canonical.Value){
		"clock":    func(v canonical.Value) { historyMutateEvent(v, 0, "capturedAt", historyNumber(0)) },
		"status":   func(v canonical.Value) { historyMutateEvent(v, 0, "status", historyString("returnedToPool")) },
		"revision": func(v canonical.Value) { historyMutateEvent(v, 0, "revision", historyNumber(2)) },
		"missing": func(v canonical.Value) {
			v.Object["classifications"].Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array}
		},
		"extra": func(v canonical.Value) {
			c := v.Object["classifications"]
			a := c.Object["snapshotEventsByTraceID"]
			a.Array[1].Array = append(a.Array[1].Array, a.Array[1].Array[0])
			c.Object["snapshotEventsByTraceID"] = a
		},
		"settlement": func(v canonical.Value) {
			for _, x := range v.Object["traces"].Array {
				if historyID(x.Object["id"]) == string(source) {
					x.Object["settledAt"] = historyNumber(0)
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			v, e := inspectEmptyHistory(s.snapshot)
			must(t, e)
			mutate(v)
			if ValidateEmptyHistory(v) == nil {
				t.Fatal("corruption accepted")
			}
			if _, e := CaptureEmptyHistory(v, s.Snapshot(), s.Snapshot()); e == nil {
				t.Fatal("reconstructed audit")
			}
		})
	}
	for _, section := range []string{"traces", "definitions", "subtasks"} {
		t.Run("freeze "+section, func(t *testing.T) {
			b := s.Snapshot()
			a := b.Clone()
			switch section {
			case "traces":
				a.Traces[0].Priority++
			case "definitions":
				a.Definitions[0].Title = "forged"
			case "subtasks":
				a.Subtasks[0].Title = "forged"
			}
			if _, e := CaptureEmptyHistory(s.snapshot, b, a); e == nil {
				t.Fatal("terminal history mutated")
			}
		})
	}
}

func TestIndependentStrippedChildContinuationRejected(t *testing.T) {
	s, _, _ := auditReviewFixture(t, true)
	before, after, _ := carryoverInput(t, s, true)
	for i := range after.Traces {
		if after.Traces[i].CarriedFromTraceID != nil {
			after.Traces[i].CarriedFromTraceID = nil
			after.Traces[i].ContinuationSeq = 0
		}
	}
	for i := range after.Subtasks {
		after.Subtasks[i].CarriedFromSubtaskID = nil
	}
	if _, e := core.NewTaskKernel(after); e != nil {
		t.Fatalf("fixture must remain a valid graph: %v", e)
	}
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic audit accepted child-backed stripped non-pool continuation")
	}
}
