// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var testDay = core.LocalDate{Year: 2026, Month: 11, Day: 5}

func fresh(t *testing.T) (*Service, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session")
	s, e := Create(p, testDay)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func TestCreateScheduleCompleteUndoRestart(t *testing.T) {
	s, p := fresh(t)
	must(t, s.AddPool("  合成任务一  "))
	g := s.Snapshot()
	if len(g.Chains) != 1 || len(g.Definitions) != 1 || g.Definitions[0].Title != "合成任务一" || len(g.Traces) != 0 {
		t.Fatal("pool graph")
	}
	must(t, s.Schedule(g.Chains[0].ID, testDay))
	g = s.Snapshot()
	must(t, s.SetCompleted(g.Traces[0].ID, true))
	saved := s.Snapshot()
	must(t, s.Close())
	r, e := Open(p, testDay)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("restart changed exact graph")
	}
	must(t, r.SetCompleted(g.Traces[0].ID, false))
	if r.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("undo")
	}
}
func TestAtomicCreateTodayAndFutureGuard(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("今天"))
	g := s.Snapshot()
	if len(g.Chains) != 1 || len(g.Definitions) != 1 || len(g.Traces) != 1 || len(g.Days) != 1 {
		t.Fatal("incomplete task graph")
	}
	must(t, s.AddPool("明天"))
	tomorrow, _ := testDay.AddDays(1)
	must(t, s.Schedule(s.Snapshot().Chains[1].ID, tomorrow))
	g = s.Snapshot()
	if e := s.SetCompleted(g.Traces[1].ID, true); !errors.Is(e, core.ErrFuturePlanCannotComplete) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(g, s.Snapshot()) {
		t.Fatal("failed transition published")
	}
}
func TestRejectedTitleAndScheduleDoNotReserve(t *testing.T) {
	s, _ := fresh(t)
	before := s.disk.revision
	if e := s.AddToday(" \t "); !errors.Is(e, core.ErrInvalidTitle) {
		t.Fatal(e)
	}
	if s.disk.revision != before || len(s.Snapshot().Chains) != 0 {
		t.Fatal("invalid task persisted")
	}
	must(t, s.AddPool("one"))
	g := s.Snapshot()
	past, _ := testDay.AddDays(-1)
	if e := s.Schedule(g.Chains[0].ID, past); e == nil {
		t.Fatal("past schedule accepted")
	}
	if !reflect.DeepEqual(g, s.Snapshot()) {
		t.Fatal("invalid schedule published")
	}
}
func TestRepeatedCommandDoesNotDuplicateHistory(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("one"))
	id := s.Snapshot().Traces[0].ID
	must(t, s.SetCompleted(id, true))
	before := s.Snapshot()
	if e := s.SetCompleted(id, true); !errors.Is(e, core.ErrInvalidTransition) {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("repeated command duplicated")
	}
}
func TestFailedSaveNeverPublishes(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit"} {
		t.Run(phase, func(t *testing.T) {
			s, p := fresh(t)
			before := s.Snapshot()
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			if e := s.AddToday("not published"); e == nil {
				t.Fatal("fault ignored")
			}
			if !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("candidate published before persistence")
			}
			r, e := Open(p, testDay)
			if phase == "before-reserve" {
				must(t, e)
				r.Close()
			} else if !errors.Is(e, ErrIncomplete) {
				if r != nil {
					r.Close()
				}
				t.Fatal(e)
			}
		})
	}
}
func TestConcurrentHandlesRejectLostUpdate(t *testing.T) {
	s, p := fresh(t)
	other, e := Open(p, testDay)
	must(t, e)
	defer other.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, svc := range []*Service{s, other} {
		wg.Add(1)
		go func(s *Service) { defer wg.Done(); errs <- s.AddPool("concurrent synthetic") }(svc)
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("ok %d conflicts %d", ok, conflict)
	}
	r, e := Open(p, testDay)
	must(t, e)
	defer r.Close()
	if len(r.Snapshot().Chains) != 1 {
		t.Fatal("lost update or duplicate")
	}
}
func TestServiceSerializesAndReturnsDeepCopies(t *testing.T) {
	s, _ := fresh(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.AddPool("parallel"); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	g := s.Snapshot()
	if len(g.Chains) != 8 {
		t.Fatal(len(g.Chains))
	}
	g.Definitions[0].Title = "changed copy"
	if s.Snapshot().Definitions[0].Title == "changed copy" {
		t.Fatal("snapshot aliases service")
	}
}
func TestBackwardsWallClockAdvancesFrontier(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("first"))
	first := s.Snapshot().Traces[0].ContentUpdatedAt
	s.clock = func() time.Time { return time.Unix(0, 0) }
	must(t, s.SetCompleted(s.Snapshot().Traces[0].ID, true))
	if !s.Snapshot().Traces[0].ContentUpdatedAt.After(first) {
		t.Fatal("clock went backwards")
	}
}
func TestForeignExistingAndSymlinkSessionsFailClosed(t *testing.T) {
	p := t.TempDir()
	original := []byte("do not touch")
	must(t, os.WriteFile(filepath.Join(p, "unrelated"), original, 0600))
	if _, e := Create(p, testDay); e == nil {
		t.Fatal("reused existing directory")
	}
	if _, e := Open(p, testDay); e == nil {
		t.Fatal("opened foreign directory")
	}
	b, e := os.ReadFile(filepath.Join(p, "unrelated"))
	must(t, e)
	if string(b) != string(original) {
		t.Fatal("foreign file changed")
	}
	s, path := fresh(t)
	s.Close()
	link := filepath.Join(t.TempDir(), "link")
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(link, testDay); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestCorruptionAndPartialRevisionsNeverRepair(t *testing.T) {
	for _, kind := range []string{"digest", "payload", "marker", "incomplete", "unexpected", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, p := fresh(t)
			s.Close()
			switch kind {
			case "digest":
				must(t, os.WriteFile(filepath.Join(p, "rev-0000000001/COMMITTED"), []byte(strings.Repeat("a", 64)+"\n"), 0600))
			case "payload":
				must(t, os.WriteFile(filepath.Join(p, "rev-0000000001/state.json"), []byte("{}"), 0600))
			case "marker":
				must(t, os.WriteFile(filepath.Join(p, markerName), []byte("wrong"), 0600))
			case "incomplete":
				must(t, os.Mkdir(filepath.Join(p, "rev-0000000002"), 0700))
			case "unexpected":
				must(t, os.WriteFile(filepath.Join(p, "unknown"), []byte("x"), 0600))
			case "symlink":
				must(t, os.Rename(filepath.Join(p, "rev-0000000001/state.json"), filepath.Join(p, "payload")))
				must(t, os.Symlink(filepath.Join(p, "payload"), filepath.Join(p, "rev-0000000001/state.json")))
			}
			before, _ := os.ReadDir(p)
			if r, e := Open(p, testDay); e == nil {
				r.Close()
				t.Fatal("corruption accepted")
			}
			after, _ := os.ReadDir(p)
			if len(before) != len(after) {
				t.Fatal("silently repaired")
			}
		})
	}
}
func TestBoundedRecordAndClosedService(t *testing.T) {
	s, p := fresh(t)
	must(t, s.Close())
	if e := s.AddPool("closed"); !errors.Is(e, ErrSession) {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(p, "rev-0000000001/state.json"), os.O_WRONLY, 0600)
	must(t, e)
	must(t, f.Truncate(maxRecordBytes+1))
	must(t, f.Close())
	if r, e := Open(p, testDay); e == nil {
		r.Close()
		t.Fatal("oversize accepted")
	}
}
func TestNativePayloadIsNotSwiftImport(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("synthetic"))
	payload, e := encodeSnapshot(s.snapshot)
	must(t, e)
	wire, e := decodeSnapshot(payload)
	state, _, e2 := v7.ToDomain(wire)
	must(t, e2)
	g := state.Tasks
	must(t, e)
	if !reflect.DeepEqual(g, s.Snapshot()) {
		t.Fatal("transport changed state")
	}
}

func TestReopenNextDaySettlesHistoryWithoutChangingPastCompletion(t *testing.T) {
	s, p := fresh(t)
	must(t, s.AddToday("pending"))
	must(t, s.AddToday("completed"))
	must(t, s.SetCompleted(s.Snapshot().Traces[1].ID, true))
	completed := s.Snapshot().Traces[1]
	s.Close()
	tomorrow, _ := testDay.AddDays(1)
	r, e := Open(p, tomorrow)
	must(t, e)
	defer r.Close()
	g := r.Snapshot()
	if g.Traces[0].Status != core.TraceUnfinished || g.Days[0].LockedAt == nil || !reflect.DeepEqual(g.Traces[1], completed) {
		t.Fatal("settlement changed completed history or missed unfinished")
	}
	if e = r.SetCompleted(g.Traces[1].ID, false); !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatal(e)
	}
}

func TestRejectUnsupportedSupplementsInsteadOfDiscarding(t *testing.T) {
	s, _ := fresh(t)
	b, e := encodeSnapshot(s.snapshot)
	must(t, e)
	changed := strings.Replace(string(b), `"text" : ""`, `"text" : "unsupported preference"`, 1)
	if changed == string(b) {
		t.Fatal("fixture did not change")
	}
	if _, e := decodeSnapshot([]byte(changed)); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("unsupported supplement was discarded: %v", e)
	}
}
