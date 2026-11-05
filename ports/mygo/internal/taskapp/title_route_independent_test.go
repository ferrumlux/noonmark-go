// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"testing"
)

func TestReviewTraceTitleSameDayCompletedWithEditableChain(t *testing.T) {
	_, g, _ := titleHistoryFixture(t)
	next, _ := testDay.AddDays(1)
	g.Days[0].Date = testDay
	g.Days[0].LockedAt = nil
	g.Traces[0].Date = testDay
	g.Days[1].Date = next
	g.Traces[1].Date = next
	extra, e := emptySupplement()
	must(t, e)
	p, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	p, g, e = normalizeSnapshot(p)
	must(t, e)
	s, _ := fresh(t)
	must(t, s.disk.commit(p))
	s.graph, s.snapshot = g, p
	before, rev := s.Snapshot(), s.disk.revision
	if e = s.SaveTraceTitleInput(g.Traces[0].ID, "new"); !errors.Is(e, core.ErrInvalidTransition) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev {
		t.Fatal("rejection published")
	}
	must(t, s.SaveTraceTitleInput(g.Traces[1].ID, "new"))
	if s.graph.Definitions[0].Title != g.Definitions[0].Title || len(s.graph.Definitions) != 2 {
		t.Fatal("history mutated")
	}
}
