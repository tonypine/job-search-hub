import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// The Jobs header beside the inspector, measured offscreen in a hosting
// view that no window holds. With a job selected in a 1400 pt window, the
// content column is what the sidebar and the inspector leave; there,
// TP-659's epic review found the last scope, All, cut to "A" under the
// search field (TP-1099). The strip still scrolls when it doesn't fit, so
// this checks the room, not the drawing.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset, and the inspector at its ideal width.
private let contentColumn: CGFloat = 1400 - 248 - InspectorWidth.ideal

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

@MainActor
@Test func allFiveScopesFitBesideTheSearchAndAddWithTheInspectorOpen() {
    // Counts as a search a few weeks in has them, with Skipped the widest.
    let counts: [JobStatusFilter: Int] = [.open: 48, .later: 6, .closed: 23, .dismissed: 112]
    let scopes = TabStrip(items: JobsPage.makeScopes(counts: counts), selection: .constant(.dismissed)).tabRow
    // As PageHeader lays out its trailing side.
    let trailing = HStack(spacing: Space.s) {
        JobsHeaderTrailing(search: .constant(""), columns: .constant(TableColumnCustomization()), factColumns: []) {}
    }

    let scopesWidth = measureWidth(scopes)
    let trailingWidth = measureWidth(trailing)
    // The search field alone is this wide; less means nothing was measured.
    #expect(trailingWidth > PageHeaderMetrics.compactSearchWidth)
    #expect(scopesWidth > 0)

    let room = contentColumn - 2 * Space.l - Space.m - trailingWidth
    #expect(scopesWidth <= room, "The scopes need \(scopesWidth) pt; the header leaves them \(room) pt beside \(trailingWidth) pt of controls")
}
