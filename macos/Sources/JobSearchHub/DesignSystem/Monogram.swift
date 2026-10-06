import JobSearchHubCore
import SwiftUI

/// A company's first letter in a small tile beside its name, so a card is
/// found by its company at a glance. It repeats the name, so it's hidden
/// from VoiceOver.
struct Monogram: View {
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
