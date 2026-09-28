import Foundation
@testable import JobSearchHubCore
import Testing

@Test func recruitersDecodeAndFilterByHiringAndAnswer() throws {
    let json = #"""
    {"recruiters":[
      {"id":"aaaaaaaa-0000-0000-0000-000000000001","started_by_name":"Rita","started_by_url":"https://www.linkedin.com/in/rita-example",
       "owner_wrote":false,"message_count":1,"last_message_at":"2026-08-01T10:00:00Z","hiring_company":"Globex","role":"Engineer",
       "is_agency":false,"open_jobs":3,"fitting_jobs":1},
      {"id":"aaaaaaaa-0000-0000-0000-000000000002","started_by_name":"Sam","started_by_url":"https://www.linkedin.com/in/sam-example",
       "owner_wrote":true,"message_count":4,"is_agency":true,"open_jobs":0,"fitting_jobs":0}
    ]}
    """#
    let recruiters = try HubJSON.makeDecoder().decode(RecruitersResponse.self, from: Data(json.utf8)).recruiters

    #expect(recruiters[0].openingsText == "3 open, 1 fit" && recruiters[1].openingsText.isEmpty)
    #expect(RecruiterFilter(hiringNowOnly: true).apply(to: recruiters).map(\.startedByName) == ["Rita"])
    #expect(RecruiterFilter(unansweredOnly: true).apply(to: recruiters).map(\.startedByName) == ["Rita"])
    #expect(RecruiterFilter().apply(to: recruiters).count == 2)
}
