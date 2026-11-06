// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound                           = errors.New("not found")
	ErrInvalidTitle                       = errors.New("task title cannot be empty")
	ErrChainAbandoned                     = errors.New("chain abandoned")
	ErrImmutableHistory                   = errors.New("immutable history")
	ErrInvalidTransition                  = errors.New("invalid transition")
	ErrFuturePlanCannotComplete           = errors.New("future plan cannot complete")
	ErrHistoricalCompletionCannotBeUndone = errors.New("historical completion cannot be undone")
	ErrOpenSubtasksPreventCompletion      = errors.New("open subtasks prevent completion")
	ErrInvalidInput                       = errors.New("invalid input")
	ErrUnsupportedRecurring               = errors.New("recurring-parent validation is not implemented in this kernel")
)

// TaskKernel is an isolated first-stage task-graph kernel, NOT a replacement for
// NoonmarkEngine. It has no classification, persistence, recurring-plan, sync or
// AI authority. Integration must compose commands with those later modules in
// one transaction before publishing a user-visible or persisted snapshot.
// All operations serialize and publish a validated deep copy only on success.
type TaskKernel struct {
	mu    sync.RWMutex
	graph TaskGraph
}

func NewTaskKernel(graph TaskGraph) (*TaskKernel, error) {
	if err := ValidateTaskGraph(graph); err != nil {
		return nil, err
	}
	// Do not mutate recurring instances without their omitted parent state.
	for _, c := range graph.Chains {
		if c.CycleMembership != nil {
			return nil, ErrUnsupportedRecurring
		}
	}
	return &TaskKernel{graph: graph.Clone()}, nil
}
func (k *TaskKernel) Snapshot() TaskGraph { k.mu.RLock(); defer k.mu.RUnlock(); return k.graph.Clone() }
func (k *TaskKernel) mutate(fn func(*TaskGraph) error) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	candidate := k.graph.Clone()
	if err := fn(&candidate); err != nil {
		return err
	}
	if err := ValidateTaskGraph(candidate); err != nil {
		return err
	}
	k.graph = candidate
	return nil
}

type CreatePoolTaskInput struct {
	ChainID         TaskChainID
	DefinitionID    TaskDefinitionID
	Title           string
	DescriptionText *string
	InitialNote     *TaskNoteEntry
	Now             Instant
}

func (k *TaskKernel) CreatePoolTask(in CreatePoolTaskInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Now.Finite() {
			return ErrInvalidClock
		}
		title := strings.TrimSpace(in.Title)
		if title == "" {
			return ErrInvalidTitle
		}
		if !validID(string(in.ChainID)) || !validID(string(in.DefinitionID)) {
			return ErrInvalidInput
		}
		for _, c := range g.Chains {
			if c.ID == in.ChainID {
				return ErrInvalidInput
			}
		}
		for _, d := range g.Definitions {
			if d.ID == in.DefinitionID {
				return ErrInvalidInput
			}
		}
		notes := []TaskNoteEntry(nil)
		if in.InitialNote != nil {
			n := cloneNote(*in.InitialNote)
			if !n.CreatedAt.Equal(in.Now) || !n.UpdatedAt.Equal(in.Now) || n.DeletedAt != nil {
				return ErrInvalidInput
			}
			n.Body = strings.TrimSpace(n.Body)
			notes = append(notes, n)
		}
		g.Chains = append(g.Chains, TaskChain{ID: in.ChainID, State: ChainActive, NoteEntries: notes, CreatedAt: in.Now, UpdatedAt: in.Now})
		g.Definitions = append(g.Definitions, TaskDefinition{ID: in.DefinitionID, ChainID: in.ChainID, Sequence: 1, Title: title, DescriptionText: clonePtr(in.DescriptionText), CreatedAt: in.Now, ContentUpdatedAt: in.Now})
		return nil
	})
}

type ScheduleFromPoolInput struct {
	ChainID     TaskChainID
	TraceID     DayTraceID
	DayID       DayID
	Date, Today LocalDate
	Now         Instant
	// One new identity for each planned subtask; no IDs are silently invented.
	SubtaskIDs map[PlannedSubtaskID]SubtaskID
}

func (k *TaskKernel) ScheduleFromPool(in ScheduleFromPoolInput) error {
	return k.mutate(func(g *TaskGraph) error {
		if !in.Date.Valid() || !in.Today.Valid() || !validID(string(in.TraceID)) {
			return ErrInvalidInput
		}
		if !in.Now.Finite() {
			return ErrInvalidClock
		}
		if in.Date.Compare(in.Today) < 0 {
			return ErrInvalidTransition
		}
		var chain *TaskChain
		for i := range g.Chains {
			if g.Chains[i].ID == in.ChainID {
				chain = &g.Chains[i]
				break
			}
		}
		if chain == nil {
			return ErrNotFound
		}
		if chain.State != ChainActive {
			return ErrChainAbandoned
		}
		if !g.inTaskPool(*chain) {
			return ErrInvalidTransition
		}
		dayExists := false
		for _, d := range g.Days {
			if d.Date == in.Date {
				dayExists = true
				if d.LockedAt != nil {
					return ErrImmutableHistory
				}
			}
		}
		var definition *TaskDefinition
		for i := range g.Definitions {
			d := &g.Definitions[i]
			if d.ChainID == in.ChainID && d.SupersededAt == nil {
				definition = d
				break
			}
		}
		if definition == nil {
			return ErrNotFound
		}
		nextPriority := 0
		hasPriority := false
		for _, t := range g.Traces {
			if t.ID == in.TraceID {
				return ErrInvalidInput
			}
			if t.Date == in.Date && t.Status.FormsDayHistory() && (!hasPriority || t.Priority > nextPriority) {
				nextPriority = t.Priority
				hasPriority = true
			}
		}
		if nextPriority == int(^uint(0)>>1) {
			return ErrInvalidInput
		}
		nextPriority++
		if len(in.SubtaskIDs) != len(definition.PlannedSubtasks) {
			return ErrInvalidInput
		}
		var notes []TaskNoteEntry
		for _, n := range chain.NoteEntries {
			if n.DeletedAt == nil {
				notes = append(notes, cloneNote(n))
			}
		}
		trace := DayTrace{ID: in.TraceID, ChainID: chain.ID, DefinitionID: definition.ID, Date: in.Date, Status: TracePending, Priority: nextPriority, DescriptionText: clonePtr(definition.DescriptionText), NoteEntries: notes, CreatedAt: in.Now, ContentUpdatedAt: in.Now}
		for _, p := range definition.PlannedSubtasks {
			id, ok := in.SubtaskIDs[p.ID]
			if !ok || !validID(string(id)) {
				return ErrInvalidInput
			}
			g.Subtasks = append(g.Subtasks, Subtask{ID: id, LineageID: p.LineageID, TraceID: trace.ID, Title: p.Title, Status: SubtaskPending, Difficulty: p.Difficulty, Position: p.Position, CreatedAt: in.Now, UpdatedAt: in.Now})
		}
		g.Traces = append(g.Traces, trace)
		if !dayExists {
			if !validID(string(in.DayID)) {
				return ErrInvalidInput
			}
			g.Days = append(g.Days, Day{ID: in.DayID, Date: in.Date, CreatedAt: in.Now, UpdatedAt: in.Now})
		}
		return nil
	})
}
func (g TaskGraph) inTaskPool(c TaskChain) bool {
	if c.State != ChainActive || c.CycleMembership != nil {
		return false
	}
	var latest *DayTrace
	for i := range g.Traces {
		t := &g.Traces[i]
		if t.ChainID != c.ID {
			continue
		}
		if t.Status == TracePending {
			return false
		}
		if latest == nil || traceLater(*t, *latest) {
			latest = t
		}
	}
	return latest == nil || latest.Status == TraceReturnedToPool || latest.Status == TraceCancelledDraft
}
func traceLater(a, b DayTrace) bool {
	if !a.ContentUpdatedAt.Equal(b.ContentUpdatedAt) {
		return a.ContentUpdatedAt.After(b.ContentUpdatedAt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID > b.ID
}

func (k *TaskKernel) MarkCompleted(id DayTraceID, today LocalDate, now Instant) error {
	return k.completion(id, today, now, false)
}
func (k *TaskKernel) UndoCompleted(id DayTraceID, today LocalDate, now Instant) error {
	return k.completion(id, today, now, true)
}
func (k *TaskKernel) completion(id DayTraceID, today LocalDate, now Instant, undo bool) error {
	return k.mutate(func(g *TaskGraph) error {
		if !today.Valid() {
			return ErrInvalidInput
		}
		if !now.Finite() {
			return ErrInvalidClock
		}
		var trace *DayTrace
		for i := range g.Traces {
			if g.Traces[i].ID == id {
				trace = &g.Traces[i]
				break
			}
		}
		if trace == nil {
			return ErrNotFound
		}
		for _, d := range g.Days {
			if d.Date == trace.Date && d.LockedAt != nil {
				return ErrImmutableHistory
			}
		}
		if trace.Date != today {
			if undo {
				return ErrHistoricalCompletionCannotBeUndone
			}
			if trace.Date.Compare(today) < 0 {
				return ErrImmutableHistory
			}
			return ErrFuturePlanCannotComplete
		}
		required := TracePending
		if undo {
			required = TraceCompleted
		}
		if trace.Status != required {
			return ErrInvalidTransition
		}
		if !undo {
			for _, s := range g.Subtasks {
				if s.TraceID == id && (s.Status == SubtaskPending || s.Status == SubtaskUnfinished) {
					return ErrOpenSubtasksPreventCompletion
				}
			}
		}
		if now.Before(trace.ContentUpdatedAt) {
			return ErrInvalidClock
		}
		var chain *TaskChain
		for i := range g.Chains {
			if g.Chains[i].ID == trace.ChainID {
				chain = &g.Chains[i]
				break
			}
		}
		if chain == nil {
			return ErrNotFound
		}
		if now.Before(chain.UpdatedAt) {
			return ErrInvalidClock
		}
		trace.ContentUpdatedAt = now
		trace.PinOrder = nil
		if undo {
			trace.Status = TracePending
			trace.CompletedAt = nil
		} else {
			trace.Status = TraceCompleted
			trace.CompletedAt = clonePtr(&now)
		}
		chain.UpdatedAt = now
		return nil
	})
}

// SettleTaskDays only settles task facts. The caller integrating the full engine
// must also capture classification snapshots before committing the same state.
func (k *TaskKernel) SettleTaskDays(today LocalDate, now Instant) error {
	return k.mutate(func(g *TaskGraph) error {
		if !today.Valid() {
			return ErrInvalidInput
		}
		if !now.Finite() {
			return ErrInvalidClock
		}
		dates := map[LocalDate]bool{}
		for _, t := range g.Traces {
			if t.Status.FormsDayHistory() && t.Date.Compare(today) < 0 {
				dates[t.Date] = true
			}
		}
		for i := range g.Days {
			day := &g.Days[i]
			if !dates[day.Date] || day.LockedAt != nil {
				continue
			}
			if now.Before(day.UpdatedAt) {
				return ErrInvalidClock
			}
			for j := range g.Traces {
				t := &g.Traces[j]
				if t.Date != day.Date || t.Status != TracePending {
					continue
				}
				if now.Before(t.ContentUpdatedAt) {
					return ErrInvalidClock
				}
				t.Status = TraceUnfinished
				t.PinOrder = nil
				t.SettledAt = clonePtr(&now)
				t.ContentUpdatedAt = now
				for z := range g.Subtasks {
					s := &g.Subtasks[z]
					if s.TraceID != t.ID || s.Status != SubtaskPending {
						continue
					}
					if now.Before(s.UpdatedAt) {
						return ErrInvalidClock
					}
					s.Status = SubtaskUnfinished
					s.SettledAt = clonePtr(&now)
					s.UpdatedAt = now
				}
			}
			day.LockedAt = clonePtr(&now)
			day.UpdatedAt = now
		}
		return nil
	})
}

// DayTodo excludes movement-source rows and internal cancellation facts. Default
// priority presentation puts pending rows first, with pinned pending rows first.
func (k *TaskKernel) DayTodo(date LocalDate) []DayTrace {
	out := func() []DayTrace {
		k.mu.RLock()
		defer k.mu.RUnlock()
		newestByChain := map[TaskChainID]Instant{}
		for _, t := range k.graph.Traces {
			if t.Date == date && t.Status.FormsDayHistory() {
				old, exists := newestByChain[t.ChainID]
				if !exists || t.CreatedAt.After(old) {
					newestByChain[t.ChainID] = t.CreatedAt
				}
			}
		}
		var out []DayTrace
		for _, t := range k.graph.Traces {
			if t.Date != date || !t.Status.VisibleInDayTodo() {
				continue
			}
			if t.Status == TraceDeferred && newestByChain[t.ChainID].After(t.CreatedAt) {
				continue
			}
			out = append(out, cloneDayTrace(t))
		}
		return out
	}()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Status == TracePending) != (b.Status == TracePending) {
			return a.Status == TracePending
		}
		if a.Status == TracePending && b.Status == TracePending {
			if (a.PinOrder != nil) != (b.PinOrder != nil) {
				return a.PinOrder != nil
			}
			if a.PinOrder != nil && b.PinOrder != nil && *a.PinOrder != *b.PinOrder {
				return *a.PinOrder < *b.PinOrder
			}
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return out
}
