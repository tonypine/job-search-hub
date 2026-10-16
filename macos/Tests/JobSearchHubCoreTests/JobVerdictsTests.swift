import Foundation
@testable import JobSearchHubCore
import Testing

private let companyID = UUID(uuidString: "cccccccc-0000-0000-0000-000000000001")!

private func makeDetails(fit: String, brief: String = "", pay: String = "") throws -> JobDetails {
    let json = """
    {"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Senior Product Engineer","url":"https://northwind.example/1",
            \(pay)"first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
     "fit":\(fit),\(brief)"unseen_updates":0}
    """
    return try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(json.utf8))
}

private let unclearFit = #"""
{"level":"unclear","checks":[{"name":"Role","verdict":"yes","reason":"product engineer"},
                             {"name":"Years","verdict":"unclear","reason":"asks 6+, you have 5"},
                             {"name":"Where they hire","verdict":"yes","reason":"Americas"},
                             {"name":"Timezone","verdict":"unclear","reason":"4 h overlap with US Eastern"},
                             {"name":"Stack","verdict":"yes","reason":"React, TypeScript"},
                             {"name":"Pay","verdict":"yes","reason":"about BRL 31.0k a month take-home, 85% of the target"}]}
"""#

private let strongBrief = #"""
"brief":{"tier":"pre","model":"local","match":"strong","reason":"Payments cases map onto their rebuild.","written_at":"2026-09-30T20:34:30Z",
         "is_stale":false,"strengths":[],"weaknesses":[],"cited_entries":[]},
"""#

@Test func theStripReadsTheMatchTheScreenTheTakeHomeAndThePeople() throws {
    let cells = try makeDetails(fit: unclearFit, brief: strongBrief).getVerdicts(peopleYouKnow: 2)

    #expect(cells.map(\.kind) == [.match, .screen, .takeHome, .people])
    #expect(cells.map(\.word) == ["Strong", "2 unclear", "≈ BRL 31k/mo", "2 you know"])
    #expect(cells.map(\.tone) == [.positive, .caution, .positive, .accent])
    #expect(cells.allSatisfy { !$0.symbolName.isEmpty })
    #expect(cells[1].accessibilityLabel == "Screen: 2 unclear")
    #expect(cells[2].accessibilityLabel == "Take-home: ≈ BRL 31k/mo. about BRL 31.0k a month take-home, 85% of the target")
}

@Test func theStripSaysWhatIsMissingInWords() throws {
    let unpublished = try makeDetails(fit: #"{"level":"good","checks":[]}"#).getVerdicts(peopleYouKnow: 0)
    #expect(unpublished.map(\.word) == ["Not briefed", "Passes", "Not published", "No one yet"])
    #expect(unpublished.map(\.tone) == [.neutral, .positive, .neutral, .neutral])

    let unjudgedPay = try makeDetails(
        fit: #"{"level":"good","checks":[]}"#, pay: #""pay":{"ranges":[{"min":100000,"max":120000,"currency":"USD","interval":"year"}]},"#
    )
    #expect(unjudgedPay.getVerdicts(peopleYouKnow: 0)[2].word == "Not judged")
}

@Test func theScreenPutsFailingThenUnclearChecksFirstAndFoldsThePasses() {
    let rows = [
        ScreenRow(name: "Role", verdict: .yes, reason: "product engineer"),
        ScreenRow(name: "Years", verdict: .unclear, reason: "asks 6+", evidence: "6+ years"),
        ScreenRow(name: "Contract", verdict: nil, reason: "the posting doesn't say"),
        ScreenRow(name: "Where they hire", verdict: .no, reason: "US only"),
        ScreenRow(name: "Stack", verdict: .yes, reason: "React"),
    ]
    let summary = ScreenBreakdown(rows, level: .poor)

    #expect(summary.exceptions.map(\.name) == ["Where they hire", "Years"])
    #expect(summary.passes.map(\.name) == ["Role", "Stack"])
    #expect(summary.notes.map(\.name) == ["Contract"])
    #expect(summary.title == "2 of 4 pass")
    #expect(summary.verdict.word == "1 fails" && summary.verdict.tone == .negative)
    #expect(summary.verdict.detail == "1 fails, 1 unclear")
}

@Test func aScreenWhereEveryCheckPassesSaysPassesAndHasNoExceptions() {
    let summary = ScreenBreakdown([ScreenRow(name: "Role", verdict: .yes, reason: "fits"), ScreenRow(name: "Stack", verdict: .yes, reason: "Go")], level: .good)

    #expect(summary.exceptions.isEmpty && summary.passes.count == 2)
    #expect(summary.title == "Passes")
    #expect(summary.verdict.word == "Passes" && summary.verdict.tone == .positive)
    #expect(ScreenBreakdown([], level: .unclear).title == nil)
    #expect(ScreenBreakdown([], level: .unclear).verdict.word == "Unclear")
}

@Test func thePayChecksEstimateReadsAsAShortTakeHome() {
    #expect(TakeHomeEstimate.describe("about BRL 31.0k a month take-home, 85% of the target") == "≈ BRL 31k/mo")
    #expect(TakeHomeEstimate.describe("at least BRL 28.4k a month take-home") == "≥ BRL 28.4k/mo")
    #expect(TakeHomeEstimate.describe("at most BRL 12.0k a month take-home, under the BRL 20.0k minimum") == "≤ BRL 12k/mo")
    #expect(TakeHomeEstimate.describe("BRL 20.0k to 35.5k a month take-home, depending on the contract or pay period") == "BRL 20k–35.5k/mo")
    #expect(TakeHomeEstimate.describe("paid by the hour") == nil)
    #expect(TakeHomeEstimate.describe("no exchange rate from EUR to BRL") == nil)
}

@Test func anHourlyOrUnconvertedPayCheckFallsBackToAWord() throws {
    let hourly = try makeDetails(fit: #"{"level":"poor","checks":[{"name":"Pay","verdict":"no","reason":"paid by the hour"}]}"#)
    #expect(hourly.getVerdicts(peopleYouKnow: 0)[2].word == "Hourly")

    let noRate = try makeDetails(fit: #"{"level":"unclear","checks":[{"name":"Pay","verdict":"unclear","reason":"no exchange rate from EUR to BRL"}]}"#)
    let cell = noRate.getVerdicts(peopleYouKnow: 0)[2]
    #expect(cell.word == "Unclear" && cell.detail == "no exchange rate from EUR to BRL")
}

@Test func aBriefPointIsTaggedWithWhatBacksIt() throws {
    let brief = #"""
    "brief":{"tier":"full","model":"claude","match":"possible","reason":"Close.","written_at":"2026-09-30T20:34:30Z","is_stale":true,
             "strengths":[{"point":"Led a checkout rewrite","entry_ids":["aaaaaaaa-0000-0000-0000-000000000001"]},
                          {"point":"Remote with US teams","entry_ids":["aaaaaaaa-0000-0000-0000-000000000002"]},
                          {"point":"Likes payments","entry_ids":[]}],
             "weaknesses":[{"point":"No GraphQL federation","entry_ids":[]}],
             "cited_entries":[{"id":"aaaaaaaa-0000-0000-0000-000000000001","kind":"case","title":"Checkout rebuild"},
                              {"id":"aaaaaaaa-0000-0000-0000-000000000002","kind":"role","title":"Staff Engineer"}]},
    """#
    let parsed = try #require(try makeDetails(fit: #"{"level":"good","checks":[]}"#, brief: brief).brief)

    #expect(parsed.strengths.map { parsed.getTag(of: $0, isWeakness: false) } == ["Case", "Experience", nil])
    #expect(parsed.getTag(of: parsed.weaknesses[0], isWeakness: true) == "Gap")
    #expect(CitedEntry.getTag(ofKind: "skill") == "Skill")
}

@Test func theOverviewOffersAMessageOrAnIntroAndDraftsItAboutTheJob() throws {
    let json = #"""
    {"people":[
      {"key":"contact:eeeeeeee-0000-0000-0000-000000000001","relation":"contact","id":"eeeeeeee-0000-0000-0000-000000000001",
       "name":"Eli Example","relevance":"engineer","company_id":"cccccccc-0000-0000-0000-000000000001","is_agency":false,"open_jobs":0,"fitting_jobs":0},
      {"key":"introducer:bbbbbbbb-0000-0000-0000-000000000001:cccccccc-0000-0000-0000-000000000001","relation":"introducer",
       "id":"bbbbbbbb-0000-0000-0000-000000000001","name":"Riley Example","role":"Former colleague","company_id":"cccccccc-0000-0000-0000-000000000001",
       "company_name":"Northwind","preferred_channel":"WhatsApp","is_agency":false,"open_jobs":0,"fitting_jobs":0},
      {"key":"connection:dddddddd-0000-0000-0000-000000000001","relation":"connection","id":"dddddddd-0000-0000-0000-000000000001",
       "name":"Alex Example","role":"Engineering Manager","company_id":"cccccccc-0000-0000-0000-000000000001","is_agency":false,"open_jobs":0,"fitting_jobs":0}
    ]}
    """#
    let people = JobOutreach.getPeople(companyID: companyID, among: try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(json.utf8)).people)

    #expect(people.map(\.name) == ["Alex Example", "Riley Example", "Eli Example"])
    #expect(people.map(JobOutreach.isKnown) == [true, true, false])
    #expect(people.map(JobOutreach.getActionTitle(for:)) == ["Message", "Ask for intro", "Message"])

    let request = JobOutreach.makeDraftRequest(prompt: "Draft outreach.\n", to: people[1], aboutJob: "Senior Product Engineer", at: "Northwind")
    #expect(request.hasPrefix("Draft outreach.\n\nWrite it to Riley Example (Former colleague)."))
    #expect(request.contains("Reach them: WhatsApp."))
    #expect(request.hasSuffix("Ask them to introduce me to the people hiring for the Senior Product Engineer role at Northwind. Draft it for me to review; I'll send it myself."))
}

@Test func aConnectionFromTheJobStandsInForThePeopleList() {
    let connection = Connection(
        id: UUID(uuidString: "dddddddd-0000-0000-0000-000000000001")!, firstName: "Alex", lastName: "Example",
        profileURL: "https://linkedin.example/in/alex", email: nil, companyName: "Northwind", position: "Engineering Manager",
        connectedOn: nil, closeness: "12 messages"
    )
    let person = RelatedPerson(connection, companyID: companyID, companyName: nil)

    #expect(person.key == "connection:dddddddd-0000-0000-0000-000000000001")
    #expect(person.relation == .connection && person.name == "Alex Example" && person.role == "Engineering Manager")
    #expect(person.companyName == "Northwind" && person.closeness == "12 messages")
    #expect(JobOutreach.isKnown(person))
}
