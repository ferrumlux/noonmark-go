// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"reflect"
	"strings"
	"testing"
)

func TestHistorySubtaskInputBudgetBoundaries(t *testing.T) {
	limit := canonical.DefaultLimits().MaxBytes
	for _, actual := range []bool{false, true} {
		fields := []string{"Kind", "ChainID", "SubtaskID", "LineageID", "Title"}
		if actual {
			fields = append(fields, "TraceID", "CancellationID")
		} else {
			fields = append(fields, "ExpectedDefinitionID")
		}
		for _, normal := range []bool{false, true} {
			for _, field := range fields {
				t.Run(field+map[bool]string{false: "planned", true: "actual"}[actual]+map[bool]string{false: "zero", true: "normal"}[normal], func(t *testing.T) {
					for _, over := range []int{0, 1} {
						var input any = &plannedSubtaskHistoryInput{}
						if actual {
							input = &actualSubtaskHistoryInput{}
						}
						v := reflect.ValueOf(input).Elem()
						if normal {
							for _, name := range fields {
								v.FieldByName(name).SetString("x")
							}
						}
						remainder := 0
						for _, name := range fields {
							if name != field {
								remainder += len(v.FieldByName(name).String())
							}
						}
						v.FieldByName(field).SetString(strings.Repeat("x", limit-remainder+over))
						var err error
						if actual {
							err = historyCommandActualSubtaskInputBudget(*input.(*actualSubtaskHistoryInput))
						} else {
							err = historyCommandPlannedSubtaskInputBudget(*input.(*plannedSubtaskHistoryInput))
						}
						if over == 0 {
							must(t, err)
						} else {
							hcBudgetError(t, err, historyCommandInputFields, historyCommandRawBytes)
						}
					}
				})
			}
		}
	}
	// Shared backing is charged per logical field; malformed content is never scanned.
	shared := strings.Repeat("x", limit/2+1)
	hcBudgetError(t, historyCommandPlannedSubtaskInputBudget(plannedSubtaskHistoryInput{Kind: core.SubtaskEditKind(shared), Title: shared}), historyCommandInputFields, historyCommandRawBytes)
	hcBudgetError(t, historyCommandActualSubtaskInputBudget(actualSubtaskHistoryInput{Title: shared, CancellationID: core.CancellationID(shared)}), historyCommandInputFields, historyCommandRawBytes)
	must(t, historyCommandPlannedSubtaskInputBudget(plannedSubtaskHistoryInput{ChainID: "short", Title: string([]byte{255})}))
	must(t, historyCommandActualSubtaskInputBudget(actualSubtaskHistoryInput{TraceID: "short", Title: string([]byte{255})}))
}
func TestHistorySubtaskBudgetPrecedence(t *testing.T) {
	f := vnSeed(t, vnDate)
	normal := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	huge := strings.Repeat("x", canonical.DefaultLimits().MaxBytes+1)
	raw := normal.Clone()
	raw.Definitions[0].Title = huge
	bad := normal.Clone()
	bad.Traces[0].ID = "invalid"
	for _, actual := range []bool{false, true} {
		audit := func(p validatedHistorySnapshot, b, a core.TaskGraph) (auditedHistoryCommand, error) {
			if actual {
				return auditActualSubtaskHistoryVNext(p, b, a, actualSubtaskHistoryInput{Title: huge})
			}
			return auditPlannedSubtaskHistoryVNext(p, b, a, plannedSubtaskHistoryInput{Title: huge})
		}
		r, e := audit(validatedHistorySnapshot{}, raw, raw)
		hcZero(t, r, e)
		var typed *historyVNextError
		if !errors.As(e, &typed) || typed.Reason != historyProof {
			t.Fatalf("seal gate: %v", e)
		}
		for _, tc := range []struct {
			b, a  core.TaskGraph
			phase historyCommandPhase
		}{
			{raw, raw, historyCommandBefore}, {bad, raw, historyCommandAfter}, {bad, bad, historyCommandInputFields},
		} {
			r, e := audit(prior, tc.b, tc.a)
			hcZero(t, r, e)
			hcBudgetError(t, e, tc.phase, historyCommandRawBytes)
		}
	}
}

func TestHistorySubtaskThreeIndependentBudgets(t *testing.T) {
	// Each endpoint and the input fit separately, but their combined raw size exceeds MaxBytes.
	k := hsgPool(t)
	g := k.Snapshot()
	large := strings.Repeat("x", canonical.DefaultLimits().MaxBytes/3+1024)
	g.Definitions[0].Title = large
	prior := vnGood(t, hcWire(t, g, canonical.Value{}))
	for _, actual := range []bool{false, true} {
		var r auditedHistoryCommand
		var e error
		if actual {
			in := hsgInput(core.SubtaskTitle)
			in.Kind = core.SubtaskEditKind(large)
			r, e = auditActualSubtaskHistoryVNext(prior, g, g, in)
		} else {
			in := hsgPlanned(core.SubtaskTitle)
			in.Kind = core.SubtaskEditKind(large)
			r, e = auditPlannedSubtaskHistoryVNext(prior, g, g, in)
		}
		hsgError(t, r, e, core.ErrInvalidInput)
	}
}

func TestHistorySubtaskGraphShapeBudgetPrecedence(t *testing.T) {
	f := vnSeed(t, vnDate)
	g := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	shape := g.Clone()
	shape.Days = make([]core.Day, canonical.DefaultLimits().MaxNodes/17+1)
	for _, actual := range []bool{false, true} {
		for _, phase := range []historyCommandPhase{historyCommandBefore, historyCommandAfter} {
			before, after := g, shape
			if phase == historyCommandBefore {
				before, after = shape, shape
			}
			var r auditedHistoryCommand
			var e error
			if actual {
				r, e = auditActualSubtaskHistoryVNext(prior, before, after, actualSubtaskHistoryInput{})
			} else {
				r, e = auditPlannedSubtaskHistoryVNext(prior, before, after, plannedSubtaskHistoryInput{})
			}
			hcZero(t, r, e)
			hcBudgetError(t, e, phase, historyCommandShape)
		}
	}
}
