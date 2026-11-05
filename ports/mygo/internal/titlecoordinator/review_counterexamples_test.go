// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
	"time"
)

// No goroutines, frame, nested loop, or Native reentry: a Changed hook opens
// another mounted editor synchronously using the public API.
func TestReviewReleaseClosedChecksRegistrationsOpenedByChangedHook(t *testing.T) {
	f := setup(t)
	f.open(t, "w")
	if !f.c.BeforeClose("w") {
		t.Fatal("setup")
	}
	var fresh Registration
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		n := &native{s: titlesession.NativeSnapshot{Text: "unsaved late editor"}}
		var err error
		fresh, err = f.c.Open("w", "late-owner", "late-trace", "persisted", n)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := f.c.ReleaseClosed("w")
	if err == nil {
		state, present := f.c.State(fresh)
		t.Fatalf("ReleaseClosed allowed native destruction with unchecked registration: present=%v state=%+v", present, state)
	}
}

func TestReviewBeforeQuitChecksRegistrationsOpenedByChangedHook(t *testing.T) {
	f := setup(t)
	f.open(t, "one")
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		_, err := f.c.Open("two", "late-owner", "late-trace", "persisted", &native{s: titlesession.NativeSnapshot{Text: "unsaved second window"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.c.BeforeQuit() {
		t.Fatal("BeforeQuit permitted quit without finalizing newly registered second window")
	}
}

func TestReviewReleaseClosedLateDraftSchedulesProgress(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	if !f.c.BeforeClose("w") {
		t.Fatal("setup")
	}
	n.s.Text = "late input before final disposal proof"
	if f.c.ReleaseClosed("w") == nil {
		t.Fatal("unsafe release")
	}
	f.advance(time.Hour)
	if len(f.l.q) == 0 {
		s, _ := f.c.State(r)
		t.Fatalf("release safely rejected but fresh draft has zero-delay job with no timer, write, or intent: state=%+v", s)
	}
}

func TestReviewCleanHistoricalTraceCanClose(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	// Verified projection says today's unchanged pending editor became historical.
	if err := f.c.LockTrace(r); err != nil {
		t.Fatal(err)
	}
	s, _ := f.c.State(r)
	if s.Dirty || s.InFlight || n.s.Text != "旧标题" {
		t.Fatal("setup was not clean", s)
	}
	if !f.c.BeforeClose("w") {
		t.Fatalf("clean, fully acknowledged historical editor permanently blocks close: state=%+v errors=%v", s, f.errs)
	}
}
