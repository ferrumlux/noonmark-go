// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func saEncode(t *testing.T, g core.TaskGraph, source canonical.Value) canonical.Value {
	t.Helper()
	_, extra, e := v7.ToDomain(source)
	must(t, e)
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	return s
}

func TestHistorySubtaskAliasOriginalSealBoundary(t *testing.T) {
	s, g, c := reviewUndoContinuationAlias(t)
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, c.apply(k))
	vnBad(t, s, historyToday)
}

// Parameter fixtures deliberately do NOT claim command reachability. Every
// audit receives a genuine full-inspector seal, never a manufactured seal.
func saAlias(t *testing.T, topology string) (canonical.Value, core.TaskGraph, core.TaskDefinitionID) {
	t.Helper()
	var s canonical.Value
	var g core.TaskGraph
	var target core.TaskDefinitionID
	if topology == "U-X" {
		var c subtaskEditCommand
		s, g, c = reviewUndoContinuationAlias(t)
		target = c.planned.ExpectedDefinitionID
		source := g.Traces[1].Date
		g.Traces[len(g.Traces)-1].DraftCancelledOn = &source
	} else {
		f := vnSeed(t, vnDate)
		f.run(t, vnStep{"R", 10})
		s = f.snapshot(t)
		g = f.k.Snapshot()
		// Undo only definition versioning, retaining the real R fact/event.
		g.Definitions = g.Definitions[:1]
		g.Definitions[0].SupersededAt = nil
		g.Definitions[0].SupersededByDefinitionID = nil
		target = g.Definitions[0].ID
	}
	for i := range g.Definitions {
		if g.Definitions[i].ID == target {
			g.Definitions[i].PlannedSubtasks = []core.PlannedSubtask{{ID: core.PlannedSubtaskID(vnID(750)), LineageID: core.SubtaskLineageID(vnID(751)), Title: "old", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: g.Definitions[i].CreatedAt}}
		}
	}
	s = saEncode(t, g, s)
	vnGood(t, s)
	return s, g, target
}

func TestHistorySubtaskAliasLayeredFence(t *testing.T) {
	for _, topology := range []string{"U-X", "R-current"} {
		t.Run(topology, func(t *testing.T) {
			s, g, target := saAlias(t, topology)
			p := vnGood(t, s)
			var chain core.TaskChainID
			for _, d := range g.Definitions {
				if d.ID == target {
					chain = d.ChainID
				}
			}
			for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
				t.Run(string(kind), func(t *testing.T) {
					in := plannedSubtaskHistoryInput{Kind: kind, ChainID: chain, ExpectedDefinitionID: target, SubtaskID: core.PlannedSubtaskID(vnID(750)), Now: vnAt(40)}
					switch kind {
					case core.SubtaskAdd:
						in.SubtaskID = core.PlannedSubtaskID(vnID(752))
						in.LineageID = core.SubtaskLineageID(vnID(753))
						in.Title = "new"
						in.Difficulty = core.DifficultySimple
					case core.SubtaskTitle:
						in.Title = "new"
					case core.SubtaskDifficulty:
						in.Difficulty = core.DifficultyHard
					}
					owned, extra, e := historyCommandBind(p, g)
					must(t, e)
					k, e := core.NewTaskKernel(owned)
					must(t, e)
					must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
					a := k.Snapshot()
					vnGood(t, saEncode(t, a, s))
					must(t, historyCommandMatch(a, a, extra))
					final, e := historyCommandFinish(p, a, extra, nil)
					must(t, e)
					if historyPlannedSubtaskFence(p, final, in) {
						t.Fatal("historical definition rewrite crossed independent fence")
					}
					r, e := auditPlannedSubtaskHistoryVNext(p, g, a, in)
					lifecycleZero(t, r, e)
					var typed *historyCommandError
					if !errors.As(e, &typed) || typed.Reason != historyCommandFence {
						t.Fatalf("not fence: %v", e)
					}
				})
			}
			for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty} {
				in := plannedSubtaskHistoryInput{Kind: kind, ChainID: chain, ExpectedDefinitionID: target, SubtaskID: core.PlannedSubtaskID(vnID(750)), Now: core.Instant(0x7ff8000000000001)}
				if kind == core.SubtaskTitle {
					in.Title = " old "
				} else {
					in.Difficulty = core.DifficultySimple
				}
				r, e := auditPlannedSubtaskHistoryVNext(p, g, g, in)
				must(t, e)
				changed, e := r.changed()
				must(t, e)
				if changed {
					t.Fatal("alias no-op changed")
				}
			}
		})
	}
}

func TestHistorySubtaskRealReturnFreshDefinition(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"R", 10})
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := plannedSubtaskHistoryInput{Kind: core.SubtaskAdd, ChainID: g.Chains[0].ID, ExpectedDefinitionID: g.Definitions[1].ID, SubtaskID: core.PlannedSubtaskID(vnID(760)), LineageID: core.SubtaskLineageID(vnID(761)), Title: "fresh", Difficulty: core.DifficultySimple, Now: vnAt(20)}
	must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
	r, e := auditPlannedSubtaskHistoryVNext(p, g, f.k.Snapshot(), in)
	must(t, e)
	if !historyEqual(p.proof.snapshot.Object["definitions"].Array[0], r.proof.after.proof.snapshot.Object["definitions"].Array[0]) {
		t.Fatal("historical definition changed")
	}
}

// History edges use real core commands plus existing typed auditors; no Service reachability claim.
func TestHistorySubtaskCommandGeneratedLineages(t *testing.T) {
	for _, path := range []string{"A-P", "A-A-P", "A-U", "U-A-U"} {
		t.Run(path, func(t *testing.T) {
			tomorrow, e := vnDate.AddDays(1)
			must(t, e)
			f := vnSeed(t, vnDate)
			// A separate future P survives settlement, then becomes today's manual owner.
			other := core.TaskChainID(vnID(20))
			otherTrace := core.DayTraceID(vnID(22))
			must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: other, DefinitionID: core.TaskDefinitionID(vnID(21)), Title: "other", Now: vnAt(1)}))
			must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: other, TraceID: otherTrace, DayID: core.DayID(vnID(23)), Date: tomorrow, Today: vnDate, Now: vnAt(2)}))
			p := vnGood(t, f.snapshot(t))
			nextEvent := 100
			clock := 3.0
			abandon := func() {
				b := f.k.Snapshot()
				in := abandonHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Now: vnAt(clock), EventID: vnID(nextEvent)}
				clock++
				nextEvent++
				must(t, f.k.AbandonChain(core.AbandonChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Now: in.Now}))
				r, e := auditAbandonHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				if !r.valid() {
					t.Fatal("abandon proof")
				}
				p = r.proof.after
			}
			react := func(day core.LocalDate, capture bool) {
				b := f.k.Snapshot()
				in := reactivateHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Today: day, Now: vnAt(clock)}
				clock++
				if capture {
					in.EventIDs = []string{vnID(nextEvent)}
					nextEvent++
				}
				_, e := f.k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Today: day, Now: in.Now})
				must(t, e)
				r, e := auditReactivateHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				p = r.proof.after
			}
			settle := func() {
				b := f.k.Snapshot()
				in := settlementHistoryInput{Today: tomorrow, Now: vnAt(clock), EventIDs: []string{vnID(nextEvent)}}
				clock++
				nextEvent++
				must(t, f.k.SettleTaskDays(in.Today, in.Now))
				r, e := auditSettlementHistoryVNext(p, b, f.k.Snapshot(), in)
				must(t, e)
				p = r.proof.after
			}
			today := vnDate
			switch path {
			case "A-P":
				abandon()
				react(vnDate, false)
			case "A-A-P":
				abandon()
				react(vnDate, false)
				abandon()
				react(vnDate, false)
			case "A-U":
				abandon()
				react(vnDate, false)
				settle()
				today = tomorrow
			case "U-A-U":
				settle()
				abandon()
				react(tomorrow, true)
				today = tomorrow
			}
			g := f.k.Snapshot()
			target, chain := g.Traces[0].ID, g.Chains[0].ID
			if today == tomorrow {
				target, chain = otherTrace, other
			}
			in := actualSubtaskHistoryInput{Kind: core.SubtaskAdd, ChainID: chain, TraceID: target, SubtaskID: core.SubtaskID(vnID(780)), LineageID: core.SubtaskLineageID(vnID(781)), Title: "lineage child", Difficulty: core.DifficultySimple, Today: today, Now: vnAt(clock)}
			frozen := historyVNextClone(p.proof.snapshot.Object["classifications"])
			must(t, f.k.EditSubtask(core.SubtaskEditInput(in)))
			r, e := auditActualSubtaskHistoryVNext(p, g, f.k.Snapshot(), in)
			must(t, e)
			if !r.valid() || !historyEqual(frozen, r.proof.after.proof.snapshot.Object["classifications"]) {
				t.Fatal("lineage classification changed")
			}
			must(t, checkForwardHistorySuccessor(p, r.proof.after))
		})
	}
}

// The source histories below are command-produced with explicit projection
// events (vnFixture), not Service output or a claim that return/defer are typed.
// Every endpoint is independently inspected; each tested child edge is typed.
func saFrozenPrefix(t *testing.T, source, final canonical.Value) {
	t.Helper()
	for _, key := range []string{"chains", "definitions", "traces", "subtasks", "days"} {
		old, got := source.Object[key].Array, final.Object[key].Array
		if len(got) < len(old) {
			t.Fatalf("lost %s history", key)
		}
		for i := range old {
			if !historyEqual(old[i], got[i]) {
				t.Fatalf("changed %s history index %d", key, i)
			}
		}
	}
	if !historyEqual(source.Object["classifications"], final.Object["classifications"]) {
		t.Fatal("classification history changed")
	}
}

func TestHistorySubtaskOtherChainElevenActionsAcrossHistories(t *testing.T) {
	paths := map[string][]vnStep{
		"A-P": {{"A", 3}, {"P", 4}}, "A-A-P": {{"A", 3}, {"P", 4}, {"A", 5}, {"P", 6}},
		"A-U": {{"A", 3}, {"U", 4}}, "U-A-U": {{"L", 3}, {"A", 4}, {"U", 5}},
		"R": {{"R", 3}}, "D": {{"D", 3}}, "C": {{"C", 3}, {"L", 4}},
	}
	for name, steps := range paths {
		t.Run(name, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			f.run(t, steps...)
			source := f.snapshot(t)
			vnGood(t, source)
			tomorrow, e := vnDate.AddDays(1)
			must(t, e)
			chain, definition := core.TaskChainID(vnID(900)), core.TaskDefinitionID(vnID(901))
			must(t, f.k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: definition, Title: "independent pool", Now: vnAt(20)}))
			child := core.PlannedSubtaskID(vnID(902))
			must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, ExpectedDefinitionID: definition, SubtaskID: child, LineageID: core.SubtaskLineageID(vnID(903)), Title: "old", Difficulty: core.DifficultySimple, Now: vnAt(21)}))
			pool := f.k.Snapshot()
			poolSnapshot := f.snapshot(t)
			poolSeal := vnGood(t, poolSnapshot)
			for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
				t.Run("planned/"+string(kind), func(t *testing.T) {
					in := plannedSubtaskHistoryInput{Kind: kind, ChainID: chain, ExpectedDefinitionID: definition, SubtaskID: child, Now: vnAt(30)}
					switch kind {
					case core.SubtaskAdd:
						in.SubtaskID = core.PlannedSubtaskID(vnID(904))
						in.LineageID = core.SubtaskLineageID(vnID(905))
						in.Title = "new"
						in.Difficulty = core.DifficultySimple
					case core.SubtaskTitle:
						in.Title = "new"
					case core.SubtaskDifficulty:
						in.Difficulty = core.DifficultyHard
					}
					k, e := core.NewTaskKernel(pool)
					must(t, e)
					must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
					r, e := auditPlannedSubtaskHistoryVNext(poolSeal, pool, k.Snapshot(), in)
					must(t, e)
					saFrozenPrefix(t, source, r.proof.after.proof.snapshot)
				})
			}
			for _, future := range []bool{false, true} {
				date := tomorrow
				if future {
					date, e = tomorrow.AddDays(1)
					must(t, e)
				}
				k, e := core.NewTaskKernel(pool)
				must(t, e)
				trace, actual := core.DayTraceID(vnID(906)), core.SubtaskID(vnID(907))
				must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: core.DayID(vnID(908)), Date: date, Today: tomorrow, Now: vnAt(22), SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{child: actual}}))
				base := k.Snapshot()
				for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
					if future && (kind == core.SubtaskComplete || kind == core.SubtaskUndoComplete || kind == core.SubtaskAbandon) {
						continue
					}
					label := "current/"
					if future {
						label = "future/"
					}
					t.Run(label+string(kind), func(t *testing.T) {
						k, e := core.NewTaskKernel(base)
						must(t, e)
						b := base
						p := vnGood(t, saEncode(t, b, poolSnapshot))
						if kind == core.SubtaskUndoComplete {
							ci := actualSubtaskHistoryInput{Kind: core.SubtaskComplete, ChainID: chain, TraceID: trace, SubtaskID: actual, Today: tomorrow, Now: vnAt(23)}
							must(t, k.EditSubtask(core.SubtaskEditInput(ci)))
							r, e := auditActualSubtaskHistoryVNext(p, b, k.Snapshot(), ci)
							must(t, e)
							p = r.proof.after
							b = k.Snapshot()
						}
						in := actualSubtaskHistoryInput{Kind: kind, ChainID: chain, TraceID: trace, SubtaskID: actual, Today: tomorrow, Now: vnAt(30)}
						switch kind {
						case core.SubtaskAdd:
							in.SubtaskID = core.SubtaskID(vnID(909))
							in.LineageID = core.SubtaskLineageID(vnID(910))
							in.Title = "new"
							in.Difficulty = core.DifficultySimple
						case core.SubtaskTitle:
							in.Title = "new"
						case core.SubtaskDifficulty:
							in.Difficulty = core.DifficultyHard
						case core.SubtaskDelete:
							in.CancellationID = core.CancellationID(vnID(911))
						}
						must(t, k.EditSubtask(core.SubtaskEditInput(in)))
						r, e := auditActualSubtaskHistoryVNext(p, b, k.Snapshot(), in)
						must(t, e)
						saFrozenPrefix(t, source, r.proof.after.proof.snapshot)
					})
				}
			}
		})
	}
}

func TestHistorySubtaskContinueUnfinishedSameChain(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	oldChild, newChild := core.SubtaskID(vnID(930)), core.SubtaskID(vnID(931))
	must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, TraceID: trace, SubtaskID: oldChild, LineageID: core.SubtaskLineageID(vnID(932)), Title: "continued child", Difficulty: core.DifficultySimple, Today: vnDate, Now: vnAt(1)}))
	p := vnGood(t, f.snapshot(t))
	before := f.k.Snapshot()
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	si := settlementHistoryInput{Today: tomorrow, Now: vnAt(2), EventIDs: []string{vnID(933)}}
	must(t, f.k.SettleTaskDays(si.Today, si.Now))
	settled, e := auditSettlementHistoryVNext(p, before, f.k.Snapshot(), si)
	must(t, e)
	history := settled.proof.after.proof.snapshot
	continuedTrace := core.DayTraceID(vnID(934))
	// Continue currently has no typed adapter; its real core result is fully sealed
	// and checked for forward classification-prefix preservation before child edits.
	must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: chain, TraceID: trace, NewTraceID: continuedTrace, DayID: core.DayID(vnID(935)), SubtaskIDs: map[core.SubtaskID]core.SubtaskID{oldChild: newChild}, TargetDate: tomorrow, Today: tomorrow, Now: vnAt(3)}))
	base := f.k.Snapshot()
	snapshot := saEncode(t, base, history)
	prior := vnGood(t, snapshot)
	must(t, checkForwardHistorySuccessor(settled.proof.after, prior))
	// Continue legitimately touches the parent chain clock. The old definition,
	// source trace/child, locked day, and classification bytes remain exact.
	history = historyVNextClone(history)
	history.Object["chains"].Array[0].Object["updatedAt"] = historyNumber(uint64(vnAt(3)))
	saFrozenPrefix(t, history, snapshot)
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon, core.SubtaskDelete} {
		t.Run(string(kind), func(t *testing.T) {
			k, e := core.NewTaskKernel(base)
			must(t, e)
			b := base
			p := prior
			if kind == core.SubtaskUndoComplete {
				ci := actualSubtaskHistoryInput{Kind: core.SubtaskComplete, ChainID: chain, TraceID: continuedTrace, SubtaskID: newChild, Today: tomorrow, Now: vnAt(4)}
				must(t, k.EditSubtask(core.SubtaskEditInput(ci)))
				r, e := auditActualSubtaskHistoryVNext(p, b, k.Snapshot(), ci)
				must(t, e)
				p = r.proof.after
				b = k.Snapshot()
			}
			in := actualSubtaskHistoryInput{Kind: kind, ChainID: chain, TraceID: continuedTrace, SubtaskID: newChild, Today: tomorrow, Now: vnAt(5)}
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.SubtaskID(vnID(936))
				in.LineageID = core.SubtaskLineageID(vnID(937))
				in.Title = "new"
				in.Difficulty = core.DifficultySimple
			case core.SubtaskTitle:
				in.Title = "edited"
			case core.SubtaskDifficulty:
				in.Difficulty = core.DifficultyHard
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(vnID(938))
			}
			must(t, k.EditSubtask(core.SubtaskEditInput(in)))
			r, e := auditActualSubtaskHistoryVNext(p, b, k.Snapshot(), in)
			must(t, e)
			saFrozenPrefix(t, history, r.proof.after.proof.snapshot)
			got := k.Snapshot().Subtasks[1]
			if got.ID != newChild || got.CarriedFromSubtaskID == nil || *got.CarriedFromSubtaskID != oldChild || got.LineageID != base.Subtasks[0].LineageID {
				t.Fatal("continued child identity lost")
			}
			for _, key := range []string{"id", "lineageID", "traceID", "position", "createdAt", "carriedFromSubtaskID"} {
				if !historyEqual(snapshot.Object["subtasks"].Array[1].Object[key], r.proof.after.proof.snapshot.Object["subtasks"].Array[1].Object[key]) {
					t.Fatalf("changed carried %s", key)
				}
			}
		})
	}
}
