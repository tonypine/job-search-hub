import AppKit
import JobSearchHubCore
import SwiftUI

@MainActor
@Observable
final class PipelineModel {
    private(set) var board = PipelineBoard()
    private(set) var isLoading = false
    private(set) var loadError: String?
    private(set) var movingCardID: UUID?
    var moveError: String?
    var showsOnlyDue = false

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
            moveError = String(describing: error)
        }
    }

    func load(with client: HubClient) async {
        isLoading = true
        defer { isLoading = false }
        do {
            board = PipelineBoard(try await client.get("v1/pipeline", as: PipelineResponse.self))
            loadError = nil
        } catch {
            loadError = String(describing: error)
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
            moveError = String(describing: error)
        }
    }
}

/// A move into a closed phase, waiting for the reason the application ended.
private struct PendingClose {
    let cardID: UUID
    let phase: PipelinePhase
}

struct PipelinePage: View {
    private static let columnSpacing: CGFloat = 12
    private static let boardPadding: CGFloat = 16
    private static let columnWidthRange: ClosedRange<CGFloat> = 180...320

    @Environment(HubConnection.self) private var connection
    @Environment(HubEventStream.self) private var events
    @Environment(UnseenUpdates.self) private var unseen
    @State private var model = PipelineModel()
    @State private var pendingClose: PendingClose?
    @State private var closedReason = ""
    @State private var selectedCardID: UUID?
    @State private var followUpCardID: UUID?
    @State private var followUpNote = ""
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
                    .onChange(of: [events.revision, unseen.revision]) { Task { await model.load(with: client) } }
                    .inspector(isPresented: Binding(get: { selectedCardID != nil }, set: { if !$0 { selectedCardID = nil } })) {
                        selectedCardDetail(client: client)
                            .inspectorColumnWidth(min: 360, ideal: 460, max: 720)
                    }
            } else {
                ContentUnavailableView("Not connected", systemImage: "network.slash", description: Text("Set the hub URL and owner token in Settings."))
            }
        }
        .navigationTitle("Pipeline")
        .navigationSubtitle(model.board.cards.count == 1 ? "1 application" : "\(model.board.cards.count) applications")
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
                            }
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
            Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                .disabled(model.isLoading)
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
                ContentUnavailableView("Could not load the pipeline", systemImage: "exclamationmark.triangle", description: Text(loadError))
            }
        }
        .alert("Could not move the card", isPresented: Binding(get: { model.moveError != nil }, set: { if !$0 { model.moveError = nil } })) {
            Button("OK") {}
        } message: {
            Text(model.moveError ?? "")
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

    /// A card's job panel. A card for a company alone has no panel, so
    /// selecting it is what marks the company's updates seen.
    @ViewBuilder
    private func selectedCardDetail(client: HubClient) -> some View {
        let application = model.board.cards.first(where: { $0.id == selectedCardID })?.application
        if let jobID = application?.jobID {
            JobPanel(jobID: jobID, client: client)
        } else {
            ContentUnavailableView("No job on this card", systemImage: "building.2", description: Text("This application is to a company, not a posting."))
                .task(id: application?.id) {
                    if let companyID = application?.companyID {
                        await unseen.markSeen(UpdateSelection(companyID: companyID), with: client)
                    }
                }
        }
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
    let onMove: (UUID, PipelinePhase) -> Void
    @State private var isTargeted = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(phase.name).font(.headline)
                Spacer()
                Text("\(cards.count)").monospacedDigit().foregroundStyle(.secondary)
            }
            ScrollView(.vertical) {
                LazyVStack(spacing: 8) {
                    ForEach(cards) { card in
                        PipelineCardView(card: card, isMoving: card.id == movingCardID, isSelected: card.id == selectedCardID)
                            .onTapGesture { selectedCardID = card.id }
                            .draggable(card.id.uuidString)
                            .contextMenu {
                                if let jobURL = card.jobURL.flatMap(URL.init(string:)) {
                                    Button("Open posting") { NSWorkspace.shared.open(jobURL) }
                                }
                                Button("Followed up…") { onFollowUp(card.id) }
                                Menu("Move to") {
                                    ForEach(phases.filter { $0.id != card.application.phaseID }) { target in
                                        Button(target.name) { onMove(card.id, target) }
                                    }
                                }
                                .disabled(movingCardID != nil)
                            }
                    }
                }
            }
        }
        .padding(10)
        .frame(width: width)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(isTargeted ? AnyShapeStyle(Color.accentColor.opacity(0.15)) : AnyShapeStyle(.quinary), in: RoundedRectangle(cornerRadius: 10))
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
        VStack(alignment: .leading, spacing: 4) {
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
            if let status = card.getFollowUpStatus(now: .now) {
                Text(status.text)
                    .font(.caption.weight(status.isDue ? .semibold : .regular))
                    .foregroundStyle(followUpColor(status))
            }
            if let closedReason = card.application.closedReason {
                Text(closedReason).font(.caption).foregroundStyle(.secondary).lineLimit(2)
            }
            HStack {
                Text(getTimeInPhaseText(days: card.getDaysInPhase(now: .now)))
                Spacer()
                if isMoving {
                    ProgressView().controlSize(.small)
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(isSelected ? AnyShapeStyle(Color.accentColor) : AnyShapeStyle(.separator), lineWidth: isSelected ? 2 : 1))
        .opacity(isMoving ? 0.6 : 1)
        .help(card.application.notes ?? "")
    }

    private func followUpColor(_ status: FollowUpStatus) -> Color {
        switch status {
        case .overdue: .red
        case .dueToday: .orange
        case .dueIn: .secondary
        }
    }

    private func getTimeInPhaseText(days: Int) -> String {
        switch days {
        case 0: "Entered today"
        case 1: "1 day in phase"
        default: "\(days) days in phase"
        }
    }
}
