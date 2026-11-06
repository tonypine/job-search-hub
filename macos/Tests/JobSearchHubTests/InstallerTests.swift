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

    /// `isSheetShowing` reads a sheet other than the install's.
    func attach(to installer: Installer, isSheetShowing: (@MainActor () -> Bool)? = nil) {
        installer.hasAttachedSheet = { [unowned self] in
            guard sheetChecksLeft > 0 else { return false }
            sheetChecksLeft -= 1
            return true
        }
        installer.quit = { [unowned self, unowned installer] answersPendingQuit in
            quits.append((answersPendingQuit, isSheetShowing?() ?? installer.isShowingSheet, sheetChecksLeft))
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

@MainActor
@Test func anInstallWhoseStateIsGoneWhileTheAppStillRunsEndsInTheApp() async throws {
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
    #expect(FileManager.default.fileExists(atPath: updates.reopenURL.path))

    // The install's state is gone, with no finished step to read.
    try FileManager.default.removeItem(at: updates.stateURL)
    for _ in 0..<500 where installer.phase == .starting {
        try await Task.sleep(for: .milliseconds(10))
    }

    // The app leaves "Installing" with its own advice, and no notice.
    #expect(installer.phase == .idle)
    #expect(installer.failure?.report.advice == "The install stopped before the app quit, so nothing was installed.")
    #expect(installer.notice == nil)
    #expect(!installer.isDraining)
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    #expect(!FileManager.default.fileExists(atPath: updates.reopenURL.path))
}

@MainActor
@Test func aSheetThatNeverClosesStillLetsTheAppQuitOnce() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, _) = makeInstaller(root: root, drain: nothingRunning)
    installer.launchHubUpdate = { _ in }
    installer.stateCheckInterval = .seconds(3600)
    installer.sheetCloseTimeout = .milliseconds(200)
    // The sheet never detaches.
    let recorder = QuitRecorder(sheetChecks: .max)
    recorder.attach(to: installer)

    installer.isShowingSheet = true
    await installer.installAnyway()

    // After the deadline the app asks to quit anyway, once.
    #expect(recorder.quits.count == 1)
    #expect(recorder.quits.first?.isShowingSheet == false)
    #expect(installer.phase == .starting)
    #expect(!installer.isShowingSheet)
}

// MARK: Going back

/// The going back's sheet, offering 0.1.247 kept in `previous/`, over an
/// installer that starts no launchd job.
@MainActor
private func makeGoingBackOffer(root: URL, prepare: @MainActor (Installer) -> Void = { _ in }) async throws -> (Installer, GoingBack) {
    let updates = UpdatesFolder(root: root)
    let contents = updates.previousURL.appending(path: "Job Search Hub.app/Contents")
    try FileManager.default.createDirectory(at: contents, withIntermediateDirectories: true)
    try PropertyListSerialization.data(fromPropertyList: ["CFBundleShortVersionString": "0.1.247"], format: .xml, options: 0)
        .write(to: contents.appending(path: "Info.plist"))
    let installer = Installer(updates: updates)
    installer.readUnsavedEdits = { [] }
    prepare(installer)
    let goingBack = GoingBack(
        updates: updates, running: HubVersion("0.1.252"), qaStandIn: QAGoingBack(previousMigration: 91, databaseMigration: 91), installer: installer
    )
    await goingBack.offerPrevious(.settings)
    #expect(goingBack.offer != nil)
    #expect(goingBack.isShowingSheet)
    return (installer, goingBack)
}

private func makeGoingBackState(_ step: InstallStep, failure: String? = nil) -> InstallState {
    var state = InstallState.makeGoingBack(
        from: HubVersion("0.1.252")!, to: HubVersion("0.1.247")!, installed: URL(filePath: "/Applications/Job Search Hub.app"),
        app: URL(filePath: "/tmp/Job Search Hub.app"), databaseMigration: 91, restoredMigration: 91, startedAt: .now, jobPlist: nil
    )
    state.step = step
    if let failure {
        state.failure = failure
    }
    return state
}

@MainActor
@Test func goingBackClosesItsSheetBeforeTheAppQuits() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, goingBack) = try await makeGoingBackOffer(root: root)
    installer.launchGoingBack = { _ in }
    installer.stateCheckInterval = .seconds(3600)
    let recorder = QuitRecorder(sheetChecks: 3)
    recorder.attach(to: installer, isSheetShowing: { goingBack.isShowingSheet })

    await goingBack.confirm()

    // AppKit won't quit with a sheet attached: the app quits once, after the
    // sheet has gone.
    #expect(goingBack.failure == nil)
    #expect(goingBack.isStarting)
    #expect(installer.phase == .starting)
    #expect(recorder.quits.count == 1)
    #expect(recorder.quits.first?.answersPendingQuit == false)
    #expect(recorder.quits.first?.isShowingSheet == false)
    #expect(recorder.quits.first?.sheetChecksLeft == 0)
}

@MainActor
@Test func aGoingBackAbandonedWhileTheAppStillRunsSaysSoInItsSheet() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, goingBack) = try await makeGoingBackOffer(root: root)
    let updates = installer.updates
    installer.launchGoingBack = { _ in try updates.writeState(makeGoingBackState(.waitingForApp)) }
    installer.stateCheckInterval = .milliseconds(10)
    // The quit never happens, as when AppKit aborts it.
    installer.hasAttachedSheet = { false }
    installer.quit = { _ in }

    await goingBack.confirm()
    #expect(goingBack.isStarting)
    #expect(!goingBack.isShowingSheet)
    #expect(installer.phase == .starting)

    // hub-update gives up waiting.
    let reason = "Job Search Hub didn't quit, so the hub stayed on 0.1.252."
    try updates.writeState(makeGoingBackState(.abandoned, failure: reason))
    for _ in 0..<500 where goingBack.isStarting {
        try await Task.sleep(for: .milliseconds(10))
    }

    // No more "Going back…": the sheet is back with the reason, and the
    // owner can try again.
    #expect(!goingBack.isStarting)
    #expect(goingBack.isShowingSheet)
    #expect(goingBack.offer != nil)
    #expect(goingBack.failure?.title == "Couldn't go back to 0.1.247")
    #expect(goingBack.failure?.report.advice == reason)
    #expect(installer.failure == nil)
    #expect(installer.phase == .idle)
    #expect(installer.notice?.outcome == .abandoned(message: reason))
    #expect(updates.readState() == nil)
    #expect(!FileManager.default.fileExists(atPath: updates.reopenURL.path))
}

@MainActor
@Test func aGoingBackThatLaunchdRefusesStaysInItsSheet() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (installer, goingBack) = try await makeGoingBackOffer(root: root)
    installer.launchGoingBack = { _ in throw InstallError("launchd didn't start hub-update: no.") }
    let recorder = QuitRecorder(sheetChecks: 0)
    recorder.attach(to: installer, isSheetShowing: { goingBack.isShowingSheet })

    await goingBack.confirm()

    #expect(goingBack.failure?.report.advice == "launchd didn't start hub-update: no.")
    #expect(!goingBack.isStarting)
    #expect(goingBack.isShowingSheet)
    #expect(installer.phase == .idle)
    #expect(recorder.quits.isEmpty)
}

/// The edits an installer reads, and the go-backs it hands to `hub-update`.
@MainActor
private final class GoingBackStandIn {
    var edits: [UnsavedEdit]
    var launches = 0

    init(edits: [UnsavedEdit]) {
        self.edits = edits
    }

    func attach(to installer: Installer) {
        installer.readUnsavedEdits = { self.edits }
        installer.launchGoingBack = { _ in self.launches += 1 }
    }
}

@MainActor
@Test func aGoingBackWithAnUnsavedEditWaitsForItWithoutLaunchingOrQuitting() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let standIn = GoingBackStandIn(edits: [UnsavedEdit(id: "criteria", title: "Criteria")])
    let (installer, goingBack) = try await makeGoingBackOffer(root: root) { standIn.attach(to: $0) }
    installer.stateCheckInterval = .seconds(3600)
    let recorder = QuitRecorder(sheetChecks: 0)
    recorder.attach(to: installer, isSheetShowing: { goingBack.isShowingSheet })
    #expect(goingBack.work.hasUnsavedEdits)

    await goingBack.confirm()

    // Nothing started: the sheet stays, and names the edit.
    #expect(standIn.launches == 0)
    #expect(recorder.quits.isEmpty)
    #expect(goingBack.failure?.report.advice == "Criteria has unsaved edits. Save or discard them first: going back would lose them.")
    #expect(!goingBack.isStarting)
    #expect(goingBack.isShowingSheet)
    #expect(installer.phase == .idle)
    #expect(!FileManager.default.fileExists(atPath: installer.updates.reopenURL.path))

    // Saved, it goes ahead, without the sheet having looked again.
    standIn.edits = []
    await goingBack.confirm()
    #expect(standIn.launches == 1)
    #expect(recorder.quits.count == 1)
    #expect(goingBack.failure == nil)
    #expect(!goingBack.work.hasUnsavedEdits)
}

@MainActor
@Test func aGoingBackRefusesAnEditMadeAfterTheSheetLooked() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let standIn = GoingBackStandIn(edits: [])
    let (installer, goingBack) = try await makeGoingBackOffer(root: root) { standIn.attach(to: $0) }
    let recorder = QuitRecorder(sheetChecks: 0)
    recorder.attach(to: installer, isSheetShowing: { goingBack.isShowingSheet })
    #expect(!goingBack.work.hasUnsavedEdits)

    standIn.edits = [UnsavedEdit(id: "prompt", title: "The job brief prompt")]
    await goingBack.confirm()

    #expect(standIn.launches == 0)
    #expect(recorder.quits.isEmpty)
    #expect(goingBack.work.hasUnsavedEdits)
    #expect(goingBack.failure?.report.advice == "The job brief prompt has unsaved edits. Save or discard them first: going back would lose them.")
}

@MainActor
@Test func aGoingBackOfferNamesTheWorkItWouldStopAndGoesAheadOverIt() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (session, _) = StubHub.makeSession(answers: [drainPath: researching])
    let (installer, goingBack) = try await makeGoingBackOffer(root: root) { installer in
        installer.makeClient = { HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session) }
    }
    installer.launchGoingBack = { _ in }
    installer.stateCheckInterval = .seconds(3600)
    let recorder = QuitRecorder(sheetChecks: 0)
    recorder.attach(to: installer, isSheetShowing: { goingBack.isShowingSheet })

    #expect(goingBack.work.running.map(\.title).contains("Researching Initech"))
    #expect(goingBack.work.costOfGoingBack(to: HubVersion("0.1.247")!).contains("Researching Initech stops; run it again afterwards."))

    // The sheet said what stops, so the owner's confirm is enough.
    await goingBack.confirm()
    #expect(goingBack.failure == nil)
    #expect(recorder.quits.count == 1)
}
