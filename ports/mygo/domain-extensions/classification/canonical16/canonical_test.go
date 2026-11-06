// SPDX-License-Identifier: AGPL-3.0-only
package canonical16

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestNameSemantics(t *testing.T) {
	cases := []struct{ input, display, key string }{
		{"  Straße\t", "Straße", "strasse"},
		{"ＡＢＣ", "ＡＢＣ", "abc"},
		{"Σςσ", "Σςσ", "σσσ"},
		{"İIı", "İIı", "i\u0307iı"},
		{"ﬃK", "ﬃK", "ffik"},
		{"É", "É", "é"},
		{"E\u0301", "E\u0301", "é"},
		{"E", "E", "e"},
		{" \u0301X", "X", "x"},
		{"a\n\u0301b", "a \u0301b", "a b"},
		{"A \u0301B", "A B", "a b"},
		{"A\r\n\t B", "A B", "a b"},
		{"\u0600 X", "\u0600 X", "\u0600 x"},
		{"foo\u00adbar", "foo\u00adbar", "foobar"},
		{"a\u200db", "a\u200db", "ab"},
		{"\u00a8", "\u00a8", " \u0308"},
		{"\x00X", "\x00X", "\x00x"},
		{"\U0010ffff", "\U0010ffff", "\U0010ffff"},
		{"A\u034f\u030a", "A\u034f\u030a", "å"},
		{"\u1100\u200d\u1161\u11a8", "\u1100\u200d\u1161\u11a8", "각"},
	}
	var c Canonicalizer
	if c.Version() != "unicode-16.0.0.0-nfkc-casefold-whitespace-v1" {
		t.Fatal(c.Version())
	}
	for _, tc := range cases {
		display, key, err := c.Canonicalize(tc.input)
		if err != nil || display != tc.display || key != tc.key {
			t.Errorf("%q got (%q,%q,%v) want (%q,%q)", tc.input, display, key, err, tc.display, tc.key)
		}
	}
}

func TestErrorsAndEmpty(t *testing.T) {
	var c Canonicalizer
	for _, s := range []string{"", "\t \r\n", " \u0301"} {
		if _, _, err := c.Canonicalize(s); !errors.Is(err, ErrEmptyDisplay) {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"\u200b", "\u034f", "\u00ad\u200d\ufe0f"} {
		if _, _, err := c.Canonicalize(s); !errors.Is(err, ErrEmptyKey) {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"\xff", "\xc0\x80", "\xed\xa0\x80", "\xf4\x90\x80\x80", "valid\xe2\x82"} {
		if _, _, err := c.Canonicalize(s); !errors.Is(err, ErrInvalidUTF8) {
			t.Error(err)
		}
		for _, f := range []func(string) (string, error){NFC, NFKCCasefold, DisplayName, CanonicalKey} {
			if _, err := f(s); !errors.Is(err, ErrInvalidUTF8) {
				t.Error(err)
			}
		}
		for _, pair := range [][2]string{{s, "valid"}, {"valid", s}, {s, s}} {
			if equal, err := Equivalent(pair[0], pair[1]); equal || !errors.Is(err, ErrInvalidUTF8) {
				t.Errorf("equivalent invalid: %v %v", equal, err)
			}
		}
	}
	for _, f := range []func(string) (string, error){NFC, NFKCCasefold, DisplayName, CanonicalKey} {
		if actual, err := f(""); actual != "" || err != nil {
			t.Errorf("empty transform: %q %v", actual, err)
		}
	}
}

func TestCanonicalEquivalenceOnly(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{
		{"É", "E\u0301", true}, {"\u212b", "A\u030a", true},
		{"a\u0315\u0300", "à\u0315", true}, {"가", "\u1100\u1161", true},
		{"É", "E", false}, {"A", "a", false}, {"Ａ", "A", false},
		{"a\u034f\u0301", "á", false}, {"", "", true},
	} {
		actual, err := Equivalent(tc.a, tc.b)
		if err != nil || actual != tc.equal {
			t.Errorf("%q, %q: %v %v", tc.a, tc.b, actual, err)
		}
	}
}

func TestLongNonstarterRuns(t *testing.T) {
	for _, n := range []int{29, 30, 31, 32, 60, 61, 4096, 65536} {
		// Exact nonstarter counts exercise both sides of the 30-scalar stream-safe boundary.
		exact := "a" + strings.Repeat("\u0301", n)
		exactWant := "á" + strings.Repeat("\u0301", n-1)
		for _, transform := range []func(string) (string, error){NFC, NFKCCasefold} {
			if got, err := transform(exact); err != nil || got != exactWant {
				t.Fatalf("exact %d nonstarters: %v", n, err)
			}
		}
		// Entire 232-class run must reorder behind the later 230-class run.
		in := "a" + strings.Repeat("\u0315", n) + strings.Repeat("\u0300", n)
		want := "à" + strings.Repeat("\u0300", n-1) + strings.Repeat("\u0315", n)
		for name, f := range map[string]func(string) (string, error){"nfc": NFC, "fold": NFKCCasefold} {
			got, err := f(in)
			if err != nil || got != want {
				t.Fatalf("%s length %d mismatch: %v", name, n, err)
			}
			if strings.ContainsRune(got, '\u034f') {
				t.Fatalf("%s inserted CGJ", name)
			}
		}
		// Equal CCC is stable and blocks later composition.
		blocked := "a" + strings.Repeat("\u0305", n) + "\u0301"
		if got, _ := NFC(blocked); got != blocked {
			t.Fatalf("blocking %d", n)
		}
		// Leading nonstarters are sorted without inventing a starter.
		leading := strings.Repeat("\u0315", n) + strings.Repeat("\u0300", n)
		if got, _ := NFC(leading); got != strings.Repeat("\u0300", n)+strings.Repeat("\u0315", n) {
			t.Fatalf("leading %d", n)
		}
	}
}

func TestStableOrderingAndCompositionExclusions(t *testing.T) {
	// U+0344 is a full exclusion due to its nonstarter decomposition.
	if got, _ := NFC("\u0344"); got != "\u0308\u0301" {
		t.Fatalf("exclusion: %U", []rune(got))
	}
	if got, _ := NFC("a\u0315\u0301\u0300"); got != "á\u0300\u0315" {
		t.Fatalf("stability: %U", []rune(got))
	}
	if got, _ := NFC("\u1100\u1161\u11a8"); got != "각" {
		t.Fatal(got)
	}
	if got, _ := NFC("\u1100\u0301\u1161"); got != "\u1100\u0301\u1161" {
		t.Fatal("blocked Hangul")
	}
	if got, _ := NFKCCasefold("A\u0315\u034f\u0300"); got != "à\u0315" {
		t.Fatalf("DI reorder: %U", []rune(got))
	}
}

func TestParallelImmutableTables(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			for j := 0; j < 100; j++ {
				d, k, e := (Canonicalizer{}).Canonicalize(" \u0301E\u0301 Straße\t")
				if e != nil || d != "E\u0301 Straße" || k != "é strasse" {
					t.Errorf("%q %q %v", d, k, e)
					return
				}
			}
		})
	}
	wg.Wait()
}

func FuzzCanonical16(f *testing.F) {
	for _, s := range []string{"", "É", "A\u034f\u030a", " \u0301X", "\u0915\u094d\u0915", "👩\u200d👩", "\xff", strings.Repeat("\u0315\u0300", 61)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n, err := NFC(s)
		if !utf8.ValidString(s) {
			if !errors.Is(err, ErrInvalidUTF8) {
				t.Fatal(err)
			}
			return
		}
		if err != nil || !utf8.ValidString(n) {
			t.Fatal(err)
		}
		if n2, _ := NFC(n); n2 != n {
			t.Fatal("NFC not idempotent")
		}
		if nfd(s) != nfd(n) {
			t.Fatal("canonical equivalence lost")
		}
		fold, _ := NFKCCasefold(s)
		if again, _ := NFKCCasefold(fold); again != fold {
			t.Fatal("fold not idempotent")
		}
	})
}

// Split/join by EGC is deliberately one pass. Introducing ASCII SPACE can
// change segmentation on a second pass; Swift Character split/join has the
// same semantic issue, which a target Swift oracle must verify independently.
func TestDisplayNormalizationIsSinglePass(t *testing.T) {
	first, _ := DisplayName("a\n\u0301b")
	second, _ := DisplayName(first)
	publicKey, _ := CanonicalKey("a\n\u0301b")
	entityDisplay, entityKey, err := (Canonicalizer{}).Canonicalize("a\n\u0301b")
	if publicKey != "a \u0301b" || entityDisplay != first || entityKey != "a b" || err != nil {
		t.Fatalf("call chain: %q %q %q %v", publicKey, entityDisplay, entityKey, err)
	}
	if first != "a \u0301b" || second != "a b" {
		t.Fatalf("single pass: %q then %q", first, second)
	}
}

func BenchmarkReverseNonstarterRun(b *testing.B) {
	for _, n := range []int{4096, 65536} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			input := "a" + strings.Repeat("\u0315", n) + strings.Repeat("\u0300", n)
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := NFC(input); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestLongGraphemeContext(t *testing.T) {
	const n = 65536
	cases := []struct {
		input    string
		clusters int
	}{
		{strings.Repeat("🇸", n), n / 2},
		{"👩" + strings.Repeat("\u0301", n) + "\u200d👩", 1},
		{"\u0915" + strings.Repeat("\u094d\u093c", n) + "\u0915", 1},
		{strings.Repeat("\u0600", n) + " \u0301X", 2},
	}
	for index, tc := range cases {
		count := 0
		visitClusters(tc.input, func(_, _ int) { count++ })
		if count != tc.clusters {
			t.Fatalf("long context %d: got %d clusters want %d", index, count, tc.clusters)
		}
	}
	if display, err := DisplayName(" " + strings.Repeat("\u0301", n) + "X"); err != nil || display != "X" {
		t.Fatalf("long whitespace EGC: %q %v", display, err)
	}
}
