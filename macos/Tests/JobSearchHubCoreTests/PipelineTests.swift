import Foundation
@testable import JobSearchHubCore
import Testing

private let savedID = "11111111-0000-0000-0000-000000000001"
private let appliedID = "11111111-0000-0000-0000-000000000002"
private let closedID = "11111111-0000-0000-0000-000000000003"

private let pipelineJSON = """
{"phases":[{"id":"\(closedID)","name":"Closed","position":3,"is_closed":true},
           {"id":"\(savedID)","name":"Saved","position":1,"is_closed":false},
           {"id":"\(appliedID)","name":"Applied","position":2,"is_closed":false}],
 "cards":[{"application":{"id":"22222222-0000-0000-0000-000000000001","job_id":"33333333-0000-0000-0000-000000000001",
                          "company_id":"44444444-0000-0000-0000-000000000001","phase_id":"\(savedID)",
                          "phase_entered_at":"2026-09-20T10:00:00Z","created_at":"2026-09-20T10:00:00Z","updated_at":"2026-09-20T10:00:00Z"},
           "job_title":"Backend Engineer","job_url":"https://acme.com/careers/backend","company_name":"Acme","unseen_updates":1},
          {"application":{"id":"22222222-0000-0000-0000-000000000002","company_id":"44444444-0000-0000-0000-000000000002",
                          "phase_id":"\(savedID)","notes":"Ask about the team.",
                          "phase_entered_at":"2026-09-27T10:00:00.5Z","created_at":"2026-09-27T10:00:00Z","updated_at":"2026-09-27T10:00:00Z"},
           "company_name":"Globex","unseen_updates":0}]}
"""

private func decodeBoard() throws -> PipelineBoard {
    PipelineBoard(try HubJSON.makeDecoder().decode(PipelineResponse.self, from: Data(pipelineJSON.utf8)))
}

@Test func theBoardOrdersPhasesByPositionAndCardsNewestInPhaseFirst() throws {
    let board = try decodeBoard()

    #expect(board.phases.map(\.name) == ["Saved", "Applied", "Closed"])
    #expect(board.getCards(in: board.phases[0]).map(\.title) == ["Globex", "Backend Engineer"])
    #expect(board.getCards(in: board.phases[1]).isEmpty)
    #expect(board.cards[1].jobURL == "https://acme.com/careers/backend")
    #expect(board.cards[0].application.notes == "Ask about the team.")
    #expect(board.cards.map(\.unseenUpdates) == [0, 1])
}

@Test func aMovedApplicationReplacesItsCardAndKeepsTheJob() throws {
    var board = try decodeBoard()
    var moved = board.cards[1].application
    moved.phaseID = UUID(uuidString: appliedID)!
    moved.phaseEnteredAt = Date(timeIntervalSince1970: 1_800_000_000)

    board.replaceApplication(moved)

    let applied = board.getCards(in: board.phases[1])
    #expect(applied.map(\.title) == ["Backend Engineer"])
    #expect(applied[0].companyName == "Acme")
    #expect(board.getCards(in: board.phases[0]).map(\.title) == ["Globex"])
}

@Test func daysInPhaseCountWholeDays() throws {
    let card = try decodeBoard().cards[1]
    let now = card.application.phaseEnteredAt.addingTimeInterval(3 * 86_400 + 3_600)

    #expect(card.getDaysInPhase(now: now) == 3)
}

@Test func requestsUseTheServersSnakeCaseKeys() throws {
    let encoder = HubJSON.makeEncoder()
    encoder.outputFormatting = .sortedKeys
    let phaseID = UUID(uuidString: closedID)!
    let jobID = UUID(uuidString: "33333333-0000-0000-0000-000000000001")!

    let move = String(decoding: try encoder.encode(MoveApplicationRequest(phaseID: phaseID, closedReason: "Rejected")), as: UTF8.self)
    let moveWithoutReason = String(decoding: try encoder.encode(MoveApplicationRequest(phaseID: phaseID)), as: UTF8.self)
    let add = String(decoding: try encoder.encode(AddApplicationRequest(jobID: jobID)), as: UTF8.self)

    #expect(move == #"{"closed_reason":"Rejected","phase_id":"\#(closedID.uppercased())"}"#)
    #expect(moveWithoutReason == #"{"phase_id":"\#(closedID.uppercased())"}"#)
    #expect(add == #"{"job_id":"33333333-0000-0000-0000-000000000001"}"#)
}

@Test func anApplicationAlreadyOnThePipelineDecodesAsNotCreated() throws {
    let body = #"{"application":{"id":"22222222-0000-0000-0000-000000000001","phase_id":"\#(savedID)","phase_entered_at":"2026-09-20T10:00:00Z","created_at":"2026-09-20T10:00:00Z","updated_at":"2026-09-20T10:00:00Z"}}"#

    let response = try HubJSON.makeDecoder().decode(ApplicationResponse.self, from: Data(body.utf8))

    #expect(!response.created)
}

@Test func movingAPhaseSwapsItWithItsNeighbourAndStopsAtTheEnds() throws {
    let phases = try decodeBoard().phases

    #expect(PipelinePhaseOrder.getIDs(of: phases, moving: phases[1].id, by: -1) == [phases[1].id, phases[0].id, phases[2].id])
    #expect(PipelinePhaseOrder.getIDs(of: phases, moving: phases[1].id, by: 1) == [phases[0].id, phases[2].id, phases[1].id])
    #expect(PipelinePhaseOrder.getIDs(of: phases, moving: phases[0].id, by: -1) == nil)
    #expect(PipelinePhaseOrder.getIDs(of: phases, moving: phases[2].id, by: 1) == nil)
}

@Test func theReorderRequestUsesTheServersKey() throws {
    let id = UUID(uuidString: savedID)!
    let body = String(decoding: try HubJSON.makeEncoder().encode(ReorderPipelinePhasesRequest(phaseIDs: [id])), as: UTF8.self)

    #expect(body == #"{"phase_ids":["\#(savedID.uppercased())"]}"#)
}

@Test func aDeleteSucceedsOnAnEmptyAnswerAndCarriesARefusal() async throws {
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/pipeline/phases/empty": .init(status: 204, body: ""),
        "/v1/pipeline/phases/busy": .init(status: 409, body: #"{"error":"the phase still has applications; move them first"}"#),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    try await client.delete("v1/pipeline/phases/empty")
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    await #expect(throws: HubError.server(status: 409, message: "the phase still has applications; move them first")) {
        try await client.delete("v1/pipeline/phases/busy")
    }
}

@Test func aCardsFollowUpReadsByCalendarDay() throws {
    var card = try decodeBoard().cards[0]
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "America/Sao_Paulo")!
    let now = Date(timeIntervalSince1970: 1_790_600_000)

    card.followUpDueAt = now.addingTimeInterval(3 * 86_400)
    #expect(card.getFollowUpStatus(now: now, calendar: calendar) == .dueIn(days: 3))
    card.followUpDueAt = now
    #expect(card.getFollowUpStatus(now: now, calendar: calendar) == .dueToday)
    card.followUpDueAt = now.addingTimeInterval(-2 * 86_400)
    #expect(card.getFollowUpStatus(now: now, calendar: calendar) == .overdue(days: 2))
    #expect(FollowUpStatus.overdue(days: 2).isDue && !FollowUpStatus.dueIn(days: 1).isDue)
    card.followUpDueAt = nil
    #expect(card.getFollowUpStatus(now: now) == nil)
}

@Test func aCardIsDismissedWithANoteAndTheDismissedBoardIsReadApart() async throws {
    let cardID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
    let application = #"{"application":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","phase_id":"0aa55565-58d2-4247-ba01-cba65060a316","#
        + #""phase_entered_at":"2026-09-28T14:00:00Z","created_at":"2026-09-28T14:00:00Z","updated_at":"2026-09-30T14:00:00Z"}}"#
    let dismissedBoard = #"{"phases":[],"cards":[{"application":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","phase_id":"0aa55565-58d2-4247-ba01-cba65060a316","#
        + #""phase_entered_at":"2026-09-28T14:00:00Z","created_at":"2026-09-28T14:00:00Z","updated_at":"2026-09-30T14:00:00Z"},"#
        + #""company_name":"Acme","unseen_updates":0,"dismissed_at":"2026-09-30T19:00:00Z","dismissal_reason":"not a good fit: agency"}]}"#
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/applications/\(cardID.uuidString)/dismiss": StubHub.Answer(status: 200, body: application),
        "/v1/pipeline": StubHub.Answer(status: 200, body: dismissedBoard),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    _ = try await client.dismissApplication(cardID, note: " agency ")
    let sent = try JSONSerialization.jsonObject(with: try #require(recording.lastBody)) as? [String: Any]
    #expect(sent?["note"] as? String == "agency")

    let board = try await client.getDismissedPipeline()
    #expect(recording.lastRequest?.url?.query == "dismissed=true")
    #expect(board.cards.first?.dismissalReason == "not a good fit: agency" && board.cards.first?.dismissedAt != nil)
}

@Test func aCardDecodesWhenSomeoneAtTheCompanyFirstWroteBack() throws {
    let body = #"{"application":{"id":"22222222-0000-0000-0000-000000000001","phase_id":"\#(appliedID)","phase_entered_at":"2026-09-20T10:00:00Z","#
        + #""contacted_at":"2026-09-24T15:30:00Z","applied_at":"2026-09-20T10:00:00Z","created_at":"2026-09-20T10:00:00Z","#
        + #""updated_at":"2026-09-24T15:30:00Z"}}"#

    let response = try HubJSON.makeDecoder().decode(ApplicationResponse.self, from: Data(body.utf8))

    #expect(response.application.contactedAt == Date(timeIntervalSince1970: 1_790_263_800))
    #expect(response.application.appliedAt == Date(timeIntervalSince1970: 1_789_898_400))
    #expect(try decodeBoard().cards.allSatisfy { $0.application.contactedAt == nil })
}

private let tallyNow = Date(timeIntervalSince1970: 1_790_600_000)

private func makeTallyCard(
    in phase: PipelinePhase, contactedAt: Date? = nil, appliedAt: Date? = nil, followUpDueAt: Date? = nil
) -> PipelineCard {
    let application = Application(
        id: UUID(), phaseID: phase.id, phaseEnteredAt: tallyNow.addingTimeInterval(-10 * 86_400), contactedAt: contactedAt,
        appliedAt: appliedAt, createdAt: tallyNow, updatedAt: tallyNow
    )
    return PipelineCard(application: application, followUpDueAt: followUpDueAt, unseenUpdates: 0)
}

private func makeTallyCalendar() -> Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "America/Sao_Paulo")!
    return calendar
}

@Test func theContactTallyCountsSentApplicationsAndTheOnesAPersonAnswered() {
    let saved = PipelinePhase(id: UUID(), name: "Saved", position: 1, isClosed: false)
    let applied = PipelinePhase(id: UUID(), name: "applied", position: 2, isClosed: false, followUpDays: 7)
    let inContact = PipelinePhase(id: UUID(), name: "In contact", position: 3, isClosed: false)
    let closed = PipelinePhase(id: UUID(), name: "Closed", position: 4, isClosed: true)
    let replied = tallyNow.addingTimeInterval(-86_400)
    let sent = tallyNow.addingTimeInterval(-20 * 86_400)
    let board = PipelineBoard(phases: [closed, inContact, applied, saved], cards: [
        makeTallyCard(in: saved),
        makeTallyCard(in: saved, contactedAt: replied),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400)),
        makeTallyCard(in: applied, contactedAt: replied, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
        makeTallyCard(in: inContact),
        makeTallyCard(in: closed, contactedAt: replied),
        makeTallyCard(in: closed),
        makeTallyCard(in: closed, contactedAt: replied, appliedAt: sent),
        makeTallyCard(in: closed, appliedAt: sent, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
    ])

    let tally = board.getContactTally(now: tallyNow, calendar: makeTallyCalendar())

    #expect(tally == ContactTally(sent: 7, heardBack: 4, unansweredPastFollowUp: 1))
    #expect(tally.text == "heard back on 4 of 7 sent · 1 unanswered past follow-up")
}

@Test func aRejectedApplicationStaysInTheTallyAndOneDroppedWhileSavedNeverJoins() throws {
    let saved = PipelinePhase(id: UUID(), name: "Saved", position: 1, isClosed: false)
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 2, isClosed: false)
    let closed = PipelinePhase(id: UUID(), name: "Closed", position: 3, isClosed: true)
    let sentAt = tallyNow.addingTimeInterval(-5 * 86_400)
    let rejected = makeTallyCard(in: applied, appliedAt: sentAt)
    var board = PipelineBoard(phases: [saved, applied, closed], cards: [rejected, makeTallyCard(in: saved)])
    #expect(board.getContactTally(now: tallyNow).text == "heard back on 0 of 1 sent")

    var closedApplication = rejected.application
    closedApplication.phaseID = closed.id
    closedApplication.closedReason = "Rejected by mail: Your application"
    board.replaceApplication(closedApplication)
    #expect(board.getContactTally(now: tallyNow).text == "heard back on 0 of 1 sent")

    var dropped = try #require(board.cards.first { $0.application.phaseID == saved.id }).application
    dropped.phaseID = closed.id
    board.replaceApplication(dropped)
    #expect(board.getContactTally(now: tallyNow).text == "heard back on 0 of 1 sent")
}

@Test func theUnansweredCardsPastFollowUpAreTheOnesTheTallyCounts() {
    let saved = PipelinePhase(id: UUID(), name: "Saved", position: 1, isClosed: false, followUpDays: 7)
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 2, isClosed: false, followUpDays: 7)
    let inContact = PipelinePhase(id: UUID(), name: "In contact", position: 3, isClosed: false, followUpDays: 7)
    let closed = PipelinePhase(id: UUID(), name: "Closed", position: 4, isClosed: true)
    let overdue = tallyNow.addingTimeInterval(-2 * 86_400)
    let unanswered = makeTallyCard(in: applied, followUpDueAt: overdue)
    let board = PipelineBoard(phases: [saved, applied, inContact, closed], cards: [
        unanswered,
        makeTallyCard(in: saved, followUpDueAt: overdue),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400)),
        makeTallyCard(in: applied, contactedAt: tallyNow.addingTimeInterval(-86_400), followUpDueAt: overdue),
        makeTallyCard(in: inContact, followUpDueAt: overdue),
        makeTallyCard(in: closed, followUpDueAt: overdue),
    ])

    let cardIDs = board.getUnansweredPastFollowUp(now: tallyNow, calendar: makeTallyCalendar())

    #expect(cardIDs == [unanswered.id])
    #expect(board.getContactTally(now: tallyNow, calendar: makeTallyCalendar()).unansweredPastFollowUp == cardIDs.count)
}

@Test func withoutAnAppliedPhaseEveryOpenPhaseAfterTheFirstCountsAsSent() {
    let wishlist = PipelinePhase(id: UUID(), name: "Wishlist", position: 1, isClosed: false)
    let sent = PipelinePhase(id: UUID(), name: "Sent", position: 2, isClosed: false)
    let talking = PipelinePhase(id: UUID(), name: "Talking", position: 3, isClosed: false)
    let board = PipelineBoard(phases: [wishlist, sent, talking], cards: [
        makeTallyCard(in: wishlist),
        makeTallyCard(in: sent),
        makeTallyCard(in: talking),
    ])

    let tally = board.getContactTally(now: tallyNow, calendar: makeTallyCalendar())

    #expect(tally == ContactTally(sent: 2, heardBack: 0, unansweredPastFollowUp: 0))
    #expect(tally.text == "heard back on 0 of 2 sent")
}

@Test func theContactTallyHasNoTextUntilAnApplicationGoesOut() throws {
    let board = try decodeBoard()

    #expect(board.getContactTally(now: tallyNow) == ContactTally())
    #expect(board.getContactTally(now: tallyNow).text == nil)
}

@Test func outreachToACompanyIsSentWithTheTrimmedNote() async throws {
    let companyID = UUID(uuidString: "3f2504e0-4f89-41d3-9a0c-0305e82c3301")!
    let answer = #"{"application":{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","phase_id":"0aa55565-58d2-4247-ba01-cba65060a316","#
        + #""phase_entered_at":"2026-10-04T14:00:00Z","created_at":"2026-10-04T14:00:00Z","updated_at":"2026-10-04T14:00:00Z"},"created":true}"#
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/companies/\(companyID.uuidString)/outreach": StubHub.Answer(status: 201, body: answer),
    ])
    let client = HubClient(baseURL: URL(string: "http://localhost:8090")!, token: "t", session: session)

    let response = try await client.recordOutreach(companyID: companyID, note: " LinkedIn message to the engineering lead ")
    #expect(response.created)
    #expect(recording.lastRequest?.httpMethod == "POST")
    let sent = try JSONSerialization.jsonObject(with: try #require(recording.lastBody)) as? [String: Any]
    #expect(sent?["note"] as? String == "LinkedIn message to the engineering lead")
}

@Test func theDueCountTakesFollowUpsDueTodayOrOverdue() {
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 1, isClosed: false, followUpDays: 7)
    let board = PipelineBoard(phases: [applied], cards: [
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
        makeTallyCard(in: applied, followUpDueAt: tallyNow),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400)),
        makeTallyCard(in: applied),
    ])

    #expect(board.getDueCount(now: tallyNow, calendar: makeTallyCalendar()) == 2)
}

@Test func onlyFollowUpsPastTheirDayAreOverdue() {
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 1, isClosed: false, followUpDays: 7)
    let board = PipelineBoard(phases: [applied], cards: [
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
        makeTallyCard(in: applied, followUpDueAt: tallyNow),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400)),
    ])

    #expect(board.getOverdueCount(now: tallyNow, calendar: makeTallyCalendar()) == 1)
}

@Test func aCardsStatusIsItsMostUrgentState() {
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 1, isClosed: false, followUpDays: 7)
    let calendar = makeTallyCalendar()
    let wroteBack = tallyNow.addingTimeInterval(-86_400)
    let overdue = makeTallyCard(in: applied, contactedAt: wroteBack, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400))
    let dueToday = makeTallyCard(in: applied, contactedAt: wroteBack, followUpDueAt: tallyNow)
    let heardBack = makeTallyCard(in: applied, contactedAt: wroteBack, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400))
    let waiting = makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(7 * 86_400))
    let quiet = makeTallyCard(in: applied)

    #expect(overdue.getStatus(now: tallyNow, calendar: calendar) == .followUp(.overdue(days: 2)))
    #expect(dueToday.getStatus(now: tallyNow, calendar: calendar) == .followUp(.dueToday))
    #expect(heardBack.getStatus(now: tallyNow, calendar: calendar) == .heardBack(wroteBack))
    #expect(waiting.getStatus(now: tallyNow, calendar: calendar) == .followUp(.dueIn(days: 7)))
    #expect(quiet.getStatus(now: tallyNow, calendar: calendar) == nil)
    #expect(PipelineCardStatus.followUp(.overdue(days: 2)).isOverdue && PipelineCardStatus.followUp(.overdue(days: 2)).isDue)
    #expect(PipelineCardStatus.followUp(.dueToday).isDue && !PipelineCardStatus.followUp(.dueToday).isOverdue)
    #expect(!PipelineCardStatus.heardBack(wroteBack).isDue && !PipelineCardStatus.followUp(.dueIn(days: 7)).isDue)
}

@Test func aFollowUpReadsInTheCardsWords() {
    #expect(FollowUpStatus.overdue(days: 2).text == "Follow-up overdue 2 days")
    #expect(FollowUpStatus.overdue(days: 1).text == "Follow-up overdue 1 day")
    #expect(FollowUpStatus.dueToday.text == "Follow-up due today")
    #expect(FollowUpStatus.dueIn(days: 7).text == "Follow up in 7 days")
    #expect(FollowUpStatus.dueIn(days: 1).text == "Follow up tomorrow")
}

@Test func aPhasesDueTallyCountsTodayAndOverdueAndTurnsRedOnOverdue() {
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 1, isClosed: false, followUpDays: 7)
    let interviewing = PipelinePhase(id: UUID(), name: "Interviewing", position: 2, isClosed: false, followUpDays: 7)
    let offer = PipelinePhase(id: UUID(), name: "Offer", position: 3, isClosed: false)
    let board = PipelineBoard(phases: [applied, interviewing, offer], cards: [
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(-2 * 86_400)),
        makeTallyCard(in: applied, followUpDueAt: tallyNow),
        makeTallyCard(in: applied, followUpDueAt: tallyNow.addingTimeInterval(3 * 86_400)),
        makeTallyCard(in: interviewing, followUpDueAt: tallyNow),
    ])
    let calendar = makeTallyCalendar()

    let appliedTally = board.getDueTally(in: applied, now: tallyNow, calendar: calendar)
    let interviewingTally = board.getDueTally(in: interviewing, now: tallyNow, calendar: calendar)
    let offerTally = board.getDueTally(in: offer, now: tallyNow, calendar: calendar)

    #expect(appliedTally == DueTally(due: 2, overdue: 1) && appliedTally.text == "2 due" && appliedTally.tone == .negative)
    #expect(interviewingTally == DueTally(due: 1, overdue: 0) && interviewingTally.tone == .caution)
    #expect(offerTally.text == nil)
}

@Test func closedCardsLeaveTheOpenPhasesForTheDrawer() {
    let applied = PipelinePhase(id: UUID(), name: "Applied", position: 1, isClosed: false)
    let closed = PipelinePhase(id: UUID(), name: "Closed", position: 2, isClosed: true)
    let open = makeTallyCard(in: applied)
    let ended = makeTallyCard(in: closed)
    let board = PipelineBoard(phases: [closed, applied], cards: [open, ended])

    #expect(board.openPhases == [applied])
    #expect(board.closedPhase == closed)
    #expect(board.openCards.map(\.id) == [open.id])
    #expect(board.closedCards.map(\.id) == [ended.id])
    #expect(PipelineBoard(phases: [applied], cards: [open]).closedPhase == nil)
}

@Test func anEmptyPhaseSaysWhatGoesThere() {
    let offer = PipelinePhase(id: UUID(), name: "Offer", position: 5, isClosed: false)
    let custom = PipelinePhase(id: UUID(), name: "Take-home", position: 6, isClosed: false)

    #expect(offer.emptyHint == "Drop a card here when an offer comes in")
    #expect(custom.emptyHint == "Drop a card here when it reaches Take-home")
}

@Test func aCardsTooltipHoldsItsNotesAndWhyItEndedOrWasSkipped() throws {
    var card = try decodeBoard().cards[0]
    #expect(card.tooltip == "Ask about the team.")

    card.application.closedReason = "Role filled internally"
    card.dismissalReason = "agency"
    #expect(card.tooltip == "Ask about the team.\nClosed: Role filled internally\nSkipped: agency")

    card.application.notes = " "
    card.application.closedReason = nil
    card.dismissalReason = ""
    #expect(card.tooltip.isEmpty)
}
