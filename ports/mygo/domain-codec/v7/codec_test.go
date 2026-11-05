// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func testID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func testTime(v float64) core.Instant {
	x, e := core.InstantFromSeconds(v)
	if e != nil {
		panic(e)
	}
	return x
}
func testDate(v string) core.LocalDate {
	x, e := core.ParseLocalDate(v)
	if e != nil {
		panic(e)
	}
	return x
}
func parse(t *testing.T, s string) canonical.Value {
	t.Helper()
	v, e := canonical.Parse([]byte(s), canonical.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func extras(t *testing.T) Supplement {
	t.Helper()
	return Supplement{
		Preferences:     parse(t, `{"theme":"coolGray","language":"chinese","themeLanguageUpdatedAt":0,"themeLanguageWriterID":"synthetic.fixture","dataMode":"localFirst","backupPolicy":{"frequency":"off","destination":"iCloudDrive"},"localFirstSyncPolicy":{"enabled":false,"endpoint":"localFolder","mode":"manual","intervalSeconds":300,"generateConflictCopy":true,"snapshotRetention":{"indexRetentionDays":30,"retentionIndexesDaily":1}},"settingsPoemDisplayPolicy":{"enabled":false,"text":"合成测试 / no network"},"syncEndpointOptions":[]}`),
		Classifications: parse(t, `{"formatVersion":1,"revision":0,"categories":[],"labels":[],"currentByChainID":[],"relationHistory":[],"categoryMerges":[],"labelMerges":[],"categoryDeletionTombstones":[],"labelDeletionTombstones":[],"snapshotsByTraceID":[],"snapshotEventsByTraceID":[],"committedReceiptsByInteractionID":[],"changeRecords":[]}`),
		Ideas:           canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}},
	}
}
func fixture(t *testing.T) (canonical.Value, recurrence.State) {
	t.Helper()
	seq := 0
	ids := func() (string, error) { seq++; return testID(seq), nil }
	planned := []core.PlannedSubtask{{ID: core.PlannedSubtaskID(testID(900)), LineageID: core.SubtaskLineageID(testID(901)), Title: "整理结论", Difficulty: core.DifficultyMedium, Position: 1, CreatedAt: testTime(100)}}
	state, series, e := (recurrence.State{}).Create(recurrence.CreateInput{Title: "候选往返 / synthetic", PlannedSubtasks: planned, StartDate: testDate("2026-07-20"), Schedule: recurrence.Daily(), EndCondition: recurrence.Until(testDate("2026-07-24")), Today: testDate("2026-07-20"), Now: testTime(100)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	state, e = state.Skip(series, testDate("2026-07-21"), testDate("2026-07-20"), testTime(math.Nextafter(100, math.Inf(1))), ids)
	if e != nil {
		t.Fatal(e)
	}
	state, _, e = state.Revise(recurrence.ReviseInput{SeriesID: series, Schedule: recurrence.Schedule{Kind: recurrence.EveryDays, DayInterval: 2}, EndCondition: recurrence.EndCondition{Kind: recurrence.DurationDays, Days: 5}, Today: testDate("2026-07-20"), Now: testTime(100.0002)}, ids)
	if e != nil {
		t.Fatal(e)
	}
	state.Tasks.Chains[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(testID(950)), Body: "原始备注 😀", CreatedAt: testTime(100), UpdatedAt: testTime(100.0001)}}
	extra := extras(t)
	extra.Ideas = parse(t, fmt.Sprintf(`[{"id":{"rawValue":%q},"body":"一闪而过","labelIDs":[],"createdAt":%d,"updatedAt":%d,"pinnedAt":%d},{"id":{"rawValue":%q},"body":"","labelIDs":[],"createdAt":%d,"updatedAt":%d,"deletedAt":%d}]`, testID(960), testTime(100), testTime(100.0001), testTime(100.0001), testID(961), testTime(100), testTime(100.0002), testTime(100.0002)))
	snapshot, e := FromDomain(state, extra)
	if e != nil {
		t.Fatal(e)
	}
	return snapshot, state
}
func rawEnvelope(t *testing.T, s canonical.Value) []byte {
	t.Helper()
	b, e := canonical.Marshal(objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": s}))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestV7SyntheticRoundTripPreservesFactsAndExactDates(t *testing.T) {
	snapshot, state := fixture(t)
	codec := New(Options{})
	encoded, e := codec.Encode(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	document, e := codec.Decode(encoded)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(snapshot, document.Snapshot()) {
		t.Fatal("wire facts lost")
	}
	restored, extra, e := ToDomain(document.Snapshot())
	if e != nil {
		t.Fatal(e)
	}
	second, e := FromDomain(restored, extra)
	if e != nil {
		t.Fatal(e)
	}
	encodedAgain, e := codec.Encode(second)
	if e != nil || !bytes.Equal(encoded, encodedAgain) {
		t.Fatal("candidate bytes changed", e)
	}
	if len(restored.Series[0].PlanRevisions) != 2 || len(restored.Series[0].CancellationFacts) != 3 || restored.Tasks.Traces[1].DraftCancellationID == nil || len(restored.Tasks.Chains[0].NoteEntries) != 1 {
		t.Fatal("history missing")
	}
	for i, tr := range state.Tasks.Traces {
		if uint64(tr.ContentUpdatedAt) != uint64(restored.Tasks.Traces[i].ContentUpdatedAt) {
			t.Fatal("date bitPattern changed")
		}
	}
	cloned := document.Snapshot()
	delete(cloned.Object, "ideas")
	if _, ok := document.Snapshot().Object["ideas"]; !ok {
		t.Fatal("document leaked mutable state")
	}
	if strings.Contains(string(encoded), `"ID"`) || !strings.Contains(string(encoded), `"rawValue"`) {
		t.Fatal("wrong ID wire shape")
	}
}
func TestVersionIsCheckedBeforeSnapshotShape(t *testing.T) {
	for _, v := range []string{"1", "6", "8"} {
		_, e := New(Options{}).Decode([]byte(`{"formatVersion":` + v + `,"snapshot":false}`))
		if !errors.Is(e, ErrUnsupportedVersion) {
			t.Fatal(v, e)
		}
	}
	for _, data := range []string{`{}`, `{"formatVersion":"7"}`, `{"formatVersion":true}`, `{"formatVersion":7.1}`, `{"formatVersion":9223372036854775808}`} {
		if _, e := New(Options{}).Decode([]byte(data)); e == nil {
			t.Fatal("malformed version accepted")
		}
	}
}
func TestRequiredAndUnknownFieldsAreStrictAtEveryDepth(t *testing.T) {
	original, _ := fixture(t)
	codec := New(Options{})
	mutations := []func(canonical.Value){
		func(v canonical.Value) { delete(v.Object, "ideas") },
		func(v canonical.Value) { delete(v.Object, "classifications") },
		func(v canonical.Value) { v.Object["futureField"] = stringValue("ignored?") },
		func(v canonical.Value) { delete(v.Object["chains"].Array[0].Object, "noteEntries") },
		func(v canonical.Value) { v.Object["chains"].Array[0].Object["future"] = stringValue("ignored?") },
		func(v canonical.Value) { v.Object["chains"].Array[0].Object["id"] = stringValue(testID(1)) },
		func(v canonical.Value) {
			v.Object["traces"].Array[0].Object["completedAt"] = canonical.Value{Kind: canonical.Null}
		},
		func(v canonical.Value) {
			v.Object["taskCycleSeries"].Array[0].Object["schedule"] = tagged("everyDays", map[string]canonical.Value{"_0": numberValue("1"), "unused": numberValue("1")})
		},
	}
	for i, change := range mutations {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			v := clone(original)
			change(v)
			if _, e := codec.Decode(rawEnvelope(t, v)); e == nil {
				t.Fatal("invalid field shape accepted")
			}
		})
	}
}
func TestNoncanonicalAndDuplicateKeysRejected(t *testing.T) {
	snapshot, _ := fixture(t)
	codec := New(Options{})
	b, e := codec.Encode(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range [][]byte{append(append([]byte{}, b...), 10), []byte(strings.ReplaceAll(string(b), " : ", ":")), []byte(strings.Replace(string(b), `"formatVersion" : 7`, `"formatVersion" : 7, "formatVersion" : 7`, 1)), []byte(strings.Replace(string(b), `"rawValue" : `, `"rawValue" : "x", "rawValue" : `, 1))} {
		if _, e := codec.Decode(v); e == nil {
			t.Fatal("noncanonical bytes accepted")
		}
	}
}
func TestFiniteDatesRequireExactUnsignedBits(t *testing.T) {
	original, _ := fixture(t)
	codec := New(Options{})
	for _, bits := range []string{strconv.FormatUint(math.Float64bits(math.NaN()), 10), strconv.FormatUint(math.Float64bits(math.Inf(1)), 10), "-1", "18446744073709551616"} {
		v := clone(original)
		v.Object["days"].Array[0].Object["createdAt"] = numberValue(bits)
		if _, e := codec.Decode(rawEnvelope(t, v)); e == nil {
			t.Fatal("invalid timestamp accepted", bits)
		}
	}
	// A negative reference time has its sign in the UInt64 bits, not a negative JSON integer.
	v := clone(original)
	negativeZero := math.Float64bits(math.Copysign(0, -1))
	v.Object["preferences"].Object["themeLanguageUpdatedAt"] = numberValue(strconv.FormatUint(negativeZero, 10))
	encoded, e := codec.Encode(v)
	if e != nil {
		t.Fatal(e)
	}
	got, e := codec.Decode(encoded)
	if e != nil || got.Snapshot().Object["preferences"].Object["themeLanguageUpdatedAt"].TextNumber != strconv.FormatUint(negativeZero, 10) {
		t.Fatal("negative zero lost", e)
	}
}
func TestCoreReferencesAndCancellationWitnessAreChecked(t *testing.T) {
	original, _ := fixture(t)
	codec := New(Options{})
	changes := []func(canonical.Value){
		func(v canonical.Value) {
			v.Object["definitions"].Array[0].Object["chainID"] = objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(999))})
		},
		func(v canonical.Value) { delete(v.Object["traces"].Array[1].Object, "draftCancellationID") },
	}
	for _, change := range changes {
		v := clone(original)
		change(v)
		if _, e := codec.Decode(rawEnvelope(t, v)); e == nil {
			t.Fatal("broken graph accepted")
		}
	}
	v := clone(original)
	days := v.Object["days"]
	days.Array = append(days.Array, days.Array[0])
	v.Object["days"] = days
	if _, e := codec.Decode(rawEnvelope(t, v)); e == nil {
		t.Fatal("duplicate day accepted")
	}
}
func TestIdeasRejectDuplicateReferencesAndBadTombstones(t *testing.T) {
	original, _ := fixture(t)
	codec := New(Options{})
	changes := []func(canonical.Value){
		func(v canonical.Value) {
			v.Object["ideas"].Array[0].Object["categoryID"] = objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(999))})
		},
		func(v canonical.Value) {
			v.Object["ideas"].Array[1].Object["body"] = stringValue("erased tombstone still has body")
		},
		func(v canonical.Value) {
			v.Object["ideas"].Array[1].Object["pinnedAt"] = numberValue(strconv.FormatUint(uint64(testTime(100)), 10))
		},
		func(v canonical.Value) { v.Object["ideas"].Array[0].Object["body"] = stringValue(" untrimmed ") },
		func(v canonical.Value) {
			v.Object["ideas"].Array[1].Object["id"] = v.Object["ideas"].Array[0].Object["id"]
		},
	}
	for _, change := range changes {
		v := clone(original)
		change(v)
		if _, e := codec.Decode(rawEnvelope(t, v)); e == nil {
			t.Fatal("invalid idea accepted")
		}
	}
}
func TestClassificationAuditFailsClosedAndIsPreserved(t *testing.T) {
	original, _ := fixture(t)
	c := original.Object["classifications"]
	c.Object["revision"] = numberValue("1")
	original.Object["classifications"] = c
	b := rawEnvelope(t, original)
	codec := New(Options{})
	inspected, e := codec.Inspect(b)
	if e != nil || !inspected.RequiresClassificationAudit {
		t.Fatal(e)
	}
	if _, e = codec.Decode(b); !errors.Is(e, ErrDomainValidationRequired) {
		t.Fatal(e)
	}
	if _, e = codec.Encode(original); !errors.Is(e, ErrDomainValidationRequired) {
		t.Fatal(e)
	}
	sentinel := errors.New("synthetic audit denied")
	guarded := New(Options{ClassificationAuditValidator: func(snapshot canonical.Value) error { delete(snapshot.Object, "ideas"); return sentinel }})
	if _, e = guarded.Decode(b); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if _, ok := original.Object["ideas"]; !ok {
		t.Fatal("callback mutated caller state")
	}
}
func TestReadersAndVerifiedWritesAreBoundedAndHonest(t *testing.T) {
	snapshot, _ := fixture(t)
	codec := New(Options{})
	data, e := codec.Encode(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = codec.Read(bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	small := New(Options{Limits: canonical.Limits{MaxBytes: len(data) - 1}})
	if _, e = small.Read(bytes.NewReader(data)); e == nil {
		t.Fatal("over-limit reader accepted")
	}
	var buffer bytes.Buffer
	receipt, e := codec.WriteVerified(snapshot, &buffer, func() ([]byte, error) { return buffer.Bytes(), nil })
	if e != nil || receipt.ByteCount != len(data) || len(receipt.SHA256) != 64 {
		t.Fatal(receipt, e)
	}
	if _, e = codec.WriteVerified(snapshot, io.Discard, func() ([]byte, error) { return nil, nil }); !errors.Is(e, ErrWriteVerification) {
		t.Fatal(e)
	}
	if _, e = codec.WriteVerified(snapshot, shortWriter{}, func() ([]byte, error) { return data, nil }); !errors.Is(e, io.ErrShortWrite) {
		t.Fatal(e)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestProgrammaticCyclesFailBeforeCloning(t *testing.T) {
	cyc := objectValue(map[string]canonical.Value{})
	cyc.Object["cycle"] = cyc
	if _, e := New(Options{}).Encode(cyc); e == nil {
		t.Fatal("cycle accepted")
	}
	if _, _, e := ToDomain(cyc); e == nil {
		t.Fatal("cycle mapped")
	}
	extra := extras(t)
	extra.Preferences = cyc
	if _, e := FromDomain(recurrence.State{}, extra); e == nil {
		t.Fatal("cycle cloned")
	}
}
func FuzzV7DecoderDoesNotPanic(f *testing.F) {
	if seed, err := os.ReadFile("testdata/synthetic-v7-candidate.json"); err == nil {
		f.Add(seed)
	}
	f.Add([]byte(`{"formatVersion":6,"snapshot":null}`))
	f.Add([]byte(`{"formatVersion":7}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		codec := New(Options{Limits: canonical.Limits{MaxBytes: 65536, MaxDepth: 32, MaxNodes: 10000}})
		d, e := codec.Decode(data)
		if e == nil {
			encoded, e := codec.Encode(d.Snapshot())
			if e != nil || !bytes.Equal(encoded, data) {
				t.Fatal("accepted nonstable document", e)
			}
		}
	})
}

func TestUUIDWireCasingIsPreservedButDomainIDsNormalize(t *testing.T) {
	snapshot, _ := fixture(t)
	var lower func(canonical.Value) canonical.Value
	lower = func(v canonical.Value) canonical.Value {
		if v.Kind == canonical.String && validUUID(v.String) {
			v.String = strings.ToLower(v.String)
		}
		for k, x := range v.Object {
			v.Object[k] = lower(x)
		}
		for i, x := range v.Array {
			v.Array[i] = lower(x)
		}
		return v
	}
	snapshot = lower(snapshot)
	codec := New(Options{})
	b, e := codec.Encode(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	d, e := codec.Decode(b)
	if e != nil || !reflect.DeepEqual(d.Snapshot(), snapshot) {
		t.Fatal("UUID spelling not preserved", e)
	}
	state, _, e := ToDomain(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	if state.Tasks.Subtasks[0].LineageID != core.SubtaskLineageID(testID(901)) {
		t.Fatal("domain identity not canonical")
	}
}
func TestReaderRejectsOverflowingLimit(t *testing.T) {
	codec := New(Options{Limits: canonical.Limits{MaxBytes: int(^uint(0) >> 1)}})
	if _, e := codec.Read(strings.NewReader("{}")); e == nil {
		t.Fatal("overflowing reader limit accepted")
	}
}

// Captured from this Go candidate's deterministic fixture, not a Swift oracle.
func TestSyntheticCandidateGoldenDoesNotDrift(t *testing.T) {
	snapshot, _ := fixture(t)
	got, e := New(Options{}).Encode(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	expected, e := os.ReadFile("testdata/synthetic-v7-candidate.json")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("candidate fixture changed; review wire changes explicitly")
	}
	if _, e = New(Options{}).Decode(expected); e != nil {
		t.Fatal(e)
	}
}

func TestRevisionWorkBudgetRejectsBeforeQuadraticValidation(t *testing.T) {
	snapshot, state := fixture(t)
	p := snapshot.Object["taskCycleSeries"].Array[0]
	revisions := p.Object["classificationRevisions"]
	for len(revisions.Array) <= MaximumRevisionsPerSeries {
		revisions.Array = append(revisions.Array, revisions.Array[0])
	}
	p.Object["classificationRevisions"] = revisions
	if _, e := New(Options{}).Inspect(rawEnvelope(t, snapshot)); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
	for len(state.Series[0].ClassificationRevisions) <= MaximumRevisionsPerSeries {
		state.Series[0].ClassificationRevisions = append(state.Series[0].ClassificationRevisions, state.Series[0].ClassificationRevisions[0])
	}
	if _, e := FromDomain(state, extras(t)); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
}
