import AppKit
@testable import JobSearchHub
import SwiftUI
import Testing

// An overdue card's Write to link drawn offscreen with ImageRenderer. The
// epic review of TP-659 saw it cut to "Write to Alex…" in a 186 pt card at
// 1400×900, so the card didn't say who to write to (TP-1101). With
// HUB_SNAPSHOT_DIR set, as CI's macos job sets it, the PNGs are written there.

@MainActor
private enum SecondRouteRowSnapshots {
    static let scale: CGFloat = 2
    /// A Pipeline card at 1400×900, as the review measured it.
    static let cardWidth: CGFloat = 186
    /// A card in the narrowest column, 180 pt, inside its padding.
    static let narrowestCardWidth: CGFloat = 180 - Space.s * 2

    /// The room the link has on a card that wide, inside the card's padding.
    static func getRoom(cardWidth: CGFloat) -> CGFloat {
        SecondRouteRow.getWriteLabelRoom(rowWidth: cardWidth - Space.m * 2)
    }

    static func render(_ content: some View, named name: String) throws -> CGImage {
        let renderer = ImageRenderer(content: content.font(.hubCaption).environment(\.colorScheme, .light))
        renderer.scale = scale
        let image = try #require(renderer.cgImage)
        if let folder = ProcessInfo.processInfo.environment["HUB_SNAPSHOT_DIR"], !folder.isEmpty,
           let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:]) {
            let url = URL(filePath: folder)
            try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
            try png.write(to: url.appending(path: "second-route-\(name).png"))
        }
        return image
    }

    /// How far right the drawing reaches, in points: the last column with a
    /// pixel drawn in it, on the renderer's clear background.
    static func getInkWidth(_ image: CGImage) -> CGFloat {
        var bytes = [UInt8](repeating: 0, count: image.width * image.height * 4)
        bytes.withUnsafeMutableBytes { buffer in
            let context = CGContext(
                data: buffer.baseAddress, width: image.width, height: image.height, bitsPerComponent: 8, bytesPerRow: image.width * 4,
                space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
            )
            context?.draw(image, in: CGRect(x: 0, y: 0, width: image.width, height: image.height))
        }
        var last = -1
        for y in 0..<image.height {
            for x in stride(from: image.width - 1, to: last, by: -1) where bytes[(y * image.width + x) * 4 + 3] > 0 {
                last = x
                break
            }
        }
        return CGFloat(last + 1) / scale
    }
}

@MainActor
@Test
func theWriteToLinkFitsACardAt1400By900() throws {
    let label = SecondRouteRow.makeWriteLabel(to: "Alex Kim").fixedSize()
    let image = try SecondRouteRowSnapshots.render(label, named: "alex-kim")
    let room = SecondRouteRowSnapshots.getRoom(cardWidth: SecondRouteRowSnapshots.cardWidth)
    // The whole of "Write to Alex Kim", at the size it asks for, fits beside
    // the chevron and the ✓, so nothing cuts it.
    #expect(CGFloat(image.width) / SecondRouteRowSnapshots.scale <= room)
}

@MainActor
@Test
func aLongerNameKeepsWriteToAndDropsThePaperplane() throws {
    let name = "Dana Whitfield"
    let room = SecondRouteRowSnapshots.getRoom(cardWidth: SecondRouteRowSnapshots.cardWidth)
    let full = try SecondRouteRowSnapshots.render(Label("Write to \(name)", systemImage: "paperplane").fixedSize(), named: "long-full")
    let words = try SecondRouteRowSnapshots.render(Text("Write to \(name)").fixedSize(), named: "long-words")
    #expect(SecondRouteRowSnapshots.getInkWidth(full) > room, "the paperplane, Write to and the name don't all fit")
    #expect(SecondRouteRowSnapshots.getInkWidth(words) < room, "Write to and the name fit")

    // At 1400×900 the link drops the paperplane rather than "Write to": it
    // draws just as far as the words alone do.
    let fitted = SecondRouteRow.makeWriteLabel(to: name).frame(width: room, alignment: .leading)
    let image = try SecondRouteRowSnapshots.render(fitted, named: "long")
    #expect(abs(SecondRouteRowSnapshots.getInkWidth(image) - SecondRouteRowSnapshots.getInkWidth(words)) <= 1)
}

@MainActor
@Test
func theWriteToLinkKeepsTheNameInTheNarrowestColumn() throws {
    let name = "Morgan Ellery"
    let room = SecondRouteRowSnapshots.getRoom(cardWidth: SecondRouteRowSnapshots.narrowestCardWidth)
    let words = try SecondRouteRowSnapshots.render(Text("Write to \(name)").fixedSize(), named: "words")
    let bare = try SecondRouteRowSnapshots.render(Label(name, systemImage: "paperplane").fixedSize(), named: "bare")
    #expect(SecondRouteRowSnapshots.getInkWidth(words) > room, "Write to and the name don't both fit")
    #expect(SecondRouteRowSnapshots.getInkWidth(bare) < room, "the name alone fits")

    // In that room the link drops "Write to" rather than cutting the name:
    // it draws just as far as the name alone does.
    let fitted = SecondRouteRow.makeWriteLabel(to: name).frame(width: room, alignment: .leading)
    let image = try SecondRouteRowSnapshots.render(fitted, named: "narrowest")
    #expect(abs(SecondRouteRowSnapshots.getInkWidth(image) - SecondRouteRowSnapshots.getInkWidth(bare)) <= 1)
}
