package taskapp

import (
	"errors"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"reflect"
	"strings"
	"testing"
)

func acID(n int) string           { return fmt.Sprintf("10000000-0000-4000-8000-%012X", n) }
func acAt(n float64) core.Instant { return core.Instant(math.Float64bits(n)) }
func acMust(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

var acDay = core.LocalDate{Year: 2026, Month: 11, Day: 6}

func acWire(t *testing.T, g core.TaskGraph, c *canonical.Value) canonical.Value {
	t.Helper()
	x, e := emptySupplement()
	acMust(t, e)
	if c != nil {
		x.Classifications = *c
	}
	s, e := v7.FromDomain(recurrence.State{Tasks: g}, x)
	acMust(t, e)
	return s
}
func acSeal(t *testing.T, g core.TaskGraph, c *canonical.Value) validatedHistorySnapshot {
	t.Helper()
	s, e := inspectForwardClockOrdinaryHistoryVNext(acWire(t, g, c))
	acMust(t, e)
	return s
}
func acPool(t *testing.T) *core.TaskKernel {
	t.Helper()
	k, e := core.NewTaskKernel(core.TaskGraph{})
	acMust(t, e)
	desc := "owned description"
	acMust(t, k.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(acID(1)), DefinitionID: core.TaskDefinitionID(acID(2)), Title: "Parent", DescriptionText: &desc, Now: acAt(0)}))
	for i := 0; i < 3; i++ {
		acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput{Kind: core.SubtaskAdd, ChainID: core.TaskChainID(acID(1)), ExpectedDefinitionID: core.TaskDefinitionID(acID(2)), SubtaskID: core.PlannedSubtaskID(acID(10 + i)), LineageID: core.SubtaskLineageID(acID(20 + i)), Title: []string{"café", "Middle", "Last"}[i], Difficulty: core.DifficultySimple, Now: acAt(1)}))
	}
	return k
}
func acActual(t *testing.T) *core.TaskKernel {
	t.Helper()
	k := acPool(t)
	acMust(t, k.ScheduleFromPool(core.ScheduleFromPoolInput{ChainID: core.TaskChainID(acID(1)), TraceID: core.DayTraceID(acID(3)), DayID: core.DayID(acID(4)), Date: acDay, Today: acDay, Now: acAt(2), SubtaskIDs: map[core.PlannedSubtaskID]core.SubtaskID{core.PlannedSubtaskID(acID(10)): core.SubtaskID(acID(30)), core.PlannedSubtaskID(acID(11)): core.SubtaskID(acID(31)), core.PlannedSubtaskID(acID(12)): core.SubtaskID(acID(32))}}))
	return k
}
func acPI(kind core.SubtaskEditKind) plannedSubtaskHistoryInput {
	return plannedSubtaskHistoryInput{Kind: kind, ChainID: core.TaskChainID(acID(1)), ExpectedDefinitionID: core.TaskDefinitionID(acID(2)), SubtaskID: core.PlannedSubtaskID(acID(11)), Now: acAt(9)}
}
func acAI(kind core.SubtaskEditKind) actualSubtaskHistoryInput {
	return actualSubtaskHistoryInput{Kind: kind, ChainID: core.TaskChainID(acID(1)), TraceID: core.DayTraceID(acID(3)), SubtaskID: core.SubtaskID(acID(31)), Today: acDay, Now: acAt(9)}
}
func acZero(t *testing.T, r auditedHistoryCommand, e, want error) {
	t.Helper()
	if !errors.Is(e, want) || r.proof != nil {
		t.Fatalf("got proof=%v error=%v want=%v", r.proof, e, want)
	}
}
func TestAcceptancePlannedConstants(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
		t.Run(string(kind), func(t *testing.T) {
			k := acPool(t)
			before := k.Snapshot()
			in := acPI(kind)
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.PlannedSubtaskID(acID(50))
				in.LineageID = core.SubtaskLineageID(acID(51))
				in.Difficulty = core.DifficultySimple
				in.Title = " \t新\r\n标题\r尾 \n"
			case core.SubtaskTitle:
				in.Title = " \t新\r\n标题\r尾 \n"
			case core.SubtaskDifficulty:
				in.Difficulty = core.Difficulty(2)
			}
			prior := acSeal(t, before, nil)
			acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
			after := k.Snapshot()
			r, e := auditPlannedSubtaskHistoryVNext(prior, before, after, in)
			acMust(t, e)
			if !r.valid() || !r.proof.changed {
				t.Fatal("missing changed proof")
			}
			want := before.Clone()
			want.Chains[0].UpdatedAt = acAt(9)
			d := &want.Definitions[0]
			d.ContentUpdatedAt = acAt(9)
			switch kind {
			case core.SubtaskAdd:
				s := after.Definitions[0].PlannedSubtasks[3]
				if string(s.ID) != acID(50) || string(s.LineageID) != acID(51) || s.Title != "新\n标题\n尾" || s.Position != 4 || s.CreatedAt != acAt(9) || s.Difficulty != core.DifficultySimple {
					t.Fatal(s)
				}
				d.PlannedSubtasks = append(d.PlannedSubtasks, s)
			case core.SubtaskTitle:
				d.PlannedSubtasks[1].Title = "新\n标题\n尾"
			case core.SubtaskDifficulty:
				d.PlannedSubtasks[1].Difficulty = 2
			case core.PlannedSubtaskRemove:
				d.PlannedSubtasks = append(d.PlannedSubtasks[:1], d.PlannedSubtasks[2:]...)
				d.PlannedSubtasks[1].Position = 2
			}
			if !reflect.DeepEqual(want, after) {
				t.Fatal("non-authorized domain delta")
			}
			if !historyPlannedSubtaskFence(prior, r.proof.after, in) {
				t.Fatal("positive direct fence")
			}
			bad := after.Clone()
			bad.Definitions[0].Title = "unrelated"
			bs := acSeal(t, bad, nil)
			if historyPlannedSubtaskFence(prior, bs, in) {
				t.Fatal("direct fence allowed parent title")
			}
			before.Definitions[0].PlannedSubtasks[0].Title = "poison"
			*after.Definitions[0].DescriptionText = "poison"
			copy1, e := r.snapshotCopy()
			acMust(t, e)
			copy1.Object["definitions"].Array[0].Object["title"] = historyString("poison")
			copy2, e := r.snapshotCopy()
			acMust(t, e)
			if copy2.Object["definitions"].Array[0].Object["title"].String != "Parent" || !historyEqual(copy2, acWire(t, want, nil)) {
				t.Fatal("nested ownership leak")
			}
		})
	}
}
func TestAcceptanceActualSevenWithHistory(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
		t.Run(string(kind), func(t *testing.T) {
			k := acActual(t)
			s := acSeal(t, k.Snapshot(), nil)
			for j := 0; j < 2; j++ {
				before := k.Snapshot()
				ai := abandonHistoryInput{ChainID: core.TaskChainID(acID(1)), TraceID: core.DayTraceID(acID(3)), Now: acAt(float64(3 + j*2)), EventID: acID(80 + j)}
				acMust(t, k.AbandonChain(core.AbandonChainInput{ChainID: ai.ChainID, TraceID: ai.TraceID, Now: ai.Now}))
				r, e := auditAbandonHistoryVNext(s, before, k.Snapshot(), ai)
				acMust(t, e)
				s = r.proof.after
				before = k.Snapshot()
				ri := reactivateHistoryInput{ChainID: ai.ChainID, TraceID: ai.TraceID, Today: acDay, Now: acAt(float64(4 + j*2))}
				_, e = k.ReactivateAbandonedChain(core.ReactivateAbandonedChainInput{ChainID: ri.ChainID, TraceID: ri.TraceID, Today: ri.Today, Now: ri.Now})
				acMust(t, e)
				r, e = auditReactivateHistoryVNext(s, before, k.Snapshot(), ri)
				acMust(t, e)
				s = r.proof.after
			}
			in := acAI(kind)
			switch kind {
			case core.SubtaskAdd:
				in.SubtaskID = core.SubtaskID(acID(50))
				in.LineageID = core.SubtaskLineageID(acID(51))
				in.Title = " Added\r\nchild "
				in.Difficulty = core.DifficultySimple
			case core.SubtaskTitle:
				in.Title = " Changed\rchild "
			case core.SubtaskDifficulty:
				in.Difficulty = 2
			case core.SubtaskDelete:
				in.CancellationID = core.CancellationID(acID(99))
			case core.SubtaskUndoComplete:
				pre := acAI(core.SubtaskComplete)
				pre.Now = acAt(7)
				acMust(t, k.EditSubtask(core.SubtaskEditInput(pre)))
				c := s.proof.snapshot.Object["classifications"]
				s = acSeal(t, k.Snapshot(), &c)
			}
			before := k.Snapshot()
			acMust(t, k.EditSubtask(core.SubtaskEditInput(in)))
			after := k.Snapshot()
			r, e := auditActualSubtaskHistoryVNext(s, before, after, in)
			acMust(t, e)
			if !r.valid() || !r.proof.changed || r.proof.after.proof.count != 2 || !historyEqual(s.proof.snapshot.Object["classifications"], r.proof.after.proof.snapshot.Object["classifications"]) {
				t.Fatal("history lost")
			}
			want := before.Clone()
			if kind == core.SubtaskAdd {
				v := after.Subtasks[3]
				if v.Title != "Added\nchild" || v.Position != 4 || v.Status != core.SubtaskPending || v.CreatedAt != acAt(9) || v.UpdatedAt != acAt(9) || v.CompletedAt != nil || v.SettledAt != nil || v.CarriedFromSubtaskID != nil || v.DraftCancellationID != nil || string(v.ID) != acID(50) || string(v.LineageID) != acID(51) {
					t.Fatal(v)
				}
				want.Subtasks = append(want.Subtasks, v)
			} else {
				v := &want.Subtasks[1]
				v.UpdatedAt = acAt(9)
				at := acAt(9)
				switch kind {
				case core.SubtaskTitle:
					v.Title = "Changed\nchild"
				case core.SubtaskDifficulty:
					v.Difficulty = 2
				case core.SubtaskDelete:
					v.Status = core.SubtaskCancelledDraft
					v.CompletedAt = nil
					v.SettledAt = &at
					v.DraftCancellationID = &in.CancellationID
				case core.SubtaskComplete:
					v.Status = core.SubtaskCompleted
					v.CompletedAt = &at
				case core.SubtaskUndoComplete:
					v.Status = core.SubtaskPending
					v.CompletedAt = nil
				case core.SubtaskAbandon:
					v.Status = core.SubtaskAbandoned
					v.SettledAt = &at
				}
			}
			if !reflect.DeepEqual(want, after) {
				t.Fatal("wrong exact domain delta")
			}
			bad := after.Clone()
			bad.Chains[0].UpdatedAt = acAt(20)
			c := s.proof.snapshot.Object["classifications"]
			bs := acSeal(t, bad, &c)
			if historyActualSubtaskFence(s, bs, in) {
				t.Fatal("direct fence allowed parent clock")
			}
			before.Subtasks[0].Title = "poison"
			if after.Subtasks[1].CompletedAt != nil {
				*after.Subtasks[1].CompletedAt = acAt(55)
			}
			copy1, e := r.snapshotCopy()
			acMust(t, e)
			copy1.Object["subtasks"].Array[0].Object["title"] = historyString("poison")
			copy2, e := r.snapshotCopy()
			acMust(t, e)
			if !historyEqual(copy2, acWire(t, want, &c)) {
				t.Fatal("actual ownership")
			}
		})
	}
}
func TestAcceptanceNoopAndGuardOrder(t *testing.T) {
	for _, now := range []core.Instant{acAt(-100), acAt(math.NaN()), acAt(math.Inf(1)), acAt(math.Inf(-1)), acAt(math.MaxFloat64)} {
		for _, planned := range []bool{true, false} {
			k := acPool(t)
			if !planned {
				k = acActual(t)
			}
			b := k.Snapshot()
			s := acSeal(t, b, nil)
			var r auditedHistoryCommand
			var e error
			if planned {
				in := acPI(core.SubtaskTitle)
				in.SubtaskID = core.PlannedSubtaskID(acID(10))
				in.Title = " cafe\u0301 "
				in.Now = now
				r, e = auditPlannedSubtaskHistoryVNext(s, b, b, in)
			} else {
				in := acAI(core.SubtaskTitle)
				in.SubtaskID = core.SubtaskID(acID(30))
				in.Title = " cafe\u0301 "
				in.Now = now
				r, e = auditActualSubtaskHistoryVNext(s, b, b, in)
			}
			acMust(t, e)
			if r.proof.changed || r.proof.now != now || !historyEqual(s.proof.snapshot, r.proof.after.proof.snapshot) {
				t.Fatal("no-op lost bits/bytes")
			}
		}
	}
	k := acActual(t)
	b := k.Snapshot()
	s := acSeal(t, b, nil)
	cases := []struct {
		name   string
		mutate func(*actualSubtaskHistoryInput)
		want   error
	}{{"wrong-owner-before-date", func(i *actualSubtaskHistoryInput) {
		i.ChainID = core.TaskChainID(acID(900))
		i.Today, _ = acDay.AddDays(1)
	}, core.ErrInvalidInput}, {"past-before-noop", func(i *actualSubtaskHistoryInput) { i.Today, _ = acDay.AddDays(1) }, core.ErrImmutableHistory}, {"shape-before-owner", func(i *actualSubtaskHistoryInput) {
		i.LineageID = core.SubtaskLineageID(acID(77))
		i.TraceID = core.DayTraceID(acID(900))
	}, core.ErrInvalidInput}, {"missing-trace-before-clock", func(i *actualSubtaskHistoryInput) { i.TraceID = core.DayTraceID(acID(900)) }, core.ErrNotFound}, {"bad-title-before-owner", func(i *actualSubtaskHistoryInput) { i.Title = "\xff"; i.TraceID = core.DayTraceID(acID(900)) }, core.ErrInvalidTitle}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := acAI(core.SubtaskTitle)
			in.Title = "Middle"
			in.Now = acAt(math.NaN())
			tc.mutate(&in)
			r, e := auditActualSubtaskHistoryVNext(s, b, b, in)
			acZero(t, r, e, tc.want)
		})
	}
	for _, kind := range []core.SubtaskEditKind{"", "unknown", core.PlannedSubtaskRemove} {
		in := acAI(kind)
		r, e := auditActualSubtaskHistoryVNext(s, b, b, in)
		acZero(t, r, e, core.ErrInvalidInput)
	}
}
func TestAcceptanceBudgetAndClosedKinds(t *testing.T) {
	k := acPool(t)
	b := k.Snapshot()
	s := acSeal(t, b, nil)
	for n := 0; n < 256; n++ {
		r := auditedHistoryCommand{&historyCommandProof{seal: forwardClockOrdinaryHistoryVNext, kind: historyCommandKind(n), prior: s, after: s}}
		if r.valid() != (n >= 1 && n <= 10) {
			t.Fatalf("kind %d", n)
		}
	}
	limit := int(canonical.DefaultLimits().MaxBytes)
	huge := strings.Repeat("x", limit+1)
	for _, planned := range []bool{true, false} {
		fields := 6
		if !planned {
			fields = 7
		}
		for j := 0; j < fields; j++ {
			for _, size := range []int{limit, limit + 1} {
				var e error
				if planned {
					in := plannedSubtaskHistoryInput{}
					v := reflect.ValueOf(&in).Elem()
					names := []string{"Kind", "ChainID", "ExpectedDefinitionID", "SubtaskID", "LineageID", "Title"}
					v.FieldByName(names[j]).SetString(huge[:size])
					e = historyCommandPlannedSubtaskInputBudget(in)
				} else {
					in := actualSubtaskHistoryInput{}
					v := reflect.ValueOf(&in).Elem()
					names := []string{"Kind", "ChainID", "TraceID", "SubtaskID", "LineageID", "Title", "CancellationID"}
					v.FieldByName(names[j]).SetString(huge[:size])
					e = historyCommandActualSubtaskInputBudget(in)
				}
				if (e == nil) != (size == limit) {
					t.Fatalf("field %d size %d: %v", j, size, e)
				}
			}
		}
	}
	in := acPI(core.SubtaskTitle)
	in.Title = huge
	bad := b.Clone()
	bad.Definitions[0].Title = huge
	checks := []struct {
		seal          validatedHistorySnapshot
		before, after core.TaskGraph
		phase         historyCommandPhase
	}{{s, bad, bad, historyCommandBefore}, {s, b, bad, historyCommandAfter}, {s, b, b, historyCommandInputFields}}
	for _, c := range checks {
		r, e := auditPlannedSubtaskHistoryVNext(c.seal, c.before, c.after, in)
		var be *historyCommandBudgetError
		if !errors.As(e, &be) || be.Phase != c.phase || r.proof != nil {
			t.Fatalf("budget order %v", e)
		}
	}
	r, e := auditPlannedSubtaskHistoryVNext(validatedHistorySnapshot{}, bad, bad, in)
	var ve *historyVNextError
	if !errors.As(e, &ve) || ve.Reason != historyProof || r.proof != nil {
		t.Fatal("seal priority", e)
	}
	in.Title = "ok"
	bad = b.Clone()
	bad.Definitions[0].Title = "mismatch"
	in.Kind = "bad"
	r, e = auditPlannedSubtaskHistoryVNext(s, bad, b, in)
	var ce *historyCommandError
	if !errors.As(e, &ce) || ce.Reason != historyCommandBinding || r.proof != nil {
		t.Fatal("binding priority", e)
	}
}
func TestAcceptanceASTFirstGates(t *testing.T) {
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, "history_subtask_command_vnext.go", nil, 0)
	acMust(t, e)
	count := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || !strings.HasPrefix(fn.Name.Name, "audit") {
			continue
		}
		count++
		want := [][]string{{"valid", "historyVNextFail"}, {"historyCommandGraphBudget"}, {"historyCommandGraphBudget"}, {"historyCommandPlannedSubtaskInputBudget"}}
		if strings.Contains(fn.Name.Name, "Actual") {
			want[3][0] = "historyCommandActualSubtaskInputBudget"
		}
		for i := 0; i < 4; i++ {
			if _, ok := fn.Body.List[i].(*ast.IfStmt); !ok {
				t.Fatal("not straight gate")
			}
			var calls []string
			ast.Inspect(fn.Body.List[i], func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					switch x := c.Fun.(type) {
					case *ast.Ident:
						calls = append(calls, x.Name)
					case *ast.SelectorExpr:
						calls = append(calls, x.Sel.Name)
					}
				}
				return true
			})
			if !reflect.DeepEqual(calls, want[i]) {
				t.Fatalf("%s gate%d calls%v", fn.Name.Name, i, calls)
			}
		}
		a, ok := fn.Body.List[4].(*ast.AssignStmt)
		if !ok {
			t.Fatal("not immediate bind")
		}
		if a.Rhs[0].(*ast.CallExpr).Fun.(*ast.Ident).Name != "historyCommandBind" {
			t.Fatal("not bind")
		}
	}
	if count != 2 {
		t.Fatal(count)
	}
}

// This parameter fixture is deliberately not claimed to be Service reachable.
// Only the baseline (e35a5d5) continuation-undo fixture is used, never candidate tests.
func TestAcceptanceHistoricalAliasFullDefinition(t *testing.T) {
	wire, g, cmd := reviewUndoContinuationAlias(t)
	_, extra, e := v7.ToDomain(wire)
	acMust(t, e)
	_, e = inspectForwardClockOrdinaryHistoryVNext(wire)
	var ve *historyVNextError
	if !errors.As(e, &ve) || ve.Reason != historyToday {
		t.Fatalf("original alias should stop at seal: %v", e)
	}
	next := &g.Traces[len(g.Traces)-1]
	day, _ := next.Date.AddDays(-1)
	next.DraftCancelledOn = &day
	di := -1
	for i := range g.Definitions {
		if g.Definitions[i].ID == cmd.planned.ExpectedDefinitionID {
			di = i
		}
	}
	if di < 0 {
		t.Fatal("definition missing")
	}
	g.Definitions[di].PlannedSubtasks = []core.PlannedSubtask{{ID: core.PlannedSubtaskID(acID(200)), LineageID: core.SubtaskLineageID(acID(201)), Title: "Alias child", Difficulty: core.DifficultySimple, Position: 1, CreatedAt: g.Definitions[di].CreatedAt}}
	prior := acSeal(t, g, &extra.Classifications)
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.PlannedSubtaskRemove} {
		t.Run(string(kind), func(t *testing.T) {
			in := plannedSubtaskHistoryInput(*cmd.planned)
			in.Kind = kind
			if kind != core.SubtaskAdd {
				in.SubtaskID = core.PlannedSubtaskID(acID(200))
				in.LineageID = ""
				in.Title = ""
				in.Difficulty = 0
			}
			if kind == core.SubtaskTitle {
				in.Title = "Changed"
			}
			if kind == core.SubtaskDifficulty {
				in.Difficulty = 2
			}
			k, e := core.NewTaskKernel(g)
			acMust(t, e)
			acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
			after := k.Snapshot()
			final := acSeal(t, after, &extra.Classifications)
			_, x, e := historyCommandBind(prior, g)
			acMust(t, e)
			acMust(t, historyCommandMatch(after, after, x))
			_, e = historyCommandFinish(prior, after, x, nil)
			acMust(t, e)
			if historyPlannedSubtaskFence(prior, final, in) {
				t.Fatal("historical full definition allowed")
			}
			r, e := auditPlannedSubtaskHistoryVNext(prior, g, after, in)
			var ce *historyCommandError
			if !errors.As(e, &ce) || ce.Reason != historyCommandFence || r.proof != nil {
				t.Fatalf("did not hit exact fence: %v", e)
			}
		})
	}
	for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty} {
		in := plannedSubtaskHistoryInput(*cmd.planned)
		in.Kind = kind
		in.SubtaskID = core.PlannedSubtaskID(acID(200))
		in.LineageID = ""
		in.Title = ""
		in.Difficulty = 0
		in.Now = acAt(math.NaN())
		if kind == core.SubtaskTitle {
			in.Title = " Alias child "
		} else {
			in.Difficulty = core.DifficultySimple
		}
		r, e := auditPlannedSubtaskHistoryVNext(prior, g, g, in)
		acMust(t, e)
		if r.proof.changed {
			t.Fatal("alias no-op changed")
		}
	}
	clockOnly := g.Clone()
	clockOnly.Definitions[di].ContentUpdatedAt = acAt(40)
	final := acSeal(t, clockOnly, &extra.Classifications)
	in := plannedSubtaskHistoryInput(*cmd.planned)
	if historyPlannedSubtaskFence(prior, final, in) {
		t.Fatal("historical definition clock not frozen")
	}
}
func TestAcceptanceReturnFreshDefinition(t *testing.T) {
	k := acActual(t)
	before := k.Snapshot()
	prior := acSeal(t, before, nil)
	acMust(t, k.ReturnToPool(core.ReturnToPoolInput{ChainID: core.TaskChainID(acID(1)), TraceID: core.DayTraceID(acID(3)), NewDefinitionID: core.TaskDefinitionID(acID(500)), PlannedIDs: map[core.SubtaskID]core.PlannedSubtaskID{core.SubtaskID(acID(30)): core.PlannedSubtaskID(acID(510)), core.SubtaskID(acID(31)): core.PlannedSubtaskID(acID(511)), core.SubtaskID(acID(32)): core.PlannedSubtaskID(acID(512))}, Today: acDay, Now: acAt(4)}))
	_, extra, e := historyCommandBind(prior, before)
	acMust(t, e)
	s, e := historyCommandFinish(prior, k.Snapshot(), extra, []historyCaptureIntent{{EventID: acID(501), TraceID: core.DayTraceID(acID(3)), Status: core.TraceReturnedToPool, At: acAt(4)}})
	acMust(t, e)
	before = k.Snapshot()
	in := acPI(core.SubtaskTitle)
	in.ExpectedDefinitionID = core.TaskDefinitionID(acID(500))
	in.SubtaskID = before.Definitions[len(before.Definitions)-1].PlannedSubtasks[1].ID
	in.Title = "Fresh only"
	acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
	r, e := auditPlannedSubtaskHistoryVNext(s, before, k.Snapshot(), in)
	acMust(t, e)
	if !historyEqual(s.proof.snapshot.Object["definitions"].Array[0], r.proof.after.proof.snapshot.Object["definitions"].Array[0]) {
		t.Fatal("retired history changed")
	}
}
func TestAcceptanceRemoveArrayOrderAndTombstone(t *testing.T) {
	k := acPool(t)
	b := k.Snapshot()
	p := b.Definitions[0].PlannedSubtasks
	p[0], p[2] = p[2], p[0]
	s := acSeal(t, b, nil)
	k, e := core.NewTaskKernel(b)
	acMust(t, e)
	in := acPI(core.PlannedSubtaskRemove)
	acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
	after := k.Snapshot()
	r, e := auditPlannedSubtaskHistoryVNext(s, b, after, in)
	acMust(t, e)
	got := after.Definitions[0].PlannedSubtasks
	if len(got) != 2 || string(got[0].ID) != acID(12) || string(got[1].ID) != acID(10) || got[0].Position != 1 || got[1].Position != 2 || !r.valid() {
		t.Fatal("sorted or wrong remove")
	}
	k = acActual(t)
	del := acAI(core.SubtaskDelete)
	del.CancellationID = core.CancellationID(acID(700))
	acMust(t, k.EditSubtask(core.SubtaskEditInput(del)))
	b = k.Snapshot()
	s = acSeal(t, b, nil)
	add := acAI(core.SubtaskAdd)
	add.Now = acAt(10)
	add.SubtaskID = core.SubtaskID(acID(701))
	add.LineageID = core.SubtaskLineageID(acID(702))
	add.Title = "fourth including tombstone"
	add.Difficulty = core.DifficultySimple
	acMust(t, k.EditSubtask(core.SubtaskEditInput(add)))
	after = k.Snapshot()
	r, e = auditActualSubtaskHistoryVNext(s, b, after, add)
	acMust(t, e)
	if len(after.Subtasks) != 4 || after.Subtasks[3].Position != 4 || !reflect.DeepEqual(after.Subtasks[:3], b.Subtasks) {
		t.Fatal("tombstone lost/count incorrect")
	}
	noop := acAI(core.SubtaskTitle)
	noop.Title = "Middle"
	noop.Now = acAt(math.NaN())
	r, e = auditActualSubtaskHistoryVNext(s, b, b, noop)
	acZero(t, r, e, core.ErrNotFound)
	del.TraceID = core.DayTraceID(acID(999))
	r, e = auditActualSubtaskHistoryVNext(s, b, b, del)
	acZero(t, r, e, core.ErrInvalidInput)
}
func TestAcceptanceShapeAndFutureStateGuards(t *testing.T) {
	k := acActual(t)
	b := k.Snapshot()
	s := acSeal(t, b, nil)
	kinds := []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskDelete, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon}
	for _, kind := range kinds {
		for _, field := range []string{"LineageID", "Title", "Difficulty", "CancellationID"} {
			in := acAI(kind)
			if kind == core.SubtaskAdd {
				in.SubtaskID = core.SubtaskID(acID(900))
				in.LineageID = core.SubtaskLineageID(acID(901))
				in.Title = "new"
				in.Difficulty = core.DifficultySimple
			}
			if kind == core.SubtaskTitle {
				in.Title = "title"
			}
			if kind == core.SubtaskDifficulty {
				in.Difficulty = 2
			}
			if kind == core.SubtaskDelete {
				in.CancellationID = core.CancellationID(acID(902))
			}
			allowed := (field == "LineageID" && kind == core.SubtaskAdd) || (field == "Title" && (kind == core.SubtaskAdd || kind == core.SubtaskTitle)) || (field == "Difficulty" && (kind == core.SubtaskAdd || kind == core.SubtaskDifficulty)) || (field == "CancellationID" && kind == core.SubtaskDelete)
			if allowed {
				continue
			}
			switch field {
			case "LineageID":
				in.LineageID = core.SubtaskLineageID(acID(903))
			case "Title":
				in.Title = "\xff"
			case "Difficulty":
				in.Difficulty = 2
			case "CancellationID":
				in.CancellationID = core.CancellationID(acID(904))
			}
			r, e := auditActualSubtaskHistoryVNext(s, b, b, in)
			if !errors.Is(e, core.ErrInvalidInput) || r.proof != nil {
				t.Fatalf("%s/%s: %v", kind, field, e)
			}
		}
	}
	// Same legal P trace, but Today is yesterday: only state transitions must fail immutable.
	for _, kind := range []core.SubtaskEditKind{core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon} {
		in := acAI(kind)
		in.Today, _ = acDay.AddDays(-1)
		r, e := auditActualSubtaskHistoryVNext(s, b, b, in)
		acZero(t, r, e, core.ErrImmutableHistory)
	}
	in := acAI(core.SubtaskTitle)
	in.Title = "future content"
	in.Today, _ = acDay.AddDays(-1)
	acMust(t, k.EditSubtask(core.SubtaskEditInput(in)))
	_, e := auditActualSubtaskHistoryVNext(s, b, k.Snapshot(), in)
	acMust(t, e)
	k = acActual(t)
	for j := 30; j <= 32; j++ {
		ci := acAI(core.SubtaskComplete)
		ci.SubtaskID = core.SubtaskID(acID(j))
		ci.Now = acAt(4)
		acMust(t, k.EditSubtask(core.SubtaskEditInput(ci)))
	}
	acMust(t, k.MarkCompleted(core.DayTraceID(acID(3)), acDay, acAt(5)))
	b = k.Snapshot()
	s = acSeal(t, b, nil)
	for _, kind := range []core.SubtaskEditKind{core.SubtaskAdd, core.SubtaskTitle} {
		in := acAI(kind)
		in.Title = "content"
		want := core.ErrImmutableHistory
		if kind == core.SubtaskAdd {
			in.SubtaskID = core.SubtaskID(acID(900))
			in.LineageID = core.SubtaskLineageID(acID(901))
			in.Difficulty = core.DifficultySimple
			want = core.ErrInvalidTransition
		}
		r, e := auditActualSubtaskHistoryVNext(s, b, b, in)
		acZero(t, r, e, want)
	}
}
func TestAcceptanceWitnessAndExactZero(t *testing.T) {
	for _, kind := range []core.SubtaskEditKind{core.SubtaskTitle, core.SubtaskDifficulty, core.SubtaskComplete, core.SubtaskUndoComplete, core.SubtaskAbandon, core.SubtaskDelete} {
		k := acActual(t)
		g := k.Snapshot()
		w := core.CancellationID(acID(990))
		g.Subtasks[1].DraftCancellationID = &w
		if kind == core.SubtaskUndoComplete {
			at := acAt(5)
			g.Subtasks[1].Status = core.SubtaskCompleted
			g.Subtasks[1].CompletedAt = &at
			g.Subtasks[1].UpdatedAt = at
		}
		s := acSeal(t, g, nil)
		k, e := core.NewTaskKernel(g)
		acMust(t, e)
		in := acAI(kind)
		if kind == core.SubtaskTitle {
			in.Title = "new"
		}
		if kind == core.SubtaskDifficulty {
			in.Difficulty = 2
		}
		if kind == core.SubtaskDelete {
			in.CancellationID = core.CancellationID(acID(991))
		}
		acMust(t, k.EditSubtask(core.SubtaskEditInput(in)))
		after := k.Snapshot()
		_, e = auditActualSubtaskHistoryVNext(s, g, after, in)
		acMust(t, e)
		want := w
		if kind == core.SubtaskDelete {
			want = in.CancellationID
		}
		if after.Subtasks[1].DraftCancellationID == nil || *after.Subtasks[1].DraftCancellationID != want {
			t.Fatalf("witness %s", kind)
		}
	}
	for _, pair := range [][2]float64{{math.Copysign(0, -1), 0}, {0, math.Copysign(0, -1)}, {-9, -8}} {
		k := acPool(t)
		g := k.Snapshot()
		g.Chains[0].CreatedAt = acAt(pair[0])
		g.Chains[0].UpdatedAt = acAt(pair[0])
		g.Definitions[0].CreatedAt = acAt(pair[0])
		g.Definitions[0].ContentUpdatedAt = acAt(pair[0])
		for i := range g.Definitions[0].PlannedSubtasks {
			g.Definitions[0].PlannedSubtasks[i].CreatedAt = acAt(pair[0])
		}
		s := acSeal(t, g, nil)
		k, e := core.NewTaskKernel(g)
		acMust(t, e)
		in := acPI(core.SubtaskTitle)
		in.Title = "exact bits"
		in.Now = acAt(pair[1])
		acMust(t, k.EditPlannedSubtask(core.PlannedSubtaskEditInput(in)))
		after := k.Snapshot()
		r, e := auditPlannedSubtaskHistoryVNext(s, g, after, in)
		acMust(t, e)
		if after.Chains[0].UpdatedAt != in.Now || after.Definitions[0].ContentUpdatedAt != in.Now || after.Definitions[0].PlannedSubtasks[1].CreatedAt != acAt(pair[0]) || r.proof.now != in.Now {
			t.Fatal("clock bits changed")
		}
	}
}
func TestAcceptanceInputCumulativeBudget(t *testing.T) {
	limit := int(canonical.DefaultLimits().MaxBytes)
	str := strings.Repeat("s", limit)
	p := acPI(core.SubtaskTitle)
	p.Title = ""
	fixed := len(p.Kind) + len(p.ChainID) + len(p.ExpectedDefinitionID) + len(p.SubtaskID)
	p.Title = str[:limit-fixed]
	acMust(t, historyCommandPlannedSubtaskInputBudget(p))
	p.Title = str[:limit-fixed+1]
	if historyCommandPlannedSubtaskInputBudget(p) == nil {
		t.Fatal("cumulative planned")
	}
	a := actualSubtaskHistoryInput{Kind: core.SubtaskEditKind(str[:limit/2]), Title: str[:limit/2]}
	acMust(t, historyCommandActualSubtaskInputBudget(a))
	a.CancellationID = "x"
	if historyCommandActualSubtaskInputBudget(a) == nil {
		t.Fatal("unused/cumulative actual")
	}
}
