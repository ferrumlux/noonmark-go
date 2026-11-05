// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"reflect"
	"strings"
	"testing"
)

// A real CGJ is not canonically equivalent to an inserted stream-safe boundary.
func TestTitleStreamSafeCollisionMustMutate(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, reverse := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			old, next := a, b
			if reverse {
				old, next = b, a
			}
			g := baseGraph()
			g.Definitions[0].Title = old
			k := kernel(t, g)
			in := titleInput()
			in.Title = next
			if err := titleCommand(k, reuse, in); err != nil {
				t.Fatal(err)
			}
			got := k.Snapshot()
			if got.Definitions[len(got.Definitions)-1].Title != next {
				t.Fatalf("distinct CGJ title swallowed: reverse=%v reuse=%v", reverse, reuse)
			}
			if got.Chains[0].UpdatedAt != in.Now {
				t.Fatal("mutation did not update clock")
			}
			if !reuse && (len(got.Definitions) != 2 || got.Definitions[0].Title != old) {
				t.Fatal("rename failed to preserve historical bytes")
			}
		}
	}
}

func TestTitleInvalidUTF8UpdateRejectsWithoutMutation(t *testing.T) {
	for _, input := range []string{string([]byte{0xff}), "new" + string([]byte{0xc3}), "\xe2\x82", "\xed\xa0\x80"} {
		for _, reuse := range []bool{false, true} {
			t.Run("invalid", func(t *testing.T) {
				g := baseGraph()
				k := kernel(t, g)
				in := titleInput()
				in.Title = input
				err := titleCommand(k, reuse, in)
				if err == nil {
					t.Error("invalid UTF-8 title update accepted")
				}
				if !reflect.DeepEqual(g, k.Snapshot()) {
					t.Error("invalid UTF-8 changed graph")
				}
			})
		}
	}
}
