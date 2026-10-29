import JobSearchHubCore
import SwiftUI

/// Records the owner's changes to applications for any page or panel: a
/// follow-up, a move to another phase, a skip and its restore. The hub
/// raises no update for them, so this bumps `revision` instead, and the
/// sidebar's Pipeline badge reads its counts again. It lives as long as the
/// app.
@MainActor
@Observable
final class ApplicationChanges {
    /// Bumps after each change, so what counts the cards reads again.
    private(set) var revision = 0

    func recordFollowUp(_ applicationID: UUID, note: String, with client: HubClient) async throws -> Application {
        let response = try await client.send(
            "POST", "v1/applications/\(applicationID.uuidString)/follow-ups", body: FollowUpRequest(note: note), as: ApplicationResponse.self
        )
        revision += 1
        return response.application
    }

    func move(_ applicationID: UUID, to phaseID: UUID, closedReason: String?, with client: HubClient) async throws -> Application {
        let response = try await client.send(
            "PATCH", "v1/applications/\(applicationID.uuidString)",
            body: MoveApplicationRequest(phaseID: phaseID, closedReason: closedReason), as: ApplicationResponse.self
        )
        revision += 1
        return response.application
    }

    func dismiss(_ applicationID: UUID, note: String, with client: HubClient) async throws -> Application {
        let application = try await client.dismissApplication(applicationID, note: note)
        revision += 1
        return application
    }

    func restore(_ applicationID: UUID, with client: HubClient) async throws -> Application {
        let application = try await client.restoreApplication(applicationID)
        revision += 1
        return application
    }
}
