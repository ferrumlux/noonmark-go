package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewerPublicationCleanupCannotSupersedeQueuedUserNavigation(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	f.io.one()                 // Disk success; publication/completion are queued on main.
	liveClick(t, tt, "切换到任务池") // Real later user intent, before publication runs.
	f.all()
	tt.Frame()
	f.ui()
	if other.page != "pool" {
		t.Fatal("automatic clean-title disposal superseded queued real user navigation", other.page)
	}
}
