// SPDX-License-Identifier: AGPL-3.0-only
package canonical

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) Value {
	t.Helper()
	v, e := Parse([]byte(s), Limits{})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestParseRejectsAdversarialJSON(t *testing.T) {
	cases := []string{
		`{"x":1,"x":2}`, `{"a":[{"x":1,"\u0078":2}]}`,
		`{"a":1,"\u0061":2}`, `"\ud800"`, `"\udfff"`, `"\ud800x"`,
		`"\ud800\u0041"`, `"\ud800\ud800"`, `"\uZZZZ"`,
		string([]byte{'"', 0xff, '"'}), string([]byte{'"', 0xed, 0xa0, 0x80, '"'}),
		`{}{}`, `[1,]`, `{"a":}`, `01`, `+1`, `1.`, `NaN`, `Infinity`, `"\x"`,
		"\xef\xbb\xbf{}", `{"a":1,}`, "", `"unterminated`,
	}
	for _, s := range cases {
		t.Run(strconv.Quote(s), func(t *testing.T) {
			if _, e := Parse([]byte(s), Limits{}); e == nil {
				t.Fatalf("accepted %q", s)
			}
		})
	}
}
func TestParseUnicodeAndExactNumbers(t *testing.T) {
	v := mustParse(t, `{"escaped":"\ud83d\ude00","literal":"😀","bits":18446744073709551615,"float":1.234567890123456789e+99,"literalEscape":"\\ud800"}`)
	if v.Object["escaped"].String != "😀" || v.Object["literalEscape"].String != `\ud800` {
		t.Fatal(v)
	}
	n, e := strconv.ParseUint(v.Object["bits"].TextNumber, 10, 64)
	if e != nil || n != ^uint64(0) {
		t.Fatal(n, e)
	}
	if v.Object["float"].TextNumber != "1.234567890123456789e+99" {
		t.Fatal("precision loss")
	}
}
func TestLimits(t *testing.T) {
	cases := []struct {
		s string
		l Limits
	}{
		{"null", Limits{MaxBytes: 3}},
		{"[[0]]", Limits{MaxDepth: 2}},
		{`{"a":0}`, Limits{MaxNodes: 2}},
		{"[0,1]", Limits{MaxNodes: 2}},
		{"null", Limits{MaxNodes: -1}},
		{"null", Limits{MaxDepth: 1025}},
	}
	for _, c := range cases {
		if _, e := Parse([]byte(c.s), c.l); e == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
	if _, e := Parse([]byte(`{"a":0}`), Limits{MaxBytes: 7, MaxDepth: 2, MaxNodes: 3}); e != nil {
		t.Fatal(e)
	}
	if _, e := Parse([]byte(strings.Repeat("[", 10000)+"0"+strings.Repeat("]", 10000)), Limits{}); e == nil {
		t.Fatal("deep input accepted")
	}
}

// Hand-authored candidate golden, NOT generated or checked by Swift/Foundation.
func TestHandAuthoredCandidateGolden(t *testing.T) {
	v := mustParse(t, `{"z":18446744073709551615,"a":["/<>😀",true,null,{},[]],"control":"\u0000\b\f\n\r\t\"\\"}`)
	want := "{\n  \"a\" : [\n    \"/<>😀\",\n    true,\n    null,\n    {},\n    []\n  ],\n  \"control\" : \"\\u0000\\b\\f\\n\\r\\t\\\"\\\\\",\n  \"z\" : 18446744073709551615\n}"
	b, e := Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != want {
		t.Fatalf("got %s\nwant %s", b, want)
	}
	if _, e = ValidateCanonical(b, Limits{}); e != nil {
		t.Fatal(e)
	}
	b2, e := Marshal(mustParse(t, string(b)))
	if e != nil || !bytes.Equal(b, b2) {
		t.Fatal("roundtrip", e)
	}
	for _, modified := range [][]byte{append(append([]byte{}, b...), '\n'), []byte(strings.ReplaceAll(string(b), " : ", ":")), []byte(`{"z":1,"a":2}`)} {
		if _, e := ValidateCanonical(modified, Limits{}); !errors.Is(e, ErrNonCanonical) {
			t.Fatalf("canonical mutation accepted: %v", e)
		}
	}
}
func TestUnsupportedNumbersFailClosed(t *testing.T) {
	for _, s := range []string{"1.0", "1e0", "-0", "1e9999"} {
		v := mustParse(t, s)
		if _, e := Marshal(v); !errors.Is(e, ErrUnsupportedNumber) {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"0", "-1", "18446744073709551615", "9007199254740993"} {
		b, e := Marshal(mustParse(t, s))
		if e != nil || string(b) != s {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"", "01", "null", "1 2", "1\n", "-"} {
		if _, e := Marshal(Value{Kind: Number, TextNumber: s}); e == nil {
			t.Fatal(s)
		}
	}
}
func TestShapeMultisetAndLengths(t *testing.T) {
	a := mustParse(t, `[{"é":1},{"a":true},{"é":2}]`)
	b := mustParse(t, `[{"é":9},{"é":8},{"a":false}]`)
	sa, e := Shape(a)
	if e != nil {
		t.Fatal(e)
	}
	sb, e := Shape(b)
	if e != nil || sa != sb {
		t.Fatal(sa, sb, e)
	}
	sc, _ := Shape(mustParse(t, `[{"é":1},{"a":true}]`))
	if sa == sc {
		t.Fatal("lost multiplicity")
	}
	sd, _ := Shape(mustParse(t, `[{"é":true},{"a":1},{"é":2}]`))
	if sa == sd {
		t.Fatal("lost nested kinds")
	}
	s, e := Shape(mustParse(t, `{"é":1}`))
	if e != nil || s != "object12:2:é6:number" {
		t.Fatal(s, e)
	}
	s, e = Shape(mustParse(t, `[null,true,1,"x"]`))
	if e != nil || s != "array31:7:boolean4:null6:number6:string" {
		t.Fatal(s, e)
	}
}
func TestConstructedValueSafety(t *testing.T) {
	v := Value{Kind: Object, Object: map[string]Value{}}
	v.Object["loop"] = v
	if _, e := Marshal(v); e == nil {
		t.Fatal("cycle marshal")
	}
	if _, e := Shape(v); e == nil {
		t.Fatal("cycle shape")
	}
	if _, e := Marshal(Value{Kind: String, String: string([]byte{0xff})}); e == nil {
		t.Fatal("invalid string")
	}
	if _, e := Marshal(Value{Kind: Kind("invalid")}); e == nil {
		t.Fatal("invalid kind")
	}
}
func FuzzParseRoundtrip(f *testing.F) {
	for _, s := range []string{`{}`, `{"n":18446744073709551615}`, `["\ud83d\ude00"]`, `{"a":1,"a":2}`, `[[[null]]]`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		v, e := Parse(b, Limits{MaxBytes: 8192, MaxDepth: 32, MaxNodes: 1024})
		if e != nil {
			return
		}
		c, e := Marshal(v)
		if e != nil {
			return
		}
		v2, e := ValidateCanonical(c, Limits{})
		if e != nil {
			t.Fatal(e)
		}
		c2, e := Marshal(v2)
		if e != nil || !bytes.Equal(c, c2) {
			t.Fatal("unstable candidate", e)
		}
	})
}

func TestRejectInconsistentValueUnion(t *testing.T) {
	cycle := map[string]Value{}
	cycle["self"] = Value{Kind: Object, Object: cycle}
	hidden := Value{Kind: String, String: "x", Object: cycle}
	invalid := []Value{
		hidden,
		{Kind: Null, Object: map[string]Value{}},
		{Kind: Null, Array: []Value{}},
		{Kind: Object, Object: map[string]Value{}, String: "hidden"},
		{Kind: Array, Array: []Value{}, TextNumber: "1"},
		{Kind: String, String: "x", Bool: true},
		{Kind: Number, TextNumber: "1", String: "hidden"},
		{Kind: Boolean, Bool: false, TextNumber: "0"},
		{Kind: Object, Object: map[string]Value{"nested": hidden}},
		{Kind: Array, Array: []Value{hidden}},
	}
	for i, v := range invalid {
		if _, err := Marshal(v); err == nil {
			t.Errorf("Marshal accepted inconsistent union %d", i)
		}
		if _, err := Shape(v); err == nil {
			t.Errorf("Shape accepted inconsistent union %d", i)
		}
	}
	// Empty active containers, empty strings and false are legitimate variants.
	valid := []Value{
		{Kind: Null}, {Kind: Object}, {Kind: Object, Object: map[string]Value{}},
		{Kind: Array}, {Kind: Array, Array: []Value{}}, {Kind: String},
		{Kind: Boolean}, {Kind: Boolean, Bool: true}, {Kind: Number, TextNumber: "0"},
	}
	for i, v := range valid {
		if _, err := Marshal(v); err != nil {
			t.Errorf("Marshal rejected valid union %d: %v", i, err)
		}
		if _, err := Shape(v); err != nil {
			t.Errorf("Shape rejected valid union %d: %v", i, err)
		}
	}
}
