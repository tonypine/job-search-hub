import JobSearchHubCore
import SwiftUI

/// Records the owner's decisions on jobs for any page or panel: pursue,
/// skip, later, and the dismissals and restores a skip is. It lives as long
/// as the app, so the pages showing jobs read again after a decision made
/// elsewhere.
@MainActor
@Observable
final class JobDecisions {
    /// Bumps after each decision, dismissal or restore, so the pages showing
    /// jobs read again.
    private(set) var revision = 0

    func decide(_ jobID: UUID, _ decision: JobDecisionKind, reason: String = "", with client: HubClient) async throws -> JobDecision {
        let recorded = try await client.decideJob(jobID, decision, reason: reason)
        revision += 1
        return recorded
    }

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

/// The job whose details the Fix sheet is asked for.
struct JobFixTarget: Identifiable {
    let id: UUID
    let title: String
}

/// Asks why the jobs are dismissed, which is optional, then dismisses them.
struct DismissJobsSheet: View {
    let jobCount: Int
    /// The action as the sheet names it: "Dismiss", or "Skip" from a brief.
    var actionName = "Dismiss"
    let onDismiss: (String) async -> String?
    @Environment(\.dismiss) private var closeSheet
    @State private var reason = ""
    @State private var isDismissing = false
    @State private var errorMessage: String?

    var body: some View {
        Form {
            Text(jobCount == 1 ? "\(actionName) this job?" : "\(actionName) \(jobCount) jobs?").font(.headline)
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
                Button(actionName) {
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

/// Asks for a job's details to be fixed: a note on what's wrong, which an
/// agent on the Mac acts on.
struct FixJobSheet: View {
    let jobTitle: String
    let onFix: (String) async -> String?
    @Environment(\.dismiss) private var closeSheet
    @State private var note = ""
    @State private var isSending = false
    @State private var errorMessage: String?

    var body: some View {
        Form {
            Text("Fix \(jobTitle)").font(.headline)
            Text("Say what's wrong. An agent reads the job and its posting, corrects the details, and the outcome arrives as an update.")
                .font(.callout).foregroundStyle(.secondary)
            TextField("What's wrong", text: $note, prompt: Text("e.g. the company is Track&Field; the title has the city in it"), axis: .vertical)
                .lineLimit(2...5)
                .accessibilityLabel("What's wrong")
            if let errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            }
            HStack {
                Spacer()
                if isSending {
                    ProgressView().controlSize(.small)
                }
                Button("Cancel") { closeSheet() }
                Button("Fix") {
                    Task {
                        isSending = true
                        errorMessage = await onFix(note)
                        isSending = false
                        if errorMessage == nil { closeSheet() }
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(isSending || note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .padding()
    }
}
