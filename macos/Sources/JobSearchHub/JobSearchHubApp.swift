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
        // A stable id keeps the scene's identity, and so its saved window state,
        // from changing with the content's modifiers.
        WindowGroup("Job Search Hub", id: "main") {
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
        // The window can't be made smaller than the content's minimum, where
        // every page lays out.
        .windowResizability(.contentMinSize)
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
    /// page by page from the terminal. Without it the app opens on Today.
    private static func pageFromLaunchArguments() -> Page {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flagIndex = arguments.firstIndex(of: "--page"), flagIndex + 1 < arguments.count,
              let page = Page(rawValue: arguments[flagIndex + 1])
        else { return .today }
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

/// Where the owner was going when the Criteria page asked to save its
/// changes first.
struct PendingLeave {
    let go: @MainActor () -> Void
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
    @State private var requests = PageRequests()
    @State private var palette = PaletteModel()
    @State private var isShowingPalette = false
    /// What an action run from the palette did, or why it failed.
    @State private var toast: ToastMessage?
    @State private var actionError: HubFailure?
    /// The Criteria page's form, here so leaving the page with changes asks
    /// to save or discard them first.
    @State private var criteria = JobCriteriaEditor()
    /// Where the owner was going when the page asked; nil when it isn't asking.
    @State private var pendingLeave: PendingLeave?
    let initialJobID: UUID?
    let opensSession: Bool

    init(initialPage: Page, initialJobID: UUID?, opensSession: Bool) {
        _selectedPage = State(initialValue: initialPage)
        self.initialJobID = initialJobID
        self.opensSession = opensSession
    }

    var body: some View {
        NavigationSplitView {
            List(selection: Binding(get: { selectedPage }, set: { page in if page != selectedPage { leave { selectedPage = page } } })) {
                ForEach(SidebarGroup.allCases) { group in
                    if let title = group.title {
                        Section(title, isExpanded: isExpanded(group)) { rows(group) }
                    } else {
                        Section { rows(group) }
                    }
                }
            }
            // The hub's state, where the sessions were: they're in its popover.
            .safeAreaInset(edge: .bottom, spacing: 0) {
                HubStatusFooter(problem: connectionProblem) { subject in openSession(subject) }
            }
            .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            // The banner hangs on a container that always renders: a page
            // without a client renders nothing, which would take it along.
            // The page stays in the tree without one, so its state outlives
            // a hub address that is briefly invalid while being edited.
            ZStack {
                page
                if connectionProblem == .notSetUp {
                    ConnectToHubView()
                        .navigationTitle(selectedPage?.title ?? "Job Search Hub")
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .safeAreaInset(edge: .top, spacing: 0) {
                if let connectionProblem {
                    ConnectionBanner(problem: connectionProblem)
                }
            }
            .overlay(alignment: .bottom) {
                if actionError != nil {
                    HubErrorView($actionError)
                        .frame(maxWidth: 560)
                        .padding(Space.l)
                }
            }
            .toast($toast)
            .confirmationDialog(
                "Save the changes to your criteria?",
                isPresented: Binding(get: { pendingLeave != nil }, set: { if !$0 { pendingLeave = nil } }),
                presenting: pendingLeave
            ) { leaving in
                Button("Save") { Task { await saveCriteria(thenRun: leaving) } }
                    .keyboardShortcut(.defaultAction)
                Button("Discard", role: .destructive) {
                    criteria.revert()
                    leaving.go()
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text("You have \(SaveBar.describe(criteria.changeCount)) on Criteria. Discarded, they're gone.")
            }
        }
        .inspector(isPresented: Binding(get: { shownEntry != nil }, set: { if !$0 { details.hide() } })) {
            if let shownEntry, let client = connection.makeClient() {
                VStack(spacing: 0) {
                    InspectorNavigationBar(details: details)
                    DetailsInspectorContent(entry: shownEntry, client: client)
                }
                // The column tells the window the same sizes whatever it
                // shows and however wide it is: a frame with every bound set
                // takes the size it's offered, and the column's width range
                // stays with inspectorColumnWidth. Sizes that follow a job's
                // header, buttons and tabs can change the column's
                // constraints on every pass, and a window whose constraint
                // updates never settle is stopped by AppKit.
                .frame(
                    minWidth: 0, idealWidth: 480, maxWidth: .infinity,
                    minHeight: 0, idealHeight: 600, maxHeight: .infinity, alignment: .top
                )
                .inspectorColumnWidth(min: 360, ideal: 480, max: 720)
            }
        }
        .overlay(alignment: .top) {
            if isShowingPalette {
                ZStack(alignment: .top) {
                    // A click outside the palette closes it.
                    Color.black.opacity(0.06)
                        .ignoresSafeArea()
                        .onTapGesture { isShowingPalette = false }
                    CommandPalette(
                        model: palette, actions: PaletteAction.getAvailable(isModelWorkPaused: palette.isModelWorkPaused, unseenUpdates: unseen.count),
                        choose: choose
                    ) {
                        isShowingPalette = false
                    }
                    .padding(.top, Space.xxl)
                }
            }
        }
        .task(id: isShowingPalette) {
            if isShowingPalette, let client = connection.makeClient() {
                await palette.load(with: client)
            }
        }
        .focusedSceneValue(\.isShowingPalette, $isShowingPalette)
        .environment(details)
        .environment(replyDraft)
        .environment(requests)
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
        case .today: TodayPage { page in leave { selectedPage = page } }
        case .decide: DecidePage()
        case .updates: UpdatesPage()
        case .companies: CompaniesPage()
        case .people: PeoplePage()
        case .profile: ProfilePage()
        case .criteria: CriteriaPage(editor: criteria)
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
            HStack(spacing: Space.s) {
                Label(page.title, systemImage: page.symbolName)
                Spacer(minLength: 0)
                SidebarBadge(count: counts.getCount(for: page, unseen: unseen.count), isUrgent: counts.isUrgent(page))
            }
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

    /// Does what the palette's row says: opens a job, company or person in
    /// the inspector over the page shown, switches the page, or runs the
    /// action.
    private func choose(_ item: PaletteItem) {
        isShowingPalette = false
        switch item.target {
        case let .open(subject):
            details.show(subject, from: selectedPage ?? .today)
            if selectedPage == nil { selectedPage = .today }
        case let .page(page):
            leave {
                selectedPage = page
                requests.ask(.focusList, on: page)
            }
        case let .action(action):
            run(action)
        }
    }

    /// Runs what leaves the page shown, once the Criteria page's unsaved
    /// changes are saved or discarded; at once when there are none.
    private func leave(_ go: @escaping @MainActor () -> Void) {
        if selectedPage == .criteria, criteria.hasChanges {
            pendingLeave = PendingLeave(go: go)
        } else {
            go()
        }
    }

    /// Saves the criteria, then leaves; a refused save stays on the page,
    /// which says why, and so does a hub this app can't reach yet.
    private func saveCriteria(thenRun leaving: PendingLeave) async {
        guard let client = connection.makeClient() else {
            actionError = HubFailure("Couldn't save the criteria", advice: "The hub's address or token isn't set. Set them in Settings, then save again.")
            return
        }
        if await criteria.save(with: client) {
            toast = ToastMessage(text: "Saved your criteria")
            leaving.go()
        }
    }

    private func run(_ action: PaletteAction) {
        switch action {
        case .addCompany:
            leave {
                selectedPage = .companies
                requests.ask(.addCompany, on: .companies)
            }
        case .addCompanyFromSuggestions:
            leave {
                selectedPage = .companies
                requests.ask(.addCompanyFromSuggestions, on: .companies)
            }
        case .addJobByURL:
            leave {
                selectedPage = .jobs
                requests.ask(.addJobByURL, on: .jobs)
            }
        case .generateMissingCVs:
            guard let client = connection.makeClient() else { return }
            Task {
                do {
                    let queued = try await client.generateMissingCVs()
                    toast = queued == 0
                        ? ToastMessage(text: "No CVs are missing, or the hub is already making them.", tone: .neutral, symbol: "info.circle")
                        : ToastMessage(text: "Generating \(queued) \(queued == 1 ? "CV" : "CVs") in the background. Each appears in its job's details once printed.")
                } catch {
                    actionError = HubFailure("Couldn't start the missing CVs", error)
                }
            }
        case .pauseLocalModels, .resumeLocalModels:
            guard let client = connection.makeClient() else { return }
            let pausing = action == .pauseLocalModels
            Task {
                do {
                    _ = try await client.send("POST", pausing ? "v1/model-work/pause" : "v1/model-work/resume", body: EmptyBody(), as: ModelWork.self)
                    toast = pausing
                        ? ToastMessage(text: "Paused the local models. Background work waits until you resume it.", tone: .caution, symbol: "pause.circle.fill")
                        : ToastMessage(text: "Resumed the local models.", symbol: "play.circle.fill")
                } catch {
                    actionError = HubFailure(pausing ? "Couldn't pause the local models" : "Couldn't resume the local models", error)
                }
            }
        case .markAllUpdatesSeen:
            guard let client = connection.makeClient() else { return }
            Task {
                await unseen.markSeen(UpdateSelection(all: true), with: client)
                if unseen.count == 0 { toast = ToastMessage(text: "Marked all updates seen") }
            }
        }
    }

    /// Opens a session's job or company on its Session tab over the page
    /// shown, or the profile interview beside the Profile page.
    private func openSession(_ subject: ClaudeSessionSubject) {
        if subject == .profile {
            leave {
                selectedPage = .profile
                details.show(.profileInterview, from: .profile)
            }
        } else if let selectedPage {
            details.openSession(InspectorSubject(subject), from: selectedPage)
        }
    }
}

