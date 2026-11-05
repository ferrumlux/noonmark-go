// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import "sync"

// SerialLane is an unbounded FIFO with one worker. Enqueue never waits for disk
// or UI completion. StopAfterDrain is terminal, after all editors are disposed
// and all command sources stopped. Done is for NON-UI cleanup/tests only.
type SerialLane struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []func()
	stopped bool
	done    chan struct{}
}

func NewSerialLane() *SerialLane {
	l := &SerialLane{done: make(chan struct{})}
	l.cond = sync.NewCond(&l.mu)
	go l.run()
	return l
}
func (l *SerialLane) Enqueue(f func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		panic("enqueue after terminal lane stop")
	}
	l.queue = append(l.queue, f)
	l.cond.Signal()
}
func (l *SerialLane) run() {
	defer close(l.done)
	for {
		l.mu.Lock()
		for len(l.queue) == 0 && !l.stopped {
			l.cond.Wait()
		}
		if len(l.queue) == 0 {
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
func (l *SerialLane) StopAfterDrain()       { l.mu.Lock(); l.stopped = true; l.cond.Signal(); l.mu.Unlock() }
func (l *SerialLane) Done() <-chan struct{} { return l.done }
