// SPDX-License-Identifier: AGPL-3.0-only
// 根据 quboliu/noonmark 43e6eef 的 Swift 状态转换派生边界与 guard 测试。
package inputdraft

import (
	"fmt"
	"math"
	"testing"
)

func mustGate(t *testing.T) *Gate {
	t.Helper()
	g, err := New("synthetic:task:title")
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func mustSchedule(t *testing.T, s *Schedule) Schedule {
	t.Helper()
	if s == nil {
		t.Fatal("expected schedule")
	}
	return *s
}
func mustRequest(t *testing.T, r *PersistenceRequest) PersistenceRequest {
	t.Helper()
	if r == nil {
		t.Fatal("expected request")
	}
	return *r
}
func saving(t *testing.T) (*Gate, PersistenceRequest) {
	t.Helper()
	g := mustGate(t)
	return g, mustRequest(t, g.BeginPersistence(mustSchedule(t, g.DraftDidChange())))
}
func wantSchedule(t *testing.T, g *Gate, got *Schedule, sequence, revision, delay uint64) {
	t.Helper()
	want := Schedule{OwnerID: g.ownerID, Generation: g.generation, Sequence: sequence, Revision: revision, DelayMilliseconds: delay}
	if got == nil || *got != want {
		t.Fatalf("schedule = %+v, want %+v", got, want)
	}
}

func TestExactSequencesAndNoOpEvents(t *testing.T) {
	g := mustGate(t)
	before := *g
	if g.NativeSnapshotDidChange(false, false) != nil || *g != before {
		t.Fatal("unchanged snapshot must be no-op")
	}
	wantSchedule(t, g, g.DraftDidChange(), 2, 1, 700)
	before = *g
	if g.CompositionDidChange(false) != nil || *g != before {
		t.Fatal("same composing state must not invalidate")
	}
	if g.CompositionDidChange(true) != nil || g.scheduleSequence != 3 || g.revision != 1 {
		t.Fatal("composition start must only invalidate")
	}
	before = *g
	if g.NativeSnapshotDidChange(false, true) != nil || *g != before {
		t.Fatal("unchanged marked snapshot must be no-op")
	}
	if g.DraftDidChange() != nil || g.scheduleSequence != 4 || g.revision != 2 {
		t.Fatal("marked edit must increment revision and invalidate")
	}
	wantSchedule(t, g, g.CompositionDidChange(false), 6, 2, 700)
	wantSchedule(t, g, g.RequestFlush(), 7, 2, 0)
	s := mustSchedule(t, g.RequestFlush())
	wantSchedule(t, g, &s, 8, 2, 0)
	r := mustRequest(t, g.BeginPersistence(s))
	if r != (PersistenceRequest{OwnerID: g.ownerID, Generation: 1, Revision: 2, RequestID: 1}) || g.scheduleSequence != 8 {
		t.Fatal("begin must only advance request sequence")
	}
	if g.PersistenceDidSucceed(r) != nil || g.scheduleSequence != 9 || g.retryAttempt != 0 || g.explicitFlushPending {
		t.Fatal("clean success must invalidate and reset")
	}
}

func TestNativeSnapshotAllBooleanBranches(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		for _, composing := range []bool{false, true} {
			for _, textChanged := range []bool{false, true} {
				for _, nextComposing := range []bool{false, true} {
					for _, flush := range []bool{false, true} {
						name := fmt.Sprintf("dirty=%t/compose=%t/text=%t/next=%t/flush=%t", dirty, composing, textChanged, nextComposing, flush)
						t.Run(name, func(t *testing.T) {
							g := mustGate(t)
							g.isComposing, g.explicitFlushPending, g.retryAttempt = composing, flush, 3
							if dirty {
								g.revision = 1
							}
							before := *g
							s := g.NativeSnapshotDidChange(textChanged, nextComposing)
							changed := textChanged || composing != nextComposing
							if !changed {
								if s != nil || *g != before {
									t.Fatal("no change mutated gate")
								}
								return
							}
							wantRevision, wantRetry := before.revision, uint64(3)
							if textChanged {
								wantRevision++
								wantRetry = 0
							}
							if g.revision != wantRevision || g.retryAttempt != wantRetry || g.isComposing != nextComposing {
								t.Fatal("wrong snapshot state")
							}
							shouldSchedule := !nextComposing && (dirty || textChanged)
							if shouldSchedule {
								delay := uint64(700)
								if flush {
									delay = 0
								}
								wantSchedule(t, g, s, 2, wantRevision, delay)
							} else if s != nil || g.scheduleSequence != 1 {
								t.Fatal("unscheduled change must invalidate exactly once")
							}
						})
					}
				}
			}
		}
	}
}

func TestPermitsAutosaveEveryGuard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Gate, *Schedule)
	}{
		{"owner", func(_ *Gate, s *Schedule) { s.OwnerID += "wrong" }},
		{"generation", func(_ *Gate, s *Schedule) { s.Generation++ }},
		{"revision", func(_ *Gate, s *Schedule) { s.Revision++ }},
		{"sequence", func(_ *Gate, s *Schedule) { s.Sequence++ }},
		{"composing", func(g *Gate, _ *Schedule) { g.isComposing = true }},
		{"clean", func(g *Gate, _ *Schedule) { g.acknowledgedRevision = g.revision }},
		{"in_flight", func(g *Gate, _ *Schedule) { g.hasInFlight = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := mustGate(t)
			s := mustSchedule(t, g.DraftDidChange())
			tc.mutate(g, &s)
			before := *g
			if g.PermitsAutosave(&s) || g.BeginPersistence(s) != nil || *g != before {
				t.Fatal("guard did not reject without mutation")
			}
		})
	}
	g := mustGate(t)
	s := mustSchedule(t, g.DraftDidChange())
	s.DelayMilliseconds = math.MaxUint64
	if g.PermitsAutosave(nil) || !g.PermitsAutosave(&s) {
		t.Fatal("nil must reject; delay is not an identity field")
	}
	mustRequest(t, g.BeginPersistence(s))
	if g.BeginPersistence(s) != nil {
		t.Fatal("second in-flight save permitted")
	}
}

func TestFlushAllGuardCombinations(t *testing.T) {
	for _, composing := range []bool{false, true} {
		for _, dirty := range []bool{false, true} {
			for _, flight := range []bool{false, true} {
				g := mustGate(t)
				g.isComposing, g.hasInFlight = composing, flight
				if dirty {
					g.revision = 1
				}
				s := g.RequestFlush()
				if !g.explicitFlushPending {
					t.Fatal("flush intent must always stick")
				}
				if !composing && dirty && !flight {
					wantSchedule(t, g, s, 1, 1, 0)
				} else if s != nil || g.scheduleSequence != 0 {
					t.Fatal("blocked flush unexpectedly scheduled or invalidated")
				}
			}
		}
	}
}

func TestCleanFlushIsStickyAndRepeatedFlushInvalidates(t *testing.T) {
	g := mustGate(t)
	if g.RequestFlush() != nil {
		t.Fatal("clean flush must not schedule")
	}
	first := mustSchedule(t, g.DraftDidChange())
	if first.DelayMilliseconds != 0 {
		t.Fatal("clean flush must apply to next edit")
	}
	second := mustSchedule(t, g.RequestFlush())
	if g.PermitsAutosave(&first) || !g.PermitsAutosave(&second) {
		t.Fatal("repeat flush must replace scheduled token")
	}
}

func TestCallbacksRejectEachIdentityMismatch(t *testing.T) {
	for _, callback := range []string{"success", "failure"} {
		for _, field := range []string{"owner", "generation", "revision", "request_id", "absent", "gate_owner", "gate_generation"} {
			t.Run(callback+"/"+field, func(t *testing.T) {
				g, r := saving(t)
				switch field {
				case "owner":
					r.OwnerID += "wrong"
				case "generation":
					r.Generation++
				case "revision":
					r.Revision++
				case "request_id":
					r.RequestID++
				case "absent":
					g.clearInFlight()
				case "gate_owner":
					g.ownerID += "wrong"
				case "gate_generation":
					g.generation++
				}
				before := *g
				var s *Schedule
				if callback == "success" {
					s = g.PersistenceDidSucceed(r)
				} else {
					s = g.PersistenceDidFail(r)
				}
				if s != nil || *g != before {
					t.Fatal("mismatched callback mutated gate")
				}
			})
		}
	}
}

func TestSingleInFlightAndNewerEdits(t *testing.T) {
	for _, flush := range []bool{false, true} {
		g, r := saving(t)
		if flush && g.RequestFlush() != nil {
			t.Fatal("in-flight flush scheduled")
		}
		during := mustSchedule(t, g.DraftDidChange())
		if g.PermitsAutosave(&during) || g.BeginPersistence(during) != nil {
			t.Fatal("new edit must not start concurrent save")
		}
		after := mustSchedule(t, g.PersistenceDidSucceed(r))
		wantDelay := uint64(700)
		if flush {
			wantDelay = 0
		}
		if after.DelayMilliseconds != wantDelay || after.Revision != 2 || g.acknowledgedRevision != 1 || !g.HasUnflushedChanges() || g.PermitsAutosave(&during) {
			t.Fatal("new edit acknowledgement mismatch")
		}
		r2 := mustRequest(t, g.BeginPersistence(after))
		if r2.RequestID != 2 {
			t.Fatal("request ID must advance")
		}
		before := *g
		if g.PersistenceDidSucceed(r) != nil || g.PersistenceDidFail(r) != nil || *g != before {
			t.Fatal("duplicate old callback disturbed next save")
		}
		g.PersistenceDidSucceed(r2)
		if g.HasUnflushedChanges() || g.HasPersistenceInFlight() || g.explicitFlushPending {
			t.Fatal("final matching ack must clean")
		}
	}
}

func TestSuccessWhileComposingReturnsBlockedSchedule(t *testing.T) {
	g, r := saving(t)
	g.NativeSnapshotDidChange(true, true)
	s := mustSchedule(t, g.PersistenceDidSucceed(r))
	if !g.IsComposing() || !g.HasUnflushedChanges() || g.PermitsAutosave(&s) || s.DelayMilliseconds != 700 {
		t.Fatal("source success/composing oddity changed")
	}
	final := mustSchedule(t, g.CompositionDidChange(false))
	if g.PermitsAutosave(&s) || !g.PermitsAutosave(&final) {
		t.Fatal("composition end must replace blocked schedule")
	}
}

func TestSuccessCleanWhileComposingKeepsComposition(t *testing.T) {
	g, r := saving(t)
	g.CompositionDidChange(true)
	if g.PersistenceDidSucceed(r) != nil || g.HasUnflushedChanges() || !g.IsComposing() {
		t.Fatal("success must clean matching revision without ending composition")
	}
	if g.CompositionDidChange(false) != nil {
		t.Fatal("clean composition end must not save")
	}
}

func TestRetrySeriesAndStickyFlush(t *testing.T) {
	g := mustGate(t)
	g.DraftDidChange()
	s := mustSchedule(t, g.RequestFlush())
	for i, want := range []uint64{1000, 2000, 4000, 8000, 10000, 10000, 10000} {
		r := mustRequest(t, g.BeginPersistence(s))
		old := s
		s = mustSchedule(t, g.PersistenceDidFail(r))
		if s.DelayMilliseconds != want || g.retryAttempt != uint64(i+1) || !g.explicitFlushPending || !g.HasUnflushedChanges() || g.HasPersistenceInFlight() || g.PermitsAutosave(&old) {
			t.Fatalf("retry %d mismatch: %+v", i+1, s)
		}
		before := *g
		if g.PersistenceDidFail(r) != nil || g.PersistenceDidSucceed(r) != nil || *g != before {
			t.Fatal("duplicate completion mutated retry")
		}
	}
}

func TestRetryResetAndCompositionResumeSemantics(t *testing.T) {
	g, r := saving(t)
	s := mustSchedule(t, g.PersistenceDidFail(r))
	r = mustRequest(t, g.BeginPersistence(s))
	g.CompositionDidChange(true)
	if g.PersistenceDidFail(r) != nil || g.retryAttempt != 2 {
		t.Fatal("failure while composing must count retry without scheduling")
	}
	s = mustSchedule(t, g.CompositionDidChange(false))
	if s.DelayMilliseconds != 700 || g.retryAttempt != 2 {
		t.Fatal("composition-only resume uses debounce and retains retry count")
	}
	r = mustRequest(t, g.BeginPersistence(s))
	s = mustSchedule(t, g.PersistenceDidFail(r))
	if s.DelayMilliseconds != 4000 {
		t.Fatal("next failure must use retained attempt")
	}
	s = mustSchedule(t, g.DraftDidChange())
	if s.DelayMilliseconds != 700 || g.retryAttempt != 0 {
		t.Fatal("new text must reset retry")
	}
	r = mustRequest(t, g.BeginPersistence(s))
	s = mustSchedule(t, g.PersistenceDidFail(r))
	if s.DelayMilliseconds != 1000 {
		t.Fatal("new text retry series must restart")
	}
	r = mustRequest(t, g.BeginPersistence(s))
	g.DraftDidChange()
	g.retryAttempt = 3
	s = mustSchedule(t, g.PersistenceDidSucceed(r))
	if g.retryAttempt != 0 || s.DelayMilliseconds != 700 {
		t.Fatal("success with later input must reset retry")
	}
}

func TestFailureAfterNewEditUsesRetryDelay(t *testing.T) {
	g, r := saving(t)
	g.retryAttempt = 4
	newer := mustSchedule(t, g.DraftDidChange())
	retry := mustSchedule(t, g.PersistenceDidFail(r))
	if retry.Revision != 2 || retry.DelayMilliseconds != 1000 || g.acknowledgedRevision != 0 || g.PermitsAutosave(&newer) {
		t.Fatal("failure must retry current revision after edit reset")
	}
}

func TestAcknowledgeRetainsCompositionAndRejectsOldWork(t *testing.T) {
	g, r := saving(t)
	g.NativeSnapshotDidChange(true, true)
	g.RequestFlush()
	g.retryAttempt = 4
	sequence, requests, generation := g.scheduleSequence, g.requestSequence, g.generation
	g.AcknowledgeWithoutPersistence()
	if !g.IsComposing() || g.HasUnflushedChanges() || g.HasPersistenceInFlight() || g.explicitFlushPending || g.retryAttempt != 0 || g.acknowledgedRevision != 2 || g.scheduleSequence != sequence+1 || g.requestSequence != requests || g.generation != generation {
		t.Fatal("acknowledge state mismatch")
	}
	before := *g
	if g.PersistenceDidSucceed(r) != nil || g.PersistenceDidFail(r) != nil || *g != before {
		t.Fatal("acknowledged request must be stale")
	}
	if g.CompositionDidChange(false) != nil {
		t.Fatal("clean composition end must not schedule")
	}
	if mustSchedule(t, g.DraftDidChange()).DelayMilliseconds != 700 {
		t.Fatal("acknowledge must clear sticky flush")
	}
}

func TestDiscardResetsOnlySpecifiedState(t *testing.T) {
	g, r := saving(t)
	old := mustSchedule(t, g.DraftDidChange())
	g.CompositionDidChange(true)
	g.RequestFlush()
	g.retryAttempt = 4
	sequence, requests := g.scheduleSequence, g.requestSequence
	g.DiscardLocalChanges()
	if g.Generation() != 2 || g.OwnerID() != "synthetic:task:title" || g.revision != 0 || g.acknowledgedRevision != 0 || g.IsComposing() || g.HasUnflushedChanges() || g.HasPersistenceInFlight() || g.explicitFlushPending || g.retryAttempt != 0 || g.scheduleSequence != sequence+1 || g.requestSequence != requests {
		t.Fatal("discard state mismatch")
	}
	before := *g
	if g.PermitsAutosave(&old) || g.PersistenceDidSucceed(r) != nil || g.PersistenceDidFail(r) != nil || *g != before {
		t.Fatal("discarded work must be stale")
	}
	fresh := mustSchedule(t, g.DraftDidChange())
	next := mustRequest(t, g.BeginPersistence(fresh))
	if next.Generation != 2 || next.Revision != 1 || next.RequestID != requests+1 || fresh.DelayMilliseconds != 700 {
		t.Fatal("discard must retain monotonically wrapping request counter")
	}
}

func TestReturnValuesAndGateCopiesDoNotAlias(t *testing.T) {
	g := mustGate(t)
	s := g.DraftDidChange()
	originalSchedule := *s
	s.OwnerID = "mutated"
	if !g.PermitsAutosave(&originalSchedule) || g.PermitsAutosave(s) {
		t.Fatal("schedule caller mutation affected state")
	}
	r := g.BeginPersistence(originalSchedule)
	originalRequest := *r
	copied := *g
	r.OwnerID = "mutated"
	if g.PersistenceDidSucceed(*r) != nil || !g.HasPersistenceInFlight() {
		t.Fatal("request pointer aliases in-flight state")
	}
	g.PersistenceDidSucceed(originalRequest)
	if !copied.HasPersistenceInFlight() || !copied.HasUnflushedChanges() {
		t.Fatal("gate copy aliases state")
	}
	copied.PersistenceDidSucceed(originalRequest)
	if copied.HasUnflushedChanges() {
		t.Fatal("independent copy must still accept its request")
	}
}

func TestOwnerIdentityIsCallerResponsibility(t *testing.T) {
	a, b := mustGate(t), mustGate(t)
	sa, sb := mustSchedule(t, a.DraftDidChange()), mustSchedule(t, b.DraftDidChange())
	if sa != sb || !a.PermitsAutosave(&sb) {
		t.Fatal("source tokens have no hidden gate nonce")
	}
	c, _ := NewWithGeneration(a.OwnerID(), 2)
	sc := mustSchedule(t, c.DraftDidChange())
	if a.PermitsAutosave(&sc) {
		t.Fatal("different generation must be isolated")
	}
}
