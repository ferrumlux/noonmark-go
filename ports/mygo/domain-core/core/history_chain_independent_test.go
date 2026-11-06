// SPDX-License-Identifier: AGPL-3.0-only
package core_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"

	c "github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func rhID(n int) string              { return fmt.Sprintf("C0000000-0000-4000-8000-%012X", n) }
func rhTime(n float64) c.Instant     { return c.Instant(math.Float64bits(n)) }
func rhPtr[T any](v T) *T            { return &v }
func rhDate(y, m, d int) c.LocalDate { return c.LocalDate{Year: y, Month: m, Day: d} }
func rhMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// 独立公开命令 fixture，不使用作者的 historyChain* helper。
func rhSeed(t *testing.T, day c.LocalDate, at c.Instant) *c.TaskKernel {
	t.Helper()
	k, err := c.NewTaskKernel(c.TaskGraph{})
	rhMust(t, err)
	description := "独立合成说明"
	note := c.TaskNoteEntry{ID: c.TaskNoteEntryID(rhID(5)), Body: "独立附言", CreatedAt: at, UpdatedAt: at}
	rhMust(t, k.CreatePoolTask(c.CreatePoolTaskInput{ChainID: c.TaskChainID(rhID(2)), DefinitionID: c.TaskDefinitionID(rhID(3)), Title: "独立任务", DescriptionText: &description, InitialNote: &note, Now: at}))
	rhMust(t, k.ScheduleFromPool(c.ScheduleFromPoolInput{ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), DayID: c.DayID(rhID(1)), Date: day, Today: day, Now: at}))
	return k
}

func rhRestore(t *testing.T, g c.TaskGraph) *c.TaskKernel {
	t.Helper()
	rhMust(t, c.ValidateTaskGraph(g))
	k, err := c.NewTaskKernel(g)
	rhMust(t, err)
	return k
}
func rhSame(t *testing.T, got, want c.TaskGraph) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("完整图不符合预期变更\ngot=%#v\nwant=%#v", got, want)
	}
	rhMust(t, c.ValidateTaskGraph(got))
}

func TestReviewerHistoryErrorPrecedenceAndRollback(t *testing.T) {
	d := rhDate(2026, 11, 6)
	seed := rhSeed(t, d, rhTime(10)).Snapshot()
	abandoned := seed.Clone()
	abandoned.Chains[0].State = c.ChainAbandoned
	abandoned.Traces[0].Status, abandoned.Traces[0].SettledAt = c.TraceAbandoned, rhPtr(rhTime(10))
	completed := seed.Clone()
	completed.Traces[0].Status, completed.Traces[0].CompletedAt = c.TraceCompleted, rhPtr(rhTime(10))
	conflict := abandoned.Clone()
	extra := seed.Traces[0]
	extra.ID, extra.Priority = c.DayTraceID(rhID(6)), 2
	conflict.Traces = append(conflict.Traces, extra)
	bad := rhTime(math.Inf(1))
	a := c.AbandonChainInput{ChainID: seed.Chains[0].ID, TraceID: seed.Traces[0].ID, Now: bad}
	r := c.ReactivateAbandonedChainInput{ChainID: a.ChainID, TraceID: a.TraceID, Today: d, Now: bad}
	for _, tc := range []struct {
		name   string
		g      c.TaskGraph
		modify func(*c.AbandonChainInput)
		want   error
	}{
		{"unknown-before-owner-state-clock", abandoned, func(v *c.AbandonChainInput) { v.TraceID = "missing"; v.ChainID = "wrong" }, c.ErrNotFound},
		{"owner-before-chain-clock", abandoned, func(v *c.AbandonChainInput) { v.ChainID = "wrong" }, c.ErrInvalidInput},
		{"chain-before-clock", abandoned, func(v *c.AbandonChainInput) {}, c.ErrChainAbandoned},
		{"status-before-clock", completed, func(v *c.AbandonChainInput) {}, c.ErrInvalidTransition},
		{"clock-last", seed, func(v *c.AbandonChainInput) {}, c.ErrInvalidClock},
	} {
		t.Run("abandon/"+tc.name, func(t *testing.T) {
			k := rhRestore(t, tc.g)
			in := a
			tc.modify(&in)
			if err := k.AbandonChain(in); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			rhSame(t, k.Snapshot(), tc.g)
		})
	}
	for _, tc := range []struct {
		name   string
		g      c.TaskGraph
		modify func(*c.ReactivateAbandonedChainInput)
		want   error
	}{
		{"invalid-civil-date-before-identity", abandoned, func(v *c.ReactivateAbandonedChainInput) { v.Today = c.LocalDate{}; v.TraceID = "missing" }, c.ErrInvalidInput},
		{"unknown-before-owner-clock", abandoned, func(v *c.ReactivateAbandonedChainInput) { v.TraceID = "missing"; v.ChainID = "wrong" }, c.ErrNotFound},
		{"owner-before-status-clock", seed, func(v *c.ReactivateAbandonedChainInput) { v.ChainID = "wrong" }, c.ErrInvalidInput},
		{"status-before-clock", seed, func(v *c.ReactivateAbandonedChainInput) {}, c.ErrInvalidTransition},
		{"active-before-clock", conflict, func(v *c.ReactivateAbandonedChainInput) {}, c.ErrActiveTraceAlreadyExists},
		{"clock-last", abandoned, func(v *c.ReactivateAbandonedChainInput) {}, c.ErrInvalidClock},
	} {
		t.Run("reactivate/"+tc.name, func(t *testing.T) {
			k := rhRestore(t, tc.g)
			in := r
			tc.modify(&in)
			id, err := k.ReactivateAbandonedChain(in)
			if id != "" || !errors.Is(err, tc.want) {
				t.Fatalf("want empty ID/%v, got %q/%v", tc.want, id, err)
			}
			rhSame(t, k.Snapshot(), tc.g)
		})
	}
}

func rhContinue(t *testing.T, k *c.TaskKernel, source c.DayTraceID, n int, target c.LocalDate, now float64) c.DayTraceID {
	t.Helper()
	id := c.DayTraceID(rhID(n))
	rhMust(t, k.ContinueUnfinishedTrace(c.TraceCarryoverInput{ChainID: c.TaskChainID(rhID(2)), TraceID: source, NewTraceID: id, DayID: c.DayID(rhID(n + 1)), TargetDate: target, Today: target, Now: rhTime(now)}))
	return id
}

func rhThreeHops(t *testing.T) (*c.TaskKernel, []c.DayTraceID) {
	t.Helper()
	k := rhSeed(t, rhDate(2026, 11, 3), rhTime(10))
	root := c.DayTraceID(rhID(4))
	rhMust(t, k.SettleTaskDays(rhDate(2026, 11, 4), rhTime(11)))
	second := rhContinue(t, k, root, 20, rhDate(2026, 11, 4), 12)
	rhMust(t, k.SettleTaskDays(rhDate(2026, 11, 5), rhTime(13)))
	third := rhContinue(t, k, second, 30, rhDate(2026, 11, 5), 14)
	return k, []c.DayTraceID{root, second, third}
}

func TestReviewerHistoryMultipleContinuationRedirectAndReturn(t *testing.T) {
	k, ids := rhThreeHops(t)
	// 调换 slice 次序，不能依赖目标在最后。历史内容时钟也不是活动目标。
	g := k.Snapshot()
	g.Traces[0].ContentUpdatedAt = rhTime(999)
	g.Traces[0], g.Traces[2] = g.Traces[2], g.Traces[0]
	k = rhRestore(t, g)
	rhMust(t, k.AbandonChain(c.AbandonChainInput{ChainID: g.Chains[0].ID, TraceID: ids[0], Now: rhTime(20)}))
	want := g.Clone()
	want.Chains[0].State, want.Chains[0].UpdatedAt = c.ChainAbandoned, rhTime(20)
	want.Traces[0].Status, want.Traces[0].ContentUpdatedAt, want.Traces[0].SettledAt = c.TraceAbandoned, rhTime(20), rhPtr(rhTime(20))
	rhSame(t, k.Snapshot(), want)
	id, err := k.ReactivateAbandonedChain(c.ReactivateAbandonedChainInput{ChainID: g.Chains[0].ID, TraceID: ids[2], Today: rhDate(2026, 11, 6), Now: rhTime(21)})
	rhMust(t, err)
	if id != ids[2] {
		t.Fatal("重新启用生成了不同身份")
	}
	want.Chains[0].State, want.Chains[0].UpdatedAt = c.ChainActive, rhTime(21)
	want.Traces[0].Status, want.Traces[0].ContentUpdatedAt = c.TraceUnfinished, rhTime(21)
	rhSame(t, k.Snapshot(), want)
	fourth := rhContinue(t, k, ids[2], 40, rhDate(2026, 11, 6), 22)
	before := k.Snapshot()
	rhMust(t, k.AbandonChain(c.AbandonChainInput{ChainID: g.Chains[0].ID, TraceID: ids[1], Now: rhTime(23)}))
	after := k.Snapshot()
	if len(after.Traces) != 4 || after.Traces[3].ID != fourth || after.Traces[3].Status != c.TraceAbandoned {
		t.Fatal("多次续接目标错误")
	}
	if !reflect.DeepEqual(after.Traces[:3], before.Traces[:3]) {
		t.Fatal("历史前缀被改写")
	}
	rhMust(t, k.SettleTaskDays(rhDate(2026, 11, 7), rhTime(24)))
	id, err = k.ReactivateAbandonedChain(c.ReactivateAbandonedChainInput{ChainID: g.Chains[0].ID, TraceID: fourth, Today: rhDate(2026, 11, 1), Now: rhTime(25)})
	rhMust(t, err)
	if id != fourth || k.Snapshot().Traces[3].Status != c.TraceUnfinished || *k.Snapshot().Traces[3].SettledAt != rhTime(23) {
		t.Fatal("锁日优先或原 settledAt 丢失")
	}
}

func TestReviewerHistoryReferencedAncestorsRejectAfterTargetCompleted(t *testing.T) {
	k, ids := rhThreeHops(t)
	rhMust(t, k.MarkCompleted(ids[2], rhDate(2026, 11, 5), rhTime(15)))
	for _, source := range ids[:2] {
		before := k.Snapshot()
		err := k.AbandonChain(c.AbandonChainInput{ChainID: before.Chains[0].ID, TraceID: source, Now: rhTime(16)})
		var ve *c.ValidationError
		if !errors.As(err, &ve) || !ve.Has("invalidTraceContinuation") {
			t.Fatalf("引用源需保留拓扑拒绝，got %v", err)
		}
		rhSame(t, k.Snapshot(), before)
	}
}

func TestReviewerHistoryFiniteClockSamplingAndExactPreservation(t *testing.T) {
	rng := rand.New(rand.NewPCG(771, 20261106))
	clocks := []float64{-math.MaxFloat64, -math.SmallestNonzeroFloat64, math.Copysign(0, -1), 0, math.SmallestNonzeroFloat64, math.MaxFloat64}
	for len(clocks) < 518 {
		v := math.Float64frombits(rng.Uint64())
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			clocks = append(clocks, v)
		}
	}
	for i, old := range clocks {
		k := rhSeed(t, rhDate(2026, 11, 6), rhTime(old))
		now := math.Nextafter(old, math.Inf(1))
		if math.IsInf(now, 0) {
			now = old
		}
		a := c.AbandonChainInput{ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), Now: rhTime(now)}
		rhMust(t, k.AbandonChain(a))
		s := k.Snapshot()
		if s.Chains[0].UpdatedAt != a.Now || s.Traces[0].ContentUpdatedAt != a.Now || *s.Traces[0].SettledAt != a.Now {
			t.Fatalf("sample %d lost IEEE bits", i)
		}
		_, err := k.ReactivateAbandonedChain(c.ReactivateAbandonedChainInput{ChainID: a.ChainID, TraceID: a.TraceID, Today: rhDate(2026, 11, 7), Now: a.Now})
		rhMust(t, err)
		before := k.Snapshot()
		a.Now = rhTime(math.Nextafter(now, math.Inf(-1)))
		if err := k.AbandonChain(a); !errors.Is(err, c.ErrInvalidClock) {
			t.Fatalf("sample %d accepted preceding float: %v", i, err)
		}
		rhSame(t, k.Snapshot(), before)
	}
}

func TestReviewerHistoryCivilDateEndpointsAndChoice(t *testing.T) {
	for _, tc := range []struct {
		on, today c.LocalDate
		want      c.TraceStatus
	}{
		{rhDate(1, 1, 1), rhDate(1, 1, 1), c.TracePending},
		{rhDate(1, 1, 1), rhDate(9999, 12, 31), c.TraceUnfinished},
		{rhDate(9999, 12, 31), rhDate(1, 1, 1), c.TracePending},
		{rhDate(9999, 12, 31), rhDate(9999, 12, 31), c.TracePending},
		{rhDate(2000, 2, 29), rhDate(2000, 3, 1), c.TraceUnfinished},
	} {
		t.Run(fmt.Sprint(tc.on, "/", tc.today), func(t *testing.T) {
			k := rhSeed(t, tc.on, rhTime(0))
			a := c.AbandonChainInput{ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), Now: rhTime(1)}
			rhMust(t, k.AbandonChain(a))
			id, err := k.ReactivateAbandonedChain(c.ReactivateAbandonedChainInput{ChainID: a.ChainID, TraceID: a.TraceID, Today: tc.today, Now: rhTime(2)})
			rhMust(t, err)
			out := k.Snapshot()
			if id != a.TraceID || out.Traces[0].Status != tc.want {
				t.Fatal("civil date endpoint changed decision")
			}
			if tc.want == c.TraceUnfinished && *out.Traces[0].SettledAt != a.Now {
				t.Fatal("settledAt replaced")
			}
			rhMust(t, c.ValidateTaskGraph(out))
		})
	}
}

func TestReviewerHistorySnapshotsStayDetachedAcrossLifecycle(t *testing.T) {
	d := rhDate(2026, 11, 6)
	k := rhSeed(t, d, rhTime(0))
	rhMust(t, k.EditSubtask(c.SubtaskEditInput{Kind: c.SubtaskAdd, ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), SubtaskID: c.SubtaskID(rhID(10)), LineageID: c.SubtaskLineageID(rhID(11)), Title: "保留子项", Difficulty: c.DifficultyHard, Today: d, Now: rhTime(0)}))
	g := k.Snapshot()
	g.Traces[0].PinOrder = rhPtr(1)
	g.Traces[0].ManualProgressPercent = rhPtr(31)
	k = rhRestore(t, g)
	before := k.Snapshot()
	frozen := before.Clone()
	a := c.AbandonChainInput{ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), Now: rhTime(1)}
	rhMust(t, k.AbandonChain(a))
	abandoned := k.Snapshot()
	saved := abandoned.Clone()
	_, err := k.ReactivateAbandonedChain(c.ReactivateAbandonedChainInput{ChainID: a.ChainID, TraceID: a.TraceID, Today: d, Now: rhTime(2)})
	rhMust(t, err)
	rhSame(t, before, frozen)
	rhSame(t, abandoned, saved)
	want := k.Snapshot()
	for _, external := range []*c.TaskGraph{&g, &before, &abandoned} {
		*external.Traces[0].DescriptionText = "改写"
		external.Chains[0].NoteEntries[0].Body = "改写"
		external.Traces[0].NoteEntries[0].Body = "改写"
		external.Subtasks[0].Title = "改写"
		*external.Traces[0].ManualProgressPercent = 92
	}
	*abandoned.Traces[0].SettledAt = rhTime(999)
	rhSame(t, k.Snapshot(), want)
}

func TestReviewerHistoryConcurrentMixedCommandsPublishWholeGraph(t *testing.T) {
	d := rhDate(2026, 11, 6)
	k := rhSeed(t, d, rhTime(0))
	a := c.AbandonChainInput{ChainID: c.TaskChainID(rhID(2)), TraceID: c.DayTraceID(rhID(4)), Now: rhTime(0)}
	r := c.ReactivateAbandonedChainInput{ChainID: a.ChainID, TraceID: a.TraceID, Today: d, Now: a.Now}
	errCh := make(chan error, 10)
	var wg sync.WaitGroup
	for worker := 0; worker < 10; worker++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if (n+j)%2 == 0 {
					if err := k.AbandonChain(a); err != nil && !errors.Is(err, c.ErrChainAbandoned) {
						errCh <- err
						return
					}
				} else {
					id, err := k.ReactivateAbandonedChain(r)
					if (err == nil && id != r.TraceID) || (err != nil && (id != "" || !errors.Is(err, c.ErrInvalidTransition))) {
						errCh <- fmt.Errorf("ID %q err %v", id, err)
						return
					}
				}
				g := k.Snapshot()
				if err := c.ValidateTaskGraph(g); err != nil {
					errCh <- err
					return
				}
				if (g.Chains[0].State == c.ChainActive) != (g.Traces[0].Status == c.TracePending) {
					errCh <- fmt.Errorf("observed partial publish")
					return
				}
				g.Chains[0].NoteEntries[0].Body = "局部快照"
				*g.Traces[0].DescriptionText = "局部快照"
			}
		}(worker)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	out := k.Snapshot()
	if out.Chains[0].NoteEntries[0].Body != "独立附言" || *out.Traces[0].DescriptionText != "独立合成说明" {
		t.Fatal("并发 snapshot 引用泄漏")
	}
}
