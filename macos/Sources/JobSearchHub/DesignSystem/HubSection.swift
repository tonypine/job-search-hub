import SwiftUI

/// A section: its title, an optional trailing link or control, and its rows
/// `s` apart. Sections stack `l` apart.
struct HubSection<Content: View, Trailing: View>: View {
    let title: String
    @ViewBuilder let content: Content
    @ViewBuilder let trailing: Trailing

    init(_ title: String, @ViewBuilder content: () -> Content, @ViewBuilder trailing: () -> Trailing) {
        self.title = title
        self.content = content()
        self.trailing = trailing()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text(title).font(.hubSection).accessibilityAddTraits(.isHeader)
                Spacer(minLength: 0)
                trailing
            }
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

extension HubSection where Trailing == EmptyView {
    init(_ title: String, @ViewBuilder content: () -> Content) {
        self.init(title, content: content) { EmptyView() }
    }
}
