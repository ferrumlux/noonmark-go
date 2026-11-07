import Foundation

public enum TaskCycleWeekday: Int, Codable, CaseIterable, Equatable, Hashable, Sendable {
    case sunday = 1
    case monday
    case tuesday
    case wednesday
    case thursday
    case friday
    case saturday
}

public enum TaskCycleSchedule: Codable, Equatable, Hashable, Sendable {
    case everyDays(Int)
    case selectedWeekdays(
        weekdays: Set<TaskCycleWeekday>,
        everyWeeks: Int
    )

    public static let daily = TaskCycleSchedule.everyDays(1)
    public static let weekdays = TaskCycleSchedule.selectedWeekdays(
        weekdays: [.monday, .tuesday, .wednesday, .thursday, .friday],
        everyWeeks: 1
    )

    public func includes(
        _ date: LocalDate,
        anchoredAt anchorDate: LocalDate
    ) -> Bool {
        guard let dayOffset = TaskCycleCivilCalendar.dayDistance(
            from: anchorDate,
            to: date
        ), dayOffset >= 0 else {
            return false
        }
        switch self {
        case let .everyDays(interval):
            return interval > 0 && dayOffset.isMultiple(of: interval)
        case let .selectedWeekdays(weekdays, everyWeeks):
            guard everyWeeks > 0,
                  weekdays.isEmpty == false,
                  (dayOffset / 7).isMultiple(of: everyWeeks),
                  let rawWeekday = TaskCycleCivilCalendar.weekday(for: date),
                  let weekday = TaskCycleWeekday(rawValue: rawWeekday)
            else {
                return false
            }
            return weekdays.contains(weekday)
        }
    }

    public func occurrenceCount(
        from startDate: LocalDate,
        through endDate: LocalDate
    ) throws -> Int {
        try TaskCycleCivilCalendar.materializableDates(
            from: startDate,
            through: endDate
        ).count {
            includes($0, anchoredAt: startDate)
        }
    }

    func validateIntegrity() throws {
        switch self {
        case let .everyDays(interval):
            guard (1 ... 30).contains(interval) else {
                throw NoonmarkError.invalidInput(
                    "task cycle day interval must be between 1 and 30"
                )
            }
        case let .selectedWeekdays(weekdays, everyWeeks):
            guard weekdays.isEmpty == false,
                  (1 ... 4).contains(everyWeeks)
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle weekly schedule is outside supported bounds"
                )
            }
        }
    }
}

public enum TaskCycleEndCondition: Codable, Equatable, Hashable, Sendable {
    case durationDays(Int)
    case onDate(LocalDate)
    case completionTarget(count: Int, latestDate: LocalDate)

    public func resolvedEndDate(
        from startDate: LocalDate
    ) throws -> LocalDate {
        switch self {
        case let .durationDays(dayCount):
            guard (1 ... TaskCycleCivilCalendar.maximumMaterializedDayCount)
                .contains(dayCount),
                  let endDate = TaskCycleCivilCalendar.offset(
                      startDate,
                      by: dayCount - 1
                  )
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle duration must be between 1 and 366 days"
                )
            }
            return endDate
        case let .onDate(endDate):
            return endDate
        case let .completionTarget(count, latestDate):
            guard (1 ... TaskCycleCivilCalendar.maximumMaterializedDayCount)
                .contains(count)
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle completion target must be between 1 and 366"
                )
            }
            return latestDate
        }
    }
}

public struct TaskCyclePlanRevision: Codable, Equatable, Hashable, Sendable,
    Identifiable
{
    public let id: UUID
    public let effectiveFrom: LocalDate
    public let endConditionAnchorDate: LocalDate
    public let schedule: TaskCycleSchedule
    public let endCondition: TaskCycleEndCondition
    public let recordedAt: Date

    public init(
        id: UUID = UUID(),
        effectiveFrom: LocalDate,
        endConditionAnchorDate: LocalDate? = nil,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        recordedAt: Date
    ) {
        self.id = id
        self.effectiveFrom = effectiveFrom
        self.endConditionAnchorDate =
            endConditionAnchorDate ?? effectiveFrom
        self.schedule = schedule
        self.endCondition = endCondition
        self.recordedAt = recordedAt
    }
}

public struct TaskCycleRevisionOutcome: Equatable, Sendable {
    public let effectiveFrom: LocalDate
    public let cancelledOccurrenceCount: Int
    public let createdOccurrenceCount: Int
    public let classificationCommitBoundaries: [NoonmarkSnapshot]

    public init(
        effectiveFrom: LocalDate,
        cancelledOccurrenceCount: Int,
        createdOccurrenceCount: Int,
        classificationCommitBoundaries: [NoonmarkSnapshot] = []
    ) {
        self.effectiveFrom = effectiveFrom
        self.cancelledOccurrenceCount = cancelledOccurrenceCount
        self.createdOccurrenceCount = createdOccurrenceCount
        self.classificationCommitBoundaries =
            classificationCommitBoundaries
    }
}

public struct TaskCycleReconciliationOutcome: Equatable, Sendable {
    public let stoppedSeriesCount: Int
    public let cancelledOccurrenceCount: Int

    public init(
        stoppedSeriesCount: Int,
        cancelledOccurrenceCount: Int
    ) {
        self.stoppedSeriesCount = stoppedSeriesCount
        self.cancelledOccurrenceCount = cancelledOccurrenceCount
    }
}

public enum TaskCycleCancellationScope: Codable, Equatable, Hashable, Sendable {
    case occurrence(LocalDate)
    case followingDates(after: LocalDate)
}

public enum TaskCycleCancellationReason: String, Codable, Equatable,
    Hashable, Sendable
{
    case userSkipped
    case planRevision
    case stoppedEarly
    case completionTargetReached
}

public struct TaskCycleCancellationFact: Codable, Equatable, Hashable, Sendable {
    public let id: UUID
    public let scope: TaskCycleCancellationScope
    public let reason: TaskCycleCancellationReason
    public let recordedAt: Date

    public init(
        id: UUID = UUID(),
        scope: TaskCycleCancellationScope,
        reason: TaskCycleCancellationReason,
        recordedAt: Date
    ) {
        self.id = id
        self.scope = scope
        self.reason = reason
        self.recordedAt = recordedAt
    }
}

public struct TaskCycleClassificationRevision: Codable, Equatable, Sendable,
    Identifiable
{
    public let id: UUID
    public let previousRevisionID: UUID?
    public let interactionID: UUID
    public let source: ClassificationSource
    public let decisionID: UUID?
    public let categoryID: TaskCategoryID?
    public let labelIDs: Set<TaskLabelID>
    public let recordedAt: Date

    public init(
        id: UUID = UUID(),
        previousRevisionID: UUID?,
        interactionID: UUID = UUID(),
        source: ClassificationSource,
        decisionID: UUID?,
        categoryID: TaskCategoryID?,
        labelIDs: Set<TaskLabelID>,
        recordedAt: Date
    ) {
        self.id = id
        self.previousRevisionID = previousRevisionID
        self.interactionID = interactionID
        self.source = source
        self.decisionID = decisionID
        self.categoryID = categoryID
        self.labelIDs = labelIDs
        self.recordedAt = recordedAt
    }
}

public struct TaskCycleSeries: Codable, Equatable, Sendable {
    public let id: TaskCycleSeriesID
    public private(set) var title: String
    public private(set) var descriptionText: String?
    public private(set) var plannedSubtasks: [PlannedSubtask]
    public private(set) var categoryID: TaskCategoryID?
    public private(set) var labelIDs: Set<TaskLabelID>
    public private(set) var startDate: LocalDate
    public private(set) var endDate: LocalDate
    public private(set) var schedule: TaskCycleSchedule
    public private(set) var endCondition: TaskCycleEndCondition
    public private(set) var planRevisions: [TaskCyclePlanRevision]
    public private(set) var classificationRevisions:
        [TaskCycleClassificationRevision]
    public private(set) var cancellationFacts: [TaskCycleCancellationFact]
    public let createdAt: Date
    public private(set) var updatedAt: Date

    public init(
        id: TaskCycleSeriesID = TaskCycleSeriesID(),
        title: String,
        descriptionText: String? = nil,
        plannedSubtasks: [PlannedSubtask] = [],
        categoryID: TaskCategoryID? = nil,
        labelIDs: Set<TaskLabelID> = [],
        startDate: LocalDate,
        endDate: LocalDate,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition? = nil,
        planRevisions: [TaskCyclePlanRevision]? = nil,
        classificationRevisions: [TaskCycleClassificationRevision]? = nil,
        cancellationFacts: [TaskCycleCancellationFact] = [],
        createdAt: Date,
        updatedAt: Date? = nil
    ) {
        self.id = id
        self.title = title
        self.descriptionText = descriptionText
        self.plannedSubtasks = plannedSubtasks.sorted {
            $0.position < $1.position
        }
        self.categoryID = categoryID
        self.labelIDs = labelIDs
        self.startDate = startDate
        self.endDate = endDate
        self.schedule = schedule
        self.endCondition = endCondition ?? .onDate(endDate)
        self.planRevisions = planRevisions ?? [
            TaskCyclePlanRevision(
                effectiveFrom: startDate,
                endConditionAnchorDate: startDate,
                schedule: schedule,
                endCondition: endCondition ?? .onDate(endDate),
                recordedAt: createdAt
            )
        ]
        self.classificationRevisions = (
            classificationRevisions ?? [
                TaskCycleClassificationRevision(
                    previousRevisionID: nil,
                    source: .deterministicDomainAction(
                        reason: "task cycle created"
                    ),
                    decisionID: nil,
                    categoryID: categoryID,
                    labelIDs: labelIDs,
                    recordedAt: createdAt
                )
            ]
        ).sorted(by: Self.classificationRevisionComesBefore)
        self.cancellationFacts = cancellationFacts.sorted(
            by: Self.cancellationFactComesBefore
        )
        self.createdAt = createdAt
        self.updatedAt = updatedAt ?? createdAt
    }

    public var stoppedAfterDate: LocalDate? {
        cancellationFacts.compactMap { fact in
            if case let .followingDates(after: date) = fact.scope {
                return date
            }
            return nil
        }.min()
    }

    public func isOccurrenceSkipped(_ date: LocalDate) -> Bool {
        cancellationFacts.contains { fact in
            if case let .occurrence(skippedDate) = fact.scope {
                return skippedDate == date
            }
            return false
        }
    }

    func latestOccurrenceCancellationReason(
        on date: LocalDate
    ) -> TaskCycleCancellationReason? {
        cancellationFacts
            .filter {
                if case let .occurrence(value) = $0.scope {
                    return value == date
                }
                return false
            }
            .max {
                if $0.recordedAt != $1.recordedAt {
                    return $0.recordedAt < $1.recordedAt
                }
                return $0.id.uuidString < $1.id.uuidString
            }?
            .reason
    }

    mutating func appendCancellation(
        _ scope: TaskCycleCancellationScope,
        reason: TaskCycleCancellationReason,
        now: Date
    ) throws {
        guard updatedAt.timeIntervalSinceReferenceDate.isFinite,
              now.timeIntervalSinceReferenceDate.isFinite,
              now >= updatedAt
        else {
            throw NoonmarkError.invalidInput(
                "content mutation time cannot move backwards"
            )
        }
        updatedAt = now
        cancellationFacts.append(
            TaskCycleCancellationFact(
                scope: scope,
                reason: reason,
                recordedAt: now
            )
        )
        cancellationFacts.sort(by: Self.cancellationFactComesBefore)
    }

    mutating func appendPlanRevision(
        effectiveFrom: LocalDate,
        endConditionAnchorDate: LocalDate? = nil,
        updatesStartDate: Bool = false,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        now: Date
    ) throws {
        guard stoppedAfterDate == nil else {
            throw NoonmarkError.invalidTransition(
                "a stopped task cycle cannot be revised"
            )
        }
        guard now.timeIntervalSinceReferenceDate.isFinite,
              now >= updatedAt
        else {
            throw NoonmarkError.invalidInput(
                "content mutation time cannot move backwards"
            )
        }
        let anchorDate = endConditionAnchorDate ?? startDate
        let endDate = try endCondition.resolvedEndDate(
            from: anchorDate
        )
        guard effectiveFrom >= anchorDate,
              effectiveFrom <= endDate
        else {
            throw NoonmarkError.invalidTransition(
                "task cycle revision must retain a future occurrence window"
            )
        }
        try schedule.validateIntegrity()
        _ = try TaskCycleCivilCalendar.materializableDates(
            from: anchorDate,
            through: endDate
        )
        if updatesStartDate {
            guard effectiveFrom == anchorDate else {
                throw NoonmarkError.invalidTransition(
                    "task cycle start revision must begin at its new start date"
                )
            }
            startDate = anchorDate
        }
        self.schedule = schedule
        self.endCondition = endCondition
        self.endDate = endDate
        planRevisions.append(
            TaskCyclePlanRevision(
                effectiveFrom: effectiveFrom,
                endConditionAnchorDate: anchorDate,
                schedule: schedule,
                endCondition: endCondition,
                recordedAt: now
            )
        )
        updatedAt = now
    }

    mutating func updateTemplateContent(
        title: String,
        descriptionText: String?,
        plannedSubtasks: [PlannedSubtask],
        now: Date
    ) throws {
        guard now.timeIntervalSinceReferenceDate.isFinite,
              now >= updatedAt
        else {
            throw NoonmarkError.invalidInput(
                "content mutation time cannot move backwards"
            )
        }
        self.title = title
        self.descriptionText = descriptionText
        self.plannedSubtasks = plannedSubtasks.sorted {
            $0.position < $1.position
        }
        updatedAt = now
    }

    mutating func updateTemplateClassification(
        categoryID: TaskCategoryID?,
        labelIDs: Set<TaskLabelID>,
        source: ClassificationSource,
        interactionID: UUID,
        decisionID: UUID?,
        now: Date
    ) throws {
        try requireUnusedClassificationInteraction(interactionID)
        guard now.timeIntervalSinceReferenceDate.isFinite,
              now >= updatedAt
        else {
            throw NoonmarkError.invalidInput(
                "content mutation time cannot move backwards"
            )
        }
        self.categoryID = categoryID
        self.labelIDs = labelIDs
        classificationRevisions.append(
            TaskCycleClassificationRevision(
                previousRevisionID:
                currentClassificationRevision?.id,
                interactionID: interactionID,
                source: source,
                decisionID: decisionID,
                categoryID: categoryID,
                labelIDs: labelIDs,
                recordedAt: now
            )
        )
        classificationRevisions.sort(
            by: Self.classificationRevisionComesBefore
        )
        updatedAt = now
    }

    func requireUnusedClassificationInteraction(
        _ interactionID: UUID
    ) throws {
        guard classificationRevisions.contains(where: {
            $0.interactionID == interactionID
        }) == false else {
            throw NoonmarkError.invalidTransition(
                "task cycle classification interaction was reused"
            )
        }
    }

    public var currentClassificationRevision:
        TaskCycleClassificationRevision?
    {
        Self.resolveCurrentClassificationRevision(
            in: classificationRevisions
        )
    }

    public static func resolveCurrentClassificationRevision(
        in revisions: [TaskCycleClassificationRevision]
    ) -> TaskCycleClassificationRevision? {
        let revisionsWithChildren = Set(
            revisions.compactMap(\.previousRevisionID)
        )
        return revisions
            .filter { revisionsWithChildren.contains($0.id) == false }
            .max(by: classificationRevisionComesBefore)
    }

    private static func classificationRevisionComesBefore(
        _ lhs: TaskCycleClassificationRevision,
        _ rhs: TaskCycleClassificationRevision
    ) -> Bool {
        if lhs.recordedAt != rhs.recordedAt {
            return lhs.recordedAt < rhs.recordedAt
        }
        return lhs.id.uuidString < rhs.id.uuidString
    }

    public func validateIntegrity() throws {
        let normalizedTitle = title.trimmingCharacters(
            in: .whitespacesAndNewlines
        )
        guard normalizedTitle.isEmpty == false,
              normalizedTitle == title,
              plannedSubtasks.enumerated().allSatisfy({
                  $0.element.position == $0.offset + 1
              }),
              Set(plannedSubtasks.map(\.id)).count
              == plannedSubtasks.count,
              Set(plannedSubtasks.map(\.lineageID)).count
              == plannedSubtasks.count,
              plannedSubtasks.allSatisfy({
                  $0.title.trimmingCharacters(
                      in: .whitespacesAndNewlines
                  ) == $0.title
                      && $0.title.isEmpty == false
                      && $0.createdAt.timeIntervalSinceReferenceDate
                      .isFinite
                      && $0.createdAt <= updatedAt
              }),
              createdAt.timeIntervalSinceReferenceDate.isFinite,
              updatedAt.timeIntervalSinceReferenceDate.isFinite,
              updatedAt >= createdAt
        else {
            throw NoonmarkError.invalidInput(
                "task cycle series contains invalid content facts"
            )
        }
        _ = try TaskCycleCivilCalendar.materializableDates(
            from: startDate,
            through: endDate
        )
        guard try endCondition.resolvedEndDate(from: startDate) == endDate,
              planRevisions.isEmpty == false,
              Set(planRevisions.map(\.id)).count == planRevisions.count
        else {
            throw NoonmarkError.invalidInput(
                "task cycle end condition does not match its end date"
            )
        }
        try schedule.validateIntegrity()
        for revision in planRevisions {
            guard revision.effectiveFrom
                >= revision.endConditionAnchorDate,
                  revision.recordedAt.timeIntervalSinceReferenceDate.isFinite,
                  revision.recordedAt >= createdAt,
                  revision.recordedAt <= updatedAt
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle contains an invalid plan revision"
                )
            }
            try revision.schedule.validateIntegrity()
            let revisionEndDate = try revision.endCondition
                .resolvedEndDate(
                    from: revision.endConditionAnchorDate
                )
            _ = try TaskCycleCivilCalendar.materializableDates(
                from: revision.endConditionAnchorDate,
                through: revisionEndDate
            )
        }
        guard let latestRevision = planRevisions.last,
            latestRevision.schedule == schedule,
            latestRevision.endCondition == endCondition,
            latestRevision.endConditionAnchorDate == startDate
        else {
            throw NoonmarkError.invalidInput(
                "task cycle current plan does not match its revision facts"
            )
        }
        try validateClassificationRevisions()
        let scheduledDates = try everPlannedDates()
        guard scheduledDates.isEmpty == false,
              Set(cancellationFacts.map(\.id)).count
              == cancellationFacts.count,
              cancellationFacts
              == cancellationFacts.sorted(
                  by: Self.cancellationFactComesBefore
              )
        else {
            throw NoonmarkError.invalidInput(
                "task cycle series contains invalid schedule facts"
            )
        }
        for fact in cancellationFacts {
            guard fact.recordedAt.timeIntervalSinceReferenceDate.isFinite,
                  fact.recordedAt >= createdAt,
                  fact.recordedAt <= updatedAt
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle series contains invalid cancellation facts"
                )
            }
            switch fact.scope {
            case let .occurrence(date):
                guard scheduledDates.contains(date) else {
                    throw NoonmarkError.invalidInput(
                        "task cycle series contains invalid cancellation facts"
                    )
                }
            case let .followingDates(after: date):
                guard date < endDate else {
                    throw NoonmarkError.invalidInput(
                        "task cycle series contains invalid cancellation facts"
                    )
                }
            }
        }
    }

    private static func cancellationFactComesBefore(
        _ lhs: TaskCycleCancellationFact,
        _ rhs: TaskCycleCancellationFact
    ) -> Bool {
        if lhs.recordedAt != rhs.recordedAt {
            return lhs.recordedAt < rhs.recordedAt
        }
        return lhs.id.uuidString < rhs.id.uuidString
    }

    private func validateClassificationRevisions() throws {
        guard classificationRevisions.isEmpty == false,
              Set(classificationRevisions.map(\.id)).count
              == classificationRevisions.count,
              Set(classificationRevisions.map(\.interactionID)).count
              == classificationRevisions.count,
              classificationRevisions
              == classificationRevisions.sorted(
                  by: Self.classificationRevisionComesBefore
              ),
              classificationRevisions.filter({
                  $0.previousRevisionID == nil
              }).count == 1
        else {
            throw NoonmarkError.invalidInput(
                "task cycle contains invalid classification revisions"
            )
        }
        let revisionsByID = Dictionary(
            uniqueKeysWithValues: classificationRevisions.map {
                ($0.id, $0)
            }
        )
        for revision in classificationRevisions {
            guard revision.recordedAt.timeIntervalSinceReferenceDate.isFinite,
                  revision.recordedAt >= createdAt,
                  revision.recordedAt <= updatedAt
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle contains invalid classification revisions"
                )
            }
            if let previousRevisionID = revision.previousRevisionID {
                guard previousRevisionID != revision.id,
                      let previous = revisionsByID[previousRevisionID],
                      previous.recordedAt <= revision.recordedAt
                else {
                    throw NoonmarkError.invalidInput(
                        "task cycle classification revision ancestry is invalid"
                    )
                }
            }
            var visited: Set<UUID> = [revision.id]
            var previousRevisionID = revision.previousRevisionID
            while let revisionID = previousRevisionID {
                guard visited.insert(revisionID).inserted,
                      let previous = revisionsByID[revisionID]
                else {
                    throw NoonmarkError.invalidInput(
                        "task cycle classification revision ancestry is invalid"
                    )
                }
                previousRevisionID = previous.previousRevisionID
            }
        }
        guard let currentClassificationRevision,
              currentClassificationRevision.categoryID == categoryID,
              currentClassificationRevision.labelIDs == labelIDs
        else {
            throw NoonmarkError.invalidInput(
                "task cycle current classification does not match its revision facts"
            )
        }
    }

    func everPlannedDates() throws -> Set<LocalDate> {
        var result: Set<LocalDate> = []
        for revision in planRevisions {
            let revisionEndDate = try revision.endCondition
                .resolvedEndDate(
                    from: revision.endConditionAnchorDate
                )
            let dates = try TaskCycleCivilCalendar.materializableDates(
                from: revision.effectiveFrom,
                through: revisionEndDate
            )
            result.formUnion(
                dates.filter {
                    revision.schedule.includes(
                        $0,
                        anchoredAt: revision.effectiveFrom
                    )
                }
            )
        }
        return result
    }

    func trackBounds() throws -> (
        startDate: LocalDate,
        endDate: LocalDate
    ) {
        let revisionEndDates = try planRevisions.map {
            try $0.endCondition.resolvedEndDate(
                from: $0.endConditionAnchorDate
            )
        }
        let boundaryDates = [startDate, endDate]
            + planRevisions.map(\.effectiveFrom)
            + planRevisions.map(\.endConditionAnchorDate)
            + revisionEndDates
        guard let earliestDate = boundaryDates.min(),
              let latestDate = boundaryDates.max()
        else {
            throw NoonmarkError.invalidInput(
                "task cycle contains no track boundaries"
            )
        }
        return (earliestDate, latestDate)
    }
}

public struct TaskCycleMembership: Codable, Equatable, Hashable, Sendable {
    public let seriesID: TaskCycleSeriesID
    public let occurrenceDate: LocalDate

    public init(
        seriesID: TaskCycleSeriesID,
        occurrenceDate: LocalDate
    ) {
        self.seriesID = seriesID
        self.occurrenceDate = occurrenceDate
    }
}

public enum TaskCycleTrackDayState: String, Codable, Equatable, Hashable, Sendable {
    case notScheduled
    case pendingPast
    case pendingToday
    case planned
    case completed
    case unfinished
    case deferred
    case changed
    case returnedToPool
    case skipped
    case abandoned
}

public struct TaskCycleTraceTarget: Equatable, Sendable {
    public let traceID: DayTraceID
    public let date: LocalDate
    public let state: TaskCycleTrackDayState
}

public struct TaskCycleTrackDay: Equatable, Identifiable, Sendable {
    public var id: LocalDate { date }
    public let date: LocalDate
    public let chainID: TaskChainID?
    public let occurrenceTarget: TaskCycleTraceTarget?
    public let futurePlanTarget: TaskCycleTraceTarget?
    public let unfinishedTarget: TaskCycleTraceTarget?
    public let completedTarget: TaskCycleTraceTarget?

    public var state: TaskCycleTrackDayState {
        occurrenceTarget?.state ?? .notScheduled
    }

    public init(
        date: LocalDate,
        chainID: TaskChainID?,
        occurrenceTarget: TaskCycleTraceTarget?,
        futurePlanTarget: TaskCycleTraceTarget?,
        unfinishedTarget: TaskCycleTraceTarget?,
        completedTarget: TaskCycleTraceTarget?
    ) {
        self.date = date
        self.chainID = chainID
        self.occurrenceTarget = occurrenceTarget
        self.futurePlanTarget = futurePlanTarget
        self.unfinishedTarget = unfinishedTarget
        self.completedTarget = completedTarget
    }

    public var navigationTarget: TaskCycleTraceTarget? {
        guard state != .skipped, state != .returnedToPool else {
            return nil
        }
        return occurrenceTarget
    }
}

public enum TaskCycleLifecycle: Equatable, Sendable {
    case upcoming(startDate: LocalDate)
    case active(nextDate: LocalDate?)
    case ended(endDate: LocalDate)
    case stopped(stopDate: LocalDate)

    public var allowsPlanEditing: Bool {
        switch self {
        case .active, .upcoming:
            true
        case .ended, .stopped:
            false
        }
    }
}

public struct TaskCycleTrack: Equatable, Identifiable, Sendable {
    public let id: TaskCycleSeriesID
    public let title: String
    public let startDate: LocalDate
    public let endDate: LocalDate
    public let schedule: TaskCycleSchedule
    public let endCondition: TaskCycleEndCondition
    public let lifecycle: TaskCycleLifecycle
    public let days: [TaskCycleTrackDay]

    public init(
        id: TaskCycleSeriesID,
        title: String,
        startDate: LocalDate,
        endDate: LocalDate,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        lifecycle: TaskCycleLifecycle,
        days: [TaskCycleTrackDay]
    ) {
        self.id = id
        self.title = title
        self.startDate = startDate
        self.endDate = endDate
        self.schedule = schedule
        self.endCondition = endCondition
        self.lifecycle = lifecycle
        self.days = days
    }

    public var scheduledCount: Int {
        days.count { $0.state != .notScheduled }
    }

    public var completedCount: Int {
        days.count { $0.completedTarget != nil }
    }

    public var unfinishedCount: Int {
        days.count { $0.unfinishedTarget != nil }
    }

    public var plannedCount: Int {
        days.count { $0.state == .planned }
    }

    public var futurePlanCount: Int {
        days.count { $0.futurePlanTarget != nil }
    }

    public var pooledOccurrenceCount: Int {
        days.count { $0.state == .returnedToPool }
    }

    public var completionGoalCount: Int {
        switch endCondition {
        case let .completionTarget(count, _):
            count
        case .durationDays, .onDate:
            scheduledCount
        }
    }

    public var canStop: Bool {
        lifecycle.allowsPlanEditing && futurePlanCount > 0
    }
}

extension TaskCycleSeries {
    func lifecycle(on today: LocalDate) -> TaskCycleLifecycle {
        let terminalFacts: [(
            date: LocalDate,
            reason: TaskCycleCancellationReason,
            recordedAt: Date,
            id: UUID
        )] = cancellationFacts.compactMap { fact in
            guard case let .followingDates(after: date) = fact.scope,
                  fact.reason == .stoppedEarly
                    || fact.reason == .completionTargetReached
            else {
                return nil
            }
            return (
                date: date,
                reason: fact.reason,
                recordedAt: fact.recordedAt,
                id: fact.id
            )
        }
        let terminalFact = terminalFacts.min {
            if $0.date != $1.date {
                return $0.date < $1.date
            }
            if $0.recordedAt != $1.recordedAt {
                return $0.recordedAt < $1.recordedAt
            }
            return $0.id.uuidString < $1.id.uuidString
        }
        if let terminalFact {
            switch terminalFact.reason {
            case .stoppedEarly:
                return .stopped(stopDate: terminalFact.date)
            case .completionTargetReached:
                return .ended(endDate: terminalFact.date)
            case .userSkipped, .planRevision:
                break
            }
        }
        if today < startDate {
            return .upcoming(startDate: startDate)
        }
        if today > endDate {
            return .ended(endDate: endDate)
        }
        return .active(nextDate: nil)
    }

    func requirePlanEditing(
        on today: LocalDate,
        operation: String
    ) throws {
        switch lifecycle(on: today) {
        case .active, .upcoming:
            return
        case .ended:
            throw NoonmarkError.invalidTransition(
                "an ended task cycle cannot be \(operation)"
            )
        case .stopped:
            throw NoonmarkError.invalidTransition(
                "a stopped task cycle cannot be \(operation)"
            )
        }
    }

    func taskCycleLifecycle(
        today: LocalDate,
        days: [TaskCycleTrackDay]
    ) -> TaskCycleLifecycle {
        let baseLifecycle = lifecycle(on: today)
        guard case .active = baseLifecycle else {
            return baseLifecycle
        }
        let nextDate = days.compactMap { day -> LocalDate? in
            if let futurePlanTarget = day.futurePlanTarget {
                return futurePlanTarget.date
            }
            guard day.date == today,
                  day.state == .pendingToday
            else {
                return nil
            }
            return day.occurrenceTarget?.date ?? day.date
        }.min()
        return .active(nextDate: nextDate)
    }
}

private extension TaskCycleLifecycle {
    var sortRank: Int {
        switch self {
        case .active:
            0
        case .upcoming:
            1
        case .ended:
            2
        case .stopped:
            3
        }
    }

    var sortDate: LocalDate? {
        switch self {
        case let .active(nextDate):
            nextDate
        case let .upcoming(startDate):
            startDate
        case let .ended(endDate):
            endDate
        case let .stopped(stopDate):
            stopDate
        }
    }
}

public extension NoonmarkEngine {
    @discardableResult
    func createTaskCycleSeries(
        title: String,
        descriptionText: String? = nil,
        startDate: LocalDate,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        try createTaskCycleSeries(
            title: title,
            descriptionText: descriptionText,
            plannedSubtasks: [],
            startDate: startDate,
            schedule: schedule,
            endCondition: endCondition,
            today: today,
            now: now
        )
    }

    @discardableResult
    func createTaskCycleSeries(
        title: String,
        descriptionText: String? = nil,
        startDate: LocalDate,
        endDate: LocalDate,
        schedule: TaskCycleSchedule,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        try createTaskCycleSeries(
            title: title,
            descriptionText: descriptionText,
            plannedSubtasks: [],
            startDate: startDate,
            endDate: endDate,
            schedule: schedule,
            today: today,
            now: now
        )
    }

    @discardableResult
    func createTaskCycleSeries(
        title: String,
        descriptionText: String? = nil,
        plannedSubtasks: [PlannedSubtask],
        startDate: LocalDate,
        endDate: LocalDate,
        schedule: TaskCycleSchedule,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        try createTaskCycleSeries(
            title: title,
            descriptionText: descriptionText,
            plannedSubtasks: plannedSubtasks,
            startDate: startDate,
            schedule: schedule,
            endCondition: .onDate(endDate),
            today: today,
            now: now
        )
    }

    @discardableResult
    func createTaskCycleSeries(
        title: String,
        descriptionText: String? = nil,
        plannedSubtasks: [PlannedSubtask],
        startDate: LocalDate,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        let normalizedTitle = try normalizedTaskCycleTitle(title)
        guard startDate >= today else {
            throw NoonmarkError.invalidTransition(
                "task cycle cannot begin in the past"
            )
        }
        let endDate = try endCondition.resolvedEndDate(
            from: startDate
        )
        guard startDate <= endDate else {
            throw NoonmarkError.invalidInput(
                "task cycle end date must not precede its start date"
            )
        }
        let dates = try TaskCycleCivilCalendar.materializableDates(
            from: startDate,
            through: endDate
        )
        let occurrenceDates = dates.filter {
            schedule.includes($0, anchoredAt: startDate)
        }
        guard occurrenceDates.isEmpty == false else {
            throw NoonmarkError.invalidInput(
                "task cycle schedule contains no occurrences"
            )
        }

        let series = TaskCycleSeries(
            title: normalizedTitle,
            descriptionText: descriptionText,
            plannedSubtasks: plannedSubtasks,
            startDate: startDate,
            endDate: endDate,
            schedule: schedule,
            endCondition: endCondition,
            createdAt: now
        )
        storeTaskCycleSeries(series)
        for occurrenceDate in occurrenceDates {
            let membership = TaskCycleMembership(
                seriesID: series.id,
                occurrenceDate: occurrenceDate
            )
            installTaskCycleOccurrence(
                normalizedTitle: normalizedTitle,
                descriptionText: descriptionText,
                plannedSubtasks: plannedSubtasks,
                membership: membership,
                now: now
            )
        }
        return series.id
    }

    @discardableResult
    func convertTaskToCycleSeries(
        chainID: TaskChainID,
        startDate: LocalDate,
        endDate: LocalDate,
        schedule: TaskCycleSchedule,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        try convertTaskToCycleSeries(
            chainID: chainID,
            startDate: startDate,
            schedule: schedule,
            endCondition: .onDate(endDate),
            today: today,
            now: now
        )
    }

    @discardableResult
    func convertTaskToCycleSeries(
        chainID: TaskChainID,
        startDate: LocalDate,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleSeriesID {
        let source = try taskCycleConversionSource(chainID: chainID)
        let sourceChain = source.chain
        guard sourceChain.state == .active,
              sourceChain.cycleMembership == nil
        else {
            throw NoonmarkError.invalidTransition(
                "only a standalone active task can become recurring"
            )
        }
        let definition = source.definition
        let normalizedTitle = try normalizedTaskCycleTitle(
            definition.title
        )
        let endDate = try endCondition.resolvedEndDate(from: startDate)
        guard startDate >= today, startDate <= endDate else {
            throw NoonmarkError.invalidTransition(
                "task cycle cannot begin in the past"
            )
        }
        let dates = try TaskCycleCivilCalendar.materializableDates(
            from: startDate,
            through: endDate
        )
        let occurrenceDates = dates.filter {
            schedule.includes($0, anchoredAt: startDate)
        }
        guard let firstOccurrenceDate = occurrenceDates.first else {
            throw NoonmarkError.invalidInput(
                "task cycle schedule contains no occurrences"
            )
        }
        let active = source.activeTrace
        if let active {
            guard active.status == .pending,
                  active.date == firstOccurrenceDate
            else {
                throw NoonmarkError.invalidTransition(
                    "an active task must keep its scheduled date when becoming recurring"
                )
            }
        } else {
            guard source.isInTaskPool else {
                throw NoonmarkError.invalidTransition(
                    "historical tasks cannot be rewritten as recurring occurrences"
                )
            }
        }
        try ensureTaskCycleDatesUnlocked(occurrenceDates)

        let descriptionText = active?.descriptionText
            ?? definition.descriptionText
        let series = TaskCycleSeries(
            title: normalizedTitle,
            descriptionText: descriptionText,
            plannedSubtasks: definition.plannedSubtasks,
            startDate: startDate,
            endDate: endDate,
            schedule: schedule,
            endCondition: endCondition,
            createdAt: now
        )
        let adoption = try prepareTaskCycleAdoption(
            chainID: chainID,
            membership: TaskCycleMembership(
                seriesID: series.id,
                occurrenceDate: firstOccurrenceDate
            ),
            descriptionText: descriptionText,
            now: now
        )
        storeTaskCycleSeries(series)
        storePreparedTaskCycleAdoption(
            chain: adoption.chain,
            insertedTrace: adoption.insertedTrace,
            now: now
        )
        for occurrenceDate in occurrenceDates.dropFirst() {
            installTaskCycleOccurrence(
                normalizedTitle: normalizedTitle,
                descriptionText: descriptionText,
                plannedSubtasks: definition.plannedSubtasks,
                membership: TaskCycleMembership(
                    seriesID: series.id,
                    occurrenceDate: occurrenceDate
                ),
                now: now
            )
        }
        return series.id
    }

    func taskCycleTracks(today: LocalDate) -> [TaskCycleTrack] {
        let projectionIndex = taskCycleProjectionIndex()
        return taskCycleSeries.values
            .compactMap { series -> TaskCycleTrack? in
                taskCycleTrack(
                    series: series,
                    today: today,
                    projectionIndex: projectionIndex
                )
            }
            .sorted {
                let lhsLifecycle = $0.lifecycle
                let rhsLifecycle = $1.lifecycle
                if lhsLifecycle.sortRank != rhsLifecycle.sortRank {
                    return lhsLifecycle.sortRank < rhsLifecycle.sortRank
                }
                let lhsDate = lhsLifecycle.sortDate
                let rhsDate = rhsLifecycle.sortDate
                if lhsDate != rhsDate {
                    if lhsLifecycle.sortRank >= 2 {
                        return (lhsDate ?? $0.endDate)
                            > (rhsDate ?? $1.endDate)
                    }
                    guard let lhsDate else {
                        return true
                    }
                    guard let rhsDate else {
                        return false
                    }
                    return lhsDate < rhsDate
                }
                return $0.id.description < $1.id.description
            }
    }

    func taskCycleTrack(
        seriesID: TaskCycleSeriesID,
        today: LocalDate
    ) -> TaskCycleTrack? {
        guard let series = taskCycleSeries[seriesID] else {
            return nil
        }
        return taskCycleTrack(
            series: series,
            today: today,
            projectionIndex: taskCycleProjectionIndex()
        )
    }

    private struct TaskCycleProjectionIndex {
        let membersBySeries: [
            TaskCycleSeriesID: [
                (
                    chain: TaskChain,
                    membership: TaskCycleMembership
                )
            ]
        ]
        let tracesByChain: [TaskChainID: [DayTrace]]
    }

    private func taskCycleProjectionIndex()
        -> TaskCycleProjectionIndex
    {
        TaskCycleProjectionIndex(
            membersBySeries: Dictionary(
                grouping: chains.values.compactMap {
                    chain -> (
                        chain: TaskChain,
                        membership: TaskCycleMembership
                    )? in
                    guard let membership =
                        chain.cycleMembership
                    else {
                        return nil
                    }
                    return (chain, membership)
                },
                by: { $0.membership.seriesID }
            ),
            tracesByChain: Dictionary(
                grouping: traces.values,
                by: \.chainID
            )
        )
    }

    private func taskCycleTrack(
        series: TaskCycleSeries,
        today: LocalDate,
        projectionIndex: TaskCycleProjectionIndex
    ) -> TaskCycleTrack? {
        let entries =
            projectionIndex.membersBySeries[series.id] ?? []
        let membershipDates = entries.map(
            \.membership.occurrenceDate
        )
        guard let seriesBounds = try? series.trackBounds()
        else {
            return nil
        }
        let allDates = membershipDates + [
            seriesBounds.startDate,
            seriesBounds.endDate
        ]
        guard let trackStartDate = allDates.min(),
              let trackEndDate = allDates.max(),
              let dates = try? TaskCycleCivilCalendar
              .materializableDates(
                  from: trackStartDate,
                  through: trackEndDate
              )
        else {
            return nil
        }
        let entriesByDate = Dictionary(
            uniqueKeysWithValues: entries.map {
                ($0.membership.occurrenceDate, $0.chain)
            }
        )
        let days = dates.map { date in
            guard let chain = entriesByDate[date] else {
                return TaskCycleTrackDay(
                    date: date,
                    chainID: nil,
                    occurrenceTarget: nil,
                    futurePlanTarget: nil,
                    unfinishedTarget: nil,
                    completedTarget: nil
                )
            }
            let chainTraces =
                projectionIndex.tracesByChain[chain.id] ?? []
            let scheduledTrace = latestTaskCycleTrace(
                chainTraces.filter { $0.date == date }
            )
            let latestTrace = latestTaskCycleTrace(chainTraces)
            let representativeTrace =
                if scheduledTrace?.status == .cancelledDraft,
                   latestTrace?.id != scheduledTrace?.id
                {
                    latestTrace
                } else {
                    scheduledTrace ?? latestTrace
                }
            let futurePlanTrace = latestTaskCycleTrace(
                chainTraces.filter {
                    $0.status == .pending && $0.date > today
                }
            )
            let completedTrace = latestTaskCycleTrace(
                chainTraces.filter {
                    $0.status == .completed
                }
            )
            let unfinishedTrace = completedTrace == nil
                ? latestTaskCycleTrace(
                    chainTraces.filter {
                        $0.status == .unfinished
                            || $0.status == .abandoned
                    }
                )
                : nil
            return TaskCycleTrackDay(
                date: date,
                chainID: chain.id,
                occurrenceTarget: taskCycleTraceTarget(
                    representativeTrace,
                    today: today,
                    cancelledState: series
                        .isOccurrenceSkipped(date)
                        ? .skipped
                        : .returnedToPool
                ),
                futurePlanTarget: taskCycleTraceTarget(
                    futurePlanTrace,
                    today: today
                ),
                unfinishedTarget: taskCycleTraceTarget(
                    unfinishedTrace,
                    today: today
                ),
                completedTarget: taskCycleTraceTarget(
                    completedTrace,
                    today: today
                )
            )
        }
        return TaskCycleTrack(
            id: series.id,
            title: series.title,
            startDate: trackStartDate,
            endDate: trackEndDate,
            schedule: series.schedule,
            endCondition: series.endCondition,
            lifecycle: series.taskCycleLifecycle(
                today: today,
                days: days
            ),
            days: days
        )
    }

    func taskCycleTemplateClassification(
        seriesID: TaskCycleSeriesID
    ) throws -> TaskClassificationProjection {
        let series = try taskCycleSeriesValue(seriesID)
        let category = series.categoryID.flatMap { id in
            classificationState.categories[id].map {
                ClassificationItemProjection(
                    id: id.description,
                    name: $0.name,
                    colorHex: $0.colorHex,
                    presentationApproval: $0.presentationApproval
                )
            }
        }
        let labels = series.labelIDs.compactMap { id in
            classificationState.labels[id].map {
                ClassificationItemProjection(
                    id: id.description,
                    name: $0.name,
                    colorHex: $0.colorHex
                )
            }
        }
        .sorted {
            $0.name.localizedStandardCompare($1.name) == .orderedAscending
        }
        let anchorID = chains.values
            .filter { $0.cycleMembership?.seriesID == seriesID }
            .sorted {
                ($0.cycleMembership?.occurrenceDate ?? series.startDate)
                    < ($1.cycleMembership?.occurrenceDate
                        ?? series.startDate)
            }
            .first?.id ?? TaskChainID()
        return TaskClassificationProjection(
            chainID: anchorID,
            category: category,
            labels: labels,
            revision: classificationState.revision
        )
    }

    func taskCycleEditableMemberChainIDs(
        seriesID: TaskCycleSeriesID,
        today: LocalDate
    ) -> [TaskChainID] {
        chains.values
            .filter { chain in
                guard chain.state == .active,
                      let membership = chain.cycleMembership,
                      membership.seriesID == seriesID,
                      membership.occurrenceDate >= today
                else {
                    return false
                }
                return traces.values.contains {
                    $0.chainID == chain.id
                        && $0.status == .pending
                        && $0.date >= today
                }
            }
            .sorted {
                guard let lhs = $0.cycleMembership,
                      let rhs = $1.cycleMembership
                else {
                    return $0.id.description < $1.id.description
                }
                if lhs.occurrenceDate != rhs.occurrenceDate {
                    return lhs.occurrenceDate < rhs.occurrenceDate
                }
                return $0.id.description < $1.id.description
            }
            .map(\.id)
    }

    func updateTaskCycleTemplateContent(
        seriesID: TaskCycleSeriesID,
        title: String,
        descriptionText: String?,
        plannedSubtasks: [PlannedSubtask],
        today: LocalDate,
        now: Date = Date()
    ) throws {
        let candidate = NoonmarkEngine(copying: self)
        try candidate.updateTaskCycleTemplateContentInPlace(
            seriesID: seriesID,
            title: title,
            descriptionText: descriptionText,
            plannedSubtasks: plannedSubtasks,
            today: today,
            now: now
        )
        try candidate.snapshot().validateIntegrity()
        adoptState(from: candidate)
    }

    func setTaskCycleTemplateClassification(
        seriesID: TaskCycleSeriesID,
        categoryID: TaskCategoryID?,
        labelIDs: Set<TaskLabelID>,
        source: ClassificationSource = .deterministicDomainAction(
            reason: "task cycle template classification"
        ),
        interactionID: UUID = UUID(),
        decisionID: UUID? = nil,
        now: Date = Date()
    ) throws {
        guard categoryID.map({
            classificationState.categories[$0] != nil
        }) ?? true,
            labelIDs.allSatisfy({
                classificationState.labels[$0] != nil
            })
        else {
            throw NoonmarkError.invalidTransition(
                "task cycle classification references a missing item"
            )
        }
        var series = try taskCycleSeriesValue(seriesID)
        try series.updateTemplateClassification(
            categoryID: categoryID,
            labelIDs: labelIDs,
            source: source,
            interactionID: interactionID,
            decisionID: decisionID,
            now: now
        )
        storeTaskCycleSeries(series)
    }

    func canReviseTaskCycleStartDate(
        seriesID: TaskCycleSeriesID,
        today: LocalDate
    ) -> Bool {
        guard let series = taskCycleSeries[seriesID],
              series.stoppedAfterDate == nil,
              series.startDate > today
        else {
            return false
        }
        let memberChainIDs = Set(chains.values.compactMap { chain in
            chain.cycleMembership?.seriesID == seriesID
                ? chain.id
                : nil
        })
        guard memberChainIDs.isEmpty == false else {
            return false
        }
        return traces.values
            .filter { memberChainIDs.contains($0.chainID) }
            .allSatisfy {
                $0.status == .cancelledDraft
                    || ($0.status == .pending && $0.date > today)
            }
    }

    @discardableResult
    func reviseTaskCycleSeries(
        seriesID: TaskCycleSeriesID,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleRevisionOutcome {
        try reviseTaskCycleSeries(
            seriesID: seriesID,
            startDate: nil,
            schedule: schedule,
            endCondition: endCondition,
            today: today,
            now: now
        )
    }

    @discardableResult
    func reviseTaskCycleSeries(
        seriesID: TaskCycleSeriesID,
        startDate revisedStartDate: LocalDate?,
        schedule: TaskCycleSchedule,
        endCondition: TaskCycleEndCondition,
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleRevisionOutcome {
        var series = try taskCycleSeriesValue(seriesID)
        try series.requirePlanEditing(
            on: today,
            operation: "revised"
        )
        guard let tomorrow = TaskCycleCivilCalendar.offset(today, by: 1)
        else {
            throw NoonmarkError.invalidInput(
                "task cycle revision date could not advance"
            )
        }
        let revisesStartDate = revisedStartDate.map {
            $0 != series.startDate
        } ?? false
        if revisesStartDate {
            guard canReviseTaskCycleStartDate(
                seriesID: seriesID,
                today: today
            ), let revisedStartDate,
                revisedStartDate >= today
            else {
                throw NoonmarkError.invalidTransition(
                    "only an unstarted task cycle without execution history can change its start date"
                )
            }
        }
        let planStartDate =
            if revisesStartDate, let revisedStartDate {
                revisedStartDate
            } else {
                series.startDate
            }
        let effectiveFrom = revisesStartDate
            ? planStartDate
            : max(series.startDate, tomorrow)
        let editableWindowStart = revisesStartDate
            ? today
            : effectiveFrom
        let revisedEndDate = try endCondition.resolvedEndDate(
            from: planStartDate
        )
        guard revisedEndDate >= effectiveFrom else {
            throw NoonmarkError.invalidTransition(
                "task cycle revision must retain a future occurrence window"
            )
        }
        try schedule.validateIntegrity()
        let revisedDates = try TaskCycleCivilCalendar.materializableDates(
            from: effectiveFrom,
            through: revisedEndDate
        ).filter {
            schedule.includes($0, anchoredAt: effectiveFrom)
        }
        guard revisedDates.isEmpty == false else {
            throw NoonmarkError.invalidInput(
                "task cycle schedule contains no occurrences"
            )
        }

        let memberChains = chains.values
            .filter {
                $0.cycleMembership?.seriesID == seriesID
            }
            .sorted {
                guard let lhs = $0.cycleMembership,
                      let rhs = $1.cycleMembership
                else {
                    return $0.id.description < $1.id.description
                }
                return lhs.occurrenceDate < rhs.occurrenceDate
            }
        guard memberChains.isEmpty == false else {
            throw NoonmarkError.invalidInput(
                "task cycle has no template occurrence"
            )
        }
        let membersByDate = Dictionary(
            uniqueKeysWithValues: memberChains.compactMap {
                chain -> (LocalDate, TaskChain)? in
                guard let membership = chain.cycleMembership else {
                    return nil
                }
                return (membership.occurrenceDate, chain)
            }
        )
        let revisedDateSet = Set(revisedDates)
        let cancellations = memberChains.compactMap {
            chain -> (LocalDate, DayTrace)? in
            guard let membership = chain.cycleMembership,
                  membership.occurrenceDate >= editableWindowStart,
                  revisedDateSet.contains(membership.occurrenceDate)
                  == false,
                  let trace = traces.values.first(where: {
                      $0.chainID == chain.id
                          && $0.status == .pending
                          && $0.date == membership.occurrenceDate
                          && $0.date >= editableWindowStart
                  })
            else {
                return nil
            }
            return (membership.occurrenceDate, trace)
        }
        let reactivations = revisedDates.compactMap {
            occurrenceDate -> (LocalDate, TaskChain)? in
            guard occurrenceDate >= editableWindowStart,
                  let chain = membersByDate[occurrenceDate],
                  series.latestOccurrenceCancellationReason(
                      on: occurrenceDate
                  ) == .planRevision,
                  traces.values.contains(where: {
                      $0.chainID == chain.id
                          && $0.status == .pending
                  }) == false
            else {
                return nil
            }
            return (occurrenceDate, chain)
        }
        try ensureTaskCycleDatesUnlocked(
            cancellations.map(\.1.date) + reactivations.map(\.0)
        )
        let preparedTraces = try cancellations.map {
            try prepareTaskCycleTraceCancellation(
                $0.1,
                today: today,
                now: now
            )
        }
        try series.appendPlanRevision(
            effectiveFrom: effectiveFrom,
            endConditionAnchorDate: planStartDate,
            updatesStartDate: revisesStartDate,
            schedule: schedule,
            endCondition: endCondition,
            now: now
        )
        for cancellation in cancellations {
            try series.appendCancellation(
                .occurrence(cancellation.0),
                reason: .planRevision,
                now: now
            )
        }
        let newDates = revisedDates.filter {
            membersByDate[$0] == nil
        }

        storeTaskCycleSeries(series)
        try storePreparedTaskCycleTraceCancellations(
            preparedTraces,
            now: now
        )
        var classificationCommitBoundaries: [NoonmarkSnapshot] = []
        for occurrenceDate in newDates {
            let chainID = installTaskCycleOccurrence(
                normalizedTitle: series.title,
                descriptionText: series.descriptionText,
                plannedSubtasks: series.plannedSubtasks,
                membership: TaskCycleMembership(
                    seriesID: series.id,
                    occurrenceDate: occurrenceDate
                ),
                now: now
            )
            try applyTaskCycleTemplateClassification(
                to: chainID,
                series: series,
                now: now
            )
            if series.categoryID != nil || series.labelIDs.isEmpty == false {
                classificationCommitBoundaries.append(snapshot())
            }
        }
        for reactivation in reactivations {
            _ = try reactivateTaskCycleOccurrence(
                chainID: reactivation.1.id,
                series: series,
                occurrenceDate: reactivation.0,
                now: now
            )
        }
        if classificationCommitBoundaries.isEmpty == false {
            classificationCommitBoundaries[
                classificationCommitBoundaries.index(
                    before: classificationCommitBoundaries.endIndex
                )
            ] = snapshot()
        }
        return TaskCycleRevisionOutcome(
            effectiveFrom: effectiveFrom,
            cancelledOccurrenceCount: cancellations.count,
            createdOccurrenceCount: newDates.count
                + reactivations.count,
            classificationCommitBoundaries:
            classificationCommitBoundaries
        )
    }

    @discardableResult
    func reconcileTaskCycleSeries(
        today: LocalDate,
        now: Date = Date()
    ) throws -> TaskCycleReconciliationOutcome {
        let candidate = NoonmarkEngine(copying: self)
        let outcome = try candidate.reconcileTaskCycleSeriesInPlace(
            today: today,
            now: now
        )
        adoptState(from: candidate)
        return outcome
    }

    func skipTaskCycleOccurrence(
        seriesID: TaskCycleSeriesID,
        occurrenceDate: LocalDate,
        today: LocalDate,
        now: Date = Date()
    ) throws {
        var series = try taskCycleSeriesValue(seriesID)
        guard let chain = chains.values.first(where: {
            $0.cycleMembership?.seriesID == seriesID
                && $0.cycleMembership?.occurrenceDate == occurrenceDate
        }),
            let trace = traces.values.first(where: {
                $0.chainID == chain.id
                    && $0.status == .pending
                    && $0.date > today
            }),
            occurrenceDate > today
        else {
            throw NoonmarkError.invalidTransition(
                "only a future pending task cycle occurrence can be skipped"
            )
        }
        try ensureTaskCycleDatesUnlocked([trace.date])
        try series.appendCancellation(
            .occurrence(occurrenceDate),
            reason: .userSkipped,
            now: now
        )
        let preparedTrace = try prepareTaskCycleTraceCancellation(
            trace,
            today: today,
            now: now
        )
        storeTaskCycleSeries(series)
        try storePreparedTaskCycleTraceCancellations(
            [preparedTrace],
            now: now
        )
    }

    @discardableResult
    func stopTaskCycleSeries(
        seriesID: TaskCycleSeriesID,
        today: LocalDate,
        now: Date = Date()
    ) throws -> Int {
        var series = try taskCycleSeriesValue(seriesID)
        guard today < series.endDate else { return 0 }
        if let stoppedAfterDate = series.stoppedAfterDate,
           stoppedAfterDate <= today
        {
            return 0
        }
        let futurePendingOccurrences = chains.values.compactMap {
            chain -> (occurrenceDate: LocalDate, trace: DayTrace)? in
            guard chain.cycleMembership?.seriesID == seriesID,
                  let membership = chain.cycleMembership,
                  membership.occurrenceDate > today
            else {
                return nil
            }
            guard let trace = traces.values.first(where: {
                $0.chainID == chain.id
                    && $0.status == .pending
                    && $0.date > today
            }) else {
                return nil
            }
            return (membership.occurrenceDate, trace)
        }
        .sorted {
            $0.occurrenceDate < $1.occurrenceDate
        }
        try ensureTaskCycleDatesUnlocked(
            futurePendingOccurrences.map(\.trace.date)
        )
        for occurrence in futurePendingOccurrences {
            try series.appendCancellation(
                .occurrence(occurrence.occurrenceDate),
                reason: .stoppedEarly,
                now: now
            )
        }
        try series.appendCancellation(
            .followingDates(after: today),
            reason: .stoppedEarly,
            now: now
        )
        let preparedTraces = try futurePendingOccurrences.map {
            try prepareTaskCycleTraceCancellation(
                $0.trace,
                today: today,
                now: now
            )
        }
        storeTaskCycleSeries(series)
        try storePreparedTaskCycleTraceCancellations(
            preparedTraces,
            now: now
        )
        return futurePendingOccurrences.count
    }

    private func taskCycleSeriesValue(
        _ id: TaskCycleSeriesID
    ) throws -> TaskCycleSeries {
        guard let series = taskCycleSeries[id] else {
            throw NoonmarkError.notFound("task cycle series")
        }
        return series
    }

    private func updateTaskCycleTemplateContentInPlace(
        seriesID: TaskCycleSeriesID,
        title: String,
        descriptionText: String?,
        plannedSubtasks: [PlannedSubtask],
        today: LocalDate,
        now: Date
    ) throws {
        var series = try taskCycleSeriesValue(seriesID)
        try series.requirePlanEditing(on: today, operation: "edited")
        let normalizedTitle = try normalizedTaskCycleTitle(title)
        let normalizedSubtasks = plannedSubtasks
            .sorted { $0.position < $1.position }
            .enumerated()
            .map { index, subtask in
                var normalized = subtask
                normalized.title = subtask.title.trimmingCharacters(
                    in: .whitespacesAndNewlines
                )
                normalized.position = index + 1
                return normalized
            }
        guard normalizedSubtasks.allSatisfy({
            $0.title.isEmpty == false
        }) else {
            throw NoonmarkError.invalidInput(
                "planned subtask title cannot be empty"
            )
        }
        let memberChainIDs = taskCycleEditableMemberChainIDs(
            seriesID: seriesID,
            today: today
        )
        for chainID in memberChainIDs {
            try replaceTaskCycleMemberTemplate(
                chainID: chainID,
                title: normalizedTitle,
                descriptionText: descriptionText,
                plannedSubtasks: normalizedSubtasks,
                today: today,
                now: now
            )
        }
        try series.updateTemplateContent(
            title: normalizedTitle,
            descriptionText: descriptionText,
            plannedSubtasks: normalizedSubtasks,
            now: now
        )
        storeTaskCycleSeries(series)
    }

    private func replaceTaskCycleMemberTemplate(
        chainID: TaskChainID,
        title: String,
        descriptionText: String?,
        plannedSubtasks: [PlannedSubtask],
        today: LocalDate,
        now: Date
    ) throws {
        let chainTraces = traces.values.filter {
            $0.chainID == chainID && $0.formsDayHistory
        }
        let pendingTraces = chainTraces.filter {
            $0.status == .pending && $0.date >= today
        }
        guard pendingTraces.isEmpty == false else {
            throw NoonmarkError.invalidTransition(
                "task cycle occurrence is no longer editable"
            )
        }
        try ensureTaskCycleDatesUnlocked(pendingTraces.map(\.date))
        var current = try currentTaskCycleDefinition(
            for: chainID
        )
        let clonedSubtasks = plannedSubtasks.map {
            PlannedSubtask(
                lineageID: $0.lineageID,
                title: $0.title,
                difficulty: $0.difficulty,
                position: $0.position,
                now: now
            )
        }
        let plannedSubtasksChanged =
            current.plannedSubtasks.count != plannedSubtasks.count
                || zip(
                    current.plannedSubtasks.sorted {
                        $0.position < $1.position
                    },
                    plannedSubtasks.sorted {
                        $0.position < $1.position
                    }
                ).contains {
                    $0.lineageID != $1.lineageID
                        || $0.title != $1.title
                        || $0.difficulty != $1.difficulty
                        || $0.position != $1.position
                }
        let hasHistoricalReference = chainTraces.contains {
            $0.definitionID == current.id && $0.status != .pending
        }
        if hasHistoricalReference == false {
            try current.markContentModified(at: now)
            current.title = title
            current.descriptionText = descriptionText
            current.plannedSubtasks = clonedSubtasks
            storeTaskCycleDefinition(current)
        } else {
            let nextSequence = (definitions.values
                .filter { $0.chainID == chainID }
                .map(\.sequence)
                .max() ?? current.sequence) + 1
            let replacement = TaskDefinition(
                chainID: chainID,
                sequence: nextSequence,
                title: title,
                descriptionText: descriptionText,
                plannedSubtasks: clonedSubtasks,
                now: now
            )
            try current.markContentModified(at: now)
            current.supersededAt = now
            current.supersededByDefinitionID = replacement.id
            storeTaskCycleDefinition(current)
            storeTaskCycleDefinition(replacement)
            for pendingTrace in pendingTraces {
                var trace = pendingTrace
                try trace.markContentModified(at: now)
                trace.definitionID = replacement.id
                trace.descriptionText = descriptionText
                storeTaskCycleTrace(trace)
            }
        }
        for pendingTrace in pendingTraces {
            if pendingTrace.descriptionText != descriptionText {
                var trace = traces[pendingTrace.id] ?? pendingTrace
                try trace.markContentModified(at: now)
                trace.descriptionText = descriptionText
                storeTaskCycleTrace(trace)
            }
            if plannedSubtasksChanged, pendingTrace.date >= today {
                try replaceTaskCycleTraceSubtasks(
                    traceID: pendingTrace.id,
                    plannedSubtasks: plannedSubtasks,
                    now: now
                )
            }
        }
        try touchTaskCycleChain(chainID, now: now)
    }

    private func applyTaskCycleTemplateClassification(
        to chainID: TaskChainID,
        series: TaskCycleSeries,
        now: Date
    ) throws {
        guard series.categoryID != nil || series.labelIDs.isEmpty == false
        else {
            return
        }
        let plan = try prepareClassification(
            .setCurrent(
                TaskClassificationDraft(
                    chainID: chainID,
                    category: series.categoryID.map(
                        TaskCategoryChoice.existing
                    ),
                    labels: series.labelIDs
                        .sorted { $0.description < $1.description }
                        .map(TaskLabelChoice.existing)
                )
            ),
            source: .deterministicDomainAction(
                reason: "recurring task template classification inherited"
            ),
            interactionID: UUID(),
            now: now
        )
        try commitDeterministicDomainClassification(plan, now: now)
    }

    private func reconcileTaskCycleSeriesInPlace(
        today: LocalDate,
        now: Date
    ) throws -> TaskCycleReconciliationOutcome {
        var stoppedSeriesCount = 0
        var cancelledOccurrenceCount = 0
        for seriesID in taskCycleSeries.keys.sorted(by: {
            $0.description < $1.description
        }) {
            var series = try taskCycleSeriesValue(seriesID)
            guard series.stoppedAfterDate == nil,
                  case let .completionTarget(targetCount, _) =
                  series.endCondition
            else {
                continue
            }
            let memberChainIDs = Set(chains.values.compactMap {
                $0.cycleMembership?.seriesID == seriesID ? $0.id : nil
            })
            let lockedCompletions = traces.values.filter { trace in
                memberChainIDs.contains(trace.chainID)
                    && trace.status == .completed
                    && trace.date < today
                    && days[trace.date]?.lockedAt != nil
            }
            let completedChainIDs = Set(
                lockedCompletions.map(\.chainID)
            )
            guard completedChainIDs.count >= targetCount,
                  let cutoff = lockedCompletions.map(\.date).max(),
                  cutoff < series.endDate
            else {
                continue
            }
            let futurePending = chains.values.compactMap {
                chain -> (LocalDate, DayTrace)? in
                guard let membership = chain.cycleMembership,
                      membership.seriesID == seriesID,
                      membership.occurrenceDate > cutoff,
                      let trace = traces.values.first(where: {
                          $0.chainID == chain.id
                              && $0.status == .pending
                              && $0.date >= today
                      })
                else {
                    return nil
                }
                return (membership.occurrenceDate, trace)
            }
            .sorted { $0.0 < $1.0 }
            try ensureTaskCycleDatesUnlocked(
                futurePending.map(\.1.date)
            )
            let preparedTraces = try futurePending.map {
                try prepareTaskCycleTraceCancellation(
                    $0.1,
                    today: today,
                    now: now
                )
            }
            for occurrence in futurePending {
                try series.appendCancellation(
                    .occurrence(occurrence.0),
                    reason: .completionTargetReached,
                    now: now
                )
            }
            try series.appendCancellation(
                .followingDates(after: cutoff),
                reason: .completionTargetReached,
                now: now
            )
            storeTaskCycleSeries(series)
            try storePreparedTaskCycleTraceCancellations(
                preparedTraces,
                now: now
            )
            stoppedSeriesCount += 1
            cancelledOccurrenceCount += preparedTraces.count
        }
        return TaskCycleReconciliationOutcome(
            stoppedSeriesCount: stoppedSeriesCount,
            cancelledOccurrenceCount: cancelledOccurrenceCount
        )
    }

    private func latestTaskCycleTrace(
        _ candidates: [DayTrace]
    ) -> DayTrace? {
        candidates.max {
            if $0.createdAt != $1.createdAt {
                return $0.createdAt < $1.createdAt
            }
            return $0.id.description < $1.id.description
        }
    }

    private func taskCycleTrackState(
        trace: DayTrace,
        date: LocalDate,
        today: LocalDate,
        cancelledState: TaskCycleTrackDayState
    ) -> TaskCycleTrackDayState {
        switch trace.status {
        case .pending:
            if date < today {
                return .pendingPast
            }
            if date == today {
                return .pendingToday
            }
            return .planned
        case .cancelledDraft:
            return cancelledState
        case .completed:
            return .completed
        case .unfinished:
            return .unfinished
        case .deferred:
            return .deferred
        case .changed:
            return .changed
        case .returnedToPool:
            return .returnedToPool
        case .abandoned:
            return .abandoned
        }
    }

    private func taskCycleTraceTarget(
        _ trace: DayTrace?,
        today: LocalDate,
        cancelledState: TaskCycleTrackDayState = .returnedToPool
    ) -> TaskCycleTraceTarget? {
        guard let trace else { return nil }
        return TaskCycleTraceTarget(
            traceID: trace.id,
            date: trace.date,
            state: taskCycleTrackState(
                trace: trace,
                date: trace.date,
                today: today,
                cancelledState: cancelledState
            )
        )
    }
}

enum TaskCycleCivilCalendar {
    static let maximumMaterializedDayCount = 366

    private static var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }

    static func weekday(for date: LocalDate) -> Int? {
        guard let instant = instant(for: date) else { return nil }
        return calendar.component(.weekday, from: instant)
    }

    static func dayDistance(
        from startDate: LocalDate,
        to endDate: LocalDate
    ) -> Int? {
        guard let start = instant(for: startDate),
              let end = instant(for: endDate)
        else {
            return nil
        }
        return calendar.dateComponents(
            [.day],
            from: start,
            to: end
        ).day
    }

    static func offset(
        _ date: LocalDate,
        by dayCount: Int
    ) -> LocalDate? {
        guard let instant = instant(for: date),
              let target = calendar.date(
                  byAdding: .day,
                  value: dayCount,
                  to: instant
              )
        else {
            return nil
        }
        let components = calendar.dateComponents(
            [.year, .month, .day],
            from: target
        )
        guard let year = components.year,
              let month = components.month,
              let day = components.day
        else {
            return nil
        }
        return LocalDate(year: year, month: month, day: day)
    }

    static func materializableDates(
        from startDate: LocalDate,
        through endDate: LocalDate
    ) throws -> [LocalDate] {
        guard startDate <= endDate,
              let start = instant(for: startDate),
              let end = instant(for: endDate)
        else {
            throw NoonmarkError.invalidInput("invalid task cycle date range")
        }
        guard let distance = calendar.dateComponents(
            [.day],
            from: start,
            to: end
        ).day,
            distance >= 0,
            distance < maximumMaterializedDayCount
        else {
            throw NoonmarkError.invalidInput(
                "task cycle cannot materialize more than 366 calendar days"
            )
        }
        var dates: [LocalDate] = []
        dates.reserveCapacity(distance + 1)
        var cursor = start
        while cursor <= end {
            let components = calendar.dateComponents(
                [.year, .month, .day],
                from: cursor
            )
            guard let year = components.year,
                  let month = components.month,
                  let day = components.day
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle date could not be resolved"
                )
            }
            dates.append(LocalDate(year: year, month: month, day: day))
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor)
            else {
                throw NoonmarkError.invalidInput(
                    "task cycle date range could not advance"
                )
            }
            cursor = next
        }
        return dates
    }

    private static func instant(for date: LocalDate) -> Date? {
        calendar.date(
            from: DateComponents(
                year: date.year,
                month: date.month,
                day: date.day
            )
        )
    }
}
