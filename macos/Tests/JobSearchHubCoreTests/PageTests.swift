import JobSearchHubCore
import Testing

@Test func theSidebarListsTheFivePagesInOrder() {
    #expect(Page.allCases.map(\.title) == ["Pipeline", "Jobs", "Companies", "Profile", "Settings"])
}

@Test func eachPageNamesItsOwnSymbol() {
    #expect(Set(Page.allCases.map(\.symbolName)).count == Page.allCases.count)
}
