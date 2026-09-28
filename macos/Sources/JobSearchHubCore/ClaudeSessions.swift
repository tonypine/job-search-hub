import Foundation

/// A Claude session about one company or one job, as the hub records it.
/// Whether it is running is not part of it: the app's own processes say so.
public struct ClaudeSession: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var companyID: UUID?
    public var jobID: UUID?
    public var claudeSessionID: UUID
    public var name: String
    public var createdAt: Date
    public var lastStartedAt: Date?
    public var lastStoppedAt: Date?

    /// When the session was last started, stopped or created.
    public var lastActiveAt: Date {
        [createdAt, lastStartedAt, lastStoppedAt].compactMap { $0 }.max() ?? createdAt
    }

    enum CodingKeys: String, CodingKey {
        case id, name, createdAt, lastStartedAt, lastStoppedAt
        case companyID = "companyId"
        case jobID = "jobId"
        case claudeSessionID = "claudeSessionId"
    }
}

public struct ClaudeSessionsResponse: Decodable, Sendable {
    public var sessions: [ClaudeSession]
}

/// What a session starts knowing, rendered by the hub when it is asked for.
public struct ClaudeSessionContext: Decodable, Sendable {
    public var context: String
    public var promptVersion: Int
}

/// What a session is about.
public enum ClaudeSessionSubject: Equatable, Hashable, Sendable {
    case company(UUID)
    case job(UUID)

    /// The hub's filter for this subject's sessions.
    public var queryItems: [URLQueryItem] {
        switch self {
        case let .company(id): [URLQueryItem(name: "company_id", value: id.uuidString)]
        case let .job(id): [URLQueryItem(name: "job_id", value: id.uuidString)]
        }
    }
}

public struct CreateClaudeSessionRequest: Encodable, Sendable {
    public var companyID: UUID?
    public var jobID: UUID?

    public init(_ subject: ClaudeSessionSubject) {
        switch subject {
        case let .company(id): companyID = id
        case let .job(id): jobID = id
        }
    }

    enum CodingKeys: String, CodingKey {
        case companyID = "companyId"
        case jobID = "jobId"
    }
}

/// How a session runs on this Mac: under `claude`, in one folder all the
/// app's sessions share, with its context in a file of its own there.
public enum ClaudeLaunch {
    /// Where `claude` installs itself, in the order its installers prefer.
    public static func findClaudeExecutable(home: URL = FileManager.default.homeDirectoryForCurrentUser) -> String? {
        let candidates = [
            home.appending(path: ".local/bin/claude").path, home.appending(path: ".claude/local/claude").path,
            "/opt/homebrew/bin/claude", "/usr/local/bin/claude",
        ]
        return candidates.first { FileManager.default.isExecutableFile(atPath: $0) }
    }

    /// The folder the app's sessions run in. Claude keeps conversations per
    /// folder and asks once whether to trust a folder, so one shared folder
    /// means one question, and every session resumes where it started.
    public static func getWorkingDirectory(applicationSupport: URL) -> URL {
        applicationSupport.appending(path: "JobSearchHub/Sessions", directoryHint: .isDirectory)
    }

    /// The session's context file, in the shared folder.
    public static func getContextFileName(for session: ClaudeSession) -> String {
        "\(session.id.uuidString.lowercased()).context.md"
    }

    /// Where Claude writes the session's conversation: under its projects
    /// folder, in a folder named after the working directory with every
    /// character but letters and digits turned into "-".
    public static func getTranscriptURL(for session: ClaudeSession, workingDirectory: URL, home: URL) -> URL {
        let folderName = String(workingDirectory.path.map { $0.isASCII && ($0.isLetter || $0.isNumber) ? $0 : "-" })
        return home.appending(path: ".claude/projects/\(folderName)/\(session.claudeSessionID.uuidString.lowercased()).jsonl")
    }

    /// The command a login shell runs: a session with a conversation is
    /// resumed, and one without starts under its own ID. Either way it reads
    /// its context fresh.
    public static func getShellCommand(claude: String, session: ClaudeSession, hasConversation: Bool) -> String {
        var arguments = [claude]
        if hasConversation {
            arguments += ["--resume", session.claudeSessionID.uuidString.lowercased()]
        } else {
            arguments += ["--session-id", session.claudeSessionID.uuidString.lowercased(), "--name", session.name]
        }
        arguments += ["--append-system-prompt-file", getContextFileName(for: session)]
        return "exec " + arguments.map(quoteForShell).joined(separator: " ")
    }

    /// Single-quotes an argument for a POSIX shell.
    static func quoteForShell(_ argument: String) -> String {
        "'" + argument.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}

public enum ClaudeSessionList {
    /// Running sessions first, then the rest; each group most recently active first.
    public static func sort(_ sessions: [ClaudeSession], running: Set<UUID>) -> [ClaudeSession] {
        sessions.sorted { left, right in
            let leftRuns = running.contains(left.id)
            let rightRuns = running.contains(right.id)
            if leftRuns != rightRuns {
                return leftRuns
            }
            return left.lastActiveAt > right.lastActiveAt
        }
    }
}

extension ClaudeSession {
    /// What the session is about.
    public var subject: ClaudeSessionSubject? {
        if let jobID { return .job(jobID) }
        if let companyID { return .company(companyID) }
        return nil
    }
}
