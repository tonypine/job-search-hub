import JobSearchHubCore
import SwiftUI

@main
struct JobSearchHubApp: App {
    var body: some Scene {
        WindowGroup("Job Search Hub") {
            ContentView(initialPage: Self.pageFromLaunchArguments())
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
            if let selectedPage {
                PlaceholderPage(page: selectedPage)
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
