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

    #expect(start == "exec '/Users/me/.local/bin/claude' '--session-id' 'bbbbbbbb-0000-0000-0000-000000000002' '--name' 'Frontend Engineer · Acme' '--append-system-prompt-file' 'aaaaaaaa-0000-0000-0000-000000000001.context.md'")
    #expect(resume == "exec '/Users/me/.local/bin/claude' '--resume' 'bbbbbbbb-0000-0000-0000-000000000002' '--append-system-prompt-file' 'aaaaaaaa-0000-0000-0000-000000000001.context.md'")
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
