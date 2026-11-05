// SPDX-License-Identifier: AGPL-3.0-only
package titlesession_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

func session(t *testing.T, text string) *titlesession.Session {
	t.Helper()
	s, err := titlesession.New(titlesession.Config{OwnerID: "task:synthetic:title", SessionID: 17, Generation: 29, TraceID: "synthetic-trace-1", PersistedText: text})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func native(s *titlesession.Session, text string, composing bool) titlesession.Update {
	return s.NativeSnapshotDidChange(s.Identity(), titlesession.NativeSnapshot{Text: text, Composing: composing})
}
func lifecycle(s *titlesession.Session, text string, composing bool, finalized bool) titlesession.Update {
	return s.RequestLifecycleFlush(s.Identity(), titlesession.NativeSnapshot{Text: text, Composing: composing}, finalized)
}
func job(t *testing.T, u titlesession.Update, delay uint64) titlesession.Job {
	t.Helper()
	if !u.Accepted || u.Job == nil || u.Job.DelayMilliseconds() != delay {
		t.Fatalf("job: accepted=%v err=%v job=%+v, want delay %d", u.Accepted, u.Err, u.Job, delay)
	}
	return *u.Job
}
func start(t *testing.T, s *titlesession.Session, j titlesession.Job, raw, normalized string) titlesession.SaveRequest {
	t.Helper()
	b := s.BeginPersistence(j)
	if !b.Update.Accepted || b.Request == nil || b.Request.RawDraft() != raw || b.Request.NormalizedText() != normalized || !b.Update.State.InFlight || !b.Update.State.Dirty {
		t.Fatalf("start: %+v, request=%+v, want raw=%q normalized=%q", b.Update, b.Request, raw, normalized)
	}
	if b.Request.TraceID() != "synthetic-trace-1" || b.Request.Identity().OwnerID() != "task:synthetic:title" || b.Request.Identity().SessionID() != 17 {
		t.Fatalf("incorrect captured route: %+v", b.Request)
	}
	return *b.Request
}
func succeed(s *titlesession.Session, r titlesession.SaveRequest, actual string) titlesession.Update {
	return s.PersistenceDidComplete(r, titlesession.PersistenceResult{VerifiedText: titlesession.OptionalText{Text: actual, Present: true}})
}
func texts(t *testing.T, s *titlesession.Session, draft, native, ack string, dirty, flight bool) {
	t.Helper()
	v := s.State()
	if v.Draft != draft || v.LastNativeText != native || v.LastAcknowledgedText != ack || v.Dirty != dirty || v.InFlight != flight {
		t.Fatalf("text state: draft=%q native=%q ack=%q dirty=%v flight=%v; want %q %q %q %v %v", v.Draft, v.LastNativeText, v.LastAcknowledgedText, v.Dirty, v.InFlight, draft, native, ack, dirty, flight)
	}
}
func status(t *testing.T, u titlesession.Update, want titlesession.LifecycleStatus) {
	t.Helper()
	if u.State.Lifecycle.Status != want || u.State.Lifecycle.SnapshotIsFlushed() != (want == titlesession.LifecycleFlushClean) {
		t.Fatalf("lifecycle=%+v, want %v", u.State.Lifecycle, want)
	}
}
func assertRejectedUnchanged(t *testing.T, s *titlesession.Session, f func() titlesession.Update) {
	t.Helper()
	before, beforeJob := s.State(), s.ScheduledJob()
	u := f()
	if u.Accepted || u.Err == nil || !reflect.DeepEqual(before, s.State()) || !reflect.DeepEqual(beforeJob, s.ScheduledJob()) {
		t.Fatalf("rejection changed state: update=%+v before=%+v after=%+v", u, before, s.State())
	}
}

func TestOrdinaryDebounceKeepsRawUntilExplicitFlush(t *testing.T) {
	s := session(t, "old")
	j := job(t, native(s, "  new title  ", false), 700)
	texts(t, s, "  new title  ", "  new title  ", "old", true, false)
	// Repeated frame observation must not cancel or postpone a pending deadline.
	u := native(s, "  new title  ", false)
	if u.Job == nil || *u.Job != j {
		t.Fatal("unchanged snapshot replaced the deadline")
	}
	r := start(t, s, j, "  new title  ", "new title")
	u = succeed(s, r, "new title")
	texts(t, s, "  new title  ", "  new title  ", "new title", false, false)
	if u.Job != nil || u.DraftReplacement.Present || !u.State.PendingPersistedText.Present {
		t.Fatalf("ordinary success canonicalized: %+v", u)
	}
	// Source preserves raw text after an ordinary save, even a clean later flush.
	u = s.RequestFlush(s.Identity())
	if u.Job != nil {
		t.Fatal("clean flush created work")
	}
	texts(t, s, "  new title  ", "  new title  ", "new title", false, false)
	status(t, u, titlesession.LifecycleNotRequested)
}

func TestExplicitFlushCanonicalizesAndRepeatedFlushHasOneWrite(t *testing.T) {
	s := session(t, "old")
	old := job(t, native(s, "  new\r\nline \t", false), 700)
	first := job(t, s.RequestFlush(s.Identity()), 0)
	second := job(t, s.RequestFlush(s.Identity()), 0)
	for _, stale := range []titlesession.Job{old, first} {
		assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(stale).Update })
	}
	r := start(t, s, second, "  new\r\nline \t", "new\nline")
	if s.RequestFlush(s.Identity()).Job != nil {
		t.Fatal("flush while saving created second write")
	}
	texts(t, s, "  new\r\nline \t", "  new\r\nline \t", "old", true, true)
	u := succeed(s, r, "new\nline")
	texts(t, s, "new\nline", "new\nline", "new\nline", false, false)
	if !u.DraftReplacement.Present || u.DraftReplacement.Text != "new\nline" {
		t.Fatal("missing canonical replacement")
	}
	assertRejectedUnchanged(t, s, func() titlesession.Update { return succeed(s, r, "new\nline") })
}

func TestLifecycleSavesNewerInputWithoutOldCompletionOverwrite(t *testing.T) {
	s := session(t, "old")
	j := job(t, lifecycle(s, " A ", false, true), 0)
	r1 := start(t, s, j, " A ", "A")
	status(t, s.RequestFlush(s.Identity()), titlesession.LifecycleWaitingPersistence)
	j2 := job(t, native(s, " B ", false), 0)
	status(t, s.BeginPersistence(j2).Update, titlesession.LifecycleNeedsNativeFinalization)
	texts(t, s, " B ", " B ", "old", true, true)
	u := succeed(s, r1, "A")
	texts(t, s, " B ", " B ", "A", true, false)
	if u.DraftReplacement.Present || !u.State.CanonicalizesAfterSuccess {
		t.Fatal("old completion lost newer lifecycle draft")
	}
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	r2 := start(t, s, job(t, u, 0), " B ", "B")
	u = succeed(s, r2, "B")
	texts(t, s, "B", "B", "B", false, false)
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	// Completion cannot manufacture finalization after intervening native input.
	u = lifecycle(s, "B", false, true)
	status(t, u, titlesession.LifecycleFlushClean)
	if u.Job != nil {
		t.Fatal("latest finalized clean state must not write again")
	}
}

func TestOrdinarySaveNewerTypingPreservesRawAndReschedules700(t *testing.T) {
	s := session(t, "old")
	r1 := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	jDuring := job(t, native(s, " B ", false), 700)
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(jDuring).Update })
	u := succeed(s, r1, "A")
	texts(t, s, " B ", " B ", "A", true, false)
	jAfter := job(t, u, 700)
	if jDuring == jAfter {
		t.Fatal("completion must issue newer candidate")
	}
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(jDuring).Update })
	r2 := start(t, s, jAfter, " B ", "B")
	succeed(s, r2, "B")
	texts(t, s, " B ", " B ", "B", false, false)
}

func TestErrorPreservesTextAndRetriesExactBackoff(t *testing.T) {
	s := session(t, "old")
	j := job(t, native(s, " raw new ", false), 700)
	failure := errors.New("synthetic disk failure")
	for _, delay := range []uint64{1000, 2000, 4000, 8000, 10000, 10000} {
		r := start(t, s, j, " raw new ", "raw new")
		u := s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: failure})
		texts(t, s, " raw new ", " raw new ", "old", true, false)
		if u.Err != failure || u.State.LastError != failure || u.DraftReplacement.Present {
			t.Fatal("failure did not preserve draft")
		}
		j = job(t, u, delay)
	}
	r := start(t, s, j, " raw new ", "raw new")
	u := succeed(s, r, "raw new")
	if u.State.LastError != nil || u.Job != nil {
		t.Fatal("successful retry did not clear error")
	}
	texts(t, s, " raw new ", " raw new ", "raw new", false, false)
}

func TestNewInputAfterFailureResetsRetry(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	failed := job(t, s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("failure")}), 1000)
	j := job(t, native(s, "B", false), 700)
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(failed).Update })
	r = start(t, s, j, "B", "B")
	job(t, s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("failure")}), 1000)
	texts(t, s, "B", "B", "old", true, false)
}

func TestOrdinaryBlankRemainsLocalAndLifecycleRestoresAck(t *testing.T) {
	s := session(t, "old")
	oldID := s.Identity()
	j := job(t, native(s, " \r\n\t ", false), 700)
	b := s.BeginPersistence(j)
	if !b.Update.Accepted || b.Request != nil || b.Update.Job != nil || b.Update.DraftReplacement.Present {
		t.Fatal("ordinary blank wrote or restored")
	}
	texts(t, s, " \r\n\t ", " \r\n\t ", "old", true, false)
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(j).Update })
	j = job(t, lifecycle(s, " \r\n\t ", false, true), 0)
	b = s.BeginPersistence(j)
	if b.Request != nil || !b.Update.DraftReplacement.Present || b.Update.DraftReplacement.Text != "old" {
		t.Fatal("blank lifecycle did not restore")
	}
	texts(t, s, "old", "old", "old", false, false)
	status(t, b.Update, titlesession.LifecycleFlushClean)
	if s.Identity().Generation() != oldID.Generation()+1 {
		t.Fatal("discard failed to invalidate generation")
	}
	assertRejectedUnchanged(t, s, func() titlesession.Update {
		return s.NativeSnapshotDidChange(oldID, titlesession.NativeSnapshot{Text: "late"})
	})
}

func TestBlankLifecycleWaitsForOlderSaveSuccess(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	u := lifecycle(s, "  ", false, true)
	status(t, u, titlesession.LifecycleWaitingPersistence)
	if u.Job != nil {
		t.Fatal("blank must wait for old write")
	}
	texts(t, s, "  ", "  ", "old", true, true)
	u = succeed(s, r, "A")
	texts(t, s, "  ", "  ", "A", true, false)
	b := s.BeginPersistence(job(t, u, 0))
	if b.Request != nil || b.Update.DraftReplacement.Text != "A" {
		t.Fatal("restored stale acknowledged title")
	}
	texts(t, s, "A", "A", "A", false, false)
	status(t, b.Update, titlesession.LifecycleFlushClean)
}

func TestBlankAfterOlderFailureStaysUntilNewExplicitLifecycle(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	lifecycle(s, "  ", false, true)
	failure := errors.New("synthetic failed A")
	u := s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: failure})
	status(t, u, titlesession.LifecycleFailed)
	texts(t, s, "  ", "  ", "old", true, false)
	b := s.BeginPersistence(job(t, u, 1000))
	if b.Request != nil || b.Update.DraftReplacement.Present {
		t.Fatal("automatic blank retry discarded error draft")
	}
	texts(t, s, "  ", "  ", "old", true, false)
	status(t, b.Update, titlesession.LifecycleFailed)
	b = s.BeginPersistence(job(t, lifecycle(s, "  ", false, true), 0))
	texts(t, s, "old", "old", "old", false, false)
	status(t, b.Update, titlesession.LifecycleFlushClean)
}

func TestFailureCannotSilentlyReviveCancelledClose(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, lifecycle(s, " A ", false, true), 0), " A ", "A")
	u := s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("fail")})
	status(t, u, titlesession.LifecycleFailed)
	r = start(t, s, job(t, u, 1000), " A ", "A")
	u = succeed(s, r, "A")
	texts(t, s, "A", "A", "A", false, false)
	status(t, u, titlesession.LifecycleFailed)
	status(t, lifecycle(s, "A", false, true), titlesession.LifecycleFlushClean)
}

func TestCompositionNeverWritesOrClaimsFinalization(t *testing.T) {
	s := session(t, "old")
	stale := job(t, native(s, "committed", false), 700)
	u := native(s, "committed", true)
	if u.Job != nil {
		t.Fatal("composition scheduled persistence")
	}
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(stale).Update })
	u = lifecycle(s, "committed", true, true)
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	if u.Job != nil {
		t.Fatal("finalized flag bypassed composing")
	}
	u = native(s, "committed final", false)
	j := job(t, u, 700)
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	r := start(t, s, j, "committed final", "committed final")
	u = succeed(s, r, "committed final")
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	status(t, lifecycle(s, "committed final", false, true), titlesession.LifecycleFlushClean)
}

func TestUnverifiedLifecycleNeverRestoresBlank(t *testing.T) {
	s := session(t, "old")
	u := lifecycle(s, " ", false, false)
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
	if u.Job != nil {
		t.Fatal("unverified finalization started lifecycle flush")
	}
	j := job(t, native(s, "  ", false), 700)
	b := s.BeginPersistence(j)
	if b.Request != nil || b.Update.DraftReplacement.Present {
		t.Fatal("unverified lifecycle restored blank")
	}
	texts(t, s, "  ", "  ", "old", true, false)
	status(t, b.Update, titlesession.LifecycleNeedsNativeFinalization)
}

func TestOlderCompletionDuringCompositionNeverReplacesNativeDraft(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, lifecycle(s, " A ", false, true), 0), " A ", "A")
	native(s, " A ", true)
	u := succeed(s, r, "A")
	texts(t, s, " A ", " A ", "A", false, false)
	if u.DraftReplacement.Present || !u.State.Composing {
		t.Fatal("success edited ongoing composition")
	}
	status(t, u, titlesession.LifecycleNeedsNativeFinalization)
}

func TestReturnIsNewlineAndNormalizationDoesNotNFCBytes(t *testing.T) {
	for _, tc := range []struct {
		raw, normalized string
		nonempty        bool
	}{
		{" \r\nline one\rline two\r\nline three \t", "line one\nline two\nline three", true},
		{"\u00a0\u2003cafe\u0301\u3000", "cafe\u0301", true},
		{"\t\r\n\r\u0085\u2003", "", false},
		{"\u200btitle\u200b", "\u200btitle\u200b", true},
	} {
		got, nonempty := titlesession.NormalizeTitle(tc.raw)
		if got != tc.normalized || nonempty != tc.nonempty {
			t.Fatalf("normalize %q = %q/%v", tc.raw, got, nonempty)
		}
	}
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "line one\nline two", false), 700), "line one\nline two", "line one\nline two")
	succeed(s, r, "line one\nline two")
	texts(t, s, "line one\nline two", "line one\nline two", "line one\nline two", false, false)
}

func TestCanonicalEquivalentSnapshotPreservesBytesWithoutRevision(t *testing.T) {
	s := session(t, "caf\u00e9")
	u := native(s, "cafe\u0301", false)
	if u.Job != nil || u.State.Dirty {
		t.Fatal("canonical equality became new draft revision")
	}
	texts(t, s, "cafe\u0301", "cafe\u0301", "caf\u00e9", false, false)
	u = s.SynchronizePersistedText(s.Identity(), "caf\u00e9")
	if u.DraftReplacement.Present {
		t.Fatal("canonical external equality rewrote bytes")
	}
	texts(t, s, "cafe\u0301", "cafe\u0301", "caf\u00e9", false, false)
}

func TestSameNormalizedStillRequiresServiceAndActualAckBytes(t *testing.T) {
	s := session(t, "caf\u00e9")
	r := start(t, s, job(t, lifecycle(s, " cafe\u0301 ", false, true), 0), " cafe\u0301 ", "cafe\u0301")
	texts(t, s, " cafe\u0301 ", " cafe\u0301 ", "caf\u00e9", true, true)
	u := succeed(s, r, "caf\u00e9")
	texts(t, s, "caf\u00e9", "caf\u00e9", "caf\u00e9", false, false)
	if u.State.PendingPersistedText.Text != "caf\u00e9" || u.DraftReplacement.Text != "caf\u00e9" {
		t.Fatal("invented input bytes instead of actual canonical no-op bytes")
	}
	status(t, u, titlesession.LifecycleFlushClean)
}

func TestUnverifiedSuccessAndMismatchedReadCannotAcknowledge(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, lifecycle(s, " new ", false, true), 0), " new ", "new")
	u := s.PersistenceDidComplete(r, titlesession.PersistenceResult{})
	texts(t, s, " new ", " new ", "old", true, true)
	if !u.State.AwaitingVerification {
		t.Fatal("unverified service success was acknowledged")
	}
	status(t, u, titlesession.LifecycleWaitingPersistence)
	// External publication is not a substitute for this request's verified result.
	s.SynchronizePersistedText(s.Identity(), "new")
	texts(t, s, " new ", " new ", "old", true, true)
	u = succeed(s, r, "unrelated later title")
	if u.Err != titlesession.ErrVerificationMismatch || !u.State.AwaitingVerification {
		t.Fatal("unrelated result accepted")
	}
	status(t, u, titlesession.LifecycleFailed)
	texts(t, s, " new ", " new ", "old", true, true)
	u = succeed(s, r, "new")
	texts(t, s, "new", "new", "new", false, false)
	status(t, u, titlesession.LifecycleFlushClean)
}

func TestPendingPublicationCannotOverwriteNewDraft(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	native(s, "B", false)
	succeed(s, r, "A")
	u := s.SynchronizePersistedText(s.Identity(), "A")
	texts(t, s, "B", "B", "A", true, false)
	if u.State.PendingPersistedText.Present || u.DraftReplacement.Present {
		t.Fatal("pending publication overwrote newer draft")
	}
	u = s.SynchronizePersistedText(s.Identity(), "external")
	texts(t, s, "B", "B", "A", true, false)
	if u.DraftReplacement.Present {
		t.Fatal("external update overwrote dirty draft")
	}
}

func TestExternalUpdateOnlyAppliesWhenSafe(t *testing.T) {
	s := session(t, "old")
	u := s.SynchronizePersistedText(s.Identity(), "external one")
	texts(t, s, "external one", "external one", "external one", false, false)
	if !u.DraftReplacement.Present || u.DraftReplacement.Text != "external one" {
		t.Fatal("clean external sync missing")
	}
	native(s, "external one", true)
	s.SynchronizePersistedText(s.Identity(), "external two")
	texts(t, s, "external one", "external one", "external one", false, false)
	if !s.State().Composing {
		t.Fatal("sync ended composition")
	}
	native(s, "external one", false)
	s.SynchronizePersistedText(s.Identity(), "external two")
	texts(t, s, "external two", "external two", "external two", false, false)
	r := start(t, s, job(t, native(s, "own draft", false), 700), "own draft", "own draft")
	s.SynchronizePersistedText(s.Identity(), "external three")
	texts(t, s, "own draft", "own draft", "external two", true, true)
	succeed(s, r, "own draft")
	s.SynchronizePersistedText(s.Identity(), "own draft")
	s.SynchronizePersistedText(s.Identity(), "external four")
	texts(t, s, "external four", "external four", "external four", false, false)
}

func TestNativeDifferentTextClearsPendingPublication(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	succeed(s, r, "A")
	if !s.State().PendingPersistedText.Present {
		t.Fatal("missing marker")
	}
	native(s, "B", false)
	if s.State().PendingPersistedText.Present {
		t.Fatal("different native text retained pending marker")
	}
}

func TestSessionIdentityRejectsOtherOwnerGenerationAndReusedNumbers(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	for _, cfg := range []titlesession.Config{
		{OwnerID: "task:other:title", SessionID: 17, Generation: 29, TraceID: "synthetic-trace-1", PersistedText: "old"},
		{OwnerID: "task:synthetic:title", SessionID: 17, Generation: 30, TraceID: "synthetic-trace-1", PersistedText: "old"},
		{OwnerID: "task:synthetic:title", SessionID: 18, Generation: 29, TraceID: "synthetic-trace-1", PersistedText: "old"},
		{OwnerID: "task:synthetic:title", SessionID: 17, Generation: 29, TraceID: "synthetic-trace-1", PersistedText: "old"},
	} {
		other, err := titlesession.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		otherJob := job(t, native(other, "A", false), 700)
		otherRequest := other.BeginPersistence(otherJob).Request
		if otherRequest == nil {
			t.Fatal("other did not start")
		}
		assertRejectedUnchanged(t, s, func() titlesession.Update {
			return s.NativeSnapshotDidChange(other.Identity(), titlesession.NativeSnapshot{Text: "malformed"})
		})
		assertRejectedUnchanged(t, s, func() titlesession.Update { return s.BeginPersistence(otherJob).Update })
		assertRejectedUnchanged(t, s, func() titlesession.Update { return succeed(s, *otherRequest, "A") })
	}
	succeed(s, r, "A")
	texts(t, s, "A", "A", "A", false, false)
}

func TestImmutableReturnedTokenCopiesAndStaleCompletion(t *testing.T) {
	s := session(t, "old")
	j := job(t, native(s, "A", false), 700)
	copiedGate := j.GateSchedule()
	copiedGate.OwnerID, copiedGate.Generation, copiedGate.Sequence = "forged", 0, 0
	r := start(t, s, j, "A", "A")
	copiedRequest := r.GateRequest()
	copiedRequest.OwnerID, copiedRequest.RequestID = "forged", 0
	succeed(s, r, "A")
	r2 := start(t, s, job(t, native(s, "B", false), 700), "B", "B")
	assertRejectedUnchanged(t, s, func() titlesession.Update { return succeed(s, r, "A") })
	assertRejectedUnchanged(t, s, func() titlesession.Update {
		return s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("stale")})
	})
	texts(t, s, "B", "B", "A", true, true)
	succeed(s, r2, "B")
	texts(t, s, "B", "B", "B", false, false)
}

func TestDisposeRequiresLifecycleFlushCleanAndRejectsAllLateEvents(t *testing.T) {
	s := session(t, "old")
	id := s.Identity()
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.Dispose(id) })
	r := start(t, s, job(t, lifecycle(s, "new", false, true), 0), "new", "new")
	assertRejectedUnchanged(t, s, func() titlesession.Update { return s.Dispose(id) })
	succeed(s, r, "new")
	u := s.Dispose(id)
	status(t, u, titlesession.LifecycleDisposed)
	if !u.Accepted || !u.State.Disposed || u.Job != nil {
		t.Fatal("dispose failed")
	}
	for _, fn := range []func() titlesession.Update{
		func() titlesession.Update {
			return s.NativeSnapshotDidChange(id, titlesession.NativeSnapshot{Text: "late"})
		},
		func() titlesession.Update { return s.RequestFlush(id) },
		func() titlesession.Update {
			return s.RequestLifecycleFlush(id, titlesession.NativeSnapshot{Text: "late"}, true)
		},
		func() titlesession.Update { return s.SynchronizePersistedText(id, "late") },
		func() titlesession.Update { return succeed(s, r, "new") },
		func() titlesession.Update { return s.Dispose(id) },
	} {
		assertRejectedUnchanged(t, s, fn)
	}
	other := session(t, "old")
	assertRejectedUnchanged(t, other, func() titlesession.Update { return succeed(other, r, "new") })
}

func TestInvalidConfigAndZeroReceiverFailClosed(t *testing.T) {
	valid := titlesession.Config{OwnerID: "owner", SessionID: 1, Generation: 1, TraceID: "trace"}
	for _, field := range []string{"owner", "session", "generation", "trace"} {
		cfg := valid
		switch field {
		case "owner":
			cfg.OwnerID = ""
		case "session":
			cfg.SessionID = 0
		case "generation":
			cfg.Generation = 0
		case "trace":
			cfg.TraceID = ""
		}
		if s, err := titlesession.New(cfg); err == nil || s != nil {
			t.Fatalf("accepted invalid %s", field)
		}
	}
	valid.OwnerID, valid.TraceID = " \t", " \t"
	if s, err := titlesession.New(valid); err != nil || s.Identity().OwnerID() != " \t" {
		t.Fatal("constructor trimmed identities")
	}
	for _, s := range []*titlesession.Session{nil, new(titlesession.Session)} {
		if s.State().Lifecycle.SnapshotIsFlushed() || s.ScheduledJob() != nil {
			t.Fatal("zero session exposed usable state")
		}
		for _, fn := range []func() titlesession.Update{
			func() titlesession.Update {
				return s.NativeSnapshotDidChange(s.Identity(), titlesession.NativeSnapshot{})
			},
			func() titlesession.Update { return s.RequestFlush(s.Identity()) },
			func() titlesession.Update {
				return s.RequestLifecycleFlush(s.Identity(), titlesession.NativeSnapshot{}, true)
			},
			func() titlesession.Update { return s.BeginPersistence(titlesession.Job{}).Update },
			func() titlesession.Update {
				return s.PersistenceDidComplete(titlesession.SaveRequest{}, titlesession.PersistenceResult{})
			},
			func() titlesession.Update { return s.SynchronizePersistedText(s.Identity(), "text") },
			func() titlesession.Update { return s.Dispose(s.Identity()) },
		} {
			assertRejectedUnchanged(t, s, fn)
		}
	}
}

func TestInvalidMixedCompletionDoesNotReleaseRequest(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	assertRejectedUnchanged(t, s, func() titlesession.Update {
		return s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("failure"), VerifiedText: titlesession.OptionalText{Text: "A", Present: true}})
	})
	succeed(s, r, "A")
	texts(t, s, "A", "A", "A", false, false)
}

func TestLifecycleWaitingChangesAndUnverifiedRetry(t *testing.T) {
	s := session(t, "old")
	u := lifecycle(s, "new", false, true)
	status(t, u, titlesession.LifecycleWaitingChanges)
	j := job(t, u, 0)
	if j.Identity() != s.Identity() {
		t.Fatal("job lost identity")
	}
	r := start(t, s, j, "new", "new")
	u = s.PersistenceDidComplete(r, titlesession.PersistenceResult{})
	status(t, u, titlesession.LifecycleWaitingPersistence)
	if s.RequestFlush(s.Identity()).Job != nil {
		t.Fatal("unverified success allowed a second request")
	}
	failure := errors.New("unable to verify acknowledged bytes")
	u = s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: failure})
	texts(t, s, "new", "new", "old", true, false)
	status(t, u, titlesession.LifecycleFailed)
	job(t, u, 1000)
}

func TestCanonicalAckEchoConsumesMarkerWithoutChangingNewerNative(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "caf\u00e9", false), 700), "caf\u00e9", "caf\u00e9")
	native(s, "later", false)
	succeed(s, r, "caf\u00e9")
	u := s.SynchronizePersistedText(s.Identity(), "cafe\u0301")
	texts(t, s, "later", "later", "cafe\u0301", true, false)
	if u.State.PendingPersistedText.Present || u.DraftReplacement.Present {
		t.Fatal("canonical ack echo changed newer text")
	}
}

// 有界合成事件序列额外检查安全不变量，不冒充 Swift 动态 oracle。
// 每轮只有串行调用；race 通过不表示 Session 支持并发访问。
func TestBoundedSerialSequencesKeepTextAndLifecycleInvariants(t *testing.T) {
	const sequences, steps = 96, 160
	var seed uint64 = 0x676174655f746578
	next := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}
	values := []string{"old", "A", " B ", "", " \r\n ", "first\nsecond", "caf\u00e9", "cafe\u0301"}
	for sequence := 0; sequence < sequences; sequence++ {
		s := session(t, "old")
		var outstanding *titlesession.SaveRequest
		var stale *titlesession.SaveRequest
		for step := 0; step < steps; step++ {
			prior := s.State()
			switch next() % 9 {
			case 0, 1:
				native(s, values[next()%uint64(len(values))], next()%4 == 0)
			case 2:
				s.RequestFlush(s.Identity())
			case 3:
				lifecycle(s, prior.Draft, prior.Composing, next()%2 == 0)
			case 4:
				if j := s.ScheduledJob(); j != nil {
					b := s.BeginPersistence(*j)
					if b.Request != nil {
						if outstanding != nil || prior.InFlight || prior.Composing {
							t.Fatal("overlapping or composing save")
						}
						if b.Request.RawDraft() != prior.Draft {
							t.Fatal("request did not capture current raw")
						}
						normalized, nonempty := titlesession.NormalizeTitle(prior.Draft)
						if !nonempty || b.Request.NormalizedText() != normalized {
							t.Fatal("request violated nonempty normalized boundary")
						}
						outstanding = b.Request
					}
				}
			case 5:
				if outstanding != nil {
					r := *outstanding
					var result titlesession.PersistenceResult
					outcome := next() % 4
					switch outcome {
					case 0:
						result.Err = errors.New("synthetic failure")
					case 1:
						result.VerifiedText = titlesession.OptionalText{Text: r.NormalizedText(), Present: true}
					case 2: // successful call, acknowledged bytes still unavailable
					case 3:
						result.VerifiedText = titlesession.OptionalText{Text: "unrelated result", Present: true}
					}
					u := s.PersistenceDidComplete(r, result)
					if !u.Accepted {
						t.Fatal("lost active request identity")
					}
					if outcome == 0 || outcome == 2 || outcome == 3 {
						if u.State.Draft != prior.Draft || u.State.LastNativeText != prior.LastNativeText || u.State.LastAcknowledgedText != prior.LastAcknowledgedText || u.DraftReplacement.Present {
							t.Fatal("failure or unverified result changed text")
						}
					}
					if !u.State.InFlight {
						stale, outstanding = outstanding, nil
					}
				}
			case 6:
				s.SynchronizePersistedText(s.Identity(), values[next()%uint64(len(values))])
				current := s.State()
				if prior.Dirty || prior.Composing || prior.InFlight {
					if current.Draft != prior.Draft || current.LastNativeText != prior.LastNativeText {
						t.Fatal("external sync overwrote editing text")
					}
				}
			case 7:
				if stale != nil {
					assertRejectedUnchanged(t, s, func() titlesession.Update { return succeed(s, *stale, stale.NormalizedText()) })
				}
			case 8:
				// Repeating the same observed snapshot is not another edit or timer reset.
				beforeJob := s.ScheduledJob()
				native(s, prior.Draft, prior.Composing)
				if !reflect.DeepEqual(beforeJob, s.ScheduledJob()) {
					t.Fatal("identical observation changed candidate")
				}
			}
			current := s.State()
			if current.InFlight != (outstanding != nil) {
				t.Fatal("request handle and Gate flight state diverged")
			}
			if current.Dirty || current.InFlight || current.Composing || current.AwaitingVerification || current.LastError != nil || s.ScheduledJob() != nil {
				if current.Lifecycle.SnapshotIsFlushed() {
					t.Fatalf("unsafe flushed verdict at %d/%d: %+v", sequence, step, current)
				}
			}
		}
	}
}

func TestRejectedLifecycleCannotReportOperationFlushed(t *testing.T) {
	s := session(t, "old")
	clean := lifecycle(s, "old", false, true)
	if !clean.SnapshotIsFlushed() {
		t.Fatal("accepted clean lifecycle did not report known snapshot flushed")
	}
	other := session(t, "old")
	u := s.RequestLifecycleFlush(other.Identity(), titlesession.NativeSnapshot{Text: "lost text"}, true)
	if u.Accepted || u.Err != titlesession.ErrStaleEvent || u.SnapshotIsFlushed() {
		t.Fatal("rejected lifecycle used prior clean state as success")
	}
	if !s.State().Lifecycle.SnapshotIsFlushed() {
		t.Fatal("rejected event changed prior state")
	}
	u = lifecycle(s, "new", false, true)
	if u.SnapshotIsFlushed() {
		t.Fatal("pending candidate reported flushed")
	}
	r := start(t, s, job(t, u, 0), "new", "new")
	u = s.PersistenceDidComplete(r, titlesession.PersistenceResult{Err: errors.New("failure")})
	if u.SnapshotIsFlushed() {
		t.Fatal("error completion reported flushed")
	}
}
