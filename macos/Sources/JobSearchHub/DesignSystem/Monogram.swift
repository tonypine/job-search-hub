import JobSearchHubCore
import SwiftUI

/// A company's first letter on a tile of its hue, before its name in a row.
/// The hue comes from the name, so a company looks the same everywhere.
struct Monogram: View {
    let name: String
    var size: CGFloat = 26

    var body: some View {
        let hue = Self.hues[Monograms.getHueIndex(for: name, count: Self.hues.count)]
        Text(Monograms.getLetter(for: name))
            .font(.system(size: size * 0.42, weight: .bold))
            .foregroundStyle(hue)
            .frame(width: size, height: size)
            .background(hue.opacity(0.12), in: RoundedRectangle(cornerRadius: size * 0.27, style: .continuous))
            .accessibilityHidden(true)
    }

    /// Hues that tell companies apart and say nothing about them, unlike
    /// the tones.
    private static let hues = [
        Color(light: 0x1B7F8C, dark: 0x4CC3D0), Color(light: 0x7A4BD6, dark: 0xAE8CFF), Color(light: 0xC2571B, dark: 0xF08A4B),
        Color(light: 0x2F6FD0, dark: 0x6EA4FF), Color(light: 0xB3365F, dark: 0xF07399), Color(light: 0x4F7F1F, dark: 0x8FC75A),
    ]
}

/// A company's first letter in a small tile beside its name, so a card is
/// found by its company at a glance. It repeats the name, so it's hidden
/// from VoiceOver.
struct CardMonogram: View {
    let name: String

    var body: some View {
        Text(Self.getLetter(of: name))
            .font(.caption2.weight(.bold))
            .foregroundStyle(Tone.accent.color)
            .frame(width: 16, height: 16)
            .background(Tone.accent.fill, in: RoundedRectangle(cornerRadius: Radius.control))
            .accessibilityHidden(true)
    }

    /// The name's first letter or digit, capitalized; a dot for a name with neither.
    static func getLetter(of name: String) -> String {
        name.first { $0.isLetter || $0.isNumber }.map { String($0).uppercased() } ?? "·"
    }
}

/// A person as their initials on a tinted square, beside their name. It
/// repeats the name, so it's hidden from VoiceOver.
struct InitialsMonogram: View {
    let initials: String
    var size: CGFloat = 32
    var tone: Tone = .neutral

    init(_ name: String, size: CGFloat = 32, tone: Tone = .neutral) {
        initials = Initials.make(from: name)
        self.size = size
        self.tone = tone
    }

    var body: some View {
        Text(initials)
            .font(.system(size: max(size * 0.4, 10), weight: .semibold))
            .foregroundStyle(tone.color)
            .frame(width: size, height: size)
            .background(tone.fill, in: RoundedRectangle(cornerRadius: size / 4))
            .accessibilityHidden(true)
    }
}
