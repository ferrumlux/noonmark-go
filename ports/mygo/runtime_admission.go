// SPDX-License-Identifier: AGPL-3.0-only
package main

import "sync"

// runtimeAdmissionLane must queue, never invoke the callback inline. Its methods
// take only short locks; Done closes after every accepted callback has returned.
type runtimeAdmissionLane interface {
	TryEnqueue(func()) bool
	StopAfterDrain()
	Done() <-chan struct{}
}

type runtimeAdmissionPhase uint8

const (
	runtimeAdmissionRunning runtimeAdmissionPhase = iota
	runtimeAdmissionWriterDraining
	runtimeAdmissionDispatchDraining
	runtimeAdmissionClosed
)

// Counts describe delivery, not persistence or business-ticket settlement. They
// deliberately contain no user text, receipt, error text, or native reference.
type runtimeAdmissionReport struct {
	WriterAccepted, WriterRejected, WorkerDone                                        uint64
	ProducerAccepted, ProducerRejected, ProducerConsumed, ProducerUndelivered         uint64
	CompletionAccepted, CompletionConsumed, CompletionUndelivered, CompletionRejected uint64
	ProtocolErrors                                                                    uint64
}

type runtimeAdmissionDelivery struct {
	completion bool
	running    bool
	deliver    func()
}

type runtimeAdmissionWork struct{ active bool }

type runtimeAdmission struct {
	mu                 sync.Mutex
	uiIdle             *sync.Cond
	writer, dispatcher runtimeAdmissionLane
	runOnMain          func(func())
	phase              runtimeAdmissionPhase
	activeUI           uint64
	nextDelivery       uint64
	deliveries         map[uint64]*runtimeAdmissionDelivery
	report             runtimeAdmissionReport
}

func newRuntimeAdmission(writer, dispatcher runtimeAdmissionLane, runOnMain func(func())) *runtimeAdmission {
	if writer == nil || dispatcher == nil || runOnMain == nil {
		panic("runtime admission requires lanes and main-thread scheduler")
	}
	a := &runtimeAdmission{writer: writer, dispatcher: dispatcher, runOnMain: runOnMain, deliveries: make(map[uint64]*runtimeAdmissionDelivery)}
	a.uiIdle = sync.NewCond(&a.mu)
	return a
}

func (a *runtimeAdmission) TrySubmitWriter(work func(postCompletion func(func()) bool)) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase != runtimeAdmissionRunning {
		a.report.WriterRejected++
		return false
	}
	w := &runtimeAdmissionWork{active: true}
	if !a.writer.TryEnqueue(func() {
		defer func() {
			a.mu.Lock()
			w.active = false
			a.report.WorkerDone++
			a.mu.Unlock()
		}()
		// This capability belongs only to this work's synchronous call chain.
		work(func(deliver func()) bool { return a.postCompletion(w, deliver) })
	}) {
		w.active = false
		a.report.WriterRejected++
		a.report.ProtocolErrors++ // writer unexpectedly stopped while Running
		return false
	}
	a.report.WriterAccepted++
	return true
}

func (a *runtimeAdmission) TryPostProducer(deliver func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase != runtimeAdmissionRunning {
		a.report.ProducerRejected++
		return false
	}
	return a.enqueueDeliveryLocked(deliver, false)
}

func (a *runtimeAdmission) postCompletion(w *runtimeAdmissionWork, deliver func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !w.active || a.phase >= runtimeAdmissionDispatchDraining {
		a.report.CompletionRejected++
		a.report.ProtocolErrors++
		return false
	}
	return a.enqueueDeliveryLocked(deliver, true)
}

func (a *runtimeAdmission) enqueueDeliveryLocked(deliver func(), completion bool) bool {
	a.nextDelivery++
	id := a.nextDelivery
	a.deliveries[id] = &runtimeAdmissionDelivery{completion: completion, deliver: deliver}
	if !a.dispatcher.TryEnqueue(func() {
		// Only the ID escapes to the framework. Final drain can release the
		// business closure even if the framework retains or loses this wrapper.
		a.runOnMain(func() { a.consume(id) })
	}) {
		delete(a.deliveries, id)
		if completion {
			a.report.CompletionRejected++
		} else {
			a.report.ProducerRejected++
		}
		a.report.ProtocolErrors++
		return false
	}
	if completion {
		a.report.CompletionAccepted++
	} else {
		a.report.ProducerAccepted++
	}
	return true
}

func (a *runtimeAdmission) consume(id uint64) {
	a.mu.Lock()
	d := a.deliveries[id]
	if d == nil || d.running {
		a.mu.Unlock()
		return
	}
	if a.phase != runtimeAdmissionRunning {
		a.undeliveredLocked(id, d)
		a.mu.Unlock()
		return
	}
	// Pending -> RunningUI is atomic, including against duplicate callbacks.
	d.running = true
	a.activeUI++
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.deliveries, id)
		if d.completion {
			a.report.CompletionConsumed++
		} else {
			a.report.ProducerConsumed++
		}
		d.deliver = nil
		a.activeUI--
		if a.activeUI == 0 {
			a.uiIdle.Broadcast()
		}
		a.mu.Unlock()
	}()
	d.deliver()
}

func (a *runtimeAdmission) undeliveredLocked(id uint64, d *runtimeAdmissionDelivery) {
	if d.completion {
		a.report.CompletionUndelivered++
	} else {
		a.report.ProducerUndelivered++
	}
	d.deliver = nil
	delete(a.deliveries, id)
}

func (a *runtimeAdmission) EndRun() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase != runtimeAdmissionRunning {
		return
	}
	a.phase = runtimeAdmissionWriterDraining
	a.writer.StopAfterDrain()
}

// WaitActiveUI is a non-UI cleanup step after EndRun. It waits only for callbacks
// already granted permission, never for callbacks still queued in the framework.
func (a *runtimeAdmission) WaitActiveUI() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase == runtimeAdmissionRunning {
		panic("wait for active UI before EndRun")
	}
	for a.activeUI != 0 {
		a.uiIdle.Wait()
	}
}

// FinishWriterDrain must be called after writer.Done and WaitActiveUI, outside
// UI callbacks. It never waits while holding the admission mutex.
func (a *runtimeAdmission) FinishWriterDrain() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase >= runtimeAdmissionDispatchDraining {
		return
	}
	if a.phase != runtimeAdmissionWriterDraining || a.activeUI != 0 {
		panic("finish writer drain before EndRun or active UI completion")
	}
	select {
	case <-a.writer.Done():
	default:
		panic("finish writer drain before writer Done")
	}
	a.phase = runtimeAdmissionDispatchDraining
	a.dispatcher.StopAfterDrain()
}

func (a *runtimeAdmission) FinishDispatchDrain() runtimeAdmissionReport {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.phase == runtimeAdmissionClosed {
		return a.report
	}
	if a.phase != runtimeAdmissionDispatchDraining || a.activeUI != 0 {
		panic("finish dispatch drain before writer drain or active UI completion")
	}
	select {
	case <-a.dispatcher.Done():
	default:
		panic("finish dispatch drain before dispatcher Done")
	}
	for id, d := range a.deliveries {
		a.undeliveredLocked(id, d)
	}
	a.phase = runtimeAdmissionClosed
	return a.report
}
