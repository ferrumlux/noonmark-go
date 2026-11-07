// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Synthetic fixtures, not captured Swift output or interoperability evidence.
func aggregateHistoryFixture(t testing.TB) canonical.Value {
	t.Helper()
	s := ordinarySnapshot(t)
	g, _, err := ordinaryAggregateProjection(s)
	if err != nil {
		t.Fatal(err)
	}
	k, err := core.NewTaskKernel(g)
	if err != nil {
		t.Fatal(err)
	}
	day := core.LocalDate{Year: 2026, Month: 11, Day: 1}
	next, _ := day.AddDays(1)
	err = k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(testID(1)), TraceID: core.DayTraceID(testID(700)), DayID: core.DayID(testID(701)), Date: day, Today: day, Now: testTime(1)})
	if err != nil {
		t.Fatal(err)
	}
	if err = k.SettleTaskDays(next, testTime(2)); err != nil {
		t.Fatal(err)
	}
	g = k.Snapshot()
	for _, f := range []struct {
		key, typ string
		v        any
	}{{"days", "[]Day", g.Days}, {"traces", "[]Trace", g.Traces}} {
		v, e := ordinaryEncode(reflect.ValueOf(f.v), f.typ)
		if e != nil {
			t.Fatal(e)
		}
		s.Object[f.key] = v
	}
	c := s.Object["classifications"]
	cat := c.Object["categories"].Array[1]
	historical := objectValue(map[string]canonical.Value{})
	for _, key := range []string{"id", "name", "colorHex", "presentationApproval"} {
		historical.Object[key] = clone(cat.Object[key])
	}
	event := objectValue(map[string]canonical.Value{"id": stringValue(testID(702)), "traceID": objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(700))}), "status": stringValue("unfinished"), "category": historical, "labels": ordinaryArray(), "capturedAt": numberValue("4611686018427387904"), "revision": numberValue("9")})
	key := objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(700))})
	c.Object["snapshotsByTraceID"] = ordinaryArray(key, clone(event))
	c.Object["snapshotEventsByTraceID"] = ordinaryArray(key, ordinaryArray(event))
	c.Object["revision"] = numberValue("9")
	return s
}

func TestOrdinaryAggregateLosslessAndIndependent(t *testing.T) {
	for _, history := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "history"}[history], func(t *testing.T) {
			s := ordinarySnapshot(t)
			if history {
				s = aggregateHistoryFixture(t)
			}
			// Raw UUID spelling and negative zero must survive even though facts normalize IDs.
			id := s.Object["definitions"].Array[0].Object["id"].Object["rawValue"]
			id.String = strings.ToLower(id.String)
			s.Object["definitions"].Array[0].Object["id"].Object["rawValue"] = id
			raw := ordinaryEnvelope(t, s)
			d, err := DecodeOrdinaryAggregate(raw, canonical.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			out, err := EncodeOrdinaryAggregate(d)
			if err != nil || !bytes.Equal(raw, out) {
				t.Fatalf("lossless %v", err)
			}
			g, a, err := d.Facts()
			if err != nil {
				t.Fatal(err)
			}
			if uint64(g.Definitions[0].CreatedAt) != math.Float64bits(math.Copysign(0, -1)) {
				t.Fatal("raw Double bits changed")
			}
			g.Definitions[0].Title = "polluted"
			for id, c := range a.Categories {
				c.Name = "polluted"
				a.Categories[id] = c
			}
			for _, events := range a.SnapshotEventsByTraceID {
				events[0].Category.Name = "polluted"
			}
			view, err := d.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			view.Object["chains"] = ordinaryArray()
			out, err = EncodeOrdinaryAggregate(d)
			if err != nil || !bytes.Equal(raw, out) {
				t.Fatal("read copy poisoned document", err)
			}
			if _, err = New(Options{}).Decode(raw); !errors.Is(err, ErrDomainValidationRequired) {
				t.Fatal("default gate changed", err)
			}
		})
	}
}

func TestOrdinaryAggregateRejectsUnsupportedUnknownAndCorrupt(t *testing.T) {
	mutations := map[string]func(canonical.Value){
		"unknown snapshot": func(s canonical.Value) { s.Object["future"] = stringValue("preserve me") },
		"unknown capture": func(s canonical.Value) {
			s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["future"] = stringValue("preserve me")
		},
		"bad revision":   func(s canonical.Value) { s.Object["classifications"].Object["revision"] = numberValue("10") },
		"missing latest": func(s canonical.Value) { s.Object["classifications"].Object["snapshotsByTraceID"] = ordinaryArray() },
		"receipt digest": func(s canonical.Value) {
			s.Object["classifications"].Object["committedReceiptsByInteractionID"].Array[1].Object["changeRecordIntegrityDigest"] = stringValue(strings.Repeat("0", 64))
		},
		"nonneutral": func(s canonical.Value) {
			s.Object["preferences"].Object["themeLanguageWriterID"] = stringValue("other")
		},
		"labels": func(s canonical.Value) {
			e := s.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0]
			e.Object["labels"] = ordinaryArray(objectValue(map[string]canonical.Value{"id": objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(800))}), "name": stringValue("label"), "colorHex": stringValue("#123456")}))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s := aggregateHistoryFixture(t)
			mutate(s)
			if _, err := DecodeOrdinaryAggregate(ordinaryEnvelope(t, s), canonical.Limits{}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestOrdinaryAggregateBounds(t *testing.T) {
	s := aggregateHistoryFixture(t)
	raw := ordinaryEnvelope(t, s)
	for _, l := range []canonical.Limits{{MaxBytes: len(raw)}, {MaxNodes: 20}, {MaxDepth: 2}, {MaxBytes: -1}} {
		if _, err := DecodeOrdinaryAggregate(raw, l); err == nil {
			t.Fatal("budget accepted", l)
		}
	}
	cyclic := objectValue(map[string]canonical.Value{})
	cyclic.Object["loop"] = cyclic
	if _, err := ImportOrdinaryAggregate(cyclic, canonical.Limits{}); err == nil {
		t.Fatal("cycle accepted")
	}
	if _, err := EncodeOrdinaryAggregate(OrdinaryAggregate{}); !errors.Is(err, ordinary.ErrInvalid) {
		t.Fatal("zero authority", err)
	}
	// Shared deterministic admission budget: a successfully encoded document must
	// decode with the exact same limits. This is payload-level, not session proof.
	var good canonical.Limits
	for n := len(raw); n <= 16<<20; n *= 2 {
		l := canonical.Limits{MaxBytes: n}
		d, e := DecodeOrdinaryAggregate(raw, l)
		if e != nil {
			continue
		}
		out, e := EncodeOrdinaryAggregate(d)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = DecodeOrdinaryAggregate(out, l); e != nil {
			t.Fatal(e)
		}
		good = l
		break
	}
	if good.MaxBytes == 0 {
		t.Fatal("no accepted budget")
	}
}

func FuzzOrdinaryAggregate(f *testing.F) {
	f.Add(ordinaryEnvelope(f, aggregateHistoryFixture(f)))
	f.Add([]byte("{}"))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		d, err := DecodeOrdinaryAggregate(raw, canonical.Limits{MaxBytes: 1 << 20, MaxNodes: 10000})
		if err != nil {
			return
		}
		out, err := EncodeOrdinaryAggregate(d)
		if err != nil || !bytes.Equal(raw, out) {
			t.Fatal("lossless contract", err)
		}
	})
}

func TestOrdinaryAggregateOverlayPreservesRawUntouchedFields(t *testing.T) {
	s := aggregateHistoryFixture(t)
	c := s.Object["classifications"]
	cats := c.Object["categories"].Array
	cats[0], cats[1], cats[2], cats[3] = cats[2], cats[3], cats[0], cats[1]
	// Preserve raw pair order and lowercased identity wrappers, not normalized output.
	for _, p := range []canonical.Value{cats[0], cats[1].Object["id"]} {
		v := p.Object["rawValue"]
		v.String = strings.ToLower(v.String)
		p.Object["rawValue"] = v
	}
	d, e := DecodeOrdinaryAggregate(ordinaryEnvelope(t, s), canonical.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	g, a, e := d.Facts()
	if e != nil {
		t.Fatal(e)
	}
	unchanged, e := WithOrdinaryFacts(d, g, a)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := EncodeOrdinaryAggregate(unchanged)
	if e != nil || !bytes.Equal(raw, ordinaryEnvelope(t, s)) {
		t.Fatal("no-op rewrite", e)
	}
	g.Definitions[0].Title = "updated synthetic title"
	changed, e := WithOrdinaryFacts(d, g, a)
	if e != nil {
		t.Fatal(e)
	}
	wire, e := changed.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	expected := clone(s)
	expected.Object["definitions"].Array[0].Object["title"] = stringValue(g.Definitions[0].Title)
	if !bytes.Equal(ordinaryEnvelope(t, wire), ordinaryEnvelope(t, expected)) {
		t.Fatal("title touched unrelated raw fields")
	}
	// Historical streams are source-valid independently of the current category;
	// adding a second event must preserve the old event exactly (runtime narrower).
	for trace, events := range a.SnapshotEventsByTraceID {
		event := events[0]
		event.ID = ordinary.UUID(testID(703))
		event.CapturedAt = testTime(3)
		event.Revision = 10
		a.SnapshotEventsByTraceID[trace] = append(events, event)
		a.SnapshotsByTraceID[trace] = event
		a.Revision = 10
	}
	added, e := WithOrdinaryFacts(changed, g, a)
	if e != nil {
		t.Fatal(e)
	}
	w, e := added.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	old := c.Object["snapshotEventsByTraceID"].Array[1].Array[0]
	preserved := w.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0]
	if !reflect.DeepEqual(old, preserved) {
		t.Fatal("append rewrote old capture")
	}
	g.Definitions[0].Title = strings.Repeat("x", 1<<20)
	if _, e = WithOrdinaryFacts(d, g, a); !errors.Is(e, ordinary.ErrResourceLimit) {
		t.Fatal("pre-encoding bound", e)
	}
}

func TestOrdinaryAggregateRawPreviewStringDigest(t *testing.T) {
	for _, mode := range []string{"absent", "present-empty", "lowercase"} {
		t.Run(mode, func(t *testing.T) {
			s := ordinarySnapshot(t)
			c := s.Object["classifications"]
			_, oldState, _ := ordinaryFixture(t)
			r := oldState.ChangeRecords[0]
			payload, e := ordinary.CandidateRecordDigestPayload(r)
			if e != nil {
				t.Fatal(e)
			}
			original := string(r.Changes[0].After.Category.ID)
			rawID := strings.ToLower(original)
			if original == rawID {
				t.Fatal("fixture needs alphabetic UUID digits")
			}
			record := c.Object["changeRecords"].Array[0]
			item := record.Object["changes"].Array[0].Object["setCurrent"].Object["after"].Object["category"]
			switch mode {
			case "absent":
				delete(item.Object, "id")
				payload = bytes.ReplaceAll(payload, []byte(`,"id":"`+original+`"`), nil)
			case "present-empty":
				rawID = ""
				fallthrough
			case "lowercase":
				item.Object["id"] = stringValue(rawID)
				payload = bytes.ReplaceAll(payload, []byte(original), []byte(rawID))
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(payload))
			record.Object["integrityDigest"] = stringValue(digest)
			receipts := c.Object["committedReceiptsByInteractionID"].Array
			for i := 0; i < len(receipts); i += 2 {
				if receipts[i].String == string(r.InteractionID) {
					receipts[i+1].Object["changeRecordIntegrityDigest"] = stringValue(digest)
				}
			}
			// Legal source admission is not whole-history replay. Neither current nor
			// removed relations claims authority from this synthetic historical record.
			c.Object["currentByChainID"] = ordinaryArray()
			c.Object["relationHistory"] = ordinaryArray()
			raw := ordinaryEnvelope(t, s)
			d, e := DecodeOrdinaryAggregate(raw, canonical.Limits{})
			if e != nil {
				t.Fatal(e)
			}
			g, facts, e := d.Facts()
			if e != nil {
				t.Fatal(e)
			}
			got := facts.ChangeRecords[0].Changes[0].After.Category.ID
			if mode == "absent" {
				if got != nil {
					t.Fatal("absent became present")
				}
			} else if got == nil || *got != rawID {
				t.Fatal("raw optional String changed")
			}
			encoded, e := EncodeOrdinaryAggregate(d)
			if e != nil || !bytes.Equal(encoded, raw) {
				t.Fatal("raw digest roundtrip", e)
			}
			same, e := WithOrdinaryFacts(d, g, facts)
			if e != nil {
				t.Fatal(e)
			}
			encoded, e = EncodeOrdinaryAggregate(same)
			if e != nil || !bytes.Equal(encoded, raw) {
				t.Fatal("overlay optional roundtrip", e)
			}
			// Changing optional presence or raw spelling without recomputing the record
			// digest/receipt must fail, even when those strings look like the same UUID.
			item.Object["id"] = stringValue(original)
			if _, e = DecodeOrdinaryAggregate(ordinaryEnvelope(t, s), canonical.Limits{}); e == nil {
				t.Fatal("optional/raw digest tamper accepted")
			}
		})
	}
}

func TestOrdinaryAggregateSourceBeyondR4Scope(t *testing.T) {
	for _, mode := range []string{"zero-changes", "multiple-changes", "historical-alias", "archived", "pending-review", "multiple-name-versions"} {
		t.Run(mode, func(t *testing.T) {
			s := ordinarySnapshot(t)
			c := s.Object["classifications"]
			if mode == "archived" {
				c.Object["categories"].Array[1].Object["lifecycle"] = stringValue("archived")
			} else if mode == "pending-review" {
				c.Object["categories"].Array[1].Object["presentationApproval"] = stringValue("pendingAIReview")
			} else if mode == "multiple-name-versions" {
				cat := c.Object["categories"].Array[1]
				first := clone(cat.Object["nameVersions"].Array[0])
				last := clone(first)
				first.Object["validUntil"] = numberValue("4607182418800017408")
				last.Object["validFrom"] = numberValue("4607182418800017408")
				last.Object["id"] = stringValue(testID(899))
				cat.Object["updatedAt"] = numberValue("4607182418800017408")
				cat.Object["nameVersions"] = ordinaryArray(first, last)
			} else {
				_, state, _ := ordinaryFixture(t)
				record := state.ChangeRecords[0]
				payload, e := ordinary.CandidateRecordDigestPayload(record)
				if e != nil {
					t.Fatal(e)
				}
				r := c.Object["changeRecords"].Array[0]
				receipt := canonical.Value{}
				pairs := c.Object["committedReceiptsByInteractionID"].Array
				for i := 0; i < len(pairs); i += 2 {
					if pairs[i].String == string(record.InteractionID) {
						receipt = pairs[i+1]
					}
				}
				switch mode {
				case "zero-changes":
					end := bytes.Index(payload, []byte(",\"committedAtBitPattern\":"))
					payload = append([]byte("{\"changes\":[]"), payload[end:]...)
					payload = bytes.ReplaceAll(payload, []byte(",\"revision\":1,"), []byte(",\"revision\":0,"))
					r.Object["changes"] = ordinaryArray()
					r.Object["revision"] = numberValue("0")
					receipt.Object["revision"] = numberValue("0")
					c.Object["revision"] = numberValue("0")
				case "multiple-changes":
					end := bytes.Index(payload, []byte(",\"committedAtBitPattern\":"))
					change := payload[len("{\"changes\":[") : end-1]
					prefix := append([]byte("{\"changes\":["), change...)
					prefix = append(prefix, ',')
					prefix = append(prefix, change...)
					prefix = append(prefix, ']')
					payload = append(prefix, payload[end:]...)
					changeWire := r.Object["changes"].Array[0]
					r.Object["changes"] = ordinaryArray(changeWire, clone(changeWire))
					c.Object["revision"] = numberValue("1")
				case "historical-alias":
					payload = bytes.ReplaceAll(payload, []byte("\"resolution\":\"new\""), []byte("\"resolution\":\"historicalAlias\""))
					r.Object["changes"].Array[0].Object["setCurrent"].Object["after"].Object["category"].Object["resolution"] = stringValue("historicalAlias")
					noticeText := "[{\"existingItemReused\":{\"currentName\":\"x\",\"inputName\":\"x\",\"kind\":\"category\",\"matchedHistoricalAlias\":true}}]"
					payload = bytes.ReplaceAll(payload, []byte("\"notices\":[]"), []byte("\"notices\":"+noticeText))
					notice, e := canonical.Parse([]byte(noticeText), canonical.Limits{})
					if e != nil {
						t.Fatal(e)
					}
					r.Object["notices"] = notice
					receipt.Object["notices"] = clone(notice)
					c.Object["revision"] = numberValue("1")
				}
				digest := fmt.Sprintf("%x", sha256.Sum256(payload))
				r.Object["integrityDigest"] = stringValue(digest)
				receipt.Object["changeRecordIntegrityDigest"] = stringValue(digest)
				c.Object["changeRecords"] = ordinaryArray(r)
				c.Object["committedReceiptsByInteractionID"] = ordinaryArray(stringValue(string(record.InteractionID)), receipt)
				c.Object["currentByChainID"] = ordinaryArray()
				c.Object["relationHistory"] = ordinaryArray()
			}
			raw := ordinaryEnvelope(t, s)
			if _, e := DecodeOrdinaryCategoryCandidate(raw, canonical.Limits{}); e == nil {
				t.Fatal("R4 unexpectedly accepted broader source fixture")
			}
			d, e := DecodeOrdinaryAggregate(raw, canonical.Limits{})
			if e != nil {
				t.Fatal("new source rejected", e)
			}
			g, a, e := d.Facts()
			if e != nil {
				t.Fatal(e)
			}
			same, e := WithOrdinaryFacts(d, g, a)
			if e != nil {
				t.Fatal(e)
			}
			out, e := EncodeOrdinaryAggregate(same)
			if e != nil || !bytes.Equal(out, raw) {
				t.Fatal("source fields lost", e)
			}
		})
	}
}

func FuzzOrdinaryAggregateFields(f *testing.F) {
	base := aggregateHistoryFixture(f)
	f.Add(uint8(0), uint64(9), "x")
	f.Add(uint8(1), uint64(0), "e\u0301")
	f.Add(uint8(2), uint64(1), "")
	f.Fuzz(func(t *testing.T, choice uint8, n uint64, text string) {
		if len(text) > 4096 {
			return
		}
		s := clone(base)
		c := s.Object["classifications"]
		switch choice % 4 {
		case 0:
			c.Object["revision"] = numberValue(fmt.Sprintf("%d", n))
		case 1:
			c.Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["category"].Object["name"] = stringValue(text)
		case 2:
			c.Object["changeRecords"].Array[0].Object["changes"].Array[0].Object["setCurrent"].Object["after"].Object["category"].Object["id"] = stringValue(text)
		case 3:
			s.Object["unknown"] = stringValue(text)
		}
		raw, e := canonical.Marshal(objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": s}))
		if e != nil {
			return
		}
		d, e := DecodeOrdinaryAggregate(raw, canonical.Limits{MaxBytes: 1 << 20, MaxNodes: 10000})
		if choice%4 == 3 && e == nil {
			t.Fatal("unknown accepted")
		}
		if e != nil {
			return
		}
		out, e := EncodeOrdinaryAggregate(d)
		if e != nil || !bytes.Equal(raw, out) {
			t.Fatal("accepted mutation not lossless", e)
		}
	})
}

// These focused overlay unit cases exercise every required-ID array schema.
// The minimal objects are not claimed to be admitted whole source snapshots.
func TestOrdinaryAggregateEntityArrayIdentityOverlay(t *testing.T) {
	types := []string{"Day", "Chain", "Definition", "Trace", "Subtask", "Note", "PlannedSubtask", "NameVersion", "RelationHistory", "ChangeRecord", "TraceSnapshot"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			entity := func(n int, lower bool) canonical.Value {
				id := fmt.Sprintf("ABCDEF01-ABCD-4ABC-8ABC-%012X", n)
				if lower {
					id = strings.ToLower(id)
				}
				v := stringValue(id)
				if objects[typ]["id"] == "ID" {
					v = objectValue(map[string]canonical.Value{"rawValue": v})
				}
				return objectValue(map[string]canonical.Value{"id": v})
			}
			before := ordinaryArray(entity(1, false), entity(2, false), entity(3, false))
			raw := ordinaryArray(entity(1, true), entity(2, true), entity(3, true))
			for _, order := range [][]int{{3, 1, 2}, {2, 3}, {4, 3, 1, 2}, {2, 4, 1}} {
				after, want := ordinaryArray(), ordinaryArray()
				for _, n := range order {
					after.Array = append(after.Array, entity(n, false))
					want.Array = append(want.Array, entity(n, n <= 3))
				}
				got := ordinaryAggregateOverlay(raw, before, after, "[]"+typ)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("identity raw mismatch order %v", order)
				}
			}
		})
	}
}

func TestOrdinaryAggregateReorderInsertDeleteRetainsRaw(t *testing.T) {
	s := ordinarySnapshot(t)
	for i := range s.Object["definitions"].Array {
		v := s.Object["definitions"].Array[i].Object["id"].Object["rawValue"]
		v.String = strings.ToLower(v.String)
		s.Object["definitions"].Array[i].Object["id"].Object["rawValue"] = v
	}
	d, e := ImportOrdinaryAggregate(s, canonical.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	g, a, e := d.Facts()
	if e != nil {
		t.Fatal(e)
	}
	g.Definitions[0], g.Definitions[1] = g.Definitions[1], g.Definitions[0]
	want := clone(s)
	defs := want.Object["definitions"].Array
	defs[0], defs[1] = defs[1], defs[0]
	next, e := WithOrdinaryFacts(d, g, a)
	if e != nil {
		t.Fatal(e)
	}
	got, e := EncodeOrdinaryAggregate(next)
	if e != nil || !bytes.Equal(got, ordinaryEnvelope(t, want)) {
		t.Fatal("reordering lost raw identity", e)
	}
	// Changed content follows the same moved identity, not its old ordinal.
	g.Definitions[0].Title = "moved and edited"
	want.Object["definitions"].Array[0].Object["title"] = stringValue("moved and edited")
	next, e = WithOrdinaryFacts(d, g, a)
	if e != nil {
		t.Fatal(e)
	}
	got, e = EncodeOrdinaryAggregate(next)
	if e != nil || !bytes.Equal(got, ordinaryEnvelope(t, want)) {
		t.Fatal("moved edit rewrote untouched fields", e)
	}
	// Add a new unrelated pool task, then move it before the old entities.
	k, e := core.NewTaskKernel(g)
	if e != nil {
		t.Fatal(e)
	}
	e = k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(testID(950)), DefinitionID: core.TaskDefinitionID(testID(951)), Title: "new synthetic", Now: testTime(4)})
	if e != nil {
		t.Fatal(e)
	}
	g = k.Snapshot()
	newDef := g.Definitions[2]
	g.Definitions = append([]core.TaskDefinition{newDef}, g.Definitions[:2]...)
	inserted, e := WithOrdinaryFacts(next, g, a)
	if e != nil {
		t.Fatal(e)
	}
	wire, e := inserted.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(wire.Object["definitions"].Array[1:], want.Object["definitions"].Array) {
		t.Fatal("insertion shifted raw identities")
	}
	// Removing only that unrelated task must restore the exact previous wire.
	g.Definitions = g.Definitions[1:]
	g.Chains = g.Chains[:2]
	deleted, e := WithOrdinaryFacts(inserted, g, a)
	if e != nil {
		t.Fatal(e)
	}
	got, e = EncodeOrdinaryAggregate(deleted)
	if e != nil || !bytes.Equal(got, ordinaryEnvelope(t, want)) {
		t.Fatal("deletion shifted raw identities", e)
	}
	// Repeated identities remain rejected by the existing complete validators.
	g.Definitions = append(g.Definitions, g.Definitions[0])
	if _, e = WithOrdinaryFacts(deleted, g, a); e == nil {
		t.Fatal("duplicate identity accepted")
	}
}

func TestOrdinaryAggregateAnonymousSequenceFollowsExactValues(t *testing.T) {
	change := func(n int, lower bool) canonical.Value {
		id := fmt.Sprintf("ABCDEF01-ABCD-4ABC-8ABC-%012X", n)
		if lower {
			id = strings.ToLower(id)
		}
		return tagged("setCurrent", map[string]canonical.Value{"chainID": objectValue(map[string]canonical.Value{"rawValue": stringValue(id)}), "before": objectValue(map[string]canonical.Value{"labels": ordinaryArray()}), "after": objectValue(map[string]canonical.Value{"labels": ordinaryArray()})})
	}
	before := ordinaryArray(change(1, false), change(2, false), change(1, false))
	raw := ordinaryArray(change(1, true), change(2, true), change(1, false))
	after := ordinaryArray(change(2, false), change(3, false), change(1, false), change(1, false))
	want := ordinaryArray(change(2, true), change(3, false), change(1, true), change(1, false))
	if got := ordinaryAggregateOverlay(raw, before, after, "[]PlanChange"); !reflect.DeepEqual(got, want) {
		t.Fatal("anonymous reorder/insertion stole another operation's raw fields")
	}
}

func FuzzOrdinaryAggregateEntityMoves(f *testing.F) {
	base := ordinarySnapshot(f)
	rawIDs := map[string]canonical.Value{}
	for _, key := range []string{"chains", "definitions"} {
		for i := range base.Object[key].Array {
			v := base.Object[key].Array[i].Object["id"].Object["rawValue"]
			v.String = strings.ToLower(v.String)
			base.Object[key].Array[i].Object["id"].Object["rawValue"] = v
			rawIDs[strings.ToUpper(v.String)] = clone(base.Object[key].Array[i].Object["id"])
		}
	}
	d, e := ImportOrdinaryAggregate(base, canonical.Limits{})
	if e != nil {
		f.Fatal(e)
	}
	f.Add(uint8(0), "updated")
	f.Add(uint8(7), "é")
	f.Fuzz(func(t *testing.T, mode uint8, title string) {
		if len(title) > 256 {
			return
		}
		g, a, e := d.Facts()
		if e != nil {
			t.Fatal(e)
		}
		if mode&1 != 0 {
			g.Definitions[0], g.Definitions[1] = g.Definitions[1], g.Definitions[0]
		}
		if mode&2 != 0 {
			g.Chains[0], g.Chains[1] = g.Chains[1], g.Chains[0]
		}
		if mode&4 != 0 {
			g.Definitions[0].Title = title
		}
		next, e := WithOrdinaryFacts(d, g, a)
		if e != nil {
			return
		}
		wire, e := next.Snapshot()
		if e != nil {
			t.Fatal(e)
		}
		for _, key := range []string{"chains", "definitions"} {
			for _, entity := range wire.Object[key].Array {
				id := wireID(entity.Object["id"])
				if !reflect.DeepEqual(entity.Object["id"], rawIDs[id]) {
					t.Fatal("move/edit lost original entity spelling")
				}
			}
		}
	})
}
