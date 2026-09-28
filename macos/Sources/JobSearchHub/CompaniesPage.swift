import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class CompaniesModel {
    private(set) var summaries: [CompanySummary] = []
    private(set) var dossier: CompanyDossier?
    private(set) var isLoading = false
    private(set) var loadError: String?
    var selectedID: UUID?

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            summaries = try await client.get("v1/companies", as: CompaniesResponse.self).companies
            loadError = nil
            if selectedID == nil || !summaries.contains(where: { $0.id == selectedID }) {
                selectedID = summaries.first?.id
            }
        } catch {
            loadError = String(describing: error)
        }
    }

    func loadDossier(with client: HubClient) async {
        guard let selectedID else {
            dossier = nil
            return
        }
        do {
            dossier = try await client.get("v1/companies/\(selectedID.uuidString)", as: CompanyDossier.self)
        } catch {
            loadError = String(describing: error)
        }
    }
}

struct CompaniesPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var model = CompaniesModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                HStack(spacing: 0) {
                    companyTable
                    Divider()
                    DossierPane(dossier: model.dossier).frame(width: 400)
                }
                .task { await model.load(with: client) }
                .task(id: model.selectedID) { await model.loadDossier(with: client) }
                .toolbar {
                    Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                        .disabled(model.isLoading)
                }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Companies")
    }

    private var companyTable: some View {
        Table(model.summaries, selection: $model.selectedID) {
            TableColumn("Company", value: \.company.name)
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
            TableColumn("Found via") { summary in
                Text(summary.company.foundVia ?? "")
                    .help(summary.company.foundVia ?? "")
            }
        }
        .overlay {
            if let loadError = model.loadError {
                ContentUnavailableView("Could not load companies", systemImage: "exclamationmark.triangle", description: Text(loadError))
            }
        }
    }
}

struct DossierPane: View {
    let dossier: CompanyDossier?

    var body: some View {
        if let dossier {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    header(dossier.company)
                    if let summary = dossier.company.summary {
                        Text(summary)
                    }
                    if let foundVia = dossier.company.foundVia {
                        LabeledContent("Found via", value: foundVia)
                    }
                    if let watchedSince = dossier.watchedSince {
                        LabeledContent("On the watch list since", value: watchedSince.formatted(date: .abbreviated, time: .omitted))
                    }

                    section("Job boards") {
                        if dossier.jobBoards.isEmpty {
                            Text("None stored").foregroundStyle(.secondary)
                        }
                        ForEach(dossier.jobBoards) { board in
                            linkOrText(board.summaryLine, url: board.boardURL)
                        }
                    }

                    section("People") {
                        if dossier.people.isEmpty {
                            Text("None stored").foregroundStyle(.secondary)
                        }
                        ForEach(dossier.people) { person in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(person.name).bold()
                                Text([person.roleTitle, person.relevance.replacingOccurrences(of: "_", with: " ")].compactMap { $0 }.joined(separator: " · "))
                                    .foregroundStyle(.secondary)
                                linkOrText("Source", url: person.sourceURL)
                                    .font(.caption)
                            }
                        }
                    }
                }
                .padding()
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        } else {
            ContentUnavailableView("No company selected", systemImage: "building.2")
        }
    }

    private func header(_ company: Company) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(company.name).font(.title2).bold()
            HStack(spacing: 12) {
                linkOrText(company.domain, url: company.websiteURL)
                if let careersURL = company.careersURL {
                    linkOrText("Careers", url: careersURL)
                }
                if let country = company.headquartersCountry {
                    Text(country).foregroundStyle(.secondary)
                }
                if let size = company.employeeCountRange {
                    Text("\(size) people").foregroundStyle(.secondary)
                }
            }
        }
    }

    private func section<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.headline)
            content()
        }
    }

    @ViewBuilder
    private func linkOrText(_ title: String, url: String?) -> some View {
        if let url, let destination = URL(string: url) {
            Link(title, destination: destination)
        } else {
            Text(title)
        }
    }
}
