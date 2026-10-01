import Foundation
@testable import JobSearchHubCore
import Testing

private let keyStack = "11111111-1111-1111-1111-111111111111"
private let localStack = "22222222-2222-2222-2222-222222222222"
private let firstJob = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

private let recordJSON = #"""
{"id":"6c2c61df-c0cc-4c5f-a16c-cf8a00231d00","task_kind":"job_facts","title":"Key against local","status":"done",
 "created_at":"2026-09-30T21:30:02.1Z","job_ids":["\#(firstJob)"],
 "stacks":[{"id":"\#(keyStack)","label":"Answer key","source":"imported","model":"gold"},
           {"id":"\#(localStack)","label":"Qwen 3.5 9B","source":"route","provider_id":"2e416d12-77c2-4239-b419-b9fdd58c4cd6","model":"Qwen3.5-9B-Q4_K_M.gguf"}],
 "jobs":[{"id":"\#(firstJob)","title":"Frontend Engineer","company_name":"Acme"}],
 "answers":[{"stack_id":"\#(keyStack)","job_id":"\#(firstJob)","answer":{"location":{"open_to_brazil":"yes"}},
             "readings":[{"field":"location.open_to_brazil","text":"yes","evidence":"Worldwide","reason":"Says worldwide."}]},
            {"stack_id":"\#(localStack)","job_id":"\#(firstJob)","answer":{"location":{"open_to_brazil":"Yes"}},
             "readings":[{"field":"location.open_to_brazil","text":"Yes","evidence":"Remote, worldwide"}]}],
 "verdicts":[{"stack_id":"\#(localStack)","job_id":"\#(firstJob)","field":"location.open_to_brazil","verdict":"right"}],
 "summary":{"fields":["location.open_to_brazil"],
            "stacks":[{"stack_id":"\#(keyStack)","answered":1,"failed":0,"fields":[{"field":"location.open_to_brazil","agreed":0,"compared":0,"right":0,"wrong":0}]},
                      {"stack_id":"\#(localStack)","answered":1,"failed":0,"fields":[{"field":"location.open_to_brazil","agreed":1,"compared":1,"right":1,"wrong":0}]}]}}
"""#

@Test func aComparisonDecodesWithItsReadingsVerdictsAndSummary() throws {
    let record = try HubJSON.makeDecoder().decode(ComparisonRecord.self, from: Data(recordJSON.utf8))
    let local = try #require(UUID(uuidString: localStack))
    let job = try #require(UUID(uuidString: firstJob))

    #expect(record.comparison.title == "Key against local" && !record.comparison.isRunning)
    #expect(record.comparison.stacks.map(\.label) == ["Answer key", "Qwen 3.5 9B"] && record.comparison.jobIDs == [job])
    #expect(record.comparison.stacks.map(\.isImported) == [true, false])
    #expect(record.comparison.postingCountText == "1 posting")
    #expect(record.jobs.first?.companyName == "Acme")
    #expect(record.getAnswer(stackID: local, jobID: job)?.readings.first?.evidence == "Remote, worldwide")
    #expect(record.getVerdict(stackID: local, jobID: job, field: "location.open_to_brazil") == .right)
    #expect(record.getScore(stackID: local, field: "location.open_to_brazil")?.agreementText == "1 of 1")
    #expect(record.isAgreed(jobID: job, field: "location.open_to_brazil"))
}

@Test func theFirstStacksScoreReadsAsNotCompared() throws {
    let record = try HubJSON.makeDecoder().decode(ComparisonRecord.self, from: Data(recordJSON.utf8))
    let key = try #require(UUID(uuidString: keyStack))
    #expect(record.getScore(stackID: key, field: "location.open_to_brazil")?.agreementText == "—")
}

@Test func aNewComparisonAndItsVerdictsEncodeAsTheHubReadsThem() throws {
    let provider = UUID()
    let new = NewComparison(title: "Sonnet against the 9B", freshJobs: 3, stacks: [
        NewComparisonStack(label: "Sonnet", source: NewComparisonStack.claudeSource, providerID: nil, model: "sonnet"),
        NewComparisonStack(label: "9B", source: NewComparisonStack.routeSource, providerID: provider, model: "Qwen3.5-9B-Q4_K_M.gguf"),
    ])
    let encoded = try #require(String(data: HubJSON.makeEncoder().encode(new), encoding: .utf8))
    #expect(encoded.contains(#""fresh_jobs":3"#) && encoded.contains(#""provider_id":"\#(provider.uuidString)""#))

    let verdict = ComparisonVerdict(stackID: provider, jobID: provider, field: "location.open_to_brazil", verdict: .wrong)
    let verdictJSON = try #require(String(data: HubJSON.makeEncoder().encode(ComparisonVerdictsBody(verdicts: [verdict])), encoding: .utf8))
    #expect(verdictJSON.contains(#""stack_id""#) && verdictJSON.contains(#""verdict":"wrong""#))
}

@Test func aFieldsPathReadsAsATitle() {
    #expect(ComparisonSummary.formatFieldTitle("location.open_to_brazil") == "Location · open to brazil")
    #expect(ComparisonSummary.formatFieldTitle("notes") == "Notes")
}
