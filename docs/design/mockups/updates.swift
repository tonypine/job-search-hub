// Draws the update-flow mockups in docs/design/mockups with SwiftUI, offscreen:
//
//     swift docs/design/mockups/updates.swift docs/design/mockups
//
// Every company, person and change here is made up. The views only sketch the
// proposal in docs/design/updates.md; they are not app code. The tokens and
// chrome repeat the ones in render.swift, so these boards match the redesign's.
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
}

let ink = Color(hex: 0x1D1D1F)
let secondaryInk = Color(hex: 0x6E6E73)
let tertiaryInk = Color(hex: 0xA1A1A6)
let separator = Color(hex: 0xE3E3E8)
let windowBackground = Color(hex: 0xFBFBFD)
let sidebarBackground = Color(hex: 0xF0F0F4)
let surface = Color.white
let fill = Color.black.opacity(0.06)
let fillSubtle = Color.black.opacity(0.045)
let hairline = Color.black.opacity(0.05)
let desk = Color(hex: 0xDADBE3)

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

struct PrimaryButton: View {
    let title: String
    var symbol: String?
    var tone = Tone.accent
    var body: some View {
        HStack(spacing: 5) {
            if let symbol { Image(systemName: symbol).font(.system(size: 11, weight: .semibold)) }
            Text(title).font(.system(size: 12, weight: .semibold))
        }
        .padding(.horizontal, 12).padding(.vertical, 5)
        .foregroundStyle(.white)
        .background(tone.color, in: Capsule())
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

struct LinkText: View {
    let text: String
    var body: some View { Text(text).font(.system(size: 12)).foregroundStyle(Tone.accent.color) }
}

struct Lamp: View {
    let color: Color
    var body: some View { Circle().fill(color).frame(width: 8, height: 8) }
}

/// A progress spinner frozen mid-turn; ImageRenderer can't draw ProgressView.
struct Spinner: View {
    var size: CGFloat = 11
    var body: some View {
        Circle().trim(from: 0, to: 0.72)
            .stroke(secondaryInk, style: StrokeStyle(lineWidth: 1.6, lineCap: .round))
            .rotationEffect(.degrees(-90))
            .frame(width: size, height: size)
    }
}

struct ProgressBar: View {
    let value: Double
    var tone = Tone.accent
    var body: some View {
        GeometryReader { proxy in
            ZStack(alignment: .leading) {
                Capsule().fill(fill)
                Capsule().fill(tone.color).frame(width: proxy.size.width * value)
            }
        }
        .frame(height: 5)
    }
}

/// A radio button, drawn: ImageRenderer can't draw a Picker.
struct Radio: View {
    let title: String
    let isOn: Bool
    var body: some View {
        HStack(spacing: 6) {
            ZStack {
                Circle().strokeBorder(isOn ? Tone.accent.color : tertiaryInk, lineWidth: 1.2).frame(width: 14, height: 14)
                if isOn { Circle().fill(Tone.accent.color).frame(width: 7, height: 7) }
            }
            Text(title).font(.system(size: 12)).foregroundStyle(ink)
        }
    }
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

// MARK: - Annotation

struct BoardHeading: View {
    let title: String
    let subtitle: String
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).font(.system(size: 26, weight: .bold)).foregroundStyle(ink)
            Text(subtitle).font(.system(size: 14)).foregroundStyle(secondaryInk)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct Board<Content: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.xl) {
            BoardHeading(title: title, subtitle: subtitle)
            content
        }
        .padding(48)
        .background(desk)
    }
}

/// A numbered or lettered label above a mockup, with what it shows.
struct Caption: View {
    let mark: String
    let title: String
    var text: String = ""
    var recommended = false
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: Space.s) {
                Text(mark).font(.system(size: 12, weight: .bold)).foregroundStyle(.white)
                    .frame(width: 22, height: 22).background(Tone.accent.color, in: Circle())
                Text(title).font(.system(size: 15, weight: .semibold)).foregroundStyle(ink)
                if recommended { Chip(text: "Recommended", tone: .positive, symbol: "checkmark") }
            }
            if !text.isEmpty {
                Text(text).font(.system(size: 12)).foregroundStyle(secondaryInk)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}

struct Captioned<Content: View>: View {
    let mark: String
    let title: String
    var text: String = ""
    var recommended = false
    var width: CGFloat?
    var captionHeight: CGFloat?
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Caption(mark: mark, title: title, text: text, recommended: recommended)
                .frame(height: captionHeight, alignment: .topLeading)
            content
        }
        .frame(width: width, alignment: .topLeading)
    }
}

/// A pointer from a note to a part of a mockup.
struct Callout: View {
    let text: String
    var body: some View {
        HStack(alignment: .top, spacing: 6) {
            Image(systemName: "arrowtriangle.right.fill").font(.system(size: 8)).foregroundStyle(Tone.accent.color).padding(.top, 3)
            Text(text).font(.system(size: 11)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Space.s)
        .background(Color(hex: 0xFFF8E1), in: RoundedRectangle(cornerRadius: 6))
        .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(Color(hex: 0xF0D98C)))
    }
}

// MARK: - Mac chrome

struct TrafficLights: View {
    var body: some View {
        HStack(spacing: 8) {
            Circle().fill(Color(hex: 0xFF5F57)).frame(width: 12, height: 12)
            Circle().fill(Color(hex: 0xFEBC2E)).frame(width: 12, height: 12)
            Circle().fill(Color(hex: 0x28C840)).frame(width: 12, height: 12)
        }
    }
}

struct WindowFrame<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        content
            .background(windowBackground)
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(Color.black.opacity(0.12)))
            .shadow(color: .black.opacity(0.16), radius: 18, y: 8)
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
                .foregroundStyle(isSelected ? .white : Tone.accent.color)
            Text(title).font(.system(size: 13)).foregroundStyle(isSelected ? .white : ink)
            Spacer()
            if let badge {
                Text("\(badge)").font(.system(size: 11, weight: .semibold)).monospacedDigit()
                    .foregroundStyle(isSelected ? .white : secondaryInk)
            }
        }
        .padding(.horizontal, Space.s).padding(.vertical, 5)
        .background(isSelected ? AnyShapeStyle(Tone.accent.color) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: Radius.control + 2))
    }
}

struct SidebarHeader: View {
    let title: String
    var body: some View {
        Text(title).font(.system(size: 11, weight: .semibold)).foregroundStyle(tertiaryInk)
            .padding(.horizontal, Space.s).padding(.top, Space.m).padding(.bottom, 2)
    }
}

/// What the foot of the sidebar shows about versions.
enum SidebarFoot {
    case none
    case ready(String)
    case onQuit(String)
    case installing
}

struct VersionLabel: View {
    let foot: SidebarFoot
    var body: some View {
        switch foot {
        case .none:
            EmptyView()
        case let .ready(version):
            row(symbol: "arrow.down.circle.fill", title: "New version \(version)", subtitle: "Ready to install", tone: .accent)
        case let .onQuit(version):
            row(symbol: "power.circle.fill", title: "Installs when you quit", subtitle: version, tone: .accent)
        case .installing:
            row(symbol: "arrow.triangle.2.circlepath.circle.fill", title: "Installing…", subtitle: "Waiting for 1 session", tone: .caution)
        }
    }

    private func row(symbol: String, title: String, subtitle: String, tone: Tone) -> some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).font(.system(size: 15)).foregroundStyle(tone.color)
            VStack(alignment: .leading, spacing: 0) {
                Text(title).font(.system(size: 12, weight: .semibold)).foregroundStyle(ink)
                Text(subtitle).font(.system(size: 10)).foregroundStyle(secondaryInk)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, Space.s).padding(.vertical, 6)
        .background(tone.color.opacity(0.09), in: RoundedRectangle(cornerRadius: Radius.control + 2))
    }
}

struct Sidebar: View {
    var foot = SidebarFoot.none
    var sessionState = "Working"
    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            TrafficLights().padding(.leading, 6).padding(.top, 4).padding(.bottom, Space.l)
            SidebarItem(title: "Today", symbol: "sun.max", badge: 4, isSelected: true)
            SidebarItem(title: "Decide", symbol: "checklist", badge: 7)
            SidebarItem(title: "Pipeline", symbol: "rectangle.split.3x1", badge: 2)
            SidebarHeader(title: "Browse")
            SidebarItem(title: "Jobs", symbol: "briefcase")
            SidebarItem(title: "Companies", symbol: "building.2")
            SidebarItem(title: "People", symbol: "person.2")
            SidebarHeader(title: "Sessions")
            sessionRow("Acme", sessionState, sessionState == "Working" ? Tone.accent.color : Tone.neutral.color)
            sessionRow("Staff Engineer · Initech", "Idle", Tone.neutral.color)
            Spacer(minLength: Space.m)
            VersionLabel(foot: foot)
        }
        .padding(Space.s)
        .frame(width: 210)
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

/// The main window's toolbar; `versionButton` draws proposal B's capsule.
struct Toolbar: View {
    var versionButton = false
    var body: some View {
        HStack(spacing: Space.m) {
            VStack(alignment: .leading, spacing: 0) {
                Text("Today").font(.system(size: 15, weight: .semibold)).foregroundStyle(ink)
                Text("Monday, 6 October").font(.system(size: 11)).foregroundStyle(secondaryInk).lineLimit(1)
            }
            Spacer()
            HStack(spacing: Space.s) {
                Image(systemName: "magnifyingglass").font(.system(size: 11)).foregroundStyle(secondaryInk)
                Text("Jump to anything").font(.system(size: 12)).foregroundStyle(tertiaryInk).lineLimit(1)
                Spacer()
                Text("⌘K").font(.system(size: 11, weight: .medium)).foregroundStyle(secondaryInk)
            }
            .padding(.horizontal, 10).padding(.vertical, 6)
            .frame(width: versionButton ? 140 : 190)
            .background(fillSubtle, in: Capsule())
            if versionButton {
                HStack(spacing: 4) {
                    Image(systemName: "arrow.down.circle.fill").font(.system(size: 11))
                    Text("New version").font(.system(size: 12, weight: .semibold))
                }
                .padding(.horizontal, 10).padding(.vertical, 5)
                .foregroundStyle(Tone.accent.color)
                .background(Tone.accent.color.opacity(0.12), in: Capsule())
                .fixedSize()
            }
        }
        .padding(.horizontal, Space.l)
        .frame(height: 52)
    }
}

/// Grey stand-ins for Today's content, so the eye goes to the part that changes.
struct TodayStandIn: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            ForEach(0 ..< 3, id: \.self) { index in
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: Space.s) {
                        Chip(text: ["Strong", "Possible", "Stretch"][index], tone: [Tone.positive, .accent, .caution][index])
                        Text(["Staff Engineer", "Senior Product Engineer", "Platform Lead"][index])
                            .font(.system(size: 13, weight: .medium)).foregroundStyle(ink)
                    }
                    Text(["Acme", "Initech", "Globex"][index]).font(.system(size: 12)).foregroundStyle(secondaryInk)
                    RoundedRectangle(cornerRadius: 3).fill(fill).frame(height: 7)
                    RoundedRectangle(cornerRadius: 3).fill(fill).frame(width: 200, height: 7)
                }
                .padding(Space.m)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
                .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
            }
        }
        .padding(.horizontal, Space.l)
    }
}

struct MainWindow<Banner: View>: View {
    var foot = SidebarFoot.none
    var versionButton = false
    var sessionState = "Working"
    var width: CGFloat = 640
    var height: CGFloat = 430
    @ViewBuilder var banner: Banner

    var body: some View {
        WindowFrame {
            HStack(spacing: 0) {
                Sidebar(foot: foot, sessionState: sessionState)
                VStack(alignment: .leading, spacing: 0) {
                    Toolbar(versionButton: versionButton)
                    banner
                    TodayStandIn().padding(.top, Space.s)
                    Spacer(minLength: 0)
                }
            }
            .frame(width: width, height: height)
        }
    }
}

extension MainWindow where Banner == EmptyView {
    init(foot: SidebarFoot = .none, versionButton: Bool = false, sessionState: String = "Working", width: CGFloat = 640, height: CGFloat = 430) {
        self.init(foot: foot, versionButton: versionButton, sessionState: sessionState, width: width, height: height) { EmptyView() }
    }
}

/// A one-line banner at the top of the page, where the connection banner goes.
struct PageBanner: View {
    let symbol: String
    let text: String
    let action: String
    var tone = Tone.accent
    var body: some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).foregroundStyle(tone.color).font(.system(size: 12, weight: .semibold))
            Text(text).font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
            Text(action).font(.system(size: 12, weight: .semibold)).foregroundStyle(tone.color)
            Spacer()
            Image(systemName: "xmark").font(.system(size: 10, weight: .semibold)).foregroundStyle(secondaryInk)
        }
        .padding(.horizontal, Space.m).padding(.vertical, 7)
        .background(tone.color.opacity(0.1), in: RoundedRectangle(cornerRadius: Radius.control + 2))
        .padding(.horizontal, Space.l)
    }
}

/// A sheet over a dimmed window.
struct SheetOver<Sheet: View>: View {
    var foot = SidebarFoot.none
    var width: CGFloat = 640
    var height: CGFloat = 470
    @ViewBuilder let sheet: Sheet
    var body: some View {
        ZStack(alignment: .top) {
            MainWindow(foot: foot, width: width, height: height)
                .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).fill(.black.opacity(0.18)))
            sheet
                .background(surface, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.black.opacity(0.1)))
                .shadow(color: .black.opacity(0.25), radius: 20, y: 10)
                .padding(.top, 34)
        }
    }
}

struct SheetBody<Content: View, Buttons: View>: View {
    let title: String
    var width: CGFloat = 430
    @ViewBuilder let content: Content
    @ViewBuilder let buttons: Buttons
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text(title).font(.system(size: 15, weight: .semibold)).foregroundStyle(ink)
            content
            HStack(spacing: Space.s) {
                Spacer()
                buttons
            }
            .padding(.top, Space.xs)
        }
        .padding(Space.l + 2)
        .frame(width: width, alignment: .leading)
    }
}

struct Prose: View {
    let text: String
    var color = ink
    var body: some View {
        Text(text).font(.system(size: 12)).foregroundStyle(color).fixedSize(horizontal: false, vertical: true)
    }
}

/// A row of what's running: a session, a run, an edit.
struct RunningRow: View {
    enum State { case working, waiting, done, blocked, idle }
    let state: State
    let title: String
    let detail: String
    var body: some View {
        HStack(spacing: Space.s) {
            Group {
                switch state {
                case .working: Spinner()
                case .waiting: Image(systemName: "pencil.circle.fill").foregroundStyle(Tone.caution.color)
                case .done: Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.positive.color)
                case .blocked: Image(systemName: "questionmark.circle.fill").foregroundStyle(Tone.caution.color)
                case .idle: Image(systemName: "moon.circle").foregroundStyle(secondaryInk)
                }
            }
            .font(.system(size: 13))
            .frame(width: 16)
            Text(title).font(.system(size: 12, weight: .medium)).foregroundStyle(state == .done ? secondaryInk : ink)
                .strikethrough(state == .done, color: secondaryInk)
            Spacer()
            Text(detail).font(.system(size: 11)).foregroundStyle(secondaryInk)
        }
        .padding(.vertical, 2)
    }
}

/// The Settings window with its toolbar of tabs, Version selected.
struct SettingsFrame<Content: View>: View {
    var height: CGFloat = 560
    @ViewBuilder let content: Content
    var body: some View {
        WindowFrame {
            VStack(spacing: 0) {
                ZStack {
                    HStack { TrafficLights(); Spacer() }.padding(.horizontal, Space.m)
                    Text("Version").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                }
                .padding(.top, 10)
                HStack(spacing: Space.l) {
                    tab("network", "Connection")
                    tab("server.rack", "Server")
                    tab("person.crop.circle", "Accounts")
                    tab("iphone", "Phones")
                    tab("cpu", "Models")
                    tab("arrow.down.circle", "Version", selected: true)
                }
                .padding(.vertical, Space.s)
                Rectangle().fill(separator).frame(height: 1)
                content
                    .padding(Space.l)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .background(Color(hex: 0xF5F5F7))
            }
            .frame(width: 560, height: height)
        }
    }

    private func tab(_ symbol: String, _ title: String, selected: Bool = false) -> some View {
        VStack(spacing: 2) {
            Image(systemName: symbol).font(.system(size: 17)).foregroundStyle(selected ? Tone.accent.color : secondaryInk)
            Text(title).font(.system(size: 10)).foregroundStyle(selected ? Tone.accent.color : secondaryInk)
        }
        .frame(width: 62, height: 40)
        .background(selected ? fill : .clear, in: RoundedRectangle(cornerRadius: 6))
    }
}

/// A grouped form section, as SwiftUI's `.formStyle(.grouped)` draws one.
struct FormSection<Content: View>: View {
    var header: String?
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let header {
                Text(header).font(.system(size: 11, weight: .semibold)).foregroundStyle(secondaryInk).padding(.leading, 4)
            }
            VStack(alignment: .leading, spacing: Space.s) { content }
                .padding(Space.m)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(surface, in: RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(separator))
        }
    }
}

struct ChangeLine: View {
    let area: String
    let text: String
    let pr: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Text(area).font(.system(size: 10, weight: .semibold)).foregroundStyle(secondaryInk)
                .frame(width: 44, alignment: .leading)
            Text(text).font(.system(size: 12)).foregroundStyle(ink).lineLimit(1)
            Spacer(minLength: Space.s)
            Text(pr).font(.system(size: 11)).foregroundStyle(Tone.accent.color)
        }
    }
}

struct ChangeGroup: View {
    let title: String
    let lines: [(String, String, String)]
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).font(.system(size: 11, weight: .semibold)).foregroundStyle(ink)
            ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                ChangeLine(area: line.0, text: line.1, pr: line.2)
            }
        }
    }
}

let newChanges = [
    ("Mac", "Suggest a second route for unanswered applications", "#67"),
    ("Phone", "Snooze a follow-up from its notification", "#73"),
]
let fixedChanges = [
    ("Mac", "Keep the inspector from looping the window's layout", "#66"),
    ("Server", "Retry board polls that time out", "#71"),
]

// MARK: - Journey

struct JourneyStep: View {
    let symbol: String
    let tone: Tone
    let title: String
    let text: String
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Image(systemName: symbol).font(.system(size: 18, weight: .medium)).foregroundStyle(tone.color)
                .frame(width: 36, height: 36).background(tone.color.opacity(0.12), in: RoundedRectangle(cornerRadius: 9))
            Text(title).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            Text(text).font(.system(size: 11)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Space.m)
        .frame(width: 176, height: 150, alignment: .topLeading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }
}

struct Arrow: View {
    var body: some View {
        Image(systemName: "arrow.right").font(.system(size: 14, weight: .semibold)).foregroundStyle(tertiaryInk).frame(width: 22)
    }
}

struct Lane<Content: View>: View {
    let title: String
    let symbol: String
    @ViewBuilder let content: Content
    var body: some View {
        HStack(alignment: .center, spacing: Space.m) {
            VStack(spacing: 4) {
                Image(systemName: symbol).font(.system(size: 18)).foregroundStyle(secondaryInk)
                Text(title).font(.system(size: 12, weight: .semibold)).foregroundStyle(secondaryInk)
            }
            .frame(width: 80)
            content
            Spacer(minLength: 0)
        }
        .padding(Space.m)
        .background(Color.white.opacity(0.45), in: RoundedRectangle(cornerRadius: Radius.panel))
    }
}

struct LinearUpdateCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Image(systemName: "flag.fill").font(.system(size: 10)).foregroundStyle(Tone.positive.color)
                Text("Initiative update · On track").font(.system(size: 10, weight: .semibold)).foregroundStyle(Tone.positive.color)
            }
            Text("Job Search Hub 0.1.252 is out.").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
            Text("Mac: Job Search Hub 0.1.252 for Mac. The app offers it in Settings › Version.")
                .font(.system(size: 11)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            Text("New").font(.system(size: 11, weight: .semibold)).foregroundStyle(ink).padding(.top, 2)
            Text("• Mac: Suggest a second route for unanswered applications (#67)")
                .font(.system(size: 11)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            Text("Fixed").font(.system(size: 11, weight: .semibold)).foregroundStyle(ink)
            Text("• Mac: Keep the inspector from looping the window's layout (#66)")
                .font(.system(size: 11)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
        }
        .padding(Space.m)
        .frame(width: 300, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }
}

struct JourneyBoard: View {
    var body: some View {
        Board(title: "One merge, from ci to the Mac and the phone", subtitle: "Each merge to main that passes ci is released by CI, announced in Linear, and found by the apps.") {
            VStack(alignment: .leading, spacing: Space.l) {
                Lane(title: "GitHub", symbol: "chevron.left.forwardslash.chevron.right") {
                    JourneyStep(symbol: "arrow.triangle.merge", tone: .accent, title: "#67 merges into main", text: "Squash merge. ci runs on the merge commit.")
                    Arrow()
                    JourneyStep(symbol: "checkmark.seal", tone: .positive, title: "ci passes", text: "server-static, server-test, macos, android: the same gate as today.")
                    Arrow()
                    JourneyStep(symbol: "shippingbox", tone: .accent, title: "release builds what changed", text: "macos/ changed: build, sign, verify the Mac bundle. android/ didn't: no APK.")
                    Arrow()
                    JourneyStep(symbol: "tag", tone: .accent, title: "mac-v0.1.252 is published", text: "Job-Search-Hub-0.1.252.zip, its SHA-256, and the changelog.")
                }
                Lane(title: "Linear", symbol: "flag") {
                    Spacer().frame(width: 176 * 2 + 22 * 2 + Space.m * 3)
                    LinearUpdateCard()
                    Callout(text: "One post per release run, covering every platform the merge released.")
                        .frame(width: 180)
                }
                Lane(title: "Mac", symbol: "laptopcomputer") {
                    JourneyStep(symbol: "arrow.down.circle", tone: .neutral, title: "The app checks GitHub", text: "At launch and hourly. Downloads and checks the bundle quietly.")
                    Arrow()
                    JourneyStep(symbol: "sidebar.left", tone: .accent, title: "New version 0.1.252", text: "A label at the foot of the sidebar. No notification.")
                    Arrow()
                    JourneyStep(symbol: "list.bullet.rectangle", tone: .accent, title: "What's new, then Install", text: "Install now, or Install when I quit. Running work finishes first.")
                    Arrow()
                    JourneyStep(symbol: "checkmark.circle", tone: .positive, title: "Now on 0.1.252", text: "Server restarted, app reopened, Claude sessions back in their tabs.")
                }
                Lane(title: "Phone", symbol: "iphone") {
                    JourneyStep(symbol: "tag", tone: .accent, title: "android-v0.1.253", text: "A later merge changes android/: a signed APK.")
                    Arrow()
                    JourneyStep(symbol: "rectangle.stack.badge.plus", tone: .accent, title: "Today: 0.1.253 is ready", text: "The phone checks GitHub daily, downloads on Wi-Fi, checks the APK.")
                    Arrow()
                    JourneyStep(symbol: "hand.tap", tone: .accent, title: "Update, then Android's installer", text: "One tap, and the system's confirmation. The app restarts on 0.1.253.")
                }
            }
        }
        .frame(width: 1180)
    }
}

// MARK: - Hearing about it

struct HearingBoard: View {
    var body: some View {
        Board(title: "Where the owner hears about a new version", subtitle: "Four proposals, drawn the same moment: 0.1.252 is downloaded, checked and ready.") {
            VStack(alignment: .leading, spacing: Space.xxl) {
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "A", title: "Sidebar label", text: "Always in view, never in the way. One click opens Settings › Version.", recommended: true, width: 640) {
                        MainWindow(foot: .ready("0.1.252"))
                    }
                    Captioned(mark: "B", title: "Toolbar button", text: "On every page, but beside the page's own actions, and crowded on narrow windows.", width: 640) {
                        MainWindow(versionButton: true)
                    }
                }
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "C", title: "A window on launch", text: "Sparkle's way. Hard to miss; with releases most days, it would interrupt most days.", width: 640) {
                        ZStack {
                            MainWindow()
                                .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).fill(.black.opacity(0.12)))
                            WindowFrame {
                                VStack(alignment: .leading, spacing: Space.m) {
                                    HStack(alignment: .top, spacing: Space.m) {
                                        HubIcon(size: 48)
                                        VStack(alignment: .leading, spacing: 4) {
                                            Text("A new version of Job Search Hub is ready").font(.system(size: 14, weight: .semibold)).foregroundStyle(ink)
                                            Prose(text: "0.1.252 is ready. You have 0.1.247.", color: secondaryInk)
                                        }
                                    }
                                    VStack(alignment: .leading, spacing: 6) {
                                        ChangeGroup(title: "New", lines: [newChanges[0]])
                                        ChangeGroup(title: "Fixed", lines: fixedChanges)
                                    }
                                    .padding(Space.m)
                                    .background(Color(hex: 0xF5F5F7), in: RoundedRectangle(cornerRadius: 8))
                                    HStack {
                                        SecondaryButton(title: "Skip This Version")
                                        Spacer()
                                        SecondaryButton(title: "Later")
                                        PrimaryButton(title: "Install")
                                    }
                                }
                                .padding(Space.l)
                                .frame(width: 440)
                            }
                        }
                    }
                    Captioned(mark: "D", title: "A notification per release", text: "Reaches the owner outside the app; at several a day, it's noise. Kept for failures only.", width: 640) {
                        ZStack(alignment: .topTrailing) {
                            MainWindow()
                            HStack(alignment: .top, spacing: Space.s) {
                                HubIcon(size: 30)
                                VStack(alignment: .leading, spacing: 1) {
                                    HStack {
                                        Text("Job Search Hub").font(.system(size: 12, weight: .semibold)).foregroundStyle(ink)
                                        Spacer()
                                        Text("now").font(.system(size: 11)).foregroundStyle(secondaryInk)
                                    }
                                    Text("New version 0.1.252").font(.system(size: 12, weight: .medium)).foregroundStyle(ink)
                                    Text("Suggest a second route for unanswered applications, and 3 more").font(.system(size: 11)).foregroundStyle(secondaryInk).lineLimit(1)
                                }
                            }
                            .padding(Space.m)
                            .frame(width: 330)
                            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 16))
                            .background(Color.white.opacity(0.85), in: RoundedRectangle(cornerRadius: 16))
                            .shadow(color: .black.opacity(0.18), radius: 10, y: 4)
                            .padding(Space.m)
                        }
                    }
                }
                HStack(alignment: .top, spacing: Space.l) {
                    Callout(text: "A's label follows the version: “New version 0.1.252” once it's ready, “Installs when you quit” after that choice, “Installing…” during an install.")
                        .frame(width: 420)
                    HStack(spacing: Space.l) {
                        VersionLabel(foot: .ready("0.1.252")).frame(width: 194)
                        VersionLabel(foot: .onQuit("0.1.252")).frame(width: 194)
                        VersionLabel(foot: .installing).frame(width: 194)
                    }
                    .padding(Space.s)
                    .background(sidebarBackground, in: RoundedRectangle(cornerRadius: Radius.panel))
                }
                AppMenuStrip()
            }
        }
        .frame(width: 1410)
    }
}

/// The app menu's new item, and the palette's commands.
struct AppMenuStrip: View {
    var body: some View {
        HStack(alignment: .top, spacing: Space.xxl) {
            VStack(alignment: .leading, spacing: 0) {
                menuItem("About Job Search Hub")
                menuItem("Check for New Version…", highlighted: true)
                Rectangle().fill(separator).frame(height: 1).padding(.vertical, 4)
                menuItem("Settings…", shortcut: "⌘,")
                Rectangle().fill(separator).frame(height: 1).padding(.vertical, 4)
                menuItem("Quit Job Search Hub", shortcut: "⌘Q")
            }
            .padding(5)
            .frame(width: 260)
            .background(surface, in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(separator))
            .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: Space.s) {
                    Image(systemName: "magnifyingglass").foregroundStyle(secondaryInk)
                    Text("version").font(.system(size: 14)).foregroundStyle(ink)
                    Spacer()
                }
                .padding(Space.m)
                Rectangle().fill(separator).frame(height: 1)
                paletteRow("arrow.down.circle", "Install new version", "0.1.252", highlighted: true)
                paletteRow("list.bullet.rectangle", "What's new", "8 changes")
                paletteRow("arrow.clockwise", "Check for new version", "")
            }
            .padding(.bottom, Space.s)
            .frame(width: 380)
            .background(surface, in: RoundedRectangle(cornerRadius: 12))
            .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(separator))
            .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
            Callout(text: "With A: Job Search Hub › Check for New Version… checks right away and opens Settings › Version. ⌘K finds the same commands.")
                .frame(width: 300)
        }
    }

    private func menuItem(_ title: String, shortcut: String = "", highlighted: Bool = false) -> some View {
        HStack {
            Text(title).font(.system(size: 13)).foregroundStyle(highlighted ? .white : ink)
            Spacer()
            Text(shortcut).font(.system(size: 12)).foregroundStyle(highlighted ? .white : secondaryInk)
        }
        .padding(.horizontal, 9).padding(.vertical, 3)
        .background(highlighted ? Tone.accent.color : .clear, in: RoundedRectangle(cornerRadius: 4))
    }

    private func paletteRow(_ symbol: String, _ title: String, _ detail: String, highlighted: Bool = false) -> some View {
        HStack(spacing: Space.s) {
            Image(systemName: symbol).foregroundStyle(Tone.accent.color).frame(width: 18)
            Text(title).font(.system(size: 13)).foregroundStyle(ink)
            Spacer()
            Text(detail).font(.system(size: 11)).foregroundStyle(secondaryInk)
        }
        .padding(.horizontal, Space.m).padding(.vertical, 6)
        .background(highlighted ? Tone.accent.color.opacity(0.12) : .clear, in: RoundedRectangle(cornerRadius: 6))
        .padding(.horizontal, Space.xs)
    }
}

// MARK: - Settings › Version

struct VersionReady: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            FormSection {
                HStack(spacing: Space.m) {
                    HubIcon(size: 40)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Job Search Hub 0.1.247").font(.system(size: 14, weight: .semibold)).foregroundStyle(ink)
                        Text("Installed yesterday at 18:12 · server 0.1.247").font(.system(size: 11)).foregroundStyle(secondaryInk)
                    }
                }
            }
            FormSection(header: "New version") {
                HStack {
                    Image(systemName: "arrow.down.circle.fill").foregroundStyle(Tone.accent.color)
                    Text("0.1.252 is ready to install").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                    Spacer()
                    Text("3 releases · 8 changes").font(.system(size: 11)).foregroundStyle(secondaryInk)
                }
                ChangeGroup(title: "New", lines: newChanges)
                ChangeGroup(title: "Fixed", lines: fixedChanges)
                LinkText(text: "Show all 8 changes")
                Rectangle().fill(separator).frame(height: 1)
                Prose(text: "The server restarts for about 10 seconds. This version changes the database, so a copy is saved first. The phone change installs on the phone.", color: secondaryInk)
                HStack(spacing: Space.s) {
                    Spacer()
                    SecondaryButton(title: "Install when I quit")
                    PrimaryButton(title: "Install now…")
                }
            }
            FormSection {
                HStack(alignment: .top) {
                    Text("Install new versions").font(.system(size: 12)).foregroundStyle(ink)
                    Spacer()
                    VStack(alignment: .leading, spacing: 4) {
                        Radio(title: "When I choose", isOn: true)
                        Radio(title: "At night, when nothing is running", isOn: false)
                    }
                }
                Rectangle().fill(separator).frame(height: 1)
                HStack {
                    Text("Previous version: 0.1.244").font(.system(size: 12)).foregroundStyle(ink)
                    Spacer()
                    SecondaryButton(title: "Go back to 0.1.244…")
                }
            }
            HStack(spacing: Space.l) {
                LinkText(text: "Check now")
                LinkText(text: "Show install log")
                LinkText(text: "Release notes on GitHub")
            }
            .padding(.leading, 4)
        }
    }
}

/// The top of Settings › Version in one of its other states.
struct VersionState: View {
    let symbol: String
    let tone: Tone
    let title: String
    let text: String
    var action: String?
    var spinner = false
    var body: some View {
        FormSection {
            HStack(alignment: .top, spacing: Space.s) {
                if spinner { Spinner(size: 14).padding(.top, 2) } else {
                    Image(systemName: symbol).font(.system(size: 15)).foregroundStyle(tone.color)
                }
                VStack(alignment: .leading, spacing: 3) {
                    Text(title).font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                    Prose(text: text, color: secondaryInk)
                }
                Spacer(minLength: Space.s)
                if let action { SecondaryButton(title: action) }
            }
        }
        .frame(width: 420)
    }
}

struct VersionBoard: View {
    var body: some View {
        Board(title: "Settings › Version", subtitle: "A new tab: what's running, what's ready and what's in it, when to install, and the way back.") {
            HStack(alignment: .top, spacing: Space.xxl) {
                Captioned(mark: "1", title: "A version is ready", text: "What's new spans every release since the running one, tagged Mac, Server or Phone.") {
                    SettingsFrame(height: 640) { VersionReady() }
                }
                Captioned(mark: "2", title: "Its other states", text: "The section under the header, as it reads in each case.") {
                    VStack(alignment: .leading, spacing: Space.m) {
                        VersionState(symbol: "", tone: .neutral, title: "Checking for a new version…", text: "Asking GitHub for the newest Mac release.", spinner: true)
                        VersionState(symbol: "checkmark.circle.fill", tone: .positive, title: "Job Search Hub is up to date", text: "0.1.252 is the newest release. Checked 5 minutes ago.", action: "Check now")
                        VersionState(symbol: "hammer.circle.fill", tone: .neutral, title: "Local build of abc1234", text: "Built from a checkout. New versions don't install over it by themselves.", action: "Install 0.1.252…")
                        VersionState(symbol: "exclamationmark.triangle.fill", tone: .negative, title: "0.1.252 didn't pass its checks", text: "Its signature isn't from your team, so it was deleted and not offered.", action: "Show log")
                        VersionState(symbol: "wifi.exclamationmark", tone: .caution, title: "Couldn't check for new versions since yesterday", text: "GitHub couldn't be reached. The app keeps trying every hour.", action: "Check now")
                        VersionState(symbol: "arrow.uturn.backward.circle.fill", tone: .caution, title: "0.1.252 was withdrawn", text: "It was pulled after release. Going back to 0.1.247 loses nothing.", action: "Go back…")
                        Callout(text: "A version that's still downloading is never shown: the owner hears about it once it's ready, so they never wait on one.")
                            .frame(width: 420)
                    }
                }
            }
        }
        .frame(width: 1110)
    }
}

// MARK: - Installing

struct InstallSheet: View {
    var body: some View {
        SheetBody(title: "Install 0.1.252") {
            Prose(text: "The hub's server restarts, which takes about 10 seconds, and Job Search Hub reopens. Your phone shows the hub as offline meanwhile.", color: secondaryInk)
            Text("Running now").font(.system(size: 11, weight: .semibold)).foregroundStyle(secondaryInk).padding(.top, 2)
            VStack(spacing: 2) {
                RunningRow(state: .working, title: "Claude session · Acme", detail: "working")
                RunningRow(state: .working, title: "Researching Initech", detail: "agent run, 2 min")
                RunningRow(state: .waiting, title: "Profile has unsaved edits", detail: "Open")
                RunningRow(state: .idle, title: "1 idle Claude session", detail: "reopens where it was")
            }
            Prose(text: "This version changes the database. A copy is saved first.", color: secondaryInk)
        } buttons: {
            SecondaryButton(title: "Cancel")
            SecondaryButton(title: "Install anyway…")
            PrimaryButton(title: "Install when these finish")
        }
    }
}

struct WaitingSheet: View {
    var body: some View {
        SheetBody(title: "Installing 0.1.252 when these finish") {
            Prose(text: "The hub isn't starting new work. You can keep working; a session you message again keeps the install waiting.", color: secondaryInk)
            VStack(spacing: 2) {
                RunningRow(state: .working, title: "Claude session · Acme", detail: "working, 3 min")
                RunningRow(state: .done, title: "Researching Initech", detail: "finished")
                RunningRow(state: .done, title: "Profile has unsaved edits", detail: "saved")
            }
        } buttons: {
            SecondaryButton(title: "Cancel install")
            SecondaryButton(title: "Install anyway…")
        }
    }
}

struct StepsPanel: View {
    var body: some View {
        WindowFrame {
            VStack(alignment: .leading, spacing: Space.m) {
                HStack(spacing: Space.m) {
                    HubIcon(size: 36)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Installing Job Search Hub 0.1.252").font(.system(size: 13, weight: .semibold)).foregroundStyle(ink)
                        Text("Job Search Hub reopens when it's done").font(.system(size: 11)).foregroundStyle(secondaryInk)
                    }
                }
                VStack(alignment: .leading, spacing: 6) {
                    step(.done, "Work finished")
                    step(.done, "Server stopped")
                    step(.working, "Updating the database…")
                    step(.todo, "Checking it works")
                    step(.todo, "Reopening Job Search Hub")
                }
                ProgressBar(value: 0.55)
            }
            .padding(Space.l)
            .frame(width: 330)
        }
    }

    enum StepState { case done, working, todo }

    private func step(_ state: StepState, _ title: String) -> some View {
        HStack(spacing: Space.s) {
            Group {
                switch state {
                case .done: Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.positive.color)
                case .working: Spinner(size: 12)
                case .todo: Image(systemName: "circle").foregroundStyle(tertiaryInk)
                }
            }
            .font(.system(size: 13)).frame(width: 16)
            Text(title).font(.system(size: 12, weight: state == .working ? .semibold : .regular)).foregroundStyle(state == .todo ? secondaryInk : ink)
        }
    }
}

struct InstallBoard: View {
    var body: some View {
        Board(title: "Installing", subtitle: "The sheet lists what's running and waits for it; the helper shows the steps once the app has quit; the new app says what's new.") {
            VStack(alignment: .leading, spacing: Space.xxl) {
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "1", title: "Install now…", text: "Built from what's running. The default waits for it.", width: 640) {
                        SheetOver(foot: .ready("0.1.252")) { InstallSheet() }
                    }
                    Captioned(mark: "2", title: "Waiting for work", text: "Each item ticks off as it ends. Install anyway first says what it costs.", width: 640) {
                        SheetOver(foot: .installing) { WaitingSheet() }
                    }
                }
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "3", title: "The steps", text: "The app has quit. hub-update keeps this small window up until the new app opens.", width: 640) {
                        ZStack {
                            RoundedRectangle(cornerRadius: 14).fill(Color(hex: 0xC9CAD6)).frame(width: 640, height: 430)
                            StepsPanel()
                        }
                    }
                    Captioned(mark: "4", title: "Now on 0.1.252", text: "Same windows, sessions resumed in their tabs, and a banner where the connection banner goes.", width: 640) {
                        MainWindow(sessionState: "Idle") {
                            PageBanner(symbol: "checkmark.circle.fill", text: "Now on 0.1.252", action: "What's new", tone: .positive)
                                .padding(.bottom, Space.xs)
                        }
                    }
                }
                HStack(alignment: .top, spacing: Space.l) {
                    Callout(text: "Install when I quit: the same, when the owner quits with ⌘Q. The sheet shows only if something is running; otherwise the install runs after the app quits and leaves it closed.")
                        .frame(width: 520)
                    Callout(text: "Install anyway… says first: “The Acme session's turn stops mid-way; its conversation reopens with the new version. Researching Initech stops; run it again afterwards.”")
                        .frame(width: 520)
                }
            }
        }
        .frame(width: 1410)
    }
}

// MARK: - When it goes wrong

struct RecoveryBoard: View {
    var body: some View {
        Board(title: "When something goes wrong", subtitle: "The hub always ends on a version that works, and says what happened and what it cost.") {
            VStack(alignment: .leading, spacing: Space.xxl) {
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "1", title: "The install rolled back", text: "The new server didn't start. The old version is back, database included.", width: 640) {
                        MainWindow(sessionState: "Idle") {
                            PageBanner(symbol: "arrow.uturn.backward.circle.fill", text: "0.1.252 couldn't start, so the hub went back to 0.1.247. Nothing was lost.", action: "Show log", tone: .caution)
                                .padding(.bottom, Space.xs)
                        }
                    }
                    Captioned(mark: "2", title: "A crash in the first day", text: "On the next launch, from the crash report or an unclean exit.", width: 640) {
                        SheetOver {
                            SheetBody(title: "Job Search Hub 0.1.252 quit unexpectedly") {
                                Prose(text: "It's the version installed this morning. Going back to 0.1.247 also takes the database back to 09:02, before 0.1.252 changed it.", color: secondaryInk)
                                Prose(text: "Since then: 2 jobs found, 1 card moved. These would be lost.", color: ink)
                            } buttons: {
                                SecondaryButton(title: "Stay on 0.1.252")
                                PrimaryButton(title: "Go back to 0.1.247", tone: .caution)
                            }
                        }
                    }
                }
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "3", title: "Go back to 0.1.247…", text: "From Settings › Version, whenever a version breaks something later.", width: 640) {
                        SheetOver {
                            SheetBody(title: "Go back to 0.1.247?") {
                                Prose(text: "Going back also takes the database back to today, 14:02, before 0.1.252 changed it.", color: secondaryInk)
                                FormSection {
                                    HStack(spacing: Space.l) {
                                        stat("3", "jobs found")
                                        stat("1", "card moved")
                                        stat("2", "people added")
                                        stat("4", "updates")
                                    }
                                }
                                Prose(text: "These would be lost. Or stay on 0.1.252 and wait for a fix: new versions arrive most days.", color: secondaryInk)
                            } buttons: {
                                SecondaryButton(title: "Cancel")
                                PrimaryButton(title: "Go back and lose these", tone: .negative)
                            }
                        }
                    }
                    Captioned(mark: "4", title: "A withdrawn version", text: "The release was marked a pre-release on GitHub. No migration since, so nothing is lost.", width: 640) {
                        MainWindow(sessionState: "Idle") {
                            PageBanner(symbol: "exclamationmark.triangle.fill", text: "0.1.252 was withdrawn. Going back to 0.1.247 loses nothing.", action: "Go back…", tone: .caution)
                                .padding(.bottom, Space.xs)
                        }
                    }
                }
            }
        }
        .frame(width: 1410)
    }

    private func stat(_ value: String, _ label: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(value).font(.system(size: 18, weight: .semibold)).foregroundStyle(ink)
            Text(label).font(.system(size: 11)).foregroundStyle(secondaryInk)
        }
    }
}

// MARK: - Android

enum M3 {
    static let primary = Color(hex: 0x4B49D6)
    static let onPrimary = Color.white
    static let primaryContainer = Color(hex: 0xE2DFFF)
    static let onPrimaryContainer = Color(hex: 0x14105E)
    static let surface = Color(hex: 0xFCF8FF)
    static let surfaceContainer = Color(hex: 0xF0ECF6)
    static let surfaceContainerHigh = Color(hex: 0xEAE6F0)
    static let onSurface = Color(hex: 0x1B1B21)
    static let onSurfaceVariant = Color(hex: 0x47464F)
    static let outline = Color(hex: 0xC8C5D0)
    static let error = Color(hex: 0xBA1A1A)
}

struct AndroidPhone<Content: View>: View {
    var nav: String? = "Today"
    @ViewBuilder let content: Content
    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("9:41").font(.system(size: 13, weight: .medium))
                Spacer()
                Image(systemName: "wifi").font(.system(size: 11))
                Image(systemName: "battery.75percent").font(.system(size: 13))
            }
            .foregroundStyle(M3.onSurface)
            .padding(.horizontal, 22).padding(.top, 12).padding(.bottom, 6)
            content
            Spacer(minLength: 0)
            if let nav { NavigationBar(selected: nav) }
            Capsule().fill(M3.onSurface.opacity(0.4)).frame(width: 110, height: 4).padding(.vertical, 8)
        }
        .frame(width: 330, height: 700)
        .background(M3.surface)
        .clipShape(RoundedRectangle(cornerRadius: 36, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 36, style: .continuous).strokeBorder(Color.black.opacity(0.8), lineWidth: 6))
        .shadow(color: .black.opacity(0.15), radius: 14, y: 6)
    }
}

struct NavigationBar: View {
    let selected: String
    var body: some View {
        HStack {
            item("sun.max", "Today")
            item("checklist", "Decide")
            item("rectangle.split.3x1", "Pipeline")
            item("briefcase", "Jobs")
        }
        .padding(.vertical, Space.s)
        .background(M3.surfaceContainer)
    }

    private func item(_ symbol: String, _ title: String) -> some View {
        VStack(spacing: 4) {
            Image(systemName: symbol).font(.system(size: 16))
                .frame(width: 56, height: 28)
                .background(title == selected ? M3.primaryContainer : .clear, in: Capsule())
            Text(title).font(.system(size: 11, weight: title == selected ? .semibold : .medium))
        }
        .foregroundStyle(title == selected ? M3.onPrimaryContainer : M3.onSurfaceVariant)
        .frame(maxWidth: .infinity)
    }
}

struct TopBar: View {
    let title: String
    var back = false
    var body: some View {
        HStack(spacing: Space.m) {
            if back { Image(systemName: "arrow.left").font(.system(size: 16)) }
            Text(title).font(.system(size: back ? 20 : 26, weight: back ? .regular : .semibold))
            Spacer()
            if !back { Image(systemName: "gearshape").font(.system(size: 17)) }
        }
        .foregroundStyle(M3.onSurface)
        .padding(.horizontal, 18).padding(.vertical, 12)
    }
}

struct M3Button: View {
    enum Kind { case filled, tonal, text }
    let title: String
    var kind = Kind.filled
    var body: some View {
        Text(title).font(.system(size: 13, weight: .semibold))
            .padding(.horizontal, kind == .text ? 10 : 18).padding(.vertical, 9)
            .foregroundStyle(kind == .filled ? M3.onPrimary : kind == .tonal ? M3.onPrimaryContainer : M3.primary)
            .background(kind == .filled ? M3.primary : kind == .tonal ? M3.primaryContainer : .clear, in: Capsule())
    }
}

struct PhoneCard<Content: View>: View {
    var tint = M3.surfaceContainer
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) { content }
            .padding(Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(tint, in: RoundedRectangle(cornerRadius: 20))
            .padding(.horizontal, Space.m)
    }
}

struct PhoneText: View {
    let text: String
    var size: CGFloat = 13
    var weight = Font.Weight.regular
    var color = M3.onSurfaceVariant
    var body: some View {
        Text(text).font(.system(size: size, weight: weight)).foregroundStyle(color).fixedSize(horizontal: false, vertical: true)
    }
}

struct TodayStandInPhone: View {
    var body: some View {
        VStack(spacing: Space.s) {
            ForEach(0 ..< 2, id: \.self) { index in
                PhoneCard {
                    PhoneText(text: ["Follow up with Acme", "Staff Engineer · Initech"][index], size: 14, weight: .semibold, color: M3.onSurface)
                    RoundedRectangle(cornerRadius: 3).fill(M3.outline.opacity(0.6)).frame(height: 7)
                    RoundedRectangle(cornerRadius: 3).fill(M3.outline.opacity(0.6)).frame(width: 160, height: 7)
                }
            }
        }
    }
}

struct VersionCard: View {
    var downloading = false
    var body: some View {
        PhoneCard(tint: M3.primaryContainer) {
            HStack(spacing: Space.s) {
                Image(systemName: "arrow.down.circle.fill").font(.system(size: 18)).foregroundStyle(M3.primary)
                PhoneText(text: downloading ? "Updating to 0.1.253…" : "Version 0.1.253 is ready", size: 15, weight: .semibold, color: M3.onPrimaryContainer)
            }
            PhoneText(text: "• Snooze a follow-up from its notification\n• Show a job's salary range on its card", color: M3.onPrimaryContainer)
            if downloading {
                ProgressBar(value: 0.7).padding(.top, 4)
            } else {
                HStack {
                    Spacer()
                    M3Button(title: "Later", kind: .text)
                    M3Button(title: "Update")
                }
            }
        }
    }
}

struct AndroidBoard: View {
    var body: some View {
        Board(title: "On the phone", subtitle: "The phone checks GitHub for its own releases, downloads and checks the APK, and installs it through Android's installer.") {
            VStack(alignment: .leading, spacing: Space.xl) {
                HStack(alignment: .top, spacing: Space.xxl) {
                    Captioned(mark: "1", title: "Today's card", text: "Once the APK is downloaded and checked. Later hides it until the next release.", width: 330, captionHeight: 78) {
                        AndroidPhone {
                            TopBar(title: "Today")
                            VersionCard()
                            TodayStandInPhone().padding(.top, Space.s)
                        }
                    }
                    Captioned(mark: "2", title: "Settings › App version", text: "The installed version, the newest, and what's new.", width: 330, captionHeight: 78) {
                        AndroidPhone(nav: nil) {
                            TopBar(title: "Settings", back: true)
                            settingsList
                        }
                    }
                    Captioned(mark: "3", title: "Android's installer", text: "Update shows its own progress, then the system's confirmation. The first time, Android asks to allow installs from the hub.", width: 330, captionHeight: 78) {
                        ZStack {
                            AndroidPhone {
                                TopBar(title: "Today")
                                VersionCard(downloading: true)
                                TodayStandInPhone().padding(.top, Space.s)
                            }
                            .overlay(RoundedRectangle(cornerRadius: 36, style: .continuous).fill(.black.opacity(0.32)).padding(6))
                            VStack(alignment: .leading, spacing: Space.m) {
                                HStack(spacing: Space.s) {
                                    HubIcon(size: 28)
                                    PhoneText(text: "Job Search Hub", size: 15, weight: .semibold, color: M3.onSurface)
                                }
                                PhoneText(text: "Do you want to update this app?")
                                HStack {
                                    Spacer()
                                    M3Button(title: "Cancel", kind: .text)
                                    M3Button(title: "Update", kind: .text)
                                }
                            }
                            .padding(Space.xl)
                            .frame(width: 280)
                            .background(M3.surfaceContainerHigh, in: RoundedRectangle(cornerRadius: 28))
                        }
                    }
                    Captioned(mark: "4", title: "Too old for the hub", text: "When the server answers 426, a full screen replaces the errors.", width: 330, captionHeight: 78) {
                        AndroidPhone(nav: nil) {
                            VStack(spacing: Space.l) {
                                Spacer().frame(height: 120)
                                Image(systemName: "arrow.down.app.fill").font(.system(size: 52)).foregroundStyle(M3.primary)
                                PhoneText(text: "This app is too old for your hub", size: 22, weight: .semibold, color: M3.onSurface)
                                    .multilineTextAlignment(.center)
                                PhoneText(text: "The hub on your Mac is on 0.1.290 and no longer serves 0.1.212. Update to 0.1.288 to keep using it.")
                                    .multilineTextAlignment(.center)
                                M3Button(title: "Update to 0.1.288")
                            }
                            .padding(.horizontal, Space.xl)
                        }
                    }
                }
                HStack(alignment: .top, spacing: Space.l) {
                    Callout(text: "No notification per release. One, on the Hub channel, only when the phone is too old for the hub, or a ready version has waited a week.")
                        .frame(width: 440)
                    Callout(text: "On the Mac, Settings › Phones shows each phone's version beside its name, and “0.1.253 is out” when one is behind.")
                        .frame(width: 440)
                    Callout(text: "After the app has installed itself once, Android 12 and later may let it update without the confirmation. The ticket checks it on the owner's phone.")
                        .frame(width: 440)
                }
            }
        }
        .frame(width: 1530)
    }

    private var settingsList: some View {
        VStack(alignment: .leading, spacing: 0) {
            row("laptopcomputer", "Paired hub", "hub.example.ts.net")
            row("bell", "Notifications", "On")
            Text("App version").font(.system(size: 13, weight: .semibold)).foregroundStyle(M3.primary)
                .padding(.horizontal, 18).padding(.top, Space.l).padding(.bottom, Space.s)
            PhoneCard {
                PhoneText(text: "0.1.248 · 0.1.253 is ready", size: 15, weight: .semibold, color: M3.onSurface)
                PhoneText(text: "New")
                PhoneText(text: "• Snooze a follow-up from its notification\n• Show a job's salary range on its card", color: M3.onSurface)
                PhoneText(text: "Fixed")
                PhoneText(text: "• Keep the Decide list's place after a swipe", color: M3.onSurface)
                HStack {
                    M3Button(title: "Check now", kind: .text)
                    Spacer()
                    M3Button(title: "Update")
                }
            }
            row("link.badge.plus", "Unpair this phone", "")
                .padding(.top, Space.m)
        }
    }

    private func row(_ symbol: String, _ title: String, _ detail: String) -> some View {
        HStack(spacing: Space.l) {
            Image(systemName: symbol).font(.system(size: 17)).foregroundStyle(M3.onSurfaceVariant).frame(width: 24)
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.system(size: 15)).foregroundStyle(M3.onSurface)
                if !detail.isEmpty { Text(detail).font(.system(size: 12)).foregroundStyle(M3.onSurfaceVariant) }
            }
            Spacer()
        }
        .padding(.horizontal, 18).padding(.vertical, 12)
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
    write(JourneyBoard(), to: directory, name: "updates-journey.png")
    write(HearingBoard(), to: directory, name: "updates-macos-hearing.png")
    write(VersionBoard(), to: directory, name: "updates-macos-version.png")
    write(InstallBoard(), to: directory, name: "updates-macos-install.png")
    write(RecoveryBoard(), to: directory, name: "updates-macos-recovery.png")
    write(AndroidBoard(), to: directory, name: "updates-android.png")
}
