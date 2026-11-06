// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

// Direct fences receive corrupted proof copies, never trusted by an adapter.
// Each existing canonical node is independently replaced. This covers frozen
// fields, optionals, exact clock bits, order-bearing arrays and supplement.
func rfMutateNodes(v canonical.Value, check func(canonical.Value)) {
	check(canonical.Value{Kind: canonical.String, String: "CORRUPTED"})
	switch v.Kind {
	case canonical.String:
		q := v
		q.String += "!"
		check(q)
	case canonical.Number:
		q := v
		if q.TextNumber == "0" {
			q.TextNumber = "1"
		} else {
			q.TextNumber = "0"
		}
		check(q)
	case canonical.Boolean:
		q := v
		q.Bool = !q.Bool
		check(q)
	case canonical.Object:
		q := historyVNextClone(v)
		q.Object["unexpected"] = historyNumber(1)
		check(q)
	case canonical.Array:
		q := historyVNextClone(v)
		q.Array = append(q.Array, historyNumber(1))
		check(q)
	}
	for k, child := range v.Object {
		rfMutateNodes(child, func(c canonical.Value) { q := historyVNextClone(v); q.Object[k] = c; check(q) })
	}
	for i, child := range v.Array {
		rfMutateNodes(child, func(c canonical.Value) { q := historyVNextClone(v); q.Array[i] = c; check(q) })
	}
}
func TestRemainingDirectFenceEveryCanonicalNode(t *testing.T) {
	for _, op := range []string{"R", "X", "D", "C"} {
		t.Run(op, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			if op == "C" {
				f.run(t, vnStep{"L", 2})
			} else if op != "X" {
				f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
			}
			before := f.k.Snapshot()
			prior := vnGood(t, f.snapshot(t))
			tomorrow, e := vnDate.AddDays(1)
			must(t, e)
			r := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(8)), CancellationID: core.CancellationID(vnID(9)), Today: vnDate, Now: vnAt(5), EventID: vnID(101 + f.captures)}
			d := deferHistoryInput{ChainID: r.ChainID, TraceID: r.TraceID, NewTraceID: core.DayTraceID(vnID(6)), DayID: core.DayID(vnID(7)), TargetDate: tomorrow, Today: vnDate, Now: vnAt(5), EventID: r.EventID}
			c := continueHistoryInput{ChainID: d.ChainID, TraceID: d.TraceID, NewTraceID: d.NewTraceID, DayID: d.DayID, TargetDate: tomorrow, Today: tomorrow, Now: d.Now}
			if op == "X" {
				r.Today, e = vnDate.AddDays(-1)
				must(t, e)
				r.EventID = ""
			}
			if op == "C" {
				must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: c.ChainID, TraceID: c.TraceID, NewTraceID: c.NewTraceID, DayID: c.DayID, TargetDate: c.TargetDate, Today: c.Today, Now: c.Now}))
			} else {
				f.run(t, vnStep{op, 5})
			}
			final := vnGood(t, f.snapshot(t))
			fence := func(q validatedHistorySnapshot) bool {
				switch op {
				case "R", "X":
					return historyReturnFence(prior, q, r)
				case "D":
					return historyDeferFence(prior, q, d)
				default:
					return historyContinueFence(prior, q, c)
				}
			}
			if !fence(final) {
				t.Fatal("legal delta rejected")
			}
			count := 0
			rfMutateNodes(final.proof.snapshot, func(s canonical.Value) {
				q := *final.proof
				q.snapshot = s
				if fence(validatedHistorySnapshot{proof: &q}) {
					t.Fatal("mutated node accepted")
				}
				count++
			})
			t.Logf("%d direct canonical node mutations rejected", count)
			if !fence(final) {
				t.Fatal("fence mutated endpoint")
			}
		})
	}
}

func TestRemainingDirectFenceRichChildren(t *testing.T) {
	for _, op := range []string{"R", "X", "D", "C"} {
		t.Run(op, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			today := vnDate
			if op == "X" {
				var e error
				today, e = vnDate.AddDays(-1)
				must(t, e)
			}
			for i := 0; i < 3; i++ {
				must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(vnID(50 + i)), LineageID: core.SubtaskLineageID(vnID(60 + i)), Title: "child", Difficulty: core.DifficultySimple, Today: today, Now: vnAt(0)}))
			}
			action := core.SubtaskComplete
			cancel := core.CancellationID("")
			if op == "X" {
				action = core.SubtaskDelete
				cancel = core.CancellationID(vnID(70))
			}
			must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: action, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(vnID(52)), CancellationID: cancel, Today: today, Now: vnAt(1)}))
			// Synthetic source-supported optional and note facts, not Service reachability.
			g := f.k.Snapshot()
			// The source deliberately references an older definition.
			nextDef := g.Definitions[0]
			nextDef.ID = core.TaskDefinitionID(vnID(73))
			nextDef.Sequence = 2
			nextDef.Title = "current, not source"
			g.Definitions[0].SupersededAt = vnPtr(vnAt(0))
			g.Definitions[0].SupersededByDefinitionID = &nextDef.ID
			g.Definitions = append(g.Definitions, nextDef)
			g.Traces[0].DescriptionText = vnPtr("")
			g.Traces[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(vnID(71)), Body: "active", CreatedAt: vnAt(0), UpdatedAt: vnAt(0)}, {ID: core.TaskNoteEntryID(vnID(72)), Body: "", CreatedAt: vnAt(0), UpdatedAt: vnAt(1), DeletedAt: vnPtr(vnAt(1))}}
			// Reverse physical child order while preserving Position.
			g.Subtasks[0], g.Subtasks[1] = g.Subtasks[1], g.Subtasks[0]
			var e error
			f.k, e = core.NewTaskKernel(g)
			must(t, e)
			if op == "C" {
				f.run(t, vnStep{"L", 2})
			}
			// Existing target day with a negative priority tests max+1 without a
			// zero floor. Its unrelated chain has a strictly newer clock.
			if op == "D" || op == "C" {
				target, _ := vnDate.AddDays(1)
				must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(75)), DefinitionID: core.TaskDefinitionID(vnID(76)), Title: "unrelated", Now: vnAt(0)}))
				must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(75)), TraceID: core.DayTraceID(vnID(77)), DayID: core.DayID(vnID(78)), Date: target, Today: target, Now: vnAt(0)}))
				other := f.k.Snapshot()
				other.Traces[1].Priority = -5
				other.Chains[1].UpdatedAt = vnAt(100)
				f.k, e = core.NewTaskKernel(other)
				must(t, e)
			}
			before := f.k.Snapshot()
			prior := vnGood(t, f.snapshot(t))
			target, e := vnDate.AddDays(1)
			must(t, e)
			r := returnHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(80)), CancellationID: core.CancellationID(vnID(81)), Today: today, Now: vnAt(5), EventID: vnID(100 + f.captures + 1), PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{core.SubtaskID(vnID(50)): core.PlannedSubtaskID(vnID(90)), core.SubtaskID(vnID(51)): core.PlannedSubtaskID(vnID(91))}}
			ids := map[core.SubtaskID]core.SubtaskID{core.SubtaskID(vnID(50)): core.SubtaskID(vnID(90)), core.SubtaskID(vnID(51)): core.SubtaskID(vnID(91))}
			d := deferHistoryInput{ChainID: r.ChainID, TraceID: r.TraceID, NewTraceID: core.DayTraceID(vnID(80)), DayID: core.DayID(vnID(81)), TargetDate: target, Today: today, Now: r.Now, EventID: r.EventID, SubtaskIDs: ids}
			c := continueHistoryInput{ChainID: d.ChainID, TraceID: d.TraceID, NewTraceID: d.NewTraceID, DayID: d.DayID, TargetDate: target, Today: target, Now: r.Now, SubtaskIDs: ids}
			carry := core.TraceCarryoverInput{ChainID: d.ChainID, TraceID: d.TraceID, NewTraceID: d.NewTraceID, DayID: d.DayID, TargetDate: d.TargetDate, Today: d.Today, Now: d.Now, SubtaskIDs: ids}
			switch op {
			case "R", "X":
				if op == "X" {
					r.EventID = ""
				}
				must(t, f.k.ReturnToPool(core.ReturnToPoolInput{ChainID: r.ChainID, TraceID: r.TraceID, NewDefinitionID: r.NewDefinitionID, CancellationID: r.CancellationID, PlannedIDs: r.PlannedIDs, Today: r.Today, Now: r.Now}))
				if op == "R" {
					f.capture(core.TraceReturnedToPool, r.Now)
				}
			case "D":
				must(t, f.k.DeferCurrentTrace(carry))
				f.capture(core.TraceDeferred, d.Now)
			case "C":
				carry.Today = target
				must(t, f.k.ContinueUnfinishedTrace(carry))
			}
			final := vnGood(t, f.snapshot(t))
			fence := func(q validatedHistorySnapshot) bool {
				switch op {
				case "R", "X":
					return historyReturnFence(prior, q, r)
				case "D":
					return historyDeferFence(prior, q, d)
				default:
					return historyContinueFence(prior, q, c)
				}
			}
			if !fence(final) {
				t.Fatal("rich legal delta rejected")
			}
			n := 0
			rfMutateNodes(final.proof.snapshot, func(s canonical.Value) {
				q := *final.proof
				q.snapshot = s
				if fence(validatedHistorySnapshot{proof: &q}) {
					t.Fatal("rich canonical mutation accepted")
				}
				n++
			})
			t.Logf("%d rich direct mutations rejected", n)
		})
	}
}

func TestRemainingDirectFenceNegativeAndZeroBits(t *testing.T) {
	for _, op := range []string{"R", "X", "D", "C"} {
		for _, zero := range []bool{false, true} {
			t.Run(op+map[bool]string{false: " negative", true: " signed zero"}[zero], func(t *testing.T) {
				f := vnSeed(t, vnDate)
				g := f.k.Snapshot()
				g.Days[0].CreatedAt = vnAt(-2)
				g.Days[0].UpdatedAt = vnAt(-2)
				g.Chains[0].CreatedAt = vnAt(-2)
				g.Chains[0].UpdatedAt = vnAt(-2)
				g.Definitions[0].CreatedAt = vnAt(-2)
				g.Definitions[0].ContentUpdatedAt = vnAt(-2)
				g.Traces[0].CreatedAt = vnAt(-2)
				g.Traces[0].ContentUpdatedAt = vnAt(-2)
				now := vnAt(-1)
				if zero {
					now = core.Instant(1 << 63)
					g.Chains[0].UpdatedAt = core.Instant(0)
				}
				if op == "C" {
					g.Days[0].LockedAt = vnPtr(vnAt(-1))
					g.Days[0].UpdatedAt = vnAt(-1)
					g.Traces[0].Status = core.TraceUnfinished
					g.Traces[0].SettledAt = vnPtr(vnAt(-1))
					g.Traces[0].ContentUpdatedAt = vnAt(-1)
					f.capture(core.TraceUnfinished, vnAt(-1))
				}
				var e error
				f.k, e = core.NewTaskKernel(g)
				must(t, e)
				prior := vnGood(t, f.snapshot(t))
				target, e := vnDate.AddDays(1)
				must(t, e)
				r := returnHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(8)), CancellationID: core.CancellationID(vnID(9)), Today: vnDate, Now: now, EventID: vnID(100 + f.captures + 1)}
				d := deferHistoryInput{ChainID: r.ChainID, TraceID: r.TraceID, NewTraceID: core.DayTraceID(vnID(6)), DayID: core.DayID(vnID(7)), TargetDate: target, Today: vnDate, Now: now, EventID: r.EventID}
				c := continueHistoryInput{ChainID: r.ChainID, TraceID: r.TraceID, NewTraceID: d.NewTraceID, DayID: d.DayID, TargetDate: target, Today: target, Now: now}
				carry := core.TraceCarryoverInput{ChainID: d.ChainID, TraceID: d.TraceID, NewTraceID: d.NewTraceID, DayID: d.DayID, TargetDate: target, Today: d.Today, Now: now}
				switch op {
				case "R", "X":
					if op == "X" {
						r.Today, e = vnDate.AddDays(-1)
						must(t, e)
						r.EventID = ""
					}
					must(t, f.k.ReturnToPool(core.ReturnToPoolInput{ChainID: r.ChainID, TraceID: r.TraceID, NewDefinitionID: r.NewDefinitionID, CancellationID: r.CancellationID, Today: r.Today, Now: now}))
					if op == "R" {
						f.capture(core.TraceReturnedToPool, now)
					}
				case "D":
					must(t, f.k.DeferCurrentTrace(carry))
					f.capture(core.TraceDeferred, now)
				case "C":
					carry.Today = target
					must(t, f.k.ContinueUnfinishedTrace(carry))
				}
				final := vnGood(t, f.snapshot(t))
				fence := func(q validatedHistorySnapshot) bool {
					switch op {
					case "R", "X":
						return historyReturnFence(prior, q, r)
					case "D":
						return historyDeferFence(prior, q, d)
					default:
						return historyContinueFence(prior, q, c)
					}
				}
				if !fence(final) {
					t.Fatal("clock case rejected")
				}
				if zero {
					want := uint64(1 << 63)
					if op == "D" || op == "C" {
						want = 0
					}
					chain := final.proof.snapshot.Object["chains"].Array[0].Object["updatedAt"]
					if !historyEqual(chain, historyNumber(want)) {
						t.Fatal("wrong equal clock bits")
					}
					bad := sfPolluted(final, func(s canonical.Value) {
						s.Object["chains"].Array[0].Object["updatedAt"] = historyNumber(want ^ (1 << 63))
					})
					if fence(bad) {
						t.Fatal("equal numeric clock with wrong bits accepted")
					}
				}
			})
		}
	}
}
