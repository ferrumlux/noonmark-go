import CryptoKit
import Foundation

public struct AutomaticTaskClassificationAuthority: Codable, Equatable, Sendable {
    private static let formatVersion = 2

    public let chainID: TaskChainID
    public let jobID: UUID
    public let generation: Int

    /// A fail-closed digest of the Provider-visible task content and the
    /// task's automatic-classification eligibility boundary. This is not a
    /// digest of the Provider request body alone: current-definition identity
    /// and mutation clocks deliberately require a fresh job generation after
    /// any touched task-definition boundary. Scheduling and task-note changes
    /// remain valid only while they leave those clocks unchanged.
    public let contentDigest: String
    public let classificationFingerprint: String
    public let catalogDigest: String

    private let integrityDigest: String

    private enum CodingKeys: String, CodingKey {
        case chainID
        case jobID
        case generation
        case contentDigest
        case classificationFingerprint
        case catalogDigest
        case integrityDigest
    }

    fileprivate init(
        chainID: TaskChainID,
        jobID: UUID,
        generation: Int,
        contentDigest: String,
        classificationFingerprint: String,
        catalogDigest: String
    ) {
        self.chainID = chainID
        self.jobID = jobID
        self.generation = generation
        self.contentDigest = contentDigest
        self.classificationFingerprint = classificationFingerprint
        self.catalogDigest = catalogDigest
        self.integrityDigest = Self.digest(
            AuthorityDigestMaterial(
                formatVersion: Self.formatVersion,
                chainID: chainID,
                jobID: jobID,
                generation: generation,
                contentDigest: contentDigest,
                classificationFingerprint: classificationFingerprint,
                catalogDigest: catalogDigest
            )
        )
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        chainID = try container.decode(TaskChainID.self, forKey: .chainID)
        jobID = try container.decode(UUID.self, forKey: .jobID)
        generation = try container.decode(Int.self, forKey: .generation)
        contentDigest = try container.decode(String.self, forKey: .contentDigest)
        classificationFingerprint = try container.decode(
            String.self,
            forKey: .classificationFingerprint
        )
        catalogDigest = try container.decode(String.self, forKey: .catalogDigest)
        integrityDigest = try container.decode(String.self, forKey: .integrityDigest)
        guard generation > 0,
              Self.isSHA256(contentDigest),
              Self.isSHA256(classificationFingerprint),
              Self.isSHA256(catalogDigest),
              hasValidIntegrityDigest
        else {
            throw DecodingError.dataCorrupted(
                .init(
                    codingPath: decoder.codingPath,
                    debugDescription: "automatic classification authority is malformed"
                )
            )
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(chainID, forKey: .chainID)
        try container.encode(jobID, forKey: .jobID)
        try container.encode(generation, forKey: .generation)
        try container.encode(contentDigest, forKey: .contentDigest)
        try container.encode(
            classificationFingerprint,
            forKey: .classificationFingerprint
        )
        try container.encode(catalogDigest, forKey: .catalogDigest)
        try container.encode(integrityDigest, forKey: .integrityDigest)
    }

    fileprivate var hasValidIntegrityDigest: Bool {
        integrityDigest == Self.digest(
            AuthorityDigestMaterial(
                formatVersion: Self.formatVersion,
                chainID: chainID,
                jobID: jobID,
                generation: generation,
                contentDigest: contentDigest,
                classificationFingerprint: classificationFingerprint,
                catalogDigest: catalogDigest
            )
        )
    }

    private static func digest(_ material: AuthorityDigestMaterial) -> String {
        automaticClassificationDigest(material)
    }

    private static func isSHA256(_ value: String) -> Bool {
        value.utf8.count == 64 && value.utf8.allSatisfy {
            (48 ... 57).contains($0) || (97 ... 102).contains($0)
        }
    }
}

public struct AutomaticTaskClassificationContext: Equatable, Sendable {
    public let authority: AutomaticTaskClassificationAuthority
    public let taskTitle: String
    public let taskDescription: String?
    public let currentClassification: TaskClassificationProjection
    public let catalog: ClassificationCatalogProjection

    fileprivate init(
        authority: AutomaticTaskClassificationAuthority,
        taskTitle: String,
        taskDescription: String?,
        currentClassification: TaskClassificationProjection,
        catalog: ClassificationCatalogProjection
    ) {
        self.authority = authority
        self.taskTitle = taskTitle
        self.taskDescription = taskDescription
        self.currentClassification = currentClassification
        self.catalog = catalog
    }
}

// swiftlint:disable:next type_name
public struct AutomaticClassificationApplicationProposal: Equatable, Sendable {
    public let category: TaskCategoryChoice
    public let labels: [TaskLabelChoice]

    public init(category: TaskCategoryChoice, labels: [TaskLabelChoice]) {
        self.category = category
        self.labels = labels
    }
}

public struct AutomaticTaskClassificationPlan: Equatable, Sendable {
    public let authority: AutomaticTaskClassificationAuthority
    public var source: ClassificationSource { classificationPlan.source }
    public var changes: [ClassificationPlanChange] { classificationPlan.changes }
    public var notices: [ClassificationNotice] { classificationPlan.notices }

    fileprivate let classificationPlan: ClassificationPlan
}

public extension NoonmarkEngine {
    @_spi(AutomaticClassificationJobAuthority)
    func issueAutomaticClassificationContext(
        for chainID: TaskChainID,
        jobID: UUID,
        generation: Int
    ) throws -> AutomaticTaskClassificationContext {
        guard generation > 0 else {
            throw NoonmarkError.invalidInput(
                "automatic classification generation must be positive"
            )
        }
        let chain = try automaticClassificationChain(for: chainID)
        let definition = try automaticClassificationDefinition(for: chainID)
        let task = try automaticTaskProjection(for: chainID)
        let catalog = try automaticClassificationCatalog()
        let authority = AutomaticTaskClassificationAuthority(
            chainID: chainID,
            jobID: jobID,
            generation: generation,
            contentDigest: automaticTaskContentAndEligibilityDigest(
                chainID: chainID,
                chain: chain,
                definition: definition
            ),
            classificationFingerprint: automaticClassificationFingerprint(
                for: chainID
            ),
            catalogDigest: automaticClassificationCatalogDigest(catalog)
        )
        return AutomaticTaskClassificationContext(
            authority: authority,
            taskTitle: definition.title,
            taskDescription: definition.descriptionText,
            currentClassification: task,
            catalog: catalog
        )
    }

    @_spi(AutomaticClassificationJobAuthority)
    func automaticClassificationContext(
        authority: AutomaticTaskClassificationAuthority
    ) throws -> AutomaticTaskClassificationContext {
        try validateAutomaticClassificationAuthority(authority)
        let definition = try automaticClassificationDefinition(
            for: authority.chainID
        )
        return AutomaticTaskClassificationContext(
            authority: authority,
            taskTitle: definition.title,
            taskDescription: definition.descriptionText,
            currentClassification: try automaticTaskProjection(
                for: authority.chainID
            ),
            catalog: try automaticClassificationCatalog()
        )
    }

    func prepareAutomaticClassification(
        _ proposal: AutomaticClassificationApplicationProposal,
        authority: AutomaticTaskClassificationAuthority,
        interactionID: UUID,
        now: Date
    ) throws -> AutomaticTaskClassificationPlan {
        try validateAutomaticClassificationAuthority(authority)
        guard (1 ... 3).contains(proposal.labels.count) else {
            throw NoonmarkError.invalidInput(
                "automatic classification requires one to three proposed labels"
            )
        }
        let current = classificationState.currentByChainID[authority.chainID]
        let protectedCategory = current?.category.flatMap {
            switch $0.source {
            case .userDirect, .zhulongSuggestion:
                TaskCategoryChoice.existing($0.categoryID)
            case .automaticAI, .inherited, .deterministicDomainAction:
                nil
            }
        }
        let protectedLabels = current?.labels.map {
            TaskLabelChoice.existing($0.labelID)
        } ?? []
        let classificationPlan = try prepareClassification(
            .setCurrent(
                TaskClassificationDraft(
                    chainID: authority.chainID,
                    category: protectedCategory ?? proposal.category,
                    labels: protectedLabels + proposal.labels
                )
            ),
            source: .automaticAI(
                jobID: authority.jobID,
                generation: authority.generation
            ),
            interactionID: interactionID,
            now: now
        )
        return AutomaticTaskClassificationPlan(
            authority: authority,
            classificationPlan: classificationPlan
        )
    }

    func commitAutomaticClassification(
        _ plan: AutomaticTaskClassificationPlan,
        authority: AutomaticTaskClassificationAuthority,
        now: Date
    ) throws -> ClassificationReceipt {
        guard plan.authority == authority,
              plan.classificationPlan.source == .automaticAI(
                  jobID: authority.jobID,
                  generation: authority.generation
              )
        else {
            throw NoonmarkError.invalidInput(
                "automatic classification plan does not match its authority"
            )
        }
        try validateAutomaticClassificationAuthority(authority)
        try validateAutomaticClassificationMutationTime(
            now,
            chainID: authority.chainID
        )
        try validateClassificationCommit(plan.classificationPlan)
        guard classificationState.committedReceiptsByInteractionID[
            plan.classificationPlan.interactionID
        ] == nil,
            classificationState.changeRecords.contains(where: {
                $0.interactionID == plan.classificationPlan.interactionID
            }) == false
        else {
            throw NoonmarkError.invalidInput(
                "classification interaction id was already used"
            )
        }
        guard let receipt = try applyClassificationCommit(
            plan.classificationPlan,
            decisionID: nil,
            recordsUserReceipt: false,
            enforcesBaseRevision: false,
            now: now
        ) else {
            throw NoonmarkError.invalidInput(
                "automatic classification commit did not create a receipt"
            )
        }
        return receipt
    }
}

private extension NoonmarkEngine {
    func automaticClassificationChain(
        for chainID: TaskChainID
    ) throws -> TaskChain {
        guard let chain = chains[chainID] else {
            throw NoonmarkError.notFound("task chain")
        }
        guard chain.state == .active else {
            throw NoonmarkError.chainAbandoned
        }
        return chain
    }

    func automaticClassificationDefinition(
        for chainID: TaskChainID
    ) throws -> TaskDefinition {
        let candidates = definitions.values.filter {
            $0.chainID == chainID && $0.supersededAt == nil
        }
        guard let definition = candidates.max(by: {
            $0.sequence < $1.sequence
        }) else {
            throw NoonmarkError.notFound("current task definition")
        }
        return definition
    }

    func validateAutomaticClassificationAuthority(
        _ authority: AutomaticTaskClassificationAuthority
    ) throws {
        guard authority.generation > 0, authority.hasValidIntegrityDigest else {
            throw NoonmarkError.invalidInput(
                "automatic classification authority is malformed"
            )
        }
        let chain = try automaticClassificationChain(for: authority.chainID)
        let definition = try automaticClassificationDefinition(
            for: authority.chainID
        )
        guard automaticTaskContentAndEligibilityDigest(
            chainID: authority.chainID,
            chain: chain,
            definition: definition
        ) == authority.contentDigest else {
            throw NoonmarkError.invalidTransition(
                "automatic classification task content changed"
            )
        }
        guard automaticClassificationFingerprint(for: authority.chainID)
            == authority.classificationFingerprint
        else {
            throw NoonmarkError.invalidTransition(
                "automatic classification was superseded by a classification change"
            )
        }
        guard automaticClassificationCatalogDigest(
            try automaticClassificationCatalog()
        ) == authority.catalogDigest else {
            throw NoonmarkError.invalidTransition(
                "automatic classification catalog changed"
            )
        }
    }

    func validateAutomaticClassificationMutationTime(
        _ now: Date,
        chainID: TaskChainID
    ) throws {
        let chain = try automaticClassificationChain(for: chainID)
        let definition = try automaticClassificationDefinition(for: chainID)
        let seconds = now.timeIntervalSinceReferenceDate
        guard seconds.isFinite,
              now >= chain.createdAt,
              now >= chain.updatedAt,
              now >= definition.createdAt,
              now >= definition.contentUpdatedAt
        else {
            throw NoonmarkError.invalidInput(
                "automatic classification mutation time predates its task content"
            )
        }
    }

    func automaticTaskProjection(
        for chainID: TaskChainID
    ) throws -> TaskClassificationProjection {
        guard case let .task(task) = try classification(.task(chainID)) else {
            throw NoonmarkError.invalidInput(
                "automatic classification task projection is unavailable"
            )
        }
        return task
    }

    func automaticClassificationCatalog() throws -> ClassificationCatalogProjection {
        guard case let .catalog(catalog) = try classification(.catalog) else {
            throw NoonmarkError.invalidInput(
                "automatic classification catalog is unavailable"
            )
        }
        return ClassificationCatalogProjection(
            categories: catalog.categories.filter { $0.lifecycle == .active },
            labels: catalog.labels.filter { $0.lifecycle == .active },
            revision: catalog.revision
        )
    }

    func automaticClassificationCatalogDigest(
        _ catalog: ClassificationCatalogProjection
    ) -> String {
        automaticClassificationDigest(
            CatalogDigestMaterial(
                formatVersion: 2,
                categories: catalog.categories.map(CatalogItemDigestMaterial.init),
                labels: catalog.labels.map(CatalogItemDigestMaterial.init)
            )
        )
    }

    func automaticTaskContentAndEligibilityDigest(
        chainID: TaskChainID,
        chain: TaskChain,
        definition: TaskDefinition
    ) -> String {
        automaticClassificationDigest(
            TaskContentAndEligibilityDigestMaterial(
                formatVersion: 1,
                chainID: chainID,
                chainState: chain.state,
                chainUpdatedAtBitPattern: chain.updatedAt
                    .timeIntervalSinceReferenceDate.bitPattern,
                definitionID: definition.id,
                definitionSequence: definition.sequence,
                title: definition.title,
                description: definition.descriptionText,
                contentUpdatedAtBitPattern: definition.contentUpdatedAt
                    .timeIntervalSinceReferenceDate.bitPattern
            )
        )
    }

    func automaticClassificationFingerprint(
        for chainID: TaskChainID
    ) -> String {
        let current = classificationState.currentByChainID[chainID]
        return automaticClassificationDigest(
            ClassificationFingerprintMaterial(
                formatVersion: 1,
                chainID: chainID,
                category: current?.category,
                labels: current?.labels ?? []
            )
        )
    }
}

private struct AuthorityDigestMaterial: Encodable {
    let formatVersion: Int
    let chainID: TaskChainID
    let jobID: UUID
    let generation: Int
    let contentDigest: String
    let classificationFingerprint: String
    let catalogDigest: String
}

private struct CatalogDigestMaterial: Encodable {
    let formatVersion: Int
    let categories: [CatalogItemDigestMaterial]
    let labels: [CatalogItemDigestMaterial]
}

private struct CatalogItemDigestMaterial: Encodable {
    let id: String
    let name: String
    let colorHex: String
    let lifecycle: ClassificationLifecycle
    let presentationApproval: CategoryPresentationApproval?

    init(_ item: ClassificationCatalogItemProjection) {
        id = item.id
        name = item.name
        colorHex = item.colorHex
        lifecycle = item.lifecycle
        presentationApproval = item.presentationApproval
    }
}

private struct TaskContentAndEligibilityDigestMaterial: Encodable {
    let formatVersion: Int
    let chainID: TaskChainID
    let chainState: TaskChainState
    let chainUpdatedAtBitPattern: UInt64
    let definitionID: TaskDefinitionID
    let definitionSequence: Int
    let title: String
    let description: String?
    let contentUpdatedAtBitPattern: UInt64
}

private struct ClassificationFingerprintMaterial: Encodable {
    let formatVersion: Int
    let chainID: TaskChainID
    let category: TaskCategoryRelation?
    let labels: [TaskLabelRelation]
}

private func automaticClassificationDigest(_ value: some Encodable) -> String {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    guard let data = try? encoder.encode(value) else {
        preconditionFailure("automatic classification digest payload cannot be encoded")
    }
    return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
}
