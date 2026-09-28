import Foundation
@testable import JobSearchHubCore
import Testing

private let detailsJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"job_board","title":"Senior Software Engineer, Agents",
        "url":"https://jobs.ashbyhq.com/acme/7c9e","description":"Build agents.","location":"Americas","workplace_type":"Remote",
        "pay":{"ranges":[{"min":230000,"max":230000,"currency":"USD","interval":"year"}],"summary":"$230K • Offers Equity"},
        "employment_type":"Full-time","department":"Engineering","other_locations":["EMEA"],"published_at":"2026-09-02T10:22:06.45Z",
        "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "company_name":"Acme",
 "facts":{"entries":[{"key":"summary","title":"Summary","description":"One line.","value":"Builds agents."},
                     {"key":"technologies","title":"Technologies","value":["Go","TypeScript"]},
                     {"key":"years_of_experience","title":"Years of experience","value":8},
                     {"key":"timezone_requirement","title":"Timezone","value":"not stated"},
                     {"key":"visa","title":"Visa","value":null},
                     {"key":"languages","title":"Languages","value":[]}],
          "prompt_version":2,"model":"qwen/qwen3.5-9b","extracted_at":"2026-09-28T15:00:00Z"},
 "application":{"id":"22222222-0000-0000-0000-000000000001","job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","phase_id":"11111111-0000-0000-0000-000000000001",
                "phase_entered_at":"2026-09-28T10:00:00Z","created_at":"2026-09-28T10:00:00Z","updated_at":"2026-09-28T10:00:00Z"},
 "phase":{"id":"11111111-0000-0000-0000-000000000001","name":"Saved","position":1,"is_closed":false}}
"""#

@Test func jobDetailsDecodeWithBoardFactsFactsAndPhase() throws {
    let details = try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(detailsJSON.utf8))

    #expect(details.companyName == "Acme")
    #expect(details.job.pay?.summary == "$230K • Offers Equity")
    #expect(details.job.otherLocations == ["EMEA"])
    #expect(details.job.employmentType == "Full-time" && details.job.publishedAt != nil)
    #expect(details.facts?.promptVersion == 2 && details.facts?.entries.count == 6)
    #expect(details.phase?.name == "Saved" && details.application?.phaseID == details.phase?.id)
}

@Test func factsDisplayAsTextListsOrNotStated() throws {
    let entries = try #require(try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(detailsJSON.utf8)).facts?.entries)

    #expect(entries.map(\.key) == ["summary", "technologies", "years_of_experience", "timezone_requirement", "visa", "languages"])
    #expect(entries[0].display == .text("Builds agents."))
    #expect(entries[1].display == .list(["Go", "TypeScript"]))
    #expect(entries[2].display == .text("8"))
    #expect(entries[3].display == .notStated)
    #expect(entries[4].display == .notStated)
    #expect(entries[5].display == .notStated)
}

@Test func payRangesReadAsAmountsWithTheirInterval() {
    let locale = Locale(identifier: "en_US")
    let yearly = PayRange(label: nil, min: 152_000, max: 190_000, currency: "USD", interval: "year")
    let single = PayRange(label: nil, min: 230_000, max: 230_000, currency: "USD", interval: nil)
    let hourly = PayRange(label: nil, min: 45.5, max: 45.5, currency: "CAD", interval: "hour")

    #expect(yearly.format(locale: locale) == "$152,000 – $190,000 a year")
    #expect(single.format(locale: locale) == "$230,000")
    #expect(hourly.format(locale: locale) == "CA$45.5 an hour")
}
