// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"reflect"
	"testing"
)

func TestReviewAddLockedCompletedParentPrecedence(t *testing.T) {
	for _, locked := range []bool{false, true} {
		for _, past := range []bool{false, true} {
			g, in := dayEditFixture()
			k := kernel(t, g)
			if err := k.MarkCompleted(in.TraceID, in.Today, in.Now); err != nil {
				t.Fatal(err)
			}
			g = k.Snapshot()
			if locked {
				g.Days[0].LockedAt = ptr(in.Now)
				g.Days[0].UpdatedAt = in.Now
			}
			if past {
				in.Today, _ = in.Today.AddDays(1)
			}
			k = kernel(t, g)
			before := k.Snapshot()
			want := ErrInvalidTransition
			if locked || past {
				want = ErrImmutableHistory
			}
			err := k.EditSubtask(in)
			if !errors.Is(err, want) {
				t.Fatalf("locked=%v past=%v: want %v got %v", locked, past, want, err)
			}
			if !reflect.DeepEqual(before, k.Snapshot()) {
				t.Fatal("rejected add changed graph")
			}
		}
	}
}
