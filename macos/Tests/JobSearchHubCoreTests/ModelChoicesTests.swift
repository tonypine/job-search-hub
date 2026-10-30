import Foundation
@testable import JobSearchHubCore
import Testing

private let claudeID = UUID(uuidString: "c1a0de00-0000-4000-8000-000000000001")!
private let runtimeID = UUID(uuidString: "a0c41000-0000-4000-8000-000000000002")!
private let openRouterID = UUID(uuidString: "0be70000-0000-4000-8000-000000000003")!
private let studioID = UUID(uuidString: "57d10000-0000-4000-8000-000000000004")!
private let labID = UUID(uuidString: "1ab00000-0000-4000-8000-000000000005")!

private let providersJSON = #"""
{"providers":[
  {"id":"1ab00000-0000-4000-8000-000000000005","kind":"openai_compatible","name":"Lab box","base_url":"https://lab.example.com/v1","has_key":false,"enforces_schema":true},
  {"id":"0be70000-0000-4000-8000-000000000003","kind":"openrouter","name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","has_key":true,"enforces_schema":false},
  {"id":"a0c41000-0000-4000-8000-000000000002","kind":"hub_runtime","name":"This Mac","base_url":"","has_key":false,"enforces_schema":true},
  {"id":"c1a0de00-0000-4000-8000-000000000001","kind":"claude_subscription","name":"Claude","base_url":"","has_key":false,"enforces_schema":false},
  {"id":"57d10000-0000-4000-8000-000000000004","kind":"openai_compatible","name":"LM Studio","base_url":"http://127.0.0.1:1234/v1","has_key":false,"enforces_schema":false}
]}
"""#

/// The hub's answers to `GET /v1/model-providers/{id}/models`, as TP-1059
/// serves them: Claude's aliases, the runtime's files, OpenRouter's
/// catalogue with prices, and a server that only names its models.
private let modelsJSON: [UUID: String] = [
    claudeID: #"""
    {"models":["haiku","opus","sonnet"],"catalog":[{"id":"haiku","name":"Claude Haiku"},{"id":"opus","name":"Claude Opus"},{"id":"sonnet","name":"Claude Sonnet"}]}
    """#,
    runtimeID: #"{"models":["Qwen3.8-27B-Q4_K_M.gguf"],"catalog":[{"id":"Qwen3.8-27B-Q4_K_M.gguf"}]}"#,
    openRouterID: #"""
    {"models":["google/gemini-2.5-flash","anthropic/claude-sonnet-4.5","google/gemini-2.5-flash-lite","meta-llama/llama-4-scout:free","openrouter/auto"],
     "catalog":[
      {"id":"google/gemini-2.5-flash","name":"Google: Gemini 2.5 Flash","input_usd_per_million":"0.3","output_usd_per_million":"2.5","context_length":1048576},
      {"id":"anthropic/claude-sonnet-4.5","name":"Anthropic: Claude Sonnet 4.5","input_usd_per_million":"3","output_usd_per_million":"15"},
      {"id":"google/gemini-2.5-flash-lite","name":"Google: Gemini 2.5 Flash Lite","input_usd_per_million":"0.1","output_usd_per_million":"0.4"},
      {"id":"meta-llama/llama-4-scout:free","name":"Meta: Llama 4 Scout (free)","input_usd_per_million":"0","output_usd_per_million":"0"},
      {"id":"openrouter/auto","name":"Auto Router"}
     ]}
    """#,
    studioID: #"{"models":["qwen3-8b"]}"#,
    labID: #"{"models":["llama-70b"],"catalog":[{"id":"llama-70b"}]}"#,
]

private let runsJSON = #"""
{"runs":[
  {"id":"11111111-1111-4111-8111-111111111111","record":"model_call","kind":"job_brief","model":"Qwen3.8-27B-Q4_K_M.gguf","provider":"This Mac",
   "started_at":"2026-10-16T12:00:00Z","duration_ms":4000,"outcome":"succeeded","prompt_tokens":900,"completion_tokens":140,"started_by_owner":false},
  {"id":"22222222-2222-4222-8222-222222222222","record":"model_call","kind":"job_facts","model":"Qwen3.8-27B-Q4_K_M.gguf","provider":"This Mac",
   "started_at":"2026-10-16T11:00:00Z","duration_ms":6000,"outcome":"succeeded","prompt_tokens":900,"completion_tokens":140,"started_by_owner":false},
  {"id":"33333333-3333-4333-8333-333333333333","record":"model_call","kind":"job_facts","model":"Qwen3.8-27B-Q4_K_M.gguf","provider":"This Mac",
   "started_at":"2026-10-16T10:00:00Z","duration_ms":100,"outcome":"failed","prompt_tokens":0,"completion_tokens":0,"started_by_owner":false},
  {"id":"44444444-4444-4444-8444-444444444444","record":"agent_run","kind":"company_triage","model":"sonnet","provider":"This Mac",
   "started_at":"2026-10-16T09:00:00Z","duration_ms":125000,"outcome":"succeeded","prompt_tokens":0,"completion_tokens":0,"started_by_owner":true},
  {"id":"55555555-5555-4555-8555-555555555555","record":"model_call","kind":"mail_triage","model":"qwen3-8b","provider":"LM Studio",
   "started_at":"2026-10-16T08:00:00Z","duration_ms":61500,"outcome":"succeeded","prompt_tokens":10,"completion_tokens":10,"started_by_owner":false}
 ],"next_cursor":""}
"""#

private func decodeProviders() throws -> [ModelProvider] {
    try HubJSON.makeDecoder().decode(ModelProvidersResponse.self, from: Data(providersJSON.utf8)).providers
}

private func decodeCatalogs() throws -> [UUID: [ProviderModel]] {
    try modelsJSON.mapValues { try HubJSON.makeDecoder().decode(ProviderModelsResponse.self, from: Data($0.utf8)).entries }
}

private func decodeAverages() throws -> ModelRunAverages {
    ModelRunAverages(try HubJSON.makeDecoder().decode(ActivityRunsPage.self, from: Data(runsJSON.utf8)).runs)
}

private func makeKind(_ id: String, engine: String) throws -> RunKind {
    let json = #"{"id":"\#(id)","title":"\#(id)","description":"","group":"jobs","subject_type":"job","engine":"\#(engine)","automatic":true,"paused":false,"running":0,"waiting":0}"#
    return try HubJSON.makeDecoder().decode(RunKind.self, from: Data(json.utf8))
}

private func makeRoute(_ kind: String, on providerID: UUID, _ model: String, fallback: RoutedModel? = nil) -> TaskRoute {
    TaskRoute(kind: kind, providerID: providerID, model: model, fallbackProviderID: fallback?.providerID, fallbackModel: fallback?.model)
}

private func getGroups(claudeOnly: Bool = false, keeping current: RoutedModel? = nil) throws -> [ModelChoiceGroup] {
    ModelChoices.group(providers: try decodeProviders(), catalogs: try decodeCatalogs(), averages: try decodeAverages(), claudeOnly: claudeOnly, keeping: current)
}

private func getOpenRouter() throws -> ModelChoiceGroup {
    try #require(try getGroups().first { $0.place == .openRouter })
}

@Test func aProvidersCatalogDecodesWithItsPricesAndAnOlderHubsIdsStandForIt() throws {
    let catalogs = try decodeCatalogs()
    let flash = try #require(catalogs[openRouterID]?.first)
    #expect(flash == ProviderModel(id: "google/gemini-2.5-flash", name: "Google: Gemini 2.5 Flash", inputUSDPerMillion: "0.3", outputUSDPerMillion: "2.5"))
    // A hub from before TP-1059 names its models without a catalog.
    #expect(catalogs[studioID] == [ProviderModel(id: "qwen3-8b")])
}

@Test func theMenuGroupsModelsByWhereTheyRunClaudeThenThisMacThenOpenRouter() throws {
    let groups = try getGroups()

    #expect(groups.map(\.title) == ["Claude subscription", "On this Mac", "OpenRouter", "Lab box"])
    #expect(groups.map(\.isSearchable) == [false, false, true, false])
    // Claude's strongest first, by the names its catalog gives them.
    #expect(groups[0].choices.map(\.title) == ["Claude Opus", "Claude Sonnet", "Claude Haiku"])
    #expect(groups[0].choices.allSatisfy { $0.routed.providerID == claudeID && $0.detail == nil })
    // The hub's runtime and a server at a loopback address both run on this
    // Mac, each with its time a run over the runs that worked.
    let thisMac = Dictionary(uniqueKeysWithValues: groups[1].choices.map { ($0.routed, $0) })
    #expect(thisMac[RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")]?.title == "Qwen3.8-27B-Q4_K_M")
    #expect(thisMac[RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")]?.detail == "about 5.0 s a run")
    #expect(thisMac[RoutedModel(providerID: studioID, model: "qwen3-8b")]?.detail == "about 1 min 1 s a run")
    #expect(thisMac.count == 2)
    // OpenRouter's by name.
    #expect(groups[2].choices.map(\.title) == [
        "Anthropic: Claude Sonnet 4.5", "Auto Router", "Google: Gemini 2.5 Flash", "Google: Gemini 2.5 Flash Lite", "Meta: Llama 4 Scout (free)",
    ])
    #expect(groups[3].choices.map(\.routed) == [RoutedModel(providerID: labID, model: "llama-70b")])
}

@Test func anAgentKindIsOfferedClaudeModelsOnly() throws {
    let groups = try getGroups(claudeOnly: try makeKind("company_triage", engine: "claude_agent").isAgent)

    #expect(groups.map(\.title) == ["Claude subscription"])
    #expect(groups[0].choices.map(\.routed.model) == ["opus", "sonnet", "haiku"])
    // A one-shot Claude call is no agent: it may run anywhere.
    #expect(try !makeKind("job_cv", engine: "claude_one_shot").isAgent)
}

@Test func searchingOpenRouterFindsAModelByAnyPartOfItsNameAndEachShowsItsPrice() throws {
    let openRouter = try getOpenRouter()

    let flash = ModelChoices.search(openRouter.choices, for: "flash")
    #expect(flash.map(\.routed.model) == ["google/gemini-2.5-flash", "google/gemini-2.5-flash-lite"])
    #expect(flash.map(\.detail) == ["$0.30 in · $2.50 out per M tokens", "$0.10 in · $0.40 out per M tokens"])
    // Any case, every word, the id as well as the name.
    #expect(ModelChoices.search(openRouter.choices, for: "  FLASH lite ").map(\.routed.model) == ["google/gemini-2.5-flash-lite"])
    #expect(ModelChoices.search(openRouter.choices, for: "meta-llama").map(\.title) == ["Meta: Llama 4 Scout (free)"])
    #expect(ModelChoices.search(openRouter.choices, for: "sonnet 4.5").map(\.routed.model) == ["anthropic/claude-sonnet-4.5"])
    #expect(ModelChoices.search(openRouter.choices, for: "mistral").isEmpty)
    #expect(ModelChoices.search(openRouter.choices, for: " ").count == openRouter.choices.count)
}

@Test func aPriceIsPerMillionTokensInAndOut() {
    #expect(ModelChoices.describePrice(inputUSDPerMillion: "3", outputUSDPerMillion: "15") == "$3.00 in · $15.00 out per M tokens")
    #expect(ModelChoices.describePrice(inputUSDPerMillion: "0.075", outputUSDPerMillion: "0.3") == "$0.075 in · $0.30 out per M tokens")
    #expect(ModelChoices.describePrice(inputUSDPerMillion: "0.0001", outputUSDPerMillion: nil) == "$0.0001 in per M tokens")
    #expect(ModelChoices.describePrice(inputUSDPerMillion: "0", outputUSDPerMillion: "0") == "Free")
    // A router priced by the model it picks has no price of its own.
    #expect(ModelChoices.describePrice(inputUSDPerMillion: nil, outputUSDPerMillion: nil) == nil)
}

@Test func theRoutesModelStaysInItsGroupWhenItsCatalogDoesntListIt() throws {
    let fullID = RoutedModel(providerID: claudeID, model: "claude-sonnet-4-5")
    let claude = try #require(try getGroups(keeping: fullID).first)
    #expect(claude.choices.map(\.routed.model) == ["opus", "sonnet", "haiku", "claude-sonnet-4-5"])

    // A provider whose models couldn't be listed still shows the route's.
    let providers = try decodeProviders()
    let groups = ModelChoices.group(providers: providers, catalogs: [:], keeping: RoutedModel(providerID: runtimeID, model: "qwen.gguf"))
    #expect(groups.map(\.title) == ["On this Mac"])
    #expect(groups[0].choices.map(\.title) == ["qwen"])
}

@Test func aProviderAtOpenRoutersOrALoopbackAddressIsPlacedThere() throws {
    let older = #"{"id":"8ea15166-a477-4083-b079-974c73236844","kind":"openai_compatible","name":"Router","base_url":"https://openrouter.ai/api/v1","has_key":true,"enforces_schema":false}"#
    let local = #"{"id":"8ea15166-a477-4083-b079-974c73236845","kind":"openai_compatible","name":"Ollama","base_url":"http://localhost:11434/v1","has_key":false,"enforces_schema":false}"#
    let decode = { (json: String) in try HubJSON.makeDecoder().decode(ModelProvider.self, from: Data(json.utf8)) }

    #expect(try ModelChoices.getPlace(of: decode(older)) == .openRouter)
    #expect(try ModelChoices.getPlace(of: decode(local)) == .thisMac)
    let lab = try #require(try decodeProviders().first { $0.id == labID })
    #expect(ModelChoices.getPlace(of: lab) == .server(labID))
}

@Test func theLineUnderTheMenuNamesTheFallbackAndWhereItRuns() throws {
    let providers = try decodeProviders()
    let catalogs = try decodeCatalogs()
    let local = makeRoute("job_brief", on: openRouterID, "google/gemini-2.5-flash", fallback: RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf"))
    let hosted = makeRoute("job_brief", on: runtimeID, "Qwen3.8-27B-Q4_K_M.gguf", fallback: RoutedModel(providerID: openRouterID, model: "google/gemini-2.5-flash"))
    let onClaude = makeRoute("job_brief", on: runtimeID, "Qwen3.8-27B-Q4_K_M.gguf", fallback: RoutedModel(providerID: claudeID, model: "haiku"))

    #expect(ModelChoices.describeFallback(of: local, providers: providers, catalogs: catalogs) == "Falls back to Qwen3.8-27B-Q4_K_M on this Mac")
    #expect(ModelChoices.describeFallback(of: hosted, providers: providers, catalogs: catalogs) == "Falls back to Google: Gemini 2.5 Flash on OpenRouter")
    #expect(ModelChoices.describeFallback(of: onClaude, providers: providers, catalogs: catalogs) == "Falls back to Claude Haiku on your Claude subscription")
    #expect(ModelChoices.describeFallback(of: makeRoute("job_brief", on: runtimeID, "q.gguf"), providers: providers, catalogs: catalogs) == nil)
    #expect(ModelChoices.describeFallback(of: nil, providers: providers, catalogs: catalogs) == nil)
}

@Test func choosingAModelKeepsTheFallbackUnlessItIsTheModelChosenOrTheKindIsAnAgent() throws {
    let brief = try makeKind("job_brief", engine: "model_call")
    let local = RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")
    let flash = RoutedModel(providerID: openRouterID, model: "google/gemini-2.5-flash")
    let route = makeRoute("job_brief", on: runtimeID, local.model, fallback: RoutedModel(providerID: claudeID, model: "haiku"))

    #expect(ModelChoices.makeRoute(choosing: flash, for: brief, route: route)
        == TaskRouteInput(providerID: openRouterID, model: flash.model, fallbackProviderID: claudeID, fallbackModel: "haiku"))
    #expect(ModelChoices.makeRoute(choosing: RoutedModel(providerID: claudeID, model: "haiku"), for: brief, route: route)
        == TaskRouteInput(providerID: claudeID, model: "haiku", fallbackProviderID: nil, fallbackModel: nil))
    // A kind with no route yet gets one with no fallback.
    #expect(ModelChoices.makeRoute(choosing: flash, for: brief, route: nil)
        == TaskRouteInput(providerID: openRouterID, model: flash.model, fallbackProviderID: nil, fallbackModel: nil))
    // The hub refuses an agent's route with a fallback.
    let research = try makeKind("company_triage", engine: "claude_agent")
    #expect(ModelChoices.makeRoute(choosing: RoutedModel(providerID: claudeID, model: "opus"), for: research, route: route).fallbackProviderID == nil)

    #expect(ModelChoices.makeRoute(fallingBackTo: flash, route: route)
        == TaskRouteInput(providerID: runtimeID, model: local.model, fallbackProviderID: openRouterID, fallbackModel: flash.model))
    #expect(ModelChoices.makeRoute(fallingBackTo: nil, route: route)
        == TaskRouteInput(providerID: runtimeID, model: local.model, fallbackProviderID: nil, fallbackModel: nil))
}

@Test func theRowNamesTheModelChosenAsItsCatalogDoes() throws {
    let providers = try decodeProviders()
    let catalogs = try decodeCatalogs()
    let brief = try makeKind("job_brief", engine: "model_call")
    let route = makeRoute("job_brief", on: openRouterID, "google/gemini-2.5-flash")

    let named = try #require(RunKinds.getModel(for: brief, routes: [route], providers: providers, catalogs: catalogs))
    #expect(named.name == "Google: Gemini 2.5 Flash" && named.providerName == "OpenRouter" && named.markSymbol == "arrow.triangle.branch")
    // Before the catalogs are read, the route's id.
    #expect(RunKinds.getModel(for: brief, routes: [route], providers: providers)?.name == "google/gemini-2.5-flash")
}

@Test func theHeaderNamesTheModelTheMenuTicksOnAProviderSetUpSinceTheColumnRead() throws {
    let brief = try makeKind("job_brief", engine: "model_call")
    let providers = try decodeProviders()
    let catalogs = try decodeCatalogs()
    // The column read the providers before OpenRouter was set up, and the
    // kind has no route yet: its header says Choose a model.
    var kindRoutes = KindRoutes(providers: providers.filter { $0.id != openRouterID })
    #expect(kindRoutes.getModel(for: brief) == nil && kindRoutes.getSelection(for: brief) == nil)

    // The menu, as it opens, reads the providers into the same list, with
    // OpenRouter; the owner chooses Gemini 2.5 Flash, and the hub answers
    // with the route saved.
    kindRoutes.providers = providers
    kindRoutes.catalogs = catalogs
    let openRouter = try #require(ModelChoices.group(providers: kindRoutes.providers, catalogs: kindRoutes.catalogs).first { $0.place == .openRouter })
    let flash = try #require(openRouter.choices.first { $0.routed.model == "google/gemini-2.5-flash" })
    let local = RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")
    kindRoutes.routes = [makeRoute("job_brief", on: flash.routed.providerID, flash.routed.model, fallback: local)]

    #expect(kindRoutes.getSelection(for: brief) == flash.routed)
    let named = try #require(kindRoutes.getModel(for: brief))
    #expect(named.name == flash.title && named.markSymbol == "arrow.triangle.branch")
    #expect(named.runsOnText == "Runs on Google: Gemini 2.5 Flash from OpenRouter")
    #expect(kindRoutes.getFallbackLine(for: brief) == "Falls back to Qwen3.8-27B-Q4_K_M on this Mac")

    // An agent's header has no fallback line, whatever its route says.
    let research = try makeKind("company_triage", engine: "claude_agent")
    kindRoutes.routes.append(makeRoute("company_triage", on: claudeID, "opus", fallback: local))
    #expect(kindRoutes.getModel(for: research)?.name == "Claude Opus")
    #expect(kindRoutes.getFallbackLine(for: research) == nil)
}

@Test func aRouteOnAProviderTheListHasntReadYetIsStillNamed() throws {
    let brief = try makeKind("job_brief", engine: "model_call")
    let research = try makeKind("company_triage", engine: "claude_agent")
    // The routes read, and the providers not yet, or not since one was set
    // up.
    let kindRoutes = KindRoutes(routes: [
        makeRoute("job_brief", on: openRouterID, "google/gemini-2.5-flash", fallback: RoutedModel(providerID: runtimeID, model: "q.gguf")),
        makeRoute("company_triage", on: claudeID, "opus"),
    ])

    let named = try #require(kindRoutes.getModel(for: brief))
    #expect(named.name == "google/gemini-2.5-flash" && named.providerName.isEmpty && named.markSymbol == "cube")
    #expect(named.runsOnText == "Runs on google/gemini-2.5-flash")
    #expect(kindRoutes.getSelection(for: brief) == RoutedModel(providerID: openRouterID, model: "google/gemini-2.5-flash"))
    // A Claude kind's route names its model, not Claude alone.
    #expect(kindRoutes.getModel(for: research)?.name == "opus")
    // The fallback's line waits for its provider, which says where it runs.
    #expect(kindRoutes.getFallbackLine(for: brief) == nil)
}

/// A job's kind as its Run popover lists it, routed to `route` when given.
private func makeRunnableKind(_ id: String, engine: String, route: RoutedModel? = nil, provider: String = "") throws -> RunnableKind {
    let routeJSON = route.map { #","route":{"provider_id":"\#($0.providerID.uuidString)","provider":"\#(provider)","model":"\#($0.model)"}"# } ?? ""
    let json = #"""
    {"id":"\#(id)","title":"\#(id)","description":"","subject_type":"job","engine":"\#(engine)","automatic":true,"paused":false,
     "can_run":true,"needs_model":false,"needs_note":false,"running_starts":[]\#(routeJSON)}
    """#
    return try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(json.utf8))
}

@Test func aRunsSelectStartsOnItsKindsModelAndOnlyAnotherModelGoesWithTheStart() throws {
    let local = RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")
    let flash = RoutedModel(providerID: openRouterID, model: "google/gemini-2.5-flash")
    var selection = RunModelSelection(kind: try makeRunnableKind("job_cv", engine: "claude_one_shot", route: local, provider: "This Mac"))

    #expect(selection.kindModel == local && selection.selected == local && selection.chosen == nil)

    selection.pick(flash)
    #expect(selection.selected == flash && selection.chosen == flash)
    // The kind's model stays the route's.
    #expect(selection.kindModel == local)

    // Picking the kind's own model again leaves the start to the kind.
    selection.pick(local)
    #expect(selection.selected == local && selection.chosen == nil)
}

@Test func aRunsSelectNamesTheModelWithItsProvidersMark() throws {
    let providers = try decodeProviders()
    let catalogs = try decodeCatalogs()
    let local = RoutedModel(providerID: runtimeID, model: "Qwen3.8-27B-Q4_K_M.gguf")
    var selection = RunModelSelection(kind: try makeRunnableKind("job_cv", engine: "claude_one_shot", route: local, provider: "This Mac"))

    let own = try #require(selection.getLabel(providers: providers, catalogs: catalogs))
    #expect(own.name == "Qwen3.8-27B-Q4_K_M" && own.markSymbol == "cpu" && own.runsOnText == "Runs on Qwen3.8-27B-Q4_K_M from This Mac")
    // Before the providers are read, the route names its provider.
    #expect(selection.getLabel(providers: [], catalogs: [:])?.runsOnText == "Runs on Qwen3.8-27B-Q4_K_M from This Mac")

    selection.pick(RoutedModel(providerID: openRouterID, model: "google/gemini-2.5-flash"))
    let picked = try #require(selection.getLabel(providers: providers, catalogs: catalogs))
    #expect(picked.name == "Google: Gemini 2.5 Flash" && picked.providerName == "OpenRouter" && picked.markSymbol == "arrow.triangle.branch")

    // A kind nothing routes names no model.
    #expect(RunModelSelection(kind: try makeRunnableKind("job_facts", engine: "model_call")).getLabel(providers: providers, catalogs: catalogs) == nil)
}

@Test func aRunsSelectListsNamesWithNoPriceOrTime() throws {
    let selection = RunModelSelection(kind: try makeRunnableKind("job_cv", engine: "claude_one_shot"))
    let groups = selection.getGroups(providers: try decodeProviders(), catalogs: try decodeCatalogs())

    #expect(groups.map(\.title) == ["Claude subscription", "On this Mac", "OpenRouter", "Lab box"])
    let choices = groups.flatMap(\.choices)
    #expect(choices.contains { $0.routed.model == "google/gemini-2.5-flash" && $0.title == "Google: Gemini 2.5 Flash" })
    #expect(choices.allSatisfy { $0.detail == nil })
    for choice in choices {
        #expect(!choice.title.contains("$") && !choice.title.contains("token") && !choice.title.contains("a run"))
    }
    // The full menu has them; the compact form drops them.
    let full = try getGroups().flatMap(\.choices)
    #expect(full.contains { $0.detail?.contains("per M tokens") == true } && full.contains { $0.detail?.hasSuffix("a run") == true })
    #expect(ModelChoices.makeCompact(try getGroups()).flatMap(\.choices).allSatisfy { $0.detail == nil })
}

@Test func anAgentsSelectListsClaudeModelsOnly() throws {
    let providers = try decodeProviders()
    let catalogs = try decodeCatalogs()
    var fix = RunModelSelection(kind: try makeRunnableKind("job_fix", engine: "claude_agent"))

    let groups = fix.getGroups(providers: providers, catalogs: catalogs)
    #expect(groups.map(\.title) == ["Claude subscription"])
    #expect(groups[0].choices.map(\.routed.model) == ["opus", "sonnet", "haiku"])
    #expect(groups[0].choices.allSatisfy { $0.detail == nil })
    // With no route it runs on the model Claude Code picks: Claude.
    #expect(fix.selected == nil && fix.getLabel(providers: providers, catalogs: catalogs)?.name == "Claude")

    fix.pick(RoutedModel(providerID: claudeID, model: "sonnet"))
    #expect(fix.chosen == RoutedModel(providerID: claudeID, model: "sonnet"))
    #expect(fix.getLabel(providers: providers, catalogs: catalogs)?.name == "Claude Sonnet")
}
