// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

func admissionRejectHop(f *titleFixture, hop string) *titleTestQueue {
	q := &titleTestQueue{}
	if hop == "producer" {
		f.h.tryPostProducer = func(func()) bool { return false }
	} else {
		f.h.tryPostProducer = func(fn func()) bool { q.Enqueue(fn); return true }
		f.h.trySubmit = func(func(func(func()) bool)) bool { return false }
	}
	return q
}

func TestRuntimeAdmissionCommandRejectSettlesOriginalTicket(t *testing.T) {
	for _, hop := range []string{"producer", "writer"} {
		t.Run(hop, func(t *testing.T) {
			f := newTitleFixture(t, true)
			q := admissionRejectHop(f, hop)
			f.s.draft = "保留原输入"
			w := f.h.windows[f.s.titleWindowID]
			calls, completions := 0, 0
			f.h.command(f.s, func() error { calls++; return nil }, true, func(err error) {
				completions++
				if !errors.Is(err, coordinator.ErrNotAdmitted) {
					t.Errorf("wrong rejection: %v", err)
				}
			})
			if hop == "writer" {
				if w.commands != 1 || !w.inputPending["保留原输入"] {
					t.Fatal("first hop lost ticket")
				}
				q.one()
			}
			if calls != 0 || completions != 1 || w.commands != 0 || len(w.inputPending) != 0 || f.s.draft != "保留原输入" || len(f.io.q) != 0 {
				t.Fatalf("calls=%d completions=%d commands=%d pending=%v draft=%q", calls, completions, w.commands, w.inputPending, f.s.draft)
			}
			if !f.h.commandOnce(f.s, "action", func() error { calls++; return nil }, func(err error) { completions++ }) {
				t.Fatal("action refused before its ticket")
			}
			if hop == "writer" {
				q.one()
			}
			if f.h.actionPending["action"] || completions != 2 || calls != 0 || w.commands != 0 {
				t.Fatal("outer action ticket leaked")
			}
		})
	}
}

func TestRuntimeAdmissionCategoryRejectPreservesNativeDraft(t *testing.T) {
	for _, hop := range []string{"producer", "writer"} {
		t.Run(hop, func(t *testing.T) {
			f := newCategoryFixture(t, true)
			e := categoryBegin(t, f, "不能丢失的分组")
			before, err := f.svc.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			handle, context, generation := e.native.handle, e.native.context, e.generation
			q := admissionRejectHop(f, hop)
			f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
			if hop == "writer" {
				if !e.pending {
					t.Fatal("missing accepted producer pending")
				}
				q.one()
			}
			w := f.h.windows[f.s.titleWindowID]
			if e.pending || w.commands != 0 || !e.creating || !errors.Is(e.failure, coordinator.ErrNotAdmitted) || e.native.draft != "不能丢失的分组" || e.generation != generation || !reflect.DeepEqual(e.native.handle, handle) || e.native.context != context || !e.savedUntil.IsZero() {
				t.Fatalf("rejection changed editor: %+v commands=%d", e, w.commands)
			}
			after, err := f.svc.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || len(f.io.q) != 0 {
				t.Fatal("rejected category changed durable facts")
			}
		})
	}
}

func TestRuntimeAdmissionCategoryOldCompletionSettlesOldEditorOnly(t *testing.T) {
	f := newCategoryFixture(t, true)
	old := categoryBegin(t, f, "原分组")
	f.s.submitCategory(old, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.ui()
	if len(f.io.q) != 1 || !old.pending {
		t.Fatal("category did not reach writer")
	}
	f.io.one()
	w := f.h.windows[f.s.titleWindowID]
	newer := &runtimeCategoryEditor{chain: old.chain, generation: old.generation + 1, creating: true, pending: true, native: titleNative{draft: "后来的草稿"}}
	w.category = newer
	// Consume old result without rendering/rebinding the new editor.
	for len(f.posts.q) > 0 {
		f.posts.one()
	}
	if old.pending || w.commands != 0 || !newer.pending || !newer.creating || newer.native.draft != "后来的草稿" || newer.failure != nil || old.native.draft != "原分组" {
		t.Fatalf("old/new tickets conflated: old=%+v new=%+v commands=%d", old, newer, w.commands)
	}
}

func TestRuntimeAdmissionObserveDayRejectResetsQueueWithoutWrite(t *testing.T) {
	for _, hop := range []string{"producer", "writer"} {
		t.Run(hop, func(t *testing.T) {
			f := newTitleFixture(t, false)
			before := f.svc.Snapshot()
			today := f.svc.Today()
			q := admissionRejectHop(f, hop)
			f.clock.now = f.clock.now.Add(24 * time.Hour)
			f.h.observeDay()
			if hop == "writer" {
				if !f.h.dayQueued {
					t.Fatal("no pending day")
				}
				q.one()
			}
			if f.h.dayQueued || f.svc.Today() != today || !reflect.DeepEqual(before, f.svc.Snapshot()) || len(f.io.q) != 0 {
				t.Fatal("day rejection wrote or leaked scheduling flag")
			}
			if f.h.observedDay != localTitleDate(f.clock.now) {
				t.Fatal("rejection self-retry guard lost")
			}
			f.h.observeDay()
			if len(q.q) != 0 {
				t.Fatal("rejected date retried itself")
			}
		})
	}
}

func TestRuntimeAdmissionNavigationRejectReleasesOnlyScheduling(t *testing.T) {
	f := newTitleFixture(t, true)
	original := f.s.titleEditor
	f.h.tryPostProducer = func(func()) bool { return false }
	called := 0
	f.s.titleTransition(func() { called++ })
	if f.s.titleTarget != "" || len(f.h.targets) != 0 || called != 0 || f.s.titleEditor != original {
		t.Fatal("navigation rejection changed owner or leaked target")
	}
	f.h.tryPostProducer = func(fn func()) bool { f.posts.Enqueue(fn); return true }
	f.s.titleTransition(func() { called++ })
	f.all()
	if called != 1 {
		t.Fatal("rejected navigation prevented a later valid transition")
	}
}

func TestRuntimeAdmissionUtilityCloseNeverClearsDraft(t *testing.T) {
	for _, clear := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep", true: "clear-requested"}[clear], func(t *testing.T) {
			f := newTitleFixture(t, false)
			f.s.page, f.s.draft = "quick-entry", "原输入"
			w := f.h.windows[f.s.titleWindowID]
			w.commands = 1
			attempts := 0
			f.s.closeWindow = func() {
				attempts++
				if f.s.draft != "原输入" {
					t.Fatal("draft cleared before veto")
				}
				if f.h.beforeClose(f.s) {
					t.Fatal("pending command did not veto")
				}
				f.s.draft = "拒绝后新输入"
			}
			f.s.dismissUtility("today", clear)
			if attempts != 1 || f.s.draft != "拒绝后新输入" || f.s.page != "quick-entry" {
				t.Fatal("close return restored/cleared draft or navigated")
			}
		})
	}
	for _, clear := range []bool{false, true} {
		s := &app{draft: "fallback", page: "quick-entry"}
		s.dismissUtility("today", clear)
		expected := "fallback"
		if clear {
			expected = ""
		}
		if s.draft != expected || s.page != "today" {
			t.Fatal("no-window fallback changed")
		}
	}
	s := &app{draft: "successful-close", page: "quick-entry"}
	s.closeWindow = func() {
		if s.draft != "successful-close" {
			t.Fatal("successful close pre-cleared")
		}
	}
	s.dismissUtility("today", true)
	if s.draft != "successful-close" || s.page != "quick-entry" {
		t.Fatal("successful close post-cleared")
	}
}

func TestRuntimeAdmissionUtilityRejectedClosePostRetainsDraft(t *testing.T) {
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(w, d, func(f func()) { f() })
	a.EndRun()
	s := &app{draft: "终止期间输入", page: "quick-entry"}
	closed := false
	s.closeWindow = func() {
		if a.TryPostProducer(func() { closed = true }) {
			t.Error("terminal close admitted")
		}
	}
	s.dismissUtility("today", true)
	if closed || s.draft != "终止期间输入" || s.page != "quick-entry" {
		t.Fatal("rejected close erased draft or pretended closed")
	}
	r := admissionTestFinish(a, w, d)
	if r.ProducerRejected != 1 {
		t.Fatalf("report=%+v", r)
	}
}

func TestRuntimeAdmissionNativeBusinessRefusalReleasesRun(t *testing.T) {
	b, _ := nativeBusinessTestProbe(t)
	b.trySubmit = func(func(func(func()) bool)) bool { return false }
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	if err := nativeBusinessWait(t, done); !errors.Is(err, coordinator.ErrNotAdmitted) {
		t.Fatalf("refusal=%v", err)
	}
	if len(b.report.Operations) != 0 || b.report.UIReadViewAcknowledged {
		t.Fatal("refusal executed or acknowledged business")
	}
}

func TestRuntimeAdmissionNativeBusinessCompletionRejectKeepsWrittenFacts(t *testing.T) {
	b, _ := nativeBusinessTestProbe(t)
	q := make(chan func(), 1)
	b.trySubmit = func(work func(func(func()) bool)) bool {
		q <- func() { work(func(func()) bool { return false }) }
		return true
	}
	done := make(chan error, 1)
	go func() { done <- b.run() }()
	nativeBusinessTakeCallback(t, q, done, "writer")()
	if err := nativeBusinessWait(t, done); err == nil || !strings.Contains(err.Error(), "completion delivery rejected") {
		t.Fatalf("delivery rejection=%v", err)
	}
	if len(b.report.Operations) != 2 || b.report.UIReadViewAcknowledged || b.report.PersistentFactsJSON != "" {
		t.Fatal("delivery failure rewrote worker outcome or fabricated UI acknowledgement")
	}
	if len(b.service.Snapshot().Chains) == 0 {
		t.Fatal("delivery rejection incorrectly represented completed writes as absent")
	}
	if err := b.PreClose(); err == nil {
		t.Fatal("missing UI acknowledgement passed pre-close")
	}
}

func TestRuntimeAdmissionObserveDayWriterRejectPreservesPreparedNativeResult(t *testing.T) {
	f := newTitleFixture(t, true)
	liveClick(t, f.tt, "日任务详情标题")
	titleDirectClient(t, f.tt).SetMarkedText(nil, "未完成组合", ui.TextInputRange{Start: 5, End: 5})
	e := f.s.titleEditor
	before, ok := e.native.Snapshot()
	if !ok || !before.Composing {
		t.Fatal("missing native marked-text fixture")
	}
	q := admissionRejectHop(f, "writer")
	var prepared titlesession.NativeSnapshot
	attempts := 0
	f.h.trySubmit = func(func(func(func()) bool)) bool {
		attempts++
		var valid bool
		prepared, valid = e.native.Snapshot()
		if !valid {
			t.Error("preparation lost native binding")
		}
		return false
	}
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.h.observeDay()
	q.one()
	after, ok := e.native.Snapshot()
	// UI preparation intentionally precedes admission. Rejection must preserve
	// its actual result, rather than restore an older marked-text snapshot.
	if attempts != 1 || !ok || after != prepared || f.h.dayQueued || len(f.io.q) != 0 {
		t.Fatalf("rejection changed prepared result: before=%+v prepared=%+v after=%+v attempts=%d", before, prepared, after, attempts)
	}
	if prepared.Composing {
		t.Fatal("fixture did not exercise original Finish preparation")
	}
}

func TestRuntimeAdmissionCommandPreservesPartialAdvanceAndOriginalError(t *testing.T) {
	f := newTitleFixture(t, false)
	original := errors.New("synthetic non-sentinel command failure")
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	today := localTitleDate(f.clock.now)
	calls := 0
	f.h.command(f.s, func() error { return original }, false, func(err error) {
		calls++
		if err != original {
			t.Errorf("original error rewritten: %v", err)
		}
	})
	f.posts.one()
	if len(f.io.q) != 1 {
		t.Fatal("accepted command missing writer")
	}
	f.io.one()
	for len(f.posts.q) > 0 {
		f.posts.one()
	}
	if calls != 1 || f.svc.Today() != today || f.h.stopped || f.h.windows[f.s.titleWindowID].commands != 0 {
		t.Fatal("command failure hid completed Advance or leaked ticket")
	}
}

func TestRuntimeAdmissionObserveRejectionDoesNotAdvanceSeenText(t *testing.T) {
	f := newTitleFixture(t, true)
	e := f.s.titleEditor
	previous := e.seenText
	acceptedPost := f.h.tryPostProducer
	f.h.tryPostProducer = func(func()) bool { return false }
	f.edit("拒绝后再次观察")
	if e.seenText != previous || e.native.draft != "拒绝后再次观察" {
		t.Fatalf("rejected observe claimed consumption or changed draft: seen=%q draft=%q", e.seenText, e.native.draft)
	}
	f.h.tryPostProducer = acceptedPost
	f.tt.Frame()
	f.ui()
	f.advance(700 * time.Millisecond)
	f.all()
	if f.persisted() != "拒绝后再次观察" {
		t.Fatal("rejected observe prevented subsequent normal save")
	}
}

func TestRuntimeAdmissionRegistrationRejectRollsBackExactEditor(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "newer-owner"}[replace], func(t *testing.T) {
			f := newTitleFixture(t, false)
			if err := f.svc.AddToday("待注册标题"); err != nil {
				t.Fatal(err)
			}
			f.h.publish(f.svc.Snapshot(), f.svc.Today())
			f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
			f.s.runtimeDayRail = true
			accepted := f.h.tryPostProducer
			var captured, newer *runtimeTitleEditor
			f.h.tryPostProducer = func(func()) bool {
				if captured == nil && f.s.titleEditor != nil && f.s.titleEditor.queued {
					captured = f.s.titleEditor
					if replace {
						copyEditor := *captured
						newer = &copyEditor
						newer.native.draft = "新对象输入"
						f.s.titleEditor = newer
					}
				}
				return false
			}
			f.tt.Frame()
			if captured == nil || captured.queued || captured.registered || captured.native.draft != "待注册标题" {
				t.Fatalf("registration rejection leaked/changed original editor: %+v", captured)
			}
			if replace {
				if f.s.titleEditor != newer || !newer.queued || newer.native.draft != "新对象输入" {
					t.Fatal("old rejection reset newer owner scheduling")
				}
			} else {
				f.h.tryPostProducer = accepted
				f.tt.Frame()
				f.ui()
				if f.s.titleEditor != captured || !captured.registered {
					t.Fatal("rejected registration could not retry same native owner")
				}
			}
		})
	}
}

func TestRuntimeAdmissionSubtaskObserveRejectRetainsSeenAndRetries(t *testing.T) {
	f, _, child := subtaskFixture(t)
	var e *runtimeTitleEditor
	for _, candidate := range f.s.subtaskEditors {
		if candidate.subtask != nil && candidate.native.draft == "原子任务" {
			e = candidate
			break
		}
	}
	if e == nil || !e.registered {
		t.Fatal("missing registered child")
	}
	seen, compose, handle := e.seenText, e.seenCompose, e.native.handle
	accepted := f.h.tryPostProducer
	f.h.tryPostProducer = func(func()) bool { return false }
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "子任务稍后保存")
	if e.seenText != seen || e.seenCompose != compose || e.native.draft != "子任务稍后保存" || !reflect.DeepEqual(handle, e.native.handle) {
		t.Fatal("rejected child Observe advanced seen state or rebound native")
	}
	f.h.tryPostProducer = accepted
	f.tt.Frame()
	f.ui()
	f.advance(700 * time.Millisecond)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "子任务稍后保存" {
		t.Fatal("child Observe retry did not save")
	}
}

func TestRuntimeAdmissionComposerClearRejectPreservesAcknowledgement(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "已提交的第一条")
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "第二条候选", ui.TextInputRange{Start: 5, End: 5})
	f.all()
	var c *runtimeSubtaskComposer
	for _, candidate := range f.s.subtaskComposers {
		if candidate.mounted {
			c = candidate
			break
		}
	}
	if c == nil || !c.hasAcknowledged || c.pending || len(f.svc.Snapshot().Subtasks) != 1 {
		t.Fatal("missing real acknowledged late-input fixture")
	}
	ack, draft, handle := c.acknowledged, c.native.draft, c.native.handle
	attempts := 0
	f.h.tryPostProducer = func(func()) bool { attempts++; return false }
	f.tt.Frame()
	if attempts == 0 || c.clearQueued || !c.hasAcknowledged || c.acknowledged != ack || c.native.draft != draft || !reflect.DeepEqual(c.native.handle, handle) || len(f.svc.Snapshot().Subtasks) != 1 {
		t.Fatalf("clear rejection changed acknowledgement/draft: %+v attempts=%d", c, attempts)
	}
}

func TestRuntimeAdmissionDisposalRejectKeepsRegistrationAndExactPointer(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "newer-request"}[replace], func(t *testing.T) {
			f := newTitleFixture(t, true)
			e := f.s.titleEditor
			// Isolate scheduling of an otherwise clean, locked retirement candidate.
			e.locked = true
			state, ok := f.h.coordinator.State(e.reg)
			if !ok || state.Dirty || state.InFlight {
				t.Fatal("retirement fixture not clean")
			}
			handle, reg, id, draft := e.native.handle, e.reg, e.identity, e.native.draft
			var captured *runtimeTitleDisposal
			newer := &runtimeTitleDisposal{}
			f.h.tryPostProducer = func(func()) bool {
				captured = f.s.titleDisposal
				if replace {
					f.s.titleDisposal = newer
				}
				return false
			}
			f.s.disposeTitleEditors()
			if captured == nil {
				t.Fatal("retirement never attempted admission")
			}
			if (!replace && f.s.titleDisposal != nil) || (replace && f.s.titleDisposal != newer) {
				t.Fatal("disposal rejection cleared wrong request")
			}
			after, exists := f.h.coordinator.State(reg)
			if !exists || after != state || !e.registered || e.reg != reg || e.identity != id || f.s.titleEditor != e || e.native.draft != draft || !reflect.DeepEqual(handle, e.native.handle) {
				t.Fatal("rejected disposal retired or rebound original editor")
			}
		})
	}
}

func TestRuntimeAdmissionCategoryButtonProducerRejectDoesNotMutate(t *testing.T) {
	for _, label := range []string{"取消新建分组", "创建并加入", "enter"} {
		t.Run(label, func(t *testing.T) {
			f := newCategoryFixture(t, true)
			e := categoryBegin(t, f, "保留分组输入")
			before, err := f.svc.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			gen, handle := e.generation, e.native.handle
			attempts := 0
			f.h.tryPostProducer = func(func()) bool { attempts++; return false }
			if label == "enter" {
				f.tt.Key(0, ui.KeyEnter)
			} else {
				liveClick(t, f.tt, label)
			}
			f.ui()
			after, err := f.svc.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			if attempts == 0 || e.pending || !e.creating || e.failure != nil || e.generation != gen || e.native.draft != "保留分组输入" || !reflect.DeepEqual(e.native.handle, handle) || f.h.windows[f.s.titleWindowID].commands != 0 || !reflect.DeepEqual(before, after) || len(f.io.q) != 0 {
				t.Fatalf("rejected category button mutated editor/facts: %+v attempts=%d", e, attempts)
			}
		})
	}
}

// Keep preparation's persistence receipt distinct from natural-day delivery.
// The underlying Service remains real; only its calls/results are observed.
type admissionDayRecordingStore struct {
	titleStore
	events  []string
	saved   string
	saveErr error
}

func (s *admissionDayRecordingStore) AdvanceTo(d core.LocalDate) error {
	s.events = append(s.events, "advance")
	return s.titleStore.AdvanceTo(d)
}
func (s *admissionDayRecordingStore) SaveTraceTitleInputAcknowledged(id core.DayTraceID, text string) (string, error) {
	s.events = append(s.events, "save")
	s.saved, s.saveErr = s.titleStore.SaveTraceTitleInputAcknowledged(id, text)
	return s.saved, s.saveErr
}

func TestRuntimeAdmissionObserveDayPreparationSavePrecedesNaturalDay(t *testing.T) {
	for _, rejectDay := range []bool{false, true} {
		t.Run(map[bool]string{false: "day-accepted", true: "day-rejected"}[rejectDay], func(t *testing.T) {
			f := newTitleFixture(t, true)
			f.edit("准备先保存")
			e := f.s.titleEditor
			state, ok := f.h.coordinator.State(e.reg)
			if !ok || !state.Dirty || state.InFlight || len(f.io.q) != 0 {
				t.Fatal("missing dirty unsubmitted title")
			}
			// Make the existing job due without firing its timer. A stale observed-day
			// marker triggers observation while keeping the real Service date unchanged,
			// isolating FIFO from the separate immutable-history policy after midnight.
			f.clock.now = f.clock.now.Add(700 * time.Millisecond)
			f.h.observedDay = core.LocalDate{}
			recorded := &admissionDayRecordingStore{titleStore: f.h.store}
			f.h.store = recorded
			submit := f.h.trySubmit
			dayAttempts, dayAccepted, dayExecuted := 0, 0, 0
			f.h.trySubmit = func(work func(func(func()) bool)) bool {
				dayAttempts++
				// Coordinator retains the original scoped submit capability. Only the
				// explicit natural-day call reaches this hook; its predecessor must exist.
				if len(f.io.q) != 1 {
					t.Errorf("preparation save not queued first: %d", len(f.io.q))
				}
				if rejectDay {
					return false
				}
				accepted := submit(func(post func(func()) bool) { dayExecuted++; work(post) })
				if accepted {
					dayAccepted++
				}
				return accepted
			}
			f.h.observeDay()
			if len(f.posts.q) != 1 {
				t.Fatalf("unexpected pre-preparation posts: %d", len(f.posts.q))
			}
			f.posts.one()
			wantQueued := 2
			if rejectDay {
				wantQueued = 1
			}
			if dayAttempts != 1 || len(f.io.q) != wantQueued || len(recorded.events) != 0 {
				t.Fatalf("admission order attempts=%d queue=%d calls=%v", dayAttempts, len(f.io.q), recorded.events)
			}
			f.io.one() // Preparation save, never the natural-day task.
			if dayExecuted != 0 || !reflect.DeepEqual(recorded.events, []string{"advance", "save"}) || recorded.saveErr != nil || recorded.saved != "准备先保存" {
				t.Fatalf("preparation receipt/events: %q %v %v", recorded.saved, recorded.saveErr, recorded.events)
			}
			for len(f.posts.q) > 0 {
				f.posts.one()
			}
			state, ok = f.h.coordinator.State(e.reg)
			if !ok || state.InFlight || state.Dirty || state.LastError != nil || f.persisted() != "准备先保存" {
				t.Fatalf("preparation save not truly settled: %+v", state)
			}
			if rejectDay {
				if dayAccepted != 0 || dayExecuted != 0 || f.h.dayQueued || len(f.io.q) != 0 {
					t.Fatal("rejected day task ran or canceled predecessor")
				}
			} else {
				if dayAccepted != 1 || !f.h.dayQueued {
					t.Fatal("accepted day lost independent ticket")
				}
				f.io.one()
				for len(f.posts.q) > 0 {
					f.posts.one()
				}
				if dayExecuted != 1 || f.h.dayQueued || !reflect.DeepEqual(recorded.events, []string{"advance", "save", "advance"}) {
					t.Fatalf("natural-day ordering: %v", recorded.events)
				}
			}
		})
	}
}

func TestRuntimeAdmissionObserveDayEndRunDuringPermittedPreparation(t *testing.T) {
	f := newTitleFixture(t, true)
	w, d := newAdmissionTestLane(), newAdmissionTestLane()
	var callbacks []func()
	a := newRuntimeAdmission(w, d, func(fn func()) { callbacks = append(callbacks, fn) })
	f.h.tryPostProducer = a.TryPostProducer
	prepared, ended, idle := make(chan struct{}), make(chan struct{}), make(chan struct{})
	attempts := 0
	f.h.trySubmit = func(work func(func(func()) bool)) bool {
		attempts++
		close(prepared)
		<-ended
		// The same already-permitted UI turn may finish its own rejection handling,
		// but cannot grant fresh writer admission after EndRun.
		return a.TrySubmitWriter(work)
	}
	f.h.observedDay = core.LocalDate{}
	f.h.observeDay()
	if !a.TryPostProducer(func() { t.Error("unstarted producer ran during cleanup") }) {
		t.Fatal("second producer not queued")
	}
	d.drain()
	if len(callbacks) != 2 {
		t.Fatal("missing framework callbacks")
	}
	go func() { <-prepared; a.EndRun(); close(ended); a.WaitActiveUI(); close(idle) }()
	callbacks[0]() // Real UI preparation stays on the fixture's calling thread.
	<-idle
	if attempts != 1 || f.h.dayQueued || len(f.io.q) != 0 {
		t.Fatal("active preparation did not settle its rejected day ticket")
	}
	a.mu.Lock()
	active := a.activeUI
	a.mu.Unlock()
	if active != 0 {
		t.Fatalf("active UI=%d", active)
	}
	r := admissionTestFinish(a, w, d)
	if r.WriterAccepted != 0 || r.WriterRejected != 1 || r.ProducerConsumed != 1 || r.ProducerUndelivered != 1 || r.ProtocolErrors != 0 {
		t.Fatalf("cleanup report=%+v", r)
	}
	callbacks[1]()
	if a.FinishDispatchDrain() != r {
		t.Fatal("unstarted callback changed closed report")
	}
}

func TestRuntimeAdmissionObserveDayPreparationEarlyReturnKeepsEarlierSave(t *testing.T) {
	f, _, _ := subtaskFixture(t)
	f.edit("早退前已接纳标题")
	title := f.s.titleEditor
	var child *runtimeTitleEditor
	for _, e := range f.s.subtaskEditors {
		if e.registered {
			child = e
			break
		}
	}
	if child == nil {
		t.Fatal("missing registered child")
	}
	f.clock.now = f.clock.now.Add(700 * time.Millisecond)
	f.h.observedDay = core.LocalDate{}
	// titleEditors visits the title before children. Invalidate only the later
	// child's native handle so its Finish fails after Observe accepted the title.
	originalHandle := child.native.handle
	var absent ui.Handle
	child.native.handle = absent
	naturalAttempts := 0
	f.h.trySubmit = func(func(func(func()) bool)) bool { naturalAttempts++; return false }
	f.h.observeDay()
	f.posts.one()
	child.native.handle = originalHandle
	if naturalAttempts != 0 || f.h.dayQueued || len(f.io.q) != 1 {
		t.Fatalf("preparation early return attempts=%d queued=%d dayQueued=%v", naturalAttempts, len(f.io.q), f.h.dayQueued)
	}
	f.io.one()
	for len(f.posts.q) > 0 {
		f.posts.one()
	}
	state, ok := f.h.coordinator.State(title.reg)
	if !ok || state.InFlight || state.Dirty || state.LastError != nil || f.persisted() != "早退前已接纳标题" {
		t.Fatalf("early return canceled accepted title receipt: %+v", state)
	}
}
