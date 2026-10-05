import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PipelineModel {
    private(set) var board = PipelineBoard()
    private(set) var isLoading = false
    private(set) var loadError: HubFailure?
    private(set) var movingCardID: UUID?
    /// Why the last change to a card failed: a move, a follow-up, a skip.
    var actionError: HubFailure?
    var showsOnlyDue = false
    /// Shows the skipped cards instead of the board.
    var showsSkipped = false
    /// Everyone who can get the owner in, read while a card waits for a
    /// second route; nil until read.
    private(set) var people: [RelatedPerson]?
    /// The company research this page started to find people, while it runs.
    var researchedCompanyID: UUID?
    /// Who the owner last drafted a message to about a card, for the note of
    /// its follow-up.
    private(set) var draftedTo: [UUID: String] = [:]

    /// The board's cards for a phase, only the due ones when asked.
    func getShownCards(in phase: PipelinePhase) -> [PipelineCard] {
        let cards = board.getCards(in: phase)
        guard showsOnlyDue else { return cards }
        return cards.filter { $0.getFollowUpStatus(now: .now)?.isDue == true }
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
            board = PipelineBoard(try await showsSkipped ? client.getDismissedPipeline() : client.getPipeline())
            loadError = nil
        } catch {
            loadError = HubFailure("Couldn't load the pipeline", error)
        }
        if !showsSkipped && !board.getUnansweredPastFollowUp(now: .now).isEmpty,
           let listed = try? await client.get("v1/people", as: PeopleResponse.self).people {
            people = listed
        }
    }

    /// Who to write to next about each card nobody answered past its
    /// follow-up, best first, by card; empty for a card whose company has
    /// nobody on file. No card has any until the people are read, and the
    /// skipped cards never do.
    func getSecondRoutes(now: Date) -> [UUID: [RelatedPerson]] {
        guard !showsSkipped, let people else { return [:] }
        let unanswered = board.getUnansweredPastFollowUp(now: now)
        var routes: [UUID: [RelatedPerson]] = [:]
        for card in board.cards where unanswered.contains(card.id) {
            guard let companyID = card.application.companyID else { continue }
            routes[card.id] = SecondRoute.getCandidates(companyID: companyID, among: people)
        }
        return routes
    }

    /// The outreach_draft prompt aimed at the person, about the card.
    func makeDraftRequest(to person: RelatedPerson, about card: PipelineCard, with client: HubClient) async -> String? {
        do {
            let prompt = try await client.get("v1/agent-prompts/outreach_draft", as: AgentPrompt.self).body
            draftedTo[card.id] = person.name
            return SecondRoute.makeDraftRequest(prompt: prompt, to: person, about: card)
        } catch {
            actionError = HubFailure("Couldn't read the outreach prompt", error)
            return nil
        }
    }

    /// Asks the card's session for the draft: typed into the running one, or
    /// the first message of its latest resumed, or of a new one. The page
    /// sends it rather than the inspector, so the Session tab only shows the
    /// session and nothing it lays out changes with the request.
    func send(_ request: String, to subject: ClaudeSessionSubject, with client: HubClient) async {
        let session = ClaudeSessionPaneModel()
        await session.load(subject, with: client)
        if session.failure == nil {
            await session.send(request, about: subject, with: client, host: .shared)
        }
        if let failure = session.failure {
            actionError = failure
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
            board.replaceApplication(response.application)
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
                board(client: client)
                    .task {
                        await model.load(with: client)
                        if let initialJobID, selectedCardID == nil {
                            selectedCardID = model.board.cards.first { $0.application.jobID == initialJobID }?.id
                        }
                    }
                    .onChange(of: [events.revision, unseen.revision, decisions.revision]) { Task { await model.load(with: client) } }
                    .onChange(of: model.showsSkipped) {
                        selectedCardID = nil
                        Task { await model.load(with: client) }
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
        let count = model.board.cards.count
        if model.showsSkipped {
            return count == 1 ? "1 skipped" : "\(count) skipped"
        }
        let applications = count == 1 ? "1 application" : "\(count) applications"
        guard let contacts = model.board.getContactTally(now: .now).text else { return applications }
        return "\(applications) · \(contacts)"
    }

    private func board(client: HubClient) -> some View {
        GeometryReader { geometry in
            let columnWidth = getColumnWidth(availableWidth: geometry.size.width)
            let secondRoutes = makeSecondRoutes(with: client)
            ScrollView(.horizontal) {
                HStack(alignment: .top, spacing: Self.columnSpacing) {
                    ForEach(model.board.phases) { phase in
                        PipelineColumn(
                            phase: phase, cards: model.getShownCards(in: phase), phases: model.board.phases,
                            movingCardID: model.movingCardID, width: columnWidth, selectedCardID: $selectedCardID,
                            secondRoutes: secondRoutes,
                            onFollowUp: { cardID in
                                followUpNote = model.draftedTo[cardID].map { "Wrote to \($0)" } ?? ""
                                followUpCardID = cardID
                            },
                            onSkip: { cardID in
                                skipNote = ""
                                skippingCardID = cardID
                            },
                            onRestore: { cardID in Task { await model.restore(cardID, with: client) } }
                        ) { cardID, target in
                            requestMove(cardID, to: target, with: client)
                        }
                    }
                }
                .padding(Self.boardPadding)
                .frame(height: geometry.size.height, alignment: .top)
            }
        }
        .toolbar {
            Toggle("Due only", systemImage: "bell.badge", isOn: $model.showsOnlyDue)
                .help("Show only the cards whose follow-up is due")
            Toggle("Skipped", systemImage: SetAside.skipped.symbolName, isOn: $model.showsSkipped)
                .help("Show the skipped cards, where they can be restored")
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
            Text("Why did it end? The reason stays on the card.")
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
            await model.send(request, to: session, with: client)
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

    /// Shares the width among the columns so every phase shows at once, down
    /// to a readable minimum; below it the board scrolls sideways.
    private func getColumnWidth(availableWidth: CGFloat) -> CGFloat {
        let count = CGFloat(max(model.board.phases.count, 1))
        let widthForColumns = availableWidth - Self.boardPadding * 2 - Self.columnSpacing * (count - 1)
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

struct PipelineColumn: View {
    let phase: PipelinePhase
    let cards: [PipelineCard]
    let phases: [PipelinePhase]
    let movingCardID: UUID?
    let width: CGFloat
    @Binding var selectedCardID: UUID?
    let secondRoutes: SecondRoutes
    let onFollowUp: (UUID) -> Void
    let onSkip: (UUID) -> Void
    let onRestore: (UUID) -> Void
    let onMove: (UUID, PipelinePhase) -> Void
    @State private var isTargeted = false

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack {
                Text(phase.name).font(.hubSection)
                Spacer()
                Text("\(cards.count)").monospacedDigit().foregroundStyle(.secondary)
            }
            ScrollView(.vertical) {
                LazyVStack(spacing: Space.s) {
                    ForEach(cards) { card in
                        let secondRoute = secondRoutes.makeRow(for: card) { onFollowUp(card.id) }
                        PipelineCardView(card: card, isMoving: card.id == movingCardID, isSelected: card.id == selectedCardID, secondRoute: secondRoute)
                            .onTapGesture { selectedCardID = card.id }
                            .draggable(card.id.uuidString)
                            .contextMenu {
                                if let jobURL = card.jobURL.flatMap(URL.init(string:)) {
                                    Button("Open posting") { NSWorkspace.shared.open(jobURL) }
                                }
                                if card.dismissedAt != nil {
                                    Button("Restore") { onRestore(card.id) }
                                } else {
                                    Button("Followed up…") { onFollowUp(card.id) }
                                    Menu("Move to") {
                                        ForEach(phases.filter { $0.id != card.application.phaseID }) { target in
                                            Button(target.name) { onMove(card.id, target) }
                                        }
                                    }
                                    .disabled(movingCardID != nil)
                                    Divider()
                                    // Ends the application with an outcome; Skip takes it out as not for you.
                                    if let closedPhase = phases.first(where: \.isClosed), closedPhase.id != card.application.phaseID {
                                        Button("Close…") { onMove(card.id, closedPhase) }
                                            .disabled(movingCardID != nil)
                                    }
                                    Button("Skip…") { onSkip(card.id) }
                                }
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
            onMove(cardID, phase)
            return true
        } isTargeted: { isTargeted = $0 }
    }

}

struct PipelineCardView: View {
    let card: PipelineCard
    let isMoving: Bool
    let isSelected: Bool
    /// Who to write to next, for a card nobody answered past its follow-up.
    var secondRoute: SecondRouteRow?

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
            HStack(alignment: .firstTextBaseline) {
                Text(card.title).fontWeight(.medium).lineLimit(2)
                Spacer(minLength: 0)
                if card.unseenUpdates > 0 {
                    UnseenDot(count: card.unseenUpdates)
                }
            }
            if card.jobTitle != nil, let companyName = card.companyName {
                Text(companyName).foregroundStyle(.secondary)
            }
            if let contactedAt = card.application.contactedAt {
                HeardBackLabel(contactedAt: contactedAt)
            }
            if let status = card.getFollowUpStatus(now: .now) {
                ToneChip(status)
            }
            if let secondRoute {
                secondRoute
            }
            if let closedReason = card.application.closedReason {
                Label(closedReason, systemImage: SetAside.closed.symbolName)
                    .font(.hubCaption).foregroundStyle(SetAside.closed.tone.color).lineLimit(2)
            }
            if let dismissalReason = card.dismissalReason, !dismissalReason.isEmpty {
                Label(dismissalReason, systemImage: SetAside.skipped.symbolName)
                    .font(.hubCaption).foregroundStyle(SetAside.skipped.tone.color).lineLimit(2)
            }
            HStack {
                Text(getTimeInPhaseText(days: card.getDaysInPhase(now: .now)))
                Spacer()
                if isMoving {
                    ProgressView().controlSize(.small)
                }
            }
            .font(.hubCaption)
            .foregroundStyle(.secondary)
        }
        .padding(Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(isSelected ? AnyShapeStyle(Tone.accent.color) : AnyShapeStyle(.separator), lineWidth: isSelected ? 2 : 1))
        .opacity(isMoving ? 0.6 : 1)
        .help(card.application.notes ?? "")
    }

    private func getTimeInPhaseText(days: Int) -> String {
        switch days {
        case 0: "Entered today"
        case 1: "1 day in phase"
        default: "\(days) days in phase"
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

/// What the board needs to offer each card nobody answered past its
/// follow-up its next route.
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

/// A card's next route once nobody answered it past its follow-up: write to
/// someone at the company or someone who can introduce you, or find who to
/// write to, then record the follow-up.
struct SecondRouteRow: View {
    let candidates: [RelatedPerson]
    let isFindingPeople: Bool
    let canFindPeople: Bool
    let onWrite: (RelatedPerson) -> Void
    let onFindPeople: () -> Void
    let onFollowUp: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Space.xs) {
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
                HStack(spacing: Space.s) {
                    ProgressView().controlSize(.small)
                    Text("Finding people…").foregroundStyle(.secondary)
                }
            } else {
                Button("Find people", systemImage: "person.2") { onFindPeople() }
                    .buttonStyle(.link)
                    .disabled(!canFindPeople)
                    .help(findPeopleHelp)
            }
            Button("Followed up…") { onFollowUp() }
                .buttonStyle(.link)
                .help("Record the message you sent; it restarts the count to the next follow-up")
        }
        .font(.hubCaption)
    }

    private var findPeopleHelp: String {
        canFindPeople
            ? "Nobody at the company is on file: an agent researches it for who to reach there, in a few minutes"
            : "Another company's research is running"
    }
}
