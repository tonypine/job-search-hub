import JobSearchHubCore
import SwiftUI

/// Dismisses and restores jobs for any page or panel. It lives as long as the
/// app, so the Jobs list reads again after a job is dismissed from its
/// details.
@MainActor
@Observable
final class JobDismissals {
    /// Bumps after each dismissal or restore, so the pages showing jobs read again.
    private(set) var revision = 0

    func dismiss(_ jobIDs: Set<UUID>, reason: String, with client: HubClient) async throws -> [Job] {
        let jobs = try await client.dismissJobs(Array(jobIDs), reason: reason)
        revision += 1
        return jobs
    }

    func restore(_ jobIDs: Set<UUID>, with client: HubClient) async throws -> [Job] {
        let jobs = try await client.restoreJobs(Array(jobIDs))
        revision += 1
        return jobs
    }
}

/// The jobs a dismissal sheet is open for.
struct JobDismissalTarget: Identifiable {
    let jobIDs: Set<UUID>
    var id: Set<UUID> { jobIDs }
}

/// Asks why the jobs are dismissed, which is optional, then dismisses them.
struct DismissJobsSheet: View {
    let jobCount: Int
    let onDismiss: (String) async -> String?
    @Environment(\.dismiss) private var closeSheet
    @State private var reason = ""
    @State private var isDismissing = false
    @State private var errorMessage: String?

    var body: some View {
        Form {
            Text(jobCount == 1 ? "Dismiss this job?" : "Dismiss \(jobCount) jobs?").font(.headline)
            Text("Dismissed jobs leave the list and stay dismissed when their board lists them again. Restore them from the Dismissed status.")
                .font(.callout).foregroundStyle(.secondary)
            TextField("Reason", text: $reason, prompt: Text("Optional, e.g. agency, US only"))
                .accessibilityLabel("Dismissal reason")
            if let errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            }
            HStack {
                Spacer()
                if isDismissing {
                    ProgressView().controlSize(.small)
                }
                Button("Cancel") { closeSheet() }
                Button("Dismiss") {
                    Task {
                        isDismissing = true
                        errorMessage = await onDismiss(reason)
                        isDismissing = false
                        if errorMessage == nil { closeSheet() }
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(isDismissing)
                .accessibilityLabel("Confirm dismissal")
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .padding()
    }
}
