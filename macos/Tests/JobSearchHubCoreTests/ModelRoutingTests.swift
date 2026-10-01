import Foundation
@testable import JobSearchHubCore
import Testing

@Test func providersAndRoutesDecodeInTheServersKeys() throws {
    let providersJSON = #"{"providers":[{"id":"2e416d12-77c2-4239-b419-b9fdd58c4cd6","kind":"hub_runtime","name":"Hub runtime","base_url":"","has_key":false,"enforces_schema":true,"created_at":"2026-09-30T17:36:58Z"},{"id":"8ea15166-a477-4083-b079-974c73236844","kind":"openai_compatible","name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","has_key":true,"enforces_schema":false,"created_at":"2026-09-30T16:46:01Z"}]}"#
    let providers = try HubJSON.makeDecoder().decode(ModelProvidersResponse.self, from: Data(providersJSON.utf8)).providers
    #expect(providers.count == 2 && providers[0].isHubRuntime && !providers[1].isHubRuntime)
    #expect(providers[1].baseURL == "https://openrouter.ai/api/v1" && providers[1].hasKey && !providers[1].enforcesSchema)

    let routesJSON = #"{"routes":[{"kind":"job_facts","provider_id":"2e416d12-77c2-4239-b419-b9fdd58c4cd6","model":"Qwen3.8-27B-Q4_K_M.gguf","fallback_provider_id":"8ea15166-a477-4083-b079-974c73236844","fallback_model":"qwen/qwen3.6","updated_at":"2026-09-30T17:36:58Z"}]}"#
    let route = try #require(try HubJSON.makeDecoder().decode(TaskRoutesResponse.self, from: Data(routesJSON.utf8)).routes.first)
    #expect(route.providerID == providers[0].id && route.fallbackProviderID == providers[1].id && route.fallbackModel == "qwen/qwen3.6")
}

@Test func aProviderInputKeepsTheSavedKeyUnlessOneIsTyped() throws {
    let keepKey = ModelProviderInput(kind: "openai_compatible", name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", apiKey: nil, enforcesSchema: true)
    let encoded = try #require(String(data: try HubJSON.makeEncoder().encode(keepKey), encoding: .utf8))
    #expect(encoded.contains(#""base_url":"https:\/\/openrouter.ai\/api\/v1""#) && !encoded.contains("api_key"))

    var newKey = keepKey
    newKey.apiKey = "sk-new"
    #expect(try #require(String(data: try HubJSON.makeEncoder().encode(newKey), encoding: .utf8)).contains(#""api_key":"sk-new""#))

    let route = TaskRouteInput(providerID: UUID(), model: "m", fallbackProviderID: nil, fallbackModel: nil)
    let encodedRoute = try #require(String(data: try HubJSON.makeEncoder().encode(route), encoding: .utf8))
    #expect(encodedRoute.contains("provider_id") && !encodedRoute.contains("fallback"))
}

@Test func theRoutedKindsAreListedWithTheirTitles() {
    #expect(RoutedTaskKind.allCases.map(\.title) == ["Job facts", "Job briefs", "Recruiter screens", "Market gaps", "Interview packs", "Mail sorting", "LinkedIn conversations"])
}
