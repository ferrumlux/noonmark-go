// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"reflect"
	"testing"
)

func TestReviewerReturnCancellationMustBeFreshBeforeNormalization(t *testing.T) {
	g := baseGraph()
	in := returnInput()
	in.Today = date("2026-11-04")
	s := child()
	s.Status = SubtaskCancelledDraft
	s.SettledAt = ptr(instant(100))
	s.DraftCancellationID = &in.CancellationID
	g.Subtasks = []Subtask{s}
	k, e := NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	before := k.Snapshot()
	if e = k.ReturnToPool(in); e == nil {
		t.Fatal("reused existing child cancellation ID after clearing it")
	}
	if !reflect.DeepEqual(before, k.Snapshot()) {
		t.Fatal("rejection mutated")
	}
}
func TestReviewerReturnPlannedIdentityMustBeFresh(t *testing.T) {
	g := baseGraph()
	in := returnInput()
	s := child()
	g.Subtasks = []Subtask{s}
	id := PlannedSubtaskID(identity(23))
	g.Definitions[0].PlannedSubtasks = []PlannedSubtask{{ID: id, LineageID: s.LineageID, Title: s.Title, Difficulty: s.Difficulty, Position: 1, CreatedAt: instant(100)}}
	in.PlannedIDs = map[SubtaskID]PlannedSubtaskID{s.ID: id}
	k, e := NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	if e = k.ReturnToPool(in); e == nil {
		t.Fatal("reused existing planned subtask identity in new definition")
	}
}
