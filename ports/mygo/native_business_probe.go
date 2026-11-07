// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	coordinator "github.com/ferrumlux/noonmark/ports/mygo/internal/titlecoordinator"
)

const nativeBusinessScenario = "ordinary-category-v1"

type nativeBusinessDateSample struct {
	Phase         string `json:"phase"`
	Date          string `json:"date"`
	Zone          string `json:"zone"`
	OffsetSeconds int    `json:"offset_seconds"`
}
type nativeBusinessOutcome struct {
	Result       taskapp.Result
	WriteExtent  taskapp.WriteExtent
	Session      taskapp.SessionState
	FailureStage taskapp.FailureStage
	Error        string
}

func businessOutcome(o taskapp.Outcome) *nativeBusinessOutcome {
	e := ""
	if o.Err != nil {
		e = o.Err.Error()
	}
	return &nativeBusinessOutcome{o.Result, o.WriteExtent, o.Session, o.FailureStage, e}
}

type nativeBusinessOperation struct {
	Name          string                 `json:"name"`
	Error         string                 `json:"error"`
	Outcome       *nativeBusinessOutcome `json:"outcome"`
	Preparation   *nativeBusinessOutcome `json:"preparation"`
	InteractionID ordinary.UUID          `json:"interaction_id"`
	PlanID        ordinary.UUID          `json:"plan_id"`
	Receipt       *ordinary.Receipt      `json:"receipt"`
}
type nativeBusinessReport struct {
	Version                      int                        `json:"version"`
	Scenario                     string                     `json:"scenario"`
	ActionLevel                  string                     `json:"action_level"`
	Stage                        string                     `json:"stage"`
	Profile                      string                     `json:"profile"`
	ActualToday                  string                     `json:"actual_today"`
	AppPID                       int                        `json:"app_pid"`
	WindowID                     int                        `json:"window_id"`
	WindowBindingConfirmed       bool                       `json:"window_binding_confirmed"`
	UIReadViewAcknowledged       bool                       `json:"ui_read_view_acknowledged"`
	UICategoryCallbackTested     bool                       `json:"ui_category_callback_tested"`
	OSInputTested                bool                       `json:"os_input_tested"`
	DisplayPixelsVerified        bool                       `json:"display_pixels_verified"`
	IMETested                    bool                       `json:"ime_tested"`
	DateSamples                  []nativeBusinessDateSample `json:"date_samples"`
	BeforeStoreRevision          int                        `json:"before_store_revision"`
	StoreRevision                int                        `json:"store_revision"`
	BeforeClassificationRevision uint64                     `json:"before_classification_revision"`
	ClassificationRevision       uint64                     `json:"classification_revision"`
	Operations                   []nativeBusinessOperation  `json:"operations"`
	PreCloseHealth               taskapp.SessionHealth      `json:"pre_close_health"`
	PersistentFactsJSON          string                     `json:"persistent_facts_json"`
	PersistentFactsSHA256        string                     `json:"persistent_facts_sha256"`
	Complete                     bool                       `json:"complete"`
	Errors                       []string                   `json:"errors"`
}

// The gate serializes the only producer's one-shot Enqueue with terminal stop.
// It is never held across disk, dispatch, UI calls, waits, or report I/O. Once
// admitted, the writer owns completion posting; dispatcher stops only after it
// drains. Cancellation waits for no UI callback (RunOnMain can discard one).
type nativeBusinessProbe struct {
	mu                               sync.Mutex
	closed, started, preCloseChecked bool
	cancel                           chan struct{}
	ack                              chan error
	report                           nativeBusinessReport
	now                              func() time.Time
	today                            core.LocalDate
	service                          *taskapp.Service
	lane                             coordinator.Lane
	post                             func(func())
	publish                          func(taskapp.OrdinaryReadView) error // main-thread only
	acknowledged                     taskapp.OrdinaryReadView
}

func newNativeBusinessProbe(stage string, now func() time.Time) *nativeBusinessProbe {
	return &nativeBusinessProbe{cancel: make(chan struct{}), ack: make(chan error, 1), now: now, report: nativeBusinessReport{
		Version: 1, Scenario: nativeBusinessScenario, ActionLevel: "service-command", Stage: stage, Profile: ordinaryDevelopment, AppPID: os.Getpid(),
		DateSamples: []nativeBusinessDateSample{}, Operations: []nativeBusinessOperation{}, Errors: []string{}}}
}
func (b *nativeBusinessProbe) fail(err error) error {
	if err == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.report.Errors) < 32 {
		b.report.Errors = append(b.report.Errors, err.Error())
	}
	return err
}
func (b *nativeBusinessProbe) canceled() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.closed }
func (b *nativeBusinessProbe) StopAdmission() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.cancel)
	}
}
func (b *nativeBusinessProbe) sample(phase string) error {
	n := b.now()
	zone, offset := n.Zone()
	d := localTitleDate(n)
	s := nativeBusinessDateSample{phase, n.Format("2006-01-02"), zone, offset}
	b.mu.Lock()
	defer b.mu.Unlock()
	if phase != "startup" && len(b.report.DateSamples) == 0 {
		return errors.New("business startup date not sampled")
	}
	if phase == "startup" && len(b.report.DateSamples) != 0 {
		return errors.New("business startup date sampled twice")
	}
	if len(b.report.DateSamples) >= 16 {
		return errors.New("date sample overflow")
	}
	b.report.DateSamples = append(b.report.DateSamples, s)
	if phase == "startup" {
		b.today = d
		b.report.ActualToday = s.Date
		return nil
	}
	first := b.report.DateSamples[0]
	if d != b.today || zone != first.Zone || offset != first.OffsetSeconds {
		return errors.New("date-boundary-not-covered")
	}
	return nil
}
func businessHealthy(v taskapp.OrdinaryReadView) bool {
	return v.Health == (taskapp.SessionHealth{Session: taskapp.SessionWritable, Reason: taskapp.HealthOK, FailureStage: taskapp.StageNone})
}
func (b *nativeBusinessProbe) read() (taskapp.OrdinaryReadView, error) {
	v, e := b.service.ReadView()
	if e != nil {
		return v, e
	}
	if v.Today != b.today {
		return v, errors.New("date-boundary-not-covered")
	}
	if !businessHealthy(v) {
		return v, errors.New("business session not healthy writable")
	}
	return v, nil
}
func (b *nativeBusinessProbe) run() error {
	b.mu.Lock()
	if b.closed || b.started {
		b.mu.Unlock()
		return b.fail(errors.New("business duplicate or canceled admission"))
	}
	b.started = true
	b.lane.Enqueue(func() { b.execute() })
	b.mu.Unlock()
	select {
	case <-b.cancel:
		return b.fail(errors.New("business canceled before completion"))
	case err := <-b.ack:
		return err
	}
}
func (b *nativeBusinessProbe) finish(err error) {
	if err != nil {
		b.fail(err)
	}
	select {
	case b.ack <- err:
	default:
	}
}
func (b *nativeBusinessProbe) beforeCommand(phase string) error {
	if b.canceled() {
		return errors.New("business canceled")
	}
	if err := b.sample(phase); err != nil {
		return err
	}
	_, err := b.read()
	return err
}
func (b *nativeBusinessProbe) execute() {
	if b.canceled() {
		b.finish(errors.New("business canceled"))
		return
	}
	v, err := b.read()
	if err != nil {
		b.finish(err)
		return
	}
	b.mu.Lock()
	b.report.BeforeStoreRevision = v.StoreRevision
	b.report.BeforeClassificationRevision = uint64(v.Classification.Revision)
	b.mu.Unlock()
	before := v
	if b.report.Stage == "create" {
		if err = validateNativeBusinessEmpty(v, b.today); err != nil {
			b.finish(err)
			return
		}
		if err = b.beforeCommand("before_add"); err != nil {
			b.finish(err)
			return
		}
		err = b.service.AddToday(nativeBusinessTaskTitle)
		op := nativeBusinessOperation{Name: "AddToday"}
		if err != nil {
			op.Error = err.Error()
		}
		b.mu.Lock()
		b.report.Operations = append(b.report.Operations, op)
		b.mu.Unlock()
		dateErr := b.sample("after_add")
		if err = errors.Join(err, dateErr); err != nil {
			b.finish(err)
			return
		}
		v, err = b.read()
		if err != nil {
			b.finish(err)
			return
		}
		chain, e := validateNativeBusinessTask(v, b.today)
		if e != nil || v.StoreRevision != before.StoreRevision+1 || v.Classification.Revision != 0 || !reflect.DeepEqual(v.Classification, before.Classification) {
			b.finish(errors.Join(e, errors.New("AddToday fixed fixture/revision mismatch")))
			return
		}
		taskGraph := v.Graph
		if err = b.beforeCommand("before_category"); err != nil {
			b.finish(err)
			return
		}
		id, e := core.NewIdentity()
		if e != nil {
			b.finish(e)
			return
		}
		out := b.service.ReplaceCategorySelectionOn(b.today, chain, ordinary.Choice{Kind: ordinary.ChoiceNew, Name: nativeBusinessCategoryName, ColorHex: nativeBusinessCategoryColor}, ordinary.UUID(id))
		op = nativeBusinessOperation{Name: "ReplaceCategorySelectionOn", Outcome: businessOutcome(out.Outcome), Preparation: businessOutcome(out.Preparation), InteractionID: out.InteractionID, PlanID: out.PlanID, Receipt: out.Receipt}
		if out.Err != nil {
			op.Error = out.Err.Error()
		}
		b.mu.Lock()
		b.report.Operations = append(b.report.Operations, op)
		b.mu.Unlock()
		dateErr = b.sample("after_category")
		if out.Err != nil || dateErr != nil {
			b.finish(errors.Join(out.Err, dateErr))
			return
		}
		if out.Result != taskapp.ResultConfirmed || out.WriteExtent != taskapp.WriteCommittedConfirmed || out.Session != taskapp.SessionWritable || out.FailureStage != taskapp.StageNone || out.Receipt == nil || out.Preparation.Err != nil || out.Preparation.Result != taskapp.ResultConfirmed || out.Preparation.WriteExtent != taskapp.WriteNone || out.Preparation.Session != taskapp.SessionWritable || out.Preparation.FailureStage != taskapp.StageNone || out.InteractionID != ordinary.UUID(id) {
			b.finish(errors.New("category outcome not confirmed"))
			return
		}
		v, err = b.read()
		if err != nil {
			b.finish(err)
			return
		}
		receipt, ok := v.Classification.CommittedReceiptsByInteractionID[ordinary.UUID(id)]
		if !ok || !nativeBusinessReceiptMatches(receipt, *out.Receipt) || out.PlanID != receipt.PlanID || !reflect.DeepEqual(v.Graph, taskGraph) || !reflect.DeepEqual(out.View, v) || v.StoreRevision != before.StoreRevision+2 {
			b.finish(errors.New("category command receipt/facts/revision mismatch"))
			return
		}
	} else if b.report.Stage != "open" {
		b.finish(errors.New("unknown business stage"))
		return
	}
	if _, err = validateNativeBusinessFinal(v, b.today); err != nil {
		b.finish(err)
		return
	}
	payload, digest, err := nativeBusinessFactsJSON(v)
	if err != nil {
		b.finish(err)
		return
	}
	if b.canceled() {
		b.finish(errors.New("business canceled before publish"))
		return
	}
	// This is the sole completion post, from the accepted writer task. Cleanup
	// cannot close dispatcher admission until this writer has fully drained.
	b.post(func() {
		if b.canceled() {
			return
		}
		if err := b.publish(v); err != nil {
			b.finish(err)
			return
		}
		if err := b.sample("publish_ack"); err != nil {
			b.finish(err)
			return
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		b.acknowledged = v
		b.report.UIReadViewAcknowledged = true
		b.report.PersistentFactsJSON = payload
		b.report.PersistentFactsSHA256 = digest
		b.report.StoreRevision = v.StoreRevision
		b.report.ClassificationRevision = uint64(v.Classification.Revision)
		b.mu.Unlock()
		b.finish(nil)
	})
}

// Called on the real UI thread, after attach, never by the writer/cleanup.
func (b *nativeBusinessProbe) bind(h *titleHub, s *app, id int) {
	b.report.WindowID = id
	b.publish = func(v taskapp.OrdinaryReadView) error {
		w := h.windows[s.titleWindowID]
		if id <= 0 || len(h.windows) != 1 || w == nil || w.app != s || s.titles != h || h.store != b.service || h.category == nil || h.category.store != b.service {
			return errors.New("business window/service binding mismatch")
		}
		runtime, ok := s.runtime.(*titleReadView)
		if !ok || runtime.hub != h {
			return errors.New("business app read-view binding mismatch")
		}
		h.category.view = v
		h.publish(v.Graph, v.Today)
		if h.windows[s.titleWindowID] != w || w.app != s || s.runtimeError != "" || !reflect.DeepEqual(h.graph, v.Graph) || h.today != v.Today || !reflect.DeepEqual(h.category.view, v) || !reflect.DeepEqual(s.runtime.Snapshot(), v.Graph) || s.runtime.Today() != v.Today {
			return errors.New("business UI read-view publish not acknowledged")
		}
		w.invalidate()
		b.mu.Lock()
		b.report.WindowBindingConfirmed = true
		b.mu.Unlock()
		return nil
	}
}

// NON-UI cleanup only, after writer drain and before Service.Close. Never read
// mutable app/hub state here; acknowledged was frozen on the main thread.
func (b *nativeBusinessProbe) PreClose() error {
	v, err := b.read()
	if err != nil {
		return b.fail(err)
	}
	b.mu.Lock()
	ack := b.acknowledged
	acknowledged := b.report.UIReadViewAcknowledged
	b.mu.Unlock()
	if !acknowledged || !reflect.DeepEqual(v, ack) {
		return b.fail(errors.New("business pre-close facts differ or UI ack absent"))
	}
	b.mu.Lock()
	b.preCloseChecked = true
	b.report.PreCloseHealth = v.Health
	b.mu.Unlock()
	return nil
}
func (b *nativeBusinessProbe) Finalize(lifecycleErr error) ([]byte, error) {
	b.fail(lifecycleErr)
	b.fail(b.sample("finalize"))
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed || !b.started || !b.preCloseChecked || !b.report.UIReadViewAcknowledged || !b.report.WindowBindingConfirmed {
		b.report.Errors = append(b.report.Errors, "business finalization incomplete")
	}
	b.report.Complete = len(b.report.Errors) == 0
	p, err := json.Marshal(b.report)
	if err != nil {
		return nil, err
	}
	p = append(p, '\n')
	if len(p) > 64*1024 {
		return nil, errors.New("business report too large")
	}
	if !b.report.Complete {
		return p, fmt.Errorf("native business failed: %v", b.report.Errors)
	}
	return p, nil
}

// The live outcome uses a nil notices slice, whereas the wire decoder yields
// an empty slice. Check every receipt field and both empty lengths explicitly;
// never normalize the persistent DTO used for cross-process comparison.
func nativeBusinessReceiptMatches(a, b ordinary.Receipt) bool {
	return a.PlanID == b.PlanID && a.Revision == b.Revision && len(a.Notices) == 0 && len(b.Notices) == 0 && a.ChangeRecordID == b.ChangeRecordID && a.DecisionID == b.DecisionID && a.ChangeRecordIntegrityDigest == b.ChangeRecordIntegrityDigest
}
