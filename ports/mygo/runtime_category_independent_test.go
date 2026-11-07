// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"testing"
	"time"
)

func TestIndependentCategoryQueuedNavigationRetainsLateNativeDraft(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "")
	navigated := false
	f.s.titleTransition(func() { navigated = true; f.s.runtimeSelection = "" })
	// Platform text arrives after navigation admission but before its posted boundary.
	titleDirectClient(t, f.tt).ReplaceText(nil, "迟到但尚未保存")
	if snap, ok := e.native.Snapshot(); !ok || snap.Text == "" {
		t.Fatal("counterexample requires a live nonempty native draft", snap, ok)
	}
	f.all()
	if navigated {
		t.Fatalf("queued navigation escaped with unsaved category draft: %+v", e)
	}
}

func TestIndependentCategoryLateDraftHasNoSavedIndicator(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "已提交旧草稿")
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	titleDirectClient(t, f.tt).ReplaceText(nil, "尚未保存新草稿")
	f.all()
	if !e.savedUntil.IsZero() {
		t.Fatalf("saved indicator remains for newer uncommitted native draft: %q", e.native.draft)
	}
}

// This seam keeps a real Service and its durable writes; only returned evidence is faulted.
type independentCategoryStore struct {
	*taskapp.Service
	mode string
	day  core.LocalDate
}

func (s *independentCategoryStore) ReplaceCategorySelectionOn(d core.LocalDate, c core.TaskChainID, choice ordinary.Choice, id ordinary.UUID) taskapp.CategoryOutcome {
	s.day = d
	out := s.Service.ReplaceCategorySelectionOn(d, c, choice, id)
	if s.mode == "no-receipt" {
		out.Receipt = nil
	}
	if s.mode == "uncertain" {
		out.Result = taskapp.ResultUncertain
	}
	return out
}
func (s *independentCategoryStore) ReadView() (taskapp.OrdinaryReadView, error) {
	if s.mode == "read-failure" {
		return taskapp.OrdinaryReadView{}, errors.New("independent read failure")
	}
	return s.Service.ReadView()
}
func TestIndependentCategoryReceiptAndReadFailure(t *testing.T) {
	for _, mode := range []string{"no-receipt", "uncertain", "read-failure"} {
		t.Run(mode, func(t *testing.T) {
			f := newCategoryFixture(t, true)
			e := categoryBegin(t, f, "保留草稿")
			seam := &independentCategoryStore{Service: f.svc, mode: mode}
			f.h.category.store = seam
			f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
			f.all()
			if e.failure == nil || !e.savedUntil.IsZero() || !e.creating || e.native.draft == "" || f.h.beforeClose(f.s) {
				t.Fatalf("failure accepted: %+v", e)
			}
		})
	}
}
func TestIndependentCategoryExecutionDayNotAdmissionDay(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "跨午夜执行")
	seam := &independentCategoryStore{Service: f.svc}
	f.h.category.store = seam
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.clock.now = f.clock.now.Add(24 * time.Hour)
	f.all()
	if seam.day != localTitleDate(f.clock.now) || e.failure != nil {
		t.Fatal("execution day incorrect", seam.day, e.failure)
	}
}

func TestIndependentCategoryCloseQuitLateInput(t *testing.T) {
	for _, action := range []string{"close", "quit"} {
		for _, composing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-composing-%v", action, composing), func(t *testing.T) {
				f := newCategoryFixture(t, true)
				e := categoryBegin(t, f, "旧草稿")
				f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
				if action == "close" {
					if f.h.beforeClose(f.s) {
						t.Fatal("pending close allowed")
					}
				} else {
					if f.h.beforeQuit() {
						t.Fatal("pending quit allowed")
					}
				}
				if composing {
					titleDirectClient(t, f.tt).SetMarkedText(nil, "迟到组合", ui.TextInputRange{Start: 4, End: 4})
				} else {
					titleDirectClient(t, f.tt).ReplaceText(nil, "迟到输入")
				}
				f.all()
				if f.closed != 0 || f.quit != 0 || f.h.windows[f.s.titleWindowID] == nil || !e.creating {
					t.Fatalf("late draft lost on %s: closed=%d quit=%d editor=%+v", action, f.closed, f.quit, e)
				}
			})
		}
	}
}
func TestIndependentCategoryNewDraftClearsPriorSavedIndicator(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "第一次保存")
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	f.all()
	if e.savedUntil.IsZero() {
		t.Fatal("fixture did not save")
	}
	e = categoryBegin(t, f, "第二份未保存")
	if !e.savedUntil.IsZero() {
		t.Fatal("prior saved indicator survives newer draft")
	}
}
