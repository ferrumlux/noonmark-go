// SPDX-License-Identifier: AGPL-3.0-only
package canonical

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// Frozen original from 824e966, independently kept for byte/error comparisons.
func (w *writer) quoteBefore(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("invalid UTF-8 string")
	}
	if e := w.put("\""); e != nil {
		return e
	}
	for _, r := range s {
		var x string
		switch r {
		case '"':
			x = "\\\""
		case '\\':
			x = "\\\\"
		case '\b':
			x = "\\b"
		case '\f':
			x = "\\f"
		case '\n':
			x = "\\n"
		case '\r':
			x = "\\r"
		case '\t':
			x = "\\t"
		default:
			if r < 32 {
				x = fmt.Sprintf("\\u%04x", r)
			} else {
				x = string(r)
			}
		}
		if e := w.put(x); e != nil {
			return e
		}
	}
	return w.put("\"")
}
