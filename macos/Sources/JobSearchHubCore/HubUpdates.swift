import Foundation

/// Something the owner should hear about, from the hub's log of updates.
public struct HubUpdate: Decodable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var sequence: Int64
    public var kind: String
    public var title: String
    public var body: String?
    public var jobID: UUID?
    public var companyID: UUID?
    public var sourceURL: String?
    public var createdAt: Date
    public var seenAt: Date?
    public var jobTitle: String?
    public var companyName: String?

    public var isUnseen: Bool { seenAt == nil }

    /// What the update is about, as one line: the job and its company, or
    /// the company.
    public var subject: String? {
        switch (jobTitle, companyName) {
        case let (job?, company?): "\(job) · \(company)"
        case let (job?, nil): job
        case let (nil, company?): company
        case (nil, nil): nil
        }
    }

    enum CodingKeys: String, CodingKey {
        case id, sequence, kind, title, body, createdAt, seenAt, jobTitle, companyName
        case jobID = "jobId"
        case companyID = "companyId"
        case sourceURL = "sourceUrl"
    }
}

public struct HubUpdateList: Decodable, Equatable, Sendable {
    public var updates: [HubUpdate]
    public var unseenCount: Int
}

/// The updates recorded on one calendar day.
public struct UpdateDay: Equatable, Identifiable, Sendable {
    /// The day's start.
    public var day: Date
    public var updates: [HubUpdate]

    public var id: Date { day }

    /// Groups updates by the day they were recorded, keeping their order.
    public static func makeDays(from updates: [HubUpdate], calendar: Calendar = .current) -> [UpdateDay] {
        var days: [UpdateDay] = []
        for update in updates {
            let day = calendar.startOfDay(for: update.createdAt)
            if days.last?.day == day {
                days[days.count - 1].updates.append(update)
            } else {
                days.append(UpdateDay(day: day, updates: [update]))
            }
        }
        return days
    }
}

/// Which updates to mark seen.
public struct UpdateSelection: Encodable, Sendable {
    public var ids: [UUID]?
    public var jobID: UUID?
    public var companyID: UUID?
    public var all: Bool?

    public init(ids: [UUID]? = nil, jobID: UUID? = nil, companyID: UUID? = nil, all: Bool? = nil) {
        self.ids = ids
        self.jobID = jobID
        self.companyID = companyID
        self.all = all
    }

    enum CodingKeys: String, CodingKey {
        case ids, all
        case jobID = "jobId"
        case companyID = "companyId"
    }
}

public struct MarkSeenResponse: Decodable, Equatable, Sendable {
    public var marked: Int
}

/// One server-sent event.
public struct ServerSentEvent: Equatable, Sendable {
    public var id: String?
    public var name: String
    public var data: String
}

/// Splits a byte stream into lines, keeping the empty ones that end
/// server-sent events; Foundation's `lines` drops them. A trailing carriage
/// return is removed, and a last line with no newline is never finished.
public struct EventStreamLines<Bytes: AsyncSequence>: AsyncSequence where Bytes.Element == UInt8 {
    public typealias Element = String
    private let bytes: Bytes

    public init(_ bytes: Bytes) {
        self.bytes = bytes
    }

    public func makeAsyncIterator() -> AsyncIterator {
        AsyncIterator(bytes: bytes.makeAsyncIterator())
    }

    public struct AsyncIterator: AsyncIteratorProtocol {
        var bytes: Bytes.AsyncIterator
        var line: [UInt8] = []

        public mutating func next() async throws -> String? {
            while let byte = try await bytes.next() {
                guard byte == UInt8(ascii: "\n") else {
                    line.append(byte)
                    continue
                }
                if line.last == UInt8(ascii: "\r") {
                    line.removeLast()
                }
                defer { line.removeAll(keepingCapacity: true) }
                return String(decoding: line, as: UTF8.self)
            }
            return nil
        }
    }
}

/// Reads a server-sent event stream line by line: fields accumulate until a
/// blank line ends the event; comment lines (heartbeats) are skipped.
public struct ServerSentEventParser: Sendable {
    private var id: String?
    private var name: String?
    private var dataLines: [String] = []

    public init() {}

    public mutating func consume(_ line: String) -> ServerSentEvent? {
        if line.isEmpty {
            defer { id = nil; name = nil; dataLines = [] }
            guard !dataLines.isEmpty else { return nil }
            return ServerSentEvent(id: id, name: name ?? "message", data: dataLines.joined(separator: "\n"))
        }
        if line.hasPrefix(":") { return nil }
        let field = line.split(separator: ":", maxSplits: 1).first.map(String.init) ?? line
        var value = line.count > field.count ? String(line.dropFirst(field.count + 1)) : ""
        if value.hasPrefix(" ") { value.removeFirst() }
        switch field {
        case "id": id = value
        case "event": name = value
        case "data": dataLines.append(value)
        default: break
        }
        return nil
    }
}
