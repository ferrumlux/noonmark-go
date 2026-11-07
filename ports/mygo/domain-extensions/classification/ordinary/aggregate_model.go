// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import "github.com/ferrumlux/noonmark/ports/mygo/domain/core"

// AggregateState is the ordinary-category source-state projection, independent
// of R4's fixed-graph State/Plan/proof. Identity strings are normalized uppercase
// UUIDs, except PlanItem.ID and RelationHistory.ItemID, which are raw Swift
// String values and must retain their original spelling. Codecs must retain
// original wire spelling of typed UUIDs separately. Unsupported wire
// containers must be rejected before projection, never silently discarded.
// Validation is not Service write authority or evidence of historical clicks.
type AggregateState struct {
	FormatVersion                    int
	Revision                         uint64
	Categories                       map[CategoryID]Category
	CurrentByChainID                 map[core.TaskChainID]Current
	RelationHistory                  []RelationHistory
	ChangeRecords                    []AggregateChangeRecord
	CommittedReceiptsByInteractionID map[UUID]Receipt
	SnapshotsByTraceID               map[core.DayTraceID]TraceClassificationSnapshot
	SnapshotEventsByTraceID          map[core.DayTraceID][]TraceClassificationSnapshot
}
type HistoricalCategoryValue struct {
	ID                                   CategoryID
	Name, ColorHex, PresentationApproval string
}
type HistoricalLabelValue struct {
	ID             UUID
	Name, ColorHex string
}
type TraceClassificationSnapshot struct {
	ID         UUID
	TraceID    core.DayTraceID
	Status     core.TraceStatus
	Category   *HistoricalCategoryValue
	Labels     []HistoricalLabelValue
	CapturedAt core.Instant
	Revision   uint64
}

func EmptyAggregateState() AggregateState { return AggregateState{FormatVersion: FormatVersion} }

// AggregatePlanItem retains Swift String? exactly. In particular nil, an empty
// string, and differently cased raw UUID-looking strings have distinct digest
// bytes. This is separate from the frozen R4 PlanItem command representation.
type AggregatePlanItem struct {
	ID                               *string
	Kind, Name, ColorHex, Resolution string
}
type AggregateSelectionPreview struct {
	Category *AggregatePlanItem
	Labels   []AggregatePlanItem
}
type AggregatePlanChange struct {
	Kind          string
	ChainID       core.TaskChainID
	Before, After AggregateSelectionPreview
}
type AggregateChangeRecord struct {
	ID, PlanID, InteractionID   UUID
	Source                      Source
	DecisionID                  UUID
	Changes                     []AggregatePlanChange
	Notices                     []Notice
	CommittedAt                 core.Instant
	Revision                    uint64
	PlanDigest, IntegrityDigest string
}
