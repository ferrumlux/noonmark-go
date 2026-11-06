// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"testing"
)

func hsgPool(t *testing.T) *core.TaskKernel {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(1)), DefinitionID: core.TaskDefinitionID(vnID(2)), Title: "parent", Now: vnAt(0)}))
	must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(2)), SubtaskID: core.PlannedSubtaskID(vnID(5)), LineageID: core.SubtaskLineageID(vnID(6)), Title: "child", Difficulty: core.DifficultySimple, Now: vnAt(1)}))
	return k
}
func hsgActual(t *testing.T) *core.TaskKernel {
	t.Helper()
	f := vnSeed(t, vnDate)
	must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(vnID(5)), LineageID: core.SubtaskLineageID(vnID(6)), Title: "child", Difficulty: core.DifficultySimple, Today: vnDate, Now: vnAt(1)}))
	return f.k
}
func hsgPlanned(kind core.SubtaskEditKind) plannedSubtaskHistoryInput {
	in := plannedSubtaskHistoryInput{Kind: kind, ChainID: core.TaskChainID(vnID(1)), ExpectedDefinitionID: core.TaskDefinitionID(vnID(2)), SubtaskID: core.PlannedSubtaskID(vnID(5)), Now: vnAt(2)}
	if kind == core.SubtaskAdd {
		in.SubtaskID = core.PlannedSubtaskID(vnID(7))
		in.LineageID = core.SubtaskLineageID(vnID(8))
	}
	if kind == core.SubtaskAdd || kind == core.SubtaskTitle {
		in.Title = "updated"
	}
	if kind == core.SubtaskAdd || kind == core.SubtaskDifficulty {
		in.Difficulty = core.DifficultySimple
	}
	return in
}
func hsgInput(kind core.SubtaskEditKind) actualSubtaskHistoryInput {
	p := hsgPlanned(kind)
	in := actualSubtaskHistoryInput{Kind: kind, ChainID: p.ChainID, TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(p.SubtaskID), LineageID: p.LineageID, Title: p.Title, Difficulty: p.Difficulty, Today: vnDate, Now: vnAt(2)}
	if kind == core.SubtaskDelete {
		in.CancellationID = core.CancellationID(vnID(9))
	}
	return in
}
func hsgError(t *testing.T, r auditedHistoryCommand, e, want error) {
	t.Helper()
	hcZero(t, r, e)
	if !errors.Is(e, want) {
		t.Fatalf("got %v, want %v", e, want)
	}
}
func TestHistorySubtaskShapeMatrix(t *testing.T) {
	for _, actual := range []bool{false, true} {
		k := hsgPool(t)
		kinds := []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove}
		if actual {
			k = hsgActual(t)
			kinds = []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon}
		}
		g := k.Snapshot()
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		audit := func(in any) (auditedHistoryCommand, error) {
			if actual {
				return auditActualSubtaskHistoryVNext(prior, g, g, *in.(*actualSubtaskHistoryInput))
			}
			return auditPlannedSubtaskHistoryVNext(prior, g, g, *in.(*plannedSubtaskHistoryInput))
		}
		for _, kind := range kinds {
			fresh := func() any {
				if actual {
					in := hsgInput(kind)
					return &in
				}
				in := hsgPlanned(kind)
				return &in
			}
			for _, field := range []string{"LineageID", "Title", "Difficulty", "CancellationID"} {
				if !actual && field == "CancellationID" {
					continue
				}
				allowed := (field == "LineageID" && kind == core.SubtaskAdd) || (field == "Title" && (kind == core.SubtaskAdd || kind == core.SubtaskTitle)) || (field == "Difficulty" && (kind == core.SubtaskAdd || kind == core.SubtaskDifficulty)) || (field == "CancellationID" && kind == core.SubtaskDelete)
				if allowed {
					continue
				}
				for _, bad := range []string{" ", string([]byte{255})} {
					in := fresh()
					v := reflect.ValueOf(in).Elem().FieldByName(field)
					if field == "Difficulty" {
						v.SetInt(int64(core.DifficultySimple))
					} else {
						v.SetString(bad)
					}
					r, e := audit(in)
					hsgError(t, r, e, core.ErrInvalidInput)
				}
			}
			if kind == core.SubtaskAdd || kind == core.SubtaskTitle {
				for _, bad := range []string{"", " \r\n ", string([]byte{255})} {
					in := fresh()
					reflect.ValueOf(in).Elem().FieldByName("Title").SetString(bad)
					r, e := audit(in)
					hsgError(t, r, e, core.ErrInvalidTitle)
				}
			}
		}
		for _, kind := range []core.SubtaskEditKind{"", "unknown", map[bool]core.SubtaskEditKind{false: core.SubtaskDelete, true: core.PlannedSubtaskRemove}[actual]} {
			var in any
			if actual {
				v := hsgInput(kind)
				in = &v
			} else {
				v := hsgPlanned(kind)
				in = &v
			}
			r, e := audit(in)
			hsgError(t, r, e, core.ErrInvalidInput)
		}
	}
}
func TestHistorySubtaskNoopAndOwnerGuards(t *testing.T) {
	for _, actual := range []bool{false, true} {
		k := hsgPool(t)
		if actual {
			k = hsgActual(t)
		}
		g := k.Snapshot()
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty} {
			for _, clock := range []float64{-10, math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64, math.Copysign(0, -1)} {
				p := hsgPlanned(kind)
				a := hsgInput(kind)
				p.Now = core.Instant(math.Float64bits(clock))
				a.Now = p.Now
				if kind == core.SubtaskTitle {
					p.Title = " child "
					a.Title = p.Title
				}
				var r auditedHistoryCommand
				var e error
				if actual {
					r, e = auditActualSubtaskHistoryVNext(prior, g, g, a)
				} else {
					r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, p)
				}
				must(t, e)
				changed, e := r.changed()
				must(t, e)
				if changed || r.proof.now != p.Now {
					t.Fatal("no-op changed state or lost request clock bits")
				}
				if actual {
					a.ChainID = core.TaskChainID(vnID(99))
					r, e = auditActualSubtaskHistoryVNext(prior, g, g, a)
					hsgError(t, r, e, core.ErrInvalidInput)
				} else {
					p.ExpectedDefinitionID = core.TaskDefinitionID(vnID(99))
					r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, p)
					hsgError(t, r, e, core.ErrInvalidTransition)
				}
			}
		}
	}
}
func TestHistorySubtaskCoreBeforeAfterMismatch(t *testing.T) {
	k := hsgPool(t)
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	in := hsgPlanned(core.SubtaskTitle)
	in.Now = vnAt(-1)
	bad := g.Clone()
	bad.Definitions[0].ID = "invalid"
	r, e := auditPlannedSubtaskHistoryVNext(prior, g, bad, in)
	hsgError(t, r, e, core.ErrInvalidClock)
	in.Now = vnAt(2)
	r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, in)
	hcZero(t, r, e)
	var mismatch *historyCommandError
	if !errors.As(e, &mismatch) || mismatch.Reason != historyCommandReplay {
		t.Fatalf("valid after mismatch: %v", e)
	}
	r, e = auditPlannedSubtaskHistoryVNext(prior, g, bad, in)
	hcZero(t, r, e)
	if errors.As(e, &mismatch) {
		t.Fatalf("invalid after must preserve bridge validation error: %v", e)
	}
	mismatchBefore := g.Clone()
	mismatchBefore.Definitions[0].Title = "other"
	in.Kind = "invalid"
	r, e = auditPlannedSubtaskHistoryVNext(prior, mismatchBefore, g, in)
	hcZero(t, r, e)
	if !errors.As(e, &mismatch) || mismatch.Reason != historyCommandBinding {
		t.Fatalf("binding must precede core: %v", e)
	}
}

func TestHistorySubtaskActualDateOwnerAndClockOrder(t *testing.T) {
	kinds := []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon}
	for _, offset := range []int{-1, 0, 1} {
		k := hsgActual(t)
		g := k.Snapshot()
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		today, e := vnDate.AddDays(offset)
		must(t, e)
		for _, kind := range kinds {
			in := hsgInput(kind)
			in.Today = today
			in.Now = core.Instant(math.Float64bits(math.NaN()))
			// Wrong chain wins over past/date; missing trace wins over every parent gate.
			in.ChainID = core.TaskChainID(vnID(99))
			r, e := auditActualSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, core.ErrInvalidInput)
			in.ChainID = core.TaskChainID(vnID(1))
			in.TraceID = core.DayTraceID(vnID(99))
			r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, core.ErrNotFound)
			in.TraceID = core.DayTraceID(vnID(3))
			if offset == 1 || (offset == -1 && (kind == core.SubtaskComplete || kind == core.SubtaskUndoComplete || kind == core.SubtaskAbandon)) {
				r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
				hsgError(t, r, e, core.ErrImmutableHistory)
				continue
			}
			if kind == core.SubtaskDifficulty {
				in.Difficulty = core.DifficultyHard
			}
			want := core.ErrInvalidClock
			if kind == core.SubtaskUndoComplete {
				want = core.ErrInvalidTransition
			}
			r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, want)
		}
	}
	// Real settlement creates a locked U parent. Lock must win over add's non-P status error.
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{op: "L", at: 2})
	g := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	for _, kind := range kinds {
		in := hsgInput(kind)
		in.Now = core.Instant(math.Float64bits(math.NaN()))
		r, e := auditActualSubtaskHistoryVNext(prior, g, g, in)
		hsgError(t, r, e, core.ErrImmutableHistory)
	}
}
func TestHistorySubtaskGlobalIdentityAndHiddenChild(t *testing.T) {
	k := hsgActual(t)
	// A second trace supplies a wrong-owner child and global hidden witness.
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(20)), DefinitionID: core.TaskDefinitionID(vnID(21)), Title: "other", Now: vnAt(0)}))
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(vnID(20)), TraceID: core.DayTraceID(vnID(22)), DayID: core.DayID(vnID(4)), Date: vnDate, Today: vnDate, Now: vnAt(0)}))
	must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(20)), TraceID: core.DayTraceID(vnID(22)), SubtaskID: core.SubtaskID(vnID(23)), LineageID: core.SubtaskLineageID(vnID(24)), Title: "other child", Difficulty: core.DifficultySimple, Today: vnDate, Now: vnAt(1)}))
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	in := hsgInput(core.SubtaskTitle)
	in.SubtaskID = core.SubtaskID(vnID(23))
	in.Now = core.Instant(math.Float64bits(math.NaN()))
	r, e := auditActualSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrInvalidInput)
	in.SubtaskID = core.SubtaskID(vnID(99))
	r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrNotFound)
	must(t, k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskDelete, ChainID: core.TaskChainID(vnID(20)), TraceID: core.DayTraceID(vnID(22)), SubtaskID: core.SubtaskID(vnID(23)), CancellationID: core.CancellationID(vnID(25)), Today: vnDate, Now: vnAt(2)}))
	g = k.Snapshot()
	prior = vnGood(t, hcWire(t, g, canonical.Value{}))
	in = hsgInput(core.SubtaskDelete)
	in.TraceID = core.DayTraceID(vnID(99))
	in.CancellationID = core.CancellationID(vnID(25))
	r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrInvalidInput)
	in = hsgInput(core.SubtaskAdd)
	in.TraceID = core.DayTraceID(vnID(99))
	in.LineageID = core.SubtaskLineageID(vnID(24))
	r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrInvalidInput)
	p := hsgPlanned(core.SubtaskAdd)
	p.ChainID = core.TaskChainID(vnID(99))
	p.LineageID = core.SubtaskLineageID(vnID(24))
	r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, p)
	hsgError(t, r, e, core.ErrInvalidInput)
	for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete} {
		in = hsgInput(kind)
		in.ChainID = core.TaskChainID(vnID(20))
		in.TraceID = core.DayTraceID(vnID(22))
		in.SubtaskID = core.SubtaskID(vnID(23))
		in.Now = core.Instant(math.Float64bits(math.NaN()))
		r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
		hsgError(t, r, e, core.ErrNotFound)
	}
}
func TestHistorySubtaskPlannedOwnerAndClockOrder(t *testing.T) {
	for _, clockField := range []string{"chain", "definition"} {
		k := hsgPool(t)
		g := k.Snapshot()
		if clockField == "chain" {
			g.Chains[0].UpdatedAt = vnAt(10)
		} else {
			g.Definitions[0].ContentUpdatedAt = vnAt(10)
		}
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
			in := hsgPlanned(kind)
			if kind == core.SubtaskDifficulty {
				in.Difficulty = core.DifficultyHard
			}
			r, e := auditPlannedSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, core.ErrInvalidClock)
			in.ChainID = core.TaskChainID(vnID(99))
			r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, core.ErrNotFound)
			in.ChainID = core.TaskChainID(vnID(1))
			in.ExpectedDefinitionID = core.TaskDefinitionID(vnID(99))
			r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, in)
			hsgError(t, r, e, core.ErrInvalidTransition)
		}
	}
	// An active scheduled parent cannot use planned commands, regardless of invalid Now.
	k := hsgActual(t)
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	in := hsgPlanned(core.SubtaskTitle)
	in.Now = core.Instant(math.Float64bits(math.NaN()))
	r, e := auditPlannedSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrInvalidTransition)
}

func TestHistorySubtaskParentStatusMatrix(t *testing.T) {
	for _, op := range []string{"A", "C", "R", "D", "X", "L"} {
		f := vnSeed(t, vnDate)
		f.run(t, vnStep{op: op, at: 2})
		g := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
			in := hsgInput(kind)
			if kind == core.SubtaskDelete {
				in.CancellationID = core.CancellationID(vnID(90))
			}
			in.Now = core.Instant(math.Float64bits(math.NaN()))
			r, e := auditActualSubtaskHistoryVNext(prior, g, g, in)
			want := core.ErrImmutableHistory
			if kind == core.SubtaskAdd && op != "L" {
				want = core.ErrInvalidTransition
			}
			hsgError(t, r, e, want)
		}
	}
}
func TestHistorySubtaskUUIDAndFreshnessPrecedence(t *testing.T) {
	for _, actual := range []bool{false, true} {
		k := hsgPool(t)
		if actual {
			k = hsgActual(t)
		}
		g := k.Snapshot()
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		audit := func(in any) (auditedHistoryCommand, error) {
			if actual {
				return auditActualSubtaskHistoryVNext(prior, g, g, *in.(*actualSubtaskHistoryInput))
			}
			return auditPlannedSubtaskHistoryVNext(prior, g, g, *in.(*plannedSubtaskHistoryInput))
		}
		fresh := func() any {
			if actual {
				in := hsgInput(core.SubtaskAdd)
				return &in
			}
			in := hsgPlanned(core.SubtaskAdd)
			return &in
		}
		fields := []string{"ChainID", "SubtaskID", "LineageID"}
		if actual {
			fields = append(fields, "TraceID")
		} else {
			fields = append(fields, "ExpectedDefinitionID")
		}
		for _, field := range fields {
			for _, bad := range []string{"", "short", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "GGGGGGGG-GGGG-GGGG-GGGG-GGGGGGGGGGGG"} {
				in := fresh()
				reflect.ValueOf(in).Elem().FieldByName(field).SetString(bad)
				r, e := audit(in)
				hsgError(t, r, e, core.ErrInvalidInput)
			}
		}
		in := fresh()
		v := reflect.ValueOf(in).Elem()
		v.FieldByName("ChainID").SetString(vnID(99))
		v.FieldByName("LineageID").SetString(vnID(6))
		r, e := audit(in)
		hsgError(t, r, e, core.ErrInvalidInput)
		// Duplicate child ID fails after owner lookup, unlike fresh lineage.
		in = fresh()
		v = reflect.ValueOf(in).Elem()
		v.FieldByName("SubtaskID").SetString(vnID(5))
		r, e = audit(in)
		hsgError(t, r, e, core.ErrInvalidInput)
	}
	k := hsgPool(t)
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	in := hsgPlanned(core.SubtaskTitle)
	in.SubtaskID = core.PlannedSubtaskID(vnID(99))
	in.Now = core.Instant(math.Float64bits(math.NaN()))
	r, e := auditPlannedSubtaskHistoryVNext(prior, g, g, in)
	hsgError(t, r, e, core.ErrNotFound)
}

func TestHistorySubtaskInvalidDifficultyAndCancellation(t *testing.T) {
	for _, actual := range []bool{false, true} {
		k := hsgPool(t)
		if actual {
			k = hsgActual(t)
		}
		g := k.Snapshot()
		prior := vnGood(t, hcWire(t, g, canonical.Value{}))
		for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskDifficulty} {
			for _, d := range []core.Difficulty{0, -1, 4} {
				var r auditedHistoryCommand
				var e error
				if actual {
					in := hsgInput(kind)
					in.Difficulty = d
					r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
				} else {
					in := hsgPlanned(kind)
					in.Difficulty = d
					r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, in)
				}
				hsgError(t, r, e, core.ErrInvalidInput)
			}
		}
	}
	k := hsgActual(t)
	g := k.Snapshot()
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	for _, id := range []core.CancellationID{"", "short", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"} {
		in := hsgInput(core.SubtaskDelete)
		in.CancellationID = id
		in.TraceID = core.DayTraceID(vnID(99))
		r, e := auditActualSubtaskHistoryVNext(prior, g, g, in)
		hsgError(t, r, e, core.ErrInvalidInput)
	}
}
