import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// Today's two columns of cards beside the inspector, measured offscreen in a
// hosting controller that no window holds. In a 1400 pt window with a Decide
// card open in the 420 pt inspector, TP-659's epic review found the cards
// stayed in two columns that needed 855 pt of the 732 pt left, and the split
// view grew past the window: the sidebar cut on the left, the inspector's
// buttons off the right (TP-1134). These check the columns stack there, stay
// side by side without the inspector, and never ask for the two columns'
// widths together.

/// The content column in a 1400 pt window: the sidebar as the window draws
/// it, 248 pt with its inset.
private let contentColumn: CGFloat = 1400 - 248

/// The width Today's columns get: the page less its padding.
private func getColumnsWidth(besideInspector: Bool) -> CGFloat {
    (besideInspector ? contentColumn - InspectorWidth.ideal : contentColumn) - 2 * Space.xl
}

/// What the view takes when offered this width.
@MainActor
private func measure(_ view: some View, width: CGFloat) -> CGSize {
    NSHostingController(rootView: view).sizeThatFits(in: CGSize(width: width, height: .infinity))
}

/// A column that can't be narrower than `minWidth`, as a card's row of
/// buttons can't.
private func makeColumn(minWidth: CGFloat, height: CGFloat = 100) -> some View {
    Color.clear.frame(minWidth: minWidth, maxWidth: .infinity, minHeight: height, maxHeight: height)
}

@MainActor
@Test func theColumnsStackBesideTheInspector() {
    let width = getColumnsWidth(besideInspector: true)
    let columns = TodayColumns(spacing: Space.l) {
        makeColumn(minWidth: 395)
        makeColumn(minWidth: 395)
    }

    let size = measure(columns, width: width)

    #expect(size.width == width, "Today's columns take \(size.width) pt of the \(width) pt beside the inspector")
    #expect(size.height == 2 * 100 + Space.l)
}

@MainActor
@Test func theColumnsStaySideBySideWithoutTheInspector() {
    let width = getColumnsWidth(besideInspector: false)
    let columns = TodayColumns(spacing: Space.l) {
        makeColumn(minWidth: 395)
        makeColumn(minWidth: 395, height: 140)
    }

    let size = measure(columns, width: width)

    #expect(size.width == width)
    #expect(size.height == 140)
}

@MainActor
@Test func theColumnsAskForOneColumnsWidthNotBoth() {
    let columns = TodayColumns(spacing: Space.l) {
        makeColumn(minWidth: 395)
        makeColumn(minWidth: 300)
    }

    #expect(measure(columns, width: 0).width == 395)
}

@MainActor
@Test func theColumnsStackBelowTheTwoColumnWidthWhateverTheyNeed() {
    let columns = TodayColumns(spacing: Space.l) {
        makeColumn(minWidth: 100)
        makeColumn(minWidth: 100)
    }

    #expect(measure(columns, width: TodayColumns.twoColumnWidth - 1).height == 2 * 100 + Space.l)
    #expect(measure(columns, width: TodayColumns.twoColumnWidth).height == 100)
}

@MainActor
@Test func anEmptyColumnLeavesNoGapWhenStacked() {
    let columns = TodayColumns(spacing: Space.l) {
        makeColumn(minWidth: 395)
        VStack(spacing: Space.l) {}
    }

    #expect(measure(columns, width: getColumnsWidth(besideInspector: true)).height == 100)
}

@MainActor
@Test func decideCardsFitBesideTheInspector() throws {
    // A job left for later shows Pursue and Skip…; one not yet decided shows
    // Later too, the widest row of buttons.
    let item = try DecisionQueueRowSnapshots.makeItem(
        .possible, title: "Senior Platform Engineer, Developer Experience",
        reason: "Owns the internal developer platform and its tooling, which the brief puts first."
    )
    func makeCard() -> some View {
        TodayDecisionRow(item: item, isOpen: true, open: {}, decide: { _ in }, skip: {}).hubCard()
    }
    let width = getColumnsWidth(besideInspector: true)
    let card = measure(makeCard(), width: 0).width
    // Pursue, Later and Skip… alone take more than this.
    #expect(card > 100)
    #expect(card <= width, "A Decide card needs \(card) pt; Today has \(width) pt beside the inspector")

    let columns = TodayColumns(spacing: Space.l) {
        VStack(spacing: Space.l) { makeCard() }
        VStack(spacing: Space.l) { makeCard() }
    }
    // However they arrange, they ask for no more than the page has.
    #expect(measure(columns, width: 0).width <= width)
    #expect(measure(columns, width: width).width == width)
}
