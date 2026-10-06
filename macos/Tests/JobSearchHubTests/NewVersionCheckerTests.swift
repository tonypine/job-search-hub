import Foundation
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

@MainActor
@Test func anUnsignedAppRefusesANewVersionBeforeDownloadingIt() async throws {
    let root = FileManager.default.temporaryDirectory.appending(path: "NewVersionCheckerTests-\(UUID().uuidString)")
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
