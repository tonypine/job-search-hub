import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// The Mac's task runner against a stub of the hub: a task it claims tells
// the hub how it ended, even once the view that asked has gone. Made-up
// jobs only.

private let taskID = UUID(uuidString: "CCCCCCCC-0000-4000-8000-000000000001")!
private let claimPath = "/v1/tasks/\(taskID.uuidString)/claim"
private let finishPath = "/v1/tasks/\(taskID.uuidString)/finish"

/// A fix that names no job, so it ends at once, failed, without an agent.
private let fixJSON = #"{"id":"\#(taskID.uuidString)","kind":"fix_job","input":"the title has the city in it","status":"running"}"#

/// A hub with the fix queued, which answers its claim and its finish.
private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/tasks": StubHub.Answer(status: 200, body: #"{"tasks":[\#(fixJSON)]}"#),
        claimPath: StubHub.Answer(status: 200, body: fixJSON),
        finishPath: StubHub.Answer(status: 200, body: fixJSON),
    ])
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

@MainActor @Test func aStartedTaskRunsHereAtOnceAndReportsHowItEnded() async throws {
    let (client, recording) = makeHub()
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder())

    // Asked from a view whose task an update has already cancelled.
    let running = Task { await runner.runStarted(taskID, with: client) }
    running.cancel()

    #expect(await running.value)
    #expect(recording.lastRequest?.url?.path() == finishPath)
    let finish = try #require(recording.lastBody.flatMap { String(data: $0, encoding: .utf8) })
    #expect(finish.contains(#""succeeded":false"#) && finish.contains("named no job"))
}

@MainActor @Test func aStartedTaskThisMacAlreadyHoldsRunsOnceAndEndsWithThatRun() async throws {
    let (client, recording) = makeHub()
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder())

    let first = Task { await runner.runStarted(taskID, with: client) }
    let second = Task { await runner.runStarted(taskID, with: client) }

    #expect(await first.value)
    #expect(await second.value)
    // One claim and one finish: the second waited for the first's run.
    #expect(recording.requestCount == 2)
    #expect(recording.lastRequest?.url?.path() == finishPath)
}

@MainActor @Test func aStartedTaskAnotherRunnerClaimedIsLeftToIt() async throws {
    let (client, recording) = makeHub()
    recording.setAnswer(StubHub.Answer(status: 409, body: #"{"error":"the task isn't queued"}"#), for: claimPath)
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder())

    #expect(await runner.runStarted(taskID, with: client) == false)
    #expect(recording.requestCount == 1)
    #expect(recording.lastRequest?.url?.path() == claimPath)
}

@MainActor @Test func aStartedTaskWaitsForTheNewVersionWhileAnInstallWaits() async throws {
    let (client, recording) = makeHub()
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder())
    runner.isPaused = true

    #expect(await runner.runStarted(taskID, with: client) == false)
    #expect(recording.requestCount == 0)
}

@MainActor @Test func aCheckFinishesTheTasksItClaimedOnceItsViewHasGone() async throws {
    let (client, recording) = makeHub()
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder())

    let checking = Task { await runner.check(with: client) }
    checking.cancel()
    await checking.value

    #expect(recording.lastRequest?.url?.path() == finishPath)
}

/// A fix that names a job, the one the owner wrote a note on.
private let jobID = UUID(uuidString: "CCCCCCCC-0000-4000-8000-000000000002")!
private let jobFixJSON = #"{"id":"\#(taskID.uuidString)","kind":"fix_job","jobId":"\#(jobID.uuidString)","input":"the pay ranges are missing","status":"running"}"#

@MainActor @Test func inQAModeAFixGoesToTheStubWhichRecordsItAndAnswersAsALaunch() async throws {
    let (session, recording) = StubHub.makeSession(answers: [
        claimPath: StubHub.Answer(status: 200, body: jobFixJSON),
        finishPath: StubHub.Answer(status: 200, body: jobFixJSON),
    ])
    let client = HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session)
    let folder = FileManager.default.temporaryDirectory.appending(path: "RemoteTaskRunnerTests-\(UUID().uuidString)", directoryHint: .isDirectory)
    defer { try? FileManager.default.removeItem(at: folder) }
    let stub = ClaudeCodeStub(directory: folder)
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder()) { .qaStub(stub) }

    #expect(await runner.runStarted(taskID, with: client))

    #expect(recording.lastRequest?.url?.path() == finishPath)
    let finish = try #require(recording.lastBody.flatMap { String(data: $0, encoding: .utf8) })
    #expect(finish.contains(#""succeeded":true"#) && finish.contains("Handed to Claude Code (QA stub)"))
    let records = try stub.readRecords()
    #expect(records.count == 1)
    #expect(records.first?.kind == ClaudeCodeHandoffRecord.fixJob)
    #expect(records.first?.jobID == jobID && records.first?.note == "the pay ranges are missing")
    #expect(runner.stubHandedNotes == [jobID: "the pay ranges are missing"])
    // The QA driver reads the hand-off back from the request log.
    let requests = try stub.readRequests()
    #expect(requests.map(\.path) == ["/claude-code/handoff"])
    #expect(requests.first?.body.jobID == jobID && requests.first?.body.note == "the pay ranges are missing")
}

@MainActor @Test func outsideQAModeAFixWithoutClaudeFailsAsToday() async throws {
    let (session, recording) = StubHub.makeSession(answers: [
        claimPath: StubHub.Answer(status: 200, body: jobFixJSON),
        finishPath: StubHub.Answer(status: 200, body: jobFixJSON),
    ])
    let client = HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session)
    let runner = RemoteTaskRunner(jobFinder: CompanyJobFinder()) { nil }

    #expect(await runner.runStarted(taskID, with: client))

    let finish = try #require(recording.lastBody.flatMap { String(data: $0, encoding: .utf8) })
    #expect(finish.contains(#""succeeded":false"#) && finish.contains("Claude Code is missing"))
    #expect(runner.stubHandedNotes.isEmpty)
}
