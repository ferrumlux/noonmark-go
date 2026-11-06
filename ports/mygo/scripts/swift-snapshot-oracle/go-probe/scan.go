// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type limits struct{ Bytes, Depth, Nodes, String, TotalString, Members, Array int }

var rawLimits = limits{262144, 24, 8192, 4096, 65536, 32, 96}

type scanner struct {
	b               []byte
	i, nodes, total int
	l               limits
}

func fail(s string) error { return fmt.Errorf("%s", s) }
func (s *scanner) ws() {
	for s.i < len(s.b) && bytes.IndexByte([]byte(" \r\n\t"), s.b[s.i]) >= 0 {
		s.i++
	}
}
func (s *scanner) node() error {
	s.nodes++
	if s.nodes > s.l.Nodes {
		return fail("resource-limit:nodes")
	}
	return nil
}
func (s *scanner) str() (string, error) {
	start := s.i
	s.i++
	decoded := 0
	for s.i < len(s.b) {
		c := s.b[s.i]
		s.i++
		if c == '"' {
			var out string
			if e := json.Unmarshal(s.b[start:s.i], &out); e != nil {
				return "", e
			}
			s.total += len(out)
			if len(out) > s.l.String || s.total > s.l.TotalString {
				return "", fail("resource-limit:string")
			}
			return out, nil
		}
		if c < 32 {
			return "", fail("schema-invalid:control")
		}
		if c == '\\' {
			if s.i >= len(s.b) {
				break
			}
			e := s.b[s.i]
			s.i++
			if e == 'u' {
				if s.i+4 > len(s.b) {
					break
				}
				n, err := strconv.ParseUint(string(s.b[s.i:s.i+4]), 16, 16)
				if err != nil {
					return "", err
				}
				s.i += 4
				if n >= 0xd800 && n <= 0xdbff {
					if s.i+6 > len(s.b) || string(s.b[s.i:s.i+2]) != "\\u" {
						return "", fail("schema-invalid:surrogate")
					}
					m, er := strconv.ParseUint(string(s.b[s.i+2:s.i+6]), 16, 16)
					if er != nil || m < 0xdc00 || m > 0xdfff {
						return "", fail("schema-invalid:surrogate")
					}
					s.i += 6
					decoded += 4
				} else if n >= 0xdc00 && n <= 0xdfff {
					return "", fail("schema-invalid:surrogate")
				} else if n < 128 {
					decoded++
				} else if n < 2048 {
					decoded += 2
				} else {
					decoded += 3
				}
			} else if strings.ContainsRune(`"\/bfnrt`, rune(e)) {
				decoded++
			} else {
				return "", fail("schema-invalid:escape")
			}
		} else {
			decoded++
		}
		if decoded > s.l.String {
			return "", fail("resource-limit:string")
		}
	}
	return "", fail("schema-invalid:string")
}
func (s *scanner) value(depth int) error {
	s.ws()
	if depth > s.l.Depth {
		return fail("resource-limit:depth")
	}
	if e := s.node(); e != nil {
		return e
	}
	if s.i >= len(s.b) {
		return fail("schema-invalid:eof")
	}
	switch s.b[s.i] {
	case '"':
		_, e := s.str()
		return e
	case '{':
		s.i++
		s.ws()
		seen := map[string]bool{}
		if s.i < len(s.b) && s.b[s.i] == '}' {
			s.i++
			return nil
		}
		for {
			if len(seen) >= s.l.Members {
				return fail("resource-limit:members")
			}
			s.ws()
			if s.i >= len(s.b) || s.b[s.i] != '"' {
				return fail("schema-invalid:key")
			}
			if e := s.node(); e != nil {
				return e
			}
			k, e := s.str()
			if e != nil {
				return e
			}
			if seen[k] {
				return fail("schema-invalid:duplicate")
			}
			seen[k] = true
			s.ws()
			if s.i >= len(s.b) || s.b[s.i] != ':' {
				return fail("schema-invalid:colon")
			}
			s.i++
			if e = s.value(depth + 1); e != nil {
				return e
			}
			s.ws()
			if s.i >= len(s.b) {
				return fail("schema-invalid:eof")
			}
			c := s.b[s.i]
			s.i++
			if c == '}' {
				return nil
			}
			if c != ',' {
				return fail("schema-invalid:comma")
			}
		}
	case '[':
		s.i++
		s.ws()
		if s.i < len(s.b) && s.b[s.i] == ']' {
			s.i++
			return nil
		}
		for n := 0; ; n++ {
			if n >= s.l.Array {
				return fail("resource-limit:array")
			}
			if e := s.value(depth + 1); e != nil {
				return e
			}
			s.ws()
			if s.i >= len(s.b) {
				return fail("schema-invalid:eof")
			}
			c := s.b[s.i]
			s.i++
			if c == ']' {
				return nil
			}
			if c != ',' {
				return fail("schema-invalid:comma")
			}
		}
	case 't', 'f':
		for _, w := range []string{"true", "false"} {
			if bytes.HasPrefix(s.b[s.i:], []byte(w)) {
				s.i += len(w)
				return nil
			}
		}
		return fail("schema-invalid:boolean")
	default:
		start := s.i
		if s.b[s.i] == '-' {
			s.i++
		}
		digits := s.i
		for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
			s.i++
		}
		if s.i == digits || s.i-digits > 20 || (s.i-digits > 1 && s.b[digits] == '0') {
			return fail("schema-invalid:integer")
		}
		v := string(s.b[start:s.i])
		if strings.HasPrefix(v, "-") {
			if _, e := strconv.ParseInt(v, 10, 64); e != nil {
				return e
			}
		} else if _, e := strconv.ParseUint(v, 10, 64); e != nil {
			return e
		}
		return nil
	}
}
func scanOnly(b []byte, l limits) error {
	if len(b) > l.Bytes {
		return fail("resource-limit:bytes")
	}
	if !utf8.Valid(b) {
		return fail("schema-invalid:utf8")
	}
	s := scanner{b: b, l: l}
	if e := s.value(1); e != nil {
		return e
	}
	s.ws()
	if s.i != len(b) {
		return fail("schema-invalid:trailing")
	}
	return nil
}
func parseBounded(b []byte, l limits) (any, error) { return parseIngress("internal", b, l) }
func parseIngress(entry string, b []byte, l limits) (any, error) {
	originalDigest := digest(b)
	mode := "normal"
	target := ingressFault != nil && ingressFault.Entry == entry && !ingressFault.Reached
	if target {
		ingressFault.Reached = true
		ingressFault.Active = true
		mode = ingressFault.Mode
		if ingressFault.Dimension != "positive" {
			b = ingressBytes(ingressFault.Dimension, l, b)
		}
	}
	if target {
		ingressFault.InputBytes = len(b)
		ingressFault.InputSHA256 = digest(b)
	}
	scan := func() error {
		if target {
			ingressEvent("scan")
		}
		e := scanOnly(b, l)
		if target {
			if e != nil {
				ingressEvent("reject")
			} else {
				ingressEvent("scan-pass")
			}
		}
		return e
	}
	materialize := func() (any, error) {
		if target {
			ingressEvent("materialize")
		}
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var v any
		e := d.Decode(&v)
		return v, e
	}
	if mode == "normal" {
		if e := scan(); e != nil {
			return nil, e
		}
		v, e := materialize()
		if e == nil && strings.HasPrefix(entry, "config-") && digest(b) != originalDigest {
			ingressEvent("profile-reject")
			return nil, fail("registry-mismatch")
		}
		return v, e
	}
	v, e := materialize()
	if e != nil {
		return nil, e
	}
	if mode == "late" {
		if e = scan(); e != nil {
			return nil, e
		}
	}
	return v, nil
}
