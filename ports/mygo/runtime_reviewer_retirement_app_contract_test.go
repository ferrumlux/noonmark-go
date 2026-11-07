package main

import (
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

// This is a legal Changed-hook reentry contract test, not a claim that current
// OS native events perform this exact interleaving. The native title is real.
func TestReviewerRetirementAppFenceMustNotAbandonCommittedOwnerRemoval(t *testing.T) {
	f := newTitleFixture(t, true)
	f.all()
	e := f.s.titleEditor
	if e == nil {
		t.Fatal("missing real title editor")
	}
	armed := false
	c, err := coordinator.New(coordinator.Config{
		Service: titleServiceAdapter{f.h}, TrySubmit: f.h.trySubmit, TryPostProducer: f.h.tryPostProducer, Clock: &f.clock,
		Today: func() coordinator.Date {
			return coordinator.Date{Year: liveTestDay.Year, Month: liveTestDay.Month, Day: liveTestDay.Day}
		},
		Permanent: func(error) bool { return false }, TraceLocked: func(error) bool { return true },
		Changed: func(r coordinator.Registration, st titlesession.State) {
			f.h.changed(r, st)
			if armed {
				armed = false
				f.s.titleTransition(func() { f.s.page = "pool" })
			}
		}, Failed: f.h.failed, Navigate: func(string, string) {}, ReenterClose: func(string) {}, ReenterQuit: func() {},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.h.coordinator = c
	reg, err := c.Open(f.s.titleWindowID, "task:"+string(e.row.ChainID)+":title", string(e.row.TraceID), e.row.Title, &e.native)
	if err != nil {
		t.Fatal(err)
	}
	e.reg = reg
	e.identity, _ = c.Identity(reg)
	e.registered = true
	if err = c.LockTrace(reg); err != nil {
		t.Fatal(err)
	}
	e.locked = true
	f.s.disposeTitleEditors()
	armed = true
	// Only automatic disposal executes. Reentered app navigation is still posted,
	// so its coordinator intent does not yet exist when retirement commits.
	f.posts.one()
	_, exists := c.State(reg)
	if !exists && f.s.titleEditor == e && e.registered {
		t.Fatal("retirement committed but changed app fence left disposed registered owner mounted")
	}
}
