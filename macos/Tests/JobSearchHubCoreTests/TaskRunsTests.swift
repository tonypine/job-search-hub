import Foundation
@testable import JobSearchHubCore
import Testing

private func makeTaskRun(_ kind: String, model: String, milliseconds: Int, outcome: String = "succeeded") -> TaskRun {
    TaskRun(
        id: UUID(), kind: kind, subjectID: nil, baseURL: "http://localhost:1234/v1", model: model, promptVersion: 1, promptTokens: 900,
        completionTokens: 140, startedAt: Date(), durationMS: milliseconds, outcome: outcome, error: nil
    )
}

@Test func runsAreSummarizedByKindWithEachModelsAverage() {
    let started = Date(timeIntervalSince1970: 1000)
    let summaries = RunsSummary.summarize(
        taskRuns: [
            makeTaskRun("job_facts", model: "qwen", milliseconds: 4000), makeTaskRun("job_facts", model: "qwen", milliseconds: 6000),
            makeTaskRun("job_facts", model: "llama", milliseconds: 7000, outcome: "invalid"),
            makeTaskRun("mail_triage", model: "qwen", milliseconds: 2000, outcome: "failed"),
        ],
        agentRuns: [
            AgentRun(id: UUID(), kind: "profile_seed", input: "owner profile", status: "succeeded", startedAt: started, finishedAt: started.addingTimeInterval(150)),
            AgentRun(id: UUID(), kind: "profile_seed", input: "owner profile", status: "running", startedAt: started),
        ]
    )

    #expect(summaries.map(\.kind) == ["job_facts", "profile_seed", "mail_triage"])
    let facts = summaries[0]
    #expect(facts.title == "Job facts" && facts.runCount == 3 && facts.invalidCount == 1 && facts.failedCount == 0)
    #expect(facts.models.map(\.model) == ["qwen", "llama"] && facts.models[0].averageSeconds == 5)
    #expect(summaries[1].models == [RunKindSummary.ModelTiming(model: "Claude", runCount: 2, averageSeconds: 150)])
    #expect(summaries[2].failedCount == 1)
}

@Test func aTaskRunDecodes() throws {
    let json = #"{"runs":[{"id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","kind":"job_facts","subject_id":"0aa55565-58d2-4247-ba01-cba65060a316","base_url":"http://x/v1","model":"qwen","prompt_version":2,"input_hash":"ab","prompt_tokens":900,"completion_tokens":140,"started_at":"2026-09-29T12:00:00Z","duration_ms":5400,"outcome":"succeeded"}]}"#
    let run = try #require(try HubJSON.makeDecoder().decode(TaskRunsResponse.self, from: Data(json.utf8)).runs.first)
    #expect(run.durationMS == 5400 && run.subjectID != nil && run.baseURL == "http://x/v1")
}
