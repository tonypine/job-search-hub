import AppKit
import JobSearchHubCore
import UserNotifications

/// Shows the app's notifications as banners even while the app is in front;
/// without a delegate, macOS keeps them out of sight. Also answers Control-
/// Command-F with full screen, and opens the main window when launch ends
/// without one.
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    private var fullScreenShortcutMonitor: Any?

    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().delegate = self
        fullScreenShortcutMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown, handler: Self.toggleFullScreenOnShortcut)
        // Window state saved by a build whose main scene differs restores
        // nothing, and SwiftUI then opens no window: the app runs with none
        // until the Dock icon is clicked. A second on, once restoration is
        // done, the app opens the main window itself if none shows.
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(1))
            Self.openMainWindowIfNone()
        }
    }

    /// Opens the main window through SwiftUI's own app delegate, which is
    /// `NSApp.delegate` and opens a window of the main scene on
    /// `showNewMainWindow:`. This stays out of the views and the menu
    /// commands, whose updates feed the toolbar. Should macOS drop that
    /// action, the public untitled-file call opens the app's initial window.
    @MainActor private static func openMainWindowIfNone() {
        let windows = NSApp.windows.map {
            LaunchWindow(isVisible: $0.isVisible, isMiniaturized: $0.isMiniaturized, canBecomeMain: $0.canBecomeMain)
        }
        guard LaunchWindows.needsMainWindow(windows, isAppHidden: NSApp.isHidden) else { return }
        if !NSApp.sendAction(Selector(("showNewMainWindow:")), to: nil, from: nil) {
            _ = NSApp.delegate?.applicationOpenUntitledFile?(NSApp)
        }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .list, .sound]
    }

    /// macOS 26 binds View › Enter Full Screen to Globe-F only, while other
    /// apps still answer Control-Command-F. The monitor sees the key before
    /// any view, so it works while an embedded terminal has focus too.
    private static func toggleFullScreenOnShortcut(_ event: NSEvent) -> NSEvent? {
        let modifiers = event.modifierFlags.intersection([.control, .command, .option, .shift])
        guard modifiers == [.control, .command], event.charactersIgnoringModifiers?.lowercased() == "f",
              let window = NSApp.keyWindow ?? NSApp.mainWindow
        else { return event }
        window.toggleFullScreen(nil)
        return nil
    }
}
