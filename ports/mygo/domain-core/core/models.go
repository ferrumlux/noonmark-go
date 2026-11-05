// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"crypto/rand"
	"fmt"
)

type DayID string
type TaskChainID string
type TaskDefinitionID string
type DayTraceID string
type TaskNoteEntryID string
type PlannedSubtaskID string
type SubtaskID string
type SubtaskLineageID string
type TaskCycleSeriesID string
type CancellationID string

// NewIdentity returns a canonical uppercase UUID v4. Commands also accept
// explicit IDs so fixture replay and differential tests are deterministic.
func NewIdentity() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%08X-%04X-%04X-%04X-%012X", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func validID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

type TaskChainState string

const (
	ChainActive    TaskChainState = "active"
	ChainAbandoned TaskChainState = "abandoned"
)

type TraceStatus string

const (
	TracePending        TraceStatus = "pending"
	TraceCompleted      TraceStatus = "completed"
	TraceUnfinished     TraceStatus = "unfinished"
	TraceDeferred       TraceStatus = "deferred"
	TraceChanged        TraceStatus = "changed"
	TraceReturnedToPool TraceStatus = "returnedToPool"
	TraceCancelledDraft TraceStatus = "cancelledDraft"
	TraceAbandoned      TraceStatus = "abandoned"
)

func (s TraceStatus) Valid() bool {
	switch s {
	case TracePending, TraceCompleted, TraceUnfinished, TraceDeferred, TraceChanged, TraceReturnedToPool, TraceCancelledDraft, TraceAbandoned:
		return true
	}
	return false
}
func (s TraceStatus) FormsDayHistory() bool { return s.Valid() && s != TraceCancelledDraft }
func (s TraceStatus) VisibleInDayTodo() bool {
	switch s {
	case TracePending, TraceCompleted, TraceUnfinished, TraceDeferred, TraceAbandoned:
		return true
	}
	return false
}

type SubtaskStatus string

const (
	SubtaskPending        SubtaskStatus = "pending"
	SubtaskCompleted      SubtaskStatus = "completed"
	SubtaskUnfinished     SubtaskStatus = "unfinished"
	SubtaskDeferred       SubtaskStatus = "deferred"
	SubtaskAbandoned      SubtaskStatus = "abandoned"
	SubtaskCancelledDraft SubtaskStatus = "cancelledDraft"
)

func (s SubtaskStatus) Valid() bool {
	switch s {
	case SubtaskPending, SubtaskCompleted, SubtaskUnfinished, SubtaskDeferred, SubtaskAbandoned, SubtaskCancelledDraft:
		return true
	}
	return false
}

type Difficulty int

const (
	DifficultySimple Difficulty = 1
	DifficultyMedium Difficulty = 2
	DifficultyHard   Difficulty = 3
)

type Day struct {
	ID                                                        DayID
	Date                                                      LocalDate
	LockedAt                                                  *Instant
	ReviewSummary, ReviewUnfinishedReason, ReviewTomorrowNote *string
	CreatedAt, UpdatedAt                                      Instant
}
type TaskCycleMembership struct {
	SeriesID       TaskCycleSeriesID
	OccurrenceDate LocalDate
}
type TaskChain struct {
	ID                   TaskChainID
	State                TaskChainState
	CycleMembership      *TaskCycleMembership
	NoteEntries          []TaskNoteEntry
	CreatedAt, UpdatedAt Instant
}
type TaskNoteEntry struct {
	ID                   TaskNoteEntryID
	Body                 string
	CreatedAt, UpdatedAt Instant
	DeletedAt            *Instant
}
type PlannedSubtask struct {
	ID         PlannedSubtaskID
	LineageID  SubtaskLineageID
	Title      string
	Difficulty Difficulty
	Position   int
	CreatedAt  Instant
}
type TaskDefinition struct {
	ID                          TaskDefinitionID
	ChainID                     TaskChainID
	Sequence                    int
	Title                       string
	DescriptionText             *string
	PlannedSubtasks             []PlannedSubtask
	CreatedAt, ContentUpdatedAt Instant
	SupersededAt                *Instant
	SupersededByDefinitionID    *TaskDefinitionID
}
type DayTrace struct {
	ID                                   DayTraceID
	ChainID                              TaskChainID
	DefinitionID                         TaskDefinitionID
	Date                                 LocalDate
	Status                               TraceStatus
	Priority                             int
	PinOrder                             *int
	ContinuationSeq                      int
	DescriptionText                      *string
	NoteEntries                          []TaskNoteEntry
	ManualProgressPercent                *int
	CarriedFromTraceID, ChangedToTraceID *DayTraceID
	CreatedAt, ContentUpdatedAt          Instant
	CompletedAt, SettledAt               *Instant
	DraftCancellationID                  *CancellationID
	DraftCancelledOn                     *LocalDate
}
type Subtask struct {
	ID                     SubtaskID
	LineageID              SubtaskLineageID
	TraceID                DayTraceID
	Title                  string
	Status                 SubtaskStatus
	Difficulty             Difficulty
	Position               int
	CarriedFromSubtaskID   *SubtaskID
	CreatedAt, UpdatedAt   Instant
	CompletedAt, SettledAt *Instant
	DraftCancellationID    *CancellationID
}

// TaskGraph is a bounded, in-memory slice of the full NoonmarkSnapshot. It
// deliberately has NO import/export JSON codec. Classification state, repeat
// parents, ideas, preferences and sync facts belong to subsequent modules.
type TaskGraph struct {
	Days        []Day
	Chains      []TaskChain
	Definitions []TaskDefinition
	Traces      []DayTrace
	Subtasks    []Subtask
}
