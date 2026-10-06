// The job in the inspector: its header and actions as built today and as
// proposed, and its Posting tab, the job description, before and after.
import SwiftUI

// MARK: - Header

/// The job's header and actions as the app draws them today.
struct CurrentJobTop: View {
    var tab = "Posting"
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            VStack(alignment: .leading, spacing: Space.xs) {
                HStack(spacing: Space.xs) {
                    Text("JOB").foregroundStyle(secondaryInk)
                    Text("·").foregroundStyle(secondaryInk)
                    Text("NORTHWIND").foregroundStyle(Tone.accent.color)
                }
                .font(.ui(11, .semibold))
                Text(SampleJob.title).font(.ui(20, .semibold)).foregroundStyle(ink)
                Text("Remote, Americas · Posted 2 days ago").font(.ui(12)).foregroundStyle(secondaryInk)
                HStack(spacing: Space.xs) {
                    Chip(text: "Strong", tone: .positive)
                    Chip(text: "Unclear screen", tone: .caution)
                }
                .padding(.top, Space.xs)
            }
            HStack(spacing: Space.s) {
                PrimaryButton(title: "Pursue", symbol: "arrow.up.forward")
                SecondaryButton(title: "Later", symbol: "clock")
                SecondaryButton(title: "Skip…", symbol: "eye.slash")
                IconButton(symbol: "ellipsis")
            }
            SegmentedTabs(names: ["Overview", "Posting", "Session"], selected: tab)
        }
        .padding(.horizontal, Space.l)
        .padding(.top, Space.xs)
        .padding(.bottom, Space.s)
    }
}

/// One cell of the verdict strip: what's being judged, and the verdict.
struct VerdictCell: View {
    let label: String
    let value: String
    let tone: Tone
    var symbol: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.ui(10.5, .medium)).foregroundStyle(secondaryInk)
            HStack(spacing: 4) {
                if let symbol { Image(systemName: symbol).font(.system(size: 10.5, weight: .semibold)) }
                Text(value).font(.ui(12.5, .semibold)).lineLimit(1)
            }
            .foregroundStyle(tone == .neutral ? ink : tone.color)
        }
        .padding(.horizontal, 10).padding(.vertical, 7)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The proposed verdict strip: the four things a decision rests on, side by
/// side, each a word and a symbol in its tone. A cell opens its evidence.
struct VerdictStrip: View {
    var compact = false
    var body: some View {
        HStack(spacing: 0) {
            VerdictCell(label: "Match", value: "Strong", tone: .positive, symbol: "star.fill")
            divider
            VerdictCell(label: "Screen", value: "2 unclear", tone: .caution, symbol: "questionmark.circle.fill")
            divider
            VerdictCell(label: "Take-home", value: "≈ R$ 31k/mo", tone: .neutral)
            if !compact {
                divider
                VerdictCell(label: "People", value: "2 you know", tone: .accent)
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .background(surface, in: RoundedRectangle(cornerRadius: Radius.card))
        .overlay(RoundedRectangle(cornerRadius: Radius.card).strokeBorder(separator))
    }

    private var divider: some View { Rectangle().fill(separator).frame(width: 1).padding(.vertical, 8) }
}

/// The job's header and actions as proposed: the title block, the verdict
/// strip, one row of actions with their keys, then the tab strip.
struct ProposedJobTop: View {
    var tab = "Posting"
    var showsStrip = true
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            HStack(alignment: .top, spacing: Space.m) {
                Monogram(letters: "N", hue: Color(hex: 0x0E7C86), size: 36)
                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 4) {
                        Text("Northwind").font(.ui(12, .medium)).foregroundStyle(Tone.accent.color)
                        Text("· Payments · 140 people").font(.ui(12)).foregroundStyle(secondaryInk)
                    }
                    Text(SampleJob.title).font(.ui(19, .semibold)).foregroundStyle(ink)
                    Text("Remote, Americas · Contractor · Posted 2 days ago").font(.ui(12)).foregroundStyle(secondaryInk)
                }
            }
            if showsStrip { VerdictStrip() }
            HStack(spacing: Space.s) {
                PrimaryButton(title: "Pursue", symbol: "arrow.up.forward", shortcut: "P")
                SecondaryButton(title: "Later", shortcut: "L")
                SecondaryButton(title: "Skip…", shortcut: "S")
                Spacer(minLength: 0)
                IconButton(symbol: "safari")
                IconButton(symbol: "ellipsis")
            }
            TabStrip(tabs: [("Overview", nil), ("Posting", nil), ("Session", "•")], selected: tab)
                .padding(.top, 2)
        }
        .padding(.horizontal, Space.l)
        .padding(.top, Space.xs)
    }
}

// MARK: - The Posting tab today

/// The Posting tab as built: the board's facts, the facts read from the
/// posting with their quotes, who read them, a button, and then the posting
/// as one block of plain text.
struct CurrentPostingTab: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.l) {
            VStack(alignment: .leading, spacing: Space.s) {
                Text("From the board").font(.ui(13, .semibold)).foregroundStyle(ink)
                FactRow(label: "Pay", value: "Not published", valueColor: secondaryInk)
                FactRow(label: "Workplace", value: "Remote")
                FactRow(label: "Employment", value: "Contractor")
                FactRow(label: "Department", value: "Engineering")
                FactRow(label: "Published", value: "4 Oct 2026")
                FactRow(label: "First seen", value: "4 Oct 2026")
            }
            .marker(2, x: -30)
            VStack(alignment: .leading, spacing: Space.s) {
                Text("Read from the posting").font(.ui(13, .semibold)).foregroundStyle(ink)
                fact("Seniority", "Senior")
                fact("Years", "6+", quote: "6+ years building web products with React")
                fact("Salary", "Not stated", muted: true)
                fact("Where they hire", "Americas", quote: "fully remote across the Americas")
                fact("Timezone", "US Eastern, 4 h overlap", quote: "At least 4 hours of overlap with US Eastern time.")
                fact("Stack", "React, TypeScript, Node.js, PostgreSQL")
                fact("Hiring", "Contractor", quote: "Paid in USD as a contractor")
                Text("Read by qwen3-8b with prompt version 4, 4 Oct 2026 at 09:12.").font(.ui(11)).foregroundStyle(tertiaryInk)
                SecondaryButton(title: "Read facts now", symbol: "arrow.clockwise").padding(.top, Space.xs)
            }
            .marker(3, x: -30)
            VStack(alignment: .leading, spacing: Space.s) {
                HStack {
                    Text("Posting").font(.ui(13, .semibold)).foregroundStyle(ink)
                    Spacer()
                    LinkText(text: "Open")
                }
                Text(SampleJob.flattened.joined(separator: "\n"))
                    .font(.ui(13)).foregroundStyle(ink).lineSpacing(1.5)
                    .fixedSize(horizontal: false, vertical: true)
                    .overlay(alignment: .topLeading) { Marker(number: 5).offset(x: -30, y: 70) }
                    .overlay(alignment: .topLeading) { Marker(number: 6).offset(x: -30, y: 190) }
            }
            .marker(4, x: -30)
        }
        .padding(Space.l)
    }

    private func fact(_ label: String, _ value: String, quote: String? = nil, muted: Bool = false) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Space.m) {
            Text(label).font(.ui(12)).foregroundStyle(secondaryInk).frame(width: 96, alignment: .trailing)
            VStack(alignment: .leading, spacing: 3) {
                Text(value).font(.ui(12)).foregroundStyle(muted ? secondaryInk : ink)
                if let quote { Evidence(text: quote) }
            }
            Spacer(minLength: 0)
        }
    }
}

// MARK: - The Posting tab proposed

/// The facts that decide whether to read on, in two columns, each with where
/// it came from: the board's own field, or read from the words.
struct KeyFacts: View {
    var columns = 2
    var body: some View {
        let facts: [(String, String, Tone?, String)] = [
            ("Pay", "Not published", nil, "board"),
            ("Hiring", "Contractor, USD", nil, "read"),
            ("Where they hire", "Americas", .positive, "read"),
            ("Timezone", "4 h with US Eastern", .caution, "read"),
            ("Level", "Senior · 6+ years", .caution, "read"),
            ("Stack", "React, TS, Node, Postgres", .positive, "read"),
        ]
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: Space.l, verticalSpacing: Space.s + 2) {
            ForEach(0..<(facts.count / columns), id: \.self) { row in
                GridRow {
                    ForEach(0..<columns, id: \.self) { column in
                        let fact = facts[row * columns + column]
                        VStack(alignment: .leading, spacing: 1) {
                            HStack(spacing: 4) {
                                Text(fact.0).font(.ui(10.5, .medium)).foregroundStyle(secondaryInk)
                                if fact.3 == "read" {
                                    Image(systemName: "text.viewfinder").font(.system(size: 8.5)).foregroundStyle(tertiaryInk)
                                }
                            }
                            HStack(spacing: 4) {
                                if let tone = fact.2 {
                                    Image(systemName: tone == .positive ? "checkmark.circle.fill" : "questionmark.circle.fill")
                                        .font(.system(size: 10)).foregroundStyle(tone.color)
                                }
                                Text(fact.1).font(.ui(12.5)).foregroundStyle(fact.1 == "Not published" ? secondaryInk : ink).lineLimit(1)
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            }
        }
    }
}

/// The posting drawn as a reader: its own headings and lists, a line length
/// that reads, and the phrases the screen and the brief quoted marked.
struct PostingReader: View {
    var size: CGFloat = 13
    var measure: CGFloat = .infinity
    var highlights = true
    var showsNotes = false
    var limit: Int?
    var body: some View {
        let blocks = limit.map { Array(SampleJob.posting.prefix($0)) } ?? SampleJob.posting
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { index, block in
                switch block {
                case let .heading(text):
                    Text(text).font(.system(size: size + 2, weight: .semibold)).foregroundStyle(ink)
                        .padding(.top, index == 0 ? 0 : size * 1.25).padding(.bottom, size * 0.45)
                case let .paragraph(text):
                    SampleJob.styled(text, size: size, highlights: highlights).font(.system(size: size)).foregroundStyle(ink)
                        .lineSpacing(size * 0.32).fixedSize(horizontal: false, vertical: true)
                case let .bullet(text):
                    HStack(alignment: .firstTextBaseline, spacing: Space.s) {
                        Text("•").font(.system(size: size, weight: .bold)).foregroundStyle(tertiaryInk)
                        SampleJob.styled(text, size: size, highlights: highlights).font(.system(size: size)).foregroundStyle(ink)
                            .lineSpacing(size * 0.32).fixedSize(horizontal: false, vertical: true)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .padding(.bottom, size * 0.35)
                    .overlay(alignment: .trailing) {
                        if showsNotes, let note = note(for: text) {
                            note.offset(x: note.width + 18)
                        }
                    }
                }
            }
        }
        .frame(maxWidth: measure, alignment: .leading)
    }

    private func note(for text: String) -> MarginNote? {
        if text.hasPrefix("[[6+") { return MarginNote(symbol: "questionmark.circle.fill", tone: .caution, title: "Screen · Years", text: "Asks 6+, you have 5 in React") }
        if text.hasPrefix("[[At least") { return MarginNote(symbol: "questionmark.circle.fill", tone: .caution, title: "Screen · Timezone", text: "4 h overlap: unclear from Brasília") }
        if text.hasPrefix("GraphQL") { return MarginNote(symbol: "minus.circle.fill", tone: .caution, title: "Brief · Gap", text: "No production federation") }
        return nil
    }
}

/// A note in the reader's margin, tied to the line beside it.
struct MarginNote: View {
    let symbol: String
    let tone: Tone
    let title: String
    let text: String
    var width: CGFloat = 210
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Image(systemName: symbol).font(.system(size: 11)).foregroundStyle(tone.color)
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.ui(11, .semibold)).foregroundStyle(tone.color)
                Text(text).font(.ui(11.5)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.leading, 10)
        .frame(width: width, alignment: .leading)
        .overlay(alignment: .leading) { Rectangle().fill(tone.color.opacity(0.5)).frame(width: 2) }
    }
}

/// The posting's sections, to jump to; the one in view is marked.
struct Outline: View {
    var current = "What you bring"
    var vertical = false
    var body: some View {
        let names = ["About", "What you'll do", "What you bring", "Nice to have", "Benefits", "How we hire"]
        if vertical {
            VStack(alignment: .leading, spacing: 7) {
                Text("ON THIS POSTING").font(.ui(10, .bold)).kerning(0.5).foregroundStyle(tertiaryInk).padding(.bottom, 2)
                ForEach(names, id: \.self) { name in
                    HStack(spacing: 8) {
                        Rectangle().fill(name == current ? Tone.accent.color : separator).frame(width: 2, height: 14)
                        Text(name).font(.ui(12, name == current ? .semibold : .regular)).foregroundStyle(name == current ? ink : secondaryInk)
                    }
                }
            }
        } else {
            FlowLayout(spacing: 6) {
                ForEach(names, id: \.self) { name in
                    Text(name).font(.ui(11.5, name == current ? .semibold : .regular))
                        .foregroundStyle(name == current ? Tone.accent.color : secondaryInk)
                        .padding(.horizontal, 8).padding(.vertical, 3)
                        .background(name == current ? Tone.accent.fill : fillSubtle, in: Capsule())
                }
            }
        }
    }
}

/// What a highlight means, under the outline.
struct HighlightLegend: View {
    var body: some View {
        HStack(spacing: Space.m) {
            legend(highlightCaution, "Screen is unclear")
            legend(highlightPositive, "Screen passes")
            Spacer(minLength: 0)
            HStack(spacing: 4) {
                Keycap(key: "⌘F")
                Text("Find").font(.ui(11)).foregroundStyle(tertiaryInk)
            }
        }
    }

    private func legend(_ color: Color, _ text: String) -> some View {
        HStack(spacing: 5) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 14, height: 9)
            Text(text).font(.ui(11)).foregroundStyle(secondaryInk)
        }
    }
}

/// The Posting tab as proposed: key facts on a card, then the posting as a
/// reader, with its outline and its marks; who read it sits in the menu.
struct ProposedPostingTab: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.l) {
            VStack(alignment: .leading, spacing: Space.m) {
                HStack {
                    Text("Key facts").font(.ui(13, .semibold)).foregroundStyle(ink)
                    Spacer()
                    Image(systemName: "ellipsis.circle").font(.system(size: 13)).foregroundStyle(secondaryInk)
                }
                KeyFacts()
            }
            .padding(Space.m + 2)
            .background(well, in: RoundedRectangle(cornerRadius: Radius.card))
            .marker(1, x: -30)
            VStack(alignment: .leading, spacing: Space.m) {
                HStack(spacing: 6) {
                    Image(systemName: "doc.plaintext").font(.system(size: 11)).foregroundStyle(secondaryInk)
                    Text("From Northwind's Greenhouse board").font(.ui(11.5)).foregroundStyle(secondaryInk)
                    Spacer()
                    LinkText(text: "Original", symbol: "arrow.up.right")
                }
                Outline()
                HighlightLegend()
            }
            .marker(2, x: -30)
            PostingReader()
                .marker(3, x: -30)
                .overlay(alignment: .topLeading) { Marker(number: 4).offset(x: -30, y: 190) }
        }
        .padding(.horizontal, Space.l)
        .padding(.top, Space.m)
        .padding(.bottom, Space.l)
    }
}

// MARK: - Board

struct PostingBeforeAfter: View {
    var body: some View {
        Board(
            eyebrow: "Page section · the job description",
            title: "The posting becomes something you can read",
            subtitle: "Today the job description is the last of four stacked blocks on the Posting tab, as one run of plain text: the server drops the board's headings and lists, and the app draws what's left at the inspector's width. The proposal keeps the posting's structure, puts the facts that decide on top, and marks the lines the screen quoted.",
            width: 1660
        ) {
            HStack(alignment: .top, spacing: 64) {
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Today", subtitle: "Posting tab, 460 pt wide. The posting starts below the first screen.", tone: .negative)
                    PanelFrame(width: 460) {
                        VStack(alignment: .leading, spacing: 0) {
                            InspectorBar(expands: false)
                            CurrentJobTop()
                            CurrentPostingTab()
                        }
                    }
                    .overlay(alignment: .topLeading) {
                        // Where a 860 pt window's first screen ends.
                        HStack(spacing: 6) {
                            Rectangle().fill(annotation).frame(height: 1.5)
                            Text("first screen ends").font(.ui(10.5, .semibold)).foregroundStyle(annotation).fixedSize()
                        }
                        .frame(width: 560)
                        .offset(x: -20, y: 760)
                    }
                    .overlay(alignment: .topLeading) { Marker(number: 1).offset(x: -30, y: 190) }
                }
                .padding(.leading, 30)
                VStack(alignment: .leading, spacing: Space.l) {
                    ColumnHeading(title: "Proposed", subtitle: "The same tab, the same width: facts, then the posting.", tone: .positive)
                    PanelFrame(width: 460) {
                        VStack(alignment: .leading, spacing: 0) {
                            InspectorBar()
                            ProposedJobTop()
                            ProposedPostingTab()
                        }
                    }
                }
                .padding(.leading, 30)
                VStack(alignment: .leading, spacing: Space.xl) {
                    Text("Today").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "The tab opens on facts, not on the posting", fix: "Key facts sit on one card at the top; the posting follows straight under it.")
                    ProblemNote(number: 2, problem: "Board facts and read facts are two lists of the same kind of thing", fix: "One grid of six, each marked board or read, with ✓ or ? where the screen looked at it.")
                    ProblemNote(number: 3, problem: "Quotes, the model's name and Read facts now are always on show", fix: "Quotes move into the posting as marks; who read it and Read again go to the card's ⋯ menu.")
                    ProblemNote(number: 4, problem: "The posting is one block of plain text", fix: "The server keeps the board's headings, lists and bold as Markdown; the app draws them.")
                    ProblemNote(number: 5, problem: "Headings and list items look the same as sentences", fix: "Headings get the section size and space above; items get bullets and room between.")
                    ProblemNote(number: 6, problem: "Nothing ties the posting to the verdicts", fix: "Lines an unclear or failing check quoted are marked in caution; lines a passing check quoted, in positive.")
                    Divider().padding(.vertical, Space.s)
                    Text("Proposed").font(.ui(15, .semibold)).foregroundStyle(ink)
                    ProblemNote(number: 1, problem: "Key facts", fix: "Pay, hiring, where, timezone, level, stack: what decides whether to read on.")
                    ProblemNote(number: 2, problem: "Source, outline and legend", fix: "Where the text came from, a link to the original, the posting's sections to jump to, and ⌘F.")
                    ProblemNote(number: 3, problem: "The reader", fix: "13 pt with 1.3 line height, headings, bullets, bold. Opened as a page, it keeps a 640 pt measure.")
                    ProblemNote(number: 4, problem: "Marks", fix: "Hovering a mark shows the check that quoted it; In posting on a screen row scrolls here.")
                    Divider().padding(.vertical, Space.s)
                    ServerChange()
                }
                .frame(width: 460)
            }
        }
    }
}

/// What the server does with a board's HTML, today and proposed.
struct ServerChange: View {
    var body: some View {
        VStack(alignment: .leading, spacing: Space.m) {
            Text("On the server").font(.ui(15, .semibold)).foregroundStyle(ink)
            code("Board HTML", "<h3>What you bring</h3>\n<ul><li><b>6+ years</b> building…</li>", tone: .neutral)
            HStack(alignment: .top, spacing: Space.m) {
                code("Today: ConvertHTMLToText", "What you bring\n6+ years building…", tone: .negative)
                code("Proposed: ConvertHTMLToMarkdown", "### What you bring\n- **6+ years** building…", tone: .positive)
            }
            Text("Facts, briefs and the screen read Markdown as well as plain text. Postings already stored are converted again the next time their board is read.")
                .font(.ui(12)).foregroundStyle(secondaryInk).fixedSize(horizontal: false, vertical: true)
        }
    }

    private func code(_ title: String, _ text: String, tone: Tone) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 5) {
                Circle().fill(tone.color).frame(width: 6, height: 6)
                Text(title).font(.ui(11, .semibold)).foregroundStyle(secondaryInk)
            }
            Text(text).font(.system(size: 11, design: .monospaced)).foregroundStyle(ink).fixedSize(horizontal: false, vertical: true)
                .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                .background(well, in: RoundedRectangle(cornerRadius: 6))
        }
    }
}
