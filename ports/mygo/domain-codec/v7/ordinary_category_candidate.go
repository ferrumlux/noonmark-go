// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// OrdinaryCategoryCandidate is an offline imported structural candidate. Its
// mutable State is NOT a ReplayProof, a storage document, or write authority.
// Context proves only the fixed ordinary graph, not its task-command history.
// Only ordinary.Replay/Continue can establish classification causal reachability.
type OrdinaryCategoryCandidate struct {
	Context ordinary.OrdinaryContext
	State   ordinary.State
}

// DecodeOrdinaryCategoryCandidate accepts only canonical candidate-v7 bytes.
// It deliberately does not call Codec.Decode, register an AuditValidator, or
// change any product reader's existing empty-catalog admission boundary.
func DecodeOrdinaryCategoryCandidate(data []byte, limits canonical.Limits) (OrdinaryCategoryCandidate, error) {
	limits, err := ordinaryWireLimits(limits)
	if err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	// Reserve the input text and two wire-sized logical copies before parsing.
	if len(data) > (limits.MaxBytes-1)/3 {
		return OrdinaryCategoryCandidate{}, ordinary.ErrResourceLimit
	}
	root, err := canonical.Parse(data, limits)
	if err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	return importOrdinaryEnvelope(root, limits, data)
}

// ImportOrdinaryCategoryCandidate rejects unsupported full-snapshot sections
// BEFORE projection. Canonical's AST cycle/byte/node/depth checks run before any
// schema walk, clone, model validation, or graph indexing. No supplement is lost.
func ImportOrdinaryCategoryCandidate(snapshot canonical.Value, limits canonical.Limits) (OrdinaryCategoryCandidate, error) {
	limits, err := ordinaryWireLimits(limits)
	if err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	return importOrdinaryEnvelope(objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": snapshot}), limits, nil)
}

func importOrdinaryEnvelope(root canonical.Value, limits canonical.Limits, original []byte) (OrdinaryCategoryCandidate, error) {
	remaining, encoded, err := ordinaryWirePreflight(root, limits)
	if err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	if original != nil && !bytes.Equal(encoded, original) {
		return OrdinaryCategoryCandidate{}, canonical.ErrNonCanonical
	}
	if err = validateType(root, "Envelope", "$"); err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	if signed(root.Object["formatVersion"]) != CurrentFormatVersion {
		return OrdinaryCategoryCandidate{}, ErrUnsupportedVersion
	}
	snapshot := root.Object["snapshot"]
	if err = ordinaryWireScope(snapshot); err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	var graph core.TaskGraph
	for _, field := range []struct {
		key, typ string
		target   any
	}{
		{"days", "[]Day", &graph.Days}, {"chains", "[]Chain", &graph.Chains}, {"definitions", "[]Definition", &graph.Definitions}, {"traces", "[]Trace", &graph.Traces}, {"subtasks", "[]Subtask", &graph.Subtasks},
	} {
		if err = ordinaryDecode(snapshot.Object[field.key], reflect.ValueOf(field.target).Elem(), field.typ); err != nil {
			return OrdinaryCategoryCandidate{}, err
		}
	}
	var state ordinary.State
	if err = ordinaryDecode(snapshot.Object["classifications"], reflect.ValueOf(&state).Elem(), "Classifications"); err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	// This entry includes integrity and entity decoding in ONE model budget. It
	// does not invent a plan transcript from final-state records/digest strings.
	ctx, state, err := ordinary.ImportCandidateWithLimits(graph, state, remaining)
	if err != nil {
		return OrdinaryCategoryCandidate{}, err
	}
	return OrdinaryCategoryCandidate{ctx, state}, nil
}

// ExportOrdinaryCategoryCandidate returns a round-trip-safe offline AST, not a
// Codec Document or a general Swift encode operation. Integrity-valid entities
// that fail the separate entity decoder are deliberately outside this subset.
// Model-owned preflight reserves graph+state+copies+encoder output before clones.
// The exact strings, ID wrappers, UInt64 Instant bits and optional omissions are
// retained; pair dictionaries are deterministically ordered by raw UUID bytes.
func ExportOrdinaryCategoryCandidate(ctx ordinary.OrdinaryContext, state ordinary.State) (canonical.Value, error) {
	graph, state, err := ordinary.ExportCandidate(ctx, state)
	if err != nil {
		return canonical.Value{}, err
	}
	out := map[string]canonical.Value{}
	for _, field := range []struct {
		key, typ string
		value    any
	}{
		{"days", "[]Day", graph.Days}, {"chains", "[]Chain", graph.Chains}, {"definitions", "[]Definition", graph.Definitions}, {"traces", "[]Trace", graph.Traces}, {"subtasks", "[]Subtask", graph.Subtasks}, {"classifications", "Classifications", state},
	} {
		v, e := ordinaryEncode(reflect.ValueOf(field.value), field.typ)
		if e != nil {
			return canonical.Value{}, e
		}
		out[field.key] = v
	}
	prefs, err := canonical.Parse([]byte(ordinaryNeutralPreferences), canonical.DefaultLimits())
	if err != nil {
		return canonical.Value{}, err
	}
	out["preferences"], out["taskCycleSeries"], out["ideas"] = prefs, ordinaryArray(), ordinaryArray()
	result := objectValue(out)
	// Verify the actual bounded output after the conservative reservation. Do not
	// use Codec.Encode: the product's nonempty-catalog audit gate remains closed.
	if _, err = canonical.Marshal(result); err != nil {
		return canonical.Value{}, err
	}
	if err = validateType(result, "Snapshot", "snapshot"); err != nil {
		return canonical.Value{}, err
	}
	return result, nil
}

func ordinaryWireLimits(l canonical.Limits) (canonical.Limits, error) {
	d := canonical.DefaultLimits()
	if l.MaxBytes < 0 || l.MaxDepth < 0 || l.MaxNodes < 0 {
		return l, ordinary.ErrResourceLimit
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxDepth == 0 {
		l.MaxDepth = d.MaxDepth
	}
	if l.MaxNodes == 0 {
		l.MaxNodes = d.MaxNodes
	}
	if l.MaxBytes > d.MaxBytes || l.MaxDepth > d.MaxDepth || l.MaxNodes > d.MaxNodes {
		return l, ordinary.ErrResourceLimit
	}
	return l, nil
}
func ordinaryWirePreflight(root canonical.Value, l canonical.Limits) (ordinary.Limits, []byte, error) {
	cost, err := ordinaryWireScan(root, l)
	if err != nil {
		return ordinary.Limits{}, nil, err
	}
	// Reserve three distinct wire-sized copies: original bytes, parsed AST string
	// material, and canonical staging. Reserve per-node AST/schema/projection
	// bookkeeping separately (a conservative logical cost, not an RSS promise).
	// This also over-reserves the AST-only entry. BEFORE Marshal allocates/sorts,
	// the model receives the remainder for actual
	// graph+state input, output, normalization, validation and digest work.
	// Zero means defaults to ordinary.Limits, never an exhausted budget.
	if cost.bytes > (l.MaxBytes-1)/3 {
		return ordinary.Limits{}, nil, ordinary.ErrResourceLimit
	}
	left := l.MaxBytes - 3*cost.bytes
	if cost.nodes > (left-1)/128 || cost.nodes > (l.MaxNodes-1)/2 {
		return ordinary.Limits{}, nil, ordinary.ErrResourceLimit
	}
	remaining := ordinary.Limits{MaxBytes: left - 128*cost.nodes, MaxNodes: l.MaxNodes - 2*cost.nodes, MaxDepth: l.MaxDepth}
	// Keep canonical's original validity/hard-bound checks as well as the stricter
	// preflight, and verify exact byte accounting against its real serializer.
	raw, err := canonical.Marshal(root)
	if err != nil {
		return ordinary.Limits{}, nil, err
	}
	if len(raw) != cost.bytes {
		return ordinary.Limits{}, nil, ErrMalformed
	}
	return remaining, raw, nil
}

// ordinaryWireScan is an allocation-free, early-terminating preflight over the
// original AST. Counts exactly match canonical.Marshal, including pretty-print
// punctuation/indentation, object keys and every repeated alias. Cycles cannot
// recurse past MaxDepth. It never sorts keys or builds a visited index.
type ordinaryWireCost struct{ bytes, nodes int }

func ordinaryWireScan(root canonical.Value, l canonical.Limits) (ordinaryWireCost, error) {
	var cost ordinaryWireCost
	charge := func(n int) error {
		if n < 0 || n > l.MaxBytes-cost.bytes {
			return ordinary.ErrResourceLimit
		}
		cost.bytes += n
		return nil
	}
	quote := func(s string) error {
		if len(s) > l.MaxBytes-cost.bytes {
			return ordinary.ErrResourceLimit
		}
		if err := charge(2); err != nil {
			return err
		}
		for _, r := range s {
			n := utf8.RuneLen(r)
			switch r {
			case '"', '\\', '\b', '\f', '\n', '\r', '\t':
				n = 2
			default:
				if r < 32 {
					n = 6
				}
			}
			if err := charge(n); err != nil {
				return err
			}
		}
		if !utf8.ValidString(s) {
			return ErrMalformed
		}
		return nil
	}
	var visit func(canonical.Value, int) error
	visit = func(v canonical.Value, depth int) error {
		if depth > l.MaxDepth || cost.nodes >= l.MaxNodes {
			return ordinary.ErrResourceLimit
		}
		cost.nodes++
		if (v.Kind != canonical.Object && v.Object != nil) || (v.Kind != canonical.Array && v.Array != nil) || (v.Kind != canonical.String && v.String != "") || (v.Kind != canonical.Number && v.TextNumber != "") || (v.Kind != canonical.Boolean && v.Bool) {
			return ErrMalformed
		}
		switch v.Kind {
		case canonical.Null:
			return charge(4)
		case canonical.Boolean:
			if v.Bool {
				return charge(4)
			}
			return charge(5)
		case canonical.String:
			return quote(v.String)
		case canonical.Number:
			return charge(len(v.TextNumber)) // canonical checks integer spelling next
		case canonical.Object:
			if len(v.Object) > (l.MaxNodes-cost.nodes)/2 {
				return ordinary.ErrResourceLimit
			}
			if err := charge(2); err != nil {
				return err
			}
			i := 0
			for key, child := range v.Object {
				cost.nodes++
				if i > 0 {
					if err := charge(1); err != nil {
						return err
					}
				}
				i++
				if err := charge(1 + 2*depth + 3); err != nil {
					return err
				}
				if err := quote(key); err != nil {
					return err
				}
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
			if len(v.Object) > 0 {
				return charge(1 + 2*(depth-1))
			}
		case canonical.Array:
			if len(v.Array) > l.MaxNodes-cost.nodes {
				return ordinary.ErrResourceLimit
			}
			if err := charge(2); err != nil {
				return err
			}
			for i, child := range v.Array {
				if i > 0 {
					if err := charge(1); err != nil {
						return err
					}
				}
				if err := charge(1 + 2*depth); err != nil {
					return err
				}
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
			if len(v.Array) > 0 {
				return charge(1 + 2*(depth-1))
			}
		default:
			return ErrMalformed
		}
		return nil
	}
	err := visit(root, 1)
	return cost, err
}

const ordinaryNeutralPreferences = `{"backupPolicy":{"destination":"iCloudDrive","frequency":"off"},"dataMode":"localFirst","language":"chinese","localFirstSyncPolicy":{"enabled":false,"endpoint":"localFolder","generateConflictCopy":true,"intervalSeconds":300,"mode":"manual","snapshotRetention":{"indexRetentionDays":30,"retentionIndexesDaily":1}},"settingsPoemDisplayPolicy":{"enabled":false,"text":""},"syncEndpointOptions":[],"theme":"coolGray","themeLanguageUpdatedAt":0,"themeLanguageWriterID":"mygo.development"}`

var ordinaryEmptySections = []string{"labels", "categoryMerges", "labelMerges", "categoryDeletionTombstones", "labelDeletionTombstones", "snapshotsByTraceID", "snapshotEventsByTraceID"}

func ordinaryWireScope(s canonical.Value) error {
	unsupported := func() error { return ordinary.ErrUnsupported }
	if len(s.Object["taskCycleSeries"].Array) != 0 || len(s.Object["ideas"].Array) != 0 {
		return unsupported()
	}
	for _, c := range s.Object["chains"].Array {
		if _, ok := c.Object["cycleMembership"]; ok {
			return unsupported()
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
	neutralBytes, err := canonical.Marshal(neutral)
	if err != nil {
		return err
	}
	if !bytes.Equal(prefs, neutralBytes) {
		return unsupported()
	}
	c := s.Object["classifications"]
	if signed(c.Object["formatVersion"]) != ordinary.FormatVersion {
		return ErrUnsupportedVersion
	}
	for _, key := range ordinaryEmptySections {
		if len(c.Object[key].Array) != 0 {
			return unsupported()
		}
	}
	for i := 1; i < len(c.Object["categories"].Array); i += 2 {
		item := c.Object["categories"].Array[i]
		if item.Object["lifecycle"].String != "active" || item.Object["presentationApproval"].String != "userApproved" || len(item.Object["nameVersions"].Array) != 1 {
			return unsupported()
		}
		if item.Object["canonicalKeyVersion"].String != canonical16.AlgorithmVersion || item.Object["nameVersions"].Array[0].Object["canonicalKeyVersion"].String != canonical16.AlgorithmVersion {
			return unsupported()
		}
		if _, ok := item.Object["nameVersions"].Array[0].Object["validUntil"]; ok {
			return unsupported()
		}
	}
	for i := 1; i < len(c.Object["currentByChainID"].Array); i += 2 {
		current := c.Object["currentByChainID"].Array[i]
		if len(current.Object["labels"].Array) != 0 {
			return unsupported()
		}
		if r, ok := current.Object["category"]; ok {
			if err = ordinaryDirect(r.Object["source"]); err != nil {
				return err
			}
		}
	}
	for _, h := range c.Object["relationHistory"].Array {
		if h.Object["kind"].String != "category" {
			return unsupported()
		}
		for _, key := range []string{"originSource", "removedBySource"} {
			if err = ordinaryDirect(h.Object[key]); err != nil {
				return err
			}
		}
	}
	for _, r := range c.Object["changeRecords"].Array {
		if err = ordinaryDirect(r.Object["source"]); err != nil {
			return err
		}
		if len(r.Object["changes"].Array) != 1 {
			return unsupported()
		}
		change, ok := r.Object["changes"].Array[0].Object["setCurrent"]
		if !ok {
			return unsupported()
		}
		for _, key := range []string{"before", "after"} {
			preview := change.Object[key]
			if len(preview.Object["labels"].Array) != 0 {
				return unsupported()
			}
			if item, ok := preview.Object["category"]; ok {
				if item.Object["kind"].String != "category" || item.Object["resolution"].String == "historicalAlias" {
					return unsupported()
				}
			}
		}
		if err = ordinaryNoticesScope(r.Object["notices"]); err != nil {
			return err
		}
	}
	for i := 1; i < len(c.Object["committedReceiptsByInteractionID"].Array); i += 2 {
		if err = ordinaryNoticesScope(c.Object["committedReceiptsByInteractionID"].Array[i].Object["notices"]); err != nil {
			return err
		}
	}
	return nil
}
func ordinaryDirect(v canonical.Value) error {
	if _, ok := v.Object["userDirect"]; !ok {
		return ordinary.ErrUnsupported
	}
	return nil
}
func ordinaryNoticesScope(v canonical.Value) error {
	if len(v.Array) > 1 {
		return ordinary.ErrUnsupported
	}
	for _, n := range v.Array {
		item, ok := n.Object["existingItemReused"]
		if !ok || item.Object["kind"].String != "category" || item.Object["matchedHistoricalAlias"].Bool {
			return ordinary.ErrUnsupported
		}
	}
	return nil
}
func ordinaryArray(values ...canonical.Value) canonical.Value {
	return canonical.Value{Kind: canonical.Array, Array: append([]canonical.Value{}, values...)}
}
func ordinaryEmptyField(typ, key string) bool {
	if (typ == "Current" || typ == "SelectionPreview") && key == "labels" {
		return true
	}
	if typ == "Classifications" {
		for _, empty := range ordinaryEmptySections {
			if key == empty {
				return true
			}
		}
	}
	return false
}

// These private schema-directed projections preserve spelling. In particular,
// the older ToDomain adapter uppercases UUIDs, so it is deliberately not used.
// Schema/scope checks have completed before this decoder allocates its output.
func ordinaryDecode(v canonical.Value, target reflect.Value, typ string) error {
	if target.Kind() == reflect.Pointer {
		target.Set(reflect.New(target.Type().Elem()))
		target = target.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		target.Set(reflect.MakeSlice(target.Type(), len(v.Array), len(v.Array)))
		for i, item := range v.Array {
			if err := ordinaryDecode(item, target.Index(i), typ[2:]); err != nil {
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
			if err := ordinaryDecode(v.Array[i], k, parts[0]); err != nil {
				return err
			}
			if err := ordinaryDecode(v.Array[i+1], value, parts[1]); err != nil {
				return err
			}
			target.SetMapIndex(k, value)
		}
		return nil
	}
	switch typ {
	case "ID":
		target.SetString(v.Object["rawValue"].String)
		return nil
	case "Source":
		target.SetString("userDirect")
		return nil
	case "PlanChange":
		target.FieldByName("Kind").SetString("setCurrent")
		return ordinaryDecodeFields(v.Object["setCurrent"], target, unions["PlanChange"]["setCurrent"], typ)
	case "Notice":
		target.FieldByName("Kind").SetString("existingItemReused")
		n := v.Object["existingItemReused"]
		target.FieldByName("ItemKind").SetString(n.Object["kind"].String)
		for _, key := range []string{"inputName", "currentName", "matchedHistoricalAlias"} {
			if err := ordinaryDecode(n.Object[key], fieldValue(target, key), unions["Notice"]["existingItemReused"][key]); err != nil {
				return err
			}
		}
		return nil
	}
	if fs, ok := objects[typ]; ok {
		return ordinaryDecodeFields(v, target, fs, typ)
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
func ordinaryDecodeFields(v canonical.Value, target reflect.Value, fs fields, typ string) error {
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
		if err := ordinaryDecode(child, field, t); err != nil {
			return err
		}
	}
	return nil
}
func ordinaryEncode(v reflect.Value, typ string) (canonical.Value, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return canonical.Value{}, ErrMalformed
		}
		v = v.Elem()
	}
	if strings.HasPrefix(typ, "[]") {
		out := ordinaryArray()
		for i := 0; i < v.Len(); i++ {
			item, err := ordinaryEncode(v.Index(i), typ[2:])
			if err != nil {
				return canonical.Value{}, err
			}
			out.Array = append(out.Array, item)
		}
		return out, nil
	}
	if strings.HasPrefix(typ, "pairs:") {
		parts := strings.SplitN(typ[6:], ":", 2)
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		out := ordinaryArray()
		for _, key := range keys {
			k, err := ordinaryEncode(key, parts[0])
			if err != nil {
				return canonical.Value{}, err
			}
			item, err := ordinaryEncode(v.MapIndex(key), parts[1])
			if err != nil {
				return canonical.Value{}, err
			}
			out.Array = append(out.Array, k, item)
		}
		return out, nil
	}
	switch typ {
	case "ID":
		return objectValue(map[string]canonical.Value{"rawValue": stringValue(v.String())}), nil
	case "Source":
		if v.String() != "userDirect" {
			return canonical.Value{}, ordinary.ErrUnsupported
		}
		return tagged("userDirect", map[string]canonical.Value{}), nil
	case "PlanChange":
		if v.FieldByName("Kind").String() != "setCurrent" {
			return canonical.Value{}, ordinary.ErrUnsupported
		}
		body, err := ordinaryEncodeFields(v, unions["PlanChange"]["setCurrent"], typ)
		if err != nil {
			return canonical.Value{}, err
		}
		return objectValue(map[string]canonical.Value{"setCurrent": body}), nil
	case "Notice":
		if v.FieldByName("Kind").String() != "existingItemReused" {
			return canonical.Value{}, ordinary.ErrUnsupported
		}
		body := map[string]canonical.Value{"kind": stringValue(v.FieldByName("ItemKind").String())}
		for _, key := range []string{"inputName", "currentName", "matchedHistoricalAlias"} {
			item, err := ordinaryEncode(fieldValue(v, key), unions["Notice"]["existingItemReused"][key])
			if err != nil {
				return canonical.Value{}, err
			}
			body[key] = item
		}
		return tagged("existingItemReused", body), nil
	}
	if fs, ok := objects[typ]; ok {
		return ordinaryEncodeFields(v, fs, typ)
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
	return canonical.Value{}, ErrMalformed
}
func ordinaryEncodeFields(v reflect.Value, fs fields, typ string) (canonical.Value, error) {
	out := map[string]canonical.Value{}
	for key, t := range fs {
		optional := strings.HasSuffix(key, "?")
		key = strings.TrimSuffix(key, "?")
		if ordinaryEmptyField(typ, key) {
			out[key] = ordinaryArray()
			continue
		}
		field := fieldValue(v, key)
		if !field.IsValid() {
			return canonical.Value{}, ErrMalformed
		}
		if optional && ((field.Kind() == reflect.Pointer && field.IsNil()) || (field.Kind() == reflect.String && field.Len() == 0)) {
			continue
		}
		item, err := ordinaryEncode(field, t)
		if err != nil {
			return canonical.Value{}, err
		}
		out[key] = item
	}
	return objectValue(out), nil
}
