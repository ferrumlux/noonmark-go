import Foundation

public enum IdeaCollectionQuery: Equatable, Sendable {
    case recent
    case search(text: String)
    case category(TaskCategoryID)
    case label(TaskLabelID)
    case filtered(IdeaTimelineFilter)
    case review(seed: UInt64, count: Int, excludingRecentDays: Int)
    case trash
}

public struct IdeaCollectionProjection: Equatable, Sendable {
    public let query: IdeaCollectionQuery
    public let ideas: [IdeaEntry]
    public let pinnedIdeas: [IdeaEntry]
    public let groups: [IdeaDayGroup]

    public init(
        query: IdeaCollectionQuery,
        ideas: [IdeaEntry],
        pinnedIdeas: [IdeaEntry] = [],
        groups: [IdeaDayGroup]
    ) {
        self.query = query
        self.ideas = ideas
        self.pinnedIdeas = pinnedIdeas
        self.groups = groups
    }
}

public extension NoonmarkEngine {
    /// Presents one explicit Flylight collection at a time. Query rules stay in
    /// the domain boundary so the page, inspector, and future compact layouts
    /// cannot silently disagree about visibility or ordering.
    func ideaCollection(
        _ query: IdeaCollectionQuery,
        today: LocalDate,
        calendar: Calendar = .current
    ) -> IdeaCollectionProjection {
        let timeline: [IdeaEntry]
        let pinned: [IdeaEntry]
        switch query {
        case .recent:
            let filter = IdeaTimelineFilter()
            timeline = ideaTimeline(filter: filter)
            pinned = pinnedIdeas(filter: filter)
        case let .search(text):
            let filter = IdeaTimelineFilter(text: text)
            timeline = ideaTimeline(filter: filter)
            pinned = pinnedIdeas(filter: filter)
        case let .category(categoryID):
            let filter = IdeaTimelineFilter(categoryID: categoryID)
            timeline = ideaTimeline(filter: filter)
            pinned = pinnedIdeas(filter: filter)
        case let .label(labelID):
            let filter = IdeaTimelineFilter(labelID: labelID)
            timeline = ideaTimeline(filter: filter)
            pinned = pinnedIdeas(filter: filter)
        case let .filtered(filter):
            timeline = ideaTimeline(filter: filter)
            pinned = pinnedIdeas(filter: filter)
        case let .review(seed, count, excludingRecentDays):
            timeline = reviewIdeas(
                seed: seed,
                count: count,
                excludingRecentDays: excludingRecentDays,
                today: today,
                calendar: calendar
            )
            pinned = []
        case .trash:
            timeline = ideaTrash()
            pinned = []
        }

        // Sticky Note is a projection over Flylight, not a collection that
        // removes its source entries. Keep the compatibility `pinnedIdeas`
        // projection while presenting every active entry in the source stream.
        let sourceTimeline: [IdeaEntry] = switch query {
        case .trash:
            timeline
        default:
            (pinned + timeline)
                .sorted(by: Self.ideaRecencyOrder)
        }

        return IdeaCollectionProjection(
            query: query,
            ideas: sourceTimeline,
            pinnedIdeas: pinned,
            groups: ideaGroups(
                for: sourceTimeline,
                query: query,
                calendar: calendar
            )
        )
    }

    private func activeIdeasOrderedByRecency() -> [IdeaEntry] {
        ideas.values
            .filter { $0.isDeleted == false }
            .sorted(by: Self.ideaRecencyOrder)
    }

    private func reviewIdeas(
        seed: UInt64,
        count: Int,
        excludingRecentDays: Int,
        today: LocalDate,
        calendar: Calendar
    ) -> [IdeaEntry] {
        guard count > 0,
              let todayStart = calendar.date(
                  from: DateComponents(
                      year: today.year,
                      month: today.month,
                      day: today.day
                  )
              ),
              let cutoff = calendar.date(
                  byAdding: .day,
                  value: -max(0, excludingRecentDays),
                  to: todayStart
              )
        else {
            return []
        }

        return activeIdeasOrderedByRecency()
            .filter { $0.createdAt < cutoff }
            .sorted {
                let lhs = Self.reviewRank(for: $0.id, seed: seed)
                let rhs = Self.reviewRank(for: $1.id, seed: seed)
                if lhs != rhs {
                    return lhs < rhs
                }
                return Self.ideaRecencyOrder($0, $1)
            }
            .prefix(count)
            .sorted(by: Self.ideaRecencyOrder)
    }

    private func ideaGroups(
        for selected: [IdeaEntry],
        query: IdeaCollectionQuery,
        calendar: Calendar
    ) -> [IdeaDayGroup] {
        var byDate: [LocalDate: [IdeaEntry]] = [:]
        for idea in selected {
            let timestamp = query == .trash
                ? idea.deletedAt ?? idea.updatedAt
                : idea.createdAt
            let components = calendar.dateComponents(
                [.year, .month, .day],
                from: timestamp
            )
            guard let year = components.year,
                  let month = components.month,
                  let day = components.day
            else {
                continue
            }
            let date = LocalDate(year: year, month: month, day: day)
            byDate[date, default: []].append(idea)
        }
        return byDate
            .map { IdeaDayGroup(date: $0.key, ideas: $0.value) }
            .sorted { $0.date > $1.date }
    }

    private static func ideaRecencyOrder(
        _ lhs: IdeaEntry,
        _ rhs: IdeaEntry
    ) -> Bool {
        if lhs.createdAt != rhs.createdAt {
            return lhs.createdAt > rhs.createdAt
        }
        return lhs.id.description > rhs.id.description
    }

    private static func reviewRank(
        for id: IdeaID,
        seed: UInt64
    ) -> UInt64 {
        var hash = 1_469_598_103_934_665_603 ^ seed
        for byte in id.description.utf8 {
            hash ^= UInt64(byte)
            hash &*= 1_099_511_628_211
        }
        return hash
    }
}
