// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var uuidRE = regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`)

func (s schema) validate(v any, typ string) error {
	if alias, ok := s.Aliases[typ]; ok {
		return s.validate(v, alias)
	}
	if strings.HasPrefix(typ, "[]") || strings.HasPrefix(typ, "pairs:") {
		a, ok := v.([]any)
		if !ok {
			return fail("schema-invalid:array")
		}
		if strings.HasPrefix(typ, "pairs:") {
			if len(a) != 0 {
				return fail("unsupported-profile:pairs")
			}
			return nil
		}
		limit := 16
		el := typ[2:]
		if el == "PlannedSubtask" {
			limit = 2
		}
		if el == "Note" {
			limit = 1
		}
		if len(a) > limit {
			return fail("resource-limit:array")
		}
		for _, x := range a {
			if e := s.validate(x, el); e != nil {
				return e
			}
		}
		return nil
	}
	if fs, ok := s.Objects[typ]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return fail("schema-invalid:object")
		}
		seen := 0
		for _, f := range fs {
			x, present := m[f.Wire]
			if !present {
				if f.Optional {
					continue
				}
				return fail("schema-invalid:missing-" + f.Wire)
			}
			seen++
			if typ == "LocalDate" {
				n, ok := x.(json.Number)
				if !ok {
					return fail("schema-invalid:date-int32")
				}
				if _, e := strconv.ParseInt(string(n), 10, 32); e != nil {
					return fail("schema-invalid:date-int32")
				}
			}
			if f.Optional && f.Wire != "descriptionText" {
				return fail("unsupported-profile:optional")
			}
			if f.M1 == "empty-array-only" {
				a, ok := x.([]any)
				if !ok || len(a) != 0 {
					return fail("unsupported-profile:empty-array")
				}
				continue
			}
			if e := s.validate(x, f.Type); e != nil {
				return e
			}
			switch f.M1 {
			case "active", "pending":
				if x != f.M1 {
					return fail("unsupported-profile:state")
				}
			case "0", "1":
				if x != json.Number(f.M1) {
					return fail("unsupported-profile:number")
				}
			}
		}
		if seen != len(m) {
			return fail("schema-invalid:unknown-field")
		}
		return nil
	}
	switch {
	case typ == "string" || typ == "uuid" || strings.HasPrefix(typ, "enum:"):
		x, ok := v.(string)
		if !ok {
			return fail("schema-invalid:string")
		}
		if typ == "uuid" && !uuidRE.MatchString(x) {
			return fail("schema-invalid:uuid")
		}
		if strings.HasPrefix(typ, "enum:") && !strings.Contains(","+typ[5:]+",", ","+x+",") {
			return fail("schema-invalid:enum")
		}
		return nil
	case typ == "bool":
		if _, ok := v.(bool); !ok {
			return fail("schema-invalid:bool")
		}
		return nil
	case typ == "int" || typ == "uint" || typ == "stamp" || strings.HasPrefix(typ, "enumint:"):
		n, ok := v.(json.Number)
		if !ok {
			return fail("schema-invalid:integer")
		}
		if typ == "uint" || typ == "stamp" {
			u, e := strconv.ParseUint(string(n), 10, 64)
			if e != nil {
				return e
			}
			if typ == "stamp" && (math.IsInf(math.Float64frombits(u), 0) || math.IsNaN(math.Float64frombits(u))) {
				return fail("schema-invalid:stamp")
			}
		} else if _, e := strconv.ParseInt(string(n), 10, 64); e != nil {
			return e
		}
		if strings.HasPrefix(typ, "enumint:") && !strings.Contains(","+typ[8:]+",", ","+string(n)+",") {
			return fail("schema-invalid:enumint")
		}
		return nil
	}
	return fail("unsupported-profile:type-" + typ)
}
func (s schema) preflight(b []byte) (any, error) { return s.preflightNamed("internal", b) }
func (s schema) preflightNamed(entry string, b []byte) (any, error) {
	v, e := parseIngress(entry, b, rawLimits)
	if e != nil {
		return nil, e
	}
	target := ingressFault != nil && ingressFault.Entry == entry && ingressFault.Active
	if target {
		ingressEvent("profile")
	}
	if e = s.validate(v, "Envelope"); e != nil {
		if target {
			ingressEvent("profile-reject")
		}
		return nil, e
	}
	m := v.(map[string]any)
	if m["formatVersion"] != json.Number("7") {
		return nil, fail("unsupported-profile:version")
	}
	n := 0
	var walk func(any)
	walk = func(x any) {
		switch q := x.(type) {
		case map[string]any:
			if _, ok := q["id"]; ok {
				n++
			}
			for _, y := range q {
				walk(y)
			}
		case []any:
			for _, y := range q {
				walk(y)
			}
		}
	}
	walk(v)
	if n > 96 {
		return nil, fail("resource-limit:entities")
	}
	return v, nil
}
func cv(v any) canonical.Value {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]canonical.Value{}
		for k, w := range x {
			m[k] = cv(w)
		}
		return canonical.Value{Kind: canonical.Object, Object: m}
	case []any:
		a := []canonical.Value{}
		for _, w := range x {
			a = append(a, cv(w))
		}
		return canonical.Value{Kind: canonical.Array, Array: a}
	case string:
		return canonical.Value{Kind: canonical.String, String: x}
	case bool:
		return canonical.Value{Kind: canonical.Boolean, Bool: x}
	case json.Number:
		return canonical.Value{Kind: canonical.Number, TextNumber: string(x)}
	}
	panic("validated value invariant")
}

// direct reads Go fields using the frozen schema, not FromDomain or serialized bytes.
func (s schema) direct(v reflect.Value, typ string) (any, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, fail("unexpected nil")
		}
		v = v.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		a := []any{}
		for i := 0; i < v.Len(); i++ {
			x, e := s.direct(v.Index(i), typ[2:])
			if e != nil {
				return nil, e
			}
			a = append(a, x)
		}
		return a, nil
	}
	if typ == "ID" {
		return map[string]any{"rawValue": v.String()}, nil
	}
	if fs, ok := s.Objects[typ]; ok {
		m := map[string]any{}
		for _, f := range fs {
			q := v.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, f.Wire) })
			if !q.IsValid() {
				return nil, fail("missing Go field:" + f.Wire)
			}
			if f.Optional && q.Kind() == reflect.Pointer && q.IsNil() {
				continue
			}
			x, e := s.direct(q, f.Type)
			if e != nil {
				return nil, e
			}
			m[f.Wire] = x
		}
		return m, nil
	}
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int32, reflect.Int64:
		return json.Number(strconv.FormatInt(v.Int(), 10)), nil
	case reflect.Uint, reflect.Uint32, reflect.Uint64:
		return json.Number(strconv.FormatUint(v.Uint(), 10)), nil
	}
	return nil, fmt.Errorf("unsupported direct type %s", typ)
}
func (s schema) project(g core.TaskGraph, extra v7.Supplement) ([]byte, error) {
	m := map[string]any{}
	for _, q := range []struct {
		name, typ string
		v         any
	}{{"days", "[]Day", g.Days}, {"chains", "[]Chain", g.Chains}, {"definitions", "[]Definition", g.Definitions}, {"traces", "[]Trace", g.Traces}, {"subtasks", "[]Subtask", g.Subtasks}} {
		x, e := s.direct(reflect.ValueOf(q.v), q.typ)
		if e != nil {
			return nil, e
		}
		m[q.name] = x
	}
	m["taskCycleSeries"] = []any{}
	for _, q := range []struct {
		name string
		v    canonical.Value
	}{{"preferences", extra.Preferences}, {"classifications", extra.Classifications}, {"ideas", extra.Ideas}} {
		b, e := canonical.Marshal(q.v)
		if e != nil {
			return nil, e
		}
		x, e := parseBounded(b, rawLimits)
		if e != nil {
			return nil, e
		}
		m[q.name] = x
	}
	v := map[string]any{"formatVersion": json.Number("7"), "snapshot": m}
	if e := s.validate(v, "Envelope"); e != nil {
		return nil, e
	}
	if e := budget(v, 0, 0); e != nil {
		return nil, e
	}
	return canonical.Marshal(cv(v))
}
func budget(v any, extraNodes, extraText int) error {
	nodes, text := extraNodes, extraText
	var walk func(any)
	walk = func(x any) {
		nodes++
		switch q := x.(type) {
		case string:
			text += len(q)
		case map[string]any:
			for k, y := range q {
				nodes++
				text += len(k)
				walk(y)
			}
		case []any:
			for _, y := range q {
				walk(y)
			}
		}
	}
	walk(v)
	if nodes > 8192 || text > 65536 || 128*nodes+6*text+2 > 262144 {
		return fail("resource-limit:predicted-output")
	}
	return nil
}
