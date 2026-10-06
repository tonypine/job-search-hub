import SwiftUI

/// A view's actions: one primary, the next step for what it shows; up to two
/// secondary; and the rest in an overflow menu. Buttons are capsules. An
/// accessory, such as an icon that opens the posting, sits at the trailing
/// edge beside the overflow.
struct ActionBar<Primary: View, Secondary: View, Overflow: View, Accessory: View>: View {
    @ViewBuilder let primary: Primary
    @ViewBuilder let secondary: Secondary
    @ViewBuilder let overflow: Overflow
    @ViewBuilder let accessory: Accessory

    init(
        @ViewBuilder primary: () -> Primary, @ViewBuilder secondary: () -> Secondary,
        @ViewBuilder overflow: () -> Overflow, @ViewBuilder accessory: () -> Accessory
    ) {
        self.primary = primary()
        self.secondary = secondary()
        self.overflow = overflow()
        self.accessory = accessory()
    }

    var body: some View {
        HStack(spacing: Space.s) {
            primary
                .buttonStyle(.borderedProminent)
                .tint(.hubAccent)
            secondary
                .buttonStyle(.bordered)
            if Accessory.self != EmptyView.self {
                Spacer(minLength: 0)
                accessory
                    .buttonStyle(.bordered)
                    .labelStyle(.iconOnly)
            }
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

extension ActionBar where Accessory == EmptyView {
    init(@ViewBuilder primary: () -> Primary, @ViewBuilder secondary: () -> Secondary, @ViewBuilder overflow: () -> Overflow) {
        self.init(primary: primary, secondary: secondary, overflow: overflow) { EmptyView() }
    }
}

extension ActionBar where Overflow == EmptyView, Accessory == EmptyView {
    init(@ViewBuilder primary: () -> Primary, @ViewBuilder secondary: () -> Secondary) {
        self.init(primary: primary, secondary: secondary) { EmptyView() }
    }
}
