import JobSearchHubCore
import SwiftUI

/// Markdown drawn as a document: headings in bold at a larger size, bullets
/// and paragraphs, with bold and links inside them. The Profile page and a
/// job's posting use it.
struct MarkdownDocument: View {
    let blocks: [MarkdownBlock]

    init(_ markdown: String) {
        blocks = MarkdownBlocks.parse(markdown)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Space.s) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, block in
                switch block {
                case .heading(let level, let text):
                    Text(inline(text))
                        .font(level == 1 ? .largeTitle.bold() : .title3.bold())
                        .padding(.top, level == 1 ? 0 : Space.s)
                case .bullet(let text):
                    HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                        Text("•")
                        Text(inline(text))
                    }
                case .paragraph(let text):
                    Text(inline(text))
                }
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .textSelection(.enabled)
    }

    private func inline(_ text: String) -> AttributedString {
        (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)))
            ?? AttributedString(text)
    }
}
