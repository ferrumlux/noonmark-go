// SPDX-License-Identifier: AGPL-3.0-only
package core

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func cloneNote(n TaskNoteEntry) TaskNoteEntry { n.DeletedAt = clonePtr(n.DeletedAt); return n }
func cloneNotes(ns []TaskNoteEntry) []TaskNoteEntry {
	if ns == nil {
		return nil
	}
	out := make([]TaskNoteEntry, len(ns))
	for i, n := range ns {
		out[i] = cloneNote(n)
	}
	return out
}
func cloneDayTrace(t DayTrace) DayTrace {
	t.PinOrder = clonePtr(t.PinOrder)
	t.DescriptionText = clonePtr(t.DescriptionText)
	t.NoteEntries = cloneNotes(t.NoteEntries)
	t.ManualProgressPercent = clonePtr(t.ManualProgressPercent)
	t.CarriedFromTraceID = clonePtr(t.CarriedFromTraceID)
	t.ChangedToTraceID = clonePtr(t.ChangedToTraceID)
	t.CompletedAt = clonePtr(t.CompletedAt)
	t.SettledAt = clonePtr(t.SettledAt)
	t.DraftCancellationID = clonePtr(t.DraftCancellationID)
	t.DraftCancelledOn = clonePtr(t.DraftCancelledOn)
	return t
}
func (g TaskGraph) Clone() TaskGraph {
	out := TaskGraph{}
	if g.Days != nil {
		out.Days = make([]Day, len(g.Days))
	}
	for i, d := range g.Days {
		d.LockedAt = clonePtr(d.LockedAt)
		d.ReviewSummary = clonePtr(d.ReviewSummary)
		d.ReviewUnfinishedReason = clonePtr(d.ReviewUnfinishedReason)
		d.ReviewTomorrowNote = clonePtr(d.ReviewTomorrowNote)
		out.Days[i] = d
	}
	if g.Chains != nil {
		out.Chains = make([]TaskChain, len(g.Chains))
	}
	for i, c := range g.Chains {
		c.CycleMembership = clonePtr(c.CycleMembership)
		c.NoteEntries = cloneNotes(c.NoteEntries)
		out.Chains[i] = c
	}
	if g.Definitions != nil {
		out.Definitions = make([]TaskDefinition, len(g.Definitions))
	}
	for i, d := range g.Definitions {
		d.DescriptionText = clonePtr(d.DescriptionText)
		d.SupersededAt = clonePtr(d.SupersededAt)
		d.SupersededByDefinitionID = clonePtr(d.SupersededByDefinitionID)
		if d.PlannedSubtasks != nil {
			d.PlannedSubtasks = append([]PlannedSubtask{}, d.PlannedSubtasks...)
		}
		out.Definitions[i] = d
	}
	if g.Traces != nil {
		out.Traces = make([]DayTrace, len(g.Traces))
	}
	for i, t := range g.Traces {
		out.Traces[i] = cloneDayTrace(t)
	}
	if g.Subtasks != nil {
		out.Subtasks = make([]Subtask, len(g.Subtasks))
	}
	for i, s := range g.Subtasks {
		s.CarriedFromSubtaskID = clonePtr(s.CarriedFromSubtaskID)
		s.CompletedAt = clonePtr(s.CompletedAt)
		s.SettledAt = clonePtr(s.SettledAt)
		s.DraftCancellationID = clonePtr(s.DraftCancellationID)
		out.Subtasks[i] = s
	}
	return out
}
