import JobSearchHubCore
import Testing

/// A posting as the hub keeps it since TP-667: headings, list items and bold
/// as Markdown.
private let posting = """
Northwind builds payment software for independent retailers. We\u{2019}re 140 people, fully remote across the Americas.

### What you'll do

- Own the **checkout rebuild** end to end, from the API to the last pixel.
- Ship every week with a small team.

### What you bring

- **6+ years** building web products with React and TypeScript.
- At least 4 hours of overlap with US Eastern time.
- Remote experience is a plus.
"""

private func quote(_ text: String, verdict: FitVerdict = .unclear) -> PostingQuote {
    PostingQuote(name: "Experience", verdict: verdict, reason: "asks for 6 years; you have 5 years", text: text)
}

/// The words each mark covers, as the reader shows them.
private func getMarkedText(_ marks: [PostingMark], in document: PostingDocument) -> [String] {
    marks.map { mark in String(Array(document.blocks[mark.span.block].plainText)[mark.span.range]) }
}

@Test func aPostingShowsItsHeadingsAsAnOutlineAndItsTextWithoutMarkdown() {
    let document = PostingDocument(markdown: posting)

    #expect(document.outline.map(\.title) == ["What you'll do", "What you bring"])
    #expect(document.blocks.map(\.kind) == [.paragraph, .heading(level: 3), .bullet, .bullet, .heading(level: 3), .bullet, .bullet, .bullet])
    #expect(document.blocks[2].plainText == "Own the checkout rebuild end to end, from the API to the last pixel.")
    #expect(!document.blocks.contains { $0.plainText.contains("**") || $0.plainText.hasPrefix("- ") || $0.plainText.hasPrefix("#") })
}

@Test func aQuoteThatSpansBoldTextIsMarked() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("6+ years building web products with React")])

    #expect(marks.map(\.span.block) == [5])
    #expect(getMarkedText(marks, in: document) == ["6+ years building web products with React"])
    #expect(marks.first?.quote.help == "Screen · Experience: asks for 6 years; you have 5 years")
}

@Test func aQuoteThatStartsAListItemIsMarked() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("- At least 4 hours of overlap with US Eastern time.")])

    #expect(getMarkedText(marks, in: document) == ["At least 4 hours of overlap with US Eastern time."])
}

@Test func aQuoteReadFromTheOldPlainTextPostingIsMarkedAcrossBlocks() {
    let document = PostingDocument(markdown: posting)
    // Before TP-667 the posting was one run of plain text, so a quote could
    // run from a heading into the line under it.
    let marks = document.findMarks(for: [quote("What you bring\n6+ years building web products")])

    #expect(marks.map(\.span.block) == [4, 5])
    #expect(getMarkedText(marks, in: document) == ["What you bring", "6+ years building web products"])
}

@Test func quotesMatchWhateverTheirCaseQuotesAndSpacing() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("\u{201C}we're 140 people,   FULLY remote\u{201D}", verdict: .yes)])

    #expect(getMarkedText(marks, in: document) == ["We\u{2019}re 140 people, fully remote"])
    #expect(marks.first?.quote.verdict == .yes)
}

@Test func aQuoteCutWithAnEllipsisMarksEachPart() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("Own the checkout rebuild … with a small team")])

    #expect(getMarkedText(marks, in: document) == ["Own the checkout rebuild", "with a small team"])
}

@Test func aQuoteCutWithThreeDotsMarksEachPartAndDropsPartsTooShortToPlace() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("Own the checkout rebuild ... to ... with a small team")])

    #expect(getMarkedText(marks, in: document) == ["Own the checkout rebuild", "with a small team"])
}

@Test func aQuoteWithMarkdownEmphasisOrAHeadingMarkerIsMarked() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("**6+ years** building web products"), quote("### What you bring")])

    #expect(marks.map(\.span.block) == [5, 4])
    #expect(getMarkedText(marks, in: document) == ["6+ years building web products", "What you bring"])
}

@Test func aQuoteEndingInOtherPunctuationIsMarkedWithoutIt() {
    let document = PostingDocument(markdown: posting)
    let marks = document.findMarks(for: [quote("Ship every week with a small team!")])

    #expect(getMarkedText(marks, in: document) == ["Ship every week with a small team"])
}

@Test func aQuoteNotInThePostingHasNoMark() {
    let document = PostingDocument(markdown: posting)

    #expect(document.findMarks(for: [quote("10+ years of Rust")]).isEmpty)
    #expect(PostingDocument(markdown: "").findMarks(for: [quote("10+ years of Rust")]).isEmpty)
}

@Test func aPlainTextPostingReadsAsParagraphsWithoutAnOutline() {
    // As an alert email, an import or a posting added by hand keeps it.
    let document = PostingDocument(markdown: """
    Northwind builds payment software.
    We hire anywhere in Brazil.

    Benefits
    Paid in USD as a contractor.
    """)

    #expect(document.outline.isEmpty)
    #expect(document.blocks.map(\.kind) == [.paragraph, .paragraph])
    #expect(getMarkedText(document.findMarks(for: [quote("We hire anywhere in Brazil")]), in: document) == ["We hire anywhere in Brazil"])
}

@Test func findMatchesEveryPlaceTheQueryShowsWhateverItsCase() {
    let document = PostingDocument(markdown: posting)
    let matches = document.findMatches(of: "remote")

    #expect(matches.map(\.block) == [0, 7])
    #expect(matches.map(\.range.count) == [6, 6])
    #expect(document.findMatches(of: "  ").isEmpty)
    #expect(document.findMatches(of: "kubernetes").isEmpty)
}
