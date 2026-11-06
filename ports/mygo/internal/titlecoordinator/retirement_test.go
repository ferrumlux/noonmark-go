// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

func retirement(t *testing.T, f *fixture, w string) (RetirementOwner, *native) {
	t.Helper()
	r, n := f.open(t, w)
	if err := f.c.LockTrace(r); err != nil {
		t.Fatal(err)
	}
	id, _ := f.c.Identity(r)
	return RetirementOwner{r, id}, n
}
func TestRetirementExactCleanNoNavigation(t *testing.T) {
	f := setup(t)
	o, _ := retirement(t, f, "w")
	ok, err := f.c.TryRetireEditors("w", []RetirementOwner{o})
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, exists := f.c.State(o.Registration); exists || len(f.nav) != 0 || len(f.l.q) != 0 || len(f.post) != 0 {
		t.Fatal("retirement created work or retained registration")
	}
}
func TestRetirementDirtyFaultStoppedRetain(t *testing.T) {
	for _, kind := range []string{"late", "native", "stopped", "stale"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			o, n := retirement(t, f, "w")
			switch kind {
			case "late":
				n.s.Text = "晚草稿"
			case "native":
				n.failFinish = true
			case "stopped":
				f.c.stopped.Store(true)
			case "stale":
				o.Identity = titlesession.Identity{}
			}
			if ok, _ := f.c.TryRetireEditors("w", []RetirementOwner{o}); ok {
				t.Fatal("unsafe retirement")
			}
			if _, exists := f.c.State(o.Registration); !exists || len(f.l.q) != 0 || len(f.post) != 0 || f.c.hasIntents() {
				t.Fatal("lost draft or created retry")
			}
		})
	}
}
func TestRetirementUserIntentPriorityAndOtherWindow(t *testing.T) {
	for _, kind := range []string{"navigate", "close", "quit"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			o, n := retirement(t, f, "w")
			n.failFinish = true
			switch kind {
			case "navigate":
				f.c.RequestNavigation("w", "user")
			case "close":
				f.c.BeforeClose("w")
			case "quit":
				f.c.BeforeQuit()
			}
			// Native failure intentionally cancels an intent in the public path. Create
			// a held real dirty writer in another entry instead for pending ownership.
			n.failFinish = false
			r, other := f.open(t, "busy")
			other.s.Text = "new"
			f.observe(t, r)
			switch kind {
			case "navigate":
				f.c.RequestNavigation("busy", "user")
			case "close":
				f.c.BeforeClose("busy")
			case "quit":
				f.c.BeforeQuit()
			}
			before := n.finishes
			if kind == "quit" {
				if ok, _ := f.c.TryRetireEditors("w", []RetirementOwner{o}); ok || n.finishes != before {
					t.Fatal("quit overridden")
				}
			} else {
				if ok, err := f.c.TryRetireEditors("w", []RetirementOwner{o}); !ok || err != nil {
					t.Fatal("other window intent blocked", ok, err)
				}
				if f.c.intents["busy"] == nil {
					t.Fatal("other intent removed")
				}
			}
		})
	}
}
func TestRetirementReentrantRegistrationCancelsWithoutRetry(t *testing.T) {
	f := setup(t)
	o, _ := retirement(t, f, "w")
	var added Registration
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		var err error
		added, err = f.c.Open("w", "new", "new", "new", &native{s: titlesession.NativeSnapshot{Text: "new"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := f.c.TryRetireEditors("w", []RetirementOwner{o}); ok {
		t.Fatal("registry replacement accepted")
	}
	if _, ok := f.c.State(added); !ok {
		t.Fatal("new owner retired")
	}
	if _, ok := f.c.State(o.Registration); !ok {
		t.Fatal("old owner partially retired")
	}
	if len(f.post) != 0 {
		t.Fatal("retry can expand captured set")
	}
}
func TestRetirementReentrantUserNavigationWins(t *testing.T) {
	f := setup(t)
	o, _ := retirement(t, f, "w")
	f.c.cfg.Changed = func(Registration, titlesession.State) {
		f.c.cfg.Changed = nil
		f.c.RequestNavigation("w", "new-user-target")
	}
	f.c.TryRetireEditors("w", []RetirementOwner{o})
	if len(f.nav) != 1 || f.nav[0] != "w:new-user-target" {
		t.Fatal("lost user navigation", f.nav)
	}
	if len(f.post) != 0 {
		t.Fatal("cleanup retried")
	}
}

func TestRetirementSameWindowPendingIntentNoNativeRead(t *testing.T) {
	for _, kind := range []string{"navigate", "close", "quit"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			o, n := retirement(t, f, "w")
			dirty := &native{s: titlesession.NativeSnapshot{Text: "changed"}}
			r, err := f.c.Open("w", "busy", "busy", "old", dirty)
			if err != nil {
				t.Fatal(err)
			}
			f.observe(t, r)
			switch kind {
			case "navigate":
				f.c.RequestNavigation("w", "user")
			case "close":
				f.c.BeforeClose("w")
			case "quit":
				f.c.BeforeQuit()
			}
			before := n.finishes
			intent := f.c.intents["w"]
			pending := f.c.pending
			if ok, _ := f.c.TryRetireEditors("w", []RetirementOwner{o}); ok || n.finishes != before {
				t.Fatal("cleanup read while user pending")
			}
			if f.c.intents["w"] != intent || f.c.pending != pending {
				t.Fatal("cleanup modified user intent")
			}
		})
	}
}
