import AppKit
import JobSearchHubCore
import SwiftUI
import UserNotifications

/// Shows the app's notifications as banners even while the app is in front;
/// without a delegate, macOS keeps them out of sight. Also answers Control-
/// Command-F with full screen, and opens the main window when launch ends
/// without one.
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    let mainWindow = MainWindowOpener()
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
            openMainWindowIfNone()
        }
    }

    @MainActor private func openMainWindowIfNone() {
        let windows = NSApp.windows.map {
            LaunchWindow(isVisible: $0.isVisible, isMiniaturized: $0.isMiniaturized, canBecomeMain: $0.canBecomeMain)
        }
        if LaunchWindows.needsMainWindow(windows, isAppHidden: NSApp.isHidden) {
            mainWindow.open()
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

/// Opens the main window from outside SwiftUI's views. The menu commands
/// hand over SwiftUI's open-window action, which they get even while no
/// window is open.
@MainActor final class MainWindowOpener {
    /// The main `WindowGroup`'s id, which keeps its identity, and so its
    /// saved window state, from changing with the content's modifiers.
    static let sceneID = "main"
    private var openWindow: OpenWindowAction?

    func remember(_ openWindow: OpenWindowAction) {
        self.openWindow = openWindow
    }

    func open() {
        openWindow?(id: Self.sceneID)
    }
}
