import JobSearchHubCore
import Testing

/// A posting as the hub keeps it, from a board's HTML.
@Test func aPostingsMarkdownBecomesHeadingsBulletsAndParagraphs() {
    let blocks = MarkdownBlocks.parse("""
    ### What you'll do

    Build the **checkout** app.

    - Ship **React** features
      - Weekly
    - Talk to customers

    1. Apply
    2. Talk to us
    """)

    #expect(blocks == [
        .heading(level: 3, text: "What you'll do"),
        .paragraph("Build the **checkout** app."),
        .bullet("Ship **React** features"),
        .bullet("Weekly"),
        .bullet("Talk to customers"),
        .paragraph("1. Apply\n2. Talk to us"),
    ])
    for block in blocks {
        if case .heading(_, let text) = block { #expect(!text.hasPrefix("#")) }
        if case .bullet(let text) = block { #expect(!text.hasPrefix("- ")) }
    }
}
