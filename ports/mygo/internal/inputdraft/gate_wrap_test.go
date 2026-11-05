// SPDX-License-Identifier: AGPL-3.0-only
package inputdraft

import (
	"math"
	"testing"
)

func TestWrappingGenerationRemainsValidButInitialZeroDoesNot(t *testing.T) {
	g, err := NewWithGeneration("synthetic", math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	old := mustSchedule(t, g.DraftDidChange())
	g.DiscardLocalChanges()
	if g.Generation() != 0 || !g.initialized || g.PermitsAutosave(&old) {
		t.Fatal("wrapped generation must remain valid and reject old work")
	}
	s := mustSchedule(t, g.DraftDidChange())
	r := mustRequest(t, g.BeginPersistence(s))
	if s.Generation != 0 || r.Generation != 0 {
		t.Fatal("zero generation from wrapping must remain usable")
	}
	g.PersistenceDidSucceed(r)
	if g.HasUnflushedChanges() {
		t.Fatal("wrapped generation must accept matching completion")
	}
}

func TestWrappingRevisionRetainsUnsignedMaximumAckBehavior(t *testing.T) {
	g := mustGate(t)
	g.revision = math.MaxUint64 - 1
	g.acknowledgedRevision = math.MaxUint64 - 1
	s := mustSchedule(t, g.DraftDidChange())
	r := mustRequest(t, g.BeginPersistence(s))
	wrapped := mustSchedule(t, g.DraftDidChange())
	if wrapped.Revision != 0 {
		t.Fatal("revision must wrap")
	}
	after := mustSchedule(t, g.PersistenceDidSucceed(r))
	if g.acknowledgedRevision != math.MaxUint64 || after.Revision != 0 {
		t.Fatal("success must apply unsigned max without modulo ordering")
	}
	r = mustRequest(t, g.BeginPersistence(after))
	after = mustSchedule(t, g.PersistenceDidSucceed(r))
	if g.acknowledgedRevision != math.MaxUint64 || !g.HasUnflushedChanges() || after.Revision != 0 {
		t.Fatal("source wrap oddity must remain: ack revision 0 cannot lower maximum ack")
	}
	g.AcknowledgeWithoutPersistence()
	if g.HasUnflushedChanges() || g.acknowledgedRevision != 0 {
		t.Fatal("explicit acknowledgement may reset wrapped ack")
	}
}

func TestRevisionWrapCanMakeDraftAppearClean(t *testing.T) {
	g := mustGate(t)
	g.revision = math.MaxUint64
	if g.DraftDidChange() != nil || g.revision != 0 || g.HasUnflushedChanges() || g.scheduleSequence != 1 {
		t.Fatal("source revision equality at wrap must be preserved")
	}
}

func TestWrappingScheduleAndRequestSequences(t *testing.T) {
	g := mustGate(t)
	g.scheduleSequence = math.MaxUint64 - 1
	g.requestSequence = math.MaxUint64
	s := mustSchedule(t, g.DraftDidChange())
	if s.Sequence != 0 || !g.PermitsAutosave(&s) {
		t.Fatal("invalidate plus schedule must wrap sequence to zero")
	}
	r := mustRequest(t, g.BeginPersistence(s))
	if r.RequestID != 0 || !g.HasPersistenceInFlight() {
		t.Fatal("request zero after wrapping remains valid")
	}
	g.PersistenceDidSucceed(r)
	if g.HasUnflushedChanges() || g.HasPersistenceInFlight() {
		t.Fatal("wrapped request ack rejected")
	}
	g.scheduleSequence = math.MaxUint64
	g.AcknowledgeWithoutPersistence()
	if g.scheduleSequence != 0 {
		t.Fatal("invalidate must wrap sequence")
	}
}

func TestFailureWhileCleanDoesNotSchedule(t *testing.T) {
	g, r := saving(t)
	// 通过 revision wrapping 造成 dirty equality 的 guard 边界，直接设置内部 fixture。
	g.revision = g.acknowledgedRevision
	sequence := g.scheduleSequence
	if g.PersistenceDidFail(r) != nil || g.retryAttempt != 1 || g.HasPersistenceInFlight() || g.scheduleSequence != sequence {
		t.Fatal("clean failure must only release request and count retry")
	}
}

func TestRetryAttemptWrapPreservesSwiftUnderflowTrap(t *testing.T) {
	g, r := saving(t)
	g.retryAttempt = math.MaxUint64
	sequence := g.scheduleSequence
	defer func() {
		if recover() == nil {
			t.Fatal("Swift retryAttempt - 1 underflow must fail closed")
		}
		if g.retryAttempt != 0 || g.HasPersistenceInFlight() || g.scheduleSequence != sequence || !g.HasUnflushedChanges() {
			t.Fatal("underflow pre-trap state must match Swift ordering")
		}
	}()
	g.PersistenceDidFail(r)
}

func TestRetryAttemptWrapGuardPreventsUnderflow(t *testing.T) {
	for _, composing := range []bool{false, true} {
		g, r := saving(t)
		g.retryAttempt = math.MaxUint64
		if composing {
			g.isComposing = true
		} else {
			g.revision = g.acknowledgedRevision
		}
		if g.PersistenceDidFail(r) != nil || g.retryAttempt != 0 || g.HasPersistenceInFlight() {
			t.Fatal("guard must return before overflowing subtraction")
		}
	}
}
