/// A markdown document as the blocks the Profile page draws: headings,
/// bullets and paragraphs. Inline markup (bold, links) stays in the text for
/// SwiftUI to render.
public enum MarkdownBlock: Equatable, Sendable {
    case heading(level: Int, text: String)
    case bullet(String)
    case paragraph(String)
}

public enum MarkdownBlocks {
    /// Consecutive plain lines form one paragraph, keeping their line breaks;
    /// a blank line ends it.
    public static func parse(_ markdown: String) -> [MarkdownBlock] {
        var blocks: [MarkdownBlock] = []
        var paragraphLines: [String] = []

        func closeParagraph() {
            if !paragraphLines.isEmpty {
                blocks.append(.paragraph(paragraphLines.joined(separator: "\n")))
                paragraphLines = []
            }
        }

        for rawLine in markdown.split(separator: "\n", omittingEmptySubsequences: false) {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.isEmpty {
                closeParagraph()
            } else if let heading = parseHeading(line) {
                closeParagraph()
                blocks.append(heading)
            } else if line.hasPrefix("- ") || line.hasPrefix("* ") {
                closeParagraph()
                blocks.append(.bullet(String(line.dropFirst(2))))
            } else {
                paragraphLines.append(line)
            }
        }
        closeParagraph()
        return blocks
    }

    private static func parseHeading(_ line: String) -> MarkdownBlock? {
        let level = line.prefix(while: { $0 == "#" }).count
        guard (1...6).contains(level), line.dropFirst(level).first == " " else { return nil }
        return .heading(level: level, text: String(line.dropFirst(level + 1)))
    }
}
