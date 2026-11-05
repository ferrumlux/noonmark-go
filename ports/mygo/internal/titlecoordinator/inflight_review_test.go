// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"reflect"
	"testing"
	"time"
)

// A new draft can have a zero-delay candidate while an older request still
// owns the gate. Repeated native lifecycle callbacks must wait, not submit
// that candidate and treat the expected gate rejection as a write failure.
func TestRepeatedTransitionsWaitForInflightTitle(t *testing.T) {
	for _, kind := range []string{"close", "quit", "navigate", "blur", "observe"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "w")
			n.s.Text = "A"
			f.observe(t, r)
			if f.c.BeforeClose("w") {
				t.Fatal("unpersisted A allowed close")
			}
			if len(f.l.q) != 1 {
				t.Fatal("A not queued")
			}
			n.s.Text = "AB"
			for range 3 {
				switch kind {
				case "close":
					if f.c.BeforeClose("w") {
						t.Fatal("inflight close accepted")
					}
				case "quit":
					if f.c.BeforeQuit() {
						t.Fatal("inflight quit accepted")
					}
				case "navigate":
					f.c.RequestNavigation("w", "pool")
				case "blur":
					if err := f.c.EndEditing(r); err != nil {
						t.Fatal(err)
					}
				case "observe":
					f.observe(t, r)
					f.advance(time.Second)
				}
			}
			if len(f.errs) != 0 || len(f.l.q) != 1 {
				t.Fatalf("pending request must wait: errors=%v queued=%d", f.errs, len(f.l.q))
			}
			f.finishSave(t)
			if len(f.l.q) != 1 || f.close != 0 || f.quit != 0 || len(f.nav) != 0 {
				t.Fatalf("A completion must schedule AB before transition: queued=%d close=%d quit=%d nav=%v", len(f.l.q), f.close, f.quit, f.nav)
			}
			f.finishSave(t)
			if got := f.s.calls; !reflect.DeepEqual(got, []string{"day:5", "trace-w:A", "day:5", "trace-w:AB"}) {
				t.Fatalf("captured writes: %v", got)
			}
			if len(f.errs) != 0 || len(f.l.q) != 0 {
				t.Fatalf("unexpected failure/extra work: errors=%v queued=%d", f.errs, len(f.l.q))
			}
			switch kind {
			case "quit":
				if f.quit != 1 || !f.c.BeforeQuit() {
					t.Fatal("latest quit intent lost")
				}
			case "navigate":
				if !reflect.DeepEqual(f.nav, []string{"w:pool"}) {
					t.Fatalf("latest navigation lost: %v", f.nav)
				}
			default:
				if f.close != 1 || !f.c.BeforeClose("w") {
					t.Fatal("close intent lost")
				}
			}
		})
	}
}
