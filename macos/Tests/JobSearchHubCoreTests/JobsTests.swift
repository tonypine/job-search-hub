import Foundation
@testable import JobSearchHubCore
import Testing

private let jobsJSON = #"""
{"jobs":[{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                 "job_board_id":"1bb55565-58d2-4247-ba01-cba65060a316","external_id":"x1","source":"job_board",
                 "title":"Senior Product Engineer","location":"Americas","workplace_type":"Remote",
                 "url":"https://jobs.ashbyhq.com/acme/x1","description":"Build things.",
                 "first_seen_at":"2026-09-28T13:57:13.161025Z","last_seen_at":"2026-09-28T13:57:13.161025Z"},
          "company_name":"Acme"},
         {"job":{"id":"8d9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Staff Engineer",
                 "url":"https://other.com/jobs/9","first_seen_at":"2026-09-28T14:00:00Z","last_seen_at":"2026-09-28T14:00:00Z",
                 "closed_at":"2026-09-29T10:00:00Z"}}],
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
}

@Test func theJobsQueryCarriesTheSearchOnlyWhenThereIsOne() {
    let withSearch = JobsQuery.makeItems(search: " react ", status: .open, limit: 500)
    let withoutSearch = JobsQuery.makeItems(search: "  ", status: .all, limit: 100)

    #expect(withSearch.contains(URLQueryItem(name: "query", value: "react")))
    #expect(!withoutSearch.contains(where: { $0.name == "query" }))
    #expect(withoutSearch.contains(URLQueryItem(name: "status", value: "all")))
}

@Test func queryItemsLandInTheURLNotThePath() {
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t")
    let request = client.makeRequest(method: "GET", path: "v1/jobs", query: [URLQueryItem(name: "query", value: "front end")], body: nil)

    #expect(request.url?.absoluteString == "http://localhost:8090/v1/jobs?query=front%20end")
}
