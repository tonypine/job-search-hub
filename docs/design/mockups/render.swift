// Draws the redesign mockups in docs/design/mockups with SwiftUI, offscreen:
//
//     swift docs/design/mockups/render.swift docs/design/mockups
//
// Every company, job and person here is made up. The views only sketch the
// proposal in docs/design/ui-redesign.md; they are not app code.
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

enum Tone: CaseIterable {
    case accent, positive, caution, negative, neutral

    var light: Color {
        switch self {
        case .accent: Color(hex: 0x4B49D6)
        case .positive: Color(hex: 0x1E8E50)
        case .caution: Color(hex: 0xB86E00)
        case .negative: Color(hex: 0xD13438)
        case .neutral: Color(hex: 0x6E6E73)
        }
    }

    var dark: Color {
        switch self {
        case .accent: Color(hex: 0x8E8CFF)
        case .positive: Color(hex: 0x3CC97F)
        case .caution: Color(hex: 0xF2A33A)
        case .negative: Color(hex: 0xFF6B6B)
        case .neutral: Color(hex: 0x98989D)
        }
    }

    /// The tone for the mode being drawn.
    var color: Color { darkMode ? dark : light }

    var name: String {
        switch self {
        case .accent: "Accent · Hub Indigo"
        case .positive: "Positive"
        case .caution: "Caution"
        case .negative: "Negative"
        case .neutral: "Neutral"
        }
    }

    var usage: String {
        switch self {
        case .accent: "Selection, links, primary action, Possible match, unseen dot"
        case .positive: "Strong match, screen passes, heard back"
        case .caution: "Stretch match, unclear, due today, stale brief"
        case .negative: "Fails a screen, overdue, errors"
        case .neutral: "Mismatch, skipped, closed, metadata"
        }
    }

    var hexText: (String, String) {
        switch self {
        case .accent: ("#4B49D6", "#8E8CFF")
        case .positive: ("#1E8E50", "#3CC97F")
        case .caution: ("#B86E00", "#F2A33A")
        case .negative: ("#D13438", "#FF6B6B")
        case .neutral: ("#6E6E73", "#98989D")
        }
    }
}

/// Draws the next views in dark mode; `write` sets it per image.
nonisolated(unsafe) var darkMode = false

var ink: Color { darkMode ? Color(hex: 0xF2F2F5) : Color(hex: 0x1D1D1F) }
var secondaryInk: Color { darkMode ? Color(hex: 0x98989D) : Color(hex: 0x6E6E73) }
var tertiaryInk: Color { darkMode ? Color(hex: 0x6E6E73) : Color(hex: 0xA1A1A6) }
var separator: Color { darkMode ? Color(hex: 0x3A3A3E) : Color(hex: 0xE3E3E8) }
var windowBackground: Color { darkMode ? Color(hex: 0x1C1C1F) : Color(hex: 0xFBFBFD) }
var sidebarBackground: Color { darkMode ? Color(hex: 0x29292D) : Color(hex: 0xF0F0F4) }
var well: Color { darkMode ? Color(hex: 0x2C2C30) : Color(hex: 0xF2F2F6) }
/// Cards, the inspector and other raised content.
var surface: Color { darkMode ? Color(hex: 0x252528) : .white }
/// Secondary buttons, segmented controls and fields.
var fill: Color { darkMode ? .white.opacity(0.1) : .black.opacity(0.06) }
var fillSubtle: Color { darkMode ? .white.opacity(0.07) : .black.opacity(0.045) }
var hairline: Color { darkMode ? .white.opacity(0.08) : .black.opacity(0.05) }
/// Text and symbols on the accent: the primary button, the selected row.
var onAccent: Color { darkMode ? Color(hex: 0x16153A) : .white }
/// What a window sits on in a mockup.
var desk: Color { darkMode ? Color(hex: 0x0E0E10) : Color(hex: 0xDADBE3) }

// MARK: - Components

struct Chip: View {
    let text: String
    let tone: Tone
    var symbol: String?

    var body: some View {
        HStack(spacing: 3) {
            if let symbol { Image(systemName: symbol).font(.system(size: 9, weight: .bold)) }
            Text(text).font(.system(size: 11, weight: .semibold))
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 2.5)
        .foregroundStyle(tone.color)
        .background(tone.color.opacity(0.13), in: Capsule())
    }
}

struct UnseenDot: View {
    var body: some View { Circle().fill(Tone.accent.color).frame(width: 7, height: 7) }
}

struct PrimaryButton: View {
    let title: String
    var symbol: String?
    var body: some View {
        HStack(spacing: 5) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11, weight: .semibold)) }
            Text(title).font(.system(size: 12, weight: .semibold))
        }
        .padding(.horizontal, 12).padding(.vertical, 5)
        .foregroundStyle(onAccent)
        .background(Tone.accent.color, in: Capsule())
    }
}

struct SecondaryButton: View {
    let title: String
    var symbol: String?
    var body: some View {
        HStack(spacing: 5) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11, weight: .medium)) }
            Text(title).font(.system(size: 12, weight: .medium))
        }
        .padding(.horizontal, 11).padding(.vertical, 5)
        .foregroundStyle(ink)
        .background(fill, in: Capsule())
    }
}

struct OverflowButton: View {
    var body: some View {
        Image(systemName: "ellipsis").font(.system(size: 12, weight: .semibold))
            .frame(width: 26, height: 24)
            .foregroundStyle(ink)
            .background(fill, in: Capsule())
    }
}

struct SectionTitle: View {
    let text: String
    var accessory: String?
    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(text).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
            Spacer()
            if let accessory { Text(accessory).font(.system(size: 11)).foregroundStyle(Tone.accent.color) }
        }
    }
}

struct Card<Content: View>: View {
    let title: String
    var accessory: String?
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            SectionTitle(text: title, accessory: accessory)
            content
        }
        .padding(Space.l)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }
}

struct VerdictRow: View {
    enum Verdict { case yes, no, unclear }
    let verdict: Verdict
    let name: String
    let reason: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol).foregroundStyle(tone.color).font(.system(size: 12))
            (Text(name).fontWeight(.medium).foregroundColor(ink) + Text("  " + reason).foregroundColor(secondaryInk))
                .font(.system(size: 12))
        }
    }

    private var symbol: String {
        switch verdict {
        case .yes: "checkmark.circle.fill"
        case .no: "xmark.circle.fill"
        case .unclear: "questionmark.circle.fill"
        }
    }

    private var tone: Tone {
        switch verdict {
        case .yes: .positive
        case .no: .negative
        case .unclear: .caution
        }
    }
}

struct FactRow: View {
    let label: String
    let value: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Text(label).font(.system(size: 12)).foregroundStyle(secondaryInk).frame(width: 90, alignment: .trailing)
            Text(value).font(.system(size: 12)).foregroundStyle(ink)
            Spacer(minLength: 0)
        }
    }
}

struct Evidence: View {
    let text: String
    var body: some View {
        HStack(spacing: Space.s) {
            RoundedRectangle(cornerRadius: 1).fill(separator).frame(width: 2)
            Text("\u{201C}\(text)\u{201D}").font(.system(size: 11)).italic().foregroundStyle(secondaryInk)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}

/// A progress spinner frozen mid-turn; ImageRenderer can't draw ProgressView.
/// A segmented control: the inspector's tabs, or a page's filter.
struct Tabs: View {
    let names: [String]
    let selected: String
    var body: some View {
        HStack(spacing: 0) {
            ForEach(names, id: \.self) { tab in
                Text(tab).font(.system(size: 12, weight: tab == selected ? .semibold : .regular))
                    .foregroundStyle(tab == selected ? ink : secondaryInk)
                    .lineLimit(1)
                    .frame(maxWidth: .infinity).padding(.vertical, 4)
                    .background(tab == selected ? AnyShapeStyle(darkMode ? Color(hex: 0x5A5A60) : .white) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 6))
                    .shadow(color: .black.opacity(tab == selected ? 0.08 : 0), radius: 1, y: 1)
            }
        }
        .padding(2)
        .background(fill, in: RoundedRectangle(cornerRadius: 8))
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

struct Lamp: View {
    let color: Color
    var body: some View { Circle().fill(color).frame(width: 8, height: 8) }
}

/// The app icon: a hub, the owner, joined to a company, a job and a person.
struct HubIcon: View {
    let size: CGFloat
    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: size * 0.225, style: .continuous)
                .fill(LinearGradient(colors: [Color(hex: 0x6461F2), Color(hex: 0x3B37B5)], startPoint: .topLeading, endPoint: .bottomTrailing))
            Canvas { context, canvasSize in
                let center = CGPoint(x: canvasSize.width / 2, y: canvasSize.height * 0.53)
                let reach = canvasSize.width * 0.27
                let angles: [Double] = [-90, 30, 150]
                let points = angles.map { CGPoint(x: center.x + reach * cos($0 * .pi / 180), y: center.y + reach * sin($0 * .pi / 180)) }
                var spokes = Path()
                for point in points {
                    spokes.move(to: center)
                    spokes.addLine(to: point)
                }
                context.stroke(spokes, with: .color(.white.opacity(0.85)), lineWidth: canvasSize.width * 0.045)
                let outer = canvasSize.width * 0.075
                for point in points {
                    context.fill(Path(ellipseIn: CGRect(x: point.x - outer, y: point.y - outer, width: outer * 2, height: outer * 2)), with: .color(.white))
                }
                let inner = canvasSize.width * 0.13
                context.fill(Path(ellipseIn: CGRect(x: center.x - inner, y: center.y - inner, width: inner * 2, height: inner * 2)), with: .color(.white))
                let core = canvasSize.width * 0.055
                context.fill(Path(ellipseIn: CGRect(x: center.x - core, y: center.y - core, width: core * 2, height: core * 2)), with: .color(Color(hex: 0x4B49D6)))
            }
        }
        .frame(width: size, height: size)
        .shadow(color: .black.opacity(0.18), radius: size * 0.04, y: size * 0.02)
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
    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 13)).frame(width: 18)
                .foregroundStyle(isSelected ? onAccent : Tone.accent.color)
            Text(title).font(.system(size: 13)).foregroundStyle(isSelected ? onAccent : ink)
            Spacer()
            if let badge {
                Text("\(badge)").font(.system(size: 11, weight: .semibold)).monospacedDigit()
                    .foregroundStyle(isSelected ? onAccent : secondaryInk)
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
            Text(title).font(.system(size: 11, weight: .semibold)).foregroundStyle(tertiaryInk)
            Spacer()
            Image(systemName: collapsed ? "chevron.right" : "chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(tertiaryInk)
        }
        .padding(.horizontal, Space.s).padding(.top, Space.m).padding(.bottom, 2)
    }
}

struct ProposedSidebar: View {
    var selected = "Today"
    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            TrafficLights().padding(.leading, 6).padding(.top, 4).padding(.bottom, Space.l)
            SidebarItem(title: "Today", symbol: "sun.max", badge: 4, isSelected: selected == "Today")
            SidebarItem(title: "Decide", symbol: "checklist", badge: 7, isSelected: selected == "Decide")
            SidebarItem(title: "Pipeline", symbol: "rectangle.split.3x1", badge: 2, isSelected: selected == "Pipeline")
            SidebarHeader(title: "Browse")
            SidebarItem(title: "Jobs", symbol: "briefcase", isSelected: selected == "Jobs")
            SidebarItem(title: "Companies", symbol: "building.2", isSelected: selected == "Companies")
            SidebarItem(title: "People", symbol: "person.2", isSelected: selected == "People")
            SidebarHeader(title: "You")
            SidebarItem(title: "Profile", symbol: "person.crop.circle", isSelected: selected == "Profile")
            SidebarItem(title: "Criteria", symbol: "slider.horizontal.3", isSelected: selected == "Criteria")
            SidebarHeader(title: "Hub", collapsed: true)
            SidebarHeader(title: "Sessions")
            sessionRow("Northwind", "Waiting for you", Tone.caution.color)
            sessionRow("Senior Product Engineer", "Working", Tone.accent.color)
            Spacer()
        }
        .padding(Space.s)
        .frame(width: 236)
        .frame(maxHeight: .infinity)
        .background(sidebarBackground, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(hairline))
        .padding(Space.s)
    }

    private func sessionRow(_ name: String, _ state: String, _ color: Color) -> some View {
        HStack(spacing: Space.s) {
            Lamp(color: color).frame(width: 18)
            VStack(alignment: .leading, spacing: 0) {
                Text(name).font(.system(size: 12)).foregroundStyle(ink).lineLimit(1)
                Text(state).font(.system(size: 10)).foregroundStyle(secondaryInk)
            }
        }
        .padding(.horizontal, Space.s).padding(.vertical, 3)
    }
}

struct CurrentSidebar: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            TrafficLights().padding(.leading, 6).padding(.top, 4).padding(.bottom, Space.l)
            ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                HStack(spacing: Space.s) {
                    Image(systemName: item.1).font(.system(size: 13)).frame(width: 18).foregroundStyle(item.0 == "Pipeline" ? .white : Color.blue)
                    Text(item.0).font(.system(size: 13)).foregroundStyle(item.0 == "Pipeline" ? .white : ink)
                    Spacer()
                    if item.0 == "Updates" { Text("12").font(.system(size: 11, weight: .semibold)).foregroundStyle(secondaryInk) }
                }
                .padding(.horizontal, Space.s).padding(.vertical, 5)
                .background(item.0 == "Pipeline" ? AnyShapeStyle(Color.blue) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 8))
            }
            Text("Sessions").font(.system(size: 11, weight: .semibold)).foregroundStyle(tertiaryInk).padding(.horizontal, Space.s).padding(.top, Space.m)
            Text("8 most recent…").font(.system(size: 12)).foregroundStyle(secondaryInk).padding(.horizontal, Space.s).padding(.top, 2)
            Spacer()
        }
        .padding(Space.s)
        .frame(width: 236)
        .frame(maxHeight: .infinity)
        .background(sidebarBackground, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(hairline))
        .padding(Space.s)
    }

    private let items: [(String, String)] = [
        ("Decide", "checklist"), ("Pipeline", "rectangle.split.3x1"), ("Updates", "bell"), ("Jobs", "briefcase"),
        ("Companies", "building.2"), ("Recruiters", "person.crop.rectangle.stack"), ("Profile", "person.crop.circle"),
        ("Prompts", "text.bubble"), ("Compare", "square.split.2x1"), ("Runs", "gauge.with.needle"), ("Settings", "gearshape"),
    ]
}

struct Toolbar: View {
    let title: String
    let subtitle: String
    var body: some View {
        HStack(spacing: Space.m) {
            VStack(alignment: .leading, spacing: 0) {
                Text(title).font(.system(size: 15, weight: .semibold)).foregroundStyle(ink)
                Text(subtitle).font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
            Spacer()
            HStack(spacing: Space.s) {
                Image(systemName: "magnifyingglass").font(.system(size: 11)).foregroundStyle(secondaryInk)
                Text("Jump to a job, company or person").font(.system(size: 12)).foregroundStyle(tertiaryInk)
                Spacer()
                Text("⌘K").font(.system(size: 11, weight: .medium)).foregroundStyle(secondaryInk)
                    .padding(.horizontal, 5).padding(.vertical, 1).background(fill, in: RoundedRectangle(cornerRadius: 4))
            }
            .padding(.horizontal, 10).padding(.vertical, 6)
            .frame(width: 300)
            .background(fillSubtle, in: Capsule())
            Image(systemName: "plus").font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                .frame(width: 30, height: 26).background(fillSubtle, in: Capsule())
        }
        .padding(.horizontal, Space.l)
        .frame(height: 52)
    }
}

struct Window<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        content
            .background(windowBackground)
            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Color.black.opacity(0.12)))
            .shadow(color: .black.opacity(0.18), radius: 24, y: 10)
            .padding(36)
            .background(desk)
    }
}

// MARK: - Today

struct DecideRow: View {
    let match: (String, Tone)
    let title: String
    let company: String
    let reason: String
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: Space.s) {
                Chip(text: match.0, tone: match.1)
                Text(title).font(.system(size: 13, weight: .medium)).foregroundStyle(ink).lineLimit(1)
                Spacer(minLength: 0)
            }
            Text(company).font(.system(size: 12)).foregroundStyle(secondaryInk)
            Text(reason).font(.system(size: 12)).foregroundStyle(secondaryInk).lineLimit(2)
            HStack(spacing: Space.s) {
                SecondaryButton(title: "Pursue", symbol: "arrow.up.forward")
                SecondaryButton(title: "Later")
                SecondaryButton(title: "Skip…")
            }
            .padding(.top, 3)
        }
    }
}

struct TodayPage: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Toolbar(title: "Today", subtitle: "Sunday, 4 October")
            VStack(alignment: .leading, spacing: 0) {
                VStack(alignment: .leading, spacing: Space.l) {
                    HStack(spacing: Space.s) {
                        Chip(text: "7 to decide", tone: .accent)
                        Chip(text: "1 overdue follow-up", tone: .negative, symbol: "bell.fill")
                        Chip(text: "1 due today", tone: .caution)
                        Chip(text: "2 replies", tone: .positive, symbol: "arrowshape.turn.up.left.fill")
                    }
                    HStack(alignment: .top, spacing: Space.l) {
                        VStack(spacing: Space.l) {
                            Card(title: "Decide", accessory: "All 7") {
                                DecideRow(match: ("Strong", .positive), title: "Senior Product Engineer", company: "Northwind · Remote, Americas",
                                          reason: "Your payments cases map to their checkout rebuild; pay clears your target.")
                                Divider()
                                DecideRow(match: ("Possible", .accent), title: "Staff Frontend Engineer", company: "Globex · Remote, LATAM",
                                          reason: "Strong React fit; the staff scope asks for platform leadership you show once.")
                            }
                            Card(title: "Recruiters waiting", accessory: "People") {
                                personRow("Jordan Lee", "Talent at Initech · 2 fitting jobs open", "Draft reply")
                                Divider()
                                personRow("Sam Rivera", "Agency · Full-stack, fintech", "Draft reply")
                            }
                        }
                        VStack(spacing: Space.l) {
                            Card(title: "Follow up", accessory: "Pipeline") {
                                followUp("Full-Stack Engineer, Payments", "Initech · Applied", ("Overdue 2 days", .negative))
                                Divider()
                                followUp("Outreach", "Acme Robotics · Applied", ("Due today", .caution))
                            }
                            Card(title: "Updates", accessory: "All updates") {
                                update("Reply from Umbrella Labs", "“Thanks for applying, could you do a call on…”", "09:12", unseen: true)
                                update("Fresh strong match", "Senior Product Engineer at Northwind, posted 2 days ago", "08:00", unseen: true)
                                update("Application confirmed", "Globex received your application", "Yesterday", unseen: false)
                            }
                            Card(title: "Hub", accessory: "Activity") {
                                HStack(spacing: Space.s) {
                                    Spinner()
                                    Text("Reading facts for 3 jobs on the local model").font(.system(size: 12)).foregroundStyle(secondaryInk)
                                }
                            }
                        }
                    }
                }
                .padding(Space.xl)
            }
        }
    }

    private func personRow(_ name: String, _ detail: String, _ action: String) -> some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: Space.s) {
                    Text(name).font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                    Chip(text: "Recruiter", tone: .neutral)
                }
                Text(detail).font(.system(size: 12)).foregroundStyle(secondaryInk)
            }
            Spacer()
            SecondaryButton(title: action, symbol: "square.and.pencil")
        }
    }

    private func followUp(_ title: String, _ detail: String, _ status: (String, Tone)) -> some View {
        HStack {
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                Text(detail).font(.system(size: 12)).foregroundStyle(secondaryInk)
                Chip(text: status.0, tone: status.1, symbol: "bell.fill")
            }
            Spacer()
            SecondaryButton(title: "Followed up…", symbol: "checkmark")
        }
    }

    private func update(_ title: String, _ body: String, _ time: String, unseen: Bool) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            UnseenDot().opacity(unseen ? 1 : 0)
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(title).font(.system(size: 13, weight: unseen ? .semibold : .regular)).foregroundStyle(ink)
                    Spacer()
                    Text(time).font(.system(size: 11)).foregroundStyle(secondaryInk)
                }
                Text(body).font(.system(size: 12)).foregroundStyle(secondaryInk).lineLimit(1)
            }
        }
    }
}

// MARK: - Job inspector

struct JobInspector: View {
    var numbered = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: Space.s) {
                navButton("chevron.left")
                navButton("chevron.right").opacity(0.4)
                Spacer()
                navButton("sidebar.trailing")
            }
            .padding(.horizontal, Space.l)
            .frame(height: 52)
            .overlay(alignment: .leading) { marker(1) }
            VStack(alignment: .leading, spacing: Space.l) {
                VStack(alignment: .leading, spacing: Space.xs) {
                    HStack(spacing: Space.xs) {
                        Text("JOB ·").font(.system(size: 10, weight: .semibold)).foregroundStyle(tertiaryInk)
                        Text("NORTHWIND").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.accent.color)
                    }
                    Text("Senior Product Engineer").font(.system(size: 20, weight: .semibold)).foregroundStyle(ink)
                    Text("Remote, Americas · Posted 2 days ago").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    HStack(spacing: Space.xs) {
                        Chip(text: "Strong match", tone: .positive)
                        Chip(text: "Passes screen", tone: .positive, symbol: "checkmark")
                        Chip(text: "New", tone: .accent)
                    }
                    .padding(.top, 2)
                }
                .overlay(alignment: .topLeading) { marker(2) }
                HStack(spacing: Space.s) {
                    PrimaryButton(title: "Pursue", symbol: "arrow.up.forward")
                    SecondaryButton(title: "Later", symbol: "clock")
                    SecondaryButton(title: "Skip…", symbol: "eye.slash")
                    Spacer()
                    OverflowButton()
                }
                .overlay(alignment: .topLeading) { marker(3) }
                Tabs(names: ["Overview", "Prep", "Posting", "Session"], selected: "Overview")
                    .overlay(alignment: .topLeading) { marker(4) }
                VStack(alignment: .leading, spacing: Space.s) {
                    SectionTitle(text: "Brief", accessory: "by Claude · 1 h ago")
                    Text("Your payments and checkout cases map straight onto their rebuild, and the take-home clears your target. The one gap is GraphQL federation, which they list as a plus.")
                        .font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                    point("plus.circle.fill", .positive, "Led a checkout rewrite that lifted conversion", "Case · Checkout rebuild")
                    point("plus.circle.fill", .positive, "Five years of React and TypeScript in product teams", "Skill · React")
                    point("minus.circle.fill", .caution, "No production GraphQL federation", "Gap · GraphQL")
                }
                .overlay(alignment: .topLeading) { marker(5) }
                Divider()
                VStack(alignment: .leading, spacing: Space.s) {
                    SectionTitle(text: "Screen", accessory: "Criteria")
                    VerdictRow(verdict: .yes, name: "Role", reason: "Product engineer")
                    VerdictRow(verdict: .yes, name: "Where they hire", reason: "Americas")
                    VerdictRow(verdict: .yes, name: "Stack", reason: "React, TypeScript, Node.js")
                    VerdictRow(verdict: .unclear, name: "Years", reason: "Asks 6+, you have 5 in the stack")
                    Evidence(text: "6+ years building web products with React")
                    VerdictRow(verdict: .yes, name: "Pay", reason: "About R$ 31k a month take-home")
                }
                .overlay(alignment: .topLeading) { marker(6) }
                Divider()
                VStack(alignment: .leading, spacing: Space.s) {
                    SectionTitle(text: "People", accessory: "Company")
                    HStack(spacing: Space.s) {
                        Text("Alex Kim").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                        Chip(text: "Connection", tone: .neutral)
                        Text("Engineering Manager").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    }
                    HStack(spacing: Space.s) {
                        Text("Riley Chen").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                        Chip(text: "Introducer", tone: .neutral)
                        Text("Former colleague").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    }
                }
                .overlay(alignment: .topLeading) { marker(7) }
            }
            .padding(.horizontal, Space.l)
            .padding(.bottom, Space.xl)
            Spacer(minLength: 0)
        }
        .frame(width: 440)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(surface, in: RoundedRectangle(cornerRadius: numbered ? Radius.panel : 0))
        .overlay(alignment: .leading) { Rectangle().fill(separator).frame(width: 1).opacity(numbered ? 0 : 1) }
    }

    private func navButton(_ symbol: String) -> some View {
        Image(systemName: symbol).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            .frame(width: 28, height: 24).background(fillSubtle, in: Capsule())
    }

    private func point(_ symbol: String, _ tone: Tone, _ text: String, _ entry: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 12)).foregroundStyle(tone.color)
            VStack(alignment: .leading, spacing: 1) {
                Text(text).font(.system(size: 12)).foregroundStyle(ink)
                Text(entry).font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
        }
    }

    @ViewBuilder
    private func marker(_ number: Int) -> some View {
        if numbered {
            Text("\(number)").font(.system(size: 11, weight: .bold)).foregroundStyle(.white)
                .frame(width: 20, height: 20).background(Color(hex: 0xE5484D), in: Circle())
                .offset(x: number == 1 ? -26 : -42, y: number == 1 ? 0 : -2)
        }
    }
}

struct MacWindowToday: View {
    var body: some View {
        Window {
            HStack(spacing: 0) {
                ProposedSidebar()
                TodayPage().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                JobInspector()
            }
            .frame(width: 1440, height: 900)
        }
    }
}

struct InspectorAnatomy: View {
    private let legend = [
        ("History", "Back and forward through what the inspector showed. Links inside it push here instead of switching pages."),
        ("Header", "Kind and parent as an eyebrow link, title, one line of facts, and at most three status chips."),
        ("Action bar", "One primary action for the entity's state, up to two secondary ones, the rest in the overflow menu."),
        ("Tabs", "The same tabs for an entity everywhere: Overview, Prep, Posting, Session for a job."),
        ("Brief", "Decision content first: the match and why, with the knowledge-base entries behind each point."),
        ("Screen", "Your criteria checks and the posting's screen-out questions in one list of verdict rows, evidence quoted."),
        ("People", "Everyone tied to the company, each with a relation chip; a name opens the person here."),
    ]

    var body: some View {
        HStack(alignment: .top, spacing: 48) {
            JobInspector(numbered: true)
                .frame(height: 900)
                .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
            VStack(alignment: .leading, spacing: Space.l) {
                Text("One inspector for every job, company and person").font(.system(size: 22, weight: .semibold)).foregroundStyle(ink)
                ForEach(Array(legend.enumerated()), id: \.offset) { index, item in
                    HStack(alignment: .firstTextBaseline, spacing: Space.m) {
                        Text("\(index + 1)").font(.system(size: 11, weight: .bold)).foregroundStyle(.white)
                            .frame(width: 20, height: 20).background(Color(hex: 0xE5484D), in: Circle())
                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.0).font(.system(size: 14, weight: .semibold)).foregroundStyle(ink)
                            Text(item.1).font(.system(size: 13)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }
            }
            .frame(width: 420)
            .padding(.top, Space.xl)
        }
        .padding(48)
        .background(windowBackground)
    }
}

// MARK: - Sidebar before and after

struct SidebarComparison: View {
    var body: some View {
        HStack(alignment: .top, spacing: 64) {
            VStack(alignment: .leading, spacing: Space.m) {
                Text("Today: 11 equal pages").font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
                Text("Daily work, browsing, tuning the hub and settings share one flat list.").font(.system(size: 13)).foregroundStyle(secondaryInk)
                CurrentSidebar().frame(height: 640)
            }
            .frame(width: 280)
            VStack(alignment: .leading, spacing: Space.m) {
                Text("Proposed: grouped by intent").font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
                Text("What needs you, what you browse, who you are, and the hub's tools, collapsed.").font(.system(size: 13)).foregroundStyle(secondaryInk)
                ProposedSidebar().frame(height: 640)
            }
            .frame(width: 280)
        }
        .padding(48)
        .background(windowBackground)
    }
}

// MARK: - Design board

struct DesignBoard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 40) {
            HStack(alignment: .center, spacing: Space.xl) {
                HubIcon(size: 112)
                VStack(alignment: .leading, spacing: Space.xs) {
                    Text("Job Search Hub").font(.system(size: 34, weight: .bold)).foregroundStyle(ink)
                    Text("Calm, dense and honest. One accent for you, four tones for state, everything else neutral.")
                        .font(.system(size: 15)).foregroundStyle(secondaryInk)
                }
                Spacer()
                HStack(alignment: .bottom, spacing: Space.l) {
                    HubIcon(size: 64)
                    HubIcon(size: 32)
                    HubIcon(size: 16)
                }
            }

            board("Tones", subtitle: "Every color means one thing on both clients. Chips tint the tone at 13%; text and icons use it solid.") {
                HStack(alignment: .top, spacing: Space.l) {
                    ForEach(Tone.allCases, id: \.self) { tone in
                        VStack(alignment: .leading, spacing: Space.s) {
                            HStack(spacing: 0) {
                                Rectangle().fill(tone.light)
                                Rectangle().fill(tone.dark)
                            }
                            .frame(height: 56)
                            .clipShape(RoundedRectangle(cornerRadius: Radius.card))
                            Text(tone.name).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                            Text("\(tone.hexText.0) light · \(tone.hexText.1) dark").font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk)
                            Text(tone.usage).font(.system(size: 12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
                        }
                        .frame(width: 236, alignment: .leading)
                    }
                }
            }

            HStack(alignment: .top, spacing: 40) {
                board("Type", subtitle: "SF Pro on the Mac, Roboto on Android; the roles match.") {
                    VStack(alignment: .leading, spacing: Space.m) {
                        typeRow("Page", "Navigation title, 15 semibold", .system(size: 15, weight: .semibold))
                        typeRow("Entity", "Senior Product Engineer, 20 semibold", .system(size: 20, weight: .semibold))
                        typeRow("Section", "Brief, 13 semibold", .system(size: 13, weight: .semibold))
                        typeRow("Body", "The posting's words, 13 regular", .system(size: 13))
                        typeRow("Secondary", "Northwind · Remote, 12 secondary", .system(size: 12))
                        typeRow("Caption", "by Claude · 1 h ago, 11", .system(size: 11))
                    }
                }
                .frame(width: 560, alignment: .leading)
                board("Space and shape", subtitle: "A 4-point scale and three radii; nothing in between.") {
                    VStack(alignment: .leading, spacing: Space.l) {
                        HStack(alignment: .bottom, spacing: Space.l) {
                            ForEach([("xs", Space.xs), ("s", Space.s), ("m", Space.m), ("l", Space.l), ("xl", Space.xl), ("xxl", Space.xxl)], id: \.0) { name, value in
                                VStack(spacing: Space.xs) {
                                    Rectangle().fill(Tone.accent.color.opacity(0.25)).frame(width: value, height: value)
                                    Text("\(name) \(Int(value))").font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk)
                                }
                            }
                        }
                        HStack(spacing: Space.l) {
                            ForEach([("control", Radius.control), ("card", Radius.card), ("panel", Radius.panel)], id: \.0) { name, value in
                                VStack(spacing: Space.xs) {
                                    RoundedRectangle(cornerRadius: value).strokeBorder(Tone.accent.color, lineWidth: 1.5).frame(width: 72, height: 44)
                                    Text("\(name) \(Int(value))").font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk)
                                }
                            }
                        }
                    }
                }
            }

            board("Chips", subtitle: "One component for every status word. Never more than three on a row.") {
                VStack(alignment: .leading, spacing: Space.m) {
                    chipRow("Match", [("Strong", .positive), ("Possible", .accent), ("Stretch", .caution), ("Mismatch", .neutral)])
                    chipRow("Screen", [("Passes", .positive), ("Unclear", .caution), ("Fails", .negative)])
                    chipRow("Follow-up", [("Overdue 2 days", .negative), ("Due today", .caution), ("Due in 3 days", .neutral)])
                    chipRow("State", [("New", .accent), ("Applied", .neutral), ("Heard back", .positive), ("Skipped", .neutral), ("Closed", .neutral)])
                    chipRow("Relation", [("Contact", .neutral), ("Connection", .neutral), ("Introducer", .neutral), ("Recruiter", .neutral), ("Agency", .neutral)])
                }
            }

            HStack(alignment: .top, spacing: 40) {
                board("Actions", subtitle: "One primary per view; secondary in capsules; the rest in the overflow.") {
                    VStack(alignment: .leading, spacing: Space.m) {
                        HStack(spacing: Space.s) {
                            PrimaryButton(title: "Pursue", symbol: "arrow.up.forward")
                            SecondaryButton(title: "Later", symbol: "clock")
                            SecondaryButton(title: "Skip…", symbol: "eye.slash")
                            OverflowButton()
                        }
                        HStack(spacing: Space.s) {
                            HStack(spacing: 6) {
                                Spinner()
                                Text("Writing brief…").font(.system(size: 12, weight: .medium)).foregroundStyle(secondaryInk)
                            }
                            .padding(.horizontal, 11).padding(.vertical, 5)
                            .background(fillSubtle, in: Capsule())
                            Text("Async button: label, spinner, disabled while it runs").font(.system(size: 11)).foregroundStyle(secondaryInk)
                        }
                    }
                }
                .frame(width: 560, alignment: .leading)
                board("Verdicts and facts", subtitle: "Fit checks, screen-out answers and brief points share one row.") {
                    VStack(alignment: .leading, spacing: Space.s) {
                        VerdictRow(verdict: .yes, name: "Where they hire", reason: "Americas")
                        VerdictRow(verdict: .unclear, name: "Years", reason: "Asks 6+, you have 5 in the stack")
                        Evidence(text: "6+ years building web products with React")
                        VerdictRow(verdict: .no, name: "Timezone", reason: "Asks for APAC hours")
                        Divider().padding(.vertical, 2)
                        FactRow(label: "Pay", value: "US$ 120k–150k a year")
                        FactRow(label: "Workplace", value: "Remote")
                    }
                }
            }

            HStack(alignment: .top, spacing: 40) {
                board("Feedback", subtitle: "Errors say what failed and what to do; the raw error folds away.") {
                    VStack(alignment: .leading, spacing: Space.m) {
                        HStack(alignment: .top, spacing: Space.m) {
                            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Tone.negative.color)
                            VStack(alignment: .leading, spacing: 3) {
                                Text("Couldn't record the follow-up").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                                Text("The hub didn't answer. It's probably restarting; try again in a moment.").font(.system(size: 12)).foregroundStyle(secondaryInk)
                                HStack(spacing: Space.xs) {
                                    Image(systemName: "chevron.right").font(.system(size: 9, weight: .semibold))
                                    Text("Details").font(.system(size: 11))
                                }
                                .foregroundStyle(secondaryInk)
                                .padding(.top, 2)
                            }
                            Spacer()
                            SecondaryButton(title: "Try again")
                        }
                        .padding(Space.m)
                        .background(Tone.negative.color.opacity(0.06), in: RoundedRectangle(cornerRadius: Radius.card))
                        HStack(spacing: Space.s) {
                            Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.positive.color)
                            Text("Pursued 3 jobs").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                            Text("Undo").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.color)
                        }
                        .padding(.horizontal, 14).padding(.vertical, 8)
                        .background(surface, in: Capsule())
                        .overlay(Capsule().strokeBorder(separator))
                        .shadow(color: .black.opacity(0.08), radius: 6, y: 2)
                    }
                }
                .frame(width: 560, alignment: .leading)
                board("Connection", subtitle: "Shown once, above every page, instead of each page's own empty state.") {
                    HStack(spacing: Space.s) {
                        Image(systemName: "network.slash").foregroundStyle(Tone.caution.color)
                        Text("Can't reach the hub at localhost:8090").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                        Spacer()
                        Text("Start server").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.color)
                        Text("Settings…").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.color)
                    }
                    .padding(.horizontal, Space.m).padding(.vertical, Space.s)
                    .background(Tone.caution.color.opacity(0.1), in: RoundedRectangle(cornerRadius: Radius.card))
                }
            }
        }
        .padding(56)
        .frame(width: 1400, alignment: .leading)
        .background(windowBackground)
    }

    private func board(_ title: String, subtitle: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text(title).font(.system(size: 20, weight: .semibold)).foregroundStyle(ink)
            Text(subtitle).font(.system(size: 13)).foregroundStyle(secondaryInk)
            content().padding(.top, Space.xs)
        }
    }

    private func typeRow(_ role: String, _ sample: String, _ font: Font) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.l) {
            Text(role).font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk).frame(width: 80, alignment: .leading)
            Text(sample).font(font).foregroundStyle(ink)
        }
    }

    private func chipRow(_ title: String, _ chips: [(String, Tone)]) -> some View {
        HStack(spacing: Space.s) {
            Text(title).font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk).frame(width: 80, alignment: .leading)
            ForEach(Array(chips.enumerated()), id: \.offset) { _, chip in Chip(text: chip.0, tone: chip.1) }
        }
    }
}

// MARK: - Android: Material 3 Expressive

/// Android icons are Material Symbols Rounded, drawn from the variable font at MATERIAL_SYMBOLS_FONT
/// (`variablefont/` in github.com/google/material-design-icons). Without it, SF Symbols stand in.
nonisolated(unsafe) var materialSymbolsLoaded = false

func loadMaterialSymbols() {
    guard let path = ProcessInfo.processInfo.environment["MATERIAL_SYMBOLS_FONT"] else {
        FileHandle.standardError.write("MATERIAL_SYMBOLS_FONT is unset: Android icons fall back to SF Symbols\n".data(using: .utf8)!)
        return
    }
    materialSymbolsLoaded = CTFontManagerRegisterFontsForURL(URL(filePath: path) as CFURL, .process, nil)
}

/// A Material Symbol by its name, outlined or filled.
struct MSymbol: View {
    let name: String
    var size: CGFloat = 24
    var fill = false

    init(_ name: String, size: CGFloat = 24, fill: Bool = false) {
        self.name = name
        self.size = size
        self.fill = fill
    }

    var body: some View {
        Group {
            if materialSymbolsLoaded {
                Text(name).font(Font(font)).fixedSize()
            } else {
                Image(systemName: Self.fallback[name] ?? "circle").font(.system(size: size * 0.75))
            }
        }
        .frame(width: size, height: size)
    }

    private var font: CTFont {
        let axes: [Int: CGFloat] = [0x4649_4C4C: fill ? 1 : 0, 0x7767_6874: 400, 0x6F70_737A: min(max(size, 20), 48)]
        let attributes: [CFString: Any] = [kCTFontNameAttribute: "MaterialSymbolsRounded-Regular", kCTFontVariationAttribute: axes]
        return CTFontCreateWithFontDescriptor(CTFontDescriptorCreateWithAttributes(attributes as CFDictionary), size, nil)
    }

    private static let fallback = [
        "home": "house", "fact_check": "checklist", "view_kanban": "rectangle.split.3x1", "work": "briefcase",
        "search": "magnifyingglass", "hub": "circle.hexagongrid", "more_vert": "ellipsis", "arrow_back": "arrow.left",
        "open_in_new": "arrow.up.right.square", "check": "checkmark", "check_circle": "checkmark.circle", "help": "questionmark.circle",
        "remove_circle": "minus.circle", "cancel": "xmark.circle", "schedule": "clock", "close": "xmark", "notifications": "bell",
        "computer": "desktopcomputer", "sync": "arrow.triangle.2.circlepath", "link_off": "link", "palette": "paintpalette",
        "filter_list": "line.3.horizontal.decrease", "expand_more": "chevron.down", "share": "square.and.arrow.up", "build": "wrench",
        "wifi": "wifi", "signal_cellular_4_bar": "cellularbars", "battery_full": "battery.100", "menu": "line.3.horizontal",
        "add": "plus", "mail": "envelope", "person": "person", "undo": "arrow.uturn.backward", "dark_mode": "moon",
        "event": "calendar", "snooze": "moon.zzz", "bluetooth": "dot.radiowaves.left.and.right", "flashlight_on": "flashlight.on.fill",
        "do_not_disturb_on": "minus.circle", "settings": "gear", "edit_note": "square.and.pencil", "group": "person.2",
        "drag_handle": "line.3.horizontal", "chevron_right": "chevron.right", "keyboard_arrow_up": "chevron.up",
    ]
}

/// Hub Indigo as a Material 3 color scheme: the fidelity variant keeps the seed as primary.
enum M3 {
    static var primary: Color { darkMode ? Color(hex: 0xC3C0FF) : Color(hex: 0x4B49D6) }
    static var onPrimary: Color { darkMode ? Color(hex: 0x1F1A8C) : .white }
    static var primaryContainer: Color { darkMode ? Color(hex: 0x3533BE) : Color(hex: 0xE2DFFF) }
    static var onPrimaryContainer: Color { darkMode ? Color(hex: 0xE2DFFF) : Color(hex: 0x100069) }
    static var secondaryContainer: Color { darkMode ? Color(hex: 0x464559) : Color(hex: 0xE3E0F9) }
    static var onSecondaryContainer: Color { darkMode ? Color(hex: 0xE3E0F9) : Color(hex: 0x1A1A2C) }
    static var tertiaryContainer: Color { darkMode ? Color(hex: 0x633B48) : Color(hex: 0xFFD8E4) }
    /// The page: the surface container lists and cards sit on.
    static var page: Color { darkMode ? Color(hex: 0x131318) : Color(hex: 0xF0ECF6) }
    /// A list item, card or group on the page.
    static var item: Color { darkMode ? Color(hex: 0x2A292F) : .white }
    static var containerHigh: Color { darkMode ? Color(hex: 0x2A292F) : Color(hex: 0xEAE7F1) }
    static var containerHighest: Color { darkMode ? Color(hex: 0x35343A) : Color(hex: 0xE4E1EB) }
    static var containerLow: Color { darkMode ? Color(hex: 0x1B1B21) : Color(hex: 0xF6F2FC) }
    static var onSurface: Color { darkMode ? Color(hex: 0xE4E1E9) : Color(hex: 0x1B1B21) }
    static var onSurfaceVariant: Color { darkMode ? Color(hex: 0xC8C5D0) : Color(hex: 0x47464F) }
    static var outline: Color { darkMode ? Color(hex: 0x928F9A) : Color(hex: 0x787680) }
    static var outlineVariant: Color { darkMode ? Color(hex: 0x47464F) : Color(hex: 0xC8C5D0) }
    static var inverseSurface: Color { darkMode ? Color(hex: 0xE4E1E9) : Color(hex: 0x303036) }
    static var inverseOnSurface: Color { darkMode ? Color(hex: 0x303036) : Color(hex: 0xF3EFF7) }
    static var inversePrimary: Color { darkMode ? Color(hex: 0x4B49D6) : Color(hex: 0xC3C0FF) }
    static var error: Color { darkMode ? Color(hex: 0xFFB4AB) : Color(hex: 0xBA1A1A) }
    static let scrim = Color.black.opacity(0.32)
}

/// The tones as Material 3 custom colors, harmonized toward the seed, each with a container role.
extension Tone {
    var m3: Color {
        switch self {
        case .accent: M3.primary
        case .positive: darkMode ? Color(hex: 0x8DD7A3) : Color(hex: 0x1B6D3F)
        case .caution: darkMode ? Color(hex: 0xFFB86E) : Color(hex: 0x8A5100)
        case .negative: M3.error
        case .neutral: M3.onSurfaceVariant
        }
    }

    var m3Container: Color {
        switch self {
        case .accent: M3.primaryContainer
        case .positive: darkMode ? Color(hex: 0x00522D) : Color(hex: 0xB4F1C6)
        case .caution: darkMode ? Color(hex: 0x693C00) : Color(hex: 0xFFDCBE)
        case .negative: darkMode ? Color(hex: 0x93000A) : Color(hex: 0xFFDAD6)
        case .neutral: M3.containerHighest
        }
    }

    var m3OnContainer: Color {
        switch self {
        case .accent: M3.onPrimaryContainer
        case .positive: darkMode ? Color(hex: 0xB4F1C6) : Color(hex: 0x00210E)
        case .caution: darkMode ? Color(hex: 0xFFDCBE) : Color(hex: 0x2C1600)
        case .negative: darkMode ? Color(hex: 0xFFDAD6) : Color(hex: 0x410002)
        case .neutral: M3.onSurfaceVariant
        }
    }
}

/// The Material 3 type scale in Roboto.
enum M3Type {
    static func roboto(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font { .custom("Roboto", size: size).weight(weight) }
    static var displaySmall: Font { roboto(36) }
    static var headlineLarge: Font { roboto(32) }
    static var headlineMedium: Font { roboto(28) }
    static var headlineSmall: Font { roboto(24) }
    static var titleLarge: Font { roboto(22) }
    static var titleMedium: Font { roboto(16, .medium) }
    static var titleSmall: Font { roboto(14, .medium) }
    static var bodyLarge: Font { roboto(16) }
    static var bodyMedium: Font { roboto(14) }
    static var bodySmall: Font { roboto(12) }
    static var labelLarge: Font { roboto(14, .medium) }
    static var labelMedium: Font { roboto(12, .medium) }
    static var labelSmall: Font { roboto(11, .medium) }
}

/// The Material 3 shape scale.
enum M3Shape {
    static let extraSmall: CGFloat = 4
    static let small: CGFloat = 8
    static let medium: CGFloat = 12
    static let large: CGFloat = 16
    static let extraLarge: CGFloat = 28
}

// MARK: Device

struct AndroidStatusBar: View {
    var color: Color = M3.onSurface
    var body: some View {
        HStack(spacing: 2) {
            Text("10:24").font(M3Type.roboto(14, .medium))
            Spacer()
            MSymbol("wifi", size: 16, fill: true)
            MSymbol("signal_cellular_4_bar", size: 16, fill: true)
            MSymbol("battery_full", size: 16, fill: true)
        }
        .foregroundStyle(color)
        .padding(.horizontal, 20)
        .frame(height: 36)
        .overlay { Circle().fill(.black).frame(width: 12, height: 12) }
    }
}

/// The gesture navigation handle at the bottom of an edge-to-edge screen.
struct GestureHandle: View {
    var body: some View {
        Capsule().fill(M3.onSurface.opacity(0.85)).frame(width: 108, height: 4).frame(maxWidth: .infinity).frame(height: 24)
    }
}

/// A phone in the Galaxy and Pixel mould: punch-hole camera, thin even bezel, gesture navigation.
struct AndroidPhone<Content: View>: View {
    /// The selected destination in the navigation bar; nil for a screen without one.
    var nav: String?
    var background: Color?
    var width: CGFloat = 360
    var height: CGFloat = 780
    @ViewBuilder let content: Content

    var body: some View {
        VStack(spacing: 0) {
            AndroidStatusBar()
            VStack(spacing: 0) { content }.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).clipped()
            if let nav {
                M3NavigationBar(selected: nav)
            } else {
                GestureHandle()
            }
        }
        .frame(width: width, height: height)
        .background(background ?? M3.page)
        .clipShape(RoundedRectangle(cornerRadius: 34, style: .continuous))
        .padding(6)
        .background(Color(hex: 0x1F1F22), in: RoundedRectangle(cornerRadius: 40, style: .continuous))
    }
}

// MARK: Components

struct M3NavigationBar: View {
    let selected: String
    var handle = true
    static let destinations = [("Today", "home"), ("Decide", "fact_check"), ("Pipeline", "view_kanban"), ("Jobs", "work")]

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                ForEach(Self.destinations, id: \.0) { item in
                    NavigationItem(title: item.0, icon: item.1, selected: item.0 == selected, badge: Self.badge(item.0))
                        .frame(maxWidth: .infinity)
                }
            }
            .padding(.top, 12)
            .padding(.bottom, handle ? 0 : 16)
            if handle { GestureHandle() }
        }
        .background(M3.containerLow)
    }

    static func badge(_ title: String) -> String? {
        switch title {
        case "Decide": "7"
        case "Pipeline": ""
        default: nil
        }
    }
}

/// A navigation bar or rail destination: the pill indicator around a filled icon when selected.
struct NavigationItem: View {
    let title: String
    let icon: String
    let selected: Bool
    var badge: String?

    var body: some View {
        VStack(spacing: 4) {
            MSymbol(icon, fill: selected)
                .foregroundStyle(selected ? M3.onSecondaryContainer : M3.onSurfaceVariant)
                .frame(width: 56, height: 32)
                .background(selected ? M3.secondaryContainer : .clear, in: Capsule())
                .overlay(alignment: .topTrailing) {
                    if let badge { M3Badge(text: badge).offset(x: badge.isEmpty ? -14 : -8, y: badge.isEmpty ? 4 : 0) }
                }
            Text(title).font(M3Type.labelMedium).foregroundStyle(selected ? M3.onSurface : M3.onSurfaceVariant)
        }
    }
}

/// A small dot, or a large badge with a count.
struct M3Badge: View {
    let text: String
    var body: some View {
        if text.isEmpty {
            Circle().fill(M3.error).frame(width: 6, height: 6)
        } else {
            Text(text).font(M3Type.labelSmall).foregroundStyle(darkMode ? Color(hex: 0x690005) : .white)
                .frame(minWidth: 16, minHeight: 16).padding(.horizontal, 2).background(M3.error, in: Capsule())
        }
    }
}

/// An icon button: 48 touch target, 24 icon.
struct M3IconButton: View {
    let icon: String
    var style: Style = .standard
    enum Style { case standard, tonal, filled }

    init(_ icon: String, style: Style = .standard) {
        self.icon = icon
        self.style = style
    }

    var body: some View {
        MSymbol(icon, fill: style == .filled)
            .foregroundStyle(foreground)
            .frame(width: 40, height: 40)
            .background(background, in: Circle())
            .frame(width: 48, height: 48)
    }

    private var foreground: Color {
        switch style {
        case .standard: M3.onSurfaceVariant
        case .tonal: M3.onSecondaryContainer
        case .filled: M3.onPrimary
        }
    }

    private var background: Color {
        switch style {
        case .standard: .clear
        case .tonal: M3.secondaryContainer
        case .filled: M3.primary
        }
    }
}

/// The hub's connection avatar at the end of a top bar: it opens the hub, Settings and Unpair.
struct HubAvatar: View {
    var body: some View {
        MSymbol("hub", size: 20, fill: true).foregroundStyle(M3.onPrimaryContainer)
            .frame(width: 32, height: 32).background(M3.primaryContainer, in: Circle())
            .overlay(alignment: .bottomTrailing) {
                Circle().fill(Tone.positive.m3).frame(width: 10, height: 10).overlay(Circle().strokeBorder(M3.page, lineWidth: 2))
            }
            .frame(width: 48, height: 48)
    }
}

/// Small, medium flexible and large flexible top app bars.
struct M3TopBar<Trailing: View>: View {
    enum Size { case small, medium, large }
    let title: String
    var subtitle: String?
    var size: Size = .small
    var back = false
    @ViewBuilder var trailing: Trailing

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                if back { M3IconButton("arrow_back").foregroundStyle(M3.onSurface) }
                if size == .small {
                    Text(title).font(M3Type.titleLarge).foregroundStyle(M3.onSurface).padding(.leading, back ? 4 : 16).lineLimit(1)
                }
                Spacer(minLength: 0)
                trailing
            }
            .padding(.horizontal, 4)
            .frame(height: 64)
            if size != .small {
                VStack(alignment: .leading, spacing: 4) {
                    Text(title).font(size == .large ? M3Type.headlineLarge : M3Type.headlineMedium).foregroundStyle(M3.onSurface)
                        .fixedSize(horizontal: false, vertical: true)
                    if let subtitle { Text(subtitle).font(M3Type.titleMedium.weight(.regular)).foregroundStyle(M3.onSurfaceVariant) }
                }
                .padding(.horizontal, 16)
                .padding(.top, size == .large ? 24 : 0)
                .padding(.bottom, 16)
            }
        }
    }
}

extension M3TopBar where Trailing == EmptyView {
    init(title: String, subtitle: String? = nil, size: Size = .small, back: Bool = false) {
        self.init(title: title, subtitle: subtitle, size: size, back: back) { EmptyView() }
    }
}

/// A status word as a Material 3 label: the tone's container with 8 dp corners.
struct M3Label: View {
    let text: String
    let tone: Tone
    var icon: String?
    var body: some View {
        HStack(spacing: 4) {
            if let icon { MSymbol(icon, size: 16, fill: true) }
            Text(text).font(M3Type.labelMedium).lineLimit(1).fixedSize()
        }
        .foregroundStyle(tone.m3OnContainer)
        .padding(.leading, icon == nil ? 8 : 6).padding(.trailing, 8)
        .frame(height: 24)
        .background(tone.m3Container, in: RoundedRectangle(cornerRadius: M3Shape.small))
    }
}

/// A company's monogram in a tonal circle, the leading element of a list item.
struct Monogram: View {
    let letter: String
    var tone: Tone = .accent
    var size: CGFloat = 40
    var body: some View {
        Text(letter).font(M3Type.roboto(size * 0.4, .medium)).foregroundStyle(tone.m3OnContainer)
            .frame(width: size, height: size).background(tone.m3Container, in: Circle())
    }
}

/// A list item: leading element, headline, supporting text and a trailing element.
struct M3ListItem<Leading: View, Trailing: View>: View {
    let headline: String
    var supporting: String?
    var supportingColor: Color = M3.onSurfaceVariant
    var headlineColor: Color = M3.onSurface
    /// A status label that leads the supporting line.
    var label: (String, Tone)?
    @ViewBuilder var leading: Leading
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(spacing: 16) {
            leading
            VStack(alignment: .leading, spacing: label == nil ? 2 : 4) {
                Text(headline).font(M3Type.bodyLarge).foregroundStyle(headlineColor).lineLimit(1)
                HStack(spacing: 8) {
                    if let label { M3Label(text: label.0, tone: label.1) }
                    if let supporting { Text(supporting).font(M3Type.bodyMedium).foregroundStyle(supportingColor).lineLimit(1) }
                }
            }
            Spacer(minLength: 0)
            trailing
        }
        .padding(.leading, 16).padding(.trailing, 16)
        .frame(minHeight: supporting == nil && label == nil ? 56 : (label == nil ? 72 : 76))
    }
}

/// Groups list items the Material 3 Expressive way: large outer corners, small inner ones, 2 dp apart.
struct SegmentedGroup: View {
    let items: [AnyView]
    var body: some View {
        VStack(spacing: 2) {
            ForEach(items.indices, id: \.self) { index in
                items[index]
                    .background(M3.item)
                    .clipShape(UnevenRoundedRectangle(
                        topLeadingRadius: index == 0 ? 20 : 4, bottomLeadingRadius: index == items.count - 1 ? 20 : 4,
                        bottomTrailingRadius: index == items.count - 1 ? 20 : 4, topTrailingRadius: index == 0 ? 20 : 4))
            }
        }
        .padding(.horizontal, 12)
    }
}

/// A group's heading: a label in primary, with an optional text button at the end.
struct GroupHeading: View {
    let title: String
    var action: String?
    var body: some View {
        HStack {
            Text(title).font(M3Type.titleSmall).foregroundStyle(M3.primary)
            Spacer()
            if let action { Text(action).font(M3Type.labelLarge).foregroundStyle(M3.primary) }
        }
        .padding(.horizontal, 28)
        .frame(height: 40)
    }
}

/// Common buttons, at the small (40) or medium (56) size; Expressive buttons are round.
struct M3Button: View {
    enum Style { case filled, tonal, outlined, text }
    let title: String
    var icon: String?
    var style: Style = .filled
    var medium = false

    var body: some View {
        HStack(spacing: 8) {
            if let icon { MSymbol(icon, size: medium ? 24 : 18) }
            Text(title).font(medium ? M3Type.titleMedium : M3Type.labelLarge).lineLimit(1).fixedSize()
        }
        .foregroundStyle(foreground)
        .padding(.horizontal, style == .text ? 12 : (medium ? 24 : 16))
        .frame(height: medium ? 56 : 40)
        .background(background, in: Capsule())
        .overlay { if style == .outlined { Capsule().strokeBorder(M3.outlineVariant) } }
    }

    private var foreground: Color {
        switch style {
        case .filled: M3.onPrimary
        case .tonal: M3.onSecondaryContainer
        case .outlined: M3.onSurfaceVariant
        case .text: M3.primary
        }
    }

    private var background: Color {
        switch style {
        case .filled: M3.primary
        case .tonal: M3.secondaryContainer
        case .outlined, .text: .clear
        }
    }
}

struct M3Switch: View {
    let isOn: Bool
    var body: some View {
        Capsule().fill(isOn ? M3.primary : M3.containerHighest)
            .overlay { if !isOn { Capsule().strokeBorder(M3.outline, lineWidth: 2) } }
            .frame(width: 52, height: 32)
            .overlay(alignment: isOn ? .trailing : .leading) {
                if isOn {
                    Circle().fill(M3.onPrimary).frame(width: 24, height: 24)
                        .overlay { MSymbol("check", size: 16).foregroundStyle(M3.primary) }
                        .padding(4)
                } else {
                    Circle().fill(M3.outline).frame(width: 16, height: 16).padding(8)
                }
            }
    }
}

/// A filter chip: 8 dp corners, a check when selected.
struct M3FilterChip: View {
    let title: String
    var selected = false
    var body: some View {
        HStack(spacing: 8) {
            if selected { MSymbol("check", size: 18) }
            Text(title).font(M3Type.labelLarge).lineLimit(1).fixedSize()
        }
        .foregroundStyle(selected ? M3.onSecondaryContainer : M3.onSurfaceVariant)
        .padding(.leading, selected ? 8 : 16).padding(.trailing, 16)
        .frame(height: 32)
        .background(selected ? M3.secondaryContainer : .clear, in: RoundedRectangle(cornerRadius: M3Shape.small))
        .overlay { if !selected { RoundedRectangle(cornerRadius: M3Shape.small).strokeBorder(M3.outlineVariant) } }
    }
}

/// An assist chip with a leading monogram: here, the link from a job to its company.
struct M3AssistChip: View {
    let title: String
    var letter: String?
    var icon: String?
    var body: some View {
        HStack(spacing: 8) {
            if let letter { Monogram(letter: letter, size: 24) }
            if let icon { MSymbol(icon, size: 18).foregroundStyle(M3.primary) }
            Text(title).font(M3Type.labelLarge).foregroundStyle(M3.onSurface).lineLimit(1).fixedSize()
        }
        .padding(.leading, letter == nil ? 8 : 4).padding(.trailing, 12)
        .frame(height: 32)
        .overlay(RoundedRectangle(cornerRadius: M3Shape.small).strokeBorder(M3.outlineVariant))
    }
}

/// Primary tabs: label and a 3 dp indicator under the selected one, over a divider.
struct M3Tabs: View {
    let tabs: [String]
    let selected: String
    var scrollable = false
    var body: some View {
        HStack(spacing: scrollable ? 24 : 0) {
            ForEach(tabs, id: \.self) { tab in
                let isSelected = tab == selected
                Text(tab).font(M3Type.titleSmall).foregroundStyle(isSelected ? M3.primary : M3.onSurfaceVariant)
                    .lineLimit(1).fixedSize()
                    .frame(height: 48)
                    .overlay(alignment: .bottom) {
                        if isSelected {
                            UnevenRoundedRectangle(topLeadingRadius: 3, topTrailingRadius: 3).fill(M3.primary).frame(height: 3)
                        }
                    }
                    .frame(maxWidth: scrollable ? nil : .infinity)
            }
            if scrollable { Spacer(minLength: 0) }
        }
        .padding(.horizontal, scrollable ? 16 : 0)
        .overlay(alignment: .bottom) { Rectangle().fill(M3.outlineVariant).frame(height: 1) }
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipped()
    }
}

struct M3Snackbar: View {
    let text: String
    var action: String?
    var body: some View {
        HStack {
            Text(text).font(M3Type.bodyMedium).foregroundStyle(M3.inverseOnSurface).lineLimit(1)
            Spacer()
            if let action { Text(action).font(M3Type.labelLarge).foregroundStyle(M3.inversePrimary) }
        }
        .padding(.leading, 16).padding(.trailing, 12)
        .frame(height: 48)
        .background(M3.inverseSurface, in: RoundedRectangle(cornerRadius: M3Shape.extraSmall))
        .shadow(color: .black.opacity(0.2), radius: 4, y: 2)
    }
}

/// A basic dialog: optional hero icon, headline, text, and text buttons at the end.
struct M3Dialog: View {
    var icon: String?
    let headline: String
    let text: String
    let dismiss: String
    let confirm: String
    var destructive = false

    var body: some View {
        VStack(alignment: icon == nil ? .leading : .center, spacing: 16) {
            if let icon { MSymbol(icon).foregroundStyle(destructive ? M3.error : M3.primary) }
            Text(headline).font(M3Type.headlineSmall).foregroundStyle(M3.onSurface)
            Text(text).font(M3Type.bodyMedium).foregroundStyle(M3.onSurfaceVariant)
                .multilineTextAlignment(icon == nil ? .leading : .center).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Spacer()
                M3Button(title: dismiss, style: .text)
                Text(confirm).font(M3Type.labelLarge).foregroundStyle(destructive ? M3.error : M3.primary).padding(.horizontal, 12).frame(height: 40)
            }
            .padding(.top, 8)
        }
        .padding(24)
        .frame(width: 312)
        .background(M3.containerHigh, in: RoundedRectangle(cornerRadius: M3Shape.extraLarge))
    }
}

/// A docked search bar: full corners, a leading search icon and the hub avatar at the end.
struct M3SearchBar: View {
    let placeholder: String
    var body: some View {
        HStack(spacing: 0) {
            MSymbol("search").foregroundStyle(M3.onSurface).padding(.leading, 16).padding(.trailing, 12)
            Text(placeholder).font(M3Type.bodyLarge).foregroundStyle(M3.onSurfaceVariant).lineLimit(1)
            Spacer(minLength: 0)
            HubAvatar().padding(.trailing, 4)
        }
        .frame(height: 56)
        .background(M3.containerHigh, in: Capsule())
    }
}

/// A menu: 48 dp items with leading icons, on a raised surface.
struct M3Menu: View {
    let items: [(String, String)]
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(items, id: \.0) { item in
                HStack(spacing: 12) {
                    MSymbol(item.1).foregroundStyle(M3.onSurfaceVariant)
                    Text(item.0).font(M3Type.bodyLarge).foregroundStyle(M3.onSurface)
                    Spacer(minLength: 0)
                }
                .padding(.horizontal, 12)
                .frame(height: 48)
            }
        }
        .padding(.vertical, 8)
        .frame(width: 200)
        .background(M3.containerHigh, in: RoundedRectangle(cornerRadius: M3Shape.large))
        .shadow(color: .black.opacity(0.18), radius: 6, y: 3)
    }
}

/// The Expressive wavy progress indicator, for work like writing a brief.
struct WavyProgress: View {
    var progress: CGFloat = 0.6
    var body: some View {
        Canvas { context, size in
            let split = size.width * progress
            var wave = Path()
            wave.move(to: CGPoint(x: 2, y: size.height / 2))
            for x in stride(from: CGFloat(2), through: split, by: 1) {
                wave.addLine(to: CGPoint(x: x, y: size.height / 2 + sin(x / 40 * 2 * .pi) * 3))
            }
            context.stroke(wave, with: .color(M3.primary), style: StrokeStyle(lineWidth: 4, lineCap: .round))
            var track = Path()
            track.move(to: CGPoint(x: split + 8, y: size.height / 2))
            track.addLine(to: CGPoint(x: size.width - 2, y: size.height / 2))
            context.stroke(track, with: .color(M3.secondaryContainer), style: StrokeStyle(lineWidth: 4, lineCap: .round))
            context.fill(Path(ellipseIn: CGRect(x: size.width - 4, y: size.height / 2 - 2, width: 4, height: 4)), with: .color(M3.primary))
        }
        .frame(height: 12)
    }
}

/// The modal bottom sheet's surface: 28 dp top corners and a drag handle.
struct M3BottomSheet<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Capsule().fill(M3.onSurfaceVariant.opacity(0.4)).frame(width: 32, height: 4).frame(maxWidth: .infinity).padding(.vertical, 22)
            content
            GestureHandle()
        }
        .background(M3.containerLow)
        .clipShape(UnevenRoundedRectangle(topLeadingRadius: M3Shape.extraLarge, topTrailingRadius: M3Shape.extraLarge))
    }
}

/// A one-line note under an Android mockup, with an icon.
struct PlatformNote: View {
    let text: String
    var icon = "check_circle"
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            MSymbol(icon, size: 16, fill: true).foregroundStyle(Tone.positive.m3).alignmentGuide(.firstTextBaseline) { $0[.bottom] - 3 }
            Text(text).font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
        }
    }
}

// MARK: Android screens

struct AndroidScreens: View {
    var body: some View {
        HStack(alignment: .top, spacing: 40) {
            labelled("Today", "Large flexible app bar; list items in groups; the hub avatar opens Settings") { today }
            labelled("Job", "Tabs like the Mac's inspector; the decision sits in thumb reach") { job }
            labelled("Pipeline", "Phases as tabs; swipe a card to record the follow-up") { pipeline }
            labelled("Settings", "Grouped like system Settings; unpairing asks first") { settings }
        }
        .padding(48)
        .background(Color(hex: 0xE8E8EE))
    }

    func labelled(_ title: String, _ subtitle: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text(title).font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
            Text(subtitle).font(.system(size: 12)).foregroundStyle(secondaryInk).lineLimit(2)
                .frame(width: 372, height: 32, alignment: .topLeading)
            content().padding(.top, Space.s)
        }
    }

    var today: some View {
        AndroidPhone(nav: "Today") { todayContent(skipped: false) }
    }

    /// Today's list; `skipped` drops the second job, `selected` marks the first one open beside it.
    func todayContent(skipped: Bool, selected: Bool = false) -> some View {
        VStack(spacing: 0) {
            M3TopBar(title: "Today", subtitle: skipped ? "6 to decide · 1 overdue · 2 replies" : "7 to decide · 1 overdue · 2 replies", size: .large) {
                M3IconButton("search")
                HubAvatar()
            }
            GroupHeading(title: skipped ? "Decide · 6" : "Decide · 7", action: "See all")
            SegmentedGroup(items: skipped ? [decideRow("N", .accent, "Senior Product Engineer", "Northwind · Remote", ("Strong", .positive))]
                : [decideRow("N", .accent, "Senior Product Engineer", "Northwind · Remote", ("Strong", .positive), selected: selected),
                   decideRow("G", .caution, "Staff Frontend Engineer", "Globex · Remote", ("Possible", .accent))])
            GroupHeading(title: "Follow up · 1")
            SegmentedGroup(items: [AnyView(M3ListItem(headline: "Full-Stack Engineer, Payments", supporting: "Initech", label: ("Overdue 2 days", .negative)) {
                Monogram(letter: "I", tone: .positive)
            } trailing: {
                M3IconButton("check", style: .tonal)
            })])
            GroupHeading(title: "Updates · 2")
            SegmentedGroup(items: [
                AnyView(M3ListItem(headline: "Reply from Umbrella Labs", supporting: "“Could you do a call on Tuesday…”") {
                    Monogram(letter: "U", tone: .neutral).overlay(alignment: .topTrailing) { M3Badge(text: "").offset(x: -2, y: 2) }
                } trailing: {
                    Text("09:12").font(M3Type.labelSmall).foregroundStyle(M3.onSurfaceVariant)
                }),
                AnyView(M3ListItem(headline: "Fresh strong match", supporting: "Northwind · posted 2 days ago") {
                    Monogram(letter: "N").overlay(alignment: .topTrailing) { M3Badge(text: "").offset(x: -2, y: 2) }
                } trailing: {
                    Text("08:40").font(M3Type.labelSmall).foregroundStyle(M3.onSurfaceVariant)
                }),
            ])
        }
    }

    private func decideRow(_ letter: String, _ tone: Tone, _ title: String, _ detail: String, _ label: (String, Tone), selected: Bool = false) -> AnyView {
        AnyView(M3ListItem(headline: title, supporting: detail, label: label) { Monogram(letter: letter, tone: tone) } trailing: { EmptyView() }
            .background(selected ? M3.secondaryContainer : M3.item))
    }

    var job: some View {
        AndroidPhone { jobContent }
    }

    var jobContent: some View { jobContent(back: true) }

    /// The job; without `back` it's the detail pane beside a list.
    func jobContent(back: Bool) -> some View {
        VStack(spacing: 0) {
            M3TopBar(title: "Senior Product Engineer", size: .medium, back: back) {
                M3IconButton("open_in_new")
                M3IconButton("more_vert")
            }
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 8) {
                    M3AssistChip(title: "Northwind", letter: "N")
                    Text("Remote, Americas · 2 days ago").font(M3Type.bodyMedium).foregroundStyle(M3.onSurfaceVariant).lineLimit(1)
                }
                HStack(spacing: 8) {
                    M3Label(text: "Strong match", tone: .positive, icon: "thumb_up")
                    M3Label(text: "Passes screen", tone: .positive, icon: "check_circle")
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 16)
            .padding(.bottom, 8)
            M3Tabs(tabs: ["Overview", "People · 2", "Posting"], selected: "Overview")
            VStack(alignment: .leading, spacing: 0) {
                VStack(alignment: .leading, spacing: 8) {
                    HStack(alignment: .firstTextBaseline) {
                        Text("Brief").font(M3Type.titleMedium).foregroundStyle(M3.onSurface)
                        Spacer()
                        Text("by Claude · today").font(M3Type.labelMedium).foregroundStyle(M3.onSurfaceVariant)
                    }
                    Text("Your payments cases map onto their checkout rebuild, and the take-home clears your target.")
                        .font(M3Type.bodyMedium).foregroundStyle(M3.onSurface).fixedSize(horizontal: false, vertical: true)
                    HStack(spacing: 8) {
                        MSymbol("remove_circle", size: 18, fill: true).foregroundStyle(Tone.caution.m3)
                        Text("No production GraphQL federation").font(M3Type.bodyMedium).foregroundStyle(M3.onSurface)
                    }
                }
                .padding(16)
                .background(M3.item, in: RoundedRectangle(cornerRadius: 20))
                .padding(.horizontal, 12)
                .padding(.top, 12)
                GroupHeading(title: "Screen")
                SegmentedGroup(items: [
                    screenRow("check_circle", .positive, "Where they hire", "Americas"),
                    screenRow("help", .caution, "Years", "Asks 6+; you have 5"),
                    screenRow("check_circle", .positive, "Pay", "About R$ 31k take-home"),
                ])
            }
            .frame(minHeight: 0, maxHeight: .infinity, alignment: .top)
            .clipped()
            HStack(spacing: 8) {
                M3Button(title: "Skip", style: .outlined, medium: true)
                M3Button(title: "Later", style: .tonal, medium: true)
                M3Button(title: "Pursue", icon: "check", style: .filled, medium: true).frame(maxWidth: .infinity).background(M3.primary, in: Capsule())
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(M3.containerLow)
        }
    }

    private func screenRow(_ icon: String, _ tone: Tone, _ name: String, _ detail: String) -> AnyView {
        AnyView(M3ListItem(headline: name, supporting: detail) { MSymbol(icon, fill: true).foregroundStyle(tone.m3) } trailing: { EmptyView() })
    }

    var pipeline: some View {
        AndroidPhone(nav: "Pipeline") { pipelineContent(swiped: true) }
    }

    func pipelineContent(swiped: Bool) -> some View {
        VStack(spacing: 0) {
            M3TopBar(title: "Pipeline") {
                M3IconButton("search")
                HubAvatar()
            }
            M3Tabs(tabs: ["Applied 4", "Screening 2", "Interviewing 1", "Offer"], selected: "Applied 4", scrollable: true)
            HStack(spacing: 8) {
                M3FilterChip(title: "Follow-up due", selected: true)
                M3FilterChip(title: "Heard back")
                M3FilterChip(title: "Agency")
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 16).padding(.vertical, 12)
            SegmentedGroup(items: [
                card("I", .positive, "Full-Stack Engineer, Payments", "Initech · 9 days", ("Overdue 2 days", .negative)),
                swiped ? swipedCard : card("A", .caution, "Outreach", "Acme Robotics · 7 days", ("Due today", .caution)),
                card("U", .neutral, "Senior Software Engineer", "Umbrella Labs · 3 days", ("Heard back", .positive)),
                card("G", .caution, "Product Engineer", "Globex · 3 days", ("Due in 4 days", .neutral)),
            ])
        }
    }

    private func card(_ letter: String, _ tone: Tone, _ title: String, _ detail: String, _ label: (String, Tone)) -> AnyView {
        AnyView(M3ListItem(headline: title, supporting: detail, label: label) { Monogram(letter: letter, tone: tone) } trailing: { EmptyView() })
    }

    /// The card mid-swipe: the follow-up action shows under it, as swipe-to-dismiss does.
    private var swipedCard: AnyView {
        AnyView(
            ZStack(alignment: .leading) {
                HStack(spacing: 8) {
                    MSymbol("check", fill: true)
                    Text("Followed up").font(M3Type.labelLarge)
                }
                .foregroundStyle(Tone.positive.m3OnContainer)
                .padding(.leading, 20)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
                .background(Tone.positive.m3Container)
                M3ListItem(headline: "Outreach", supporting: "Acme Robotics", label: ("Due today", .caution)) { Monogram(letter: "A", tone: .caution) } trailing: { EmptyView() }
                    .background(M3.item, in: RoundedRectangle(cornerRadius: 12))
                    .offset(x: 140)
            }
            .frame(height: 76)
        )
    }

    var settings: some View {
        AndroidPhone {
            VStack(spacing: 0) {
                M3TopBar(title: "Settings", back: true)
                GroupHeading(title: "Hub")
                SegmentedGroup(items: [
                    settingsRow("computer", "Paired with", "Your Mac, over Tailscale"),
                    settingsRow("sync", "Last update", "Just now"),
                ])
                GroupHeading(title: "Notifications")
                SegmentedGroup(items: [
                    toggleRow("Fresh strong matches", true),
                    toggleRow("Follow-ups due", true),
                    toggleRow("Replies and confirmations", false),
                    AnyView(M3ListItem(headline: "Sound and vibration", supporting: "Per kind, in system settings") {
                        MSymbol("notifications").foregroundStyle(M3.onSurfaceVariant)
                    } trailing: { MSymbol("open_in_new", size: 20).foregroundStyle(M3.onSurfaceVariant) }),
                ])
                GroupHeading(title: "Appearance")
                SegmentedGroup(items: [
                    AnyView(M3ListItem(headline: "Match wallpaper colors", supporting: "Off: Hub Indigo") {
                        MSymbol("palette").foregroundStyle(M3.onSurfaceVariant)
                    } trailing: { M3Switch(isOn: false) }),
                ])
                Spacer().frame(height: 12)
                SegmentedGroup(items: [
                    AnyView(M3ListItem(headline: "Unpair this phone", headlineColor: M3.error) {
                        MSymbol("link_off").foregroundStyle(M3.error)
                    } trailing: { EmptyView() }),
                ])
            }
        }
        .overlay {
            ZStack {
                RoundedRectangle(cornerRadius: 34, style: .continuous).fill(M3.scrim).padding(6)
                M3Dialog(icon: "link_off", headline: "Unpair this phone?",
                         text: "It stops getting updates and reminders until you pair it again from the Mac.",
                         dismiss: "Cancel", confirm: "Unpair", destructive: true)
                    .offset(y: 60)
            }
        }
    }

    private func settingsRow(_ icon: String, _ title: String, _ detail: String) -> AnyView {
        AnyView(M3ListItem(headline: title, supporting: detail) { MSymbol(icon).foregroundStyle(M3.onSurfaceVariant) } trailing: { EmptyView() })
    }

    private func toggleRow(_ title: String, _ isOn: Bool) -> AnyView {
        AnyView(M3ListItem(headline: title) { MSymbol(isOn ? "notifications" : "notifications_off").foregroundStyle(M3.onSurfaceVariant) } trailing: { M3Switch(isOn: isOn) })
    }
}

// MARK: - Annotation pieces

/// A numbered callout pinned to a mockup, matched by a note beside it.
struct Marker: View {
    let number: Int
    var color = Color(hex: 0xE5484D)
    var body: some View {
        Text("\(number)").font(.system(size: 11, weight: .bold)).foregroundStyle(.white)
            .frame(width: 20, height: 20).background(color, in: Circle())
    }
}

struct BoardHeading: View {
    let title: String
    let subtitle: String
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            Text(title).font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
            Text(subtitle).font(.system(size: 13)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// A problem in the before picture and what the proposal does about it.
struct ProblemNote: View {
    let number: Int
    let problem: String
    let fix: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Marker(number: number)
            VStack(alignment: .leading, spacing: 3) {
                Text(problem).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                    Image(systemName: "arrow.turn.down.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.positive.color)
                    Text(fix).font(.system(size: 12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

/// A window drawn on a board, beside others: rounded, outlined and lifted.
struct Framed: ViewModifier {
    func body(content: Content) -> some View {
        content
            .background(windowBackground)
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.black.opacity(0.12)))
            .compositingGroup()
            .shadow(color: .black.opacity(0.14), radius: 18, y: 8)
    }
}

extension View {
    func framed() -> some View { modifier(Framed()) }
}

struct Keycap: View {
    let key: String
    var body: some View {
        Text(key).font(.system(size: 11, weight: .semibold)).foregroundStyle(ink)
            .frame(minWidth: 20).padding(.horizontal, 5).padding(.vertical, 2)
            .background(surface, in: RoundedRectangle(cornerRadius: 5))
            .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(separator))
            .shadow(color: .black.opacity(0.08), radius: 0, y: 1)
    }
}

struct Switch: View {
    let isOn: Bool
    var body: some View {
        Capsule().fill(isOn ? Tone.accent.color : fill)
            .frame(width: 34, height: 20)
            .overlay(alignment: isOn ? .trailing : .leading) {
                Circle().fill(.white).frame(width: 16, height: 16).padding(2).shadow(color: .black.opacity(0.15), radius: 1, y: 1)
            }
    }
}

// MARK: - Today's look, for the before pictures

/// A macOS push button as the app draws them today: every action the same.
struct LegacyButton: View {
    let title: String
    var symbol: String?
    var body: some View {
        HStack(spacing: 4) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11)) }
            Text(title).font(.system(size: 12))
        }
        .fixedSize()
        .foregroundStyle(ink)
        .padding(.horizontal, 8).padding(.vertical, 3)
        .background(.white, in: RoundedRectangle(cornerRadius: 5))
        .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(Color.black.opacity(0.13)))
        .shadow(color: .black.opacity(0.06), radius: 0.5, y: 0.5)
    }
}

/// `MatchLabel` today: the tone at 18% behind it.
struct LegacyMatchLabel: View {
    let text: String
    let color: Color
    var body: some View {
        Text(text).font(.system(size: 11, weight: .semibold)).foregroundStyle(color)
            .padding(.horizontal, 6).padding(.vertical, 2)
            .background(color.opacity(0.18), in: RoundedRectangle(cornerRadius: 4))
    }
}

/// `FitLabel` today: colored text with no background.
struct LegacyFitLabel: View {
    let text: String
    let color: Color
    var body: some View {
        Text(text).font(.system(size: 12, weight: .semibold)).foregroundStyle(color)
    }
}

// MARK: - Information architecture map

struct GraphNode {
    let id: String
    let title: String
    var detail: String?
    let center: CGPoint
    var size = CGSize(width: 150, height: 40)
    var isDetail = false
    var isRegion = false
}

struct GraphEdge {
    enum Kind { case opens, linked, switchesPage, missing }
    let from: String
    let to: String
    let kind: Kind
    var label: String?
    var labelAt: CGPoint?
    /// Bows the edge sideways by this much, to route around nodes.
    var bend: CGFloat = 0
    /// Ends level with its start, for arrows into a wide region.
    var level = false
}

struct Graph: View {
    let nodes: [GraphNode]
    let edges: [GraphEdge]
    let size: CGSize

    var body: some View {
        ZStack(alignment: .topLeading) {
            ForEach(Array(nodes.filter(\.isRegion).enumerated()), id: \.offset) { _, node in region(node) }
            Canvas { context, _ in
                for edge in edges { draw(edge, in: &context) }
            }
            ForEach(Array(nodes.filter { !$0.isRegion }.enumerated()), id: \.offset) { _, node in box(node).position(node.center) }
            ForEach(Array(edges.enumerated()), id: \.offset) { _, edge in
                if edge.kind == .missing {
                    Image(systemName: "xmark.circle.fill").font(.system(size: 15)).foregroundStyle(Tone.negative.color)
                        .background(Circle().fill(windowBackground))
                        .position(midpoint(edge))
                }
                if let label = edge.label {
                    Text(label).font(.system(size: 11, weight: .medium)).foregroundStyle(color(edge.kind))
                        .multilineTextAlignment(.center).fixedSize()
                        .padding(.horizontal, 3)
                        .background(windowBackground.opacity(0.9))
                        .position(edge.labelAt ?? midpoint(edge))
                }
            }
        }
        .frame(width: size.width, height: size.height, alignment: .topLeading)
    }

    private func node(_ id: String) -> GraphNode { nodes.first { $0.id == id }! }

    private func box(_ node: GraphNode) -> some View {
        VStack(spacing: 1) {
            Text(node.title).font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
            if let detail = node.detail {
                Text(detail).font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
        }
        .frame(width: node.size.width, height: node.size.height)
        .background(node.isDetail ? Tone.accent.color.opacity(0.08) : surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(node.isDetail ? Tone.accent.color.opacity(0.55) : separator))
    }

    private func region(_ node: GraphNode) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(node.title).font(.system(size: 13, weight: .semibold)).foregroundStyle(Tone.accent.color)
            if let detail = node.detail {
                Text(detail).font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
        }
        .padding(Space.m)
        .frame(width: node.size.width, height: node.size.height, alignment: .topLeading)
        .background(Tone.accent.color.opacity(0.05), in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(Tone.accent.color.opacity(0.5), style: StrokeStyle(lineWidth: 1.2, dash: [5, 4])))
        .offset(x: node.center.x - node.size.width / 2, y: node.center.y - node.size.height / 2)
    }

    private func color(_ kind: GraphEdge.Kind) -> Color {
        switch kind {
        case .opens, .linked: Tone.accent.color
        case .switchesPage: Tone.caution.color
        case .missing: Tone.negative.color
        }
    }

    private func clip(_ node: GraphNode, toward point: CGPoint) -> CGPoint {
        let dx = point.x - node.center.x
        let dy = point.y - node.center.y
        let halfWidth = node.size.width / 2 + 4
        let halfHeight = node.size.height / 2 + 4
        let t = min(dx == 0 ? .infinity : halfWidth / abs(dx), dy == 0 ? .infinity : halfHeight / abs(dy))
        return CGPoint(x: node.center.x + dx * t, y: node.center.y + dy * t)
    }

    private func geometry(_ edge: GraphEdge) -> (start: CGPoint, control: CGPoint, end: CGPoint) {
        let a = node(edge.from)
        let b = node(edge.to)
        if edge.level {
            let start = CGPoint(x: a.center.x + a.size.width / 2 + 4, y: a.center.y)
            let end = CGPoint(x: b.center.x - b.size.width / 2 - 4, y: a.center.y)
            return (start, CGPoint(x: (start.x + end.x) / 2, y: start.y), end)
        }
        let dx = b.center.x - a.center.x
        let dy = b.center.y - a.center.y
        let length = max(1, (dx * dx + dy * dy).squareRoot())
        let control = CGPoint(x: (a.center.x + b.center.x) / 2 - dy / length * edge.bend, y: (a.center.y + b.center.y) / 2 + dx / length * edge.bend)
        return (clip(a, toward: control), control, clip(b, toward: control))
    }

    private func midpoint(_ edge: GraphEdge) -> CGPoint {
        let (start, control, end) = geometry(edge)
        return CGPoint(x: 0.25 * start.x + 0.5 * control.x + 0.25 * end.x, y: 0.25 * start.y + 0.5 * control.y + 0.25 * end.y)
    }

    private func draw(_ edge: GraphEdge, in context: inout GraphicsContext) {
        let (start, control, end) = geometry(edge)
        var path = Path()
        path.move(to: start)
        path.addQuadCurve(to: end, control: control)
        let shading = GraphicsContext.Shading.color(color(edge.kind))
        switch edge.kind {
        case .opens, .linked: context.stroke(path, with: shading, lineWidth: 1.5)
        case .switchesPage: context.stroke(path, with: shading, style: StrokeStyle(lineWidth: 1.5, dash: [6, 4]))
        case .missing: context.stroke(path, with: shading, style: StrokeStyle(lineWidth: 1.5, dash: [2, 4]))
        }
        if edge.kind != .missing { context.fill(arrowhead(at: end, from: control), with: shading) }
        if edge.kind == .linked { context.fill(arrowhead(at: start, from: control), with: shading) }
    }

    private func arrowhead(at tip: CGPoint, from tail: CGPoint) -> Path {
        let angle = atan2(tip.y - tail.y, tip.x - tail.x)
        let length: CGFloat = 8
        var path = Path()
        path.move(to: tip)
        path.addLine(to: CGPoint(x: tip.x - length * cos(angle - 0.42), y: tip.y - length * sin(angle - 0.42)))
        path.addLine(to: CGPoint(x: tip.x - length * cos(angle + 0.42), y: tip.y - length * sin(angle + 0.42)))
        path.closeSubpath()
        return path
    }
}

struct InformationMap: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            VStack(alignment: .leading, spacing: Space.xs) {
                Text("Where a job, a company and a person open").font(.system(size: 22, weight: .semibold)).foregroundStyle(ink)
                Text("Every arrow is a click in the Mac app. Today a job, a company and a recruiter each open their own way, three clicks throw you onto another page, and a company links to neither its jobs nor its recruiters.")
                    .font(.system(size: 13)).foregroundStyle(secondaryInk)
            }
            HStack(alignment: .top, spacing: 48) {
                panel("Today", "Three detail patterns that don't link") { today }
                panel("Proposed", "One inspector, reached from every page and ⌘K") { proposed }
            }
            HStack(spacing: Space.xl) {
                legend(.opens, "Opens beside the page")
                legend(.linked, "Links both ways, inside the inspector")
                legend(.switchesPage, "Switches the page under you")
                legend(.missing, "No link")
            }
        }
        .padding(48)
        .background(windowBackground)
    }

    private func panel(_ title: String, _ subtitle: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            BoardHeading(title: title, subtitle: subtitle)
            content()
                .padding(Space.l)
                .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
                .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
        }
    }

    private var today: some View {
        let detail = CGSize(width: 210, height: 52)
        return Graph(
            nodes: [
                GraphNode(id: "decide", title: "Decide", center: CGPoint(x: 200, y: 50)),
                GraphNode(id: "jobs", title: "Jobs", center: CGPoint(x: 200, y: 130)),
                GraphNode(id: "pipeline", title: "Pipeline", center: CGPoint(x: 200, y: 210)),
                GraphNode(id: "updates", title: "Updates", center: CGPoint(x: 200, y: 300)),
                GraphNode(id: "companies", title: "Companies", center: CGPoint(x: 200, y: 390)),
                GraphNode(id: "recruiters", title: "Recruiters", center: CGPoint(x: 200, y: 490)),
                GraphNode(id: "job", title: "Job details", detail: "the window's inspector", center: CGPoint(x: 500, y: 130), size: detail, isDetail: true),
                GraphNode(id: "company", title: "Company dossier", detail: "its own fixed 460 pt panel", center: CGPoint(x: 500, y: 390), size: detail, isDetail: true),
                GraphNode(id: "recruiter", title: "Recruiter details", detail: "the page's own inspector", center: CGPoint(x: 500, y: 490), size: detail, isDetail: true),
            ],
            edges: [
                GraphEdge(from: "decide", to: "job", kind: .opens),
                GraphEdge(from: "jobs", to: "job", kind: .opens),
                GraphEdge(from: "pipeline", to: "job", kind: .opens),
                GraphEdge(from: "companies", to: "company", kind: .opens),
                GraphEdge(from: "recruiters", to: "recruiter", kind: .opens),
                GraphEdge(from: "updates", to: "jobs", kind: .switchesPage, label: "switches\nto Jobs", labelAt: CGPoint(x: 48, y: 215), bend: -170),
                GraphEdge(from: "updates", to: "companies", kind: .switchesPage, label: "switches to Companies", labelAt: CGPoint(x: 64, y: 345)),
                GraphEdge(from: "recruiter", to: "companies", kind: .switchesPage, label: "Open company\nswitches page", labelAt: CGPoint(x: 335, y: 450)),
                GraphEdge(from: "job", to: "company", kind: .missing, label: "no link either way;\nthe dossier doesn't\nlist its open jobs", labelAt: CGPoint(x: 572, y: 220)),
                GraphEdge(from: "company", to: "recruiter", kind: .missing, label: "doesn't list\nits recruiters", labelAt: CGPoint(x: 563, y: 440)),
            ],
            size: CGSize(width: 640, height: 530)
        )
    }

    private var proposed: some View {
        let entity = CGSize(width: 136, height: 52)
        return Graph(
            nodes: [
                GraphNode(id: "inspector", title: "One inspector", detail: "Same anatomy for every kind · ⌘[ ⌘] history", center: CGPoint(x: 460, y: 265), size: CGSize(width: 330, height: 470), isRegion: true),
                GraphNode(id: "today", title: "Today", center: CGPoint(x: 110, y: 50)),
                GraphNode(id: "decide", title: "Decide", center: CGPoint(x: 110, y: 115)),
                GraphNode(id: "pipeline", title: "Pipeline", center: CGPoint(x: 110, y: 180)),
                GraphNode(id: "jobs", title: "Jobs", center: CGPoint(x: 110, y: 245)),
                GraphNode(id: "companies", title: "Companies", center: CGPoint(x: 110, y: 310)),
                GraphNode(id: "people", title: "People", center: CGPoint(x: 110, y: 375)),
                GraphNode(id: "palette", title: "⌘K  Jump anywhere", center: CGPoint(x: 110, y: 465)),
                GraphNode(id: "job", title: "Job", detail: "Overview · Prep · Posting", center: CGPoint(x: 460, y: 160), size: CGSize(width: 180, height: 52), isDetail: true),
                GraphNode(id: "company", title: "Company", detail: "Overview · Jobs · People", center: CGPoint(x: 375, y: 380), size: entity, isDetail: true),
                GraphNode(id: "person", title: "Person", detail: "Overview · Conversation", center: CGPoint(x: 545, y: 380), size: entity, isDetail: true),
            ],
            edges: [
                GraphEdge(from: "today", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "decide", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "pipeline", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "jobs", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "companies", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "people", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "palette", to: "inspector", kind: .opens, level: true),
                GraphEdge(from: "job", to: "company", kind: .linked, label: "eyebrow ·\nJobs tab", labelAt: CGPoint(x: 362, y: 268)),
                GraphEdge(from: "job", to: "person", kind: .linked, label: "People\nsection", labelAt: CGPoint(x: 560, y: 268)),
                GraphEdge(from: "company", to: "person", kind: .linked, label: "People tab · company link", labelAt: CGPoint(x: 460, y: 422)),
            ],
            size: CGSize(width: 640, height: 530)
        )
    }

    private func legend(_ kind: GraphEdge.Kind, _ text: String) -> some View {
        HStack(spacing: Space.s) {
            Canvas { context, size in
                var path = Path()
                path.move(to: CGPoint(x: 0, y: size.height / 2))
                path.addLine(to: CGPoint(x: size.width, y: size.height / 2))
                switch kind {
                case .opens, .linked: context.stroke(path, with: .color(Tone.accent.color), lineWidth: 1.5)
                case .switchesPage: context.stroke(path, with: .color(Tone.caution.color), style: StrokeStyle(lineWidth: 1.5, dash: [6, 4]))
                case .missing: context.stroke(path, with: .color(Tone.negative.color), style: StrokeStyle(lineWidth: 1.5, dash: [2, 4]))
                }
            }
            .frame(width: 36, height: 10)
            if kind == .linked {
                Image(systemName: "arrow.left.and.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.accent.color)
            }
            Text(text).font(.system(size: 12)).foregroundStyle(secondaryInk)
        }
    }
}

// MARK: - Job detail before and after

/// The job detail as `JobDetailView` draws it today, for a job not yet decided.
struct LegacyJobDetail: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                segment("Details", selected: true)
                segment("Session", selected: false)
            }
            .padding(2)
            .background(fill, in: RoundedRectangle(cornerRadius: 7))
            .padding(8)
            VStack(alignment: .leading, spacing: 20) {
                header.overlay(alignment: .topLeading) { Marker(number: 6).offset(x: -40) }
                actions.overlay(alignment: .topLeading) { Marker(number: 1).offset(x: -40) }
                brief
                screenOut.overlay(alignment: .topLeading) { Marker(number: 4).offset(x: -40) }
                section("People you know at Northwind") {
                    (Text("Alex Kim").foregroundColor(ink) + Text(" · Engineering Manager").foregroundColor(secondaryInk)).font(.system(size: 13))
                }
                fit.overlay(alignment: .topLeading) { Marker(number: 3).offset(x: -40) }
                facts.overlay(alignment: .topLeading) { Marker(number: 7).offset(x: -40) }
                HStack(spacing: 6) {
                    Image(systemName: "chevron.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(secondaryInk)
                    Text("Posting").font(.system(size: 13, weight: .bold)).foregroundStyle(ink)
                }
            }
            .padding(.horizontal, 20).padding(.top, 6)
            Spacer(minLength: 0)
        }
        .frame(width: 440)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(.white, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
    }

    private func segment(_ title: String, selected: Bool) -> some View {
        Text(title).font(.system(size: 12, weight: selected ? .medium : .regular)).foregroundStyle(ink)
            .frame(maxWidth: .infinity).padding(.vertical, 3)
            .background(selected ? AnyShapeStyle(.white) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 5))
            .shadow(color: .black.opacity(selected ? 0.1 : 0), radius: 1, y: 1)
    }

    private func section(_ title: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.system(size: 13, weight: .bold)).foregroundStyle(ink)
            content()
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Senior Product Engineer").font(.system(size: 20, weight: .semibold)).foregroundStyle(ink)
            Text("Northwind · Remote, Americas").font(.system(size: 13)).foregroundStyle(secondaryInk)
        }
    }

    private var actions: some View {
        HStack(spacing: 6) {
            LegacyButton(title: "Open posting", symbol: "safari")
            LegacyButton(title: "Pursue", symbol: "arrow.up.forward.circle")
            LegacyButton(title: "Skip…", symbol: "eye.slash")
            LegacyButton(title: "Later", symbol: "clock")
            LegacyButton(title: "Fix…", symbol: "wrench.adjustable")
        }
    }

    private var brief: some View {
        section("Brief") {
            HStack(spacing: 8) {
                LegacyMatchLabel(text: "Strong", color: .green)
                Text("by the local model").font(.system(size: 11)).foregroundStyle(secondaryInk)
                Label("Your knowledge base changed since", systemImage: "clock.arrow.circlepath").font(.system(size: 11)).foregroundStyle(.orange)
            }
            .overlay(alignment: .topLeading) { Marker(number: 2).offset(x: -40) }
            Text("Your payments and checkout cases map straight onto their rebuild, and the take-home clears your target.")
                .font(.system(size: 13)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            Text("Strengths").font(.system(size: 11, weight: .semibold)).foregroundStyle(ink)
            point("plus.circle.fill", .green, "Led a checkout rewrite that lifted conversion", "Checkout rebuild")
            point("plus.circle.fill", .green, "Five years of React and TypeScript", "React")
            Text("Weaknesses").font(.system(size: 11, weight: .semibold)).foregroundStyle(ink)
                .overlay(alignment: .topLeading) { Marker(number: 5).offset(x: -40) }
            point("minus.circle.fill", .orange, "No production GraphQL federation", "GraphQL")
            LegacyButton(title: "Write full brief", symbol: "sparkles")
        }
    }

    private func point(_ symbol: String, _ color: Color, _ text: String, _ entry: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Image(systemName: symbol).font(.system(size: 12)).foregroundStyle(color)
            VStack(alignment: .leading, spacing: 1) {
                Text(text).font(.system(size: 13)).foregroundStyle(ink)
                Text(entry).font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
        }
    }

    private func check(_ symbol: String, _ color: Color, _ name: String, _ reason: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Image(systemName: symbol).font(.system(size: 12)).foregroundStyle(color)
            (Text(name).fontWeight(.medium).foregroundColor(ink) + Text("  " + reason).foregroundColor(secondaryInk)).font(.system(size: 13))
        }
    }

    private var screenOut: some View {
        section("Screen-out checks") {
            check("checkmark.circle.fill", .green, "Where they hire", "Americas")
            check("questionmark.circle.fill", .orange, "Years", "6+ years")
            Text("\u{201C}6+ years building web products with React\u{201D}").font(.system(size: 11)).foregroundStyle(secondaryInk).padding(.leading, 18)
            check("info.circle", secondaryInk, "Timezone", "US business hours")
        }
    }

    private var fit: some View {
        section("Fit") {
            HStack(spacing: 6) {
                LegacyFitLabel(text: "Unclear", color: .orange)
                Text("for your criteria").font(.system(size: 13)).foregroundStyle(secondaryInk)
            }
            check("checkmark.circle.fill", .green, "Role", "Product engineer")
            check("checkmark.circle.fill", .green, "Where they hire", "Americas")
            check("checkmark.circle.fill", .green, "Stack", "React, TypeScript, Node.js")
            check("questionmark.circle.fill", .orange, "Years", "Asks 6+, you have 5")
            check("checkmark.circle.fill", .green, "Pay", "About R$ 31k a month take-home")
        }
    }

    private var facts: some View {
        VStack(alignment: .leading, spacing: 20) {
            section("From the board") {
                factRow("Workplace", "Remote")
                factRow("Pay", "Not published", secondary: true)
                factRow("Posted", "2 Oct 2026")
            }
            section("Read from the posting") {
                factRow("Seniority", "Senior")
                factRow("Salary", "Not stated", secondary: true)
                factRow("Timezone", "US business hours")
                Text("Read by the local model with prompt version 4, 2 Oct 2026 at 09:12.").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                LegacyButton(title: "Read facts now", symbol: "arrow.clockwise")
            }
        }
    }

    private func factRow(_ label: String, _ value: String, secondary: Bool = false) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Text(label).font(.system(size: 13)).foregroundStyle(secondaryInk).frame(width: 80, alignment: .trailing)
            Text(value).font(.system(size: 13)).foregroundStyle(secondary ? secondaryInk : ink)
        }
    }
}

struct JobBeforeAfter: View {
    var body: some View {
        HStack(alignment: .top, spacing: 56) {
            VStack(alignment: .leading, spacing: Space.l) {
                BoardHeading(title: "Today: the job detail", subtitle: "Nine stacked sections for a job to decide, eleven after Pursue.")
                LegacyJobDetail().frame(height: 1130)
            }
            .padding(.leading, 24)
            VStack(alignment: .leading, spacing: Space.l) {
                BoardHeading(title: "Proposed: the job inspector", subtitle: "The decision on top, one verdict list, the rest in tabs.")
                JobInspector()
                    .frame(height: 1130)
                    .clipShape(RoundedRectangle(cornerRadius: Radius.panel))
                    .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
            }
            VStack(alignment: .leading, spacing: Space.l) {
                Text("What changes").font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
                ProblemNote(number: 1, problem: "Five buttons with the same weight",
                            fix: "Pursue is the one primary action. Later and Skip are secondary; Open posting and Fix go in the overflow menu.")
                ProblemNote(number: 2, problem: "Match is one verdict, in the brief…",
                            fix: "Match stays the brief's verdict, as a chip in the header.")
                ProblemNote(number: 3, problem: "…and Fit is another, on another scale and color",
                            fix: "Fit is renamed Screen and becomes the second header chip: Passes, Unclear or Fails.")
                ProblemNote(number: 4, problem: "Screen-out checks and Fit both ask \"does this rule me out?\"",
                            fix: "They merge into one Screen list of verdict rows, evidence quoted. Where they hire shows once, not twice.")
                ProblemNote(number: 5, problem: "Orange means a stale brief, a weakness and an unclear fit",
                            fix: "Caution is the only orange, and every use has a word and a symbol next to it.")
                ProblemNote(number: 6, problem: "Northwind is plain text, with no way to the company",
                            fix: "The eyebrow JOB · NORTHWIND links to the company in the same inspector; ⌘[ comes back.")
                ProblemNote(number: 7, problem: "Board facts, posting facts and the posting sit in the way",
                            fix: "They move to the Posting tab. Prep (CV, interview pack, answers) gets its own tab after Pursue.")
            }
            .frame(width: 400)
            .padding(.top, 52)
        }
        .padding(48)
        .background(windowBackground)
    }
}

// MARK: - Vocabulary

struct VocabularyBoard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 40) {
            VStack(alignment: .leading, spacing: Space.xs) {
                Text("One word per concept, on both clients").font(.system(size: 22, weight: .semibold)).foregroundStyle(ink)
                Text("The server keeps its field names (dismiss, dismissed_at); only the words and their look change.")
                    .font(.system(size: 13)).foregroundStyle(secondaryInk)
            }
            HStack(alignment: .top, spacing: 40) {
                VStack(alignment: .leading, spacing: Space.m) {
                    BoardHeading(title: "A pipeline card today", subtitle: "Six lines in four colors, and two ways out that read alike")
                    HStack(alignment: .top, spacing: Space.m) {
                        legacyCard
                        menu(["Open posting", "Followed up…", "Move to ›", "Not a good fit…"], highlighted: "Not a good fit…", color: .blue)
                    }
                }
                Image(systemName: "arrow.right").font(.system(size: 22, weight: .medium)).foregroundStyle(tertiaryInk).padding(.top, 110)
                VStack(alignment: .leading, spacing: Space.m) {
                    BoardHeading(title: "Proposed", subtitle: "Title, company, at most two chips and the time in phase")
                    HStack(alignment: .top, spacing: Space.m) {
                        proposedCard
                        menu(["Open posting", "Followed up…", "Move to ›", "Skip…", "Close…"], highlighted: "Skip…")
                    }
                }
            }
            VStack(alignment: .leading, spacing: 0) {
                row(header: true, "Concept", today: { Text("Today").font(.system(size: 12, weight: .semibold)).foregroundStyle(secondaryInk) },
                    proposed: { Text("Proposed").font(.system(size: 12, weight: .semibold)).foregroundStyle(secondaryInk) })
                Divider()
                row("Taking a job out", today: {
                    LegacyButton(title: "Skip…", symbol: "eye.slash")
                    LegacyButton(title: "Dismiss")
                    old("Not a good fit…")
                    Label("Dismissed: too junior", systemImage: "eye.slash").font(.system(size: 11)).foregroundStyle(.orange)
                }, proposed: {
                    SecondaryButton(title: "Skip…", symbol: "eye.slash")
                    Chip(text: "Skipped: too junior", tone: .neutral, symbol: "eye.slash")
                    SecondaryButton(title: "Restore", symbol: "arrow.uturn.backward")
                })
                Divider()
                row("Is it for me?", today: {
                    LegacyMatchLabel(text: "Strong", color: .green)
                    Text("Match in the brief").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                    LegacyFitLabel(text: "Unclear", color: .orange)
                    Text("Fit in the table").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                    Text("Poor").font(.system(size: 12, weight: .semibold)).foregroundStyle(secondaryInk)
                    Text("grey on the Mac, red on Android").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                }, proposed: {
                    Chip(text: "Strong match", tone: .positive)
                    Chip(text: "Screen unclear", tone: .caution, symbol: "questionmark")
                    Chip(text: "Fails screen", tone: .negative, symbol: "xmark")
                })
                Divider()
                row("Follow-ups", today: {
                    Text("Follow-up 2 days overdue").font(.system(size: 11, weight: .semibold)).foregroundStyle(.red)
                    Text("Follow up today").font(.system(size: 11, weight: .semibold)).foregroundStyle(.orange)
                }, proposed: {
                    Chip(text: "Overdue 2 days", tone: .negative, symbol: "bell.fill")
                    Chip(text: "Due today", tone: .caution, symbol: "bell.fill")
                    Chip(text: "Due in 3 days", tone: .neutral)
                })
                Divider()
                row("A company's record", today: {
                    old("Dossier")
                    Text("on the Mac").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                    old("Company brief")
                    Text("on Android").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                }, proposed: {
                    Text("Company").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                    Text("its Overview tab").font(.system(size: 11)).foregroundStyle(tertiaryInk)
                })
                Divider()
                row("Who can get me in", today: {
                    old("People")
                    old("People you know")
                    old("Can introduce you")
                    old("Recruiters")
                }, proposed: {
                    Text("People").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                    Chip(text: "Contact", tone: .neutral)
                    Chip(text: "Connection", tone: .neutral)
                    Chip(text: "Introducer", tone: .neutral)
                    Chip(text: "Recruiter", tone: .neutral)
                })
                Divider()
                row("The hub's own work", today: {
                    old("Runs")
                    old("agent runs")
                    old("task runs")
                    old("model work")
                }, proposed: {
                    Text("Activity").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                })
            }
            .padding(.horizontal, Space.l)
            .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
            .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
        }
        .padding(48)
        .frame(width: 1400, alignment: .leading)
        .background(windowBackground)
    }

    private func old(_ word: String) -> some View {
        Text(word).font(.system(size: 12)).foregroundStyle(secondaryInk)
            .strikethrough(color: Tone.negative.color.opacity(0.7))
            .padding(.horizontal, 6).padding(.vertical, 2)
            .background(fillSubtle, in: RoundedRectangle(cornerRadius: 4))
    }

    private func row(header: Bool = false, _ concept: String, @ViewBuilder today: () -> some View, @ViewBuilder proposed: () -> some View) -> some View {
        HStack(alignment: .center, spacing: Space.l) {
            Text(concept).font(.system(size: header ? 12 : 13, weight: .semibold)).foregroundStyle(header ? secondaryInk : ink)
                .frame(width: 170, alignment: .leading)
            HStack(spacing: Space.s) { today() }.frame(width: 640, alignment: .leading)
            HStack(spacing: Space.s) { proposed() }
            Spacer(minLength: 0)
        }
        .padding(.vertical, header ? Space.s : Space.m)
    }

    private var legacyCard: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline) {
                Text("Full-Stack Engineer, Payments").font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                Spacer(minLength: 0)
                Text("2").font(.system(size: 10, weight: .bold)).foregroundStyle(.white)
                    .frame(width: 16, height: 16).background(Color.blue, in: Circle())
            }
            Text("Initech").font(.system(size: 13)).foregroundStyle(secondaryInk)
            Label("Heard back 28 Sep", systemImage: "arrowshape.turn.up.left").font(.system(size: 11)).foregroundStyle(.green)
            Text("Follow-up 2 days overdue").font(.system(size: 11, weight: .semibold)).foregroundStyle(.red)
            Label("Asked for on-site in the first month", systemImage: "eye.slash").font(.system(size: 11)).foregroundStyle(.orange)
            Text("9 days in phase").font(.system(size: 11)).foregroundStyle(secondaryInk)
        }
        .padding(10)
        .frame(width: 260, alignment: .leading)
        .background(.white, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(Color.blue, lineWidth: 2))
    }

    private var proposedCard: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .firstTextBaseline) {
                Text("Full-Stack Engineer, Payments").font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                Spacer(minLength: 0)
                UnseenDot()
            }
            Text("Initech").font(.system(size: 12)).foregroundStyle(secondaryInk)
            HStack(spacing: Space.xs) {
                Chip(text: "Overdue 2 days", tone: .negative, symbol: "bell.fill")
                Chip(text: "Heard back", tone: .positive, symbol: "arrowshape.turn.up.left.fill")
            }
            Text("9 days in Applied").font(.system(size: 11)).foregroundStyle(tertiaryInk)
        }
        .padding(Space.m)
        .frame(width: 260, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(Tone.accent.color, lineWidth: 2))
    }

    private func menu(_ items: [String], highlighted: String, color: Color = Tone.accent.color) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(items, id: \.self) { item in
                Text(item).font(.system(size: 13)).foregroundStyle(item == highlighted ? .white : ink)
                    .frame(width: 150, alignment: .leading)
                    .padding(.horizontal, 8).padding(.vertical, 3)
                    .background(item == highlighted ? AnyShapeStyle(color) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 4))
            }
        }
        .padding(5)
        .background(surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(separator))
        .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
    }
}

// MARK: - Company and person inspectors

/// The inspector's anatomy for any kind: history, header, action bar, tabs.
struct InspectorShell<Content: View>: View {
    let kind: String
    var parent: String?
    let title: String
    let facts: String
    let chips: [(String, Tone, String?)]
    let primary: (String, String)
    var secondary: [(String, String)] = []
    let tabs: [String]
    let selectedTab: String
    var canGoBack = true
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: Space.s) {
                navButton("chevron.left").opacity(canGoBack ? 1 : 0.4)
                navButton("chevron.right").opacity(0.4)
                Spacer()
                navButton("sidebar.trailing")
            }
            .padding(.horizontal, Space.l)
            .frame(height: 52)
            VStack(alignment: .leading, spacing: Space.l) {
                VStack(alignment: .leading, spacing: Space.xs) {
                    HStack(spacing: Space.xs) {
                        Text(parent == nil ? kind : "\(kind) ·").font(.system(size: 10, weight: .semibold)).foregroundStyle(tertiaryInk)
                        if let parent { Text(parent).font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.accent.color) }
                    }
                    Text(title).font(.system(size: 20, weight: .semibold)).foregroundStyle(ink)
                    Text(facts).font(.system(size: 12)).foregroundStyle(secondaryInk)
                    HStack(spacing: Space.xs) {
                        ForEach(Array(chips.enumerated()), id: \.offset) { _, chip in Chip(text: chip.0, tone: chip.1, symbol: chip.2) }
                    }
                    .padding(.top, 2)
                }
                HStack(spacing: Space.s) {
                    PrimaryButton(title: primary.0, symbol: primary.1)
                    ForEach(Array(secondary.enumerated()), id: \.offset) { _, action in SecondaryButton(title: action.0, symbol: action.1) }
                    Spacer()
                    OverflowButton()
                }
                Tabs(names: tabs, selected: selectedTab)
                content
            }
            .padding(.horizontal, Space.l)
            .padding(.bottom, Space.xl)
            Spacer(minLength: 0)
        }
        .frame(width: 440)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(surface)
    }

    private func navButton(_ symbol: String) -> some View {
        Image(systemName: symbol).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            .frame(width: 28, height: 24).background(fillSubtle, in: Capsule())
    }
}

/// A row that opens its job, company or person in the inspector.
struct EntityRow: View {
    let title: String
    let detail: String
    var chips: [(String, Tone, String?)] = []
    var body: some View {
        HStack(alignment: .center, spacing: Space.s) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                Text(detail).font(.system(size: 11)).foregroundStyle(secondaryInk)
                if !chips.isEmpty {
                    HStack(spacing: Space.xs) {
                        ForEach(Array(chips.enumerated()), id: \.offset) { _, chip in Chip(text: chip.0, tone: chip.1, symbol: chip.2) }
                    }
                }
            }
            Spacer(minLength: 0)
            Image(systemName: "chevron.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(tertiaryInk)
        }
    }
}

struct PersonLine: View {
    let name: String
    let relation: String
    let detail: String
    var body: some View {
        HStack(spacing: Space.s) {
            Text(name).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            Chip(text: relation, tone: .neutral)
            Text(detail).font(.system(size: 12)).foregroundStyle(secondaryInk)
        }
    }
}

struct CompanyInspector: View {
    var body: some View {
        InspectorShell(
            kind: "COMPANY", title: "Initech", facts: "Payments infrastructure · 200–500 people · Remote-first",
            chips: [("Watching", .accent, nil), ("3 open jobs", .neutral, nil), ("Overdue 2 days", .negative, "bell.fill")],
            primary: ("Find jobs", "magnifyingglass"), secondary: [("Messaged someone…", "paperplane")],
            tabs: ["Overview", "Jobs", "People", "Session"], selectedTab: "Jobs"
        ) {
            VStack(alignment: .leading, spacing: Space.s) {
                SectionTitle(text: "Open jobs", accessory: "Found 2 h ago")
                EntityRow(title: "Full-Stack Engineer, Payments", detail: "Applied 25 Sep · in Applied for 9 days",
                          chips: [("Strong match", .positive, nil), ("Passes screen", .positive, "checkmark")])
                Divider()
                EntityRow(title: "Senior Frontend Engineer", detail: "Remote, Americas · posted yesterday",
                          chips: [("Possible match", .accent, nil), ("New", .accent, nil)])
                Divider()
                EntityRow(title: "Data Platform Engineer", detail: "Remote, Americas · posted 4 days ago",
                          chips: [("Mismatch", .neutral, nil), ("Screen unclear", .caution, "questionmark")])
                Text("2 skipped and 1 closed · Show").font(.system(size: 11)).foregroundStyle(Tone.accent.color).padding(.top, Space.xs)
            }
            Divider()
            VStack(alignment: .leading, spacing: Space.s) {
                SectionTitle(text: "Who can get you in", accessory: "People tab")
                PersonLine(name: "Jordan Lee", relation: "Recruiter", detail: "Talent Partner")
                PersonLine(name: "Avery Stone", relation: "Connection", detail: "Product Designer")
            }
        }
    }
}

struct PersonInspector: View {
    var tab = "Conversation"
    var body: some View {
        InspectorShell(
            kind: "PERSON", parent: "INITECH", title: "Jordan Lee", facts: "Talent Partner · wrote 3 days ago on LinkedIn",
            chips: [("Recruiter", .neutral, nil), ("Unanswered", .caution, nil), ("2 jobs fit you", .positive, nil)],
            primary: ("Draft reply", "square.and.pencil"), secondary: [("Open on LinkedIn", "arrow.up.right.square")],
            tabs: ["Overview", "Conversation"], selectedTab: tab
        ) {
            if tab == "Conversation" { conversation } else { overview }
        }
    }

    private var conversation: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            VStack(alignment: .leading, spacing: Space.xs) {
                Text("Hi! I'm hiring for two product engineering roles at Initech, both remote across the Americas. Open to a quick chat this week?")
                    .font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                    .padding(Space.m)
                    .background(well, in: RoundedRectangle(cornerRadius: Radius.card))
                Text("Jordan · 1 Oct").font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
            VStack(alignment: .leading, spacing: Space.s) {
                SectionTitle(text: "Drafted reply", accessory: "by Claude · just now")
                Text("Hi Jordan, thanks for reaching out. The Full-Stack Engineer, Payments role is close to the checkout and payments work I've led, and I've already applied. Tuesday or Wednesday afternoon works for me.")
                    .font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                    .padding(Space.m)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Tone.accent.color.opacity(0.06), in: RoundedRectangle(cornerRadius: Radius.card))
                    .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(Tone.accent.color.opacity(0.25)))
                HStack(spacing: Space.s) {
                    SecondaryButton(title: "Copy", symbol: "doc.on.doc")
                    SecondaryButton(title: "Redraft…", symbol: "arrow.clockwise")
                }
            }
            Divider()
            fittingJobs
        }
    }

    private var overview: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            VStack(alignment: .leading, spacing: Space.s) {
                SectionTitle(text: "What they can do for you")
                Text("Recruits for Initech, which has two open jobs that fit you. You applied to one of them on 25 Sep.")
                    .font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            }
            VStack(alignment: .leading, spacing: Space.xs) {
                FactRow(label: "Relation", value: "Recruiter, in-house")
                FactRow(label: "Company", value: "Initech")
                FactRow(label: "Source", value: "LinkedIn conversation")
                FactRow(label: "Last message", value: "1 Oct, from them")
            }
            Divider()
            fittingJobs
            Divider()
            VStack(alignment: .leading, spacing: Space.s) {
                SectionTitle(text: "Also at Initech", accessory: "Company")
                PersonLine(name: "Avery Stone", relation: "Connection", detail: "Product Designer")
            }
        }
    }

    private var fittingJobs: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            SectionTitle(text: "Jobs that fit you at Initech", accessory: "Company")
            EntityRow(title: "Full-Stack Engineer, Payments", detail: "Applied 25 Sep", chips: [("Strong match", .positive, nil)])
            EntityRow(title: "Senior Frontend Engineer", detail: "Posted yesterday", chips: [("Possible match", .accent, nil), ("New", .accent, nil)])
        }
    }
}

struct CompanyAndPerson: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            VStack(alignment: .leading, spacing: Space.xs) {
                Text("Companies and people open in the same inspector").font(.system(size: 22, weight: .semibold)).foregroundStyle(ink)
                Text("The same header, action bar and tabs as a job. A company gains a Jobs tab (the Mac can't list a company's jobs today), and a recruiter's conversation moves out of its own page.")
                    .font(.system(size: 13)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
            .frame(width: 1060, alignment: .leading)
            HStack(alignment: .top, spacing: 0) {
                VStack(alignment: .leading, spacing: Space.s) {
                    Text("Company · Jobs tab").font(.system(size: 13, weight: .semibold)).foregroundStyle(secondaryInk)
                    CompanyInspector().frame(height: 820).framed()
                }
                VStack(spacing: Space.s) {
                    Text("Click Jordan Lee").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                    Image(systemName: "arrow.right").font(.system(size: 22, weight: .medium)).foregroundStyle(Tone.accent.color)
                    Text("opens here")
                        .font(.system(size: 12)).foregroundStyle(secondaryInk)
                    HStack(spacing: 3) {
                        Keycap(key: "⌘[")
                        Text("back to Initech").font(.system(size: 11)).foregroundStyle(secondaryInk)
                    }
                    .padding(.top, Space.s)
                }
                .frame(width: 180)
                .padding(.top, 430)
                VStack(alignment: .leading, spacing: Space.s) {
                    Text("Person · Conversation tab").font(.system(size: 13, weight: .semibold)).foregroundStyle(secondaryInk)
                    PersonInspector().frame(height: 820).framed()
                }
            }
        }
        .padding(48)
        .background(windowBackground)
    }
}

// MARK: - People

struct PeoplePage: View {
    private let rows: [(String, String, String, String, String, (String, Tone)?)] = [
        ("Jordan Lee", "Recruiter", "Initech", "Talent Partner", "3 days ago", ("Unanswered", .caution)),
        ("Taylor Brooks", "Recruiter", "Umbrella Labs", "Technical Recruiter", "Yesterday", ("Unanswered", .caution)),
        ("Alex Kim", "Connection", "Northwind", "Engineering Manager", "–", nil),
        ("Riley Chen", "Introducer", "Northwind", "Former colleague", "2 weeks ago", nil),
        ("Morgan Diaz", "Contact", "Globex", "Head of Engineering", "–", nil),
        ("Sam Rivera", "Recruiter", "Agency", "Fintech recruiter", "5 days ago", ("Answered", .neutral)),
        ("Casey Park", "Connection", "Umbrella Labs", "Staff Engineer", "–", nil),
        ("Avery Stone", "Connection", "Initech", "Product Designer", "–", nil),
        ("Jamie Fox", "Contact", "Acme Robotics", "CTO", "–", nil),
        ("Quinn Hale", "Introducer", "Acme Robotics", "Former manager", "1 month ago", nil),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Toolbar(title: "People", subtitle: "42 people at 15 companies")
            VStack(alignment: .leading, spacing: Space.l) {
                HStack(spacing: Space.m) {
                    Tabs(names: ["All", "Contacts", "Connections", "Introducers", "Recruiters"], selected: "All")
                        .frame(width: 440)
                    Spacer()
                    filter("Hiring now", on: true)
                    filter("Unanswered", on: false)
                }
                VStack(spacing: 0) {
                    header
                    ForEach(Array(rows.enumerated()), id: \.offset) { index, row in
                        line(row, selected: index == 0)
                            .background(index % 2 == 1 && index != 0 ? fillSubtle.opacity(0.6) : .clear)
                    }
                }
                .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
                .clipShape(RoundedRectangle(cornerRadius: Radius.card))
                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
                Text("Contacts come from company research, connections and recruiters from your LinkedIn import, introducers from what you add on a company. One list, one relation chip each.")
                    .font(.system(size: 11)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
            .padding(Space.xl)
        }
    }

    private func filter(_ title: String, on: Bool) -> some View {
        HStack(spacing: 4) {
            if on { Image(systemName: "checkmark").font(.system(size: 9, weight: .bold)) }
            Text(title).font(.system(size: 12, weight: .medium))
        }
        .foregroundStyle(on ? Tone.accent.color : ink)
        .padding(.horizontal, 10).padding(.vertical, 4)
        .background(on ? Tone.accent.color.opacity(0.12) : fillSubtle, in: Capsule())
    }

    private var header: some View {
        HStack(spacing: 0) {
            column("Name", 130)
            column("Relation", 96)
            column("Company", 116)
            column("Role", 150)
            column("Last message", 96)
            column("", 90)
        }
        .font(.system(size: 11, weight: .semibold))
        .foregroundStyle(secondaryInk)
        .padding(.horizontal, Space.m).frame(height: 30)
        .overlay(alignment: .bottom) { Rectangle().fill(separator).frame(height: 1) }
    }

    private func column(_ title: String, _ width: CGFloat) -> some View {
        Text(title).frame(width: width, alignment: .leading)
    }

    private func line(_ row: (String, String, String, String, String, (String, Tone)?), selected: Bool) -> some View {
        HStack(spacing: 0) {
            Text(row.0).font(.system(size: 12, weight: .medium)).foregroundStyle(ink).frame(width: 130, alignment: .leading)
            HStack { Chip(text: row.1, tone: .neutral) }.frame(width: 96, alignment: .leading)
            Text(row.2).font(.system(size: 12)).foregroundStyle(row.2 == "Agency" ? secondaryInk : Tone.accent.color).frame(width: 116, alignment: .leading)
            Text(row.3).font(.system(size: 12)).foregroundStyle(secondaryInk).frame(width: 150, alignment: .leading).lineLimit(1)
            Text(row.4).font(.system(size: 12)).foregroundStyle(secondaryInk).frame(width: 96, alignment: .leading)
            HStack {
                if let status = row.5 { Chip(text: status.0, tone: status.1) }
            }
            .frame(width: 90, alignment: .leading)
        }
        .padding(.horizontal, Space.m).frame(height: 34)
        .background(selected ? Tone.accent.color.opacity(0.14) : .clear)
    }
}

struct MacWindowPeople: View {
    var body: some View {
        Window {
            HStack(spacing: 0) {
                ProposedSidebar(selected: "People")
                PeoplePage().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                PersonInspector(tab: "Overview")
                    .overlay(alignment: .leading) { Rectangle().fill(separator).frame(width: 1) }
            }
            .frame(width: 1440, height: 900)
        }
    }
}

// MARK: - Jump anywhere

struct Palette: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: Space.s) {
                Image(systemName: "magnifyingglass").font(.system(size: 16)).foregroundStyle(secondaryInk)
                Text("north").font(.system(size: 18)).foregroundStyle(ink)
                Rectangle().fill(Tone.accent.color).frame(width: 2, height: 20)
                Spacer()
                Keycap(key: "esc")
            }
            .padding(.horizontal, Space.l).frame(height: 52)
            Divider()
            VStack(alignment: .leading, spacing: 2) {
                group("Jobs")
                result("briefcase", job("Senior Product Engineer"), chips: [("Strong match", .positive)], hint: "↩ Open", selected: true)
                result("briefcase", job("Staff Platform Engineer"), chips: [("Possible match", .accent)])
                group("Companies")
                result("building.2", northwind(ink), detail: "3 open jobs · watching")
                group("People")
                result("person", Text("Alex Kim").foregroundColor(ink), detail: "Connection at Northwind")
                result("person", Text("Riley Chen").foregroundColor(ink), detail: "Introducer at Northwind and Globex")
                group("Actions")
                result("plus.circle", Text("Add a job by URL…").foregroundColor(ink), hint: "⌘N")
                result("magnifyingglass.circle", Text("Find jobs at ").foregroundColor(ink) + northwind(ink))
                result("doc.badge.plus", Text("Generate missing CVs").foregroundColor(ink))
                result("pause.circle", Text("Pause local models").foregroundColor(ink))
            }
            .padding(Space.s)
            Divider()
            HStack(spacing: Space.l) {
                hint("↑ ↓", "move")
                hint("↩", "open in the inspector")
                hint("⌘↩", "open in a window")
                Spacer()
            }
            .padding(.horizontal, Space.l).frame(height: 38)
        }
        .frame(width: 640)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
        .shadow(color: .black.opacity(0.35), radius: 40, y: 18)
    }

    /// "Northwind" with the typed "north" in bold.
    private func northwind(_ color: Color) -> Text {
        Text("North").fontWeight(.bold).foregroundColor(color) + Text("wind").foregroundColor(color)
    }

    private func job(_ title: String) -> Text {
        Text(title + "  ").foregroundColor(ink) + northwind(secondaryInk)
    }

    private func group(_ title: String) -> some View {
        Text(title).font(.system(size: 11, weight: .semibold)).foregroundStyle(tertiaryInk)
            .padding(.horizontal, Space.s).padding(.top, Space.s).padding(.bottom, 2)
    }

    private func result(_ symbol: String, _ title: Text, detail: String? = nil, chips: [(String, Tone)] = [], hint: String? = nil, selected: Bool = false) -> some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 13)).foregroundStyle(Tone.accent.color).frame(width: 20)
            title.font(.system(size: 13))
            if let detail { Text(detail).font(.system(size: 12)).foregroundStyle(secondaryInk) }
            ForEach(Array(chips.enumerated()), id: \.offset) { _, chip in Chip(text: chip.0, tone: chip.1) }
            Spacer()
            if let hint { Text(hint).font(.system(size: 11, weight: .medium)).foregroundStyle(secondaryInk) }
        }
        .padding(.horizontal, Space.s).frame(height: 30)
        .background(selected ? Tone.accent.color.opacity(0.12) : .clear, in: RoundedRectangle(cornerRadius: Radius.control))
    }

    private func hint(_ key: String, _ text: String) -> some View {
        HStack(spacing: Space.xs) {
            Keycap(key: key)
            Text(text).font(.system(size: 11)).foregroundStyle(secondaryInk)
        }
    }
}

struct MacWindowPalette: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Window {
                ZStack(alignment: .top) {
                    HStack(spacing: 0) {
                        ProposedSidebar()
                        TodayPage().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                        JobInspector()
                    }
                    .frame(width: 1440, height: 900)
                    Palette().padding(.top, 110)
                }
            }
            HStack(spacing: Space.xl) {
                Text("Keyboard decisions").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                keys("P", "Pursue")
                keys("L", "Later")
                keys("S", "Skip…")
                keys("↑ ↓", "next or previous job")
                keys("⌘[ ⌘]", "inspector history")
                keys("⌘N", "Add menu")
                Spacer()
            }
            .padding(.horizontal, 56).padding(.bottom, 36)
            .background(desk)
        }
    }

    private func keys(_ key: String, _ text: String) -> some View {
        HStack(spacing: Space.xs) {
            Keycap(key: key)
            Text(text).font(.system(size: 12)).foregroundStyle(ink)
        }
    }
}

// MARK: - Settings window and Criteria page

struct Token: View {
    let text: String
    var body: some View {
        Text(text).font(.system(size: 12)).foregroundStyle(ink).fixedSize()
            .padding(.horizontal, 7).padding(.vertical, 2)
            .background(Tone.accent.color.opacity(0.1), in: RoundedRectangle(cornerRadius: 5))
    }
}

struct CriteriaPage: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Toolbar(title: "Criteria", subtitle: "What the hub searches for, and screens every job against")
            HStack(alignment: .top, spacing: Space.l) {
                VStack(spacing: Space.l) {
                    Card(title: "The work") {
                        field("Roles") { Token(text: "Senior Full-Stack Engineer"); Token(text: "Product Engineer") }
                        field("Levels") { Token(text: "Senior"); Token(text: "Staff") }
                        field("Technologies") { Token(text: "TypeScript"); Token(text: "React"); Token(text: "Node.js") }
                        field("Search terms") { Token(text: "react"); Token(text: "typescript") }
                        field("Rules a title out") { Token(text: "Sales"); Token(text: "Manager"); Token(text: "Analyst") }
                    }
                    Card(title: "Pipeline phases", accessory: "Follow up after") {
                        phase("Applied", 7)
                        phase("Screening", 5)
                        phase("Interviewing", 4)
                        phase("Offer", 3)
                        HStack(spacing: Space.s) {
                            Image(systemName: "plus").font(.system(size: 11, weight: .semibold))
                            Text("Add a phase").font(.system(size: 12))
                        }
                        .foregroundStyle(Tone.accent.color)
                    }
                }
                VStack(spacing: Space.l) {
                    Card(title: "Where you can work") {
                        field("Home country") { Text("Brazil").font(.system(size: 12)).foregroundStyle(ink) }
                        field("Hires from") { Token(text: "Brazil"); Token(text: "LATAM"); Token(text: "Americas"); Token(text: "Worldwide") }
                        field("Rules me out") { Token(text: "must reside in the US") }
                        field("Hours that work") { Token(text: "US business hours"); Token(text: "EST") }
                        field("Hours that don't") { Token(text: "APAC"); Token(text: "AEST") }
                        field("Hourly work") { Switch(isOn: true); Text("Refuse it").font(.system(size: 12)).foregroundStyle(secondaryInk) }
                    }
                    Card(title: "Take-home pay") {
                        field("Judge by take-home") { Switch(isOn: true) }
                        field("Currency") { Text("BRL").font(.system(size: 12)).foregroundStyle(ink) }
                        field("Minimum a month") { Text("25,000").font(.system(size: 12)).monospacedDigit().foregroundStyle(ink) }
                        field("Target a month") { Text("31,000").font(.system(size: 12)).monospacedDigit().foregroundStyle(ink) }
                        Text("Below the minimum, a job fails the Pay screen.").font(.system(size: 11)).foregroundStyle(secondaryInk)
                    }
                }
            }
            .padding(Space.xl)
        }
    }

    private func field(_ label: String, @ViewBuilder content: () -> some View) -> some View {
        HStack(alignment: .center, spacing: Space.m) {
            Text(label).font(.system(size: 12)).foregroundStyle(secondaryInk).frame(width: 118, alignment: .trailing)
            HStack(spacing: Space.xs) { content() }
            Spacer(minLength: 0)
        }
    }

    private func phase(_ name: String, _ days: Int) -> some View {
        HStack(spacing: Space.s) {
            Image(systemName: "line.3.horizontal").font(.system(size: 11)).foregroundStyle(tertiaryInk)
            Text(name).font(.system(size: 12)).foregroundStyle(ink)
            Spacer()
            Text("\(days) days").font(.system(size: 12)).monospacedDigit().foregroundStyle(secondaryInk)
        }
    }
}

struct SettingsWindow: View {
    private let tabs = [("Connection", "network"), ("Server", "server.rack"), ("Accounts", "person.crop.circle"), ("Phones", "iphone"), ("Models", "cpu")]

    var body: some View {
        VStack(spacing: 0) {
            ZStack {
                HStack { TrafficLights(); Spacer() }
                Text("Connection").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
            }
            .padding(.horizontal, 14).frame(height: 36)
            HStack(spacing: Space.xs) {
                ForEach(tabs, id: \.0) { tab in
                    VStack(spacing: 3) {
                        Image(systemName: tab.1).font(.system(size: 17))
                        Text(tab.0).font(.system(size: 11))
                    }
                    .foregroundStyle(tab.0 == "Connection" ? Tone.accent.color : secondaryInk)
                    .frame(width: 84, height: 50)
                    .background(tab.0 == "Connection" ? fill : .clear, in: RoundedRectangle(cornerRadius: Radius.control + 2))
                }
            }
            .padding(.bottom, Space.s)
            Divider()
            VStack(alignment: .leading, spacing: Space.l) {
                group {
                    row("Hub address") { Text(verbatim: "http://localhost:8090").font(.system(size: 12)).foregroundStyle(ink) }
                    Divider()
                    row("Owner token") {
                        Text("Stored in the Keychain").font(.system(size: 12)).foregroundStyle(secondaryInk)
                        Spacer()
                        SecondaryButton(title: "Change…")
                    }
                }
                group {
                    row("Status") {
                        Lamp(color: Tone.positive.color)
                        Text("Connected · live updates on").font(.system(size: 12)).foregroundStyle(ink)
                    }
                    Divider()
                    row("This Mac") { Text("Serves the hub; start and stop it in Server").font(.system(size: 12)).foregroundStyle(secondaryInk) }
                }
                Text("Settings is for how the app connects. What you search for moved to Criteria in the sidebar.")
                    .font(.system(size: 11)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
            .padding(Space.xl)
            Spacer(minLength: 0)
        }
        .frame(width: 600, height: 370)
    }

    private func group(@ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.s) { content() }
            .padding(Space.m)
            .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
            .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }

    private func row(_ label: String, @ViewBuilder content: () -> some View) -> some View {
        HStack(spacing: Space.s) {
            Text(label).font(.system(size: 12)).foregroundStyle(ink).frame(width: 110, alignment: .leading)
            content()
            Spacer(minLength: 0)
        }
    }
}

struct ConnectionBanner: View {
    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: "network.slash").foregroundStyle(Tone.caution.color)
            Text("Can't reach the hub at localhost:8090").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            Spacer()
            Text("Start server").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.color)
            Text("Settings…").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.color)
        }
        .padding(.horizontal, Space.m).padding(.vertical, Space.s)
        .background(Tone.caution.color.opacity(0.1), in: RoundedRectangle(cornerRadius: Radius.card))
    }
}

struct SettingsAndCriteria: View {
    var body: some View {
        HStack(alignment: .top, spacing: 48) {
            VStack(alignment: .leading, spacing: Space.m) {
                BoardHeading(title: "Criteria: its own page, under You",
                             subtitle: "Job criteria, take-home pay and pipeline phases leave Settings: they change what every page shows.")
                HStack(spacing: 0) {
                    ProposedSidebar(selected: "Criteria")
                    VStack(spacing: 0) {
                        CriteriaPage()
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                }
                .frame(width: 1180, height: 760)
                .framed()
            }
            VStack(alignment: .leading, spacing: Space.xl) {
                VStack(alignment: .leading, spacing: Space.m) {
                    BoardHeading(title: "Settings: the standard window (⌘,)",
                                 subtitle: "Five tabs in place of one form with eleven sections.")
                    SettingsWindow().framed()
                }
                VStack(alignment: .leading, spacing: Space.m) {
                    BoardHeading(title: "Connection: one banner per window",
                                 subtitle: "Above every page when the hub can't be reached, instead of a \"Not connected\" page on each of ten screens.")
                    ConnectionBanner().frame(width: 600)
                }
                .frame(width: 600, alignment: .leading)
                VStack(alignment: .leading, spacing: Space.s) {
                    Text("Where each Settings section goes").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                    ForEach(moves, id: \.0) { move in
                        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                            Text(move.0).font(.system(size: 12)).foregroundStyle(secondaryInk).frame(width: 200, alignment: .leading)
                            Image(systemName: "arrow.right").font(.system(size: 9, weight: .semibold)).foregroundStyle(tertiaryInk)
                            Text(move.1).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                        }
                    }
                }
                .frame(width: 600, alignment: .leading)
            }
        }
        .padding(48)
        .background(desk)
    }

    private let moves = [
        ("Hub, Status", "Settings › Connection"),
        ("Server, Session files", "Settings › Server"),
        ("Google, Network (LinkedIn import)", "Settings › Accounts"),
        ("Phones", "Settings › Phones"),
        ("Models", "Settings › Models"),
        ("Job criteria, Take-home pay", "Criteria page"),
        ("Pipeline phases", "Criteria page"),
    ]
}

// MARK: - Android system surfaces

struct AndroidSystemBoard: View {
    private let screens = AndroidScreens()

    var body: some View {
        HStack(alignment: .top, spacing: 40) {
            screens.labelled("Notifications", "A channel per kind; act on a reminder from the shade") { notifications }
            screens.labelled("Followed up…", "A modal bottom sheet, from the swipe or the card's ⋮ menu") { followUpSheet }
            screens.labelled("Undo", "Skip from Today; the snackbar takes it back") { undo }
            screens.labelled("Predictive back", "Swipe from either edge to see where back goes before letting go") { predictiveBack }
        }
        .padding(48)
        .background(Color(hex: 0xE8E8EE))
    }

    private var notifications: some View {
        AndroidPhone(background: M3.containerHighest) {
            VStack(alignment: .leading, spacing: 12) {
                HStack(alignment: .lastTextBaseline) {
                    Text("10:24").font(M3Type.roboto(44)).foregroundStyle(M3.onSurface)
                    Spacer()
                    Text("Sun, 4 Oct").font(M3Type.titleSmall).foregroundStyle(M3.onSurfaceVariant)
                }
                HStack(spacing: 8) {
                    tile("wifi", "Internet", "Home", on: true)
                    tile("bluetooth", "Bluetooth", "Off", on: false)
                }
                HStack(spacing: 8) {
                    ForEach(["flashlight_on", "do_not_disturb_on", "dark_mode", "settings"], id: \.self) { icon in
                        MSymbol(icon).foregroundStyle(M3.onSurfaceVariant).frame(maxWidth: .infinity).frame(height: 56)
                            .background(M3.containerLow, in: Capsule())
                    }
                }
                VStack(spacing: 2) {
                    notification("Follow-ups", "Follow up with Initech", "Full-Stack Engineer, Payments is 2 days overdue.", actions: ["Followed up", "Snooze a day"], first: true)
                    notification("Replies", "Reply from Umbrella Labs", "“Could you do a call on Tuesday…”", actions: ["Open", "Mark as read"])
                    notification("Matches", "2 fresh strong matches", "Northwind and Globex", actions: [], last: true)
                }
                HStack {
                    M3Button(title: "Manage", style: .tonal)
                    Spacer()
                    M3Button(title: "Clear all", style: .tonal)
                }
            }
            .frame(width: 336)
            .padding(.horizontal, 12)
            .padding(.top, 8)
        }
    }

    private func tile(_ icon: String, _ title: String, _ detail: String, on: Bool) -> some View {
        HStack(spacing: 10) {
            MSymbol(icon, fill: on)
            VStack(alignment: .leading, spacing: 0) {
                Text(title).font(M3Type.titleSmall)
                Text(detail).font(M3Type.bodySmall)
            }
            Spacer(minLength: 0)
        }
        .foregroundStyle(on ? M3.onPrimary : M3.onSurfaceVariant)
        .padding(.horizontal, 16)
        .frame(height: 64)
        .background(on ? M3.primary : M3.containerLow, in: RoundedRectangle(cornerRadius: 20))
    }

    private func notification(_ channel: String, _ title: String, _ text: String, actions: [String], first: Bool = false, last: Bool = false) -> some View {
        HStack(alignment: .top, spacing: 12) {
            MSymbol("hub", size: 20, fill: true).foregroundStyle(M3.onPrimary).frame(width: 32, height: 32).background(M3.primary, in: Circle())
            VStack(alignment: .leading, spacing: 4) {
                Text("Job Search Hub · \(channel) · now").font(M3Type.labelMedium).foregroundStyle(M3.onSurfaceVariant)
                Text(title).font(M3Type.titleSmall).foregroundStyle(M3.onSurface)
                Text(text).font(M3Type.bodyMedium).foregroundStyle(M3.onSurfaceVariant).lineLimit(2).fixedSize(horizontal: false, vertical: true)
                if !actions.isEmpty {
                    HStack(spacing: 8) {
                        ForEach(actions, id: \.self) { action in
                            Text(action).font(M3Type.labelLarge).foregroundStyle(M3.onSecondaryContainer).lineLimit(1).fixedSize()
                                .padding(.horizontal, 14).frame(height: 32).background(M3.secondaryContainer, in: Capsule())
                        }
                    }
                    .padding(.top, 6)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            MSymbol("expand_more", size: 20).foregroundStyle(M3.onSurfaceVariant).frame(width: 28, height: 28).background(M3.containerHigh, in: Circle())
        }
        .padding(16)
        .frame(width: 336)
        .background(M3.item)
        .clipShape(UnevenRoundedRectangle(topLeadingRadius: first ? 24 : 6, bottomLeadingRadius: last ? 24 : 6,
                                          bottomTrailingRadius: last ? 24 : 6, topTrailingRadius: first ? 24 : 6))
    }

    private var followUpSheet: some View {
        AndroidPhone(nav: "Pipeline") { screens.pipelineContent(swiped: false) }
            .overlay {
                ZStack(alignment: .bottom) {
                    M3.scrim
                    M3BottomSheet {
                        VStack(alignment: .leading, spacing: 16) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text("Followed up with Acme Robotics").font(M3Type.titleLarge).foregroundStyle(M3.onSurface)
                                Text("Outreach · 7 days in Applied").font(M3Type.bodyMedium).foregroundStyle(M3.onSurfaceVariant)
                            }
                            Text("Next follow-up").font(M3Type.titleSmall).foregroundStyle(M3.onSurface)
                            HStack(spacing: 8) {
                                M3FilterChip(title: "Tomorrow")
                                M3FilterChip(title: "In 3 days", selected: true)
                                M3FilterChip(title: "In a week")
                            }
                            M3AssistChip(title: "Pick a date", icon: "event")
                            ZStack(alignment: .topLeading) {
                                RoundedRectangle(cornerRadius: M3Shape.extraSmall).strokeBorder(M3.outline)
                                    .frame(height: 56)
                                    .overlay(alignment: .leading) {
                                        Text("Sent a short note to the hiring manager").font(M3Type.bodyLarge).foregroundStyle(M3.onSurface).lineLimit(1).padding(.horizontal, 16)
                                    }
                                Text("Note").font(M3Type.bodySmall).foregroundStyle(M3.onSurfaceVariant)
                                    .padding(.horizontal, 4).background(M3.containerLow).offset(x: 12, y: -8)
                            }
                            HStack(spacing: 8) {
                                Spacer()
                                M3Button(title: "Cancel", style: .text)
                                M3Button(title: "Save")
                            }
                        }
                        .padding(.horizontal, 24)
                        .padding(.bottom, 8)
                    }
                }
                .clipShape(RoundedRectangle(cornerRadius: 34, style: .continuous))
                .padding(6)
            }
    }

    private var undo: some View {
        AndroidPhone(nav: "Today") { screens.todayContent(skipped: true) }
            .overlay(alignment: .bottom) {
                M3Snackbar(text: "Skipped Staff Frontend Engineer", action: "Undo")
                    .padding(.horizontal, 18)
                    .padding(.bottom, 6 + 100)
            }
    }

    private var predictiveBack: some View {
        AndroidPhone(nav: "Today") { screens.todayContent(skipped: false) }
            .overlay {
                ZStack(alignment: .leading) {
                    Color.black.opacity(0.12)
                    VStack(spacing: 0) {
                        AndroidStatusBar()
                        screens.jobContent
                        GestureHandle()
                    }
                    .frame(width: 360, height: 780)
                    .background(M3.page)
                    .clipShape(RoundedRectangle(cornerRadius: 32, style: .continuous))
                    .shadow(color: .black.opacity(0.25), radius: 16, x: 0, y: 6)
                    .scaleEffect(0.86)
                    .offset(x: 26)
                    MSymbol("arrow_back", size: 20).foregroundStyle(M3.onSurface)
                        .frame(width: 40, height: 40).background(M3.containerHighest.opacity(0.95), in: Circle())
                        .shadow(color: .black.opacity(0.15), radius: 3, y: 1)
                        .padding(.leading, 4)
                }
                .clipShape(RoundedRectangle(cornerRadius: 34, style: .continuous))
                .padding(6)
            }
    }
}

// MARK: - Android on large screens, and dark

struct AndroidFoldableBoard: View {
    /// Today on a phone in dark theme, drawn on its own because the board is light.
    let darkPhone: NSImage
    private let screens = AndroidScreens()

    var body: some View {
        HStack(alignment: .top, spacing: 48) {
            VStack(alignment: .leading, spacing: Space.m) {
                BoardHeading(title: "Unfolded, on a Galaxy Z Fold or a tablet",
                             subtitle: "The navigation bar becomes a rail, and Today and the job sit side by side: list and detail, like the Mac's list and inspector.")
                fold
                VStack(alignment: .leading, spacing: Space.s) {
                    PlatformNote(text: "NavigationSuiteScaffold picks the bar or the rail from the window size class; ListDetailPaneScaffold shows one pane or two.")
                    PlatformNote(text: "The handle between the panes resizes them; folding the phone keeps the open job and its scroll position.")
                    PlatformNote(text: "With no back arrow in the detail pane, back closes the job on a phone and moves focus to the list here.")
                }
                .frame(width: 880, alignment: .leading)
            }
            VStack(alignment: .leading, spacing: Space.m) {
                BoardHeading(title: "Dark theme", subtitle: "The same scheme's dark roles; tones keep their meaning.")
                Image(nsImage: darkPhone)
            }
            .frame(width: 372)
        }
        .padding(48)
        .background(Color(hex: 0xE8E8EE))
    }

    private var fold: some View {
        VStack(spacing: 0) {
            AndroidStatusBar()
            HStack(alignment: .top, spacing: 0) {
                rail
                screens.todayContent(skipped: false, selected: true)
                    .frame(width: 360, height: 680, alignment: .top).clipped()
                Capsule().fill(M3.outline).frame(width: 4, height: 48).frame(width: 24, height: 680)
                screens.jobContent(back: false)
                    .frame(height: 672)
                    .background(M3.containerLow)
                    .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
                    .padding(.trailing, 16)
                    .padding(.bottom, 8)
            }
            .frame(height: 740 - 36 - 24, alignment: .top)
            .clipped()
            GestureHandle()
        }
        .frame(width: 880, height: 740)
        .background(M3.page)
        .overlay {
            LinearGradient(colors: [.clear, .black.opacity(0.05), .clear], startPoint: .leading, endPoint: .trailing).frame(width: 24)
        }
        .clipShape(RoundedRectangle(cornerRadius: 26, style: .continuous))
        .padding(6)
        .background(Color(hex: 0x1F1F22), in: RoundedRectangle(cornerRadius: 32, style: .continuous))
    }

    private var rail: some View {
        VStack(spacing: 12) {
            M3IconButton("menu").foregroundStyle(M3.onSurfaceVariant).padding(.top, 8)
            ForEach(M3NavigationBar.destinations, id: \.0) { item in
                NavigationItem(title: item.0, icon: item.1, selected: item.0 == "Today", badge: M3NavigationBar.badge(item.0))
            }
            Spacer()
        }
        .frame(width: 96)
    }
}

// MARK: - Android components

struct AndroidComponentsBoard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            BoardHeading(title: "Android: Material 3 Expressive components, not iOS ones",
                         subtitle: "Each shared component as Android builds it, and the iOS habit it replaces. The scheme comes from the Hub Indigo seed; type is Roboto; icons are Material Symbols Rounded.")
            HStack(alignment: .top, spacing: 20) {
                cell("Color", "Scheme from the seed", not: "Hub tones at 13% on white") { colors }
                cell("Type", "Material 3 type scale", not: "SF Pro sizes") { type }
                cell("Shape", "Material 3 shape scale", not: "Mac radii of 6, 10 and 14") { shapes }
                cell("Icons", "Material Symbols Rounded", not: "SF Symbols") { icons }
            }
            HStack(alignment: .top, spacing: 20) {
                cell("ToneChip", "Label in a tone container", not: "capsules at 13% tint") {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 8) { M3Label(text: "Strong match", tone: .positive, icon: "thumb_up"); M3Label(text: "Possible", tone: .accent) }
                        HStack(spacing: 8) { M3Label(text: "Stretch", tone: .caution); M3Label(text: "Overdue 2 days", tone: .negative); M3Label(text: "Skipped", tone: .neutral) }
                    }
                }
                cell("ActionBar", "Button group, docked at the bottom", not: "rows of equal buttons at the top") {
                    HStack(spacing: 8) {
                        M3Button(title: "Skip", style: .outlined, medium: true)
                        M3Button(title: "Later", style: .tonal, medium: true)
                        M3Button(title: "Pursue", icon: "check", medium: true)
                    }
                }
                cell("Overflow", "⋮ icon button and a menu", not: "a horizontal … button") {
                    HStack(alignment: .top, spacing: 8) {
                        M3IconButton("more_vert").background(M3.secondaryContainer.opacity(0.6), in: Circle())
                        M3Menu(items: [("Open posting", "open_in_new"), ("Fix…", "build"), ("Share", "share"), ("Skip…", "close")])
                    }
                }
                cell("Lists", "List items in segmented groups", not: "chevron rows under ALL-CAPS headers") {
                    VStack(spacing: 0) {
                        GroupHeading(title: "Screen").padding(.horizontal, -12)
                        SegmentedGroup(items: [
                            AnyView(M3ListItem(headline: "Where they hire", supporting: "Americas") { MSymbol("check_circle", fill: true).foregroundStyle(Tone.positive.m3) } trailing: { EmptyView() }),
                            AnyView(M3ListItem(headline: "Years", supporting: "Asks 6+; you have 5") { MSymbol("help", fill: true).foregroundStyle(Tone.caution.m3) } trailing: { EmptyView() }),
                        ])
                        .padding(.horizontal, -12)
                    }
                }
            }
            HStack(alignment: .top, spacing: 20) {
                cell("Switch", "Switch with a thumb icon", not: "the iOS toggle") {
                    VStack(spacing: 2) {
                        AnyView(M3ListItem(headline: "Follow-ups due") { EmptyView() } trailing: { M3Switch(isOn: true) }).background(M3.item)
                        AnyView(M3ListItem(headline: "Match wallpaper colors") { EmptyView() } trailing: { M3Switch(isOn: false) }).background(M3.item)
                    }
                    .clipShape(RoundedRectangle(cornerRadius: 20))
                }
                cell("Confirm", "Dialog: icon, headline, text buttons at the end", not: "an alert with a red, stacked destructive button") {
                    M3Dialog(icon: "link_off", headline: "Unpair this phone?", text: "It stops getting updates until you pair it again.",
                             dismiss: "Cancel", confirm: "Unpair", destructive: true)
                        .scaleEffect(0.9).frame(height: 230)
                }
                cell("Toast", "Snackbar with Undo", not: "a floating capsule notice") {
                    VStack(spacing: 12) {
                        M3Snackbar(text: "Skipped Staff Frontend Engineer", action: "Undo")
                        M3Snackbar(text: "Sent to the Mac. The result comes as an update.")
                    }
                }
                cell("Navigation", "Navigation bar; rail when unfolded", not: "a tab bar of thin outline icons") {
                    M3NavigationBar(selected: "Today", handle: false).clipShape(RoundedRectangle(cornerRadius: 16))
                }
            }
            HStack(alignment: .top, spacing: 20) {
                cell("Page title", "Small and large flexible top app bars", not: "a centered title with text buttons") {
                    VStack(spacing: 8) {
                        M3TopBar(title: "Pipeline") { M3IconButton("search"); HubAvatar() }.background(M3.page)
                        M3TopBar(title: "Today", subtitle: "7 to decide · 1 overdue", size: .large) { M3IconButton("search") }
                            .frame(height: 150, alignment: .bottom).clipped()
                    }
                }
                cell("Search", "Search bar, then a full-screen search view", not: "a grey field with a Cancel button") {
                    M3SearchBar(placeholder: "Search jobs, companies, people")
                }
                cell("AsyncButton", "Wavy progress and the loading indicator", not: "a spinner beside a hand-made label") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("Writing the brief…").font(M3Type.titleSmall).foregroundStyle(M3.onSurface)
                        WavyProgress()
                        Text("Claude is reading the posting").font(M3Type.bodySmall).foregroundStyle(M3.onSurfaceVariant)
                    }
                }
                cell("Filters", "Tabs and filter chips", not: "a segmented control") {
                    VStack(alignment: .leading, spacing: 12) {
                        M3Tabs(tabs: ["Applied 4", "Screening 2", "Interviewing 1"], selected: "Applied 4", scrollable: true).padding(.horizontal, -16)
                        HStack(spacing: 8) { M3FilterChip(title: "Follow-up due", selected: true); M3FilterChip(title: "Agency") }
                    }
                }
            }
        }
        .padding(48)
        .background(windowBackground)
    }

    private func cell(_ hub: String, _ material: String, not: String, @ViewBuilder demo: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(spacing: 6) {
                Text(hub).font(.system(size: 12, weight: .semibold).monospaced()).foregroundStyle(Tone.accent.color)
                Image(systemName: "arrow.right").font(.system(size: 10, weight: .semibold)).foregroundStyle(tertiaryInk)
                Text(material).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink).lineLimit(1)
            }
            demo()
                .frame(maxWidth: .infinity, minHeight: 200)
                .padding(16)
                .background(M3.page, in: RoundedRectangle(cornerRadius: Radius.card))
            HStack(spacing: 4) {
                Image(systemName: "nosign").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.negative.color)
                Text("Not \(not)").font(.system(size: 11)).foregroundStyle(secondaryInk)
            }
        }
        .padding(Space.l)
        .frame(width: 400)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }

    private var colors: some View {
        let swatches: [(String, Color, Color)] = [
            ("primary", M3.primary, M3.onPrimary), ("primaryContainer", M3.primaryContainer, M3.onPrimaryContainer),
            ("secondaryContainer", M3.secondaryContainer, M3.onSecondaryContainer), ("surfaceContainer", M3.page, M3.onSurface),
            ("positive container", Tone.positive.m3Container, Tone.positive.m3OnContainer), ("caution container", Tone.caution.m3Container, Tone.caution.m3OnContainer),
            ("errorContainer", Tone.negative.m3Container, Tone.negative.m3OnContainer), ("inverseSurface", M3.inverseSurface, M3.inverseOnSurface),
        ]
        return LazyVGrid(columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)], spacing: 8) {
            ForEach(swatches, id: \.0) { swatch in
                Text(swatch.0).font(M3Type.labelSmall).foregroundStyle(swatch.2).lineLimit(1)
                    .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 8).frame(height: 36)
                    .background(swatch.1, in: RoundedRectangle(cornerRadius: M3Shape.small))
                    .overlay(RoundedRectangle(cornerRadius: M3Shape.small).strokeBorder(M3.outlineVariant.opacity(swatch.0 == "surfaceContainer" ? 1 : 0)))
            }
        }
    }

    private var type: some View {
        VStack(alignment: .leading, spacing: 8) {
            specimen("headlineMedium 28", "Senior Product…", M3Type.headlineMedium)
            specimen("titleMedium 16", "Brief", M3Type.titleMedium)
            specimen("bodyLarge 16", "Where they hire", M3Type.bodyLarge)
            specimen("bodyMedium 14", "Northwind · Remote", M3Type.bodyMedium)
            specimen("labelMedium 12", "Strong match", M3Type.labelMedium)
        }
    }

    private func specimen(_ role: String, _ sample: String, _ font: Font) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(sample).font(font).foregroundStyle(M3.onSurface).lineLimit(1)
            Spacer()
            Text(role).font(.system(size: 10).monospaced()).foregroundStyle(M3.onSurfaceVariant)
        }
    }

    private var shapes: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 12), count: 3), spacing: 12) {
            ForEach([("XS 4", M3Shape.extraSmall), ("S 8", M3Shape.small), ("M 12", M3Shape.medium), ("L 16", M3Shape.large), ("XL 28", M3Shape.extraLarge), ("Full", 40)], id: \.0) { shape in
                VStack(spacing: 4) {
                    RoundedRectangle(cornerRadius: shape.1).fill(M3.primaryContainer).frame(width: 64, height: 56)
                    Text(shape.0).font(.system(size: 10).monospaced()).foregroundStyle(M3.onSurfaceVariant)
                }
            }
        }
    }

    private var icons: some View {
        VStack(alignment: .leading, spacing: 12) {
            LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 6), spacing: 14) {
                ForEach(["home", "fact_check", "view_kanban", "work", "search", "more_vert", "arrow_back", "open_in_new", "check_circle", "help", "link_off", "hub"], id: \.self) { icon in
                    MSymbol(icon, fill: ["home", "check_circle", "help", "hub"].contains(icon)).foregroundStyle(M3.onSurfaceVariant)
                }
            }
            Text("24 dp, outlined; filled when selected or for a verdict").font(M3Type.bodySmall).foregroundStyle(M3.onSurfaceVariant)
        }
    }
}

// MARK: - Android job before and after

struct AndroidJobBeforeAfter: View {
    /// A wallpaper-driven dynamic scheme, as Android 12+ picks it today.
    private let wallpaperPrimary = Color(hex: 0x4C662B)
    private let wallpaperSurface = Color(hex: 0xF9FAEF)
    private let wallpaperVariant = Color(hex: 0x44483D)
    private let wallpaperOutline = Color(hex: 0x75796C)

    var body: some View {
        HStack(alignment: .top, spacing: 48) {
            VStack(alignment: .leading, spacing: Space.m) {
                BoardHeading(title: "Today", subtitle: "The wallpaper's colors, two rows of buttons, Fit and Match apart")
                legacyJob
                notes([
                    (1, "Two rows of buttons at the top, two of them filled: Open posting competes with Pursue, out of thumb reach."),
                    (2, "Colors come from the wallpaper, so the hub has no look of its own, and the tones are hard-coded hex."),
                    (3, "Match in the brief and Fit further down, as on the Mac, in one long scroll with no tabs."),
                ])
            }
            .frame(width: 380)
            VStack(alignment: .leading, spacing: Space.m) {
                BoardHeading(title: "Proposed", subtitle: "Material 3 Expressive in Hub Indigo, the same words as the Mac")
                AndroidScreens().job
                VStack(alignment: .leading, spacing: Space.s) {
                    PlatformNote(text: "Medium flexible top app bar: back, Open posting and ⋮ (Fix…, Share, Skip with a reason).")
                    PlatformNote(text: "The company is an assist chip that opens it; status words are M3 labels in tone containers.")
                    PlatformNote(text: "Overview, People and Posting are primary tabs, the same split as the Mac's inspector.")
                    PlatformNote(text: "Skip, Later and Pursue are one medium button group, docked at the bottom where the thumb is.")
                }
                .frame(width: 372)
            }
        }
        .padding(48)
        .background(Color(hex: 0xE8E8EE))
    }

    private var legacyJob: some View {
        AndroidPhone(background: wallpaperSurface) {
            HStack(spacing: 0) {
                MSymbol("arrow_back").frame(width: 48, height: 48)
                Text("Northwind").font(M3Type.titleLarge).padding(.leading, 4)
                Spacer()
            }
            .foregroundStyle(M3.onSurface)
            .padding(.horizontal, 4).frame(height: 64)
            VStack(alignment: .leading, spacing: 12) {
                Text("Senior Product Engineer").font(M3Type.titleLarge).foregroundStyle(M3.onSurface)
                Text("Northwind · Remote, Americas · Remote").font(M3Type.bodyLarge).foregroundStyle(wallpaperVariant)
                HStack(spacing: 8) {
                    filled("Open posting")
                    outlined("Company brief")
                    outlined("Fix…")
                }
                .overlay(alignment: .topLeading) { Marker(number: 1).offset(x: -26, y: 10) }
                HStack(spacing: 8) {
                    filled("Pursue")
                    outlined("Skip")
                    outlined("Later")
                }
                HStack(spacing: 8) {
                    Text("Brief").font(M3Type.roboto(16, .medium)).foregroundStyle(M3.onSurface)
                    Text("Strong").font(M3Type.labelLarge).foregroundStyle(Color(hex: 0x2E9E4F))
                    Text("by Claude").font(M3Type.bodySmall).foregroundStyle(wallpaperVariant)
                }
                .overlay(alignment: .topLeading) { Marker(number: 3).offset(x: -26) }
                Text("Your payments cases map onto their checkout rebuild, and the take-home clears your target.")
                    .font(M3Type.bodyLarge).foregroundStyle(M3.onSurface).fixedSize(horizontal: false, vertical: true)
                line("check_circle", Color(hex: 0x2E9E4F), "Led a checkout rewrite")
                line("cancel", Color(hex: 0xD08A00), "No production GraphQL federation")
                Text("Screen-out checks").font(M3Type.roboto(16, .medium)).foregroundStyle(M3.onSurface)
                line("check_circle", Color(hex: 0x2E9E4F), "Where they hire: Americas")
                line("help", Color(hex: 0xD08A00), "Years: 6+ years")
                HStack(spacing: 0) {
                    Text("Fit  ").font(M3Type.roboto(16, .medium)).foregroundStyle(M3.onSurface)
                    Text("Unclear").font(M3Type.bodyLarge).foregroundStyle(Color(hex: 0xD08A00))
                }
                line("check_circle", Color(hex: 0x2E9E4F), "Where they hire: Americas")
            }
            .padding(.horizontal, 16)
        }
        .overlay(alignment: .topTrailing) { Marker(number: 2).offset(x: 8, y: 150) }
    }

    private func filled(_ title: String) -> some View {
        Text(title).font(M3Type.labelLarge).foregroundStyle(.white).lineLimit(1).fixedSize()
            .padding(.horizontal, 16).frame(height: 40).background(wallpaperPrimary, in: Capsule())
    }

    private func outlined(_ title: String) -> some View {
        Text(title).font(M3Type.labelLarge).foregroundStyle(wallpaperPrimary).lineLimit(1).fixedSize()
            .padding(.horizontal, 16).frame(height: 40).overlay(Capsule().strokeBorder(wallpaperOutline))
    }

    private func line(_ symbol: String, _ color: Color, _ text: String) -> some View {
        HStack(spacing: 8) {
            MSymbol(symbol, fill: true).foregroundStyle(color)
            Text(text).font(M3Type.bodyLarge).foregroundStyle(M3.onSurface)
        }
    }

    private func notes(_ items: [(Int, String)]) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            ForEach(items, id: \.0) { item in
                HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                    Marker(number: item.0)
                    Text(item.1).font(.system(size: 12)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

// MARK: - Rollout

struct RolloutBoard: View {
    let directory: URL

    private struct Ticket {
        let id: String
        let title: String
        let platform: String
        let images: [String]
        let column: Int
        let top: CGFloat
        var alignment: Alignment = .top
    }

    private let cardSize = CGSize(width: 300, height: 236)
    private let columnGap: CGFloat = 90

    private let tickets = [
        Ticket(id: "TP-449", title: "Design tokens, tones and shared components, restyled in place", platform: "macOS",
               images: ["design-board.png", "macos-job-before-after.png"], column: 0, top: 0),
        Ticket(id: "TP-457", title: "The hub icon", platform: "Both", images: ["design-board.png"], column: 0, top: 276, alignment: .topLeading),
        Ticket(id: "TP-456", title: "Android: Material 3 theme from Hub Indigo, tones, components and vocabulary", platform: "Android",
               images: ["android-components.png", "android-job-before-after.png"], column: 0, top: 552, alignment: .topLeading),
        Ticket(id: "TP-450", title: "One vocabulary: Skip, Screen, Match", platform: "macOS",
               images: ["macos-vocabulary.png"], column: 1, top: 0),
        Ticket(id: "TP-451", title: "Grouped sidebar, Settings window, Criteria page, Profile tabs, connection banner", platform: "macOS",
               images: ["macos-settings-criteria.png", "macos-sidebar.png"], column: 1, top: 276, alignment: .topLeading),
        Ticket(id: "TP-458", title: "Android: Today, Pipeline and Settings", platform: "Android",
               images: ["android.png", "android-system.png"], column: 1, top: 552),
        Ticket(id: "TP-463", title: "Android: adaptive layout for foldables and tablets", platform: "Android",
               images: ["android-foldable.png"], column: 2, top: 552, alignment: .topLeading),
        Ticket(id: "TP-462", title: "Android: notification channels and actions", platform: "Android",
               images: ["android-system.png"], column: 2, top: 828, alignment: .topLeading),
        Ticket(id: "TP-452", title: "One inspector for jobs, companies and people, with history and links", platform: "macOS",
               images: ["macos-inspector.png", "macos-company-person.png", "ia-map.png"], column: 2, top: 138, alignment: .topLeading),
        Ticket(id: "TP-453", title: "People across companies, replacing Recruiters", platform: "Server + macOS",
               images: ["macos-people.png"], column: 3, top: 0, alignment: .topLeading),
        Ticket(id: "TP-454", title: "The Today page, replacing Updates", platform: "macOS",
               images: ["macos-today.png", "macos-today-dark.png"], column: 3, top: 276, alignment: .topLeading),
        Ticket(id: "TP-455", title: "⌘K palette and keyboard decisions", platform: "macOS",
               images: ["macos-palette.png"], column: 3, top: 552),
    ]

    private let dependencies = [
        ("TP-449", "TP-450"), ("TP-449", "TP-451"), ("TP-450", "TP-452"), ("TP-451", "TP-452"),
        ("TP-452", "TP-453"), ("TP-452", "TP-454"), ("TP-452", "TP-455"), ("TP-456", "TP-458"),
        ("TP-458", "TP-463"), ("TP-458", "TP-462"),
    ]

    private let stages = ["1 · Foundations", "2 · Words and structure", "3 · One inspector", "4 · New pages"]

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            VStack(alignment: .leading, spacing: Space.xs) {
                Text("Rollout: twelve tickets, each one shippable on its own").font(.system(size: 22, weight: .semibold)).foregroundStyle(ink)
                Text("Arrows are blocked-by links. Each card shows the mockup of what that ticket delivers; the names below it are the files in docs/design/mockups.")
                    .font(.system(size: 13)).foregroundStyle(secondaryInk)
            }
            HStack(spacing: columnGap) {
                ForEach(stages, id: \.self) { stage in
                    Text(stage).font(.system(size: 13, weight: .semibold)).foregroundStyle(secondaryInk).frame(width: cardSize.width, alignment: .leading)
                }
            }
            ZStack(alignment: .topLeading) {
                Canvas { context, _ in
                    for (from, to) in dependencies { draw(from, to, in: &context) }
                }
                ForEach(tickets, id: \.id) { ticket in
                    card(ticket).offset(x: x(ticket.column), y: ticket.top)
                }
            }
            .frame(width: x(4) - columnGap, height: 828 + cardSize.height, alignment: .topLeading)
        }
        .padding(48)
        .background(windowBackground)
    }

    private func x(_ column: Int) -> CGFloat { CGFloat(column) * (cardSize.width + columnGap) }

    private func ticket(_ id: String) -> Ticket { tickets.first { $0.id == id }! }

    private func draw(_ from: String, _ to: String, in context: inout GraphicsContext) {
        let a = ticket(from)
        let b = ticket(to)
        let start = CGPoint(x: x(a.column) + cardSize.width + 4, y: a.top + cardSize.height / 2)
        let end = CGPoint(x: x(b.column) - 6, y: b.top + cardSize.height / 2)
        var path = Path()
        path.move(to: start)
        path.addCurve(to: end, control1: CGPoint(x: start.x + 50, y: start.y), control2: CGPoint(x: end.x - 50, y: end.y))
        context.stroke(path, with: .color(Tone.accent.color.opacity(0.7)), lineWidth: 1.6)
        var head = Path()
        head.move(to: CGPoint(x: end.x + 4, y: end.y))
        head.addLine(to: CGPoint(x: end.x - 5, y: end.y - 5))
        head.addLine(to: CGPoint(x: end.x - 5, y: end.y + 5))
        head.closeSubpath()
        context.fill(head, with: .color(Tone.accent.color.opacity(0.7)))
    }

    private func card(_ ticket: Ticket) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack {
                Text(ticket.id).font(.system(size: 12, weight: .bold).monospaced()).foregroundStyle(Tone.accent.color)
                Spacer()
                Chip(text: ticket.platform, tone: .neutral)
            }
            Text(ticket.title).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                .lineLimit(2).fixedSize(horizontal: false, vertical: true)
                .frame(height: 34, alignment: .topLeading)
            thumbnail(ticket)
            Text(ticket.images.joined(separator: ", ")).font(.system(size: 10).monospaced()).foregroundStyle(tertiaryInk)
                .lineLimit(2).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Space.m)
        .frame(width: cardSize.width, height: cardSize.height, alignment: .topLeading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }

    @ViewBuilder
    private func thumbnail(_ ticket: Ticket) -> some View {
        let size = CGSize(width: cardSize.width - 2 * Space.m, height: 120)
        Group {
            if ticket.id == "TP-457" {
                HStack(alignment: .bottom, spacing: Space.l) {
                    HubIcon(size: 84)
                    HubIcon(size: 48)
                    HubIcon(size: 28)
                }
                .frame(width: size.width, height: size.height)
                .background(well)
            } else if let image = NSImage(contentsOf: directory.appending(path: ticket.images[0])) {
                Image(nsImage: image).resizable().interpolation(.high).scaledToFill()
                    .frame(width: size.width, height: size.height, alignment: ticket.alignment)
            } else {
                well.frame(width: size.width, height: size.height)
            }
        }
        .frame(width: size.width, height: size.height)
        .clipShape(RoundedRectangle(cornerRadius: Radius.control))
        .overlay(RoundedRectangle(cornerRadius: Radius.control).strokeBorder(separator))
    }
}

// MARK: - Rendering

@MainActor
func write(_ view: some View, to directory: URL, name: String, dark: Bool = false) {
    darkMode = dark
    defer { darkMode = false }
    let renderer = ImageRenderer(content: view.environment(\.colorScheme, dark ? .dark : .light))
    renderer.scale = 2
    guard let image = renderer.nsImage, let tiff = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff),
          let png = bitmap.representation(using: .png, properties: [:])
    else {
        FileHandle.standardError.write("could not render \(name)\n".data(using: .utf8)!)
        exit(1)
    }
    try! png.write(to: directory.appending(path: name))
    print("wrote \(name)")
}

/// Draws a view on its own, for a board that shows a dark view among light ones.
@MainActor
func snapshot(dark: Bool, _ view: () -> some View) -> NSImage {
    darkMode = dark
    defer { darkMode = false }
    // Built after the mode is set: views read their colors when they're made.
    let renderer = ImageRenderer(content: view().environment(\.colorScheme, dark ? .dark : .light))
    renderer.scale = 2
    return renderer.nsImage!
}

MainActor.assumeIsolated {
    loadMaterialSymbols()
    let directory = URL(filePath: CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : ".")
    write(DesignBoard(), to: directory, name: "design-board.png")
    write(MacWindowToday(), to: directory, name: "macos-today.png")
    write(InspectorAnatomy(), to: directory, name: "macos-inspector.png")
    write(SidebarComparison(), to: directory, name: "macos-sidebar.png")
    write(AndroidScreens(), to: directory, name: "android.png")
    write(MacWindowToday(), to: directory, name: "macos-today-dark.png", dark: true)
    write(InformationMap(), to: directory, name: "ia-map.png")
    write(JobBeforeAfter(), to: directory, name: "macos-job-before-after.png")
    write(VocabularyBoard(), to: directory, name: "macos-vocabulary.png")
    write(CompanyAndPerson(), to: directory, name: "macos-company-person.png")
    write(MacWindowPeople(), to: directory, name: "macos-people.png")
    write(MacWindowPalette(), to: directory, name: "macos-palette.png")
    write(SettingsAndCriteria(), to: directory, name: "macos-settings-criteria.png")
    write(AndroidJobBeforeAfter(), to: directory, name: "android-job-before-after.png")
    write(AndroidSystemBoard(), to: directory, name: "android-system.png")
    write(AndroidComponentsBoard(), to: directory, name: "android-components.png")
    let darkPhone = snapshot(dark: true) { AndroidPhone(nav: "Today") { AndroidScreens().todayContent(skipped: false) } }
    write(AndroidFoldableBoard(darkPhone: darkPhone), to: directory, name: "android-foldable.png")
    // Last: the rollout board shows thumbnails of the images above.
    write(RolloutBoard(directory: directory), to: directory, name: "rollout.png")
}
