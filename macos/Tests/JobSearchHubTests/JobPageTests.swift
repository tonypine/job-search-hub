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
