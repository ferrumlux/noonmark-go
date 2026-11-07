// SPDX-License-Identifier: AGPL-3.0-only
package canonical

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestQuoteFastPathScope(t *testing.T) {
	source, err := os.ReadFile("canonical.go")
	if err != nil {
		t.Fatal(err)
	}
	const insertion = `	// Valid UTF-8 bytes above ASCII cannot need JSON escaping. Write an
	// escape-free payload once, rather than one bounded buffer write per rune.
	// If the payload cannot fit, retain the original partial-write/error path.
	if len(s) <= w.lim.MaxBytes-w.b.Len() {
		i := 0
		for i < len(s) && s[i] >= 0x20 && s[i] != '"' && s[i] != '\\' {
			i++
		}
		if i == len(s) {
			if e := w.put(s); e != nil {
				return e
			}
			return w.put("\"")
		}
	}
`
	if strings.Count(string(source), insertion) != 1 {
		t.Fatal("unexpected fast path scope")
	}
	before := strings.Replace(string(source), insertion, "", 1)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(before))); got != "6076f80103b6ea549df109a45722138918a42dd11c6c69757adbfbcb5cc100e8" {
		t.Fatal("unreviewed canonical change", got)
	}
	oracle, err := os.ReadFile("quote_before_test.go")
	if err != nil {
		t.Fatal(err)
	}
	frozen := strings.TrimSpace(string(oracle[strings.Index(string(oracle), "func (w *writer) quoteBefore("):]))
	frozen = strings.Replace(frozen, "quoteBefore(", "quote(", 1)
	start := strings.Index(before, "func (w *writer) quote(")
	end := strings.Index(before[start:], "\nfunc integer(") + start
	if frozen != strings.TrimSpace(before[start:end]) {
		t.Fatal("old quote oracle changed")
	}
}

func quoteFastCompare(t *testing.T, s, prefix string, max int) {
	t.Helper()
	a, b := writer{lim: Limits{MaxBytes: max}}, writer{lim: Limits{MaxBytes: max}}
	a.b.WriteString(prefix)
	b.b.WriteString(prefix)
	oldErr, err := a.quoteBefore(s), b.quote(s)
	text := func(e error) string {
		if e != nil {
			return e.Error()
		}
		return ""
	}
	if reflect.TypeOf(oldErr) != reflect.TypeOf(err) || text(oldErr) != text(err) || !bytes.Equal(a.b.Bytes(), b.b.Bytes()) || a.nodes != b.nodes {
		t.Fatalf("quote differs s=%q prefix=%q max=%d: old=%q/%v new=%q/%v", s, prefix, max, a.b.Bytes(), oldErr, b.b.Bytes(), err)
	}
	if max >= len(prefix) && b.b.Len() > max {
		t.Fatal("output crossed limit")
	}
}

func TestQuoteFastPathDifferential(t *testing.T) {
	cases := []string{"", "key", "00000000-0000-4000-8000-000000000001", "合成年度任务 · 😀", "/<>é e\u0301 \u2028\u2029 \ufeff", "prefix\x00middle\nend", "\xff", "\xc0\xaf", "\xed\xa0\x80", "\xf4\x90\x80\x80", "\xe2\x82", strings.Repeat("说明😀", 64)}
	for n := 0; n < 256; n++ {
		cases = append(cases, string([]byte{byte(n)}), "a"+string([]byte{byte(n)})+"z")
	}
	for _, r := range []rune{0, 31, 32, 127, 128, 255, 0x7ff, 0x800, 0xd7ff, 0xe000, 0xfffd, 0xffff, 0x10000, 0x10ffff} {
		cases = append(cases, string(r), "x"+string(r)+"y")
	}
	random := rand.New(rand.NewSource(824966))
	alphabet := []rune("ASCII 019 /<>é说明😀\u0301\u2028\u2029\x00\b\f\n\r\t\"\\")
	for n := 0; n < 300; n++ {
		var b strings.Builder
		for i := 0; i < random.Intn(48); i++ {
			b.WriteRune(alphabet[random.Intn(len(alphabet))])
		}
		cases = append(cases, b.String())
	}
	for _, s := range cases {
		quoteFastCompare(t, s, "", DefaultLimits().MaxBytes)
		for _, prefix := range []string{"", "pre"} {
			for _, extra := range []int{0, 1, 2, 3} {
				quoteFastCompare(t, s, prefix, len(prefix)+len(s)+extra)
			}
		}
		// This includes exact-fit, a missing closing quote, mid-codepoint failure,
		// every escape boundary and pre-existing output. Preserve partial bytes too.
		for _, prefix := range []string{"", "pre"} {
			for max := -1; max <= len(s)*6+len(prefix)+3; max++ {
				if max > 300 && max < len(s)*6 {
					continue
				}
				quoteFastCompare(t, s, prefix, max)
			}
		}
	}
}

func TestQuoteFastPathOutputOwnership(t *testing.T) {
	v := Value{Kind: Object, Object: map[string]Value{"title": {Kind: String, String: "合成文字 /<>😀"}, "rows": {Kind: Array, Array: []Value{{Kind: String, String: strings.Repeat("A", 1024)}, {Kind: String, String: "a\n\tb"}}}}}
	want, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	old, err := Parse(want, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	a[0] = '!'
	if !bytes.Equal(b, want) {
		t.Fatal("outputs share mutable backing")
	}
	if !reflect.DeepEqual(v, old) {
		t.Fatal("Marshal changed input")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				out, e := Marshal(v)
				if e != nil || !bytes.Equal(out, want) {
					t.Errorf("parallel marshal changed output: %v", e)
					return
				}
				out[0] = '!'
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(v, old) {
		t.Fatal("parallel Marshal changed input")
	}
}

func TestQuoteFastPathAllocationReduction(t *testing.T) {
	s := strings.Repeat("unescaped合成😀", 4096)
	measure := func(f func(*writer, string) error) float64 {
		return testing.AllocsPerRun(3, func() {
			w := writer{lim: DefaultLimits()}
			if err := f(&w, s); err != nil {
				t.Fatal(err)
			}
		})
	}
	old, new := measure((*writer).quoteBefore), measure((*writer).quote)
	if new >= old {
		t.Fatalf("long escape-free buffer growth not reduced: old=%g new=%g", old, new)
	}
	t.Logf("fresh long-string buffer allocations old=%g new=%g", old, new)
}

func BenchmarkQuoteFastPathAB(b *testing.B) {
	for _, tc := range []struct{ name, s string }{{"key", "definitionID"}, {"uuid", "00000000-0000-4000-8000-000000000001"}, {"unicode", "合成年度任务 · 2026-11-05 😀"}, {"long", strings.Repeat("合成 note /<>😀 ", 256)}, {"late_escape", strings.Repeat("long unescaped 合成😀", 256) + "\n"}, {"escaped", strings.Repeat("has\nquote\"slash\\", 32)}} {
		for _, f := range []struct {
			name string
			fn   func(*writer, string) error
		}{{"before", (*writer).quoteBefore}, {"after", (*writer).quote}} {
			b.Run(tc.name+"/"+f.name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(tc.s)))
				for i := 0; i < b.N; i++ {
					w := writer{lim: DefaultLimits()}
					if err := f.fn(&w, tc.s); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func FuzzQuoteFastPathParity(f *testing.F) {
	for _, s := range []string{"", "key", "合成😀", "a\n\"\\", "\xff"} {
		f.Add(s, uint16(64), "pre")
	}
	f.Fuzz(func(t *testing.T, s string, max uint16, prefix string) {
		if len(s) > 4096 || len(prefix) > 64 {
			return
		}
		quoteFastCompare(t, s, prefix, int(max))
	})
}

func BenchmarkQuoteFastPathFallbackAB(b *testing.B) {
	plain := strings.Repeat("long unescaped 合成😀", 256)
	for _, tc := range []struct {
		name, s string
		max     int
	}{
		{"first_byte_escape", "\n" + plain, DefaultLimits().MaxBytes},
		{"last_byte_escape", plain + "\n", DefaultLimits().MaxBytes},
		{"payload_overflow", plain, len(plain) / 2},
		{"closing_quote_overflow", plain, len(plain) + 1},
	} {
		before, after := writer{lim: Limits{MaxBytes: tc.max}}, writer{lim: Limits{MaxBytes: tc.max}}
		oldErr, err := before.quoteBefore(tc.s), after.quote(tc.s)
		if fmt.Sprint(oldErr) != fmt.Sprint(err) || !bytes.Equal(before.b.Bytes(), after.b.Bytes()) {
			b.Fatal("fallback setup differs")
		}
		for _, f := range []struct {
			name string
			fn   func(*writer, string) error
		}{{"before", (*writer).quoteBefore}, {"after", (*writer).quote}} {
			b.Run(tc.name+"/"+f.name, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					w := writer{lim: Limits{MaxBytes: tc.max}}
					result := f.fn(&w, tc.s)
					if (result == nil) != (oldErr == nil) {
						b.Fatal(result)
					}
				}
			})
		}
	}
}
