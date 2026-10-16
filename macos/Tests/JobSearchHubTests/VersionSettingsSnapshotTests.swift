import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// Settings › Version, drawn offscreen with ImageRenderer in each state. With
// HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the PNGs are written there.

@MainActor
@Test(arguments: VersionSnapshots.names)
func settingsVersionDrawsOffscreen(state: String) throws {
    let image = try #require(VersionSnapshots.render(state))

    #expect(image.size.width == 620)
    #expect(image.size.height > 200)
    if let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty {
        try VersionSnapshots.write(image, named: state, to: URL(filePath: folder))
    }
}

@MainActor
@Test func eachStateSaysItsOwnThing() {
    #expect(VersionSnapshots.makePage("ready").status == .ready)
    #expect(VersionSnapshots.makePage("checking").status == .checking)
    #expect(VersionSnapshots.makePage("local-build").status == .localBuild(commit: "abc1234"))
    #expect(VersionSnapshots.makePage("withdrawn").status == .withdrawn(VersionSnapshots.ready.version))
    if case .failedChecks = VersionSnapshots.makePage("failed-checks").status {} else { Issue.record("not failed checks") }
    if case .cannotCheck = VersionSnapshots.makePage("cannot-check").status {} else { Issue.record("not cannot check") }
    if case .upToDate = VersionSnapshots.makePage("up-to-date").status {} else { Issue.record("not up to date") }
}
