import Foundation
@testable import JobSearchHubCore
import Testing

private func makeItem(_ title: String, firstSeen: Date = Date(timeIntervalSince1970: 0), level: FitLevel = .good, checks: [FitCheck] = []) -> JobListItem {
    let job = Job(id: UUID(), source: "manual", title: title, url: "https://acme.com/\(title)", firstSeenAt: firstSeen, lastSeenAt: firstSeen)
    return JobListItem(job: job, companyName: "Acme", fit: JobFit(level: level, checks: checks), unseenUpdates: 0)
}

@Test func theScreenSaysPassesTheCountUnclearOrTheCheckThatFails() {
    let passes = ScreenSummary(JobFit(level: .good, checks: [FitCheck(name: "Role", verdict: .yes, reason: "")]))
    let unclear = ScreenSummary(JobFit(level: .unclear, checks: [
        FitCheck(name: "Timezone", verdict: .unclear, reason: ""), FitCheck(name: "Level", verdict: .unclear, reason: ""),
        FitCheck(name: "Role", verdict: .yes, reason: ""),
    ]))
    let fails = ScreenSummary(JobFit(level: .poor, checks: [
        FitCheck(name: "Role", verdict: .yes, reason: ""), FitCheck(name: "Where they hire", verdict: .no, reason: "names only \"US\""),
    ]))
    let failsTwice = ScreenSummary(JobFit(level: .poor, checks: [
        FitCheck(name: "Pay", verdict: .no, reason: ""), FitCheck(name: "Stack", verdict: .no, reason: ""),
    ]))

    #expect(passes.text == "Passes" && passes.verdict == .yes)
    #expect(unclear.text == "2 unclear" && unclear.verdict == .unclear)
    #expect(unclear.accessibilityLabel == "Screen unclear on Timezone and Level")
    #expect(fails.text == "Where" && fails.verdict == .no)
    #expect(fails.accessibilityLabel == "Fails the screen on Where they hire")
    #expect(failsTwice.text == "Pay +1")
    #expect(ScreenSummary(JobFit(level: .poor, checks: [])).text == "Fails")
}

@Test func theTakeHomeIsReadFromThePayChecksReason() {
    func read(_ reason: String, name: String = "Pay") -> String? {
        TakeHomeEstimate(FitCheck(name: name, verdict: .yes, reason: reason))?.text
    }

    #expect(read("about BRL 31.0k a month take-home, 103% of the target") == "R$ 31k")
    #expect(read("at least BRL 28.5k a month take-home") == "R$ 28.5k")
    #expect(read("at most BRL 22.0k a month take-home, under the BRL 25.0k minimum") == "R$ 22k")
    #expect(read("BRL 20.0k to 33.0k a month take-home, depending on the contract or pay period") == "R$ 20–33k")
    #expect(read("paid by the hour") == nil)
    #expect(read("about BRL 31.0k a month take-home", name: "Stack") == nil)
    #expect(TakeHomeEstimate(nil) == nil)
    #expect(TakeHomeEstimate(FitCheck(name: "Pay", verdict: .unclear, reason: "BRL 20.0k to 33.0k a month"))?.lowest == 20_000)
}

@Test func agesReadInTheLargestWholeUnit() {
    let now = Date(timeIntervalSince1970: 100 * 86_400)
    func age(_ seconds: TimeInterval) -> String { JobAge.format(now.addingTimeInterval(-seconds), now: now) }

    #expect(age(60) == "now")
    #expect(age(5 * 3600) == "5 h")
    #expect(age(2 * 86_400) == "2 d")
    #expect(age(8 * 86_400) == "1 w")
    #expect(age(70 * 86_400) == "2 mo")
    #expect(age(800 * 86_400) == "2 y")
    #expect(age(-3600) == "now")
}

@Test func postedIsTheBoardsDateOrElseFirstSeen() {
    var job = makeItem("a", firstSeen: Date(timeIntervalSince1970: 500)).job
    #expect(job.postedAt == Date(timeIntervalSince1970: 500))
    job.publishedAt = Date(timeIntervalSince1970: 100)
    #expect(job.postedAt == Date(timeIntervalSince1970: 100))
}

@Test func rowsGroupByWhenTheHubFirstSawThem() throws {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = try #require(TimeZone(identifier: "UTC"))
    let now = try #require(calendar.date(from: DateComponents(year: 2026, month: 10, day: 6, hour: 15)))
    func daysAgo(_ days: Double) -> Date { now.addingTimeInterval(-days * 86_400) }
    let items = [
        makeItem("today", firstSeen: daysAgo(0.1)), makeItem("old", firstSeen: daysAgo(20)),
        makeItem("yesterday", firstSeen: daysAgo(1.5)), makeItem("monday", firstSeen: daysAgo(5)),
    ]

    let newestFirst = JobGroups.make(items, newestFirst: true, now: now, calendar: calendar)
    #expect(newestFirst.map(\.kind) == [.newSinceYesterday, .thisWeek, .earlier])
    #expect(newestFirst.map { $0.items.map(\.job.title) } == [["today", "yesterday"], ["monday"], ["old"]])
    #expect(newestFirst.map(\.kind.title) == ["New since yesterday", "Earlier this week", "Earlier"])

    let oldestFirst = JobGroups.make(Array(items.prefix(1)) + [items[1]], newestFirst: false, now: now, calendar: calendar)
    #expect(oldestFirst.map(\.kind) == [.earlier, .newSinceYesterday])
}
