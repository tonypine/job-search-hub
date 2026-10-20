import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// The sidebar's Pipeline badge after a change made in the app, against a
// stub of the hub: the hub raises no update for a follow-up, a move, a skip
// or a restore, so the badge reads again on the application changes'
// revision. Made-up companies only.

private let pipelinePath = "/v1/pipeline"
private let appliedID = UUID(uuidString: "11111111-0000-4000-8000-000000000001")!
private let closedID = UUID(uuidString: "11111111-0000-4000-8000-000000000002")!
private let firstCardID = UUID(uuidString: "22222222-0000-4000-8000-000000000001")!
private let secondCardID = UUID(uuidString: "22222222-0000-4000-8000-000000000002")!

private func application(_ id: UUID, phaseID: UUID = appliedID) -> String {
    #"""
    {"id":"\#(id.uuidString)","company_id":"44444444-0000-4000-8000-000000000001","phase_id":"\#(phaseID.uuidString)",
     "phase_entered_at":"2026-09-20T10:00:00Z","created_at":"2026-09-20T10:00:00Z","updated_at":"2026-09-20T10:00:00Z"}
    """#
}

/// A card in Applied whose follow-up falls due the given days from now:
/// below zero is overdue.
private func card(_ id: UUID, company: String, dueInDays days: Int) -> String {
    let dueAt = Date.now.addingTimeInterval(TimeInterval(days) * 86_400).ISO8601Format()
    return #"{"application":\#(application(id)),"company_name":"\#(company)","follow_up_due_at":"\#(dueAt)","unseen_updates":0}"#
}

private func pipeline(_ cards: [String]) -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"""
    {"phases":[{"id":"\#(appliedID.uuidString)","name":"Applied","position":1,"is_closed":false},
               {"id":"\#(closedID.uuidString)","name":"Closed","position":2,"is_closed":true}],
     "cards":[\#(cards.joined(separator: ","))]}
    """#)
}

private func applicationAnswer(_ id: UUID, phaseID: UUID = appliedID) -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"{"application":\#(application(id, phaseID: phaseID)),"created":false}"#)
}

/// A hub with two cards overdue, nothing to decide, and the answers a
/// follow-up, a move, a skip and a restore get.
@MainActor
private func makeHub() -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        pipelinePath: pipeline([card(firstCardID, company: "Brightwater", dueInDays: -3), card(secondCardID, company: "Northwind", dueInDays: -2)]),
        "\(pipelinePath)?dismissed=true": pipeline([]),
        "/v1/decision-queue": StubHub.Answer(status: 200, body: #"{"items":[],"total":0}"#),
        "/v1/people": StubHub.Answer(status: 200, body: #"{"people":[]}"#),
        "/v1/applications/\(firstCardID.uuidString)/follow-ups": applicationAnswer(firstCardID),
        "/v1/applications/\(secondCardID.uuidString)/follow-ups": applicationAnswer(secondCardID),
        "/v1/applications/\(secondCardID.uuidString)": applicationAnswer(secondCardID, phaseID: closedID),
        "/v1/applications/\(firstCardID.uuidString)/dismiss": applicationAnswer(firstCardID),
        "/v1/applications/\(firstCardID.uuidString)/restore": applicationAnswer(firstCardID),
    ])
    let client = HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session)
    return (client, recording)
}

@MainActor @Test func aFollowUpOnThePipelineClearsTheBadge() async throws {
    let (client, recording) = makeHub()
    let counts = SidebarCounts()
    let changes = ApplicationChanges()
    let model = PipelineModel()
    await counts.refresh(with: client)
    await model.load(with: client)
    #expect(counts.getCount(for: .pipeline, unseen: 0) == 2)
    #expect(counts.isUrgent(.pipeline))

    // ✓ › Record on Brightwater: the hub's next follow-up is a week out.
    recording.setAnswer(pipeline([card(firstCardID, company: "Brightwater", dueInDays: 7), card(secondCardID, company: "Northwind", dueInDays: -2)]), for: pipelinePath)
    await model.recordFollowUp(firstCardID, note: "Wrote to the hiring manager", through: changes, with: client)
    #expect(model.actionError == nil)
    #expect(changes.revision == 1)
    await counts.refresh(with: client)
    #expect(counts.getCount(for: .pipeline, unseen: 0) == 1)
    #expect(counts.isUrgent(.pipeline))

    // The last overdue card followed up: no count, and no red.
    recording.setAnswer(pipeline([card(firstCardID, company: "Brightwater", dueInDays: 7), card(secondCardID, company: "Northwind", dueInDays: 7)]), for: pipelinePath)
    await model.recordFollowUp(secondCardID, note: "", through: changes, with: client)
    #expect(changes.revision == 2)
    await counts.refresh(with: client)
    #expect(counts.getCount(for: .pipeline, unseen: 0) == 0)
    #expect(!counts.isUrgent(.pipeline))
}

@MainActor @Test func aFollowUpFromTodayBumpsTheRevision() async throws {
    let (client, _) = makeHub()
    let changes = ApplicationChanges()
    let model = TodayModel()
    let board = PipelineBoard(try await client.getPipeline())
    let brightwater = try #require(board.cards.first { $0.id == firstCardID })

    await model.recordFollowUp(brightwater, note: "", through: changes, with: client)

    #expect(model.actionError == nil)
    #expect(changes.revision == 1)
}

@MainActor @Test func aMoveASkipAndARestoreEachBumpTheRevision() async throws {
    let (client, recording) = makeHub()
    let changes = ApplicationChanges()
    let model = PipelineModel()
    await model.load(with: client)
    let closed = try #require(model.board.phases.first { $0.id == closedID })

    await model.move(secondCardID, to: closed, closedReason: "Filled", through: changes, with: client)
    #expect(model.actionError == nil)
    #expect(recording.lastRequest?.httpMethod == "PATCH")
    #expect(changes.revision == 1)

    await model.skip(firstCardID, note: "", through: changes, with: client)
    #expect(model.actionError == nil)
    #expect(changes.revision == 2)

    await model.restore(firstCardID, through: changes, with: client)
    #expect(model.actionError == nil)
    #expect(changes.revision == 3)
}

@MainActor @Test func aFailedFollowUpLeavesTheBadgeAsItWas() async throws {
    let (client, recording) = makeHub()
    let changes = ApplicationChanges()
    let model = PipelineModel()
    recording.setAnswer(StubHub.Answer(status: 500, body: #"{"error":"down"}"#), for: "/v1/applications/\(firstCardID.uuidString)/follow-ups")

    await model.recordFollowUp(firstCardID, note: "", through: changes, with: client)

    #expect(model.actionError != nil)
    #expect(changes.revision == 0)
}
