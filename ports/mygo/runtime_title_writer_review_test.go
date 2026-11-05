// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"testing"
	"time"
)

type reviewStore struct {
	*taskapp.Service
	advances int
	fail     error
}

func (s *reviewStore) AdvanceTo(d core.LocalDate) error {
	s.advances++
	if s.fail != nil {
		return s.fail
	}
	return s.Service.AdvanceTo(d)
}
func TestReviewStoppedDayNoWrites(t *testing.T) {
	f := newTitleFixture(t, true)
	store := &reviewStore{Service: f.svc}
	f.h.store = store
	f.h.command(f.s, func() error { return taskapp.ErrIncomplete }, false, nil)
	f.all()
	if !f.h.stopped {
		t.Fatal("fixture did not stop after command failure")
	}
	store.advances = 0
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.tt.Frame()
	f.all()
	if store.advances != 0 {
		t.Fatalf("stopped hub still called AdvanceTo %d times; today now %v", store.advances, store.Today())
	}
}
func TestReviewDayFailureStopsCommands(t *testing.T) {
	f := newTitleFixture(t, false)
	store := &reviewStore{Service: f.svc, fail: taskapp.ErrIncomplete}
	f.h.store = store
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.tt.Frame()
	f.ui()
	f.io.one()
	f.ui()
	if !f.h.stopped {
		t.Fatal("day settlement ErrIncomplete did not stop shared hub")
	}
}
func TestReviewQuickAddWaitsForTitle(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("先标题")
	liveType(t, f.tt, "后新增")
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if len(f.io.q) == 0 {
		t.Fatal("no work")
	}
	f.io.one()
	if len(f.svc.Snapshot().Chains) > 1 && f.persisted() != "先标题" {
		t.Fatal("quick-add mutation overtook pending title flush")
	}
	f.all()
}

func TestReviewDayFailureDoesNotImmediatelyRetry(t *testing.T) {
	f := newTitleFixture(t, false)
	store := &reviewStore{Service: f.svc, fail: taskapp.ErrLimit}
	f.h.store = store
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.tt.Frame()
	f.ui()
	f.io.one()
	f.ui()
	if len(f.io.q) > 0 {
		t.Fatalf("failed day immediately retried on error redraw: %d jobs", len(f.io.q))
	}
}
func TestReviewCommandFailureCancelsQuitAcrossWindows(t *testing.T) {
	f := newTitleFixture(t, false)
	other := newRuntimeApp(f.svc)
	f.h.attach(other, func() {}, func() { t.Fatal("unexpected close") })
	f.h.command(f.s, func() error { return taskapp.ErrLimit }, false, nil)
	f.h.command(other, func() error { return f.svc.AddPool("other") }, false, nil)
	if f.h.beforeQuit() {
		t.Fatal("pending commands allowed quit")
	}
	f.all()
	if f.quit != 0 || f.h.quitting {
		t.Fatal("later success revived failed quit")
	}
	if len(f.svc.Snapshot().Chains) != 1 {
		t.Fatal("independent command did not finish")
	}
}
