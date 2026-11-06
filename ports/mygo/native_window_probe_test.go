package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func finishNativeProbe(p *nativeWindowProbe) ([]byte, error) {
	p.RecordRunReturned(nil)
	p.RecordDrain(true)
	p.RecordDrain(false)
	p.RecordServiceClose(nil)
	return p.Finalize()
}
func successfulNativeProbe(t *testing.T, attempts int) (*nativeWindowProbe, *bytes.Buffer) {
	t.Helper()
	out := new(bytes.Buffer)
	p := newNativeWindowProbe("create", "2026-11-06", out)
	p.observeCreated(1)
	// ready can arrive before the controller waits; its wake remains available.
	p.observeWindow(1, "ready")
	p.startController(1, func(f func()) { f() }, func() (bool, bool, int, int) { return true, true, 1200, 768 }, func() {
		for i := 0; i < attempts; i++ {
			p.observeWindow(1, "close_attempt")
		}
		p.observeWindow(1, "closed")
	})
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("controller hung")
	}
	return p, out
}
func TestNativeProbeLifecycle(t *testing.T) {
	for _, n := range []int{1, 2, 32} {
		p, out := successfulNativeProbe(t, n)
		b, err := finishNativeProbe(p)
		if err != nil {
			t.Fatal(err)
		}
		if out.String() != nativeWindowReadyLine {
			t.Fatal("marker")
		}
		var r nativeWindowReport
		if err = json.Unmarshal(b, &r); err != nil || !r.Complete || r.DisplayPixelsVerified || !r.ControllerStopped || r.Today != "2026-11-06" {
			t.Fatalf("report %s %v", b, err)
		}
	}
}
func TestNativeProbeRejectsFalseSuccess(t *testing.T) {
	cases := []string{"no-window", "second-window", "early-close", "duplicate-ready", "unknown-window", "overflow", "run-error", "writer-missing", "dispatcher-missing", "first-close-error", "guard-never-allows"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			p := newNativeWindowProbe("open", "2026-11-06", io.Discard)
			if name != "no-window" {
				p.observeCreated(1)
				p.observeWindow(1, "ready")
				p.mu.Lock()
				p.report.Visible = true
				p.report.NativeHandlePresent = true
				p.report.ContentWidth = 1
				p.report.ContentHeight = 1
				p.eventLocked("visibility_checked")
				p.eventLocked("probe_close_requested")
				p.mu.Unlock()
				p.observeWindow(1, "close_attempt")
				if name != "guard-never-allows" {
					p.observeWindow(1, "closed")
				}
			}
			switch name {
			case "second-window":
				p.observeCreated(2)
			case "early-close":
				p.observeWindow(1, "closed")
			case "duplicate-ready":
				p.observeWindow(1, "ready")
			case "unknown-window":
				p.observeWindow(4, "ready")
			case "overflow":
				for i := 0; i < 80; i++ {
					p.observeWindow(1, "close_attempt")
				}
			}
			var err error
			if name == "run-error" {
				err = errors.New("run")
			}
			p.RecordRunReturned(err)
			if name != "writer-missing" {
				p.RecordDrain(true)
			}
			if name != "dispatcher-missing" {
				p.RecordDrain(false)
			}
			if name == "first-close-error" {
				p.RecordServiceClose(errors.New("first"))
				p.RecordServiceClose(nil)
			} else {
				p.RecordServiceClose(nil)
			}
			b, err := p.Finalize()
			if err == nil {
				t.Fatalf("false green %s", b)
			}
			if name == "first-close-error" && !bytes.Contains(b, []byte(`"service_close_error":"first"`)) {
				t.Fatal(string(b))
			}
		})
	}
}
func TestNativeProbeDroppedDispatchAndInvisible(t *testing.T) {
	for _, drop := range []bool{true, false} {
		p := newNativeWindowProbe("create", "2026-11-06", io.Discard)
		p.observeCreated(1)
		p.observeWindow(1, "ready")
		called := make(chan struct{}, 1)
		p.startController(1, func(f func()) {
			if !drop {
				f()
			}
			select {
			case called <- struct{}{}:
			default:
			}
		}, func() (bool, bool, int, int) { return false, true, 1, 1 }, func() { t.Error("unexpected close") })
		<-called
		b, err := finishNativeProbe(p)
		if err == nil || bytes.Contains(b, []byte(`"name":"visibility_checked"`)) {
			t.Fatalf("discarded callback became success %s", b)
		}
	}
}

type brokenNativeWriter struct{}

func (brokenNativeWriter) Write(b []byte) (int, error) { return 0, errors.New("blocked output") }
func TestNativeProbeMarkerFailure(t *testing.T) {
	p := newNativeWindowProbe("create", "2026-11-06", brokenNativeWriter{})
	p.observeCreated(1)
	p.observeWindow(1, "ready")
	p.startController(1, func(f func()) { f() }, func() (bool, bool, int, int) { return true, true, 1, 1 }, func() { t.Error("closed despite marker failure") })
	<-p.done
	if _, err := finishNativeProbe(p); err == nil {
		t.Fatal("false success")
	}
}
func TestNativeProbeCancelRace(t *testing.T) {
	for i := 0; i < 50; i++ {
		p := newNativeWindowProbe("create", "2026-11-06", io.Discard)
		p.observeCreated(1)
		p.startController(1, func(f func()) { f() }, func() (bool, bool, int, int) { return true, true, 1, 1 }, func() { p.observeWindow(1, "close_attempt"); p.observeWindow(1, "closed") })
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); p.observeWindow(1, "ready") }()
		go func() { defer wg.Done(); p.RecordRunReturned(nil) }()
		wg.Wait()
		p.RecordDrain(true)
		p.RecordDrain(false)
		p.RecordServiceClose(nil)
		p.Finalize()
	}
}
func TestNativeProbeMainKeepsGuardOrdering(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "OnWindowAllClosed") {
		t.Fatal("observer changes default quit")
	}
	guard := strings.Index(s, "if !hub.beforeClose(s)")
	bind := strings.Index(s, "probe.BindWindow(win)")
	if guard < 0 || bind < guard {
		t.Fatal("probe must follow real hub guard")
	}
	if !strings.Contains(s, "Content: ui.View(s.view)") {
		t.Fatal("real content replaced")
	}
}

// This uses the real Service, titleHub and pending writer/dispatcher work. Only
// native event delivery is simulated: it is not a native-window pass.
func TestNativeProbeRealHubGuardRetry(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		t.Run(fmt.Sprint(failSave), func(t *testing.T) {
			f := newTitleFixture(t, true)
			f.edit("guarded draft")
			p := newNativeWindowProbe("create", "2026-11-06", io.Discard)
			p.observeCreated(1)
			p.observeWindow(1, "ready")
			p.mu.Lock()
			p.report.Visible = true
			p.report.NativeHandlePresent = true
			p.report.ContentWidth = 1200
			p.report.ContentHeight = 768
			p.eventLocked("visibility_checked")
			p.eventLocked("probe_close_requested")
			p.mu.Unlock()
			attempts, closed := 0, 0
			closeAttempt := func() {
				allowed := f.h.beforeClose(f.s)
				attempts++
				p.observeWindow(1, "close_attempt")
				if allowed {
					closed++
					p.observeWindow(1, "closed")
				}
			}
			f.h.windows[f.s.titleWindowID].close = closeAttempt
			if failSave {
				if err := f.svc.Close(); err != nil {
					t.Fatal(err)
				}
			}
			closeAttempt()
			if closed != 0 {
				t.Fatal("guard bypassed")
			}
			f.all()
			b, err := finishNativeProbe(p)
			if failSave {
				if closed != 0 || err == nil {
					t.Fatalf("failed writer allowed close %s", b)
				}
			} else {
				if err != nil || closed != 1 || attempts < 2 {
					t.Fatalf("guard retry lost: attempts=%d closed=%d err=%v", attempts, closed, err)
				}
			}
		})
	}
}

func TestNativeProbeStalledControllerCannotFinalize(t *testing.T) {
	p := newNativeWindowProbe("create", "2026-11-06", io.Discard)
	p.observeCreated(1)
	p.observeWindow(1, "ready")
	entered, release := make(chan struct{}), make(chan struct{})
	p.startController(1, func(f func()) { close(entered); <-release; f() }, func() (bool, bool, int, int) { return true, true, 1, 1 }, func() { t.Error("close after cancellation") })
	<-entered
	p.RecordRunReturned(nil)
	p.RecordDrain(true)
	p.RecordDrain(false)
	p.RecordServiceClose(nil)
	start := time.Now()
	b, err := p.Finalize()
	if err == nil || b != nil || time.Since(start) > 2*time.Second {
		t.Fatal("stalled controller fabricated final report", err)
	}
	close(release)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("controller did not finish after release")
	}
}
