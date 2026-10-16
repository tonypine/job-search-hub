import Foundation

/// The screen as a row of the jobs list says it, in a word or two: "Passes",
/// "2 unclear", or the name of the check that fails, "Where".
public struct ScreenSummary: Equatable, Sendable {
    public var text: String
    /// Yes when it passes, unclear, or no when a check fails: the symbol and
    /// tone it shows in.
    public var verdict: FitVerdict
    public var accessibilityLabel: String

    public init(_ fit: JobFit) {
        let failing = fit.checks.filter { $0.verdict == .no }
        let unclear = fit.checks.filter { $0.verdict == .unclear }
        switch fit.level {
        case .good:
            text = "Passes"
            verdict = .yes
            accessibilityLabel = "Passes the screen"
        case .unclear:
            let count = max(unclear.count, 1)
            text = "\(count) unclear"
            verdict = .unclear
            accessibilityLabel = unclear.isEmpty ? "Screen unclear" : "Screen unclear on " + Self.list(unclear.map(\.name))
        case .poor:
            if let first = failing.first {
                text = failing.count == 1 ? Self.getShortName(first.name) : "\(Self.getShortName(first.name)) +\(failing.count - 1)"
                accessibilityLabel = "Fails the screen on " + Self.list(failing.map(\.name))
            } else {
                text = "Fails"
                accessibilityLabel = "Fails the screen"
            }
            verdict = .no
        }
    }

    /// A check's name short enough for a column: "Where they hire" is "Where".
    public static func getShortName(_ checkName: String) -> String {
        checkName == "Where they hire" ? "Where" : checkName
    }

    private static func list(_ names: [String]) -> String {
        guard let last = names.last, names.count > 1 else { return names.first ?? "" }
        return names.dropLast().joined(separator: ", ") + " and " + last
    }
}

/// What a job would leave each month, as its screen's Pay check estimated
/// it: "R$ 31k", or "R$ 20–33k" when it depends on the contract.
public struct TakeHomeEstimate: Equatable, Sendable {
    public var currency: String
    public var lowest: Double
    public var highest: Double

    /// Reads the estimate from the Pay check's reason, such as "about BRL
    /// 31.0k a month take-home"; nil without a check or an amount in it, as
    /// for "paid by the hour".
    public init?(_ check: FitCheck?) {
        guard let check, check.name == "Pay",
              let match = check.reason.firstMatch(of: #/([A-Z]{3}) (\d+(?:\.\d+)?)k(?: to (\d+(?:\.\d+)?)k)?/#),
              let lowest = Double(match.output.2)
        else { return nil }
        currency = String(match.output.1)
        self.lowest = lowest * 1000
        highest = (match.output.3.flatMap { Double($0) } ?? lowest) * 1000
    }

    public var text: String {
        let symbol = Self.getSymbol(currency)
        let low = Self.formatThousands(lowest)
        let high = Self.formatThousands(highest)
        return low == high ? "\(symbol) \(low)k" : "\(symbol) \(low)–\(high)k"
    }

    /// The currency's sign where it has a well-known one, "R$" for BRL, or
    /// else its code.
    static func getSymbol(_ currency: String) -> String {
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US")
        formatter.numberStyle = .currency
        formatter.currencyCode = currency
        return formatter.currencySymbol ?? currency
    }

    /// Thousands with one decimal at most: 31000 is "31", 31500 "31.5".
    private static func formatThousands(_ amount: Double) -> String {
        (amount / 1000).formatted(.number.precision(.fractionLength(0...1)).locale(Locale(identifier: "en_US")).grouping(.never))
    }
}

/// How long ago something happened, as a column says it: "now", "5 h",
/// "2 d", "1 w", "3 mo", "2 y".
public enum JobAge {
    public static func format(_ date: Date, now: Date) -> String {
        let hours = Int(now.timeIntervalSince(date) / 3600)
        let days = hours / 24
        switch days {
        case ..<1: return hours < 1 ? "now" : "\(hours) h"
        case ..<7: return "\(days) d"
        case ..<30: return "\(days / 7) w"
        case ..<365: return "\(days / 30) mo"
        default: return "\(days / 365) y"
        }
    }
}

extension Job {
    /// When the job was posted: the board's date, or else when the hub first
    /// saw it.
    public var postedAt: Date { publishedAt ?? firstSeenAt }
}

/// The rows of the jobs list under one heading, by when the hub first saw
/// them.
public struct JobGroup: Identifiable, Equatable, Sendable {
    public enum Kind: String, CaseIterable, Sendable {
        case newSinceYesterday, thisWeek, earlier

        public var title: String {
            switch self {
            case .newSinceYesterday: "New since yesterday"
            case .thisWeek: "Earlier this week"
            case .earlier: "Earlier"
            }
        }
    }

    public var kind: Kind
    public var items: [JobListItem]

    public var id: Kind { kind }
}

public enum JobGroups {
    /// Splits the rows, in their order, by when the hub first saw them: since
    /// the start of yesterday, in the six days before, or earlier. The newest
    /// group comes first unless the rows are oldest first; empty groups are
    /// left out.
    public static func make(_ items: [JobListItem], newestFirst: Bool, now: Date, calendar: Calendar = .current) -> [JobGroup] {
        let today = calendar.startOfDay(for: now)
        let yesterday = calendar.date(byAdding: .day, value: -1, to: today) ?? today
        let weekStart = calendar.date(byAdding: .day, value: -6, to: today) ?? today
        let grouped = Dictionary(grouping: items) { item -> JobGroup.Kind in
            if item.job.firstSeenAt >= yesterday { return .newSinceYesterday }
            return item.job.firstSeenAt >= weekStart ? .thisWeek : .earlier
        }
        let kinds = newestFirst ? JobGroup.Kind.allCases : JobGroup.Kind.allCases.reversed()
        return kinds.compactMap { kind in
            grouped[kind].map { JobGroup(kind: kind, items: $0) }
        }
    }
}
