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

func reviewManualTarget(t *testing.T, s *Service, id core.DayTraceID) core.DayTrace {
	t.Helper()
	for _, tr := range s.Snapshot().Traces {
		if tr.ID == id {
			return tr
		}
	}
	t.Fatal("manual target missing")
	return core.DayTrace{}
}

// This test alters only the isolated store to reproduce external corruption.
// Every valid revision preceding the injected fault came from Service commands.
func TestManualProgressReviewNoopVerifiesEntireDiskPrefix(t *testing.T) {
	for _, fault := range []string{"old-payload", "old-seal", "identity", "incomplete-successor"} {
		t.Run(fault, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("独立合成任务"))
			tr := s.Snapshot().Traces[0]
			must(t, s.SetManualProgress(tr.ChainID, tr.ID, 40))
			before, revision := s.Snapshot(), s.disk.revision
			if revision < 3 {
				t.Fatal("fixture does not have an older prefix")
			}
			s.clock = func() time.Time { t.Fatal("no-op sampled wall clock"); return time.Time{} }
			want := ErrCorrupt
			switch fault {
			case "old-payload":
				p := filepath.Join(path, "rev-0000000001", "state.json")
				b, err := os.ReadFile(p)
				must(t, err)
				must(t, os.WriteFile(p, append(b, ' '), 0600))
			case "old-seal":
				must(t, os.WriteFile(filepath.Join(path, "rev-0000000001", "COMMITTED"), []byte("not-a-seal\n"), 0600))
			case "identity":
				must(t, os.WriteFile(filepath.Join(path, markerName), []byte("wrong identity\n"), 0600))
				want = ErrSession
			case "incomplete-successor":
				must(t, os.Mkdir(filepath.Join(path, fmt.Sprintf("rev-%010d", revision+1)), 0700))
				want = ErrConflict
			}
			accepted, err := s.SetManualProgressAcknowledged(tr.ChainID, tr.ID, 40)
			if !errors.Is(err, want) || accepted != 0 {
				t.Fatalf("corrupted no-op got acknowledgment=%d error=%v, want %v", accepted, err, want)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || revision != s.disk.revision {
				t.Fatal("rejected no-op published memory or disk frontier")
			}
		})
	}
}

func TestManualProgressReviewUsesUnrelatedSubtaskClock(t *testing.T) {
	s, path := fresh(t)
	s.clock = func() time.Time { return time.Unix(978307300, 0) }
	must(t, s.AddToday("独立进度目标"))
	target := s.Snapshot().Traces[0]
	must(t, s.AddToday("独立时钟来源"))
	other := s.Snapshot().Traces[1]
	s.clock = func() time.Time { return time.Unix(978317200, 0) }
	_, err := s.AddSubtask(other.ChainID, other.ID, "独立时钟子项", core.DifficultyHard)
	must(t, err)
	before := s.Snapshot()
	frontier := before.Subtasks[0].UpdatedAt
	if !frontier.After(reviewManualTarget(t, s, other.ID).ContentUpdatedAt) {
		t.Fatal("fixture's latest clock must exist only on the unrelated subtask")
	}
	calls := 0
	s.clock = func() time.Time { calls++; return time.Unix(978307201, 0) }
	accepted, err := s.SetManualProgressAcknowledged(target.ChainID, target.ID, 37)
	must(t, err)
	actual := reviewManualTarget(t, s, target.ID)
	want, err := core.InstantFromSeconds(math.Nextafter(frontier.Seconds(), math.Inf(1)))
	must(t, err)
	if accepted != 37 || actual.ContentUpdatedAt != want || calls != 1 {
		t.Fatalf("global frontier mismatch: accepted=%d clock=%v want=%v calls=%d", accepted, actual.ContentUpdatedAt, want, calls)
	}
	after := s.Snapshot()
	unmodified := after.Clone()
	for i := range unmodified.Traces {
		if unmodified.Traces[i].ID == target.ID {
			unmodified.Traces[i] = target
		}
	}
	if !reflect.DeepEqual(before, unmodified) {
		t.Fatal("manual edit changed another domain fact")
	}
	revision := s.disk.revision
	s.clock = func() time.Time { t.Fatal("equal no-op sampled wall clock"); return time.Time{} }
	accepted, err = s.SetManualProgressAcknowledged(target.ChainID, target.ID, 37)
	must(t, err)
	if accepted != 37 || revision != s.disk.revision || !reflect.DeepEqual(after, s.Snapshot()) {
		t.Fatal("equal no-op changed facts")
	}
	must(t, s.Close())
	r, err := Open(path, testDay)
	must(t, err)
	defer r.Close()
	if !reflect.DeepEqual(after, r.Snapshot()) {
		t.Fatal("reopen lost exact normalized clock or progress")
	}
}

func TestManualProgressReviewFloorNoopAndGuardClockPrecedence(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("独立下限任务"))
	old := s.Snapshot().Traces[0]
	must(t, s.SetManualProgress(old.ChainID, old.ID, 68))
	must(t, s.ReturnToPool(old.ChainID, old.ID))
	tomorrow, err := testDay.AddDays(1)
	must(t, err)
	must(t, s.Schedule(old.ChainID, tomorrow))
	var current core.DayTrace
	for _, tr := range s.Snapshot().Traces {
		if tr.Status == core.TracePending {
			current = tr
		}
	}
	must(t, s.AdvanceTo(tomorrow))
	accepted, err := s.SetManualProgressAcknowledged(current.ChainID, current.ID, -1)
	must(t, err)
	if accepted != 68 {
		t.Fatal("historical floor was not normalized", accepted)
	}
	before, revision := s.Snapshot(), s.disk.revision
	s.clock = func() time.Time { t.Fatal("no-op or guard read wall clock"); return time.Time{} }
	for _, request := range []int{math.MinInt, -1, 0, 67, 68} {
		accepted, err = s.SetManualProgressAcknowledged(current.ChainID, current.ID, request)
		must(t, err)
		if accepted != 68 || s.disk.revision != revision {
			t.Fatal("floor no-op acknowledged requested value or wrote")
		}
	}
	for _, route := range []struct {
		chain core.TaskChainID
		trace core.DayTraceID
		want  error
	}{
		{current.ChainID, old.ID, core.ErrImmutableHistory},
		{"00000000-0000-4000-8000-000000009001", current.ID, core.ErrInvalidInput},
		{current.ChainID, "00000000-0000-4000-8000-000000009002", core.ErrNotFound},
	} {
		accepted, err = s.SetManualProgressAcknowledged(route.chain, route.trace, 68)
		if !errors.Is(err, route.want) || accepted != 0 {
			t.Fatalf("guard precedence: accepted=%d error=%v want=%v", accepted, err, route.want)
		}
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != revision {
		t.Fatal("guard or no-op changed aggregate")
	}
}

func TestManualProgressReviewReturnAndDayBoundarySerialization(t *testing.T) {
	for _, boundary := range []string{"return", "advance"} {
		for iteration := 0; iteration < 8; iteration++ {
			t.Run(fmt.Sprintf("%s/%d", boundary, iteration), func(t *testing.T) {
				s, path := fresh(t)
				must(t, s.AddToday("独立竞争任务"))
				tr := s.Snapshot().Traces[0]
				revision := s.disk.revision
				tomorrow, err := testDay.AddDays(1)
				must(t, err)
				var accepted int
				var progressErr, boundaryErr error
				var wg sync.WaitGroup
				start := make(chan struct{})
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					accepted, progressErr = s.SetManualProgressAcknowledged(tr.ChainID, tr.ID, 100)
				}()
				go func() {
					defer wg.Done()
					<-start
					if boundary == "return" {
						boundaryErr = s.ReturnToPool(tr.ChainID, tr.ID)
					} else {
						boundaryErr = s.AdvanceTo(tomorrow)
					}
				}()
				close(start)
				wg.Wait()
				must(t, boundaryErr)
				actual := reviewManualTarget(t, s, tr.ID)
				wantStatus := core.TraceReturnedToPool
				if boundary == "advance" {
					wantStatus = core.TraceUnfinished
				}
				if actual.Status != wantStatus || actual.CompletedAt != nil {
					t.Fatal("100 bypassed boundary or automatically completed")
				}
				if progressErr == nil {
					if accepted != 100 || actual.ManualProgressPercent == nil || *actual.ManualProgressPercent != 100 || s.disk.revision != revision+2 {
						t.Fatal("successful progress was lost or unconfirmed")
					}
				} else if !errors.Is(progressErr, core.ErrImmutableHistory) || accepted != 0 || actual.ManualProgressPercent != nil || s.disk.revision != revision+1 {
					t.Fatalf("rejected progress published: accepted=%d error=%v", accepted, progressErr)
				}
				saved, today := s.Snapshot(), s.Today()
				must(t, s.Close())
				r, err := Open(path, today)
				must(t, err)
				defer r.Close()
				if !reflect.DeepEqual(saved, r.Snapshot()) {
					t.Fatal("boundary race did not reopen exactly")
				}
			})
		}
	}
}

// An exhausted clock is generated by a real domain command, normalized and
// committed through the same isolated aggregate store. Reopening at this extreme
// is a separately reported pre-existing service reconciliation limitation.
func TestManualProgressReviewExhaustedFrontierNoopOnExistingHandle(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("独立最大时钟任务"))
	before := s.Snapshot()
	tr := before.Traces[0]
	maximum, err := core.InstantFromSeconds(math.MaxFloat64)
	must(t, err)
	command := core.SetManualProgressInput{ChainID: tr.ChainID, TraceID: tr.ID, Percent: 40, Today: testDay, Now: maximum}
	k, err := core.NewTaskKernel(before)
	must(t, err)
	must(t, k.SetManualProgress(command))
	wire, err := captureManualProgressHistory(s.snapshot, before, k.Snapshot(), command)
	must(t, err)
	wire, graph, err := normalizeSnapshot(wire)
	must(t, err)
	must(t, s.disk.commit(wire))
	s.graph, s.snapshot = graph, wire
	r := s
	saved, revision := r.Snapshot(), r.disk.revision
	r.clock = func() time.Time { t.Fatal("exhausted no-op read wall clock"); return time.Time{} }
	accepted, err := r.SetManualProgressAcknowledged(tr.ChainID, tr.ID, 40)
	must(t, err)
	if accepted != 40 || revision != r.disk.revision || !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("exhausted no-op changed revision or normalized facts")
	}
	r.clock = func() time.Time { return time.Unix(978307201, 0) }
	accepted, err = r.SetManualProgressAcknowledged(tr.ChainID, tr.ID, 41)
	if !errors.Is(err, core.ErrInvalidClock) || accepted != 0 || revision != r.disk.revision || !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatalf("unrepresentable mutation acknowledged: accepted=%d error=%v", accepted, err)
	}
}
