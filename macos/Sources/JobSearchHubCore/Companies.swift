import Foundation

// These mirror the hub's JSON. The decoder converts snake_case keys, so a key
// like website_url arrives as "websiteUrl"; the coding keys map it onto the
// Swift spelling.

public struct Company: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var name: String
    public var domain: String
    public var websiteURL: String?
    public var careersURL: String?
    public var headquartersCountry: String?
    public var employeeCountRange: String?
    public var summary: String?
    public var foundVia: String?
    public var createdAt: Date
    public var updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, name, domain, headquartersCountry, employeeCountRange, summary, foundVia, createdAt, updatedAt
        case websiteURL = "websiteUrl"
        case careersURL = "careersUrl"
    }
}

public struct JobBoard: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var companyID: UUID
    public var provider: String
    public var boardToken: String
    public var boardURL: String?
    public var verifiedAt: Date?
    public var openPostingCount: Int?

    enum CodingKeys: String, CodingKey {
        case id, provider, boardToken, verifiedAt, openPostingCount
        case companyID = "companyId"
        case boardURL = "boardUrl"
    }

    /// "greenhouse/stripe · 12 open", "… · open postings unknown" or "… · unverified".
    public var summaryLine: String {
        let board = "\(provider)/\(boardToken)"
        switch (verifiedAt, openPostingCount) {
        case (nil, _): return "\(board) · unverified"
        case (_, nil): return "\(board) · open postings unknown"
        case (_, let count?): return "\(board) · \(count) open"
        }
    }
}

public struct Person: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var companyID: UUID
    public var name: String
    public var roleTitle: String?
    public var relevance: String
    public var profileURL: String?
    public var sourceURL: String
    public var notes: String?
    public var email: String?
    public var createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, name, roleTitle, relevance, notes, email, createdAt
        case companyID = "companyId"
        case profileURL = "profileUrl"
        case sourceURL = "sourceUrl"
    }
}

/// One row of the companies list.
public struct CompanySummary: Codable, Equatable, Identifiable, Sendable {
    public var company: Company
    public var watchedSince: Date?
    public var jobBoards: [JobBoard]
    public var peopleCount: Int
    /// How many of the owner's connections work here.
    public var connectionCount: Int
    /// Updates about the company or its jobs the owner hasn't seen yet.
    public var unseenUpdates: Int

    public var id: UUID { company.id }
}

/// Everything the hub knows about one company.
public struct CompanyDossier: Codable, Equatable, Sendable {
    public var company: Company
    public var watchedSince: Date?
    public var jobBoards: [JobBoard]
    public var people: [Person]
    /// The owner's connections who work here.
    public var connections: [Connection]?
    /// People the owner knows who don't work here but can open doors.
    public var warmPaths: [WarmPath]?
}

/// Someone the owner knows who can open doors at a company without working
/// there, and how.
public struct WarmPath: Codable, Equatable, Identifiable, Sendable {
    public var contactID: UUID
    public var name: String
    public var howKnown: String?
    public var preferredChannel: String?
    public var note: String?

    public var id: UUID { contactID }

    enum CodingKeys: String, CodingKey {
        case name, howKnown, preferredChannel, note
        case contactID = "contactId"
    }
}

public struct AddWarmPathRequest: Encodable, Sendable {
    public var name: String
    public var howKnown: String
    public var preferredChannel: String
    public var note: String

    public init(name: String, howKnown: String, preferredChannel: String, note: String) {
        self.name = name
        self.howKnown = howKnown
        self.preferredChannel = preferredChannel
        self.note = note
    }
}

public struct CompaniesResponse: Codable, Equatable, Sendable {
    public var companies: [CompanySummary]
}
