// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

const historyCommandInputFields historyCommandPhase = 4

// Charge every logical string use, including unused IDs, before shape checks.
// No string contents are inspected and no caller-sized objects are allocated.
func historyCommandTitleInputBudget(in titleHistoryInput) error {
	b := historyCommandBudget{}
	if !b.raw(in.Title) || !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) || !b.raw(string(in.NewDefinitionID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	if len(in.ChainID) != 36 || (in.Route == titleHistoryTrace && len(in.TraceID) != 36) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandIDLength)
	}
	return nil
}
func historyCommandManualProgressInputBudget(in manualProgressHistoryInput) error {
	b := historyCommandBudget{}
	if !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	if len(in.ChainID) != 36 || len(in.TraceID) != 36 {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandIDLength)
	}
	return nil
}
