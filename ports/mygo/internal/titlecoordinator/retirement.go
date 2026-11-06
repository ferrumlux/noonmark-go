// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import "github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"

// RetirementOwner is an exact already-registered historical editor, not a route
// to whichever editor happens to occupy the window when a callback runs.
type RetirementOwner struct {
	Registration Registration
	Identity     titlesession.Identity
}

// RetirementIdle is an admission check before the application touches ordinary
// composer owners. TryRetireEditors rechecks it after every native boundary.
func (c *Coordinator) RetirementIdle(window string) bool {
	return !c.stopped.Load() && c.pending == nil && c.intents[window] == nil
}

// TryRetireEditors is a UI-thread, no-IO, no-retry boundary. It neither creates
// nor replaces navigation/close/quit intents. Late native drafts remain owned.
func (c *Coordinator) TryRetireEditors(window string, owners []RetirementOwner) (bool, error) {
	epoch := c.epoch
	entries := make([]*entry, len(owners))
	valid := func() bool {
		if c.epoch != epoch || c.pending != nil || c.intents[window] != nil || c.stopped.Load() {
			return false
		}
		for i, o := range owners {
			e := c.find(o.Registration)
			if e == nil || e != entries[i] || e.window != window || e.session.Identity() != o.Identity {
				return false
			}
		}
		return true
	}
	if c.stopped.Load() {
		return false, ErrStopped
	}
	if c.pending != nil || c.intents[window] != nil {
		return false, nil
	}
	seen := map[Registration]bool{}
	for i, o := range owners {
		e := c.find(o.Registration)
		if seen[o.Registration] || e == nil || e.window != window || e.session.Identity() != o.Identity {
			return false, ErrStale
		}
		seen[o.Registration] = true
		st := e.session.State()
		if e.locked == nil || st.Dirty || st.Composing || st.InFlight || st.AwaitingVerification || st.LastError != nil {
			return false, nil
		}
		if e.locked == ErrNative {
			return false, ErrNative
		}
		entries[i] = e
	}
	for _, e := range entries {
		if !valid() {
			return false, ErrRegistry
		}
		snapshot, ok := e.native.Finish()
		if !valid() {
			return false, ErrRegistry
		}
		if !ok || snapshot.Composing {
			return false, ErrNative
		}
		u := e.session.RequestLifecycleFlush(e.session.Identity(), snapshot, true)
		// No accept/fail: those own user intent cancellation and persistence. This
		// fixed historical set may only retain a newly discovered draft or retire.
		c.cancelTimer(e)
		c.notify(e)
		if !valid() {
			return false, ErrRegistry
		}
		if !u.SnapshotIsFlushed() || !c.cleanEntry(e) {
			return false, nil
		}
	}
	if !valid() {
		return false, ErrRegistry
	}
	if !c.clean(entries) {
		return false, nil
	}
	for _, e := range entries {
		// Dispose invokes no hooks on this fresh flushed identity. Never expand the
		// captured set and never invoke registryChanged's asynchronous recheck.
		u := e.session.Dispose(e.session.Identity())
		if !u.Accepted {
			return false, u.Err
		}
		c.cancelTimer(e)
		for i, current := range c.entries {
			if current == e {
				c.entries = append(c.entries[:i], c.entries[i+1:]...)
				c.epoch++
				break
			}
		}
	}
	return true, nil
}
