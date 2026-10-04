import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class JobDetailModel {
    private(set) var details: JobDetails?
    private(set) var loadError: HubFailure?
    var actionError: HubFailure?
    /// Where a "Read facts now" run stands, while one is going.
    private(set) var factsReadState: JobFactsReadState = .notQueued
    private(set) var isReadingFacts = false
    var factsError: HubFailure?
    private(set) var isWritingFullBrief = false

    func load(_ jobID: UUID, with client: HubClient) async {
        do {
            details = try await client.get("v1/jobs/\(jobID.uuidString)", as: JobDetails.self)
            loadError = nil
        } catch {
            details = nil
            loadError = HubFailure("Couldn't load the job", error)
        }
    }
}

extension JobDetailModel {
    /// Asks the hub to read the job's facts ahead of background work, follows
    /// the run through the model queue, then shows the fresh facts.
    func readFactsNow(_ jobID: UUID, with client: HubClient) async {
        isReadingFacts = true
        factsReadState = .queued
        defer {
            isReadingFacts = false
            factsReadState = .notQueued
        }
        do {
            _ = try await client.send("POST", "v1/jobs/\(jobID.uuidString)/facts/read", body: EmptyBody(), as: FactsReadResponse.self)
            let deadline = Date.now.addingTimeInterval(20 * 60)
            while Date.now < deadline {
                try await Task.sleep(for: .seconds(2))
                factsReadState = try await client.get("v1/model-work", as: ModelWork.self).getFactsReadState(for: jobID)
                if factsReadState == .notQueued { break }
            }
            await load(jobID, with: client)
        } catch is CancellationError {
        } catch {
            factsError = HubFailure("Couldn't read the facts", error)
        }
    }
}

private struct FactsReadResponse: Decodable {
    var queued: Bool
}

extension JobDetailModel {
    /// Records the decision, then shows the job as it now stands.
    func decide(_ jobID: UUID, _ decision: JobDecisionKind, reason: String = "", through decisions: JobDecisions, with client: HubClient) async {
        do {
            _ = try await decisions.decide(jobID, decision, reason: reason, with: client)
            await load(jobID, with: client)
        } catch {
            actionError = HubFailure("Couldn't record the decision", error)
        }
    }

    /// Asks Claude for the job's full brief, then follows the job until the
    /// full brief replaces the pre-brief, for up to three minutes.
    func writeFullBrief(_ jobID: UUID, with client: HubClient) async {
        isWritingFullBrief = true
        defer { isWritingFullBrief = false }
        let writtenBefore = details?.brief?.isFull == true ? details?.brief?.writtenAt : nil
        do {
            try await client.writeFullBrief(jobID)
            let deadline = Date.now.addingTimeInterval(3 * 60)
            while Date.now < deadline {
                try await Task.sleep(for: .seconds(3))
                await load(jobID, with: client)
                if let brief = details?.brief, brief.isFull, brief.writtenAt != writtenBefore { return }
            }
            actionError = HubFailure("The full brief isn't ready", advice: "Claude hasn't written the brief yet; it shows here once it's saved.")
        } catch is CancellationError {
        } catch {
            actionError = HubFailure("Couldn't ask for the full brief", error)
        }
    }
}

/// Which side of a job or company panel shows: what the hub knows, or its
/// Claude session.
enum PanelSide: String, CaseIterable, Identifiable {
    case details = "Details"
    case session = "Session"

    var id: String { rawValue }
}

/// A job's panel: its details, or its Claude session.
struct JobPanel: View {
    let jobID: UUID
    let client: HubClient
    /// Opens on the Session side and starts or resumes the session there.
    var opensSession = false
    @State private var side: PanelSide = .details

    var body: some View {
        VStack(spacing: 0) {
            Picker("Show", selection: $side) {
                ForEach(PanelSide.allCases) { side in Text(side.rawValue).tag(side) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(Space.s)
            switch side {
            case .details: JobDetailView(jobID: jobID, client: client)
            case .session: ClaudeSessionPane(subject: .job(jobID), client: client, startsOnAppear: opensSession)
            }
        }
        .onAppear { if opensSession { side = .session } }
    }
}

/// One job's details, read from the hub: the decision and its brief, what the
/// board publishes, the facts read from the posting, and the posting itself.
struct JobDetailView: View {
    let jobID: UUID
    let client: HubClient
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(JobDecisions.self) private var decisions
    @State private var model = JobDetailModel()
    @State private var isAskingToSkip = false
    @State private var isAskingForFix = false
    @Environment(RemoteTaskRunner.self) private var taskRunner
    @State private var isPostingShown = false

    var body: some View {
        Group {
            if let details = model.details, details.job.id == jobID {
                ScrollView {
                    VStack(alignment: .leading, spacing: Space.l) {
                        if model.actionError != nil {
                            HubErrorView($model.actionError)
                        }
                        if model.factsError != nil {
                            HubErrorView($model.factsError)
                        }
                        header(details)
                        actions(details)
                        brief(details.brief)
                        if details.decision?.decision == .pursue || details.cvID != nil {
                            JobCVSection(jobID: jobID, details: details, client: client)
                        }
                        if details.decision?.decision == .pursue {
                            InterviewPackSection(jobID: jobID, client: client)
                        }
                        screen(details)
                        if let connections = details.connections, !connections.isEmpty {
                            HubSection("People you know at \(details.companyName ?? "this company")") {
                                ConnectionList(connections: connections)
                            }
                        }
                        boardFacts(details.job)
                        readFacts(details.facts)
                        if let description = details.job.description, !description.isEmpty {
                            // With a brief to decide from, the posting folds away.
                            DisclosureGroup(isExpanded: Binding(get: { isPostingShown || details.brief == nil }, set: { isPostingShown = $0 })) {
                                Text(description).textSelection(.enabled).fixedSize(horizontal: false, vertical: true).padding(.top, Space.xs)
                            } label: {
                                Text("Posting").font(.hubSection)
                            }
                        }
                    }
                    .padding(Space.l)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(jobID, with: client) } }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .onChange(of: [decisions.revision, taskRunner.fixRevision]) { Task { await model.load(jobID, with: client) } }
        .sheet(isPresented: $isAskingForFix) {
            FixJobSheet(jobTitle: model.details?.job.title ?? "this job") { note in
                do {
                    try await taskRunner.fixJob(jobID, note: note, with: client)
                    return nil
                } catch {
                    return HubFailure("Couldn't ask for the fix", error)
                }
            }
        }
        .sheet(isPresented: $isAskingToSkip) {
            SkipJobsSheet(jobCount: 1) { reason in
                do {
                    _ = try await decisions.decide(jobID, .skip, reason: reason, with: client)
                    return nil
                } catch {
                    return HubFailure("Couldn't skip the job", error)
                }
            }
        }
        .task(id: jobID) {
            await model.load(jobID, with: client)
            if let details = model.details, details.unseenUpdates > 0 {
                await unseen.markSeen(UpdateSelection(jobID: jobID), with: client)
            }
        }
    }

    /// The job's kind and company, its title and location, and up to three
    /// chips: the brief's match, the screen, and where the job stands.
    private func header(_ details: JobDetails) -> some View {
        EntityHeader(eyebrow: ["Job", details.companyName].compactMap { $0 }.joined(separator: " · "), title: details.job.title, facts: [details.job.location]) {
            if let brief = details.brief {
                ToneChip(brief.match)
            }
            ToneChip(screen: details.fit.level)
            if details.job.dismissedAt != nil {
                ToneChip("Skipped", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName)
                    .help(details.job.dismissalReason.map { "Skipped: \($0)" } ?? "Skipped")
            } else if let phase = details.phase {
                ToneChip("In \(phase.name)", tone: .accent, symbol: "rectangle.split.3x1")
            } else if details.decision?.decision == .later {
                ToneChip("Later", tone: .neutral, symbol: "clock")
            }
        }
    }

    /// The decision to make: Pursue, the one primary action, then Later and
    /// Skip, with the posting and Fix in the overflow. A skipped job's next
    /// step is Restore, and a pursued one's is its posting.
    private func actions(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            ActionBar {
                if details.job.dismissedAt != nil {
                    restoreButton
                } else if details.phase != nil {
                    openPostingButton(details)
                } else {
                    AsyncButton("Pursue", busyTitle: "Pursuing…", systemImage: "arrow.up.forward") {
                        await model.decide(jobID, .pursue, through: decisions, with: client)
                    }
                    .help("Put it on the pipeline")
                }
            } secondary: {
                if details.decision?.decision != .later && details.phase == nil && details.job.dismissedAt == nil {
                    AsyncButton("Later", busyTitle: "Later", systemImage: "clock") {
                        await model.decide(jobID, .later, through: decisions, with: client)
                    }
                    .help("Leave it for another day")
                }
                if details.job.dismissedAt == nil {
                    Button("Skip…", systemImage: "eye.slash") { isAskingToSkip = true }
                        .help("Take it out as not for you, with a reason")
                }
            } overflow: {
                if details.phase == nil {
                    openPostingButton(details)
                }
                Button("Fix…", systemImage: "wrench.adjustable") { isAskingForFix = true }
                    .disabled(taskRunner.fixingJobIDs.contains(jobID))
                    .help("Say what's wrong with its details, and an agent corrects them")
            }
            if let decision = details.decision {
                Text(describeDecision(decision)).font(.hubCaption).foregroundStyle(.secondary)
            }
            if taskRunner.fixingJobIDs.contains(jobID) {
                Label("An agent is fixing its details…", systemImage: "wrench.adjustable").font(.hubCaption).foregroundStyle(.secondary)
            }
        }
    }

    private var restoreButton: some View {
        AsyncButton("Restore", busyTitle: "Restoring…", systemImage: "arrow.uturn.backward") {
            do {
                _ = try await decisions.restore([jobID], with: client)
            } catch {
                model.actionError = HubFailure("Couldn't restore the job", error)
            }
        }
    }

    @ViewBuilder
    private func openPostingButton(_ details: JobDetails) -> some View {
        if let url = URL(string: details.job.url) {
            Button("Open posting", systemImage: "safari") { NSWorkspace.shared.open(url) }
        }
    }

    private func describeDecision(_ decision: JobDecision) -> String {
        var text = "\(decision.decision.pastTense) \(decision.decidedAt.formatted(.relative(presentation: .named)))"
        if let reason = decision.reason, !reason.isEmpty {
            text += ": \(reason)"
        }
        return text
    }

    /// The brief the decision rests on: the match and why, then the strengths
    /// and weaknesses with the knowledge-base entries behind them.
    private func brief(_ brief: JobBrief?) -> some View {
        HubSection("Brief") {
            if let brief {
                if brief.isStale {
                    Label("Your knowledge base changed since", systemImage: "clock.arrow.circlepath")
                        .font(.hubCaption)
                        .foregroundStyle(Tone.caution.color)
                }
                Text(brief.reason).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                briefPoints("Strengths", brief.strengths, in: brief, symbol: "plus.circle.fill", tone: .positive)
                briefPoints("Weaknesses", brief.weaknesses, in: brief, symbol: "minus.circle.fill", tone: .caution)
            } else {
                Text("Not briefed yet. The local model briefs the jobs that don't fail the screen once their facts are read.").foregroundStyle(.secondary)
            }
            if brief?.isFull != true {
                AsyncButton("Write full brief", busyTitle: "Writing…", systemImage: "sparkles", isBusy: model.isWritingFullBrief) {
                    await model.writeFullBrief(jobID, with: client)
                }
                .help("Have Claude write a fuller brief now")
            }
        } trailing: {
            if let brief {
                Text(brief.isFull ? "by Claude" : "by the local model").font(.hubCaption).foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func briefPoints(_ title: String, _ points: [JobBriefPoint], in brief: JobBrief, symbol: String, tone: Tone) -> some View {
        if !points.isEmpty {
            Text(title).font(.hubSecondary.weight(.semibold)).foregroundStyle(.secondary)
            ForEach(Array(points.enumerated()), id: \.offset) { _, point in
                VerdictRow(
                    symbol: symbol, tone: tone, reason: point.point,
                    note: brief.getEntries(of: point).map(\.label).joined(separator: "; ")
                )
            }
        }
    }

    /// Whether a rule rules you out: the screen-out answers from the
    /// posting's words, then the criteria checks they don't repeat.
    @ViewBuilder
    private func screen(_ details: JobDetails) -> some View {
        let rows = details.screenRows
        if !rows.isEmpty {
            HubSection("Screen") {
                // A screen-out answer and a criteria check can share a name.
                ForEach(Array(rows.enumerated()), id: \.offset) { _, row in
                    VerdictRow(row.verdict, name: row.name, reason: row.reason, evidence: row.evidence)
                }
            } trailing: {
                ToneChip(details.fit.level)
            }
        }
    }

    private func boardFacts(_ job: Job) -> some View {
        HubSection("From the board") {
            FactGrid {
                FactRow("Pay") {
                    if let pay = job.pay {
                        VStack(alignment: .leading, spacing: 2) {
                            ForEach(Array(pay.ranges.enumerated()), id: \.offset) { _, range in
                                Text([range.label, range.format()].compactMap { $0 }.joined(separator: ": "))
                            }
                            if let summary = pay.summary {
                                Text(summary).foregroundStyle(.secondary)
                            }
                        }
                    } else {
                        Text("Not published").foregroundStyle(.secondary)
                    }
                }
                FactRow("Workplace", text: job.workplaceType)
                FactRow("Employment", text: job.employmentType)
                FactRow("Department", text: job.department)
                FactRow("Also hiring in", text: job.otherLocations?.joined(separator: ", "))
                FactRow("Published", text: job.publishedAt?.formatted(date: .abbreviated, time: .omitted))
                FactRow("First seen", text: job.firstSeenAt.formatted(date: .abbreviated, time: .omitted))
            }
        }
    }

    private func readFacts(_ facts: LabelledJobFacts?) -> some View {
        HubSection("Read from the posting") {
            if let facts {
                FactGrid {
                    ForEach(facts.entries) { entry in
                        FactRow(entry.title, help: entry.description) {
                            VStack(alignment: .leading, spacing: 2) {
                                switch entry.display {
                                case let .text(text): Text(text)
                                case let .list(items): Text(items.joined(separator: ", "))
                                case .notStated: Text("Not stated").foregroundStyle(.secondary)
                                }
                                if let evidence = entry.evidence {
                                    Evidence(text: evidence)
                                }
                            }
                        }
                    }
                }
                Text("Read by \(facts.model) with prompt version \(facts.promptVersion), \(facts.extractedAt.formatted(date: .abbreviated, time: .shortened)).")
                    .font(.hubCaption)
                    .foregroundStyle(.tertiary)
            } else {
                Text("Not read yet. The hub reads new postings as they arrive, unless its model work is paused.")
                    .foregroundStyle(.secondary)
            }
            readFactsNowRow
        }
    }

    /// The "Read facts now" button, and where its run stands.
    private var readFactsNowRow: some View {
        HStack(spacing: Space.s) {
            AsyncButton("Read facts now", busyTitle: "Reading…", systemImage: "arrow.clockwise", isBusy: model.isReadingFacts) {
                await model.readFactsNow(jobID, with: client)
            }
            switch model.factsReadState {
            case .queued: Text("Waiting for its turn").foregroundStyle(.secondary)
            case .running: Text("Reading now").foregroundStyle(.secondary)
            case .notQueued: EmptyView()
            }
        }
        .padding(.top, Space.xs)
    }
}
