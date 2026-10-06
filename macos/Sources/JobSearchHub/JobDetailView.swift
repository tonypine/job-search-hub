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

    /// Counts a follow-up on the job's application, then shows the job as it
    /// now stands.
    func recordFollowUp(_ applicationID: UUID, note: String, of jobID: UUID, with client: HubClient) async {
        do {
            _ = try await client.send(
                "POST", "v1/applications/\(applicationID.uuidString)/follow-ups", body: FollowUpRequest(note: note), as: ApplicationResponse.self
            )
            await load(jobID, with: client)
        } catch {
            actionError = HubFailure("Couldn't record the follow-up", error)
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

/// One job in the inspector: its header and actions, then Overview (the
/// brief, the screen, the people), Prep once pursued (the CV and the
/// interview pack), Posting (the key facts, then the posting to read) and its
/// Session.
struct JobDetailView: View {
    let jobID: UUID
    let client: HubClient
    @Binding var tab: InspectorTab
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(JobDecisions.self) private var decisions
    @Environment(DetailsInspector.self) private var inspector
    @State private var model = JobDetailModel()
    @State private var isAskingToSkip = false
    @State private var isAskingForFix = false
    @State private var followingUpApplicationID: UUID?
    @State private var followUpNote = ""
    @Environment(RemoteTaskRunner.self) private var taskRunner

    var body: some View {
        Group {
            if let details = model.details, details.job.id == jobID {
                EntityInspector(subject: .job(jobID), tabs: InspectorTab.getTabs(for: .job(jobID), hasPrep: hasPrep(details)), tab: $tab) {
                    header(details)
                    actions(details)
                } content: { tab in
                    if model.actionError != nil {
                        HubErrorView($model.actionError)
                    }
                    switch tab {
                    case .prep:
                        JobCVSection(jobID: jobID, details: details, client: client)
                        if details.decision?.decision == .pursue {
                            InterviewPackSection(jobID: jobID, client: client)
                        }
                    case .posting:
                        if model.factsError != nil {
                            HubErrorView($model.factsError)
                        }
                        JobPostingTab(jobID: jobID, details: details, client: client, model: model)
                            .id(jobID)
                    case .session:
                        InspectorSessionTab(subject: .job(jobID), client: client)
                    default:
                        brief(details.brief)
                        screen(details)
                        people(details)
                    }
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(jobID, with: client) } }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .onChange(of: [decisions.revision, taskRunner.fixRevision]) { Task { await model.load(jobID, with: client) } }
        .alert("Followed up", isPresented: Binding(get: { followingUpApplicationID != nil }, set: { if !$0 { followingUpApplicationID = nil } })) {
            TextField("What you did", text: $followUpNote)
            Button("Record") {
                guard let applicationID = followingUpApplicationID else { return }
                let note = followUpNote
                Task { await model.recordFollowUp(applicationID, note: note, of: jobID, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Restarts the count to the next follow-up.")
        }
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

    /// Prep opens once the job is pursued, or once it has a CV.
    private func hasPrep(_ details: JobDetails) -> Bool {
        details.decision?.decision == .pursue || details.cvID != nil
    }

    /// The job's kind and company, its title and location, and up to three
    /// chips: the brief's match, the screen, and where the job stands.
    private func header(_ details: JobDetails) -> some View {
        EntityHeader(
            kind: "Job", parent: details.companyName, openParent: details.job.companyID.map { id -> () -> Void in { inspector.open(.company(id)) } },
            title: details.job.title, facts: [details.job.location, describePosted(details.job)]
        ) {
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
    /// step is Restore, one in an open phase of the pipeline is Followed
    /// up…, and a closed one's is its posting.
    private func actions(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            ActionBar {
                if details.job.dismissedAt != nil {
                    restoreButton
                } else if let application = details.application, details.phase?.isClosed == false {
                    Button("Followed up…", systemImage: "arrowshape.turn.up.right") {
                        followUpNote = ""
                        followingUpApplicationID = application.id
                    }
                    .help("Restarts the count to the next follow-up")
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
                if details.phase?.isClosed == false && details.application != nil {
                    openPostingButton(details)
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
            // What the pipeline card keeps off its face.
            if details.phase?.isClosed == true, let closedReason = details.application?.closedReason, !closedReason.isEmpty {
                Label("Closed: \(closedReason)", systemImage: SetAside.closed.symbolName).font(.hubCaption).foregroundStyle(.secondary)
            }
            if details.job.dismissedAt != nil, let dismissalReason = details.job.dismissalReason, !dismissalReason.isEmpty,
               details.decision?.reason != dismissalReason {
                Label("Skipped: \(dismissalReason)", systemImage: SetAside.skipped.symbolName).font(.hubCaption).foregroundStyle(.secondary)
            }
            if let notes = details.application?.notes?.trimmingCharacters(in: .whitespacesAndNewlines), !notes.isEmpty {
                Label(notes, systemImage: "note.text").font(.hubCaption).foregroundStyle(.secondary)
            }
            if taskRunner.fixingJobIDs.contains(jobID) {
                Label("An agent is fixing its details…", systemImage: "wrench.adjustable").font(.hubCaption).foregroundStyle(.secondary)
            }
        }
    }

    /// "Posted 2 days ago", from the board's date or else when the hub first
    /// saw it.
    private func describePosted(_ job: Job) -> String {
        "Posted \((job.publishedAt ?? job.firstSeenAt).formatted(.relative(presentation: .named)))"
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

    /// The people you know at the company, and a link to its People tab.
    @ViewBuilder
    private func people(_ details: JobDetails) -> some View {
        let connections = details.connections ?? []
        if !connections.isEmpty || details.job.companyID != nil {
            HubSection("People") {
                if connections.isEmpty {
                    Text("No one you know at \(details.companyName ?? "this company") yet.").foregroundStyle(.secondary)
                }
                ConnectionList(connections: connections)
            } trailing: {
                if let companyID = details.job.companyID {
                    Button("Company") { inspector.open(.company(companyID), tab: .people) }
                        .buttonStyle(.link)
                        .help("Everyone at \(details.companyName ?? "the company")")
                }
            }
        }
    }
}
