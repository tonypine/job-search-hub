import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// The run controls of a job's sections, against a stub of the hub: the
// facts popover's Read facts, the Why it fits card's Write full brief and
// the CV section's Draft CV. A select reads no provider's models until it
// opens: the brief's shows on every pre-briefed job, and the hub asks
// openrouter.ai for OpenRouter's each time (TP-1267's review). Made-up jobs
// only.

private let jobID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000001")!
private let openRouterID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000002")!
private let kindsPath = "/v1/jobs/\(jobID.uuidString)/runnable-kinds"
private let providersPath = "/v1/model-providers"
private let modelsPath = "/v1/model-providers/\(openRouterID.uuidString)/models"

private let providersJSON = #"""
{"providers":[{"id":"\#(openRouterID.uuidString)","kind":"openrouter","name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","has_key":true,"enforces_schema":false}]}
"""#

/// The job's full brief, routed to a model on OpenRouter.
private let kindsJSON = #"""
{"kinds":[{"id":"job_brief_full","title":"Full briefs","description":"Writes a full brief.","subject_type":"job","engine":"claude_one_shot",
 "automatic":true,"paused":false,"route":{"provider_id":"\#(openRouterID.uuidString)","provider":"OpenRouter","model":"anthropic/claude-sonnet-4.5"},
 "can_run":true,"needs_note":false,"running_starts":[]}]}
"""#

private let modelsJSON = #"""
{"models":["anthropic/claude-sonnet-4.5","google/gemini-2.5-flash"],
 "catalog":[{"id":"anthropic/claude-sonnet-4.5","name":"Anthropic: Claude Sonnet 4.5","input_usd_per_million":"3","output_usd_per_million":"15"},
  {"id":"google/gemini-2.5-flash","name":"Google: Gemini 2.5 Flash","input_usd_per_million":"0.3","output_usd_per_million":"2.5"}]}
"""#

/// A hub that lists the job's kinds, the providers and OpenRouter's models.
private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        kindsPath: StubHub.Answer(status: 200, body: kindsJSON),
        providersPath: StubHub.Answer(status: 200, body: providersJSON),
        modelsPath: StubHub.Answer(status: 200, body: modelsJSON),
    ])
    return (HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session), recording)
}

@MainActor @Test func aSectionsSelectShowsTheKindsModelWithoutReadingAnyProvidersModels() async throws {
    let (client, recording) = makeHub()
    let choices = ModelChoicesModel()

    let prepared = await RunModelSelect.prepare(kind: JobRuns.fullBriefKind, forJob: jobID, choices: choices, with: client)

    let selection = try #require(prepared)
    #expect(selection.selected == RoutedModel(providerID: openRouterID, model: "anthropic/claude-sonnet-4.5"))
    #expect(selection.chosen == nil)
    // The providers alone name and mark it.
    let label = try #require(selection.getLabel(providers: choices.providers, catalogs: choices.catalogs))
    #expect(label.providerName == "OpenRouter" && label.markSymbol == "arrow.triangle.branch")
    #expect(Set(recording.requestPaths) == [kindsPath, providersPath])
    #expect(recording.requestCount == 2)
    #expect(choices.catalogs.isEmpty && !choices.hasLoaded)
}

@MainActor @Test func aSectionWhoseKindTheJobDoesntListShowsNoSelect() async {
    let (client, _) = makeHub()

    let prepared = await RunModelSelect.prepare(kind: JobRuns.cvKind, forJob: jobID, choices: ModelChoicesModel(), with: client)

    #expect(prepared == nil)
}

@MainActor @Test func theSelectListsTheModelsOnceItOpensWithNoRunTimes() async throws {
    let (client, recording) = makeHub()
    let choices = ModelChoicesModel()
    let prepared = await RunModelSelect.prepare(kind: JobRuns.fullBriefKind, forJob: jobID, choices: choices, with: client)
    let selection = try #require(prepared)

    // What the select's menu reads as it opens.
    await choices.load(with: client, readsAverages: false)

    #expect(recording.requestPaths.contains(modelsPath))
    #expect(!recording.requestPaths.contains("/v1/runs"))
    let titles = selection.getGroups(providers: choices.providers, catalogs: choices.catalogs).flatMap(\.choices).map(\.title)
    #expect(Set(titles) == ["Anthropic: Claude Sonnet 4.5", "Google: Gemini 2.5 Flash"])
}
