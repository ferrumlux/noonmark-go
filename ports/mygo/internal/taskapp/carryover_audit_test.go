// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func carryoverInput(t *testing.T, s *Service, continuation bool) (core.TaskGraph, core.TaskGraph, core.TraceCarryoverInput) {
	t.Helper()
	before := s.Snapshot()
	tr := before.Traces[0]
	ids, e := newIDs(2)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	target, _ := s.Today().AddDays(1)
	in := core.TraceCarryoverInput{ChainID: tr.ChainID, TraceID: tr.ID, NewTraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Today: s.Today(), TargetDate: target, Now: now, SubtaskIDs: map[core.SubtaskID]core.SubtaskID{}}
	for _, child := range before.Subtasks {
		if child.TraceID == tr.ID && (child.Status == core.SubtaskPending || child.Status == core.SubtaskUnfinished) {
			id, e := newIDs(1)
			must(t, e)
			in.SubtaskIDs[child.ID] = core.SubtaskID(id[0])
		}
	}
	k, e := core.NewTaskKernel(before)
	must(t, e)
	must(t, replayCarryover(k, in, continuation))
	return before, k.Snapshot(), in
}

func TestCarryoverStrippedContinuationCannotBypassGenericAudit(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("合成绕行"))
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
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
		t.Logf("domain rejected stripped continuation: %v", e)
		return
	}
	if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
		t.Fatal("generic audit accepted stripped continuation for non-pool source")
	}
}

func TestGenericScheduleAuditPreservesBatchesAndReturn(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddPool("合成池一"))
	must(t, s.AddPool("合成池二"))
	before := s.Snapshot()
	k, e := core.NewTaskKernel(before)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	for _, chain := range before.Chains {
		ids, e := newIDs(2)
		must(t, e)
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain.ID, TraceID: core.DayTraceID(ids[0]), DayID: core.DayID(ids[1]), Date: testDay, Today: testDay, Now: now}))
	}
	next, _ := testDay.AddDays(1)
	later, e := core.NextMutationInstant(now, now)
	must(t, e)
	must(t, k.SettleTaskDays(next, later))
	wire, e := CaptureEmptyHistory(s.snapshot, before, k.Snapshot())
	must(t, e)
	must(t, ValidateEmptyHistory(wire))
	r, _, tr := prepareCarryover(t, false)
	must(t, r.ReturnToPool(tr.ChainID, tr.ID))
	must(t, r.Schedule(tr.ChainID, testDay))
	must(t, ValidateEmptyHistory(r.snapshot))
}

func TestCarryoverAuditExactReplayAndReader(t *testing.T) {
	for _, cont := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[cont], func(t *testing.T) {
			s, _, _ := prepareCarryover(t, cont)
			before, after, in := carryoverInput(t, s, cont)
			original := futureAggregate(t, s)
			if _, e := CaptureEmptyHistory(s.snapshot, before, after); e == nil {
				t.Fatal("generic carryover accepted")
			}
			good, e := captureCarryoverHistory(s.snapshot, before, after, in, cont)
			must(t, e)
			must(t, ValidateEmptyHistory(good))
			for _, mode := range []string{"other-chain-clock", "trace-description", "missing-target", "wrong-input-target", "wrong-input-clock", "wrong-input-owner", "wrong-command"} {
				t.Run(mode, func(t *testing.T) {
					k, e := core.NewTaskKernel(after)
					must(t, e)
					bad := k.Snapshot()
					cmd := in
					continuation := cont
					switch mode {
					case "other-chain-clock":
						bad.Chains[0].UpdatedAt = before.Chains[0].UpdatedAt
					case "trace-description":
						v := "forged"
						bad.Traces[1].DescriptionText = &v
					case "missing-target":
						bad.Traces = bad.Traces[:1]
					case "wrong-input-target":
						cmd.TargetDate, _ = cmd.TargetDate.AddDays(1)
					case "wrong-input-clock":
						cmd.Now, e = core.NextMutationInstant(cmd.Now, cmd.Now)
						must(t, e)
					case "wrong-input-owner":
						cmd.ChainID = "00000000-0000-4000-8000-000000000099"
					case "wrong-command":
						continuation = !cont
					}
					if _, e := captureCarryoverHistory(s.snapshot, before, bad, cmd, continuation); e == nil {
						t.Fatal("forged candidate/input accepted")
					}
				})
			}
			if string(original) != string(futureAggregate(t, s)) {
				t.Fatal("audit mutated prior")
			}
			for _, mode := range []string{"missing", "duplicate", "clock", "status", "nonempty-catalog", "supplement"} {
				t.Run(mode, func(t *testing.T) {
					bad, e := inspectEmptyHistory(good)
					must(t, e)
					c := bad.Object["classifications"]
					switch mode {
					case "missing":
						v := c.Object["snapshotsByTraceID"]
						v.Array = nil
						c.Object["snapshotsByTraceID"] = v
						v = c.Object["snapshotEventsByTraceID"]
						v.Array = nil
						c.Object["snapshotEventsByTraceID"] = v
						c.Object["revision"] = historyNumber(0)
					case "duplicate":
						stream := c.Object["snapshotEventsByTraceID"].Array[1]
						stream.Array = append(stream.Array, stream.Array[0])
						c.Object["snapshotEventsByTraceID"].Array[1] = stream
					case "clock":
						c.Object["snapshotsByTraceID"].Array[1].Object["capturedAt"] = historyNumber(0)
						c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["capturedAt"] = historyNumber(0)
					case "status":
						c.Object["snapshotsByTraceID"].Array[1].Object["status"] = historyString("completed")
						c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["status"] = historyString("completed")
					case "nonempty-catalog":
						v := c.Object["labels"]
						v.Array = append(v.Array, historyString("unsupported"))
						c.Object["labels"] = v
					case "supplement":
						v := bad.Object["ideas"]
						v.Array = append(v.Array, historyString("unsupported"))
						bad.Object["ideas"] = v
					}
					bad.Object["classifications"] = c
					if ValidateEmptyHistory(bad) == nil {
						t.Fatal("bad audit accepted")
					}
					if _, e := CaptureEmptyHistory(bad, after, after); e == nil {
						t.Fatal("generic reconstructed or accepted bad history")
					}
				})
			}
			if !cont {
				for _, mode := range []string{"description", "removed", "clock"} {
					t.Run("terminal-"+mode, func(t *testing.T) {
						k, e := core.NewTaskKernel(after)
						must(t, e)
						bad := k.Snapshot()
						switch mode {
						case "description":
							v := "forged"
							bad.Traces[0].DescriptionText = &v
						case "removed":
							bad.Traces = bad.Traces[1:]
						case "clock":
							bad.Traces[0].ContentUpdatedAt = in.Now + 1
						}
						if _, e := CaptureEmptyHistory(good, after, bad); e == nil {
							t.Fatal("terminal edit accepted")
						}
					})
				}
			}
		})
	}
}
