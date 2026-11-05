// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func subtaskEdit(t *testing.T, f *titleFixture, label, text string) {
	t.Helper()
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type(text)
	f.ui()
}
func subtaskAddUI(t *testing.T, f *titleFixture, surface, id, text string) {
	t.Helper()
	subtaskEdit(t, f, subtaskPrefix(surface, id)+".new", text)
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if f.s.runtimeError != "" {
		t.Fatal(f.s.runtimeError)
	}
}
func subtaskFixture(t *testing.T) (*titleFixture, core.DayTrace, core.Subtask) {
	t.Helper()
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	subtaskAddUI(t, f, "day", string(trace.ID), "原子任务")
	g := f.svc.Snapshot()
	if len(g.Subtasks) != 1 {
		t.Fatalf("add did not persist: %+v", g.Subtasks)
	}
	return f, trace, g.Subtasks[0]
}
func subtaskPersisted(t *testing.T, f *titleFixture, id core.SubtaskID) core.Subtask {
	t.Helper()
	for _, r := range f.svc.Snapshot().Subtasks {
		if r.ID == id {
			return r
		}
	}
	t.Fatal("missing child")
	return core.Subtask{}
}
func TestSubtaskUIDayCommandsAndReopen(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	subtaskEdit(t, f, prefix+".title", "  修改后的标题  ")
	f.advance(699 * time.Millisecond)
	if subtaskPersisted(t, f, child.ID).Title != "原子任务" {
		t.Fatal("early save")
	}
	f.advance(time.Millisecond)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "修改后的标题" {
		t.Fatal("missing trim ack")
	}
	liveClick(t, f.tt, prefix+".difficulty")
	if !reflect.DeepEqual(f.tt.Menu(), []string{"简单", "中等", "困难"}) {
		t.Fatal(f.tt.Menu())
	}
	if err := f.tt.ChooseMenuItem("困难"); err != nil {
		t.Fatal(err)
	}
	f.all()
	if subtaskPersisted(t, f, child.ID).Difficulty != core.DifficultyHard {
		t.Fatal("difficulty")
	}
	liveClick(t, f.tt, prefix+".completion")
	f.all()
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskCompleted || f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("child completion auto-completed parent")
	}
	liveClick(t, f.tt, prefix+".completion")
	f.all()
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskPending {
		t.Fatal("undo completion")
	}
	subtaskEdit(t, f, prefix+".title", "Return 保存")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "Return 保存" {
		t.Fatal("Return not flush")
	}
	liveClick(t, f.tt, prefix+".delete")
	f.all()
	deleted := subtaskPersisted(t, f, child.ID)
	if deleted.Status != core.SubtaskCancelledDraft || deleted.CompletedAt != nil || deleted.SettledAt == nil {
		t.Fatal("not tombstone")
	}
	if _, found := f.tt.Find(prefix + ".title"); found {
		t.Fatal("deleted surface retained without dirty draft")
	}
	subtaskAddUI(t, f, "day", string(trace.ID), "接续子任务")
	g := f.svc.Snapshot()
	if len(g.Subtasks) != 2 || g.Subtasks[1].Position != 2 {
		t.Fatal("add reused tombstone slot")
	}
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !reflect.DeepEqual(again.Snapshot(), g) {
		t.Fatal("reopen mismatch")
	}
}
func TestSubtaskUICompositionNavigationAndBlank(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	liveClick(t, f.tt, prefix+".title")
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Compose("未定", 2)
	f.ui()
	f.advance(time.Second)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "原子任务" {
		t.Fatal("saved marked text")
	}
	f.s.selectRuntimeDate(core.LocalDate{Year: 2026, Month: 11, Day: 6}, false)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "未定" || f.s.runtimeDate == liveTestDay {
		t.Fatalf("native navigation finalization %q %v %s", subtaskPersisted(t, f, child.ID).Title, f.s.runtimeDate, f.s.runtimeError)
	}
	f.s.selectRuntimeDate(liveTestDay, false)
	f.all()
	f.s.titleTransition(func() { f.s.runtimeSelection = string(trace.ID) })
	f.all()
	subtaskEdit(t, f, prefix+".title", "   ")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "未定" {
		t.Fatal("blank wrote")
	}
	e := f.s.subtaskEditors[subtaskOwner("day", string(child.ID))]
	if e == nil || e.native.draft != "未定" {
		t.Fatalf("blank not restored %+v", e)
	}
}
func TestSubtaskUIPoolPlannedMaterializesAndOldCallback(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("计划父项"); err != nil {
		t.Fatal(err)
	}
	g := f.svc.Snapshot()
	chain, definition := g.Chains[0].ID, g.Definitions[0].ID
	f.h.publish(g, liveTestDay)
	f.s.page = "pool"
	f.s.runtimeSelection = string(chain)
	f.tt.Frame()
	f.ui()
	subtaskAddUI(t, f, "pool", string(chain), "计划子任务")
	child := f.svc.Snapshot().Definitions[0].PlannedSubtasks[0]
	prefix := subtaskPrefix("pool", string(child.ID))
	if _, found := f.tt.Find(prefix + ".completion"); found {
		t.Fatal("pool invented completion")
	}
	subtaskEdit(t, f, prefix+".title", "计划改名")
	liveClick(t, f.tt, prefix+".difficulty")
	if err := f.tt.ChooseMenuItem("中等"); err != nil {
		t.Fatal(err)
	}
	f.all()
	changed := f.svc.Snapshot().Definitions[0].PlannedSubtasks[0]
	if changed.Title != "计划改名" || changed.Difficulty != core.DifficultyMedium {
		t.Fatal(changed)
	}
	if f.svc.Snapshot().Definitions[0].ID != definition {
		t.Fatal("planned forked definition")
	}
	e := f.s.subtaskEditors[subtaskOwner("pool", string(child.ID))]
	reg, id := e.reg, e.identity
	subtaskEdit(t, f, prefix+".title", "排期前保存")
	liveClick(t, f.tt, "排到今天 "+string(chain))
	f.all()
	g = f.svc.Snapshot()
	if len(g.Subtasks) != 1 || g.Subtasks[0].Title != "排期前保存" || g.Subtasks[0].LineageID != child.LineageID || string(g.Subtasks[0].ID) == string(child.ID) {
		t.Fatal("materialization", g.Subtasks)
	}
	before := g.Clone()
	_ = f.h.coordinator.Observe(reg, id)
	f.all()
	if !reflect.DeepEqual(before, f.svc.Snapshot()) {
		t.Fatal("old callback wrote")
	}
}
func TestSubtaskUITwoSurfacesSeparateOwnersAndFIFO(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	list, detail := subtaskOwner("day-list", string(child.ID)), subtaskOwner("day", string(child.ID))
	a, b := f.s.subtaskEditors[list], f.s.subtaskEditors[detail]
	if a == nil || b == nil || a == b || a.reg == b.reg || a.identity.OwnerID() == b.identity.OwnerID() {
		t.Fatal("shared surface owner")
	}
	subtaskEdit(t, f, subtaskPrefix("day-list", string(child.ID))+".title", "列表草稿")
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "详情较新草稿")
	if a.native.draft != "列表草稿" || b.native.draft != "详情较新草稿" {
		t.Fatal("cross-surface overwrite")
	}
	f.s.selectRuntimeDate(core.LocalDate{Year: 2026, Month: 11, Day: 6}, false)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "详情较新草稿" || f.s.runtimeDate == liveTestDay {
		t.Fatalf("FIFO %q %s", subtaskPersisted(t, f, child.ID).Title, f.s.runtimeError)
	}
}
func TestSubtaskUIFutureReadOnlyCompletionAndHistory(t *testing.T) {
	f := newTitleFixture(t, false)
	future, _ := liveTestDay.AddDays(1)
	if err := f.svc.AddOnDate("未来父项", future); err != nil {
		t.Fatal(err)
	}
	trace := f.svc.Snapshot().Traces[0]
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.s.runtimeDate = future
	f.s.runtimeSelection = string(trace.ID)
	f.s.runtimeDayRail = true
	f.tt.Frame()
	f.ui()
	subtaskAddUI(t, f, "day", string(trace.ID), "未来子任务")
	child := f.svc.Snapshot().Subtasks[0]
	prefix := subtaskPrefix("day", string(child.ID))
	subtaskEdit(t, f, prefix+".title", "未来可改")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "未来可改" {
		t.Fatal("future edit disabled")
	}
	access, ok := f.tt.Access(prefix + ".completion")
	if !ok || !access.Disabled {
		t.Fatal("future toggle enabled")
	}
	liveClick(t, f.tt, prefix+".difficulty")
	if err := f.tt.ChooseMenuItem("困难"); err != nil {
		t.Fatal(err)
	}
	f.all()
	if subtaskPersisted(t, f, child.ID).Difficulty != core.DifficultyHard {
		t.Fatal("future difficulty")
	}
	f.clock.now = f.clock.now.Add(48 * time.Hour)
	f.h.observeDay()
	f.all()
	if s := subtaskPersisted(t, f, child.ID); s.Status != core.SubtaskUnfinished {
		t.Fatal("history settlement", s.Status)
	}
	if _, found := f.tt.Find(prefix + ".delete"); found {
		t.Fatal("historical delete")
	}
	if _, found := f.tt.Find(subtaskPrefix("day", string(trace.ID)) + ".new"); found {
		t.Fatal("historical add")
	}
}
func TestSubtaskUISourceGeometryAndCapture(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	f.tt.SetScale(2)
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	for _, surface := range []string{"day", "day-list"} {
		prefix := subtaskPrefix(surface, string(child.ID))
		check, ok := f.tt.Find(prefix + ".completion")
		if !ok {
			t.Fatal("missing checkbox")
		}
		menu, ok := f.tt.Find(prefix + ".difficulty")
		if !ok {
			t.Fatal("missing menu")
		}
		if check.W != 28 || check.H != 28 || menu.W != 12 || menu.H != 28 {
			t.Fatal("source control geometry", check, menu)
		}
	}
	if root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT"); root != "" {
		dir := filepath.Join(root, "evidence", "subtask-ui")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, "day-list-detail.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	if strings.Contains(f.s.runtimeError, "尚未接通") {
		t.Fatal(f.s.runtimeError)
	}
}

func TestSubtaskUITwoSurfaceFailureRetainsBothDrafts(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	listOwner, detailOwner := subtaskOwner("day-list", string(child.ID)), subtaskOwner("day", string(child.ID))
	subtaskEdit(t, f, subtaskPrefix("day-list", string(child.ID))+".title", "列表未确认草稿")
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "详情已确认草稿")
	calls := 0
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			calls++
			if calls == 1 {
				return os.ErrPermission
			}
		}
		return nil
	})
	f.advance(700 * time.Millisecond)
	f.all()
	a, b := f.s.subtaskEditors[listOwner], f.s.subtaskEditors[detailOwner]
	if a == nil || b == nil || a.failure == nil || a.native.draft != "列表未确认草稿" || a.state.LastAcknowledgedText != "原子任务" || b.native.draft != "详情已确认草稿" || b.state.Dirty {
		t.Fatalf("failure or other ack destroyed draft: a=%+v b=%+v", a, b)
	}
	if subtaskPersisted(t, f, child.ID).Title != "详情已确认草稿" {
		t.Fatal("second surface did not save")
	}
	f.advance(10 * time.Second)
	f.all()
	if a.state.Dirty || a.failure != nil || subtaskPersisted(t, f, child.ID).Title != "列表未确认草稿" {
		t.Fatal("retry did not use original object")
	}
}
func TestSubtaskUIDurableFaultMatrixRetainsAckAndClose(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f, _, child := subtaskFixture(t)
			prefix := subtaskPrefix("day", string(child.ID))
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			subtaskEdit(t, f, prefix+".title", "未确认子项草稿")
			if f.h.beforeClose(f.s) {
				t.Fatal("premature close")
			}
			f.all()
			e := f.s.subtaskEditors[subtaskOwner("day", string(child.ID))]
			if e == nil || e.native.draft != "未确认子项草稿" || e.state.LastAcknowledgedText != "原子任务" || f.closed != 0 || subtaskPersisted(t, f, child.ID).Title != "原子任务" {
				t.Fatal("uncertain acknowledgment or lost draft")
			}
			f.svc.Close()
			again, err := taskapp.Open(f.path, liveTestDay)
			if phase == "before-reserve" || phase == "after-persist" {
				if err != nil {
					t.Fatal(err)
				}
				defer again.Close()
				want := "原子任务"
				if phase == "after-persist" {
					want = "未确认子项草稿"
				}
				if again.Snapshot().Subtasks[0].Title != want {
					t.Fatal("reopen wrong durable fact")
				}
			} else if err == nil {
				again.Close()
				t.Fatal("opened incomplete commit")
			}
		})
	}
}
func TestSubtaskUITwoWindowsDuplicateAndRemoteDeleteDraft(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	other := newRuntimeApp(f.h.readView())
	other.runtimeSelection = string(trace.ID)
	other.runtimeDayRail = true
	otherInvalid := false
	f.h.attach(other, func() { otherInvalid = true }, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	prefix := subtaskPrefix("day", string(child.ID))
	// Both native surfaces capture the same explicit desired state while pending.
	liveClick(t, f.tt, prefix+".completion")
	liveClick(t, tt, prefix+".completion")
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatal("same object command not globally deduplicated", len(f.io.q))
	}
	f.all()
	if otherInvalid {
		tt.Frame()
		f.ui()
		otherInvalid = false
	}
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskCompleted {
		t.Fatal("duplicate toggled twice")
	}
	// A remote command may invalidate another window's hidden native draft, never
	// clear it or silently retarget it to the current selection.
	liveClick(t, tt, prefix+".title")
	tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, tt).ReplaceText(nil, "另一窗口尚未保存")
	liveClick(t, f.tt, prefix+".delete")
	f.all()
	if otherInvalid {
		tt.Frame()
		f.ui()
	}
	e := other.subtaskEditors[subtaskOwner("day", string(child.ID))]
	if e == nil || e.native.draft != "另一窗口尚未保存" || !e.locked || other.runtimeError == "" {
		t.Fatal("remote deletion lost dirty owner")
	}
	if _, ok := tt.Find(prefix + ".title"); !ok {
		t.Fatal("retained draft not visible")
	}
	if f.h.beforeClose(other) {
		t.Fatal("discarded remote dirty draft on close")
	}
}
func TestSubtaskUIParentRenameDoesNotAcknowledgeChild(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	subtaskEdit(t, f, prefix+".title", "子项独立草稿")
	e := f.s.subtaskEditors[subtaskOwner("day", string(child.ID))]
	if _, err := f.svc.SaveTraceTitleInputAcknowledged(trace.ID, "父项改名"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	if e.native.draft != "子项独立草稿" || !e.state.Dirty || e.state.LastAcknowledgedText != "原子任务" {
		t.Fatal("parent ack leaked into child")
	}
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "子项独立草稿" {
		t.Fatal("child failed after parent version change")
	}
}
func TestSubtaskUIPlannedDefinitionFenceRetainsDraft(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddToday("父项"); err != nil {
		t.Fatal(err)
	}
	g := f.svc.Snapshot()
	chain := g.Chains[0].ID
	if err := f.svc.ReturnToPool(chain, g.Traces[0].ID); err != nil {
		t.Fatal(err)
	}
	g = f.svc.Snapshot()
	var definition core.TaskDefinitionID
	for _, d := range g.Definitions {
		if d.SupersededAt == nil {
			definition = d.ID
		}
	}
	if _, err := f.svc.AddPlannedSubtask(chain, definition, "子项", core.DifficultySimple); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.s.page = "pool"
	f.s.runtimeSelection = string(chain)
	f.tt.Frame()
	f.ui()
	var child core.PlannedSubtask
	for _, d := range f.svc.Snapshot().Definitions {
		if d.ID == definition {
			child = d.PlannedSubtasks[0]
		}
	}
	subtaskEdit(t, f, subtaskPrefix("pool", string(child.ID))+".title", "旧definition草稿")
	if err := f.svc.RenameTaskTitle(chain, "父项分叉"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	e := f.s.subtaskEditors[subtaskOwner("pool", string(child.ID))]
	if e == nil || e.subtask.target.definition != definition || e.native.draft != "旧definition草稿" || !e.locked {
		t.Fatal("retargeted planned draft")
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("discarded superseded draft")
	}
	for _, d := range f.svc.Snapshot().Definitions {
		if d.SupersededAt == nil {
			for _, sub := range d.PlannedSubtasks {
				if sub.Title != "子项" {
					t.Fatal("old callback changed current definition")
				}
			}
		}
	}
}
func TestSubtaskUIComposerDuplicateCloseAndLateAck(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "只新增一次")
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.h.beforeClose(f.s) {
		t.Fatal("closed pending new child")
	}
	f.all()
	if len(f.svc.Snapshot().Subtasks) != 1 || f.closed != 1 {
		t.Fatal("duplicate add or close missing")
	}
	f2 := newTitleFixture(t, true)
	trace = f2.svc.Snapshot().Traces[0]
	label = subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f2, label, "保存前输入")
	f2.tt.Key(0, ui.KeyEnter)
	f2.ui()
	f2.s.titleTransition(func() { f2.s.page = "pool"; f2.s.runtimeSelection = "" })
	f2.ui()
	f2.all()
	f2.s.titleTransition(func() { f2.s.page = "day"; f2.s.runtimeSelection = string(trace.ID) })
	f2.all()
	for _, composer := range f2.s.subtaskComposers {
		if composer.mounted && composer.native.draft != "" {
			t.Fatal("offscreen ack left submitted text", composer.native.draft)
		}
	}
}

func TestSubtaskUIComposerFailureAndNewerNativeDraft(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "newer", true: "failure"}[fail], func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			label := subtaskPrefix("day", string(trace.ID)) + ".new"
			subtaskEdit(t, f, label, "提交草稿")
			f.tt.Key(0, ui.KeyEnter)
			f.ui()
			if fail {
				titleStoreFault(f.svc, func(phase string) error {
					if phase == "before-reserve" {
						return os.ErrPermission
					}
					return nil
				})
			} else {
				liveClick(t, f.tt, label)
				f.tt.Key(ui.Cmd, ui.KeyA)
				titleDirectClient(t, f.tt).ReplaceText(nil, "未提交的新草稿")
			}
			f.all()
			for _, composer := range f.s.subtaskComposers {
				want := "未提交的新草稿"
				if fail {
					want = "提交草稿"
				}
				if composer.native.draft != want || composer.pending {
					t.Fatal("draft cleared or pending stuck", composer.native.draft)
				}
			}
			if fail && (len(f.svc.Snapshot().Subtasks) != 0 || f.s.runtimeError == "") {
				t.Fatal("failed add published")
			}
			if !fail && (len(f.svc.Snapshot().Subtasks) != 1 || f.svc.Snapshot().Subtasks[0].Title != "提交草稿") {
				t.Fatal("captured target/text replaced")
			}
		})
	}
}
func TestSubtaskUICloseFlushesDeliveredNativeComposition(t *testing.T) {
	f, _, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	liveClick(t, f.tt, prefix+".title")
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "关闭时已送达", ui.TextInputRange{Start: 6, End: 6})
	if f.h.beforeClose(f.s) {
		t.Fatal("close before persistent ack")
	}
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "关闭时已送达" || f.closed != 1 {
		t.Fatal("native close failed", f.s.runtimeError)
	}
}
func TestSubtaskUILongWrappedTitlesAndPoolScreenshots(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	f.tt.SetScale(2)
	long := "合成：检查较长的子任务标题换行、难度菜单与删除按钮之间的间距，中文与 English 混合内容不能遮挡控件。"
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", long)
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	for _, surface := range []string{"day", "day-list"} {
		prefix := subtaskPrefix(surface, string(child.ID))
		field, _ := f.tt.Find(prefix + ".title")
		difficulty, _ := f.tt.Find(prefix + ".difficulty")
		delete, _ := f.tt.Find(prefix + ".delete")
		if field.X < difficulty.X+difficulty.W+4 || field.X+field.W > delete.X-4+.01 || field.H < 32 || field.H > 120 {
			t.Fatal("overlap or source height", field, difficulty, delete)
		}
	}
	capture := func(name string) {
		root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
		if root == "" {
			return
		}
		im := f.tt.Image()
		if im.Bounds().Dx() != 2400 || im.Bounds().Dy() != 1536 {
			t.Fatal("not native2x")
		}
		dir := filepath.Join(root, "evidence", "subtask-ui")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err = png.Encode(file, im); err != nil {
			t.Fatal(err)
		}
	}
	capture("day-long-2x.png")
	// Shrink to the supported minimum and prove title width is still separate
	// from 28pt controls; then return to baseline for the pool capture.
	f.tt.SetSize(960, 720)
	f.ui()
	field, _ := f.tt.Find(subtaskPrefix("day", string(child.ID)) + ".title")
	if field.W <= 20 {
		t.Fatal("collapsed title at minimum width")
	}
	f.tt.SetSize(1200, 768)
	f.ui()
	f.s.returnTaskToPool(trace.ChainID, trace.ID)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(trace.ChainID) })
	f.all()
	capture("pool-planned-2x.png")
}

func TestSubtaskUIDisclosureUsesObjectScopedKeys(t *testing.T) {
	f := newTitleFixture(t, false)
	for i := 0; i < 3; i++ {
		if err := f.svc.AddToday("合成父项"); err != nil {
			t.Fatal(err)
		}
		g := f.svc.Snapshot()
		tr := g.Traces[len(g.Traces)-1]
		if _, err := f.svc.AddSubtask(tr.ChainID, tr.ID, "合成子项", core.DifficultySimple); err != nil {
			t.Fatal(err)
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.tt.Frame()
	f.ui()
	for _, tr := range f.svc.Snapshot().Traces {
		if _, ok := f.tt.Find("展开子任务 " + string(tr.ID)); !ok {
			t.Fatal("missing intrinsic disclosure")
		}
	}
}
func TestSubtaskUILongScrollFlushesOffscreenDraft(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	for i := 0; i < 24; i++ {
		if _, err := f.svc.AddSubtask(trace.ChainID, trace.ID, "隔离长列表条目", core.DifficultySimple); err != nil {
			t.Fatal(err)
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	subtaskEdit(t, f, subtaskPrefix("day", string(child.ID))+".title", "滚动前的草稿")
	f.tt.Scroll(1190, 600, 0, 3000)
	f.ui()
	if f.h.beforeClose(f.s) {
		t.Fatal("closed before offscreen native proof was persisted")
	}
	f.all()
	if f.closed != 1 || subtaskPersisted(t, f, child.ID).Title != "滚动前的草稿" {
		t.Fatal("scroll lost pending draft", f.s.runtimeError)
	}
}

func TestSubtaskUIPoolRemoveReindexesAndReopens(t *testing.T) {
	f := newTitleFixture(t, false)
	if err := f.svc.AddPool("池父项"); err != nil {
		t.Fatal(err)
	}
	g := f.svc.Snapshot()
	chain := g.Chains[0].ID
	f.h.publish(g, liveTestDay)
	f.s.page = "pool"
	f.s.runtimeSelection = string(chain)
	f.tt.Frame()
	f.ui()
	subtaskAddUI(t, f, "pool", string(chain), "移除前一项")
	subtaskAddUI(t, f, "pool", string(chain), "保留后一项")
	children := f.svc.Snapshot().Definitions[0].PlannedSubtasks
	first, second := children[0], children[1]
	e := f.s.subtaskEditors[subtaskOwner("pool", string(first.ID))]
	reg, id := e.reg, e.identity
	subtaskEdit(t, f, subtaskPrefix("pool", string(first.ID))+".title", "删除之前保存")
	liveClick(t, f.tt, subtaskPrefix("pool", string(first.ID))+".delete")
	f.all()
	remaining := f.svc.Snapshot().Definitions[0].PlannedSubtasks
	if len(remaining) != 1 || remaining[0].ID != second.ID || remaining[0].Position != 1 || remaining[0].Title != "保留后一项" {
		t.Fatal("pool did not remove/reindex", remaining)
	}
	before := f.svc.Snapshot()
	_ = f.h.coordinator.Observe(reg, id)
	f.all()
	if !reflect.DeepEqual(before, f.svc.Snapshot()) {
		t.Fatal("removed planned owner changed replacement")
	}
	f.svc.Close()
	again, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !reflect.DeepEqual(before, again.Snapshot()) {
		t.Fatal("pool removal reopen mismatch")
	}
}
func TestSubtaskUIFutureDeleteKeepsTombstone(t *testing.T) {
	f := newTitleFixture(t, false)
	future, _ := liveTestDay.AddDays(1)
	if err := f.svc.AddOnDate("未来父项", future); err != nil {
		t.Fatal(err)
	}
	trace := f.svc.Snapshot().Traces[0]
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.s.runtimeDate = future
	f.s.runtimeSelection = string(trace.ID)
	f.s.runtimeDayRail = true
	f.tt.Frame()
	f.ui()
	subtaskAddUI(t, f, "day", string(trace.ID), "未来删除项")
	child := f.svc.Snapshot().Subtasks[0]
	liveClick(t, f.tt, subtaskPrefix("day", string(child.ID))+".delete")
	f.all()
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskCancelledDraft {
		t.Fatal("future delete denied or removed entity")
	}
	subtaskAddUI(t, f, "day", string(trace.ID), "未来新增项")
	for _, sub := range f.svc.Snapshot().Subtasks {
		if sub.ID != child.ID && sub.Position != 2 {
			t.Fatal("future reused deleted position")
		}
	}
}
func TestSubtaskUIParentCapabilityRefreshAndCompletedEditing(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	prefix := subtaskPrefix("day", string(child.ID))
	parentLabel := "切换完成 " + string(trace.ID)
	capability, _ := f.tt.Access(parentLabel)
	if !capability.Disabled {
		t.Fatal("open child did not block parent")
	}
	liveClick(t, f.tt, prefix+".completion")
	f.all()
	capability, _ = f.tt.Access(parentLabel)
	if capability.Disabled {
		t.Fatal("complete child did not unlock parent")
	}
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("automatically completed parent")
	}
	subtaskEdit(t, f, prefix+".title", "完成子项仍可改名")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	liveClick(t, f.tt, prefix+".difficulty")
	if err := f.tt.ChooseMenuItem("困难"); err != nil {
		t.Fatal(err)
	}
	f.all()
	if subtaskPersisted(t, f, child.ID).Title != "完成子项仍可改名" || subtaskPersisted(t, f, child.ID).Difficulty != core.DifficultyHard {
		t.Fatal("completed child wrongly made immutable")
	}
	liveClick(t, f.tt, parentLabel)
	f.all()
	childAccess, _ := f.tt.Access(prefix + ".completion")
	if !childAccess.Disabled {
		t.Fatal("child toggles while parent completed")
	}
	if _, ok := f.tt.Find(prefix + ".delete"); ok {
		t.Fatal("child deletion while parent completed")
	}
	liveClick(t, f.tt, parentLabel)
	f.all()
	childAccess, _ = f.tt.Access(prefix + ".completion")
	if childAccess.Disabled {
		t.Fatal("undo parent did not unlock child")
	}
	liveClick(t, f.tt, prefix+".completion")
	f.all()
	if subtaskPersisted(t, f, child.ID).Status != core.SubtaskPending {
		t.Fatal("undo child after parent undo failed")
	}
}

func TestSubtaskUIFailedComposerCloseRetryAndExplicitClear(t *testing.T) {
	for _, mode := range []string{"retry", "clear", "offscreen-quit"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			label := subtaskPrefix("day", string(trace.ID)) + ".new"
			titleStoreFault(f.svc, func(phase string) error {
				if phase == "before-reserve" {
					return os.ErrPermission
				}
				return nil
			})
			subtaskEdit(t, f, label, "添加失败的草稿")
			f.tt.Key(0, ui.KeyEnter)
			f.all()
			if f.h.beforeClose(f.s) || f.s.runtimeError != subtaskComposerFailureMessage {
				t.Fatal("failed add allowed implicit discard")
			}
			switch mode {
			case "retry":
				titleStoreFault(f.svc, func(string) error { return nil })
				liveClick(t, f.tt, label) // FinishTextEditing intentionally revoked the former focus lease.
				f.tt.Key(0, ui.KeyEnter)
				f.all()
				if len(f.svc.Snapshot().Subtasks) != 1 || f.s.runtimeError != "" || !f.h.beforeClose(f.s) {
					t.Fatal("retry did not release close", f.s.runtimeError)
				}
			case "clear":
				subtaskEdit(t, f, label, "")
				if !f.h.beforeClose(f.s) {
					t.Fatal("explicit native erase did not release failed composer")
				}
			case "offscreen-quit":
				f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
				f.all()
				if f.s.page != "pool" {
					t.Fatal("navigation prevents returning to failed object")
				}
				if f.h.beforeQuit() {
					t.Fatal("offscreen failed add lost on quit")
				}
				f.s.titleTransition(func() { f.s.page = "day"; f.s.runtimeSelection = string(trace.ID) })
				f.all()
				subtaskEdit(t, f, label, "")
				if !f.h.beforeQuit() {
					t.Fatal("explicit erase did not release quit")
				}
			}
		})
	}
}

func TestSubtaskUIDisclosureFollowsIntrinsicContent(t *testing.T) {
	f, trace, _ := subtaskFixture(t)
	row, _ := f.tt.Find("查看任务 " + string(trace.ID))
	capsule, _ := f.tt.Find("展开子任务 " + string(trace.ID))
	if capsule.X-row.X > 140 {
		t.Fatalf("short title capsule is fixed at row trailing edge: row=%v capsule=%v", row, capsule)
	}
	title, found := f.tt.Find("原题")
	if !found || math.Abs(float64(capsule.X-(title.X+title.W+12))) > .5 {
		t.Fatal("short title does not use source content plus 12pt spacing", title, capsule)
	}
	longTitle := strings.Repeat("较长父任务标题", 25)
	f.edit(longTitle)
	f.advance(700 * time.Millisecond)
	f.all()
	row, _ = f.tt.Find("查看任务 " + string(trace.ID))
	capsule, _ = f.tt.Find("展开子任务 " + string(trace.ID))
	title, _ = f.tt.Find(longTitle)
	if math.Abs(float64(capsule.X-(title.X+title.W+12))) > .5 {
		t.Fatal("long title/accessory overlap", title, capsule)
	}
	if capsule.X < row.X+52 || capsule.X+capsule.W > row.X+row.W-11.5 {
		t.Fatal("long title capsule overlaps or exceeds row", row, capsule)
	}
}
func TestSubtaskUIFailedPlannedComposerKeepsOriginalDefinitionReachable(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	if err := f.svc.ReturnToPool(trace.ChainID, trace.ID); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = string(trace.ChainID) })
	f.all()
	label := subtaskPrefix("pool", string(trace.ChainID)) + ".new"
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	subtaskEdit(t, f, label, "旧definition新增失败")
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	var failed *runtimeSubtaskComposer
	for _, c := range f.s.subtaskComposers {
		if c.failure != nil {
			failed = c
		}
	}
	if failed == nil {
		t.Fatal("missing failure")
	}
	original := failed.target
	liveClick(t, f.tt, label)
	titleDirectClient(t, f.tt).SetMarkedText(nil, "又送达的候选", ui.TextInputRange{Start: 6, End: 6})
	titleStoreFault(f.svc, nil)
	if err := f.svc.RenameTaskTitle(trace.ChainID, "新父标题版本"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	if snapshot, ok := failed.native.handle.TextInputSnapshot(failed.native.context); !ok || !strings.Contains(snapshot.Text+snapshot.MarkedText, "又送达的候选") {
		t.Fatal("definition fork dropped native marked input", snapshot, ok)
	}
	if failed.target != original || !failed.mounted {
		t.Fatal("old definition composer inaccessible or retargeted")
	}
	liveClick(t, f.tt, label)
	f.tt.Key(ui.Cmd, ui.KeyA)
	titleDirectClient(t, f.tt).ReplaceText(nil, "")
	if !f.h.beforeClose(f.s) {
		t.Fatal("cannot explicitly clear original failed owner")
	}
}

func TestSubtaskUIFailedActualComposerRemainsEditableForRecovery(t *testing.T) {
	for _, action := range []string{"complete", "return", "history"} {
		t.Run(action, func(t *testing.T) {
			f := newTitleFixture(t, true)
			trace := f.svc.Snapshot().Traces[0]
			label := subtaskPrefix("day", string(trace.ID)) + ".new"
			subtaskEdit(t, f, label, "失败草稿")
			titleStoreFault(f.svc, func(phase string) error {
				if phase == "before-reserve" {
					return os.ErrPermission
				}
				return nil
			})
			f.tt.Key(0, ui.KeyEnter)
			f.all()
			var failed *runtimeSubtaskComposer
			for _, c := range f.s.subtaskComposers {
				if c.failure != nil {
					failed = c
				}
			}
			if failed == nil {
				t.Fatal("no failure")
			}
			original := failed.target
			liveClick(t, f.tt, label)
			titleDirectClient(t, f.tt).SetMarkedText(nil, "原生候选", ui.TextInputRange{Start: 4, End: 4})
			titleStoreFault(f.svc, nil)
			switch action {
			case "complete":
				if err := f.svc.SetCompleted(trace.ID, true); err != nil {
					t.Fatal(err)
				}
			case "return":
				if err := f.svc.ReturnToPool(trace.ChainID, trace.ID); err != nil {
					t.Fatal(err)
				}
			case "history":
				next, _ := liveTestDay.AddDays(1)
				f.clock.now = f.clock.now.Add(24 * time.Hour)
				if err := f.svc.AdvanceTo(next); err != nil {
					t.Fatal(err)
				}
			}
			if action == "history" {
				for _, current := range f.svc.Snapshot().Traces {
					if current.ID == trace.ID && (current.Status == core.TracePending || current.Date.Compare(f.svc.Today()) >= 0) {
						t.Fatal("fixture did not become historical", current, f.svc.Today())
					}
				}
			}
			f.h.publish(f.svc.Snapshot(), f.svc.Today())
			f.all()
			if failed.target != original || !failed.mounted {
				t.Fatal("failed original not mounted", action)
			}
			snapshot, ok := failed.native.handle.TextInputSnapshot(failed.native.context)
			if !ok || !strings.Contains(snapshot.Text+snapshot.MarkedText, "原生候选") {
				t.Fatal("remote status change lost native marked input", action, snapshot, ok)
			}
			if f.h.beforeClose(f.s) {
				t.Fatal("uncleared failure allowed close")
			}
			if action == "return" && os.Getenv("NOONMARK_MYGO_BUILD_ROOT") != "" {
				f.tt.SetScale(2)
				dir := filepath.Join(os.Getenv("NOONMARK_MYGO_BUILD_ROOT"), "evidence", "subtask-ui")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				out, err := os.Create(filepath.Join(dir, "recovery-original-2x.png"))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(out, f.tt.Image()); err != nil {
					t.Fatal(err)
				}
				out.Close()
			}
			liveClick(t, f.tt, label)
			f.tt.Key(ui.Cmd, ui.KeyA)
			titleDirectClient(t, f.tt).ReplaceText(nil, "")
			if !f.h.beforeClose(f.s) {
				t.Fatal("fresh explicit erase cannot release original failed draft")
			}
		})
	}
}
