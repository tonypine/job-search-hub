import JobSearchHubCore
import SwiftUI

/// The hub's kinds of run and what each is doing: its pause, its runs
/// running and waiting, its failures over the last day, and its model.
@MainActor
@Observable
final class RunKindsModel {
    private(set) var groups: [RunGroup] = []
    private(set) var kinds: [RunKind] = []
    /// Each kind's failed runs over the last day.
    private(set) var failedCounts: [String: Int] = [:]
    private(set) var routes: [TaskRoute] = []
    private(set) var providers: [ModelProvider] = []
    private(set) var hasLoaded = false
    /// Why the last read failed; the next read that works clears it.
    var failure: HubFailure?
    /// Why the last pause or resume failed. Reads leave it, so it stays
    /// until the owner dismisses it or tries again.
    var pauseFailure: HubFailure?

    var sections: [RunKindSection] { RunKinds.group(kinds, in: groups) }
    var all: AllRunKinds { AllRunKinds(kinds, failedCounts: failedCounts) }

    func getKind(_ id: String) -> RunKind? {
        kinds.first { $0.id == id }
    }

    func getModel(for kind: RunKind) -> RunKindModel? {
        RunKinds.getModel(for: kind, routes: routes, providers: providers)
    }

    func getFailedCount(_ kind: RunKind) -> Int {
        failedCounts[kind.id, default: 0]
    }

    /// Reads the kinds and their failures. The failures are an extra: the
    /// column shows without them.
    func load(with client: HubClient) async {
        do {
            apply(try await client.get("v1/run-kinds", as: RunKindsResponse.self))
            failure = nil
        } catch {
            failure = HubFailure("Couldn't read the kinds of run", error)
        }
        let now = Date.now
        let window = [
            URLQueryItem(name: "from", value: now.addingTimeInterval(-24 * 60 * 60).formatted(.iso8601)),
            URLQueryItem(name: "to", value: now.formatted(.iso8601)), URLQueryItem(name: "bucket", value: "day"),
        ]
        if let metrics = try? await client.get("v1/runs/metrics", query: window, as: RunMetrics.self) {
            failedCounts = RunKinds.countFailures(metrics)
        }
    }

    /// Reads the routes and providers that name each kind's model. Without
    /// them the rows show no model.
    func loadModels(with client: HubClient) async {
        async let loadedRoutes = client.get("v1/task-routes", as: TaskRoutesResponse.self)
        async let loadedProviders = client.get("v1/model-providers", as: ModelProvidersResponse.self)
        if let routes = try? await loadedRoutes.routes, let providers = try? await loadedProviders.providers {
            (self.routes, self.providers) = (routes, providers)
        }
    }

    /// Reads the kinds every few seconds while the page shows: the hub's
    /// events don't carry its queue, so what runs and waits is read again.
    func watch(with client: HubClient, every interval: Duration = .seconds(3)) async {
        while !Task.isCancelled {
            await load(with: client)
            try? await Task.sleep(for: interval)
        }
    }

    /// Pauses or resumes one kind, or, without one, every kind the hub
    /// starts by itself. The hub answers with the kinds as they now are.
    func setPaused(_ paused: Bool, kind: RunKind?, with client: HubClient) async {
        pauseFailure = nil
        let action = paused ? "pause" : "resume"
        let path = kind.map { "v1/run-kinds/\($0.id)/\(action)" } ?? "v1/run-kinds/\(action)-all"
        do {
            apply(try await client.send("POST", path, body: EmptyBody(), as: RunKindsResponse.self))
        } catch {
            let subject = kind?.title ?? "every kind"
            pauseFailure = HubFailure(paused ? "Couldn't pause \(subject)" : "Couldn't resume \(subject)", error)
        }
    }

    private func apply(_ response: RunKindsResponse) {
        groups = response.groups
        kinds = response.kinds
        hasLoaded = true
    }
}

/// The hub's local model work, read every few seconds while a view that
/// shows it is on screen.
@MainActor
@Observable
final class ModelWorkModel {
    private(set) var work: ModelWork?
    /// Why the last read failed; the next read that works clears it.
    var failure: HubFailure?
    /// Why the last pause or resume failed. Reads leave it, so it stays
    /// until the owner dismisses it or tries again.
    var pauseFailure: HubFailure?

    func load(with client: HubClient) async {
        do {
            work = try await client.get("v1/model-work", as: ModelWork.self)
            failure = nil
        } catch {
            failure = HubFailure("Couldn't read the local models' work", error)
        }
    }

    func watch(with client: HubClient, every interval: Duration = .seconds(2)) async {
        while !Task.isCancelled {
            await load(with: client)
            try? await Task.sleep(for: interval)
        }
    }

    /// Pauses or resumes every kind the hub starts by itself.
    func setPaused(_ paused: Bool, with client: HubClient) async {
        pauseFailure = nil
        do {
            work = try await client.send("POST", paused ? "v1/model-work/pause" : "v1/model-work/resume", body: EmptyBody(), as: ModelWork.self)
        } catch {
            pauseFailure = HubFailure(paused ? "Couldn't pause every kind" : "Couldn't resume every kind", error)
        }
    }
}

/// What the hub's models and agents are doing: a column of the kinds of run,
/// each with its state, model and pause, beside the chosen kind's page, its
/// header and its runs. All kinds, first, has them all.
struct ActivityPage: View {
    /// The kind shown, kept when the app reopens; empty for All kinds.
    @AppStorage("activitySelectedKind") private var selectedKindID = ""
    @AppStorage("activityShowsRunKinds") private var showsRunKinds = true
    @AppStorage("activityRunKindsWidth") private var runKindsWidth = 268.0
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @State private var kinds = RunKindsModel()
    @State private var modelWork = ModelWorkModel()
    @State private var runs = ActivityRunsModel()
    @State private var showsOnlyFailed = false
    /// The column steps aside while the inspector shows a run's job or
    /// company, and comes back when it closes; Show run kinds brings it back
    /// sooner.
    @State private var hidesRunKindsForInspector = false

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header
                    // Not a split view: its panes' least widths, added up,
                    // would widen the window when the inspector opens. The
                    // column keeps its width, and the kind's page takes what
                    // is left and gives way to the inspector. Beside the
                    // inspector the column steps aside, and the runs drop
                    // their model, time and usage. Brought back there, the
                    // column takes its least width; the width it was dragged
                    // to comes back when the inspector closes.
                    HStack(spacing: 0) {
                        if isShowingRunKinds {
                            RunKindsColumn(
                                model: kinds, selection: selection, client: client,
                                showOnlyFailed: { kind in
                                    selectedKindID = kind.id
                                    showsOnlyFailed = true
                                }
                            )
                            .frame(width: runKindsColumnWidth)
                            if isInspectorShown {
                                Divider()
                            } else {
                                ColumnResizeHandle(width: $runKindsWidth, range: Self.runKindsWidths)
                                    .zIndex(1)
                            }
                        }
                        kindPage(client)
                            .frame(minWidth: 0, maxWidth: .infinity, minHeight: 0, maxHeight: .infinity, alignment: .topLeading)
                            .clipped()
                    }
                    .clipped()
                }
                // The page tells the window the same sizes whatever it shows,
                // as the inspector does: a frame with every bound set takes
                // the size it's offered. Sizes that followed the run kinds
                // column and the runs' columns, added up, pushed the window's
                // content past its edges beside the inspector.
                .frame(
                    minWidth: 0, idealWidth: Self.idealSize.width, maxWidth: .infinity,
                    minHeight: 0, idealHeight: Self.idealSize.height, maxHeight: .infinity, alignment: .topLeading
                )
                .task(id: events.revision) {
                    await kinds.load(with: client)
                    await kinds.loadModels(with: client)
                }
                .task { await kinds.watch(with: client) }
                .task { await modelWork.watch(with: client, every: .seconds(3)) }
                .onChange(of: isInspectorShown, initial: true) { _, shown in
                    hidesRunKindsForInspector = shown
                }
            }
        }
        .navigationTitle("Activity")
        .navigationSubtitle(selectedKind?.title ?? "All kinds")
    }

    static let runKindsWidths = 220.0...400.0
    /// The size the page asks for: less than a 1400 pt window leaves it
    /// beside the sidebar and the widest inspector, 720 pt.
    static let idealSize = CGSize(width: 400, height: 600)

    private var isInspectorShown: Bool {
        details.getEntry(on: .activity) != nil
    }

    private var isShowingRunKinds: Bool {
        showsRunKinds && !hidesRunKindsForInspector
    }

    private var runKindsColumnWidth: Double {
        isInspectorShown ? Self.runKindsWidths.lowerBound : min(max(runKindsWidth, Self.runKindsWidths.lowerBound), Self.runKindsWidths.upperBound)
    }

    private var runsRoom: ActivityRunsList.Room {
        guard isInspectorShown else { return .full }
        return isShowingRunKinds ? .besideInspectorAndKinds : .besideInspector
    }

    /// The selected kind; nil on All kinds, and while the kinds load.
    private var selectedKind: RunKind? {
        selectedKindID.isEmpty ? nil : kinds.getKind(selectedKindID)
    }

    private var selection: Binding<String?> {
        Binding(get: { selectedKindID }, set: { selectedKindID = $0 ?? "" })
    }

    private var header: some View {
        PageHeader {
            Button(isShowingRunKinds ? "Hide run kinds" : "Show run kinds", systemImage: "sidebar.left") {
                withAnimation {
                    showsRunKinds = !isShowingRunKinds
                    hidesRunKindsForInspector = false
                }
            }
            .labelStyle(.iconOnly)
            .buttonStyle(.borderless)
            .help(isShowingRunKinds ? "Hide the column of run kinds" : "Show the column of run kinds")
        } trailing: {
            Toggle("Only failed runs", isOn: $showsOnlyFailed)
                .toggleStyle(.checkbox)
                .help("List only the runs that failed")
        }
    }

    /// The kind's page, once the kinds are read: a kind kept from before
    /// that the hub no longer has shows All kinds.
    @ViewBuilder
    private func kindPage(_ client: HubClient) -> some View {
        if kinds.hasLoaded {
            VStack(alignment: .leading, spacing: 0) {
                RunKindHeader(model: kinds, kind: selectedKind, work: modelWork.work, client: client)
                Divider()
                ActivityRunsList(
                    model: runs, kind: selectedKind, showsOnlyFailed: showsOnlyFailed, room: runsRoom, client: client
                )
            }
            .onAppear {
                if selectedKind == nil { selectedKindID = "" }
            }
        } else if let failure = kinds.failure {
            HubErrorView(failure, retry: { Task { await kinds.load(with: client) } })
                .padding(Space.xl)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

/// The top of a kind's page: its name, what it does, its state and model,
/// and its pause. All kinds has Pause all and Resume all, and a strip of
/// what runs now, what's next, and what waits behind a pause.
private struct RunKindHeader: View {
    let model: RunKindsModel
    /// Nil for All kinds.
    let kind: RunKind?
    let work: ModelWork?
    let client: HubClient

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text(kind?.title ?? "All kinds").font(.hubEntity).lineLimit(1)
                    .layoutPriority(1)
                let status = kind.map { $0.getStatus(failedCount: model.getFailedCount($0)) } ?? model.all.status
                ToneChip(status.title, tone: status.tone, symbol: status.symbolName)
                if let kind, let kindModel = model.getModel(for: kind) {
                    Label(kindModel.name, systemImage: kindModel.markSymbol)
                        .font(.hubSecondary)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .help("Runs on \(kindModel.name) from \(kindModel.providerName)")
                }
                Spacer(minLength: Space.m)
                controls
            }
            Text(kind?.description ?? "Every kind of run the hub records. Pausing holds only what the hub starts by itself; runs you start always go ahead.")
                .font(.hubSecondary)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if model.pauseFailure != nil {
                HubErrorView(Bindable(model).pauseFailure)
            }
            if kind == nil {
                queueStrip
            }
        }
        .padding(Space.l)
    }

    @ViewBuilder
    private var controls: some View {
        if let kind {
            if kind.isPausable {
                AsyncButton(
                    kind.paused ? "Resume" : "Pause", busyTitle: kind.paused ? "Resuming…" : "Pausing…",
                    systemImage: kind.paused ? "play.fill" : "pause.fill"
                ) {
                    await model.setPaused(!kind.paused, kind: kind, with: client)
                }
                .help(kind.paused ? "Resume \(kind.title)" : "Pause \(kind.title); runs you start still go ahead")
            }
        } else {
            let all = model.all
            AsyncButton("Pause all", busyTitle: "Pausing…", systemImage: "pause.fill") {
                await model.setPaused(true, kind: nil, with: client)
            }
            .disabled(!all.canPauseAll)
            .help("Pause every kind the hub starts by itself; runs you start still go ahead")
            AsyncButton("Resume all", busyTitle: "Resuming…", systemImage: "play.fill") {
                await model.setPaused(false, kind: nil, with: client)
            }
            .disabled(!all.canResumeAll)
            .help("Resume every paused kind")
        }
    }

    private var queueStrip: some View {
        let strip = RunKinds.describeQueue(model.kinds, work: work)
        return FactGrid {
            FactRow("Running now", text: strip.now)
            FactRow("Next", text: strip.next, help: "The first call in the local models' queue that no pause holds")
            FactRow("Held by a pause", text: strip.held, help: "The calls of paused kinds that wait for a resume")
        }
        .font(.hubSecondary)
    }
}

/// The line between the run kinds column and the kind's page: dragged, it
/// sets the column's width within range.
private struct ColumnResizeHandle: View {
    @Binding var width: Double
    let range: ClosedRange<Double>
    /// The width when the drag began.
    @State private var startWidth: Double?

    var body: some View {
        Divider()
            .overlay {
                Color.clear
                    .frame(width: 9)
                    .contentShape(Rectangle())
                    .pointerStyle(.columnResize)
                    .gesture(
                        DragGesture(minimumDistance: 1, coordinateSpace: .global)
                            .onChanged { drag in
                                let start = startWidth ?? width
                                startWidth = start
                                width = min(max(start + drag.translation.width, range.lowerBound), range.upperBound)
                            }
                            .onEnded { _ in startWidth = nil }
                    )
            }
            .accessibilityElement()
            .accessibilityLabel("Run kinds column width")
            .accessibilityValue("\(Int(width)) points")
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: width = min(width + 20, range.upperBound)
                case .decrement: width = max(width - 20, range.lowerBound)
                @unknown default: break
                }
            }
    }
}
