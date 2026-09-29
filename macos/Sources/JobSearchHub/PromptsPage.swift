import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PromptsModel {
    private(set) var summaries: [AgentPromptSummary] = []
    /// The selected prompt's versions, the active one first.
    private(set) var versions: [AgentPrompt] = []
    private(set) var selectedKind: String?
    /// The version whose text the editor started from.
    private(set) var shownVersion: Int?
    var draft = ""
    var note = ""
    private(set) var isSaving = false
    private(set) var errorMessage: String?

    var selectedSummary: AgentPromptSummary? { summaries.first { $0.kind == selectedKind } }
    var activeVersion: AgentPrompt? { versions.first }
    var shownPrompt: AgentPrompt? { versions.first { $0.version == shownVersion } }
    /// Whether the editor holds text that saving would make a new version.
    var hasChanges: Bool { !versions.isEmpty && draft != activeVersion?.body }

    func loadSummaries(with client: HubClient) async {
        do {
            summaries = try await client.get("v1/agent-prompts", as: AgentPromptsResponse.self).prompts
            errorMessage = nil
        } catch {
            errorMessage = "Could not load the prompts: \(error)"
        }
    }

    func select(_ kind: String?, with client: HubClient) async {
        selectedKind = kind
        versions = []
        draft = ""
        note = ""
        guard let kind else { return }
        do {
            versions = try await client.get("v1/agent-prompts/\(kind)/versions", as: AgentPromptVersionsResponse.self).versions
            show(versions.first?.version)
            errorMessage = nil
        } catch {
            errorMessage = "Could not load the prompt's versions: \(error)"
        }
    }

    /// Puts a version's text in the editor; saving it makes it active again.
    func show(_ version: Int?) {
        shownVersion = version
        draft = shownPrompt?.body ?? ""
    }

    func save(with client: HubClient) async {
        guard let kind = selectedKind else { return }
        isSaving = true
        defer { isSaving = false }
        do {
            let saved = try await client.send(
                "POST", "v1/agent-prompts/\(kind)", body: SaveAgentPromptRequest(body: draft, note: note), as: AgentPrompt.self
            )
            await loadSummaries(with: client)
            await select(kind, with: client)
            shownVersion = saved.version
        } catch {
            errorMessage = "Could not save the prompt: \(error)"
        }
    }
}

/// The agents' prompts: what each is for, its versions, and an editor that
/// saves a new version the next run uses.
struct PromptsPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var model = PromptsModel()
    /// A prompt picked while the editor held unsaved changes.
    @State private var pendingKind: String?

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client: client)
                    .task {
                        await model.loadSummaries(with: client)
                        if model.selectedKind == nil {
                            await model.select(model.summaries.first?.kind, with: client)
                        }
                    }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Prompts")
    }

    private func content(client: HubClient) -> some View {
        HStack(spacing: 0) {
            List(selection: Binding(get: { model.selectedKind }, set: { kind in pick(kind, with: client) })) {
                ForEach(model.summaries) { summary in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(summary.title)
                        Text(summary.version.map { "Version \($0)" } ?? "No version yet")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .tag(Optional(summary.kind))
                }
            }
            .frame(width: 230)
            Divider()
            editor(client: client)
        }
        .alert("Discard your changes?", isPresented: Binding(get: { pendingKind != nil }, set: { if !$0 { pendingKind = nil } })) {
            Button("Discard", role: .destructive) {
                let kind = pendingKind
                pendingKind = nil
                Task { await model.select(kind, with: client) }
            }
            Button("Keep editing", role: .cancel) { pendingKind = nil }
        } message: {
            Text("The edits to \(model.selectedSummary?.title ?? "this prompt") aren't saved.")
        }
    }

    @ViewBuilder
    private func editor(client: HubClient) -> some View {
        if let summary = model.selectedSummary {
            VStack(alignment: .leading, spacing: 12) {
                Text(summary.title).font(.title2.weight(.semibold))
                Text(summary.description).foregroundStyle(.secondary)
                if !summary.placeholders.isEmpty {
                    HStack(spacing: 6) {
                        Text("The hub fills").foregroundStyle(.secondary)
                        ForEach(summary.placeholders, id: \.self) { placeholder in
                            Text(placeholder).font(.callout.monospaced()).textSelection(.enabled)
                                .padding(.horizontal, 6).padding(.vertical, 2)
                                .background(.quinary, in: RoundedRectangle(cornerRadius: 4))
                        }
                    }
                    .font(.callout)
                }
                Picker("Version", selection: Binding(get: { model.shownVersion }, set: { model.show($0) })) {
                    ForEach(model.versions) { version in
                        Text(describe(version)).tag(Optional(version.version))
                    }
                }
                .frame(maxWidth: 560, alignment: .leading)
                TextEditor(text: $model.draft)
                    .font(.body.monospaced())
                    .scrollContentBackground(.hidden)
                    .padding(6)
                    .background(.quinary, in: RoundedRectangle(cornerRadius: 8))
                    .disabled(model.isSaving)
                if let errorMessage = model.errorMessage {
                    Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                }
                HStack {
                    TextField("What this version changes", text: $model.note)
                        .textFieldStyle(.roundedBorder)
                    if model.isSaving {
                        ProgressView().controlSize(.small)
                    }
                    Button("Discard changes") { model.show(model.activeVersion?.version) }
                        .disabled(!model.hasChanges || model.isSaving)
                    Button("Save as version \((model.activeVersion?.version ?? 0) + 1)") { Task { await model.save(with: client) } }
                        .keyboardShortcut("s")
                        .disabled(!model.hasChanges || model.isSaving)
                }
                if let shown = model.shownVersion, let active = model.activeVersion?.version, shown != active {
                    Text("Showing version \(shown). Saving it makes it the active prompt again, as version \(active + 1).")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            ContentUnavailableView("Pick a prompt", systemImage: "text.bubble")
        }
    }

    private func describe(_ version: AgentPrompt) -> String {
        var parts = ["Version \(version.version)"]
        if version.version == model.activeVersion?.version { parts.append("active") }
        if let createdAt = version.createdAt { parts.append(createdAt.formatted(date: .abbreviated, time: .shortened)) }
        if let note = version.note, !note.isEmpty {
            parts.append(note.count > 60 ? note.prefix(60) + "…" : note)
        }
        return parts.joined(separator: " · ")
    }

    /// Switches prompts, asking first when the editor holds unsaved changes.
    private func pick(_ kind: String?, with client: HubClient) {
        guard kind != model.selectedKind else { return }
        if model.hasChanges {
            pendingKind = kind
        } else {
            Task { await model.select(kind, with: client) }
        }
    }
}
