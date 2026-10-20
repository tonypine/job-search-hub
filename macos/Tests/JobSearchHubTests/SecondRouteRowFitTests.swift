import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// An overdue Pipeline card's second route, measured offscreen in a hosting
// view that no window holds. In a 1400 pt window, TP-659's epic review found
// it cut to "Write to Alex…" where the design shows "Write to Alex Kim"
// (TP-1104): the name sat in a borderless menu's label, which asks about
// 160 pt for it, and the card has 163 pt for the whole row, ✓ included.
// This checks the row's own width against the room the card leaves it.

/// The board in a 1400 pt window: what the sidebar, 248 pt with its inset,
/// leaves, and with the inspector open at its ideal 480 pt, where the
/// columns keep their minimum and the board scrolls.
private let boardWidths: [CGFloat] = [1400 - 248, 1400 - 248 - 480]

private func makePeople(_ names: [String]) throws -> [RelatedPerson] {
    let people = names.enumerated().map { index, name in
        #"""
        {"key":"connection:dddddddd-0000-0000-0000-00000000000\#(index)","relation":"connection",
         "id":"dddddddd-0000-0000-0000-00000000000\#(index)","name":"\#(name)","role":"Engineering Manager",
         "company_name":"Globex","is_agency":false,"open_jobs":0,"fitting_jobs":0}
        """#
    }
    let json = #"{"people":[\#(people.joined(separator: ","))]}"#
    return try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(json.utf8)).people
}

@MainActor
private func makeRow(_ people: [RelatedPerson]) -> SecondRouteRow {
    SecondRouteRow(candidates: people, isFindingPeople: false, canFindPeople: true, onWrite: { _ in }, onFindPeople: {}, onFollowUp: {})
}

@MainActor
private func measureWidth(_ view: some View) -> CGFloat {
    NSHostingView(rootView: view.fixedSize()).fittingSize.width
}

@MainActor
@Test func writeToAlexKimFitsAnOverdueCardOnA1400PointBoard() throws {
    let name = Label("Write to Alex Kim", systemImage: "paperplane").font(.hubCaption)
    let nameWidth = measureWidth(name)
    // Less than this means nothing was measured.
    #expect(nameWidth > 0)

    for boardWidth in boardWidths {
        // Saved, Applied, In contact, Interviewing and Offer, with the Closed drawer.
        let columnWidth = PipelinePage.getColumnWidth(availableWidth: boardWidth, columnCount: 5, hasDrawer: true)
        let room = PipelineCardView.getContentWidth(columnWidth: columnWidth)
        for people in [try makePeople(["Alex Kim"]), try makePeople(["Alex Kim", "Riley Example", "Morgan Example"])] {
            let rowWidth = measureWidth(makeRow(people))
            #expect(rowWidth > nameWidth)
            #expect(rowWidth <= room, "On a \(boardWidth) pt board with \(people.count) people the row needs \(rowWidth) pt; the card leaves it \(room) pt")
        }
    }
}
