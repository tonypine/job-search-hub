import JobSearchHubCore
import SwiftUI

/// A company's or a person's initials on a tile of its hue, before its name
/// in a Jobs row, a Pipeline card or the inspector. The letters and the hue
/// come from the name, so a name looks the same everywhere. It repeats the
/// name, so it's hidden from VoiceOver.
struct Monogram: View {
    let name: String
    let size: CGFloat

    init(_ name: String, size: CGFloat = 26) {
        self.name = name
        self.size = size
    }

    var body: some View {
        let look = Monograms.getLook(for: name)
        let hue = Self.hues[look.hueIndex % Self.hues.count]
        Text(look.letters)
            .font(.system(size: max(size * (look.letters.count > 1 ? 0.36 : 0.42), 8), weight: .bold))
            .foregroundStyle(hue)
            .frame(width: size, height: size)
            .background(hue.opacity(0.12), in: RoundedRectangle(cornerRadius: size * 0.27, style: .continuous))
            .accessibilityHidden(true)
    }

    /// Hues that tell names apart and say nothing about them, unlike the
    /// tones; one per `Monograms.hueCount`.
    private static let hues = [
        Color(light: 0x1B7F8C, dark: 0x4CC3D0), Color(light: 0x7A4BD6, dark: 0xAE8CFF), Color(light: 0xC2571B, dark: 0xF08A4B),
        Color(light: 0x2F6FD0, dark: 0x6EA4FF), Color(light: 0xB3365F, dark: 0xF07399), Color(light: 0x4F7F1F, dark: 0x8FC75A),
    ]
}
