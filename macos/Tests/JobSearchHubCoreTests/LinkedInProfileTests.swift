import Foundation
@testable import JobSearchHubCore
import Testing

@Test func theProfileDecodesWithItsDifferences() throws {
    let json = #"""
    {"profile":{"headline":"Senior Software Engineer","positions":[{"company":"Globex","title":"Engineer","started_on":"Jan 2024","finished_on":""}],
                "skills":["React"]},
     "criteria_differences":["Open to recruiters is off on LinkedIn."]}
    """#
    let response = try HubJSON.makeDecoder().decode(LinkedInProfileResponse.self, from: Data(json.utf8))
    #expect(!response.profile.isEmpty && response.profile.positions?.first?.period == "Jan 2024 – now")
    #expect(response.criteriaDifferences.count == 1)

    let folder = URL(fileURLWithPath: "/tmp/export")
    let profileFiles = LinkedInArchive.findProfileFiles(in: ["Skills.csv", "messages.csv", "Jobs/Job Seeker Preferences.csv"].map { folder.appending(path: $0) })
    #expect(profileFiles.map(\.lastPathComponent) == ["Skills.csv", "Job Seeker Preferences.csv"])
}
