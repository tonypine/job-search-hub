import Foundation
@testable import JobSearchHubCore
import Testing

private let hour: TimeInterval = 60 * 60
private let day: TimeInterval = 24 * hour

private var newYork: Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "America/New_York")!
    calendar.locale = Locale(identifier: "en_US")
    return calendar
}

private func makeDate(_ year: Int, _ month: Int, _ day: Int, _ hour: Int = 0, _ minute: Int = 0) -> Date {
    newYork.date(from: DateComponents(year: year, month: month, day: day, hour: hour, minute: minute))!
}

/// A window's metrics with a bucket from each boundary to the next, as the
/// hub clips them: the first starts at the window's start, the last ends at
/// its end.
private func makeMetrics(_ bucket: String, _ bounds: [Date]) -> RunMetrics {
    let buckets = zip(bounds, bounds.dropFirst()).map { RunMetrics.Bucket(start: $0, end: $1, runs: 1) }
    let totals = RunMetrics.Totals(from: bounds[0], to: bounds[bounds.count - 1])
    return RunMetrics(bucket: bucket, timeZone: "America/New_York", totals: totals, previous: totals, buckets: buckets)
}

/// 24 hours by the hour, read at 4:10 AM: the 4 AM hour has run 10 minutes.
private let lastDay: RunMetrics = {
    let now = makeDate(2026, 10, 30, 4, 10)
    let hours = (5 ... 23).map { makeDate(2026, 10, 29, $0) } + (0 ... 4).map { makeDate(2026, 10, 30, $0) }
    return makeMetrics("hour", [now.addingTimeInterval(-day)] + hours + [now])
}()

/// 7 days by the day, read at 4:10 AM on Oct 30: today has run 4 hours.
private let lastWeek: RunMetrics = {
    let now = makeDate(2026, 10, 30, 4, 10)
    return makeMetrics("day", [now.addingTimeInterval(-7 * day)] + (24 ... 30).map { makeDate(2026, 10, $0) } + [now])
}()

@Test func theHourInProgressIsDrawnAsWideAsAWholeHour() {
    let last = lastDay.buckets[lastDay.buckets.count - 1]
    #expect(last.end.timeIntervalSince(last.start) == 10 * 60)

    let drawn = lastDay.getDrawnInterval(start: last.start, end: last.end, calendar: newYork)
    #expect(drawn == DateInterval(start: makeDate(2026, 10, 30, 4), end: makeDate(2026, 10, 30, 5)))
    #expect(lastDay.getDrawnRange(calendar: newYork) == DateInterval(start: makeDate(2026, 10, 29, 4, 10), end: makeDate(2026, 10, 30, 5)))
}

@Test func theBucketsBeforeTheLastAreDrawnAsTheHubCountedThem() {
    // The first, clipped to the window's start, stays clipped.
    for bucket in lastDay.buckets.dropLast() {
        #expect(lastDay.getDrawnInterval(start: bucket.start, end: bucket.end, calendar: newYork) == bucket.interval)
    }
    #expect(lastDay.buckets[0].interval == DateInterval(start: makeDate(2026, 10, 29, 4, 10), end: makeDate(2026, 10, 29, 5)))
}

@Test func todayIsDrawnAsWideAsADayAndItsDateLabelsTheAxis() {
    let today = lastWeek.buckets[lastWeek.buckets.count - 1]
    let drawn = lastWeek.getDrawnInterval(start: today.start, end: today.end, calendar: newYork)
    #expect(drawn == DateInterval(start: makeDate(2026, 10, 30), end: makeDate(2026, 10, 31)))

    // On a plot 815 points wide, "Oct 30" now fits before the plot's end;
    // with the axis ending at 4:10 AM it didn't.
    let range = lastWeek.getDrawnRange(calendar: newYork)!
    let ticks = RunTimeAxis.getTicks(from: range.start, to: range.end, width: 815, calendar: newYork)
    #expect(ticks.last?.date == makeDate(2026, 10, 30))
    #expect(ticks.last?.title == "Oct 30")
    let clipped = RunTimeAxis.getTicks(from: range.start, to: today.end, width: 815, calendar: newYork)
    #expect(!clipped.map(\.title).contains("Oct 30"))
}

@Test func aMomentPastNowOnTheBarInProgressFindsItsBucket() {
    let last = lastDay.buckets[lastDay.buckets.count - 1]
    #expect(lastDay.getDrawnBucket(at: makeDate(2026, 10, 30, 4, 40), calendar: newYork) == last)
    #expect(lastDay.getDrawnBucket(at: makeDate(2026, 10, 30, 4, 5), calendar: newYork) == last)
    #expect(lastDay.getDrawnBucket(at: makeDate(2026, 10, 30, 3, 30), calendar: newYork)?.start == makeDate(2026, 10, 30, 3))
    #expect(lastDay.getDrawnBucket(at: makeDate(2026, 10, 30, 5), calendar: newYork) == nil)
    #expect(lastDay.getDrawnBucket(at: makeDate(2026, 10, 29, 4), calendar: newYork) == nil)
}

@Test func aRangeOfPastDaysEndingAtMidnightDrawsAsItIs() {
    let metrics = makeMetrics("day", (20 ... 23).map { makeDate(2026, 9, $0) })
    let last = metrics.buckets[metrics.buckets.count - 1]

    #expect(metrics.getDrawnInterval(start: last.start, end: last.end, calendar: newYork) == last.interval)
    #expect(metrics.getDrawnRange(calendar: newYork) == DateInterval(start: makeDate(2026, 9, 20), end: makeDate(2026, 9, 23)))
}

@Test func theDayTheClocksGoBackIsDrawnTwentyFiveHoursWide() {
    let metrics = makeMetrics("day", [makeDate(2026, 10, 31), makeDate(2026, 11, 1), makeDate(2026, 11, 1, 9)])
    let drawn = metrics.getDrawnInterval(start: makeDate(2026, 11, 1), end: makeDate(2026, 11, 1, 9), calendar: newYork)

    #expect(drawn == DateInterval(start: makeDate(2026, 11, 1), end: makeDate(2026, 11, 2)))
    #expect(drawn.duration == 25 * hour)
}

@Test func theWeekInProgressRunsToTheNextMonday() {
    // A US calendar's weeks start on Sunday; the hub's on Monday.
    let metrics = makeMetrics("week", [makeDate(2026, 10, 19), makeDate(2026, 10, 26), makeDate(2026, 10, 30, 4, 10)])
    let drawn = metrics.getDrawnInterval(start: makeDate(2026, 10, 26), end: makeDate(2026, 10, 30, 4, 10), calendar: newYork)

    #expect(drawn == DateInterval(start: makeDate(2026, 10, 26), end: makeDate(2026, 11, 2)))
    #expect(RunBucketSize.week.getPeriod(containing: makeDate(2026, 11, 1, 12), calendar: newYork)?.start == makeDate(2026, 10, 26))
}

@Test func aBucketTheAppDoesNotKnowDrawsAsItIs() {
    let metrics = makeMetrics("month", [makeDate(2026, 10, 1), makeDate(2026, 10, 30, 4, 10)])
    let last = metrics.buckets[0]

    #expect(metrics.getDrawnInterval(start: last.start, end: last.end, calendar: newYork) == last.interval)
}
