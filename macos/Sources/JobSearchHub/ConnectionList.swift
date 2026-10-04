import JobSearchHubCore
import SwiftUI

/// The owner's connections at a company, closest first, each with their
/// position, how close the owner is to them, and a link to their profile.
struct ConnectionList: View {
    let connections: [Connection]

    var body: some View {
        ForEach(connections) { connection in
            PersonRow(connection.fullName, relation: .connection, role: connection.position, detail: describeCloseness(connection)) {
                if let profile = URL(string: connection.profileURL) {
                    Link("Profile", destination: profile)
                }
                if let email = connection.email, let mail = URL(string: "mailto:\(email)") {
                    Link(email, destination: mail)
                }
            }
        }
    }

    private func describeCloseness(_ connection: Connection) -> String? {
        if let closeness = connection.closeness, !closeness.isEmpty {
            return closeness
        }
        return connection.connectedSince
    }
}
