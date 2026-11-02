import AppKit
import Foundation
@testable import JobSearchHub
import JobSearchHubCore
import Testing

/// Answers for a QA build, which no team signed.
private struct QABuildInspector: BundleInspecting {
    func verifySignature(of app: URL) async -> String? { nil }
    func readSigning(of app: URL) async -> BundleSigning { BundleSigning(teamID: nil, designatedRequirement: nil) }
    func readServerBuild(of app: URL) async -> ServerBuild? { nil }
}

/// The fixtures committed for QA, in `macos/Tests/Fixtures/`.
private let committedFixtures = URL(filePath: #filePath).deletingLastPathComponent().deletingLastPathComponent().appending(path: "Fixtures")

/// A copy of the committed fixture, as Symphony's QA launch copies it to
/// the QA host: the app writes in the copy, never in the checkout.
private func copyFixture(_ name: String) throws -> QAFixtures {
    let copy = FileManager.default.temporaryDirectory.appending(path: "QAFixtureVersionTests-\(UUID().uuidString)")
    try FileManager.default.copyItem(at: committedFixtures.appending(path: name), to: copy)
    return QAFixtures(directory: copy)
}

/// What a QA launch with the fixture builds: the Updates folder, the
/// running version and the release feed all come from it.
@MainActor
private func makeChecker(_ fixtures: QAFixtures) throws -> NewVersionChecker {
    let running = try #require(fixtures.readRunningVersion())
    return NewVersionChecker(
        updates: UpdatesFolder.makeDefault(home: URL(filePath: "/Users/qa"), fixtures: fixtures), inspector: QABuildInspector(),
        feed: ReleaseFeed(appVersion: running.description, arguments: ["JobSearchHub"], environment: [:], isQABuild: true, bundledURL: nil, fixtures: fixtures),
        running: running
    )
}

@MainActor
private func makePage(_ checker: NewVersionChecker, installer: Installer) -> VersionPage {
    VersionPage(
        facts: checker.facts, status: checker.status, installedAt: nil, serverVersion: nil, previousVersion: checker.previousVersion,
        hasInstallLog: checker.hasInstallLog, runningReleaseURL: nil, now: .now, installBlockedReason: installer.cannotInstallReason
    )
}

@MainActor
@Test func theNothingLostFixtureMakesSettingsVersionOfferToGoBack() async throws {
    let fixtures = try copyFixture("updates-nothing-lost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let checker = try makeChecker(fixtures)
    let installer = Installer(updates: checker.updates)

    await checker.checkAndWait()
    let page = makePage(checker, installer: installer)

    #expect(page.previousVersion == HubVersion("0.1.247"))
    #expect(page.goBackTitle == "Go back to 0.1.247…")
    #expect(page.status == .upToDate(newest: HubVersion("0.1.252"), checkedAt: checker.facts.lastCheckedAt))
    #expect(page.installBlockedReason == Installer.qaFixtureReason)
}

@MainActor
@Test func theWithdrawnFixtureShowsTheRunningVersionWithdrawnWithAWayBack() async throws {
    let fixtures = try copyFixture("updates-withdrawn")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let checker = try makeChecker(fixtures)

    await checker.checkAndWait()
    let page = makePage(checker, installer: Installer(updates: checker.updates))

    #expect(page.status == .withdrawn(try #require(HubVersion("0.1.252"))))
    #expect(page.goBackTitle == "Go back to 0.1.247…")
}

@MainActor
@Test func withAFixturesUpdatesFolderNothingIsHandedToHubUpdate() throws {
    let fixtures = try copyFixture("updates-nothing-lost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let updates = UpdatesFolder.makeDefault(home: URL(filePath: "/Users/qa"), fixtures: fixtures)
    let installer = Installer(updates: updates)

    #expect(throws: InstallError.self) {
        try installer.refuseQAFixture("Go back to 0.1.247")
    }

    // The request is recorded in the copy's log, and nothing else is
    // written: no state for hub-update, no copy of it, no launchd job.
    let log = try String(contentsOf: updates.logURL, encoding: .utf8)
    #expect(log.contains("QA fixture: Go back to 0.1.247 was asked; hub-update wasn't started."))
    #expect(!FileManager.default.fileExists(atPath: updates.stateURL.path))
    #expect(!FileManager.default.fileExists(atPath: updates.installerURL.path))
    #expect(installer.cannotInstallReason == Installer.qaFixtureReason)
}

@MainActor
@Test func theRealUpdatesFolderIsNotRefused() throws {
    let installer = Installer(updates: UpdatesFolder(root: FileManager.default.temporaryDirectory.appending(path: "QAFixtureVersionTests-\(UUID().uuidString)")))

    try installer.refuseQAFixture("Install 0.1.252")
    #expect(installer.cannotInstallReason != Installer.qaFixtureReason)
    #expect(!FileManager.default.fileExists(atPath: installer.updates.logURL.path))
}

@Test func theQAAppearanceDrawsAquaOrDarkAqua() {
    #expect(QAAppearance.light.appearanceName == .aqua)
    #expect(QAAppearance.dark.appearanceName == .darkAqua)
}
