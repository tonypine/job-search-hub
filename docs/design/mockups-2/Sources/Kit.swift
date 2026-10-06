// The tokens and pieces every mockup draws with. The tokens are the app's
// (macos/Sources/JobSearchHub/DesignSystem/Tokens.swift); the pieces only
// sketch the components the proposal names.
import AppKit
import SwiftUI

// MARK: - Tokens

enum Space {
    static let xs: CGFloat = 4
    static let s: CGFloat = 8
    static let m: CGFloat = 12
    static let l: CGFloat = 16
    static let xl: CGFloat = 24
    static let xxl: CGFloat = 32
}

enum Radius {
    static let control: CGFloat = 6
    static let card: CGFloat = 10
    static let panel: CGFloat = 14
}

extension Color {
    init(hex: UInt32) {
        self.init(red: Double((hex >> 16) & 0xFF) / 255, green: Double((hex >> 8) & 0xFF) / 255, blue: Double(hex & 0xFF) / 255)
    }
}

enum Tone {
    case accent, positive, caution, negative, neutral

    var color: Color {
        switch self {
        case .accent: Color(hex: 0x4B49D6)
        case .positive: Color(hex: 0x1E8E50)
        case .caution: Color(hex: 0xB86E00)
        case .negative: Color(hex: 0xD13438)
        case .neutral: Color(hex: 0x6E6E73)
        }
    }

    var fill: Color { color.opacity(0.12) }
}

let ink = Color(hex: 0x1D1D1F)
let secondaryInk = Color(hex: 0x6E6E73)
let tertiaryInk = Color(hex: 0xA1A1A6)
let separator = Color(hex: 0xE3E3E8)
let windowBackground = Color(hex: 0xFBFBFD)
let sidebarBackground = Color(hex: 0xF0F0F4)
let well = Color(hex: 0xF3F3F7)
let surface = Color.white
let fill = Color.black.opacity(0.06)
let fillSubtle = Color.black.opacity(0.04)
let hairline = Color.black.opacity(0.06)
let desk = Color(hex: 0xDADBE3)
let board = Color(hex: 0xF6F6F9)
/// What a highlight from the screen or the brief sits on in the posting.
let highlightCaution = Color(hex: 0xFFE7B8)
let highlightPositive = Color(hex: 0xD3F0DF)
let annotation = Color(hex: 0xE5484D)

extension Font {
    static func ui(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font { .system(size: size, weight: weight) }
}

// MARK: - Small pieces

struct Chip: View {
    let text: String
    var tone: Tone = .neutral
    var symbol: String?

    var body: some View {
        HStack(spacing: 3) {
            if let symbol { Image(systemName: symbol).font(.system(size: 9, weight: .bold)) }
            Text(text).font(.ui(11, .semibold))
        }
        .lineLimit(1)
        .padding(.horizontal, 7)
        .padding(.vertical, 2.5)
        .foregroundStyle(tone.color)
        .background(tone.fill, in: Capsule())
        .fixedSize()
    }
}

struct UnseenDot: View {
    var body: some View { Circle().fill(Tone.accent.color).frame(width: 7, height: 7) }
}

struct PrimaryButton: View {
    let title: String
    var symbol: String?
    var shortcut: String?
    var body: some View {
        HStack(spacing: 5) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11, weight: .semibold)) }
            Text(title).font(.ui(12, .semibold))
            if let shortcut { Text(shortcut).font(.ui(10, .semibold)).opacity(0.7) }
        }
        .padding(.horizontal, 12).padding(.vertical, 5)
        .foregroundStyle(.white)
        .background(Tone.accent.color, in: Capsule())
        .fixedSize()
    }
}

struct SecondaryButton: View {
    let title: String
    var symbol: String?
    var shortcut: String?
    var body: some View {
        HStack(spacing: 5) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11, weight: .medium)) }
            Text(title).font(.ui(12, .medium))
            if let shortcut { Text(shortcut).font(.ui(10, .semibold)).foregroundStyle(tertiaryInk) }
        }
        .padding(.horizontal, 11).padding(.vertical, 5)
        .foregroundStyle(ink)
        .background(fill, in: Capsule())
        .fixedSize()
    }
}

struct IconButton: View {
    let symbol: String
    var isOn = false
    var body: some View {
        Image(systemName: symbol).font(.system(size: 12, weight: .medium))
            .frame(width: 26, height: 24)
            .foregroundStyle(isOn ? Tone.accent.color : ink)
            .background(isOn ? Tone.accent.fill : fill, in: Capsule())
    }
}

struct PlainIcon: View {
    let symbol: String
    var color: Color = secondaryInk
    var body: some View {
        Image(systemName: symbol).font(.system(size: 13, weight: .regular)).foregroundStyle(color).frame(width: 22, height: 22)
    }
}

struct LinkText: View {
    let text: String
    var symbol: String?
    var body: some View {
        HStack(spacing: 3) {
            Text(text)
            if let symbol { Image(systemName: symbol).font(.system(size: 9, weight: .semibold)) }
        }
        .font(.ui(12)).foregroundStyle(Tone.accent.color)
    }
}

struct Keycap: View {
    let key: String
    var body: some View {
        Text(key).font(.ui(10, .semibold)).foregroundStyle(secondaryInk)
            .frame(minWidth: 16).padding(.horizontal, 4).padding(.vertical, 1)
            .background(surface, in: RoundedRectangle(cornerRadius: 4))
            .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(separator))
    }
}

struct Spinner: View {
    var body: some View {
        Circle().trim(from: 0, to: 0.72)
            .stroke(secondaryInk, style: StrokeStyle(lineWidth: 1.6, lineCap: .round))
            .rotationEffect(.degrees(-90))
            .frame(width: 11, height: 11)
    }
}

struct Monogram: View {
    let letters: String
    var hue: Color = Tone.accent.color
    var size: CGFloat = 22
    var body: some View {
        Text(letters).font(.system(size: size * 0.42, weight: .bold)).foregroundStyle(hue)
            .frame(width: size, height: size)
            .background(hue.opacity(0.12), in: RoundedRectangle(cornerRadius: size * 0.27, style: .continuous))
    }
}

struct Switch: View {
    let isOn: Bool
    var body: some View {
        Capsule().fill(isOn ? Tone.accent.color : fill)
            .frame(width: 30, height: 18)
            .overlay(alignment: isOn ? .trailing : .leading) {
                Circle().fill(.white).frame(width: 14, height: 14).padding(2).shadow(color: .black.opacity(0.15), radius: 1, y: 1)
            }
    }
}

struct SearchBox: View {
    let prompt: String
    var width: CGFloat = 200
    var shortcut: String?
    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass").font(.system(size: 11)).foregroundStyle(secondaryInk)
            Text(prompt).font(.ui(12)).foregroundStyle(tertiaryInk).lineLimit(1)
            Spacer(minLength: 0)
            if let shortcut { Keycap(key: shortcut) }
        }
        .padding(.horizontal, 9).padding(.vertical, 5)
        .frame(width: width)
        .background(fillSubtle, in: RoundedRectangle(cornerRadius: 7))
        .overlay(RoundedRectangle(cornerRadius: 7).strokeBorder(hairline))
    }
}

// MARK: - Verdicts

enum Verdict {
    case pass, unclear, fail, info

    var symbol: String {
        switch self {
        case .pass: "checkmark.circle.fill"
        case .unclear: "questionmark.circle.fill"
        case .fail: "xmark.circle.fill"
        case .info: "info.circle"
        }
    }

    var tone: Tone {
        switch self {
        case .pass: .positive
        case .unclear: .caution
        case .fail: .negative
        case .info: .neutral
        }
    }
}

struct VerdictRow: View {
    let verdict: Verdict
    let name: String
    let reason: String
    var size: CGFloat = 12
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: verdict.symbol).foregroundStyle(verdict.tone.color).font(.system(size: size))
            (Text(name).fontWeight(.medium).foregroundColor(ink) + Text("  " + reason).foregroundColor(secondaryInk))
                .font(.ui(size))
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}

struct Evidence: View {
    let text: String
    var body: some View {
        HStack(spacing: Space.s) {
            RoundedRectangle(cornerRadius: 1).fill(separator).frame(width: 2)
            Text("\u{201C}\(text)\u{201D}").font(.ui(11)).italic().foregroundStyle(secondaryInk)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}

struct FactRow: View {
    let label: String
    let value: String
    var labelWidth: CGFloat = 96
    var valueColor: Color = ink
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Text(label).font(.ui(12)).foregroundStyle(secondaryInk).frame(width: labelWidth, alignment: .trailing)
            Text(value).font(.ui(12)).foregroundStyle(valueColor).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
    }
}

// MARK: - Sections and cards

struct SectionTitle: View {
    let text: String
    var meta: String?
    var trailing: String?
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Text(text).font(.ui(13, .semibold)).foregroundStyle(ink)
            if let meta { Text(meta).font(.ui(11)).foregroundStyle(tertiaryInk) }
            Spacer(minLength: 0)
            if let trailing { LinkText(text: trailing) }
        }
    }
}

/// A group on a page or in the inspector: a title row, then its content, on
/// a white card with a hairline.
struct Card<Content: View>: View {
    let title: String
    var meta: String?
    var trailing: String?
    var padding: CGFloat = Space.l
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            SectionTitle(text: title, meta: meta, trailing: trailing)
            content
        }
        .padding(padding)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }
}

/// A folded section: its title and a one-line summary, with a chevron.
struct DisclosureRow: View {
    let title: String
    var summary: String?
    var isOpen = false
    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: isOpen ? "chevron.down" : "chevron.right").font(.system(size: 9, weight: .bold)).foregroundStyle(secondaryInk).frame(width: 10)
            Text(title).font(.ui(12, .semibold)).foregroundStyle(ink)
            if let summary { Text(summary).font(.ui(12)).foregroundStyle(secondaryInk).lineLimit(1) }
            Spacer(minLength: 0)
        }
    }
}

// MARK: - Tabs

/// The proposed tab strip: text tabs with an accent underline, a count or a
/// dot where a tab holds news, over a hairline.
struct TabStrip: View {
    let tabs: [(String, String?)]
    let selected: String
    var size: CGFloat = 12.5
    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: Space.l + 2) {
                ForEach(Array(tabs.enumerated()), id: \.offset) { _, tab in
                    VStack(spacing: 6) {
                        HStack(spacing: 4) {
                            Text(tab.0).font(.ui(size, tab.0 == selected ? .semibold : .regular))
                                .foregroundStyle(tab.0 == selected ? ink : secondaryInk)
                            if let badge = tab.1 {
                                if badge == "•" {
                                    Circle().fill(Tone.caution.color).frame(width: 6, height: 6)
                                } else {
                                    Text(badge).font(.ui(10.5, .semibold)).monospacedDigit().foregroundStyle(tertiaryInk)
                                }
                            }
                        }
                        Rectangle().fill(tab.0 == selected ? Tone.accent.color : .clear).frame(height: 2).clipShape(Capsule())
                    }
                    .fixedSize()
                }
                Spacer(minLength: 0)
            }
            Rectangle().fill(separator).frame(height: 1)
        }
    }
}

/// Today's tabs: a segmented control at its natural width.
struct SegmentedTabs: View {
    let names: [String]
    let selected: String
    var stretches = false
    var body: some View {
        HStack(spacing: 0) {
            ForEach(names, id: \.self) { tab in
                Text(tab).font(.ui(12, tab == selected ? .semibold : .regular))
                    .foregroundStyle(ink)
                    .lineLimit(1)
                    .padding(.horizontal, 12).padding(.vertical, 3)
                    .frame(maxWidth: stretches ? .infinity : nil)
                    .background(tab == selected ? AnyShapeStyle(Color.white) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 5))
                    .shadow(color: .black.opacity(tab == selected ? 0.12 : 0), radius: 1, y: 0.5)
            }
        }
        .padding(2)
        .background(fill, in: RoundedRectangle(cornerRadius: 7))
        .fixedSize(horizontal: !stretches, vertical: true)
    }
}

// MARK: - Filters and tokens

/// A filter that's on, with its value and a cross to take it off.
struct FilterChip: View {
    let text: String
    var isOn = true
    var hasMenu = false
    var body: some View {
        HStack(spacing: 4) {
            Text(text).font(.ui(11.5, isOn ? .medium : .regular))
            if hasMenu {
                Image(systemName: "chevron.down").font(.system(size: 8, weight: .bold))
            } else if isOn {
                Image(systemName: "xmark").font(.system(size: 8, weight: .bold)).opacity(0.7)
            }
        }
        .foregroundStyle(isOn ? Tone.accent.color : ink)
        .padding(.horizontal, 9).padding(.vertical, 4)
        .background(isOn ? Tone.accent.fill : surface, in: Capsule())
        .overlay(Capsule().strokeBorder(isOn ? Tone.accent.color.opacity(0.25) : separator))
        .fixedSize()
    }
}

/// One value in a token field.
struct Token: View {
    let text: String
    var tone: Tone = .neutral
    var body: some View {
        HStack(spacing: 4) {
            Text(text).font(.ui(11.5)).foregroundStyle(tone == .neutral ? ink : tone.color)
            Image(systemName: "xmark").font(.system(size: 7, weight: .bold)).foregroundStyle(tertiaryInk)
        }
        .padding(.horizontal, 7).padding(.vertical, 2.5)
        .background(tone == .neutral ? fill : tone.fill, in: RoundedRectangle(cornerRadius: 5))
        .fixedSize()
    }
}

/// A field of tokens that wraps, with room to type the next.
struct TokenField: View {
    let tokens: [String]
    var tone: Tone = .neutral
    var prompt = "Add…"
    var body: some View {
        FlowLayout(spacing: 5) {
            ForEach(tokens, id: \.self) { Token(text: $0, tone: tone) }
            Text(prompt).font(.ui(11.5)).foregroundStyle(tertiaryInk).padding(.vertical, 2.5)
        }
        .padding(5)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.control))
        .overlay(RoundedRectangle(cornerRadius: Radius.control).strokeBorder(separator))
    }
}

/// Lays its views out in rows, wrapping at the width it's given.
struct FlowLayout: Layout {
    var spacing: CGFloat = 6
    var lineSpacing: CGFloat?

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? 10_000
        let rows = arrange(subviews, width: width)
        let height = rows.last.map { $0.y + $0.height } ?? 0
        let usedWidth = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? usedWidth, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for row in arrange(subviews, width: bounds.width) {
            var x = bounds.minX
            for index in row.indices {
                let size = subviews[index].sizeThatFits(.unspecified)
                subviews[index].place(at: CGPoint(x: x, y: bounds.minY + row.y + (row.height - size.height) / 2), proposal: .unspecified)
                x += size.width + spacing
            }
        }
    }

    private struct Row {
        var indices: [Int] = []
        var y: CGFloat = 0
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func arrange(_ subviews: Subviews, width: CGFloat) -> [Row] {
        var rows: [Row] = [Row()]
        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            if !rows[rows.count - 1].indices.isEmpty && rows[rows.count - 1].width + spacing + size.width > width {
                let last = rows[rows.count - 1]
                rows.append(Row(y: last.y + last.height + (lineSpacing ?? spacing)))
            }
            var row = rows[rows.count - 1]
            row.width += (row.indices.isEmpty ? 0 : spacing) + size.width
            row.height = max(row.height, size.height)
            row.indices.append(index)
            rows[rows.count - 1] = row
        }
        return rows
    }
}

// MARK: - Window chrome

struct TrafficLights: View {
    var body: some View {
        HStack(spacing: 8) {
            Circle().fill(Color(hex: 0xFF5F57)).frame(width: 12, height: 12)
            Circle().fill(Color(hex: 0xFEBC2E)).frame(width: 12, height: 12)
            Circle().fill(Color(hex: 0x28C840)).frame(width: 12, height: 12)
        }
    }
}

struct SidebarItem: View {
    let title: String
    let symbol: String
    var badge: Int?
    var isSelected = false
    var badgeTone: Tone?
    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 13)).frame(width: 18)
                .foregroundStyle(isSelected ? .white : Tone.accent.color)
            Text(title).font(.ui(13)).foregroundStyle(isSelected ? .white : ink)
            Spacer()
            if let badge {
                if let badgeTone, !isSelected {
                    Text("\(badge)").font(.ui(10.5, .bold)).monospacedDigit().foregroundStyle(.white)
                        .padding(.horizontal, 5).padding(.vertical, 1).background(badgeTone.color, in: Capsule())
                } else {
                    Text("\(badge)").font(.ui(11, .semibold)).monospacedDigit()
                        .foregroundStyle(isSelected ? .white : secondaryInk)
                }
            }
        }
        .padding(.horizontal, Space.s).padding(.vertical, 5)
        .background(isSelected ? AnyShapeStyle(Tone.accent.color) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: Radius.control + 2))
    }
}

struct SidebarHeader: View {
    let title: String
    var collapsed = false
    var body: some View {
        HStack {
            Text(title).font(.ui(11, .semibold)).foregroundStyle(tertiaryInk)
            Spacer()
            Image(systemName: collapsed ? "chevron.right" : "chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(tertiaryInk)
        }
        .padding(.horizontal, Space.s).padding(.top, Space.m).padding(.bottom, 2)
    }
}

/// The sidebar as it's built today (TP-451): grouped by intent, sessions
/// listed at the end, and nothing that says how the hub is doing.
struct Sidebar: View {
    var selected = "Jobs"
    var proposed = false
    var width: CGFloat = 220
    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            TrafficLights().padding(.leading, 6).padding(.top, 4).padding(.bottom, Space.l)
            SidebarItem(title: "Today", symbol: "sun.max", badge: 4, isSelected: selected == "Today")
            SidebarItem(title: "Decide", symbol: "checklist", badge: 7, isSelected: selected == "Decide")
            SidebarItem(title: "Pipeline", symbol: "rectangle.split.3x1", badge: 2, isSelected: selected == "Pipeline", badgeTone: proposed ? .negative : nil)
            SidebarHeader(title: "Browse")
            SidebarItem(title: "Jobs", symbol: "briefcase", isSelected: selected == "Jobs")
            SidebarItem(title: "Companies", symbol: "building.2", isSelected: selected == "Companies")
            SidebarItem(title: "People", symbol: "person.2", isSelected: selected == "People")
            SidebarHeader(title: "You")
            SidebarItem(title: "Profile", symbol: "person.crop.circle", isSelected: selected == "Profile")
            SidebarItem(title: "Criteria", symbol: "slider.horizontal.3", isSelected: selected == "Criteria")
            SidebarHeader(title: "Hub", collapsed: true)
            if !proposed {
                SidebarHeader(title: "Sessions")
                sessionRow("Northwind", "Waiting for you", Tone.caution.color)
                sessionRow("Senior Product Engineer", "Working", Tone.accent.color)
                sessionRow("Globex", "Idle", Tone.positive.color)
            }
            Spacer(minLength: 0)
            if proposed { HubStatusFooter() }
        }
        .padding(Space.s)
        .frame(width: width)
        .frame(maxHeight: .infinity)
        .background(sidebarBackground, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(hairline))
        .padding(Space.s)
    }

    private func sessionRow(_ name: String, _ state: String, _ color: Color) -> some View {
        HStack(spacing: Space.s) {
            Circle().fill(color).frame(width: 7, height: 7).frame(width: 18)
            VStack(alignment: .leading, spacing: 0) {
                Text(name).font(.ui(12)).foregroundStyle(ink).lineLimit(1)
                Text(state).font(.ui(10)).foregroundStyle(secondaryInk)
            }
        }
        .padding(.horizontal, Space.s).padding(.vertical, 3)
    }
}

/// The proposed foot of the sidebar: the hub's connection and what it's
/// doing, and the sessions that need you, in one place.
struct HubStatusFooter: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(spacing: Space.s) {
                Circle().fill(Tone.positive.color).frame(width: 7, height: 7)
                Text("Hub connected").font(.ui(11.5, .medium)).foregroundStyle(ink)
                Spacer()
                Image(systemName: "pause.circle").font(.system(size: 12)).foregroundStyle(secondaryInk)
            }
            HStack(spacing: Space.s) {
                Spinner().scaleEffect(0.8)
                Text("Reading 3 postings").font(.ui(11)).foregroundStyle(secondaryInk)
            }
            HStack(spacing: Space.s) {
                Circle().fill(Tone.caution.color).frame(width: 7, height: 7)
                Text("Northwind session waits for you").font(.ui(11)).foregroundStyle(secondaryInk).lineLimit(1)
            }
        }
        .padding(Space.m)
        .background(Color.white.opacity(0.7), in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(hairline))
    }
}

/// The window's title area as macOS draws it: the page's name and subtitle.
struct TitleBar<Trailing: View>: View {
    let title: String
    var subtitle: String?
    @ViewBuilder var trailing: Trailing
    var body: some View {
        HStack(spacing: Space.m) {
            VStack(alignment: .leading, spacing: 0) {
                Text(title).font(.ui(15, .semibold)).foregroundStyle(ink)
                if let subtitle { Text(subtitle).font(.ui(11)).foregroundStyle(secondaryInk) }
            }
            Spacer(minLength: 0)
            trailing
        }
        .padding(.horizontal, Space.l)
        .frame(height: 52)
    }
}

extension TitleBar where Trailing == EmptyView {
    init(title: String, subtitle: String? = nil) {
        self.init(title: title, subtitle: subtitle) { EmptyView() }
    }
}

/// The proposed page header: the page's scopes on the left, its search and
/// one Add on the right, then the filters that are on, as chips.
struct PageHeader: View {
    let scopes: [(String, String?)]
    let selectedScope: String
    var filters: [String] = []
    var addFilter = true
    var search = "Search"
    var addTitle: String? = "Add"
    var trailingNote: String?
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(spacing: Space.m) {
                TabStrip(tabs: scopes, selected: selectedScope).fixedSize().offset(y: 7)
                Spacer(minLength: 0)
                SearchBox(prompt: search, width: 200)
                if let addTitle {
                    HStack(spacing: 4) {
                        Image(systemName: "plus").font(.system(size: 11, weight: .semibold))
                        Text(addTitle).font(.ui(12, .medium))
                    }
                    .padding(.horizontal, 10).padding(.vertical, 5)
                    .foregroundStyle(ink)
                    .background(fill, in: Capsule())
                }
            }
            if !filters.isEmpty || addFilter {
                HStack(spacing: 6) {
                    ForEach(filters, id: \.self) { FilterChip(text: $0) }
                    if addFilter {
                        HStack(spacing: 4) {
                            Image(systemName: "line.3.horizontal.decrease").font(.system(size: 10, weight: .semibold))
                            Text("Filter").font(.ui(11.5))
                        }
                        .foregroundStyle(secondaryInk)
                        .padding(.horizontal, 9).padding(.vertical, 4)
                        .overlay(Capsule().strokeBorder(separator, style: StrokeStyle(lineWidth: 1, dash: [3, 2])))
                    }
                    Spacer(minLength: 0)
                    if let trailingNote { Text(trailingNote).font(.ui(11)).foregroundStyle(tertiaryInk) }
                }
                .padding(.top, 4)
            }
        }
        .padding(.horizontal, Space.l)
        .padding(.bottom, Space.s + 2)
        .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
    }
}

/// The inspector column's own bar: back, forward, open as a page, close.
struct InspectorBar: View {
    var expands = true
    var body: some View {
        HStack(spacing: 2) {
            PlainIcon(symbol: "chevron.backward")
            PlainIcon(symbol: "chevron.forward", color: tertiaryInk)
            Spacer()
            if expands { PlainIcon(symbol: "arrow.up.left.and.arrow.down.right") }
            PlainIcon(symbol: "sidebar.trailing")
        }
        .padding(.horizontal, Space.s)
        .frame(height: 40)
    }
}

/// A whole window on a board.
struct WindowFrame<Content: View>: View {
    var width: CGFloat
    var height: CGFloat
    @ViewBuilder let content: Content
    var body: some View {
        content
            .frame(width: width, height: height, alignment: .topLeading)
            .background(windowBackground)
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.black.opacity(0.14)))
            .compositingGroup()
            .shadow(color: .black.opacity(0.14), radius: 18, y: 8)
    }
}

/// A panel drawn on a board on its own, like the inspector column.
struct PanelFrame<Content: View>: View {
    var width: CGFloat
    var height: CGFloat?
    @ViewBuilder let content: Content
    var body: some View {
        content
            .frame(width: width, alignment: .topLeading)
            .frame(height: height, alignment: .topLeading)
            .background(surface, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.black.opacity(0.12)))
            .compositingGroup()
            .shadow(color: .black.opacity(0.10), radius: 14, y: 6)
    }
}

// MARK: - Annotation

struct Marker: View {
    let number: Int
    var color = annotation
    var body: some View {
        Text("\(number)").font(.ui(11, .bold)).foregroundStyle(.white)
            .frame(width: 20, height: 20).background(color, in: Circle())
            .overlay(Circle().strokeBorder(.white, lineWidth: 1.5))
    }
}

struct BoardTitle: View {
    let eyebrow: String
    let title: String
    let subtitle: String
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(eyebrow.uppercased()).font(.ui(11, .bold)).kerning(0.6).foregroundStyle(Tone.accent.color)
            Text(title).font(.ui(26, .bold)).foregroundStyle(ink)
            Text(subtitle).font(.ui(14)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
    }
}

struct ColumnHeading: View {
    let title: String
    let subtitle: String
    var tone: Tone?
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: Space.s) {
                if let tone { Circle().fill(tone.color).frame(width: 8, height: 8) }
                Text(title).font(.ui(16, .semibold)).foregroundStyle(ink)
            }
            Text(subtitle).font(.ui(12.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// A problem in the before picture, and what the proposal does about it.
struct ProblemNote: View {
    let number: Int
    let problem: String
    let fix: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Marker(number: number)
            VStack(alignment: .leading, spacing: 3) {
                Text(problem).font(.ui(13, .semibold)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                    Image(systemName: "arrow.turn.down.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.positive.color)
                    Text(fix).font(.ui(12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

/// A board: a heading, then its content, on a light background.
struct Board<Content: View>: View {
    let eyebrow: String
    let title: String
    let subtitle: String
    var width: CGFloat
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xxl) {
            BoardTitle(eyebrow: eyebrow, title: title, subtitle: subtitle).frame(maxWidth: 900, alignment: .leading)
            content
        }
        .padding(48)
        .frame(width: width, alignment: .topLeading)
        .background(board)
    }
}

extension EnvironmentValues {
    /// Off for a view drawn again inside a bigger mockup, whose own markers
    /// would be out of place there.
    var showsMarkers: Bool {
        get { self[ShowsMarkersKey.self] }
        set { self[ShowsMarkersKey.self] = newValue }
    }
}

private struct ShowsMarkersKey: EnvironmentKey {
    static let defaultValue = true
}

struct PinnedMarker: ViewModifier {
    let number: Int
    let alignment: Alignment
    let x: CGFloat
    let y: CGFloat
    @Environment(\.showsMarkers) private var showsMarkers

    func body(content: Content) -> some View {
        content.overlay(alignment: alignment) {
            if showsMarkers { Marker(number: number).offset(x: x, y: y) }
        }
    }
}

extension View {
    /// Pins a numbered marker to a corner of the view, outside its edge.
    func marker(_ number: Int, _ alignment: Alignment = .topLeading, x: CGFloat = -30, y: CGFloat = 0) -> some View {
        modifier(PinnedMarker(number: number, alignment: alignment, x: x, y: y))
    }

    func withoutMarkers() -> some View { environment(\.showsMarkers, false) }
}

// MARK: - Rendering

@MainActor
func write(_ view: some View, to directory: URL, name: String) {
    let renderer = ImageRenderer(content: view.environment(\.colorScheme, .light))
    renderer.scale = 2
    guard let image = renderer.nsImage, let tiff = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff),
          let png = bitmap.representation(using: .png, properties: [:])
    else {
        FileHandle.standardError.write("could not render \(name)\n".data(using: .utf8)!)
        exit(1)
    }
    try! png.write(to: directory.appending(path: name))
    print("wrote \(name) \(Int(image.size.width))×\(Int(image.size.height))")
}

@MainActor
func snapshot(_ view: some View) -> NSImage {
    let renderer = ImageRenderer(content: view.environment(\.colorScheme, .light))
    renderer.scale = 2
    return renderer.nsImage!
}
