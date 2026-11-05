import AppKit
@testable import JobSearchHub
import SwiftUI
import Testing

// The detail column, measured offscreen in a hosting controller that no window
// holds. A column that asks for more than it's offered grows the split view
// past the window, which clipped the sidebar's foot under the withdrawn
// banner (TP-592).

private let offered = NSSize(width: 1000, height: 870)

@MainActor
private func measureHeight(_ view: some View) -> CGFloat {
    NSHostingController(rootView: view).sizeThatFits(in: offered).height
}

@MainActor
@Test func theColumnTakesTheHeightItsOfferedWithABanner() {
    let column = DetailColumn {
        Color.clear.frame(height: 52)
    } content: {
        Color.clear
    }

    #expect(measureHeight(column) == offered.height)
}

@MainActor
@Test func aPageThatAsksForMoreThanTheWindowDoesntGrowTheColumn() {
    let column = DetailColumn {
        Color.clear.frame(height: 52)
    } content: {
        Color.clear.frame(minHeight: 2000)
    }

    #expect(measureHeight(column) == offered.height)
}

@MainActor
@Test func theColumnTakesTheHeightItsOfferedWithoutABanner() {
    let column = DetailColumn {
        EmptyView()
    } content: {
        Color.clear
    }

    #expect(measureHeight(column) == offered.height)
}
