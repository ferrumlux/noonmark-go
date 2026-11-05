// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

func TestReviewOrderedPublicationReentryCannotRestoreOlderTitle(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	id, _ := f.c.Identity(r)
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		// Both values were verified on the same writer lane, B then C. A hook
		// drains the next already-available publication without a nested event loop.
		if err := f.c.SynchronizePersistedText(r, id, "C newest"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.c.SynchronizePersistedText(r, id, "B older"); err != nil {
		t.Fatal(err)
	}
	s, _ := f.c.State(r)
	if s.LastAcknowledgedText != "C newest" || n.s.Text != "C newest" {
		t.Fatalf("older outer publication overwrote newer nested publication: state=%+v native=%q", s, n.s.Text)
	}
}
