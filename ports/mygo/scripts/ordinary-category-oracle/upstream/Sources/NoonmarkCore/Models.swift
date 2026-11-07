import Foundation

public enum TaskChainState: String, Codable, Hashable, Sendable {
    case active
    case abandoned
}

public enum TraceStatus: String, Codable, Hashable, Sendable {
    case pending
    case completed
    case unfinished
    case deferred
    case changed
    case returnedToPool
    case cancelledDraft
    case abandoned

    /// `cancelledDraft` is an internal append-only persistence/sync fact.
    /// Every other status is part of the user-visible Day history.
    public var formsDayHistory: Bool {
        self != .cancelledDraft
    }

    public var isUserPresentable: Bool {
        formsDayHistory
    }

    /// A Day Todo row represents work that still belongs to that date, or a
    /// real outcome for that date. Movement source facts remain available to
    /// the task trail without occupying another task row.
    public var isVisibleInDayTodo: Bool {
        switch self {
        case .pending, .completed, .unfinished, .deferred, .abandoned:
            true
        case .changed, .returnedToPool, .cancelledDraft:
            false
        }
    }
}

public enum DayTraceCompletionAction: Equatable, Sendable {
    case complete
    case undo
}

public enum DayTraceCompletionBlocker: Equatable, Sendable {
    case lockedDay
    case historicalDay
    case futureDay
    case unsupportedStatus(TraceStatus)
    case openSubtasks
}

public enum DayTraceCompletionCapability: Equatable, Sendable {
    case available(DayTraceCompletionAction)
    case unavailable(DayTraceCompletionBlocker)
}

public enum SubtaskStatus: String, Codable, Hashable, Sendable {
    case pending
    case completed
    case unfinished
    case deferred
    case abandoned
    case cancelledDraft

    /// `cancelledDraft` is an internal append-only undo fact and must never
    /// cross a user-facing projection boundary.
    public var isUserPresentable: Bool {
        self != .cancelledDraft
    }
}

public enum TraceCarryoverKind: String, Codable, Equatable, Hashable, Sendable {
    case deferral
    case continuation
}

public enum SubtaskDifficulty: Int, Codable, Hashable, Sendable, CaseIterable {
    case simple = 1
    case medium = 2
    case hard = 3

    public var label: String {
        switch self {
        case .simple:
            return "简"
        case .medium:
            return "中"
        case .hard:
            return "难"
        }
    }
}

public enum TraceProgressMode: String, Codable, Hashable, Sendable {
    case manual
    case weightedSubtasks
}

public enum NewTaskTarget: Equatable, Sendable {
    case taskPool
    case date(LocalDate)
}

public enum TaskPoolRemovalOutcome: Equatable, Sendable {
    case deleted
    case removedKeepingHistory
}

public enum ViewSort: String, Codable, Hashable, Sendable {
    case priority
    case createdAt
    case title
}

public enum AppTheme: String, Codable, Hashable, Sendable, CaseIterable {
    case coolGray
    case warmPaper
}

public enum AppLanguage: String, Codable, Hashable, Sendable, CaseIterable {
    case chinese
    case english
}

public enum AppDataMode: String, Codable, Hashable, Sendable, CaseIterable {
    case localFirst
    case onlineFirst
}

public enum BackupDestinationKind: String, Codable, Hashable, Sendable, CaseIterable {
    case iCloudDrive
    case s3
}

public enum ScheduledBackupFrequency: String, Codable, Hashable, Sendable, CaseIterable {
    case off
    case daily
    case weekly
}

public struct ScheduledBackupPolicy: Codable, Equatable, Sendable {
    public var frequency: ScheduledBackupFrequency
    public var destination: BackupDestinationKind

    public var isEnabled: Bool {
        frequency != .off
    }

    public init(
        frequency: ScheduledBackupFrequency = .off,
        destination: BackupDestinationKind = .iCloudDrive
    ) {
        self.frequency = frequency
        self.destination = destination
    }
}

public enum CloudSyncEndpointKind: String, Codable, Hashable, Sendable, CaseIterable {
    case iCloud
    case s3
    case webDAV
    case localFolder
}

public enum CloudSyncMode: String, Codable, Hashable, Sendable, CaseIterable {
    case manual
    case automatic
}

public struct SyncSnapshotRetentionPolicy: Codable, Equatable, Sendable {
    public var indexRetentionDays: Int
    public var retentionIndexesDaily: Int

    public init(indexRetentionDays: Int = 180, retentionIndexesDaily: Int = 2) {
        self.indexRetentionDays = max(1, indexRetentionDays)
        self.retentionIndexesDaily = max(1, retentionIndexesDaily)
    }
}

public struct LocalFirstCloudSyncPolicy: Codable, Equatable, Sendable {
    public var enabled: Bool
    public var endpoint: CloudSyncEndpointKind
    public var mode: CloudSyncMode
    public var intervalSeconds: Int
    public var generateConflictCopy: Bool
    public var snapshotRetention: SyncSnapshotRetentionPolicy

    public init(
        enabled: Bool = false,
        endpoint: CloudSyncEndpointKind = .iCloud,
        mode: CloudSyncMode = .manual,
        intervalSeconds: Int = 300,
        generateConflictCopy: Bool = true,
        snapshotRetention: SyncSnapshotRetentionPolicy = SyncSnapshotRetentionPolicy()
    ) {
        self.enabled = enabled
        self.endpoint = endpoint
        self.mode = mode
        self.intervalSeconds = min(max(intervalSeconds, 30), 43200)
        self.generateConflictCopy = generateConflictCopy
        self.snapshotRetention = snapshotRetention
    }
}

public struct SettingsPoemDisplayPolicy: Codable, Equatable, Sendable {
    public var enabled: Bool
    public var text: String

    public init(enabled: Bool = true, text: String = SettingsPoemDisplayPolicy.defaultText) {
        self.enabled = enabled
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.text = trimmed.isEmpty ? SettingsPoemDisplayPolicy.defaultText : text
    }

    public static let defaultText = """
    飞光飞光，劝尔一杯酒。
    吾不识青天高，黄地厚。
    唯见月寒日暖，来煎人寿。
    食熊则肥，食蛙则瘦。
    神君何在？太一安有？
    天东有若木，下置衔烛龙。
    吾将斩龙足，嚼龙肉，
    使之朝不得回，夜不得伏。
    自然老者不死，少者不哭。
    何为服黄金，吞白玉？
    谁似任公子，云中骑碧驴？
    刘彻茂陵多滞骨，嬴政梓棺费鲍鱼。
    """
}

public enum SyncEndpointKind: String, Codable, Hashable, Sendable, CaseIterable {
    case customEndpoint
    case iCloud
    case s3
    case webDAV
    case localFolder
}

public enum SyncEndpointAvailability: String, Codable, Hashable, Sendable {
    case available
    case planned
}

public struct SyncEndpointOption: Codable, Equatable, Sendable {
    public let kind: SyncEndpointKind
    public let title: String
    public let description: String
    public let availability: SyncEndpointAvailability

    public init(kind: SyncEndpointKind, title: String, description: String, availability: SyncEndpointAvailability) {
        self.kind = kind
        self.title = title
        self.description = description
        self.availability = availability
    }
}

public struct AppPreferences: Codable, Equatable, Sendable {
    public static let bootstrapThemeLanguageWriterID =
        "noonmark.bootstrap"
    public static let defaultLocalThemeLanguageWriterID =
        "noonmark.local"

    public var theme: AppTheme
    public var language: AppLanguage
    public private(set) var themeLanguageUpdatedAt: Date
    public private(set) var themeLanguageWriterID: String
    public var dataMode: AppDataMode
    public var backupPolicy: ScheduledBackupPolicy
    public var localFirstSyncPolicy: LocalFirstCloudSyncPolicy
    public var settingsPoemDisplayPolicy: SettingsPoemDisplayPolicy
    public var syncEndpointOptions: [SyncEndpointOption]

    public init(
        theme: AppTheme = .coolGray,
        language: AppLanguage = .chinese,
        themeLanguageUpdatedAt: Date = Date(timeIntervalSince1970: 0),
        themeLanguageWriterID: String =
            AppPreferences.bootstrapThemeLanguageWriterID,
        dataMode: AppDataMode = .localFirst,
        backupPolicy: ScheduledBackupPolicy = ScheduledBackupPolicy(),
        localFirstSyncPolicy: LocalFirstCloudSyncPolicy = LocalFirstCloudSyncPolicy(),
        settingsPoemDisplayPolicy: SettingsPoemDisplayPolicy = SettingsPoemDisplayPolicy(),
        syncEndpointOptions: [SyncEndpointOption] = AppPreferences.defaultSyncEndpointOptions
    ) {
        self.theme = theme
        self.language = language
        self.themeLanguageUpdatedAt = themeLanguageUpdatedAt
        self.themeLanguageWriterID = themeLanguageWriterID
        self.dataMode = dataMode
        self.backupPolicy = backupPolicy
        self.localFirstSyncPolicy = localFirstSyncPolicy
        self.settingsPoemDisplayPolicy = settingsPoemDisplayPolicy
        self.syncEndpointOptions = syncEndpointOptions
    }

    public static let defaultSyncEndpointOptions: [SyncEndpointOption] = [
        SyncEndpointOption(
            kind: .customEndpoint,
            title: "自定义同步端点",
            description: "连接自有服务端",
            availability: .planned
        ),
        SyncEndpointOption(
            kind: .iCloud,
            title: "iCloud 云同步",
            description: "写入当前 Apple Account 的 iCloud Drive 同步仓库",
            availability: .available
        ),
        SyncEndpointOption(
            kind: .s3,
            title: "S3 云同步",
            description: "兼容对象存储同步端点",
            availability: .planned
        ),
        SyncEndpointOption(
            kind: .webDAV,
            title: "WebDAV 云同步",
            description: "兼容 WebDAV 的同步端点",
            availability: .planned
        ),
        SyncEndpointOption(
            kind: .localFolder,
            title: "本地同步文件夹",
            description: "自管目录或局域网共享端点",
            availability: .available
        )
    ]

    public func applyingSyncedThemeLanguage(
        theme: AppTheme,
        language: AppLanguage,
        updatedAt: Date,
        writerID: String
    ) throws -> AppPreferences {
        guard let order = Self.themeLanguageVersionOrder(
            clock: updatedAt,
            writerID: writerID,
            comparedToClock: themeLanguageUpdatedAt,
            writerID: themeLanguageWriterID
        ), order != .orderedAscending else {
            throw NoonmarkError.invalidInput(
                "synced theme and language clock cannot move backwards"
            )
        }
        if order == .orderedSame {
            guard theme == self.theme, language == self.language else {
                throw NoonmarkError.invalidInput(
                    "synced theme and language version diverged"
                )
            }
            return self
        }
        return AppPreferences(
            theme: theme,
            language: language,
            themeLanguageUpdatedAt: updatedAt,
            themeLanguageWriterID: writerID,
            dataMode: dataMode,
            backupPolicy: backupPolicy,
            localFirstSyncPolicy: localFirstSyncPolicy,
            settingsPoemDisplayPolicy: settingsPoemDisplayPolicy,
            syncEndpointOptions: syncEndpointOptions
        )
    }

    public static func themeLanguageVersionOrder(
        clock lhsClock: Date,
        writerID lhsWriterID: String,
        comparedToClock rhsClock: Date,
        writerID rhsWriterID: String
    ) -> ComparisonResult? {
        let lhsSeconds = lhsClock.timeIntervalSinceReferenceDate
        let rhsSeconds = rhsClock.timeIntervalSinceReferenceDate
        let lhsTrimmedWriter = lhsWriterID.trimmingCharacters(
            in: .whitespacesAndNewlines
        )
        let rhsTrimmedWriter = rhsWriterID.trimmingCharacters(
            in: .whitespacesAndNewlines
        )
        guard lhsSeconds.isFinite,
              rhsSeconds.isFinite,
              lhsTrimmedWriter.isEmpty == false,
              rhsTrimmedWriter.isEmpty == false
        else {
            return nil
        }
        if lhsSeconds < rhsSeconds {
            return .orderedAscending
        }
        if lhsSeconds > rhsSeconds {
            return .orderedDescending
        }
        let lhsBits = lhsSeconds.bitPattern
        let rhsBits = rhsSeconds.bitPattern
        if lhsBits < rhsBits {
            return .orderedAscending
        }
        if lhsBits > rhsBits {
            return .orderedDescending
        }
        let lhsBytes = Data(lhsWriterID.utf8)
        let rhsBytes = Data(rhsWriterID.utf8)
        if lhsBytes == rhsBytes {
            return .orderedSame
        }
        return lhsBytes.lexicographicallyPrecedes(rhsBytes)
            ? .orderedAscending
            : .orderedDescending
    }
}

public struct Day: Codable, Equatable, Sendable {
    public var id: DayID
    public var date: LocalDate
    public var lockedAt: Date?
    public var reviewSummary: String?
    public var reviewUnfinishedReason: String?
    public var reviewTomorrowNote: String?
    public var createdAt: Date
    public var updatedAt: Date

    public init(id: DayID = DayID(), date: LocalDate, now: Date) {
        self.id = id
        self.date = date
        self.lockedAt = nil
        self.reviewSummary = nil
        self.reviewUnfinishedReason = nil
        self.reviewTomorrowNote = nil
        self.createdAt = now
        self.updatedAt = now
    }

    mutating func markContentModified(at now: Date) throws {
        try ContentMutationClock.advance(&updatedAt, to: now)
    }
}

public struct TaskChain: Codable, Equatable, Sendable {
    public var id: TaskChainID
    public var state: TaskChainState
    public var cycleMembership: TaskCycleMembership?
    public var noteEntries: [TaskNoteEntry]
    public var createdAt: Date
    public var updatedAt: Date

    public var activeNoteEntries: [TaskNoteEntry] {
        noteEntries.filter { !$0.isDeleted }
    }

    public init(
        id: TaskChainID = TaskChainID(),
        state: TaskChainState = .active,
        cycleMembership: TaskCycleMembership? = nil,
        noteEntries: [TaskNoteEntry] = [],
        now: Date
    ) {
        self.id = id
        self.state = state
        self.cycleMembership = cycleMembership
        self.noteEntries = noteEntries
        self.createdAt = now
        self.updatedAt = now
    }

    mutating func markContentModified(at now: Date) throws {
        try ContentMutationClock.advance(&updatedAt, to: now)
    }
}

public enum TaskNoteEntryValidationIssue: Equatable, Sendable {
    case duplicateIdentity
    case invalidTimestamps
    case invalidTombstone
    case invalidActiveBody
}

public enum TaskNoteEntryValidator {
    public static func firstIssue(
        in entries: [TaskNoteEntry]
    ) -> TaskNoteEntryValidationIssue? {
        guard Set(entries.map(\.id)).count == entries.count else {
            return .duplicateIdentity
        }
        for entry in entries {
            guard TaskNoteEntry.isValidMutationTime(
                entry.updatedAt,
                notBefore: entry.createdAt
            ) else {
                return .invalidTimestamps
            }
            if let deletedAt = entry.deletedAt {
                guard TaskNoteEntry.isValidMutationTime(
                    deletedAt,
                    notBefore: entry.createdAt
                ), deletedAt == entry.updatedAt,
                      entry.body.isEmpty
                else {
                    return .invalidTombstone
                }
            } else {
                guard TaskNoteEntry.normalizedBody(entry.body) == entry.body else {
                    return .invalidActiveBody
                }
            }
        }
        return nil
    }
}

public struct TaskNoteEntry: Codable, Equatable, Identifiable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case id
        case body
        case createdAt
        case updatedAt
        case deletedAt
    }

    public let id: TaskNoteEntryID
    public private(set) var body: String
    public let createdAt: Date
    public private(set) var updatedAt: Date
    public private(set) var deletedAt: Date?

    init(
        id: TaskNoteEntryID = TaskNoteEntryID(),
        body: String,
        now: Date
    ) throws {
        guard let normalizedBody = Self.normalizedBody(body) else {
            throw NoonmarkError.invalidTransition("task note body cannot be empty")
        }
        try Self.validateMutationTime(now, notBefore: now)
        self.id = id
        self.body = normalizedBody
        self.createdAt = now
        self.updatedAt = now
        self.deletedAt = nil
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(TaskNoteEntryID.self, forKey: .id)
        body = try container.decode(String.self, forKey: .body)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        deletedAt = try container.decodeIfPresent(Date.self, forKey: .deletedAt)

        if let issue = TaskNoteEntryValidator.firstIssue(in: [self]) {
            throw DecodingError.dataCorrupted(
                .init(
                    codingPath: decoder.codingPath,
                    debugDescription: "Invalid task note entry: \(issue)"
                )
            )
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(body, forKey: .body)
        try container.encode(createdAt, forKey: .createdAt)
        try container.encode(updatedAt, forKey: .updatedAt)
        try container.encodeIfPresent(deletedAt, forKey: .deletedAt)
    }

    public var isDeleted: Bool { deletedAt != nil }

    mutating func edit(body: String, now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("task note")
        }
        try Self.validateMutationTime(now, notBefore: updatedAt)
        guard let normalizedBody = Self.normalizedBody(body) else {
            throw NoonmarkError.invalidTransition("task note body cannot be empty")
        }
        self.body = normalizedBody
        updatedAt = now
    }

    mutating func delete(now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("task note")
        }
        try Self.validateMutationTime(now, notBefore: updatedAt)
        body = ""
        updatedAt = now
        deletedAt = now
    }

    static func validateMutationTime(_ now: Date, notBefore lowerBound: Date) throws {
        guard isValidMutationTime(now, notBefore: lowerBound) else {
            throw NoonmarkError.invalidInput("task note mutation time cannot move backwards")
        }
    }

    static func isValidMutationTime(_ now: Date, notBefore lowerBound: Date) -> Bool {
        now.timeIntervalSinceReferenceDate.isFinite
            && lowerBound.timeIntervalSinceReferenceDate.isFinite
            && now >= lowerBound
    }

    static func normalizedBody(_ body: String) -> String? {
        let normalizedBody = body.trimmingCharacters(in: .whitespacesAndNewlines)
        return normalizedBody.isEmpty ? nil : normalizedBody
    }
}

extension [TaskNoteEntry] {
    mutating func editTaskNote(
        id: TaskNoteEntryID,
        body: String,
        now: Date,
        ownerUpdatedAt: Date? = nil
    ) throws {
        try mutateTaskNote(id: id, now: now, ownerUpdatedAt: ownerUpdatedAt) { entry in
            try entry.edit(body: body, now: now)
        }
    }

    mutating func deleteTaskNote(
        id: TaskNoteEntryID,
        now: Date,
        ownerUpdatedAt: Date? = nil
    ) throws {
        try mutateTaskNote(id: id, now: now, ownerUpdatedAt: ownerUpdatedAt) { entry in
            try entry.delete(now: now)
        }
    }

    private mutating func mutateTaskNote(
        id: TaskNoteEntryID,
        now: Date,
        ownerUpdatedAt: Date?,
        mutation: (inout TaskNoteEntry) throws -> Void
    ) throws {
        guard let index = firstIndex(where: { $0.id == id && !$0.isDeleted }) else {
            throw NoonmarkError.notFound("task note")
        }
        if let ownerUpdatedAt {
            try TaskNoteEntry.validateMutationTime(now, notBefore: ownerUpdatedAt)
        }
        try mutation(&self[index])
    }
}

public enum IdeaEntryValidationIssue: Equatable, Sendable {
    case duplicateIdentity
    case invalidTimestamps
    case invalidTombstone
    case invalidActiveBody
    case duplicateLabels
    case invalidPin
}

public enum IdeaEntryValidator {
    public static func firstIssue(
        in entries: [IdeaEntry]
    ) -> IdeaEntryValidationIssue? {
        guard Set(entries.map(\.id)).count == entries.count else {
            return .duplicateIdentity
        }
        for entry in entries {
            guard TaskNoteEntry.isValidMutationTime(
                entry.updatedAt,
                notBefore: entry.createdAt
            ) else {
                return .invalidTimestamps
            }
            guard Set(entry.labelIDs).count == entry.labelIDs.count else {
                return .duplicateLabels
            }
            if let pinnedAt = entry.pinnedAt {
                guard TaskNoteEntry.isValidMutationTime(
                    pinnedAt,
                    notBefore: entry.createdAt
                ), TaskNoteEntry.isValidMutationTime(
                    entry.updatedAt,
                    notBefore: pinnedAt
                ) else {
                    return .invalidPin
                }
            }
            if let deletedAt = entry.deletedAt {
                // Tombstoning always clears pinnedAt, so a restore returns the
                // idea unpinned; projections never surface tombstones anyway.
                guard entry.pinnedAt == nil else {
                    return .invalidPin
                }
                guard TaskNoteEntry.isValidMutationTime(
                    deletedAt,
                    notBefore: entry.createdAt
                ), deletedAt == entry.updatedAt,
                      entry.body.isEmpty,
                      entry.categoryID == nil,
                      entry.labelIDs.isEmpty
                else {
                    return .invalidTombstone
                }
            } else {
                guard TaskNoteEntry.normalizedBody(entry.body) == entry.body else {
                    return .invalidActiveBody
                }
            }
        }
        return nil
    }
}

public struct IdeaEntry: Codable, Equatable, Identifiable, Sendable {
    private enum CodingKeys: String, CodingKey {
        case id
        case body
        case categoryID
        case labelIDs
        case createdAt
        case updatedAt
        case deletedAt
        case pinnedAt
    }

    public let id: IdeaID
    public private(set) var body: String
    public private(set) var categoryID: TaskCategoryID?
    public private(set) var labelIDs: [TaskLabelID]
    public let createdAt: Date
    public private(set) var updatedAt: Date
    public private(set) var deletedAt: Date?
    public private(set) var pinnedAt: Date?

    init(
        id: IdeaID = IdeaID(),
        body: String,
        categoryID: TaskCategoryID? = nil,
        labelIDs: [TaskLabelID] = [],
        now: Date
    ) throws {
        guard let normalizedBody = TaskNoteEntry.normalizedBody(body) else {
            throw NoonmarkError.invalidTransition("idea body cannot be empty")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: now)
        self.id = id
        self.body = normalizedBody
        self.categoryID = categoryID
        self.labelIDs = Self.normalizedLabelIDs(labelIDs)
        self.createdAt = now
        self.updatedAt = now
        self.deletedAt = nil
        self.pinnedAt = nil
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(IdeaID.self, forKey: .id)
        body = try container.decode(String.self, forKey: .body)
        categoryID = try container.decodeIfPresent(
            TaskCategoryID.self,
            forKey: .categoryID
        )
        labelIDs = try container.decode([TaskLabelID].self, forKey: .labelIDs)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        deletedAt = try container.decodeIfPresent(Date.self, forKey: .deletedAt)
        pinnedAt = try container.decodeIfPresent(Date.self, forKey: .pinnedAt)

        if let issue = IdeaEntryValidator.firstIssue(in: [self]) {
            throw DecodingError.dataCorrupted(
                .init(
                    codingPath: decoder.codingPath,
                    debugDescription: "Invalid idea entry: \(issue)"
                )
            )
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(body, forKey: .body)
        try container.encodeIfPresent(categoryID, forKey: .categoryID)
        try container.encode(labelIDs, forKey: .labelIDs)
        try container.encode(createdAt, forKey: .createdAt)
        try container.encode(updatedAt, forKey: .updatedAt)
        try container.encodeIfPresent(deletedAt, forKey: .deletedAt)
        try container.encodeIfPresent(pinnedAt, forKey: .pinnedAt)
    }

    public var isDeleted: Bool { deletedAt != nil }

    mutating func edit(body: String, now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("idea")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        guard let normalizedBody = TaskNoteEntry.normalizedBody(body) else {
            throw NoonmarkError.invalidTransition("idea body cannot be empty")
        }
        self.body = normalizedBody
        updatedAt = now
    }

    mutating func delete(now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("idea")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        body = ""
        categoryID = nil
        labelIDs = []
        // Tombstoning also drops the pin: projections never surface
        // tombstones, and restore returns the idea unpinned, which keeps
        // restore semantics free of resurrected pin state.
        pinnedAt = nil
        updatedAt = now
        deletedAt = now
    }

    mutating func pin(now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("idea")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        pinnedAt = now
        updatedAt = now
    }

    mutating func unpin(now: Date) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("idea")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        pinnedAt = nil
        updatedAt = now
    }

    mutating func restore(body: String, now: Date) throws {
        guard isDeleted else {
            throw NoonmarkError.invalidTransition(
                "only tombstoned ideas can be restored"
            )
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        // Tombstones strip content for sync convergence, so a restore must
        // supply a fresh non-empty body to satisfy the active-body invariant.
        guard let normalizedBody = TaskNoteEntry.normalizedBody(body) else {
            throw NoonmarkError.invalidTransition("idea body cannot be empty")
        }
        self.body = normalizedBody
        updatedAt = now
        deletedAt = nil
    }

    mutating func setClassification(
        categoryID: TaskCategoryID?,
        labelIDs: [TaskLabelID],
        now: Date
    ) throws {
        guard !isDeleted else {
            throw NoonmarkError.notFound("idea")
        }
        try TaskNoteEntry.validateMutationTime(now, notBefore: updatedAt)
        self.categoryID = categoryID
        self.labelIDs = Self.normalizedLabelIDs(labelIDs)
        updatedAt = now
    }

    static func normalizedLabelIDs(_ labelIDs: [TaskLabelID]) -> [TaskLabelID] {
        var seen = Set<TaskLabelID>()
        return labelIDs.filter { seen.insert($0).inserted }
    }
}

public struct IdeaDayGroup: Equatable, Sendable {
    public let date: LocalDate
    public let ideas: [IdeaEntry]

    public init(date: LocalDate, ideas: [IdeaEntry]) {
        self.date = date
        self.ideas = ideas
    }
}

public struct IdeaTimelineFilter: Equatable, Sendable {
    public var text: String?
    public var categoryID: TaskCategoryID?
    public var labelID: TaskLabelID?

    public init(
        text: String? = nil,
        categoryID: TaskCategoryID? = nil,
        labelID: TaskLabelID? = nil
    ) {
        self.text = text
        self.categoryID = categoryID
        self.labelID = labelID
    }
}

public struct TaskDefinition: Codable, Equatable, Sendable {
    public var id: TaskDefinitionID
    public var chainID: TaskChainID
    public var sequence: Int
    public var title: String
    public var descriptionText: String?
    public var plannedSubtasks: [PlannedSubtask]
    public var createdAt: Date
    public private(set) var contentUpdatedAt: Date
    public var supersededAt: Date?
    public var supersededByDefinitionID: TaskDefinitionID?

    public init(
        id: TaskDefinitionID = TaskDefinitionID(),
        chainID: TaskChainID,
        sequence: Int,
        title: String,
        descriptionText: String? = nil,
        plannedSubtasks: [PlannedSubtask] = [],
        now: Date,
        contentUpdatedAt: Date? = nil
    ) {
        self.id = id
        self.chainID = chainID
        self.sequence = sequence
        self.title = title
        self.descriptionText = descriptionText
        self.plannedSubtasks = plannedSubtasks.sorted { $0.position < $1.position }
        self.createdAt = now
        self.contentUpdatedAt = contentUpdatedAt ?? now
        self.supersededAt = nil
        self.supersededByDefinitionID = nil
    }

    mutating func markContentModified(at now: Date) throws {
        try ContentMutationClock.advance(&contentUpdatedAt, to: now)
    }
}

public struct PlannedSubtask: Codable, Equatable, Sendable, Identifiable {
    public var id: PlannedSubtaskID
    public var lineageID: SubtaskLineageID
    public var title: String
    public var difficulty: SubtaskDifficulty
    public var position: Int
    public var createdAt: Date

    public init(
        id: PlannedSubtaskID = PlannedSubtaskID(),
        lineageID: SubtaskLineageID = SubtaskLineageID(),
        title: String,
        difficulty: SubtaskDifficulty = .simple,
        position: Int,
        now: Date
    ) {
        self.id = id
        self.lineageID = lineageID
        self.title = title
        self.difficulty = difficulty
        self.position = position
        self.createdAt = now
    }
}

public struct DayTrace: Codable, Equatable, Sendable {
    public var id: DayTraceID
    public var chainID: TaskChainID
    public var definitionID: TaskDefinitionID
    public var date: LocalDate
    public var status: TraceStatus
    public var priority: Int
    public var pinOrder: Int?
    public var continuationSeq: Int
    public var descriptionText: String?
    public var noteEntries: [TaskNoteEntry]
    public var manualProgressPercent: Int?
    public var carriedFromTraceID: DayTraceID?
    public var changedToTraceID: DayTraceID?
    public var createdAt: Date
    public private(set) var contentUpdatedAt: Date
    public var completedAt: Date?
    public var settledAt: Date?
    public var draftCancellationID: UUID?
    public var draftCancelledOn: LocalDate?

    public var activeNoteEntries: [TaskNoteEntry] {
        noteEntries.filter { !$0.isDeleted }
    }

    /// A cancelled draft is an append-only persistence/sync fact, not a
    /// user-visible Day Todo or historical trajectory fact.
    public var formsDayHistory: Bool {
        status.formsDayHistory
    }

    public init(
        id: DayTraceID = DayTraceID(),
        chainID: TaskChainID,
        definitionID: TaskDefinitionID,
        date: LocalDate,
        status: TraceStatus = .pending,
        priority: Int,
        pinOrder: Int? = nil,
        continuationSeq: Int = 0,
        descriptionText: String? = nil,
        noteEntries: [TaskNoteEntry] = [],
        manualProgressPercent: Int? = nil,
        carriedFromTraceID: DayTraceID? = nil,
        draftCancellationID: UUID? = nil,
        draftCancelledOn: LocalDate? = nil,
        now: Date,
        contentUpdatedAt: Date? = nil
    ) {
        self.id = id
        self.chainID = chainID
        self.definitionID = definitionID
        self.date = date
        self.status = status
        self.priority = priority
        self.pinOrder = pinOrder
        self.continuationSeq = continuationSeq
        self.descriptionText = descriptionText
        self.noteEntries = noteEntries
        self.manualProgressPercent = manualProgressPercent
        self.carriedFromTraceID = carriedFromTraceID
        self.changedToTraceID = nil
        self.createdAt = now
        self.contentUpdatedAt = contentUpdatedAt ?? now
        self.completedAt = nil
        self.settledAt = nil
        self.draftCancellationID = draftCancellationID
        self.draftCancelledOn = draftCancelledOn
    }

    mutating func markContentModified(at now: Date) throws {
        try ContentMutationClock.advance(&contentUpdatedAt, to: now)
    }
}

private enum ContentMutationClock {
    static func advance(_ clock: inout Date, to now: Date) throws {
        guard clock.timeIntervalSinceReferenceDate.isFinite,
              now.timeIntervalSinceReferenceDate.isFinite,
              now >= clock
        else {
            throw NoonmarkError.invalidInput(
                "content mutation time cannot move backwards"
            )
        }
        clock = now
    }
}

public struct Subtask: Codable, Equatable, Sendable {
    public var id: SubtaskID
    public var lineageID: SubtaskLineageID
    public var traceID: DayTraceID
    public var title: String
    public var status: SubtaskStatus
    public var difficulty: SubtaskDifficulty
    public var position: Int
    public var carriedFromSubtaskID: SubtaskID?
    public var createdAt: Date
    public var updatedAt: Date
    public var completedAt: Date?
    public var settledAt: Date?
    public var draftCancellationID: UUID?

    public var isDone: Bool {
        status == .completed
    }

    public var isUserPresentable: Bool {
        status.isUserPresentable
    }

    public init(
        id: SubtaskID = SubtaskID(),
        lineageID: SubtaskLineageID = SubtaskLineageID(),
        traceID: DayTraceID,
        title: String,
        status: SubtaskStatus = .pending,
        difficulty: SubtaskDifficulty = .simple,
        position: Int,
        carriedFromSubtaskID: SubtaskID? = nil,
        draftCancellationID: UUID? = nil,
        now: Date
    ) {
        self.id = id
        self.lineageID = lineageID
        self.traceID = traceID
        self.title = title
        self.status = status
        self.difficulty = difficulty
        self.position = position
        self.carriedFromSubtaskID = carriedFromSubtaskID
        self.createdAt = now
        self.updatedAt = now
        self.completedAt = nil
        self.settledAt = nil
        self.draftCancellationID = draftCancellationID
    }

    mutating func markContentModified(at now: Date) throws {
        try ContentMutationClock.advance(&updatedAt, to: now)
    }
}

public struct SubtaskProgress: Equatable, Sendable {
    public let total: Int
    public let completed: Int
    public let pending: Int
    public let unfinished: Int
    public let deferred: Int
    public let abandoned: Int

    public init(total: Int, completed: Int, pending: Int, unfinished: Int, deferred: Int, abandoned: Int) {
        self.total = total
        self.completed = completed
        self.pending = pending
        self.unfinished = unfinished
        self.deferred = deferred
        self.abandoned = abandoned
    }

    public var isPartiallyCompleted: Bool {
        let actionableTotal = total - abandoned
        return completed > 0 && completed < actionableTotal
    }

    public var canCompleteParent: Bool {
        pending == 0 && unfinished == 0
    }
}

public struct TraceProgress: Equatable, Sendable {
    public let mode: TraceProgressMode
    public let percent: Int
    public let floorPercent: Int
    public let completedWeight: Int
    public let totalWeight: Int

    public init(mode: TraceProgressMode, percent: Int, floorPercent: Int, completedWeight: Int, totalWeight: Int) {
        self.mode = mode
        self.percent = percent
        self.floorPercent = floorPercent
        self.completedWeight = completedWeight
        self.totalWeight = totalWeight
    }
}

public struct CalendarDaySummary: Equatable, Sendable {
    public let date: LocalDate
    public let total: Int
    public let completed: Int
    public let pending: Int
    public let unfinished: Int
    public let heatLevel: Int

    public init(date: LocalDate, total: Int, completed: Int, pending: Int, unfinished: Int, heatLevel: Int) {
        self.date = date
        self.total = total
        self.completed = completed
        self.pending = pending
        self.unfinished = unfinished
        self.heatLevel = heatLevel
    }
}

public struct SubtaskTrajectoryRecord: Equatable, Sendable {
    public let subtask: Subtask
    public let date: LocalDate
}

public struct SubtaskTrajectory: Equatable, Sendable {
    public let lineageID: SubtaskLineageID
    public let title: String
    public let startDate: LocalDate
    public let continuedDates: [LocalDate]
    public let completedDate: LocalDate?
    public let records: [SubtaskTrajectoryRecord]
}

public struct PoolTask: Equatable, Sendable {
    public let chain: TaskChain
    public let definition: TaskDefinition
}

public enum TaskTrailEntryKind: String, Equatable, Hashable, Sendable {
    case createdInPool
    case createdFromChange
    case scheduled
    case completed
    case unfinished
    case deferred
    case continued
    case changed
    case returnedToPool
    case abandoned
}

/// A user-facing lifecycle projection. One trace can contribute both its
/// scheduling event and a later outcome, while the stored trace remains the
/// single stable identity used by persistence and sync.
public struct TaskTrailEntry: Equatable, Identifiable, Sendable {
    public let id: String
    public let chainID: TaskChainID
    public let traceID: DayTraceID?
    public let relatedTraceID: DayTraceID?
    public let kind: TaskTrailEntryKind
    public let date: LocalDate?
    public let occurredAt: Date

    public init(
        id: String,
        chainID: TaskChainID,
        traceID: DayTraceID?,
        relatedTraceID: DayTraceID? = nil,
        kind: TaskTrailEntryKind,
        date: LocalDate?,
        occurredAt: Date
    ) {
        self.id = id
        self.chainID = chainID
        self.traceID = traceID
        self.relatedTraceID = relatedTraceID
        self.kind = kind
        self.date = date
        self.occurredAt = occurredAt
    }
}

public struct DayTodoView: Equatable, Sendable {
    public let day: Day?
    public let traces: [DayTrace]
}

public struct FuturePlanItem: Equatable, Sendable {
    public let trace: DayTrace
    public let definition: TaskDefinition
}

public struct UnfinishedPoolItem: Equatable, Sendable {
    public let chain: TaskChain
    public let definition: TaskDefinition
    public let unfinishedTraces: [DayTrace]
    public let activeTrace: DayTrace?
    public let isInTaskPool: Bool

    public init(
        chain: TaskChain,
        definition: TaskDefinition,
        unfinishedTraces: [DayTrace],
        activeTrace: DayTrace?,
        isInTaskPool: Bool = false
    ) {
        self.chain = chain
        self.definition = definition
        self.unfinishedTraces = unfinishedTraces
        self.activeTrace = activeTrace
        self.isInTaskPool = isInTaskPool
    }

    public var isContinuedPending: Bool {
        activeTrace?.status == .pending
    }

    public var actionPlan: [UnfinishedPoolAction] {
        if chain.state == .abandoned {
            guard let source = unfinishedTraces.last(where: {
                $0.status == .abandoned
            }) else {
                return []
            }
            return [.reactivateChain(source.id)]
        }
        if let activeTrace {
            return [.abandonChain(activeTrace.id)]
        }
        if isInTaskPool {
            return [
                .openTaskPool(chain.id),
                .abandonPooledChain(chain.id),
            ]
        }
        guard let source = unfinishedTraces.last(where: {
            $0.status == .unfinished
        }) else {
            return []
        }
        return [
            .continueTrace(source.id),
            .abandonChain(source.id)
        ]
    }
}

public enum UnfinishedPoolAction: Equatable, Hashable, Sendable {
    case continueTrace(DayTraceID)
    case abandonChain(DayTraceID)
    case reactivateChain(DayTraceID)
    case openTaskPool(TaskChainID)
    case abandonPooledChain(TaskChainID)
}

public struct CompletedTaskTrajectory: Equatable, Sendable {
    public let startDate: LocalDate
    public let continuedDates: [LocalDate]
    public let completedDate: LocalDate
    public let traces: [DayTrace]
    public let subtaskTrajectories: [SubtaskTrajectory]
}

public struct CompletedPoolItem: Equatable, Sendable {
    public let trace: DayTrace
    public let definition: TaskDefinition
    public let trajectory: CompletedTaskTrajectory
}

public struct CompletedSubtaskRecord: Equatable, Sendable {
    public let date: LocalDate
    public let subtask: Subtask
    public let parentTrace: DayTrace
    public let parentDefinition: TaskDefinition
}

public struct CompletedTaskHierarchy: Equatable, Sendable {
    public let chain: TaskChain
    public let definition: TaskDefinition
    public let parentCompletion: CompletedPoolItem?
    public let completedChildren: [CompletedSubtaskRecord]

    public init(
        chain: TaskChain,
        definition: TaskDefinition,
        parentCompletion: CompletedPoolItem?,
        completedChildren: [CompletedSubtaskRecord]
    ) {
        self.chain = chain
        self.definition = definition
        self.parentCompletion = parentCompletion
        self.completedChildren = completedChildren
    }

    public var latestCompletionAt: Date {
        let parentCompletedAt = parentCompletion.map {
            $0.trace.completedAt ?? $0.trace.createdAt
        }
        return ([parentCompletedAt] + completedChildren.map {
            $0.subtask.completedAt ?? $0.subtask.updatedAt
        })
        .compactMap { $0 }
        .max() ?? definition.createdAt
    }
}

public struct DailyReviewStats: Equatable, Sendable {
    public var total: Int
    public var completed: Int
    public var unfinished: Int
    public var deferred: Int
    public var changed: Int
    public var returnedToPool: Int
    public var abandoned: Int
}
