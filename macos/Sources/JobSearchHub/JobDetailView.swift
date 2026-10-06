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
    /// Everyone at the job's company who can get you in; nil until read.
    private(set) var people: [RelatedPerson]?
    /// The job's company, for its size; nil until read.
    private(set) var company: Company?

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

extension JobDetailModel {
    /// Reads the job's company and the people there, once per job: the
    /// header shows its size, the People card who to write to.
    func loadCompany(_ companyID: UUID, with client: HubClient) async {
        async let people = try? client.get("v1/people", query: PeopleQuery.make(companyID: companyID), as: PeopleResponse.self).people
        async let dossier = try? client.get("v1/companies/\(companyID.uuidString)", as: CompanyDossier.self)
        self.people = await people
        company = await dossier?.company
    }

    /// Asks the job's session for a message to the person, drafted with the
    /// outreach prompt for the owner to send: typed into the running
    /// session, or the first message of its latest resumed, or of a new one.
    /// True once the session has it.
    func draft(to person: RelatedPerson, about details: JobDetails, with client: HubClient) async -> Bool {
        let prompt: String
        do {
            prompt = try await client.get("v1/agent-prompts/outreach_draft", as: AgentPrompt.self).body
        } catch {
            actionError = HubFailure("Couldn't read the outreach prompt", error)
            return false
        }
        let request = JobOutreach.makeDraftRequest(prompt: prompt, to: person, aboutJob: details.job.title, at: details.companyName)
        let session = ClaudeSessionPaneModel()
        await session.load(.job(details.job.id), with: client)
        if session.failure == nil {
            await session.send(request, about: .job(details.job.id), with: client, host: .shared)
        }
        if let failure = session.failure {
            actionError = failure
            return false
        }
        return true
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

/// One job in the inspector: its header with the verdict strip and its
/// actions, then Overview (why it fits, the screen, the people), Prep once
/// pursued (the CV and the interview pack), Posting (the board's facts, the
/// facts read, the posting) and its Session.
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
    /// The card or section to scroll to, once its tab shows.
    @State private var scrollTarget: String?
    @Environment(RemoteTaskRunner.self) private var taskRunner

    /// The ids the strip and the screen scroll to.
    private enum Anchor {
        static let whyItFits = "why-it-fits"
        static let screen = "screen"
        static let people = "people"
        static let posting = "posting"

        static func getCard(of kind: VerdictCell.Kind) -> String {
            switch kind {
            case .match: whyItFits
            // The Pay check is one of the screen's.
            case .screen, .takeHome: screen
            case .people: people
            }
        }
    }

    var body: some View {
        Group {
            if let details = model.details, details.job.id == jobID {
                EntityInspector(
                    subject: .job(jobID), tabs: InspectorTab.getTabs(for: .job(jobID), hasPrep: hasPrep(details)), tab: $tab,
                    scrollTarget: $scrollTarget
                ) {
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
                        boardFacts(details.job)
                        readFacts(details.facts)
                        posting(details.job).id(Anchor.posting)
                    case .session:
                        InspectorSessionTab(subject: .job(jobID), client: client)
                    default:
                        whyItFits(details.brief).id(Anchor.whyItFits)
                        screen(details).id(Anchor.screen)
                        people(details).id(Anchor.people)
                    }
                }
            } else if let loadError = model.loadError {
                HubErrorView(loadError, style: .page, retry: { Task { await model.load(jobID, with: client) } })
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
            if let companyID = model.details?.job.companyID {
                await model.loadCompany(companyID, with: client)
            }
        }
    }

    /// Prep opens once the job is pursued, or once it has a CV.
    private func hasPrep(_ details: JobDetails) -> Bool {
        details.decision?.decision == .pursue || details.cvID != nil
    }

    /// Shows a verdict's card on Overview.
    private func open(_ kind: VerdictCell.Kind) {
        tab = .overview
        scrollTarget = Anchor.getCard(of: kind)
    }

    // MARK: Header

    /// The company with its monogram and size, the title, one line of facts
    /// and where the job stands, then the verdict strip.
    private func header(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .top, spacing: Space.m) {
                Monogram(name: details.companyName ?? details.job.title, size: 40)
                VStack(alignment: .leading, spacing: 2) {
                    companyLine(details)
                    Text(details.job.title).font(.hubEntity).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                    Text(describeFacts(details.job)).font(.hubSecondary).foregroundStyle(.secondary)
                    standing(details)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            VerdictStrip(cells: details.getVerdicts(peopleYouKnow: getPeople(details).count(where: JobOutreach.isKnown))) { open($0) }
        }
    }

    /// "Northwind · 51-200 people", the name opening the company.
    private func companyLine(_ details: JobDetails) -> some View {
        HStack(spacing: Space.xs) {
            if let name = details.companyName, !name.isEmpty {
                if let companyID = details.job.companyID {
                    Button(name) { inspector.open(.company(companyID)) }
                        .buttonStyle(.link)
                        .help("Open \(name)")
                } else {
                    Text(name)
                }
            } else {
                Text("Job").foregroundStyle(.secondary)
            }
            if let size = model.company?.employeeCountRange, !size.isEmpty {
                Text("· \(size) people").foregroundStyle(.secondary)
            }
        }
        .font(.hubSecondary)
        .lineLimit(1)
    }

    /// "Remote, Americas · Contractor · Posted 2 days ago".
    private func describeFacts(_ job: Job) -> String {
        [job.location, job.employmentType, describePosted(job)].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// Where the job stands, when it's out of the feed: skipped, in a phase
    /// of the pipeline, or left for later.
    @ViewBuilder
    private func standing(_ details: JobDetails) -> some View {
        if details.job.dismissedAt != nil {
            ToneChip("Skipped", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName)
                .help(details.job.dismissalReason.map { "Skipped: \($0)" } ?? "Skipped")
        } else if let phase = details.phase {
            ToneChip("In \(phase.name)", tone: .accent, symbol: "rectangle.split.3x1")
        } else if details.decision?.decision == .later {
            ToneChip("Later", tone: .neutral, symbol: "clock")
        }
    }

    /// The decision to make: Pursue, the one primary action, then Later and
    /// Skip, each with its key, the posting as an icon and Fix in the
    /// overflow. A skipped job's next step is Restore, one in an open phase
    /// of the pipeline is Followed up…, and a closed one's is its posting.
    private func actions(_ details: JobDetails) -> some View {
        let isFollowingUp = details.application != nil && details.phase?.isClosed == false
        let isPostingNext = details.job.dismissedAt == nil && details.phase != nil && !isFollowingUp
        return VStack(alignment: .leading, spacing: Space.s) {
            ActionBar {
                if details.job.dismissedAt != nil {
                    restoreButton
                } else if isFollowingUp, let application = details.application {
                    Button("Followed up…", systemImage: "arrowshape.turn.up.right") {
                        followUpNote = ""
                        followingUpApplicationID = application.id
                    }
                    .help("Restarts the count to the next follow-up")
                } else if isPostingNext {
                    openPostingButton(details)
                } else {
                    AsyncButton("Pursue", busyTitle: "Pursuing…", systemImage: "arrow.up.forward", key: "P") {
                        await model.decide(jobID, .pursue, through: decisions, with: client)
                    }
                    .help("Put it on the pipeline (P on Decide and Today)")
                }
            } secondary: {
                if details.decision?.decision != .later && details.phase == nil && details.job.dismissedAt == nil {
                    AsyncButton("Later", busyTitle: "Later", key: "L") {
                        await model.decide(jobID, .later, through: decisions, with: client)
                    }
                    .help("Leave it for another day (L on Decide and Today)")
                }
                if details.job.dismissedAt == nil {
                    Button { isAskingToSkip = true } label: { KeyLabel(title: "Skip…", key: "S") }
                        .help("Take it out as not for you, with a reason (S on Decide and Today)")
                }
            } overflow: {
                Button("Fix…", systemImage: "wrench.adjustable") { isAskingForFix = true }
                    .disabled(taskRunner.fixingJobIDs.contains(jobID))
                    .help("Say what's wrong with its details, and an agent corrects them")
            } accessory: {
                if !isPostingNext {
                    openPostingButton(details)
                        .help("Open the posting on its board")
                }
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

    // MARK: Overview

    /// The brief the decision rests on: its verdict at reading size, then
    /// what speaks for you and against, side by side, each with what backs
    /// it.
    private func whyItFits(_ brief: JobBrief?) -> some View {
        HubCard("Why it fits", meta: brief.map(describeAuthor)) {
            if let brief {
                Text(brief.reason).hubReading().textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                if !brief.strengths.isEmpty || !brief.weaknesses.isEmpty {
                    HStack(alignment: .top, spacing: Space.l) {
                        briefPoints("For you", brief.strengths, in: brief, isWeakness: false)
                        briefPoints("Against", brief.weaknesses, in: brief, isWeakness: true)
                    }
                }
                if brief.isStale {
                    Label("Your knowledge base changed since it was written", systemImage: "clock.arrow.circlepath")
                        .font(.hubCaption)
                        .foregroundStyle(Tone.caution.color)
                }
            } else {
                Text("Not briefed yet. The local model briefs the jobs that don't fail the screen once their facts are read.")
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        } trailing: {
            if brief?.isFull != true {
                AsyncButton("Write full brief", busyTitle: "Writing…", isBusy: model.isWritingFullBrief) {
                    await model.writeFullBrief(jobID, with: client)
                }
                .help("Have Claude write a fuller brief now")
            }
        }
    }

    /// "local model · 2 days ago".
    private func describeAuthor(_ brief: JobBrief) -> String {
        "\(brief.isFull ? "Claude" : "local model") · \(brief.writtenAt.formatted(.relative(presentation: .named)))"
    }

    /// For you or Against: a line per point, with what backs it as a tag
    /// and the knowledge-base entries it cites in its tooltip.
    private func briefPoints(_ title: String, _ points: [JobBriefPoint], in brief: JobBrief, isWeakness: Bool) -> some View {
        let tone: Tone = isWeakness ? .caution : .positive
        return VStack(alignment: .leading, spacing: Space.s) {
            Text(title.uppercased())
                .font(.hubCaption.weight(.semibold))
                .foregroundStyle(tone.color)
                .accessibilityAddTraits(.isHeader)
            if points.isEmpty {
                Text("Nothing").foregroundStyle(.secondary)
            }
            ForEach(Array(points.enumerated()), id: \.offset) { _, point in
                HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                    Image(systemName: isWeakness ? "minus" : "plus")
                        .foregroundStyle(tone.color)
                        .accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(point.point).fixedSize(horizontal: false, vertical: true)
                        if let tag = brief.getTag(of: point, isWeakness: isWeakness) {
                            Text(tag).font(.hubCaption).foregroundStyle(.secondary)
                        }
                    }
                }
                .textSelection(.enabled)
                .help(brief.getEntries(of: point).map(\.label).joined(separator: "; "))
                .accessibilityElement(children: .combine)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// Whether a rule rules you out, exceptions first: the checks that fail
    /// or are unclear, each with its quote and a link into the posting, then
    /// the ones that pass as a row of chips, with what no rule judges.
    private func screen(_ details: JobDetails) -> some View {
        let summary = ScreenBreakdown(details.screenRows, level: details.fit.level)
        let hasPosting = details.job.description?.isEmpty == false
        return HubCard("Screen", meta: summary.title) {
            if summary.exceptions.isEmpty && summary.passes.isEmpty && summary.notes.isEmpty {
                Text("Nothing judged yet. The screen reads the posting once its facts are read.").foregroundStyle(.secondary)
            }
            // A screen-out answer and a criteria check can share a name.
            ForEach(Array(summary.exceptions.enumerated()), id: \.offset) { _, row in
                ExceptionRow(row: row, openInPosting: hasPosting && row.evidence != nil ? { openInPosting() } : nil)
            }
            if !summary.passes.isEmpty || !summary.notes.isEmpty {
                FlowRow {
                    ForEach(Array(summary.passes.enumerated()), id: \.offset) { _, row in
                        ToneChip(row.name, tone: .positive, symbol: "checkmark")
                            .help(describePass(row))
                            .accessibilityValue("passes: \(row.reason)")
                    }
                    ForEach(Array(summary.notes.enumerated()), id: \.offset) { _, row in
                        ToneChip(row.name, tone: .neutral, symbol: "info.circle")
                            .help(describePass(row))
                            .accessibilityValue(row.reason)
                    }
                }
            }
        }
    }

    /// A chip's tooltip: the reason, and the posting's words behind it.
    private func describePass(_ row: ScreenRow) -> String {
        guard let evidence = row.evidence, !evidence.isEmpty else { return row.reason }
        return "\(row.reason): \u{201C}\(evidence)\u{201D}"
    }

    /// Shows the posting the screen quotes.
    private func openInPosting() {
        tab = .posting
        scrollTarget = Anchor.posting
    }

    /// Who to write to at the company, the people you know first, each with
    /// the message that fits: one about the job, or a request for an
    /// introduction.
    private func people(_ details: JobDetails) -> some View {
        let everyone = getPeople(details)
        let shown = Array(everyone.prefix(JobOutreach.shownLimit))
        let known = everyone.count(where: JobOutreach.isKnown)
        let companyName = details.companyName ?? "the company"
        let total = model.people?.count ?? everyone.count
        return HubCard("People", meta: known > 0 ? "\(known) you know at \(companyName)" : nil) {
            if shown.isEmpty {
                Text("No one you know at \(companyName) yet.").foregroundStyle(.secondary)
            }
            ForEach(shown) { person in
                personRow(person, about: details)
                if person.id != shown.last?.id {
                    Divider()
                }
            }
        } trailing: {
            if let companyID = details.job.companyID {
                Button(total > 0 ? "All \(total)" : "Company") { inspector.open(.company(companyID), tab: .people) }
                    .help("Everyone at \(companyName)")
            }
        }
    }

    /// The people at the job's company worth writing to, best first; the
    /// job's own connections until the people list is read.
    private func getPeople(_ details: JobDetails) -> [RelatedPerson] {
        if let companyID = details.job.companyID, let people = model.people {
            return JobOutreach.getPeople(companyID: companyID, among: people)
        }
        return (details.connections ?? []).map { RelatedPerson($0, companyID: details.job.companyID, companyName: details.companyName) }
    }

    /// A person: monogram, name and relation, their role and how close you
    /// are, opening them on a click, and the message that fits.
    private func personRow(_ person: RelatedPerson, about details: JobDetails) -> some View {
        HStack(alignment: .center, spacing: Space.s) {
            InitialsMonogram(person.name, size: 28)
            Button { inspector.open(.person(person.reference)) } label: {
                VStack(alignment: .leading, spacing: 2) {
                    HStack(alignment: .firstTextBaseline, spacing: Space.xs) {
                        Text(person.name).fontWeight(.semibold).lineLimit(1)
                        ToneChip(person.relationTitle, tone: .neutral)
                    }
                    let line = [person.role, person.closeness ?? person.note].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
                    if !line.isEmpty {
                        Text(line).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(2)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help("Open \(person.name)")
            AsyncButton(JobOutreach.getActionTitle(for: person), busyTitle: "Drafting…") {
                if await model.draft(to: person, about: details, with: client) {
                    tab = .session
                }
            }
            .buttonStyle(.bordered)
            .buttonBorderShape(.capsule)
            .help("The job's session drafts it with the outreach prompt; you send it yourself")
        }
        .contextMenu {
            if let profile = person.profileURL.flatMap(URL.init(string:)) {
                Button("Open profile") { NSWorkspace.shared.open(profile) }
            }
            if let email = person.email, let mail = URL(string: "mailto:\(email)") {
                Button("Email \(email)") { NSWorkspace.shared.open(mail) }
            }
        }
    }

    // MARK: Posting

    @ViewBuilder
    private func posting(_ job: Job) -> some View {
        if let description = job.description, !description.isEmpty {
            HubSection("Posting") {
                MarkdownDocument(description)
            } trailing: {
                if let url = URL(string: job.url) {
                    Link("Open", destination: url)
                }
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
