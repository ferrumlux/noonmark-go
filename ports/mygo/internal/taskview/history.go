// SPDX-License-Identifier: AGPL-3.0-only
package taskview

import (
	"fmt"
	"sort"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// TitleCompare must be a pure, total preorder, returning negative/zero/positive.
// Inject the presentation locale's localized-standard comparator. This package
// deliberately does not claim byte ordering is Swift localizedStandardCompare.
type TitleCompare func(string, string) int

// HistoryProjection owns an immutable validated snapshot. Accessors return new
// detached values, safe to edit without changing the source or later queries.
// It selects ordinary chains only; omitted snapshot supplements are not validated.
type HistoryProjection struct {
	graph   core.TaskGraph
	compare TitleCompare
}

func NewHistoryProjection(graph core.TaskGraph, compare TitleCompare) (*HistoryProjection, error) {
	if compare == nil {
		return nil, fmt.Errorf("history projection: title comparator is required")
	}
	if err := core.ValidateTaskGraph(graph); err != nil {
		return nil, fmt.Errorf("history graph: %w", err)
	}
	return &HistoryProjection{graph: graph.Clone(), compare: compare}, nil
}

type CompletedSubtaskRecord struct {
	Date             core.LocalDate
	Subtask          core.Subtask
	ParentTrace      core.DayTrace
	ParentDefinition core.TaskDefinition
}
type SubtaskTrajectoryRecord struct {
	Date    core.LocalDate
	Subtask core.Subtask
}
type SubtaskTrajectory struct {
	LineageID      core.SubtaskLineageID
	Title          string
	StartDate      core.LocalDate
	ContinuedDates []core.LocalDate
	CompletedDate  *core.LocalDate
	Records        []SubtaskTrajectoryRecord
}
type CompletedTaskTrajectory struct {
	StartDate           core.LocalDate
	ContinuedDates      []core.LocalDate
	CompletedDate       core.LocalDate
	Traces              []core.DayTrace
	SubtaskTrajectories []SubtaskTrajectory
}
type CompletedPoolItem struct {
	Trace      core.DayTrace
	Definition core.TaskDefinition
	Trajectory CompletedTaskTrajectory
}
type CompletedTaskHierarchy struct {
	Chain              core.TaskChain
	Definition         core.TaskDefinition
	ParentCompletion   *CompletedPoolItem
	CompletedChildren  []CompletedSubtaskRecord
	LatestCompletionAt core.Instant
}

type UnfinishedActionKind string

const (
	ContinueHistoryTrace      UnfinishedActionKind = "continueTrace"
	AbandonHistoryChain       UnfinishedActionKind = "abandonChain"
	ReactivateHistoryChain    UnfinishedActionKind = "reactivateChain"
	OpenHistoryTaskPool       UnfinishedActionKind = "openTaskPool"
	AbandonHistoryPooledChain UnfinishedActionKind = "abandonPooledChain"
)

// ActionPlan describes source intent, not an assertion that all commands exist.
type UnfinishedAction struct {
	Kind    UnfinishedActionKind
	TraceID core.DayTraceID
	ChainID core.TaskChainID
}
type UnfinishedPoolItem struct {
	Chain            core.TaskChain
	Definition       core.TaskDefinition
	UnfinishedTraces []core.DayTrace
	ActiveTrace      *core.DayTrace
	IsInTaskPool     bool
}

func (i UnfinishedPoolItem) IsContinuedPending() bool {
	return i.ActiveTrace != nil && i.ActiveTrace.Status == core.TracePending
}

// Presentation metadata follows UnfinishedPoolPage.swift:105–121, including
// its max(sequence)-1 convention rather than counting history rows.
func (i UnfinishedPoolItem) LatestUnfinishedTrace() *core.DayTrace {
	if len(i.UnfinishedTraces) == 0 {
		return nil
	}
	g := (core.TaskGraph{Traces: []core.DayTrace{i.UnfinishedTraces[len(i.UnfinishedTraces)-1]}}).Clone()
	return &g.Traces[0]
}
func (i UnfinishedPoolItem) IsAbandoned() bool {
	latest := i.LatestUnfinishedTrace()
	return i.Chain.State == core.ChainAbandoned || (latest != nil && latest.Status == core.TraceAbandoned)
}
func (i UnfinishedPoolItem) ContinuationCount() int {
	latest := 0
	for _, t := range i.UnfinishedTraces {
		if t.ContinuationSeq > latest {
			latest = t.ContinuationSeq
		}
	}
	if i.ActiveTrace != nil && i.ActiveTrace.ContinuationSeq > latest {
		latest = i.ActiveTrace.ContinuationSeq
	}
	if latest > 1 {
		return latest - 1
	}
	return 0
}
func (i UnfinishedPoolItem) ActionPlan() []UnfinishedAction {
	if i.Chain.State == core.ChainAbandoned {
		for n := len(i.UnfinishedTraces) - 1; n >= 0; n-- {
			t := i.UnfinishedTraces[n]
			if t.Status == core.TraceAbandoned {
				return []UnfinishedAction{{Kind: ReactivateHistoryChain, TraceID: t.ID}}
			}
		}
		return nil
	}
	if i.ActiveTrace != nil {
		return []UnfinishedAction{{Kind: AbandonHistoryChain, TraceID: i.ActiveTrace.ID}}
	}
	if i.IsInTaskPool {
		return []UnfinishedAction{{Kind: OpenHistoryTaskPool, ChainID: i.Chain.ID}, {Kind: AbandonHistoryPooledChain, ChainID: i.Chain.ID}}
	}
	for n := len(i.UnfinishedTraces) - 1; n >= 0; n-- {
		t := i.UnfinishedTraces[n]
		if t.Status == core.TraceUnfinished {
			return []UnfinishedAction{{Kind: ContinueHistoryTrace, TraceID: t.ID}, {Kind: AbandonHistoryChain, TraceID: t.ID}}
		}
	}
	return nil
}

type historyIndex struct {
	chains      map[core.TaskChainID]core.TaskChain
	definitions map[core.TaskDefinitionID]core.TaskDefinition
	current     map[core.TaskChainID]core.TaskDefinition
	traces      map[core.DayTraceID]core.DayTrace
	byChain     map[core.TaskChainID][]core.DayTrace
	children    map[core.DayTraceID][]core.Subtask
}

func indexHistory(g core.TaskGraph) historyIndex {
	x := historyIndex{map[core.TaskChainID]core.TaskChain{}, map[core.TaskDefinitionID]core.TaskDefinition{}, map[core.TaskChainID]core.TaskDefinition{}, map[core.DayTraceID]core.DayTrace{}, map[core.TaskChainID][]core.DayTrace{}, map[core.DayTraceID][]core.Subtask{}}
	for _, c := range g.Chains {
		if c.CycleMembership == nil {
			x.chains[c.ID] = c
		}
	}
	for _, d := range g.Definitions {
		x.definitions[d.ID] = d
		if d.SupersededAt == nil {
			x.current[d.ChainID] = d
		}
	}
	for _, t := range g.Traces {
		x.traces[t.ID] = t
		x.byChain[t.ChainID] = append(x.byChain[t.ChainID], t)
	}
	for _, s := range g.Subtasks {
		if s.Status != core.SubtaskCancelledDraft {
			x.children[s.TraceID] = append(x.children[s.TraceID], s)
		}
	}
	return x
}
func historyTraceBefore(a, b core.DayTrace) bool {
	if a.Date != b.Date {
		return a.Date.Compare(b.Date) < 0
	}
	if a.ContinuationSeq != b.ContinuationSeq {
		return a.ContinuationSeq < b.ContinuationSeq
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}
func parentMoment(t core.DayTrace) core.Instant {
	if t.CompletedAt != nil {
		return *t.CompletedAt
	}
	return t.CreatedAt
}
func childMoment(s core.Subtask) core.Instant {
	if s.CompletedAt != nil {
		return *s.CompletedAt
	}
	return s.UpdatedAt
}
func childBefore(a, b core.Subtask) bool {
	if a.Position != b.Position {
		return a.Position < b.Position
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}
func (x historyIndex) continued(t core.DayTrace) bool {
	return t.CarriedFromTraceID != nil && x.traces[*t.CarriedFromTraceID].Status == core.TraceUnfinished
}
func appendHistoryDate(ds []core.LocalDate, d core.LocalDate) []core.LocalDate {
	for _, v := range ds {
		if v == d {
			return ds
		}
	}
	return append(ds, d)
}
func (p *HistoryProjection) trajectory(x historyIndex, completion core.DayTrace) CompletedTaskTrajectory {
	out := CompletedTaskTrajectory{StartDate: completion.Date, CompletedDate: completion.Date}
	for _, t := range x.byChain[completion.ChainID] {
		if t.Status.FormsDayHistory() {
			out.Traces = append(out.Traces, t)
		}
	}
	sort.SliceStable(out.Traces, func(i, j int) bool { return historyTraceBefore(out.Traces[i], out.Traces[j]) })
	if len(out.Traces) > 0 {
		out.StartDate = out.Traces[0].Date
	}
	groups := map[core.SubtaskLineageID][]SubtaskTrajectoryRecord{}
	for _, t := range out.Traces {
		if x.continued(t) {
			out.ContinuedDates = appendHistoryDate(out.ContinuedDates, t.Date)
		}
		for _, s := range x.children[t.ID] {
			groups[s.LineageID] = append(groups[s.LineageID], SubtaskTrajectoryRecord{Date: t.Date, Subtask: s})
		}
	}
	for lineage, records := range groups {
		sort.SliceStable(records, func(i, j int) bool {
			a, b := records[i], records[j]
			if a.Date != b.Date {
				return a.Date.Compare(b.Date) < 0
			}
			if a.Subtask.Position != b.Subtask.Position {
				return a.Subtask.Position < b.Subtask.Position
			}
			return a.Subtask.CreatedAt.Before(b.Subtask.CreatedAt)
		})
		st := SubtaskTrajectory{LineageID: lineage, Title: records[0].Subtask.Title, StartDate: records[0].Date, Records: records}
		for _, r := range records {
			if st.CompletedDate == nil && r.Subtask.Status == core.SubtaskCompleted {
				d := r.Date
				st.CompletedDate = &d
			}
			if r.Subtask.CarriedFromSubtaskID != nil && x.continued(x.traces[r.Subtask.TraceID]) {
				st.ContinuedDates = appendHistoryDate(st.ContinuedDates, r.Date)
			}
		}
		out.SubtaskTrajectories = append(out.SubtaskTrajectories, st)
	}
	sort.SliceStable(out.SubtaskTrajectories, func(i, j int) bool {
		a, b := out.SubtaskTrajectories[i], out.SubtaskTrajectories[j]
		if a.StartDate != b.StartDate {
			return a.StartDate.Compare(b.StartDate) < 0
		}
		if c := p.compare(a.Title, b.Title); c != 0 {
			return c < 0
		}
		return false // Swift leaves equal date/title ties unspecified.
	})
	return out
}

// CompletedTaskHierarchies follows NoonmarkEngine.swift:1877 and 4020.
func (p *HistoryProjection) CompletedTaskHierarchies() []CompletedTaskHierarchy {
	x := indexHistory(p.graph.Clone())
	out := make([]CompletedTaskHierarchy, 0)
	for id, chain := range x.chains {
		h := CompletedTaskHierarchy{Chain: chain, Definition: x.current[id]}
		children := map[core.SubtaskLineageID]CompletedSubtaskRecord{}
		for _, t := range x.byChain[id] {
			if t.Status == core.TraceCompleted && (h.ParentCompletion == nil || parentMoment(t).After(parentMoment(h.ParentCompletion.Trace))) {
				h.ParentCompletion = &CompletedPoolItem{Trace: t, Definition: x.definitions[t.DefinitionID]}
			}
			if !t.Status.FormsDayHistory() {
				continue
			}
			for _, s := range x.children[t.ID] {
				if s.Status != core.SubtaskCompleted {
					continue
				}
				old, ok := children[s.LineageID]
				if !ok || childMoment(s).After(childMoment(old.Subtask)) || (childMoment(s).Equal(childMoment(old.Subtask)) && s.ID > old.Subtask.ID) {
					children[s.LineageID] = CompletedSubtaskRecord{Date: t.Date, Subtask: s, ParentTrace: t, ParentDefinition: x.definitions[t.DefinitionID]}
				}
			}
		}
		if h.ParentCompletion == nil && len(children) == 0 {
			continue
		}
		if h.ParentCompletion != nil {
			h.Definition = h.ParentCompletion.Definition
			h.ParentCompletion.Trajectory = p.trajectory(x, h.ParentCompletion.Trace)
			h.LatestCompletionAt = parentMoment(h.ParentCompletion.Trace)
		}
		hasMoment := h.ParentCompletion != nil
		for _, r := range children {
			h.CompletedChildren = append(h.CompletedChildren, r)
			if m := childMoment(r.Subtask); !hasMoment || m.After(h.LatestCompletionAt) {
				h.LatestCompletionAt = m
				hasMoment = true
			}
		}
		sort.SliceStable(h.CompletedChildren, func(i, j int) bool {
			return childBefore(h.CompletedChildren[i].Subtask, h.CompletedChildren[j].Subtask)
		})
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.LatestCompletionAt.Equal(b.LatestCompletionAt) {
			return a.LatestCompletionAt.After(b.LatestCompletionAt)
		}
		if c := p.compare(a.Definition.Title, b.Definition.Title); c != 0 {
			return c < 0
		}
		return a.Chain.ID < b.Chain.ID
	})
	return out
}

// UnfinishedPool follows NoonmarkEngine.swift:1750. A completion excludes the
// entire chain even when older unfinished or abandoned history remains.
func (p *HistoryProjection) UnfinishedPool() []UnfinishedPoolItem {
	x := indexHistory(p.graph.Clone())
	out := make([]UnfinishedPoolItem, 0)
	for id, c := range x.chains {
		if c.State != core.ChainActive && c.State != core.ChainAbandoned {
			continue
		}
		item := UnfinishedPoolItem{Chain: c, Definition: x.current[id]}
		completed := false
		var latest *core.DayTrace
		for _, t := range x.byChain[id] {
			if latest == nil || poolTraceLater(t, *latest) {
				v := t
				latest = &v
			}
			if t.Status == core.TracePending {
				v := t
				item.ActiveTrace = &v
			}
			if t.Status == core.TraceCompleted {
				completed = true
			}
			if t.Status == core.TraceUnfinished || t.Status == core.TraceAbandoned {
				item.UnfinishedTraces = append(item.UnfinishedTraces, t)
			}
		}
		if completed || len(item.UnfinishedTraces) == 0 {
			continue
		}
		item.Chain.UpdatedAt = item.UnfinishedTraces[0].CreatedAt
		for _, t := range item.UnfinishedTraces {
			if t.CreatedAt.After(item.Chain.UpdatedAt) {
				item.Chain.UpdatedAt = t.CreatedAt
			}
		}
		sort.SliceStable(item.UnfinishedTraces, func(i, j int) bool {
			a, b := item.UnfinishedTraces[i], item.UnfinishedTraces[j]
			if a.Date != b.Date {
				return a.Date.Compare(b.Date) < 0
			}
			return false // Source sorts unfinished details by date only.
		})
		item.IsInTaskPool = c.State == core.ChainActive && item.ActiveTrace == nil && (latest == nil || latest.Status == core.TraceReturnedToPool || latest.Status == core.TraceCancelledDraft)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.Chain.CreatedAt.Equal(b.Chain.CreatedAt) {
			return a.Chain.CreatedAt.Before(b.Chain.CreatedAt)
		}
		if c := p.compare(a.Definition.Title, b.Definition.Title); c != 0 {
			return c < 0
		}
		return false // Source has no UUID tie-break here.
	})
	return out
}
