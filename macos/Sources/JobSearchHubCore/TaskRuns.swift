import Foundation

/// One request the hub made to a model: what it was for, which model
/// answered, how long it took, and how it went.
public struct TaskRun: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var subjectID: UUID?
    public var baseURL: String
    public var model: String
    public var promptVersion: Int?
    public var promptTokens: Int
    public var completionTokens: Int
    public var startedAt: Date
    public var durationMS: Int
    public var outcome: String
    public var error: String?

    enum CodingKeys: String, CodingKey {
        case id, kind, model, promptVersion, promptTokens, completionTokens, startedAt, outcome, error
        case subjectID = "subjectId"
        case baseURL = "baseUrl"
        case durationMS = "durationMs"
    }
}

public struct TaskRunsResponse: Codable, Sendable {
    public var runs: [TaskRun]
}

/// A Claude agent run: company research, the job finder, the knowledge-base
/// build.
public struct AgentRun: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var input: String
    public var status: String
    public var costUSDEstimate: String?
    public var error: String?
    public var startedAt: Date
    public var finishedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id, kind, input, status, error, startedAt, finishedAt
        case costUSDEstimate = "costUsdEstimate"
    }

    public var isRunning: Bool { status == "running" }
}

public struct AgentRunsResponse: Codable, Sendable {
    public var runs: [AgentRun]
}

/// One kind of run at a glance: how many ran, how many went wrong, and how
/// long each model took on average.
public struct RunKindSummary: Identifiable, Equatable, Sendable {
    public struct ModelTiming: Identifiable, Equatable, Sendable {
        public var model: String
        public var runCount: Int
        public var averageSeconds: Double

        public var id: String { model }
    }

    public var kind: String
    public var runCount: Int
    public var failedCount: Int
    public var invalidCount: Int
    public var models: [ModelTiming]

    public var id: String { kind }
    public var title: String { RunsSummary.getKindTitle(kind) }
}

public enum RunsSummary {
    /// The model an agent run's time is shown under: agents run on Claude.
    public static let agentModelName = "Claude"

    public static func getKindTitle(_ kind: String) -> String {
        switch kind {
        case "job_facts": "Job facts"
        case "mail_triage": "Mail sorting"
        case "linkedin_conversation": "LinkedIn conversations"
        case "company_triage": "Company research"
        case "job_finder": "Find jobs"
        case "profile_seed": "Knowledge base build"
        default: kind.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }

    /// Summaries per kind, the most runs first. Running agent runs aren't
    /// timed yet, so they count but add no time.
    public static func summarize(taskRuns: [TaskRun], agentRuns: [AgentRun]) -> [RunKindSummary] {
        struct Timed {
            let kind: String
            let model: String
            let seconds: Double?
            let outcome: String
        }
        let timed = taskRuns.map { Timed(kind: $0.kind, model: $0.model, seconds: Double($0.durationMS) / 1000, outcome: $0.outcome) }
            + agentRuns.map { run in
                Timed(kind: run.kind, model: agentModelName, seconds: run.finishedAt.map { $0.timeIntervalSince(run.startedAt) }, outcome: run.status)
            }
        return Dictionary(grouping: timed, by: \.kind).map { kind, runs in
            let models = Dictionary(grouping: runs, by: \.model).map { model, modelRuns in
                let seconds = modelRuns.compactMap(\.seconds)
                return RunKindSummary.ModelTiming(
                    model: model, runCount: modelRuns.count, averageSeconds: seconds.isEmpty ? 0 : seconds.reduce(0, +) / Double(seconds.count)
                )
            }
            .sorted { $0.runCount > $1.runCount }
            return RunKindSummary(
                kind: kind, runCount: runs.count, failedCount: runs.count { $0.outcome == "failed" },
                invalidCount: runs.count { $0.outcome == "invalid" }, models: models
            )
        }
        .sorted { $0.runCount != $1.runCount ? $0.runCount > $1.runCount : $0.title < $1.title }
    }
}
