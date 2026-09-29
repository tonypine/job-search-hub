import Foundation

/// Work the phone asked of the Mac, which is the only place agents run.
public struct TaskRequest: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var companyID: UUID?
    public var input: String?
    public var status: String

    enum CodingKeys: String, CodingKey {
        case id, kind, input, status
        case companyID = "companyId"
    }

    public static let findJobs = "find_jobs"
    public static let researchCompany = "research_company"
}

public struct TasksResponse: Decodable, Sendable {
    public var tasks: [TaskRequest]
}

/// How a task ended: the first line of the result becomes the update's title.
public struct FinishTaskRequest: Encodable, Sendable {
    public var succeeded: Bool
    public var result: String
    public var companyID: UUID?

    public init(succeeded: Bool, result: String, companyID: UUID?) {
        self.succeeded = succeeded
        self.result = result
        self.companyID = companyID
    }

    enum CodingKeys: String, CodingKey {
        case succeeded, result
        case companyID = "companyId"
    }
}
