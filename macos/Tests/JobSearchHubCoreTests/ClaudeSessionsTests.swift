import Foundation
@testable import JobSearchHubCore
import Testing

private func makeSession(name: String = "Frontend Engineer · Acme") -> ClaudeSession {
    ClaudeSession(
        id: UUID(uuidString: "AAAAAAAA-0000-0000-0000-000000000001")!, companyID: nil, jobID: UUID(),
        claudeSessionID: UUID(uuidString: "BBBBBBBB-0000-0000-0000-000000000002")!, name: name,
        createdAt: Date(timeIntervalSince1970: 100), lastStartedAt: Date(timeIntervalSince1970: 300), lastStoppedAt: nil
    )
}

@Test func aSessionDecodesFromTheHub() throws {
    let json = #"{"id":"aaaaaaaa-0000-0000-0000-000000000001","job_id":"cccccccc-0000-0000-0000-000000000003","claude_session_id":"bbbbbbbb-0000-0000-0000-000000000002","name":"Acme","created_at":"2026-09-28T15:00:00Z","last_started_at":"2026-09-28T15:05:00.123Z"}"#

    let session = try HubJSON.makeDecoder().decode(ClaudeSession.self, from: Data(json.utf8))

    #expect(session.jobID != nil && session.companyID == nil && session.lastStoppedAt == nil)
    #expect(session.lastActiveAt == session.lastStartedAt)
}

@Test func aNewSessionStartsUnderItsIDAndAnEarlierOneIsResumed() {
    let session = makeSession()

    let start = ClaudeLaunch.getShellCommand(claude: "/Users/me/.local/bin/claude", session: session, hasConversation: false)
    let resume = ClaudeLaunch.getShellCommand(claude: "/Users/me/.local/bin/claude", session: session, hasConversation: true)

    #expect(start == "exec '/Users/me/.local/bin/claude' '--settings' 'hub-session-hooks.json' '--session-id' 'bbbbbbbb-0000-0000-0000-000000000002' '--name' 'Frontend Engineer · Acme' '--append-system-prompt-file' 'aaaaaaaa-0000-0000-0000-000000000001.context.md'")
    #expect(resume == "exec '/Users/me/.local/bin/claude' '--settings' 'hub-session-hooks.json' '--resume' 'bbbbbbbb-0000-0000-0000-000000000002' '--append-system-prompt-file' 'aaaaaaaa-0000-0000-0000-000000000001.context.md'")
}

@Test func aNameWithQuotesStaysOneArgument() {
    let command = ClaudeLaunch.getShellCommand(claude: "claude", session: makeSession(name: "Tony's \"dream\" job; rm -rf ~"), hasConversation: false)

    #expect(command.contains(#"'--name' 'Tony'\''s "dream" job; rm -rf ~'"#))
}

@Test func theTranscriptLivesWhereClaudeNamesTheWorkingDirectory() {
    let session = makeSession()
    let support = URL(filePath: "/Users/me/Library/Application Support", directoryHint: .isDirectory)
    let folder = ClaudeLaunch.getWorkingDirectory(applicationSupport: support)

    #expect(folder.path == "/Users/me/Library/Application Support/JobSearchHub/Sessions")
    let transcript = ClaudeLaunch.getTranscriptURL(for: session, workingDirectory: folder, home: URL(filePath: "/Users/me", directoryHint: .isDirectory))
    #expect(transcript.path == "/Users/me/.claude/projects/-Users-me-Library-Application-Support-JobSearchHub-Sessions/bbbbbbbb-0000-0000-0000-000000000002.jsonl")
}

@Test func runningSessionsListFirstThenTheMostRecentlyActive() {
    func session(_ name: String, activeAt: TimeInterval) -> ClaudeSession {
        ClaudeSession(id: UUID(), companyID: UUID(), jobID: nil, claudeSessionID: UUID(), name: name,
                      createdAt: Date(timeIntervalSince1970: activeAt), lastStartedAt: nil, lastStoppedAt: nil)
    }
    let old = session("old", activeAt: 100)
    let recent = session("recent", activeAt: 300)
    let runningOld = session("running old", activeAt: 50)

    let sorted = ClaudeSessionList.sort([old, recent, runningOld], running: [runningOld.id])

    #expect(sorted.map(\.name) == ["running old", "recent", "old"])
    #expect(runningOld.subject == .company(runningOld.companyID!))
}

@Test func theHooksWriteEachEventsActivityToTheSessionsStateFile() throws {
    let settings = try #require(try JSONSerialization.jsonObject(with: Data(ClaudeHooks.getSettingsJSON().utf8)) as? [String: Any])
    let hooks = try #require(settings["hooks"] as? [String: [[String: Any]]])

    #expect(Set(hooks.keys) == ["SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Notification", "Stop"])
    let stop = try #require((hooks["Stop"]?.first?["hooks"] as? [[String: String]])?.first)
    #expect(stop["type"] == "command")
    #expect(stop["command"] == #"[ -n "$JOB_SEARCH_HUB_SESSION_STATE_FILE" ] && printf '%s' idle > "$JOB_SEARCH_HUB_SESSION_STATE_FILE" || true"#)
    #expect(ClaudeHooks.parseActivity("blocked\n") == .blocked)
    #expect(ClaudeHooks.parseActivity("?") == nil)
}

@Test func aFirstMessageComesLastAfterTheOptions() {
    let command = ClaudeLaunch.getShellCommand(claude: "claude", session: makeSession(), hasConversation: true, firstMessage: "-draft this")

    #expect(command.hasSuffix("'--' '-draft this'"))
    #expect(ClaudeLaunch.getPastedMessage("line one\nline two") == "\u{1b}[200~line one\nline two\u{1b}[201~\r")
}

@Test func aSessionInheritsNoClaudeCodeVariables() {
    let inherited = [
        "HOME": "/Users/me", "LANG": "pt_BR.UTF-8", "SSH_AUTH_SOCK": "/tmp/agent", "PATH": "/odd/path",
        "CLAUDE_CODE_CHILD_SESSION": "1", "CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "cli", "SOME_TOKEN": "secret",
    ]

    let environment = SessionEnvironment.make(from: inherited, stateFile: "/tmp/s.state")

    #expect(environment["HOME"] == "/Users/me" && environment["LANG"] == "pt_BR.UTF-8" && environment["SSH_AUTH_SOCK"] == "/tmp/agent")
    #expect(environment["CLAUDE_CODE_CHILD_SESSION"] == nil && environment["CLAUDECODE"] == nil && environment["CLAUDE_CODE_ENTRYPOINT"] == nil)
    #expect(environment["SOME_TOKEN"] == nil)
    #expect(environment["PATH"] == SessionEnvironment.basePath)
    #expect(environment[ClaudeHooks.stateFileVariable] == "/tmp/s.state" && environment["TERM"] == "xterm-256color")
}

@Test func aProfileInterviewSessionIsItsOwnSubject() throws {
    let json = #"{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","about_profile":true,"claude_session_id":"0aa55565-58d2-4247-ba01-cba65060a316","name":"Enhance profile","created_at":"2026-09-29T12:00:00Z"}"#
    let session = try HubJSON.makeDecoder().decode(ClaudeSession.self, from: Data(json.utf8))
    #expect(session.subject == .profile)
    #expect(ClaudeSessionSubject.profile.queryItems == [URLQueryItem(name: "about_profile", value: "true")])
    let request = try #require(try JSONSerialization.jsonObject(with: HubJSON.makeEncoder().encode(CreateClaudeSessionRequest(.profile))) as? [String: Any])
    #expect(request["about_profile"] as? Bool == true)
}

@Test func aReadableFolderIsAddedOnStartAndResumeAndQuoted() {
    let folder = "/Users/me/Job Search/Tony's files"
    for hasConversation in [false, true] {
        let command = ClaudeLaunch.getShellCommand(claude: "claude", session: makeSession(), hasConversation: hasConversation, readableFolder: folder, firstMessage: "hi")
        #expect(command.contains(#"'--add-dir' '/Users/me/Job Search/Tony'\''s files'"#))
        #expect(command.hasSuffix("'--' 'hi'"))
    }
    #expect(!ClaudeLaunch.getShellCommand(claude: "claude", session: makeSession(), hasConversation: false, readableFolder: "").contains("--add-dir"))
    #expect(ClaudeLaunch.getDefaultReadableFolder(home: URL(filePath: "/Users/me")) == "/Users/me/Interview")
}
