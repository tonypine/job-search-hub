// Draws the app's mark, a hub (you) joined to three nodes (a company, a job, a
// person) on a Hub Indigo squircle, for both clients. This file is its source:
// change the mark here and regenerate.
//
//   make-icon iconset <dir.iconset>    the macOS PNGs, for `iconutil -c icns`
//   make-icon android <res dir>        the Android adaptive icon's layers
//
// Scripts/make-app.sh runs the first on every build. Run the second by hand
// and commit what it writes:
//   mkdir -p build && swiftc -O Scripts/make-icon.swift -o build/make-icon
//   build/make-icon android ../android/app/src/main/res
import CoreGraphics
import Foundation
import ImageIO

// MARK: - The mark

// Coordinates are in tiles: the squircle is 1 wide, (0, 0) is its centre and y
// points down.
struct Point { var x, y: Double }

enum Shape {
    case circle(Point, radius: Double)
    case ring(Point, outer: Double, inner: Double)
    case line(Point, Point, width: Double)
}

struct Element {
    var shape: Shape
    var opacity: Double
}

struct Mark {
    static let cornerRadius = 0.225
    // A diagonal from the top-left corner to the bottom-right one.
    static let gradient = ["#6360F0", "#4B49D6", "#3C39B8"]

    static let hub = Point(x: 0, y: 0.029)
    static let hubOuter = 0.128
    static let hubInner = 0.055
    static let nodeDistance = 0.27
    static let nodeRadius = 0.074
    static let spokeWidth = 0.045
    // Company, job and person: up, then the lower right and lower left.
    static let nodeAngles = [-90.0, 30.0, 150.0]

    // Spokes first, so the hub and the nodes cover their ends. The hub is a
    // ring, so a spoke starts inside the ring rather than at its hole.
    static func elements(spokeOpacity: Double) -> [Element] {
        let nodes = nodeAngles.map { degrees -> Point in
            let angle = degrees * .pi / 180
            return Point(x: hub.x + nodeDistance * cos(angle), y: hub.y + nodeDistance * sin(angle))
        }
        let spokeStart = (hubOuter + hubInner) / 2
        let spokes = nodes.map { node -> Element in
            let length = hypot(node.x - hub.x, node.y - hub.y)
            let start = Point(
                x: hub.x + (node.x - hub.x) / length * spokeStart,
                y: hub.y + (node.y - hub.y) / length * spokeStart)
            return Element(shape: .line(start, node, width: spokeWidth), opacity: spokeOpacity)
        }
        return spokes
            + nodes.map { Element(shape: .circle($0, radius: nodeRadius), opacity: 1) }
            + [Element(shape: .ring(hub, outer: hubOuter, inner: hubInner), opacity: 1)]
    }
}

// MARK: - macOS

// Apple's grid: an 824 squircle centred on a 1024 canvas, with a drop shadow.
let canvas = 1024.0
let tileSide = 824.0

func color(_ hex: String, alpha: Double = 1) -> CGColor {
    let value = UInt32(hex.dropFirst(), radix: 16)!
    return CGColor(
        srgbRed: Double((value >> 16) & 0xFF) / 255,
        green: Double((value >> 8) & 0xFF) / 255,
        blue: Double(value & 0xFF) / 255,
        alpha: alpha)
}

// A rounded rectangle with continuous corners, the shape of macOS app icons.
// The control points are the ones UIKit uses for its continuous corners.
func squircle(_ rect: CGRect, radius r: Double) -> CGPath {
    let corners: [(corner: CGPoint, back: CGVector, ahead: CGVector)] = [
        (CGPoint(x: rect.maxX, y: rect.minY), CGVector(dx: -1, dy: 0), CGVector(dx: 0, dy: 1)),
        (CGPoint(x: rect.maxX, y: rect.maxY), CGVector(dx: 0, dy: -1), CGVector(dx: -1, dy: 0)),
        (CGPoint(x: rect.minX, y: rect.maxY), CGVector(dx: 1, dy: 0), CGVector(dx: 0, dy: -1)),
        (CGPoint(x: rect.minX, y: rect.minY), CGVector(dx: 0, dy: 1), CGVector(dx: 1, dy: 0)),
    ]
    let path = CGMutablePath()
    for (index, (corner, back, ahead)) in corners.enumerated() {
        func point(_ alongBack: Double, _ alongAhead: Double) -> CGPoint {
            CGPoint(
                x: corner.x + (back.dx * alongBack + ahead.dx * alongAhead) * r,
                y: corner.y + (back.dy * alongBack + ahead.dy * alongAhead) * r)
        }
        if index == 0 { path.move(to: point(1.52866483, 0)) } else { path.addLine(to: point(1.52866483, 0)) }
        path.addCurve(to: point(0.63149399, 0.07491100), control1: point(1.08849323, 0), control2: point(0.86840689, 0))
        path.addCurve(
            to: point(0.07491100, 0.63149399), control1: point(0.37282392, 0.16905899),
            control2: point(0.16905899, 0.37282392))
        path.addCurve(to: point(0, 1.52866483), control1: point(0, 0.86840689), control2: point(0, 1.08849323))
    }
    path.closeSubpath()
    return path
}

func renderMac(pixels: Int) -> CGImage {
    let context = CGContext(
        data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
        space: CGColorSpace(name: CGColorSpace.sRGB)!,
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    let scale = Double(pixels) / canvas
    // Work in a 1024 canvas with y pointing down, like the mark.
    context.scaleBy(x: scale, y: -scale)
    context.translateBy(x: 0, y: -canvas)

    let origin = (canvas - tileSide) / 2
    let tileRect = CGRect(x: origin, y: origin, width: tileSide, height: tileSide)
    let tile = squircle(tileRect, radius: Mark.cornerRadius * tileSide)

    // The shadow's offset isn't flipped by the transform: negative is down.
    context.saveGState()
    context.setShadow(offset: CGSize(width: 0, height: -10 * scale), blur: 20 * scale, color: color("#000000", alpha: 0.3))
    context.addPath(tile)
    context.setFillColor(color(Mark.gradient[1]))
    context.fillPath()
    context.restoreGState()

    context.saveGState()
    context.addPath(tile)
    context.clip()
    let gradient = CGGradient(
        colorsSpace: CGColorSpace(name: CGColorSpace.sRGB)!,
        colors: Mark.gradient.map { color($0) } as CFArray, locations: [0, 0.5, 1])!
    context.drawLinearGradient(
        gradient, start: CGPoint(x: tileRect.minX, y: tileRect.minY),
        end: CGPoint(x: tileRect.maxX, y: tileRect.maxY), options: [])

    func place(_ point: Point) -> CGPoint {
        CGPoint(x: canvas / 2 + point.x * tileSide, y: canvas / 2 + point.y * tileSide)
    }
    func circle(_ centre: Point, _ radius: Double) -> CGRect {
        let c = place(centre), r = radius * tileSide
        return CGRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r)
    }
    for element in Mark.elements(spokeOpacity: 0.85) {
        let white = color("#FFFFFF", alpha: element.opacity)
        switch element.shape {
        case .circle(let centre, let radius):
            context.setFillColor(white)
            context.fillEllipse(in: circle(centre, radius))
        case .ring(let centre, let outer, let inner):
            context.setFillColor(white)
            context.addEllipse(in: circle(centre, outer))
            context.addEllipse(in: circle(centre, inner))
            context.fillPath(using: .evenOdd)
        case .line(let from, let to, let width):
            context.setStrokeColor(white)
            context.setLineWidth(width * tileSide)
            context.strokeLineSegments(between: [place(from), place(to)])
        }
    }
    context.restoreGState()
    return context.makeImage()!
}

func writeIconset(to directory: URL) throws {
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    for points in [16, 32, 128, 256, 512] {
        for factor in [1, 2] {
            let name = factor == 1 ? "icon_\(points)x\(points).png" : "icon_\(points)x\(points)@2x.png"
            let url = directory.appendingPathComponent(name)
            guard let destination = CGImageDestinationCreateWithURL(url as CFURL, "public.png" as CFString, 1, nil)
            else { throw IconError("can't write \(url.path)") }
            CGImageDestinationAddImage(destination, renderMac(pixels: points * factor), nil)
            guard CGImageDestinationFinalize(destination) else { throw IconError("can't write \(url.path)") }
        }
    }
}

// MARK: - Android

// An adaptive icon's layers are 108 dp; launchers mask them to at most the
// middle 72, and keep the middle 66 clear of the mask, which the mark fits in.
let layerSide = 108.0
let maskSide = 72.0

func dp(_ value: Double) -> String {
    var text = String(format: "%.2f", value)
    while text.hasSuffix("0") { text.removeLast() }
    if text.hasSuffix(".") { text.removeLast() }
    return text
}

func androidPoint(_ point: Point) -> String {
    "\(dp(layerSide / 2 + point.x * maskSide)),\(dp(layerSide / 2 + point.y * maskSide))"
}

func androidCircle(_ centre: Point, _ radius: Double) -> String {
    // Both arcs span exactly twice the radius, so the circle closes.
    let rounded = (radius * maskSide * 100).rounded() / 100
    let r = dp(rounded), d = dp(2 * rounded)
    let start = Point(x: centre.x - rounded / maskSide, y: centre.y)
    return "M\(androidPoint(start))a\(r),\(r) 0,1 1,\(d),0a\(r),\(r) 0,1 1,-\(d),0z"
}

func vector(_ comment: String, _ body: String, aapt: Bool = false) -> String {
    let namespaces =
        "xmlns:android=\"http://schemas.android.com/apk/res/android\""
        + (aapt ? "\n    xmlns:aapt=\"http://schemas.android.com/aapt\"" : "")
    return """
        <?xml version="1.0" encoding="utf-8"?>
        <!-- \(comment) Written by macos/Scripts/make-icon.swift: change the mark there and rerun it. -->
        <vector \(namespaces)
            android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">
        \(body)
        </vector>

        """
}

func markPaths(spokeOpacity: Double) -> String {
    var spokes: [String] = []
    var paths: [String] = []
    for element in Mark.elements(spokeOpacity: spokeOpacity) {
        switch element.shape {
        case .line(let from, let to, _):
            spokes.append("M\(androidPoint(from))L\(androidPoint(to))")
        case .circle(let centre, let radius):
            paths.append("""
                    <path android:fillColor="#FFFFFFFF"
                        android:pathData="\(androidCircle(centre, radius))" />
                """)
        case .ring(let centre, let outer, let inner):
            paths.append("""
                    <path android:fillColor="#FFFFFFFF" android:fillType="evenOdd"
                        android:pathData="\(androidCircle(centre, outer))\(androidCircle(centre, inner))" />
                """)
        }
    }
    let alpha = spokeOpacity < 1 ? " android:strokeAlpha=\"\(dp(spokeOpacity))\"" : ""
    let spokePath = """
            <path android:strokeColor="#FFFFFFFF"\(alpha) android:strokeWidth="\(dp(Mark.spokeWidth * maskSide))"
                android:pathData="\(spokes.joined())" />
        """
    return ([spokePath] + paths).joined(separator: "\n")
}

func writeAndroid(to resources: URL) throws {
    let drawables = resources.appendingPathComponent("drawable")
    try FileManager.default.createDirectory(at: drawables, withIntermediateDirectories: true)
    let start = dp(layerSide / 2 - maskSide / 2), end = dp(layerSide / 2 + maskSide / 2)
    let background = """
            <path android:pathData="M0,0h108v108h-108z">
                <aapt:attr name="android:fillColor">
                    <gradient android:type="linear"
                        android:startX="\(start)" android:startY="\(start)" android:endX="\(end)" android:endY="\(end)"
                        android:startColor="#FF\(Mark.gradient[0].dropFirst())"
                        android:centerColor="#FF\(Mark.gradient[1].dropFirst())"
                        android:endColor="#FF\(Mark.gradient[2].dropFirst())" />
                </aapt:attr>
            </path>
        """
    let files = [
        "ic_launcher_background.xml": vector("The launcher icon's Hub Indigo.", background, aapt: true),
        "ic_launcher_foreground.xml": vector("The launcher icon's mark.", markPaths(spokeOpacity: 0.85)),
        // Themed icons tint this layer, so it's the mark in one opaque color.
        "ic_launcher_monochrome.xml": vector("The mark for themed icons.", markPaths(spokeOpacity: 1)),
    ]
    for (name, text) in files {
        try text.write(to: drawables.appendingPathComponent(name), atomically: false, encoding: .utf8)
    }
}

// MARK: - Main

struct IconError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

let arguments = CommandLine.arguments
guard arguments.count == 3, ["iconset", "android"].contains(arguments[1]) else {
    FileHandle.standardError.write(Data("usage: make-icon iconset <dir.iconset> | android <res dir>\n".utf8))
    exit(2)
}
let target = URL(fileURLWithPath: arguments[2])
do {
    if arguments[1] == "iconset" { try writeIconset(to: target) } else { try writeAndroid(to: target) }
} catch {
    FileHandle.standardError.write(Data("make-icon: \(error)\n".utf8))
    exit(1)
}
