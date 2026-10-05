import Foundation
@testable import JobSearchHubCore
import Testing

// Made-up names only: fixtures of a real search would put real people and
// companies in the repository.

private func makeJob(_ title: String, company: String? = nil, location: String? = nil, level: FitLevel = .good) -> PaletteItem {
    var job = Job(id: UUID(), source: "manual", title: title, url: "https://example.com/job", firstSeenAt: .now, lastSeenAt: .now)
    job.location = location
    return .job(JobListItem(job: job, companyName: company, fit: JobFit(level: level, checks: []), unseenUpdates: 0))
}

private func makeCompany(_ name: String, domain: String) -> PaletteItem {
    PaletteItem(kind: .company, target: .open(.company(UUID())), title: name, detail: domain, symbolName: "building.2")
}

private func makePerson(_ name: String, detail: String, keywords: [String] = []) -> PaletteItem {
    PaletteItem(
        kind: .person, target: .open(.person(PersonReference(key: "contact:\(UUID())", companyID: nil))), title: name, detail: detail,
        symbolName: "person", keywords: keywords
    )
}

private let pages = Page.allCases.map(PaletteItem.page)
private let actions = PaletteAction.allCases.map(PaletteItem.action)

private func getTitles(_ sections: [PaletteSection]) -> [[String]] {
    sections.map { $0.items.map(\.title) }
}

@Test func nothingTypedListsThePagesThenTheActions() {
    let items = [makeJob("Senior Product Engineer"), makeCompany("Northwind", domain: "northwind.example")] + pages + actions

    let sections = PaletteSearch.rank(items, query: "  ")

    #expect(sections.map(\.kind) == [.page, .action])
    #expect(sections[0].items.map(\.title) == Page.allCases.map(\.title))
    #expect(sections[1].items.map(\.title) == PaletteAction.allCases.map(\.title))
}

@Test func theGroupWithTheBestMatchComesFirst() {
    let items = [
        makeJob("Senior Product Engineer", company: "Northwind", location: "Remote"),
        makeJob("Staff Platform Engineer", company: "Northwind"),
        makeCompany("Northwind", domain: "northwind.example"),
        makePerson("Alex Example", detail: "Connection at Northwind"),
        makePerson("Sam Example", detail: "Recruiter at Globex"),
    ] + pages + actions

    let sections = PaletteSearch.rank(items, query: "north")

    // The company is named "north…"; the jobs and person only work there.
    #expect(sections.map(\.kind) == [.company, .job, .person])
    #expect(getTitles(sections) == [["Northwind"], ["Senior Product Engineer", "Staff Platform Engineer"], ["Alex Example"]])
}

@Test func aTitleMatchRanksAboveADetailMatch() {
    let items = [
        makeJob("Backend Engineer", company: "Payments Inc"),
        makeJob("Payments Engineer", company: "Globex"),
        makeJob("Engineer, Payment Systems", company: "Initech"),
    ]

    let titles = getTitles(PaletteSearch.rank(items, query: "payment"))

    // The title's first word, then a later word of the title, then the company.
    #expect(titles == [["Payments Engineer", "Engineer, Payment Systems", "Backend Engineer"]])
}

@Test func everyWordTypedHasToMatch() {
    let items = [
        makeJob("Senior Product Engineer", company: "Northwind"),
        makeJob("Senior Product Engineer", company: "Globex"),
        makeCompany("Northwind", domain: "northwind.example"),
    ]

    let sections = PaletteSearch.rank(items, query: "senior north")

    #expect(sections.map(\.kind) == [.job])
    #expect(sections[0].items.map(\.detail) == ["Northwind"])
}

@Test func theWholeTitleTypedComesFirst() {
    let items = [makeJob("Data Pipeline Engineer"), makeJob("Pipeline Lead")] + pages + actions

    let sections = PaletteSearch.rank(items, query: "Pipeline")

    #expect(sections.first?.kind == .page)
    #expect(sections.first?.items.map(\.target) == [.page(.pipeline)])
    #expect(sections.last?.items.map(\.title) == ["Pipeline Lead", "Data Pipeline Engineer"])
}

@Test func equalMatchesKeepTheOrderGiven() {
    let items = [makeJob("Engineer III"), makeJob("Engineer I"), makeJob("Engineer II")]

    #expect(getTitles(PaletteSearch.rank(items, query: "eng")) == [["Engineer III", "Engineer I", "Engineer II"]])
}

@Test func caseAndAccentsDoNotMatter() {
    let items = [makeJob("Ingénieur logiciel", location: "Zürich"), makeJob("Designer", location: "Berlin")]

    #expect(getTitles(PaletteSearch.rank(items, query: "INGENIEUR")) == [["Ingénieur logiciel"]])
    #expect(getTitles(PaletteSearch.rank(items, query: "zurich")) == [["Ingénieur logiciel"]])
}

@Test func initialsFindATitle() {
    let items = [makeJob("Senior Product Engineer"), makeJob("Staff Engineer")]

    #expect(getTitles(PaletteSearch.rank(items, query: "spe")) == [["Senior Product Engineer"]])
}

@Test func actionsAreFoundByWordsTheirTitleDoesNotSay() {
    let sections = PaletteSearch.rank(actions, query: "resume")

    // "Resume local models" by its title first, then the CVs by "resume".
    #expect(getTitles(sections) == [["Resume local models", "Generate missing CVs"]])
    #expect(getTitles(PaletteSearch.rank(actions, query: "cv")) == [["Generate missing CVs"]])
}

@Test func aPersonIsFoundByTheirRole() {
    let items = [makePerson("Alex Example", detail: "Connection at Northwind", keywords: ["Engineering Manager"])]

    #expect(getTitles(PaletteSearch.rank(items, query: "manager")) == [["Alex Example"]])
}

@Test func eachKindListsAFewRowsAtMost() {
    let items = (1...20).map { makeJob("Engineer \($0)") } + (1...20).map { makeCompany("Engine \($0)", domain: "engine\($0).example") }

    let sections = PaletteSearch.rank(items, query: "engin")

    #expect(sections.map(\.items.count) == [PaletteKind.job.limit, PaletteKind.company.limit])
    #expect(sections[0].items.first?.title == "Engineer 1")
}

@Test func whatMatchesNothingIsLeftOut() {
    #expect(PaletteSearch.rank([makeJob("Designer")] + pages + actions, query: "zzz").isEmpty)
}

@Test func theRowsSayWhatTheyAre() throws {
    let job = makeJob("Senior Product Engineer", company: "Northwind", location: "Remote, Americas", level: .unclear)
    #expect(job.detail == "Northwind · Remote, Americas")
    #expect(job.chip == PaletteChip("Screen unclear", tone: .caution))

    let people = try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(#"""
    {"people":[{"key":"connection:dddddddd-0000-0000-0000-000000000001","relation":"connection","id":"dddddddd-0000-0000-0000-000000000001",
      "name":"Alex Example","role":"Engineering Manager","company_id":"cccccccc-0000-0000-0000-000000000002","company_name":"Northwind",
      "is_agency":false,"open_jobs":1,"fitting_jobs":0}]}
    """#.utf8)).people
    let person = PaletteItem.person(people[0])
    #expect(person.detail == "Connection at Northwind")
    #expect(person.target == .open(.person(people[0].reference)))
    #expect(getTitles(PaletteSearch.rank([person], query: "engineering")) == [["Alex Example"]])
}

@Test func theActionsOfferedFollowWhatTheyWouldChange() {
    #expect(PaletteAction.getAvailable(isModelWorkPaused: false, unseenUpdates: 0) == [
        .addCompany, .addCompanyFromSuggestions, .addJobByURL, .generateMissingCVs, .pauseLocalModels,
    ])
    #expect(PaletteAction.getAvailable(isModelWorkPaused: true, unseenUpdates: 2).suffix(2) == [.resumeLocalModels, .markAllUpdatesSeen])
    #expect(!PaletteAction.getAvailable(isModelWorkPaused: nil, unseenUpdates: 0).contains { $0 == .pauseLocalModels || $0 == .resumeLocalModels })
}

@Test func keysDecideTheSelectedJob() {
    #expect(KeyboardDecision.getDecision(for: "p") == .pursue)
    #expect(KeyboardDecision.getDecision(for: "L") == .later)
    #expect(KeyboardDecision.getDecision(for: "s") == .skip)
    #expect(KeyboardDecision.getDecision(for: "x") == nil)
}

@Test func theNextJobComesUpAfterADecision() {
    let ids = [UUID(), UUID(), UUID()]

    #expect(KeyboardDecision.getNextID(after: ids[0], in: ids) == ids[1])
    #expect(KeyboardDecision.getNextID(after: ids[2], in: ids) == ids[1])
    #expect(KeyboardDecision.getNextID(after: ids[0], in: [ids[0]]) == nil)
    #expect(KeyboardDecision.getNextID(after: UUID(), in: ids) == nil)
}

@Test func arrowsMoveThroughTheJobsAndStopAtTheEnds() {
    let ids = [UUID(), UUID(), UUID()]

    #expect(KeyboardDecision.move(from: nil, by: 1, in: ids) == ids[0])
    #expect(KeyboardDecision.move(from: ids[0], by: 1, in: ids) == ids[1])
    #expect(KeyboardDecision.move(from: ids[2], by: 1, in: ids) == ids[2])
    #expect(KeyboardDecision.move(from: ids[0], by: -1, in: ids) == ids[0])
    #expect(KeyboardDecision.move(from: UUID(), by: -1, in: ids) == ids[0])
    #expect(KeyboardDecision.move(from: nil, by: 1, in: []) == nil)
}
