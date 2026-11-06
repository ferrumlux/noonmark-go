// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestHistoryRemainingEveryRawInputField(t *testing.T) {
	g := hsActual(t)
	p := vnGood(t, hsSnapshot(t, g))
	huge := strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	cases := []struct {
		in     any
		budget func(any) error
	}{
		{returnHistoryInput{}, func(v any) error { return historyCommandReturnInputBudget(p, v.(returnHistoryInput)) }},
		{deferHistoryInput{}, func(v any) error { return historyCommandDeferInputBudget(p, v.(deferHistoryInput)) }},
		{continueHistoryInput{}, func(v any) error { return historyCommandContinueInputBudget(p, v.(continueHistoryInput)) }},
	}
	for _, c := range cases {
		typ := reflect.TypeOf(c.in)
		for i := 0; i < typ.NumField(); i++ {
			ft := typ.Field(i)
			if ft.Type.Kind() != reflect.String {
				continue
			}
			t.Run(typ.Name()+"/"+ft.Name, func(t *testing.T) {
				v := reflect.New(typ).Elem()
				v.Field(i).SetString(huge)
				hcBudgetError(t, c.budget(v.Interface()), historyCommandInputFields, historyCommandRawBytes)
			})
		}
	}
	child := g.Subtasks[0].ID
	for _, m := range []map[core.SubtaskID]core.PlannedSubtaskID{{core.SubtaskID(huge): "x"}, {child: core.PlannedSubtaskID(huge)}} {
		hcBudgetError(t, historyCommandReturnInputBudget(p, returnHistoryInput{PlannedIDs: m}), historyCommandInputFields, historyCommandRawBytes)
	}
	for _, m := range []map[core.SubtaskID]core.SubtaskID{{core.SubtaskID(huge): "x"}, {child: core.SubtaskID(huge)}} {
		hcBudgetError(t, historyCommandDeferInputBudget(p, deferHistoryInput{SubtaskIDs: m}), historyCommandInputFields, historyCommandRawBytes)
		hcBudgetError(t, historyCommandContinueInputBudget(p, continueHistoryInput{SubtaskIDs: m}), historyCommandInputFields, historyCommandRawBytes)
	}
	// Charge logical occurrences even when all headers share one backing string.
	s := strings.Repeat("a", canonical.DefaultLimits().MaxBytes/4+1)
	hcBudgetError(t, historyCommandReturnInputBudget(p, returnHistoryInput{ChainID: core.TaskChainID(s), TraceID: core.DayTraceID(s), NewDefinitionID: core.TaskDefinitionID(s), CancellationID: core.CancellationID(s)}), historyCommandInputFields, historyCommandRawBytes)
}
func TestHistoryRemainingAllEntryBudgetPrecedence(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	huge := g.Clone()
	huge.Definitions[0].Title = strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	calls := []func(validatedHistorySnapshot, core.TaskGraph, core.TaskGraph) (auditedHistoryCommand, error){
		func(p validatedHistorySnapshot, b, a core.TaskGraph) (auditedHistoryCommand, error) {
			return auditReturnHistoryVNext(p, b, a, returnHistoryInput{PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{"bad": "bad"}})
		},
		func(p validatedHistorySnapshot, b, a core.TaskGraph) (auditedHistoryCommand, error) {
			return auditDeferHistoryVNext(p, b, a, deferHistoryInput{SubtaskIDs: map[core.SubtaskID]core.SubtaskID{"bad": "bad"}})
		},
		func(p validatedHistorySnapshot, b, a core.TaskGraph) (auditedHistoryCommand, error) {
			return auditContinueHistoryVNext(p, b, a, continueHistoryInput{SubtaskIDs: map[core.SubtaskID]core.SubtaskID{"bad": "bad"}})
		},
	}
	for _, call := range calls {
		r, e := call(validatedHistorySnapshot{}, huge, huge)
		lifecycleZero(t, r, e)
		var budget *historyCommandBudgetError
		if errors.As(e, &budget) {
			t.Fatal("seal lost precedence")
		}
		r, e = call(p, huge, huge)
		lifecycleZero(t, r, e)
		hcBudgetError(t, e, historyCommandBefore, historyCommandRawBytes)
		r, e = call(p, g, huge)
		lifecycleZero(t, r, e)
		hcBudgetError(t, e, historyCommandAfter, historyCommandRawBytes)
		r, e = call(p, g, g)
		lifecycleZero(t, r, e)
		hcBudgetError(t, e, historyCommandInputFields, historyCommandIDCount)
	}
}
func TestHistoryRemainingCaptureErrorsAfterMatch(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 2}, vnStep{"P", 3})
	g := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := returnHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(880)), Today: vnDate, Now: vnAt(30)}
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, Today: in.Today, Now: in.Now}))
	after := k.Snapshot()
	for _, event := range []string{"bad", strings.ToLower(f.c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["id"].String)} {
		in.EventID = event
		r, e := auditReturnHistoryVNext(p, g, after, in)
		lifecycleZero(t, r, e)
		var ce *historyCommandError
		if errors.As(e, &ce) && ce.Reason == historyCommandCount {
			t.Fatal("nonempty event should reach builder")
		}
	}
	in.EventID = ""
	wrong := after.Clone()
	wrong.Definitions[len(wrong.Definitions)-1].Title = "different"
	r, e := auditReturnHistoryVNext(p, g, wrong, in)
	lifecycleZero(t, r, e)
	var ce *historyCommandError
	if !errors.As(e, &ce) || ce.Reason != historyCommandReplay {
		t.Fatal("count hid match error", e)
	}
}
