import AppKit
@testable import JobSearchHub
import SwiftUI
import Testing

// The Why it fits card on a job page's rail, measured offscreen in a hosting
// view that no window holds, as PipelineHeaderFitTests does. TP-659's epic
// review found its meta cut short beside Write full brief on the 340 pt rail
// ("local model · 5 minutes a…", TP-1129); this checks that the meta shows
// whole at any age a brief can have.

/// Ages from seconds to most of a year, with the widest of each unit.
private let ages: [TimeInterval] = [30, 5 * 60, 59 * 60, 23 * 3600, 3 * 86400, 20 * 86400, 300 * 86400]

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

/// The card's header as the rail shows a pre-brief: the title, who wrote it
/// and when, and Write full brief.
@MainActor
private func makeHeader(writtenAgo age: TimeInterval) -> some View {
    let meta = JobDetailView.describeAuthor(isFull: false, writtenAt: Date.now.addingTimeInterval(-age))
    return HubCard("Why it fits", meta: meta) {
        EmptyView()
    } trailing: {
        AsyncButton("Write full brief", busyTitle: "Writing…") {}
    }
}

@MainActor
@Test(arguments: ages)
func theBriefsMetaFitsBesideWriteFullBriefOnTheRail(age: TimeInterval) {
    let width = measureWidth(makeHeader(writtenAgo: age))
    let rail = JobPostingTab<EmptyView>.railWidth
    // The title and the button alone are wider than this; less means the
    // header wasn't measured.
    #expect(width > 150)
    #expect(width <= rail, "The header needs \(width) pt; the rail is \(rail) pt")
}
