// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSpecPath() string {
	if path := os.Getenv("SNAPSHOT_SPEC"); path != "" {
		return path
	}
	return "../spec"
}

func TestPreflightRejectsBeforeMaterialize(t *testing.T) {
	for _, s := range []string{`{"a":1,"a":2}`, `{"a":null}`, `{"a":1.5}`, `{"a":1e2}`, `{"a":"\ud800"}`, `{"a":"` + strings.Repeat("x", 4097) + `"}`, strings.Repeat("[", 25) + "0" + strings.Repeat("]", 25), `{"a":` + strings.Repeat("9", 21) + `}`, string([]byte{0xff})} {
		if _, e := parseBounded([]byte(s), rawLimits); e == nil {
			t.Fatalf("accepted malicious input %.50q", s)
		}
	}
}
func TestFrozenCasesRealGo(t *testing.T) {
	spec := testSpecPath()
	if spec == "" {
		t.Fatal("SNAPSHOT_SPEC required")
	}
	f, e := loadFrozen(spec)
	if e != nil {
		t.Fatal(e)
	}
	if len(f.Cases) != 18 {
		t.Fatal("case count")
	}
	points := 0
	for _, c := range f.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw, e := os.ReadFile(filepath.Join(spec, f.Seeds[c.Seed].File))
			if e != nil {
				t.Fatal(e)
			}
			out := filepath.Join(t.TempDir(), "out")
			r, e := runCase(f, c, raw, out)
			if e != nil {
				t.Fatal(e)
			}
			points += len(r.Points)
			for _, p := range r.Points {
				if !p.SelfEqual {
					t.Fatal("self roundtrip")
				}
				if string(p.Kernel) != string(p.FromDomain) {
					t.Fatal("direct/fromDomain mismatch")
				}
				var env map[string]any
				if e = json.Unmarshal(p.Package, &env); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
	if points != 32 {
		t.Fatal(points)
	}
}
func TestRawArchiveBeforeSchema(t *testing.T) {
	d := t.TempDir()
	a := archive{Root: d}
	raw := []byte("{malformed")
	n, e := a.save("seed", "go-package", raw)
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(d, n))
	if e != nil || string(got) != string(raw) {
		t.Fatal("raw lost")
	}
	if _, e = parseBounded(got, rawLimits); e == nil {
		t.Fatal("malformed accepted")
	}
	if _, e = a.save("seed", "go-package", raw); e == nil {
		t.Fatal("overwrite")
	}
	if _, e = a.save("seed", "go-export", make([]byte, 262145)); e == nil {
		t.Fatal("oversized")
	}
}

func TestCrossGateOrderAndNoCanonicalization(t *testing.T) {
	f, e := loadFrozen(testSpecPath())
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range [][]byte{[]byte(`{"base64":"AA=="}`), []byte(`{"x":1,"x":2}`), make([]byte, 262145)} {
		crossInspectCalls, crossDecodeCalls = 0, 0
		if _, e := cross(f, b); e == nil {
			t.Fatal("bad cross accepted")
		}
		if crossInspectCalls != 0 || crossDecodeCalls != 0 {
			t.Fatal("decoder invoked before preflight")
		}
	}
	raw, e := os.ReadFile(filepath.Join(testSpecPath(), "r2-seeds/empty.json"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := runCase(f, f.Cases[0], raw, filepath.Join(t.TempDir(), "out"))
	if e != nil {
		t.Fatal(e)
	}
	pkg := r.Points[0].Package
	crossInspectCalls, crossDecodeCalls = 0, 0
	if status, e := cross(f, pkg); e != nil || status != "accepted" {
		t.Fatal(status, e)
	}
	if crossInspectCalls != 1 || crossDecodeCalls != 1 {
		t.Fatal("missing actual decoder")
	}
	crossInspectCalls, crossDecodeCalls = 0, 0
	if status, e := cross(f, append([]byte(" "), pkg...)); e == nil || status != "inspect-rejected" {
		t.Fatal("noncanonical cross washed", status, e)
	}
	if crossInspectCalls != 1 || crossDecodeCalls != 0 {
		t.Fatal("wrong reject order")
	}
}
func TestScannerEachLimit(t *testing.T) {
	tests := []struct {
		input string
		l     limits
	}{
		{`{}`, limits{1, 24, 8192, 4096, 65536, 32, 96}},
		{`{"a":{}}`, limits{262144, 1, 8192, 4096, 65536, 32, 96}},
		{`{"a":1}`, limits{262144, 24, 2, 4096, 65536, 32, 96}},
		{`{"a":"xx"}`, limits{262144, 24, 8192, 1, 65536, 32, 96}},
		{`{"a":"x","b":"y"}`, limits{262144, 24, 8192, 4096, 3, 32, 96}},
		{`{"a":1,"b":2}`, limits{262144, 24, 8192, 4096, 65536, 1, 96}},
		{`[1,2]`, limits{262144, 24, 8192, 4096, 65536, 32, 1}},
	}
	for _, x := range tests {
		if _, e := parseBounded([]byte(x.input), x.l); e == nil {
			t.Fatalf("limit ignored %+v", x)
		}
	}
}
func TestProfileRejectsBeforeDecoder(t *testing.T) {
	f, e := loadFrozen(testSpecPath())
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(testSpecPath(), "r2-seeds/empty.json"))
	if e != nil {
		t.Fatal(e)
	}
	v, e := parseBounded(b, rawLimits)
	if e != nil {
		t.Fatal(e)
	}
	m := v.(map[string]any)
	snap := m["snapshot"].(map[string]any)
	snap["ideas"] = []any{map[string]any{"anything": true}}
	bad, _ := json.Marshal(v)
	crossInspectCalls, crossDecodeCalls = 0, 0
	if _, e = cross(f, bad); e == nil {
		t.Fatal("nonempty unsupported accepted")
	}
	if crossInspectCalls != 0 || crossDecodeCalls != 0 {
		t.Fatal("decoder used")
	}
}
func TestPredictionRejectsBeforeCommand(t *testing.T) {
	if e := budget(map[string]any{}, 8193, 0); e == nil {
		t.Fatal("nodes ignored")
	}
	if e := budget(map[string]any{}, 0, 65537); e == nil {
		t.Fatal("strings ignored")
	}
	if e := budget(map[string]any{}, 2048, 0); e == nil {
		t.Fatal("output bound ignored")
	}
}
func TestObserveArchivesBeforeSchemaCallback(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"a":1,"a":2}`), {0xff}, []byte(`{"base64":"AA=="}`)} {
		d := t.TempDir()
		a := archive{Root: d}
		called := 0
		ar, e := a.observe("seed", "go-from-domain-projection", raw, func(b []byte) (any, error) {
			called++
			got, e := os.ReadFile(filepath.Join(d, "seed/go-from-domain-projection.json"))
			if e != nil || string(got) != string(raw) {
				t.Fatal("schema ran before archive")
			}
			return nil, fail("schema-invalid")
		})
		if e == nil || called != 1 || ar.RawState != "malformed-raw" || ar.SHA256 != digest(raw) {
			t.Fatal("raw qualification incorrect")
		}
	}
	a := archive{Root: t.TempDir()}
	called := 0
	ar, e := a.observe("seed", "go-package", make([]byte, 262145), func([]byte) (any, error) { called++; return nil, nil })
	if e == nil || called != 0 || ar.Name != "" {
		t.Fatal("oversized treated as archived")
	}
}
func TestFixedMutationGateWitnesses(t *testing.T) {
	f, e := loadFrozen(testSpecPath())
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range mutationIDs {
		t.Run(id, func(t *testing.T) {
			r, e := runMutation(f, id)
			if e != nil {
				t.Fatal(e)
			}
			if r.Accepted || r.Reason == "" {
				t.Fatal("mutation not rejected")
			}
			for name, n := range r.Counters {
				if n != 0 {
					t.Fatalf("downstream %s=%d", name, n)
				}
			}
		})
	}
	if _, e := runMutation(f, "free-payload"); e == nil {
		t.Fatal("unknown mutation accepted")
	}
}
func TestOperationCountersObserveRealCalls(t *testing.T) {
	f, e := loadFrozen(testSpecPath())
	if e != nil {
		t.Fatal(e)
	}
	var c caseSpec
	for _, x := range f.Cases {
		if x.ID == "create-valid" {
			c = x
		}
	}
	b, e := os.ReadFile(filepath.Join(testSpecPath(), "r2-seeds/empty.json"))
	if e != nil {
		t.Fatal(e)
	}
	resetCalls()
	if _, e = runCase(f, c, b, filepath.Join(t.TempDir(), "out")); e != nil {
		t.Fatal(e)
	}
	for name, n := range operationCalls {
		if n <= 0 {
			t.Fatalf("real call not counted %s=%d", name, n)
		}
	}
}
func TestFixedPositiveMutation(t *testing.T) {
	f, e := loadFrozen(testSpecPath())
	if e != nil {
		t.Fatal(e)
	}
	r, e := runMutation(f, "control-valid")
	if e != nil {
		t.Fatal(e)
	}
	if !r.Accepted || r.Reason != "" {
		t.Fatal(r)
	}
	for name, n := range r.Counters {
		if n <= 0 {
			t.Fatalf("positive control missing %s", name)
		}
	}
}
func TestActualIngressMatrixFaultControls(t *testing.T) {
	spec := os.Getenv("SNAPSHOT_SPEC")
	for _, mode := range []string{"normal", "bypass", "late"} {
		r, e := runIngressMatrix(spec, mode, filepath.Join(t.TempDir(), "matrix"))
		if e != nil {
			t.Fatal(e)
		}
		if r.Pass != (mode == "normal") {
			t.Fatalf("%s gate=%v", mode, r.Pass)
		}
		if len(r.Rows) != 294 {
			t.Fatal("missing closed matrix")
		}
	}
}
