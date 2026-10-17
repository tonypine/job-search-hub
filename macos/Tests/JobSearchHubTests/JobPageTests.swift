import AppKit
import Foundation
@testable import JobSearchHub
import JobSearchHubCore
import Testing

private let jobIDs = (0..<48).map { _ in UUID() }

/// The inspector over Jobs with the list's 48 jobs, the third one shown.
@MainActor
private func makeInspector(showing index: Int = 2) -> DetailsInspector {
    let details = DetailsInspector()
    details.show(.job(jobIDs[index]), from: .jobs)
    details.setJobOrder(jobIDs, on: .jobs)
    return details
}

@MainActor @Test func aJobInTheInspectorOpensAsAPageInTheListsPlace() {
    let details = makeInspector()
    #expect(details.getExpandedJob(on: .jobs) == nil)

    details.expand(on: .jobs)

    #expect(details.getExpandedJob(on: .jobs) == jobIDs[2])
    #expect(details.getPosition(on: .jobs) == "3 of 48")
    // Another page's list never shows it.
    #expect(details.getExpandedJob(on: .pipeline) == nil)
}

@MainActor @Test func onlyAJobOpensAsAPage() {
    let details = DetailsInspector()
    details.show(.company(UUID()), from: .jobs)

    details.expand(on: .jobs)

    #expect(details.expandedPage == nil)
}

@MainActor @Test func downAndUpShowTheListsNextAndPreviousJobOnTheTabShown() {
    let details = makeInspector()
    details.expand(on: .jobs)
    details.select(.session)

    details.move(by: 1, on: .jobs)
    #expect(details.getExpandedJob(on: .jobs) == jobIDs[3])
    #expect(details.getPosition(on: .jobs) == "4 of 48")
    #expect(details.history.current?.tab == .session)

    details.move(by: -1, on: .jobs)
    details.move(by: -1, on: .jobs)
    #expect(details.getPosition(on: .jobs) == "2 of 48")
}

@MainActor @Test func theListsEndsStopUpAndDown() {
    let details = makeInspector(showing: 0)
    details.expand(on: .jobs)

    #expect(details.getNeighbor(by: -1, on: .jobs) == nil)
    details.move(by: -1, on: .jobs)
    #expect(details.getExpandedJob(on: .jobs) == jobIDs[0])
}

@MainActor @Test func aJobDecidedOnItsPageGoesOnFromWhereItStood() {
    let details = makeInspector()
    details.expand(on: .jobs)
    // Pursued, it leaves the Open scope and the list reads again.
    details.setJobOrder(jobIDs.filter { $0 != jobIDs[2] }, on: .jobs)

    #expect(details.getPosition(on: .jobs) == nil)
    details.move(by: 1, on: .jobs)
    #expect(details.getExpandedJob(on: .jobs) == jobIDs[3])
    #expect(details.getPosition(on: .jobs) == "3 of 47")
}

@MainActor @Test func goingBackKeepsTheJobForTheListsSelectionAndTheInspector() {
    let details = makeInspector()
    details.expand(on: .jobs)
    details.move(by: 1, on: .jobs)

    details.collapse()

    #expect(details.getExpandedJob(on: .jobs) == nil)
    #expect(details.getEntry(on: .jobs)?.subject == .job(jobIDs[3]))
}

@MainActor @Test func aCompanyOpenedFromThePageShowsInTheInspectorBesideTheList() {
    let details = makeInspector()
    details.expand(on: .jobs)

    details.open(.company(UUID()))

    #expect(details.expandedPage == nil)
    // Back in the inspector's history, the job stays beside the list.
    details.goBack()
    #expect(details.getExpandedJob(on: .jobs) == nil)
}

@MainActor @Test func anotherPagesSelectionClosesThePage() {
    let details = makeInspector()
    details.expand(on: .jobs)

    details.show(.job(jobIDs[5]), from: .pipeline)

    #expect(details.expandedPage == nil)
    #expect(details.getExpandedJob(on: .jobs) == nil)
}

@MainActor @Test func aJobFromThePaletteShowsOnThePageAlreadyOpen() {
    let details = makeInspector()
    details.expand(on: .jobs)
    let other = UUID()

    details.show(.job(other), from: .jobs)

    #expect(details.getExpandedJob(on: .jobs) == other)
    #expect(details.getPosition(on: .jobs) == nil)
}

@MainActor @Test func hidingTheInspectorClosesThePage() {
    let details = makeInspector()
    details.expand(on: .jobs)

    details.hide()

    #expect(details.expandedPage == nil)
}

// MARK: The page's keys

private func getKey(_ keyCode: UInt16, _ characters: String?, _ modifiers: NSEvent.ModifierFlags = [], isTyping: Bool = false) -> JobPageKey? {
    JobPageKey.get(keyCode: keyCode, characters: characters, modifiers: modifiers, isTyping: isTyping)
}

@Test func upDownAndEscapeMoveThroughTheListAndGoBack() {
    // Arrow keys carry the numeric pad and function flags.
    #expect(getKey(126, "\u{F700}", [.numericPad, .function]) == .previous)
    #expect(getKey(125, "\u{F701}", [.numericPad, .function]) == .next)
    #expect(getKey(53, "\u{1B}") == .back)
    #expect(getKey(33, "[", .command) == .back)
    // ⇧↓ or ⌥↓ is not the page's.
    #expect(getKey(125, "\u{F701}", [.numericPad, .function, .shift]) == nil)
}

@Test func plainLettersGoToTheJobInEitherCase() {
    #expect(getKey(35, "p") == .letter("p"))
    #expect(getKey(37, "L", .shift) == .letter("L"))
    #expect(getKey(1, "s", .command) == nil)
    #expect(getKey(1, "s", .control) == nil)
    #expect(getKey(18, "1") == nil)
}

@Test func aFieldOrATerminalKeepsEveryKeyButCommandBracket() {
    #expect(getKey(125, "\u{F701}", [.numericPad, .function], isTyping: true) == nil)
    #expect(getKey(53, "\u{1B}", isTyping: true) == nil)
    #expect(getKey(35, "p", isTyping: true) == nil)
    #expect(getKey(33, "[", .command, isTyping: true) == .back)
}
