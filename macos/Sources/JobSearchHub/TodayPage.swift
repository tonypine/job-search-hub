import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class TodayModel {
    /// The most unseen updates read; Today lists a few and links to the rest.
    static let updatesLimit = 50

    private(set) var queue: [DecisionQueueItem] = []
    private(set) var board = PipelineBoard()
    private(set) var updates: [HubUpdate] = []
    /// Everyone who can get the owner in; Today lists the recruiters waiting.
    private(set) var people: [RelatedPerson] = []
    /// The features waiting on the owner's first use.
    private(set) var firstSteps: [FirstStep] = []
    private(set) var hasLoaded = false
    private(set) var loadError: HubFailure?
    /// Why the last action on an item failed: a decision, a follow-up.
    var actionError: HubFailure?
    var toast: ToastMessage?

    /// Reads the five sources Today sums up. One that fails keeps what it
    /// showed, and the error says so.
    func load(with client: HubClient) async {
        async let queue = client.getDecisionQueue()
        async let pipeline = client.getPipeline()
        async let updates = client.get(
            "v1/updates", query: [URLQueryItem(name: "unseen", value: "true"), URLQueryItem(name: "limit", value: String(Self.updatesLimit))],
            as: HubUpdateList.self
        )
        async let people = client.get("v1/people", as: PeopleResponse.self)
        async let firstSteps = client.get("v1/first-steps", as: FirstStepsResponse.self)
        var failure: (any Error)?
        do { self.queue = try await queue.items } catch { failure = failure ?? error }
        do { board = PipelineBoard(try await pipeline) } catch { failure = failure ?? error }
        do { self.updates = try await updates.updates } catch { failure = failure ?? error }
        do { self.people = try await people.people } catch { failure = failure ?? error }
        do { self.firstSteps = try await firstSteps.steps } catch { failure = failure ?? error }
        loadError = failure.map { HubFailure("Couldn't load everything for Today", $0) }
        hasLoaded = true
    }

    /// Records the decision and says so; the queue reads again through the
    /// decisions' revision.
    func decide(_ item: DecisionQueueItem, _ decision: JobDecisionKind, reason: String = "", through decisions: JobDecisions, with client: HubClient) async -> HubFailure? {
        do {
            _ = try await decisions.decide(item.id, decision, reason: reason, with: client)
        } catch {
            return HubFailure("Couldn't record the decision", error)
        }
        switch decision {
        case .pursue: toast = ToastMessage(text: "Pursued \(item.job.title)")
        case .later: toast = ToastMessage(text: "Left \(item.job.title) for later", tone: .neutral, symbol: "clock")
        case .skip: toast = ToastMessage(text: "Skipped \(item.job.title)", tone: SetAside.skipped.tone, symbol: SetAside.skipped.symbolName)
        }
        return nil
    }

    /// Records a follow-up, then reads again, so the card leaves Follow up
    /// with its new due date.
    func recordFollowUp(_ card: PipelineCard, note: String, with client: HubClient) async {
        do {
            _ = try await client.send("POST", "v1/applications/\(card.id.uuidString)/follow-ups", body: FollowUpRequest(note: note), as: ApplicationResponse.self)
            toast = ToastMessage(text: "Recorded the follow-up on \(card.title)")
            await load(with: client)
        } catch {
            actionError = HubFailure("Couldn't record the follow-up", error)
        }
    }
}

/// What the hub's agents and local models are doing, read every few seconds
/// while Today shows: model work raises no updates.
@MainActor
@Observable
final class HubActivityModel {
    static let agentRunsLimit = 20

    private(set) var lines: [String] = []

    func watch(with client: HubClient) async {
        while !Task.isCancelled {
            await load(with: client)
            try? await Task.sleep(for: .seconds(5))
        }
    }

    func load(with client: HubClient) async {
        async let work = try? client.get("v1/model-work", as: ModelWork.self)
        async let runs = try? client.get(
            "v1/agent-runs", query: [URLQueryItem(name: "limit", value: String(Self.agentRunsLimit))], as: AgentRunsResponse.self
        )
        let (readWork, readRuns) = await (work, runs)
        lines = HubActivity.describe(work: readWork, agentRuns: readRuns?.runs ?? [])
    }
}

/// What needs you now: a card for each source that has something, under a
/// line of chips that sums them up. Each item acts in place and opens in the
/// inspector over this page.
struct TodayPage: View {
    /// How many follow-ups, updates and recruiters a card lists before its
    /// link to the rest.
    private static let shownCount = 5
    /// Below it, the cards stack in one column.
    private static let twoColumnWidth: CGFloat = 720

    /// Switches the window to another page: Decide, Pipeline, the full
    /// history of updates, and the pages Get started's rows land on.
    let openPage: (Page) -> Void
    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(DetailsInspector.self) private var details
    @Environment(JobDecisions.self) private var decisions
    @Environment(RecruiterReplyDraft.self) private var replyDraft
    @Environment(PageRequests.self) private var requests
    @State private var model = TodayModel()
    @State private var activity = HubActivityModel()
    @State private var isWide = true
    @State private var skipping: DecisionQueueItem?
    @State private var followingUp: PipelineCard?
    @State private var followUpNote = ""
    /// The Decide card has the keyboard: ↑↓ move through its jobs, Return
    /// opens one, and P, L and S decide the one open.
    @FocusState private var isDecideFocused: Bool
    /// The palette asked for the keyboard before the queue was read.
    @State private var focusesDecideOnLoad = false

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header
                    content(client)
                }
                .task { await model.load(with: client) }
                .task { await activity.watch(with: client) }
                .onChange(of: [events.revision, unseen.revision, decisions.revision]) { Task { await model.load(with: client) } }
                .onChange(of: events.revision) { Task { await activity.load(with: client) } }
                .onPageRequest(.today) { request in
                    guard request == .focusList else { return }
                    if model.hasLoaded {
                        isDecideFocused = !model.queue.isEmpty
                    } else {
                        focusesDecideOnLoad = true
                    }
                }
                .onChange(of: model.hasLoaded) {
                    if focusesDecideOnLoad && model.hasLoaded {
                        focusesDecideOnLoad = false
                        isDecideFocused = !model.queue.isEmpty
                    }
                }
            }
        }
        .navigationTitle("Today")
        .navigationSubtitle(Date.now.formatted(.dateTime.weekday(.wide).day().month(.wide)))
    }

    /// The page's controls, in its header rather than in the window's
    /// toolbar: Add, which opens the page that adds the thing.
    private var header: some View {
        PageHeader {
            EmptyView()
        } trailing: {
            Menu("Add", systemImage: "plus") {
                Button("Job by URL…") {
                    openPage(.jobs)
                    requests.ask(.addJobByURL, on: .jobs)
                }
                Button("Company…") {
                    openPage(.companies)
                    requests.ask(.addCompany, on: .companies)
                }
            }
            .fixedSize()
            .help("Add a job by its posting's URL, or a company to watch")
        }
    }

    /// What each card lists, read from the model once per render.
    private var items: TodayItems {
        let followUps = Today.getDueFollowUps(model.board, now: .now)
        return TodayItems(
            decisions: Today.getTopDecisions(model.queue), followUps: followUps, news: Today.getUnseenNews(model.updates),
            recruiters: Today.getWaitingRecruiters(model.people), starts: Today.getStartRows(model.firstSteps),
            chips: Today.getChips(toDecide: model.queue.count, followUps: followUps, updates: model.updates)
        )
    }

    private func content(_ client: HubClient) -> some View {
        let items = self.items
        let twoColumnWidth = Self.twoColumnWidth
        return ScrollView {
            cards(items, client: client)
                .padding(Space.xl)
                .frame(maxWidth: 1200, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .onGeometryChange(for: Bool.self) { proxy in proxy.size.width >= twoColumnWidth } action: { isWide = $0 }
        .overlay {
            if items.isEmpty && activity.lines.isEmpty && model.hasLoaded && model.loadError == nil {
                ContentUnavailableView(
                    "Nothing needs you now", systemImage: "sun.max",
                    description: Text("Jobs to decide, follow-ups due, replies, recruiters waiting and features to start show here.")
                )
            }
        }
        .overlay(alignment: .bottom) {
            if model.actionError != nil {
                HubErrorView($model.actionError)
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
        .toast($model.toast)
        .sheet(item: $skipping) { item in
            SkipJobsSheet(jobCount: 1) { reason in
                await decide(item, .skip, reason: reason, client: client)
            }
        }
        .alert("Followed up", isPresented: Binding(get: { followingUp != nil }, set: { if !$0 { followingUp = nil } })) {
            TextField("What you did", text: $followUpNote)
            Button("Record") {
                guard let card = followingUp else { return }
                Task { await model.recordFollowUp(card, note: followUpNote, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Restarts the count to the next follow-up.")
        }
    }

    /// The chips, then the cards that have something: in two columns when
    /// the page is wide enough, else one.
    @ViewBuilder
    private func cards(_ items: TodayItems, client: HubClient) -> some View {
        VStack(alignment: .leading, spacing: Space.l) {
            if !items.chips.isEmpty {
                HStack(spacing: Space.s) {
                    ForEach(items.chips) { chip in ToneChip(chip.text, tone: chip.tone, symbol: chip.symbol) }
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(Today.summarize(items.chips))
            }
            if let loadError = model.loadError {
                HubErrorView(loadError, retry: { Task { await model.load(with: client) } })
            }
            if isWide {
                HStack(alignment: .top, spacing: Space.l) {
                    VStack(spacing: Space.l) { leftCards(items, client: client) }.frame(maxWidth: .infinity, alignment: .top)
                    VStack(spacing: Space.l) { rightCards(items, client: client) }.frame(maxWidth: .infinity, alignment: .top)
                }
            } else {
                VStack(spacing: Space.l) {
                    leftCards(items, client: client)
                    rightCards(items, client: client)
                }
            }
        }
    }

    @ViewBuilder
    private func leftCards(_ items: TodayItems, client: HubClient) -> some View {
        if !items.starts.isEmpty { startCard(items.starts) }
        if !items.decisions.isEmpty { decideCard(items.decisions, client: client) }
        if !items.recruiters.isEmpty { recruitersCard(items.recruiters, client: client) }
    }

    @ViewBuilder
    private func rightCards(_ items: TodayItems, client: HubClient) -> some View {
        if !items.followUps.isEmpty { followUpCard(items.followUps) }
        if !items.news.isEmpty { updatesCard(items.news, client: client) }
        if !activity.lines.isEmpty { hubCard }
    }

    // MARK: Cards

    private func decideCard(_ items: [DecisionQueueItem], client: HubClient) -> some View {
        TodayCard("Decide", link: "All \(model.queue.count)") { openPage(.decide) } rows: {
            ForEach(items) { item in
                TodayDecisionRow(item: item, isOpen: isOpen(.job(item.id))) {
                    open(.job(item.id))
                    isDecideFocused = true
                } decide: { decision in
                    if let failure = await decide(item, decision, client: client) {
                        model.actionError = failure
                    }
                } skip: {
                    skipping = item
                }
                if item.id != items.last?.id { Divider() }
            }
        }
        .focusable()
        .focused($isDecideFocused)
        .focusEffectDisabled()
        .overlay {
            RoundedRectangle(cornerRadius: Radius.card)
                .strokeBorder(Tone.accent.color, lineWidth: 2)
                .opacity(isDecideFocused ? 1 : 0)
                .accessibilityHidden(true)
        }
        .onKeyPress(.downArrow) {
            open(.job(KeyboardDecision.move(from: openDecisionID(items), by: 1, in: items.map(\.id)) ?? items[0].id))
            return .handled
        }
        .onKeyPress(.upArrow) {
            open(.job(KeyboardDecision.move(from: openDecisionID(items), by: -1, in: items.map(\.id)) ?? items[0].id))
            return .handled
        }
        .onKeyPress(.return) {
            open(.job(openDecisionID(items) ?? items[0].id))
            return .handled
        }
        .onKeyPress(characters: .letters, phases: .down) { press in
            guard press.modifiers.isDisjoint(with: [.command, .control, .option]),
                  let decision = press.characters.first.flatMap(KeyboardDecision.getDecision(for:)),
                  let item = items.first(where: { $0.id == openDecisionID(items) })
            else { return .ignored }
            switch decision {
            case .skip:
                skipping = item
            case .later where item.decision != nil:
                break
            case .pursue, .later:
                Task {
                    if let failure = await decide(item, decision, client: client) {
                        model.actionError = failure
                    }
                }
            }
            return .handled
        }
        .accessibilityHint("Up and down arrows move through the jobs; P pursues, L leaves for later and S skips the one open.")
    }

    /// The job to decide open in the inspector, if one is.
    private func openDecisionID(_ items: [DecisionQueueItem]) -> UUID? {
        items.first { isOpen(.job($0.id)) }?.id
    }

    /// Records the decision; when the job was open, the next one in the
    /// queue opens in its place.
    private func decide(_ item: DecisionQueueItem, _ decision: JobDecisionKind, reason: String = "", client: HubClient) async -> HubFailure? {
        let nextID = KeyboardDecision.getNextID(after: item.id, in: model.queue.map(\.id))
        let wasOpen = isOpen(.job(item.id))
        if let failure = await model.decide(item, decision, reason: reason, through: decisions, with: client) {
            return failure
        }
        if wasOpen, let nextID { open(.job(nextID)) }
        return nil
    }

    private func followUpCard(_ followUps: [DueFollowUp]) -> some View {
        let shown = followUps.prefix(Self.shownCount)
        return TodayCard("Follow up", link: followUps.count > shown.count ? "All \(followUps.count)" : "Pipeline") { openPage(.pipeline) } rows: {
            ForEach(shown) { followUp in
                let card = followUp.card
                HStack(alignment: .center, spacing: Space.m) {
                    TodayItemButton(isOpen: subject(of: card).map(isOpen) ?? false) {
                        if let subject = subject(of: card) { open(subject) }
                    } label: {
                        VStack(alignment: .leading, spacing: Space.xs) {
                            Text(card.title).fontWeight(.semibold)
                            Text(describe(card)).font(.hubSecondary).foregroundStyle(.secondary)
                            ToneChip(followUp.status)
                        }
                    }
                    Button("Followed up…", systemImage: "checkmark") {
                        followUpNote = ""
                        followingUp = card
                    }
                    .buttonStyle(.bordered)
                    .buttonBorderShape(.capsule)
                    .help("Restarts the count to the next follow-up")
                    .accessibilityLabel("Followed up on \(card.title)…")
                }
                if followUp.id != shown.last?.id { Divider() }
            }
        }
    }

    private func updatesCard(_ news: [HubUpdate], client: HubClient) -> some View {
        let shown = news.prefix(Self.shownCount)
        return TodayCard("Updates", link: "All updates") { openPage(.updates) } rows: {
            ForEach(shown) { update in
                TodayItemButton(isOpen: subject(of: update).map(isOpen) ?? false) {
                    Task { await unseen.markSeen(UpdateSelection(ids: [update.id]), with: client) }
                    if let subject = subject(of: update) { open(subject) }
                } label: {
                    HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                        UnseenDot(count: 1)
                        VStack(alignment: .leading, spacing: 2) {
                            HStack(alignment: .firstTextBaseline) {
                                Text(update.title).fontWeight(.semibold).lineLimit(1)
                                Spacer(minLength: Space.s)
                                Text(describeWhen(update.createdAt)).font(.hubCaption).foregroundStyle(.secondary)
                            }
                            if let line = getSummaryLine(update) {
                                Text(line).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(1)
                            }
                        }
                    }
                }
                .contextMenu {
                    Button("Mark as seen") { Task { await unseen.markSeen(UpdateSelection(ids: [update.id]), with: client) } }
                    if let sourceURL = update.sourceURL.flatMap(URL.init(string:)) {
                        Button("Open source") { NSWorkspace.shared.open(sourceURL) }
                    }
                }
            }
        }
    }

    private func recruitersCard(_ recruiters: [RelatedPerson], client: HubClient) -> some View {
        let shown = recruiters.prefix(Self.shownCount)
        return TodayCard("Recruiters waiting", link: recruiters.count > shown.count ? "All \(recruiters.count)" : "People") {
            openPage(.people)
        } rows: {
            ForEach(shown) { recruiter in
                HStack(alignment: .center, spacing: Space.m) {
                    TodayItemButton(isOpen: isOpen(.person(recruiter.reference))) {
                        open(.person(recruiter.reference))
                    } label: {
                        VStack(alignment: .leading, spacing: Space.xs) {
                            HStack(spacing: Space.s) {
                                Text(recruiter.name).fontWeight(.semibold)
                                ToneChip(recruiter.relationTitle, tone: .neutral)
                            }
                            Text(describe(recruiter)).font(.hubSecondary).foregroundStyle(.secondary)
                        }
                    }
                    if let conversationID = recruiter.conversationID {
                        let isDrafting = replyDraft.conversationID == conversationID && replyDraft.state == .drafting
                        AsyncButton("Draft reply", busyTitle: "Drafting…", systemImage: "square.and.pencil", isBusy: isDrafting) {
                            details.show(.person(recruiter.reference), tab: .conversation, from: .today)
                            await replyDraft.draft(conversationID: conversationID, client: client)
                        }
                        .buttonStyle(.bordered)
                        .buttonBorderShape(.capsule)
                        .help("Claude drafts a message that picks up from this conversation and names the roles that fit you at their company")
                        .accessibilityLabel("Draft a reply to \(recruiter.name)")
                    }
                }
                if recruiter.id != shown.last?.id { Divider() }
            }
        }
    }

    private func startCard(_ rows: [StartRow]) -> some View {
        TodayCard("Get started", rows: {
            ForEach(rows) { row in
                HStack(alignment: .center, spacing: Space.m) {
                    VStack(alignment: .leading, spacing: Space.xs) {
                        Text(row.title).fontWeight(.semibold)
                        Text(row.detail).font(.hubSecondary).foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    Button(row.button, systemImage: row.symbol) { start(row.destination) }
                        .buttonStyle(.bordered)
                        .buttonBorderShape(.capsule)
                }
                if row.id != rows.last?.id { Divider() }
            }
        })
    }

    /// Lands on the page a Get started row names.
    private func start(_ destination: StartDestination) {
        switch destination {
        case .profileInterview:
            openPage(.profile)
            details.show(.profileInterview, from: .profile)
        case .decide:
            openPage(.decide)
        case let .comparison(id):
            openPage(.modelLab)
            requests.ask(.openComparison(id), on: .modelLab)
        }
    }

    private var hubCard: some View {
        TodayCard("Hub", link: "Activity") { openPage(.activity) } rows: {
            // The lines' values, not indices into them: a stale index after the
            // polled list shrinks would trap.
            ForEach(Array(activity.lines.enumerated()), id: \.offset) { _, line in
                HStack(spacing: Space.s) {
                    ProgressView().controlSize(.small)
                    Text(line).lineLimit(1)
                }
            }
        }
    }

    // MARK: Opening items

    private func open(_ subject: InspectorSubject) {
        details.show(subject, from: .today)
    }

    private func isOpen(_ subject: InspectorSubject) -> Bool {
        details.getEntry(on: .today)?.subject == subject
    }

    /// A card's job, or its company for a card without one.
    private func subject(of card: PipelineCard) -> InspectorSubject? {
        card.application.jobID.map(InspectorSubject.job) ?? card.application.companyID.map(InspectorSubject.company)
    }

    private func subject(of update: HubUpdate) -> InspectorSubject? {
        update.jobID.map(InspectorSubject.job) ?? update.companyID.map(InspectorSubject.company)
    }

    // MARK: Words

    /// "Initech · Applied": the company under a job's title, and the phase.
    private func describe(_ card: PipelineCard) -> String {
        let company = card.jobTitle != nil ? card.companyName : nil
        let phase = model.board.phases.first { $0.id == card.application.phaseID }?.name
        return [company, phase].compactMap { $0 }.joined(separator: " · ")
    }

    /// "Talent Partner at Initech · 2 fitting jobs open".
    private func describe(_ recruiter: RelatedPerson) -> String {
        let fitting = recruiter.fittingJobs == 1 ? "1 fitting job open" : "\(recruiter.fittingJobs) fitting jobs open"
        let role: String? = switch (recruiter.role, recruiter.companyName) {
        case let (role?, company?): "\(role) at \(company)"
        case let (role, company): role ?? company
        }
        return [role, fitting].compactMap { $0 }.joined(separator: " · ")
    }

    /// The update's first line of text, or what it's about.
    private func getSummaryLine(_ update: HubUpdate) -> String? {
        let line = update.body?.split(separator: "\n").first { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
        return line.map { String($0) } ?? update.subject
    }

    /// The time today, "Yesterday", or the date.
    private func describeWhen(_ date: Date) -> String {
        let calendar = Calendar.current
        if calendar.isDateInToday(date) { return date.formatted(date: .omitted, time: .shortened) }
        if calendar.isDateInYesterday(date) { return "Yesterday" }
        return date.formatted(.dateTime.day().month(.abbreviated))
    }
}

/// What Today's cards list.
private struct TodayItems {
    let decisions: [DecisionQueueItem]
    let followUps: [DueFollowUp]
    let news: [HubUpdate]
    let recruiters: [RelatedPerson]
    let starts: [StartRow]
    let chips: [TodayChip]

    /// No card has anything; the Hub card's lines are read apart.
    var isEmpty: Bool { decisions.isEmpty && followUps.isEmpty && news.isEmpty && recruiters.isEmpty && starts.isEmpty }
}

/// One of Today's cards: its title, a link to the page with the rest when
/// it has one, and its rows.
private struct TodayCard<Rows: View>: View {
    let title: String
    let link: String?
    let openLink: () -> Void
    @ViewBuilder let rows: Rows

    init(_ title: String, link: String? = nil, openLink: @escaping () -> Void = {}, @ViewBuilder rows: () -> Rows) {
        self.title = title
        self.link = link
        self.openLink = openLink
        self.rows = rows()
    }

    var body: some View {
        HubSection(title) {
            VStack(alignment: .leading, spacing: Space.m) { rows }
        } trailing: {
            if let link {
                Button(link, action: openLink).buttonStyle(.link)
            }
        }
        .hubCard()
    }
}

/// An item's text, which opens it in the inspector; marked while it's open
/// there.
private struct TodayItemButton<Content: View>: View {
    let isOpen: Bool
    let open: () -> Void
    @ViewBuilder let label: Content

    var body: some View {
        Button(action: open) {
            label
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(Space.xs)
        .background(isOpen ? AnyShapeStyle(Tone.accent.fill) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: Radius.control))
        .padding(-Space.xs)
    }
}

/// A job to decide: its match, title, company and the brief's reason, with
/// Pursue, Later and Skip… under it.
private struct TodayDecisionRow: View {
    let item: DecisionQueueItem
    let isOpen: Bool
    let open: () -> Void
    let decide: @MainActor (JobDecisionKind) async -> Void
    let skip: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            TodayItemButton(isOpen: isOpen, open: open) {
                VStack(alignment: .leading, spacing: Space.xs) {
                    HStack(spacing: Space.s) {
                        ToneChip(item.match)
                        Text(item.job.title).fontWeight(.semibold).lineLimit(1)
                        if item.decision != nil {
                            ToneChip("Later", tone: .neutral, symbol: "clock")
                        }
                    }
                    let facts = [item.companyName, item.job.location].compactMap { $0 }.filter { !$0.isEmpty }
                    if !facts.isEmpty {
                        Text(facts.joined(separator: " · ")).font(.hubSecondary).foregroundStyle(.secondary)
                    }
                    Text(item.reason).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(2)
                }
            }
            HStack(spacing: Space.s) {
                AsyncButton("Pursue", busyTitle: "Pursuing…", systemImage: "arrow.up.forward") { await decide(.pursue) }
                    .help("Put it on the pipeline")
                    .accessibilityLabel("Pursue \(item.job.title)")
                if item.decision == nil {
                    AsyncButton("Later", busyTitle: "Later", systemImage: "clock") { await decide(.later) }
                        .help("Leave it for another day")
                        .accessibilityLabel("Leave \(item.job.title) for later")
                }
                Button("Skip…", systemImage: SetAside.skipped.symbolName, action: skip)
                    .help("Take it out as not for you, with a reason")
                    .accessibilityLabel("Skip \(item.job.title)…")
            }
            .buttonStyle(.bordered)
            .buttonBorderShape(.capsule)
        }
    }
}
