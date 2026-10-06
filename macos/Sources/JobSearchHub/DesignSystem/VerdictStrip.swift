import JobSearchHubCore
import SwiftUI

/// The few judgments a decision rests on, side by side under the title
/// (P4): each cell a label, a symbol and a word in its tone. A click opens
/// the cell's evidence. The cells share the width they're given and ask for
/// none of their own, so a long word shrinks or truncates rather than
/// widening the inspector.
struct VerdictStrip: View {
    let cells: [VerdictCell]
    let open: (VerdictCell.Kind) -> Void

    var body: some View {
        HStack(spacing: 0) {
            ForEach(Array(cells.enumerated()), id: \.element.id) { index, cell in
                if index > 0 {
                    Divider()
                }
                Button { open(cell.kind) } label: { label(cell) }
                    .buttonStyle(.plain)
                    .help(cell.detail ?? "Show \(cell.kind.title)")
                    .accessibilityLabel(cell.accessibilityLabel)
                    .accessibilityHint("Shows its card")
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(.separator))
    }

    private func label(_ cell: VerdictCell) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(cell.kind.title)
                .font(.hubCaption)
                .foregroundStyle(.secondary)
            HStack(spacing: 3) {
                Image(systemName: cell.symbolName).imageScale(.small)
                Text(cell.word)
            }
            .font(.hubSecondary.weight(.semibold))
            .foregroundStyle(cell.tone.color)
        }
        .lineLimit(1)
        .minimumScaleFactor(0.75)
        .padding(.horizontal, Space.s)
        .padding(.vertical, Space.s - 2)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
    }
}
