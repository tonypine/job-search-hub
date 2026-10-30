import Foundation

/// A model on a provider, as a route names it.
public struct RoutedModel: Hashable, Sendable {
    public var providerID: UUID
    public var model: String

    public init(providerID: UUID, model: String) {
        self.providerID = providerID
        self.model = model
    }
}

/// Where a provider's models run, which names their group in the model
/// menu: the owner's Claude subscription, this Mac, OpenRouter, or another
/// server, by its provider.
public enum ModelPlace: Hashable, Sendable {
    case claude
    case thisMac
    case openRouter
    case server(UUID)
}

/// A model the menu offers: its provider, the id its route names, its name,
/// and what tells it apart, OpenRouter's price or a local model's time.
public struct ModelChoice: Equatable, Identifiable, Sendable {
    public var routed: RoutedModel
    public var providerName: String
    public var title: String
    public var detail: String?

    public var id: RoutedModel { routed }
}

/// The models of one place, in the order the menu lists them.
public struct ModelChoiceGroup: Equatable, Identifiable, Sendable {
    public var place: ModelPlace
    public var title: String
    public var choices: [ModelChoice]

    public var id: ModelPlace { place }

    /// OpenRouter lists hundreds of models; its group has a search.
    public var isSearchable: Bool { place == .openRouter }
}

/// How long the latest model calls on each provider's models took, for the
/// menu's local models: the runs that worked, by provider name and model.
public struct ModelRunAverages: Equatable, Sendable {
    private var averages: [String: [String: Int]] = [:]

    public init() {}

    public init(_ runs: [ActivityRun]) {
        var totals: [String: [String: (sum: Int, count: Int)]] = [:]
        for run in runs where !run.isAgentRun && run.outcome == "succeeded" && !run.model.isEmpty {
            guard let duration = run.durationMS else { continue }
            let total = totals[run.provider, default: [:]][run.model, default: (0, 0)]
            totals[run.provider, default: [:]][run.model] = (total.sum + duration, total.count + 1)
        }
        averages = totals.mapValues { models in models.mapValues { $0.sum / $0.count } }
    }

    public func getAverageMS(provider: String, model: String) -> Int? {
        averages[provider]?[model]
    }
}

/// What names each kind's model in its row and its header, and what its
/// model menu ticks: the kinds' routes, with the providers and catalogs the
/// menus list. The row, the header and the menus read the one list of
/// providers, so a model chosen on a provider set up since the routes were
/// read is named as the menu ticks it.
public struct KindRoutes: Equatable, Sendable {
    public var routes: [TaskRoute]
    public var providers: [ModelProvider]
    public var catalogs: [UUID: [ProviderModel]]

    public init(routes: [TaskRoute] = [], providers: [ModelProvider] = [], catalogs: [UUID: [ProviderModel]] = [:]) {
        self.routes = routes
        self.providers = providers
        self.catalogs = catalogs
    }

    public func getRoute(for kind: RunKind) -> TaskRoute? {
        routes.first { $0.kind == kind.id }
    }

    /// The model the kind's menu ticks; none before it has a route.
    public func getSelection(for kind: RunKind) -> RoutedModel? {
        getRoute(for: kind).map { RoutedModel(providerID: $0.providerID, model: $0.model) }
    }

    /// The model the kind's row and header name.
    public func getModel(for kind: RunKind) -> RunKindModel? {
        RunKinds.getModel(for: kind, routes: routes, providers: providers, catalogs: catalogs)
    }

    /// The line under the header's menu that names the fallback; an agent
    /// has none.
    public func getFallbackLine(for kind: RunKind) -> String? {
        kind.isAgent ? nil : ModelChoices.describeFallback(of: getRoute(for: kind), providers: providers, catalogs: catalogs)
    }
}

public enum ModelChoices {
    /// The Claude subscription's models in the menu's order: the strongest
    /// first.
    private static let claudeOrder = ["opus", "sonnet", "haiku"]
    private static let loopbackHosts: Set<String> = ["localhost", "127.0.0.1", "::1", "[::1]", "0.0.0.0"]

    /// Where provider's models run: an OpenAI-compatible server is on this
    /// Mac when it answers at a loopback address, and is OpenRouter when it
    /// answers at OpenRouter's.
    public static func getPlace(of provider: ModelProvider) -> ModelPlace {
        switch provider.kind {
        case ModelProvider.claudeSubscriptionKind: return .claude
        case ModelProvider.openRouterKind: return .openRouter
        case ModelProvider.hubRuntimeKind: return .thisMac
        default:
            let host = URL(string: provider.baseURL)?.host()?.lowercased() ?? ""
            if loopbackHosts.contains(host) { return .thisMac }
            if host == "openrouter.ai" { return .openRouter }
            return .server(provider.id)
        }
    }

    /// The models the menu offers, by place: the Claude subscription, this
    /// Mac and OpenRouter, then any other server under its provider's name.
    /// An agent, which the hub runs only on Claude, is offered Claude alone.
    /// The model a route names stays in its group when its provider's
    /// catalog doesn't list it, so the menu can tick it. Places with no
    /// model are left out.
    public static func group(
        providers: [ModelProvider], catalogs: [UUID: [ProviderModel]], averages: ModelRunAverages = ModelRunAverages(),
        claudeOnly: Bool = false, keeping current: RoutedModel? = nil
    ) -> [ModelChoiceGroup] {
        var groups: [ModelChoiceGroup] = []
        for provider in providers {
            let place = getPlace(of: provider)
            if claudeOnly && place != .claude { continue }
            var entries = catalogs[provider.id] ?? []
            if let current, current.providerID == provider.id, !entries.contains(where: { $0.id == current.model }) {
                entries.insert(ProviderModel(id: current.model), at: 0)
            }
            let choices = entries.map { entry in
                ModelChoice(
                    routed: RoutedModel(providerID: provider.id, model: entry.id), providerName: provider.name,
                    title: getTitle(of: entry), detail: describe(entry, at: place, on: provider, averages: averages)
                )
            }
            if let index = groups.firstIndex(where: { $0.place == place }) {
                groups[index].choices += choices
            } else {
                groups.append(ModelChoiceGroup(place: place, title: getTitle(of: place, provider: provider), choices: choices))
            }
        }
        return groups.enumerated()
            .sorted { (getRank($0.element.place), $0.offset) < (getRank($1.element.place), $1.offset) }
            .map { _, group in
                var group = group
                group.choices = sort(group.choices, at: group.place)
                return group
            }
            .filter { !$0.choices.isEmpty }
    }

    /// The groups as a run's model select lists them: names alone, without
    /// OpenRouter's prices or this Mac's time a run.
    public static func makeCompact(_ groups: [ModelChoiceGroup]) -> [ModelChoiceGroup] {
        groups.map { group in
            var group = group
            group.choices = group.choices.map { choice in
                var choice = choice
                choice.detail = nil
                return choice
            }
            return group
        }
    }

    /// The models whose name or id holds every word of query, in any case
    /// and with or without accents: "flash" finds Gemini 2.5 Flash, and
    /// "gemini flash" too. An empty query keeps them all.
    public static func search(_ choices: [ModelChoice], for query: String) -> [ModelChoice] {
        let words = query.split(whereSeparator: \.isWhitespace).map(String.init)
        guard !words.isEmpty else { return choices }
        return choices.filter { choice in
            words.allSatisfy { word in
                [choice.title, choice.routed.model].contains { $0.range(of: word, options: [.caseInsensitive, .diacriticInsensitive]) != nil }
            }
        }
    }

    /// What OpenRouter charges for a model, per million tokens in and out:
    /// "$0.30 in · $2.50 out per M tokens", "Free"; nil when it gave no
    /// price.
    public static func describePrice(inputUSDPerMillion: String?, outputUSDPerMillion: String?) -> String? {
        let input = inputUSDPerMillion.flatMap(Double.init)
        let output = outputUSDPerMillion.flatMap(Double.init)
        if input == 0 && output == 0 { return "Free" }
        let parts = [input.map { "\(formatDollars($0)) in" }, output.map { "\(formatDollars($0)) out" }].compactMap { $0 }
        return parts.isEmpty ? nil : parts.joined(separator: " · ") + " per M tokens"
    }

    /// A local model's time a run: "about 4.2 s a run".
    public static func describeAverage(milliseconds: Int) -> String {
        "about \(RunFormat.describeDuration(milliseconds: milliseconds)) a run"
    }

    /// The name of a model a route names: its catalog's, or its id without
    /// a local file's ".gguf".
    public static func getModelName(_ routed: RoutedModel, catalogs: [UUID: [ProviderModel]]) -> String {
        let entry = catalogs[routed.providerID]?.first { $0.id == routed.model } ?? ProviderModel(id: routed.model)
        return getTitle(of: entry)
    }

    /// The line under a kind's model menu that names its fallback: "Falls
    /// back to Qwen3.8 27B on this Mac"; nil without one.
    public static func describeFallback(of route: TaskRoute?, providers: [ModelProvider], catalogs: [UUID: [ProviderModel]]) -> String? {
        guard let route, let providerID = route.fallbackProviderID, let model = route.fallbackModel, !model.isEmpty,
              let provider = providers.first(where: { $0.id == providerID })
        else { return nil }
        let name = getModelName(RoutedModel(providerID: providerID, model: model), catalogs: catalogs)
        let place = switch getPlace(of: provider) {
        case .claude: "on your Claude subscription"
        case .thisMac: "on this Mac"
        case .openRouter: "on OpenRouter"
        case .server: "on \(provider.name)"
        }
        return "Falls back to \(name) \(place)"
    }

    /// The route that runs kind on chosen from its next run: the fallback
    /// kept, unless it is the model chosen or kind is an agent, which has
    /// none.
    public static func makeRoute(choosing chosen: RoutedModel, for kind: RunKind, route: TaskRoute?) -> TaskRouteInput {
        var fallback = route.flatMap { route in route.fallbackProviderID.map { RoutedModel(providerID: $0, model: route.fallbackModel ?? "") } }
        if kind.isAgent || fallback == chosen || fallback?.model.isEmpty == true {
            fallback = nil
        }
        return TaskRouteInput(providerID: chosen.providerID, model: chosen.model, fallbackProviderID: fallback?.providerID, fallbackModel: fallback?.model)
    }

    /// The route with its fallback set to fallback, or with none.
    public static func makeRoute(fallingBackTo fallback: RoutedModel?, route: TaskRoute) -> TaskRouteInput {
        TaskRouteInput(providerID: route.providerID, model: route.model, fallbackProviderID: fallback?.providerID, fallbackModel: fallback?.model)
    }

    private static func getTitle(of entry: ProviderModel) -> String {
        if let name = entry.name?.trimmingCharacters(in: .whitespaces), !name.isEmpty { return name }
        return RunKinds.describeModel(entry.id)
    }

    private static func getTitle(of place: ModelPlace, provider: ModelProvider) -> String {
        switch place {
        case .claude: "Claude subscription"
        case .thisMac: "On this Mac"
        case .openRouter: "OpenRouter"
        case .server: provider.name
        }
    }

    /// OpenRouter's price, or how long a run on this Mac takes.
    private static func describe(_ entry: ProviderModel, at place: ModelPlace, on provider: ModelProvider, averages: ModelRunAverages) -> String? {
        switch place {
        case .openRouter:
            describePrice(inputUSDPerMillion: entry.inputUSDPerMillion, outputUSDPerMillion: entry.outputUSDPerMillion)
        case .thisMac:
            averages.getAverageMS(provider: provider.name, model: entry.id).map { describeAverage(milliseconds: $0) }
        case .claude, .server:
            nil
        }
    }

    /// Claude's strongest model first, then any full id; this Mac's and
    /// OpenRouter's by name; another server's as it lists them.
    private static func sort(_ choices: [ModelChoice], at place: ModelPlace) -> [ModelChoice] {
        switch place {
        case .claude:
            choices.enumerated().sorted { left, right in
                let leftRank = claudeOrder.firstIndex(of: left.element.routed.model) ?? claudeOrder.count
                let rightRank = claudeOrder.firstIndex(of: right.element.routed.model) ?? claudeOrder.count
                return (leftRank, left.offset) < (rightRank, right.offset)
            }.map(\.element)
        case .thisMac, .openRouter:
            choices.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
        case .server:
            choices
        }
    }

    private static func getRank(_ place: ModelPlace) -> Int {
        switch place {
        case .claude: 0
        case .thisMac: 1
        case .openRouter: 2
        case .server: 3
        }
    }

    /// Dollars to the cent, or to the tenth of a cent and below for what
    /// costs less: "$2.50", "$0.075", "$0.0001".
    private static func formatDollars(_ value: Double) -> String {
        var text = String(format: "%.4f", value)
        while text.hasSuffix("0"), let dot = text.firstIndex(of: "."), text.distance(from: dot, to: text.endIndex) > 3 {
            text.removeLast()
        }
        return "$" + text
    }
}
