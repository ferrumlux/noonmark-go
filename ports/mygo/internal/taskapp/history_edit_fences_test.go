// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

func TestHistoryEditIndependentFencesAndBinding(t *testing.T) {
	g := vnSeed(t, vnDate).k.Snapshot()
	g.Definitions[0].DescriptionText = vnPtr("")
	in := editInput(g)
	in.Action = titleHistoryRename
	p := editPrior(t, g)
	owned, _, e := historyCommandBind(p, g)
	must(t, e)
	a, e := editTitle(t, owned, in)
	must(t, e)
	for _, tc := range []struct {
		name   string
		change func(*core.TaskGraph)
	}{
		{"chain", func(x *core.TaskGraph) { x.Chains[0].CreatedAt = vnAt(-1) }},
		{"day", func(x *core.TaskGraph) { x.Days[0].UpdatedAt = vnAt(3) }},
		{"old body", func(x *core.TaskGraph) { x.Definitions[0].Title = "bad" }},
		{"old description", func(x *core.TaskGraph) { x.Definitions[0].DescriptionText = nil }},
		{"new description", func(x *core.TaskGraph) { x.Definitions[1].DescriptionText = nil }},
		{"sequence", func(x *core.TaskGraph) { x.Definitions[1].Sequence++ }},
		{"new time", func(x *core.TaskGraph) { x.Definitions[1].CreatedAt = vnAt(1) }},
		{"trace note", func(x *core.TaskGraph) { x.Traces[0].DescriptionText = vnPtr("extra") }},
		{"trace manual", func(x *core.TaskGraph) { x.Traces[0].ManualProgressPercent = vnPtr(1) }},
		{"trace reference", func(x *core.TaskGraph) { x.Traces[0].DefinitionID = g.Definitions[0].ID }},
		{"reorder definitions", func(x *core.TaskGraph) { x.Definitions[0], x.Definitions[1] = x.Definitions[1], x.Definitions[0] }},
		{"extra definition", func(x *core.TaskGraph) { x.Definitions = append(x.Definitions, x.Definitions[1]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := a.Clone()
			tc.change(&bad)
			if historyTitleFence(owned, bad, in) {
				t.Fatal("independent fence accepted")
			}
			r, e := auditTitleHistoryVNext(p, g, bad, in)
			hcZero(t, r, e)
		})
	}
	badBefore := g.Clone()
	badBefore.Definitions[0].Title = "unbound"
	r, e := auditTitleHistoryVNext(p, badBefore, a, in)
	hcZero(t, r, e)
	mi := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Today: vnDate, Now: vnAt(5), Percent: 50}
	k, e := core.NewTaskKernel(owned)
	must(t, e)
	must(t, k.SetManualProgress(core.SetManualProgressInput(mi)))
	ma := k.Snapshot()
	for _, change := range []func(*core.TaskGraph){func(x *core.TaskGraph) { x.Chains[0].UpdatedAt = vnAt(5) }, func(x *core.TaskGraph) { x.Traces[0].DescriptionText = vnPtr("x") }, func(x *core.TaskGraph) { x.Definitions[0].Title = "x" }, func(x *core.TaskGraph) { x.Traces[0].ID = core.DayTraceID(vnID(99)) }} {
		bad := ma.Clone()
		change(&bad)
		if historyManualProgressFence(owned, bad, mi) {
			t.Fatal("manual independent fence")
		}
		r, e := auditManualProgressHistoryVNext(p, g, bad, mi)
		hcZero(t, r, e)
	}
}
func TestHistoryEditDetachedEndpointsAndKinds(t *testing.T) {
	for _, action := range []titleHistoryAction{titleHistoryRename, titleHistorySaveInput} {
		g := vnSeed(t, vnDate).k.Snapshot()
		g.Definitions[0].DescriptionText = vnPtr("description")
		p := editPrior(t, g)
		original, e := p.snapshotCopy()
		must(t, e)
		in := editInput(g)
		in.Action = action
		a, e := editTitle(t, g, in)
		must(t, e)
		r, e := auditTitleHistoryVNext(p, g, a, in)
		editCheck(t, p, r, e)
		saved, e := r.snapshotCopy()
		must(t, e)
		*g.Definitions[0].DescriptionText = "caller before"
		*a.Definitions[0].DescriptionText = "caller after"
		g.Definitions[0].Title = "before"
		copy1, e := r.snapshotCopy()
		must(t, e)
		copy1.Object["definitions"].Array[0].Object["title"] = historyString("copy one")
		copy2, e := r.snapshotCopy()
		must(t, e)
		if !historyEqual(saved, copy2) || !historyEqual(original, p.proof.snapshot) {
			t.Fatal("endpoint alias")
		}
		copy2.Object["chains"].Array[0].Object["id"] = historyString("copy two")
		copy3, e := r.snapshotCopy()
		must(t, e)
		if !historyEqual(copy3, saved) {
			t.Fatal("second copy alias")
		}
	}
	p := editPrior(t, vnSeed(t, vnDate).k.Snapshot())
	for _, kind := range []historyCommandKind{historyCommandRename, historyCommandTitleInput, historyCommandManualProgress, historyCommandPlannedSubtaskEdit, historyCommandActualSubtaskEdit} {
		r := auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, prior: p, after: p}}
		if !r.valid() {
			t.Fatal("new kind registration")
		}
	}
	for _, kind := range []historyCommandKind{0, historyCommandActualSubtaskEdit + 1, 255} {
		if (auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: kind, prior: p, after: p}}).valid() {
			t.Fatal("unknown kind")
		}
	}
	typ := reflect.TypeOf(historyCommandProof{})
	for _, name := range []string{"titleAction", "titleRoute", "requestedTitle", "requestedNewDefinitionID", "requestedPercent"} {
		f, ok := typ.FieldByName(name)
		if !ok || f.IsExported() || f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Map {
			t.Fatal("request identity shape")
		}
	}
}
