// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

// A manual queue preserves the production non-inline contract. Barriers, not
// timeouts or sleeps, decide exactly when accepted callbacks execute.
type admissionTestLane struct {
	mu              sync.Mutex
	queue           []func()
	stopped, closed bool
	done            chan struct{}
	beforeAppend    func()
}

func newAdmissionTestLane() *admissionTestLane { return &admissionTestLane{done: make(chan struct{})} }
func (l *admissionTestLane) TryEnqueue(f func()) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return false
	}
	if l.beforeAppend != nil {
		l.beforeAppend()
	}
	l.queue = append(l.queue, f)
	return true
}
func (l *admissionTestLane) StopAfterDrain() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = true
	// Done is closed by drain, which also represents the worker returning.
}
func (l *admissionTestLane) Done() <-chan struct{} { return l.done }
func (l *admissionTestLane) drain() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			if l.stopped && !l.closed {
				l.closed = true
				close(l.done)
			}
			l.mu.Unlock()
			return
		}
		f := l.queue[0]
		l.queue[0] = nil
		l.queue = l.queue[1:]
		l.mu.Unlock()
		f()
	}
}
func admissionTestFinish(a *runtimeAdmission, w, d *admissionTestLane) runtimeAdmissionReport {
	a.EndRun()
	a.WaitActiveUI()
	w.drain()
	a.FinishWriterDrain()
	d.drain()
	return a.FinishDispatchDrain()
}

func TestRuntimeAdmissionWriterLinearizesWithEndRun(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(w, d, func(f func()) { f() })
	atAppend, release := make(chan struct{}), make(chan struct{})
	w.beforeAppend = func() { close(atAppend); <-release }
	accepted := make(chan bool, 1)
	var calls int
	go func() { accepted <- a.TrySubmitWriter(func(func(func()) bool) { calls++ }) }()
	<-atAppend
	ending, ended := make(chan struct{}), make(chan struct{})
	go func() { close(ending); a.EndRun(); close(ended) }()
	<-ending
	close(release)
	if !<-accepted {
		t.Fatal("writer lost ownership after entering enqueue")
	}
	<-ended
	if a.TrySubmitWriter(func(func(func()) bool) { calls += 100 }) {
		t.Fatal("accepted after EndRun")
	}
	r := admissionTestFinish(a, w, d)
	if calls != 1 || r.WriterAccepted != 1 || r.WriterRejected != 1 || r.WorkerDone != 1 || r.ProtocolErrors != 0 {
		t.Fatalf("calls=%d report=%+v", calls, r)
	}
	if again := a.FinishDispatchDrain(); again != r {
		t.Fatalf("non-idempotent report: %+v", again)
	}
}

func TestRuntimeAdmissionProducerTimersAcrossEndRun(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	var ui []func()
	a := newRuntimeAdmission(w, d, func(f func()) { ui = append(ui, f) })
	calls := 0
	if !a.TryPostProducer(func() { calls++ }) {
		t.Fatal("live timer rejected")
	}
	d.drain()
	ui[0]() // live token is consumed
	if !a.TryPostProducer(func() { calls += 100 }) {
		t.Fatal("queued timer rejected")
	}
	d.drain()
	a.EndRun()
	// Both timer sources race terminal state without touching UI on rejection.
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.TryPostProducer(func() { t.Error("terminal timer entered UI") }) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	ui[1]()
	r := admissionTestFinish(a, w, d)
	if calls != 1 || accepted.Load() != 0 || r.ProducerAccepted != 2 || r.ProducerConsumed != 1 || r.ProducerUndelivered != 1 || r.ProducerRejected != 2 {
		t.Fatalf("calls=%d report=%+v", calls, r)
	}
}

func TestRuntimeAdmissionCompletionCapabilitySurvivesEndRun(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(w, d, func(f func()) { f() })
	var escaped func(func()) bool
	if !a.TrySubmitWriter(func(post func(func()) bool) {
		escaped = post
		for i := 0; i < 2; i++ {
			if !post(func() { t.Error("completion entered UI after EndRun") }) {
				t.Error("accepted worker completion rejected during writer drain")
			}
		}
	}) {
		t.Fatal("work rejected")
	}
	a.EndRun()
	w.drain()
	if escaped(func() { t.Error("escaped completion delivered") }) {
		t.Fatal("ended work retained capability")
	}
	a.WaitActiveUI()
	a.FinishWriterDrain()
	if escaped(func() {}) {
		t.Fatal("completion accepted after writer drain")
	}
	d.drain()
	r := a.FinishDispatchDrain()
	if r.WorkerDone != 1 || r.CompletionAccepted != 2 || r.CompletionUndelivered != 2 || r.CompletionRejected != 2 || r.ProtocolErrors != 2 {
		t.Fatalf("report=%+v", r)
	}
}

func TestRuntimeAdmissionUIConsumptionIsNotDispatcherDrain(t *testing.T) {
	for _, mode := range []string{"drop", "late", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			w, d := newAdmissionTestLane(), newAdmissionTestLane()
			var ui []func()
			a := newRuntimeAdmission(w, d, func(f func()) {
				if mode != "drop" {
					ui = append(ui, f)
				}
			})
			calls := 0
			a.TrySubmitWriter(func(post func(func()) bool) { post(func() { calls++ }) })
			w.drain()
			d.drain()
			if mode == "duplicate" {
				ui[0]()
				ui[0]()
			}
			a.EndRun()
			if mode == "late" {
				ui[0]()
				ui[0]()
			}
			r := admissionTestFinish(a, w, d)
			if mode == "duplicate" {
				if calls != 1 || r.CompletionConsumed != 1 || r.CompletionUndelivered != 0 {
					t.Fatalf("calls=%d report=%+v", calls, r)
				}
			} else if calls != 0 || r.CompletionConsumed != 0 || r.CompletionUndelivered != 1 {
				t.Fatalf("calls=%d report=%+v", calls, r)
			}
			if len(ui) > 0 {
				ui[0]()
			} // no resurrection after final report
			if a.FinishDispatchDrain() != r {
				t.Fatal("late duplicate changed report")
			}
			if len(a.deliveries) != 0 {
				t.Fatal("drain retained result closures")
			}
		})
	}
}

func TestRuntimeAdmissionLiveCompletionRejectionIsProtocolError(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(w, d, func(f func()) { f() })
	d.StopAfterDrain() // fault injection: production admission owns this stop
	a.TrySubmitWriter(func(post func(func()) bool) {
		if post(func() { t.Error("rejected completion ran inline") }) {
			t.Error("stopped dispatcher accepted completion")
		}
	})
	w.drain()
	r := admissionTestFinish(a, w, d)
	if r.WorkerDone != 1 || r.CompletionRejected != 1 || r.CompletionConsumed != 0 || r.ProtocolErrors != 1 {
		t.Fatalf("report=%+v", r)
	}
}

func TestRuntimeAdmissionActiveUIAndConcurrentDuplicate(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	var ui []func()
	a := newRuntimeAdmission(w, d, func(f func()) { ui = append(ui, f) })
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.TryPostProducer(func() {
		calls.Add(1)
		// Reentrant admission demonstrates callback execution is outside mu.
		if !a.TrySubmitWriter(func(func(func()) bool) {}) {
			t.Error("live callback writer rejected")
		}
		close(entered)
		<-release
		if a.TrySubmitWriter(func(func(func()) bool) { t.Error("late writer executed") }) {
			t.Error("writer accepted after EndRun")
		}
	})
	a.TryPostProducer(func() { t.Error("pending callback acquired terminal permission") })
	d.drain()
	go func() { ui[0](); close(finished) }()
	<-entered
	ui[0]() // duplicate while original is RunningUI, not merely after consumed
	a.EndRun()
	ui[1]()
	a.mu.Lock()
	if a.activeUI != 1 {
		t.Errorf("active UI=%d", a.activeUI)
	}
	a.mu.Unlock()
	waitStarted, idle := make(chan struct{}), make(chan struct{})
	go func() { close(waitStarted); a.WaitActiveUI(); close(idle) }()
	<-waitStarted
	select {
	case <-idle:
		t.Fatal("wait returned while callback active")
	default:
	}
	close(release)
	<-finished
	<-idle
	r := admissionTestFinish(a, w, d)
	if calls.Load() != 1 || r.ProducerConsumed != 1 || r.ProducerUndelivered != 1 || r.WriterAccepted != 1 || r.WriterRejected != 1 {
		t.Fatalf("calls=%d report=%+v", calls.Load(), r)
	}
}

func TestRuntimeAdmissionClosedLanesRejectWithoutRunningCallbacks(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(w, d, func(f func()) { f() })
	w.StopAfterDrain()
	if a.TrySubmitWriter(func(func(func()) bool) { t.Error("rejected writer executed") }) {
		t.Fatal("stopped writer accepted")
	}
	d.StopAfterDrain()
	if a.TryPostProducer(func() { t.Error("rejected producer executed") }) {
		t.Fatal("stopped dispatcher accepted")
	}
	r := admissionTestFinish(a, w, d)
	if r.WriterAccepted != 0 || r.ProducerAccepted != 0 || r.WriterRejected != 1 || r.ProducerRejected != 1 || r.ProtocolErrors != 2 {
		t.Fatalf("report=%+v", r)
	}
}
