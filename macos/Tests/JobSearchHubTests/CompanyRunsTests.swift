import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// A company's run controls, against a stub of the hub: its open jobs
// section's select is set on find jobs' model from the company's own kinds,
// and, find jobs being an agent, lists Claude's models only. Made-up
// companies only.

private let companyID = UUID(uuidString: "DDDDDDDD-0000-4000-8000-000000000001")!
private let claudeID = UUID(uuidString: "DDDDDDDD-0000-4000-8000-000000000002")!
private let openRouterID = UUID(uuidString: "DDDDDDDD-0000-4000-8000-000000000003")!
private let companyKindsPath = "/v1/companies/\(companyID.uuidString)/runnable-kinds"
private let providersPath = "/v1/model-providers"

private let providersJSON = #"""
{"providers":[
 {"id":"\#(claudeID.uuidString)","kind":"claude_subscription","name":"Claude","base_url":"","has_key":false,"enforces_schema":false},
 {"id":"\#(openRouterID.uuidString)","kind":"openrouter","name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","has_key":true,"enforces_schema":false}
]}
"""#

/// The company's research and find jobs; find jobs routed to Sonnet.
private let kindsJSON = #"""
{"kinds":[
 {"id":"company_triage","title":"Company research","description":"An agent researches a company.","group":"companies","subject_type":"company",
  "engine":"claude_agent","automatic":false,"paused":false,"can_run":true,"needs_model":false,"needs_note":false,"running_starts":[]},
 {"id":"job_finder","title":"Find jobs","description":"An agent finds a company's open jobs.","group":"companies","subject_type":"company",
  "engine":"claude_agent","automatic":false,"paused":false,"route":{"provider_id":"\#(claudeID.uuidString)","provider":"Claude","model":"claude-sonnet-4-5"},
  "can_run":true,"needs_model":false,"needs_note":false,"running_starts":[]}
]}
"""#

private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        companyKindsPath: StubHub.Answer(status: 200, body: kindsJSON),
        providersPath: StubHub.Answer(status: 200, body: providersJSON),
    ])
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

@MainActor @Test func aCompanysFindJobsSelectIsSetOnItsKindsModelFromTheCompanysKinds() async throws {
    let (client, recording) = makeHub()
    let choices = ModelChoicesModel()

    let prepared = await RunModelSelect.prepare(kind: JobRuns.findJobsKind, for: .company(companyID), choices: choices, with: client)

    let selection = try #require(prepared)
    #expect(selection.selected == RoutedModel(providerID: claudeID, model: "claude-sonnet-4-5") && selection.chosen == nil)
    #expect(selection.isAgent)
    #expect(Set(recording.requestPaths) == [companyKindsPath, providersPath])
    // Claude's models alone, OpenRouter's left out, with no prices.
    let catalogs: [UUID: [ProviderModel]] = [
        claudeID: [ProviderModel(id: "claude-sonnet-4-5"), ProviderModel(id: "claude-opus-4-5")],
        openRouterID: [ProviderModel(id: "google/gemini-2.5-flash")],
    ]
    let groups = selection.getGroups(providers: choices.providers, catalogs: catalogs)
    #expect(!groups.isEmpty)
    #expect(groups.flatMap(\.choices).allSatisfy { $0.routed.providerID == claudeID })
}

@MainActor @Test func aCompanysPopoverRowsAreNamedForItsRuns() {
    // The company's two kinds, as the inspector's Run popover and Runs tab
    // name them.
    #expect(SubjectRunsModel.Subject.company(companyID).type == JobRuns.companySubject)
    #expect(SubjectRuns.getTitle(ofKind: JobRuns.companyResearchKind) == "Company research")
    #expect(SubjectRuns.getTitle(ofKind: JobRuns.findJobsKind) == "Find jobs")
}
