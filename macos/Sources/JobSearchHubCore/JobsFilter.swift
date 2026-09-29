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
