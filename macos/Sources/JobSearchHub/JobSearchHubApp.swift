import Foundation
import JobSearchHubCore
import SwiftUI

@main
struct JobSearchHubApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var connection: HubConnection
    @State private var events = HubEventStream()
    @State private var unseen = UnseenUpdates()
    @State private var sidebarCounts = SidebarCounts()
    @State private var jobFinder: CompanyJobFinder
    @State private var research: CompanyResearch
    @State private var taskRunner: RemoteTaskRunner
    @State private var profileSeed = ProfileSeed()
    @State private var jobDecisions = JobDecisions()

    init() {
        Self.importOwnerTokenIfAsked()
        _connection = State(initialValue: HubConnection())
        let jobFinder = CompanyJobFinder()
        _jobFinder = State(initialValue: jobFinder)
        _research = State(initialValue: CompanyResearch(jobFinder: jobFinder))
        _taskRunner = State(initialValue: RemoteTaskRunner(jobFinder: jobFinder))
    }

    var body: some Scene {
        WindowGroup("Job Search Hub") {
            ContentView(
                initialPage: Self.pageFromLaunchArguments(), initialJobID: Self.jobFromLaunchArguments(),
                opensSession: ProcessInfo.processInfo.arguments.contains("--session")
            )
                .environment(connection)
                .environment(events)
                .environment(unseen)
                .environment(sidebarCounts)
                .environment(research)
                .environment(jobFinder)
                .environment(profileSeed)
                .environment(jobDecisions)
                .environment(taskRunner)
                .frame(minWidth: 900, minHeight: 600)
                // Hub Indigo marks you and your actions: selection, links, the primary button.
                .tint(.hubAccent)
                // The stream holds its client, so it starts again with a new
                // URL or token, and once the token arrives from the Keychain.
                .task(id: [connection.hubURLText, connection.token.value]) {
                    if let client = connection.makeClient() {
                        await events.run(with: client)
                    }
                }
                // Work the phone asked for: checked on each update the hub
                // announces, which includes a new request, and every minute.
                .task(id: HubWorkKey(revision: events.revision, hasToken: connection.hasToken)) {
                    if let client = connection.makeClient() {
                        await taskRunner.check(with: client)
                    }
                }
                .task {
                    while !Task.isCancelled {
                        try? await Task.sleep(for: .seconds(60))
                        if let client = connection.makeClient() {
                            await taskRunner.check(with: client)
                        }
                    }
                }
                // While the event stream is down, the connection is checked
                // every ten seconds, so the banner says why and goes once the
                // hub answers again.
                .task(id: ConnectionCheckKey(hubURLText: connection.hubURLText, token: connection.token.value, isStreamConnected: events.isConnected)) {
                    guard !events.isConnected else { return }
                    while !Task.isCancelled {
                        await connection.check()
                        try? await Task.sleep(for: .seconds(10))
                    }
                }
        }
        .defaultSize(width: 1400, height: 860)
        .commands {
            HubCommands(events: events)
        }

        // A session opened in a window of its own, out of the inspector's width.
        WindowGroup("Session", id: "session", for: ClaudeSessionSubject.self) { $subject in
            SessionWindow(subject: subject)
                .environment(connection)
                .frame(minWidth: 600, minHeight: 400)
                .tint(.hubAccent)
        }
        .defaultSize(width: 900, height: 700)

        Settings {
            SettingsWindow()
                .environment(connection)
                .tint(.hubAccent)
        }
    }

    /// `--page <name>` opens the app on that page, so a build can be checked
    /// page by page from the terminal.
    private static func pageFromLaunchArguments() -> Page {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flagIndex = arguments.firstIndex(of: "--page"), flagIndex + 1 < arguments.count,
              let page = Page(rawValue: arguments[flagIndex + 1])
        else { return .pipeline }
        return page
    }

    /// `--job <id>` opens that job's details on the Jobs or Pipeline page;
    /// with `--session`, the Jobs page opens it on its Session tab.
    private static func jobFromLaunchArguments() -> UUID? {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flagIndex = arguments.firstIndex(of: "--job"), flagIndex + 1 < arguments.count else { return nil }
        return UUID(uuidString: arguments[flagIndex + 1])
    }

    /// `--import-owner-token` saves HUB_OWNER_TOKEN from the app's environment
    /// into the Keychain, for setting up without typing the token:
    /// `open JobSearchHub.app --env HUB_OWNER_TOKEN=… --args --import-owner-token`.
    /// The app writes the item itself, which is what keeps later reads free of
    /// Keychain prompts.
    private static func importOwnerTokenIfAsked() {
        guard ProcessInfo.processInfo.arguments.contains("--import-owner-token"),
              let token = ProcessInfo.processInfo.environment["HUB_OWNER_TOKEN"], !token.isEmpty
        else { return }
        try? OwnerTokenKeychain.save(token)
    }
}

/// Keys a task that works with the hub on each update it announces, and once
/// the owner token arrives from the Keychain after launch.
struct HubWorkKey: Equatable {
    let revision: Int
    let hasToken: Bool
}

/// Keys the connection check: again with a new URL or token, and stopped
/// while the event stream shows the hub answers.
struct ConnectionCheckKey: Equatable {
    let hubURLText: String
    let token: String?
    let isStreamConnected: Bool
}

struct ContentView: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(SidebarCounts.self) private var counts
    @Environment(JobDecisions.self) private var decisions
    @State private var selectedPage: Page?
    /// The sidebar groups folded away, by raw value: the Hub's at first.
    @AppStorage("sidebarCollapsedGroups") private var collapsedGroups = SidebarGroup.allCases
        .filter { !$0.isExpandedByDefault }.map(\.rawValue).joined(separator: ",")
    @State private var details = DetailsInspector()
    @State private var replyDraft = RecruiterReplyDraft()
    let initialJobID: UUID?
    let opensSession: Bool

    init(initialPage: Page, initialJobID: UUID?, opensSession: Bool) {
        _selectedPage = State(initialValue: initialPage)
        self.initialJobID = initialJobID
        self.opensSession = opensSession
    }

    var body: some View {
        NavigationSplitView {
            List(selection: $selectedPage) {
                ForEach(SidebarGroup.allCases) { group in
                    if let title = group.title {
                        Section(title, isExpanded: isExpanded(group)) { rows(group) }
                    } else {
                        Section { rows(group) }
                    }
                }
                if let client = connection.makeClient() {
                    SessionSidebarSection(client: client) { subject in openSession(subject) }
                }
            }
            .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            page
                .safeAreaInset(edge: .top, spacing: 0) {
                    if let connectionProblem {
                        ConnectionBanner(problem: connectionProblem)
                    }
                }
        }
        .inspector(isPresented: Binding(get: { shownEntry != nil }, set: { if !$0 { details.hide() } })) {
            if let shownEntry, let client = connection.makeClient() {
                DetailsInspectorContent(entry: shownEntry, client: client)
                    .inspectorColumnWidth(min: 360, ideal: 480, max: 720)
                    .toolbar { InspectorToolbar(details: details) }
            }
        }
        .environment(details)
        .environment(replyDraft)
        .task(id: HubWorkKey(revision: events.revision, hasToken: connection.hasToken)) {
            if let client = connection.makeClient() {
                await unseen.refresh(with: client)
            }
        }
        .task(id: [events.revision, decisions.revision, connection.hasToken ? 1 : 0]) {
            if let client = connection.makeClient() {
                await counts.refresh(with: client)
            }
        }
        // A page opened in a folded group, from `--page` or a link, unfolds it.
        .onChange(of: selectedPage, initial: true) {
            if let group = selectedPage?.group { isExpanded(group).wrappedValue = true }
        }
    }

    @ViewBuilder
    private var page: some View {
        switch selectedPage {
        case .decide: DecidePage()
        case .updates: UpdatesPage()
        case .companies: CompaniesPage()
        case .recruiters: RecruitersPage()
        case .profile: ProfilePage()
        case .criteria: CriteriaPage()
        case .activity: ActivityPage()
        case .prompts: PromptsPage()
        case .modelLab: ModelLabPage()
        case .jobs: JobsPage(initialJobID: initialJobID, opensSession: opensSession)
        case .pipeline: PipelinePage(initialJobID: initialJobID)
        case nil: EmptyView()
        }
    }

    private func rows(_ group: SidebarGroup) -> some View {
        ForEach(group.pages) { page in
            Label(page.title, systemImage: page.symbolName)
                .badge(counts.getCount(for: page, unseen: unseen.count))
                .tag(page)
        }
    }

    private func isExpanded(_ group: SidebarGroup) -> Binding<Bool> {
        Binding(
            get: { !collapsedGroups.split(separator: ",").contains(Substring(group.rawValue)) },
            set: { isExpanded in
                var collapsed = Set(collapsedGroups.split(separator: ",").map(String.init))
                if isExpanded { collapsed.remove(group.rawValue) } else { collapsed.insert(group.rawValue) }
                collapsedGroups = collapsed.sorted().joined(separator: ",")
            }
        )
    }

    /// What keeps the app from the hub, which the banner above every page
    /// says; nil when nothing does.
    private var connectionProblem: ConnectionProblem? {
        ConnectionProblem.diagnose(
            isReadingToken: connection.token == .reading, hasClient: connection.makeClient() != nil,
            status: connection.status, isStreamConnected: events.isConnected
        )
    }

    private var shownEntry: InspectorEntry? {
        selectedPage.flatMap { details.getEntry(on: $0) }
    }

    /// Opens a session's job or company on its Session tab over the page
    /// shown, or the profile interview beside the Profile page.
    private func openSession(_ subject: ClaudeSessionSubject) {
        if subject == .profile {
            selectedPage = .profile
            details.show(.profileInterview, from: .profile)
        } else if let selectedPage {
            details.openSession(InspectorSubject(subject), from: selectedPage)
        }
    }
}

