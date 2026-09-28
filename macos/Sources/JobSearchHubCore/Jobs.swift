import Foundation

public struct Job: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var companyID: UUID?
    public var jobBoardID: UUID?
    public var source: String
    public var title: String
    public var location: String?
    public var workplaceType: String?
    public var url: String
    public var description: String?
    public var firstSeenAt: Date
    public var lastSeenAt: Date
    public var closedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id, source, title, location, workplaceType, url, description, firstSeenAt, lastSeenAt, closedAt
        case companyID = "companyId"
        case jobBoardID = "jobBoardId"
    }
}

/// One row of the jobs list: a job and its company's name.
public struct JobListItem: Codable, Equatable, Identifiable, Sendable {
    public var job: Job
    public var companyName: String?

    public var id: UUID { job.id }
}

public struct JobsResponse: Codable, Equatable, Sendable {
    public var jobs: [JobListItem]
    public var total: Int
}

public struct AddJobRequest: Codable, Equatable, Sendable {
    public var url: String
    public var title: String

    public init(url: String, title: String) {
        self.url = url
        self.title = title
    }
}

public struct AddJobResponse: Codable, Equatable, Sendable {
    public var job: Job
    public var created: Bool
}

public enum JobStatusFilter: String, CaseIterable, Identifiable, Sendable {
    case open, closed, all

    public var id: String { rawValue }
    public var title: String { rawValue.capitalized }
}

public enum JobsQuery {
    /// The query of the jobs list: its status, a page size and the search text when there is one.
    public static func makeItems(search: String, status: JobStatusFilter, limit: Int) -> [URLQueryItem] {
        var items = [URLQueryItem(name: "status", value: status.rawValue), URLQueryItem(name: "limit", value: String(limit))]
        let trimmed = search.trimmingCharacters(in: .whitespaces)
        if !trimmed.isEmpty {
            items.append(URLQueryItem(name: "query", value: trimmed))
        }
        return items
    }
}
