// SPDX-License-Identifier: AGPL-3.0-only
// Derived from Noonmark 43e6eefaae3a04be89cab3e234d470111afd80ef.

// Package ordinary is an isolated, user-direct single-category command model.
// It is NOT product admission, a storage reader, or a proof of Swift wire parity.
// Imported integrity, entity decoding, and causal replay are separate checks.
package ordinary

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type UUID string
type CategoryID string
type Source string

const UserDirect Source = "userDirect"
const FormatVersion = 1

type ChoiceKind string

const (
	ChoiceNone     ChoiceKind = "none"
	ChoiceExisting ChoiceKind = "existing"
	ChoiceNew      ChoiceKind = "new"
)

// Choice is a strict tagged union. Inactive fields must be empty.
type Choice struct {
	Kind           ChoiceKind
	CategoryID     CategoryID
	Name, ColorHex string
}
type Request struct {
	ChainID       core.TaskChainID
	Choice        Choice
	InteractionID UUID
}

// IDs are explicit causal slots, never derived from names. Unneeded slots may
// be empty; supplied unused slots are ignored and are not domain facts.
type PrepareIDs struct {
	PlanID     UUID
	CategoryID CategoryID
}
type CommitIDs struct{ RecordID, NameVersionID, RemovalID UUID }

type NameVersion struct {
	ID                                      UUID
	Name, CanonicalKey, CanonicalKeyVersion string
	ValidFrom                               core.Instant
	ValidUntil                              *core.Instant
}
type Category struct {
	ID                                                CategoryID
	Name, CanonicalKey, CanonicalKeyVersion, ColorHex string
	PresentationApproval, Lifecycle                   string
	NameVersions                                      []NameVersion
	CreatedAt, UpdatedAt                              core.Instant
}
type CategoryRelation struct {
	CategoryID           CategoryID
	Source               Source
	DecisionID           UUID
	CreatedAt, UpdatedAt core.Instant
	Revision             uint64
}
type Current struct{ Category *CategoryRelation }
type RelationHistory struct {
	ID                  UUID
	Kind                string
	ChainID             core.TaskChainID
	ItemID              CategoryID
	OriginSource        Source
	OriginDecisionID    UUID
	CreatedAt           core.Instant
	CreatedRevision     uint64
	RemovedBySource     Source
	RemovedByDecisionID UUID
	RemovedAt           core.Instant
	RemovedRevision     uint64
}
type PlanItem struct {
	ID                               CategoryID
	Kind, Name, ColorHex, Resolution string
}
type SelectionPreview struct {
	Category *PlanItem
	Labels   []PlanItem
}
type PlanChange struct {
	Kind          string
	ChainID       core.TaskChainID
	Before, After SelectionPreview
}
type Notice struct {
	Kind, ItemKind, InputName, CurrentName string
	MatchedHistoricalAlias                 bool
}

// PlanData exposes the original plan fields for inspection and offline evidence.
// Intent is deliberately absent from the Foundation plan digest; Apply and
// Replay independently verify its prepared input and resulting changes.
type PlanData struct {
	ID                UUID
	BaseRevision      uint64
	InteractionID     UUID
	Intent            Request
	Source            Source
	Changes           []PlanChange
	CreatedItemID     UUID
	PlannedCategoryID CategoryID
	PlannedLabelIDs   []UUID
	Notices           []Notice
	Blockers          []string
	Digest            string
}
type ChangeRecord struct {
	ID, PlanID, InteractionID   UUID
	Source                      Source
	DecisionID                  UUID
	Changes                     []PlanChange
	Notices                     []Notice
	CommittedAt                 core.Instant
	Revision                    uint64
	PlanDigest, IntegrityDigest string
}
type Receipt struct {
	PlanID                      UUID
	Revision                    uint64
	Notices                     []Notice
	ChangeRecordID, DecisionID  UUID
	ChangeRecordIntegrityDigest string
}

// State deliberately cannot carry labels, history captures, management facts,
// AI or recurring supplements. The offline adapter must reject these on input.
type State struct {
	FormatVersion                    int
	Revision                         uint64
	Categories                       map[CategoryID]Category
	CurrentByChainID                 map[core.TaskChainID]Current
	RelationHistory                  []RelationHistory
	ChangeRecords                    []ChangeRecord
	CommittedReceiptsByInteractionID map[UUID]Receipt
}

func EmptyState() State { return State{FormatVersion: FormatVersion} }

type OrdinaryContext struct{ sealed *contextData }
type contextData struct {
	graph  core.TaskGraph
	chains map[core.TaskChainID]struct{}
	limits Limits
}

// Plan and Confirmation are sealed values. A zero value is never authority.
// Inspection returns independent copies, not mutable access to the seal.
type Plan struct{ sealed *preparedPlan }
type preparedPlan struct {
	context *contextData
	request Request
	ids     PrepareIDs
	data    PlanData
}
type Confirmation struct{ sealed *confirmationData }
type confirmationData struct {
	plan       *preparedPlan
	decisionID UUID
}

// CommittedStep retains complete prepare input and IDs, the plan, and committed
// output receipt. It is evidence to re-execute, not a declaration of validity.
// Exact retries retain the original plan and receipt; their Now is ignored.
type CommittedStep struct {
	Request    Request
	PrepareIDs PrepareIDs
	Plan       PlanData
	DecisionID UUID
	Now        core.Instant
	CommitIDs  CommitIDs
	Receipt    Receipt
}
type ReplayProof struct{ sealed *replayData }
type replayData struct {
	context *contextData
	steps   []CommittedStep
	state   State
}

var (
	ErrResourceLimit = errors.New("ordinary category resource budget exceeded")
	ErrInvalid       = errors.New("invalid ordinary category fact")
	ErrUnsupported   = errors.New("unsupported ordinary category scope")
	ErrContext       = errors.New("invalid or mismatched ordinary context")
	ErrStale         = errors.New("ordinary category plan is stale")
	ErrConflict      = errors.New("ordinary category identity or interaction conflict")
	ErrConfirmation  = errors.New("ordinary category confirmation does not bind this plan")
	ErrClock         = errors.New("invalid or backwards ordinary category clock")
	ErrName          = errors.New("invalid ordinary category name")
)
