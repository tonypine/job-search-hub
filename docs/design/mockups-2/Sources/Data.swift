// The made-up job the mockups show: a posting at Northwind, as its board
// wrote it, and what the hub read and decided about it.
import SwiftUI

/// One block of a posting once its structure is kept: a heading, a
/// paragraph, or a bullet. `**…**` marks bold, `[[…]]` a phrase an unclear
/// screen check quoted, and `{{…}}` one a passing check quoted.
enum PostingBlock {
    case heading(String)
    case paragraph(String)
    case bullet(String)
}

enum SampleJob {
    static let title = "Senior Product Engineer"
    static let company = "Northwind"
    static let location = "Remote, Americas"

    static let posting: [PostingBlock] = [
        .paragraph("Northwind builds payment software for independent retailers across the Americas. We're 140 people, {{fully remote across the Americas}}, and profitable since 2023."),
        .heading("What you'll do"),
        .bullet("Own the **checkout rebuild** end to end, from the API to the last pixel."),
        .bullet("Ship every week with a small team: two engineers, a designer and a PM."),
        .bullet("Sit in on merchant calls to find what slows them down, and fix it."),
        .heading("What you bring"),
        .bullet("[[6+ years building web products with React]] and TypeScript."),
        .bullet("Production experience with Node.js and PostgreSQL."),
        .bullet("You own features without waiting for a spec."),
        .bullet("[[At least 4 hours of overlap with US Eastern time.]]"),
        .heading("Nice to have"),
        .bullet("GraphQL federation in production."),
        .bullet("Payments or fintech experience."),
        .heading("Benefits"),
        .bullet("Paid in USD as a contractor, anywhere in the Americas."),
        .bullet("30 days off, plus your local holidays."),
        .bullet("A yearly team retreat."),
        .heading("How we hire"),
        .bullet("A 30-minute call with the hiring manager."),
        .bullet("A paid take-home, about three hours."),
        .bullet("A pairing session and a values chat."),
    ]

    /// The posting as the hub keeps it today: the board's HTML with its
    /// markup dropped, a line per paragraph or list item.
    static var flattened: [String] {
        posting.map { block in
            switch block {
            case let .heading(text), let .paragraph(text), let .bullet(text): strip(text)
            }
        }
    }

    static func strip(_ text: String) -> String {
        text.replacingOccurrences(of: "**", with: "").replacingOccurrences(of: "[[", with: "").replacingOccurrences(of: "]]", with: "")
            .replacingOccurrences(of: "{{", with: "").replacingOccurrences(of: "}}", with: "")
    }

    /// The text with its bold and its highlights drawn.
    static func styled(_ text: String, size: CGFloat, highlights: Bool = true) -> Text {
        var result = AttributedString()
        var rest = Substring(text)
        let markers: [(open: String, close: String, kind: Int)] = [("**", "**", 0), ("[[", "]]", 1), ("{{", "}}", 2)]
        while !rest.isEmpty {
            let next = markers.compactMap { marker -> (Range<Substring.Index>, (open: String, close: String, kind: Int))? in
                rest.range(of: marker.open).map { ($0, marker) }
            }.min { $0.0.lowerBound < $1.0.lowerBound }
            guard let (openRange, marker) = next, let closeRange = rest[openRange.upperBound...].range(of: marker.close) else {
                result += AttributedString(String(rest))
                break
            }
            result += AttributedString(String(rest[..<openRange.lowerBound]))
            var piece = AttributedString(String(rest[openRange.upperBound..<closeRange.lowerBound]))
            switch marker.kind {
            case 0:
                piece.font = .system(size: size, weight: .semibold)
            case 1 where highlights:
                piece.backgroundColor = highlightCaution
            case 2 where highlights:
                piece.backgroundColor = highlightPositive
            default:
                break
            }
            result += piece
            rest = rest[closeRange.upperBound...]
        }
        return Text(result)
    }
}
