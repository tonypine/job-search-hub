import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

/// Answers for a running app `codesign` finds unsigned, as `swift run`
/// builds it.
private struct UnsignedInspector: BundleInspecting {
    func verifySignature(of app: URL) async -> String? { nil }
    func readSigning(of app: URL) async -> BundleSigning { BundleSigning(teamID: nil, designatedRequirement: nil) }
    func readServerBuild(of app: URL) async -> ServerBuild? { nil }
}

/// Answers for a running app signed by a team, so a download goes ahead.
private struct SignedInspector: BundleInspecting {
    func verifySignature(of app: URL) async -> String? { nil }
    func readSigning(of app: URL) async -> BundleSigning {
        BundleSigning(teamID: "ABCDE12345", designatedRequirement: "anchor apple generic")
    }
    func readServerBuild(of app: URL) async -> ServerBuild? { nil }
}

private let releasesPath = "/repos/tonypine/job-search-hub/releases"

/// A Mac release as GitHub lists it. Nothing answers at `.invalid`, so its
/// download fails as a download.
private func makeRelease(_ version: String, prerelease: Bool = false) -> String {
    let url = "https://downloads.invalid/Job-Search-Hub-\(version).zip"
    return #"""
    {"tag_name": "mac-v\#(version)", "draft": false, "prerelease": \#(prerelease), "assets": [
        {"name": "Job-Search-Hub-\#(version).zip", "browser_download_url": "\#(url)"},
        {"name": "Job-Search-Hub-\#(version).zip.sha256", "browser_download_url": "\#(url).sha256"}
    ]}
    """#
}

private func makeFeed(_ releases: String...) -> StubHub.Answer {
    StubHub.Answer(status: 200, body: "[" + releases.joined(separator: ",") + "]")
}

/// A checker running 0.1.250 in its own `Updates/` folder, reading the
/// releases from a stub of GitHub.
@MainActor
private func makeChecker(
    root: URL, inspector: any BundleInspecting, feed: StubHub.Answer
) -> (NewVersionChecker, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [releasesPath: feed])
    let checker = NewVersionChecker(
        updates: UpdatesFolder(root: root), inspector: inspector,
        feed: ReleaseFeed(session: session, appVersion: "0.1.250"), running: HubVersion("0.1.250")!
    )
    return (checker, recording)
}

private func makeRoot() -> URL {
    FileManager.default.temporaryDirectory.appending(path: "NewVersionCheckerTests-\(UUID().uuidString)")
}

/// Leaves the version in `Updates/` as a download that passed its checks.
private func recordChecked(_ version: HubVersion, in updates: UpdatesFolder, changesDatabase: Bool? = false) throws {
    let app = updates.makeFolderURL(for: version).appending(path: "Job Search Hub.app")
    try FileManager.default.createDirectory(at: app.appending(path: "Contents"), withIntermediateDirectories: true)
    try Data().write(to: app.appending(path: "Contents/Info.plist"))
    try updates.recordChecked(CheckedVersion(version: version, app: app, changesDatabase: changesDatabase), at: .now)
}

/// The install log's lines that name the version.
private func readLogLines(_ updates: UpdatesFolder, naming version: HubVersion) -> [String] {
    let log = (try? String(contentsOf: updates.logURL, encoding: .utf8)) ?? ""
    return log.split(separator: "\n").map(String.init).filter { $0.contains(" \(version) ") }
}

@MainActor
@Test func anUnsignedAppRefusesANewVersionBeforeDownloadingIt() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let checker = NewVersionChecker(updates: UpdatesFolder(root: root), inspector: UnsignedInspector())
    // Nothing answers at `.invalid`: a download would fail as a download.
    let release = try #require(MacReleases.parse([
        GitHubRelease(tagName: "mac-v0.1.252", assets: [
            .init(name: "Job-Search-Hub-0.1.252.zip", browserDownloadURL: URL(string: "https://downloads.invalid/Job-Search-Hub-0.1.252.zip")!),
            .init(name: "Job-Search-Hub-0.1.252.zip.sha256", browserDownloadURL: URL(string: "https://downloads.invalid/Job-Search-Hub-0.1.252.zip.sha256")!),
        ]),
    ]).first)

    await #expect(throws: DownloadProblem.runningAppUnsigned) {
        try await checker.download(release)
    }
    #expect(!FileManager.default.fileExists(atPath: checker.updates.makeFolderURL(for: release.version).path))
}


@MainActor
@Test func aVersionRefusedByItsChecksIsNotDownloadedAgain() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (checker, recording) = makeChecker(root: root, inspector: UnsignedInspector(), feed: makeFeed(makeRelease("0.1.252")))
    let newest = try #require(HubVersion("0.1.252"))

    await checker.checkAndWait()
    await checker.checkAndWait()

    #expect(recording.requestCount == 2)
    #expect(checker.facts.failed == FailedVersion(version: newest, problem: .runningAppUnsigned))
    #expect(readLogLines(checker.updates, naming: newest).count == 1)
}

@MainActor
@Test func aVersionWhoseDownloadFailedIsTriedAgainOnTheNextCheck() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (checker, recording) = makeChecker(root: root, inspector: SignedInspector(), feed: makeFeed(makeRelease("0.1.252")))
    let newest = try #require(HubVersion("0.1.252"))

    await checker.checkAndWait()
    await checker.checkAndWait()

    #expect(recording.requestCount == 2)
    let failed = try #require(checker.facts.failed)
    #expect(failed.version == newest)
    guard case .downloadFailed = failed.problem else {
        Issue.record("Expected a failed download, got \(failed.problem).")
        return
    }
    #expect(readLogLines(checker.updates, naming: newest).count == 2)
    #expect(!FileManager.default.fileExists(atPath: checker.updates.makeFolderURL(for: newest).path))
}

@MainActor
@Test func aCheckThatFindsNothingNewerClearsTheReadyAndFailedVersionsAndTheDownloads() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (checker, recording) = makeChecker(root: root, inspector: SignedInspector(), feed: makeFeed(makeRelease("0.1.252")))
    let ready = try #require(HubVersion("0.1.252"))
    try recordChecked(ready, in: checker.updates)

    await checker.checkAndWait()
    recording.setAnswer(makeFeed(makeRelease("0.1.253"), makeRelease("0.1.252")), for: releasesPath)
    await checker.checkAndWait()
    #expect(checker.facts.ready?.version == ready)
    #expect(checker.facts.failed?.version == HubVersion("0.1.253"))

    recording.setAnswer(makeFeed(makeRelease("0.1.250")), for: releasesPath)
    await checker.checkAndWait()

    #expect(checker.facts.ready == nil)
    #expect(checker.facts.failed == nil)
    #expect(!FileManager.default.fileExists(atPath: checker.updates.makeFolderURL(for: ready).path))
    #expect(FileManager.default.fileExists(atPath: checker.updates.logURL.path))
}

@MainActor
@Test func aVersionAlreadyCheckedIsReusedWithoutADownload() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    // Unsigned, a download would be refused before it began.
    let (checker, _) = makeChecker(root: root, inspector: UnsignedInspector(), feed: makeFeed(makeRelease("0.1.252")))
    let newest = try #require(HubVersion("0.1.252"))
    try recordChecked(newest, in: checker.updates, changesDatabase: true)

    await checker.checkAndWait()

    #expect(checker.facts.ready?.version == newest)
    #expect(checker.facts.ready?.changesDatabase == true)
    #expect(checker.facts.failed == nil)
    #expect(checker.updates.readChecked(newest) != nil)
    #expect(readLogLines(checker.updates, naming: newest).isEmpty)
}

@MainActor
@Test func aNewerVersionRefusedThenWithdrawnNoLongerShowsAsRefused() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    let (checker, recording) = makeChecker(root: root, inspector: UnsignedInspector(), feed: makeFeed(makeRelease("0.1.252")))
    let ready = try #require(HubVersion("0.1.252"))
    let refused = try #require(HubVersion("0.1.253"))
    try recordChecked(ready, in: checker.updates)

    await checker.checkAndWait()
    recording.setAnswer(makeFeed(makeRelease("0.1.253"), makeRelease("0.1.252")), for: releasesPath)
    await checker.checkAndWait()
    #expect(checker.facts.ready?.version == ready)
    #expect(checker.facts.failed == FailedVersion(version: refused, problem: .runningAppUnsigned))

    recording.setAnswer(makeFeed(makeRelease("0.1.253", prerelease: true), makeRelease("0.1.252")), for: releasesPath)
    await checker.checkAndWait()

    #expect(checker.facts.ready?.version == ready)
    #expect(checker.facts.failed == nil)
}

@MainActor
@Test func inQAModeTheCheckerReadsTheReleasesFromHUB_RELEASES_URL() async throws {
    let root = makeRoot()
    defer { try? FileManager.default.removeItem(at: root) }
    // Only the stub feed answers; GitHub's path would get a 404.
    let (session, recording) = StubHub.makeSession(answers: ["/releases.json": makeFeed(makeRelease("0.1.252"))])
    let feed = ReleaseFeed(
        session: session, appVersion: "0.1.250", arguments: ["JobSearchHub", "--qa-mode"],
        environment: [ReleaseFeed.urlVariable: "http://localhost:8765/releases.json"]
    )
    let checker = NewVersionChecker(updates: UpdatesFolder(root: root), inspector: UnsignedInspector(), feed: feed, running: HubVersion("0.1.250")!)

    await checker.checkAndWait()

    #expect(recording.lastRequest?.url == URL(string: "http://localhost:8765/releases.json"))
    #expect(checker.facts.failingSince == nil)
    #expect(checker.facts.newestRelease == HubVersion("0.1.252"))
}
