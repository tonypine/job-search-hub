import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// The Pipeline header beside the inspector, measured offscreen in a hosting
// view that no window holds, as JobsHeaderFitTests does for Jobs. TP-659's
// epic review found the header held only the scopes, where the design puts
// the search and Add on the right (P1, TP-1124); this checks that all four
// scopes still fit beside them with a card selected in a 1400 pt window.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset, and the inspector at its ideal 480 pt.
private let contentColumn: CGFloat = 1400 - 248 - 480

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

@MainActor
@Test func allFourScopesFitBesideTheSearchAndAddWithTheInspectorOpen() {
    // Counts as a search a few weeks in has them, with Skipped the widest.
    let counts: [PipelineScope: Int] = [.active: 48, .due: 12, .closed: 23, .skipped: 112]
    let scopes = TabStrip(items: PipelinePage.makeScopes(counts: counts), selection: .constant(.skipped)).tabRow
    // As PageHeader lays out its trailing side.
    let trailing = HStack(spacing: Space.s) {
        PipelineHeaderTrailing(search: .constant("")) {}
    }

    let scopesWidth = measureWidth(scopes)
    let trailingWidth = measureWidth(trailing)
    // The search field alone is this wide; less means the search or Add is missing.
    #expect(trailingWidth > PageHeaderMetrics.searchWidth)
    #expect(scopesWidth > 0)

    let room = contentColumn - 2 * Space.l - Space.m - trailingWidth
    #expect(scopesWidth <= room, "The scopes need \(scopesWidth) pt; the header leaves them \(room) pt beside \(trailingWidth) pt of controls")
}

@MainActor
@Test func theScopesCarryTheirCountsInOrder() {
    let items = PipelinePage.makeScopes(counts: [.active: 7, .due: 3, .closed: 1, .skipped: 0])

    #expect(items.map(\.title) == ["Active", "Due", "Closed", "Skipped"])
    #expect(items.map(\.count) == [7, 3, 1, 0])
}
