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
    public var pay: Pay?
    public var employmentType: String?
    public var department: String?
    public var otherLocations: [String]?
    public var publishedAt: Date?
    public var firstSeenAt: Date
    public var lastSeenAt: Date
    public var closedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id, source, title, location, workplaceType, url, description, pay, employmentType, department, otherLocations, publishedAt
        case firstSeenAt, lastSeenAt, closedAt
        case companyID = "companyId"
        case jobBoardID = "jobBoardId"
    }
}

/// The pay a posting publishes: one range per region or tier, and the
/// board's own summary when it gives one, such as "$230K • Offers Equity".
public struct Pay: Codable, Equatable, Sendable {
    public var ranges: [PayRange]
    public var summary: String?
}

public struct PayRange: Codable, Equatable, Sendable {
    public var label: String?
    public var min: Double
    public var max: Double
    public var currency: String
    /// year, month, week, day or hour; nil when the board does not say.
    public var interval: String?

    /// The range as people read it: "$152,000 – $190,000 a year", or one
    /// amount when both ends are equal.
    public func format(locale: Locale = .current) -> String {
        let style = FloatingPointFormatStyle<Double>.Currency(code: currency, locale: locale).precision(.fractionLength(0...2))
        let amounts = min == max ? min.formatted(style) : "\(min.formatted(style)) – \(max.formatted(style))"
        guard let interval, let period = Self.periods[interval] else { return amounts }
        return "\(amounts) \(period)"
    }

    private static let periods = ["year": "a year", "month": "a month", "week": "a week", "day": "a day", "hour": "an hour"]
}

/// One row of the jobs list: a job, its company's name, and its fit.
public struct JobListItem: Codable, Equatable, Identifiable, Sendable {
    public var job: Job
    public var companyName: String?
    public var fit: JobFit

    public var id: UUID { job.id }

    /// Whether the job was first seen after `lastVisit`; with no earlier
    /// visit, nothing is new.
    public func isNew(since lastVisit: Date?) -> Bool {
        guard let lastVisit else { return false }
        return job.firstSeenAt > lastVisit
    }
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
