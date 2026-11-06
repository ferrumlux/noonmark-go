// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

const historyMachineSlots = 7 * 2 * 3 * 2
const (
	historyP = iota
	historyC
	historyU
	historyA
	historyR
	historyD
	historyX
)
const (
	historyNilToken = iota
	historyAToken
	historyLockToken
)

type historyMachineKey struct{ status, lock, token, anchor int }

func (k historyMachineKey) slot() int { return ((k.status*2+k.lock)*3+k.token)*2 + k.anchor }
func historyMachineDecode(i int) historyMachineKey {
	return historyMachineKey{i / 12, (i / 6) % 2, (i / 2) % 3, i % 2}
}

type historyMachineSlot struct {
	used  bool
	lower float64
}
type historyMachineSet [historyMachineSlots]historyMachineSlot

func (s *historyMachineSet) put(k historyMachineKey, lower float64) bool {
	i := k.slot()
	if !s[i].used || lower < s[i].lower {
		s[i] = historyMachineSlot{true, lower}
		return true
	}
	return false
}

type historyMachineStats struct{ MaxSlots, MaxClosureRounds int }

func (s *historyMachineStats) merge(o historyMachineStats) {
	if o.MaxSlots > s.MaxSlots {
		s.MaxSlots = o.MaxSlots
	}
	if o.MaxClosureRounds > s.MaxClosureRounds {
		s.MaxClosureRounds = o.MaxClosureRounds
	}
}
func (s *historyMachineStats) count(set *historyMachineSet) {
	n := 0
	for _, v := range set {
		if v.used {
			n++
		}
	}
	if n > s.MaxSlots {
		s.MaxSlots = n
	}
}

// Fixed-index dominance: token source selects either the most recent A's exact
// bits or the stored lock's exact bits. Neither is merged by float equality.
// Equal keys have the same future guards. Their earliest numeric lower bound
// dominates later ones. Cycles cannot lower it; every useful epsilon path is
// simple and has <84 edges. We retain all equal-time lock/event orderings.
func historyVNextClosure(set *historyMachineSet, t core.DayTrace, d core.Day, bound float64, noEvents bool, stats *historyMachineStats) error {
	for round := 1; round <= historyMachineSlots; round++ {
		changed := false
		for i, slot := range *set {
			if !slot.used {
				continue
			}
			k := historyMachineDecode(i)
			if k.lock == 0 && slot.lower <= bound && slot.lower <= t.ContentUpdatedAt.Seconds() {
				if k.status == historyP {
					n := k
					n.status = historyC
					changed = set.put(n, slot.lower) || changed
					if t.Status == core.TraceCompleted && t.CompletedAt != nil && slot.lower <= t.CompletedAt.Seconds() && t.CompletedAt.Seconds() <= bound {
						n.anchor = 1
						changed = set.put(n, t.CompletedAt.Seconds()) || changed
					}
					if noEvents && t.Status == core.TraceCancelledDraft && t.SettledAt != nil && slot.lower <= t.SettledAt.Seconds() && t.SettledAt.Seconds() <= bound {
						n = historyMachineKey{status: historyX}
						changed = set.put(n, t.SettledAt.Seconds()) || changed
					}
				}
				if k.status == historyC && k.anchor == 0 || k.status == historyA {
					n := historyMachineKey{status: historyP}
					changed = set.put(n, slot.lower) || changed
				}
			}
			if k.lock == 0 && d.LockedAt != nil && historyHasLaterDay(t.Date) && slot.lower <= d.LockedAt.Seconds() && d.LockedAt.Seconds() <= bound {
				switch k.status {
				case historyU, historyA, historyR, historyD, historyX:
					n := k
					n.lock = 1
					changed = set.put(n, d.LockedAt.Seconds()) || changed
				}
			}
		}
		stats.count(set)
		if round > stats.MaxClosureRounds {
			stats.MaxClosureRounds = round
		}
		if !changed {
			return nil
		}
	}
	// This is a proved finite graph bound, not a path truncation or timeout.
	return historyVNextFail(historyProfile)
}
func historyVNextScan(t core.DayTrace, d core.Day, events []historyVNextEvent) (historyMachineStats, error) {
	stats := historyMachineStats{}
	set := historyMachineSet{}
	set.put(historyMachineKey{status: historyP}, t.CreatedAt.Seconds())
	var lastA core.Instant
	for _, event := range events {
		if err := historyVNextClosure(&set, t, d, event.at.Seconds(), false, &stats); err != nil {
			return stats, err
		}
		next := historyMachineSet{}
		for i, slot := range set {
			if !slot.used || slot.lower > event.at.Seconds() {
				continue
			}
			k := historyMachineDecode(i)
			if k.lock == 0 && d.LockedAt != nil && d.LockedAt.Before(event.at) {
				continue
			}
			switch event.status {
			case core.TraceAbandoned:
				if k.status == historyP || k.status == historyU {
					next.put(historyMachineKey{status: historyA, lock: k.lock, token: historyAToken}, event.at.Seconds())
				}
			case core.TraceUnfinished:
				if k.status == historyA && (k.lock == 1 || historyHasLaterDay(t.Date)) {
					next.put(historyMachineKey{status: historyU, lock: k.lock, token: historyAToken}, event.at.Seconds())
				}
				if k.status == historyP && k.lock == 0 && d.LockedAt != nil && event.at == *d.LockedAt && historyHasLaterDay(t.Date) {
					next.put(historyMachineKey{status: historyU, lock: 1, token: historyLockToken}, event.at.Seconds())
				}
			case core.TraceCompleted:
				if k.status == historyC && k.anchor == 1 && k.lock == 0 && d.LockedAt != nil && event.at == *d.LockedAt && historyHasLaterDay(t.Date) {
					next.put(historyMachineKey{status: historyC, lock: 1, anchor: 1}, event.at.Seconds())
				}
			case core.TraceReturnedToPool, core.TraceDeferred:
				if k.status == historyP && k.lock == 0 {
					status := historyR
					if event.status == core.TraceDeferred {
						status = historyD
					}
					next.put(historyMachineKey{status: status}, event.at.Seconds())
				}
			}
		}
		if event.status == core.TraceAbandoned {
			lastA = event.at
		}
		set = next
		stats.count(&set)
	}
	bound := t.ContentUpdatedAt.Seconds()
	if d.LockedAt != nil && d.LockedAt.Seconds() > bound {
		bound = d.LockedAt.Seconds()
	}
	if err := historyVNextClosure(&set, t, d, bound, len(events) == 0, &stats); err != nil {
		return stats, err
	}
	want := map[core.TraceStatus]int{core.TracePending: historyP, core.TraceCompleted: historyC, core.TraceUnfinished: historyU, core.TraceAbandoned: historyA, core.TraceReturnedToPool: historyR, core.TraceDeferred: historyD, core.TraceCancelledDraft: historyX}[t.Status]
	locked := 0
	if d.LockedAt != nil {
		locked = 1
	}
	for i, slot := range set {
		k := historyMachineDecode(i)
		if !slot.used || k.status != want || k.lock != locked || slot.lower > bound || want == historyC && k.anchor != 1 {
			continue
		}
		switch want {
		case historyP, historyC:
			if k.token != historyNilToken || t.SettledAt != nil {
				continue
			}
		case historyA, historyU:
			if t.SettledAt == nil {
				continue
			}
			if k.token == historyAToken {
				if *t.SettledAt != lastA {
					continue
				}
			} else if k.token == historyLockToken {
				if d.LockedAt == nil || *t.SettledAt != *d.LockedAt {
					continue
				}
			} else {
				continue
			}
		}
		return stats, nil
	}
	if !historyHasLaterDay(t.Date) && t.Status == core.TraceUnfinished {
		return stats, historyVNextFail(historyToday)
	}
	return stats, historyVNextFail(historyProfile)
}
