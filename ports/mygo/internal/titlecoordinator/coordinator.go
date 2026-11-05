// SPDX-License-Identifier: AGPL-3.0-only
// Package titlecoordinator coordinates title drafts without owning a MyGo binding.
// Methods and hooks except Service, Today, Permanent and Lane run serially on
// the UI thread BETWEEN build frames, without nested event loops. Never copy it.
package titlecoordinator

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"sync/atomic"
	"time"
)

var (
	ErrRegistry = errors.New("title coordinator: registration changed during finalization; retry transition")
	ErrNative   = errors.New("title coordinator: native snapshot/finalization unavailable")
	ErrLocked   = errors.New("title coordinator: trace no longer editable; draft retained")
	ErrStopped  = errors.New("title coordinator: service requires inspection or reopen")
	ErrOwner    = errors.New("title coordinator: owner registered with another trace")
	ErrStale    = errors.New("title coordinator: stale registration or identity")
)

type Date struct{ Year, Month, Day int }

// Save returns exact bytes acknowledged in the SAME transaction, not Snapshot.
type Service interface {
	AdvanceTo(Date) error
	SaveTraceTitleInputAcknowledged(trace, title string) (string, error)
}

// Native references stable Handle/binding identity, never retained Element.
// Replace updates that binding without yielding/reentering or returns false.
// Called only immediately after a fresh read on the serial UI safe stage.
type Native interface {
	Snapshot() (titlesession.NativeSnapshot, bool)
	Finish() (titlesession.NativeSnapshot, bool)
	Replace(string) bool
}

// Enqueue never waits for IO. All writers sharing Service must use ONE lane.
type Lane interface{ Enqueue(func()) }

// Cancellation functions must not reenter coordinator APIs or invoke user hooks.
type Clock interface {
	Now() time.Time
	After(time.Duration, func()) func()
}
type Config struct {
	// TraceLocked maps core.ErrImmutableHistory/ErrInvalidTransition/ErrNotFound.
	TraceLocked  func(error) bool
	Service      Service
	Lane         Lane
	Clock        Clock
	Post         func(func())     // always queue onto UI safe stage, never inline
	Today        func() Date      // worker-safe; sample at execution, not debounce creation
	Permanent    func(error) bool // errors.Is ErrConflict/ErrIncomplete/ErrSession
	Changed      func(Registration, titlesession.State)
	Failed       func(Registration, error)
	Navigate     func(window, target string)
	ReenterClose func(window string) // actual Window.Close, never Destroy
	ReenterQuit  func()              // actual App.Quit; MUST invoke BeforeQuit again
}
type Registration struct {
	coordinator *Coordinator
	number      uint64
}

// Nonzero size gives simultaneously live publication calls distinct identities.
type publicationToken struct{ marker byte }

type entry struct {
	publication   *publicationToken
	flight        *titlesession.SaveRequest
	reg           Registration
	window, owner string
	session       *titlesession.Session
	native        Native
	locked        error
	job           *titlesession.Job
	deadline      time.Time
	cancel        func()
	timer         uint64
}
type intent struct {
	kind, window, target string
	notified             bool
	recheckQueued        bool
	rechecks             uint8
}
type Coordinator struct {
	epoch   uint64             // changes on every registration addition/removal, never a count
	intents map[string]*intent // independent window navigation/close intents
	cfg     Config
	entries []*entry
	next    uint64
	pending *intent
	stopped atomic.Bool
}

func New(cfg Config) (*Coordinator, error) {
	if cfg.Service == nil || cfg.Lane == nil || cfg.Clock == nil || cfg.Post == nil || cfg.Today == nil || cfg.Permanent == nil || cfg.TraceLocked == nil || cfg.Navigate == nil || cfg.ReenterClose == nil || cfg.ReenterQuit == nil {
		return nil, errors.New("title coordinator: missing dependency")
	}
	return &Coordinator{cfg: cfg, intents: make(map[string]*intent)}, nil
}

// Same-window owner reopen reuses session AND native source, preserving draft.
// A different trace cannot silently replace an outstanding owner.
func (c *Coordinator) Open(window, owner, trace, persisted string, native Native) (Registration, error) {
	if window == "" || native == nil {
		return Registration{}, errors.New("title coordinator: missing window/native")
	}
	for _, e := range c.entries {
		if e.window == window && e.owner == owner {
			if e.session.State().TraceID != trace {
				return e.reg, ErrOwner
			}
			return e.reg, nil
		}
	}
	c.next++
	s, err := titlesession.New(titlesession.Config{OwnerID: owner, SessionID: c.next, Generation: 1, TraceID: trace, PersistedText: persisted})
	if err != nil {
		return Registration{}, err
	}
	e := &entry{reg: Registration{c, c.next}, window: window, owner: owner, session: s, native: native}
	c.entries = append(c.entries, e)
	c.epoch++
	return e.reg, nil
}
func (c *Coordinator) find(r Registration) *entry {
	if r.coordinator != c {
		return nil
	}
	for _, e := range c.entries {
		if e.reg == r {
			return e
		}
	}
	return nil
}
func (c *Coordinator) State(r Registration) (titlesession.State, bool) {
	e := c.find(r)
	if e == nil {
		return titlesession.State{}, false
	}
	return e.session.State(), true
}
func (c *Coordinator) Identity(r Registration) (titlesession.Identity, bool) {
	e := c.find(r)
	if e == nil {
		return titlesession.Identity{}, false
	}
	return e.session.Identity(), true
}
func (c *Coordinator) notify(e *entry) {
	if c.cfg.Changed != nil {
		c.cfg.Changed(e.reg, e.session.State())
	}
}
func (c *Coordinator) fail(e *entry, err error) {
	c.pending = nil
	delete(c.intents, e.window)
	if c.cfg.Failed != nil {
		c.cfg.Failed(e.reg, err)
	}
}
func (c *Coordinator) cancelTimer(e *entry) {
	e.timer++
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.job = nil
}
func (c *Coordinator) usable(e *entry) bool { return e.locked == nil && !c.stopped.Load() }
func (c *Coordinator) accept(e *entry, u titlesession.Update) {
	if !u.Accepted {
		if u.Err != nil {
			c.fail(e, u.Err)
		}
		return
	}
	if u.DraftReplacement.Present && !e.native.Replace(u.DraftReplacement.Text) {
		e.locked = ErrNative
		c.cancelTimer(e)
		c.fail(e, ErrNative)
		c.notify(e)
		return
	}
	if u.Err != nil {
		c.fail(e, u.Err)
	}
	c.syncJob(e, u.Job)
	c.notify(e)
}
func (c *Coordinator) syncJob(e *entry, j *titlesession.Job) {
	if !c.usable(e) || j == nil {
		c.cancelTimer(e)
		return
	}
	if e.job != nil && *e.job == *j {
		return
	} // identical job preserves original deadline
	c.cancelTimer(e)
	v := *j
	e.job = &v
	e.deadline = c.cfg.Clock.Now().Add(time.Duration(j.DelayMilliseconds()) * time.Millisecond)
	if j.DelayMilliseconds() == 0 {
		return
	} // pump now, no After(0)/frame wait
	token := e.timer
	e.cancel = c.cfg.Clock.After(time.Duration(j.DelayMilliseconds())*time.Millisecond, func() {
		c.cfg.Post(func() {
			if c.find(e.reg) != e || e.timer != token {
				return
			}
			if !c.read(e, false) {
				return
			}
			c.pump(e)
			c.continueIntent()
		})
	})
}
func (c *Coordinator) read(e *entry, finish bool) bool {
	var s titlesession.NativeSnapshot
	var ok bool
	if finish {
		s, ok = e.native.Finish()
	} else {
		s, ok = e.native.Snapshot()
	}
	if !ok || finish && s.Composing {
		c.cancelTimer(e)
		c.fail(e, ErrNative)
		return false
	}
	var u titlesession.Update
	if finish {
		u = e.session.RequestLifecycleFlush(e.session.Identity(), s, true)
	} else {
		u = e.session.NativeSnapshotDidChange(e.session.Identity(), s)
	}
	c.accept(e, u)
	return u.Accepted && u.Err == nil && e.locked != ErrNative
}

// Identity must be captured at event source, never relabel old events current.
func (c *Coordinator) Observe(r Registration, id titlesession.Identity) error {
	e := c.find(r)
	if e == nil || e.session.Identity() != id {
		return ErrStale
	}
	if !c.read(e, false) {
		return ErrNative
	}
	c.pump(e)
	c.continueIntent()
	return nil
}

// EndEditing preserves the upstream ordinary blur RequestFlush semantics.
// It still captures real finalized native text first. The known blank-after-
// inflight race may remain dirty with no job until a lifecycle transition.
func (c *Coordinator) EndEditing(r Registration) error {
	e := c.find(r)
	if e == nil {
		return ErrStale
	}
	snapshot, ok := e.native.Finish()
	if !ok || snapshot.Composing {
		c.fail(e, ErrNative)
		return ErrNative
	}
	c.accept(e, e.session.NativeSnapshotDidChange(e.session.Identity(), snapshot))
	c.accept(e, e.session.RequestFlush(e.session.Identity()))
	c.pump(e)
	return nil
}
func (c *Coordinator) pump(e *entry) {
	if c.find(e.reg) != e || !c.usable(e) || e.job == nil || c.cfg.Clock.Now().Before(e.deadline) {
		return
	}
	b := e.session.BeginPersistence(*e.job)
	c.accept(e, b.Update)
	if b.Request == nil {
		return
	}
	request := *b.Request // immutable captured trace/text/token; no UI access in worker
	e.flight = &request
	c.cfg.Lane.Enqueue(func() {
		result := titlesession.PersistenceResult{}
		if c.stopped.Load() {
			result.Err = ErrStopped
		} else {
			result.Err = c.cfg.Service.AdvanceTo(c.cfg.Today())
			if result.Err == nil {
				actual, err := c.cfg.Service.SaveTraceTitleInputAcknowledged(request.TraceID(), request.NormalizedText())
				result.Err = err
				if err == nil {
					result.VerifiedText = titlesession.OptionalText{Text: actual, Present: true}
				}
			}
			if result.Err != nil && c.cfg.Permanent(result.Err) {
				c.stopped.Store(true)
			}
		}
		c.cfg.Post(func() { c.complete(e, request, result) })
	})
}
func (c *Coordinator) complete(e *entry, r titlesession.SaveRequest, result titlesession.PersistenceResult) {
	if c.find(e.reg) != e || e.flight == nil || *e.flight != r {
		return
	}
	// Read latest native text BEFORE success may canonicalize the old raw draft.
	nativeOK := c.read(e, false)
	if !nativeOK {
		e.locked = ErrNative
	}
	u := e.session.PersistenceDidComplete(r, result)
	if u.Accepted && !u.State.InFlight {
		e.flight = nil
	}
	if result.Err != nil && c.cfg.TraceLocked(result.Err) {
		e.locked = result.Err
	}
	if !nativeOK {
		u.DraftReplacement = titlesession.OptionalText{}
	}
	c.accept(e, u)
	if !nativeOK || u.Err != nil {
		return
	}
	if c.hasIntents() {
		c.continueIntent()
		if c.find(e.reg) != e {
			return
		}
	}
	// blank-after-inflight must re-finalize, never rely on previous clean boolean.
	status := e.session.State().Lifecycle.Status
	if status != titlesession.LifecycleNotRequested && status != titlesession.LifecycleFailed {
		if !c.read(e, true) {
			return
		}
	}
	c.pump(e)
}
func (c *Coordinator) affected(i *intent) []*entry {
	var out []*entry
	for _, e := range c.entries {
		if i.kind == "quit" || e.window == i.window {
			out = append(out, e)
		}
	}
	return out
}

// A proof records actual registration identity changes, including remove+add
// with equal count. Hooks can synchronously call public APIs without a loop.
type proof struct {
	entries []*entry
	epoch   uint64
}

func (c *Coordinator) validProof(i *intent, p proof) bool {
	return c.epoch == p.epoch && c.currentIntent(i)
}

// Schedule at most one asynchronous retry for an intent. A hook which changes
// registration every pass is a blocker, never a synchronous/infinite retry loop.
func (c *Coordinator) registryChanged(i *intent) {
	if !c.currentIntent(i) || i.recheckQueued {
		return
	}
	if i.rechecks >= 1 {
		c.removeIntent(i)
		if c.cfg.Failed != nil {
			var r Registration
			if es := c.affected(i); len(es) > 0 {
				r = es[0].reg
			}
			c.cfg.Failed(r, ErrRegistry)
		}
		return
	}
	i.rechecks++
	i.recheckQueued = true
	c.cfg.Post(func() {
		i.recheckQueued = false
		if c.currentIntent(i) {
			c.continueOne(i)
		}
	})
}

// Finalize the entire captured set before starting persistence. Any hook that
// changes registration or replaces the intent invalidates this attempt.
func (c *Coordinator) finalizeAll(i *intent) (proof, bool) {
	p := proof{entries: c.affected(i), epoch: c.epoch}
	ok := true
	for _, e := range p.entries {
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return p, false
		}
		if !c.read(e, true) {
			ok = false
		}
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return p, false
		}
		if e.locked != nil && !c.cleanEntry(e) {
			c.fail(e, e.locked)
			ok = false
		}
		if c.stopped.Load() {
			c.fail(e, ErrStopped)
			ok = false
		}
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return p, false
		}
	}
	if !c.validProof(i, p) {
		c.registryChanged(i)
		return p, false
	}
	return p, ok
}
func (c *Coordinator) cleanEntry(e *entry) bool {
	// A trace lock prohibits writes, not disposal of a freshly finalized and
	// fully acknowledged snapshot. Native uncertainty remains an absolute block.
	return c.find(e.reg) == e && e.locked != ErrNative && e.session.State().Lifecycle.SnapshotIsFlushed()
}
func (c *Coordinator) clean(entries []*entry) bool {
	for _, e := range entries {
		if !c.cleanEntry(e) {
			return false
		}
	}
	return !c.stopped.Load()
}
func (c *Coordinator) pumpProof(i *intent, p proof) bool {
	for _, e := range p.entries {
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return false
		}
		c.pump(e)
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return false
		}
	}
	return true
}

// Block navigation with old editor still visible; latest explicit target wins.
func (c *Coordinator) RequestNavigation(window, target string) {
	// App quit owns all affected editors until accepted/cancelled. A navigation
	// request explicitly replaces quit, while other windows retain their intents.
	c.pending = nil
	i := &intent{kind: "navigate", window: window, target: target}
	c.intents[window] = i
	c.continueOne(i)
}

// Invoke inside EVERY actual close callback. False => PreventDefault now.
func (c *Coordinator) BeforeClose(window string) bool {
	if c.pending != nil {
		c.continueIntent()
		return false
	} // quit takes precedence
	return c.before(&intent{kind: "close", window: window})
}

// App-scoped callback must precede any window destruction. Never cache true.
func (c *Coordinator) BeforeQuit() bool { return c.before(&intent{kind: "quit"}) }
func (c *Coordinator) before(i *intent) bool {
	if i.kind == "quit" {
		c.pending = i
		clear(c.intents)
	} else {
		c.intents[i.window] = i
	}
	p, ok := c.finalizeAll(i)
	if !ok {
		return false
	}
	if !c.pumpProof(i, p) {
		return false
	}
	if c.validProof(i, p) && c.clean(p.entries) {
		c.removeIntent(i)
		return true
	}
	return false
}
func (c *Coordinator) hasIntents() bool { return c.pending != nil || len(c.intents) > 0 }
func (c *Coordinator) currentIntent(i *intent) bool {
	if i.kind == "quit" {
		return c.pending == i
	}
	return c.intents[i.window] == i
}
func (c *Coordinator) removeIntent(i *intent) {
	if i.kind == "quit" {
		if c.pending == i {
			c.pending = nil
		}
	} else if c.intents[i.window] == i {
		delete(c.intents, i.window)
	}
}
func (c *Coordinator) continueIntent() {
	if c.pending != nil {
		c.continueOne(c.pending)
		return
	}
	// registration order gives deterministic cross-window continuation.
	seen := map[string]bool{}
	var intents []*intent
	for _, e := range c.entries {
		if !seen[e.window] {
			seen[e.window] = true
			if i := c.intents[e.window]; i != nil {
				intents = append(intents, i)
			}
		}
	}
	for _, i := range intents {
		c.continueOne(i)
	}
}
func (c *Coordinator) continueOne(i *intent) {
	if !c.currentIntent(i) {
		return
	}
	p, ok := c.finalizeAll(i)
	if !ok {
		return
	}
	if !c.pumpProof(i, p) {
		return
	}
	if !c.validProof(i, p) || !c.clean(p.entries) {
		return
	}
	if i.kind == "navigate" {
		if !c.disposeProof(i, p) {
			return
		}
		c.removeIntent(i)
		c.cfg.Navigate(i.window, i.target)
		return
	}

	if i.notified {
		return
	}
	i.notified = true
	if i.kind == "quit" {
		c.cfg.ReenterQuit()
	} else {
		c.cfg.ReenterClose(i.window)
	}
}

// Successful disposal invokes no user hooks. Track the expected epoch after
// each own removal; any unexpected mutation invalidates remaining disposal.
func (c *Coordinator) disposeProof(i *intent, p proof) bool {
	for _, e := range p.entries {
		if !c.validProof(i, p) || !c.cleanEntry(e) {
			c.registryChanged(i)
			return false
		}
		if !c.dispose(e) {
			return false
		}
		p.epoch++
		if !c.validProof(i, p) {
			c.registryChanged(i)
			return false
		}
	}
	return c.validProof(i, p)
}
func (c *Coordinator) dispose(e *entry) bool {
	u := e.session.Dispose(e.session.Identity())
	if !u.Accepted {
		c.fail(e, u.Err)
		return false
	}
	c.cancelTimer(e)
	for n, x := range c.entries {
		if x == e {
			c.entries = append(c.entries[:n], c.entries[n+1:]...)
			c.epoch++
			break
		}
	}
	return true
}

// Call BEFORE native disposal, in the same serial close callback; freshly
// finalizes again. Never call after hidden/removed or use an old clean flag.
func (c *Coordinator) ReleaseClosed(window string) error {
	if c.pending != nil {
		c.continueIntent()
		return ErrRegistry
	}
	// A new final proof is also an explicit continuing close intent, so newly
	// discovered zero-delay work cannot be left dormant after refusal.
	i := &intent{kind: "close", window: window}
	c.intents[window] = i
	p, ok := c.finalizeAll(i)
	if !ok {
		if c.epoch != p.epoch {
			return ErrRegistry
		}
		return ErrNative
	}
	if !c.pumpProof(i, p) {
		return ErrRegistry
	}
	if !c.validProof(i, p) || !c.clean(p.entries) {
		return ErrNative
	}
	// No hooks between proof and these removals. Clock cancellation is explicitly
	// non-reentrant; Dispose itself has no callback on this checked clean path.
	if !c.disposeProof(i, p) {
		return ErrRegistry
	}
	c.removeIntent(i)
	return nil
}

// LockTrace prohibits further writes to a historical/completed trace, while a
// freshly finalized, fully acknowledged clean editor may still close or become
// readonly through RequestNavigation. Dirty/flight/error drafts stay mounted.
func (c *Coordinator) LockTrace(r Registration) error {
	e := c.find(r)
	if e == nil {
		return ErrStale
	}
	if !c.read(e, true) {
		if c.find(r) == e {
			e.locked = ErrNative
			c.cancelTimer(e)
		}
		return ErrNative
	}
	if c.find(r) != e {
		return ErrStale
	}
	e.locked = ErrLocked
	c.cancelTimer(e)
	if !c.cleanEntry(e) {
		c.fail(e, ErrLocked)
	}
	c.notify(e)
	return nil
}

// SynchronizePersistedText accepts only a verified, ordered publication from
// the application's serial service lane, never a later unversioned readback.
// Identity is captured at publication source, not relabeled on delivery. This
// adapter does not fabricate ordering evidence from title bytes themselves.
func (c *Coordinator) SynchronizePersistedText(r Registration, id titlesession.Identity, text string) error {
	e := c.find(r)
	if e == nil || e.session.Identity() != id {
		return ErrStale
	}
	// Calls arrive in verified writer-lane order, but Changed may synchronously
	// deliver a newer publication while this call reads native state. A per-entry
	// call token makes the older continuation a successful no-op, never a rollback.
	token := &publicationToken{marker: 1}
	e.publication = token
	// Hidden native input/composition must enter the session BEFORE any permitted
	// replacement or acknowledgment of an external publication.
	if !c.read(e, false) {
		return ErrNative
	}
	if c.find(r) != e || e.session.Identity() != id {
		return ErrStale
	}
	if e.publication != token {
		return nil
	}
	u := e.session.SynchronizePersistedText(id, text)
	c.accept(e, u)
	if u.Err != nil {
		return u.Err
	}
	if e.locked == ErrNative {
		return ErrNative
	}
	if e.publication != token {
		return nil
	}
	c.pump(e)
	c.continueIntent()
	return nil
}
