import Foundation

/// One cell of a job's verdict strip: what was judged, its word, a symbol
/// and a tone, so color never stands alone.
public struct VerdictCell: Equatable, Identifiable, Sendable {
    public enum Kind: String, CaseIterable, Sendable {
        case match, screen, takeHome, people

        public var title: String {
            switch self {
            case .match: "Match"
            case .screen: "Screen"
            case .takeHome: "Take-home"
            case .people: "People"
            }
        }
    }

    public var kind: Kind
    /// The verdict in a word or two: "Strong", "2 unclear", "≈ BRL 31k/mo".
    public var word: String
    public var symbolName: String
    public var tone: Tone
    /// What the word leaves out, for its tooltip: the Pay check's reason.
    public var detail: String?

    public var id: Kind { kind }

    /// "Screen: 2 unclear", then the detail when there is one.
    public var accessibilityLabel: String {
        ["\(kind.title): \(word)", detail].compactMap { $0 }.joined(separator: ". ")
    }
}

public extension JobDetails {
    /// The verdicts a decision rests on, in the strip's order: the brief's
    /// match, the screen, the take-home the Pay check estimates, and how many
    /// people the owner knows at the company.
    func getVerdicts(peopleYouKnow: Int) -> [VerdictCell] {
        [matchVerdict, ScreenBreakdown(screenRows, level: fit.level).verdict, takeHomeVerdict, Self.makePeopleVerdict(knowing: peopleYouKnow)]
    }

    private var matchVerdict: VerdictCell {
        guard let brief else {
            return VerdictCell(kind: .match, word: "Not briefed", symbolName: "clock", tone: .neutral)
        }
        return VerdictCell(kind: .match, word: brief.match.title, symbolName: brief.match.symbolName, tone: brief.match.tone)
    }

    /// The Pay check's estimate in a few characters; without one, whether
    /// the posting publishes pay at all.
    private var takeHomeVerdict: VerdictCell {
        guard let check = fit.checks.first(where: { $0.name == FitCheck.payName }) else {
            if job.pay == nil {
                return VerdictCell(kind: .takeHome, word: "Not published", symbolName: "minus.circle", tone: .neutral)
            }
            return VerdictCell(
                kind: .takeHome, word: "Not judged", symbolName: "minus.circle", tone: .neutral,
                detail: "Set the take-home you need in Criteria, and the screen judges the pay"
            )
        }
        let word = TakeHomeEstimate.describe(check.reason) ?? (check.reason == "paid by the hour" ? "Hourly" : check.verdict.title)
        return VerdictCell(kind: .takeHome, word: word, symbolName: check.verdict.symbolName, tone: check.verdict.tone, detail: check.reason)
    }

    private static func makePeopleVerdict(knowing count: Int) -> VerdictCell {
        guard count > 0 else {
            return VerdictCell(kind: .people, word: "No one yet", symbolName: "person.2", tone: .neutral)
        }
        return VerdictCell(kind: .people, word: "\(count) you know", symbolName: "person.2.fill", tone: .accent)
    }
}

public extension FitCheck {
    /// The check that estimates the take-home from the published pay.
    static let payName = "Pay"
}

/// A job's Screen, exceptions first: the checks that fail or are unclear,
/// then the ones that pass, folded into a row, and what no rule judges.
public struct ScreenBreakdown: Equatable, Sendable {
    /// The failing checks, then the unclear ones, each in the screen's order.
    public var exceptions: [ScreenRow]
    public var passes: [ScreenRow]
    /// Information no rule judges, such as the contract.
    public var notes: [ScreenRow]
    /// The hub's own reading, for a screen without judged rows.
    public var level: FitLevel

    public init(_ rows: [ScreenRow], level: FitLevel) {
        exceptions = rows.filter { $0.verdict == .no } + rows.filter { $0.verdict == .unclear }
        passes = rows.filter { $0.verdict == .yes }
        notes = rows.filter { $0.verdict == nil }
        self.level = level
    }

    public var failing: Int { exceptions.count(where: { $0.verdict == .no }) }
    public var unclear: Int { exceptions.count(where: { $0.verdict == .unclear }) }

    /// The card's meta: "Passes" when every check passes, else "4 of 6
    /// pass"; nil with nothing judged.
    public var title: String? {
        let judged = exceptions.count + passes.count
        guard judged > 0 else { return nil }
        return exceptions.isEmpty ? FitLevel.good.title : "\(passes.count) of \(judged) pass"
    }

    /// The strip's cell: "Passes", "2 unclear" or "1 fails"; failing checks
    /// outrank unclear ones, and the tooltip names both.
    public var verdict: VerdictCell {
        if failing > 0 {
            let word = failing == 1 ? "1 fails" : "\(failing) fail"
            let detail = unclear > 0 ? "\(word), \(unclear) unclear" : nil
            return VerdictCell(kind: .screen, word: word, symbolName: FitVerdict.no.symbolName, tone: .negative, detail: detail)
        }
        if unclear > 0 {
            return VerdictCell(kind: .screen, word: "\(unclear) unclear", symbolName: FitVerdict.unclear.symbolName, tone: .caution)
        }
        if !passes.isEmpty {
            return VerdictCell(kind: .screen, word: FitLevel.good.title, symbolName: FitVerdict.yes.symbolName, tone: .positive)
        }
        return VerdictCell(kind: .screen, word: level.title, symbolName: level.symbolName, tone: level.tone)
    }
}

public extension TakeHomeEstimate {
    private static let suffix = " a month take-home"

    /// The Pay check's estimate of the monthly take-home, as the strip shows
    /// it: "about BRL 31.0k a month take-home, 85% of the target" reads
    /// "≈ BRL 31k/mo"; "at least" and "at most" read ≥ and ≤, and a range
    /// "BRL 20k–35.5k/mo". Nil for a reason that isn't an estimate.
    static func describe(_ reason: String) -> String? {
        guard let end = reason.range(of: suffix) else { return nil }
        var amount = String(reason[..<end.lowerBound])
        let qualifiers = [("about ", "≈ "), ("at least ", "≥ "), ("at most ", "≤ ")]
        var symbol = ""
        for (words, sign) in qualifiers where amount.hasPrefix(words) {
            amount.removeFirst(words.count)
            symbol = sign
        }
        amount = amount.replacingOccurrences(of: " to ", with: "–").replacingOccurrences(of: ".0k", with: "k")
        guard !amount.isEmpty else { return nil }
        return "\(symbol)\(amount)/mo"
    }
}

public extension JobBrief {
    /// What backs a point, as a short tag: the kind of its first cited
    /// entry ("Case", "Skill"), or "Gap" for a weakness that cites none.
    func getTag(of point: JobBriefPoint, isWeakness: Bool) -> String? {
        guard let entry = getEntries(of: point).first else { return isWeakness ? "Gap" : nil }
        return CitedEntry.getTag(ofKind: entry.kind)
    }
}

public extension CitedEntry {
    /// A knowledge-base kind as a tag: "case" reads "Case", "role"
    /// "Experience".
    static func getTag(ofKind kind: String) -> String {
        kind == "role" ? "Experience" : kind.prefix(1).uppercased() + kind.dropFirst()
    }
}

public extension FitLevel {
    var symbolName: String {
        switch self {
        case .good: FitVerdict.yes.symbolName
        case .unclear: FitVerdict.unclear.symbolName
        case .poor: FitVerdict.no.symbolName
        }
    }
}

public extension FitVerdict {
    /// One check's word: "Passes", "Unclear" or "Fails".
    var title: String {
        switch self {
        case .yes: FitLevel.good.title
        case .unclear: FitLevel.unclear.title
        case .no: FitLevel.poor.title
        }
    }
}
