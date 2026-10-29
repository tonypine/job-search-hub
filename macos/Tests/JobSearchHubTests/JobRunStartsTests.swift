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
private let retryRunID = UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000005")!
private let kindsPath = "/v1/jobs/\(jobID.uuidString)/runnable-kinds"
private let briefKind = "job_brief_full"

/// The full brief's kind for the job, with its last run when it has one,
/// and the started runs the hub is still running.
private func briefJSON(lastRun: UUID?, outcome: String, running: [UUID]) -> String {
    let run = lastRun.map { id in
        #"""
        ,"last_run":{"id":"\#(id.uuidString)","record":"model_call","kind":"job_brief_full",
         "subject":{"type":"job","id":"\#(jobID.uuidString)","title":"Staff engineer"},"model":"claude-sonnet","provider":"Claude",
         "started_at":"2020-10-29T14:14:00Z","outcome":"\#(outcome)","prompt_tokens":0,"completion_tokens":0,"started_by_owner":true}
        """#
    } ?? ""
    let runningIDs = running.map { "\"\($0.uuidString)\"" }.joined(separator: ",")
    return #"""
    {"id":"job_brief_full","title":"Full briefs","description":"Claude writes a full brief.","group":"jobs","subject_type":"job",
     "engine":"claude_one_shot","automatic":true,"paused":true,"can_run":true,"needs_note":false,
     "running_starts":[\#(runningIDs)]\#(run)}
    """#
}

/// The job's kinds as the hub lists them: the full brief, its last run
/// `lastRun` with `outcome`, and the started runs still `running`.
private func kinds(lastRun: UUID?, outcome: String = "succeeded", running: [UUID] = []) -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"{"kinds":[\#(briefJSON(lastRun: lastRun, outcome: outcome, running: running))]}"#)
}

/// The full brief as the popover read it, with an earlier run that landed
/// long before the starts here.
private func makeBrief() throws -> RunnableKind {
    try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(briefJSON(lastRun: UUID(), outcome: "succeeded", running: []).utf8))
}

/// What a start answers: the run's id, and the Mac's task for an agent.
private func started(_ runID: UUID, taskID: UUID? = nil) -> StubHub.Answer {
    let task = taskID.map { #","task_id":"\#($0.uuidString)""# } ?? ""
    return StubHub.Answer(status: 202, body: #"""
    {"queued":true,"run":{"id":"\#(runID.uuidString)","kind":"job_brief_full","subject_type":"job","subject_id":"\#(jobID.uuidString)",
     "provider":"Claude","model":"claude-sonnet","chosen_model":false,"started_by_owner":true\#(task)}}
    """#)
}

/// A hub that starts the first run and runs it, with no record yet.
private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/runs": started(firstRunID),
        kindsPath: kinds(lastRun: nil, running: [firstRunID]),
    ])
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

/// Waits, for up to five seconds, until the hub has had `count` requests:
/// a follow asks again only once it has read the answer before.
@MainActor
private func waitForRequests(_ count: Int, on recording: StubHub.Recording) async {
    let deadline = Date.now.addingTimeInterval(5)
    while recording.requestCount < count, Date.now < deadline {
        try? await Task.sleep(for: .milliseconds(5))
    }
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

@MainActor @Test func aRowSaysRunningThroughTheRetryOfAnInvalidFirstAnswer() async throws {
    let (client, recording) = makeHub()
    let starts = JobRunStarts(pollInterval: .milliseconds(10))
    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)

    // The first answer's record, under the started id, says invalid; the
    // hub asks again, so it still runs the start.
    recording.setAnswer(kinds(lastRun: firstRunID, outcome: "invalid", running: [firstRunID]), for: kindsPath)
    let asked = recording.requestCount
    await waitForRequests(asked + 2, on: recording)

    #expect(recording.requestCount >= asked + 2)
    #expect(starts.getRunning(briefKind, of: jobID)?.runID == firstRunID)
    #expect(starts.isBusy(briefKind, of: jobID))
    #expect(starts.revision == 0)

    // The retry, under its own id, answered, and the brief is saved.
    recording.setAnswer(kinds(lastRun: retryRunID, running: []), for: kindsPath)
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
    recording.setAnswer(kinds(lastRun: nil, running: [firstRunID, secondRunID]), for: kindsPath)
    recording.setAnswer(started(secondRunID), for: "/v1/runs")
    let secondFollow = try await starts.start(try makeBrief(), for: jobID, with: client)

    recording.setAnswer(kinds(lastRun: firstRunID, running: [secondRunID]), for: kindsPath)
    await firstFollow.value

    #expect(starts.getRunning(briefKind, of: jobID)?.runID == secondRunID)
    #expect(starts.revision == 1)

    recording.setAnswer(kinds(lastRun: secondRunID), for: kindsPath)
    await secondFollow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 2)
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
    // The hub lists no agent run as running: the agent's record says so.
    recording.setAnswer(kinds(lastRun: firstRunID, outcome: "running"), for: kindsPath)
    let starts = JobRunStarts(pollInterval: .milliseconds(10), runTask: { _, _ in false })

    let follow = try await starts.start(try makeBrief(), for: jobID, with: client)
    let asked = recording.requestCount
    await waitForRequests(asked + 2, on: recording)

    #expect(starts.getRunning(briefKind, of: jobID)?.runID == firstRunID)

    recording.setAnswer(kinds(lastRun: firstRunID), for: kindsPath)
    await follow.value

    #expect(starts.getRunning(briefKind, of: jobID) == nil)
    #expect(starts.revision == 1)
    #expect(recording.lastRequest?.url?.path() == kindsPath)
}

@MainActor @Test func aStartOnAModelPickedForTheRunAsksTheHubForThatModel() async throws {
    let (client, recording) = makeHub()
    recording.setAnswer(started(firstRunID, taskID: taskID), for: "/v1/runs")
    let starts = JobRunStarts(pollInterval: .milliseconds(10), runTask: { _, _ in true })
    let sonnet = RoutedModel(providerID: UUID(uuidString: "BBBBBBBB-0000-4000-8000-000000000006")!, model: "sonnet")

    let follow = try await starts.start(try makeBrief(), for: jobID, note: "the title has the city in it", model: sonnet, with: client)
    await follow.value

    // Only the start went to the hub, with the model and the note.
    #expect(recording.requestCount == 1)
    let body = try #require(recording.lastBody)
    let sent = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    #expect(sent["provider_id"] as? String == sonnet.providerID.uuidString && sent["model"] as? String == "sonnet")
    #expect(sent["note"] as? String == "the title has the city in it" && sent["kind"] as? String == briefKind)
}
