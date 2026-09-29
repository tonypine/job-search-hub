import Foundation

/// A column the Jobs table can sort by.
public enum JobsSortColumn: Codable, Hashable, Sendable {
    case fit, title, company, location, firstSeen
    case pay, workplace, employment, department, published, source
    /// A fit check, by its name.
    case fitCheck(String)
    /// A fact read from postings, by its key.
    case fact(String)
}

/// Sorts jobs by one column. Jobs without a value go last in either order,
/// and ties keep the newest first.
public struct JobsSortComparator: SortComparator, Codable, Hashable, Sendable {
    public var column: JobsSortColumn
    public var order: SortOrder

    public init(_ column: JobsSortColumn, order: SortOrder = .forward) {
        self.column = column
        self.order = order
    }

    public func compare(_ left: JobListItem, _ right: JobListItem) -> ComparisonResult {
        switch (Self.getSortValue(of: left, by: column), Self.getSortValue(of: right, by: column)) {
        case let (leftValue?, rightValue?):
            let result = leftValue.compare(to: rightValue)
            if result != .orderedSame {
                return order == .forward ? result : Self.reverse(result)
            }
        case (nil, .some):
            return .orderedDescending
        case (.some, nil):
            return .orderedAscending
        case (nil, nil):
            break
        }
        return Self.compareNewestFirst(left, right)
    }

    private enum SortValue {
        case text(String)
        case date(Date)
        case rank(Int)
        /// Pay sorts within its currency, since there's no rate to compare across them.
        case pay(currency: String, yearlyMaximum: Double)

        func compare(to other: SortValue) -> ComparisonResult {
            switch (self, other) {
            case let (.text(left), .text(right)):
                left.localizedStandardCompare(right)
            case let (.date(left), .date(right)):
                JobsSortComparator.compare(left, right)
            case let (.rank(left), .rank(right)):
                JobsSortComparator.compare(left, right)
            case let (.pay(leftCurrency, leftAmount), .pay(rightCurrency, rightAmount)):
                leftCurrency == rightCurrency ? JobsSortComparator.compare(leftAmount, rightAmount) : leftCurrency.compare(rightCurrency)
            default:
                .orderedSame
            }
        }
    }

    private static func getSortValue(of item: JobListItem, by column: JobsSortColumn) -> SortValue? {
        let job = item.job
        switch column {
        case .fit: return .rank(item.fit.level.rank)
        case .title: return .text(job.title)
        case .company: return getTextValue(item.companyName)
        case .location: return getTextValue(job.location)
        case .firstSeen: return .date(job.firstSeenAt)
        case .pay: return getPayValue(job.pay)
        case .workplace: return getTextValue(job.workplaceType)
        case .employment: return getTextValue(job.employmentType)
        case .department: return getTextValue(job.department)
        case .published: return job.publishedAt.map(SortValue.date)
        case .source: return .text(job.sourceName)
        case let .fitCheck(name): return item.getFitCheck(name).map { .rank(getVerdictRank($0.verdict)) }
        case let .fact(key): return getTextValue(item.getFactText(key))
        }
    }

    private static func getTextValue(_ text: String?) -> SortValue? {
        guard let text, !text.trimmingCharacters(in: .whitespaces).isEmpty else { return nil }
        return .text(text)
    }

    /// A check passed sorts first, then unclear, then failed.
    private static func getVerdictRank(_ verdict: FitVerdict) -> Int {
        switch verdict {
        case .yes: 0
        case .unclear: 1
        case .no: 2
        }
    }

    private static let hoursWorkedInAYear = 2080.0
    private static let workingDaysInAYear = 260.0
    private static let weeksInAYear = 52.0
    private static let monthsInAYear = 12.0

    /// The pay's highest range, as a yearly amount in its currency.
    private static func getPayValue(_ pay: Pay?) -> SortValue? {
        guard let range = pay?.ranges.max(by: { $0.max < $1.max }) else { return nil }
        let yearlyMaximum = switch range.interval {
        case "hour": range.max * hoursWorkedInAYear
        case "day": range.max * workingDaysInAYear
        case "week": range.max * weeksInAYear
        case "month": range.max * monthsInAYear
        default: range.max
        }
        return .pay(currency: range.currency, yearlyMaximum: yearlyMaximum)
    }

    private static func compareNewestFirst(_ left: JobListItem, _ right: JobListItem) -> ComparisonResult {
        if left.job.firstSeenAt != right.job.firstSeenAt {
            return compare(right.job.firstSeenAt, left.job.firstSeenAt)
        }
        return left.job.title.localizedStandardCompare(right.job.title)
    }

    private static func compare<Value: Comparable>(_ left: Value, _ right: Value) -> ComparisonResult {
        left < right ? .orderedAscending : left > right ? .orderedDescending : .orderedSame
    }

    private static func reverse(_ result: ComparisonResult) -> ComparisonResult {
        switch result {
        case .orderedAscending: .orderedDescending
        case .orderedDescending: .orderedAscending
        case .orderedSame: .orderedSame
        }
    }
}
