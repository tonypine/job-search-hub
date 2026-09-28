import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class GoogleSectionModel {
    private(set) var status: GoogleStatus?
    private(set) var check: GoogleCheck?
    private(set) var isWorking = false
    var errorMessage: String?

    func load(with client: HubClient) async {
        do {
            status = try await client.get("v1/google", as: GoogleStatus.self)
        } catch {
            errorMessage = String(describing: error)
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
                    errorMessage = nil
                    return
                }
            }
            errorMessage = "No sign-in arrived. Finish it in the browser, or connect again."
        } catch {
            errorMessage = String(describing: error)
        }
    }

    func runCheck(with client: HubClient) async {
        isWorking = true
        defer { isWorking = false }
        do {
            check = try await client.get("v1/google/check", as: GoogleCheck.self)
            errorMessage = nil
        } catch HubError.server(_, let message) {
            errorMessage = message
            await load(with: client)
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

/// Settings' Google connection: read-only Gmail and Calendar access, and Pub/Sub
/// access to hear new mail, that the hub asks for again when Google expires it.
struct GoogleSection: View {
    let client: HubClient
    @State private var model = GoogleSectionModel()

    var body: some View {
        Section {
            if let status = model.status {
                Label(status.summary, systemImage: status.needsSignIn ? "exclamationmark.triangle.fill" : "checkmark.circle.fill")
                    .foregroundStyle(status.needsSignIn ? AnyShapeStyle(.orange) : AnyShapeStyle(.green))
                if let check = model.check {
                    Text("Reads \(check.labelCount) Gmail labels and \(check.calendarCount) calendars as \(check.email).")
                        .foregroundStyle(.secondary)
                }
                HStack {
                    if let errorMessage = model.errorMessage {
                        Text(errorMessage).foregroundStyle(.red)
                    }
                    Spacer()
                    if model.isWorking {
                        ProgressView().controlSize(.small)
                    }
                    if status.connection != nil && !status.needsSignIn {
                        Button("Check") { Task { await model.runCheck(with: client) } }
                    }
                    if status.configured {
                        Button(status.connection == nil ? "Connect Google" : "Connect again") { Task { await model.connect(with: client) } }
                    }
                }
                .disabled(model.isWorking)
            } else {
                ProgressView().controlSize(.small)
            }
        } header: {
            Text("Google")
        } footer: {
            Text("Read-only access to Gmail and Calendar, for follow-ups and replies, and Pub/Sub access to hear new mail as it arrives. Google expires it every 7 days while the app is in testing, and the hub asks you to connect again.")
                .foregroundStyle(.secondary)
        }
        .task { await model.load(with: client) }
    }
}
