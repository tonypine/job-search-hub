import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI

/// Settings › Version in each of its states, with made-up changes, for the
/// offscreen snapshots.
@MainActor
enum VersionSnapshots {
    static let now = Date(timeIntervalSince1970: 1_791_300_000)
    static let running = HubVersion("0.1.247")!
    nonisolated static let names = ["ready", "ready-all-changes", "checking", "up-to-date", "local-build", "failed-checks", "cannot-check", "withdrawn"]

    static let ready = ReadyVersion(
        version: HubVersion("0.1.252")!,
        whatsNew: WhatsNew.merge(MacReleases.getOffered([
            GitHubRelease(tagName: "mac-v0.1.252", body: """
            ## New

            - Mac: Suggest a second route for unanswered applications (#67)

            ## Fixed

            - Server: Retry board polls that time out (#71)

            ## Other changes

            - Server: chore: pin staticcheck in CI (#72)
            """),
            GitHubRelease(tagName: "mac-v0.1.250", body: """
            ## New

            - Server, Phone: Snooze a follow-up from its notification (#73)

            ## Fixed

            - Mac: Keep the inspector from looping the window's layout (#66)

            ## Other changes

            - Mac: refactor: split the version settings (#74)
            """),
            GitHubRelease(tagName: "mac-v0.1.249", body: "## Other changes\n\n- Server: chore: build the server on Go 1.27 (#69)\n"),
        ])),
        changesDatabase: true
    )

    static func makePage(_ name: String) -> VersionPage {
        var facts = NewVersionFacts(running: running, lastCheckedAt: now.addingTimeInterval(-5 * 60), newestRelease: ready.version, ready: ready)
        switch name {
        case "checking":
            facts.isChecking = true
        case "up-to-date":
            facts = NewVersionFacts(running: ready.version, lastCheckedAt: now.addingTimeInterval(-5 * 60), newestRelease: ready.version)
        case "local-build":
            facts.running = HubVersion("0.1.0-dev.abc1234")!
        case "failed-checks":
            facts.ready = nil
            facts.failed = FailedVersion(version: ready.version, problem: .otherTeam(found: "OTHERTEAM9", expected: "OWNERTEAM1"))
        case "cannot-check":
            facts = NewVersionFacts(running: running, lastCheckedAt: now.addingTimeInterval(-30 * 3600), failingSince: now.addingTimeInterval(-26 * 3600))
        case "withdrawn":
            facts = NewVersionFacts(running: ready.version, lastCheckedAt: now, newestRelease: running, isRunningWithdrawn: true)
        default:
            break
        }
        return VersionPage(
            facts: facts, status: NewVersionStatus.make(facts, now: now), installedAt: now.addingTimeInterval(-20 * 3600),
            serverVersion: facts.running.description, previousVersion: HubVersion("0.1.244"), hasInstallLog: true,
            runningReleaseURL: nil, now: now
        )
    }

    /// The tab's content as the Settings window draws it, 620 points wide.
    static func render(_ name: String) -> NSImage? {
        let content = VersionSettingsContent(page: makePage(name), isShowingAllChanges: name == "ready-all-changes", checkNow: {}, showLog: {})
            .padding(Space.xl)
            .frame(width: 620)
            .background(Color(nsColor: .windowBackgroundColor))
            .tint(.hubAccent)
            .environment(\.colorScheme, .light)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        return renderer.nsImage
    }

    /// Writes the snapshot as `version-<name>.png` into the folder.
    static func write(_ image: NSImage, named name: String, to folder: URL) throws {
        guard let tiff = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff),
              let png = bitmap.representation(using: .png, properties: [:])
        else { return }
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try png.write(to: folder.appending(path: "version-\(name).png"))
    }
}
