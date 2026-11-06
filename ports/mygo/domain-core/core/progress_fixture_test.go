// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"reflect"
	"testing"
)

// Only synthetic development/e2e facts. Unported history commands are assembled
// explicitly and still validated; these fixtures are NOT Swift-oracle output.
type progressFixture struct {
	g    TaskGraph
	next int
}

func newProgressFixture() *progressFixture { return &progressFixture{g: baseGraph(), next: 1000} }
func (f *progressFixture) id() string      { f.next++; return identity(f.next) }
func (f *progressFixture) trace(when LocalDate, status TraceStatus) int {
	found := false
	for _, d := range f.g.Days {
		if d.Date == when {
			found = true
		}
	}
	if !found {
		f.g.Days = append(f.g.Days, Day{ID: DayID(f.id()), Date: when, CreatedAt: instant(100), UpdatedAt: instant(100)})
	}
	i := len(f.g.Traces)
	f.g.Traces = append(f.g.Traces, DayTrace{ID: DayTraceID(f.id()), ChainID: f.g.Chains[0].ID, DefinitionID: f.g.Definitions[0].ID, Date: when, Priority: i + 1, CreatedAt: instant(100), ContentUpdatedAt: instant(100)})
	f.traceStatus(i, status)
	return i
}
func (f *progressFixture) traceStatus(i int, status TraceStatus) {
	tr := &f.g.Traces[i]
	tr.Status = status
	tr.CompletedAt, tr.SettledAt, tr.DraftCancellationID, tr.DraftCancelledOn = nil, nil, nil, nil
	switch status {
	case TraceCompleted:
		tr.CompletedAt = ptr(instant(100))
	case TracePending:
	case TraceCancelledDraft:
		tr.SettledAt = ptr(instant(100))
		tr.DraftCancellationID = ptr(CancellationID(f.id()))
		tr.DraftCancelledOn = ptr(tr.Date)
	default:
		tr.SettledAt = ptr(instant(100))
	}
}
func (f *progressFixture) continuation(source int, when LocalDate, status TraceStatus) int {
	target := f.trace(when, status)
	f.g.Traces[target].CarriedFromTraceID = ptr(f.g.Traces[source].ID)
	f.g.Traces[target].ContinuationSeq = f.g.Traces[source].ContinuationSeq
	if f.g.Traces[source].Status == TraceUnfinished {
		f.g.Traces[target].ContinuationSeq++
	}
	return target
}
func (f *progressFixture) sub(trace int, difficulty Difficulty, status SubtaskStatus, lineage SubtaskLineageID) int {
	if lineage == "" {
		lineage = SubtaskLineageID(f.id())
	}
	position := 1
	for _, s := range f.g.Subtasks {
		if s.TraceID == f.g.Traces[trace].ID {
			position++
		}
	}
	s := Subtask{ID: SubtaskID(f.id()), LineageID: lineage, TraceID: f.g.Traces[trace].ID, Title: "合成子任务", Difficulty: difficulty, Status: status, Position: position, CreatedAt: instant(100), UpdatedAt: instant(100)}
	switch status {
	case SubtaskCompleted:
		s.CompletedAt = ptr(instant(100))
	case SubtaskPending:
	case SubtaskCancelledDraft:
		s.SettledAt = ptr(instant(100))
		s.DraftCancellationID = ptr(CancellationID(f.id()))
	default:
		s.SettledAt = ptr(instant(100))
	}
	f.g.Subtasks = append(f.g.Subtasks, s)
	return len(f.g.Subtasks) - 1
}
func progressView(t *testing.T, g TaskGraph) *ProgressProjection {
	t.Helper()
	before := g.Clone()
	p, e := NewProgressProjection(g)
	if e != nil {
		t.Fatalf("projection fixture rejected: %#v", e)
	}
	if !reflect.DeepEqual(before, g) {
		t.Fatal("constructor mutated input")
	}
	return p
}
func wantProgress(t *testing.T, p *ProgressProjection, id DayTraceID, want TraceProgress) {
	t.Helper()
	if got := p.TraceProgress(id); got != want {
		t.Fatalf("progress = %+v, want %+v", got, want)
	}
}
func wantCompletion(t *testing.T, p *ProgressProjection, id DayTraceID, today LocalDate, want DayTraceCompletionCapability) {
	t.Helper()
	got, e := p.CompletionCapability(id, today)
	if e != nil || got != want {
		t.Fatalf("capability = %+v, %v; want %+v", got, e, want)
	}
}
func wantManual(t *testing.T, p *ProgressProjection, id DayTraceID, today LocalDate, want bool) {
	t.Helper()
	got, e := p.CanSetManualProgress(id, today)
	if e != nil || got != want {
		t.Fatalf("manual = %v, %v; want %v", got, e, want)
	}
}
