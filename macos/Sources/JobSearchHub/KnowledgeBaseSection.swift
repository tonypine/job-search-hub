import JobSearchHubCore
import SwiftUI

/// The knowledge base as the Profile page edits it: read, add, replace,
/// confirm and delete entries.
@MainActor
@Observable
final class KnowledgeBaseModel {
    private(set) var entries: [ProfileEntry] = []
    private(set) var errorMessage: String?
    private(set) var isWorking = false

    var groups: ProfileEntryGroups { ProfileEntryGroups(entries: entries) }
    var roles: [ProfileEntry] { groups.roles.map(\.role) }

    func load(with client: HubClient) async {
        await perform("load the knowledge base") {
            self.entries = try await client.get("v1/profile/entries", as: ProfileEntriesResponse.self).entries
        }
    }

    /// Saves an entry, confirming it when the owner wrote it themselves.
    func save(_ input: ProfileEntryInput, id: UUID?, confirming: Bool, with client: HubClient) async -> Bool {
        await perform("save the entry") {
            let saved = if let id {
                try await client.send("PUT", "v1/profile/entries/\(id.uuidString.lowercased())", body: input, as: ProfileEntry.self)
            } else {
                try await client.send("POST", "v1/profile/entries", body: input, as: ProfileEntry.self)
            }
            if confirming {
                _ = try await client.send("POST", "v1/profile/entries/confirm", body: ConfirmProfileEntriesRequest(ids: [saved.id]), as: ProfileEntriesResponse.self)
            }
            self.entries = try await client.get("v1/profile/entries", as: ProfileEntriesResponse.self).entries
        }
    }

    func confirm(_ ids: [UUID], with client: HubClient) async {
        guard !ids.isEmpty else { return }
        _ = await perform("confirm") {
            _ = try await client.send("POST", "v1/profile/entries/confirm", body: ConfirmProfileEntriesRequest(ids: ids), as: ProfileEntriesResponse.self)
            self.entries = try await client.get("v1/profile/entries", as: ProfileEntriesResponse.self).entries
        }
    }

    func delete(_ id: UUID, with client: HubClient) async {
        _ = await perform("delete the entry") {
            try await client.delete("v1/profile/entries/\(id.uuidString.lowercased())")
            self.entries = try await client.get("v1/profile/entries", as: ProfileEntriesResponse.self).entries
        }
    }

    @discardableResult
    private func perform(_ action: String, _ work: () async throws -> Void) async -> Bool {
        isWorking = true
        defer { isWorking = false }
        do {
            try await work()
            errorMessage = nil
            return true
        } catch {
            errorMessage = "Could not \(action): \(error)"
            return false
        }
    }
}

/// The knowledge base on the Profile page: roles with their cases, then the
/// other entries by kind, each confirmable, editable and deletable, plus the
/// button that builds it from the CV and LinkedIn.
struct KnowledgeBaseSection: View {
    let client: HubClient
    @Environment(ProfileSeed.self) private var seed
    @State private var model = KnowledgeBaseModel()
    @State private var editing: EditedEntry?
    @State private var deleting: ProfileEntry?

    struct EditedEntry: Identifiable {
        let id = UUID()
        var entryID: UUID?
        var input: ProfileEntryInput
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            seedStatus
            if let errorMessage = model.errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
            Text(describeCounts()).foregroundStyle(.secondary)
            let groups = model.groups
            ForEach(groups.roles) { group in roleGroup(group) }
            ForEach(groups.others) { kindGroup in
                DisclosureGroup {
                    ForEach(kindGroup.entries) { entry in entryRow(entry) }
                } label: {
                    groupLabel(title: kindGroup.title, subtitle: "", unconfirmedIDs: kindGroup.entries.filter { !$0.isConfirmed }.map(\.id))
                }
            }
        }
        .task(id: seed.revision) { await model.load(with: client) }
        .sheet(item: $editing) { edited in
            ProfileEntrySheet(entryID: edited.entryID, input: edited.input, roles: model.roles) { input in
                await model.save(input, id: edited.entryID, confirming: edited.entryID == nil, with: client)
            }
        }
        .confirmationDialog(
            "Delete “\(deleting?.title ?? "")”?", isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } }), presenting: deleting
        ) { entry in
            Button("Delete", role: .destructive) { Task { await model.delete(entry.id, with: client) } }
        } message: { entry in
            Text(entry.kind == "role" ? "Its cases stay, no longer tied to a role." : "This can't be undone.")
        }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Knowledge base").font(.title3.weight(.semibold))
                Text("Your roles, cases of work and skills, which briefs and CVs draw on. Only confirmed entries speak for you.")
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Button("Add entry", systemImage: "plus") { editing = EditedEntry(input: ProfileEntryInput(kind: "case", title: "")) }
            Button(model.entries.isEmpty ? "Build from CV and LinkedIn" : "Rebuild from CV and LinkedIn", systemImage: "square.stack.3d.up") {
                seed.start(with: client)
            }
            .disabled(seed.state == .building)
        }
    }

    @ViewBuilder
    private var seedStatus: some View {
        switch seed.state {
        case .building:
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("Reading your CV and LinkedIn… this takes a few minutes; you can leave this page.")
            }
        case let .built(added, updated):
            Label(
                added == 0 && updated == 0 ? "Nothing new in your CV and LinkedIn." : "Added \(added) entries, updated \(updated). Anything to settle is in Updates.",
                systemImage: "checkmark.circle.fill"
            )
            .foregroundStyle(.green)
        case let .failed(reason):
            Label(reason, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
        case .idle:
            EmptyView()
        }
    }

    private func roleGroup(_ group: ProfileRoleGroup) -> some View {
        DisclosureGroup {
            entryRow(group.role)
            ForEach(group.entries) { entry in entryRow(entry) }
        } label: {
            groupLabel(
                title: group.role.title,
                subtitle: [group.role.organization, group.role.monthsText].filter { !$0.isEmpty }.joined(separator: " · "),
                unconfirmedIDs: group.unconfirmedIDs
            )
        }
    }

    private func groupLabel(title: String, subtitle: String, unconfirmedIDs: [UUID]) -> some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 1) {
                Text(title).fontWeight(.semibold)
                if !subtitle.isEmpty {
                    Text(subtitle).font(.callout).foregroundStyle(.secondary)
                }
            }
            Spacer()
            if !unconfirmedIDs.isEmpty {
                Text("\(unconfirmedIDs.count) to confirm").font(.callout).foregroundStyle(.orange)
                Button("Confirm all") { Task { await model.confirm(unconfirmedIDs, with: client) } }
                    .disabled(model.isWorking)
            }
        }
    }

    private func entryRow(_ entry: ProfileEntry) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: entry.isConfirmed ? "checkmark.seal.fill" : "circle.dashed")
                .foregroundStyle(entry.isConfirmed ? .green : .orange)
                .help(entry.isConfirmed ? "Confirmed" : "Not confirmed yet")
            VStack(alignment: .leading, spacing: 3) {
                Text(entry.kind == "role" ? "The role itself" : entry.title).fontWeight(.medium)
                if !entry.body.isEmpty {
                    Text(entry.body).foregroundStyle(.secondary).lineLimit(4)
                }
                if !entry.outcome.isEmpty {
                    Text("Outcome: \(entry.outcome)")
                }
                if !entry.skills.isEmpty {
                    Text(entry.skills.joined(separator: ", ")).font(.caption).foregroundStyle(.secondary)
                }
                Text(describeSource(entry)).font(.caption).foregroundStyle(.tertiary)
            }
            Spacer()
            if !entry.isConfirmed {
                Button("Confirm") { Task { await model.confirm([entry.id], with: client) } }
                    .disabled(model.isWorking)
            }
            Menu {
                Button("Edit…") { editing = EditedEntry(entryID: entry.id, input: ProfileEntryInput(entry)) }
                Button("Delete…", role: .destructive) { deleting = entry }
            } label: {
                Image(systemName: "ellipsis.circle")
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
        .padding(.vertical, 4)
    }

    private func describeSource(_ entry: ProfileEntry) -> String {
        let months = entry.kind == "role" ? "" : entry.monthsText
        return (["From \(entry.source)", entry.sourceDetail, months]).filter { !$0.isEmpty }.joined(separator: " · ")
    }

    private func describeCounts() -> String {
        if model.entries.isEmpty { return "No entries yet." }
        let unconfirmedCount = model.entries.count { !$0.isConfirmed }
        let roleCount = model.entries.count { $0.kind == "role" }
        let caseCount = model.entries.count { $0.kind == "case" }
        return "\(model.entries.count) entries: \(roleCount) roles, \(caseCount) cases. \(unconfirmedCount) waiting for you to confirm."
    }
}

/// Adds or edits one entry. Fields follow the kind: a role has its
/// organization and months; a case, skill or project may belong to a role.
struct ProfileEntrySheet: View {
    let entryID: UUID?
    @State var input: ProfileEntryInput
    let roles: [ProfileEntry]
    let onSave: (ProfileEntryInput) async -> Bool
    @Environment(\.dismiss) private var dismiss
    @State private var skillsText = ""
    @State private var isSaving = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(entryID == nil ? "Add to the knowledge base" : "Edit entry").font(.title3.weight(.semibold))
            Form {
                Picker("Kind", selection: $input.kind) {
                    ForEach(ProfileEntryGroups.kinds, id: \.self) { kind in Text(Self.getKindName(kind)).tag(kind) }
                }
                TextField("Title", text: $input.title, prompt: Text(input.kind == "role" ? "e.g. Senior Software Engineer" : "What it is, in a few words"))
                if input.kind == "role" {
                    TextField("Organization", text: $input.organization)
                }
                if ["case", "skill", "project"].contains(input.kind) {
                    Picker("Role", selection: $input.roleID) {
                        Text("None").tag(UUID?.none)
                        ForEach(roles) { role in Text([role.title, role.organization].filter { !$0.isEmpty }.joined(separator: " at ")).tag(UUID?.some(role.id)) }
                    }
                }
                if ["role", "case", "project", "education"].contains(input.kind) {
                    HStack {
                        TextField("From", text: $input.startMonth, prompt: Text("YYYY-MM"))
                        TextField("To", text: $input.endMonth, prompt: Text(input.kind == "role" ? "YYYY-MM, empty if current" : "YYYY-MM"))
                    }
                }
                TextField("Details", text: $input.body, prompt: Text("What was done and how"), axis: .vertical).lineLimit(3...8)
                if input.kind == "case" || input.kind == "project" {
                    TextField("Outcome", text: $input.outcome, prompt: Text("What changed because of it, only if you know"), axis: .vertical).lineLimit(1...4)
                }
                TextField("Skills", text: $skillsText, prompt: Text("Comma-separated, e.g. TypeScript, React"))
            }
            .formStyle(.grouped)
            if entryID != nil {
                Text("From \(input.source)\(input.sourceDetail.isEmpty ? "" : " · \(input.sourceDetail)")").font(.caption).foregroundStyle(.secondary)
            } else {
                Text("What you add yourself counts as confirmed.").font(.caption).foregroundStyle(.secondary)
            }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                Button(entryID == nil ? "Add" : "Save") { Task { await save() } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(isSaving || input.title.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
        .padding(20)
        .frame(width: 560)
        .onAppear { skillsText = input.skills.joined(separator: ", ") }
    }

    private func save() async {
        isSaving = true
        defer { isSaving = false }
        input.skills = skillsText.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        if !["case", "skill", "project"].contains(input.kind) {
            input.roleID = nil
        }
        if await onSave(input) {
            dismiss()
        }
    }

    static func getKindName(_ kind: String) -> String {
        switch kind {
        case "role": "Role"
        case "case": "Case of work"
        case "skill": "Skill"
        case "project": "Project"
        case "education": "Education"
        case "preference": "Preference"
        case "fact": "Fact"
        default: kind.capitalized
        }
    }
}
