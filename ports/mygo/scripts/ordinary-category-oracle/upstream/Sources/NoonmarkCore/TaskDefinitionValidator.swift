import Foundation

public enum TaskDefinitionValidationIssue: Equatable, Sendable {
    case invalidContentClock(TaskDefinitionID)
    case invalidTitle(TaskDefinitionID)
    case invalidPlannedSubtasks(TaskDefinitionID)
    case invalidSequence(TaskDefinitionID)
    case incompleteSupersession(TaskDefinitionID)
    case selfSuccessor(TaskDefinitionID)
    case missingDefinitionForChain(TaskChainID)
    case duplicateSequence(TaskChainID, Int)
    case invalidCurrentCount(TaskChainID)
    case missingSuccessor(TaskDefinitionID, TaskDefinitionID)
    case crossChainSuccessor(TaskDefinitionID, TaskDefinitionID)
    case successorCycle(TaskChainID)
}

public enum TaskDefinitionTopologyCompleteness: Sendable {
    case complete
    case permittingMissingSuccessors
}

public enum TaskDefinitionValidator {
    public static func firstSelfContainedIssue(
        in definition: TaskDefinition,
        chainCreatedAt: Date? = nil
    ) -> TaskDefinitionValidationIssue? {
        guard definition.createdAt.timeIntervalSinceReferenceDate.isFinite,
              definition.contentUpdatedAt.timeIntervalSinceReferenceDate.isFinite,
              definition.contentUpdatedAt >= definition.createdAt,
              definition.supersededAt.map({
                  $0.timeIntervalSinceReferenceDate.isFinite
                      && $0 >= definition.createdAt
                      && $0 <= definition.contentUpdatedAt
              }) ?? true
        else {
            return .invalidContentClock(definition.id)
        }
        guard definition.sequence > 0 else {
            return .invalidSequence(definition.id)
        }
        let normalizedTitle = definition.title.trimmingCharacters(
            in: .whitespacesAndNewlines
        )
        guard normalizedTitle == definition.title
        else {
            return .invalidTitle(definition.id)
        }
        guard plannedSubtaskFactsAreValid(
            in: definition,
            chainCreatedAt: chainCreatedAt
        ) else {
            return .invalidPlannedSubtasks(definition.id)
        }
        let hasSupersededDate = definition.supersededAt != nil
        let hasSuccessor = definition.supersededByDefinitionID != nil
        guard hasSupersededDate == false || hasSuccessor else {
            return .incompleteSupersession(definition.id)
        }
        guard definition.supersededByDefinitionID != definition.id else {
            return .selfSuccessor(definition.id)
        }
        return nil
    }

    public static func topologyIssues(
        definitions: [TaskDefinition],
        chainIDs: Set<TaskChainID>,
        completeness: TaskDefinitionTopologyCompleteness = .complete
    ) -> [TaskDefinitionValidationIssue] {
        let definitionsByID = Dictionary(
            uniqueKeysWithValues: definitions.map { ($0.id, $0) }
        )
        let definitionsByChain = Dictionary(
            grouping: definitions,
            by: \.chainID
        )
        var issues: [TaskDefinitionValidationIssue] = []

        for definition in definitions.sorted(by: definitionComesBefore) {
            if let issue = firstSelfContainedIssue(in: definition) {
                issues.append(issue)
            }
        }

        let topologyChainIDs = chainIDs.union(
            definitions.map(\.chainID)
        )
        for chainID in topologyChainIDs.sorted(by: chainComesBefore) {
            let chainDefinitions = definitionsByChain[chainID] ?? []
            issues.append(contentsOf: topologyIssues(
                for: chainID,
                chainDefinitions: chainDefinitions,
                definitionsByID: definitionsByID,
                completeness: completeness
            ))
        }
        return issues
    }

    private static func topologyIssues(
        for chainID: TaskChainID,
        chainDefinitions: [TaskDefinition],
        definitionsByID: [TaskDefinitionID: TaskDefinition],
        completeness: TaskDefinitionTopologyCompleteness
    ) -> [TaskDefinitionValidationIssue] {
        guard chainDefinitions.isEmpty == false else {
            return [.missingDefinitionForChain(chainID)]
        }
        var issues = duplicateSequenceIssues(
            chainID: chainID,
            definitions: chainDefinitions
        )
        if currentCountIsInvalid(
            definitions: chainDefinitions,
            definitionsByID: definitionsByID,
            completeness: completeness
        ) {
            issues.append(.invalidCurrentCount(chainID))
        }
        issues.append(contentsOf: successorReferenceIssues(
            definitions: chainDefinitions,
            definitionsByID: definitionsByID
        ))
        if containsSuccessorCycle(
            chainID: chainID,
            definitionsByID: definitionsByID,
            chainDefinitions: chainDefinitions
        ) {
            issues.append(.successorCycle(chainID))
        }
        return issues
    }

    private static func duplicateSequenceIssues(
        chainID: TaskChainID,
        definitions: [TaskDefinition]
    ) -> [TaskDefinitionValidationIssue] {
        let groups = Dictionary(grouping: definitions, by: \.sequence)
        return groups.keys.sorted().compactMap { sequence in
            groups[sequence, default: []].count > 1
                ? .duplicateSequence(chainID, sequence)
                : nil
        }
    }

    private static func currentCountIsInvalid(
        definitions: [TaskDefinition],
        definitionsByID: [TaskDefinitionID: TaskDefinition],
        completeness: TaskDefinitionTopologyCompleteness
    ) -> Bool {
        let currentCount = definitions.filter {
            $0.supersededAt == nil
        }.count
        let hasMissingSuccessor = definitions.contains { definition in
            guard let successorID = definition.supersededByDefinitionID else {
                return false
            }
            return definitionsByID[successorID] == nil
        }
        let canAwaitCurrentDefinition =
            completeness == .permittingMissingSuccessors
                && currentCount == 0
                && hasMissingSuccessor
        return currentCount != 1 && canAwaitCurrentDefinition == false
    }

    private static func successorReferenceIssues(
        definitions: [TaskDefinition],
        definitionsByID: [TaskDefinitionID: TaskDefinition]
    ) -> [TaskDefinitionValidationIssue] {
        definitions.sorted(by: definitionComesBefore).compactMap { definition in
            guard let successorID = definition.supersededByDefinitionID else {
                return nil
            }
            guard let successor = definitionsByID[successorID] else {
                return .missingSuccessor(definition.id, successorID)
            }
            return successor.chainID != definition.chainID
                ? .crossChainSuccessor(definition.id, successorID)
                : nil
        }
    }

    private static func plannedSubtaskFactsAreValid(
        in definition: TaskDefinition,
        chainCreatedAt: Date?
    ) -> Bool {
        let plannedSubtasks = definition.plannedSubtasks
        let positions = plannedSubtasks.map(\.position).sorted()
        let expectedPositions = plannedSubtasks.indices.map { $0 + 1 }
        return Set(plannedSubtasks.map(\.id)).count == plannedSubtasks.count
            && Set(plannedSubtasks.map(\.lineageID)).count
            == plannedSubtasks.count
            && positions == expectedPositions
            && plannedSubtasks.allSatisfy {
                let normalizedTitle = $0.title.trimmingCharacters(
                    in: .whitespacesAndNewlines
                )
                return normalizedTitle.isEmpty == false
                    && normalizedTitle == $0.title
            }
            && plannedSubtasks.allSatisfy { plannedSubtask in
                plannedSubtask.createdAt.timeIntervalSinceReferenceDate.isFinite
                    && (chainCreatedAt.map {
                        plannedSubtask.createdAt >= $0
                    } ?? true)
                    && plannedSubtask.createdAt <= definition.contentUpdatedAt
            }
    }

    private static func containsSuccessorCycle(
        chainID: TaskChainID,
        definitionsByID: [TaskDefinitionID: TaskDefinition],
        chainDefinitions: [TaskDefinition]
    ) -> Bool {
        var state: [TaskDefinitionID: DefinitionVisitState] = [:]

        func visit(_ definitionID: TaskDefinitionID) -> Bool {
            switch state[definitionID] {
            case .visiting:
                return true
            case .visited:
                return false
            case nil:
                break
            }
            state[definitionID] = .visiting
            guard definitionsByID[definitionID]?.supersededAt != nil else {
                state[definitionID] = .visited
                return false
            }
            let successorID = definitionsByID[definitionID]?
                .supersededByDefinitionID
            if let successorID {
                if let successor = definitionsByID[successorID] {
                    if successor.chainID == chainID, visit(successorID) {
                        return true
                    }
                }
            }
            state[definitionID] = .visited
            return false
        }

        return chainDefinitions.contains { visit($0.id) }
    }

    private static func definitionComesBefore(
        _ lhs: TaskDefinition,
        _ rhs: TaskDefinition
    ) -> Bool {
        if lhs.sequence != rhs.sequence {
            return lhs.sequence < rhs.sequence
        }
        return lhs.id.description < rhs.id.description
    }

    private static func chainComesBefore(
        _ lhs: TaskChainID,
        _ rhs: TaskChainID
    ) -> Bool {
        lhs.description < rhs.description
    }
}

private enum DefinitionVisitState {
    case visiting
    case visited
}
