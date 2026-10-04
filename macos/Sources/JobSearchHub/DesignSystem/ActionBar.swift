import SwiftUI

/// A view's actions: one primary, the next step for what it shows; up to two
/// secondary; and the rest in an overflow menu. Buttons are capsules.
struct ActionBar<Primary: View, Secondary: View, Overflow: View>: View {
    @ViewBuilder let primary: Primary
    @ViewBuilder let secondary: Secondary
    @ViewBuilder let overflow: Overflow

    init(
        @ViewBuilder primary: () -> Primary, @ViewBuilder secondary: () -> Secondary,
        @ViewBuilder overflow: () -> Overflow
    ) {
        self.primary = primary()
        self.secondary = secondary()
        self.overflow = overflow()
    }

    var body: some View {
        HStack(spacing: Space.s) {
            primary
                .buttonStyle(.borderedProminent)
                .tint(.hubAccent)
            secondary
                .buttonStyle(.bordered)
            if Overflow.self != EmptyView.self {
                Menu {
                    overflow
                } label: {
                    Label("More", systemImage: "ellipsis")
                }
                .menuStyle(.button)
                .buttonStyle(.bordered)
                .menuIndicator(.hidden)
                .labelStyle(.iconOnly)
                .fixedSize()
                .help("More actions")
            }
        }
        .buttonBorderShape(.capsule)
    }
}

extension ActionBar where Overflow == EmptyView {
    init(@ViewBuilder primary: () -> Primary, @ViewBuilder secondary: () -> Secondary) {
        self.init(primary: primary, secondary: secondary) { EmptyView() }
    }
}
