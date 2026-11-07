// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"path/filepath"
	"reflect"
	"testing"
)

func freshOrdinary(t *testing.T) (*Service, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session")
	s, e := CreateOrdinaryDevelopment(p, testDay)
	must(t, e)
	t.Cleanup(func() { s.Close() })
	return s, p
}
func newCategoryRequest() ordinary.Choice {
	return ordinary.Choice{Kind: ordinary.ChoiceNew, Name: " 工作 / 中文 ", ColorHex: "#D87831"}
}
func TestOrdinaryServiceOriginalPlanAcrossTaskEditsAndRetry(t *testing.T) {
	s, path := freshOrdinary(t)
	must(t, s.AddPool("A"))
	chain := s.Snapshot().Chains[0].ID
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(41)))
	must(t, e)
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	before := s.disk.revision
	must(t, s.AddPool("B"))
	must(t, s.SaveTaskTitleInput(chain, "A 新标题"))
	out := s.CommitCategorySelection(p, c)
	must(t, out.Err)
	if out.WriteExtent != WriteCommittedConfirmed || out.Receipt == nil || s.disk.revision != before+3 {
		t.Fatal(out)
	}
	first := *out.Receipt
	out = s.ReplaceCategorySelection(chain, ordinary.Choice{Kind: ordinary.ChoiceNone}, ordinary.UUID(ordinaryTestID(42)))
	must(t, out.Err)
	revision := s.disk.revision
	out = s.CommitCategorySelection(p, c)
	must(t, out.Err)
	if out.WriteExtent != WriteNone || s.disk.revision != revision || !reflect.DeepEqual(first, *out.Receipt) {
		t.Fatal("retry wrote or replaced original receipt")
	}
	if out.View.Classification.CurrentByChainID[chain].Category != nil {
		t.Fatal("retry view shows old category")
	}
	view, e := s.ReadView()
	must(t, e)
	must(t, s.Close())
	reopened, e := OpenOrdinaryDevelopment(path, testDay)
	must(t, e)
	defer reopened.Close()
	got, e := reopened.ReadView()
	must(t, e)
	if !reflect.DeepEqual(view.Classification, got.Classification) || !reflect.DeepEqual(view.Graph, got.Graph) {
		t.Fatal("full reopen mismatch")
	}
	out = reopened.CommitCategorySelection(p, c)
	if !errors.Is(out.Err, ordinary.ErrContext) || out.WriteExtent != WriteNone {
		t.Fatal("old owner accepted", out)
	}
}
func TestOrdinaryServiceTrueStaleAndHistoricalCapture(t *testing.T) {
	s, path := freshOrdinary(t)
	must(t, s.AddToday("A"))
	g := s.Snapshot()
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(43)))
	must(t, e)
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	out := s.ReplaceCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(44)))
	must(t, out.Err)
	if o := s.CommitCategorySelection(p, c); !errors.Is(o.Err, ordinary.ErrStale) {
		t.Fatal(o)
	}
	must(t, s.SetCompleted(trace, true))
	must(t, s.SetCompleted(trace, false))
	must(t, s.SetCompleted(trace, true))
	tomorrow, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(tomorrow))
	view, e := s.ReadView()
	must(t, e)
	event := view.Classification.SnapshotsByTraceID[trace]
	if event.Category == nil {
		t.Fatal("missing historical value")
	}
	out = s.ReplaceCategorySelection(chain, ordinary.Choice{Kind: ordinary.ChoiceNone}, ordinary.UUID(ordinaryTestID(45)))
	must(t, out.Err)
	if !reflect.DeepEqual(event, out.View.Classification.SnapshotsByTraceID[trace]) {
		t.Fatal("setCurrent rewrote old event")
	}
	must(t, s.Close())
	r, e := OpenOrdinaryDevelopment(path, tomorrow)
	must(t, e)
	defer r.Close()
}
func TestOrdinaryServiceRejectedAndUncertainDoNotSave(t *testing.T) {
	s, _ := freshOrdinary(t)
	must(t, s.AddPool("A"))
	chain := s.Snapshot().Chains[0].ID
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(46)))
	must(t, e)
	before := s.disk.revision
	bad := s.CommitCategorySelection(p, CategoryConfirmation{})
	if !errors.Is(bad.Err, ordinary.ErrConfirmation) || s.disk.revision != before {
		t.Fatal(bad)
	}
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	s.disk.fault = func(stage string) error {
		if stage == "after-reserve" {
			return errors.New("synthetic fault")
		}
		return nil
	}
	out := s.CommitCategorySelection(p, c)
	if out.Result != ResultUncertain || out.Receipt != nil || out.Session != SessionNeedsReopen || out.WriteExtent == WriteCommittedConfirmed {
		t.Fatal(out)
	}
	if e = s.AddPool("blocked"); e == nil {
		t.Fatal("continued after uncertain")
	}
}

func TestOrdinaryServiceViewsAndPlanProjectionAreOwned(t *testing.T) {
	s, _ := freshOrdinary(t)
	must(t, s.AddPool("A"))
	chain := s.Snapshot().Chains[0].ID
	before := s.disk.revision
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(70)))
	must(t, e)
	data, e := p.Inspect()
	must(t, e)
	data.Changes[0].After.Category.Name = "污染"
	data.Intent.Choice.Name = "污染"
	if s.disk.revision != before {
		t.Fatal("draft allocated facts")
	}
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	out := s.CommitCategorySelection(p, c)
	must(t, out.Err)
	v, e := s.ReadView()
	must(t, e)
	v.Graph.Definitions[0].Title = "污染"
	for id := range v.Classification.Categories {
		delete(v.Classification.Categories, id)
	}
	got, e := s.ReadView()
	must(t, e)
	if got.Graph.Definitions[0].Title != "A" || len(got.Classification.Categories) != 1 {
		t.Fatal("view aliases service")
	}
	id := got.Classification.CurrentByChainID[chain].Category.CategoryID
	out = s.ReplaceCategorySelection(chain, ordinary.Choice{Kind: ordinary.ChoiceExisting, CategoryID: id}, ordinary.UUID(ordinaryTestID(71)))
	must(t, out.Err)
	if out.View.Classification.Revision != 2 {
		t.Fatal("new interaction same selection must record")
	}
}
func TestOrdinaryServiceWriterFaultMatrix(t *testing.T) {
	for _, stage := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(stage, func(t *testing.T) {
			s, path := freshOrdinary(t)
			must(t, s.AddPool("A"))
			chain := s.Snapshot().Chains[0].ID
			before, e := s.ReadView()
			must(t, e)
			s.disk.fault = func(at string) error {
				if at == stage {
					return errors.New("synthetic transaction fault")
				}
				return nil
			}
			out := s.ReplaceCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(72)))
			if out.Err == nil || out.Receipt != nil || out.WriteExtent == WriteCommittedConfirmed {
				t.Fatal("false saved", out)
			}
			after, e := s.ReadView()
			must(t, e)
			if !reflect.DeepEqual(before.Classification, after.Classification) || !reflect.DeepEqual(before.Graph, after.Graph) {
				t.Fatal("published failed candidate")
			}
			if stage == "before-reserve" {
				if out.WriteExtent != WriteNone || out.Session != SessionWritable {
					t.Fatal(out)
				}
				s.disk.fault = nil
				must(t, s.AddPool("still writable"))
				return
			}
			if out.Session != SessionNeedsReopen || out.Result != ResultUncertain {
				t.Fatal(out)
			}
			must(t, s.Close())
			r, e := OpenOrdinaryDevelopment(path, testDay)
			if stage == "after-persist" {
				must(t, e)
				defer r.Close()
				v, e := r.ReadView()
				must(t, e)
				if len(v.Classification.Categories) != 1 {
					t.Fatal("confirmed disk record not restored")
				}
			} else if e == nil {
				r.Close()
				t.Fatal("incomplete reserved tail silently repaired")
			}
		})
	}
}

func TestOrdinaryServiceNaturalDayPreparationPrecedesStale(t *testing.T) {
	s, path := freshOrdinary(t)
	must(t, s.AddToday("A"))
	g := s.Snapshot()
	chain, trace := g.Chains[0].ID, g.Traces[0].ID
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(73)))
	must(t, e)
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	tomorrow, _ := testDay.AddDays(1)
	before := s.disk.revision
	out := s.CommitCategorySelectionOn(tomorrow, p, c)
	if !errors.Is(out.Err, ordinary.ErrStale) || out.WriteExtent != WriteNone || out.Preparation.WriteExtent != WriteCommittedConfirmed || s.disk.revision != before+1 || out.Receipt != nil {
		t.Fatal(out)
	}
	if out.View.Classification.SnapshotsByTraceID[trace].Status != core.TraceUnfinished || out.View.Classification.Revision != 1 {
		t.Fatal("settlement rolled back")
	}
	out = s.ReplaceCategorySelectionOn(tomorrow, chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(74)))
	must(t, out.Err)
	if out.View.Classification.Revision != 2 || out.View.Classification.SnapshotsByTraceID[trace].Category != nil {
		t.Fatal("post-setCurrent capture or old value rewrite")
	}
	before = s.disk.revision
	bad := s.ReplaceCategorySelectionOn(core.LocalDate{}, chain, ordinary.Choice{Kind: ordinary.ChoiceNone}, ordinary.UUID(ordinaryTestID(75)))
	if !errors.Is(bad.Err, core.ErrInvalidInput) || bad.Preparation.FailureStage != StageParameter || s.disk.revision != before {
		t.Fatal(bad)
	}
	// Match existing AdvanceTo semantics: a backwards calendar sample does not
	// unlock a day or rewrite historical facts/Instant frontiers.
	oldEvent := out.View.Classification.SnapshotsByTraceID[trace]
	out = s.ReplaceCategorySelectionOn(testDay, chain, ordinary.Choice{Kind: ordinary.ChoiceNone}, ordinary.UUID(ordinaryTestID(76)))
	must(t, out.Err)
	if !reflect.DeepEqual(oldEvent, out.View.Classification.SnapshotsByTraceID[trace]) {
		t.Fatal("calendar rollback rewrote history")
	}
	must(t, s.Close())
	r, e := OpenOrdinaryDevelopment(path, tomorrow)
	must(t, e)
	defer r.Close()
}

func TestOrdinaryServiceRetainedEdgeCannotAuthorizeAnotherCandidate(t *testing.T) {
	s, _ := freshOrdinary(t)
	must(t, s.AddPool("A"))
	g := s.Snapshot()
	s.mu.Lock()
	defer s.mu.Unlock()
	now, e := s.nextInstant()
	must(t, e)
	in := ordinaryTransition{kind: ordinaryTitle, title: titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: g.Chains[0].ID, Title: "合法候选", Today: s.today, Now: now}}
	candidate, e := s.taskCandidateLocked(in)
	must(t, e)
	before := s.disk.revision
	candidate.task.input.title.Title = "另一条请求"
	if e = s.persistOrdinaryLocked(candidate); e == nil || s.disk.revision != before {
		t.Fatal("edge substitution authorized write")
	}
	candidate.task = nil
	if e = s.persistOrdinaryLocked(candidate); e == nil || s.disk.revision != before {
		t.Fatal("bare validated bytes authorized write")
	}
}
func TestOrdinaryServiceExactRetryStillPreparesNaturalDay(t *testing.T) {
	s, _ := freshOrdinary(t)
	must(t, s.AddToday("A"))
	chain := s.Snapshot().Chains[0].ID
	p, e := s.PrepareCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(77)))
	must(t, e)
	c, e := ConfirmCategoryPlan(p)
	must(t, e)
	first := s.CommitCategorySelection(p, c)
	must(t, first.Err)
	tomorrow, _ := testDay.AddDays(1)
	before := s.disk.revision
	out := s.CommitCategorySelectionOn(tomorrow, p, c)
	must(t, out.Err)
	if out.Preparation.WriteExtent != WriteCommittedConfirmed || out.WriteExtent != WriteNone || s.disk.revision != before+1 || !reflect.DeepEqual(first.Receipt, out.Receipt) || out.View.Classification.Revision != 2 {
		t.Fatal("retry skipped preparation or duplicated classification", out)
	}
}
