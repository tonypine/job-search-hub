import Foundation
@testable import JobSearchHubCore
import Testing

private func makeItem(_ title: String, firstSeen: Date = Date(timeIntervalSince1970: 0), level: FitLevel = .good, checks: [FitCheck] = []) -> JobListItem {
    let job = Job(id: UUID(), source: "manual", title: title, url: "https://acme.com/\(title)", firstSeenAt: firstSeen, lastSeenAt: firstSeen)
    return JobListItem(job: job, companyName: "Acme", fit: JobFit(level: level, checks: checks), unseenUpdates: 0)
}

@Test func theScreenSaysPassesTheCountUnclearOrTheCheckThatFails() {
    func summarize(_ level: FitLevel, _ rows: [(String, FitVerdict?)]) -> ScreenSummary {
        ScreenSummary(ScreenBreakdown(rows.map { ScreenRow(name: $0.0, verdict: $0.1, reason: "") }, level: level))
    }
    let passes = summarize(.good, [("Role", .yes), ("Contract", nil)])
    let unclear = summarize(.unclear, [("Timezone", .unclear), ("Level", .unclear), ("Role", .yes)])
    let fails = summarize(.poor, [("Role", .yes), ("Hires from Brazil", .no)])
    let failsTwice = summarize(.poor, [("Pay", .no), ("Stack", .no), ("Level", .unclear)])

    #expect(passes.text == "Passes" && passes.verdict == .yes)
    #expect(unclear.text == "2 unclear" && unclear.verdict == .unclear)
    #expect(unclear.accessibilityLabel == "Screen unclear on Timezone and Level")
    #expect(fails.text == "Where" && fails.verdict == .no)
    #expect(fails.accessibilityLabel == "Fails the screen on Hires from Brazil")
    #expect(failsTwice.text == "Pay +1")
    #expect(summarize(.poor, []).text == "Fails")
    #expect(summarize(.unclear, []).text == "1 unclear")
    #expect(summarize(.good, [("Contract", nil)]).text == "Passes")
}

private let jobJSON = #"""
{"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Senior iOS Engineer","url":"https://acme.com/1",
        "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
 "fit":{"level":"good","checks":[{"name":"Role","verdict":"yes","reason":"names iOS"},
                                 {"name":"Where they hire","verdict":"yes","reason":"open to someone in Brazil"}]},
 "unseen_updates":0,
 "screen_out":[{"name":"Hires from Brazil","verdict":"yes","answer":"open to someone in Brazil"},
               {"name":"Timezone","verdict":"%TIMEZONE%","answer":"the posting doesn't say"},
               {"name":"Experience","verdict":"%EXPERIENCE%","answer":"asks for 12 years; you have 8 years"},
               {"name":"Contract","answer":"the posting doesn't say"}]}
"""#

/// The same job as a row of the jobs list and as its page, with the
/// Timezone and Experience answers given.
private func decodeJob(timezone: String, experience: String) throws -> (row: JobListItem, details: JobDetails) {
    let json = jobJSON.replacingOccurrences(of: "%TIMEZONE%", with: timezone).replacingOccurrences(of: "%EXPERIENCE%", with: experience)
    let row = try HubJSON.makeDecoder().decode(JobsResponse.self, from: Data(#"{"jobs":[\#(json)],"total":1}"#.utf8)).jobs[0]
    let details = try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(json.utf8))
    return (row, details)
}

@Test func aRowsScreenReadsAsItsPagesVerdictStrip() throws {
    for (timezone, experience, word, level) in [
        ("yes", "no", "Experience", FitLevel.poor), ("unclear", "unclear", "2 unclear", .unclear), ("yes", "yes", "Passes", .good),
    ] {
        let (row, details) = try decodeJob(timezone: timezone, experience: experience)
        let strip = try #require(details.getVerdicts(people: []).first(where: { $0.kind == .screen }))
        let summary = ScreenSummary(row.screen)

        #expect(row.fit.level == .good)
        #expect(row.screenRows == details.screenRows)
        #expect(summary.text == word && row.screenLevel == level)
        #expect(summary.verdict.tone == strip.tone && summary.verdict.symbolName == strip.symbolName)
    }
}

@Test func aRowFromAHubWithoutScreenOutAnswersReadsItsFit() {
    let item = makeItem("job", level: .unclear, checks: [FitCheck(name: "Stack", verdict: .unclear, reason: "no stack named")])

    #expect(item.screenOut == nil)
    #expect(item.screenRows == [ScreenRow(name: "Stack", verdict: .unclear, reason: "no stack named")])
    #expect(ScreenSummary(item.screen).text == "1 unclear" && item.screenLevel == .unclear)
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

@Test func aMonogramTakesTheFirstLettersOfTwoWords() {
    #expect(Monograms.getLetters(for: "Alex Kim") == "AK")
    #expect(Monograms.getLetters(for: "northwind") == "N")
    #expect(Monograms.getLetters(for: "riley  chen example") == "RC")
    #expect(Monograms.getLetters(for: "(Acme) Labs") == "AL")
    #expect(Monograms.getLetters(for: "37signals") == "3")
    #expect(Monograms.getLetters(for: "  -- ") == "?")
    #expect(Monograms.getLetters(for: "") == "?")
    #expect((0..<Monograms.hueCount).contains(Monograms.getHueIndex(for: "Globex", count: Monograms.hueCount)))
    #expect(Monograms.getHueIndex(for: "Globex", count: 0) == 0)
}

@Test func aCompanyHasTheSameMonogramInARowACardAndTheInspector() throws {
    let row = makeItem("Engineer")
    let now = Date(timeIntervalSince1970: 0)
    let application = Application(id: UUID(), phaseID: UUID(), phaseEnteredAt: now, createdAt: now, updatedAt: now)
    let card = PipelineCard(application: application, jobTitle: "Engineer", companyName: "Acme", unseenUpdates: 0)
    let inspector = try HubJSON.makeDecoder().decode(JobDetails.self, from: Data(#"""
    {"job":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","source":"manual","title":"Engineer","url":"https://acme.com/1",
            "first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},
     "company_name":"acme ","fit":{"level":"good","checks":[]},"unseen_updates":0}
    """#.utf8))

    let names = [row.companyName, card.companyName, inspector.companyName]
    let looks = names.map { Monograms.getLook(for: $0 ?? "") }
    #expect(looks == Array(repeating: Monograms.Look(letters: "A", hueIndex: Monograms.getHueIndex(for: "Acme", count: Monograms.hueCount)), count: 3))
}
