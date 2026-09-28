import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobsModel {
    static let pageSize = 500

    private(set) var items: [JobListItem] = []
    private(set) var total = 0
    private(set) var isLoading = false
    private(set) var loadError: String?
    var search = ""
    var status: JobStatusFilter = .open
    var selectedID: UUID?

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let response = try await client.get(
                "v1/jobs", query: JobsQuery.makeItems(search: search, status: status, limit: Self.pageSize), as: JobsResponse.self
            )
            items = response.jobs
            total = response.total
            loadError = nil
        } catch {
            loadError = String(describing: error)
        }
    }
}

struct JobsPage: View {
    @Environment(HubConnection.self) private var connection
    @State private var model = JobsModel()
    @State private var isAddingByURL = false

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                table(client: client)
                    .task(id: "\(model.search)|\(model.status.rawValue)") {
                        try? await Task.sleep(for: .milliseconds(250))
                        await model.load(with: client)
                    }
                    .sheet(isPresented: $isAddingByURL) {
                        AddJobSheet(client: client) { added in
                            model.selectedID = added.id
                            Task { await model.load(with: client) }
                        }
                    }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Jobs")
        .navigationSubtitle(model.total == model.items.count ? "\(model.total) jobs" : "\(model.items.count) of \(model.total) jobs")
    }

    private func table(client: HubClient) -> some View {
        Table(model.items, selection: $model.selectedID) {
            TableColumn("Title") { item in Text(item.job.title).help(item.job.title) }
            TableColumn("Company") { item in Text(item.companyName ?? "–") }
                .width(min: 100, ideal: 140)
            TableColumn("Location") { item in Text(item.job.location ?? "").help(item.job.location ?? "") }
                .width(min: 120, ideal: 200)
            TableColumn("First seen") { item in Text(item.job.firstSeenAt.formatted(date: .abbreviated, time: .omitted)) }
                .width(100)
        }
        .contextMenu(forSelectionType: UUID.self) { ids in
            Button("Open posting") { open(ids) }
            Button("Add to pipeline") {}
                .disabled(true)
                .help("Arrives with the pipeline board")
        } primaryAction: { ids in
            open(ids)
        }
        .searchable(text: $model.search, prompt: "Title, location or company")
        .toolbar {
            Picker("Status", selection: $model.status) {
                ForEach(JobStatusFilter.allCases) { status in Text(status.title).tag(status) }
            }
            .pickerStyle(.segmented)
            Button("Add by URL", systemImage: "plus") { isAddingByURL = true }
            Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                .disabled(model.isLoading)
        }
        .overlay {
            if let loadError = model.loadError {
                ContentUnavailableView("Could not load jobs", systemImage: "exclamationmark.triangle", description: Text(loadError))
            } else if model.items.isEmpty && !model.isLoading {
                ContentUnavailableView("No jobs", systemImage: "briefcase", description: Text("Jobs from watched companies' boards appear here after the next poll."))
            }
        }
    }

    private func open(_ ids: Set<UUID>) {
        for item in model.items where ids.contains(item.id) {
            if let url = URL(string: item.job.url) {
                NSWorkspace.shared.open(url)
            }
        }
    }
}

struct AddJobSheet: View {
    let client: HubClient
    let onAdded: (Job) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var url = ""
    @State private var title = ""
    @State private var isAdding = false
    @State private var errorMessage: String?

    var body: some View {
        Form {
            TextField("Posting URL", text: $url, prompt: Text("https://jobs.ashbyhq.com/…"))
            TextField("Title", text: $title, prompt: Text("Only needed for sites other than Greenhouse, Lever and Ashby"))
            if let errorMessage {
                Text(errorMessage).foregroundStyle(.red)
            }
            HStack {
                Spacer()
                if isAdding {
                    ProgressView().controlSize(.small)
                }
                Button("Cancel") { dismiss() }
                Button("Add") { Task { await add() } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(url.isEmpty || isAdding)
            }
        }
        .formStyle(.grouped)
        .frame(width: 520)
        .padding()
    }

    private func add() async {
        isAdding = true
        defer { isAdding = false }
        do {
            let response = try await client.send("POST", "v1/jobs", body: AddJobRequest(url: url, title: title), as: AddJobResponse.self)
            onAdded(response.job)
            dismiss()
        } catch {
            errorMessage = String(describing: error)
        }
    }
}
