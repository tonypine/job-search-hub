import Foundation
import JobSearchHubCore
import SwiftUI

@main
struct JobSearchHubApp: App {
    @State private var connection: HubConnection

    init() {
        Self.importOwnerTokenIfAsked()
        _connection = State(initialValue: HubConnection())
    }

    var body: some Scene {
        WindowGroup("Job Search Hub") {
            ContentView(
                initialPage: Self.pageFromLaunchArguments(), initialJobID: Self.jobFromLaunchArguments(),
                opensSession: ProcessInfo.processInfo.arguments.contains("--session")
            )
                .environment(connection)
                .frame(minWidth: 900, minHeight: 600)
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

struct ContentView: View {
    @Environment(HubConnection.self) private var connection
    @State private var selectedPage: Page?
    @State private var focus: SessionFocus?
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
                        Label(page.title, systemImage: page.symbolName).tag(page)
                    }
                }
                if let client = connection.makeClient() {
                    SessionSidebarSection(client: client) { subject in open(subject) }
                }
            }
            .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            switch selectedPage {
            case .settings: SettingsPage()
            case .companies:
                CompaniesPage(initialCompanyID: focusedCompanyID, opensSession: focusedCompanyID != nil).id(focus?.id)
            case .profile: ProfilePage()
            case .jobs:
                JobsPage(initialJobID: focusedJobID ?? initialJobID, opensSession: focusedJobID != nil || opensSession).id(focus?.id)
            case .pipeline: PipelinePage(initialJobID: initialJobID)
            case nil: EmptyView()
            }
        }
    }

    private var focusedJobID: UUID? {
        if case let .job(id)? = focus?.subject { return id }
        return nil
    }

    private var focusedCompanyID: UUID? {
        if case let .company(id)? = focus?.subject { return id }
        return nil
    }

    /// Shows a session's job or company on its Session side.
    private func open(_ subject: ClaudeSessionSubject) {
        focus = SessionFocus(subject: subject)
        switch subject {
        case .job: selectedPage = .jobs
        case .company: selectedPage = .companies
        }
    }
}

