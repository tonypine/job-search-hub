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
