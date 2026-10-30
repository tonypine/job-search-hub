import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// A Runs tab's list drawn offscreen with ImageRenderer, from made-up runs,
// then drawn again as a run started here ends and the hub lists it. Drawn
// as a list of starts and a list of runs keyed by the same ids, the lazy
// stack kept the start's "Running" row, its timer going, after the hub had
// listed the run as succeeded (TP-1269's QA). With HUB_SNAPSHOT_DIR set, as
// CI's macos job sets it, the PNGs are written there.

@MainActor
private enum SubjectRunsListSnapshots {
    static let width: CGFloat = 360
    static let scale: CGFloat = 2
    static let olderID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000101")!
    static let startedID = UUID(uuidString: "EEEEEEEE-0000-4000-8000-000000000099")!

    /// A brief for a made-up job, started at 14:`minute`, that succeeded.
    static func makeRun(_ id: UUID, minute: Int, byOwner: Bool = false) throws -> ActivityRun {
        let json = #"""
        {"id":"\#(id.uuidString)","record":"model_call","kind":"job_brief",
         "subject":{"type":"job","id":"EEEEEEEE-0000-4000-8000-000000000001","title":"Staff engineer"},"model":"qwen3-8b.gguf",
         "provider":"This Mac","started_at":"2026-10-29T14:\#(String(format: "%02d", minute)):00Z","duration_ms":4200,
         "outcome":"succeeded","prompt_tokens":0,"completion_tokens":0,"started_by_owner":\#(byOwner)}
        """#
        return try HubJSON.makeDecoder().decode(ActivityRun.self, from: Data(json.utf8))
    }

    static func draw(_ rows: [SubjectRunItem]) -> some View {
        SubjectRunsList(rows: rows)
            .frame(width: width)
            .background(Color(nsColor: .windowBackgroundColor))
            .environment(\.colorScheme, .light)
    }

    static func makePNG(_ image: CGImage) -> Data? {
        NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])
    }

    static func write(_ image: CGImage, named name: String) throws {
        guard let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
              let png = makePNG(image)
        else { return }
        let url = URL(filePath: folder)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try png.write(to: url.appending(path: "runs-tab-\(name).png"))
    }
}

@MainActor
@Test func aRunStartedHereRedrawsAsItsRunOnceTheHubListsIt() throws {
    typealias Snapshots = SubjectRunsListSnapshots
    let older = try Snapshots.makeRun(Snapshots.olderID, minute: 50)
    let landed = try Snapshots.makeRun(Snapshots.startedID, minute: 59, byOwner: true)
    let start = UnlistedStart(id: Snapshots.startedID, kind: "job_brief", startedAt: landed.startedAt)
    let renderer = ImageRenderer(content: Snapshots.draw([SubjectRunItem(start: start), SubjectRunItem(run: older)]))
    renderer.scale = Snapshots.scale
    let going = try #require(renderer.cgImage)

    // In one update the start leaves and the hub lists its run, as when the
    // tab reads the runs before the Run popover hears that the run landed.
    let ended = [SubjectRunItem(run: landed), SubjectRunItem(run: older)]
    renderer.content = Snapshots.draw(ended)
    let redrawn = try #require(renderer.cgImage)
    let fresh = ImageRenderer(content: Snapshots.draw(ended))
    fresh.scale = Snapshots.scale
    let expected = try #require(fresh.cgImage)
    try Snapshots.write(going, named: "started")
    try Snapshots.write(redrawn, named: "ended")

    #expect(Snapshots.makePNG(redrawn) != Snapshots.makePNG(going))
    #expect(Snapshots.makePNG(redrawn) == Snapshots.makePNG(expected), "The redrawn list reads as a fresh one: the brief succeeded, not Running")
}
