import AppKit
import JobSearchHubCore
import SwiftTerm
import UserNotifications

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
    /// What each running session is doing, as its hooks last reported.
    private(set) var activities: [UUID: SessionActivity] = [:]
    /// The sessions a pane is showing; they raise no notifications.
    var shownSessionIDs: Set<UUID> = []
    @ObservationIgnored private var terminals: [UUID: LocalProcessTerminalView] = [:]
    @ObservationIgnored private var watchers: [UUID: ProcessEndWatcher] = [:]
    @ObservationIgnored private var names: [UUID: String] = [:]
    @ObservationIgnored private var stateFiles: [UUID: URL] = [:]
    @ObservationIgnored private var activityTimer: Timer?

    func isRunning(_ sessionID: UUID) -> Bool {
        runningSessionIDs.contains(sessionID)
    }

    func getTerminal(for sessionID: UUID) -> LocalProcessTerminalView? {
        terminals[sessionID]
    }

    /// Writes the session's context to the sessions folder, runs `claude` there through
    /// a login shell so the owner's whole setup loads, and tells the hub it
    /// started. A session with a conversation is resumed.
    func start(_ session: ClaudeSession, with client: HubClient, firstMessage: String? = nil) async throws {
        guard !isRunning(session.id) else { return }
        guard let claude = ClaudeLaunch.findClaudeExecutable() else { throw ClaudeSessionLaunchError.claudeNotFound }
        let context = try await client.get("v1/claude-sessions/\(session.id.uuidString)/context", as: ClaudeSessionContext.self)

        let fileManager = FileManager.default
        let applicationSupport = try fileManager.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
        let folder = ClaudeLaunch.getWorkingDirectory(applicationSupport: applicationSupport)
        try fileManager.createDirectory(at: folder, withIntermediateDirectories: true)
        try context.context.write(to: folder.appending(path: ClaudeLaunch.getContextFileName(for: session)), atomically: true, encoding: .utf8)
        try ClaudeHooks.getSettingsJSON().write(to: folder.appending(path: ClaudeHooks.settingsFileName), atomically: true, encoding: .utf8)
        let states = applicationSupport.appending(path: "JobSearchHub/SessionStates", directoryHint: .isDirectory)
        try fileManager.createDirectory(at: states, withIntermediateDirectories: true)
        let stateFile = states.appending(path: "\(session.id.uuidString.lowercased()).state")
        try? fileManager.removeItem(at: stateFile)
        let transcript = ClaudeLaunch.getTranscriptURL(for: session, workingDirectory: folder, home: fileManager.homeDirectoryForCurrentUser)
        let readableFolder = UserDefaults.standard.string(forKey: ClaudeLaunch.readableFolderKey)
            ?? ClaudeLaunch.getDefaultReadableFolder(home: fileManager.homeDirectoryForCurrentUser)
        let command = ClaudeLaunch.getShellCommand(
            claude: claude, session: session, hasConversation: fileManager.fileExists(atPath: transcript.path),
            readableFolder: fileManager.fileExists(atPath: readableFolder) ? readableFolder : nil, firstMessage: firstMessage
        )

        let terminal = LocalProcessTerminalView(frame: NSRect(x: 0, y: 0, width: 640, height: 480))
        let watcher = ProcessEndWatcher { [weak self] in self?.handleEnd(of: session.id, client: client) }
        terminal.processDelegate = watcher
        let environment = SessionEnvironment.make(from: ProcessInfo.processInfo.environment, stateFile: stateFile.path)
        terminal.startProcess(
            executable: "/bin/zsh", args: ["-l", "-i", "-c", command],
            environment: environment.map { "\($0.key)=\($0.value)" }, execName: nil, currentDirectory: folder.path
        )
        terminals[session.id] = terminal
        watchers[session.id] = watcher
        names[session.id] = session.name
        stateFiles[session.id] = stateFile
        runningSessionIDs.insert(session.id)
        startReadingActivities()
        requestNotificationPermission()
        _ = try? await client.send("POST", "v1/claude-sessions/\(session.id.uuidString)/start", body: EmptyRequest(), as: ClaudeSession.self)
    }

    /// Types a message into a running session and sends it.
    func send(_ message: String, to sessionID: UUID) {
        guard let terminal = terminals[sessionID] else { return }
        terminal.send(source: terminal, data: ArraySlice(Array(ClaudeLaunch.getPastedMessage(message).utf8)))
    }

    /// Ends the session's process and the session with it. SwiftTerm stops
    /// watching for the process's exit when asked to terminate it, so the end
    /// never arrives from the terminal: the session ends here at once, and its
    /// process is reaped once it exits.
    func stop(_ sessionID: UUID) {
        guard let terminal = terminals[sessionID] else { return }
        let pid = terminal.process.shellPid
        terminal.terminate()
        Self.reapProcess(pid)
        watchers[sessionID]?.notifyEnd()
    }

    /// Waits for the stopped process to exit, so it doesn't linger as a
    /// zombie, and kills it if it hasn't within a few seconds.
    private nonisolated static func reapProcess(_ pid: pid_t) {
        guard pid > 0 else { return }
        let exited = DispatchSemaphore(value: 0)
        DispatchQueue.global(qos: .utility).async {
            var status: Int32 = 0
            waitpid(pid, &status, 0)
            exited.signal()
        }
        DispatchQueue.global(qos: .utility).async {
            if exited.wait(timeout: .now() + 5) == .timedOut {
                kill(pid, SIGKILL)
            }
        }
    }

    private func handleEnd(of sessionID: UUID, client: HubClient) {
        guard terminals[sessionID] != nil else { return }
        terminals[sessionID] = nil
        watchers[sessionID] = nil
        stateFiles[sessionID] = nil
        activities[sessionID] = nil
        runningSessionIDs.remove(sessionID)
        Task { _ = try? await client.send("POST", "v1/claude-sessions/\(sessionID.uuidString)/stop", body: EmptyRequest(), as: ClaudeSession.self) }
    }

    /// Reads each running session's state file once a second, the file its
    /// hooks write.
    private func startReadingActivities() {
        guard activityTimer == nil else { return }
        activityTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { _ in
            Task { @MainActor in ClaudeSessionHost.shared.readActivities() }
        }
    }

    private func readActivities() {
        for (sessionID, stateFile) in stateFiles {
            guard let text = try? String(contentsOf: stateFile, encoding: .utf8), let activity = ClaudeHooks.parseActivity(text) else { continue }
            let previous = activities[sessionID]
            guard activity != previous else { continue }
            activities[sessionID] = activity
            if previous == .working, activity != .working {
                notifyAboutTurn(of: sessionID, activity: activity)
            }
        }
        if stateFiles.isEmpty {
            activityTimer?.invalidate()
            activityTimer = nil
        }
    }

    /// A session that stopped working while nobody looks at it says so.
    private func notifyAboutTurn(of sessionID: UUID, activity: SessionActivity) {
        guard !(shownSessionIDs.contains(sessionID) && NSApp.isActive) else { return }
        let content = UNMutableNotificationContent()
        content.title = names[sessionID] ?? "Claude session"
        content.body = activity == .blocked ? "Waiting for you" : "Finished its turn"
        content.sound = .default
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
    }

    private func requestNotificationPermission() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
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

    /// Ends the session now, for a stop the terminal won't report.
    @MainActor func notifyEnd() {
        onEnd()
    }

    func sizeChanged(source: LocalProcessTerminalView, newCols: Int, newRows: Int) {}
    func setTerminalTitle(source: LocalProcessTerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
}
