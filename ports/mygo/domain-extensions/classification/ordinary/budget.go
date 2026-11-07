// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"math"
	"reflect"
	"unicode/utf8"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
)

// Limits are a deliberately conservative cumulative resource subset, NOT an
// exact RSS bound or a promise that a 16 MiB name/catalog is usable. Counts cover
// inputs, every logical alias, prospective copies, encoder escapes, and work.
// Zero fields select defaults. Negative or larger-than-default limits fail.
type Limits struct{ MaxBytes, MaxDepth, MaxNodes int }

func DefaultLimits() Limits { return Limits{16 << 20, 128, 1_000_000} }
func checkedLimits(l Limits) (Limits, error) {
	d := DefaultLimits()
	if l.MaxBytes < 0 || l.MaxDepth < 0 || l.MaxNodes < 0 {
		return l, ErrResourceLimit
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
		return l, ErrResourceLimit
	}
	return l, nil
}

type budget struct {
	lim          Limits
	bytes, nodes uint64
}

func newBudget(l Limits) (*budget, error) {
	l, e := checkedLimits(l)
	if e != nil {
		return nil, e
	}
	return &budget{lim: l}, nil
}
func add(a, b uint64) (uint64, error) {
	if b > math.MaxUint64-a {
		return 0, ErrResourceLimit
	}
	return a + b, nil
}
func mul(a, b uint64) (uint64, error) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, ErrResourceLimit
	}
	return a * b, nil
}
func (b *budget) charge(bytes, nodes uint64) error {
	nb, e := add(b.bytes, bytes)
	if e != nil || nb > uint64(b.lim.MaxBytes) {
		return ErrResourceLimit
	}
	nn, e := add(b.nodes, nodes)
	if e != nil || nn > uint64(b.lim.MaxNodes) {
		return ErrResourceLimit
	}
	b.bytes, b.nodes = nb, nn
	return nil
}
func (b *budget) scale(n, factor uint64) error {
	v, e := mul(n, factor)
	if e != nil {
		return e
	}
	return b.charge(v, 0)
}

// scan is a read-only, early-terminating scan of these fixed typed structures.
// It visits map keys and each alias occurrence, never a caller claimed revision.
// Content validation is deliberately a second pass after the ENTIRE raw scan.
func (b *budget) scan(values ...any) error {
	for _, v := range values {
		if e := b.value(reflect.ValueOf(v), 1); e != nil {
			return e
		}
	}
	return nil
}
func (b *budget) value(v reflect.Value, depth int) error {
	if depth > b.lim.MaxDepth {
		return ErrResourceLimit
	}
	if e := b.charge(8, 1); e != nil {
		return e
	}
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		return b.charge(uint64(v.Len()), 0)
	case reflect.Bool:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return b.charge(20, 0)
	case reflect.Pointer:
		if !v.IsNil() {
			return b.value(v.Elem(), depth+1)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if e := b.charge(uint64(len(t.Field(i).Name))+8, 1); e != nil {
				return e
			}
			if e := b.value(v.Field(i), depth+1); e != nil {
				return e
			}
		}
	case reflect.Slice, reflect.Array:
		if uint64(v.Len()) > uint64(b.lim.MaxNodes)-b.nodes {
			return ErrResourceLimit
		}
		for i := 0; i < v.Len(); i++ {
			if e := b.value(v.Index(i), depth+1); e != nil {
				return e
			}
		}
	case reflect.Map:
		count, e := mul(uint64(v.Len()), 2)
		if e != nil || count > uint64(b.lim.MaxNodes)-b.nodes {
			return ErrResourceLimit
		}
		it := v.MapRange()
		for it.Next() {
			if e := b.value(it.Key(), depth+1); e != nil {
				return e
			}
			if e := b.value(it.Value(), depth+1); e != nil {
				return e
			}
		}
	default:
		return ErrUnsupported
	}
	return nil
}
func validStrings(values ...any) bool {
	for _, v := range values {
		if !validStringValue(reflect.ValueOf(v)) {
			return false
		}
	}
	return true
}
func validStringValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Pointer:
		return v.IsNil() || validStringValue(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !validStringValue(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !validStringValue(v.Index(i)) {
				return false
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			if !validStringValue(it.Key()) || !validStringValue(it.Value()) {
				return false
			}
		}
	}
	return true
}

// Every actual D/K/NFC invocation is prepaid. The 288/16 constants are proven
// from the pinned UCD16 (18-scalar NFKC_CF mapping, <=4 canonical decomposition,
// <=4 UTF-8 bytes/scalar). Rune growth, sorting scratch and counts are separate.
// Materialized outputs are checked against those bounds BEFORE being used; their
// checked exact byte length is a tighter input bound for any subsequent call.
func (b *budget) display(s string) (string, error) {
	if e := b.scale(uint64(len(s)), 3); e != nil {
		return "", e
	}
	out, e := canonical16.DisplayName(s)
	if e != nil {
		return "", ErrName
	}
	if len(out) > len(s) {
		return "", ErrResourceLimit
	}
	return out, nil
}
func (b *budget) key(s string) (string, error) {
	// CanonicalKey contains a D call. Charge it and the K output/rune/sort work.
	if e := b.scale(uint64(len(s)), 3+288*4); e != nil {
		return "", e
	}
	if e := b.charge(2048, 0); e != nil {
		return "", e
	}
	out, e := canonical16.CanonicalKey(s)
	if e != nil {
		return "", ErrName
	}
	bound, e := mul(uint64(len(s)), 288)
	if e != nil || uint64(len(out)) > bound {
		return "", ErrResourceLimit
	}
	return out, nil
}
func (b *budget) nfc(s string) (string, error) {
	if e := b.scale(uint64(len(s)), 16*4); e != nil {
		return "", e
	}
	if e := b.charge(2048, 0); e != nil {
		return "", e
	}
	out, e := canonical16.NFC(s)
	if e != nil {
		return "", ErrName
	}
	bound, e := mul(uint64(len(s)), 16)
	if e != nil || uint64(len(out)) > bound {
		return "", ErrResourceLimit
	}
	return out, nil
}
func (b *budget) equivalent(a, c string) (bool, error) {
	// Equivalent is explicitly implemented as two prepaid NFC calls. No uncharged
	// direct canonical16.Equivalent call or canonical-key feedback exists.
	x, e := b.nfc(a)
	if e != nil {
		return false, e
	}
	y, e := b.nfc(c)
	if e != nil {
		return false, e
	}
	return x == y, nil
}
func (b *budget) normalized(s string) (string, error) {
	d, e := b.display(s)
	if e != nil {
		return "", e
	}
	if d == "" {
		return "", ErrName
	}
	k, e := b.key(d)
	if e != nil {
		return "", e
	}
	if k == "" {
		return "", ErrName
	}
	return d, nil
}

// reserveEncoding bounds all strings at 6x UTF-8 plus quotes/structural fields.
// scan's scalar/field overhead also bounds integer formatting and containers.
func (b *budget) encodingCapacity(v any) (int, error) {
	old, oldNodes := b.bytes, b.nodes
	if e := b.scan(v); e != nil {
		return 0, e
	}
	bound, e := mul(b.bytes-old, 6)
	if e != nil {
		return 0, e
	}
	nodes, e := mul(b.nodes-oldNodes, 6)
	if e != nil {
		return 0, e
	}
	if e = b.charge(bound, nodes); e != nil {
		return 0, e
	}
	return int(bound), nil
}
func (b *budget) reserveEncoding(v any) error { _, e := b.encodingCapacity(v); return e }
