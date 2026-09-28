import AppKit
import JobSearchHubCore
import SwiftTerm

enum ClaudeSessionLaunchError: LocalizedError {
    case claudeNotFound

    var errorDescription: String? {
        "Claude Code is not installed where its installers put it (~/.local/bin/claude, Homebrew)."
    }
}

/// The Claude sessions running inside the app, one terminal each.
///
/// The terminals live here rather than in the views that show them: a view
/// SwiftUI owns goes away when another job is selected or the window closes,
/// and its process would go with it. Views only borrow a terminal while on
/// screen. What counts as running is what has a live process here, never a
/// stored flag.
@MainActor
@Observable
final class ClaudeSessionHost {
    static let shared = ClaudeSessionHost()

    private(set) var runningSessionIDs: Set<UUID> = []
    @ObservationIgnored private var terminals: [UUID: LocalProcessTerminalView] = [:]
    @ObservationIgnored private var watchers: [UUID: ProcessEndWatcher] = [:]

    func isRunning(_ sessionID: UUID) -> Bool {
        runningSessionIDs.contains(sessionID)
    }

    func getTerminal(for sessionID: UUID) -> LocalProcessTerminalView? {
        terminals[sessionID]
    }

    /// Writes the session's context to the sessions folder, runs `claude` there through
    /// a login shell so the owner's whole setup loads, and tells the hub it
    /// started. A session with a conversation is resumed.
    func start(_ session: ClaudeSession, with client: HubClient) async throws {
        guard !isRunning(session.id) else { return }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else { throw ClaudeSessionLaunchError.claudeNotFound }
        let context = try await client.get("v1/claude-sessions/\(session.id.uuidString)/context", as: ClaudeSessionContext.self)

        let fileManager = FileManager.default
        let applicationSupport = try fileManager.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
        let folder = ClaudeLaunch.getWorkingDirectory(applicationSupport: applicationSupport)
        try fileManager.createDirectory(at: folder, withIntermediateDirectories: true)
        try context.context.write(to: folder.appending(path: ClaudeLaunch.getContextFileName(for: session)), atomically: true, encoding: .utf8)
        let transcript = ClaudeLaunch.getTranscriptURL(for: session, workingDirectory: folder, home: fileManager.homeDirectoryForCurrentUser)
        let command = ClaudeLaunch.getShellCommand(claude: claude, session: session, hasConversation: fileManager.fileExists(atPath: transcript.path))

        let terminal = LocalProcessTerminalView(frame: NSRect(x: 0, y: 0, width: 640, height: 480))
        let watcher = ProcessEndWatcher { [weak self] in self?.handleEnd(of: session.id, client: client) }
        terminal.processDelegate = watcher
        var environment = ProcessInfo.processInfo.environment
        environment["TERM"] = "xterm-256color"
        environment["COLORTERM"] = "truecolor"
        terminal.startProcess(
            executable: "/bin/zsh", args: ["-l", "-i", "-c", command],
            environment: environment.map { "\($0.key)=\($0.value)" }, execName: nil, currentDirectory: folder.path
        )
        terminals[session.id] = terminal
        watchers[session.id] = watcher
        runningSessionIDs.insert(session.id)
        _ = try? await client.send("POST", "v1/claude-sessions/\(session.id.uuidString)/start", body: EmptyRequest(), as: ClaudeSession.self)
    }

    /// Ends the session's process; the end is recorded when it happens.
    func stop(_ sessionID: UUID) {
        terminals[sessionID]?.terminate()
    }

    private func handleEnd(of sessionID: UUID, client: HubClient) {
        terminals[sessionID] = nil
        watchers[sessionID] = nil
        runningSessionIDs.remove(sessionID)
        Task { _ = try? await client.send("POST", "v1/claude-sessions/\(sessionID.uuidString)/stop", body: EmptyRequest(), as: ClaudeSession.self) }
    }
}

private struct EmptyRequest: Encodable {}

/// Tells the host when a session's process ends.
private final class ProcessEndWatcher: LocalProcessTerminalViewDelegate {
    private let onEnd: @MainActor @Sendable () -> Void

    init(onEnd: @escaping @MainActor @Sendable () -> Void) {
        self.onEnd = onEnd
    }

    func processTerminated(source: TerminalView, exitCode: Int32?) {
        Task { @MainActor [onEnd] in onEnd() }
    }

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}
    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
}
