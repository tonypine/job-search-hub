import Foundation
@testable import JobSearchHubCore
import Testing

private let now = Date(timeIntervalSince1970: 1_792_525_620) // 2026-10-20 19:47 UTC
private let hour: TimeInterval = 60 * 60
private let day: TimeInterval = 24 * hour

private var utc: Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC")!
    return calendar
}

@Test func aPresetCountsItsLengthUpToNow() {
    let window = RunWindowChoice.standard.resolve(now: now, calendar: utc)

    #expect(window.from == now.addingTimeInterval(-day))
    #expect(window.to == now)
    #expect(window.bucket == .hour)
    #expect(window.readAt == now)
}

@Test func eachPresetStartsWithItsOwnBucket() {
    let choice = RunWindowChoice.standard
    #expect(choice.setting(.lastWeek, now: now, calendar: utc).bucket == .day)
    #expect(choice.setting(.lastMonth, now: now, calendar: utc).bucket == .day)
    #expect(choice.setting(.lastQuarter, now: now, calendar: utc).bucket == .day)
    #expect(choice.setting(.lastDay, now: now, calendar: utc).bucket == .hour)
}

@Test func aBucketTheWindowDoesntOfferGivesWayToItsOwn() {
    // 90 days by the hour is too many bars; 24 hours by the week too few.
    let byHour = RunWindowChoice(preset: .lastQuarter, bucket: .hour)
    #expect(byHour.resolve(now: now, calendar: utc).bucket == .day)
    let byWeek = RunWindowChoice(preset: .lastDay, bucket: .week)
    #expect(byWeek.resolve(now: now, calendar: utc).bucket == .hour)
    let weeks = RunWindowChoice(preset: .lastQuarter, bucket: .week)
    #expect(weeks.resolve(now: now, calendar: utc).bucket == .week)

    #expect(RunBucketSize.getOffered(forLength: day) == [.hour, .day])
    #expect(RunBucketSize.getOffered(forLength: 7 * day) == [.hour, .day])
    #expect(RunBucketSize.getOffered(forLength: 30 * day) == [.day, .week])
}

@Test func aCustomRangeCountsItsDaysFromMidnightAndNeverPastNow() {
    let first = now.addingTimeInterval(-10 * day)
    let choice = RunWindowChoice(preset: .custom, bucket: .day, customFrom: first, customThrough: now.addingTimeInterval(-3 * day))
    let window = choice.resolve(now: now, calendar: utc)

    #expect(window.from == utc.startOfDay(for: first))
    #expect(window.to == utc.startOfDay(for: now.addingTimeInterval(-2 * day)))
    #expect(window.bucket == .day)

    let throughToday = RunWindowChoice(preset: .custom, bucket: .day, customFrom: first, customThrough: now)
    #expect(throughToday.resolve(now: now, calendar: utc).to == now)

    // Its last day before its first counts the first day alone.
    let backwards = RunWindowChoice(preset: .custom, bucket: .hour, customFrom: first, customThrough: first.addingTimeInterval(-5 * day))
    let single = backwards.resolve(now: now, calendar: utc)
    #expect(single.to.timeIntervalSince(single.from) == day)
}

@Test func choosingACustomRangeStartsFromTheWindowShown() {
    let custom = RunWindowChoice(preset: .lastWeek, bucket: .day).setting(.custom, now: now, calendar: utc)

    #expect(custom.customFrom == utc.startOfDay(for: now.addingTimeInterval(-7 * day)))
    #expect(custom.customThrough == utc.startOfDay(for: now))
    #expect(custom.bucket == .day)
}

@Test func eachPageKeepsItsOwnChoice() {
    let choices = [
        "": RunWindowChoice(preset: .lastMonth, bucket: .week),
        "job_brief": RunWindowChoice(preset: .custom, bucket: .day, customFrom: now, customThrough: now),
    ]
    let text = RunWindowChoice.encodeChoices(choices)

    #expect(RunWindowChoice.decodeChoices(text) == choices)
    #expect(RunWindowChoice.decodeChoices("").isEmpty)
    #expect(RunWindowChoice.decodeChoices("not json").isEmpty)
}

@Test func theMetricsQueryNamesTheWindowTheBucketAndTheTimeZone() {
    let window = ResolvedRunWindow(from: now.addingTimeInterval(-day), to: now, bucket: .hour, readAt: now)
    let query = window.makeMetricsQuery(kind: "job_brief", timeZone: TimeZone(identifier: "Europe/Lisbon")!)

    #expect(query.map(\.name) == ["from", "to", "bucket", "time_zone", "kind"])
    #expect(query[0].value == "2026-10-19T19:47:00Z")
    #expect(query[1].value == "2026-10-20T19:47:00Z")
    #expect(query[2].value == "hour")
    #expect(query[3].value == "Europe/Lisbon")
    #expect(!window.makeMetricsQuery(kind: nil, timeZone: .gmt).contains { $0.name == "kind" })
}

private func makeTotals(
    runs: Int, failed: Int = 0, input: Int = 0, output: Int = 0, average: Int? = nil, cost: String = "0", claudePlan: Int = 0
) -> RunMetrics.Totals {
    RunMetrics.Totals(
        from: now.addingTimeInterval(-day), to: now, runs: runs, failed: failed, inputTokens: input, outputTokens: output,
        averageDurationMS: average, costUSD: cost, claudePlanRuns: claudePlan
    )
}

@Test func theFiveTilesCompareWithTheWindowBefore() throws {
    let metrics = RunMetrics(
        bucket: "hour",
        totals: makeTotals(runs: 120, failed: 6, input: 1_240_000, output: 34000, average: 4200, cost: "1.234"),
        previous: makeTotals(runs: 100, failed: 8, input: 0, output: 0, average: 4200, cost: "0"),
        buckets: []
    )
    let tiles = RunMetricTiles.make(metrics, isOnClaudePlan: false)

    #expect(tiles.map(\.title) == ["Runs", "Failed", "Tokens", "Average time", "Cost"])
    #expect(tiles.map(\.value) == ["120", "6", "1.2M in · 34K out", "4.2 s", "$1.23"])
    #expect(tiles.map(\.change) == ["+20% vs before", "−25% vs before", "Up from 0", "Same as before", "Up from 0"])
    #expect(tiles[1].detail == "5% of runs")
    // Fewer failures read well; more runs aren't good or bad.
    #expect(tiles[1].changeTone == .positive)
    #expect(tiles[0].changeTone == .neutral)
}

@Test func costOnTheClaudePlanReadsAsThePlan() {
    let metrics = RunMetrics(
        bucket: "day", totals: makeTotals(runs: 3, cost: "2.5"), previous: makeTotals(runs: 1, cost: "1"), buckets: []
    )
    let cost = RunMetricTiles.make(metrics, isOnClaudePlan: true).last

    #expect(cost?.value == "On your Claude plan")
    #expect(cost?.detail == "$2.50 at API prices")
    #expect(cost?.change == nil)

    let allKinds = RunMetrics(
        bucket: "day", totals: makeTotals(runs: 3, cost: "0.001", claudePlan: 2), previous: makeTotals(runs: 0), buckets: []
    )
    let mixed = RunMetricTiles.make(allKinds, isOnClaudePlan: false)
    #expect(mixed.last?.value == "<$0.01")
    #expect(mixed.last?.detail == "2 on your Claude plan")
    #expect(mixed[3].value == "—")
    #expect(mixed[3].change == nil)
    #expect(mixed[0].change == "Up from 0")
}

@Test func countsShortenForTilesAndAxes() {
    #expect(RunWindowFormat.formatCompact(950) == "950")
    #expect(RunWindowFormat.formatCompact(12345) == "12K")
    #expect(RunWindowFormat.formatCompact(1234) == "1.2K")
    #expect(RunWindowFormat.formatCompact(1_250_000) == "1.2M")
    #expect(RunWindowFormat.formatCompact(25_000_000) == "25M")
    #expect(RunMetricTiles.describeChange(0, from: 0) == "Same as before")
    #expect(RunMetricTiles.describeChange(50, from: 100) == "−50% vs before")
}
