import Foundation

/// Where a key fact came from: the board's own field, or the words of the
/// posting, read by the local model.
public enum KeyFactSource: Equatable, Sendable {
    case board, posting

    public var label: String {
        switch self {
        case .board: "From the board"
        case .posting: "Read from the posting"
        }
    }
}

/// One of the facts that decide whether to read a posting on, with where it
/// came from and, where the screen judged it, its verdict.
public struct KeyFact: Equatable, Identifiable, Sendable {
    public var title: String
    /// The fact, or what stands in for it: "Not read yet", "Not stated",
    /// "Not published".
    public var text: String
    /// Nil when nothing is known, and `text` says why.
    public var source: KeyFactSource?
    /// The screen's verdict on it, the worst when two checks judge it.
    public var verdict: FitVerdict?
    /// The checks that judged it and why, one per line: "Screen · Timezone: …".
    public var help: String?

    public var id: String { title }
}

public extension JobDetails {
    /// Pay, Hiring, Where they hire, Timezone, Level and Stack. Pay comes
    /// from the board first, as its ranges are exact; the others from the
    /// posting first, falling back on the board's field where it has one.
    var keyFacts: [KeyFact] {
        var pay = makeKeyFact("Pay", read: getReadFact("pay_in_text", "pay"), judgedBy: ["Pay"])
        if let line = job.pay?.summaryLine, !line.isEmpty {
            pay.text = line
            pay.source = .board
        } else if pay.source == nil && facts != nil {
            pay.text = "Not published"
        }
        var level = makeKeyFact("Level", read: getLevel(), judgedBy: ["Level", "Experience"])
        if level.source == nil, let years = getYears() {
            level.text = years
            level.source = .posting
        }
        return [
            pay,
            makeKeyFact("Hiring", read: getReadFact("contract", "contract_type"), board: job.employmentType, judgedBy: []),
            makeKeyFact("Where they hire", read: getReadFact("location", "location_restriction"), board: job.location, judgedBy: ["Where they hire", "Hires from Brazil"]),
            makeKeyFact("Timezone", read: getReadFact("timezone_requirement", "timezone"), judgedBy: ["Timezone"]),
            level,
            makeKeyFact("Stack", read: getReadFact("technologies"), judgedBy: ["Stack"]),
        ]
    }

    /// The screen's quotes to mark in the posting: each judged row's
    /// evidence, with its verdict and reason.
    var postingQuotes: [PostingQuote] {
        screenRows.compactMap { row in
            guard let verdict = row.verdict, let evidence = row.evidence, !evidence.isEmpty else { return nil }
            return PostingQuote(name: row.name, verdict: verdict, reason: row.reason, text: evidence)
        }
    }

    private func makeKeyFact(_ title: String, read: String?, board: String? = nil, judgedBy checks: [String]) -> KeyFact {
        let judged = screenRows.filter { row in row.verdict != nil && checks.contains(row.name) }
        let worst = judged.compactMap(\.verdict).max { getRank($0) < getRank($1) }
        let help = judged.isEmpty ? nil : judged.map { "Screen · \($0.name): \($0.reason)" }.joined(separator: "\n")
        var fact = KeyFact(title: title, text: facts == nil ? "Not read yet" : "Not stated", verdict: worst, help: help)
        if let read {
            fact.text = read
            fact.source = .posting
        } else if let board = board?.trimmingCharacters(in: .whitespacesAndNewlines), !board.isEmpty {
            fact.text = board
            fact.source = .board
        }
        return fact
    }

    /// The first of the facts by these keys that says something, as text.
    private func getReadFact(_ keys: String...) -> String? {
        for key in keys {
            switch facts?.entries.first(where: { $0.key == key })?.display {
            case let .text(text): return text
            case let .list(items): return items.joined(separator: ", ")
            case .notStated, nil: continue
            }
        }
        return nil
    }

    /// "Senior · 6+ years", or either part alone.
    private func getLevel() -> String? {
        guard let seniority = getReadFact("seniority") else { return nil }
        return [seniority, getYears()].compactMap { $0 }.joined(separator: " · ")
    }

    /// The years the posting asks for: "6+ years".
    private func getYears() -> String? {
        guard let years = getReadFact("years_of_experience") else { return nil }
        guard let count = Int(years) else { return years }
        return count == 1 ? "1+ year" : "\(count)+ years"
    }

    /// No is worse than unclear, unclear worse than yes.
    private func getRank(_ verdict: FitVerdict) -> Int {
        switch verdict {
        case .yes: 0
        case .unclear: 1
        case .no: 2
        }
    }
}

public extension Job {
    /// Where the posting's text came from: "From Northwind's Greenhouse
    /// board", "From Northwind's board", "From LinkedIn alert", "Added by hand".
    func describeSource(companyName: String?) -> String {
        guard source == "job_board" || source == "careers_page" else {
            return source == "manual" ? sourceName : "From \(sourceName)"
        }
        let owner = companyName.map { "\($0)'s" } ?? "the company's"
        if source == "careers_page" {
            return "From \(owner) careers page"
        }
        if let provider = boardProvider {
            return "From \(owner) \(provider) board"
        }
        return "From \(owner) board"
    }

    /// The board's provider, from the posting's address: Greenhouse, Lever…
    var boardProvider: String? {
        guard let host = URL(string: url)?.host()?.lowercased() else { return nil }
        return Self.providers.first { host == $0.domain || host.hasSuffix("." + $0.domain) }?.name
    }

    private static let providers: [(domain: String, name: String)] = [
        ("greenhouse.io", "Greenhouse"), ("lever.co", "Lever"), ("ashbyhq.com", "Ashby"), ("workable.com", "Workable"),
        ("smartrecruiters.com", "SmartRecruiters"), ("recruitee.com", "Recruitee"), ("personio.com", "Personio"),
        ("personio.de", "Personio"), ("bamboohr.com", "BambooHR"), ("gupy.io", "Gupy"), ("pinpointhq.com", "Pinpoint"),
        ("eightfold.ai", "Eightfold"),
    ]
}
