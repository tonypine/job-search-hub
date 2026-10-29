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
    /// header shows its industry and size, the People card who to write to.
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
    func recordFollowUp(_ applicationID: UUID, note: String, of jobID: UUID, through changes: ApplicationChanges, with client: HubClient) async {
        do {
            _ = try await changes.recordFollowUp(applicationID, note: note, with: client)
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

/// Where a job shows: in the inspector beside a list, or as a page in the
/// list's place (P2).
enum JobDetailLayout {
    case inspector, page

    /// Under it, a page hides the posting's outline, puts its rail under the
    /// posting and its actions under the title.
    static var wideWidth: CGFloat { 1100 }
}

/// One job in the inspector: its header with the verdict strip and its
/// actions, then Overview (why it fits, the screen, the people), Prep once
/// pursued (the CV and the interview pack), Posting (the key facts, then the
/// posting to read) and its Session. As a page, the header runs across the
/// top with the actions on its right, and the tabs are Posting, with the
/// Overview's cards on a rail beside the posting, Prep and Session.
struct JobDetailView: View {
    let jobID: UUID
    let client: HubClient
    @Binding var tab: InspectorTab
    var layout = JobDetailLayout.inspector
    /// Told the job's title once it's read, for a page's top bar.
    var showTitle: (String?) -> Void = { _ in }
    /// A page shows the posting's outline beside it when wide; Decide's
    /// page, beside its queue, leaves it out.
    var showsOutline = true
    /// Makes Pursue, Later and Skip, from the action bar or P, L and S, when
    /// the page around the job decides them, as Decide does: it brings up the
    /// next job and offers Undo, so Skip asks nothing first. Nil records them
    /// here, and Skip asks why.
    var decide: (@MainActor (JobDecisionKind) async -> Void)?
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(JobDecisions.self) private var decisions
    @Environment(ApplicationChanges.self) private var changes
    @Environment(DetailsInspector.self) private var inspector
    @Environment(JobRunStarts.self) private var runStarts
    @State private var model = JobDetailModel()
    @State private var isAskingToSkip = false
    @State private var followingUpApplicationID: UUID?
    @State private var followUpNote = ""
    /// The card or section to scroll to, once its tab shows.
    @State private var scrollTarget: String?
    /// The page is at least the wide width.
    @State private var isWide = true
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
            if let details = model.details, details.job.id == jobID, layout == .page {
                page(details)
            } else if let details = model.details, details.job.id == jobID {
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
                        prep(details)
                    case .posting:
                        if model.factsError != nil {
                            HubErrorView($model.factsError)
                        }
                        JobPostingTab(jobID: jobID, details: details, client: client, model: model, postingAnchor: Anchor.posting)
                            .id(jobID)
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
        .background {
            if layout == .page {
                JobPageKeys { key in
                    guard case let .letter(letter) = key else { return false }
                    return decide(byKey: letter)
                }
            }
        }
        .onChange(of: model.details?.job.title, initial: true) { showTitle(model.details?.job.title) }
        .onChange(of: [decisions.revision, taskRunner.fixRevision, runStarts.revision]) { Task { await model.load(jobID, with: client) } }
        .alert("Followed up", isPresented: Binding(get: { followingUpApplicationID != nil }, set: { if !$0 { followingUpApplicationID = nil } })) {
            TextField("What you did", text: $followUpNote)
            Button("Record") {
                guard let applicationID = followingUpApplicationID else { return }
                let note = followUpNote
                Task { await model.recordFollowUp(applicationID, note: note, of: jobID, through: changes, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Restarts the count to the next follow-up.")
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

    /// The CV, and the interview pack once pursued.
    @ViewBuilder
    private func prep(_ details: JobDetails) -> some View {
        JobCVSection(jobID: jobID, details: details, client: client)
        if details.decision?.decision == .pursue {
            InterviewPackSection(jobID: jobID, client: client)
        }
    }

    /// Shows a verdict's card: on Overview, or on a page's rail beside the
    /// posting.
    private func open(_ kind: VerdictCell.Kind) {
        tab = layout == .page ? .posting : .overview
        scrollTarget = Anchor.getCard(of: kind)
    }

    // MARK: Page

    /// The job as a page: the header across the top, with the actions on
    /// its right on a wide page, then the verdict strip and the tabs; on
    /// Posting, the outline, the posting and a rail with the Overview's
    /// cards.
    private func page(_ details: JobDetails) -> some View {
        let tabs: [InspectorTab] = hasPrep(details) ? [.posting, .prep, .session] : [.posting, .session]
        let shown = InspectorTab.resolve(tab, among: tabs)
        let wideWidth = showsOutline ? JobDetailLayout.wideWidth : JobDetailLayout.wideWidth - JobPostingTab<EmptyView>.outlineWidth - Space.xl
        return VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: Space.m) {
                if isWide {
                    HStack(alignment: .top, spacing: Space.l) {
                        VStack(alignment: .leading, spacing: Space.s) {
                            titleBlock(details)
                            actionNotes(details)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        actionBar(details)
                            .fixedSize()
                    }
                } else {
                    titleBlock(details)
                    actionBar(details)
                    actionNotes(details)
                }
                verdictStrip(details)
                TabStrip(items: tabs.map(makePageTab), selection: Binding(get: { shown }, set: { tab = $0 }), showsRule: true)
            }
            .padding(.horizontal, Space.xl)
            .padding(.top, Space.l)
            if model.actionError != nil {
                HubErrorView($model.actionError)
                    .padding(.horizontal, Space.xl)
                    .padding(.top, Space.m)
            }
            switch shown {
            case .session:
                InspectorSessionTab(subject: .job(jobID), client: client)
            case .prep:
                ScrollView {
                    VStack(alignment: .leading, spacing: Space.l) { prep(details) }
                        .frame(minWidth: 0, maxWidth: JobPostingTab<EmptyView>.measure + JobPostingTab<EmptyView>.railWidth, alignment: .topLeading)
                        .padding(Space.xl)
                        .frame(maxWidth: .infinity, alignment: .topLeading)
                }
                // No scroller, as on Posting (TP-1146).
                .scrollIndicators(.never)
            default:
                if model.factsError != nil {
                    HubErrorView($model.factsError)
                        .padding(.horizontal, Space.xl)
                        .padding(.top, Space.m)
                }
                JobPostingTab(
                    jobID: jobID, details: details, client: client, model: model, postingAnchor: Anchor.posting, layout: isWide ? .wide(showsOutline: showsOutline) : .narrow,
                    scrollTarget: $scrollTarget
                ) {
                    whyItFits(details.brief).id(Anchor.whyItFits)
                    screen(details).id(Anchor.screen)
                    people(details).id(Anchor.people)
                }
            }
        }
        .frame(minWidth: 0, maxWidth: .infinity, minHeight: 0, maxHeight: .infinity, alignment: .topLeading)
        // The column's width decides the layout; the layout asks for no
        // width of its own (see JobPageView), so this never feeds back.
        .onGeometryChange(for: Bool.self) { proxy in proxy.size.width >= wideWidth } action: { isWide = $0 }
    }

    /// A page's tab, with a dot on Session while the job's session waits
    /// for you.
    private func makePageTab(_ tab: InspectorTab) -> TabStripItem<InspectorTab> {
        let isWaiting = tab == .session && ClaudeSessionHost.shared.getActivity(about: .job(jobID)) == .blocked
        return TabStripItem(id: tab, title: tab.title, dot: isWaiting ? .caution : nil)
    }

    /// P pursues, L leaves for later and S asks to skip, when the page's
    /// actions offer it. Typing in a field or a session never decides.
    private func decide(byKey key: Character) -> Bool {
        guard let details = model.details, details.job.id == jobID, let decision = KeyboardDecision.getDecision(for: key) else { return false }
        let isOpen = details.job.dismissedAt == nil && details.phase == nil
        switch decision {
        case .pursue where isOpen:
            Task { await record(.pursue) }
        case .later where isOpen && details.decision?.decision != .later:
            Task { await record(.later) }
        case .skip where details.job.dismissedAt == nil:
            skip()
        default:
            return false
        }
        return true
    }

    /// Records the decision through the page that decides it, or here.
    private func record(_ decision: JobDecisionKind) async {
        if let decide {
            await decide(decision)
        } else {
            await model.decide(jobID, decision, through: decisions, with: client)
        }
    }

    /// Skips the job at once where the page offers Undo, or asks why first.
    private func skip() {
        if decide != nil {
            Task { await record(.skip) }
        } else {
            isAskingToSkip = true
        }
    }

    // MARK: Header

    /// The company with its monogram and size, the title, one line of facts
    /// and where the job stands, then the verdict strip.
    private func header(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: Space.m) {
            titleBlock(details)
            verdictStrip(details)
        }
    }

    /// The monogram beside the company, the title, one line of facts and
    /// where the job stands.
    private func titleBlock(_ details: JobDetails) -> some View {
        HStack(alignment: .top, spacing: Space.m) {
            Monogram(details.companyName ?? details.job.title, size: 40)
            VStack(alignment: .leading, spacing: 2) {
                companyLine(details)
                Text(details.job.title).font(.hubEntity).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                Text(describeFacts(details.job)).font(.hubSecondary).foregroundStyle(.secondary)
                standing(details)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func verdictStrip(_ details: JobDetails) -> some View {
        VerdictStrip(cells: details.getVerdicts(people: getPeople(details))) { open($0) }
    }

    /// "Northwind · Developer tools · 51-200 people", the name opening the
    /// company.
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
            if let facts = model.company?.industryAndSize {
                Text("· \(facts)").foregroundStyle(.secondary)
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

    /// The decision to make, then what was decided and why.
    private func actions(_ details: JobDetails) -> some View {
        VStack(alignment: .leading, spacing: Space.s) {
            actionBar(details)
            actionNotes(details)
        }
    }

    /// The decision to make: Pursue, the one primary action, then Later and
    /// Skip, each with its key, Run ▾, the only way the header starts a run,
    /// fixing the job's details included, and the posting as an icon. A
    /// skipped job's next step is Restore, one in an open phase of the
    /// pipeline is Followed up…, and a closed one's is its posting.
    private func actionBar(_ details: JobDetails) -> some View {
        let isFollowingUp = details.application != nil && details.phase?.isClosed == false
        let isPostingNext = details.job.dismissedAt == nil && details.phase != nil && !isFollowingUp
        return ActionBar {
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
                    await record(.pursue)
                }
                .help("Put it on the pipeline (P on Decide, Today and a job's page)")
            }
        } secondary: {
            if details.decision?.decision != .later && details.phase == nil && details.job.dismissedAt == nil {
                AsyncButton("Later", busyTitle: "Later", key: "L") {
                    await record(.later)
                }
                .help("Leave it for another day (L on Decide, Today and a job's page)")
            }
            if details.job.dismissedAt == nil {
                Button(action: skip) { KeyLabel(title: decide == nil ? "Skip…" : "Skip", key: "S") }
                    .help(decide == nil
                        ? "Take it out as not for you, with a reason (S on Decide, Today and a job's page)"
                        : "Take it out as not for you, with Undo and a reason from the note that says so (S)")
            }
            JobRunButton(jobID: jobID, jobTitle: details.job.title, client: client)
        } overflow: {
            EmptyView()
        } accessory: {
            if !isPostingNext {
                openPostingButton(details)
                    .help("Open the posting on its board")
            }
        }
    }

    /// What was decided and when, and what the pipeline card keeps off its
    /// face: why it closed or was skipped, its notes, a fix under way.
    @ViewBuilder
    private func actionNotes(_ details: JobDetails) -> some View {
        if let decision = details.decision {
            Text(describeDecision(decision)).font(.hubCaption).foregroundStyle(.secondary)
        }
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
    /// it; one over the other on a page's rail.
    private func whyItFits(_ brief: JobBrief?) -> some View {
        HubCard("Why it fits", meta: brief.map { Self.describeAuthor(isFull: $0.isFull, writtenAt: $0.writtenAt) }) {
            if let brief {
                Text(brief.reason).hubReading().textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                if !brief.strengths.isEmpty || !brief.weaknesses.isEmpty {
                    let points = layout == .page
                        ? AnyLayout(VStackLayout(alignment: .leading, spacing: Space.l))
                        : AnyLayout(HStackLayout(alignment: .top, spacing: Space.l))
                    points {
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

    /// "local model · 2 days ago", "local model · 5 min ago": short units
    /// keep it whole beside Write full brief on a page's rail.
    static func describeAuthor(isFull: Bool, writtenAt: Date) -> String {
        "\(isFull ? "Claude" : "local model") · \(writtenAt.formatted(.relative(presentation: .named, unitsStyle: .abbreviated)))"
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
        let companyName = details.companyName ?? "the company"
        let total = model.people?.count ?? everyone.count
        return HubCard("People", meta: JobOutreach.describe(everyone).map { "\($0) at \(companyName)" }) {
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
            Monogram(person.name, size: 28)
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
}
