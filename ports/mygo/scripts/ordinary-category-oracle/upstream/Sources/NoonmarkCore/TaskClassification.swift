import CryptoKit
import Darwin
import Foundation

public enum ClassificationLifecycle: String, Codable, Equatable, Sendable {
    case active
    case archived
    case merged
}

public enum CategoryPresentationApproval: String, Codable, Equatable, Sendable {
    case userApproved
    case pendingAIReview
}

public enum ClassificationNameCanonicalizer {
    private static let caseFolder = ICUNFKCCaseFolder()

    public static let algorithmVersion =
        "unicode-\(caseFolder.unicodeVersion)-nfkc-casefold-whitespace-v1"

    public static func displayName(_ name: String) -> String {
        name.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
    }

    public static func canonicalKey(_ name: String) -> String {
        caseFolder.normalize(displayName(name))
    }
}

private struct ICUNFKCCaseFolder: @unchecked Sendable {
    private typealias ErrorCode = Int32
    private typealias GetNormalizer = @convention(c) (UnsafeMutablePointer<ErrorCode>?) -> OpaquePointer?
    private typealias Normalize = @convention(c) (
        OpaquePointer?,
        UnsafePointer<UInt16>?,
        Int32,
        UnsafeMutablePointer<UInt16>?,
        Int32,
        UnsafeMutablePointer<ErrorCode>?
    ) -> Int32
    private typealias GetUnicodeVersion = @convention(c) (UnsafeMutablePointer<UInt8>) -> Void

    private static let bufferOverflowError: ErrorCode = 15

    private let normalizer: OpaquePointer
    private let normalizeFunction: Normalize
    let unicodeVersion: String

    init() {
        let defaultHandle = UnsafeMutableRawPointer(bitPattern: -2)
        guard let getNormalizerSymbol = dlsym(defaultHandle, "unorm2_getNFKCCasefoldInstance"),
              let normalizeSymbol = dlsym(defaultHandle, "unorm2_normalize"),
              let getUnicodeVersionSymbol = dlsym(defaultHandle, "u_getUnicodeVersion")
        else {
            preconditionFailure("required ICU NFKC_Casefold symbols are unavailable")
        }

        let getNormalizer = unsafeBitCast(getNormalizerSymbol, to: GetNormalizer.self)
        normalizeFunction = unsafeBitCast(normalizeSymbol, to: Normalize.self)
        let getUnicodeVersion = unsafeBitCast(getUnicodeVersionSymbol, to: GetUnicodeVersion.self)

        var error: ErrorCode = 0
        guard let normalizer = getNormalizer(&error), error <= 0 else {
            preconditionFailure("ICU NFKC_Casefold normalizer initialization failed: \(error)")
        }
        self.normalizer = normalizer

        var version: [UInt8] = [0, 0, 0, 0]
        version.withUnsafeMutableBufferPointer { buffer in
            getUnicodeVersion(buffer.baseAddress!)
        }
        unicodeVersion = version.map(String.init).joined(separator: ".")
    }

    func normalize(_ value: String) -> String {
        let source = Array(value.utf16)
        return source.withUnsafeBufferPointer { sourceBuffer in
            var error: ErrorCode = 0
            let requiredCount = normalizeFunction(
                normalizer,
                sourceBuffer.baseAddress,
                Int32(sourceBuffer.count),
                nil,
                0,
                &error
            )
            guard error == Self.bufferOverflowError || (error <= 0 && requiredCount == 0) else {
                preconditionFailure("ICU NFKC_Casefold preflight failed: \(error)")
            }

            var output = [UInt16](repeating: 0, count: Int(requiredCount) + 1)
            error = 0
            let writtenCount = output.withUnsafeMutableBufferPointer { outputBuffer in
                normalizeFunction(
                    normalizer,
                    sourceBuffer.baseAddress,
                    Int32(sourceBuffer.count),
                    outputBuffer.baseAddress,
                    Int32(outputBuffer.count),
                    &error
                )
            }
            guard error <= 0 else {
                preconditionFailure("ICU NFKC_Casefold normalization failed: \(error)")
            }
            return String(decoding: output.prefix(Int(writtenCount)), as: UTF16.self)
        }
    }
}

public struct ClassificationNameVersion: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let name: String
    public let canonicalKey: String
    public let canonicalKeyVersion: String
    public let validFrom: Date
    public var validUntil: Date?

    public init(
        id: UUID = UUID(),
        name: String,
        canonicalKey: String,
        canonicalKeyVersion: String = ClassificationNameCanonicalizer.algorithmVersion,
        validFrom: Date,
        validUntil: Date? = nil
    ) {
        self.id = id
        self.name = name
        self.canonicalKey = canonicalKey
        self.canonicalKeyVersion = canonicalKeyVersion
        self.validFrom = validFrom
        self.validUntil = validUntil
    }
}

private struct ClassificationIdentityValidation {
    let name: String
    let canonicalKey: String
    let canonicalKeyVersion: String
    let nameVersions: [ClassificationNameVersion]
    let createdAt: Date
    let updatedAt: Date
}

private func validateClassificationIdentity(
    _ identity: ClassificationIdentityValidation,
    codingPath: [any CodingKey]
) throws {
    func invalid(_ description: String) -> DecodingError {
        .dataCorrupted(.init(codingPath: codingPath, debugDescription: description))
    }

    guard ClassificationNameCanonicalizer.displayName(identity.name) == identity.name,
          identity.name.isEmpty == false
    else {
        throw invalid("classification name is empty or not whitespace-normalized")
    }
    guard identity.canonicalKey.isEmpty == false,
          identity.canonicalKeyVersion == ClassificationNameCanonicalizer.algorithmVersion,
          identity.canonicalKey == ClassificationNameCanonicalizer.canonicalKey(identity.name)
    else {
        throw invalid("classification canonical key is empty, unsupported, or inconsistent")
    }
    guard identity.createdAt <= identity.updatedAt, identity.nameVersions.isEmpty == false else {
        throw invalid("classification timestamps or name versions are invalid")
    }
    guard Set(identity.nameVersions.map(\.id)).count == identity.nameVersions.count else {
        throw invalid("classification name version IDs are not unique")
    }
    try validateClassificationNameVersions(identity, invalid: invalid)
}

private func validateClassificationNameVersions(
    _ identity: ClassificationIdentityValidation,
    invalid: (String) -> DecodingError
) throws {
    for index in identity.nameVersions.indices {
        let version = identity.nameVersions[index]
        guard ClassificationNameCanonicalizer.displayName(version.name) == version.name,
              version.name.isEmpty == false,
              version.canonicalKey.isEmpty == false,
              version.canonicalKeyVersion == ClassificationNameCanonicalizer.algorithmVersion,
              version.canonicalKey == ClassificationNameCanonicalizer.canonicalKey(version.name)
        else {
            throw invalid("classification name version is malformed")
        }
        if let validUntil = version.validUntil {
            guard validUntil > version.validFrom else {
                throw invalid("classification name version has an empty or backwards interval")
            }
        }
        if index < identity.nameVersions.index(before: identity.nameVersions.endIndex) {
            guard version.validUntil == identity.nameVersions[identity.nameVersions.index(after: index)].validFrom else {
                throw invalid("classification name version intervals are not contiguous")
            }
        } else {
            guard version.validUntil == nil else {
                throw invalid("classification current name version is already closed")
            }
        }
    }

    guard identity.nameVersions.first?.validFrom == identity.createdAt,
          let currentVersion = identity.nameVersions.last,
          currentVersion.validFrom <= identity.updatedAt,
          currentVersion.name == identity.name,
          currentVersion.canonicalKey == identity.canonicalKey,
          currentVersion.canonicalKeyVersion == identity.canonicalKeyVersion
    else {
        throw invalid("classification current name does not match its version chain")
    }
}

public struct TaskCategory: Codable, Equatable, Sendable, Identifiable {
    private enum CodingKeys: String, CodingKey {
        case id
        case name
        case canonicalKey
        case canonicalKeyVersion
        case colorHex
        case presentationApproval
        case lifecycle
        case nameVersions
        case createdAt
        case updatedAt
    }

    public let id: TaskCategoryID
    public var name: String
    public var canonicalKey: String
    public var canonicalKeyVersion: String
    public var colorHex: String
    public var presentationApproval: CategoryPresentationApproval
    public var lifecycle: ClassificationLifecycle
    public var nameVersions: [ClassificationNameVersion]
    public let createdAt: Date
    public var updatedAt: Date

    public init(
        id: TaskCategoryID = TaskCategoryID(),
        name: String,
        colorHex: String,
        now: Date
    ) {
        self.init(
            id: id,
            name: name,
            colorHex: colorHex,
            presentationApproval: .userApproved,
            now: now
        )
    }

    public init(
        id: TaskCategoryID,
        name: String,
        colorHex: String,
        presentationApproval: CategoryPresentationApproval,
        now: Date
    ) {
        let displayName = ClassificationNameCanonicalizer.displayName(name)
        let canonicalKey = ClassificationNameCanonicalizer.canonicalKey(displayName)
        self.id = id
        self.name = displayName
        self.canonicalKey = canonicalKey
        self.canonicalKeyVersion = ClassificationNameCanonicalizer.algorithmVersion
        self.colorHex = colorHex
        self.presentationApproval = presentationApproval
        self.lifecycle = .active
        self.nameVersions = [
            ClassificationNameVersion(name: displayName, canonicalKey: canonicalKey, validFrom: now)
        ]
        self.createdAt = now
        self.updatedAt = now
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(TaskCategoryID.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        colorHex = try container.decode(String.self, forKey: .colorHex)
        presentationApproval = try container.decode(
            CategoryPresentationApproval.self,
            forKey: .presentationApproval
        )
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        canonicalKey = try container.decode(String.self, forKey: .canonicalKey)
        canonicalKeyVersion = try container.decode(String.self, forKey: .canonicalKeyVersion)
        lifecycle = try container.decode(ClassificationLifecycle.self, forKey: .lifecycle)
        nameVersions = try container.decode(
            [ClassificationNameVersion].self,
            forKey: .nameVersions
        )
        try validateClassificationIdentity(
            ClassificationIdentityValidation(
                name: name,
                canonicalKey: canonicalKey,
                canonicalKeyVersion: canonicalKeyVersion,
                nameVersions: nameVersions,
                createdAt: createdAt,
                updatedAt: updatedAt
            ),
            codingPath: decoder.codingPath
        )
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(name, forKey: .name)
        try container.encode(canonicalKey, forKey: .canonicalKey)
        try container.encode(canonicalKeyVersion, forKey: .canonicalKeyVersion)
        try container.encode(colorHex, forKey: .colorHex)
        try container.encode(presentationApproval, forKey: .presentationApproval)
        try container.encode(lifecycle, forKey: .lifecycle)
        try container.encode(nameVersions, forKey: .nameVersions)
        try container.encode(createdAt, forKey: .createdAt)
        try container.encode(updatedAt, forKey: .updatedAt)
    }
}

public struct TaskLabel: Codable, Equatable, Sendable, Identifiable {
    private enum CodingKeys: String, CodingKey {
        case id
        case name
        case canonicalKey
        case canonicalKeyVersion
        case colorHex
        case lifecycle
        case nameVersions
        case createdAt
        case updatedAt
    }

    public let id: TaskLabelID
    public var name: String
    public var canonicalKey: String
    public var canonicalKeyVersion: String
    public var colorHex: String
    public var lifecycle: ClassificationLifecycle
    public var nameVersions: [ClassificationNameVersion]
    public let createdAt: Date
    public var updatedAt: Date

    public init(
        id: TaskLabelID = TaskLabelID(),
        name: String,
        colorHex: String,
        now: Date
    ) {
        let displayName = ClassificationNameCanonicalizer.displayName(name)
        let canonicalKey = ClassificationNameCanonicalizer.canonicalKey(displayName)
        self.id = id
        self.name = displayName
        self.canonicalKey = canonicalKey
        self.canonicalKeyVersion = ClassificationNameCanonicalizer.algorithmVersion
        self.colorHex = colorHex
        self.lifecycle = .active
        self.nameVersions = [
            ClassificationNameVersion(name: displayName, canonicalKey: canonicalKey, validFrom: now)
        ]
        self.createdAt = now
        self.updatedAt = now
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(TaskLabelID.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        colorHex = try container.decode(String.self, forKey: .colorHex)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        canonicalKey = try container.decode(String.self, forKey: .canonicalKey)
        canonicalKeyVersion = try container.decode(String.self, forKey: .canonicalKeyVersion)
        lifecycle = try container.decode(ClassificationLifecycle.self, forKey: .lifecycle)
        nameVersions = try container.decode(
            [ClassificationNameVersion].self,
            forKey: .nameVersions
        )
        try validateClassificationIdentity(
            ClassificationIdentityValidation(
                name: name,
                canonicalKey: canonicalKey,
                canonicalKeyVersion: canonicalKeyVersion,
                nameVersions: nameVersions,
                createdAt: createdAt,
                updatedAt: updatedAt
            ),
            codingPath: decoder.codingPath
        )
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(name, forKey: .name)
        try container.encode(canonicalKey, forKey: .canonicalKey)
        try container.encode(canonicalKeyVersion, forKey: .canonicalKeyVersion)
        try container.encode(colorHex, forKey: .colorHex)
        try container.encode(lifecycle, forKey: .lifecycle)
        try container.encode(nameVersions, forKey: .nameVersions)
        try container.encode(createdAt, forKey: .createdAt)
        try container.encode(updatedAt, forKey: .updatedAt)
    }
}

public enum TaskCategoryChoice: Equatable, Sendable {
    case existing(TaskCategoryID)
    case new(name: String, colorHex: String)
}

public enum TaskLabelChoice: Equatable, Sendable {
    case existing(TaskLabelID)
    case new(name: String, colorHex: String)
}

public struct TaskClassificationDraft: Equatable, Sendable {
    public let chainID: TaskChainID
    public let category: TaskCategoryChoice?
    public let labels: [TaskLabelChoice]

    public init(
        chainID: TaskChainID,
        category: TaskCategoryChoice?,
        labels: [TaskLabelChoice]
    ) {
        self.chainID = chainID
        self.category = category
        self.labels = labels
    }
}

public enum ClassificationIntent: Equatable, Sendable {
    case createCategory(name: String, colorHex: String)
    case createLabel(name: String, colorHex: String)
    case mergeCategory(source: TaskCategoryID, into: TaskCategoryID)
    case mergeLabel(source: TaskLabelID, into: TaskLabelID)
    case hardDeleteCategory(TaskCategoryID)
    case hardDeleteLabel(TaskLabelID)
    case setCurrent(TaskClassificationDraft)
    case renameCategory(TaskCategoryID, to: String)
    case approveCategoryPresentation(TaskCategoryID)
    case renameLabel(TaskLabelID, to: String)
    case archiveCategory(TaskCategoryID)
    case restoreCategory(TaskCategoryID)
    case archiveLabel(TaskLabelID)
    case restoreLabel(TaskLabelID)
}

public enum ClassificationSource: Codable, Equatable, Sendable {
    case userDirect
    case zhulongSuggestion(
        sessionID: UUID,
        draftID: UUID,
        draftVersion: Int,
        evidenceID: UUID
    )
    case automaticAI(jobID: UUID, generation: Int)
    case inherited(fromChainID: TaskChainID)
    case deterministicDomainAction(reason: String)
}

private extension ClassificationSource {
    var categoryPresentationApproval: CategoryPresentationApproval {
        if case .automaticAI = self {
            return .pendingAIReview
        }
        return .userApproved
    }
}

public struct ClassificationConfirmation: Equatable, Sendable {
    let decisionID: UUID
    private let planID: UUID?
    private let interactionID: UUID?
    private let planDigest: String?
    private let authority: ClassificationConfirmationAuthority

    static func user(decisionID: UUID) -> ClassificationConfirmation {
        ClassificationConfirmation(
            decisionID: decisionID,
            planID: nil,
            interactionID: nil,
            planDigest: nil,
            authority: .coreTest
        )
    }

    static func user(
        confirming plan: ClassificationPlan,
        decisionID: UUID
    ) -> ClassificationConfirmation {
        boundToUserDecision(plan: plan, decisionID: decisionID)
    }

    @_spi(ClassificationUserDecision)
    public static func confirmedByUser(
        confirming plan: ClassificationPlan,
        decisionID: UUID
    ) -> ClassificationConfirmation {
        boundToUserDecision(plan: plan, decisionID: decisionID)
    }

    func authorizes(_ plan: ClassificationPlan) -> Bool {
        switch authority {
        case .userInterface:
            return planID == plan.id
                && interactionID == plan.interactionID
                && planDigest == plan.digest
        case .coreTest:
            return true
        }
    }

    private static func boundToUserDecision(
        plan: ClassificationPlan,
        decisionID: UUID
    ) -> ClassificationConfirmation {
        ClassificationConfirmation(
            decisionID: decisionID,
            planID: plan.id,
            interactionID: plan.interactionID,
            planDigest: plan.digest,
            authority: .userInterface
        )
    }
}

private enum ClassificationConfirmationAuthority: Equatable {
    case userInterface
    case coreTest
}

public enum ClassificationItemKind: String, Codable, Equatable, Sendable {
    case category
    case label
}

public enum ClassificationNotice: Codable, Equatable, Sendable {
    case duplicateLabelCollapsed(name: String)
    case existingItemReused(
        kind: ClassificationItemKind,
        inputName: String,
        currentName: String,
        matchedHistoricalAlias: Bool
    )
}

public enum ClassificationBlocker: Codable, Equatable, Sendable {
    case referencedItem(
        kind: ClassificationItemKind,
        itemID: String,
        references: ClassificationReferenceSummary
    )
}

public enum ClassificationPlanItemResolution: String, Codable, Equatable, Sendable {
    case existing
    case historicalAlias
    case new
}

public struct ClassificationPlanItem: Codable, Equatable, Sendable, Identifiable {
    public let id: String?
    public let kind: ClassificationItemKind
    public let name: String
    public let colorHex: String
    public let resolution: ClassificationPlanItemResolution

    public init(
        id: String?,
        kind: ClassificationItemKind,
        name: String,
        colorHex: String,
        resolution: ClassificationPlanItemResolution
    ) {
        self.id = id
        self.kind = kind
        self.name = name
        self.colorHex = colorHex
        self.resolution = resolution
    }
}

public struct ClassificationSelectionPreview: Codable, Equatable, Sendable {
    public let category: ClassificationPlanItem?
    public let labels: [ClassificationPlanItem]

    public init(category: ClassificationPlanItem?, labels: [ClassificationPlanItem]) {
        self.category = category
        self.labels = labels
    }
}

public struct ClassificationMergeImpact: Codable, Equatable, Sendable {
    public let currentChainIDs: [TaskChainID]
    public let historicalTraceIDs: [DayTraceID]
    public let historicalEventIDs: [UUID]
    public let migratedRelationCount: Int
    public let deduplicatedRelationCount: Int

    public init(
        currentChainIDs: [TaskChainID],
        historicalTraceIDs: [DayTraceID],
        historicalEventIDs: [UUID],
        migratedRelationCount: Int,
        deduplicatedRelationCount: Int
    ) {
        self.currentChainIDs = currentChainIDs
        self.historicalTraceIDs = historicalTraceIDs
        self.historicalEventIDs = historicalEventIDs
        self.migratedRelationCount = migratedRelationCount
        self.deduplicatedRelationCount = deduplicatedRelationCount
    }
}

public struct ClassificationReferenceSummary: Codable, Equatable, Sendable {
    public let currentRelationCount: Int
    public let relationHistoryCount: Int
    public let historicalEventCount: Int
    public let mergeSourceCount: Int
    public let mergeTargetCount: Int

    public init(
        currentRelationCount: Int = 0,
        relationHistoryCount: Int = 0,
        historicalEventCount: Int = 0,
        mergeSourceCount: Int = 0,
        mergeTargetCount: Int = 0
    ) {
        self.currentRelationCount = currentRelationCount
        self.relationHistoryCount = relationHistoryCount
        self.historicalEventCount = historicalEventCount
        self.mergeSourceCount = mergeSourceCount
        self.mergeTargetCount = mergeTargetCount
    }

    public var isEmpty: Bool {
        currentRelationCount == 0
            && relationHistoryCount == 0
            && historicalEventCount == 0
            && mergeSourceCount == 0
            && mergeTargetCount == 0
    }
}

public struct TaskCategoryDeletionOutcome: Equatable, Sendable {
    public let ungroupedTaskCount: Int
    public let updatedTodaySnapshotCount: Int

    public init(
        ungroupedTaskCount: Int,
        updatedTodaySnapshotCount: Int
    ) {
        self.ungroupedTaskCount = ungroupedTaskCount
        self.updatedTodaySnapshotCount = updatedTodaySnapshotCount
    }
}

public struct TaskCategoryDeletionCommitSequence: Equatable, Sendable {
    public let outcome: TaskCategoryDeletionOutcome
    public let commitBoundaries: [NoonmarkSnapshot]

    public init(
        outcome: TaskCategoryDeletionOutcome,
        commitBoundaries: [NoonmarkSnapshot]
    ) {
        self.outcome = outcome
        self.commitBoundaries = commitBoundaries
    }
}

public enum ClassificationPlanChange: Codable, Equatable, Sendable {
    case create(
        kind: ClassificationItemKind,
        itemID: String,
        name: String,
        colorHex: String
    )
    case merge(
        kind: ClassificationItemKind,
        sourceID: String,
        sourceName: String,
        targetID: String,
        targetName: String,
        sourceLifecycle: ClassificationLifecycle,
        impact: ClassificationMergeImpact
    )
    case hardDelete(
        kind: ClassificationItemKind,
        itemID: String,
        name: String,
        lifecycle: ClassificationLifecycle,
        verifiedReferences: ClassificationReferenceSummary
    )
    case setCurrent(
        chainID: TaskChainID,
        before: ClassificationSelectionPreview,
        after: ClassificationSelectionPreview
    )
    case rename(
        kind: ClassificationItemKind,
        itemID: String,
        beforeName: String,
        afterName: String
    )
    case categoryPresentationApproval(
        itemID: String,
        name: String,
        before: CategoryPresentationApproval,
        after: CategoryPresentationApproval
    )
    case lifecycle(
        kind: ClassificationItemKind,
        itemID: String,
        name: String,
        before: ClassificationLifecycle,
        after: ClassificationLifecycle
    )
}

public struct ClassificationPlan: Equatable, Sendable {
    private static let digestFormatVersion = 1

    public let id: UUID
    public let baseRevision: UInt64
    public let interactionID: UUID
    public let notices: [ClassificationNotice]
    public let source: ClassificationSource
    public let changes: [ClassificationPlanChange]
    public let blockers: [ClassificationBlocker]
    public let digest: String

    let intent: ClassificationIntent
    let createdItemID: UUID?
    let plannedCreations: ClassificationPlannedCreations

    init(
        id: UUID = UUID(),
        baseRevision: UInt64,
        interactionID: UUID,
        intent: ClassificationIntent,
        source: ClassificationSource,
        changes: [ClassificationPlanChange],
        createdItemID: UUID? = nil,
        plannedCreations: ClassificationPlannedCreations = ClassificationPlannedCreations(),
        notices: [ClassificationNotice] = [],
        blockers: [ClassificationBlocker] = [],
        digest: String? = nil
    ) throws {
        self.id = id
        self.baseRevision = baseRevision
        self.interactionID = interactionID
        self.intent = intent
        self.source = source
        self.changes = changes
        self.createdItemID = createdItemID
        self.plannedCreations = plannedCreations
        self.notices = notices
        self.blockers = blockers
        let digestPayload = ClassificationPlanDigestPayload(
            formatVersion: Self.digestFormatVersion,
            planID: id,
            baseRevision: baseRevision,
            interactionID: interactionID,
            source: source,
            changes: changes,
            createdItemID: createdItemID,
            plannedCategoryID: plannedCreations.categoryID,
            plannedLabelIDs: plannedCreations.digestLabelIDs,
            notices: notices,
            blockers: blockers
        )
        self.digest = try digest ?? Self.computeDigest(for: digestPayload)
    }

    func hasValidDigest() throws -> Bool {
        let payload = ClassificationPlanDigestPayload(
            formatVersion: Self.digestFormatVersion,
            planID: id,
            baseRevision: baseRevision,
            interactionID: interactionID,
            source: source,
            changes: changes,
            createdItemID: createdItemID,
            plannedCategoryID: plannedCreations.categoryID,
            plannedLabelIDs: plannedCreations.digestLabelIDs,
            notices: notices,
            blockers: blockers
        )
        return digest == (try Self.computeDigest(for: payload))
    }

    private static func computeDigest(
        for payload: ClassificationPlanDigestPayload
    ) throws -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(payload)
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }
}

private struct ClassificationPlanDigestPayload: Encodable {
    let formatVersion: Int
    let planID: UUID
    let baseRevision: UInt64
    let interactionID: UUID
    let source: ClassificationSource
    let changes: [ClassificationPlanChange]
    let createdItemID: UUID?
    let plannedCategoryID: TaskCategoryID?
    let plannedLabelIDs: [ClassificationPlannedLabelDigestEntry]
    let notices: [ClassificationNotice]
    let blockers: [ClassificationBlocker]
}

private struct ClassificationPlannedLabelDigestEntry: Encodable {
    let canonicalKey: String
    let id: TaskLabelID
}

struct ClassificationPlannedCreations: Equatable {
    var categoryID: TaskCategoryID?
    var labelIDsByCanonicalKey: [String: TaskLabelID]

    init(
        categoryID: TaskCategoryID? = nil,
        labelIDsByCanonicalKey: [String: TaskLabelID] = [:]
    ) {
        self.categoryID = categoryID
        self.labelIDsByCanonicalKey = labelIDsByCanonicalKey
    }

    fileprivate var digestLabelIDs: [ClassificationPlannedLabelDigestEntry] {
        labelIDsByCanonicalKey.map {
            ClassificationPlannedLabelDigestEntry(canonicalKey: $0.key, id: $0.value)
        }
        .sorted { $0.canonicalKey < $1.canonicalKey }
    }
}

public struct ClassificationReceipt: Codable, Equatable, Sendable {
    public let planID: UUID
    public let revision: UInt64
    public let notices: [ClassificationNotice]
    public let changeRecordID: UUID
    public let decisionID: UUID?
    public let changeRecordIntegrityDigest: String

    public init(
        planID: UUID,
        revision: UInt64,
        notices: [ClassificationNotice],
        changeRecordID: UUID,
        decisionID: UUID?,
        changeRecordIntegrityDigest: String
    ) {
        self.planID = planID
        self.revision = revision
        self.notices = notices
        self.changeRecordID = changeRecordID
        self.decisionID = decisionID
        self.changeRecordIntegrityDigest = changeRecordIntegrityDigest
    }
}

public struct ClassificationChangeRecord: Codable, Equatable, Sendable, Identifiable {
    private static let integrityDigestFormatVersion = 1

    public let id: UUID
    public let planID: UUID
    public let interactionID: UUID
    public let source: ClassificationSource
    public let decisionID: UUID?
    public let changes: [ClassificationPlanChange]
    public let notices: [ClassificationNotice]
    public let committedAt: Date
    public let revision: UInt64
    public let planDigest: String
    public let integrityDigest: String

    public var advancesStateRevision: Bool {
        changes.contains { change in
            switch change {
            case let .rename(_, _, beforeName, afterName):
                beforeName != afterName
            case let .categoryPresentationApproval(_, _, before, after):
                before != after
            case .create, .merge, .hardDelete, .setCurrent, .lifecycle:
                true
            }
        }
    }

    public init(
        id: UUID = UUID(),
        planID: UUID,
        interactionID: UUID,
        source: ClassificationSource,
        decisionID: UUID?,
        changes: [ClassificationPlanChange],
        notices: [ClassificationNotice] = [],
        committedAt: Date,
        revision: UInt64,
        planDigest: String
    ) {
        self.id = id
        self.planID = planID
        self.interactionID = interactionID
        self.source = source
        self.decisionID = decisionID
        self.changes = changes
        self.notices = notices
        self.committedAt = committedAt
        self.revision = revision
        self.planDigest = planDigest
        self.integrityDigest = Self.computeIntegrityDigest(
            ClassificationRecordDigestMaterial(
                formatVersion: Self.integrityDigestFormatVersion,
                id: id,
                planID: planID,
                interactionID: interactionID,
                source: source,
                decisionID: decisionID,
                changes: changes,
                notices: notices,
                committedAtBitPattern: committedAt.timeIntervalSinceReferenceDate.bitPattern,
                revision: revision,
                planDigest: planDigest
            )
        )
    }

    public init(
        id: UUID,
        planID: UUID,
        interactionID: UUID,
        source: ClassificationSource,
        decisionID: UUID?,
        changes: [ClassificationPlanChange],
        notices: [ClassificationNotice],
        committedAt: Date,
        revision: UInt64,
        planDigest: String,
        integrityDigest: String
    ) {
        self.id = id
        self.planID = planID
        self.interactionID = interactionID
        self.source = source
        self.decisionID = decisionID
        self.changes = changes
        self.notices = notices
        self.committedAt = committedAt
        self.revision = revision
        self.planDigest = planDigest
        self.integrityDigest = integrityDigest
    }

    func hasValidIntegrityDigest() -> Bool {
        return integrityDigest == Self.computeIntegrityDigest(
            ClassificationRecordDigestMaterial(
                formatVersion: Self.integrityDigestFormatVersion,
                id: id,
                planID: planID,
                interactionID: interactionID,
                source: source,
                decisionID: decisionID,
                changes: changes,
                notices: notices,
                committedAtBitPattern: committedAt.timeIntervalSinceReferenceDate.bitPattern,
                revision: revision,
                planDigest: planDigest
            )
        )
    }

    private static func computeIntegrityDigest(
        _ material: ClassificationRecordDigestMaterial
    ) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        guard let data = try? encoder.encode(material) else {
            preconditionFailure("classification change record integrity payload cannot be encoded")
        }
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }
}

private struct ClassificationRecordDigestMaterial: Encodable {
    let formatVersion: Int
    let id: UUID
    let planID: UUID
    let interactionID: UUID
    let source: ClassificationSource
    let decisionID: UUID?
    let changes: [ClassificationPlanChange]
    let notices: [ClassificationNotice]
    let committedAtBitPattern: UInt64
    let revision: UInt64
    let planDigest: String
}

public enum ClassificationAuditCanonicalOrderError: Error, Equatable, Sendable {
    case nonFiniteChangeRecordDate(UUID)
    case nonFiniteRelationHistoryDate(UUID)
}

public enum ClassificationAuditCanonicalOrder {
    public static func changeRecords(
        _ records: [ClassificationChangeRecord]
    ) throws -> [ClassificationChangeRecord] {
        for record in records
        where record.committedAt.timeIntervalSinceReferenceDate.isFinite == false {
            throw ClassificationAuditCanonicalOrderError.nonFiniteChangeRecordDate(record.id)
        }
        return records.sorted(by: changeRecordComesBefore)
    }

    public static func relationHistory(
        _ entries: [ClassificationRelationHistoryEntry]
    ) throws -> [ClassificationRelationHistoryEntry] {
        for entry in entries
        where entry.removedAt.timeIntervalSinceReferenceDate.isFinite == false {
            throw ClassificationAuditCanonicalOrderError.nonFiniteRelationHistoryDate(entry.id)
        }
        return entries.sorted(by: relationHistoryEntryComesBefore)
    }

    private static func changeRecordComesBefore(
        _ lhs: ClassificationChangeRecord,
        _ rhs: ClassificationChangeRecord
    ) -> Bool {
        if lhs.revision != rhs.revision {
            return lhs.revision < rhs.revision
        }
        if lhs.committedAt != rhs.committedAt {
            return lhs.committedAt < rhs.committedAt
        }
        return lhs.id.uuidString < rhs.id.uuidString
    }

    private static func relationHistoryEntryComesBefore(
        _ lhs: ClassificationRelationHistoryEntry,
        _ rhs: ClassificationRelationHistoryEntry
    ) -> Bool {
        if lhs.removedRevision != rhs.removedRevision {
            return lhs.removedRevision < rhs.removedRevision
        }
        if lhs.removedAt != rhs.removedAt {
            return lhs.removedAt < rhs.removedAt
        }
        if lhs.kind != rhs.kind {
            return lhs.kind.rawValue < rhs.kind.rawValue
        }
        if lhs.chainID != rhs.chainID {
            return lhs.chainID.description < rhs.chainID.description
        }
        if lhs.itemID != rhs.itemID {
            return lhs.itemID < rhs.itemID
        }
        return lhs.id.uuidString < rhs.id.uuidString
    }
}

public enum ClassificationQuery: Equatable, Sendable {
    case task(TaskChainID)
    case history(DayTraceID)
    case catalog
}

public struct ClassificationItemProjection: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let colorHex: String
    public let presentationApproval: CategoryPresentationApproval?

    public init(
        id: String,
        name: String,
        colorHex: String,
        presentationApproval: CategoryPresentationApproval? = nil
    ) {
        self.id = id
        self.name = name
        self.colorHex = colorHex
        self.presentationApproval = presentationApproval
    }
}

public struct TaskClassificationProjection: Equatable, Sendable {
    public let chainID: TaskChainID
    public let category: ClassificationItemProjection?
    public let labels: [ClassificationItemProjection]
    public let revision: UInt64
}

public struct TraceClassificationProjection: Equatable, Sendable {
    public let traceID: DayTraceID
    public let category: ClassificationItemProjection?
    public let labels: [ClassificationItemProjection]
    public let capturedAt: Date
    public let events: [TraceClassificationEventProjection]
}

public struct TraceClassificationEventProjection: Equatable, Sendable, Identifiable {
    public let id: UUID
    public let status: TraceStatus
    public let category: ClassificationItemProjection?
    public let labels: [ClassificationItemProjection]
    public let capturedAt: Date
}

public struct ClassificationCatalogItemProjection: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let colorHex: String
    public let lifecycle: ClassificationLifecycle
    public let mergedIntoID: String?
    public let currentUsageCount: Int
    public let historicalUsageCount: Int
    public let presentationApproval: CategoryPresentationApproval?

    public init(
        id: String,
        name: String,
        colorHex: String,
        lifecycle: ClassificationLifecycle,
        mergedIntoID: String? = nil,
        currentUsageCount: Int,
        historicalUsageCount: Int,
        presentationApproval: CategoryPresentationApproval? = nil
    ) {
        self.id = id
        self.name = name
        self.colorHex = colorHex
        self.lifecycle = lifecycle
        self.mergedIntoID = mergedIntoID
        self.currentUsageCount = currentUsageCount
        self.historicalUsageCount = historicalUsageCount
        self.presentationApproval = presentationApproval
    }
}

public struct ClassificationCatalogProjection: Equatable, Sendable {
    public let categories: [ClassificationCatalogItemProjection]
    public let labels: [ClassificationCatalogItemProjection]
    public let revision: UInt64
}

public enum ClassificationProjection: Equatable, Sendable {
    case task(TaskClassificationProjection)
    case history(TraceClassificationProjection)
    case catalog(ClassificationCatalogProjection)
}

public struct TaskCategoryRelation: Codable, Equatable, Sendable {
    public let categoryID: TaskCategoryID
    public let source: ClassificationSource
    public let decisionID: UUID?
    public let createdAt: Date
    public let updatedAt: Date
    public let revision: UInt64

    public init(
        categoryID: TaskCategoryID,
        source: ClassificationSource,
        decisionID: UUID?,
        createdAt: Date,
        updatedAt: Date,
        revision: UInt64
    ) {
        self.categoryID = categoryID
        self.source = source
        self.decisionID = decisionID
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.revision = revision
    }
}

public struct TaskLabelRelation: Codable, Equatable, Sendable {
    public let labelID: TaskLabelID
    public let source: ClassificationSource
    public let decisionID: UUID?
    public let createdAt: Date
    public let updatedAt: Date
    public let revision: UInt64

    public init(
        labelID: TaskLabelID,
        source: ClassificationSource,
        decisionID: UUID?,
        createdAt: Date,
        updatedAt: Date,
        revision: UInt64
    ) {
        self.labelID = labelID
        self.source = source
        self.decisionID = decisionID
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.revision = revision
    }
}

public struct ClassificationRelationHistoryEntry: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let kind: ClassificationItemKind
    public let chainID: TaskChainID
    public let itemID: String
    public let originSource: ClassificationSource
    public let originDecisionID: UUID?
    public let createdAt: Date
    public let createdRevision: UInt64
    public let removedBySource: ClassificationSource
    public let removedByDecisionID: UUID?
    public let removedAt: Date
    public let removedRevision: UInt64

    public init(
        id: UUID = UUID(),
        kind: ClassificationItemKind,
        chainID: TaskChainID,
        itemID: String,
        originSource: ClassificationSource,
        originDecisionID: UUID?,
        createdAt: Date,
        createdRevision: UInt64,
        removedBySource: ClassificationSource,
        removedByDecisionID: UUID?,
        removedAt: Date,
        removedRevision: UInt64
    ) {
        self.id = id
        self.kind = kind
        self.chainID = chainID
        self.itemID = itemID
        self.originSource = originSource
        self.originDecisionID = originDecisionID
        self.createdAt = createdAt
        self.createdRevision = createdRevision
        self.removedBySource = removedBySource
        self.removedByDecisionID = removedByDecisionID
        self.removedAt = removedAt
        self.removedRevision = removedRevision
    }
}

public struct TaskCategoryMerge: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let sourceID: TaskCategoryID
    public let targetID: TaskCategoryID
    public let mergedAt: Date
    public let revision: UInt64
    public let changeRecordID: UUID

    public init(
        id: UUID = UUID(),
        sourceID: TaskCategoryID,
        targetID: TaskCategoryID,
        mergedAt: Date,
        revision: UInt64,
        changeRecordID: UUID
    ) {
        self.id = id
        self.sourceID = sourceID
        self.targetID = targetID
        self.mergedAt = mergedAt
        self.revision = revision
        self.changeRecordID = changeRecordID
    }
}

public struct TaskLabelMerge: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let sourceID: TaskLabelID
    public let targetID: TaskLabelID
    public let mergedAt: Date
    public let revision: UInt64
    public let changeRecordID: UUID

    public init(
        id: UUID = UUID(),
        sourceID: TaskLabelID,
        targetID: TaskLabelID,
        mergedAt: Date,
        revision: UInt64,
        changeRecordID: UUID
    ) {
        self.id = id
        self.sourceID = sourceID
        self.targetID = targetID
        self.mergedAt = mergedAt
        self.revision = revision
        self.changeRecordID = changeRecordID
    }
}

public struct TaskCategoryDeletionTombstone: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let itemID: TaskCategoryID
    public let deletedAt: Date
    public let revision: UInt64
    public let changeRecordID: UUID

    public init(
        id: UUID = UUID(),
        itemID: TaskCategoryID,
        deletedAt: Date,
        revision: UInt64,
        changeRecordID: UUID
    ) {
        self.id = id
        self.itemID = itemID
        self.deletedAt = deletedAt
        self.revision = revision
        self.changeRecordID = changeRecordID
    }
}

public struct TaskLabelDeletionTombstone: Codable, Equatable, Sendable, Identifiable {
    public let id: UUID
    public let itemID: TaskLabelID
    public let deletedAt: Date
    public let revision: UInt64
    public let changeRecordID: UUID

    public init(
        id: UUID = UUID(),
        itemID: TaskLabelID,
        deletedAt: Date,
        revision: UInt64,
        changeRecordID: UUID
    ) {
        self.id = id
        self.itemID = itemID
        self.deletedAt = deletedAt
        self.revision = revision
        self.changeRecordID = changeRecordID
    }
}

public struct CurrentTaskClassification: Codable, Equatable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case category
        case labels
    }

    public private(set) var category: TaskCategoryRelation?
    public private(set) var labels: [TaskLabelRelation]

    public var categoryID: TaskCategoryID? { category?.categoryID }
    public var labelIDs: Set<TaskLabelID> { Set(labels.map(\.labelID)) }

    public init(category: TaskCategoryRelation?, labels: [TaskLabelRelation]) {
        precondition(Set(labels.map(\.labelID)).count == labels.count, "task label relations must be unique")
        self.category = category
        self.labels = labels.sorted { $0.labelID.description < $1.labelID.description }
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let category = try container.decodeIfPresent(TaskCategoryRelation.self, forKey: .category)
        let labels = try container.decode([TaskLabelRelation].self, forKey: .labels)
        guard Set(labels.map(\.labelID)).count == labels.count else {
            throw DecodingError.dataCorruptedError(
                forKey: .labels,
                in: container,
                debugDescription: "task label relations must be unique"
            )
        }
        self.init(category: category, labels: labels)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encodeIfPresent(category, forKey: .category)
        try container.encode(labels, forKey: .labels)
    }
}

public struct HistoricalCategoryValue: Codable, Equatable, Sendable {
    public let id: TaskCategoryID
    public let name: String
    public let colorHex: String
    public let presentationApproval: CategoryPresentationApproval

    public init(
        id: TaskCategoryID,
        name: String,
        colorHex: String,
        presentationApproval: CategoryPresentationApproval = .userApproved
    ) {
        self.id = id
        self.name = name
        self.colorHex = colorHex
        self.presentationApproval = presentationApproval
    }
}

public struct HistoricalLabelValue: Codable, Equatable, Sendable {
    public let id: TaskLabelID
    public let name: String
    public let colorHex: String

    public init(id: TaskLabelID, name: String, colorHex: String) {
        self.id = id
        self.name = name
        self.colorHex = colorHex
    }
}

public struct TraceClassificationSnapshot: Codable, Equatable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case id
        case traceID
        case status
        case category
        case labels
        case capturedAt
        case revision
    }

    public let id: UUID
    public let traceID: DayTraceID
    public let status: TraceStatus
    public let category: HistoricalCategoryValue?
    public let labels: [HistoricalLabelValue]
    public let capturedAt: Date
    public let revision: UInt64

    public init(
        id: UUID = UUID(),
        traceID: DayTraceID,
        status: TraceStatus,
        category: HistoricalCategoryValue?,
        labels: [HistoricalLabelValue],
        capturedAt: Date,
        revision: UInt64
    ) {
        self.id = id
        self.traceID = traceID
        self.status = status
        self.category = category
        self.labels = labels
        self.capturedAt = capturedAt
        self.revision = revision
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        traceID = try container.decode(DayTraceID.self, forKey: .traceID)
        id = try container.decode(UUID.self, forKey: .id)
        status = try container.decode(TraceStatus.self, forKey: .status)
        category = try container.decodeIfPresent(HistoricalCategoryValue.self, forKey: .category)
        labels = try container.decode([HistoricalLabelValue].self, forKey: .labels)
        capturedAt = try container.decode(Date.self, forKey: .capturedAt)
        revision = try container.decode(UInt64.self, forKey: .revision)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(traceID, forKey: .traceID)
        try container.encode(status, forKey: .status)
        try container.encodeIfPresent(category, forKey: .category)
        try container.encode(labels, forKey: .labels)
        try container.encode(capturedAt, forKey: .capturedAt)
        try container.encode(revision, forKey: .revision)
    }
}

public struct TaskClassificationState: Codable, Equatable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case formatVersion
        case revision
        case categories
        case labels
        case currentByChainID
        case relationHistory
        case categoryMerges
        case labelMerges
        case categoryDeletionTombstones
        case labelDeletionTombstones
        case snapshotsByTraceID
        case snapshotEventsByTraceID
        case committedReceiptsByInteractionID
        case changeRecords
    }

    private static let currentFormatVersion = 1

    public var revision: UInt64
    public var categories: [TaskCategoryID: TaskCategory]
    public var labels: [TaskLabelID: TaskLabel]
    public var currentByChainID: [TaskChainID: CurrentTaskClassification]
    public var relationHistory: [ClassificationRelationHistoryEntry]
    public var categoryMerges: [TaskCategoryID: TaskCategoryMerge]
    public var labelMerges: [TaskLabelID: TaskLabelMerge]
    public var categoryDeletionTombstones: [TaskCategoryID: TaskCategoryDeletionTombstone]
    public var labelDeletionTombstones: [TaskLabelID: TaskLabelDeletionTombstone]
    public var snapshotsByTraceID: [DayTraceID: TraceClassificationSnapshot]
    public var snapshotEventsByTraceID: [DayTraceID: [TraceClassificationSnapshot]]
    public var committedReceiptsByInteractionID: [UUID: ClassificationReceipt]
    public var changeRecords: [ClassificationChangeRecord]

    public init(
        revision: UInt64 = 0,
        categories: [TaskCategoryID: TaskCategory] = [:],
        labels: [TaskLabelID: TaskLabel] = [:],
        currentByChainID: [TaskChainID: CurrentTaskClassification] = [:],
        relationHistory: [ClassificationRelationHistoryEntry] = [],
        categoryMerges: [TaskCategoryID: TaskCategoryMerge] = [:],
        labelMerges: [TaskLabelID: TaskLabelMerge] = [:],
        categoryDeletionTombstones: [TaskCategoryID: TaskCategoryDeletionTombstone] = [:],
        labelDeletionTombstones: [TaskLabelID: TaskLabelDeletionTombstone] = [:],
        snapshotsByTraceID: [DayTraceID: TraceClassificationSnapshot] = [:],
        snapshotEventsByTraceID: [DayTraceID: [TraceClassificationSnapshot]] = [:],
        committedReceiptsByInteractionID: [UUID: ClassificationReceipt] = [:],
        changeRecords: [ClassificationChangeRecord] = []
    ) {
        self.revision = revision
        self.categories = categories
        self.labels = labels
        self.currentByChainID = currentByChainID
        self.relationHistory = relationHistory
        self.categoryMerges = categoryMerges
        self.labelMerges = labelMerges
        self.categoryDeletionTombstones = categoryDeletionTombstones
        self.labelDeletionTombstones = labelDeletionTombstones
        self.snapshotsByTraceID = snapshotsByTraceID
        self.snapshotEventsByTraceID = snapshotEventsByTraceID
        self.committedReceiptsByInteractionID = committedReceiptsByInteractionID
        self.changeRecords = changeRecords
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let formatVersion = try container.decode(Int.self, forKey: .formatVersion)
        guard formatVersion == Self.currentFormatVersion else {
            throw DecodingError.dataCorruptedError(
                forKey: .formatVersion,
                in: container,
                debugDescription: "unsupported task classification state format version"
            )
        }
        self.init(
            revision: try container.decode(UInt64.self, forKey: .revision),
            categories: try container.decode(
                [TaskCategoryID: TaskCategory].self,
                forKey: .categories
            ),
            labels: try container.decode(
                [TaskLabelID: TaskLabel].self,
                forKey: .labels
            ),
            currentByChainID: try container.decode(
                [TaskChainID: CurrentTaskClassification].self,
                forKey: .currentByChainID
            ),
            relationHistory: try container.decode(
                [ClassificationRelationHistoryEntry].self,
                forKey: .relationHistory
            ),
            categoryMerges: try container.decode(
                [TaskCategoryID: TaskCategoryMerge].self,
                forKey: .categoryMerges
            ),
            labelMerges: try container.decode(
                [TaskLabelID: TaskLabelMerge].self,
                forKey: .labelMerges
            ),
            categoryDeletionTombstones: try container.decode(
                [TaskCategoryID: TaskCategoryDeletionTombstone].self,
                forKey: .categoryDeletionTombstones
            ),
            labelDeletionTombstones: try container.decode(
                [TaskLabelID: TaskLabelDeletionTombstone].self,
                forKey: .labelDeletionTombstones
            ),
            snapshotsByTraceID: try container.decode(
                [DayTraceID: TraceClassificationSnapshot].self,
                forKey: .snapshotsByTraceID
            ),
            snapshotEventsByTraceID: try container.decode(
                [DayTraceID: [TraceClassificationSnapshot]].self,
                forKey: .snapshotEventsByTraceID
            ),
            committedReceiptsByInteractionID: try container.decode(
                [UUID: ClassificationReceipt].self,
                forKey: .committedReceiptsByInteractionID
            ),
            changeRecords: try container.decode(
                [ClassificationChangeRecord].self,
                forKey: .changeRecords
            )
        )
        try validateIntegrity(codingPath: decoder.codingPath)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(Self.currentFormatVersion, forKey: .formatVersion)
        try container.encode(revision, forKey: .revision)
        try container.encode(categories, forKey: .categories)
        try container.encode(labels, forKey: .labels)
        try container.encode(currentByChainID, forKey: .currentByChainID)
        try container.encode(relationHistory, forKey: .relationHistory)
        try container.encode(categoryMerges, forKey: .categoryMerges)
        try container.encode(labelMerges, forKey: .labelMerges)
        try container.encode(categoryDeletionTombstones, forKey: .categoryDeletionTombstones)
        try container.encode(labelDeletionTombstones, forKey: .labelDeletionTombstones)
        try container.encode(snapshotsByTraceID, forKey: .snapshotsByTraceID)
        try container.encode(snapshotEventsByTraceID, forKey: .snapshotEventsByTraceID)
        try container.encode(committedReceiptsByInteractionID, forKey: .committedReceiptsByInteractionID)
        try container.encode(changeRecords, forKey: .changeRecords)
    }
}

struct ClassificationMutationContext {
    let source: ClassificationSource
    let decisionID: UUID?
    let revision: UInt64
    let changeRecordID: UUID
    let now: Date
}

struct PreparedClassificationLabelChoice {
    let choice: TaskLabelChoice
    let displayName: String
    let deduplicationKey: String
    let notice: ClassificationNotice?
}

public extension NoonmarkEngine {
    func classification(_ query: ClassificationQuery) throws -> ClassificationProjection {
        switch query {
        case let .task(chainID):
            guard chains[chainID] != nil else {
                throw NoonmarkError.notFound("task chain")
            }
            let current = classificationState.currentByChainID[chainID]
            let category = current?.categoryID.flatMap { id in
                classificationState.categories[id].map {
                    ClassificationItemProjection(
                        id: id.description,
                        name: $0.name,
                        colorHex: $0.colorHex,
                        presentationApproval: $0.presentationApproval
                    )
                }
            }
            let labels = (current?.labelIDs ?? [])
                .compactMap { id in
                    classificationState.labels[id].map {
                        ClassificationItemProjection(id: id.description, name: $0.name, colorHex: $0.colorHex)
                    }
                }
                .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
            return .task(
                TaskClassificationProjection(
                    chainID: chainID,
                    category: category,
                    labels: labels,
                    revision: classificationState.revision
                )
            )
        case let .history(traceID):
            guard traces[traceID] != nil else {
                throw NoonmarkError.notFound("day trace")
            }
            guard let snapshot = classificationState.snapshotsByTraceID[traceID] else {
                throw NoonmarkError.notFound("trace classification snapshot")
            }
            let eventSnapshots = classificationState.snapshotEventsByTraceID[traceID] ?? [snapshot]
            let events = eventSnapshots.map(classificationEventProjection)
            let latest = classificationEventProjection(snapshot)
            return .history(
                TraceClassificationProjection(
                    traceID: traceID,
                    category: latest.category,
                    labels: latest.labels,
                    capturedAt: latest.capturedAt,
                    events: events
                )
            )
        case .catalog:
            let historicalEvents = classificationHistoryEvents(in: classificationState)
            let categories = classificationState.categories.values.map { category in
                let historicalIDs = category.lifecycle == .active
                    ? categoryMergeFamily(convergingOn: category.id, in: classificationState)
                    : [category.id]
                return ClassificationCatalogItemProjection(
                    id: category.id.description,
                    name: category.name,
                    colorHex: category.colorHex,
                    lifecycle: category.lifecycle,
                    mergedIntoID: terminalCategoryID(for: category.id, in: classificationState)
                        .flatMap { $0 == category.id ? nil : $0.description },
                    currentUsageCount: classificationState.currentByChainID.values.count {
                        $0.categoryID == category.id
                    },
                    historicalUsageCount: historicalEvents.count {
                        $0.category.map { historicalIDs.contains($0.id) } ?? false
                    },
                    presentationApproval: category.presentationApproval
                )
            }
            let labels = classificationState.labels.values.map { label in
                let historicalIDs = label.lifecycle == .active
                    ? labelMergeFamily(convergingOn: label.id, in: classificationState)
                    : [label.id]
                return ClassificationCatalogItemProjection(
                    id: label.id.description,
                    name: label.name,
                    colorHex: label.colorHex,
                    lifecycle: label.lifecycle,
                    mergedIntoID: terminalLabelID(for: label.id, in: classificationState)
                        .flatMap { $0 == label.id ? nil : $0.description },
                    currentUsageCount: classificationState.currentByChainID.values.count {
                        $0.labelIDs.contains(label.id)
                    },
                    historicalUsageCount: historicalEvents.count { event in
                        event.labels.contains { historicalIDs.contains($0.id) }
                    }
                )
            }
            return .catalog(
                ClassificationCatalogProjection(
                    categories: sortedCatalogItems(categories),
                    labels: sortedCatalogItems(labels),
                    revision: classificationState.revision
                )
            )
        }
    }

    func replaceTaskCycleClassification(
        seriesID: TaskCycleSeriesID,
        category: TaskCategoryChoice?,
        labels: [TaskLabelChoice],
        today: LocalDate,
        interactionID: UUID = UUID(),
        now: Date = Date()
    ) throws -> [NoonmarkSnapshot] {
        let candidate = NoonmarkEngine(copying: self)
        let boundaries = try candidate
            .replaceTaskCycleClassificationInPlace(
                seriesID: seriesID,
                category: category,
                labels: labels,
                today: today,
                interactionID: interactionID,
                now: now
            )
        try candidate.snapshot().validateIntegrity()
        adoptState(from: candidate)
        return boundaries
    }

    private func replaceTaskCycleClassificationInPlace(
        seriesID: TaskCycleSeriesID,
        category: TaskCategoryChoice?,
        labels: [TaskLabelChoice],
        today: LocalDate,
        interactionID: UUID,
        now: Date
    ) throws -> [NoonmarkSnapshot] {
        guard let series = taskCycleSeries[seriesID] else {
            throw NoonmarkError.notFound("task cycle series")
        }
        try series.requireUnusedClassificationInteraction(interactionID)
        let memberChainIDs = series.lifecycle(on: today)
            .allowsPlanEditing
            ? taskCycleEditableMemberChainIDs(
                seriesID: seriesID,
                today: today
            )
            : []
        if let anchorChainID = memberChainIDs.first {
            return try replaceEditableTaskCycleClassification(
                seriesID: seriesID,
                memberChainIDs: memberChainIDs,
                anchorChainID: anchorChainID,
                category: category,
                labels: labels,
                interactionID: interactionID,
                now: now
            )
        }
        return try replaceTaskCycleParentClassification(
            series: series,
            category: category,
            labels: labels,
            decisionID: interactionID,
            now: now
        )
    }

    private func replaceEditableTaskCycleClassification(
        seriesID: TaskCycleSeriesID,
        memberChainIDs: [TaskChainID],
        anchorChainID: TaskChainID,
        category: TaskCategoryChoice?,
        labels: [TaskLabelChoice],
        interactionID: UUID,
        now: Date
    ) throws -> [NoonmarkSnapshot] {
        let plan = try prepareClassification(
            .setCurrent(
                TaskClassificationDraft(
                    chainID: anchorChainID,
                    category: category,
                    labels: labels
                )
            ),
            source: .userDirect,
            interactionID: interactionID,
            now: now
        )
        _ = try commitClassification(
            plan,
            confirmation: .user(
                confirming: plan,
                decisionID: interactionID
            ),
            now: now
        )
        var boundaries = [snapshot()]
        guard let classification = classificationState
            .currentByChainID[anchorChainID]
        else {
            throw NoonmarkError.invalidInput(
                "recurring task classification did not commit"
            )
        }
        try setTaskCycleTemplateClassification(
            seriesID: seriesID,
            categoryID: classification.categoryID,
            labelIDs: classification.labelIDs,
            source: .userDirect,
            interactionID: interactionID,
            decisionID: interactionID,
            now: now
        )
        for chainID in memberChainIDs where chainID != anchorChainID {
            if try inheritCurrentClassification(
                from: anchorChainID,
                to: chainID,
                now: now
            ) {
                boundaries.append(snapshot())
            }
        }
        boundaries[boundaries.index(before: boundaries.endIndex)] =
            snapshot()
        return boundaries
    }

    private func replaceTaskCycleParentClassification(
        series: TaskCycleSeries,
        category: TaskCategoryChoice?,
        labels: [TaskLabelChoice],
        decisionID: UUID,
        now: Date
    ) throws -> [NoonmarkSnapshot] {
        var boundaries: [NoonmarkSnapshot] = []
        let categoryID = try resolveTaskCycleParentCategory(
            category,
            currentID: series.categoryID,
            decisionID: decisionID,
            now: now,
            boundaries: &boundaries
        )
        var labelIDs: Set<TaskLabelID> = []
        for label in labels {
            labelIDs.insert(
                try resolveTaskCycleParentLabel(
                    label,
                    currentIDs: series.labelIDs,
                    decisionID: decisionID,
                    now: now,
                    boundaries: &boundaries
                )
            )
        }
        try setTaskCycleTemplateClassification(
            seriesID: series.id,
            categoryID: categoryID,
            labelIDs: labelIDs,
            source: .userDirect,
            interactionID: decisionID,
            decisionID: decisionID,
            now: now
        )
        if boundaries.isEmpty == false {
            boundaries[boundaries.index(before: boundaries.endIndex)] =
                snapshot()
        }
        return boundaries
    }

    private func resolveTaskCycleParentCategory(
        _ choice: TaskCategoryChoice?,
        currentID: TaskCategoryID?,
        decisionID: UUID,
        now: Date,
        boundaries: inout [NoonmarkSnapshot]
    ) throws -> TaskCategoryID? {
        guard let choice else { return nil }
        switch choice {
        case let .existing(id):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            guard category.lifecycle == .active || currentID == id else {
                throw NoonmarkError.invalidTransition(
                    "task category is not active"
                )
            }
            return id
        case let .new(name, colorHex):
            let normalizedName = try normalizedClassificationName(name)
            let key = classificationNameKey(normalizedName)
            if let existing = classificationState.categories.values
                .first(where: { $0.allCanonicalKeys.contains(key) })
            {
                guard existing.lifecycle == .active
                    || currentID == existing.id
                else {
                    throw NoonmarkError.invalidTransition(
                        "task category is not active"
                    )
                }
                return existing.id
            }
            let plan = try prepareClassification(
                .createCategory(
                    name: normalizedName,
                    colorHex: colorHex
                ),
                source: .userDirect,
                interactionID: UUID(),
                now: now
            )
            guard let rawID = plan.createdItemID else {
                throw NoonmarkError.invalidInput(
                    "classification plan is missing its created identity"
                )
            }
            _ = try commitClassification(
                plan,
                confirmation: .user(
                    confirming: plan,
                    decisionID: decisionID
                ),
                now: now
            )
            boundaries.append(snapshot())
            return TaskCategoryID(rawID)
        }
    }

    private func resolveTaskCycleParentLabel(
        _ choice: TaskLabelChoice,
        currentIDs: Set<TaskLabelID>,
        decisionID: UUID,
        now: Date,
        boundaries: inout [NoonmarkSnapshot]
    ) throws -> TaskLabelID {
        switch choice {
        case let .existing(id):
            guard let label = classificationState.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            guard label.lifecycle == .active || currentIDs.contains(id)
            else {
                throw NoonmarkError.invalidTransition(
                    "task label is not active"
                )
            }
            return id
        case let .new(name, colorHex):
            let normalizedName = try normalizedClassificationName(name)
            let key = classificationNameKey(normalizedName)
            if let existing = classificationState.labels.values
                .first(where: { $0.allCanonicalKeys.contains(key) })
            {
                guard existing.lifecycle == .active
                    || currentIDs.contains(existing.id)
                else {
                    throw NoonmarkError.invalidTransition(
                        "task label is not active"
                    )
                }
                return existing.id
            }
            let plan = try prepareClassification(
                .createLabel(
                    name: normalizedName,
                    colorHex: colorHex
                ),
                source: .userDirect,
                interactionID: UUID(),
                now: now
            )
            guard let rawID = plan.createdItemID else {
                throw NoonmarkError.invalidInput(
                    "classification plan is missing its created identity"
                )
            }
            _ = try commitClassification(
                plan,
                confirmation: .user(
                    confirming: plan,
                    decisionID: decisionID
                ),
                now: now
            )
            boundaries.append(snapshot())
            return TaskLabelID(rawID)
        }
    }

    func prepareClassification(
        _ intent: ClassificationIntent,
        source: ClassificationSource,
        interactionID: UUID,
        now _: Date = Date()
    ) throws -> ClassificationPlan {
        try requireClassificationIntentReferences(intent)
        let prepared = try preparedClassificationIntent(intent)
        let createdItemID: UUID? = switch prepared.intent {
        case .createCategory, .createLabel:
            UUID()
        default:
            nil
        }
        let plannedCreations = plannedCreationIDs(for: prepared.intent)
        let changes = try classificationPlanChanges(
            for: prepared.intent,
            createdItemID: createdItemID,
            plannedCreations: plannedCreations,
            notices: prepared.notices
        )
        let blockers = classificationPlanBlockers(for: prepared.intent)
        return try ClassificationPlan(
            baseRevision: classificationState.revision,
            interactionID: interactionID,
            intent: prepared.intent,
            source: source,
            changes: changes,
            createdItemID: createdItemID,
            plannedCreations: plannedCreations,
            notices: prepared.notices,
            blockers: blockers
        )
    }

    private func plannedCreationIDs(
        for intent: ClassificationIntent
    ) -> ClassificationPlannedCreations {
        guard case let .setCurrent(draft) = intent else {
            return ClassificationPlannedCreations()
        }
        let categoryID: TaskCategoryID? = switch draft.category {
        case .new:
            TaskCategoryID()
        case .existing, nil:
            nil
        }
        let labelIDs = draft.labels.reduce(into: [String: TaskLabelID]()) { result, choice in
            guard case let .new(name, _) = choice else { return }
            result[classificationKey(name)] = TaskLabelID()
        }
        return ClassificationPlannedCreations(
            categoryID: categoryID,
            labelIDsByCanonicalKey: labelIDs
        )
    }

    private func requireClassificationIntentReferences(
        _ intent: ClassificationIntent
    ) throws {
        switch intent {
        case .createCategory, .createLabel:
            return
        case let .mergeCategory(source, target):
            try requireCategoryIDs([source, target])
        case let .mergeLabel(source, target):
            try requireLabelIDs([source, target])
        case let .setCurrent(draft):
            guard chains[draft.chainID] != nil else {
                throw NoonmarkError.notFound("task chain")
            }
        case let .renameCategory(id, _),
             let .approveCategoryPresentation(id),
             let .archiveCategory(id),
             let .restoreCategory(id),
             let .hardDeleteCategory(id):
            try requireCategoryIDs([id])
        case let .renameLabel(id, _),
             let .archiveLabel(id),
             let .restoreLabel(id),
             let .hardDeleteLabel(id):
            try requireLabelIDs([id])
        }
    }

    private func requireCategoryIDs(_ ids: [TaskCategoryID]) throws {
        guard ids.allSatisfy({ classificationState.categories[$0] != nil }) else {
            throw NoonmarkError.notFound("task category")
        }
    }

    private func requireLabelIDs(_ ids: [TaskLabelID]) throws {
        guard ids.allSatisfy({ classificationState.labels[$0] != nil }) else {
            throw NoonmarkError.notFound("task label")
        }
    }

    func commitClassification(
        _ plan: ClassificationPlan,
        confirmation: ClassificationConfirmation,
        now: Date = Date()
    ) throws -> ClassificationReceipt {
        switch plan.source {
        case .userDirect, .zhulongSuggestion:
            break
        case .automaticAI, .inherited, .deterministicDomainAction:
            throw NoonmarkError.invalidInput(
                "classification source requires its bounded domain authority"
            )
        }
        guard confirmation.authorizes(plan) else {
            throw NoonmarkError.invalidInput("classification confirmation does not authorize this plan")
        }
        let decisionID = confirmation.decisionID
        try validateClassificationCommit(plan)
        if let committed = classificationState.committedReceiptsByInteractionID[plan.interactionID] {
            guard committed.planID == plan.id else {
                throw NoonmarkError.invalidInput("classification interaction id was already used")
            }
            guard committed.decisionID == decisionID else {
                throw NoonmarkError.invalidInput("classification interaction decision does not match")
            }
            guard let record = classificationState.changeRecords.first(where: {
                $0.id == committed.changeRecordID
            }), record.interactionID == plan.interactionID,
                record.planID == plan.id,
                record.planDigest == plan.digest,
                record.hasValidIntegrityDigest(),
                committed.changeRecordIntegrityDigest == record.integrityDigest
            else {
                throw NoonmarkError.invalidInput("classification receipt is missing its matching audit record")
            }
            return committed
        }
        guard let receipt = try applyClassificationCommit(
            plan,
            decisionID: decisionID,
            recordsUserReceipt: true,
            now: now
        ) else {
            throw NoonmarkError.invalidInput(
                "user-authorized classification commit did not create a receipt"
            )
        }
        return receipt
    }

    internal func commitDeterministicDomainClassification(
        _ plan: ClassificationPlan,
        now: Date = Date()
    ) throws {
        guard case .deterministicDomainAction = plan.source else {
            throw NoonmarkError.invalidInput(
                "deterministic classification commit requires a domain-action source"
            )
        }
        try validateClassificationCommit(plan)
        guard classificationState.committedReceiptsByInteractionID[plan.interactionID] == nil,
              classificationState.changeRecords.contains(where: {
                  $0.interactionID == plan.interactionID
              }) == false
        else {
            throw NoonmarkError.invalidInput("classification interaction id was already used")
        }
        _ = try applyClassificationCommit(
            plan,
            decisionID: nil,
            recordsUserReceipt: false,
            now: now
        )
    }

    func validateClassificationCommit(
        _ plan: ClassificationPlan
    ) throws {
        guard try plan.hasValidDigest() else {
            throw NoonmarkError.invalidInput("classification plan digest does not match its contents")
        }
        guard plan.blockers.isEmpty else {
            throw NoonmarkError.invalidTransition("classification plan has unresolved blockers")
        }
    }

    func applyClassificationCommit(
        _ plan: ClassificationPlan,
        decisionID: UUID?,
        recordsUserReceipt: Bool,
        enforcesBaseRevision: Bool = true,
        now: Date
    ) throws -> ClassificationReceipt? {
        guard enforcesBaseRevision == false
            || plan.baseRevision == classificationState.revision
        else {
            throw NoonmarkError.invalidTransition("classification plan is stale")
        }
        try validatePreparedClassificationPlan(plan)

        var next = classificationState
        let changeRecordID = UUID()
        let mutation = ClassificationMutationContext(
            source: plan.source,
            decisionID: decisionID,
            revision: next.revision + 1,
            changeRecordID: changeRecordID,
            now: now
        )
        let advancesRevision = try applyClassificationPlan(plan, mutation: mutation, to: &next)
        if advancesRevision {
            next.revision += 1
        }
        let changeRecord = ClassificationChangeRecord(
            id: changeRecordID,
            planID: plan.id,
            interactionID: plan.interactionID,
            source: plan.source,
            decisionID: decisionID,
            changes: plan.changes,
            notices: plan.notices,
            committedAt: now,
            revision: next.revision,
            planDigest: plan.digest
        )
        next.relationHistory = try ClassificationAuditCanonicalOrder.relationHistory(
            next.relationHistory
        )
        next.changeRecords = try ClassificationAuditCanonicalOrder.changeRecords(
            next.changeRecords + [changeRecord]
        )
        let receipt = ClassificationReceipt(
            planID: plan.id,
            revision: next.revision,
            notices: plan.notices,
            changeRecordID: changeRecord.id,
            decisionID: decisionID,
            changeRecordIntegrityDigest: changeRecord.integrityDigest
        )
        if recordsUserReceipt {
            next.committedReceiptsByInteractionID[plan.interactionID] = receipt
        }
        classificationState = next
        try normalizeTaskCycleTemplateClassification(
            after: plan.intent,
            source: plan.source,
            interactionID: plan.interactionID,
            decisionID: decisionID,
            now: now
        )
        return receipt
    }

    private func normalizeTaskCycleTemplateClassification(
        after intent: ClassificationIntent,
        source classificationSource: ClassificationSource,
        interactionID: UUID,
        decisionID: UUID?,
        now: Date
    ) throws {
        switch intent {
        case let .mergeCategory(source, target):
            let affectedIDs = taskCycleSeries.values
                .filter { $0.categoryID == source }
                .map(\.id)
            for id in affectedIDs {
                guard var series = taskCycleSeries[id] else { continue }
                try series.updateTemplateClassification(
                    categoryID: target,
                    labelIDs: series.labelIDs,
                    source: classificationSource,
                    interactionID: interactionID,
                    decisionID: decisionID,
                    now: now
                )
                storeTaskCycleSeries(series)
            }
        case let .mergeLabel(source, target):
            let affectedIDs = taskCycleSeries.values
                .filter { $0.labelIDs.contains(source) }
                .map(\.id)
            for id in affectedIDs {
                guard var series = taskCycleSeries[id] else { continue }
                var labels = series.labelIDs
                labels.remove(source)
                labels.insert(target)
                try series.updateTemplateClassification(
                    categoryID: series.categoryID,
                    labelIDs: labels,
                    source: classificationSource,
                    interactionID: interactionID,
                    decisionID: decisionID,
                    now: now
                )
                storeTaskCycleSeries(series)
            }
        default:
            break
        }
    }

    private func validatePreparedClassificationPlan(
        _ plan: ClassificationPlan
    ) throws {
        try validatePlannedCreationIDs(plan)
        let expectedChanges = try classificationPlanChanges(
            for: plan.intent,
            createdItemID: plan.createdItemID,
            plannedCreations: plan.plannedCreations,
            notices: plan.notices
        )
        let expectedBlockers = classificationPlanBlockers(for: plan.intent)
        guard expectedChanges == plan.changes,
              expectedBlockers == plan.blockers
        else {
            throw NoonmarkError.invalidInput("classification plan changes do not match its intent")
        }
    }

    private func validatePlannedCreationIDs(
        _ plan: ClassificationPlan
    ) throws {
        switch plan.intent {
        case .createCategory, .createLabel:
            guard plan.createdItemID != nil,
                  plan.plannedCreations.categoryID == nil,
                  plan.plannedCreations.labelIDsByCanonicalKey.isEmpty
            else {
                throw NoonmarkError.invalidInput("classification create plan identities are inconsistent")
            }
        case let .setCurrent(draft):
            try validateCurrentPlanCreationIDs(plan, draft: draft)
        default:
            guard plan.createdItemID == nil,
                  plan.plannedCreations.categoryID == nil,
                  plan.plannedCreations.labelIDsByCanonicalKey.isEmpty
            else {
                throw NoonmarkError.invalidInput("classification plan has unexpected created identities")
            }
        }
    }

    private func validateCurrentPlanCreationIDs(
        _ plan: ClassificationPlan,
        draft: TaskClassificationDraft
    ) throws {
        let createsCategory = if case .new = draft.category {
            true
        } else {
            false
        }
        let expectedLabelKeys = Set(draft.labels.compactMap { choice -> String? in
            guard case let .new(name, _) = choice else { return nil }
            return classificationKey(name)
        })
        let plannedLabelKeys = Set(plan.plannedCreations.labelIDsByCanonicalKey.keys)
        let plannedRawIDs = plan.plannedCreations.labelIDsByCanonicalKey.values.map(\.rawValue)
        guard (plan.plannedCreations.categoryID != nil) == createsCategory,
              plannedLabelKeys == expectedLabelKeys,
              Set(plannedRawIDs).count == plannedRawIDs.count,
              plan.createdItemID == nil
        else {
            throw NoonmarkError.invalidInput("classification current plan identities are inconsistent")
        }
        if let categoryRawID = plan.plannedCreations.categoryID?.rawValue {
            guard plannedRawIDs.contains(categoryRawID) == false else {
                throw NoonmarkError.invalidInput("classification planned identities are not unique")
            }
        }
    }

    private func applyClassificationPlan(
        _ plan: ClassificationPlan,
        mutation: ClassificationMutationContext,
        to state: inout TaskClassificationState
    ) throws -> Bool {
        switch plan.intent {
        case .createCategory, .createLabel:
            try applyClassificationCreate(plan, mutation: mutation, to: &state)
        case .mergeCategory, .mergeLabel:
            try applyClassificationMerge(plan.intent, mutation: mutation, to: &state)
        case .hardDeleteCategory, .hardDeleteLabel:
            try applyClassificationHardDelete(plan.intent, mutation: mutation, to: &state)
        case .setCurrent:
            try applyCurrentClassification(plan, mutation: mutation, to: &state)
        case .renameCategory, .renameLabel:
            return try applyClassificationRename(plan.intent, now: mutation.now, to: &state)
        case let .approveCategoryPresentation(id):
            return try approveCategoryPresentation(id, in: &state, now: mutation.now)
        case .archiveCategory, .restoreCategory, .archiveLabel, .restoreLabel:
            try applyClassificationLifecycle(plan.intent, now: mutation.now, to: &state)
        }
        return true
    }

    private func applyClassificationCreate(
        _ plan: ClassificationPlan,
        mutation: ClassificationMutationContext,
        to state: inout TaskClassificationState
    ) throws {
        guard let rawID = plan.createdItemID else {
            throw NoonmarkError.invalidInput("classification plan is missing its created identity")
        }
        switch plan.intent {
        case let .createCategory(name, colorHex):
            let id = TaskCategoryID(rawID)
            guard state.categories[id] == nil, state.categoryDeletionTombstones[id] == nil else {
                throw NoonmarkError.invalidInput("task category identity already exists")
            }
            state.categories[id] = TaskCategory(
                id: id,
                name: name,
                colorHex: colorHex,
                presentationApproval: mutation.source.categoryPresentationApproval,
                now: mutation.now
            )
        case let .createLabel(name, colorHex):
            let id = TaskLabelID(rawID)
            guard state.labels[id] == nil, state.labelDeletionTombstones[id] == nil else {
                throw NoonmarkError.invalidInput("task label identity already exists")
            }
            state.labels[id] = TaskLabel(id: id, name: name, colorHex: colorHex, now: mutation.now)
        default:
            throw NoonmarkError.invalidInput("classification plan is not a create intent")
        }
    }

    private func applyClassificationMerge(
        _ intent: ClassificationIntent,
        mutation: ClassificationMutationContext,
        to state: inout TaskClassificationState
    ) throws {
        switch intent {
        case let .mergeCategory(source, target):
            try mergeCategory(
                source,
                into: target,
                mutation: mutation,
                in: &state
            )
        case let .mergeLabel(source, target):
            try mergeLabel(
                source,
                into: target,
                mutation: mutation,
                in: &state
            )
        default:
            throw NoonmarkError.invalidInput("classification plan is not a merge intent")
        }
    }

    private func applyClassificationHardDelete(
        _ intent: ClassificationIntent,
        mutation: ClassificationMutationContext,
        to state: inout TaskClassificationState
    ) throws {
        switch intent {
        case let .hardDeleteCategory(id):
            try hardDeleteCategory(id, mutation: mutation, in: &state)
        case let .hardDeleteLabel(id):
            try hardDeleteLabel(id, mutation: mutation, in: &state)
        default:
            throw NoonmarkError.invalidInput("classification plan is not a hard-delete intent")
        }
    }

    private func applyCurrentClassification(
        _ plan: ClassificationPlan,
        mutation: ClassificationMutationContext,
        to state: inout TaskClassificationState
    ) throws {
        guard case let .setCurrent(draft) = plan.intent else {
            throw NoonmarkError.invalidInput("classification plan is not a current classification intent")
        }
        guard chains[draft.chainID] != nil else {
            throw NoonmarkError.notFound("task chain")
        }
        let categoryID = try resolve(
            draft.category,
            plannedID: plan.plannedCreations.categoryID,
            for: draft.chainID,
            in: &state,
            mutation: mutation
        )
        let labelIDs = try Set(draft.labels.map {
            let plannedID: TaskLabelID? = if case let .new(name, _) = $0 {
                plan.plannedCreations.labelIDsByCanonicalKey[classificationKey(name)]
            } else {
                nil
            }
            return try resolve(
                $0,
                plannedID: plannedID,
                for: draft.chainID,
                in: &state,
                now: mutation.now
            )
        })
        try validateCurrentRelationMutationTime(
            for: draft.chainID,
            at: mutation.now,
            in: state
        )
        replaceCurrentClassification(
            for: draft.chainID,
            categoryID: categoryID,
            labelIDs: labelIDs,
            mutation: mutation,
            in: &state
        )
    }

    private func applyClassificationRename(
        _ intent: ClassificationIntent,
        now: Date,
        to state: inout TaskClassificationState
    ) throws -> Bool {
        switch intent {
        case let .renameCategory(id, name):
            return try renameCategory(id, to: name, in: &state, now: now)
        case let .renameLabel(id, name):
            return try renameLabel(id, to: name, in: &state, now: now)
        default:
            throw NoonmarkError.invalidInput("classification plan is not a rename intent")
        }
    }

    private func applyClassificationLifecycle(
        _ intent: ClassificationIntent,
        now: Date,
        to state: inout TaskClassificationState
    ) throws {
        switch intent {
        case let .archiveCategory(id):
            try transitionCategory(id, from: .active, to: .archived, in: &state, now: now)
        case let .restoreCategory(id):
            try transitionCategory(id, from: .archived, to: .active, in: &state, now: now)
        case let .archiveLabel(id):
            try transitionLabel(id, from: .active, to: .archived, in: &state, now: now)
        case let .restoreLabel(id):
            try transitionLabel(id, from: .archived, to: .active, in: &state, now: now)
        default:
            throw NoonmarkError.invalidInput("classification plan is not a lifecycle intent")
        }
    }
}

extension NoonmarkEngine {
    func classificationPlanBlockers(
        for intent: ClassificationIntent
    ) -> [ClassificationBlocker] {
        switch intent {
        case let .hardDeleteCategory(id):
            let references = categoryReferenceSummary(id, in: classificationState)
            return references.isEmpty
                ? []
                : [.referencedItem(kind: .category, itemID: id.description, references: references)]
        case let .hardDeleteLabel(id):
            let references = labelReferenceSummary(id, in: classificationState)
            return references.isEmpty
                ? []
                : [.referencedItem(kind: .label, itemID: id.description, references: references)]
        default:
            return []
        }
    }

    func classificationPlanChanges(
        for intent: ClassificationIntent,
        createdItemID: UUID?,
        plannedCreations: ClassificationPlannedCreations,
        notices: [ClassificationNotice]
    ) throws -> [ClassificationPlanChange] {
        switch intent {
        case .createCategory, .createLabel:
            return [try createPlanChange(for: intent, createdItemID: createdItemID)]
        case .mergeCategory, .mergeLabel:
            return [try mergePlanChange(for: intent)]
        case .hardDeleteCategory, .hardDeleteLabel:
            return [try hardDeletePlanChange(for: intent)]
        case let .setCurrent(draft):
            return [
                .setCurrent(
                    chainID: draft.chainID,
                    before: currentClassificationPreview(for: draft.chainID),
                    after: try classificationPreview(
                        for: draft,
                        plannedCreations: plannedCreations,
                        notices: notices
                    )
                )
            ]
        case .renameCategory, .renameLabel:
            return [try renamePlanChange(for: intent)]
        case let .approveCategoryPresentation(id):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            return [
                .categoryPresentationApproval(
                    itemID: id.description,
                    name: category.name,
                    before: category.presentationApproval,
                    after: .userApproved
                )
            ]
        case .archiveCategory, .restoreCategory, .archiveLabel, .restoreLabel:
            return [try lifecyclePlanChange(for: intent)]
        }
    }

    private func createPlanChange(
        for intent: ClassificationIntent,
        createdItemID: UUID?
    ) throws -> ClassificationPlanChange {
        guard let createdItemID else {
            throw NoonmarkError.invalidInput("classification plan is missing its created identity")
        }
        switch intent {
        case let .createCategory(name, colorHex):
            return .create(
                kind: .category,
                itemID: TaskCategoryID(createdItemID).description,
                name: name,
                colorHex: colorHex
            )
        case let .createLabel(name, colorHex):
            return .create(
                kind: .label,
                itemID: TaskLabelID(createdItemID).description,
                name: name,
                colorHex: colorHex
            )
        default:
            throw NoonmarkError.invalidInput("classification plan is not a create intent")
        }
    }

    private func mergePlanChange(
        for intent: ClassificationIntent
    ) throws -> ClassificationPlanChange {
        switch intent {
        case let .mergeCategory(source, target):
            guard let sourceCategory = classificationState.categories[source],
                  let targetCategory = classificationState.categories[target]
            else {
                throw NoonmarkError.notFound("task category")
            }
            return .merge(
                kind: .category,
                sourceID: source.description,
                sourceName: sourceCategory.name,
                targetID: target.description,
                targetName: targetCategory.name,
                sourceLifecycle: sourceCategory.lifecycle,
                impact: categoryMergeImpact(source: source)
            )
        case let .mergeLabel(source, target):
            guard let sourceLabel = classificationState.labels[source],
                  let targetLabel = classificationState.labels[target]
            else {
                throw NoonmarkError.notFound("task label")
            }
            return .merge(
                kind: .label,
                sourceID: source.description,
                sourceName: sourceLabel.name,
                targetID: target.description,
                targetName: targetLabel.name,
                sourceLifecycle: sourceLabel.lifecycle,
                impact: labelMergeImpact(source: source, target: target)
            )
        default:
            throw NoonmarkError.invalidInput("classification plan is not a merge intent")
        }
    }

    private func hardDeletePlanChange(
        for intent: ClassificationIntent
    ) throws -> ClassificationPlanChange {
        switch intent {
        case let .hardDeleteCategory(id):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            return .hardDelete(
                kind: .category,
                itemID: id.description,
                name: category.name,
                lifecycle: category.lifecycle,
                verifiedReferences: categoryReferenceSummary(id, in: classificationState)
            )
        case let .hardDeleteLabel(id):
            guard let label = classificationState.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            return .hardDelete(
                kind: .label,
                itemID: id.description,
                name: label.name,
                lifecycle: label.lifecycle,
                verifiedReferences: labelReferenceSummary(id, in: classificationState)
            )
        default:
            throw NoonmarkError.invalidInput("classification plan is not a hard-delete intent")
        }
    }

    private func renamePlanChange(
        for intent: ClassificationIntent
    ) throws -> ClassificationPlanChange {
        switch intent {
        case let .renameCategory(id, name):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            return .rename(
                kind: .category,
                itemID: id.description,
                beforeName: category.name,
                afterName: name
            )
        case let .renameLabel(id, name):
            guard let label = classificationState.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            return .rename(kind: .label, itemID: id.description, beforeName: label.name, afterName: name)
        default:
            throw NoonmarkError.invalidInput("classification plan is not a rename intent")
        }
    }

    private func lifecyclePlanChange(
        for intent: ClassificationIntent
    ) throws -> ClassificationPlanChange {
        switch intent {
        case let .archiveCategory(id), let .restoreCategory(id):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            let target: ClassificationLifecycle = if case .archiveCategory = intent {
                .archived
            } else {
                .active
            }
            return .lifecycle(
                kind: .category,
                itemID: id.description,
                name: category.name,
                before: category.lifecycle,
                after: target
            )
        case let .archiveLabel(id), let .restoreLabel(id):
            guard let label = classificationState.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            let target: ClassificationLifecycle = if case .archiveLabel = intent {
                .archived
            } else {
                .active
            }
            return .lifecycle(
                kind: .label,
                itemID: id.description,
                name: label.name,
                before: label.lifecycle,
                after: target
            )
        default:
            throw NoonmarkError.invalidInput("classification plan is not a lifecycle intent")
        }
    }

    func currentClassificationPreview(for chainID: TaskChainID) -> ClassificationSelectionPreview {
        let current = classificationState.currentByChainID[chainID]
        let category = current?.categoryID.flatMap { id in
            classificationState.categories[id].map {
                ClassificationPlanItem(
                    id: id.description,
                    kind: .category,
                    name: $0.name,
                    colorHex: $0.colorHex,
                    resolution: .existing
                )
            }
        }
        let labels = (current?.labelIDs ?? []).compactMap { id in
            classificationState.labels[id].map {
                ClassificationPlanItem(
                    id: id.description,
                    kind: .label,
                    name: $0.name,
                    colorHex: $0.colorHex,
                    resolution: .existing
                )
            }
        }
        return ClassificationSelectionPreview(category: category, labels: sortedPlanItems(labels))
    }

    func classificationPreview(
        for draft: TaskClassificationDraft,
        plannedCreations: ClassificationPlannedCreations,
        notices: [ClassificationNotice]
    ) throws -> ClassificationSelectionPreview {
        let category: ClassificationPlanItem? = try draft.category.map { choice in
            switch choice {
            case let .existing(id):
                guard let category = classificationState.categories[id] else {
                    throw NoonmarkError.notFound("task category")
                }
                return ClassificationPlanItem(
                    id: id.description,
                    kind: .category,
                    name: category.name,
                    colorHex: category.colorHex,
                    resolution: reusedResolution(for: .category, currentName: category.name, notices: notices)
                )
            case let .new(name, colorHex):
                guard let id = plannedCreations.categoryID else {
                    throw NoonmarkError.invalidInput("classification plan is missing its planned category identity")
                }
                return ClassificationPlanItem(
                    id: id.description,
                    kind: .category,
                    name: name,
                    colorHex: colorHex,
                    resolution: .new
                )
            }
        }
        let labels = try draft.labels.map { choice in
            switch choice {
            case let .existing(id):
                guard let label = classificationState.labels[id] else {
                    throw NoonmarkError.notFound("task label")
                }
                return ClassificationPlanItem(
                    id: id.description,
                    kind: .label,
                    name: label.name,
                    colorHex: label.colorHex,
                    resolution: reusedResolution(for: .label, currentName: label.name, notices: notices)
                )
            case let .new(name, colorHex):
                guard let id = plannedCreations.labelIDsByCanonicalKey[classificationKey(name)] else {
                    throw NoonmarkError.invalidInput("classification plan is missing its planned label identity")
                }
                return ClassificationPlanItem(
                    id: id.description,
                    kind: .label,
                    name: name,
                    colorHex: colorHex,
                    resolution: .new
                )
            }
        }
        return ClassificationSelectionPreview(category: category, labels: sortedPlanItems(labels))
    }

    func reusedResolution(
        for kind: ClassificationItemKind,
        currentName: String,
        notices: [ClassificationNotice]
    ) -> ClassificationPlanItemResolution {
        for notice in notices {
            guard case let .existingItemReused(noticeKind, _, noticeCurrentName, historicalAlias) = notice,
                  noticeKind == kind,
                  noticeCurrentName == currentName
            else { continue }
            return historicalAlias ? .historicalAlias : .existing
        }
        return .existing
    }

    func sortedPlanItems(_ items: [ClassificationPlanItem]) -> [ClassificationPlanItem] {
        items.sorted {
            let lhsKey = ClassificationNameCanonicalizer.canonicalKey($0.name)
            let rhsKey = ClassificationNameCanonicalizer.canonicalKey($1.name)
            if lhsKey != rhsKey {
                return lhsKey < rhsKey
            }
            return ($0.id ?? "") < ($1.id ?? "")
        }
    }

    func sortedCatalogItems(
        _ items: [ClassificationCatalogItemProjection]
    ) -> [ClassificationCatalogItemProjection] {
        items.sorted {
            let lhsKey = ClassificationNameCanonicalizer.canonicalKey($0.name)
            let rhsKey = ClassificationNameCanonicalizer.canonicalKey($1.name)
            if lhsKey != rhsKey {
                return lhsKey < rhsKey
            }
            return $0.id < $1.id
        }
    }

    func classificationEventProjection(
        _ snapshot: TraceClassificationSnapshot
    ) -> TraceClassificationEventProjection {
        let category = snapshot.category.map {
            ClassificationItemProjection(
                id: $0.id.description,
                name: $0.name,
                colorHex: $0.colorHex,
                presentationApproval: $0.presentationApproval
            )
        }
        let labels = snapshot.labels.map {
            ClassificationItemProjection(id: $0.id.description, name: $0.name, colorHex: $0.colorHex)
        }
        return TraceClassificationEventProjection(
            id: snapshot.id,
            status: snapshot.status,
            category: category,
            labels: labels,
            capturedAt: snapshot.capturedAt
        )
    }

    func categoryMergeImpact(source: TaskCategoryID) -> ClassificationMergeImpact {
        let currentChainIDs = classificationState.currentByChainID.compactMap { chainID, current in
            current.categoryID == source ? chainID : nil
        }
        .sorted { $0.description < $1.description }
        let historicalIDs = categoryMergeFamily(convergingOn: source, in: classificationState)
        let historicalEvents = classificationHistoryEvents(in: classificationState).compactMap { event in
            event.category.map { historicalIDs.contains($0.id) } == true ? (event.traceID, event.id) : nil
        }
        return ClassificationMergeImpact(
            currentChainIDs: currentChainIDs,
            historicalTraceIDs: Set(historicalEvents.map(\.0)).sorted { $0.description < $1.description },
            historicalEventIDs: historicalEvents.map(\.1).sorted { $0.uuidString < $1.uuidString },
            migratedRelationCount: currentChainIDs.count,
            deduplicatedRelationCount: 0
        )
    }

    func labelMergeImpact(source: TaskLabelID, target: TaskLabelID) -> ClassificationMergeImpact {
        let affected = classificationState.currentByChainID.compactMap { chainID, current in
            current.labelIDs.contains(source) ? (chainID, current.labelIDs.contains(target)) : nil
        }
        .sorted { $0.0.description < $1.0.description }
        let historicalIDs = labelMergeFamily(convergingOn: source, in: classificationState)
        let historicalEvents = classificationHistoryEvents(in: classificationState).compactMap { event in
            event.labels.contains(where: { historicalIDs.contains($0.id) }) ? (event.traceID, event.id) : nil
        }
        return ClassificationMergeImpact(
            currentChainIDs: affected.map(\.0),
            historicalTraceIDs: Set(historicalEvents.map(\.0)).sorted { $0.description < $1.description },
            historicalEventIDs: historicalEvents.map(\.1).sorted { $0.uuidString < $1.uuidString },
            migratedRelationCount: affected.count,
            deduplicatedRelationCount: affected.count(where: \.1)
        )
    }

    func resolve(
        _ choice: TaskCategoryChoice?,
        plannedID: TaskCategoryID?,
        for chainID: TaskChainID,
        in state: inout TaskClassificationState,
        mutation: ClassificationMutationContext
    ) throws -> TaskCategoryID? {
        guard let choice else { return nil }
        switch choice {
        case let .existing(id):
            guard let category = state.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            guard category.lifecycle == .active
                || state.currentByChainID[chainID]?.categoryID == id
            else {
                throw NoonmarkError.invalidTransition("task category is not active")
            }
            return id
        case let .new(name, colorHex):
            let normalizedName = try normalizedClassificationName(name)
            if let existing = state.categories.values.first(where: {
                $0.allCanonicalKeys.contains(classificationNameKey(normalizedName))
            }) {
                guard existing.lifecycle == .active
                    || state.currentByChainID[chainID]?.categoryID == existing.id
                else {
                    throw NoonmarkError.invalidTransition("task category is not active")
                }
                return existing.id
            }
            guard let plannedID,
                  state.categories[plannedID] == nil,
                  state.categoryDeletionTombstones[plannedID] == nil
            else {
                throw NoonmarkError.invalidInput("task category planned identity is unavailable")
            }
            let category = TaskCategory(
                id: plannedID,
                name: normalizedName,
                colorHex: colorHex,
                presentationApproval: mutation.source.categoryPresentationApproval,
                now: mutation.now
            )
            state.categories[category.id] = category
            return category.id
        }
    }

    func resolve(
        _ choice: TaskLabelChoice,
        plannedID: TaskLabelID?,
        for chainID: TaskChainID,
        in state: inout TaskClassificationState,
        now: Date
    ) throws -> TaskLabelID {
        switch choice {
        case let .existing(id):
            guard let label = state.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            guard label.lifecycle == .active
                || state.currentByChainID[chainID]?.labelIDs.contains(id) == true
            else {
                throw NoonmarkError.invalidTransition("task label is not active")
            }
            return id
        case let .new(name, colorHex):
            let normalizedName = try normalizedClassificationName(name)
            if let existing = state.labels.values.first(where: {
                $0.allCanonicalKeys.contains(classificationNameKey(normalizedName))
            }) {
                guard existing.lifecycle == .active
                    || state.currentByChainID[chainID]?.labelIDs.contains(existing.id) == true
                else {
                    throw NoonmarkError.invalidTransition("task label is not active")
                }
                return existing.id
            }
            guard let plannedID,
                  state.labels[plannedID] == nil,
                  state.labelDeletionTombstones[plannedID] == nil
            else {
                throw NoonmarkError.invalidInput("task label planned identity is unavailable")
            }
            let label = TaskLabel(id: plannedID, name: normalizedName, colorHex: colorHex, now: now)
            state.labels[label.id] = label
            return label.id
        }
    }

    func normalizedClassificationName(_ name: String) throws -> String {
        let normalized = ClassificationNameCanonicalizer.displayName(name)
        guard normalized.isEmpty == false else {
            throw NoonmarkError.invalidInput("classification name cannot be empty")
        }
        guard ClassificationNameCanonicalizer.canonicalKey(normalized).isEmpty == false else {
            throw NoonmarkError.invalidInput("classification name has no searchable Unicode content")
        }
        return normalized
    }

    func classificationKey(_ name: String) -> String {
        ClassificationNameCanonicalizer.canonicalKey(name)
    }

    private func classificationNameKey(_ name: String) -> VersionedClassificationNameKey {
        VersionedClassificationNameKey(
            algorithmVersion: ClassificationNameCanonicalizer.algorithmVersion,
            value: classificationKey(name)
        )
    }

    func preparedClassificationIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case .createCategory, .createLabel:
            return try preparedCreateIntent(intent)
        case .mergeCategory, .mergeLabel:
            return try preparedMergeIntent(intent)
        case .hardDeleteCategory, .hardDeleteLabel:
            return try preparedHardDeleteIntent(intent)
        case let .setCurrent(draft):
            return try preparedSetCurrentIntent(draft)
        case .renameCategory, .renameLabel:
            return try preparedRenameIntent(intent)
        case let .approveCategoryPresentation(id):
            guard let category = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            guard category.lifecycle != .merged else {
                throw NoonmarkError.invalidTransition("merged task category cannot be approved")
            }
            guard category.presentationApproval == .pendingAIReview else {
                throw NoonmarkError.invalidTransition("task category presentation is already approved")
            }
            return (intent, [])
        case .archiveCategory, .restoreCategory, .archiveLabel, .restoreLabel:
            return try preparedLifecycleIntent(intent)
        }
    }

    private func preparedCreateIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case let .createCategory(name, colorHex):
            let displayName = try normalizedClassificationName(name)
            let key = classificationNameKey(displayName)
            guard classificationState.categories.values.contains(where: {
                $0.allCanonicalKeys.contains(key)
            }) == false else {
                throw NoonmarkError.invalidInput("task category name already exists")
            }
            return (.createCategory(name: displayName, colorHex: colorHex), [])
        case let .createLabel(name, colorHex):
            let displayName = try normalizedClassificationName(name)
            let key = classificationNameKey(displayName)
            guard classificationState.labels.values.contains(where: {
                $0.allCanonicalKeys.contains(key)
            }) == false else {
                throw NoonmarkError.invalidInput("task label name already exists")
            }
            return (.createLabel(name: displayName, colorHex: colorHex), [])
        default:
            throw NoonmarkError.invalidInput("classification intent is not a create intent")
        }
    }

    private func preparedMergeIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case let .mergeCategory(source, target):
            try validateCategoryMerge(source: source, target: target)
            return (.mergeCategory(source: source, into: target), [])
        case let .mergeLabel(source, target):
            try validateLabelMerge(source: source, target: target)
            return (.mergeLabel(source: source, into: target), [])
        default:
            throw NoonmarkError.invalidInput("classification intent is not a merge intent")
        }
    }

    private func validateCategoryMerge(source: TaskCategoryID, target: TaskCategoryID) throws {
        guard source != target else {
            throw NoonmarkError.invalidInput("task category cannot be merged into itself")
        }
        guard let sourceCategory = classificationState.categories[source],
              let targetCategory = classificationState.categories[target]
        else {
            throw NoonmarkError.notFound("task category")
        }
        guard sourceCategory.lifecycle == .active || sourceCategory.lifecycle == .archived,
              classificationState.categoryMerges[source] == nil
        else {
            throw NoonmarkError.invalidTransition("task category already has a merge target")
        }
        guard targetCategory.lifecycle == .active else {
            throw NoonmarkError.invalidTransition("merge target task category is not active")
        }
    }

    private func validateLabelMerge(source: TaskLabelID, target: TaskLabelID) throws {
        guard source != target else {
            throw NoonmarkError.invalidInput("task label cannot be merged into itself")
        }
        guard let sourceLabel = classificationState.labels[source],
              let targetLabel = classificationState.labels[target]
        else {
            throw NoonmarkError.notFound("task label")
        }
        guard sourceLabel.lifecycle == .active || sourceLabel.lifecycle == .archived,
              classificationState.labelMerges[source] == nil
        else {
            throw NoonmarkError.invalidTransition("task label already has a merge target")
        }
        guard targetLabel.lifecycle == .active else {
            throw NoonmarkError.invalidTransition("merge target task label is not active")
        }
    }

    private func preparedHardDeleteIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case let .hardDeleteCategory(id):
            try validateHardDeleteCategory(id)
            return (.hardDeleteCategory(id), [])
        case let .hardDeleteLabel(id):
            try validateHardDeleteLabel(id)
            return (.hardDeleteLabel(id), [])
        default:
            throw NoonmarkError.invalidInput("classification intent is not a hard-delete intent")
        }
    }

    private func validateHardDeleteCategory(_ id: TaskCategoryID) throws {
        guard let category = classificationState.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle == .active || category.lifecycle == .archived else {
            throw NoonmarkError.invalidTransition("merged task category cannot be hard deleted")
        }
    }

    private func validateHardDeleteLabel(_ id: TaskLabelID) throws {
        guard let label = classificationState.labels[id] else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle == .active || label.lifecycle == .archived else {
            throw NoonmarkError.invalidTransition("merged task label cannot be hard deleted")
        }
    }

    private func preparedSetCurrentIntent(
        _ draft: TaskClassificationDraft
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        let current = classificationState.currentByChainID[draft.chainID]
        var notices: [ClassificationNotice] = []
        let category = try preparedCategoryChoice(draft.category, current: current, notices: &notices)
        let labels = try preparedLabelChoices(draft.labels, current: current, notices: &notices)
        return (
            .setCurrent(
                TaskClassificationDraft(
                    chainID: draft.chainID,
                    category: category,
                    labels: labels
                )
            ),
            notices
        )
    }

    private func preparedCategoryChoice(
        _ choice: TaskCategoryChoice?,
        current: CurrentTaskClassification?,
        notices: inout [ClassificationNotice]
    ) throws -> TaskCategoryChoice? {
        switch choice {
        case let .new(name, colorHex):
            return try preparedNewCategoryChoice(
                name: name,
                colorHex: colorHex,
                current: current,
                notices: &notices
            )
        case let .existing(id):
            guard let existing = classificationState.categories[id] else {
                throw NoonmarkError.notFound("task category")
            }
            guard existing.lifecycle == .active || current?.categoryID == id else {
                throw NoonmarkError.invalidTransition("task category is not active")
            }
            return .existing(id)
        case nil:
            return nil
        }
    }

    private func preparedNewCategoryChoice(
        name: String,
        colorHex: String,
        current: CurrentTaskClassification?,
        notices: inout [ClassificationNotice]
    ) throws -> TaskCategoryChoice {
        let displayName = try normalizedClassificationName(name)
        let key = classificationNameKey(displayName)
        guard let existing = classificationState.categories.values.first(where: {
            $0.allCanonicalKeys.contains(key)
        }) else {
            return .new(name: displayName, colorHex: colorHex)
        }
        guard existing.lifecycle == .active || current?.categoryID == existing.id else {
            throw NoonmarkError.invalidTransition("task category is not active")
        }
        notices.append(
            .existingItemReused(
                kind: .category,
                inputName: displayName,
                currentName: existing.name,
                matchedHistoricalAlias: key != currentNameKey(for: existing)
            )
        )
        return .existing(existing.id)
    }

    private func preparedLabelChoices(
        _ choices: [TaskLabelChoice],
        current: CurrentTaskClassification?,
        notices: inout [ClassificationNotice]
    ) throws -> [TaskLabelChoice] {
        var seenKeys: Set<String> = []
        var prepared: [TaskLabelChoice] = []
        for choice in choices {
            let candidate = try preparedLabelChoice(choice, current: current)
            if let notice = candidate.notice {
                notices.append(notice)
            }
            if seenKeys.insert(candidate.deduplicationKey).inserted {
                prepared.append(candidate.choice)
            } else {
                notices.append(.duplicateLabelCollapsed(name: candidate.displayName))
            }
        }
        return prepared
    }

    private func preparedLabelChoice(
        _ choice: TaskLabelChoice,
        current: CurrentTaskClassification?
    ) throws -> PreparedClassificationLabelChoice {
        switch choice {
        case let .new(name, colorHex):
            return try preparedNewLabelChoice(name: name, colorHex: colorHex, current: current)
        case let .existing(id):
            guard let existing = classificationState.labels[id] else {
                throw NoonmarkError.notFound("task label")
            }
            guard existing.lifecycle == .active || current?.labelIDs.contains(id) == true else {
                throw NoonmarkError.invalidTransition("task label is not active")
            }
            return PreparedClassificationLabelChoice(
                choice: .existing(id),
                displayName: existing.name,
                deduplicationKey: "id:\(id.description)",
                notice: nil
            )
        }
    }

    private func preparedNewLabelChoice(
        name: String,
        colorHex: String,
        current: CurrentTaskClassification?
    ) throws -> PreparedClassificationLabelChoice {
        let displayName = try normalizedClassificationName(name)
        let key = classificationNameKey(displayName)
        guard let existing = classificationState.labels.values.first(where: {
            $0.allCanonicalKeys.contains(key)
        }) else {
            return PreparedClassificationLabelChoice(
                choice: .new(name: displayName, colorHex: colorHex),
                displayName: displayName,
                deduplicationKey: "canonical:\(key.algorithmVersion):\(key.value)",
                notice: nil
            )
        }
        guard existing.lifecycle == .active || current?.labelIDs.contains(existing.id) == true else {
            throw NoonmarkError.invalidTransition("task label is not active")
        }
        return PreparedClassificationLabelChoice(
            choice: .existing(existing.id),
            displayName: displayName,
            deduplicationKey: "id:\(existing.id.description)",
            notice: .existingItemReused(
                kind: .label,
                inputName: displayName,
                currentName: existing.name,
                matchedHistoricalAlias: key != currentNameKey(for: existing)
            )
        )
    }

    private func currentNameKey(for category: TaskCategory) -> VersionedClassificationNameKey {
        VersionedClassificationNameKey(
            algorithmVersion: category.canonicalKeyVersion,
            value: category.canonicalKey
        )
    }

    private func currentNameKey(for label: TaskLabel) -> VersionedClassificationNameKey {
        VersionedClassificationNameKey(
            algorithmVersion: label.canonicalKeyVersion,
            value: label.canonicalKey
        )
    }

    private func preparedRenameIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case let .renameCategory(id, name):
            return (try preparedCategoryRename(id: id, name: name), [])
        case let .renameLabel(id, name):
            return (try preparedLabelRename(id: id, name: name), [])
        default:
            throw NoonmarkError.invalidInput("classification intent is not a rename intent")
        }
    }

    private func preparedCategoryRename(id: TaskCategoryID, name: String) throws -> ClassificationIntent {
        let normalizedName = try normalizedClassificationName(name)
        let key = classificationNameKey(normalizedName)
        guard let category = classificationState.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle != .merged else {
            throw NoonmarkError.invalidTransition("merged task category cannot be renamed")
        }
        guard classificationState.categories.values.contains(where: {
            $0.id != id && $0.allCanonicalKeys.contains(key)
        }) == false else {
            throw NoonmarkError.invalidInput("task category name already exists")
        }
        return .renameCategory(id, to: normalizedName)
    }

    private func preparedLabelRename(id: TaskLabelID, name: String) throws -> ClassificationIntent {
        let normalizedName = try normalizedClassificationName(name)
        let key = classificationNameKey(normalizedName)
        guard let label = classificationState.labels[id] else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle != .merged else {
            throw NoonmarkError.invalidTransition("merged task label cannot be renamed")
        }
        guard classificationState.labels.values.contains(where: {
            $0.id != id && $0.allCanonicalKeys.contains(key)
        }) == false else {
            throw NoonmarkError.invalidInput("task label name already exists")
        }
        return .renameLabel(id, to: normalizedName)
    }

    private func preparedLifecycleIntent(
        _ intent: ClassificationIntent
    ) throws -> (intent: ClassificationIntent, notices: [ClassificationNotice]) {
        switch intent {
        case let .archiveCategory(id):
            try requireCategoryLifecycle(id, is: .active)
            guard classificationState.categoryMerges.values.contains(where: { $0.targetID == id }) == false else {
                throw NoonmarkError.invalidTransition("merge target task category must remain active")
            }
        case let .restoreCategory(id):
            try requireCategoryLifecycle(id, is: .archived)
        case let .archiveLabel(id):
            try requireLabelLifecycle(id, is: .active)
            guard classificationState.labelMerges.values.contains(where: { $0.targetID == id }) == false else {
                throw NoonmarkError.invalidTransition("merge target task label must remain active")
            }
        case let .restoreLabel(id):
            try requireLabelLifecycle(id, is: .archived)
        default:
            throw NoonmarkError.invalidInput("classification intent is not a lifecycle intent")
        }
        return (intent, [])
    }

    func renameCategory(
        _ id: TaskCategoryID,
        to name: String,
        in state: inout TaskClassificationState,
        now: Date
    ) throws -> Bool {
        guard var category = state.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle != .merged else {
            throw NoonmarkError.invalidTransition("merged task category cannot be renamed")
        }
        guard category.name != name else { return false }
        guard now >= category.updatedAt,
              let currentVersion = category.nameVersions.last,
              now > currentVersion.validFrom
        else {
            throw NoonmarkError.invalidTransition("task category rename time moved backwards")
        }
        if var previous = category.nameVersions.popLast() {
            previous.validUntil = now
            category.nameVersions.append(previous)
        }
        let canonicalKey = classificationKey(name)
        category.name = name
        category.canonicalKey = canonicalKey
        category.canonicalKeyVersion = ClassificationNameCanonicalizer.algorithmVersion
        category.presentationApproval = .userApproved
        category.nameVersions.append(
            ClassificationNameVersion(name: name, canonicalKey: canonicalKey, validFrom: now)
        )
        category.updatedAt = now
        state.categories[id] = category
        return true
    }

    func approveCategoryPresentation(
        _ id: TaskCategoryID,
        in state: inout TaskClassificationState,
        now: Date
    ) throws -> Bool {
        guard var category = state.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle != .merged else {
            throw NoonmarkError.invalidTransition("merged task category cannot be approved")
        }
        guard category.presentationApproval != .userApproved else { return false }
        guard now >= category.updatedAt else {
            throw NoonmarkError.invalidTransition("task category approval time moved backwards")
        }
        category.presentationApproval = .userApproved
        category.updatedAt = now
        state.categories[id] = category
        return true
    }

    func requireCategoryLifecycle(
        _ id: TaskCategoryID,
        is lifecycle: ClassificationLifecycle
    ) throws {
        guard let category = classificationState.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle == lifecycle else {
            throw NoonmarkError.invalidTransition("task category lifecycle is not \(lifecycle.rawValue)")
        }
    }

    func transitionCategory(
        _ id: TaskCategoryID,
        from expected: ClassificationLifecycle,
        to lifecycle: ClassificationLifecycle,
        in state: inout TaskClassificationState,
        now: Date
    ) throws {
        guard var category = state.categories[id] else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle == expected else {
            throw NoonmarkError.invalidTransition("task category lifecycle changed")
        }
        guard now >= category.updatedAt else {
            throw NoonmarkError.invalidTransition("task category lifecycle time moved backwards")
        }
        let isIncomingMergeTarget = state.categoryMerges.values.contains { $0.targetID == id }
        if lifecycle == .archived, isIncomingMergeTarget {
            throw NoonmarkError.invalidTransition("merge target task category must remain active")
        }
        category.lifecycle = lifecycle
        category.updatedAt = now
        state.categories[id] = category
    }

    func renameLabel(
        _ id: TaskLabelID,
        to name: String,
        in state: inout TaskClassificationState,
        now: Date
    ) throws -> Bool {
        guard var label = state.labels[id] else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle != .merged else {
            throw NoonmarkError.invalidTransition("merged task label cannot be renamed")
        }
        guard label.name != name else { return false }
        guard now >= label.updatedAt,
              let currentVersion = label.nameVersions.last,
              now > currentVersion.validFrom
        else {
            throw NoonmarkError.invalidTransition("task label rename time moved backwards")
        }
        if var previous = label.nameVersions.popLast() {
            previous.validUntil = now
            label.nameVersions.append(previous)
        }
        let canonicalKey = classificationKey(name)
        label.name = name
        label.canonicalKey = canonicalKey
        label.canonicalKeyVersion = ClassificationNameCanonicalizer.algorithmVersion
        label.nameVersions.append(
            ClassificationNameVersion(name: name, canonicalKey: canonicalKey, validFrom: now)
        )
        label.updatedAt = now
        state.labels[id] = label
        return true
    }

    func requireLabelLifecycle(
        _ id: TaskLabelID,
        is lifecycle: ClassificationLifecycle
    ) throws {
        guard let label = classificationState.labels[id] else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle == lifecycle else {
            throw NoonmarkError.invalidTransition("task label lifecycle is not \(lifecycle.rawValue)")
        }
    }

    func transitionLabel(
        _ id: TaskLabelID,
        from expected: ClassificationLifecycle,
        to lifecycle: ClassificationLifecycle,
        in state: inout TaskClassificationState,
        now: Date
    ) throws {
        guard var label = state.labels[id] else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle == expected else {
            throw NoonmarkError.invalidTransition("task label lifecycle changed")
        }
        guard now >= label.updatedAt else {
            throw NoonmarkError.invalidTransition("task label lifecycle time moved backwards")
        }
        let isIncomingMergeTarget = state.labelMerges.values.contains { $0.targetID == id }
        if lifecycle == .archived, isIncomingMergeTarget {
            throw NoonmarkError.invalidTransition("merge target task label must remain active")
        }
        label.lifecycle = lifecycle
        label.updatedAt = now
        state.labels[id] = label
    }

    func categoryReferenceSummary(
        _ id: TaskCategoryID,
        in state: TaskClassificationState
    ) -> ClassificationReferenceSummary {
        let historicalEventIDs = Set(classificationHistoryEvents(in: state).compactMap { event in
            event.category?.id == id ? event.id : nil
        })
        return ClassificationReferenceSummary(
            currentRelationCount:
            state.currentByChainID.values.count { $0.categoryID == id }
                + taskCycleSeries.values.count { $0.categoryID == id },
            relationHistoryCount: state.relationHistory.count {
                $0.kind == .category && $0.itemID == id.description
            } + taskCycleSeries.values.count {
                $0.categoryID != id
                    && $0.classificationRevisions.contains {
                        $0.categoryID == id
                    }
            },
            historicalEventCount: historicalEventIDs.count,
            mergeSourceCount: state.categoryMerges[id] == nil ? 0 : 1,
            mergeTargetCount: state.categoryMerges.values.count { $0.targetID == id }
        )
    }

    func labelReferenceSummary(
        _ id: TaskLabelID,
        in state: TaskClassificationState
    ) -> ClassificationReferenceSummary {
        let historicalEventIDs = Set(classificationHistoryEvents(in: state).compactMap { event in
            event.labels.contains(where: { $0.id == id }) ? event.id : nil
        })
        return ClassificationReferenceSummary(
            currentRelationCount:
            state.currentByChainID.values.count {
                $0.labelIDs.contains(id)
            }
                + taskCycleSeries.values.count {
                    $0.labelIDs.contains(id)
                },
            relationHistoryCount: state.relationHistory.count {
                $0.kind == .label && $0.itemID == id.description
            } + taskCycleSeries.values.count {
                $0.labelIDs.contains(id) == false
                    && $0.classificationRevisions.contains {
                        $0.labelIDs.contains(id)
                    }
            },
            historicalEventCount: historicalEventIDs.count,
            mergeSourceCount: state.labelMerges[id] == nil ? 0 : 1,
            mergeTargetCount: state.labelMerges.values.count { $0.targetID == id }
        )
    }

    func hardDeleteCategory(
        _ id: TaskCategoryID,
        mutation: ClassificationMutationContext,
        in state: inout TaskClassificationState
    ) throws {
        guard let category = state.categories[id], state.categoryDeletionTombstones[id] == nil else {
            throw NoonmarkError.notFound("task category")
        }
        guard category.lifecycle == .active || category.lifecycle == .archived else {
            throw NoonmarkError.invalidTransition("merged task category cannot be hard deleted")
        }
        guard categoryReferenceSummary(id, in: state).isEmpty else {
            throw NoonmarkError.invalidTransition("used task category cannot be hard deleted")
        }
        guard mutation.now >= category.updatedAt else {
            throw NoonmarkError.invalidTransition("task category deletion time moved backwards")
        }
        state.categories[id] = nil
        state.categoryDeletionTombstones[id] = TaskCategoryDeletionTombstone(
            itemID: id,
            deletedAt: mutation.now,
            revision: mutation.revision,
            changeRecordID: mutation.changeRecordID
        )
    }

    func hardDeleteLabel(
        _ id: TaskLabelID,
        mutation: ClassificationMutationContext,
        in state: inout TaskClassificationState
    ) throws {
        guard let label = state.labels[id], state.labelDeletionTombstones[id] == nil else {
            throw NoonmarkError.notFound("task label")
        }
        guard label.lifecycle == .active || label.lifecycle == .archived else {
            throw NoonmarkError.invalidTransition("merged task label cannot be hard deleted")
        }
        guard labelReferenceSummary(id, in: state).isEmpty else {
            throw NoonmarkError.invalidTransition("used task label cannot be hard deleted")
        }
        guard mutation.now >= label.updatedAt else {
            throw NoonmarkError.invalidTransition("task label deletion time moved backwards")
        }
        state.labels[id] = nil
        state.labelDeletionTombstones[id] = TaskLabelDeletionTombstone(
            itemID: id,
            deletedAt: mutation.now,
            revision: mutation.revision,
            changeRecordID: mutation.changeRecordID
        )
    }

    func mergeCategory(
        _ sourceID: TaskCategoryID,
        into targetID: TaskCategoryID,
        mutation: ClassificationMutationContext,
        in state: inout TaskClassificationState
    ) throws {
        guard sourceID != targetID,
              var source = state.categories[sourceID],
              let target = state.categories[targetID]
        else {
            throw NoonmarkError.invalidInput("invalid task category merge")
        }
        guard source.lifecycle == .active || source.lifecycle == .archived,
              state.categoryMerges[sourceID] == nil
        else {
            throw NoonmarkError.invalidTransition("task category already has a merge target")
        }
        guard target.lifecycle == .active, state.categoryMerges[targetID] == nil else {
            throw NoonmarkError.invalidTransition("merge target task category is not active")
        }
        guard mutation.now >= source.updatedAt, mutation.now >= target.updatedAt else {
            throw NoonmarkError.invalidTransition("task category merge time moved backwards")
        }

        let affectedChainIDs = state.currentByChainID.compactMap { chainID, current in
            current.categoryID == sourceID ? chainID : nil
        }
        .sorted { $0.description < $1.description }
        source.lifecycle = .merged
        source.updatedAt = mutation.now
        state.categories[sourceID] = source
        state.categoryMerges[sourceID] = TaskCategoryMerge(
            sourceID: sourceID,
            targetID: targetID,
            mergedAt: mutation.now,
            revision: mutation.revision,
            changeRecordID: mutation.changeRecordID
        )
        for chainID in affectedChainIDs {
            guard let current = state.currentByChainID[chainID] else { continue }
            try validateCurrentRelationMutationTime(for: chainID, at: mutation.now, in: state)
            replaceCurrentClassification(
                for: chainID,
                categoryID: targetID,
                labelIDs: current.labelIDs,
                mutation: mutation,
                in: &state
            )
        }
    }

    func mergeLabel(
        _ sourceID: TaskLabelID,
        into targetID: TaskLabelID,
        mutation: ClassificationMutationContext,
        in state: inout TaskClassificationState
    ) throws {
        guard sourceID != targetID,
              var source = state.labels[sourceID],
              let target = state.labels[targetID]
        else {
            throw NoonmarkError.invalidInput("invalid task label merge")
        }
        guard source.lifecycle == .active || source.lifecycle == .archived,
              state.labelMerges[sourceID] == nil
        else {
            throw NoonmarkError.invalidTransition("task label already has a merge target")
        }
        guard target.lifecycle == .active, state.labelMerges[targetID] == nil else {
            throw NoonmarkError.invalidTransition("merge target task label is not active")
        }
        guard mutation.now >= source.updatedAt, mutation.now >= target.updatedAt else {
            throw NoonmarkError.invalidTransition("task label merge time moved backwards")
        }

        let affectedChainIDs = state.currentByChainID.compactMap { chainID, current in
            current.labelIDs.contains(sourceID) ? chainID : nil
        }
        .sorted { $0.description < $1.description }
        source.lifecycle = .merged
        source.updatedAt = mutation.now
        state.labels[sourceID] = source
        state.labelMerges[sourceID] = TaskLabelMerge(
            sourceID: sourceID,
            targetID: targetID,
            mergedAt: mutation.now,
            revision: mutation.revision,
            changeRecordID: mutation.changeRecordID
        )
        for chainID in affectedChainIDs {
            guard let current = state.currentByChainID[chainID] else { continue }
            try validateCurrentRelationMutationTime(for: chainID, at: mutation.now, in: state)
            var labels = current.labelIDs
            labels.remove(sourceID)
            labels.insert(targetID)
            replaceCurrentClassification(
                for: chainID,
                categoryID: current.categoryID,
                labelIDs: labels,
                mutation: mutation,
                in: &state
            )
        }
    }

    func replaceCurrentClassification(
        for chainID: TaskChainID,
        categoryID: TaskCategoryID?,
        labelIDs: Set<TaskLabelID>,
        mutation: ClassificationMutationContext,
        in state: inout TaskClassificationState
    ) {
        let previous = state.currentByChainID[chainID]
        let category: TaskCategoryRelation?
        if previous?.categoryID == categoryID {
            category = previous?.category
        } else {
            if let previousCategory = previous?.category {
                state.relationHistory.append(
                    relationHistoryEntry(
                        chainID: chainID,
                        relation: previousCategory,
                        mutation: mutation
                    )
                )
            }
            category = categoryID.map {
                TaskCategoryRelation(
                    categoryID: $0,
                    source: mutation.source,
                    decisionID: mutation.decisionID,
                    createdAt: mutation.now,
                    updatedAt: mutation.now,
                    revision: mutation.revision
                )
            }
        }

        let previousLabels = Dictionary(
            uniqueKeysWithValues: (previous?.labels ?? []).map { ($0.labelID, $0) }
        )
        for removedID in Set(previousLabels.keys).subtracting(labelIDs) {
            guard let removed = previousLabels[removedID] else { continue }
            state.relationHistory.append(
                relationHistoryEntry(
                    chainID: chainID,
                    relation: removed,
                    mutation: mutation
                )
            )
        }
        let labels = labelIDs.map { labelID in
            previousLabels[labelID] ?? TaskLabelRelation(
                labelID: labelID,
                source: mutation.source,
                decisionID: mutation.decisionID,
                createdAt: mutation.now,
                updatedAt: mutation.now,
                revision: mutation.revision
            )
        }
        state.currentByChainID[chainID] = CurrentTaskClassification(category: category, labels: labels)
    }

    func validateCurrentRelationMutationTime(
        for chainID: TaskChainID,
        at mutationTime: Date,
        in state: TaskClassificationState
    ) throws {
        guard let current = state.currentByChainID[chainID] else { return }
        let latestRelationTime = ([current.category?.updatedAt].compactMap { $0 } + current.labels.map(\.updatedAt)).max()
        guard latestRelationTime.map({ mutationTime >= $0 }) ?? true else {
            throw NoonmarkError.invalidTransition("task classification relation time moved backwards")
        }
    }

    func classificationHistoryEvents(
        in state: TaskClassificationState
    ) -> [TraceClassificationSnapshot] {
        var eventsByID: [UUID: TraceClassificationSnapshot] = [:]
        for event in state.snapshotEventsByTraceID.values.flatMap({ $0 }) {
            eventsByID[event.id] = event
        }
        for snapshot in state.snapshotsByTraceID.values {
            eventsByID[snapshot.id] = snapshot
        }
        return Array(eventsByID.values)
    }

    func terminalCategoryID(
        for categoryID: TaskCategoryID,
        in state: TaskClassificationState
    ) -> TaskCategoryID? {
        guard state.categories[categoryID] != nil else { return nil }
        var visited: Set<TaskCategoryID> = []
        var cursor = categoryID
        while let merge = state.categoryMerges[cursor] {
            guard visited.insert(cursor).inserted else { return nil }
            cursor = merge.targetID
        }
        return state.categories[cursor] == nil ? nil : cursor
    }

    func terminalLabelID(
        for labelID: TaskLabelID,
        in state: TaskClassificationState
    ) -> TaskLabelID? {
        guard state.labels[labelID] != nil else { return nil }
        var visited: Set<TaskLabelID> = []
        var cursor = labelID
        while let merge = state.labelMerges[cursor] {
            guard visited.insert(cursor).inserted else { return nil }
            cursor = merge.targetID
        }
        return state.labels[cursor] == nil ? nil : cursor
    }

    func categoryMergeFamily(
        convergingOn targetID: TaskCategoryID,
        in state: TaskClassificationState
    ) -> Set<TaskCategoryID> {
        Set(state.categories.keys.filter {
            terminalCategoryID(for: $0, in: state) == targetID
        })
    }

    func labelMergeFamily(
        convergingOn targetID: TaskLabelID,
        in state: TaskClassificationState
    ) -> Set<TaskLabelID> {
        Set(state.labels.keys.filter {
            terminalLabelID(for: $0, in: state) == targetID
        })
    }

    func relationHistoryEntry(
        chainID: TaskChainID,
        relation: TaskCategoryRelation,
        mutation: ClassificationMutationContext
    ) -> ClassificationRelationHistoryEntry {
        ClassificationRelationHistoryEntry(
            kind: .category,
            chainID: chainID,
            itemID: relation.categoryID.description,
            originSource: relation.source,
            originDecisionID: relation.decisionID,
            createdAt: relation.createdAt,
            createdRevision: relation.revision,
            removedBySource: mutation.source,
            removedByDecisionID: mutation.decisionID,
            removedAt: mutation.now,
            removedRevision: mutation.revision
        )
    }

    func relationHistoryEntry(
        chainID: TaskChainID,
        relation: TaskLabelRelation,
        mutation: ClassificationMutationContext
    ) -> ClassificationRelationHistoryEntry {
        ClassificationRelationHistoryEntry(
            kind: .label,
            chainID: chainID,
            itemID: relation.labelID.description,
            originSource: relation.source,
            originDecisionID: relation.decisionID,
            createdAt: relation.createdAt,
            createdRevision: relation.revision,
            removedBySource: mutation.source,
            removedByDecisionID: mutation.decisionID,
            removedAt: mutation.now,
            removedRevision: mutation.revision
        )
    }

    func captureClassificationSnapshot(
        traceID: DayTraceID,
        chainID: TaskChainID,
        now: Date
    ) {
        guard let status = traces[traceID]?.status else { return }
        let current = classificationState.currentByChainID[chainID]
        let category = current?.categoryID.flatMap { id in
            classificationState.categories[id].map {
                HistoricalCategoryValue(
                    id: id,
                    name: $0.name,
                    colorHex: $0.colorHex,
                    presentationApproval: $0.presentationApproval
                )
            }
        }
        let labels = (current?.labelIDs ?? [])
            .compactMap { id in
                classificationState.labels[id].map {
                    HistoricalLabelValue(id: id, name: $0.name, colorHex: $0.colorHex)
                }
            }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        let snapshot = TraceClassificationSnapshot(
            traceID: traceID,
            status: status,
            category: category,
            labels: labels,
            capturedAt: now,
            revision: classificationState.revision + 1
        )
        classificationState.snapshotEventsByTraceID[traceID, default: []].append(snapshot)
        classificationState.snapshotsByTraceID[traceID] = snapshot
        classificationState.revision += 1
    }

    func appendTodayCategoryRemovalSnapshot(
        traceID: DayTraceID,
        categoryID: TaskCategoryID,
        today: LocalDate,
        now: Date
    ) throws -> Bool {
        guard let trace = traces[traceID] else {
            throw NoonmarkError.notFound("day trace")
        }
        guard trace.date == today else {
            throw NoonmarkError.immutableHistory
        }
        guard now.timeIntervalSinceReferenceDate.isFinite else {
            throw NoonmarkError.invalidInput(
                "classification audit time must be finite"
            )
        }
        guard let latest = classificationState
            .snapshotsByTraceID[traceID],
            latest.category?.id == categoryID
        else {
            return false
        }
        guard now >= latest.capturedAt else {
            throw NoonmarkError.invalidTransition(
                "classification snapshot time moved backwards"
            )
        }
        let snapshot = TraceClassificationSnapshot(
            traceID: traceID,
            status: trace.status,
            category: nil,
            labels: latest.labels,
            capturedAt: now,
            revision: classificationState.revision + 1
        )
        classificationState.snapshotEventsByTraceID[
            traceID,
            default: []
        ].append(snapshot)
        classificationState.snapshotsByTraceID[traceID] = snapshot
        classificationState.revision += 1
        return true
    }

    @discardableResult
    package func inheritCurrentClassification(
        from sourceChainID: TaskChainID,
        to targetChainID: TaskChainID,
        now: Date,
        changeRecordID: UUID = UUID(),
        planID: UUID = UUID(),
        interactionID: UUID = UUID()
    ) throws -> Bool {
        guard now.timeIntervalSinceReferenceDate.isFinite else {
            throw NoonmarkError.invalidInput("classification audit time must be finite")
        }
        guard let current = classificationState
            .currentByChainID[sourceChainID]
        else {
            return false
        }
        let before = currentClassificationPreview(for: targetChainID)
        let baseRevision = classificationState.revision
        let mutation = ClassificationMutationContext(
            source: .inherited(fromChainID: sourceChainID),
            decisionID: nil,
            revision: classificationState.revision + 1,
            changeRecordID: changeRecordID,
            now: now
        )
        replaceCurrentClassification(
            for: targetChainID,
            categoryID: current.categoryID,
            labelIDs: current.labelIDs,
            mutation: mutation,
            in: &classificationState
        )
        classificationState.revision += 1
        let after = currentClassificationPreview(for: targetChainID)
        let changes: [ClassificationPlanChange] = [
            .setCurrent(chainID: targetChainID, before: before, after: after)
        ]
        let plan = try ClassificationPlan(
            id: planID,
            baseRevision: baseRevision,
            interactionID: interactionID,
            intent: .setCurrent(
                TaskClassificationDraft(
                    chainID: targetChainID,
                    category: current.categoryID.map(TaskCategoryChoice.existing),
                    labels: current.labelIDs
                        .sorted { $0.description < $1.description }
                        .map(TaskLabelChoice.existing)
                )
            ),
            source: .inherited(fromChainID: sourceChainID),
            changes: changes
        )
        let record = ClassificationChangeRecord(
            id: changeRecordID,
            planID: planID,
            interactionID: interactionID,
            source: .inherited(fromChainID: sourceChainID),
            decisionID: nil,
            changes: changes,
            committedAt: now,
            revision: classificationState.revision,
            planDigest: plan.digest
        )
        classificationState.relationHistory = try ClassificationAuditCanonicalOrder.relationHistory(
            classificationState.relationHistory
        )
        classificationState.changeRecords = try ClassificationAuditCanonicalOrder.changeRecords(
            classificationState.changeRecords + [record]
        )
        return true
    }
}

extension TaskClassificationState {
    func referencesTaskChain(_ chainID: TaskChainID) -> Bool {
        if currentByChainID[chainID] != nil {
            return true
        }
        if currentByChainID.values.contains(where: { current in
            current.category?.source.referencesTaskChain(chainID) == true
                || current.labels.contains { $0.source.referencesTaskChain(chainID) }
        }) {
            return true
        }
        if relationHistory.contains(where: { history in
            history.chainID == chainID
                || history.originSource.referencesTaskChain(chainID)
                || history.removedBySource.referencesTaskChain(chainID)
        }) {
            return true
        }
        return changeRecords.contains { record in
            record.source.referencesTaskChain(chainID)
                || record.changes.contains { $0.referencesTaskChain(chainID) }
        }
    }
}

private extension ClassificationSource {
    func referencesTaskChain(_ chainID: TaskChainID) -> Bool {
        guard case let .inherited(fromChainID) = self else { return false }
        return fromChainID == chainID
    }
}

private extension ClassificationPlanChange {
    func referencesTaskChain(_ chainID: TaskChainID) -> Bool {
        switch self {
        case let .setCurrent(id, _, _):
            return id == chainID
        case let .merge(_, _, _, _, _, _, impact):
            return impact.currentChainIDs.contains(chainID)
        case .create, .hardDelete, .rename, .lifecycle, .categoryPresentationApproval:
            return false
        }
    }
}

private struct VersionedClassificationNameKey: Hashable {
    let algorithmVersion: String
    let value: String
}

private extension TaskCategory {
    var allCanonicalKeys: Set<VersionedClassificationNameKey> {
        Set(nameVersions.map {
            VersionedClassificationNameKey(
                algorithmVersion: $0.canonicalKeyVersion,
                value: $0.canonicalKey
            )
        }).union([
            VersionedClassificationNameKey(
                algorithmVersion: canonicalKeyVersion,
                value: canonicalKey
            )
        ])
    }
}

private extension TaskLabel {
    var allCanonicalKeys: Set<VersionedClassificationNameKey> {
        Set(nameVersions.map {
            VersionedClassificationNameKey(
                algorithmVersion: $0.canonicalKeyVersion,
                value: $0.canonicalKey
            )
        }).union([
            VersionedClassificationNameKey(
                algorithmVersion: canonicalKeyVersion,
                value: canonicalKey
            )
        ])
    }
}
