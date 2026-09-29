import Foundation

/// One piece of the owner's experience in the knowledge base: a role held, a
/// case of work within one, a skill, and so on. Only a confirmed entry speaks
/// for the owner.
public struct ProfileEntry: Codable, Equatable, Identifiable, Sendable {
    public var id: UUID
    public var kind: String
    public var roleID: UUID?
    public var title: String
    public var body: String
    public var organization: String
    public var startMonth: String
    public var endMonth: String
    public var skills: [String]
    public var outcome: String
    public var source: String
    public var sourceDetail: String
    public var confirmedAt: Date?
    public var updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, kind, title, body, organization, startMonth, endMonth, skills, outcome, source, sourceDetail, confirmedAt, updatedAt
        case roleID = "roleId"
    }

    public var isConfirmed: Bool { confirmedAt != nil }
}

public struct ProfileEntriesResponse: Codable, Equatable, Sendable {
    public var entries: [ProfileEntry]
}

/// How the app builds the knowledge base: `hub profile seed`, which ends with
/// "Entries added: <n>, updated: <m>".
public enum ProfileSeedLaunch {
    public static let arguments = ["profile", "seed"]

    public static func parseOutcome(_ output: String) -> (added: Int, updated: Int)? {
        guard let line = output.components(separatedBy: "\n").last(where: { $0.hasPrefix("Entries added: ") }) else { return nil }
        let numbers = line.components(separatedBy: CharacterSet.decimalDigits.inverted).compactMap(Int.init)
        return numbers.count == 2 ? (numbers[0], numbers[1]) : nil
    }
}
