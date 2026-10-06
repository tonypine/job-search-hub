import Foundation
@testable import JobSearchHubCore
import Testing

private func makeItem(
    _ title: String, company: String? = nil, firstSeen: TimeInterval = 0, published: TimeInterval? = nil,
    checks: [FitCheck] = [], pay: Pay? = nil
) -> JobListItem {
    var job = Job(id: UUID(), source: "manual", title: title, url: "https://acme.com/\(title)",
                  firstSeenAt: Date(timeIntervalSince1970: firstSeen), lastSeenAt: Date(timeIntervalSince1970: firstSeen))
    job.publishedAt = published.map(Date.init(timeIntervalSince1970:))
    job.pay = pay
    return JobListItem(job: job, companyName: company, fit: JobFit(level: .good, checks: checks), unseenUpdates: 0)
}

private func sortTitles(_ items: [JobListItem], by column: JobsSortColumn, _ order: SortOrder = .forward) -> [String] {
    items.sorted(using: JobsSortComparator(column, order: order)).map(\.job.title)
}

@Test func textColumnsSortEitherWayWithMissingValuesLast() {
    let items = [makeItem("b", company: "Beta"), makeItem("none"), makeItem("a", company: "alpha"), makeItem("c", company: "Gamma")]

    #expect(sortTitles(items, by: .company) == ["a", "b", "c", "none"])
    #expect(sortTitles(items, by: .company, .reverse) == ["c", "b", "a", "none"])
}

@Test func tiesKeepTheNewestFirst() {
    let items = [makeItem("older", company: "Acme", firstSeen: 100), makeItem("newer", company: "Acme", firstSeen: 200)]

    #expect(sortTitles(items, by: .company) == ["newer", "older"])
    #expect(sortTitles(items, by: .company, .reverse) == ["newer", "older"])
}

@Test func datesSortOldestFirstThenReverse() {
    let items = [makeItem("late", published: 300), makeItem("unpublished"), makeItem("early", published: 100)]

    #expect(sortTitles(items, by: .published) == ["early", "late", "unpublished"])
    #expect(sortTitles(items, by: .published, .reverse) == ["late", "early", "unpublished"])
}

@Test func aFitCheckSortsPassedThenUnclearThenFailed() {
    let items = [
        makeItem("failed", checks: [FitCheck(name: "Stack", verdict: .no, reason: "")]),
        makeItem("unchecked"),
        makeItem("passed", checks: [FitCheck(name: "Stack", verdict: .yes, reason: "")]),
        makeItem("unclear", checks: [FitCheck(name: "Stack", verdict: .unclear, reason: "")]),
    ]

    #expect(sortTitles(items, by: .fitCheck("Stack")) == ["passed", "unclear", "failed", "unchecked"])
}

@Test func paySortsByItsYearlyTopWithinACurrency() {
    let yearly = Pay(ranges: [PayRange(min: 90_000, max: 120_000, currency: "USD", interval: "year")])
    let hourly = Pay(ranges: [PayRange(min: 60, max: 70, currency: "USD", interval: "hour")])
    let canadian = Pay(ranges: [PayRange(min: 200_000, max: 250_000, currency: "CAD", interval: "year")])
    let items = [makeItem("yearly", pay: yearly), makeItem("unpaid"), makeItem("hourly", pay: hourly), makeItem("canadian", pay: canadian)]

    #expect(sortTitles(items, by: .pay) == ["canadian", "yearly", "hourly", "unpaid"])
}

@Test func matchSortsStrongFirstWithUnbriefedLast() {
    var strong = makeItem("strong"), stretch = makeItem("stretch"), mismatch = makeItem("mismatch")
    strong.match = .strong
    stretch.match = .stretch
    mismatch.match = .mismatch
    let items = [mismatch, makeItem("unbriefed"), strong, stretch]

    #expect(sortTitles(items, by: .match) == ["strong", "stretch", "mismatch", "unbriefed"])
    #expect(sortTitles(items, by: .match, .reverse) == ["mismatch", "stretch", "strong", "unbriefed"])
}

@Test func takeHomeSortsByTheLowestEstimate() {
    func pay(_ reason: String) -> [FitCheck] { [FitCheck(name: "Pay", verdict: .yes, reason: reason)] }
    let items = [
        makeItem("high", checks: pay("about BRL 40.0k a month take-home")), makeItem("hourly", checks: pay("paid by the hour")),
        makeItem("range", checks: pay("BRL 20.0k to 33.0k a month take-home, depending on the contract or pay period")),
        makeItem("none"),
    ]

    #expect(sortTitles(items, by: .takeHome, .reverse) == ["high", "range", "hourly", "none"])
}

@Test func postedSortsByTheBoardsDateOrElseFirstSeen() {
    let items = [makeItem("seen late", firstSeen: 300), makeItem("published early", firstSeen: 400, published: 100), makeItem("seen mid", firstSeen: 200)]

    #expect(sortTitles(items, by: .posted, .reverse) == ["seen late", "seen mid", "published early"])
}

@Test func sortOrdersSavedBeforeStillDecode() throws {
    let saved = try JSONEncoder().encode([JobsSortComparator(.firstSeen, order: .reverse)])
    let decoded = try JSONDecoder().decode([JobsSortComparator].self, from: saved)

    #expect(decoded == [JobsSortComparator(.firstSeen, order: .reverse)])
}
