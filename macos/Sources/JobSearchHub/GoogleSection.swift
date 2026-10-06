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

/// Settings' Google connection: read-only Gmail and Calendar access, and Pub/Sub
/// access to hear new mail, that the hub asks for again when Google expires it.
struct GoogleSection: View {
    let client: HubClient
    @State private var model = GoogleSectionModel()

    var body: some View {
        Section {
            if let status = model.status {
                Label(status.summary, systemImage: status.needsSignIn ? "exclamationmark.triangle.fill" : "checkmark.circle.fill")
                    .foregroundStyle((status.needsSignIn ? Tone.caution : Tone.positive).color)
                if let check = model.check {
                    Text("Reads \(check.labelCount) Gmail labels and \(check.calendarCount) calendars as \(check.email).")
                        .foregroundStyle(.secondary)
                }
                if model.failure != nil {
                    HubErrorView($model.failure)
                }
                HStack {
                    Spacer()
                    if status.connection != nil && !status.needsSignIn {
                        AsyncButton("Check", busyTitle: "Checking…") { await model.runCheck(with: client) }
                    }
                    if status.configured {
                        AsyncButton(status.connection == nil ? "Connect Google" : "Connect again", busyTitle: "Waiting for Google…") {
                            await model.connect(with: client)
                        }
                    }
                }
                .disabled(model.isWorking)
            } else if model.failure != nil {
                HubErrorView($model.failure) { Task { await model.load(with: client) } }
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
