import Foundation

extension TaskClassificationState {
    public func validateIntegrity() throws {
        try validateIntegrity(codingPath: [])
    }

    func validateIntegrity(codingPath: [any CodingKey]) throws {
        try ClassificationStateValidator(
            state: self,
            codingPath: codingPath
        ).validate()
    }
}

private struct ClassificationStateValidator {
    private let state: TaskClassificationState
    private let codingPath: [any CodingKey]

    init(state: TaskClassificationState, codingPath: [any CodingKey]) {
        self.state = state
        self.codingPath = codingPath
    }

    func validate() throws {
        try validateRevisionBoundary()
        try validateCanonicalAuditOrder()
        try validateIdentityDictionaryKeys()
        try validateSnapshotStreams()
        try validateCanonicalNameOwnership()
        try validateClassificationSources()
        try validateCurrentRelations()
        try validateRelationHistory()
        try validateChangeRecords()
        try validateReceipts()
        try validateManagementFactIDs()
        try validateCategoryManagementFacts()
        try validateLabelManagementFacts()
    }

    func validateRevisionBoundary() throws {
        let eventCount = state.snapshotEventsByTraceID.values.reduce(0) {
            $0 + $1.count
        }
        let advancingRecordCount = state.changeRecords.count(where: \.advancesStateRevision)
        guard let expectedRevision = UInt64(exactly: advancingRecordCount + eventCount),
              state.revision == expectedRevision
        else {
            throw invalid("classification state revision does not equal its audit fact count")
        }
    }

    func validateCanonicalAuditOrder() throws {
        do {
            guard try ClassificationAuditCanonicalOrder.changeRecords(state.changeRecords)
                == state.changeRecords
            else {
                throw invalid("classification change records are not in canonical order")
            }
            guard try ClassificationAuditCanonicalOrder.relationHistory(state.relationHistory)
                == state.relationHistory
            else {
                throw invalid("classification relation history is not in canonical order")
            }
        } catch let error as ClassificationAuditCanonicalOrderError {
            throw invalid("classification audit ordering facts are invalid: \(error)")
        }
    }

    private func invalid(_ description: String) -> DecodingError {
        .dataCorrupted(
            .init(codingPath: codingPath, debugDescription: description)
        )
    }
}

private extension ClassificationStateValidator {
    func validateIdentityDictionaryKeys() throws {
        for (key, category) in state.categories {
            guard key == category.id else {
                throw invalid("task category dictionary key does not match identity")
            }
        }
        for (key, label) in state.labels {
            guard key == label.id else {
                throw invalid("task label dictionary key does not match identity")
            }
        }
    }

    func validateCanonicalNameOwnership() throws {
        try validateCategoryCanonicalNameOwnership()
        try validateLabelCanonicalNameOwnership()
    }

    func validateCategoryCanonicalNameOwnership() throws {
        var owners: [ClassificationCanonicalPair: TaskCategoryID] = [:]
        for category in state.categories.values {
            try claimCategoryPair(
                ClassificationCanonicalPair(category),
                for: category.id,
                owners: &owners
            )
            for version in category.nameVersions {
                try claimCategoryPair(
                    ClassificationCanonicalPair(version),
                    for: category.id,
                    owners: &owners
                )
            }
        }
        for events in state.snapshotEventsByTraceID.values {
            for event in events {
                if let category = event.category {
                    guard state.categories[category.id] != nil else {
                        throw invalid("historical task category references a missing identity")
                    }
                    try claimCategoryPair(
                        ClassificationCanonicalPair(historicalName: category.name),
                        for: category.id,
                        owners: &owners
                    )
                }
            }
        }
    }

    func claimCategoryPair(
        _ pair: ClassificationCanonicalPair,
        for id: TaskCategoryID,
        owners: inout [ClassificationCanonicalPair: TaskCategoryID]
    ) throws {
        if let owner = owners[pair], owner != id {
            throw invalid("task category canonical name is owned by multiple identities")
        }
        owners[pair] = id
    }

    func validateLabelCanonicalNameOwnership() throws {
        var owners: [ClassificationCanonicalPair: TaskLabelID] = [:]
        for label in state.labels.values {
            try claimLabelPair(
                ClassificationCanonicalPair(label),
                for: label.id,
                owners: &owners
            )
            for version in label.nameVersions {
                try claimLabelPair(
                    ClassificationCanonicalPair(version),
                    for: label.id,
                    owners: &owners
                )
            }
        }
        for events in state.snapshotEventsByTraceID.values {
            for event in events {
                for label in event.labels {
                    guard state.labels[label.id] != nil else {
                        throw invalid("historical task label references a missing identity")
                    }
                    try claimLabelPair(
                        ClassificationCanonicalPair(historicalName: label.name),
                        for: label.id,
                        owners: &owners
                    )
                }
            }
        }
    }

    func claimLabelPair(
        _ pair: ClassificationCanonicalPair,
        for id: TaskLabelID,
        owners: inout [ClassificationCanonicalPair: TaskLabelID]
    ) throws {
        if let owner = owners[pair], owner != id {
            throw invalid("task label canonical name is owned by multiple identities")
        }
        owners[pair] = id
    }
}

private struct ClassificationCanonicalPair: Hashable {
    let key: String
    let version: String

    init(_ category: TaskCategory) {
        key = category.canonicalKey
        version = category.canonicalKeyVersion
    }

    init(_ label: TaskLabel) {
        key = label.canonicalKey
        version = label.canonicalKeyVersion
    }

    init(_ nameVersion: ClassificationNameVersion) {
        key = nameVersion.canonicalKey
        version = nameVersion.canonicalKeyVersion
    }

    init(historicalName: String) {
        key = ClassificationNameCanonicalizer.canonicalKey(historicalName)
        version = ClassificationNameCanonicalizer.algorithmVersion
    }
}

private extension ClassificationStateValidator {
    func validateClassificationSources() throws {
        for current in state.currentByChainID.values {
            if let category = current.category {
                try validateClassificationSource(category.source)
            }
            for label in current.labels {
                try validateClassificationSource(label.source)
            }
        }
        for history in state.relationHistory {
            try validateClassificationSource(history.originSource)
            try validateClassificationSource(history.removedBySource)
        }
        for record in state.changeRecords {
            try validateClassificationSource(record.source)
        }
    }

    func validateClassificationSource(_ source: ClassificationSource) throws {
        guard case let .automaticAI(_, generation) = source,
              generation <= 0
        else { return }
        throw invalid("automatic classification source generation must be positive")
    }

    func validateCurrentRelations() throws {
        for (chainID, current) in state.currentByChainID {
            if let category = current.category {
                try validateCurrentCategoryRelation(category, chainID: chainID)
            }
            guard Set(current.labels.map(\.labelID)).count == current.labels.count else {
                throw invalid("current task label relations are not unique")
            }
            for label in current.labels {
                try validateCurrentLabelRelation(label, chainID: chainID)
            }
        }
    }

    func validateCurrentCategoryRelation(
        _ relation: TaskCategoryRelation,
        chainID: TaskChainID
    ) throws {
        guard let category = state.categories[relation.categoryID],
              category.lifecycle != .merged
        else {
            throw invalid("current task category relation is missing or merged")
        }
        try validateCurrentRelationTimeline(
            createdAt: relation.createdAt,
            updatedAt: relation.updatedAt,
            revision: relation.revision
        )
        try validateRelationAuthority(ClassificationRelationAuthorityFact(
            kind: .category,
            chainID: chainID,
            itemID: relation.categoryID.description,
            source: relation.source,
            decisionID: relation.decisionID,
            revision: relation.revision,
            effect: .present
        ))
    }

    func validateCurrentLabelRelation(
        _ relation: TaskLabelRelation,
        chainID: TaskChainID
    ) throws {
        guard let label = state.labels[relation.labelID],
              label.lifecycle != .merged
        else {
            throw invalid("current task label relation is missing or merged")
        }
        try validateCurrentRelationTimeline(
            createdAt: relation.createdAt,
            updatedAt: relation.updatedAt,
            revision: relation.revision
        )
        try validateRelationAuthority(ClassificationRelationAuthorityFact(
            kind: .label,
            chainID: chainID,
            itemID: relation.labelID.description,
            source: relation.source,
            decisionID: relation.decisionID,
            revision: relation.revision,
            effect: .present
        ))
    }

    func validateCurrentRelationTimeline(
        createdAt: Date,
        updatedAt: Date,
        revision: UInt64
    ) throws {
        guard createdAt <= updatedAt, revision <= state.revision else {
            throw invalid("current classification relation timeline is invalid")
        }
    }

    func validateRelationHistory() throws {
        guard Set(state.relationHistory.map(\.id)).count == state.relationHistory.count else {
            throw invalid("classification relation history IDs are not unique")
        }
        for entry in state.relationHistory {
            try validateRelationHistoryTimeline(entry)
            try validateRelationHistoryIdentity(entry)
        }
    }

    func validateRelationHistoryTimeline(
        _ entry: ClassificationRelationHistoryEntry
    ) throws {
        guard entry.createdAt <= entry.removedAt,
              entry.createdRevision < entry.removedRevision,
              entry.removedRevision <= state.revision
        else {
            throw invalid("classification relation history timeline is invalid")
        }
    }

    func validateRelationHistoryIdentity(
        _ entry: ClassificationRelationHistoryEntry
    ) throws {
        guard let rawID = UUID(uuidString: entry.itemID) else {
            throw invalid("classification relation history item ID is not a UUID")
        }
        switch entry.kind {
        case .category:
            guard state.categories[TaskCategoryID(rawID)] != nil else {
                throw invalid("classification relation history references a missing task category")
            }
        case .label:
            guard state.labels[TaskLabelID(rawID)] != nil else {
                throw invalid("classification relation history references a missing task label")
            }
        }
        try validateRelationAuthority(ClassificationRelationAuthorityFact(
            kind: entry.kind,
            chainID: entry.chainID,
            itemID: entry.itemID,
            source: entry.originSource,
            decisionID: entry.originDecisionID,
            revision: entry.createdRevision,
            effect: .present
        ))
        try validateRelationAuthority(ClassificationRelationAuthorityFact(
            kind: entry.kind,
            chainID: entry.chainID,
            itemID: entry.itemID,
            source: entry.removedBySource,
            decisionID: entry.removedByDecisionID,
            revision: entry.removedRevision,
            effect: .removed
        ))
    }

    func validateRelationAuthority(
        _ fact: ClassificationRelationAuthorityFact
    ) throws {
        guard fact.source.requiresExplicitUserDecision else { return }
        guard let decisionID = fact.decisionID,
              state.changeRecords.contains(where: { record in
                  record.revision == fact.revision
                      && record.source == fact.source
                      && record.decisionID == decisionID
                      && record.changes.contains(where: {
                          $0.accountsForRelation(
                              kind: fact.kind,
                              chainID: fact.chainID,
                              itemID: fact.itemID,
                              effect: fact.effect
                          )
                      })
              })
        else {
            throw invalid("user-authorized classification relation is missing its causal audit")
        }
    }
}

private enum ClassificationRelationAuditEffect {
    case present
    case removed
}

private struct ClassificationRelationAuthorityFact {
    let kind: ClassificationItemKind
    let chainID: TaskChainID
    let itemID: String
    let source: ClassificationSource
    let decisionID: UUID?
    let revision: UInt64
    let effect: ClassificationRelationAuditEffect
}

private extension ClassificationStateValidator {
    func validateSnapshotStreams() throws {
        guard Set(state.snapshotsByTraceID.keys) == Set(state.snapshotEventsByTraceID.keys) else {
            throw invalid("classification snapshot indexes have different trace IDs")
        }
        var eventIDs: Set<UUID> = []
        for (traceID, events) in state.snapshotEventsByTraceID {
            try validateSnapshotStream(traceID: traceID, events: events, eventIDs: &eventIDs)
        }
    }

    func validateSnapshotStream(
        traceID: DayTraceID,
        events: [TraceClassificationSnapshot],
        eventIDs: inout Set<UUID>
    ) throws {
        guard let latest = events.last,
              state.snapshotsByTraceID[traceID] == latest
        else {
            throw invalid("classification snapshot stream is empty or its latest index is stale")
        }
        guard zip(events, events.dropFirst()).allSatisfy({ pair in
            pair.0.revision < pair.1.revision
        }) else {
            throw invalid("classification snapshot event revisions are not strictly increasing")
        }
        for event in events {
            try validateSnapshotEvent(event, key: traceID, eventIDs: &eventIDs)
        }
    }

    func validateSnapshotEvent(
        _ event: TraceClassificationSnapshot,
        key: DayTraceID,
        eventIDs: inout Set<UUID>
    ) throws {
        guard event.traceID == key else {
            throw invalid("classification snapshot event key does not match its trace ID")
        }
        guard eventIDs.insert(event.id).inserted else {
            throw invalid("classification snapshot event IDs are not unique")
        }
        guard event.revision > 0, event.revision <= state.revision else {
            throw invalid("classification snapshot event revision is outside the state boundary")
        }
        guard Set(event.labels.map(\.id)).count == event.labels.count else {
            throw invalid("historical task label values are not unique within an event")
        }
    }
}

private extension ClassificationStateValidator {
    var changeRecordsByID: [UUID: ClassificationChangeRecord] {
        state.changeRecords.reduce(into: [:]) { records, record in
            records[record.id] = record
        }
    }

    func validateChangeRecords() throws {
        guard Set(state.changeRecords.map(\.id)).count == state.changeRecords.count else {
            throw invalid("classification change record IDs are not unique")
        }
        guard Set(state.changeRecords.map(\.planID)).count == state.changeRecords.count else {
            throw invalid("classification change record plan IDs are not unique")
        }
        guard Set(state.changeRecords.map(\.interactionID)).count == state.changeRecords.count else {
            throw invalid("classification change record interaction IDs are not unique")
        }
        guard state.changeRecords.allSatisfy({ $0.revision <= state.revision }) else {
            throw invalid("classification change record revision exceeds state revision")
        }
        guard state.changeRecords.allSatisfy({ record in
            let digest = record.planDigest
            return digest.utf8.count == 64 && digest.utf8.allSatisfy {
                (48 ... 57).contains($0) || (97 ... 102).contains($0)
            }
        }) else {
            throw invalid("classification change record plan digest is malformed")
        }
        guard state.changeRecords.allSatisfy({ $0.hasValidIntegrityDigest() }) else {
            throw invalid("classification change record integrity digest does not match its contents")
        }
        for record in state.changeRecords where record.source.requiresExplicitUserDecision {
            guard record.decisionID != nil else {
                throw invalid("user-authorized classification change is missing a decision")
            }
        }
    }

    func validateReceipts() throws {
        let recordsByInteractionID = Dictionary(
            uniqueKeysWithValues: state.changeRecords.map { ($0.interactionID, $0) }
        )
        for (interactionID, receipt) in state.committedReceiptsByInteractionID {
            guard receipt.revision <= state.revision else {
                throw invalid("classification receipt revision exceeds state revision")
            }
            guard let record = recordsByInteractionID[interactionID],
                  receipt.planID == record.planID,
                  receipt.changeRecordID == record.id,
                  receipt.revision == record.revision,
                  receipt.decisionID == record.decisionID,
                  receipt.notices == record.notices,
                  receipt.changeRecordIntegrityDigest == record.integrityDigest
            else {
                throw invalid("classification receipt does not match its change record")
            }
        }
        for record in state.changeRecords where record.source.requiresExplicitUserDecision {
            guard state.committedReceiptsByInteractionID[record.interactionID] != nil else {
                throw invalid("user-authorized classification change is missing its receipt")
            }
        }
    }

    func validateManagementFactIDs() throws {
        let ids = state.categoryMerges.values.map(\.id)
            + state.labelMerges.values.map(\.id)
            + state.categoryDeletionTombstones.values.map(\.id)
            + state.labelDeletionTombstones.values.map(\.id)
        guard Set(ids).count == ids.count else {
            throw invalid("classification management fact IDs are not unique")
        }
    }
}

private extension ClassificationSource {
    var requiresExplicitUserDecision: Bool {
        switch self {
        case .userDirect, .zhulongSuggestion:
            true
        case .automaticAI, .inherited, .deterministicDomainAction:
            false
        }
    }
}

private extension ClassificationPlanChange {
    func accountsForRelation(
        kind expectedKind: ClassificationItemKind,
        chainID expectedChainID: TaskChainID,
        itemID expectedItemID: String,
        effect: ClassificationRelationAuditEffect
    ) -> Bool {
        switch self {
        case let .setCurrent(chainID, before, after):
            guard chainID == expectedChainID else { return false }
            switch effect {
            case .present:
                return after.contains(kind: expectedKind, itemID: expectedItemID)
            case .removed:
                return before.contains(kind: expectedKind, itemID: expectedItemID)
                    && after.contains(kind: expectedKind, itemID: expectedItemID) == false
            }
        case let .merge(kind, sourceID, _, targetID, _, _, impact):
            guard kind == expectedKind,
                  impact.currentChainIDs.contains(expectedChainID)
            else { return false }
            switch effect {
            case .present:
                return targetID == expectedItemID
            case .removed:
                return sourceID == expectedItemID
            }
        case .create, .hardDelete, .rename, .lifecycle, .categoryPresentationApproval:
            return false
        }
    }
}

private extension ClassificationSelectionPreview {
    func contains(kind: ClassificationItemKind, itemID: String) -> Bool {
        switch kind {
        case .category:
            category?.kind == .category && category?.id == itemID
        case .label:
            labels.contains { $0.kind == .label && $0.id == itemID }
        }
    }
}

private extension ClassificationStateValidator {
    func validateCategoryManagementFacts() throws {
        for (sourceID, merge) in state.categoryMerges {
            try validateCategoryMerge(merge, dictionaryKey: sourceID)
        }
        try validateCategoryMergeLifecycleCoverage()
        for sourceID in state.categoryMerges.keys {
            try validateCategoryMergePath(from: sourceID)
        }
        for (itemID, tombstone) in state.categoryDeletionTombstones {
            try validateCategoryTombstone(tombstone, dictionaryKey: itemID)
        }
    }

    func validateCategoryMerge(
        _ merge: TaskCategoryMerge,
        dictionaryKey: TaskCategoryID
    ) throws {
        guard merge.sourceID == dictionaryKey, merge.sourceID != merge.targetID,
              let source = state.categories[merge.sourceID],
              let target = state.categories[merge.targetID],
              source.lifecycle == .merged
        else {
            throw invalid("task category merge is dangling or inconsistent")
        }
        guard merge.revision <= state.revision,
              source.createdAt <= merge.mergedAt,
              target.createdAt <= merge.mergedAt,
              source.updatedAt == merge.mergedAt
        else {
            throw invalid("task category merge timeline is inconsistent")
        }
        try validateCategoryMergeRecord(merge, source: source, target: target)
    }

    func validateCategoryMergeLifecycleCoverage() throws {
        for category in state.categories.values {
            guard (category.lifecycle == .merged) == (state.categoryMerges[category.id] != nil) else {
                throw invalid("task category merged lifecycle does not match merge graph")
            }
        }
    }

    func validateCategoryMergePath(from sourceID: TaskCategoryID) throws {
        var visited: Set<TaskCategoryID> = []
        var cursor = sourceID
        while let merge = state.categoryMerges[cursor] {
            guard visited.insert(cursor).inserted else {
                throw invalid("task category merge graph contains a cycle")
            }
            try validateCategoryMergeCausality(merge)
            cursor = merge.targetID
        }
        guard state.categories[cursor]?.lifecycle == .active else {
            throw invalid("task category merge graph does not end at an active target")
        }
    }

    func validateCategoryMergeCausality(_ merge: TaskCategoryMerge) throws {
        guard let successor = state.categoryMerges[merge.targetID] else { return }
        guard merge.revision < successor.revision,
              merge.mergedAt <= successor.mergedAt
        else {
            throw invalid("task category merge graph timeline is out of causal order")
        }
    }

    func validateCategoryMergeRecord(
        _ merge: TaskCategoryMerge,
        source: TaskCategory,
        target: TaskCategory
    ) throws {
        guard let record = changeRecordsByID[merge.changeRecordID],
              record.revision == merge.revision,
              record.committedAt == merge.mergedAt
        else {
            throw invalid("task category merge audit record is missing or inconsistent")
        }
        let matches = record.changes.filter {
            isMatchingCategoryMergeChange($0, merge: merge, source: source, target: target)
        }
        guard matches.count == 1 else {
            throw invalid("task category merge audit change does not match the merge fact")
        }
        try validateRemovalProvenance(
            kind: .category,
            itemID: merge.sourceID.description,
            revision: merge.revision,
            time: merge.mergedAt,
            record: record
        )
    }

    func isMatchingCategoryMergeChange(
        _ change: ClassificationPlanChange,
        merge: TaskCategoryMerge,
        source: TaskCategory,
        target: TaskCategory
    ) -> Bool {
        guard case let .merge(kind, sourceID, sourceName, targetID, targetName, sourceLifecycle, _) = change else {
            return false
        }
        return kind == .category
            && sourceID == merge.sourceID.description
            && sourceName == source.name
            && targetID == merge.targetID.description
            && targetName == classificationName(
                for: .category,
                itemID: target.id.description,
                in: target.nameVersions,
                at: merge.mergedAt,
                revision: merge.revision
            )
            && (sourceLifecycle == .active || sourceLifecycle == .archived)
    }

    func validateCategoryTombstone(
        _ tombstone: TaskCategoryDeletionTombstone,
        dictionaryKey: TaskCategoryID
    ) throws {
        guard tombstone.itemID == dictionaryKey, state.categories[dictionaryKey] == nil else {
            throw invalid("deleted task category identity is live or mismatched")
        }
        guard tombstone.revision <= state.revision else {
            throw invalid("task category deletion revision exceeds state revision")
        }
        try validateHardDeleteRecord(
            kind: .category,
            itemID: dictionaryKey.description,
            revision: tombstone.revision,
            time: tombstone.deletedAt,
            changeRecordID: tombstone.changeRecordID
        )
    }
}

private extension ClassificationStateValidator {
    func validateLabelManagementFacts() throws {
        for (sourceID, merge) in state.labelMerges {
            try validateLabelMerge(merge, dictionaryKey: sourceID)
        }
        try validateLabelMergeLifecycleCoverage()
        for sourceID in state.labelMerges.keys {
            try validateLabelMergePath(from: sourceID)
        }
        for (itemID, tombstone) in state.labelDeletionTombstones {
            try validateLabelTombstone(tombstone, dictionaryKey: itemID)
        }
    }

    func validateLabelMerge(
        _ merge: TaskLabelMerge,
        dictionaryKey: TaskLabelID
    ) throws {
        guard merge.sourceID == dictionaryKey, merge.sourceID != merge.targetID,
              let source = state.labels[merge.sourceID],
              let target = state.labels[merge.targetID],
              source.lifecycle == .merged
        else {
            throw invalid("task label merge is dangling or inconsistent")
        }
        guard merge.revision <= state.revision,
              source.createdAt <= merge.mergedAt,
              target.createdAt <= merge.mergedAt,
              source.updatedAt == merge.mergedAt
        else {
            throw invalid("task label merge timeline is inconsistent")
        }
        try validateLabelMergeRecord(merge, source: source, target: target)
    }

    func validateLabelMergeLifecycleCoverage() throws {
        for label in state.labels.values {
            guard (label.lifecycle == .merged) == (state.labelMerges[label.id] != nil) else {
                throw invalid("task label merged lifecycle does not match merge graph")
            }
        }
    }

    func validateLabelMergePath(from sourceID: TaskLabelID) throws {
        var visited: Set<TaskLabelID> = []
        var cursor = sourceID
        while let merge = state.labelMerges[cursor] {
            guard visited.insert(cursor).inserted else {
                throw invalid("task label merge graph contains a cycle")
            }
            try validateLabelMergeCausality(merge)
            cursor = merge.targetID
        }
        guard state.labels[cursor]?.lifecycle == .active else {
            throw invalid("task label merge graph does not end at an active target")
        }
    }

    func validateLabelMergeCausality(_ merge: TaskLabelMerge) throws {
        guard let successor = state.labelMerges[merge.targetID] else { return }
        guard merge.revision < successor.revision,
              merge.mergedAt <= successor.mergedAt
        else {
            throw invalid("task label merge graph timeline is out of causal order")
        }
    }

    func validateLabelMergeRecord(
        _ merge: TaskLabelMerge,
        source: TaskLabel,
        target: TaskLabel
    ) throws {
        guard let record = changeRecordsByID[merge.changeRecordID],
              record.revision == merge.revision,
              record.committedAt == merge.mergedAt
        else {
            throw invalid("task label merge audit record is missing or inconsistent")
        }
        let matches = record.changes.filter {
            isMatchingLabelMergeChange($0, merge: merge, source: source, target: target)
        }
        guard matches.count == 1 else {
            throw invalid("task label merge audit change does not match the merge fact")
        }
        try validateRemovalProvenance(
            kind: .label,
            itemID: merge.sourceID.description,
            revision: merge.revision,
            time: merge.mergedAt,
            record: record
        )
    }

    func isMatchingLabelMergeChange(
        _ change: ClassificationPlanChange,
        merge: TaskLabelMerge,
        source: TaskLabel,
        target: TaskLabel
    ) -> Bool {
        guard case let .merge(kind, sourceID, sourceName, targetID, targetName, sourceLifecycle, _) = change else {
            return false
        }
        return kind == .label
            && sourceID == merge.sourceID.description
            && sourceName == source.name
            && targetID == merge.targetID.description
            && targetName == classificationName(
                for: .label,
                itemID: target.id.description,
                in: target.nameVersions,
                at: merge.mergedAt,
                revision: merge.revision
            )
            && (sourceLifecycle == .active || sourceLifecycle == .archived)
    }

    func validateLabelTombstone(
        _ tombstone: TaskLabelDeletionTombstone,
        dictionaryKey: TaskLabelID
    ) throws {
        guard tombstone.itemID == dictionaryKey, state.labels[dictionaryKey] == nil else {
            throw invalid("deleted task label identity is live or mismatched")
        }
        guard tombstone.revision <= state.revision else {
            throw invalid("task label deletion revision exceeds state revision")
        }
        try validateHardDeleteRecord(
            kind: .label,
            itemID: dictionaryKey.description,
            revision: tombstone.revision,
            time: tombstone.deletedAt,
            changeRecordID: tombstone.changeRecordID
        )
    }
}

private extension ClassificationStateValidator {
    func validateRemovalProvenance(
        kind: ClassificationItemKind,
        itemID: String,
        revision: UInt64,
        time: Date,
        record: ClassificationChangeRecord
    ) throws {
        let removals = state.relationHistory.filter {
            $0.kind == kind
                && $0.itemID == itemID
                && $0.removedRevision == revision
                && $0.removedAt == time
        }
        guard removals.allSatisfy({
            $0.removedBySource == record.source
                && $0.removedByDecisionID == record.decisionID
        }) else {
            throw invalid("classification merge removal source does not match its audit record")
        }
    }

    func isNormalizedClassificationName(_ name: String) -> Bool {
        name.isEmpty == false && ClassificationNameCanonicalizer.displayName(name) == name
    }

    func classificationName(
        for kind: ClassificationItemKind,
        itemID: String,
        in versions: [ClassificationNameVersion],
        at time: Date,
        revision: UInt64
    ) -> String? {
        if let beforeRename = laterRenameBeforeName(
            kind: kind,
            itemID: itemID,
            at: time,
            revision: revision
        ), versions.contains(where: { $0.name == beforeRename && $0.validUntil == time }) {
            return beforeRename
        }
        return versions.first { version in
            version.validFrom <= time && (version.validUntil.map { time < $0 } ?? true)
        }?.name
    }

    func laterRenameBeforeName(
        kind: ClassificationItemKind,
        itemID: String,
        at time: Date,
        revision: UInt64
    ) -> String? {
        let laterRecords = state.changeRecords
            .filter { $0.committedAt == time && $0.revision > revision }
            .sorted { $0.revision < $1.revision }
        for record in laterRecords {
            for change in record.changes {
                guard case let .rename(changeKind, changeItemID, beforeName, _) = change else { continue }
                guard changeKind == kind, changeItemID == itemID else { continue }
                return beforeName
            }
        }
        return nil
    }

    func validateHardDeleteRecord(
        kind: ClassificationItemKind,
        itemID: String,
        revision: UInt64,
        time: Date,
        changeRecordID: UUID
    ) throws {
        guard let record = changeRecordsByID[changeRecordID],
              record.revision == revision,
              record.committedAt == time
        else {
            throw invalid("classification deletion audit record is missing or inconsistent")
        }
        let matches = record.changes.filter {
            isMatchingHardDeleteChange($0, kind: kind, itemID: itemID)
        }
        guard matches.count == 1 else {
            throw invalid("classification deletion audit change does not match its tombstone")
        }
    }

    func isMatchingHardDeleteChange(
        _ change: ClassificationPlanChange,
        kind: ClassificationItemKind,
        itemID: String
    ) -> Bool {
        guard case let .hardDelete(
            changeKind,
            changeItemID,
            name,
            lifecycle,
            references
        ) = change else {
            return false
        }
        return changeKind == kind
            && changeItemID == itemID
            && isNormalizedClassificationName(name)
            && (lifecycle == .active || lifecycle == .archived)
            && references.isEmpty
    }
}
