// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

func ordinaryTestID(n byte) string {
	const digits = "0123456789ABCDEF"
	return "00000000-0000-4000-8000-0000000000" + string([]byte{digits[n>>4], digits[n&15]})
}
func ordinaryTestGraph(t *testing.T, date core.LocalDate) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(ordinaryTestID(1)), DefinitionID: core.TaskDefinitionID(ordinaryTestID(2)), Title: "原始任务", Now: core.Instant(0)}))
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(ordinaryTestID(1)), TraceID: core.DayTraceID(ordinaryTestID(3)), DayID: core.DayID(ordinaryTestID(4)), Date: date, Today: testDay, Now: core.Instant(0)}))
	return k.Snapshot()
}
func ordinaryTestClass(g core.TaskGraph) ordinary.AggregateState {
	id := ordinary.CategoryID(ordinaryTestID(5))
	return ordinary.AggregateState{FormatVersion: 1, Revision: 7, Categories: map[ordinary.CategoryID]ordinary.Category{id: {ID: id, Name: "当时的名字", ColorHex: "#D87831", PresentationApproval: "userApproved"}}, CurrentByChainID: map[core.TaskChainID]ordinary.Current{g.Chains[0].ID: {Category: &ordinary.CategoryRelation{CategoryID: id}}}}
}
func TestOrdinaryCaptureDerivesHistoricalValueFromSettlement(t *testing.T) {
	g := ordinaryTestGraph(t, testDay)
	s := ordinaryTestClass(g)
	tomorrow, _ := testDay.AddDays(1)
	in := ordinaryTransition{kind: ordinarySettle, settle: ordinarySettlementInput{Today: tomorrow, Now: core.Instant(0)}}
	_, events, e := deriveOrdinaryCaptures(g, s, in, []ordinary.UUID{ordinary.UUID(ordinaryTestID(8))})
	must(t, e)
	if len(events) != 1 || events[0].Status != core.TraceUnfinished || events[0].Revision != 8 || events[0].Category.Name != "当时的名字" {
		t.Fatal(events)
	}
	c := s.Categories[ordinary.CategoryID(ordinaryTestID(5))]
	c.Name = "后来名字"
	s.Categories[c.ID] = c
	if events[0].Category.Name != "当时的名字" {
		t.Fatal("historical alias")
	}
	if _, _, e = deriveOrdinaryCaptures(g, s, in, nil); e == nil {
		t.Fatal("missing exact capture IDs accepted")
	}
	if _, _, e = deriveOrdinaryCaptures(g, s, in, []ordinary.UUID{ordinary.UUID(ordinaryTestID(8)), ordinary.UUID(ordinaryTestID(9))}); e == nil {
		t.Fatal("extra capture accepted")
	}
}
func TestOrdinaryCaptureCompletionAndFutureReturnDoNotCapture(t *testing.T) {
	g := ordinaryTestGraph(t, testDay)
	s := ordinaryTestClass(g)
	in := ordinaryTransition{kind: ordinaryComplete, completion: completionHistoryInput{Action: completionHistoryComplete, TraceID: g.Traces[0].ID, Today: testDay}}
	_, events, e := deriveOrdinaryCaptures(g, s, in, nil)
	must(t, e)
	if len(events) != 0 {
		t.Fatal("premature completed capture")
	}
	tomorrow, _ := testDay.AddDays(1)
	g = ordinaryTestGraph(t, tomorrow)
	in = ordinaryTransition{kind: ordinaryReturn, returning: core.ReturnToPoolInput{TraceID: g.Traces[0].ID, ChainID: g.Chains[0].ID, NewDefinitionID: core.TaskDefinitionID(ordinaryTestID(6)), CancellationID: core.CancellationID(ordinaryTestID(7)), Today: testDay}}
	after, events, e := deriveOrdinaryCaptures(g, s, in, nil)
	must(t, e)
	if len(events) != 0 || after.Traces[0].Status != core.TraceCancelledDraft {
		t.Fatal("draft cancel captured")
	}
}
func TestOrdinaryTransitionExactReplayAndInputOwnership(t *testing.T) {
	before := ordinaryTestGraph(t, testDay)
	in := ordinaryTransition{kind: ordinaryTitle, title: titleHistoryInput{Action: titleHistoryRename, Route: titleHistoryChain, ChainID: before.Chains[0].ID, Title: "新标题", Today: testDay, NewDefinitionID: core.TaskDefinitionID(ordinaryTestID(9))}}
	after, e := in.replay(before)
	must(t, e)
	must(t, in.match(before, after))
	after.Chains[0].CreatedAt = 1
	if e = in.match(before, after); e == nil {
		t.Fatal("unrelated after mutation accepted")
	}
	if !reflect.DeepEqual(before, ordinaryTestGraph(t, testDay)) {
		t.Fatal("before changed")
	}
	if _, e = (ordinaryTransition{}).replay(before); e == nil {
		t.Fatal("zero command accepted")
	}
}
