import JobSearchHubCore
import SwiftUI

/// A company or a person as its initials on a tinted square.
struct Monogram: View {
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
            .font(.system(size: size * 0.4, weight: .semibold))
            .foregroundStyle(tone.color)
            .frame(width: size, height: size)
            .background(tone.fill, in: RoundedRectangle(cornerRadius: size / 4))
            .accessibilityHidden(true)
    }
}
