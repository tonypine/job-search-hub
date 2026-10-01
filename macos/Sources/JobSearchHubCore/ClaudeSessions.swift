import Foundation

/// A Claude session about one company or one job, as the hub records it.
/// Whether it is running is not part of it: the app's own processes say so.
public struct ClaudeSession: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var companyID: UUID?
    public var jobID: UUID?
    /// The interview that deepens the owner's knowledge base; absent on the
    /// others.
    public var aboutProfile: Bool?
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
        case id, name, createdAt, lastStartedAt, lastStoppedAt, aboutProfile
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
    /// The interview that deepens the owner's knowledge base.
    case profile

    /// The hub's filter for this subject's sessions.
    public var queryItems: [URLQueryItem] {
        switch self {
        case let .company(id): [URLQueryItem(name: "company_id", value: id.uuidString)]
        case let .job(id): [URLQueryItem(name: "job_id", value: id.uuidString)]
        case .profile: [URLQueryItem(name: "about_profile", value: "true")]
        }
    }
}

public struct CreateClaudeSessionRequest: Encodable, Sendable {
    public var companyID: UUID?
    public var jobID: UUID?
    public var aboutProfile = false

    public init(_ subject: ClaudeSessionSubject) {
        switch subject {
        case let .company(id): companyID = id
        case let .job(id): jobID = id
        case .profile: aboutProfile = true
        }
    }

    enum CodingKeys: String, CodingKey {
        case aboutProfile
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
    /// its context fresh and loads the app's hooks on top of the owner's own
    /// settings.
    public static func getShellCommand(
        claude: String, session: ClaudeSession, hasConversation: Bool, readableFolder: String? = nil, firstMessage: String? = nil
    ) -> String {
        var arguments = [claude, "--settings", ClaudeHooks.settingsFileName]
        if hasConversation {
            arguments += ["--resume", session.claudeSessionID.uuidString.lowercased()]
        } else {
            arguments += ["--session-id", session.claudeSessionID.uuidString.lowercased(), "--name", session.name]
        }
        arguments += ["--append-system-prompt-file", getContextFileName(for: session)]
        if let readableFolder, !readableFolder.isEmpty {
            // Read without /add-dir, so an agent can attach the owner's files, such as the CV, to a form.
            arguments += ["--add-dir", readableFolder]
        }
        if let firstMessage {
            arguments += ["--", firstMessage]
        }
        return "exec " + arguments.map(quoteForShell).joined(separator: " ")
    }

    /// A message typed into a running session: pasted whole, so its line
    /// breaks don't submit it early, then sent with Enter.
    public static func getPastedMessage(_ message: String) -> String {
        "\u{1b}[200~" + message + "\u{1b}[201~\r"
    }

    /// The settings key of the folder sessions can read.
    public static let readableFolderKey = "sessionReadableFolder"

    /// The folder sessions can read when the owner hasn't chosen one:
    /// ~/Interview, where the CV is kept.
    public static func getDefaultReadableFolder(home: URL) -> String {
        home.appending(path: "Interview").path
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
        return aboutProfile == true ? .profile : nil
    }
}

/// What a running session is doing, as its hooks report it.
public enum SessionActivity: String, Sendable {
    case working
    /// Waiting for the owner: a permission prompt or a question.
    case blocked
    /// Finished its turn.
    case idle
}

/// The hooks the app adds to its own sessions, and only to them, through
/// `--settings`. Each writes the session's activity to the file named by
/// `stateFileVariable`, which the app sets for each session.
public enum ClaudeHooks {
    public static let settingsFileName = "hub-session-hooks.json"
    public static let stateFileVariable = "JOB_SEARCH_HUB_SESSION_STATE_FILE"

    /// Which hook events mean which activity.
    static let activityByEvent: [(event: String, activity: SessionActivity)] = [
        ("SessionStart", .idle), ("UserPromptSubmit", .working), ("PreToolUse", .working),
        ("PostToolUse", .working), ("Notification", .blocked), ("Stop", .idle),
    ]

    public static func getSettingsJSON() -> String {
        var hooks: [String: Any] = [:]
        for (event, activity) in activityByEvent {
            let command = "[ -n \"$\(stateFileVariable)\" ] && printf '%s' \(activity.rawValue) > \"$\(stateFileVariable)\" || true"
            hooks[event] = [["hooks": [["type": "command", "command": command]]]]
        }
        let data = try! JSONSerialization.data(withJSONObject: ["hooks": hooks], options: [.prettyPrinted, .sortedKeys])
        return String(decoding: data, as: UTF8.self)
    }

    /// Reads a state file's text; anything unknown reads as nil.
    public static func parseActivity(_ text: String) -> SessionActivity? {
        SessionActivity(rawValue: text.trimmingCharacters(in: .whitespacesAndNewlines))
    }
}

/// The active version of one of the hub's prompts.
public struct AgentPrompt: Decodable, Equatable, Identifiable, Sendable {
    public var kind: String
    public var version: Int
    public var body: String
    /// Why this version exists, as its author wrote it.
    public var note: String?
    public var createdAt: Date?

    public var id: Int { version }
}

/// The environment a session starts in. Only what a login shell needs is
/// passed on, never the app's whole environment: an app launched from a
/// Claude Code session carries that session's variables, and a session that
/// inherits them takes itself for its child and stops saving its transcript.
public enum SessionEnvironment {
    static let passedVariables: Set<String> = [
        "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "SSH_AUTH_SOCK", "__CF_USER_TEXT_ENCODING",
    ]
    static let basePath = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"

    public static func make(from inherited: [String: String], stateFile: String) -> [String: String] {
        var environment = inherited.filter { passedVariables.contains($0.key) }
        environment["PATH"] = basePath
        environment["TERM"] = "xterm-256color"
        environment["COLORTERM"] = "truecolor"
        environment[ClaudeHooks.stateFileVariable] = stateFile
        return environment
    }
}
