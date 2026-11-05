// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"strings"
	"testing"
)

// Reconstruct the precise facts left by source continue + snapshot undo.
// This is a Go compatibility probe, not an executed Swift oracle.
func reviewUndoContinuationAlias(t *testing.T) (canonical.Value, core.TaskGraph, subtaskEditCommand) {
	t.Helper()
	prior, before, after := historyFixture(t)
	history, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	_, extra, err := v7.ToDomain(history)
	must(t, err)
	g := after.Clone()
	source := g.Traces[1]
	if source.Status != core.TraceUnfinished {
		t.Fatal("fixture source", source.Status)
	}
	next := source
	next.ID = core.DayTraceID("00000000-0000-4000-8000-000000000081")
	next.Date, _ = source.Date.AddDays(1)
	next.CreatedAt = titleAt(20)
	next.ContentUpdatedAt = titleAt(30)
	next.Status = core.TraceCancelledDraft
	next.ContinuationSeq = source.ContinuationSeq + 1
	next.CarriedFromTraceID = &source.ID
	now := titleAt(30)
	next.SettledAt = &now
	cid := core.CancellationID("00000000-0000-4000-8000-000000000082")
	next.DraftCancellationID = &cid
	next.DraftCancelledOn = &next.Date
	g.Traces = append(g.Traces, next)
	_, err = core.NewTaskKernel(g)
	must(t, err)
	prior, err = v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, err)

	c := subtaskEditCommand{planned: &core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: source.ChainID, ExpectedDefinitionID: source.DefinitionID, SubtaskID: core.PlannedSubtaskID("00000000-0000-4000-8000-000000000083"), LineageID: core.SubtaskLineageID("00000000-0000-4000-8000-000000000084"), Title: "合法回池后新计划", Difficulty: core.DifficultySimple, Now: titleAt(40)}}
	return prior, g, c
}
func TestReviewUndoContinuationAliasCoreEdit(t *testing.T) {
	_, g, c := reviewUndoContinuationAlias(t)
	k, err := core.NewTaskKernel(g)
	must(t, err)
	must(t, c.apply(k))
	t.Log("合法 core graph：source-equivalent planned edit 获准")
}
func TestReviewUndoContinuationAliasServiceScope(t *testing.T) {
	prior, g, c := reviewUndoContinuationAlias(t)
	k, err := core.NewTaskKernel(g)
	must(t, err)
	must(t, c.apply(k))
	_, err = captureSubtaskEditHistory(prior, g, k.Snapshot(), c)
	if err == nil || !strings.Contains(fmt.Sprint(err), "unsupported cancellation fact") {
		t.Fatal("expected existing scope rejection", err)
	}
	t.Log("既有 history 子集先拒绝；尚未到新增 historical-definition fence：", err)
}
