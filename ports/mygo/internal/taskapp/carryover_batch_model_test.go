// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func TestGenericNewTraceBatchCompletionWitness(t *testing.T) {
	for _, mode := range []string{"complete", "complete-undo", "complete-undo-settle"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.AddPool("合成组合"))
			before := s.Snapshot()
			k, e := core.NewTaskKernel(before)
			must(t, e)
			ids, e := newIDs(2)
			must(t, e)
			now, e := s.nextInstant()
			must(t, e)
			tr := core.DayTraceID(ids[0])
			must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: before.Chains[0].ID, TraceID: tr, DayID: core.DayID(ids[1]), Date: testDay, Today: testDay, Now: now}))
			complete, e := core.NextMutationInstant(now, now)
			must(t, e)
			must(t, k.MarkCompleted(tr, testDay, complete))
			if mode != "complete" {
				undo, e := core.NextMutationInstant(complete, complete)
				must(t, e)
				must(t, k.UndoCompleted(tr, testDay, undo))
				if mode == "complete-undo-settle" {
					settle, e := core.NextMutationInstant(undo, undo)
					must(t, e)
					next, _ := testDay.AddDays(1)
					must(t, k.SettleTaskDays(next, settle))
				}
			}
			wire, e := CaptureEmptyHistory(s.snapshot, before, k.Snapshot())
			must(t, e)
			must(t, ValidateEmptyHistory(wire))
		})
	}
}
