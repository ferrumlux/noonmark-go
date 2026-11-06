// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

// Every logical string use is charged, including Kind and unused fields. This
// gate inspects metadata only: shape and UUID checks belong to core's order.
func historyCommandPlannedSubtaskInputBudget(in plannedSubtaskHistoryInput) error {
	b := historyCommandBudget{}
	if !b.raw(string(in.Kind)) || !b.raw(string(in.ChainID)) || !b.raw(string(in.ExpectedDefinitionID)) || !b.raw(string(in.SubtaskID)) || !b.raw(string(in.LineageID)) || !b.raw(in.Title) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	return nil
}
func historyCommandActualSubtaskInputBudget(in actualSubtaskHistoryInput) error {
	b := historyCommandBudget{}
	if !b.raw(string(in.Kind)) || !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) || !b.raw(string(in.SubtaskID)) || !b.raw(string(in.LineageID)) || !b.raw(in.Title) || !b.raw(string(in.CancellationID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	return nil
}
