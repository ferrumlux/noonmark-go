// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
	"testing"
)

func TestTitleRetirementReconciliationPreservesReplacementOwner(t *testing.T) {
	for _, reusePointer := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-pointer", true: "same-pointer-new-registration"}[reusePointer], func(t *testing.T) {
			f := newTitleFixture(t, true)
			f.all()
			e := f.s.titleEditor
			if err := f.h.coordinator.LockTrace(e.reg); err != nil {
				t.Fatal(err)
			}
			e.locked = true
			r := &runtimeTitleDisposal{editors: []*runtimeTitleEditor{e}, owners: []coordinator.RetirementOwner{{Registration: e.reg, Identity: e.identity}}}
			if retired, err := f.h.coordinator.TryRetireEditors(f.s.titleWindowID, r.owners); !retired || err != nil {
				t.Fatal(retired, err)
			}
			next := new(runtimeTitleEditor)
			*next = *e
			if reusePointer {
				next = e
			}
			reg, err := f.h.coordinator.Open(f.s.titleWindowID, "replacement-owner", "replacement-trace", e.row.Title, &next.native)
			if err != nil {
				t.Fatal(err)
			}
			next.reg = reg
			next.identity, _ = f.h.coordinator.Identity(reg)
			next.registered = true
			f.s.titleEditor = next
			f.s.titleTarget = "new-user-target"
			f.h.targets[f.s.titleTarget] = func() {}
			nav := f.h.windows[f.s.titleWindowID].navigation
			f.s.reconcileTitleRetirement(r)
			if f.s.titleEditor != next || !next.registered {
				t.Fatal("new owner cleared")
			}
			if _, ok := f.h.coordinator.State(reg); !ok {
				t.Fatal("new registration removed")
			}
			if f.s.titleTarget != "new-user-target" || f.h.targets[f.s.titleTarget] == nil || f.h.windows[f.s.titleWindowID].navigation != nav {
				t.Fatal("new navigation changed")
			}
		})
	}
}
