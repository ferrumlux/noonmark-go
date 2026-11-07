// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

// This fixture always uses the real durable ordinary Service, never a writer mock.
func newCategoryFixture(t *testing.T, pool bool) *titleFixture {
	f := &titleFixture{t: t}
	f.clock.now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f.path = filepath.Join(t.TempDir(), "ordinary-session")
	var err error
	f.svc, err = taskapp.CreateOrdinaryDevelopment(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.svc.Close() })
	if pool {
		err = f.svc.AddPool("任务 A")
	} else {
		err = f.svc.AddToday("任务 A")
	}
	if err != nil {
		t.Fatal(err)
	}
	f.h = newTitleHub(f.svc, &f.io, f.posts.Enqueue, &f.clock, f.clock.Now, func() {
		if f.h.beforeQuit() {
			f.quit++
		}
	})
	if err = f.h.enableCategoryCandidate(); err != nil {
		t.Fatal(err)
	}
	f.s = newRuntimeApp(f.h.readView())
	f.h.attach(f.s, func() { f.invalid = true }, func() {
		if f.h.beforeClose(f.s) {
			f.closed++
		}
	})
	if pool {
		f.s.page = "pool"
		f.s.runtimeSelection = string(f.svc.Snapshot().Chains[0].ID)
	} else {
		f.s.runtimeSelection = string(f.svc.Snapshot().Traces[0].ID)
		f.s.runtimeDayRail = true
	}
	f.tt = ui.NewTester(f.s.view, 1200, 768)
	f.ui()
	return f
}
func categoryBegin(t *testing.T, f *titleFixture, name string) *runtimeCategoryEditor {
	t.Helper()
	chain := f.svc.Snapshot().Chains[0].ID
	liveClick(t, f.tt, "任务分组 "+string(chain))
	if err := f.tt.ChooseMenuItem("新建分组…"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.ui()
	liveClick(t, f.tt, "新分组名称")
	f.tt.Type(name)
	f.ui()
	return f.h.windows[f.s.titleWindowID].category
}
func TestCategoryCandidateCancelAndDurableSelection(t *testing.T) {
	f := newCategoryFixture(t, true)
	before, _ := f.svc.ReadView()
	categoryBegin(t, f, "取消不会落盘")
	liveClick(t, f.tt, "取消新建分组")
	f.all()
	after, _ := f.svc.ReadView()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cancel wrote facts")
	}
	e := categoryBegin(t, f, "工程 / 中文")
	liveClick(t, f.tt, "创建并加入")
	f.all()
	view, err := f.svc.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if e.failure != nil || e.pending || e.creating || e.savedUntil.IsZero() || len(view.Classification.Categories) != 1 {
		t.Fatalf("create state: %+v view=%+v", e, view.Classification)
	}
	if err = f.svc.AddPool("任务 B"); err != nil {
		t.Fatal(err)
	}
	f.h.publishFromLane()
	f.all()
	var other core.TaskChainID
	for _, c := range f.svc.Snapshot().Chains {
		if c.ID != e.chain {
			other = c.ID
		}
	}
	f.s.runtimeSelection = string(other)
	f.tt.Frame()
	f.ui()
	e2 := f.h.windows[f.s.titleWindowID].category
	selected := categoryCurrent(view, e.chain)
	f.s.submitCategory(e2, ordinary.Choice{Kind: ordinary.ChoiceExisting, CategoryID: selected})
	f.all()
	f.s.submitCategory(e2, ordinary.Choice{Kind: ordinary.ChoiceNone})
	f.all()
	final, _ := f.svc.ReadView()
	if categoryCurrent(final, other) != "" {
		t.Fatal("remove failed")
	}
	if err = f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.OpenOrdinaryDevelopment(f.path, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, _ := reopened.ReadView()
	if !reflect.DeepEqual(final.Classification, got.Classification) {
		t.Fatal("reopen mismatch")
	}
}
func TestCategoryCandidateCompositionAndCloseBarrier(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "")
	titleDirectClient(t, f.tt).SetMarkedText(nil, "中文组合", ui.TextInputRange{Start: 4, End: 4})
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.all()
	view, _ := f.svc.ReadView()
	if len(view.Classification.Categories) != 0 || e.pending {
		t.Fatal("composition submitted")
	}
	titleDirectClient(t, f.tt).ReplaceText(nil, "工程")
	f.tt.Frame()
	f.ui()
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	if !e.pending || f.h.beforeClose(f.s) {
		t.Fatal("inflight close bypass")
	}
	f.all()
	if f.closed != 1 {
		t.Fatalf("confirmed close not reentered: %d failure=%v", f.closed, e.failure)
	}
}
func TestCategoryCandidateFailureRetainsDraft(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "失败保留")
	if err := f.svc.Close(); err != nil {
		t.Fatal(err)
	}
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.all()
	if e.failure == nil || e.native.draft != "失败保留" || !e.creating || !e.savedUntil.IsZero() || f.h.beforeClose(f.s) {
		t.Fatalf("failure discarded draft: %+v", e)
	}
}
func TestCategoryCandidateHistoryChangesCurrentOnly(t *testing.T) {
	f := newCategoryFixture(t, false)
	e := categoryBegin(t, f, "历史分类")
	liveClick(t, f.tt, "创建并加入")
	f.all()
	trace := f.svc.Snapshot().Traces[0].ID
	if err := f.svc.SetCompleted(trace, true); err != nil {
		t.Fatal(err)
	}
	tomorrow, _ := liveTestDay.AddDays(1)
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	if err := f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	before, _ := f.svc.ReadView()
	f.h.publishFromLane()
	f.all()
	// Selected old detail date is not the actual natural day passed to Service.
	f.s.runtimeDate = liveTestDay
	f.s.runtimeSelection = string(trace)
	f.tt.Frame()
	f.ui()
	e = f.s.categoryEditor(e.chain)
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNone})
	f.all()
	after, _ := f.svc.ReadView()
	if e.failure != nil || categoryCurrent(after, e.chain) != "" || !reflect.DeepEqual(before.Classification.SnapshotEventsByTraceID, after.Classification.SnapshotEventsByTraceID) {
		t.Fatal("history current/capture mismatch", e.failure)
	}
}

func TestCategoryCandidateLateReceiptAndNewerNativeText(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "第一份草稿")
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	// Delivered platform input after admission must survive the earlier receipt.
	titleDirectClient(t, f.tt).ReplaceText(nil, "较新的草稿")
	f.all()
	if e.failure != nil || !e.creating || e.native.draft == "" {
		t.Fatal("late native input cleared", e.failure, e.native.draft)
	}
	if f.h.beforeClose(f.s) {
		t.Fatal("newer draft did not protect close")
	}
	// A detached card must not receive another card's saved indicator.
	e.failure = nil
	e.creating = false
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNone})
	replacement := &runtimeCategoryEditor{chain: e.chain, generation: e.generation + 1}
	f.h.windows[f.s.titleWindowID].category = replacement
	f.all()
	if !replacement.savedUntil.IsZero() || replacement.failure != nil {
		t.Fatal("late receipt leaked to replacement")
	}
}
func TestCategoryCandidateCatalogOrderAndDefaultDisabled(t *testing.T) {
	view := taskapp.OrdinaryReadView{Classification: ordinary.AggregateState{Categories: map[ordinary.CategoryID]ordinary.Category{
		"B": {ID: "B", CanonicalKey: "alpha", Lifecycle: "active"}, "A": {ID: "A", CanonicalKey: "alpha", Lifecycle: "active"}, "Z": {ID: "Z", CanonicalKey: "zeta", Lifecycle: "active"}, "X": {ID: "X", CanonicalKey: "0", Lifecycle: "archived"},
	}}}
	got := categoryCatalog(view)
	if len(got) != 3 || got[0].ID != "A" || got[1].ID != "B" || got[2].ID != "Z" {
		t.Fatal(got)
	}
	f := newTitleFixture(t, true)
	if f.h.category != nil || f.s.categoryEditor(f.svc.Snapshot().Chains[0].ID) != nil {
		t.Fatal("default entry enabled")
	}
}

func TestCategoryCandidateHeadlessEvidence(t *testing.T) {
	root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if root == "" {
		t.Skip("external evidence root not configured")
	}
	f := newCategoryFixture(t, true)
	save := func(name string) {
		t.Helper()
		out, err := os.Create(filepath.Join(root, "evidence", name))
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		if err = png.Encode(out, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	categoryBegin(t, f, "工程与中文长名称视觉检查")
	save("category-synthetic-real-service-draft-headless.png")
	liveClick(t, f.tt, "创建并加入")
	f.all()
	save("category-synthetic-real-service-confirmed-headless.png")
	if err := os.WriteFile(filepath.Join(root, "evidence", "CATEGORY-SCREENSHOT-SCOPE.txt"), []byte("Synthetic ordinary development Service, MyGo headless renderer. Draft and confirmed real Service states; not OS native-window screenshots or macOS/Windows acceptance. Production entry remains disabled. System menu chrome is not rasterized.\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCategoryCandidateCancelCompositionWritesNothing(t *testing.T) {
	f := newCategoryFixture(t, true)
	before, _ := f.svc.ReadView()
	e := categoryBegin(t, f, "")
	titleDirectClient(t, f.tt).SetMarkedText(nil, "取消组合", ui.TextInputRange{Start: 4, End: 4})
	liveClick(t, f.tt, "取消新建分组")
	f.all()
	after, _ := f.svc.ReadView()
	if e.creating || e.native.draft != "" || !reflect.DeepEqual(before, after) {
		t.Fatal("composition cancel did not clear locally without writing")
	}
}
func TestCategoryCandidateTwoWindowsShareLatestView(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "窗口一")
	liveClick(t, f.tt, "创建并加入")
	f.all()
	second := newRuntimeApp(f.h.readView())
	f.h.attach(second, func() {}, func() {})
	second.page = "pool"
	second.runtimeSelection = string(e.chain)
	tt := ui.NewTester(second.view, 1200, 768)
	tt.Frame()
	e2 := f.h.windows[second.titleWindowID].category
	if e2 == nil || e2 == e {
		t.Fatal("window owner not isolated")
	}
	second.submitCategory(e2, ordinary.Choice{Kind: ordinary.ChoiceNone})
	f.all()
	if categoryCurrent(f.h.category.view, e.chain) != "" || e2.savedUntil.IsZero() {
		t.Fatal("second window missing current receipt")
	}
	if e.native.draft != "" || e.failure != nil {
		t.Fatal("second window contaminated first draft")
	}
}

func TestCategoryCandidateOldNativeMenuAfterNavigation(t *testing.T) {
	f := newCategoryFixture(t, true)
	chain := f.svc.Snapshot().Chains[0].ID
	liveClick(t, f.tt, "任务分组 "+string(chain))
	old := independentNativeMenuCallback(t, f.tt)
	f.s.titleTransition(func() { f.s.runtimeSelection = "" })
	f.all()
	f.s.titleTransition(func() { f.s.runtimeSelection = string(chain) })
	f.all()
	old(2)
	f.tt.Frame()
	f.all()
	e := f.h.windows[f.s.titleWindowID].category
	if e.creating || e.pending {
		t.Fatal("stale native menu crossed navigation")
	}
}
func TestCategoryCandidateMidnightRetainsUnsavedHistoricalDraft(t *testing.T) {
	f := newCategoryFixture(t, false)
	e := categoryBegin(t, f, "跨日草稿")
	tomorrow, _ := liveTestDay.AddDays(1)
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	if err := f.svc.AdvanceTo(tomorrow); err != nil {
		t.Fatal(err)
	}
	f.h.publishFromLane()
	f.all()
	if f.s.runtimeDate != liveTestDay || f.s.runtimeSelection == "" || e.native.draft != "跨日草稿" {
		t.Fatal("midnight hid category draft")
	}
	liveClick(t, f.tt, "创建并加入")
	f.all()
	if e.failure != nil {
		t.Fatal(e.failure)
	}
}

func TestCategoryCandidateHitTargetsAndScale(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			f := newCategoryFixture(t, true)
			f.tt.SetScale(scale)
			e := categoryBegin(t, f, "中文 English 长名称测试")
			input, ok := f.tt.Find("新分组名称")
			if !ok {
				t.Fatal("missing native input")
			}
			cancel, _ := f.tt.Find("取消新建分组")
			create, _ := f.tt.Find("创建并加入")
			menu, _ := f.tt.Find("任务分组 " + string(e.chain))
			if input.H != 28 || cancel.H != 28 || create.H != 28 || menu.H != 28 || cancel.W < 28 || create.W < 28 || menu.W < 28 {
				t.Fatal("undersized hit target", input, cancel, create, menu)
			}
			if input.X+input.W > cancel.X-7.9 || cancel.X+cancel.W > create.X-7.9 {
				t.Fatal("overlapping draft controls", input, cancel, create)
			}
			if scale == 2 {
				if root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT"); root != "" {
					file, err := os.Create(filepath.Join(root, "evidence", "category-synthetic-real-service-draft-headless-2x.png"))
					if err != nil {
						t.Fatal(err)
					}
					if err = png.Encode(file, f.tt.Image()); err != nil {
						t.Fatal(err)
					}
					file.Close()
				}
			}
			liveClick(t, f.tt, "创建并加入")
			f.all()
			chip, _ := f.tt.Find("任务分组 " + string(e.chain))
			if chip.W > 92 || chip.H != 28 {
				t.Fatal("source chip bounds", chip)
			}
		})
	}
}
