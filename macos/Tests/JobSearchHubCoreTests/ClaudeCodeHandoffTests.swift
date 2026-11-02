import Foundation
@testable import JobSearchHubCore
import Testing

// Where a hand-off to Claude Code goes: the stub in QA mode, the real
// `claude` outside it. Made-up jobs and paths only.

private let home = URL(filePath: "/Users/qa")

private extension JSONDecoder {
    static var iso8601: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}

private func makeTemporaryFolder() -> URL {
    FileManager.default.temporaryDirectory.appending(path: "ClaudeCodeHandoffTests-\(UUID().uuidString)", directoryHint: .isDirectory)
}

@Test func qaModeIsAQABuildOrALaunchWithTheFlag() {
    #expect(ClaudeCodeHandoff.isQAMode(arguments: ["JobSearchHub"], isQABuild: true))
    #expect(ClaudeCodeHandoff.isQAMode(arguments: ["JobSearchHub", "--qa-mode"], isQABuild: false))
    #expect(!ClaudeCodeHandoff.isQAMode(arguments: ["JobSearchHub"], isQABuild: false))
}

@Test func inQAModeAHandoffGoesToTheStubAndNeverLooksForClaude() {
    let target = ClaudeCodeHandoff.resolve(isQAMode: true, environment: [:], home: home) {
        Issue.record("QA mode looked for the real claude")
        return "/Users/qa/.local/bin/claude"
    }

    #expect(target == .qaStub(ClaudeCodeStub(directory: home.appending(path: "Library/Application Support/JobSearchHub/QA", directoryHint: .isDirectory))))
}

@Test func outsideQAModeAHandoffGoesToTheRealClaudeAsToday() {
    var looked = false
    let target = ClaudeCodeHandoff.resolve(isQAMode: false, environment: [ClaudeCodeStub.fixturesVariable: "/tmp/qa"], home: home) {
        looked = true
        return "/opt/homebrew/bin/claude"
    }

    #expect(looked)
    #expect(target == .claude("/opt/homebrew/bin/claude"))
    #expect(ClaudeCodeHandoff.resolve(isQAMode: false, environment: [:], home: home) { nil } == nil)
}

@Test func theStubRecordsInTheQAFixtureFolderWhenQANamesOne() {
    let target = ClaudeCodeHandoff.resolve(isQAMode: true, environment: [ClaudeCodeStub.fixturesVariable: "/private/tmp/qa-fixture"], home: home) { nil }

    #expect(target == .qaStub(ClaudeCodeStub(directory: URL(filePath: "/private/tmp/qa-fixture", directoryHint: .isDirectory))))
    // A blank variable leaves the default folder.
    let blank = ClaudeCodeHandoff.resolve(isQAMode: true, environment: [ClaudeCodeStub.fixturesVariable: " "], home: home) { nil }
    #expect(blank == .qaStub(ClaudeCodeStub(directory: home.appending(path: "Library/Application Support/JobSearchHub/QA", directoryHint: .isDirectory))))
}

@Test func theStubRecordsInTheQARootTheDriverReadsBeforeTheFixtureFolder() {
    let environment = [
        ClaudeCodeStub.rootVariable: "/Users/admin/.symphony-qa/runs/run.a/root-1",
        ClaudeCodeStub.fixturesVariable: "/Users/admin/.symphony-qa/runs/run.a/api-fixtures-1",
    ]
    let target = ClaudeCodeHandoff.resolve(isQAMode: true, environment: environment, home: home) { nil }

    let root = URL(filePath: "/Users/admin/.symphony-qa/runs/run.a/root-1", directoryHint: .isDirectory)
    #expect(target == .qaStub(ClaudeCodeStub(directory: root)))
    #expect(ClaudeCodeStub(directory: root).requestLogURL.path == "/Users/admin/.symphony-qa/runs/run.a/root-1/api-requests.jsonl")
    // A QA root alone is enough, and a blank one falls through to the fixture folder.
    #expect(ClaudeCodeHandoff.resolve(isQAMode: true, environment: [ClaudeCodeStub.rootVariable: "/private/tmp/qa-root"], home: home) { nil }
        == .qaStub(ClaudeCodeStub(directory: URL(filePath: "/private/tmp/qa-root", directoryHint: .isDirectory))))
    let blank = ClaudeCodeHandoff.resolve(isQAMode: true, environment: [ClaudeCodeStub.rootVariable: " ", ClaudeCodeStub.fixturesVariable: "/private/tmp/qa-fixture"], home: home) { nil }
    #expect(blank == .qaStub(ClaudeCodeStub(directory: URL(filePath: "/private/tmp/qa-fixture", directoryHint: .isDirectory))))
}

@Test func theStubAddsOneLinePerHandoffAndReadsThemBack() throws {
    let folder = makeTemporaryFolder()
    defer { try? FileManager.default.removeItem(at: folder) }
    let stub = ClaudeCodeStub(directory: folder)
    let jobID = UUID()
    let fix = ClaudeCodeHandoffRecord(
        kind: ClaudeCodeHandoffRecord.fixJob, jobID: jobID, title: "Platform Engineer", note: "the title has the city in it",
        workingDirectory: "/", handedAt: Date(timeIntervalSince1970: 1_800_000_000)
    )
    let session = ClaudeCodeHandoffRecord(
        kind: ClaudeCodeHandoffRecord.session, jobID: jobID, title: "Platform Engineer · Example Co",
        file: "/Users/qa/Library/Application Support/JobSearchHub/Sessions/a.context.md",
        workingDirectory: "/Users/qa/Library/Application Support/JobSearchHub/Sessions", handedAt: Date(timeIntervalSince1970: 1_800_000_060)
    )

    #expect(try stub.readRecords().isEmpty)
    try stub.record(fix)
    try stub.record(session)

    #expect(try stub.readRecords() == [fix, session])
    let text = try String(contentsOf: stub.recordURL, encoding: .utf8)
    #expect(text.split(separator: "\n").count == 2)
    #expect(text.contains(#""note":"the title has the city in it""#))
}

@Test func theStubLogsEachHandoffAsARequestTheQADriverReadsBack() throws {
    let folder = makeTemporaryFolder()
    defer { try? FileManager.default.removeItem(at: folder) }
    let stub = ClaudeCodeStub(directory: folder)
    // The request log may already hold the app's other requests.
    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    try Data(#"{"method":"GET","path":"/v1/health"}"#.utf8 + [0x0A]).write(to: stub.requestLogURL)
    let fix = ClaudeCodeHandoffRecord(
        kind: ClaudeCodeHandoffRecord.fixJob, jobID: UUID(), title: "Platform Engineer", note: "the pay ranges are missing",
        workingDirectory: "/", handedAt: Date(timeIntervalSince1970: 1_800_000_000)
    )

    try stub.record(fix)

    #expect(stub.requestLogURL.lastPathComponent == "api-requests.jsonl")
    #expect(stub.requestLogURL.deletingLastPathComponent() == stub.recordURL.deletingLastPathComponent())
    #expect(try stub.readRequests() == [ClaudeCodeStubRequest(fix)])
    let lines = try String(contentsOf: stub.requestLogURL, encoding: .utf8).split(separator: "\n")
    #expect(lines.count == 2)
    #expect(lines[1].hasPrefix(#"{"body":{"#))
    #expect(lines[1].contains(#""method":"POST""#) && lines[1].contains(#""path":"/claude-code/handoff""#))
    #expect(lines[1].contains(#""note":"the pay ranges are missing""#))
}

@Test func theCommittedFixtureHoldsOneFixHandoff() throws {
    let fixture = URL(filePath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        .appending(path: "Fixtures/claude-code-handoff/\(ClaudeCodeStub.recordFileName)")

    let records = try ClaudeCodeStub.parseRecords(Data(contentsOf: fixture))

    #expect(records.count == 1)
    #expect(records.first?.kind == ClaudeCodeHandoffRecord.fixJob)
    #expect(records.first?.jobID != nil && records.first?.note?.isEmpty == false)
    let requestLog = fixture.deletingLastPathComponent().appending(path: ClaudeCodeStub.requestLogFileName)
    let request = try JSONDecoder.iso8601.decode(ClaudeCodeStubRequest.self, from: Data(contentsOf: requestLog))
    let fix = try #require(records.first)
    #expect(request == ClaudeCodeStubRequest(fix))
}

@Test func aStubSessionSaysItWentToTheStubAndWaits() {
    #expect(ClaudeCodeStub.confirmation == "Handed to Claude Code (QA stub)")
    #expect(ClaudeCodeStub.sessionShellCommand.hasPrefix("printf '%s\\n' 'Handed to Claude Code (QA stub)';"))
    #expect(!ClaudeCodeStub.sessionShellCommand.contains("claude'"))
}
