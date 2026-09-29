import Foundation
@testable import JobSearchHubCore
import Testing

private func makeItem(
    _ title: String, level: FitLevel = .good, source: String = "himalayas", workplace: String? = "Remote",
    employment: String? = "Full-time", checks: [FitCheck] = [], pay: Pay? = nil, firstSeen: TimeInterval = 0
) -> JobListItem {
    var job = Job(id: UUID(), source: source, title: title, url: "https://acme.com/\(title)",
                  firstSeenAt: Date(timeIntervalSince1970: firstSeen), lastSeenAt: Date(timeIntervalSince1970: firstSeen))
    job.workplaceType = workplace
    job.employmentType = employment
    job.pay = pay
    return JobListItem(job: job, companyName: nil, fit: JobFit(level: level, checks: checks), unseenUpdates: 0)
}

private func getTitles(_ filter: JobsFilter, _ items: [JobListItem], previousVisit: Date? = nil) -> [String] {
    filter.getMatchingItems(items, previousVisit: previousVisit).map(\.job.title)
}

@Test func anEmptyFilterShowsEveryJobAndIsInactive() {
    let items = [makeItem("a", level: .poor), makeItem("b", workplace: nil)]

    #expect(!JobsFilter().isActive)
    #expect(getTitles(JobsFilter(), items) == ["a", "b"])
}

@Test func fitLevelsSourcesAndEmploymentTypesHide() {
    let items = [
        makeItem("poor", level: .poor), makeItem("alert", source: "indeed"),
        makeItem("contract", employment: "Contract"), makeItem("unstated", employment: nil), makeItem("kept"),
    ]
    var filter = JobsFilter()
    filter.hiddenFitLevels = [.poor]
    filter.hiddenSources = ["indeed"]
    filter.hiddenEmploymentTypes = ["Contract", Job.unstatedName]

    #expect(filter.isActive)
    #expect(getTitles(filter, items) == ["kept"])
}

@Test func workplaceSpellingsReadAsOne() {
    let items = [makeItem("dashed", workplace: "On-site"), makeItem("joined", workplace: "OnSite"), makeItem("remote"), makeItem("blank", workplace: " ")]
    var filter = JobsFilter()
    filter.hiddenWorkplaces = ["On-site", Job.unstatedName]

    #expect(getTitles(filter, items) == ["remote"])
    #expect(JobsFilterChoices(items: items).workplaces.map(\.title) == ["On-site", "Remote", Job.unstatedName])
}

@Test func aHiddenCheckFailureHidesOnlyJobsThatFailIt() {
    let items = [
        makeItem("fails", checks: [FitCheck(name: "Stack", verdict: .no, reason: "")]),
        makeItem("unclear", checks: [FitCheck(name: "Stack", verdict: .unclear, reason: "")]),
        makeItem("unchecked"),
        makeItem("fails another", checks: [FitCheck(name: "Level", verdict: .no, reason: "")]),
    ]
    var filter = JobsFilter()
    filter.hiddenCheckFailures = ["Stack"]

    #expect(getTitles(filter, items) == ["unclear", "unchecked", "fails another"])
}

@Test func payAndNewnessNarrowTheList() {
    let pay = Pay(ranges: [PayRange(min: 1, max: 2, currency: "USD", interval: "year")])
    let items = [makeItem("paid old", pay: pay, firstSeen: 100), makeItem("paid new", pay: pay, firstSeen: 300), makeItem("unpaid new", firstSeen: 300)]
    var filter = JobsFilter()
    filter.showsOnlyJobsWithPay = true
    filter.showsOnlyNewJobs = true

    #expect(getTitles(filter, items, previousVisit: Date(timeIntervalSince1970: 200)) == ["paid new"])
}

@Test func choicesCountTheLoadedJobs() {
    let items = [
        makeItem("a", level: .good, source: "himalayas", checks: [FitCheck(name: "Stack", verdict: .no, reason: "")]),
        makeItem("b", level: .poor, source: "himalayas", checks: [FitCheck(name: "Stack", verdict: .no, reason: "")]),
        makeItem("c", level: .poor, source: "job_board", pay: Pay(ranges: [])),
    ]
    let choices = JobsFilterChoices(items: items)

    #expect(choices.fitLevelCounts == [.good: 1, .poor: 2])
    #expect(choices.checkFailureCounts == ["Stack": 2])
    #expect(choices.sources == [
        JobsFilterChoice(value: "himalayas", title: "Himalayas", count: 2), JobsFilterChoice(value: "job_board", title: "Company board", count: 1),
    ])
    #expect(choices.withPayCount == 1)
}

@Test func aFilterSurvivesEncoding() throws {
    var filter = JobsFilter()
    filter.hiddenFitLevels = [.poor]
    filter.showsOnlyNewJobs = true

    #expect(try JSONDecoder().decode(JobsFilter.self, from: JSONEncoder().encode(filter)) == filter)
}
