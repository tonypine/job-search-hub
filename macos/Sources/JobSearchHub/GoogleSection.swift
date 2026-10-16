import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class GoogleSectionModel {
    private(set) var status: GoogleStatus?
    private(set) var check: GoogleCheck?
    private(set) var isWorking = false
    var failure: HubFailure?

    func load(with client: HubClient) async {
        do {
            status = try await client.get("v1/google", as: GoogleStatus.self)
        } catch {
            failure = HubFailure("Couldn't read the Google connection", error)
        }
    }

    /// Opens Google's consent page in the browser, then watches for the hub
    /// to hold a new sign-in, for up to three minutes.
    func connect(with client: HubClient) async {
        isWorking = true
        defer { isWorking = false }
        do {
            let signIn = try await client.send("POST", "v1/google/sign-in", body: [String: String](), as: GoogleSignIn.self)
            guard let url = URL(string: signIn.url) else { return }
            NSWorkspace.shared.open(url)
            let before = status?.connection?.connectedAt
            for _ in 0..<90 {
                try await Task.sleep(for: .seconds(2))
                let latest = try await client.get("v1/google", as: GoogleStatus.self)
                if let connectedAt = latest.connection?.connectedAt, connectedAt != before {
                    status = latest
                    failure = nil
                    return
                }
            }
            failure = HubFailure("Google didn't connect", advice: "No sign-in arrived. Finish it in the browser, or connect again.")
        } catch {
            failure = HubFailure("Couldn't connect Google", error)
        }
    }

    func runCheck(with client: HubClient) async {
        isWorking = true
        defer { isWorking = false }
        do {
            check = try await client.get("v1/google/check", as: GoogleCheck.self)
            failure = nil
        } catch HubError.server(_, let message) {
            failure = HubFailure("The Google check failed", advice: message)
            await load(with: client)
        } catch {
            failure = HubFailure("Couldn't check Google", error)
        }
    }
}

/// Settings' Google connection as a status row: read-only Gmail and
/// Calendar access, and Pub/Sub access to hear new mail, that the hub asks
/// for again when Google expires it.
struct GoogleSection: View {
    let client: HubClient
    @State private var model = GoogleSectionModel()

    var body: some View {
        Section {
            StatusRow(
                "Google", symbol: "envelope.fill", state: model.status?.stateTitle ?? "Checking…", stateTone: model.status?.stateTone ?? .neutral,
                detail: detail,
                help: "Read-only access to Gmail and Calendar, for follow-ups and replies, and Pub/Sub access to hear new mail as it arrives. "
                    + "Google expires it every 7 days while the app is in testing, and the hub asks you to connect again."
            ) {
                action
            }
            if model.failure != nil {
                if model.status == nil {
                    HubErrorView($model.failure) { Task { await model.load(with: client) } }
                } else {
                    HubErrorView($model.failure)
                }
            }
        }
        .task { await model.load(with: client) }
    }

    private var detail: String? {
        guard let status = model.status else { return nil }
        guard status.configured else { return "The hub has no Google OAuth client file" }
        guard let connection = status.connection else { return "Reads Gmail and Calendar once connected" }
        if status.needsSignIn { return "Connect again to read Gmail and Calendar" }
        if let check = model.check {
            return "\(check.labelCount) labels and \(check.calendarCount) calendars as \(check.email)"
        }
        return "Reads Gmail and Calendar as \(connection.email)"
    }

    /// Check while connected, with Connect again in its menu; Connect while
    /// Google needs a sign-in.
    @ViewBuilder
    private var action: some View {
        if let status = model.status, status.configured {
            if model.isWorking {
                ProgressView().controlSize(.small)
            } else if status.needsSignIn {
                Button(status.connection == nil ? "Connect…" : "Connect again…") {
                    Task { await model.connect(with: client) }
                }
            } else {
                Menu("Check") {
                    Button("Connect again…") { Task { await model.connect(with: client) } }
                } primaryAction: {
                    Task { await model.runCheck(with: client) }
                }
                .menuStyle(.button)
                .fixedSize()
                .help("Check that the hub reads Gmail and Calendar")
            }
        }
    }
}
