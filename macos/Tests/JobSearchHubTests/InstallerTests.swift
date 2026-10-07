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
