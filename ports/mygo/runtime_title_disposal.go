// SPDX-License-Identifier: AGPL-3.0-only
package main

import coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"

type runtimeTitleDisposal struct {
	window     *titleWindow
	navigation uint64
	editors    []*runtimeTitleEditor
	owners     []coordinator.RetirementOwner
}

// Automatic retirement has no user navigation target, closure or generation.
func (s *app) disposeTitleEditors() {
	h := s.titles
	if h == nil {
		return
	}
	w := h.windows[s.titleWindowID]
	if w == nil || w.closing || h.quitting || s.titleTarget != "" {
		return
	}
	r := &runtimeTitleDisposal{window: w, navigation: w.navigation}
	for _, e := range s.titleEditors() {
		if !e.registered || !e.locked {
			continue
		}
		state, ok := h.coordinator.State(e.reg)
		if !ok || state.Dirty || state.Composing || state.InFlight || state.AwaitingVerification || state.LastError != nil {
			continue
		}
		r.editors = append(r.editors, e)
		r.owners = append(r.owners, coordinator.RetirementOwner{Registration: e.reg, Identity: e.identity})
	}
	if len(r.editors) == 0 {
		return
	}
	s.titleDisposal = r
	valid := func() bool {
		if s.titleDisposal != r || h.windows[s.titleWindowID] != w || w.navigation != r.navigation || w.closing || h.quitting || s.titleTarget != "" {
			return false
		}
		current := s.titleEditors()
		for i, e := range r.editors {
			found := false
			for _, now := range current {
				if now == e {
					found = true
					break
				}
			}
			if !found || !e.registered || e.reg != r.owners[i].Registration || e.identity != r.owners[i].Identity {
				return false
			}
		}
		return true
	}
	if !h.tryPostProducer(func() {
		defer func() {
			if s.titleDisposal == r {
				s.titleDisposal = nil
			}
		}()
		if !valid() || !h.coordinator.RetirementIdle(s.titleWindowID) {
			return
		}
		if !s.finalizeComposerOwners(false) || !valid() {
			return
		}
		retired, err := h.coordinator.TryRetireEditors(s.titleWindowID, r.owners)
		if err != nil {
			s.runtimeError = titleFailureMessage
			return
		}
		if !retired {
			return
		}
		s.reconcileTitleRetirement(r)
		w.invalidate()
	}) {
		if s.titleDisposal == r {
			s.titleDisposal = nil
		}
	}
}

// Reconcile the completed exact side effect without acting on an old UI intent.
func (s *app) reconcileTitleRetirement(r *runtimeTitleDisposal) {
	for i, e := range r.editors {
		// Retirement already happened. Reconcile only that exact owner,
		// even if a reentrant newer navigation invalidated this request.
		if e.reg != r.owners[i].Registration || e.identity != r.owners[i].Identity {
			continue
		}
		e.registered = false
		if s.titleEditor == e {
			s.titleEditor = nil
		}
		for key, current := range s.subtaskEditors {
			if current == e {
				delete(s.subtaskEditors, key)
			}
		}
	}
}
