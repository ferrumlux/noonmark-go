// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"testing"
)

func TestCategoryBoundaryNavigationAfterTitleCommitRechecksNative(t *testing.T) {
	f := newCategoryFixture(t, false)
	f.edit("标题已保存后仍不准隐藏分类草稿")
	e := categoryBegin(t, f, "")
	moved := false
	f.s.titleTransition(func() { moved = true; f.s.runtimeSelection = "" })
	f.ui() // Admission and posted boundary are past; title writer is still pending.
	if len(f.io.q) == 0 {
		t.Fatal("requires an outstanding title write")
	}
	titleDirectClient(t, f.tt).ReplaceText(nil, "最终导航前晚到分类")
	f.all()
	if moved || f.s.runtimeSelection == "" || e.native.draft == "" {
		t.Fatal("final navigation hid late category draft")
	}
	// Already-disposed title registrations must not be reused after the veto.
	liveClick(t, f.tt, "取消新建分组")
	f.all()
	f.edit("阻止导航后标题仍可保存")
	f.s.titleTransition(func() { moved = true })
	f.all()
	if !moved || f.persisted() != "阻止导航后标题仍可保存" {
		t.Fatal("veto broke title registration lifecycle", f.persisted())
	}
}
func TestCategoryBoundaryNewDraftRevokesSaved(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "第一组")
	liveClick(t, f.tt, "创建并加入")
	f.all()
	if e.savedUntil.IsZero() {
		t.Fatal("requires first confirmed indicator")
	}
	categoryBegin(t, f, "新草稿")
	if !e.savedUntil.IsZero() {
		t.Fatal("new draft retained earlier saved badge")
	}
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNone})
	f.all()
	if !e.savedUntil.IsZero() || !e.creating || e.native.draft == "" {
		t.Fatal("existing choice acknowledged unrelated draft")
	}
}
func TestCategoryBoundaryReturnReceiptPreservesNewDraft(t *testing.T) {
	f := newCategoryFixture(t, false)
	trace := f.svc.Snapshot().Traces[0]
	f.s.returnTaskToPool(trace.ChainID, trace.ID)
	f.ui()
	if len(f.io.q) == 0 {
		t.Fatal("requires pending return")
	}
	e := categoryBegin(t, f, "回池执行中新草稿")
	f.all()
	if f.s.runtimeSelection == "" || e.native.draft == "" {
		t.Fatal("automatic return cleared draft owner")
	}
	if _, ok := f.tt.Find("新分组名称"); !ok {
		t.Fatal("returned trace hides retained category draft")
	}
	liveClick(t, f.tt, "创建并加入")
	f.all()
	if e.failure != nil || e.creating {
		t.Fatal("returned chain current draft no longer usable", e.failure)
	}
}

func TestCategoryBoundaryCompositionAtReceiptHasNoSavedBadge(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "原提交")
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	titleDirectClient(t, f.tt).SetMarkedText(nil, "新组合草稿", ui.TextInputRange{Start: 5, End: 5})
	f.all()
	snap, ok := e.native.Snapshot()
	if !ok || !snap.Composing || !e.creating || !e.savedUntil.IsZero() {
		t.Fatal("old receipt acknowledged active composition", snap, e.savedUntil)
	}
}

func TestCategoryBoundaryConfirmedDraftRevokesOldLease(t *testing.T) {
	for _, composition := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "composition"}[composition], func(t *testing.T) {
			f := newCategoryFixture(t, true)
			e := categoryBegin(t, f, "原提交")
			old := titleDirectClient(t, f.tt)
			f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
			f.ui()
			if len(f.io.q) != 1 {
				t.Fatal("requires one queued category writer")
			}
			f.io.one()
			if len(f.posts.q) != 1 {
				t.Fatal("requires one queued receipt")
			}
			f.posts.one() // Receipt processed, old text field not yet unmounted by a frame.
			if e.creating || e.savedUntil.IsZero() {
				t.Fatal("requires confirmed retirement")
			}
			if composition {
				old.SetMarkedText(nil, "旧租约晚到", ui.TextInputRange{Start: 5, End: 5})
			} else {
				old.ReplaceText(nil, "旧租约晚到")
			}
			snap, ok := e.native.Snapshot()
			if !ok || snap.Text != "" || snap.Composing {
				t.Fatal("retired old lease changed input before unmount", snap, ok)
			}
			f.all()
		})
	}
}

func TestCategoryBoundaryPendingFrameAndFreshLease(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "原提交")
	old := titleDirectClient(t, f.tt)
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.ui()
	f.tt.Frame() // A pending redraw must not disable the native terminal boundary.
	f.all()
	if e.creating || e.savedUntil.IsZero() {
		t.Fatal("pending frame prevented acknowledged retirement")
	}
	categoryBegin(t, f, "新租约草稿")
	old.ReplaceText(nil, "旧租约污染")
	snap, ok := e.native.Snapshot()
	if !ok || snap.Text != "新租约草稿" || !e.creating || !e.savedUntil.IsZero() {
		t.Fatal("old lease contaminated fresh draft", snap, ok)
	}
}
