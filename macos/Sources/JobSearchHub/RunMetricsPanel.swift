import Charts
import JobSearchHubCore
import SwiftUI

/// A window of a kind's runs counted, or of every kind's, as the hub's
/// metrics give them, and the colors All kinds' kinds wear.
@MainActor
@Observable
final class RunMetricsModel {
    /// Where each kind's color is remembered, so a kind keeps it across
    /// windows and launches.
    static let colorsKey = "activityKindColors"

    private(set) var metrics: RunMetrics?
    /// The kind and window the metrics are of.
    private(set) var kind: String?
    private(set) var window: ResolvedRunWindow?
    /// The color slots of All kinds' four busiest kinds.
    private(set) var colors: [String: Int] = [:]
    var failure: HubFailure?

    /// Reads a kind's metrics, or every kind's without one, over a window.
    func load(kind: String?, window: ResolvedRunWindow, with client: HubClient) async {
        do {
            let query = window.makeMetricsQuery(kind: kind, timeZone: .current)
            let metrics = try await client.get("v1/runs/metrics", query: query, as: RunMetrics.self)
            if kind == nil {
                let history = UserDefaults.standard.dictionary(forKey: Self.colorsKey) as? [String: Int] ?? [:]
                colors = RunCharts.assignColors(to: RunCharts.rankKinds(metrics), remembering: history)
                UserDefaults.standard.set(history.merging(colors) { $1 }, forKey: Self.colorsKey)
            }
            (self.metrics, self.kind, self.window) = (metrics, kind, window)
            failure = nil
        } catch {
            // A read the page cancelled, for another window, isn't a failure.
            if !Task.isCancelled {
                failure = HubFailure("Couldn't count the runs", error)
            }
        }
    }
}

/// Over the runs list: the window and its bucket, five tiles compared with
/// the window before, and the runs and tokens charts on one time axis, or
/// the same numbers as a table. A bar clicked narrows the runs list to its
/// bucket, and the chip it shows clears it.
struct RunMetricsPanel: View {
    let model: RunMetricsModel
    /// Nil for All kinds, whose bars stack by kind.
    let kind: RunKind?
    /// The kind runs on the owner's Claude plan, so its cost is the plan's.
    let isOnClaudePlan: Bool
    @Binding var choice: RunWindowChoice
    let window: ResolvedRunWindow
    @Binding var selectedBucket: DateInterval?
    let getTitle: (String) -> String
    let client: HubClient
    @AppStorage("activityShowsMetricsTable") private var showsTable = false

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            controls
            if model.failure != nil {
                HubErrorView(Bindable(model).failure, retry: { Task { await model.load(kind: kind?.id, window: window, with: client) } })
            }
            if let metrics = shownMetrics {
                RunMetricTilesRow(tiles: RunMetricTiles.make(metrics, isOnClaudePlan: isPlanOnly(metrics)))
                let chart = kind == nil ? RunCharts.makeKindsChart(metrics, colors: model.colors, getTitle: getTitle) : RunCharts.makeKindChart(metrics)
                if showsTable {
                    RunMetricsTable(metrics: metrics, bucket: window.bucket, selectedBucket: $selectedBucket)
                } else {
                    RunMetricsCharts(metrics: metrics, chart: chart, window: window, selectedBucket: $selectedBucket)
                }
            } else if model.failure == nil {
                ProgressView().frame(maxWidth: .infinity, minHeight: 120)
            }
        }
        .padding(.horizontal, Space.l)
        .padding(.vertical, Space.m)
        .task(id: LoadKey(kind: kind?.id, window: window)) {
            await model.load(kind: kind?.id, window: window, with: client)
        }
    }

    /// The metrics read for this kind: while another window loads, the last
    /// one's stay, so the page doesn't jump.
    private var shownMetrics: RunMetrics? {
        guard model.kind == kind?.id else { return nil }
        return model.metrics
    }

    /// A kind's cost is its plan's when it runs on Claude, or when every run
    /// of the window did.
    private func isPlanOnly(_ metrics: RunMetrics) -> Bool {
        guard kind != nil else { return false }
        return isOnClaudePlan || (metrics.totals.runs > 0 && metrics.totals.claudePlanRuns == metrics.totals.runs)
    }

    private var controls: some View {
        FlowLayout(spacing: Space.s, lineSpacing: Space.s) {
            Picker("Window", selection: presetBinding) {
                ForEach(RunWindowPreset.allCases) { preset in
                    Text(preset.title).tag(preset)
                }
            }
            .pickerStyle(.menu)
            .labelsHidden()
            .fixedSize()
            .help("How far back to count the runs")
            if choice.preset == .custom {
                DatePicker("From", selection: dayBinding(\.customFrom), in: ...Date.now, displayedComponents: .date)
                    .fixedSize()
                DatePicker("Through", selection: dayBinding(\.customThrough), in: ...Date.now, displayedComponents: .date)
                    .fixedSize()
            }
            Picker("Bucket", selection: bucketBinding) {
                ForEach(RunBucketSize.getOffered(forLength: window.range.duration)) { size in
                    Text(size.title).tag(size)
                }
            }
            .pickerStyle(.menu)
            .labelsHidden()
            .fixedSize()
            .help("How the charts split the window")
            if let selectedBucket {
                Button {
                    self.selectedBucket = nil
                } label: {
                    HStack(spacing: 3) {
                        Text(RunMetricsFormat.describeBucket(start: selectedBucket.start, end: selectedBucket.end, size: window.bucket))
                        Image(systemName: "xmark.circle.fill").imageScale(.small)
                    }
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2)
                    .foregroundStyle(Tone.accent.color)
                    .background(Tone.accent.fill, in: Capsule())
                }
                .buttonStyle(.plain)
                .fixedSize()
                .help("List every run of the window again")
                .accessibilityLabel("Showing the runs of \(RunMetricsFormat.describeBucket(start: selectedBucket.start, end: selectedBucket.end, size: window.bucket)); clear")
            }
            Picker("View", selection: $showsTable) {
                Label("Chart", systemImage: "chart.bar").tag(false)
                Label("Table", systemImage: "tablecells").tag(true)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .fixedSize()
            .help("Show the numbers as charts or as a table")
        }
    }

    private var presetBinding: Binding<RunWindowPreset> {
        Binding(get: { choice.preset }, set: { choice = choice.setting($0, now: .now, calendar: .current) })
    }

    private var bucketBinding: Binding<RunBucketSize> {
        Binding(get: { window.bucket }, set: { choice.bucket = $0 })
    }

    private func dayBinding(_ path: WritableKeyPath<RunWindowChoice, Date?>) -> Binding<Date> {
        Binding(get: { choice[keyPath: path] ?? .now }, set: { choice[keyPath: path] = $0 })
    }

    private struct LoadKey: Equatable {
        let kind: String?
        let window: ResolvedRunWindow
    }
}

/// The five tiles side by side, wrapping where the page is narrow.
private struct RunMetricTilesRow: View {
    let tiles: [RunMetricTile]

    var body: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 116), spacing: Space.s)], alignment: .leading, spacing: Space.s) {
            ForEach(tiles) { tile in
                VStack(alignment: .leading, spacing: 2) {
                    Text(tile.title).font(.hubCaption).foregroundStyle(.secondary)
                    Text(tile.value)
                        .font(.title3.weight(.semibold))
                        .monospacedDigit()
                        .lineLimit(1)
                        .minimumScaleFactor(0.6)
                    if let change = tile.change {
                        HStack(spacing: 3) {
                            // Only the measures where less is better take a
                            // tone: down reads well, up badly.
                            if tile.changeTone != .neutral {
                                Image(systemName: tile.changeTone == .positive ? "arrow.down.right" : "arrow.up.right")
                                    .foregroundStyle(tile.changeTone.color)
                            }
                            Text(change).foregroundStyle(.secondary)
                        }
                        .font(.hubCaption)
                    }
                    if let detail = tile.detail {
                        Text(detail).font(.hubCaption).foregroundStyle(.secondary).lineLimit(2)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .padding(Space.s)
                // Tall enough for a value, a change and a detail, so the
                // tiles of a row line up.
                .frame(maxWidth: .infinity, minHeight: 84, alignment: .topLeading)
                .background(.quinary, in: RoundedRectangle(cornerRadius: Radius.card))
                .help(tile.help ?? "")
                .accessibilityElement(children: .combine)
            }
        }
    }
}

/// The runs chart, failures stacked on top in the negative tone, or All
/// kinds' stacked by kind, over the tokens chart on its own scale, both on
/// the window's time axis. Hovering a bucket shows its numbers; a click
/// narrows the runs list to it.
private struct RunMetricsCharts: View {
    let metrics: RunMetrics
    let chart: RunChart
    let window: ResolvedRunWindow
    @Binding var selectedBucket: DateInterval?
    @State private var hoveredStart: Date?

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            if chart.series.count > 1 {
                RunChartLegend(series: chart.series)
            }
            Text("Runs per \(window.bucket.noun)").font(.hubCaption).foregroundStyle(.secondary)
            RunBarChart(
                segments: chart.runs, metrics: metrics, chart: chart, window: window, isTokens: false,
                hoveredStart: $hoveredStart, selectedBucket: $selectedBucket
            )
            .frame(height: 120)
            .padding(.top, Space.xs)
            .zIndex(1)
            Text("Tokens per \(window.bucket.noun)").font(.hubCaption).foregroundStyle(.secondary)
            RunBarChart(
                segments: chart.tokens, metrics: metrics, chart: chart, window: window, isTokens: true,
                hoveredStart: $hoveredStart, selectedBucket: $selectedBucket
            )
            .frame(height: 96)
            .padding(.top, Space.xs)
        }
    }
}

/// Who each color is: a swatch and its name, never color alone.
private struct RunChartLegend: View {
    let series: [RunChartSeries]

    var body: some View {
        FlowLayout(spacing: Space.m, lineSpacing: Space.xs) {
            ForEach(series) { one in
                HStack(spacing: Space.xs) {
                    RoundedRectangle(cornerRadius: 2)
                        .fill(ChartColor.of(one.paint))
                        .frame(width: 10, height: 10)
                    Text(one.title).font(.hubCaption).foregroundStyle(.secondary).lineLimit(1)
                }
                .help(one.kinds.count > 1 ? one.kinds.map(RunsSummary.getKindTitle).joined(separator: ", ") : one.title)
            }
        }
    }
}

/// One of the two charts: stacked bars per bucket, a hover band, and the
/// runs chart's tooltip.
private struct RunBarChart: View {
    let segments: [RunChartSegment]
    let metrics: RunMetrics
    let chart: RunChart
    let window: ResolvedRunWindow
    let isTokens: Bool
    @Binding var hoveredStart: Date?
    @Binding var selectedBucket: DateInterval?

    /// The width of the value labels, the same in both charts, so their
    /// plots start at the same x and share the time axis.
    private static let valueLabelWidth: CGFloat = 40
    /// The room between a value label and the plot.
    private static let valueLabelSpacing: CGFloat = 6

    var body: some View {
        let plotted = Chart {
            if let hovered {
                RectangleMark(xStart: .value("Start", hovered.start), xEnd: .value("End", hovered.end))
                    .foregroundStyle(Color.primary.opacity(0.06))
            }
            ForEach(segments) { segment in
                let span = inset(segment)
                RectangleMark(
                    xStart: .value("Start", span.start), xEnd: .value("End", span.end),
                    yStart: .value("From", Double(segment.base)), yEnd: .value("To", Double(segment.top))
                )
                .foregroundStyle(ChartColor.of(segment.paint))
                .opacity(isDimmed(segment) ? 0.35 : 1)
                .accessibilityLabel("\(segment.seriesTitle), \(RunMetricsFormat.describeBucket(start: segment.bucketStart, end: segment.bucketEnd, size: window.bucket))")
                .accessibilityValue(isTokens ? "\(segment.value) tokens" : "\(segment.value) runs")
            }
            if !isTokens, let hovered {
                RuleMark(x: .value("Time", hovered.start.addingTimeInterval(hovered.end.timeIntervalSince(hovered.start) / 2)))
                    .foregroundStyle(Color.clear)
                    .annotation(position: .top, spacing: 2, overflowResolution: .init(x: .fit(to: .chart), y: .disabled)) {
                        RunBucketTooltip(bucket: hovered, chart: chart, size: window.bucket, isAllKinds: metrics.kind.isEmpty)
                    }
            }
        }
        .chartXScale(domain: window.from ... max(window.to, window.from.addingTimeInterval(1)))
        .chartYAxis {
            AxisMarks(position: .leading, values: .automatic(desiredCount: 3)) { value in
                AxisGridLine()
                AxisValueLabel(horizontalSpacing: Self.valueLabelSpacing) {
                    if let number = value.as(Double.self) {
                        Text(RunWindowFormat.formatCompact(number))
                            .frame(width: Self.valueLabelWidth, alignment: .trailing)
                    }
                }
            }
        }
        .chartOverlay { proxy in
            GeometryReader { geometry in
                Rectangle()
                    .fill(.clear)
                    .contentShape(Rectangle())
                    .onContinuousHover { phase in
                        switch phase {
                        case let .active(location): hoveredStart = getBucket(at: location, proxy, geometry)?.start
                        case .ended: hoveredStart = nil
                        }
                    }
                    .onTapGesture { location in
                        guard let bucket = getBucket(at: location, proxy, geometry) else { return }
                        selectedBucket = selectedBucket == bucket.interval ? nil : bucket.interval
                    }
            }
        }
        .accessibilityLabel(isTokens ? "Tokens per \(window.bucket.noun)" : "Runs per \(window.bucket.noun)")

        if isTokens {
            // As many labels as the plot's width reads in full, none past
            // its end.
            GeometryReader { geometry in
                let ticks = RunTimeAxis.getTicks(
                    from: window.from, to: window.to,
                    width: geometry.size.width - Self.valueLabelWidth - Self.valueLabelSpacing, calendar: .current
                )
                plotted.chartXAxis {
                    AxisMarks(values: ticks.map(\.date)) { value in
                        AxisGridLine()
                        AxisTick()
                        AxisValueLabel(ticks.indices.contains(value.index) ? ticks[value.index].title : "")
                    }
                }
            }
        } else {
            plotted.chartXAxis(.hidden)
        }
    }

    private var hovered: RunMetrics.Bucket? {
        hoveredStart.flatMap { start in metrics.buckets.first { $0.start == start } }
    }

    /// A bar a little narrower than its bucket, so bars stand apart.
    private func inset(_ segment: RunChartSegment) -> DateInterval {
        let gap = segment.bucketEnd.timeIntervalSince(segment.bucketStart) * 0.1
        return DateInterval(start: segment.bucketStart.addingTimeInterval(gap), end: segment.bucketEnd.addingTimeInterval(-gap))
    }

    private func isDimmed(_ segment: RunChartSegment) -> Bool {
        guard let selectedBucket else { return false }
        return selectedBucket.start != segment.bucketStart
    }

    private func getBucket(at location: CGPoint, _ proxy: ChartProxy, _ geometry: GeometryProxy) -> RunMetrics.Bucket? {
        guard let plotFrame = proxy.plotFrame else { return nil }
        let x = location.x - geometry[plotFrame].origin.x
        guard let date = proxy.value(atX: x, as: Date.self) else { return nil }
        return metrics.getBucket(at: date)
    }
}

/// A bucket's numbers: its runs by series, failures and tokens.
private struct RunBucketTooltip: View {
    let bucket: RunMetrics.Bucket
    let chart: RunChart
    let size: RunBucketSize
    let isAllKinds: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(RunMetricsFormat.describeBucket(start: bucket.start, end: bucket.end, size: size))
                .font(.caption.weight(.semibold))
            if isAllKinds {
                ForEach(chart.getRuns(in: bucket)) { segment in
                    HStack(spacing: Space.xs) {
                        RoundedRectangle(cornerRadius: 2).fill(ChartColor.of(segment.paint)).frame(width: 8, height: 8)
                        Text(segment.seriesTitle)
                        Spacer(minLength: Space.s)
                        Text(segment.value.formatted()).monospacedDigit()
                    }
                }
            }
            row("Runs", bucket.runs.formatted())
            row("Failed", bucket.failed.formatted())
            row("Tokens", "\(RunWindowFormat.formatCompact(Double(bucket.inputTokens))) in · \(RunWindowFormat.formatCompact(Double(bucket.outputTokens))) out")
        }
        .font(.caption)
        .padding(Space.s)
        .frame(minWidth: 160, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: Radius.control))
        .overlay(RoundedRectangle(cornerRadius: Radius.control).strokeBorder(.separator))
        .fixedSize()
    }

    private func row(_ title: String, _ value: String) -> some View {
        HStack(spacing: Space.s) {
            Text(title).foregroundStyle(.secondary)
            Spacer(minLength: Space.s)
            Text(value).monospacedDigit()
        }
    }
}

/// The charts' numbers as a table, a bucket a row; selecting a row narrows
/// the runs list as a click on its bar does.
private struct RunMetricsTable: View {
    let metrics: RunMetrics
    let bucket: RunBucketSize
    @Binding var selectedBucket: DateInterval?

    var body: some View {
        Table(metrics.buckets, selection: selection) {
            TableColumn(bucket.noun.capitalized) { row in
                Text(RunMetricsFormat.describeBucket(start: row.start, end: row.end, size: bucket)).lineLimit(1)
            }
            .width(min: 140, ideal: 180)
            TableColumn("Runs") { row in number(row.runs) }
                .width(64)
            TableColumn("Failed") { row in number(row.failed) }
                .width(64)
            TableColumn("Tokens in") { row in number(row.inputTokens) }
                .width(96)
            TableColumn("Tokens out") { row in number(row.outputTokens) }
                .width(96)
        }
        .frame(height: 236)
    }

    private var selection: Binding<Date?> {
        Binding(
            get: { selectedBucket?.start },
            set: { start in selectedBucket = start.flatMap { start in metrics.buckets.first { $0.start == start }?.interval } }
        )
    }

    private func number(_ value: Int) -> some View {
        Text(value.formatted()).monospacedDigit().frame(maxWidth: .infinity, alignment: .trailing)
    }
}

enum RunMetricsFormat {
    /// A bucket as the owner reads it: "Oct 20, 2:00 PM – 3:00 PM", "Mon,
    /// Oct 20", "Week of Oct 13".
    static func describeBucket(start: Date, end: Date, size: RunBucketSize) -> String {
        switch size {
        case .hour:
            "\(start.formatted(.dateTime.month(.abbreviated).day().hour().minute())) – \(end.formatted(.dateTime.hour().minute()))"
        case .day:
            start.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day())
        case .week:
            "Week of \(start.formatted(.dateTime.month(.abbreviated).day()))"
        }
    }
}
