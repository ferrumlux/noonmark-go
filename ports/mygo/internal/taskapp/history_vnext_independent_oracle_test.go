// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// This deliberately slower oracle retains full concrete clock/settled/completed
// values in every state, never uses the implementation's slot keys, dominance,
// anchor bit, closure routine or token abstractions. Candidate epsilon clocks
// are fixed interval endpoints. Enumeration is finite because this exact state
// set is finite; it has no timeout or path-count success fallback.
type iaConcreteState struct {
	status                             core.TraceStatus
	locked                             bool
	consumed                           int
	clock, content, settled, completed core.Instant
	hasSettled, hasCompleted           bool
}

func iaConcretePossible(t core.DayTrace, d core.Day, events []historyVNextEvent) bool {
	times := map[core.Instant]bool{t.CreatedAt: true, t.ContentUpdatedAt: true}
	if t.CompletedAt != nil {
		times[*t.CompletedAt] = true
	}
	if t.SettledAt != nil {
		times[*t.SettledAt] = true
	}
	if d.LockedAt != nil {
		times[*d.LockedAt] = true
	}
	for _, e := range events {
		times[e.at] = true
	}
	bound := t.ContentUpdatedAt
	if d.LockedAt != nil && d.LockedAt.After(bound) {
		bound = *d.LockedAt
	}
	first := iaConcreteState{status: core.TracePending, clock: t.CreatedAt, content: t.CreatedAt}
	queue := []iaConcreteState{first}
	seen := map[iaConcreteState]bool{first: true}
	push := func(s iaConcreteState) {
		if !seen[s] {
			seen[s] = true
			queue = append(queue, s)
		}
	}
	later := t.Date.Compare(core.LocalDate{Year: 9999, Month: 12, Day: 31}) < 0
	for head := 0; head < len(queue); head++ {
		s := queue[head]
		if s.consumed == len(events) && s.status == t.Status && s.locked == (d.LockedAt != nil) && !s.clock.After(bound) {
			settleOK := (t.SettledAt == nil && !s.hasSettled) || (t.SettledAt != nil && s.hasSettled && *t.SettledAt == s.settled)
			completeOK := (t.CompletedAt == nil && !s.hasCompleted) || (t.CompletedAt != nil && s.hasCompleted && *t.CompletedAt == s.completed)
			contentOK := s.content == t.ContentUpdatedAt
			if s.status == core.TracePending || s.status == core.TraceCompleted {
				contentOK = !s.content.After(t.ContentUpdatedAt)
			}
			if settleOK && completeOK && contentOK {
				return true
			}
		}
		if !s.locked {
			for at := range times {
				if at.Before(s.clock) || at.After(bound) || at.After(t.ContentUpdatedAt) {
					continue
				}
				n := s
				n.clock, n.content = at, at
				switch s.status {
				case core.TracePending:
					n.status = core.TraceCompleted
					n.hasCompleted = true
					n.completed = at
					push(n)
				case core.TraceCompleted:
					n.status = core.TracePending
					n.hasCompleted = false
					n.completed = 0
					push(n)
				case core.TraceAbandoned:
					n.status = core.TracePending
					n.hasSettled = false
					n.settled = 0
					push(n)
				}
				if s.status == core.TracePending && len(events) == 0 && t.Status == core.TraceCancelledDraft && t.SettledAt != nil && at == *t.SettledAt && t.DraftCancelledOn != nil && t.DraftCancelledOn.Valid() && t.DraftCancelledOn.Compare(t.Date) < 0 {
					n = s
					n.status = core.TraceCancelledDraft
					n.clock, n.content, n.settled = at, at, at
					n.hasSettled = true
					push(n)
				}
			}
		}
		if !s.locked && d.LockedAt != nil && later && !d.LockedAt.Before(s.clock) && !d.LockedAt.After(bound) {
			n := s
			n.locked = true
			n.clock = *d.LockedAt
			switch s.status {
			case core.TracePending, core.TraceCompleted:
				if s.consumed < len(events) {
					event := events[s.consumed]
					want := core.TraceCompleted
					if s.status == core.TracePending {
						want = core.TraceUnfinished
					}
					if event.status == want && event.at == *d.LockedAt {
						n.consumed++
						if want == core.TraceUnfinished {
							n.status = want
							n.content, n.settled = *d.LockedAt, *d.LockedAt
							n.hasSettled = true
						}
						push(n)
					}
				}
			case core.TraceUnfinished, core.TraceAbandoned, core.TraceReturnedToPool, core.TraceDeferred, core.TraceCancelledDraft:
				push(n)
			}
		}
		if s.consumed < len(events) {
			event := events[s.consumed]
			if event.at.Before(s.clock) || event.at.After(bound) {
				continue
			}
			n := s
			n.clock, n.content = event.at, event.at
			n.consumed++
			switch event.status {
			case core.TraceAbandoned:
				if s.status == core.TracePending || s.status == core.TraceUnfinished {
					n.status = event.status
					n.settled = event.at
					n.hasSettled = true
					push(n)
				}
			case core.TraceUnfinished:
				if s.status == core.TraceAbandoned && (s.locked || later) {
					n.status = event.status
					push(n)
				}
			case core.TraceReturnedToPool, core.TraceDeferred:
				if s.status == core.TracePending && !s.locked {
					n.status = event.status
					n.settled = event.at
					n.hasSettled = true
					push(n)
				}
			}
		}
	}
	return false
}

func TestIndependentHistoryExactStateOracle(t *testing.T) {
	statuses := []core.TraceStatus{core.TraceAbandoned, core.TraceUnfinished, core.TraceCompleted, core.TraceReturnedToPool, core.TraceDeferred}
	finals := []core.TraceStatus{core.TracePending, core.TraceCompleted, core.TraceAbandoned, core.TraceUnfinished, core.TraceReturnedToPool, core.TraceDeferred}
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	checked := 0
	var walk func([]core.TraceStatus, int)
	walk = func(seq []core.TraceStatus, remaining int) {
		for _, equal := range []bool{false, true} {
			events := make([]historyVNextEvent, len(seq))
			for i, status := range seq {
				at := float64(i + 1)
				if equal {
					at = 1
				}
				events[i] = historyVNextEvent{status: status, at: iaNow(at)}
			}
			for _, status := range finals {
				for lock := -1; lock <= len(seq)+1; lock++ {
					for terminal := 0; terminal <= len(seq)+1; terminal++ {
						tr := core.DayTrace{Date: d, Status: status, CreatedAt: iaNow(0), ContentUpdatedAt: iaNow(float64(len(seq) + 1))}
						day := core.Day{Date: d}
						if lock >= 0 {
							day.LockedAt = iaPtr(iaNow(float64(lock)))
						}
						if status == core.TraceCompleted {
							tr.CompletedAt = iaPtr(iaNow(float64(terminal)))
						} else if status != core.TracePending {
							tr.SettledAt = iaPtr(iaNow(float64(terminal)))
							if len(events) > 0 {
								tr.ContentUpdatedAt = events[len(events)-1].at
							}
						}
						if historyVNextFinal(tr, day, events) != nil {
							continue
						}
						_, err := historyVNextScan(tr, day, events)
						got := err == nil
						want := iaConcretePossible(tr, day, events)
						checked++
						if got != want {
							t.Fatalf("NFA differs from exact-state oracle: seq=%v equal=%v final=%s lock=%d terminal=%d got=%v want=%v err=%v", seq, equal, status, lock, terminal, got, want, err)
						}
					}
				}
			}
		}
		if remaining > 0 {
			for _, status := range statuses {
				next := append(append([]core.TraceStatus{}, seq...), status)
				walk(next, remaining-1)
			}
		}
	}
	walk(nil, 3)
	if checked < 100 {
		t.Fatal("oracle matrix unexpectedly empty")
	}
	t.Log(fmt.Sprintf("compared %d final-field-valid concrete histories against independent finite-state BFS", checked))
}
