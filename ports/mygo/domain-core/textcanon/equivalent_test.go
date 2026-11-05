// SPDX-License-Identifier: AGPL-3.0-only
package textcanon

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func TestEquivalent(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		want       bool
	}{
		{"empty", "", "", true},
		{"accent", "caf\u00e9", "cafe\u0301", true},
		{"Hangul", "\uac01", "\u1100\u1161\u11a8", true},
		{"emoji", "👩‍💻", "👩💻", false},
		{"compatibility", "\ufb01", "fi", false},
		{"equal CCC stable", "x\u0301\u0300", "x\u0300\u0301", false},
		{"real CGJ boundary", "x\u0301\u034f\u0323", "x\u0323\u034f\u0301", false},
		{"invalid identical", "\xff", "\xff", false},
		{"invalid replacement", "\xff", "\ufffd", false},
		{"invalid unequal", "\xff", "\xfe", false},
		{"no trim", " a", "a", false},
		{"no newline conversion", "a\r\n", "a\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Equivalent(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if Equivalent(tc.b, tc.a) != tc.want {
				t.Fatal("asymmetric")
			}
		})
	}
	for _, n := range []int{1, 29, 30, 31, 32, 60, 61, 4096, 65536} {
		a := strings.Repeat("\u0301", n)
		b := a + "\u034f"
		if n > 30 {
			b = strings.Repeat("\u0301", 30) + "\u034f" + strings.Repeat("\u0301", n-30)
		}
		if Equivalent(a, b) {
			t.Fatalf("real CGJ swallowed at %d", n)
		}
		// Sorting must cross the 30-scalar stream-safe boundary.
		a = "x" + strings.Repeat("\u0301", n) + strings.Repeat("\u0323", n)
		b = "x" + strings.Repeat("\u0323", n) + strings.Repeat("\u0301", n)
		if !Equivalent(a, b) {
			t.Fatalf("long reordered sequence at %d", n)
		}
	}
}

// Exhaust the actual pinned scalar table rather than relying on a guessed
// decomposition bound. A dependency/table upgrade must revisit this proof.
func TestScalarDecompositionBound(t *testing.T) {
	if norm.Version != "17.0.0" {
		t.Fatalf("review Unicode table upgrade: %s", norm.Version)
	}
	maxRun := 0
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		d := norm.NFD.String(string(r))
		if r != '\u034f' && strings.ContainsRune(d, '\u034f') {
			t.Fatalf("scalar U+%04X introduced CGJ", r)
		}
		run := 0
		for _, v := range d {
			if norm.NFD.PropertiesString(string(v)).CCC() == 0 {
				run = 0
			} else {
				run++
				if run > maxRun {
					maxRun = run
				}
				if run > 30 {
					t.Fatalf("unsafe scalar U+%04X", r)
				}
			}
		}
		if run > 30 {
			t.Fatalf("unsafe scalar U+%04X", r)
		}
		if len(d) > 4*utf8.UTFMax {
			t.Fatalf("review expansion bound U+%04X", r)
		}
	}
	t.Logf("Unicode %s maximum scalar nonstarter run=%d", norm.Version, maxRun)
}

func BenchmarkLongReversedRun(b *testing.B) {
	a := "x" + strings.Repeat("\u0301", 65536) + strings.Repeat("\u0323", 65536)
	z := "x" + strings.Repeat("\u0323", 65536) + strings.Repeat("\u0301", 65536)
	b.ReportAllocs()
	for b.Loop() {
		if !Equivalent(a, z) {
			b.Fatal("not equivalent")
		}
	}
}
