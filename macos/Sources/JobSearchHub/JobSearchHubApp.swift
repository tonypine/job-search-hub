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
            ContentView(initialPage: Self.pageFromLaunchArguments(), initialJobID: Self.jobFromLaunchArguments())
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

    /// `--job <id>` opens that job's details on the Jobs or Pipeline page.
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
    @State private var selectedPage: Page?
    let initialJobID: UUID?

    init(initialPage: Page, initialJobID: UUID?) {
        _selectedPage = State(initialValue: initialPage)
        self.initialJobID = initialJobID
    }

    var body: some View {
        NavigationSplitView {
            List(Page.allCases, selection: $selectedPage) { page in
                Label(page.title, systemImage: page.symbolName).tag(page)
            }
            .navigationSplitViewColumnWidth(min: 180, ideal: 200)
        } detail: {
            switch selectedPage {
            case .settings: SettingsPage()
            case .companies: CompaniesPage()
            case .profile: ProfilePage()
            case .jobs: JobsPage(initialJobID: initialJobID)
            case .pipeline: PipelinePage(initialJobID: initialJobID)
            case nil: EmptyView()
            }
        }
    }
}

