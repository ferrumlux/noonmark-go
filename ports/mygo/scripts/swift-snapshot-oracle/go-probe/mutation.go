// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/json"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var mutationIDs = []string{"raw-bytes", "raw-depth", "raw-nodes", "raw-string", "raw-string-sum", "raw-array", "raw-members", "profile-empty-array", "profile-pairs-odd", "profile-pairs-duplicate", "profile-pairs-nonempty", "profile-base64", "hex-odd", "hex-invalid-utf8", "hex-decoded-plus-one", "hex-double-expansion", "create-growth-nodes", "create-growth-entities", "create-growth-text", "create-growth-output", "schedule-growth-nodes", "schedule-growth-entities", "schedule-growth-text", "schedule-growth-output"}

type mutationResult struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Accepted      bool           `json:"accepted"`
	Reason        string         `json:"reason"`
	Counters      map[string]int `json:"counters"`
}

func validateCommandText(f frozen, c command) error {
	if c.Op != "create" {
		return nil
	}
	if _, e := textHex(c.TitleUTF8Hex); e != nil {
		return e
	}
	if _, e := textHex(c.Description.UTF8Hex); e != nil {
		return e
	}
	v, ok := f.Vectors[c.NoteVectorID]
	if !ok {
		return fail("registry-mismatch:vector")
	}
	if _, e := textHex(v.Input.UTF8Hex); e != nil {
		return e
	}
	_, e := textHex(v.GoInitialNote.BodyUTF8Hex)
	return e
}
func predictCommand(before any, c command) error {
	entities := 0
	var count func(any)
	count = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if _, ok := x["id"]; ok {
				entities++
			}
			for _, v := range x {
				count(v)
			}
		case []any:
			for _, v := range x {
				count(v)
			}
		}
	}
	count(before)
	growth := 3
	if c.Op == "schedule" {
		growth = 5
	}
	if entities+growth > 96 {
		return fail("resource-limit:predicted-entities")
	}
	return budget(before, 400, 16384)
}

// Fixed mutations exercise the same scanner/profile/budget functions as production
// capture paths. No payload, path, source bytes, or limits may be supplied by caller.
func runMutation(f frozen, id string) (mutationResult, error) {
	r := mutationResult{SchemaVersion: 2, ID: id}
	known := id == "control-valid"
	for _, v := range mutationIDs {
		if id == v {
			known = true
		}
	}
	if !known {
		return r, fail("unknown-mutation")
	}
	b, e := readBounded(filepath.Join(f.sourceDir, "r2-seeds/empty.json"), 262144)
	if e != nil {
		return r, e
	}
	if digest(b) != f.Seeds["empty"].SHA256 {
		return r, fail("source-mismatch")
	}
	v, e := f.Schema.preflight(b)
	if e != nil {
		return r, e
	}
	var cmd command
	for _, c := range f.Cases {
		if c.ID == "create-valid" {
			cmd = c.Steps[0]
		}
	}
	resetCalls()
	var rejection error
	if id == "control-valid" {
		rejection = positiveControl(f, b, cmd)
	} else if strings.HasPrefix(id, "raw-") {
		switch id {
		case "raw-bytes":
			b = []byte(strings.Repeat(" ", 262145))
		case "raw-depth":
			b = []byte(strings.Repeat("[", 25) + "0" + strings.Repeat("]", 25))
		case "raw-nodes":
			row := "[" + strings.TrimSuffix(strings.Repeat("0,", 96), ",") + "]"
			b = []byte("[" + strings.TrimSuffix(strings.Repeat(row+",", 96), ",") + "]")
		case "raw-string":
			b = []byte(`"` + strings.Repeat("x", 4097) + `"`)
		case "raw-string-sum":
			b = []byte("[" + strings.TrimSuffix(strings.Repeat(`"`+strings.Repeat("x", 4096)+`",`, 17), ",") + "]")
		case "raw-array":
			b = []byte("[" + strings.TrimSuffix(strings.Repeat("0,", 97), ",") + "]")
		case "raw-members":
			m := map[string]any{}
			for i := 0; i < 33; i++ {
				m[strings.Repeat("k", i+1)] = true
			}
			b, _ = json.Marshal(m)
		}
		rejection = mutationDomainPath(f, b)
	} else if strings.HasPrefix(id, "profile-") {
		m := v.(map[string]any)
		s := m["snapshot"].(map[string]any)
		cl := s["classifications"].(map[string]any)
		switch id {
		case "profile-empty-array":
			s["ideas"] = []any{map[string]any{}}
		case "profile-pairs-odd":
			cl["currentByChainID"] = []any{"a"}
		case "profile-pairs-duplicate":
			cl["currentByChainID"] = []any{"a", map[string]any{}, "a", map[string]any{}}
		case "profile-pairs-nonempty":
			cl["currentByChainID"] = []any{"a", map[string]any{}}
		case "profile-base64":
			m["base64"] = "AA=="
		}
		b, _ = json.Marshal(m)
		rejection = mutationDomainPath(f, b)
	} else if strings.HasPrefix(id, "hex-") {
		switch id {
		case "hex-odd":
			cmd.TitleUTF8Hex = "a"
		case "hex-invalid-utf8":
			cmd.TitleUTF8Hex = "ff"
		case "hex-decoded-plus-one":
			cmd.TitleUTF8Hex = strings.Repeat("61", 4097)
		case "hex-double-expansion":
			cmd.TitleUTF8Hex = strings.Repeat("61", 8192)
		}
		rejection = validateCommandText(f, cmd)
		if rejection == nil {
			rejection = mutationDomainPath(f, b)
		}
	} else {
		if strings.HasPrefix(id, "schedule-") {
			for _, c := range f.Cases {
				if c.ID == "schedule-planned-12" {
					cmd = c.Steps[0]
				}
			}
		}
		var before any
		switch {
		case strings.HasSuffix(id, "-nodes"):
			a := make([]any, 8193)
			for i := range a {
				a[i] = true
			}
			before = a
		case strings.HasSuffix(id, "-entities"):
			a := []any{}
			for i := 0; i < 96; i++ {
				a = append(a, map[string]any{"id": "fixed-negative"})
			}
			before = a
		case strings.HasSuffix(id, "-text"):
			before = strings.Repeat("x", 65537)
		case strings.HasSuffix(id, "-output"):
			a := make([]any, 2000)
			for i := range a {
				a[i] = true
			}
			before = a
		}
		rejection = predictCommand(before, cmd)
		if rejection == nil {
			rejection = mutationDomainPath(f, b)
		}
	}
	r.Accepted = rejection == nil
	if rejection != nil {
		r.Reason = rejection.Error()
	}
	r.Counters = map[string]int{}
	for k, n := range operationCalls {
		r.Counters[k] = n
	}
	return r, nil
}
func mutationDomainPath(f frozen, b []byte) error {
	v, e := f.Schema.preflight(b)
	if e != nil {
		return e
	}
	if e = budget(v, 0, 0); e != nil {
		return e
	}
	p, e := canonical.Parse(b, codecLimits)
	if e != nil {
		return e
	}
	s, x, e := domainDecode(p.Object["snapshot"])
	if e != nil {
		return e
	}
	k, e := kernelNew(s.Tasks)
	if e != nil {
		return e
	}
	wire, e := domainBridge(recurrence.State{Tasks: kernelSnapshot(k), Series: []recurrence.Series{}}, x)
	if e != nil {
		return e
	}
	_, e = codecEncode(newCodec(), wire)
	return e
}

// Positive control uses prepared canonical fixture bytes only at seed input.
// Cross-decode inputs are never passed through this preparation.
func positiveControl(f frozen, b []byte, cmd command) error {
	v, e := f.Schema.preflight(b)
	if e != nil {
		return e
	}
	if e = budget(v, 0, 0); e != nil {
		return e
	}
	parsed, e := canonical.Parse(b, codecLimits)
	if e != nil {
		return e
	}
	prepared, e := canonical.Marshal(parsed)
	if e != nil {
		return e
	}
	c := newCodec()
	inspection, e := c.Inspect(prepared)
	if e != nil {
		return e
	}
	if inspection.RequiresClassificationAudit {
		return fail("unsupported-profile:audit")
	}
	doc, e := codecDecode(c, prepared)
	if e != nil {
		return e
	}
	state, extra, e := domainDecode(doc.Snapshot())
	if e != nil {
		return e
	}
	k, e := kernelNew(state.Tasks)
	if e != nil {
		return e
	}
	if e = execute(f, k, cmd, v); e != nil {
		return e
	}
	wire, e := domainBridge(recurrence.State{Tasks: kernelSnapshot(k), Series: []recurrence.Series{}}, extra)
	if e != nil {
		return e
	}
	_, e = codecEncode(c, wire)
	return e
}

// Ingress fault injection is internal to this executable and can only select a
// closed entry/dimension matrix. Ordinary paths use these exact same wrappers.
type ingressTrace struct {
	InputBytes             int
	InputSHA256            string
	Entry, Dimension, Mode string
	Reached, Active        bool
	Events                 []string
	Counts                 map[string]int
}

var ingressFault *ingressTrace

func ingressEvent(event string) {
	if ingressFault == nil || !ingressFault.Active {
		return
	}
	if len(ingressFault.Events) < 64 {
		ingressFault.Events = append(ingressFault.Events, event)
	}
	switch event {
	case "materialize", "decode", "clone", "bridge", "command", "serializer":
		ingressFault.Counts[event]++
	}
}

type ingressRow struct {
	InputBytes  int            `json:"inputBytes"`
	InputSHA256 string         `json:"inputSHA256"`
	Entry       string         `json:"entry"`
	Dimension   string         `json:"dimension"`
	Outcome     string         `json:"outcome"`
	Reason      string         `json:"reason"`
	Reached     bool           `json:"reached"`
	Events      []string       `json:"events"`
	Counters    map[string]int `json:"counters"`
	Pass        bool           `json:"pass"`
}
type ingressReport struct {
	SchemaVersion int          `json:"schemaVersion"`
	Mode          string       `json:"mode"`
	Pass          bool         `json:"pass"`
	Rows          []ingressRow `json:"rows"`
	ExpectedRows  []string     `json:"expectedRows"`
}

var ingressEntries = []string{"config-FieldSchema-R2.json", "config-SeedRegistry-R2.json", "config-NoteVectors-R2.json", "config-case", "seed-empty", "seed-planned-12", "seed-planned-21", "seed-multi-interleaved", "go-kernel-projection", "go-from-domain-projection", "go-package", "go-export", "go-self-reencoded", "cross"}

var ingressPlanned map[string]any
var ingressMulti map[string]any

func ingressBytes(dim string, l limits, original []byte) []byte {
	switch dim {
	case "utf8":
		return []byte{0xff}
	case "duplicate":
		return []byte(`{"x":1,"x":2}`)
	case "float":
		return []byte(`{"x":1.25}`)
	case "cardinality", "hex-odd", "hex-invalid-utf8", "hex-decoded-plus-one", "hex-double-expansion", "empty-array", "pairs-odd", "pairs-duplicate", "pairs-nonempty", "base64", "entities-array", "planned-array", "notes-array", "days-array", "definitions-array", "traces-array", "subtasks-array", "trace-notes-array":
		var v any
		d := json.NewDecoder(bytes.NewReader(original))
		d.UseNumber()
		_ = d.Decode(&v)
		if dim == "cardinality" {
			switch x := v.(type) {
			case []any:
				v = append(x, x[0])
			case map[string]any:
				if a, ok := x["seeds"].([]any); ok {
					x["seeds"] = append(a, a[0])
				} else if o, ok := x["objects"].(map[string]any); ok {
					delete(o, "Envelope")
				} else {
					x["unknown"] = true
				}
			}
		} else if strings.HasPrefix(dim, "hex-") {
			a := v.([]any)
			row := a[0].(map[string]any)
			input := row["input"].(map[string]any)
			h := "0"
			if dim == "hex-invalid-utf8" {
				h = "ff"
			}
			if dim == "hex-decoded-plus-one" {
				h = strings.Repeat("61", 4097)
			}
			if dim == "hex-double-expansion" {
				h = strings.Repeat("61", 8192)
			}
			input["present"] = true
			input["utf8Hex"] = h
		} else {
			m := v.(map[string]any)
			snap := m["snapshot"].(map[string]any)
			switch dim {
			case "empty-array":
				snap["ideas"] = []any{true}
			case "base64":
				m["base64"] = "AA=="
			case "pairs-odd", "pairs-duplicate", "pairs-nonempty":
				n := 2
				if dim == "pairs-odd" {
					n = 1
				}
				if dim == "pairs-duplicate" {
					n = 4
				}
				a := []any{}
				for i := 0; i < n; i++ {
					a = append(a, "duplicate")
				}
				snap["classifications"].(map[string]any)["currentByChainID"] = a
			case "days-array", "definitions-array", "traces-array", "subtasks-array":
				a := []any{}
				for i := 0; i < 17; i++ {
					a = append(a, map[string]any{})
				}
				snap[strings.TrimSuffix(dim, "-array")] = a
			case "trace-notes-array":
				template, _ := json.Marshal(ingressMulti)
				var copy map[string]any
				dd := json.NewDecoder(bytes.NewReader(template))
				dd.UseNumber()
				_ = dd.Decode(&copy)
				tr := copy["snapshot"].(map[string]any)["traces"].([]any)[0].(map[string]any)
				tr["noteEntries"] = []any{map[string]any{}, map[string]any{}}
				snap["traces"] = []any{tr}
			case "entities-array":
				a := []any{}
				for i := 0; i < 17; i++ {
					a = append(a, ingressPlanned["snapshot"].(map[string]any)["chains"].([]any)[0])
				}
				snap["chains"] = a
			case "planned-array", "notes-array":
				template, _ := json.Marshal(ingressPlanned)
				var copy map[string]any
				dd := json.NewDecoder(bytes.NewReader(template))
				dd.UseNumber()
				_ = dd.Decode(&copy)
				ts := copy["snapshot"].(map[string]any)
				if dim == "planned-array" {
					def := ts["definitions"].([]any)[0].(map[string]any)
					p := def["plannedSubtasks"].([]any)[0]
					def["plannedSubtasks"] = []any{p, p, p}
					snap["definitions"] = []any{def}
				} else {
					chain := ts["chains"].([]any)[0].(map[string]any)
					chain["noteEntries"] = []any{map[string]any{}, map[string]any{}}
					snap["chains"] = []any{chain}
				}
			}
		}
		b, _ := json.Marshal(v)
		return b
	case "bytes":
		return []byte(strings.Repeat(" ", l.Bytes+1))
	case "depth":
		return []byte(strings.Repeat("[", l.Depth) + "0" + strings.Repeat("]", l.Depth))
	case "string":
		return []byte(`"` + strings.Repeat("x", l.String+1) + `"`)
	case "sum": // nested rows keep individual array length below each entry limit
		a := []any{}
		for n := 0; n <= l.TotalString/l.String; n++ {
			a = append(a, strings.Repeat("x", l.String))
		}
		if len(a) > l.Array {
			rows := []any{}
			for len(a) > 0 {
				n := l.Array
				if n > len(a) {
					n = len(a)
				}
				rows = append(rows, a[:n])
				a = a[n:]
			}
			b, _ := json.Marshal(rows)
			return b
		}
		b, _ := json.Marshal(a)
		return b
	case "array":
		a := make([]bool, l.Array+1)
		b, _ := json.Marshal(a)
		return b
	case "members":
		m := map[string]bool{}
		for i := 0; i <= l.Members; i++ {
			m[strings.Repeat("k", i+1)] = true
		}
		b, _ := json.Marshal(m)
		return b
	case "nodes": // balanced bounded fanout avoids hitting the array limit first
		var tree func(int) any
		tree = func(n int) any {
			if n <= 1 {
				return true
			}
			a := []any{}
			chunk := (n + l.Array - 1) / l.Array
			for n > 0 {
				k := chunk
				if k > n {
					k = n
				}
				a = append(a, tree(k))
				n -= k
			}
			return a
		}
		b, _ := json.Marshal(tree(l.Nodes + 1))
		return b
	}
	return []byte(`{}`)
}
func runIngressMatrix(spec, mode, out string) (ingressReport, error) {
	r := ingressReport{SchemaVersion: 2, Mode: mode, Pass: true}
	if mode != "normal" && mode != "bypass" && mode != "late" {
		return r, fail("unknown-matrix-mode")
	}
	if e := os.Mkdir(out, 0700); e != nil {
		return r, e
	}
	defer func() { ingressFault = nil }()
	template, e := readBounded(filepath.Join(spec, "r2-seeds/planned-12.json"), 262144)
	if e != nil {
		return r, e
	}
	if digest(template) != "aecba81054e72fbdf749c928cbd5d8346c856c258e3e718f30a6f0ff71c2decb" {
		return r, fail("source-mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(template))
	decoder.UseNumber()
	if e = decoder.Decode(&ingressPlanned); e != nil {
		return r, e
	}
	multi, e := readBounded(filepath.Join(spec, "r2-seeds/multi-interleaved.json"), 262144)
	if e != nil {
		return r, e
	}
	if digest(multi) != "4e3159d88491d994a2410972513c6312258609d8ce3ad89aceed28c5585120da" {
		return r, fail("source-mismatch")
	}
	md := json.NewDecoder(bytes.NewReader(multi))
	md.UseNumber()
	if e = md.Decode(&ingressMulti); e != nil {
		return r, e
	}
	ordinal := 0
	for _, entry := range ingressEntries {
		dims := []string{"positive", "bytes", "depth", "nodes", "string", "sum", "array", "members", "utf8", "duplicate", "float"}
		if strings.HasPrefix(entry, "config-") {
			dims = append(dims, "cardinality")
			if entry == "config-NoteVectors-R2.json" {
				dims = append(dims, "hex-odd", "hex-invalid-utf8", "hex-decoded-plus-one", "hex-double-expansion")
			}
		} else {
			dims = append(dims, "empty-array", "pairs-odd", "pairs-duplicate", "pairs-nonempty", "base64", "entities-array", "planned-array", "notes-array", "days-array", "definitions-array", "traces-array", "subtasks-array", "trace-notes-array")
		}
		for _, dim := range dims {
			ordinal++
			ingressFault = &ingressTrace{Entry: entry, Dimension: dim, Mode: mode, Counts: map[string]int{"materialize": 0, "decode": 0, "clone": 0, "bridge": 0, "command": 0, "serializer": 0}}
			e := dispatchIngress(spec, entry, filepath.Join(out, "work-"+strconv.Itoa(ordinal)))
			tr := ingressFault
			row := ingressRow{InputBytes: tr.InputBytes, InputSHA256: tr.InputSHA256, Entry: entry, Dimension: dim, Reached: tr.Reached, Events: tr.Events, Counters: tr.Counts, Outcome: "accepted"}
			if e != nil {
				row.Outcome = "rejected"
				row.Reason = e.Error()
			}
			if dim == "positive" {
				row.Pass = tr.Reached && e == nil && tr.Counts["materialize"] > 0 && len(tr.Events) > 1 && tr.Events[0] == "scan" && tr.Events[1] == "scan-pass"
			} else {
				zero := true
				for _, n := range tr.Counts {
					zero = zero && n == 0
				}
				row.Pass = tr.Reached && e != nil && zero && len(tr.Events) == 2 && tr.Events[0] == "scan" && tr.Events[1] == "reject"
				if dim == "cardinality" || strings.HasPrefix(dim, "hex-") || strings.Contains(dim, "pairs-") || dim == "empty-array" || dim == "base64" || strings.HasSuffix(dim, "-array") {
					domainZero := true
					for k, n := range tr.Counts {
						if k != "materialize" {
							domainZero = domainZero && n == 0
						}
					}
					row.Pass = tr.Reached && e != nil && domainZero && len(tr.Events) >= 2 && tr.Events[0] == "scan" && (tr.Events[1] == "reject" || tr.Events[1] == "scan-pass")
				}
			}
			r.Pass = r.Pass && row.Pass
			r.Rows = append(r.Rows, row)
			r.ExpectedRows = append(r.ExpectedRows, row.Entry+"/"+row.Dimension)
		}
	}
	for _, op := range []string{"create", "schedule"} {
		ingressFault = nil
		f, e := loadFrozen(spec)
		if e != nil {
			return r, e
		}
		var c caseSpec
		for _, x := range f.Cases {
			if x.ID == map[string]string{"create": "create-valid", "schedule": "schedule-planned-12"}[op] {
				c = x
			}
		}
		b, e := readBounded(filepath.Join(spec, f.Seeds[c.Seed].File), 262144)
		if e != nil {
			return r, e
		}
		ingressFault = &ingressTrace{Entry: op, Dimension: "positive", Active: true, Reached: true, Counts: map[string]int{"materialize": 0, "decode": 0, "clone": 0, "bridge": 0, "command": 0, "serializer": 0}}
		resetCalls()
		caseBytes, ce := json.Marshal(f.CaseRaw[c.ID])
		if ce != nil {
			return r, ce
		}
		ingressFault.InputBytes = len(caseBytes)
		ingressFault.InputSHA256 = digest(caseBytes)
		c, ce = f.matchCase(caseBytes)
		if ce != nil {
			return r, ce
		}
		_, e = runCase(f, c, b, filepath.Join(out, "control-"+op))
		tr := ingressFault
		pass := e == nil
		for _, k := range []string{"decode", "clone", "bridge", "command", "serializer"} {
			pass = pass && tr.Counts[k] > 0
		}
		row := ingressRow{InputBytes: tr.InputBytes, InputSHA256: tr.InputSHA256, Entry: op, Dimension: "positive", Reached: true, Events: tr.Events, Counters: tr.Counts, Pass: pass, Outcome: "accepted"}
		if e != nil {
			row.Outcome = "rejected"
			row.Reason = e.Error()
		}
		r.Rows = append(r.Rows, row)
		r.ExpectedRows = append(r.ExpectedRows, row.Entry+"/"+row.Dimension)
		r.Pass = r.Pass && pass
	}
	return r, nil
}
func dispatchIngress(spec, entry, out string) (err error) {
	defer func() {
		if recover() != nil {
			err = fail("downstream-panic")
		}
	}()
	f, e := loadFrozen(spec)
	if e != nil {
		return e
	}
	c := f.Cases[0]
	if strings.HasPrefix(entry, "seed-") {
		seedID := strings.TrimPrefix(entry, "seed-")
		for _, x := range f.Cases {
			if x.ID == "observe-"+seedID {
				c = x
			}
		}
	}
	cb, e := json.Marshal(f.CaseRaw[c.ID])
	if e != nil {
		return e
	}
	c, e = f.matchCase(cb)
	if e != nil {
		return e
	}
	raw, e := readBounded(filepath.Join(spec, f.Seeds[c.Seed].File), 262144)
	if e != nil {
		return e
	}
	r, e := runCase(f, c, raw, out)
	if e != nil {
		return e
	}
	if entry == "cross" {
		_, e = cross(f, r.Points[0].Package)
	}
	return e
}
