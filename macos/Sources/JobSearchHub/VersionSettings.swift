import AppKit
import JobSearchHubCore
import SwiftUI

/// Settings › Version: what's running, what's ready and what's in it, when
/// to install, and the way back. It reads the checker live; the snapshots
/// draw the same content from a page of their own.
struct VersionSettings: View {
    @Environment(NewVersionChecker.self) private var checker

    var body: some View {
        // Each minute, so "Checked 5 minutes ago" moves on.
        TimelineView(.everyMinute) { context in
            ScrollView {
                VersionSettingsContent(page: makePage(now: context.date), checkNow: checker.checkNow) {
                    NSWorkspace.shared.open(checker.updates.logURL)
                }
                .padding(Space.xl)
            }
        }
    }

    private func makePage(now: Date) -> VersionPage {
        VersionPage(
            facts: checker.facts, status: NewVersionStatus.make(checker.facts, now: now), installedAt: checker.installedAt,
            serverVersion: checker.runningServer?.version, previousVersion: checker.previousVersion,
            hasInstallLog: checker.hasInstallLog, runningReleaseURL: checker.runningReleaseURL, now: now
        )
    }
}

/// Everything Settings › Version shows, at one moment.
struct VersionPage {
    var facts: NewVersionFacts
    var status: NewVersionStatus
    var installedAt: Date?
    var serverVersion: String?
    var previousVersion: HubVersion?
    var hasInstallLog: Bool
    var runningReleaseURL: URL?
    var now: Date
}

/// The tab's content, drawn without AppKit controls (buttons and radios
/// are SwiftUI shapes), so `ImageRenderer` can draw it offscreen too.
struct VersionSettingsContent: View {
    /// Until the installer ships, the install buttons say why they wait.
    static let installerHelp = "Installing from the app comes with a later version. Until then, macos/Scripts/install-app.sh installs a build."
    static let goBackHelp = "Going back comes with a later version of the app."

    let page: VersionPage
    let checkNow: () -> Void
    let showLog: () -> Void
    @State private var isShowingAllChanges: Bool
    @Environment(\.openURL) private var openURL

    init(page: VersionPage, isShowingAllChanges: Bool = false, checkNow: @escaping () -> Void, showLog: @escaping () -> Void) {
        self.page = page
        _isShowingAllChanges = State(initialValue: isShowingAllChanges)
        self.checkNow = checkNow
        self.showLog = showLog
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.l) {
            header
            if page.status != .ready {
                statusCard
            }
            if let ready = page.facts.ready {
                VStack(alignment: .leading, spacing: Space.s) {
                    Text("New version").font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
                        .padding(.leading, Space.xs)
                    ReadyVersionCard(ready: ready, isShowingAllChanges: $isShowingAllChanges)
                }
            }
            preferences
            footer
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }

    // MARK: Header

    private var header: some View {
        HStack(spacing: Space.m) {
            Image(nsImage: NSWorkspace.shared.icon(forFile: Bundle.main.bundlePath))
                .resizable()
                .frame(width: 48, height: 48)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text("Job Search Hub \(page.facts.running.description)").font(.title3.weight(.semibold))
                Text(headerDetail).font(.hubSecondary).foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
        }
        .hubCard()
    }

    private var headerDetail: String {
        var parts: [String] = []
        if let installedAt = page.installedAt {
            parts.append("Installed \(installedAt.formatted(date: .abbreviated, time: .shortened))")
        }
        parts.append(page.serverVersion.map { "server \($0)" } ?? "server not reached")
        return parts.joined(separator: " · ")
    }

    // MARK: Status

    @ViewBuilder
    private var statusCard: some View {
        switch page.status {
        case .checking:
            StatusCard(symbol: "arrow.triangle.2.circlepath", tone: .accent, title: "Checking for a new version…",
                       detail: "Asking GitHub for the newest Mac release.") { EmptyView() }
        case let .upToDate(newest, checkedAt):
            StatusCard(symbol: "checkmark.circle.fill", tone: .positive, title: "Job Search Hub is up to date",
                       detail: describeUpToDate(newest: newest, checkedAt: checkedAt)) { checkNowButton }
        case let .localBuild(commit):
            StatusCard(symbol: "hammer.circle.fill", tone: .neutral, title: commit.map { "Local build of \($0)" } ?? "Local build",
                       detail: "Built from a checkout. New versions don't install over it by themselves.") {
                if let ready = page.facts.ready {
                    Button("Install \(ready.version.description)…") {}
                        .buttonStyle(VersionCapsuleStyle())
                        .disabled(true)
                        .help(Self.installerHelp)
                } else {
                    checkNowButton
                }
            }
        case .ready:
            EmptyView()
        case let .failedChecks(failed):
            StatusCard(symbol: "exclamationmark.triangle.fill", tone: .negative, title: failed.title, detail: failed.problem.explanation) {
                Button("Show log", action: showLog)
                    .buttonStyle(VersionCapsuleStyle())
                    .disabled(!page.hasInstallLog)
            }
        case let .cannotCheck(since):
            StatusCard(symbol: "wifi.exclamationmark", tone: .caution, title: "Couldn't check for new versions since \(describeDay(since))",
                       detail: "GitHub couldn't be reached. The app keeps trying every hour.") { checkNowButton }
        case let .withdrawn(version):
            StatusCard(symbol: "arrow.uturn.backward.circle.fill", tone: .caution, title: "\(version.description) was withdrawn",
                       detail: "It was pulled after its release. The next release replaces it.") {
                Button("Go back…") {}
                    .buttonStyle(VersionCapsuleStyle())
                    .disabled(true)
                    .help(Self.goBackHelp)
            }
        }
    }

    private var checkNowButton: some View {
        Button("Check now", action: checkNow)
            .buttonStyle(VersionCapsuleStyle())
            .disabled(page.facts.isChecking)
    }

    private func describeUpToDate(newest: HubVersion?, checkedAt: Date?) -> String {
        let release = newest.map { "\($0.description) is the newest release." } ?? "No Mac release is out yet."
        guard let checkedAt else { return "\(release) Not checked yet." }
        let ago = RelativeDateTimeFormatter().localizedString(for: checkedAt, relativeTo: page.now)
        return "\(release) Checked \(ago)."
    }

    /// "today", "yesterday", or the day: "3 Oct".
    private func describeDay(_ date: Date) -> String {
        let calendar = Calendar.current
        let days = calendar.dateComponents([.day], from: calendar.startOfDay(for: date), to: calendar.startOfDay(for: page.now)).day ?? 0
        switch days {
        case 0: return "today"
        case 1: return "yesterday"
        default: return date.formatted(.dateTime.day().month(.abbreviated))
        }
    }

    // MARK: Preferences

    private var preferences: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .top) {
                Text("Install new versions")
                Spacer(minLength: Space.l)
                VStack(alignment: .leading, spacing: Space.s) {
                    RadioRow(title: "When I choose", isOn: true)
                    RadioRow(title: "At night, when nothing is running", isOn: false)
                        .opacity(0.45)
                        .help("Night installs come with a later version of the app.")
                }
            }
            Divider()
            HStack {
                Text(page.previousVersion.map { "Previous version: \($0.description)" } ?? "No previous version on this Mac")
                    .foregroundStyle(page.previousVersion == nil ? .secondary : .primary)
                Spacer(minLength: Space.s)
                if let previous = page.previousVersion {
                    Button("Go back to \(previous.description)…") {}
                        .buttonStyle(VersionCapsuleStyle())
                        .disabled(true)
                        .help(Self.goBackHelp)
                }
            }
        }
        .hubCard()
    }

    // MARK: Footer

    private var footer: some View {
        HStack(spacing: Space.l) {
            Button("Check now", action: checkNow)
                .disabled(page.facts.isChecking)
            Button("Show install log", action: showLog)
                .disabled(!page.hasInstallLog)
            Button("Release notes on GitHub") {
                openURL(page.facts.ready?.releaseURL ?? page.runningReleaseURL ?? GitHubRepository.releasesPageURL)
            }
            Spacer(minLength: 0)
        }
        .buttonStyle(VersionLinkStyle())
        .padding(.horizontal, Space.xs)
    }
}

/// One of the tab's states, in a card: a symbol in its tone, what's so, and
/// what to do about it.
private struct StatusCard<Action: View>: View {
    let symbol: String
    let tone: Tone
    let title: String
    let detail: String
    @ViewBuilder let action: () -> Action

    var body: some View {
        HStack(alignment: .top, spacing: Space.m) {
            Image(systemName: symbol)
                .font(.title3)
                .foregroundStyle(tone.color)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Space.xs) {
                Text(title).font(.hubSection)
                Text(detail).font(.hubSecondary).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: Space.s)
            action()
        }
        .hubCard()
        .accessibilityElement(children: .combine)
    }
}

/// The version ready to install, with what's new since the running one:
/// New and Fixed, each line tagged with the part of the hub it changes and
/// linked to its pull request, and the other changes behind *Show all*.
private struct ReadyVersionCard: View {
    let ready: ReadyVersion
    @Binding var isShowingAllChanges: Bool
    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(spacing: Space.s) {
                Image(systemName: "arrow.down.circle.fill").foregroundStyle(Tone.accent.color).accessibilityHidden(true)
                Text("\(ready.version.description) is ready to install").font(.hubSection)
                Spacer(minLength: Space.s)
                Text(ready.whatsNew.summary).font(.hubCaption).foregroundStyle(.secondary)
            }
            changes
            Divider()
            Text(ready.installSummary)
                .font(.hubSecondary)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: Space.s) {
                Spacer()
                Button("Install when I quit") {}
                    .buttonStyle(VersionCapsuleStyle())
                Button("Install now…") {}
                    .buttonStyle(VersionCapsuleStyle(isProminent: true))
            }
            .disabled(true)
            .help(VersionSettingsContent.installerHelp)
        }
        .hubCard()
    }

    @ViewBuilder
    private var changes: some View {
        let whatsNew = ready.whatsNew
        let showsOther = isShowingAllChanges || whatsNew.highlights.isEmpty
        if whatsNew.notes.isEmpty {
            Text("Its release notes list no changes.").foregroundStyle(.secondary)
        }
        ForEach(ChangeKind.allCases, id: \.self) { kind in
            let notes = whatsNew.getNotes(kind)
            if !notes.isEmpty && (kind != .other || showsOther) {
                VStack(alignment: .leading, spacing: Space.xs) {
                    Text(kind.title).font(.hubSecondary.weight(.semibold))
                    ForEach(notes, id: \.self) { note in row(note) }
                }
            }
        }
        if !whatsNew.highlights.isEmpty && !whatsNew.getNotes(.other).isEmpty {
            Button(isShowingAllChanges ? "Show fewer" : "Show all \(whatsNew.changeCount) changes") {
                isShowingAllChanges.toggle()
            }
            .buttonStyle(VersionLinkStyle())
        }
    }

    private func row(_ note: ChangeNote) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Text(note.partsLabel)
                .font(.hubCaption)
                .foregroundStyle(.secondary)
                .frame(width: 84, alignment: .leading)
            Text(note.text)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: Space.s)
            if let pullRequest = note.pullRequest, let url = note.pullRequestURL {
                Button("#\(pullRequest)") { openURL(url) }
                    .buttonStyle(VersionLinkStyle())
                    .help("Open pull request #\(pullRequest) on GitHub")
            }
        }
        .accessibilityElement(children: .combine)
    }
}

/// A radio button, drawn: the one choice there is until night installs.
private struct RadioRow: View {
    let title: String
    let isOn: Bool

    var body: some View {
        HStack(spacing: 6) {
            ZStack {
                Circle().strokeBorder(isOn ? Tone.accent.color : Color.secondary, lineWidth: 1.2)
                if isOn {
                    Circle().fill(Tone.accent.color).padding(3.5)
                }
            }
            .frame(width: 14, height: 14)
            Text(title)
        }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(isOn ? [.isSelected] : [])
    }
}

/// A capsule button drawn in SwiftUI; prominent for the main action.
private struct VersionCapsuleStyle: ButtonStyle {
    var isProminent = false
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.callout.weight(isProminent ? .semibold : .medium))
            .lineLimit(1)
            .padding(.horizontal, Space.m)
            .padding(.vertical, 5)
            .foregroundStyle(isProminent ? AnyShapeStyle(Color.white) : AnyShapeStyle(.primary))
            .background(isProminent ? AnyShapeStyle(Tone.accent.color) : AnyShapeStyle(.quinary), in: Capsule())
            .opacity(isEnabled ? (configuration.isPressed ? 0.7 : 1) : 0.45)
            .contentShape(Capsule())
    }
}

/// A text link drawn in SwiftUI, in the accent.
private struct VersionLinkStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(Tone.accent.color)
            .opacity(isEnabled ? (configuration.isPressed ? 0.6 : 1) : 0.45)
            .contentShape(Rectangle())
    }
}

/// The foot of the sidebar once a new version is ready: `New version
/// 0.1.252`. Clicking opens Settings › Version.
struct NewVersionLabel: View {
    let version: HubVersion
    let open: () -> Void

    var body: some View {
        Button(action: open) {
            HStack(spacing: Space.s) {
                Image(systemName: "arrow.down.circle.fill")
                    .foregroundStyle(Tone.accent.color)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 1) {
                    Text("New version \(version.description)").fontWeight(.medium)
                    Text("Ready to install").font(.hubCaption).foregroundStyle(.secondary)
                }
                Spacer(minLength: 0)
            }
            .padding(Space.s)
            .background(Tone.accent.fill, in: RoundedRectangle(cornerRadius: Radius.card))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, Space.s)
        .help("Show what's new in \(version.description) in Settings › Version")
        .accessibilityLabel("New version \(version.description), ready to install")
    }
}
