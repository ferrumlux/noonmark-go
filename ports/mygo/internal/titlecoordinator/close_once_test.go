// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

func TestOncePersistenceAndFreshRetry(t *testing.T) {
	for _, kind := range []string{"close", "quit", "release"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "one")
			n.s.Text = "A"
			attempt := func() bool {
				switch kind {
				case "quit":
					return f.c.BeforeQuitOnce()
				case "release":
					return f.c.ReleaseClosedOnce("one") == nil
				default:
					return f.c.BeforeCloseOnce("one")
				}
			}
			if attempt() || attempt() {
				t.Fatal("pending attempt allowed")
			}
			if f.c.hasIntents() {
				t.Fatal("once leaked intent")
			}
			e := f.c.find(r)
			if e == nil || e.flight == nil {
				t.Fatal("cancelled save")
			}
			f.finishSave(t)
			if f.quit != 0 || f.close != 0 || n.s.Text != "A" {
				t.Fatal("automatic exit or draft change")
			}
			if !attempt() {
				t.Fatal("fresh explicit retry refused")
			}
			if f.c.hasIntents() {
				t.Fatal("fresh retry left intent")
			}
			if kind == "release" && f.c.find(r) != nil {
				t.Fatal("fresh disposal omitted")
			}
		})
	}
}

func TestOnceSynchronousContinuationCannotNotify(t *testing.T) {
	for _, kind := range []string{"close", "quit", "release"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.open(t, "one")
			entered := false
			f.c.cfg.Changed = func(Registration, titlesession.State) {
				if entered {
					return
				}
				entered = true
				var i *intent
				if kind == "quit" {
					i = f.c.pending
				} else {
					i = f.c.intents["one"]
				}
				if i == nil || !i.once {
					t.Fatal("intent not once at creation")
				}
				f.c.continueOne(i)
			}
			switch kind {
			case "quit":
				f.c.BeforeQuitOnce()
			case "release":
				f.c.ReleaseClosedOnce("one")
			default:
				f.c.BeforeCloseOnce("one")
			}
			if !entered || f.close != 0 || f.quit != 0 || f.c.hasIntents() {
				t.Fatal("synchronous once notification/leak")
			}
		})
	}
}

func TestOnceCleanupPreservesReentrantReplacement(t *testing.T) {
	for _, quit := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "quit"}[quit], func(t *testing.T) {
			f := setup(t)
			_, n := f.open(t, "one")
			entered := false
			var replacement *intent
			f.c.cfg.Changed = func(Registration, titlesession.State) {
				if entered {
					return
				}
				entered = true
				n.s.Text = "A"
				if quit {
					f.c.BeforeQuit()
					replacement = f.c.pending
				} else {
					f.c.BeforeClose("one")
					replacement = f.c.intents["one"]
				}
			}
			if quit {
				f.c.BeforeQuitOnce()
			} else {
				f.c.BeforeCloseOnce("one")
			}
			if replacement == nil || !f.c.currentIntent(replacement) {
				t.Fatal("old cleanup deleted new intent")
			}
			f.finishSave(t)
			if f.close+f.quit != 1 {
				t.Fatal("replacement cannot complete")
			}
		})
	}
}

func TestOnceRegistryRecheckCannotResurrect(t *testing.T) {
	f := setup(t)
	f.open(t, "one")
	entered := false
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		if entered {
			return
		}
		entered = true
		f.open(t, "two")
	}
	if f.c.BeforeQuitOnce() {
		t.Fatal("registration mutation passed")
	}
	if len(f.post) == 0 {
		t.Fatal("no queued registry recheck")
	}
	f.ui()
	if f.c.hasIntents() || f.quit != 0 {
		t.Fatal("queued retry resurrected once")
	}
}

func TestCloseRetryPolicyRejectsOriginalIntent(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	original := new(int)
	current := original
	f.c.cfg.CloseRetryGuard = func(string) func() bool { captured := current; return func() bool { return current == captured } }
	n.s.Text = "A"
	f.observe(t, r)
	if f.c.BeforeClose("one") {
		t.Fatal("pending")
	}
	current = new(int)
	*current = 1
	f.finishSave(t)
	if f.close != 0 || f.c.hasIntents() {
		t.Fatal("old window closed replacement")
	}
	if !f.c.BeforeClose("one") {
		t.Fatal("fresh window proof failed")
	}
}

func TestCloseRetryPolicyReentryInvalidatesProof(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	var replacement *intent
	f.c.cfg.CloseRetryGuard = func(string) func() bool {
		return func() bool {
			// Defensive hostile hook: despite violating the pure predicate contract,
			// replacement of the exact intent must still prevent the old notification.
			replacement = &intent{kind: "close", window: "one", once: true}
			f.c.intents["one"] = replacement
			return true
		}
	}
	n.s.Text = "A"
	f.observe(t, r)
	f.c.BeforeClose("one")
	f.finishSave(t)
	if f.close != 0 || !f.c.currentIntent(replacement) {
		t.Fatal("stale policy result notified or removed new intent")
	}
}

func TestDisableQuitRetryPreservesSaving(t *testing.T) {
	f := setup(t)
	f.c.cfg.DisableQuitRetry = true
	r, n := f.open(t, "one")
	n.s.Text = "A"
	f.observe(t, r)
	if f.c.BeforeQuit() {
		t.Fatal("pending")
	}
	f.finishSave(t)
	if f.quit != 0 || f.c.pending != nil {
		t.Fatal("policy leaked notification/intent")
	}
	if !f.c.BeforeQuitOnce() || !f.c.BeforeClose("one") {
		t.Fatal("fresh explicit exit blocked")
	}
}

func TestOnceRealFailureAndAdmissionRejection(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "service", true: "admission"}[reject], func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "one")
			n.s.Text = "A"
			f.rejectSubmit = reject
			if !reject {
				f.s.saveErr = ErrLocked
			}
			if f.c.BeforeQuitOnce() {
				t.Fatal("failed save allowed quit")
			}
			if !reject {
				f.finishSave(t)
			}
			state, _ := f.c.State(r)
			if state.InFlight || !state.Dirty || state.LastError == nil || n.s.Text != "A" || f.c.hasIntents() || f.quit != 0 {
				t.Fatalf("failure or ticket lost: %+v", state)
			}
		})
	}
}

func TestCloseRetryPolicyRegistryMutationInvalidatesProof(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	entered := false
	f.c.cfg.CloseRetryGuard = func(string) func() bool {
		return func() bool {
			if !entered {
				entered = true
				f.open(t, "new")
			}
			return true
		}
	}
	n.s.Text = "A"
	f.observe(t, r)
	f.c.BeforeClose("one")
	f.finishSave(t)
	if f.close != 0 {
		t.Fatal("changed registry borrowed old proof")
	}
}

func TestReleaseClosedOnceRechecksAfterAcceptedFirstPhase(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	if !f.c.BeforeCloseOnce("one") {
		t.Fatal("clean first phase refused")
	}
	n.s.Text = "second phase input"
	if f.c.ReleaseClosedOnce("one") == nil {
		t.Fatal("second phase reused old clean proof")
	}
	if f.c.find(r) == nil || f.c.find(r).flight == nil || f.c.hasIntents() {
		t.Fatal("second phase cancelled save or leaked intent")
	}
	f.finishSave(t)
	if f.close != 0 || n.s.Text != "second phase input" {
		t.Fatal("second phase late close")
	}
	if err := f.c.ReleaseClosedOnce("one"); err != nil {
		t.Fatal(err)
	}
	if f.c.find(r) != nil {
		t.Fatal("fresh release did not dispose")
	}
}
