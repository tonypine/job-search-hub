import Foundation
@testable import JobSearchHubCore
import Testing

@Test func researchRunsTheBundledCommandWithOnlyWhatItNeeds() throws {
    #expect(CompanyResearchLaunch.makeArguments(company: " acme.com ", foundVia: "") == ["company", "add", "acme.com"])
    #expect(CompanyResearchLaunch.makeArguments(company: "Acme", foundVia: "A referral") == ["company", "add", "Acme", "--found-via", "A referral"])

    let environment = BundledHubCommand.makeEnvironment(
        from: ["HOME": "/Users/x", "CLAUDE_CODE_CHILD_SESSION": "1", "SECRET": "y"],
        hubURL: try #require(URL(string: "http://localhost:8090")), ownerToken: "token", claude: "/Users/x/.local/bin/claude"
    )
    #expect(environment["HOME"] == "/Users/x" && environment["CLAUDE_CODE_CHILD_SESSION"] == nil && environment["SECRET"] == nil)
    #expect(environment["HUB_URL"] == "http://localhost:8090" && environment["HUB_OWNER_TOKEN"] == "token")
    #expect(environment["HUB_CLAUDE_BIN"] == "/Users/x/.local/bin/claude")
}

@Test func theAddedCompanyIsReadFromTheLastLine() {
    #expect(CompanyResearchLaunch.parseCompanyID("Company id: 0aa55565-58d2-4247-ba01-cba65060a316") == UUID(uuidString: "0aa55565-58d2-4247-ba01-cba65060a316"))
    #expect(CompanyResearchLaunch.parseCompanyID("Researching acme.com (agent run 1)") == nil)
}

@Test func aReplyDraftRunsTheRecruiterReplyCommand() throws {
    let id = try #require(UUID(uuidString: "AAAAAAAA-0000-0000-0000-000000000001"))
    #expect(RecruiterReplyLaunch.makeArguments(conversationID: id) == ["recruiter", "reply", "aaaaaaaa-0000-0000-0000-000000000001"])
}


@Test func theJobFinderRunsOnTheCompanyAndReportsItsOpenJobs() throws {
    let companyID = try #require(UUID(uuidString: "9CE11299-A89A-4506-96F9-E27BF4B8D0AF"))
    #expect(JobFinderLaunch.makeArguments(companyID: companyID) == ["company", "find-jobs", "9ce11299-a89a-4506-96f9-e27bf4b8d0af"])

    let output = "  · WebFetch {…}\n\nSummary.\nJob board: workable/acme (verified: true)\nRoles recorded from the careers page: 0\nOpen jobs at the company now: 12\nCompany id: 9ce11299-a89a-4506-96f9-e27bf4b8d0af\n"
    let outcome = JobFinderLaunch.parseOutcome(output)
    #expect(outcome.openJobs == 12)
    #expect(outcome.jobBoard == "workable/acme")
    #expect(JobFinderLaunch.parseOutcome("Summary only.").openJobs == nil)
}
