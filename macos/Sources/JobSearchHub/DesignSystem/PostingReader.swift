import JobSearchHubCore
import SwiftUI

/// Long text to read (P3 Reader): a posting's headings at the section size
/// with space above, its list items as bullets with room between, bold kept,
/// at 13 pt with a 1.3 line height. Phrases a screen check quoted are marked
/// in the check's tone, and find matches in the accent; hovering a marked
/// block names the checks and why. Each block is identified by
/// `PostingReader.blockID(_:)`, to scroll to.
struct PostingReader: View {
    let document: PostingDocument
    var marks: [PostingMark] = []
    var matches: [PostingSpan] = []
    /// Called as a heading's top passes the reading line near the top of the
    /// scroll view, going down (true) or back up (false), with its block.
    var onHeadingPassed: (Int, Bool) -> Void = { _, _ in }

    private static let lineSpacing: CGFloat = 13 * 0.3
    private static let headingSpace: CGFloat = 13 * 1.25
    private static let paragraphSpace: CGFloat = 13 * 0.75
    private static let itemSpace: CGFloat = 13 * 0.35
    /// How far below the scroll view's top a heading counts as the one in
    /// view.
    private static let readingLine: CGFloat = 96

    static func blockID(_ index: Int) -> String { "posting-block-\(index)" }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(document.blocks.indices, id: \.self) { index in
                block(index)
                    .help(getHelp(index))
                    .id(Self.blockID(index))
            }
        }
        .textSelection(.enabled)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder
    private func block(_ index: Int) -> some View {
        let readingLine = Self.readingLine
        switch document.blocks[index].kind {
        case .heading:
            Text(style(index))
                .font(.hubSection)
                .accessibilityAddTraits(.isHeader)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, index == 0 ? 0 : Self.headingSpace)
                .padding(.bottom, Self.itemSpace)
                .onGeometryChange(for: Bool.self) { proxy in
                    proxy.frame(in: .scrollView).minY < readingLine
                } action: { isPassed in
                    onHeadingPassed(index, isPassed)
                }
        case .bullet:
            HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                Text("•").foregroundStyle(.tertiary).accessibilityHidden(true)
                reading(index)
            }
            .padding(.bottom, Self.itemSpace)
        case .paragraph:
            reading(index)
                .padding(.bottom, Self.paragraphSpace)
        }
    }

    private func reading(_ index: Int) -> some View {
        Text(style(index))
            .font(.hubReading)
            .lineSpacing(Self.lineSpacing)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// The block's text with its marks, then its find matches over them.
    private func style(_ index: Int) -> AttributedString {
        var text = document.blocks[index].text
        for mark in marks where mark.span.block == index {
            highlight(&text, mark.span.range, in: mark.quote.verdict == .yes ? Tone.positive.mark : Tone.caution.mark)
        }
        for match in matches where match.block == index {
            highlight(&text, match.range, in: Tone.accent.mark)
        }
        return text
    }

    private func highlight(_ text: inout AttributedString, _ range: Range<Int>, in color: Color) {
        let characters = text.characters
        guard range.lowerBound >= 0, range.upperBound <= characters.count else { return }
        let start = characters.index(characters.startIndex, offsetBy: range.lowerBound)
        let end = characters.index(start, offsetBy: range.count)
        text[start..<end][AttributeScopes.SwiftUIAttributes.BackgroundColorAttribute.self] = color
    }

    /// Each check quoted in the block, once: "Screen · Timezone: …".
    private func getHelp(_ index: Int) -> String {
        var lines: [String] = []
        for mark in marks where mark.span.block == index && !lines.contains(mark.quote.help) {
            lines.append(mark.quote.help)
        }
        return lines.joined(separator: "\n")
    }
}

/// What the reader's marks mean, so color never stands alone.
struct MarkLegend: View {
    var body: some View {
        HStack(spacing: Space.m) {
            item("Screen is unclear or fails", tone: .caution)
            item("Screen passes", tone: .positive)
        }
        .font(.hubCaption)
        .foregroundStyle(.secondary)
    }

    private func item(_ text: String, tone: Tone) -> some View {
        HStack(spacing: Space.xs) {
            RoundedRectangle(cornerRadius: 2).fill(tone.mark).frame(width: 14, height: 9)
            Text(text).lineLimit(1)
        }
    }
}
