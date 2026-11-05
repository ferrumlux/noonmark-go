// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"reflect"
	"testing"
)

func TestTraceTitleAcknowledgesActualCommittedBytes(t *testing.T) {
	for _, pair := range [][2]string{{"café", "cafe\u0301"}, {"cafe\u0301", "café"}} {
		s, path := fresh(t)
		must(t, s.AddToday(pair[0]))
		trace := s.Snapshot().Traces[0].ID
		before, rev := s.Snapshot(), s.disk.revision
		actual, err := s.SaveTraceTitleInputAcknowledged(trace, pair[1])
		must(t, err)
		if actual != pair[0] || s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) {
			t.Fatal("canonical no-op did not acknowledge actual bytes")
		}
		actual, err = s.SaveTraceTitleInputAcknowledged(trace, "  新题\r\n第二行  ")
		must(t, err)
		if actual != "新题\n第二行" || actual != s.Snapshot().Definitions[0].Title {
			t.Fatal("acknowledged uncommitted input")
		}
		must(t, s.Close())
		reopened, err := Open(path, testDay)
		must(t, err)
		if reopened.Snapshot().Definitions[0].Title != actual {
			t.Fatal("ack does not match disk")
		}
		must(t, reopened.Close())
	}
}

func TestTraceTitleFailuresReturnNoAcknowledgment(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.AddToday("旧题"))
			before := s.Snapshot()
			trace := before.Traces[0].ID
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			actual, err := s.SaveTraceTitleInputAcknowledged(trace, "新题")
			if err == nil || actual != "" || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("uncertain save acknowledged", actual, err)
			}
		})
	}
}

func TestTraceTitleStaleCanonicalNoopReturnsNoAcknowledgment(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("café"))
	trace := s.Snapshot().Traces[0].ID
	other, err := Open(path, testDay)
	must(t, err)
	defer other.Close()
	_, err = other.SaveTraceTitleInputAcknowledged(trace, "另一窗口")
	must(t, err)
	actual, err := s.SaveTraceTitleInputAcknowledged(trace, "cafe\u0301")
	if !errors.Is(err, ErrConflict) || actual != "" {
		t.Fatal("stale no-op acknowledged", actual, err)
	}
}

func TestTraceTitleConcurrentAcknowledgmentsBelongToEachRequest(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("旧题"))
	trace := s.Snapshot().Traces[0].ID
	type result struct {
		expected, actual string
		err              error
	}
	start := make(chan struct{})
	out := make(chan result, 2)
	for _, text := range []string{"甲", "乙"} {
		go func(text string) {
			<-start
			actual, err := s.SaveTraceTitleInputAcknowledged(trace, text)
			out <- result{text, actual, err}
		}(text)
	}
	close(start)
	for i := 0; i < 2; i++ {
		r := <-out
		must(t, r.err)
		if r.actual != r.expected {
			t.Fatal("receipt read a later request", r)
		}
	}
	if len(s.Snapshot().Definitions) != 1 {
		t.Fatal("input inflated definitions")
	}
}
