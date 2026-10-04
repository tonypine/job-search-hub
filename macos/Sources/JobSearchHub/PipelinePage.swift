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
                    .onChange(of: details.getSubject(on: .pipeline)) {
                        if details.getSubject(on: .pipeline) == nil { selectedCardID = nil }
                    }
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
            ScrollView(.horizontal) {
                HStack(alignment: .top, spacing: Self.columnSpacing) {
                    ForEach(model.board.phases) { phase in
                        PipelineColumn(
                            phase: phase, cards: model.getShownCards(in: phase), phases: model.board.phases,
                            movingCardID: model.movingCardID, width: columnWidth, selectedCardID: $selectedCardID,
                            onFollowUp: { cardID in
                                followUpNote = ""
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

    /// The selected card's details: its job's, or its company's for a card
    /// without one.
    private var selectedCardSubject: DetailsInspector.Subject? {
        guard let application = model.board.cards.first(where: { $0.id == selectedCardID })?.application else { return nil }
        if let jobID = application.jobID {
            return .job(jobID, opensSession: false)
        }
        return .companyApplication(companyID: application.companyID)
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
                        PipelineCardView(card: card, isMoving: card.id == movingCardID, isSelected: card.id == selectedCardID)
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
