import Foundation

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

public enum RunsSummary {
    /// A kind's title as the hub's catalog of run kinds names it, for the
    /// places that show a kind without reading the catalog.
    public static func getKindTitle(_ kind: String) -> String {
        switch kind {
        case "job_facts": "Job facts"
        case "job_brief": "Job briefs"
        case "job_brief_full": "Full briefs"
        case "job_cv": "Tailored CVs"
        case "job_fix": "Job fixes"
        case "hiring_thread": "Hacker News hiring thread"
        case "recruiter_screen": "Recruiter screens"
        case "market_gaps": "Market gaps"
        case "interview_prep": "Interview packs"
        case "mail_triage": "Mail sorting"
        case "linkedin_conversation": "LinkedIn conversations"
        case "company_triage": "Company research"
        case "job_finder": "Find jobs"
        case "profile_seed": "Knowledge base build"
        default: kind.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }
}
