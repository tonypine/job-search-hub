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
    /// The closest of those connections, by name, at most three; nil from a
    /// hub that doesn't send them.
    public var knownPeople: [String]?
    /// The phase of the owner's open application here updated last.
    public var applicationPhase: String?
    /// How many of its open jobs pass the screen.
    public var fittingJobs: Int?
    /// The best match the briefs of those jobs found.
    public var bestMatch: JobMatch?
    /// When the hub first saw the newest of those jobs.
    public var newestFittingJobSeenAt: Date?
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
    /// The company's cards on the board. Only the owner's company page gets
    /// them, with the mail below; nil from a hub that doesn't send them.
    public var applications: [CompanyApplication]?
    /// The company's latest matched mail, newest first, without newsletters
    /// and job alerts.
    public var mail: [MailMessage]?
    /// How many newsletters and job alerts the hub left out of `mail`.
    public var foldedMailCount: Int?

    /// Says how many newsletters and job alerts were left out of the latest
    /// mail; nil when none were.
    public var foldedMailLine: String? {
        switch foldedMailCount ?? 0 {
        case 0: nil
        case 1: "1 newsletter or job alert left out"
        case let count: "\(count) newsletters and job alerts left out"
        }
    }

    /// The applications still open, then the closed ones.
    public var applicationsOpenFirst: [CompanyApplication] {
        let applications = applications ?? []
        return applications.filter { !$0.phaseIsClosed } + applications.filter(\.phaseIsClosed)
    }
}

/// One of a company's cards on the board, with the phase it sits in. The hub
/// sends the card's fields and the phase's side by side.
public struct CompanyApplication: Codable, Equatable, Identifiable, Sendable {
    public var card: PipelineCard
    public var phaseName: String
    public var phaseIsClosed: Bool

    public var id: UUID { card.id }

    enum CodingKeys: String, CodingKey {
        case phaseName, phaseIsClosed
    }

    public init(from decoder: Decoder) throws {
        card = try PipelineCard(from: decoder)
        let container = try decoder.container(keyedBy: CodingKeys.self)
        phaseName = try container.decode(String.self, forKey: .phaseName)
        phaseIsClosed = try container.decode(Bool.self, forKey: .phaseIsClosed)
    }

    public func encode(to encoder: Encoder) throws {
        try card.encode(to: encoder)
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(phaseName, forKey: .phaseName)
        try container.encode(phaseIsClosed, forKey: .phaseIsClosed)
    }
}

/// One message of the owner's mail, matched to a company.
public struct MailMessage: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var threadID: String
    /// "received" or "sent".
    public var direction: String
    public var sender: String
    public var subject: String
    public var sentAt: Date
    /// What kind of mail it is, such as "interview_invite"; nil until read.
    public var classification: String?

    enum CodingKeys: String, CodingKey {
        case id, direction, sender, subject, sentAt, classification
        case threadID = "threadId"
    }

    /// Who wrote it and what kind of mail it is: "You wrote" or the sender,
    /// then e.g. "interview invite".
    public var fromLine: String {
        let from = direction == "sent" ? "You wrote" : sender
        guard let classification, !classification.isEmpty else { return from }
        return "\(from) · \(classification.replacingOccurrences(of: "_", with: " "))"
    }
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
