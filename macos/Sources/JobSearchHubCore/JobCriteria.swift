import Foundation
import Observation

/// What makes a job worth the owner's time, as the hub stores it.
public struct JobCriteria: Codable, Equatable, Sendable {
    public var roles: [String]
    public var excludedRoleTerms: [String]
    public var searchTerms: [String]
    public var technologies: [String]
    public var seniorityLevels: [String]
    public var homeCountry: String
    public var eligibleLocationTerms: [String]
    public var ineligibleLocationTerms: [String]
    public var takeHome: TakeHome?
    public var refuseHourlyWork: Bool

    enum CodingKeys: String, CodingKey {
        case roles, excludedRoleTerms, searchTerms, technologies, seniorityLevels, homeCountry, eligibleLocationTerms, ineligibleLocationTerms, takeHome
        case refuseHourlyWork
    }

    /// The hub sends null for a list it holds none of.
    public init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        roles = try container.decodeIfPresent([String].self, forKey: .roles) ?? []
        excludedRoleTerms = try container.decodeIfPresent([String].self, forKey: .excludedRoleTerms) ?? []
        searchTerms = try container.decodeIfPresent([String].self, forKey: .searchTerms) ?? []
        technologies = try container.decodeIfPresent([String].self, forKey: .technologies) ?? []
        seniorityLevels = try container.decodeIfPresent([String].self, forKey: .seniorityLevels) ?? []
        homeCountry = try container.decodeIfPresent(String.self, forKey: .homeCountry) ?? ""
        eligibleLocationTerms = try container.decodeIfPresent([String].self, forKey: .eligibleLocationTerms) ?? []
        ineligibleLocationTerms = try container.decodeIfPresent([String].self, forKey: .ineligibleLocationTerms) ?? []
        takeHome = try container.decodeIfPresent(TakeHome.self, forKey: .takeHome)
        refuseHourlyWork = try container.decodeIfPresent(Bool.self, forKey: .refuseHourlyWork) ?? false
    }

    public init(
        roles: [String] = [], excludedRoleTerms: [String] = [], searchTerms: [String] = [], technologies: [String] = [], seniorityLevels: [String] = [], homeCountry: String = "",
        eligibleLocationTerms: [String] = [], ineligibleLocationTerms: [String] = [], takeHome: TakeHome? = nil, refuseHourlyWork: Bool = false
    ) {
        self.roles = roles
        self.excludedRoleTerms = excludedRoleTerms
        self.searchTerms = searchTerms
        self.technologies = technologies
        self.seniorityLevels = seniorityLevels
        self.homeCountry = homeCountry
        self.eligibleLocationTerms = eligibleLocationTerms
        self.ineligibleLocationTerms = ineligibleLocationTerms
        self.takeHome = takeHome
        self.refuseHourlyWork = refuseHourlyWork
    }
}

/// The pay the owner needs, as monthly take-home, and the share of a
/// posting's pay each way of being hired would leave.
public struct TakeHome: Codable, Equatable, Sendable {
    public var currency: String
    public var minimumMonthly: Double
    public var targetMonthly: Double
    public var clt: HiringTakeHome
    public var pj: HiringTakeHome
    public var foreignContractor: HiringTakeHome

    public static let empty = TakeHome(
        currency: "BRL", minimumMonthly: 0, targetMonthly: 0,
        clt: HiringTakeHome(share: 0.73, paymentsPerYear: 13.33), pj: HiringTakeHome(share: 0.82, paymentsPerYear: 12),
        foreignContractor: HiringTakeHome(share: 0.84, paymentsPerYear: 12)
    )
}

public struct HiringTakeHome: Codable, Equatable, Sendable {
    public var share: Double
    public var paymentsPerYear: Double
}

public struct SavedJobCriteria: Codable, Equatable, Sendable {
    public var criteria: JobCriteria
    public var updatedAt: Date
}

/// The criteria as the Settings form edits them: lists as comma-separated
/// text, so a comma being typed isn't lost, and the take-home kept while its
/// check is off.
public struct JobCriteriaDraft: Equatable, Sendable {
    public var roles: String
    public var excludedRoleTerms: String
    public var searchTerms: String
    public var technologies: String
    public var seniorityLevels: String
    public var homeCountry: String
    public var eligibleLocationTerms: String
    public var ineligibleLocationTerms: String
    public var judgesTakeHome: Bool
    public var takeHome: TakeHome
    public var refuseHourlyWork: Bool

    public init(_ criteria: JobCriteria) {
        roles = Self.formatList(criteria.roles)
        excludedRoleTerms = Self.formatList(criteria.excludedRoleTerms)
        searchTerms = Self.formatList(criteria.searchTerms)
        technologies = Self.formatList(criteria.technologies)
        seniorityLevels = Self.formatList(criteria.seniorityLevels)
        homeCountry = criteria.homeCountry
        eligibleLocationTerms = Self.formatList(criteria.eligibleLocationTerms)
        ineligibleLocationTerms = Self.formatList(criteria.ineligibleLocationTerms)
        judgesTakeHome = criteria.takeHome != nil
        takeHome = criteria.takeHome ?? .empty
        refuseHourlyWork = criteria.refuseHourlyWork
    }

    public func makeCriteria() -> JobCriteria {
        JobCriteria(
            roles: Self.parseList(roles), excludedRoleTerms: Self.parseList(excludedRoleTerms), searchTerms: Self.parseList(searchTerms), technologies: Self.parseList(technologies),
            seniorityLevels: Self.parseList(seniorityLevels), homeCountry: homeCountry.trimmingCharacters(in: .whitespaces),
            eligibleLocationTerms: Self.parseList(eligibleLocationTerms), ineligibleLocationTerms: Self.parseList(ineligibleLocationTerms),
            takeHome: judgesTakeHome ? takeHome : nil, refuseHourlyWork: refuseHourlyWork
        )
    }

    static func formatList(_ items: [String]) -> String {
        items.joined(separator: ", ")
    }

    static func parseList(_ text: String) -> [String] {
        text.split(separator: ",").map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }
    }
}

/// The Settings criteria form. A save shows as saving until the hub answers,
/// and the form then shows what the hub stored; a failed save keeps the edit
/// and says why.
@MainActor
@Observable
public final class JobCriteriaEditor {
    public private(set) var saved: SavedJobCriteria?
    public var draft = JobCriteriaDraft(JobCriteria())
    public private(set) var isSaving = false
    public private(set) var errorMessage: String?

    public init() {}

    public var hasChanges: Bool {
        guard let saved else { return false }
        return draft.makeCriteria() != saved.criteria
    }

    public func load(with client: HubClient) async {
        do {
            let loaded = try await client.get("v1/job-criteria", as: SavedJobCriteria.self)
            saved = loaded
            draft = JobCriteriaDraft(loaded.criteria)
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }

    public func revert() {
        if let saved {
            draft = JobCriteriaDraft(saved.criteria)
        }
        errorMessage = nil
    }

    public func save(with client: HubClient) async {
        isSaving = true
        defer { isSaving = false }
        do {
            let stored = try await client.send("PUT", "v1/job-criteria", body: draft.makeCriteria(), as: SavedJobCriteria.self)
            saved = stored
            draft = JobCriteriaDraft(stored.criteria)
            errorMessage = nil
        } catch HubError.server(_, let message) {
            errorMessage = message
        } catch {
            errorMessage = String(describing: error)
        }
    }
}
