// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
)

func TestReviewCompletionNativeFailureRemainsVisible(t *testing.T) {
	f := newTitleFixture(t, true)
	f.edit("存储成功但原生失效")
	f.advance(700 * time.Millisecond)
	f.s.titleEditor.native.handle = ui.Handle{}
	f.all()
	if f.s.runtimeError == "" {
		t.Fatalf("native failure hidden; notice=%s state=%+v", f.s.runtimeNotice, f.s.titleEditor.state)
	}
}
