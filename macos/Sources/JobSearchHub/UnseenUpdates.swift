import JobSearchHubCore
import SwiftUI

/// How many updates the owner hasn't seen, for the sidebar, and the one place
/// updates are marked seen. `revision` moves whenever something was marked, so
/// pages showing unseen marks read them again.
@MainActor
@Observable
final class UnseenUpdates {
    private(set) var count = 0
    private(set) var revision = 0

    func refresh(with client: HubClient) async {
        let query = [URLQueryItem(name: "unseen", value: "true"), URLQueryItem(name: "limit", value: "1")]
        if let list = try? await client.get("v1/updates", query: query, as: HubUpdateList.self) {
            count = list.unseenCount
        }
    }

    func markSeen(_ selection: UpdateSelection, with client: HubClient) async {
        guard let response = try? await client.send("POST", "v1/updates/seen", body: selection, as: MarkSeenResponse.self),
              response.marked > 0
        else { return }
        revision += 1
        await refresh(with: client)
    }
}

/// The dot on something with updates the owner hasn't seen. With none it
/// stays clear but keeps its space, so rows line up.
struct UnseenDot: View {
    let count: Int

    var body: some View {
        Circle()
            .fill(Color.accentColor)
            .frame(width: 7, height: 7)
            .opacity(count > 0 ? 1 : 0)
            .accessibilityHidden(count == 0)
            .accessibilityLabel("Unseen updates")
    }
}
