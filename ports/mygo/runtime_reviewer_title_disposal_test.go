package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestReviewerQueuedTitleDisposalDoesNotRevokeRecoveryLease(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	if other.titleEditor == nil {
		t.Fatal("setup: no original clean title editor")
	}
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "原恢复草稿", ui.TextInputRange{Start: 5, End: 5})
	f.io.one()
	f.posts.one() // Actual publication, before queued title disposal.
	tt.Frame()
	liveClick(t, tt, label)
	client := titleDirectClient(t, tt)
	client.SetMarkedText(nil, "恢复字段重新取得租约", ui.TextInputRange{Start: 10, End: 10})
	f.ui()
	client.SetMarkedText(nil, "无导航仍继续原生输入", ui.TextInputRange{Start: 10, End: 10})
	var c *runtimeSubtaskComposer
	for _, item := range other.subtaskComposers {
		if item.target.trace == tr.ID {
			c = item
		}
	}
	snapshot, ok := c.native.handle.TextInputSnapshot(c.native.context)
	if !ok || snapshot.MarkedText != "无导航仍继续原生输入" {
		t.Fatal("queued title-only disposal revoked recovery native lease", snapshot, ok)
	}
	if other.runtimeSelection != string(tr.ID) {
		t.Fatal("unexpected real navigation")
	}
}
