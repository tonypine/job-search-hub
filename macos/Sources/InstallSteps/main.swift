import AppKit
import JobSearchHubCore
import SwiftUI

// hub-install-steps <state.json>: the small window hub-update keeps on
// screen while the app is closed for an install. It reads the install's
// state twice a second and shows its steps, and goes once the new app has
// opened, or a few seconds after the install ended. A rollback that stopped
// stays up, with where it stopped, until closed.

/// The steps, from the state.
struct InstallStepsView: View {
    let icon: NSImage
    let progress: InstallProgress
    let failure: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Image(nsImage: icon)
                    .resizable()
                    .frame(width: 40, height: 40)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text(progress.title).font(.headline)
                    Text(progress.subtitle).font(.subheadline).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            VStack(alignment: .leading, spacing: 6) {
                ForEach(Array(progress.rows.enumerated()), id: \.offset) { _, row in
                    HStack(spacing: 8) {
                        symbol(row.status).frame(width: 16)
                        Text(row.title)
                            .fontWeight(row.status == .current ? .semibold : .regular)
                            .foregroundStyle(row.status == .waiting ? .secondary : .primary)
                    }
                }
            }
            if let failure {
                Text(failure).font(.callout).foregroundStyle(.secondary).textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
            ProgressView(value: progress.fraction)
        }
        .padding(20)
        .frame(width: 380, alignment: .leading)
    }

    @ViewBuilder
    private func symbol(_ status: InstallProgressRow.Status) -> some View {
        switch status {
        case .done: Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
        case .current: ProgressView().controlSize(.small)
        case .waiting: Image(systemName: "circle").foregroundStyle(.secondary)
        case .failed: Image(systemName: "xmark.circle.fill").foregroundStyle(.red)
        }
    }
}

@MainActor
final class StepsWindow: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let stateURL: URL
    let launchedURL: URL
    let window: NSWindow
    let hosting: NSHostingView<InstallStepsView>
    var timer: Timer?
    var endedAt: Date?
    /// The app's icon, from the bundle this command sits in, at
    /// Contents/Helpers/bin.
    let icon: NSImage = {
        let bundle = URL(filePath: CommandLine.arguments[0]).resolvingSymlinksInPath()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        return bundle.pathExtension == "app" ? NSWorkspace.shared.icon(forFile: bundle.path) : NSApp.applicationIconImage
    }()

    init(stateURL: URL) {
        self.stateURL = stateURL
        launchedURL = stateURL.deletingLastPathComponent().appending(path: "launched")
        hosting = NSHostingView(rootView: InstallStepsView(
            icon: NSImage(), progress: InstallProgress(title: "Installing Job Search Hub", subtitle: "", rows: [], fraction: 0), failure: nil
        ))
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 380, height: 220), styleMask: [.titled, .closable], backing: .buffered, defer: false)
        super.init()
        window.title = "Job Search Hub"
        window.contentView = hosting
        window.level = .floating
        window.isReleasedWhenClosed = false
        window.delegate = self
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        update()
        window.setContentSize(hosting.fittingSize)
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate()
        timer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in
            MainActor.assumeIsolated { self.update() }
        }
    }

    func windowWillClose(_ notification: Notification) {
        NSApp.terminate(nil)
    }

    private func update() {
        guard let data = try? Data(contentsOf: stateURL), let state = try? InstallState.decode(data) else {
            // The app moved the state aside once it read how the install ended.
            NSApp.terminate(nil)
            return
        }
        let failure = state.step == .rollbackFailed ? [state.error, "Job Search Hub shows the commands that finish it."].compactMap { $0 }.joined(separator: "\n") : nil
        hosting.rootView = InstallStepsView(icon: icon, progress: InstallProgress.make(state), failure: failure)
        window.setContentSize(hosting.fittingSize)
        // The new app is up: it says the rest.
        if state.step == .checkingApp, let mark = try? String(contentsOf: launchedURL, encoding: .utf8),
           mark.trimmingCharacters(in: .whitespacesAndNewlines) == state.to {
            NSApp.terminate(nil)
            return
        }
        guard state.step.isFinished, state.step != .rollbackFailed else { return }
        let ended = endedAt ?? .now
        endedAt = ended
        if Date.now.timeIntervalSince(ended) > 4 {
            NSApp.terminate(nil)
        }
    }
}

let arguments = CommandLine.arguments
guard arguments.count == 2 else {
    FileHandle.standardError.write(Data("usage: hub-install-steps <state.json>\n".utf8))
    exit(2)
}
let application = NSApplication.shared
application.setActivationPolicy(.accessory)
let steps = StepsWindow(stateURL: URL(filePath: arguments[1]))
application.delegate = steps
application.run()
