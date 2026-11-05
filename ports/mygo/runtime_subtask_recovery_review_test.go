// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"os"
	"testing"
)

func TestReviewFailedComposerRemainsRecoverableAfterRemoteReturn(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "原任务新增失败草稿")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if f.h.beforeClose(f.s) {
		t.Fatal("initial failed draft did not block close")
	}
	titleStoreFault(f.svc, nil)
	// The other window is authorized to return the clean parent. This leaves the
	// failed composer attached to its old actual trace, never the new planned one.
	if err := f.svc.ReturnToPool(trace.ChainID, trace.ID); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(trace.ChainID) })
	f.all()
	if f.h.beforeClose(f.s) {
		t.Fatal("remote return silently discarded failed composer")
	}
	// Navigate back to the originally selected parent through the runtime path.
	f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
	f.all()
	reachable := false
	for _, c := range f.s.subtaskComposers {
		if c.target.trace == trace.ID && c.failure != nil && c.mounted && c.native.draft == "原任务新增失败草稿" {
			reachable = true
		}
	}
	if !reachable {
		t.Fatal("Close/Quit remains blocked, but original failed composer can no longer mount after parent return; user cannot retry or explicitly clear it")
	}
}

func TestReviewRetainedDeletedDraftDoesNotCountAsSavedChild(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	label := subtaskPrefix("day-list", string(child.ID)) + ".title"
	subtaskEdit(t, f, label, "被删子项未保存草稿")
	if err := f.svc.DeleteSubtask(trace.ChainID, trace.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	e := f.s.subtaskEditors[subtaskOwner("day-list", string(child.ID))]
	if e == nil || e.native.draft != "被删子项未保存草稿" {
		t.Fatal("original failed child draft lost")
	}
	if len(actualSubtaskRows(f.svc.Snapshot(), trace.ChainID, trace.ID, liveTestDay)) != 0 {
		t.Fatal("fixture still has a saved visible child")
	}
	for _, text := range f.tt.Texts() {
		if text == "1 子任务" || text == "子任务 1" {
			t.Fatal("retained deleted draft is falsely counted as a saved current child")
		}
	}
}
