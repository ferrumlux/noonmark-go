// SPDX-License-Identifier: AGPL-3.0-only
// Derived from Noonmark 43e6eefaae3a04be89cab3e234d470111afd80ef.

// Package canonical16 implements a version-pinned Unicode16 classification-name
// candidate. It uses Unicode16 extended grapheme clusters and their first scalar's
// White_Space property for display, then NFKC_CF and real NFC for public keys.
//
// Unicode conformance and ICU differential tests do not establish which Unicode
// segmentation version a target Swift runtime uses. This package does not enable
// nonempty classification catalogs or change any stored canonical-key version.
package canonical16

import (
	"errors"
	"strings"
	"unicode/utf8"
)

//go:generate python3 internal/generate/generate.py

const UnicodeVersion = "16.0.0"
const AlgorithmVersion = "unicode-16.0.0.0-nfkc-casefold-whitespace-v1"

var (
	ErrInvalidUTF8  = errors.New("canonical16: invalid UTF-8")
	ErrEmptyDisplay = errors.New("canonical16: empty display name")
	ErrEmptyKey     = errors.New("canonical16: empty canonical key")
)

// Canonicalizer is immutable, safe for concurrent use, and needs no constructor.
// Its method set satisfies classification.NameCanonicalizer without importing
// that parent package. It models the direct Swift TaskCategory/TaskLabel
// initializer's two-stage call chain: DisplayName(s), CanonicalKey(display).
// These are two display passes, deliberately not fold(display), because display
// split/join is not idempotent for all EGC input. Unlike the raw Swift initializer,
// this adapter enforces nonempty display/key name constraints.
//
// Prepared create, resolve and rename have additional preprocessing upstream.
// This adapter is not a drop-in proof of those workflow semantics.
type Canonicalizer struct{}

func (Canonicalizer) Version() string { return AlgorithmVersion }

func (Canonicalizer) Canonicalize(s string) (display, key string, err error) {
	display, err = DisplayName(s)
	if err != nil {
		return "", "", err
	}
	if display == "" {
		return "", "", ErrEmptyDisplay
	}
	key, err = CanonicalKey(display)
	if err != nil {
		return "", "", err
	}
	if key == "" {
		return "", "", ErrEmptyKey
	}
	return display, key, nil
}

// DisplayName collapses clusters whose first scalar is White_Space, matching the
// source's split/join rule under Unicode16 EGC segmentation. It preserves every
// byte in the remaining clusters and accepts the empty string.
func DisplayName(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", ErrInvalidUTF8
	}
	var out strings.Builder
	out.Grow(len(s))
	pendingSpace := false
	visitClusters(s, func(start, end int) {
		first, _ := utf8.DecodeRuneInString(s[start:end])
		if properties(first)&whiteSpaceMask != 0 {
			pendingSpace = out.Len() > 0
			return
		}
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
		out.WriteString(s[start:end])
	})
	return out.String(), nil
}

// CanonicalKey matches the public source canonicalKey function under the fixed
// Unicode16 segmentation contract: exactly one DisplayName pass, then NFKC_CF.
// Empty display/key is permitted, just as in that public source function.
func CanonicalKey(s string) (string, error) {
	display, err := DisplayName(s)
	if err != nil {
		return "", err
	}
	return nfkcCasefoldValid(display), nil
}

// NFKCCasefold applies Unicode16 NFKC_CF mappings to the input scalars followed
// by true NFC of the entire mapped string. It does not trim before or after
// folding, and allows empty output. Use Canonicalize for name constraints.
func NFKCCasefold(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", ErrInvalidUTF8
	}
	return nfkcCasefoldValid(s), nil
}

func nfkcCasefoldValid(s string) string {
	out := make([]rune, 0, len(s))
	for _, cp := range s {
		if mapping, found := lookupMapping(casefoldMappings[:], cp); found {
			for _, mapped := range mapping {
				out = decompose(out, mapped)
			}
		} else {
			out = decompose(out, cp)
		}
	}
	return string(compose(order(out)))
}

// NFC returns true Unicode16 NFC without stream-safe CGJ insertion, scalar
// deletion, compatibility folding, or a bound on nonstarter-run length.
func NFC(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", ErrInvalidUTF8
	}
	return nfcValid(s), nil
}

func nfcValid(s string) string {
	out := make([]rune, 0, len(s))
	for _, cp := range s {
		out = decompose(out, cp)
	}
	return string(compose(order(out)))
}

// Equivalent compares canonical equivalence only, without case/compatibility
// folding or whitespace normalization. Invalid UTF-8 is never equivalent.
func Equivalent(a, b string) (bool, error) {
	if !utf8.ValidString(a) || !utf8.ValidString(b) {
		return false, ErrInvalidUTF8
	}
	return nfcValid(a) == nfcValid(b), nil
}
