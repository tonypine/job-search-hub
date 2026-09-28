import Foundation

/// How many LinkedIn connections the hub keeps, how many work at a company in
/// the hub, and when they were last imported.
public struct ConnectionsSummary: Decodable, Equatable, Sendable {
    public var count: Int
    public var matched: Int
    public var lastImportedAt: Date?
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

    public var fullName: String { [firstName, lastName].filter { !$0.isEmpty }.joined(separator: " ") }

    /// "Connected since Sep 2026", read in UTC so the day never shifts.
    public var connectedSince: String? {
        guard let connectedOn else { return nil }
        var style = Date.FormatStyle.dateTime.month(.abbreviated).year()
        style.timeZone = .gmt
        return "Connected since \(connectedOn.formatted(style))"
    }

    enum CodingKeys: String, CodingKey {
        case id, firstName, lastName, email, companyName, position, connectedOn
        case profileURL = "profileUrl"
    }
}

