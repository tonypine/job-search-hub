import Foundation
import JobSearchHubCore
import Testing

private let ids = (0..<48).map { _ in UUID() }

@Test func aJobSaysWhereItStandsInTheList() {
    let list = JobPageList(order: ids)

    #expect(list.getPosition(of: ids[2]) == "3 of 48")
    #expect(list.getPosition(of: UUID()) == nil)
}

@Test func upAndDownFollowTheListsOrderAndStopAtItsEnds() {
    let list = JobPageList(order: ids)

    #expect(list.getNeighbor(of: ids[2], by: 1) == ids[3])
    #expect(list.getNeighbor(of: ids[2], by: -1) == ids[1])
    #expect(list.getNeighbor(of: ids[0], by: -1) == nil)
    #expect(list.getNeighbor(of: ids[47], by: 1) == nil)
}

@Test func aJobThatLeftTheListGoesOnFromWhereItStood() {
    var list = JobPageList(order: ids)
    list.show(ids[2])
    // Pursued, it leaves the Open scope.
    list.setOrder(ids.filter { $0 != ids[2] }, showing: ids[2])

    #expect(list.getPosition(of: ids[2]) == nil)
    #expect(list.getNeighbor(of: ids[2], by: 1) == ids[3])
    #expect(list.getNeighbor(of: ids[2], by: -1) == ids[1])
}

@Test func theLastJobLeavingTheListHasNoNext() {
    var list = JobPageList(order: Array(ids.prefix(3)))
    list.show(ids[2])
    list.setOrder(Array(ids.prefix(2)), showing: ids[2])

    #expect(list.getNeighbor(of: ids[2], by: 1) == nil)
    #expect(list.getNeighbor(of: ids[2], by: -1) == ids[1])
}

@Test func aJobNeverListedHasNoNeighbors() {
    let list = JobPageList(order: ids)
    let other = UUID()

    #expect(list.getNeighbor(of: other, by: 1) == nil)
    #expect(list.getNeighbor(of: other, by: -1) == nil)
}

@Test func aReorderedListMovesTheJobsPlace() {
    var list = JobPageList(order: ids)
    list.show(ids[2])
    list.setOrder(Array(ids.reversed()), showing: ids[2])

    #expect(list.lastIndex == 45)
    #expect(list.getPosition(of: ids[2]) == "46 of 48")
    #expect(list.getNeighbor(of: ids[2], by: 1) == ids[1])
}
