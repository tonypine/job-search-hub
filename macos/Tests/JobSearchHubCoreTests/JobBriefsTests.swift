import Foundation
@testable import JobSearchHubCore
import Testing

private let briefedDetailsJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Frontend Engineer","url":"https://acme.com/1",
        "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "fit":{"level":"good","checks":[]},"unseen_updates":0,
 "brief":{"job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","tier":"full","prompt_id":"11111111-0000-0000-0000-000000000001","model":"claude-sonnet-5-5",
          "match":"strong","reason":"React at scale.","written_at":"2026-09-30T20:34:30.1Z","is_stale":true,
          "strengths":[{"point":"React","entry_ids":["aaaaaaaa-0000-0000-0000-000000000001","aaaaaaaa-0000-0000-0000-000000000009"]}],
          "weaknesses":[{"point":"No Playwright","entry_ids":[]}],
          "cited_entries":[{"id":"aaaaaaaa-0000-0000-0000-000000000001","kind":"case","title":"Built the platform","organization":"Maple"}]},
 "screen_out":[{"name":"Hires from Brazil","verdict":"yes","answer":"open to someone in Brazil","evidence":"Americas Remote"},
               {"name":"Contract","answer":"the posting doesn't say"}],
 "decision":{"job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","decision":"later","decided_at":"2026-09-30T21:00:00.5Z"}}
"""#

@Test func aJobsDetailsCarryItsBriefScreenOutAnswersAndDecision() throws {
    let details = try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(briefedDetailsJSON.utf8))
    let brief = try #require(details.brief)

    #expect(brief.isFull && brief.match == .strong && brief.isStale)
    #expect(brief.getEntries(of: brief.strengths[0]).map(\.label) == ["Built the platform · Maple"])
    #expect(brief.weaknesses[0].entryIDs.isEmpty)
    #expect(details.screenOut?.map(\.verdict) == [.yes, nil])
    #expect(details.decision?.decision == .later && details.decision?.decision.pastTense == "Left for later")
}

@Test func aDecisionAndAFullBriefAreAskedFor() async throws {
    let jobID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/jobs/\(jobID.uuidString)/decision": StubHub.Answer(status: 200, body: #"{"decision":"skip","reason":"agency","decided_at":"2026-09-30T21:00:00Z"}"#),
        "/v1/jobs/\(jobID.uuidString)/brief/full": StubHub.Answer(status: 202, body: #"{"queued":true}"#),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    let decision = try await client.decideJob(jobID, .skip, reason: " agency ")
    #expect(decision.decision == .skip)
    let sent = try JSONSerialization.jsonObject(with: try #require(recording.lastBody)) as? [String: Any]
    #expect(sent?["decision"] as? String == "skip" && sent?["reason"] as? String == "agency")

    try await client.writeFullBrief(jobID)
    #expect(recording.lastRequest?.httpMethod == "POST" && recording.lastRequest?.url?.path == "/v1/jobs/\(jobID.uuidString)/brief/full")
}
