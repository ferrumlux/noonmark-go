// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// A prefix observation must survive even if another actor restores the bytes.
// This test deliberately uses only legacy APIs, so its baseline failure is a
// product behavior regression, not an absent typed-API compilation failure.
func TestOutcomeLegacyPrefixFailureCannotRevive(t *testing.T) {
	s, path, _, trace := manualServiceFixture(t)
	before, revision := s.Snapshot(), s.disk.revision
	target := filepath.Join(path, fmt.Sprintf("rev-%010d", revision), "COMMITTED")
	original, err := os.ReadFile(target)
	must(t, err)
	must(t, os.WriteFile(target, []byte("broken\n"), 0600))
	_, err = s.SaveTraceTitleInputAcknowledged(trace, "合成手动进度")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("first detection must preserve ErrCorrupt: %v", err)
	}
	must(t, os.WriteFile(target, original, 0600))
	if err = s.AddToday("不应写入"); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("known prefix failure was forgotten; subsequent legacy AddToday = %v, want ErrIncomplete", err)
	}
	if revision != s.disk.revision || !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed session published a later command")
	}
}

func checkOutcome(t *testing.T, o Outcome, receipt bool) {
	t.Helper()
	switch o.Result {
	case ResultConfirmed:
		if o.Err != nil || !receipt || o.Session != SessionWritable || (o.WriteExtent != WriteNone && o.WriteExtent != WriteCommittedConfirmed) {
			t.Fatalf("invalid confirmed outcome: %+v, receipt=%v", o, receipt)
		}
	case ResultRejected:
		if o.Err == nil || receipt || o.WriteExtent != WriteNone || (o.Session != SessionWritable && o.Session != SessionNeedsReopen && o.Session != SessionClosed) {
			t.Fatalf("invalid rejected outcome: %+v", o)
		}
	case ResultUncertain:
		if o.Err == nil || receipt || o.Session != SessionNeedsReopen || (o.WriteExtent != WriteReservationPossible && o.WriteExtent != WriteReservedOrPartial && o.WriteExtent != WriteWrittenUnacknowledged) {
			t.Fatalf("invalid uncertain outcome: %+v", o)
		}
	default:
		t.Fatalf("unknown result: %+v", o)
	}
}

func TestOutcomeManualReceiptNoopClampAndOwning(t *testing.T) {
	s, _, chain, trace := manualServiceFixture(t)
	rev := s.disk.revision
	o := s.SetManualProgressOutcome(chain, trace, -9)
	checkOutcome(t, o.Outcome, o.Receipt != nil)
	if o.Result != ResultConfirmed || o.WriteExtent != WriteCommittedConfirmed || o.Receipt.Revision != rev+1 || o.Receipt.Hash != s.disk.hash || o.Receipt.Percent != 0 || o.Receipt.Chain != chain || o.Receipt.Trace != trace {
		t.Fatalf("nil to zero: %+v %+v", o, o.Receipt)
	}
	saved := *o.Receipt
	s.clock = func() time.Time { t.Fatal("no-op sampled clock"); return time.Time{} }
	noop := s.SetManualProgressOutcome(chain, trace, 0)
	checkOutcome(t, noop.Outcome, noop.Receipt != nil)
	if noop.WriteExtent != WriteNone || *noop.Receipt != saved {
		t.Fatalf("no-op receipt differs: %+v", noop)
	}
	s.clock = time.Now
	max := s.SetManualProgressOutcome(chain, trace, 1000)
	checkOutcome(t, max.Outcome, max.Receipt != nil)
	if max.Receipt.Percent != 100 || serviceManualTrace(t, s, trace).Status != core.TracePending {
		t.Fatal("clamp must not complete")
	}
	must(t, s.Close())
	if *o.Receipt != saved {
		t.Fatal("retained receipt changed with later write/close")
	}
}

func TestOutcomeFaultPhaseNotErrorName(t *testing.T) {
	sentinel := errors.New("same injected failure")
	for _, tc := range []struct {
		point  string
		result Result
		extent WriteExtent
		stage  FailureStage
		health SessionState
	}{
		{"before-reserve", ResultRejected, WriteNone, StageReserve, SessionWritable},
		{"after-reserve", ResultUncertain, WriteReservedOrPartial, StageReserve, SessionNeedsReopen},
		{"before-commit", ResultUncertain, WriteReservedOrPartial, StageMarker, SessionNeedsReopen},
		{"after-persist", ResultUncertain, WriteWrittenUnacknowledged, StagePostPersist, SessionNeedsReopen},
	} {
		t.Run(tc.point, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			before, rev, hash := s.Snapshot(), s.disk.revision, s.disk.hash
			calls := 0
			s.disk.fault = func(point string) error {
				if point == tc.point {
					calls++
					return sentinel
				}
				return nil
			}
			o := s.SetManualProgressOutcome(chain, trace, 45)
			checkOutcome(t, o.Outcome, o.Receipt != nil)
			if o.Result != tc.result || o.WriteExtent != tc.extent || o.Session != tc.health || o.FailureStage != tc.stage || calls != 1 {
				t.Fatalf("phase mismatch: %+v calls %d", o, calls)
			}
			want := sentinel
			if tc.point == "after-persist" {
				want = ErrIncomplete
			}
			if !errors.Is(o.Err, want) {
				t.Fatalf("original error changed: %v", o.Err)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || rev != s.disk.revision || hash != s.disk.hash {
				t.Fatal("failure published candidate")
			}
			entries, err := os.ReadDir(path)
			must(t, err)
			s.disk.fault = nil
			err = s.AddToday("显式新请求")
			if tc.health == SessionWritable {
				must(t, err)
			} else {
				if !errors.Is(err, ErrIncomplete) {
					t.Fatalf("legacy bypass: %v", err)
				}
				after, err := os.ReadDir(path)
				must(t, err)
				if len(entries) != len(after) {
					t.Fatal("failed session wrote later revision")
				}
			}
		})
	}
}

func TestOutcomeGuardsCloseAndDatePriority(t *testing.T) {
	s, _, chain, trace := manualServiceFixture(t)
	guard := s.SetManualProgressOutcome("missing", trace, 30)
	checkOutcome(t, guard.Outcome, guard.Receipt != nil)
	if guard.Result != ResultRejected || guard.Session != SessionWritable {
		t.Fatal(guard)
	}
	must(t, s.AddToday("合法无关操作"))
	must(t, s.Close())
	invalid := s.AdvanceToOutcome(core.LocalDate{})
	checkOutcome(t, invalid.Outcome, invalid.Receipt != nil)
	if !errors.Is(invalid.Err, core.ErrInvalidInput) || invalid.Session != SessionClosed {
		t.Fatal(invalid)
	}
	closed := s.SetManualProgressOutcome(chain, trace, 30)
	checkOutcome(t, closed.Outcome, closed.Receipt != nil)
	if !errors.Is(closed.Err, ErrSession) || closed.Session != SessionClosed {
		t.Fatal(closed)
	}
	must(t, s.Close())
}

func TestOutcomeTwoStageReceiptsAndInterleaving(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, _, chain, trace := manualServiceFixture(t)
			next, err := testDay.AddDays(1)
			must(t, err)
			a := s.AdvanceToOutcome(next)
			checkOutcome(t, a.Outcome, a.Receipt != nil)
			saved := *a.Receipt
			if a.Receipt.Today != next || s.Today() != next {
				t.Fatal("advance lost date")
			}
			must(t, s.AddToday("两阶段间另一命令"))
			if uncertain {
				tr := s.Snapshot().Traces[len(s.Snapshot().Traces)-1]
				chain, trace = tr.ChainID, tr.ID
				s.disk.fault = func(p string) error {
					if p == "after-reserve" {
						return errors.New("reserve failure")
					}
					return nil
				}
			}
			m := s.SetManualProgressOutcome(chain, trace, 25)
			checkOutcome(t, m.Outcome, m.Receipt != nil)
			if uncertain {
				if m.Result != ResultUncertain {
					t.Fatal(m)
				}
			} else {
				if m.Result != ResultRejected || m.Session != SessionWritable {
					t.Fatal(m)
				}
				must(t, s.AddToday("后段业务拒绝后继续"))
			}
			if *a.Receipt != saved {
				t.Fatal("later stage rewrote advance receipt")
			}
		})
	}
}

func TestOutcomeLegacyAllMutatorsRespectStickyGate(t *testing.T) {
	s, path, chain, trace := manualServiceFixture(t)
	def := s.Snapshot().Definitions[0].ID
	target := filepath.Join(path, fmt.Sprintf("rev-%010d", s.disk.revision), "COMMITTED")
	original, err := os.ReadFile(target)
	must(t, err)
	must(t, os.WriteFile(target, []byte("bad\n"), 0600))
	_, err = s.SaveTraceTitleInputAcknowledged(trace, "合成手动进度")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	must(t, os.WriteFile(target, original, 0600))
	before, rev := s.Snapshot(), s.disk.revision
	clockCalls := 0
	s.clock = func() time.Time { clockCalls++; return time.Now() }
	calls := []struct {
		name string
		call func() error
	}{
		{"AdvanceTo", func() error { return s.AdvanceTo(testDay) }},
		{"AddPool", func() error { return s.AddPool("a") }},
		{"AddToday", func() error { return s.AddToday("a") }},
		{"AddOnDate", func() error { return s.AddOnDate("a", testDay) }},
		{"Schedule", func() error { return s.Schedule(chain, testDay) }},
		{"SetCompleted", func() error { return s.SetCompleted(trace, true) }},
		{"Manual", func() error { return s.SetManualProgress(chain, trace, 2) }},
		{"ManualAcknowledged", func() error { _, e := s.SetManualProgressAcknowledged(chain, trace, 2); return e }},
		{"RenameTitle", func() error { return s.RenameTaskTitle(chain, "a") }},
		{"SaveTitle", func() error { return s.SaveTaskTitleInput(chain, "a") }},
		{"SaveTraceTitle", func() error { return s.SaveTraceTitleInput(trace, "a") }},
		{"SaveTraceTitleAcknowledged", func() error { _, e := s.SaveTraceTitleInputAcknowledged(trace, "a"); return e }},
		{"AddPlanned", func() error { _, e := s.AddPlannedSubtask(chain, def, "a", core.DifficultySimple); return e }},
		{"SavePlanned", func() error { _, e := s.SavePlannedSubtaskTitleAcknowledged(chain, def, "missing", "a"); return e }},
		{"DifficultyPlanned", func() error { return s.SetPlannedSubtaskDifficulty(chain, def, "missing", core.DifficultySimple) }},
		{"RemovePlanned", func() error { return s.RemovePlannedSubtask(chain, def, "missing") }},
		{"AddSubtask", func() error { _, e := s.AddSubtask(chain, trace, "a", core.DifficultySimple); return e }},
		{"SaveSubtask", func() error { _, e := s.SaveSubtaskTitleAcknowledged(chain, trace, "missing", "a"); return e }},
		{"DifficultySubtask", func() error { return s.SetSubtaskDifficulty(chain, trace, "missing", core.DifficultySimple) }},
		{"DeleteSubtask", func() error { return s.DeleteSubtask(chain, trace, "missing") }},
		{"CompleteSubtask", func() error { return s.SetSubtaskCompleted(chain, trace, "missing", true) }},
		{"AbandonSubtask", func() error { return s.AbandonSubtask(chain, trace, "missing") }},
		{"Return", func() error { return s.ReturnToPool(chain, trace) }},
		{"Defer", func() error { _, e := s.DeferCurrentTrace(chain, trace, testDay); return e }},
		{"Continue", func() error { _, e := s.ContinueUnfinishedTrace(chain, trace, testDay); return e }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			if e := c.call(); !errors.Is(e, ErrIncomplete) {
				t.Fatalf("bypassed gate: %v", e)
			}
		})
	}
	if clockCalls != 0 {
		t.Errorf("session gate sampled clock %d times", clockCalls)
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev || s.Health().Session != SessionNeedsReopen {
		t.Fatal("sticky gate mutated state")
	}
}

func TestOutcomeHealthCloseWriteRaces(t *testing.T) {
	for i := 0; i < 12; i++ {
		s, _, chain, trace := manualServiceFixture(t)
		var o ManualOutcome
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); o = s.SetManualProgressOutcome(chain, trace, 33) }()
		go func() { defer wg.Done(); _ = s.Close() }()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Health()
				_ = s.Snapshot()
				_ = s.Today()
			}
		}()
		wg.Wait()
		checkOutcome(t, o.Outcome, o.Receipt != nil)
		if o.Result == ResultConfirmed && o.Receipt.Percent != 33 {
			t.Fatal("wrong successful race ack")
		}
		if o.Result == ResultRejected && !errors.Is(o.Err, ErrSession) {
			t.Fatal(o)
		}
		if s.Health().Session != SessionClosed {
			t.Fatal("close not sticky")
		}
	}
}

func TestOutcomePrefixReadFailuresHaveNoCurrentWrite(t *testing.T) {
	for _, kind := range []string{"marker", "extra-incomplete-revision", "payload", "commit-marker", "head", "path"} {
		t.Run(kind, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			must(t, s.SetManualProgress(chain, trace, 20))
			rev := s.disk.revision
			switch kind {
			case "marker":
				must(t, os.WriteFile(filepath.Join(path, markerName), []byte("wrong"), 0600))
			case "extra-incomplete-revision":
				must(t, os.Mkdir(filepath.Join(path, fmt.Sprintf("rev-%010d", rev+1)), 0700))
			case "payload":
				must(t, os.WriteFile(filepath.Join(path, fmt.Sprintf("rev-%010d", rev), "state.json"), []byte("bad"), 0600))
			case "commit-marker":
				must(t, os.Remove(filepath.Join(path, fmt.Sprintf("rev-%010d", rev), "COMMITTED")))
			case "head":
				s.disk.hash = "wrong"
			case "path":
				s.disk.path = filepath.Join(path, "absent")
			}
			before := s.Snapshot()
			o := s.SetManualProgressOutcome(chain, trace, 20)
			checkOutcome(t, o.Outcome, o.Receipt != nil)
			if o.Result != ResultRejected || o.WriteExtent != WriteNone || o.Session != SessionNeedsReopen || o.FailureStage != StagePrefix {
				t.Fatal(o)
			}
			if rev != s.disk.revision || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("pure prefix read published candidate")
			}
			invalid := s.AdvanceToOutcome(core.LocalDate{})
			if !errors.Is(invalid.Err, core.ErrInvalidInput) || invalid.Session != SessionNeedsReopen {
				t.Fatal("parameter rejection cleared health", invalid)
			}
			health := s.Health()
			health.Session = SessionWritable
			if s.Health().Session != SessionNeedsReopen {
				t.Fatal("Health aliases mutable state")
			}
		})
	}
}

func TestOutcomeFilesystemReservationPayloadMarkerReadback(t *testing.T) {
	for _, kind := range []string{"mkdir", "payload", "marker", "readback"} {
		t.Run(kind, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			name := fmt.Sprintf("rev-%010d", s.disk.revision+1)
			s.disk.fault = func(point string) error {
				if kind == "mkdir" && point == "before-reserve" {
					must(t, os.Mkdir(filepath.Join(path, name), 0700))
				}
				if kind == "payload" && point == "after-reserve" {
					must(t, os.Mkdir(filepath.Join(path, name, "state.json"), 0700))
				}
				if kind == "marker" && point == "before-commit" {
					must(t, os.Mkdir(filepath.Join(path, name, "COMMITTED"), 0700))
				}
				if kind == "readback" && point == "after-persist" {
					must(t, os.WriteFile(filepath.Join(path, name, "COMMITTED"), []byte("bad"), 0600))
				}
				return nil
			}
			before := s.Snapshot()
			rev := s.disk.revision
			o := s.SetManualProgressOutcome(chain, trace, 23)
			checkOutcome(t, o.Outcome, o.Receipt != nil)
			wantExtent, wantStage := WriteReservedOrPartial, StagePayload
			switch kind {
			case "mkdir":
				wantExtent, wantStage = WriteReservationPossible, StageReserve
			case "marker":
				wantStage = StageMarker
			case "readback":
				wantExtent, wantStage = WriteWrittenUnacknowledged, StageReadback
			}
			if o.Result != ResultUncertain || o.WriteExtent != wantExtent || o.FailureStage != wantStage {
				t.Fatal(o)
			}
			if rev != s.disk.revision || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("uncertain result published")
			}
			s.disk.fault = nil
			if !errors.Is(s.AddPool("不能重写"), ErrIncomplete) {
				t.Fatal("uncertain session accepted write")
			}
		})
	}
}

func TestOutcomeAdvanceNoopReceiptAndLegacyBlocked(t *testing.T) {
	s, _ := fresh(t)
	s.clock = func() time.Time { t.Fatal("no-op clock"); return time.Time{} }
	o := s.AdvanceToOutcome(testDay)
	checkOutcome(t, o.Outcome, o.Receipt != nil)
	if o.WriteExtent != WriteNone || o.Receipt.Revision != s.disk.revision || o.Receipt.Hash != s.disk.hash || o.Receipt.Today != testDay {
		t.Fatal(o)
	}
	s.disk.blocked = true
	blocked := s.AdvanceToOutcome(testDay)
	checkOutcome(t, blocked.Outcome, blocked.Receipt != nil)
	if !errors.Is(blocked.Err, ErrIncomplete) || blocked.Session != SessionNeedsReopen {
		t.Fatal(blocked)
	}
	s.disk.blocked = false
	if !errors.Is(s.AdvanceTo(testDay), ErrIncomplete) {
		t.Fatal("legacy blocked evidence not sticky")
	}
}

func TestOutcomeProtocolRejectsInvalidCombinations(t *testing.T) {
	err := errors.New("failure")
	good := []struct {
		o       Outcome
		receipt bool
	}{
		{Outcome{ResultConfirmed, WriteNone, SessionWritable, StageNone, nil}, true},
		{Outcome{ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, nil}, true},
		{Outcome{ResultRejected, WriteNone, SessionWritable, StageDomain, err}, false},
		{Outcome{ResultRejected, WriteNone, SessionNeedsReopen, StagePrefix, err}, false},
		{Outcome{ResultRejected, WriteNone, SessionClosed, StageSessionGate, err}, false},
		{Outcome{ResultUncertain, WriteReservationPossible, SessionNeedsReopen, StageReserve, err}, false},
		{Outcome{ResultUncertain, WriteReservedOrPartial, SessionNeedsReopen, StagePayload, err}, false},
		{Outcome{ResultUncertain, WriteWrittenUnacknowledged, SessionNeedsReopen, StageReadback, err}, false},
	}
	for _, g := range good {
		if !validOutcome(g.o, g.receipt) {
			t.Fatalf("valid combination rejected: %+v", g)
		}
	}
	bad := []struct {
		o       Outcome
		receipt bool
	}{
		{Outcome{}, false},
		{Outcome{ResultConfirmed, WriteNone, SessionWritable, StageNone, err}, true},
		{Outcome{ResultConfirmed, WriteNone, SessionWritable, StageNone, nil}, false},
		{Outcome{ResultUncertain, WriteReservedOrPartial, SessionWritable, StagePayload, err}, false},
		{Outcome{ResultRejected, WriteNone, SessionNeedsReopen, StagePrefix, err}, true},
		{Outcome{ResultUncertain, WriteNone, SessionNeedsReopen, StagePrefix, err}, false},
	}
	for _, b := range bad {
		if validOutcome(b.o, b.receipt) {
			t.Fatalf("invalid combination accepted: %+v", b)
		}
	}
}

func TestOutcomeAdvanceWritePhasesAndWrapperOnce(t *testing.T) {
	for _, point := range []string{"", "before-reserve", "after-reserve", "after-persist"} {
		t.Run(point, func(t *testing.T) {
			s, _, _, _ := manualServiceFixture(t)
			before, rev := s.Snapshot(), s.disk.revision
			next, err := testDay.AddDays(1)
			must(t, err)
			clockCalls := 0
			s.clock = func() time.Time { clockCalls++; return time.Now() }
			sentinel := errors.New("advance fault")
			s.disk.fault = func(p string) error {
				if p == point {
					return sentinel
				}
				return nil
			}
			o := s.AdvanceToOutcome(next)
			checkOutcome(t, o.Outcome, o.Receipt != nil)
			if clockCalls != 1 {
				t.Fatalf("advance clock calls=%d", clockCalls)
			}
			if point == "" {
				if o.Result != ResultConfirmed || o.WriteExtent != WriteCommittedConfirmed || o.Receipt.Revision != rev+1 || o.Receipt.Today != next {
					t.Fatal(o)
				}
			} else {
				if o.Receipt != nil || s.Today() != testDay || !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev {
					t.Fatal("failed advance published")
				}
				if point == "before-reserve" {
					if o.Result != ResultRejected || o.Session != SessionWritable {
						t.Fatal(o)
					}
				} else if o.Result != ResultUncertain || o.Session != SessionNeedsReopen {
					t.Fatal(o)
				}
			}
		})
	}
	s, _, chain, trace := manualServiceFixture(t)
	clockCalls, reserveCalls := 0, 0
	s.clock = func() time.Time { clockCalls++; return time.Now() }
	s.disk.fault = func(p string) error {
		if p == "before-reserve" {
			reserveCalls++
		}
		return nil
	}
	rev := s.disk.revision
	ack, err := s.SetManualProgressAcknowledged(chain, trace, 32)
	must(t, err)
	if ack != 32 || s.disk.revision != rev+1 || clockCalls != 1 || reserveCalls != 1 {
		t.Fatal("ack wrapper replayed command")
	}
	must(t, s.SetManualProgress(chain, trace, 32))
	if s.disk.revision != rev+1 || clockCalls != 1 || reserveCalls != 1 {
		t.Fatal("legacy no-op replayed command")
	}
	next, err := testDay.AddDays(1)
	must(t, err)
	must(t, s.AdvanceTo(next))
	if s.disk.revision != rev+2 || clockCalls != 2 || reserveCalls != 2 {
		t.Fatal("advance wrapper replayed command")
	}
}

func TestOutcomeFailedAdvanceDoesNotAttemptManual(t *testing.T) {
	for _, point := range []string{"before-reserve", "after-reserve"} {
		t.Run(point, func(t *testing.T) {
			s, _, chain, trace := manualServiceFixture(t)
			next, err := testDay.AddDays(1)
			must(t, err)
			s.disk.fault = func(p string) error {
				if p == point {
					return errors.New("failure")
				}
				return nil
			}
			a := s.AdvanceToOutcome(next)
			calls := 0
			// The composition belongs to a caller. Service must not invent a second
			// transaction or a zero-percentage acknowledgment after this failure.
			if a.Result == ResultConfirmed && a.Session == SessionWritable {
				calls++
				_ = s.SetManualProgressOutcome(chain, trace, 0)
			}
			if calls != 0 || a.Receipt != nil {
				t.Fatal("failed advance triggered manual stage")
			}
		})
	}
}
