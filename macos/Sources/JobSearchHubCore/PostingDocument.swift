import Foundation

/// A posting as the reader draws it: its blocks with their inline markup
/// (bold, links) parsed, so the text here is the text that shows. Marks and
/// find matches are spans of that text, never of the Markdown source.
public struct PostingDocument: Equatable, Sendable {
    public var blocks: [PostingBlock]

    public init(markdown: String) {
        blocks = MarkdownBlocks.parse(markdown).map(PostingBlock.init)
    }

    /// The posting's headings, to jump to; empty for a posting without any,
    /// such as one stored as plain text.
    public var outline: [PostingHeading] {
        blocks.indices.compactMap { index in
            guard case .heading = blocks[index].kind else { return nil }
            let title = blocks[index].plainText.trimmingCharacters(in: .whitespacesAndNewlines)
            return title.isEmpty ? nil : PostingHeading(block: index, title: title)
        }
    }

    /// Where each quote shows in the posting. A quote is plain words, while
    /// the posting may hold bold, list items and line breaks, so case,
    /// accents, curly quotes and runs of white space don't count, a quote may
    /// run across blocks, and a quote cut with an ellipsis matches each part.
    /// A quote that isn't in the posting has no mark.
    public func findMarks(for quotes: [PostingQuote]) -> [PostingMark] {
        let text = FlatText(blocks)
        return quotes.flatMap { quote in
            Self.splitQuote(quote.text).flatMap { part -> [PostingMark] in
                guard let match = text.findFirst(part) ?? text.findFirst(Self.trimEndPunctuation(part)) else { return [] }
                return text.getSpans(match).map { PostingMark(span: $0, quote: quote) }
            }
        }
    }

    /// Every place the query shows in the posting, in reading order, as for
    /// ⌘F: case, accents and runs of white space don't count.
    public func findMatches(of query: String) -> [PostingSpan] {
        let text = FlatText(blocks)
        return text.findAll(query).flatMap { text.getSpans($0) }
    }

    /// The quote's parts around any ellipsis, without a leading list marker,
    /// heading marker, Markdown emphasis or surrounding quotes. Parts of
    /// fewer than four characters would mark the wrong words, so they go.
    static func splitQuote(_ quote: String) -> [String] {
        quote
            .replacingOccurrences(of: "...", with: "…")
            .split(separator: "…")
            .map { part in
                var text = part.replacingOccurrences(of: "**", with: "").replacingOccurrences(of: "__", with: "")
                text = text.trimmingCharacters(in: .whitespacesAndNewlines.union(.init(charactersIn: "\"\u{201C}\u{201D}'\u{2018}\u{2019}")))
                while let marker = ["- ", "* ", "• ", "#"].first(where: { text.hasPrefix($0) }) {
                    text = String(text.dropFirst(marker.count)).trimmingCharacters(in: .whitespaces)
                }
                return text
            }
            .filter { $0.count >= 4 }
    }

    private static func trimEndPunctuation(_ text: String) -> String {
        String(text.reversed().drop { $0.isPunctuation || $0.isWhitespace }.reversed())
    }
}

/// One block of a posting: a heading, a list item or a paragraph, with its
/// inline markup parsed.
public struct PostingBlock: Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case heading(level: Int)
        case bullet
        case paragraph
    }

    public var kind: Kind
    /// The text as it shows, with bold and links as attributes.
    public var text: AttributedString

    init(_ block: MarkdownBlock) {
        let (kind, markdown): (Kind, String) = switch block {
        case let .heading(level, text): (.heading(level: level), text)
        case let .bullet(text): (.bullet, text)
        case let .paragraph(text): (.paragraph, text)
        }
        self.kind = kind
        text = (try? AttributedString(markdown: markdown, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)))
            ?? AttributedString(markdown)
    }

    /// The text as it shows, without its attributes.
    public var plainText: String { String(text.characters) }
}

/// A heading of the posting, by the index of its block.
public struct PostingHeading: Equatable, Identifiable, Sendable {
    public var block: Int
    public var title: String

    public var id: Int { block }
}

/// A run of characters in one block's shown text, by character offsets.
public struct PostingSpan: Equatable, Sendable {
    public var block: Int
    public var range: Range<Int>

    public init(block: Int, range: Range<Int>) {
        self.block = block
        self.range = range
    }
}

/// The posting's words a screen check rests on, and that check.
public struct PostingQuote: Equatable, Sendable {
    public var name: String
    public var verdict: FitVerdict
    public var reason: String
    public var text: String

    public init(name: String, verdict: FitVerdict, reason: String, text: String) {
        self.name = name
        self.verdict = verdict
        self.reason = reason
        self.text = text
    }

    /// Hovering its mark says it: "Screen · Experience: asks for 6 years; you have 5 years".
    public var help: String { "Screen · \(name): \(reason)" }
}

/// A quote found in the posting.
public struct PostingMark: Equatable, Sendable {
    public var span: PostingSpan
    public var quote: PostingQuote
}

/// The blocks' shown text as one run of folded characters, a space between
/// blocks and for each run of white space, with where each character came
/// from.
private struct FlatText {
    var characters: [Character] = []
    /// The block and character offset of each flat character; nil for the
    /// space between two blocks.
    var origins: [(block: Int, offset: Int)?] = []

    init(_ blocks: [PostingBlock]) {
        for (index, block) in blocks.enumerated() {
            if !characters.isEmpty && characters.last != " " {
                characters.append(" ")
                origins.append(nil)
            }
            for (offset, character) in block.text.characters.enumerated() {
                if character.isWhitespace {
                    if characters.isEmpty || characters.last == " " { continue }
                    characters.append(" ")
                } else {
                    characters.append(Self.fold(character))
                }
                origins.append((index, offset))
            }
        }
    }

    /// The character for comparing: lower case, without accents, with
    /// typographic quotes and dashes as their plain forms.
    static func fold(_ character: Character) -> Character {
        switch character {
        case "\u{2018}", "\u{2019}": return "'"
        case "\u{201C}", "\u{201D}": return "\""
        case "\u{2013}", "\u{2014}": return "-"
        default:
            let folded = String(character).folding(options: [.caseInsensitive, .diacriticInsensitive], locale: nil)
            return folded.count == 1 ? Character(folded) : character
        }
    }

    static func fold(_ text: String) -> [Character] {
        var folded: [Character] = []
        for character in text.trimmingCharacters(in: .whitespacesAndNewlines) {
            if character.isWhitespace {
                if folded.last != " " { folded.append(" ") }
            } else {
                folded.append(fold(character))
            }
        }
        return folded
    }

    func findFirst(_ text: String) -> Range<Int>? {
        let needle = Self.fold(text)
        guard !needle.isEmpty, needle.count <= characters.count else { return nil }
        return (0...(characters.count - needle.count)).lazy
            .first { start in characters[start..<(start + needle.count)].elementsEqual(needle) }
            .map { $0..<($0 + needle.count) }
    }

    func findAll(_ text: String) -> [Range<Int>] {
        let needle = Self.fold(text)
        guard !needle.isEmpty, needle.count <= characters.count else { return [] }
        var matches: [Range<Int>] = []
        var start = 0
        while start <= characters.count - needle.count {
            if characters[start..<(start + needle.count)].elementsEqual(needle) {
                matches.append(start..<(start + needle.count))
                start += needle.count
            } else {
                start += 1
            }
        }
        return matches
    }

    /// A flat range as one span per block it covers.
    func getSpans(_ range: Range<Int>) -> [PostingSpan] {
        var spans: [PostingSpan] = []
        for origin in origins[range].compactMap({ $0 }) {
            if let last = spans.last, last.block == origin.block {
                spans[spans.count - 1].range = last.range.lowerBound..<(origin.offset + 1)
            } else {
                spans.append(PostingSpan(block: origin.block, range: origin.offset..<(origin.offset + 1)))
            }
        }
        return spans
    }
}
