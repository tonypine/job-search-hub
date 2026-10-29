import JobSearchHubCore
import SwiftUI

/// The models the hub can run on: its providers, each provider's models,
/// and how long the latest runs on each model took, read when a model menu
/// opens. Activity's column reads the providers with the routes, into the
/// same list, which names the kinds' models.
@MainActor
@Observable
final class ModelChoicesModel {
    private(set) var providers: [ModelProvider] = []
    private(set) var catalogs: [UUID: [ProviderModel]] = [:]
    private(set) var averages = ModelRunAverages()
    /// The providers whose models the last read couldn't list, by name.
    private(set) var unlistedProviders: [String] = []
    private(set) var hasLoaded = false
    /// Why the last read of the providers failed.
    private(set) var failure: HubFailure?

    func getGroups(claudeOnly: Bool = false, keeping current: RoutedModel? = nil) -> [ModelChoiceGroup] {
        ModelChoices.group(providers: providers, catalogs: catalogs, averages: averages, claudeOnly: claudeOnly, keeping: current)
    }

    /// Reads the providers, then their models, and the time a run on each
    /// unless the menu leaves times out, as a run's model select does.
    func load(with client: HubClient, readsAverages: Bool = true) async {
        guard await loadProviders(with: client) else {
            hasLoaded = true
            return
        }
        await loadModels(with: client, readsAverages: readsAverages)
    }

    /// Reads the providers alone, which name a model and give its mark,
    /// and lists none of their models: the hub asks openrouter.ai for
    /// OpenRouter's each time. Whether it could.
    @discardableResult
    func loadProviders(with client: HubClient) async -> Bool {
        do {
            setProviders(try await client.get("v1/model-providers", as: ModelProvidersResponse.self).providers)
            failure = nil
            return true
        } catch {
            failure = HubFailure("Couldn't read the model providers", error)
            return false
        }
    }

    /// The providers as another read found them. A provider gone takes its
    /// models with it.
    func setProviders(_ providers: [ModelProvider]) {
        self.providers = providers
        let ids = Set(providers.map(\.id))
        catalogs = catalogs.filter { ids.contains($0.key) }
    }

    /// Reads every provider's models and, unless told not to, the latest
    /// runs that worked at once. A provider whose models can't be listed
    /// keeps the ones read before.
    func loadModels(with client: HubClient, readsAverages: Bool = true) async {
        let read = providers
        let listed = await withTaskGroup(of: (UUID, [ProviderModel]?).self) { group in
            for provider in read {
                group.addTask {
                    let models = try? await client.get("v1/model-providers/\(provider.id.uuidString)/models", as: ProviderModelsResponse.self)
                    return (provider.id, models?.entries)
                }
            }
            var listed: [UUID: [ProviderModel]?] = [:]
            for await (id, entries) in group {
                listed[id] = entries
            }
            return listed
        }
        // The providers now, which a read since may have changed.
        var catalogs: [UUID: [ProviderModel]] = [:]
        for provider in providers {
            catalogs[provider.id] = listed[provider.id].flatMap { $0 } ?? self.catalogs[provider.id]
        }
        self.catalogs = catalogs
        unlistedProviders = read.filter { listed[$0.id].flatMap { $0 } == nil }.map(\.name)
        let latest = [URLQueryItem(name: "outcome", value: "succeeded"), URLQueryItem(name: "limit", value: "200")]
        if readsAverages, let page = try? await client.get("v1/runs", query: latest, as: ActivityRunsPage.self) {
            averages = ModelRunAverages(page.runs)
        }
        hasLoaded = true
    }
}

/// The model menu: the models by where they run, the chosen one ticked,
/// OpenRouter's with a search and their prices, this Mac's with their time
/// a run, and what the caller puts below them. A kind's page uses it for
/// the kind's model; a job's page, for one run's.
struct ModelPicker<Footer: View>: View {
    static var width: CGFloat { 360 }
    /// The list's height at most; OpenRouter's models scroll in it.
    static var listHeight: CGFloat { 420 }

    let groups: [ModelChoiceGroup]
    let selection: RoutedModel?
    let isLoading: Bool
    let unlistedProviders: [String]
    let choose: (ModelChoice) -> Void
    @ViewBuilder let footer: () -> Footer
    @State private var query = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: Space.xs) {
                    ForEach(groups) { group in
                        section(group)
                    }
                    if groups.isEmpty {
                        Text(isLoading ? "Reading the models…" : "No models yet. Set up a provider in Settings.")
                            .font(.hubSecondary)
                            .foregroundStyle(.secondary)
                            .padding(Space.s)
                    }
                }
                .padding(Space.s)
            }
            .frame(maxHeight: Self.listHeight)
            .fixedSize(horizontal: false, vertical: true)
            if !unlistedProviders.isEmpty {
                Label("Couldn't list the models of \(unlistedProviders.formatted(.list(type: .and)))", systemImage: "exclamationmark.triangle.fill")
                    .font(.hubCaption)
                    .foregroundStyle(Tone.caution.color)
                    .padding(.horizontal, Space.m)
                    .padding(.bottom, Space.s)
            }
            Divider()
            VStack(alignment: .leading, spacing: Space.s) {
                footer()
            }
            .padding(Space.m)
        }
        .frame(width: Self.width)
    }

    @ViewBuilder
    private func section(_ group: ModelChoiceGroup) -> some View {
        Text(group.title)
            .font(.hubCaption.weight(.semibold))
            .foregroundStyle(.secondary)
            .padding(.horizontal, Space.s)
            .padding(.top, Space.s)
            .accessibilityAddTraits(.isHeader)
        if group.isSearchable {
            SearchField(text: $query, prompt: "Search \(group.title)")
                .padding(.horizontal, Space.s)
                .padding(.bottom, Space.xs)
            let found = ModelChoices.search(group.choices, for: query)
            if found.isEmpty {
                Text("No model matches “\(query)”")
                    .font(.hubSecondary)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, Space.s)
            }
            ForEach(found) { choice in
                row(choice)
            }
        } else {
            ForEach(group.choices) { choice in
                row(choice)
            }
        }
    }

    private func row(_ choice: ModelChoice) -> some View {
        ModelChoiceRow(choice: choice, isSelected: choice.routed == selection) {
            choose(choice)
        }
    }
}

/// A model in the menu: a tick when it's the one chosen, its name, and its
/// price or time a run under it.
private struct ModelChoiceRow: View {
    let choice: ModelChoice
    let isSelected: Bool
    let choose: () -> Void
    @State private var isHovering = false

    var body: some View {
        Button(action: choose) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Image(systemName: "checkmark")
                    .font(.hubCaption.weight(.semibold))
                    .opacity(isSelected ? 1 : 0)
                    .frame(width: 12)
                VStack(alignment: .leading, spacing: 1) {
                    Text(choice.title).lineLimit(1)
                    if let detail = choice.detail {
                        Text(detail).font(.hubCaption).foregroundStyle(.secondary).lineLimit(1)
                    }
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, Space.s)
            .padding(.vertical, Space.xs)
            .contentShape(Rectangle())
            .background(isHovering ? AnyShapeStyle(.quaternary) : AnyShapeStyle(Color.clear), in: RoundedRectangle(cornerRadius: Radius.control))
        }
        .buttonStyle(.plain)
        .onHover { isHovering = $0 }
        .help(choice.title == choice.routed.model ? "From \(choice.providerName)" : "\(choice.routed.model) from \(choice.providerName)")
        .accessibilityAddTraits(isSelected ? .isSelected : [])
    }
}

/// The same models as menu items, for a right-click menu or a pop-up:
/// each group a section, with a tick on the one chosen. OpenRouter's,
/// too many for a section, are a submenu.
struct ModelChoiceMenuItems: View {
    let groups: [ModelChoiceGroup]
    let selection: RoutedModel?
    let choose: (ModelChoice) -> Void

    var body: some View {
        ForEach(groups) { group in
            if group.isSearchable {
                Menu(group.title) {
                    items(group.choices)
                }
            } else {
                Section(group.title) {
                    items(group.choices)
                }
            }
        }
    }

    private func items(_ choices: [ModelChoice]) -> some View {
        ForEach(choices) { choice in
            // Choosing the ticked model again changes nothing.
            Toggle(isOn: Binding(get: { choice.routed == selection }, set: { if $0 { choose(choice) } })) {
                Text(choice.title)
                if let detail = choice.detail {
                    Text(detail)
                }
            }
        }
    }
}

/// The fallback's menu items: None, then the models.
struct FallbackMenuItems: View {
    let groups: [ModelChoiceGroup]
    let selection: RoutedModel?
    let choose: (RoutedModel?) -> Void

    var body: some View {
        Toggle("None", isOn: Binding(get: { selection == nil }, set: { if $0 { choose(nil) } }))
        Divider()
        ModelChoiceMenuItems(groups: groups, selection: selection) { choose($0.routed) }
    }
}

/// Opens Settings on Models, where the providers are set up.
struct ProviderSettingsButton: View {
    @Environment(\.openSettings) private var openSettings
    @AppStorage(SettingsTab.storageKey) private var settingsTab = SettingsTab.connection

    var body: some View {
        Button("Set up providers in Settings…", systemImage: "gearshape") {
            settingsTab = .models
            openSettings()
        }
    }
}
