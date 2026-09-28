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

