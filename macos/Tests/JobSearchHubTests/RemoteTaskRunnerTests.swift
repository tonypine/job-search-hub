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
