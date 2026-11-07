// SPDX-License-Identifier: AGPL-3.0-only
// Regression from the independently reproduced C1 failure.
// Real Service + runtime UI Tester binding; not an OS close/IME assertion.
package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func captureLateAckFixture(t *testing.T) *titleFixture {
	f := newTitleFixture(t, false)
	f.s.page = "quick-entry" // Presentation setup only, never assigns draft.
	f.s.closeWindow = func() {
		if f.h.beforeClose(f.s) {
			f.closed++
		}
	}
	f.tt.Frame()
	return f
}
func captureLateAckSubmitA(t *testing.T, f *titleFixture) {
	liveType(t, f.tt, "A")
	liveClick(t, f.tt, "保存到任务池")
	f.ui() // Drain producer posts, hold real writer execution.
	w := f.h.windows[f.s.titleWindowID]
	if w == nil || w.commands != 1 || len(f.io.q) != 1 || f.s.draft != "A" {
		t.Fatal("precondition: actual capture submission did not reach pending writer")
	}
	if len(f.svc.Snapshot().Chains) != 0 {
		t.Fatal("writer was not held")
	}
}
func captureLateAckAssertOneA(t *testing.T, f *titleFixture, w *titleWindow) {
	g := f.svc.Snapshot()
	if len(g.Chains) != 1 || len(g.Definitions) != 1 || g.Definitions[0].Title != "A" {
		t.Fatalf("real saved graph is not exactly one A: %+v", g)
	}
	if w == nil || w.commands != 0 || len(w.inputPending) != 0 {
		t.Fatal("real completion did not settle its ticket")
	}
}

// C1: exactly ONE cancellation before newer input. Never issue another cancel.
func TestCaptureLateAckCancelOnceThenNewInput(t *testing.T) {
	f := captureLateAckFixture(t)
	captureLateAckSubmitA(t, f)
	submittedWindow := f.h.windows[f.s.titleWindowID]
	liveClick(t, f.tt, "取消快速记录")
	if f.closed != 0 {
		t.Fatal("pending close was not vetoed")
	}
	liveClick(t, f.tt, "开发任务标题")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("AB")
	if f.s.draft != "AB" {
		t.Fatal("new input not delivered to real binding")
	}
	f.all()
	captureLateAckAssertOneA(t, f, submittedWindow)
	if f.closed != 0 || f.h.windows[f.s.titleWindowID] != submittedWindow || f.s.draft != "AB" {
		t.Fatalf("old ack removed newer draft access: closed=%d draft=%q", f.closed, f.s.draft)
	}
}

// C1-Q: direct beforeQuit models the real quit gate, NOT a native OS quit event.
func TestCaptureLateAckQuitOnceThenNewInput(t *testing.T) {
	f := captureLateAckFixture(t)
	captureLateAckSubmitA(t, f)
	submittedWindow := f.h.windows[f.s.titleWindowID]
	if f.h.beforeQuit() {
		t.Fatal("pending quit was not vetoed")
	}
	liveClick(t, f.tt, "开发任务标题")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("AB")
	if f.s.draft != "AB" {
		t.Fatal("new input not delivered to real binding")
	}
	f.all()
	captureLateAckAssertOneA(t, f, submittedWindow)
	if f.quit != 0 || f.s.draft != "AB" || f.h.windows[f.s.titleWindowID] != submittedWindow {
		t.Fatalf("old quit consumed new input: quit=%d draft=%q sameWindow=%t", f.quit, f.s.draft, f.h.windows[f.s.titleWindowID] == submittedWindow)
	}
}
