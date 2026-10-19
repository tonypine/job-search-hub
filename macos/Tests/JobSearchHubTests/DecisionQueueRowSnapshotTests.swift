import AppKit
@testable import JobSearchHub
import JobSearchHubCore
import SwiftUI
import Testing

// Decide's queue row drawn offscreen with ImageRenderer, from made-up jobs,
// as the queue draws it and as the focused queue draws its selected row: on
// the accent, with increased prominence and white text. A match word or a
// dot left in its tone there can vanish into the accent (TP-672's QA). With
// HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the PNGs are written there.

private let matches: [JobMatch] = [.strong, .possible, .stretch, .mismatch]

@MainActor
enum DecisionQueueRowSnapshots {
    static let width: CGFloat = 280
    static let scale: CGFloat = 2

    static func makeItem(_ match: JobMatch, title: String = "Design Engineer", reason: String = "Fits the brief.") throws -> DecisionQueueItem {
        let json = #"""
        {"job":{"id":"AAAAAAAA-0000-4000-8000-000000000001","source":"manual","title":"\#(title)","url":"https://acme.com/1",
                "first_seen_at":"2026-10-01T00:00:00Z","last_seen_at":"2026-10-01T00:00:00Z"},
         "match":"\#(match.rawValue)","reason":"\#(reason)","brief_tier":"pre",
         "fit":{"level":"good","checks":[]},"unseen_updates":2}
        """#
        return try HubJSON.makeDecoder().decode(DecisionQueueItem.self, from: Data(json.utf8))
    }

    static func render(_ item: DecisionQueueItem, isSelected: Bool) -> CGImage? {
        let row = DecisionQueueRow(item: item)
            .padding(.horizontal, Space.s)
            .frame(width: width, alignment: .leading)
        let content = Group {
            if isSelected {
                row.foregroundStyle(.white)
                    .environment(\.backgroundProminence, .increased)
                    .background(Color.hubAccent)
            } else {
                row.background(Color(nsColor: .windowBackgroundColor))
            }
        }
        .tint(.hubAccent)
        .environment(\.colorScheme, .light)
        let renderer = ImageRenderer(content: content)
        renderer.scale = scale
        return renderer.cgImage
    }

    static func write(_ image: CGImage, named name: String) throws {
        guard let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
              let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])
        else { return }
        let url = URL(filePath: folder)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try png.write(to: url.appending(path: "decide-row-\(name).png"))
    }
}

/// A drawing read in sRGB against the color at its corner, its background:
/// how far each pixel goes from the background toward white, and how far
/// it strays from that line, as a tone's color does. Pixels the drawing
/// only partly covers, at its edges, count as background.
struct InkOnBackground {
    let width: Int
    let height: Int
    private let pixels: [SIMD3<Double>]
    let background: SIMD3<Double>

    init(_ image: CGImage) {
        width = image.width
        height = image.height
        var bytes = [UInt8](repeating: 0, count: width * height * 4)
        bytes.withUnsafeMutableBytes { buffer in
            let context = CGContext(
                data: buffer.baseAddress, width: image.width, height: image.height, bitsPerComponent: 8, bytesPerRow: image.width * 4,
                space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
            )
            context?.draw(image, in: CGRect(x: 0, y: 0, width: image.width, height: image.height))
        }
        let colors = stride(from: 0, to: bytes.count, by: 4).map { index -> SIMD3<Double>? in
            bytes[index + 3] == 255 ? SIMD3(Double(bytes[index]), Double(bytes[index + 1]), Double(bytes[index + 2])) / 255 : nil
        }
        let background = colors.lazy.compactMap { $0 }.first ?? SIMD3(repeating: 0)
        self.background = background
        pixels = colors.map { $0 ?? background }
    }

    /// The most any pixel in the columns goes toward white: 0 is the
    /// background, 1 white.
    func getLightest(in columns: Range<Int>) -> Double {
        columns.flatMap { x in (0..<height).map { y in measure(pixels[y * width + x]).toward } }.max() ?? 0
    }

    /// The most any pixel strays from the line between the background and
    /// white.
    var widestStray: Double {
        pixels.map { measure($0).stray }.max() ?? 0
    }

    /// The pixel nearest to the color, as a distance in sRGB.
    func getNearest(to color: SIMD3<Double>) -> Double {
        pixels.map { length($0 - color) }.min() ?? .infinity
    }

    private func measure(_ pixel: SIMD3<Double>) -> (toward: Double, stray: Double) {
        let line = SIMD3<Double>(1, 1, 1) - background
        let toward = min(max(((pixel - background) * line).sum() / (line * line).sum(), 0), 1)
        return (toward, length(pixel - (background + toward * line)))
    }

    private func length(_ vector: SIMD3<Double>) -> Double {
        (vector * vector).sum().squareRoot()
    }
}

/// The tone's color as the light appearance draws it, in sRGB.
@MainActor
private func resolveLight(_ tone: Tone) -> SIMD3<Double> {
    var resolved = SIMD3<Double>(repeating: 0)
    NSAppearance(named: .aqua)!.performAsCurrentDrawingAppearance {
        if let color = NSColor(tone.color).usingColorSpace(.sRGB) {
            resolved = SIMD3(Double(color.redComponent), Double(color.greenComponent), Double(color.blueComponent))
        }
    }
    return resolved
}

@MainActor
@Test(arguments: matches)
func aSelectedQueueRowReadsOnTheSelection(match: JobMatch) throws {
    let image = try #require(DecisionQueueRowSnapshots.render(DecisionQueueRowSnapshots.makeItem(match), isSelected: true))
    try DecisionQueueRowSnapshots.write(image, named: "selected-\(match.rawValue)")
    // Nothing keeps a tone's color: every pixel is the selection, white, or
    // a blend of the two.
    #expect(InkOnBackground(image).widestStray < 0.12)

    // Without a title and a reason, the match word is all there is on the
    // left and the unseen dot all there is on the right: both show white.
    let bare = try #require(DecisionQueueRowSnapshots.render(DecisionQueueRowSnapshots.makeItem(match, title: "", reason: ""), isSelected: true))
    let ink = InkOnBackground(bare)
    let dotWidth = Int(24 * DecisionQueueRowSnapshots.scale)
    #expect(ink.getLightest(in: 0..<(ink.width / 2)) > 0.85, "the match word")
    #expect(ink.getLightest(in: (ink.width - dotWidth)..<ink.width) > 0.85, "the unseen dot")
}

@MainActor
@Test(arguments: matches)
func aQueueRowShowsItsMatchInItsTone(match: JobMatch) throws {
    let image = try #require(DecisionQueueRowSnapshots.render(DecisionQueueRowSnapshots.makeItem(match), isSelected: false))
    try DecisionQueueRowSnapshots.write(image, named: match.rawValue)
    #expect(image.width == Int(DecisionQueueRowSnapshots.width * DecisionQueueRowSnapshots.scale))
    #expect(InkOnBackground(image).getNearest(to: resolveLight(match.tone)) < 0.1)
}
