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
        hasInstallLog: checker.hasInstallLog, runningReleaseURL: nil, now: .now, installBlockedReason: installer.cannotInstallReason,
        goBackBlockedReason: installer.cannotGoBackReason
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
    #expect(page.goBackTarget == HubVersion("0.1.247"))
    #expect(page.status == .upToDate(newest: HubVersion("0.1.252"), checkedAt: checker.facts.lastCheckedAt))
    #expect(page.installBlockedReason == Installer.qaFixtureReason)
    // Going back opens its sheet, so QA sees the cost; only *Go back* is
    // refused.
    #expect(page.goBackBlockedReason == nil)
}

@MainActor
@Test func theWithdrawnFixtureShowsTheRunningVersionWithdrawnWithAWayBack() async throws {
    let fixtures = try copyFixture("updates-withdrawn")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let checker = try makeChecker(fixtures)

    await checker.checkAndWait()
    let page = makePage(checker, installer: Installer(updates: checker.updates))

    #expect(page.status == .withdrawn(try #require(HubVersion("0.1.252"))))
    #expect(page.goBackTarget == HubVersion("0.1.247"))
    #expect(page.goBackBlockedReason == nil)
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

// MARK: Going back with a fixture

/// What a QA launch with the fixture weighs going back with: its Updates
/// folder, its running version and its stand-in for the servers.
@MainActor
private func makeGoingBack(_ fixtures: QAFixtures) throws -> GoingBack {
    GoingBack(
        updates: UpdatesFolder.makeDefault(home: URL(filePath: "/Users/qa"), fixtures: fixtures), running: try #require(fixtures.readRunningVersion()),
        qaStandIn: try #require(fixtures.readGoingBack())
    )
}

@MainActor
@Test func theNothingLostFixtureOpensTheSheetSayingNothingIsLost() async throws {
    let fixtures = try copyFixture("updates-nothing-lost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let goingBack = try makeGoingBack(fixtures)

    await goingBack.offerPrevious(.settings)

    let offer = try #require(goingBack.offer)
    #expect(goingBack.isShowingSheet)
    #expect(goingBack.failure == nil)
    #expect(offer.target == HubVersion("0.1.247"))
    #expect(offer.cost == .nothingLost)
}

@MainActor
@Test func theCountedCostFixtureOpensTheSheetCountingWhatWouldBeLost() async throws {
    let fixtures = try copyFixture("updates-counted-cost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let goingBack = try makeGoingBack(fixtures)

    await goingBack.offerPrevious(.settings)

    let offer = try #require(goingBack.offer)
    #expect(offer.target == HubVersion("0.1.247"))
    #expect(offer.databaseMigration == 91)
    #expect(offer.restoredMigration == 88)
    #expect(offer.cost.lostSentence == "Since then: 3 jobs found, 1 card moved, 2 people added, 4 updates. These would be lost.")
}

@MainActor
@Test func theWithdrawnFixtureWeighsTheBannersWayBack() async throws {
    let fixtures = try copyFixture("updates-withdrawn")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let goingBack = try makeGoingBack(fixtures)

    await goingBack.offerPrevious(.withdrawn)

    let offer = try #require(goingBack.offer)
    #expect(offer.cost.summarize(target: offer.target) == "Going back to 0.1.247 loses nothing.")
}

@MainActor
@Test func theFirstDayCrashFixtureAsksToGoBackAndTheOthersDont() async throws {
    let crashed = try copyFixture("updates-first-day-crash")
    let quiet = try copyFixture("updates-counted-cost")
    defer {
        try? FileManager.default.removeItem(at: crashed.directory)
        try? FileManager.default.removeItem(at: quiet.directory)
    }
    let goingBack = try makeGoingBack(crashed)

    #expect(goingBack.didCrashInFirstDay(now: .now))
    let quietGoingBack = try makeGoingBack(quiet)
    #expect(!quietGoingBack.didCrashInFirstDay(now: .now))

    await goingBack.offerPrevious(.crash)
    let offer = try #require(goingBack.offer)
    #expect(offer.reason == .crash)
    #expect(offer.cost.losesWrites)
    #expect(offer.installedAt.map { abs($0.timeIntervalSinceNow + 2 * 60 * 60) < 60 } == true)
}

@MainActor
@Test func theCountedCostFixtureListsTheOlderVersionWithItsCopy() throws {
    let fixtures = QAFixtures(directory: committedFixtures.appending(path: "updates-counted-cost"))
    let releases = try HubJSON.makeDecoder().decode([GitHubRelease].self, from: Data(contentsOf: try #require(fixtures.releasesURL)))
    let report = try #require(fixtures.readGoingBack()?.report)

    let older = OlderVersions.pick(releases, running: try #require(HubVersion("0.1.252")), previous: HubVersion("0.1.247"), bad: [], report: report)

    #expect(older.map(\.version) == [HubVersion("0.1.240")])
    #expect(older.first?.migration == 85)
}

@MainActor
@Test func goingBackWithAFixtureIsRecordedInItsLogAndRefused() async throws {
    let fixtures = try copyFixture("updates-nothing-lost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let updates = UpdatesFolder.makeDefault(home: URL(filePath: "/Users/qa"), fixtures: fixtures)
    let installer = Installer(updates: updates)
    let state = InstallState.makeGoingBack(
        from: try #require(HubVersion("0.1.252")), to: try #require(HubVersion("0.1.247")), installed: URL(filePath: "/Users/qa/Applications/JobSearchHub.app"),
        app: updates.previousURL.appending(path: "JobSearchHub.app"), databaseMigration: 91, restoredMigration: 91, startedAt: .now, jobPlist: nil
    )

    await #expect(throws: InstallError.self) {
        try await installer.goBack(state)
    }

    let log = try String(contentsOf: updates.logURL, encoding: .utf8)
    #expect(log.contains("QA fixture: Go back to 0.1.247 was asked; hub-update wasn't started."))
    #expect(!FileManager.default.fileExists(atPath: updates.stateURL.path))
}

@MainActor
@Test func aGoingBackThatCantStartSaysWhyInItsSheet() async throws {
    let fixtures = try copyFixture("updates-nothing-lost")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let updates = UpdatesFolder.makeDefault(home: URL(filePath: "/Users/qa"), fixtures: fixtures)
    let goingBack = GoingBack(
        updates: updates, running: try #require(fixtures.readRunningVersion()), qaStandIn: try #require(fixtures.readGoingBack()),
        installer: Installer(updates: updates)
    )
    await goingBack.offerPrevious(.settings)

    await goingBack.confirm()

    let failure = try #require(goingBack.failure)
    #expect(failure.title == "Couldn't go back to 0.1.247")
    #expect(failure.report.advice == Installer.qaFixtureReason)
    #expect(failure.report.details == nil)
    #expect(!goingBack.isStarting)
    #expect(goingBack.isShowingSheet)
}

// MARK: The withdrawn banner with a fixture

@MainActor
@Test func aWithdrawnBannerDismissedWithAFixtureLastsOnlyThatLaunch() async throws {
    let key = GoingBack.withdrawnDismissedKey
    let saved = UserDefaults.standard.object(forKey: key)
    // A banner dismissed on an earlier launch of this host, before fixtures
    // kept it to themselves, doesn't hide it either.
    UserDefaults.standard.set("0.1.252", forKey: key)
    defer { UserDefaults.standard.set(saved, forKey: key) }
    let fixtures = try copyFixture("updates-withdrawn")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let goingBack = try makeGoingBack(fixtures)

    await goingBack.readWithdrawnBanner()

    #expect(!goingBack.isWithdrawnBannerDismissed)
    let offer = try #require(goingBack.withdrawnOffer)
    #expect(offer.cost.summarize(target: offer.target) == "Going back to 0.1.247 loses nothing.")

    // Every check reads the banner again: it stays dismissed on this launch.
    UserDefaults.standard.removeObject(forKey: key)
    goingBack.dismissWithdrawnBanner()
    await goingBack.readWithdrawnBanner()
    #expect(goingBack.isWithdrawnBannerDismissed)
    #expect(UserDefaults.standard.string(forKey: key) == nil)

    // The next launch shows it again.
    let nextLaunch = try makeGoingBack(fixtures)
    await nextLaunch.readWithdrawnBanner()
    #expect(!nextLaunch.isWithdrawnBannerDismissed)
}

// MARK: After going back

@MainActor
@Test func theAfterGoingBackFixtureDoesntOfferTheVersionItLeft() async throws {
    let fixtures = try copyFixture("updates-after-going-back")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let checker = try makeChecker(fixtures)

    await checker.checkAndWait()
    let page = makePage(checker, installer: Installer(updates: checker.updates))

    #expect(page.facts.running == HubVersion("0.1.247"))
    #expect(page.facts.ready == nil)
    #expect(page.facts.failed == nil)
    #expect(page.previousVersion == nil)
    #expect(page.goBackTarget == nil)
    #expect(page.status == .newerNotOffered(HubVersion("0.1.252")!, checkedAt: checker.facts.lastCheckedAt))
}

/// Going back past the kept version, to an older download, leaves the newer
/// one in `previous/`: it's listed, and not offered.
@MainActor
@Test func aKeptVersionNewerThanTheRunningOneIsntOffered() async throws {
    let fixtures = try copyFixture("updates-after-going-back")
    defer { try? FileManager.default.removeItem(at: fixtures.directory) }
    let contents = fixtures.directory.appending(path: "updates/previous/JobSearchHub.app/Contents")
    try FileManager.default.createDirectory(at: contents, withIntermediateDirectories: true)
    try PropertyListSerialization.data(fromPropertyList: ["CFBundleShortVersionString": "0.1.252"], format: .xml, options: 0)
        .write(to: contents.appending(path: "Info.plist"))
    let checker = try makeChecker(fixtures)

    await checker.checkAndWait()
    let page = makePage(checker, installer: Installer(updates: checker.updates))

    #expect(page.previousVersion == HubVersion("0.1.252"))
    #expect(page.goBackTarget == nil)
}
