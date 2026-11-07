// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"strings"
)

const deferTaskLabel = "延期到…"
const continueTaskLabel = "延续到…"

type carryoverStore interface {
	DeferCurrentTrace(core.TaskChainID, core.DayTraceID, core.LocalDate) (core.DayTraceID, error)
	ContinueUnfinishedTrace(core.TaskChainID, core.DayTraceID, core.LocalDate) (core.DayTraceID, error)
}

type carryoverSourceKey struct {
	chain core.TaskChainID
	trace core.DayTraceID
}

type carryoverPresentation struct {
	picker          *carryoverPicker
	hub             *titleHub
	window          *titleWindow
	navigation      uint64
	page, selection string
	date            core.LocalDate
	pending         bool
}

func (s *app) livePickerOpen() bool { return s.runtimePoolPicker || s.carryover != nil }
func (s *app) carryoverCurrent(p *carryoverPresentation) bool {
	return p != nil && s.carryover == p && s.titles == p.hub && p.hub.windows[s.titleWindowID] == p.window && p.window.navigation == p.navigation && s.page == p.page && s.runtimeDate == p.date && s.runtimeSelection == p.selection
}

// The source Day menu deliberately does not test frontier or pool location.
// Those are mutation constraints, distinct from the Unfinished page actionPlan.
func carryoverMenuCapabilities(g core.TaskGraph, row taskview.DayRow, today core.LocalDate) (deferable, continuable bool) {
	var source *core.DayTrace
	for i := range g.Traces {
		t := &g.Traces[i]
		if t.ID == row.TraceID && t.ChainID == row.ChainID && t.Status == row.Status {
			source = t
			break
		}
	}
	if source == nil {
		return
	}
	ordinary, active := false, false
	for _, c := range g.Chains {
		if c.ID == row.ChainID {
			ordinary = c.CycleMembership == nil
			active = c.State == core.ChainActive
			break
		}
	}
	if !ordinary {
		return
	}
	locked := false
	for _, d := range g.Days {
		if d.Date == source.Date {
			locked = d.LockedAt != nil
			break
		}
	}
	deferable = source.Date == today && !locked && source.Status == core.TracePending
	continuable = (source.Date.Compare(today) < 0 || locked) && source.Status == core.TraceUnfinished && active
	if continuable {
		for _, t := range g.Traces {
			if t.ChainID == source.ChainID && t.Status == core.TracePending {
				continuable = false
				break
			}
		}
	}
	return
}
func (s *app) openCarryover(kind carryoverPickerKind, row taskview.DayRow) {
	h := s.titles
	if h == nil || s.page != "day" {
		return
	}
	w := h.windows[s.titleWindowID]
	if w == nil {
		return
	}
	if _, ok := h.store.(carryoverStore); !ok {
		return
	}
	page, date, selection := s.page, s.runtimeDate, s.runtimeSelection
	s.titleTransition(func() {
		if s.titles != h || h.windows[s.titleWindowID] != w || s.page != page || s.runtimeDate != date || s.runtimeSelection != selection {
			return
		}
		if !s.carryoverComposersReady(row.ChainID, row.TraceID) {
			return
		}
		d, c := carryoverMenuCapabilities(h.graph, row, h.today)
		if kind == carryoverDefer && !d || kind == carryoverContinue && !c {
			return
		}
		s.carryoverEpoch++
		id := carryoverPickerIdentity{Kind: kind, Window: s.titleWindowID, Session: fmt.Sprintf("%p/%p", h, w), SourceContext: fmt.Sprintf("%s/%v/%s/%d", page, date, selection, w.navigation), Chain: row.ChainID, Trace: row.TraceID, Presentation: s.carryoverEpoch}
		picker, err := newCarryoverPicker(id, h.today)
		if err != nil {
			s.commandResult(err, false)
			return
		}
		s.runtimePoolPicker = false
		s.carryover = &carryoverPresentation{picker: picker, hub: h, window: w, navigation: w.navigation, page: page, date: date, selection: selection}
	})
}
func (s *app) liveCarryoverPicker(c *ui.Context) {
	p := s.carryover
	if p == nil {
		return
	}
	if !s.carryoverCurrent(p) {
		s.carryover = nil
		return
	}
	if intent := p.picker.drawPending(c, p.picker.identity, s.runtime.Today(), p.pending); intent != nil {
		s.performCarryover(p, *intent)
	}
}
func (s *app) performCarryover(p *carryoverPresentation, intent carryoverPickerIntent) {
	if !s.carryoverCurrent(p) || intent.Identity != p.picker.identity {
		return
	}
	if intent.Cancelled {
		s.carryover = nil
		return
	}
	// Component emits only a draft intent. Keep the original sheet alive until
	// its native barrier and audited asynchronous command have completed.
	p.picker.open = true
	if p.pending {
		return
	}
	if !intent.Identity.allows(intent.Date, s.runtime.Today()) {
		p.picker.validation = intent.Identity.hint()
		return
	}
	h, w := p.hub, p.window
	s.titleTransition(func() {
		if !s.carryoverCurrent(p) {
			return
		}
		if !intent.Identity.allows(intent.Date, h.today) {
			p.picker.validation = intent.Identity.hint()
			return
		}
		if !s.carryoverComposersReady(intent.Identity.Chain, intent.Identity.Trace) {
			return
		}
		store, ok := h.store.(carryoverStore)
		if !ok {
			return
		}
		p.picker.selected = intent.Date
		p.picker.month = pickerMonth(intent.Date)
		var acknowledged core.DayTraceID
		source := carryoverSourceKey{intent.Identity.Chain, intent.Identity.Trace}
		key := fmt.Sprintf("carryover:%d:%s:%s:%v", intent.Identity.Kind, intent.Identity.Chain, intent.Identity.Trace, intent.Date)
		p.pending = h.commandOnceAdmitted(s, key, func() {
			if h.carryoverInFlight == nil {
				h.carryoverInFlight = map[carryoverSourceKey]int{}
			}
			h.carryoverInFlight[source]++
		}, func() (err error) {
			if intent.Identity.Kind == carryoverDefer {
				acknowledged, err = store.DeferCurrentTrace(intent.Identity.Chain, intent.Identity.Trace, intent.Date)
			} else {
				acknowledged, err = store.ContinueUnfinishedTrace(intent.Identity.Chain, intent.Identity.Trace, intent.Date)
			}
			return
		}, func(err error) {
			if err != nil {
				h.protectFailedCarryoverSource(source)
			}
			h.carryoverInFlight[source]--
			if h.carryoverInFlight[source] <= 0 {
				delete(h.carryoverInFlight, source)
			}
			if !s.carryoverCurrent(p) {
				return
			}
			s.carryover = nil
			if err != nil || acknowledged == "" || w.closing || h.quitting {
				return
			}
			// Only Day origin is exposed. A future Unfinished origin must keep its page.
			if p.page != "unfinished" && s.automaticOwnerExitAllowed() {
				s.page = "day"
				s.runtimeDate = intent.Date
				s.runtimeSelection = string(acknowledged)
			}
		})
	})
	// titleTransition increments synchronously. This is the new submission fence;
	// later navigation/close/quit invalidates it before any completion can act.
	p.navigation = w.navigation
}

// Unlike an ordinary page switch, carryover makes the original trace read-only.
// Never auto-submit a transient draft or strand it behind an immutable source.
func (s *app) carryoverComposersReady(chain core.TaskChainID, trace core.DayTraceID) bool {
	for _, w := range s.titles.windows {
		for _, c := range w.app.subtaskComposers {
			if c.target.chain != chain || c.target.trace != trace {
				continue
			}
			// Explicit native blank is consent to discard a failed Add, even
			// when that composer belongs to another window. Pending/ack flags
			// are never cleared by this readiness check.
			snapshot, ok := titlesession.NativeSnapshot{Text: c.finalizedText}, c.finalizedValid
			if c.mounted {
				snapshot, ok = c.native.Snapshot()
				if ok && !snapshot.Composing && strings.TrimSpace(snapshot.Text) == "" && !c.pending && !c.hasAcknowledged {
					c.failure = nil
					c.carryoverRecovery = carryoverRecoveryNone
				}
			}
			blocked := c.pending || c.failure != nil || c.hasAcknowledged || !ok || snapshot.Composing || strings.TrimSpace(snapshot.Text) != ""

			if blocked {
				s.runtimeError = "这个任务仍有未确认的子任务输入，请先完成或清空原窗口的输入。草稿已保留。"
				s.runtimeNotice = ""
				return false
			}
		}
	}
	return true
}

type carryoverRecoveryReason uint8

const (
	carryoverRecoveryNone carryoverRecoveryReason = iota
	carryoverRecoveryUnconfirmed
	carryoverRecoverySourceSaved
)
const carryoverRecoveryMessage = "延期已保存；原任务的子任务输入仍需处理，草稿已保留。"
const carryoverUnconfirmedMessage = "本次日期操作尚未确认；原任务输入仍需处理，草稿已保留。"

func carryoverSourceDeferred(g core.TaskGraph, source carryoverSourceKey) bool {
	for _, tr := range g.Traces {
		if tr.ChainID == source.chain && tr.ID == source.trace {
			return tr.Status == core.TraceDeferred
		}
	}
	return false
}
func (s *app) retainCarryoverComposer(c *runtimeSubtaskComposer, snapshot titlesession.NativeSnapshot, ok bool, reason carryoverRecoveryReason) {
	if !ok || snapshot.Composing || strings.TrimSpace(snapshot.Text) != "" || c.pending || c.hasAcknowledged || c.failure != nil {
		if c.carryoverRecovery != carryoverRecoverySourceSaved {
			c.carryoverRecovery = reason
		}
		s.refreshCarryoverRecoveryNotice()
	}
}

// UI-thread publication owns every guarded editable->noneditable transition,
// using the same predicate as rendering, before a frame can remove the node.
func (s *app) preserveGuardedComposerOwners(previous core.TaskGraph, previousToday core.LocalDate, g core.TaskGraph, today core.LocalDate) {
	for _, c := range s.subtaskComposers {
		source := carryoverSourceKey{c.target.chain, c.target.trace}
		if s.titles.carryoverInFlight[source] == 0 && c.carryoverRecovery == carryoverRecoveryNone {
			continue
		}
		saved := carryoverSourceDeferred(g, source)
		if c.carryoverRecovery != carryoverRecoveryNone {
			if saved {
				c.carryoverRecovery = carryoverRecoverySourceSaved
				s.refreshCarryoverRecoveryNotice()
			}
			continue // stable recovery owner: do not revoke its new editing lease.
		}
		if !actualSubtaskTargetEditable(previous, previousToday, c.target) || actualSubtaskTargetEditable(g, today, c.target) {
			continue
		}
		snapshot, ok := titlesession.NativeSnapshot{Text: c.finalizedText}, c.finalizedValid
		if c.mounted {
			snapshot, ok = c.native.Finish()
			c.finalizedText, c.finalizedValid = snapshot.Text, ok && !snapshot.Composing
		}
		reason := carryoverRecoveryUnconfirmed
		if saved {
			reason = carryoverRecoverySourceSaved
		}
		s.retainCarryoverComposer(c, snapshot, ok, reason)
	}
}

// Failed writes leave the confirmed editable owner mounted. Snapshot identifies
// content to retain; it is not a finalization/lease-revocation proof. Close and
// navigation still require Finish. Never finalize a stable recovery owner again.
func (h *titleHub) protectFailedCarryoverSource(source carryoverSourceKey) {
	if carryoverSourceDeferred(h.graph, source) {
		return
	}
	for _, w := range h.windows {
		for _, c := range w.app.subtaskComposers {
			if c.target.chain != source.chain || c.target.trace != source.trace || c.carryoverRecovery != carryoverRecoveryNone {
				continue
			}
			snapshot, ok := titlesession.NativeSnapshot{Text: c.finalizedText}, c.finalizedValid
			if c.mounted {
				snapshot, ok = c.native.Snapshot()
			}
			w.app.retainCarryoverComposer(c, snapshot, ok, carryoverRecoveryUnconfirmed)
		}
	}
}
func (s *app) refreshCarryoverRecoveryNotice() {
	message := ""
	for _, c := range s.failedSubtaskComposers("") {
		if c.carryoverRecovery == carryoverRecoveryUnconfirmed {
			message = carryoverUnconfirmedMessage
			break
		}
		if c.carryoverRecovery == carryoverRecoverySourceSaved {
			message = carryoverRecoveryMessage
		}
	}
	if s.runtimeError == "" || s.runtimeError == subtaskComposerFailureMessage || s.runtimeError == carryoverRecoveryMessage || s.runtimeError == carryoverUnconfirmedMessage {
		if message != "" {
			s.runtimeError = message
			s.runtimeNotice = ""
		} else if s.runtimeError == carryoverRecoveryMessage || s.runtimeError == carryoverUnconfirmedMessage {
			s.runtimeError = ""
		}
	}
}

func (s *app) deferredComposerRecovery() *runtimeSubtaskComposer {
	for _, c := range s.failedSubtaskComposers("") {
		if c.carryoverRecovery != carryoverRecoveryNone {
			return c
		}
	}
	return nil
}
func (s *app) liveCarryoverRecoveryLink(c *ui.Context, height, width float32, g core.TaskGraph) {
	// A permanent keyed host keeps conditional recovery controls from changing
	// the native editor ancestor scope in the rest of the same window.
	ui.Box(c.Key("carryover-recovery-anchor")).Absolute().Left(0).Top(height-76).Size(width, 28).Children(func() {
		s.liveCarryoverRecoveryLinkContent(c, height, width, g)
	})
}
func (s *app) liveCarryoverRecoveryLinkContent(c *ui.Context, height, width float32, g core.TaskGraph) {
	composer := s.deferredComposerRecovery()
	if composer == nil {
		return
	}
	for _, tr := range g.Traces {
		if tr.ID != composer.target.trace || tr.ChainID != composer.target.chain {
			continue
		}
		source := tr
		label := fmt.Sprintf("原任务草稿 · %d月%d日", tr.Date.Month, tr.Date.Day)
		if width <= 100 {
			label = "草稿"
		}
		liveAction(c, "carryover-recovery", 12, 0, width-24, label, "查看原任务子任务草稿", true, func() {
			s.titleTransition(func() {
				valid := false
				if composer.carryoverRecovery == carryoverRecoveryNone {
					return
				}
				for _, current := range s.titles.graph.Traces {
					if current.ID == source.ID && current.ChainID == source.ChainID && current.Date == source.Date {
						valid = true
						break
					}
				}
				if !valid {
					return
				}
				s.page = "day"
				s.runtimeDate = source.Date
				s.runtimeSelection = string(source.ID)
				s.runtimeDayRail = true
			})
		})
		return
	}
}

// Called after the window's native Finish barriers, before detaching it or
// recording an automatic close/quit intent. Admission protects all windows
// sharing this source until the writer's UI completion, even if its sheet was
// cancelled. Unsubmitted text is never silently treated as an Add or discarded.
func (h *titleHub) carryoverInputsCanClose(s *app) bool {
	for _, c := range s.subtaskComposers {
		if h.carryoverInFlight[carryoverSourceKey{c.target.chain, c.target.trace}] == 0 {
			continue
		}
		if c.pending || c.hasAcknowledged || c.failure != nil || !c.finalizedValid || strings.TrimSpace(c.finalizedText) != "" {
			s.runtimeError = "任务变更尚未完成，原任务输入已保留。请等待结果，或明确清空原输入后再关闭。"
			s.runtimeNotice = ""
			if w := h.windows[s.titleWindowID]; w != nil {
				w.closing = false
			}
			h.quitting = false
			return false
		}
	}
	return true
}

// Automatic navigation cannot retire a protected native owner using an old
// admission-time proof. Publication may Finish it; a later mount invalidates
// that proof. This check never repeats Finish or changes the persisted outcome.
func (s *app) automaticOwnerExitAllowed() bool {
	if s.titles == nil {
		return true
	}
	if !s.titles.categoryCanNavigate(s) {
		return false
	}
	for _, c := range s.subtaskComposers {
		source := carryoverSourceKey{c.target.chain, c.target.trace}
		if (s.titles.carryoverInFlight[source] > 0 || c.carryoverRecovery != carryoverRecoveryNone) && !c.finalizedValid {
			return false
		}
	}
	return true
}
