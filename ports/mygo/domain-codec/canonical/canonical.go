// SPDX-License-Identifier: AGPL-3.0-only
// Package canonical provides bounded, lossless JSON parsing and a Foundation-style
// integer-only formatting candidate. It is not a Swift interoperability oracle.
package canonical

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Kind string

const (
	Null    Kind = "null"
	Object  Kind = "object"
	Array   Kind = "array"
	String  Kind = "string"
	Number  Kind = "number"
	Boolean Kind = "boolean"
)

// Value preserves number spelling; no conversion through float64 occurs.
// Only the field selected by Kind may be populated. Inactive maps and slices
// must be nil; active Object and Array fields may be nil or empty.
type Value struct {
	Kind       Kind
	Object     map[string]Value
	Array      []Value
	String     string
	TextNumber string
	Bool       bool
}

// Limits counts the root at depth one and counts object keys as nodes too.
// Zero fields use defaults. Negative fields are invalid.
type Limits struct{ MaxBytes, MaxDepth, MaxNodes int }

func DefaultLimits() Limits { return Limits{16 << 20, 128, 1_000_000} }
func limits(l Limits) (Limits, error) {
	d := DefaultLimits()
	if l.MaxBytes < 0 || l.MaxDepth < 0 || l.MaxNodes < 0 {
		return l, errors.New("negative JSON limit")
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
	// A hard ceiling prevents caller-supplied limits from allowing stack exhaustion.
	if l.MaxDepth > 1024 {
		return l, errors.New("JSON depth limit exceeds hard ceiling")
	}
	return l, nil
}

var ErrNonCanonical = errors.New("JSON differs from integer-only canonical candidate")
var ErrUnsupportedNumber = errors.New("canonical candidate requires a canonical integer")

// Parse rejects duplicate decoded keys, malformed UTF-8, unpaired UTF-16
// surrogates, trailing data, and inputs exceeding any limit.
func Parse(data []byte, l Limits) (Value, error) {
	l, e := limits(l)
	if e != nil {
		return Value{}, e
	}
	if len(data) > l.MaxBytes {
		return Value{}, errors.New("JSON byte limit exceeded")
	}
	if !utf8.Valid(data) {
		return Value{}, errors.New("invalid UTF-8")
	}
	if e = strictEscapes(data); e != nil {
		return Value{}, e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	p := parser{d: d, lim: l}
	v, e := p.value(1)
	if e != nil {
		return Value{}, e
	}
	if _, e = d.Token(); e != io.EOF {
		return Value{}, errors.New("trailing JSON data")
	}
	return v, nil
}

type parser struct {
	d     *json.Decoder
	lim   Limits
	nodes int
}

func (p *parser) count() error {
	p.nodes++
	if p.nodes > p.lim.MaxNodes {
		return errors.New("JSON node limit exceeded")
	}
	return nil
}
func (p *parser) value(depth int) (Value, error) {
	if depth > p.lim.MaxDepth {
		return Value{}, errors.New("JSON depth limit exceeded")
	}
	if e := p.count(); e != nil {
		return Value{}, e
	}
	t, e := p.d.Token()
	if e != nil {
		return Value{}, e
	}
	switch x := t.(type) {
	case nil:
		return Value{Kind: Null}, nil
	case bool:
		return Value{Kind: Boolean, Bool: x}, nil
	case string:
		return Value{Kind: String, String: x}, nil
	case json.Number:
		return Value{Kind: Number, TextNumber: string(x)}, nil
	case json.Delim:
		switch x {
		case '{':
			v := Value{Kind: Object, Object: map[string]Value{}}
			for p.d.More() {
				if e = p.count(); e != nil {
					return Value{}, e
				}
				t, e = p.d.Token()
				if e != nil {
					return Value{}, e
				}
				key, ok := t.(string)
				if !ok {
					return Value{}, errors.New("expected object key")
				}
				if _, ok = v.Object[key]; ok {
					return Value{}, errors.New("duplicate JSON object key")
				}
				child, e := p.value(depth + 1)
				if e != nil {
					return Value{}, e
				}
				v.Object[key] = child
			}
			t, e = p.d.Token()
			if e != nil || t != json.Delim('}') {
				return Value{}, errors.New("unterminated object")
			}
			return v, nil
		case '[':
			v := Value{Kind: Array, Array: []Value{}}
			for p.d.More() {
				child, e := p.value(depth + 1)
				if e != nil {
					return Value{}, e
				}
				v.Array = append(v.Array, child)
			}
			t, e = p.d.Token()
			if e != nil || t != json.Delim(']') {
				return Value{}, errors.New("unterminated array")
			}
			return v, nil
		}
	}
	return Value{}, errors.New("unexpected JSON token")
}

// encoding/json replaces unpaired surrogates. Inspect them before decoding.
func strictEscapes(b []byte) error {
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			continue
		}
		i++
		for ; i < len(b) && b[i] != '"'; i++ {
			if b[i] != '\\' {
				continue
			}
			i++
			if i >= len(b) {
				return errors.New("unfinished escape")
			}
			if b[i] != 'u' {
				continue
			}
			n, e := hex4(b, i+1)
			if e != nil {
				return e
			}
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return errors.New("unpaired low surrogate")
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
					return errors.New("unpaired high surrogate")
				}
				low, e := hex4(b, i+3)
				if e != nil {
					return e
				}
				if low < 0xdc00 || low > 0xdfff {
					return errors.New("unpaired high surrogate")
				}
				i += 6
			}
		}
	}
	return nil
}
func hex4(b []byte, i int) (uint64, error) {
	if i+4 > len(b) {
		return 0, errors.New("short Unicode escape")
	}
	n, e := strconv.ParseUint(string(b[i:i+4]), 16, 16)
	if e != nil {
		return 0, errors.New("invalid Unicode escape")
	}
	return n, nil
}

// Marshal emits sorted UTF-8 keys, two-space indentation, spaced colons,
// literal slashes and no final newline. Only canonical integer lexemes are
// supported: Foundation float formatting and Unicode key collation remain
// unverified. Non-integer numbers fail closed, rather than lose precision.
// Default limits also protect manually constructed or cyclic Values.
func Marshal(v Value) ([]byte, error) {
	w := writer{lim: DefaultLimits()}
	if e := w.value(v, 1); e != nil {
		return nil, e
	}
	return w.b.Bytes(), nil
}

type writer struct {
	b     bytes.Buffer
	lim   Limits
	nodes int
}

func (w *writer) put(s string) error {
	if len(s) > w.lim.MaxBytes-w.b.Len() {
		return errors.New("JSON output byte limit exceeded")
	}
	w.b.WriteString(s)
	return nil
}
func (w *writer) quote(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("invalid UTF-8 string")
	}
	if e := w.put("\""); e != nil {
		return e
	}
	for _, r := range s {
		var x string
		switch r {
		case '"':
			x = "\\\""
		case '\\':
			x = "\\\\"
		case '\b':
			x = "\\b"
		case '\f':
			x = "\\f"
		case '\n':
			x = "\\n"
		case '\r':
			x = "\\r"
		case '\t':
			x = "\\t"
		default:
			if r < 32 {
				x = fmt.Sprintf("\\u%04x", r)
			} else {
				x = string(r)
			}
		}
		if e := w.put(x); e != nil {
			return e
		}
	}
	return w.put("\"")
}
func integer(s string) bool {
	if s == "" || s == "-0" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i++
	}
	if i == len(s) {
		return false
	}
	if s[i] == '0' {
		return i+1 == len(s)
	}
	if s[i] < '1' || s[i] > '9' {
		return false
	}
	for i++; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
func (w *writer) value(v Value, depth int) error {
	if err := validUnion(v); err != nil {
		return err
	}
	if depth > w.lim.MaxDepth {
		return errors.New("JSON output depth limit exceeded")
	}
	w.nodes++
	if w.nodes > w.lim.MaxNodes {
		return errors.New("JSON output node limit exceeded")
	}
	indent := func(n int) error { return w.put(strings.Repeat("  ", n)) }
	switch v.Kind {
	case Null:
		return w.put("null")
	case Boolean:
		return w.put(strconv.FormatBool(v.Bool))
	case String:
		return w.quote(v.String)
	case Number:
		if !integer(v.TextNumber) {
			return ErrUnsupportedNumber
		}
		return w.put(v.TextNumber)
	case Object:
		if len(v.Object) > (w.lim.MaxNodes-w.nodes)/2 {
			return errors.New("JSON output node limit exceeded")
		}
		if e := w.put("{"); e != nil {
			return e
		}
		keys := make([]string, 0, len(v.Object))
		for k := range v.Object {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			w.nodes++
			if w.nodes > w.lim.MaxNodes {
				return errors.New("JSON output node limit exceeded")
			}
			if i > 0 {
				if e := w.put(","); e != nil {
					return e
				}
			}
			if e := w.put("\n"); e != nil {
				return e
			}
			if e := indent(depth); e != nil {
				return e
			}
			if e := w.quote(k); e != nil {
				return e
			}
			if e := w.put(" : "); e != nil {
				return e
			}
			if e := w.value(v.Object[k], depth+1); e != nil {
				return e
			}
		}
		if len(keys) > 0 {
			if e := w.put("\n"); e != nil {
				return e
			}
			if e := indent(depth - 1); e != nil {
				return e
			}
		}
		return w.put("}")
	case Array:
		if len(v.Array) > w.lim.MaxNodes-w.nodes {
			return errors.New("JSON output node limit exceeded")
		}
		if e := w.put("["); e != nil {
			return e
		}
		for i, c := range v.Array {
			if i > 0 {
				if e := w.put(","); e != nil {
					return e
				}
			}
			if e := w.put("\n"); e != nil {
				return e
			}
			if e := indent(depth); e != nil {
				return e
			}
			if e := w.value(c, depth+1); e != nil {
				return e
			}
		}
		if len(v.Array) > 0 {
			if e := w.put("\n"); e != nil {
				return e
			}
			if e := indent(depth - 1); e != nil {
				return e
			}
		}
		return w.put("]")
	}
	return errors.New("invalid JSON value kind")
}

// ValidateCanonical compares bytes to this package's candidate, not an Apple
// Foundation oracle. Domain/schema validation must be performed separately.
func ValidateCanonical(data []byte, l Limits) (Value, error) {
	v, e := Parse(data, l)
	if e != nil {
		return Value{}, e
	}
	b, e := Marshal(v)
	if e != nil {
		return Value{}, e
	}
	if !bytes.Equal(data, b) {
		return Value{}, ErrNonCanonical
	}
	return v, nil
}

// Shape reproduces upstream length-prefixed shapes with array element shapes
// sorted as a multiset, retaining multiplicity, nested keys and value kinds.
func Shape(v Value) (string, error) {
	// Validate bounds and kinds without rejecting valid non-integer number tokens.
	n := 0
	return shape(v, 1, &n)
}
func shape(v Value, depth int, n *int) (string, error) {
	if err := validUnion(v); err != nil {
		return "", err
	}
	*n++
	if depth > DefaultLimits().MaxDepth || *n > DefaultLimits().MaxNodes {
		return "", errors.New("JSON shape limit exceeded")
	}
	lp := func(s string) string { return strconv.Itoa(len(s)) + ":" + s }
	switch v.Kind {
	case Null, Boolean:
		return string(v.Kind), nil
	case String:
		if !utf8.ValidString(v.String) || len(v.String) > DefaultLimits().MaxBytes {
			return "", errors.New("invalid or oversized JSON shape string")
		}
		return "string", nil
	case Number:
		if len(v.TextNumber) > DefaultLimits().MaxBytes {
			return "", errors.New("JSON shape byte limit exceeded")
		}
		if !json.Valid([]byte(v.TextNumber)) || len(v.TextNumber) == 0 || !(v.TextNumber[0] == '-' || v.TextNumber[0] >= '0' && v.TextNumber[0] <= '9') {
			return "", errors.New("invalid JSON number")
		}
		return "number", nil
	case Object:
		if len(v.Object) > (DefaultLimits().MaxNodes-*n)/2 {
			return "", errors.New("JSON shape node limit exceeded")
		}
		keys := make([]string, 0, len(v.Object))
		for k := range v.Object {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			*n++
			if !utf8.ValidString(k) || len(k) > DefaultLimits().MaxBytes || *n > DefaultLimits().MaxNodes {
				return "", errors.New("JSON shape key limit or encoding")
			}
			s, e := shape(v.Object[k], depth+1, n)
			if e != nil {
				return "", e
			}
			b.WriteString(lp(k))
			b.WriteString(lp(s))
			if b.Len() > DefaultLimits().MaxBytes {
				return "", errors.New("JSON shape byte limit exceeded")
			}
		}
		return "object" + lp(b.String()), nil
	case Array:
		if len(v.Array) > DefaultLimits().MaxNodes-*n {
			return "", errors.New("JSON shape node limit exceeded")
		}
		shapes := make([]string, 0, len(v.Array))
		size := 0
		for _, c := range v.Array {
			s, e := shape(c, depth+1, n)
			if e != nil {
				return "", e
			}
			s = lp(s)
			size += len(s)
			if size > DefaultLimits().MaxBytes {
				return "", errors.New("JSON shape byte limit exceeded")
			}
			shapes = append(shapes, s)
		}
		// Sort raw shapes, not their length prefixes.
		sort.Slice(shapes, func(i, j int) bool {
			a := strings.IndexByte(shapes[i], ':')
			b := strings.IndexByte(shapes[j], ':')
			return shapes[i][a+1:] < shapes[j][b+1:]
		})
		return "array" + lp(strings.Join(shapes, "")), nil
	}
	return "", errors.New("invalid JSON shape kind")
}

// validUnion rejects hidden data without traversing it, including cycles in
// inactive fields. This must run at every visited node, not only at the root.
func validUnion(v Value) error {
	if (v.Kind != Object && v.Object != nil) ||
		(v.Kind != Array && v.Array != nil) ||
		(v.Kind != String && v.String != "") ||
		(v.Kind != Number && v.TextNumber != "") ||
		(v.Kind != Boolean && v.Bool) {
		return errors.New("inconsistent JSON value union")
	}
	return nil
}
