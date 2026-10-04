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

let ink = Color(hex: 0x1D1D1F)
let secondaryInk = Color(hex: 0x6E6E73)
let tertiaryInk = Color(hex: 0xA1A1A6)
let separator = Color(hex: 0xE3E3E8)
let windowBackground = Color(hex: 0xFBFBFD)
let sidebarBackground = Color(hex: 0xF0F0F4)
let well = Color(hex: 0xF2F2F6)

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
        .foregroundStyle(tone.light)
        .background(tone.light.opacity(0.13), in: Capsule())
    }
}

struct UnseenDot: View {
    var body: some View { Circle().fill(Tone.accent.light).frame(width: 7, height: 7) }
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
        .foregroundStyle(.white)
        .background(Tone.accent.light, in: Capsule())
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
        .background(Color.black.opacity(0.06), in: Capsule())
    }
}

struct OverflowButton: View {
    var body: some View {
        Image(systemName: "ellipsis").font(.system(size: 12, weight: .semibold))
            .frame(width: 26, height: 24)
            .foregroundStyle(ink)
            .background(Color.black.opacity(0.06), in: Capsule())
    }
}

struct SectionTitle: View {
    let text: String
    var accessory: String?
    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(text).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
            Spacer()
            if let accessory { Text(accessory).font(.system(size: 11)).foregroundStyle(Tone.accent.light) }
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
        .background(.white, in: RoundedRectangle(cornerRadius: Radius.card))
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
            Image(systemName: symbol).foregroundStyle(tone.light).font(.system(size: 12))
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
                .foregroundStyle(isSelected ? .white : Tone.accent.light)
            Text(title).font(.system(size: 13)).foregroundStyle(isSelected ? .white : ink)
            Spacer()
            if let badge {
                Text("\(badge)").font(.system(size: 11, weight: .semibold)).monospacedDigit()
                    .foregroundStyle(isSelected ? .white : secondaryInk)
            }
        }
        .padding(.horizontal, Space.s).padding(.vertical, 5)
        .background(isSelected ? AnyShapeStyle(Tone.accent.light) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: Radius.control + 2))
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
            SidebarItem(title: "Jobs", symbol: "briefcase")
            SidebarItem(title: "Companies", symbol: "building.2")
            SidebarItem(title: "People", symbol: "person.2")
            SidebarHeader(title: "You")
            SidebarItem(title: "Profile", symbol: "person.crop.circle")
            SidebarItem(title: "Criteria", symbol: "slider.horizontal.3")
            SidebarHeader(title: "Hub", collapsed: true)
            SidebarHeader(title: "Sessions")
            sessionRow("Northwind", "Waiting for you", Tone.caution.light)
            sessionRow("Senior Product Engineer", "Working", Tone.accent.light)
            Spacer()
        }
        .padding(Space.s)
        .frame(width: 236)
        .frame(maxHeight: .infinity)
        .background(sidebarBackground, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(Color.black.opacity(0.05)))
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
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(Color.black.opacity(0.05)))
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
                    .padding(.horizontal, 5).padding(.vertical, 1).background(Color.black.opacity(0.06), in: RoundedRectangle(cornerRadius: 4))
            }
            .padding(.horizontal, 10).padding(.vertical, 6)
            .frame(width: 300)
            .background(Color.black.opacity(0.045), in: Capsule())
            Image(systemName: "plus").font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                .frame(width: 30, height: 26).background(Color.black.opacity(0.045), in: Capsule())
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
            .background(Color(hex: 0xDADBE3))
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
                        Text("NORTHWIND").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.accent.light)
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
                HStack(spacing: 0) {
                    ForEach(["Overview", "Prep", "Posting", "Session"], id: \.self) { tab in
                        Text(tab).font(.system(size: 12, weight: tab == "Overview" ? .semibold : .regular))
                            .foregroundStyle(tab == "Overview" ? ink : secondaryInk)
                            .frame(maxWidth: .infinity).padding(.vertical, 4)
                            .background(tab == "Overview" ? AnyShapeStyle(.white) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 6))
                            .shadow(color: .black.opacity(tab == "Overview" ? 0.08 : 0), radius: 1, y: 1)
                    }
                }
                .padding(2)
                .background(Color.black.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
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
        .background(.white, in: RoundedRectangle(cornerRadius: numbered ? Radius.panel : 0))
        .overlay(alignment: .leading) { Rectangle().fill(separator).frame(width: 1).opacity(numbered ? 0 : 1) }
    }

    private func navButton(_ symbol: String) -> some View {
        Image(systemName: symbol).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            .frame(width: 28, height: 24).background(Color.black.opacity(0.045), in: Capsule())
    }

    private func point(_ symbol: String, _ tone: Tone, _ text: String, _ entry: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 12)).foregroundStyle(tone.light)
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
                                    Rectangle().fill(Tone.accent.light.opacity(0.25)).frame(width: value, height: value)
                                    Text("\(name) \(Int(value))").font(.system(size: 11).monospaced()).foregroundStyle(secondaryInk)
                                }
                            }
                        }
                        HStack(spacing: Space.l) {
                            ForEach([("control", Radius.control), ("card", Radius.card), ("panel", Radius.panel)], id: \.0) { name, value in
                                VStack(spacing: Space.xs) {
                                    RoundedRectangle(cornerRadius: value).strokeBorder(Tone.accent.light, lineWidth: 1.5).frame(width: 72, height: 44)
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
                            .background(Color.black.opacity(0.04), in: Capsule())
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
                            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Tone.negative.light)
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
                        .background(Tone.negative.light.opacity(0.06), in: RoundedRectangle(cornerRadius: Radius.card))
                        HStack(spacing: Space.s) {
                            Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.positive.light)
                            Text("Pursued 3 jobs").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                            Text("Undo").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.light)
                        }
                        .padding(.horizontal, 14).padding(.vertical, 8)
                        .background(.white, in: Capsule())
                        .overlay(Capsule().strokeBorder(separator))
                        .shadow(color: .black.opacity(0.08), radius: 6, y: 2)
                    }
                }
                .frame(width: 560, alignment: .leading)
                board("Connection", subtitle: "Shown once, above every page, instead of each page's own empty state.") {
                    HStack(spacing: Space.s) {
                        Image(systemName: "network.slash").foregroundStyle(Tone.caution.light)
                        Text("Can't reach the hub at localhost:8090").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                        Spacer()
                        Text("Start server").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.light)
                        Text("Settings…").font(.system(size: 12, weight: .semibold)).foregroundStyle(Tone.accent.light)
                    }
                    .padding(.horizontal, Space.m).padding(.vertical, Space.s)
                    .background(Tone.caution.light.opacity(0.1), in: RoundedRectangle(cornerRadius: Radius.card))
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

// MARK: - Android

struct Phone<Content: View>: View {
    let tab: String?
    @ViewBuilder let content: Content
    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("9:41").font(.system(size: 12, weight: .semibold))
                Spacer()
                Image(systemName: "wifi").font(.system(size: 11))
                Image(systemName: "battery.75percent").font(.system(size: 13))
            }
            .foregroundStyle(ink)
            .padding(.horizontal, 22).frame(height: 32)
            VStack(spacing: 0) { content }.frame(maxHeight: .infinity, alignment: .top)
            if let tab {
                HStack {
                    ForEach([("Today", "sun.max"), ("Decide", "checklist"), ("Pipeline", "rectangle.split.3x1"), ("Jobs", "briefcase")], id: \.0) { item in
                        VStack(spacing: 4) {
                            Image(systemName: item.1).font(.system(size: 15, weight: .medium))
                                .frame(width: 56, height: 28)
                                .background(item.0 == tab ? Tone.accent.light.opacity(0.16) : .clear, in: Capsule())
                            Text(item.0).font(.system(size: 11, weight: item.0 == tab ? .semibold : .regular))
                        }
                        .foregroundStyle(item.0 == tab ? Tone.accent.light : secondaryInk)
                        .frame(maxWidth: .infinity)
                    }
                }
                .padding(.vertical, 10)
                .background(Color(hex: 0xF3F2FA))
            }
        }
        .frame(width: 360, height: 760)
        .background(Color(hex: 0xFCFBFF))
        .clipShape(RoundedRectangle(cornerRadius: 36, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 36, style: .continuous).strokeBorder(Color(hex: 0x2A2A2E), lineWidth: 8))
    }
}

struct AppBar: View {
    let title: String
    var back = false
    var actions: [String] = []
    var body: some View {
        HStack(spacing: Space.l) {
            if back { Image(systemName: "arrow.left").font(.system(size: 17)) }
            Text(title).font(.system(size: 21, weight: back ? .regular : .medium))
            Spacer()
            ForEach(actions, id: \.self) { Image(systemName: $0).font(.system(size: 17)) }
        }
        .foregroundStyle(ink)
        .padding(.horizontal, 18).frame(height: 56)
    }
}

struct PhoneCard<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title).font(.system(size: 13, weight: .semibold)).foregroundStyle(secondaryInk)
            content
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(hex: 0xF1F0F8), in: RoundedRectangle(cornerRadius: 16))
    }
}

struct AndroidScreens: View {
    var body: some View {
        HStack(alignment: .top, spacing: 40) {
            labelled("Today", "What needs you, from every part of the hub") { today }
            labelled("Job", "One primary action, the rest one tap away") { job }
            labelled("Pipeline", "New on the phone: follow up where the reminder lands") { pipeline }
        }
        .padding(48)
        .background(Color(hex: 0xE8E8EE))
    }

    private func labelled(_ title: String, _ subtitle: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            Text(title).font(.system(size: 17, weight: .semibold)).foregroundStyle(ink)
            Text(subtitle).font(.system(size: 12)).foregroundStyle(secondaryInk)
            content().padding(.top, Space.s)
        }
    }

    private var today: some View {
        Phone(tab: "Today") {
            AppBar(title: "Today", actions: ["magnifyingglass", "ellipsis"])
            VStack(spacing: 12) {
                PhoneCard(title: "DECIDE · 7") {
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(spacing: 6) {
                            Chip(text: "Strong", tone: .positive)
                            Text("Senior Product Engineer").font(.system(size: 14, weight: .medium)).lineLimit(1)
                        }
                        Text("Northwind · Remote, Americas").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(spacing: 6) {
                            Chip(text: "Possible", tone: .accent)
                            Text("Staff Frontend Engineer").font(.system(size: 14, weight: .medium)).lineLimit(1)
                        }
                        Text("Globex · Remote, LATAM").font(.system(size: 12)).foregroundStyle(secondaryInk)
                    }
                }
                PhoneCard(title: "FOLLOW UP · 2") {
                    HStack {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Full-Stack Engineer, Payments").font(.system(size: 14, weight: .medium))
                            Chip(text: "Overdue 2 days", tone: .negative, symbol: "bell.fill")
                        }
                        Spacer()
                        Image(systemName: "checkmark").font(.system(size: 14, weight: .semibold)).foregroundStyle(Tone.accent.light)
                            .frame(width: 36, height: 36).background(Tone.accent.light.opacity(0.14), in: Circle())
                    }
                }
                PhoneCard(title: "UPDATES") {
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        UnseenDot()
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Reply from Umbrella Labs").font(.system(size: 14, weight: .semibold))
                            Text("“Could you do a call on Tuesday…”").font(.system(size: 12)).foregroundStyle(secondaryInk)
                        }
                    }
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        UnseenDot()
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Fresh strong match").font(.system(size: 14, weight: .semibold))
                            Text("Northwind, posted 2 days ago").font(.system(size: 12)).foregroundStyle(secondaryInk)
                        }
                    }
                }
            }
            .foregroundStyle(ink)
            .padding(.horizontal, 14)
        }
    }

    private var job: some View {
        Phone(tab: nil) {
            AppBar(title: "Northwind", back: true, actions: ["ellipsis"])
            VStack(alignment: .leading, spacing: 12) {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Senior Product Engineer").font(.system(size: 22, weight: .regular))
                    Text("Remote, Americas · Posted 2 days ago").font(.system(size: 13)).foregroundStyle(secondaryInk)
                    HStack(spacing: 6) {
                        Chip(text: "Strong match", tone: .positive)
                        Chip(text: "Passes screen", tone: .positive, symbol: "checkmark")
                    }
                }
                HStack(spacing: 8) {
                    Text("Pursue").font(.system(size: 14, weight: .medium)).foregroundStyle(.white)
                        .frame(maxWidth: .infinity).frame(height: 40).background(Tone.accent.light, in: Capsule())
                    Text("Later").font(.system(size: 14, weight: .medium)).foregroundStyle(Tone.accent.light)
                        .frame(width: 80, height: 40).background(Tone.accent.light.opacity(0.13), in: Capsule())
                    Text("Skip").font(.system(size: 14, weight: .medium)).foregroundStyle(Tone.accent.light)
                        .frame(width: 80, height: 40).background(Tone.accent.light.opacity(0.13), in: Capsule())
                }
                PhoneCard(title: "BRIEF · BY CLAUDE") {
                    Text("Your payments cases map onto their checkout rebuild, and the take-home clears your target.")
                        .font(.system(size: 13)).fixedSize(horizontal: false, vertical: true)
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Image(systemName: "minus.circle.fill").font(.system(size: 12)).foregroundStyle(Tone.caution.light)
                        Text("No production GraphQL federation").font(.system(size: 13))
                    }
                }
                PhoneCard(title: "SCREEN") {
                    VerdictRow(verdict: .yes, name: "Where they hire", reason: "Americas")
                    VerdictRow(verdict: .unclear, name: "Years", reason: "Asks 6+")
                    VerdictRow(verdict: .yes, name: "Pay", reason: "About R$ 31k take-home")
                }
                HStack {
                    Text("People · 2").font(.system(size: 14, weight: .medium))
                    Spacer()
                    Image(systemName: "chevron.right").font(.system(size: 12)).foregroundStyle(secondaryInk)
                }
                .padding(.horizontal, 4)
                HStack {
                    Text("Posting").font(.system(size: 14, weight: .medium))
                    Spacer()
                    Image(systemName: "chevron.right").font(.system(size: 12)).foregroundStyle(secondaryInk)
                }
                .padding(.horizontal, 4)
            }
            .foregroundStyle(ink)
            .padding(.horizontal, 16)
        }
    }

    private var pipeline: some View {
        Phone(tab: "Pipeline") {
            AppBar(title: "Pipeline", actions: ["bell.badge"])
            HStack(spacing: 0) {
                HStack(spacing: 8) {
                    phase("Applied 4", true)
                    phase("Screening 2", false)
                    phase("Interviewing 1", false)
                    phase("Offer", false)
                }
                .padding(.horizontal, 14)
            }
            .frame(width: 344, alignment: .leading)
            .clipped()
            .padding(.bottom, 10)
            VStack(spacing: 10) {
                card("Full-Stack Engineer, Payments", "Initech", ("Overdue 2 days", .negative), "9 days in Applied")
                card("Outreach", "Acme Robotics", ("Due today", .caution), "7 days in Applied")
                card("Senior Software Engineer", "Umbrella Labs", ("Heard back 2 Oct", .positive), "3 days in Applied")
                card("Product Engineer", "Globex", ("Due in 4 days", .neutral), "3 days in Applied")
            }
            .padding(.horizontal, 14)
        }
    }

    private func phase(_ title: String, _ selected: Bool) -> some View {
        Text(title).font(.system(size: 13, weight: .medium)).lineLimit(1).fixedSize()
            .foregroundStyle(selected ? Tone.accent.light : ink)
            .padding(.horizontal, 12).frame(height: 32)
            .background(selected ? Tone.accent.light.opacity(0.14) : .clear, in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(selected ? .clear : separator))
    }

    private func card(_ title: String, _ company: String, _ status: (String, Tone), _ age: String) -> some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 5) {
                Text(title).font(.system(size: 14, weight: .medium)).foregroundStyle(ink)
                Text(company).font(.system(size: 12)).foregroundStyle(secondaryInk)
                HStack(spacing: 6) {
                    Chip(text: status.0, tone: status.1)
                    Text(age).font(.system(size: 11)).foregroundStyle(tertiaryInk)
                }
            }
            Spacer()
            Image(systemName: "ellipsis").font(.system(size: 14)).foregroundStyle(secondaryInk).padding(.top, 2)
        }
        .padding(14)
        .background(Color(hex: 0xF1F0F8), in: RoundedRectangle(cornerRadius: 16))
    }
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
    print("wrote \(name)")
}

MainActor.assumeIsolated {
    let directory = URL(filePath: CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : ".")
    write(DesignBoard(), to: directory, name: "design-board.png")
    write(MacWindowToday(), to: directory, name: "macos-today.png")
    write(InspectorAnatomy(), to: directory, name: "macos-inspector.png")
    write(SidebarComparison(), to: directory, name: "macos-sidebar.png")
    write(AndroidScreens(), to: directory, name: "android.png")
}
