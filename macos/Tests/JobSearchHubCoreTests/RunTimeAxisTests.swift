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

/// Every label reads in full: each starts inside the plot and ends before
/// its end, none comes closer to the next than a label's width and a gap,
/// and no two say the same.
private func expectReadable(_ ticks: [RunTimeAxis.Tick], from: Date, to: Date, width: Double) {
    let pointsPerSecond = width / to.timeIntervalSince(from)
    for tick in ticks {
        #expect(tick.date >= from)
        #expect(to.timeIntervalSince(tick.date) * pointsPerSecond >= RunTimeAxis.labelWidth)
    }
    for (one, next) in zip(ticks, ticks.dropFirst()) {
        #expect(next.date.timeIntervalSince(one.date) * pointsPerSecond >= RunTimeAxis.labelWidth + RunTimeAxis.labelGap)
    }
    #expect(Set(ticks.map(\.title)).count == ticks.count)
}

@Test func aDaysWindowEndingMidHourKeepsItsLastLabelInsideThePlot() {
    // 24 hours up to 8:50 PM, every three hours: 9 PM, 50 minutes before
    // the end, is too close to it for its label, so 6 PM is the last.
    let to = makeDate(2026, 10, 20, 20, 50)
    let from = to.addingTimeInterval(-day)
    let ticks = RunTimeAxis.getTicks(from: from, to: to, width: 815, calendar: newYork)

    expectReadable(ticks, from: from, to: to, width: 815)
    #expect(ticks.map(\.date) == [makeDate(2026, 10, 19, 21)] + stride(from: 0, through: 18, by: 3).map { makeDate(2026, 10, 20, $0) })
    #expect(ticks.map(\.title).filter { !$0.hasSuffix("AM") && !$0.hasSuffix("PM") } == ["Oct 20"])
}

@Test func aNarrowPlotLabelsFewerHoursAndStillReadsThemInFull() {
    // The inspector open leaves the plot a few hundred points.
    let to = makeDate(2026, 10, 20, 20, 50)
    let from = to.addingTimeInterval(-day)
    let wide = RunTimeAxis.getTicks(from: from, to: to, width: 815, calendar: newYork)
    let narrow = RunTimeAxis.getTicks(from: from, to: to, width: 340, calendar: newYork)

    expectReadable(narrow, from: from, to: to, width: 340)
    #expect(!narrow.isEmpty)
    #expect(narrow.count < wide.count)
}

@Test func aOneDayWindowReadsInHoursAndNeverRepeatsItsDate() {
    // Sep 20 through Sep 20, counted by the day: one bar, and its axis
    // reads the hours, not "Sep 20" under each.
    let from = makeDate(2026, 9, 20)
    let to = makeDate(2026, 9, 21)
    let ticks = RunTimeAxis.getTicks(from: from, to: to, width: 815, calendar: newYork)

    expectReadable(ticks, from: from, to: to, width: 815)
    #expect(ticks.count >= 4)
    #expect(ticks.first?.title == "Sep 20")
    #expect(ticks.dropFirst().allSatisfy { $0.title.hasSuffix("AM") || $0.title.hasSuffix("PM") })
}

@Test func aMonthsWindowReadsMondays() {
    let to = makeDate(2026, 10, 20, 20, 50)
    let from = to.addingTimeInterval(-30 * day)
    let ticks = RunTimeAxis.getTicks(from: from, to: to, width: 815, calendar: newYork)

    expectReadable(ticks, from: from, to: to, width: 815)
    // Monday Oct 19 is two days from the end, too close to it to read.
    #expect(ticks.map(\.title) == ["Sep 21", "Sep 28", "Oct 5", "Oct 12"])
    #expect(ticks.allSatisfy { newYork.component(.weekday, from: $0.date) == 2 && newYork.startOfDay(for: $0.date) == $0.date })
}

@Test func aYearsWindowReadsMonthsWithTheirYear() {
    let from = makeDate(2026, 1, 1)
    let to = makeDate(2027, 1, 1)
    let ticks = RunTimeAxis.getTicks(from: from, to: to, width: 600, calendar: newYork)

    expectReadable(ticks, from: from, to: to, width: 600)
    #expect(ticks.map(\.title) == ["Jan 2026", "Mar 2026", "May 2026", "Jul 2026", "Sep 2026", "Nov 2026"])
}

@Test func aPlotWithNoRoomOrAWindowWithNoLengthHasNoLabels() {
    let to = makeDate(2026, 10, 20, 20, 50)
    let from = to.addingTimeInterval(-day)
    #expect(RunTimeAxis.getTicks(from: from, to: to, width: 0, calendar: newYork).isEmpty)
    #expect(RunTimeAxis.getTicks(from: from, to: to, width: 40, calendar: newYork).isEmpty)
    #expect(RunTimeAxis.getTicks(from: to, to: to, width: 815, calendar: newYork).isEmpty)
}
