import Foundation

/// How well a job fits the criteria, as the hub judged it. Poor when any
/// check says no, good when every check says yes, unclear otherwise.
public struct JobFit: Codable, Equatable, Sendable {
    public var level: FitLevel
    public var checks: [FitCheck]
}

public enum FitLevel: String, Codable, Comparable, Sendable {
    case good, unclear, poor

    public var title: String { rawValue.capitalized }

    /// Good sorts first, poor last.
    private var rank: Int {
        switch self {
        case .good: 0
        case .unclear: 1
        case .poor: 2
        }
    }

    public static func < (left: FitLevel, right: FitLevel) -> Bool { left.rank < right.rank }
}

public struct FitCheck: Codable, Equatable, Identifiable, Sendable {
    public var name: String
    public var verdict: FitVerdict
    public var reason: String

    public var id: String { name }
}

public enum FitVerdict: String, Codable, Sendable {
    case yes, no, unclear
}

public enum JobsOrder {
    /// Best fit first, then newest first.
    public static func sort(_ items: [JobListItem]) -> [JobListItem] {
        items.sorted { left, right in
            if left.fit.level != right.fit.level {
                return left.fit.level < right.fit.level
            }
            return left.job.firstSeenAt > right.job.firstSeenAt
        }
    }

    /// Hides the jobs judged poor; good and unclear stay.
    public static func hidePoorFits(_ items: [JobListItem]) -> [JobListItem] {
        items.filter { $0.fit.level != .poor }
    }
}
