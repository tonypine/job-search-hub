import Foundation

/// How well the owner matches a job, as its brief judges it.
public enum JobMatch: String, Codable, Sendable {
    case strong, possible, stretch, mismatch

    public var title: String { rawValue.capitalized }
}

/// A knowledge-base entry a brief cites.
public struct CitedEntry: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var title: String
    public var organization: String?

    /// The entry as a citation reads: its title, and where when it has one.
    public var label: String {
        guard let organization, !organization.isEmpty else { return title }
        return "\(title) · \(organization)"
    }
}

/// One strength or weakness, with the entries it rests on.
public struct JobBriefPoint: Codable, Equatable, Sendable {
    public var point: String
    public var entryIDs: [UUID]

    enum CodingKeys: String, CodingKey {
        case point
        case entryIDs = "entryIds"
    }
}

/// A job's brief: the match, why, and the strengths and weaknesses behind
/// it. A pre-brief comes from the local model, a full brief from Claude.
public struct JobBrief: Codable, Equatable, Sendable {
    public static let fullTier = "full"

    public var tier: String
    public var model: String
    public var match: JobMatch
    public var reason: String
    public var strengths: [JobBriefPoint]
    public var weaknesses: [JobBriefPoint]
    public var writtenAt: Date
    /// The knowledge base, profile or criteria changed since it was written.
    public var isStale: Bool
    public var citedEntries: [CitedEntry]

    public var isFull: Bool { tier == Self.fullTier }

    /// The cited entries a point rests on, in the point's order; entries
    /// since deleted are left out.
    public func getEntries(of point: JobBriefPoint) -> [CitedEntry] {
        point.entryIDs.compactMap { id in citedEntries.first { $0.id == id } }
    }
}

/// One reason a posting could screen the owner out at once. A nil verdict is
/// information the fit doesn't judge, such as the contract.
public struct ScreenOutAnswer: Codable, Equatable, Identifiable, Sendable {
    public var name: String
    public var verdict: FitVerdict?
    public var answer: String
    public var evidence: String?

    public var id: String { name }
}

public enum JobDecisionKind: String, Codable, Sendable {
    case pursue, skip, later

    /// The decision as its record reads: "Pursued", "Skipped", "Left for later".
    public var pastTense: String {
        switch self {
        case .pursue: "Pursued"
        case .skip: "Skipped"
        case .later: "Left for later"
        }
    }
}

/// The owner's latest decision on a job.
public struct JobDecision: Codable, Equatable, Sendable {
    public var decision: JobDecisionKind
    public var reason: String?
    public var decidedAt: Date
}

public struct JobDecisionRequest: Encodable, Equatable, Sendable {
    public var decision: JobDecisionKind
    public var reason: String

    public init(decision: JobDecisionKind, reason: String = "") {
        self.decision = decision
        self.reason = reason
    }
}

struct FullBriefResponse: Decodable {
    var queued: Bool
}

public extension HubClient {
    /// Records the decision and acts on it: pursue puts the job on the
    /// pipeline, skip dismisses it, later only records.
    func decideJob(_ id: UUID, _ decision: JobDecisionKind, reason: String = "") async throws -> JobDecision {
        let request = JobDecisionRequest(decision: decision, reason: reason.trimmingCharacters(in: .whitespacesAndNewlines))
        return try await send("POST", "v1/jobs/\(id.uuidString)/decision", body: request, as: JobDecision.self)
    }

    /// Asks Claude for the job's full brief; it's written in the background.
    func writeFullBrief(_ id: UUID) async throws {
        _ = try await send("POST", "v1/jobs/\(id.uuidString)/brief/full", body: EmptyBody(), as: FullBriefResponse.self)
    }
}
