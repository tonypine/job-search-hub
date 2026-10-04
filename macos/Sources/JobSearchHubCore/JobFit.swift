import Foundation

/// The job's screen: whether a rule of the criteria rules it out, as the hub
/// judged it. Poor (Fails) when any check says no, good (Passes) when every
/// check says yes, unclear otherwise. The hub calls it the fit.
public struct JobFit: Codable, Equatable, Sendable {
    public var level: FitLevel
    public var checks: [FitCheck]
}

public enum FitLevel: String, Codable, Comparable, Sendable {
    case good, unclear, poor

    /// The screen's word: "Passes", "Unclear" or "Fails".
    public var title: String {
        switch self {
        case .good: "Passes"
        case .unclear: "Unclear"
        case .poor: "Fails"
        }
    }

    /// The screen as a chip on its own reads it: "Passes screen".
    public var label: String {
        switch self {
        case .good: "Passes screen"
        case .unclear: "Screen unclear"
        case .poor: "Fails screen"
        }
    }

    /// Good sorts first, poor last.
    var rank: Int {
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
    /// Passes the screen first, then newest first.
    public static func sort(_ items: [JobListItem]) -> [JobListItem] {
        items.sorted { left, right in
            if left.fit.level != right.fit.level {
                return left.fit.level < right.fit.level
            }
            return left.job.firstSeenAt > right.job.firstSeenAt
        }
    }
}
