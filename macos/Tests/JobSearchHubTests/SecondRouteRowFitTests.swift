import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// An overdue Pipeline card's second route, measured offscreen in a hosting
// view that no window holds. In a 1400 pt window, TP-659's epic review found
// it cut to "Write to Alex…" where the design shows "Write to Alex Kim", and
// QA then found "Write to Dana Whitfi…" beside a chevron (TP-1104): the card
// has 163 pt for the whole row, ✓ included. These check the row against the
// room the card leaves it: one line where the name fits, and a second line,
// not a cut name, where it doesn't.

/// The board in a 1400 pt window: what the sidebar, 248 pt with its inset,
/// leaves.
private let boardWidth: CGFloat = 1400 - 248
/// The same with the inspector open at its ideal width, where the columns
/// keep their minimum and the board scrolls.
private let boardWidthBesideInspector: CGFloat = boardWidth - InspectorWidth.ideal

/// The width a card's lines get on a board that wide: Saved, Applied, In
/// contact, Interviewing and Offer, with the Closed drawer.
@MainActor
private func getRoom(boardWidth: CGFloat) -> CGFloat {
    let columnWidth = PipelinePage.getColumnWidth(availableWidth: boardWidth, columnCount: 5, hasDrawer: true)
    return PipelineCardView.getContentWidth(columnWidth: columnWidth)
}

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
private func measure(_ view: some View) -> CGSize {
    NSHostingView(rootView: view).fittingSize
}

@MainActor
@Test func theFullNameFitsOneLineOfAnOverdueCardOnA1400PointBoard() throws {
    let room = getRoom(boardWidth: boardWidth)
    // The design's name, and the first person at the seeded hub's overdue
    // companies with one person and with three.
    for name in ["Alex Kim", "Dana Whitfield", "Ines Marchetti"] {
        let nameWidth = measure(Label("Write to \(name)", systemImage: "paperplane").font(.hubCaption).fixedSize()).width
        for people in [try makePeople([name]), try makePeople([name, "Priya Castellanos", "Tomasz Ellery"])] {
            let rowWidth = measure(makeRow(people).fixedSize()).width
            // Less than the name means nothing was measured.
            #expect(rowWidth > nameWidth)
            #expect(rowWidth <= room, "With \(people.count) people the row needs \(rowWidth) pt for \(name); the card leaves it \(room) pt")
        }
    }
}

@MainActor
@Test func aNameTooLongForTheLineWrapsRatherThanBeingCut() throws {
    let cases: [(board: CGFloat, name: String)] = [
        (boardWidthBesideInspector, "Dana Whitfield"),
        (boardWidthBesideInspector, "Ines Marchetti"),
        (boardWidth, "Maximiliana Featherstonehaugh"),
    ]
    for (board, name) in cases {
        let room = getRoom(boardWidth: board)
        let row = makeRow(try makePeople([name, "Priya Castellanos"]))
        let oneLine = measure(row.fixedSize())
        #expect(oneLine.width > room, "\(name) fits one line of \(room) pt, so this checks nothing")
        // A cut name would keep the row one line tall.
        let atRoom = measure(row.frame(width: room))
        #expect(atRoom.height > oneLine.height * 1.5, "At \(room) pt the row for \(name) is \(atRoom.height) pt tall; one line is \(oneLine.height) pt")
    }
}
