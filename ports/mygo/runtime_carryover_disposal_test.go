// SPDX-License-Identifier: AGPL-3.0-only
package main

import "testing"

func TestCarryoverDisposalDoesNotSwallowNewerNavigation(t *testing.T) {
	f := newTitleFixture(t, true)
	f.s.disposeTitleEditors()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
	f.all()
	if f.s.page != "pool" {
		t.Fatal("queued disposal swallowed newer navigation")
	}
	f.s.titleTransition(func() {
		f.s.page = "day"
		f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
		f.s.runtimeDayRail = true
	})
	f.all()
	f.tt.Frame()
	editor := f.s.titleEditor
	if editor == nil {
		t.Fatal("new title session missing")
	}
	f.ui()
	f.tt.Frame()
	if f.s.titleEditor != editor {
		t.Fatal("stale disposal removed new editor")
	}
}
