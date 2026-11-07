import Foundation

public struct LocalDate: Codable, Hashable, Comparable, Sendable, CustomStringConvertible {
    public let year: Int
    public let month: Int
    public let day: Int

    public init(year: Int, month: Int, day: Int) {
        precondition((1...12).contains(month), "month must be 1...12")
        precondition((1...31).contains(day), "day must be 1...31")
        self.year = year
        self.month = month
        self.day = day
    }

    public init(_ iso8601Date: String) {
        let parts = iso8601Date.split(separator: "-").map(String.init)
        precondition(parts.count == 3, "date must use yyyy-mm-dd")
        self.init(
            year: Int(parts[0])!,
            month: Int(parts[1])!,
            day: Int(parts[2])!
        )
    }

    public init?(validatingISO8601Date value: String) {
        let components = value.split(
            separator: "-",
            omittingEmptySubsequences: false
        )
        guard components.count == 3,
              components[0].count == 4,
              components[1].count == 2,
              components[2].count == 2,
              let year = Int(components[0]),
              let month = Int(components[1]),
              let day = Int(components[2]),
              (1 ... 12).contains(month),
              (1 ... 31).contains(day)
        else {
            return nil
        }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        guard let date = calendar.date(
            from: DateComponents(
                year: year,
                month: month,
                day: day
            )
        ) else {
            return nil
        }
        let resolved = calendar.dateComponents(
            [.year, .month, .day],
            from: date
        )
        guard resolved.year == year,
              resolved.month == month,
              resolved.day == day
        else {
            return nil
        }
        self.init(year: year, month: month, day: day)
    }

    public var description: String {
        String(format: "%04d-%02d-%02d", year, month, day)
    }

    public static func < (lhs: LocalDate, rhs: LocalDate) -> Bool {
        (lhs.year, lhs.month, lhs.day) < (rhs.year, rhs.month, rhs.day)
    }
}
