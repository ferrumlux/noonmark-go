// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"path/filepath"
	"testing"
)

func independentCarry(t *testing.T, s *Service, continuation bool) (core.DayTraceID, error) {
	t.Helper()
	tr := s.Snapshot().Traces[0]
	target, _ := s.Today().AddDays(1)
	if continuation {
		return s.ContinueUnfinishedTrace(tr.ChainID, tr.ID, target)
	}
	return s.DeferCurrentTrace(tr.ChainID, tr.ID, target)
}
func TestIndependentCarryoverFourFaults(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
			t.Run(map[bool]string{false: "defer", true: "continue"}[continuation]+"/"+phase, func(t *testing.T) {
				s, p, _ := auditReviewFixture(t, continuation)
				before := futureAggregate(t, s)
				today := s.Today()
				revision := s.disk.revision
				count := len(s.Snapshot().Traces)
				s.disk.fault = func(at string) error {
					if at == phase {
						return ErrIncomplete
					}
					return nil
				}
				id, e := independentCarry(t, s, continuation)
				if id != "" || !errors.Is(e, ErrIncomplete) {
					t.Fatalf("false acknowledgement %q %v", id, e)
				}
				if string(before) != string(futureAggregate(t, s)) || s.Today() != today || s.disk.revision != revision {
					t.Fatal("failed operation published")
				}
				s.disk.fault = nil
				if phase != "before-reserve" {
					id, e = independentCarry(t, s, continuation)
					if id != "" || !errors.Is(e, ErrIncomplete) {
						t.Fatalf("blocked handle reused %q %v", id, e)
					}
				}
				must(t, s.Close())
				r, e := Open(p, today)
				if phase == "after-reserve" || phase == "before-commit" {
					if e == nil {
						r.Close()
						t.Fatal("uncommitted reservation opened")
					}
					return
				}
				must(t, e)
				defer r.Close()
				if phase == "after-persist" {
					if len(r.Snapshot().Traces) != count+1 {
						t.Fatal("lost persisted candidate")
					}
					must(t, ValidateEmptyHistory(r.snapshot))
					id, e = independentCarry(t, r, continuation)
					if id != "" || e == nil {
						t.Fatal("replayed original command")
					}
				} else if !historyEqual(s.snapshot, r.snapshot) {
					t.Fatal("before-reserve changed disk")
				}
			})
		}
	}
}
func TestIndependentCarryoverCompetingHandles(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[continuation], func(t *testing.T) {
			s, p, _ := auditReviewFixture(t, continuation)
			r, e := Open(p, s.Today())
			must(t, e)
			defer r.Close()
			a := futureAggregate(t, s)
			b := futureAggregate(t, r)
			type result struct {
				id core.DayTraceID
				e  error
				s  *Service
			}
			ch := make(chan result, 2)
			start := make(chan struct{})
			for _, h := range []*Service{s, r} {
				go func(h *Service) { <-start; id, e := independentCarry(t, h, continuation); ch <- result{id, e, h} }(h)
			}
			close(start)
			wins := 0
			for i := 0; i < 2; i++ {
				x := <-ch
				if x.e == nil {
					wins++
					if x.id == "" {
						t.Fatal("empty winner")
					}
				} else {
					if x.id != "" || !errors.Is(x.e, ErrConflict) {
						t.Fatalf("loser %q %v", x.id, x.e)
					}
					want := a
					if x.s == r {
						want = b
					}
					if string(want) != string(futureAggregate(t, x.s)) {
						t.Fatal("loser published")
					}
				}
			}
			if wins != 1 {
				t.Fatalf("winners %d", wins)
			}
		})
	}
}
func TestIndependentCarryoverCorruptPrefix(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		t.Run(map[bool]string{false: "defer", true: "continue"}[continuation], func(t *testing.T) {
			s, p, _ := auditReviewFixture(t, continuation)
			before := futureAggregate(t, s)
			must(t, os.WriteFile(filepath.Join(p, "rev-0000000001", "COMMITTED"), []byte("corrupt fixture"), 0600))
			id, e := independentCarry(t, s, continuation)
			if id != "" || e == nil {
				t.Fatal("corrupt prefix acknowledged")
			}
			if string(before) != string(futureAggregate(t, s)) {
				t.Fatal("corrupt prefix published")
			}
			must(t, s.Close())
			r, e := Open(p, s.Today())
			if e == nil {
				r.Close()
				t.Fatal("corrupt prefix reopened")
			}
		})
	}
}
