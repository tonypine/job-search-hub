import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// A job's and a company's Runs tab, against a stub of the hub: every run for
// the subject, page after page, and a run started from the Run popover at
// the top. Made-up jobs only.

private let jobID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000001")!
private let companyID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000002")!
private let startedID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000099")!

private func runID(_ number: Int) -> UUID {
    UUID(uuidString: "EEEEEEEE-0000-4000-8000-\(String(format: "%012d", 100 + number))")!
}

/// The path and query of a page of the subject's runs, after `cursor`.
private func pageKey(_ type: String, _ id: UUID, limit: Int = 50, cursor: String? = nil) -> String {
    let after = cursor.map { "&cursor=\($0)" } ?? ""
    return "/v1/runs?subject_type=\(type)&subject_id=\(id.uuidString.lowercased())&limit=\(limit)\(after)"
}

/// A run for the job, started at 14:`minute`.
private func runJSON(_ id: UUID, minute: Int, kind: String = "job_brief", outcome: String = "succeeded", byOwner: Bool = false) -> String {
    #"""
    {"id":"\#(id.uuidString)","record":"model_call","kind":"\#(kind)",
     "subject":{"type":"job","id":"\#(jobID.uuidString)","title":"Staff engineer"},"model":"qwen3-8b.gguf","provider":"This Mac",
     "started_at":"2026-10-29T14:\#(String(format: "%02d", minute)):00Z","duration_ms":4200,"outcome":"\#(outcome)",
     "prompt_tokens":0,"completion_tokens":0,"started_by_owner":\#(byOwner)}
    """#
}

private func page(_ runs: [String], next: String = "") -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"{"runs":[\#(runs.joined(separator: ","))],"next_cursor":"\#(next)"}"#)
}

private func makeClient(_ answers: [String: StubHub.Answer]) -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: answers)
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

@MainActor
@Test func theTabListsEveryRunForTheJobPastTheFirstPage() async {
    let (client, _) = makeClient([
        pageKey("job", jobID): page([runJSON(runID(1), minute: 58), runJSON(runID(2), minute: 57)], next: "c1"),
        pageKey("job", jobID, cursor: "c1"): page([runJSON(runID(3), minute: 56)]),
    ])
    let model = SubjectRunsModel()

    await model.load(.job(jobID), with: client)
    #expect(model.subject == .job(jobID) && model.failure == nil)
    #expect(model.runs.map(\.id) == [runID(1), runID(2)] && model.hasMore)

    await model.loadMore(with: client)
    #expect(model.runs.map(\.id) == [runID(1), runID(2), runID(3)])
    #expect(!model.hasMore)
}

@MainActor
@Test func readingAgainPutsANewRunOnTopAndKeepsTheOlderPages() async {
    let (client, recording) = makeClient([
        pageKey("job", jobID): page([runJSON(runID(1), minute: 58), runJSON(runID(2), minute: 57)], next: "c1"),
        pageKey("job", jobID, cursor: "c1"): page([runJSON(runID(3), minute: 56)], next: "c2"),
    ])
    let model = SubjectRunsModel()
    await model.load(.job(jobID), with: client)
    await model.loadMore(with: client)

    // The owner started a brief: the hub's latest page now leads with it.
    recording.setAnswer(
        page([runJSON(startedID, minute: 59, outcome: "running", byOwner: true), runJSON(runID(1), minute: 58), runJSON(runID(2), minute: 57)], next: "c1"),
        for: pageKey("job", jobID)
    )
    await model.load(.job(jobID), with: client)

    #expect(model.runs.map(\.id) == [startedID, runID(1), runID(2), runID(3)])
    #expect(model.runs.first?.startedByOwner == true && model.hasRunning)
    #expect(model.nextCursor == "c2")
}

@MainActor
@Test func aRunStartedHereShowsFirstUntilTheHubListsIt() async {
    let (client, recording) = makeClient([pageKey("job", jobID): page([runJSON(runID(1), minute: 50)])])
    let model = SubjectRunsModel()
    let start = UnlistedStart(id: startedID, kind: "job_brief", startedAt: .now)
    #expect(model.getUnlisted([start]).isEmpty, "Nothing shows before the first read")

    await model.load(.job(jobID), with: client)
    #expect(model.getUnlisted([start]) == [start])

    recording.setAnswer(page([runJSON(startedID, minute: 59, outcome: "running", byOwner: true), runJSON(runID(1), minute: 50)]), for: pageKey("job", jobID))
    await model.load(.job(jobID), with: client)
    #expect(model.getUnlisted([start]).isEmpty)
    #expect(model.runs.first?.id == startedID && model.runs.first?.startedByOwner == true)
}

@MainActor
@Test func aCompanysTabAsksForTheCompanysRuns() async {
    let (client, recording) = makeClient([
        pageKey("company", companyID): page([runJSON(runID(1), minute: 58, kind: "company_triage"), runJSON(runID(2), minute: 57, kind: "job_finder")]),
    ])
    let model = SubjectRunsModel()

    await model.load(.company(companyID), with: client)

    #expect(recording.lastRequest?.url?.query()?.hasPrefix("subject_type=company&subject_id=\(companyID.uuidString.lowercased())") == true)
    #expect(model.runs.map(\.kind) == ["company_triage", "job_finder"] && !model.hasMore)
}

@MainActor
@Test func aSubjectWithNoRunsListsNone() async {
    let (client, _) = makeClient([pageKey("job", jobID): page([])])
    let model = SubjectRunsModel()

    await model.load(.job(jobID), with: client)

    #expect(model.subject == .job(jobID) && model.runs.isEmpty && !model.hasMore && model.failure == nil)
}
