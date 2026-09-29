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

/// An entry to add or replace, as the hub takes it.
public struct ProfileEntryInput: Codable, Equatable, Sendable {
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

    enum CodingKeys: String, CodingKey {
        case kind, title, body, organization, startMonth, endMonth, skills, outcome, source, sourceDetail
        case roleID = "roleId"
    }

    public init(
        kind: String, roleID: UUID? = nil, title: String, body: String = "", organization: String = "", startMonth: String = "",
        endMonth: String = "", skills: [String] = [], outcome: String = "", source: String = "owner", sourceDetail: String = ""
    ) {
        self.kind = kind
        self.roleID = roleID
        self.title = title
        self.body = body
        self.organization = organization
        self.startMonth = startMonth
        self.endMonth = endMonth
        self.skills = skills
        self.outcome = outcome
        self.source = source
        self.sourceDetail = sourceDetail
    }

    public init(_ entry: ProfileEntry) {
        self.init(
            kind: entry.kind, roleID: entry.roleID, title: entry.title, body: entry.body, organization: entry.organization,
            startMonth: entry.startMonth, endMonth: entry.endMonth, skills: entry.skills, outcome: entry.outcome,
            source: entry.source, sourceDetail: entry.sourceDetail
        )
    }
}

public struct ConfirmProfileEntriesRequest: Encodable, Sendable {
    public var ids: [UUID]

    public init(ids: [UUID]) {
        self.ids = ids
    }
}

/// A role with the entries that happened in it.
public struct ProfileRoleGroup: Identifiable, Equatable, Sendable {
    public var role: ProfileEntry
    public var entries: [ProfileEntry]

    public var id: UUID { role.id }
    public var unconfirmedIDs: [UUID] { ([role] + entries).filter { !$0.isConfirmed }.map(\.id) }
}

/// Entries of one kind that belong to no role.
public struct ProfileKindGroup: Identifiable, Equatable, Sendable {
    public var kind: String
    public var entries: [ProfileEntry]

    public var id: String { kind }
    public var title: String { ProfileEntryGroups.getKindTitle(kind) }
}

/// The knowledge base as the Profile page lays it out: roles with their
/// entries, the current and latest first, then the other entries by kind.
public struct ProfileEntryGroups: Equatable, Sendable {
    public static let kinds = ["role", "case", "skill", "project", "education", "preference", "fact"]

    public var roles: [ProfileRoleGroup]
    public var others: [ProfileKindGroup]

    public init(entries: [ProfileEntry]) {
        let roles = entries.filter { $0.kind == "role" }
        let roleIDs = Set(roles.map(\.id))
        let byRole = Dictionary(grouping: entries.filter { $0.kind != "role" && $0.roleID.map(roleIDs.contains) == true }) { $0.roleID! }
        self.roles = roles.sorted(by: Self.isLater).map { role in
            ProfileRoleGroup(role: role, entries: (byRole[role.id] ?? []).sorted(by: Self.isListedBefore))
        }
        let unlinked = entries.filter { $0.kind != "role" && $0.roleID.map(roleIDs.contains) != true }
        others = Self.kinds.dropFirst().compactMap { kind in
            let ofKind = unlinked.filter { $0.kind == kind }.sorted(by: Self.isListedBefore)
            return ofKind.isEmpty ? nil : ProfileKindGroup(kind: kind, entries: ofKind)
        }
    }

    public static func getKindTitle(_ kind: String) -> String {
        switch kind {
        case "role": "Role"
        case "case": "Cases of work"
        case "skill": "Skills"
        case "project": "Projects"
        case "education": "Education"
        case "preference": "Preferences"
        case "fact": "Facts"
        default: kind.capitalized
        }
    }

    /// A current role (no end month) first, then the latest ended.
    private static func isLater(_ left: ProfileEntry, _ right: ProfileEntry) -> Bool {
        let leftEnd = left.endMonth.isEmpty ? "9999" : left.endMonth
        let rightEnd = right.endMonth.isEmpty ? "9999" : right.endMonth
        if leftEnd != rightEnd { return leftEnd > rightEnd }
        return left.startMonth > right.startMonth
    }

    /// Cases before skills and projects; within a kind, the latest first, then by title.
    private static func isListedBefore(_ left: ProfileEntry, _ right: ProfileEntry) -> Bool {
        let leftRank = kinds.firstIndex(of: left.kind) ?? kinds.count
        let rightRank = kinds.firstIndex(of: right.kind) ?? kinds.count
        if leftRank != rightRank { return leftRank < rightRank }
        if left.startMonth != right.startMonth { return left.startMonth > right.startMonth }
        return left.title.localizedStandardCompare(right.title) == .orderedAscending
    }
}

extension ProfileEntry {
    /// "2022-05 – 2024-01", "2024-01 – now", or "" when no month is known.
    public var monthsText: String {
        switch (startMonth.isEmpty, endMonth.isEmpty) {
        case (true, true): ""
        case (false, true): kind == "role" ? "\(startMonth) – now" : startMonth
        case (true, false): "until \(endMonth)"
        case (false, false): "\(startMonth) – \(endMonth)"
        }
    }
}
