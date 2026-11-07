// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

// Count invocation of the actual injected host callback, even if a subsequent
// gate would veto it. Existing titleFixture.quit counts only successful gates.
func independentCaptureFixture(t *testing.T, producer string) *titleFixture {
	t.Helper()
	f := &titleFixture{t: t}
	f.clock.now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f.path = filepath.Join(t.TempDir(), "independent-c1-session")
	var err error
	if producer == "category" {
		f.svc, err = taskapp.CreateOrdinaryDevelopment(f.path, liveTestDay)
	} else {
		f.svc, err = taskapp.Create(f.path, liveTestDay)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.svc.Close() })
	if producer == "category" {
		err = f.svc.AddPool("原题")
	}
	if producer == "title" || producer == "carryover" {
		err = f.svc.AddToday("原题")
	}
	if err != nil {
		t.Fatal(err)
	}
	f.h = newTestTitleHub(f.svc, &f.io, f.posts.Enqueue, &f.clock, f.clock.Now, func() { f.quit++ })
	if producer == "category" {
		if err = f.h.enableCategoryCandidate(); err != nil {
			t.Fatal(err)
		}
	}
	f.s = newRuntimeApp(f.h.readView())
	f.h.attach(f.s, func() { f.invalid = true }, func() {
		f.closed++ // Count requests, not merely successful beforeClose calls.
		f.h.beforeClose(f.s)
	})
	if producer == "category" {
		f.s.page = "pool"
		f.s.runtimeSelection = string(f.svc.Snapshot().Chains[0].ID)
	}
	if producer == "title" || producer == "carryover" {
		f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
		f.s.runtimeDayRail = true
	}
	f.tt = ui.NewTester(f.s.view, 1200, 768)
	f.ui()
	return f
}

func independentCaptureCompanion(t *testing.T, f *titleFixture) (*app, *ui.Tester, *int) {
	t.Helper()
	s := newRuntimeApp(f.h.readView())
	s.page = "quick-entry"
	closed := new(int)
	f.h.attach(s, func() {}, func() { (*closed)++; f.h.beforeClose(s) })
	s.closeWindow = func() {
		if f.h.beforeClose(s) {
			(*closed)++
		}
	}
	tt := ui.NewTester(s.view, 1200, 768)
	f.ui()
	return s, tt, closed
}

func independentCaptureStart(t *testing.T, f *titleFixture, producer string) *runtimeCategoryEditor {
	t.Helper()
	var e *runtimeCategoryEditor
	switch producer {
	case "generic":
		liveType(t, f.tt, "新输入")
		liveSubmit(t, f.tt)
	case "category":
		e = categoryBegin(t, f, "独立分组")
		liveClick(t, f.tt, "创建并加入")
	case "title":
		f.edit("新标题")
		f.advance(700 * time.Millisecond)
	default:
		t.Fatal("unknown producer", producer)
	}
	f.ui()
	w := f.h.windows[f.s.titleWindowID]
	wantCommands := 1
	if producer == "title" {
		wantCommands = 0
	}
	if w == nil || w.commands != wantCommands || len(f.io.q) != 1 {
		t.Fatalf("real producer not held: producer=%s window=%+v writers=%d", producer, w, len(f.io.q))
	}
	if producer == "title" {
		st, ok := f.h.coordinator.State(f.s.titleEditor.reg)
		if !ok || !st.InFlight {
			t.Fatal("title producer did not enter a real coordinator flight")
		}
	}
	return e
}

func independentCaptureSettled(t *testing.T, f *titleFixture, producer string, w *titleWindow, e *runtimeCategoryEditor) {
	t.Helper()
	if w.commands != 0 || len(w.inputPending) != 0 || len(f.io.q) != 0 {
		t.Fatalf("original ticket not settled: commands=%d input=%v writers=%d", w.commands, w.inputPending, len(f.io.q))
	}
	if f.s.runtimeError != "" {
		t.Fatal("unexpected real error", f.s.runtimeError)
	}
	g := f.svc.Snapshot()
	switch producer {
	case "generic":
		if len(g.Chains) != 1 || len(g.Definitions) != 1 || g.Definitions[0].Title != "新输入" {
			t.Fatal("wrong durable command result", g)
		}
	case "title":
		if f.persisted() != "新标题" {
			t.Fatal("real title save missing", f.persisted())
		}
	case "category":
		view, err := f.svc.ReadView()
		if err != nil {
			t.Fatal(err)
		}
		if e.pending || e.failure != nil || e.creating || e.savedUntil.IsZero() || len(view.Classification.Categories) != 1 || categoryCurrent(view, e.chain) == "" {
			t.Fatalf("confirmed category receipt not reflected: editor=%+v classification=%+v", e, view.Classification)
		}
	}
}

func TestCaptureCloseProductIndependentThreeProducersQuitRequiresFreshRequest(t *testing.T) {
	for _, producer := range []string{"generic", "category", "title"} {
		for _, presence := range []string{"no-capture", "capture-attached", "capture-attached-after-veto"} {
			for _, next := range []string{"quit", "close"} {
				t.Run(producer+"/"+presence+"/"+next, func(t *testing.T) {
					f := independentCaptureFixture(t, producer)
					if presence == "capture-attached" {
						independentCaptureCompanion(t, f)
					}
					e := independentCaptureStart(t, f, producer)
					w := f.h.windows[f.s.titleWindowID]
					if f.h.beforeQuit() || f.h.beforeQuit() {
						t.Fatal("pending explicit Quit was not vetoed twice")
					}
					if presence == "capture-attached-after-veto" {
						independentCaptureCompanion(t, f)
					}
					f.all()
					independentCaptureSettled(t, f, producer, w, e)
					if f.quit != 0 || f.closed != 0 || f.h.quitting || f.h.windows[f.s.titleWindowID] != w {
						t.Fatalf("completion retained/used old exit: quit=%d close=%d waiting=%t", f.quit, f.closed, f.h.quitting)
					}
					if next == "quit" {
						if !f.h.beforeQuit() {
							t.Fatal("fresh clean explicit Quit remained blocked")
						}
					} else if !f.h.beforeClose(f.s) {
						t.Fatal("old Quit blocked a fresh clean Close")
					}
					if f.quit != 0 {
						t.Fatal("checking an explicit gate emitted a host Quit")
					}
				})
			}
		}
	}
}

func TestCaptureCloseProductIndependentOrdinaryDelayedCloseThreeProducers(t *testing.T) {
	for _, producer := range []string{"generic", "category", "title"} {
		t.Run(producer, func(t *testing.T) {
			f := independentCaptureFixture(t, producer)
			e := independentCaptureStart(t, f, producer)
			w := f.h.windows[f.s.titleWindowID]
			if f.h.beforeClose(f.s) {
				t.Fatal("pending ordinary Close passed")
			}
			f.all()
			independentCaptureSettled(t, f, producer, w, e)
			if f.closed != 1 || f.h.windows[f.s.titleWindowID] != nil || f.quit != 0 {
				t.Fatalf("ordinary delayed Close changed: close=%d quit=%d", f.closed, f.quit)
			}
		})
	}
}

func TestCaptureCloseProductIndependentCaptureFreshCloseAfterSettlement(t *testing.T) {
	for _, page := range []string{"quick-entry"} {
		t.Run(page, func(t *testing.T) {
			f := independentCaptureFixture(t, "generic")
			f.s.page = page
			f.tt.Frame()
			liveType(t, f.tt, "新输入")
			liveSubmit(t, f.tt)
			f.ui()
			w := f.h.windows[f.s.titleWindowID]
			if w.commands != 1 || len(f.io.q) != 1 {
				t.Fatal("capture submission missing")
			}
			if f.h.beforeClose(f.s) || w.closing {
				t.Fatal("capture Close not one-shot")
			}
			f.all()
			independentCaptureSettled(t, f, "generic", w, nil)
			if f.closed != 0 || f.h.windows[f.s.titleWindowID] != w {
				t.Fatal("old capture Close retried")
			}
			if !f.h.beforeClose(f.s) || f.h.windows[f.s.titleWindowID] != nil {
				t.Fatal("fresh capture Close failed")
			}
		})
	}
}

// Defensive route mutation, NOT a reachable capture editor UI: category/title
// controls live in ordinary pages. Admit via their real bindings, request an
// ordinary delayed Close, then change only the presentation route before ack.
// This checks all three completion policies without fabricating pending tickets.
func TestCaptureCloseProductIndependentDefensiveRouteBecomesCapture(t *testing.T) {
	for _, producer := range []string{"generic", "category", "title"} {
		t.Run(producer, func(t *testing.T) {
			f := independentCaptureFixture(t, producer)
			e := independentCaptureStart(t, f, producer)
			w := f.h.windows[f.s.titleWindowID]
			if f.h.beforeClose(f.s) {
				t.Fatal("pending Close not held")
			}
			f.s.page = "quick-entry" // Explicitly defensive, not UI navigation.
			// Avoid rerendering a fictitious mixed capture/category/title surface.
			w.invalidate = func() {}
			f.invalid = false
			f.all()
			independentCaptureSettled(t, f, producer, w, e)
			if f.closed != 0 || f.h.windows[f.s.titleWindowID] != w || f.quit != 0 {
				t.Fatal("capture route accepted an old automatic exit")
			}
		})
	}
}

// Normal pending gates prohibit destroying this owner. Direct registry removal
// here is a defensive stale-callback test, never an OS-reachable close claim.
func TestCaptureCloseProductIndependentDefensiveOldWindowCompletion(t *testing.T) {
	for _, producer := range []string{"generic", "category", "title"} {
		t.Run(producer, func(t *testing.T) {
			f := independentCaptureFixture(t, producer)
			independentCaptureStart(t, f, producer)
			old := f.h.windows[f.s.titleWindowID]
			if f.h.beforeClose(f.s) {
				t.Fatal("pending Close not held")
			}
			delete(f.h.windows, f.s.titleWindowID)
			newer, tt, closed := independentCaptureCompanion(t, f)
			liveType(t, tt, "新窗口输入")
			f.all()
			if old.commands != 0 || len(old.inputPending) != 0 {
				t.Fatal("stale owner ticket was skipped")
			}
			if *closed != 0 || f.closed != 0 || f.quit != 0 || newer.draft != "新窗口输入" || f.h.windows[newer.titleWindowID].app != newer {
				t.Fatal("old completion affected a new window")
			}
		})
	}
}

func TestCaptureCloseProductIndependentMultiwindowFIFOOrders(t *testing.T) {
	for _, first := range []string{"category", "generic"} {
		t.Run(first, func(t *testing.T) {
			f := independentCaptureFixture(t, "category")
			e := categoryBegin(t, f, "顺序分组")
			s, tt, closed := independentCaptureCompanion(t, f)
			startGeneric := func() { liveType(t, tt, "第二窗口"); liveSubmit(t, tt); f.ui() }
			startCategory := func() { liveClick(t, f.tt, "创建并加入"); f.ui() }
			if first == "category" {
				startCategory()
				startGeneric()
			} else {
				startGeneric()
				startCategory()
			}
			w, other := f.h.windows[f.s.titleWindowID], f.h.windows[s.titleWindowID]
			if len(f.io.q) != 2 || w.commands != 1 || other.commands != 1 {
				t.Fatal("two actual writers not pending")
			}
			if f.h.beforeQuit() {
				t.Fatal("multiwindow pending Quit passed")
			}
			f.io.one()
			f.ui() // Preserve writer FIFO. Change admission order, not lane order.
			if f.quit != 0 || f.closed != 0 || *closed != 0 || w.commands+other.commands != 1 {
				t.Fatal("first completion exited or settled wrong count")
			}
			if f.h.beforeQuit() {
				t.Fatal("remaining producer not protected")
			}
			f.all()
			view, err := f.svc.ReadView()
			if err != nil {
				t.Fatal(err)
			}
			if f.quit != 0 || f.closed != 0 || *closed != 0 || w.commands != 0 || other.commands != 0 || len(other.inputPending) != 0 || e.pending || e.failure != nil || len(view.Graph.Chains) != 2 || len(view.Classification.Categories) != 1 {
				t.Fatal("multiwindow completion/receipt mismatch")
			}
			if !f.h.beforeQuit() {
				t.Fatal("fresh multiwindow Quit blocked")
			}
			if err = f.svc.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := taskapp.OpenOrdinaryDevelopment(f.path, liveTestDay)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err := reopened.ReadView()
			if err != nil || !reflect.DeepEqual(view, got) {
				t.Fatal("cold reopened full read view differs", err)
			}
		})
	}
}

func TestCaptureCloseProductIndependentRealFailureKeepsErrorAndTicket(t *testing.T) {
	for _, producer := range []string{"generic", "category", "title"} {
		t.Run(producer, func(t *testing.T) {
			f := independentCaptureFixture(t, producer)
			e := independentCaptureStart(t, f, producer)
			w := f.h.windows[f.s.titleWindowID]
			if f.h.beforeQuit() {
				t.Fatal("pending Quit passed")
			}
			if err := f.svc.Close(); err != nil {
				t.Fatal(err)
			} // Real Service failure, no invented success receipt.
			f.all()
			if w.commands != 0 || len(w.inputPending) != 0 || f.quit != 0 || f.closed != 0 {
				t.Fatal("failure failed settlement or emitted exit")
			}
			switch producer {
			case "generic":
				if f.s.runtimeError == "" || f.s.draft != "新输入" {
					t.Fatal("actual command error or original input lost")
				}
			case "category":
				if !errors.Is(e.failure, taskapp.ErrSession) || e.pending || !e.creating || !e.savedUntil.IsZero() || e.native.draft != "独立分组" {
					t.Fatal("category failure forged receipt or discarded input", e.failure)
				}
			case "title":
				st, ok := f.h.coordinator.State(f.s.titleEditor.reg)
				if !ok || st.InFlight || !errors.Is(st.LastError, taskapp.ErrSession) {
					t.Fatal("actual title error lost", st)
				}
				f.s.disposeTitleEditors()
				f.ui()
				if !f.s.titleEditor.registered {
					t.Fatal("failed title owner retired")
				}
			}
			f.all() // A second drain must not settle a ticket twice.
			if w.commands != 0 || len(w.inputPending) != 0 {
				t.Fatal("double settlement")
			}
		})
	}
}

func TestCaptureCloseProductIndependentRealAdmissionRejectsWithoutWrite(t *testing.T) {
	// Terminal admission is a shutdown race fixture. It is not a claim that
	// a normal fresh UI action can arrive after the native host has stopped.
	for _, producer := range []string{"generic", "category"} {
		for _, hop := range []string{"producer", "writer"} {
			t.Run(producer+"/"+hop, func(t *testing.T) {
				f := independentCaptureFixture(t, producer)
				var e *runtimeCategoryEditor
				if producer == "category" {
					e = categoryBegin(t, f, "拒绝分组")
				} else {
					liveType(t, f.tt, "拒绝输入")
				}
				before := f.svc.Snapshot()
				writer, dispatcher := newAdmissionTestLane(), newAdmissionTestLane()
				a := newRuntimeAdmission(writer, dispatcher, f.posts.Enqueue)
				a.EndRun()
				if hop == "producer" {
					f.h.tryPostProducer = a.TryPostProducer
				}
				f.h.trySubmit = a.TrySubmitWriter
				w := f.h.windows[f.s.titleWindowID]
				if producer == "category" {
					// Direct UI-safe category API test with the original real
					// native binding, NOT an admitted outer-button/OS path.
					// Do not replay a UI closure across admission.EndRun.
					snap, ok := e.native.Snapshot()
					if !ok || snap.Composing || snap.Text != "拒绝分组" || w.category != e || e.pending {
						t.Fatal("original category binding is not ready for direct UI-safe submission")
					}
					f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
				} else {
					liveSubmit(t, f.tt)
				}
				if hop == "writer" {
					if w.commands != 1 || f.h.beforeQuit() {
						t.Fatal("accepted first hop not protected")
					}
				}
				f.ui()
				if w.commands != 0 || len(w.inputPending) != 0 || len(f.io.q) != 0 || !reflect.DeepEqual(before, f.svc.Snapshot()) {
					t.Fatal("rejected producer wrote or leaked ticket")
				}
				if producer == "category" {
					if !errors.Is(e.failure, coordinator.ErrNotAdmitted) || e.pending || e.native.draft != "拒绝分组" || !e.savedUntil.IsZero() {
						t.Fatal("original category rejection lost", e.failure)
					}
				} else if f.s.runtimeError == "" || f.s.draft != "拒绝输入" {
					t.Fatal("command rejection/input lost")
				}
				r := admissionTestFinish(a, writer, dispatcher)
				if hop == "producer" && (r.ProducerRejected != 1 || r.WriterRejected != 0) || hop == "writer" && (r.WriterRejected != 1 || r.ProducerRejected != 0) {
					t.Fatal("wrong admission hop rejected", r)
				}
				if r.WriterAccepted != 0 || r.ProtocolErrors != 0 || f.quit != 0 || f.closed != 0 {
					t.Fatal("rejected admission emitted work/exit", r)
				}
			})
		}
	}
}

// The UI-action admission is before business admission. Rejecting it must not
// invent an ErrNotAdmitted result for a category operation that never began.
func TestCaptureCloseProductIndependentCategoryOuterAdmissionPreservesBinding(t *testing.T) {
	f := independentCaptureFixture(t, "category")
	e := categoryBegin(t, f, "外层拒绝保留")
	w := f.h.windows[f.s.titleWindowID]
	before, err := f.svc.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	generation, handle := e.generation, e.native.handle
	writer, dispatcher := newAdmissionTestLane(), newAdmissionTestLane()
	a := newRuntimeAdmission(writer, dispatcher, f.posts.Enqueue)
	a.EndRun()
	f.h.tryPostProducer = a.TryPostProducer
	f.h.trySubmit = a.TrySubmitWriter
	liveClick(t, f.tt, "创建并加入")
	f.ui()
	after, err := f.svc.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if w.category != e || w.commands != 0 || len(w.inputPending) != 0 || e.pending || !e.creating || e.failure != nil || e.generation != generation || !reflect.DeepEqual(e.native.handle, handle) || e.native.draft != "外层拒绝保留" || !e.savedUntil.IsZero() || !reflect.DeepEqual(before, after) || len(f.io.q) != 0 {
		t.Fatal("outer rejection mutated binding, business ticket, or durable facts")
	}
	if f.h.beforeQuit() {
		t.Fatal("unsubmitted category draft no longer protects Quit")
	}
	r := admissionTestFinish(a, writer, dispatcher)
	if r.ProducerRejected != 1 || r.WriterAccepted != 0 || r.WriterRejected != 0 || r.ProtocolErrors != 0 || f.quit != 0 || f.closed != 0 {
		t.Fatal("outer rejection reached business admission or emitted exit", r)
	}
}

func TestCaptureCloseProductIndependentCarryoverAfterVetoedQuit(t *testing.T) {
	f := independentCaptureFixture(t, "carryover")
	openCarryoverMenu(t, f, carryoverDefer)
	target := f.s.carryover.picker.selected
	confirmCarryover(t, f)
	w := f.h.windows[f.s.titleWindowID]
	if f.h.beforeQuit() {
		t.Fatal("pending carryover Quit passed")
	}
	f.all()
	if f.quit != 0 || f.closed != 0 || f.h.quitting || w.commands != 0 || f.s.runtimeError != "" || f.s.carryover != nil || f.s.runtimeDate != target || len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("old Quit suppressed allowed carryover navigation or changed its result")
	}
	if !f.h.beforeQuit() {
		t.Fatal("fresh Quit blocked after carryover")
	}
}

func TestCaptureCloseProductIndependentCommandOriginalErrorExactlyOnce(t *testing.T) {
	for _, failure := range []string{"service-closed", "writer-not-admitted"} {
		t.Run(failure, func(t *testing.T) {
			f := independentCaptureFixture(t, "generic")
			liveType(t, f.tt, "真实命令输入")
			w := f.h.windows[f.s.titleWindowID]
			var want error = taskapp.ErrSession
			var admission *runtimeAdmission
			var writer, dispatcher *admissionTestLane
			if failure == "writer-not-admitted" {
				want = coordinator.ErrNotAdmitted
				writer, dispatcher = newAdmissionTestLane(), newAdmissionTestLane()
				admission = newRuntimeAdmission(writer, dispatcher, f.posts.Enqueue)
				admission.EndRun()
				f.h.trySubmit = admission.TrySubmitWriter
			}
			calls := 0
			// Invoke the real generic command entry to observe its exact error.
			// Its input is from the original UI binding; its body is real Service.
			input := f.s.draft
			f.h.command(f.s, func() error { return f.svc.AddPool(input) }, true, func(err error) {
				calls++
				if !errors.Is(err, want) {
					t.Errorf("original error changed: got=%v want=%v", err, want)
				}
			})
			if w.commands != 1 || !w.inputPending[input] || f.h.beforeQuit() {
				t.Fatal("command admission missing")
			}
			if failure == "service-closed" {
				f.ui()
				if err := f.svc.Close(); err != nil {
					t.Fatal(err)
				}
			}
			f.all()
			f.all()
			if calls != 1 || w.commands != 0 || len(w.inputPending) != 0 || f.s.draft != input || f.s.runtimeError == "" || f.quit != 0 || f.closed != 0 {
				t.Fatal("error or exactly-once settlement lost", calls)
			}
			if admission != nil {
				admissionTestFinish(admission, writer, dispatcher)
			}
		})
	}
}
