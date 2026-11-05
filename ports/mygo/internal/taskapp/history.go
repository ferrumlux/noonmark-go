// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// ValidateEmptyHistory validates ONLY the isolated task runtime's empty-catalog
// settlement history. It is not a rich classification validator or a Swift
// interoperability claim. All other classification facts fail closed. Unlike
// codec's empty-audit shortcut, this also rejects missing settlement events.
func ValidateEmptyHistory(snapshot canonical.Value) error {
	s, err := inspectEmptyHistory(snapshot)
	if err != nil {
		return err
	}
	c := s.Object["classifications"]
	for _, key := range []string{"categories", "labels", "currentByChainID", "relationHistory", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones", "committedReceiptsByInteractionID", "changeRecords"} {
		if len(c.Object[key].Array) != 0 {
			return historyError("unsupported " + key)
		}
	}
	if len(s.Object["taskCycleSeries"].Array) != 0 || len(s.Object["ideas"].Array) != 0 {
		return historyError("recurrence and ideas are outside this runtime")
	}
	state, _, err := v7.ToDomain(s)
	if err != nil {
		return err
	}
	days := map[core.LocalDate]core.Day{}
	for _, d := range state.Tasks.Days {
		days[d.Date] = d
	}
	expected := map[string]core.DayTrace{}
	for _, t := range state.Tasks.Traces {
		if t.Status != core.TracePending && t.Status != core.TraceCompleted && t.Status != core.TraceUnfinished {
			return historyError("unsupported trace status")
		}
		d, ok := days[t.Date]
		if !ok {
			return historyError("trace day missing")
		}
		if d.LockedAt == nil {
			if t.Status == core.TraceUnfinished {
				return historyError("unfinished trace on unlocked day")
			}
			continue
		}
		if t.Status == core.TracePending {
			return historyError("pending trace on locked day")
		}
		if d.LockedAt.Before(t.ContentUpdatedAt) || d.LockedAt.Before(t.CreatedAt) {
			return historyError("capture precedes trace clock")
		}
		if t.Status == core.TraceUnfinished && (t.SettledAt == nil || *t.SettledAt != *d.LockedAt) {
			return historyError("unfinished settlement clock mismatch")
		}
		expected[string(t.ID)] = t
	}
	latest := historyPairs(c.Object["snapshotsByTraceID"])
	streams := historyPairs(c.Object["snapshotEventsByTraceID"])
	if len(latest) != len(expected) || len(streams) != len(expected) || historyUint(c.Object["revision"]) != uint64(len(expected)) {
		return historyError("missing/extra snapshot or revision count mismatch")
	}
	type fact struct {
		revision uint64
		at       core.Instant
	}
	facts := make([]fact, 0, len(expected))
	revisions := map[uint64]bool{}
	ids := map[string]bool{}
	for id, trace := range expected {
		stream, ok := streams[id]
		if !ok || len(stream.Array) != 1 {
			return historyError("exactly one settlement event required")
		}
		event := stream.Array[0]
		if !historyEqual(event, latest[id]) {
			return historyError("latest snapshot mismatch")
		}
		o := event.Object
		if _, exists := o["category"]; exists {
			return historyError("historical category unsupported")
		}
		if len(o["labels"].Array) != 0 || o["status"].String != string(trace.Status) || historyID(o["traceID"]) != id {
			return historyError("event classification/status/trace mismatch")
		}
		at := core.Instant(historyUint(o["capturedAt"]))
		if at != *days[trace.Date].LockedAt {
			return historyError("capture clock differs from day lock")
		}
		rev := historyUint(o["revision"])
		if rev == 0 || rev > uint64(len(expected)) || revisions[rev] {
			return historyError("event revision is not unique and contiguous")
		}
		revisions[rev] = true
		eid := strings.ToUpper(o["id"].String)
		if ids[eid] {
			return historyError("duplicate event UUID")
		}
		ids[eid] = true
		facts = append(facts, fact{rev, at})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].revision < facts[j].revision })
	for i := 1; i < len(facts); i++ {
		if facts[i].at.Before(facts[i-1].at) {
			return historyError("capture clock moved backwards")
		}
	}
	return nil
}

// CaptureEmptyHistory takes and returns a COMPLETE v7 snapshot. The supplied
// before graph must exactly match prior's task sections. Only newly locked days
// generate events; an already locked graph with missing audit is never repaired.
// Neither prior nor either graph is mutated, including on failure.
func CaptureEmptyHistory(prior canonical.Value, before, after core.TaskGraph) (canonical.Value, error) {
	fail := func(err error) (canonical.Value, error) { return canonical.Value{}, err }
	if err := ValidateEmptyHistory(prior); err != nil {
		return fail(err)
	}
	// Inspect supplies an independent bounded copy, also defending direct callers.
	prior, err := inspectEmptyHistory(prior)
	if err != nil {
		return fail(err)
	}
	_, extra, err := v7.ToDomain(prior)
	if err != nil {
		return fail(err)
	}
	expected, err := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	if err != nil {
		return fail(err)
	}
	for _, section := range []string{"days", "chains", "definitions", "traces", "subtasks"} {
		if !historyEqual(prior.Object[section], expected.Object[section]) {
			return fail(historyError("prior does not match before graph"))
		}
	}
	result, err := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	if err != nil {
		return fail(err)
	}
	oldDays := map[core.LocalDate]core.Day{}
	newDays := map[core.LocalDate]core.Day{}
	for _, d := range before.Days {
		oldDays[d.Date] = d
	}
	for _, d := range after.Days {
		newDays[d.Date] = d
	}
	for _, d := range before.Days {
		if d.LockedAt != nil {
			n, ok := newDays[d.Date]
			if !ok || n.ID != d.ID || n.LockedAt == nil || *n.LockedAt != *d.LockedAt {
				return fail(historyError("historical day lock changed"))
			}
		}
	}
	// Locked traces and their materialised content are immutable in this narrow
	// runtime. Chain clocks/pool notes are deliberately not frozen: a chain can
	// still have an editable current task. Forward definition supersession is
	// not supported here and must be implemented as a separate audited command.
	lockedTraces := map[string]bool{}
	lockedDefinitions := map[string]bool{}
	for _, t := range before.Traces {
		if oldDays[t.Date].LockedAt != nil {
			lockedTraces[string(t.ID)] = true
			lockedDefinitions[string(t.DefinitionID)] = true
		}
	}
	for _, section := range []string{"traces", "definitions", "subtasks"} {
		oldFacts := map[string]canonical.Value{}
		newFacts := map[string]canonical.Value{}
		selected := func(v canonical.Value) bool {
			switch section {
			case "traces":
				// Dates are encoded identically by FromDomain. Include additions
				// on an already locked date, not merely previously known IDs.
				date := v.Object["date"].Object
				d := core.LocalDate{Year: int(historyUint(date["year"])), Month: int(historyUint(date["month"])), Day: int(historyUint(date["day"]))}
				return oldDays[d].LockedAt != nil || lockedTraces[historyID(v.Object["id"])]
			case "definitions":
				return lockedDefinitions[historyID(v.Object["id"])]
			default:
				return lockedTraces[historyID(v.Object["traceID"])]
			}
		}
		for _, v := range prior.Object[section].Array {
			if selected(v) {
				oldFacts[historyID(v.Object["id"])] = v
			}
		}
		for _, v := range result.Object[section].Array {
			if selected(v) {
				newFacts[historyID(v.Object["id"])] = v
			}
		}
		if len(oldFacts) != len(newFacts) {
			return fail(historyError("locked " + section + " added or removed"))
		}
		for id, v := range oldFacts {
			if !historyEqual(v, newFacts[id]) {
				return fail(historyError("locked " + section + " changed"))
			}
		}
	}
	var capture []core.DayTrace
	for _, t := range after.Traces {
		d := newDays[t.Date]
		if d.LockedAt == nil || oldDays[t.Date].LockedAt != nil {
			continue
		}
		if t.Status == core.TraceCompleted || t.Status == core.TraceUnfinished {
			capture = append(capture, t)
		}
	}
	sort.Slice(capture, func(i, j int) bool {
		if n := capture[i].Date.Compare(capture[j].Date); n != 0 {
			return n < 0
		}
		return capture[i].ID < capture[j].ID
	})
	c := result.Object["classifications"]
	rev := historyUint(c.Object["revision"])
	for _, t := range capture {
		id, err := core.NewIdentity()
		if err != nil {
			return fail(err)
		}
		rev++
		key := historyObject(map[string]canonical.Value{"rawValue": historyString(string(t.ID))})
		event := historyObject(map[string]canonical.Value{
			"id": historyString(id), "traceID": key, "status": historyString(string(t.Status)),
			"labels":     {Kind: canonical.Array, Array: []canonical.Value{}},
			"capturedAt": historyNumber(uint64(*newDays[t.Date].LockedAt)), "revision": historyNumber(rev),
		})
		latest := c.Object["snapshotsByTraceID"]
		latest.Array = append(latest.Array, key, event)
		c.Object["snapshotsByTraceID"] = latest
		streams := c.Object["snapshotEventsByTraceID"]
		streams.Array = append(streams.Array, key, canonical.Value{Kind: canonical.Array, Array: []canonical.Value{event}})
		c.Object["snapshotEventsByTraceID"] = streams
	}
	c.Object["revision"] = historyNumber(rev)
	result.Object["classifications"] = c
	if err := ValidateEmptyHistory(result); err != nil {
		return fail(err)
	}
	return result, nil
}

func inspectEmptyHistory(snapshot canonical.Value) (canonical.Value, error) {
	b, err := canonical.Marshal(historyObject(map[string]canonical.Value{"formatVersion": historyNumber(7), "snapshot": snapshot}))
	if err != nil {
		return canonical.Value{}, err
	}
	inspected, err := v7.New(v7.Options{}).Inspect(b)
	return inspected.Snapshot, err
}
func historyError(reason string) error { return fmt.Errorf("empty classification history: %s", reason) }
func historyObject(v map[string]canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Object, Object: v}
}
func historyString(v string) canonical.Value {
	return canonical.Value{Kind: canonical.String, String: v}
}
func historyNumber(v uint64) canonical.Value {
	return canonical.Value{Kind: canonical.Number, TextNumber: strconv.FormatUint(v, 10)}
}
func historyUint(v canonical.Value) uint64 { n, _ := strconv.ParseUint(v.TextNumber, 10, 64); return n }
func historyID(v canonical.Value) string   { return strings.ToUpper(v.Object["rawValue"].String) }
func historyPairs(v canonical.Value) map[string]canonical.Value {
	out := map[string]canonical.Value{}
	for i := 0; i < len(v.Array); i += 2 {
		out[historyID(v.Array[i])] = v.Array[i+1]
	}
	return out
}
func historyEqual(a, b canonical.Value) bool {
	aa, ae := canonical.Marshal(a)
	bb, be := canonical.Marshal(b)
	return ae == nil && be == nil && bytes.Equal(aa, bb)
}
