// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func TestIndependentCarryoverActualTodayAndLockedTarget(t *testing.T) {
	t.Run("continue today", func(t *testing.T) {
		s, _, source := auditReviewFixture(t, true)
		tr := s.Snapshot().Traces[0]
		rev := s.disk.revision
		id, e := s.ContinueUnfinishedTrace(tr.ChainID, source, s.Today())
		must(t, e)
		if id == "" || s.disk.revision != rev+1 {
			t.Fatal("today continuation unconfirmed")
		}
		found := false
		for _, tr := range s.Snapshot().Traces {
			if tr.ID == id {
				found = true
				if tr.Date != s.Today() {
					t.Fatal("wrong date")
				}
			}
		}
		if !found {
			t.Fatal("missing target")
		}
	})
	t.Run("defer today", func(t *testing.T) {
		s, _, source := auditReviewFixture(t, false)
		tr := s.Snapshot().Traces[0]
		before := futureAggregate(t, s)
		rev := s.disk.revision
		id, e := s.DeferCurrentTrace(tr.ChainID, source, s.Today())
		if e == nil || id != "" {
			t.Fatal("defer to today accepted")
		}
		if string(before) != string(futureAggregate(t, s)) || rev != s.disk.revision {
			t.Fatal("reject published")
		}
	})
	t.Run("locked target", func(t *testing.T) {
		s, _, source := auditReviewFixture(t, false)
		tr := s.Snapshot().Traces[0]
		target, _ := s.Today().AddDays(1)
		must(t, s.AddOnDate("锁定目标合成项", target))
		later, _ := target.AddDays(1)
		must(t, s.AdvanceTo(later))
		before := futureAggregate(t, s)
		rev := s.disk.revision
		id, e := s.ContinueUnfinishedTrace(tr.ChainID, source, target)
		if !errors.Is(e, core.ErrImmutableHistory) || id != "" {
			t.Fatalf("locked target: %q %v", id, e)
		}
		if string(before) != string(futureAggregate(t, s)) || rev != s.disk.revision {
			t.Fatal("reject published")
		}
	})
}
