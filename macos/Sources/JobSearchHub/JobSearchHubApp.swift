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
            ContentView(initialPage: Self.pageFromLaunchArguments())
                .environment(connection)
                .frame(minWidth: 900, minHeight: 600)
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

    init(initialPage: Page) {
        _selectedPage = State(initialValue: initialPage)
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
            case let page?: PlaceholderPage(page: page)
            case nil: EmptyView()
            }
        }
    }
}

struct PlaceholderPage: View {
    let page: Page

    var body: some View {
        ContentUnavailableView(page.title, systemImage: page.symbolName, description: Text("Coming next."))
            .navigationTitle(page.title)
    }
}
