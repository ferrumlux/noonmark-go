// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestTraceTitleTodayFutureAndRestart(t *testing.T) {
	for _, delta := range []int{0, 1} {
		s, path := fresh(t)
		date, _ := testDay.AddDays(delta)
		must(t, s.AddOnDate("原题", date))
		trace := s.Snapshot().Traces[0].ID
		must(t, s.SaveTraceTitleInput(trace, "  新标题\r\n第二行  "))
		before := s.Snapshot()
		if before.Definitions[0].Title != "新标题\n第二行" || len(before.Definitions) != 1 {
			t.Fatal("input semantics")
		}
		rev := s.disk.revision
		must(t, s.SaveTraceTitleInput(trace, "新标题\n第二行"))
		if s.disk.revision != rev {
			t.Fatal("no-op consumed revision")
		}
		must(t, s.Close())
		reopened, err := Open(path, testDay)
		must(t, err)
		if !reflect.DeepEqual(before, reopened.Snapshot()) {
			t.Fatal("restart mismatch")
		}
		must(t, reopened.Close())
	}
}

func TestTraceTitleRejectsHistoricalCallbackIntoEditableChain(t *testing.T) {
	s, _ := fresh(t)
	prior, _, _ := titleHistoryFixture(t)
	prior, graph, err := normalizeSnapshot(prior)
	must(t, err)
	must(t, s.disk.commit(prior))
	s.graph, s.snapshot = graph, prior
	before, rev := s.Snapshot(), s.disk.revision
	for _, title := range []string{"偷改", graph.Definitions[0].Title} {
		if err := s.SaveTraceTitleInput(graph.Traces[0].ID, title); !errors.Is(err, core.ErrImmutableHistory) {
			t.Fatal("historical route", err)
		}
		if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(prior, s.snapshot) || s.disk.revision != rev {
			t.Fatal("historical route published")
		}
	}
	// The same chain really does have an editable current trace. Its own route is
	// allowed to fork, while the historical trace and old title remain frozen.
	must(t, s.SaveTraceTitleInput(graph.Traces[1].ID, "当前任务新题"))
	after := s.Snapshot()
	if len(after.Definitions) != 2 || after.Definitions[0].Title != graph.Definitions[0].Title || !reflect.DeepEqual(after.Traces[0], graph.Traces[0]) {
		t.Fatal("history rewritten")
	}
}

func TestTraceTitleMissingCompletedAndNaturalDayGuardsBeforeNoop(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("café"))
	trace := s.Snapshot().Traces[0].ID
	if err := s.SaveTraceTitleInput(core.DayTraceID(titleID(800)), "cafe\u0301"); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, s.SetCompleted(trace, true))
	before, rev := s.Snapshot(), s.disk.revision
	if err := s.SaveTraceTitleInput(trace, "cafe\u0301"); !errors.Is(err, core.ErrInvalidTransition) {
		t.Fatal("completed no-op", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
		t.Fatal("completed changed")
	}
	must(t, s.SetCompleted(trace, false))
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
	before, rev = s.Snapshot(), s.disk.revision
	if err := s.SaveTraceTitleInput(trace, "cafe\u0301"); !errors.Is(err, core.ErrImmutableHistory) {
		t.Fatal("past no-op", err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
		t.Fatal("past changed")
	}
}

func TestTraceTitleStaleBlockedClosedCanonicalNoop(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("café"))
	trace := s.Snapshot().Traces[0].ID
	before, rev := s.Snapshot(), s.disk.revision
	s.disk.blocked = true
	if err := s.SaveTraceTitleInput(trace, "cafe\u0301"); !errors.Is(err, ErrIncomplete) {
		t.Fatal(err)
	}
	s.disk.blocked = false
	other, err := Open(path, testDay)
	must(t, err)
	defer other.Close()
	must(t, other.SaveTraceTitleInput(trace, "另一窗口"))
	if err := s.SaveTraceTitleInput(trace, "cafe\u0301"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision {
		t.Fatal("stale published")
	}
	must(t, s.Close())
	if err := s.SaveTraceTitleInput(trace, "cafe\u0301"); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
}

func TestTraceTitleFaultsNeverPublishAndReopen(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("旧题"))
			before, wire, rev := s.Snapshot(), s.snapshot, s.disk.revision
			trace := before.Traces[0].ID
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			if err := s.SaveTraceTitleInput(trace, "新题"); err == nil {
				t.Fatal("fault accepted")
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || rev != s.disk.revision {
				t.Fatal("failure published")
			}
			s.disk.fault = nil
			if phase != "before-reserve" {
				if err := s.SaveTraceTitleInput(trace, "旧题"); !errors.Is(err, ErrIncomplete) {
					t.Fatal("uncertain acknowledged", err)
				}
			}
			must(t, s.Close())
			r, err := Open(path, testDay)
			switch phase {
			case "before-reserve":
				must(t, err)
				defer r.Close()
				if !reflect.DeepEqual(before, r.Snapshot()) {
					t.Fatal("early fault changed disk")
				}
			case "after-persist":
				must(t, err)
				defer r.Close()
				if r.Snapshot().Definitions[0].Title != "新题" || r.disk.revision != rev+1 {
					t.Fatal("durable uncertain result missing")
				}
			default:
				if !errors.Is(err, ErrIncomplete) {
					if r != nil {
						r.Close()
					}
					t.Fatal("partial reopened", err)
				}
			}
		})
	}
}

func TestTraceTitleConcurrentRolloverFreezesAcceptedValue(t *testing.T) {
	for i := 0; i < 8; i++ {
		s, path := fresh(t)
		must(t, s.AddToday("旧题"))
		trace := s.Snapshot().Traces[0].ID
		next, _ := testDay.AddDays(1)
		start := make(chan struct{})
		edit := make(chan error, 1)
		advance := make(chan error, 1)
		go func() { <-start; edit <- s.SaveTraceTitleInput(trace, "新题") }()
		go func() { <-start; advance <- s.AdvanceTo(next) }()
		close(start)
		editErr := <-edit
		must(t, <-advance)
		if editErr != nil && !errors.Is(editErr, core.ErrImmutableHistory) {
			t.Fatal(editErr)
		}
		want := "新题"
		if editErr != nil {
			want = "旧题"
		}
		before := s.Snapshot()
		if before.Definitions[0].Title != want || before.Traces[0].Status != core.TraceUnfinished {
			t.Fatal("incorrect serialization")
		}
		if err := s.SaveTraceTitleInput(trace, "迟到回调"); !errors.Is(err, core.ErrImmutableHistory) {
			t.Fatal(err)
		}
		must(t, ValidateEmptyHistory(s.snapshot))
		must(t, s.Close())
		r, err := Open(path, next)
		must(t, err)
		if !reflect.DeepEqual(before, r.Snapshot()) {
			t.Fatal("restart differs")
		}
		must(t, r.Close())
	}
}
