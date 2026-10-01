import Foundation
@testable import JobSearchHubCore
import Testing

@Test func aJobFixRunsTheHubCommandWithTheNote() {
    let jobID = UUID(uuidString: "7C9E6679-7425-40DE-944B-E07FC1F90AE7")!
    #expect(JobFixLaunch.makeArguments(jobID: jobID, note: " the company is Track&Field \n") ==
        ["job", "fix", "7c9e6679-7425-40de-944b-e07fc1f90ae7", "--note", "the company is Track&Field"])
}

@Test func aFixOutcomeIsTheCommandsLastLine() {
    let fixed = JobFixLaunch.parseOutcome("Fixing job x (agent run y)\n  · update_job {…}\n\nFixed: the title is Frontend Engineer.\n", status: 0)
    #expect(fixed.fixed && fixed.summary == "Fixed: the title is Frontend Engineer.")

    let refused = JobFixLaunch.parseOutcome("Fixing job x\nNothing changed: the note doesn't say which field is wrong.\n", status: 1)
    #expect(!refused.fixed && refused.summary == "Nothing changed: the note doesn't say which field is wrong.")

    #expect(JobFixLaunch.parseOutcome("", status: 2) == (false, "exit status 2"))
}

@Test func aFixRequestAndItsTaskCarryTheJob() throws {
    let jobID = UUID()
    let encoded = try #require(String(data: HubJSON.makeEncoder().encode(QueueJobFixRequest(jobID: jobID, note: "n", claim: true)), encoding: .utf8))
    #expect(encoded.contains(#""kind":"fix_job""#) && encoded.contains(#""job_id":"\#(jobID.uuidString)""#) && encoded.contains(#""claim":true"#))

    let task = try HubJSON.makeDecoder().decode(TaskRequest.self, from: Data(#"{"id":"\#(jobID.uuidString)","kind":"fix_job","job_id":"\#(jobID.uuidString)","input":"n","status":"queued"}"#.utf8))
    #expect(task.jobID == jobID && task.kind == TaskRequest.fixJob)
}
