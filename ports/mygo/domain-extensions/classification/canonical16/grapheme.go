// SPDX-License-Identifier: AGPL-3.0-only
package canonical16

const (
	gOther uint16 = iota
	gCR
	gLF
	gControl
	gExtend
	gZWJ
	gRI
	gPrepend
	gSpacingMark
	gL
	gV
	gT
	gLV
	gLVT
)
const (
	iNone uint16 = iota
	iConsonant
	iExtend
	iLinker
)
const epMask uint16 = 1 << 14
const whiteSpaceMask uint16 = 1 << 15

// visitClusters implements UAX #29 rev 45 default extended grapheme boundaries.
// All context is forward state, so even long RI/Extend/Indic runs remain linear.
// Input must be valid UTF-8. Returned offsets index the original bytes.
func visitClusters(s string, visit func(start, end int)) {
	start := 0
	var previous uint16
	riRun := 0
	epExtend, zwjAfterEP := false, false
	indicConsonant, indicLinked := false, false
	for offset, cp := range s {
		prop := properties(cp)
		current, incb := (prop>>8)&15, (prop>>12)&3
		boundary := offset != 0
		if boundary {
			switch {
			case previous == gCR && current == gLF: // GB3
				boundary = false
			case previous == gCR || previous == gLF || previous == gControl: // GB4
			case current == gCR || current == gLF || current == gControl: // GB5
			case previous == gL && (current == gL || current == gV || current == gLV || current == gLVT): // GB6
				boundary = false
			case (previous == gLV || previous == gV) && (current == gV || current == gT): // GB7
				boundary = false
			case (previous == gLVT || previous == gT) && current == gT: // GB8
				boundary = false
			case current == gExtend || current == gZWJ: // GB9
				boundary = false
			case current == gSpacingMark: // GB9a
				boundary = false
			case previous == gPrepend: // GB9b
				boundary = false
			case incb == iConsonant && indicConsonant && indicLinked: // GB9c
				boundary = false
			case prop&epMask != 0 && previous == gZWJ && zwjAfterEP: // GB11
				boundary = false
			case previous == gRI && current == gRI && riRun%2 == 1: // GB12/13
				boundary = false
			}
		}
		if boundary {
			visit(start, offset)
			start = offset
		}
		// Track the raw left context, not just scalars in the current cluster.
		zwjAfterEP = current == gZWJ && epExtend
		if prop&epMask != 0 {
			epExtend = true
		} else if current != gExtend {
			epExtend = false
		}
		switch incb {
		case iConsonant:
			indicConsonant, indicLinked = true, false
		case iLinker:
			indicLinked = indicConsonant
		case iExtend:
		default:
			indicConsonant, indicLinked = false, false
		}
		if current == gRI {
			riRun++
		} else {
			riRun = 0
		}
		previous = current
	}
	if len(s) > 0 {
		visit(start, len(s))
	}
}
