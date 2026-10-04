import JobSearchHubCore
import SwiftUI

/// A status word in a tone, with an optional symbol: a match, a screen, a
/// follow-up, a relation. The word always shows, so color never stands alone.
struct ToneChip: View {
    let text: String
    let tone: Tone
    var symbol: String?

    init(_ text: String, tone: Tone, symbol: String? = nil) {
        self.text = text
        self.tone = tone
        self.symbol = symbol
    }

    var body: some View {
        HStack(spacing: 3) {
            if let symbol {
                Image(systemName: symbol).imageScale(.small)
            }
            Text(text)
        }
        .font(.caption.weight(.semibold))
        .lineLimit(1)
        .padding(.horizontal, 7)
        .padding(.vertical, 2)
        .foregroundStyle(tone.color)
        .background(tone.fill, in: Capsule())
        .fixedSize()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(text)
    }
}

extension ToneChip {
    /// A brief's match: Strong, Possible, Stretch or Mismatch.
    init(_ match: JobMatch) {
        self.init(match.title, tone: match.tone)
    }

    /// A job's screen, where the column or section names it: Passes, Unclear
    /// or Fails.
    init(_ screen: FitLevel) {
        self.init(screen.title, tone: screen.tone)
    }

    /// A job's screen among other chips: "Passes screen".
    init(screen: FitLevel) {
        self.init(screen.label, tone: screen.tone)
    }

    /// When a card's follow-up falls due.
    init(_ followUp: FollowUpStatus) {
        self.init(followUp.text, tone: followUp.tone)
    }
}
