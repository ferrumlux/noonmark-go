// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func TestIndependentGenericCannotScheduleSamePoolChainTwice(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddPool("合成池项"))
	before := s.Snapshot()
	k, e := core.NewTaskKernel(before)
	must(t, e)
	ids, e := newIDs(4)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	next, _ := s.Today().AddDays(1)
	first := core.DayTraceID(ids[0])
	chain := before.Chains[0].ID
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: first, DayID: core.DayID(ids[1]), Date: s.Today(), Today: s.Today(), Now: now}))
	later, _ := core.NextMutationInstant(now, now)
	must(t, k.SettleTaskDays(next, later))
	last, _ := core.NextMutationInstant(later, later)
	must(t, k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: chain, TraceID: first, NewTraceID: core.DayTraceID(ids[2]), DayID: core.DayID(ids[3]), Today: next, TargetDate: next, Now: last, SubtaskIDs: map[core.SubtaskID]core.SubtaskID{}}))
	after := k.Snapshot()
	for i := range after.Traces {
		after.Traces[i].CarriedFromTraceID = nil
		after.Traces[i].ContinuationSeq = 0
	}
	if _, e := core.NewTaskKernel(after); e != nil {
		t.Fatalf("fixture core invalid: %v", e)
	}
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic eligibility reused one original pool membership for two schedules")
	}
}

func TestIndependentGenericScheduleRejectsOldDefinition(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("合成旧定义"))
	tr := s.Snapshot().Traces[0]
	must(t, s.ReturnToPool(tr.ChainID, tr.ID))
	before := s.Snapshot()
	if len(before.Definitions) < 2 {
		t.Fatal("fixture missing historical definition")
	}
	k, e := core.NewTaskKernel(before)
	must(t, e)
	ids, e := newIDs(2)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: tr.ChainID, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: s.Today(), Today: s.Today(), Now: now}))
	after := k.Snapshot()
	for i := range after.Traces {
		if after.Traces[i].ID == core.DayTraceID(ids[0]) {
			after.Traces[i].DefinitionID = tr.DefinitionID
		}
	}
	if _, e := core.NewTaskKernel(after); e != nil {
		t.Logf("core rejects wrong definition: %v", e)
		return
	}
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic eligibility accepted old-definition schedule")
	}
}

func TestIndependentGenericSchedulePlannedChildProof(t *testing.T) {
	s, _, chain, _, _, _, _ := subtaskServiceFixture(t, true)
	before := s.Snapshot()
	k, e := core.NewTaskKernel(before)
	must(t, e)
	ids, e := newIDs(4)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	plan := before.Definitions[0].PlannedSubtasks[0]
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: s.Today(), Today: s.Today(), Now: now, SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{plan.ID: core.SubtaskID(ids[2])}}))
	after := k.Snapshot()
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"missing child", "wrong lineage"} {
		t.Run(mode, func(t *testing.T) {
			bad := after.Clone()
			if mode == "missing child" {
				bad.Subtasks = nil
			} else {
				bad.Subtasks[0].LineageID = core.SubtaskLineageID(ids[3])
			}
			if _, e := core.NewTaskKernel(bad); e != nil {
				t.Fatalf("fixture invalid: %v", e)
			}
			if _, e := CaptureEmptyHistory(s.snapshot, before, bad); e == nil {
				t.Fatal("planned-child proof accepted mismatch")
			}
		})
	}
}

func TestIndependentGenericNewChainCannotHideContinuation(t *testing.T) {
	s, _ := fresh(t)
	before := s.Snapshot()
	k, e := core.NewTaskKernel(before)
	must(t, e)
	ids, e := newIDs(6)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	next, _ := s.Today().AddDays(1)
	first := core.DayTraceID(ids[0])
	chain := core.TaskChainID(ids[4])
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(ids[5]), Title: "合成新池链", Now: now}))
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: first, DayID: core.DayID(ids[1]), Date: s.Today(), Today: s.Today(), Now: now}))
	later, _ := core.NextMutationInstant(now, now)
	must(t, k.SettleTaskDays(next, later))
	last, _ := core.NextMutationInstant(later, later)
	must(t, k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: chain, TraceID: first, NewTraceID: core.DayTraceID(ids[2]), DayID: core.DayID(ids[3]), Today: next, TargetDate: next, Now: last, SubtaskIDs: map[core.SubtaskID]core.SubtaskID{}}))
	after := k.Snapshot()
	for i := range after.Traces {
		after.Traces[i].CarriedFromTraceID = nil
		after.Traces[i].ContinuationSeq = 0
	}
	if _, e := core.NewTaskKernel(after); e != nil {
		t.Fatalf("fixture invalid: %v", e)
	}
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic audit admitted new-chain stripped continuation outside generic create/schedule/complete/settle batch")
	}
}

func TestIndependentGenericNewChainsValidBatch(t *testing.T) {
	s, _ := fresh(t)
	before := s.Snapshot()
	k, e := core.NewTaskKernel(before)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	next, _ := s.Today().AddDays(1)
	for i := 0; i < 3; i++ {
		ids, e := newIDs(4)
		must(t, e)
		chain := core.TaskChainID(ids[0])
		trace := core.DayTraceID(ids[2])
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: core.TaskDefinitionID(ids[1]), Title: "合成合法批次", Now: now}))
		target := s.Today()
		if i == 2 {
			target = next
		}
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(ids[3]), Date: target, Today: s.Today(), Now: now}))
		if i == 0 {
			must(t, k.MarkCompleted(trace, s.Today(), now))
		}
	}
	later, e := core.NextMutationInstant(now, now)
	must(t, e)
	must(t, k.SettleTaskDays(next, later))
	v, e := CaptureEmptyHistory(s.snapshot, before, k.Snapshot())
	must(t, e)
	must(t, ValidateEmptyHistory(v))
	if historyUint(v.Object["classifications"].Object["revision"]) != 2 {
		t.Fatal("lost settlement batch events")
	}
}
