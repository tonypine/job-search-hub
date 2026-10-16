@testable import JobSearchHub
import JobSearchHubCore
import Testing

@MainActor @Test func aParagraphAfterAListGetsAParagraphsSpaceAbove() {
    let document = PostingDocument(markdown: """
    ### What you bring

    - 6+ years building web products with React.
    - At least 4 hours of overlap with US Eastern time.

    We hire across the Americas.

    - Remote experience is a plus.
    """)

    #expect(document.blocks.map(\.kind) == [.heading(level: 3), .bullet, .bullet, .paragraph, .bullet])
    #expect(PostingReader.getSpaceBelow(1, in: document) == PostingReader.itemSpace)
    #expect(PostingReader.getSpaceBelow(2, in: document) == PostingReader.paragraphSpace)
    #expect(PostingReader.getSpaceBelow(3, in: document) == PostingReader.paragraphSpace)
    #expect(PostingReader.getSpaceBelow(4, in: document) == PostingReader.paragraphSpace)
}
