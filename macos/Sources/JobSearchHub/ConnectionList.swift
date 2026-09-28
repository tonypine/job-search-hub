import JobSearchHubCore
import SwiftUI

/// The owner's connections at a company, each with their position, since
/// when they are connected, and a link to their profile.
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
                    if let connectedSince = connection.connectedSince {
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
