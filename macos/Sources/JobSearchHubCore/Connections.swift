import Foundation

/// How many LinkedIn connections the hub keeps, how many work at a company in
/// the hub, and when they were last imported.
public struct ConnectionsSummary: Decodable, Equatable, Sendable {
    public var count: Int
    public var matched: Int
    public var lastImportedAt: Date?
    public var conversations: Int
    public var invitations: Int
}

/// What one import of Connections.csv did.
public struct ConnectionsImport: Decodable, Equatable, Sendable {
    public var added: Int
    public var updated: Int
    public var matched: Int
    public var skipped: Int

    /// The import in one sentence, for Settings.
    public var summary: String {
        var parts = ["\(added) added", "\(updated) updated"]
        if skipped > 0 { parts.append("\(skipped) skipped without a profile URL") }
        return "Imported: \(parts.joined(separator: ", ")). \(matched) work at companies in the hub."
    }
}

/// Someone in the owner's LinkedIn network: a warm path to their company.
public struct Connection: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var firstName: String
    public var lastName: String
    public var profileURL: String
    public var email: String?
    public var companyName: String?
    public var position: String?
    /// The day they connected, at midnight UTC.
    public var connectedOn: Date?
    /// How close the owner is to them, e.g. "12 messages, last in Mar 2025".
    public var closeness: String?

    public var fullName: String { [firstName, lastName].filter { !$0.isEmpty }.joined(separator: " ") }

    /// "Connected since Sep 2026", read in UTC so the day never shifts.
    public var connectedSince: String? {
        guard let connectedOn else { return nil }
        var style = Date.FormatStyle.dateTime.month(.abbreviated).year()
        style.timeZone = .gmt
        return "Connected since \(connectedOn.formatted(style))"
    }

    enum CodingKeys: String, CodingKey {
        case id, firstName, lastName, email, companyName, position, connectedOn, closeness
        case profileURL = "profileUrl"
    }
}


/// What an import of messages.csv stored.
public struct MessagesImport: Decodable, Equatable, Sendable {
    public var conversations: Int
    public var messages: Int
    public var connectionsWithHistory: Int

    public var summary: String {
        "Conversations: \(conversations) (\(messages) messages); \(connectionsWithHistory) connections you've talked with."
    }
}

/// What an import of Invitations.csv stored.
public struct InvitationsImport: Decodable, Equatable, Sendable {
    public var incoming: Int
    public var outgoing: Int

    public var summary: String { "Invitations: \(incoming) received, \(outgoing) sent." }
}

/// The files of a LinkedIn data export the hub imports, and the route each
/// goes to. A single CSV of another name is taken for Connections.csv.
public enum LinkedInArchive {
    public enum Kind: String, CaseIterable, Sendable {
        // In import order: connections first, so conversations find them.
        case connections = "Connections.csv"
        case messages = "messages.csv"
        case invitations = "Invitations.csv"
        case jobApplications = "Job Applications.csv"
        case savedJobs = "Saved Jobs.csv"
        case endorsementsReceived = "Endorsement_Received_Info.csv"
        case endorsementsGiven = "Endorsement_Given_Info.csv"
        case recommendationsReceived = "Recommendations_Received.csv"
        case recommendationsGiven = "Recommendations_Given.csv"

        /// What the import summary calls a vouching file.
        public var vouchingTitle: String {
            switch self {
            case .endorsementsReceived: "Endorsements received"
            case .endorsementsGiven: "Endorsements given"
            case .recommendationsReceived: "Recommendations received"
            case .recommendationsGiven: "Recommendations given"
            default: rawValue
            }
        }

        public var importPath: String {
            switch self {
            case .connections: "v1/connections/import"
            case .messages: "v1/linkedin/messages/import"
            case .invitations: "v1/linkedin/invitations/import"
            case .jobApplications: "v1/linkedin/applications/import"
            case .savedJobs: "v1/linkedin/saved-jobs/import"
            case .endorsementsReceived: "v1/linkedin/endorsements-received/import"
            case .endorsementsGiven: "v1/linkedin/endorsements-given/import"
            case .recommendationsReceived: "v1/linkedin/recommendations-received/import"
            case .recommendationsGiven: "v1/linkedin/recommendations-given/import"
            }
        }
    }

    /// The files to import out of an export's files, in import order.
    public static func findImports(in files: [URL]) -> [(kind: Kind, file: URL)] {
        Kind.allCases.compactMap { kind in
            files.first { $0.lastPathComponent == kind.rawValue }.map { (kind, $0) }
        }
    }
}

/// What an import of LinkedIn applications or saved jobs put on the pipeline.
public struct LinkedInJobsImport: Decodable, Equatable, Sendable {
    public var active: Int
    public var closed: Int
    public var skipped: Int
    public var alreadyOnBoard: Int

    /// The import in one line, naming what it read.
    public func makeSummary(of what: String) -> String {
        var parts = ["\(active) on the board", "\(closed) kept as history"]
        if skipped > 0 { parts.append("\(skipped) too old to keep") }
        if alreadyOnBoard > 0 { parts.append("\(alreadyOnBoard) already there") }
        return "\(what): \(parts.joined(separator: ", "))."
    }
}

/// What an import of endorsements or recommendations stored.
public struct VouchingImport: Decodable, Equatable, Sendable {
    public var stored: Int
    public var connectionsWithVouches: Int

    public func makeSummary(of what: String) -> String {
        "\(what): \(stored); \(connectionsWithVouches) connections vouch for you or you for them."
    }
}

