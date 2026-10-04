import JobSearchHubCore
import Testing

@Test func theSidebarListsThePagesInOrder() {
    #expect(SidebarGroup.allCases.flatMap(\.pages).map(\.title) == [
        "Today", "Decide", "Pipeline", "Jobs", "Companies", "People", "Profile", "Criteria", "Activity", "Prompts", "Model lab",
    ])
}

@Test func todayReplacesUpdatesInTheSidebar() {
    #expect(Page.allCases.first == .today)
    #expect(!Page.updates.isInSidebar)
    #expect(Page.allCases.filter { !$0.isInSidebar } == [.updates])
    #expect(Page(rawValue: "updates") == .updates)
}

@Test func eachPageNamesItsOwnSymbol() {
    #expect(Set(Page.allCases.map(\.symbolName)).count == Page.allCases.count)
}

@Test func theSidebarGroupsPagesByIntent() {
    #expect(SidebarGroup.allCases.map(\.title) == [nil, "Browse", "You", "Hub"])
    #expect(SidebarGroup.work.pages == [.today, .decide, .pipeline])
    #expect(SidebarGroup.browse.pages == [.jobs, .companies, .people])
    #expect(SidebarGroup.you.pages == [.profile, .criteria])
    #expect(SidebarGroup.hub.pages == [.activity, .prompts, .modelLab])
}

@Test func theGroupsKeepThePagesInSidebarOrder() {
    #expect(SidebarGroup.allCases.flatMap(\.pages) == Page.allCases.filter(\.isInSidebar))
}

@Test func onlyTheHubStartsCollapsed() {
    #expect(SidebarGroup.allCases.filter { !$0.isExpandedByDefault } == [.hub])
}

@Test func pagesOpenFromTheirLaunchArgument() {
    #expect(Page(rawValue: "today") == .today)
    #expect(Page(rawValue: "activity") == .activity)
    #expect(Page(rawValue: "model-lab") == .modelLab)
    #expect(Page(rawValue: "people") == .people)
    #expect(Page(rawValue: "recruiters") == nil)
    #expect(Page(rawValue: "settings") == nil)
}
