import CoreGraphics
import JobSearchHubCore
import Testing

private let screen = CGRect(x: 0, y: 0, width: 1728, height: 1080)

@Test func theMinimumHasRoom() {
    #expect(MainWindow.hasRoom(MainWindow.minimumSize))
    #expect(MainWindow.hasRoom(MainWindow.defaultSize))
}

@Test func aNarrowOrShortWindowHasNoRoom() {
    #expect(!MainWindow.hasRoom(CGSize(width: 102, height: 98)))
    #expect(!MainWindow.hasRoom(CGSize(width: 899, height: 860)))
    #expect(!MainWindow.hasRoom(CGSize(width: 1400, height: 599)))
}

@Test func aWindowWithRoomStaysAsItIs() {
    #expect(MainWindow.grownFrame(CGRect(x: 100, y: 100, width: 900, height: 600), within: screen) == nil)
}

@Test func aTinyWindowGrowsToTheDefaultSizeFromItsTopLeft() {
    let tiny = CGRect(x: 200, y: 800, width: 102, height: 98)
    #expect(MainWindow.grownFrame(tiny, within: screen) == CGRect(x: 200, y: 898 - 860, width: 1400, height: 860))
}

@Test func aGrownWindowStaysOnScreen() {
    let nearTheCorner = CGRect(x: 1600, y: 10, width: 102, height: 98)
    #expect(MainWindow.grownFrame(nearTheCorner, within: screen) == CGRect(x: 328, y: 0, width: 1400, height: 860))
}

@Test func aSmallScreenCapsTheGrownWindow() {
    let small = CGRect(x: 0, y: 25, width: 1280, height: 775)
    let tiny = CGRect(x: 40, y: 600, width: 102, height: 98)
    #expect(MainWindow.grownFrame(tiny, within: small) == CGRect(x: 0, y: 25, width: 1280, height: 775))
}

@Test func aScreenTooSmallForTheMinimumLeavesTheWindow() {
    let lockedScreen = CGRect(x: 0, y: 0, width: 102, height: 98)
    #expect(MainWindow.grownFrame(CGRect(x: 0, y: 0, width: 102, height: 98), within: lockedScreen) == nil)
}
