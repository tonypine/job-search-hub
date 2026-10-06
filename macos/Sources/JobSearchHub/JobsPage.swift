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

    /// The jobs whose skip the reason sheet is open for, from the toast.
    var reasonTarget: JobSkipTarget?

    /// Decides the jobs at once and says so. Pursue puts them on the
    /// pipeline, which Undo can't take back; Later and Skip offer Undo, and
    /// Skip a reason too, so nothing asks first.
    func decide(_ ids: Set<UUID>, _ decision: JobDecisionKind, through decisions: JobDecisions, with client: HubClient) async {
        guard !ids.isEmpty else { return }
        let name = describe(ids)
        switch decision {
        case .pursue:
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
            toast = ToastMessage(text: "Pursued \(name)")
        case .later:
            for id in ids {
                do {
                    _ = try await decisions.decide(id, .later, with: client)
                } catch {
                    actionError = HubFailure("Couldn't leave the jobs for later", error)
                    return
                }
            }
            toast = ToastMessage(text: "Left \(name) for later", tone: .neutral, symbol: "clock") { [weak self] in
                Task { await self?.undoLater(ids, through: decisions, with: client) }
            }
        case .skip:
            do {
                _ = try await decisions.dismiss(ids, reason: "", with: client)
            } catch {
                actionError = HubFailure("Couldn't skip the jobs", error)
                return
            }
            selectedIDs.subtract(ids)
            toast = ToastMessage(
                text: "Skipped \(name)", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName,
                undo: { [weak self] in
                    Task { await self?.restore(ids, through: decisions, with: client, reports: false) }
                },
                action: ToastAction(title: "Add reason") { [weak self] in
                    self?.reasonTarget = JobSkipTarget(jobIDs: ids)
                }
            )
        }
    }

    /// Saves why the skipped jobs were skipped, or returns why it failed.
    func addSkipReason(_ reason: String, to ids: Set<UUID>, through decisions: JobDecisions, with client: HubClient) async -> HubFailure? {
        do {
            _ = try await decisions.dismiss(ids, reason: reason, with: client)
            return nil
        } catch {
            return HubFailure("Couldn't save the reason", error)
        }
    }

    private func undoLater(_ ids: Set<UUID>, through decisions: JobDecisions, with client: HubClient) async {
        do {
            try await decisions.clear(ids, with: client)
        } catch {
            actionError = HubFailure("Couldn't undo Later", error)
        }
    }

    /// One job by its title, or several by their count.
    private func describe(_ ids: Set<UUID>) -> String {
        if ids.count == 1, let item = items.first(where: { ids.contains($0.id) }) {
            return item.job.title
        }
        return ids.count == 1 ? "1 job" : "\(ids.count) jobs"
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
    @State private var fix: JobFixTarget?
    @State private var hover = JobRowHover()
    @Environment(RemoteTaskRunner.self) private var taskRunner
    /// Which columns show, in what order and width, kept across launches.
    @AppStorage("jobsTableColumns") private var savedColumns = Data()
    /// The column the table sorts by, kept across launches; Posted, newest
    /// first, until one is chosen.
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
                .sheet(item: $model.reasonTarget) { target in
                    SkipReasonSheet(jobCount: target.jobIDs.count) { reason in
                        await model.addSkipReason(reason, to: target.jobIDs, through: decisions, with: client)
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
        let sort = sortOrder.wrappedValue
        let shownItems = model.getShownItems(filteredBy: filter.wrappedValue, sortedBy: sort)
        // Sorted by Posted, the rows group by when the hub first saw them.
        let groups = sort.first?.column == .posted
            ? JobGroups.make(shownItems, newestFirst: sort.first?.order == .reverse, now: .now)
            : nil
        return Table(of: JobListItem.self, selection: $model.selectedIDs, sortOrder: sortOrder, columnCustomization: columnCustomization) {
            TableColumn("Job", sortUsing: JobsSortComparator(.title)) { item in
                JobCell(
                    item: item, isSelected: model.selectedIDs.contains(item.id), hover: hover, isSkipped: model.status == .dismissed,
                    onDecide: { decision in decide(getTargets(of: item.id), decision, with: client) },
                    onRestore: { Task { await model.restore(getTargets(of: item.id), through: decisions, with: client) } }
                )
            }
            .width(min: 240, ideal: 380)
            .customizationID("job")
            .disabledCustomizationBehavior(.visibility)
            TableColumn("Match", sortUsing: JobsSortComparator(.match)) { item in
                MatchCell(match: item.match).jobRowCell(item.id, hover: hover)
            }
            .width(min: 80, ideal: 96)
            .customizationID("match")
            TableColumn("Screen", sortUsing: JobsSortComparator(.fit)) { item in
                ScreenCell(fit: item.fit).jobRowCell(item.id, hover: hover)
            }
            .width(min: 84, ideal: 110)
            .customizationID("screen")
            TableColumn("Take-home", sortUsing: JobsSortComparator(.takeHome)) { item in
                TakeHomeCell(check: item.getFitCheck("Pay")).jobRowCell(item.id, hover: hover, alignment: .trailing)
            }
            .width(min: 64, ideal: 84)
            .alignment(.trailing)
            .customizationID("takeHome")
            TableColumn("Posted", sortUsing: JobsSortComparator(.posted)) { item in
                PostedCell(job: item.job).jobRowCell(item.id, hover: hover, alignment: .trailing)
            }
            .width(min: 48, ideal: 60)
            .alignment(.trailing)
            .customizationID("posted")
            TableColumnForEach(JobsColumns.jobDetails + JobsColumns.boardFacts) { column in
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
            if let groups {
                ForEach(groups) { group in
                    Section {
                        ForEach(group.items) { item in TableRow(item) }
                    } header: {
                        JobGroupHeader(title: group.kind.title, count: group.items.count)
                    }
                }
            } else {
                ForEach(shownItems) { item in TableRow(item) }
            }
        }
        .alternatingRowBackgrounds(.disabled)
        .contextMenu(forSelectionType: UUID.self) { ids in
            Button("Open posting") { open(ids) }
            Button("Pursue") { decide(ids, .pursue, with: client) }
                .disabled(ids.isEmpty || model.isPursuing)
            Button("Later") { decide(ids, .later, with: client) }
                .disabled(ids.isEmpty)
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
                Button("Skip") { decide(ids, .skip, with: client) }
                    .disabled(ids.isEmpty)
            }
        } primaryAction: { ids in
            open(ids)
        }
        .onKeyPress(characters: .letters, phases: .down) { press in
            // P, L and S decide the selected rows; Skip leaves skipped jobs alone.
            guard press.modifiers.isDisjoint(with: [.command, .control, .option]),
                  let decision = press.characters.first.flatMap(KeyboardDecision.getDecision(for:)),
                  !model.selectedIDs.isEmpty, !(decision == .skip && model.status == .dismissed)
            else { return .ignored }
            decide(model.selectedIDs, decision, with: client)
            return .handled
        }
        .onDeleteCommand {
            if model.status != .dismissed {
                decide(model.selectedIDs, .skip, with: client)
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
            get: { (try? JSONDecoder().decode([JobsSortComparator].self, from: savedSortOrder)) ?? [JobsSortComparator(.posted, order: .reverse)] },
            set: { savedSortOrder = (try? JSONEncoder().encode($0)) ?? Data() }
        )
    }

    private var columnCustomization: Binding<TableColumnCustomization<JobListItem>> {
        Binding(
            get: { (try? JSONDecoder().decode(TableColumnCustomization<JobListItem>.self, from: savedColumns)) ?? TableColumnCustomization() },
            set: { savedColumns = (try? JSONEncoder().encode($0)) ?? Data() }
        )
    }

    /// The jobs a row's action works on: the selection when the row is in
    /// it, or else that row alone.
    private func getTargets(of id: UUID) -> Set<UUID> {
        model.selectedIDs.contains(id) ? model.selectedIDs : [id]
    }

    private func decide(_ ids: Set<UUID>, _ decision: JobDecisionKind, with client: HubClient) {
        Task { await model.decide(ids, decision, through: decisions, with: client) }
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
