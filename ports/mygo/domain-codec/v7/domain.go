// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Supplement preserves the v7 sections which are not represented by the first
// two domain batches. These are NOT the extensions classification.Snapshot.
type Supplement struct{ Preferences, Classifications, Ideas canonical.Value }

// FromDomain creates all nine wire sections without losing imported supplements.
// Encode still performs schema, graph, reference and classification audit checks.
func FromDomain(s recurrence.State, extra Supplement) (canonical.Value, error) {
	total := 0
	for _, p := range s.Series {
		n := len(p.PlanRevisions) + len(p.ClassificationRevisions)
		if n > MaximumRevisionsPerSeries || total > MaximumTotalRevisions-n {
			return canonical.Value{}, ErrResourceLimit
		}
		total += n
	}

	if err := s.Validate(); err != nil {
		return canonical.Value{}, err
	}
	for _, v := range []canonical.Value{extra.Preferences, extra.Classifications, extra.Ideas} {
		if _, e := canonical.Marshal(v); e != nil {
			return canonical.Value{}, e
		}
	}
	out := canonical.Value{Kind: canonical.Object, Object: map[string]canonical.Value{}}
	sources := map[string]struct {
		value any
		typ   string
	}{
		"days": {s.Tasks.Days, "[]Day"}, "chains": {s.Tasks.Chains, "[]Chain"}, "definitions": {s.Tasks.Definitions, "[]Definition"}, "traces": {s.Tasks.Traces, "[]Trace"}, "subtasks": {s.Tasks.Subtasks, "[]Subtask"}, "taskCycleSeries": {s.Series, "[]Series"},
	}
	for key, source := range sources {
		value, e := encodeTyped(reflect.ValueOf(source.value), source.typ)
		if e != nil {
			return canonical.Value{}, e
		}
		out.Object[key] = value
	}
	out.Object["preferences"] = clone(extra.Preferences)
	out.Object["classifications"] = clone(extra.Classifications)
	out.Object["ideas"] = clone(extra.Ideas)
	if err := validateType(out, "Snapshot", "snapshot"); err != nil {
		return canonical.Value{}, err
	}
	return out, nil
}

// ToDomain decodes supported task/recurrence facts and returns every remaining
// section intact. It does not authorize replacing a live engine or database.
func ToDomain(snapshot canonical.Value) (recurrence.State, Supplement, error) {
	var state recurrence.State
	if _, e := canonical.Marshal(snapshot); e != nil {
		return state, Supplement{}, e
	}
	if err := validateType(snapshot, "Snapshot", "snapshot"); err != nil {
		return state, Supplement{}, err
	}
	if err := validateWorkBudget(snapshot); err != nil {
		return state, Supplement{}, err
	}
	destinations := map[string]struct {
		value any
		typ   string
	}{
		"days": {&state.Tasks.Days, "[]Day"}, "chains": {&state.Tasks.Chains, "[]Chain"}, "definitions": {&state.Tasks.Definitions, "[]Definition"}, "traces": {&state.Tasks.Traces, "[]Trace"}, "subtasks": {&state.Tasks.Subtasks, "[]Subtask"}, "taskCycleSeries": {&state.Series, "[]Series"},
	}
	for key, destination := range destinations {
		if e := decodeTyped(snapshot.Object[key], reflect.ValueOf(destination.value).Elem(), destination.typ); e != nil {
			return recurrence.State{}, Supplement{}, fmt.Errorf("%s: %w", key, e)
		}
	}
	if e := state.Validate(); e != nil {
		return recurrence.State{}, Supplement{}, e
	}
	extra := Supplement{clone(snapshot.Object["preferences"]), clone(snapshot.Object["classifications"]), clone(snapshot.Object["ideas"])}
	return state, extra, nil
}
func stringValue(s string) canonical.Value { return canonical.Value{Kind: canonical.String, String: s} }
func numberValue(s string) canonical.Value {
	return canonical.Value{Kind: canonical.Number, TextNumber: s}
}
func objectValue(m map[string]canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Object, Object: m}
}
func fieldValue(v reflect.Value, name string) reflect.Value {
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if strings.EqualFold(typ.Field(i).Name, name) {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}
func encodeTyped(v reflect.Value, typ string) (canonical.Value, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return canonical.Value{}, ErrMalformed
		}
		v = v.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		out := canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}
		for i := 0; i < v.Len(); i++ {
			x, e := encodeTyped(v.Index(i), typ[2:])
			if e != nil {
				return out, e
			}
			out.Array = append(out.Array, x)
		}
		return out, nil
	}
	switch typ {
	case "ID":
		return objectValue(map[string]canonical.Value{"rawValue": stringValue(v.String())}), nil
	case "Schedule":
		s := v.Interface().(recurrence.Schedule)
		if s.Kind == recurrence.EveryDays {
			return tagged("everyDays", map[string]canonical.Value{"_0": numberValue(strconv.Itoa(s.DayInterval))}), nil
		}
		days, e := encodeTyped(reflect.ValueOf(s.Weekdays), "[]int")
		if e != nil {
			return canonical.Value{}, e
		}
		return tagged("selectedWeekdays", map[string]canonical.Value{"weekdays": days, "everyWeeks": numberValue(strconv.Itoa(s.EveryWeeks))}), nil
	case "EndCondition":
		s := v.Interface().(recurrence.EndCondition)
		switch s.Kind {
		case recurrence.DurationDays:
			return tagged("durationDays", map[string]canonical.Value{"_0": numberValue(strconv.Itoa(s.Days))}), nil
		case recurrence.OnDate:
			d, e := encodeTyped(reflect.ValueOf(s.Date), "LocalDate")
			return tagged("onDate", map[string]canonical.Value{"_0": d}), e
		case recurrence.CompletionTarget:
			d, e := encodeTyped(reflect.ValueOf(s.LatestDate), "LocalDate")
			return tagged("completionTarget", map[string]canonical.Value{"count": numberValue(strconv.Itoa(s.Count)), "latestDate": d}), e
		}
		return canonical.Value{}, ErrMalformed
	case "Source":
		s := v.Interface().(classification.Source)
		m := map[string]canonical.Value{}
		fs, ok := unions["Source"][string(s.Kind)]
		if !ok {
			return canonical.Value{}, ErrMalformed
		}
		for key, t := range fs {
			x, e := encodeTyped(fieldValue(v, key), t)
			if e != nil {
				return canonical.Value{}, e
			}
			m[key] = x
		}
		return tagged(string(s.Kind), m), nil
	case "CancellationFact":
		f := v.Interface().(recurrence.CancellationFact)
		date, e := encodeTyped(reflect.ValueOf(f.Date), "LocalDate")
		if e != nil {
			return canonical.Value{}, e
		}
		key := "_0"
		if f.Scope == recurrence.FollowingDates {
			key = "after"
		}
		return objectValue(map[string]canonical.Value{"id": stringValue(f.ID), "scope": tagged(string(f.Scope), map[string]canonical.Value{key: date}), "reason": stringValue(string(f.Reason)), "recordedAt": numberValue(strconv.FormatUint(uint64(f.RecordedAt), 10))}), nil
	}
	if fs, ok := objects[typ]; ok {
		out := map[string]canonical.Value{}
		for key, t := range fs {
			optional := strings.HasSuffix(key, "?")
			key = strings.TrimSuffix(key, "?")
			field := fieldValue(v, key)
			if !field.IsValid() {
				return canonical.Value{}, fmt.Errorf("%w: missing Go adapter field %s.%s", ErrMalformed, typ, key)
			}
			if optional && field.Kind() == reflect.Pointer && field.IsNil() {
				continue
			}
			x, e := encodeTyped(field, t)
			if e != nil {
				return canonical.Value{}, e
			}
			out[key] = x
		}
		return objectValue(out), nil
	}
	switch v.Kind() {
	case reflect.String:
		return stringValue(v.String()), nil
	case reflect.Int, reflect.Int64:
		return numberValue(strconv.FormatInt(v.Int(), 10)), nil
	case reflect.Uint64:
		return numberValue(strconv.FormatUint(v.Uint(), 10)), nil
	case reflect.Bool:
		return canonical.Value{Kind: canonical.Boolean, Bool: v.Bool()}, nil
	}
	return canonical.Value{}, fmt.Errorf("%w: unsupported Go adapter %s", ErrMalformed, typ)
}
func tagged(name string, fields map[string]canonical.Value) canonical.Value {
	return objectValue(map[string]canonical.Value{name: objectValue(fields)})
}
func decodeTyped(value canonical.Value, target reflect.Value, typ string) error {
	if target.Kind() == reflect.Pointer {
		target.Set(reflect.New(target.Type().Elem()))
		target = target.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		target.Set(reflect.MakeSlice(target.Type(), len(value.Array), len(value.Array)))
		for i, v := range value.Array {
			if e := decodeTyped(v, target.Index(i), typ[2:]); e != nil {
				return e
			}
		}
		return nil
	}
	switch typ {
	case "ID":
		target.SetString(strings.ToUpper(value.Object["rawValue"].String))
		return nil
	case "Schedule":
		var out recurrence.Schedule
		for tag, v := range value.Object {
			out.Kind = recurrence.ScheduleKind(tag)
			if out.Kind == recurrence.EveryDays {
				out.DayInterval = int(signed(v.Object["_0"]))
			} else {
				out.EveryWeeks = int(signed(v.Object["everyWeeks"]))
				for _, d := range v.Object["weekdays"].Array {
					out.Weekdays = append(out.Weekdays, int(signed(d)))
				}
			}
		}
		target.Set(reflect.ValueOf(out))
		return nil
	case "EndCondition":
		var out recurrence.EndCondition
		for tag, v := range value.Object {
			out.Kind = recurrence.EndKind(tag)
			switch out.Kind {
			case recurrence.DurationDays:
				out.Days = int(signed(v.Object["_0"]))
			case recurrence.OnDate:
				out.Date = localDate(v.Object["_0"])
			case recurrence.CompletionTarget:
				out.Count = int(signed(v.Object["count"]))
				out.LatestDate = localDate(v.Object["latestDate"])
			}
		}
		target.Set(reflect.ValueOf(out))
		return nil
	case "Source":
		var out classification.Source
		for tag, v := range value.Object {
			out.Kind = classification.SourceKind(tag)
			r := reflect.ValueOf(&out).Elem()
			for key, t := range unions["Source"][tag] {
				if e := decodeTyped(v.Object[key], fieldValue(r, key), t); e != nil {
					return e
				}
			}
		}
		target.Set(reflect.ValueOf(out))
		return nil
	case "CancellationFact":
		out := recurrence.CancellationFact{ID: strings.ToUpper(value.Object["id"].String), Reason: recurrence.CancellationReason(value.Object["reason"].String), RecordedAt: core.Instant(unsigned(value.Object["recordedAt"]))}
		for tag, v := range value.Object["scope"].Object {
			out.Scope = recurrence.CancellationScope(tag)
			key := "_0"
			if out.Scope == recurrence.FollowingDates {
				key = "after"
			}
			out.Date = localDate(v.Object[key])
		}
		target.Set(reflect.ValueOf(out))
		return nil
	}
	if fs, ok := objects[typ]; ok {
		for key, t := range fs {
			key = strings.TrimSuffix(key, "?")
			v, exists := value.Object[key]
			if !exists {
				continue
			}
			field := fieldValue(target, key)
			if !field.IsValid() {
				return fmt.Errorf("%w: missing Go adapter field %s.%s", ErrMalformed, typ, key)
			}
			if e := decodeTyped(v, field, t); e != nil {
				return e
			}
		}
		return nil
	}
	switch target.Kind() {
	case reflect.String:
		if typ == "uuid" {
			target.SetString(strings.ToUpper(value.String))
		} else {
			target.SetString(value.String)
		}
		return nil
	case reflect.Int, reflect.Int64:
		n := signed(value)
		if target.OverflowInt(n) {
			return ErrMalformed
		}
		target.SetInt(n)
		return nil
	case reflect.Uint64:
		target.SetUint(unsigned(value))
		return nil
	case reflect.Bool:
		target.SetBool(value.Bool)
		return nil
	}
	return fmt.Errorf("%w: unsupported Go adapter %s", ErrMalformed, typ)
}
func clone(v canonical.Value) canonical.Value {
	out := v
	if v.Object != nil {
		out.Object = make(map[string]canonical.Value, len(v.Object))
		for k, c := range v.Object {
			out.Object[k] = clone(c)
		}
	}
	if v.Array != nil {
		out.Array = make([]canonical.Value, len(v.Array))
		for i, c := range v.Array {
			out.Array[i] = clone(c)
		}
	}
	return out
}
