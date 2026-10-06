import Foundation

/// Which jobs the Jobs table shows. Each filter hides values rather than
/// picking them, so a source or workplace that first appears later shows.
public struct JobsFilter: Codable, Equatable, Sendable {
    public var hiddenFitLevels: Set<FitLevel> = []
    /// Fit checks, by name, whose failures are hidden: a job judged no on one is hidden.
    public var hiddenCheckFailures: Set<String> = []
    /// Sources, by the job's `source` key.
    public var hiddenSources: Set<String> = []
    public var hiddenWorkplaces: Set<String> = []
    public var hiddenEmploymentTypes: Set<String> = []
    public var showsOnlyJobsWithPay = false
    public var showsOnlyNewJobs = false

    public init() {}

    public var isActive: Bool { self != JobsFilter() }

    public func getMatchingItems(_ items: [JobListItem], previousVisit: Date?) -> [JobListItem] {
        items.filter { matches($0, previousVisit: previousVisit) }
    }

    public func matches(_ item: JobListItem, previousVisit: Date?) -> Bool {
        let job = item.job
        if hiddenFitLevels.contains(item.fit.level) { return false }
        if item.fit.checks.contains(where: { $0.verdict == .no && hiddenCheckFailures.contains($0.name) }) { return false }
        if hiddenSources.contains(job.source) { return false }
        if hiddenWorkplaces.contains(job.workplaceName) { return false }
        if hiddenEmploymentTypes.contains(job.employmentName) { return false }
        if showsOnlyJobsWithPay && job.pay == nil { return false }
        if showsOnlyNewJobs && !item.isNew(since: previousVisit) { return false }
        return true
    }
}

extension Job {
    /// What a job leaves blank, as a filter names it.
    public static let unstatedName = "Not stated"

    /// The workplace as one name, whatever the board's spelling: "On-site" and "OnSite" are one.
    public var workplaceName: String {
        guard let workplaceType, !workplaceType.trimmingCharacters(in: .whitespaces).isEmpty else { return Self.unstatedName }
        switch workplaceType.lowercased().filter(\.isLetter) {
        case "remote": return "Remote"
        case "hybrid": return "Hybrid"
        case "onsite": return "On-site"
        default: return workplaceType
        }
    }

    public var employmentName: String {
        guard let employmentType, !employmentType.trimmingCharacters(in: .whitespaces).isEmpty else { return Self.unstatedName }
        return employmentType
    }
}

/// A value a filter can hide, with how many jobs have it.
public struct JobsFilterChoice: Identifiable, Equatable, Sendable {
    /// What the filter stores.
    public var value: String
    public var title: String
    public var count: Int

    public var id: String { value }

    public init(value: String, title: String, count: Int) {
        self.value = value
        self.title = title
        self.count = count
    }
}

/// What the loaded jobs offer to filter on, with counts, for the filter popover.
public struct JobsFilterChoices: Equatable, Sendable {
    public var fitLevelCounts: [FitLevel: Int]
    /// How many jobs fail each fit check, by the check's name.
    public var checkFailureCounts: [String: Int]
    public var sources: [JobsFilterChoice]
    public var workplaces: [JobsFilterChoice]
    public var employmentTypes: [JobsFilterChoice]
    public var withPayCount: Int

    public init(items: [JobListItem]) {
        fitLevelCounts = items.reduce(into: [:]) { counts, item in counts[item.fit.level, default: 0] += 1 }
        checkFailureCounts = items.reduce(into: [:]) { counts, item in
            for check in item.fit.checks where check.verdict == .no { counts[check.name, default: 0] += 1 }
        }
        sources = Self.makeChoices(items.map { ($0.job.source, $0.job.sourceName) })
        workplaces = Self.makeChoices(items.map { ($0.job.workplaceName, $0.job.workplaceName) })
        employmentTypes = Self.makeChoices(items.map { ($0.job.employmentName, $0.job.employmentName) })
        withPayCount = items.count { $0.job.pay != nil }
    }

    /// Choices with the most jobs first, and the unstated ones last.
    private static func makeChoices(_ valuesAndTitles: [(value: String, title: String)]) -> [JobsFilterChoice] {
        let grouped = Dictionary(grouping: valuesAndTitles, by: \.value)
        let choices = grouped.map { value, entries in JobsFilterChoice(value: value, title: entries[0].title, count: entries.count) }
        return choices.sorted { left, right in
            if (left.value == Job.unstatedName) != (right.value == Job.unstatedName) { return right.value == Job.unstatedName }
            if left.count != right.count { return left.count > right.count }
            return left.title.localizedStandardCompare(right.title) == .orderedAscending
        }
    }
}

/// A filter that is on, as the page header's chip says it. Removing a chip
/// turns its whole filter off.
public struct JobsFilterChip: Identifiable, Equatable, Sendable {
    public enum Kind: Hashable, Sendable {
        case screen
        /// Hides the jobs that fail the check of that name.
        case checkFailure(String)
        case sources
        case workplaces
        case employmentTypes
        case withPay
        case new
    }

    public var kind: Kind
    public var title: String

    public var id: Kind { kind }
}

extension JobsFilter {
    /// The screen levels that keep only the jobs that pass.
    public static let passesScreenHiddenLevels: Set<FitLevel> = [.unclear, .poor]

    /// The filters that are on, in the order the menu lists them. A filter of
    /// values names what it keeps when that's one value, else what it hides.
    public func getChips(choices: JobsFilterChoices, getCheckTitle: (String) -> String) -> [JobsFilterChip] {
        var chips: [JobsFilterChip] = []
        if !hiddenFitLevels.isEmpty {
            let shown = [FitLevel.good, .unclear, .poor].filter { !hiddenFitLevels.contains($0) }
            let title = switch shown.count {
            case 0: "Screen: none"
            case 1: shown[0].label
            default: "Screen: " + shown.map(\.title).joined(separator: ", ")
            }
            chips.append(JobsFilterChip(kind: .screen, title: title))
        }
        for name in hiddenCheckFailures.sorted() {
            chips.append(JobsFilterChip(kind: .checkFailure(name), title: "Doesn't fail \(getCheckTitle(name))"))
        }
        let valueFilters: [(JobsFilterChip.Kind, Set<String>, [JobsFilterChoice], (String) -> String)] = [
            (.sources, hiddenSources, choices.sources, Job.getSourceName),
            (.workplaces, hiddenWorkplaces, choices.workplaces, { $0 }),
            (.employmentTypes, hiddenEmploymentTypes, choices.employmentTypes, { $0 }),
        ]
        for (kind, hidden, values, getTitle) in valueFilters where !hidden.isEmpty {
            chips.append(JobsFilterChip(kind: kind, title: Self.describeHidden(hidden, among: values, getTitle: getTitle)))
        }
        if showsOnlyJobsWithPay {
            chips.append(JobsFilterChip(kind: .withPay, title: "Lists pay"))
        }
        if showsOnlyNewJobs {
            chips.append(JobsFilterChip(kind: .new, title: "New since last visit"))
        }
        return chips
    }

    /// The filter without the chip's part.
    public func removing(_ kind: JobsFilterChip.Kind) -> JobsFilter {
        var filter = self
        switch kind {
        case .screen: filter.hiddenFitLevels = []
        case let .checkFailure(name): filter.hiddenCheckFailures.remove(name)
        case .sources: filter.hiddenSources = []
        case .workplaces: filter.hiddenWorkplaces = []
        case .employmentTypes: filter.hiddenEmploymentTypes = []
        case .withPay: filter.showsOnlyJobsWithPay = false
        case .new: filter.showsOnlyNewJobs = false
        }
        return filter
    }

    /// "Remote" when one value of the loaded jobs is left, else "Not Indeed
    /// alert" or "Not Contract and 2 more".
    private static func describeHidden(_ hidden: Set<String>, among values: [JobsFilterChoice], getTitle: (String) -> String) -> String {
        let shown = values.filter { !hidden.contains($0.value) }
        if shown.count == 1 { return shown[0].title }
        let titlesByValue = Dictionary(values.map { ($0.value, $0.title) }, uniquingKeysWith: { first, _ in first })
        let hiddenTitles = hidden.map { titlesByValue[$0] ?? getTitle($0) }.sorted { $0.localizedStandardCompare($1) == .orderedAscending }
        return hiddenTitles.count == 1 ? "Not \(hiddenTitles[0])" : "Not \(hiddenTitles[0]) and \(hiddenTitles.count - 1) more"
    }
}
