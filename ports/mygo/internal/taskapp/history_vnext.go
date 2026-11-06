// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"sort"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// This candidate is deliberately NOT wired into the v2 reader, writer or codec.
// It proves necessary per-stream consistency for one fixed forward-clock
// ordinary profile, NOT aggregate Service reachability or permission to write.
type historyVNextReason string

const (
	historyMalformed  historyVNextReason = "malformed structure"
	historyScope      historyVNextReason = "unsupported scope"
	historyProfile    historyVNextReason = "unsupported profile chronology"
	historyMissing    historyVNextReason = "missing capture"
	historyOrder      historyVNextReason = "invalid event order"
	historyCompletion historyVNextReason = "invalid completedAt witness"
	historyToday      historyVNextReason = "no legal Today"
	historyPrefix     historyVNextReason = "invalid prefix"
	historyProof      historyVNextReason = "unvalidated snapshot"
	historyCapacity   historyVNextReason = "capacity exceeded"
)

type historyVNextError struct{ Reason historyVNextReason }

func (e *historyVNextError) Error() string             { return "candidate ordinary history: " + string(e.Reason) }
func historyVNextFail(reason historyVNextReason) error { return &historyVNextError{reason} }

// No exported constructor, mutable view, graph pointer or index escapes.
// The zero value is not a proof. Even a valid proof is not a write capability.
type validatedHistorySnapshot struct{ proof *historySnapshotProof }
type historySnapshotProof struct {
	seal     *historyProfileSeal
	snapshot canonical.Value
	streams  map[string][]historyVNextEvent
	ids      map[string]bool
	days     map[core.DayID]core.Day
	count    uint64
	stats    historyMachineStats
}
type historyProfileSeal struct{ profile byte }

var forwardClockOrdinaryHistoryVNext = &historyProfileSeal{1}

func (v validatedHistorySnapshot) valid() bool {
	return v.proof != nil && v.proof.seal == forwardClockOrdinaryHistoryVNext
}
func (v validatedHistorySnapshot) snapshotCopy() (canonical.Value, error) {
	if !v.valid() {
		return canonical.Value{}, historyVNextFail(historyProof)
	}
	return historyVNextClone(v.proof.snapshot), nil
}

type historyVNextEvent struct {
	value    canonical.Value
	status   core.TraceStatus
	at       core.Instant
	revision uint64
}

func inspectForwardClockOrdinaryHistoryVNext(input canonical.Value) (validatedHistorySnapshot, error) {
	fail := func(r historyVNextReason) (validatedHistorySnapshot, error) {
		return validatedHistorySnapshot{}, historyVNextFail(r)
	}
	// Marshal bounds caller-built ASTs (including cycles) BEFORE copying/walking.
	snapshot, err := inspectEmptyHistory(input)
	if err != nil {
		return fail(historyMalformed)
	}
	if err = validateRuntimeScope(snapshot); err != nil {
		return fail(historyScope)
	}
	c := snapshot.Object["classifications"]
	for _, key := range []string{"categories", "labels", "currentByChainID", "relationHistory", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones", "committedReceiptsByInteractionID", "changeRecords"} {
		if len(c.Object[key].Array) != 0 {
			return fail(historyScope)
		}
	}
	state, _, err := v7.ToDomain(snapshot)
	if err != nil {
		return fail(historyMalformed)
	}
	proof := &historySnapshotProof{snapshot: snapshot, streams: map[string][]historyVNextEvent{}, ids: map[string]bool{}, days: map[core.DayID]core.Day{}}
	days := map[core.LocalDate]core.Day{}
	for _, day := range state.Tasks.Days {
		if day.LockedAt != nil && !historyHasLaterDay(day.Date) {
			return fail(historyToday)
		}
		days[day.Date], proof.days[day.ID] = day, day
	}
	traces := map[string]core.DayTrace{}
	for _, trace := range state.Tasks.Traces {
		if trace.Status == core.TraceChanged {
			return fail(historyScope)
		}
		traces[string(trace.ID)] = trace
	}
	latest, streams := historyPairs(c.Object["snapshotsByTraceID"]), historyPairs(c.Object["snapshotEventsByTraceID"])
	if len(latest) != len(streams) {
		return fail(historyMissing)
	}
	// Count checked from bounded arrays, never allocate by claimed revision.
	var count uint64
	for id, stream := range streams {
		if len(stream.Array) == 0 {
			return fail(historyMissing)
		}
		if _, ok := traces[id]; !ok {
			return fail(historyMalformed)
		}
		if !historyEqual(latest[id], stream.Array[len(stream.Array)-1]) {
			return fail(historyMalformed)
		}
		var ok bool
		count, ok = historyCheckedAdd(count, uint64(len(stream.Array)), uint64(canonical.DefaultLimits().MaxNodes))
		if !ok {
			return fail(historyCapacity)
		}
	}
	if historyUint(c.Object["revision"]) != count {
		return fail(historyOrder)
	}
	facts := make([]historyVNextEvent, 0, int(count))
	revisions := make(map[uint64]bool, int(count))
	for id, stream := range streams {
		var previous uint64
		events := make([]historyVNextEvent, 0, len(stream.Array))
		for _, value := range stream.Array {
			o := value.Object
			status := core.TraceStatus(o["status"].String)
			at := core.Instant(historyUint(o["capturedAt"]))
			if _, exists := o["category"]; exists {
				return fail(historyScope)
			}
			if len(o["labels"].Array) != 0 || !historyCaptureStatus(status) {
				return fail(historyScope)
			}
			if historyID(o["traceID"]) != id {
				return fail(historyMalformed)
			}
			if !at.Finite() || at.Before(traces[id].CreatedAt) {
				return fail(historyProfile)
			}
			revision := historyUint(o["revision"])
			if revision <= previous || revision > count || revisions[revision] {
				return fail(historyOrder)
			}
			previous, revisions[revision] = revision, true
			uuid := strings.ToUpper(o["id"].String)
			if !historyVNextUUID(uuid) || proof.ids[uuid] {
				return fail(historyMalformed)
			}
			proof.ids[uuid] = true
			event := historyVNextEvent{value: value, status: status, at: at, revision: revision}
			events, facts = append(events, event), append(facts, event)
		}
		proof.streams[id] = events
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].revision < facts[j].revision })
	for i, event := range facts {
		if event.revision != uint64(i)+1 {
			return fail(historyOrder)
		}
		if i > 0 && event.at.Before(facts[i-1].at) {
			return fail(historyProfile)
		}
	}
	for _, trace := range state.Tasks.Traces {
		day := days[trace.Date]
		events := proof.streams[string(trace.ID)]
		if err = historyVNextFinal(trace, day, events); err != nil {
			return validatedHistorySnapshot{}, err
		}
		stats, e := historyVNextScan(trace, day, events)
		if e != nil {
			return validatedHistorySnapshot{}, e
		}
		proof.stats.merge(stats)
	}
	proof.count, proof.seal = count, forwardClockOrdinaryHistoryVNext
	return validatedHistorySnapshot{proof}, nil
}

// Direct final-field checks complement, but never replace, the lock-aware NFA.
func historyVNextFinal(t core.DayTrace, d core.Day, events []historyVNextEvent) error {
	fail := historyVNextFail
	if d.LockedAt != nil && d.LockedAt.Before(t.CreatedAt) {
		return fail(historyProfile)
	}
	var tail *historyVNextEvent
	if len(events) > 0 {
		tail = &events[len(events)-1]
	}
	sameSettled := func(at core.Instant) bool {
		return t.SettledAt != nil && *t.SettledAt == at && t.ContentUpdatedAt == at
	}
	for _, event := range events {
		if event.status != core.TraceCompleted && event.at.After(t.ContentUpdatedAt) {
			return fail(historyProfile)
		}
	}
	switch t.Status {
	case core.TracePending:
		if d.LockedAt != nil {
			return fail(historyProfile)
		}
		if t.SettledAt != nil || tail != nil && (tail.status != core.TraceAbandoned || tail.at.After(t.ContentUpdatedAt)) {
			return fail(historyOrder)
		}
	case core.TraceCompleted:
		if t.CompletedAt == nil || t.SettledAt != nil {
			return fail(historyCompletion)
		}
		for _, event := range events {
			if event.status == core.TraceAbandoned && event.at.After(*t.CompletedAt) {
				return fail(historyCompletion)
			}
		}
		if d.LockedAt == nil {
			if tail != nil && tail.status != core.TraceAbandoned {
				return fail(historyOrder)
			}
		} else {
			if tail == nil {
				return fail(historyMissing)
			}
			if tail.status != core.TraceCompleted || tail.at != *d.LockedAt || d.LockedAt.Before(t.ContentUpdatedAt) || d.LockedAt.Before(*t.CompletedAt) {
				return fail(historyCompletion)
			}
		}
	case core.TraceAbandoned:
		if tail == nil {
			return fail(historyMissing)
		}
		if tail.status != core.TraceAbandoned || !sameSettled(tail.at) {
			return fail(historyOrder)
		}
	case core.TraceUnfinished:
		if tail == nil {
			return fail(historyMissing)
		}
		if tail.status != core.TraceUnfinished || t.ContentUpdatedAt != tail.at || t.SettledAt == nil {
			return fail(historyOrder)
		}
		settlement := d.LockedAt != nil && tail.at == *d.LockedAt && *t.SettledAt == tail.at
		reactivation := len(events) >= 2 && events[len(events)-2].status == core.TraceAbandoned && *t.SettledAt == events[len(events)-2].at
		if !settlement && !reactivation {
			return fail(historyOrder)
		}
	case core.TraceReturnedToPool, core.TraceDeferred:
		if tail == nil {
			return fail(historyMissing)
		}
		if tail.status != t.Status || !sameSettled(tail.at) {
			return fail(historyOrder)
		}
		if d.LockedAt != nil && d.LockedAt.Before(tail.at) {
			return fail(historyProfile)
		}
	case core.TraceCancelledDraft:
		if tail != nil {
			return fail(historyOrder)
		}
		if t.SettledAt == nil || !sameSettled(*t.SettledAt) || t.DraftCancelledOn == nil || !t.DraftCancelledOn.Valid() || t.DraftCancelledOn.Compare(t.Date) >= 0 {
			return fail(historyToday)
		}
		if d.LockedAt != nil && d.LockedAt.Before(*t.SettledAt) {
			return fail(historyProfile)
		}
	default:
		return fail(historyScope)
	}
	return nil
}
func historyHasLaterDay(d core.LocalDate) bool {
	return d.Compare(core.LocalDate{Year: 9999, Month: 12, Day: 31}) < 0
}
func historyCaptureStatus(s core.TraceStatus) bool {
	switch s {
	case core.TraceAbandoned, core.TraceUnfinished, core.TraceCompleted, core.TraceReturnedToPool, core.TraceDeferred:
		return true
	}
	return false
}
func historyCheckedAdd(a, b, limit uint64) (uint64, bool) {
	if a > limit || b > limit-a {
		return 0, false
	}
	return a + b, true
}
func historyVNextUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Only call on already bounded, independently owned values.
func historyVNextClone(v canonical.Value) canonical.Value {
	out := v
	if v.Object != nil {
		out.Object = make(map[string]canonical.Value, len(v.Object))
		for k, c := range v.Object {
			out.Object[k] = historyVNextClone(c)
		}
	}
	if v.Array != nil {
		out.Array = make([]canonical.Value, len(v.Array))
		for i, c := range v.Array {
			out.Array[i] = historyVNextClone(c)
		}
	}
	return out
}
