// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
	"time"
)

func TestRegistryMutationHasBoundedContinuation(t *testing.T) {
	f := setup(t)
	f.open(t, "one")
	mutations := 0
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		mutations++
		_, err := f.c.Open("two", fmt.Sprint(mutations), fmt.Sprint(mutations), "old", &native{s: titlesession.NativeSnapshot{Text: "new"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.c.BeforeQuit() {
		t.Fatal("unchecked new editor")
	}
	if len(f.post) != 1 {
		t.Fatal("no explicit continuation")
	}
	f.ui()
	if mutations != 2 || f.c.hasIntents() || len(f.post) != 0 || f.quit != 0 {
		t.Fatalf("unbounded/revived intent: %d", mutations)
	}
	if !errors.Is(f.errs[len(f.errs)-1], ErrRegistry) {
		t.Fatal(f.errs)
	}
}
func TestEqualCountReplacementInvalidatesRegistrationProof(t *testing.T) {
	f := setup(t)
	f.open(t, "one")
	old, _ := f.open(t, "other")
	before := len(f.c.entries)
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		if err := f.c.ReleaseClosed("other"); err != nil {
			t.Fatal(err)
		}
		f.open(t, "other")
	}
	if f.c.BeforeClose("one") {
		t.Fatal("equal count hid replaced registration")
	}
	if len(f.c.entries) != before {
		t.Fatal("test needs same count")
	}
	if _, ok := f.c.State(old); ok {
		t.Fatal("old not removed")
	}
	f.ui()
	if f.close != 1 {
		t.Fatal("safe recheck did not request close")
	}
}
func TestNativeFailureLockAndReleaseDoNotReviveClose(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.invalid = true
	if err := f.c.LockTrace(r); err != ErrNative {
		t.Fatal("native failure hidden", err)
	}
	if f.c.ReleaseClosed("w") == nil {
		t.Fatal("unsafe release")
	}
	f.ui()
	f.advance(time.Hour)
	if f.c.hasIntents() || f.close != 0 || len(f.l.q) != 0 {
		t.Fatal("native failure looping")
	}
}
func TestCleanLockedTraceCanDisposeIntoReadonly(t *testing.T) {
	f := setup(t)
	r, _ := f.open(t, "w")
	if err := f.c.LockTrace(r); err != nil {
		t.Fatal(err)
	}
	f.c.RequestNavigation("w", "readonly")
	if _, ok := f.c.State(r); ok {
		t.Fatal("clean historical editor stuck")
	}
	if len(f.nav) != 1 || len(f.l.q) != 0 {
		t.Fatal("wrong transition")
	}
}
func TestDirtyLockedTraceNeverPersistsOrDisposes(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "unsaved historical"
	if err := f.c.LockTrace(r); err != nil {
		t.Fatal(err)
	}
	f.c.RequestNavigation("w", "readonly")
	f.advance(time.Hour)
	s, ok := f.c.State(r)
	if !ok || s.Draft != "unsaved historical" || !s.Dirty || len(f.nav) != 0 || len(f.l.q) != 0 {
		t.Fatal(s)
	}
}
func TestReleaseLateDraftReentersActualCloseAfterSave(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	if !f.c.BeforeClose("w") {
		t.Fatal("setup")
	}
	closed := 0
	f.c.cfg.ReenterClose = func(w string) {
		if f.c.BeforeClose(w) {
			if err := f.c.ReleaseClosed(w); err != nil {
				t.Fatal(err)
			}
			closed++
		}
	}
	n.s.Text = "late"
	if f.c.ReleaseClosed("w") == nil {
		t.Fatal("unsafe release")
	}
	f.finishSave(t)
	if closed != 1 {
		t.Fatal("did not actually reenter")
	}
	if _, ok := f.c.State(r); ok {
		t.Fatal("not disposed")
	}
}
func TestSyncOrderedPublicationUpdatesOtherCleanWindowWithoutWrite(t *testing.T) {
	f := setup(t)
	a, n := f.open(t, "a")
	b, m := f.open(t, "b")
	n.s.Text = "published"
	f.observe(t, a)
	f.advance(700 * time.Millisecond)
	f.finishSave(t)
	id, _ := f.c.Identity(b)
	if err := f.c.SynchronizePersistedText(b, id, "published"); err != nil {
		t.Fatal(err)
	}
	s, _ := f.c.State(b)
	if s.Draft != "published" || s.LastAcknowledgedText != "published" || m.s.Text != "published" || s.Dirty {
		t.Fatal(s)
	}
	f.advance(time.Hour)
	if len(f.l.q) != 0 {
		t.Fatal("external publish created redundant write")
	}
}
func TestSyncSameValueEchoKeepsNewDraft(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.advance(700 * time.Millisecond)
	f.finishSave(t)
	id, _ := f.c.Identity(r)
	if err := f.c.SynchronizePersistedText(r, id, "A"); err != nil {
		t.Fatal(err)
	}
	s, _ := f.c.State(r)
	if s.PendingPersistedText.Present || s.Dirty {
		t.Fatal(s)
	}
	n.s.Text = "native B" // not yet built/observed
	if err := f.c.SynchronizePersistedText(r, id, "A"); err != nil {
		t.Fatal(err)
	}
	s, _ = f.c.State(r)
	if s.Draft != "native B" || !s.Dirty || n.s.Text != "native B" {
		t.Fatal(s)
	}
}
func TestSyncProtectsDirtyComposingInflightAndNativeGap(t *testing.T) {
	for _, mode := range []string{"dirty", "composition", "inflight", "native gap"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "w")
			switch mode {
			case "dirty":
				n.s.Text = "local"
				f.observe(t, r)
			case "composition":
				n.s.Composing = true
				n.marked = "未定"
			case "inflight":
				n.s.Text = "local"
				f.observe(t, r)
				f.advance(700 * time.Millisecond)
			case "native gap":
				n.s.Text = "local"
			}
			id, _ := f.c.Identity(r)
			if err := f.c.SynchronizePersistedText(r, id, "external"); err != nil {
				t.Fatal(err)
			}
			s, _ := f.c.State(r)
			if s.LastAcknowledgedText != "旧标题" || s.Draft == "external" || n.s.Text == "external" {
				t.Fatal(s)
			}
			if mode == "composition" && (!n.s.Composing || n.marked != "未定") {
				t.Fatal("preedit overwritten")
			}
		})
	}
}
func TestSyncRejectsStaleIdentityAndUnavailableNative(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	id, _ := f.c.Identity(r)
	n.s.Text = " "
	f.observe(t, r)
	f.c.EndEditing(r)
	if err := f.c.SynchronizePersistedText(r, id, "stale"); err != ErrStale {
		t.Fatal(err)
	}
	current, _ := f.c.Identity(r)
	n.invalid = true
	if err := f.c.SynchronizePersistedText(r, current, "external"); err != ErrNative {
		t.Fatal(err)
	}
	s, _ := f.c.State(r)
	if s.LastAcknowledgedText != "旧标题" {
		t.Fatal(s)
	}
}
func TestBeforeCloseCannotAcceptSupersededIntent(t *testing.T) {
	f := setup(t)
	f.open(t, "w")
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		if !f.c.BeforeQuit() {
			t.Fatal("nested clean quit")
		}
	}
	if f.c.BeforeClose("w") {
		t.Fatal("old close accepted after replacement by quit")
	}
}
func TestPumpHookNewRegistrationMustBeFinalized(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	var fresh Registration
	f.c.cfg.Changed = func(_ Registration, s titlesession.State) {
		if !s.InFlight {
			return
		}
		f.c.cfg.Changed = nil
		var err error
		fresh, err = f.c.Open("w", "new", "new", "old", &native{s: titlesession.NativeSnapshot{Text: "B"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.c.BeforeQuit() {
		t.Fatal("early quit")
	}
	f.ui()
	if len(f.l.q) != 2 {
		t.Fatal("new editor not captured in explicit recheck")
	}
	f.finishSave(t)
	if f.quit != 0 {
		t.Fatal("quit before second save")
	}
	f.finishSave(t)
	s, _ := f.c.State(fresh)
	if s.LastAcknowledgedText != "B" || f.quit != 1 {
		t.Fatal(s)
	}
}
