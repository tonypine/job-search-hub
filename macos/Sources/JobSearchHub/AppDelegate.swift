import AppKit
import UserNotifications

/// Shows the app's notifications as banners even while the app is in front;
/// without a delegate, macOS keeps them out of sight. Also answers Control-
/// Command-F with full screen.
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    private var fullScreenShortcutMonitor: Any?

    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().delegate = self
        fullScreenShortcutMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown, handler: Self.toggleFullScreenOnShortcut)
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
