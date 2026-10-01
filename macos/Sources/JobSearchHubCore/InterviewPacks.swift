import Foundation

/// A pursued job's interview preparation: the likely questions with the
/// confirmed cases to tell, and honest answers for the role's market gaps.
public struct InterviewPack: Decodable, Equatable, Sendable {
    public var model: String
    public var createdAt: Date
    public var pack: InterviewPackContent
}

public struct InterviewPackContent: Decodable, Equatable, Sendable {
    public var questions: [InterviewQuestion]
    public var roleGaps: [InterviewRoleGap]
}

public struct InterviewQuestion: Decodable, Equatable, Sendable {
    public var question: String
    public var reason: String
    public var stories: [InterviewStory]
    public var talkingPoints: String
}

/// A confirmed knowledge-base entry to tell for a question.
public struct InterviewStory: Decodable, Equatable, Sendable {
    public var entryID: UUID
    public var title: String

    enum CodingKeys: String, CodingKey {
        case title
        case entryID = "entryId"
    }
}

public struct InterviewRoleGap: Decodable, Equatable, Sendable {
    public var gap: String
    public var honestAnswer: String
}

public extension HubClient {
    func getInterviewPack(_ jobID: UUID) async throws -> InterviewPack {
        try await get("v1/jobs/\(jobID.uuidString)/interview-pack", as: InterviewPack.self)
    }
}
