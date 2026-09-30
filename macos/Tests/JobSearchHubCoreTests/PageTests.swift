import JobSearchHubCore
import Testing

@Test func theSidebarListsThePagesInOrder() {
    #expect(Page.allCases.map(\.title) == ["Decide", "Pipeline", "Updates", "Jobs", "Companies", "Recruiters", "Profile", "Prompts", "Runs", "Settings"])
}

@Test func eachPageNamesItsOwnSymbol() {
    #expect(Set(Page.allCases.map(\.symbolName)).count == Page.allCases.count)
}
