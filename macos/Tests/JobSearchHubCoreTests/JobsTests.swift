import Foundation
@testable import JobSearchHubCore
import Testing

private let jobsJSON = #"""
{"jobs":[{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                 "job_board_id":"1bb55565-58d2-4247-ba01-cba65060a316","external_id":"x1","source":"job_board",
                 "title":"Senior Product Engineer","location":"Americas","workplace_type":"Remote",
                 "url":"https://jobs.ashbyhq.com/acme/x1","description":"Build things.",
                 "first_seen_at":"2026-09-28T13:57:13.161025Z","last_seen_at":"2026-09-28T13:57:13.161025Z"},
          "company_name":"Acme","fit":{"level":"good","checks":[{"name":"Stack","verdict":"yes","reason":"React"}]},"unseen_updates":2},
         {"job":{"id":"8d9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Staff Engineer",
                 "url":"https://other.com/jobs/9","first_seen_at":"2026-09-28T14:00:00Z","last_seen_at":"2026-09-28T14:00:00Z",
                 "closed_at":"2026-09-29T10:00:00Z"},"fit":{"level":"poor","checks":[]},"unseen_updates":0}],
 "total":2}
"""#

@Test func theJobsListDecodes() throws {
    let response = try HubJSON.makeDecoder().decode(JobsResponse.self, from: Data(jobsJSON.utf8))

    #expect(response.total == 2)
    #expect(response.jobs[0].companyName == "Acme")
    #expect(response.jobs[0].job.companyID != nil && response.jobs[0].job.jobBoardID != nil)
    #expect(response.jobs[0].job.workplaceType == "Remote")
    #expect(response.jobs[1].job.companyID == nil && response.jobs[1].companyName == nil)
    #expect(response.jobs[1].job.closedAt != nil)
    #expect(response.jobs.map(\.unseenUpdates) == [2, 0])
}

@Test func theJobsQueryCarriesTheSearchOnlyWhenThereIsOne() {
    let withSearch = JobsQuery.makeItems(search: " react ", status: .open, limit: 500)
    let withoutSearch = JobsQuery.makeItems(search: "  ", status: .all, limit: 100)

    #expect(withSearch.contains(URLQueryItem(name: "query", value: "react")))
    #expect(!withoutSearch.contains(where: { $0.name == "query" }))
    #expect(withoutSearch.contains(URLQueryItem(name: "status", value: "all")))
}

private func makeJobsPageJSON(titles: [String], total: Int) -> String {
    let jobs = titles.map { title in
        #"{"job":{"id":"\#(UUID().uuidString)","source":"manual","title":"\#(title)","url":"https://acme.com/\#(title)","#
            + #""first_seen_at":"2026-09-28T14:00:00Z","last_seen_at":"2026-09-28T14:00:00Z"},"fit":{"level":"good","checks":[]},"unseen_updates":0}"#
    }
    return #"{"jobs":[\#(jobs.joined(separator: ","))],"total":\#(total)}"#
}

@Test func everyJobIsReadAPageAtATime() async throws {
    let (session, _) = StubHub.makeSession(answers: [
        "/v1/jobs?status=open&limit=500": StubHub.Answer(status: 200, body: makeJobsPageJSON(titles: ["a", "b"], total: 3)),
        "/v1/jobs?status=open&limit=500&offset=2": StubHub.Answer(status: 200, body: makeJobsPageJSON(titles: ["c"], total: 3)),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    let response = try await client.getAllJobs(search: "", status: .open)

    #expect(response.jobs.map(\.job.title) == ["a", "b", "c"])
    #expect(response.total == 3)
}

@Test func queryItemsLandInTheURLNotThePath() {
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t")
    let request = client.makeRequest(method: "GET", path: "v1/jobs", query: [URLQueryItem(name: "query", value: "front end")], body: nil)

    #expect(request.url?.absoluteString == "http://localhost:8090/v1/jobs?query=front%20end")
}

private func makeItem(_ title: String, _ level: FitLevel, firstSeen: TimeInterval) -> JobListItem {
    JobListItem(
        job: Job(id: UUID(), source: "manual", title: title, url: "https://acme.com/\(title)",
                 firstSeenAt: Date(timeIntervalSince1970: firstSeen), lastSeenAt: Date(timeIntervalSince1970: firstSeen)),
        companyName: nil, fit: JobFit(level: level, checks: []), unseenUpdates: 0
    )
}

@Test func jobsSortByFitThenNewest() {
    let items = [
        makeItem("old good", .good, firstSeen: 100), makeItem("poor", .poor, firstSeen: 500),
        makeItem("new unclear", .unclear, firstSeen: 400), makeItem("new good", .good, firstSeen: 300),
    ]

    #expect(JobsOrder.sort(items).map(\.job.title) == ["new good", "old good", "new unclear", "poor"])
}

@Test func aJobIsNewWhenFirstSeenAfterTheLastVisit() {
    let item = makeItem("job", .good, firstSeen: 200)

    #expect(item.isNew(since: Date(timeIntervalSince1970: 100)))
    #expect(!item.isNew(since: Date(timeIntervalSince1970: 300)))
    #expect(!item.isNew(since: nil))
}

@Test func jobsAreDismissedWithAReasonAndRestoredWithout() async throws {
    let jobID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
    let dismissedJob = #"{"jobs":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Agency Role","url":"https://acme.com/1","#
        + #""first_seen_at":"2026-09-28T14:00:00Z","last_seen_at":"2026-09-28T14:00:00Z","dismissed_at":"2026-09-30T19:00:00Z","dismissal_reason":"agency"}]}"#
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/jobs/dismiss": StubHub.Answer(status: 200, body: dismissedJob),
        "/v1/jobs/restore": StubHub.Answer(status: 200, body: #"{"jobs":[]}"#),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    let dismissed = try await client.dismissJobs([jobID], reason: "  agency \n")
    #expect(dismissed.first?.dismissedAt != nil && dismissed.first?.dismissalReason == "agency")
    let sent = try JSONSerialization.jsonObject(with: try #require(recording.lastBody)) as? [String: Any]
    #expect(sent?["job_ids"] as? [String] == [jobID.uuidString])
    #expect(sent?["reason"] as? String == "agency")
    #expect(recording.lastRequest?.httpMethod == "POST")

    _ = try await client.restoreJobs([jobID])
    let restoreBody = try JSONSerialization.jsonObject(with: try #require(recording.lastBody)) as? [String: Any]
    #expect(recording.lastRequest?.url?.path == "/v1/jobs/restore")
    #expect(restoreBody?["reason"] == nil)
}

@Test func skippedJobsAreAStatusOfTheirOwnThatTheHubCallsDismissed() {
    #expect(JobsQuery.makeItems(search: "", status: .dismissed, limit: 100).contains(URLQueryItem(name: "status", value: "dismissed")))
    #expect(JobStatusFilter.dismissed.title == "Skipped")
}
