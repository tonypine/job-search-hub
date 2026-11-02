import Foundation
import JobSearchHubCore
import Testing

/// The fixtures committed for QA, in `macos/Tests/Fixtures/`.
private let committedFixtures = URL(filePath: #filePath).deletingLastPathComponent().deletingLastPathComponent().appending(path: "Fixtures")

private let home = URL(filePath: "/Users/qa")
private let realUpdates = home.appending(path: "Library/Application Support/JobSearchHub/Updates")

private func makeFixture(updates: Bool = true, runningVersion: String? = nil) throws -> URL {
    let directory = FileManager.default.temporaryDirectory.appending(path: "QAFixturesTests-\(UUID().uuidString)")
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    if updates {
        try FileManager.default.createDirectory(at: directory.appending(path: "updates"), withIntermediateDirectories: true)
    }
    if let runningVersion {
        try Data(runningVersion.utf8).write(to: directory.appending(path: "running-version"))
    }
    return directory
}

private func environment(_ directory: URL) -> [String: String] {
    [QAFixtures.directoryVariable: directory.path]
}

// MARK: The fixture

@Test func theFixtureIsReadOnlyInQAMode() throws {
    let directory = try makeFixture()
    defer { try? FileManager.default.removeItem(at: directory) }

    #expect(QAFixtures.find(isQAMode: true, environment: environment(directory))?.directory.path == directory.path)
    #expect(QAFixtures.find(isQAMode: false, environment: environment(directory)) == nil)
    #expect(QAFixtures.find(isQAMode: true, environment: [:]) == nil)
    #expect(QAFixtures.find(isQAMode: true, environment: [QAFixtures.directoryVariable: " "]) == nil)
}

@Test func qaModeIsAQABuildOrTheFlag() {
    #expect(QAMode.isOn(arguments: ["JobSearchHub"], isQABuild: true))
    #expect(QAMode.isOn(arguments: ["JobSearchHub", "--qa-mode"], isQABuild: false))
    #expect(!QAMode.isOn(arguments: ["JobSearchHub"], isQABuild: false))
}

// MARK: The Updates folder

@Test func inQAModeTheFixturesUpdatesFolderStandsInForTheRealOne() throws {
    let directory = try makeFixture()
    defer { try? FileManager.default.removeItem(at: directory) }

    let updates = UpdatesFolder.makeDefault(home: home, fixtures: QAFixtures.find(isQAMode: true, environment: environment(directory)))

    #expect(updates.root.path == directory.appending(path: "updates").path)
    #expect(updates.isQAFixture)
    #expect(updates.logURL.path == directory.appending(path: "updates/install.log").path)
}

@Test func outsideQAModeTheUpdatesFolderIsTheRealOne() throws {
    let directory = try makeFixture()
    defer { try? FileManager.default.removeItem(at: directory) }

    let updates = UpdatesFolder.makeDefault(home: home, fixtures: QAFixtures.find(isQAMode: false, environment: environment(directory)))

    #expect(updates.root == realUpdates)
    #expect(!updates.isQAFixture)
    #expect(UpdatesFolder.makeDefault(home: home, fixtures: nil).root == realUpdates)
}

@Test func aFixtureWithoutAnUpdatesFolderLeavesTheRealOne() throws {
    let directory = try makeFixture(updates: false)
    defer { try? FileManager.default.removeItem(at: directory) }

    let updates = UpdatesFolder.makeDefault(home: home, fixtures: QAFixtures(directory: directory))

    #expect(updates.root == realUpdates)
    #expect(!updates.isQAFixture)
}

// MARK: The running version

@Test func theFixtureNamesTheRunningVersion() throws {
    let named = try makeFixture(runningVersion: "0.1.252\n")
    let unnamed = try makeFixture()
    let garbled = try makeFixture(runningVersion: "latest")
    defer {
        for directory in [named, unnamed, garbled] {
            try? FileManager.default.removeItem(at: directory)
        }
    }

    #expect(QAFixtures(directory: named).readRunningVersion() == HubVersion("0.1.252"))
    #expect(QAFixtures(directory: unnamed).readRunningVersion() == nil)
    #expect(QAFixtures(directory: garbled).readRunningVersion() == nil)
}

// MARK: The release feed

@Test func inQAModeTheFixturesReleasesAreTheFeedWhenNothingElseNamesOne() throws {
    let fixtures = QAFixtures(directory: committedFixtures.appending(path: "updates-withdrawn"))
    let releases = try #require(fixtures.releasesURL)

    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub"], environment: [:], isQABuild: true, fixtures: fixtures) == releases)
    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub"], environment: [:], isQABuild: false, fixtures: fixtures) == GitHubRepository.releasesAPIURL)
    let stub = [ReleaseFeed.urlVariable: "http://localhost:8765/releases.json"]
    #expect(ReleaseFeed.makeURL(arguments: ["JobSearchHub"], environment: stub, isQABuild: true, fixtures: fixtures) == URL(string: "http://localhost:8765/releases.json"))
}

@Test func aFileFeedIsReadFromTheFile() async throws {
    let fixtures = QAFixtures(directory: committedFixtures.appending(path: "updates-withdrawn"))
    let feed = ReleaseFeed(url: try #require(fixtures.releasesURL))

    let releases = try await feed.fetch()

    #expect(MacReleases.isWithdrawn(try #require(HubVersion("0.1.252")), in: releases))
    #expect(!MacReleases.isWithdrawn(try #require(HubVersion("0.1.247")), in: releases))
}

// MARK: The committed fixtures

@Test(arguments: ["updates-nothing-lost", "updates-withdrawn"])
func eachCommittedFixtureKeepsAnOlderVersionThanItRuns(_ name: String) throws {
    let fixtures = QAFixtures(directory: committedFixtures.appending(path: name))
    let updates = UpdatesFolder.makeDefault(home: home, fixtures: fixtures)
    let running = try #require(fixtures.readRunningVersion())
    let previous = try #require(updates.readPreviousVersion())

    #expect(updates.isQAFixture)
    #expect(!running.isLocalBuild)
    #expect(previous < running)
    #expect(updates.readBadVersions().isEmpty)
}

// MARK: The appearance

@Test func theAppearanceMapsLightAndDark() {
    #expect(QAAppearance.read(isQAMode: true, environment: [QAAppearance.variable: "light"]) == .light)
    #expect(QAAppearance.read(isQAMode: true, environment: [QAAppearance.variable: "dark"]) == .dark)
    #expect(QAAppearance.read(isQAMode: true, environment: [QAAppearance.variable: " Dark\n"]) == .dark)
}

@Test func anUnsetOrUnknownAppearanceIsIgnored() {
    #expect(QAAppearance.read(isQAMode: true, environment: [:]) == nil)
    #expect(QAAppearance.read(isQAMode: true, environment: [QAAppearance.variable: ""]) == nil)
    #expect(QAAppearance.read(isQAMode: true, environment: [QAAppearance.variable: "sepia"]) == nil)
}

@Test func theAppearanceIsReadOnlyInQAMode() {
    #expect(QAAppearance.read(isQAMode: false, environment: [QAAppearance.variable: "dark"]) == nil)
    #expect(QAAppearance.read(isQAMode: false, environment: [QAAppearance.variable: "light"]) == nil)
}
