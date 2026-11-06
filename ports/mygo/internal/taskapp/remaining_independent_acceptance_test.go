package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Existing pre-batch fixture helpers only supply seeds and explicit history.
// Assertions below are independent field deltas/constants, not replay equality.
func riChild(t *testing.T, f *vnFixture, n int) {
	t.Helper()
	must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(vnID(n)), LineageID: core.SubtaskLineageID(vnID(n + 1000)), Title: "child", Difficulty: core.DifficultyHard, Today: f.date, Now: vnAt(0)}))
}
func riGraph(t *testing.T, p validatedHistorySnapshot) core.TaskGraph {
	t.Helper()
	s, _, e := v7.ToDomain(p.proof.snapshot)
	must(t, e)
	return s.Tasks
}
func riSeal(t *testing.T, g core.TaskGraph, c canonical.Value) validatedHistorySnapshot {
	t.Helper()
	x, e := emptySupplement()
	must(t, e)
	x.Classifications = c
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, x)
	must(t, e)
	return vnGood(t, s)
}
func riZero(t *testing.T, r auditedHistoryCommand, e error) {
	t.Helper()
	if e == nil || r.proof != nil {
		t.Fatalf("failure not zero: %v", e)
	}
}
func riReturn(f *vnFixture, now core.Instant) returnHistoryInput {
	return returnHistoryInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), NewDefinitionID: core.TaskDefinitionID(vnID(80)), CancellationID: core.CancellationID(vnID(81)), Today: f.date, Now: now, EventID: vnID(900)}
}
func riReplayR(t *testing.T, g core.TaskGraph, in returnHistoryInput) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: in.ChainID, TraceID: in.TraceID, NewDefinitionID: in.NewDefinitionID, CancellationID: in.CancellationID, PlannedIDs: in.PlannedIDs, Today: in.Today, Now: in.Now}))
	return k.Snapshot()
}
func riCarry(f *vnFixture, now core.Instant) core.TraceCarryoverInput {
	d, _ := f.date.AddDays(1)
	return core.TraceCarryoverInput{ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), NewTraceID: core.DayTraceID(vnID(82)), DayID: core.DayID(vnID(83)), TargetDate: d, Today: f.date, Now: now}
}
func riD(i core.TraceCarryoverInput) deferHistoryInput {
	return deferHistoryInput{ChainID: i.ChainID, TraceID: i.TraceID, NewTraceID: i.NewTraceID, DayID: i.DayID, SubtaskIDs: i.SubtaskIDs, Today: i.Today, TargetDate: i.TargetDate, Now: i.Now, EventID: vnID(900)}
}
func riC(i core.TraceCarryoverInput) continueHistoryInput {
	return continueHistoryInput{ChainID: i.ChainID, TraceID: i.TraceID, NewTraceID: i.NewTraceID, DayID: i.DayID, SubtaskIDs: i.SubtaskIDs, Today: i.Today, TargetDate: i.TargetDate, Now: i.Now}
}
func riEvents(p validatedHistorySnapshot) []canonical.Value {
	return p.proof.snapshot.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array
}

func TestRemainingIndependentReturnPaths(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "R", true: "X"}[future], func(t *testing.T) {
			f := vnSeed(t, vnDate)
			riChild(t, f, 20)
			riChild(t, f, 21)
			if future {
				must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskDelete, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), SubtaskID: core.SubtaskID(vnID(21)), Today: core.LocalDate{Year: 2026, Month: 10, Day: 31}, Now: vnAt(1), CancellationID: core.CancellationID(vnID(90))}))
			} else {
				f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
			}
			b := f.k.Snapshot()
			p := vnGood(t, f.snapshot(t))
			in := riReturn(f, vnAt(5))
			in.PlannedIDs = map[core.SubtaskID]core.PlannedSubtaskID{core.SubtaskID(vnID(20)): core.PlannedSubtaskID(vnID(120))}
			if future {
				in.Today = core.LocalDate{Year: 2026, Month: 10, Day: 31}
				in.EventID = ""
			} else {
				in.PlannedIDs[core.SubtaskID(vnID(21))] = core.PlannedSubtaskID(vnID(121))
			}
			a := riReplayR(t, b, in)
			r, e := auditReturnHistoryVNext(p, b, a, in)
			must(t, e)
			if !r.valid() || !r.proof.changed {
				t.Fatal("missing changed proof")
			}
			g := riGraph(t, r.proof.after)
			if len(g.Definitions) != 2 || g.Definitions[1].Sequence != 2 || g.Definitions[1].Title != "synthetic" || g.Traces[0].DefinitionID != core.TaskDefinitionID(vnID(2)) || g.Definitions[1].PlannedSubtasks[0].LineageID != core.SubtaskLineageID(vnID(1020)) {
				t.Fatal("return field delta")
			}
			if future {
				if len(g.Definitions[1].PlannedSubtasks) != 1 || g.Subtasks[1].Status != core.SubtaskPending || g.Subtasks[1].DraftCancellationID != nil || r.proof.after.proof.count != 0 {
					t.Fatal("hidden X restored before planning or captured")
				}
				bad := in
				bad.CancellationID = core.CancellationID(vnID(90))
				z, e := auditReturnHistoryVNext(p, b, a, bad)
				riZero(t, z, e)
				if !errors.Is(e, core.ErrInvalidInput) {
					t.Fatal(e)
				}
			} else {
				ev := riEvents(r.proof.after)
				if len(ev) != 2 || !historyEqual(ev[0], riEvents(p)[0]) || ev[1].Object["status"].String != "returnedToPool" || ev[1].Object["id"].String != vnID(900) {
					t.Fatal("capture prefix/constants")
				}
			}
			riDirectMutations(t, r.proof.after, func(v validatedHistorySnapshot) bool { return historyReturnFence(p, v, in) })
			original := historyVNextClone(r.proof.after.proof.snapshot)
			in.PlannedIDs[core.SubtaskID(vnID(20))] = "corrupt"
			if r.proof.returnDetail.PlannedIDs[core.SubtaskID(vnID(20))] != core.PlannedSubtaskID(vnID(120)) {
				t.Fatal("map alias")
			}
			a.Definitions[0].Title = "caller"
			copy, e := r.snapshotCopy()
			must(t, e)
			copy.Object["definitions"].Array[0].Object["title"] = historyString("copy")
			copy2, e := r.snapshotCopy()
			must(t, e)
			if !historyEqual(original, copy2) {
				t.Fatal("endpoint alias")
			}
			// Direct fence: valid but unrelated definition-title mutation must fail,
			// without routing through replay match.
			badG := g.Clone()
			badG.Definitions[0].Title = "polluted"
			badP := riSeal(t, badG, r.proof.after.proof.snapshot.Object["classifications"])
			if historyReturnFence(p, badP, *r.proof.returnDetail) {
				t.Fatal("direct return fence accepted pollution")
			}
		})
	}
}
func TestRemainingIndependentCarryPaths(t *testing.T) {
	for _, cont := range []bool{false, true} {
		for _, locked := range []bool{false, true} {
			if !cont && locked {
				continue
			}
			t.Run(map[bool]string{false: "D", true: "C"}[cont]+map[bool]string{false: "open", true: "locked"}[locked], func(t *testing.T) {
				f := vnSeed(t, vnDate)
				riChild(t, f, 20)
				if cont {
					if locked {
						f.run(t, vnStep{"L", 2})
					} else {
						f.run(t, vnStep{"A", 1}, vnStep{"U", 2})
					}
				} else {
					f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
				}
				b := f.k.Snapshot()
				if cont && !locked {
					b.Subtasks[0].Status = core.SubtaskUnfinished
					b.Subtasks[0].UpdatedAt = vnAt(2)
					b.Subtasks[0].SettledAt = vnPtr(vnAt(2))
				} // Synthetic open-U, not Service reachability.
				b.Traces[0].ManualProgressPercent = vnPtr(47)
				b.Traces[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(vnID(60)), Body: "active", CreatedAt: vnAt(0), UpdatedAt: vnAt(0)}, {ID: core.TaskNoteEntryID(vnID(61)), Body: "", CreatedAt: vnAt(0), UpdatedAt: vnAt(1), DeletedAt: vnPtr(vnAt(1))}} // A current definition differs from source definition.
				b.Definitions[0].ContentUpdatedAt = vnAt(2)
				b.Definitions[0].SupersededAt = vnPtr(vnAt(2))
				id := core.TaskDefinitionID(vnID(70))
				b.Definitions[0].SupersededByDefinitionID = &id
				b.Definitions = append(b.Definitions, core.TaskDefinition{ID: id, ChainID: b.Chains[0].ID, Sequence: 2, Title: "new current", CreatedAt: vnAt(2), ContentUpdatedAt: vnAt(2)})
				p := riSeal(t, b, f.c)
				i := riCarry(f, vnAt(5))
				i.SubtaskIDs = map[core.SubtaskID]core.SubtaskID{core.SubtaskID(vnID(20)): core.SubtaskID(vnID(120))}
				if cont {
					i.Today = i.TargetDate
				}
				k, e := core.NewTaskKernel(b)
				must(t, e)
				if cont {
					if e := k.ContinueUnfinishedTrace(i); e != nil {
						var ve *core.ValidationError
						if errors.As(e, &ve) {
							t.Fatalf("synthetic continuation: %+v", ve.Issues)
						}
						t.Fatal(e)
					}
				} else {
					must(t, k.DeferCurrentTrace(i))
				}
				a := k.Snapshot()
				var r auditedHistoryCommand
				if cont {
					r, e = auditContinueHistoryVNext(p, b, a, riC(i))
				} else {
					r, e = auditDeferHistoryVNext(p, b, a, riD(i))
				}
				must(t, e)
				g := riGraph(t, r.proof.after)
				n := g.Traces[1]
				if len(n.NoteEntries) != 1 || n.NoteEntries[0].Body != "active" {
					t.Fatal("active notes filter")
				}
				if n.DefinitionID != id || n.Priority != 1 || n.ManualProgressPercent == nil || *n.ManualProgressPercent != 0 || n.CarriedFromTraceID == nil || *n.CarriedFromTraceID != b.Traces[0].ID || g.Subtasks[1].LineageID != core.SubtaskLineageID(vnID(1020)) || *g.Subtasks[1].CarriedFromSubtaskID != b.Subtasks[0].ID {
					t.Fatal("carry fields")
				}
				if cont {
					if !historyEqual(p.proof.snapshot.Object["traces"].Array[0], r.proof.after.proof.snapshot.Object["traces"].Array[0]) || !historyEqual(p.proof.snapshot.Object["subtasks"].Array[0], r.proof.after.proof.snapshot.Object["subtasks"].Array[0]) || n.ContinuationSeq != b.Traces[0].ContinuationSeq+1 || !historyEqual(p.proof.snapshot.Object["classifications"], r.proof.after.proof.snapshot.Object["classifications"]) {
						t.Fatal("continue modified old source/history")
					}
				} else {
					if g.Traces[0].Status != core.TraceDeferred || g.Subtasks[0].Status != core.SubtaskDeferred || len(riEvents(r.proof.after)) != 2 || !historyEqual(riEvents(p)[0], riEvents(r.proof.after)[0]) {
						t.Fatal("defer status/prefix")
					}
				}
				bad := g.Clone()
				bad.Definitions[0].Title = "pollution"
				bp := riSeal(t, bad, r.proof.after.proof.snapshot.Object["classifications"])
				if cont {
					if historyContinueFence(p, bp, riC(i)) {
						t.Fatal("C direct fence")
					}
				} else {
					if historyDeferFence(p, bp, riD(i)) {
						t.Fatal("D direct fence")
					}
				}
				if cont {
					riDirectMutations(t, r.proof.after, func(v validatedHistorySnapshot) bool { return historyContinueFence(p, v, riC(i)) })
				} else {
					riDirectMutations(t, r.proof.after, func(v validatedHistorySnapshot) bool { return historyDeferFence(p, v, riD(i)) })
				}
				i.SubtaskIDs[core.SubtaskID(vnID(20))] = "caller"
				if cont {
					if r.proof.continueDetail.SubtaskIDs[core.SubtaskID(vnID(20))] != core.SubtaskID(vnID(120)) {
						t.Fatal("C alias")
					}
				} else {
					if r.proof.deferDetail.SubtaskIDs[core.SubtaskID(vnID(20))] != core.SubtaskID(vnID(120)) {
						t.Fatal("D alias")
					}
				}
			})
		}
	}
}

func TestRemainingIndependentOrderBudgetAndZero(t *testing.T) {
	f := vnSeed(t, vnDate)
	b := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := riReturn(f, vnAt(5))
	a := riReplayR(t, b, in)
	in.EventID = ""
	r, e := auditReturnHistoryVNext(p, b, a, in)
	riZero(t, r, e)
	var ce *historyCommandError
	if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
		t.Fatalf("count %v", e)
	}
	in.TraceID = "missing"
	r, e = auditReturnHistoryVNext(p, b, a, in)
	riZero(t, r, e)
	if !errors.Is(e, core.ErrNotFound) {
		t.Fatal(e)
	}
	in.Now = core.Instant(math.Float64bits(math.NaN()))
	r, e = auditReturnHistoryVNext(p, b, a, in)
	riZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidClock) {
		t.Fatal(e)
	}
	i := riCarry(f, in.Now)
	i.TraceID = "missing"
	d := riD(i)
	d.EventID = ""
	r, e = auditDeferHistoryVNext(p, b, b, d)
	riZero(t, r, e)
	if !errors.Is(e, core.ErrNotFound) {
		t.Fatal(e)
	}
	r, e = auditContinueHistoryVNext(p, b, b, riC(i))
	riZero(t, r, e)
	if !errors.Is(e, core.ErrNotFound) {
		t.Fatal(e)
	}
	// Invalid binding and invalid graph content are behind raw input budgets.
	bad := b.Clone()
	bad.Traces[0].ID = "bad binding"
	huge := strings.Repeat("s", canonical.DefaultLimits().MaxBytes+1)
	in = riReturn(f, vnAt(5))
	in.CancellationID = core.CancellationID(huge)
	r, e = auditReturnHistoryVNext(p, bad, b, in)
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	d = riD(riCarry(f, vnAt(5)))
	d.DayID = core.DayID(huge)
	r, e = auditDeferHistoryVNext(p, bad, b, d)
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	c := riC(riCarry(f, vnAt(5)))
	c.DayID = core.DayID(huge)
	r, e = auditContinueHistoryVNext(p, bad, b, c)
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	in = riReturn(f, vnAt(5))
	in.PlannedIDs = map[core.SubtaskID]core.PlannedSubtaskID{"bad": "bad"}
	r, e = auditReturnHistoryVNext(p, bad, b, in)
	riZero(t, r, e)
	riBudget(t, e, historyCommandIDCount)
	d.SubtaskIDs = map[core.SubtaskID]core.SubtaskID{"bad": "bad"}
	r, e = auditDeferHistoryVNext(p, bad, b, d)
	riZero(t, r, e)
	riBudget(t, e, historyCommandIDCount)
	c.SubtaskIDs = d.SubtaskIDs
	r, e = auditContinueHistoryVNext(p, bad, b, c)
	riZero(t, r, e)
	riBudget(t, e, historyCommandIDCount)
}
func riBudget(t *testing.T, e error, reason historyCommandBudgetReason) {
	t.Helper()
	var be *historyCommandBudgetError
	if !errors.As(e, &be) || be.Phase != historyCommandInputFields || be.Reason != reason {
		t.Fatalf("budget got %v", e)
	}
}
func TestRemainingIndependentFutureHistoryRejected(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
	b := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := riReturn(f, vnAt(5))
	in.Today = core.LocalDate{Year: 2026, Month: 10, Day: 31}
	in.EventID = ""
	a := riReplayR(t, b, in)
	r, e := auditReturnHistoryVNext(p, b, a, in)
	riZero(t, r, e)
	var he *historyVNextError
	if !errors.As(e, &he) {
		t.Fatalf("expected final inspector: %T %v", e, e)
	}
	if len(riEvents(p)) != 1 {
		t.Fatal("prior history erased")
	}
	in.EventID = "not empty"
	r, e = auditReturnHistoryVNext(p, b, a, in)
	riZero(t, r, e)
	var ce *historyCommandError
	if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
		t.Fatal(e)
	}
}

func TestRemainingIndependentClockBitsAndUnusedIDs(t *testing.T) {
	for _, neg := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive_to_negative", true: "negative_to_positive"}[neg], func(t *testing.T) {
			f := vnSeed(t, vnDate)
			b := f.k.Snapshot()
			old := core.Instant(0)
			now := core.Instant(uint64(1) << 63)
			if neg {
				old, now = now, old
			}
			b.Chains[0].UpdatedAt = old
			p := riSeal(t, b, f.c)
			i := riCarry(f, now)
			// Existing target day means an invalid unused DayID must be tolerated.
			b.Days = append(b.Days, core.Day{ID: core.DayID(vnID(95)), Date: i.TargetDate, CreatedAt: old, UpdatedAt: old})
			p = riSeal(t, b, f.c)
			i.DayID = "unused invalid"
			k, e := core.NewTaskKernel(b)
			must(t, e)
			must(t, k.DeferCurrentTrace(i))
			r, e := auditDeferHistoryVNext(p, b, k.Snapshot(), riD(i))
			must(t, e)
			g := riGraph(t, r.proof.after)
			if g.Chains[0].UpdatedAt != old || g.Traces[0].ContentUpdatedAt != now || *g.Traces[0].SettledAt != now || g.Traces[1].CreatedAt != now || !reflect.DeepEqual(g.Days, b.Days) {
				t.Fatal("zero bits/frozen existing day")
			}
			if !historyEqual(riEvents(r.proof.after)[0].Object["capturedAt"], historyNumber(uint64(now))) {
				t.Fatal("capture clock bits")
			}
			bad := g.Clone()
			bad.Chains[0].UpdatedAt = now
			bp := riSeal(t, bad, r.proof.after.proof.snapshot.Object["classifications"])
			if historyDeferFence(p, bp, riD(i)) {
				t.Fatal("direct fence accepted numerical-equal bit pollution")
			}
			// Return ignores cancellation UUID for current-day R, but owns original bits.
			b = f.k.Snapshot()
			p = vnGood(t, f.snapshot(t))
			in := riReturn(f, now)
			in.CancellationID = "unused invalid"
			a := riReplayR(t, b, in)
			r, e = auditReturnHistoryVNext(p, b, a, in)
			must(t, e)
			if riGraph(t, r.proof.after).Chains[0].UpdatedAt != now || r.proof.returnDetail.Now != now {
				t.Fatal("R now bits")
			}
		})
	}
}
func TestRemainingIndependentProofKindClosure(t *testing.T) {
	f := vnSeed(t, vnDate)
	b := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	in := riReturn(f, vnAt(5))
	r, e := auditReturnHistoryVNext(p, b, riReplayR(t, b, in), in)
	must(t, e)
	for _, kind := range []historyCommandKind{0, 255, historyCommandDefer, historyCommandContinue} {
		q := *r.proof
		q.kind = kind
		if (auditedHistoryCommand{&q}).valid() {
			t.Fatalf("kind %d accepted mismatched detail", kind)
		}
	}
	q := *r.proof
	q.returnDetail = nil
	if (auditedHistoryCommand{&q}).valid() {
		t.Fatal("missing detail accepted")
	}
	q = *r.proof
	q.deferDetail = &deferHistoryInput{}
	if (auditedHistoryCommand{&q}).valid() {
		t.Fatal("extra detail accepted")
	}
	// Old proof kinds must not accept a newly registered return detail either.
	q = *r.proof
	q.kind = historyCommandComplete
	if (auditedHistoryCommand{&q}).valid() {
		t.Fatal("mismatched complete kind with return detail accepted")
	}
}

// Corrupt the sealed output locally to isolate the independent fence from
// historyCommandMatch and inspector. These are test-only internal copies.
func riDirectMutations(t *testing.T, p validatedHistorySnapshot, fence func(validatedHistorySnapshot) bool) {
	t.Helper()
	n := 0
	var walk func(canonical.Value, func(canonical.Value) canonical.Value)
	check := func(v canonical.Value) {
		q := *p.proof
		q.snapshot = v
		if fence(validatedHistorySnapshot{&q}) {
			t.Fatal("direct fence accepted canonical mutation")
		}
		n++
	}
	walk = func(v canonical.Value, put func(canonical.Value) canonical.Value) {
		x := historyVNextClone(v)
		switch v.Kind {
		case canonical.String:
			x.String += "mutation"
			check(put(x))
		case canonical.Number:
			if x.TextNumber == "0" {
				x.TextNumber = "1"
			} else {
				x.TextNumber = "0"
			}
			check(put(x))
		case canonical.Boolean:
			x.Bool = !x.Bool
			check(put(x))
		case canonical.Null:
			check(put(historyString("present")))
		case canonical.Object:
			for key, child := range v.Object {
				k := key
				without := historyVNextClone(v)
				delete(without.Object, k)
				check(put(without))
				walk(child, func(replacement canonical.Value) canonical.Value {
					out := historyVNextClone(v)
					out.Object[k] = replacement
					return put(out)
				})
			}
		case canonical.Array:
			extra := historyVNextClone(v)
			extra.Array = append(extra.Array, canonical.Value{Kind: canonical.Null})
			check(put(extra))
			for index, child := range v.Array {
				i := index
				walk(child, func(replacement canonical.Value) canonical.Value {
					out := historyVNextClone(v)
					out.Array[i] = replacement
					return put(out)
				})
			}
		}
	}
	walk(p.proof.snapshot, func(v canonical.Value) canonical.Value { return v })
	t.Logf("direct canonical fence mutations rejected: %d", n)
}

func TestRemainingIndependentMapRawAndCaptureOrdering(t *testing.T) {
	f := vnSeed(t, vnDate)
	riChild(t, f, 20)
	f.run(t, vnStep{"A", 1}, vnStep{"P", 2})
	b := f.k.Snapshot()
	p := vnGood(t, f.snapshot(t))
	bad := b.Clone()
	bad.Traces[0].ID = "invalid before"
	huge := strings.Repeat("z", canonical.DefaultLimits().MaxBytes+1)
	in := riReturn(f, vnAt(5))
	in.PlannedIDs = map[core.SubtaskID]core.PlannedSubtaskID{core.SubtaskID(vnID(20)): core.PlannedSubtaskID(huge)}
	r, e := auditReturnHistoryVNext(p, bad, b, in)
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	i := riCarry(f, vnAt(5))
	i.SubtaskIDs = map[core.SubtaskID]core.SubtaskID{core.SubtaskID(vnID(20)): core.SubtaskID(huge)}
	r, e = auditDeferHistoryVNext(p, bad, b, riD(i))
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	r, e = auditContinueHistoryVNext(p, bad, b, riC(i))
	riZero(t, r, e)
	riBudget(t, e, historyCommandRawBytes)
	in.PlannedIDs[core.SubtaskID(vnID(20))] = core.PlannedSubtaskID(vnID(120))
	a := riReplayR(t, b, in)
	for _, event := range []string{"", "invalid", strings.ToLower(vnID(101))} {
		in.EventID = event
		r, e = auditReturnHistoryVNext(p, b, a, in)
		riZero(t, r, e)
		if event == "" {
			var ce *historyCommandError
			if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
				t.Fatal(e)
			}
		} else {
			var ce *historyCommandError
			if errors.As(e, &ce) && ce.Reason == historyCommandCount {
				t.Fatal("nonempty format/freshness preempted by count")
			}
		}
	}
	in.EventID = ""
	wrong := a.Clone()
	wrong.Definitions[0].Title = "after mismatch"
	r, e = auditReturnHistoryVNext(p, b, wrong, in)
	riZero(t, r, e)
	var ce *historyCommandError
	if !errors.As(e, &ce) || ce.Reason != historyCommandReplay {
		t.Fatal("match must precede event count", e)
	}
	i.SubtaskIDs[core.SubtaskID(vnID(20))] = core.SubtaskID(vnID(120))
	k, e := core.NewTaskKernel(b)
	must(t, e)
	must(t, k.DeferCurrentTrace(i))
	d := riD(i)
	d.EventID = ""
	r, e = auditDeferHistoryVNext(p, b, k.Snapshot(), d)
	riZero(t, r, e)
	if !errors.As(e, &ce) || ce.Reason != historyCommandCount {
		t.Fatal(e)
	}
}
