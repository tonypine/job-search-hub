import Foundation
@testable import JobSearchHubCore
import Testing

// Made-up companies and sessions only.

private let now = Date(timeIntervalSince1970: 1_791_000_000)
private let acme = RunningSession(id: UUID(uuidString: "00000000-0000-0000-0000-00000000000a")!, name: "Acme", activity: .working)
private let globex = RunningSession(id: UUID(uuidString: "00000000-0000-0000-0000-00000000000b")!, name: "Globex", activity: .blocked)
private let initechIdle = RunningSession(id: UUID(uuidString: "00000000-0000-0000-0000-00000000000c")!, name: "Staff Engineer · Initech", activity: .idle)
private let research = DrainStatus.Work(type: "agent_run", kind: "company_triage", subject: "Initech", startedAt: now.addingTimeInterval(-130), ageSeconds: 130)
private let brief = DrainStatus.Work(type: "claude_run", kind: "job_brief", subject: "0b5c", startedAt: now.addingTimeInterval(-20), ageSeconds: 20)
private let fix = RunningTask(id: UUID(uuidString: "00000000-0000-0000-0000-0000000000f1")!, title: "Fixing Platform Lead", startedAt: now.addingTimeInterval(-185))
private let criteria = UnsavedEdit(id: "criteria", title: "Criteria")

@Test func theSheetListsWhatsRunningFromSessionsRunsTasksAndEdits() {
    let work = RunningWork.make(sessions: [initechIdle, globex, acme], drain: [research, brief], tasks: [fix], edits: [criteria], now: now)

    #expect(work.items.map(\.title) == [
        "Claude session · Acme", "Claude session · Globex", "Researching Initech", "Job briefs", "Fixing Platform Lead", "Criteria has unsaved edits",
    ])
    #expect(work.items.map(\.detail) == ["working", "waiting for you", "agent run, 2 min", "Claude, just now", "task, 3 min", "Open"])
    #expect(work.items.map(\.kind) == [.session(.working), .session(.blocked), .agentRun, .serverWork, .remoteTask, .unsavedEdit])
    // Idle sessions hold nothing up: they're a line of their own.
    #expect(work.idleSessionCount == 1)
    #expect(work.idleSessionsLine == "1 idle Claude session reopens where it was")
    #expect(!work.isClear)
    #expect(work.hasUnsavedEdits)
}

@Test func nothingRunningIsClear() {
    let work = RunningWork.make(sessions: [initechIdle, initechIdle], drain: [], tasks: [], edits: [], now: now)
    #expect(work.items.isEmpty)
    #expect(work.isClear)
    #expect(work.idleSessionsLine == "2 idle Claude sessions reopen where they were")
    #expect(RunningWork.make(sessions: [], drain: [], tasks: [], edits: [], now: now).idleSessionsLine == nil)
}

@Test func agentRunsAreNamedForWhatTheyDo() {
    #expect(RunningWork.describeAgentRun(kind: "company_triage", subject: "Initech") == "Researching Initech")
    #expect(RunningWork.describeAgentRun(kind: "job_finder", subject: "Acme") == "Finding jobs at Acme")
    #expect(RunningWork.describeAgentRun(kind: "company_triage", subject: "") == "Researching a company")
    #expect(RunningWork.describeAgentRun(kind: "profile_seed", subject: nil) == "Building your knowledge base")
}

@Test func theWaitingSheetTicksItemsOffAsTheyEnd() {
    let start = RunningWork.make(sessions: [acme], drain: [research], tasks: [], edits: [criteria], now: now)

    // The research finished and the criteria were saved; the session works on.
    let later = start.update(with: RunningWork.make(sessions: [acme], drain: [], tasks: [], edits: [], now: now))
    #expect(later.items.map(\.title) == ["Claude session · Acme", "Researching Initech", "Criteria has unsaved edits"])
    #expect(later.items.map(\.hasEnded) == [false, true, true])
    #expect(later.items.filter(\.hasEnded).map(\.endedDetail) == ["finished", "saved"])
    #expect(!later.hasUnsavedEdits)
    #expect(later.waitingSummary == "Waiting for 1 session")

    // Its turn ends: everything is ticked off.
    let idle = RunningSession(id: acme.id, name: acme.name, activity: .idle)
    let done = later.update(with: RunningWork.make(sessions: [idle], drain: [], tasks: [], edits: [], now: now))
    #expect(done.isClear)
    #expect(done.items.allSatisfy(\.hasEnded))
    #expect(done.items.first?.endedDetail == "turn ended")

    // Messaged again, it's back to working, and the install waits again.
    let again = done.update(with: RunningWork.make(sessions: [acme], drain: [], tasks: [], edits: [], now: now))
    #expect(!again.isClear)
    #expect(again.items.map(\.hasEnded) == [false, true, true])
}

@Test func workStartedWhileWaitingJoinsTheList() {
    let start = RunningWork.make(sessions: [acme], drain: [], tasks: [], edits: [], now: now)
    let later = start.update(with: RunningWork.make(sessions: [acme, globex], drain: [], tasks: [], edits: [], now: now))
    #expect(later.items.map(\.title) == ["Claude session · Acme", "Claude session · Globex"])
    #expect(later.waitingSummary == "Waiting for 2 sessions")
}

@Test func installAnywaySaysWhatItCostsFirst() {
    let work = RunningWork.make(sessions: [acme], drain: [research, brief], tasks: [fix], edits: [criteria], now: now)
    #expect(work.cost == "The Acme session's turn stops mid-way; its conversation reopens with the new version. "
        + "Researching Initech stops; run it again afterwards. Job briefs stops; the hub does it again after the restart. "
        + "Fixing Platform Lead stops; it goes back to the queue and runs again after the restart.")
    #expect(work.waitingSummary == "Waiting for 5 items")
}

@Test func theStateHubUpdateWritesIsRead() throws {
    // As Go writes it: snake_case, times with nanoseconds.
    let json = """
    {
      "from": "0.1.247", "to": "0.1.252",
      "installed": "/Users/owner/Applications/Job Search Hub.app",
      "new_app": "/Users/owner/Library/Application Support/JobSearchHub/Updates/0.1.252/JobSearchHub.app",
      "reopen_app": true, "from_migration": 88, "to_migration": 91,
      "started_at": "2026-10-06T03:30:00.123456789Z",
      "step": "rolled_back", "server_log": 1024, "runs": 1,
      "failure": "0.1.252 couldn't start, so the hub went back to 0.1.247.",
      "dump": "/backups/hub-pre-migration-88.dump", "lost": "Lost: 2 updates.",
      "finished_at": "2026-10-06T03:33:10Z"
    }
    """
    let state = try InstallState.decode(Data(json.utf8))
    #expect(state.step == .rolledBack)
    #expect(state.changesDatabase)
    #expect(state.newApp.hasSuffix("Updates/0.1.252/JobSearchHub.app"))
    #expect(InstallOutcome.make(state) == .rolledBack(message: "0.1.252 couldn't start, so the hub went back to 0.1.247.", lost: "Lost: 2 updates."))
}

@Test func theStateTheAppWritesUsesHubUpdatesKeys() throws {
    let state = InstallState(
        from: HubVersion("0.1.247")!, to: HubVersion("0.1.252")!, installed: URL(filePath: "/Applications/Job Search Hub.app"),
        newApp: URL(filePath: "/Updates/0.1.252/JobSearchHub.app"), reopenApp: false, fromMigration: 88, toMigration: 88,
        startedAt: now, jobPlist: URL(filePath: "/LaunchAgents/com.tonypine.jobsearchhub.update.plist")
    )
    let object = try JSONSerialization.jsonObject(with: state.encode()) as? [String: Any]
    #expect(object?["new_app"] as? String == "/Updates/0.1.252/JobSearchHub.app")
    #expect(object?["reopen_app"] as? Bool == false)
    #expect(object?["from_migration"] as? Int == 88)
    #expect(object?["step"] as? String == "waiting_for_app")
    #expect(object?["job_plist"] as? String == "/LaunchAgents/com.tonypine.jobsearchhub.update.plist")
    #expect((object?["started_at"] as? String)?.hasPrefix("2026-") == true)
    #expect(try InstallState.decode(state.encode()) == state)
}

private func makeState(_ step: InstallStep, migrating: Bool = false, toMigration: Int = 91, reopen: Bool = true) -> InstallState {
    var state = InstallState(
        from: HubVersion("0.1.247")!, to: HubVersion("0.1.252")!, installed: URL(filePath: "/a.app"), newApp: URL(filePath: "/b.app"),
        reopenApp: reopen, fromMigration: 88, toMigration: toMigration, startedAt: now, jobPlist: nil
    )
    state.step = step
    state.migrating = migrating
    return state
}

@Test func theStepsWindowFollowsTheInstall() {
    let migrating = InstallProgress.make(makeState(.checkingServer, migrating: true))
    #expect(migrating.title == "Installing Job Search Hub 0.1.252")
    #expect(migrating.subtitle == "Job Search Hub reopens when it's done")
    #expect(migrating.rows == [
        InstallProgressRow("Work finished", .done), InstallProgressRow("Server stopped", .done),
        InstallProgressRow("Updating the database…", .current), InstallProgressRow("Checking it works", .waiting),
        InstallProgressRow("Reopening Job Search Hub", .waiting),
    ])

    let checking = InstallProgress.make(makeState(.checkingServer, toMigration: 88, reopen: false))
    #expect(checking.subtitle == "Job Search Hub stays closed, as you left it")
    #expect(checking.rows.map(\.title) == ["Work finished", "Server stopped", "Server restarted", "Checking it works"])
    #expect(checking.rows.map(\.status) == [.done, .done, .done, .current])

    let reopening = InstallProgress.make(makeState(.checkingApp))
    #expect(reopening.rows.map(\.status) == [.done, .done, .done, .done, .current])
    #expect(InstallProgress.make(makeState(.waitingForApp)).rows.first?.status == .current)
    #expect(InstallProgress.make(makeState(.installed)).fraction == 1)
}

@Test func theStepsWindowShowsARollbackAndWhereItStopped() {
    var state = makeState(.restoringDatabase)
    state.dump = "/backups/hub-pre-migration-88.dump"
    let restoring = InstallProgress.make(state)
    #expect(restoring.title == "Going back to Job Search Hub 0.1.247")
    #expect(restoring.rows.map(\.status) == [.done, .current, .waiting, .waiting])

    state.step = .rollbackFailed
    state.error = "The rollback stopped at restoring_database: pg_restore failed"
    state.commands = ["launchctl kill SIGTERM gui/501/com.tonypine.jobsearchhub.server"]
    let failed = InstallProgress.make(state)
    #expect(failed.title == "The hub couldn't go back by itself")
    #expect(failed.rows.map(\.status) == [.done, .failed, .waiting, .waiting])
    #expect(InstallOutcome.make(state) == .rollbackFailed(
        message: "0.1.252 couldn't be installed, and the hub couldn't go back to 0.1.247 by itself.",
        error: "The rollback stopped at restoring_database: pg_restore failed",
        commands: ["launchctl kill SIGTERM gui/501/com.tonypine.jobsearchhub.server"]
    ))
}

@Test func anInstallThatEndedSaysHow() {
    #expect(InstallOutcome.make(makeState(.installed)) == .installed(HubVersion("0.1.252")!))
    var abandoned = makeState(.abandoned)
    abandoned.failure = "The server didn't stop, so nothing was installed."
    #expect(InstallOutcome.make(abandoned) == .abandoned(message: "The server didn't stop, so nothing was installed."))
    #expect(InstallOutcome.make(makeState(.checkingApp)) == nil)
}

@Test func theUpdatesFolderKeepsTheInstallsFiles() throws {
    let root = FileManager.default.temporaryDirectory.appending(path: "installs-\(UUID().uuidString)")
    defer { try? FileManager.default.removeItem(at: root) }
    let updates = UpdatesFolder(root: root)
    try FileManager.default.createDirectory(at: root.appending(path: "0.1.250"), withIntermediateDirectories: true)
    try Data(#"["0.1.250", "not a version"]"#.utf8).write(to: updates.badVersionsURL)
    try updates.writeState(makeState(.waitingForApp))
    try updates.writeLaunched(HubVersion("0.1.252")!)
    let record = ReopenRecord(
        version: "0.1.247", savedAt: now, sessions: [.init(id: acme.id, subject: .company(acme.id))], windowedSubjects: [.profile], shownSubject: .company(acme.id),
        page: "jobs", taskIDs: [fix.id]
    )
    try updates.writeReopenRecord(record)

    updates.removeDownloads(keeping: nil)
    #expect(updates.readBadVersions() == [HubVersion("0.1.250")!])
    #expect(updates.readState()?.step == .waitingForApp)
    #expect(try String(contentsOf: updates.launchedURL, encoding: .utf8) == "0.1.252\n")
    #expect(updates.readReopenRecord() == record)
    #expect(!FileManager.default.fileExists(atPath: root.appending(path: "0.1.250").path))
    #expect(record.isFresh(at: now.addingTimeInterval(60)))
    #expect(!record.isFresh(at: now.addingTimeInterval(ReopenRecord.freshFor + 1)))
}

@Test func aVersionThatFailedHereIsntOfferedAgain() {
    let releases = ["mac-v0.1.247", "mac-v0.1.252", "mac-v0.1.250"].map {
        GitHubRelease(tagName: $0, draft: false, prerelease: false, body: "", htmlURL: nil, assets: [])
    }
    let running = HubVersion("0.1.247")!
    #expect(MacReleases.findNewest(in: releases, above: running)?.version == HubVersion("0.1.252"))
    #expect(MacReleases.findNewest(in: releases, above: running, excluding: [HubVersion("0.1.252")!])?.version == HubVersion("0.1.250"))
    #expect(MacReleases.findNewest(in: releases, above: running, excluding: [HubVersion("0.1.252")!, HubVersion("0.1.250")!]) == nil)
}
