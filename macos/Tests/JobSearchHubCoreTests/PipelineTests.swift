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
