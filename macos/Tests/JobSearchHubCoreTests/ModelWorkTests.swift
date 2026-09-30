import Foundation
@testable import JobSearchHubCore
import Testing

private let jobID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
private let otherJobID = UUID(uuidString: "0aa55565-58d2-4247-ba01-cba65060a316")!

private let workJSON = #"""
{"paused":true,
 "running":{"kind":"job_facts","subject_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","model":"Qwen3.8-27B-Q4_K_M.gguf","priority":"dispatched","since":"2026-09-30T17:40:00Z"},
 "waiting":[{"kind":"job_facts","subject_id":"0aa55565-58d2-4247-ba01-cba65060a316","model":"Qwen3.8-27B-Q4_K_M.gguf","priority":"background","since":"2026-09-30T17:36:57.996973-03:00"},
            {"kind":"mail_triage","model":"Qwen3.5-9B-Q4_K_M.gguf","priority":"sorting","since":"2026-09-30T17:41:00Z"}],
 "runtime":{"state":"ready","model":"Qwen3.8-27B-Q4_K_M.gguf","busy":true,"loaded_at":"2026-09-30T17:40:05Z","last_used_at":"2026-09-30T17:40:05Z"},
 "jobs_awaiting_facts":509}
"""#

@Test func modelWorkDecodesWithTheRuntimeAndTheQueue() throws {
    let work = try HubJSON.makeDecoder().decode(ModelWork.self, from: Data(workJSON.utf8))

    #expect(work.paused && work.jobsAwaitingFacts == 509)
    #expect(work.running?.subjectID == jobID && work.running?.priority == "dispatched")
    #expect(work.waiting.count == 2 && work.waiting[1].subjectID == nil)
    #expect(work.runtime.state == "ready" && work.runtime.busy && work.runtime.loadedAt != nil)
}

@Test func aDispatchedReadIsFollowedThroughTheQueue() throws {
    let work = try HubJSON.makeDecoder().decode(ModelWork.self, from: Data(workJSON.utf8))

    #expect(work.getFactsReadState(for: jobID) == .running)
    #expect(work.getFactsReadState(for: otherJobID) == .queued)
    #expect(work.getFactsReadState(for: UUID()) == .notQueued)
    #expect(work.waitingCountsByKind.map(\.title) == ["Job facts", "Mail sorting"])
}
