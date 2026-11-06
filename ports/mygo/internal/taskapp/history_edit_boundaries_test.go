// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"strings"
	"testing"
)

func editPrior(t *testing.T, g core.TaskGraph) validatedHistorySnapshot {
	t.Helper()
	f := vnSeed(t, vnDate)
	return vnGood(t, lifecycleSnapshot(t, g, f.c))
}
func editTitle(t *testing.T, g core.TaskGraph, in titleHistoryInput) (core.TaskGraph, error) {
	t.Helper()
	k, e := core.NewTaskKernel(g)
	must(t, e)
	x := core.EditTaskTitleInput{ChainID: in.ChainID, Title: in.Title, Today: in.Today, Now: in.Now, NewDefinitionID: in.NewDefinitionID}
	if in.Action == titleHistoryRename {
		e = k.RenameTaskTitle(x)
	} else {
		e = k.SaveTaskTitleInput(x)
	}
	return k.Snapshot(), e
}
func editInput(g core.TaskGraph) titleHistoryInput {
	return titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: g.Chains[0].ID, Title: "changed", Today: vnDate, Now: vnAt(5), NewDefinitionID: core.TaskDefinitionID(vnID(70))}
}
func editCheck(t *testing.T, p validatedHistorySnapshot, r auditedHistoryCommand, e error) {
	t.Helper()
	must(t, e)
	if !r.valid() {
		t.Fatal("invalid result")
	}
	s, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(s.Object["classifications"], p.proof.snapshot.Object["classifications"]) || r.proof.after.proof.count != p.proof.count {
		t.Fatal("capture drift")
	}
	if r.proof.changed == historyEqual(s, p.proof.snapshot) {
		t.Fatal("changed drift")
	}
}

func TestHistoryEditTitleBranches(t *testing.T) {
	for _, pool := range []bool{false, true} {
		for _, action := range []titleHistoryAction{titleHistoryRename, titleHistorySaveInput} {
			for _, unused := range []string{"", "bad", vnID(2), string([]byte{255})} {
				f := vnSeed(t, vnDate)
				g := f.k.Snapshot()
				if pool {
					g.Traces = nil
					g.Days = nil
				}
				p := editPrior(t, g)
				in := editInput(g)
				in.Action = action
				in.NewDefinitionID = core.TaskDefinitionID(unused)
				version := !pool && action == titleHistoryRename
				if version {
					in.NewDefinitionID = core.TaskDefinitionID(vnID(70))
				}
				after, e := editTitle(t, g, in)
				must(t, e)
				r, e := auditTitleHistoryVNext(p, g, after, in)
				editCheck(t, p, r, e)
				if version {
					if len(after.Definitions) != 2 || after.Traces[0].DefinitionID != in.NewDefinitionID || after.Definitions[0].Title != g.Definitions[0].Title {
						t.Fatal("version delta")
					}
				} else {
					if len(after.Definitions) != 1 || after.Definitions[0].ID != g.Definitions[0].ID || !reflect.DeepEqual(g.Traces, after.Traces) {
						t.Fatal("reuse delta")
					}
				}
			}
		}
	}
}
func TestHistoryEditTitleNormalizationAndUnusedClocks(t *testing.T) {
	for _, now := range []core.Instant{core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), core.Instant(math.Float64bits(math.Inf(-1))), vnAt(-10), vnAt(math.MaxFloat64), vnAt(math.Copysign(0, -1))} {
		g := vnSeed(t, vnDate).k.Snapshot()
		g.Definitions[0].Title = "é"
		p := editPrior(t, g)
		in := editInput(g)
		in.Title = " \r\ne\u0301\r\n "
		in.Now = now
		in.NewDefinitionID = "\xff"
		r, e := auditTitleHistoryVNext(p, g, g, in)
		editCheck(t, p, r, e)
		if r.proof.changed || r.proof.now != now || r.proof.requestedNewDefinitionID != "\xff" {
			t.Fatal("no-op consumed parameters")
		}
	}
	for _, tc := range []struct{ raw, want string }{{" \rA\r\nB\r ", "A\nB"}, {"\u2003\t", ""}, {"a" + strings.Repeat("\u0301", 64), "a" + strings.Repeat("\u0301", 64)}} {
		g := vnSeed(t, vnDate).k.Snapshot()
		in := editInput(g)
		in.Title = tc.raw
		a, e := editTitle(t, g, in)
		must(t, e)
		r, e := auditTitleHistoryVNext(editPrior(t, g), g, a, in)
		must(t, e)
		if !r.valid() || a.Definitions[0].Title != tc.want {
			t.Fatal("normalization")
		}
	}
	g := vnSeed(t, vnDate).k.Snapshot()
	in := editInput(g)
	in.Title = "\xff"
	r, e := auditTitleHistoryVNext(editPrior(t, g), g, g, in)
	hcZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal(e)
	}
}
func TestHistoryEditTitleClockBits(t *testing.T) {
	for _, tc := range []struct{ old, now float64 }{{math.Copysign(0, -1), 0}, {0, math.Copysign(0, -1)}, {-3, -2}, {5, 5}, {20, 5}} {
		g := vnSeed(t, vnDate).k.Snapshot()
		g.Traces = nil
		g.Days = nil
		g.Chains[0].CreatedAt = vnAt(-10)
		g.Chains[0].UpdatedAt = vnAt(tc.old)
		g.Definitions[0].CreatedAt = vnAt(-10)
		g.Definitions[0].ContentUpdatedAt = vnAt(-5)
		p := editPrior(t, g)
		in := editInput(g)
		in.Now = vnAt(tc.now)
		a, e := editTitle(t, g, in)
		must(t, e)
		r, e := auditTitleHistoryVNext(p, g, a, in)
		editCheck(t, p, r, e)
		want := vnAt(tc.old)
		if tc.now > tc.old {
			want = vnAt(tc.now)
		}
		if a.Chains[0].UpdatedAt != want {
			t.Fatal("max/tie bits")
		}
	}
	g := vnSeed(t, vnDate).k.Snapshot()
	in := editInput(g)
	in.Now = vnAt(-1)
	r, e := auditTitleHistoryVNext(editPrior(t, g), g, g, in)
	hcZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal(e)
	}
}
func TestHistoryEditManualClampNoopAndOwner(t *testing.T) {
	for _, tc := range []struct{ value, want int }{{-1, 0}, {0, 0}, {50, 50}, {100, 100}, {101, 100}, {int(^uint(0) >> 1), 100}, {-int(^uint(0)>>1) - 1, 0}} {
		f := vnSeed(t, vnDate)
		g := f.k.Snapshot()
		p := editPrior(t, g)
		in := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Percent: tc.value, Today: vnDate, Now: vnAt(0)}
		must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
		a := f.k.Snapshot()
		r, e := auditManualProgressHistoryVNext(p, g, a, in)
		editCheck(t, p, r, e)
		if *a.Traces[0].ManualProgressPercent != tc.want || a.Traces[0].Status != core.TracePending || !reflect.DeepEqual(a.Chains, g.Chains) || !r.proof.changed {
			t.Fatal("clamp or nil delta")
		}
		for _, now := range []core.Instant{core.Instant(math.Float64bits(math.NaN())), core.Instant(math.Float64bits(math.Inf(1))), vnAt(-1), vnAt(math.MaxFloat64)} {
			in.Now = now
			p2 := editPrior(t, a)
			r, e = auditManualProgressHistoryVNext(p2, a, a, in)
			editCheck(t, p2, r, e)
			if r.proof.changed {
				t.Fatal("manual no-op")
			}
		}
	}
	g := vnSeed(t, vnDate).k.Snapshot()
	in := manualProgressHistoryInput{ChainID: core.TaskChainID(vnID(999)), TraceID: g.Traces[0].ID, Today: vnDate, Now: vnAt(1)}
	r, e := auditManualProgressHistoryVNext(editPrior(t, g), g, g, in)
	hcZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidInput) {
		t.Fatal("core owner error", e)
	}
}
func TestHistoryEditBudgetBoundaries(t *testing.T) {
	g := vnSeed(t, vnDate).k.Snapshot()
	in := editInput(g)
	in.TraceID = ""
	in.NewDefinitionID = ""
	max := canonical.DefaultLimits().MaxBytes
	in.Title = strings.Repeat(" ", max-len(in.ChainID))
	must(t, historyCommandTitleInputBudget(in))
	in.Title += " "
	hcBudgetError(t, historyCommandTitleInputBudget(in), historyCommandInputFields, historyCommandRawBytes)
	shared := strings.Repeat("x", max/2+1)
	in.Title = shared
	in.NewDefinitionID = core.TaskDefinitionID(shared)
	hcBudgetError(t, historyCommandTitleInputBudget(in), historyCommandInputFields, historyCommandRawBytes)
	p := editPrior(t, g)
	bad := g.Clone()
	bad.Traces[0].ID = "bad"
	huge := g.Clone()
	huge.Definitions[0].Title = strings.Repeat("x", max+1)
	for _, tc := range []struct {
		b, a  core.TaskGraph
		phase historyCommandPhase
	}{{huge, bad, historyCommandBefore}, {bad, huge, historyCommandAfter}, {g, g, historyCommandInputFields}} {
		r, e := auditTitleHistoryVNext(p, tc.b, tc.a, in)
		hcZero(t, r, e)
		hcBudgetError(t, e, tc.phase, historyCommandRawBytes)
	}
	r, e := auditTitleHistoryVNext(validatedHistorySnapshot{}, huge, huge, in)
	hcZero(t, r, e)
	var budget *historyCommandBudgetError
	if errors.As(e, &budget) {
		t.Fatal("seal precedence")
	}
	for _, field := range []string{"title", "chain", "trace", "new"} {
		v := editInput(g)
		switch field {
		case "title":
			v.Title = shared + shared
		case "chain":
			v.ChainID = core.TaskChainID(shared + shared)
		case "trace":
			v.TraceID = core.DayTraceID(shared + shared)
		case "new":
			v.NewDefinitionID = core.TaskDefinitionID(shared + shared)
		}
		hcBudgetError(t, historyCommandTitleInputBudget(v), historyCommandInputFields, historyCommandRawBytes)
	}
}
