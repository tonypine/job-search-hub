import Foundation
@testable import JobSearchHubCore
import Testing

@Test func anInterviewPackDecodesWithItsStoriesAndGaps() throws {
    let json = #"{"job_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","model":"local","created_at":"2026-09-30T23:20:00Z","pack":{"#
        + #""questions":[{"question":"Tell us about a scheduler.","reason":"Scheduling software.","stories":[{"entry_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","title":"Built the scheduler"}],"talking_points":"No-shows fell."}],"#
        + #""role_gaps":[{"gap":"GraphQL","honest_answer":"Point to REST work."}]}}"#
    let pack = try HubJSON.makeDecoder().decode(InterviewPack.self, from: Data(json.utf8))
    #expect(pack.pack.questions.first?.stories.first?.title == "Built the scheduler")
    #expect(pack.pack.questions.first?.talkingPoints == "No-shows fell." && pack.pack.roleGaps.first?.honestAnswer == "Point to REST work.")
}
