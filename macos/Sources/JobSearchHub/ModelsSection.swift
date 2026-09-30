import JobSearchHubCore
import SwiftUI

/// Settings › Models: the model servers the hub can call, and the model each
/// kind of background task runs on, with a fallback.
struct ModelsSection: View {
    let client: HubClient
    @State private var providers: [ModelProvider] = []
    @State private var routes: [String: TaskRoute] = [:]
    @State private var errorMessage: String?
    @State private var editedProvider: ProviderSheetTarget?

    var body: some View {
        Section("Models") {
            ForEach(providers) { provider in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(provider.name)
                        Text(describe(provider)).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Edit") { editedProvider = .edit(provider) }
                        .accessibilityLabel("Edit \(provider.name)")
                }
            }
            Button("Add a provider", systemImage: "plus") { editedProvider = .add }
            ForEach(RoutedTaskKind.allCases) { kind in
                TaskRouteRow(kind: kind, providers: providers, route: routes[kind.rawValue], client: client) { saved in
                    routes[saved.kind] = saved
                }
            }
            if let errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
        }
        .task { await load() }
        .sheet(item: $editedProvider, onDismiss: { Task { await load() } }) { target in
            ProviderSheet(target: target, client: client)
        }
    }

    private func describe(_ provider: ModelProvider) -> String {
        var parts = [provider.isHubRuntime ? "Runs models on this Mac" : provider.baseURL]
        if provider.hasKey { parts.append("key set") }
        parts.append(provider.enforcesSchema ? "enforces schemas" : "answers are checked")
        return parts.joined(separator: " · ")
    }

    private func load() async {
        do {
            async let loadedProviders = client.get("v1/model-providers", as: ModelProvidersResponse.self)
            async let loadedRoutes = client.get("v1/task-routes", as: TaskRoutesResponse.self)
            let (providerList, routeList) = try await (loadedProviders, loadedRoutes)
            providers = providerList.providers
            routes = Dictionary(uniqueKeysWithValues: routeList.routes.map { ($0.kind, $0) })
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

enum ProviderSheetTarget: Identifiable {
    case add
    case edit(ModelProvider)

    var id: String {
        switch self {
        case .add: "add"
        case let .edit(provider): provider.id.uuidString
        }
    }
}

/// One kind of task's route: its provider and model, and a fallback for
/// when that provider can't be reached.
struct TaskRouteRow: View {
    let kind: RoutedTaskKind
    let providers: [ModelProvider]
    let route: TaskRoute?
    let client: HubClient
    let onSaved: (TaskRoute) -> Void

    @State private var providerID: UUID?
    @State private var model = ""
    @State private var fallbackProviderID: UUID?
    @State private var fallbackModel = ""
    @State private var modelsByProvider: [UUID: [String]] = [:]
    @State private var errorMessage: String?
    @State private var isSaving = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(kind.title).fontWeight(.medium)
                Spacer()
                Button("Save") { Task { await save() } }
                    .disabled(!hasChanges || isSaving || providerID == nil || model.isEmpty)
                    .accessibilityLabel("Save \(kind.title)")
            }
            HStack {
                providerPicker("Runs on", selection: $providerID, allowsNone: false)
                    .accessibilityLabel("\(kind.title) provider")
                modelPicker(for: providerID, selection: $model)
                    .accessibilityLabel("\(kind.title) model")
            }
            HStack {
                providerPicker("Fallback", selection: $fallbackProviderID, allowsNone: true)
                    .accessibilityLabel("\(kind.title) fallback provider")
                if fallbackProviderID != nil {
                    modelPicker(for: fallbackProviderID, selection: $fallbackModel)
                        .accessibilityLabel("\(kind.title) fallback model")
                }
            }
            if let errorMessage {
                Text(errorMessage).font(.caption).foregroundStyle(.orange)
            }
        }
        .padding(.vertical, 4)
        .task(id: route?.model) { resetToRoute() }
        .task(id: providerID) { await loadModels(for: providerID) }
        .task(id: fallbackProviderID) { await loadModels(for: fallbackProviderID) }
    }

    private var hasChanges: Bool {
        providerID != route?.providerID || model != (route?.model ?? "")
            || fallbackProviderID != route?.fallbackProviderID || fallbackModel != (route?.fallbackModel ?? "")
    }

    private func providerPicker(_ title: String, selection: Binding<UUID?>, allowsNone: Bool) -> some View {
        Picker(title, selection: selection) {
            if allowsNone {
                Text("None").tag(UUID?.none)
            }
            ForEach(providers) { provider in
                Text(provider.name).tag(UUID?.some(provider.id))
            }
        }
    }

    private func modelPicker(for providerID: UUID?, selection: Binding<String>) -> some View {
        let models = providerID.flatMap { modelsByProvider[$0] } ?? []
        let choices = models.contains(selection.wrappedValue) || selection.wrappedValue.isEmpty ? models : [selection.wrappedValue] + models
        return Picker("Model", selection: selection) {
            if selection.wrappedValue.isEmpty {
                Text("Choose a model").tag("")
            }
            ForEach(choices, id: \.self) { name in
                Text(name).tag(name)
            }
        }
    }

    private func resetToRoute() {
        providerID = route?.providerID
        model = route?.model ?? ""
        fallbackProviderID = route?.fallbackProviderID
        fallbackModel = route?.fallbackModel ?? ""
    }

    private func loadModels(for providerID: UUID?) async {
        guard let providerID, modelsByProvider[providerID] == nil else { return }
        do {
            modelsByProvider[providerID] = try await client.get("v1/model-providers/\(providerID.uuidString)/models", as: ProviderModelsResponse.self).models
        } catch {
            modelsByProvider[providerID] = []
            errorMessage = "Couldn't list the models: \(error)"
        }
    }

    private func save() async {
        guard let providerID else { return }
        isSaving = true
        defer { isSaving = false }
        let input = TaskRouteInput(
            providerID: providerID, model: model,
            fallbackProviderID: fallbackProviderID, fallbackModel: fallbackProviderID == nil ? nil : fallbackModel
        )
        do {
            let saved = try await client.send("PUT", "v1/task-routes/\(kind.rawValue)", body: input, as: TaskRoute.self)
            errorMessage = nil
            onSaved(saved)
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

/// Adds or edits a provider. The key is typed here and never shown again.
struct ProviderSheet: View {
    let target: ProviderSheetTarget
    let client: HubClient
    @Environment(\.dismiss) private var dismiss

    @State private var kind = ModelProvider.openAICompatibleKind
    @State private var name = ""
    @State private var baseURL = ""
    @State private var key = ""
    @State private var enforcesSchema = true
    @State private var hasSavedKey = false
    @State private var errorMessage: String?

    var body: some View {
        Form {
            Picker("Kind", selection: $kind) {
                Text("OpenAI-compatible server").tag(ModelProvider.openAICompatibleKind)
                Text("The hub's own runtime").tag(ModelProvider.hubRuntimeKind)
            }
            .accessibilityLabel("Provider kind")
            TextField("Name", text: $name)
                .accessibilityLabel("Provider name")
            if kind == ModelProvider.openAICompatibleKind {
                TextField("Address", text: $baseURL, prompt: Text("https://openrouter.ai/api/v1"))
                    .accessibilityLabel("Provider address")
                SecureField("Key", text: $key, prompt: Text(hasSavedKey ? "Set; type a new one to replace it" : "Optional"))
                    .accessibilityLabel("Provider key")
            }
            Toggle("Enforces a JSON schema", isOn: $enforcesSchema)
            if let errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            }
            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { dismiss() }
                Button("Save") { Task { await save() } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty)
                    .accessibilityLabel("Save provider")
            }
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .padding()
        .onAppear {
            if case let .edit(provider) = target {
                kind = provider.kind
                name = provider.name
                baseURL = provider.baseURL
                enforcesSchema = provider.enforcesSchema
                hasSavedKey = provider.hasKey
            }
        }
    }

    private func save() async {
        let input = ModelProviderInput(kind: kind, name: name, baseURL: baseURL, apiKey: key.isEmpty ? nil : key, enforcesSchema: enforcesSchema)
        do {
            switch target {
            case .add:
                _ = try await client.send("POST", "v1/model-providers", body: input, as: ModelProvider.self)
            case let .edit(provider):
                _ = try await client.send("PUT", "v1/model-providers/\(provider.id.uuidString)", body: input, as: ModelProvider.self)
            }
            dismiss()
        } catch {
            errorMessage = String(describing: error)
        }
    }
}
