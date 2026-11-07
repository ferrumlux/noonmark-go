import Foundation

public extension NoonmarkEngine {
    /// Produces a local mutation timestamp that follows every persisted fact.
    ///
    /// Natural-day time remains the source of wall-clock intent. When that
    /// clock is equal to or behind an existing fact, Noonmark advances by one
    /// representable instant so content and sync boundaries remain ordered.
    func nextMutationDate(reference: Date) throws -> Date {
        let referenceSeconds = reference.timeIntervalSinceReferenceDate
        guard referenceSeconds.isFinite else {
            throw NoonmarkError.invalidInput(
                "mutation clock reference must be finite"
            )
        }

        let persistedSeconds = persistedMutationDates().map(
            \.timeIntervalSinceReferenceDate
        )
        guard persistedSeconds.allSatisfy(\.isFinite) else {
            throw NoonmarkError.invalidInput(
                "persisted mutation clock contains a non-finite timestamp"
            )
        }
        guard let frontierSeconds = persistedSeconds.max() else {
            return reference
        }
        guard frontierSeconds >= referenceSeconds else { return reference }
        let nextSeconds = frontierSeconds.nextUp
        guard nextSeconds.isFinite else {
            throw NoonmarkError.invalidInput(
                "persisted mutation clock frontier cannot advance"
            )
        }
        return Date(timeIntervalSinceReferenceDate: nextSeconds)
    }

    private func persistedMutationDates() -> [Date] {
        dayMutationDates()
            + taskCycleSeriesMutationDates()
            + chainMutationDates()
            + definitionMutationDates()
            + traceMutationDates()
            + subtaskMutationDates()
            + ideaMutationDates()
            + classificationMutationDates()
            + [preferences.themeLanguageUpdatedAt]
    }

    private func taskCycleSeriesMutationDates() -> [Date] {
        taskCycleSeries.values.flatMap { series in
            [series.createdAt, series.updatedAt]
                + series.cancellationFacts.map(\.recordedAt)
        }
    }

    private func dayMutationDates() -> [Date] {
        days.values.flatMap { day in
            [day.createdAt, day.updatedAt] + [day.lockedAt].compactMap { $0 }
        }
    }

    private func chainMutationDates() -> [Date] {
        chains.values.flatMap { chain in
            [chain.createdAt, chain.updatedAt]
                + noteMutationDates(chain.noteEntries)
        }
    }

    private func definitionMutationDates() -> [Date] {
        definitions.values.flatMap { definition in
            [definition.createdAt, definition.contentUpdatedAt]
                + [definition.supersededAt].compactMap { $0 }
                + definition.plannedSubtasks.map(\.createdAt)
        }
    }

    private func traceMutationDates() -> [Date] {
        traces.values.flatMap { trace in
            [trace.createdAt, trace.contentUpdatedAt]
                + [trace.completedAt, trace.settledAt].compactMap { $0 }
                + noteMutationDates(trace.noteEntries)
        }
    }

    private func subtaskMutationDates() -> [Date] {
        subtasks.values.flatMap { subtask in
            [subtask.createdAt, subtask.updatedAt]
                + [subtask.completedAt, subtask.settledAt].compactMap { $0 }
        }
    }

    private func ideaMutationDates() -> [Date] {
        ideas.values.flatMap { idea in
            [idea.createdAt, idea.updatedAt]
                + [idea.deletedAt, idea.pinnedAt].compactMap { $0 }
        }
    }

    private func noteMutationDates(_ entries: [TaskNoteEntry]) -> [Date] {
        entries.flatMap { entry in
            [entry.createdAt, entry.updatedAt] + [entry.deletedAt].compactMap {
                $0
            }
        }
    }

    private func classificationMutationDates() -> [Date] {
        classificationIdentityMutationDates()
            + currentClassificationMutationDates()
            + classificationLifecycleMutationDates()
            + classificationSnapshotMutationDates()
            + classificationState.changeRecords.map(\.committedAt)
    }

    private func classificationIdentityMutationDates() -> [Date] {
        let categoryDates = classificationState.categories.values.flatMap {
            classificationIdentityDates(
                createdAt: $0.createdAt,
                updatedAt: $0.updatedAt,
                nameVersions: $0.nameVersions
            )
        }
        let labelDates = classificationState.labels.values.flatMap {
            classificationIdentityDates(
                createdAt: $0.createdAt,
                updatedAt: $0.updatedAt,
                nameVersions: $0.nameVersions
            )
        }
        return categoryDates + labelDates
    }

    private func classificationIdentityDates(
        createdAt: Date,
        updatedAt: Date,
        nameVersions: [ClassificationNameVersion]
    ) -> [Date] {
        [createdAt, updatedAt] + nameVersions.flatMap {
            [$0.validFrom] + [$0.validUntil].compactMap { $0 }
        }
    }

    private func currentClassificationMutationDates() -> [Date] {
        classificationState.currentByChainID.values.flatMap { current in
            let categoryDates = current.category.map {
                [$0.createdAt, $0.updatedAt]
            } ?? []
            let labelDates = current.labels.flatMap {
                [$0.createdAt, $0.updatedAt]
            }
            return categoryDates + labelDates
        }
    }

    private func classificationLifecycleMutationDates() -> [Date] {
        classificationState.relationHistory.flatMap {
            [$0.createdAt, $0.removedAt]
        }
            + classificationState.categoryMerges.values.map(\.mergedAt)
            + classificationState.labelMerges.values.map(\.mergedAt)
            + classificationState.categoryDeletionTombstones.values
            .map(\.deletedAt)
            + classificationState.labelDeletionTombstones.values
            .map(\.deletedAt)
    }

    private func classificationSnapshotMutationDates() -> [Date] {
        classificationState.snapshotsByTraceID.values.map(\.capturedAt)
            + classificationState.snapshotEventsByTraceID.values.flatMap {
                $0.map(\.capturedAt)
            }
    }
}
