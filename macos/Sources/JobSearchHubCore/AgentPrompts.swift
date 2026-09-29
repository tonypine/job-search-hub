import Foundation

/// One agent's prompt as the Prompts page lists it: what it is for, which
/// placeholders the hub fills, and its active version.
public struct AgentPromptSummary: Decodable, Equatable, Identifiable, Sendable {
    public var kind: String
    public var title: String
    public var description: String
    public var placeholders: [String]
    /// The active version; nil until the prompt has one.
    public var version: Int?
    public var note: String?
    public var updatedAt: Date?

    public var id: String { kind }
}

public struct AgentPromptsResponse: Decodable, Sendable {
    public var prompts: [AgentPromptSummary]
}

public struct AgentPromptVersionsResponse: Decodable, Sendable {
    public var versions: [AgentPrompt]
}

public struct SaveAgentPromptRequest: Encodable, Sendable {
    public var body: String
    public var note: String

    public init(body: String, note: String) {
        self.body = body
        self.note = note
    }
}
