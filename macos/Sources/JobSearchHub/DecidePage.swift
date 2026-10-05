import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class DecideModel {
    private(set) var items: [DecisionQueueItem] = []
    private(set) var signals: DecisionSignals?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    /// Why the last decision failed.
    var actionError: HubFailure?
    var toast: ToastMessage?
    var selectedID: UUID?
    private var hasLoaded = false

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
        if let selectedID, items.contains(where: { $0.id == selectedID }) { return }
        if isFirstLoad {
            selectedID = items.first?.id
        } else if let previousIndex, !items.isEmpty {
            selectedID = items[min(previousIndex, items.count - 1)].id
        } else {
            selectedID = nil
        }
    }

    /// Records the decision and says so, then brings up the next job: the
    /// one after it, which stays selected once the queue reads again.
    func decide(_ item: DecisionQueueItem, _ decision: JobDecisionKind, reason: String = "", through decisions: JobDecisions, with client: HubClient) async -> HubFailure? {
        let nextID = KeyboardDecision.getNextID(after: item.id, in: items.map(\.id))
        do {
            _ = try await decisions.decide(item.id, decision, reason: reason, with: client)
        } catch {
            return HubFailure("Couldn't record the decision", error)
        }
        if selectedID == item.id { selectedID = nextID }
        switch decision {
        case .pursue: toast = ToastMessage(text: "Pursued \(item.job.title)")
        case .later: toast = ToastMessage(text: "Left \(item.job.title) for later", tone: .neutral, symbol: "clock")
        case .skip: toast = ToastMessage(text: "Skipped \(item.job.title)", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName)
        }
        return nil
    }
}

/// The briefed jobs to decide, best match first. The selected job opens on
/// its brief, where Pursue, Skip and Later decide it and bring up the next.
/// From the keyboard: ↑↓ move, Return opens, and P, L and S decide.
struct DecidePage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @Environment(JobDecisions.self) private var decisions
    @State private var model = DecideModel()
    @State private var skipping: DecisionQueueItem?
    @FocusState private var isQueueFocused: Bool

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                queue(client)
                    .task { await model.load(with: client) }
                    .onChange(of: [events.revision, decisions.revision]) { Task { await model.load(with: client) } }
                    .onChange(of: model.selectedID, initial: true) {
                        details.show(model.selectedID.map(InspectorSubject.job), from: .decide)
                    }
                    .onChange(of: details.getEntry(on: .decide)) {
                        // ⌘K can open another queued job; the list then selects it too.
                        if details.getEntry(on: .decide) == nil {
                            model.selectedID = nil
                        } else if let openItem {
                            model.selectedID = openItem.id
                        }
                    }
                    .onPageRequest(.decide) { request in
                        if request == .focusList { isQueueFocused = true }
                    }
                    .sheet(item: $skipping) { item in
                        SkipJobsSheet(jobCount: 1) { reason in
                            await model.decide(item, .skip, reason: reason, through: decisions, with: client)
                        }
                    }
            }
        }
        .navigationTitle("Decide")
        .navigationSubtitle(model.items.count == 1 ? "1 job to decide" : "\(model.items.count) jobs to decide")
    }

    /// The queued job the inspector shows, the one P, L and S decide.
    private var openItem: DecisionQueueItem? {
        model.items.first { details.getEntry(on: .decide)?.subject == .job($0.id) }
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }

    private func queue(_ client: HubClient) -> some View {
        List(selection: $model.selectedID) {
            if let signals = model.signals {
                Text(signals.summary).font(.hubCaption).foregroundStyle(.secondary)
                    .selectionDisabled()
            }
            ForEach(model.items) { item in
                DecisionQueueRow(item: item).tag(item.id)
            }
        }
        .focused($isQueueFocused)
        .contextMenu(forSelectionType: UUID.self) { ids in
            if let item = model.items.first(where: { ids == [$0.id] }) {
                Button("Pursue") { decide(item, .pursue, with: client) }
                    .keyboardShortcut("p", modifiers: [])
                if item.decision == nil {
                    Button("Later") { decide(item, .later, with: client) }
                        .keyboardShortcut("l", modifiers: [])
                }
                Button("Skip…") { decide(item, .skip, with: client) }
                    .keyboardShortcut("s", modifiers: [])
            }
        } primaryAction: { ids in
            // Return, or a double-click, opens the job, also after its details were closed.
            if let id = ids.first { details.show(.job(id), from: .decide) }
        }
        .onKeyPress(characters: .letters, phases: .down) { press in
            guard press.modifiers.isDisjoint(with: [.command, .control, .option]),
                  let decision = press.characters.first.flatMap(KeyboardDecision.getDecision(for:)),
                  let item = openItem
            else { return .ignored }
            decide(item, decision, with: client)
            return .handled
        }
        .overlay(alignment: .bottom) {
            if model.actionError != nil {
                HubErrorView($model.actionError)
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
        .toast($model.toast)
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            } else if model.items.isEmpty && !model.isLoading {
                ContentUnavailableView("Nothing to decide", systemImage: "checkmark.circle",
                                       description: Text("Briefed jobs wait here until you pursue, skip or leave them for later."))
            }
        }
    }

    /// Pursues the job or leaves it for later, then brings up the next; Skip
    /// asks why first. Later does nothing to a job already left for later.
    private func decide(_ item: DecisionQueueItem, _ decision: JobDecisionKind, with client: HubClient) {
        switch decision {
        case .skip:
            skipping = item
        case .later where item.decision != nil:
            break
        case .pursue, .later:
            Task {
                if let failure = await model.decide(item, decision, through: decisions, with: client) {
                    model.actionError = failure
                }
            }
        }
    }
}

/// One job in the queue: its match, title, company and the brief's reason.
struct DecisionQueueRow: View {
    let item: DecisionQueueItem

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            HStack(spacing: Space.s) {
                ToneChip(item.match)
                Text(item.job.title).fontWeight(.medium).lineLimit(1)
                if item.decision != nil {
                    ToneChip("Later", tone: .neutral, symbol: "clock")
                }
            }
            if let company = item.companyName {
                Text(company).font(.hubSecondary).foregroundStyle(.secondary)
            }
            Text(item.reason).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(2)
        }
        .padding(.vertical, Space.xs)
    }
}
