package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Independent tests use only baseline helpers. All lifecycle events below come
// from actual core commands followed by their existing typed auditors.
func independentEditSeed(t *testing.T, events int) (*core.TaskKernel, validatedHistorySnapshot) {
	f := vnSeed(t, vnDate)
	p := vnGood(t, f.snapshot(t))
	k := f.k
	for i := 0; i < events; i++ {
		b := k.Snapshot()
		at := vnAt(float64(2*i + 1))
		in := abandonHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Now: at, EventID: vnID(100 + i)}
		must(t, k.AbandonChain(core.AbandonChainInput{ChainID: in.ChainID, TraceID: in.TraceID, Now: at}))
		r, e := auditAbandonHistoryVNext(p, b, k.Snapshot(), in)
		must(t, e)
		p = r.proof.after
		b = k.Snapshot()
		ri := reactivateHistoryInput{ChainID: in.ChainID, TraceID: in.TraceID, Today: vnDate, Now: vnAt(float64(2*i + 2))}
		_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: ri.ChainID, TraceID: ri.TraceID, Today: ri.Today, Now: ri.Now})
		must(t, e)
		r, e = auditReactivateHistoryVNext(p, b, k.Snapshot(), ri)
		must(t, e)
		p = r.proof.after
	}
	return k, p
}
func independentEditTitle(t *testing.T, k *core.TaskKernel, p validatedHistorySnapshot, in titleHistoryInput) auditedHistoryCommand {
	t.Helper()
	b := k.Snapshot()
	ci := core.EditTaskTitleInput{ChainID: in.ChainID, Title: in.Title, Today: in.Today, Now: in.Now, NewDefinitionID: in.NewDefinitionID}
	var e error
	if in.Action == titleHistoryRename {
		e = k.RenameTaskTitle(ci)
	} else {
		e = k.SaveTaskTitleInput(ci)
	}
	must(t, e)
	r, e := auditTitleHistoryVNext(p, b, k.Snapshot(), in)
	must(t, e)
	if !r.valid() {
		t.Fatal("invalid proof")
	}
	return r
}
func TestIndependentEditTitleReuseForkAndEvents(t *testing.T) {
	for _, events := range []int{0, 1, 2} {
		for _, action := range []titleHistoryAction{titleHistoryRename, titleHistorySaveInput} {
			k, p := independentEditSeed(t, events)
			b := k.Snapshot()
			in := titleHistoryInput{Action: action, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: " revised\r\ntext ", Today: vnDate, Now: vnAt(10), NewDefinitionID: core.TaskDefinitionID(vnID(500))}
			r := independentEditTitle(t, k, p, in)
			a := k.Snapshot()
			want := 1
			if action == titleHistoryRename {
				want = 2
			}
			if len(a.Definitions) != want {
				t.Fatalf("action %d: versions %d", action, len(a.Definitions))
			}
			if a.Definitions[len(a.Definitions)-1].Title != "revised\ntext" {
				t.Fatal("normalization")
			}
			if !historyEqual(p.proof.snapshot.Object["classifications"], r.proof.after.proof.snapshot.Object["classifications"]) {
				t.Fatal("event mutation")
			}
			if action == titleHistorySaveInput && a.Traces[0].ContentUpdatedAt != b.Traces[0].ContentUpdatedAt {
				t.Fatal("reuse touched trace")
			}
			in.Title = "next character"
			in.Now = vnAt(11)
			in.Action = titleHistorySaveInput
			in.NewDefinitionID = core.TaskDefinitionID(string([]byte{0xff, 0, 0xfe}))
			independentEditTitle(t, k, r.proof.after, in)
			if len(k.Snapshot().Definitions) != want {
				t.Fatal("input unexpectedly forked")
			}
		}
	}
}
func TestIndependentEditNoopClocksAndUnusedIdentity(t *testing.T) {
	for _, bits := range []uint64{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1)), math.Float64bits(math.Inf(-1)), math.Float64bits(-10), math.Float64bits(math.MaxFloat64)} {
		k, p := independentEditSeed(t, 2)
		b := k.Snapshot()
		in := titleHistoryInput{Action: titleHistoryRename, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: "  synthetic\r\n", Today: vnDate, Now: core.Instant(bits), NewDefinitionID: core.TaskDefinitionID(string([]byte{0xff, 0}))}
		r, e := auditTitleHistoryVNext(p, b, b, in)
		must(t, e)
		changed, e := r.changed()
		must(t, e)
		if changed {
			t.Fatal("no-op changed")
		}
		mi := manualProgressHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Today: vnDate, Now: vnAt(10), Percent: 0}
		must(t, k.SetManualProgress(core.SetManualProgressInput(mi)))
		r, e = auditManualProgressHistoryVNext(p, b, k.Snapshot(), mi)
		must(t, e)
		p = r.proof.after
		b = k.Snapshot()
		mi.Now = core.Instant(bits)
		r, e = auditManualProgressHistoryVNext(p, b, b, mi)
		must(t, e)
		changed, e = r.changed()
		must(t, e)
		if changed {
			t.Fatal("manual no-op changed")
		}
	}
}
func TestIndependentEditManualErrorsClampAndFences(t *testing.T) {
	for _, percent := range []int{-int(^uint(0)>>1) - 1, -1, 0, 50, 100, int(^uint(0) >> 1)} {
		k, p := independentEditSeed(t, 2)
		b := k.Snapshot()
		in := manualProgressHistoryInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Percent: percent, Today: vnDate, Now: vnAt(10)}
		must(t, k.SetManualProgress(core.SetManualProgressInput(in)))
		a := k.Snapshot()
		r, e := auditManualProgressHistoryVNext(p, b, a, in)
		must(t, e)
		want := min(100, max(0, percent))
		if *a.Traces[0].ManualProgressPercent != want || a.Traces[0].Status != core.TracePending || !reflect.DeepEqual(a.Chains, b.Chains) {
			t.Fatal("manual semantics")
		}
		bad := a.Clone()
		bad.Definitions[0].Title = "unrelated"
		if historyManualProgressFence(b, bad, in) {
			t.Fatal("independent fence allowed definition change")
		}
		r, e = auditManualProgressHistoryVNext(p, b, bad, in)
		lifecycleZero(t, r, e)
		bad = a.Clone()
		bad.Traces[0].Status = core.TraceCompleted
		if historyManualProgressFence(b, bad, in) {
			t.Fatal("independent fence allowed status")
		}
		in.ChainID = core.TaskChainID(vnID(900))
		r, e = auditManualProgressHistoryVNext(p, b, b, in)
		lifecycleZero(t, r, e)
		if !errors.Is(e, core.ErrInvalidInput) {
			t.Fatalf("owner: %v", e)
		}
		in.TraceID = core.DayTraceID(strings.Repeat("x", 36))
		r, e = auditManualProgressHistoryVNext(p, b, b, in)
		lifecycleZero(t, r, e)
		if !errors.Is(e, core.ErrNotFound) {
			t.Fatalf("nonUUID trace: %v", e)
		}
	}
}
func TestIndependentEditTitleFenceAndAlias(t *testing.T) {
	k, p := independentEditSeed(t, 1)
	b := k.Snapshot()
	in := titleHistoryInput{Action: titleHistoryRename, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: "renamed", Today: vnDate, Now: vnAt(10), NewDefinitionID: core.TaskDefinitionID(vnID(501))}
	r := independentEditTitle(t, k, p, in)
	a := k.Snapshot()
	owned, _, e := historyCommandBind(p, b)
	must(t, e)
	for _, mutate := range []func(*core.TaskGraph){func(g *core.TaskGraph) { g.Definitions[0].Title = "rewrite history" }, func(g *core.TaskGraph) { g.Definitions[1].DescriptionText = vnPtr("smuggled") }, func(g *core.TaskGraph) { g.Traces[0].ManualProgressPercent = vnPtr(25) }, func(g *core.TaskGraph) { g.Definitions[0], g.Definitions[1] = g.Definitions[1], g.Definitions[0] }} {
		bad := a.Clone()
		mutate(&bad)
		if historyTitleFence(owned, bad, in) {
			t.Fatal("title fence allowed unrelated mutation")
		}
	}
	snap, e := r.snapshotCopy()
	must(t, e)
	want := historyVNextClone(snap)
	b.Definitions[0].Title = "caller mutation"
	a.Definitions[0].Title = "caller after mutation"
	snap.Object["definitions"].Array[0].Object["title"] = historyString("return mutation")
	again, e := r.snapshotCopy()
	must(t, e)
	if !historyEqual(want, again) {
		t.Fatal("alias escaped")
	}
}

func TestIndependentEditInputBudgetAndOrdering(t *testing.T) {
	k, p := independentEditSeed(t, 0)
	b := k.Snapshot()
	base := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: "synthetic", Today: vnDate, Now: vnAt(0)}
	// Metadata exact raw boundary, including an unused arbitrary-byte identity.
	const limit = 16 * 1024 * 1024
	in := base
	in.NewDefinitionID = core.TaskDefinitionID(strings.Repeat("x", limit-len(in.ChainID)-len(in.Title)))
	must(t, historyCommandTitleInputBudget(in))
	in.NewDefinitionID += "x"
	r, e := auditTitleHistoryVNext(p, b, b, in)
	lifecycleZero(t, r, e)
	var be *historyCommandBudgetError
	if !errors.As(e, &be) || be.Phase != historyCommandInputFields || be.Reason != historyCommandRawBytes {
		t.Fatalf("input budget %v", e)
	}
	huge := b.Clone()
	huge.Definitions[0].Title = strings.Repeat("q", limit+1)
	for _, before := range []bool{true, false} {
		a, c := b, huge
		want := historyCommandAfter
		if before {
			a, c = huge, b
			want = historyCommandBefore
		}
		r, e = auditTitleHistoryVNext(p, a, c, in)
		lifecycleZero(t, r, e)
		if !errors.As(e, &be) || be.Phase != want {
			t.Fatalf("budget precedence %v", e)
		}
	}
	r, e = auditTitleHistoryVNext(validatedHistorySnapshot{}, huge, huge, in)
	lifecycleZero(t, r, e)
	if errors.As(e, &be) {
		t.Fatal("seal did not precede budgets")
	}
}

// Signed-zero and negative epochs are valid pure profile parameters, not claims
// that the current Service clock allocator emits these values.
func TestIndependentEditSignedZeroAndNegativeClocks(t *testing.T) {
	for _, pair := range [][2]float64{{math.Copysign(0, -1), 0}, {0, math.Copysign(0, -1)}, {-20, -10}, {-20, -20}} {
		k, e := core.NewTaskKernel(core.TaskGraph{})
		must(t, e)
		old := core.Instant(math.Float64bits(pair[0]))
		now := core.Instant(math.Float64bits(pair[1]))
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(vnID(1)), DefinitionID: core.TaskDefinitionID(vnID(2)), Title: "é", Now: old}))
		f := &vnFixture{k: k, date: vnDate}
		extra, e := emptySupplement()
		must(t, e)
		f.c = extra.Classifications
		p := vnGood(t, f.snapshot(t))
		b := k.Snapshot()
		in := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: "e\u0301", Today: vnDate, Now: core.Instant(math.Float64bits(math.NaN()))}
		r, e := auditTitleHistoryVNext(p, b, b, in)
		must(t, e)
		if r.proof.after.proof.snapshot.Object["definitions"].Array[0].Object["title"].String != "é" {
			t.Fatal("canonical no-op changed stored bytes")
		}
		in.Title = "new"
		in.Now = now
		r = independentEditTitle(t, k, p, in)
		want := old
		if now.After(old) {
			want = now
		}
		if k.Snapshot().Chains[0].UpdatedAt != want {
			t.Fatal("chain time tie bit drift")
		}
	}
}

func TestIndependentEditHistoricalFloorAndTitleFork(t *testing.T) {
	f := vnSeed(t, vnDate)
	b := f.k.Snapshot()
	must(t, f.k.SetManualProgress(core.SetManualProgressInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, Percent: 60, Today: vnDate, Now: vnAt(1)}))
	// Settlement is captured using the existing typed adapter, not a forged seal.
	p := vnGood(t, f.snapshot(t))
	b = f.k.Snapshot()
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	must(t, f.k.SettleTaskDays(tomorrow, vnAt(2)))
	sr, e := auditSettlementHistoryVNext(p, b, f.k.Snapshot(), settlementHistoryInput{Today: tomorrow, Now: vnAt(2), EventIDs: []string{vnID(701)}})
	must(t, e)
	must(t, f.k.ContinueUnfinishedTrace(core.TraceCarryoverInput{ChainID: b.Chains[0].ID, TraceID: b.Traces[0].ID, NewTraceID: core.DayTraceID(vnID(702)), DayID: core.DayID(vnID(703)), TargetDate: tomorrow, Today: tomorrow, Now: vnAt(3)}))
	// Existing continuation core-only edge: re-inspect its unchanged supplement.
	f.c = historyVNextClone(sr.proof.after.proof.snapshot.Object["classifications"])
	p = vnGood(t, f.snapshot(t))
	b = f.k.Snapshot()
	mi := manualProgressHistoryInput{ChainID: b.Chains[0].ID, TraceID: core.DayTraceID(vnID(702)), Percent: 20, Today: tomorrow, Now: vnAt(4)}
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(mi)))
	r, e := auditManualProgressHistoryVNext(p, b, f.k.Snapshot(), mi)
	must(t, e)
	if *f.k.Snapshot().Traces[1].ManualProgressPercent != 60 {
		t.Fatal("historical floor lost")
	}
	p = r.proof.after
	b = f.k.Snapshot()
	ti := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryTrace, ChainID: b.Chains[0].ID, TraceID: b.Traces[1].ID, Title: "fork history", Today: tomorrow, Now: vnAt(5), NewDefinitionID: core.TaskDefinitionID(vnID(704))}
	r = independentEditTitle(t, f.k, p, ti)
	a := f.k.Snapshot()
	if len(a.Definitions) != 2 || a.Definitions[0].Title != "synthetic" || !reflect.DeepEqual(a.Traces[0], b.Traces[0]) {
		t.Fatal("historical definition/trace rewritten")
	}
	ti.TraceID = b.Traces[0].ID
	ti.Title = "fork history"
	x, e := auditTitleHistoryVNext(r.proof.after, a, a, ti)
	lifecycleZero(t, x, e)
	if !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatalf("explicit past owner borrowed pending peer: %v", e)
	}
}

func TestIndependentEditSameDayPeerExcluded(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	must(t, f.k.SetManualProgress(core.SetManualProgressInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, Percent: 65, Today: vnDate, Now: vnAt(1)}))
	f.run(t, vnStep{"R", 2})
	must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: g.Chains[0].ID, TraceID: core.DayTraceID(vnID(810)), DayID: core.DayID(vnID(811)), Date: vnDate, Today: vnDate, Now: vnAt(3)}))
	p := vnGood(t, f.snapshot(t))
	b := f.k.Snapshot()
	if b.Traces[1].ContinuationSeq != b.Traces[0].ContinuationSeq {
		t.Fatal("fixture missing equal-sequence peer")
	}
	in := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: b.Traces[1].ID, Today: vnDate, Now: vnAt(4), Percent: 20}
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
	r, e := auditManualProgressHistoryVNext(p, b, f.k.Snapshot(), in)
	must(t, e)
	if !r.valid() || *f.k.Snapshot().Traces[1].ManualProgressPercent != 20 {
		t.Fatal("same-day peer incorrectly sets floor")
	}
}

// Pure core fixture uses a future returned trace then an earlier request date.
// This is profile-valid temporal input, not Service clock/date reachability.
func TestIndependentEditFutureActualChildBlocks(t *testing.T) {
	tomorrow, e := vnDate.AddDays(1)
	must(t, e)
	f := vnSeed(t, tomorrow)
	g := f.k.Snapshot()
	must(t, f.k.EditSubtask(core.SubtaskEditInput{Kind: core.SubtaskAdd, ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, SubtaskID: core.SubtaskID(vnID(822)), LineageID: core.SubtaskLineageID(vnID(823)), Title: "future child", Difficulty: core.DifficultySimple, Today: tomorrow, Now: vnAt(1)}))
	must(t, f.k.ReturnToPool(core.ReturnToPoolInput{ChainID: g.Chains[0].ID, TraceID: g.Traces[0].ID, NewDefinitionID: core.TaskDefinitionID(vnID(824)), Today: tomorrow, Now: vnAt(2), PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{core.SubtaskID(vnID(822)): core.PlannedSubtaskID(vnID(825))}}))
	f.capture(core.TraceReturnedToPool, vnAt(2))
	must(t, f.k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.PlannedSubtaskRemove, ChainID: g.Chains[0].ID, ExpectedDefinitionID: core.TaskDefinitionID(vnID(824)), SubtaskID: core.PlannedSubtaskID(vnID(825)), Now: vnAt(3)}))
	must(t, f.k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: g.Chains[0].ID, TraceID: core.DayTraceID(vnID(826)), DayID: core.DayID(vnID(827)), Date: vnDate, Today: vnDate, Now: vnAt(4)}))
	p := vnGood(t, f.snapshot(t))
	b := f.k.Snapshot()
	in := manualProgressHistoryInput{ChainID: g.Chains[0].ID, TraceID: core.DayTraceID(vnID(826)), Today: vnDate, Now: vnAt(5), Percent: 0}
	r, e := auditManualProgressHistoryVNext(p, b, b, in)
	lifecycleZero(t, r, e)
	if !errors.Is(e, core.ErrInvalidTransition) {
		t.Fatalf("future child blocker: %v", e)
	}
}
func TestIndependentEditPlannedCopy(t *testing.T) {
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	chain := core.TaskChainID(vnID(1))
	def := core.TaskDefinitionID(vnID(2))
	must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: def, Title: "parent", Now: vnAt(0)}))
	must(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: chain, ExpectedDefinitionID: def, SubtaskID: core.PlannedSubtaskID(vnID(830)), LineageID: core.SubtaskLineageID(vnID(831)), Title: "planned", Difficulty: core.DifficultyMedium, Now: vnAt(1)}))
	// Pool-only planned children do not materialize a day-history child blocker.
	extra, e := emptySupplement()
	must(t, e)
	f := &vnFixture{k: k, c: extra.Classifications, date: vnDate}
	p := vnGood(t, f.snapshot(t))
	b := k.Snapshot()
	in := titleHistoryInput{Action: titleHistoryRename, Route: titleHistoryChain, ChainID: chain, Title: "renamed pool", Today: vnDate, Now: vnAt(2)}
	independentEditTitle(t, k, p, in)
	if !reflect.DeepEqual(b.Definitions[0].PlannedSubtasks, k.Snapshot().Definitions[0].PlannedSubtasks) {
		t.Fatal("planned child changed")
	}
	must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: core.DayTraceID(vnID(3)), DayID: core.DayID(vnID(4)), Date: vnDate, Today: vnDate, Now: vnAt(3), SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{core.PlannedSubtaskID(vnID(830)): core.SubtaskID(vnID(832))}}))
	p = vnGood(t, f.snapshot(t))
	b = k.Snapshot()
	in.Title = "fork planned"
	in.Now = vnAt(4)
	in.NewDefinitionID = core.TaskDefinitionID(vnID(833))
	independentEditTitle(t, k, p, in)
	a := k.Snapshot()
	if !reflect.DeepEqual(a.Definitions[1].PlannedSubtasks, b.Definitions[0].PlannedSubtasks) || !reflect.DeepEqual(a.Subtasks, b.Subtasks) {
		t.Fatal("version planned copy/body")
	}
}

func TestIndependentEditClosedKinds(t *testing.T) {
	k, p := independentEditSeed(t, 0)
	b := k.Snapshot()
	in := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: b.Chains[0].ID, Title: "new", Today: vnDate, Now: vnAt(1)}
	r := independentEditTitle(t, k, p, in)
	for i := 0; i <= 255; i++ {
		proof := *r.proof
		proof.kind = historyCommandKind(i)
		got := auditedHistoryCommand{proof: &proof}
		want := i >= 1 && i <= 8
		if got.valid() != want {
			t.Fatalf("closed kind %d: got %v want %v", i, got.valid(), want)
		}
	}
}
func TestIndependentEditEventRepresentationsAndOtherChain(t *testing.T) {
	k, e := core.NewTaskKernel(core.TaskGraph{})
	must(t, e)
	for i := 0; i < 2; i++ {
		n := i * 10
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(liID(n + 1)), DefinitionID: core.TaskDefinitionID(liID(n + 2)), Title: "task", Now: vnAt(0)}))
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(liID(n + 1)), TraceID: core.DayTraceID(liID(n + 3)), DayID: core.DayID(liID(4)), Date: vnDate, Today: vnDate, Now: vnAt(0)}))
	}
	x, e := emptySupplement()
	must(t, e)
	f := &vnFixture{k: k, c: x.Classifications, date: vnDate}
	p := vnGood(t, f.snapshot(t))
	for i := 0; i < 4; i++ {
		n := (i % 2) * 10
		b := k.Snapshot()
		ai := abandonHistoryInput{ChainID: core.TaskChainID(liID(n + 1)), TraceID: core.DayTraceID(liID(n + 3)), Now: vnAt(float64(2*i + 1)), EventID: liID(900 + i)}
		must(t, k.AbandonChain(core.AbandonChainInput{ChainID: ai.ChainID, TraceID: ai.TraceID, Now: ai.Now}))
		r, e := auditAbandonHistoryVNext(p, b, k.Snapshot(), ai)
		must(t, e)
		p = r.proof.after
		b = k.Snapshot()
		ri := reactivateHistoryInput{ChainID: ai.ChainID, TraceID: ai.TraceID, Today: vnDate, Now: vnAt(float64(2*i + 2))}
		_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: ri.ChainID, TraceID: ri.TraceID, Today: ri.Today, Now: ri.Now})
		must(t, e)
		r, e = auditReactivateHistoryVNext(p, b, k.Snapshot(), ri)
		must(t, e)
		p = r.proof.after
	}
	snap, e := p.snapshotCopy()
	must(t, e)
	c := snap.Object["classifications"]
	for _, field := range []string{"snapshotEventsByTraceID", "snapshotsByTraceID"} {
		a := c.Object[field]
		a.Array[0], a.Array[2] = a.Array[2], a.Array[0]
		a.Array[1], a.Array[3] = a.Array[3], a.Array[1]
		for i := 0; i < len(a.Array); i += 2 {
			key := a.Array[i]
			key.Object["rawValue"] = historyString(strings.ToLower(key.Object["rawValue"].String))
			a.Array[i] = key
			if field == "snapshotEventsByTraceID" {
				for j := range a.Array[i+1].Array {
					ev := a.Array[i+1].Array[j]
					ev.Object["id"] = historyString(strings.ToLower(ev.Object["id"].String))
					ev.Object["traceID"].Object["rawValue"] = historyString(strings.ToLower(ev.Object["traceID"].Object["rawValue"].String))
				}
			} else {
				ev := a.Array[i+1]
				ev.Object["id"] = historyString(strings.ToLower(ev.Object["id"].String))
				ev.Object["traceID"].Object["rawValue"] = historyString(strings.ToLower(ev.Object["traceID"].Object["rawValue"].String))
			}
		}
		c.Object[field] = a
	}
	snap.Object["classifications"] = c
	p = vnGood(t, snap)
	in := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryTrace, ChainID: core.TaskChainID(liID(1)), TraceID: core.DayTraceID(liID(3)), Title: "edited", Today: vnDate, Now: vnAt(20)}
	r := independentEditTitle(t, k, p, in)
	if !historyEqual(c, r.proof.after.proof.snapshot.Object["classifications"]) {
		t.Fatal("event representation/pair order rewritten")
	}
}
