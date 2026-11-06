// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func manualServiceFixture(t *testing.T) (*Service, string, core.TaskChainID, core.DayTraceID) {
	t.Helper()
	s, path := fresh(t)
	must(t, s.AddToday("合成手动进度"))
	g := s.Snapshot()
	return s, path, g.Chains[0].ID, g.Traces[0].ID
}
func serviceManualTrace(t *testing.T, s *Service, trace core.DayTraceID) core.DayTrace {
	t.Helper()
	for _, tr := range s.Snapshot().Traces {
		if tr.ID == trace {
			return tr
		}
	}
	t.Fatal("missing trace")
	return core.DayTrace{}
}

func TestManualProgressCreateSetReopenExactFacts(t *testing.T) {
	s, path, chain, trace := manualServiceFixture(t)
	before := s.Snapshot()
	rev := s.disk.revision
	accepted, err := s.SetManualProgressAcknowledged(chain, trace, math.MinInt)
	must(t, err)
	if accepted != 0 || s.disk.revision != rev+1 || serviceManualTrace(t, s, trace).ManualProgressPercent == nil {
		t.Fatal("nil to zero not persisted")
	}
	for _, percent := range []int{58, 12, math.MaxInt} {
		accepted, err = s.SetManualProgressAcknowledged(chain, trace, percent)
		must(t, err)
		if accepted != min(100, max(0, percent)) || *serviceManualTrace(t, s, trace).ManualProgressPercent != accepted {
			t.Fatal("requested rather than accepted progress")
		}
	}
	after := s.Snapshot()
	if after.Traces[0].Status != core.TracePending || after.Traces[0].CompletedAt != nil {
		t.Fatal("100 auto-completed")
	}
	if !reflect.DeepEqual(before.Days, after.Days) || !reflect.DeepEqual(before.Chains, after.Chains) || !reflect.DeepEqual(before.Definitions, after.Definitions) || !reflect.DeepEqual(before.Subtasks, after.Subtasks) {
		t.Fatal("manual command touched another aggregate")
	}
	must(t, s.Close())
	r, err := Open(path, testDay)
	must(t, err)
	defer r.Close()
	if !reflect.DeepEqual(after, r.Snapshot()) {
		t.Fatal("reopen changed exact graph")
	}
	copy := r.Snapshot()
	*copy.Traces[0].ManualProgressPercent = 2
	if *serviceManualTrace(t, r, trace).ManualProgressPercent != 100 {
		t.Fatal("snapshot pointer aliases service")
	}
}

func TestManualProgressNoopSkipsClockAndRevisionButVerifiesPrefix(t *testing.T) {
	for _, kind := range []string{"unchanged", "stale", "corrupt", "blocked", "closed"} {
		t.Run(kind, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			must(t, s.SetManualProgress(chain, trace, 100))
			before, wire, rev := s.Snapshot(), s.snapshot, s.disk.revision
			s.clock = func() time.Time { t.Fatal("no-op sampled clock"); return time.Time{} }
			var want error
			switch kind {
			case "stale":
				other, err := Open(path, testDay)
				must(t, err)
				must(t, other.AddPool("合成并发"))
				must(t, other.Close())
				want = ErrConflict
			case "corrupt":
				must(t, os.WriteFile(filepath.Join(path, fmt.Sprintf("rev-%010d", rev), "COMMITTED"), []byte("broken\n"), 0600))
				want = ErrCorrupt
			case "blocked":
				s.disk.blocked = true
				want = ErrIncomplete
			case "closed":
				must(t, s.Close())
				want = ErrSession
			}
			accepted, err := s.SetManualProgressAcknowledged(chain, trace, math.MaxInt)
			if want == nil {
				must(t, err)
				if accepted != 100 {
					t.Fatal("wrong no-op acknowledgment")
				}
			} else if !errors.Is(err, want) || accepted != 0 {
				t.Fatal("unconfirmed no-op acknowledgment", accepted, err, want)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || rev != s.disk.revision {
				t.Fatal("no-op changed memory or revision")
			}
		})
	}
}

func TestManualProgressDurableFaultMatrix(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			must(t, s.SetManualProgress(chain, trace, 20))
			before, wire, rev := s.Snapshot(), s.snapshot, s.disk.revision
			s.disk.fault = func(at string) error {
				if at == phase {
					return ErrIncomplete
				}
				return nil
			}
			accepted, err := s.SetManualProgressAcknowledged(chain, trace, 61)
			if !errors.Is(err, ErrIncomplete) || accepted != 0 {
				t.Fatal("fault acknowledged", accepted, err)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || s.disk.revision != rev {
				t.Fatal("uncertain write published")
			}
			s.disk.fault = nil
			if phase != "before-reserve" {
				if accepted, err = s.SetManualProgressAcknowledged(chain, trace, 20); !errors.Is(err, ErrIncomplete) || accepted != 0 {
					t.Fatal("blocked handle allowed no-op", accepted, err)
				}
			}
			must(t, s.Close())
			r, err := Open(path, testDay)
			if phase == "after-reserve" || phase == "before-commit" {
				if r != nil {
					r.Close()
				}
				if !errors.Is(err, ErrIncomplete) {
					t.Fatal("partial write reopened", err)
				}
				return
			}
			must(t, err)
			defer r.Close()
			want := 20
			if phase == "after-persist" {
				want = 61
				rev++
			}
			if r.disk.revision != rev || *serviceManualTrace(t, r, trace).ManualProgressPercent != want {
				t.Fatal("persisted state not exact")
			}
			must(t, r.SetManualProgress(chain, trace, 62))
		})
	}
}

func TestManualProgressConcurrentAcknowledgmentsAndConflicts(t *testing.T) {
	t.Run("same-handle", func(t *testing.T) {
		s, path, chain, trace := manualServiceFixture(t)
		before := s.Snapshot()
		rev := s.disk.revision
		s.clock = func() time.Time { return time.Unix(978307201, 0) }
		var wg sync.WaitGroup
		for _, percent := range []int{11, 22, 33, 44, 55, 66} {
			wg.Add(1)
			go func(percent int) {
				defer wg.Done()
				accepted, err := s.SetManualProgressAcknowledged(chain, trace, percent)
				if err != nil || accepted != percent {
					t.Errorf("crossed acknowledgment: want %d got %d err %v", percent, accepted, err)
				}
			}(percent)
		}
		wg.Wait()
		after := s.Snapshot()
		if s.disk.revision != rev+6 || !after.Traces[0].ContentUpdatedAt.After(before.Traces[0].ContentUpdatedAt) {
			t.Fatal("lost write or backwards clock")
		}
		must(t, s.Close())
		r, err := Open(path, testDay)
		must(t, err)
		defer r.Close()
		if !reflect.DeepEqual(after, r.Snapshot()) {
			t.Fatal("reopen lost concurrent result")
		}
	})
	t.Run("separate-handles", func(t *testing.T) {
		s, path, chain, trace := manualServiceFixture(t)
		other, err := Open(path, testDay)
		must(t, err)
		defer other.Close()
		before := s.Snapshot()
		type result struct {
			accepted int
			err      error
			service  *Service
		}
		out := make(chan result, 2)
		var wg sync.WaitGroup
		for i, svc := range []*Service{s, other} {
			wg.Add(1)
			go func(svc *Service, percent int) {
				defer wg.Done()
				a, e := svc.SetManualProgressAcknowledged(chain, trace, percent)
				out <- result{a, e, svc}
			}(svc, 30+i)
		}
		wg.Wait()
		close(out)
		ok, conflicts := 0, 0
		for result := range out {
			if result.err == nil {
				ok++
				if result.accepted < 30 || result.accepted > 31 {
					t.Fatal("wrong winner acknowledgment")
				}
			} else if errors.Is(result.err, ErrConflict) {
				conflicts++
				if result.accepted != 0 || !reflect.DeepEqual(before, result.service.Snapshot()) {
					t.Fatal("loser published")
				}
			} else {
				t.Fatal(result.err)
			}
		}
		if ok != 1 || conflicts != 1 {
			t.Fatal("not exactly one winner", ok, conflicts)
		}
	})
}

func TestManualProgressServiceRoutingCompletionReturnAndDayBoundary(t *testing.T) {
	s, path, chain, trace := manualServiceFixture(t)
	must(t, s.SetManualProgress(chain, trace, 35))
	before, rev := s.Snapshot(), s.disk.revision
	for _, route := range []struct {
		chain core.TaskChainID
		trace core.DayTraceID
		want  error
	}{{"00000000-0000-4000-8000-000000001111", trace, core.ErrInvalidInput}, {chain, "00000000-0000-4000-8000-000000001112", core.ErrNotFound}} {
		if err := s.SetManualProgress(route.chain, route.trace, 50); !errors.Is(err, route.want) {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
		t.Fatal("bad route published")
	}
	must(t, s.SetCompleted(trace, true))
	if err := s.SetManualProgress(chain, trace, 35); !errors.Is(err, core.ErrImmutableHistory) {
		t.Fatal("completed no-op bypassed guards", err)
	}
	must(t, s.SetCompleted(trace, false))
	must(t, s.SetManualProgress(chain, trace, 65))
	must(t, s.ReturnToPool(chain, trace))
	if err := s.SetManualProgress(chain, trace, 50); !errors.Is(err, core.ErrImmutableHistory) {
		t.Fatal(err)
	}
	tomorrow, _ := testDay.AddDays(1)
	must(t, s.Schedule(chain, tomorrow))
	var next core.DayTraceID
	for _, tr := range s.Snapshot().Traces {
		if tr.Status == core.TracePending {
			next = tr.ID
		}
	}
	if err := s.SetManualProgress(chain, next, 70); !errors.Is(err, core.ErrImmutableHistory) {
		t.Fatal("future accepted", err)
	}
	must(t, s.AdvanceTo(tomorrow))
	audit := s.snapshot.Object["classifications"]
	accepted, err := s.SetManualProgressAcknowledged(chain, next, -100)
	must(t, err)
	if accepted != 65 || !historyEqual(audit, s.snapshot.Object["classifications"]) {
		t.Fatal("floor or immutable audit changed")
	}
	if err = s.SetManualProgress(chain, trace, 65); !errors.Is(err, core.ErrImmutableHistory) {
		t.Fatal("locked past accepted", err)
	}
	saved := s.Snapshot()
	must(t, s.Close())
	r, err := Open(path, tomorrow)
	must(t, err)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("audited history lost on reopen")
	}
}

func TestManualProgressServiceActualHistoryAndCancellation(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprint(deleted), func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			child, err := s.AddSubtask(chain, trace, "合成子项", core.DifficultyHard)
			must(t, err)
			if deleted {
				must(t, s.DeleteSubtask(chain, trace, child))
				must(t, s.SetManualProgress(chain, trace, 40))
			} else {
				must(t, s.SetSubtaskCompleted(chain, trace, child, true))
				must(t, s.ReturnToPool(chain, trace))
				must(t, s.Schedule(chain, testDay))
				for _, tr := range s.Snapshot().Traces {
					if tr.Status == core.TracePending {
						trace = tr.ID
					}
				}
				before, rev := s.Snapshot(), s.disk.revision
				if err := s.SetManualProgress(chain, trace, 40); !errors.Is(err, core.ErrInvalidTransition) {
					t.Fatal("historical child ignored", err)
				}
				if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
					t.Fatal("blocked child edit published")
				}
			}
			saved := s.Snapshot()
			must(t, s.Close())
			r, err := Open(path, testDay)
			must(t, err)
			defer r.Close()
			if !reflect.DeepEqual(saved, r.Snapshot()) {
				t.Fatal("child history lost")
			}
		})
	}
}
