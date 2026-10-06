import JobSearchHubCore
import SwiftUI

/// One tab of a tab strip: its title, and a count or a dot when it holds
/// something new.
struct TabStripItem<ID: Hashable>: Identifiable {
    let id: ID
    let title: String
    /// How many items the tab holds, shown after its title; nil shows none.
    var count: Int?
    /// A dot in the tone after the title: news, such as a session waiting
    /// for you.
    var dot: Tone?
}

/// Tabs as text with an accent underline under the one shown, left-aligned.
/// Its height is constant, and it asks for no width of its own: tabs that
/// don't fit scroll sideways, so what it shows never resizes the column it
/// sits in (see the inspector's column in ContentView).
struct TabStrip<ID: Hashable>: View {
    static var height: CGFloat { 30 }

    let items: [TabStripItem<ID>]
    @Binding var selection: ID
    /// A hairline under the whole strip, where nothing below draws one.
    var showsRule = false

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(alignment: .center, spacing: Space.l) {
                ForEach(items) { item in tab(item) }
            }
            .frame(height: Self.height)
        }
        .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        .frame(minWidth: 0, maxWidth: .infinity, minHeight: Self.height, maxHeight: Self.height, alignment: .leading)
        .overlay(alignment: .bottom) {
            if showsRule { Divider() }
        }
    }

    private func tab(_ item: TabStripItem<ID>) -> some View {
        let isSelected = item.id == selection
        return Button {
            selection = item.id
        } label: {
            HStack(spacing: Space.xs) {
                Text(item.title)
                    .fontWeight(isSelected ? .semibold : .regular)
                    .foregroundStyle(isSelected ? .primary : .secondary)
                if let count = item.count {
                    Text(count.formatted())
                        .font(.hubCaption)
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                }
                if let dot = item.dot {
                    Circle().fill(dot.color).frame(width: 6, height: 6)
                }
            }
            .lineLimit(1)
            .fixedSize()
            .frame(maxHeight: .infinity)
            .overlay(alignment: .bottom) {
                Rectangle()
                    .fill(isSelected ? Color.hubAccent : .clear)
                    .frame(height: 2)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(describe(item))
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    private func describe(_ item: TabStripItem<ID>) -> String {
        var label = item.title
        if let count = item.count { label += ", \(count)" }
        if item.dot != nil { label += ", new" }
        return label
    }
}
