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

        public var importPath: String {
            switch self {
            case .connections: "v1/connections/import"
            case .messages: "v1/linkedin/messages/import"
            case .invitations: "v1/linkedin/invitations/import"
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
