import JobSearchHubCore
import SwiftUI

/// One judgment: a symbol in its tone, the name of what was judged and why,
/// and the posting's words behind it. Fit checks, screen-out answers and a
/// brief's points all use it.
struct VerdictRow: View {
    let symbol: String
    let tone: Tone
    /// What was judged, in medium weight; nil for a row that is all reason.
    var name: String?
    let reason: String
    /// A line under the reason, such as the knowledge-base entries cited.
    var note: String?
    /// Words quoted from the posting.
    var evidence: String?

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol)
                .foregroundStyle(tone.color)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Space.xs) {
                if let name {
                    Text("\(Text(name).fontWeight(.medium))  \(Text(reason).foregroundStyle(.secondary))")
                        .fixedSize(horizontal: false, vertical: true)
                } else {
                    Text(reason).fixedSize(horizontal: false, vertical: true)
                }
                if let note, !note.isEmpty {
                    Text(note).font(.hubCaption).foregroundStyle(.secondary)
                }
                if let evidence, !evidence.isEmpty {
                    Evidence(text: evidence)
                }
            }
            .textSelection(.enabled)
        }
        .accessibilityElement(children: .combine)
    }
}

extension VerdictRow {
    /// A fit check or a screen-out answer. Without a verdict it's information
    /// the screen doesn't judge, such as the contract.
    init(_ verdict: FitVerdict?, name: String, reason: String, evidence: String? = nil) {
        self.init(
            symbol: verdict?.symbolName ?? "info.circle", tone: verdict?.tone ?? .neutral,
            name: name, reason: reason, evidence: evidence
        )
    }
}

/// Words quoted from a posting, in italics beside a rule.
struct Evidence: View {
    let text: String

    var body: some View {
        HStack(spacing: Space.s) {
            RoundedRectangle(cornerRadius: 1).fill(.separator).frame(width: 2)
            Text("\u{201C}\(text)\u{201D}").font(.hubEvidence).foregroundStyle(.secondary).textSelection(.enabled)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}
