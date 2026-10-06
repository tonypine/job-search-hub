// The rollout: the sub-tickets of TP-659 in the order they can land, each
// with its mockup. Drawn last, from the images drawn before it.
import AppKit
import SwiftUI

struct Ticket {
    let id: String
    let title: String
    let level: String
    let after: [String]
    let mockup: String
}

enum Rollout {
    static let waves: [(String, [Ticket])] = [
        ("First", [
            Ticket(id: "TP-667", title: "Keep a posting's headings, lists and bold as Markdown", level: "Server, Mac, Android", after: [], mockup: "posting-before-after"),
            Ticket(id: "TP-668", title: "One page header, a tab strip, and the hub's status in the sidebar", level: "Patterns · shell", after: [], mockup: "shell-before-after"),
        ]),
        ("Then, in any order", [
            Ticket(id: "TP-669", title: "The Posting tab as a reader, key facts on top, quotes marked", level: "Section · posting", after: ["TP-667", "TP-668"], mockup: "posting-before-after"),
            Ticket(id: "TP-670", title: "The inspector's Overview: a verdict strip and three cards", level: "Section · job details", after: ["TP-668"], mockup: "job-overview-before-after"),
            Ticket(id: "TP-673", title: "Jobs table: two-line rows, verdicts as words, hover actions", level: "Section · tables", after: ["TP-668"], mockup: "shell-before-after"),
            Ticket(id: "TP-674", title: "Pipeline: one status line, due counts, Closed as a drawer", level: "Page · Pipeline", after: ["TP-668"], mockup: "pipeline-before-after"),
            Ticket(id: "TP-676", title: "Criteria with token fields and a save bar; Settings as status rows", level: "Section · settings", after: ["TP-668"], mockup: "criteria-settings-before-after"),
        ]),
        ("Then", [
            Ticket(id: "TP-671", title: "Open a job as a page, the posting beside its cards", level: "Pattern · page", after: ["TP-669", "TP-670"], mockup: "job-page"),
            Ticket(id: "TP-675", title: "Companies rows: open jobs, best match, who you know", level: "Server, Mac · page", after: ["TP-668", "TP-673"], mockup: "companies-before-after"),
        ]),
        ("Last", [
            Ticket(id: "TP-672", title: "Decide as a narrow queue beside the job's page", level: "Page · Decide", after: ["TP-671"], mockup: "decide"),
        ]),
    ]
}

struct TicketCard: View {
    let ticket: Ticket
    let image: NSImage?
    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            HStack(spacing: Space.s) {
                Text(ticket.id).font(.ui(11.5, .bold)).monospacedDigit().foregroundStyle(Tone.accent.color)
                Text(ticket.level).font(.ui(10.5, .medium)).foregroundStyle(secondaryInk)
                    .padding(.horizontal, 6).padding(.vertical, 1.5).background(fill, in: Capsule())
            }
            Text(ticket.title).font(.ui(13, .semibold)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
            if let image {
                Image(nsImage: image).resizable().aspectRatio(contentMode: .fill)
                    .frame(width: 330, height: 150, alignment: .top).clipped()
                    .clipShape(RoundedRectangle(cornerRadius: 6))
                    .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(separator))
            }
            HStack(spacing: 4) {
                Image(systemName: ticket.after.isEmpty ? "flag" : "arrow.turn.down.right").font(.system(size: 9.5, weight: .semibold))
                Text(ticket.after.isEmpty ? "Blocked by nothing" : "After " + ticket.after.joined(separator: ", ")).font(.ui(11))
                Spacer()
                Text(ticket.mockup + ".png").font(.system(size: 10, design: .monospaced)).foregroundStyle(tertiaryInk)
            }
            .foregroundStyle(secondaryInk)
        }
        .padding(Space.m + 2)
        .frame(width: 358, alignment: .leading)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.panel))
        .overlay(RoundedRectangle(cornerRadius: Radius.panel).strokeBorder(separator))
    }
}

struct RolloutBoard: View {
    let directory: URL
    var body: some View {
        Board(
            eyebrow: "Rollout",
            title: "Ten sub-tickets of TP-659, in the order they can land",
            subtitle: "Each ships on its own and leaves the app working. The server change and the shell's patterns go first; the sections follow in any order; the job page waits for its two sections, and Decide for the job page.",
            width: 1780
        ) {
            HStack(alignment: .top, spacing: Space.xl) {
                ForEach(Array(Rollout.waves.enumerated()), id: \.offset) { index, wave in
                    VStack(alignment: .leading, spacing: Space.m) {
                        HStack(spacing: Space.s) {
                            Text("\(index + 1)").font(.ui(12, .bold)).foregroundStyle(.white)
                                .frame(width: 22, height: 22).background(Tone.accent.color, in: Circle())
                            Text(wave.0).font(.ui(15, .semibold)).foregroundStyle(ink)
                        }
                        ForEach(wave.1, id: \.id) { ticket in
                            TicketCard(ticket: ticket, image: NSImage(contentsOf: directory.appending(path: ticket.mockup + ".png")))
                        }
                    }
                    if index < Rollout.waves.count - 1 {
                        Image(systemName: "chevron.right").font(.system(size: 18, weight: .semibold)).foregroundStyle(tertiaryInk).padding(.top, 140)
                    }
                }
            }
        }
    }
}
