package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

type nativeBusinessQueuedLane struct{ q chan func() }

func (l *nativeBusinessQueuedLane) Enqueue(f func()) {
	select {
	case l.q <- f:
	default:
		panic("test lane duplicate enqueue")
	}
}
func nativeBusinessTestProbe(t *testing.T) (*nativeBusinessProbe, *nativeBusinessQueuedLane) {
	t.Helper()
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	b := newNativeBusinessProbe("create", func() time.Time { return now })
	if err := b.sample("startup"); err != nil {
		t.Fatal(err)
	}
	s, err := taskapp.CreateOrdinaryDevelopment(filepath.Join(t.TempDir(), "session"), core.LocalDate{Year: 2026, Month: 6, Day: 12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.StopAdmission(); _ = s.Close() })
	b.service = s
	l := &nativeBusinessQueuedLane{make(chan func(), 1)}
	b.lane = l
	b.post = func(f func()) { f() }
	b.publish = func(taskapp.OrdinaryReadView) error { return nil }
	return b, l
}
func nativeBusinessWait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("unit controller deadlock")
		return nil
	}
}

// Prefer an already-delivered command error over a timeout waiting for a
// callback which execute never posted. All waits are bounded unit-test guards.
func nativeBusinessTakeCallback(t *testing.T, q <-chan func(), done <-chan error, phase string) func() {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("controller completed before %s: %v", phase, err)
	default:
	}
	select {
	case err := <-done:
		t.Fatalf("controller completed before %s: %v", phase, err)
	case f := <-q:
		return f
	case <-time.After(5 * time.Second):
		select {
		case err := <-done:
			t.Fatalf("controller failed waiting for %s: %v", phase, err)
		default:
		}
		t.Fatalf("unit controller timeout waiting for %s", phase)
	}
	return nil
}
func TestNativeBusinessControllerDuplicateAdmission(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	f := nativeBusinessTakeCallback(t, l.q, done, "writer admission")
	if err := b.run(); err == nil {
		t.Fatal("duplicate admitted")
	}
	f()
	_ = nativeBusinessWait(t, done)
	b.StopAdmission()
	if len(l.q) != 0 {
		t.Fatal("duplicate writer task")
	}
	if len(b.report.Operations) != 2 {
		t.Fatalf("operation count %d", len(b.report.Operations))
	}
	if _, err := b.Finalize(nil); err == nil {
		t.Fatal("duplicate failure turned green")
	}
}
func TestNativeBusinessControllerCancelBeforeExecute(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	f := nativeBusinessTakeCallback(t, l.q, done, "writer admission")
	b.StopAdmission()
	if err := nativeBusinessWait(t, done); err == nil {
		t.Fatal("cancellation succeeded")
	}
	f()
	if len(b.report.Operations) != 0 || b.report.UIReadViewAcknowledged {
		t.Fatal("canceled task performed commands")
	}
}
func TestNativeBusinessControllerDiscardedAndLateCallbacks(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	posted := make(chan func(), 1)
	b.post = func(f func()) {
		select {
		case posted <- f:
		default:
			panic("duplicate publish callback")
		}
	}
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	nativeBusinessTakeCallback(t, l.q, done, "writer admission")()
	f := nativeBusinessTakeCallback(t, posted, done, "publish callback")
	select {
	case err := <-done:
		t.Fatalf("controller completed without executing callback: %v", err)
	default:
	}
	b.StopAdmission()
	if err := nativeBusinessWait(t, done); err == nil {
		t.Fatal("discarded callback succeeded")
	}
	f()
	if b.report.UIReadViewAcknowledged || b.report.PersistentFactsJSON != "" {
		t.Fatal("late callback published after cancellation")
	}
	if err := b.PreClose(); err == nil {
		t.Fatal("pre-close accepted no UI ack")
	}
}
func TestNativeBusinessControllerPublishFailure(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	b.publish = func(taskapp.OrdinaryReadView) error { return errors.New("publish rejected") }
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	nativeBusinessTakeCallback(t, l.q, done, "writer admission")()
	if err := nativeBusinessWait(t, done); err == nil || !strings.Contains(err.Error(), "publish rejected") {
		t.Fatalf("publish failure: %v", err)
	}
	b.StopAdmission()
	if b.report.UIReadViewAcknowledged {
		t.Fatal("publish failure acknowledged")
	}
	if _, err := b.Finalize(nil); err == nil {
		t.Fatal("publish failure finalized green")
	}
}
func TestNativeBusinessControllerDateBeforeFirstWrite(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	b.now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) }
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	nativeBusinessTakeCallback(t, l.q, done, "writer admission")()
	if err := nativeBusinessWait(t, done); err == nil || !strings.Contains(err.Error(), "date-boundary-not-covered") {
		t.Fatalf("boundary result %v", err)
	}
	b.StopAdmission()
	if len(b.report.Operations) != 0 {
		t.Fatal("write after observed boundary")
	}
}
func TestNativeBusinessControllerPreCloseChangeAndCloseFailure(t *testing.T) {
	b, l := nativeBusinessTestProbe(t)
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	nativeBusinessTakeCallback(t, l.q, done, "writer admission")()
	if err := nativeBusinessWait(t, done); err != nil {
		t.Fatal(err)
	}
	b.StopAdmission()
	if err := b.service.AddToday("unexpected later task"); err != nil {
		t.Fatal(err)
	}
	if err := b.PreClose(); err == nil {
		t.Fatal("pre-close changed facts accepted")
	}
	b.mu.Lock()
	b.report.WindowBindingConfirmed = true
	b.mu.Unlock()
	payload, err := b.Finalize(errors.New("injected Service.Close failure"))
	if err == nil || !strings.Contains(string(payload), "injected Service.Close failure") || strings.Contains(string(payload), `"complete":true`) {
		t.Fatal("close failure finalized green")
	}
}
func TestNativeBusinessControllerAdmissionTerminalRace(t *testing.T) {
	// The stop path closes the same admission gate before lane.StopAfterDrain.
	// No panic is recovered: an enqueue-after-stop bug must fail the test.
	for i := 0; i < 128; i++ {
		b := newNativeBusinessProbe("open", time.Now)
		lane := coordinator.NewSerialLane()
		b.lane = lane
		// If admitted, canceled execute returns before needing service/publish.
		barrier := make(chan struct{})
		lane.Enqueue(func() {
			select {
			case <-barrier:
			case <-time.After(5 * time.Second):
				panic("test barrier not released")
			}
		})
		done := make(chan error, 1)
		go func() { done <- b.run() }()
		stopped := make(chan struct{})
		go func() { b.StopAdmission(); lane.StopAfterDrain(); close(barrier); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("terminal admission stop did not return")
		}
		if err := nativeBusinessWait(t, done); err == nil {
			t.Fatal("terminal race succeeded")
		}
		select {
		case <-lane.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("lane did not drain")
		}
	}
}
func TestNativeBusinessDateOffsetBoundary(t *testing.T) {
	b := newNativeBusinessProbe("open", func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) })
	if err := b.sample("startup"); err != nil {
		t.Fatal(err)
	}
	b.now = func() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.FixedZone("changed", 3600)) }
	if err := b.sample("after_open"); err == nil {
		t.Fatal("offset boundary accepted")
	}
}
