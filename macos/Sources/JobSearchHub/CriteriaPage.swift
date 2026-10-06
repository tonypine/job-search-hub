import JobSearchHubCore
import SwiftUI

/// The Criteria page's scopes: what the hub searches for and screens jobs
/// against, and the phases applications move through.
enum CriteriaScope: String, CaseIterable, Identifiable {
    case search
    case phases

    var id: String { rawValue }

    var title: String {
        switch self {
        case .search: "Search"
        case .phases: "Pipeline phases"
        }
    }
}

/// What the hub searches for and screens every job against, and the phases
/// applications move through: the settings that change what every page shows.
struct CriteriaPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var scope = CriteriaScope.search
    @State private var editor = JobCriteriaEditor()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    PageHeader {
                        TabStrip(items: CriteriaScope.allCases.map { TabStripItem(id: $0, title: $0.title) }, selection: $scope)
                    } trailing: {
                        EmptyView()
                    }
                    Form {
                        switch scope {
                        case .search: JobCriteriaSection(client: client, editor: editor)
                        case .phases: PipelinePhasesSection(client: client)
                        }
                    }
                    .formStyle(.grouped)
                }
                .task { await editor.load(with: client) }
            }
        }
        .navigationTitle("Criteria")
        .navigationSubtitle("What the hub searches for, and screens every job against")
    }
}
