import JobSearchHubCore
import SwiftUI

/// A filter that is on, as a chip in the page header: its words, and what
/// removing it does.
struct PageFilterChip: Identifiable {
    let id: String
    let title: String
    let remove: () -> Void
}

/// Every page's controls, at the top of its content: the page's scopes (a
/// tab strip) or a quiet line on the left, its search and one Add on the
/// right, and, on a page with filters, the filters that are on as chips you
/// can remove, with *+ Filter* to add one.
///
/// It stays out of the window's toolbar, which keeps only the page's title
/// and subtitle: with the details inspector open, the toolbar runs over the
/// inspector's column too, and items there looped AppKit's layout (#66,
/// #71). It never wraps and asks for no width of its own: each row has a
/// constant height, the scopes and the chips scroll sideways, and what
/// doesn't fit is clipped, so the page's width never follows its content.
struct PageHeader<Leading: View, Trailing: View, FilterMenu: View>: View {
    private let leading: Leading
    private let trailing: Trailing
    private let chips: [PageFilterChip]?
    private let filterMenu: FilterMenu

    /// A header without filters.
    init(@ViewBuilder leading: () -> Leading, @ViewBuilder trailing: () -> Trailing) where FilterMenu == EmptyView {
        self.leading = leading()
        self.trailing = trailing()
        chips = nil
        filterMenu = EmptyView()
    }

    /// A header with a row of filter chips under it, and the *+ Filter*
    /// menu's items.
    init(
        chips: [PageFilterChip], @ViewBuilder leading: () -> Leading, @ViewBuilder trailing: () -> Trailing,
        @ViewBuilder filterMenu: () -> FilterMenu
    ) {
        self.leading = leading()
        self.trailing = trailing()
        self.chips = chips
        self.filterMenu = filterMenu()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: Space.m) {
                leading
                    .lineLimit(1)
                    .frame(minWidth: 0, maxWidth: .infinity, alignment: .leading)
                HStack(spacing: Space.s) { trailing }
                    .fixedSize()
            }
            .frame(height: PageHeaderMetrics.rowHeight)
            if let chips {
                filterRow(chips)
            }
        }
        .padding(.horizontal, Space.l)
        .frame(minWidth: 0, maxWidth: .infinity, alignment: .leading)
        .clipped()
        .overlay(alignment: .bottom) { Divider() }
    }

    private func filterRow(_ chips: [PageFilterChip]) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: Space.s) {
                ForEach(chips) { chip in FilterChipView(chip: chip) }
                Menu {
                    filterMenu
                } label: {
                    Label("Filter", systemImage: "plus")
                }
                .menuStyle(.button)
                .buttonStyle(.borderless)
                .menuIndicator(.hidden)
                .fixedSize()
                .font(.hubSecondary)
                .padding(.horizontal, Space.s)
                .padding(.vertical, 2)
                .overlay(Capsule().strokeBorder(.secondary.opacity(0.5), style: StrokeStyle(lineWidth: 1, dash: [3, 2])))
                .help("Add a filter")
            }
            .frame(height: PageHeaderMetrics.chipRowHeight)
        }
        .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        .frame(height: PageHeaderMetrics.chipRowHeight)
        .padding(.bottom, Space.xs)
    }
}

/// The page header's sizes, the same on every page.
enum PageHeaderMetrics {
    /// The scopes, search and Add.
    static let rowHeight: CGFloat = 40
    /// The filter chips under them.
    static let chipRowHeight: CGFloat = 26
    /// The search field's width.
    static let searchWidth: CGFloat = 200
}

/// The search field in a page header, at the same width on every page.
struct PageSearchField: View {
    @Binding var text: String
    let prompt: String

    var body: some View {
        SearchField(text: $text, prompt: prompt)
            .frame(width: PageHeaderMetrics.searchWidth)
    }
}

/// A filter that is on: its words in the accent, and × to remove it.
private struct FilterChipView: View {
    let chip: PageFilterChip

    var body: some View {
        HStack(spacing: Space.xs) {
            Text(chip.title).lineLimit(1)
            Button("Remove", systemImage: "xmark", action: chip.remove)
                .labelStyle(.iconOnly)
                .buttonStyle(.plain)
                .imageScale(.small)
                .help("Remove the filter")
                .accessibilityLabel("Remove \(chip.title)")
        }
        .font(.hubSecondary)
        .foregroundStyle(Tone.accent.color)
        .fixedSize()
        .padding(.horizontal, Space.s)
        .padding(.vertical, 2)
        .background(Tone.accent.fill, in: Capsule())
    }
}
