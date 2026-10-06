import SwiftUI

/// A button's title with the key that does the same beside it: Pursue P.
struct KeyLabel: View {
    let title: String
    var systemImage: String?
    let key: String

    var body: some View {
        HStack(spacing: Space.xs) {
            if let systemImage {
                Label(title, systemImage: systemImage)
            } else {
                Text(title)
            }
            Text(key)
                .font(.caption2.weight(.medium))
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)
        }
    }
}
