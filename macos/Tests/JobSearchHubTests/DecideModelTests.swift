import Foundation
import HubTestSupport
@testable import JobSearchHub
import JobSearchHubCore
import Testing

// Decide, against a stub of the hub: the queue beside the job's page, and
// P, L and S deciding at once with Undo. Made-up jobs only.

private let queuePath = "/v1/decision-queue"
private let signalsPath = "/v1/decision-signals"

private let firstID = UUID(uuidString: "AAAAAAAA-0000-4000-8000-000000000001")!
private let secondID = UUID(uuidString: "AAAAAAAA-0000-4000-8000-000000000002")!
private let thirdID = UUID(uuidString: "AAAAAAAA-0000-4000-8000-000000000003")!

private let signalsJSON = #"{"since":"2026-10-01T00:00:00Z","decisions":{"pursue":3,"skip":8,"later":1},"good_or_unclear_seen":0,"good_or_unclear_decided":0}"#

private func queueItem(_ id: UUID, title: String, match: String = "strong") -> String {
    #"""
    {"job":{"id":"\#(id.uuidString)","source":"manual","title":"\#(title)","url":"https://acme.com/\#(id.uuidString)",
            "first_seen_at":"2026-10-01T00:00:00Z","last_seen_at":"2026-10-01T00:00:00Z"},
     "company_name":"Acme","match":"\#(match)","reason":"Fits the brief.","brief_tier":"pre",
     "fit":{"level":"good","checks":[]},"unseen_updates":0}
    """#
}

private let threeJobs = [
    queueItem(firstID, title: "Senior Product Engineer"),
    queueItem(secondID, title: "Platform Engineer"),
    queueItem(thirdID, title: "Design Engineer"),
]

private func queue(_ items: [String]) -> StubHub.Answer {
    StubHub.Answer(status: 200, body: #"{"items":[\#(items.joined(separator: ","))],"total":\#(items.count)}"#)
}

/// A hub with the three jobs queued, the day's signals, and the answers a
/// decision and its Undo get.
@MainActor
private func makeHub(items: [String] = threeJobs) -> (HubClient, StubHub.Recording) {
    let (session, recording) = StubHub.makeSession(answers: [
        queuePath: queue(items),
        signalsPath: StubHub.Answer(status: 200, body: signalsJSON),
        "/v1/jobs/dismiss": StubHub.Answer(status: 200, body: #"{"jobs":[]}"#),
    ])
    // A decision records and an Undo clears: both answer with the job's
    // standing, which Decide only reads for the note an Undo may show.
    recording.setAnswer(StubHub.Answer(status: 200, body: #"{"decision":"pursue","decided_at":"2026-10-17T12:00:00Z"}"#), for: "/v1/jobs/\(firstID.uuidString)/decision")
    recording.setAnswer(StubHub.Answer(status: 200, body: #"{"decision":"skip","decided_at":"2026-10-17T12:00:00Z"}"#), for: "/v1/jobs/\(secondID.uuidString)/decision")
    let client = HubClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: "test", session: session)
    return (client, recording)
}

@MainActor @Test func decidingBringUpTheNextJobWithUndo() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    let decisions = JobDecisions()

    await model.load(with: client)
    #expect(model.selectedID == firstID)
    #expect(model.subtitle == "3 to decide · 12 decided this week")

    await model.decide(model.items[0], .pursue, through: decisions, with: client)

    // The next job is open, and the note says what happened and offers Undo.
    #expect(model.selectedID == secondID)
    #expect(model.toast?.text == "Pursued Senior Product Engineer")
    #expect(model.toast?.undo != nil)
    #expect(recording.lastRequest?.httpMethod == "POST")
    #expect(recording.lastRequest?.url?.path() == "/v1/jobs/\(firstID.uuidString)/decision")
}

@MainActor @Test func undoTakesTheDecisionBackAndOpensTheJobAgain() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    let decisions = JobDecisions()
    await model.load(with: client)
    await model.decide(model.items[0], .skip, through: decisions, with: client)
    #expect(model.selectedID == secondID)

    // A read that started before the Undo lands without the job; it must
    // not take the open job away.
    recording.setAnswer(queue([queueItem(secondID, title: "Platform Engineer")]), for: queuePath)
    await model.undo(model.items[0], through: decisions, with: client)
    #expect(recording.lastRequest?.httpMethod == "DELETE")
    #expect(recording.lastRequest?.url?.path() == "/v1/jobs/\(firstID.uuidString)/decision")
    #expect(model.selectedID == firstID)

    await model.load(with: client)
    #expect(model.selectedID == firstID)

    // Once the queue reads with the job back, it stays open.
    recording.setAnswer(queue(threeJobs), for: queuePath)
    await model.load(with: client)
    #expect(model.items.count == 3)
    #expect(model.selectedID == firstID)
}

@MainActor @Test func aSkipsNoteOffersAReason() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    let decisions = JobDecisions()
    await model.load(with: client)

    await model.decide(model.items[0], .skip, through: decisions, with: client)

    #expect(model.selectedID == secondID)
    #expect(model.toast?.text == "Skipped Senior Product Engineer")
    #expect(model.toast?.action?.title == "Add reason")

    model.toast?.action?.perform()
    #expect(model.reasonTarget?.jobIDs == [firstID])

    let failure = await model.addSkipReason("agency", to: [firstID], through: decisions, with: client)
    #expect(failure == nil)
    #expect(recording.lastRequest?.url?.path() == "/v1/jobs/dismiss")
}

@MainActor @Test func arrowsMoveWithoutDeciding() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    await model.load(with: client)
    let requests = recording.requestCount

    model.move(by: 1)
    #expect(model.selectedID == secondID)
    model.move(by: 1)
    #expect(model.selectedID == thirdID)
    // The last job's step down stays put, and nothing was decided.
    model.move(by: 1)
    #expect(model.selectedID == thirdID)
    model.move(by: -1)
    #expect(model.selectedID == secondID)
    #expect(recording.requestCount == requests)
}

@MainActor @Test func laterDoesNothingToAJobAlreadyLeftForLater() async throws {
    let later = #"""
    {"job":{"id":"\#(firstID.uuidString)","source":"manual","title":"Senior Product Engineer","url":"https://acme.com/1",
            "first_seen_at":"2026-10-01T00:00:00Z","last_seen_at":"2026-10-01T00:00:00Z"},
     "company_name":"Acme","match":"strong","reason":"Fits the brief.","brief_tier":"pre",
     "fit":{"level":"good","checks":[]},"decision":{"decision":"later","decided_at":"2026-10-16T12:00:00Z"}}
    """#
    let (client, recording) = makeHub(items: [later])
    let model = DecideModel()
    let decisions = JobDecisions()
    await model.load(with: client)
    let requests = recording.requestCount

    await model.decide(model.items[0], .later, through: decisions, with: client)

    #expect(model.items.count == 1)
    #expect(recording.requestCount == requests)
    #expect(model.toast == nil)
}

@MainActor @Test func aDecisionThatFailsKeepsTheJobOpen() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    let decisions = JobDecisions()
    await model.load(with: client)

    recording.setAnswer(StubHub.Answer(status: 500, body: #"{"error":"boom"}"#), for: "/v1/jobs/\(firstID.uuidString)/decision")
    await model.decide(model.items[0], .pursue, through: decisions, with: client)

    #expect(model.actionError?.title == "Couldn't record the decision")
    #expect(model.selectedID == firstID)
    #expect(model.toast == nil)
}

@MainActor @Test func anUndoThatFailsLeavesTheNextJobOpen() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    let decisions = JobDecisions()
    await model.load(with: client)
    let item = model.items[0]
    await model.decide(item, .pursue, through: decisions, with: client)
    #expect(model.selectedID == secondID)

    recording.setAnswer(StubHub.Answer(status: 500, body: #"{"error":"boom"}"#), for: "/v1/jobs/\(firstID.uuidString)/decision")
    await model.undo(item, through: decisions, with: client)

    #expect(model.actionError?.title == "Couldn't undo the decision")
    #expect(model.selectedID == secondID)
}

@MainActor @Test func aReadThatFailsKeepsTheQueueUp() async throws {
    let (client, recording) = makeHub()
    let model = DecideModel()
    await model.load(with: client)
    model.move(by: 1)

    recording.setAnswer(StubHub.Answer(status: 500, body: #"{"error":"boom"}"#), for: queuePath)
    await model.load(with: client)

    #expect(model.loadError?.title == "Couldn't load the queue")
    #expect(model.items.count == 3)
    #expect(model.selectedID == secondID)

    recording.setAnswer(queue(threeJobs), for: queuePath)
    await model.load(with: client)
    #expect(model.loadError == nil)
}
