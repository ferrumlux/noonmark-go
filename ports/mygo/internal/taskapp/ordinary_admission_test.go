// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func ordinaryAdmissionBytes(t *testing.T, s canonical.Value) []byte {
	t.Helper()
	b, e := canonical.Marshal(historyObject(map[string]canonical.Value{"formatVersion": historyNumber(7), "snapshot": s}))
	must(t, e)
	return b
}
func TestOrdinaryAdmissionEmptyPositiveAndCaptureCompleteness(t *testing.T) {
	empty, e := initialSnapshot()
	must(t, e)
	if _, e = admitOrdinaryAggregate(ordinaryAdmissionBytes(t, empty), canonical.Limits{}); e != nil {
		t.Fatal(e)
	}
	prior, before, after := historyFixture(t)
	full, e := CaptureEmptyHistory(prior, before, after)
	must(t, e)
	raw := ordinaryAdmissionBytes(t, full)
	a, e := admitOrdinaryAggregate(raw, canonical.Limits{})
	must(t, e)
	out, e := v7.EncodeOrdinaryAggregate(a.document)
	must(t, e)
	if !bytes.Equal(raw, out) {
		t.Fatal("raw history changed")
	}
	// Source valid all-empty state on a settled graph is insufficient. The
	// runtime profile must run even without classification audit facts.
	full.Object["classifications"] = empty.Object["classifications"]
	missing := ordinaryAdmissionBytes(t, full)
	if _, e = v7.DecodeOrdinaryAggregate(missing, canonical.Limits{}); e != nil {
		t.Fatal("source-state control", e)
	}
	if _, e = admitOrdinaryAggregate(missing, canonical.Limits{}); e == nil {
		t.Fatal("all-empty capture omission accepted")
	}
}
func TestOrdinaryAdmissionCaptureClockAndCopies(t *testing.T) {
	prior, before, after := historyFixture(t)
	s, e := CaptureEmptyHistory(prior, before, after)
	must(t, e)
	a, e := admitOrdinaryAggregate(ordinaryAdmissionBytes(t, s), canonical.Limits{})
	must(t, e)
	view, e := a.document.Snapshot()
	must(t, e)
	historyMutateEvent(view, 0, "capturedAt", historyNumber(0))
	if _, e = admitOrdinaryAggregate(ordinaryAdmissionBytes(t, view), canonical.Limits{}); e == nil {
		t.Fatal("bad capture accepted")
	}
	raw, e := v7.EncodeOrdinaryAggregate(a.document)
	must(t, e)
	if _, e = admitOrdinaryAggregate(raw, canonical.Limits{}); e != nil {
		t.Fatal("copy mutation changed admitted document", e)
	}
}

func TestOrdinaryAdmissionNonemptyCapturedValue(t *testing.T) {
	prior, before, after := historyFixture(t)
	captured, e := CaptureEmptyHistory(prior, before, after)
	must(t, e)
	d, e := v7.DecodeOrdinaryAggregate(ordinaryAdmissionBytes(t, captured), canonical.Limits{})
	must(t, e)
	g, a, e := d.Facts()
	must(t, e)
	ctx, e := ordinary.NewEmptyContext(g)
	must(t, e)
	id := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
	req := ordinary.Request{ChainID: g.Chains[0].ID, Choice: ordinary.Choice{Kind: ordinary.ChoiceNew, Name: "原值 e\u0301", ColorHex: "#123456"}, InteractionID: ordinary.UUID(id(900))}
	p, e := ordinary.Prepare(ctx, ordinary.EmptyState(), req, ordinary.PrepareIDs{PlanID: ordinary.UUID(id(901)), CategoryID: ordinary.CategoryID(id(902))})
	must(t, e)
	confirm, e := ordinary.ConfirmUser(p, req.InteractionID)
	must(t, e)
	now, e := core.InstantFromSeconds(4)
	must(t, e)
	state, _, e := ordinary.Apply(ctx, ordinary.EmptyState(), p, confirm, now, ordinary.CommitIDs{RecordID: ordinary.UUID(id(903)), NameVersionID: ordinary.UUID(id(904))})
	must(t, e)
	converted, e := ordinary.FromOrdinaryState(state, ordinary.Limits{})
	must(t, e)
	converted.SnapshotsByTraceID = a.SnapshotsByTraceID
	converted.SnapshotEventsByTraceID = a.SnapshotEventsByTraceID
	a = converted
	a.Revision = 3
	for trace, events := range a.SnapshotEventsByTraceID {
		event := events[0]
		event.Revision++
		if trace == g.Traces[0].ID {
			cat := state.Categories[ordinary.CategoryID(id(902))]
			event.Category = &ordinary.HistoricalCategoryValue{ID: cat.ID, Name: cat.Name, ColorHex: cat.ColorHex, PresentationApproval: cat.PresentationApproval}
		}
		a.SnapshotEventsByTraceID[trace] = []ordinary.TraceClassificationSnapshot{event}
		a.SnapshotsByTraceID[trace] = event
	}
	rich, e := v7.WithOrdinaryFacts(d, g, a)
	must(t, e)
	raw, e := v7.EncodeOrdinaryAggregate(rich)
	must(t, e)
	admitted, e := admitOrdinaryAggregate(raw, canonical.Limits{})
	must(t, e)
	_, facts, e := admitted.document.Facts()
	must(t, e)
	if len(facts.Categories) != 1 || facts.Revision != 3 {
		t.Fatal("lost category/capture revision")
	}
	if _, e = decodeSnapshot(raw); e == nil {
		t.Fatal("legacy empty gate opened")
	}
	for _, scope := range []string{"archived", "pending-review", "multi-event"} {
		t.Run(scope, func(t *testing.T) {
			view, e := rich.Snapshot()
			must(t, e)
			if scope == "archived" {
				view.Object["classifications"].Object["categories"].Array[1].Object["lifecycle"] = historyString("archived")
			} else if scope == "pending-review" {
				view.Object["classifications"].Object["categories"].Array[1].Object["presentationApproval"] = historyString("pendingAIReview")
			} else {
				g, f, e := rich.Facts()
				must(t, e)
				for trace, stream := range f.SnapshotEventsByTraceID {
					event := stream[0]
					event.ID = ordinary.UUID(id(999))
					event.Revision = 4
					f.Revision = 4
					f.SnapshotEventsByTraceID[trace] = append(stream, event)
					f.SnapshotsByTraceID[trace] = event
					break
				}
				multi, e := v7.WithOrdinaryFacts(rich, g, f)
				must(t, e)
				view, e = multi.Snapshot()
				must(t, e)
			}
			candidate := ordinaryAdmissionBytes(t, view)
			if _, e = v7.DecodeOrdinaryAggregate(candidate, canonical.Limits{}); e != nil {
				t.Fatal("source fixture", e)
			}
			if _, e = admitOrdinaryAggregate(candidate, canonical.Limits{}); !errors.Is(e, ordinary.ErrUnsupported) {
				t.Fatal("runtime profile boundary", e)
			}
		})
	}
}
