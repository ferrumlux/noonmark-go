// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func TestTitleUIStreamSafeCollisionSavesDistinctDraft(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		f := newTitleFixture(t, true)
		f.edit(pair[0])
		f.advance(700 * time.Millisecond)
		f.all()
		if f.persisted() != pair[0] {
			t.Fatal("initial title bytes changed")
		}
		f.edit(pair[1])
		f.advance(700 * time.Millisecond)
		f.all()
		if f.persisted() != pair[1] || f.s.titleEditor.native.draft != pair[1] {
			t.Fatal("real UI swallowed distinct CGJ title")
		}
		if err := f.svc.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := taskapp.Open(f.path, liveTestDay)
		if err != nil {
			t.Fatal(err)
		}
		if reopened.Snapshot().Definitions[0].Title != pair[1] {
			t.Fatal("UI title disk bytes changed")
		}
		reopened.Close()
	}
}
func TestTitleUIStreamSafeCollisionNewerInputDuringSave(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		f := newTitleFixture(t, true)
		f.edit(pair[0])
		f.advance(700 * time.Millisecond)
		if len(f.io.q) != 1 {
			t.Fatal("save not queued")
		}
		f.edit(pair[1])
		f.io.one()
		f.ui()
		if f.s.titleEditor.native.draft != pair[1] || !f.s.titleEditor.state.Dirty {
			t.Fatal("old completion swallowed new CGJ input")
		}
		f.advance(700 * time.Millisecond)
		f.all()
		if f.persisted() != pair[1] {
			t.Fatal("new CGJ input never persisted")
		}
	}
}
