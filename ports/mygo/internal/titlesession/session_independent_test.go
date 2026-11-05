// SPDX-License-Identifier: AGPL-3.0-only
package titlesession_test

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

func TestReviewOrdinaryEndEditingDuringSaveLeavesBlankDirty(t *testing.T) {
	s := session(t, "old")
	r := start(t, s, job(t, native(s, "A", false), 700), "A", "A")
	native(s, "  ", false)
	s.RequestFlush(s.Identity())
	u := succeed(s, r, "A")
	b := s.BeginPersistence(job(t, u, 0))
	if b.Request != nil || b.Update.DraftReplacement.Present || s.State().Draft != "  " || !s.State().Dirty || s.ScheduledJob() != nil {
		t.Fatalf("unexpected state %+v", s.State())
	}
	// A new explicit lifecycle request can finish the retained blank safely.
	b = s.BeginPersistence(job(t, lifecycle(s, "  ", false, true), 0))
	if !b.Update.SnapshotIsFlushed() || b.Update.DraftReplacement.Text != "A" {
		t.Fatal("lifecycle could not recover")
	}
	_ = titlesession.LifecycleFlushClean
}
