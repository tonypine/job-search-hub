import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// The installer's drain against a stub of the hub, in its own `Updates/`
// folder. Made-up companies only.

private let drainPath = "/v1/drain"
private let nothingRunning = StubHub.Answer(status: 200, body: #"{"draining": true, "running": []}"#)
private let researching = StubHub.Answer(status: 200, body: #"""
{"draining": true, "running": [
    {"type": "agent_run", "kind": "company_triage", "subject": "Initech", "started_at": "2026-10-06T12:00:00Z", "age_seconds": 130}
]}
"""#)

/// An installer with a version to install from a folder that holds no app,
/// so its install fails to start once the hub has drained.
@MainActor
private func makeInstaller(root: URL, drain: StubHub.Answer) -> (Installer, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [drainPath: drain])
    let installer = Installer(updates: UpdatesFolder(root: root))
    installer.makeClient = { HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session) }
    installer.target = Installer.Target(
        version: HubVersion("0.1.252")!, app: root.appending(path: "missing/Job Search Hub.app"), fromMigration: 90, toMigration: 91
    )
    return (installer, recording)
}

private func makeRoot() -> URL {
    FileManager.default.temporaryDirectory.appending(path: "InstallerTests-\(UUID().uuidString)")
}

@MainActor
@Test func anInstallThatCantStartLetsTheHubStartWorkAgain() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, recording) = makeInstaller(root: root, drain: nothingRunning)

    // Nothing runs, so the install starts at once, and fails: the new app
    // isn't there.
    await installer.installWhenTheseFinish()

    #expect(installer.failure != nil)
    #expect(installer.phase == .reviewing)
    #expect(!installer.isDraining)
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    #expect(recording.lastRequest?.url?.path() == drainPath)
}

@MainActor
@Test func cancellingAWaitingInstallLetsTheHubStartWorkAgain() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, recording) = makeInstaller(root: root, drain: researching)

    await installer.installWhenTheseFinish()
    #expect(installer.phase == .waiting)
    #expect(installer.isDraining)

    await installer.cancel()

    #expect(installer.phase == .idle)
    #expect(!installer.isDraining)
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    #expect(recording.lastRequest?.url?.path() == drainPath)
}

@MainActor
@Test func aDrainListThatCantBeReadEndsNothing() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, recording) = makeInstaller(root: root, drain: researching)
    await installer.installWhenTheseFinish()
    #expect(installer.work.running.map(\.title) == ["Researching Initech"])

    recording.setAnswer(StubHub.Answer(status: 500, body: #"{"error":"down"}"#), for: drainPath)
    await installer.refreshWork()

    // The research still runs, as far as the app knows: the install waits.
    #expect(installer.work.running.map(\.title) == ["Researching Initech"])
    #expect(!installer.work.isClear)
    #expect(installer.phase == .waiting)

    // Once the hub answers that it finished, it ticks off.
    recording.setAnswer(nothingRunning, for: drainPath)
    await installer.refreshWork()
    #expect(installer.work.items.map(\.hasEnded) == [true])
    await installer.cancel()
}

/// What the app looked like each time it quit for the install, with the
/// install sheet attached to a window for its first `sheetChecks` looks.
@MainActor
private final class QuitRecorder {
    var sheetChecksLeft: Int
    var quits: [(answersPendingQuit: Bool, isShowingSheet: Bool, sheetChecksLeft: Int)] = []

    init(sheetChecks: Int) {
        sheetChecksLeft = sheetChecks
    }

    func attach(to installer: Installer) {
        installer.hasAttachedSheet = { [unowned self] in
            guard sheetChecksLeft > 0 else { return false }
            sheetChecksLeft -= 1
            return true
        }
        installer.quit = { [unowned self, unowned installer] answersPendingQuit in
            quits.append((answersPendingQuit, installer.isShowingSheet, sheetChecksLeft))
        }
    }
}

private func makeState(_ step: InstallStep, failure: String? = nil) -> InstallState {
    var state = InstallState(
        from: HubVersion("0.1.251")!, to: HubVersion("0.1.252")!, installed: URL(filePath: "/Applications/Job Search Hub.app"),
        newApp: URL(filePath: "/tmp/Job Search Hub.app"), reopenApp: true, fromMigration: 90, toMigration: 91, startedAt: .now, jobPlist: nil
    )
    state.step = step
    state.failure = failure
    return state
}

@MainActor
@Test func installingFromTheSheetClosesItBeforeTheAppQuits() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, _) = makeInstaller(root: root, drain: nothingRunning)
    installer.launchHubUpdate = { _ in }
    installer.stateCheckInterval = .seconds(3600)
    let recorder = QuitRecorder(sheetChecks: 3)
    recorder.attach(to: installer)

    // *Install anyway* in its alert, over the sheet.
    installer.isShowingSheet = true
    installer.isConfirmingInstallAnyway = true
    await installer.installAnyway()

    // AppKit won't quit with a sheet attached: the app quits once, after
    // the sheet and its alert have gone.
    #expect(installer.failure == nil)
    #expect(installer.phase == .starting)
    #expect(!installer.isConfirmingInstallAnyway)
    #expect(recorder.quits.count == 1)
    #expect(recorder.quits.first?.answersPendingQuit == false)
    #expect(recorder.quits.first?.isShowingSheet == false)
    #expect(recorder.quits.first?.sheetChecksLeft == 0)
}

@MainActor
@Test func waitingForWorkInTheSheetClosesItBeforeTheAppQuits() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, _) = makeInstaller(root: root, drain: nothingRunning)
    installer.launchHubUpdate = { _ in }
    installer.stateCheckInterval = .seconds(3600)
    let recorder = QuitRecorder(sheetChecks: 2)
    recorder.attach(to: installer)

    // *Install when these finish*, with nothing left to wait for.
    installer.isShowingSheet = true
    await installer.installWhenTheseFinish()

    #expect(recorder.quits.count == 1)
    #expect(recorder.quits.first?.isShowingSheet == false)
    #expect(recorder.quits.first?.sheetChecksLeft == 0)
}

@MainActor
@Test func anInstallAbandonedWhileTheAppStillRunsEndsInTheApp() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, recording) = makeInstaller(root: root, drain: nothingRunning)
    let updates = UpdatesFolder(root: root)
    installer.launchHubUpdate = { _ in try updates.writeState(makeState(.waitingForApp)) }
    installer.stateCheckInterval = .milliseconds(10)
    // The quit never happens, as when AppKit aborts it.
    installer.hasAttachedSheet = { false }
    installer.quit = { _ in }

    await installer.installWhenTheseFinish()
    #expect(installer.phase == .starting)
    #expect(installer.isDraining)

    // hub-update gives up waiting.
    try updates.writeState(makeState(.abandoned, failure: "Job Search Hub didn't quit, so nothing was installed."))
    for _ in 0..<500 where installer.phase == .starting {
        try await Task.sleep(for: .milliseconds(10))
    }

    // No more "Installing": the failure shows, and the hub and the app
    // start work again.
    #expect(installer.phase == .idle)
    #expect(installer.failure?.report.advice == "Job Search Hub didn't quit, so nothing was installed.")
    #expect(installer.notice?.outcome == .abandoned(message: "Job Search Hub didn't quit, so nothing was installed."))
    #expect(!installer.isDraining)
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    #expect(updates.readState() == nil)
    #expect(!FileManager.default.fileExists(atPath: updates.reopenURL.path))
}
