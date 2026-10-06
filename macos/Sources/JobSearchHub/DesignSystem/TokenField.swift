import JobSearchHubCore
import SwiftUI

/// A list a person edits (P10): each value a token with ×, and a field for
/// the next after them. Return or a comma makes what's typed a token, and
/// Delete in the empty field takes the last one off. Tokens that let
/// something in are in the positive tone, ones that rule it out in the
/// negative.
///
/// The words typed and not yet a token are the caller's, so a save can take
/// them too.
struct TokenField: View {
    /// What the list is, for VoiceOver: "Hires from".
    let title: String
    @Binding var tokens: [String]
    @Binding var text: String
    var tone = Tone.neutral
    var prompt = "Add…"
    @FocusState private var isFocused: Bool

    var body: some View {
        TokenFlowLayout(spacing: Space.xs, minimumFieldWidth: 90) {
            ForEach(Array(tokens.enumerated()), id: \.offset) { index, token in
                TokenChip(text: token, tone: tone) { remove(at: index) }
                    .accessibilityLabel(token)
            }
            TextField(title, text: $text, prompt: Text(prompt))
                .labelsHidden()
                .textFieldStyle(.plain)
                .focused($isFocused)
                .accessibilityLabel("Add to \(title)")
                .accessibilityHint("Type a value and press Return. A comma adds one too.")
                .onSubmit { addTyped() }
                .onChange(of: text) { addBeforeLastComma() }
                .onChange(of: isFocused) {
                    if !isFocused { addTyped() }
                }
                .onKeyPress(.delete) {
                    guard text.isEmpty, !tokens.isEmpty else { return .ignored }
                    tokens.removeLast()
                    return .handled
                }
        }
        .padding(.horizontal, Space.s)
        .padding(.vertical, Space.xs)
        .frame(minHeight: 28)
        .background(.background, in: RoundedRectangle(cornerRadius: Radius.control))
        .overlay {
            RoundedRectangle(cornerRadius: Radius.control)
                .strokeBorder(isFocused ? AnyShapeStyle(Color.hubAccent) : AnyShapeStyle(.separator), lineWidth: isFocused ? 1.5 : 1)
        }
        .contentShape(Rectangle())
        .onTapGesture { isFocused = true }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(title)
        .accessibilityValue(tokens.isEmpty ? "None" : tokens.joined(separator: ", "))
    }

    private func remove(at index: Int) {
        guard tokens.indices.contains(index) else { return }
        tokens.remove(at: index)
    }

    private func addTyped() {
        guard !text.trimmingCharacters(in: .whitespaces).isEmpty else { return }
        tokens = JobCriteriaDraft.addTokens(from: text, to: tokens)
        text = ""
    }

    /// A comma typed or pasted makes what's before it tokens, and leaves
    /// what's after it to type on.
    private func addBeforeLastComma() {
        guard let comma = text.lastIndex(of: ",") else { return }
        tokens = JobCriteriaDraft.addTokens(from: String(text[..<comma]), to: tokens)
        text = String(text[text.index(after: comma)...]).trimmingCharacters(in: .whitespaces)
    }
}

/// One value of a token field, in its tone, with × to remove it.
private struct TokenChip: View {
    let text: String
    let tone: Tone
    let remove: () -> Void

    var body: some View {
        HStack(spacing: 3) {
            Text(text).lineLimit(1)
            Button("Remove \(text)", systemImage: "xmark", action: remove)
                .labelStyle(.iconOnly)
                .buttonStyle(.plain)
                .imageScale(.small)
                .foregroundStyle(.secondary)
                .help("Remove \(text)")
        }
        .font(.hubSecondary)
        .foregroundStyle(tone == .neutral ? AnyShapeStyle(.primary) : AnyShapeStyle(tone.color))
        .padding(.leading, Space.s)
        .padding(.trailing, Space.xs + 2)
        .padding(.vertical, 2)
        .background(tone.fill, in: RoundedRectangle(cornerRadius: Radius.control - 2))
        .accessibilityElement(children: .contain)
    }
}

/// The tokens in rows that wrap at the width offered, and the field after
/// them taking the rest of its row, or a row of its own when less than its
/// minimum is left. Its height follows the width it's given; it asks for no
/// width of its own beyond one token, so it never widens the form it's in.
private struct TokenFlowLayout: Layout {
    let spacing: CGFloat
    let minimumFieldWidth: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let frames = arrange(subviews, width: proposal.width ?? .infinity)
        let used = frames.reduce(CGRect.zero) { $0.union($1) }
        return CGSize(width: proposal.width ?? used.maxX, height: used.maxY)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let frames = arrange(subviews, width: bounds.width)
        for (subview, frame) in zip(subviews, frames) {
            subview.place(
                at: CGPoint(x: bounds.minX + frame.minX, y: bounds.minY + frame.minY),
                proposal: ProposedViewSize(width: frame.width, height: frame.height)
            )
        }
    }

    /// Each subview's frame: tokens left to right, wrapping, each row's
    /// items centered on the row; the last subview is the field.
    private func arrange(_ subviews: Subviews, width: CGFloat) -> [CGRect] {
        var frames: [CGRect] = []
        var rowStart = 0
        var x: CGFloat = 0
        var y: CGFloat = 0
        var rowHeight: CGFloat = 0

        func endRow() {
            for index in rowStart..<frames.count {
                frames[index].origin.y = y + (rowHeight - frames[index].height) / 2
            }
            y += rowHeight + spacing
            rowStart = frames.count
            x = 0
            rowHeight = 0
        }

        for (index, subview) in subviews.enumerated() {
            let isField = index == subviews.count - 1
            var size: CGSize
            if isField {
                size = subview.sizeThatFits(ProposedViewSize(width: minimumFieldWidth, height: nil))
                size.width = minimumFieldWidth
            } else {
                size = subview.sizeThatFits(.unspecified)
                size.width = min(size.width, width)
            }
            if x > 0, x + size.width > width {
                endRow()
            }
            if isField, width.isFinite {
                size.width = max(width - x, min(minimumFieldWidth, width))
            }
            frames.append(CGRect(x: x, y: 0, width: size.width, height: size.height))
            x += size.width + spacing
            rowHeight = max(rowHeight, size.height)
        }
        if rowStart < frames.count {
            endRow()
        }
        return frames
    }
}
