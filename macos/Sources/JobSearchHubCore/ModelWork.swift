import Foundation

/// The hub's model work: whether background work is paused, the call
/// running, the calls waiting, the local runtime's state, and how many jobs
/// wait for facts.
public struct ModelWork: Decodable, Equatable, Sendable {
    public var paused: Bool
    public var running: ModelWorkTicket?
    public var waiting: [ModelWorkTicket]
    public var runtime: ModelRuntimeStatus
    public var jobsAwaitingFacts: Int

    /// Where a dispatched read of the job's facts stands.
    public func getFactsReadState(for jobID: UUID) -> JobFactsReadState {
        if running?.subjectID == jobID, running?.kind == ModelWorkTicket.jobFactsKind {
            return .running
        }
        if waiting.contains(where: { $0.subjectID == jobID && $0.kind == ModelWorkTicket.jobFactsKind }) {
            return .queued
        }
        return .notQueued
    }

    /// What the local models are doing, in a few words for the sidebar's
    /// foot: "Job facts · 3 waiting", "2 waiting", "Local models idle".
    public var statusLine: String {
        if paused { return waiting.isEmpty ? "Local models paused" : "Local models paused · \(waiting.count) waiting" }
        let waitingText = "\(waiting.count) waiting"
        guard let running else { return waiting.isEmpty ? "Local models idle" : waitingText }
        let kind = RunsSummary.getKindTitle(running.kind)
        return waiting.isEmpty ? kind : "\(kind) · \(waitingText)"
    }

    /// The local models' state in a word, for Settings' status row:
    /// Paused, Working or Idle.
    public var stateTitle: String {
        if paused { return "Paused" }
        return running == nil ? "Idle" : "Working"
    }

    public var stateTone: Tone {
        if paused { return .caution }
        return running == nil ? .neutral : .positive
    }

    /// What follows the state: the call running and on which model, and how
    /// many wait.
    public var stateDetail: String? {
        var parts: [String] = []
        if !paused, let running {
            parts.append("\(RunsSummary.getKindTitle(running.kind)) on \(running.model)")
        }
        if !waiting.isEmpty {
            parts.append("\(waiting.count) waiting")
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    /// How many calls wait, by the kind's title.
    public var waitingCountsByKind: [(title: String, count: Int)] {
        Dictionary(grouping: waiting, by: \.kind)
            .map { (title: RunsSummary.getKindTitle($0.key), count: $0.value.count) }
            .sorted { $0.title < $1.title }
    }
}

public struct ModelWorkTicket: Decodable, Equatable, Sendable {
    static let jobFactsKind = "job_facts"

    public var kind: String
    public var subjectID: UUID?
    public var model: String
    /// dispatched, sorting or background.
    public var priority: String
    /// When it started waiting, or running.
    public var since: Date

    enum CodingKeys: String, CodingKey {
        case kind, model, priority, since
        case subjectID = "subjectId"
    }
}

public struct ModelRuntimeStatus: Decodable, Equatable, Sendable {
    /// stopped, loading or ready.
    public var state: String
    public var model: String?
    public var busy: Bool
    public var loadedAt: Date?
    public var lastUsedAt: Date?
}

public enum JobFactsReadState: Equatable, Sendable {
    case queued
    case running
    case notQueued
}
