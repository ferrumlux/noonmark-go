import Foundation

public enum TrajectoryTopologyValidationIssue: Equatable, Sendable {
    case invalidTraceClock(DayTraceID)
    case invalidTraceStatusFacts(DayTraceID)
    case invalidTraceProgress(DayTraceID)
    case invalidTraceSequence(DayTraceID)
    case duplicateActiveTrace(TaskChainID)
    case duplicateVisiblePriority(LocalDate, Int)
    case missingTraceContinuationSource(DayTraceID, DayTraceID)
    case missingTraceContinuationTarget(DayTraceID)
    case reusedTraceContinuationSource(DayTraceID)
    case invalidTraceContinuation(DayTraceID)
    case missingChangedTraceTarget(DayTraceID, DayTraceID)
    case reusedChangedTraceTarget(DayTraceID)
    case invalidChangedTraceLink(DayTraceID)
    case traceCausalCycle(DayTraceID)
    case duplicateSubtaskPosition(DayTraceID, Int)
    case noncontiguousSubtaskPosition(DayTraceID)
    case duplicateSubtaskLineage(DayTraceID)
    case missingSubtaskParent(SubtaskID, DayTraceID)
    case missingSubtaskContinuationSource(SubtaskID, SubtaskID)
    case missingSubtaskContinuationTarget(SubtaskID)
    case reusedSubtaskContinuationSource(SubtaskID)
    case invalidSubtaskContinuation(SubtaskID)
    case subtaskContinuationCycle(SubtaskID)
    case reusedDraftCancellationIdentity(UUID)
}

public enum TrajectoryTopologyValidator {
    public static func firstSelfContainedIssue(
        in trace: DayTrace
    ) -> TrajectoryTopologyValidationIssue? {
        let terminalDates = [trace.completedAt, trace.settledAt]
            .compactMap { $0 }
        guard TaskNoteEntry.isValidMutationTime(
            trace.contentUpdatedAt,
            notBefore: trace.createdAt
        ), terminalDates.allSatisfy({
            TaskNoteEntry.isValidMutationTime(
                $0,
                notBefore: trace.createdAt
            ) && $0 <= trace.contentUpdatedAt
        }) else {
            return .invalidTraceClock(trace.id)
        }
        guard trace.continuationSeq >= 0 else {
            return .invalidTraceSequence(trace.id)
        }
        guard trace.manualProgressPercent.map({ (0 ... 100).contains($0) })
            ?? true
        else {
            return .invalidTraceProgress(trace.id)
        }
        guard trace.pinOrder.map({
            $0 > 0 && trace.status == .pending
        }) ?? true else {
            return .invalidTraceStatusFacts(trace.id)
        }

        let statusFactsAreValid: Bool = switch trace.status {
        case .pending:
            trace.completedAt == nil
                && trace.settledAt == nil
                && trace.changedToTraceID == nil
                && trace.draftCancelledOn == nil
        case .completed:
            trace.completedAt != nil
                && trace.settledAt == nil
                && trace.changedToTraceID == nil
                && trace.draftCancelledOn == nil
        case .unfinished, .deferred, .returnedToPool, .abandoned:
            trace.completedAt == nil
                && trace.settledAt != nil
                && trace.changedToTraceID == nil
                && trace.draftCancelledOn == nil
        case .changed:
            trace.completedAt == nil
                && trace.settledAt != nil
                && trace.changedToTraceID != nil
                && trace.draftCancelledOn == nil
        case .cancelledDraft:
            cancelledDraftFactsAreValid(trace)
        }
        return statusFactsAreValid
            ? nil
            : .invalidTraceStatusFacts(trace.id)
    }

    public static func topologyIssues(
        traces: [DayTrace],
        subtasks: [Subtask]
    ) -> [TrajectoryTopologyValidationIssue] {
        let orderedTraces = traces.sorted(by: traceComesBefore)
        let orderedSubtasks = subtasks.sorted(by: subtaskComesBefore)
        let tracesByID = Dictionary(
            uniqueKeysWithValues: orderedTraces.map { ($0.id, $0) }
        )
        let subtasksByID = Dictionary(
            uniqueKeysWithValues: orderedSubtasks.map { ($0.id, $0) }
        )
        var issues: [TrajectoryTopologyValidationIssue] = []

        for trace in orderedTraces {
            if let issue = firstSelfContainedIssue(in: trace) {
                issues.append(issue)
            }
        }
        issues.append(contentsOf: activeTraceIssues(orderedTraces))
        issues.append(contentsOf: priorityIssues(orderedTraces))
        issues.append(contentsOf: traceLinkIssues(
            orderedTraces,
            tracesByID: tracesByID
        ))
        issues.append(contentsOf: subtaskCollectionIssues(orderedSubtasks))
        issues.append(contentsOf: subtaskLinkIssues(
            orderedSubtasks,
            subtasksByID: subtasksByID,
            tracesByID: tracesByID
        ))
        issues.append(contentsOf: cancellationIdentityIssues(
            traces: orderedTraces,
            subtasks: orderedSubtasks
        ))
        return issues
    }

    private static func cancelledDraftFactsAreValid(
        _ trace: DayTrace
    ) -> Bool {
        guard trace.draftCancellationID != nil,
              let draftCancelledOn = trace.draftCancelledOn
        else {
            return false
        }
        let isSnapshotUndoCancellation = trace.date == draftCancelledOn
        let isFutureDraftCancellation = trace.date > draftCancelledOn
        return (isSnapshotUndoCancellation || isFutureDraftCancellation)
            && trace.completedAt == nil
            && trace.settledAt != nil
            && trace.changedToTraceID == nil
    }

    private static func activeTraceIssues(
        _ traces: [DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        let pendingByChain = Dictionary(
            grouping: traces.filter { $0.status == .pending },
            by: \.chainID
        )
        return pendingByChain.keys
            .sorted(by: chainComesBefore)
            .compactMap { chainID in
                pendingByChain[chainID, default: []].count > 1
                    ? .duplicateActiveTrace(chainID)
                    : nil
        }
    }

    private static func priorityIssues(
        _ traces: [DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        let visible = traces.filter(\.formsDayHistory)
        let groups = Dictionary(
            grouping: visible,
            by: { VisiblePrioritySlot(date: $0.date, priority: $0.priority) }
        )
        return groups.keys.sorted().compactMap { slot in
            groups[slot, default: []].count > 1
                ? .duplicateVisiblePriority(slot.date, slot.priority)
                : nil
        }
    }

    private static func traceLinkIssues(
        _ traces: [DayTrace],
        tracesByID: [DayTraceID: DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        var issues = traceContinuationIssues(
            traces,
            tracesByID: tracesByID
        )
        issues.append(contentsOf: changedTraceLinkIssues(
            traces,
            tracesByID: tracesByID
        ))
        if let cycleID = firstTraceCycle(in: tracesByID) {
            issues.append(.traceCausalCycle(cycleID))
        }
        return issues
    }

    private static func traceContinuationIssues(
        _ traces: [DayTrace],
        tracesByID: [DayTraceID: DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        var issues: [TrajectoryTopologyValidationIssue] = []
        let continuationTargets = Dictionary(
            grouping: traces.compactMap { trace -> TraceLink? in
                trace.carriedFromTraceID.map {
                    TraceLink(source: $0, target: trace.id)
                }
            },
            by: \.source
        )
        for sourceID in continuationTargets.keys.sorted(by: traceIDComesBefore)
        where continuationTargets[sourceID, default: []].count > 1 {
            issues.append(.reusedTraceContinuationSource(sourceID))
        }

        for target in traces {
            let isUnlinkedNonRoot = target.carriedFromTraceID == nil
                && target.continuationSeq != 0
            if isUnlinkedNonRoot {
                issues.append(.invalidTraceSequence(target.id))
            }
            guard let sourceID = target.carriedFromTraceID else {
                continue
            }
            guard let source = tracesByID[sourceID] else {
                issues.append(
                    .missingTraceContinuationSource(target.id, sourceID)
                )
                continue
            }
            let targetIsCancelledUndo = target.status == .cancelledDraft
                && source.status.isUserPresentable
            let sequenceIsValid = switch source.status {
            case .deferred, .pending:
                target.continuationSeq == source.continuationSeq
            case .unfinished:
                source.continuationSeq < Int.max
                    && target.continuationSeq == source.continuationSeq + 1
            case .completed, .changed, .returnedToPool, .cancelledDraft, .abandoned:
                false
            }
            let linkIsValid = source.id != target.id
                && source.chainID == target.chainID
                && sequenceIsValid
                && target.date > source.date
                && target.createdAt >= source.createdAt
                && source.status != .cancelledDraft
                && (
                    source.status == .deferred
                        || source.status == .unfinished
                        || targetIsCancelledUndo
                )
            if linkIsValid == false {
                issues.append(.invalidTraceContinuation(target.id))
            }
        }
        for source in traces {
            let targetIsMissing = source.status == .deferred
                && continuationTargets[source.id, default: []].isEmpty
            if targetIsMissing {
                issues.append(.missingTraceContinuationTarget(source.id))
            }
        }
        return issues
    }

    private static func changedTraceLinkIssues(
        _ traces: [DayTrace],
        tracesByID: [DayTraceID: DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        var issues: [TrajectoryTopologyValidationIssue] = []
        let changedTargets = Dictionary(
            grouping: traces.compactMap { trace -> TraceLink? in
                trace.changedToTraceID.map {
                    TraceLink(source: trace.id, target: $0)
                }
            },
            by: \.target
        )
        for targetID in changedTargets.keys.sorted(by: traceIDComesBefore)
        where changedTargets[targetID, default: []].count > 1 {
            issues.append(.reusedChangedTraceTarget(targetID))
        }
        for source in traces {
            guard let targetID = source.changedToTraceID else {
                continue
            }
            guard let target = tracesByID[targetID] else {
                issues.append(.missingChangedTraceTarget(source.id, targetID))
                continue
            }
            let linkIsValid = source.id != target.id
                && source.status == .changed
                && source.chainID != target.chainID
                && source.date == target.date
                && target.continuationSeq == 0
                && target.carriedFromTraceID == nil
                && target.status != .cancelledDraft
                && target.createdAt >= source.createdAt
            if linkIsValid == false {
                issues.append(.invalidChangedTraceLink(source.id))
            }
        }
        return issues
    }

    private static func subtaskCollectionIssues(
        _ subtasks: [Subtask]
    ) -> [TrajectoryTopologyValidationIssue] {
        let grouped = Dictionary(grouping: subtasks, by: \.traceID)
        var issues: [TrajectoryTopologyValidationIssue] = []
        for traceID in grouped.keys.sorted(by: traceIDComesBefore) {
            let traceSubtasks = grouped[traceID, default: []]
            let positions = traceSubtasks.map(\.position).sorted()
            let expected = traceSubtasks.indices.map { $0 + 1 }
            let positionGroups = Dictionary(
                grouping: positions,
                by: { $0 }
            )
            for position in positionGroups.keys.sorted()
            where positionGroups[position, default: []].count > 1 {
                issues.append(.duplicateSubtaskPosition(traceID, position))
            }
            let positionsAreUnique = Set(positions).count == positions.count
            if positionsAreUnique, positions != expected {
                issues.append(.noncontiguousSubtaskPosition(traceID))
            }
            let lineageCount = Set(traceSubtasks.map(\.lineageID)).count
            if lineageCount != traceSubtasks.count {
                issues.append(.duplicateSubtaskLineage(traceID))
            }
        }
        return issues
    }

    private static func subtaskLinkIssues(
        _ subtasks: [Subtask],
        subtasksByID: [SubtaskID: Subtask],
        tracesByID: [DayTraceID: DayTrace]
    ) -> [TrajectoryTopologyValidationIssue] {
        var issues: [TrajectoryTopologyValidationIssue] = []
        let targetsBySource = Dictionary(
            grouping: subtasks.compactMap { subtask -> SubtaskLink? in
                subtask.carriedFromSubtaskID.map {
                    SubtaskLink(source: $0, target: subtask.id)
                }
            },
            by: \.source
        )
        for sourceID in targetsBySource.keys.sorted(by: subtaskIDComesBefore)
        where targetsBySource[sourceID, default: []].count > 1 {
            issues.append(.reusedSubtaskContinuationSource(sourceID))
        }

        for target in subtasks {
            guard let targetTrace = tracesByID[target.traceID] else {
                issues.append(.missingSubtaskParent(target.id, target.traceID))
                continue
            }
            guard let sourceID = target.carriedFromSubtaskID else {
                continue
            }
            guard let source = subtasksByID[sourceID] else {
                issues.append(
                    .missingSubtaskContinuationSource(target.id, sourceID)
                )
                continue
            }
            guard let sourceTrace = tracesByID[source.traceID] else {
                issues.append(.missingSubtaskParent(source.id, source.traceID))
                continue
            }
            let targetIsCancelledUndo = (
                target.status == .cancelledDraft
                    || targetTrace.status == .cancelledDraft
            ) && source.status.isUserPresentable
            let linkIsValid = source.id != target.id
                && source.lineageID == target.lineageID
                && sourceTrace.chainID == targetTrace.chainID
                && targetTrace.carriedFromTraceID == sourceTrace.id
                && target.createdAt >= source.createdAt
                && source.status != .cancelledDraft
                && (
                    source.status == .deferred
                        || source.status == .unfinished
                        || targetIsCancelledUndo
                )
            if linkIsValid == false {
                issues.append(.invalidSubtaskContinuation(target.id))
            }
        }

        for source in subtasks where source.status == .deferred {
            if targetsBySource[source.id, default: []].isEmpty {
                issues.append(.missingSubtaskContinuationTarget(source.id))
            }
        }
        if let cycleID = firstSubtaskCycle(in: subtasksByID) {
            issues.append(.subtaskContinuationCycle(cycleID))
        }
        return issues
    }

    private static func cancellationIdentityIssues(
        traces: [DayTrace],
        subtasks: [Subtask]
    ) -> [TrajectoryTopologyValidationIssue] {
        let identities = traces.compactMap(\.draftCancellationID)
            + subtasks.compactMap(\.draftCancellationID)
        let grouped = Dictionary(grouping: identities, by: { $0 })
        return grouped.keys.sorted {
            $0.uuidString < $1.uuidString
        }.compactMap { identity in
            grouped[identity, default: []].count > 1
                ? .reusedDraftCancellationIdentity(identity)
                : nil
        }
    }

    private static func firstTraceCycle(
        in tracesByID: [DayTraceID: DayTrace]
    ) -> DayTraceID? {
        let continuationTargets = Dictionary(
            grouping: tracesByID.values.compactMap { trace -> TraceLink? in
                trace.carriedFromTraceID.map {
                    TraceLink(source: $0, target: trace.id)
                }
            },
            by: \.source
        )
        var state: [DayTraceID: VisitState] = [:]
        func visit(_ id: DayTraceID) -> DayTraceID? {
            switch state[id] {
            case .visiting:
                return id
            case .visited:
                return nil
            case nil:
                break
            }
            state[id] = .visiting
            guard let trace = tracesByID[id] else {
                state[id] = .visited
                return nil
            }
            let targetIDs = Set(
                continuationTargets[id, default: []].map(\.target)
                    + [trace.changedToTraceID].compactMap { $0 }
            ).sorted(by: traceIDComesBefore)
            for targetID in targetIDs where tracesByID[targetID] != nil {
                if let cycle = visit(targetID) {
                    return cycle
                }
            }
            state[id] = .visited
            return nil
        }
        for id in tracesByID.keys.sorted(by: traceIDComesBefore) {
            if let cycle = visit(id) {
                return cycle
            }
        }
        return nil
    }

    private static func firstSubtaskCycle(
        in subtasksByID: [SubtaskID: Subtask]
    ) -> SubtaskID? {
        var state: [SubtaskID: VisitState] = [:]
        func visit(_ id: SubtaskID) -> SubtaskID? {
            switch state[id] {
            case .visiting:
                return id
            case .visited:
                return nil
            case nil:
                break
            }
            state[id] = .visiting
            if let sourceID = subtasksByID[id]?.carriedFromSubtaskID {
                if subtasksByID[sourceID] != nil {
                    if let cycle = visit(sourceID) {
                        return cycle
                    }
                }
            }
            state[id] = .visited
            return nil
        }
        for id in subtasksByID.keys.sorted(by: subtaskIDComesBefore) {
            if let cycle = visit(id) {
                return cycle
            }
        }
        return nil
    }

    private static func traceComesBefore(
        _ lhs: DayTrace,
        _ rhs: DayTrace
    ) -> Bool {
        lhs.id.description < rhs.id.description
    }

    private static func subtaskComesBefore(
        _ lhs: Subtask,
        _ rhs: Subtask
    ) -> Bool {
        lhs.id.description < rhs.id.description
    }

    private static func chainComesBefore(
        _ lhs: TaskChainID,
        _ rhs: TaskChainID
    ) -> Bool {
        lhs.description < rhs.description
    }

    private static func traceIDComesBefore(
        _ lhs: DayTraceID,
        _ rhs: DayTraceID
    ) -> Bool {
        lhs.description < rhs.description
    }

    private static func subtaskIDComesBefore(
        _ lhs: SubtaskID,
        _ rhs: SubtaskID
    ) -> Bool {
        lhs.description < rhs.description
    }
}

private struct VisiblePrioritySlot: Hashable, Comparable {
    let date: LocalDate
    let priority: Int

    static func < (
        lhs: VisiblePrioritySlot,
        rhs: VisiblePrioritySlot
    ) -> Bool {
        lhs.date != rhs.date
            ? lhs.date < rhs.date
            : lhs.priority < rhs.priority
    }
}

private struct TraceLink {
    let source: DayTraceID
    let target: DayTraceID
}

private struct SubtaskLink {
    let source: SubtaskID
    let target: SubtaskID
}

private enum VisitState {
    case visiting
    case visited
}
