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
    /// The run whose note the Fix sheet asks for.
    @State private var askingNoteFor: NoteRequest?
    /// The run whose note is asked for once the popover has closed.
    @State private var noteNext: NoteRequest?
    /// Why the last start failed, kept here so it still shows once the
    /// popover closes and opens again.
    @State private var startFailure: HubFailure?

    var body: some View {
        RunPopoverButton(isShowingRuns: $isShowingRuns, help: "Start any of the job's runs now, whatever their pause") {
            RunPopover(subject: .job(jobID), client: client, starts: starts, startFailure: $startFailure) { kind, model in
                noteNext = NoteRequest(kind: kind, model: model)
                isShowingRuns = false
            }
        }
        // The note's sheet opens once the popover has gone: the window shows
        // one of them at a time.
        .onChange(of: isShowingRuns) {
            if !isShowingRuns, let request = noteNext {
                noteNext = nil
                askingNoteFor = request
            }
        }
        .sheet(item: $askingNoteFor) { request in
            FixJobSheet(jobTitle: jobTitle) { note in
                do {
                    try await starts.start(request.kind, for: jobID, note: note, model: request.model, with: client)
                    return nil
                } catch {
                    return HubFailure("Couldn't ask for the fix", error)
                }
            }
        }
    }
}

/// Run ▾ in a company's action bar: a popover with a row for each kind of
/// run the hub can start for the company, its research and find jobs, each
/// started with its ▶.
struct CompanyRunButton: View {
    let companyID: UUID
    let client: HubClient
    @Environment(JobRunStarts.self) private var starts
    @State private var isShowingRuns = false
    /// Why the last start failed, kept here so it still shows once the
    /// popover closes and opens again.
    @State private var startFailure: HubFailure?

    var body: some View {
        RunPopoverButton(isShowingRuns: $isShowingRuns, help: "Research the company or find its jobs now, whatever their pause") {
            RunPopover(subject: .company(companyID), client: client, starts: starts, startFailure: $startFailure)
        }
    }
}

/// The Run ▾ button of an action bar, which opens its popover.
struct RunPopoverButton<Popover: View>: View {
    @Binding var isShowingRuns: Bool
    let help: String
    @ViewBuilder let popover: () -> Popover

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
        .help(help)
        .popover(isPresented: $isShowingRuns, arrowEdge: .bottom, content: popover)
    }
}

/// A run that asks for its note before it starts, on the model picked for
/// it, nil for its kind's.
private struct NoteRequest: Identifiable {
    let kind: RunnableKind
    let model: RoutedModel?

    var id: String { kind.id }
}

/// The job's or the company's kinds of run, in the hub's order, each a row
/// of its name and last run, its model select and ▶; the fix of the job's
/// details last, under a separator. Only ▶ starts a run, on the model its
/// select shows. Each time the popover opens, every select is on its kind's
/// model. No prices or tokens show here.
struct RunPopover: View {
    let subject: SubjectRunsModel.Subject
    let client: HubClient
    /// Handed over rather than read from the environment, which a popover's
    /// own window may not carry.
    let starts: JobRunStarts
    @Binding var startFailure: HubFailure?
    /// Asks for the note a kind needs before it runs on the model picked; a
    /// company's kinds take none.
    var askForNote: (RunnableKind, RoutedModel?) -> Void = { _, _ in }
    @State private var kinds: [RunnableKind]?
    @State private var choices = ModelChoicesModel()
    /// Each kind's select, by kind, once it is set; the rest show their
    /// kind's model.
    @State private var selections: [String: RunModelSelection] = [:]
    @State private var loadFailure: HubFailure?

    /// Wide enough that a second line as long as "Needs the tailored CV
    /// first · kind paused" reads on one line beside the model and ▶.
    static let width: CGFloat = 460
    /// What a row has once the popover's own padding is taken.
    static let rowWidth = width - 2 * Space.s

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
                    Text("No runs to start for this \(subject.type).").foregroundStyle(.secondary).padding(Space.s)
                }
                if loadFailure != nil {
                    HubErrorView($loadFailure).padding(.top, Space.s)
                }
            } else if let loadFailure {
                HubErrorView(loadFailure, retry: { Task { await load() } })
            } else {
                ProgressView().controlSize(.small).frame(maxWidth: .infinity).padding(Space.l)
            }
            if startFailure != nil {
                HubErrorView($startFailure).padding(.top, Space.s)
            }
        }
        .padding(Space.s)
        .frame(width: Self.width)
        .onAppear { selections = [:] }
        .task(id: starts.revision) { await load() }
        // The selects' labels need the providers alone; a select reads the
        // models once it opens.
        .task { await choices.loadProviders(with: client) }
    }

    private func load() async {
        do {
            kinds = try await client.listRunnableKinds(subjectType: subject.type, id: subject.id)
            loadFailure = nil
        } catch {
            loadFailure = HubFailure("Couldn't read the \(subject.type)'s runs", error)
        }
    }

    /// The kind's select: as set in this popover, else on the kind's model.
    private func getSelection(_ kind: RunnableKind) -> Binding<RunModelSelection> {
        Binding(
            get: { selections[kind.id] ?? RunModelSelection(kind: kind) },
            set: { selections[kind.id] = $0 }
        )
    }

    private func row(_ kind: RunnableKind) -> some View {
        let selection = getSelection(kind)
        return JobRunRow(
            kind: kind,
            chosen: selection.wrappedValue.chosen,
            running: starts.getRunning(kind.id, of: subject.id),
            isBusy: starts.isBusy(kind.id, of: subject.id)
        ) {
            RunModelSelect(selection: selection, choices: choices, client: client)
        } onStart: {
            start(kind, on: selection.wrappedValue.chosen)
        }
    }

    private func start(_ kind: RunnableKind, on model: RoutedModel?) {
        if kind.needsNote {
            askForNote(kind, model)
            return
        }
        startFailure = nil
        Task {
            do {
                try await starts.start(kind, for: subject.id, model: model, with: client)
            } catch {
                startFailure = HubFailure("Couldn't start \(JobRuns.getTitle(of: kind))", error)
            }
        }
    }
}

/// A row of the Run popover: the kind's name over its last run or why it
/// can't run yet, its model select, and ▶, which a kind with no model
/// routed takes once a model is picked. The second line wraps rather than
/// being cut, so "kind paused" at its end always shows.
struct JobRunRow<Select: View>: View {
    let kind: RunnableKind
    /// The model picked in the row's select, nil for the kind's own.
    let chosen: RoutedModel?
    /// The run started here that still goes.
    let running: JobRunStarts.Running?
    /// Starting or running, so ▶ waits.
    let isBusy: Bool
    @ViewBuilder let select: () -> Select
    let onStart: () -> Void

    static var modelWidth: CGFloat { 140 }

    var body: some View {
        let title = JobRuns.getTitle(of: kind)
        HStack(spacing: Space.m) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).fontWeight(.medium).lineLimit(1)
                stateLine
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            select()
                .frame(width: Self.modelWidth, alignment: .leading)
            Button(action: onStart) {
                Image(systemName: "play.fill")
            }
            .buttonStyle(.bordered)
            .buttonBorderShape(.circle)
            .tint(.hubAccent)
            .disabled(!JobRuns.canStart(kind, on: chosen) || isBusy)
            .help(JobRuns.describeStart(of: kind, on: chosen))
            .accessibilityLabel("Start \(title)")
        }
        .padding(.vertical, Space.xs)
        .padding(.horizontal, Space.s)
        .accessibilityElement(children: .contain)
    }

    /// "Running 0:42" while a run started here goes, else the last run or
    /// why it can't run yet.
    @ViewBuilder
    private var stateLine: some View {
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
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}
