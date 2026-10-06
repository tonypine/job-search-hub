import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompaniesModel {
    private(set) var summaries: [CompanySummary] = []
    /// The companies the hub suggests researching; nil until read.
    private(set) var suggestions: [CompanySuggestion]?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    private(set) var suggestionsError: HubFailure?
    var selectedID: UUID?
    var search = ""
    var scope: CompanyScope = .watching

    /// The scope's companies whose name or domain holds the search.
    var shownSummaries: [CompanySummary] {
        let query = search.trimmingCharacters(in: .whitespaces)
        let inScope = scope.getSummaries(summaries)
        guard !query.isEmpty else { return inScope }
        return inScope.filter { $0.company.name.localizedStandardContains(query) || $0.company.domain.localizedStandardContains(query) }
    }

    /// The suggestions whose name holds the search.
    var shownSuggestions: [CompanySuggestion] {
        let query = search.trimmingCharacters(in: .whitespaces)
        guard !query.isEmpty else { return suggestions ?? [] }
        return (suggestions ?? []).filter { $0.organization.localizedStandardContains(query) }
    }

    /// How many each scope holds, Suggested once read.
    func getCount(of scope: CompanyScope) -> Int? {
        scope == .suggested ? suggestions?.count : scope.getSummaries(summaries).count
    }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            summaries = try await client.get("v1/companies", as: CompaniesResponse.self).companies
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the companies", error)
        }
    }

    func loadSuggestions(with client: HubClient) async {
        do {
            suggestions = try await client.get("v1/company-suggestions", as: CompanySuggestionsResponse.self).suggestions
            suggestionsError = nil
        } catch {
            suggestionsError = HubFailure("Couldn't load the suggestions", error)
        }
    }
}

/// The companies, by scope: the watch list, the ones with open jobs that
/// pass the screen, and the ones suggested for it. The selected company opens
/// in the window's inspector.
struct CompaniesPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(CompanyResearch.self) private var research
    @Environment(CompanyJobFinder.self) private var jobFinder
    @Environment(DetailsInspector.self) private var details
    @State private var isAddingCompany = false
    @State private var model = CompaniesModel()
    /// Which columns show, in what order and width, kept across launches.
    @AppStorage("companiesTableColumns") private var savedColumns = Data()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header
                    table(client: client)
                }
                .task { await model.load(with: client) }
                .task { await model.loadSuggestions(with: client) }
                .onChange(of: [events.revision, unseen.revision, research.revision, jobFinder.revision]) { Task { await model.load(with: client) } }
                .onChange(of: research.revision) { Task { await model.loadSuggestions(with: client) } }
                .sheet(isPresented: $isAddingCompany) {
                    AddCompanySheet(client: client, getCompanyName: { id in model.summaries.first { $0.id == id }?.company.name }) { companyID in
                        model.scope = .watching
                        model.selectedID = companyID
                        Task { await model.load(with: client) }
                    }
                }
                .onChange(of: model.selectedID, initial: true) {
                    details.show(model.selectedID.map(InspectorSubject.company), from: .companies)
                }
                .onChange(of: details.getEntry(on: .companies)) {
                    if details.getEntry(on: .companies) == nil { model.selectedID = nil }
                }
                .focusedSceneValue(\.pageAdd, PageAddAction(title: "Add Company…") { isAddingCompany = true })
                .onPageRequest(.companies) { request in
                    switch request {
                    case .addCompany: isAddingCompany = true
                    case .addCompanyFromSuggestions: model.scope = .suggested
                    default: break
                    }
                }
            }
        }
        .navigationTitle("Companies")
        .navigationSubtitle(subtitle)
    }

    /// "38 watched · 12 with open jobs that pass".
    private var subtitle: String {
        let watched = model.getCount(of: .watching) ?? 0
        let withOpenJobs = model.getCount(of: .withOpenJobs) ?? 0
        return "\(watched) watched · \(withOpenJobs) with open jobs that pass"
    }

    /// The page's controls, in its header rather than in the window's
    /// toolbar: the scopes, a research that failed, the search, the columns
    /// and Add.
    private var header: some View {
        PageHeader {
            HStack(spacing: Space.m) {
                TabStrip(
                    items: CompanyScope.allCases.map { scope in
                        TabStripItem(id: scope, title: scope.title, count: model.getCount(of: scope))
                    },
                    selection: $model.scope
                )
                if case .failed = research.state {
                    Button {
                        isAddingCompany = true
                    } label: {
                        Label("Research of \(research.company) failed", systemImage: "exclamationmark.triangle.fill")
                    }
                    .buttonStyle(.borderless)
                    .foregroundStyle(Tone.negative.color)
                    .help("Show why, and try again")
                }
            }
        } trailing: {
            PageSearchField(text: $model.search, prompt: "Search companies")
                .help("Search names and domains")
            CompanyColumnsMenu(customization: columnCustomization)
                .labelStyle(.iconOnly)
                .fixedSize()
            Button("Add", systemImage: "plus") { isAddingCompany = true }
                .help("Research a company and watch it (⌘N)")
        }
    }

    private var rows: [CompanyRow] {
        if model.scope == .suggested {
            return model.shownSuggestions.map(CompanyRow.suggestion)
        }
        let companies = model.shownSummaries.map(CompanyRow.company)
        // A research whose company the watch list doesn't hold yet has a row
        // of its own at the top of it.
        let isListed = model.scope.getSummaries(model.summaries).contains { $0.isResearched(as: research.company) }
        if model.scope == .watching && research.isRunning && !isListed {
            // Named as the suggestion it came from, when it did.
            let name = model.suggestions?.first { $0.isResearched(as: research.company) }?.organization ?? research.company
            return [.research(name)] + companies
        }
        return companies
    }

    /// The research's step, when the row is what's being researched.
    private func getResearchStep(of row: CompanyRow) -> ResearchStep? {
        guard research.isRunning else { return nil }
        let isResearched = switch row {
        case let .company(summary): summary.isResearched(as: research.company)
        case let .suggestion(suggestion): suggestion.isResearched(as: research.company)
        case .research: true
        }
        return isResearched ? ResearchStep(lines: research.lines) : nil
    }

    private var selection: Binding<String?> {
        Binding(
            get: { model.selectedID.map(CompanyRow.getID) },
            set: { model.selectedID = $0.flatMap(CompanyRow.getCompanyID) }
        )
    }

    private func table(client: HubClient) -> some View {
        Table(of: CompanyRow.self, selection: selection, columnCustomization: columnCustomization) {
            TableColumn("Company") { row in
                CompanyCell(row: row, researchStep: getResearchStep(of: row))
            }
            .width(min: 220, ideal: 320)
            .customizationID("company")
            .disabledCustomizationBehavior(.visibility)
            TableColumn("Open jobs") { row in
                switch row {
                case let .company(summary): OpenJobsCell(fittingJobs: summary.fittingJobs ?? 0, bestMatch: summary.bestMatch)
                case let .suggestion(suggestion): OpenJobsCell(fittingJobs: suggestion.fittingJobs, bestMatch: nil)
                case .research: EmptyView()
                }
            }
            .width(min: 110, ideal: 190)
            .customizationID("openJobs")
            TableColumn("People you know") { row in
                switch row {
                case let .company(summary): KnownPeopleCell(names: summary.knownPeople ?? [], count: summary.connectionCount)
                case let .suggestion(suggestion): KnownPeopleCell(names: [], count: suggestion.connectionCount)
                case .research: EmptyView()
                }
            }
            .width(min: 90, ideal: 130)
            .customizationID("peopleYouKnow")
            TableColumn("You") { row in
                if let summary = row.summary {
                    StandingCell(standing: summary.getStanding(now: .now))
                }
            }
            .width(min: 90, ideal: 130)
            .customizationID("you")
            TableColumn("Board") { row in
                switch row {
                case let .company(summary):
                    BoardCell(summary: summary, isFinding: jobFinder.isRunning(summary.id)) {
                        jobFinder.start(companyID: summary.id, client: client)
                    }
                case let .suggestion(suggestion):
                    addCell(suggestion, client: client)
                case .research: EmptyView()
                }
            }
            .width(min: 90, ideal: 130)
            .customizationID("board")
            TableColumn("Domain") { row in
                optionalCell(row.summary?.company.domain)
            }
            .width(min: 80, ideal: 150)
            .customizationID("domain")
            .defaultVisibility(.hidden)
            TableColumn("Watched since") { row in
                optionalCell(row.summary?.watchedSince?.formatted(date: .abbreviated, time: .omitted))
            }
            .width(min: 80, ideal: 110)
            .customizationID("watchedSince")
            .defaultVisibility(.hidden)
            TableColumn("People stored") { row in
                optionalCell(row.summary.map { "\($0.peopleCount)" })
            }
            .width(min: 60, ideal: 90)
            .customizationID("people")
            .defaultVisibility(.hidden)
            TableColumn("Found via") { row in
                optionalCell(row.summary?.company.foundVia)
            }
            .width(min: 80, ideal: 160)
            .customizationID("foundVia")
            .defaultVisibility(.hidden)
        } rows: {
            ForEach(rows) { row in TableRow(row) }
        }
        .alternatingRowBackgrounds(.disabled)
        .overlay { emptyState(client: client) }
    }

    /// Add on a suggestion: researches it, as Add company does, and its row
    /// shows the research's step until the company joins the watch list.
    @ViewBuilder
    private func addCell(_ suggestion: CompanySuggestion, client: HubClient) -> some View {
        Group {
            if !suggestion.isResearched(as: research.company) || !research.isRunning {
                Button("Add") {
                    research.start(company: suggestion.researchTarget, foundVia: suggestion.origin, client: client)
                }
                .controlSize(.small)
                .disabled(research.isRunning)
                .help(research.isRunning ? "One research runs at a time" : "Research it and watch it. Researching costs an agent run.")
            }
        }
        .companyRowCell()
    }

    private func optionalCell(_ text: String?) -> some View {
        let shown = text.flatMap { $0.isEmpty ? nil : $0 } ?? "–"
        return Text(shown).font(.hubSecondary).lineLimit(1).help(shown).companyRowCell()
    }

    @ViewBuilder
    private func emptyState(client: HubClient) -> some View {
        let query = model.search.trimmingCharacters(in: .whitespaces)
        if model.scope == .suggested {
            if let error = model.suggestionsError {
                HubErrorView(error, style: .page, retry: { Task { await model.loadSuggestions(with: client) } })
            } else if model.suggestions?.isEmpty == true {
                ContentUnavailableView(
                    "No suggestions", systemImage: "sparkles",
                    description: Text("Import your LinkedIn archive in Settings › Accounts; the companies you follow show up here, beside the ones on startups.gallery's remote list with a job that passes your screen, read weekly.")
                )
            } else if model.suggestions != nil && model.shownSuggestions.isEmpty {
                ContentUnavailableView.search(text: query)
            }
        } else if let loadError = model.loadError {
            HubErrorView(loadError, style: .page, retry: { Task { await model.load(with: client) } })
        } else if rows.isEmpty && !model.isLoading && !query.isEmpty {
            ContentUnavailableView.search(text: query)
        } else if rows.isEmpty && !model.isLoading && model.scope == .withOpenJobs {
            ContentUnavailableView("No open jobs that pass", systemImage: "briefcase", description: Text("Companies with an open job that passes your screen show here."))
        } else if rows.isEmpty && !model.isLoading {
            ContentUnavailableView("No companies watched", systemImage: "building.2", description: Text("Add a company, or pick one under Suggested."))
        }
    }

    private var columnCustomization: Binding<TableColumnCustomization<CompanyRow>> {
        Binding(
            get: { (try? JSONDecoder().decode(TableColumnCustomization<CompanyRow>.self, from: savedColumns)) ?? TableColumnCustomization() },
            set: { savedColumns = (try? JSONEncoder().encode($0)) ?? Data() }
        )
    }
}

/// The header's menu that shows and hides the table's optional columns; the
/// table header's own menu does the same.
struct CompanyColumnsMenu: View {
    @Binding var customization: TableColumnCustomization<CompanyRow>

    /// The columns hidden until shown, by their customization IDs.
    private static let optionalColumns = [
        ("domain", "Domain"), ("watchedSince", "Watched since"), ("people", "People stored"), ("foundVia", "Found via"),
    ]

    var body: some View {
        Menu("Columns", systemImage: "tablecells") {
            ForEach(Self.optionalColumns, id: \.0) { id, title in
                Toggle(title, isOn: Binding(
                    get: { customization[visibility: id] == .visible },
                    set: { customization[visibility: id] = $0 ? .visible : .hidden }
                ))
            }
        }
        .help("Choose the columns the table shows")
    }
}
