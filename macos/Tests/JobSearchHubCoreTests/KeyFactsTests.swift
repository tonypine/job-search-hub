import Foundation
@testable import JobSearchHubCore
import Testing

private let readJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"job_board","title":"Senior Product Engineer",
        "url":"https://boards.greenhouse.io/northwind/jobs/1","description":"Build checkout.","location":"Americas",
        "pay":{"ranges":[{"min":230000,"max":230000,"currency":"USD","interval":"year"}],"summary":"$230K • Offers Equity"},
        "employment_type":"Full-time","first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "company_name":"Northwind",
 "facts":{"entries":[{"key":"location","title":"Where they hire","value":"LATAM","evidence":"Remote in LATAM"},
                     {"key":"seniority","title":"Seniority","value":"Senior","evidence":"Senior engineer"},
                     {"key":"years_of_experience","title":"Years","value":6,"evidence":"6+ years building web products with React"},
                     {"key":"timezone_requirement","title":"Timezone","value":"4 h with US Eastern"},
                     {"key":"technologies","title":"Technologies","value":["React","TypeScript"]},
                     {"key":"contract","title":"Contract","value":"contractor","evidence":"as a contractor"}],
          "prompt_version":4,"model":"local-model","extracted_at":"2026-09-28T15:00:00Z"},
 "fit":{"level":"unclear","checks":[{"name":"Where they hire","verdict":"yes","reason":"names LATAM"},
                                     {"name":"Level","verdict":"yes","reason":"senior matches"},
                                     {"name":"Timezone","verdict":"unclear","reason":"4 h overlap with US Eastern"},
                                     {"name":"Stack","verdict":"yes","reason":"React matches"}]},
 "screen_out":[{"name":"Hires from Brazil","verdict":"yes","answer":"names LATAM","evidence":"Remote in LATAM"},
               {"name":"Level","verdict":"yes","answer":"senior matches","evidence":"Senior engineer"},
               {"name":"Timezone","verdict":"unclear","answer":"4 h overlap with US Eastern"},
               {"name":"Experience","verdict":"unclear","answer":"asks for 6 years; you have 5 years","evidence":"6+ years building web products with React"},
               {"name":"Contract","answer":"contractor","evidence":"as a contractor"}],
 "unseen_updates":0}
"""#

private let unreadJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Senior Product Engineer",
        "url":"https://example.com/jobs/1","location":"Americas","employment_type":"Full-time",
        "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "fit":{"level":"unclear","checks":[]},
 "unseen_updates":0}
"""#

private let seniorityEntry = #"{"key":"seniority","title":"Seniority","value":"Senior","evidence":"Senior engineer"},"#

private func decode(_ json: String) throws -> JobDetails {
    try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(json.utf8))
}

@Test func keyFactsTakeTheBoardsPayAndTheRestFromThePostingWithTheScreensVerdicts() throws {
    let facts = try decode(readJSON).keyFacts

    #expect(facts.map(\.title) == ["Pay", "Hiring", "Where they hire", "Timezone", "Level", "Stack"])
    #expect(facts.map(\.text) == ["$230K • Offers Equity", "contractor", "LATAM", "4 h with US Eastern", "Senior · 6+ years", "React, TypeScript"])
    #expect(facts.map(\.source) == [.board, .posting, .posting, .posting, .posting, .posting])
    #expect(facts.map(\.verdict) == [nil, nil, .yes, .unclear, .unclear, .yes])
    #expect(facts[4].help == "Screen · Level: senior matches\nScreen · Experience: asks for 6 years; you have 5 years")
    #expect(facts[3].help == "Screen · Timezone: 4 h overlap with US Eastern")
}

@Test func keyFactsNotReadYetSayItAndFallBackOnTheBoard() throws {
    let facts = try decode(unreadJSON).keyFacts

    #expect(facts.map(\.text) == ["Not read yet", "Full-time", "Americas", "Not read yet", "Not read yet", "Not read yet"])
    #expect(facts.map(\.source) == [nil, .board, .board, nil, nil, nil])
    #expect(facts.allSatisfy { $0.verdict == nil && $0.help == nil })
}

@Test func payNeitherOnTheBoardNorInThePostingIsNotPublished() throws {
    var details = try decode(readJSON)
    details.job.pay = nil

    #expect(details.keyFacts[0].text == "Not published" && details.keyFacts[0].source == nil)
}

@Test func payReadFromThePostingShowsWhenTheBoardHasNone() throws {
    var details = try decode(readJSON.replacingOccurrences(of: seniorityEntry, with: #"{"key":"pay_in_text","title":"Pay","value":"$120K"},"#))
    details.job.pay = nil

    #expect(details.keyFacts[0].text == "$120K" && details.keyFacts[0].source == .posting)
}

@Test func levelIsTheYearsAloneWhenThePostingStatesNoSeniority() throws {
    let withoutSeniority = readJSON.replacingOccurrences(of: seniorityEntry, with: "")

    let level = try decode(withoutSeniority).keyFacts[4]
    #expect(level.text == "6+ years" && level.source == .posting)
    let oneYear = try decode(withoutSeniority.replacingOccurrences(of: #""value":6,"#, with: #""value":1,"#)).keyFacts[4]
    #expect(oneYear.text == "1+ year")
}

@Test func theScreensQuotesAreItsJudgedRowsWithEvidence() throws {
    let quotes = try decode(readJSON).postingQuotes

    #expect(quotes.map(\.name) == ["Hires from Brazil", "Level", "Experience"])
    #expect(quotes.map(\.verdict) == [.yes, .yes, .unclear])
    #expect(quotes[2].text == "6+ years building web products with React")
}

@Test func theSourceLineNamesTheBoardOrWhereTheJobCameFrom() throws {
    var job = try decode(readJSON).job

    #expect(job.describeSource(companyName: "Northwind") == "From Northwind's Greenhouse board")
    job.url = "https://example.com/careers/1"
    #expect(job.describeSource(companyName: "Northwind") == "From Northwind's board")
    #expect(job.describeSource(companyName: nil) == "From the company's board")
    job.source = "careers_page"
    #expect(job.describeSource(companyName: "Northwind") == "From Northwind's careers page")
    job.source = "linkedin"
    #expect(job.describeSource(companyName: "Northwind") == "From LinkedIn alert")
    job.source = "manual"
    #expect(job.describeSource(companyName: "Northwind") == "Added by hand")
}
