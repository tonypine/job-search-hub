import Foundation
@testable import JobSearchHubCore
import Testing

// A job's or a company's Runs tab: its query, its rows' words, and the list
// kept once the latest page is read again. Made-up jobs only.

private let jobID = UUID(uuidString: "DDDDDDDD-0000-4000-8000-000000000001")!

/// Run `number` of `kind` for the job, started at 14:(59 - `number`).
private func makeRun(
    _ number: Int, kind: String = "job_brief", outcome: String = "succeeded", durationMS: Int? = 4200, error: String? = nil
) throws -> ActivityRun {
    let duration = durationMS.map { #","duration_ms":\#($0)"# } ?? ""
    let failure = error.map { #","error":"\#($0)""# } ?? ""
    let json = #"""
    {"id":"DDDDDDDD-0000-4000-8000-\#(String(format: "%012d", 100 + number))","record":"model_call","kind":"\#(kind)",
     "subject":{"type":"job","id":"\#(jobID.uuidString)","title":"Staff engineer"},"model":"qwen3-8b.gguf","provider":"This Mac",
     "started_at":"2026-10-29T14:\#(String(format: "%02d", 59 - number)):00Z"\#(duration),"outcome":"\#(outcome)","prompt_tokens":0,"completion_tokens":0,
     "started_by_owner":false\#(failure)}
    """#
    return try HubJSON.makeDecoder().decode(ActivityRun.self, from: Data(json.utf8))
}

@Test func theQueryAsksForTheSubjectsRunsAPageAtATime() {
    let first = SubjectRuns.makeQuery(subjectType: "job", subjectID: jobID, limit: 50)
    #expect(first == [
        URLQueryItem(name: "subject_type", value: "job"),
        URLQueryItem(name: "subject_id", value: "dddddddd-0000-4000-8000-000000000001"),
        URLQueryItem(name: "limit", value: "50"),
    ])

    let next = SubjectRuns.makeQuery(subjectType: "company", subjectID: jobID, limit: 50, cursor: "abc")
    #expect(next.last == URLQueryItem(name: "cursor", value: "abc"))
    #expect(SubjectRuns.makeQuery(subjectType: "job", subjectID: jobID, limit: 50, cursor: "") == first)
}

@Test func aRowIsNamedForOneRunOfItsKind() {
    #expect(SubjectRuns.getTitle(ofKind: "job_brief") == "Brief")
    #expect(SubjectRuns.getTitle(ofKind: "job_cv") == "Tailored CV")
    #expect(SubjectRuns.getTitle(ofKind: "company_triage") == "Company research")
    #expect(SubjectRuns.getTitle(ofKind: "job_finder") == "Find jobs")
    #expect(SubjectRuns.getTitle(ofKind: "market_gaps") == "Market gaps")
}

@Test func aFinishedRunSaysHowItEndedAndHowLongItTook() throws {
    #expect(SubjectRuns.describeOutcome(try makeRun(1)) == "Succeeded in 4.2 s")
    #expect(SubjectRuns.describeOutcome(try makeRun(2, outcome: "failed", durationMS: 125_000, error: "timed out")) == "Failed after 2 min 5 s")
    #expect(SubjectRuns.describeOutcome(try makeRun(3, outcome: "invalid", durationMS: 3000)) == "Invalid answer after 3.0 s")
    #expect(SubjectRuns.describeOutcome(try makeRun(4, outcome: "failed", durationMS: nil)) == "Failed")
}

@Test func readingAgainKeepsTheRunsLoadedPastTheLatestPage() throws {
    let runs = try (1...6).map { try makeRun($0) }
    let latest = try makeRun(0)

    // Four loaded to cursor "c2"; a new run started, so the page of four
    // now ends at the third.
    let merged = SubjectRuns.merge(ActivityRunsPage(runs: [latest] + runs[0..<3], nextCursor: "c1"), into: Array(runs[0..<4]), cursor: "c2")
    #expect(merged.runs.map(\.id) == ([latest] + runs[0..<4]).map(\.id))
    #expect(merged.nextCursor == "c2")
}

@Test func aPageThatHoldsEveryRunOrLosesItsPlaceIsTheList() throws {
    let runs = try (1...4).map { try makeRun($0) }

    let everyRun = ActivityRunsPage(runs: Array(runs[0..<2]), nextCursor: "")
    #expect(SubjectRuns.merge(everyRun, into: runs, cursor: "c2") == everyRun)

    let elsewhere = ActivityRunsPage(runs: [try makeRun(9)], nextCursor: "c1")
    #expect(SubjectRuns.merge(elsewhere, into: runs, cursor: "c2") == elsewhere)

    let sameRuns = ActivityRunsPage(runs: runs, nextCursor: "c1")
    #expect(SubjectRuns.merge(sameRuns, into: runs, cursor: "c2") == sameRuns)
}
