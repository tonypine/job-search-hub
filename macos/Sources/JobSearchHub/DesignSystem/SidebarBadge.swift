import JobSearchHubCore
import SwiftUI

/// A count at the end of a sidebar row: a grey number, or white on red when
/// it needs you now. Nothing shows for zero.
struct SidebarBadge: View {
    let count: Int
    var isUrgent = false

    var body: some View {
        if count > 0 {
            Text(count.formatted())
                .font(.hubCaption.weight(isUrgent ? .semibold : .regular))
                .monospacedDigit()
                .foregroundStyle(isUrgent ? AnyShapeStyle(Color.white) : AnyShapeStyle(.secondary))
                .padding(.horizontal, isUrgent ? 6 : 0)
                .padding(.vertical, isUrgent ? 1 : 0)
                .background {
                    if isUrgent { Capsule().fill(Tone.negative.color) }
                }
                .accessibilityLabel(isUrgent ? "\(count), overdue" : "\(count)")
        }
    }
}
