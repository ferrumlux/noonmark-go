// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func historyFixture(t *testing.T) (canonical.Value, core.TaskGraph, core.TaskGraph) {
	t.Helper()
	k, err := core.NewTaskKernel(core.TaskGraph{})
	must(t, err)
	d := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	tomorrow, _ := d.AddDays(1)
	id := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
	for i := 1; i <= 3; i++ {
		now, _ := core.InstantFromSeconds(float64(i))
		chain := core.TaskChainID(id(i))
		def := core.TaskDefinitionID(id(i + 10))
		trace := core.DayTraceID(id(i + 20))
		must(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: chain, DefinitionID: def, Title: "synthetic", Now: now}))
		date := d
		dayID := core.DayID(id(40))
		if i == 3 {
			date = tomorrow
			dayID = core.DayID(id(41))
		}
		must(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: chain, TraceID: trace, DayID: dayID, Date: date, Today: d, Now: now}))
		if i == 1 {
			must(t, k.MarkCompleted(trace, d, now))
		}
	}
	before := k.Snapshot()
	extra, err := emptySupplement()
	must(t, err)
	prior, err := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	must(t, err)
	now, _ := core.InstantFromSeconds(10)
	must(t, k.SettleTaskDays(tomorrow, now))
	return prior, before, k.Snapshot()
}

func TestEmptyHistoryCapturePreserveAndIdempotence(t *testing.T) {
	prior, before, after := historyFixture(t)
	original, err := canonical.Marshal(prior)
	must(t, err)
	result, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	must(t, ValidateEmptyHistory(result))
	b, err := canonical.Marshal(prior)
	must(t, err)
	if string(b) != string(original) {
		t.Fatal("capture mutated prior")
	}
	c := result.Object["classifications"]
	if historyUint(c.Object["revision"]) != 2 {
		t.Fatal("both completed and unfinished require capture")
	}
	events := c.Object["snapshotEventsByTraceID"].Array
	if len(events) != 4 || events[1].Array[0].Object["status"].String != "completed" || events[3].Array[0].Object["status"].String != "unfinished" {
		t.Fatal("wrong event order/status")
	}
	if historyUint(events[1].Array[0].Object["revision"]) != 1 || historyUint(events[3].Array[0].Object["revision"]) != 2 {
		t.Fatal("nondeterministic revision order")
	}
	again, err := CaptureEmptyHistory(result, after, after)
	must(t, err)
	if !historyEqual(result, again) {
		t.Fatal("idempotent call changed history")
	}
	k, err := core.NewTaskKernel(after)
	must(t, err)
	day := core.LocalDate{Year: 2026, Month: 11, Day: 3}
	now, _ := core.InstantFromSeconds(20)
	must(t, k.SettleTaskDays(day, now))
	extended, err := CaptureEmptyHistory(result, after, k.Snapshot())
	must(t, err)
	newEvents := extended.Object["classifications"].Object["snapshotEventsByTraceID"].Array
	if len(newEvents) != 6 || !reflect.DeepEqual(events, newEvents[:4]) {
		t.Fatal("earlier events changed")
	}
	codec := v7.New(v7.Options{ClassificationAuditValidator: ValidateEmptyHistory})
	encoded, err := codec.Encode(extended)
	must(t, err)
	doc, err := codec.Decode(encoded)
	must(t, err)
	if !historyEqual(extended, doc.Snapshot()) {
		t.Fatal("codec round trip changed history")
	}
	if _, err := v7.New(v7.Options{}).Encode(extended); err == nil {
		t.Fatal("default codec audit gate bypassed")
	}
}

func TestEmptyHistoryRejectsCorruption(t *testing.T) {
	prior, before, after := historyFixture(t)
	valid, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	mutations := map[string]func(canonical.Value){
		"truncate all": func(v canonical.Value) {
			c := v.Object["classifications"]
			c.Object["revision"] = historyNumber(0)
			c.Object["snapshotsByTraceID"] = canonical.Value{Kind: canonical.Array}
			c.Object["snapshotEventsByTraceID"] = canonical.Value{Kind: canonical.Array}
		},
		"truncate one": func(v canonical.Value) {
			c := v.Object["classifications"]
			a := c.Object["snapshotEventsByTraceID"]
			a.Array = a.Array[:2]
			c.Object["snapshotEventsByTraceID"] = a
		},
		"revision count":     func(v canonical.Value) { v.Object["classifications"].Object["revision"] = historyNumber(3) },
		"revision duplicate": func(v canonical.Value) { historyMutateEvent(v, 1, "revision", historyNumber(1)) },
		"revision zero":      func(v canonical.Value) { historyMutateEvent(v, 0, "revision", historyNumber(0)) },
		"revision gap":       func(v canonical.Value) { historyMutateEvent(v, 1, "revision", historyNumber(3)) },
		"clock":              func(v canonical.Value) { historyMutateEvent(v, 0, "capturedAt", historyNumber(0)) },
		"status":             func(v canonical.Value) { historyMutateEvent(v, 0, "status", historyString("unfinished")) },
		"unsupported status": func(v canonical.Value) { historyMutateEvent(v, 0, "status", historyString("abandoned")) },
		"duplicate UUID": func(v canonical.Value) {
			id := v.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["id"]
			historyMutateEvent(v, 1, "id", id)
		},
		"forged reference": func(v canonical.Value) {
			historyMutateEvent(v, 0, "traceID", historyObject(map[string]canonical.Value{"rawValue": historyString("00000000-0000-4000-8000-000000000099")}))
		},
		"latest mismatch": func(v canonical.Value) {
			v.Object["classifications"].Object["snapshotsByTraceID"].Array[1].Object["id"] = historyString("00000000-0000-4000-8000-000000000099")
		},
		"null category": func(v canonical.Value) { historyMutateEvent(v, 0, "category", canonical.Value{Kind: canonical.Null}) },
		"extra event": func(v canonical.Value) {
			c := v.Object["classifications"]
			a := c.Object["snapshotEventsByTraceID"]
			a.Array[1].Array = append(a.Array[1].Array, a.Array[1].Array[0])
			c.Object["snapshotEventsByTraceID"] = a
		},
		"unknown field": func(v canonical.Value) { v.Object["classifications"].Object["unknown"] = historyNumber(1) },
		"empty current relation": func(v canonical.Value) {
			key := v.Object["chains"].Array[0].Object["id"]
			v.Object["classifications"].Object["currentByChainID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{key, historyObject(map[string]canonical.Value{"labels": {Kind: canonical.Array}})}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			v, err := inspectEmptyHistory(valid)
			must(t, err)
			mutate(v)
			if ValidateEmptyHistory(v) == nil {
				t.Fatal("accepted corruption")
			}
		})
	}
	for _, key := range []string{"categories", "labels", "relationHistory", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones", "committedReceiptsByInteractionID", "changeRecords"} {
		t.Run("unsupported "+key, func(t *testing.T) {
			v, err := inspectEmptyHistory(valid)
			must(t, err)
			v.Object["classifications"].Object[key] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyObject(nil)}}
			if ValidateEmptyHistory(v) == nil {
				t.Fatal("unsupported facts accepted")
			}
		})
	}
}

// Independently update the two duplicated wire values, as a forged but
// structurally consistent payload might do. Merely relying on latest mismatch
// would not exercise revision, clock, status or UUID audit checks.
func historyMutateEvent(v canonical.Value, index int, key string, value canonical.Value) {
	c := v.Object["classifications"]
	c.Object["snapshotEventsByTraceID"].Array[index*2+1].Array[0].Object[key] = value
	c.Object["snapshotsByTraceID"].Array[index*2+1].Object[key] = value
}

func TestEmptyHistoryNeverReconstructsMissingAudit(t *testing.T) {
	prior, before, after := historyFixture(t)
	extra, err := emptySupplement()
	must(t, err)
	lost, err := v7.FromDomain(recurrence.State{Tasks: after}, extra)
	must(t, err)
	if ValidateEmptyHistory(lost) == nil {
		t.Fatal("missing history accepted")
	}
	if _, err := CaptureEmptyHistory(lost, after, after); err == nil {
		t.Fatal("invented authoritative history")
	}
	initial, err := canonical.Marshal(prior)
	must(t, err)
	if _, err := CaptureEmptyHistory(prior, after, after); err == nil {
		t.Fatal("before graph mismatch accepted")
	}
	bad := after.Clone()
	bad.Days[0].LockedAt = nil
	if _, err := CaptureEmptyHistory(prior, before, bad); err == nil {
		t.Fatal("unfinished unlocked graph accepted")
	}
	b, err := canonical.Marshal(prior)
	must(t, err)
	if string(initial) != string(b) {
		t.Fatal("error mutated prior")
	}
	empty, err := v7.FromDomain(recurrence.State{}, extra)
	must(t, err)
	must(t, ValidateEmptyHistory(empty))
}

func TestEmptyHistoryRejectsBackwardCaptureClock(t *testing.T) {
	prior, before, after := historyFixture(t)
	valid, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	k, err := core.NewTaskKernel(after)
	must(t, err)
	// Day 2's task clocks are 3, so 9 is domain-valid but earlier than the
	// already captured audit frontier at 10. The aggregate must reject it.
	now, _ := core.InstantFromSeconds(9)
	must(t, k.SettleTaskDays(core.LocalDate{Year: 2026, Month: 11, Day: 3}, now))
	if _, err := CaptureEmptyHistory(valid, after, k.Snapshot()); err == nil {
		t.Fatal("backward aggregate audit clock accepted")
	}
}

func TestEmptyHistoryProtectsLockedFactsButAllowsCurrentCommands(t *testing.T) {
	prior, before, after := historyFixture(t)
	valid, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	mutations := map[string]func(*core.TaskGraph){
		"title":       func(g *core.TaskGraph) { g.Definitions[0].Title = "rewritten history" },
		"description": func(g *core.TaskGraph) { s := "rewritten history"; g.Traces[0].DescriptionText = &s },
		"trace notes": func(g *core.TaskGraph) {
			g.Traces[0].NoteEntries = append(g.Traces[0].NoteEntries, core.TaskNoteEntry{ID: "00000000-0000-4000-8000-000000000090", Body: "injected", CreatedAt: g.Traces[0].CreatedAt, UpdatedAt: g.Traces[0].CreatedAt})
		},
		"delete trace": func(g *core.TaskGraph) { g.Traces = g.Traces[1:] },
		"inject subtask": func(g *core.TaskGraph) {
			now := g.Traces[0].CreatedAt
			g.Subtasks = append(g.Subtasks, core.Subtask{ID: "00000000-0000-4000-8000-000000000090", LineageID: "00000000-0000-4000-8000-000000000091", TraceID: g.Traces[0].ID, Title: "injected", Status: core.SubtaskCompleted, Difficulty: core.DifficultySimple, CreatedAt: now, UpdatedAt: now, CompletedAt: &now})
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			g := after.Clone()
			mutate(&g)
			if _, err := CaptureEmptyHistory(valid, after, g); err == nil {
				t.Fatal("locked fact mutation accepted")
			}
		})
	}
	k, err := core.NewTaskKernel(after)
	must(t, err)
	now, _ := core.InstantFromSeconds(15)
	must(t, k.MarkCompleted(after.Traces[2].ID, after.Traces[2].Date, now))
	updated, err := CaptureEmptyHistory(valid, after, k.Snapshot())
	must(t, err)
	if !historyEqual(valid.Object["classifications"], updated.Object["classifications"]) {
		t.Fatal("current command modified settled audit")
	}
}

func TestEmptyHistoryRejectsStructurallyValidRichCatalog(t *testing.T) {
	prior, before, after := historyFixture(t)
	valid, err := CaptureEmptyHistory(prior, before, after)
	must(t, err)
	for _, kind := range []string{"categories", "labels"} {
		t.Run(kind, func(t *testing.T) {
			v, err := inspectEmptyHistory(valid)
			must(t, err)
			key := historyObject(map[string]canonical.Value{"rawValue": historyString("00000000-0000-4000-8000-000000000099")})
			item := historyObject(map[string]canonical.Value{"id": key, "name": historyString("Synthetic"), "canonicalKey": historyString("synthetic"), "canonicalKeyVersion": historyString("synthetic-test"), "colorHex": historyString("#123456"), "lifecycle": historyString("active"), "nameVersions": {Kind: canonical.Array}, "createdAt": historyNumber(0), "updatedAt": historyNumber(0)})
			if kind == "categories" {
				item.Object["presentationApproval"] = historyString("userApproved")
			}
			v.Object["classifications"].Object[kind] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{key, item}}
			if _, err := inspectEmptyHistory(v); err != nil {
				t.Fatalf("fixture must pass transport/reference validation: %v", err)
			}
			if ValidateEmptyHistory(v) == nil {
				t.Fatal("structurally valid rich catalog bypassed subset boundary")
			}
		})
	}
}
