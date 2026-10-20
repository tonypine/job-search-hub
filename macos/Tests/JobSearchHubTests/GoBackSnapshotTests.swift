import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// Going back to an earlier version, the crash prompt, the older versions
// and the withdrawn banner, drawn offscreen with ImageRenderer from made-up
// counts. With HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the PNGs are
// written there.

@MainActor
enum GoBackSnapshots {
    static let now = Date(timeIntervalSince1970: 1_791_302_400)
    static let running = HubVersion("0.1.252")!
    static let previous = HubVersion("0.1.247")!
    static let lost = [
        LostCount(rows: 3, words: "jobs found"), LostCount(rows: 1, words: "card moved"), LostCount(rows: 2, words: "people added"),
        LostCount(rows: 4, words: "updates"),
    ]

    nonisolated static let names = ["nothing-lost", "loses", "uncountable", "no-copy", "crash", "weighing"]

    static func makeOffer(_ name: String) -> GoingBack.Offer? {
        let cost: GoingBackCost
        switch name {
        case "weighing": return nil
        case "nothing-lost": cost = .nothingLost
        case "uncountable": cost = .uncountable(since: now.addingTimeInterval(-2 * 3600))
        case "no-copy": cost = .noCopy
        case "crash": cost = .loses(since: now.addingTimeInterval(-7 * 3600), lost: Array(lost.prefix(2)))
        default: cost = .loses(since: now.addingTimeInterval(-2 * 3600), lost: lost)
        }
        return GoingBack.Offer(
            reason: name == "crash" ? .crash : .settings, running: running, target: previous,
            app: URL(filePath: "/Updates/previous/Job Search Hub.app"), databaseMigration: 91, restoredMigration: 88, cost: cost,
            installedAt: now.addingTimeInterval(-7 * 3600)
        )
    }

    static func render(_ name: String) -> NSImage? {
        draw(GoBackSheetContent(
            offer: makeOffer(name), isWeighing: name == "weighing", now: now, failure: .constant(nil), goBack: {}, cancel: {}
        ))
    }

    static func renderOlderVersions() -> NSImage? {
        let releases = MacReleases.parse(["0.1.250", "0.1.245", "0.1.240"].map { GitHubRelease(tagName: "mac-v\($0)") })
        let versions = [
            OlderVersion(release: releases[0], migration: 91, cost: .nothingLost),
            OlderVersion(release: releases[1], migration: 88, cost: .loses(since: now.addingTimeInterval(-86400), lost: lost)),
            OlderVersion(release: releases[2], migration: 85, cost: .uncountable(since: now.addingTimeInterval(-5 * 86400))),
        ]
        return draw(OlderVersionsContent(
            versions: versions, isLoading: false, downloading: releases[1].version, failure: .constant(nil), choose: { _ in }, retry: {}, cancel: {}
        ))
    }

    static func renderWithdrawnBanner() -> NSImage? {
        draw(WithdrawnBanner(running: running, summary: GoingBackCost.nothingLost.summarize(target: previous), goBack: {}, dismiss: {})
            .frame(width: 720))
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
        try png.write(to: url.appending(path: "go-back-\(name).png"))
    }
}

@MainActor
@Test(arguments: GoBackSnapshots.names)
func goBackSheetDrawsOffscreen(state: String) throws {
    let image = try #require(GoBackSnapshots.render(state))
    #expect(image.size.width == 500)
    #expect(image.size.height > 60)
    try GoBackSnapshots.write(image, named: "sheet-\(state)")
}

@MainActor
@Test func olderVersionsAndTheWithdrawnBannerDrawOffscreen() throws {
    let older = try #require(GoBackSnapshots.renderOlderVersions())
    #expect(older.size.width == 500)
    try GoBackSnapshots.write(older, named: "older-versions")

    let banner = try #require(GoBackSnapshots.renderWithdrawnBanner())
    #expect(banner.size.width == 720)
    try GoBackSnapshots.write(banner, named: "withdrawn-banner")
}

@MainActor
@Test func theCrashPromptSaysWhenTheVersionWasInstalledAndWhatGoingBackLoses() throws {
    let offer = try #require(GoBackSnapshots.makeOffer("crash"))
    #expect(offer.cost.losesWrites)
    #expect(offer.cost.lostSentence == "Since then: 3 jobs found, 1 card moved. These would be lost.")
    #expect(offer.cost.describeDatabase(running: offer.running, target: offer.target, now: GoBackSnapshots.now).hasSuffix(", before 0.1.252 changed it."))
}
