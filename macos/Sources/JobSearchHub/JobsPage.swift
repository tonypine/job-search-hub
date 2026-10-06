import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobsModel {
    private static let lastVisitPreferenceKey = "jobsLastVisitedAt"

    private(set) var items: [JobListItem] = []
    /// When the Jobs page was opened before this visit; jobs first seen since
    /// are marked new.
    let previousVisit: Date?

    init() {
        previousVisit = UserDefaults.standard.object(forKey: Self.lastVisitPreferenceKey) as? Date
        UserDefaults.standard.set(Date.now, forKey: Self.lastVisitPreferenceKey)
    }

    func getMatchingItems(_ filter: JobsFilter) -> [JobListItem] {
        filter.getMatchingItems(items, previousVisit: previousVisit)
    }

    /// The rows shown: the jobs the filter keeps, in the chosen order, or else
    /// best fit first, then newest.
    func getShownItems(filteredBy filter: JobsFilter, sortedBy sortOrder: [JobsSortComparator]) -> [JobListItem] {
        let matchingItems = getMatchingItems(filter)
        return sortOrder.isEmpty ? JobsOrder.sort(matchingItems) : matchingItems.sorted(using: sortOrder)
    }
    private(set) var total = 0
    /// The facts read from postings, which the table can show as columns.
    private(set) var factColumns: [JobFactColumn] = []
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    private(set) var isPursuing = false
    /// What the last action on the selected jobs did, shown for a moment.
    var toast: ToastMessage?
    /// Why the last action on the selected jobs failed.
    var actionError: HubFailure?
    var search = ""
    var status: JobStatusFilter = .open
    /// How many jobs each scope holds for the search; All has no count.
    private(set) var scopeCounts: [JobStatusFilter: Int] = [:]
    var selectedIDs: Set<UUID> = []

    /// The job whose details show: the selection, when it's one job.
    var selectedID: UUID? {
        selectedIDs.count == 1 ? selectedIDs.first : nil
    }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let response = try await client.getAllJobs(search: search, status: status)
            items = response.jobs
            total = response.total
            factColumns = response.factColumns ?? []
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the jobs", error)
        }
        await loadScopeCounts(with: client)
    }

    /// Reads each scope's count; one that fails keeps its last.
    private func loadScopeCounts(with client: HubClient) async {
        let search = search
        async let open = try? client.getJobCount(search: search, status: .open)
        async let later = try? client.getJobCount(search: search, status: .later)
        async let closed = try? client.getJobCount(search: search, status: .closed)
        async let skipped = try? client.getJobCount(search: search, status: .dismissed)
        let counts: [(JobStatusFilter, Int?)] = await [(.open, open), (.later, later), (.closed, closed), (.dismissed, skipped)]
        for case let (status, count?) in counts {
            scopeCounts[status] = count
        }
    }

    /// Pursues each job, which puts it on the pipeline's first phase, and
    /// reports it; a job already on the pipeline stays where it is.
    func pursue(_ ids: Set<UUID>, through decisions: JobDecisions, with client: HubClient) async {
        isPursuing = true
        defer { isPursuing = false }
        for id in ids {
            do {
                _ = try await decisions.decide(id, .pursue, with: client)
            } catch {
                actionError = HubFailure("Couldn't pursue the jobs", error)
                return
            }
        }
        toast = ToastMessage(text: ids.count == 1 ? "Pursued 1 job" : "Pursued \(ids.count) jobs")
    }

    /// Skips the jobs and reports it, with Undo, or returns why it failed.
    func skip(_ ids: Set<UUID>, reason: String, through decisions: JobDecisions, with client: HubClient) async -> HubFailure? {
        do {
            let jobs = try await decisions.dismiss(ids, reason: reason, with: client)
            selectedIDs.subtract(ids)
            toast = ToastMessage(
                text: jobs.count == 1 ? "Skipped 1 job" : "Skipped \(jobs.count) jobs", tone: SetAside.skipped.tone,
                symbol: SetAside.skipped.symbolName
            ) { [weak self] in
                Task { await self?.restore(ids, through: decisions, with: client, reports: false) }
            }
            return nil
        } catch {
            return HubFailure("Couldn't skip the jobs", error)
        }
    }

    /// Restores the jobs, and says so unless it's an Undo.
    func restore(_ ids: Set<UUID>, through decisions: JobDecisions, with client: HubClient, reports: Bool = true) async {
        do {
            let jobs = try await decisions.restore(ids, with: client)
            selectedIDs.subtract(ids)
            if reports {
                toast = ToastMessage(text: jobs.count == 1 ? "Restored 1 job" : "Restored \(jobs.count) jobs")
            }
        } catch {
            actionError = HubFailure("Couldn't restore the jobs", error)
        }
    }
}

struct JobsPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(DetailsInspector.self) private var details
    @Environment(CompanyJobFinder.self) private var jobFinder
    @Environment(JobDecisions.self) private var decisions
    @State private var model = JobsModel()
    @State private var isAddingByURL = false
    @State private var skipping: JobSkipTarget?
    @State private var fix: JobFixTarget?
    @Environment(RemoteTaskRunner.self) private var taskRunner
    /// Which columns show, in what order and width, kept across launches.
    @AppStorage("jobsTableColumns") private var savedColumns = Data()
    /// The column the table sorts by, kept across launches; none keeps best fit first.
    @AppStorage("jobsSortOrder") private var savedSortOrder = Data()
    /// The filters chosen in the header, kept across launches.
    @AppStorage("jobsFilter") private var savedFilter = Data()

    private let initialJobID: UUID?
    private let opensSession: Bool

    init(initialJobID: UUID? = nil, opensSession: Bool = false) {
        self.initialJobID = initialJobID
        self.opensSession = opensSession
        let model = JobsModel()
        model.selectedIDs = Set([initialJobID].compactMap { $0 })
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header
                    table(client: client)
                }
                .task(id: "\(model.search)|\(model.status.rawValue)") {
                    try? await Task.sleep(for: .milliseconds(250))
                    await model.load(with: client)
                }
                .onChange(of: [events.revision, unseen.revision, jobFinder.revision, decisions.revision, taskRunner.fixRevision]) { Task { await model.load(with: client) } }
                .onChange(of: model.selectedID, initial: true) {
                    // `--session` opens the launch job on its Session tab, once.
                    if opensSession, let jobID = model.selectedID, jobID == initialJobID, details.getEntry(on: .jobs) == nil {
                        details.openSession(.job(jobID), from: .jobs)
                    } else {
                        details.show(model.selectedID.map(InspectorSubject.job), from: .jobs)
                    }
                }
                .onChange(of: details.getEntry(on: .jobs)) {
                    // Closing the details of one job deselects it; several selected jobs show no details at all.
                    if details.getEntry(on: .jobs) == nil && model.selectedID != nil { model.selectedIDs = [] }
                }
                .sheet(item: $skipping) { target in
                    SkipJobsSheet(jobCount: target.jobIDs.count) { reason in
                        await model.skip(target.jobIDs, reason: reason, through: decisions, with: client)
                    }
                }
                .sheet(item: $fix) { target in
                    FixJobSheet(jobTitle: target.title) { note in
                        do {
                            try await taskRunner.fixJob(target.id, note: note, with: client)
                            return nil
                        } catch {
                            return HubFailure("Couldn't ask for the fix", error)
                        }
                    }
                }
                .sheet(isPresented: $isAddingByURL) {
                    AddJobSheet(client: client) { added in
                        model.selectedIDs = [added.id]
                        Task { await model.load(with: client) }
                    }
                }
                .focusedSceneValue(\.pageAdd, PageAddAction(title: "Add Job by URL…") { isAddingByURL = true })
                .onPageRequest(.jobs) { request in
                    if request == .addJobByURL { isAddingByURL = true }
                }
            }
        }
        .navigationTitle("Jobs")
        .navigationSubtitle(describeCounts())
    }

    /// How many jobs show out of all, and how many of those pass the screen.
    private func describeCounts() -> String {
        let shownItems = model.getMatchingItems(filter.wrappedValue)
        let passCount = shownItems.count { $0.fit.level == .good }
        let jobCount = model.total == shownItems.count ? "\(model.total) jobs" : "\(shownItems.count) of \(model.total) jobs"
        return "\(jobCount) · " + (passCount == 1 ? "1 passes the screen" : "\(passCount) pass the screen")
    }

    /// The page's controls, in its header over the table rather than in the
    /// window's toolbar, which reaches over the details inspector: the
    /// status as scopes, the filters that are on as chips.
    private var header: some View {
        let choices = JobsFilterChoices(items: model.items)
        return PageHeader(chips: getFilterChips(choices: choices)) {
            TabStrip(
                items: JobStatusFilter.allCases.map { status in
                    TabStripItem(id: status, title: status.title, count: status == .all ? nil : model.scopeCounts[status])
                },
                selection: $model.status
            )
        } trailing: {
            PageSearchField(text: $model.search, prompt: "Search jobs")
                .help("Search titles, locations and companies")
            ColumnsMenu(customization: columnCustomization, factColumns: model.factColumns)
                .labelStyle(.iconOnly)
                .fixedSize()
            Button("Add", systemImage: "plus") { isAddingByURL = true }
                .help("Add a job by URL (⌘N). Generate missing CVs is in Jump to (⌘K).")
        } filterMenu: {
            JobsFilterMenu(filter: filter, choices: choices, newCount: model.items.count { $0.isNew(since: model.previousVisit) })
        }
    }

    private func getFilterChips(choices: JobsFilterChoices) -> [PageFilterChip] {
        let chips = filter.wrappedValue.getChips(choices: choices) { name in JobsColumns.fitChecks.first { $0.name == name }?.title ?? name }
        return chips.map { chip in
            PageFilterChip(id: String(describing: chip.kind), title: chip.title) {
                filter.wrappedValue = filter.wrappedValue.removing(chip.kind)
            }
        }
    }

    private func table(client: HubClient) -> some View {
        Table(of: JobListItem.self, selection: $model.selectedIDs, sortOrder: sortOrder, columnCustomization: columnCustomization) {
            TableColumn("Screen", sortUsing: JobsSortComparator(.fit)) { item in ToneChip(item.fit.level) }
                .width(70)
                .customizationID("fit")
            TableColumn("Title", sortUsing: JobsSortComparator(.title)) { item in
                HStack(spacing: Space.s) {
                    UnseenDot(count: item.unseenUpdates)
                    Text(item.job.title).help(item.job.title).layoutPriority(1)
                    if let reason = item.job.dismissalReason, !reason.isEmpty {
                        Text(reason).foregroundStyle(.secondary).help("Skipped: \(reason)")
                    }
                    if item.isNew(since: model.previousVisit) {
                        ToneChip("New", tone: .accent)
                    }
                }
            }
            .width(min: 160, ideal: 280)
            .customizationID("title")
            .disabledCustomizationBehavior(.visibility)
            TableColumn("Company", sortUsing: JobsSortComparator(.company)) { item in Text(item.companyName ?? "–") }
                .width(min: 100, ideal: 140)
                .customizationID("company")
            TableColumn("Location", sortUsing: JobsSortComparator(.location)) { item in Text(item.job.location ?? "").help(item.job.location ?? "") }
                .width(min: 120, ideal: 200)
                .customizationID("location")
            TableColumn("First seen", sortUsing: JobsSortComparator(.firstSeen)) { item in
                Text(item.job.firstSeenAt.formatted(date: .abbreviated, time: .omitted))
            }
                .width(100)
                .customizationID("firstSeen")
            TableColumnForEach(JobsColumns.boardFacts) { column in
                TableColumn(column.title, sortUsing: column.sortComparator) { item in
                    let text = column.getText(item).flatMap { $0.isEmpty ? nil : $0 } ?? "–"
                    Text(text).help(text)
                }
                .width(min: 70, ideal: 130)
                .customizationID(column.id)
                .defaultVisibility(.hidden)
            }
            TableColumnForEach(JobsColumns.fitChecks) { check in
                TableColumn(check.title, sortUsing: check.sortComparator) { item in FitCheckCell(check: item.getFitCheck(check.name)) }
                    .width(min: 80, ideal: 160)
                    .customizationID(check.id)
                    .defaultVisibility(.hidden)
            }
            TableColumnForEach(model.factColumns) { column in
                TableColumn(column.title, sortUsing: JobsSortComparator(.fact(column.key))) { item in
                    let text = item.getFactText(column.key) ?? "–"
                    Text(text).help(text)
                }
                .width(min: 80, ideal: 160)
                .customizationID(JobsColumns.getFactColumnID(column.key))
                .defaultVisibility(.hidden)
            }
        } rows: {
            ForEach(model.getShownItems(filteredBy: filter.wrappedValue, sortedBy: sortOrder.wrappedValue)) { item in TableRow(item) }
        }
        .contextMenu(forSelectionType: UUID.self) { ids in
            Button("Open posting") { open(ids) }
            Button("Pursue") { Task { await model.pursue(ids, through: decisions, with: client) } }
                .disabled(ids.isEmpty || model.isPursuing)
            Button("Fix…") {
                if let id = ids.first, let item = model.items.first(where: { $0.id == id }) {
                    fix = JobFixTarget(id: id, title: item.job.title)
                }
            }
            .disabled(ids.count != 1)
            Divider()
            if model.status == .dismissed {
                Button("Restore") { Task { await model.restore(ids, through: decisions, with: client) } }
                    .disabled(ids.isEmpty)
            } else {
                Button("Skip…") { skipping = JobSkipTarget(jobIDs: ids) }
                    .disabled(ids.isEmpty)
            }
        } primaryAction: { ids in
            open(ids)
        }
        .onDeleteCommand {
            if model.status != .dismissed && !model.selectedIDs.isEmpty {
                skipping = JobSkipTarget(jobIDs: model.selectedIDs)
            }
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(with: client) } }
            } else if model.items.isEmpty && !model.isLoading && model.status == .dismissed {
                ContentUnavailableView("No skipped jobs", systemImage: "tray", description: Text("Jobs skipped from the list show here, where they can be restored."))
            } else if model.items.isEmpty && !model.isLoading && model.status == .later {
                ContentUnavailableView("Nothing left for later", systemImage: "clock", description: Text("Jobs you leave for later show here until you decide them."))
            } else if model.items.isEmpty && !model.isLoading {
                ContentUnavailableView("No jobs", systemImage: "briefcase", description: Text("Jobs from watched companies' boards appear here after the next poll."))
            } else if model.getMatchingItems(filter.wrappedValue).isEmpty && !model.isLoading {
                ContentUnavailableView {
                    Label("No jobs match the filters", systemImage: "line.3.horizontal.decrease.circle")
                } actions: {
                    Button("Clear filters") { filter.wrappedValue = JobsFilter() }
                }
            }
        }
        .overlay(alignment: .bottom) {
            if model.actionError != nil {
                HubErrorView($model.actionError)
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
        .toast($model.toast)
    }

    private var filter: Binding<JobsFilter> {
        Binding(
            get: { (try? JSONDecoder().decode(JobsFilter.self, from: savedFilter)) ?? JobsFilter() },
            set: { savedFilter = (try? JSONEncoder().encode($0)) ?? Data() }
        )
    }

    private var sortOrder: Binding<[JobsSortComparator]> {
        Binding(
            get: { (try? JSONDecoder().decode([JobsSortComparator].self, from: savedSortOrder)) ?? [] },
            set: { savedSortOrder = (try? JSONEncoder().encode($0)) ?? Data() }
        )
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
    @State private var failure: HubFailure?

    var body: some View {
        Form {
            TextField("Posting URL", text: $url, prompt: Text("https://jobs.ashbyhq.com/…"))
            TextField("Title", text: $title, prompt: Text("Only needed for sites other than Greenhouse, Lever and Ashby"))
            if let failure {
                HubErrorView(failure)
            }
            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                AsyncButton("Add", busyTitle: "Adding…") { await add() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(url.isEmpty)
            }
        }
        .formStyle(.grouped)
        .frame(width: 520)
        .padding()
    }

    private func add() async {
        do {
            let response = try await client.send("POST", "v1/jobs", body: AddJobRequest(url: url, title: title), as: AddJobResponse.self)
            onAdded(response.job)
            dismiss()
        } catch {
            failure = HubFailure("Couldn't add the job", error)
        }
    }
}
