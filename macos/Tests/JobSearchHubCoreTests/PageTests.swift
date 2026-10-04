import JobSearchHubCore
import Testing

@Test func theSidebarListsThePagesInOrder() {
    #expect(Page.allCases.map(\.title) == [
        "Decide", "Pipeline", "Updates", "Jobs", "Companies", "People", "Profile", "Criteria", "Activity", "Prompts", "Model lab",
    ])
}

@Test func eachPageNamesItsOwnSymbol() {
    #expect(Set(Page.allCases.map(\.symbolName)).count == Page.allCases.count)
}

@Test func theSidebarGroupsPagesByIntent() {
    #expect(SidebarGroup.allCases.map(\.title) == [nil, "Browse", "You", "Hub"])
    #expect(SidebarGroup.work.pages == [.decide, .pipeline, .updates])
    #expect(SidebarGroup.browse.pages == [.jobs, .companies, .people])
    #expect(SidebarGroup.you.pages == [.profile, .criteria])
    #expect(SidebarGroup.hub.pages == [.activity, .prompts, .modelLab])
}

@Test func theGroupsKeepThePagesInSidebarOrder() {
    #expect(SidebarGroup.allCases.flatMap(\.pages) == Page.allCases)
}

@Test func onlyTheHubStartsCollapsed() {
    #expect(SidebarGroup.allCases.filter { !$0.isExpandedByDefault } == [.hub])
}

@Test func pagesOpenFromTheirLaunchArgument() {
    #expect(Page(rawValue: "activity") == .activity)
    #expect(Page(rawValue: "model-lab") == .modelLab)
    #expect(Page(rawValue: "people") == .people)
    #expect(Page(rawValue: "recruiters") == nil)
    #expect(Page(rawValue: "settings") == nil)
}
