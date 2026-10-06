import Foundation
@testable import JobSearchHubCore
import Testing

private let now = Date(timeIntervalSince1970: 1_790_600_000)
private let day: TimeInterval = 86_400

private func makeCalendar() -> Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC")!
    return calendar
}

private func makeCard(_ title: String, followUpDueAt: Date?) -> PipelineCard {
    let application = Application(id: UUID(), phaseID: UUID(), phaseEnteredAt: now.addingTimeInterval(-10 * day), createdAt: now, updatedAt: now)
    return PipelineCard(application: application, jobTitle: title, followUpDueAt: followUpDueAt, unseenUpdates: 0)
}

private func makeUpdate(_ kind: String, seen: Bool = false) -> HubUpdate {
    HubUpdate(id: UUID(), sequence: 1, kind: kind, title: kind, createdAt: now, seenAt: seen ? now : nil)
}

private func makeQueue(_ titles: [String]) throws -> [DecisionQueueItem] {
    let items = titles.map { title in
        #"{"job":{"id":"\#(UUID().uuidString)","source":"manual","title":"\#(title)","url":"https://example.com/\#(title)","#
            + #""first_seen_at":"2026-09-28T13:57:13Z","last_seen_at":"2026-09-28T13:57:13Z"},"#
            + #""match":"strong","reason":"Fits.","brief_tier":"pre","fit":{"level":"good","checks":[]}}"#
    }
    let json = #"{"items":[\#(items.joined(separator: ","))],"total":\#(titles.count)}"#
    return try HubJSON.makeDecoder().decode(DecisionQueueResponse.self, from: Data(json.utf8)).items
}

@Test func todayDecidesTheFirstThreeOfTheQueue() throws {
    let queue = try makeQueue(["One", "Two", "Three", "Four", "Five"])

    #expect(Today.getTopDecisions(queue).map(\.job.title) == ["One", "Two", "Three"])
    #expect(Today.getTopDecisions(try makeQueue(["Only"])).map(\.job.title) == ["Only"])
}

@Test func followUpsDueListTheMostOverdueFirstAndLeaveTheRest() {
    let board = PipelineBoard(cards: [
        makeCard("Due today", followUpDueAt: now),
        makeCard("Due tomorrow", followUpDueAt: now.addingTimeInterval(day)),
        makeCard("Overdue 3 days", followUpDueAt: now.addingTimeInterval(-3 * day)),
        makeCard("No follow-up", followUpDueAt: nil),
        makeCard("Overdue 1 day", followUpDueAt: now.addingTimeInterval(-day)),
    ])

    let due = Today.getDueFollowUps(board, now: now, calendar: makeCalendar())

    #expect(due.map(\.card.title) == ["Overdue 3 days", "Overdue 1 day", "Due today"])
    #expect(due.map(\.status) == [.overdue(days: 3), .overdue(days: 1), .dueToday])
}

@Test func todayListsUnseenRepliesConfirmationsAndFreshMatches() {
    let updates = [
        makeUpdate("human_reply"), makeUpdate("interview_invite"), makeUpdate("application_confirmation"), makeUpdate("fresh_match"),
        makeUpdate("rejection", seen: true), makeUpdate("follow_up_due"), makeUpdate("task_finished"), makeUpdate("task_queued"),
    ]

    #expect(Today.getUnseenNews(updates).map(\.kind) == ["human_reply", "interview_invite", "application_confirmation", "fresh_match"])
}

@Test func recruitersWaitWhenUnansweredWithJobsThatFit() throws {
    let json = #"""
    {"people":[
      {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000001","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000001",
       "name":"Older","is_agency":false,"last_contact_at":"2026-08-01T10:00:00Z","answered":false,"open_jobs":3,"fitting_jobs":1},
      {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000002","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000002",
       "name":"Answered","is_agency":false,"last_contact_at":"2026-09-01T10:00:00Z","answered":true,"open_jobs":2,"fitting_jobs":2},
      {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000003","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000003",
       "name":"Nothing fits","is_agency":true,"last_contact_at":"2026-09-02T10:00:00Z","answered":false,"open_jobs":2,"fitting_jobs":0},
      {"key":"recruiter:aaaaaaaa-0000-0000-0000-000000000004","relation":"recruiter","id":"aaaaaaaa-0000-0000-0000-000000000004",
       "name":"Newer","is_agency":true,"last_contact_at":"2026-09-03T10:00:00Z","answered":false,"open_jobs":4,"fitting_jobs":2},
      {"key":"connection:dddddddd-0000-0000-0000-000000000001","relation":"connection","id":"dddddddd-0000-0000-0000-000000000001",
       "name":"A connection","is_agency":false,"open_jobs":1,"fitting_jobs":1}
    ]}
    """#
    let people = try HubJSON.makeDecoder().decode(PeopleResponse.self, from: Data(json.utf8)).people

    #expect(Today.getWaitingRecruiters(people).map(\.name) == ["Newer", "Older"])
}

@Test func theChipsSumTodayUp() {
    let followUps = [
        DueFollowUp(card: makeCard("A", followUpDueAt: nil), status: .overdue(days: 2)),
        DueFollowUp(card: makeCard("B", followUpDueAt: nil), status: .dueToday),
    ]
    let updates = [makeUpdate("human_reply"), makeUpdate("interview_invite"), makeUpdate("fresh_match"), makeUpdate("rejection", seen: true)]

    let chips = Today.getChips(toDecide: 7, followUps: followUps, updates: updates)

    #expect(Today.summarize(chips) == "7 to decide · 1 overdue · 1 due today · 2 replies")
    #expect(chips.map(\.tone) == [.accent, .negative, .caution, .positive])
}

@Test func aCountOfNothingLeavesItsChipOut() {
    let oneReply = Today.getChips(toDecide: 0, followUps: [], updates: [makeUpdate("recruiter_outreach")])

    #expect(oneReply.map(\.text) == ["1 reply"])
    #expect(Today.getChips(toDecide: 0, followUps: [], updates: [makeUpdate("fresh_match")]).isEmpty)
    #expect(Today.summarize([]) == "Nothing needs you now")
}

@Test func theHubCardSaysWhatRunsAndNothingWhileIdle() throws {
    let busyJSON = #"""
    {"paused":false,
     "running":{"kind":"job_facts","model":"local.gguf","priority":"background","since":"2026-09-30T17:40:00Z"},
     "waiting":[{"kind":"job_facts","model":"local.gguf","priority":"background","since":"2026-09-30T17:41:00Z"},
                {"kind":"job_brief","model":"local.gguf","priority":"background","since":"2026-09-30T17:42:00Z"}],
     "runtime":{"state":"ready","model":"local.gguf","busy":true},
     "jobs_awaiting_facts":3}
    """#
    let idleJSON = #"{"paused":true,"waiting":[{"kind":"job_facts","model":"local.gguf","priority":"background","since":"2026-09-30T17:41:00Z"}],"runtime":{"state":"stopped","busy":false},"jobs_awaiting_facts":1}"#
    let busy = try HubJSON.makeDecoder().decode(ModelWork.self, from: Data(busyJSON.utf8))
    let idle = try HubJSON.makeDecoder().decode(ModelWork.self, from: Data(idleJSON.utf8))
    let runs = [
        AgentRun(id: UUID(), kind: "company_triage", input: "example.com", status: "running", startedAt: now),
        AgentRun(id: UUID(), kind: "job_finder", input: "example.org", status: "succeeded", startedAt: now, finishedAt: now),
    ]

    #expect(HubActivity.describe(work: busy, agentRuns: runs) == ["Company research: example.com", "Job facts on the local model · 2 waiting"])
    #expect(HubActivity.describe(work: idle, agentRuns: [runs[1]]).isEmpty)
    #expect(HubActivity.describe(work: nil, agentRuns: []).isEmpty)
}
