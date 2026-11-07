// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestFoundationCandidatePayloadEscapesAndOmission(t *testing.T) {
	// These assertions are Go candidate unit tests, not Swift fixtures/capture.
	f := newFixture(t, 1)
	p, _, _ := f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "中文/E\u0301\\\"\x00\b\f\t\r\n\u2028\u2029X", ColorHex: "#D87831"}, instant(0.0000001))
	data, e := CandidatePlanDigestPayload(p.sealed.data)
	if e != nil {
		t.Fatal(e)
	}
	if digest(data) != p.sealed.data.Digest {
		t.Fatal("plan digest")
	}
	if bytes.Contains(data, []byte(`createdItemID`)) || bytes.Contains(data, []byte(`intent`)) || !bytes.Contains(data, []byte(`"plannedCategoryID":{"rawValue":`)) || !bytes.Contains(data, []byte(`"plannedLabelIDs":[]`)) || !bytes.Contains(data, []byte(`"source":{"userDirect":{}}`)) {
		t.Fatalf("payload shape: %s", data)
	}
	var w foundationWriter
	w.str("中文/\\\"\x00\b\f\t\r\n\u2028\u2029<>&")
	if got, want := w.String(), "\"中文\\/\\\\\\\"\\u0000\\b\\f\\t\\r\\n\u2028\u2029<>&\""; got != want {
		t.Fatalf("%q != %q", got, want)
	}
	var decoded string
	if e = json.Unmarshal([]byte(w.String()), &decoded); e != nil || decoded != "中文/\\\"\x00\b\f\t\r\n\u2028\u2029<>&" {
		t.Fatal(e)
	}
	none := newFixture(t, 1)
	plan, _, _ := none.commit(t, 1, Choice{Kind: ChoiceNone}, instant(0))
	payload, e := CandidatePlanDigestPayload(plan.sealed.data)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(payload, []byte("plannedCategoryID")) || bytes.Contains(payload, []byte(`"category"`)) {
		t.Fatal("optional none encoded")
	}
	mutated := clonePlan(p.sealed.data)
	mutated.Intent.Choice.Name = "different retained intent"
	other, e := CandidatePlanDigestPayload(mutated)
	if e != nil || !bytes.Equal(data, other) {
		t.Fatal("intent unexpectedly digested", e)
	}
}
func TestRecordPayloadUsesUInt64DateBits(t *testing.T) {
	f := newFixture(t, 1)
	f.commit(t, 1, Choice{Kind: ChoiceNone}, instant(0))
	r := f.s.ChangeRecords[0]
	for _, n := range []float64{math.Copysign(0, -1), 0, 0.0000001, -0.0000001} {
		r.CommittedAt = instant(n)
		payload, e := CandidateRecordDigestPayload(r)
		if e != nil {
			t.Fatal(e)
		}
		expected := `"committedAtBitPattern":` + strconv.FormatUint(math.Float64bits(n), 10)
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("date bits %s", payload)
		}
		if strings.Contains(string(payload), `"committedAt":`) {
			t.Fatal("Date string/float field")
		}
	}
}
