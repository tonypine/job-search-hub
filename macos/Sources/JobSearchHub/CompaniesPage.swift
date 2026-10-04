import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompaniesModel {
    private(set) var summaries: [CompanySummary] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    var selectedID: UUID?

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
}

/// The watch list. The selected company opens in the window's inspector.
struct CompaniesPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(CompanyResearch.self) private var research
    @Environment(CompanyJobFinder.self) private var jobFinder
    @Environment(DetailsInspector.self) private var details
    @State private var isAddingCompany = false
    @State private var isShowingSuggestions = false
    @State private var model = CompaniesModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                companyTable
                    .task { await model.load(with: client) }
                    .onChange(of: [events.revision, unseen.revision, research.revision, jobFinder.revision]) { Task { await model.load(with: client) } }
                    .sheet(isPresented: $isShowingSuggestions) {
                        CompanySuggestionsSheet(client: client) { suggestion in
                            research.start(company: suggestion.researchTarget, foundVia: suggestion.origin, client: client)
                            isShowingSuggestions = false
                            isAddingCompany = true
                        }
                    }
                    .sheet(isPresented: $isAddingCompany) {
                        AddCompanySheet(client: client, getCompanyName: { id in model.summaries.first { $0.id == id }?.company.name }) { companyID in
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
                    .toolbar {
                        if research.isRunning {
                            Button {
                                isAddingCompany = true
                            } label: {
                                HStack(spacing: Space.s) {
                                    ProgressView().controlSize(.small)
                                    Text("Researching \(research.company)")
                                }
                            }
                            .help("Show the research's progress")
                        }
                        Menu("Add", systemImage: "plus") {
                            Button("Company…") { isAddingCompany = true }
                            Button("From suggestions…") { isShowingSuggestions = true }
                                .help("Companies you follow on LinkedIn, or remote ones on startups.gallery, to research")
                        }
                        .help("Add a company (⌘N), or pick one from suggestions")
                    }
                    .focusedSceneValue(\.pageAdd, PageAddAction(title: "Add Company…") { isAddingCompany = true })
            }
        }
        .navigationTitle("Companies")
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }

    private var companyTable: some View {
        Table(model.summaries, selection: $model.selectedID) {
            TableColumn("Company") { summary in
                HStack(spacing: Space.s) {
                    UnseenDot(count: summary.unseenUpdates)
                    Text(summary.company.name)
                }
            }
            TableColumn("Domain", value: \.company.domain)
            TableColumn("Watched since") { summary in
                Text(summary.watchedSince.map { $0.formatted(date: .abbreviated, time: .omitted) } ?? "–")
            }
            TableColumn("Job board") { summary in
                Text(summary.jobBoards.first?.summaryLine ?? "–")
            }
            TableColumn("People") { summary in
                Text("\(summary.peopleCount)")
            }
            .width(60)
            TableColumn("You know") { summary in
                Text(summary.connectionCount > 0 ? "\(summary.connectionCount)" : "–")
                    .fontWeight(summary.connectionCount > 0 ? .semibold : .regular)
            }
            .width(70)
            TableColumn("Found via") { summary in
                Text(summary.company.foundVia ?? "")
                    .help(summary.company.foundVia ?? "")
            }
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            }
        }
    }
}
