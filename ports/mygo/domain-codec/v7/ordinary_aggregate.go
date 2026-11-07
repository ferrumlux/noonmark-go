// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// OrdinaryAggregate is an isolated validated document, not a Service owner or
// permission to append. The original full wire is its lossless source of truth.
// No mutable facts or arbitrary audit callbacks are retained from the caller.
type OrdinaryAggregate struct {
	wire   canonical.Value
	limits canonical.Limits
	valid  bool
}

// DecodeOrdinaryAggregate checks the complete supported ordinary source state.
// Runtime terminal-capture completeness is an additional taskapp profile check.
// It does not change Codec.Decode or the frozen R4 candidate/proof paths.
func DecodeOrdinaryAggregate(data []byte, limits canonical.Limits) (OrdinaryAggregate, error) {
	l, err := ordinaryWireLimits(limits)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	if len(data) > (l.MaxBytes-1)/3 {
		return OrdinaryAggregate{}, ordinary.ErrResourceLimit
	}
	root, err := canonical.Parse(data, l)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	remaining, raw, err := ordinaryWirePreflight(root, l)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	if !bytes.Equal(raw, data) {
		return OrdinaryAggregate{}, canonical.ErrNonCanonical
	}
	if err = validateType(root, "Envelope", "$"); err != nil {
		return OrdinaryAggregate{}, err
	}
	if signed(root.Object["formatVersion"]) != CurrentFormatVersion {
		return OrdinaryAggregate{}, ErrUnsupportedVersion
	}
	snapshot := root.Object["snapshot"]
	if err = ordinaryAggregateScope(snapshot); err != nil {
		return OrdinaryAggregate{}, err
	}
	// Reserve schema-directed typed allocations BEFORE any reflection conversion.
	// The wire preflight already charged nodes, strings and bounded canonical copies.
	// A's validator independently reserves normalization, indexing and digest work.
	graph, state, err := ordinaryAggregateProjection(snapshot)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	if err = core.ValidateTaskGraph(graph); err != nil {
		return OrdinaryAggregate{}, err
	}
	if err = ordinary.ValidateAggregate(state, remaining); err != nil {
		return OrdinaryAggregate{}, err
	}
	if err = ordinary.ValidateAggregateReferences(state, graph, remaining); err != nil {
		return OrdinaryAggregate{}, err
	}
	return OrdinaryAggregate{wire: snapshot, limits: l, valid: true}, nil
}

// EncodeOrdinaryAggregate emits the exact retained canonical representation,
// including dictionary order, optional absence, UUID spelling and Instant bits.
// It reuses exactly the decode contract, so successful output is cold-decodable.
func EncodeOrdinaryAggregate(d OrdinaryAggregate) ([]byte, error) {
	if !d.valid {
		return nil, ordinary.ErrInvalid
	}
	root := objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": d.wire})
	_, raw, err := ordinaryWirePreflight(root, d.limits)
	if err != nil {
		return nil, err
	}
	if _, err = DecodeOrdinaryAggregate(raw, d.limits); err != nil {
		return nil, err
	}
	return raw, nil
}

// Snapshot returns an independent full-wire copy, never write authority.
func (d OrdinaryAggregate) Snapshot() (canonical.Value, error) {
	if !d.valid {
		return canonical.Value{}, ordinary.ErrInvalid
	}
	return clone(d.wire), nil
}

// Facts returns fresh projections. Changing them cannot change the document.
func (d OrdinaryAggregate) Facts() (core.TaskGraph, ordinary.AggregateState, error) {
	if !d.valid {
		return core.TaskGraph{}, ordinary.AggregateState{}, ordinary.ErrInvalid
	}
	return ordinaryAggregateProjection(d.wire)
}

func ordinaryAggregateProjection(s canonical.Value) (core.TaskGraph, ordinary.AggregateState, error) {
	var g core.TaskGraph
	var a ordinary.AggregateState
	for _, f := range []struct {
		key, typ string
		target   any
	}{
		{"days", "[]Day", &g.Days}, {"chains", "[]Chain", &g.Chains}, {"definitions", "[]Definition", &g.Definitions}, {"traces", "[]Trace", &g.Traces}, {"subtasks", "[]Subtask", &g.Subtasks},
	} {
		if err := ordinaryAggregateDecode(s.Object[f.key], reflect.ValueOf(f.target).Elem(), f.typ); err != nil {
			return g, a, err
		}
	}
	c := s.Object["classifications"]
	if err := ordinaryAggregateDecode(c, reflect.ValueOf(&a).Elem(), "Classifications"); err != nil {
		return g, a, err
	}
	for _, f := range []struct {
		key, typ string
		target   any
	}{
		{"snapshotsByTraceID", "pairs:ID:TraceSnapshot", &a.SnapshotsByTraceID},
		{"snapshotEventsByTraceID", "pairs:ID:[]TraceSnapshot", &a.SnapshotEventsByTraceID},
	} {
		if err := ordinaryAggregateDecode(c.Object[f.key], reflect.ValueOf(f.target).Elem(), f.typ); err != nil {
			return g, a, err
		}
	}
	return g, a, nil
}

func ordinaryAggregateScope(s canonical.Value) error {
	// Full supported source shape, independent of frozen R4 replay scope. Fields
	// outside the ordinary projection are rejected whole, never filtered away.
	if len(s.Object["taskCycleSeries"].Array) != 0 || len(s.Object["ideas"].Array) != 0 {
		return ordinary.ErrUnsupported
	}
	for _, chain := range s.Object["chains"].Array {
		if _, ok := chain.Object["cycleMembership"]; ok {
			return ordinary.ErrUnsupported
		}
	}
	prefs, err := canonical.Marshal(s.Object["preferences"])
	if err != nil {
		return err
	}
	neutral, err := canonical.Parse([]byte(ordinaryNeutralPreferences), canonical.DefaultLimits())
	if err != nil {
		return err
	}
	neutralRaw, err := canonical.Marshal(neutral)
	if err != nil {
		return err
	}
	if !bytes.Equal(prefs, neutralRaw) {
		return ordinary.ErrUnsupported
	}
	c := s.Object["classifications"]
	if signed(c.Object["formatVersion"]) != ordinary.FormatVersion {
		return ErrUnsupportedVersion
	}
	for _, key := range []string{"labels", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones"} {
		if len(c.Object[key].Array) != 0 {
			return ordinary.ErrUnsupported
		}
	}
	for i := 1; i < len(c.Object["currentByChainID"].Array); i += 2 {
		current := c.Object["currentByChainID"].Array[i]
		if len(current.Object["labels"].Array) != 0 {
			return ordinary.ErrUnsupported
		}
		if r, ok := current.Object["category"]; ok {
			if err = ordinaryDirect(r.Object["source"]); err != nil {
				return err
			}
		}
	}
	for _, h := range c.Object["relationHistory"].Array {
		if h.Object["kind"].String != "category" {
			return ordinary.ErrUnsupported
		}
		for _, key := range []string{"originSource", "removedBySource"} {
			if err = ordinaryDirect(h.Object[key]); err != nil {
				return err
			}
		}
	}
	notices := func(ns canonical.Value) error {
		for _, n := range ns.Array {
			v, ok := n.Object["existingItemReused"]
			if !ok || v.Object["kind"].String != "category" {
				return ordinary.ErrUnsupported
			}
		}
		return nil
	}
	for _, r := range c.Object["changeRecords"].Array {
		if err = ordinaryDirect(r.Object["source"]); err != nil {
			return err
		}
		for _, change := range r.Object["changes"].Array {
			v, ok := change.Object["setCurrent"]
			if !ok {
				return ordinary.ErrUnsupported
			}
			for _, side := range []string{"before", "after"} {
				p := v.Object[side]
				if len(p.Object["labels"].Array) != 0 {
					return ordinary.ErrUnsupported
				}
				if item, ok := p.Object["category"]; ok && item.Object["kind"].String != "category" {
					return ordinary.ErrUnsupported
				}
			}
		}
		if err = notices(r.Object["notices"]); err != nil {
			return err
		}
	}
	for i := 1; i < len(c.Object["committedReceiptsByInteractionID"].Array); i += 2 {
		if err = notices(c.Object["committedReceiptsByInteractionID"].Array[i].Object["notices"]); err != nil {
			return err
		}
	}
	for _, key := range []string{"snapshotsByTraceID", "snapshotEventsByTraceID"} {
		pairs := c.Object[key].Array
		for i := 1; i < len(pairs); i += 2 {
			events := []canonical.Value{pairs[i]}
			if key == "snapshotEventsByTraceID" {
				events = pairs[i].Array
			}
			for _, event := range events {
				if len(event.Object["labels"].Array) != 0 {
					return ordinary.ErrUnsupported
				}
			}
		}
	}
	return nil
}

func ordinaryAggregateDecode(v canonical.Value, target reflect.Value, typ string) error {
	if target.Kind() == reflect.Pointer {
		target.Set(reflect.New(target.Type().Elem()))
		target = target.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		target.Set(reflect.MakeSlice(target.Type(), len(v.Array), len(v.Array)))
		for i, item := range v.Array {
			if err := ordinaryAggregateDecode(item, target.Index(i), typ[2:]); err != nil {
				return err
			}
		}
		return nil
	}
	if strings.HasPrefix(typ, "pairs:") {
		parts := strings.SplitN(typ[6:], ":", 2)
		target.Set(reflect.MakeMapWithSize(target.Type(), len(v.Array)/2))
		for i := 0; i < len(v.Array); i += 2 {
			k := reflect.New(target.Type().Key()).Elem()
			value := reflect.New(target.Type().Elem()).Elem()
			if err := ordinaryAggregateDecode(v.Array[i], k, parts[0]); err != nil {
				return err
			}
			if err := ordinaryAggregateDecode(v.Array[i+1], value, parts[1]); err != nil {
				return err
			}
			target.SetMapIndex(k, value)
		}
		return nil
	}
	switch typ {
	case "uuid":
		target.SetString(strings.ToUpper(v.String))
		return nil
	case "ID":
		target.SetString(strings.ToUpper(v.Object["rawValue"].String))
		return nil
	case "Source":
		target.SetString("userDirect")
		return nil
	case "PlanChange":
		target.FieldByName("Kind").SetString("setCurrent")
		return ordinaryAggregateDecodeFields(v.Object["setCurrent"], target, unions["PlanChange"]["setCurrent"], typ)
	case "Notice":
		target.FieldByName("Kind").SetString("existingItemReused")
		n := v.Object["existingItemReused"]
		target.FieldByName("ItemKind").SetString(n.Object["kind"].String)
		for _, key := range []string{"inputName", "currentName", "matchedHistoricalAlias"} {
			if err := ordinaryAggregateDecode(n.Object[key], fieldValue(target, key), unions["Notice"]["existingItemReused"][key]); err != nil {
				return err
			}
		}
		return nil
	}
	if fs, ok := objects[typ]; ok {
		return ordinaryAggregateDecodeFields(v, target, fs, typ)
	}
	switch target.Kind() {
	case reflect.String:
		target.SetString(v.String)
	case reflect.Int, reflect.Int64:
		n := signed(v)
		if target.OverflowInt(n) {
			return ErrMalformed
		}
		target.SetInt(n)
	case reflect.Uint64:
		target.SetUint(unsigned(v))
	case reflect.Bool:
		target.SetBool(v.Bool)
	default:
		return fmt.Errorf("%w: ordinary adapter type %s", ErrMalformed, typ)
	}
	return nil
}
func ordinaryAggregateDecodeFields(v canonical.Value, target reflect.Value, fs fields, typ string) error {
	for key, t := range fs {
		key = strings.TrimSuffix(key, "?")
		if ordinaryEmptyField(typ, key) {
			continue
		}
		child, ok := v.Object[key]
		if !ok {
			continue
		}
		field := fieldValue(target, key)
		if !field.IsValid() {
			return ErrMalformed
		}
		if err := ordinaryAggregateDecode(child, field, t); err != nil {
			return err
		}
	}
	return nil
}

// ImportOrdinaryAggregate is the bounded AST counterpart. It validates the full
// candidate and does not authorize mutation or weaken the default codec gate.
// A transition must preserve unchanged raw fields in this complete wire itself.
func ImportOrdinaryAggregate(snapshot canonical.Value, limits canonical.Limits) (OrdinaryAggregate, error) {
	l, err := ordinaryWireLimits(limits)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	root := objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": snapshot})
	_, raw, err := ordinaryWirePreflight(root, l)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	return DecodeOrdinaryAggregate(raw, l)
}

// WithOrdinaryFacts builds a full, validated candidate without granting write
// authority. Unchanged typed fields retain their exact original wire values.
// Existing dictionary entries keep original order/key spelling; new keys append
// in deterministic typed order. No event is inferred from arbitrary task facts.
func WithOrdinaryFacts(prior OrdinaryAggregate, graph core.TaskGraph, state ordinary.AggregateState) (OrdinaryAggregate, error) {
	if !prior.valid {
		return OrdinaryAggregate{}, ordinary.ErrInvalid
	}
	// Bound caller-owned typed values before validation, reflection encoding,
	// normalization, allocation or copying. Includes prospective escaped output,
	// old/new typed encodings, merge output and per-node bookkeeping.
	if err := ordinaryAggregateTypedPreflight(graph, state, prior.limits); err != nil {
		return OrdinaryAggregate{}, err
	}
	limits := ordinary.Limits{MaxBytes: prior.limits.MaxBytes, MaxDepth: prior.limits.MaxDepth, MaxNodes: prior.limits.MaxNodes}
	if err := ordinary.ValidateAggregate(state, limits); err != nil {
		return OrdinaryAggregate{}, err
	}
	if err := ordinary.ValidateAggregateReferences(state, graph, limits); err != nil {
		return OrdinaryAggregate{}, err
	}
	if err := core.ValidateTaskGraph(graph); err != nil {
		return OrdinaryAggregate{}, err
	}
	beforeGraph, beforeState, err := prior.Facts()
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	before, err := ordinaryAggregateEncodeFacts(beforeGraph, beforeState)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	after, err := ordinaryAggregateEncodeFacts(graph, state)
	if err != nil {
		return OrdinaryAggregate{}, err
	}
	merged := ordinaryAggregateOverlay(prior.wire, before, after, "Snapshot")
	return ImportOrdinaryAggregate(merged, prior.limits)
}

func ordinaryAggregateTypedPreflight(g core.TaskGraph, s ordinary.AggregateState, l canonical.Limits) error {
	bytesLeft, nodesLeft := l.MaxBytes, l.MaxNodes
	var visit func(reflect.Value, int) error
	visit = func(v reflect.Value, depth int) error {
		if depth > l.MaxDepth || nodesLeft <= 0 || bytesLeft < 128 {
			return ordinary.ErrResourceLimit
		}
		nodesLeft--
		bytesLeft -= 128
		switch v.Kind() {
		case reflect.String:
			if v.Len() > bytesLeft/24 {
				return ordinary.ErrResourceLimit
			}
			bytesLeft -= v.Len() * 24
		case reflect.Pointer:
			if !v.IsNil() {
				return visit(v.Elem(), depth+1)
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if e := visit(v.Field(i), depth+1); e != nil {
					return e
				}
			}
		case reflect.Slice, reflect.Array:
			if v.Len() > nodesLeft {
				return ordinary.ErrResourceLimit
			}
			for i := 0; i < v.Len(); i++ {
				if e := visit(v.Index(i), depth+1); e != nil {
					return e
				}
			}
		case reflect.Map:
			if v.Len() > nodesLeft/2 {
				return ordinary.ErrResourceLimit
			}
			iter := v.MapRange()
			for iter.Next() {
				if e := visit(iter.Key(), depth+1); e != nil {
					return e
				}
				if e := visit(iter.Value(), depth+1); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := visit(reflect.ValueOf(g), 1); e != nil {
		return e
	}
	return visit(reflect.ValueOf(s), 1)
}

func ordinaryAggregateEncodeFacts(g core.TaskGraph, s ordinary.AggregateState) (canonical.Value, error) {
	out := objectValue(map[string]canonical.Value{})
	for _, f := range []struct {
		key, typ string
		v        any
	}{{"days", "[]Day", g.Days}, {"chains", "[]Chain", g.Chains}, {"definitions", "[]Definition", g.Definitions}, {"traces", "[]Trace", g.Traces}, {"subtasks", "[]Subtask", g.Subtasks}, {"classifications", "Classifications", s}} {
		v, e := ordinaryEncode(reflect.ValueOf(f.v), f.typ)
		if e != nil {
			return canonical.Value{}, e
		}
		out.Object[f.key] = v
	}
	for _, f := range []struct {
		key, typ string
		v        any
	}{{"snapshotsByTraceID", "pairs:ID:TraceSnapshot", s.SnapshotsByTraceID}, {"snapshotEventsByTraceID", "pairs:ID:[]TraceSnapshot", s.SnapshotEventsByTraceID}} {
		v, e := ordinaryEncode(reflect.ValueOf(f.v), f.typ)
		if e != nil {
			return canonical.Value{}, e
		}
		out.Object["classifications"].Object[f.key] = v
	}
	return out, nil
}

func ordinaryAggregateOverlay(raw, before, after canonical.Value, typ string) canonical.Value {
	if reflect.DeepEqual(before, after) {
		return raw
	}
	if strings.HasPrefix(typ, "pairs:") {
		parts := strings.SplitN(typ[6:], ":", 2)
		key := func(v canonical.Value) string {
			if parts[0] == "ID" {
				return wireID(v)
			}
			return strings.ToUpper(v.String)
		}
		bm, am := map[string]canonical.Value{}, map[string]canonical.Value{}
		for i := 0; i < len(before.Array); i += 2 {
			bm[key(before.Array[i])] = before.Array[i+1]
		}
		for i := 0; i < len(after.Array); i += 2 {
			am[key(after.Array[i])] = after.Array[i+1]
		}
		out := ordinaryArray()
		seen := map[string]bool{}
		for i := 0; i < len(raw.Array); i += 2 {
			id := key(raw.Array[i])
			a, ok := am[id]
			if !ok {
				continue
			}
			seen[id] = true
			out.Array = append(out.Array, raw.Array[i], ordinaryAggregateOverlay(raw.Array[i+1], bm[id], a, parts[1]))
		}
		for i := 0; i < len(after.Array); i += 2 {
			if !seen[key(after.Array[i])] {
				out.Array = append(out.Array, after.Array[i], after.Array[i+1])
			}
		}
		return out
	}
	if strings.HasPrefix(typ, "[]") {
		elementType := typ[2:]
		// Entity order is not entity identity. Both views are indexed independently
		// so a moved/inserted/deleted item can never inherit another item's raw leaves.
		if idType := objects[elementType]["id"]; idType == "ID" || idType == "uuid" {
			key := func(v canonical.Value) string {
				if idType == "ID" {
					return wireID(v.Object["id"])
				}
				return strings.ToUpper(v.Object["id"].String)
			}
			bm, rm := make(map[string]canonical.Value, len(before.Array)), make(map[string]canonical.Value, len(raw.Array))
			for _, v := range before.Array {
				bm[key(v)] = v
			}
			for _, v := range raw.Array {
				rm[key(v)] = v
			}
			out := ordinaryArray()
			for _, a := range after.Array {
				id := key(a)
				b, bok := bm[id]
				r, rok := rm[id]
				if bok && rok {
					out.Array = append(out.Array, ordinaryAggregateOverlay(r, b, a, elementType))
				} else {
					out.Array = append(out.Array, a)
				}
			}
			return out
		}
		return ordinaryAggregateSequenceOverlay(raw, before, after, elementType)
	}
	if fs, ok := objects[typ]; ok {
		out := objectValue(map[string]canonical.Value{})
		for k, v := range raw.Object {
			out.Object[k] = v
		}
		for name, subtyp := range fs {
			k := strings.TrimSuffix(name, "?")
			a, aok := after.Object[k]
			b, bok := before.Object[k]
			// Sections outside the typed projection (preferences/ideas/series) stay raw.
			if !aok && !bok {
				continue
			}
			if !aok {
				delete(out.Object, k)
				continue
			}
			if !bok {
				out.Object[k] = a
				continue
			}
			out.Object[k] = ordinaryAggregateOverlay(raw.Object[k], b, a, subtyp)
		}
		return out
	}
	if cases, ok := unions[typ]; ok && len(before.Object) == 1 && len(after.Object) == 1 {
		for tag, a := range after.Object {
			if b, ok := before.Object[tag]; ok {
				fields := cases[tag]
				body := objectValue(map[string]canonical.Value{})
				for name, subtyp := range fields {
					k := strings.TrimSuffix(name, "?")
					av, aok := a.Object[k]
					bv, bok := b.Object[k]
					if !aok {
						continue
					}
					if !bok {
						body.Object[k] = av
					} else {
						body.Object[k] = ordinaryAggregateOverlay(raw.Object[tag].Object[k], bv, av, subtyp)
					}
				}
				return objectValue(map[string]canonical.Value{tag: body})
			}
		}
	}
	return after
}

// Anonymous sequences have no invented positional identity. Match unchanged
// typed elements first, including their occurrence order for duplicate values.
// A changed setCurrent element may match only another operation on that chain.
// All indices are built after WithOrdinaryFacts' typed budget and validators.
func ordinaryAggregateSequenceOverlay(raw, before, after canonical.Value, typ string) canonical.Value {
	type entry struct {
		before, raw canonical.Value
		used        bool
	}
	entries := make([]entry, len(before.Array))
	exact := make(map[string][]int, len(before.Array))
	exactNext := make(map[string]int)
	chains := make(map[string][]int)
	fingerprint := func(v canonical.Value) (string, bool) {
		b, e := canonical.Marshal(v)
		if e != nil {
			return "", false
		}
		return string(b), true
	}
	for i, b := range before.Array {
		if i >= len(raw.Array) {
			continue
		}
		entries[i] = entry{before: b, raw: raw.Array[i]}
		if h, ok := fingerprint(b); ok {
			exact[h] = append(exact[h], i)
		}
		if typ == "PlanChange" {
			id := wireID(b.Object["setCurrent"].Object["chainID"])
			chains[id] = append(chains[id], i)
		}
	}
	// Reserve exact matches before matching changed operations. This prevents an
	// insertion from stealing the raw occurrence of a later unchanged operation.
	matched := make([]int, len(after.Array))
	for i := range matched {
		matched[i] = -1
	}
	for i, a := range after.Array {
		if h, ok := fingerprint(a); ok {
			q := exact[h]
			n := exactNext[h]
			if n < len(q) {
				j := q[n]
				exactNext[h] = n + 1
				matched[i] = j
				entries[j].used = true
			}
		}
	}
	if typ == "PlanChange" {
		next := make(map[string]int)
		for i, a := range after.Array {
			if matched[i] >= 0 {
				continue
			}
			id := wireID(a.Object["setCurrent"].Object["chainID"])
			q := chains[id]
			n := next[id]
			for n < len(q) && entries[q[n]].used {
				n++
			}
			if n < len(q) {
				j := q[n]
				n++
				matched[i] = j
				entries[j].used = true
			}
			next[id] = n
		}
	}
	out := ordinaryArray()
	for i, a := range after.Array {
		if j := matched[i]; j >= 0 {
			out.Array = append(out.Array, ordinaryAggregateOverlay(entries[j].raw, entries[j].before, a, typ))
		} else {
			out.Array = append(out.Array, a)
		}
	}
	return out
}
