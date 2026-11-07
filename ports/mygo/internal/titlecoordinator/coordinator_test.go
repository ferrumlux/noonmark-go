// SPDX-License-Identifier: AGPL-3.0-only
package titlecoordinator

import (
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
	"reflect"
	"testing"
	"time"
)

type timer struct {
	at        time.Time
	f         func()
	cancelled bool
}
type clock struct {
	now    time.Time
	timers []*timer
}

func (k *clock) Now() time.Time { return k.now }
func (k *clock) After(d time.Duration, f func()) func() {
	x := &timer{at: k.now.Add(d), f: f}
	k.timers = append(k.timers, x)
	return func() { x.cancelled = true }
}
func (k *clock) advance(d time.Duration) {
	k.now = k.now.Add(d)
	ts := k.timers
	k.timers = nil
	for _, x := range ts {
		if !x.at.After(k.now) {
			if !x.cancelled {
				x.f()
			}
		} else {
			k.timers = append(k.timers, x)
		}
	}
}

type lane struct{ q []func() }

func (l *lane) Enqueue(f func()) { l.q = append(l.q, f) }
func (l *lane) one()             { f := l.q[0]; l.q = l.q[1:]; f() }

type native struct {
	s                                titlesession.NativeSnapshot
	marked                           string
	invalid, failFinish, failReplace bool
	finishes, replaces               int
}

func (n *native) Snapshot() (titlesession.NativeSnapshot, bool) { return n.s, !n.invalid }
func (n *native) Finish() (titlesession.NativeSnapshot, bool) {
	n.finishes++
	if n.invalid || n.failFinish {
		return n.s, false
	}
	if n.s.Composing {
		n.s.Text += n.marked
		n.marked = ""
		n.s.Composing = false
	}
	return n.s, true
}
func (n *native) Replace(s string) bool {
	n.replaces++
	if n.failReplace {
		return false
	}
	n.s.Text = s
	return true
}

type service struct {
	calls               []string
	day                 Date
	advanceErr, saveErr error
	ack                 string
	useAck              bool
}

func (s *service) AdvanceTo(d Date) error {
	s.calls = append(s.calls, fmt.Sprintf("day:%d", d.Day))
	if s.advanceErr == nil {
		s.day = d
	}
	return s.advanceErr
}
func (s *service) SaveTraceTitleInputAcknowledged(trace, title string) (string, error) {
	s.calls = append(s.calls, trace+":"+title)
	if s.useAck {
		return s.ack, s.saveErr
	}
	return title, s.saveErr
}

var permanent = errors.New("synthetic permanent")

type fixture struct {
	c                                              *Coordinator
	k                                              *clock
	l                                              *lane
	s                                              *service
	post                                           []func()
	day                                            Date
	nav                                            []string
	close, quit                                    int
	errs                                           []error
	rejectSubmit, rejectProducer, rejectCompletion bool
	completionRejected                             int
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{k: &clock{now: time.Unix(0, 0)}, l: &lane{}, s: &service{}, day: Date{2026, 11, 5}}
	var err error
	f.c, err = New(Config{TraceLocked: func(e error) bool { return errors.Is(e, ErrLocked) }, Service: f.s, TrySubmit: f.trySubmit, Clock: f.k, TryPostProducer: f.tryPostProducer, Today: func() Date { return f.day }, Permanent: func(e error) bool { return errors.Is(e, permanent) }, Failed: func(_ Registration, e error) { f.errs = append(f.errs, e) }, Navigate: func(w, target string) { f.nav = append(f.nav, w+":"+target) }, ReenterClose: func(string) { f.close++ }, ReenterQuit: func() { f.quit++ }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) trySubmit(work func(func(func()) bool)) bool {
	if f.rejectSubmit {
		return false
	}
	f.l.Enqueue(func() {
		work(func(fn func()) bool {
			if f.rejectCompletion {
				f.completionRejected++
				return false
			}
			f.post = append(f.post, fn)
			return true
		})
	})
	return true
}
func (f *fixture) tryPostProducer(fn func()) bool {
	if f.rejectProducer {
		return false
	}
	f.post = append(f.post, fn)
	return true
}
func (f *fixture) ui() {
	for len(f.post) > 0 {
		fn := f.post[0]
		f.post = f.post[1:]
		fn()
	}
}
func (f *fixture) advance(d time.Duration) { f.k.advance(d); f.ui() }
func (f *fixture) open(t *testing.T, w string) (Registration, *native) {
	t.Helper()
	n := &native{s: titlesession.NativeSnapshot{Text: "旧标题"}}
	r, e := f.c.Open(w, "task:"+w+":title", "trace-"+w, "旧标题", n)
	if e != nil {
		t.Fatal(e)
	}
	return r, n
}
func (f *fixture) observe(t *testing.T, r Registration) {
	t.Helper()
	id, _ := f.c.Identity(r)
	if e := f.c.Observe(r, id); e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) finishSave(t *testing.T) {
	t.Helper()
	if len(f.l.q) == 0 {
		t.Fatal("no queued save")
	}
	f.l.one()
	f.ui()
}
func TestDebounceIdenticalDoesNotReset(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "新标题"
	f.observe(t, r)
	f.advance(400 * time.Millisecond)
	f.observe(t, r)
	f.advance(299 * time.Millisecond)
	if len(f.l.q) != 0 {
		t.Fatal("early save")
	}
	f.advance(time.Millisecond)
	if len(f.l.q) != 1 {
		t.Fatal("deadline reset")
	}
	f.finishSave(t)
	if got := f.s.calls; !reflect.DeepEqual(got, []string{"day:5", "trace-w:新标题"}) {
		t.Fatal(got)
	}
}
func TestDueWorkReadsNativeBeforeBuild(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	n.s.Text = "帧前B"
	f.advance(700 * time.Millisecond)
	if len(f.l.q) != 0 {
		t.Fatal("stale title saved")
	}
	f.advance(700 * time.Millisecond)
	f.finishSave(t)
	if f.s.calls[1] != "trace-w:帧前B" {
		t.Fatal(f.s.calls)
	}
}
func TestCompositionBlocksAutosaveThenFinishesWithoutFrame(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	n.s.Composing = true
	n.marked = "未定"
	f.advance(time.Second)
	if len(f.l.q) != 0 {
		t.Fatal("composition saved")
	}
	if f.c.BeforeClose("w") {
		t.Fatal("closed before save")
	}
	if len(f.l.q) != 1 {
		t.Fatal("0ms did not start")
	}
	f.finishSave(t)
	if f.close != 1 || !f.c.BeforeClose("w") {
		t.Fatal("close not ready")
	}
	if f.s.calls[1] != "trace-w:A未定" {
		t.Fatal(f.s.calls)
	}
}
func TestNewInputDuringInflightNeverOverwritten(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = " A "
	f.observe(t, r)
	if f.c.BeforeClose("w") {
		t.Fatal("early")
	}
	n.s.Text = " B "
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.Draft != " B " || n.s.Text != " B " || s.LastAcknowledgedText != "A" {
		t.Fatalf("%+v native=%q", s, n.s.Text)
	}
	if f.close != 0 || len(f.l.q) != 1 {
		t.Fatal("new draft not flushed")
	}
	f.finishSave(t)
	if !f.c.BeforeClose("w") {
		t.Fatal("not clean")
	}
}
func TestBlankAfterInflightLifecycleRestoresActualAck(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "保存A"
	f.observe(t, r)
	f.advance(700 * time.Millisecond)
	n.s.Text = "  \n"
	if f.c.BeforeClose("w") {
		t.Fatal("closed while inflight")
	}
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.Draft != "保存A" || n.s.Text != "保存A" || s.Dirty || s.InFlight {
		t.Fatalf("%+v native=%q", s, n.s.Text)
	}
	if len(f.s.calls) != 2 {
		t.Fatal("blank persisted")
	}
}
func TestEveryReentrantCloseReadsFreshNative(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	if f.c.BeforeClose("w") {
		t.Fatal("early")
	}
	f.finishSave(t)
	if f.close != 1 {
		t.Fatal("no reentry")
	}
	n.s.Text = "late B"
	before := n.finishes
	if f.c.BeforeClose("w") {
		t.Fatal("cached clean permitted close")
	}
	if n.finishes <= before {
		t.Fatal("not finalized")
	}
	f.finishSave(t)
	if !f.c.BeforeClose("w") {
		t.Fatal("not clean")
	}
	if err := f.c.ReleaseClosed("w"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.c.State(r); ok {
		t.Fatal("not disposed")
	}
}
func TestFailureCancelsClosePreservesDraft(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "保留"
	f.observe(t, r)
	f.c.BeforeClose("w")
	f.s.saveErr = errors.New("disk fault")
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.Draft != "保留" || s.LastAcknowledgedText != "旧标题" || f.c.hasIntents() || f.close != 0 {
		t.Fatal(s)
	}
	f.s.saveErr = nil
	f.advance(time.Second)
	f.finishSave(t)
	if f.close != 0 {
		t.Fatal("retry resurrected cancelled close")
	}
	if !f.c.BeforeClose("w") {
		t.Fatal("fresh close not accepted")
	}
}
func TestPermanentStopsAlreadyQueuedOtherWindowAndRetries(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	q, m := f.open(t, "two")
	n.s.Text = "A"
	m.s.Text = "B"
	f.observe(t, r)
	f.observe(t, q)
	f.c.BeforeQuit()
	if len(f.l.q) != 2 {
		t.Fatal("not queued")
	}
	f.s.saveErr = fmt.Errorf("wrapped: %w", permanent)
	f.finishSave(t)
	f.finishSave(t)
	f.advance(time.Hour)
	if len(f.s.calls) != 2 || len(f.l.q) != 0 || f.quit != 0 {
		t.Fatal(f.s.calls)
	}
	if f.c.BeforeQuit() {
		t.Fatal("blocked service allowed quit")
	}
}
func TestOnlyAcknowledgedBytes(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "é"
	f.observe(t, r)
	f.s.useAck = true
	f.s.ack = "e\u0301"
	f.advance(700 * time.Millisecond)
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.LastAcknowledgedText != "e\u0301" {
		t.Fatal("normalized request substituted for ack")
	}
}
func TestWrongAckCannotPermitClose(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.s.useAck = true
	f.s.ack = "wrong"
	f.c.BeforeClose("w")
	f.finishSave(t)
	s, _ := f.c.State(r)
	if !s.InFlight || !s.AwaitingVerification || f.close != 0 || f.c.hasIntents() {
		t.Fatal(s)
	}
}
func TestLatestNavigationAndSameOwnerReopen(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.c.RequestNavigation("w", "first")
	again, err := f.c.Open("w", "task:w:title", "trace-w", "ignore", &native{})
	if err != nil || again != r {
		t.Fatal("lost owner")
	}
	f.c.RequestNavigation("w", "second")
	if len(f.nav) != 0 {
		t.Fatal("navigated while pending")
	}
	f.finishSave(t)
	if !reflect.DeepEqual(f.nav, []string{"w:second"}) {
		t.Fatal(f.nav)
	}
	if _, ok := f.c.State(r); ok {
		t.Fatal("old session registered")
	}
}
func TestNavigationFailureRetainsEditor(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.c.RequestNavigation("w", "away")
	f.s.saveErr = errors.New("fault")
	f.finishSave(t)
	if len(f.nav) != 0 {
		t.Fatal("left failing editor")
	}
	if s, ok := f.c.State(r); !ok || s.Draft != "A" {
		t.Fatal(s)
	}
}
func TestStaleTimersIdentitiesAndRegistration(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	oldID, _ := f.c.Identity(r)
	n.s.Text = "A"
	f.observe(t, r)
	oldTimer := f.k.timers[0].f
	n.s.Text = " "
	f.observe(t, r)
	f.c.EndEditing(r)
	if err := f.c.Observe(r, oldID); !errors.Is(err, ErrStale) {
		t.Fatal("old generation accepted", err)
	}
	oldTimer()
	f.ui()
	if len(f.l.q) != 0 {
		t.Fatal("old timer saved")
	}
	f.c.RequestNavigation("w", "away")
	r2, _ := f.open(t, "w")
	if r == r2 {
		t.Fatal("registration reused")
	}
	oldTimer()
	f.ui()
	if err := f.c.Observe(r, oldID); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}
func TestStaleCompletionCannotMutateNewSession(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.c.BeforeClose("w")
	e := f.c.find(r) // capture actual immutable inflight request using scheduled worker callback
	f.l.one()
	callback := f.post[0]
	f.ui()
	if err := f.c.ReleaseClosed("w"); err != nil {
		t.Fatal(err)
	}
	r2, m := f.open(t, "w")
	m.s.Text = "B"
	f.observe(t, r2)
	callback()
	s, _ := f.c.State(r2)
	if s.Draft != "B" || s.LastAcknowledgedText != "旧标题" || f.c.find(r) == e {
		t.Fatal(s)
	}
}
func TestQuitSnapshotsEveryWindowAndFailureKeepsAll(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	q, m := f.open(t, "two")
	n.s.Text = "A"
	m.s.Text = "B"
	f.observe(t, r)
	f.observe(t, q)
	m.failFinish = true
	if f.c.BeforeQuit() {
		t.Fatal("quit on invalid native")
	}
	if len(f.l.q) != 0 || n.finishes == 0 || m.finishes == 0 {
		t.Fatal("started writes before all finalized")
	}
	m.failFinish = false
	f.c.BeforeQuit()
	f.finishSave(t)
	f.s.saveErr = errors.New("last window fault")
	f.finishSave(t)
	if f.quit != 0 {
		t.Fatal("quit despite final failure")
	}
	if _, ok := f.c.State(r); !ok {
		t.Fatal("first window disposed")
	}
	if _, ok := f.c.State(q); !ok {
		t.Fatal("last window disposed")
	}
}
func TestQuitReentryIncludesNewWindowAndLateInput(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	n.s.Text = "A"
	f.observe(t, r)
	f.c.BeforeQuit()
	f.finishSave(t)
	if f.quit != 1 {
		t.Fatal("no quit reentry")
	}
	_, m := f.open(t, "two")
	m.s.Text = "native before frame"
	if f.c.BeforeQuit() {
		t.Fatal("new window skipped")
	}
	f.finishSave(t)
	if !f.c.BeforeQuit() {
		t.Fatal("not ready")
	}
}
func TestWorkerSamplesDayAtExecutionAndNeverReroutes(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "旧日草稿"
	f.observe(t, r)
	f.advance(700 * time.Millisecond)
	f.day.Day = 6
	f.s.saveErr = ErrLocked
	f.finishSave(t)
	if !reflect.DeepEqual(f.s.calls, []string{"day:6", "trace-w:旧日草稿"}) {
		t.Fatal(f.s.calls)
	}
	if f.s.day.Day != 6 {
		t.Fatal("settlement did not occur")
	}
	f.c.LockTrace(r)
	f.advance(time.Hour)
	s, _ := f.c.State(r)
	if s.Draft != "旧日草稿" || len(f.l.q) != 0 {
		t.Fatal(s)
	}
	if f.c.BeforeClose("w") {
		t.Fatal("locked dirty draft lost")
	}
}
func TestAdvanceFailureSkipsTitleWrite(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.s.advanceErr = errors.New("settlement fault")
	f.c.BeforeClose("w")
	f.finishSave(t)
	if len(f.s.calls) != 1 || f.close != 0 {
		t.Fatal(f.s.calls)
	}
	s, _ := f.c.State(r)
	if s.Draft != "A" {
		t.Fatal(s)
	}
}
func TestInvalidNativeAndReplacementFailureFailClosed(t *testing.T) {
	for _, mode := range []string{"snapshot", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "w")
			n.s.Text = " A "
			f.observe(t, r)
			f.c.BeforeClose("w")
			if mode == "snapshot" {
				n.invalid = true
			} else {
				n.failReplace = true
			}
			f.finishSave(t)
			if f.close != 0 || f.c.hasIntents() {
				t.Fatal("unsafe close")
			}
			if err := f.c.ReleaseClosed("w"); err == nil {
				t.Fatal("unsafe disposal")
			}
		})
	}
}
func TestSerialLaneFIFOAndNonBlockingEnqueue(t *testing.T) {
	l := NewSerialLane()
	started := make(chan struct{})
	release := make(chan struct{})
	var order []int
	l.Enqueue(func() { close(started); <-release; order = append(order, 1) })
	<-started
	l.Enqueue(func() { order = append(order, 2) })
	l.Enqueue(func() { order = append(order, 3) })
	l.StopAfterDrain()
	close(release)
	<-l.Done()
	if !reflect.DeepEqual(order, []int{1, 2, 3}) {
		t.Fatal(order)
	}
}

func TestOrdinaryBlurBlankInflightPreservesUpstreamBoundary(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.advance(700 * time.Millisecond)
	n.s.Text = " "
	if err := f.c.EndEditing(r); err != nil {
		t.Fatal(err)
	}
	f.finishSave(t)
	s, _ := f.c.State(r)
	if s.Draft != " " || !s.Dirty || len(f.l.q) != 0 {
		t.Fatalf("upstream boundary changed: %+v", s)
	}
	if !f.c.BeforeClose("w") {
		t.Fatal("lifecycle did not restore acknowledged A")
	}
	s, _ = f.c.State(r)
	if s.Draft != "A" || n.s.Text != "A" {
		t.Fatal(s)
	}
}

func TestDuplicateCompletionDoesNotCancelNewIntent(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.advance(700 * time.Millisecond)
	f.l.one()
	duplicate := f.post[0]
	f.ui()
	n.s.Text = "B"
	f.c.RequestNavigation("w", "target")
	duplicate()
	if f.c.intents["w"] == nil || f.c.intents["w"].target != "target" {
		t.Fatal("duplicate completion cancelled newer intent")
	}
	f.finishSave(t)
	if !reflect.DeepEqual(f.nav, []string{"w:target"}) {
		t.Fatal(f.nav)
	}
}
func TestTraceRejectionStopsRetriesWithoutProjectionCallback(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = "A"
	f.observe(t, r)
	f.s.saveErr = ErrLocked
	f.advance(700 * time.Millisecond)
	f.finishSave(t)
	f.advance(time.Hour)
	if len(f.l.q) != 0 || len(f.s.calls) != 2 {
		t.Fatal("locked trace retried")
	}
	s, _ := f.c.State(r)
	if s.Draft != "A" {
		t.Fatal(s)
	}
}
func TestCleanReleaseStillRequiresCurrentNativeEvidence(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	if !f.c.BeforeClose("w") {
		t.Fatal("initial clean")
	}
	n.s.Text = "late"
	if f.c.ReleaseClosed("w") == nil {
		t.Fatal("old clean boolean used to dispose")
	}
	if s, ok := f.c.State(r); !ok || s.Draft != "late" {
		t.Fatal(s)
	}
}
func TestConstructorAndOwnerGuards(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
	f := setup(t)
	r, _ := f.open(t, "w")
	if _, err := f.c.Open("w", "task:w:title", "different", "old", &native{}); err != ErrOwner {
		t.Fatal(err)
	}
	if _, err := f.c.Open("", "owner", "trace", "old", &native{}); err == nil {
		t.Fatal("empty window")
	}
	if err := f.c.EndEditing(Registration{}); err != ErrStale {
		t.Fatal(err)
	}
	if err := f.c.LockTrace(Registration{}); err != ErrStale {
		t.Fatal(err)
	}
	if _, ok := f.c.Identity(Registration{}); ok {
		t.Fatal("stale identity")
	}
	other := setup(t)
	if _, ok := other.c.State(r); ok {
		t.Fatal("foreign registration")
	}
}

type barrierService struct {
	started chan struct{}
	release chan struct{}
	calls   int
}

func (s *barrierService) AdvanceTo(Date) error { return nil }
func (s *barrierService) SaveTraceTitleInputAcknowledged(_, title string) (string, error) {
	s.calls++
	if s.calls == 1 {
		close(s.started)
		<-s.release
	}
	return title, nil
}
func TestActualWorkerDoesNotWaitForUIAndCapturesImmutableRequest(t *testing.T) {
	l := NewSerialLane()
	s := &barrierService{started: make(chan struct{}), release: make(chan struct{})}
	post := make(chan func(), 4)
	k := &clock{now: time.Unix(0, 0)}
	postCompletion := func(f func()) bool { post <- f; return true }
	c, err := New(Config{Service: s, TrySubmit: func(work func(func(func()) bool)) bool {
		return l.TryEnqueue(func() { work(postCompletion) })
	}, Clock: k, TryPostProducer: postCompletion, Today: func() Date { return Date{2026, 11, 5} }, Permanent: func(error) bool { return false }, TraceLocked: func(error) bool { return false }, Navigate: func(string, string) {}, ReenterClose: func(string) {}, ReenterQuit: func() {}})
	if err != nil {
		t.Fatal(err)
	}
	n := &native{s: titlesession.NativeSnapshot{Text: "A"}}
	r, err := c.Open("w", "owner", "trace", "old", n)
	if err != nil {
		t.Fatal(err)
	}
	if c.BeforeClose("w") {
		t.Fatal("early")
	}
	<-s.started
	// Worker is deliberately stalled; UI remains able to inspect and accept input.
	n.s.Text = "B"
	id, _ := c.Identity(r)
	if err := c.Observe(r, id); err != nil {
		t.Fatal(err)
	}
	close(s.release)
	(<-post)()
	(<-post)()
	state, _ := c.State(r)
	if state.LastAcknowledgedText != "B" || state.InFlight {
		t.Fatal(state)
	}
	l.StopAfterDrain()
	<-l.Done()
}

func TestCompletionDuringNewCompositionDoesNotReplaceMarkedText(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "w")
	n.s.Text = " A "
	f.observe(t, r)
	f.c.EndEditing(r)
	n.s.Composing = true
	n.marked = "未定"
	f.finishSave(t)
	if !n.s.Composing || n.marked != "未定" || n.replaces != 0 {
		t.Fatalf("composition changed: %+v", n)
	}
	s, _ := f.c.State(r)
	if !s.Composing || s.LastAcknowledgedText != "A" {
		t.Fatal(s)
	}
}
func TestIndependentWindowsCloseIntentSurvivesCleanOtherWindow(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	_, _ = f.open(t, "two")
	n.s.Text = "A"
	f.observe(t, r)
	if f.c.BeforeClose("one") {
		t.Fatal("early")
	}
	if !f.c.BeforeClose("two") {
		t.Fatal("clean second window")
	}
	f.finishSave(t)
	if f.close != 1 {
		t.Fatal("first window close abandoned")
	}
}
func TestQuitHasPrecedenceOverIndividualClose(t *testing.T) {
	f := setup(t)
	r, n := f.open(t, "one")
	_, _ = f.open(t, "two")
	n.s.Text = "A"
	f.observe(t, r)
	if f.c.BeforeQuit() {
		t.Fatal("early")
	}
	if f.c.BeforeClose("two") {
		t.Fatal("individual close bypassed pending quit")
	}
	f.finishSave(t)
	if f.quit != 1 || f.close != 0 {
		t.Fatal("quit intent lost")
	}
}
func TestSynchronousReentrantCloseAndQuitDisposal(t *testing.T) {
	for _, kind := range []string{"close", "quit"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			r, n := f.open(t, "one")
			n.s.Text = "A"
			f.observe(t, r)
			closed := 0
			f.c.cfg.ReenterClose = func(w string) {
				if f.c.BeforeClose(w) {
					if err := f.c.ReleaseClosed(w); err != nil {
						t.Fatal(err)
					}
					closed++
				}
			}
			f.c.cfg.ReenterQuit = func() {
				if f.c.BeforeQuit() {
					if !f.c.BeforeClose("one") {
						t.Fatal("window refused finalized quit")
					}
					if err := f.c.ReleaseClosed("one"); err != nil {
						t.Fatal(err)
					}
					closed++
				}
			}
			if kind == "close" {
				f.c.BeforeClose("one")
			} else {
				f.c.BeforeQuit()
			}
			f.finishSave(t)
			if closed != 1 {
				t.Fatal("not disposed exactly once")
			}
			if _, ok := f.c.State(r); ok {
				t.Fatal("registration survived")
			}
		})
	}
}

func TestUnrelatedWindowIntentCannotStarveZeroDelayFollowup(t *testing.T) {
	f := setup(t)
	a, n := f.open(t, "a")
	b, m := f.open(t, "b")
	n.s.Text = "A"
	f.observe(t, a)
	f.c.EndEditing(a)
	n.s.Text = "A newer"
	f.observe(t, a)
	m.s.Text = "B"
	f.observe(t, b)
	f.c.BeforeClose("b")
	// Queue is A1, B1. A1 completion must enqueue A2 even while B still waiting.
	f.finishSave(t)
	if len(f.l.q) != 2 {
		t.Fatal("zero-delay A2 starved behind unrelated B intent")
	}
	f.finishSave(t)
	f.finishSave(t)
	state, _ := f.c.State(a)
	if state.LastAcknowledgedText != "A newer" || state.Dirty {
		t.Fatal(state)
	}
}
