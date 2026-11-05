// SPDX-License-Identifier: AGPL-3.0-only
// 改编自 quboliu/noonmark 43e6eef 的 IMETextDraftAutosaveGateTests.swift。
package inputdraft_test

import (
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/inputdraft"
)

func newGate(t *testing.T, owner string) *inputdraft.Gate {
	t.Helper()
	g, err := inputdraft.New(owner)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func schedule(t *testing.T, s *inputdraft.Schedule) inputdraft.Schedule {
	t.Helper()
	if s == nil {
		t.Fatal("expected schedule")
	}
	return *s
}

func request(t *testing.T, r *inputdraft.PersistenceRequest) inputdraft.PersistenceRequest {
	t.Helper()
	if r == nil {
		t.Fatal("expected persistence request")
	}
	return *r
}

func TestOrdinaryDraftPersistsOnlyAfterMatchingSuccessAck(t *testing.T) {
	g := newGate(t, "task:one:title")
	s := schedule(t, g.DraftDidChange())
	r := request(t, g.BeginPersistence(s))
	if !g.HasUnflushedChanges() {
		t.Fatal("begin must not acknowledge")
	}
	if g.PersistenceDidSucceed(r) != nil || g.HasUnflushedChanges() {
		t.Fatal("matching success must clean draft")
	}
}

func TestCompositionInvalidatesTimerAndPersistsOnlyFinalRevision(t *testing.T) {
	g := newGate(t, "task:one:title")
	stale := schedule(t, g.DraftDidChange())
	if g.NativeSnapshotDidChange(true, true) != nil || g.PermitsAutosave(&stale) {
		t.Fatal("composing must block old timer")
	}
	if g.NativeSnapshotDidChange(true, true) != nil {
		t.Fatal("marked update must not schedule")
	}
	final := schedule(t, g.NativeSnapshotDidChange(true, false))
	if !g.PermitsAutosave(&final) || final.Revision != 4 {
		t.Fatalf("wrong final schedule: %+v", final)
	}
}

func TestExplicitFlushWaitsForCompositionFinalization(t *testing.T) {
	g := newGate(t, "review:day:summary")
	g.CompositionDidChange(true)
	g.DraftDidChange()
	if g.RequestFlush() != nil {
		t.Fatal("flush must wait for composition")
	}
	s := schedule(t, g.CompositionDidChange(false))
	if s.DelayMilliseconds != 0 {
		t.Fatal("finalized flush must be immediate")
	}
}

func TestCompositionTransitionInvalidatesSameRevisionSchedule(t *testing.T) {
	g := newGate(t, "review:day:summary")
	before := schedule(t, g.DraftDidChange())
	if g.CompositionDidChange(true) != nil {
		t.Fatal("composing must not schedule")
	}
	after := schedule(t, g.CompositionDidChange(false))
	if g.PermitsAutosave(&before) || !g.PermitsAutosave(&after) || before.Revision != after.Revision {
		t.Fatal("composition must invalidate same-revision token")
	}
}

func TestLaterInputMakesOlderScheduleAndAckUnableToClearDirty(t *testing.T) {
	g := newGate(t, "task:one:body")
	first := schedule(t, g.DraftDidChange())
	r := request(t, g.BeginPersistence(first))
	g.DraftDidChange()
	next := schedule(t, g.PersistenceDidSucceed(r))
	if !g.HasUnflushedChanges() || !g.PermitsAutosave(&next) || g.PermitsAutosave(&first) {
		t.Fatal("old success must leave newer draft dirty")
	}
}

func TestFailureKeepsDirtyAndSchedulesBoundedRetry(t *testing.T) {
	g := newGate(t, "task:one:body")
	r := request(t, g.BeginPersistence(schedule(t, g.DraftDidChange())))
	retry := schedule(t, g.PersistenceDidFail(r))
	if !g.HasUnflushedChanges() || retry.DelayMilliseconds != 1000 || !g.PermitsAutosave(&retry) {
		t.Fatalf("wrong retry: %+v", retry)
	}
}

func TestWrongOwnerOrGenerationAckIsIgnored(t *testing.T) {
	g := newGate(t, "task:one:title")
	r := request(t, g.BeginPersistence(schedule(t, g.DraftDidChange())))
	wrong := r
	wrong.OwnerID = "task:two:title"
	if g.PersistenceDidSucceed(wrong) != nil || !g.HasUnflushedChanges() {
		t.Fatal("wrong owner must not acknowledge")
	}
	g.DiscardLocalChanges()
	if g.PersistenceDidSucceed(r) != nil || g.HasUnflushedChanges() {
		t.Fatal("old generation must not acknowledge")
	}
}

func TestDuplicateEndEditingAndDisappearCannotPersistTwice(t *testing.T) {
	g := newGate(t, "task:one:title")
	g.DraftDidChange()
	r := request(t, g.BeginPersistence(schedule(t, g.RequestFlush())))
	if g.RequestFlush() != nil {
		t.Fatal("second flush must not start second save")
	}
	if g.PersistenceDidSucceed(r) != nil || g.RequestFlush() != nil {
		t.Fatal("clean duplicate flush must not save")
	}
}

func TestConstructionAndZeroValuesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		owner      string
		generation uint64
	}{{"", 1}, {"owner", 0}, {"", 0}} {
		g, err := inputdraft.NewWithGeneration(tc.owner, tc.generation)
		if err == nil || g != nil {
			t.Fatalf("invalid construction accepted: %+v", tc)
		}
	}
	g, err := inputdraft.NewWithGeneration(" \t", 7)
	if err != nil || g.OwnerID() != " \t" || g.Generation() != 7 {
		t.Fatal("owner must not be trimmed")
	}
	if g.IsComposing() || g.HasUnflushedChanges() || g.HasPersistenceInFlight() {
		t.Fatal("new gate must be clean")
	}
	for _, invalid := range []*inputdraft.Gate{nil, new(inputdraft.Gate)} {
		if invalid.OwnerID() != "" || invalid.Generation() != 0 || invalid.IsComposing() || invalid.HasUnflushedChanges() || invalid.HasPersistenceInFlight() {
			t.Fatal("invalid getter exposed state")
		}
		if invalid.DraftDidChange() != nil || invalid.CompositionDidChange(true) != nil || invalid.NativeSnapshotDidChange(true, false) != nil || invalid.RequestFlush() != nil {
			t.Fatal("invalid gate scheduled work")
		}
		if invalid.PermitsAutosave(nil) || invalid.PermitsAutosave(&inputdraft.Schedule{}) || invalid.BeginPersistence(inputdraft.Schedule{}) != nil {
			t.Fatal("invalid gate permitted save")
		}
		if invalid.PersistenceDidSucceed(inputdraft.PersistenceRequest{}) != nil || invalid.PersistenceDidFail(inputdraft.PersistenceRequest{}) != nil {
			t.Fatal("invalid gate accepted callback")
		}
		invalid.AcknowledgeWithoutPersistence()
		invalid.DiscardLocalChanges()
		if invalid.Generation() != 0 || invalid.HasUnflushedChanges() || invalid.IsComposing() {
			t.Fatal("invalid gate mutated")
		}
	}
}
