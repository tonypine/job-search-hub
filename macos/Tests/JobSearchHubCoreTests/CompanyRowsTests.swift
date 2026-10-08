import Foundation
@testable import JobSearchHubCore
import Testing

// Made-up companies, shaped like the hub's JSON.
private let rowsJSON = ##"""
{"companies":[
  {"company":{"id":"0aa55565-58d2-4247-ba01-cba65060a316","name":"Northwind","domain":"northwind.example",
              "headquarters_country":"Canada","employee_count_range":"51-200",
              "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
   "watched_since":"2026-09-28T12:10:00Z",
   "job_boards":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"0aa55565-58d2-4247-ba01-cba65060a316",
                  "provider":"greenhouse","board_token":"northwind"}],
   "people_count":0,"connection_count":2,"known_people":["Ada Example","Kim Ray Example"],
   "fitting_jobs":3,"best_match":"strong","newest_fitting_job_seen_at":"2026-10-06T09:00:00Z","unseen_updates":0},
  {"company":{"id":"1bb55565-58d2-4247-ba01-cba65060a316","name":"Globex","domain":"globex.example",
              "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
   "watched_since":"2026-09-28T12:10:00Z",
   "job_boards":[{"id":"8c9e6679-7425-40de-944b-e07fc1f90ae7","company_id":"1bb55565-58d2-4247-ba01-cba65060a316",
                  "provider":"smartrecruiters","board_token":"globex"}],
   "people_count":0,"connection_count":0,"known_people":[],"application_phase":"Interviewing",
   "fitting_jobs":1,"best_match":"possible","newest_fitting_job_seen_at":"2026-10-06T09:00:00Z","unseen_updates":0},
  {"company":{"id":"2cc55565-58d2-4247-ba01-cba65060a316","name":"Brightline","domain":"brightline.example",
              "created_at":"2026-09-28T12:00:00Z","updated_at":"2026-09-28T12:00:00Z"},
   "job_boards":[],"people_count":0,"connection_count":0,"known_people":[],"fitting_jobs":0,"unseen_updates":0}
]}
"""##

private func makeRows() throws -> [CompanySummary] {
    try HubJSON.makeDecoder().decode(CompaniesResponse.self, from: Data(rowsJSON.utf8)).companies
}

private let today = ISO8601DateFormatter().date(from: "2026-10-06T15:00:00Z")!

private var utc: Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC")!
    return calendar
}

@Test func aCompanyRowSaysWhatIsOpenThereAndWhereYouStand() throws {
    let rows = try makeRows()
    let northwind = rows[0], globex = rows[1], brightline = rows[2]

    #expect(northwind.contextLine == "Canada · 51-200 people")
    #expect(brightline.contextLine == "brightline.example")
    #expect(northwind.openJobsText == "3 open" && northwind.bestMatch == .strong)
    #expect(brightline.openJobsText == "None that pass" && brightline.bestMatch == nil)
    #expect(northwind.getStanding(now: today, calendar: utc) == .newJobToday)
    #expect(northwind.getStanding(now: today.addingTimeInterval(86400), calendar: utc) == nil)
    #expect(globex.getStanding(now: today, calendar: utc) == .phase("Interviewing"))
    #expect(CompanyStanding.newJobToday.text == "New job today")
    #expect(brightline.getStanding(now: today, calendar: utc) == nil)
    #expect(northwind.boardName == "Greenhouse" && globex.boardName == "SmartRecruiters" && brightline.boardName == nil)
}

@Test func theScopesListTheWatchListAndTheCompaniesWithOpenJobs() throws {
    let rows = try makeRows()
    #expect(CompanyScope.watching.getSummaries(rows).map(\.company.name) == ["Northwind", "Globex"])
    #expect(CompanyScope.withOpenJobs.getSummaries(rows).map(\.company.name) == ["Northwind", "Globex"])
    #expect(CompanyScope.suggested.getSummaries(rows).isEmpty)
    #expect(CompanyScope.allCases.map(\.title) == ["Watching", "With open jobs", "Suggested"])
}

@Test func aResearchIsMatchedToItsRowByNameDomainOrLink() throws {
    let northwind = try makeRows()[0]
    #expect(northwind.isResearched(as: "Northwind"))
    #expect(northwind.isResearched(as: "northwind.example"))
    #expect(northwind.isResearched(as: "https://www.northwind.example/careers"))
    #expect(!northwind.isResearched(as: "Northwind Labs"))
    #expect(!northwind.isResearched(as: ""))

    let json = #"{"organization":"Hooli","source":"startups_gallery","website":"https://www.hooli.example/","connection_count":0,"open_jobs":3,"fitting_jobs":1}"#
    let suggestion = try HubJSON.makeDecoder().decode(CompanySuggestion.self, from: Data(json.utf8))
    #expect(suggestion.isResearched(as: "hooli.example") && suggestion.isResearched(as: "Hooli"))
    #expect(!suggestion.isResearched(as: "Globex"))
}

@Test func theResearchStepIsTheLastToolTheAgentCalled() {
    #expect(ResearchStep(lines: ["Researching Acme (agent run 1, prompt version 3)"]) == ResearchStep(text: "starting", number: 0))
    let lines = [
        "Researching Acme (agent run 1, prompt version 3)",
        #"  · WebSearch {"query":"acme careers"}"#,
        #"  · find_companies {"query":"Acme"}"#,
        #"  · WebFetch {"url":"https://www.acme.example/careers","prompt":"List the open roles…"#,
    ]
    let step = ResearchStep(lines: lines)
    #expect(step == ResearchStep(text: "reading acme.example", number: 3))
    #expect(step.line == "Researching: reading acme.example · step 3")
    #expect(ResearchStep(lines: Array(lines.prefix(2))).text == "searching the web")
    #expect(ResearchStep(lines: [#"  · set_job_board {"provider":"ashby"}"#]).text == "setting its job board")
    #expect(ResearchStep(lines: [#"  · record_company_jobs {}"#]).text == "record company jobs")
}
