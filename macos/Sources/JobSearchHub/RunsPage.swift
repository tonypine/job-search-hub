import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class RunsModel {
    static let pageSize = 500

    private(set) var taskRuns: [TaskRun] = []
    private(set) var agentRuns: [AgentRun] = []
    private(set) var errorMessage: String?

    func load(with client: HubClient) async {
        do {
            let limit = [URLQueryItem(name: "limit", value: String(Self.pageSize))]
            async let tasks = client.get("v1/task-runs", query: limit, as: TaskRunsResponse.self)
            async let agents = client.get("v1/agent-runs", query: limit, as: AgentRunsResponse.self)
            (taskRuns, agentRuns) = try await (tasks.runs, agents.runs)
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }
}

/// What the hub's models and agents are doing: the agent runs going on now,
/// every kind of run with its failures and each model's average time, and
/// the latest runs.
struct RunsPage: View {
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @State private var model = RunsModel()

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                content
                    .task(id: events.revision) { await model.load(with: client) }
                    .toolbar {
                        Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                    }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Runs")
        .navigationSubtitle("The latest \(model.taskRuns.count + model.agentRuns.count) runs")
    }

    private var content: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                if let errorMessage = model.errorMessage {
                    Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                }
                let running = model.agentRuns.filter(\.isRunning)
                if !running.isEmpty {
                    section("Running now") {
                        ForEach(running) { run in
                            HStack(spacing: 8) {
                                ProgressView().controlSize(.small)
                                Text(RunsSummary.getKindTitle(run.kind)).fontWeight(.medium)
                                Text(run.input).foregroundStyle(.secondary).lineLimit(1)
                                Spacer()
                                Text(run.startedAt.formatted(.relative(presentation: .named))).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                section("By kind") {
                    let summaries = RunsSummary.summarize(taskRuns: model.taskRuns, agentRuns: model.agentRuns)
                    if summaries.isEmpty {
                        Text("No runs recorded yet.").foregroundStyle(.secondary)
                    }
                    Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 8) {
                        ForEach(summaries) { summary in
                            GridRow {
                                Text(summary.title).fontWeight(.medium)
                                Text("\(summary.runCount) runs")
                                Text(describeProblems(summary)).foregroundStyle(summary.failedCount + summary.invalidCount > 0 ? .orange : .secondary)
                                Text(summary.models.map { "\($0.model): \(describeSeconds($0.averageSeconds)) average (\($0.runCount))" }.joined(separator: " · "))
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
                section("Latest") {
                    ForEach(getLatestRows().prefix(80)) { row in
                        HStack(alignment: .firstTextBaseline, spacing: 10) {
                            Image(systemName: row.symbolName).foregroundStyle(row.color)
                            Text(RunsSummary.getKindTitle(row.kind)).fontWeight(.medium).frame(width: 190, alignment: .leading)
                            Text(row.detail).foregroundStyle(.secondary).lineLimit(1)
                            Spacer()
                            Text(row.startedAt.formatted(date: .abbreviated, time: .shortened)).foregroundStyle(.secondary)
                        }
                        if let error = row.error, !error.isEmpty {
                            Text(error).font(.caption).foregroundStyle(.orange).lineLimit(2).padding(.leading, 28)
                        }
                    }
                }
            }
            .padding(24)
            .frame(maxWidth: 1000, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func section(_ title: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title).font(.title3.weight(.semibold))
            content()
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

        var color: Color {
            switch outcome {
            case "succeeded": .green
            case "running": .blue
            default: .orange
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
