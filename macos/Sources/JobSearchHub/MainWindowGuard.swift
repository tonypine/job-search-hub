import AppKit
import JobSearchHubCore
import SwiftUI

/// Watches the main window's size from the view's background. It says
/// whether the window has room for the pages and the inspector, and grows a
/// window the window server left too small, such as one opened while the
/// screen was locked, once it can: when it attaches, becomes key, changes
/// screen, or the screen unlocks. It never grows one on a resize, so it
/// can't fight the window server over the frame.
struct MainWindowGuard: NSViewRepresentable {
    let onRoomChange: (Bool) -> Void

    func makeNSView(context: Context) -> GuardView {
        GuardView(onRoomChange: onRoomChange)
    }

    func updateNSView(_ view: GuardView, context: Context) {
        view.onRoomChange = onRoomChange
    }

    final class GuardView: NSView {
        var onRoomChange: (Bool) -> Void
        private var hasRoom: Bool?
        private var observers: [(NotificationCenter, NSObjectProtocol)] = []

        init(onRoomChange: @escaping (Bool) -> Void) {
            self.onRoomChange = onRoomChange
            super.init(frame: .zero)
        }

        @available(*, unavailable)
        required init?(coder: NSCoder) {
            fatalError("init(coder:) has not been implemented")
        }

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            for (center, observer) in observers {
                center.removeObserver(observer)
            }
            observers = []
            guard let window else { return }
            observe(NSWindow.didResizeNotification, of: window) { $0.reportRoom() }
            observe(NSWindow.didBecomeKeyNotification, of: window) { $0.growIfTooSmall() }
            observe(NSWindow.didChangeScreenNotification, of: window) { $0.growIfTooSmall() }
            observe(Notification.Name("com.apple.screenIsUnlocked"), in: DistributedNotificationCenter.default()) { $0.growIfTooSmall() }
            // Out of the layout pass that attached the view.
            DispatchQueue.main.async { [weak self] in
                self?.growIfTooSmall()
                self?.reportRoom()
            }
        }

        private func observe(
            _ name: Notification.Name, of window: NSWindow? = nil, in center: NotificationCenter = .default,
            perform: @escaping @MainActor @Sendable (GuardView) -> Void
        ) {
            let observer = center.addObserver(forName: name, object: window, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated {
                    if let self { perform(self) }
                }
            }
            observers.append((center, observer))
        }

        private func growIfTooSmall() {
            guard let window, let screen = window.screen,
                  let frame = MainWindow.grownFrame(window.frame, within: screen.visibleFrame)
            else { return }
            window.setFrame(frame, display: true, animate: false)
        }

        /// Tells the content only when the answer changes, and after the
        /// resize's layout, so the inspector never comes and goes within one.
        private func reportRoom() {
            guard let window else { return }
            let hasRoom = MainWindow.hasRoom(window.frame.size)
            guard hasRoom != self.hasRoom else { return }
            self.hasRoom = hasRoom
            DispatchQueue.main.async { [weak self] in
                self?.onRoomChange(hasRoom)
            }
        }
    }
}
