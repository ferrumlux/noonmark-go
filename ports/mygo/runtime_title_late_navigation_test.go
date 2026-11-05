// SPDX-License-Identifier: AGPL-3.0-only
package main

import "testing"

func TestReviewNewestNavigationBeforeOlderCompletionDispatch(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("标题")
	f.s.titleTransition(func() { f.s.page = "calendar" })
	f.ui()
	f.io.one()
	f.s.titleTransition(func() { f.s.page = "pool" })
	f.all()
	if f.s.page != "pool" {
		t.Fatalf("latest navigation dropped, page=%s targets=%d", f.s.page, len(f.h.targets))
	}
}
