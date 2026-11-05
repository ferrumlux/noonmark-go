// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestReviewRecoveryActualHistoryPreservesMarkedText(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	subtaskEdit(t, f, label, "跨日新增失败")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	var c *runtimeSubtaskComposer
	for _, v := range f.s.subtaskComposers {
		if v.failure != nil {
			c = v
		}
	}
	if c == nil {
		t.Fatal("no failure")
	}
	original := c.target
	liveClick(t, f.tt, label)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "跨日原生候选", ui.TextInputRange{Start: 6, End: 6})
	titleStoreFault(f.svc, nil)
	tomorrow, err := liveTestDay.AddDays(1)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), f.svc.Today())
	f.all()
	if f.svc.Today() != tomorrow || f.svc.Snapshot().Traces[0].Date.Compare(f.svc.Today()) >= 0 {
		t.Fatal("parent never became historical")
	}
	snap, ok := c.native.handle.TextInputSnapshot(c.native.context)
	if c.target != original || !c.mounted || !ok || !strings.Contains(snap.Text+snap.MarkedText, "跨日原生候选") {
		t.Fatalf("history dropped original marked text: %+v %v mounted=%v", snap, ok, c.mounted)
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("historical failure disappeared")
	}
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).ReplaceText(nil, "")
	if !f.h.beforeClose(f.s) {
		t.Fatal("historical original clear inaccessible")
	}
}

func TestReviewRecoveryCannotWriteIntoNewPoolTarget(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	subtaskEdit(t, f, label, "原actual新增失败")
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	var c *runtimeSubtaskComposer
	for _, v := range f.s.subtaskComposers {
		if v.failure != nil {
			c = v
		}
	}
	original := c.target
	titleStoreFault(f.svc, nil)
	if err := f.svc.ReturnToPool(tr.ChainID, tr.ID); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(tr.ChainID) })
	f.all()
	before := f.svc.Snapshot()
	if !f.tt.HasText("原日期 " + tr.Date.String() + "的新增草稿尚未确认；可重试或清空。") {
		t.Fatal("recovery lacks original-object unsaved label")
	}
	subtaskEdit(t, f, label, "绝不能新增到新planned")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if c.target != original || c.failure == nil || !reflect.DeepEqual(before, f.svc.Snapshot()) {
		t.Fatal("recovery wrote into replacement target or cleared failure")
	}
	subtaskEdit(t, f, label, "")
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(tr.ChainID) })
	f.all()
	if c.failure != nil {
		t.Fatal("explicit clear did not release original failure")
	}
	for _, v := range f.s.subtaskComposers {
		if v.mounted && (v.target.trace != "" || v.native.draft != "") {
			t.Fatal("old actual draft retargeted into new planned composer")
		}
	}
	if !reflect.DeepEqual(before, f.svc.Snapshot()) {
		t.Fatal("clear changed persistent data")
	}
}

func TestReviewForkedPlannedDirtyEditorIsLabeledRecovery(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	if err := f.svc.ReturnToPool(tr.ChainID, tr.ID); err != nil {
		t.Fatal(err)
	}
	var def core.TaskDefinitionID
	for _, d := range f.svc.Snapshot().Definitions {
		if d.SupersededAt == nil {
			def = d.ID
		}
	}
	id, err := f.svc.AddPlannedSubtask(tr.ChainID, def, "已保存计划子项", core.DifficultySimple)
	if err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(tr.ChainID) })
	f.all()
	label := subtaskPrefix("pool", string(id)) + ".title"
	subtaskEdit(t, f, label, "旧definition尚未确认草稿")
	if err := f.svc.RenameTaskTitle(tr.ChainID, "父标题合法分叉"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	e := f.s.subtaskEditors[subtaskOwner("pool", string(id))]
	if e == nil || e.subtask.target.definition != def || e.native.draft != "旧definition尚未确认草稿" || !e.locked {
		t.Fatal("old native draft not safely retained")
	}
	if !f.tt.HasText("原对象的未保存草稿") {
		t.Fatal("old-definition dirty editor is rendered as the current saved child without a recovery label")
	}
}

func TestReviewClearingOneFailedComposerKeepsOther(t *testing.T) {
	f := newTitleFixture(t, true)
	first := f.svc.Snapshot().Traces[0]
	if err := f.svc.AddToday("另一个原对象"); err != nil {
		t.Fatal(err)
	}
	second := f.svc.Snapshot().Traces[1]
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	firstLabel := subtaskPrefix("day", string(first.ID)) + ".new"
	subtaskEdit(t, f, firstLabel, "第一对象失败草稿")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	f.s.titleTransition(func() { f.s.runtimeSelection = string(second.ID) })
	f.all()
	secondLabel := subtaskPrefix("day", string(second.ID)) + ".new"
	subtaskEdit(t, f, secondLabel, "第二对象失败草稿")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	var a, b *runtimeSubtaskComposer
	for _, c := range f.s.subtaskComposers {
		if c.target.trace == first.ID {
			a = c
		}
		if c.target.trace == second.ID {
			b = c
		}
	}
	if a == nil || b == nil || a.failure == nil || b.failure == nil {
		t.Fatal("two failed owner setup failed")
	}
	subtaskEdit(t, f, secondLabel, "")
	if f.h.beforeClose(f.s) {
		t.Fatal("clearing one owner discarded the other's failure")
	}
	if a.failure == nil || a.native.draft != "第一对象失败草稿" || b.failure != nil {
		t.Fatal("clear crossed owner or failed native proof")
	}
	f.s.titleTransition(func() { f.s.runtimeSelection = string(first.ID) })
	f.all()
	subtaskEdit(t, f, firstLabel, "")
	if !f.h.beforeClose(f.s) {
		t.Fatal("both explicit clears did not release close")
	}
	if len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("clear created child")
	}
}

func TestReviewUnsubmittedComposerRemainsTransient(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	subtaskEdit(t, f, subtaskPrefix("day", string(tr.ID))+".new", "仅输入未提交")
	if !f.h.beforeClose(f.s) {
		t.Fatal("ordinary transient composer incorrectly blocks close")
	}
	if len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("ordinary transient composer was automatically submitted")
	}
}
