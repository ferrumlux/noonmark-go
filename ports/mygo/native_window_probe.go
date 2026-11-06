package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const nativeWindowReadyLine = "NOONMARK_NATIVE_WINDOW_READY_V1\n"

type nativeWindowEvent struct {
	Seq       int    `json:"seq"`
	Name      string `json:"name"`
	ElapsedNS int64  `json:"elapsed_ns"`
}
type nativeWindowReport struct {
	Version               int                 `json:"version"`
	Scope                 string              `json:"scope"`
	Stage                 string              `json:"stage"`
	Today                 string              `json:"today"`
	WindowID              int                 `json:"window_id"`
	WindowCount           int                 `json:"window_count"`
	Events                []nativeWindowEvent `json:"events"`
	Visible               bool                `json:"visible"`
	NativeHandlePresent   bool                `json:"native_handle_present"`
	ContentWidth          int                 `json:"content_width"`
	ContentHeight         int                 `json:"content_height"`
	NativeFrameSubmitted  bool                `json:"native_frame_submitted"`
	DisplayPixelsVerified bool                `json:"display_pixels_verified"`
	RunError              string              `json:"run_error"`
	WriterDrained         bool                `json:"writer_drained"`
	DispatcherDrained     bool                `json:"dispatcher_drained"`
	ServiceCloseError     string              `json:"service_close_error"`
	ControllerStopped     bool                `json:"controller_stopped"`
	Errors                []string            `json:"errors"`
	Complete              bool                `json:"complete"`
}

// All observations are made at their actual callback/completion sites. The lock
// never spans native calls, dispatch, waits or report I/O.
type nativeWindowProbe struct {
	mu                        sync.Mutex
	report                    nativeWindowReport
	startedAt                 time.Time
	phase                     string
	attempts                  int
	canceled, started, frozen bool
	serviceCloseRecorded      bool
	wake, cancel, done        chan struct{}
	output                    io.Writer
}

func newNativeWindowProbe(stage, today string, output io.Writer) *nativeWindowProbe {
	return &nativeWindowProbe{report: nativeWindowReport{Version: 1, Scope: "lifecycle-only", Stage: stage, Today: today, Events: []nativeWindowEvent{}, Errors: []string{}}, startedAt: time.Now(), wake: make(chan struct{}, 1), cancel: make(chan struct{}), done: make(chan struct{}), output: output}
}
func (p *nativeWindowProbe) failLocked(s string) {
	if len(p.report.Errors) < 32 {
		p.report.Errors = append(p.report.Errors, s)
	}
}
func (p *nativeWindowProbe) fail(s string) { p.mu.Lock(); defer p.mu.Unlock(); p.failLocked(s) }
func (p *nativeWindowProbe) eventLocked(name string) {
	if p.frozen {
		return
	}
	want := map[string]string{"created": "", "ready": "created", "visibility_checked": "ready", "probe_close_requested": "visibility_checked", "closed": "close_attempt", "run_returned": "closed", "writer_drained": "run_returned", "dispatcher_drained": "writer_drained", "service_close_result": "dispatcher_drained", "finalized": "service_close_result"}
	valid := false
	if name == "close_attempt" {
		p.attempts++
		valid = (p.phase == "probe_close_requested" || p.phase == "close_attempt") && p.attempts <= 32
	} else {
		prior, ok := want[name]
		valid = ok && p.phase == prior
	}
	if !valid {
		p.failLocked("unexpected event: " + name)
	}
	if len(p.report.Events) >= 64 {
		p.failLocked("event overflow")
		return
	}
	p.report.Events = append(p.report.Events, nativeWindowEvent{len(p.report.Events) + 1, name, time.Since(p.startedAt).Nanoseconds()})
	p.phase = name
}
func (p *nativeWindowProbe) observeCreated(id int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.report.WindowCount++
	if id <= 0 || p.report.WindowCount != 1 {
		p.failLocked("expected one window")
	}
	if p.report.WindowCount == 1 {
		p.report.WindowID = id
	}
	p.eventLocked("created")
}
func (p *nativeWindowProbe) observeWindow(id int, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id != p.report.WindowID {
		p.failLocked("unknown window")
		return
	}
	p.eventLocked(name)
	if name == "ready" {
		p.report.NativeFrameSubmitted = true
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
}
func (p *nativeWindowProbe) BindWindow(win *mygo.Window) {
	id := win.ID()
	win.OnReadyToShow(func() { p.observeWindow(id, "ready") })
	win.OnClose(func(_ *mygo.CloseEvent) { p.observeWindow(id, "close_attempt") })
	win.OnClosed(func() { p.observeWindow(id, "closed") })
	p.startController(id, mygo.RunOnMain, func() (bool, bool, int, int) {
		if win.IsDestroyed() {
			return false, false, 0, 0
		}
		w, h := win.ContentSize()
		return win.IsVisible(), win.NativeHandle() != 0, w, h
	}, win.Close)
}
func (p *nativeWindowProbe) startController(id int, dispatch func(func()), inspect func() (bool, bool, int, int), closeWindow func()) {
	p.mu.Lock()
	if p.started || p.canceled || id != p.report.WindowID {
		p.failLocked("invalid controller binding")
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()
	go func() {
		defer close(p.done)
		select {
		case <-p.cancel:
			return
		case <-p.wake:
		}
		checked := false
		for attempt := 0; attempt < 2250; attempt++ {
			select {
			case <-p.cancel:
				return
			default:
			}
			// RunOnMain may discard this callback. Only its body can set checked.
			dispatch(func() {
				p.mu.Lock()
				cancel := p.canceled
				p.mu.Unlock()
				if cancel {
					return
				}
				visible, handle, w, h := inspect()
				p.mu.Lock()
				defer p.mu.Unlock()
				if p.canceled {
					return
				}
				if visible && handle && w > 0 && h > 0 {
					p.report.Visible = visible
					p.report.NativeHandlePresent = handle
					p.report.ContentWidth = w
					p.report.ContentHeight = h
					p.eventLocked("visibility_checked")
					checked = true
				}
			})
			p.mu.Lock()
			success := checked
			p.mu.Unlock()
			if success {
				break
			}
			select {
			case <-p.cancel:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		p.mu.Lock()
		success := checked
		p.mu.Unlock()
		if !success {
			p.fail("visibility check exhausted")
			return
		}
		n, err := io.WriteString(p.output, nativeWindowReadyLine)
		if err != nil || n != len(nativeWindowReadyLine) {
			p.fail("ready marker write failed")
			return
		}
		dispatch(func() {
			p.mu.Lock()
			if p.canceled || p.phase != "visibility_checked" || id != p.report.WindowID {
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
			visible, handle, w, h := inspect()
			p.mu.Lock()
			if p.canceled {
				p.mu.Unlock()
				return
			}
			if !visible || !handle || w <= 0 || h <= 0 {
				p.failLocked("window changed before close")
				p.mu.Unlock()
				return
			}
			p.eventLocked("probe_close_requested")
			p.mu.Unlock()
			closeWindow()
		})
	}()
}
func (p *nativeWindowProbe) RecordRunReturned(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventLocked("run_returned")
	if err != nil {
		p.report.RunError = err.Error()
		p.failLocked("Run failed")
	}
	if !p.canceled {
		p.canceled = true
		close(p.cancel)
		if !p.started {
			close(p.done)
		}
	}
}
func (p *nativeWindowProbe) RecordDrain(writer bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if writer {
		p.eventLocked("writer_drained")
		p.report.WriterDrained = true
	} else {
		p.eventLocked("dispatcher_drained")
		p.report.DispatcherDrained = true
	}
}
func (p *nativeWindowProbe) RecordServiceClose(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventLocked("service_close_result")
	if p.serviceCloseRecorded {
		return
	}
	p.serviceCloseRecorded = true
	if err != nil {
		if p.report.ServiceCloseError == "" {
			p.report.ServiceCloseError = err.Error()
		}
		p.failLocked("Service.Close failed")
	}
}
func (p *nativeWindowProbe) Finalize() ([]byte, error) {
	select {
	case <-p.done:
	case <-time.After(time.Second):
		return nil, errors.New("probe controller did not stop")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.report.ControllerStopped = true
	p.eventLocked("finalized")
	if p.report.WindowCount != 1 || !p.report.NativeFrameSubmitted || !p.report.Visible || !p.report.NativeHandlePresent || p.attempts < 1 || !p.report.WriterDrained || !p.report.DispatcherDrained {
		p.failLocked("incomplete lifecycle")
	}
	var request int64 = -1
	for _, e := range p.report.Events {
		if e.Name == "probe_close_requested" {
			request = e.ElapsedNS
		}
		if e.Name == "finalized" && (request < 0 || e.ElapsedNS-request > int64(10*time.Second)) {
			p.failLocked("close budget exceeded")
		}
	}
	p.report.Complete = len(p.report.Errors) == 0
	p.frozen = true
	b, err := json.Marshal(p.report)
	if err != nil {
		return nil, err
	}
	if len(b) > 64*1024 {
		return nil, errors.New("report too large")
	}
	b = append(b, '\n')
	if !p.report.Complete {
		return b, fmt.Errorf("native lifecycle failed: %v", p.report.Errors)
	}
	return b, nil
}
