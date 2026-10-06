import AppKit
import JobSearchHubCore
import Observation

/// Installs the ready version: the install sheet lists what's running and
/// waits for it, then the app records what it has open, starts `hub-update`
/// from the new bundle as a launchd job, and quits; `hub-update` does the
/// rest. At the next launch it says how the install ended, tells
/// `hub-update` it opened, and reopens its sessions.
@MainActor
@Observable
final class Installer {
    static let shared = Installer()
    /// The launchd job that runs `hub-update`, apart from the server's.
    static let jobLabel = "com.tonypine.jobsearchhub.update"
    /// How often a waiting sheet looks at what's still running.
    static let waitInterval: Duration = .seconds(2)
    /// How long the banner says how the last install ended.
    static let bannerLifetime: TimeInterval = 24 * 60 * 60
    private static let dismissedKey = "installBannerDismissed"

    enum Phase: Equatable {
        case idle
        /// The sheet lists what runs, before the owner picks.
        case reviewing
        /// The hub drains and the sheet ticks work off as it ends.
        case waiting
        /// *Install when I quit*: the app installs at ⌘Q.
        case whenQuit
        /// The install is starting; the app is about to quit.
        case starting
    }

    /// The version to install, and what its install changes.
    struct Target: Equatable {
        var version: HubVersion
        var app: URL
        var fromMigration: Int
        var toMigration: Int

        var changesDatabase: Bool { toMigration > fromMigration }
    }

    /// How the last install ended, for the banner over the pages.
    struct Notice: Equatable {
        var outcome: InstallOutcome
        var from: String
        var to: String
        var endedAt: Date

        /// Remembers a dismissed banner across launches.
        var key: String { "\(to)@\(endedAt.timeIntervalSince1970)" }
    }

    private(set) var phase: Phase = .idle
    var isShowingSheet = false
    var isConfirmingInstallAnyway = false
    private(set) var target: Target?
    private(set) var work = RunningWork(items: [], idleSessionCount: 0)
    var failure: HubFailure?
    /// How the last install ended, until dismissed or a day has passed.
    private(set) var notice: Notice?
    /// The install under way while this app runs, which isn't its own: one
    /// cut short that `hub-update` finishes.
    private(set) var installUnderWay: InstallState?
    /// The page an unsaved edit's line asked the main window to show.
    var requestedPage: Page?
    /// The page the main window shows, which the app reopens on.
    var shownPage: Page?
    /// Whether the app should reopen once the install is done: false after
    /// *Install when I quit*.
    private(set) var reopensApp = true

    @ObservationIgnored let updates = UpdatesFolder.makeDefault(home: FileManager.default.homeDirectoryForCurrentUser)
    @ObservationIgnored weak var taskRunner: RemoteTaskRunner?
    @ObservationIgnored var makeClient: @MainActor () -> HubClient? = { nil }
    @ObservationIgnored private var waitLoop: Task<Void, Never>?
    /// The app is quitting for the install; the quit isn't asked about.
    @ObservationIgnored private var isQuittingForInstall = false
    /// A ⌘Q waits for the answer, while the app checks what runs.
    @ObservationIgnored private var isTerminationPending = false
    @ObservationIgnored private var launchedMarkDue = false
    @ObservationIgnored private var reopenRecord: ReopenRecord?
    @ObservationIgnored private let inspector = CodesignInspector()

    /// Why this copy of the app can't install versions; nil when it can. Only
    /// the installed app, carrying `hub-update`, installs over itself.
    var cannotInstallReason: String? {
        if !ServerAgent.isInstalledCopy {
            return "Only the app installed in ~/Applications installs new versions. Install a build with macos/Scripts/install-app.sh first."
        }
        if !FileManager.default.isExecutableFile(atPath: ServerLaunchAgent.makeCommandURL("hub-update", bundle: Bundle.main.bundleURL).path) {
            return "This build carries no hub-update to install with."
        }
        if let installUnderWay {
            return "The install of \(installUnderWay.to) is still finishing."
        }
        return nil
    }

    var isDraining: Bool { phase == .waiting }

    // MARK: Choosing

    /// *Install now…*: builds the sheet from what's running, and shows it.
    func review(_ ready: ReadyVersion) async {
        guard phase == .idle || phase == .whenQuit else {
            isShowingSheet = true
            return
        }
        guard await prepare(ready) else { return }
        reopensApp = true
        phase = .reviewing
        await refreshWork()
        isShowingSheet = true
    }

    /// *Install when I quit*: nothing happens until ⌘Q.
    func installWhenIQuit(_ ready: ReadyVersion) async {
        guard phase == .idle, await prepare(ready) else { return }
        phase = .whenQuit
    }

    /// *Install when these finish*: the hub stops starting work, the app
    /// stops claiming tasks, and the sheet waits.
    func installWhenTheseFinish() async {
        guard target != nil, !work.hasUnsavedEdits else { return }
        if let client = makeClient() {
            do {
                let status: DrainStatus = try await client.send("POST", "v1/drain", body: EmptyBody())
                work = work.update(with: makeWork(drain: status.running))
            } catch {
                failure = HubFailure("Couldn't ask the hub to finish its work", error)
                return
            }
        }
        taskRunner?.isPaused = true
        phase = .waiting
        if work.isClear {
            await start()
            return
        }
        waitLoop?.cancel()
        waitLoop = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: Self.waitInterval)
                guard let self, self.phase == .waiting else { return }
                await self.refreshWork()
                if self.work.isClear {
                    await self.start()
                    return
                }
            }
        }
    }

    /// *Install anyway…*, once its cost was confirmed.
    func installAnyway() async {
        guard target != nil, !work.hasUnsavedEdits else { return }
        await start()
    }

    /// *Cancel*: the hub starts work again, and so does the app.
    func cancel() async {
        waitLoop?.cancel()
        waitLoop = nil
        if phase == .waiting, let client = makeClient() {
            _ = try? await client.delete("v1/drain", as: DrainStatus.self)
        }
        taskRunner?.isPaused = false
        if isTerminationPending {
            isTerminationPending = false
            NSApp.reply(toApplicationShouldTerminate: false)
        }
        phase = .idle
        isShowingSheet = false
        failure = nil
    }

    /// An unsaved edit's line: its page shows, and the sheet steps aside
    /// until the sidebar's label brings it back.
    func openEdit(_ item: RunningItem) {
        guard item.kind == .unsavedEdit else { return }
        requestedPage = UnsavedEdits.shared.getPage(of: String(item.id.dropFirst("edit-".count)))
        isShowingSheet = false
    }

    // MARK: What runs

    private func prepare(_ ready: ReadyVersion) async -> Bool {
        failure = nil
        if let reason = cannotInstallReason {
            failure = HubFailure("Couldn't install \(ready.version)", advice: reason)
            return false
        }
        guard let checked = NewVersionChecker.shared.updates.readChecked(ready.version) else {
            failure = HubFailure("Couldn't install \(ready.version)", advice: "Its download is gone. Check for new versions again.")
            return false
        }
        async let running = inspector.readServerBuild(of: Bundle.main.bundleURL)
        async let new = inspector.readServerBuild(of: checked.app)
        let (from, to) = await (running?.newestMigration, new?.newestMigration)
        guard let to else {
            failure = HubFailure("Couldn't install \(ready.version)", advice: "Its server doesn't say which migrations it has.")
            return false
        }
        target = Target(version: ready.version, app: checked.app, fromMigration: from ?? to, toMigration: to)
        return true
    }

    /// Reads what runs now: the sessions, the hub's drain list, the app's
    /// tasks and unsaved edits. A waiting sheet ticks off what ended.
    func refreshWork() async {
        var drain: [DrainStatus.Work] = []
        if let client = makeClient(), let status: DrainStatus = try? await client.get("v1/drain") {
            drain = status.running
        }
        let now = makeWork(drain: drain)
        work = phase == .waiting ? work.update(with: now) : now
    }

    private func makeWork(drain: [DrainStatus.Work]) -> RunningWork {
        RunningWork.make(
            sessions: ClaudeSessionHost.shared.runningSessions, drain: drain,
            tasks: taskRunner.map { Array($0.runningTasks.values) } ?? [], edits: UnsavedEdits.shared.list, now: .now
        )
    }

    // MARK: Quitting

    /// ⌘Q: after *Install when I quit*, the install runs once the app has
    /// quit. With work running, the sheet shows and waits for it first.
    func shouldTerminate() -> NSApplication.TerminateReply {
        if isQuittingForInstall || phase != .whenQuit {
            return .terminateNow
        }
        isTerminationPending = true
        reopensApp = false
        Task {
            await refreshWork()
            if work.isClear {
                await start()
            } else {
                // The sheet shows only when something runs.
                isTerminationPending = false
                NSApp.reply(toApplicationShouldTerminate: false)
                phase = .reviewing
                isShowingSheet = true
            }
        }
        return .terminateLater
    }

    /// Records what's open, hands the install to `hub-update`, and quits.
    private func start() async {
        guard let target, phase != .starting else { return }
        waitLoop?.cancel()
        phase = .starting
        do {
            try saveReopenRecord()
            try startHubUpdate(target)
        } catch {
            failure = HubFailure("Couldn't start the install", error)
            phase = .reviewing
            taskRunner?.isPaused = false
            if isTerminationPending {
                isTerminationPending = false
                NSApp.reply(toApplicationShouldTerminate: false)
            }
            isShowingSheet = true
            return
        }
        updates.log("Installing \(target.version) over \(HubClient.appVersion); the app quits for hub-update.", at: .now)
        isQuittingForInstall = true
        if isTerminationPending {
            NSApp.reply(toApplicationShouldTerminate: true)
        } else {
            NSApp.terminate(nil)
        }
    }

    private func saveReopenRecord() throws {
        let host = ClaudeSessionHost.shared
        let sessions = host.runningSessionIDs.sorted { $0.uuidString < $1.uuidString }.compactMap { id in
            host.getSubject(of: id).map { ReopenRecord.Session(id: id, subject: $0) }
        }
        let shown = host.shownSessionIDs.compactMap(host.getSubject(of:)).first { !host.windowedSubjects.contains($0) }
        let record = ReopenRecord(
            version: HubClient.appVersion, savedAt: .now, sessions: sessions, windowedSubjects: Array(host.windowedSubjects),
            shownSubject: shown, page: shownPage?.rawValue, taskIDs: taskRunner.map { Array($0.runningTasks.keys) } ?? []
        )
        try updates.writeReopenRecord(record)
    }

    /// Writes the install's state, copies `hub-update` out of the new
    /// bundle, which the install moves, and loads it as a one-shot launchd
    /// job that runs again if it crashes, or at login after a power loss.
    private func startHubUpdate(_ target: Target) throws {
        if let state = updates.readState(), !state.step.isFinished {
            throw InstallError("The install of \(state.to) is still under way.")
        }
        let fileManager = FileManager.default
        let home = fileManager.homeDirectoryForCurrentUser
        guard let running = HubVersion(HubClient.appVersion) else { throw InstallError("This app's version, \(HubClient.appVersion), can't be read.") }
        try fileManager.createDirectory(at: updates.root, withIntermediateDirectories: true)

        let installer = updates.installerURL
        try? fileManager.removeItem(at: installer)
        try fileManager.copyItem(at: ServerLaunchAgent.makeCommandURL("hub-update", bundle: target.app), to: installer)

        let plist = home.appending(path: "Library/LaunchAgents/\(Self.jobLabel).plist")
        let state = InstallState(
            from: running, to: target.version, installed: ServerLaunchAgent.makeInstalledAppURL(home: home), newApp: target.app,
            reopenApp: reopensApp, fromMigration: target.fromMigration, toMigration: target.toMigration, startedAt: .now, jobPlist: plist
        )
        try updates.writeState(state)
        try? fileManager.removeItem(at: updates.launchedURL)

        let job: [String: Any] = [
            "Label": Self.jobLabel,
            "ProgramArguments": [installer.path, "install", updates.stateURL.path],
            "RunAtLoad": true,
            // Run again after a crash; an install that ended exits 0.
            "KeepAlive": ["SuccessfulExit": false],
            "StandardOutPath": updates.root.appending(path: "hub-update.log").path,
            "StandardErrorPath": updates.root.appending(path: "hub-update.log").path,
            "LimitLoadToSessionType": "Aqua",
            // The steps window it starts outlives it by a few seconds.
            "AbandonProcessGroup": true,
        ]
        try fileManager.createDirectory(at: plist.deletingLastPathComponent(), withIntermediateDirectories: true)
        try PropertyListSerialization.data(fromPropertyList: job, format: .xml, options: 0).write(to: plist, options: .atomic)

        let domain = "gui/\(getuid())"
        // A job from the last install stays loaded, finished, until logout.
        _ = Self.runLaunchctl(["bootout", "\(domain)/\(Self.jobLabel)"])
        let (status, output) = Self.runLaunchctl(["bootstrap", domain, plist.path])
        guard status == 0 else {
            try? fileManager.removeItem(at: updates.stateURL)
            throw InstallError("launchd didn't start hub-update: \(output)")
        }
    }

    private static func runLaunchctl(_ arguments: [String]) -> (Int32, String) {
        let process = Process()
        process.executableURL = URL(filePath: "/bin/launchctl")
        process.arguments = arguments
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe
        do {
            try process.run()
        } catch {
            return (-1, error.localizedDescription)
        }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return (process.terminationStatus, String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines))
    }

    // MARK: After

    /// At launch: how the last install ended, whether `hub-update` waits to
    /// hear this app opened, and what to reopen.
    func readLaunchState() {
        let fileManager = FileManager.default
        if let state = updates.readState() {
            if state.step.isFinished {
                try? fileManager.removeItem(at: updates.lastInstallURL)
                try? fileManager.moveItem(at: updates.stateURL, to: updates.lastInstallURL)
            } else {
                if (state.step == .openingApp || state.step == .checkingApp) && state.to == HubClient.appVersion {
                    launchedMarkDue = true
                } else {
                    installUnderWay = state
                }
                // hub-update opened this app before it finished: the banner
                // waits for the end.
                watchInstallEnd()
            }
        }
        readNotice()
        if let record = updates.readReopenRecord() {
            try? fileManager.removeItem(at: updates.reopenURL)
            if record.isFresh(at: .now) { reopenRecord = record }
        }
    }

    /// Looks at the install `hub-update` is finishing every second, until
    /// it ends; then says how, as at a launch.
    private func watchInstallEnd() {
        Task { [weak self] in
            // Its longest wait, for migrations, is minutes.
            for _ in 0..<(30 * 60) {
                try? await Task.sleep(for: .seconds(1))
                guard let self else { return }
                guard let state = self.updates.readState() else {
                    self.installUnderWay = nil
                    return
                }
                guard state.step.isFinished else { continue }
                try? FileManager.default.removeItem(at: self.updates.lastInstallURL)
                try? FileManager.default.moveItem(at: self.updates.stateURL, to: self.updates.lastInstallURL)
                self.installUnderWay = nil
                self.readNotice()
                // What's new in the version just installed.
                NewVersionChecker.shared.checkNow()
                return
            }
        }
    }

    /// The banner for the last install, unless dismissed or a day old.
    private func readNotice() {
        if let data = try? Data(contentsOf: updates.lastInstallURL), let last = try? InstallState.decode(data),
           let outcome = InstallOutcome.make(last) {
            let endedAt = last.finishedAt ?? (try? updates.lastInstallURL.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .now
            let shown = Notice(outcome: outcome, from: last.from, to: last.to, endedAt: endedAt)
            if Date.now.timeIntervalSince(endedAt) < Self.bannerLifetime, UserDefaults.standard.string(forKey: Self.dismissedKey) != shown.key {
                notice = shown
            }
        }
    }

    /// Once the window is up and connected, tells `hub-update` the new app
    /// opened, which ends its check.
    func markLaunchedIfDue() {
        guard launchedMarkDue, let version = HubVersion(HubClient.appVersion) else { return }
        launchedMarkDue = false
        do {
            try updates.writeLaunched(version)
        } catch {
            updates.log("Couldn't say \(version) opened: \(error.localizedDescription)", at: .now)
        }
    }

    /// The sessions, windows and page the app had open when it quit for the
    /// install, resumed; and the tasks it cut off, back in the queue.
    func reopen(
        with client: HubClient, openWindow: (ClaudeSessionSubject) -> Void, showSession: (ClaudeSessionSubject) -> Void, showPage: (Page) -> Void
    ) async {
        guard let record = reopenRecord else { return }
        reopenRecord = nil
        if let page = record.page.flatMap(Page.init(rawValue:)) {
            showPage(page)
        }
        let host = ClaudeSessionHost.shared
        for reopened in record.sessions where !host.isRunning(reopened.id) {
            guard let sessions = try? await client.get("v1/claude-sessions", query: reopened.subject.queryItems, as: ClaudeSessionsResponse.self).sessions,
                  let session = sessions.first(where: { $0.id == reopened.id })
            else { continue }
            try? await host.start(session, with: client)
        }
        for subject in record.windowedSubjects {
            host.windowedSubjects.insert(subject)
            openWindow(subject)
        }
        if let shown = record.shownSubject {
            showSession(shown)
        }
        for id in record.taskIDs {
            _ = try? await client.send("POST", "v1/tasks/\(id.uuidString)/release", body: EmptyBody(), as: TaskRequest.self)
        }
        if !record.taskIDs.isEmpty {
            await taskRunner?.check(with: client)
        }
    }

    /// The banner goes; the install log keeps what it said.
    func dismissNotice() {
        guard let notice else { return }
        UserDefaults.standard.set(notice.key, forKey: Self.dismissedKey)
        self.notice = nil
    }

    /// The version the last install installed, when it was this one, for
    /// Settings › Version's *What's new*.
    var installedFrom: HubVersion? {
        guard let notice, case let .installed(version) = notice.outcome, version.description == HubClient.appVersion else { return nil }
        return HubVersion(notice.from)
    }
}

/// Why an install couldn't start, in a sentence.
struct InstallError: LocalizedError {
    let message: String

    init(_ message: String) {
        self.message = message
    }

    var errorDescription: String? { message }
}
