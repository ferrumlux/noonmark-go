// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func titleID(n int) string           { return fmt.Sprintf("00000000-0000-4000-8000-%012X", n) }
func titleAt(n float64) core.Instant { at, _ := core.InstantFromSeconds(n); return at }
func titlePtr[T any](v T) *T         { return &v }

// Entirely synthetic future-lifecycle fixture. The current UI does not create
// a completed historical trace plus a new same-chain pending continuation.
func titleHistoryFixture(t *testing.T) (canonical.Value, core.TaskGraph, titleCommand) {
	t.Helper()
	yesterday, _ := testDay.AddDays(-1)
	chain := core.TaskChainID(titleID(1))
	definition := core.TaskDefinitionID(titleID(2))
	oldTrace := core.DayTraceID(titleID(4))
	g := core.TaskGraph{
		Days:        []core.Day{{ID: core.DayID(titleID(3)), Date: yesterday, CreatedAt: titleAt(1), UpdatedAt: titleAt(3)}, {ID: core.DayID(titleID(5)), Date: testDay, CreatedAt: titleAt(4), UpdatedAt: titleAt(4)}},
		Chains:      []core.TaskChain{{ID: chain, State: core.ChainActive, CreatedAt: titleAt(1), UpdatedAt: titleAt(4)}},
		Definitions: []core.TaskDefinition{{ID: definition, ChainID: chain, Sequence: 1, Title: "历史标题", DescriptionText: titlePtr("历史详情"), PlannedSubtasks: []core.PlannedSubtask{{ID: core.PlannedSubtaskID(titleID(7)), LineageID: core.SubtaskLineageID(titleID(8)), Title: "计划子任务", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: titleAt(1)}}, CreatedAt: titleAt(1), ContentUpdatedAt: titleAt(1)}},
		Traces:      []core.DayTrace{{ID: oldTrace, ChainID: chain, DefinitionID: definition, Date: yesterday, Status: core.TraceCompleted, Priority: 1, DescriptionText: titlePtr("当日详情"), CreatedAt: titleAt(1), ContentUpdatedAt: titleAt(3), CompletedAt: titlePtr(titleAt(3))}, {ID: core.DayTraceID(titleID(6)), ChainID: chain, DefinitionID: definition, Date: testDay, Status: core.TracePending, Priority: 1, CreatedAt: titleAt(4), ContentUpdatedAt: titleAt(4)}},
		Subtasks:    []core.Subtask{{ID: core.SubtaskID(titleID(9)), LineageID: core.SubtaskLineageID(titleID(8)), TraceID: oldTrace, Title: "历史子任务", Status: core.SubtaskCompleted, Difficulty: core.DifficultySimple, Position: 1, CreatedAt: titleAt(1), UpdatedAt: titleAt(3), CompletedAt: titlePtr(titleAt(3))}},
	}
	k, e := core.NewTaskKernel(g)
	must(t, e)
	extra, e := emptySupplement()
	must(t, e)
	prior, e := v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	must(t, k.SettleTaskDays(testDay, titleAt(5)))
	settled := k.Snapshot()
	prior, e = CaptureEmptyHistory(prior, g, settled)
	must(t, e)
	must(t, ValidateEmptyHistory(prior))
	_, _, e = normalizeSnapshot(prior)
	must(t, e)
	return prior, settled, titleCommand{titleInput, core.EditTaskTitleInput{ChainID: chain, Title: "新标题", Today: testDay, Now: titleAt(10), NewDefinitionID: core.TaskDefinitionID(titleID(20))}}
}
func titleCandidate(t *testing.T, g core.TaskGraph, c titleCommand) core.TaskGraph {
	t.Helper()
	k, e := core.NewTaskKernel(g)
	must(t, e)
	must(t, c.apply(k))
	return k.Snapshot()
}
func TestTitleAuditHistoricalForkReuseAndStrictCaptureUnchanged(t *testing.T) {
	prior, before, command := titleHistoryFixture(t)
	after := titleCandidate(t, before, command)
	if _, e := CaptureEmptyHistory(prior, before, after); e == nil {
		t.Fatal("generic capture was loosened")
	}
	result, e := captureTitleHistory(prior, before, after, command)
	must(t, e)
	if len(after.Definitions) != 2 || !historyEqual(prior.Object["classifications"], result.Object["classifications"]) {
		t.Fatal("fork/audit")
	}
	command.input.Title = "再次输入"
	command.input.Now = titleAt(11)
	command.input.NewDefinitionID = core.TaskDefinitionID(titleID(21))
	second := titleCandidate(t, after, command)
	next, e := captureTitleHistory(result, after, second, command)
	must(t, e)
	if len(second.Definitions) != 2 || !reflect.DeepEqual(before.Traces[0], second.Traces[0]) || !reflect.DeepEqual(before.Subtasks, second.Subtasks) || second.Definitions[0].Title != "历史标题" {
		t.Fatal("history or reuse changed")
	}
	if !historyEqual(next.Object["classifications"], prior.Object["classifications"]) {
		t.Fatal("audit changed")
	}
}
func TestTitleAuditRejectsTamperedCandidateWithoutMutatingInputs(t *testing.T) {
	prior, before, command := titleHistoryFixture(t)
	valid := titleCandidate(t, before, command)
	attacks := map[string]func(*core.TaskGraph){
		"old title":       func(g *core.TaskGraph) { g.Definitions[0].Title = "偷改" },
		"old description": func(g *core.TaskGraph) { *g.Definitions[0].DescriptionText = "偷改" },
		"old plan":        func(g *core.TaskGraph) { g.Definitions[0].PlannedSubtasks[0].Title = "偷改" },
		"old created":     func(g *core.TaskGraph) { g.Definitions[0].CreatedAt = titleAt(0) },
		"old sequence":    func(g *core.TaskGraph) { g.Definitions[0].Sequence = 7 },
		"old trace":       func(g *core.TaskGraph) { *g.Traces[0].DescriptionText = "偷改" },
		"old subtask":     func(g *core.TaskGraph) { g.Subtasks[0].Title = "偷改" },
		"day review":      func(g *core.TaskGraph) { g.Days[0].ReviewSummary = titlePtr("偷改") },
		"day lock":        func(g *core.TaskGraph) { g.Days[0].LockedAt = titlePtr(titleAt(6)) },
		"missing trace":   func(g *core.TaskGraph) { g.Traces = g.Traces[1:] },
		"extra trace": func(g *core.TaskGraph) {
			x := g.Traces[0]
			x.ID = core.DayTraceID(titleID(90))
			x.Priority = 9
			g.Traces = append(g.Traces, x)
		},
		"extra subtask": func(g *core.TaskGraph) {
			x := g.Subtasks[0]
			x.ID = core.SubtaskID(titleID(91))
			x.LineageID = core.SubtaskLineageID(titleID(92))
			x.Position = 2
			g.Subtasks = append(g.Subtasks, x)
		},
		"successor sequence":      func(g *core.TaskGraph) { g.Definitions[1].Sequence = 3 },
		"successor chain":         func(g *core.TaskGraph) { g.Definitions[1].ChainID = core.TaskChainID(titleID(99)) },
		"successor copied detail": func(g *core.TaskGraph) { *g.Definitions[1].DescriptionText = "wrong" },
		"successor copied plan":   func(g *core.TaskGraph) { g.Definitions[1].PlannedSubtasks[0].Title = "wrong" },
		"successor clock":         func(g *core.TaskGraph) { g.Definitions[1].CreatedAt = titleAt(9) },
		"old pointer": func(g *core.TaskGraph) {
			g.Definitions[0].SupersededByDefinitionID = titlePtr(core.TaskDefinitionID(titleID(99)))
		},
		"old metadata clock": func(g *core.TaskGraph) { g.Definitions[0].ContentUpdatedAt = titleAt(11) },
		"extra definition": func(g *core.TaskGraph) {
			x := g.Definitions[0]
			x.ID = core.TaskDefinitionID(titleID(93))
			x.Sequence = 8
			g.Definitions = append(g.Definitions, x)
		},
	}
	for name, attack := range attacks {
		t.Run(name, func(t *testing.T) {
			candidate := valid.Clone()
			attack(&candidate)
			saved := candidate.Clone()
			old := before.Clone()
			wire, e := canonical.Marshal(prior)
			must(t, e)
			if _, e = captureTitleHistory(prior, before, candidate, command); e == nil {
				t.Fatal("tamper accepted")
			}
			got, e := canonical.Marshal(prior)
			must(t, e)
			if !reflect.DeepEqual(saved, candidate) || !reflect.DeepEqual(old, before) || string(wire) != string(got) {
				t.Fatal("input mutated")
			}
		})
	}
}
func TestTitleAuditRejectsWrongEvidenceAndMissingAudit(t *testing.T) {
	prior, before, c := titleHistoryFixture(t)
	after := titleCandidate(t, before, c)
	for _, name := range []string{"mode", "clock", "ID", "existing ID", "wrong title", "wrong before", "audit revision", "missing audit", "audit event"} {
		t.Run(name, func(t *testing.T) {
			p, e := inspectEmptyHistory(prior)
			must(t, e)
			b := before.Clone()
			cmd := c
			switch name {
			case "mode":
				cmd.mode = 99
			case "clock":
				cmd.input.Now = titleAt(0)
			case "ID":
				cmd.input.NewDefinitionID = "invalid"
			case "existing ID":
				cmd.input.NewDefinitionID = b.Definitions[0].ID
			case "wrong title":
				cmd.input.Title = "different"
			case "wrong before":
				b.Definitions[0].Title = "different"
			case "audit revision":
				p.Object["classifications"].Object["revision"] = historyNumber(9)
			case "missing audit":
				extra, e := emptySupplement()
				must(t, e)
				p.Object["classifications"] = extra.Classifications
			case "audit event":
				p.Object["classifications"].Object["snapshotEventsByTraceID"].Array[1].Array[0].Object["status"] = historyString("unfinished")
			}
			if _, e = captureTitleHistory(p, b, after, cmd); e == nil {
				t.Fatal("bad command evidence accepted")
			}
		})
	}
}
func TestTitleServicePoolPendingNoopAndReopen(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) {
			s, path := fresh(t)
			if pending {
				must(t, s.AddToday("旧标题"))
			} else {
				must(t, s.AddPool("旧标题"))
			}
			chain := s.graph.Chains[0].ID
			must(t, s.SaveTaskTitleInput(chain, "  新\r\n标题  "))
			if len(s.graph.Definitions) != 1 || s.graph.Definitions[0].Title != "新\n标题" {
				t.Fatal("input failed")
			}
			revision := s.disk.revision
			must(t, s.SaveTaskTitleInput(chain, "新\n标题"))
			if s.disk.revision != revision {
				t.Fatal("noop wrote revision")
			}
			must(t, s.RenameTaskTitle(chain, "显式重命名"))
			want := 1
			if pending {
				want = 2
			}
			if len(s.graph.Definitions) != want {
				t.Fatal("rename version count")
			}
			must(t, s.SaveTaskTitleInput(chain, "   "))
			saved := s.Snapshot()
			rev := s.disk.revision
			must(t, s.Close())
			r, e := Open(path, testDay)
			must(t, e)
			defer r.Close()
			if !reflect.DeepEqual(saved, r.Snapshot()) || r.disk.revision != rev {
				t.Fatal("reopen changed title")
			}
		})
	}
}
func TestTitleServiceHistoricalRoundTrip(t *testing.T) {
	s, path := fresh(t)
	prior, g, _ := titleHistoryFixture(t)
	must(t, s.disk.commit(prior))
	s.graph = g
	s.snapshot = prior
	must(t, s.SaveTaskTitleInput(g.Chains[0].ID, "本次编辑"))
	must(t, s.SaveTaskTitleInput(g.Chains[0].ID, "后续编辑"))
	if len(s.graph.Definitions) != 2 || s.graph.Definitions[0].Title != g.Definitions[0].Title {
		t.Fatal("history overwritten")
	}
	saved := s.Snapshot()
	wire := s.snapshot
	must(t, s.Close())
	r, e := Open(path, testDay)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) || !historyEqual(wire, r.snapshot) || !historyEqual(prior.Object["classifications"], r.snapshot.Object["classifications"]) {
		t.Fatal("historical round trip")
	}
}
func TestTitleServiceFaultMatrix(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(phase, func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("old"))
			before := s.Snapshot()
			wire := s.snapshot
			rev := s.disk.revision
			chain := s.graph.Chains[0].ID
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			if e := s.RenameTaskTitle(chain, "new"); e == nil {
				t.Fatal("fault succeeded")
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) || s.disk.revision != rev {
				t.Fatal("failed publication")
			}
			s.disk.fault = nil
			if phase != "before-reserve" {
				if e := s.SaveTaskTitleInput(chain, "old"); !errors.Is(e, ErrIncomplete) {
					t.Fatal("uncertain no-op accepted", e)
				}
			}
			must(t, s.Close())
			r, e := Open(path, testDay)
			switch phase {
			case "before-reserve":
				must(t, e)
				defer r.Close()
				if !reflect.DeepEqual(before, r.Snapshot()) {
					t.Fatal("early failure changed disk")
				}
			case "after-persist":
				must(t, e)
				defer r.Close()
				if r.disk.revision != rev+1 || len(r.graph.Definitions) != 2 || r.graph.Definitions[1].Title != "new" {
					t.Fatal("durable uncertain commit absent")
				}
			default:
				if !errors.Is(e, ErrIncomplete) {
					if r != nil {
						r.Close()
					}
					t.Fatal("partial revision reopened", e)
				}
			}
		})
	}
}
func TestTitleServiceStaleAndClosed(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("old"))
	other, e := Open(path, testDay)
	must(t, e)
	defer other.Close()
	before := other.Snapshot()
	wire := other.snapshot
	chain := s.graph.Chains[0].ID
	must(t, s.SaveTaskTitleInput(chain, "winner"))
	for _, title := range []string{"loser", "old"} {
		if e = other.SaveTaskTitleInput(chain, title); !errors.Is(e, ErrConflict) {
			t.Fatal("stale accepted", e)
		}
	}
	if !reflect.DeepEqual(before, other.Snapshot()) || !historyEqual(wire, other.snapshot) {
		t.Fatal("stale published")
	}
	must(t, other.Close())
	if e = other.RenameTaskTitle(chain, "old"); !errors.Is(e, ErrSession) {
		t.Fatal(e)
	}
}
func TestTitleServiceConcurrentSavesAndNaturalDay(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("old"))
	chain := s.graph.Chains[0].ID
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs <- s.SaveTaskTitleInput(chain, fmt.Sprint(i)) }(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	if len(s.Snapshot().Definitions) != 1 {
		t.Fatal("autosave inflated versions")
	}
	next, _ := testDay.AddDays(1)
	start := make(chan struct{})
	out := make(chan error, 2)
	go func() { <-start; out <- s.AdvanceTo(next) }()
	go func() { <-start; out <- s.SaveTaskTitleInput(chain, "boundary") }()
	close(start)
	for i := 0; i < 2; i++ {
		e := <-out
		if e != nil && !errors.Is(e, core.ErrInvalidTransition) && !errors.Is(e, core.ErrImmutableHistory) {
			t.Fatal(e)
		}
	}
	must(t, ValidateEmptyHistory(s.snapshot))
	before := s.Snapshot()
	if e := s.SaveTaskTitleInput(chain, "late"); e == nil {
		t.Fatal("past task accepted")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("past edit published")
	}
}
func TestTitleServiceUsesWholeClockFrontier(t *testing.T) {
	s, _ := fresh(t)
	prior, g, _ := titleHistoryFixture(t)
	g.Chains[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(titleID(66)), Body: "future clock", CreatedAt: titleAt(80), UpdatedAt: titleAt(90)}}
	g.Chains[0].UpdatedAt = titleAt(90)
	_, extra, e := v7.ToDomain(prior)
	must(t, e)
	prior, e = v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	must(t, s.disk.commit(prior))
	s.graph = g
	s.snapshot = prior
	s.clock = func() time.Time { return time.Unix(978307201, 0) }
	must(t, s.SaveTaskTitleInput(g.Chains[0].ID, "new"))
	if !s.graph.Definitions[1].CreatedAt.After(titleAt(90)) {
		t.Fatal("frontier omitted")
	}
}

func TestTitleAuditRestorationWitnessAndAlreadySupersededFreeze(t *testing.T) {
	prior, g, c := titleHistoryFixture(t)
	old := g.Clone().Definitions[0]
	old.ID = core.TaskDefinitionID(titleID(70))
	old.Sequence = 7
	old.SupersededAt = titlePtr(titleAt(4))
	old.ContentUpdatedAt = titleAt(4)
	old.SupersededByDefinitionID = titlePtr(g.Definitions[0].ID)
	g.Definitions[0].SupersededByDefinitionID = titlePtr(old.ID)
	g.Definitions = append(g.Definitions, old)
	// The fixture also exercises absent optional content on both version sides.
	for i := range g.Definitions {
		g.Definitions[i].DescriptionText = nil
	}
	_, extra, e := v7.ToDomain(prior)
	must(t, e)
	prior, e = v7.FromDomain(recurrence.State{Tasks: g}, extra)
	must(t, e)
	must(t, ValidateEmptyHistory(prior))
	after := titleCandidate(t, g, c)
	result, e := captureTitleHistory(prior, g, after, c)
	must(t, e)
	if after.Definitions[2].Sequence != 8 || !reflect.DeepEqual(g.Definitions[1], after.Definitions[1]) {
		t.Fatal("restoration witness lost or old supersession rewritten")
	}
	for _, field := range []string{"clock", "pointer"} {
		t.Run(field, func(t *testing.T) {
			bad := after.Clone()
			if field == "clock" {
				bad.Definitions[1].ContentUpdatedAt = titleAt(5)
			} else {
				bad.Definitions[1].SupersededByDefinitionID = titlePtr(c.input.NewDefinitionID)
			}
			if _, e := captureTitleHistory(prior, g, bad, c); e == nil {
				t.Fatal("existing supersession rewritten")
			}
		})
	}
	must(t, ValidateEmptyHistory(result))
}
func TestTitleRuntimeScopeStaysClosed(t *testing.T) {
	prior, g, c := titleHistoryFixture(t)
	for _, name := range []string{"returnedToPool", "cancelledDraft", "category", "recurring", "preferences"} {
		t.Run(name, func(t *testing.T) {
			p, e := inspectEmptyHistory(prior)
			must(t, e)
			candidate := g.Clone()
			switch name {
			case "returnedToPool":
				candidate.Traces[1].Status = core.TraceReturnedToPool
				candidate.Traces[1].SettledAt = titlePtr(titleAt(4))
				_, extra, e := v7.ToDomain(p)
				must(t, e)
				p, e = v7.FromDomain(recurrence.State{Tasks: candidate}, extra)
				must(t, e)
			case "cancelledDraft":
				candidate.Traces[1].Status = core.TraceCancelledDraft
				candidate.Traces[1].SettledAt = titlePtr(titleAt(4))
				candidate.Traces[1].DraftCancellationID = titlePtr(core.CancellationID(titleID(80)))
				candidate.Traces[1].DraftCancelledOn = titlePtr(testDay)
				_, extra, e := v7.ToDomain(p)
				must(t, e)
				p, e = v7.FromDomain(recurrence.State{Tasks: candidate}, extra)
				must(t, e)
			case "category":
				id := historyObject(map[string]canonical.Value{"rawValue": historyString(titleID(81))})
				item := historyObject(map[string]canonical.Value{"id": id, "name": historyString("Synthetic"), "canonicalKey": historyString("synthetic"), "canonicalKeyVersion": historyString("synthetic-test"), "colorHex": historyString("#123456"), "lifecycle": historyString("active"), "nameVersions": {Kind: canonical.Array}, "createdAt": historyNumber(0), "updatedAt": historyNumber(0), "presentationApproval": historyString("userApproved")})
				p.Object["classifications"].Object["categories"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{id, item}}
				_, e = inspectEmptyHistory(p)
				must(t, e)
			case "recurring":
				n := 100
				state, _, e := (recurrence.State{Tasks: candidate}).Create(recurrence.CreateInput{Title: "合成周期", StartDate: testDay, Schedule: recurrence.Daily(), EndCondition: recurrence.Until(testDay), Today: testDay, Now: titleAt(10)}, func() (string, error) { n++; return titleID(n), nil })
				must(t, e)
				_, extra, e := v7.ToDomain(p)
				must(t, e)
				p, e = v7.FromDomain(state, extra)
				must(t, e)
				_, e = inspectEmptyHistory(p)
				must(t, e)
			case "preferences":
				p.Object["preferences"].Object["language"] = historyString("english")
			}
			if name != "preferences" && ValidateEmptyHistory(p) == nil {
				t.Fatal("scope accepted by history")
			}
			if _, _, e = normalizeSnapshot(p); e == nil {
				t.Fatal("scope accepted by runtime")
			}
			if name != "preferences" {
				if _, e = captureTitleHistory(p, candidate, candidate, c); e == nil {
					t.Fatal("scope accepted by title audit")
				}
			}
		})
	}
}

func TestTitleServiceCanonicalEquivalentNoop(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprint(rename), func(t *testing.T) {
			s, path := fresh(t)
			must(t, s.AddToday("café"))
			chain := s.graph.Chains[0].ID
			before := s.Snapshot()
			wire := s.snapshot
			rev := s.disk.revision
			edit := s.SaveTaskTitleInput
			if rename {
				edit = s.RenameTaskTitle
			}
			must(t, edit(chain, "cafe\u0301"))
			if s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) {
				t.Fatal("canonical-equivalent edit changed bytes or revision")
			}
			s.disk.blocked = true
			if e := edit(chain, "cafe\u0301"); !errors.Is(e, ErrIncomplete) {
				t.Fatal("blocked equivalent noop accepted", e)
			}
			s.disk.blocked = false
			other, e := Open(path, testDay)
			must(t, e)
			defer other.Close()
			must(t, other.SaveTaskTitleInput(chain, "winner"))
			if e = edit(chain, "cafe\u0301"); !errors.Is(e, ErrConflict) {
				t.Fatal("stale equivalent noop accepted", e)
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(wire, s.snapshot) {
				t.Fatal("stale noop published")
			}
		})
	}
}

func TestTitleServiceAuditAndNormalizationFailureBeforeReservation(t *testing.T) {
	for _, name := range []string{"missing audit", "scope", "graph mismatch"} {
		t.Run(name, func(t *testing.T) {
			s, _ := fresh(t)
			prior, g, _ := titleHistoryFixture(t)
			must(t, s.disk.commit(prior))
			s.graph = g
			s.snapshot = prior
			switch name {
			case "missing audit":
				extra, e := emptySupplement()
				must(t, e)
				s.snapshot.Object["classifications"] = extra.Classifications
			case "scope":
				s.snapshot.Object["preferences"].Object["language"] = historyString("english")
			case "graph mismatch":
				s.graph.Definitions[0].Title = "unsaved mutation"
			}
			before := s.Snapshot()
			wire, e := canonical.Marshal(s.snapshot)
			must(t, e)
			rev := s.disk.revision
			reserved := false
			s.disk.fault = func(string) error { reserved = true; return nil }
			if e = s.SaveTaskTitleInput(g.Chains[0].ID, "new"); e == nil {
				t.Fatal("invalid aggregate accepted")
			}
			after, e := canonical.Marshal(s.snapshot)
			must(t, e)
			if reserved || s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) || string(wire) != string(after) {
				t.Fatal("failure changed aggregate or reached disk reservation")
			}
		})
	}
}

func TestTitleServiceConcurrentHandlesPublishOnlyWinner(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("old"))
	other, e := Open(path, testDay)
	must(t, e)
	defer other.Close()
	before := s.Snapshot()
	chain := before.Chains[0].ID
	type outcome struct {
		s   *Service
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for i, handle := range []*Service{s, other} {
		go func(i int, h *Service) {
			<-start
			results <- outcome{h, h.RenameTaskTitle(chain, fmt.Sprintf("winner %d", i))}
		}(i, handle)
	}
	close(start)
	winners := 0
	var saved core.TaskGraph
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			winners++
			saved = result.s.Snapshot()
		} else {
			if !errors.Is(result.err, ErrConflict) {
				t.Fatal(result.err)
			}
			if !reflect.DeepEqual(before, result.s.Snapshot()) {
				t.Fatal("losing handle published")
			}
		}
	}
	if winners != 1 {
		t.Fatal("expected exactly one winner", winners)
	}
	r, e := Open(path, testDay)
	must(t, e)
	defer r.Close()
	if !reflect.DeepEqual(saved, r.Snapshot()) {
		t.Fatal("disk differs from winner")
	}
}
