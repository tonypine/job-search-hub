import Foundation
@testable import JobSearchHubCore
import Testing

private let jobID = UUID(uuidString: "7c9e6679-7425-40de-944b-e07fc1f90ae7")!
private let companyID = UUID(uuidString: "0aa55565-58d2-4247-ba01-cba65060a316")!
private let providerID = UUID(uuidString: "5f1b1c7e-3e0a-4a51-9f0e-6c1d2b7a8e90")!

private let kindsJSON = #"""
{"groups":[{"id":"jobs","title":"Jobs"},{"id":"companies","title":"Companies"},{"id":"mail_and_messages","title":"Mail and messages"},
           {"id":"you","title":"You"},{"id":"sources","title":"Sources"}],
 "kinds":[
  {"id":"job_facts","title":"Job facts","description":"Reads a job's facts.","group":"jobs","subject_type":"job","engine":"model_call","automatic":true,"paused":false,"running":1,"waiting":6},
  {"id":"job_brief","title":"Job briefs","description":"Writes a short brief.","group":"jobs","subject_type":"job","engine":"model_call","automatic":true,"paused":true,"running":0,"waiting":3},
  {"id":"job_cv","title":"Tailored CVs","description":"Tailors your CV.","group":"jobs","subject_type":"job","engine":"claude_one_shot","automatic":true,"paused":false,"running":0,"waiting":0},
  {"id":"company_triage","title":"Company research","description":"Researches a company.","group":"companies","subject_type":"company","engine":"claude_agent","automatic":false,"paused":false,"running":0,"waiting":0},
  {"id":"mail_triage","title":"Mail sorting","description":"Sorts mail.","group":"mail_and_messages","subject_type":"mail_message","engine":"model_call","automatic":true,"paused":false,"running":0,"waiting":0},
  {"id":"market_gaps","title":"Market gaps","description":"Plans the gaps.","group":"you","subject_type":"none","engine":"model_call","automatic":true,"paused":false,"running":0,"waiting":2},
  {"id":"new_thing","title":"New thing","description":"A kind in a group this app doesn't know.","group":"tools","subject_type":"none","engine":"model_call","automatic":true,"paused":false,"running":0,"waiting":0}
 ]}
"""#

private func decodeKinds() throws -> RunKindsResponse {
    try HubJSON.makeDecoder().decode(RunKindsResponse.self, from: Data(kindsJSON.utf8))
}

private func getKind(_ id: String, in response: RunKindsResponse) throws -> RunKind {
    try #require(response.kinds.first { $0.id == id })
}

@Test func kindsAreGroupedInTheHubsOrderWithUnknownGroupsLast() throws {
    let response = try decodeKinds()
    let sections = RunKinds.group(response.kinds, in: response.groups)

    #expect(sections.map(\.group.title) == ["Jobs", "Companies", "Mail and messages", "You", "Tools"])
    #expect(sections[0].kinds.map(\.id) == ["job_facts", "job_brief", "job_cv"])
    #expect(sections.last?.kinds.map(\.id) == ["new_thing"])
}

@Test func eachKindsWordSaysWhatItIsDoing() throws {
    let response = try decodeKinds()

    #expect(try getKind("job_facts", in: response).getStateWord(failedCount: 0) == "running · 6 waiting")
    #expect(try getKind("job_brief", in: response).getStateWord(failedCount: 2) == "paused · 3")
    #expect(try getKind("market_gaps", in: response).getStateWord(failedCount: 0) == "2 waiting")
    #expect(try getKind("mail_triage", in: response).getStateWord(failedCount: 1) == "1 failed")
    #expect(try getKind("mail_triage", in: response).getStateWord(failedCount: 0) == "idle")
    #expect(try getKind("company_triage", in: response).getStateWord(failedCount: 0) == "on demand")
    #expect(try getKind("company_triage", in: response).getStatus(failedCount: 0) == .onDemand)
    #expect(try getKind("job_brief", in: response).getStatus(failedCount: 0) == .paused)
}

@Test func onlyKindsTheHubStartsByItselfPause() throws {
    let response = try decodeKinds()

    #expect(try getKind("job_brief", in: response).isPausable)
    #expect(try !getKind("company_triage", in: response).isPausable)
    // An on-demand kind never reads as paused, whatever the hub says.
    var research = try getKind("company_triage", in: response)
    research.paused = true
    #expect(research.getStatus(failedCount: 0) == .onDemand)
}

@Test func allKindsCountsWhatRunsWaitsAndIsPaused() throws {
    var kinds = try decodeKinds().kinds
    var all = AllRunKinds(kinds, failedCounts: ["mail_triage": 1])

    #expect(all.running == 1 && all.waiting == 11 && all.failed == 1)
    #expect(all.activityLine == "1 running · 11 waiting")
    #expect(all.pausedLine == "1 of 6 automatic kinds paused")
    #expect(all.stateWord == "1 running" && all.canPauseAll && all.canResumeAll)

    for index in kinds.indices where kinds[index].automatic {
        kinds[index].paused = true
    }
    all = AllRunKinds(kinds)
    #expect(all.pausedLine == "6 of 6 automatic kinds paused")
    #expect(all.status == .paused && all.stateWord == "paused" && !all.canPauseAll)

    let idle = AllRunKinds(kinds.map { kind in
        var kind = kind
        (kind.paused, kind.running, kind.waiting) = (false, 0, 0)
        return kind
    })
    #expect(idle.pausedLine == nil && idle.activityLine == "Nothing running" && idle.stateWord == "idle" && !idle.canResumeAll)
}

@Test func aKindsModelComesFromItsRouteOrClaude() throws {
    let response = try decodeKinds()
    let provider = ModelProvider(id: providerID, kind: "hub_runtime", name: "Local model", baseURL: "", hasKey: false, enforcesSchema: true)
    let route = TaskRoute(kind: "job_facts", providerID: providerID, model: "Qwen3.8-27B-Q4_K_M.gguf")

    let facts = try #require(RunKinds.getModel(for: getKind("job_facts", in: response), routes: [route], providers: [provider]))
    #expect(facts.name == "Qwen3.8-27B-Q4_K_M" && facts.providerName == "Local model" && facts.markSymbol == "cpu")
    let research = try #require(RunKinds.getModel(for: getKind("company_triage", in: response), routes: [route], providers: [provider]))
    #expect(research.name == "Claude" && research.markSymbol == "sparkle")
    #expect(try RunKinds.getModel(for: getKind("mail_triage", in: response), routes: [route], providers: [provider]) == nil)
}

@Test func theQueueStripSaysWhatRunsWhatIsNextAndWhatIsHeld() throws {
    let response = try decodeKinds()
    let workJSON = #"""
    {"paused":false,"running":{"kind":"job_facts","model":"qwen.gguf","priority":"background","since":"2026-10-16T17:40:00Z"},
     "waiting":[{"kind":"job_brief","model":"qwen.gguf","priority":"background","since":"2026-10-16T17:36:00Z"},
                {"kind":"market_gaps","model":"llama.gguf","priority":"background","since":"2026-10-16T17:41:00Z"}],
     "runtime":{"state":"ready","model":"qwen.gguf","busy":true},"jobs_awaiting_facts":3}
    """#
    let work = try HubJSON.makeDecoder().decode(ModelWork.self, from: Data(workJSON.utf8))

    let strip = RunKinds.describeQueue(response.kinds, work: work)
    #expect(strip == RunQueueStrip(now: "Job facts", next: "Market gaps on llama", held: "Job briefs 3"))
    #expect(RunKinds.describeQueue([], work: nil) == RunQueueStrip(now: "Nothing", next: "Nothing waits", held: "Nothing"))
}

@Test func failuresAreCountedByKindAcrossTheWindow() throws {
    let json = #"""
    {"kind":"","bucket":"day","time_zone":"UTC","totals":{},"previous":{},
     "buckets":[{"start":"2026-10-15T00:00:00Z","kinds":[{"kind":"mail_triage","runs":4,"failed":1},{"kind":"job_facts","runs":9,"failed":0}]},
                {"start":"2026-10-16T00:00:00Z","kinds":[{"kind":"mail_triage","runs":2,"failed":2}]}]}
    """#
    let metrics = try HubJSON.makeDecoder().decode(RunMetrics.self, from: Data(json.utf8))

    #expect(RunKinds.countFailures(metrics) == ["mail_triage": 3])
}

private let runsJSON = #"""
{"runs":[
  {"id":"11111111-1111-4111-8111-111111111111","record":"model_call","kind":"job_brief",
   "subject":{"type":"job","id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","title":"Staff engineer","company_id":"0aa55565-58d2-4247-ba01-cba65060a316","company_name":"Initech"},
   "model":"Qwen3.8-27B-Q4_K_M.gguf","provider":"Local model","started_at":"2026-10-16T12:00:00Z","duration_ms":4250,"outcome":"failed",
   "prompt_tokens":900,"completion_tokens":140,"error":"the model timed out","started_by_owner":false},
  {"id":"22222222-2222-4222-8222-222222222222","record":"agent_run","kind":"company_triage",
   "subject":{"type":"company","id":"0aa55565-58d2-4247-ba01-cba65060a316","title":"Initech"},
   "model":"","provider":"Claude","started_at":"2026-10-16T11:00:00Z","duration_ms":125000,"outcome":"succeeded",
   "prompt_tokens":0,"completion_tokens":0,"cost_usd_estimate":"0.4210","started_by_owner":true},
  {"id":"33333333-3333-4333-8333-333333333333","record":"agent_run","kind":"company_triage",
   "subject":{"type":"company","id":"0aa55565-58d2-4247-ba01-cba65060a316","title":""},
   "model":"","provider":"Claude","started_at":"2026-10-16T10:00:00Z","outcome":"running",
   "prompt_tokens":0,"completion_tokens":0,"started_by_owner":true},
  {"id":"44444444-4444-4444-8444-444444444444","record":"model_call","kind":"mail_triage",
   "subject":{"type":"mail_message","id":"55555555-5555-4555-8555-555555555555","title":"Your application"},
   "model":"qwen","provider":"Local model","started_at":"2026-10-16T09:00:00Z","duration_ms":800,"outcome":"succeeded",
   "prompt_tokens":0,"completion_tokens":0,"started_by_owner":false},
  {"id":"66666666-6666-4666-8666-666666666666","record":"model_call","kind":"market_gaps",
   "model":"qwen","provider":"Local model","started_at":"2026-10-16T08:00:00Z","duration_ms":800,"outcome":"succeeded",
   "prompt_tokens":10,"completion_tokens":5,"started_by_owner":false},
  {"id":"77777777-7777-4777-8777-777777777777","record":"agent_run","kind":"profile_seed","subject":{"type":"profile","title":""},
   "model":"","provider":"Claude","started_at":"2026-10-16T07:00:00Z","duration_ms":800,"outcome":"succeeded",
   "prompt_tokens":0,"completion_tokens":0,"started_by_owner":true}
 ],
 "next_cursor":"MjAyNi0xMC0xNg"}
"""#

@Test func aPageOfRunsDecodesWithItsSubjectsAndCursor() throws {
    let page = try HubJSON.makeDecoder().decode(ActivityRunsPage.self, from: Data(runsJSON.utf8))

    #expect(page.runs.count == 6 && page.hasMore)
    let brief = page.runs[0]
    #expect(brief.subject?.companyID == companyID && brief.durationMS == 4250 && brief.isFailed && !brief.isAgentRun)
    #expect(brief.modelText == "Qwen3.8-27B-Q4_K_M" && brief.durationText == "4.2 s")
    #expect(page.runs[1].isAgentRun && page.runs[1].startedByOwner && page.runs[1].modelText == "Claude")
    #expect(page.runs[1].costUSDEstimate == "0.4210" && page.runs[1].durationText == "2 min 5 s")
    #expect(page.runs[2].durationMS == nil && page.runs[2].durationText == "running")
    #expect(try HubJSON.makeDecoder().decode(ActivityRunsPage.self, from: Data(#"{"runs":[],"next_cursor":""}"#.utf8)).hasMore == false)
}

@Test func aRunsSubjectReadsAndOpensWhatItWasFor() throws {
    let runs = try HubJSON.makeDecoder().decode(ActivityRunsPage.self, from: Data(runsJSON.utf8)).runs

    #expect(runs[0].subjectParts == [
        RunSubjectPart(text: "Staff engineer", opens: .job(jobID)), RunSubjectPart(text: "Initech", opens: .company(companyID)),
    ])
    #expect(runs[0].subjectText == "Staff engineer · Initech" && runs[0].jobID == jobID && runs[0].companyID == companyID)
    #expect(runs[1].subjectParts == [RunSubjectPart(text: "Initech", opens: .company(companyID))])
    #expect(runs[1].jobID == nil && runs[1].companyID == companyID)
    // A company deleted since: named so, and opening nothing.
    #expect(runs[2].subjectParts == [RunSubjectPart(text: "A deleted company")] && runs[2].companyID == nil)
    #expect(runs[3].subjectText == "Your application" && runs[3].subjectParts[0].opens == nil)
    #expect(runs[4].subjectParts.isEmpty && runs[4].subjectText == nil)
    #expect(runs[5].subjectText == "Your profile")
}

@Test func aRunsUsageIsItsTokensOrItsCost() throws {
    let runs = try HubJSON.makeDecoder().decode(ActivityRunsPage.self, from: Data(runsJSON.utf8)).runs

    #expect(runs[0].usageText == "900 → 140 tokens")
    #expect(runs[1].usageText == 0.421.formatted(.currency(code: "USD").precision(.fractionLength(2))))
    #expect(runs[3].usageText == nil)
}

@Test func anAgentRunsAnswerReadsAsIndentedJSON() throws {
    let json = #"{"agent_run":{"id":"22222222-2222-4222-8222-222222222222","kind":"company_triage","input":"initech.example","status":"failed","result":{"jobs":2},"error":"stopped"}}"#
    let detail = try HubJSON.makeDecoder().decode(AgentRunDetailResponse.self, from: Data(json.utf8)).agentRun

    #expect(detail.input == "initech.example" && detail.resultText == "{\n  \"jobs\" : 2\n}")
    let empty = try HubJSON.makeDecoder().decode(AgentRunDetail.self, from: Data(#"{"id":"22222222-2222-4222-8222-222222222222","kind":"x","input":"","status":"running"}"#.utf8))
    #expect(empty.resultText == nil)
}

@Test func kindTitlesFollowTheHubsCatalog() {
    #expect(RunsSummary.getKindTitle("job_brief_full") == "Full briefs")
    #expect(RunsSummary.getKindTitle("job_cv") == "Tailored CVs")
    #expect(RunsSummary.getKindTitle("hiring_thread") == "Hacker News hiring thread")
    #expect(RunsSummary.getKindTitle("something_new") == "Something New")
}
