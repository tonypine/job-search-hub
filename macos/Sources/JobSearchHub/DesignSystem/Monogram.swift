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
