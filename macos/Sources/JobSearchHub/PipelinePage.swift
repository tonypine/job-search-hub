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
    @State private var model = PipelineModel()
    @State private var pendingClose: PendingClose?
    @State private var closedReason = ""

    var body: some View {
        Group {
            if let client = connection.makeClient() {
                board(client: client)
                    .task { await model.load(with: client) }
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
                            phase: phase, cards: model.board.getCards(in: phase), phases: model.board.phases,
                            movingCardID: model.movingCardID, width: columnWidth
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
            Button("Refresh", systemImage: "arrow.clockwise") { Task { await model.load(with: client) } }
                .disabled(model.isLoading)
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
                        PipelineCardView(card: card, isMoving: card.id == movingCardID)
                            .draggable(card.id.uuidString)
                            .contextMenu {
                                if let jobURL = card.jobURL.flatMap(URL.init(string:)) {
                                    Button("Open posting") { NSWorkspace.shared.open(jobURL) }
                                }
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

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(card.title).fontWeight(.medium).lineLimit(2)
            if card.jobTitle != nil, let companyName = card.companyName {
                Text(companyName).foregroundStyle(.secondary)
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
        .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(.separator))
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
