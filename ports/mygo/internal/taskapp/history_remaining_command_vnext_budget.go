// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import "github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"

// Only lengths and string headers are inspected. Map count is bounded before
// any traversal or allocation, including requests with unused scalar fields.
func historyRemainingMapBudget[K ~string, V ~string](prior validatedHistorySnapshot, ids map[K]V) (historyCommandBudget, error) {
	b := historyCommandBudget{}
	n := uint64(len(ids))
	if n > uint64(len(prior.proof.snapshot.Object["subtasks"].Array)) || n > uint64(canonical.DefaultLimits().MaxNodes)/2 || !b.nodesAdd(n, 2) {
		return b, historyCommandBudgetFail(historyCommandInputFields, historyCommandIDCount)
	}
	for k, v := range ids {
		if !b.raw(string(k)) || !b.raw(string(v)) {
			return b, historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
		}
	}
	return b, nil
}

func historyCommandReturnInputBudget(prior validatedHistorySnapshot, in returnHistoryInput) error {
	b, err := historyRemainingMapBudget(prior, in.PlannedIDs)
	if err != nil {
		return err
	}
	if !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) || !b.raw(string(in.NewDefinitionID)) || !b.raw(string(in.CancellationID)) || !b.raw(string(in.EventID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	return nil
}

func historyCommandDeferInputBudget(prior validatedHistorySnapshot, in deferHistoryInput) error {
	b, err := historyRemainingMapBudget(prior, in.SubtaskIDs)
	if err != nil {
		return err
	}
	if !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) || !b.raw(string(in.NewTraceID)) || !b.raw(string(in.DayID)) || !b.raw(string(in.EventID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	return nil
}

func historyCommandContinueInputBudget(prior validatedHistorySnapshot, in continueHistoryInput) error {
	b, err := historyRemainingMapBudget(prior, in.SubtaskIDs)
	if err != nil {
		return err
	}
	if !b.raw(string(in.ChainID)) || !b.raw(string(in.TraceID)) || !b.raw(string(in.NewTraceID)) || !b.raw(string(in.DayID)) {
		return historyCommandBudgetFail(historyCommandInputFields, historyCommandRawBytes)
	}
	return nil
}
