// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"fmt"
	"sort"
	"strings"
)

// Issue is for tests and internal error handling, not diagnostics logging.
// Invalid imported identities are untrusted input: do not log raw Issues.
type Issue struct {
	Code    string
	Entity  string
	Related string
}
type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("task graph rejected: %d invariant violation(s)", len(e.Issues))
}
func (e *ValidationError) Has(code string) bool {
	for _, i := range e.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

type validation struct{ issues []Issue }

func (v *validation) add(code string, entity any, related any) {
	v.issues = append(v.issues, Issue{code, fmt.Sprint(entity), fmt.Sprint(related)})
}
func (v *validation) result() error {
	if len(v.issues) == 0 {
		return nil
	}
	sort.Slice(v.issues, func(i, j int) bool {
		a, b := v.issues[i], v.issues[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Entity != b.Entity {
			return a.Entity < b.Entity
		}
		return a.Related < b.Related
	})
	return &ValidationError{Issues: v.issues}
}
func indexBy[T any, ID ~string](v *validation, kind string, values []T, id func(T) ID) map[ID]T {
	out := make(map[ID]T, len(values))
	for _, x := range values {
		key := id(x)
		if !validID(string(key)) {
			v.add("invalidIdentity", kind, key)
		}
		if _, ok := out[key]; ok {
			v.add("duplicateIdentity", kind, key)
		} else {
			out[key] = x
		}
	}
	return out
}
func validDifficulty(d Difficulty) bool { return d >= DifficultySimple && d <= DifficultyHard }
func (v *validation) notes(owner string, notes []TaskNoteEntry) {
	seen := map[TaskNoteEntryID]bool{}
	for _, n := range notes {
		if !validID(string(n.ID)) {
			v.add("invalidNoteIdentity", owner, n.ID)
		}
		if seen[n.ID] {
			v.add("duplicateNoteIdentity", owner, n.ID)
		}
		seen[n.ID] = true
		if !clockValid(n.CreatedAt, n.UpdatedAt, n.DeletedAt) {
			v.add("invalidNoteClock", owner, n.ID)
		}
		if n.DeletedAt != nil {
			if n.Body != "" || !n.DeletedAt.Equal(n.UpdatedAt) {
				v.add("invalidNoteTombstone", owner, n.ID)
			}
		} else if n.Body == "" || strings.TrimSpace(n.Body) != n.Body {
			v.add("invalidNoteBody", owner, n.ID)
		}
	}
}

// ValidateTaskGraph validates this bounded graph only. In particular it does NOT
// validate classification history, repeat-parent membership or a v7 package.
// Never substitute it for full NoonmarkSnapshot validation at an import seam.
func ValidateTaskGraph(g TaskGraph) error {
	v := &validation{}
	days := indexBy(v, "day", g.Days, func(x Day) DayID { return x.ID })
	_ = days
	chains := indexBy(v, "chain", g.Chains, func(x TaskChain) TaskChainID { return x.ID })
	defs := indexBy(v, "definition", g.Definitions, func(x TaskDefinition) TaskDefinitionID { return x.ID })
	traces := indexBy(v, "trace", g.Traces, func(x DayTrace) DayTraceID { return x.ID })
	subs := indexBy(v, "subtask", g.Subtasks, func(x Subtask) SubtaskID { return x.ID })
	dates := map[LocalDate]Day{}
	for _, d := range g.Days {
		if !d.Date.Valid() {
			v.add("invalidLocalDate", d.ID, d.Date)
		}
		if _, ok := dates[d.Date]; ok {
			v.add("duplicateDayDate", d.Date, "")
		}
		dates[d.Date] = d
		if !clockValid(d.CreatedAt, d.UpdatedAt, d.LockedAt) {
			v.add("invalidDayClock", d.ID, "")
		}
	}
	for _, c := range g.Chains {
		if c.State != ChainActive && c.State != ChainAbandoned {
			v.add("invalidChainState", c.ID, "")
		}
		if !clockValid(c.CreatedAt, c.UpdatedAt) {
			v.add("invalidChainClock", c.ID, "")
		}
		if c.CycleMembership != nil && (!validID(string(c.CycleMembership.SeriesID)) || !c.CycleMembership.OccurrenceDate.Valid()) {
			v.add("invalidCycleMembership", c.ID, "")
		}
		v.notes(string(c.ID), c.NoteEntries)
	}
	v.definitions(g.Definitions, chains, defs)
	v.traces(g.Traces, chains, defs, traces, dates)
	v.subtasks(g.Subtasks, subs, traces)
	// Cancellation identities span traces and subtasks and remain reserved even
	// after a snapshot undo reactivates the corresponding entity.
	cancellations := map[CancellationID]string{}
	reserve := func(id *CancellationID, owner string) {
		if id == nil {
			return
		}
		if !validID(string(*id)) {
			v.add("invalidCancellationIdentity", owner, *id)
		}
		if old, ok := cancellations[*id]; ok {
			v.add("reusedDraftCancellationIdentity", owner, old)
		} else {
			cancellations[*id] = owner
		}
	}
	for _, t := range g.Traces {
		reserve(t.DraftCancellationID, string(t.ID))
	}
	for _, s := range g.Subtasks {
		reserve(s.DraftCancellationID, string(s.ID))
	}
	return v.result()
}

func (v *validation) definitions(values []TaskDefinition, chains map[TaskChainID]TaskChain, defs map[TaskDefinitionID]TaskDefinition) {
	grouped := map[TaskChainID][]TaskDefinition{}
	edges := map[string][]string{}
	for _, d := range values {
		grouped[d.ChainID] = append(grouped[d.ChainID], d)
		c, exists := chains[d.ChainID]
		if !exists {
			v.add("missingDefinitionChain", d.ID, d.ChainID)
		}
		if !clockValid(d.CreatedAt, d.ContentUpdatedAt, d.SupersededAt) {
			v.add("invalidDefinitionClock", d.ID, "")
		}
		// Existing titles may be empty; only task creation rejects an empty title.
		if strings.TrimSpace(d.Title) != d.Title {
			v.add("invalidDefinitionTitle", d.ID, "")
		}
		if d.Sequence <= 0 {
			v.add("invalidDefinitionSequence", d.ID, "")
		}
		if d.SupersededAt != nil && d.SupersededByDefinitionID == nil {
			v.add("incompleteSupersession", d.ID, "")
		}
		if d.SupersededByDefinitionID != nil {
			next := *d.SupersededByDefinitionID
			if next == d.ID {
				v.add("selfSuccessor", d.ID, next)
			}
			successor, ok := defs[next]
			if !ok {
				v.add("missingDefinitionSuccessor", d.ID, next)
			} else if successor.ChainID != d.ChainID {
				v.add("crossChainSuccessor", d.ID, next)
			}
			// A live definition can retain a restoration witness without a cycle edge.
			if d.SupersededAt != nil && ok && successor.ChainID == d.ChainID {
				edges[string(d.ID)] = append(edges[string(d.ID)], string(next))
			}
		}
		ids := map[PlannedSubtaskID]bool{}
		lineages := map[SubtaskLineageID]bool{}
		positions := map[int]bool{}
		for _, s := range d.PlannedSubtasks {
			bad := !validID(string(s.ID)) || !validID(string(s.LineageID)) || ids[s.ID] || lineages[s.LineageID] || positions[s.Position] || s.Position < 1 || s.Position > len(d.PlannedSubtasks) || s.Title == "" || strings.TrimSpace(s.Title) != s.Title || !validDifficulty(s.Difficulty) || !s.CreatedAt.Finite() || s.CreatedAt.After(d.ContentUpdatedAt) || (exists && s.CreatedAt.Before(c.CreatedAt))
			if bad {
				v.add("invalidPlannedSubtasks", d.ID, s.ID)
			}
			ids[s.ID] = true
			lineages[s.LineageID] = true
			positions[s.Position] = true
		}
	}
	for id := range chains {
		ds := grouped[id]
		if len(ds) == 0 {
			v.add("missingDefinitionForChain", id, "")
			continue
		}
		current := 0
		seq := map[int]bool{}
		for _, d := range ds {
			if d.SupersededAt == nil {
				current++
			}
			if seq[d.Sequence] {
				v.add("duplicateDefinitionSequence", id, d.Sequence)
			}
			seq[d.Sequence] = true
		}
		if current != 1 {
			v.add("invalidCurrentDefinitionCount", id, current)
		}
	}
	if cycle, ok := firstCycle(edges); ok {
		v.add("definitionSuccessorCycle", cycle, "")
	}
}

func (v *validation) traces(values []DayTrace, chains map[TaskChainID]TaskChain, defs map[TaskDefinitionID]TaskDefinition, traces map[DayTraceID]DayTrace, days map[LocalDate]Day) {
	active := map[TaskChainID]int{}
	priorities := map[string]DayTraceID{}
	carryTargets := map[DayTraceID][]DayTraceID{}
	changeSources := map[DayTraceID][]DayTraceID{}
	edges := map[string][]string{}
	for _, t := range values {
		if !t.Date.Valid() {
			v.add("invalidLocalDate", t.ID, t.Date)
		}
		if _, ok := chains[t.ChainID]; !ok {
			v.add("missingTraceChain", t.ID, t.ChainID)
		}
		if _, ok := days[t.Date]; !ok {
			v.add("missingTraceDay", t.ID, t.Date)
		}
		d, ok := defs[t.DefinitionID]
		if !ok {
			v.add("missingTraceDefinition", t.ID, t.DefinitionID)
		} else if d.ChainID != t.ChainID {
			v.add("crossChainTraceDefinition", t.ID, d.ID)
		}
		if !clockValid(t.CreatedAt, t.ContentUpdatedAt, t.CompletedAt, t.SettledAt) {
			v.add("invalidTraceClock", t.ID, "")
		}
		if t.ContinuationSeq < 0 || (t.CarriedFromTraceID == nil && t.ContinuationSeq != 0) {
			v.add("invalidTraceSequence", t.ID, "")
		}
		if t.ManualProgressPercent != nil && (*t.ManualProgressPercent < 0 || *t.ManualProgressPercent > 100) {
			v.add("invalidTraceProgress", t.ID, "")
		}
		facts := t.Status.Valid()
		switch t.Status {
		case TracePending:
			facts = facts && t.CompletedAt == nil && t.SettledAt == nil && t.ChangedToTraceID == nil && t.DraftCancelledOn == nil
		case TraceCompleted:
			facts = facts && t.CompletedAt != nil && t.SettledAt == nil && t.ChangedToTraceID == nil && t.DraftCancelledOn == nil
		case TraceUnfinished, TraceDeferred, TraceReturnedToPool, TraceAbandoned:
			facts = facts && t.CompletedAt == nil && t.SettledAt != nil && t.ChangedToTraceID == nil && t.DraftCancelledOn == nil
		case TraceChanged:
			facts = facts && t.CompletedAt == nil && t.SettledAt != nil && t.ChangedToTraceID != nil && t.DraftCancelledOn == nil
		case TraceCancelledDraft:
			facts = facts && t.DraftCancellationID != nil && t.DraftCancelledOn != nil && t.DraftCancelledOn.Valid() && t.Date.Compare(*t.DraftCancelledOn) >= 0 && t.CompletedAt == nil && t.SettledAt != nil && t.ChangedToTraceID == nil
		}
		if t.PinOrder != nil && (*t.PinOrder <= 0 || t.Status != TracePending) {
			facts = false
		}
		if !facts {
			v.add("invalidTraceStatusFacts", t.ID, "")
		}
		v.notes(string(t.ID), t.NoteEntries)
		if t.Status == TracePending {
			active[t.ChainID]++
		}
		// All history facts occupy priority slots, even moved rows hidden by DayTodo.
		if t.Status.FormsDayHistory() {
			slot := fmt.Sprint(t.Date, "/", t.Priority)
			if old, ok := priorities[slot]; ok {
				v.add("duplicateVisiblePriority", t.ID, old)
			} else {
				priorities[slot] = t.ID
			}
		}
		if t.CarriedFromTraceID != nil {
			source := *t.CarriedFromTraceID
			carryTargets[source] = append(carryTargets[source], t.ID)
			edges[string(source)] = append(edges[string(source)], string(t.ID))
		}
		if t.ChangedToTraceID != nil {
			target := *t.ChangedToTraceID
			changeSources[target] = append(changeSources[target], t.ID)
			edges[string(t.ID)] = append(edges[string(t.ID)], string(target))
		}
	}
	for id, count := range active {
		if count > 1 {
			v.add("duplicateActiveTrace", id, count)
		}
	}
	for id, targets := range carryTargets {
		if len(targets) > 1 {
			v.add("reusedTraceContinuationSource", id, "")
		}
	}
	for id, sources := range changeSources {
		if len(sources) > 1 {
			v.add("reusedChangedTraceTarget", id, "")
		}
	}
	for _, t := range values {
		if t.CarriedFromTraceID != nil {
			source, ok := traces[*t.CarriedFromTraceID]
			if !ok {
				v.add("missingTraceContinuationSource", t.ID, *t.CarriedFromTraceID)
			} else {
				cancelledUndo := t.Status == TraceCancelledDraft && source.Status.FormsDayHistory()
				seqOK := (source.Status == TraceDeferred || source.Status == TracePending) && t.ContinuationSeq == source.ContinuationSeq
				if source.Status == TraceUnfinished && source.ContinuationSeq < int(^uint(0)>>1) {
					seqOK = t.ContinuationSeq == source.ContinuationSeq+1
				}
				valid := source.ID != t.ID && source.ChainID == t.ChainID && seqOK && t.Date.Compare(source.Date) > 0 && !t.CreatedAt.Before(source.CreatedAt) && source.Status != TraceCancelledDraft && (source.Status == TraceDeferred || source.Status == TraceUnfinished || cancelledUndo)
				if !valid {
					v.add("invalidTraceContinuation", t.ID, source.ID)
				}
			}
		}
		if t.Status == TraceDeferred && len(carryTargets[t.ID]) == 0 {
			v.add("missingTraceContinuationTarget", t.ID, "")
		}
		if t.ChangedToTraceID != nil {
			target, ok := traces[*t.ChangedToTraceID]
			if !ok {
				v.add("missingChangedTraceTarget", t.ID, *t.ChangedToTraceID)
			} else if t.ID == target.ID || t.Status != TraceChanged || t.ChainID == target.ChainID || t.Date != target.Date || target.ContinuationSeq != 0 || target.CarriedFromTraceID != nil || target.Status == TraceCancelledDraft || target.CreatedAt.Before(t.CreatedAt) {
				v.add("invalidChangedTraceLink", t.ID, target.ID)
			}
		}
	}
	if cycle, ok := firstCycle(edges); ok {
		v.add("traceCausalCycle", cycle, "")
	}
}

func (v *validation) subtasks(values []Subtask, subs map[SubtaskID]Subtask, traces map[DayTraceID]DayTrace) {
	grouped := map[DayTraceID][]Subtask{}
	targets := map[SubtaskID][]SubtaskID{}
	edges := map[string][]string{}
	for _, s := range values {
		grouped[s.TraceID] = append(grouped[s.TraceID], s)
		parent, exists := traces[s.TraceID]
		if !exists {
			v.add("missingSubtaskParent", s.ID, s.TraceID)
		}
		facts := s.Status.Valid()
		switch s.Status {
		case SubtaskPending:
			facts = facts && s.CompletedAt == nil && s.SettledAt == nil
		case SubtaskCompleted:
			facts = facts && s.CompletedAt != nil && s.SettledAt == nil
		case SubtaskUnfinished, SubtaskDeferred, SubtaskAbandoned:
			facts = facts && s.CompletedAt == nil && s.SettledAt != nil
		case SubtaskCancelledDraft:
			facts = facts && s.CompletedAt == nil && s.SettledAt != nil && s.DraftCancellationID != nil
		}
		if !facts || !clockValid(s.CreatedAt, s.UpdatedAt, s.CompletedAt, s.SettledAt) || s.Title == "" || strings.TrimSpace(s.Title) != s.Title || s.Position <= 0 || !validDifficulty(s.Difficulty) || !validID(string(s.LineageID)) {
			v.add("invalidSubtaskFacts", s.ID, "")
		}
		if exists && parent.Status == TraceCancelledDraft && (s.Status != SubtaskPending || s.CompletedAt != nil || s.SettledAt != nil) {
			v.add("cancelledTraceHasHistoricalSubtask", s.ID, parent.ID)
		}
		if s.CarriedFromSubtaskID != nil {
			source := *s.CarriedFromSubtaskID
			targets[source] = append(targets[source], s.ID)
			edges[string(s.ID)] = append(edges[string(s.ID)], string(source))
		}
	}
	for traceID, ss := range grouped {
		positions := map[int]bool{}
		lineages := map[SubtaskLineageID]bool{}
		for _, s := range ss {
			if positions[s.Position] {
				v.add("duplicateSubtaskPosition", traceID, s.Position)
			}
			positions[s.Position] = true
			if lineages[s.LineageID] {
				v.add("duplicateSubtaskLineage", traceID, s.LineageID)
			}
			lineages[s.LineageID] = true
		}
		if len(positions) == len(ss) {
			for pos := range positions {
				if pos < 1 || pos > len(ss) {
					v.add("noncontiguousSubtaskPosition", traceID, "")
					break
				}
			}
		}
	}
	for id, ts := range targets {
		if len(ts) > 1 {
			v.add("reusedSubtaskContinuationSource", id, "")
		}
	}
	for _, s := range values {
		if s.Status == SubtaskDeferred && len(targets[s.ID]) == 0 {
			v.add("missingSubtaskContinuationTarget", s.ID, "")
		}
		if s.CarriedFromSubtaskID == nil {
			continue
		}
		source, ok := subs[*s.CarriedFromSubtaskID]
		if !ok {
			v.add("missingSubtaskContinuationSource", s.ID, *s.CarriedFromSubtaskID)
			continue
		}
		targetTrace, tok := traces[s.TraceID]
		sourceTrace, sok := traces[source.TraceID]
		if !tok || !sok {
			continue
		}
		cancelledUndo := (s.Status == SubtaskCancelledDraft || targetTrace.Status == TraceCancelledDraft) && source.Status != SubtaskCancelledDraft
		valid := source.ID != s.ID && source.LineageID == s.LineageID && sourceTrace.ChainID == targetTrace.ChainID && targetTrace.CarriedFromTraceID != nil && *targetTrace.CarriedFromTraceID == sourceTrace.ID && !s.CreatedAt.Before(source.CreatedAt) && source.Status != SubtaskCancelledDraft && (source.Status == SubtaskDeferred || source.Status == SubtaskUnfinished || cancelledUndo)
		if !valid {
			v.add("invalidSubtaskContinuation", s.ID, source.ID)
		}
		if sourceTrace.Status == TraceCancelledDraft {
			v.add("cancelledTraceHasReferencedSubtask", s.ID, source.ID)
		}
	}
	if cycle, ok := firstCycle(edges); ok {
		v.add("subtaskContinuationCycle", cycle, "")
	}
}

// Iterative DFS prevents hostile import graphs from overflowing the call stack.
func firstCycle(edges map[string][]string) (string, bool) {
	keys := make([]string, 0, len(edges))
	for k := range edges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	state := map[string]uint8{}
	type frame struct {
		id      string
		next    int
		targets []string
	}
	for _, root := range keys {
		if state[root] != 0 {
			continue
		}
		state[root] = 1
		targets := append([]string(nil), edges[root]...)
		sort.Strings(targets)
		stack := []frame{{id: root, targets: targets}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next == len(top.targets) {
				state[top.id] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			target := top.targets[top.next]
			top.next++
			if state[target] == 1 {
				return target, true
			}
			if state[target] == 2 {
				continue
			}
			state[target] = 1
			children := append([]string(nil), edges[target]...)
			sort.Strings(children)
			stack = append(stack, frame{id: target, targets: children})
		}
	}
	return "", false
}
