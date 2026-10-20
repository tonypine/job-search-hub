import Foundation
@testable import JobSearchHubCore
import Testing

private let start = Date(timeIntervalSince1970: 1_792_454_400) // 2026-10-20 00:00 UTC
private let hour: TimeInterval = 60 * 60

/// A window of hourly buckets, each counting the kinds given for it.
private func makeMetrics(_ buckets: [[RunMetrics.KindCount]]) -> RunMetrics {
    let made = buckets.enumerated().map { index, kinds in
        RunMetrics.Bucket(
            start: start.addingTimeInterval(Double(index) * hour), end: start.addingTimeInterval(Double(index + 1) * hour),
            runs: kinds.reduce(0) { $0 + $1.runs }, failed: kinds.reduce(0) { $0 + $1.failed },
            inputTokens: kinds.reduce(0) { $0 + $1.inputTokens }, outputTokens: kinds.reduce(0) { $0 + $1.outputTokens }, kinds: kinds
        )
    }
    let end = start.addingTimeInterval(Double(buckets.count) * hour)
    return RunMetrics(
        bucket: "hour", totals: RunMetrics.Totals(from: start, to: end),
        previous: RunMetrics.Totals(from: start.addingTimeInterval(-end.timeIntervalSince(start)), to: start), buckets: made
    )
}

private func count(_ kind: String, _ runs: Int, failed: Int = 0, tokens: Int = 0) -> RunMetrics.KindCount {
    RunMetrics.KindCount(kind: kind, runs: runs, failed: failed, inputTokens: tokens, outputTokens: 0)
}

/// Six kinds, job_facts the busiest, mail_triage and market_gaps tied on
/// runs with mail_triage ahead on tokens.
private let sixKinds = makeMetrics([
    [count("job_facts", 9), count("job_brief", 5, failed: 2), count("mail_triage", 2, tokens: 900)],
    [count("job_facts", 3), count("job_cv", 4), count("market_gaps", 2, tokens: 100), count("company_triage", 1)],
    [],
])

@Test func kindsRankByRunsThenTokensThenName() {
    #expect(RunCharts.rankKinds(sixKinds) == ["job_facts", "job_brief", "job_cv", "mail_triage", "market_gaps", "company_triage"])
}

@Test func theFourBusiestKindsGetColorsAndTheRestFoldIntoOneGraySeries() {
    let colors = RunCharts.assignColors(to: RunCharts.rankKinds(sixKinds), remembering: [:])
    #expect(colors == ["job_facts": 0, "job_brief": 1, "job_cv": 2, "mail_triage": 3])

    let chart = RunCharts.makeKindsChart(sixKinds, colors: colors, getTitle: RunsSummary.getKindTitle)

    #expect(chart.series.map(\.id) == ["job_facts", "job_brief", "job_cv", "mail_triage", RunCharts.otherSeriesID])
    #expect(chart.series.last?.title == "2 more kinds")
    #expect(chart.series.last?.paint == .other)
    #expect(chart.series.last?.kinds == ["market_gaps", "company_triage"])
    // Never more than five colors: four slots and the gray.
    let paints = Set(chart.series.map(\.paint) + chart.runs.map(\.paint) + chart.tokens.map(\.paint))
    #expect(paints.count <= 5)
    #expect(!paints.contains(.failed))
}

@Test func barsStackByKindAndTheFoldedKindsAddUp() throws {
    let colors = RunCharts.assignColors(to: RunCharts.rankKinds(sixKinds), remembering: [:])
    let chart = RunCharts.makeKindsChart(sixKinds, colors: colors, getTitle: RunsSummary.getKindTitle)

    let second = chart.getRuns(in: sixKinds.buckets[1])
    #expect(second.map(\.seriesID) == ["job_facts", "job_cv", RunCharts.otherSeriesID])
    #expect(second.map(\.base) == [0, 3, 7])
    #expect(second.map(\.value) == [3, 4, 3])
    #expect(second.last?.top == sixKinds.buckets[1].runs)
    // An empty bucket draws nothing.
    #expect(chart.getRuns(in: sixKinds.buckets[2]).isEmpty)
    #expect(chart.getTokens(in: sixKinds.buckets[0]).map(\.value) == [900])
    #expect(try #require(chart.getTokens(in: sixKinds.buckets[1]).last).seriesID == RunCharts.otherSeriesID)
}

@Test func fourKindsOrFewerFoldNothing() {
    let metrics = makeMetrics([[count("job_facts", 2), count("mail_triage", 1)]])
    let colors = RunCharts.assignColors(to: RunCharts.rankKinds(metrics), remembering: [:])
    let chart = RunCharts.makeKindsChart(metrics, colors: colors, getTitle: RunsSummary.getKindTitle)

    #expect(chart.series.map(\.title) == ["Job facts", "Mail sorting"])
    #expect(!chart.series.contains { $0.paint == .other })

    let five = makeMetrics([[count("a", 5), count("b", 4), count("c", 3), count("d", 2), count("e", 1)]])
    let fiveColors = RunCharts.assignColors(to: RunCharts.rankKinds(five), remembering: [:])
    #expect(RunCharts.makeKindsChart(five, colors: fiveColors, getTitle: { $0 }).series.last?.title == "1 more kind")
}

@Test func aKindKeepsItsColorWhenTheWindowChanges() {
    let day = RunCharts.assignColors(to: ["job_facts", "job_brief", "job_cv", "mail_triage"], remembering: [:])
    var history = day

    // A longer window ranks them otherwise, and market_gaps comes in.
    let month = RunCharts.assignColors(to: ["mail_triage", "market_gaps", "job_facts", "job_cv", "job_brief"], remembering: history)
    #expect(month["mail_triage"] == day["mail_triage"])
    #expect(month["job_facts"] == day["job_facts"])
    #expect(month["job_cv"] == day["job_cv"])
    // job_brief folds, and market_gaps takes the slot it frees.
    #expect(month["job_brief"] == nil)
    #expect(month["market_gaps"] == day["job_brief"])
    history.merge(month) { $1 }

    // Back to the day: market_gaps folds, and job_brief takes its own slot
    // back.
    let again = RunCharts.assignColors(to: ["job_facts", "job_brief", "job_cv", "mail_triage"], remembering: history)
    #expect(again == day)
    #expect(again["job_facts"] == day["job_facts"])
    #expect(again["job_cv"] == day["job_cv"])
    #expect(again["mail_triage"] == day["mail_triage"])
    #expect(Set(again.values).count == 4)
    #expect(again.values.allSatisfy { (0 ..< RunCharts.colorCount).contains($0) })
}

@Test func twoKindsRememberingOneSlotLeaveItToTheBusier() {
    let colors = RunCharts.assignColors(to: ["a", "b"], remembering: ["a": 2, "b": 2, "c": 0])
    #expect(colors == ["a": 2, "b": 0])
    // A slot out of range is forgotten.
    #expect(RunCharts.assignColors(to: ["a"], remembering: ["a": 9]) == ["a": 0])
}

@Test func aKindsChartStacksItsFailuresOnTopOfItsRuns() {
    let metrics = makeMetrics([[count("job_brief", 5, failed: 2, tokens: 1200)], [count("job_brief", 1, failed: 1)], []])
    let chart = RunCharts.makeKindChart(metrics)

    #expect(chart.series.map(\.paint) == [.slot(0), .failed])
    #expect(chart.getRuns(in: metrics.buckets[0]).map(\.seriesID) == [RunCharts.runsSeriesID, RunCharts.failedSeriesID])
    #expect(chart.getRuns(in: metrics.buckets[0]).map(\.base) == [0, 3])
    #expect(chart.getRuns(in: metrics.buckets[0]).map(\.top) == [3, 5])
    #expect(chart.getRuns(in: metrics.buckets[1]).map(\.paint) == [.failed])
    #expect(chart.getTokens(in: metrics.buckets[0]).map(\.value) == [1200])
    #expect(chart.runs.count == 3)
}

@Test func aMomentFindsItsBucket() {
    #expect(sixKinds.getBucket(at: start.addingTimeInterval(90 * 60))?.start == start.addingTimeInterval(hour))
    #expect(sixKinds.getBucket(at: start.addingTimeInterval(hour))?.start == start.addingTimeInterval(hour))
    #expect(sixKinds.getBucket(at: start.addingTimeInterval(-1)) == nil)
    #expect(sixKinds.getBucket(at: start.addingTimeInterval(3 * hour)) == nil)
}
