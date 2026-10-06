import Foundation

/// Where an install is, as `hub-update` records it in `Updates/state.json`.
public enum InstallStep: String, Codable, Sendable, CaseIterable {
    case waitingForApp = "waiting_for_app"
    case stoppingServer = "stopping_server"
    case swapping
    case startingServer = "starting_server"
    case checkingServer = "checking_server"
    case openingApp = "opening_app"
    case checkingApp = "checking_app"
    case recording

    case stoppingNewServer = "stopping_new_server"
    case countingWrites = "counting_writes"
    case restoringDatabase = "restoring_database"
    case swappingBack = "swapping_back"
    case startingOldServer = "starting_old_server"
    case checkingOldServer = "checking_old_server"
    case reporting

    case installed
    case abandoned
    case rolledBack = "rolled_back"
    case rollbackFailed = "rollback_failed"

    /// Whether the install has ended, one way or another.
    public var isFinished: Bool {
        switch self {
        case .installed, .abandoned, .rolledBack, .rollbackFailed: true
        default: false
        }
    }

    /// Whether the step belongs to a rollback.
    public var isRollback: Bool {
        switch self {
        case .stoppingNewServer, .countingWrites, .restoringDatabase, .swappingBack, .startingOldServer, .checkingOldServer, .reporting: true
        default: false
        }
    }
}

/// An install, as `Updates/state.json` holds it. The app writes the first
/// one, waiting for the app, then `hub-update` takes it from there.
public struct InstallState: Codable, Equatable, Sendable {
    public var from: String
    public var to: String
    /// Where the app is installed, and where the checked download of `to` is.
    public var installed: String
    public var newApp: String
    /// The app quit for this install, so it opens again once the server is
    /// up; after *Install when I quit* it stays closed.
    public var reopenApp: Bool
    public var fromMigration: Int
    public var toMigration: Int
    public var startedAt: Date
    /// The launchd job running `hub-update`, which goes once the install ends.
    public var jobPlist: String?
    public var step: InstallStep
    /// The new server's log shows its migrations running.
    public var migrating: Bool?
    public var failure: String?
    public var dump: String?
    public var lost: String?
    public var error: String?
    public var commands: [String]?
    public var finishedAt: Date?

    public init(
        from: HubVersion, to: HubVersion, installed: URL, newApp: URL, reopenApp: Bool, fromMigration: Int, toMigration: Int,
        startedAt: Date, jobPlist: URL?
    ) {
        self.from = from.description
        self.to = to.description
        self.installed = installed.path
        self.newApp = newApp.path
        self.reopenApp = reopenApp
        self.fromMigration = fromMigration
        self.toMigration = toMigration
        self.startedAt = startedAt
        self.jobPlist = jobPlist?.path
        step = .waitingForApp
    }

    /// Whether this version changes the database.
    public var changesDatabase: Bool { toMigration > fromMigration }

    public static func decode(_ data: Data) throws -> InstallState {
        try HubJSON.makeDecoder().decode(InstallState.self, from: data)
    }

    public func encode() throws -> Data {
        let encoder = HubJSON.makeEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(self)
    }
}

/// One line of the steps window: what's done, what's under way, what's next.
public struct InstallProgressRow: Equatable, Sendable {
    public enum Status: Equatable, Sendable {
        case done, current, waiting, failed
    }

    public var title: String
    public var status: Status

    public init(_ title: String, _ status: Status) {
        self.title = title
        self.status = status
    }
}

/// What the steps window shows for an install's state.
public struct InstallProgress: Equatable, Sendable {
    public var title: String
    public var subtitle: String
    public var rows: [InstallProgressRow]
    /// How far along, from 0 to 1.
    public var fraction: Double

    public static func make(_ state: InstallState) -> InstallProgress {
        if state.step.isRollback || state.step == .rolledBack || state.step == .rollbackFailed {
            return makeRollback(state)
        }
        if state.step == .abandoned {
            return InstallProgress(
                title: "\(state.to) wasn't installed", subtitle: state.failure ?? "Nothing changed.",
                rows: [], fraction: 1
            )
        }
        let order: [InstallStep] = [.waitingForApp, .stoppingServer, .swapping, .startingServer, .checkingServer, .openingApp, .checkingApp, .recording, .installed]
        let at = order.firstIndex(of: state.step) ?? 0
        let migrating = state.migrating == true
        func status(doneFrom: InstallStep, currentFrom: InstallStep) -> InstallProgressRow.Status {
            let done = order.firstIndex(of: doneFrom)!, current = order.firstIndex(of: currentFrom)!
            return at >= done ? .done : at >= current ? .current : .waiting
        }
        var rows = [
            InstallProgressRow("Work finished", status(doneFrom: .stoppingServer, currentFrom: .waitingForApp)),
            InstallProgressRow(at >= 2 ? "Server stopped" : "Stopping the server…", status(doneFrom: .swapping, currentFrom: .stoppingServer)),
        ]
        // The new server dumps and migrates before it answers, so the
        // database row stays under way until the check passes.
        var restart = status(doneFrom: .openingApp, currentFrom: .swapping)
        var check = status(doneFrom: .openingApp, currentFrom: .checkingServer)
        if state.step == .checkingServer && (migrating || state.changesDatabase) {
            restart = migrating ? .current : .done
            check = migrating ? .waiting : .current
        } else if state.step == .checkingServer {
            restart = .done
        }
        let restartTitle = state.changesDatabase
            ? (restart == .done ? "Database updated" : "Updating the database…")
            : (restart == .done ? "Server restarted" : "Restarting the server…")
        rows.append(InstallProgressRow(restartTitle, restart))
        rows.append(InstallProgressRow(check == .done ? "It works" : "Checking it works", check))
        if state.reopenApp {
            rows.append(InstallProgressRow("Reopening Job Search Hub", status(doneFrom: .recording, currentFrom: .openingApp)))
        }
        let done = rows.filter { $0.status == .done }.count
        let current = rows.contains { $0.status == .current } ? 0.5 : 0
        return InstallProgress(
            title: state.step == .installed ? "Installed Job Search Hub \(state.to)" : "Installing Job Search Hub \(state.to)",
            subtitle: state.reopenApp ? "Job Search Hub reopens when it's done" : "Job Search Hub stays closed, as you left it",
            rows: rows, fraction: min(1, (Double(done) + current) / Double(rows.count))
        )
    }

    private static func makeRollback(_ state: InstallState) -> InstallProgress {
        let order: [InstallStep] = [.stoppingNewServer, .countingWrites, .restoringDatabase, .swappingBack, .startingOldServer, .checkingOldServer, .reporting, .rolledBack]
        let failed = state.step == .rollbackFailed
        // A failed rollback's error names its step: rows from there on wait.
        let reached: Int
        if failed {
            reached = order.firstIndex(where: { state.error?.contains($0.rawValue) == true }) ?? 0
        } else {
            reached = order.firstIndex(of: state.step) ?? 0
        }
        func status(_ step: InstallStep, through end: InstallStep) -> InstallProgressRow.Status {
            let start = order.firstIndex(of: step)!, finish = order.firstIndex(of: end)!
            if reached > finish { return .done }
            if reached >= start { return failed ? .failed : .current }
            return .waiting
        }
        var rows = [InstallProgressRow("\(state.to) stopped", status(.stoppingNewServer, through: .countingWrites))]
        if state.dump != nil || state.changesDatabase {
            rows.append(InstallProgressRow("Restoring the database from before the install", status(.restoringDatabase, through: .restoringDatabase)))
        }
        rows.append(InstallProgressRow("\(state.from) back in place", status(.swappingBack, through: .swappingBack)))
        rows.append(InstallProgressRow("Checking \(state.from) works", status(.startingOldServer, through: .checkingOldServer)))
        let done = rows.filter { $0.status == .done }.count
        let title: String
        switch state.step {
        case .rolledBack: title = "Back on Job Search Hub \(state.from)"
        case .rollbackFailed: title = "The hub couldn't go back by itself"
        default: title = "Going back to Job Search Hub \(state.from)"
        }
        return InstallProgress(
            title: title, subtitle: state.failure ?? "\(state.to) didn't pass its checks.",
            rows: rows, fraction: Double(done) / Double(rows.count)
        )
    }
}

/// How an install ended, as the app says it at its next launch.
public enum InstallOutcome: Equatable, Sendable {
    /// Now on the version: the banner, with *What's new*.
    case installed(HubVersion)
    /// The install didn't start: nothing changed.
    case abandoned(message: String)
    /// "0.1.252 couldn't start, so the hub went back to 0.1.247." and what
    /// that lost.
    case rolledBack(message: String, lost: String)
    /// The rollback stopped: where, and the commands that finish it.
    case rollbackFailed(message: String, error: String, commands: [String])

    public static func make(_ state: InstallState) -> InstallOutcome? {
        switch state.step {
        case .installed:
            return HubVersion(state.to).map(InstallOutcome.installed)
        case .abandoned:
            return .abandoned(message: state.failure ?? "\(state.to) wasn't installed.")
        case .rolledBack:
            return .rolledBack(message: state.failure ?? "\(state.to) didn't work, so the hub went back to \(state.from).", lost: state.lost ?? "Nothing was lost.")
        case .rollbackFailed:
            return .rollbackFailed(
                message: state.failure ?? "\(state.to) couldn't be installed, and the hub couldn't go back to \(state.from) by itself.",
                error: state.error ?? "", commands: state.commands ?? []
            )
        default:
            return nil
        }
    }
}

/// The work the server lists while it drains, from `GET /v1/drain`.
public struct DrainStatus: Decodable, Equatable, Sendable {
    public var draining: Bool
    public var running: [Work]

    public struct Work: Decodable, Equatable, Sendable {
        /// `model_call`, `claude_run` or `agent_run`.
        public var type: String
        public var kind: String
        public var subject: String?
        public var startedAt: Date
        public var ageSeconds: Int

        public init(type: String, kind: String, subject: String?, startedAt: Date, ageSeconds: Int) {
            self.type = type
            self.kind = kind
            self.subject = subject
            self.startedAt = startedAt
            self.ageSeconds = ageSeconds
        }
    }

    public init(draining: Bool, running: [Work]) {
        self.draining = draining
        self.running = running
    }
}

/// A Claude session running in the app, as the install sheet sees it.
public struct RunningSession: Equatable, Sendable {
    public var id: UUID
    public var name: String
    public var activity: SessionActivity

    public init(id: UUID, name: String, activity: SessionActivity) {
        self.id = id
        self.name = name
        self.activity = activity
    }
}

/// A task the app runs for the phone or for itself.
public struct RunningTask: Equatable, Sendable {
    public var id: UUID
    public var title: String
    public var startedAt: Date

    public init(id: UUID, title: String, startedAt: Date) {
        self.id = id
        self.title = title
        self.startedAt = startedAt
    }
}

/// An edit not saved yet, in a page of the app.
public struct UnsavedEdit: Equatable, Hashable, Sendable {
    public var id: String
    /// "Criteria", "The job brief prompt".
    public var title: String

    public init(id: String, title: String) {
        self.id = id
        self.title = title
    }
}

/// One line of the install sheet's *Running now*.
public struct RunningItem: Equatable, Identifiable, Sendable {
    public enum Kind: Equatable, Sendable {
        case session(SessionActivity)
        case agentRun
        case serverWork
        case remoteTask
        case unsavedEdit
    }

    public var id: String
    public var kind: Kind
    public var title: String
    /// "working", "agent run, 2 min", "Open".
    public var detail: String
    /// Ended since the sheet started waiting; it stays, ticked off.
    public var hasEnded = false

    /// What the line says once it has ended.
    public var endedDetail: String {
        switch kind {
        case .session: "turn ended"
        case .unsavedEdit: "saved"
        case .agentRun, .serverWork, .remoteTask: "finished"
        }
    }

    /// What installing now costs this item, in a sentence.
    public var cost: String {
        switch kind {
        case .session:
            "The \(title.replacingOccurrences(of: "Claude session · ", with: "")) session's turn stops mid-way; its conversation reopens with the new version."
        case .agentRun:
            "\(title) stops; run it again afterwards."
        case .serverWork:
            "\(title) stops; the hub does it again after the restart."
        case .remoteTask:
            "\(title) stops; it goes back to the queue and runs again after the restart."
        case .unsavedEdit:
            "\(title) has unsaved edits."
        }
    }
}

/// What the install sheet lists: the work running now, and what it waits
/// for. Built from the sessions' activity, the server's drain list, the
/// app's tasks and its unsaved edits.
public struct RunningWork: Equatable, Sendable {
    public var items: [RunningItem]
    /// Idle sessions hold nothing up; they reopen where they were.
    public var idleSessionCount: Int

    public init(items: [RunningItem], idleSessionCount: Int) {
        self.items = items
        self.idleSessionCount = idleSessionCount
    }

    public static func make(
        sessions: [RunningSession], drain: [DrainStatus.Work], tasks: [RunningTask], edits: [UnsavedEdit], now: Date
    ) -> RunningWork {
        var items: [RunningItem] = []
        for session in sessions.sorted(by: { $0.name < $1.name }) where session.activity != .idle {
            items.append(RunningItem(
                id: "session-\(session.id.uuidString)", kind: .session(session.activity), title: "Claude session · \(session.name)",
                detail: session.activity == .blocked ? "waiting for you" : "working"
            ))
        }
        for work in drain {
            let age = describeAge(seconds: work.ageSeconds)
            switch work.type {
            case "agent_run":
                items.append(RunningItem(
                    id: "agent-\(work.kind)-\(work.subject ?? "")-\(work.startedAt.timeIntervalSince1970)", kind: .agentRun,
                    title: describeAgentRun(kind: work.kind, subject: work.subject), detail: "agent run, \(age)"
                ))
            default:
                let engine = work.type == "claude_run" ? "Claude" : "local model"
                items.append(RunningItem(
                    id: "work-\(work.type)-\(work.kind)-\(work.subject ?? "")-\(work.startedAt.timeIntervalSince1970)", kind: .serverWork,
                    title: RunsSummary.getKindTitle(work.kind), detail: "\(engine), \(age)"
                ))
            }
        }
        for task in tasks.sorted(by: { $0.startedAt < $1.startedAt }) {
            let seconds = max(0, Int(now.timeIntervalSince(task.startedAt)))
            items.append(RunningItem(id: "task-\(task.id.uuidString)", kind: .remoteTask, title: task.title, detail: "task, \(describeAge(seconds: seconds))"))
        }
        for edit in edits.sorted(by: { $0.title < $1.title }) {
            items.append(RunningItem(id: "edit-\(edit.id)", kind: .unsavedEdit, title: "\(edit.title) has unsaved edits", detail: "Open"))
        }
        return RunningWork(items: items, idleSessionCount: sessions.count { $0.activity == .idle })
    }

    /// "Researching Initech", as the sheet names an agent run.
    public static func describeAgentRun(kind: String, subject: String?) -> String {
        let subject = subject.flatMap { $0.isEmpty ? nil : $0 }
        switch kind {
        case "company_triage": return subject.map { "Researching \($0)" } ?? "Researching a company"
        case "job_finder": return subject.map { "Finding jobs at \($0)" } ?? "Finding jobs"
        case "profile_seed": return "Building your knowledge base"
        case "job_fix": return "Fixing a job's details"
        default: return RunsSummary.getKindTitle(kind)
        }
    }

    /// "2 min", or "just now" under a minute.
    public static func describeAge(seconds: Int) -> String {
        seconds < 60 ? "just now" : "\(seconds / 60) min"
    }

    /// "2 idle Claude sessions reopen where they were", or nil without one.
    public var idleSessionsLine: String? {
        switch idleSessionCount {
        case 0: nil
        case 1: "1 idle Claude session reopens where it was"
        default: "\(idleSessionCount) idle Claude sessions reopen where they were"
        }
    }

    /// Lines still running, ended ones aside.
    public var running: [RunningItem] { items.filter { !$0.hasEnded } }

    /// Whether nothing holds the install up any more.
    public var isClear: Bool { running.isEmpty }

    /// Unsaved edits block both ways to install, until saved or discarded.
    public var hasUnsavedEdits: Bool { running.contains { $0.kind == .unsavedEdit } }

    /// The list a waiting sheet shows next: every line seen since it started
    /// waiting, in the order first seen, those gone from `now` ticked off,
    /// and a line back in `now` (a session messaged again) running again.
    public func update(with now: RunningWork) -> RunningWork {
        var items = self.items.map { item in
            if let current = now.items.first(where: { $0.id == item.id }) { return current }
            var ended = item
            ended.hasEnded = true
            return ended
        }
        for item in now.items where !items.contains(where: { $0.id == item.id }) {
            items.append(item)
        }
        return RunningWork(items: items, idleSessionCount: now.idleSessionCount)
    }

    /// What *Install anyway* costs, a sentence per line still running.
    public var cost: String {
        running.filter { $0.kind != .unsavedEdit }.map(\.cost).joined(separator: " ")
    }

    /// The sidebar's line while the install waits: "Waiting for 1 session".
    public var waitingSummary: String {
        let running = self.running
        guard !running.isEmpty else { return "Starting…" }
        let sessions = running.count { if case .session = $0.kind { true } else { false } }
        if sessions == running.count {
            return sessions == 1 ? "Waiting for 1 session" : "Waiting for \(sessions) sessions"
        }
        return running.count == 1 ? "Waiting for 1 item" : "Waiting for \(running.count) items"
    }
}

/// What the app had open when it quit for an install, which it reopens at
/// its next launch: the sessions running, those in windows of their own, the
/// page shown, and the tasks cut off, which go back to the queue.
public struct ReopenRecord: Codable, Equatable, Sendable {
    /// A session that ran when the app quit, which it resumes.
    public struct Session: Codable, Equatable, Sendable {
        public var id: UUID
        public var subject: ClaudeSessionSubject

        public init(id: UUID, subject: ClaudeSessionSubject) {
            self.id = id
            self.subject = subject
        }
    }

    public var version: String
    public var savedAt: Date
    public var sessions: [Session]
    public var windowedSubjects: [ClaudeSessionSubject]
    /// The session the main window's inspector showed.
    public var shownSubject: ClaudeSessionSubject?
    public var page: String?
    public var taskIDs: [UUID]

    public init(
        version: String, savedAt: Date, sessions: [Session], windowedSubjects: [ClaudeSessionSubject], shownSubject: ClaudeSessionSubject?,
        page: String?, taskIDs: [UUID]
    ) {
        self.version = version
        self.savedAt = savedAt
        self.sessions = sessions
        self.windowedSubjects = windowedSubjects
        self.shownSubject = shownSubject
        self.page = page
        self.taskIDs = taskIDs
    }

    /// A record older than this is from another time, and reopens nothing.
    public static let freshFor: TimeInterval = 24 * 60 * 60

    public func isFresh(at now: Date) -> Bool {
        now.timeIntervalSince(savedAt) < Self.freshFor
    }
}

extension UpdatesFolder {
    public var stateURL: URL { root.appending(path: "state.json") }
    /// The last install's state, kept once the app has said how it ended.
    public var lastInstallURL: URL { root.appending(path: "last-install.json") }
    /// The app writes its version here once its window is up and connected.
    public var launchedURL: URL { root.appending(path: "launched") }
    public var badVersionsURL: URL { root.appending(path: "bad-versions.json") }
    public var reopenURL: URL { root.appending(path: "reopen.json") }
    /// The copy of `hub-update` the install's launchd job runs: outside the
    /// bundles, which the install moves.
    public var installerURL: URL { root.appending(path: "hub-update") }

    public func readState() -> InstallState? {
        (try? Data(contentsOf: stateURL)).flatMap { try? InstallState.decode($0) }
    }

    public func writeState(_ state: InstallState) throws {
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        try state.encode().write(to: stateURL, options: .atomic)
    }

    /// The versions that failed on this Mac, which aren't offered again.
    public func readBadVersions() -> Set<HubVersion> {
        guard let data = try? Data(contentsOf: badVersionsURL), let versions = try? JSONDecoder().decode([String].self, from: data) else { return [] }
        return Set(versions.compactMap(HubVersion.init))
    }

    public func writeLaunched(_ version: HubVersion) throws {
        try Data("\(version.description)\n".utf8).write(to: launchedURL, options: .atomic)
    }

    public func readReopenRecord() -> ReopenRecord? {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return (try? Data(contentsOf: reopenURL)).flatMap { try? decoder.decode(ReopenRecord.self, from: $0) }
    }

    public func writeReopenRecord(_ record: ReopenRecord) throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        try encoder.encode(record).write(to: reopenURL, options: .atomic)
    }
}
