package titlecoordinator

import (
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"testing"
)

type reviewerFinishHook struct {
	*native
	hook func()
}

func (n *reviewerFinishHook) Finish() (titlesession.NativeSnapshot, bool) {
	s, ok := n.native.Finish()
	if fn := n.hook; fn != nil {
		n.hook = nil
		fn()
	}
	return s, ok
}

func TestReviewerRetirementNativeReentryPreservesUserIntent(t *testing.T) {
	for _, kind := range []string{"navigate", "close", "quit", "register"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			o, n := retirement(t, f, "w")
			if kind == "close" || kind == "quit" {
				busy := &native{s: titlesession.NativeSnapshot{Text: "unsaved busy text"}}
				reg, err := f.c.Open("w", "busy-owner", "busy-trace", "old", busy)
				if err != nil {
					t.Fatal(err)
				}
				f.observe(t, reg)
			}
			holder := &reviewerFinishHook{native: n}
			f.c.find(o.Registration).native = holder
			var added Registration
			holder.hook = func() {
				switch kind {
				case "navigate":
					f.c.RequestNavigation("w", "native-user")
				case "close":
					f.c.BeforeClose("w")
				case "quit":
					f.c.BeforeQuit()
				case "register":
					var err error
					added, err = f.c.Open("w", "new-native-owner", "new-trace", "new", &native{s: titlesession.NativeSnapshot{Text: "new"}})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			retired, _ := f.c.TryRetireEditors("w", []RetirementOwner{o})
			if retired {
				t.Fatal("automatic retirement claimed success after native reentry")
			}
			switch kind {
			case "navigate":
				if len(f.nav) != 1 || f.nav[0] != "w:native-user" {
					t.Fatal("new navigation swallowed", f.nav)
				}
			case "close":
				if f.c.intents["w"] == nil {
					t.Fatal("pending close swallowed")
				}
			case "quit":
				if f.c.pending == nil {
					t.Fatal("pending quit swallowed")
				}
			case "register":
				if _, ok := f.c.State(added); !ok {
					t.Fatal("new owner removed")
				}
				if _, ok := f.c.State(o.Registration); !ok {
					t.Fatal("old set partially retired")
				}
			}
			if len(f.post) != 0 {
				t.Fatal("automatic retirement scheduled expanding retry")
			}
		})
	}
}

func TestReviewerRetirementFinalWholeSetRecheckPreventsPartialRetirement(t *testing.T) {
	f := setup(t)
	first, n := retirement(t, f, "w")
	secondNative := &native{s: titlesession.NativeSnapshot{Text: "second"}}
	r, err := f.c.Open("w", "second-owner", "second-trace", "second", secondNative)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.c.LockTrace(r); err != nil {
		t.Fatal(err)
	}
	id, _ := f.c.Identity(r)
	second := RetirementOwner{r, id}
	f.c.cfg.Changed = func(reg Registration, _ titlesession.State) {
		if reg == second.Registration {
			f.c.cfg.Changed = nil
			n.s.Text = "late first-owner text"
			_ = f.c.Observe(first.Registration, first.Identity)
		}
	}
	if ok, _ := f.c.TryRetireEditors("w", []RetirementOwner{first, second}); ok {
		t.Fatal("partial/stale clean proof accepted")
	}
	for _, o := range []RetirementOwner{first, second} {
		if _, ok := f.c.State(o.Registration); !ok {
			t.Fatal("partially retired original set")
		}
	}
	if len(f.l.q) != 0 || len(f.post) != 0 {
		t.Fatal("cleanup created writes/retry")
	}
}
