import Foundation

package struct IndependentPoolTaskCopyRequest {
    package struct Source {
        package let chainID: TaskChainID
        package let title: String
        package let descriptionText: String?
        package let noteBodies: [String]
        package let plannedSubtasks: [PlannedSubtask]

        package init(
            chainID: TaskChainID,
            title: String,
            descriptionText: String?,
            noteBodies: [String],
            plannedSubtasks: [PlannedSubtask]
        ) {
            self.chainID = chainID
            self.title = title
            self.descriptionText = descriptionText
            self.noteBodies = noteBodies
            self.plannedSubtasks = plannedSubtasks
        }
    }

    package struct DestinationIdentity {
        package let chainID: TaskChainID
        package let definitionID: TaskDefinitionID
        package let noteIDs: [TaskNoteEntryID]
        package let plannedSubtaskIDs: [PlannedSubtaskID]
        package let lineageIDs: [SubtaskLineageID]

        package init(
            chainID: TaskChainID,
            definitionID: TaskDefinitionID,
            noteIDs: [TaskNoteEntryID],
            plannedSubtaskIDs: [PlannedSubtaskID],
            lineageIDs: [SubtaskLineageID]
        ) {
            self.chainID = chainID
            self.definitionID = definitionID
            self.noteIDs = noteIDs
            self.plannedSubtaskIDs = plannedSubtaskIDs
            self.lineageIDs = lineageIDs
        }
    }

    package struct ClassificationIdentity {
        package let changeRecordID: UUID
        package let planID: UUID
        package let interactionID: UUID

        package init(
            changeRecordID: UUID,
            planID: UUID,
            interactionID: UUID
        ) {
            self.changeRecordID = changeRecordID
            self.planID = planID
            self.interactionID = interactionID
        }
    }

    package let source: Source
    package let destination: DestinationIdentity
    package let classification: ClassificationIdentity
    package let now: Date

    package init(
        source: Source,
        destination: DestinationIdentity,
        classification: ClassificationIdentity,
        now: Date
    ) {
        self.source = source
        self.destination = destination
        self.classification = classification
        self.now = now
    }
}

public struct SnapshotUndoOutcome: Equatable, Sendable {
    public let automaticClassificationCancelledChainIDs: [TaskChainID]
    public let automaticClassificationRestoredChainIDs: [TaskChainID]

    public init(
        automaticClassificationCancelledChainIDs: [TaskChainID] = [],
        automaticClassificationRestoredChainIDs: [TaskChainID] = []
    ) {
        self.automaticClassificationCancelledChainIDs =
            automaticClassificationCancelledChainIDs
        self.automaticClassificationRestoredChainIDs =
            automaticClassificationRestoredChainIDs
    }
}

public final class NoonmarkEngine {
    /// 单调递增的内存数据版本：任何领域集合变更（包括对同一引擎实例的
    /// 原地变更）都会推进。视图投影备忘层以此识别原地突变，不能只依赖
    /// store 层的引擎赋值（E2E fixture 与 seed 等路径会原地写入）。
    public private(set) var mutationEpoch: UInt64 = 0
    public private(set) var days: [LocalDate: Day] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var taskCycleSeries: [
        TaskCycleSeriesID: TaskCycleSeries
    ] { didSet { mutationEpoch &+= 1 } }
    public private(set) var chains: [TaskChainID: TaskChain] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var definitions: [TaskDefinitionID: TaskDefinition] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var traces: [DayTraceID: DayTrace] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var subtasks: [SubtaskID: Subtask] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var ideas: [IdeaID: IdeaEntry] {
        didSet { mutationEpoch &+= 1 }
    }

    public private(set) var preferences: AppPreferences {
        didSet { mutationEpoch &+= 1 }
    }

    var classificationState: TaskClassificationState {
        didSet { mutationEpoch &+= 1 }
    }

    public init() {
        self.days = [:]
        self.taskCycleSeries = [:]
        self.chains = [:]
        self.definitions = [:]
        self.traces = [:]
        self.subtasks = [:]
        self.ideas = [:]
        self.preferences = AppPreferences()
        self.classificationState = TaskClassificationState()
    }

    public convenience init(snapshot: NoonmarkSnapshot) throws {
        self.init()
        try snapshot.validateIntegrity()
        days = Dictionary(uniqueKeysWithValues: snapshot.days.map { ($0.date, $0) })
        taskCycleSeries = Dictionary(
            uniqueKeysWithValues: snapshot.taskCycleSeries.map {
                ($0.id, $0)
            }
        )
        chains = Dictionary(uniqueKeysWithValues: snapshot.chains.map { ($0.id, $0) })
        definitions = Dictionary(uniqueKeysWithValues: snapshot.definitions.map { ($0.id, $0) })
        traces = Dictionary(uniqueKeysWithValues: snapshot.traces.map { ($0.id, $0) })
        subtasks = Dictionary(uniqueKeysWithValues: snapshot.subtasks.map { ($0.id, $0) })
        ideas = Dictionary(uniqueKeysWithValues: snapshot.ideas.map { ($0.id, $0) })
        preferences = snapshot.preferences
        classificationState = snapshot.classifications
    }

    /// Creates an isolated copy of an already-valid live engine without
    /// repeating full snapshot integrity validation. Dictionary storage stays
    /// copy-on-write until the candidate mutates an affected collection.
    public convenience init(copying source: NoonmarkEngine) {
        self.init()
        days = source.days
        taskCycleSeries = source.taskCycleSeries
        chains = source.chains
        definitions = source.definitions
        traces = source.traces
        subtasks = source.subtasks
        ideas = source.ideas
        preferences = source.preferences
        classificationState = source.classificationState
    }

    public func snapshot() -> NoonmarkSnapshot {
        let traceByID = traces
        return NoonmarkSnapshot(
            days: days.values.sorted { $0.date < $1.date },
            taskCycleSeries: taskCycleSeries.values.sorted {
                if $0.createdAt != $1.createdAt {
                    return $0.createdAt < $1.createdAt
                }
                return $0.id.description < $1.id.description
            },
            chains: chains.values.sorted {
                if $0.createdAt == $1.createdAt {
                    return $0.id.description < $1.id.description
                }
                return $0.createdAt < $1.createdAt
            },
            definitions: definitions.values.sorted {
                if $0.chainID == $1.chainID {
                    return $0.sequence < $1.sequence
                }
                return $0.chainID.description < $1.chainID.description
            },
            traces: traces.values.sorted(by: traceChronology),
            subtasks: subtasks.values.sorted {
                if let lhsTrace = traceByID[$0.traceID], let rhsTrace = traceByID[$1.traceID], lhsTrace != rhsTrace {
                    return traceChronology(lhsTrace, rhsTrace)
                }
                if $0.traceID == $1.traceID, $0.position != $1.position {
                    return $0.position < $1.position
                }
                if $0.createdAt != $1.createdAt {
                    return $0.createdAt < $1.createdAt
                }
                return $0.id.description < $1.id.description
            },
            preferences: preferences,
            classifications: classificationState,
            ideas: ideas.values.sorted {
                if $0.createdAt == $1.createdAt {
                    return $0.id.description < $1.id.description
                }
                return $0.createdAt < $1.createdAt
            }
        )
    }

    @discardableResult
    public func prepareSnapshotUndo(
        replacing currentSnapshot: NoonmarkSnapshot,
        now: Date = Date()
    ) throws -> SnapshotUndoOutcome {
        let candidate = NoonmarkEngine(copying: self)
        let outcome = try candidate.prepareSnapshotUndoInPlace(
            replacing: currentSnapshot,
            now: now
        )
        adoptState(from: candidate)
        return outcome
    }

    private func prepareSnapshotUndoInPlace(
        replacing currentSnapshot: NoonmarkSnapshot,
        now: Date
    ) throws -> SnapshotUndoOutcome {
        try currentSnapshot.validateIntegrity()
        let current = SnapshotUndoCurrentFacts(currentSnapshot)
        let outcome = snapshotUndoOutcome(from: current)
        retainCurrentOnlySnapshotUndoFacts(from: current, now: now)
        try restoreSnapshotUndoChains(from: current, now: now)
        try restoreSnapshotUndoDefinitions(from: current, now: now)
        try restoreSnapshotUndoTraces(from: current, now: now)
        try restoreSnapshotUndoSubtasks(from: current, now: now)

        // Classification commits and captured history are immutable audit facts.
        // New-task undo compensates only current automatic relations. Explicit,
        // inherited and deterministic relations remain current across undo/redo.
        classificationState = currentSnapshot.classifications
        for chainID in outcome.automaticClassificationCancelledChainIDs {
            try removeAutomaticClassificationForSnapshotUndo(
                chainID: chainID,
                now: now
            )
        }
        try snapshot().validateIntegrity()
        return outcome
    }

    func adoptState(from candidate: NoonmarkEngine) {
        days = candidate.days
        taskCycleSeries = candidate.taskCycleSeries
        chains = candidate.chains
        definitions = candidate.definitions
        traces = candidate.traces
        subtasks = candidate.subtasks
        ideas = candidate.ideas
        preferences = candidate.preferences
        classificationState = candidate.classificationState
    }

    @discardableResult
    public func deleteTaskCategoryFromToday(
        _ categoryID: TaskCategoryID,
        today: LocalDate,
        interactionID: UUID = UUID(),
        decisionID: UUID,
        now: Date = Date()
    ) throws -> TaskCategoryDeletionOutcome {
        try deleteTaskCategoryFromTodayRecordingCommitBoundaries(
            categoryID,
            today: today,
            interactionID: interactionID,
            decisionID: decisionID,
            now: now
        ).outcome
    }

    @discardableResult
    public func deleteTaskCategoryFromTodayRecordingCommitBoundaries(
        _ categoryID: TaskCategoryID,
        today: LocalDate,
        interactionID: UUID = UUID(),
        decisionID: UUID,
        now: Date = Date()
    ) throws -> TaskCategoryDeletionCommitSequence {
        let candidate = NoonmarkEngine(copying: self)
        let sequence = try candidate.deleteTaskCategoryFromTodayInPlace(
            categoryID,
            today: today,
            interactionID: interactionID,
            decisionID: decisionID,
            now: now
        )
        try candidate.snapshot().validateIntegrity()
        adoptState(from: candidate)
        return sequence
    }

    private func deleteTaskCategoryFromTodayInPlace(
        _ categoryID: TaskCategoryID,
        today: LocalDate,
        interactionID: UUID,
        decisionID: UUID,
        now: Date
    ) throws -> TaskCategoryDeletionCommitSequence {
        guard classificationState.categories[categoryID]?.lifecycle
            == .active
        else {
            throw NoonmarkError.invalidTransition(
                "only an active task category can be deleted"
            )
        }

        let affectedChainIDs = classificationState.currentByChainID
            .compactMap { chainID, current in
                current.categoryID == categoryID ? chainID : nil
            }
            .sorted { $0.description < $1.description }
        var commitBoundaries: [NoonmarkSnapshot] = []
        for chainID in affectedChainIDs {
            guard let current = classificationState
                .currentByChainID[chainID]
            else {
                continue
            }
            let plan = try prepareClassification(
                .setCurrent(
                    TaskClassificationDraft(
                        chainID: chainID,
                        category: nil,
                        labels: current.labelIDs
                            .sorted { $0.description < $1.description }
                            .map(TaskLabelChoice.existing)
                    )
                ),
                source: .deterministicDomainAction(
                    reason: "task category deleted from today onward"
                ),
                interactionID: UUID(),
                now: now
            )
            try commitDeterministicDomainClassification(plan, now: now)
            commitBoundaries.append(snapshot())
        }

        let affectedChainIDSet = Set(affectedChainIDs)
        var updatedTodaySnapshotCount = 0
        let todayTraceIDs = traces.values
            .filter { trace in
                trace.date == today
                    && affectedChainIDSet.contains(trace.chainID)
                    && classificationState.snapshotsByTraceID[
                        trace.id
                    ]?.category?.id == categoryID
            }
            .map(\.id)
            .sorted { $0.description < $1.description }
        for traceID in todayTraceIDs {
            if try appendTodayCategoryRemovalSnapshot(
                traceID: traceID,
                categoryID: categoryID,
                today: today,
                now: now
            ) {
                updatedTodaySnapshotCount += 1
            }
        }

        let affectedSeriesIDs = taskCycleSeries.values
            .filter { $0.categoryID == categoryID }
            .map(\.id)
        for seriesID in affectedSeriesIDs {
            guard var series = taskCycleSeries[seriesID] else { continue }
            try series.updateTemplateClassification(
                categoryID: nil,
                labelIDs: series.labelIDs,
                source: .userDirect,
                interactionID: interactionID,
                decisionID: decisionID,
                now: now
            )
            taskCycleSeries[seriesID] = series
        }

        let archivePlan = try prepareClassification(
            .archiveCategory(categoryID),
            source: .userDirect,
            interactionID: interactionID,
            now: now
        )
        _ = try commitClassification(
            archivePlan,
            confirmation: .confirmedByUser(
                confirming: archivePlan,
                decisionID: decisionID
            ),
            now: now
        )
        commitBoundaries.append(snapshot())
        return TaskCategoryDeletionCommitSequence(
            outcome: TaskCategoryDeletionOutcome(
                ungroupedTaskCount: affectedChainIDs.count,
                updatedTodaySnapshotCount: updatedTodaySnapshotCount
            ),
            commitBoundaries: commitBoundaries
        )
    }

    @discardableResult
    public func createPoolTask(
        title: String,
        descriptionText: String? = nil,
        initialNoteBody: String? = nil,
        now: Date = Date()
    ) throws -> TaskChainID {
        let normalizedTitle = try normalizeTitle(title)
        let noteEntries = try normalizedOptionalText(initialNoteBody)
            .map { [try TaskNoteEntry(body: $0, now: now)] } ?? []
        let chain = TaskChain(noteEntries: noteEntries, now: now)
        let definition = TaskDefinition(
            chainID: chain.id,
            sequence: 1,
            title: normalizedTitle,
            descriptionText: descriptionText,
            now: now
        )

        chains[chain.id] = chain
        definitions[definition.id] = definition
        return chain.id
    }

    func normalizedTaskCycleTitle(_ title: String) throws -> String {
        try normalizeTitle(title)
    }

    func storeTaskCycleSeries(_ series: TaskCycleSeries) {
        taskCycleSeries[series.id] = series
    }

    func currentTaskCycleDefinition(
        for chainID: TaskChainID
    ) throws -> TaskDefinition {
        try currentDefinition(for: chainID)
    }

    func storeTaskCycleDefinition(_ definition: TaskDefinition) {
        definitions[definition.id] = definition
    }

    func storeTaskCycleTrace(_ trace: DayTrace) {
        traces[trace.id] = trace
    }

    func touchTaskCycleChain(
        _ chainID: TaskChainID,
        now: Date
    ) throws {
        try touchChain(chainID, now: now)
    }

    func ensureTaskCycleDatesUnlocked(
        _ dates: [LocalDate]
    ) throws {
        for date in Set(dates) {
            try ensureUnlockedDay(date)
        }
    }

    func taskCycleConversionSource(
        chainID: TaskChainID
    ) throws -> (
        chain: TaskChain,
        definition: TaskDefinition,
        activeTrace: DayTrace?,
        isInTaskPool: Bool
    ) {
        (
            try chain(chainID),
            try currentDefinition(for: chainID),
            activeTrace(for: chainID),
            isInTaskPool(chainID)
        )
    }

    func prepareTaskCycleAdoption(
        chainID: TaskChainID,
        membership: TaskCycleMembership,
        descriptionText: String?,
        now: Date
    ) throws -> (
        chain: TaskChain,
        insertedTrace: DayTrace?
    ) {
        var adoptedChain = try chain(chainID)
        try adoptedChain.markContentModified(at: now)
        adoptedChain.cycleMembership = membership

        guard activeTrace(for: chainID) == nil else {
            return (adoptedChain, nil)
        }
        let definition = try currentDefinition(for: chainID)
        return (
            adoptedChain,
            DayTrace(
                chainID: chainID,
                definitionID: definition.id,
                date: membership.occurrenceDate,
                priority: nextPriority(on: membership.occurrenceDate),
                descriptionText: descriptionText,
                now: now
            )
        )
    }

    func storePreparedTaskCycleAdoption(
        chain: TaskChain,
        insertedTrace: DayTrace?,
        now: Date
    ) {
        chains[chain.id] = chain
        if let insertedTrace {
            traces[insertedTrace.id] = insertedTrace
            if let definition = definitions[insertedTrace.definitionID] {
                copyPlannedSubtasks(
                    from: definition,
                    to: insertedTrace.id,
                    now: now
                )
            }
            ensureDay(insertedTrace.date, now: now)
        }
    }

    func prepareTaskCycleTraceCancellation(
        _ value: DayTrace,
        today: LocalDate,
        now: Date
    ) throws -> DayTrace {
        var trace = value
        try trace.markContentModified(at: now)
        trace.status = .cancelledDraft
        trace.pinOrder = nil
        trace.completedAt = nil
        trace.settledAt = now
        trace.draftCancellationID = UUID()
        trace.draftCancelledOn = today
        return trace
    }

    func storePreparedTaskCycleTraceCancellations(
        _ preparedTraces: [DayTrace],
        now: Date
    ) throws {
        for trace in preparedTraces {
            traces[trace.id] = trace
            if let updatedAt = chains[trace.chainID]?.updatedAt {
                chains[trace.chainID]?.updatedAt = max(updatedAt, now)
            }
        }
    }

    @discardableResult
    func installTaskCycleOccurrence(
        normalizedTitle: String,
        descriptionText: String?,
        plannedSubtasks: [PlannedSubtask] = [],
        membership: TaskCycleMembership,
        now: Date
    ) -> TaskChainID {
        let chain = TaskChain(
            cycleMembership: membership,
            now: now
        )
        let definition = TaskDefinition(
            chainID: chain.id,
            sequence: 1,
            title: normalizedTitle,
            descriptionText: descriptionText,
            plannedSubtasks: plannedSubtasks.map {
                PlannedSubtask(
                    lineageID: $0.lineageID,
                    title: $0.title,
                    difficulty: $0.difficulty,
                    position: $0.position,
                    now: now
                )
            },
            now: now
        )
        let trace = DayTrace(
            chainID: chain.id,
            definitionID: definition.id,
            date: membership.occurrenceDate,
            priority: nextPriority(on: membership.occurrenceDate),
            descriptionText: descriptionText,
            now: now
        )
        chains[chain.id] = chain
        definitions[definition.id] = definition
        traces[trace.id] = trace
        copyPlannedSubtasks(
            from: definition,
            to: trace.id,
            now: now
        )
        ensureDay(membership.occurrenceDate, now: now)
        return chain.id
    }

    @discardableResult
    func reactivateTaskCycleOccurrence(
        chainID: TaskChainID,
        series: TaskCycleSeries,
        occurrenceDate: LocalDate,
        now: Date
    ) throws -> DayTraceID {
        var current = try currentDefinition(for: chainID)
        let nextSequence = (definitions.values
            .filter { $0.chainID == chainID }
            .map(\.sequence)
            .max() ?? current.sequence) + 1
        let definition = TaskDefinition(
            chainID: chainID,
            sequence: nextSequence,
            title: series.title,
            descriptionText: series.descriptionText,
            plannedSubtasks: series.plannedSubtasks.map {
                PlannedSubtask(
                    lineageID: $0.lineageID,
                    title: $0.title,
                    difficulty: $0.difficulty,
                    position: $0.position,
                    now: now
                )
            },
            now: now
        )
        try current.markContentModified(at: now)
        current.supersededAt = now
        current.supersededByDefinitionID = definition.id
        let trace = DayTrace(
            chainID: chainID,
            definitionID: definition.id,
            date: occurrenceDate,
            priority: nextPriority(on: occurrenceDate),
            descriptionText: series.descriptionText,
            now: now
        )
        definitions[current.id] = current
        definitions[definition.id] = definition
        traces[trace.id] = trace
        copyPlannedSubtasks(
            from: definition,
            to: trace.id,
            now: now
        )
        ensureDay(occurrenceDate, now: now)
        try touchChain(chainID, now: now)
        return trace.id
    }

    func replaceTaskCycleTraceSubtasks(
        traceID: DayTraceID,
        plannedSubtasks: [PlannedSubtask],
        now: Date
    ) throws {
        let existingSubtasks = subtasks.values.filter {
            $0.traceID == traceID
        }
        let desiredSubtasks = plannedSubtasks.sorted {
            $0.position < $1.position
        }
        let desiredLineageIDs = Set(desiredSubtasks.map(\.lineageID))

        for value in existingSubtasks
            where value.status == .pending
                && desiredLineageIDs.contains(value.lineageID) == false
        {
            var subtask = value
            try subtask.markContentModified(at: now)
            subtask.status = .cancelledDraft
            subtask.completedAt = nil
            subtask.settledAt = now
            subtask.draftCancellationID = UUID()
            subtasks[subtask.id] = subtask
        }

        var nextPosition =
            (existingSubtasks.map(\.position).max() ?? 0) + 1
        for plannedSubtask in desiredSubtasks {
            if var existing = existingSubtasks.first(where: {
                $0.lineageID == plannedSubtask.lineageID
            }) {
                guard existing.status == .pending
                    || existing.status == .cancelledDraft
                else {
                    throw NoonmarkError.invalidTransition(
                        "future task cycle subtasks must remain editable"
                    )
                }
                try existing.markContentModified(at: now)
                existing.title = plannedSubtask.title
                existing.difficulty = plannedSubtask.difficulty
                existing.status = .pending
                existing.completedAt = nil
                existing.settledAt = nil
                existing.draftCancellationID = nil
                subtasks[existing.id] = existing
                continue
            }
            let subtask = Subtask(
                lineageID: plannedSubtask.lineageID,
                traceID: traceID,
                title: plannedSubtask.title,
                difficulty: plannedSubtask.difficulty,
                position: nextPosition,
                now: now
            )
            subtasks[subtask.id] = subtask
            nextPosition += 1
        }
    }

    public func updatePoolTask(
        chainID: TaskChainID,
        title: String,
        descriptionText: String? = nil,
        plannedSubtasks: [PlannedSubtask]? = nil,
        now: Date = Date()
    ) throws {
        let normalizedTitle = try normalizeTitle(title, allowsEmpty: true)
        var definition = try currentDefinition(for: chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can be edited")
        }

        let nextPlannedSubtasks = plannedSubtasks?
            .sorted { $0.position < $1.position } ?? definition.plannedSubtasks
        guard definition.title != normalizedTitle
            || definition.descriptionText != descriptionText
            || definition.plannedSubtasks != nextPlannedSubtasks
        else { return }
        try definition.markContentModified(at: now)
        definition.title = normalizedTitle
        definition.descriptionText = descriptionText
        definition.plannedSubtasks = nextPlannedSubtasks
        definitions[definition.id] = definition
        try touchChain(chainID, now: now)
    }

    package func acceptRemoteTaskDefinitionAsForwardRevision(
        _ remoteDefinition: TaskDefinition,
        conflictedDefinitionID: TaskDefinitionID,
        forwardDefinitionID: TaskDefinitionID,
        now: Date
    ) throws {
        guard now.timeIntervalSinceReferenceDate.isFinite,
              remoteDefinition.id == conflictedDefinitionID,
              let conflicted = definitions[conflictedDefinitionID],
              conflicted.chainID == remoteDefinition.chainID,
              definitions[forwardDefinitionID] == nil
        else {
            throw NoonmarkError.invalidInput(
                "remote task definition resolution is not bound to current facts"
            )
        }
        try ensureActiveChain(conflicted.chainID)
        var current = try currentDefinition(for: conflicted.chainID)
        let maximumSequence = definitions.values
            .filter { $0.chainID == conflicted.chainID }
            .map(\.sequence)
            .max() ?? current.sequence
        guard maximumSequence < Int.max else {
            throw NoonmarkError.invalidTransition(
                "task definition sequence is exhausted"
            )
        }
        let forward = TaskDefinition(
            id: forwardDefinitionID,
            chainID: conflicted.chainID,
            sequence: maximumSequence + 1,
            title: remoteDefinition.title,
            descriptionText: remoteDefinition.descriptionText,
            plannedSubtasks: remoteDefinition.plannedSubtasks,
            now: now
        )

        try current.markContentModified(at: now)
        current.supersededAt = now
        current.supersededByDefinitionID = forward.id
        var reboundTraces: [DayTrace] = []
        for value in traces.values
        where value.definitionID == current.id && value.status == .pending {
            var trace = value
            try trace.markContentModified(at: now)
            trace.definitionID = forward.id
            reboundTraces.append(trace)
        }

        definitions[current.id] = current
        definitions[forward.id] = forward
        for trace in reboundTraces {
            traces[trace.id] = trace
        }
        try touchChain(conflicted.chainID, now: now)
        try snapshot().validateIntegrity()
    }

    @discardableResult
    package func copyTaskAsIndependentPoolTask(
        _ request: IndependentPoolTaskCopyRequest
    ) throws -> TaskChainID {
        let source = request.source
        let destination = request.destination
        let now = request.now
        guard now.timeIntervalSinceReferenceDate.isFinite,
              chains[source.chainID] != nil,
              chains[destination.chainID] == nil,
              definitions[destination.definitionID] == nil,
              source.noteBodies.count == destination.noteIDs.count,
              source.plannedSubtasks.count
              == destination.plannedSubtaskIDs.count,
              source.plannedSubtasks.count == destination.lineageIDs.count,
              Set(destination.noteIDs).count == destination.noteIDs.count,
              Set(destination.plannedSubtaskIDs).count
              == destination.plannedSubtaskIDs.count,
              Set(destination.lineageIDs).count
              == destination.lineageIDs.count
        else {
            throw NoonmarkError.invalidInput(
                "replication conflict copy identities are invalid"
            )
        }
        let normalizedTitle = try normalizeTitle(
            source.title,
            allowsEmpty: true
        )
        let notes = try zip(source.noteBodies, destination.noteIDs).map {
            try TaskNoteEntry(id: $0.1, body: $0.0, now: now)
        }
        let copiedPlannedSubtasks = zip(
            source.plannedSubtasks,
            zip(destination.plannedSubtaskIDs, destination.lineageIDs)
        ).enumerated().map { index, value in
            PlannedSubtask(
                id: value.1.0,
                lineageID: value.1.1,
                title: value.0.title,
                difficulty: value.0.difficulty,
                position: index + 1,
                now: now
            )
        }
        let chain = TaskChain(
            id: destination.chainID,
            noteEntries: notes,
            now: now
        )
        let definition = TaskDefinition(
            id: destination.definitionID,
            chainID: destination.chainID,
            sequence: 1,
            title: normalizedTitle,
            descriptionText: source.descriptionText,
            plannedSubtasks: copiedPlannedSubtasks,
            now: now
        )
        chains[chain.id] = chain
        definitions[definition.id] = definition
        _ = try inheritCurrentClassification(
            from: source.chainID,
            to: destination.chainID,
            now: now,
            changeRecordID: request.classification.changeRecordID,
            planID: request.classification.planID,
            interactionID: request.classification.interactionID
        )
        try snapshot().validateIntegrity()
        return destination.chainID
    }

    @discardableResult
    public func appendPoolNote(
        chainID: TaskChainID,
        body: String,
        now: Date = Date()
    ) throws -> TaskNoteEntryID {
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can append notes")
        }
        var chain = try chain(chainID)
        try TaskNoteEntry.validateMutationTime(now, notBefore: chain.createdAt)
        let note = try TaskNoteEntry(body: body, now: now)
        chain.noteEntries.append(note)
        chains[chain.id] = chain
        return note.id
    }

    public func editPoolNote(
        chainID: TaskChainID,
        noteID: TaskNoteEntryID,
        body: String,
        now: Date = Date()
    ) throws {
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can edit notes")
        }
        var chain = try chain(chainID)
        try chain.noteEntries.editTaskNote(
            id: noteID,
            body: body,
            now: now,
            ownerUpdatedAt: chain.createdAt
        )
        chains[chain.id] = chain
    }

    public func deletePoolNote(
        chainID: TaskChainID,
        noteID: TaskNoteEntryID,
        now: Date = Date()
    ) throws {
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can delete notes")
        }
        var chain = try chain(chainID)
        try chain.noteEntries.deleteTaskNote(
            id: noteID,
            now: now,
            ownerUpdatedAt: chain.createdAt
        )
        chains[chain.id] = chain
    }

    @discardableResult
    public func appendIdea(
        body: String,
        categoryID: TaskCategoryID? = nil,
        labelIDs: [TaskLabelID] = [],
        now: Date = Date()
    ) throws -> IdeaEntry {
        try validateIdeaClassification(
            categoryID: categoryID,
            labelIDs: labelIDs
        )
        let idea = try IdeaEntry(
            body: body,
            categoryID: categoryID,
            labelIDs: labelIDs,
            now: now
        )
        ideas[idea.id] = idea
        return idea
    }

    public func editIdea(
        id: IdeaID,
        body: String,
        now: Date = Date()
    ) throws {
        let idea = try activeIdea(id)
        try editIdea(
            id: id,
            body: body,
            categoryID: idea.categoryID,
            labelIDs: idea.labelIDs,
            now: now
        )
    }

    public func editIdea(
        id: IdeaID,
        body: String,
        categoryID: TaskCategoryID?,
        labelIDs: [TaskLabelID],
        now: Date = Date()
    ) throws {
        try validateIdeaClassification(
            categoryID: categoryID,
            labelIDs: labelIDs
        )
        var idea = try activeIdea(id)
        try idea.edit(body: body, now: now)
        try idea.setClassification(
            categoryID: categoryID,
            labelIDs: labelIDs,
            now: now
        )
        ideas[idea.id] = idea
    }

    public func deleteIdea(
        id: IdeaID,
        now: Date = Date()
    ) throws {
        var idea = try activeIdea(id)
        try idea.delete(now: now)
        ideas[idea.id] = idea
    }

    public func setIdeaClassification(
        id: IdeaID,
        categoryID: TaskCategoryID?,
        labelIDs: [TaskLabelID] = [],
        now: Date = Date()
    ) throws {
        try validateIdeaClassification(
            categoryID: categoryID,
            labelIDs: labelIDs
        )
        var idea = try activeIdea(id)
        try idea.setClassification(
            categoryID: categoryID,
            labelIDs: labelIDs,
            now: now
        )
        ideas[idea.id] = idea
    }

    public func pinIdea(
        id: IdeaID,
        now: Date = Date()
    ) throws {
        var idea = try activeIdea(id)
        try idea.pin(now: now)
        ideas[idea.id] = idea
    }

    public func unpinIdea(
        id: IdeaID,
        now: Date = Date()
    ) throws {
        var idea = try activeIdea(id)
        try idea.unpin(now: now)
        ideas[idea.id] = idea
    }

    public func restoreIdea(
        id: IdeaID,
        body: String,
        now: Date = Date()
    ) throws {
        guard var idea = ideas[id] else {
            throw NoonmarkError.notFound("idea")
        }
        try idea.restore(body: body, now: now)
        ideas[idea.id] = idea
    }

    public func ideaTimeline(filter: String? = nil) -> [IdeaEntry] {
        ideaTimeline(filter: IdeaTimelineFilter(text: filter))
    }

    public func ideaTimeline(filter: IdeaTimelineFilter) -> [IdeaEntry] {
        visibleIdeas(matching: filter, pinned: false)
            .sorted {
                if $0.createdAt != $1.createdAt {
                    return $0.createdAt > $1.createdAt
                }
                return $0.id.description > $1.id.description
            }
    }

    public func pinnedIdeas(
        filter: IdeaTimelineFilter = IdeaTimelineFilter()
    ) -> [IdeaEntry] {
        visibleIdeas(matching: filter, pinned: true)
            .sorted {
                if $0.pinnedAt != $1.pinnedAt {
                    return ($0.pinnedAt ?? .distantPast)
                        > ($1.pinnedAt ?? .distantPast)
                }
                return $0.id.description > $1.id.description
            }
    }

    /// Tombstoned ideas, newest deletion first. Tombstones are never hard
    /// purged: sync convergence relies on them, so the trash is a projection,
    /// not a queue for physical deletion.
    public func ideaTrash() -> [IdeaEntry] {
        ideas.values
            .filter(\.isDeleted)
            .sorted {
                if $0.deletedAt != $1.deletedAt {
                    return ($0.deletedAt ?? .distantPast)
                        > ($1.deletedAt ?? .distantPast)
                }
                return $0.id.description > $1.id.description
            }
    }

    public func ideaTimelineByDay(
        filter: String? = nil,
        calendar: Calendar = .current
    ) -> [IdeaDayGroup] {
        ideaTimelineByDay(
            filter: IdeaTimelineFilter(text: filter),
            calendar: calendar
        )
    }

    public func ideaTimelineByDay(
        filter: IdeaTimelineFilter,
        calendar: Calendar = .current
    ) -> [IdeaDayGroup] {
        var ideasByDate: [LocalDate: [IdeaEntry]] = [:]
        for idea in ideaTimeline(filter: filter) {
            let components = calendar.dateComponents(
                [.year, .month, .day],
                from: idea.createdAt
            )
            guard let year = components.year,
                  let month = components.month,
                  let day = components.day
            else { continue }
            ideasByDate[LocalDate(year: year, month: month, day: day), default: []]
                .append(idea)
        }
        return ideasByDate
            .map { IdeaDayGroup(date: $0.key, ideas: $0.value) }
            .sorted { $0.date > $1.date }
    }

    private func visibleIdeas(
        matching filter: IdeaTimelineFilter,
        pinned: Bool
    ) -> [IdeaEntry] {
        let needle = filter.text?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return ideas.values
            .filter { idea in
                guard idea.isDeleted == false else { return false }
                guard (idea.pinnedAt != nil) == pinned else { return false }
                if let categoryID = filter.categoryID,
                   idea.categoryID != categoryID
                {
                    return false
                }
                if let labelID = filter.labelID,
                   idea.labelIDs.contains(labelID) == false
                {
                    return false
                }
                guard needle.isEmpty == false else { return true }
                return ideaMatchesSearchText(idea, text: needle)
            }
    }

    /// Search text spans the memo body and the user-visible names of its
    /// category and labels. The query itself remains transient and is never
    /// copied into persisted facts or diagnostics.
    func ideaMatchesSearchText(_ idea: IdeaEntry, text: String) -> Bool {
        let needle = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard needle.isEmpty == false else { return true }
        if idea.body.localizedCaseInsensitiveContains(needle) {
            return true
        }
        if let categoryID = idea.categoryID {
            let categoryName = classificationState.categories[categoryID]?.name
            if categoryName?.localizedCaseInsensitiveContains(needle) == true {
                return true
            }
        }
        return idea.labelIDs.contains { labelID in
            classificationState.labels[labelID]?
                .name.localizedCaseInsensitiveContains(needle) == true
        }
    }

    private func activeIdea(_ id: IdeaID) throws -> IdeaEntry {
        guard let idea = ideas[id], idea.isDeleted == false else {
            throw NoonmarkError.notFound("idea")
        }
        return idea
    }

    private func validateIdeaClassification(
        categoryID: TaskCategoryID?,
        labelIDs: [TaskLabelID]
    ) throws {
        guard categoryID.map({
            classificationState.categories[$0] != nil
        }) ?? true,
            labelIDs.allSatisfy({
                classificationState.labels[$0] != nil
            })
        else {
            throw NoonmarkError.invalidTransition(
                "idea classification references a missing item"
            )
        }
    }

    public func renameTaskTitle(
        chainID: TaskChainID,
        title: String,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        try updateTaskTitle(
            chainID: chainID,
            title: title,
            today: today,
            now: now,
            reusingEditableDefinition: false
        )
    }

    /// Persists a character-level title input without creating redundant
    /// definition versions after the editable task has been detached from its
    /// historical definition.
    public func saveTaskTitleInput(
        chainID: TaskChainID,
        title: String,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        try updateTaskTitle(
            chainID: chainID,
            title: title,
            today: today,
            now: now,
            reusingEditableDefinition: true
        )
    }

    private func updateTaskTitle(
        chainID: TaskChainID,
        title: String,
        today: LocalDate,
        now: Date,
        reusingEditableDefinition: Bool
    ) throws {
        let normalizedTitle = try normalizeTitle(title, allowsEmpty: true)
        let chain = try chain(chainID)
        guard chain.state == .active else {
            throw NoonmarkError.chainAbandoned
        }

        let chainTraces = traces.values.filter {
            $0.chainID == chainID && $0.formsDayHistory
        }
        let editableStatuses: Set<TraceStatus> = [.pending, .returnedToPool]
        guard chainTraces.isEmpty || chainTraces.contains(where: { editableStatuses.contains($0.status) }) else {
            throw NoonmarkError.invalidTransition("completed and unfinished task facts cannot be renamed")
        }
        guard chainTraces.allSatisfy({ $0.status != .pending || $0.date >= today }) else {
            throw NoonmarkError.immutableHistory
        }
        for trace in chainTraces where trace.status == .pending {
            try ensureUnlockedDay(trace.date)
        }

        var currentDefinition = try currentDefinition(for: chainID)
        guard currentDefinition.title != normalizedTitle else { return }

        let currentDefinitionHasHistoricalReference = chainTraces.contains {
            $0.definitionID == currentDefinition.id && $0.status != .pending
        }
        let canReuseCurrentDefinition = chainTraces.isEmpty
            || (reusingEditableDefinition && currentDefinitionHasHistoricalReference == false)
        if canReuseCurrentDefinition {
            try currentDefinition.markContentModified(at: now)
            currentDefinition.title = normalizedTitle
            definitions[currentDefinition.id] = currentDefinition
        } else {
            let nextSequence = (definitions.values
                .filter { $0.chainID == chainID }
                .map(\.sequence)
                .max() ?? currentDefinition.sequence) + 1
            let renamedDefinition = TaskDefinition(
                chainID: chainID,
                sequence: nextSequence,
                title: normalizedTitle,
                descriptionText: currentDefinition.descriptionText,
                plannedSubtasks: currentDefinition.plannedSubtasks,
                now: now
            )

            try currentDefinition.markContentModified(at: now)
            currentDefinition.supersededAt = now
            currentDefinition.supersededByDefinitionID = renamedDefinition.id
            var renamedTraces: [DayTrace] = []
            for trace in chainTraces where trace.status == .pending {
                var renamedTrace = trace
                try renamedTrace.markContentModified(at: now)
                renamedTrace.definitionID = renamedDefinition.id
                renamedTraces.append(renamedTrace)
            }

            definitions[currentDefinition.id] = currentDefinition
            definitions[renamedDefinition.id] = renamedDefinition
            for renamedTrace in renamedTraces {
                traces[renamedTrace.id] = renamedTrace
            }
        }

        try touchChain(chainID, now: now)
    }

    @discardableResult
    public func addPlannedSubtask(
        chainID: TaskChainID,
        title: String,
        difficulty: SubtaskDifficulty = .simple,
        now: Date = Date()
    ) throws -> PlannedSubtaskID {
        let normalizedTitle = try normalizeTitle(title)
        var definition = try currentDefinition(for: chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("planned subtasks can only be edited in the task pool")
        }

        let plannedSubtask = PlannedSubtask(
            title: normalizedTitle,
            difficulty: difficulty,
            position: definition.plannedSubtasks.count + 1,
            now: now
        )
        try definition.markContentModified(at: now)
        definition.plannedSubtasks.append(plannedSubtask)
        definitions[definition.id] = definition
        try touchChain(chainID, now: now)
        return plannedSubtask.id
    }

    public func removePlannedSubtask(
        chainID: TaskChainID,
        plannedSubtaskID: PlannedSubtaskID,
        now: Date = Date()
    ) throws {
        var definition = try currentDefinition(for: chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("planned subtasks can only be edited in the task pool")
        }
        guard definition.plannedSubtasks.contains(where: { $0.id == plannedSubtaskID }) else {
            throw NoonmarkError.notFound("planned subtask")
        }

        let nextPlannedSubtasks = definition.plannedSubtasks
            .filter { $0.id != plannedSubtaskID }
            .enumerated()
            .map { index, plannedSubtask in
                var plannedSubtask = plannedSubtask
                plannedSubtask.position = index + 1
                return plannedSubtask
            }
        try definition.markContentModified(at: now)
        definition.plannedSubtasks = nextPlannedSubtasks
        definitions[definition.id] = definition
        try touchChain(chainID, now: now)
    }

    public func updatePlannedSubtaskTitle(
        chainID: TaskChainID,
        plannedSubtaskID: PlannedSubtaskID,
        title: String,
        now: Date = Date()
    ) throws {
        let normalizedTitle = try normalizeTitle(title)
        var definition = try currentDefinition(for: chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("planned subtasks can only be edited in the task pool")
        }
        guard let index = definition.plannedSubtasks.firstIndex(where: {
            $0.id == plannedSubtaskID
        }) else {
            throw NoonmarkError.notFound("planned subtask")
        }
        guard definition.plannedSubtasks[index].title != normalizedTitle else {
            return
        }

        try definition.markContentModified(at: now)
        definition.plannedSubtasks[index].title = normalizedTitle
        definitions[definition.id] = definition
        try touchChain(chainID, now: now)
    }

    public func updatePlannedSubtaskDifficulty(
        chainID: TaskChainID,
        plannedSubtaskID: PlannedSubtaskID,
        difficulty: SubtaskDifficulty,
        now: Date = Date()
    ) throws {
        var definition = try currentDefinition(for: chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("planned subtasks can only be edited in the task pool")
        }
        guard let index = definition.plannedSubtasks.firstIndex(where: { $0.id == plannedSubtaskID }) else {
            throw NoonmarkError.notFound("planned subtask")
        }
        guard definition.plannedSubtasks[index].difficulty != difficulty else { return }

        try definition.markContentModified(at: now)
        definition.plannedSubtasks[index].difficulty = difficulty
        definitions[definition.id] = definition
        try touchChain(chainID, now: now)
    }

    @discardableResult
    public func scheduleFromPool(
        chainID: TaskChainID,
        date: LocalDate,
        today: LocalDate,
        now: Date = Date()
    ) throws -> DayTraceID {
        try ensureActiveChain(chainID)
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can be scheduled from pool")
        }
        guard date >= today else {
            throw NoonmarkError.invalidTransition("task-pool tasks cannot be scheduled into the past")
        }
        try ensureUnlockedDay(date)

        let definition = try currentDefinition(for: chainID)
        let chain = try chain(chainID)
        let trace = DayTrace(
            chainID: chainID,
            definitionID: definition.id,
            date: date,
            priority: nextPriority(on: date),
            descriptionText: definition.descriptionText,
            noteEntries: chain.activeNoteEntries,
            now: now
        )
        traces[trace.id] = trace
        copyPlannedSubtasks(from: definition, to: trace.id, now: now)
        ensureDay(date, now: now)
        return trace.id
    }

    @discardableResult
    public func deleteUnscheduledTask(
        chainID: TaskChainID,
        now: Date = Date()
    ) throws -> TaskPoolRemovalOutcome {
        guard traces.values.contains(where: { $0.chainID == chainID }) == false else {
            throw NoonmarkError.invalidTransition("scheduled tasks cannot be deleted")
        }
        guard chains[chainID] != nil else {
            throw NoonmarkError.notFound("task chain")
        }

        let keepsClassificationHistory =
            classificationState.referencesTaskChain(chainID)
        var chain = try chain(chainID)
        try chain.markContentModified(at: now)
        if keepsClassificationHistory {
            try removeCurrentClassificationForRetiredTask(chainID: chainID, now: now)
        }

        // “删除”是用户投影语义；存储与同步必须保留同一 chain／definition
        // 身份作为 append-only 取消事实，避免旧 creation record 在重启或
        // 其他设备上复活。
        chain.state = .abandoned
        chains[chainID] = chain
        return keepsClassificationHistory
            ? .removedKeepingHistory
            : .deleted
    }

    @discardableResult
    public func removeTaskFromPool(
        chainID: TaskChainID,
        now: Date = Date()
    ) throws -> TaskPoolRemovalOutcome {
        guard isInTaskPool(chainID) else {
            throw NoonmarkError.invalidTransition("only task-pool tasks can be removed")
        }

        let chainTraces = traces.values.filter { $0.chainID == chainID }
        guard chainTraces.isEmpty == false else {
            return try deleteUnscheduledTask(chainID: chainID, now: now)
        }

        var chain = try chain(chainID)
        try chain.markContentModified(at: now)
        try removeCurrentClassificationForRetiredTask(
            chainID: chainID,
            now: now
        )
        chain.state = .abandoned
        chains[chainID] = chain
        return .removedKeepingHistory
    }

    private func removeCurrentClassificationForRetiredTask(
        chainID: TaskChainID,
        reason: String = "task removed from task pool while preserving classification history",
        now: Date
    ) throws {
        guard let current = classificationState.currentByChainID[chainID],
              current.category != nil || current.labels.isEmpty == false
        else { return }

        let interactionID = UUID()
        let plan = try prepareClassification(
            .setCurrent(
                TaskClassificationDraft(
                    chainID: chainID,
                    category: nil,
                    labels: []
                )
            ),
            source: .deterministicDomainAction(reason: reason),
            interactionID: interactionID,
            now: now
        )
        try commitDeterministicDomainClassification(plan, now: now)
    }

    private func removeAutomaticClassificationForSnapshotUndo(
        chainID: TaskChainID,
        now: Date
    ) throws {
        guard let current = classificationState.currentByChainID[chainID]
        else { return }
        let category = current.category.flatMap { relation in
            guard case .automaticAI = relation.source else {
                return TaskCategoryChoice.existing(relation.categoryID)
            }
            return nil
        }
        let labels = current.labels.compactMap { relation in
            guard case .automaticAI = relation.source else {
                return TaskLabelChoice.existing(relation.labelID)
            }
            return nil
        }
        guard category != current.categoryID.map(TaskCategoryChoice.existing)
            || labels.count != current.labels.count
        else { return }
        let plan = try prepareClassification(
            .setCurrent(
                TaskClassificationDraft(
                    chainID: chainID,
                    category: category,
                    labels: labels
                )
            ),
            source: .deterministicDomainAction(
                reason: "snapshot undo cancelled a new task classification"
            ),
            interactionID: UUID(),
            now: now
        )
        try commitDeterministicDomainClassification(plan, now: now)
    }

    public func getDayTodo(date: LocalDate, sort: ViewSort = .priority) -> DayTodoView {
        let candidates = traces.values.filter {
            $0.date == date && $0.status.isVisibleInDayTodo
        }
        let newestDayFactCreatedAtByChain = traces.values
            .filter { $0.date == date && $0.formsDayHistory }
            .reduce(into: [TaskChainID: Date]()) { newest, trace in
                newest[trace.chainID] = max(
                    newest[trace.chainID] ?? trace.createdAt,
                    trace.createdAt
                )
            }
        let items = candidates.filter { trace in
            guard trace.status == .deferred,
                  let newestCreatedAt =
                  newestDayFactCreatedAtByChain[trace.chainID]
            else {
                return true
            }
            return newestCreatedAt <= trace.createdAt
        }
        return DayTodoView(day: days[date], traces: sorted(items, by: sort))
    }

    public func taskPool() -> [PoolTask] {
        let traceFactsByChain = chainTraceFactsByChain()
        let currentDefinitionsByChain = currentDefinitionsByChain()

        return chains.values
            .compactMap { chain in
                guard
                    isInTaskPool(
                        chain,
                        traceFacts: traceFactsByChain[chain.id]
                    ),
                    let definition = currentDefinitionsByChain[chain.id]
                else {
                    return nil
                }
                return PoolTask(chain: chain, definition: definition)
            }
            .sorted { $0.definition.createdAt < $1.definition.createdAt }
    }

    public func taskPoolCount() -> Int {
        let traceFactsByChain = chainTraceFactsByChain()
        let currentDefinitionsByChain = currentDefinitionsByChain()
        return chains.values.count { chain in
            currentDefinitionsByChain[chain.id] != nil
                && isInTaskPool(
                    chain,
                    traceFacts: traceFactsByChain[chain.id]
                )
        }
    }

    public func isRecurringTaskChain(_ chainID: TaskChainID) -> Bool {
        chains[chainID]?.cycleMembership != nil
    }

    public func taskTrail(chainID: TaskChainID) throws -> [TaskTrailEntry] {
        let chain = try chain(chainID)
        let changedSource = traces.values.first { source in
            guard let targetID = source.changedToTraceID,
                  let target = traces[targetID]
            else { return false }
            return target.chainID == chainID
        }
        let creationKind: TaskTrailEntryKind = changedSource == nil
            ? .createdInPool
            : .createdFromChange
        var entries = [
            TaskTrailEntry(
                id: "\(chain.id.description).created",
                chainID: chain.id,
                traceID: nil,
                relatedTraceID: changedSource?.id,
                kind: creationKind,
                date: nil,
                occurredAt: chain.createdAt
            )
        ]

        for trace in traces.values where trace.chainID == chainID {
            entries.append(
                TaskTrailEntry(
                    id: "\(trace.id.description).scheduled",
                    chainID: chainID,
                    traceID: trace.id,
                    kind: .scheduled,
                    date: trace.date,
                    occurredAt: trace.createdAt
                )
            )
            if carryoverKind(for: trace.id) == .continuation,
               let sourceID = trace.carriedFromTraceID,
               let source = traces[sourceID]
            {
                entries.append(
                    TaskTrailEntry(
                        id: "\(trace.id.description).continued",
                        chainID: chainID,
                        traceID: source.id,
                        relatedTraceID: trace.id,
                        kind: .continued,
                        date: source.date,
                        occurredAt: trace.createdAt
                    )
                )
            }

            guard let outcome = taskTrailOutcome(for: trace) else { continue }
            entries.append(
                TaskTrailEntry(
                    id: "\(trace.id.description).\(outcome.kind.rawValue)",
                    chainID: chainID,
                    traceID: trace.id,
                    relatedTraceID: outcome.relatedTraceID,
                    kind: outcome.kind,
                    date: trace.date,
                    occurredAt: outcome.occurredAt
                )
            )
        }

        return entries.sorted { lhs, rhs in
            if lhs.occurredAt != rhs.occurredAt {
                return lhs.occurredAt < rhs.occurredAt
            }
            let lhsOrder = taskTrailSortOrder(lhs.kind)
            let rhsOrder = taskTrailSortOrder(rhs.kind)
            if lhsOrder != rhsOrder {
                return lhsOrder < rhsOrder
            }
            return lhs.id < rhs.id
        }
    }

    public func futurePlans(today: LocalDate) -> [FuturePlanItem] {
        traces.values
            .filter { $0.status == .pending && $0.date > today }
            .compactMap { trace in
                guard let definition = definitions[trace.definitionID] else { return nil }
                return FuturePlanItem(trace: trace, definition: definition)
            }
            .sorted {
                if $0.trace.date == $1.trace.date {
                    return $0.trace.priority < $1.trace.priority
                }
                return $0.trace.date < $1.trace.date
            }
    }

    public func visibleFuturePlans(
        today: LocalDate,
        recurringVisibilityDays: Int
    ) -> [FuturePlanItem] {
        let recurringCutoff = (1 ... TaskCycleCivilCalendar
            .maximumMaterializedDayCount)
            .contains(recurringVisibilityDays)
            ? TaskCycleCivilCalendar.offset(
                today,
                by: recurringVisibilityDays
            )
            : nil
        return futurePlans(today: today).filter { item in
            guard isRecurringTaskChain(item.trace.chainID) else {
                return true
            }
            guard let recurringCutoff else {
                return false
            }
            return item.trace.date <= recurringCutoff
        }
    }

    public func unfinishedPool() -> [UnfinishedPoolItem] {
        let unresolvedStatuses: Set<TraceStatus> = [.unfinished, .abandoned]
        let unresolvedCandidatesByChain = Dictionary(grouping: traces.values.filter { unresolvedStatuses.contains($0.status) }) {
            $0.chainID
        }

        return unresolvedCandidatesByChain.compactMap { chainID, candidateTraces in
            let unfinishedTraces = candidateTraces.filter {
                $0.status == .unfinished || $0.status == .abandoned
            }

            guard
                unfinishedTraces.isEmpty == false,
                var chain = chains[chainID],
                isRecurringTaskChain(chainID) == false,
                chain.state == .active || chain.state == .abandoned,
                let definition = try? currentDefinition(for: chainID),
                hasCompletedTrace(chainID) == false
            else {
                return nil
            }

            let activeTrace = activeTrace(for: chainID)
            chain.updatedAt = unfinishedTraces.map(\.createdAt).max() ?? chain.updatedAt
            return UnfinishedPoolItem(
                chain: chain,
                definition: definition,
                unfinishedTraces: unfinishedTraces.sorted { $0.date < $1.date },
                activeTrace: activeTrace,
                isInTaskPool: isInTaskPool(chainID)
            )
        }
        .sorted { lhs, rhs in
            if lhs.chain.createdAt != rhs.chain.createdAt {
                return lhs.chain.createdAt < rhs.chain.createdAt
            }
            return lhs.definition.title.localizedStandardCompare(rhs.definition.title) == .orderedAscending
        }
    }

    public func unfinishedPoolCount() -> Int {
        let traceFactsByChain = chainTraceFactsByChain()
        let currentDefinitionsByChain = currentDefinitionsByChain()
        return chains.values.count { chain in
            guard let traceFacts = traceFactsByChain[chain.id] else {
                return false
            }
            return chain.cycleMembership == nil
                && (chain.state == .active || chain.state == .abandoned)
                && currentDefinitionsByChain[chain.id] != nil
                && traceFacts.hasUnfinishedCandidate
                && traceFacts.hasCompletedTrace == false
        }
    }

    public func completedPool() -> [CompletedPoolItem] {
        completedPoolItems().sorted {
            if $0.trace.date == $1.trace.date {
                return ($0.trace.completedAt ?? $0.trace.createdAt)
                    > ($1.trace.completedAt ?? $1.trace.createdAt)
            }
            return $0.trace.date > $1.trace.date
        }
    }

    private func completedPoolItems() -> [CompletedPoolItem] {
        let historyTracesByChain = Dictionary(
            grouping: traces.values.filter { $0.formsDayHistory },
            by: \.chainID
        ).mapValues { $0.sorted(by: traceChronology) }
        let presentableSubtasksByTraceID = Dictionary(
            grouping: subtasks.values.filter { $0.isUserPresentable },
            by: \.traceID
        )

        return traces.values
            .filter {
                $0.status == .completed
                    && isRecurringTaskChain($0.chainID) == false
            }
            .compactMap { trace in
                guard let definition = definitions[trace.definitionID] else { return nil }
                return CompletedPoolItem(
                    trace: trace,
                    definition: definition,
                    trajectory: completedTrajectory(
                        for: trace,
                        chainTraces: historyTracesByChain[trace.chainID] ?? [],
                        presentableSubtasksByTraceID: presentableSubtasksByTraceID
                    )
                )
            }
    }

    public func completedSubtaskRecords() -> [CompletedSubtaskRecord] {
        completedSubtaskRecordsUnsorted().sorted {
            if $0.date != $1.date {
                return $0.date > $1.date
            }
            if $0.subtask.position != $1.subtask.position {
                return $0.subtask.position < $1.subtask.position
            }
            return $0.subtask.createdAt < $1.subtask.createdAt
        }
    }

    private func completedSubtaskRecordsUnsorted() -> [CompletedSubtaskRecord] {
        subtasks.values
            .filter { $0.status == .completed }
            .compactMap { subtask -> CompletedSubtaskRecord? in
                guard
                    let trace = traces[subtask.traceID],
                    trace.formsDayHistory,
                    isRecurringTaskChain(trace.chainID) == false,
                    let definition = definitions[trace.definitionID]
                else {
                    return nil
                }
                return CompletedSubtaskRecord(
                    date: trace.date,
                    subtask: subtask,
                    parentTrace: trace,
                    parentDefinition: definition
                )
            }
    }

    public func completedTaskHierarchies() -> [CompletedTaskHierarchy] {
        let parentCompletionsByChain = Dictionary(
            grouping: completedPoolItems(),
            by: \.trace.chainID
        )
        let childrenByChain = Dictionary(
            grouping: completedSubtaskRecordsUnsorted(),
            by: \.parentTrace.chainID
        )
        let chainIDs = Set(parentCompletionsByChain.keys)
            .union(childrenByChain.keys)

        return chainIDs.compactMap { chainID in
            guard let chain = chains[chainID] else { return nil }
            let parentCompletion = parentCompletionsByChain[chainID]?
                .max {
                    ($0.trace.completedAt ?? $0.trace.createdAt)
                        < ($1.trace.completedAt ?? $1.trace.createdAt)
                }
            guard let definition =
                parentCompletion?.definition
                    ?? (try? currentDefinition(for: chainID))
            else {
                return nil
            }
            let completedChildren = latestCompletedChildren(
                from: childrenByChain[chainID] ?? []
            )
            return CompletedTaskHierarchy(
                chain: chain,
                definition: definition,
                parentCompletion: parentCompletion,
                completedChildren: completedChildren
            )
        }
        .sorted {
            if $0.latestCompletionAt != $1.latestCompletionAt {
                return $0.latestCompletionAt > $1.latestCompletionAt
            }
            let titleOrder = $0.definition.title.localizedStandardCompare(
                $1.definition.title
            )
            if titleOrder != .orderedSame {
                return titleOrder == .orderedAscending
            }
            return $0.chain.id.description < $1.chain.id.description
        }
    }

    public func completedTaskHierarchyCount() -> Int {
        var completedChainIDs: Set<TaskChainID> = []
        for trace in traces.values
        where trace.status == .completed
            && chains[trace.chainID]?.cycleMembership == nil
            && definitions[trace.definitionID] != nil
        {
            completedChainIDs.insert(trace.chainID)
        }
        for subtask in subtasks.values where subtask.status == .completed {
            guard
                let trace = traces[subtask.traceID],
                trace.formsDayHistory,
                chains[trace.chainID]?.cycleMembership == nil,
                definitions[trace.definitionID] != nil
            else {
                continue
            }
            completedChainIDs.insert(trace.chainID)
        }
        return completedChainIDs.count
    }

    private func latestCompletedChildren(
        from records: [CompletedSubtaskRecord]
    ) -> [CompletedSubtaskRecord] {
        Dictionary(grouping: records, by: \.subtask.lineageID)
            .values
            .compactMap { lineageRecords in
                lineageRecords.max {
                    let lhs = $0.subtask.completedAt
                        ?? $0.subtask.updatedAt
                    let rhs = $1.subtask.completedAt
                        ?? $1.subtask.updatedAt
                    if lhs != rhs {
                        return lhs < rhs
                    }
                    return $0.subtask.id.description
                        < $1.subtask.id.description
                }
            }
            .sorted {
                if $0.subtask.position != $1.subtask.position {
                    return $0.subtask.position < $1.subtask.position
                }
                if $0.subtask.createdAt != $1.subtask.createdAt {
                    return $0.subtask.createdAt < $1.subtask.createdAt
                }
                return $0.subtask.id.description
                    < $1.subtask.id.description
            }
    }

    public func completionCapability(
        for traceID: DayTraceID,
        today: LocalDate
    ) throws -> DayTraceCompletionCapability {
        completionCapability(
            for: try trace(traceID),
            today: today
        )
    }

    public func markCompleted(traceID: DayTraceID, today: LocalDate, now: Date = Date()) throws {
        var trace = try tracePermittedForCompletionAction(
            .complete,
            traceID: traceID,
            today: today
        )

        try trace.markContentModified(at: now)
        trace.status = .completed
        trace.pinOrder = nil
        trace.completedAt = now
        traces[trace.id] = trace
        try touchChain(trace.chainID, now: now)
    }

    public func undoCompleted(traceID: DayTraceID, today: LocalDate, now: Date = Date()) throws {
        var trace = try tracePermittedForCompletionAction(
            .undo,
            traceID: traceID,
            today: today
        )

        try trace.markContentModified(at: now)
        trace.status = .pending
        trace.pinOrder = nil
        trace.completedAt = nil
        traces[trace.id] = trace
        try touchChain(trace.chainID, now: now)
    }

    public func returnToPool(traceID: DayTraceID, today: LocalDate, now: Date = Date()) throws {
        var trace = try trace(traceID)
        guard isRecurringTaskChain(trace.chainID) == false else {
            throw NoonmarkError.invalidTransition(
                "recurring occurrences cannot return to the task pool"
            )
        }
        try ensureUnlockedDay(trace.date)

        if trace.date > today {
            guard trace.status == .pending else {
                throw NoonmarkError.invalidTransition(
                    "only pending future plans can return to pool"
                )
            }
            try materializePoolDraft(from: trace, now: now)
            try prepareSubtasksForHiddenTrace(
                traceID: trace.id,
                now: now
            )
            try trace.markContentModified(at: now)
            trace.status = .cancelledDraft
            trace.pinOrder = nil
            trace.settledAt = now
            trace.draftCancellationID = UUID()
            trace.draftCancelledOn = today
            traces[trace.id] = trace
            try touchChain(trace.chainID, now: now)
            return
        }

        guard trace.date == today else {
            throw NoonmarkError.immutableHistory
        }
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending current-day traces can return to pool")
        }

        try materializePoolDraft(from: trace, now: now)
        try trace.markContentModified(at: now)
        trace.status = .returnedToPool
        trace.pinOrder = nil
        trace.settledAt = now
        traces[trace.id] = trace
        captureClassificationSnapshot(traceID: trace.id, chainID: trace.chainID, now: now)
        try touchChain(trace.chainID, now: now)
    }

    /// Deletes a task that was first placed on the current Day Todo.
    ///
    /// The trace remains as an internal cancellation fact so a stale sync
    /// record cannot restore it, but it leaves every user-facing projection.
    public func deleteNewCurrentDayTask(
        traceID: DayTraceID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        var chain = try chain(trace.chainID)
        try ensureUnlockedDay(trace.date)
        guard isRecurringTaskChain(trace.chainID) == false else {
            throw NoonmarkError.invalidTransition(
                "recurring occurrences must be managed from recurring plans"
            )
        }
        guard chain.state == .active else {
            throw NoonmarkError.chainAbandoned
        }
        guard trace.date == today, trace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "only pending current-day tasks can be deleted"
            )
        }
        guard isNewDayTrace(trace) else {
            throw NoonmarkError.invalidTransition(
                "only tasks newly added to the current day can be deleted"
            )
        }

        try trace.markContentModified(at: now)
        try chain.markContentModified(at: now)
        trace.status = .cancelledDraft
        trace.pinOrder = nil
        trace.settledAt = now
        trace.draftCancellationID = UUID()
        trace.draftCancelledOn = today
        try removeCurrentClassificationForRetiredTask(
            chainID: trace.chainID,
            reason: "new current-day task deleted",
            now: now
        )
        chain.state = .abandoned
        traces[trace.id] = trace
        chains[chain.id] = chain
    }

    public func canDeleteNewCurrentDayTask(
        traceID: DayTraceID,
        today: LocalDate
    ) -> Bool {
        guard let trace = traces[traceID],
              let chain = chains[trace.chainID],
              isRecurringTaskChain(trace.chainID) == false,
              chain.state == .active,
              days[trace.date]?.lockedAt == nil,
              trace.date == today,
              trace.status == .pending
        else {
            return false
        }
        return isNewDayTrace(trace)
    }

    public func rescheduleFuturePlan(traceID: DayTraceID, targetDate: LocalDate, today: LocalDate, now: Date = Date()) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        try ensureUnlockedDay(targetDate)
        guard trace.date > today, targetDate > today else {
            throw NoonmarkError.invalidTransition("only future plans can be rescheduled to future dates")
        }
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending future plans can be rescheduled")
        }

        try trace.markContentModified(at: now)
        trace.date = targetDate
        trace.priority = nextPriority(on: targetDate)
        trace.pinOrder = nil
        traces[trace.id] = trace
        ensureDay(targetDate, now: now)
    }

    @discardableResult
    public func deferCurrentTrace(
        traceID: DayTraceID,
        targetDate: LocalDate,
        today: LocalDate,
        now: Date = Date()
    ) throws -> DayTraceID {
        var source = try trace(traceID)
        try ensureActiveChain(source.chainID)
        try ensureUnlockedDay(source.date)
        try ensureUnlockedDay(targetDate)
        guard source.status == .pending, source.date == today else {
            throw NoonmarkError.invalidTransition("only a current pending trace can be deferred")
        }
        guard targetDate > today else {
            throw NoonmarkError.invalidTransition("deferral target must be after today")
        }
        let frontier = traces.values
            .filter { $0.chainID == source.chainID && $0.formsDayHistory }
            .sorted(by: traceChronology)
            .last
        guard frontier?.id == source.id else {
            throw NoonmarkError.invalidTransition("only the latest task-chain frontier can be deferred")
        }
        if let active = activeTrace(for: source.chainID), active.id != source.id {
            throw NoonmarkError.activeTraceAlreadyExists
        }

        let definition = try currentDefinition(for: source.chainID)
        let nextTrace = DayTrace(
            chainID: source.chainID,
            definitionID: definition.id,
            date: targetDate,
            priority: nextPriority(on: targetDate),
            continuationSeq: source.continuationSeq,
            descriptionText: source.descriptionText,
            noteEntries: source.activeNoteEntries,
            manualProgressPercent: traceProgress(for: source.id).percent,
            carriedFromTraceID: source.id,
            now: now
        )

        try source.markContentModified(at: now)
        source.status = .deferred
        source.pinOrder = nil
        source.settledAt = now
        traces[source.id] = source
        traces[nextTrace.id] = nextTrace
        captureClassificationSnapshot(traceID: source.id, chainID: source.chainID, now: now)

        try copyOpenSubtasks(
            from: source.id,
            to: nextTrace.id,
            kind: .deferral,
            now: now
        )
        ensureDay(targetDate, now: now)
        try touchChain(source.chainID, now: now)
        return nextTrace.id
    }

    @discardableResult
    public func continueUnfinishedTrace(
        traceID: DayTraceID,
        targetDate: LocalDate,
        today: LocalDate,
        now: Date = Date()
    ) throws -> DayTraceID {
        let source = try trace(traceID)
        try ensureActiveChain(source.chainID)
        try ensureUnlockedDay(targetDate)
        guard source.status == .unfinished,
              source.date < today || days[source.date]?.lockedAt != nil
        else {
            throw NoonmarkError.invalidTransition("only a locked unfinished trace can continue")
        }
        guard targetDate >= today, targetDate > source.date else {
            throw NoonmarkError.invalidTransition("continuation target must follow its source and cannot be in the past")
        }
        let frontier = traces.values
            .filter { $0.chainID == source.chainID && $0.formsDayHistory }
            .sorted(by: traceChronology)
            .last
        guard frontier?.id == source.id else {
            throw NoonmarkError.invalidTransition("only the latest task-chain frontier can continue")
        }
        guard activeTrace(for: source.chainID) == nil else {
            throw NoonmarkError.activeTraceAlreadyExists
        }
        guard isInTaskPool(source.chainID) == false else {
            throw NoonmarkError.invalidTransition(
                "task-pool tasks must be scheduled from the task pool"
            )
        }

        let definition = try currentDefinition(for: source.chainID)
        let nextTrace = DayTrace(
            chainID: source.chainID,
            definitionID: definition.id,
            date: targetDate,
            priority: nextPriority(on: targetDate),
            continuationSeq: source.continuationSeq + 1,
            descriptionText: source.descriptionText,
            noteEntries: source.activeNoteEntries,
            manualProgressPercent: traceProgress(for: source.id).percent,
            carriedFromTraceID: source.id,
            now: now
        )
        traces[nextTrace.id] = nextTrace
        try copyOpenSubtasks(
            from: source.id,
            to: nextTrace.id,
            kind: .continuation,
            now: now
        )
        ensureDay(targetDate, now: now)
        try touchChain(source.chainID, now: now)
        return nextTrace.id
    }

    public func carryoverKind(for traceID: DayTraceID) -> TraceCarryoverKind? {
        guard let sourceID = traces[traceID]?.carriedFromTraceID,
              let source = traces[sourceID]
        else {
            return nil
        }
        return switch source.status {
        case .deferred:
            .deferral
        case .unfinished:
            .continuation
        case .pending, .completed, .changed, .returnedToPool, .cancelledDraft, .abandoned:
            nil
        }
    }

    public func carryoverTarget(for sourceTraceID: DayTraceID) -> DayTrace? {
        traces.values.first { $0.carriedFromTraceID == sourceTraceID }
    }

    public func canWithdrawDeferral(
        sourceTraceID: DayTraceID,
        today: LocalDate
    ) -> Bool {
        guard let source = traces[sourceTraceID],
              source.status == .deferred,
              source.date == today,
              days[source.date]?.lockedAt == nil,
              chains[source.chainID]?.state == .active
        else {
            return false
        }
        let targets = traces.values.filter {
            $0.carriedFromTraceID == source.id
        }
        guard targets.count == 1, let target = targets.first else {
            return false
        }
        return target.status == .pending
            && target.date > today
            && days[target.date]?.lockedAt == nil
            && activeTrace(for: source.chainID)?.id == target.id
            && carryoverKind(for: target.id) == .deferral
    }

    public func withdrawDeferral(
        sourceTraceID: DayTraceID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        guard canWithdrawDeferral(
            sourceTraceID: sourceTraceID,
            today: today
        ) else {
            throw NoonmarkError.invalidTransition(
                "only a current-day deferral with an active future target can be withdrawn"
            )
        }
        var source = try trace(sourceTraceID)
        guard var target = traces.values.first(where: {
            $0.carriedFromTraceID == source.id && $0.status == .pending
        }) else {
            throw NoonmarkError.notFound("deferral target")
        }

        try source.markContentModified(at: now)
        try target.markContentModified(at: now)
        source.definitionID = target.definitionID
        source.descriptionText = target.descriptionText
        source.noteEntries = target.noteEntries
        source.manualProgressPercent = target.manualProgressPercent
        source.status = .pending
        source.pinOrder = nil
        source.completedAt = nil
        source.settledAt = nil

        let targetSubtasks = subtasks.values
            .filter { $0.traceID == target.id }
            .sorted { lhs, rhs in
                if lhs.position != rhs.position {
                    return lhs.position < rhs.position
                }
                return lhs.id.description < rhs.id.description
            }
        for targetSubtaskValue in targetSubtasks {
            var targetSubtask = targetSubtaskValue
            if let sourceSubtaskID = targetSubtask.carriedFromSubtaskID,
               var sourceSubtask = subtasks[sourceSubtaskID]
            {
                try sourceSubtask.markContentModified(at: now)
                if targetSubtask.isUserPresentable {
                    sourceSubtask.title = targetSubtask.title
                    sourceSubtask.difficulty = targetSubtask.difficulty
                    sourceSubtask.position = targetSubtask.position
                    sourceSubtask.status = .pending
                    sourceSubtask.completedAt = nil
                    sourceSubtask.settledAt = nil
                    sourceSubtask.draftCancellationID = nil
                } else {
                    sourceSubtask.status = .cancelledDraft
                    sourceSubtask.completedAt = nil
                    sourceSubtask.settledAt = now
                    sourceSubtask.draftCancellationID = UUID()
                }
                subtasks[sourceSubtask.id] = sourceSubtask

                if targetSubtask.status == .cancelledDraft {
                    try targetSubtask.markContentModified(at: now)
                    targetSubtask.status = .pending
                    targetSubtask.completedAt = nil
                    targetSubtask.settledAt = nil
                    targetSubtask.draftCancellationID = nil
                    targetSubtask.carriedFromSubtaskID = nil
                    subtasks[targetSubtask.id] = targetSubtask
                }
            } else if targetSubtask.isUserPresentable {
                try targetSubtask.markContentModified(at: now)
                targetSubtask.traceID = source.id
                targetSubtask.carriedFromSubtaskID = nil
                subtasks[targetSubtask.id] = targetSubtask
            }
        }

        target.status = .cancelledDraft
        target.pinOrder = nil
        target.completedAt = nil
        target.settledAt = now
        target.draftCancellationID = UUID()
        target.draftCancelledOn = target.date
        traces[source.id] = source
        traces[target.id] = target
        try touchChain(source.chainID, now: now)
    }

    public func pinDayTrace(
        _ traceID: DayTraceID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today, trace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "only current or future pending day traces can be pinned"
            )
        }
        guard trace.pinOrder == nil else { return }
        let nextPinOrder = (
            traces.values
                .filter {
                    $0.date == trace.date
                        && $0.status == .pending
                }
                .compactMap(\.pinOrder)
                .max() ?? 0
        ) + 1
        guard nextPinOrder > 0 else {
            throw NoonmarkError.invalidInput("day trace pin queue is exhausted")
        }
        try trace.markContentModified(at: now)
        trace.pinOrder = nextPinOrder
        traces[trace.id] = trace
    }

    public func unpinDayTrace(
        _ traceID: DayTraceID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today, trace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "only current or future pending day traces can be unpinned"
            )
        }
        guard trace.pinOrder != nil else { return }
        try trace.markContentModified(at: now)
        trace.pinOrder = nil
        traces[trace.id] = trace
    }

    @discardableResult
    public func changeTrace(
        traceID: DayTraceID,
        newTitle: String,
        newDescriptionText: String? = nil,
        initialNoteBody: String? = nil,
        today: LocalDate,
        now: Date = Date()
    ) throws -> DayTraceID {
        let normalizedTitle = try normalizeTitle(newTitle)
        var oldTrace = try trace(traceID)
        try ensureActiveChain(oldTrace.chainID)
        try ensureUnlockedDay(oldTrace.date)
        guard oldTrace.date == today, oldTrace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending current-day traces can change definition")
        }

        var noteBodies = oldTrace.activeNoteEntries.map(\.body)
        if let initialNote = normalizedOptionalText(initialNoteBody) {
            noteBodies.append(initialNote)
        }
        let noteEntries = try noteBodies.map {
            try TaskNoteEntry(body: $0, now: now)
        }
        let newChain = TaskChain(noteEntries: noteEntries, now: now)
        let newDefinition = TaskDefinition(
            chainID: newChain.id,
            sequence: 1,
            title: normalizedTitle,
            descriptionText: newDescriptionText ?? oldTrace.descriptionText,
            now: now
        )

        let newTrace = DayTrace(
            chainID: newChain.id,
            definitionID: newDefinition.id,
            date: today,
            priority: nextPriority(on: today),
            descriptionText: newDefinition.descriptionText,
            noteEntries: newChain.activeNoteEntries,
            now: now
        )

        try oldTrace.markContentModified(at: now)
        oldTrace.status = .changed
        oldTrace.pinOrder = nil
        oldTrace.changedToTraceID = newTrace.id
        oldTrace.settledAt = now

        chains[newChain.id] = newChain
        definitions[newDefinition.id] = newDefinition
        traces[oldTrace.id] = oldTrace
        traces[newTrace.id] = newTrace
        copyOpenSubtasksToChangedTask(from: oldTrace.id, to: newTrace.id, now: now)
        captureClassificationSnapshot(traceID: oldTrace.id, chainID: oldTrace.chainID, now: now)
        try inheritCurrentClassification(from: oldTrace.chainID, to: newChain.id, now: now)
        try touchChain(oldTrace.chainID, now: now)
        return newTrace.id
    }

    public func abandonChain(from traceID: DayTraceID, now: Date = Date()) throws {
        let requestedTrace = try trace(traceID)
        var chain = try chain(requestedTrace.chainID)
        guard chain.state == .active else {
            throw NoonmarkError.chainAbandoned
        }
        var trace = activeTrace(for: requestedTrace.chainID) ?? requestedTrace
        guard trace.status == .pending || trace.status == .unfinished else {
            throw NoonmarkError.invalidTransition("only pending or unfinished traces can abandon their chain")
        }

        try trace.markContentModified(at: now)
        try chain.markContentModified(at: now)
        trace.status = .abandoned
        trace.pinOrder = nil
        trace.settledAt = now
        chain.state = .abandoned

        traces[trace.id] = trace
        chains[chain.id] = chain
        captureClassificationSnapshot(traceID: trace.id, chainID: trace.chainID, now: now)
    }

    @discardableResult
    public func reactivateAbandonedChain(
        from traceID: DayTraceID,
        today: LocalDate,
        now: Date = Date()
    ) throws -> DayTraceID {
        var source = try trace(traceID)
        var chain = try chain(source.chainID)
        guard chain.state == .abandoned, source.status == .abandoned else {
            throw NoonmarkError.invalidTransition("only abandoned chains can be reactivated")
        }
        if activeTrace(for: source.chainID) != nil {
            throw NoonmarkError.activeTraceAlreadyExists
        }

        try source.markContentModified(at: now)
        try chain.markContentModified(at: now)
        if days[source.date]?.lockedAt != nil {
            source.status = .unfinished
            source.settledAt = source.settledAt ?? now
        } else if source.date >= today {
            source.status = .pending
            source.settledAt = nil
        } else {
            source.status = .unfinished
            source.settledAt = source.settledAt ?? now
        }
        source.pinOrder = nil
        chain.state = .active

        traces[source.id] = source
        chains[chain.id] = chain
        if source.status == .unfinished {
            captureClassificationSnapshot(traceID: source.id, chainID: source.chainID, now: now)
        }
        return source.id
    }

    @discardableResult
    public func copyAsNewTask(
        from traceID: DayTraceID,
        target: NewTaskTarget,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskChainID {
        let source = try trace(traceID)
        let sourceDefinition = try definition(source.definitionID)
        let sourceNotes = source.activeNoteEntries
        let newChainID = try createPoolTask(
            title: sourceDefinition.title,
            descriptionText: source.descriptionText ?? sourceDefinition.descriptionText,
            initialNoteBody: sourceNotes.first?.body,
            now: now
        )
        for note in sourceNotes.dropFirst() {
            _ = try appendPoolNote(chainID: newChainID, body: note.body, now: now)
        }

        if case let .date(date) = target {
            _ = try scheduleFromPool(chainID: newChainID, date: date, today: today, now: now)
        }

        try inheritCurrentClassification(from: source.chainID, to: newChainID, now: now)
        return newChainID
    }

    @discardableResult
    public func addSubtask(
        traceID: DayTraceID,
        title: String,
        difficulty: SubtaskDifficulty = .simple,
        now: Date = Date()
    ) throws -> SubtaskID {
        let normalizedTitle = try normalizeTitle(title)
        let trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("subtasks can only be added to pending traces")
        }

        let position = subtasks.values.filter { $0.traceID == traceID }.count + 1
        let subtask = Subtask(
            traceID: trace.id,
            title: normalizedTitle,
            difficulty: difficulty,
            position: position,
            now: now
        )
        subtasks[subtask.id] = subtask
        return subtask.id
    }

    public func completeSubtask(_ subtaskID: SubtaskID, today: LocalDate, now: Date = Date()) throws {
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date == today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending subtasks can be completed")
        }
        try subtask.markContentModified(at: now)
        subtask.status = .completed
        subtask.completedAt = now
        subtasks[subtask.id] = subtask
    }

    public func undoCompletedSubtask(
        _ subtaskID: SubtaskID,
        today: LocalDate,
        now: Date
    ) throws {
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date == today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.status == .completed else {
            throw NoonmarkError.invalidTransition("only completed subtasks can be undone")
        }

        try subtask.markContentModified(at: now)
        subtask.status = .pending
        subtask.completedAt = nil
        subtasks[subtask.id] = subtask
    }

    public func abandonSubtask(_ subtaskID: SubtaskID, today: LocalDate, now: Date = Date()) throws {
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date == today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending subtasks can be abandoned")
        }
        try subtask.markContentModified(at: now)
        subtask.status = .abandoned
        subtask.settledAt = now
        subtasks[subtask.id] = subtask
    }

    public func updateSubtaskDifficulty(
        _ subtaskID: SubtaskID,
        difficulty: SubtaskDifficulty,
        today: LocalDate,
        now: Date
    ) throws {
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.status != .cancelledDraft else {
            throw NoonmarkError.notFound("subtask")
        }

        guard subtask.difficulty != difficulty else { return }
        try subtask.markContentModified(at: now)
        subtask.difficulty = difficulty
        subtasks[subtask.id] = subtask
    }

    public func updateSubtaskTitle(
        _ subtaskID: SubtaskID,
        title: String,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        let normalizedTitle = try normalizeTitle(title)
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.isUserPresentable else {
            throw NoonmarkError.notFound("subtask")
        }
        guard subtask.title != normalizedTitle else { return }

        try subtask.markContentModified(at: now)
        subtask.title = normalizedTitle
        subtasks[subtask.id] = subtask
    }

    public func deleteSubtask(
        _ subtaskID: SubtaskID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        guard var subtask = subtasks[subtaskID] else {
            throw NoonmarkError.notFound("subtask")
        }
        let trace = try trace(subtask.traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard subtask.isUserPresentable else {
            throw NoonmarkError.notFound("subtask")
        }

        try subtask.markContentModified(at: now)
        subtask.status = .cancelledDraft
        subtask.completedAt = nil
        subtask.settledAt = now
        subtask.draftCancellationID = UUID()
        subtasks[subtask.id] = subtask
    }

    public func updateTraceText(
        traceID: DayTraceID,
        descriptionText: String?,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending traces can update descriptive text")
        }
        guard trace.date >= today else {
            throw NoonmarkError.immutableHistory
        }

        let nextDescription = optionalTextPreservingWhitespace(descriptionText)
        guard trace.descriptionText != nextDescription else { return }
        try trace.markContentModified(at: now)
        trace.descriptionText = nextDescription
        traces[trace.id] = trace
    }

    public func editTraceNote(
        traceID: DayTraceID,
        noteID: TaskNoteEntryID,
        body: String,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending traces can edit notes")
        }
        guard trace.date >= today else {
            throw NoonmarkError.immutableHistory
        }
        try trace.noteEntries.editTaskNote(id: noteID, body: body, now: now)
        traces[trace.id] = trace
    }

    @discardableResult
    public func appendTraceNote(
        traceID: DayTraceID,
        body: String,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskNoteEntryID {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending traces can append notes")
        }
        guard trace.date >= today else {
            throw NoonmarkError.immutableHistory
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: trace.createdAt)
        let note = try TaskNoteEntry(body: body, now: now)
        trace.noteEntries.append(note)
        traces[trace.id] = trace
        return note.id
    }

    public func deleteTraceNote(
        traceID: DayTraceID,
        noteID: TaskNoteEntryID,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition("only pending traces can delete notes")
        }
        guard trace.date >= today else {
            throw NoonmarkError.immutableHistory
        }
        try trace.noteEntries.deleteTaskNote(id: noteID, now: now)
        traces[trace.id] = trace
    }

    public func setManualProgress(
        traceID: DayTraceID,
        percent: Int,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date == today, trace.status == .pending else {
            throw NoonmarkError.immutableHistory
        }
        guard hasSubtasks(in: trace.chainID) == false else {
            throw NoonmarkError.invalidTransition("manual progress is unavailable when subtasks define weighted progress")
        }

        let clamped = min(100, max(0, percent))
        let floor = progressFloor(for: trace)
        let nextProgress = max(floor, clamped)
        guard trace.manualProgressPercent != nextProgress else { return }
        try trace.markContentModified(at: now)
        trace.manualProgressPercent = nextProgress
        traces[trace.id] = trace
    }

    public func subtaskProgress(for traceID: DayTraceID) -> SubtaskProgress {
        let items = subtasks.values.filter {
            $0.traceID == traceID && $0.isUserPresentable
        }
        return SubtaskProgress(
            total: items.count,
            completed: items.filter { $0.status == .completed }.count,
            pending: items.filter { $0.status == .pending }.count,
            unfinished: items.filter { $0.status == .unfinished }.count,
            deferred: items.filter { $0.status == .deferred }.count,
            abandoned: items.filter { $0.status == .abandoned }.count
        )
    }

    public func traceProgress(for traceID: DayTraceID) -> TraceProgress {
        guard let trace = traces[traceID] else {
            return TraceProgress(mode: .manual, percent: 0, floorPercent: 0, completedWeight: 0, totalWeight: 0)
        }

        let chainTraces = traces.values
            .filter {
                $0.chainID == trace.chainID && $0.formsDayHistory
            }
            .sorted(by: traceChronology)
        let raw = rawProgress(for: trace, chainTraces: chainTraces)
        let floor = progressFloor(for: trace, chainTraces: chainTraces)
        return TraceProgress(
            mode: raw.mode,
            percent: max(raw.percent, floor),
            floorPercent: floor,
            completedWeight: raw.completedWeight,
            totalWeight: raw.totalWeight
        )
    }

    public func calendarSummary(for date: LocalDate) -> CalendarDaySummary {
        let items = calendarTraces(for: date)
        let completed = items.filter { $0.status == .completed }.count
        return CalendarDaySummary(
            date: date,
            total: items.count,
            completed: completed,
            pending: items.filter { $0.status == .pending }.count,
            unfinished: items.filter { $0.status == .unfinished }.count,
            heatLevel: min(completed, 4)
        )
    }

    public func calendarTraces(for date: LocalDate) -> [DayTrace] {
        getDayTodo(date: date).traces.filter {
            isRecurringTaskChain($0.chainID) == false
        }
    }

    public func updatePriority(
        traceID: DayTraceID,
        newPriority: Int,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        let trace = try trace(traceID)
        try ensureUnlockedDay(trace.date)
        guard trace.date >= today else {
            throw NoonmarkError.immutableHistory
        }
        guard trace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "only pending day traces can change priority"
            )
        }
        let pendingSlots = pendingPrioritySlots(on: trace.date)
        guard let currentIndex = pendingSlots.firstIndex(where: { $0.id == traceID }) else {
            throw NoonmarkError.notFound("day trace priority")
        }
        let targetIndices = pendingSlots.indices.filter {
            pendingSlots[$0].priority == newPriority
        }
        guard targetIndices.count == 1, let targetIndex = targetIndices.first else {
            throw NoonmarkError.invalidInput(
                "priority must reference an existing pending day-trace slot"
            )
        }
        guard currentIndex != targetIndex else { return }

        let targetTraceID: DayTraceID?
        if targetIndex < currentIndex {
            targetTraceID = pendingSlots[targetIndex].id
        } else {
            let followingIndex = targetIndex + 1
            targetTraceID = pendingSlots.indices.contains(followingIndex)
                ? pendingSlots[followingIndex].id
                : nil
        }
        try reorderDayTrace(
            traceID,
            before: targetTraceID,
            today: today,
            now: now
        )
    }

    public func reorderDayTrace(
        _ traceID: DayTraceID,
        before targetTraceID: DayTraceID?,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        let movingTrace = try trace(traceID)
        try ensureUnlockedDay(movingTrace.date)
        guard movingTrace.date >= today else {
            throw NoonmarkError.immutableHistory
        }
        guard movingTrace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "only pending day traces can change priority"
            )
        }
        try validatePriorityTarget(
            targetTraceID,
            movingTrace: movingTrace
        )
        guard targetTraceID != traceID else { return }

        let pendingSlots = pendingPrioritySlots(on: movingTrace.date)
        let prioritySlots = pendingSlots.map(\.priority)
        var orderedIDs = pendingSlots.map(\.id)
        guard let movingIndex = orderedIDs.firstIndex(of: traceID) else {
            throw NoonmarkError.notFound("day trace priority")
        }
        orderedIDs.remove(at: movingIndex)
        if let targetTraceID, let targetIndex = orderedIDs.firstIndex(of: targetTraceID) {
            orderedIDs.insert(traceID, at: targetIndex)
        } else {
            orderedIDs.append(traceID)
        }

        var reordered: [DayTrace] = []
        reordered.reserveCapacity(orderedIDs.count)
        for (id, priority) in zip(orderedIDs, prioritySlots) {
            var candidate = try trace(id)
            if candidate.priority != priority {
                try candidate.markContentModified(at: now)
                candidate.priority = priority
            }
            reordered.append(candidate)
        }
        for candidate in reordered {
            traces[candidate.id] = candidate
        }
    }

    public func settleDays(upTo today: LocalDate, now: Date = Date()) throws {
        let dates = Set(
            traces.values
                .filter(\.formsDayHistory)
                .map(\.date)
        ).filter { $0 < today }
        for date in dates where days[date]?.lockedAt == nil {
            if let day = days[date] {
                guard now.timeIntervalSinceReferenceDate.isFinite,
                      now >= day.updatedAt
                else {
                    throw NoonmarkError.invalidInput(
                        "day settlement time cannot move backwards"
                    )
                }
            }
            for trace in traces.values where trace.date == date && trace.status == .pending {
                var candidate = trace
                try candidate.markContentModified(at: now)
            }
        }

        for date in dates {
            ensureDay(date, now: now)

            guard days[date]?.lockedAt == nil else {
                continue
            }

            var traceIDsNeedingClassificationSnapshot: Set<DayTraceID> = []
            for trace in traces.values where trace.date == date && trace.status == .pending {
                var settled = trace
                try settled.markContentModified(at: now)
                settled.status = .unfinished
                settled.pinOrder = nil
                settled.settledAt = now
                traces[settled.id] = settled
                traceIDsNeedingClassificationSnapshot.insert(settled.id)
                try settleOpenSubtasks(on: settled.id, now: now)
            }

            for trace in traces.values where trace.date == date && trace.status == .completed {
                traceIDsNeedingClassificationSnapshot.insert(trace.id)
            }

            for traceID in traceIDsNeedingClassificationSnapshot {
                guard let trace = traces[traceID] else { continue }
                captureClassificationSnapshot(traceID: trace.id, chainID: trace.chainID, now: now)
            }

            days[date]?.lockedAt = now
            days[date]?.updatedAt = now
        }
    }

    public func dailyReviewStats(date: LocalDate) -> DailyReviewStats {
        reviewStats(for: getDayTodo(date: date).traces)
    }

    public func calendarReviewStats(date: LocalDate) -> DailyReviewStats {
        reviewStats(for: calendarTraces(for: date))
    }

    public func updateDailyReview(
        date: LocalDate,
        summary: String?,
        unfinishedReason: String?,
        tomorrowNote: String?,
        now: Date = Date()
    ) {
        ensureDay(date, now: now)
        days[date]?.reviewSummary = summary
        days[date]?.reviewUnfinishedReason = unfinishedReason
        days[date]?.reviewTomorrowNote = tomorrowNote
        days[date]?.updatedAt = now
    }

    public func updateTheme(
        _ theme: AppTheme,
        writerID: String = AppPreferences.defaultLocalThemeLanguageWriterID,
        now: Date
    ) throws {
        guard preferences.theme != theme else { return }
        try advanceThemeLanguageClock(to: now, writerID: writerID)
        preferences.theme = theme
    }

    public func updateLanguage(
        _ language: AppLanguage,
        writerID: String = AppPreferences.defaultLocalThemeLanguageWriterID,
        now: Date
    ) throws {
        guard preferences.language != language else { return }
        try advanceThemeLanguageClock(to: now, writerID: writerID)
        preferences.language = language
    }

    public func updateDataMode(_ dataMode: AppDataMode) {
        preferences.dataMode = dataMode
        if dataMode == .localFirst {
            preferences.backupPolicy = ScheduledBackupPolicy()
        }
    }

    public func updateBackupPolicy(_ backupPolicy: ScheduledBackupPolicy) {
        preferences.backupPolicy = backupPolicy
    }

    public func updateLocalFirstSyncPolicy(_ policy: LocalFirstCloudSyncPolicy) {
        preferences.localFirstSyncPolicy = policy
    }

    public func updateSettingsPoemDisplayPolicy(_ policy: SettingsPoemDisplayPolicy) {
        preferences.settingsPoemDisplayPolicy = policy
    }

    public func syncEndpointOptions() -> [SyncEndpointOption] {
        preferences.syncEndpointOptions
    }

    private func advanceThemeLanguageClock(
        to now: Date,
        writerID: String
    ) throws {
        let currentSeconds = preferences.themeLanguageUpdatedAt
            .timeIntervalSinceReferenceDate
        let nextSeconds = now.timeIntervalSinceReferenceDate
        let trimmedWriterID = writerID.trimmingCharacters(
            in: .whitespacesAndNewlines
        )
        guard currentSeconds.isFinite,
              nextSeconds.isFinite,
              now > preferences.themeLanguageUpdatedAt,
              trimmedWriterID.isEmpty == false
        else {
            throw NoonmarkError.invalidInput(
                "theme and language mutation time must advance"
            )
        }
        preferences = try preferences.applyingSyncedThemeLanguage(
            theme: preferences.theme,
            language: preferences.language,
            updatedAt: now,
            writerID: writerID
        )
    }
}

private struct SnapshotUndoCurrentFacts {
    let days: [LocalDate: Day]
    let chains: [TaskChainID: TaskChain]
    let definitions: [TaskDefinitionID: TaskDefinition]
    let traces: [DayTraceID: DayTrace]
    let subtasks: [SubtaskID: Subtask]

    init(_ snapshot: NoonmarkSnapshot) {
        days = Dictionary(
            uniqueKeysWithValues: snapshot.days.map { ($0.date, $0) }
        )
        chains = Dictionary(
            uniqueKeysWithValues: snapshot.chains.map { ($0.id, $0) }
        )
        definitions = Dictionary(
            uniqueKeysWithValues: snapshot.definitions.map { ($0.id, $0) }
        )
        traces = Dictionary(
            uniqueKeysWithValues: snapshot.traces.map { ($0.id, $0) }
        )
        subtasks = Dictionary(
            uniqueKeysWithValues: snapshot.subtasks.map { ($0.id, $0) }
        )
    }
}

private extension NoonmarkEngine {
    func taskTrailOutcome(
        for trace: DayTrace
    ) -> (kind: TaskTrailEntryKind, occurredAt: Date, relatedTraceID: DayTraceID?)? {
        switch trace.status {
        case .pending:
            nil
        case .completed:
            (.completed, trace.completedAt ?? trace.contentUpdatedAt, nil)
        case .unfinished:
            (.unfinished, trace.settledAt ?? trace.contentUpdatedAt, nil)
        case .deferred:
            (
                .deferred,
                trace.contentUpdatedAt,
                carryoverTarget(for: trace.id)?.id
            )
        case .changed:
            (.changed, trace.settledAt ?? trace.contentUpdatedAt, trace.changedToTraceID)
        case .returnedToPool, .cancelledDraft:
            (.returnedToPool, trace.settledAt ?? trace.contentUpdatedAt, nil)
        case .abandoned:
            (.abandoned, trace.settledAt ?? trace.contentUpdatedAt, nil)
        }
    }

    func taskTrailSortOrder(_ kind: TaskTrailEntryKind) -> Int {
        switch kind {
        case .createdInPool, .createdFromChange:
            0
        case .scheduled:
            1
        case .completed, .unfinished, .deferred, .continued, .changed, .returnedToPool, .abandoned:
            2
        }
    }

    func snapshotUndoOutcome(
        from current: SnapshotUndoCurrentFacts
    ) -> SnapshotUndoOutcome {
        let cancelled: [TaskChainID] = current.chains.compactMap { entry -> TaskChainID? in
            let (chainID, chain) = entry
            guard chain.state == .active,
                  chains[chainID]?.state != .active
            else { return nil }
            return chainID
        }
        .sorted { $0.description < $1.description }
        let restored: [TaskChainID] = chains.compactMap { entry -> TaskChainID? in
            let (chainID, chain) = entry
            guard chain.state == .active,
                  current.chains[chainID]?.state == .abandoned
            else { return nil }
            return chainID
        }
        .sorted { $0.description < $1.description }
        return SnapshotUndoOutcome(
            automaticClassificationCancelledChainIDs: cancelled,
            automaticClassificationRestoredChainIDs: restored
        )
    }

    func retainCurrentOnlySnapshotUndoFacts(
        from current: SnapshotUndoCurrentFacts,
        now: Date
    ) {
        let targetChainIDs = Set(chains.keys)
        let targetCurrentDefinitions = snapshotUndoCurrentDefinitionsByChain()
        let currentOnlyTraceIDs = Set(current.traces.keys).subtracting(
            traces.keys
        )

        retainCurrentOnlyDays(from: current.days)
        retainCurrentOnlyChains(from: current.chains)
        retainCurrentOnlyDefinitions(
            from: current.definitions,
            targetChainIDs: targetChainIDs,
            targetCurrentDefinitions: targetCurrentDefinitions,
            now: now
        )
        retainCurrentOnlyTraces(
            from: current.traces,
            ids: currentOnlyTraceIDs,
            now: now
        )
        retainCurrentOnlySubtasks(
            from: current.subtasks,
            currentOnlyTraceIDs: currentOnlyTraceIDs,
            now: now
        )
    }

    func retainCurrentOnlyDays(from currentDays: [LocalDate: Day]) {
        for (date, day) in currentDays where days[date] == nil {
            days[date] = day
        }
    }

    func retainCurrentOnlyChains(
        from currentChains: [TaskChainID: TaskChain]
    ) {
        for (id, current) in currentChains where chains[id] == nil {
            var hidden = current
            hidden.state = .abandoned
            chains[id] = hidden
        }
    }

    func retainCurrentOnlyDefinitions(
        from currentDefinitions: [TaskDefinitionID: TaskDefinition],
        targetChainIDs: Set<TaskChainID>,
        targetCurrentDefinitions: [TaskChainID: TaskDefinition],
        now: Date
    ) {
        for (id, current) in currentDefinitions where definitions[id] == nil {
            var hidden = current
            if targetChainIDs.contains(current.chainID), let restored = targetCurrentDefinitions[current.chainID] {
                hidden.supersededAt = now
                hidden.supersededByDefinitionID = restored.id
            }
            definitions[id] = hidden
        }
    }

    func retainCurrentOnlyTraces(
        from currentTraces: [DayTraceID: DayTrace],
        ids currentOnlyTraceIDs: Set<DayTraceID>,
        now: Date
    ) {
        for id in currentOnlyTraceIDs {
            guard var hidden = currentTraces[id] else { continue }
            hidden.status = .cancelledDraft
            hidden.pinOrder = nil
            hidden.completedAt = nil
            hidden.settledAt = now
            hidden.draftCancellationID = hidden.draftCancellationID ?? UUID()
            hidden.draftCancelledOn = hidden.date
            traces[id] = hidden
        }
    }

    func retainCurrentOnlySubtasks(
        from currentSubtasks: [SubtaskID: Subtask],
        currentOnlyTraceIDs: Set<DayTraceID>,
        now: Date
    ) {
        for (id, current) in currentSubtasks where subtasks[id] == nil {
            var hidden = current
            if currentOnlyTraceIDs.contains(current.traceID) == false {
                hidden.status = .cancelledDraft
                hidden.completedAt = nil
                hidden.settledAt = now
                hidden.draftCancellationID =
                    hidden.draftCancellationID ?? UUID()
            }
            subtasks[id] = hidden
        }
    }

    func snapshotUndoCurrentDefinitionsByChain() -> [TaskChainID: TaskDefinition] {
        definitions.values.reduce(
            into: [TaskChainID: TaskDefinition]()
        ) { result, definition in
            guard definition.supersededAt == nil else { return }
            if let existing = result[definition.chainID], existing.sequence >= definition.sequence {
                return
            }
            result[definition.chainID] = definition
        }
    }

    func restoreSnapshotUndoChains(
        from current: SnapshotUndoCurrentFacts,
        now: Date
    ) throws {
        for (id, storedChain) in chains {
            var candidate = storedChain
            guard let currentChain = current.chains[id] else {
                candidate.updatedAt = max(candidate.updatedAt, now)
                chains[id] = candidate
                continue
            }
            // Task-note versions are append-only sync facts. A snapshot undo
            // restores other chain fields while carrying the current note log
            // forward instead of rolling it back.
            candidate.noteEntries = currentChain.noteEntries
            candidate.updatedAt = if candidate.state == currentChain.state {
                currentChain.updatedAt
            } else {
                try forwardMutationDate(
                    after: currentChain.updatedAt,
                    now: now
                )
            }
            chains[id] = candidate
        }
    }

    func restoreSnapshotUndoDefinitions(
        from current: SnapshotUndoCurrentFacts,
        now: Date
    ) throws {
        for (id, storedDefinition) in definitions {
            var candidate = storedDefinition
            guard let currentDefinition = current.definitions[id] else {
                try candidate.markContentModified(at: now)
                definitions[id] = candidate
                continue
            }
            let restoresSupersededDefinition =
                currentDefinition.supersededAt != nil
                    && candidate.supersededAt == nil
            if restoresSupersededDefinition, let transitionWitnessID = currentDefinition.supersededByDefinitionID {
                candidate.supersededByDefinitionID = transitionWitnessID
            }
            let clock = taskDefinitionContentMatches(
                candidate,
                currentDefinition
            )
                ? currentDefinition.contentUpdatedAt
                : try forwardMutationDate(
                    after: currentDefinition.contentUpdatedAt,
                    now: now
                )
            try candidate.markContentModified(at: clock)
            definitions[id] = candidate
        }
    }

    func restoreSnapshotUndoTraces(
        from current: SnapshotUndoCurrentFacts,
        now: Date
    ) throws {
        for (id, storedTrace) in traces {
            var candidate = storedTrace
            guard let currentTrace = current.traces[id] else {
                try candidate.markContentModified(at: now)
                traces[id] = candidate
                continue
            }
            // Undo composes with later note commands by preserving their
            // current version/tombstone history.
            candidate.noteEntries = currentTrace.noteEntries
            let restoresCancelledDraft =
                currentTrace.status == .cancelledDraft
                    && candidate.status == .pending
            let restoresAbandonedBoundary =
                currentTrace.status != .abandoned
                    && candidate.status == .abandoned
            if restoresCancelledDraft, let cancellationID = currentTrace.draftCancellationID {
                candidate.draftCancellationID = cancellationID
                candidate.draftCancelledOn = nil
            }
            let clock = dayTraceContentMatches(candidate, currentTrace)
                ? currentTrace.contentUpdatedAt
                : try forwardMutationDate(
                    after: currentTrace.contentUpdatedAt,
                    now: now
                )
            try candidate.markContentModified(at: clock)
            if restoresAbandonedBoundary {
                candidate.settledAt = clock
            }
            traces[id] = candidate
        }
    }

    func restoreSnapshotUndoSubtasks(
        from current: SnapshotUndoCurrentFacts,
        now: Date
    ) throws {
        for (id, storedSubtask) in subtasks {
            var candidate = storedSubtask
            guard let currentSubtask = current.subtasks[id] else {
                let clock = try forwardMutationDate(
                    after: candidate.updatedAt,
                    now: now
                )
                try candidate.markContentModified(at: clock)
                subtasks[id] = candidate
                continue
            }
            let restoresCancelledDraft =
                currentSubtask.status == .cancelledDraft
                    && candidate.status == .pending
            if restoresCancelledDraft, let cancellationID = currentSubtask.draftCancellationID {
                candidate.draftCancellationID = cancellationID
                candidate.settledAt = nil
            }
            let clock = subtaskContentMatches(candidate, currentSubtask)
                ? currentSubtask.updatedAt
                : try forwardMutationDate(
                    after: currentSubtask.updatedAt,
                    now: now
                )
            try candidate.markContentModified(at: clock)
            subtasks[id] = candidate
        }
    }

    func forwardMutationDate(after frontier: Date, now: Date) throws -> Date {
        let frontierSeconds = frontier.timeIntervalSinceReferenceDate
        let nowSeconds = now.timeIntervalSinceReferenceDate
        guard frontierSeconds.isFinite, nowSeconds.isFinite,
              frontierSeconds.nextUp.isFinite
        else {
            throw NoonmarkError.invalidInput("undo mutation time is invalid")
        }
        return Date(
            timeIntervalSinceReferenceDate: max(frontierSeconds.nextUp, nowSeconds)
        )
    }

    func taskDefinitionContentMatches(
        _ lhs: TaskDefinition,
        _ rhs: TaskDefinition
    ) -> Bool {
        lhs.id == rhs.id
            && lhs.chainID == rhs.chainID
            && lhs.sequence == rhs.sequence
            && lhs.title == rhs.title
            && lhs.descriptionText == rhs.descriptionText
            && lhs.plannedSubtasks == rhs.plannedSubtasks
            && lhs.createdAt == rhs.createdAt
            && lhs.supersededAt == rhs.supersededAt
            && lhs.supersededByDefinitionID == rhs.supersededByDefinitionID
    }

    func subtaskContentMatches(
        _ lhs: Subtask,
        _ rhs: Subtask
    ) -> Bool {
        lhs.id == rhs.id
            && lhs.lineageID == rhs.lineageID
            && lhs.traceID == rhs.traceID
            && lhs.title == rhs.title
            && lhs.status == rhs.status
            && lhs.difficulty == rhs.difficulty
            && lhs.position == rhs.position
            && lhs.carriedFromSubtaskID == rhs.carriedFromSubtaskID
            && lhs.createdAt == rhs.createdAt
            && lhs.completedAt == rhs.completedAt
            && lhs.settledAt == rhs.settledAt
            && lhs.draftCancellationID == rhs.draftCancellationID
    }

    func dayTraceContentMatches(_ lhs: DayTrace, _ rhs: DayTrace) -> Bool {
        lhs.id == rhs.id
            && lhs.chainID == rhs.chainID
            && lhs.definitionID == rhs.definitionID
            && lhs.date == rhs.date
            && lhs.status == rhs.status
            && lhs.priority == rhs.priority
            && lhs.continuationSeq == rhs.continuationSeq
            && lhs.descriptionText == rhs.descriptionText
            && lhs.manualProgressPercent == rhs.manualProgressPercent
            && lhs.carriedFromTraceID == rhs.carriedFromTraceID
            && lhs.changedToTraceID == rhs.changedToTraceID
            && lhs.createdAt == rhs.createdAt
            && lhs.completedAt == rhs.completedAt
            && lhs.settledAt == rhs.settledAt
            && lhs.draftCancellationID == rhs.draftCancellationID
            && lhs.draftCancelledOn == rhs.draftCancelledOn
    }

    func normalizeTitle(
        _ title: String,
        allowsEmpty: Bool = false
    ) throws -> String {
        let normalized = title
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard allowsEmpty || normalized.isEmpty == false else {
            throw NoonmarkError.invalidTitle
        }
        return normalized
    }

    func normalizedOptionalText(_ text: String?) -> String? {
        let trimmed = text?.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed?.isEmpty == true ? nil : trimmed
    }

    /// Descriptions are Markdown text. Leading indentation and trailing spaces can be
    /// meaningful, and must survive character-by-character immediate persistence.
    func optionalTextPreservingWhitespace(_ text: String?) -> String? {
        guard let text,
              text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        else {
            return nil
        }
        return text
    }

    func pendingPrioritySlots(on date: LocalDate) -> [DayTrace] {
        getDayTodo(date: date).traces.filter { $0.status == .pending }
    }

    func validatePriorityTarget(
        _ targetTraceID: DayTraceID?,
        movingTrace: DayTrace
    ) throws {
        guard let targetTraceID else { return }
        let targetTrace = try trace(targetTraceID)
        guard targetTrace.date == movingTrace.date else {
            throw NoonmarkError.invalidInput(
                "day traces can only be reordered within the same date"
            )
        }
        guard targetTrace.status == .pending else {
            throw NoonmarkError.invalidTransition(
                "priority targets must be pending day traces"
            )
        }
    }

    func sorted(_ traces: [DayTrace], by sort: ViewSort) -> [DayTrace] {
        traces.sorted { lhs, rhs in
            let lhsLifecycleRank = lhs.status == .pending ? 0 : 1
            let rhsLifecycleRank = rhs.status == .pending ? 0 : 1
            if lhsLifecycleRank != rhsLifecycleRank {
                return lhsLifecycleRank < rhsLifecycleRank
            }

            let lhsIsPinned = lhs.status == .pending && lhs.pinOrder != nil
            let rhsIsPinned = rhs.status == .pending && rhs.pinOrder != nil
            if lhsIsPinned != rhsIsPinned {
                return lhsIsPinned
            }
            if let lhsPinOrder = lhs.pinOrder,
               let rhsPinOrder = rhs.pinOrder,
               lhsPinOrder != rhsPinOrder
            {
                return lhsPinOrder < rhsPinOrder
            }

            return switch sort {
            case .priority:
                tracePriorityOrder(lhs, rhs)
            case .createdAt:
                if lhs.createdAt != rhs.createdAt {
                    lhs.createdAt < rhs.createdAt
                } else {
                    lhs.id.description < rhs.id.description
                }
            case .title:
                traceTitleOrder(lhs, rhs)
            }
        }
    }

    func traceTitleOrder(_ lhs: DayTrace, _ rhs: DayTrace) -> Bool {
        let lhsTitle = definitions[lhs.definitionID]?.title ?? ""
        let rhsTitle = definitions[rhs.definitionID]?.title ?? ""
        let comparison = lhsTitle.localizedStandardCompare(rhsTitle)
        if comparison != .orderedSame {
            return comparison == .orderedAscending
        }
        return lhs.id.description < rhs.id.description
    }

    func tracePriorityOrder(_ lhs: DayTrace, _ rhs: DayTrace) -> Bool {
        if lhs.priority != rhs.priority {
            return lhs.priority < rhs.priority
        }
        if lhs.createdAt != rhs.createdAt {
            return lhs.createdAt < rhs.createdAt
        }
        return lhs.id.description < rhs.id.description
    }

    func nextPriority(on date: LocalDate) -> Int {
        (
            traces.values
                .filter { $0.date == date && $0.formsDayHistory }
                .map(\.priority)
                .max() ?? 0
        ) + 1
    }

    func ensureDay(_ date: LocalDate, now: Date) {
        if days[date] == nil {
            days[date] = Day(date: date, now: now)
        }
    }

    func ensureUnlockedDay(_ date: LocalDate) throws {
        guard days[date]?.lockedAt == nil else {
            throw NoonmarkError.immutableHistory
        }
    }

    func completionCapability(
        for trace: DayTrace,
        today: LocalDate
    ) -> DayTraceCompletionCapability {
        if days[trace.date]?.lockedAt != nil {
            return .unavailable(.lockedDay)
        }
        if trace.date < today {
            return .unavailable(.historicalDay)
        }
        if trace.date > today {
            return .unavailable(.futureDay)
        }
        switch trace.status {
        case .pending:
            return subtaskProgress(for: trace.id).canCompleteParent
                ? .available(.complete)
                : .unavailable(.openSubtasks)
        case .completed:
            return .available(.undo)
        case .unfinished, .deferred, .changed, .returnedToPool, .cancelledDraft, .abandoned:
            return .unavailable(.unsupportedStatus(trace.status))
        }
    }

    func tracePermittedForCompletionAction(
        _ requestedAction: DayTraceCompletionAction,
        traceID: DayTraceID,
        today: LocalDate
    ) throws -> DayTrace {
        let trace = try trace(traceID)
        let capability = completionCapability(for: trace, today: today)
        guard capability == .available(requestedAction) else {
            throw completionError(
                for: requestedAction,
                capability: capability
            )
        }
        return trace
    }

    func completionError(
        for requestedAction: DayTraceCompletionAction,
        capability: DayTraceCompletionCapability
    ) -> NoonmarkError {
        switch requestedAction {
        case .complete:
            switch capability {
            case .unavailable(.lockedDay), .unavailable(.historicalDay):
                return .immutableHistory
            case .unavailable(.futureDay):
                return .futurePlanCannotComplete
            case .unavailable(.openSubtasks):
                return .openSubtasksPreventCompletion
            case .available, .unavailable(.unsupportedStatus):
                return .invalidTransition("only pending traces can be completed")
            }
        case .undo:
            switch capability {
            case .unavailable(.lockedDay):
                return .immutableHistory
            case .unavailable(.historicalDay), .unavailable(.futureDay):
                return .historicalCompletionCannotBeUndone
            case .available, .unavailable(.unsupportedStatus), .unavailable(.openSubtasks):
                return .invalidTransition("only completed traces can be undone")
            }
        }
    }

    func trace(_ id: DayTraceID) throws -> DayTrace {
        guard let trace = traces[id] else {
            throw NoonmarkError.notFound("day trace")
        }
        return trace
    }

    func chain(_ id: TaskChainID) throws -> TaskChain {
        guard let chain = chains[id] else {
            throw NoonmarkError.notFound("task chain")
        }
        return chain
    }

    func definition(_ id: TaskDefinitionID) throws -> TaskDefinition {
        guard let definition = definitions[id] else {
            throw NoonmarkError.notFound("task definition")
        }
        return definition
    }

    func currentDefinition(for chainID: TaskChainID) throws -> TaskDefinition {
        let candidates = definitions.values.filter { $0.chainID == chainID && $0.supersededAt == nil }
        guard let definition = candidates.max(by: { $0.sequence < $1.sequence }) else {
            throw NoonmarkError.notFound("current task definition")
        }
        return definition
    }

    func currentDefinitionsByChain() -> [TaskChainID: TaskDefinition] {
        definitions.values.reduce(
            into: [TaskChainID: TaskDefinition]()
        ) { currentByChain, definition in
            guard definition.supersededAt == nil else {
                return
            }
            if let current = currentByChain[definition.chainID],
               current.sequence >= definition.sequence
            {
                return
            }
            currentByChain[definition.chainID] = definition
        }
    }

    func ensureActiveChain(_ id: TaskChainID) throws {
        let chain = try chain(id)
        guard chain.state == .active else {
            throw NoonmarkError.chainAbandoned
        }
    }

    func touchChain(_ id: TaskChainID, now: Date) throws {
        guard now.timeIntervalSinceReferenceDate.isFinite else {
            throw NoonmarkError.invalidInput("task chain mutation time is invalid")
        }
        if let updatedAt = chains[id]?.updatedAt {
            chains[id]?.updatedAt = max(updatedAt, now)
        }
    }

    func activeTrace(for chainID: TaskChainID) -> DayTrace? {
        traces.values
            .filter { $0.chainID == chainID && $0.status == .pending }
            .sorted { $0.createdAt > $1.createdAt }
            .first
    }

    func isNewDayTrace(_ trace: DayTrace) -> Bool {
        traces.values.allSatisfy {
            $0.chainID != trace.chainID
                || $0.id == trace.id
                || $0.formsDayHistory == false
        }
    }

    func isInTaskPool(_ chainID: TaskChainID) -> Bool {
        guard let chain = chains[chainID] else {
            return false
        }

        var traceFacts: ChainTraceFacts?
        for trace in traces.values where trace.chainID == chainID {
            if traceFacts == nil {
                traceFacts = ChainTraceFacts()
            }
            traceFacts?.ingest(trace)
        }

        return isInTaskPool(chain, traceFacts: traceFacts)
    }

    func isInTaskPool(
        _ chain: TaskChain,
        traceFacts: ChainTraceFacts?
    ) -> Bool {
        guard
            chain.state == .active,
            chain.cycleMembership == nil
        else {
            return false
        }

        guard let traceFacts else {
            return true
        }
        guard traceFacts.activeTrace == nil else {
            return false
        }
        return traceFacts.latestTrace?.status == .returnedToPool
            || traceFacts.latestTrace?.status == .cancelledDraft
    }

    func chainTraceFactsByChain() -> [TaskChainID: ChainTraceFacts] {
        traces.values.reduce(
            into: [TaskChainID: ChainTraceFacts]()
        ) { factsByChain, trace in
            factsByChain[trace.chainID, default: ChainTraceFacts()]
                .ingest(trace)
        }
    }

    struct ChainTraceFacts {
        var activeTrace: DayTrace?
        var latestTrace: DayTrace?
        var hasCompletedTrace = false
        var hasUnfinishedCandidate = false

        mutating func ingest(_ trace: DayTrace) {
            if trace.status == .pending,
               activeTrace == nil
                   || trace.createdAt
                   > (activeTrace?.createdAt ?? .distantPast)
            {
                activeTrace = trace
            }
            if latestTrace == nil
                || Self.isLater(trace, than: latestTrace)
            {
                latestTrace = trace
            }
            if trace.status == .completed {
                hasCompletedTrace = true
            }
            if trace.status == .unfinished
                || trace.status == .abandoned
            {
                hasUnfinishedCandidate = true
            }
        }

        private static func isLater(
            _ candidate: DayTrace,
            than current: DayTrace?
        ) -> Bool {
            guard let current else {
                return true
            }
            if candidate.contentUpdatedAt != current.contentUpdatedAt {
                return candidate.contentUpdatedAt > current.contentUpdatedAt
            }
            if candidate.createdAt != current.createdAt {
                return candidate.createdAt > current.createdAt
            }
            return candidate.id.description > current.id.description
        }
    }

    func reviewStats(for items: [DayTrace]) -> DailyReviewStats {
        DailyReviewStats(
            total: items.count,
            completed: items.filter { $0.status == .completed }.count,
            unfinished: items.filter { $0.status == .unfinished }.count,
            deferred: items.filter { $0.status == .deferred }.count,
            changed: items.filter { $0.status == .changed }.count,
            returnedToPool: items.filter { $0.status == .returnedToPool }.count,
            abandoned: items.filter { $0.status == .abandoned }.count
        )
    }

    func hasCompletedTrace(_ chainID: TaskChainID) -> Bool {
        traces.values.contains { $0.chainID == chainID && $0.status == .completed }
    }

    func hasSubtasks(in chainID: TaskChainID) -> Bool {
        let traceIDs = Set(
            traces.values
                .filter {
                    $0.chainID == chainID && $0.formsDayHistory
                }
                .map(\.id)
        )
        return subtasks.values.contains {
            traceIDs.contains($0.traceID) && $0.isUserPresentable
        }
    }

    func rawProgress(for trace: DayTrace, chainTraces: [DayTrace]) -> TraceProgress {
        if trace.status == .completed {
            return TraceProgress(mode: .manual, percent: 100, floorPercent: 0, completedWeight: 1, totalWeight: 1)
        }

        let traceByID = Dictionary(uniqueKeysWithValues: chainTraces.map { ($0.id, $0) })
        let lineageRecords = Dictionary(grouping: subtasks.values.compactMap { subtask -> (SubtaskLineageID, Subtask, DayTrace)? in
            guard subtask.isUserPresentable,
                  let subtaskTrace = traceByID[subtask.traceID],
                  subtaskTrace.date <= trace.date
            else {
                return nil
            }
            return (subtask.lineageID, subtask, subtaskTrace)
        }) { $0.0 }

        if lineageRecords.isEmpty == false {
            var totalWeight = 0
            var completedWeight = 0

            for (_, records) in lineageRecords {
                let sortedRecords = records.sorted {
                    if $0.2.date != $1.2.date {
                        return $0.2.date < $1.2.date
                    }
                    return $0.1.createdAt < $1.1.createdAt
                }
                let weight = sortedRecords.map { $0.1.difficulty.rawValue }.max() ?? SubtaskDifficulty.simple.rawValue
                totalWeight += weight

                if sortedRecords.contains(where: { $0.1.status == .completed && $0.2.date <= trace.date }) {
                    completedWeight += weight
                }
            }

            let percent = totalWeight == 0 ? 0 : Int((Double(completedWeight) / Double(totalWeight) * 100).rounded())
            return TraceProgress(
                mode: .weightedSubtasks,
                percent: percent,
                floorPercent: 0,
                completedWeight: completedWeight,
                totalWeight: totalWeight
            )
        }

        return TraceProgress(
            mode: .manual,
            percent: trace.manualProgressPercent ?? 0,
            floorPercent: 0,
            completedWeight: trace.manualProgressPercent ?? 0,
            totalWeight: 100
        )
    }

    func progressFloor(for trace: DayTrace) -> Int {
        let chainTraces = traces.values
            .filter {
                $0.chainID == trace.chainID && $0.formsDayHistory
            }
            .sorted(by: traceChronology)
        return progressFloor(for: trace, chainTraces: chainTraces)
    }

    func progressFloor(for trace: DayTrace, chainTraces: [DayTrace]) -> Int {
        chainTraces
            .filter { $0.date < trace.date || ($0.date == trace.date && $0.continuationSeq < trace.continuationSeq) }
            .map { rawProgress(for: $0, chainTraces: chainTraces).percent }
            .max() ?? 0
    }

    func completedTrajectory(
        for completedTrace: DayTrace,
        chainTraces: [DayTrace],
        presentableSubtasksByTraceID: [DayTraceID: [Subtask]]
    ) -> CompletedTaskTrajectory {
        CompletedTaskTrajectory(
            startDate: chainTraces.first?.date ?? completedTrace.date,
            continuedDates: uniqueDates(
                chainTraces
                    .filter { carryoverKind(for: $0.id) == .continuation }
                    .map(\.date)
            ),
            completedDate: completedTrace.date,
            traces: chainTraces,
            subtaskTrajectories: completedSubtaskTrajectories(
                for: chainTraces,
                presentableSubtasksByTraceID: presentableSubtasksByTraceID
            )
        )
    }

    func completedSubtaskTrajectories(
        for chainTraces: [DayTrace],
        presentableSubtasksByTraceID: [DayTraceID: [Subtask]]
    ) -> [SubtaskTrajectory] {
        let traceByID = Dictionary(uniqueKeysWithValues: chainTraces.map { ($0.id, $0) })
        let groupedSubtasks = Dictionary(
            grouping: chainTraces.flatMap {
                presentableSubtasksByTraceID[$0.id] ?? []
            }
        ) {
            $0.lineageID
        }

        return groupedSubtasks.map { lineageID, items in
            let records = items
                .compactMap { subtask -> SubtaskTrajectoryRecord? in
                    guard let trace = traceByID[subtask.traceID] else { return nil }
                    return SubtaskTrajectoryRecord(subtask: subtask, date: trace.date)
                }
                .sorted(by: subtaskRecordChronology)

            return SubtaskTrajectory(
                lineageID: lineageID,
                title: records.first?.subtask.title ?? "",
                startDate: records.first?.date ?? LocalDate("0001-01-01"),
                continuedDates: uniqueDates(
                    records
                        .filter {
                            carryoverKind(for: $0.subtask.traceID)
                                == .continuation
                                && $0.subtask.carriedFromSubtaskID != nil
                        }
                        .map(\.date)
                ),
                completedDate: records.first(where: { $0.subtask.status == .completed })?.date,
                records: records
            )
        }
        .sorted {
            if $0.startDate != $1.startDate {
                return $0.startDate < $1.startDate
            }
            return $0.title.localizedStandardCompare($1.title) == .orderedAscending
        }
    }

    func traceChronology(_ lhs: DayTrace, _ rhs: DayTrace) -> Bool {
        if lhs.date != rhs.date {
            return lhs.date < rhs.date
        }
        if lhs.continuationSeq != rhs.continuationSeq {
            return lhs.continuationSeq < rhs.continuationSeq
        }
        if lhs.priority != rhs.priority {
            return lhs.priority < rhs.priority
        }
        if lhs.createdAt != rhs.createdAt {
            return lhs.createdAt < rhs.createdAt
        }
        return lhs.id.description < rhs.id.description
    }

    func subtaskRecordChronology(_ lhs: SubtaskTrajectoryRecord, _ rhs: SubtaskTrajectoryRecord) -> Bool {
        if lhs.date != rhs.date {
            return lhs.date < rhs.date
        }
        if lhs.subtask.position != rhs.subtask.position {
            return lhs.subtask.position < rhs.subtask.position
        }
        return lhs.subtask.createdAt < rhs.subtask.createdAt
    }

    func uniqueDates(_ dates: [LocalDate]) -> [LocalDate] {
        var seen = Set<LocalDate>()
        var result: [LocalDate] = []

        for date in dates where seen.insert(date).inserted {
            result.append(date)
        }

        return result
    }

    func copyOpenSubtasks(
        from sourceTraceID: DayTraceID,
        to targetTraceID: DayTraceID,
        kind: TraceCarryoverKind,
        now: Date
    ) throws {
        let openSubtasks = subtasks.values
            .filter { $0.traceID == sourceTraceID && ($0.status == .pending || $0.status == .unfinished) }
            .sorted { $0.position < $1.position }

        for (index, oldSubtask) in openSubtasks.enumerated() {
            if kind == .deferral {
                var deferredSubtask = oldSubtask
                try deferredSubtask.markContentModified(at: now)
                deferredSubtask.status = .deferred
                deferredSubtask.settledAt = now
                subtasks[deferredSubtask.id] = deferredSubtask
            }

            let newSubtask = Subtask(
                lineageID: oldSubtask.lineageID,
                traceID: targetTraceID,
                title: oldSubtask.title,
                difficulty: oldSubtask.difficulty,
                position: index + 1,
                carriedFromSubtaskID: oldSubtask.id,
                now: now
            )
            subtasks[newSubtask.id] = newSubtask
        }
    }

    func copyOpenSubtasksToChangedTask(
        from sourceTraceID: DayTraceID,
        to targetTraceID: DayTraceID,
        now: Date
    ) {
        let openSubtasks = subtasks.values
            .filter {
                $0.traceID == sourceTraceID
                    && ($0.status == .pending || $0.status == .unfinished)
            }
            .sorted { $0.position < $1.position }

        for (index, oldSubtask) in openSubtasks.enumerated() {
            let copiedSubtask = Subtask(
                traceID: targetTraceID,
                title: oldSubtask.title,
                difficulty: oldSubtask.difficulty,
                position: index + 1,
                now: now
            )
            subtasks[copiedSubtask.id] = copiedSubtask
        }
    }

    func materializePoolDraft(from trace: DayTrace, now: Date) throws {
        var chain = try chain(trace.chainID)
        var currentDefinition = try currentDefinition(for: trace.chainID)
        let nextSequence = (definitions.values
            .filter { $0.chainID == trace.chainID }
            .map(\.sequence)
            .max() ?? currentDefinition.sequence) + 1
        let plannedSubtasks = subtasks.values
            .filter {
                $0.traceID == trace.id
                    && ($0.status == .pending || $0.status == .unfinished)
            }
            .sorted { $0.position < $1.position }
            .enumerated()
            .map { index, subtask in
                PlannedSubtask(
                    lineageID: subtask.lineageID,
                    title: subtask.title,
                    difficulty: subtask.difficulty,
                    position: index + 1,
                    now: now
                )
            }
        let returnedDefinition = TaskDefinition(
            chainID: trace.chainID,
            sequence: nextSequence,
            title: currentDefinition.title,
            descriptionText: trace.descriptionText,
            plannedSubtasks: plannedSubtasks,
            now: now
        )

        try currentDefinition.markContentModified(at: now)
        currentDefinition.supersededAt = now
        currentDefinition.supersededByDefinitionID = returnedDefinition.id
        try chain.markContentModified(at: now)
        chain.noteEntries = trace.noteEntries

        definitions[currentDefinition.id] = currentDefinition
        definitions[returnedDefinition.id] = returnedDefinition
        chains[chain.id] = chain
    }

    func prepareSubtasksForHiddenTrace(
        traceID: DayTraceID,
        now: Date
    ) throws {
        for subtaskValue in subtasks.values where subtaskValue.traceID == traceID {
            guard subtaskValue.status == .cancelledDraft else { continue }
            var subtask = subtaskValue
            try subtask.markContentModified(at: now)
            subtask.status = .pending
            subtask.completedAt = nil
            subtask.settledAt = nil
            subtask.draftCancellationID = nil
            subtasks[subtask.id] = subtask
        }
    }

    func copyPlannedSubtasks(from definition: TaskDefinition, to traceID: DayTraceID, now: Date) {
        for (index, plannedSubtask) in definition.plannedSubtasks.sorted(by: { $0.position < $1.position }).enumerated() {
            let subtask = Subtask(
                lineageID: plannedSubtask.lineageID,
                traceID: traceID,
                title: plannedSubtask.title,
                difficulty: plannedSubtask.difficulty,
                position: index + 1,
                now: now
            )
            subtasks[subtask.id] = subtask
        }
    }

    func settleOpenSubtasks(on traceID: DayTraceID, now: Date) throws {
        for subtask in subtasks.values where subtask.traceID == traceID && subtask.status == .pending {
            var settled = subtask
            try settled.markContentModified(at: now)
            settled.status = .unfinished
            settled.settledAt = now
            subtasks[settled.id] = settled
        }
    }
}
