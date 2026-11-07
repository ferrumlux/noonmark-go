// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

const titleFailureMessage = "标题未保存，草稿已保留。请检查会话后再离开。"

type titleStore interface {
	taskService
	AdvanceTo(core.LocalDate) error
	SaveTraceTitleInputAcknowledged(core.DayTraceID, string) (string, error)
}

// titleReadView is UI-thread owned. Reads never acquire the Service lock held
// across disk commit. Mutators are forwarding-only and called on the writer lane.
type titleReadView struct{ hub *titleHub }

func (v *titleReadView) Snapshot() core.TaskGraph { return v.hub.graph.Clone() }
func (v *titleReadView) Today() core.LocalDate    { return v.hub.today }
func (v *titleReadView) AddPool(t string) error   { return v.hub.store.AddPool(t) }
func (v *titleReadView) AddToday(t string) error  { return v.hub.store.AddToday(t) }
func (v *titleReadView) AddOnDate(t string, d core.LocalDate) error {
	return v.hub.store.AddOnDate(t, d)
}
func (v *titleReadView) Schedule(id core.TaskChainID, d core.LocalDate) error {
	return v.hub.store.Schedule(id, d)
}
func (v *titleReadView) SetCompleted(id core.DayTraceID, b bool) error {
	return v.hub.store.SetCompleted(id, b)
}
func (v *titleReadView) ReturnToPool(chain core.TaskChainID, trace core.DayTraceID) error {
	return v.hub.store.ReturnToPool(chain, trace)
}
func (h *titleHub) readView() taskService { return &titleReadView{h} }

type wallTitleClock struct{}

func (wallTitleClock) Now() time.Time { return time.Now() }
func (wallTitleClock) After(d time.Duration, fn func()) func() {
	t := time.AfterFunc(d, fn)
	return func() { t.Stop() }
}

type titleNative struct {
	handle  ui.Handle
	context *ui.Context
	draft   string
	proof   titlesession.NativeSnapshot
	proven  bool
}

func (n *titleNative) read(finish bool) (titlesession.NativeSnapshot, bool) {
	v, ok := snapshotTitleNative(n.handle, n.context, finish)
	n.proof, n.proven = v, ok
	return v, ok
}
func (n *titleNative) Snapshot() (titlesession.NativeSnapshot, bool) { return n.read(false) }
func (n *titleNative) Finish() (titlesession.NativeSnapshot, bool)   { return n.read(true) }
func (n *titleNative) Replace(text string) bool {
	if !n.proven {
		return false
	}
	proof := n.proof
	// Re-read before touching the binding: late native input/IME wins.
	fresh, ok := snapshotTitleNative(n.handle, n.context, false)
	if !ok || fresh != proof || fresh.Composing {
		n.proven = false
		return false
	}
	n.draft = text
	actual, ok := snapshotTitleNative(n.handle, n.context, false)
	n.proof, n.proven = actual, ok
	return ok && actual.Text == text && !actual.Composing
}

type titleEditorKey struct {
	Owner               string
	Session, Generation uint64
}
type runtimeTitleEditor struct {
	subtask            *runtimeSubtaskRow
	owner              string
	native             titleNative
	row                taskview.DayRow
	reg                coordinator.Registration
	identity           titlesession.Identity
	key                titleEditorKey
	registered, queued bool
	seenText           string
	seenCompose        bool
	state              titlesession.State
	locked             bool
	failure            error
}
type titleWindow struct {
	category          *runtimeCategoryEditor
	app               *app
	invalidate, close func()
	commands          int
	navigation        uint64
	inputPending      map[string]bool
	closing           bool
}
type titleHub struct {
	categoryClock                      coordinator.Clock
	category                           *runtimeCategoryHub
	carryoverInFlight                  map[carryoverSourceKey]int
	actionPending                      map[string]bool
	graph                              core.TaskGraph
	progress                           *runtimeProgressCache
	today                              core.LocalDate
	store                              titleStore
	coordinator                        *coordinator.Coordinator
	lane                               coordinator.Lane
	post                               func(func())
	now                                func() time.Time
	windows                            map[string]*titleWindow
	nextWindow, nextEditor, nextTarget uint64
	targets                            map[string]func()
	quit                               func()
	quitting                           bool
	dayQueued                          bool
	observedDay                        core.LocalDate
	stopped                            bool
}
type titleServiceAdapter struct{ hub *titleHub }

func (a titleServiceAdapter) AdvanceTo(d coordinator.Date) error {
	if a.hub.stopped {
		return taskapp.ErrSession
	}
	err := a.hub.store.AdvanceTo(core.LocalDate{Year: d.Year, Month: d.Month, Day: d.Day})
	a.hub.stopOnError(err)
	a.hub.publishFromLane()
	return err
}
func (a titleServiceAdapter) SaveTraceTitleInputAcknowledged(trace, title string) (string, error) {
	text, err := a.hub.saveTitleAcknowledged(trace, title)
	a.hub.stopOnError(err)
	a.hub.publishFromLane()
	return text, err
}
func newTitleHub(store titleStore, lane coordinator.Lane, post func(func()), clock coordinator.Clock, now func() time.Time, quit func()) *titleHub {
	h := &titleHub{today: store.Today(), store: store, lane: lane, post: post, now: now, quit: quit, windows: map[string]*titleWindow{}, targets: map[string]func(){}, observedDay: store.Today()}
	h.categoryClock = clock
	h.progress = newRuntimeProgressCache(store.Snapshot())
	h.graph = h.progress.graph
	c, err := coordinator.New(coordinator.Config{
		Service: titleServiceAdapter{h}, Lane: lane, Post: post, Clock: clock,
		Today: func() coordinator.Date {
			d := localTitleDate(now())
			return coordinator.Date{Year: d.Year, Month: d.Month, Day: d.Day}
		},
		Permanent: func(e error) bool {
			return errors.Is(e, taskapp.ErrConflict) || errors.Is(e, taskapp.ErrIncomplete) || errors.Is(e, taskapp.ErrSession)
		},
		TraceLocked: func(e error) bool {
			return errors.Is(e, core.ErrImmutableHistory) || errors.Is(e, core.ErrInvalidTransition) || errors.Is(e, core.ErrNotFound)
		},
		Changed: h.changed, Failed: h.failed,
		Navigate: func(window, target string) {
			fn := h.targets[target]
			delete(h.targets, target)
			w := h.windows[window]
			if w == nil {
				return
			}
			w.app.titleEditor = nil
			w.app.subtaskEditors = nil
			// The coordinator has disposed confirmed title registrations. Category
			// native input may have arrived while their save was in flight.
			if !h.categoryCanNavigate(w.app) {
				if w.app.titleTarget == target {
					w.app.titleTarget = ""
				}
				w.invalidate()
				return
			}
			if w.app.titleTarget == target {
				w.app.titleTarget = ""
			} else {
				fn = nil
			}
			if fn != nil {
				fn()
			}
			w.invalidate()
		},
		ReenterClose: func(window string) {
			if w := h.windows[window]; w != nil {
				w.close()
			}
		}, ReenterQuit: quit,
	})
	if err != nil {
		panic(err)
	}
	h.coordinator = c
	return h
}
func localTitleDate(t time.Time) core.LocalDate {
	return core.LocalDate{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}
func (h *titleHub) attach(s *app, invalidate, close func()) {
	h.nextWindow++
	s.titleWindowID = fmt.Sprint(h.nextWindow)
	s.titles = h
	s.runtime = h.readView()
	s.progress = h.progress
	if h.progress.err != nil {
		s.runtimeError = progressFailureMessage
	}
	h.windows[s.titleWindowID] = &titleWindow{app: s, invalidate: invalidate, close: close}
}

// Preserve deterministic registration/publication order across surfaces. A child
// has its own immutable object route, while every native surface owns a binding.
func (s *app) titleEditors() []*runtimeTitleEditor {
	var out []*runtimeTitleEditor
	if s.titleEditor != nil {
		out = append(out, s.titleEditor)
	}
	keys := make([]string, 0, len(s.subtaskEditors))
	for key := range s.subtaskEditors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, s.subtaskEditors[key])
	}
	return out
}
func (h *titleHub) changed(reg coordinator.Registration, state titlesession.State) {
	for _, w := range h.windows {
		for _, e := range w.app.titleEditors() {
			if !e.registered || e.reg != reg {
				continue
			}
			before := e.state
			e.state = state
			if before.InFlight && !state.InFlight && !state.Dirty && state.LastError == nil && (e.failure == nil || before.LastError != nil && errors.Is(e.failure, before.LastError)) {
				e.failure = nil
				if w.app.runtimeError == titleFailureMessage && !w.app.hasTitleFailure() {
					w.app.runtimeError = ""
					w.app.runtimeNotice = "已保存到隔离开发会话"
				}
			}
			e.identity = state.Identity
			e.key = titleEditorKey{state.Identity.OwnerID(), e.key.Session, state.Identity.Generation()}
			if before != state {
				w.invalidate()
			}
		}
	}
}
func (s *app) hasTitleFailure() bool {
	for _, e := range s.titleEditors() {
		if e.failure != nil {
			return true
		}
	}
	return false
}
func (h *titleHub) failed(reg coordinator.Registration, err error) {
	for _, w := range h.windows {
		for _, e := range w.app.titleEditors() {
			if e.reg != reg {
				continue
			}
			delete(h.targets, w.app.titleTarget)
			w.app.titleTarget = ""
			e.failure = err
			w.app.runtimeError = titleFailureMessage
			w.app.runtimeNotice = ""
			w.invalidate()
		}
	}
}
func (h *titleHub) register(s *app) bool {
	for _, e := range s.titleEditors() {
		if !h.registerEditor(s, e) {
			return false
		}
	}
	return true
}
func (h *titleHub) registerEditor(s *app, e *runtimeTitleEditor) bool {
	if e == nil || e.registered {
		return true
	}
	if _, ok := e.native.Snapshot(); !ok {
		s.runtimeError = "标题输入尚未能安全读取，已取消离开。"
		return false
	}
	owner, target, persisted := "task:"+string(e.row.ChainID)+":title", string(e.row.TraceID), e.row.Title
	if e.subtask != nil {
		owner, target, persisted = e.owner, e.subtask.target.route(), e.subtask.title
	}
	reg, err := h.coordinator.Open(s.titleWindowID, owner, target, persisted, &e.native)
	if err != nil {
		s.runtimeError = "标题会话无法安全注册，草稿已保留。"
		return false
	}
	e.reg, e.registered = reg, true
	e.identity, _ = h.coordinator.Identity(reg)
	e.state, _ = h.coordinator.State(reg)
	e.key = titleEditorKey{e.identity.OwnerID(), e.key.Session, e.identity.Generation()}
	_ = h.coordinator.Observe(reg, e.identity)
	return true
}
func (s *app) titleTransition(fn func()) {
	s.titleDisposal = nil
	if s.titles == nil {
		fn()
		return
	}
	h := s.titles
	if !h.categoryCanNavigate(s) {
		return
	}
	if w := h.windows[s.titleWindowID]; w != nil {
		w.navigation++
	}
	h.nextTarget++
	target := fmt.Sprint(h.nextTarget)
	// Only the newest queued same-window navigation owns its closure.
	delete(h.targets, s.titleTarget)
	s.titleTarget = target
	h.targets[target] = fn
	h.post(func() {
		if h.windows[s.titleWindowID] == nil || s.titleTarget != target {
			delete(h.targets, target)
			return
		}
		if !h.categoryCanNavigate(s) || !h.register(s) || !s.finalizeSubtaskComposers() {
			delete(h.targets, target)
			if s.titleTarget == target {
				s.titleTarget = ""
			}
			return
		}
		h.coordinator.RequestNavigation(s.titleWindowID, target)
	})
}
func (s *app) liveTitle(c *ui.Context, width float32, row taskview.DayRow) {
	h := s.titles
	editable := row.Status == core.TracePending && s.runtimeDate.Compare(s.runtime.Today()) >= 0
	if h == nil || h.windows[s.titleWindowID] == nil || (!editable && s.titleEditor == nil) || (s.titleEditor == nil && (h.windows[s.titleWindowID].commands > 0 || s.livePickerOpen())) {
		title := row.Title
		if title == "" {
			title = "无标题"
		}
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(ink).Width(width).Label("日任务详情标题")
		return
	}
	if s.titleEditor == nil {
		h.nextEditor++
		s.titleEditor = &runtimeTitleEditor{row: row, native: titleNative{draft: row.Title}, seenText: row.Title, key: titleEditorKey{"task:" + string(row.ChainID) + ":title", h.nextEditor, 1}}
	}
	e := s.titleEditor
	e.native.context = c
	control := ui.TextAreaBase(c.Key(e.key), &e.native.draft).Bind(&e.native.handle).Width(width).FontSize(14).FontWeight(600).TextColor(ink).Padding(3, 0).Lines(1, 1000).MinHeight(22).MaxHeight(72).Background(ui.Transparent).Border(0, ui.Transparent).Label("日任务详情标题").Placeholder("任务标题")
	if !e.registered && !e.queued {
		e.queued = true
		h.post(func() {
			if s.titleEditor == e {
				h.register(s)
			}
		})
	}
	text, compose := e.native.draft, control.Composing()
	if e.registered && (text != e.seenText || compose != e.seenCompose) {
		reg, id := e.reg, e.identity
		h.post(func() { _ = h.coordinator.Observe(reg, id) })
	}
	e.seenText, e.seenCompose = text, compose
	if e.registered && control.FocusLeft() {
		reg, id := e.reg, e.identity
		h.post(func() {
			if current, ok := h.coordinator.Identity(reg); ok && current == id {
				_ = h.coordinator.EndEditing(reg)
			}
		})
	}
}
func (h *titleHub) beforeClose(s *app) bool {
	w := h.windows[s.titleWindowID]
	if w == nil {
		return true
	}
	if !h.register(s) || !s.finalizeSubtaskComposers() || !s.subtaskComposersCanClose() || !h.carryoverInputsCanClose(s) || !h.categoryCanClose(s) {
		w.invalidate()
		return false
	}
	if w.commands > 0 {
		w.closing = true
		return false
	}
	if !h.coordinator.BeforeClose(s.titleWindowID) {
		return false
	}
	if err := h.coordinator.ReleaseClosed(s.titleWindowID); err != nil {
		return false
	}
	delete(h.targets, s.titleTarget)
	s.titleTarget = ""
	delete(h.windows, s.titleWindowID)
	s.titleEditor = nil
	s.subtaskEditors = nil
	return true
}
func (h *titleHub) beforeQuit() bool {
	for _, w := range h.windows {
		if !h.register(w.app) || !w.app.finalizeSubtaskComposers() || !w.app.subtaskComposersCanClose() || !h.carryoverInputsCanClose(w.app) || !h.categoryCanClose(w.app) {
			w.invalidate()
			return false
		}
	}
	for _, w := range h.windows {
		if w.commands > 0 {
			h.quitting = true
			return false
		}
	}
	return h.coordinator.BeforeQuit()
}

// The graph is sampled by the one writer lane, then delivered in the same FIFO
// as title completions. No delayed unordered readback becomes acknowledged text.
func (h *titleHub) publishFromLane() {
	if h.category != nil {
		view, err := h.category.store.ReadView()
		h.post(func() {
			if err != nil {
				for _, w := range h.windows {
					w.app.runtimeError = categoryFailureMessage
					w.invalidate()
				}
				return
			}
			h.category.view = view
			h.publish(view.Graph, view.Today)
		})
		return
	}
	graph, today := h.store.Snapshot(), h.store.Today()
	h.post(func() { h.publish(graph, today) })
}
func (h *titleHub) publish(graph core.TaskGraph, today core.LocalDate) {
	previous, previousToday := h.graph, h.today
	if !h.progress.publish(graph) {
		for _, w := range h.windows {
			w.app.runtimeError = progressFailureMessage
			w.invalidate()
		}
		return
	}
	h.graph = h.progress.graph
	h.today = today
	for _, w := range h.windows {
		s := w.app
		if s.runtimeError == progressFailureMessage {
			s.runtimeError = ""
		}
		e := s.titleEditor
		if e != nil && e.registered {
			var trace *core.DayTrace
			for i := range graph.Traces {
				if graph.Traces[i].ID == e.row.TraceID {
					trace = &graph.Traces[i]
					break
				}
			}
			if trace != nil {
				e.row.Status = trace.Status // Refresh facts, never the retained native draft.
				text := currentTitle(graph, trace.ChainID, trace.DefinitionID)
				_ = h.coordinator.SynchronizePersistedText(e.reg, e.identity, text)
			}
			if trace == nil || trace.Status != core.TracePending || trace.Date.Compare(today) < 0 {
				if !e.locked {
					e.locked = true
					if err := h.coordinator.LockTrace(e.reg); err != nil {
						s.runtimeError = "这个标题已成为历史，未保存草稿仍保留在原任务。"
					}
				}
				state, ok := h.coordinator.State(e.reg)
				if ok && !state.Dirty && !state.InFlight && !state.AwaitingVerification && state.LastError == nil {
					// Disposal still requires a fresh native proof; do not simply remove it.
					s.disposeTitleEditors()
				}
			}
		}
		s.preserveGuardedComposerOwners(previous, previousToday, graph, today)
		h.publishSubtaskEditors(s, graph, today)
		if today != s.runtimeAppliedDay {
			if len(s.titleEditors()) == 0 && s.runtimeDate == s.runtimeAppliedDay && s.automaticOwnerExitAllowed() {
				s.runtimeDate = today
				s.runtimeSelection = ""
				s.runtimeDayFocus = ""
			}
			if s.runtimeCalendarDate == s.runtimeAppliedDay {
				s.runtimeCalendarDate = today
				s.runtimeCalendarFocus = ""
			}
			s.runtimeAppliedDay = today
		}
		w.invalidate()
	}
}
func (h *titleHub) observeDay() {
	today := localTitleDate(h.now())
	if h.dayQueued || today == h.observedDay {
		return
	}
	h.dayQueued = true
	h.observedDay = today // A failed boundary is not retried by its own error redraw.
	h.post(func() {
		// Finalize existing input before advancing the domain. A dirty old trace is
		// NOT backfilled after midnight; the service will reject and retain it.
		for _, w := range h.windows {
			if !h.register(w.app) || !w.app.finalizeSubtaskComposers() {
				h.dayQueued = false
				return
			}
			for _, e := range w.app.titleEditors() {
				if _, ok := e.native.Finish(); !ok {
					h.dayQueued = false
					return
				}
				_ = h.coordinator.Observe(e.reg, e.identity)
			}
		}
		h.lane.Enqueue(func() {
			d := localTitleDate(h.now())
			var err error
			if h.stopped {
				err = taskapp.ErrSession
			} else {
				err = h.store.AdvanceTo(d)
			}
			h.stopOnError(err)
			h.publishFromLane()
			h.post(func() {
				h.dayQueued = false
				if err == nil {
					h.observedDay = d
				} else {
					for _, w := range h.windows {
						w.app.runtimeError = naturalDayFailureMessage
						w.invalidate()
					}
				}
			})
		})
	})
}
func (h *titleHub) stopOnError(err error) {
	if errors.Is(err, taskapp.ErrConflict) || errors.Is(err, taskapp.ErrIncomplete) || errors.Is(err, taskapp.ErrSession) {
		h.stopped = true
	}
}

// Immutable action keys suppress repeated activation of the same pending
// operation, not unrelated commands or a later intentional activation.
func (h *titleHub) commandOnce(s *app, key string, fn func() error, done func(error)) bool {
	return h.commandOnceAdmitted(s, key, nil, fn, done)
}

// Admission is after deduplication but before dispatch can invoke completion.
func (h *titleHub) commandOnceAdmitted(s *app, key string, admitted func(), fn func() error, done func(error)) bool {
	w := h.windows[s.titleWindowID]
	if w == nil {
		return false
	}
	if h.actionPending == nil {
		h.actionPending = map[string]bool{}
	}
	if h.actionPending[key] {
		return false
	}
	h.actionPending[key] = true
	if admitted != nil {
		admitted()
	}
	h.command(s, fn, false, func(err error) {
		delete(h.actionPending, key)
		if done != nil {
			done(err)
		}
	})
	return true
}
func (s *app) runAction(key string, fn func() error) {
	if s.titles == nil {
		s.runCommand(fn, false)
		return
	}
	s.titles.commandOnce(s, key, fn, nil)
}
func (h *titleHub) command(s *app, fn func() error, clear bool, completion func(error)) {
	rawDraft := s.draft
	w := h.windows[s.titleWindowID]
	if w == nil {
		return
	}
	if clear {
		if w.inputPending == nil {
			w.inputPending = map[string]bool{}
		}
		if w.inputPending[rawDraft] {
			return
		}
		w.inputPending[rawDraft] = true
	}
	w.commands++
	h.post(func() {
		h.lane.Enqueue(func() {
			var err error
			if h.stopped {
				err = taskapp.ErrSession
			} else {
				err = h.store.AdvanceTo(localTitleDate(h.now()))
				if err == nil {
					err = fn()
				}
			}
			if errors.Is(err, taskapp.ErrConflict) || errors.Is(err, taskapp.ErrIncomplete) || errors.Is(err, taskapp.ErrSession) {
				h.stopped = true
			}
			h.publishFromLane()
			h.post(func() {
				w.commands--
				if clear {
					delete(w.inputPending, rawDraft)
				}
				s.commandResult(err, clear && s.draft == rawDraft)
				if completion != nil {
					completion(err)
				}
				w.invalidate()
				if err != nil {
					w.closing = false
					h.quitting = false
					return
				}
				if w.closing && w.commands == 0 {
					w.closing = false
					w.close()
				}
				if h.quitting {
					h.quitting = false
					h.quit()
				}
			})
		})
	})
}

// A worker is the only caller of RunOnMain: it queues behind the current frame.
// Neither builds nor UI callbacks wait on this lane. RunOnMain does not redraw.
func newTitleDispatch() (*coordinator.SerialLane, func(func())) {
	lane := coordinator.NewSerialLane()
	return lane, func(fn func()) { lane.Enqueue(func() { mygo.RunOnMain(fn) }) }
}
