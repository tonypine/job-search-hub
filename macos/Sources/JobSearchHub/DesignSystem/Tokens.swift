import AppKit
import JobSearchHubCore
import SwiftUI

// The design system's tokens: one accent, the four tones, the type roles, a
// 4-point spacing scale and three radii. docs/design/ui-redesign.md explains
// each; nothing outside DesignSystem/ names a color, a size or a radius of
// its own.

extension Color {
    /// Hub Indigo: you and your actions. Selection, links, the primary
    /// button, the unseen dot.
    static let hubAccent = Color(light: 0x4B49D6, dark: 0x8E8CFF)

    /// A color that follows the window's appearance.
    init(light: UInt32, dark: UInt32) {
        self.init(nsColor: NSColor(name: nil) { appearance in
            let isDark = appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            return NSColor(hex: isDark ? dark : light)
        })
    }
}

private extension NSColor {
    convenience init(hex: UInt32) {
        self.init(
            srgbRed: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255, alpha: 1
        )
    }
}

extension Tone {
    /// The tone solid, for text and symbols.
    var color: Color {
        switch self {
        case .accent: .hubAccent
        case .positive: Self.positiveColor
        case .caution: Self.cautionColor
        case .negative: Self.negativeColor
        case .neutral: Self.neutralColor
        }
    }

    /// The tone behind a chip or a banner, under solid text.
    var fill: Color { color.opacity(0.13) }

    /// The tone behind words marked in text to read, such as a posting's
    /// phrases a screen check quoted.
    var mark: Color { color.opacity(0.24) }

    private static let positiveColor = Color(light: 0x1E8E50, dark: 0x3CC97F)
    private static let cautionColor = Color(light: 0xB86E00, dark: 0xF2A33A)
    private static let negativeColor = Color(light: 0xD13438, dark: 0xFF6B6B)
    private static let neutralColor = Color(light: 0x6E6E73, dark: 0x98989D)
}

/// The spacing scale. Rows within a section are `s` apart, sections `l`, and
/// pages pad by `xl`.
enum Space {
    static let xs: CGFloat = 4
    static let s: CGFloat = 8
    static let m: CGFloat = 12
    static let l: CGFloat = 16
    static let xl: CGFloat = 24
    static let xxl: CGFloat = 32
}

/// The corner radii. Chips and buttons are capsules.
enum Radius {
    /// Fields and small buttons.
    static let control: CGFloat = 6
    /// Cards and wells.
    static let card: CGFloat = 10
    /// Sheets and floating panels.
    static let panel: CGFloat = 14
}

/// The type roles. The page's own name is its navigation title.
extension Font {
    /// A job's, company's or person's name, where it's the subject.
    static let hubEntity = Font.title2.weight(.semibold)
    /// A section's title: Brief, Screen, People.
    static let hubSection = Font.headline
    /// Text to read.
    static let hubBody = Font.body
    /// Text read at length, such as a brief's verdict: a size up from body.
    static let hubReading = Font.system(size: 14)
    /// A posting read in the inspector (P3 Reader): 13 pt.
    static let hubPosting = Font.system(size: 13)
    /// Company, location, reasons; shown in `.secondary`.
    static let hubSecondary = Font.callout
    /// Who wrote it and when.
    static let hubCaption = Font.caption
    /// Words quoted from a posting.
    static let hubEvidence = Font.caption.italic()
}

extension View {
    /// A group on a page: the content on the window's background, with a
    /// separator hairline around it.
    func hubCard() -> some View {
        padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .topLeading)
            .background(.background, in: RoundedRectangle(cornerRadius: Radius.card))
            .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(.separator))
    }

    /// Text read at length, at 1.3 line height.
    func hubReading() -> some View {
        font(.hubReading).lineSpacing(2)
    }

    /// Quoted or generated text, such as a drafted reply.
    func hubWell() -> some View {
        padding(Space.m)
            .frame(maxWidth: .infinity, alignment: .topLeading)
            .background(.quinary, in: RoundedRectangle(cornerRadius: Radius.card))
    }
}
