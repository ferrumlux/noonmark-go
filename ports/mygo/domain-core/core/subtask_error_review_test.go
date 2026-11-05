// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"testing"
)

func TestReviewAddCompletedParentError(t *testing.T) {
	g, in := dayEditFixture()
	k := kernel(t, g)
	if err := k.MarkCompleted(in.TraceID, in.Today, in.Now); err != nil {
		t.Fatal(err)
	}
	err := k.EditSubtask(in)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Swift Engine:2613 nonpending add expects invalidTransition; got %v", err)
	}
}
