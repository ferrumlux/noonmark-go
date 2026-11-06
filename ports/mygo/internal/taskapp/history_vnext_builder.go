// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type historyCaptureIntent struct {
	EventID string
	TraceID core.DayTraceID
	Status  core.TraceStatus
	At      core.Instant
}

// A classification candidate is NOT a validated snapshot. Compose it with the
// intended graph and unchanged supplement, then inspect that complete snapshot.
type classificationCandidate struct{ classifications canonical.Value }

func (c classificationCandidate) classificationsCopy() (canonical.Value, error) {
	if c.classifications.Kind != canonical.Object {
		return canonical.Value{}, historyVNextFail(historyProof)
	}
	return historyVNextClone(c.classifications), nil
}

func appendEmptyHistoryEvents(prior validatedHistorySnapshot, intents []historyCaptureIntent) (classificationCandidate, error) {
	fail := func(r historyVNextReason) (classificationCandidate, error) {
		return classificationCandidate{}, historyVNextFail(r)
	}
	if !prior.valid() {
		return fail(historyProof)
	}
	const eventNodes = 15     // Includes object keys; fixed empty category/labels shape.
	const newStreamNodes = 37 // Event+latest, two wrapped keys and a stream array.
	limit := uint64(canonical.DefaultLimits().MaxNodes)
	if uint64(len(intents)) > limit/eventNodes {
		return fail(historyCapacity)
	}
	finalRevision, ok := historyCheckedAdd(prior.proof.count, uint64(len(intents)), ^uint64(0))
	if !ok {
		return fail(historyCapacity)
	}
	nodes := historyVNextNodes(prior.proof.snapshot.Object["classifications"])
	seen := make(map[string]bool, len(intents))
	streams := make(map[string]bool, len(prior.proof.streams))
	for id := range prior.proof.streams {
		streams[id] = true
	}
	// Validate and size the entire request before allocating/cloning the output.
	// No after graph is present: reference/eligibility/final-state checks belong
	// to the complete after inspector and future command replay, not this helper.
	for _, in := range intents {
		if !historyVNextUUID(in.EventID) || !historyVNextUUID(string(in.TraceID)) {
			return fail(historyMalformed)
		}
		id, trace := strings.ToUpper(in.EventID), strings.ToUpper(string(in.TraceID))
		if prior.proof.ids[id] || seen[id] {
			return fail(historyMalformed)
		}
		seen[id] = true
		if !historyCaptureStatus(in.Status) {
			return fail(historyScope)
		}
		if !in.At.Finite() {
			return fail(historyProfile)
		}
		addition := uint64(eventNodes)
		if !streams[trace] {
			addition = newStreamNodes
			streams[trace] = true
		}
		nodes, ok = historyCheckedAdd(nodes, addition, limit)
		if !ok {
			return fail(historyCapacity)
		}
	}
	c := historyVNextClone(prior.proof.snapshot.Object["classifications"])
	latest, events := c.Object["snapshotsByTraceID"], c.Object["snapshotEventsByTraceID"]
	latestPositions, streamPositions := map[string]int{}, map[string]int{}
	for i := 0; i < len(latest.Array); i += 2 {
		latestPositions[historyID(latest.Array[i])] = i + 1
	}
	for i := 0; i < len(events.Array); i += 2 {
		streamPositions[historyID(events.Array[i])] = i + 1
	}
	revision := prior.proof.count
	for _, in := range intents {
		revision++ // Preflight proved the full increment cannot overflow.
		id := strings.ToUpper(string(in.TraceID))
		key := historyObject(map[string]canonical.Value{"rawValue": historyString(id)})
		event := historyObject(map[string]canonical.Value{
			"id": historyString(strings.ToUpper(in.EventID)), "traceID": key, "status": historyString(string(in.Status)),
			"labels": {Kind: canonical.Array, Array: []canonical.Value{}}, "capturedAt": historyNumber(uint64(in.At)), "revision": historyNumber(revision),
		})
		if pos, exists := streamPositions[id]; exists {
			stream := events.Array[pos]
			stream.Array = append(stream.Array, event)
			events.Array[pos] = stream
			latest.Array[latestPositions[id]] = historyVNextClone(event)
		} else {
			streamPositions[id] = len(events.Array) + 1
			latestPositions[id] = len(latest.Array) + 1
			events.Array = append(events.Array, key, canonical.Value{Kind: canonical.Array, Array: []canonical.Value{event}})
			latest.Array = append(latest.Array, historyVNextClone(key), historyVNextClone(event))
		}
	}
	c.Object["snapshotsByTraceID"], c.Object["snapshotEventsByTraceID"] = latest, events
	c.Object["revision"] = historyNumber(finalRevision)
	// Enforce bytes/depth too. This is classification-only structural bounding,
	// deliberately not an aggregate validation or a write authorization.
	if _, err := canonical.Marshal(c); err != nil {
		return fail(historyCapacity)
	}
	return classificationCandidate{c}, nil
}
func historyVNextNodes(v canonical.Value) uint64 {
	n := uint64(1 + len(v.Object))
	for _, c := range v.Object {
		n += historyVNextNodes(c)
	}
	for _, c := range v.Array {
		n += historyVNextNodes(c)
	}
	return n
}

func checkForwardHistorySuccessor(previous, next validatedHistorySnapshot) error {
	if !previous.valid() || !next.valid() {
		return historyVNextFail(historyProof)
	}
	if next.proof.count < previous.proof.count {
		return historyVNextFail(historyPrefix)
	}
	var added uint64
	for key, old := range previous.proof.streams {
		stream, ok := next.proof.streams[key]
		if !ok || len(stream) < len(old) {
			return historyVNextFail(historyPrefix)
		}
		for i, event := range old {
			if !historyEqual(event.value, stream[i].value) {
				return historyVNextFail(historyPrefix)
			}
		}
	}
	for key, stream := range next.proof.streams {
		delta := len(stream) - len(previous.proof.streams[key])
		var ok bool
		added, ok = historyCheckedAdd(added, uint64(delta), next.proof.count)
		if !ok {
			return historyVNextFail(historyPrefix)
		}
	}
	if next.proof.count-previous.proof.count != added {
		return historyVNextFail(historyPrefix)
	}
	for id, day := range previous.proof.days {
		later, ok := next.proof.days[id]
		if !ok || later.Date != day.Date {
			return historyVNextFail(historyPrefix)
		}
		if day.LockedAt != nil && (later.LockedAt == nil || *later.LockedAt != *day.LockedAt) {
			return historyVNextFail(historyPrefix)
		}
	}
	return nil
}
