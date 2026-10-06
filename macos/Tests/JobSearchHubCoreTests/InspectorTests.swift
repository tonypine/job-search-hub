import Foundation
import JobSearchHubCore
import Testing

private let job = InspectorSubject.job(UUID())
private let company = InspectorSubject.company(UUID())
private let person = InspectorSubject.person(PersonReference(key: "recruiter:\(UUID())"))

@Test func anEmptyHistoryShowsNothingAndGoesNowhere() {
    var history = InspectorHistory()
    history.goBack()
    history.goForward()
    history.select(.posting)

    #expect(history.current == nil && !history.canGoBack && !history.canGoForward)
}

@Test func aJobLinksToItsCompanyAndBackWalksTheHistory() {
    var history = InspectorHistory()
    history.push(job)
    history.push(company, tab: .jobs)

    #expect(history.current == InspectorEntry(company, tab: .jobs))
    #expect(history.canGoBack && !history.canGoForward)

    history.goBack()
    #expect(history.current == InspectorEntry(job))
    #expect(!history.canGoBack && history.canGoForward)

    history.goForward()
    #expect(history.current == InspectorEntry(company, tab: .jobs))
}

@Test func openingAfterGoingBackDropsWhatLayAhead() {
    var history = InspectorHistory()
    history.push(job)
    history.push(company)
    history.goBack()
    history.push(person)

    #expect(history.entries.map(\.subject) == [job, person])
    #expect(!history.canGoForward)
}

@Test func theShownSubjectStaysOneStep() {
    var history = InspectorHistory()
    history.push(job)
    history.select(.posting)
    history.push(job)
    #expect(history.entries == [InspectorEntry(job, tab: .posting)])

    history.push(job, tab: .session)
    #expect(history.entries == [InspectorEntry(job, tab: .session)])
}

@Test func goingBackReturnsToTheTabLeft() {
    var history = InspectorHistory()
    history.push(company)
    history.select(.jobs)
    history.push(job)
    history.goBack()

    #expect(history.current == InspectorEntry(company, tab: .jobs))
}

@Test func theHistoryKeepsItsNewestSteps() {
    var history = InspectorHistory()
    let subjects = (0...InspectorHistory.limit).map { _ in InspectorSubject.job(UUID()) }
    subjects.forEach { history.push($0) }

    #expect(history.entries.count == InspectorHistory.limit)
    #expect(history.entries.first?.subject == subjects[1] && history.current?.subject == subjects.last)
}

@Test func clearingEmptiesTheHistory() {
    var history = InspectorHistory()
    history.push(job)
    history.push(company)
    history.clear()

    #expect(history.current == nil && history.entries.isEmpty && !history.canGoBack)
}

@Test func eachKindHasTheSameTabs() {
    #expect(InspectorTab.getTabs(for: job).map(\.title) == ["Overview", "Posting", "Session"])
    #expect(InspectorTab.getTabs(for: job, hasPrep: true).map(\.title) == ["Overview", "Prep", "Posting", "Session"])
    #expect(InspectorTab.getTabs(for: company).map(\.title) == ["Overview", "Jobs", "People", "Session"])
    #expect(InspectorTab.getTabs(for: person).map(\.title) == ["Overview", "Conversation"])
    #expect(InspectorTab.getTabs(for: .profileInterview) == [.session])
}

@Test func aTabTheSubjectLacksFallsBackToItsFirst() {
    let tabs = InspectorTab.getTabs(for: job)
    #expect(InspectorTab.resolve(.prep, among: tabs) == .overview)
    #expect(InspectorTab.resolve(.posting, among: tabs) == .posting)
    #expect(InspectorTab.resolve(.session, among: InspectorTab.getTabs(for: .profileInterview)) == .session)
}

@Test func sessionSubjectsMapToTheInspectorAndBack() {
    let id = UUID()
    #expect(InspectorSubject(.job(id)) == .job(id) && InspectorSubject(.company(id)) == .company(id))
    #expect(InspectorSubject(.profile) == .profileInterview)
    #expect(InspectorSubject.company(id).sessionSubject == .company(id))
    #expect(InspectorSubject.person(PersonReference(key: "contact:\(id)", companyID: id)).sessionSubject == nil)
}

@Test func aSessionSubjectRoundTripsForItsWindow() throws {
    let subject = ClaudeSessionSubject.job(UUID())
    let decoded = try JSONDecoder().decode(ClaudeSessionSubject.self, from: JSONEncoder().encode(subject))
    #expect(decoded == subject)
}
