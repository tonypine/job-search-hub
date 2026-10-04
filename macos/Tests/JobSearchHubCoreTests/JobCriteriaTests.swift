import Foundation
@testable import JobSearchHubCore
import Testing

private let savedJSON = #"""
{"criteria":{"roles":["Product Engineer"],"excluded_role_terms":["Sales"],"search_terms":["react","typescript"],"technologies":["TypeScript"],"seniority_levels":["Senior"],
 "home_country":"Brazil","eligible_location_terms":["LATAM"],"ineligible_location_terms":null,
 "take_home":{"currency":"BRL","minimum_monthly":16000,"target_monthly":44000,"clt":{"share":0.73,"payments_per_year":13.33},
              "pj":{"share":0.82,"payments_per_year":12},"foreign_contractor":{"share":0.84,"payments_per_year":12}},
 "refuse_hourly_work":true},
 "updated_at":"2026-09-28T15:03:29.292041Z"}
"""#

@Test func criteriaDecodeWithNullListsAsEmpty() throws {
    let saved = try HubJSON.makeDecoder().decode(SavedJobCriteria.self, from: Data(savedJSON.utf8))

    #expect(saved.criteria.searchTerms == ["react", "typescript"])
    #expect(saved.criteria.excludedRoleTerms == ["Sales"])
    #expect(saved.criteria.ineligibleLocationTerms.isEmpty)
    #expect(saved.criteria.takeHome?.clt.paymentsPerYear == 13.33)
    #expect(saved.criteria.takeHome?.foreignContractor.share == 0.84)
}

@Test func theDraftEditsListsAsTextAndKeepsTheTakeHomeWhileItsCheckIsOff() throws {
    let criteria = try HubJSON.makeDecoder().decode(SavedJobCriteria.self, from: Data(savedJSON.utf8)).criteria
    var draft = JobCriteriaDraft(criteria)

    #expect(draft.searchTerms == "react, typescript")
    #expect(draft.makeCriteria() == criteria)

    draft.searchTerms = " react ,, vue,"
    draft.judgesTakeHome = false
    let edited = draft.makeCriteria()
    #expect(edited.searchTerms == ["react", "vue"])
    #expect(edited.takeHome == nil)
    #expect(draft.takeHome.minimumMonthly == 16000)
}

@MainActor @Test func aSaveSendsTheWholeRecordInTheServersKeys() async throws {
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/job-criteria": .init(status: 200, body: savedJSON),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)
    let editor = JobCriteriaEditor()
    await editor.load(with: client)
    editor.draft.technologies = "TypeScript, React"

    await editor.save(with: client)

    #expect(recording.lastRequest?.httpMethod == "PUT")
    let body = try #require(recording.lastBody)
    let sent = try #require(try JSONSerialization.jsonObject(with: body) as? [String: Any])
    #expect(Set(sent.keys) == ["roles", "excluded_role_terms", "search_terms", "technologies", "seniority_levels", "home_country", "eligible_location_terms",
                                "ineligible_location_terms", "workable_timezone_terms", "unworkable_timezone_terms", "take_home", "refuse_hourly_work"])
    #expect(sent["technologies"] as? [String] == ["TypeScript", "React"])
    let takeHome = try #require(sent["take_home"] as? [String: Any])
    #expect(Set(takeHome.keys) == ["currency", "minimum_monthly", "target_monthly", "clt", "pj", "foreign_contractor"])
    #expect(editor.error == nil)
}

@MainActor @Test func aRefusedSaveKeepsTheEditAndShowsTheServersReason() async throws {
    let (session, _) = StubHub.makeSession(answers: [
        "/v1/job-criteria": .init(status: 400, body: #"{"error":"the take-home minimum and target cannot be negative"}"#),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)
    let editor = JobCriteriaEditor()
    editor.draft.judgesTakeHome = true
    editor.draft.takeHome.minimumMonthly = -1

    await editor.save(with: client)

    #expect(editor.error?.advice == "The hub turned it down: the take-home minimum and target cannot be negative")
    #expect(editor.draft.takeHome.minimumMonthly == -1)
}
