import Foundation
import HubTestSupport
@testable import JobSearchHubCore
import Testing

private let hubURL = URL(string: "http://localhost:8090")!
private let jobID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
private let providerID = UUID(uuidString: "5f1b1c7e-3e0a-4a51-9f0e-6c1d2b7a8e90")!
private let lastFactsRunID = UUID(uuidString: "0aa55565-58d2-4247-ba01-cba65060a316")!
private let runningBriefID = UUID(uuidString: "0aa55565-58d2-4247-ba01-cba65060a317")!

/// A job's runnable kinds as the hub answers them, in its catalog's order.
private let kindsJSON = #"""
{"kinds":[
 {"id":"job_facts","title":"Job facts","description":"Reads a job's facts.","group":"jobs","subject_type":"job","engine":"model_call","automatic":true,
  "paused":false,"route":{"provider_id":"5f1b1c7e-3e0a-4a51-9f0e-6c1d2b7a8e90","provider":"This Mac","model":"qwen3-8b.gguf"},
  "last_run":{"id":"0aa55565-58d2-4247-ba01-cba65060a316","record":"model_call","kind":"job_facts","subject":{"type":"job","id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","title":"Staff engineer"},
   "model":"qwen3-8b.gguf","provider":"This Mac","started_at":"2026-10-29T13:12:05Z","duration_ms":4200,"outcome":"invalid","prompt_tokens":900,"completion_tokens":80,
   "cost_usd_estimate":"0.0012","started_by_owner":false},
  "can_run":true,"needs_model":false,"needs_note":false,"running_starts":[]},
 {"id":"job_brief_full","title":"Full briefs","description":"Claude writes a full brief.","group":"jobs","subject_type":"job","engine":"claude_one_shot","automatic":true,
  "paused":true,"route":{"provider_id":"5f1b1c7e-3e0a-4a51-9f0e-6c1d2b7a8e91","provider":"Claude","model":"claude-sonnet"},"can_run":true,"needs_model":false,"needs_note":false,
  "running_starts":["0aa55565-58d2-4247-ba01-cba65060a317"]},
 {"id":"recruiter_screen","title":"Recruiter screens","description":"Reads the CV as a recruiter.","group":"jobs","subject_type":"job","engine":"model_call","automatic":true,
  "paused":false,"can_run":false,"reason":"needs the tailored CV first","needs_model":false,"needs_note":false,"running_starts":[]},
 {"id":"job_fix","title":"Job fixes","description":"An agent fixes a job's details.","group":"jobs","subject_type":"job","engine":"claude_agent","automatic":false,
  "paused":false,"can_run":true,"needs_model":false,"needs_note":true,"running_starts":[]}
]}
"""#

private func decodeKinds() throws -> [RunnableKind] {
    try HubJSON.makeDecoder().decode(RunnableKindsResponse.self, from: Data(kindsJSON.utf8)).kinds
}

private func getKind(_ id: String) throws -> RunnableKind {
    try #require(try decodeKinds().first { $0.id == id })
}

private var utc: Calendar {
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC")!
    return calendar
}

private let afternoon = Date(timeIntervalSince1970: 1_793_284_800) // 2026-10-29T14:40:00Z
private let english = Locale(identifier: "en_US")

@Test func aJobsRunnableKindsDecodeWithTheirModelLastRunAndReason() throws {
    let kinds = try decodeKinds()

    #expect(kinds.map(\.id) == ["job_facts", "job_brief_full", "recruiter_screen", "job_fix"])
    let facts = kinds[0]
    #expect(facts.route == RunnableKindRoute(providerID: providerID, provider: "This Mac", model: "qwen3-8b.gguf"))
    #expect(facts.lastRun?.id == lastFactsRunID && facts.lastRun?.outcome == "invalid")
    #expect(facts.canRun && !facts.paused && !facts.needsNote)
    #expect(kinds[1].paused && kinds[1].lastRun == nil && kinds[1].runningStarts == [runningBriefID])
    #expect(facts.runningStarts.isEmpty)
    #expect(!kinds[2].canRun && kinds[2].reason == "needs the tailored CV first" && kinds[2].route == nil)
    #expect(kinds[3].needsNote && kinds[3].isAgent)
}

@Test func theFixOfTheJobsDetailsSitsLastUnderTheRuns() throws {
    let rows = JobRuns.split(try decodeKinds())

    #expect(rows.runs.map(\.id) == ["job_facts", "job_brief_full", "recruiter_screen"])
    #expect(rows.withNote.map(\.id) == ["job_fix"])
}

@Test func eachRowIsNamedForOneRun() throws {
    let titles = try decodeKinds().map(JobRuns.getTitle(of:))

    #expect(titles == ["Job facts", "Full brief", "Recruiter screen", "Fix the job's details"])
}

@Test func aRowSaysItsLastRunOrNever() throws {
    let facts = try getKind("job_facts")
    #expect(JobRuns.describeState(of: facts, now: afternoon, calendar: utc, locale: english) == "Read 13:12 · invalid answer")

    var written = try #require(facts.lastRun)
    written.kind = "job_brief_full"
    written.outcome = "succeeded"
    written.startedAt = Date(timeIntervalSince1970: 1_793_283_240) // 14:14
    #expect(JobRuns.describeLastRun(written, now: afternoon, calendar: utc, locale: english) == "Written 14:14")

    written.startedAt = afternoon.addingTimeInterval(-2 * 86400)
    #expect(JobRuns.describeLastRun(written, now: afternoon, calendar: utc, locale: english) == "Written Oct 27")

    written.outcome = "failed"
    written.startedAt = afternoon
    #expect(JobRuns.describeLastRun(written, now: afternoon, calendar: utc, locale: english) == "Written 14:40 · failed")

    #expect(JobRuns.describeLastRun(nil, now: afternoon) == "Never")
}

@Test func aKindThatCantRunYetSaysTheHubsReason() throws {
    let screen = try getKind("recruiter_screen")

    #expect(JobRuns.describeState(of: screen, now: afternoon) == "Needs the tailored CV first")
    #expect(JobRuns.describeStart(of: screen) == "Needs the tailored CV first")
}

@Test func aPausedKindSaysSoAndItsStartGoesAheadAnyway() throws {
    let brief = try getKind("job_brief_full")

    #expect(JobRuns.describeState(of: brief, now: afternoon) == "Never · kind paused")
    #expect(JobRuns.describeStart(of: brief) == "Start full brief now. Full briefs are paused, but a run you start goes ahead anyway.")
    #expect(JobRuns.describeStart(of: try getKind("job_facts")) == "Start job facts now")
}

/// A job's tailored CV with nothing routed for it, as the hub lists it: it
/// runs only on a model picked for the run.
private let unroutedCVJSON = #"""
{"id":"job_cv","title":"Tailored CVs","description":"Drafts a CV for the job.","group":"jobs","subject_type":"job","engine":"model_call","automatic":true,
 "paused":false,"can_run":false,"reason":"no model is routed for it yet","needs_model":true,"needs_note":false,"running_starts":[]}
"""#

private let flash = RoutedModel(providerID: providerID, model: "google/gemini-2.5-flash")

@Test func aKindWithNoModelRoutedStartsOnceAModelIsPickedForTheRun() throws {
    let cv = try HubJSON.makeDecoder().decode(RunnableKind.self, from: Data(unroutedCVJSON.utf8))
    #expect(cv.needsModel && !cv.canRun && cv.route == nil)

    // Its select starts on no model, so ▶ waits for a pick.
    var selection = RunModelSelection(kind: cv)
    #expect(selection.chosen == nil && !JobRuns.canStart(cv, on: selection.chosen))
    #expect(JobRuns.describeStart(of: cv, on: selection.chosen) == "No model is routed for it yet. Pick a model for this run to start it on that one")

    selection.pick(flash)
    #expect(selection.chosen == flash && JobRuns.canStart(cv, on: selection.chosen))
    #expect(JobRuns.describeStart(of: cv, on: selection.chosen) == "Start tailored CV now on the model picked")
}

@Test func aKindThatLacksMoreThanAModelWaitsWhateverIsPicked() throws {
    let screen = try getKind("recruiter_screen")
    #expect(!screen.needsModel)

    #expect(!JobRuns.canStart(screen, on: flash))
    #expect(JobRuns.describeStart(of: screen, on: flash) == "Needs the tailored CV first")
}

@Test func aRoutedKindStartsOnItsOwnModelOrOnePicked() throws {
    let facts = try getKind("job_facts")

    #expect(JobRuns.canStart(facts, on: nil) && JobRuns.canStart(facts, on: flash))
    #expect(JobRuns.describeStart(of: facts, on: nil) == "Start job facts now")
    #expect(JobRuns.describeStart(of: facts, on: flash) == "Start job facts now on the model picked")
}

@Test func aRunningRowCountsUp() {
    let start = afternoon
    #expect(JobRuns.describeRunning(since: start, now: start) == "Running 0:00")
    #expect(JobRuns.describeRunning(since: start, now: start.addingTimeInterval(42)) == "Running 0:42")
    #expect(JobRuns.describeRunning(since: start, now: start.addingTimeInterval(12 * 60 + 5)) == "Running 12:05")
    #expect(JobRuns.describeRunning(since: start, now: start.addingTimeInterval(3723)) == "Running 1:02:03")
    #expect(JobRuns.describeRunning(since: start, now: start.addingTimeInterval(-5)) == "Running 0:00")
}

@Test func aRowsModelCarriesItsProvidersMark() throws {
    let providers = [ModelProvider(id: providerID, kind: ModelProvider.hubRuntimeKind, name: "This Mac", baseURL: "", hasKey: false, enforcesSchema: true)]

    let facts = try #require(RunModelSelection(kind: try getKind("job_facts")).getLabel(providers: providers, catalogs: [:]))
    #expect(facts.name == "qwen3-8b" && facts.markSymbol == "cpu" && facts.runsOnText == "Runs on qwen3-8b from This Mac")
    let fix = try #require(RunModelSelection(kind: try getKind("job_fix")).getLabel(providers: providers, catalogs: [:]))
    #expect(fix.name == "Claude" && fix.markSymbol == "sparkle")
    #expect(RunModelSelection(kind: try getKind("recruiter_screen")).getLabel(providers: providers, catalogs: [:]) == nil)
}

@Test func theRowShowsNoPriceOrTokens() throws {
    let facts = try getKind("job_facts")
    let shown = [JobRuns.getTitle(of: facts), JobRuns.describeState(of: facts, now: afternoon, calendar: utc, locale: english), JobRuns.describeStart(of: facts)]

    for text in shown {
        #expect(!text.contains("$") && !text.contains("token") && !text.contains("900"))
    }
}

@Test func aStartedModelRunLandsOnceTheHubNoLongerRunsIt() throws {
    var facts = try getKind("job_facts")
    let started = StartedRun(id: UUID(), kind: "job_facts", subjectID: jobID, startedByOwner: true)
    facts.runningStarts = [UUID(), started.id]
    #expect(!JobRuns.hasLanded(started, of: facts))

    // Its first answer's record, under its id, says invalid while the hub
    // asks again; another start of the kind ends meanwhile.
    facts.lastRun?.id = started.id
    facts.lastRun?.outcome = "invalid"
    facts.runningStarts = [started.id]
    #expect(!JobRuns.hasLanded(started, of: facts))

    // The retry is recorded under its own id, and the run ends.
    facts.lastRun?.id = UUID()
    facts.lastRun?.outcome = "succeeded"
    facts.runningStarts = []
    #expect(JobRuns.hasLanded(started, of: facts))

    // One that gave up before asking a model has no record.
    facts.lastRun = nil
    #expect(JobRuns.hasLanded(started, of: facts))
}

@Test func anAgentsStartedRunLandsOnceItsRecordFinishes() throws {
    var fix = try getKind("job_fix")
    let started = StartedRun(id: UUID(), kind: JobRuns.fixKind, subjectID: jobID, startedByOwner: true, taskID: UUID())
    #expect(!JobRuns.hasLanded(started, of: fix))

    // An earlier run of the kind isn't it.
    fix.lastRun = try getKind("job_facts").lastRun
    fix.lastRun?.outcome = "succeeded"
    #expect(!JobRuns.hasLanded(started, of: fix))

    fix.lastRun?.id = started.id
    fix.lastRun?.outcome = "running"
    #expect(!JobRuns.hasLanded(started, of: fix))

    fix.lastRun?.outcome = "failed"
    #expect(JobRuns.hasLanded(started, of: fix))
}

@Test func anAgentIsFollowedLongerThanAModelRun() throws {
    #expect(JobRuns.getFollowLimit(of: try getKind("job_fix")) > JobRuns.getFollowLimit(of: try getKind("job_facts")))
}

@Test func aStartAsksForTheKindForTheJob() throws {
    let start = StartRunRequest(kind: "job_brief_full", subjectType: JobRuns.jobSubject, subjectID: jobID)
    let encoded = try #require(String(data: HubJSON.makeEncoder().encode(start), encoding: .utf8))

    #expect(encoded.contains(#""kind":"job_brief_full""#) && encoded.contains(#""subject_type":"job""#))
    #expect(encoded.contains(#""subject_id":"\#(jobID.uuidString)""#) && !encoded.contains("note"))

    let fix = StartRunRequest(kind: JobRuns.fixKind, subjectType: JobRuns.jobSubject, subjectID: jobID, note: "the title has the city in it")
    let fixEncoded = try #require(String(data: HubJSON.makeEncoder().encode(fix), encoding: .utf8))
    #expect(fixEncoded.contains(#""note":"the title has the city in it""#))
    // On the kind's model, the start names none.
    #expect(!encoded.contains("provider_id") && !encoded.contains("model") && !fixEncoded.contains("model"))
}

@Test func theSectionsStartTheKindsTheHubRunsThemAs() {
    // A job's facts, full brief and tailored CV, as the hub names their kinds.
    #expect(JobRuns.factsKind == "job_facts")
    #expect(JobRuns.fullBriefKind == "job_brief_full")
    #expect(JobRuns.cvKind == "job_cv")
}

@Test func aStartOnAModelChosenForTheRunNamesItsProviderAndModel() throws {
    let flash = RoutedModel(providerID: providerID, model: "google/gemini-2.5-flash")
    let start = StartRunRequest(kind: JobRuns.cvKind, subjectType: JobRuns.jobSubject, subjectID: jobID, model: flash)
    let sent = try #require(try JSONSerialization.jsonObject(with: HubJSON.makeEncoder().encode(start)) as? [String: Any])

    #expect(sent["kind"] as? String == "job_cv" && sent["subject_id"] as? String == jobID.uuidString)
    #expect(sent["provider_id"] as? String == providerID.uuidString && sent["model"] as? String == "google/gemini-2.5-flash")
}

@Test func theClientListsAJobsKindsAndStartsOne() async throws {
    let runID = UUID()
    let (session, recording) = StubHub.makeSession(answers: [
        "/v1/jobs/\(jobID.uuidString)/runnable-kinds": .init(status: 200, body: kindsJSON),
        "/v1/runs": .init(status: 202, body: #"""
        {"queued":true,"run":{"id":"\#(runID.uuidString)","kind":"job_brief_full","subject_type":"job","subject_id":"\#(jobID.uuidString)",
         "provider":"Claude","model":"claude-sonnet","chosen_model":false,"started_by_owner":true}}
        """#),
    ])
    let client = HubClient(baseURL: hubURL, token: "owner-token", session: session)

    let kinds = try await client.listRunnableKinds(forJob: jobID)
    #expect(kinds.count == 4)
    #expect(recording.lastRequest?.httpMethod == "GET")

    let started = try await client.startRun(StartRunRequest(kind: "job_brief_full", subjectType: JobRuns.jobSubject, subjectID: jobID))
    #expect(started.id == runID && started.subjectID == jobID && started.startedByOwner && started.taskID == nil)
    #expect(recording.lastRequest?.httpMethod == "POST" && recording.lastRequest?.url?.path() == "/v1/runs")
    let body = try #require(recording.lastBody.flatMap { String(data: $0, encoding: .utf8) })
    #expect(body.contains(#""kind":"job_brief_full""#))
}

@Test func anAgentsStartNamesTheMacsTaskItWaitsIn() throws {
    let runID = UUID()
    let taskID = UUID()
    let body = #"""
    {"queued":true,"run":{"id":"\#(runID.uuidString)","kind":"job_fix","subject_type":"job","subject_id":"\#(jobID.uuidString)",
     "provider":"","model":"","chosen_model":false,"started_by_owner":true,"task_id":"\#(taskID.uuidString)"}}
    """#

    let started = try HubJSON.makeDecoder().decode(StartedRunResponse.self, from: Data(body.utf8)).run

    #expect(started.id == runID && started.kind == JobRuns.fixKind && started.taskID == taskID)
}
