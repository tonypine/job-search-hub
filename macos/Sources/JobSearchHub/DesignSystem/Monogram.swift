import JobSearchHubCore
import SwiftUI

/// A company or a person as its initials on a tinted square. It repeats a
/// name shown beside it, so it's hidden from VoiceOver.
struct Monogram: View {
    let initials: String
    var size: CGFloat = 32
    var tone: Tone = .neutral

    init(_ name: String, size: CGFloat = 32, tone: Tone = .neutral) {
        let letters = Initials.make(from: name)
        // A small tile, such as a Pipeline card's, only fits one letter.
        initials = size < 24 ? String(letters.prefix(1)) : letters
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
