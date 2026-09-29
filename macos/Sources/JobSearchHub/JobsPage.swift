import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobsModel {
    static let pageSize = 500
    private static let lastVisitPreferenceKey = "jobsLastVisitedAt"

    private(set) var items: [JobListItem] = []
    var hidesPoorFits = false
    /// When the Jobs page was opened before this visit; jobs first seen since
    /// are marked new.
    let previousVisit: Date?

    init() {
        previousVisit = UserDefaults.standard.object(forKey: Self.lastVisitPreferenceKey) as? Date
        UserDefaults.standard.set(Date.now, forKey: Self.lastVisitPreferenceKey)
    }

    /// The rows shown: best fit first, then newest, without poor fits when hidden.
    var shownItems: [JobListItem] {
        JobsOrder.sort(hidesPoorFits ? JobsOrder.hidePoorFits(items) : items)
    }
    private(set) var total = 0
    /// The facts read from postings, which the table can show as columns.
    private(set) var factColumns: [JobFactColumn] = []
    private(set) var isLoading = false
    private(set) var loadError: String?
    private(set) var isAddingToPipeline = false
    private(set) var pipelineNotice: String?
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
            factColumns = response.factColumns ?? []
            loadError = nil
        } catch {
            loadError = String(describing: error)
        }
    }

    /// Adds each job to the pipeline's first phase and reports what the
    /// server answered; a job already on the pipeline stays where it is.
    func addToPipeline(_ ids: Set<UUID>, with client: HubClient) async {
        isAddingToPipeline = true
        defer { isAddingToPipeline = false }
        var addedCount = 0
        var alreadyThereCount = 0
        for id in ids {
            do {
                let response = try await client.send("POST", "v1/applications", body: AddApplicationRequest(jobID: id), as: ApplicationResponse.self)
                if response.created { addedCount += 1 } else { alreadyThereCount += 1 }
            } catch {
                pipelineNotice = "Could not add to the pipeline: \(error)"
                return
            }
        }
        pipelineNotice = getPipelineAdditionsNotice(added: addedCount, alreadyThere: alreadyThereCount)
    }

    func clearPipelineNotice() {
        pipelineNotice = nil
    }

    private func getPipelineAdditionsNotice(added: Int, alreadyThere: Int) -> String {
        var parts: [String] = []
        if added > 0 { parts.append(added == 1 ? "Added 1 job to the pipeline" : "Added \(added) jobs to the pipeline") }
        if alreadyThere > 0 { parts.append(alreadyThere == 1 ? "1 was already on it" : "\(alreadyThere) were already on it") }
        return parts.joined(separator: "; ")
    }
}

struct JobsPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(DetailsInspector.self) private var details
    @Environment(CompanyJobFinder.self) private var jobFinder
    @State private var model = JobsModel()
    @State private var isAddingByURL = false
    /// Which columns show, in what order and width, kept across launches.
    @AppStorage("jobsTableColumns") private var savedColumns = Data()

    private let initialJobID: UUID?
    private let opensSession: Bool

    init(initialJobID: UUID? = nil, opensSession: Bool = false) {
        self.initialJobID = initialJobID
        self.opensSession = opensSession
        let model = JobsModel()
        model.selectedID = initialJobID
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                table(client: client)
                    .task(id: "\(model.search)|\(model.status.rawValue)") {
                        try? await Task.sleep(for: .milliseconds(250))
                        await model.load(with: client)
                    }
                    .onChange(of: [events.revision, unseen.revision, jobFinder.revision]) { Task { await model.load(with: client) } }
                    .onChange(of: model.selectedID, initial: true) {
                        details.show(model.selectedID.map { .job($0, opensSession: opensSession && $0 == initialJobID) }, from: .jobs)
                    }
                    .onChange(of: details.getSubject(on: .jobs)) {
                        if details.getSubject(on: .jobs) == nil { model.selectedID = nil }
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
        .navigationSubtitle(describeCounts())
    }

    private func describeCounts() -> String {
        let goodCount = model.items.filter { $0.fit.level == .good }.count
        let jobCount = model.total == model.items.count ? "\(model.total) jobs" : "\(model.items.count) of \(model.total) jobs"
        return "\(jobCount) · \(goodCount) good fits"
    }

    private func table(client: HubClient) -> some View {
        Table(of: JobListItem.self, selection: $model.selectedID, columnCustomization: columnCustomization) {
            TableColumn("Fit") { item in FitLabel(level: item.fit.level) }
                .width(70)
                .customizationID("fit")
            TableColumn("Title") { item in
                HStack(spacing: 6) {
                    UnseenDot(count: item.unseenUpdates)
                    Text(item.job.title).help(item.job.title)
                    if item.isNew(since: model.previousVisit) {
                        Text("New").font(.caption2.weight(.semibold)).padding(.horizontal, 5).padding(.vertical, 1)
                            .background(Color.accentColor.opacity(0.2), in: Capsule())
                    }
                }
            }
            .width(min: 160, ideal: 280)
            .customizationID("title")
            .disabledCustomizationBehavior(.visibility)
            TableColumn("Company") { item in Text(item.companyName ?? "–") }
                .width(min: 100, ideal: 140)
                .customizationID("company")
            TableColumn("Location") { item in Text(item.job.location ?? "").help(item.job.location ?? "") }
                .width(min: 120, ideal: 200)
                .customizationID("location")
            TableColumn("First seen") { item in Text(item.job.firstSeenAt.formatted(date: .abbreviated, time: .omitted)) }
                .width(100)
                .customizationID("firstSeen")
            TableColumnForEach(JobsColumns.boardFacts) { column in
                TableColumn(column.title) { item in
                    let text = column.getText(item).flatMap { $0.isEmpty ? nil : $0 } ?? "–"
                    Text(text).help(text)
                }
                .width(min: 70, ideal: 130)
                .customizationID(column.id)
                .defaultVisibility(.hidden)
            }
            TableColumnForEach(JobsColumns.fitChecks) { check in
                TableColumn(check.title) { item in FitCheckCell(check: item.getFitCheck(check.name)) }
                    .width(min: 80, ideal: 160)
                    .customizationID(check.id)
                    .defaultVisibility(.hidden)
            }
            TableColumnForEach(model.factColumns) { column in
                TableColumn(column.title) { item in
                    let text = item.getFactText(column.key) ?? "–"
                    Text(text).help(text)
                }
                .width(min: 80, ideal: 160)
                .customizationID(JobsColumns.getFactColumnID(column.key))
                .defaultVisibility(.hidden)
            }
        } rows: {
            ForEach(model.shownItems) { item in TableRow(item) }
        }
        .contextMenu(forSelectionType: UUID.self) { ids in
            Button("Open posting") { open(ids) }
            Button("Add to pipeline") { Task { await model.addToPipeline(ids, with: client) } }
                .disabled(ids.isEmpty || model.isAddingToPipeline)
        } primaryAction: { ids in
            open(ids)
        }
        .toolbar {
            Toggle("Hide poor fits", systemImage: "line.3.horizontal.decrease.circle", isOn: $model.hidesPoorFits)
                .help("Hide the jobs judged a poor fit; good and unclear ones stay")
            Picker("Status", selection: $model.status) {
                ForEach(JobStatusFilter.allCases) { status in Text(status.title).tag(status) }
            }
            .pickerStyle(.menu)
            .fixedSize()
            ColumnsMenu(customization: columnCustomization, factColumns: model.factColumns)
            Button("Add by URL", systemImage: "plus") { isAddingByURL = true }
            Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                .disabled(model.isLoading)
            ToolbarSearchField(text: $model.search, prompt: "Title, location or company")
                .frame(width: 180)
        }
        .overlay {
            if let loadError = model.loadError {
                ContentUnavailableView("Could not load jobs", systemImage: "exclamationmark.triangle", description: Text(loadError))
            } else if model.items.isEmpty && !model.isLoading {
                ContentUnavailableView("No jobs", systemImage: "briefcase", description: Text("Jobs from watched companies' boards appear here after the next poll."))
            }
        }
        .overlay(alignment: .bottom) {
            if let notice = model.pipelineNotice {
                Text(notice)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(.regularMaterial, in: Capsule())
                    .padding(.bottom, 16)
                    .task(id: notice) {
                        try? await Task.sleep(for: .seconds(4))
                        model.clearPipelineNotice()
                    }
            }
        }
    }

    private var columnCustomization: Binding<TableColumnCustomization<JobListItem>> {
        Binding(
            get: { (try? JSONDecoder().decode(TableColumnCustomization<JobListItem>.self, from: savedColumns)) ?? TableColumnCustomization() },
            set: { savedColumns = (try? JSONEncoder().encode($0)) ?? Data() }
        )
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

/// A job's fit level as a colored word.
struct FitLabel: View {
    let level: FitLevel

    var body: some View {
        Text(level.title)
            .foregroundStyle(color)
            .fontWeight(level == .good ? .semibold : .regular)
    }

    private var color: Color {
        switch level {
        case .good: .green
        case .unclear: .orange
        case .poor: .secondary
        }
    }
}
