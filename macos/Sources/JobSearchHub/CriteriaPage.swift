import JobSearchHubCore
import SwiftUI

/// The Criteria page's scopes: what the hub searches for and screens jobs
/// against, the pay a job must reach, and the phases applications move
/// through.
enum CriteriaScope: String, CaseIterable, Identifiable {
    case search
    case pay
    case phases

    var id: String { rawValue }

    var title: String {
        switch self {
        case .search: "Search"
        case .pay: "Pay"
        case .phases: "Pipeline phases"
        }
    }
}

/// What the hub searches for and screens every job against, the pay it
/// must reach, and the phases applications move through: the settings that
/// change what every page shows. A save bar rises once a criterion changed,
/// on every scope, until it's saved or reverted.
struct CriteriaPage: View {
    @Environment(HubConnection.self) private var connection
    /// ContentView's, so it can ask before the page is left with changes.
    let editor: JobCriteriaEditor
    @State private var scope = CriteriaScope.search
    @State private var toast: ToastMessage?

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    PageHeader {
                        TabStrip(items: CriteriaScope.allCases.map { TabStripItem(id: $0, title: $0.title) }, selection: $scope)
                    } trailing: {
                        EmptyView()
                    }
                    switch scope {
                    case .search, .pay:
                        JobCriteriaForm(scope: scope, editor: editor)
                    case .phases:
                        Form { PipelinePhasesSection(client: client) }
                            .formStyle(.grouped)
                    }
                }
                .saveBar(changeCount: editor.changeCount, isSaving: editor.isSaving, revert: { editor.revert() }) {
                    if await editor.save(with: client) {
                        toast = ToastMessage(text: "Saved your criteria")
                    }
                }
                .toast($toast)
                // Unsaved edits are kept: the page is only left with them
                // saved or discarded.
                .task {
                    if !editor.hasChanges { await editor.load(with: client) }
                }
            }
        }
        .navigationTitle("Criteria")
        .navigationSubtitle("What the hub searches for, and screens every job against")
    }
}
