import JobSearchHubCore
import SwiftUI

/// The owner's connections at a company, closest first, each with their
/// position, how close the owner is to them, and a link to their profile.
struct ConnectionList: View {
    let connections: [Connection]

    var body: some View {
        ForEach(connections) { connection in
            VStack(alignment: .leading, spacing: 2) {
                Text(connection.fullName).bold()
                if let position = connection.position, !position.isEmpty {
                    Text(position).foregroundStyle(.secondary)
                }
                HStack(spacing: 12) {
                    if let closeness = connection.closeness, !closeness.isEmpty {
                        Text(closeness).foregroundStyle(.secondary)
                    } else if let connectedSince = connection.connectedSince {
                        Text(connectedSince).foregroundStyle(.secondary)
                    }
                    if let profile = URL(string: connection.profileURL) {
                        Link("Profile", destination: profile)
                    }
                    if let email = connection.email, let mail = URL(string: "mailto:\(email)") {
                        Link(email, destination: mail)
                    }
                }
                .font(.caption)
            }
        }
    }
}
