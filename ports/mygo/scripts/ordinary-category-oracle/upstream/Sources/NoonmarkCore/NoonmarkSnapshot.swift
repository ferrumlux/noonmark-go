import Foundation

public struct NoonmarkSnapshot: Codable, Equatable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case days
        case taskCycleSeries
        case chains
        case definitions
        case traces
        case subtasks
        case preferences
        case classifications
        case ideas
    }

    public var days: [Day]
    public var taskCycleSeries: [TaskCycleSeries]
    public var chains: [TaskChain]
    public var definitions: [TaskDefinition]
    public var traces: [DayTrace]
    public var subtasks: [Subtask]
    public var preferences: AppPreferences
    public var classifications: TaskClassificationState
    public var ideas: [IdeaEntry]

    public init(
        days: [Day],
        taskCycleSeries: [TaskCycleSeries] = [],
        chains: [TaskChain],
        definitions: [TaskDefinition],
        traces: [DayTrace],
        subtasks: [Subtask],
        preferences: AppPreferences,
        classifications: TaskClassificationState,
        ideas: [IdeaEntry] = []
    ) {
        self.days = days
        self.taskCycleSeries = taskCycleSeries
        self.chains = chains
        self.definitions = definitions
        self.traces = traces
        self.subtasks = subtasks
        self.preferences = preferences
        self.classifications = classifications
        self.ideas = ideas
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        days = try container.decode([Day].self, forKey: .days)
        taskCycleSeries = try container.decode(
            [TaskCycleSeries].self,
            forKey: .taskCycleSeries
        )
        chains = try container.decode([TaskChain].self, forKey: .chains)
        definitions = try container.decode([TaskDefinition].self, forKey: .definitions)
        traces = try container.decode([DayTrace].self, forKey: .traces)
        subtasks = try container.decode([Subtask].self, forKey: .subtasks)
        preferences = try container.decode(AppPreferences.self, forKey: .preferences)
        classifications = try container.decode(
            TaskClassificationState.self,
            forKey: .classifications
        )
        ideas = try container.decode([IdeaEntry].self, forKey: .ideas)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(days, forKey: .days)
        try container.encode(taskCycleSeries, forKey: .taskCycleSeries)
        try container.encode(chains, forKey: .chains)
        try container.encode(definitions, forKey: .definitions)
        try container.encode(traces, forKey: .traces)
        try container.encode(subtasks, forKey: .subtasks)
        try container.encode(preferences, forKey: .preferences)
        try container.encode(classifications, forKey: .classifications)
        try container.encode(ideas, forKey: .ideas)
    }
}

public extension NoonmarkSnapshot {
    func validateIntegrity() throws {
        try classifications.validateIntegrity()
        guard preferences.themeLanguageUpdatedAt
            .timeIntervalSinceReferenceDate.isFinite,
              preferences.themeLanguageWriterID.trimmingCharacters(
                  in: .whitespacesAndNewlines
              ).isEmpty == false
        else {
            throw NoonmarkError.invalidInput(
                "app preferences contain an invalid theme and language version"
            )
        }

        let dayDates = Set(days.map(\.date))
        let taskCycleSeriesIDs = Set(taskCycleSeries.map(\.id))
        let chainIDs = Set(chains.map(\.id))
        let traceIDs = Set(traces.map(\.id))
        try validateUniqueIdentities()
        let chainsByID = Dictionary(
            uniqueKeysWithValues: chains.map { ($0.id, $0) }
        )
        for day in days {
            try validateDayTodoClock(day)
        }
        for series in taskCycleSeries {
            try series.validateIntegrity()
            let classificationCategoryIDs = Set(
                series.classificationRevisions.compactMap(\.categoryID)
            )
            let classificationLabelIDs = Set(
                series.classificationRevisions.flatMap(\.labelIDs)
            )
            guard classificationCategoryIDs.allSatisfy({
                classifications.categories[$0] != nil
            }),
                classificationLabelIDs.allSatisfy({
                    classifications.labels[$0] != nil
                })
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle template references missing classification"
                )
            }
        }
        for chain in chains {
            guard chain.createdAt.timeIntervalSinceReferenceDate.isFinite,
                  chain.updatedAt.timeIntervalSinceReferenceDate.isFinite,
                  chain.updatedAt >= chain.createdAt
            else {
                throw NoonmarkError.invalidInput(
                    "task chain contains an invalid content clock"
                )
            }
            try validateTaskNoteEntries(chain.noteEntries, owner: "task chain")
        }
        try validateTaskCycleMemberships(
            seriesIDs: taskCycleSeriesIDs
        )
        for definition in definitions {
            try validateContentClock(
                createdAt: definition.createdAt,
                contentUpdatedAt: definition.contentUpdatedAt,
                terminalDates: [definition.supersededAt],
                owner: "task definition"
            )
            try validatePlannedSubtaskFacts(
                in: definition,
                chainCreatedAt: chainsByID[definition.chainID]?.createdAt
            )
        }
        for trace in traces {
            try validateContentClock(
                createdAt: trace.createdAt,
                contentUpdatedAt: trace.contentUpdatedAt,
                terminalDates: [trace.completedAt, trace.settledAt],
                owner: "day trace"
            )
            if TrajectoryTopologyValidator.firstSelfContainedIssue(
                in: trace
            ) != nil {
                throw NoonmarkError.invalidInput(
                    "day trace contains invalid status or progress facts"
                )
            }
            try validateTaskNoteEntries(trace.noteEntries, owner: "day trace")
        }
        for subtask in subtasks {
            try subtask.validateIntegrity()
        }
        try validateIdeaEntries()
        let definitionsByID = Dictionary(
            uniqueKeysWithValues: definitions.map { ($0.id, $0) }
        )
        let tracesByID = Dictionary(
            uniqueKeysWithValues: traces.map { ($0.id, $0) }
        )
        try validateParentReferences(
            dayDates: dayDates,
            chainIDs: chainIDs,
            definitionsByID: definitionsByID,
            tracesByID: tracesByID
        )
        try validateDefinitionTopology(
            chainIDs: chainIDs,
            definitionsByID: definitionsByID
        )
        try validateTrajectoryTopology()
        try validateCancelledDraftFacts()
        try validateClassificationReferences(
            chainIDs: chainIDs,
            traceIDs: traceIDs
        )
        try validateCurrentAndHistoricalClassificationSources(chainIDs: chainIDs)
    }

    private func validateUniqueIdentities() throws {
        let dayDates = Set(days.map(\.date))
        let dayIDs = Set(days.map(\.id))
        let taskCycleSeriesIDs = Set(taskCycleSeries.map(\.id))
        let chainIDs = Set(chains.map(\.id))
        let definitionIDs = Set(definitions.map(\.id))
        let traceIDs = Set(traces.map(\.id))
        let subtaskIDs = Set(subtasks.map(\.id))
        guard dayDates.count == days.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate Day Todo dates"
            )
        }
        guard dayIDs.count == days.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate Day Todo identities"
            )
        }
        guard taskCycleSeriesIDs.count == taskCycleSeries.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate task cycle series identities"
            )
        }
        guard chainIDs.count == chains.count else {
            throw NoonmarkError.invalidInput("snapshot contains duplicate task chain identities")
        }
        guard definitionIDs.count == definitions.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate task definition identities"
            )
        }
        guard traceIDs.count == traces.count else {
            throw NoonmarkError.invalidInput("snapshot contains duplicate trace identities")
        }
        guard subtaskIDs.count == subtasks.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate subtask identities"
            )
        }
        guard Set(ideas.map(\.id)).count == ideas.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate idea identities"
            )
        }
    }

    private func validateIdeaEntries() throws {
        switch IdeaEntryValidator.firstIssue(in: ideas) {
        case .none:
            break
        case .duplicateIdentity:
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate idea identities"
            )
        case .invalidTimestamps:
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid idea timestamps"
            )
        case .invalidTombstone:
            throw NoonmarkError.invalidInput(
                "snapshot contains an invalid idea tombstone"
            )
        case .invalidActiveBody:
            throw NoonmarkError.invalidInput(
                "snapshot contains an invalid active idea"
            )
        case .duplicateLabels:
            throw NoonmarkError.invalidInput(
                "idea contains duplicate labels"
            )
        case .invalidPin:
            throw NoonmarkError.invalidInput(
                "snapshot contains an invalid idea pin"
            )
        }
        let activeIdeas = ideas.filter { $0.isDeleted == false }
        guard activeIdeas.allSatisfy({ idea in
            (idea.categoryID.map { classifications.categories[$0] != nil }
                ?? true)
                && idea.labelIDs.allSatisfy {
                    classifications.labels[$0] != nil
                }
        }) else {
            throw NoonmarkError.invalidInput(
                "idea references missing classification"
            )
        }
    }

    private func validateTaskCycleMemberships(
        seriesIDs: Set<TaskCycleSeriesID>
    ) throws {
        let memberships = chains.compactMap(\.cycleMembership)
        let chainIDsWithTrace = Set(traces.map(\.chainID))
        let occurrenceKeys = memberships.map {
            "\($0.seriesID.description):\($0.occurrenceDate.description)"
        }
        guard Set(occurrenceKeys).count == occurrenceKeys.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate task cycle occurrences"
            )
        }

        for chain in chains {
            guard let membership = chain.cycleMembership else { continue }
            guard seriesIDs.contains(membership.seriesID) else {
                throw NoonmarkError.invalidInput(
                    "task cycle membership references a missing series"
                )
            }
            guard chainIDsWithTrace.contains(chain.id)
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle membership contains invalid schedule facts"
                )
            }
        }

        let membershipsBySeries = Dictionary(
            grouping: memberships,
            by: \.seriesID
        )
        for series in taskCycleSeries {
            let seriesMemberships = membershipsBySeries[series.id] ?? []
            let expectedDates = try series.everPlannedDates()
            guard Set(seriesMemberships.map(\.occurrenceDate))
                == expectedDates
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle series is missing a scheduled occurrence"
                )
            }
        }
    }

    private func validatePlannedSubtaskFacts(
        in definition: TaskDefinition,
        chainCreatedAt: Date?
    ) throws {
        guard let issue = TaskDefinitionValidator.firstSelfContainedIssue(
            in: definition,
            chainCreatedAt: chainCreatedAt
        ) else {
            return
        }
        switch issue {
        case .invalidPlannedSubtasks:
            throw NoonmarkError.invalidInput(
                "task definition contains invalid planned subtask facts"
            )
        case .invalidContentClock:
            throw NoonmarkError.invalidInput(
                "task definition contains an invalid content clock"
            )
        case .invalidTitle:
            throw NoonmarkError.invalidInput(
                "task definition contains an invalid title"
            )
        case .invalidSequence, .incompleteSupersession, .selfSuccessor,
             .missingDefinitionForChain, .duplicateSequence,
             .invalidCurrentCount, .missingSuccessor,
             .crossChainSuccessor, .successorCycle:
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid task definition topology"
            )
        }
    }

    private func validateParentReferences(
        dayDates: Set<LocalDate>,
        chainIDs: Set<TaskChainID>,
        definitionsByID: [TaskDefinitionID: TaskDefinition],
        tracesByID: [DayTraceID: DayTrace]
    ) throws {
        guard definitions.allSatisfy({
            chainIDs.contains($0.chainID)
                && ($0.supersededByDefinitionID.map {
                    definitionsByID[$0] != nil
                } ?? true)
        }), traces.allSatisfy({ trace in
            guard chainIDs.contains(trace.chainID),
                  dayDates.contains(trace.date),
                  let definition = definitionsByID[trace.definitionID]
            else {
                return false
            }
            return definition.chainID == trace.chainID
        }), subtasks.allSatisfy({
            tracesByID[$0.traceID] != nil
        }) else {
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid parent references"
            )
        }
    }

    private func validateTrajectoryTopology() throws {
        guard let issue = TrajectoryTopologyValidator.topologyIssues(
            traces: traces,
            subtasks: subtasks
        ).first else {
            return
        }
        switch issue {
        case .duplicateSubtaskPosition, .noncontiguousSubtaskPosition,
             .duplicateSubtaskLineage,
             .missingSubtaskParent, .missingSubtaskContinuationSource,
             .missingSubtaskContinuationTarget,
             .reusedSubtaskContinuationSource,
             .invalidSubtaskContinuation, .subtaskContinuationCycle:
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid subtask topology"
            )
        case .invalidTraceClock, .invalidTraceStatusFacts,
             .invalidTraceProgress, .invalidTraceSequence,
             .duplicateActiveTrace, .duplicateVisiblePriority,
             .missingTraceContinuationSource,
             .missingTraceContinuationTarget,
             .reusedTraceContinuationSource,
             .invalidTraceContinuation, .missingChangedTraceTarget,
             .reusedChangedTraceTarget, .invalidChangedTraceLink,
             .traceCausalCycle, .reusedDraftCancellationIdentity:
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid day trace topology"
            )
        }
    }

    private func validateDefinitionTopology(
        chainIDs: Set<TaskChainID>,
        definitionsByID: [TaskDefinitionID: TaskDefinition]
    ) throws {
        let issues = TaskDefinitionValidator.topologyIssues(
            definitions: Array(definitionsByID.values),
            chainIDs: chainIDs
        )
        guard issues.isEmpty else {
            throw NoonmarkError.invalidInput(
                "snapshot contains invalid task definition topology"
            )
        }
    }

    private func validateCancelledDraftFacts() throws {
        let cancelledDraftTraceIDs = Set(
            traces
                .filter { $0.status == .cancelledDraft }
                .map(\.id)
        )
        let draftCancellationIDs = traces.compactMap(\.draftCancellationID)
        guard Set(draftCancellationIDs).count == draftCancellationIDs.count else {
            throw NoonmarkError.invalidInput(
                "snapshot contains duplicate trace draft cancellation identities"
            )
        }
        guard traces.allSatisfy({ trace in
            if trace.status == .cancelledDraft {
                guard trace.draftCancellationID != nil,
                      let draftCancelledOn = trace.draftCancelledOn
                else {
                    return false
                }
                let isSnapshotUndoCancellation =
                    trace.date == draftCancelledOn
                let isFutureDraftCancellation =
                    trace.date > draftCancelledOn
                return (isSnapshotUndoCancellation
                    || isFutureDraftCancellation)
                    && trace.completedAt == nil
                    && trace.settledAt != nil
                    && trace.changedToTraceID == nil
            }
            return trace.draftCancelledOn == nil
        }) else {
            throw NoonmarkError.invalidInput(
                "cancelled trace draft contains invalid terminal facts"
            )
        }
        let cancelledDraftSubtasks = subtasks.filter {
            cancelledDraftTraceIDs.contains($0.traceID)
        }
        let cancelledDraftSubtaskIDs = Set(cancelledDraftSubtasks.map(\.id))
        guard cancelledDraftSubtasks.allSatisfy({
            $0.status == .pending
                && $0.completedAt == nil
                && $0.settledAt == nil
        }), subtasks.allSatisfy({
            $0.carriedFromSubtaskID.map {
                cancelledDraftSubtaskIDs.contains($0) == false
            } ?? true
        }), traces.allSatisfy({
            $0.carriedFromTraceID.map {
                cancelledDraftTraceIDs.contains($0) == false
            } ?? true
        }), traces.allSatisfy({
            $0.changedToTraceID.map {
                cancelledDraftTraceIDs.contains($0) == false
            } ?? true
        }) else {
            throw NoonmarkError.invalidInput(
                "cancelled trace draft contains referenced history facts"
            )
        }
        let cancelledSubtasks = subtasks.filter {
            $0.status == .cancelledDraft
        }
        let cancelledSubtaskIDs = Set(cancelledSubtasks.map(\.id))
        let subtaskCancellationIDs = subtasks.compactMap(
            \.draftCancellationID
        )
        guard Set(subtaskCancellationIDs).count
                == subtaskCancellationIDs.count,
              subtasks.allSatisfy({
                  $0.carriedFromSubtaskID.map {
                      cancelledSubtaskIDs.contains($0) == false
                  } ?? true
              })
        else {
            throw NoonmarkError.invalidInput(
                "cancelled subtask draft contains invalid cancellation facts"
            )
        }
        guard classifications.snapshotsByTraceID.keys.allSatisfy({
            cancelledDraftTraceIDs.contains($0) == false
        }), classifications.snapshotEventsByTraceID.keys.allSatisfy({
            cancelledDraftTraceIDs.contains($0) == false
        }) else {
            throw NoonmarkError.invalidInput(
                "cancelled trace draft cannot contain historical classification facts"
            )
        }
    }

    private func validateTaskNoteEntries(
        _ entries: [TaskNoteEntry],
        owner: String
    ) throws {
        switch TaskNoteEntryValidator.firstIssue(in: entries) {
        case .none:
            return
        case .duplicateIdentity:
            throw NoonmarkError.invalidInput("\(owner) contains duplicate task note identities")
        case .invalidTimestamps:
            throw NoonmarkError.invalidInput("\(owner) contains invalid task note timestamps")
        case .invalidTombstone:
            throw NoonmarkError.invalidInput("\(owner) contains an invalid task note tombstone")
        case .invalidActiveBody:
            throw NoonmarkError.invalidInput("\(owner) contains an invalid active task note")
        }
    }

    private func validateDayTodoClock(_ day: Day) throws {
        guard TaskNoteEntry.isValidMutationTime(
            day.updatedAt,
            notBefore: day.createdAt
        ) else {
            throw NoonmarkError.invalidInput(
                "Day Todo contains an invalid content clock"
            )
        }
        if let lockedAt = day.lockedAt {
            guard TaskNoteEntry.isValidMutationTime(
                lockedAt,
                notBefore: day.createdAt
            ), day.updatedAt >= lockedAt
            else {
                throw NoonmarkError.invalidInput(
                    "Day Todo contains an invalid content clock"
                )
            }
        }
    }

    private func validateContentClock(
        createdAt: Date,
        contentUpdatedAt: Date,
        terminalDates: [Date?],
        owner: String
    ) throws {
        guard TaskNoteEntry.isValidMutationTime(
            contentUpdatedAt,
            notBefore: createdAt
        ) else {
            throw NoonmarkError.invalidInput("\(owner) contains an invalid content clock")
        }
        for terminalDate in terminalDates.compactMap({ $0 }) {
            guard TaskNoteEntry.isValidMutationTime(
                terminalDate,
                notBefore: createdAt
            ), contentUpdatedAt >= terminalDate
            else {
                throw NoonmarkError.invalidInput(
                    "\(owner) content clock does not cover its terminal state"
                )
            }
        }
    }

    private func validateClassificationReferences(
        chainIDs: Set<TaskChainID>,
        traceIDs: Set<DayTraceID>
    ) throws {
        guard classifications.currentByChainID.keys.allSatisfy(chainIDs.contains) else {
            throw NoonmarkError.invalidInput("classification references a missing task chain")
        }
        guard classifications.relationHistory.allSatisfy({
            chainIDs.contains($0.chainID)
        }) else {
            throw NoonmarkError.invalidInput("classification history references a missing task chain")
        }
        guard classifications.snapshotsByTraceID.keys.allSatisfy(traceIDs.contains),
              classifications.snapshotEventsByTraceID.keys.allSatisfy(traceIDs.contains)
        else {
            throw NoonmarkError.invalidInput("classification history references a missing trace")
        }

        let historicalEventIDs = Set(
            classifications.snapshotEventsByTraceID.values
                .flatMap { $0 }
                .map(\.id)
        )
        for record in classifications.changeRecords {
            try validateClassificationSource(record.source, chainIDs: chainIDs)
            for change in record.changes {
                try validateClassificationChange(
                    change,
                    chainIDs: chainIDs,
                    traceIDs: traceIDs,
                    historicalEventIDs: historicalEventIDs
                )
            }
        }
    }

    private func validateCurrentAndHistoricalClassificationSources(
        chainIDs: Set<TaskChainID>
    ) throws {
        for current in classifications.currentByChainID.values {
            if let category = current.category {
                try validateClassificationSource(category.source, chainIDs: chainIDs)
            }
            for label in current.labels {
                try validateClassificationSource(label.source, chainIDs: chainIDs)
            }
        }
        for history in classifications.relationHistory {
            try validateClassificationSource(history.originSource, chainIDs: chainIDs)
            try validateClassificationSource(history.removedBySource, chainIDs: chainIDs)
        }
    }

    private func validateClassificationSource(
        _ source: ClassificationSource,
        chainIDs: Set<TaskChainID>
    ) throws {
        switch source {
        case let .automaticAI(_, generation):
            guard generation > 0 else {
                throw NoonmarkError.invalidInput(
                    "automatic classification source generation must be positive"
                )
            }
        case let .inherited(fromChainID):
            guard chainIDs.contains(fromChainID) else {
                throw NoonmarkError.invalidInput(
                    "classification source references a missing task chain"
                )
            }
        case .userDirect, .zhulongSuggestion, .deterministicDomainAction:
            break
        }
    }

    private func validateClassificationChange(
        _ change: ClassificationPlanChange,
        chainIDs: Set<TaskChainID>,
        traceIDs: Set<DayTraceID>,
        historicalEventIDs: Set<UUID>
    ) throws {
        switch change {
        case let .setCurrent(chainID, _, _):
            guard chainIDs.contains(chainID) else {
                throw NoonmarkError.invalidInput("classification audit references a missing task chain")
            }
        case let .merge(_, _, _, _, _, _, impact):
            guard impact.currentChainIDs.allSatisfy(chainIDs.contains),
                  impact.historicalTraceIDs.allSatisfy(traceIDs.contains),
                  impact.historicalEventIDs.allSatisfy(historicalEventIDs.contains)
            else {
                throw NoonmarkError.invalidInput("classification merge impact contains a missing reference")
            }
        case .create, .hardDelete, .rename, .lifecycle, .categoryPresentationApproval:
            break
        }
    }
}

public extension Subtask {
    func validateIntegrity() throws {
        let terminalDates = [completedAt, settledAt].compactMap { $0 }
        let clockIsValid = TaskNoteEntry.isValidMutationTime(
            updatedAt,
            notBefore: createdAt
        ) && terminalDates.allSatisfy {
            TaskNoteEntry.isValidMutationTime($0, notBefore: createdAt)
                && $0 <= updatedAt
        }
        let statusFactsAreValid = switch status {
        case .pending:
            completedAt == nil && settledAt == nil
        case .completed:
            completedAt != nil && settledAt == nil
        case .unfinished, .deferred, .abandoned:
            completedAt == nil && settledAt != nil
        case .cancelledDraft:
            completedAt == nil
                && settledAt != nil
                && draftCancellationID != nil
        }
        guard clockIsValid,
              statusFactsAreValid,
              title.trimmingCharacters(in: .whitespacesAndNewlines) == title,
              title.isEmpty == false,
              position > 0
        else {
            throw NoonmarkError.invalidInput(
                "subtask contains invalid status or clock facts"
            )
        }
    }
}
