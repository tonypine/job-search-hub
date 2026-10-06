import AppKit
import JobSearchHubCore
import SwiftUI

/// What the Pipeline page shows: the board's open phases, only their cards
/// whose follow-up is due, the closed cards, or the skipped cards.
enum PipelineScope: String, CaseIterable, Identifiable {
    case active, due, closed, skipped

    var id: String { rawValue }
    var title: String { rawValue.capitalized }
}

@MainActor
@Observable
final class PipelineModel {
    private(set) var activeBoard = PipelineBoard()
    private(set) var skippedBoard = PipelineBoard()
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    private(set) var movingCardID: UUID?
    /// Why the last change to a card failed: a move, a follow-up, a skip.
    var actionError: HubFailure?
    var scope = PipelineScope.active
    /// Everyone who can get the owner in, read while a card waits for a
    /// second route; nil until read.
    private(set) var people: [RelatedPerson]?
    /// The company research this page started to find people, while it runs.
    var researchedCompanyID: UUID?
    /// Who the owner last drafted a message to about a card, for the note of
    /// its follow-up.
    private(set) var draftedTo: [UUID: String] = [:]

    /// The board the scope shows: the skipped cards, or the active ones.
    var board: PipelineBoard { scope == .skipped ? skippedBoard : activeBoard }

    /// The phases the board shows as columns: the open ones, with Closed
    /// folded into a drawer at the end, or every phase for the skipped cards.
    var columns: [PipelinePhase] { scope == .skipped ? skippedBoard.phases : activeBoard.openPhases }

    /// The phase the drawer at the board's end closes a card into; nil on
    /// the skipped cards' board and on a board without one.
    var drawerPhase: PipelinePhase? { scope == .skipped ? nil : activeBoard.closedPhase }

    /// The cards that ended, for the Closed scope and the drawer's count.
    var closedCards: [PipelineCard] { activeBoard.closedCards }

    /// The board's cards for a phase, only the due ones in the Due scope.
    func getShownCards(in phase: PipelinePhase) -> [PipelineCard] {
        let cards = board.getCards(in: phase)
        guard scope == .due else { return cards }
        return cards.filter { $0.getFollowUpStatus(now: .now)?.isDue == true }
    }

    /// How many cards each scope holds.
    func getCount(_ scope: PipelineScope) -> Int {
        switch scope {
        case .active: activeBoard.openCards.count
        case .due: activeBoard.getDueCount(now: .now)
        case .closed: activeBoard.closedCards.count
        case .skipped: skippedBoard.cards.count
        }
    }

    /// Records a follow-up, then reads the board again for its new due date.
    func recordFollowUp(_ cardID: UUID, note: String, with client: HubClient) async {
        do {
            _ = try await client.send("POST", "v1/applications/\(cardID.uuidString)/follow-ups", body: FollowUpRequest(note: note), as: ApplicationResponse.self)
            draftedTo[cardID] = nil
            await load(with: client)
        } catch {
            actionError = HubFailure("Couldn't record the follow-up", error)
        }
    }

    /// Skips the card, taking it off the board without closing it, then reads
    /// the board again.
    func skip(_ cardID: UUID, note: String, with client: HubClient) async {
        do {
            _ = try await client.dismissApplication(cardID, note: note)
            await load(with: client)
        } catch {
            actionError = HubFailure("Couldn't skip the card", error)
        }
    }

    /// Puts the card back in the phase it left, then reads the skipped cards again.
    func restore(_ cardID: UUID, with client: HubClient) async {
        do {
            _ = try await client.restoreApplication(cardID)
            await load(with: client)
        } catch {
            actionError = HubFailure("Couldn't restore the card", error)
        }
    }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            // Both, so every scope's count is right whichever shows.
            async let active = client.getPipeline()
            async let skipped = client.getDismissedPipeline()
            (activeBoard, skippedBoard) = try await (PipelineBoard(active), PipelineBoard(skipped))
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the pipeline", error)
        }
        guard scope != .skipped && !activeBoard.getUnansweredPastFollowUp(now: .now).isEmpty else { return }
        do {
            people = try await client.get("v1/people", as: PeopleResponse.self).people
        } catch {
            actionError = HubFailure("Couldn't read who to write to", error)
        }
    }

    /// Who to write to next about each card nobody answered whose follow-up
    /// is overdue, best first, by card; empty for a card whose company has
    /// nobody on file. No card has any until the people are read, and the
    /// skipped cards never do.
    func getSecondRoutes(now: Date) -> [UUID: [RelatedPerson]] {
        guard scope != .skipped, let people else { return [:] }
        let unanswered = activeBoard.getUnansweredPastFollowUp(now: now)
        var routes: [UUID: [RelatedPerson]] = [:]
        for card in activeBoard.cards where unanswered.contains(card.id) && card.getStatus(now: now)?.isOverdue == true {
            guard let companyID = card.application.companyID else { continue }
            routes[card.id] = SecondRoute.getCandidates(companyID: companyID, among: people)
        }
        return routes
    }

    /// The outreach_draft prompt aimed at the person, about the card.
    func makeDraftRequest(to person: RelatedPerson, about card: PipelineCard, with client: HubClient) async -> String? {
        do {
            let prompt = try await client.get("v1/agent-prompts/outreach_draft", as: AgentPrompt.self).body
            return SecondRoute.makeDraftRequest(prompt: prompt, to: person, about: card)
        } catch {
            actionError = HubFailure("Couldn't read the outreach prompt", error)
            return nil
        }
    }

    /// Asks the card's session for the draft to the person: typed into the
    /// running one, or the first message of its latest resumed, or of a new
    /// one. The page sends it rather than the inspector, so the Session tab
    /// only shows the session and nothing it lays out changes with the
    /// request. Only a draft the session got names the person in the card's
    /// next follow-up.
    func send(_ request: String, to subject: ClaudeSessionSubject, writingTo person: RelatedPerson, about cardID: UUID, with client: HubClient) async {
        let session = ClaudeSessionPaneModel()
        await session.load(subject, with: client)
        if session.failure == nil {
            await session.send(request, about: subject, with: client, host: .shared)
        }
        if let failure = session.failure {
            actionError = failure
        } else {
            draftedTo[cardID] = person.name
        }
    }

    /// Researches the card's company again, the agent that finds who to
    /// reach there; the page reads the people once it ends.
    func findPeople(at companyID: UUID, with client: HubClient, research: CompanyResearch) async {
        guard !research.isRunning else { return }
        do {
            let dossier = try await client.get("v1/companies/\(companyID.uuidString)", as: CompanyDossier.self)
            // From idle, a run that fails at once still shows as a change.
            research.reset()
            researchedCompanyID = companyID
            research.start(company: dossier.company.domain, foundVia: "", client: client)
        } catch {
            actionError = HubFailure("Couldn't find people", error)
        }
    }

    /// Saves the move, then places the card from the server's answer. A move
    /// the server refuses leaves the card where it was.
    func move(_ cardID: UUID, to phase: PipelinePhase, closedReason: String?, with client: HubClient) async {
        guard movingCardID == nil, let card = board.cards.first(where: { $0.id == cardID }), card.application.phaseID != phase.id else { return }
        movingCardID = cardID
        defer { movingCardID = nil }
        do {
            let response = try await client.send(
                "PATCH", "v1/applications/\(cardID.uuidString)",
                body: MoveApplicationRequest(phaseID: phase.id, closedReason: closedReason), as: ApplicationResponse.self
            )
            if scope == .skipped {
                skippedBoard.replaceApplication(response.application)
            } else {
                activeBoard.replaceApplication(response.application)
            }
        } catch {
            actionError = HubFailure("Couldn't move the card", error)
        }
    }
}

/// A move into a closed phase, waiting for the reason the application ended.
private struct PendingClose {
    let cardID: UUID
    let phase: PipelinePhase
}

struct PipelinePage: View {
    private static let columnSpacing = Space.m
    private static let boardPadding = Space.l
    private static let columnWidthRange: ClosedRange<CGFloat> = 180...320
    private static let closedCardWidthRange: ClosedRange<CGFloat> = 220...320

    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @Environment(DetailsInspector.self) private var details
    @Environment(JobDecisions.self) private var decisions
    @Environment(CompanyResearch.self) private var research
    @State private var model = PipelineModel()
    @State private var pendingClose: PendingClose?
    @State private var closedReason = ""
    @State private var selectedCardID: UUID?
    @State private var followUpCardID: UUID?
    @State private var followUpNote = ""
    @State private var skippingCardID: UUID?
    @State private var skipNote = ""
    /// A job whose card is selected once the board loads.
    let initialJobID: UUID?

    init(initialJobID: UUID? = nil) {
        self.initialJobID = initialJobID
    }

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                VStack(spacing: 0) {
                    header
                    content(client: client)
                }
                .task {
                    await model.load(with: client)
                    if let initialJobID, selectedCardID == nil {
                        selectedCardID = model.board.cards.first { $0.application.jobID == initialJobID }?.id
                    }
                }
                .onChange(of: [events.revision, unseen.revision, decisions.revision]) { Task { await model.load(with: client) } }
                .onChange(of: model.scope) {
                    selectedCardID = nil
                    // The people to write to are read for the active cards.
                    if model.scope != .skipped { Task { await model.load(with: client) } }
                }
                .onChange(of: selectedCardSubject, initial: true) { details.show(selectedCardSubject, from: .pipeline) }
                .onChange(of: details.getEntry(on: .pipeline)) {
                    if details.getEntry(on: .pipeline) == nil { selectedCardID = nil }
                }
                .onChange(of: research.state) { finishFindingPeople(with: client) }
            }
        }
        .navigationTitle("Pipeline")
        .navigationSubtitle(describeCount())
    }

    private func describeCount() -> String {
        switch model.scope {
        case .skipped:
            let count = model.getCount(.skipped)
            return count == 1 ? "1 skipped" : "\(count) skipped"
        case .closed:
            let count = model.getCount(.closed)
            return count == 1 ? "1 closed" : "\(count) closed"
        case .active, .due:
            let active = "\(model.getCount(.active)) active"
            guard let contacts = model.board.getContactTally(now: .now).text else { return active }
            return "\(active) · \(contacts)"
        }
    }

    /// The page's controls, in its header over the board rather than in
    /// the window's toolbar, which reaches over the details inspector: the
    /// board, its cards whose follow-up is due, the closed cards, or the
    /// skipped cards.
    private var header: some View {
        PageHeader {
            TabStrip(
                items: PipelineScope.allCases.map { scope in TabStripItem(id: scope, title: scope.title, count: model.getCount(scope)) },
                selection: $model.scope
            )
        } trailing: {
            EmptyView()
        }
    }

    /// The board, or the closed cards in the Closed scope, with the
    /// questions a card's actions ask and the errors they meet.
    private func content(client: HubClient) -> some View {
        Group {
            if model.scope == .closed {
                closedList(client: client)
            } else {
                board(client: client)
            }
        }
        .alert("Followed up", isPresented: Binding(get: { followUpCardID != nil }, set: { if !$0 { followUpCardID = nil } })) {
            TextField("What you did", text: $followUpNote)
            Button("Record") {
                guard let cardID = followUpCardID else { return }
                Task { await model.recordFollowUp(cardID, note: followUpNote, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Restarts the count to the next follow-up.")
        }
        .overlay {
            if let loadError = model.loadError {
                HubErrorView(loadError, style: .page) { Task { await model.load(with: client) } }
            }
        }
        .overlay(alignment: .bottom) {
            if model.actionError != nil {
                HubErrorView($model.actionError)
                    .frame(maxWidth: 560)
                    .padding(Space.l)
            }
        }
        .alert("Skip the application", isPresented: Binding(get: { skippingCardID != nil }, set: { if !$0 { skippingCardID = nil } })) {
            TextField("Reason (optional)", text: $skipNote)
            Button("Skip") {
                guard let cardID = skippingCardID else { return }
                Task { await model.skip(cardID, note: skipNote, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The card leaves the board without closing, and its job leaves the Jobs list. Restore it from Skipped.")
        }
        .alert("Close the application", isPresented: Binding(get: { pendingClose != nil }, set: { if !$0 { pendingClose = nil } })) {
            TextField("Reason", text: $closedReason)
            Button("Close") {
                guard let pendingClose else { return }
                let reason = closedReason.trimmingCharacters(in: .whitespacesAndNewlines)
                Task { await model.move(pendingClose.cardID, to: pendingClose.phase, closedReason: reason.isEmpty ? nil : reason, with: client) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Why did it end? The reason stays with the card, in its tooltip and details.")
        }
    }

    /// What a card's click, drag and menu do, the same on the board and in
    /// the Closed scope.
    private func makeCardActions(with client: HubClient) -> PipelineCardActions {
        PipelineCardActions(
            phases: model.board.phases, movingCardID: model.movingCardID, selectedCardID: $selectedCardID,
            onFollowUp: { cardID in
                followUpNote = model.draftedTo[cardID].map { "Wrote to \($0)" } ?? ""
                followUpCardID = cardID
            },
            onSkip: { cardID in
                skipNote = ""
                skippingCardID = cardID
            },
            onRestore: { cardID in Task { await model.restore(cardID, with: client) } },
            onMove: { cardID, target in requestMove(cardID, to: target, with: client) }
        )
    }

    private func board(client: HubClient) -> some View {
        GeometryReader { geometry in
            let columns = model.columns
            let drawerPhase = model.drawerPhase
            let columnWidth = getColumnWidth(availableWidth: geometry.size.width, columnCount: columns.count, hasDrawer: drawerPhase != nil)
            let secondRoutes = makeSecondRoutes(with: client)
            let actions = makeCardActions(with: client)
            ScrollView(.horizontal) {
                HStack(alignment: .top, spacing: Self.columnSpacing) {
                    ForEach(columns) { phase in
                        PipelineColumn(
                            phase: phase, cards: model.getShownCards(in: phase), dueTally: model.board.getDueTally(in: phase, now: .now),
                            emptyText: getEmptyText(for: phase), width: columnWidth, secondRoutes: secondRoutes, actions: actions
                        )
                    }
                    if let drawerPhase {
                        ClosedDrawer(count: model.getCount(.closed), isEnabled: model.movingCardID == nil) {
                            model.scope = .closed
                        } onDrop: { cardID in
                            requestMove(cardID, to: drawerPhase, with: client)
                        }
                    }
                }
                .padding(Self.boardPadding)
                .frame(height: geometry.size.height, alignment: .top)
            }
        }
    }

    /// The closed cards, side by side in as many columns as fit, newest
    /// first. A card goes back on the board from its menu.
    private func closedList(client: HubClient) -> some View {
        let cards = model.closedCards
        let actions = makeCardActions(with: client)
        return ScrollView(.vertical) {
            if cards.isEmpty {
                Text("No application has ended. Drop a card on Closed, at the board's end, when one does.")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
                    .padding(Space.xxl)
            } else {
                LazyVGrid(
                    columns: [GridItem(.adaptive(minimum: Self.closedCardWidthRange.lowerBound, maximum: Self.closedCardWidthRange.upperBound), spacing: Self.columnSpacing, alignment: .top)],
                    alignment: .leading, spacing: Self.columnSpacing
                ) {
                    ForEach(cards) { card in
                        PipelineCardItem(card: card, secondRoute: nil, actions: actions)
                    }
                }
                .padding(Self.boardPadding)
            }
        }
    }

    /// What an empty column says: what goes there, or that nothing in it is
    /// due or skipped.
    private func getEmptyText(for phase: PipelinePhase) -> String {
        switch model.scope {
        case .due: "Nothing due"
        case .skipped: "Nothing skipped"
        case .active, .closed: phase.emptyHint
        }
    }

    /// The selected card's details.
    private var selectedCardSubject: InspectorSubject? {
        model.board.cards.first { $0.id == selectedCardID }.flatMap(getSubject)
    }

    /// A card's details: its job's, or its company's for a card without one.
    private func getSubject(of card: PipelineCard) -> InspectorSubject? {
        if let jobID = card.application.jobID {
            return .job(jobID)
        }
        return card.application.companyID.map(InspectorSubject.company)
    }

    private func makeSecondRoutes(with client: HubClient) -> SecondRoutes {
        SecondRoutes(
            candidates: model.getSecondRoutes(now: .now),
            findingPeopleCompanyID: research.isRunning ? model.researchedCompanyID : nil,
            canFindPeople: !research.isRunning,
            onWrite: { card, person in write(to: person, about: card, with: client) },
            onFindPeople: { card in
                guard let companyID = card.application.companyID else { return }
                Task { await model.findPeople(at: companyID, with: client, research: research) }
            }
        )
    }

    /// Selects the card and opens its session on a message to the person,
    /// drafted with the outreach prompt for the owner to send.
    private func write(to person: RelatedPerson, about card: PipelineCard, with client: HubClient) {
        guard let subject = getSubject(of: card), let session = subject.sessionSubject else { return }
        Task {
            guard let request = await model.makeDraftRequest(to: person, about: card, with: client) else { return }
            // Sent before the tab shows, so the pane finds the session running.
            await model.send(request, to: session, writingTo: person, about: card.id, with: client)
            selectedCardID = card.id
            details.show(subject, tab: .session, from: .pipeline)
        }
    }

    /// Once the research this page started ends, reads the people again, or
    /// says why it failed, and leaves the research ready for the next one.
    private func finishFindingPeople(with client: HubClient) {
        guard model.researchedCompanyID != nil, !research.isRunning else { return }
        if case let .failed(reason) = research.state {
            model.actionError = HubFailure("Couldn't find people", advice: reason)
        }
        model.researchedCompanyID = nil
        research.reset()
        Task { await model.load(with: client) }
    }

    /// Shares the width left of the Closed drawer among the columns so every
    /// phase shows at once, down to a readable minimum; below it the board
    /// scrolls sideways.
    private func getColumnWidth(availableWidth: CGFloat, columnCount: Int, hasDrawer: Bool) -> CGFloat {
        let count = CGFloat(max(columnCount, 1))
        let drawerWidth = hasDrawer ? ClosedDrawer.width + Self.columnSpacing : 0
        let widthForColumns = availableWidth - Self.boardPadding * 2 - Self.columnSpacing * (count - 1) - drawerWidth
        return min(max(widthForColumns / count, Self.columnWidthRange.lowerBound), Self.columnWidthRange.upperBound)
    }

    private func requestMove(_ cardID: UUID, to phase: PipelinePhase, with client: HubClient) {
        if phase.isClosed {
            closedReason = ""
            pendingClose = PendingClose(cardID: cardID, phase: phase)
        } else {
            Task { await model.move(cardID, to: phase, closedReason: nil, with: client) }
        }
    }
}

/// What a card's click, drag and menu do: select it, carry it to another
/// phase, follow up, close, skip or restore it.
struct PipelineCardActions {
    let phases: [PipelinePhase]
    let movingCardID: UUID?
    let selectedCardID: Binding<UUID?>
    let onFollowUp: (UUID) -> Void
    let onSkip: (UUID) -> Void
    let onRestore: (UUID) -> Void
    let onMove: (UUID, PipelinePhase) -> Void
}

struct PipelineColumn: View {
    let phase: PipelinePhase
    let cards: [PipelineCard]
    /// The follow-ups due among the phase's cards, for the header's chip.
    let dueTally: DueTally
    /// What the column says while it has no cards.
    let emptyText: String
    let width: CGFloat
    let secondRoutes: SecondRoutes
    let actions: PipelineCardActions
    @State private var isTargeted = false

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            header
            if cards.isEmpty {
                Text(emptyText)
                    .font(.hubCaption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
                    .padding(Space.m)
                    .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(.separator, style: StrokeStyle(lineWidth: 1, dash: [4, 3])))
            } else {
                ScrollView(.vertical) {
                    LazyVStack(spacing: Space.s) {
                        ForEach(cards) { card in
                            let secondRoute = secondRoutes.makeRow(for: card) { actions.onFollowUp(card.id) }
                            PipelineCardItem(card: card, secondRoute: secondRoute, actions: actions)
                        }
                    }
                }
            }
        }
        .padding(Space.s)
        .frame(width: width)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(isTargeted ? AnyShapeStyle(Tone.accent.fill) : AnyShapeStyle(.quinary), in: RoundedRectangle(cornerRadius: Radius.card))
        .dropDestination(for: String.self) { items, _ in
            guard let cardID = items.first.flatMap(UUID.init(uuidString:)) else { return false }
            actions.onMove(cardID, phase)
            return true
        } isTargeted: { isTargeted = $0 }
    }

    /// The phase, its count, and a chip when something in it is due: red
    /// while one is overdue.
    private var header: some View {
        HStack(spacing: Space.s) {
            Text(phase.name).font(.hubSection).lineLimit(1)
            Text("\(cards.count)").monospacedDigit().foregroundStyle(.secondary)
            Spacer(minLength: 0)
            if let dueText = dueTally.text {
                ToneChip(dueText, tone: dueTally.tone, symbol: "bell.fill")
                    .help(describeDue())
            }
        }
        .frame(height: 22)
    }

    private func describeDue() -> String {
        let dueToday = dueTally.due - dueTally.overdue
        return [
            dueTally.overdue > 0 ? "\(dueTally.overdue) overdue" : nil,
            dueToday > 0 ? "\(dueToday) due today" : nil,
        ].compactMap { $0 }.joined(separator: ", ")
    }
}

/// Closed, folded into a narrow drawer at the board's end: its count, a
/// click lists the closed cards, and a card dropped on it closes, asking
/// why it ended.
struct ClosedDrawer: View {
    static let width: CGFloat = 44

    let count: Int
    /// False while a card is moving, since one moves at a time.
    let isEnabled: Bool
    let onOpen: () -> Void
    let onDrop: (UUID) -> Void
    @State private var isTargeted = false

    var body: some View {
        Button(action: onOpen) {
            VStack(spacing: Space.s) {
                Image(systemName: SetAside.closed.symbolName)
                    .foregroundStyle(.secondary)
                Text("Closed")
                    .font(.hubSection)
                    .fixedSize()
                    .rotationEffect(.degrees(90))
                    .frame(width: 20, height: 56)
                Text("\(count)")
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                Spacer(minLength: 0)
            }
            .padding(.vertical, Space.m)
            .frame(width: Self.width)
            .frame(maxHeight: .infinity)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .background(isTargeted ? AnyShapeStyle(Tone.accent.fill) : AnyShapeStyle(.quinary), in: RoundedRectangle(cornerRadius: Radius.card))
        .dropDestination(for: String.self) { items, _ in
            guard isEnabled, let cardID = items.first.flatMap(UUID.init(uuidString:)) else { return false }
            onDrop(cardID)
            return true
        } isTargeted: { isTargeted = $0 }
        .help("Drop a card here to close it, with the reason it ended. Click to list the closed cards.")
        .accessibilityLabel(count == 1 ? "Closed, 1 card" : "Closed, \(count) cards")
    }
}

/// A card with what a click, a drag and its menu do.
struct PipelineCardItem: View {
    let card: PipelineCard
    /// Who to write to next, for an overdue card nobody answered.
    let secondRoute: SecondRouteRow?
    let actions: PipelineCardActions

    var body: some View {
        PipelineCardView(
            card: card, isMoving: card.id == actions.movingCardID, isSelected: card.id == actions.selectedCardID.wrappedValue,
            secondRoute: secondRoute
        )
        .onTapGesture { actions.selectedCardID.wrappedValue = card.id }
        .draggable(card.id.uuidString)
        .contextMenu {
            if let jobURL = card.jobURL.flatMap(URL.init(string:)) {
                Button("Open posting") { NSWorkspace.shared.open(jobURL) }
            }
            if card.dismissedAt != nil {
                Button("Restore") { actions.onRestore(card.id) }
            } else {
                Button("Followed up…") { actions.onFollowUp(card.id) }
                Menu("Move to") {
                    ForEach(actions.phases.filter { $0.id != card.application.phaseID }) { target in
                        Button(target.name) { actions.onMove(card.id, target) }
                    }
                }
                .disabled(actions.movingCardID != nil)
                Divider()
                // Ends the application with an outcome; Skip takes it out as not for you.
                if let closedPhase = actions.phases.first(where: \.isClosed), closedPhase.id != card.application.phaseID {
                    Button("Close…") { actions.onMove(card.id, closedPhase) }
                        .disabled(actions.movingCardID != nil)
                }
                Button("Skip…") { actions.onSkip(card.id) }
            }
        }
    }
}

/// A card says one thing at most per line: the company with its age in the
/// phase, the job, the most urgent of its follow-up and its reply, and, on
/// an overdue card nobody answered, who to write to next. A due card is
/// edged in its status's tone. Its notes and the reasons it closed or was
/// skipped wait in its tooltip and the inspector.
struct PipelineCardView: View {
    private static let edgeWidth: CGFloat = 3

    let card: PipelineCard
    let isMoving: Bool
    let isSelected: Bool
    /// Who to write to next, for an overdue card nobody answered.
    var secondRoute: SecondRouteRow?

    var body: some View {
        let status = card.getStatus(now: .now)
        VStack(alignment: .leading, spacing: Space.xs) {
            HStack(spacing: Space.s) {
                CardMonogram(name: companyName)
                Text(companyName).font(.hubSecondary).foregroundStyle(.secondary).lineLimit(1)
                Spacer(minLength: 0)
                if card.unseenUpdates > 0 {
                    UnseenDot(count: card.unseenUpdates)
                }
                if isMoving {
                    ProgressView().controlSize(.small)
                }
                age
            }
            Text(card.jobTitle ?? "Outreach, no posting").fontWeight(.semibold).lineLimit(2)
            if let status {
                Label(describe(status), systemImage: status.symbolName)
                    .font(.hubCaption.weight(.medium))
                    .foregroundStyle(status.tone.color)
                    .lineLimit(1)
            }
            if let secondRoute {
                secondRoute
            }
        }
        .padding(Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background)
        .overlay(alignment: .leading) {
            if let status, status.isDue {
                Rectangle().fill(status.tone.color).frame(width: Self.edgeWidth)
            }
        }
        .clipShape(RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(isSelected ? AnyShapeStyle(Tone.accent.color) : AnyShapeStyle(.separator), lineWidth: isSelected ? 2 : 1))
        .opacity(isMoving ? 0.6 : 1)
        .help(card.tooltip)
    }

    private var companyName: String { card.companyName ?? "No company" }

    /// Days in the phase, in the corner: "12 d".
    private var age: some View {
        let days = card.getDaysInPhase(now: .now)
        let text = switch days {
        case 0: "Entered today"
        case 1: "1 day in phase"
        default: "\(days) days in phase"
        }
        return Text("\(days) d")
            .font(.hubCaption)
            .monospacedDigit()
            .foregroundStyle(.secondary)
            .fixedSize()
            .help(text)
            .accessibilityLabel(text)
    }

    private func describe(_ status: PipelineCardStatus) -> String {
        switch status {
        case let .followUp(followUp): followUp.text
        case let .heardBack(contactedAt): "Heard back \(contactedAt.formatted(.dateTime.day().month(.abbreviated)))"
        }
    }
}

/// When a person at the company first wrote back about an application.
struct HeardBackLabel: View {
    let contactedAt: Date

    var body: some View {
        Label("Heard back \(contactedAt.formatted(date: .abbreviated, time: .omitted))", systemImage: "arrowshape.turn.up.left")
            .font(.hubCaption)
            .foregroundStyle(Tone.positive.color)
    }
}

/// What the board needs to offer each overdue card nobody answered its
/// next route.
@MainActor
struct SecondRoutes {
    /// Who to write to, best first, by card; empty for a card whose company
    /// has nobody on file.
    var candidates: [UUID: [RelatedPerson]] = [:]
    /// The company whose people are being found.
    var findingPeopleCompanyID: UUID?
    /// False while any company research runs, since one runs at a time.
    var canFindPeople = true
    var onWrite: (PipelineCard, RelatedPerson) -> Void = { _, _ in }
    var onFindPeople: (PipelineCard) -> Void = { _ in }

    func makeRow(for card: PipelineCard, onFollowUp: @escaping () -> Void) -> SecondRouteRow? {
        guard let people = candidates[card.id] else { return nil }
        return SecondRouteRow(
            candidates: people,
            isFindingPeople: card.application.companyID != nil && card.application.companyID == findingPeopleCompanyID,
            canFindPeople: canFindPeople,
            onWrite: { onWrite(card, $0) }, onFindPeople: { onFindPeople(card) }, onFollowUp: onFollowUp
        )
    }
}

/// An overdue card's next route, on one line: write to someone at the
/// company or someone who can introduce you (the others in its menu), or
/// find who to write to, then ✓ records the follow-up.
struct SecondRouteRow: View {
    let candidates: [RelatedPerson]
    let isFindingPeople: Bool
    let canFindPeople: Bool
    let onWrite: (RelatedPerson) -> Void
    let onFindPeople: () -> Void
    let onFollowUp: () -> Void

    var body: some View {
        HStack(spacing: Space.s) {
            if let first = candidates.first {
                Menu {
                    ForEach(candidates) { person in
                        Button(SecondRoute.getMenuTitle(for: person)) { onWrite(person) }
                            .help(person.whatTheyCanDo)
                    }
                } label: {
                    Label("Write to \(first.name)", systemImage: "paperplane").lineLimit(1)
                } primaryAction: {
                    onWrite(first)
                }
                .menuStyle(.borderlessButton)
                .menuIndicator(.visible)
                .help("Draft a message to \(first.name) with the outreach prompt, or pick someone else; you send it yourself. \(first.whatTheyCanDo)")
            } else if isFindingPeople {
                ProgressView().controlSize(.small)
                Text("Finding people…").foregroundStyle(.secondary).lineLimit(1)
            } else {
                Button("Find people", systemImage: "person.2") { onFindPeople() }
                    .buttonStyle(.link)
                    .lineLimit(1)
                    .disabled(!canFindPeople)
                    .help(findPeopleHelp)
            }
            Spacer(minLength: 0)
            Button("Followed up…", systemImage: "checkmark") { onFollowUp() }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
                .help("Followed up…: record the message you sent; it restarts the count to the next follow-up")
        }
        .font(.hubCaption)
    }

    private var findPeopleHelp: String {
        canFindPeople
            ? "Nobody at the company is on file: an agent researches it for who to reach there, in a few minutes"
            : "Another company's research is running"
    }
}
