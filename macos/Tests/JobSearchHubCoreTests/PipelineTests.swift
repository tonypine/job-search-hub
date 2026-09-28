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
           "job_title":"Backend Engineer","job_url":"https://acme.com/careers/backend","company_name":"Acme"},
          {"application":{"id":"22222222-0000-0000-0000-000000000002","company_id":"44444444-0000-0000-0000-000000000002",
                          "phase_id":"\(savedID)","notes":"Ask about the team.",
                          "phase_entered_at":"2026-09-27T10:00:00.5Z","created_at":"2026-09-27T10:00:00Z","updated_at":"2026-09-27T10:00:00Z"},
           "company_name":"Globex"}]}
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
