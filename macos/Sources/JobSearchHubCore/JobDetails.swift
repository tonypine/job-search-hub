import Foundation

/// Everything the hub knows about one job: the job with its board facts, its
/// company, the facts read from its text, and its place on the pipeline.
public struct JobDetails: Decodable, Equatable, Sendable {
    public var job: Job
    public var companyName: String?
    public var facts: LabelledJobFacts?
    public var application: Application?
    public var phase: PipelinePhase?
    public var fit: JobFit
    public var unseenUpdates: Int
    /// The owner's connections at the job's company.
    public var connections: [Connection]?
    /// The full brief, or else the pre-brief; nil until briefed.
    public var brief: JobBrief?
    public var screenOut: [ScreenOutAnswer]?
    /// The owner's latest decision; nil while undecided.
    public var decision: JobDecision?
    /// The job's tailored CV; nil until drafted.
    public var cvID: UUID?

    enum CodingKeys: String, CodingKey {
        case job, companyName, facts, application, phase, fit, unseenUpdates, connections, brief, screenOut, decision
        case cvID = "cvId"
    }
}

/// One row of a job's Screen: a rule, its verdict, why, and the posting's
/// words when there are some. A nil verdict is information no rule judges.
public struct ScreenRow: Equatable, Sendable {
    public var name: String
    public var verdict: FitVerdict?
    public var reason: String
    public var evidence: String?
}

public extension JobDetails {
    /// The job's Screen: the screen-out answers, then the fit checks they
    /// don't already answer. The hub answers some screen-out checks from a fit
    /// check under another name, with the check's verdict and reason, so those
    /// show once.
    var screenRows: [ScreenRow] {
        let answers = screenOut ?? []
        let answerRows = answers.map { ScreenRow(name: $0.name, verdict: $0.verdict, reason: $0.answer, evidence: $0.evidence) }
        let checkRows = fit.checks
            .filter { check in !answers.contains { $0.verdict == check.verdict && $0.answer == check.reason } }
            .map { ScreenRow(name: $0.name, verdict: $0.verdict, reason: $0.reason) }
        return answerRows + checkRows
    }
}

/// A job's facts in the order and with the labels of the prompt version they
/// were read under.
public struct LabelledJobFacts: Decodable, Equatable, Sendable {
    public var entries: [JobFactEntry]
    public var promptVersion: Int
    public var model: String
    public var extractedAt: Date
}

public struct JobFactEntry: Decodable, Equatable, Identifiable, Sendable {
    public var key: String
    public var title: String
    public var description: String?
    public var value: JSONValue
    /// The posting's words the fact rests on, when it was read with them.
    public var evidence: String?

    public var id: String { key }

    /// What the detail view shows: text, a list, or that the posting doesn't say.
    public var display: JobFactDisplay {
        switch value {
        case .null:
            return .notStated
        case let .string(text):
            let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
            return trimmed.isEmpty || trimmed.lowercased() == "not stated" ? .notStated : .text(trimmed)
        case let .array(items):
            let texts = items.compactMap(\.plainText).filter { !$0.isEmpty }
            return texts.isEmpty ? .notStated : .list(texts)
        default:
            return value.plainText.map(JobFactDisplay.text) ?? .notStated
        }
    }
}

public enum JobFactDisplay: Equatable, Sendable {
    case text(String)
    case list([String])
    case notStated
}

/// Any JSON value, for facts whose shape the prompt decides.
public enum JSONValue: Codable, Equatable, Sendable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case null
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: any Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let bool = try? container.decode(Bool.self) {
            self = .bool(bool)
        } else if let number = try? container.decode(Double.self) {
            self = .number(number)
        } else if let string = try? container.decode(String.self) {
            self = .string(string)
        } else if let array = try? container.decode([JSONValue].self) {
            self = .array(array)
        } else {
            self = .object(try container.decode([String: JSONValue].self))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case let .string(text): try container.encode(text)
        case let .number(number): try container.encode(number)
        case let .bool(bool): try container.encode(bool)
        case .null: try container.encodeNil()
        case let .array(items): try container.encode(items)
        case let .object(members): try container.encode(members)
        }
    }

    /// A scalar as text: whole numbers without decimals, booleans as Yes or
    /// No; nil for null, arrays and objects.
    public var plainText: String? {
        switch self {
        case let .string(text): text
        case let .number(number): number.rounded() == number ? String(Int(number)) : String(number)
        case let .bool(bool): bool ? "Yes" : "No"
        case .null, .array, .object: nil
        }
    }
}
