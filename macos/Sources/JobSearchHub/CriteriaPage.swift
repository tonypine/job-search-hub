import JobSearchHubCore
import SwiftUI

/// What the hub searches for and screens every job against, and the phases
/// applications move through: the settings that change what every page shows.
struct CriteriaPage: View {
    @Environment(HubConnection.self) private var connection

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                Form {
                    JobCriteriaSection(client: client)
                    PipelinePhasesSection(client: client)
                }
                .formStyle(.grouped)
            }
        }
        .navigationTitle("Criteria")
        .navigationSubtitle("What the hub searches for, and screens every job against")
    }
}
