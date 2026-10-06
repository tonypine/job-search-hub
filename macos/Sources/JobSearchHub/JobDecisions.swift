import JobSearchHubCore
import SwiftUI

/// Records the owner's decisions on jobs for any page or panel: pursue,
/// skip, later, and the skips and restores of whole lists, which the hub
/// calls dismissals. It lives as long
/// as the app, so the pages showing jobs read again after a decision made
/// elsewhere.
@MainActor
@Observable
final class JobDecisions {
    /// Bumps after each decision, skip or restore, so the pages showing
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

    /// Takes back the decisions on the jobs, as Undo does after Later.
    func clear(_ jobIDs: Set<UUID>, with client: HubClient) async throws {
        defer { revision += 1 }
        for id in jobIDs {
            try await client.clearJobDecision(id)
        }
    }
}

/// The jobs a Skip sheet is open for.
struct JobSkipTarget: Identifiable {
    let jobIDs: Set<UUID>
    var id: Set<UUID> { jobIDs }
}

/// The job whose details the Fix sheet is asked for.
struct JobFixTarget: Identifiable {
    let id: UUID
    let title: String
}

/// Asks why the jobs are skipped, which is optional, then skips them.
struct SkipJobsSheet: View {
    let jobCount: Int
    /// Skips the jobs, or says what failed.
    let onSkip: (String) async -> HubFailure?
    @Environment(\.dismiss) private var closeSheet
    @State private var reason = ""
    @State private var failure: HubFailure?

    var body: some View {
        Form {
            Text(jobCount == 1 ? "Skip this job?" : "Skip \(jobCount) jobs?").font(.hubSection)
            Text("Skipped jobs leave the list and stay skipped when their board lists them again. Restore them from the Skipped status.")
                .font(.hubSecondary).foregroundStyle(.secondary)
            TextField("Reason", text: $reason, prompt: Text("Optional, e.g. agency, US only"))
                .accessibilityLabel("Reason for skipping")
            if let failure {
                HubErrorView(failure)
            }
            HStack {
                Spacer()
                Button("Cancel") { closeSheet() }
                AsyncButton("Skip", busyTitle: "Skipping…") {
                    failure = await onSkip(reason)
                    if failure == nil { closeSheet() }
                }
                .keyboardShortcut(.defaultAction)
                .accessibilityLabel("Confirm skipping")
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .padding()
    }
}

/// Asks why jobs already skipped were skipped, from the toast that says so.
struct SkipReasonSheet: View {
    let jobCount: Int
    /// Saves the reason, or says what failed.
    let onSave: (String) async -> HubFailure?
    @Environment(\.dismiss) private var closeSheet
    @State private var reason = ""
    @State private var failure: HubFailure?

    var body: some View {
        Form {
            Text(jobCount == 1 ? "Why skip this job?" : "Why skip these \(jobCount) jobs?").font(.hubSection)
            Text("The reason shows on the row under Skipped.")
                .font(.hubSecondary).foregroundStyle(.secondary)
            TextField("Reason", text: $reason, prompt: Text("e.g. agency, hybrid only"))
                .accessibilityLabel("Reason for skipping")
            if let failure {
                HubErrorView(failure)
            }
            HStack {
                Spacer()
                Button("Cancel") { closeSheet() }
                AsyncButton("Save", busyTitle: "Saving…") {
                    failure = await onSave(reason)
                    if failure == nil { closeSheet() }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(reason.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
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
    /// Sends the note, or says what failed.
    let onFix: (String) async -> HubFailure?
    @Environment(\.dismiss) private var closeSheet
    @State private var note = ""
    @State private var failure: HubFailure?

    var body: some View {
        Form {
            Text("Fix \(jobTitle)").font(.hubSection)
            Text("Say what's wrong. An agent reads the job and its posting, corrects the details, and the outcome arrives as an update.")
                .font(.hubSecondary).foregroundStyle(.secondary)
            TextField("What's wrong", text: $note, prompt: Text("e.g. the company is Track&Field; the title has the city in it"), axis: .vertical)
                .lineLimit(2...5)
                .accessibilityLabel("What's wrong")
            if let failure {
                HubErrorView(failure)
            }
            HStack {
                Spacer()
                Button("Cancel") { closeSheet() }
                AsyncButton("Fix", busyTitle: "Sending…") {
                    failure = await onFix(note)
                    if failure == nil { closeSheet() }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .padding()
    }
}
