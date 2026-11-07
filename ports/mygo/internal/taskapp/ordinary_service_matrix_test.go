// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

// Every current V2 operation is exercised on the same ordinary Service, with
// both empty/rich catalog and absent/present prior history. All data is made by
// real commands; no imported fake aggregate or replacement validator is used.
func TestOrdinaryServiceAllCommandsMatrix(t *testing.T) {
	for _, rich := range []bool{false, true} {
		for _, history := range []bool{false, true} {
			t.Run(fmt.Sprintf("category_%t_history_%t", rich, history), func(t *testing.T) {
				s, path := freshOrdinary(t)
				if history {
					must(t, s.AddToday("已有历史"))
					g := s.Snapshot()
					must(t, s.SetCompleted(g.Traces[0].ID, true))
					tomorrow, _ := s.Today().AddDays(1)
					must(t, s.AdvanceTo(tomorrow))
				}
				must(t, s.AddPool("主任务"))
				g := s.Snapshot()
				chain := g.Chains[len(g.Chains)-1].ID
				def := g.Definitions[len(g.Definitions)-1].ID
				if rich {
					o := s.ReplaceCategorySelection(chain, newCategoryRequest(), ordinary.UUID(ordinaryTestID(60)))
					must(t, o.Err)
				}
				checkpoint := func() {
					t.Helper()
					v, e := s.ReadView()
					must(t, e)
					r, e := OpenOrdinaryDevelopment(path, s.Today())
					must(t, e)
					rv, e := r.ReadView()
					must(t, e)
					must(t, r.Close())
					if !reflect.DeepEqual(v.Graph, rv.Graph) || !reflect.DeepEqual(v.Classification, rv.Classification) {
						t.Fatal("reopen mismatch")
					}
				}
				p, e := s.AddPlannedSubtask(chain, def, "计划子项", core.DifficultyMedium)
				must(t, e)
				_, e = s.SavePlannedSubtaskTitleAcknowledged(chain, def, p, "计划新标题")
				must(t, e)
				must(t, s.SetPlannedSubtaskDifficulty(chain, def, p, core.DifficultyHard))
				remove, e := s.AddPlannedSubtask(chain, def, "移除", core.DifficultySimple)
				must(t, e)
				must(t, s.RemovePlannedSubtask(chain, def, remove))
				checkpoint()
				must(t, s.RenameTaskTitle(chain, "改任务标题"))
				must(t, s.SaveTaskTitleInput(chain, "直接输入标题"))
				must(t, s.Schedule(chain, s.Today()))
				g = s.Snapshot()
				trace := g.Traces[len(g.Traces)-1].ID
				_, e = s.SaveTraceTitleInputAcknowledged(trace, "当天标题")
				must(t, e)
				a, e := s.AddSubtask(chain, trace, "实际子项", core.DifficultyMedium)
				must(t, e)
				_, e = s.SaveSubtaskTitleAcknowledged(chain, trace, a, "实际新标题")
				must(t, e)
				must(t, s.SetSubtaskDifficulty(chain, trace, a, core.DifficultyHard))
				must(t, s.SetSubtaskCompleted(chain, trace, a, true))
				must(t, s.SetSubtaskCompleted(chain, trace, a, false))
				deleted, e := s.AddSubtask(chain, trace, "删除", core.DifficultySimple)
				must(t, e)
				must(t, s.DeleteSubtask(chain, trace, deleted))
				abandoned, e := s.AddSubtask(chain, trace, "放弃子项", core.DifficultySimple)
				must(t, e)
				must(t, s.AbandonSubtask(chain, trace, abandoned))
				must(t, s.AddToday("独立进度任务"))
				mg := s.Snapshot()
				mt := mg.Traces[len(mg.Traces)-1]
				if rich {
					v, e := s.ReadView()
					must(t, e)
					for id := range v.Classification.Categories {
						o := s.ReplaceCategorySelection(mt.ChainID, ordinary.Choice{Kind: ordinary.ChoiceExisting, CategoryID: id}, ordinary.UUID(ordinaryTestID(61)))
						must(t, o.Err)
						break
					}
				}
				must(t, s.SetManualProgress(mt.ChainID, mt.ID, 25))
				for _, sub := range s.Snapshot().Subtasks {
					if sub.TraceID == trace && sub.Status == core.SubtaskPending {
						must(t, s.SetSubtaskCompleted(chain, trace, sub.ID, true))
					}
				}
				must(t, s.SetCompleted(trace, true))
				must(t, s.SetCompleted(trace, false))
				must(t, s.SetSubtaskCompleted(chain, trace, a, false))
				checkpoint()
				tomorrow, _ := s.Today().AddDays(1)
				future, e := s.DeferCurrentTrace(chain, trace, tomorrow)
				must(t, e)
				checkpoint()
				must(t, s.ReturnToPool(chain, future))
				checkpoint() // future cancelledDraft, no capture
				must(t, s.Schedule(chain, s.Today()))
				g = s.Snapshot()
				trace = g.Traces[len(g.Traces)-1].ID
				must(t, s.ReturnToPool(chain, trace))
				checkpoint() // current-day capture
				must(t, s.Schedule(chain, s.Today()))
				g = s.Snapshot()
				trace = g.Traces[len(g.Traces)-1].ID
				must(t, s.AdvanceTo(tomorrow))
				checkpoint()
				_, e = s.ContinueUnfinishedTrace(chain, trace, tomorrow)
				must(t, e)
				checkpoint()
				must(t, s.AddToday("快捷当天"))
				next, _ := tomorrow.AddDays(1)
				must(t, s.AddOnDate("明确日期", next))
				checkpoint()
				if rich {
					v, e := s.ReadView()
					must(t, e)
					if len(v.Classification.Categories) != 1 || v.Classification.CurrentByChainID[chain].Category == nil {
						t.Fatal("classification lost across task matrix")
					}
					for _, events := range v.Classification.SnapshotEventsByTraceID {
						for _, event := range events {
							for _, tr := range v.Graph.Traces {
								if tr.ID == event.TraceID && tr.ChainID == chain && event.Category == nil {
									t.Fatal("capture lost category")
								}
							}
						}
					}
				}
			})
		}
	}
}
