// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func futureAggregate(t *testing.T, s *Service) []byte {
	t.Helper()
	b, err := encodeSnapshot(s.snapshot)
	must(t, err)
	return b
}

func TestAddOnDateFutureLifecycleAndAggregateRestart(t *testing.T) {
	s, p := fresh(t)
	future, _ := testDay.AddDays(2)
	revision := s.disk.revision
	must(t, s.AddOnDate("  合成未来任务  ", future))
	g := s.Snapshot()
	if s.disk.revision != revision+1 || len(g.Chains) != 1 || len(g.Definitions) != 1 || len(g.Traces) != 1 || len(g.Days) != 1 || g.Traces[0].Date != future || g.Definitions[0].Title != "合成未来任务" {
		t.Fatal("quick add did not persist one complete scheduled aggregate")
	}
	before := futureAggregate(t, s)
	if err := s.SetCompleted(g.Traces[0].ID, true); !errors.Is(err, core.ErrFuturePlanCannotComplete) {
		t.Fatal(err)
	}
	if string(before) != string(futureAggregate(t, s)) || s.disk.revision != revision+1 {
		t.Fatal("future completion changed aggregate")
	}
	must(t, s.AdvanceTo(future))
	if s.disk.revision != revision+1 {
		t.Fatal("arrival unnecessarily committed a revision")
	}
	must(t, s.SetCompleted(g.Traces[0].ID, true))
	if s.disk.revision != revision+2 {
		t.Fatal("completion revision")
	}
	completed := s.Snapshot().Traces[0]
	next, _ := future.AddDays(1)
	must(t, s.AdvanceTo(next))
	if s.disk.revision != revision+3 || s.Snapshot().Days[0].LockedAt == nil || !reflect.DeepEqual(completed, s.Snapshot().Traces[0]) {
		t.Fatal("settlement changed completion or revision")
	}
	classes := s.snapshot.Object["classifications"]
	if historyUint(classes.Object["revision"]) != 1 || len(classes.Object["snapshotEventsByTraceID"].Array) != 2 {
		t.Fatal("settlement missed classification audit")
	}
	saved := futureAggregate(t, s)
	revision = s.disk.revision
	must(t, s.Close())
	r, err := Open(p, next)
	must(t, err)
	defer r.Close()
	if string(saved) != string(futureAggregate(t, r)) || r.disk.revision != revision {
		t.Fatal("restart changed complete aggregate or audit")
	}
}

func TestAddOnDateRejectedWithoutOrphansOrRevision(t *testing.T) {
	past, _ := testDay.AddDays(-1)
	future, _ := testDay.AddDays(1)
	for _, tc := range []struct {
		name, title string
		date        core.LocalDate
		want        error
	}{
		{"past", "合成", past, core.ErrInvalidTransition},
		{"zero", "合成", core.LocalDate{}, core.ErrInvalidInput},
		{"invalid day", "合成", core.LocalDate{Year: 2026, Month: 2, Day: 30}, core.ErrInvalidInput},
		{"invalid title", " \t ", future, core.ErrInvalidTitle},
		{"locked today", "合成", testDay, core.ErrImmutableHistory},
		{"locked future", "合成", future, core.ErrImmutableHistory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p := fresh(t)
			if tc.name == "locked today" || tc.name == "locked future" {
				must(t, s.AddOnDate("合成历史", tc.date))
				later, _ := tc.date.AddDays(1)
				must(t, s.AdvanceTo(later))
				must(t, s.AdvanceTo(testDay))
			}
			before := futureAggregate(t, s)
			revision := s.disk.revision
			entries, err := os.ReadDir(p)
			must(t, err)
			if err = s.AddOnDate(tc.title, tc.date); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			after, err := os.ReadDir(p)
			must(t, err)
			if string(before) != string(futureAggregate(t, s)) || s.disk.revision != revision || len(entries) != len(after) {
				t.Fatal("rejection created an orphan, audit mutation or reservation")
			}
		})
	}
}

func TestAddOnDateTodayAndPoolShortcutsRemainAtomic(t *testing.T) {
	s, _ := fresh(t)
	revision := s.disk.revision
	must(t, s.AddPool("合成池任务"))
	must(t, s.AddToday("合成今天快捷"))
	must(t, s.AddOnDate("合成今天日期", testDay))
	g := s.Snapshot()
	if s.disk.revision != revision+3 || len(g.Chains) != 3 || len(g.Definitions) != 3 || len(g.Traces) != 2 || len(g.Days) != 1 {
		t.Fatal("shortcut regression")
	}
	for _, trace := range g.Traces {
		if trace.Date != testDay {
			t.Fatal("wrong shortcut date")
		}
	}
}

func TestAddOnDateFailureNeverPublishesPartialAggregate(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, p := fresh(t)
			// Retain a real settled audit to catch replacing the complete aggregate
			// with only task facts on either the success or failure path.
			must(t, s.AddToday("合成历史"))
			today, _ := testDay.AddDays(1)
			must(t, s.AdvanceTo(today))
			future, _ := today.AddDays(1)
			before := futureAggregate(t, s)
			revision := s.disk.revision
			s.disk.fault = func(at string) error {
				if at == phase {
					return ErrIncomplete
				}
				return nil
			}
			if err := s.AddOnDate("合成未来", future); !errors.Is(err, ErrIncomplete) {
				t.Fatal(err)
			}
			if string(before) != string(futureAggregate(t, s)) || s.disk.revision != revision || s.Today() != today {
				t.Fatal("failed commit published candidate")
			}
			s.disk.fault = nil
			if phase == "before-reserve" {
				must(t, s.AddOnDate("合成重试", future))
				if s.disk.revision != revision+1 {
					t.Fatal("safe retry did not commit once")
				}
			} else if err := s.AddOnDate("禁止盲目重试", future); !errors.Is(err, ErrIncomplete) {
				t.Fatalf("uncertain writer was not blocked: %v", err)
			}
			r, err := Open(p, today)
			if phase == "after-reserve" || phase == "before-commit" {
				if r != nil {
					r.Close()
				}
				if !errors.Is(err, ErrIncomplete) {
					t.Fatalf("partial revision reopened: %v", err)
				}
				return
			}
			must(t, err)
			defer r.Close()
			g := r.Snapshot()
			if r.disk.revision != revision+1 || len(g.Chains) != 2 || len(g.Definitions) != 2 || len(g.Traces) != 2 || len(g.Days) != 2 {
				t.Fatal("restart did not recover exactly one whole scheduled task")
			}
			if !reflect.DeepEqual(s.snapshot.Object["classifications"], r.snapshot.Object["classifications"]) {
				t.Fatal("restart changed prior audit")
			}
		})
	}
}

func TestAddOnDateConcurrentHandlesRejectLostUpdate(t *testing.T) {
	s, p := fresh(t)
	other, err := Open(p, testDay)
	must(t, err)
	defer other.Close()
	future, _ := testDay.AddDays(1)
	before := futureAggregate(t, s)
	revision := s.disk.revision
	var wg sync.WaitGroup
	errs := make([]error, 2)
	services := []*Service{s, other}
	for i, svc := range services {
		wg.Add(1)
		go func(i int, svc *Service) { defer wg.Done(); errs[i] = svc.AddOnDate("合成并发", future) }(i, svc)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for i, err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
			if services[i].disk.revision != revision || string(before) != string(futureAggregate(t, services[i])) {
				t.Fatal("conflicting writer published candidate")
			}
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("success %d conflict %d", ok, conflict)
	}
	r, err := Open(p, testDay)
	must(t, err)
	defer r.Close()
	g := r.Snapshot()
	if r.disk.revision != revision+1 || len(g.Chains) != 1 || len(g.Definitions) != 1 || len(g.Traces) != 1 || len(g.Days) != 1 {
		t.Fatal("lost update or orphan task")
	}
}

func TestAddTodayResolvesDateAfterWaitingForRollover(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("合成旧日"))
	next, _ := testDay.AddDays(1)
	entered, release := make(chan struct{}), make(chan struct{})
	wall := time.Now()
	s.clock = func() time.Time { close(entered); <-release; return wall }
	advanced := make(chan error, 1)
	go func() { advanced <- s.AdvanceTo(next) }()
	<-entered // AdvanceTo owns the transaction lock but has not published today.
	added := make(chan error, 1)
	go func() { added <- s.AddToday("合成新日") }()
	// Restore the clock while its only invocation is paused; subsequent calls
	// cannot run until AdvanceTo releases the lock.
	s.clock = func() time.Time { return wall }
	close(release)
	must(t, <-advanced)
	must(t, <-added)
	g := s.Snapshot()
	if len(g.Traces) != 2 || g.Traces[1].Date != next || g.Traces[0].Status != core.TraceUnfinished {
		t.Fatal("today shortcut captured a pre-rollover date")
	}
}
