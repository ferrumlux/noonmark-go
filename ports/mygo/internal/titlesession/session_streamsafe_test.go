// SPDX-License-Identifier: AGPL-3.0-only
package titlesession_test

import (
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

func TestStreamSafeCollisionDraftIsNotSwallowed(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		s := session(t, pair[0])
		r := start(t, s, job(t, native(s, pair[1], false), 700), pair[1], pair[1])
		succeed(s, r, pair[1])
		texts(t, s, pair[1], pair[1], pair[1], false, false)
	}
}
func TestStreamSafeCollisionNewInputDuringSaveIsRetained(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		s := session(t, "old")
		r := start(t, s, job(t, native(s, pair[0], false), 700), pair[0], pair[0])
		native(s, pair[1], false)
		u := succeed(s, r, pair[0])
		texts(t, s, pair[1], pair[1], pair[0], true, false)
		r2 := start(t, s, job(t, u, 700), pair[1], pair[1])
		succeed(s, r2, pair[1])
		texts(t, s, pair[1], pair[1], pair[1], false, false)
	}
}

func TestStreamSafeCollisionWrongAcknowledgmentCannotComplete(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		s := session(t, "old")
		r := start(t, s, job(t, native(s, pair[0], false), 700), pair[0], pair[0])
		u := succeed(s, r, pair[1])
		if u.Err != titlesession.ErrVerificationMismatch || !u.State.AwaitingVerification || u.DraftReplacement.Present {
			t.Fatal("different real CGJ acknowledgment accepted")
		}
		texts(t, s, pair[0], pair[0], "old", true, true)
		succeed(s, r, pair[0])
		texts(t, s, pair[0], pair[0], pair[0], false, false)
	}
}
