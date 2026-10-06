import JobSearchHubCore
import Testing

private let shown = LaunchWindow(isVisible: true, isMiniaturized: false, canBecomeMain: true)
private let minimized = LaunchWindow(isVisible: false, isMiniaturized: true, canBecomeMain: true)
private let orderedOut = LaunchWindow(isVisible: false, isMiniaturized: false, canBecomeMain: true)
private let panel = LaunchWindow(isVisible: true, isMiniaturized: false, canBecomeMain: false)

@Test func aLaunchWithNoWindowNeedsTheMainWindow() {
    #expect(LaunchWindows.needsMainWindow([], isAppHidden: false))
}

@Test func aRestoredWindowIsEnough() {
    #expect(!LaunchWindows.needsMainWindow([shown], isAppHidden: false))
    #expect(!LaunchWindows.needsMainWindow([orderedOut, shown], isAppHidden: false))
}

@Test func aMinimizedWindowIsEnough() {
    #expect(!LaunchWindows.needsMainWindow([minimized], isAppHidden: false))
}

@Test func windowsOutOfSightDontCount() {
    #expect(LaunchWindows.needsMainWindow([orderedOut], isAppHidden: false))
}

@Test func panelsDontCount() {
    #expect(LaunchWindows.needsMainWindow([panel], isAppHidden: false))
}

@Test func aHiddenAppKeepsTheWindowsItRestored() {
    #expect(!LaunchWindows.needsMainWindow([orderedOut], isAppHidden: true))
    #expect(LaunchWindows.needsMainWindow([], isAppHidden: true))
}
