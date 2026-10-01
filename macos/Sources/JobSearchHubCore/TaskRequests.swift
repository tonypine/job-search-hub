import Foundation

/// Work asked of the Mac, which is the only place agents run.
public struct TaskRequest: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var companyID: UUID?
    public var jobID: UUID?
    public var input: String?
    public var status: String

    enum CodingKeys: String, CodingKey {
        case id, kind, input, status
        case companyID = "companyId"
        case jobID = "jobId"
    }

    public static let findJobs = "find_jobs"
    public static let researchCompany = "research_company"
    /// Corrects a job's details from the owner's note, the task's input.
    public static let fixJob = "fix_job"
}

/// A request to fix a job's details from a note on what's wrong. A claimed
/// fix starts running for the asker, so no other runner takes it.
public struct QueueJobFixRequest: Encodable, Sendable {
    public var kind = TaskRequest.fixJob
    public var jobID: UUID
    public var note: String
    public var claim: Bool

    public init(jobID: UUID, note: String, claim: Bool) {
        self.jobID = jobID
        self.note = note
        self.claim = claim
    }

    enum CodingKeys: String, CodingKey {
        case kind, note, claim
        case jobID = "jobId"
    }
}

/// How the app fixes a job: `hub job fix`, whose last line is the agent's
/// summary, starting "Fixed:" when it changed something.
public enum JobFixLaunch {
    public static func makeArguments(jobID: UUID, note: String) -> [String] {
        ["job", "fix", jobID.uuidString.lowercased(), "--note", note.trimmingCharacters(in: .whitespacesAndNewlines)]
    }

    /// The fix's outcome from what the command printed: its last line, and
    /// whether it says something was fixed.
    public static func parseOutcome(_ output: String, status: Int32) -> (fixed: Bool, summary: String) {
        let lastLine = output.components(separatedBy: "\n").last { !$0.trimmingCharacters(in: .whitespaces).isEmpty }?
            .trimmingCharacters(in: .whitespaces) ?? ""
        let fixed = status == 0 && lastLine.hasPrefix("Fixed:")
        return (fixed, lastLine.isEmpty ? "exit status \(status)" : lastLine)
    }
}

public extension HubClient {
    /// Queues a fix for the job. A claimed one is the caller's to run.
    func queueJobFix(_ jobID: UUID, note: String, claim: Bool) async throws -> TaskRequest {
        try await send("POST", "v1/tasks", body: QueueJobFixRequest(jobID: jobID, note: note, claim: claim), as: TaskRequest.self)
    }
}

public struct TasksResponse: Decodable, Sendable {
    public var tasks: [TaskRequest]
}

/// How a task ended: the first line of the result becomes the update's title.
public struct FinishTaskRequest: Encodable, Sendable {
    public var succeeded: Bool
    public var result: String
    public var companyID: UUID?

    public init(succeeded: Bool, result: String, companyID: UUID?) {
        self.succeeded = succeeded
        self.result = result
        self.companyID = companyID
    }

    enum CodingKeys: String, CodingKey {
        case succeeded, result
        case companyID = "companyId"
    }
}
