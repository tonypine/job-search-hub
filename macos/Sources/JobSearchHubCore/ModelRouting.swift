import Foundation

/// A model server the hub can call: an OpenAI-compatible one at an address,
/// or the hub's own runtime. Its key is never served back; `hasKey` says
/// whether one is set.
public struct ModelProvider: Decodable, Equatable, Identifiable, Hashable, Sendable {
    public static let hubRuntimeKind = "hub_runtime"
    public static let openAICompatibleKind = "openai_compatible"

    public var id: UUID
    public var kind: String
    public var name: String
    public var baseURL: String
    public var hasKey: Bool
    public var enforcesSchema: Bool

    public var isHubRuntime: Bool { kind == Self.hubRuntimeKind }

    enum CodingKeys: String, CodingKey {
        case id, kind, name, hasKey, enforcesSchema
        case baseURL = "baseUrl"
    }
}

/// A provider to add or replace. A nil key keeps the one saved; an empty
/// one removes it.
public struct ModelProviderInput: Encodable, Equatable, Sendable {
    public var kind: String
    public var name: String
    public var baseURL: String
    public var apiKey: String?
    public var enforcesSchema: Bool

    public init(kind: String, name: String, baseURL: String, apiKey: String?, enforcesSchema: Bool) {
        self.kind = kind
        self.name = name
        self.baseURL = baseURL
        self.apiKey = apiKey
        self.enforcesSchema = enforcesSchema
    }

    enum CodingKeys: String, CodingKey {
        case kind, name, apiKey, enforcesSchema
        case baseURL = "baseUrl"
    }
}

public struct ModelProvidersResponse: Decodable, Sendable {
    public var providers: [ModelProvider]
}

/// The provider and model a kind of task runs on, and its fallback.
public struct TaskRoute: Decodable, Equatable, Sendable {
    public var kind: String
    public var providerID: UUID
    public var model: String
    public var fallbackProviderID: UUID?
    public var fallbackModel: String?

    enum CodingKeys: String, CodingKey {
        case kind, model, fallbackModel
        case providerID = "providerId"
        case fallbackProviderID = "fallbackProviderId"
    }
}

public struct TaskRouteInput: Encodable, Equatable, Sendable {
    public var providerID: UUID
    public var model: String
    public var fallbackProviderID: UUID?
    public var fallbackModel: String?

    public init(providerID: UUID, model: String, fallbackProviderID: UUID?, fallbackModel: String?) {
        self.providerID = providerID
        self.model = model
        self.fallbackProviderID = fallbackProviderID
        self.fallbackModel = fallbackModel
    }

    enum CodingKeys: String, CodingKey {
        case model, fallbackModel
        case providerID = "providerId"
        case fallbackProviderID = "fallbackProviderId"
    }
}

public struct TaskRoutesResponse: Decodable, Sendable {
    public var routes: [TaskRoute]
}

public struct ProviderModelsResponse: Decodable, Sendable {
    public var models: [String]
}

/// The kinds of background task that run on a routed model, in the order
/// Settings lists them.
public enum RoutedTaskKind: String, CaseIterable, Identifiable, Sendable {
    case jobFacts = "job_facts"
    case jobBrief = "job_brief"
    case recruiterScreen = "recruiter_screen"
    case marketGaps = "market_gaps"
    case interviewPrep = "interview_prep"
    case mailTriage = "mail_triage"
    case linkedInConversation = "linkedin_conversation"
    case jobFactsSecondReading = "job_facts_second_reading"

    public var id: String { rawValue }
    public var title: String { RunsSummary.getKindTitle(rawValue) }
    /// An optional task runs only once routed, and its route can be removed
    /// to turn it off again.
    public var isOptional: Bool { self == .jobFactsSecondReading }
}
