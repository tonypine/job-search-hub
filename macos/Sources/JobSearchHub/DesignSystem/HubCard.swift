import SwiftUI

/// A group in a detail or on a page (P5): its title, quiet meta beside it,
/// one trailing link, then its content.
struct HubCard<Content: View, Trailing: View>: View {
    let title: String
    var meta: String?
    @ViewBuilder let content: Content
    @ViewBuilder let trailing: Trailing

    init(_ title: String, meta: String? = nil, @ViewBuilder content: () -> Content, @ViewBuilder trailing: () -> Trailing) {
        self.title = title
        self.meta = meta
        self.content = content()
        self.trailing = trailing()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text(title).font(.hubSection).accessibilityAddTraits(.isHeader)
                if let meta, !meta.isEmpty {
                    Text(meta).font(.hubCaption).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer(minLength: 0)
                trailing
                    .buttonStyle(.link)
                    .lineLimit(1)
            }
            content
        }
        .hubCard()
    }
}

extension HubCard where Trailing == EmptyView {
    init(_ title: String, meta: String? = nil, @ViewBuilder content: () -> Content) {
        self.init(title, meta: meta, content: content) { EmptyView() }
    }
}
