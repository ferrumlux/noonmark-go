// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
	"reflect"
	"testing"
	"time"
)

func openCarryoverMenu(t *testing.T, f *titleFixture, kind carryoverPickerKind) {
	t.Helper()
	tr := f.svc.Snapshot().Traces[0]
	openReturnMenu(t, f.tt, tr.ID)
	label := deferTaskLabel
	if kind == carryoverContinue {
		label = continueTaskLabel
	}
	if err := f.tt.ChooseMenuItem(label); err != nil {
		t.Fatal(err)
	}
	f.all()
	f.tt.Frame()
	if f.s.carryover == nil {
		t.Fatal("picker did not open", f.s.runtimeError)
	}
}
func confirmCarryover(t *testing.T, f *titleFixture) {
	t.Helper()
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if f.s.carryover == nil || !f.s.carryover.pending {
		t.Fatal("confirmation not asynchronous")
	}
}
func TestCarryoverDayNativeMenuDraftCancelAndDeferredReopen(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	before := f.svc.Snapshot()
	disk := calendarDiskSignature(t, f.path)
	openCarryoverMenu(t, f, carryoverDefer)
	f.tt.Key(0, ui.KeyRight)
	f.tt.Key(0, ui.KeyEscape)
	f.all()
	if f.s.carryover != nil || !reflect.DeepEqual(before, f.svc.Snapshot()) || !reflect.DeepEqual(disk, calendarDiskSignature(t, f.path)) {
		t.Fatal("draft/cancel wrote")
	}
	openCarryoverMenu(t, f, carryoverDefer)
	target := f.s.carryover.picker.selected
	confirmCarryover(t, f)
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	if len(f.io.q) != 1 || len(f.svc.Snapshot().Traces) != 1 {
		t.Fatal("early/duplicate write")
	}
	f.all()
	g := f.svc.Snapshot()
	if len(g.Traces) != 2 || f.s.carryover != nil || f.s.runtimeDate != target || f.s.runtimeSelection == string(tr.ID) || f.s.runtimeError != "" {
		t.Fatal("success navigation", f.s.runtimeError)
	}
	id := f.s.runtimeSelection
	f.svc.Close()
	again, err := taskapp.Open(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !reflect.DeepEqual(g, again.Snapshot()) {
		t.Fatal("reopen mismatch")
	}
	found := false
	for _, r := range again.Snapshot().Traces {
		if string(r.ID) == id && r.Date == target {
			found = true
		}
	}
	if !found {
		t.Fatal("unacknowledged id")
	}
}
func TestCarryoverDayContinueHistoricalNative(t *testing.T) {
	f := newTitleFixture(t, true)
	source := f.svc.Snapshot().Traces[0]
	tomorrow, _ := liveTestDay.AddDays(1)
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	if err := f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), tomorrow)
	f.ui()
	f.tt.Frame()
	openCarryoverMenu(t, f, carryoverContinue)
	confirmCarryover(t, f)
	f.all()
	g := f.svc.Snapshot()
	if len(g.Traces) != 2 || g.Traces[0].Status != core.TraceUnfinished || f.s.runtimeDate != tomorrow || f.s.runtimeSelection == string(source.ID) || f.s.runtimeError != "" {
		t.Fatal("continue", g.Traces, f.s.runtimeError)
	}
}
func TestCarryoverDayMenuSourceVisibilityNotMutationPolicy(t *testing.T) {
	f := newTitleFixture(t, true)
	g := f.svc.Snapshot()
	tr := g.Traces[0]
	row := taskview.DayRow{TraceID: tr.ID, ChainID: tr.ChainID, Status: tr.Status}
	d, c := carryoverMenuCapabilities(g, row, liveTestDay)
	if !d || c {
		t.Fatal("today")
	}
	tomorrow, _ := liveTestDay.AddDays(1)
	g.Traces[0].Status = core.TraceUnfinished
	row.Status = core.TraceUnfinished
	d, c = carryoverMenuCapabilities(g, row, tomorrow)
	if d || !c {
		t.Fatal("history")
	}
	// Menu must not require a frontier/pool lookup; the real Service checks it.
	g.Chains[0].State = core.ChainAbandoned
	_, c = carryoverMenuCapabilities(g, row, tomorrow)
	if c {
		t.Fatal("abandoned")
	}
	g.Chains[0].State = core.ChainActive
	pending := tr
	pending.ID = "synthetic-pending"
	g.Traces = append(g.Traces, pending)
	_, c = carryoverMenuCapabilities(g, row, tomorrow)
	if c {
		t.Fatal("pending")
	}
	g.Traces = g.Traces[:1]
	row.TraceID = "missing"
	_, c = carryoverMenuCapabilities(g, row, tomorrow)
	if c {
		t.Fatal("identity")
	}
}
func TestCarryoverDaySourceChangedWhilePickerOpen(t *testing.T) {
	for _, change := range []string{"complete", "return", "defer", "midnight"} {
		t.Run(change, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			openCarryoverMenu(t, f, carryoverDefer)
			next, _ := liveTestDay.AddDays(2)
			switch change {
			case "complete":
				if err := f.svc.SetCompleted(tr.ID, true); err != nil {
					t.Fatal(err)
				}
			case "return":
				if err := f.svc.ReturnToPool(tr.ChainID, tr.ID); err != nil {
					t.Fatal(err)
				}
			case "defer":
				if _, err := f.svc.DeferCurrentTrace(tr.ChainID, tr.ID, next); err != nil {
					t.Fatal(err)
				}
			case "midnight":
				f.clock.now = f.clock.now.Add(24 * time.Hour)
			}
			// Keep confirmed old view until command publication, as in a queued worker.
			confirmCarryover(t, f)
			f.all()
			if f.s.carryover != nil || (change != "midnight" && (f.s.runtimeDate != liveTestDay || f.s.runtimeSelection != string(tr.ID))) || f.s.runtimeError == "" {
				t.Fatal("changed source escaped", f.s.runtimeError)
			}
		})
	}
}
func TestCarryoverDayInvalidFreshDateRetainsSheet(t *testing.T) {
	f := newTitleFixture(t, true)
	openCarryoverMenu(t, f, carryoverDefer)
	p := f.s.carryover
	tomorrow, _ := liveTestDay.AddDays(1)
	f.h.today = tomorrow
	f.tt.Key(0, ui.KeyEnter)
	f.all()
	if f.s.carryover != p || p.picker.validation == "" || len(f.svc.Snapshot().Traces) != 1 {
		t.Fatal("invalid date")
	}
}
func TestCarryoverDayMutationFaultClosesOnlyOriginalAndNoRetry(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			openCarryoverMenu(t, f, carryoverDefer)
			source := f.s.runtimeSelection
			calls := 0
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					calls++
					return errors.New("synthetic private failure")
				}
				return nil
			})
			confirmCarryover(t, f)
			f.all()
			if f.s.carryover != nil || f.s.runtimeSelection != source || f.s.runtimeDate != liveTestDay || f.s.runtimeError == "" {
				t.Fatal("false success")
			}
			for i := 0; i < 5; i++ {
				f.tt.Frame()
				f.all()
			}
			if calls != 1 {
				t.Fatal("automatic retry", calls)
			}
			if phase != "before-reserve" {
				tr := f.svc.Snapshot().Traces[0]
				target, _ := liveTestDay.AddDays(1)
				id, err := f.svc.DeferCurrentTrace(tr.ChainID, tr.ID, target)
				if err == nil || id != "" || calls != 1 {
					t.Fatal("uncertain store handle reused")
				}
			}
			f.svc.Close()
			again, err := taskapp.Open(f.path, liveTestDay)
			if phase == "after-reserve" || phase == "before-commit" {
				if err == nil {
					again.Close()
					t.Fatal("incomplete reopened")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			want := 1
			if phase == "after-persist" {
				want = 2
			}
			if len(again.Snapshot().Traces) != want {
				t.Fatal("disk outcome")
			}
		})
	}
}
func TestCarryoverDayLateCompletionCannotStealPurposeNavigationOrClose(t *testing.T) {
	for _, mode := range []string{"new-purpose", "navigation", "close", "quit", "new-session"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			openCarryoverMenu(t, f, carryoverDefer)
			confirmCarryover(t, f)
			original := f.s.carryover
			switch mode {
			case "new-purpose":
				f.tt.Key(0, ui.KeyEscape)
				f.ui()
				tr := f.svc.Snapshot().Traces[0]
				f.s.openCarryover(carryoverDefer, taskview.DayRow{TraceID: tr.ID, ChainID: tr.ChainID, Status: tr.Status})
				f.ui()
				f.tt.Frame()
				if f.s.carryover == nil || f.s.carryover == original {
					t.Fatal("fresh purpose absent")
				}
			case "navigation":
				f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
				f.ui()
			case "close":
				if f.h.beforeClose(f.s) {
					t.Fatal("closed before commit")
				}
			case "quit":
				if f.h.beforeQuit() {
					t.Fatal("quit before commit")
				}
			case "new-session":
				delete(f.h.windows, f.s.titleWindowID)
			}
			f.all()
			if len(f.svc.Snapshot().Traces) != 2 {
				t.Fatal("lost accepted operation")
			}
			switch mode {
			case "new-purpose":
				if f.s.carryover == nil || f.s.carryover == original || f.s.runtimeDate != liveTestDay {
					t.Fatal("new purpose stolen")
				}
			case "navigation":
				if f.s.page != "pool" || f.s.runtimeSelection != "" {
					t.Fatal("navigation stolen")
				}
			default:
				if f.s.runtimeDate != liveTestDay {
					t.Fatal("late selection")
				}
			}
		})
	}
}
func TestCarryoverDayTwoWindowsDifferentTargetsOneWinner(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	openCarryoverMenu(t, f, carryoverDefer)
	openReturnMenu(t, tt, tr.ID)
	if err := tt.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.ui()
	tt.Frame()
	if other.carryover == nil {
		t.Fatal("second picker")
	}
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	f.ui()
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 || (f.s.runtimeError == "") == (other.runtimeError == "") {
		t.Fatal("not exactly one winner", f.s.runtimeError, other.runtimeError)
	}
}
func TestCarryoverDayOpeningWaitsForNativeTitlePersistence(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	f.edit("先存标题")
	f.advance(700 * time.Millisecond)
	f.edit("最终标题")
	openReturnMenu(t, f.tt, tr.ID)
	if err := f.tt.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.ui()
	if f.s.carryover != nil {
		t.Fatal("opened before native acknowledgement")
	}
	f.all()
	f.tt.Frame()
	if f.s.carryover == nil || f.persisted() != "最终标题" {
		t.Fatal("barrier failed")
	}
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("defer failed")
	}
}
func TestCarryoverDayFailedTitleBarrierPreservesDraft(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			f.edit("保留草稿")
			titleStoreFault(f.svc, func(at string) error {
				if at == phase {
					return taskapp.ErrIncomplete
				}
				return nil
			})
			openReturnMenu(t, f.tt, tr.ID)
			if err := f.tt.ChooseMenuItem(deferTaskLabel); err != nil {
				t.Fatal(err)
			}
			f.all()
			if f.s.carryover != nil || len(f.svc.Snapshot().Traces) != 1 || f.s.titleEditor == nil || f.s.titleEditor.native.draft != "保留草稿" || f.s.runtimeError == "" {
				t.Fatal("failed barrier discarded draft")
			}
		})
	}
}

func TestCarryoverDayNativeCompositionBarriers(t *testing.T) {
	for _, surface := range []string{"title", "subtask", "composer"} {
		t.Run(surface, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			label := "日任务详情标题"
			var child core.SubtaskID
			if surface == "subtask" {
				var r core.DayTrace
				var c core.Subtask
				f, r, c = subtaskFixture(t)
				tr = r
				child = c.ID
				label = subtaskPrefix("day", string(child)) + ".title"
			}
			if surface == "composer" {
				label = subtaskPrefix("day", string(tr.ID)) + ".new"
			}
			liveClick(t, f.tt, label)
			f.tt.Key(ui.Cmd, ui.KeyA)
			titleDirectClient(t, f.tt).SetMarkedText(nil, "原生组合字", ui.TextInputRange{Start: 5, End: 5})
			openReturnMenu(t, f.tt, tr.ID)
			if err := f.tt.ChooseMenuItem(deferTaskLabel); err != nil {
				t.Fatal(err)
			}
			f.ui()
			if f.s.carryover != nil {
				t.Fatal("opened before native persistence")
			}
			f.all()
			f.tt.Frame()
			if surface == "composer" {
				if f.s.carryover != nil || len(f.svc.Snapshot().Subtasks) != 0 {
					t.Fatal("unsubmitted composer was committed")
				}
				liveClick(t, f.tt, label)
				f.tt.Key(0, ui.KeyEnter)
				f.all()
				openCarryoverMenu(t, f, carryoverDefer)
			}
			if f.s.carryover == nil {
				t.Fatal("native barrier", f.s.runtimeError)
			}
			switch surface {
			case "title":
				if f.persisted() != "原生组合字" {
					t.Fatal("title preedit not finalized")
				}
			case "subtask":
				if subtaskPersisted(t, f, child).Title != "原生组合字" {
					t.Fatal("child preedit lost")
				}
			case "composer":
				if len(f.svc.Snapshot().Subtasks) != 1 || f.svc.Snapshot().Subtasks[0].Title != "原生组合字" {
					t.Fatal("composer lost")
				}
			}
			confirmCarryover(t, f)
			f.all()
			if len(f.svc.Snapshot().Traces) != 2 {
				t.Fatal("command missing")
			}
		})
	}
}
func TestCarryoverDayNativeFinishFailureAndFailedComposerBlockOpening(t *testing.T) {
	for _, mode := range []string{"title-native", "composer-save"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			if mode == "title-native" {
				f.edit("不可丢")
				f.s.titleEditor.native.handle = ui.Handle{}
			} else {
				label := subtaskPrefix("day", string(tr.ID)) + ".new"
				subtaskEdit(t, f, label, "不可丢新增")
				titleStoreFault(f.svc, func(at string) error {
					if at == "before-reserve" {
						return errors.New("synthetic fail")
					}
					return nil
				})
				f.tt.Key(0, ui.KeyEnter)
				f.all()
			}
			// Direct delivery before another frame ensures registration sees the failed native handle.
			f.s.openCarryover(carryoverDefer, taskview.DayRow{TraceID: tr.ID, ChainID: tr.ChainID, Status: tr.Status})
			f.all()
			if f.s.carryover != nil || len(f.svc.Snapshot().Traces) != 1 || f.s.runtimeError == "" {
				t.Fatal("failed input admitted")
			}
		})
	}
}
func TestCarryoverDayOtherWindowDirtyHistorySurvives(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	liveClick(t, tt, "日任务详情标题")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("另一窗未保存")
	f.ui()
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	f.all()
	tt.Frame()
	f.ui()
	if other.titleEditor == nil || other.titleEditor.native.draft != "另一窗未保存" || !other.titleEditor.locked || len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("other window history draft discarded")
	}
}
func TestCarryoverDayStaleNativeMenuAfterNavigationAndSession(t *testing.T) {
	for _, mode := range []string{"navigation", "session"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			openReturnMenu(t, f.tt, tr.ID)
			old := independentNativeMenuCallback(t, f.tt)
			if mode == "navigation" {
				f.s.titleTransition(func() { f.s.page = "pool" })
				f.all()
				f.tt.Frame()
				f.s.titleTransition(func() { f.s.page = "day" })
				f.all()
			} else {
				if !f.h.beforeClose(f.s) {
					t.Fatal("close")
				}
				f.h.attach(f.s, func() { f.invalid = true }, func() {})
			}
			f.tt.Frame()
			old(2)
			f.tt.Frame()
			f.all()
			if f.s.carryover != nil || len(f.svc.Snapshot().Traces) != 1 {
				t.Fatal("stale native menu opened")
			}
		})
	}
}

func TestCarryoverDayTwoWindowsSameTargetDeduplicate(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	tt := ui.NewTester(other.view, 1200, 768)
	openCarryoverMenu(t, f, carryoverDefer)
	openReturnMenu(t, tt, tr.ID)
	if err := tt.ChooseMenuItem(deferTaskLabel); err != nil {
		t.Fatal(err)
	}
	f.ui()
	tt.Frame()
	confirmCarryover(t, f)
	tt.Key(0, ui.KeyEnter)
	f.ui()
	if len(f.io.q) != 1 {
		t.Fatal("same target duplicated", len(f.io.q))
	}
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 || f.s.runtimeError != "" || other.runtimeError != "" {
		t.Fatal("dedup result")
	}
}
func TestCarryoverDayLockedHistoryReturnedPoolMenuStillVisible(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	tomorrow, _ := liveTestDay.AddDays(1)
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	if err := f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	target, _ := tomorrow.AddDays(1)
	id, err := f.svc.ContinueUnfinishedTrace(tr.ChainID, tr.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ReturnToPool(tr.ChainID, id); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), tomorrow)
	f.ui()
	f.tt.Frame()
	openCarryoverMenu(t, f, carryoverContinue)
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Traces) != 2 || f.s.runtimeError == "" || f.s.carryover != nil {
		t.Fatal("Day menu must expose source continue, Service must reject pool location")
	}
}
func TestCarryoverDayConfirmBarrierDoesNotBlockBackgroundUI(t *testing.T) {
	f := newTitleFixture(t, true)
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	for i := 0; i < 10; i++ {
		f.tt.Frame()
		f.ui()
	}
	if len(f.io.q) != 1 || len(f.svc.Snapshot().Traces) != 1 || f.s.carryover == nil {
		t.Fatal("UI wrote or lost intent while worker delayed")
	}
	f.io.one()
	if f.s.runtimeDate != liveTestDay || len(f.s.runtime.Snapshot().Traces) != 1 {
		t.Fatal("read ahead of publication")
	}
	f.all()
	if len(f.s.runtime.Snapshot().Traces) != 2 || f.s.runtimeDate == liveTestDay {
		t.Fatal("ack publication order")
	}
}

func TestCarryoverDayRootModalGatesAndStaleIntent(t *testing.T) {
	f := newTitleFixture(t, true)
	opened := 0
	f.s.openUtility = func(string) { opened++ }
	openCarryoverMenu(t, f, carryoverDefer)
	p := f.s.carryover
	date := f.s.runtimeDate
	f.tt.Key(ui.Cmd, ui.KeyN)
	f.tt.Key(ui.Cmd, ui.KeyComma)
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyI)
	f.tt.Key(0, ui.KeyRight)
	f.ui()
	if opened != 0 || f.s.runtimeDate != date {
		t.Fatal("root modal leaked input")
	}
	stale := carryoverPickerIntent{Identity: p.picker.identity, Date: p.picker.selected}
	f.tt.Key(0, ui.KeyEscape)
	f.all()
	openCarryoverMenu(t, f, carryoverDefer)
	fresh := f.s.carryover
	f.s.performCarryover(p, stale)
	f.all()
	if f.s.carryover != fresh || len(f.svc.Snapshot().Traces) != 1 {
		t.Fatal("stale intent acted")
	}
}

func TestCarryoverDayRelatedComposerClearAndUnrelatedDraft(t *testing.T) {
	for _, mode := range []string{"same-window", "other-window", "unrelated"} {
		t.Run(mode, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			input := f.tt
			owner := f.s
			inputTrace := tr
			if mode == "unrelated" {
				if err := f.svc.AddToday("另一个任务"); err != nil {
					t.Fatal(err)
				}
				f.h.publish(f.svc.Snapshot(), liveTestDay)
				inputTrace = f.svc.Snapshot().Traces[1]
			}
			if mode != "same-window" {
				owner = newRuntimeApp(f.h.readView())
				f.h.attach(owner, func() {}, func() {})
				owner.runtimeSelection = string(inputTrace.ID)
				owner.runtimeDayRail = true
				input = ui.NewTester(owner.view, 1200, 768)
				f.ui()
			}
			label := subtaskPrefix("day", string(inputTrace.ID)) + ".new"
			liveClick(t, input, label)
			input.Type("未提交草稿")
			f.ui()
			row := taskview.DayRow{TraceID: tr.ID, ChainID: tr.ChainID, Status: tr.Status}
			f.s.openCarryover(carryoverDefer, row)
			f.all()
			f.tt.Frame()
			if mode == "unrelated" {
				if f.s.carryover == nil {
					t.Fatal("unrelated draft blocked")
				}
				confirmCarryover(t, f)
				f.all()
				for _, c := range owner.subtaskComposers {
					if c.native.draft != "未提交草稿" {
						t.Fatal("unrelated draft lost")
					}
				}
				return
			}
			if f.s.carryover != nil || len(f.svc.Snapshot().Traces) != 1 {
				t.Fatal("related draft admitted")
			}
			for i := 0; i < 3; i++ {
				f.tt.Frame()
				f.all()
			}
			if f.s.carryover != nil {
				t.Fatal("auto reopened")
			}
			liveClick(t, input, label)
			input.Key(ui.Cmd, ui.KeyA)
			input.Key(0, ui.KeyBackspace)
			f.ui()
			f.s.openCarryover(carryoverDefer, row)
			f.all()
			f.tt.Frame()
			if f.s.carryover == nil {
				t.Fatal("explicit clear did not release", f.s.runtimeError)
			}
			confirmCarryover(t, f)
			f.all()
			if len(f.svc.Snapshot().Traces) != 2 {
				t.Fatal("clear then defer")
			}
		})
	}
}
func TestCarryoverDayPendingComposerWaitsForExplicitRetryAfterAcknowledgment(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	subtaskEdit(t, f, label, "提交中的子项")
	f.tt.Key(0, ui.KeyEnter)
	f.ui()
	row := taskview.DayRow{TraceID: tr.ID, ChainID: tr.ChainID, Status: tr.Status}
	f.s.openCarryover(carryoverDefer, row)
	f.ui()
	if f.s.carryover != nil {
		t.Fatal("pending composer admitted")
	}
	f.all()
	f.tt.Frame()
	if f.s.carryover != nil {
		t.Fatal("auto retried")
	}
	f.s.openCarryover(carryoverDefer, row)
	f.all()
	f.tt.Frame()
	if f.s.carryover == nil {
		t.Fatal("ack did not release", f.s.runtimeError)
	}
	confirmCarryover(t, f)
	f.all()
	if len(f.svc.Snapshot().Subtasks) != 2 {
		t.Fatal("committed child not copied")
	}
}

func TestCarryoverDayIntegratedEvidence(t *testing.T) {
	f := newTitleFixture(t, true)
	f.tt.SetScale(2)
	openCarryoverMenu(t, f, carryoverDefer)
	savePickerTestFrame(t, f.tt, "carryover-day-defer-2x.png")
	confirmCarryover(t, f)
	f.all()
	f.tt.Frame()
	savePickerTestFrame(t, f.tt, "carryover-day-deferred-2x.png")
	g := newTitleFixture(t, true)
	tomorrow, _ := liveTestDay.AddDays(1)
	g.clock.now = g.clock.now.Add(24 * time.Hour)
	if err := g.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	g.h.publish(g.svc.Snapshot(), tomorrow)
	g.ui()
	g.tt.Frame()
	g.tt.SetScale(2)
	openCarryoverMenu(t, g, carryoverContinue)
	savePickerTestFrame(t, g.tt, "carryover-day-continue-2x.png")
}
func TestCarryoverDayOtherWindowDirtySubtaskHistorySurvives(t *testing.T) {
	f, tr, child := subtaskFixture(t)
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeSelection = string(tr.ID)
	other.runtimeDayRail = true
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	label := subtaskPrefix("day", string(child.ID)) + ".title"
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("另一窗子项草稿")
	f.ui()
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	f.all()
	tt.Frame()
	f.ui()
	e := other.subtaskEditors[subtaskOwner("day", string(child.ID))]
	if e == nil || e.native.draft != "另一窗子项草稿" || !e.locked {
		t.Fatal("other child draft lost")
	}
}

func TestCarryoverDayLateComposerFinalizationRevokesOldNativeClient(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	old := titleDirectClient(t, tt)
	old.SetMarkedText(nil, "原对象已交付", ui.TextInputRange{Start: 6, End: 6})
	f.all()
	tt.Frame()
	f.ui()
	old.SetMarkedText(nil, "旧租约禁止输入", ui.TextInputRange{Start: 7, End: 7})
	tt.Frame()
	f.ui()
	var composer *runtimeSubtaskComposer
	for _, c := range other.subtaskComposers {
		if c.target.trace == tr.ID {
			composer = c
		}
	}
	snapshot, ok := composer.native.Snapshot()
	if !ok || snapshot.Text != "原对象已交付" || snapshot.Composing {
		t.Fatal("old lease still active", snapshot, ok)
	}
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	f.ui()
	tt.Frame()
	if !f.h.beforeClose(other) {
		t.Fatal("explicit blank did not release recovery")
	}
	if len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("automatic Add")
	}
}
func TestCarryoverDayLateComposerFinishFailureKeepsNativeNode(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "边界失败保留", ui.TextInputRange{Start: 6, End: 6})
	var composer *runtimeSubtaskComposer
	for _, c := range other.subtaskComposers {
		if c.target.trace == tr.ID {
			composer = c
		}
	}
	// Lose the application's bound handle, not the native editor or its buffer.
	composer.native.handle = ui.Handle{}
	f.all()
	tt.Frame()
	f.ui()
	snapshot, ok := composer.native.handle.TextInputSnapshot(composer.native.context)
	if !ok || snapshot.Text+snapshot.MarkedText != "边界失败保留" || !composer.mounted || composer.carryoverRecovery == carryoverRecoveryNone {
		t.Fatal("failed Finish destroyed native state", snapshot, ok)
	}
	if len(f.svc.Snapshot().Traces) != 2 {
		t.Fatal("pretended disk failure")
	}
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	f.ui()
	tt.Frame()
	if !f.h.beforeClose(other) {
		t.Fatal("fresh native blank could not release unknown proof")
	}
}

func TestCarryoverDayLateComposerNavigationRecoveryPortal(t *testing.T) {
	for _, destination := range []string{"pool", "other-chain"} {
		t.Run(destination, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			if err := f.svc.AddToday("独立任务"); err != nil {
				t.Fatal(err)
			}
			f.h.publish(f.svc.Snapshot(), liveTestDay)
			f.ui()
			second := f.svc.Snapshot().Traces[1]
			other := newRuntimeApp(f.h.readView())
			f.h.attach(other, func() {}, func() {})
			other.runtimeSelection = string(tr.ID)
			other.runtimeDayRail = true
			tt := ui.NewTester(other.view, 1200, 768)
			f.ui()
			tt.Frame()
			openCarryoverMenu(t, f, carryoverDefer)
			confirmCarryover(t, f)
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			titleDirectClient(t, tt).SetMarkedText(nil, "离页前已交付", ui.TextInputRange{Start: 6, End: 6})
			other.titleTransition(func() {
				if destination == "pool" {
					other.page = "pool"
					other.runtimeSelection = ""
				} else {
					other.runtimeSelection = string(second.ID)
				}
			})
			f.ui()
			tt.Frame()
			f.all()
			tt.Frame()
			f.ui()
			if _, ok := tt.Find("查看原任务子任务草稿"); !ok {
				t.Fatal("no discoverable recovery portal", other.runtimeError)
			}
			liveClick(t, tt, "查看原任务子任务草稿")
			f.all()
			tt.Frame()
			f.ui()
			if other.page != "day" || other.runtimeDate != tr.Date || other.runtimeSelection != string(tr.ID) {
				t.Fatal("portal did not route original identity")
			}
			var original *runtimeSubtaskComposer
			for _, c := range other.subtaskComposers {
				if c.target.trace == tr.ID {
					original = c
				}
			}
			snapshot, ok := original.native.Snapshot()
			if !ok || snapshot.Text != "离页前已交付" || !original.mounted {
				t.Fatal("portal lost original native draft", snapshot, ok)
			}
			tt.SetScale(2)
			savePickerTestFrame(t, tt, "carryover-recovery-"+destination+"-2x.png")
			liveClick(t, tt, label)
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Key(0, ui.KeyBackspace)
			f.ui()
			tt.Frame()
			if !f.h.beforeClose(other) {
				t.Fatal("portal recovery cannot close after explicit clear")
			}
		})
	}
}

func TestCarryoverDayMultipleRecoverySourcesAndFreshRouting(t *testing.T) {
	f := newTitleFixture(t, true)
	if err := f.svc.AddToday("第二来源"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	sources := append([]core.DayTrace(nil), f.svc.Snapshot().Traces...)
	other := newRuntimeApp(f.h.readView())
	f.h.attach(other, func() {}, func() {})
	other.runtimeDayRail = true
	other.runtimeSelection = string(sources[0].ID)
	tt := ui.NewTester(other.view, 1200, 768)
	f.ui()
	tt.Frame()
	for i, tr := range sources {
		if i > 0 {
			f.tt.Key(0, ui.KeyEscape)
			f.ui()
			other.titleTransition(func() { other.runtimeSelection = string(tr.ID) })
			f.ui()
			tt.Frame()
		}
		openReturnMenu(t, f.tt, tr.ID)
		if err := f.tt.ChooseMenuItem(deferTaskLabel); err != nil {
			t.Fatal(err)
		}
		f.ui()
		f.tt.Frame()
		confirmCarryover(t, f)
		label := subtaskPrefix("day", string(tr.ID)) + ".new"
		liveClick(t, tt, label)
		tt.Type("原对象草稿")
		f.ui()
	}
	other.titleTransition(func() { other.page = "pool"; other.runtimeSelection = "" })
	f.ui()
	tt.Frame()
	f.all()
	tt.Frame()
	count := 0
	for _, c := range other.subtaskComposers {
		if c.carryoverRecovery != carryoverRecoveryNone {
			count++
		}
	}
	if count != 2 {
		t.Fatal("lost recovery source", count)
	}
	// A later legal mutation changes the continuation's pool location, not the immutable source route.
	for _, tr := range f.svc.Snapshot().Traces {
		if tr.Status == core.TracePending {
			if err := f.svc.ReturnToPool(tr.ChainID, tr.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.ui()
	tt.Frame()
	other.runtimeError = "另一项操作需要检查"
	other.runtimeNotice = "新的提示"
	tt.Frame()
	if other.runtimeError != "另一项操作需要检查" || other.runtimeNotice != "新的提示" {
		t.Fatal("recovery stole newer notice")
	}
	visited := map[string]bool{}
	for i := 0; i < 2; i++ {
		liveClick(t, tt, "查看原任务子任务草稿")
		f.all()
		tt.Frame()
		f.ui()
		id := other.runtimeSelection
		if visited[id] {
			t.Fatal("portal repeated resolved source")
		}
		visited[id] = true
		label := subtaskPrefix("day", id) + ".new"
		liveClick(t, tt, label)
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		f.ui()
		other.titleTransition(func() { other.page = "pool"; other.runtimeSelection = "" })
		f.ui()
		tt.Frame()
	}
	if other.deferredComposerRecovery() != nil || !f.h.beforeClose(other) {
		t.Fatal("multi-source cleanup blocked")
	}
}

func TestCarryoverDayStalePresentationDoesNotGateNewPageInput(t *testing.T) {
	f := newTitleFixture(t, true)
	openCarryoverMenu(t, f, carryoverDefer)
	confirmCarryover(t, f)
	f.s.titleTransition(func() { f.s.page = "pool"; f.s.runtimeSelection = "" })
	f.ui()
	f.tt.Frame()
	if f.s.livePickerOpen() {
		t.Fatal("stale sheet gated new page")
	}
	f.s.draft = "新页面任务"
	f.s.submitLiveDraft(true)
	f.all()
	if f.s.page != "pool" || len(f.svc.Snapshot().Chains) != 2 {
		t.Fatal("new page input blocked")
	}
}

func TestCarryoverDayUnrelatedPublicationDoesNotRevokeRecoveryEditor(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	tt.Type("恢复草稿")
	f.ui()
	f.all()
	tt.Frame()
	liveClick(t, tt, label)
	native := titleDirectClient(t, tt)
	native.SetMarkedText(nil, "继续编辑", ui.TextInputRange{Start: 4, End: 4})
	if err := f.svc.AddPool("无关资料"); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	native.SetMarkedText(nil, "同租约后续输入", ui.TextInputRange{Start: 7, End: 7})
	var original *runtimeSubtaskComposer
	for _, c := range other.subtaskComposers {
		if c.target.trace == tr.ID {
			original = c
		}
	}
	snapshot, ok := original.native.handle.TextInputSnapshot(original.native.context)
	if !ok || snapshot.MarkedText != "同租约后续输入" {
		t.Fatal("unrelated publication revoked existing recovery lease", snapshot, ok)
	}
}

func TestCarryoverDaySiblingCloseDuringWriterPreservesDeliveredInput(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "关闭前已交付", ui.TextInputRange{Start: 6, End: 6})
	if f.h.beforeClose(other) {
		t.Fatal("sibling closed before in-flight source publication with delivered input")
	}
	if f.h.windows[other.titleWindowID] == nil {
		t.Fatal("detached sibling")
	}
	f.all()
	tt.Frame()
	f.ui()
	var original *runtimeSubtaskComposer
	for _, c := range other.subtaskComposers {
		if c.target.trace == tr.ID {
			original = c
		}
	}
	snapshot, ok := original.native.Snapshot()
	if !ok || snapshot.Text != "关闭前已交付" || original.carryoverRecovery == carryoverRecoveryNone {
		t.Fatal("close lost input")
	}
	liveClick(t, tt, label)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	f.ui()
	tt.Frame()
	if !f.h.beforeClose(other) {
		t.Fatal("explicit blank cannot close")
	}
}

func TestCarryoverDayGuardedOwnerSurvivesAnyReadonlyPublication(t *testing.T) {
	for _, mode := range []string{"complete", "return", "midnight"} {
		t.Run(mode, func(t *testing.T) {
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
			if mode != "midnight" {
				f.h.commandOnce(other, "prior-source-change", func() error {
					if mode == "complete" {
						return f.svc.SetCompleted(tr.ID, true)
					}
					return f.svc.ReturnToPool(tr.ChainID, tr.ID)
				}, nil)
				f.ui()
			}
			confirmCarryover(t, f)
			if mode == "midnight" {
				f.clock.now = f.clock.now.Add(24 * time.Hour)
			}
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, tt, label)
			titleDirectClient(t, tt).SetMarkedText(nil, "退场前已交付", ui.TextInputRange{Start: 6, End: 6})
			var original *runtimeSubtaskComposer
			for _, c := range other.subtaskComposers {
				if c.target.trace == tr.ID {
					original = c
				}
			}
			f.io.one()
			if len(f.posts.q) == 0 {
				t.Fatal("missing publication")
			}
			f.posts.one()
			// Render between publication and any completion; callback repair is too late.
			tt.Frame()
			snapshot, ok := original.native.handle.TextInputSnapshot(original.native.context)
			if !ok || snapshot.Text+snapshot.MarkedText != "退场前已交付" || !original.mounted {
				t.Fatal("readonly publication destroyed guarded owner", snapshot, ok)
			}
			f.all()
			tt.Frame()
			f.ui()
		})
	}
}
func TestCarryoverDayGuardedNavigationExposesRecoveryBeforeCompletion(t *testing.T) {
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
	label := subtaskPrefix("day", string(tr.ID)) + ".new"
	liveClick(t, tt, label)
	titleDirectClient(t, tt).SetMarkedText(nil, "离开前已交付", ui.TextInputRange{Start: 6, End: 6})
	other.titleTransition(func() { other.page = "pool"; other.runtimeSelection = "" })
	f.ui()
	tt.Frame()
	if _, ok := tt.Find("查看原任务子任务草稿"); !ok {
		t.Fatal("guarded offscreen input has no recovery path before completion")
	}
	if f.h.beforeClose(other) {
		t.Fatal("offscreen guarded draft closed")
	}
}
