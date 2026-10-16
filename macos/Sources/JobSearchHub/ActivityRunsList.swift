import JobSearchHubCore
import SwiftUI

/// The runs of a kind, or of every kind, the latest first, a page at a time.
@MainActor
@Observable
final class ActivityRunsModel {
    static let pageSize = 50
    /// The most runs the hub gives in one page.
    static let largestPage = 200

    /// The runs listed: of one kind, or of every kind without one, and only
    /// the failed ones, or all.
    struct Scope: Equatable {
        var kind: String?
        var failedOnly: Bool
    }

    private(set) var runs: [ActivityRun] = []
    private(set) var nextCursor = ""
    /// The scope the runs are of; nil before the first read.
    private(set) var scope: Scope?
    private(set) var isLoadingMore = false
    var failure: HubFailure?

    var hasMore: Bool { !nextCursor.isEmpty }

    /// Reads the latest runs of scope again. Read again for the same scope,
    /// as on a hub event, it keeps as many runs as were shown, up to the
    /// hub's largest page.
    func load(_ scope: Scope, with client: HubClient) async {
        let limit = self.scope == scope ? min(max(runs.count, Self.pageSize), Self.largestPage) : Self.pageSize
        do {
            let page = try await client.get("v1/runs", query: makeQuery(scope, limit: limit, cursor: nil), as: ActivityRunsPage.self)
            (runs, nextCursor, self.scope) = (page.runs, page.nextCursor, scope)
            failure = nil
        } catch {
            // A read the page cancelled, for another kind, isn't a failure.
            if !Task.isCancelled {
                failure = HubFailure("Couldn't load the runs", error)
            }
        }
    }

    /// Reads the page after the last run shown.
    func loadMore(with client: HubClient) async {
        guard let scope, hasMore, !isLoadingMore else { return }
        isLoadingMore = true
        defer { isLoadingMore = false }
        do {
            let page = try await client.get("v1/runs", query: makeQuery(scope, limit: Self.pageSize, cursor: nextCursor), as: ActivityRunsPage.self)
            guard scope == self.scope else { return }
            let shown = Set(runs.map(\.id))
            runs += page.runs.filter { !shown.contains($0.id) }
            nextCursor = page.nextCursor
            failure = nil
        } catch {
            failure = HubFailure("Couldn't load more runs", error)
        }
    }

    private func makeQuery(_ scope: Scope, limit: Int, cursor: String?) -> [URLQueryItem] {
        var query = [URLQueryItem(name: "limit", value: String(limit))]
        if let kind = scope.kind {
            query.append(URLQueryItem(name: "kind", value: kind))
        }
        if scope.failedOnly {
            query.append(URLQueryItem(name: "outcome", value: "failed"))
        }
        if let cursor {
            query.append(URLQueryItem(name: "cursor", value: cursor))
        }
        return query
    }
}

/// A kind's runs, or every kind's on All kinds, read again on each hub
/// event. A run's job and company open in the inspector; a failed run,
/// selected, shows its error and what to open under the table.
struct ActivityRunsList: View {
    let model: ActivityRunsModel
    /// Nil for All kinds, whose rows name each run's kind.
    let kind: RunKind?
    let showsOnlyFailed: Bool
    let client: HubClient
    @Environment(HubEventStream.self) private var events
    @Environment(DetailsInspector.self) private var details
    @State private var selectedRunID: UUID?
    @State private var promptRun: ActivityRun?

    private var scope: ActivityRunsModel.Scope {
        ActivityRunsModel.Scope(kind: kind?.id, failedOnly: showsOnlyFailed)
    }

    var body: some View {
        Group {
            if model.scope != scope {
                if model.failure == nil {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    Color.clear
                }
            } else if model.runs.isEmpty {
                ContentUnavailableView(
                    showsOnlyFailed ? "No failed runs" : "No runs yet", systemImage: showsOnlyFailed ? "checkmark.circle" : "clock",
                    description: Text(kind.map { "Runs of \($0.title) show here as they start." } ?? "Every run the hub records shows here as it starts.")
                )
            } else {
                list
            }
        }
        .overlay(alignment: .bottom) {
            if model.failure != nil {
                HubErrorView(Bindable(model).failure, retry: { Task { await model.load(scope, with: client) } })
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
        .task(id: ReloadKey(scope: scope, revision: events.revision)) {
            await model.load(scope, with: client)
        }
        .sheet(item: $promptRun) { run in
            RunPromptSheet(run: run, client: client)
        }
    }

    /// The runs as a table, whose columns give way to the inspector rather
    /// than widen the window, with the selected failed run under it.
    private var list: some View {
        VStack(spacing: 0) {
            Table(model.runs, selection: $selectedRunID) {
                TableColumn("") { run in
                    Image(systemName: run.outcomeSymbol)
                        .foregroundStyle(Tone.ofRunOutcome(run.outcome).color)
                        .help(run.outcome)
                        .accessibilityLabel(run.outcome)
                }
                .width(18)
                if kind == nil {
                    TableColumn("Kind") { run in
                        Text(RunsSummary.getKindTitle(run.kind)).lineLimit(1)
                    }
                    .width(min: 80, ideal: 130)
                }
                TableColumn("Ran for") { run in
                    RunSubjectCell(run: run) { subject in details.show(subject, from: .activity) }
                }
                .width(min: 120, ideal: 320)
                TableColumn("Model") { run in
                    Text(run.modelText).foregroundStyle(.secondary).lineLimit(1)
                        .help(run.provider.isEmpty ? run.modelText : "\(run.modelText) from \(run.provider)")
                }
                .width(min: 60, ideal: 130)
                TableColumn("Took") { run in
                    Text(run.durationText).monospacedDigit().foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .trailing)
                }
                .width(min: 44, ideal: 60)
                TableColumn("Tokens or cost") { run in
                    Text(run.usageText ?? "").monospacedDigit().foregroundStyle(.secondary).lineLimit(1)
                        .frame(maxWidth: .infinity, alignment: .trailing)
                        .help(run.usageText ?? "")
                }
                .width(min: 60, ideal: 130)
                TableColumn("Started") { run in
                    Text(run.startedAt.formatted(Self.startFormat)).foregroundStyle(.secondary).lineLimit(1)
                        .help(run.startedAt.formatted(date: .complete, time: .standard))
                }
                .width(min: 80, ideal: 120)
            }
            if let run = selectedFailedRun {
                Divider()
                FailedRunDetail(
                    run: run, open: { subject in details.show(subject, from: .activity) },
                    showPrompt: { promptRun = run }
                )
            }
            if model.hasMore {
                Divider()
                AsyncButton("Show More Runs", busyTitle: "Loading…", isBusy: model.isLoadingMore) {
                    await model.loadMore(with: client)
                }
                .controlSize(.small)
                .frame(maxWidth: .infinity)
                .padding(.vertical, Space.s)
            }
        }
    }

    /// A run's start: the day and the time, short enough for its column.
    private static let startFormat = Date.FormatStyle.dateTime.month(.abbreviated).day().hour().minute()

    private var selectedFailedRun: ActivityRun? {
        guard let selectedRunID, let run = model.runs.first(where: { $0.id == selectedRunID }), run.isFailed else { return nil }
        return run
    }

    /// Keys the read: again for another kind or filter, and on each update
    /// the hub announces.
    private struct ReloadKey: Equatable {
        let scope: ActivityRunsModel.Scope
        let revision: Int
    }
}

/// What a run was for: each part that opens something is a link, and a
/// chip when you started the run.
private struct RunSubjectCell: View {
    let run: ActivityRun
    let open: (InspectorSubject) -> Void

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
            let parts = run.subjectParts
            if parts.isEmpty {
                Text("—").foregroundStyle(.secondary)
            }
            ForEach(Array(parts.enumerated()), id: \.offset) { index, part in
                if index > 0 {
                    Text("·").foregroundStyle(.secondary)
                }
                if let opens = part.opens {
                    Button(part.text) { open(opens) }
                        .buttonStyle(.link)
                        .lineLimit(1)
                        .truncationMode(.tail)
                        .help(Self.describeOpening(opens, part.text))
                        // The job gives way before the company.
                        .layoutPriority(index == 0 ? 0 : 1)
                } else {
                    Text(part.text).lineLimit(1)
                }
            }
            if run.startedByOwner {
                ToneChip("You", tone: .accent)
                    .help("You started this run")
                    .layoutPriority(2)
            }
        }
    }

    static func describeOpening(_ subject: InspectorSubject, _ text: String) -> String {
        switch subject {
        case .job: "Open the job \(text)"
        case .company: "Open the company \(text)"
        default: "Open \(text)"
        }
    }
}

/// A failed run, selected: what it was for, its error, and what to open.
private struct FailedRunDetail: View {
    let run: ActivityRun
    let open: (InspectorSubject) -> Void
    let showPrompt: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Image(systemName: run.outcomeSymbol)
                    .foregroundStyle(Tone.ofRunOutcome(run.outcome).color)
                Text("\(RunsSummary.getKindTitle(run.kind)) failed").font(.hubSecondary.weight(.semibold))
                if let subject = run.subjectText {
                    Text(subject).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            ScrollView {
                Text(describeError())
                    .font(.hubSecondary)
                    .foregroundStyle(Tone.negative.color)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(maxHeight: 120)
            .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: Space.s) {
                if let jobID = run.jobID {
                    Button("Open Job", systemImage: "briefcase") { open(.job(jobID)) }
                }
                if let companyID = run.companyID {
                    Button("Open Company", systemImage: "building.2") { open(.company(companyID)) }
                }
                if run.isAgentRun {
                    Button("Show Prompt and Answer", systemImage: "text.bubble") { showPrompt() }
                        .help("Show what the agent was asked and what it answered")
                }
            }
            .controlSize(.small)
        }
        .padding(Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func describeError() -> String {
        if let error = run.error, !error.isEmpty { return error }
        return run.outcome == "invalid" ? "The model answered, but not in the shape asked for." : "The hub recorded no error."
    }
}

/// What a Claude agent run was asked, and what it answered or why it
/// failed.
private struct RunPromptSheet: View {
    let run: ActivityRun
    let client: HubClient
    @Environment(\.dismiss) private var dismiss
    @State private var detail: AgentRunDetail?
    @State private var failure: HubFailure?

    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("\(RunsSummary.getKindTitle(run.kind)): prompt and answer").font(.hubSection)
            ScrollView {
                VStack(alignment: .leading, spacing: Space.l) {
                    if let detail {
                        part("Asked", detail.input.isEmpty ? "Nothing recorded" : detail.input)
                        part("Answered", detail.resultText ?? "No answer recorded")
                        if let error = detail.error, !error.isEmpty {
                            part("Error", error)
                        }
                    } else if let failure {
                        HubErrorView(failure)
                    } else {
                        ProgressView()
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            HStack {
                Spacer()
                Button("Done") { dismiss() }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(Space.l)
        .frame(width: 640, height: 520)
        .task {
            do {
                detail = try await client.get("v1/agent-runs/\(run.id.uuidString)", as: AgentRunDetailResponse.self).agentRun
            } catch {
                failure = HubFailure("Couldn't read the run", error)
            }
        }
    }

    private func part(_ title: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            Text(title).font(.hubSecondary).foregroundStyle(.secondary)
            Text(text)
                .font(.system(.body, design: .monospaced))
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}
