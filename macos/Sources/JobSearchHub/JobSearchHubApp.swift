import Foundation
import JobSearchHubCore
import SwiftUI

@main
struct JobSearchHubApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var connection: HubConnection
    @State private var events = HubEventStream()
    @State private var unseen = UnseenUpdates()
    @State private var jobFinder: CompanyJobFinder
    @State private var research: CompanyResearch
    @State private var taskRunner: RemoteTaskRunner
    @State private var profileSeed = ProfileSeed()

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
                .environment(research)
                .environment(jobFinder)
                .environment(profileSeed)
                .frame(minWidth: 900, minHeight: 600)
                .task(id: connection.hubURLText) {
                    if let client = connection.makeClient() {
                        await events.run(with: client)
                    }
                }
                // Work the phone asked for: checked on each update the hub
                // announces, which includes a new request, and every minute.
                .task(id: events.revision) {
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
        }
        .defaultSize(width: 1400, height: 860)
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
    /// with `--session`, the Jobs page opens its session instead.
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

/// A request to show a job or a company, on its Details side or on its
/// Session side, where the session is resumed when it has ended. Each request
/// is new, so asking for the same one twice still opens it.
struct SubjectFocus: Equatable {
    let id = UUID()
    let subject: ClaudeSessionSubject
    let opensSession: Bool
}

struct ContentView: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @State private var selectedPage: Page?
    @State private var focus: SubjectFocus?
    @State private var details = DetailsInspector()
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
                Section {
                    ForEach(Page.allCases) { page in
                        Label(page.title, systemImage: page.symbolName)
                            .badge(page == .updates ? unseen.count : 0)
                            .tag(page)
                    }
                }
                if let client = connection.makeClient() {
                    SessionSidebarSection(client: client) { subject in open(subject, opensSession: true) }
                }
            }
            .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            switch selectedPage {
            case .settings: SettingsPage()
            case .updates:
                UpdatesPage(
                    onOpenJob: { open(.job($0), opensSession: false) },
                    onOpenCompany: { open(.company($0), opensSession: false) }
                )
            case .companies:
                CompaniesPage(initialCompanyID: focusedCompanyID, opensSession: focusedCompanyID != nil && focus?.opensSession == true).id(focus?.id)
            case .recruiters:
                RecruitersPage(onOpenCompany: { open(.company($0), opensSession: false) })
            case .profile: ProfilePage()
            case .prompts: PromptsPage()
            case .jobs:
                JobsPage(
                    initialJobID: focusedJobID ?? initialJobID,
                    opensSession: focusedJobID == nil ? opensSession : focus?.opensSession == true
                ).id(focus?.id)
            case .pipeline: PipelinePage(initialJobID: initialJobID)
            case nil: EmptyView()
            }
        }
        .inspector(isPresented: Binding(get: { shownDetails != nil }, set: { if !$0 { details.hide() } })) {
            if let shownDetails, let client = connection.makeClient() {
                DetailsInspectorContent(subject: shownDetails, client: client)
                    .inspectorColumnWidth(min: 360, ideal: 480, max: 720)
                    .toolbar { HideDetailsButton { details.hide() } }
            }
        }
        .environment(details)
        .task(id: events.revision) {
            if let client = connection.makeClient() {
                await unseen.refresh(with: client)
            }
        }
    }

    private var shownDetails: DetailsInspector.Subject? {
        selectedPage.flatMap { details.getSubject(on: $0) }
    }

    private var focusedJobID: UUID? {
        if case let .job(id)? = focus?.subject { return id }
        return nil
    }

    private var focusedCompanyID: UUID? {
        if case let .company(id)? = focus?.subject { return id }
        return nil
    }

    /// Shows a job or company, on its Session side when asked, or the
    /// profile interview beside the Profile page.
    private func open(_ subject: ClaudeSessionSubject, opensSession: Bool) {
        focus = SubjectFocus(subject: subject, opensSession: opensSession)
        switch subject {
        case .job: selectedPage = .jobs
        case .company: selectedPage = .companies
        case .profile:
            selectedPage = .profile
            details.show(.profileInterview, from: .profile)
        }
    }
}

