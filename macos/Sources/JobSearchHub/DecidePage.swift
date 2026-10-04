import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class DecideModel {
    private(set) var items: [DecisionQueueItem] = []
    private(set) var signals: DecisionSignals?
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
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
}

/// The briefed jobs to decide, best match first. The selected job opens on
/// its brief, where Pursue, Skip and Later decide it and bring up the next.
struct DecidePage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @Environment(JobDecisions.self) private var decisions
    @State private var model = DecideModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                queue
                    .task { await model.load(with: client) }
                    .onChange(of: [events.revision, decisions.revision]) { Task { await model.load(with: client) } }
                    .onChange(of: model.selectedID, initial: true) {
                        details.show(model.selectedID.map { .job($0, opensSession: false) }, from: .decide)
                    }
                    .onChange(of: details.getSubject(on: .decide)) {
                        if details.getSubject(on: .decide) == nil { model.selectedID = nil }
                    }
                    .toolbar {
                        Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                            .disabled(model.isLoading)
                    }
            } else {
                NotConnectedView()
            }
        }
        .navigationTitle("Decide")
        .navigationSubtitle(model.items.count == 1 ? "1 job to decide" : "\(model.items.count) jobs to decide")
    }

    private func reload() async {
        if let client = connection.makeClient() { await model.load(with: client) }
    }

    private var queue: some View {
        List(selection: $model.selectedID) {
            if let signals = model.signals {
                Text(signals.summary).font(.hubCaption).foregroundStyle(.secondary)
                    .selectionDisabled()
            }
            ForEach(model.items) { item in
                DecisionQueueRow(item: item).tag(item.id)
            }
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await reload() } }
            } else if model.items.isEmpty && !model.isLoading {
                ContentUnavailableView("Nothing to decide", systemImage: "checkmark.circle",
                                       description: Text("Briefed jobs wait here until you pursue, skip or leave them for later."))
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
