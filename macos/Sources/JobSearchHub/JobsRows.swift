import JobSearchHubCore
import SwiftUI

/// The cells of the Jobs table's two-line rows: the job, then its match, its
/// screen, its take-home and when it was posted, each a word or a number.
/// Every cell keeps one height whatever the column's width, and every line
/// is truncated: rows whose height follows the table's width can loop the
/// window's layout as the inspector opens (#54, #66).
enum JobRowLayout {
    /// The rows' height. The table is told it as its rows' minimum height:
    /// it doesn't size every row from its cells on every macOS, and a 24 pt
    /// row cuts the second line off.
    static let rowHeight: CGFloat = 44
    /// The cells' height; the table's own padding makes the row 44 pt.
    static let cellHeight: CGFloat = 36
}

/// The row the pointer is over, whose actions show. Only the job cell reads
/// it, so moving the pointer redraws those cells and not the page.
@MainActor
@Observable
final class JobRowHover {
    var id: UUID?
}

extension View {
    /// Fills the cell at the rows' height and tells `hover` when the pointer
    /// is over it.
    func jobRowCell(_ id: UUID, hover: JobRowHover, alignment: Alignment = .leading) -> some View {
        frame(maxWidth: .infinity, minHeight: JobRowLayout.cellHeight, maxHeight: JobRowLayout.cellHeight, alignment: alignment)
            .contentShape(Rectangle())
            .onHover { isInside in
                if isInside {
                    if hover.id != id { hover.id = id }
                } else if hover.id == id {
                    hover.id = nil
                }
            }
    }
}

/// The job: its company's monogram, the title in semibold with its marks,
/// and the company and location under it. Hovered or selected, it ends in
/// the row's actions.
struct JobCell: View {
    let item: JobListItem
    let isSelected: Bool
    let hover: JobRowHover
    /// Restore stands in for Skip on jobs already skipped.
    let isSkipped: Bool
    let onDecide: (JobDecisionKind) -> Void
    let onRestore: () -> Void

    var body: some View {
        HStack(spacing: Space.m) {
            Monogram(name: item.companyName ?? item.job.title)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 6) {
                    Text(item.job.title).fontWeight(.semibold).lineLimit(1).help(item.job.title)
                    if item.unseenUpdates > 0 {
                        UnseenDot(count: item.unseenUpdates)
                    }
                    if let phase = item.pipelinePhase, !phase.isEmpty {
                        ToneChip("In \(phase)", tone: .accent, symbol: "rectangle.split.3x1")
                            .help("On the pipeline, in \(phase)")
                    }
                    if let reason = item.job.dismissalReason, !reason.isEmpty {
                        ToneChip(Self.shorten(reason), tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName)
                            .help("Skipped: \(reason)")
                            .accessibilityLabel("Skipped: \(reason)")
                    }
                }
                Text(context).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(1).help(context)
            }
            Spacer(minLength: 0)
            if isSelected || hover.id == item.id {
                JobRowActions(isSkipped: isSkipped, onDecide: onDecide, onRestore: onRestore)
            }
        }
        .jobRowCell(item.id, hover: hover)
    }

    /// "Northwind · Remote, Americas".
    private var context: String {
        [item.companyName, item.job.location].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// A skip reason short enough for a chip; the whole of it is its tooltip.
    private static func shorten(_ reason: String) -> String {
        reason.count > 24 ? String(reason.prefix(23)) + "…" : reason
    }
}

/// Pursue, Later and Skip at a row's end, each with its key in its tooltip.
struct JobRowActions: View {
    let isSkipped: Bool
    let onDecide: (JobDecisionKind) -> Void
    let onRestore: () -> Void

    var body: some View {
        HStack(spacing: Space.xs) {
            action("Pursue", symbol: "arrow.up.forward", key: "P") { onDecide(.pursue) }
            action("Later", symbol: "clock", key: "L") { onDecide(.later) }
            if isSkipped {
                action("Restore", symbol: "arrow.uturn.backward", key: nil, perform: onRestore)
            } else {
                action("Skip", symbol: SetAside.skipped.symbolName, key: "S") { onDecide(.skip) }
            }
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
        .labelStyle(.iconOnly)
    }

    private func action(_ title: String, symbol: String, key: String?, perform: @escaping () -> Void) -> some View {
        Button(title, systemImage: symbol, action: perform)
            .help(key.map { "\(title) (\($0))" } ?? title)
            .accessibilityLabel(title)
    }
}

/// The brief's match as a word and a symbol in its tone, or *Not briefed*.
struct MatchCell: View {
    let match: JobMatch?

    var body: some View {
        HStack(spacing: Space.xs) {
            if let match {
                Image(systemName: match.symbolName).imageScale(.small).accessibilityHidden(true)
                Text(match.title).fontWeight(.medium)
            } else {
                Image(systemName: "circle.dashed").imageScale(.small).accessibilityHidden(true)
                Text("Not briefed")
            }
        }
        .font(.hubSecondary)
        .lineLimit(1)
        .foregroundStyle(match.map { AnyShapeStyle($0.tone.color) } ?? AnyShapeStyle(.tertiary))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(match.map { "Match: \($0.title)" } ?? "Not briefed")
    }
}

/// The screen: ✓ Passes, ? 2 unclear, or ✗ and the check that fails.
struct ScreenCell: View {
    let fit: JobFit

    var body: some View {
        let summary = ScreenSummary(fit)
        HStack(spacing: Space.xs) {
            Image(systemName: summary.verdict.symbolName).imageScale(.small).foregroundStyle(summary.verdict.tone.color)
                .accessibilityHidden(true)
            Text(summary.text).foregroundStyle(summary.verdict == .yes ? AnyShapeStyle(.secondary) : AnyShapeStyle(summary.verdict.tone.color))
        }
        .font(.hubSecondary)
        .lineLimit(1)
        .help(details)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(summary.accessibilityLabel)
    }

    /// The checks that don't pass, with why.
    private var details: String {
        let doubts = fit.checks.filter { $0.verdict != .yes }.map { "\($0.name): \($0.reason)" }
        return doubts.isEmpty ? fit.level.label : doubts.joined(separator: "\n")
    }
}

/// What the job would leave each month, from the screen's Pay check.
struct TakeHomeCell: View {
    let check: FitCheck?

    var body: some View {
        let estimate = TakeHomeEstimate(check)
        Text(estimate?.text ?? "–")
            .font(.hubSecondary)
            .monospacedDigit()
            .lineLimit(1)
            .foregroundStyle(estimate == nil ? AnyShapeStyle(.tertiary) : AnyShapeStyle(.primary))
            .help(check?.reason ?? "No published pay to estimate from")
            .accessibilityLabel(check.map { "Take-home: \($0.reason)" } ?? "No take-home estimate")
    }
}

/// How long ago the job was posted: "2 d", "1 w".
struct PostedCell: View {
    let job: Job

    var body: some View {
        let now = Date.now
        Text(JobAge.format(job.postedAt, now: now))
            .font(.hubSecondary)
            .monospacedDigit()
            .lineLimit(1)
            .foregroundStyle(.secondary)
            .help(help)
            .accessibilityLabel("Posted \(job.postedAt.formatted(.relative(presentation: .named, unitsStyle: .wide)))")
    }

    private var help: String {
        let firstSeen = "First seen \(job.firstSeenAt.formatted(date: .abbreviated, time: .omitted))"
        guard let published = job.publishedAt else { return firstSeen }
        return "Posted \(published.formatted(date: .abbreviated, time: .omitted)) · \(firstSeen)"
    }
}

/// A group's heading in the table: "New since yesterday 3".
struct JobGroupHeader: View {
    let title: String
    let count: Int

    var body: some View {
        HStack(spacing: 6) {
            Text(title).fontWeight(.semibold)
            Text("\(count)").foregroundStyle(.secondary).monospacedDigit()
        }
        .font(.hubSecondary)
        .accessibilityElement(children: .combine)
    }
}
