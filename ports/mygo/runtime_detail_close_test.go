// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"testing"
)

const detailCloseLabel = "关闭任务详情"

func detailCloseReview(t *testing.T, f *titleFixture) {
	t.Helper()
	if f.s.runtimeSelection != "" || !f.s.runtimeDayRail || f.s.page != "day" || !f.tt.HasText("每日复盘") {
		t.Fatalf("close did not return to expanded day review: selection=%q rail=%v page=%s", f.s.runtimeSelection, f.s.runtimeDayRail, f.s.page)
	}
}

func TestDetailCloseDirtyParentWaitsAndDeduplicates(t *testing.T) {
	f := newTitleFixture(t, true)
	selected := f.s.runtimeSelection
	f.edit("关闭前父任务草稿")
	liveClick(t, f.tt, detailCloseLabel)
	f.ui()
	if f.s.runtimeSelection != selected || f.s.titleEditor == nil || len(f.io.q) != 1 {
		t.Fatal("close bypassed save acknowledgment")
	}
	liveClick(t, f.tt, detailCloseLabel)
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatal("repeated close submitted duplicate save")
	}
	f.all()
	detailCloseReview(t, f)
	if f.persisted() != "关闭前父任务草稿" {
		t.Fatal("parent draft lost")
	}
	f.s.titleTransition(func() { f.s.runtimeSelection = selected })
	f.all()
	if f.s.titleEditor == nil || f.s.titleEditor.native.draft != "关闭前父任务草稿" {
		t.Fatal("reopen lost title")
	}
	liveClick(t, f.tt, "收起日待办详情")
	f.all()
	if f.s.runtimeDayRail || f.s.runtimeSelection != selected {
		t.Fatal("pane collapse confused with selection close")
	}
}

func TestDetailCloseParentAndChildSaveFailureVeto(t *testing.T) {
	for _, childCase := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "child"}[childCase], func(t *testing.T) {
			f, _, child := subtaskFixture(t)
			selected := f.s.runtimeSelection
			if childCase {
				subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "关闭失败保留子任务")
			} else {
				f.edit("关闭失败保留父任务")
			}
			titleStoreFault(f.svc, func(phase string) error {
				if phase == "before-reserve" {
					return os.ErrPermission
				}
				return nil
			})
			liveClick(t, f.tt, detailCloseLabel)
			f.all()
			if f.s.runtimeSelection != selected || !f.s.runtimeDayRail || f.s.runtimeError == "" {
				t.Fatal("failed save permitted close or hid error")
			}
			if childCase {
				e := f.s.subtaskEditors[subtaskOwner("day", string(child.ID))]
				if e == nil || e.native.draft != "关闭失败保留子任务" || subtaskPersisted(t, f, child.ID).Title != "原子任务" {
					t.Fatal("child failure lost draft or published failed write")
				}
			} else if f.s.titleEditor == nil || f.s.titleEditor.native.draft != "关闭失败保留父任务" || f.persisted() != "原题" {
				t.Fatal("parent failure lost draft or published failed write")
			}
		})
	}
}

func TestDetailCloseDirtyChildPersists(t *testing.T) {
	f, _, child := subtaskFixture(t)
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "关闭前子任务草稿")
	liveClick(t, f.tt, detailCloseLabel)
	f.ui()
	if f.s.runtimeSelection == "" {
		t.Fatal("child close before save acknowledgment")
	}
	f.all()
	detailCloseReview(t, f)
	if subtaskPersisted(t, f, child.ID).Title != "关闭前子任务草稿" {
		t.Fatal("child draft lost")
	}
}

func TestDetailCloseComposerRetainedAcrossReturn(t *testing.T) {
	for _, state := range []string{"draft", "pending", "failure"} {
		t.Run(state, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			selected := string(tr.ID)
			label := subtaskPrefix("day", selected) + ".new"
			subtaskEdit(t, f, label, "关闭时新增草稿")
			if state != "draft" {
				if state == "failure" {
					titleStoreFault(f.svc, func(phase string) error {
						if phase == "before-reserve" {
							return os.ErrPermission
						}
						return nil
					})
				}
				f.tt.Key(0, ui.KeyEnter)
				f.ui()
				if state == "failure" {
					f.all()
				}
			}
			liveClick(t, f.tt, detailCloseLabel)
			f.all()
			detailCloseReview(t, f)
			f.s.titleTransition(func() { f.s.runtimeSelection = selected })
			f.all()
			var found bool
			for _, c := range f.s.subtaskComposers {
				if c.target.trace == tr.ID && c.mounted {
					found = true
					want := "关闭时新增草稿"
					if state == "pending" {
						want = ""
					}
					if c.native.draft != want {
						t.Fatalf("composer lost or duplicate draft: %q want %q", c.native.draft, want)
					}
					if state == "failure" && (c.failure == nil || f.h.beforeClose(f.s)) {
						t.Fatal("failed explicit add no longer recoverable")
					}
				}
			}
			if !found {
				t.Fatal("composer not remounted")
			}
			wantCount := 0
			if state == "pending" {
				wantCount = 1
			}
			if len(f.svc.Snapshot().Subtasks) != wantCount {
				t.Fatal("duplicate or unintended child submission")
			}
		})
	}
}

func TestDetailCloseLateCompletionCannotOverrideNewSelection(t *testing.T) {
	f := newTitleFixture(t, true)
	if err := f.svc.AddToday("另一个任务"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	second := string(f.svc.Snapshot().Traces[1].ID)
	f.edit("先保存的任务")
	liveClick(t, f.tt, detailCloseLabel)
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatal("save not queued")
	}
	f.io.one()
	f.s.titleTransition(func() { f.s.runtimeSelection = second })
	f.all()
	if f.s.runtimeSelection != second || f.persisted() != "先保存的任务" {
		t.Fatal("stale close stole newer navigation or draft")
	}
}

func TestDetailCloseKeyboardActivation(t *testing.T) {
	for _, key := range []ui.Key{ui.KeyEnter, ui.KeySpace} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			f := newTitleFixture(t, true)
			f.edit("键盘关闭前标题")
			for i := 0; i < 100 && !f.tt.Focused(detailCloseLabel); i++ {
				f.tt.Key(0, ui.KeyTab)
				f.ui()
			}
			if !f.tt.Focused(detailCloseLabel) {
				t.Fatal("close not reachable by Tab")
			}
			f.tt.Key(0, key)
			f.all()
			detailCloseReview(t, f)
			if f.persisted() != "键盘关闭前标题" {
				t.Fatal("keyboard close lost dirty title")
			}
		})
	}
}

func TestDetailCloseDeliveredNativeComposition(t *testing.T) {
	for _, childCase := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "child"}[childCase], func(t *testing.T) {
			f, _, child := subtaskFixture(t)
			label := "日任务详情标题"
			if childCase {
				label = subtaskPrefix("day", string(child.ID)) + ".title"
			}
			liveClick(t, f.tt, label)
			f.tt.Key(ui.Cmd, ui.KeyA)
			titleDirectClient(t, f.tt).SetMarkedText(nil, "关闭时中文候选", ui.TextInputRange{Start: 7, End: 7})
			liveClick(t, f.tt, detailCloseLabel)
			f.all()
			detailCloseReview(t, f)
			got := f.persisted()
			if childCase {
				got = subtaskPersisted(t, f, child.ID).Title
			}
			if got != "关闭时中文候选" {
				t.Fatalf("delivered native composition lost: %q", got)
			}
		})
	}
}

func TestDetailCloseLongScrollRetainsOffscreenChild(t *testing.T) {
	f, tr, child := subtaskFixture(t)
	for i := 0; i < 24; i++ {
		if _, err := f.svc.AddSubtask(tr.ChainID, tr.ID, "隔离滚动测试子任务", core.DifficultySimple); err != nil {
			t.Fatal(err)
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "滚动后关闭保留草稿")
	f.tt.Scroll(1190, 600, 0, 3000)
	f.ui()
	liveClick(t, f.tt, detailCloseLabel)
	f.all()
	detailCloseReview(t, f)
	if subtaskPersisted(t, f, child.ID).Title != "滚动后关闭保留草稿" {
		t.Fatal("offscreen child draft lost")
	}
}
