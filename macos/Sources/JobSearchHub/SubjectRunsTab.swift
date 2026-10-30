import JobSearchHubCore
import SwiftUI

/// A run started from a job's Run popover that the hub doesn't list yet:
/// it waits in the queue, or its record isn't written.
struct UnlistedStart: Equatable, Identifiable {
    /// The id the run is recorded under once it runs.
    let id: UUID
    let kind: String
    let startedAt: Date
}

/// A row of a Runs tab: a run the hub lists, or one started here that it
/// doesn't list yet, which says "Running" and "You started".
struct SubjectRunItem: Equatable, Identifiable {
    /// The run's id, whichever it comes from, so a start's row turns into
    /// its run's in place once the hub lists it.
    let id: UUID
    let kind: String
    let outcome: String
    let startedAt: Date
    /// Nil while the hub hasn't listed the run.
    let model: String?
    let startedByOwner: Bool
    /// How a finished run ended: "Succeeded in 4.2 s".
    let detail: String?
    /// A failed run's error, as its outcome's tooltip.
    let error: String?

    init(run: ActivityRun) {
        id = run.id
        kind = run.kind
        outcome = run.outcome
        startedAt = run.startedAt
        model = run.modelText
        startedByOwner = run.startedByOwner
        detail = SubjectRuns.describeOutcome(run)
        error = run.isFailed ? run.error : nil
    }

    init(start: UnlistedStart) {
        id = start.id
        kind = start.kind
        outcome = "running"
        startedAt = start.startedAt
        model = nil
        startedByOwner = true
        detail = nil
        error = nil
    }
}

/// Every run for a job or a company, the latest first, a page at a time.
@MainActor
@Observable
final class SubjectRunsModel {
    struct Subject: Hashable {
        let type: String
        let id: UUID

        static func job(_ id: UUID) -> Subject { Subject(type: SubjectRuns.jobSubject, id: id) }
        static func company(_ id: UUID) -> Subject { Subject(type: SubjectRuns.companySubject, id: id) }
    }

    private(set) var runs: [ActivityRun] = []
    private(set) var nextCursor = ""
    /// The subject the runs are for; nil before the first read.
    private(set) var subject: Subject?
    var failure: HubFailure?

    var hasMore: Bool { !nextCursor.isEmpty }
    var hasRunning: Bool { runs.contains { $0.outcome == "running" } }

    /// Reads the subject's latest runs. Read again for the same subject, as
    /// a run goes or lands, it keeps every run loaded: the latest page, up
    /// to the hub's largest, then those loaded past it.
    func load(_ subject: Subject, with client: HubClient) async {
        let limit = self.subject == subject ? min(max(runs.count, SubjectRuns.pageSize), SubjectRuns.largestPage) : SubjectRuns.pageSize
        do {
            let query = SubjectRuns.makeQuery(subjectType: subject.type, subjectID: subject.id, limit: limit)
            let page = try await client.get("v1/runs", query: query, as: ActivityRunsPage.self)
            let listed = self.subject == subject ? SubjectRuns.merge(page, into: runs, cursor: nextCursor) : page
            (runs, nextCursor, self.subject) = (listed.runs, listed.nextCursor, subject)
            failure = nil
        } catch {
            // A read the tab cancelled, for another subject, isn't a failure.
            if !Task.isCancelled {
                failure = HubFailure("Couldn't load the runs", error)
            }
        }
    }

    /// Reads the page after the last run loaded. A read for a cursor the
    /// list has moved past, as another page landed or a read again started
    /// the list over, is dropped, so a read in flight never holds up the
    /// next one.
    func loadMore(with client: HubClient) async {
        guard let subject, hasMore else { return }
        let cursor = nextCursor
        do {
            let query = SubjectRuns.makeQuery(subjectType: subject.type, subjectID: subject.id, limit: SubjectRuns.pageSize, cursor: cursor)
            let page = try await client.get("v1/runs", query: query, as: ActivityRunsPage.self)
            guard subject == self.subject, cursor == nextCursor else { return }
            let shown = Set(runs.map(\.id))
            runs += page.runs.filter { !shown.contains($0.id) }
            nextCursor = page.nextCursor
            failure = nil
        } catch {
            // A read the tab cancelled, as the end scrolled away or the
            // list started over, isn't a failure.
            if !Task.isCancelled {
                failure = HubFailure("Couldn't load more runs", error)
            }
        }
    }

    /// The runs started here that the runs read don't hold yet, the latest
    /// first.
    func getUnlisted(_ starts: [UnlistedStart]) -> [UnlistedStart] {
        guard subject != nil else { return [] }
        let listed = Set(runs.map(\.id))
        return starts.filter { !listed.contains($0.id) }.sorted { $0.startedAt > $1.startedAt }
    }

    /// The tab's rows: the runs started here that the runs read don't hold
    /// yet, then the runs read. Each run is one row, under its id.
    func getRows(_ starts: [UnlistedStart]) -> [SubjectRunItem] {
        getUnlisted(starts).map(SubjectRunItem.init(start:)) + runs.map(SubjectRunItem.init(run:))
    }
}

/// A job's or a company's Runs tab: every run for it, the latest first, the
/// next page loading as the end scrolls in. A run started from the job's
/// Run popover shows at the top at once, and the tab reads again every few
/// seconds while anything for the subject runs.
struct SubjectRunsTab: View {
    let subject: SubjectRunsModel.Subject
    let client: HubClient
    /// Bumps as something run for the subject here ends: a company's
    /// research or job finding.
    var revision = 0
    /// Something for the subject runs here, so the tab reads again until it
    /// ends.
    var isRunningHere = false
    /// A page scrolls the runs itself; the inspector's tab sits in its
    /// scroll view.
    var layout = JobDetailLayout.inspector
    @Environment(HubEventStream.self) private var events
    @Environment(JobRunStarts.self) private var starts
    @State private var model = SubjectRunsModel()

    static let pollInterval: Duration = .seconds(3)

    var body: some View {
        switch layout {
        case .inspector:
            content
        case .page:
            ScrollView {
                content
                    .frame(minWidth: 0, maxWidth: 720, alignment: .topLeading)
                    .padding(Space.xl)
                    .frame(maxWidth: .infinity, alignment: .topLeading)
            }
        }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            if model.subject != subject {
                if model.failure == nil {
                    ProgressView().controlSize(.small)
                }
            } else {
                let rows = model.getRows(startedHere)
                if rows.isEmpty {
                    Text(emptyText).foregroundStyle(.secondary)
                } else {
                    SubjectRunsList(rows: rows, nextCursor: model.nextCursor, hasFailed: model.failure != nil) {
                        await model.loadMore(with: client)
                    }
                }
            }
            if model.failure != nil {
                HubErrorView(Bindable(model).failure, retry: { Task { await model.load(subject, with: client) } })
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .task(id: ReloadKey(subject: subject, events: events.revision, starts: starts.revision, revision: revision, startedHere: Set(startedHere.map(\.id)), isRunningHere: isRunningHere)) {
            await model.load(subject, with: client)
            while !Task.isCancelled, isWatching {
                try? await Task.sleep(for: Self.pollInterval)
                guard !Task.isCancelled else { return }
                await model.load(subject, with: client)
            }
        }
    }

    private var emptyText: String {
        subject.type == SubjectRuns.jobSubject
            ? "No runs for this job yet. Start one from Run."
            : "No runs for this company yet. Find jobs or research it to start one."
    }

    /// The runs started for the job from its Run popover that still go.
    private var startedHere: [UnlistedStart] {
        guard subject.type == SubjectRuns.jobSubject else { return [] }
        return starts.getRunning(of: subject.id).map { UnlistedStart(id: $0.running.runID, kind: $0.kind, startedAt: $0.running.startedAt) }
    }

    /// Something for the subject still runs: a run listed as running, or
    /// one started here.
    private var isWatching: Bool {
        isRunningHere || model.hasRunning || !startedHere.isEmpty
    }

    /// Keys the read: again for another subject, on each update the hub
    /// announces, and as a run starts or lands here.
    private struct ReloadKey: Equatable {
        let subject: SubjectRunsModel.Subject
        let events: Int
        let starts: Int
        let revision: Int
        let startedHere: Set<UUID>
        let isRunningHere: Bool
    }
}

/// A Runs tab's rows and, while older runs remain, the row at the end that
/// reads the next page. One list, each run once under its id: drawn as a
/// list of starts and a list of runs keyed by the same ids, the lazy stack
/// kept a start's "Running" row after its run had moved to the other list.
struct SubjectRunsList: View {
    let rows: [SubjectRunItem]
    /// The page after the last row; empty once every run is loaded.
    var nextCursor = ""
    /// A read of a page failed, and the tab still says so.
    var hasFailed = false
    var loadMore: @MainActor () async -> Void = {}

    var body: some View {
        // Lazy, so the row at the end appears, and reads the next page,
        // only once it scrolls in.
        LazyVStack(alignment: .leading, spacing: 0) {
            ForEach(rows) { row in
                SubjectRunRow(item: row)
                if row.id != rows.last?.id || !nextCursor.isEmpty {
                    Divider()
                }
            }
            if !nextCursor.isEmpty {
                ProgressView()
                    .controlSize(.small)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, Space.s)
                    // Again for each page read while the end still shows,
                    // and once a failure to read one is dismissed or retried.
                    .task(id: PageKey(cursor: nextCursor, hasFailed: hasFailed)) {
                        guard !hasFailed else { return }
                        await loadMore()
                    }
            }
        }
    }

    private struct PageKey: Equatable {
        let cursor: String
        let hasFailed: Bool
    }
}

/// A run in a Runs tab: Activity's run row without what it ran for. Its
/// outcome, its kind and "You started" when you did, how it ended or
/// "Running" with its timer, when, and its model.
struct SubjectRunRow: View {
    let item: SubjectRunItem

    private static let startFormat = Date.FormatStyle.dateTime.month(.abbreviated).day().hour().minute()
    static let trailingWidth: CGFloat = 140

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.s) {
            Image(systemName: ActivityRun.getOutcomeSymbol(item.outcome))
                .foregroundStyle(Tone.ofRunOutcome(item.outcome).color)
                .help(item.outcome)
                .accessibilityLabel(item.outcome)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                    Text(SubjectRuns.getTitle(ofKind: item.kind)).fontWeight(.medium).lineLimit(1)
                    if item.startedByOwner {
                        ToneChip("You started", tone: .accent)
                            .fixedSize()
                            .help("You started this run")
                    }
                }
                stateLine
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            // As wide as "Oct 19, 4:42 PM" and a short model's name; a
            // longer name is cut in its middle.
            VStack(alignment: .trailing, spacing: 2) {
                Text(item.startedAt.formatted(Self.startFormat)).lineLimit(1)
                    .help(item.startedAt.formatted(date: .complete, time: .standard))
                if let model = item.model {
                    Text(model).lineLimit(1).truncationMode(.middle)
                        .help(model)
                }
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
            .frame(maxWidth: Self.trailingWidth, alignment: .trailing)
        }
        .padding(.vertical, Space.s)
        .accessibilityElement(children: .combine)
    }

    /// "Running 0:42" while it goes, as in the Run popover; else how it
    /// ended.
    @ViewBuilder
    private var stateLine: some View {
        if item.outcome == "running" {
            TimelineView(.periodic(from: item.startedAt, by: 1)) { context in
                Text(JobRuns.describeRunning(since: item.startedAt, now: context.date))
                    .monospacedDigit()
            }
            .font(.hubCaption)
            .foregroundStyle(Color.hubAccent)
        } else if let detail = item.detail {
            Text(detail)
                .font(.hubCaption)
                .foregroundStyle(item.error == nil ? Color.secondary : Tone.negative.color)
                .help(item.error ?? "")
        }
    }
}
