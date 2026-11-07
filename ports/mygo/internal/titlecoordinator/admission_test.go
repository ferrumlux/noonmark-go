// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

func TestRejectedWriterCompletesOriginalFlightWithoutAcknowledgment(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "  retained draft  "
	f.observe(t, r)
	var request titlesession.SaveRequest
	f.c.cfg.TrySubmit = func(work func(func(func()) bool)) bool {
		e := f.c.find(r)
		if e.flight == nil || !e.session.State().InFlight {
			t.Fatal("request was not begun")
		}
		request = *e.flight
		return false
	}
	if f.c.BeforeClose("w") {
		t.Fatal("rejected write permitted close")
	}
	e := f.c.find(r)
	s := e.session.State()
	if e.flight != nil || s.InFlight || s.AwaitingVerification || !errors.Is(s.LastError, ErrNotAdmitted) {
		t.Fatalf("original request not completed: %+v", s)
	}
	if s.Draft != n.s.Text || s.LastAcknowledgedText != "旧标题" || n.replaces != 0 || len(f.s.calls) != 0 || f.c.hasIntents() {
		t.Fatalf("rejection changed draft/acknowledgment or retained intent: %+v", s)
	}
	if len(f.errs) != 1 || !errors.Is(f.errs[0], ErrNotAdmitted) {
		t.Fatal(f.errs)
	}
	f.c.complete(e, request, titlesession.PersistenceResult{Err: ErrNotAdmitted})
	if len(f.errs) != 1 {
		t.Fatal("duplicate completion settled twice")
	}
	f.c.cfg.TrySubmit = f.trySubmit
	f.c.BeforeClose("w")
	f.finishSave(t)
	if f.close != 1 {
		t.Fatal("rejection prevented later retry")
	}
}

func TestDebounceProducerRejectionDoesNotTouchNativeOrSession(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "draft"
	f.observe(t, r)
	e := f.c.find(r)
	before := e.session.State()
	job, token := *e.job, e.timer
	f.rejectProducer = true
	f.k.advance(time.Second)
	if !reflect.DeepEqual(before, e.session.State()) || e.job == nil || *e.job != job || e.timer != token {
		t.Fatal("timer rejection mutated UI-owned state")
	}
	if len(f.l.q) != 0 || len(f.post) != 0 || len(f.s.calls) != 0 || n.finishes != 0 || n.replaces != 0 {
		t.Fatal("rejected timer ran work")
	}
	f.rejectProducer = false
	f.observe(t, r)
	f.finishSave(t)
	if e.session.State().LastAcknowledgedText != "draft" {
		t.Fatal("later UI observation could not save")
	}
}

func TestRegistryRetryRejectionClearsOnlyItsIntentAndCanRetry(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "unobserved draft"
	i := &intent{kind: "navigate", window: "w", target: "target"}
	other := &intent{kind: "navigate", window: "other", target: "other-target"}
	f.c.intents["w"], f.c.intents["other"] = i, other
	f.rejectProducer = true
	f.c.registryChanged(i)
	if i.recheckQueued || f.c.currentIntent(i) || !f.c.currentIntent(other) {
		t.Fatal("incorrect rejected intent cleanup")
	}
	if len(f.errs) != 1 || !errors.Is(f.errs[0], ErrNotAdmitted) || len(f.post) != 0 {
		t.Fatal(f.errs)
	}
	if n.s.Text != "unobserved draft" || n.finishes != 0 || n.replaces != 0 || f.c.find(r) == nil {
		t.Fatal("rejection touched editor")
	}
	f.rejectProducer = false
	f.c.RequestNavigation("w", "new-target")
	f.finishSave(t)
	if !reflect.DeepEqual(f.nav, []string{"w:new-target"}) {
		t.Fatal(f.nav)
	}
}

func TestAcceptedWriterUsesCompletionWhenProducersAreRejected(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "draft"
	f.observe(t, r)
	f.advance(time.Second)
	f.rejectProducer, f.rejectSubmit = true, true
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.InFlight || s.LastAcknowledgedText != "draft" || s.LastError != nil {
		t.Fatal(s)
	}
}

func TestRejectedCompletionNeverRunsUIFallbackOrClaimsFlightFinished(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "draft"
	f.observe(t, r)
	f.advance(time.Second)
	f.rejectCompletion = true
	f.l.one()
	f.ui()
	s, _ := f.c.State(r)
	if f.completionRejected != 1 || !s.InFlight || f.c.find(r).flight == nil || s.LastAcknowledgedText != "旧标题" {
		t.Fatalf("lost completion was disguised as success: %+v", s)
	}
	if len(f.s.calls) != 2 || len(f.errs) != 0 || n.replaces != 0 {
		t.Fatal("completion rejection rewrote worker outcome or touched UI")
	}
}
