// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestHistoryEditRouteGuards(t *testing.T) {
	for _, name := range []string{"owner", "missing", "past", "future", "completed", "rename-trace", "unknown-action", "unknown-route", "chain-trace", "bad-chain", "bad-trace"} {
		t.Run(name, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			g := f.k.Snapshot()
			in := editInput(g)
			in.Route = titleHistoryTrace
			in.TraceID = g.Traces[0].ID
			var want error
			switch name {
			case "owner":
				in.ChainID = core.TaskChainID(vnID(99))
				want = historyCommandFail(historyCommandOwner)
			case "missing":
				in.TraceID = core.DayTraceID(vnID(99))
				want = core.ErrNotFound
			case "past":
				in.Today, _ = vnDate.AddDays(1)
				want = core.ErrImmutableHistory
			case "future":
				in.Today, _ = vnDate.AddDays(-1)
			case "completed":
				must(t, f.k.MarkCompleted(in.TraceID, vnDate, vnAt(1)))
				g = f.k.Snapshot()
				want = core.ErrInvalidTransition
			case "rename-trace":
				in.Action = titleHistoryRename
				want = historyCommandFail(historyCommandInput)
			case "unknown-action":
				in.Action = 255
				want = historyCommandFail(historyCommandInput)
			case "unknown-route":
				in.Route = 255
				want = historyCommandFail(historyCommandInput)
			case "chain-trace":
				in.Route = titleHistoryChain
				want = historyCommandFail(historyCommandInput)
			case "bad-chain":
				in.ChainID = core.TaskChainID(strings.Repeat("x", 36))
				want = historyCommandFail(historyCommandInput)
			case "bad-trace":
				in.TraceID = core.DayTraceID(strings.Repeat("x", 36))
				want = historyCommandFail(historyCommandInput)
			}
			p := vnGood(t, f.snapshot(t))
			a := g
			if want == nil {
				var e error
				a, e = editTitle(t, g, in)
				must(t, e)
			}
			r, e := auditTitleHistoryVNext(p, g, a, in)
			if want == nil {
				editCheck(t, p, r, e)
			} else {
				hcZero(t, r, e)
				if reflect.TypeOf(e) != reflect.TypeOf(want) || e.Error() != want.Error() {
					t.Fatal(e, want)
				}
			}
		})
	}
}
func TestHistoryEditManualGuardOrderAndClocks(t *testing.T) {
	for _, name := range []string{"missing", "malformed", "owner", "invalid-date", "past", "future", "C", "R", "X", "D", "A", "U", "locked", "child", "early", "nan", "inf"} {
		t.Run(name, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			g := f.k.Snapshot()
			in := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: vnDate, Now: vnAt(3)}
			want := core.ErrImmutableHistory
			switch name {
			case "missing":
				in.TraceID = core.DayTraceID(vnID(99))
				want = core.ErrNotFound
			case "malformed":
				in.TraceID = core.DayTraceID(strings.Repeat("x", 36))
				want = core.ErrNotFound
			case "owner":
				in.ChainID = core.TaskChainID(vnID(99))
				want = core.ErrInvalidInput
			case "invalid-date":
				in.Today = core.LocalDate{}
				want = core.ErrInvalidInput
			case "past":
				in.Today, _ = vnDate.AddDays(1)
			case "future":
				in.Today, _ = vnDate.AddDays(-1)
			case "C", "R", "X", "D", "A":
				f.run(t, vnStep{name, 1})
			case "U":
				f.run(t, vnStep{"A", 1}, vnStep{"U", 2})
			case "locked":
				f.run(t, vnStep{"L", 2})
			case "child":
				must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: in.ChainID, TraceID: in.TraceID, SubtaskID: core.SubtaskID(vnID(31)), LineageID: core.SubtaskLineageID(vnID(32)), Title: "child", Difficulty: core.DifficultyHard, Today: vnDate, Now: vnAt(1)}))
				want = core.ErrInvalidTransition
			case "early":
				in.Now = vnAt(-1)
				want = core.ErrInvalidClock
			case "nan":
				in.Now = core.Instant(math.Float64bits(math.NaN()))
				want = core.ErrInvalidClock
			case "inf":
				in.Now = core.Instant(math.Float64bits(math.Inf(1)))
				want = core.ErrInvalidClock
			}
			g = f.k.Snapshot()
			p := vnGood(t, f.snapshot(t))
			saved := g.Clone()
			r, e := auditManualProgressHistoryVNext(p, g, g, in)
			hcZero(t, r, e)
			if !errors.Is(e, want) {
				t.Fatal(e, want)
			}
			if !reflect.DeepEqual(saved, g) {
				t.Fatal("mutated failure")
			}
		})
	}
}
func TestHistoryEditTitleVersionConsumedParameters(t *testing.T) {
	for _, name := range []string{"bad", "duplicate", "overflow", "pending-clock", "restored"} {
		t.Run(name, func(t *testing.T) {
			g := vnSeed(t, vnDate).k.Snapshot()
			in := editInput(g)
			in.Action = titleHistoryRename
			want := core.ErrInvalidInput
			switch name {
			case "bad":
				in.NewDefinitionID = "bad"
			case "duplicate":
				in.NewDefinitionID = g.Definitions[0].ID
			case "overflow":
				g.Definitions[0].Sequence = int(^uint(0) >> 1)
			case "pending-clock":
				g.Traces[0].ContentUpdatedAt = vnAt(10)
				want = core.ErrInvalidClock
			case "restored":
				old := g.Definitions[0]
				old.ID = core.TaskDefinitionID(vnID(99))
				old.Sequence = 7
				old.SupersededAt = vnPtr(vnAt(0))
				old.SupersededByDefinitionID = vnPtr(g.Definitions[0].ID)
				g.Definitions[0].SupersededByDefinitionID = vnPtr(old.ID)
				g.Definitions = append(g.Definitions, old)
				want = nil
			}
			p := editPrior(t, g)
			a := g
			if want == nil {
				var e error
				a, e = editTitle(t, g, in)
				must(t, e)
			}
			r, e := auditTitleHistoryVNext(p, g, a, in)
			if want == nil {
				editCheck(t, p, r, e)
				if *a.Definitions[0].SupersededByDefinitionID != in.NewDefinitionID {
					t.Fatal("witness")
				}
			} else {
				hcZero(t, r, e)
				if !errors.Is(e, want) {
					t.Fatal(e, want)
				}
			}
		})
	}
}
