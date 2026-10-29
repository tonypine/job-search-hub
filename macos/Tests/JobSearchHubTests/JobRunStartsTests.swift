import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// Runs started from a job's Run popover, against a stub of the hub: a row
// says "Running" from its start until its run lands. Made-up jobs only.

private let jobID = UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000001")!
private let firstRunID = UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000002")!
private let secondRunID = UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000003")!
private let taskID = UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000004")!
private let kindsPath = "/v1/jobs/\(jobID.uuidString)/runnable-kinds"
private let briefKind = "job_brief_full"

/// The full brief's kind for the job, with its last run when it has one.
private func briefJSON(lastRun: UUID?, outcome: String) -> String {
    let run = lastRun.map { id in
        #"""
        ,"last_run":{"id":"\#(id.uuidString)","record":"model_call","kind":"job_brief_full",
         "subject":{"type":"job","id":"\#(jobID.uuidString)","title":"Staff engineer"},"model":"claude-sonnet","provider":"Claude",
         "started_at":"2020-10-29T14:14:00Z","outcome":"\#(outcome)","prompt_tokens":0,"completion_tokens":0,"started_by_owner":true}
        """#
    } ?? ""
    return #"""
    {"id":"job_brief_full","title":"Full briefs","description":"Claude writes a full brief.","group":"jobs","subject_type":"job",
     "engine":"claude_one_shot","automatic":true,"paused":true,"can_run":true,"needs_note":false\#(run)}
    """#
}

/// The job's kinds as the hub lists them: the full brief, its last run
/// `lastRun` with `outcome`.
private func kinds(lastRun: UUID?, outcome: String = "succeeded") -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"{"kinds":[\#(briefJSON(lastRun: lastRun, outcome: outcome))]}"#)
}

/// The full brief as the popover read it, with an earlier run that landed
/// long before the starts here.
private func makeBrief() throws -> RunnableKind {
    try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(briefJSON(lastRun: UUID(), outcome: "succeeded").utf8))
}

/// What a start answers: the run's id, and the Mac's task for an agent.
private func started(_ runID: UUID, taskID: UUID? = nil) -> StubHub.Answer {
    let task = taskID.map { #","task_id":"\#($0.uuidString)""# } ?? ""
    return StubHub.Answer(status: 202, body: #"""
    {"queued":true,"run":{"id":"\#(runID.uuidString)","kind":"job_brief_full","subject_type":"job","subject_id":"\#(jobID.uuidString)",
     "provider":"Claude","model":"claude-sonnet","chosen_model":false,"started_by_owner":true\#(task)}}
    """#)
}

/// A hub that starts the first run, whose record says it is running.
private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/runs": started(firstRunID),
        kindsPath: kinds(lastRun: firstRunID, outcome: "running"),
    ])
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

/// The tasks a run's start handed the Mac.
@MainActor
private final class TaskLog {
    var taskIDs: [UUID] = []
}

@MainActor @Test func aStartedRunSaysRunningUntilItLands() async throws {
    let (client, recording) = makeHub()
    let starts = JobRunStarts(pollInterval: .milliseconds(10))

    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)

    #expect(starts.getRunning(briefKind, of: jobID)?.runID == firstRunID)
    #expect(starts.isBusy(briefKind, of: jobID))
    #expect(starts.revision == 0)

    recording.setAnswer(kinds(lastRun: firstRunID), for: kindsPath)
    await follow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 1)
}

@MainActor @Test func aStartTheHubRefusesLeavesNoRunningRow() async throws {
    let (client, recording) = makeHub()
    recording.setAnswer(StubHub.Answer(status: 409, body: #"{"error":"needs the tailored CV first"}"#), for: "/v1/runs")
    let starts = JobRunStarts(pollInterval: .milliseconds(10))

    let follow = try? await starts.start(try makeBrief(), for: jobID, with: client)

    #expect(follow == nil)
    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(!starts.isBusy(briefKind, of: jobID))
    #expect(starts.revision == 0)
}

@MainActor @Test func anEarlierRunLandingLeavesALaterStartRunning() async throws {
    let (client, recording) = makeHub()
    let starts = JobRunStarts(pollInterval: .milliseconds(10))
    let firstFollow = try await starts.start(try makeBrief(), for: jobID, with: client)
    recording.setAnswer(started(secondRunID), for: "/v1/runs")
    let secondFollow = try await starts.start(try makeBrief(), for: jobID, with: client)

    recording.setAnswer(kinds(lastRun: firstRunID), for: kindsPath)
    await firstFollow.value

    #expect(starts.getRunning(briefKind, of: jobID)?.runID == secondRunID)
    #expect(starts.revision == 1)

    recording.setAnswer(kinds(lastRun: secondRunID), for: kindsPath)
    await secondFollow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 2)
}

@MainActor @Test func aRunThatLandedBeforeTheStartDoesntEndItsRow() async throws {
    let (client, recording) = makeHub()
    // Another run of the kind, finished since the popover read the kinds.
    recording.setAnswer(kinds(lastRun: secondRunID), for: kindsPath)
    let starts = JobRunStarts(pollInterval: .milliseconds(10), followLimit: { _ in 1 })

    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)
    await follow.value

    // The start and more than one read of the runs: the first read, which
    // found the other run, didn't end the row; the follow limit did.
    #expect(recording.requestCount > 2)
    #expect(starts.getRunning(briefKind, of: jobID) == nil)
}

@MainActor @Test func aRowStopsSayingRunningOnceTheRunCouldNoLongerBeGoing() async throws {
    let (client, _) = makeHub()
    let starts = JobRunStarts(pollInterval: .milliseconds(10), followLimit: { _ in 0.05 })

    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)
    await follow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 1)
}

@MainActor @Test func anAgentsTaskRunsOnThisMacAtOnceAndItsRowEndsWithIt() async throws {
    let (client, recording) = makeHub()
    recording.setAnswer(started(firstRunID, taskID: taskID), for: "/v1/runs")
    let log = TaskLog()
    let starts = JobRunStarts(pollInterval: .milliseconds(10), runTask: { taskID, _ in
        log.taskIDs.append(taskID)
        return true
    })

    let follow = try await starts.start(try makeBrief(), for: jobID, note: "the title has the city in it", with: client)
    await follow.value

    #expect(log.taskIDs == [taskID])
    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 1)
    // Only the start: the run had landed once its task ended here.
    #expect(recording.requestCount == 1)
}

@MainActor @Test func anAgentsTaskAnotherRunnerTookIsFollowedOnTheHub() async throws {
    let (client, recording) = makeHub()
    recording.setAnswer(started(firstRunID, taskID: taskID), for: "/v1/runs")
    recording.setAnswer(kinds(lastRun: firstRunID), for: kindsPath)
    let starts = JobRunStarts(pollInterval: .milliseconds(10), runTask: { _, _ in false })

    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)
    await follow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 1)
    #expect(recording.lastRequest?.url?.path() == kindsPath)
}
