import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class RunsModel {
    static let pageSize = 500

    private(set) var taskRuns: [TaskRun] = []
    private(set) var agentRuns: [AgentRun] = []
    private(set) var failure: HubFailure?

    func load(with client: HubClient) async {
        do {
            let limit = [URLQueryItem(name: "limit", value: String(Self.pageSize))]
            async let tasks = client.get("v1/task-runs", query: limit, as: TaskRunsResponse.self)
            async let agents = client.get("v1/agent-runs", query: limit, as: AgentRunsResponse.self)
            (taskRuns, agentRuns) = try await (tasks.runs, agents.runs)
            failure = nil
        } catch {
            failure = HubFailure("Couldn't load the runs", error)
        }
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

    func setPaused(_ paused: Bool, with client: HubClient) async {
        pauseFailure = nil
        do {
            work = try await client.send("POST", paused ? "v1/model-work/pause" : "v1/model-work/resume", body: EmptyBody(), as: ModelWork.self)
        } catch {
            pauseFailure = HubFailure(paused ? "Couldn't pause the local models" : "Couldn't resume the local models", error)
        }
    }
}

/// What the hub's models and agents are doing: the agent runs going on now,
/// every kind of run with its failures and each model's average time, and
/// the latest runs.
struct ActivityPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @State private var model = RunsModel()
    @State private var modelWork = ModelWorkModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header(client)
                    content(client)
                }
                .task(id: events.revision) { await model.load(with: client) }
                .task { await modelWork.watch(with: client) }
            }
        }
        .navigationTitle("Activity")
        .navigationSubtitle("The latest \(model.taskRuns.count + model.agentRuns.count) runs")
    }

    /// The page's header: the local models' pause, the one thing to do here.
    private func header(_ client: HubClient) -> some View {
        PageHeader {
            EmptyView()
        } trailing: {
            if let work = modelWork.work {
                AsyncButton(
                    work.paused ? "Resume local models" : "Pause local models", busyTitle: work.paused ? "Resuming…" : "Pausing…",
                    systemImage: work.paused ? "play.fill" : "pause.fill"
                ) {
                    await modelWork.setPaused(!work.paused, with: client)
                }
            }
        }
    }

    private func content(_ client: HubClient) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Space.xl) {
                if let failure = model.failure {
                    HubErrorView(failure, retry: { Task { await model.load(with: client) } })
                }
                localModels(client)
                let running = model.agentRuns.filter(\.isRunning)
                if !running.isEmpty {
                    HubSection("Running now") {
                        ForEach(running) { run in
                            HStack(spacing: Space.s) {
                                ProgressView().controlSize(.small)
                                Text(RunsSummary.getKindTitle(run.kind)).fontWeight(.medium)
                                Text(run.input).foregroundStyle(.secondary).lineLimit(1)
                                Spacer()
                                Text(run.startedAt.formatted(.relative(presentation: .named))).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                HubSection("By kind") {
                    let summaries = RunsSummary.summarize(taskRuns: model.taskRuns, agentRuns: model.agentRuns)
                    if summaries.isEmpty {
                        Text("No runs recorded yet.").foregroundStyle(.secondary)
                    }
                    Grid(alignment: .leading, horizontalSpacing: Space.l, verticalSpacing: Space.s) {
                        ForEach(summaries) { summary in
                            GridRow {
                                Text(summary.title).fontWeight(.medium)
                                Text("\(summary.runCount) runs")
                                Text(describeProblems(summary))
                                    .foregroundStyle(summary.failedCount + summary.invalidCount > 0 ? AnyShapeStyle(Tone.negative.color) : AnyShapeStyle(.secondary))
                                Text(summary.models.map { "\($0.model): \(describeSeconds($0.averageSeconds)) average (\($0.runCount))" }.joined(separator: " · "))
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                HubSection("Latest") {
                    ForEach(getLatestRows().prefix(80)) { row in
                        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                            Image(systemName: row.symbolName).foregroundStyle(Tone.ofRunOutcome(row.outcome).color)
                                .accessibilityLabel(row.outcome)
                            Text(RunsSummary.getKindTitle(row.kind)).fontWeight(.medium).frame(width: 190, alignment: .leading)
                            Text(row.detail).foregroundStyle(.secondary).lineLimit(1)
                            Spacer()
                            Text(row.startedAt.formatted(date: .abbreviated, time: .shortened)).foregroundStyle(.secondary)
                        }
                        if let error = row.error, !error.isEmpty {
                            Text(error).font(.hubCaption).foregroundStyle(Tone.negative.color).lineLimit(2).padding(.leading, Space.xl)
                        }
                    }
                }
            }
            .padding(Space.xl)
            .frame(maxWidth: 1000, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    /// The local runtime and its queue: what runs, on which model, what
    /// waits, and whether it's paused. The header has the pause.
    @ViewBuilder
    private func localModels(_ client: HubClient) -> some View {
        HubSection("Local models") {
            if let work = modelWork.work {
                HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                    ToneChip(work.paused ? "Paused" : "Working", tone: work.paused ? .caution : .positive, symbol: work.paused ? "pause.fill" : "play.fill")
                    if work.paused {
                        Text("Only runs you start go ahead.").foregroundStyle(.secondary)
                    }
                }
                if modelWork.pauseFailure != nil {
                    HubErrorView($modelWork.pauseFailure)
                }
                if modelWork.failure != nil {
                    HubErrorView($modelWork.failure)
                }
                FactGrid {
                    FactRow("Model", text: describeRuntime(work.runtime))
                    FactRow("Running") {
                        if let running = work.running {
                            HStack(spacing: Space.s) {
                                ProgressView().controlSize(.small)
                                Text("\(RunsSummary.getKindTitle(running.kind)) on \(running.model), \(running.priority), started \(running.since.formatted(.relative(presentation: .named)))")
                            }
                        } else {
                            Text("Nothing").foregroundStyle(.secondary)
                        }
                    }
                    FactRow("Waiting") {
                        Text(work.waiting.isEmpty ? "Nothing" : work.waitingCountsByKind.map { "\($0.title): \($0.count)" }.joined(separator: ", "))
                            .foregroundStyle(work.waiting.isEmpty ? .secondary : .primary)
                    }
                    FactRow("Job facts", text: work.jobsAwaitingFacts == 1 ? "1 job waits to be read" : "\(work.jobsAwaitingFacts) jobs wait to be read")
                }
            } else if let failure = modelWork.failure {
                HubErrorView(failure, retry: { Task { await modelWork.load(with: client) } })
            } else {
                ProgressView().controlSize(.small)
            }
        }
    }

    private func describeRuntime(_ runtime: ModelRuntimeStatus) -> String {
        switch runtime.state {
        case "loading": "Loading \(runtime.model ?? "a model")…"
        case "ready": "\(runtime.model ?? "A model") loaded" + (runtime.busy ? ", answering" : ", idle")
        default: "None loaded"
        }
    }

    private func describeProblems(_ summary: RunKindSummary) -> String {
        if summary.failedCount + summary.invalidCount == 0 { return "no failures" }
        return [summary.failedCount > 0 ? "\(summary.failedCount) failed" : nil, summary.invalidCount > 0 ? "\(summary.invalidCount) invalid" : nil]
            .compactMap { $0 }.joined(separator: ", ")
    }

    private func describeSeconds(_ seconds: Double) -> String {
        seconds >= 60 ? "\(Int(seconds / 60)) min \(Int(seconds.truncatingRemainder(dividingBy: 60))) s" : String(format: "%.1f s", seconds)
    }

    private struct LatestRow: Identifiable {
        let id: UUID
        let kind: String
        let detail: String
        let startedAt: Date
        let outcome: String
        let error: String?

        var symbolName: String {
            switch outcome {
            case "succeeded": "checkmark.circle.fill"
            case "running": "clock"
            case "invalid": "questionmark.circle.fill"
            default: "xmark.circle.fill"
            }
        }
    }

    private func getLatestRows() -> [LatestRow] {
        let taskRows = model.taskRuns.map { run in
            LatestRow(
                id: run.id, kind: run.kind, detail: "\(run.model) · \(describeSeconds(Double(run.durationMS) / 1000)) · \(run.promptTokens) → \(run.completionTokens) tokens",
                startedAt: run.startedAt, outcome: run.outcome, error: run.error
            )
        }
        let agentRows = model.agentRuns.map { run in
            let took = run.finishedAt.map { describeSeconds($0.timeIntervalSince(run.startedAt)) } ?? "running"
            let cost = run.costUSDEstimate.flatMap(Double.init).map { " · " + $0.formatted(.currency(code: "USD").precision(.fractionLength(2))) } ?? ""
            return LatestRow(id: run.id, kind: run.kind, detail: "\(run.input) · \(took)\(cost)", startedAt: run.startedAt, outcome: run.status, error: run.error)
        }
        return (taskRows + agentRows).sorted { $0.startedAt > $1.startedAt }
    }
}
