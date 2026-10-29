import Foundation

/// How a series of a runs chart is painted: one of the four categorical
/// colors, the gray of the kinds folded together, or the negative tone of
/// failures.
public enum RunChartPaint: Equatable, Hashable, Sendable {
    case slot(Int)
    case other
    case failed
}

/// A series of a runs chart, named in its legend: a kind, the kinds folded
/// into "N more kinds", or a kind's runs and its failures.
public struct RunChartSeries: Identifiable, Equatable, Sendable {
    public var id: String
    public var title: String
    public var paint: RunChartPaint
    /// The kinds it counts: one, the folded ones, or none on a kind's page.
    public var kinds: [String]
}

/// A piece of a stacked bar: a series' count in a bucket, stacked from base.
public struct RunChartSegment: Identifiable, Equatable, Sendable {
    public var bucketStart: Date
    public var bucketEnd: Date
    public var seriesID: String
    public var seriesTitle: String
    public var paint: RunChartPaint
    public var value: Int
    public var base: Int

    public var top: Int { base + value }
    public var id: String { "\(bucketStart.timeIntervalSinceReferenceDate)/\(seriesID)" }
}

/// What a page's two charts draw: the legend's series, the runs bars and the
/// tokens bars, each a stack of segments per bucket.
public struct RunChart: Equatable, Sendable {
    public var series: [RunChartSeries]
    public var runs: [RunChartSegment]
    public var tokens: [RunChartSegment]

    /// A bucket's segments of runs, bottom first.
    public func getRuns(in bucket: RunMetrics.Bucket) -> [RunChartSegment] {
        runs.filter { $0.bucketStart == bucket.start }
    }

    public func getTokens(in bucket: RunMetrics.Bucket) -> [RunChartSegment] {
        tokens.filter { $0.bucketStart == bucket.start }
    }
}

public enum RunCharts {
    /// The kinds that get a color of their own; the rest fold into one gray
    /// series, so a chart never shows more than five colors.
    public static let colorCount = 4
    public static let otherSeriesID = "more_kinds"
    public static let runsSeriesID = "runs"
    public static let failedSeriesID = "failed"

    /// The kinds that ran in the window, the busiest first: by runs, then by
    /// tokens, then by name.
    public static func rankKinds(_ metrics: RunMetrics) -> [String] {
        var runs: [String: Int] = [:]
        var tokens: [String: Int] = [:]
        for bucket in metrics.buckets {
            for count in bucket.kinds {
                runs[count.kind, default: 0] += count.runs
                tokens[count.kind, default: 0] += count.tokens
            }
        }
        return runs.keys.sorted { first, second in
            if runs[first] != runs[second] { return runs[first, default: 0] > runs[second, default: 0] }
            if tokens[first] != tokens[second] { return tokens[first, default: 0] > tokens[second, default: 0] }
            return first < second
        }
    }

    /// The color slots of the four busiest kinds. A kind keeps the slot it
    /// had when it was last shown, so changing the window doesn't repaint
    /// it; a kind whose slot a busier one holds, or that never had one,
    /// takes the lowest slot free.
    public static func assignColors(to ranked: [String], remembering history: [String: Int]) -> [String: Int] {
        let shown = Array(ranked.prefix(colorCount))
        var colors: [String: Int] = [:]
        var taken = Set<Int>()
        for kind in shown {
            if let slot = history[kind], (0 ..< colorCount).contains(slot), !taken.contains(slot) {
                colors[kind] = slot
                taken.insert(slot)
            }
        }
        for kind in shown where colors[kind] == nil {
            guard let slot = (0 ..< colorCount).first(where: { !taken.contains($0) }) else { break }
            colors[kind] = slot
            taken.insert(slot)
        }
        return colors
    }

    /// "1 more kind", "5 more kinds".
    public static func describeOther(_ count: Int) -> String {
        count == 1 ? "1 more kind" : "\(count) more kinds"
    }

    /// All kinds' chart: bars stacked by kind, the kinds with a color first
    /// in their rank, then one gray series for the rest. Kinds without a
    /// color in colors fold into it.
    public static func makeKindsChart(_ metrics: RunMetrics, colors: [String: Int], getTitle: (String) -> String) -> RunChart {
        let ranked = rankKinds(metrics)
        let colored = ranked.filter { colors[$0] != nil }
        let folded = ranked.filter { colors[$0] == nil }
        var series = colored.map { kind in
            RunChartSeries(id: kind, title: getTitle(kind), paint: .slot(colors[kind] ?? 0), kinds: [kind])
        }
        if !folded.isEmpty {
            series.append(RunChartSeries(id: otherSeriesID, title: describeOther(folded.count), paint: .other, kinds: folded))
        }
        let seriesOfKind = Dictionary(uniqueKeysWithValues: series.flatMap { one in one.kinds.map { ($0, one) } })

        var runs: [RunChartSegment] = []
        var tokens: [RunChartSegment] = []
        for bucket in metrics.buckets {
            var runsBySeries: [String: Int] = [:]
            var tokensBySeries: [String: Int] = [:]
            for count in bucket.kinds {
                guard let one = seriesOfKind[count.kind] else { continue }
                runsBySeries[one.id, default: 0] += count.runs
                tokensBySeries[one.id, default: 0] += count.tokens
            }
            runs += stack(bucket, series.map { ($0, runsBySeries[$0.id, default: 0]) })
            tokens += stack(bucket, series.map { ($0, tokensBySeries[$0.id, default: 0]) })
        }
        return RunChart(series: series, runs: runs, tokens: tokens)
    }

    /// A kind's chart: its runs that didn't fail, with its failures stacked
    /// on top in the negative tone, and its tokens.
    public static func makeKindChart(_ metrics: RunMetrics) -> RunChart {
        let ran = RunChartSeries(id: runsSeriesID, title: "Runs", paint: .slot(0), kinds: [])
        let failed = RunChartSeries(id: failedSeriesID, title: "Failed", paint: .failed, kinds: [])
        var runs: [RunChartSegment] = []
        var tokens: [RunChartSegment] = []
        for bucket in metrics.buckets {
            runs += stack(bucket, [(ran, max(bucket.runs - bucket.failed, 0)), (failed, bucket.failed)])
            tokens += stack(bucket, [(RunChartSeries(id: "tokens", title: "Tokens", paint: .slot(0), kinds: []), bucket.tokens)])
        }
        return RunChart(series: [ran, failed], runs: runs, tokens: tokens)
    }

    /// The segments of one bucket's bar, bottom first; series that counted
    /// nothing in it leave no segment.
    private static func stack(_ bucket: RunMetrics.Bucket, _ values: [(RunChartSeries, Int)]) -> [RunChartSegment] {
        var base = 0
        var segments: [RunChartSegment] = []
        for (series, value) in values where value > 0 {
            segments.append(RunChartSegment(
                bucketStart: bucket.start, bucketEnd: bucket.end, seriesID: series.id, seriesTitle: series.title,
                paint: series.paint, value: value, base: base
            ))
            base += value
        }
        return segments
    }
}
