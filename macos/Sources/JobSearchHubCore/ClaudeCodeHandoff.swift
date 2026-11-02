import Foundation

/// Where a hand-off to Claude Code goes: the real `claude` on this Mac, or,
/// in QA mode, a stub that records what it was asked and answers as a
/// successful launch. A QA machine has no Claude Code and no account to sign
/// it in with, so a QA pass proves a hand-off by the stub's record.
public enum ClaudeCodeTarget: Equatable, Sendable {
    /// The `claude` executable at this path.
    case claude(String)
    case qaStub(ClaudeCodeStub)
}

public enum ClaudeCodeHandoff {
    /// QA mode as the app has it: a QA build, which make-app.sh marks with
    /// HubQABuild, or a launch with `--qa-mode`.
    public static func isQAMode(arguments: [String], isQABuild: Bool) -> Bool {
        isQABuild || arguments.contains(HubConnection.qaModeArgument)
    }

    /// The stub in QA mode, without ever looking for `claude`; outside QA
    /// mode the `claude` `findClaude` finds, or nil when it finds none.
    public static func resolve(
        isQAMode: Bool, environment: [String: String], home: URL, findClaude: () -> String?
    ) -> ClaudeCodeTarget? {
        if isQAMode {
            return .qaStub(ClaudeCodeStub(directory: ClaudeCodeStub.findDirectory(environment: environment, home: home)))
        }
        return findClaude().map(ClaudeCodeTarget.claude)
    }

    /// This launch's.
    public static func resolveCurrent() -> ClaudeCodeTarget? {
        resolve(
            isQAMode: isQAMode(
                arguments: ProcessInfo.processInfo.arguments,
                isQABuild: Bundle.main.object(forInfoDictionaryKey: HubConnection.qaBuildInfoKey) as? Bool == true
            ),
            environment: ProcessInfo.processInfo.environment,
            home: FileManager.default.homeDirectoryForCurrentUser,
            findClaude: { ClaudeLaunch.findClaudeExecutable() }
        )
    }
}

/// The stand-in for Claude Code in QA mode. Each hand-off adds one JSON line
/// to `claude-code-handoffs.jsonl` in its folder: the QA fixture folder
/// Symphony's QA launch names in `SYMPHONY_BAR_QA_API_FIXTURES`, or
/// `~/Library/Application Support/JobSearchHub/QA/` when the variable is
/// unset. It also logs the hand-off as a request, `POST /claude-code/handoff`
/// with the record as its body, to `api-requests.jsonl` in the same folder,
/// the request log Symphony's QA driver reads back with `qa_api_requests`.
public struct ClaudeCodeStub: Equatable, Sendable {
    /// What the app says where it would say Claude Code took the work.
    public static let confirmation = "Handed to Claude Code (QA stub)"
    public static let recordFileName = "claude-code-handoffs.jsonl"
    /// The QA root's request log, and the request a hand-off is logged as.
    public static let requestLogFileName = "api-requests.jsonl"
    public static let requestMethod = "POST"
    public static let requestPath = "/claude-code/handoff"
    public static let fixturesVariable = "SYMPHONY_BAR_QA_API_FIXTURES"

    public let directory: URL

    public init(directory: URL) {
        self.directory = directory
    }

    public var recordURL: URL {
        directory.appending(path: Self.recordFileName)
    }

    public var requestLogURL: URL {
        directory.appending(path: Self.requestLogFileName)
    }

    static func findDirectory(environment: [String: String], home: URL) -> URL {
        if let path = environment[fixturesVariable]?.trimmingCharacters(in: .whitespacesAndNewlines), !path.isEmpty {
            return URL(filePath: path, directoryHint: .isDirectory)
        }
        return home.appending(path: "Library/Application Support/JobSearchHub/QA", directoryHint: .isDirectory)
    }

    /// Adds the hand-off to the record file and, as a request, to the request
    /// log, creating the folder and the files when they don't exist yet.
    public func record(_ handoff: ClaudeCodeHandoffRecord) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let encoder = Self.makeEncoder()
        try Self.append(encoder.encode(handoff), to: recordURL)
        try Self.append(encoder.encode(ClaudeCodeStubRequest(handoff)), to: requestLogURL)
    }

    private static func append(_ line: Data, to url: URL) throws {
        var line = line
        line.append(0x0A)
        if FileManager.default.fileExists(atPath: url.path) {
            let file = try FileHandle(forWritingTo: url)
            defer { try? file.close() }
            try file.seekToEnd()
            try file.write(contentsOf: line)
        } else {
            try line.write(to: url)
        }
    }

    /// The hand-offs logged as requests so far, oldest first. Lines another
    /// writer of the request log added are left out.
    public func readRequests() throws -> [ClaudeCodeStubRequest] {
        guard FileManager.default.fileExists(atPath: requestLogURL.path) else { return [] }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let data = try Data(contentsOf: requestLogURL)
        return String(decoding: data, as: UTF8.self).split(separator: "\n").compactMap {
            guard let request = try? decoder.decode(ClaudeCodeStubRequest.self, from: Data($0.utf8)),
                  request.method == Self.requestMethod, request.path == Self.requestPath else { return nil }
            return request
        }
    }

    /// The hand-offs recorded so far, oldest first; none when there is no
    /// record file.
    public func readRecords() throws -> [ClaudeCodeHandoffRecord] {
        guard FileManager.default.fileExists(atPath: recordURL.path) else { return [] }
        return try Self.parseRecords(Data(contentsOf: recordURL))
    }

    public static func parseRecords(_ data: Data) throws -> [ClaudeCodeHandoffRecord] {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try String(decoding: data, as: UTF8.self).split(separator: "\n").map {
            try decoder.decode(ClaudeCodeHandoffRecord.self, from: Data($0.utf8))
        }
    }

    private static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        return encoder
    }

    /// What a session's terminal runs in place of `claude`: it prints the
    /// confirmation and then waits, as an open session does, until stopped.
    public static let sessionShellCommand = "printf '%s\\n' '\(confirmation)'; while read -r line; do :; done"
}

/// One hand-off as the stub records it.
public struct ClaudeCodeHandoffRecord: Codable, Equatable, Sendable {
    /// A job's fix, `fix_job`, or a session, `session`.
    public var kind: String
    public var jobID: UUID?
    /// The job's title, or the session's name.
    public var title: String?
    public var companyID: UUID?
    /// The owner's note, or a session's first message.
    public var note: String?
    /// The file Claude Code would have been opened with, such as a session's
    /// context file.
    public var file: String?
    /// The folder Claude Code would have run in.
    public var workingDirectory: String
    public var handedAt: Date

    public static let fixJob = "fix_job"
    public static let session = "session"

    public init(
        kind: String, jobID: UUID? = nil, title: String? = nil, companyID: UUID? = nil, note: String? = nil,
        file: String? = nil, workingDirectory: String, handedAt: Date = .now
    ) {
        self.kind = kind
        self.jobID = jobID
        self.title = title
        self.companyID = companyID
        self.note = note
        self.file = file
        self.workingDirectory = workingDirectory
        self.handedAt = handedAt
    }
}

/// A hand-off as the request log holds it: `POST /claude-code/handoff`, the
/// record as its body.
public struct ClaudeCodeStubRequest: Codable, Equatable, Sendable {
    public var method: String
    public var path: String
    public var body: ClaudeCodeHandoffRecord

    public init(_ handoff: ClaudeCodeHandoffRecord) {
        method = ClaudeCodeStub.requestMethod
        path = ClaudeCodeStub.requestPath
        body = handoff
    }
}
