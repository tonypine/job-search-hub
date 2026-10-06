/// What the launch check needs to know about one of the app's windows.
public struct LaunchWindow: Equatable, Sendable {
    public let isVisible: Bool
    public let isMiniaturized: Bool
    /// False for panels and the menu bar's own windows.
    public let canBecomeMain: Bool

    public init(isVisible: Bool, isMiniaturized: Bool, canBecomeMain: Bool) {
        self.isVisible = isVisible
        self.isMiniaturized = isMiniaturized
        self.canBecomeMain = canBecomeMain
    }
}

public enum LaunchWindows {
    /// Whether launch ended with no window to work in, which happens when
    /// macOS restores window state saved by a build whose main scene
    /// differs: SwiftUI then opens no window of its own. A minimized window
    /// counts, since it waits in the Dock, and so does every window while
    /// the app launched hidden.
    public static func needsMainWindow(_ windows: [LaunchWindow], isAppHidden: Bool) -> Bool {
        !windows.contains { $0.canBecomeMain && ($0.isVisible || $0.isMiniaturized || isAppHidden) }
    }
}
