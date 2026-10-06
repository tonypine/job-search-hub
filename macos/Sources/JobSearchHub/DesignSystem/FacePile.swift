import JobSearchHubCore
import SwiftUI

/// The people you know somewhere, as overlapping faces of their initials,
/// then how many there are. The names are its tooltip and its label.
struct FacePile: View {
    let names: [String]
    let count: Int
    var size: CGFloat = 20

    var body: some View {
        HStack(spacing: Space.xs) {
            HStack(spacing: -size * 0.25) {
                ForEach(Array(names.prefix(3).enumerated()), id: \.offset) { _, name in
                    Text(Faces.getInitials(name))
                        .font(.system(size: size * 0.4, weight: .semibold))
                        .foregroundStyle(Tone.neutral.color)
                        .frame(width: size, height: size)
                        // The tint over an opaque circle, so the face behind doesn't show through.
                        .background(Circle().fill(Tone.neutral.fill))
                        .background(Circle().fill(Color(nsColor: .controlBackgroundColor)))
                        .overlay(Circle().strokeBorder(Color(nsColor: .windowBackgroundColor), lineWidth: 1.5))
                }
            }
            if count > 0 {
                Text("\(count)").font(.hubSecondary).foregroundStyle(.secondary).monospacedDigit()
            }
        }
        .lineLimit(1)
        .help(names.joined(separator: ", "))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(count == 1 ? "1 person you know" : "\(count) people you know")
    }
}
