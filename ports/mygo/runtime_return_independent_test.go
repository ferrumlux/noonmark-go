// SPDX-License-Identifier: AGPL-3.0-only
// Independent review regressions, retained without weakening their assertions.
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"reflect"
	"testing"
	"unsafe"
)

func TestIndependentOldMenuAfterNavigationRoundTrip(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	f.s.titleTransition(func() { f.s.page = "pool" })
	f.all()
	f.tt.Frame()
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
	f.all()
	f.tt.Frame()
	chooseReturn(t, f.tt)
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("stale menu callback returned task after navigation round trip")
	}
}

func independentNativeMenuCallback(t *testing.T, tt *ui.Tester) func(int) {
	t.Helper()
	v := reflect.ValueOf(tt).Elem().FieldByName("h").Elem().FieldByName("chosen")
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Interface().(func(int))
}
func TestIndependentCancelledOldNativeMenuAfterReopen(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	old := independentNativeMenuCallback(t, f.tt)
	f.tt.CloseMenu()
	openReturnMenu(t, f.tt, trace.ID)
	old(2) // Native host delivers the old return item after a newer menu opened.
	f.tt.Frame()
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("cancelled old native callback returned task using reopened same-trace menu")
	}
}

func TestIndependentOldNativeMenuCannotCrossReattachedSession(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, trace.ID)
	old := independentNativeMenuCallback(t, f.tt)
	if !f.h.beforeClose(f.s) {
		t.Fatal("clean close blocked")
	}
	f.svc.Close()
	var err error
	f.svc, err = taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	f.h = newTitleHub(f.svc, &f.io, f.posts.Enqueue, &f.clock, f.clock.Now, func() {})
	f.h.attach(f.s, func() { f.invalid = true }, func() {})
	f.tt.Frame()
	f.all()
	old(2)
	f.tt.Frame()
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("old native menu wrote into reattached session")
	}
}
func TestIndependentMenuOmitsCompleteWithOpenSubtask(t *testing.T) {
	f := newTitleFixture(t, true)
	g := f.svc.Snapshot()
	tr := g.Traces[0]
	id, _ := core.NewIdentity()
	lineage, _ := core.NewIdentity()
	g.Subtasks = []core.Subtask{{ID: core.SubtaskID(id), LineageID: core.SubtaskLineageID(lineage), TraceID: tr.ID, Title: "合成子任务", Status: core.SubtaskPending, Difficulty: core.DifficultyMedium, Position: 1, CreatedAt: tr.CreatedAt, UpdatedAt: tr.CreatedAt}}
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatal(err)
	}
	f.h.publish(g, liveTestDay)
	f.ui()
	f.tt.Frame()
	openReturnMenu(t, f.tt, tr.ID)
	if !reflect.DeepEqual(f.tt.Menu(), []string{returnToPoolLabel}) {
		t.Fatalf("source completion capability omitted: menu=%v", f.tt.Menu())
	}
}
