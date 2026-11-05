// SPDX-License-Identifier: AGPL-3.0-only
// Package textcanon compares canonical Unicode text without stream-safe edits.
package textcanon

import (
	"slices"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Equivalent reports canonical equivalence using the pinned x/text Unicode
// tables. Invalid UTF-8 is never equivalent, including to identical bad bytes.
// This function only compares; neither original string is rewritten.
func Equivalent(a, b string) bool {
	if !utf8.ValidString(a) || !utf8.ValidString(b) {
		return false
	}
	if a == b {
		return true
	}
	return slices.Equal(decompose(a), decompose(b))
}

// decompose implements UAX #15 canonical decomposition and ordering. Applying
// x/text NFD to a whole string is NOT sufficient: it inserts CGJ after 30
// non-starters. A single scalar's full canonical decomposition is bounded by
// the pinned tables (exhaustively checked in TestScalarDecompositionBound).
// Thus scalar-local NFD performs decomposition, including algorithmic Hangul,
// without inserting CGJ. Reordering must then span the entire combining run,
// not x/text's stream-safe chunks. A real CGJ has CCC=0 and remains a boundary.
// Stable counting sort (256 possible CCCs) avoids quadratic insertion sorting
// on maliciously long/reversed runs. Time and storage are O(input bytes), with
// fixed Unicode-table expansion; no arbitrary length cutoff changes equality.
func decompose(s string) []rune {
	out := make([]rune, 0, len(s))
	run := make([]rune, 0)
	var counts [256]int
	flush := func() {
		if len(run) == 0 {
			return
		}
		start := len(out)
		out = append(out, run...)
		var offsets [256]int
		next := start
		for c, count := range counts {
			offsets[c] = next
			next += count
		}
		for _, r := range run {
			c := norm.NFD.PropertiesString(string(r)).CCC()
			out[offsets[c]] = r
			offsets[c]++
		}
		clear(counts[:])
		run = run[:0]
	}
	for _, r := range s {
		for _, d := range norm.NFD.String(string(r)) {
			c := norm.NFD.PropertiesString(string(d)).CCC()
			if c == 0 {
				flush()
				out = append(out, d)
			} else {
				run = append(run, d)
				counts[c]++
			}
		}
	}
	flush()
	return out
}
