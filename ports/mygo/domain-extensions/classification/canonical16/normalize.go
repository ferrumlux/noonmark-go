// SPDX-License-Identifier: AGPL-3.0-only
package canonical16

import "sort"

type propertyRange struct {
	lo, hi rune
	value  uint16
}
type scalarMapping struct {
	scalar rune
	value  string
}
type compositionPair struct {
	pair  uint64
	value rune
}

func properties(cp rune) uint16 {
	i := sort.Search(len(propertyRanges), func(i int) bool { return propertyRanges[i].hi >= cp })
	if i < len(propertyRanges) && propertyRanges[i].lo <= cp {
		return propertyRanges[i].value
	}
	return 0
}
func ccc(cp rune) uint8 { return uint8(properties(cp)) }

func lookupMapping(table []scalarMapping, cp rune) (string, bool) {
	i := sort.Search(len(table), func(i int) bool { return table[i].scalar >= cp })
	if i < len(table) && table[i].scalar == cp {
		return table[i].value, true
	}
	return "", false
}

const (
	sBase  rune = 0xac00
	lBase  rune = 0x1100
	vBase  rune = 0x1161
	tBase  rune = 0x11a7
	lCount      = 19
	vCount      = 21
	tCount      = 28
	nCount      = vCount * tCount
	sCount      = lCount * nCount
)

func decompose(out []rune, cp rune) []rune {
	if s := cp - sBase; s >= 0 && s < sCount {
		out = append(out, lBase+s/nCount, vBase+(s%nCount)/tCount)
		if tail := s % tCount; tail != 0 {
			out = append(out, tBase+tail)
		}
		return out
	}
	if mapping, found := lookupMapping(canonicalDecompositions[:], cp); found {
		for _, child := range mapping {
			out = decompose(out, child)
		}
		return out
	}
	return append(out, cp)
}

// order sorts each entire nonstarter run stably. Counting sort avoids quadratic
// insertion-sort behaviour and never partitions a run at a stream-safe limit.
// Already-ordered runs are linear scans; only disordered runs allocate scratch.
func order(in []rune) []rune {
	var scratch []rune
	for start := 0; start < len(in); {
		if ccc(in[start]) == 0 {
			start++
			continue
		}
		end, previous, disordered := start, uint8(0), false
		var counts [256]int
		for end < len(in) {
			class := ccc(in[end])
			if class == 0 {
				break
			}
			if class < previous {
				disordered = true
			}
			previous = class
			counts[class]++
			end++
		}
		if disordered {
			length := end - start
			if cap(scratch) < length {
				scratch = make([]rune, length)
			} else {
				scratch = scratch[:length]
			}
			offset := 0
			for class := 1; class < len(counts); class++ {
				count := counts[class]
				counts[class] = offset
				offset += count
			}
			for _, cp := range in[start:end] {
				class := ccc(cp)
				scratch[counts[class]] = cp
				counts[class]++
			}
			copy(in[start:end], scratch)
		}
		start = end
	}
	return in
}

func composite(a, b rune) (rune, bool) {
	if l, v := a-lBase, b-vBase; l >= 0 && l < lCount && v >= 0 && v < vCount {
		return sBase + (l*vCount+v)*tCount, true
	}
	if s, t := a-sBase, b-tBase; s >= 0 && s < sCount && s%tCount == 0 && t > 0 && t < tCount {
		return a + t, true
	}
	pair := uint64(a)<<21 | uint64(b)
	i := sort.Search(len(compositionPairs), func(i int) bool { return compositionPairs[i].pair >= pair })
	if i < len(compositionPairs) && compositionPairs[i].pair == pair {
		return compositionPairs[i].value, true
	}
	return 0, false
}

func compose(in []rune) []rune {
	out := in[:0]
	starter, previous := -1, uint8(0)
	for _, cp := range in {
		class := ccc(cp)
		if starter >= 0 && (previous < class || previous == 0) {
			if result, ok := composite(out[starter], cp); ok {
				out[starter] = result
				continue
			}
		}
		if class == 0 {
			starter = len(out)
		}
		out = append(out, cp)
		previous = class
	}
	return out
}
