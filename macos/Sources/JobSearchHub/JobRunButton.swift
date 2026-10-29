import JobSearchHubCore
import SwiftUI

/// Run ▾ in a job's action bar: a popover with a row for each kind of run
/// the hub can start for the job, each started with its ▶.
struct JobRunButton: View {
    let jobID: UUID
    let jobTitle: String
    let client: HubClient
    @Environment(JobRunStarts.self) private var starts
    @State private var isShowingRuns = false
    /// The kind whose note the Fix sheet asks for.
    @State private var askingNoteFor: RunnableKind?

    var body: some View {
        Button {
            isShowingRuns.toggle()
        } label: {
            HStack(spacing: Space.xs) {
                Label("Run", systemImage: "play.circle")
                Image(systemName: "chevron.down").font(.caption2.weight(.semibold))
            }
        }
        .buttonStyle(.bordered)
        .tint(.hubAccent)
        .help("Start any of the job's runs now, whatever their pause")
        .popover(isPresented: $isShowingRuns, arrowEdge: .bottom) {
            JobRunPopover(jobID: jobID, client: client, starts: starts) { kind in
                isShowingRuns = false
                askingNoteFor = kind
            }
        }
        .sheet(item: $askingNoteFor) { kind in
            FixJobSheet(jobTitle: jobTitle) { note in
                do {
                    try await starts.start(kind, for: jobID, note: note, with: client)
                    return nil
                } catch {
                    return HubFailure("Couldn't ask for the fix", error)
                }
            }
        }
    }
}

/// The job's kinds of run, in the hub's order, each a row of its name and
/// last run, its model and ▶; the fix of the job's details last, under a
/// separator. Only ▶ starts a run. No prices or tokens show here.
struct JobRunPopover: View {
    let jobID: UUID
    let client: HubClient
    /// Handed over rather than read from the environment, which a popover's
    /// own window may not carry.
    let starts: JobRunStarts
    /// Asks for the note a kind needs before it runs.
    let askForNote: (RunnableKind) -> Void
    @State private var kinds: [RunnableKind]?
    @State private var providers: [ModelProvider] = []
    @State private var starting: Set<String> = []
    @State private var failure: HubFailure?

    private static let modelWidth: CGFloat = 150

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let kinds {
                let rows = JobRuns.split(kinds)
                ForEach(rows.runs) { row($0) }
                if !rows.withNote.isEmpty {
                    Divider().padding(.vertical, Space.xs)
                    ForEach(rows.withNote) { row($0) }
                }
                if kinds.isEmpty {
                    Text("No runs to start for this job.").foregroundStyle(.secondary).padding(Space.s)
                }
            } else if failure == nil {
                ProgressView().controlSize(.small).frame(maxWidth: .infinity).padding(Space.l)
            }
            if failure != nil {
                HubErrorView($failure).padding(.top, Space.s)
            }
        }
        .padding(Space.s)
        .frame(width: 420)
        .task(id: starts.revision) { await load() }
    }

    private func load() async {
        async let listed = client.listRunnableKinds(forJob: jobID)
        async let readProviders = try? client.get("v1/model-providers", as: ModelProvidersResponse.self).providers
        do {
            kinds = try await listed
            failure = nil
        } catch {
            failure = HubFailure("Couldn't read the job's runs", error)
        }
        if let read = await readProviders {
            providers = read
        }
    }

    private func row(_ kind: RunnableKind) -> some View {
        let title = JobRuns.getTitle(of: kind)
        let running = starts.getRunning(kind.id, of: jobID)
        return HStack(spacing: Space.m) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).fontWeight(.medium).lineLimit(1)
                stateLine(kind, running: running)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Group {
                if let model = JobRuns.getModel(of: kind, providers: providers) {
                    Label(model.name, systemImage: model.markSymbol)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .help(model.runsOnText)
                } else {
                    Text("No model").help("No model is routed for it yet; choose one in Activity")
                }
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
            .frame(width: Self.modelWidth, alignment: .leading)
            Button {
                start(kind)
            } label: {
                Image(systemName: "play.fill")
            }
            .buttonStyle(.bordered)
            .buttonBorderShape(.circle)
            .tint(.hubAccent)
            .disabled(!kind.canRun || running != nil || starting.contains(kind.id))
            .help(JobRuns.describeStart(of: kind))
            .accessibilityLabel("Start \(title)")
        }
        .padding(.vertical, Space.xs)
        .padding(.horizontal, Space.s)
        .accessibilityElement(children: .contain)
    }

    /// "Running 0:42" while a run started here goes, else the last run or
    /// why it can't run yet.
    @ViewBuilder
    private func stateLine(_ kind: RunnableKind, running: JobRunStarts.Running?) -> some View {
        if let running {
            TimelineView(.periodic(from: running.startedAt, by: 1)) { context in
                Text(JobRuns.describeRunning(since: running.startedAt, now: context.date))
                    .monospacedDigit()
            }
            .font(.hubCaption)
            .foregroundStyle(Color.hubAccent)
        } else {
            Text(JobRuns.describeState(of: kind, now: .now))
                .font(.hubCaption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
    }

    private func start(_ kind: RunnableKind) {
        if kind.needsNote {
            askForNote(kind)
            return
        }
        starting.insert(kind.id)
        Task {
            defer { starting.remove(kind.id) }
            do {
                try await starts.start(kind, for: jobID, with: client)
            } catch {
                failure = HubFailure("Couldn't start \(JobRuns.getTitle(of: kind))", error)
            }
        }
    }
}
