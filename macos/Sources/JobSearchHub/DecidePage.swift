import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class DecideModel {
    private(set) var items: [DecisionQueueItem] = []
    private(set) var signals: DecisionSignals?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    /// Why the last decision, or its Undo, failed.
    var actionError: HubFailure?
    var toast: ToastMessage?
    /// The skipped job whose reason sheet is open, from the toast.
    var reasonTarget: JobSkipTarget?
    /// The job open beside the queue, the one P, L and S decide.
    var selectedID: UUID? {
        didSet {
            if selectedID != restoringID { restoringID = nil }
        }
    }

    /// The job an Undo brought back, which stays open while the queue reads
    /// again, even when a read started before the Undo lands without it.
    private var restoringID: UUID?
    private var hasLoaded = false

    /// "7 to decide · 12 decided this week".
    var subtitle: String {
        DecisionSignals.makeSubtitle(toDecide: items.count, signals: signals)
    }

    /// Reads the queue and the signals. The first read selects the best job;
    /// later reads keep the selection's place in the queue, so after a
    /// decision takes a job out, the next one is selected.
    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        let previousIndex = items.firstIndex { $0.id == selectedID }
        do {
            async let queue = client.getDecisionQueue()
            async let readSignals = client.getDecisionSignals()
            let (loadedQueue, loadedSignals) = try await (queue, readSignals)
            items = loadedQueue.items
            signals = loadedSignals
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the queue", error)
            return
        }
        let isFirstLoad = !hasLoaded
        hasLoaded = true
        if let restoringID {
            if items.contains(where: { $0.id == restoringID }) { self.restoringID = nil }
            return
        }
        if let selectedID, items.contains(where: { $0.id == selectedID }) { return }
        if isFirstLoad {
            selectedID = items.first?.id
        } else if let previousIndex, !items.isEmpty {
            selectedID = items[min(previousIndex, items.count - 1)].id
        } else {
            selectedID = nil
        }
    }

    /// The job a step down (1) or up (-1) the queue, without deciding; it
    /// stops at either end.
    func move(by step: Int) {
        guard !items.isEmpty else { return }
        selectedID = KeyboardDecision.move(from: selectedID, by: step, in: items.map(\.id))
    }

    /// Decides the job at once and brings up the next, then says so with
    /// Undo, so nothing asks first; Skip's note offers a reason too. Later
    /// does nothing to a job already left for later.
    func decide(_ item: DecisionQueueItem, _ decision: JobDecisionKind, through decisions: JobDecisions, with client: HubClient) async {
        if decision == .later && item.decision != nil { return }
        let nextID = KeyboardDecision.getNextID(after: item.id, in: items.map(\.id))
        do {
            _ = try await decisions.decide(item.id, decision, with: client)
        } catch {
            actionError = HubFailure("Couldn't record the decision", error)
            return
        }
        if selectedID == item.id { selectedID = nextID }
        let undo: @MainActor () -> Void = { [weak self] in
            Task { await self?.undo(item, through: decisions, with: client) }
        }
        switch decision {
        case .pursue:
            toast = ToastMessage(text: "Pursued \(item.job.title)", undo: undo)
        case .later:
            toast = ToastMessage(text: "Left \(item.job.title) for later", tone: .neutral, symbol: "clock", undo: undo)
        case .skip:
            toast = ToastMessage(
                text: "Skipped \(item.job.title)", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName, undo: undo,
                action: ToastAction(title: "Add reason") { [weak self] in
                    self?.reasonTarget = JobSkipTarget(jobIDs: [item.id])
                }
            )
        }
    }

    /// Takes the decision back, which puts the job back in the queue, and
    /// opens it again. A pursue's card leaves the pipeline with it, unless
    /// the card changed since, which the note then says.
    func undo(_ item: DecisionQueueItem, through decisions: JobDecisions, with client: HubClient) async {
        let cleared: [UUID: ClearedJobDecision]
        do {
            cleared = try await decisions.clear([item.id], with: client)
        } catch {
            actionError = HubFailure("Couldn't undo the decision", error)
            return
        }
        restoringID = item.id
        selectedID = item.id
        if cleared[item.id]?.keptApplicationID != nil {
            toast = ToastMessage(
                text: "\(item.job.title) stays on the pipeline: its card changed since the pursue", tone: .neutral, symbol: "info.circle"
            )
        }
    }

    /// Saves why the skipped job was skipped, or returns why it failed.
    func addSkipReason(_ reason: String, to ids: Set<UUID>, through decisions: JobDecisions, with client: HubClient) async -> HubFailure? {
        do {
            _ = try await decisions.dismiss(ids, reason: reason, with: client)
            return nil
        } catch {
            return HubFailure("Couldn't save the reason", error)
        }
    }
}

/// The briefed jobs to decide, best match first, in a narrow column, with
/// the selected job's page beside it: the posting and the cards that judge
/// it, so deciding is reading (TP-672). There's no inspector: P, L and S
/// decide the open job and bring up the next, with Undo; ↑↓ move through the
/// queue without deciding.
///
/// Nothing here follows its content's width: the queue is a constant 300
/// pt, its rows a constant height, and the job's page takes what's left and
/// asks for none of its own, so no window width can loop the layout (#54).
struct DecidePage: View {
    /// The queue's width, whatever the window's.
    static let queueWidth: CGFloat = 300

    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @Environment(JobDecisions.self) private var decisions
    @Environment(UnseenUpdates.self) private var unseen
    @State private var model = DecideModel()
    /// The open job's tab, kept as the queue moves on.
    @State private var tab = InspectorTab.posting
    @FocusState private var isQueueFocused: Bool

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content(client)
                    .task { await model.load(with: client) }
                    .onChange(of: [events.revision, unseen.revision, decisions.revision]) { Task { await model.load(with: client) } }
                    // The page owns the inspector's history while it shows, so a
                    // company opened from the job's page shows beside it.
                    .onAppear { details.show(nil, from: .decide) }
                    .onChange(of: details.getEntry(on: .decide)) { openQueuedJob() }
                    .onPageRequest(.decide) { request in
                        if request == .focusList { isQueueFocused = true }
                    }
                    .sheet(item: $model.reasonTarget) { target in
                        SkipReasonSheet(jobCount: target.jobIDs.count) { reason in
                            await model.addSkipReason(reason, to: target.jobIDs, through: decisions, with: client)
                        }
                    }
            }
        }
        .navigationTitle("Decide")
        .navigationSubtitle(model.subtitle)
    }

    @ViewBuilder
    private func content(_ client: HubClient) -> some View {
        if let loadError = model.loadError, model.items.isEmpty {
            HubErrorView(loadError, style: .page, retry: { Task { await model.load(with: client) } })
        } else if model.items.isEmpty && !model.isLoading {
            ContentUnavailableView("Nothing to decide", systemImage: "checkmark.circle",
                                   description: Text("Briefed jobs wait here until you pursue, skip or leave them for later."))
        } else {
            HStack(spacing: 0) {
                DecideQueueColumn(items: model.items, selectedID: $model.selectedID, isFocused: $isQueueFocused) { item, decision in
                    Task { await model.decide(item, decision, through: decisions, with: client) }
                }
                .frame(width: Self.queueWidth)
                Divider()
                jobPage(client)
            }
            // ↑ and ↓ go through the queue wherever the keyboard is on the
            // page, but in a field; the job's page takes P, L and S.
            .background {
                JobPageKeys { key in
                    switch key {
                    case .previous: model.move(by: -1)
                    case .next: model.move(by: 1)
                    case .back, .letter: return false
                    }
                    return true
                }
            }
            .overlay(alignment: .bottom) {
                if model.loadError != nil || model.actionError != nil {
                    VStack(spacing: Space.s) {
                        // A read that failed leaves the queue from the last one up.
                        if let loadError = model.loadError {
                            HubErrorView(loadError, retry: { Task { await model.load(with: client) } })
                        }
                        if model.actionError != nil {
                            HubErrorView($model.actionError)
                        }
                    }
                    .frame(maxWidth: 560)
                    .padding(Space.l)
                }
            }
            .toast($model.toast)
        }
    }

    /// The open job as a page, without the posting's outline: the queue
    /// takes its place on the left.
    @ViewBuilder
    private func jobPage(_ client: HubClient) -> some View {
        Group {
            if let item = openItem {
                JobDetailView(jobID: item.id, client: client, tab: $tab, layout: .page, showsOutline: false) { decision in
                    await model.decide(item, decision, through: decisions, with: client)
                }
                .id(item.id)
            } else if let selectedID = model.selectedID {
                // A job an Undo brought back, before the queue reads it again.
                JobDetailView(jobID: selectedID, client: client, tab: $tab, layout: .page, showsOutline: false)
                    .id(selectedID)
            } else {
                ContentUnavailableView("Select a job", systemImage: "rectangle.stack",
                                       description: Text("Its posting and the cards that judge it open here."))
            }
        }
        .frame(minWidth: 0, maxWidth: .infinity, minHeight: 0, maxHeight: .infinity, alignment: .top)
        .background(.background)
    }

    /// The queued job open beside the queue, the one P, L and S decide.
    private var openItem: DecisionQueueItem? {
        model.items.first { $0.id == model.selectedID }
    }

    /// A queued job opened in the inspector, from ⌘K or a company's jobs,
    /// opens beside the queue instead; any other job stays in the inspector.
    private func openQueuedJob() {
        guard case let .job(jobID)? = details.getEntry(on: .decide)?.subject, model.items.contains(where: { $0.id == jobID }) else { return }
        model.selectedID = jobID
        details.hide()
    }
}

/// The queue's column: how it's ordered on top, the jobs, and the keys that
/// work them at its foot.
struct DecideQueueColumn: View {
    let items: [DecisionQueueItem]
    @Binding var selectedID: UUID?
    var isFocused: FocusState<Bool>.Binding
    let decide: (DecisionQueueItem, JobDecisionKind) -> Void

    var body: some View {
        VStack(spacing: 0) {
            Text("Best match first")
                .font(.hubSecondary)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .padding(.horizontal, Space.l)
                .frame(maxWidth: .infinity, minHeight: 36, maxHeight: 36, alignment: .leading)
            Divider()
            List(selection: $selectedID) {
                ForEach(items) { item in
                    DecisionQueueRow(item: item).tag(item.id)
                }
            }
            .focused(isFocused)
            .contextMenu(forSelectionType: UUID.self) { ids in
                if let item = items.first(where: { ids == [$0.id] }) {
                    Button("Pursue") { decide(item, .pursue) }
                    if item.decision == nil {
                        Button("Later") { decide(item, .later) }
                    }
                    Button("Skip") { decide(item, .skip) }
                }
            }
            Divider()
            DecideKeyLegend()
        }
    }
}

/// The keys that work the queue, at its foot: P Pursue, L Later, S Skip,
/// ↑↓ Move.
struct DecideKeyLegend: View {
    var body: some View {
        HStack(spacing: Space.m) {
            entry("P", "Pursue")
            entry("L", "Later")
            entry("S", "Skip")
            entry("↑↓", "Move")
            Spacer(minLength: 0)
        }
        .font(.hubCaption)
        .foregroundStyle(.secondary)
        .lineLimit(1)
        .padding(.horizontal, Space.l)
        .frame(height: 32)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("P pursues, L leaves for later, S skips, and the arrow keys move through the queue")
    }

    private func entry(_ key: String, _ title: String) -> some View {
        HStack(spacing: Space.xs) {
            Text(key)
                .font(.caption2.weight(.medium).monospaced())
                .padding(.horizontal, Space.xs)
                .padding(.vertical, 1)
                .overlay(RoundedRectangle(cornerRadius: 3).strokeBorder(.separator))
            Text(title)
        }
        .fixedSize()
    }
}

/// One job in the queue: its match in its tone and its company, the title,
/// the brief's reason, and a dot for updates you haven't seen. Its height
/// never depends on the list's width: each line is truncated, and the
/// reason always takes two lines. Rows that grow as the list narrows can
/// loop the window's layout until AppKit stops the app.
struct DecisionQueueRow: View {
    let item: DecisionQueueItem
    /// Increased on the selected row of the focused queue, drawn in the
    /// accent, which a match in the accent tone would vanish into.
    @Environment(\.backgroundProminence) private var prominence

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: Space.xs) {
                Text(item.match.title)
                    .fontWeight(.medium)
                    .foregroundStyle(prominence == .increased ? AnyShapeStyle(.primary) : AnyShapeStyle(item.match.tone.color))
                if let company = item.companyName, !company.isEmpty {
                    Text("· \(company)").foregroundStyle(.secondary)
                }
                if item.decision != nil {
                    Label("Later", systemImage: "clock")
                        .labelStyle(.iconOnly)
                        .foregroundStyle(.secondary)
                        .help("Left for later")
                }
                Spacer(minLength: Space.xs)
                UnseenDot(count: item.unseenUpdates ?? 0)
            }
            .font(.hubCaption)
            .lineLimit(1)
            Text(item.job.title).fontWeight(.semibold).lineLimit(1)
            Text(item.reason).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(2, reservesSpace: true)
        }
        .padding(.vertical, Space.xs)
    }
}
