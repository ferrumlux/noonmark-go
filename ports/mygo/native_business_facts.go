package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

const nativeBusinessTaskTitle = "Native smoke task v1"
const nativeBusinessCategoryName = "Native smoke category v1"
const nativeBusinessCategoryColor = "#D87831"

// PersistentFactsV1 is the complete supported ordinary projection, not a v7
// wire codec. Instant.MarshalJSON retains the original uint64 IEEE bits.
// Do not normalize nil collections, omit fields or include runtime authority.
type PersistentFactsV1 struct {
	Version        int
	Graph          core.TaskGraph
	Classification ordinary.AggregateState
}

func nativeBusinessFactsJSON(v taskapp.OrdinaryReadView) (string, string, error) {
	b, err := json.Marshal(PersistentFactsV1{1, v.Graph, v.Classification})
	if err != nil {
		return "", "", err
	}
	if len(b) > 48*1024 {
		return "", "", fmt.Errorf("business facts exceed bounded payload")
	}
	h := sha256.Sum256(b)
	return string(b), hex.EncodeToString(h[:]), nil
}
func nativeBusinessHealth(v taskapp.OrdinaryReadView, today core.LocalDate) error {
	if !today.Valid() || v.Today != today {
		return fmt.Errorf("date-boundary-not-covered")
	}
	if v.Health != (taskapp.SessionHealth{Session: taskapp.SessionWritable, Reason: taskapp.HealthOK, FailureStage: taskapp.StageNone}) {
		return fmt.Errorf("business read health is not writable")
	}
	return nil
}
func nativeBusinessEmptyGraph() core.TaskGraph {
	return core.TaskGraph{Days: []core.Day{}, Chains: []core.TaskChain{}, Definitions: []core.TaskDefinition{}, Traces: []core.DayTrace{}, Subtasks: []core.Subtask{}}
}
func nativeBusinessEmptyClassification() ordinary.AggregateState {
	return ordinary.AggregateState{FormatVersion: 1, Categories: map[ordinary.CategoryID]ordinary.Category{}, CurrentByChainID: map[core.TaskChainID]ordinary.Current{}, RelationHistory: []ordinary.RelationHistory{}, ChangeRecords: []ordinary.AggregateChangeRecord{}, CommittedReceiptsByInteractionID: map[ordinary.UUID]ordinary.Receipt{}, SnapshotsByTraceID: map[core.DayTraceID]ordinary.TraceClassificationSnapshot{}, SnapshotEventsByTraceID: map[core.DayTraceID][]ordinary.TraceClassificationSnapshot{}}
}
func validateNativeBusinessEmpty(v taskapp.OrdinaryReadView, today core.LocalDate) error {
	if err := nativeBusinessHealth(v, today); err != nil {
		return err
	}
	if !reflect.DeepEqual(v.Graph, nativeBusinessEmptyGraph()) || !reflect.DeepEqual(v.Classification, nativeBusinessEmptyClassification()) {
		return fmt.Errorf("business initial facts are not the exact empty fixture")
	}
	return nil
}
func validateNativeBusinessTask(v taskapp.OrdinaryReadView, today core.LocalDate) (core.TaskChainID, error) {
	if err := nativeBusinessHealth(v, today); err != nil {
		return "", err
	}
	g := v.Graph
	if len(g.Chains) != 1 || len(g.Definitions) != 1 || len(g.Traces) != 1 || len(g.Days) != 1 {
		return "", fmt.Errorf("business task cardinality")
	}
	if err := core.ValidateTaskGraph(g); err != nil {
		return "", err
	}
	c, d, t, day := g.Chains[0], g.Definitions[0], g.Traces[0], g.Days[0]
	now := c.CreatedAt
	want := nativeBusinessEmptyGraph()
	want.Chains = []core.TaskChain{{ID: c.ID, State: core.ChainActive, NoteEntries: []core.TaskNoteEntry{}, CreatedAt: now, UpdatedAt: now}}
	want.Definitions = []core.TaskDefinition{{ID: d.ID, ChainID: c.ID, Sequence: 1, Title: nativeBusinessTaskTitle, PlannedSubtasks: []core.PlannedSubtask{}, CreatedAt: now, ContentUpdatedAt: now}}
	want.Traces = []core.DayTrace{{ID: t.ID, ChainID: c.ID, DefinitionID: d.ID, Date: today, Status: core.TracePending, Priority: 1, NoteEntries: []core.TaskNoteEntry{}, CreatedAt: now, ContentUpdatedAt: now}}
	want.Days = []core.Day{{ID: day.ID, Date: today, CreatedAt: now, UpdatedAt: now}}
	if !reflect.DeepEqual(g, want) {
		return "", fmt.Errorf("business task differs from fixed full-field fixture")
	}
	return c.ID, nil
}
func validateNativeBusinessFinal(v taskapp.OrdinaryReadView, today core.LocalDate) (core.TaskChainID, error) {
	chain, err := validateNativeBusinessTask(v, today)
	if err != nil {
		return "", err
	}
	a := v.Classification
	if err = ordinary.ValidateAggregate(a, ordinary.DefaultLimits()); err != nil {
		return "", err
	}
	if err = ordinary.ValidateAggregateReferences(a, v.Graph, ordinary.DefaultLimits()); err != nil {
		return "", err
	}
	if a.Revision != 1 || len(a.Categories) != 1 || len(a.CurrentByChainID) != 1 || len(a.ChangeRecords) != 1 || len(a.CommittedReceiptsByInteractionID) != 1 {
		return "", fmt.Errorf("business category cardinality/revision")
	}
	r := a.ChangeRecords[0]
	current, ok := a.CurrentByChainID[chain]
	if !ok || current.Category == nil {
		return "", fmt.Errorf("business current relation missing")
	}
	cat, ok := a.Categories[current.Category.CategoryID]
	if !ok || len(cat.NameVersions) != 1 {
		return "", fmt.Errorf("business category/name version missing")
	}
	key, err := canonical16.CanonicalKey(nativeBusinessCategoryName)
	if err != nil {
		return "", err
	}
	// Validate existing persistent IDs and bytes only. In particular cold Open
	// does not create a domain plan, Service plan, confirmation, or interaction.
	if nativeBusinessPlanDigest(chain, cat.ID, r.PlanID, r.InteractionID) != r.PlanDigest {
		return "", fmt.Errorf("business fixed plan digest mismatch")
	}
	now := r.CommittedAt
	if !now.Finite() || !now.After(v.Graph.Chains[0].CreatedAt) {
		return "", fmt.Errorf("business category clock")
	}
	want := nativeBusinessEmptyClassification()
	want.Revision = 1
	want.Categories[cat.ID] = ordinary.Category{ID: cat.ID, Name: nativeBusinessCategoryName, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, ColorHex: nativeBusinessCategoryColor, PresentationApproval: "userApproved", Lifecycle: "active", NameVersions: []ordinary.NameVersion{{ID: cat.NameVersions[0].ID, Name: nativeBusinessCategoryName, CanonicalKey: key, CanonicalKeyVersion: canonical16.AlgorithmVersion, ValidFrom: now}}, CreatedAt: now, UpdatedAt: now}
	want.CurrentByChainID[chain] = ordinary.Current{Category: &ordinary.CategoryRelation{CategoryID: cat.ID, Source: ordinary.UserDirect, DecisionID: r.DecisionID, CreatedAt: now, UpdatedAt: now, Revision: 1}}
	id := string(cat.ID)
	changes := []ordinary.AggregatePlanChange{{Kind: "setCurrent", ChainID: chain, Before: ordinary.AggregateSelectionPreview{}, After: ordinary.AggregateSelectionPreview{Category: &ordinary.AggregatePlanItem{ID: &id, Kind: "category", Name: nativeBusinessCategoryName, ColorHex: nativeBusinessCategoryColor, Resolution: "new"}}}}
	want.ChangeRecords = []ordinary.AggregateChangeRecord{{ID: r.ID, PlanID: r.PlanID, InteractionID: r.InteractionID, Source: ordinary.UserDirect, DecisionID: r.DecisionID, Changes: changes, Notices: []ordinary.Notice{}, CommittedAt: now, Revision: 1, PlanDigest: r.PlanDigest, IntegrityDigest: r.IntegrityDigest}}
	want.CommittedReceiptsByInteractionID[r.InteractionID] = ordinary.Receipt{PlanID: r.PlanID, Revision: 1, Notices: []ordinary.Notice{}, ChangeRecordID: r.ID, DecisionID: r.DecisionID, ChangeRecordIntegrityDigest: r.IntegrityDigest}
	if !reflect.DeepEqual(a, want) {
		return "", fmt.Errorf("business classification differs from fixed full-field fixture")
	}
	return chain, nil
}

// Fixed ASCII-only digest payload from ordinary/digest.go. Call only after
// ValidateAggregate/References validate the UUID inputs. This is verification
// of stored facts, not command preparation; it constructs no PlanData. The
// independent Python collector recomputes the same domain digest separately.
func nativeBusinessPlanDigest(chain core.TaskChainID, category ordinary.CategoryID, plan, interaction ordinary.UUID) string {
	payload := fmt.Sprintf(`{"baseRevision":0,"blockers":[],"changes":[{"setCurrent":{"after":{"category":{"colorHex":%q,"id":%q,"kind":"category","name":%q,"resolution":"new"},"labels":[]},"before":{"labels":[]},"chainID":{"rawValue":%q}}}],"formatVersion":1,"interactionID":%q,"notices":[],"planID":%q,"plannedCategoryID":{"rawValue":%q},"plannedLabelIDs":[],"source":{"userDirect":{}}}`, nativeBusinessCategoryColor, category, nativeBusinessCategoryName, chain, interaction, plan, category)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
