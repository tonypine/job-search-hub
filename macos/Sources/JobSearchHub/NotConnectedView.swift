import SwiftUI

/// What a page shows without a client: a wait while the owner token is read
/// from the Keychain, which lasts as long as its access prompt stays up, and
/// otherwise a pointer to Settings.
struct NotConnectedView: View {
    @Environment(HubConnection.self) private var connection

    var body: some View {
        if connection.token == .reading {
            ContentUnavailableView(
                "Waiting for Keychain access", systemImage: "lock",
                description: Text("Allow Job Search Hub to read its owner token when the Keychain asks.")
            )
        } else {
            ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
        }
    }
}
