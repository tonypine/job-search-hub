import Foundation

/// A briefed job waiting for the owner's decision.
public struct DecisionQueueItem: Decodable, Equatable, Identifiable, Sendable {
    public var job: Job
    public var companyName: String?
    public var match: JobMatch
    public var reason: String
    public var briefTier: String
    public var fit: JobFit
    /// Set when the job was left for later.
    public var decision: JobDecision?

    public var id: UUID { job.id }
}

public struct DecisionQueueResponse: Decodable, Sendable {
    public var items: [DecisionQueueItem]
    public var total: Int
}

/// How deciding goes over a period: the decisions by kind, how long jobs
/// waited for them, and how many good or unclear jobs seen got decided.
public struct DecisionSignals: Decodable, Equatable, Sendable {
    public var since: Date
    public var decisions: [String: Int]
    public var medianHoursToDecide: Double?
    public var goodOrUnclearSeen: Int
    public var goodOrUnclearDecided: Int

    /// One line for the Decide page: "This week: 1 pursued, 2 skipped, 0 for
    /// later · median 36 hours to decide · 9 of 127 good or unclear jobs decided".
    public var summary: String {
        let counts = "This week: \(decisions["pursue"] ?? 0) pursued, \(decisions["skip"] ?? 0) skipped, \(decisions["later"] ?? 0) for later"
        var parts = [counts]
        if let hours = medianHoursToDecide {
            parts.append("median \(Self.formatWait(hours)) to decide")
        }
        parts.append("\(goodOrUnclearDecided) of \(goodOrUnclearSeen) good or unclear jobs decided")
        return parts.joined(separator: " · ")
    }

    /// A wait in hours as people say it: "40 minutes", "5 hours", "1.5 days".
    static func formatWait(_ hours: Double) -> String {
        if hours < 1 {
            return formatCount(Int((hours * 60).rounded()), "minute")
        }
        if hours < 48 {
            return formatCount(Int(hours.rounded()), "hour")
        }
        let days = (hours / 24).formatted(.number.precision(.fractionLength(0...1)))
        return days == "1" ? "1 day" : "\(days) days"
    }

    private static func formatCount(_ count: Int, _ unit: String) -> String {
        count == 1 ? "1 \(unit)" : "\(count) \(unit)s"
    }
}

public extension HubClient {
    func getDecisionQueue() async throws -> DecisionQueueResponse {
        try await get("v1/decision-queue", as: DecisionQueueResponse.self)
    }

    func getDecisionSignals() async throws -> DecisionSignals {
        try await get("v1/decision-signals", as: DecisionSignals.self)
    }
}
