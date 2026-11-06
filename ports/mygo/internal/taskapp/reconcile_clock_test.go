// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Only real commands build the graph. The aggregate test seam uses the same
// replay, normalization and disk commit as the manual-progress service.
func installReconcileMaximum(t *testing.T, s *Service) {
	t.Helper()
	before := s.Snapshot()
	tr := before.Traces[0]
	now, err := core.InstantFromSeconds(math.MaxFloat64)
	must(t, err)
	command := core.SetManualProgressInput{ChainID: tr.ChainID, TraceID: tr.ID, Percent: 40, Today: s.Today(), Now: now}
	k, err := core.NewTaskKernel(before)
	must(t, err)
	must(t, k.SetManualProgress(command))
	wire, err := captureManualProgressHistory(s.snapshot, before, k.Snapshot(), command)
	must(t, err)
	wire, graph, err := normalizeSnapshot(wire)
	must(t, err)
	must(t, s.disk.commit(wire))
	s.graph, s.snapshot = graph, wire
}
func TestReconcileClockOpenMatrix(t *testing.T) {
	for _, maximum := range []bool{false, true} {
		for _, advance := range []bool{false, true} {
			name := "ordinary"
			if maximum {
				name = "maximum"
			}
			if advance {
				name += "/next-day"
			} else {
				name += "/same-day"
			}
			t.Run(name, func(t *testing.T) {
				s, path := fresh(t)
				must(t, s.AddToday("结算极值"))
				if maximum {
					installReconcileMaximum(t, s)
				}
				before, revision := s.Snapshot(), s.disk.revision
				wire, err := encodeSnapshot(s.snapshot)
				must(t, err)
				target := testDay
				if advance {
					target, err = testDay.AddDays(1)
					must(t, err)
				}
				must(t, s.Close())
				r, err := Open(path, target)
				if maximum && advance {
					if !errors.Is(err, core.ErrInvalidClock) || r != nil {
						t.Fatalf("exhausted settlement: %v", err)
					}
					r, err = Open(path, testDay)
					must(t, err)
					defer r.Close()
					after, err := encodeSnapshot(r.snapshot)
					must(t, err)
					if !reflect.DeepEqual(before, r.Snapshot()) || r.disk.revision != revision || string(wire) != string(after) {
						t.Fatal("failed open changed persisted aggregate")
					}
					return
				}
				must(t, err)
				defer r.Close()
				if !advance {
					after, err := encodeSnapshot(r.snapshot)
					must(t, err)
					if !reflect.DeepEqual(before, r.Snapshot()) || r.disk.revision != revision || string(wire) != string(after) {
						t.Fatal("same-day open mutated")
					}
				} else {
					if r.disk.revision != revision+1 || r.Snapshot().Traces[0].Status != core.TraceUnfinished {
						t.Fatal("open skipped settlement")
					}
					must(t, ValidateEmptyHistory(r.snapshot))
					must(t, r.Close())
					again, err := Open(path, target)
					must(t, err)
					defer again.Close()
					if !reflect.DeepEqual(r.Snapshot(), again.Snapshot()) || again.disk.revision != revision+1 {
						t.Fatal("reopen duplicated settlement")
					}
				}
			})
		}
	}
}
func TestReconcileClockNoopAndUnsettledSameDay(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "empty", false: "future-maximum"}[empty], func(t *testing.T) {
			s, _ := fresh(t)
			if !empty {
				must(t, s.AddToday("尚未过去"))
				installReconcileMaximum(t, s)
			}
			before, revision := s.Snapshot(), s.disk.revision
			s.clock = func() time.Time { t.Fatal("no-op sampled clock"); return time.Time{} }
			must(t, s.AdvanceTo(testDay))
			earlier, err := testDay.AddDays(-1)
			must(t, err)
			must(t, s.AdvanceTo(earlier))
			if s.Today() != earlier || s.disk.revision != revision || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("no-op changed aggregate")
			}
		})
	}
	t.Run("same-today-unsettled-history", func(t *testing.T) {
		s, _ := fresh(t)
		must(t, s.AddToday("仍待结算"))
		next, err := testDay.AddDays(1)
		must(t, err)
		// Open sets today to the caller's date before reconciling persisted facts.
		// Reproduce that state directly to guard against an equality shortcut.
		s.today = next
		frontier, err := s.persistedFrontier()
		must(t, err)
		s.clock = func() time.Time { return time.Unix(978307201, 0) }
		revision := s.disk.revision
		must(t, s.AdvanceTo(next))
		g := s.Snapshot()
		if g.Traces[0].Status != core.TraceUnfinished || !g.Traces[0].ContentUpdatedAt.After(frontier) || s.disk.revision != revision+1 {
			t.Fatal("same-today skipped facts or lowered clock")
		}
		must(t, ValidateEmptyHistory(s.snapshot))
	})
	t.Run("exhausted-advance-is-atomic", func(t *testing.T) {
		s, _ := fresh(t)
		must(t, s.AddToday("不准溢出"))
		installReconcileMaximum(t, s)
		before, revision := s.Snapshot(), s.disk.revision
		next, err := testDay.AddDays(1)
		must(t, err)
		if err = s.AdvanceTo(next); !errors.Is(err, core.ErrInvalidClock) {
			t.Fatal(err)
		}
		if s.Today() != testDay || s.disk.revision != revision || !reflect.DeepEqual(before, s.Snapshot()) {
			t.Fatal("failed settlement published")
		}
	})
}
func TestReconcileClockNoopSafetyMatrix(t *testing.T) {
	for _, fault := range []string{"closed", "blocked", "old-payload", "old-seal", "marker", "conflict"} {
		t.Run(fault, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("完整前缀"))
			installReconcileMaximum(t, s)
			before, revision := s.Snapshot(), s.disk.revision
			next, err := testDay.AddDays(-1)
			must(t, err)
			want := ErrCorrupt
			switch fault {
			case "closed":
				must(t, s.Close())
				want = ErrSession
			case "blocked":
				s.disk.blocked = true
				want = ErrIncomplete
			case "old-payload":
				p := filepath.Join(path, "rev-0000000001", "state.json")
				b, err := os.ReadFile(p)
				must(t, err)
				must(t, os.WriteFile(p, append(b, ' '), 0600))
			case "old-seal":
				must(t, os.WriteFile(filepath.Join(path, "rev-0000000001", "COMMITTED"), []byte("broken"), 0600))
			case "marker":
				must(t, os.WriteFile(filepath.Join(path, markerName), []byte("broken"), 0600))
				want = ErrSession
			case "conflict":
				r, err := Open(path, testDay)
				must(t, err)
				defer r.Close()
				// A separate valid committed revision makes this handle stale even when
				// it contains the same facts. No production store is involved.
				must(t, r.disk.commit(r.snapshot))
				want = ErrConflict
			}
			s.clock = func() time.Time { t.Fatal("rejected no-op sampled clock"); return time.Time{} }
			for _, day := range []core.LocalDate{testDay, next} {
				if err = s.AdvanceTo(day); !errors.Is(err, want) {
					t.Fatalf("got %v want %v", err, want)
				}
				if s.Today() != testDay || s.disk.revision != revision || !reflect.DeepEqual(before, s.Snapshot()) {
					t.Fatal("rejected no-op published")
				}
			}
			if fault == "old-payload" || fault == "old-seal" || fault == "marker" {
				r, err := Open(path, testDay)
				if r != nil {
					r.Close()
				}
				// openStore maps a failed revision scan to ErrIncomplete;
				// live verifyPrefix reports the exact corrupt-prefix class.
				openWant := ErrIncomplete
				if fault == "marker" {
					openWant = ErrSession
				}
				if !errors.Is(err, openWant) {
					t.Fatalf("corrupt open: %v", err)
				}
			}
		})
	}
}
func TestReconcileClockFailureMatrix(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("原子结算"))
			before := s.Snapshot()
			wire, err := encodeSnapshot(s.snapshot)
			must(t, err)
			next, err := testDay.AddDays(1)
			must(t, err)
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			if err = s.AdvanceTo(next); !errors.Is(err, ErrIncomplete) {
				t.Fatal(err)
			}
			after, err := encodeSnapshot(s.snapshot)
			must(t, err)
			if s.Today() != testDay || !reflect.DeepEqual(before, s.Snapshot()) || string(wire) != string(after) {
				t.Fatal("failure published task/audit/today")
			}
			s.disk.fault = nil
			if phase != "before-reserve" {
				if err = s.AdvanceTo(testDay); !errors.Is(err, ErrIncomplete) {
					t.Fatalf("blocked same-day allowed: %v", err)
				}
			}
			must(t, s.Close())
			r, err := Open(path, testDay)
			switch phase {
			case "before-reserve":
				must(t, err)
				defer r.Close()
				if !reflect.DeepEqual(before, r.Snapshot()) {
					t.Fatal("early failure changed disk")
				}
			case "after-persist":
				must(t, err)
				defer r.Close()
				if r.Snapshot().Traces[0].Status != core.TraceUnfinished {
					t.Fatal("persisted settlement lost")
				}
				must(t, ValidateEmptyHistory(r.snapshot))
			default:
				if r != nil {
					r.Close()
				}
				if !errors.Is(err, ErrIncomplete) {
					t.Fatalf("partial store opened: %v", err)
				}
			}
		})
	}
}
