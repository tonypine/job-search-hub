import Foundation

/// One task run on the same postings through two or more stacks, so their
/// answers can be judged side by side. The first stack is what the others
/// are measured against.
public struct Comparison: Decodable, Equatable, Identifiable, Sendable {
    public static let runningStatus = "running"

    public var id: UUID
    public var taskKind: String
    public var title: String
    public var status: String
    public var createdAt: Date
    public var stacks: [ComparisonStack]
    public var jobIDs: [UUID]

    public var isRunning: Bool { status == Self.runningStatus }

    /// "1 posting", "70 postings".
    public var postingCountText: String {
        jobIDs.count == 1 ? "1 posting" : "\(jobIDs.count) postings"
    }

    enum CodingKeys: String, CodingKey {
        case id, taskKind, title, status, createdAt, stacks
        case jobIDs = "jobIds"
    }
}

/// One way of answering: a model on a provider ("route"), Claude through
/// the CLI ("claude"), or answers brought in from elsewhere ("imported").
public struct ComparisonStack: Decodable, Equatable, Identifiable, Hashable, Sendable {
    public static let importedSource = "imported"

    public var id: UUID
    public var label: String
    public var source: String
    public var providerID: UUID?
    public var model: String?

    /// Whether the stack's answers came from elsewhere rather than a run.
    public var isImported: Bool { source == Self.importedSource }

    enum CodingKeys: String, CodingKey {
        case id, label, source, model
        case providerID = "providerId"
    }
}

public struct ComparisonsResponse: Decodable, Sendable {
    public var comparisons: [Comparison]
}

/// A posting in a comparison, as its list shows it.
public struct ComparisonJob: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var title: String
    public var companyName: String?
}

/// One leaf of a stack's answer: its path ("location.open_to_brazil"), its
/// value as text, and what the stack quoted or gave as its reason.
public struct ComparisonReading: Decodable, Equatable, Sendable {
    public var field: String
    public var text: String
    public var evidence: String?
    public var reason: String?
}

/// A stack's answer on one posting: its readings, or why it has none.
public struct ComparisonAnswer: Decodable, Equatable, Sendable {
    public var stackID: UUID
    public var jobID: UUID
    public var error: String?
    public var readings: [ComparisonReading]

    enum CodingKeys: String, CodingKey {
        case error, readings
        case stackID = "stackId"
        case jobID = "jobId"
    }
}

public enum ComparisonVerdictKind: String, Codable, Sendable {
    case right
    case wrong
}

/// The owner's judgment of one stack's field on one posting.
public struct ComparisonVerdict: Codable, Equatable, Sendable {
    public var stackID: UUID
    public var jobID: UUID
    public var field: String
    public var verdict: ComparisonVerdictKind

    public init(stackID: UUID, jobID: UUID, field: String, verdict: ComparisonVerdictKind) {
        self.stackID = stackID
        self.jobID = jobID
        self.field = field
        self.verdict = verdict
    }

    enum CodingKeys: String, CodingKey {
        case field, verdict
        case stackID = "stackId"
        case jobID = "jobId"
    }
}

/// One field of one stack: on how many postings it agreed with the first
/// stack, out of those both answered, and the owner's verdicts.
public struct ComparisonFieldScore: Decodable, Equatable, Sendable {
    public var field: String
    public var agreed: Int
    public var compared: Int
    public var right: Int
    public var wrong: Int

    /// "68 of 70", or "—" when nothing was compared.
    public var agreementText: String {
        compared == 0 ? "—" : "\(agreed) of \(compared)"
    }
}

public struct ComparisonStackSummary: Decodable, Equatable, Sendable {
    public var stackID: UUID
    public var answered: Int
    public var failed: Int
    public var fields: [ComparisonFieldScore]

    enum CodingKeys: String, CodingKey {
        case answered, failed, fields
        case stackID = "stackId"
    }
}

public struct ComparisonSummary: Decodable, Equatable, Sendable {
    public var fields: [String]
    public var stacks: [ComparisonStackSummary]

    /// A field's path as a title: "location.open_to_brazil" reads
    /// "Location · open to brazil".
    public static func formatFieldTitle(_ field: String) -> String {
        let title = field.split(separator: ".").map { $0.replacingOccurrences(of: "_", with: " ") }.joined(separator: " · ")
        return title.prefix(1).uppercased() + title.dropFirst()
    }
}

/// A comparison with its postings, every answer and verdict, and its summary.
public struct ComparisonRecord: Decodable, Equatable, Sendable {
    public var comparison: Comparison
    public var jobs: [ComparisonJob]
    public var answers: [ComparisonAnswer]
    public var verdicts: [ComparisonVerdict]
    public var summary: ComparisonSummary

    enum CodingKeys: String, CodingKey {
        case jobs, answers, verdicts, summary
    }

    public init(from decoder: any Decoder) throws {
        comparison = try Comparison(from: decoder)
        let container = try decoder.container(keyedBy: CodingKeys.self)
        jobs = try container.decode([ComparisonJob].self, forKey: .jobs)
        answers = try container.decode([ComparisonAnswer].self, forKey: .answers)
        verdicts = try container.decode([ComparisonVerdict].self, forKey: .verdicts)
        summary = try container.decode(ComparisonSummary.self, forKey: .summary)
    }

    public func getAnswer(stackID: UUID, jobID: UUID) -> ComparisonAnswer? {
        answers.first { $0.stackID == stackID && $0.jobID == jobID }
    }

    public func getVerdict(stackID: UUID, jobID: UUID, field: String) -> ComparisonVerdictKind? {
        verdicts.first { $0.stackID == stackID && $0.jobID == jobID && $0.field == field }?.verdict
    }

    public func getScore(stackID: UUID, field: String) -> ComparisonFieldScore? {
        summary.stacks.first { $0.stackID == stackID }?.fields.first { $0.field == field }
    }

    /// Whether every stack read the field the same on the posting, comparing
    /// their texts as the summary does: trimmed and in lowercase.
    public func isAgreed(jobID: UUID, field: String) -> Bool {
        let texts = comparison.stacks.map { stack in
            getAnswer(stackID: stack.id, jobID: jobID)?.readings.first { $0.field == field }?.text
                .trimmingCharacters(in: .whitespaces).lowercased()
        }
        return Set(texts).count == 1 && texts.first != nil
    }
}

/// A stack to run in a new comparison: Claude with a model, or a model on a
/// provider.
public struct NewComparisonStack: Encodable, Equatable, Sendable {
    public static let claudeSource = "claude"
    public static let routeSource = "route"

    public var label: String
    public var source: String
    public var providerID: UUID?
    public var model: String

    public init(label: String, source: String, providerID: UUID?, model: String) {
        self.label = label
        self.source = source
        self.providerID = providerID
        self.model = model
    }

    enum CodingKeys: String, CodingKey {
        case label, source, model
        case providerID = "providerId"
    }
}

/// A comparison to run on the newest postings with text.
public struct NewComparison: Encodable, Equatable, Sendable {
    public var title: String
    public var freshJobs: Int
    public var stacks: [NewComparisonStack]

    public init(title: String, freshJobs: Int, stacks: [NewComparisonStack]) {
        self.title = title
        self.freshJobs = freshJobs
        self.stacks = stacks
    }
}

struct ComparisonVerdictsBody: Encodable, Sendable {
    var verdicts: [ComparisonVerdict]
}

public extension HubClient {
    func getComparisons() async throws -> [Comparison] {
        try await get("v1/comparisons", as: ComparisonsResponse.self).comparisons
    }

    func getComparison(_ id: UUID) async throws -> ComparisonRecord {
        try await get("v1/comparisons/\(id.uuidString)", as: ComparisonRecord.self)
    }

    /// Starts the comparison; it runs in the background on the hub.
    func startComparison(_ comparison: NewComparison) async throws -> ComparisonRecord {
        try await send("POST", "v1/comparisons", body: comparison, as: ComparisonRecord.self)
    }

    func saveComparisonVerdicts(_ verdicts: [ComparisonVerdict], comparisonID: UUID) async throws -> ComparisonRecord {
        try await send("PUT", "v1/comparisons/\(comparisonID.uuidString)/verdicts", body: ComparisonVerdictsBody(verdicts: verdicts), as: ComparisonRecord.self)
    }
}
