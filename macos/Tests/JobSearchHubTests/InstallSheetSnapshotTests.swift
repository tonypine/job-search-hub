import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// The install sheet and the banner after an install, drawn offscreen with
// ImageRenderer, from made-up sessions and companies. With HUB_SNAPSHOT_DIR
// set, as CI's macos job sets it, the PNGs are written there.

@MainActor
enum InstallSnapshots {
    static let now = Date(timeIntervalSince1970: 1_791_000_000)
    static let version = HubVersion("0.1.252")!
    static let acme = RunningSession(id: UUID(), name: "Acme", activity: .working)
    static let staff = RunningSession(id: UUID(), name: "Staff Engineer · Initech", activity: .idle)
    static let research = DrainStatus.Work(type: "agent_run", kind: "company_triage", subject: "Initech", startedAt: now, ageSeconds: 130)
    static let profile = UnsavedEdit(id: "criteria", title: "Criteria")

    nonisolated static let names = ["review", "waiting", "nothing-running", "when-i-quit"]

    static func makeWork(_ name: String) -> RunningWork {
        let start = RunningWork.make(sessions: [acme, staff], drain: [research], tasks: [], edits: [profile], now: now)
        switch name {
        case "waiting":
            // The research finished and the criteria were saved; Acme works on.
            return start.update(with: RunningWork.make(sessions: [acme, staff], drain: [], tasks: [], edits: [], now: now))
        case "nothing-running":
            return RunningWork.make(sessions: [staff], drain: [], tasks: [], edits: [], now: now)
        default:
            return start
        }
    }

    static func render(_ name: String) -> NSImage? {
        let content = InstallSheetContent(
            version: version, isWaiting: name == "waiting", work: makeWork(name), changesDatabase: name != "nothing-running",
            reopensApp: name != "when-i-quit", failure: .constant(nil), cancel: {}, installAnyway: {}, installWhenTheseFinish: {}, openEdit: { _ in }
        )
        return draw(content)
    }

    static func renderBanner(_ outcome: InstallOutcome) -> NSImage? {
        let banner = InstallNoticeBanner(
            notice: Installer.Notice(outcome: outcome, from: "0.1.247", to: "0.1.252", endedAt: now), showWhatsNew: {}, showLog: {}, dismiss: {}
        )
        .frame(width: 720)
        return draw(banner)
    }

    private static func draw(_ view: some View) -> NSImage? {
        let renderer = ImageRenderer(content: view
            .background(Color(nsColor: .windowBackgroundColor))
            .tint(.hubAccent)
            .environment(\.colorScheme, .light))
        renderer.scale = 2
        return renderer.nsImage
    }

    static func write(_ image: NSImage, named name: String) throws {
        guard let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
              let tiff = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff),
              let png = bitmap.representation(using: .png, properties: [:])
        else { return }
        let url = URL(filePath: folder)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try png.write(to: url.appending(path: "install-\(name).png"))
    }
}

@MainActor
@Test(arguments: InstallSnapshots.names)
func installSheetDrawsOffscreen(state: String) throws {
    let image = try #require(InstallSnapshots.render(state))
    #expect(image.size.width == 500)
    #expect(image.size.height > 120)
    try InstallSnapshots.write(image, named: "sheet-\(state)")
}

@MainActor
@Test func theSheetWaitsForWhatRunsAndTicksItOff() {
    let review = InstallSnapshots.makeWork("review")
    #expect(review.items.map(\.title) == ["Claude session · Acme", "Researching Initech", "Criteria has unsaved edits"])
    #expect(review.hasUnsavedEdits)
    #expect(review.idleSessionsLine == "1 idle Claude session reopens where it was")

    let waiting = InstallSnapshots.makeWork("waiting")
    #expect(waiting.items.map(\.hasEnded) == [false, true, true])
    #expect(!waiting.hasUnsavedEdits)
    #expect(waiting.waitingSummary == "Waiting for 1 session")
    #expect(InstallSnapshots.makeWork("nothing-running").isClear)
}

@MainActor
@Test func theBannerSaysHowTheInstallEnded() throws {
    let outcomes: [(String, InstallOutcome)] = [
        ("installed", .installed(InstallSnapshots.version)),
        ("rolled-back", .rolledBack(message: "0.1.252 couldn't start, so the hub went back to 0.1.247.", lost: "Nothing was lost.")),
        ("rollback-failed", .rollbackFailed(
            message: "0.1.252 couldn't start, and the hub couldn't go back to 0.1.247 by itself.",
            error: "The rollback stopped at restoring_database: pg_restore failed", commands: ["launchctl kickstart gui/501/com.tonypine.jobsearchhub.server"]
        )),
    ]
    for (name, outcome) in outcomes {
        let image = try #require(InstallSnapshots.renderBanner(outcome))
        #expect(image.size.width == 720)
        try InstallSnapshots.write(image, named: "banner-\(name)")
    }
}
