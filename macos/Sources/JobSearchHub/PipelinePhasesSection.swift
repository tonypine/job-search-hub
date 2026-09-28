import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PipelinePhasesModel {
    private(set) var phases: [PipelinePhase] = []
    private(set) var isSaving = false
    var errorMessage: String?

    func load(with client: HubClient) async {
        await perform {
            phases = try await client.get("v1/pipeline", as: PipelineResponse.self).phases
        }
    }

    /// Adds the phase after the last one; reports whether the server took it.
    func add(named name: String, with client: HubClient) async -> Bool {
        await perform {
            let phase = try await client.send("POST", "v1/pipeline/phases", body: PipelinePhaseNameRequest(name: name), as: PipelinePhase.self)
            phases.append(phase)
        }
    }

    /// Renames the phase and returns its name as the server holds it, which is
    /// the old name when the rename is refused.
    func rename(_ phase: PipelinePhase, to name: String, with client: HubClient) async -> String {
        var savedName = phase.name
        await perform {
            let renamed = try await client.send("PATCH", "v1/pipeline/phases/\(phase.id.uuidString)", body: PipelinePhaseNameRequest(name: name), as: PipelinePhase.self)
            if let index = phases.firstIndex(where: { $0.id == renamed.id }) {
                phases[index] = renamed
            }
            savedName = renamed.name
        }
        return savedName
    }

    func move(_ phase: PipelinePhase, by offset: Int, with client: HubClient) async {
        guard let phaseIDs = PipelinePhaseOrder.getIDs(of: phases, moving: phase.id, by: offset) else { return }
        await perform {
            phases = try await client.send("PUT", "v1/pipeline/phases/order", body: ReorderPipelinePhasesRequest(phaseIDs: phaseIDs), as: PipelinePhasesResponse.self).phases
        }
    }

    func delete(_ phase: PipelinePhase, with client: HubClient) async {
        await perform {
            try await client.delete("v1/pipeline/phases/\(phase.id.uuidString)")
            phases.removeAll { $0.id == phase.id }
        }
    }

    /// Runs one request at a time, showing the server's refusal when there is one.
    @discardableResult
    private func perform(_ request: () async throws -> Void) async -> Bool {
        isSaving = true
        defer { isSaving = false }
        do {
            try await request()
            errorMessage = nil
            return true
        } catch HubError.server(_, let message) {
            errorMessage = message
        } catch {
            errorMessage = String(describing: error)
        }
        return false
    }
}

struct PipelinePhasesSection: View {
    let client: HubClient
    @State private var model = PipelinePhasesModel()
    @State private var newPhaseName = ""

    var body: some View {
        Section {
            ForEach(Array(model.phases.enumerated()), id: \.element.id) { index, phase in
                PipelinePhaseRow(
                    phase: phase, isFirst: index == 0, isLast: index == model.phases.count - 1, isSaving: model.isSaving,
                    onRename: { name in await model.rename(phase, to: name, with: client) },
                    onMove: { offset in Task { await model.move(phase, by: offset, with: client) } },
                    onDelete: { Task { await model.delete(phase, with: client) } }
                )
            }
            HStack {
                TextField("New phase", text: $newPhaseName, prompt: Text("New phase"))
                    .labelsHidden()
                    .onSubmit(add)
                Button("Add", action: add)
                    .disabled(newPhaseName.trimmingCharacters(in: .whitespaces).isEmpty || model.isSaving)
            }
            if let errorMessage = model.errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            }
        } header: {
            HStack {
                Text("Pipeline phases")
                if model.isSaving {
                    ProgressView().controlSize(.small)
                }
            }
        } footer: {
            Text("The board shows one column per phase, in this order. A phase can be deleted once it holds no applications.")
                .foregroundStyle(.secondary)
        }
        .task { await model.load(with: client) }
    }

    private func add() {
        let name = newPhaseName.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { return }
        Task {
            if await model.add(named: name, with: client) {
                newPhaseName = ""
            }
        }
    }
}

struct PipelinePhaseRow: View {
    let phase: PipelinePhase
    let isFirst: Bool
    let isLast: Bool
    let isSaving: Bool
    let onRename: (String) async -> String
    let onMove: (Int) -> Void
    let onDelete: () -> Void
    @State private var draftName = ""

    var body: some View {
        HStack {
            TextField("Name", text: $draftName)
                .labelsHidden()
                .onSubmit {
                    let name = draftName.trimmingCharacters(in: .whitespaces)
                    guard name != phase.name else { return }
                    Task { draftName = await onRename(name) }
                }
            if phase.isClosed {
                Text("Closed").font(.caption).foregroundStyle(.secondary)
            }
            Button("Move up", systemImage: "chevron.up") { onMove(-1) }
                .disabled(isFirst || isSaving)
            Button("Move down", systemImage: "chevron.down") { onMove(1) }
                .disabled(isLast || isSaving)
            Button("Delete", systemImage: "trash", role: .destructive, action: onDelete)
                .disabled(isSaving)
        }
        .labelStyle(.iconOnly)
        .buttonStyle(.borderless)
        .onAppear { draftName = phase.name }
        .onChange(of: phase.name) { _, name in draftName = name }
    }
}
