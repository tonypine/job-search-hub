import JobSearchHubCore
import Testing

@Test func theSidebarListsTheFivePagesInOrder() {
    #expect(Page.allCases.map(\.title) == ["Pipeline", "Updates", "Jobs", "Companies", "Recruiters", "Profile", "Settings"])
}

@Test func eachPageNamesItsOwnSymbol() {
    #expect(Set(Page.allCases.map(\.symbolName)).count == Page.allCases.count)
}
