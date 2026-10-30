import Foundation

/// How far back a kind's page counts its runs: the last 24 hours, 7, 30 or
/// 90 days, or a range of days the owner picks.
public enum RunWindowPreset: String, CaseIterable, Codable, Identifiable, Sendable {
    case lastDay = "24h"
    case lastWeek = "7d"
    case lastMonth = "30d"
    case lastQuarter = "90d"
    case custom

    public var id: String { rawValue }

    public var title: String {
        switch self {
        case .lastDay: "24 hours"
        case .lastWeek: "7 days"
        case .lastMonth: "30 days"
        case .lastQuarter: "90 days"
        case .custom: "Custom range"
        }
    }

    /// How long the window is, up to now; nil for a custom range.
    public var length: TimeInterval? {
        switch self {
        case .lastDay: 24 * RunBucketSize.hourLength
        case .lastWeek: 7 * 24 * RunBucketSize.hourLength
        case .lastMonth: 30 * 24 * RunBucketSize.hourLength
        case .lastQuarter: 90 * 24 * RunBucketSize.hourLength
        case .custom: nil
        }
    }
}

/// How a window's runs are split for its charts: by the owner's hours, days
/// or weeks, which start on Monday. The raw values are the hub's.
public enum RunBucketSize: String, CaseIterable, Codable, Identifiable, Sendable {
    case hour, day, week

    public static let hourLength: TimeInterval = 60 * 60
    /// The most buckets the hub splits a window into.
    public static let hubLimit = 5000
    /// The longest window counted by the hour: more bars than that can't be
    /// read.
    public static let longestByHour: TimeInterval = 14 * 24 * hourLength
    /// The shortest window counted by the week: fewer than two weeks is one
    /// or two clipped bars.
    public static let shortestByWeek: TimeInterval = 14 * 24 * hourLength

    public var id: String { rawValue }

    public var title: String {
        switch self {
        case .hour: "By hour"
        case .day: "By day"
        case .week: "By week"
        }
    }

    /// The bucket in a sentence: "hour", "day", "week".
    public var noun: String { rawValue }

    /// About how long a bucket is: a day or a week is an hour shorter or
    /// longer across a daylight-saving change.
    public var length: TimeInterval {
        switch self {
        case .hour: Self.hourLength
        case .day: 24 * Self.hourLength
        case .week: 7 * 24 * Self.hourLength
        }
    }

    /// The whole hour, day or week a moment falls in, in the calendar's
    /// time zone; weeks start on Monday, as the hub's do.
    public func getPeriod(containing date: Date, calendar: Calendar) -> DateInterval? {
        switch self {
        case .hour:
            return calendar.dateInterval(of: .hour, for: date)
        case .day:
            return calendar.dateInterval(of: .day, for: date)
        case .week:
            var mondayFirst = calendar
            mondayFirst.firstWeekday = 2
            return mondayFirst.dateInterval(of: .weekOfYear, for: date)
        }
    }

    /// The buckets a window of this length offers: by hour up to two weeks,
    /// by week from two weeks, by day whenever the hub can split it so.
    public static func getOffered(forLength length: TimeInterval) -> [RunBucketSize] {
        allCases.filter { size in
            guard length / size.length <= Double(hubLimit) else { return false }
            switch size {
            case .hour: return length <= longestByHour
            case .day: return true
            case .week: return length >= shortestByWeek
            }
        }
    }

    /// The bucket a window of this length starts with: by hour up to two
    /// days, by day up to 120 days, by week past that.
    public static func getDefault(forLength length: TimeInterval) -> RunBucketSize {
        if length <= 2 * day.length { return .hour }
        if length <= 120 * day.length { return .day }
        return .week
    }
}

/// The window a page counts its runs over, as the owner chose it. Each
/// kind's page, and All kinds, keeps its own.
public struct RunWindowChoice: Codable, Equatable, Hashable, Sendable {
    public var preset: RunWindowPreset
    public var bucket: RunBucketSize
    /// The custom range's first and last days, both counted.
    public var customFrom: Date?
    public var customThrough: Date?

    public static let standard = RunWindowChoice(preset: .lastDay, bucket: .hour)

    public init(preset: RunWindowPreset, bucket: RunBucketSize, customFrom: Date? = nil, customThrough: Date? = nil) {
        self.preset = preset
        self.bucket = bucket
        self.customFrom = customFrom
        self.customThrough = customThrough
    }

    /// The choice with another preset, and that preset's bucket. A custom
    /// range starts as the window shown before it, by days.
    public func setting(_ preset: RunWindowPreset, now: Date, calendar: Calendar) -> RunWindowChoice {
        var choice = self
        choice.preset = preset
        if preset == .custom, customFrom == nil || customThrough == nil {
            let shown = resolve(now: now, calendar: calendar)
            choice.customFrom = calendar.startOfDay(for: shown.from)
            choice.customThrough = calendar.startOfDay(for: shown.to)
        }
        choice.bucket = RunBucketSize.getDefault(forLength: choice.getRange(now: now, calendar: calendar).duration)
        return choice
    }

    /// The window from and to: the preset's length up to now, or the custom
    /// range's days, from the first's midnight to the midnight after the
    /// last, and never past now.
    public func getRange(now: Date, calendar: Calendar) -> DateInterval {
        if let length = preset.length {
            return DateInterval(start: now.addingTimeInterval(-length), end: now)
        }
        let first = calendar.startOfDay(for: customFrom ?? now)
        let last = calendar.startOfDay(for: max(customThrough ?? now, first))
        let afterLast = calendar.date(byAdding: .day, value: 1, to: last) ?? last.addingTimeInterval(RunBucketSize.day.length)
        let end = min(afterLast, now)
        // A range that starts today, before now, still has a moment in it.
        return DateInterval(start: first, end: max(end, first.addingTimeInterval(60)))
    }

    /// The window as the page reads it now: its range, and the chosen
    /// bucket when the range offers it, else the range's own.
    public func resolve(now: Date, calendar: Calendar) -> ResolvedRunWindow {
        let range = getRange(now: now, calendar: calendar)
        let offered = RunBucketSize.getOffered(forLength: range.duration)
        let size = offered.contains(bucket) ? bucket : RunBucketSize.getDefault(forLength: range.duration)
        return ResolvedRunWindow(from: range.start, to: range.end, bucket: size, readAt: now)
    }

    /// The choices of every page, by kind, "" for All kinds, as the app keeps
    /// them; a text that isn't one reads as none.
    public static func decodeChoices(_ text: String) -> [String: RunWindowChoice] {
        guard !text.isEmpty, let data = text.data(using: .utf8) else { return [:] }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .secondsSince1970
        return (try? decoder.decode([String: RunWindowChoice].self, from: data)) ?? [:]
    }

    public static func encodeChoices(_ choices: [String: RunWindowChoice]) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = .sortedKeys
        encoder.dateEncodingStrategy = .secondsSince1970
        return (try? encoder.encode(choices)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
    }
}

/// A window resolved at one moment: what the metrics and the runs list
/// read. Read again, as on a hub event, it is a new window even when its
/// range is the same, so both read again.
public struct ResolvedRunWindow: Equatable, Hashable, Sendable {
    public var from: Date
    public var to: Date
    public var bucket: RunBucketSize
    public var readAt: Date

    public init(from: Date, to: Date, bucket: RunBucketSize, readAt: Date) {
        self.from = from
        self.to = to
        self.bucket = bucket
        self.readAt = readAt
    }

    public var range: DateInterval { DateInterval(start: from, end: max(from, to)) }

    /// `GET /v1/runs/metrics`'s query for a kind, or every kind without one,
    /// split in the owner's time zone.
    public func makeMetricsQuery(kind: String?, timeZone: TimeZone) -> [URLQueryItem] {
        var query = [
            URLQueryItem(name: "from", value: RunWindowFormat.formatInstant(from)),
            URLQueryItem(name: "to", value: RunWindowFormat.formatInstant(to)),
            URLQueryItem(name: "bucket", value: bucket.rawValue),
            URLQueryItem(name: "time_zone", value: timeZone.identifier),
        ]
        if let kind {
            query.append(URLQueryItem(name: "kind", value: kind))
        }
        return query
    }
}

public enum RunWindowFormat {
    /// An instant as the hub reads it: RFC 3339 in UTC, to the second.
    public static func formatInstant(_ date: Date) -> String {
        date.formatted(.iso8601)
    }

    /// A count short enough for a tile or an axis: "950", "12.3K", "340K",
    /// "1.2M".
    public static func formatCompact(_ value: Double) -> String {
        let magnitude = abs(value)
        switch magnitude {
        case ..<1000: return String(format: "%.0f", value)
        case ..<10000: return String(format: "%.1fK", value / 1000)
        case ..<1_000_000: return String(format: "%.0fK", value / 1000)
        case ..<10_000_000: return String(format: "%.1fM", value / 1_000_000)
        default: return String(format: "%.0fM", value / 1_000_000)
        }
    }

    /// Dollars to the cent, under a cent as "<$0.01".
    public static func formatDollars(_ value: Double) -> String {
        if value > 0 && value < 0.005 { return "<$0.01" }
        return String(format: "$%.2f", value)
    }
}

/// One of the five numbers over a kind's charts: its value, and how it
/// compares with the window just before.
public struct RunMetricTile: Identifiable, Equatable, Sendable {
    public enum Measure: String, CaseIterable, Sendable {
        case runs, failed, tokens, averageTime, cost
    }

    public var measure: Measure
    public var title: String
    public var value: String
    /// A line under the value: the failure rate, the tokens in and out.
    public var detail: String?
    /// "+12% vs before"; nil when it can't compare.
    public var change: String?
    /// The change's tone: a measure where less is better reads positive
    /// down and negative up; the rest stay neutral.
    public var changeTone: Tone
    /// The window before, for the change's help.
    public var help: String?

    public var id: String { measure.rawValue }
}

public enum RunMetricTiles {
    /// The five tiles: runs, failed, tokens, average time and cost. On a kind
    /// on the Claude plan, cost reads "On your Claude plan".
    public static func make(_ metrics: RunMetrics, isOnClaudePlan: Bool) -> [RunMetricTile] {
        let now = metrics.totals
        let before = metrics.previous
        let previousLine = "The window before: \(before.from.formatted(date: .abbreviated, time: .shortened)) to \(before.to.formatted(date: .abbreviated, time: .shortened))"
        func tile(_ measure: RunMetricTile.Measure, _ title: String, _ value: String, detail: String? = nil,
                  current: Double?, previous: Double?, lessIsBetter: Bool = false) -> RunMetricTile {
            let change = current.flatMap { current in previous.map { describeChange(current, from: $0) } }
            var tone = Tone.neutral
            if lessIsBetter, let current, let previous, current != previous {
                tone = current < previous ? .positive : .negative
            }
            return RunMetricTile(
                measure: measure, title: title, value: value, detail: detail, change: change, changeTone: tone,
                help: change == nil ? nil : previousLine
            )
        }

        let failedDetail = now.runs > 0 ? "\(Int((Double(now.failed) / Double(now.runs) * 100).rounded()))% of runs" : nil
        let tokensValue = "\(RunWindowFormat.formatCompact(Double(now.inputTokens))) in · \(RunWindowFormat.formatCompact(Double(now.outputTokens))) out"
        let average = now.averageDurationMS.map { RunFormat.describeDuration(milliseconds: $0) } ?? "—"
        let cost = Double(now.costUSD) ?? 0
        let previousCost = Double(before.costUSD) ?? 0
        let costTile: RunMetricTile
        if isOnClaudePlan {
            costTile = RunMetricTile(
                measure: .cost, title: "Cost", value: "On your Claude plan",
                detail: cost > 0 ? "\(RunWindowFormat.formatDollars(cost)) at API prices" : nil, change: nil, changeTone: .neutral, help: nil
            )
        } else {
            let planDetail = now.claudePlanRuns > 0 ? "\(now.claudePlanRuns) on your Claude plan" : nil
            costTile = tile(.cost, "Cost", RunWindowFormat.formatDollars(cost), detail: planDetail, current: cost, previous: previousCost)
        }
        return [
            tile(.runs, "Runs", now.runs.formatted(), current: Double(now.runs), previous: Double(before.runs)),
            tile(.failed, "Failed", now.failed.formatted(), detail: failedDetail,
                 current: Double(now.failed), previous: Double(before.failed), lessIsBetter: true),
            tile(.tokens, "Tokens", tokensValue, current: Double(now.tokens), previous: Double(before.tokens)),
            tile(.averageTime, "Average time", average,
                 current: now.averageDurationMS.map(Double.init), previous: before.averageDurationMS.map(Double.init), lessIsBetter: true),
            costTile,
        ]
    }

    /// How a number changed from the window before: "+12% vs before",
    /// "−8% vs before", "Same as before", or "Up from 0".
    public static func describeChange(_ current: Double, from previous: Double) -> String {
        if previous == 0 {
            return current == 0 ? "Same as before" : "Up from 0"
        }
        let percent = Int(((current - previous) / previous * 100).rounded())
        if percent == 0 { return "Same as before" }
        return percent > 0 ? "+\(percent)% vs before" : "−\(-percent)% vs before"
    }
}
