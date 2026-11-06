// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// 独立验收只以真实 Service 领域命令建图，文件变更仅用于隔离故障注入。
func independentFixture(t *testing.T) (*Service, string, core.TaskChainID, core.DayTraceID) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "development")
	s, err := Create(path, core.LocalDate{Year: 2026, Month: 11, Day: 6})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.clock = func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }
	if err := s.AddToday("独立合成任务"); err != nil {
		t.Fatal(err)
	}
	tr := s.Snapshot().Traces[0]
	return s, path, tr.ChainID, tr.ID
}
func independentFiles(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(path, p)
		if e != nil {
			return e
		}
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func independentUnchanged(t *testing.T, path string, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(want, independentFiles(t, path)) {
		t.Fatal("隔离磁盘资料被改写")
	}
}
func independentCorrupt(t *testing.T, path string) func() {
	t.Helper()
	p := filepath.Join(path, "rev-0000000001", "COMMITTED")
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, []byte("broken\n"), 0600); e != nil {
		t.Fatal(e)
	}
	return func() {
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestIndependentLegacyEveryMutatorStopsAfterKnownPrefixFailure(t *testing.T) {
	s, path, chain, trace := independentFixture(t)
	restore := independentCorrupt(t, path)
	if _, e := s.SaveTraceTitleInputAcknowledged(trace, "独立合成任务"); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("first prefix error=%v", e)
	}
	// 即使外部修回原 bytes，已经失效的对象也不可复活。
	restore()
	before := independentFiles(t, path)
	graph := s.Snapshot()
	calls := 0
	s.clock = func() time.Time { calls++; return time.Now() }
	def := graph.Definitions[0].ID
	day := s.Today()
	commands := map[string]func() error{
		"AddPool":             func() error { return s.AddPool("另一个任务") },
		"AddToday":            func() error { return s.AddToday("另一个任务") },
		"AddOnDate":           func() error { return s.AddOnDate("另一个任务", day) },
		"Schedule":            func() error { return s.Schedule(chain, day) },
		"SetCompleted":        func() error { return s.SetCompleted(trace, true) },
		"AdvanceTo":           func() error { return s.AdvanceTo(day) },
		"SetManualProgress":   func() error { return s.SetManualProgress(chain, trace, 30) },
		"ManualAcknowledged":  func() error { _, e := s.SetManualProgressAcknowledged(chain, trace, 30); return e },
		"RenameTaskTitle":     func() error { return s.RenameTaskTitle(chain, "新标题") },
		"SaveTaskTitleInput":  func() error { return s.SaveTaskTitleInput(chain, "新标题") },
		"SaveTraceTitleInput": func() error { return s.SaveTraceTitleInput(trace, "新标题") },
		"TraceAcknowledged":   func() error { _, e := s.SaveTraceTitleInputAcknowledged(trace, "新标题"); return e },
		"AddPlannedSubtask":   func() error { _, e := s.AddPlannedSubtask(chain, def, "步骤", core.DifficultySimple); return e },
		"SavePlannedTitle":    func() error { _, e := s.SavePlannedSubtaskTitleAcknowledged(chain, def, "", "步骤"); return e },
		"PlannedDifficulty":   func() error { return s.SetPlannedSubtaskDifficulty(chain, def, "", core.DifficultySimple) },
		"RemovePlanned":       func() error { return s.RemovePlannedSubtask(chain, def, "") },
		"AddSubtask":          func() error { _, e := s.AddSubtask(chain, trace, "步骤", core.DifficultySimple); return e },
		"SaveSubtaskTitle":    func() error { _, e := s.SaveSubtaskTitleAcknowledged(chain, trace, "", "步骤"); return e },
		"SubtaskDifficulty":   func() error { return s.SetSubtaskDifficulty(chain, trace, "", core.DifficultySimple) },
		"DeleteSubtask":       func() error { return s.DeleteSubtask(chain, trace, "") },
		"SubtaskCompleted":    func() error { return s.SetSubtaskCompleted(chain, trace, "", true) },
		"AbandonSubtask":      func() error { return s.AbandonSubtask(chain, trace, "") },
		"ReturnToPool":        func() error { return s.ReturnToPool(chain, trace) },
		"Defer":               func() error { _, e := s.DeferCurrentTrace(chain, trace, day); return e },
		"Continue":            func() error { _, e := s.ContinueUnfinishedTrace(chain, trace, day); return e },
	}
	for name, fn := range commands {
		t.Run(name, func(t *testing.T) {
			if e := fn(); !errors.Is(e, ErrIncomplete) {
				t.Errorf("known failure bypass: %v", e)
			}
			independentUnchanged(t, path, before)
		})
	}
	if calls != 0 {
		t.Errorf("blocked commands sampled clock %d times", calls)
	}
	if !reflect.DeepEqual(graph, s.Snapshot()) {
		t.Error("blocked commands published graph")
	}
}

func independentOutcome(t *testing.T, o Outcome, r Result, w WriteExtent, s SessionState, stage FailureStage, receipt bool) {
	t.Helper()
	if o.Result != r || o.WriteExtent != w || o.Session != s || o.FailureStage != stage {
		t.Fatalf("outcome=%+v want %v/%v/%v/%v", o, r, w, s, stage)
	}
	if (o.Err == nil) != (r == ResultConfirmed) || receipt != (r == ResultConfirmed) {
		t.Fatalf("invalid receipt/error combination: %+v receipt=%v", o, receipt)
	}
}
func TestIndependentTypedManualSuccessNoopAndReceiptIsolation(t *testing.T) {
	s, path, c, tr := independentFixture(t)
	n := s.disk.revision
	o := s.SetManualProgressOutcome(c, tr, -5)
	independentOutcome(t, o.Outcome, ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, o.Receipt != nil)
	if o.Receipt.Percent != 0 || o.Receipt.Revision != n+1 || o.Receipt.Hash != s.disk.hash || o.Receipt.Chain != c || o.Receipt.Trace != tr {
		t.Fatalf("wrong receipt %+v", o.Receipt)
	}
	receipt := *o.Receipt
	files := independentFiles(t, path)
	calls := 0
	s.clock = func() time.Time { calls++; return time.Now() }
	same := s.SetManualProgressOutcome(c, tr, 0)
	independentOutcome(t, same.Outcome, ResultConfirmed, WriteNone, SessionWritable, StageNone, same.Receipt != nil)
	a := s.AdvanceToOutcome(s.Today())
	independentOutcome(t, a.Outcome, ResultConfirmed, WriteNone, SessionWritable, StageNone, a.Receipt != nil)
	if calls != 0 {
		t.Fatalf("no-op sampled clock %d", calls)
	}
	independentUnchanged(t, path, files)
	max := s.SetManualProgressOutcome(c, tr, 999)
	independentOutcome(t, max.Outcome, ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, max.Receipt != nil)
	if max.Receipt.Percent != 100 || s.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("100 completed or wrong clamp")
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if *o.Receipt != receipt {
		t.Fatal("retained receipt mutated")
	}
}
func TestIndependentTypedGuardAndClosedOrder(t *testing.T) {
	s, path, _, _ := independentFixture(t)
	before := independentFiles(t, path)
	o := s.SetManualProgressOutcome("", "", 20)
	independentOutcome(t, o.Outcome, ResultRejected, WriteNone, SessionWritable, StageDomain, o.Receipt != nil)
	independentUnchanged(t, path, before)
	if e := s.AddToday("仍然可写"); e != nil {
		t.Fatal(e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	a := s.AdvanceToOutcome(core.LocalDate{})
	independentOutcome(t, a.Outcome, ResultRejected, WriteNone, SessionClosed, StageParameter, a.Receipt != nil)
	if !errors.Is(a.Err, core.ErrInvalidInput) {
		t.Fatal(a.Err)
	}
	a = s.AdvanceToOutcome(s.Today())
	independentOutcome(t, a.Outcome, ResultRejected, WriteNone, SessionClosed, StageSessionGate, a.Receipt != nil)
	if !errors.Is(a.Err, ErrSession) {
		t.Fatal(a.Err)
	}
}
func TestIndependentTypedFaultStagesAndRecovery(t *testing.T) {
	boom := errors.New("independent injected fault")
	cases := []struct {
		name   string
		extent WriteExtent
		stage  FailureStage
		want   error
	}{
		{"before-reserve", WriteNone, StageReserve, boom},
		{"after-reserve", WriteReservedOrPartial, StageReserve, boom},
		{"before-commit", WriteReservedOrPartial, StageMarker, boom},
		{"after-persist", WriteWrittenUnacknowledged, StagePostPersist, ErrIncomplete},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, path, c, tr := independentFixture(t)
			before := independentFiles(t, path)
			g := s.Snapshot()
			rev := s.disk.revision
			calls := 0
			s.disk.fault = func(point string) error {
				if point == tt.name {
					calls++
					return boom
				}
				return nil
			}
			o := s.SetManualProgressOutcome(c, tr, 51)
			result, session := ResultUncertain, SessionNeedsReopen
			if tt.extent == WriteNone {
				result, session = ResultRejected, SessionWritable
			}
			independentOutcome(t, o.Outcome, result, tt.extent, session, tt.stage, o.Receipt != nil)
			if !errors.Is(o.Err, tt.want) || calls != 1 {
				t.Fatalf("error=%v calls=%d", o.Err, calls)
			}
			if s.disk.revision != rev || !reflect.DeepEqual(g, s.Snapshot()) {
				t.Fatal("failed attempt published")
			}
			s.disk.fault = nil
			if session == SessionWritable {
				independentUnchanged(t, path, before)
				if e := s.AddToday("新显式操作"); e != nil {
					t.Fatal(e)
				}
				return
			}
			failed := independentFiles(t, path)
			if e := s.AddToday("禁止绕行"); !errors.Is(e, ErrIncomplete) {
				t.Fatal(e)
			}
			independentUnchanged(t, path, failed)
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			reopened, e := Open(path, s.Today())
			if tt.name == "after-persist" {
				if e != nil {
					t.Fatal(e)
				}
				defer reopened.Close()
				if reopened.Snapshot().Traces[0].ManualProgressPercent == nil || *reopened.Snapshot().Traces[0].ManualProgressPercent != 51 {
					t.Fatal("reopen lost committed unknown fact")
				}
			} else {
				if e == nil {
					reopened.Close()
					t.Fatal("partial write silently repaired")
				}
			}
			independentUnchanged(t, path, failed)
		})
	}
}
func TestIndependentOSAndReadbackFailure(t *testing.T) {
	cases := []struct {
		name, point, file string
		extent            WriteExtent
		stage             FailureStage
		want              error
	}{
		{"mkdir-race", "before-reserve", "", WriteReservationPossible, StageReserve, ErrConflict},
		{"payload-exclusive", "after-reserve", "state.json", WriteReservedOrPartial, StagePayload, ErrIncomplete},
		{"marker-exclusive", "before-commit", "COMMITTED", WriteReservedOrPartial, StageMarker, ErrIncomplete},
		{"readback-corrupt", "after-persist", "COMMITTED", WriteWrittenUnacknowledged, StageReadback, ErrIncomplete},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, path, c, tr := independentFixture(t)
			g := s.Snapshot()
			rev := s.disk.revision
			calls := 0
			s.disk.fault = func(p string) error {
				if p != tt.point {
					return nil
				}
				calls++
				target := filepath.Join(path, fmt.Sprintf("rev-%010d", rev+1))
				if tt.file == "" {
					return os.Mkdir(target, 0700)
				}
				return os.WriteFile(filepath.Join(target, tt.file), []byte("independent external bytes\n"), 0600)
			}
			o := s.SetManualProgressOutcome(c, tr, 42)
			independentOutcome(t, o.Outcome, ResultUncertain, tt.extent, SessionNeedsReopen, tt.stage, o.Receipt != nil)
			if !errors.Is(o.Err, tt.want) || calls != 1 {
				t.Fatalf("err=%v calls=%d", o.Err, calls)
			}
			if !reflect.DeepEqual(g, s.Snapshot()) || s.disk.revision != rev {
				t.Fatal("published uncertain candidate")
			}
			after := independentFiles(t, path)
			s.disk.fault = nil
			if e := s.SetManualProgress(c, tr, 42); !errors.Is(e, ErrIncomplete) {
				t.Fatal(e)
			}
			independentUnchanged(t, path, after)
		})
	}
}
func TestIndependentPrefixFailureIsNotNoWriteProofForSession(t *testing.T) {
	cases := []struct {
		name      string
		breakDisk func(*testing.T, string)
		want      error
	}{
		{"old-commit-corrupt", func(t *testing.T, p string) { independentCorrupt(t, p) }, ErrCorrupt},
		{"marker-mismatch", func(t *testing.T, p string) {
			if e := os.WriteFile(filepath.Join(p, markerName), []byte("wrong\n"), 0600); e != nil {
				t.Fatal(e)
			}
		}, ErrSession},
		{"prior-incomplete-extra-revision", func(t *testing.T, p string) {
			if e := os.Mkdir(filepath.Join(p, "rev-0000000004"), 0700); e != nil {
				t.Fatal(e)
			}
		}, ErrConflict},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, path, c, tr := independentFixture(t)
			if e := s.SetManualProgress(c, tr, 10); e != nil {
				t.Fatal(e)
			}
			tt.breakDisk(t, path)
			files := independentFiles(t, path)
			// Health 无 scan；尚未观察的磁盘变更不能伪造前次回执。
			if h := s.Health(); h.Session != SessionWritable {
				t.Fatalf("Health unexpectedly probed disk %+v", h)
			}
			o := s.SetManualProgressOutcome(c, tr, 10)
			independentOutcome(t, o.Outcome, ResultRejected, WriteNone, SessionNeedsReopen, StagePrefix, o.Receipt != nil)
			if !errors.Is(o.Err, tt.want) {
				t.Fatalf("first original error=%v want=%v", o.Err, tt.want)
			}
			h := s.Health()
			h.Session = SessionWritable
			if s.Health().Session != SessionNeedsReopen {
				t.Fatal("mutable health alias")
			}
			gate := s.SetManualProgressOutcome("", "", 10)
			independentOutcome(t, gate.Outcome, ResultRejected, WriteNone, SessionNeedsReopen, StageSessionGate, gate.Receipt != nil)
			a := s.AdvanceToOutcome(core.LocalDate{})
			independentOutcome(t, a.Outcome, ResultRejected, WriteNone, SessionNeedsReopen, StageParameter, a.Receipt != nil)
			independentUnchanged(t, path, files)
		})
	}
}
func TestIndependentAdvanceConfirmedBeforeSetterRejection(t *testing.T) {
	s, _, c, tr := independentFixture(t)
	day, e := s.Today().AddDays(1)
	if e != nil {
		t.Fatal(e)
	}
	a := s.AdvanceToOutcome(day)
	independentOutcome(t, a.Outcome, ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, a.Receipt != nil)
	receipt := *a.Receipt
	o := s.SetManualProgressOutcome(c, tr, 20)
	independentOutcome(t, o.Outcome, ResultRejected, WriteNone, SessionWritable, StageDomain, o.Receipt != nil)
	if a.Receipt.Today != day || s.Today() != day || s.disk.revision != a.Receipt.Revision {
		t.Fatal("front stage confirmation lost")
	}
	if e := s.AddToday("新一天任务"); e != nil {
		t.Fatal(e)
	}
	if *a.Receipt != receipt {
		t.Fatal("receipt follows mutable head")
	}
}

func TestIndependentPrefixLimitInvalidatesRatherThanCapacityGuard(t *testing.T) {
	s, path, c, tr := independentFixture(t)
	for i := s.disk.revision + 1; i <= maxRevisions+1; i++ {
		if e := os.Mkdir(filepath.Join(path, fmt.Sprintf("rev-%010d", i)), 0700); e != nil {
			t.Fatal(e)
		}
	}
	before := independentFiles(t, path)
	a := s.AdvanceToOutcome(s.Today())
	independentOutcome(t, a.Outcome, ResultRejected, WriteNone, SessionNeedsReopen, StagePrefix, a.Receipt != nil)
	if !errors.Is(a.Err, ErrLimit) {
		t.Fatal(a.Err)
	}
	if e := s.SetManualProgress(c, tr, 30); !errors.Is(e, ErrIncomplete) {
		t.Fatal(e)
	}
	independentUnchanged(t, path, before)
}
func TestIndependentAdvanceFaultAndCloseSerialization(t *testing.T) {
	s, path, _, _ := independentFixture(t)
	day, e := s.Today().AddDays(1)
	if e != nil {
		t.Fatal(e)
	}
	before := s.Today()
	g := s.Snapshot()
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan AdvanceOutcome, 1)
	closed := make(chan error, 1)
	s.disk.fault = func(point string) error {
		if point == "after-persist" {
			close(entered)
			<-release
			return errors.New("independent")
		}
		return nil
	}
	go func() { finished <- s.AdvanceToOutcome(day) }()
	<-entered
	go func() { closed <- s.Close() }()
	close(release)
	a := <-finished
	independentOutcome(t, a.Outcome, ResultUncertain, WriteWrittenUnacknowledged, SessionNeedsReopen, StagePostPersist, a.Receipt != nil)
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if s.Today() != before || !reflect.DeepEqual(s.Snapshot(), g) {
		t.Fatal("unconfirmed advance published")
	}
	if s.Health().Session != SessionClosed {
		t.Fatal("close health")
	}
	files := independentFiles(t, path)
	opened, e := Open(path, day)
	if e != nil {
		t.Fatal(e)
	}
	defer opened.Close()
	if opened.Today() != day || opened.Health().Session != SessionWritable {
		t.Fatal("new lifecycle")
	}
	independentUnchanged(t, path, files)
}

func TestIndependentManualHistoricalFloorAndChildGuard(t *testing.T) {
	s, path, c, tr := independentFixture(t)
	if e := s.SetManualProgress(c, tr, 37); e != nil {
		t.Fatal(e)
	}
	day, e := s.Today().AddDays(1)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AdvanceTo(day); e != nil {
		t.Fatal(e)
	}
	current, e := s.ContinueUnfinishedTrace(c, tr, day)
	if e != nil {
		t.Fatal(e)
	}
	o := s.SetManualProgressOutcome(c, current, 80)
	independentOutcome(t, o.Outcome, ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, o.Receipt != nil)
	o = s.SetManualProgressOutcome(c, current, 0)
	independentOutcome(t, o.Outcome, ResultConfirmed, WriteCommittedConfirmed, SessionWritable, StageNone, o.Receipt != nil)
	if o.Receipt.Percent != 37 {
		t.Fatalf("historical floor=%d", o.Receipt.Percent)
	}
	if _, e := s.AddSubtask(c, current, "真实子项", core.DifficultySimple); e != nil {
		t.Fatal(e)
	}
	before := independentFiles(t, path)
	o = s.SetManualProgressOutcome(c, current, 50)
	independentOutcome(t, o.Outcome, ResultRejected, WriteNone, SessionWritable, StageDomain, o.Receipt != nil)
	if !errors.Is(o.Err, core.ErrInvalidTransition) {
		t.Fatal(o.Err)
	}
	independentUnchanged(t, path, before)
	if e := s.AddToday("无关合法任务"); e != nil {
		t.Fatal(e)
	}
}
