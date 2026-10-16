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
    public var workableTimezoneTerms: [String]
    public var unworkableTimezoneTerms: [String]
    public var takeHome: TakeHome?
    public var refuseHourlyWork: Bool

    enum CodingKeys: String, CodingKey {
        case roles, excludedRoleTerms, searchTerms, technologies, seniorityLevels, homeCountry, eligibleLocationTerms, ineligibleLocationTerms, takeHome
        case workableTimezoneTerms, unworkableTimezoneTerms, refuseHourlyWork
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
        workableTimezoneTerms = try container.decodeIfPresent([String].self, forKey: .workableTimezoneTerms) ?? []
        unworkableTimezoneTerms = try container.decodeIfPresent([String].self, forKey: .unworkableTimezoneTerms) ?? []
        takeHome = try container.decodeIfPresent(TakeHome.self, forKey: .takeHome)
        refuseHourlyWork = try container.decodeIfPresent(Bool.self, forKey: .refuseHourlyWork) ?? false
    }

    public init(
        roles: [String] = [], excludedRoleTerms: [String] = [], searchTerms: [String] = [], technologies: [String] = [], seniorityLevels: [String] = [], homeCountry: String = "",
        eligibleLocationTerms: [String] = [], ineligibleLocationTerms: [String] = [], workableTimezoneTerms: [String] = [],
        unworkableTimezoneTerms: [String] = [], takeHome: TakeHome? = nil, refuseHourlyWork: Bool = false
    ) {
        self.roles = roles
        self.excludedRoleTerms = excludedRoleTerms
        self.searchTerms = searchTerms
        self.technologies = technologies
        self.seniorityLevels = seniorityLevels
        self.homeCountry = homeCountry
        self.eligibleLocationTerms = eligibleLocationTerms
        self.ineligibleLocationTerms = ineligibleLocationTerms
        self.workableTimezoneTerms = workableTimezoneTerms
        self.unworkableTimezoneTerms = unworkableTimezoneTerms
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

/// One of the criteria's lists, as the Criteria page groups them: what
/// the owner looks for, where they can work, and what rules a job out.
public enum JobCriteriaList: String, CaseIterable, Sendable {
    case roles, seniorityLevels, technologies, searchTerms
    case eligibleLocationTerms, ineligibleLocationTerms, workableTimezoneTerms, unworkableTimezoneTerms
    case excludedRoleTerms

    /// Its tokens' tone: positive where a value lets a job in, negative
    /// where it rules one out, and neutral where it only finds jobs.
    public var tone: Tone {
        switch self {
        case .eligibleLocationTerms, .workableTimezoneTerms, .technologies: .positive
        case .ineligibleLocationTerms, .unworkableTimezoneTerms, .excludedRoleTerms: .negative
        case .roles, .seniorityLevels, .searchTerms: .neutral
        }
    }

    var draftKeyPath: WritableKeyPath<JobCriteriaDraft, [String]> {
        switch self {
        case .roles: \.roles
        case .seniorityLevels: \.seniorityLevels
        case .technologies: \.technologies
        case .searchTerms: \.searchTerms
        case .eligibleLocationTerms: \.eligibleLocationTerms
        case .ineligibleLocationTerms: \.ineligibleLocationTerms
        case .workableTimezoneTerms: \.workableTimezoneTerms
        case .unworkableTimezoneTerms: \.unworkableTimezoneTerms
        case .excludedRoleTerms: \.excludedRoleTerms
        }
    }

    var criteriaKeyPath: KeyPath<JobCriteria, [String]> {
        switch self {
        case .roles: \.roles
        case .seniorityLevels: \.seniorityLevels
        case .technologies: \.technologies
        case .searchTerms: \.searchTerms
        case .eligibleLocationTerms: \.eligibleLocationTerms
        case .ineligibleLocationTerms: \.ineligibleLocationTerms
        case .workableTimezoneTerms: \.workableTimezoneTerms
        case .unworkableTimezoneTerms: \.unworkableTimezoneTerms
        case .excludedRoleTerms: \.excludedRoleTerms
        }
    }
}

/// The criteria as the Criteria page edits them: each list as its tokens,
/// with the words typed after them that aren't a token yet, and the
/// take-home kept while its check is off.
public struct JobCriteriaDraft: Equatable, Sendable {
    public var roles: [String]
    public var excludedRoleTerms: [String]
    public var searchTerms: [String]
    public var technologies: [String]
    public var seniorityLevels: [String]
    public var homeCountry: String
    public var eligibleLocationTerms: [String]
    public var ineligibleLocationTerms: [String]
    public var workableTimezoneTerms: [String]
    public var unworkableTimezoneTerms: [String]
    public var judgesTakeHome: Bool
    public var takeHome: TakeHome
    public var refuseHourlyWork: Bool
    /// What is typed in a list's field and not yet a token. A save takes it
    /// as tokens, so words typed without Return aren't lost.
    public var unaddedText: [JobCriteriaList: String] = [:]

    public init(_ criteria: JobCriteria) {
        roles = criteria.roles
        excludedRoleTerms = criteria.excludedRoleTerms
        searchTerms = criteria.searchTerms
        technologies = criteria.technologies
        seniorityLevels = criteria.seniorityLevels
        homeCountry = criteria.homeCountry
        eligibleLocationTerms = criteria.eligibleLocationTerms
        ineligibleLocationTerms = criteria.ineligibleLocationTerms
        workableTimezoneTerms = criteria.workableTimezoneTerms
        unworkableTimezoneTerms = criteria.unworkableTimezoneTerms
        judgesTakeHome = criteria.takeHome != nil
        takeHome = criteria.takeHome ?? .empty
        refuseHourlyWork = criteria.refuseHourlyWork
    }

    public subscript(list: JobCriteriaList) -> [String] {
        get { self[keyPath: list.draftKeyPath] }
        set { self[keyPath: list.draftKeyPath] = newValue }
    }

    /// The list's tokens with the words typed after them.
    public func getTokens(_ list: JobCriteriaList) -> [String] {
        Self.addTokens(from: unaddedText[list] ?? "", to: self[list])
    }

    public func makeCriteria() -> JobCriteria {
        JobCriteria(
            roles: getTokens(.roles), excludedRoleTerms: getTokens(.excludedRoleTerms), searchTerms: getTokens(.searchTerms),
            technologies: getTokens(.technologies), seniorityLevels: getTokens(.seniorityLevels),
            homeCountry: homeCountry.trimmingCharacters(in: .whitespaces),
            eligibleLocationTerms: getTokens(.eligibleLocationTerms), ineligibleLocationTerms: getTokens(.ineligibleLocationTerms),
            workableTimezoneTerms: getTokens(.workableTimezoneTerms), unworkableTimezoneTerms: getTokens(.unworkableTimezoneTerms),
            takeHome: judgesTakeHome ? takeHome : nil, refuseHourlyWork: refuseHourlyWork
        )
    }

    /// How many fields differ from the saved criteria: each list, the home
    /// country, hourly work, judging by take-home, and each take-home field.
    public func countChanges(from saved: JobCriteria) -> Int {
        let edited = makeCriteria()
        var changes = JobCriteriaList.allCases.count { edited[keyPath: $0.criteriaKeyPath] != saved[keyPath: $0.criteriaKeyPath] }
        if edited.homeCountry != saved.homeCountry { changes += 1 }
        if edited.refuseHourlyWork != saved.refuseHourlyWork { changes += 1 }
        switch (edited.takeHome, saved.takeHome) {
        case let (editedPay?, savedPay?):
            changes += [
                editedPay.currency != savedPay.currency, editedPay.minimumMonthly != savedPay.minimumMonthly,
                editedPay.targetMonthly != savedPay.targetMonthly, editedPay.clt != savedPay.clt, editedPay.pj != savedPay.pj,
                editedPay.foreignContractor != savedPay.foreignContractor,
            ].count { $0 }
        case (nil, nil):
            break
        default:
            changes += 1
        }
        return changes
    }

    /// The tokens with those in the text added: the text split at commas
    /// and trimmed, leaving out blanks and any already there in any case.
    public static func addTokens(from text: String, to tokens: [String]) -> [String] {
        var added = tokens
        for part in text.split(separator: ",") {
            let token = part.trimmingCharacters(in: .whitespacesAndNewlines)
            if !token.isEmpty, !added.contains(where: { $0.caseInsensitiveCompare(token) == .orderedSame }) {
                added.append(token)
            }
        }
        return added
    }
}

/// The countries the home country is picked from, by their English names,
/// which is how the hub matches them in postings and searches feeds.
public enum HomeCountries {
    /// Every country's English name, in order.
    public static let names: [String] = {
        let english = Locale(identifier: "en_US")
        return Set(
            Locale.Region.isoRegions
                .filter { $0.identifier.count == 2 && $0.identifier.allSatisfy(\.isLetter) }
                .compactMap { english.localizedString(forRegionCode: $0.identifier) }
        )
        .sorted { $0.localizedStandardCompare($1) == .orderedAscending }
    }()

    /// The names to pick from, with the saved one first when it isn't one of
    /// them, so a picker never drops it.
    public static func makeChoices(keeping current: String) -> [String] {
        current.isEmpty || names.contains(current) ? names : [current] + names
    }
}

/// The Criteria page's form. A save shows as saving until the hub answers,
/// and the form then shows what the hub stored; a failed save keeps the edit
/// and says why.
@MainActor
@Observable
public final class JobCriteriaEditor {
    public private(set) var saved: SavedJobCriteria?
    public var draft = JobCriteriaDraft(JobCriteria())
    public private(set) var isSaving = false
    public private(set) var error: ErrorReport?

    public init() {}

    public var hasChanges: Bool {
        guard let saved else { return false }
        return draft.makeCriteria() != saved.criteria
    }

    /// How many fields the draft changed, for the save bar.
    public var changeCount: Int {
        guard let saved else { return 0 }
        return draft.countChanges(from: saved.criteria)
    }

    public func load(with client: HubClient) async {
        do {
            let loaded = try await client.get("v1/job-criteria", as: SavedJobCriteria.self)
            saved = loaded
            draft = JobCriteriaDraft(loaded.criteria)
            self.error = nil
        } catch {
            self.error = ErrorReport(error)
        }
    }

    public func revert() {
        if let saved {
            draft = JobCriteriaDraft(saved.criteria)
        }
        error = nil
    }

    /// Saves the draft; reports whether the hub took it.
    @discardableResult
    public func save(with client: HubClient) async -> Bool {
        isSaving = true
        defer { isSaving = false }
        do {
            let stored = try await client.send("PUT", "v1/job-criteria", body: draft.makeCriteria(), as: SavedJobCriteria.self)
            saved = stored
            draft = JobCriteriaDraft(stored.criteria)
            self.error = nil
            return true
        } catch {
            self.error = ErrorReport(error)
            return false
        }
    }
}
